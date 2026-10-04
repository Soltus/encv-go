package server

// peerlink_bandwidth.go —— **主动测速**（下载法）与远端读限额的动态校准
//
// 为什么必须主动测（2026-10-04）：
//
//	`/sys/class/net/*/speed` 报的是**网卡标称速率**，不是真实可用带宽。
//	本机实测对照：
//	    标称：10000 Mbps（eth0 10G）
//	    真实：≈ 3.35 Mbps（向公网下载 5MB 实测 11.9s）
//	**虚高约 3000 倍**。照标称值算限额 = 9000Mbps ≈ 1GB/s，而真实链路只有 0.4MB/s
//	⇒ 限流彻底失效，远端读能把链路打满（同机其它服务、在线播放都会卡）。
//
// 所以：启动时先按**保守档**（1Mbps 保底）限着，后台异步做一次真实测速，测完再放宽；
// 之后按周期重测（链路可能变化：切 WiFi/蜂窝/换网络）。
//
// 不想让它联网测速？设 ENCV_PEERLINK_LINK_MBPS 直接指定，则完全跳过探测。

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// envSpeedtestURL 测速源（可覆盖）。默认用 Cloudflare 的按字节下载端点。
	envSpeedtestURL = "ENCV_PEERLINK_SPEEDTEST_URL"
	defaultSpeedtestURL = "https://speed.cloudflare.com/__down?bytes=8000000"

	// speedtestMaxDuration 时间盒：**慢链路也能测**的关键 —— 不要求"必须下完 N MB"，
	// 而是"最多花这么久"，否则 3Mbps 的机器测 8MB 要 20s，根本没法在启动期做。
	speedtestMaxDuration = 6 * time.Second
	// speedtestMaxBytes 字节上限（防止极快网络下无限读）。
	speedtestMaxBytes = 16 << 20
	// 有效样本门槛：读得太少或时间太短都会让结果失真（TCP 握手/TLS 握手占比过大），
	// 这种样本宁可丢弃，也不要用一个错误的宽松值去放开限流。
	speedtestMinBytes    = 128 << 10
	speedtestMinDuration = 500 * time.Millisecond
	// speedtestRefreshInterval 重测间隔（链路可能变化）。
	speedtestRefreshInterval = 5 * time.Minute
)

// probeBandwidthMbps 下载法实测下行带宽（Mbps）。
//
// 返回值与错误：样本不可信（读得太少/时间太短/HTTP 非 200）时返回错误，
// 调用方应**保留原有保守限额**，而不是退化成网卡标称值。
func probeBandwidthMbps(ctx context.Context) (float64, error) {
	url := strings.TrimSpace(os.Getenv(envSpeedtestURL))
	if url == "" {
		url = defaultSpeedtestURL
	}

	// 超时比时间盒多留一点余量（连接建立也要时间）
	cctx, cancel := context.WithTimeout(ctx, speedtestMaxDuration+3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build speedtest request: %w", err)
	}

	// ⚠️ 强制 IPv4 —— 2026-10-04 真机验证踩到的坑：
	//	Go 默认会先试 IPv6(AAAA)，而不少容器/网络里 IPv6 **没有路由**，
	//	于是光等 IPv6 失败就把连接耗掉 5s+，直接顶穿整个超时（本机实测 client.Do 超时，
	//	而同一地址 curl 200 —— 差别就在 curl 有 happy-eyeballs 并行回退、Go 串行等）。
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	client := &http.Client{
		Timeout: speedtestMaxDuration + 3*time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true, // 测速要的是持续吞吐，复用连接会测到 RTT 抖动
			Proxy:             http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp4", addr)
			},
		},
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("speedtest request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("speedtest status %d", resp.StatusCode)
	}

	// 时间盒内尽量多读；ctx 到点就停，已读的字节依然是有效样本
	n, copyErr := io.Copy(io.Discard, io.LimitReader(resp.Body, speedtestMaxBytes))
	elapsed := time.Since(start)
	// copyErr 含 context deadline（时间盒到）是预期情况，不算失败
	if n <= 0 && copyErr != nil && !isTimeoutErr(copyErr) {
		return 0, fmt.Errorf("speedtest read: %w", copyErr)
	}

	// 样本可信度检查（宁缺勿滥：错误的宽松值会让限流失效）
	if n < speedtestMinBytes {
		return 0, fmt.Errorf("speedtest sample too small: %d bytes in %s", n, elapsed)
	}
	if elapsed < speedtestMinDuration {
		return 0, fmt.Errorf("speedtest sample too short: %d bytes in %s", n, elapsed)
	}

	mbps := float64(n) * 8 / elapsed.Seconds() / 1e6
	return mbps, nil
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "Client.Timeout") ||
		strings.Contains(s, "timeout")
}

// bandwidthOnce 保证"首次使用时启动一次后台测速循环"。
var bandwidthOnce sync.Once

// startBandwidthProbeIfNeeded 启动后台测速循环（幂等）。
//
// 初值刻意**保守**（1Mbps 保底档）：在测速完成前宁可限得紧，也不能出现
// "启动头几秒不限流" 的窗口 —— 那正是限流最容易被击穿的时刻。
// 测速成功后再放宽到实测的 90%。
func (s *Server) startBandwidthProbeIfNeeded() {
	if strings.TrimSpace(os.Getenv(envLinkMbpsOverride)) != "" {
		// 运维已显式指定 ⇒ 完全跳过探测
		return
	}
	bandwidthOnce.Do(func() {
		go s.bandwidthProbeLoop()
	})
}

func (s *Server) bandwidthProbeLoop() {
	refresh := func() {
		ctx, cancel := context.WithTimeout(context.Background(), speedtestMaxDuration+5*time.Second)
		defer cancel()
		mbps, err := probeBandwidthMbps(ctx)
		if err != nil {
			slog.Warn("peerlink bandwidth probe failed, keeping conservative quota",
				"err", err,
				"hint", "可用 "+envLinkMbpsOverride+" 或 "+envSpeedtestURL+" 手动指定")
			return
		}
		quota := quotaBytesPerMinFor(mbps)
		s.peerLimits().file.setLimit(quota)
		slog.Info("peerlink bandwidth probed, file quota updated",
			"measuredMbps", mbps, "bytesPerMin", quota)
	}

	refresh()
	t := time.NewTicker(speedtestRefreshInterval)
	defer t.Stop()
	for range t.C {
		refresh()
	}
}
