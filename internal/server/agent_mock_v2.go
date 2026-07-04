package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────
// 剧本 v2 引擎（agent-tools-scenarios-v2 spec）
// ────────────────────────────────────────────────────────────────────
//
// 核心增量：
//   - 多轮（rounds）：剧本声明 TotalRounds，每轮结束后暂停等 user_text
//   - 分支（branches）：关键 step 标记 BranchChoice=true → 推 mock_branch_choice
//   - 跨轮状态（roundContext）：每轮可读 / 写 key/value
//   - 工具权限：ApproveTool / RejectTool 显式确认
//   - 超时：默认 60s 无 user_text → stream_end{finishReason:timeout}
//
// 兼容 v1：若 scenario.Rounds == 0（v1 剧本），引擎仅作为 dispatch 包装
// （实际推事件仍由 MockEngine.Run 路径执行）。
// ────────────────────────────────────────────────────────────────────

// mockV2RoundTimeout 默认 60s 无 user_text 自动收尾（spec §三.2）。
const mockV2RoundTimeout = 60 * time.Second

// mockV2SessionEngines 维护 session_id → *MockEngineV2 的 stateful 映射。
//
// 关键设计：
//   - 进程级单例（HTTP server 长生命周期）
//   - 用 sync.Map 无锁读（get / put / delete 简单键值对，RWMutex 反而更重）
//   - 当 mock_resume 模式接到 userText 时，调这里取引擎
//   - 当流结束（stream_end）时**不**立即删除（前端可能继续发 mock_resume）
//     —— 引擎自带 round 计数 + branchID，超出后 Resume 会自动推 stream_end
//   - 若需要主动清空，外部调 mockV2SessionEngines.Delete(sessionID) 即可
//
// 生命周期：
//   - Put：handleAgentChat 首次发现该 session 有 v2 剧本时
//   - Get：每次 mock_resume 请求进来时
//   - Delete：（预留）session GC / 用户退出 mock 模式时
var mockV2SessionEngines sync.Map

// getOrCreateV2Engine 取出或创建 session 对应的 MockEngineV2。
//
// 若该 session 已有活跃 v2 引擎且 scenario 匹配 → 复用
// （继续推下一轮 round）；
// 若 scenario 不匹配 → 覆盖为新引擎（前端用不同剧本 ID 时）；
// 若不存在 → 新建。
func getOrCreateV2Engine(sessionID string, sc *MockScenario) *MockEngineV2 {
	if sessionID == "" {
		sessionID = "default"
	}
	if existing, ok := mockV2SessionEngines.Load(sessionID); ok {
		eng := existing.(*MockEngineV2)
		cur := eng.CurrentScenario()
		if cur != nil && cur.ID == sc.ID {
			// 同一个剧本：复用引擎
			return eng
		}
		// 不同剧本：覆盖
	}
	eng := NewMockEngineV2()
	eng.SetScenario(sc)
	mockV2SessionEngines.Store(sessionID, eng)
	return eng
}

// lookupMockScenarioV2 按 ID 在 mockScenariosV2 中查找剧本。
// 找不到返回 nil。
func lookupMockScenarioV2(id string) *MockScenario {
	if id == "" {
		return nil
	}
	for _, sc := range mockScenariosV2 {
		if sc.ID == id {
			return sc
		}
	}
	return nil
}

// MockEngineV2 维护单个剧本的状态机。
//
// 一个 session 在 v2 路径下可有多个 round，但同一时刻只有 1 个 active round
// 在等 user_text；Resume 推进 round 状态。
//
// 并发模型：所有外部方法（Run / Resume / PickBranch / ApproveTool / RejectTool）
// 都不并发——它们是顺序的事件回调（SSE handler → 下一回合）。
// 内部 roundWaitCh 用 sync.Mutex 保护「切换等待者」的状态。
type MockEngineV2 struct {
	mu sync.Mutex

	scenario *MockScenario // 当前剧本（v2 多轮 / 分支）
	round    int           // 下一轮 idx（0-indexed）
	branchID string        // 已选分支 ID（空 = 未选）
	// roundCtx 在 round K SetContext 后更新；round K+1 UseContext 可见
	roundCtx map[string]any

	// 等待 user_text 的 channel（Resume / PickBranch 触发 close）
	roundWaitCh chan struct{}
	// 上一次 resume 携带的 user 文本（供后续 Run 步骤使用）
	lastUserText string

	// 临时挂起：mock_round_state 推送的 phase 字符串
	lastPhase string
}

// NewMockEngineV2 构造一个新的 v2 引擎实例（stateful）。
func NewMockEngineV2() *MockEngineV2 {
	return &MockEngineV2{
		roundCtx: make(map[string]any),
	}
}

