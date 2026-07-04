// internal/server/agent_agui_adapter.go
//
// AG-UI 协议适配层 —— 将内部自定义 SSE 事件映射为标准 AG-UI 格式。
//
// AG-UI (Agent User Interface) 是一种标准化的 agent 通信协议，
// 定义了 RUN_STARTED / TEXT_MESSAGE_START / TEXT_MESSAGE_CONTENT /
// TEXT_MESSAGE_END / TOOL_CALL_START / TOOL_CALL_ARGS / TOOL_CALL_END /
// TOOL_CALL_RESULT / RUN_FINISHED / STATE_SNAPSHOT / MESSAGES_SNAPSHOT
// 等事件类型。
//
// 本文件提供：
//   - AGUIThreadState：稳定的 threadId / runId / messageId 生成器
//   - AGUIEventMapper：将 MockEvent / 内部事件转换为 AG-UI 标准格式
//   - NewAGUIMapper 工厂函数
//
// 触发方式：
//   - Header: X-Agent-Protocol: agui
//   - Query:  ?protocol=agui
//
// SPEC: /workspace/.trae/specs/multi-engine-chat-architecture/ Phase 4
//
//	/workspace/.trae/specs/agui-real-llm-path-completion/ Phase 1
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ─── AGUIThreadState ─────────────────────────────────────────
//
// AGUIThreadState 生成和管理 AG-UI 协议要求的三类稳定 ID：
//   - threadId：会话级，整个 session 内不变
//   - runId：单次 chat/confirm 调用级，每次 NewRun() 生成新 UUID
//   - messageId：消息级，格式 `msg_<runId>_<seq>`，seq 跨 run 全局递增
//
// 关键认知：
//   - threadId 来自 sessID（"thread_" + sessID 派生），保证 session 内外一致
//   - runId 用 google/uuid（项目内已有依赖），与 task_manager.go 用法一致
//   - seq 计数器加锁保护（streamChat 在多 round 循环中可能并发更新）
//   - timestamp 使用 ISO 8601 毫秒精度（前端 AG-UI parser 期望）
type AGUIThreadState struct {
	mu       sync.Mutex
	threadId string
	runId    string
	seq      int
}

// NewAGUIThreadState 从 sessID 派生稳定的 threadId。
//
// threadId 格式：`thread_<sessID>`
//
// 如果 sessID 为空，使用占位符 "default"（与 handleAgentChat 的 sessID fallback 一致）。
func NewAGUIThreadState(sessID string) *AGUIThreadState {
	if sessID == "" {
		sessID = "default"
	}
	return &AGUIThreadState{
		threadId: "thread_" + sessID,
	}
}

// ThreadId 返回当前 threadId（只读）。
func (s *AGUIThreadState) ThreadId() string {
	return s.threadId
}

// RunId 返回当前 runId（只读，可能为空直到 NewRun() 被调用）。
func (s *AGUIThreadState) RunId() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runId
}

// NewRun 生成新 runId。
//
// 同一 session 可调用多次：每次 chat / confirm 视为新 run。
// 使用 google/uuid.New().String()（与 internal/service/task_manager.go 风格一致）。
func (s *AGUIThreadState) NewRun() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runId = uuid.New().String()
}

// NextMessageID 返回下一个 messageId（`msg_<runId>_<seq>`）并递增 seq。
//
// 注意：seq 跨 run 共享，单调递增（符合 spec §"稳定 ID 生成"）。
// 调用方必须先调 NewRun()，否则 runId 为空，messageId 也会异常。
func (s *AGUIThreadState) NextMessageID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return fmt.Sprintf("msg_%s_%d", s.runId, s.seq)
}

// CurrentTimestamp 返回当前 ISO 8601 毫秒精度时间戳。
//
// 格式：`2026-06-07T12:34:56.789Z`
//
// 使用 time.Now().UTC() 避免时区歧义；毫秒精度匹配前端 AG-UI parser 期望。
func (s *AGUIThreadState) CurrentTimestamp() string {
	now := time.Now().UTC()
	return now.Format("2006-01-02T15:04:05.000Z")
}

