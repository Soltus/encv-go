package server

// peerlink_limits.go —— P5 / R12：速率限制 + 失败熔断
//
// 为什么需要：
//   - Hub 跑在**公网**（cnb）⇒ 端点会被扫描/误撞；没有限流等于把本机索引与
//     文件读通道暴露给无限次调用（R13 的报文上限挡不住"高频小报文"）。
//   - 对端不可达时若无熔断，前端轮询会持续打 RPC，每次都等满超时 ⇒ 雪崩。
//
// ⚠️ 全部状态**只存进程内存**（与 psk/token 同一套存储纪律）：
//    重启后计数归零，这是刻意的（不把限流状态落盘，避免隐私与复杂度）。

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── 速率限制（滑动窗口计数，够用且实现简单可测）────────────────────

type rateWindow struct {
	limit  int
	window time.Duration
	mu     sync.Mutex
	hits   map[string][]time.Time // key → 最近命中的时间戳
}

func newRateWindow(limit int, window time.Duration) *rateWindow {
	return &rateWindow{limit: limit, window: window, hits: make(map[string][]time.Time)}
}

// allow 判定是否放行；不放行时返回建议的重试等待时间。
func (r *rateWindow) allow(key string, now time.Time) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cut := now.Add(-r.window)
	ts := r.hits[key]
	kept := ts[:0]
	for _, t := range ts {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= r.limit {
		r.hits[key] = kept
		return false, r.window - now.Sub(kept[0])
	}
	r.hits[key] = append(kept, now)
	return true, 0
}

// ── 配额（按**字节数**计权的滑动窗口）──────────────────────────────
//
// 2026-10-04：远端读（file）换成这个。原因 ——
//
//	远端读是**分片**的（单片上限 MaxReadChunk=4MB），同一个文件读下来要几十次调用。
//	按"调用次数"限制根本拦不住真正要防的东西（**流量**），却会把"读一个大文件"
//	这种完全正常的使用卡死（旧值 60 次/分钟 ⇔ 默认单片下约 15MB/分钟，而单片
//	一旦设大就直接飙到 240MB/分钟 —— 约束与风险都不成比例）。
//
//	改成按字节计费后：约束的是"这段时间搬了多少数据"，与分片大小/分片次数无关
//	⇒ 既不会误伤正常使用，也不会因为调大单片而失控。
type quotaHit struct {
	at     time.Time
	weight int64 // 本次消耗的字节数
}

type rateQuota struct {
	limit  int64
	window time.Duration
	mu     sync.Mutex
	hits   map[string][]quotaHit
}

func newRateQuota(limitBytes int64, window time.Duration) *rateQuota {
	return &rateQuota{limit: limitBytes, window: window, hits: make(map[string][]quotaHit)}
}

// setLimit 动态调整上限（主动测速完成后用）。
//
// ⚠️ 只改上限，**不清空已记账的 hits**：否则测速一完成，先前用保守档积累的额度就归零，
// 等于给正在传输的会话"突然提速"，反而可能瞬时打满链路。
func (r *rateQuota) setLimit(limitBytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limit = limitBytes
}

// allow 判定是否还有额度（**不扣减**）；不足时返回建议等待时间。
func (r *rateQuota) allow(key string, now time.Time) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var used int64
	oldest := now
	cut := now.Add(-r.window)
	kept := r.hits[key][:0]
	for _, h := range r.hits[key] {
		if h.at.After(cut) {
			kept = append(kept, h)
			used += h.weight
			if h.at.Before(oldest) {
				oldest = h.at
			}
		}
	}
	r.hits[key] = kept
	if used >= r.limit {
		return false, r.window - now.Sub(oldest)
	}
	return true, 0
}

// consume 按实际字节数扣减（在拿到响应之后调用）。
func (r *rateQuota) consume(key string, now time.Time, bytes int64) {
	if bytes <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hits[key] = append(r.hits[key], quotaHit{at: now, weight: bytes})
}

// ── 熔断（连续失败 → 开路冷却 → 半开探测）──────────────────────────

type cbState struct {
	failures  int
	openUntil time.Time
	lastErr   string
}