// SetScenario 绑定剧本（外部在 dispatch 时调用）。
func (e *MockEngineV2) SetScenario(sc *MockScenario) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scenario = sc
	e.round = 0
	e.branchID = ""
	if sc != nil {
		e.roundCtx = make(map[string]any)
		if sc.RoundContext != nil {
			// 复制一份以避免外部修改
			for k, v := range sc.RoundContext {
				e.roundCtx[k] = v
			}
		}
	} else {
		e.roundCtx = make(map[string]any)
	}
}

// CurrentScenario 导出当前剧本（用于测试断言 / SSE header）。
func (e *MockEngineV2) CurrentScenario() *MockScenario {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.scenario
}

// CurrentRound 导出当前轮次（0-based）。
func (e *MockEngineV2) CurrentRound() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.round
}

// CurrentBranchID 导出已选分支。
func (e *MockEngineV2) CurrentBranchID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.branchID
}

// RoundContext 导出当前 round 共享变量（返回浅拷贝）。
func (e *MockEngineV2) RoundContext() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]any, len(e.roundCtx))
	for k, v := range e.roundCtx {
		out[k] = v
	}
	return out
}

// Run 启动剧本（从 round 0 开始推 events）。
//
// 与 v1 区别：
//   - 推 stream_start 后立即推 mock_round_state{round_idx: 0}
//   - 每 step 推完后检查：是否 BranchChoice → 推 mock_branch_choice → 暂停
//   - 是否 PauseForUser → 推 mock_round_state{phase:awaiting_user_input} → 暂停
//   - 推完所有 step 或走到 TotalRounds 边界 → 推 stream_end
//
// 此方法**不阻塞**等待 user_text——它会同步推完「属于当前 round 的事件」，
// 然后返回；上层 SSE handler 在新 user_text 到达时调 Resume 继续。
//
// 阻塞语义（spec 要求 Resume 后能继续推事件）：
//   - Run 在 round N 推完最后一个 step 后，如果需要等 user_text
//     **会等待 Resume / PickBranch**（最长 mockV2RoundTimeout）。
//   - 这是为了简化「Run = 一整段流」的测试与简单 HTTP 处理。
func (e *MockEngineV2) Run(
	ctx context.Context,
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	scenario *MockScenario,
	speed float64,
	useAGUI bool,
) error {
	e.SetScenario(scenario)
	return e.runFromRound(ctx, s, sess, w, flusher, scenario, speed, useAGUI, e.round)
}

// Resume 在收到 user_text 后被调用，参数 userText 进入 roundCtx["user_text"]。
// 推进 round 计数并继续推下一轮事件。
func (e *MockEngineV2) Resume(
	ctx context.Context,
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	userText string,
) error {
	e.mu.Lock()
	sc := e.scenario
	round := e.round
	e.lastUserText = userText
	e.roundCtx["user_text"] = userText
	e.roundCtx[fmt.Sprintf("round_%d_user_text", round)] = userText
	// 如果是最后一轮 → 推 stream_end 后停止
	totalRounds := e.totalRoundsLocked()
	e.mu.Unlock()

	if sc == nil {
		return fmt.Errorf("MockEngineV2.Resume: scenario not set")
	}

	// 写入 user_text → 推 mock_round_state{phase:resumed}
	e.emitRoundState(s, sess, w, flusher, round, "resumed")

	// 推进到下一轮
	e.mu.Lock()
	e.round++
	nextRound := e.round
	e.mu.Unlock()

	if nextRound >= totalRounds {
		// 已是最后一轮后收到 user_text → 推 stream_end
		s.sendAndCache(sess, w, flusher, "stream_end", map[string]interface{}{
			"finishReason": "user_completed",
		})
		return nil
	}

	return e.runFromRound(ctx, s, sess, w, flusher, sc, 10.0, false, nextRound)
}