// ─── AGUIEventMapper ─────────────────────────────────────────

// AGUIEventMapper 将内部事件格式转换为 AG-UI 标准协议事件。
//
// 使用方式：
//
//	state := NewAGUIThreadState(sessID)
//	state.NewRun()  // 在每个 chat/confirm 入口调用一次
//	mapper := NewAGUIMapper(w, flusher, state)
//	mapper.MapEvent(mockEvent, stepIdx, evIdx)
//
// 或者使用高级方法（推荐用于 streamChat 真实 LLM 路径）：
//
//	mapper.EmitTextMessageStart(messageID)
//	mapper.EmitTextMessageContent(messageID, "hello ")
//	mapper.EmitTextMessageContent(messageID, "world")
//	mapper.EmitTextMessageEnd(messageID)
type AGUIEventMapper struct {
	w     http.ResponseWriter
	f     http.Flusher
	state *AGUIThreadState
}

// NewAGUIMapper 创建一个新的 AG-UI 事件映射器。
//
// 参数：
//   - w: HTTP ResponseWriter（用于写入 SSE 数据）
//   - f: HTTP Flusher（用于立即刷新缓冲区）
//   - sess: 会话 ID（用作 threadId 派生源）
//
// 便利构造函数：内部自动调用 NewAGUIThreadState(sess)。
// 如果已有 AGUIThreadState 实例（共享 threadId），请使用 NewAGUIMapperWithState。
func NewAGUIMapper(w http.ResponseWriter, f http.Flusher, sess string) *AGUIEventMapper {
	return NewAGUIMapperWithState(w, f, NewAGUIThreadState(sess))
}

// NewAGUIMapperWithState 使用已存在的 AGUIThreadState 创建 mapper。
//
// 适用于需要在多个 mapper 之间共享 threadId / runId 的场景
// （如 handleAgentChat 入口先建 state，再分给多个 mapper）。
func NewAGUIMapperWithState(w http.ResponseWriter, f http.Flusher, state *AGUIThreadState) *AGUIEventMapper {
	return &AGUIEventMapper{w: w, f: f, state: state}
}

// State 返回底层 AGUIThreadState（用于生成 messageId）。
func (m *AGUIEventMapper) State() *AGUIThreadState {
	return m.state
}

// NewRun 在底层 state 上调用 NewRun()。便利方法。
func (m *AGUIEventMapper) NewRun() {
	m.state.NewRun()
}

// ─── 事件映射（MapEvent 兼容旧 API）─────────────────────────

