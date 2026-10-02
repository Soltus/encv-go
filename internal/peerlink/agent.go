package peerlink

// agent.go —— P4：远程 Agent 调用与授权（**执行端**授权模型）
//
// 安全红线（spec desktop-web-android-pairing §安全红线 / Task 4.3-4.6）：
//   1. **绝不默认同意**：挂起超时 → 自动 decline（不是 accept）。
//   2. `trust_device` 是**进程级**信任：只存内存，**重启即失效**；
//      与既有 `accept_for_session`（会话级 `sess.GrantedTools`）严格区分，两者不得混用。
//   3. **破坏性工具即使已信任也强制确认**（Task 4.6）。
//   4. 审批**只发生在执行端**（谁的文件谁点头），Hub 只是搬运工。
//   5. 审计脱敏（R14）：不记参数全文/路径全文，只记工具名、字节数、JSON 顶层键。

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
)

// ── 错误哨兵 ────────────────────────────────────────────────────────

var (
	// ErrDeclined 用户拒绝（或超时自动拒绝）
	ErrDeclined = errors.New("peerlink: declined")
	// ErrCancelled 用户取消
	ErrCancelled = errors.New("peerlink: cancelled")
	// ErrNoPending 没有对应的挂起请求（已被决策/已超时/不存在）
	ErrNoPending = errors.New("peerlink: no pending call")
	// ErrBadDecision 非法决策值
	ErrBadDecision = errors.New("peerlink: bad decision")
)

// ── 决策值 ──────────────────────────────────────────────────────────

const (
	DecisionAuto    = "auto"         // 无需人工确认（已信任且非破坏性）
	DecisionAccept  = "accept"       // 本次同意
	DecisionDecline = "decline"      // 本次拒绝
	DecisionCancel  = "cancel"       // 取消
	DecisionTrust   = "trust_device" // 同意 + 记住该设备（进程级，重启失效）
	DecisionTimeout = "timeout"      // 挂起超时 → 自动拒绝
)

// ── 报文 ────────────────────────────────────────────────────────────

const (
	// DefaultApprovalTimeout 挂起等待人工决策的时长（Task 4.3：后台/息屏 90s）
	DefaultApprovalTimeout = 90 * time.Second
	// AgentCallTimeout 远程 Agent 调用总超时（含挂起等待，必须 > 审批超时）
	AgentCallTimeout = 120 * time.Second
)

// AgentInvokeRequest 远程 Agent 调用请求（Hub → 执行端）。
type AgentInvokeRequest struct {
	CallId   string          `json:"callId"`
	Tool     string          `json:"tool"`
	Args     json.RawMessage `json:"args,omitempty"`
	FromId   string          `json:"fromId,omitempty"`   // 发起端 peerId（Hub 填充）
	FromName string          `json:"fromName,omitempty"` // 发起端名称（UI 展示用）
}

// AgentInvokeResult 远程 Agent 调用结果（执行端 → Hub）。
type AgentInvokeResult struct {
	Ok       bool            `json:"ok"`
	Decision string          `json:"decision"`         // auto|accept|decline|cancel|timeout
	Result   json.RawMessage `json:"result,omitempty"` // 工具返回的原始 JSON
	Error    string          `json:"error,omitempty"`  // 失败原因（脱敏后）
}

// ApprovalRequest 推给**执行端 UI** 的审批请求（Task 4.2）。
type ApprovalRequest struct {
	CallId      string    `json:"callId"`
	PeerId      string    `json:"peerId"`   // 发起端
	PeerName    string    `json:"peerName"` // 发起端名称
	Tool        string    `json:"tool"`
	Destructive bool      `json:"destructive"` // 破坏性（即使已信任也强制确认）
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// AuditEntry 审计条目（**脱敏**：无参数全文、无路径全文，R14）。
type AuditEntry struct {
	At          time.Time `json:"at"`
	PeerId      string    `json:"peerId"`
	Tool        string    `json:"tool"`
	Decision    string    `json:"decision"`
	Destructive bool      `json:"destructive"`
	ArgBytes    int       `json:"argBytes"`
	ArgKeys     []string  `json:"argKeys,omitempty"` // 仅 JSON 对象的顶层键
}

// ── 授权器 ──────────────────────────────────────────────────────────

// ApproverOptions 执行端授权器配置。
type ApproverOptions struct {
	// Timeout 挂起等待人工决策的时长；<=0 用 DefaultApprovalTimeout
	Timeout time.Duration
	// IsDestructive 判定工具是否破坏性（宿主注入：插件工具 needConfirm=true 即为破坏性）
	IsDestructive func(tool string) bool
	// Notify 有新审批请求时推给本端 UI（可为 nil）
	Notify func(ApprovalRequest)
	// AuditLimit 审计环形缓冲上限；<=0 用 200
	AuditLimit int
}

type pendingCall struct {
	req    ApprovalRequest
	done   chan string
	closed bool
}

// Approver 执行端授权器。
//
// ⚠️ 全部状态**只在进程内存**：`trusted` 与 `pending` 均不落盘、不写配置，
//
//	进程重启后信任态必然为空（Task 4.4 的回归锁见 agent_test.go）。
type Approver struct {
	mu      sync.Mutex
	pending map[string]*pendingCall
	trusted map[string]bool // peerId → 已信任（进程级）
	audit   []AuditEntry
	opts    ApproverOptions
	seq     int
}

func NewApprover(opts ApproverOptions) *Approver {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultApprovalTimeout
	}
	if opts.AuditLimit <= 0 {
		opts.AuditLimit = 200
	}
	return &Approver{
		pending: make(map[string]*pendingCall),
		trusted: make(map[string]bool),
		opts:    opts,
	}
}

