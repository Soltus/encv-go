package server

// peerlink_agent_calllog.go —— **发起端**的远程调用台账
//
// 为什么需要（2026-10-04 真机压测发现的可观测性缺口）：
//
//	`/api/peerlink/agent/audit` 读的是 **Approver.Audit()**，而 Approver 只在
//	**执行端**工作（谁的文件谁点头）⇒ 桌面端（发起端）永远查不到"我远程调用过什么"。
//	对"远程读别人手机上的文件"这种能力，这是合规与排障的硬缺口：
//	出问题只能在对端（手机）上翻日志，而手机上的日志又没人去看。
//
// 设计纪律（与 Approver 的 trust / audit 一致）：
//   - **只存进程内存**，重启即失效（不落盘 ⇒ 不引入隐私与一致性问题）
//   - **脱敏**：不记参数全文 / 路径全文，只记 argBytes 与顶层 argKeys（R14）
//   - 有界环形缓冲，避免长期运行吃内存

import (
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// agentCallLogLimit 发起端台账上限
const agentCallLogLimit = 200

// AgentCallEntry 发起端的一条调用记录。
type AgentCallEntry struct {
	At       time.Time `json:"at"`
	PeerId   string    `json:"peerId"`
	PeerName string    `json:"peerName"`
	Tool     string    `json:"tool"`
	CallId   string    `json:"callId,omitempty"`
	// Decision 执行端授权器的真实决策（auto / accept / trust_device / decline / cancel ...）
	Decision string `json:"decision"`
	// Ok 该次调用是否成功拿到结果
	Ok bool `json:"ok"`
	// ErrorType 失败类别（脱敏：只记类型，不记对端返回的细节文本）
	ErrorType string   `json:"errorType,omitempty"`
	ArgBytes  int      `json:"argBytes"`
	ArgKeys   []string `json:"argKeys,omitempty"`
}

type agentCallLog struct {
	mu      sync.Mutex
	entries []AgentCallEntry
}

func (l *agentCallLog) add(e AgentCallEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	if len(l.entries) > agentCallLogLimit {
		// 丢弃最老的一半，避免每次追加都搬移整个切片
		keep := len(l.entries) - agentCallLogLimit/2
		l.entries = append([]AgentCallEntry(nil), l.entries[keep:]...)
	}
}

func (l *agentCallLog) snapshot() []AgentCallEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]AgentCallEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

// agentCallLogGet 惰性创建（进程内存）。
func (s *Server) agentCallLogGet() *agentCallLog {
	s.agentCallLogMu.Lock()
	defer s.agentCallLogMu.Unlock()
	if s.agentCallLog == nil {
		s.agentCallLog = &agentCallLog{}
	}
	return s.agentCallLog
}

// argKeysOf 取 args 的**顶层键**（脱敏：不记值）。
func argKeysOf(args json.RawMessage) (int, []string) {
	if len(args) == 0 {
		return 0, nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(args, &obj) != nil || len(obj) == 0 {
		return len(args), nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return len(args), keys
}