// MapEvent 根据内部事件类型输出对应的 AG-UI 事件。
//
// 事件映射表：
//
//	| 内部事件类型          | AG-UI 事件类型          |
//	|---------------------|------------------------|
//	| stream_start         | RUN_STARTED            |
//	| text_delta           | TEXT_MESSAGE_CONTENT   |
//	| text_delta_templated | TEXT_MESSAGE_CONTENT   |
//	| tool_call            | TOOL_CALL_START + ARGS |
//	| tool_status (success)| TOOL_CALL_END          |
//	| tool_result          | TOOL_CALL_RESULT       |
//	| stream_end           | RUN_FINISHED           |
//
// 兼容性说明：此方法保留旧版 mapper 行为（无 threadId/runId/timestamp 注入），
// 供 MockEngine.Run 内部事件流兼容使用。新代码请直接调用 Emit* 高级方法。
func (m *AGUIEventMapper) MapEvent(ev MockEvent, _stepIdx, _evIdx int) {
	switch ev.Type {
	case "stream_start":
		m.sendAGUI("RUN_STARTED", map[string]interface{}{
			"runId":     m.state.runId,
			"threadId":  m.state.threadId,
			"timestamp": m.state.CurrentTimestamp(),
		})

	case "text_delta", "text_delta_templated":
		text, _ := ev.Data["text"].(string)
		if text == "" {
			return
		}
		// 兼容性：保持 messageId 格式为 `msg_<sessID>`
		// 新版高级方法（EmitTextMessageContent）使用 `msg_<runId>_<seq>`
		messageId, _ := ev.Data["messageId"].(string)
		if messageId == "" {
			// 从 sessID 派生（与旧行为一致）
			messageId = fmt.Sprintf("msg_%s", m.state.threadId)
		}
		m.sendAGUI("TEXT_MESSAGE_CONTENT", map[string]interface{}{
			"messageId": messageId,
			"delta":     text,
			"type":      "TEXT_MESSAGE_CONTENT",
		})

	case "tool_call":
		id, _ := ev.Data["id"].(string)
		name, _ := ev.Data["name"].(string)
		args, _ := ev.Data["args"].(string)

		if id == "" || name == "" {
			return // 必填字段缺失，跳过
		}

		// AG-UI 分两个事件推送：先 START 再 ARGS
		m.sendAGUI("TOOL_CALL_START", map[string]interface{}{
			"toolCallId":   id,
			"toolCallName": name,
			"type":         "TOOL_CALL_START",
		})

		m.EmitToolCallArgs(id, args) // 空 args 时跳过（按 spec 降级）

	case "tool_status":
		// success → TOOL_CALL_END
		if status, ok := ev.Data["status"].(string); ok && status == "success" {
			id, _ := ev.Data["id"].(string)
			if id != "" {
				m.sendAGUI("TOOL_CALL_END", map[string]interface{}{
					"toolCallId": id,
					"type":       "TOOL_CALL_END",
				})
			}
		}
		// failed / cancelled 状态也发送 END（带 error 字段）
		if status, ok := ev.Data["status"].(string); ok && (status == "failed" || status == "cancelled") {
			id, _ := ev.Data["id"].(string)
			if id != "" {
				m.sendAGUI("TOOL_CALL_END", map[string]interface{}{
					"toolCallId": id,
					"error":      status,
					"type":       "TOOL_CALL_END",
				})
			}
		}

	case "tool_result":
		id, _ := ev.Data["id"].(string)
		result, _ := ev.Data["result"].(string)
		if id != "" {
			m.sendAGUI("TOOL_CALL_RESULT", map[string]interface{}{
				"toolCallId": id,
				"content":    result,
				"type":       "TOOL_CALL_RESULT",
			})
		}

	case "stream_end":
		m.sendAGUI("RUN_FINISHED", map[string]interface{}{
			"runId":     m.state.runId,
			"threadId":  m.state.threadId,
			"timestamp": m.state.CurrentTimestamp(),
		})

	default:
		// 其他事件类型（stream_status, reasoning_delta 等）不映射到 AG-UI，
		// 静默跳过。未来可扩展支持更多类型。
	}
}

// ─── 高级事件方法（streamChat 真实 LLM 路径用）──────────────

// EmitTextMessageStart 推 TEXT_MESSAGE_START 事件。
//
// 必须在第一次 EmitTextMessageContent 之前调用。messageID 应由
// AGUIThreadState.NextMessageID() 生成（保证 runId 稳定 + seq 递增）。
//
// JSON data：{ messageId, role: "assistant", timestamp, threadId, runId, type }
func (m *AGUIEventMapper) EmitTextMessageStart(messageID string) {
	m.sendAGUI("TEXT_MESSAGE_START", map[string]interface{}{
		"messageId": messageID,
		"role":      "assistant",
		"threadId":  m.state.threadId,
		"runId":     m.state.runId,
		"timestamp": m.state.CurrentTimestamp(),
		"type":      "TEXT_MESSAGE_START",
	})
}

