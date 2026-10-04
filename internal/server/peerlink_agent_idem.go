package server

// peerlink_agent_idem.go —— 远程 Agent 调用的**幂等表**（执行端）
//
// 为什么必须有（2026-10-04 真机压测发现）：
//
//	同一 callId 重复提交时，工具会被**完整地执行第二次**（实测两次调用都返回 ok:true）。
//	只读工具（list_mounts/list_files）看不出差别，但换成 **写类工具**（加密 / 解密 / 删除）
//	时，一次网络重试或用户误双击就是两次副作用 —— 这是不可接受的事故。
//
// 设计取舍：
//
//	- 只在**执行端（Edge）**做：执行发生在那里，Hub 并不知道对端是否真的跑过。
//	- 进程内存、有界 + TTL：与 Approver 的 trust / audit 保持同一套存储纪律
//	  （不落盘、重启即失效）。重启后重复保护会短暂失效，这是可接受的权衡；
//	  反过来若强行落盘，就要承担"到底有没有执行过"的跨进程一致性问题。
//	- **并发重复要共用一次执行**：不是简单 cache 结果，而是让第二个请求等在
//	  entry.done 上 —— 否则审批还在挂起时重复提交会**给用户弹两次审批框**。
//	- **失败不入表**：被 decline / 执行报错的请求应当允许重试（用户可能改主意），
//	  只有成功结果才会被后续重复调用复用。

import (
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
)

const (
	// agentIdemLimit 幂等记录上限（与 peerlink.Approver 的审计缓冲同数量级）
	agentIdemLimit = 200
	// agentIdemTTL 记录有效期；超时自动淘汰，避免长期占内存
	agentIdemTTL = 10 * time.Minute
)

type agentIdemEntry struct {
	created time.Time
	done    chan struct{}
	out     peerlink.AgentInvokeOutcome
}

type agentIdemTable struct {
	mu      sync.Mutex
	entries map[string]*agentIdemEntry
	order   []string // FIFO：超限时淘汰最老的一条
}

func newAgentIdemTable() *agentIdemTable {
	return &agentIdemTable{entries: make(map[string]*agentIdemEntry)}
}

// begin 登记一次执行。isNew=false 表示同 key 已在处理/已完成 ⇒ 调用方应等待 entry.done。
func (t *agentIdemTable) begin(key string, now time.Time) (entry *agentIdemEntry, isNew bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.evictLocked(now)
	if e, ok := t.entries[key]; ok {
		return e, false
	}
	e := &agentIdemEntry{created: now, done: make(chan struct{})}
	t.entries[key] = e
	t.order = append(t.order, key)
	for len(t.order) > agentIdemLimit {
		oldest := t.order[0]
		t.order = t.order[1:]
		delete(t.entries, oldest)
	}
	return e, true
}

// drop 让该 key 失效（执行失败时用 ⇒ 允许重试）。
func (t *agentIdemTable) drop(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.entries[key]; !ok {
		return
	}
	delete(t.entries, key)
	for i, k := range t.order {
		if k == key {
			t.order = append(t.order[:i], t.order[i+1:]...)
			return
		}
	}
}

// evictLocked 清掉过期记录（调用方持锁）。顺手修复 order/entries 不一致的状态。
func (t *agentIdemTable) evictLocked(now time.Time) {
	i := 0
	for i < len(t.order) {
		k := t.order[i]
		e, ok := t.entries[k]
		if !ok {
			t.order = append(t.order[:i], t.order[i+1:]...)
			continue
		}
		if now.Sub(e.created) > agentIdemTTL {
			delete(t.entries, k)
			t.order = append(t.order[:i], t.order[i+1:]...)
			continue
		}
		i++
	}
}