func (a *Approver) destructive(tool string) bool {
	if a.opts.IsDestructive == nil {
		return false
	}
	return a.opts.IsDestructive(tool)
}

// RequiresApproval 是否需要人工确认（纯查询，不改状态）。
// 规则：**破坏性工具永远要确认**；否则已信任的设备免确认。
func (a *Approver) RequiresApproval(peerId, tool string) bool {
	if a.destructive(tool) {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.trusted[peerId]
}

// Require 判定决策：无需确认时立即返回 DecisionAuto；
// 否则挂起等待本端 UI 决策，超时**自动 decline**（绝不默认同意）。
func (a *Approver) Require(ctx context.Context, req AgentInvokeRequest, peerId, peerName string) (string, error) {
	destructive := a.destructive(req.Tool)
	if !destructive {
		a.mu.Lock()
		trusted := a.trusted[peerId]
		a.mu.Unlock()
		if trusted {
			a.record(peerId, req, DecisionAuto, destructive)
			return DecisionAuto, nil
		}
	}

	callId := req.CallId
	if callId == "" {
		a.mu.Lock()
		a.seq++
		callId = "call-" + time.Now().Format("20060102-150405.000") + "-" + itoa(a.seq)
		a.mu.Unlock()
	}

	now := time.Now()
	ar := ApprovalRequest{
		CallId:      callId,
		PeerId:      peerId,
		PeerName:    peerName,
		Tool:        req.Tool,
		Destructive: destructive,
		CreatedAt:   now,
		ExpiresAt:   now.Add(a.opts.Timeout),
	}
	p := &pendingCall{req: ar, done: make(chan string, 1)}

	a.mu.Lock()
	a.pending[callId] = p
	a.mu.Unlock()

	if a.opts.Notify != nil {
		a.opts.Notify(ar)
	}

	// 等待决策 / 超时 / 上游取消
	var decision string
	select {
	case decision = <-p.done:
	case <-time.After(a.opts.Timeout):
		decision = DecisionTimeout // ⚠️ 超时 = 拒绝，绝不默认同意
	case <-ctx.Done():
		decision = DecisionCancel
	}

	a.mu.Lock()
	delete(a.pending, callId)
	p.closed = true
	a.mu.Unlock()

	a.record(peerId, req, decision, destructive)

	switch decision {
	case DecisionAccept, DecisionTrust:
		// trust_device：本次同意 + 记住设备（**仅进程内存**）
		if decision == DecisionTrust {
			a.mu.Lock()
			a.trusted[peerId] = true
			a.mu.Unlock()
		}
		return decision, nil
	case DecisionDecline:
		return decision, ErrDeclined
	case DecisionTimeout:
		return decision, ErrDeclined
	default: // cancel / 其它
		return decision, ErrCancelled
	}
}

// Decide 执行端 UI 提交决策（accept | decline | cancel | trust_device）。
func (a *Approver) Decide(callId, decision string) error {
	switch decision {
	case DecisionAccept, DecisionDecline, DecisionCancel, DecisionTrust:
	default:
		return ErrBadDecision
	}
	a.mu.Lock()
	p, ok := a.pending[callId]
	a.mu.Unlock()
	if !ok || p == nil || p.closed {
		return ErrNoPending
	}
	select {
	case p.done <- decision:
		return nil
	default:
		return ErrNoPending
	}
}

// Pending 当前挂起的审批请求（执行端 UI 拉取）。
func (a *Approver) Pending() []ApprovalRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ApprovalRequest, 0, len(a.pending))
	for _, p := range a.pending {
		out = append(out, p.req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// Trust / Untrust / Trusted / TrustedPeers —— 进程级信任态（重启失效）
func (a *Approver) Trust(peerId string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.trusted[peerId] = true
}

func (a *Approver) Untrust(peerId string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.trusted, peerId)
}

func (a *Approver) Trusted(peerId string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.trusted[peerId]
}

func (a *Approver) TrustedPeers() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.trusted))
	for k, v := range a.trusted {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Audit 审计（脱敏）快照，按时间正序。
func (a *Approver) Audit() []AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]AuditEntry, len(a.audit))
	copy(out, a.audit)
	return out
}

// record 写审计：**只记元数据，绝不记参数/路径全文**（R14）
func (a *Approver) record(peerId string, req AgentInvokeRequest, decision string, destructive bool) {
	e := AuditEntry{
		At:          time.Now(),
		PeerId:      peerId,
		Tool:        req.Tool,
		Decision:    decision,
		Destructive: destructive,
		ArgBytes:    len(req.Args),
	}
	var obj map[string]json.RawMessage
	if len(req.Args) > 0 && json.Unmarshal(req.Args, &obj) == nil && len(obj) > 0 {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		e.ArgKeys = keys
	}
	a.mu.Lock()
	a.audit = append(a.audit, e)
	if len(a.audit) > a.opts.AuditLimit {
		a.audit = a.audit[len(a.audit)-a.opts.AuditLimit:]
	}
	a.mu.Unlock()
}

// truncateErr 截断错误文本（避免把路径/参数全文带回发起端，R14）
func truncateErr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