type circuitBreaker struct {
	mu          sync.Mutex
	states      map[string]*cbState
	threshold   int           // 连续失败多少次开路
	cooldown    time.Duration // 开路冷却时长
	halfOpenMax int           // 冷却后允许的探测次数（简化：每次冷却结束放行 1 次）
}

func newCircuitBreaker(threshold int, cooldown time.Duration) *circuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &circuitBreaker{states: make(map[string]*cbState), threshold: threshold, cooldown: cooldown}
}

// allow 是否允许发起调用（开路期间直接拒绝，不再消耗超时预算）
func (c *circuitBreaker) allow(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.states[key]
	if st == nil {
		return true
	}
	if st.failures < c.threshold {
		return true
	}
	return now.After(st.openUntil) // 冷却结束 → 半开（放行这一次探测）
}

func (c *circuitBreaker) recordSuccess(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, key)
}

func (c *circuitBreaker) recordFailure(key string, err string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.states[key]
	if st == nil {
		st = &cbState{}
		c.states[key] = st
	}
	// 冷却期内的失败不重复累加（避免把 openUntil 无限前推）
	if st.failures >= c.threshold && now.Before(st.openUntil) {
		st.lastErr = err
		return
	}
	st.failures++
	st.lastErr = err
	if st.failures >= c.threshold {
		st.openUntil = now.Add(c.cooldown)
	}
}

func (c *circuitBreaker) state(key string) (failures int, open bool, retryAfter time.Duration, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now = time.Now()
	st := c.states[key]
	if st == nil {
		return 0, false, 0, now
	}
	open = st.failures >= c.threshold && now.Before(st.openUntil)
	if open {
		retryAfter = st.openUntil.Sub(now)
	}
	return st.failures, open, retryAfter, now
}

// ── 组装 ────────────────────────────────────────────────────────────

// 限流阈值（次/分钟）。
//
// ⚠️ 2026-10-04 调整 `agent`：10 → **100**。
//
//	10 次/分钟是为"公网 Hub 防扫描"定的，但**远程调试是交互式**的：
//	列目录 → 展开几层 → 逐个读文件，随手点几下就撞线，然后被冷却 ~60s，
//	表现为"点几下就卡住不动"，非常像功能坏了（真机压测时我自己就被卡过）。
//	100/min 仍远超人类交互频率，防扫描的目的不失效。
//
//	`file` 保持 60：远端读是**分片**的（单片 256KB），读大文件会连续多次调用，
//	这里偏低同样是真实痛点 —— 若后续反馈"大文件在线打开很慢"，优先调它。
const (
	rateLimitSearch = 30
	rateLimitAgent  = 100

	// 远端读流量上限的取值策略（2026-10-04，用户指令）：
	//
	//	固定值（如 64MB/min）**毫无意义** —— 机器可能是百兆小盒子，也可能是万兆服务器，
	//	一个常数对前者太宽、对后者太紧。所以改为**按实测链路带宽的 90%**，
	//	并以 **1Mbps** 作为保底（实测不出来/极低时也不能小到没法用）。
	fileQuotaLinkUtilization = 0.9
	fileQuotaFloorMbps       = 1.0
	// envLinkMbpsOverride 覆盖实测值。
	//
	//	为什么需要：容器里的 veth/bridge 网卡 `speed` 经常**虚高**（本机实测报 10000Mbps
	//	即 10G，而真实可用带宽可能低得多），照着它算等于不限流。运维可显式指定真值。
	envLinkMbpsOverride = "ENCV_PEERLINK_LINK_MBPS"
)

// measureLinkMbps 实测本机可用链路速率（Mbps）；测不到返回 0。
//
// 数据来源：Linux 的 `/sys/class/net/<iface>/speed`（单位 Mbps）。
// 只认 operstate=up 的**非回环**接口；多网卡取最快的一个（服务器口径）。
func measureLinkMbps() float64 {
	if v := strings.TrimSpace(os.Getenv(envLinkMbpsOverride)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	ents, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return 0
	}
	var best float64
	for _, e := range ents {
		name := e.Name()
		if name == "lo" {
			continue
		}
		base := filepath.Join("/sys/class/net", name)
		if st, err := os.ReadFile(filepath.Join(base, "operstate")); err == nil {
			if strings.TrimSpace(string(st)) != "up" {
				continue
			}
		}
		raw, err := os.ReadFile(filepath.Join(base, "speed"))
		if err != nil {
			continue // 虚拟接口常常没有这个文件
		}
		mbps, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
		if err != nil || mbps <= 0 {
			continue // -1 / 0 表示未知
		}
		if mbps > best {
			best = mbps
		}
	}
	return best
}

