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

type peerlinkLimiters struct {
	search *rateWindow // 联邦搜索：30 次/分钟
	file   *rateWindow // 远端读：60 次/分钟
	agent  *rateWindow // 远程 Agent：10 次/分钟
	call   *circuitBreaker
}

func (s *Server) peerLimits() *peerlinkLimiters {
	s.peerLimitMu.Lock()
	defer s.peerLimitMu.Unlock()
	if s.peerLimiter == nil {
		s.peerLimiter = &peerlinkLimiters{
			search: newRateWindow(30, time.Minute),
			file:   newRateWindow(60, time.Minute),
			agent:  newRateWindow(10, time.Minute),
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

// ── 测试钩子（仅测试用：把阈值/冷却调小，避免测试跑几十秒）──────────

func (s *Server) setPeerLimitsForTests(threshold int, cooldown time.Duration) {
	l := s.peerLimits()
	l.call.threshold = threshold
	l.call.cooldown = cooldown
}