// EmitTextMessageContent 推 TEXT_MESSAGE_CONTENT 事件。
//
// 同一 message 多次调用应共用 messageID（与 EmitTextMessageStart 一致）。
//
// JSON data：{ messageId, delta, timestamp, threadId, runId, type }
func (m *AGUIEventMapper) EmitTextMessageContent(messageID, delta string) {
	if delta == "" {
		return // 空 delta 不推送（避免前端解析器误判）
	}
	m.sendAGUI("TEXT_MESSAGE_CONTENT", map[string]interface{}{
		"messageId": messageID,
		"delta":     delta,
		"threadId":  m.state.threadId,
		"runId":     m.state.runId,
		"timestamp": m.state.CurrentTimestamp(),
		"type":      "TEXT_MESSAGE_CONTENT",
	})
}

// EmitTextMessageEnd 推 TEXT_MESSAGE_END 事件。
//
// 标记一个完整消息的边界。必须在最后一次 EmitTextMessageContent 之后调用。
//
// JSON data：{ messageId, timestamp, threadId, runId, type }
func (m *AGUIEventMapper) EmitTextMessageEnd(messageID string) {
	m.sendAGUI("TEXT_MESSAGE_END", map[string]interface{}{
		"messageId": messageID,
		"threadId":  m.state.threadId,
		"runId":     m.state.runId,
		"timestamp": m.state.CurrentTimestamp(),
		"type":      "TEXT_MESSAGE_END",
	})
}

// EmitToolCallStart 推 TOOL_CALL_START 事件（单事件版本，不嵌在 MapEvent 里）。
//
// JSON data：{ toolCallId, toolCallName, timestamp, threadId, runId, type }
func (m *AGUIEventMapper) EmitToolCallStart(toolCallID, toolCallName string) {
	m.sendAGUI("TOOL_CALL_START", map[string]interface{}{
		"toolCallId":   toolCallID,
		"toolCallName": toolCallName,
		"threadId":     m.state.threadId,
		"runId":        m.state.runId,
		"timestamp":    m.state.CurrentTimestamp(),
		"type":         "TOOL_CALL_START",
	})
}

// EmitToolCallArgs 推 TOOL_CALL_ARGS 事件。
//
// 关键行为：空 delta 时**直接 return 不发送任何 SSE 事件**（spec §"字段缺失 graceful 降级"）。
// 这避免 TDesign 解析器收到空 delta 报错。
//
// 非空 delta 推：{ toolCallId, delta, timestamp, threadId, runId, type }
func (m *AGUIEventMapper) EmitToolCallArgs(toolCallID, delta string) {
	if delta == "" {
		return // 跳过空 args（spec 要求）
	}
	m.sendAGUI("TOOL_CALL_ARGS", map[string]interface{}{
		"toolCallId": toolCallID,
		"delta":      delta,
		"threadId":   m.state.threadId,
		"runId":      m.state.runId,
		"timestamp":  m.state.CurrentTimestamp(),
		"type":       "TOOL_CALL_ARGS",
	})
}

// EmitToolCallEnd 推 TOOL_CALL_END 事件。
//
// JSON data：{ toolCallId, timestamp, threadId, runId, type } (无 error)
// 或 { toolCallId, error, timestamp, threadId, runId, type } (有 error)
func (m *AGUIEventMapper) EmitToolCallEnd(toolCallID string) {
	m.sendAGUI("TOOL_CALL_END", map[string]interface{}{
		"toolCallId": toolCallID,
		"threadId":   m.state.threadId,
		"runId":      m.state.runId,
		"timestamp":  m.state.CurrentTimestamp(),
		"type":       "TOOL_CALL_END",
	})
}

// EmitToolCallResult 推 TOOL_CALL_RESULT 事件。
//
// JSON data：{ toolCallId, content, timestamp, threadId, runId, type }
func (m *AGUIEventMapper) EmitToolCallResult(toolCallID, content string) {
	m.sendAGUI("TOOL_CALL_RESULT", map[string]interface{}{
		"toolCallId": toolCallID,
		"content":    content,
		"threadId":   m.state.threadId,
		"runId":      m.state.runId,
		"timestamp":  m.state.CurrentTimestamp(),
		"type":       "TOOL_CALL_RESULT",
	})
}

