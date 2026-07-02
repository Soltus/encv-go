// internal/server/agent_mock_executor.go
//
// 剧本执行器 — 核心职责：
//  1. 接收一个 step 的 events 列表
//  2. 遇到 tool_call → 自动调 ToolRegistry.Execute(name, args)
//  3. 自动生成 tool_result 事件（id / name / isError / result 全部来自真实执行）
//  4. 把 tool_result 推入流（**不**依赖 YAML 里的声明）
//  5. YAML 里的 events 列表**不**包含 tool_result（schema 校验已拒绝）
//
// 与原 agent_mock.go §Run 关系：
//   - 原 Run 仍保留作为"事件序列 → SSE 推送"的统一通道
//   - 本 executor 是 Run 内部处理 tool_call 时的"自动执行"分支
//   - 旧剧本（Go 字面量）若有显式 tool_result event，按原逻辑走（向后兼容）
//   - 新剧本（YAML 转换后）的 tool_result event 由 schema auto-injected，
//     __yaml_auto_generated=true 标记 → executor 必须调真实工具覆盖
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Soltus/encv-go/internal/tools"
)

// ════════════════════════════════════════════════════════════════
// 工具执行抽象
// ════════════════════════════════════════════════════════════════

// ToolExecutor 是 executor 调用真实工具的统一接口。
//
// 实际实现：(*Server).executeAgentTool — 走 tools.GlobalRegistry.Dispatch 派发。
// 单测可注入 mock 实现返回固定 JSON。
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, name, argsJSON string) (ToolExecutorResult, error)
}

// ToolExecutorResult 是 executor 拿到的工具执行结果。
//
// 与 tools.ToolResult 字段一致，但避免循环引用。
type ToolExecutorResult struct {
	Result     string // JSON 字符串
	IsError    bool
	Status     string // "success" / "failed" / "cancelled"
	DurationMs int64
}

// ════════════════════════════════════════════════════════════════
// executor 实例
// ════════════════════════════════════════════════════════════════

// MockExecutor 是 step 级别的执行器。
//
// 每次 Run() 调用前构造一个（绑定 ctx / s / sess / writer），
// step 内 tool_call 事件用 exec.Execute() 触发真实工具。
type MockExecutor struct {
	ctx     context.Context
	s       *Server
	sess    *agentSession
	w       http.ResponseWriter
	flusher http.Flusher

	// 收集所有已完成 tool_result（id → JSON 结果），供动态文本模板使用
	collectedResults map[string]toolResultInfo
}

// newMockExecutor 构造 step 级别执行器。
func newMockExecutor(ctx context.Context, s *Server, sess *agentSession, w http.ResponseWriter, flusher http.Flusher) *MockExecutor {
	return &MockExecutor{
		ctx:              ctx,
		s:                s,
		sess:             sess,
		w:                w,
		flusher:          flusher,
		collectedResults: make(map[string]toolResultInfo),
	}
}

// executeStep 处理一个 MockStep 的 events 列表。
//
// 关键改造：
//   - 遇到 tool_call → 立即调真实工具（不依赖 YAML 里是否声明 tool_result）
//   - 自动 push tool_call + tool_status(running) + 真实结果 + tool_status(success/failed)
//   - YAML auto-injected tool_result（__yaml_auto_generated=true）会被此函数**覆盖**
func (e *MockExecutor) executeStep(step MockStep, scenario *MockScenario, stepIdx int, emitEvent func(ev MockEvent, stepIdx, evIdx int)) error {
	for evIdx, ev := range step.Events {
		// ctx 取消检查
		select {
		case <-e.ctx.Done():
			return e.ctx.Err()
		default:
		}

		switch ev.Type {
		case "tool_call":
			if err := e.handleToolCall(ev, scenario, stepIdx, evIdx, emitEvent); err != nil {
				return err
			}

		case "tool_result":
			// YAML 转换后注入的 auto tool_result：必须由真实执行覆盖
			if autoGen, _ := ev.Data["__yaml_auto_generated"].(bool); autoGen {
				// 已被 handleToolCall 处理（推了真实 tool_result）→ 跳过
				continue
			}
			// 旧剧本（Go 字面量）显式声明的 tool_result：原样推送
			e.collectResult(ev)
			emitEvent(ev, stepIdx, evIdx)

		case "mid_stream_disconnect", "sse_corrupt_chunk", "text_delta",
			"text_delta_templated", "reasoning_delta", "stream_status",
			"stream_start", "stream_end", "mock_presets", "mock_presets_clear",
			"mock_branch_choice":
			// 委托给 emitEvent（由 Run 包装为 sendAndCache / aguiMapper）
			emitEvent(ev, stepIdx, evIdx)

		default:
			// 未知类型：原样推送（向后兼容）
			emitEvent(ev, stepIdx, evIdx)
		}
	}
	return nil
}