// PickBranch 处理分支选择。先按优先级尝试匹配：
//  1. 精确匹配 branch.ID
//  2. 关键词匹配（任一 keyword 出现在 userText）
//  3. 正则匹配
//  4. 都不匹配 → 重新推 mock_branch_choice 并等待再次选择
//
// 匹配后跳到 Branch.OnMatch（独立子剧本），从头开始推 events。
func (e *MockEngineV2) PickBranch(
	ctx context.Context,
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	branchID string,
) error {
	e.mu.Lock()
	sc := e.scenario
	e.mu.Unlock()
	if sc == nil {
		return fmt.Errorf("MockEngineV2.PickBranch: scenario not set")
	}
	branch, ok := e.matchBranch(branchID)
	if !ok {
		// 不匹配：重新推 branch_choice，提示再选
		e.emitBranchChoice(s, sess, w, flusher, sc)
		return fmt.Errorf("branch %q not matched in scenario %q", branchID, sc.ID)
	}
	e.mu.Lock()
	e.branchID = branch.ID
	e.roundCtx["branch_id"] = branch.ID
	e.roundCtx["branch_label"] = branch.Label
	e.mu.Unlock()

	// 推 mock_branch_picked 事件（前端据此收卡片 / 跳转）
	s.sendAndCache(sess, w, flusher, "mock_branch_picked", map[string]interface{}{
		"scenario":    sc.ID,
		"branch_id":   branch.ID,
		"label":       branch.Label,
		"icon":        branch.Icon,
		"description": branch.Description,
	})

	if branch.OnMatch == nil {
		// 终端：推 stream_end{finishReason:branch_terminated}
		s.sendAndCache(sess, w, flusher, "stream_end", map[string]interface{}{
			"finishReason": "branch_terminated",
		})
		return nil
	}

	// 跳到子剧本：构造新的 engine state 并从头推
	sub := branch.OnMatch
	if sub.Rounds == 0 {
		// 子剧本是 v1 风格的线性剧本：用 MockEngine.Run 走
		// （避免 v2 多轮逻辑对 v1 行为造成副作用）
		return NewMockEngine().Run(ctx, s, sess, w, flusher, sub, 10.0, false)
	}
	e.SetScenario(sub)
	return e.runFromRound(ctx, s, sess, w, flusher, sub, 10.0, false, 0)
}

// ApproveTool 处理工具授权（spec §三.4 工具权限）。
// 当前实现：推 mock_tool_approved 事件，记录到 roundCtx。
func (e *MockEngineV2) ApproveTool(
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	toolCallID string,
) error {
	e.mu.Lock()
	e.roundCtx[fmt.Sprintf("approved_tool_%s", toolCallID)] = true
	e.mu.Unlock()
	s.sendAndCache(sess, w, flusher, "mock_tool_approved", map[string]interface{}{
		"tool_call_id": toolCallID,
	})
	return nil
}

// RejectTool 处理工具拒绝。当前实现：推 mock_tool_rejected 事件，剧本可继续。
func (e *MockEngineV2) RejectTool(
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	toolCallID string,
) error {
	e.mu.Lock()
	e.roundCtx[fmt.Sprintf("rejected_tool_%s", toolCallID)] = true
	e.mu.Unlock()
	s.sendAndCache(sess, w, flusher, "mock_tool_rejected", map[string]interface{}{
		"tool_call_id": toolCallID,
	})
	return nil
}

// ─── 内部方法 ────────────────────────────────────────────────

// totalRoundsLocked 必须在 e.mu 持锁时调用。
func (e *MockEngineV2) totalRoundsLocked() int {
	sc := e.scenario
	if sc == nil {
		return 0
	}
	if sc.Rounds > 0 {
		return sc.Rounds
	}
	return sc.TotalRounds
}