// EmitRunStarted 推 RUN_STARTED 事件。
//
// 通常在 streamChat 入口调用一次（与 mapper.NewRun() 配合）。
func (m *AGUIEventMapper) EmitRunStarted() {
	m.sendAGUI("RUN_STARTED", map[string]interface{}{
		"threadId":  m.state.threadId,
		"runId":     m.state.runId,
		"timestamp": m.state.CurrentTimestamp(),
		"type":      "RUN_STARTED",
	})
}

// EmitRunFinished 推 RUN_FINISHED 事件。
func (m *AGUIEventMapper) EmitRunFinished() {
	m.sendAGUI("RUN_FINISHED", map[string]interface{}{
		"threadId":  m.state.threadId,
		"runId":     m.state.runId,
		"timestamp": m.state.CurrentTimestamp(),
		"type":      "RUN_FINISHED",
	})
}

// EmitStateSnapshot 推 STATE_SNAPSHOT 事件。
//
// 用于会话级共享状态同步（context usage / tool schema / etc）。
// state 是任意 JSON-serializable object。
//
// JSON data：{ state, timestamp, threadId, runId, type }
func (m *AGUIEventMapper) EmitStateSnapshot(state map[string]interface{}) {
	m.sendAGUI("STATE_SNAPSHOT", map[string]interface{}{
		"state":     state,
		"threadId":  m.state.threadId,
		"runId":     m.state.runId,
		"timestamp": m.state.CurrentTimestamp(),
		"type":      "STATE_SNAPSHOT",
	})
}

// EmitMessagesSnapshot 推 MESSAGES_SNAPSHOT 事件。
//
// 用于断点续传时的完整消息历史对齐。messages 是任意 JSON-serializable 数组
// （通常是 useAgent Message[] 的归一化形式）。
//
// JSON data：{ messages, timestamp, threadId, runId, type }
func (m *AGUIEventMapper) EmitMessagesSnapshot(messages interface{}) {
	m.sendAGUI("MESSAGES_SNAPSHOT", map[string]interface{}{
		"messages":  messages,
		"threadId":  m.state.threadId,
		"runId":     m.state.runId,
		"timestamp": m.state.CurrentTimestamp(),
		"type":      "MESSAGES_SNAPSHOT",
	})
}

// ─── 内部：SSE 写入 ──────────────────────────────────────────

// sendAGUI 发送一个 AG-UI 格式的 SSE 事件。
//
// 格式: event: <eventType>\ndata: <jsonPayload>\n\n
//
// 注入 threadId / runId / timestamp 到 payload 顶层（即使调用方没显式传入，
// 也保证所有事件都有这三个稳定字段，匹配 spec §"所有事件 JSON 顶层含 threadId/runId/timestamp"）。
func (m *AGUIEventMapper) sendAGUI(eventType string, payload map[string]interface{}) {
	// 防御性注入：即使 Emit* 方法已经注入过，这里也保证不缺
	if _, ok := payload["threadId"]; !ok {
		payload["threadId"] = m.state.threadId
	}
	if _, ok := payload["runId"]; !ok {
		payload["runId"] = m.state.runId
	}
	if _, ok := payload["timestamp"]; !ok {
		payload["timestamp"] = m.state.CurrentTimestamp()
	}
	// type 字段在所有 Emit* 方法已显式注入；这里仅做兜底
	if _, ok := payload["type"]; !ok {
		payload["type"] = eventType
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return // 序列化失败，静默跳过
	}
	// AG-UI SSE 格式：event 行 + data 行
	fmt.Fprintf(m.w, "event: %s\ndata: %s\n\n", eventType, raw)
	if m.f != nil {
		m.f.Flush()
	}
}

// randomHex 是 crypto/rand 短辅助：返回 n 字节随机 hex 字符串。
// 保留备用：当前使用 google/uuid 生成 runId。
// 若未来需要自定义 runId 格式，可改用此函数。
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