// handleToolCall 处理 tool_call 事件：调真实工具 + 推送完整事件序列。
//
// 事件序列：
//  1. tool_call (声明)
//  2. tool_status(running)
//  3. [execute_real=true] 真实执行：
//     - tool_status(success/failed)
//     - tool_result (id / name / result / isError / status / durationMs)
//
// execute_real 默认为 true（v2 YAML 模式），保持向后兼容。
// 显式 execute_real=false 才跳过真实执行（用于演示失败路径 / mock 错误信息）。
func (e *MockExecutor) handleToolCall(ev MockEvent, scenario *MockScenario, stepIdx, evIdx int, emitEvent func(ev MockEvent, stepIdx, evIdx int)) error {
	name, _ := ev.Data["name"].(string)
	id, _ := ev.Data["id"].(string)
	if name == "" || id == "" {
		slog.Warn("mock executor: skip tool_call (missing name or id)",
			"scenario", scenario.ID, "step", stepIdx, "ev", evIdx)
		return nil
	}

	// 序列化 args：YAML 给出的是 map[string]any，需转回 JSON 字符串
	argsJSON, err := argsToJSON(ev.Data["args"])
	if err != nil {
		slog.Warn("mock executor: args serialization failed, using {}",
			"id", id, "name", name, "error", err)
		argsJSON = "{}"
	}

	// 1. 推 tool_call
	emitEvent(ev, stepIdx, evIdx)

	// 2. 推 tool_status(running) — 不在 YAML 里强制要求，但前端依赖此事件显示进度
	if _, hasStatus := ev.Data["auto_run"]; hasStatus {
		emitEvent(MockEvent{
			Type: "tool_status",
			Data: map[string]interface{}{
				"id":     id,
				"status": "running",
			},
		}, stepIdx, evIdx)
	}

	// 3. execute_real 字段处理
	executeReal, _ := ev.Data["execute_real"].(bool)
	if !executeReal {
		// YAML 显式 execute_real=false → 仅推 tool_call + status，不实际执行
		// 用于演示"用户取消工具"等特殊路径
		emitEvent(MockEvent{
			Type: "tool_status",
			Data: map[string]interface{}{"id": id, "status": "cancelled"},
		}, stepIdx, evIdx)
		return nil
	}

	// 4. 真实执行
	t0 := time.Now()
	result, execErr := e.s.executeAgentToolAsExecutor(e.ctx, name, argsJSON)
	dur := time.Since(t0).Milliseconds()

	if execErr != nil && result.Result == "" {
		// executor 出错（unknown tool / dispatch 失败）→ isError=true
		result = ToolExecutorResult{
			Result:     fmt.Sprintf(`{"error":%q}`, execErr.Error()),
			IsError:    true,
			Status:     "failed",
			DurationMs: dur,
		}
	} else if result.DurationMs == 0 {
		result.DurationMs = dur
	}

	// 5. 推 tool_status(success / failed)
	statusVal := "success"
	if result.IsError {
		statusVal = "failed"
	}
	emitEvent(MockEvent{
		Type: "tool_status",
		Data: map[string]interface{}{"id": id, "status": statusVal},
	}, stepIdx, evIdx)

	// 6. 推真实 tool_result
	emitEvent(MockEvent{
		Type: "tool_result",
		Data: map[string]interface{}{
			"id":         id,
			"name":       name,
			"result":     result.Result,
			"isError":    result.IsError,
			"status":     result.Status,
			"durationMs": result.DurationMs,
		},
	}, stepIdx, evIdx)

	// 7. 收集结果供动态模板使用
	e.collectedResults[id] = toolResultInfo{
		id:     id,
		name:   name,
		result: result.Result,
	}

	return nil
}

// collectResult 收集旧剧本（Go 字面量）tool_result 结果。
func (e *MockExecutor) collectResult(ev MockEvent) {
	id, _ := ev.Data["id"].(string)
	name, _ := ev.Data["name"].(string)
	result, _ := ev.Data["result"].(string)
	if id == "" || result == "" {
		return
	}
	e.collectedResults[id] = toolResultInfo{id: id, name: name, result: result}
}

// argsToJSON 把 YAML 反序列化得到的 args（map[string]any）转回 JSON 字符串。
func argsToJSON(v interface{}) (string, error) {
	if v == nil {
		return "{}", nil
	}
	// 已经是 string（YAML 里直接写 JSON 字符串）→ 透传
	if s, ok := v.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ════════════════════════════════════════════════════════════════
// Server.executeAgentToolAsExecutor — Server 暴露给 executor 的钩子
// ════════════════════════════════════════════════════════════════

// executeAgentToolAsExecutor 是 Server 暴露给 MockExecutor 的工具执行入口。
//
// 内部走 tools.GlobalRegistry.Dispatch（与真实 LLM 路径完全一致）。
// 返回 ToolExecutorResult 而非 tools.ToolResult 是为了切断 server ↔ tools 的循环引用。
func (s *Server) executeAgentToolAsExecutor(ctx context.Context, name, argsJSON string) (ToolExecutorResult, error) {
	if s.toolDeps == nil {
		return ToolExecutorResult{}, fmt.Errorf("server: toolDeps not initialized")
	}
	res, err := tools.GlobalRegistry.Dispatch(ctx, name, argsJSON, s.toolDeps)
	if err != nil && res.Result == "" {
		return ToolExecutorResult{
			Result:  fmt.Sprintf(`{"error":%q}`, err.Error()),
			IsError: true,
			Status:  "failed",
		}, err
	}
	return ToolExecutorResult{
		Result:     res.Result,
		IsError:    res.IsError,
		Status:     res.Status,
		DurationMs: res.DurationMs,
	}, nil
}
