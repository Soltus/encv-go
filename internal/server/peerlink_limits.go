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
	// rateLimitFileBytes 远端读的**流量**上限（字节/分钟）。
	//
	//	取值依据：在线播放/预览是本通道的正当用途，中等码率（~1MB/s）连续播放一分钟
	//	约 60MB ⇒ 取 64MB/分钟既能流畅播放，又能挡住"无限搬数据"。
	//	（旧实现按次数 60/min：默认单片下约 15MB/min 偏紧，而单片调大后又能到 240MB/min
	//	 失控 —— 换成按字节后这个问题消失。）
	rateLimitFileBytes = 64 << 20 // 64 MiB
	rateLimitAgent     = 100
)

type peerlinkLimiters struct {
	search *rateWindow // 联邦搜索（按次数）
	file   *rateQuota  // 远端读（**按流量**，原因见 rateQuota 注释）
	agent  *rateWindow // 远程 Agent（按次数）
	call   *circuitBreaker
}

func (s *Server) peerLimits() *peerlinkLimiters {
	s.peerLimitMu.Lock()
	defer s.peerLimitMu.Unlock()
	if s.peerLimiter == nil {
		s.peerLimiter = &peerlinkLimiters{
			search: newRateWindow(rateLimitSearch, time.Minute),
			file:   newRateQuota(rateLimitFileBytes, time.Minute),
			agent:  newRateWindow(rateLimitAgent, time.Minute),
			call:   newCircuitBreaker(5, 30*time.Second),
		}
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