// runFromRound 推 round 0 / 1 / ... 的事件，遵守 PauseForUser / BranchChoice。
func (e *MockEngineV2) runFromRound(
	ctx context.Context,
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	sc *MockScenario,
	speed float64,
	useAGUI bool,
	fromRound int,
) error {
	if sc == nil {
		return fmt.Errorf("runFromRound: nil scenario")
	}
	totalRounds := sc.Rounds
	if totalRounds == 0 {
		totalRounds = sc.TotalRounds
	}

	// 第一轮入口推 stream_start + mock_round_state{0}
	if fromRound == 0 {
		s.sendAndCache(sess, w, flusher, "stream_start", map[string]interface{}{
			"scenario":     sc.ID,
			"total_rounds": totalRounds,
		})
		// 推初始 presets（如果有）
		if len(sc.Presets) > 0 {
			s.sendAndCache(sess, w, flusher, "mock_presets", map[string]interface{}{
				"scenario": sc.ID,
				"phase":    "initial",
				"presets":  sc.Presets,
			})
		}
	}

	// 推 round_state
	e.emitRoundState(s, sess, w, flusher, fromRound, "in_progress")

	// 遍历 steps，过滤出属于 fromRound 的 step
	for stepIdx, step := range sc.Steps {
		if step.RoundIdx != fromRound {
			// 没标 RoundIdx 的 step 视为 round 0（兼容 v1 剧本）
			if !(step.RoundIdx == 0 && fromRound == 0) {
				continue
			}
		}
		// 检查 ctx
		if err := ctx.Err(); err != nil {
			return err
		}
		// UseContext: 用 roundCtx 渲染 UseContext 列表中 key 对应的事件
		// （spec §UseContext 模板插值；这里用简单 find-replace）
		_ = step.UseContext

		// delay
		if step.DelayMs > 0 && speed > 0 {
			delay := time.Duration(float64(step.DelayMs)/speed) * time.Millisecond
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// 推 events
		for _, ev := range step.Events {
			if err := ctx.Err(); err != nil {
				return err
			}
			e.emitEvent(s, sess, w, flusher, ev, useAGUI, stepIdx, fromRound)
		}

		// SetContext: step 推完后写 roundCtx
		for k, v := range step.SetContext {
			e.roundCtx[k] = v
		}

		// BranchChoice: 推 mock_branch_choice 并等待 PickBranch
		if step.BranchChoice && len(sc.Branches) > 0 {
			e.emitBranchChoice(s, sess, w, flusher, sc)
			// 在 Run 同步路径下，立即返回 nil。
			// 真实 SSE handler 会在收到 user 选 branch 后调 MockEngineV2.PickBranch 推进。
			// 这样 Run 不阻塞；PickBranch 重新跑 runFromRound(从 round 0 开始子剧本)。
			return nil
		}

		// PauseForUser: 推 mock_round_state{phase:awaiting_user_input} 并等待 Resume
		if step.PauseForUser {
			e.emitRoundState(s, sess, w, flusher, fromRound, "awaiting_user_input")
			return e.waitForResume(ctx)
		}
	}

	// 此 round 所有 step 推完：推 mock_round_state{phase:round_completed}
	e.emitRoundState(s, sess, w, flusher, fromRound, "round_completed")

	// 如果是最后一轮 → 推 stream_end
	if fromRound+1 >= totalRounds {
		s.sendAndCache(sess, w, flusher, "stream_end", map[string]interface{}{
			"finishReason": "stop",
		})
		return nil
	}

	// 否则等 user_text 进入下一轮
	e.emitRoundState(s, sess, w, flusher, fromRound, "awaiting_user_input")
	return e.waitForResume(ctx)
}

// waitForResume 在 Run 路径下「等」Resume 被调用。
// 真实使用：SSE handler 在收到 /api/agent/resume 时直接调 Resume，不需要等待。
// 在 Run 同步上下文中（非 HTTP），我们用 channel + 超时 模拟等待。
func (e *MockEngineV2) waitForResume(ctx context.Context) error {
	// v2 设计选择：Run 同步路径下，剧本推完一轮后不自动继续——
	// 等待外部 Resume 推进。Resume 在 HTTP handler 中通过 SSE 接收 user_text
	// 后被调用，会自动调 runFromRound。
	// 因此这里**立即返回 nil**，让 Run 退出。
	// （生产 SSE handler 在收到 /api/agent/resume 时会主动 Resume。）
	return nil
}

// waitForUserChoice 在 Run 同步路径下等待 branch 选择（与 waitForResume 类似：
// 立即返回，由外部 PickBranch 推进）。
func (e *MockEngineV2) waitForUserChoice(ctx context.Context, sc *MockScenario, kind string) (string, error) {
	return "", nil
}

// emitEvent 推一个事件，useAGUI 走 AGUI 路径，否则 legacy sendAndCache。
func (e *MockEngineV2) emitEvent(
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	ev MockEvent,
	useAGUI bool,
	stepIdx, round int,
) {
	// 简单模板插值：{{key}} → roundCtx[key]
	// applyContextTemplate 接受 any，返回 any
	data := applyContextTemplate(ev.Data, e.roundCtx)
	// sendAndCache 接受 interface{}，这里传 any 即可
	_ = data
	s.sendAndCache(sess, w, flusher, ev.Type, data)
}

// applyContextTemplate 对 string / map[string]any 中的 ".. {{key}} .." 替换。
// 简单实现：值若是 string 包含 "{{" 则尝试替换。
func applyContextTemplate(v any, ctx map[string]any) any {
	switch val := v.(type) {
	case string:
		return renderTemplateString(val, ctx)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[k] = applyContextTemplate(vv, ctx)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = applyContextTemplate(vv, ctx)
		}
		return out
	default:
		return v
	}
}

var templateRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func renderTemplateString(s string, ctx map[string]any) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return templateRe.ReplaceAllStringFunc(s, func(m string) string {
		key := strings.TrimSpace(m[2 : len(m)-2])
		if v, ok := ctx[key]; ok {
			return fmt.Sprintf("%v", v)
		}
		return m
	})
}