// quotaBytesPerMinFor 把链路 Mbps 换算成每分钟字节配额：
// **实测带宽 × 90%**，且**不低于 1Mbps** 保底。
//
// 抽成纯函数是为了能直接单测换算（Mbps 是**比特**，要 /8 才是字节）。
func quotaBytesPerMinFor(linkMbps float64) int64 {
	utilMbps := linkMbps * fileQuotaLinkUtilization
	if utilMbps < fileQuotaFloorMbps {
		utilMbps = fileQuotaFloorMbps // 实测失败/极低 ⇒ 走保底档
	}
	return int64(utilMbps * 1e6 / 8 * 60)
}

// fileQuotaBytesPerMin 本机远端读的流量上限（字节/分钟）。
//
// ⚠️ 这里返回的是**保守初值**（1Mbps 保底档），真实值由后台主动测速校准
// （见 peerlink_bandwidth.go）。原因：网卡标称速率**不能当可用带宽用**
// —— 本机标称 10000Mbps 而实测仅 3.35Mbps，虚高约 3000 倍。
func fileQuotaBytesPerMin() int64 {
	if v := strings.TrimSpace(os.Getenv(envLinkMbpsOverride)); v != "" {
		if mbps, err := strconv.ParseFloat(v, 64); err == nil && mbps > 0 {
			q := quotaBytesPerMinFor(mbps)
			slog.Info("peerlink file quota from env override", "linkMbps", mbps, "bytesPerMin", q)
			return q
		}
	}
	// 保守初值：宁可先限紧，也不能有"启动头几秒不限流"的窗口
	q := quotaBytesPerMinFor(0)
	slog.Info("peerlink file quota conservative until bandwidth probe completes",
		"bytesPerMin", q, "linkSpeedMbps", measureLinkMbps())
	return q
}

type peerlinkLimiters struct {
	search *rateWindow // 联邦搜索（按次数）
	file   *rateQuota  // 远端读（**按流量**：实测带宽 × 90%）
	agent  *rateWindow // 远程 Agent（按次数）
	call   *circuitBreaker
}

func (s *Server) peerLimits() *peerlinkLimiters {
	s.peerLimitMu.Lock()
	defer s.peerLimitMu.Unlock()
	if s.peerLimiter == nil {
		s.peerLimiter = &peerlinkLimiters{
			search: newRateWindow(rateLimitSearch, time.Minute),
			file:   newRateQuota(fileQuotaBytesPerMin(), time.Minute),
			agent:  newRateWindow(rateLimitAgent, time.Minute),
			call:   newCircuitBreaker(5, 30*time.Second),
		}
		// 首次创建限流器时启动后台测速循环（幂等），实测带宽回来后放宽 file 限额
		go s.startBandwidthProbeIfNeeded()
	}
	return s.peerLimiter
}

// rateAllow 分类限流；超限返回 false + 建议等待时间。
func (s *Server) rateAllow(kind, key string) (bool, time.Duration) {
	l := s.peerLimits()
	now := time.Now()
	switch kind {
	case "search":
		return l.search.allow(key, now)
	case "file":
		return l.file.allow(key, now)
	case "agent":
		return l.agent.allow(key, now)
	default:
		return true, 0
	}
}

// fileQuotaConsume 远端读**拿到响应后**按实际字节数扣减额度。
//
// ⚠️ 必须在实际读取之后记账：调用前并不知道这一片会有多大（单片大小可由调用方指定，
// 上限 4MB），事前估算要么过松要么过紧。
func (s *Server) fileQuotaConsume(key string, bytes int64) {
	s.peerLimits().file.consume(key, time.Now(), bytes)
}

// ── 测试钩子（仅测试用：把阈值/冷却调小，避免测试跑几十秒）──────────

func (s *Server) setPeerLimitsForTests(threshold int, cooldown time.Duration) {
	l := s.peerLimits()
	l.call.threshold = threshold
	l.call.cooldown = cooldown
}
