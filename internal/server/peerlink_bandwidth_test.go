package server

// peerlink_bandwidth_test.go —— 主动测速的契约锁
//
// 为什么要主动测（2026-10-04 实测对照）：
//
//	网卡标称 10000 Mbps，而向公网下载实测仅 ≈3.35 Mbps —— **虚高约 3000 倍**。
//	照标称值算限额等于不限流。所以限额必须由**真实下载吞吐**校准。
//
// 这些用例用**本地 httptest 服务器**做确定性验证（不依赖外网、不受网络波动影响）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeBandwidthMbps_LocalServer(t *testing.T) {
	// 造一个"已知速率"的源：每次写 64KB，间隔 8ms ⇒ 约 8MB/s（64Mbps）
	const chunk = 64 << 10
	const gap = 8 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for sent := 0; sent < 8<<20; sent += chunk {
			if _, err := w.Write(make([]byte, chunk)); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(gap)
		}
	}))
	defer srv.Close()

	t.Setenv(envSpeedtestURL, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	mbps, err := probeBandwidthMbps(ctx)
	if err != nil {
		t.Fatalf("本地源探测失败: %v", err)
	}
	// 期望 ~64Mbps；给宽松区间，因为 CI 上调度抖动会让结果偏低/偏高
	if mbps < 20 || mbps > 200 {
		t.Fatalf("实测速率 %.1f Mbps 明显偏离期望(~64Mbps)，换算或计时有问题", mbps)
	}
	t.Logf("measured %.2f Mbps", mbps)
}

func TestProbeBandwidthMbps_RejectsUntrustworthySamples(t *testing.T) {
	t.Run("样本太小（读得太少不可信）", func(t *testing.T) {
		// 只给 1KB：低于 speedtestMinBytes ⇒ 必须判为不可信，而不是返回个乐观值
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(make([]byte, 1024))
		}))
		defer srv.Close()
		t.Setenv(envSpeedtestURL, srv.URL)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := probeBandwidthMbps(ctx); err == nil {
			t.Fatal("样本过小必须报错（宁缺勿滥：错误的宽松值会让限流失效）")
		}
	})

	t.Run("非 200 状态必须报错", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		t.Setenv(envSpeedtestURL, srv.URL)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := probeBandwidthMbps(ctx); err == nil {
			t.Fatal("HTTP 非 200 必须报错")
		}
	})
}

// TestRateQuota_SetLimitKeepsAccounting 动态放宽上限时**不能清空已记账的 hits**。
//
// 否则：测速一完成，先前按保守档累计的额度就归零 ⇒ 等于给正在传输的会话
// "突然提速"，反而可能瞬时打满链路（限流被自己绕过）。
func TestRateQuota_SetLimitKeepsAccounting(t *testing.T) {
	q := newRateQuota(100, time.Minute)
	now := time.Now()

	q.consume("k", now, 90) // 逼近上限
	q.setLimit(1000)         // 测速完成，放宽

	if ok, _ := q.allow("k", now.Add(time.Millisecond)); !ok {
		t.Fatal("放宽后应放行（但仅限新的额度）")
	}
	// 关键：之前记账的 90 字节必须仍然计入
	if used := quotaUsedForTest(q, "k", now.Add(time.Millisecond)); used < 90 {
		t.Fatalf("setLimit 清空了历史记账（used=%d < 90）⇒ 正在传输的会话会被突然放开", used)
	}
}

func TestBandwidthProbe_SkippedWhenEnvOverrideSet(t *testing.T) {
	// 运维显式指定带宽时，不应启动联网探测
	t.Setenv(envLinkMbpsOverride, "50")
	// startBandwidthProbeIfNeeded 里会直接 return；这里只验证它不 panic 且立即返回
	s := &Server{}
	done := make(chan struct{})
	go func() {
		s.startBandwidthProbeIfNeeded()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("设了覆盖变量时不应阻塞")
	}
}

func TestIsTimeoutErr(t *testing.T) {
	if !isTimeoutErr(context.DeadlineExceeded) {
		t.Fatal("context deadline 应识别为超时")
	}
	if isTimeoutErr(nil) {
		t.Fatal("nil 不是超时")
	}
	if !isTimeoutErr(&timeoutStringError{strings.NewReplacer().Replace("Client.Timeout exceeded")}) {
		t.Fatal("含 Client.Timeout 的错误应识别为超时")
	}
}

type timeoutStringError struct{ s string }

func (e *timeoutStringError) Error() string { return e.s }

// quotaUsedForTest 汇总某 key 当前窗口内已计的字节（测试辅助）。
func quotaUsedForTest(q *rateQuota, key string, now time.Time) int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	cut := now.Add(-q.window)
	var used int64
	for _, h := range q.hits[key] {
		if h.at.After(cut) {
			used += h.weight
		}
	}
	return used
}