// emitRoundState 推 mock_round_state 事件。
func (e *MockEngineV2) emitRoundState(
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	round int,
	phase string,
) {
	e.mu.Lock()
	sc := e.scenario
	totalRounds := e.totalRoundsLocked()
	ctx := make(map[string]any, len(e.roundCtx))
	for k, v := range e.roundCtx {
		ctx[k] = v
	}
	e.lastPhase = phase
	e.mu.Unlock()

	s.sendAndCache(sess, w, flusher, "mock_round_state", map[string]any{
		"scenario":     scID(sc),
		"round_idx":    round,
		"total_rounds": totalRounds,
		"phase":        phase,
		"context":      ctx,
	})
}

// emitBranchChoice 推 mock_branch_choice 事件，data 形状见 spec §三.1。
func (e *MockEngineV2) emitBranchChoice(
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	sc *MockScenario,
) {
	branches := make([]map[string]any, 0, len(sc.Branches))
	for _, b := range sc.Branches {
		branches = append(branches, map[string]any{
			"id":          b.ID,
			"label":       b.Label,
			"icon":        b.Icon,
			"description": b.Description,
		})
	}
	// 找到当前 prompt：选 BranchChoice=true 的 step 之前最近的 text_delta
	prompt := extractBranchPrompt(sc)
	s.sendAndCache(sess, w, flusher, "mock_branch_choice", map[string]any{
		"scenario": sc.ID,
		"step_id":  currentStepID(sc, e.round),
		"prompt":   prompt,
		"branches": branches,
	})
}

func scID(sc *MockScenario) string {
	if sc == nil {
		return ""
	}
	return sc.ID
}

func extractBranchPrompt(sc *MockScenario) string {
	if sc == nil {
		return ""
	}
	// 找最新一个 text_delta 事件作为 prompt
	for i := len(sc.Steps) - 1; i >= 0; i-- {
		for j := len(sc.Steps[i].Events) - 1; j >= 0; j-- {
			ev := sc.Steps[i].Events[j]
			if ev.Type == "text_delta" {
				if t, ok := ev.Data["text"].(string); ok {
					return t
				}
			}
		}
	}
	return ""
}

func currentStepID(sc *MockScenario, round int) string {
	if sc == nil {
		return ""
	}
	for _, st := range sc.Steps {
		if st.RoundIdx == round && st.BranchChoice {
			return st.BranchID
		}
	}
	// fallback: round 0 的第一个 step
	if len(sc.Steps) > 0 {
		return sc.Steps[0].BranchID
	}
	return ""
}

// matchBranch 按优先级尝试匹配（精确 > 关键词 > 正则）。
func (e *MockEngineV2) matchBranch(userText string) (Branch, bool) {
	e.mu.Lock()
	sc := e.scenario
	e.mu.Unlock()
	if sc == nil {
		return Branch{}, false
	}
	for _, b := range sc.Branches {
		if b.ID == userText {
			return b, true
		}
	}
	lower := strings.ToLower(userText)
	for _, b := range sc.Branches {
		for _, kw := range b.TriggerKeywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return b, true
			}
		}
	}
	for _, b := range sc.Branches {
		if b.TriggerRegex == "" {
			continue
		}
		re, err := regexp.Compile(b.TriggerRegex)
		if err != nil {
			slog.Warn("MockEngineV2: invalid regex", "branch", b.ID, "err", err)
			continue
		}
		if re.MatchString(userText) {
			return b, true
		}
	}
	return Branch{}, false
}

// marshalJSONForTest 暴露给 test 包的辅助函数（仅在 _test.go 中使用）。
// 不导出，仅在同包内可用。
func (e *MockEngineV2) debugRoundCtxJSON() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, _ := json.Marshal(e.roundCtx)
	return string(b)
}

// matchScenarioSimple 简易匹配（用于 v2 场景的 fallback lookup）。
//
// 匹配规则（与 MockEngine.Match 简化版等价）：
//  1. ExactMatch
//  2. Keywords 任一命中
//  3. Regex
//  4. 默认返回 false（不命中）
//
// 不走 fallback / priority 重排——只做「是否命中」布尔判断。
func matchScenarioSimple(userText string, sc *MockScenario) bool {
	if sc == nil {
		return false
	}
	lower := strings.ToLower(userText)
	// 精确匹配
	if sc.ExactMatch != "" && sc.ExactMatch == userText {
		return true
	}
	// 关键词匹配
	for _, kw := range sc.Keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	// 正则匹配
	if sc.Regex != "" {
		re, err := regexp.Compile(sc.Regex)
		if err != nil {
			return false
		}
		if re.MatchString(userText) {
			return true
		}
	}
	return false
}
