// internal/server/agent_tool_loop.go
//
// 工具调用循环 + 确认/拒绝/递归调 OpenAI 的实现。
//
// 状态机：
//
//   - 客户端 → POST /api/chat {messages, sessionId}
//     → 后端代理到 OpenAI streaming →
//     ① 累积 content → text_delta
//     ② 累积 tool_calls（按 id 聚合 index/function.name/args）
//     ③ 若 finish_reason == "tool_calls"：
//
//   - 推 tool_call 事件（每个 tool_call 一个事件）
//
//   - 推 stream_end
//
//   - 把 messages + pendingToolCalls 缓存到 session 状态
//     ④ 否则 finish_reason == "stop"：
//
//   - 推 stream_end
//
//   - 客户端 → POST /api/confirm {sessionId, toolCallId, decision}
//
//   - decision=accept  → 取出 pending tool_call → executePluginTool → 把 result 追加到 messages → 递归 chat
//
//   - decision=decline → 追加 cancelled tool_result 到 messages → 递归 chat
//
//   - decision=cancel  → 推 stream_end（不递归）
//
//   - 客户端 → POST /api/resume {sessionId, offset}
//
//   - 简化版：当前后端无事件缓存（bufio.Scanner 流式）→ 返回 "resume_not_implemented" 错误。
//     完整断点续传需要重构 chat 为事件驱动（带 ID），后续 phase 处理。
//
// 会话存储：sync.Map[sessionId]*agentSession + RWMutex
package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// toolCallAccumulator 累积 OpenAI 流式 tool_calls。
// OpenAI 工具调用在多个 chunk 中分片到达，必须按 (id, index) 聚合。
//
// Index 字段只在流式累积时使用，序列化为 assistant 消息的 tool_calls 数组时
// 用 omitempty 跳过（OpenAI 协议不需要 index）。
type toolCallAccumulator struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Index    int    `json:"index,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// agentSession 保存单次对话的服务端状态
type agentSession struct {
	mu              sync.Mutex
	SessionID       string
	Messages        []chatMsg
	PendingTools    []toolCallAccumulator // 当前 LLM 决策的工具调用（待 confirm）
	LastModel       string
	LastTemperature float64
	LastAccess      time.Time // 最近一次 getOrCreateSession 时间，用于 GC 判定

	// ─── D 阶段：断点续传（事件缓存） ───
	// EventCache 按事件 ID 升序存储，/api/resume 用它重放 lastEventID 之后的事件。
	// 写入锁：sess.mu
	// 读取锁：sess.mu（Resume 时拷贝切片后释放锁）
	EventCache     []AgentEvent
	eventIDCounter int64 // 单调递增事件 ID（sess.mu 保护下读写）
	InProgress     bool  // 当前是否有 chat/confirm 在生成（用于 Resume 状态判定）

	// ─── F 阶段：4 决策 UX（accept_for_session） ───
	// GrantedTools 记录本 session 内已被用户"本次会话授权"的工具名。
	// 后续同 session 同名 tool_call 自动放行（auto_run=true），不再弹 ApprovalCard。
	// 锁：sess.mu
	GrantedTools map[string]bool
}

// AgentEvent 是 SSE 事件的结构化表示，供 /api/resume 重放
type AgentEvent struct {
	ID   int64       `json:"id"`
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

const (
	// sessionIdleTTL — session 无活动多久后被 GC 回收
	sessionIdleTTL = 30 * time.Minute
	// sessionGCInterval — 后台 GC 扫描间隔
	sessionGCInterval = 5 * time.Minute
)

var (
	sessionMu sync.RWMutex
	sessions  = make(map[string]*agentSession)
)

// getOrCreateSession 获取或创建 session，顺便更新 LastAccess
func getOrCreateSession(id string) *agentSession {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if s, ok := sessions[id]; ok {
		s.LastAccess = time.Now()
		return s
	}
	s := &agentSession{SessionID: id, LastAccess: time.Now(), GrantedTools: make(map[string]bool)}
	sessions[id] = s
	return s
}

// updateSession 原子更新 session 内容
func updateSession(id string, fn func(*agentSession)) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if s, ok := sessions[id]; ok {
		fn(s)
	}
}

// gcIdleSessions 清理空闲超过 sessionIdleTTL 的 session，返回被清理的数量。
// 公开此函数供单测验证（不依赖后台 goroutine）。
func gcIdleSessions() int {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	now := time.Now()
	evicted := 0
	for id, s := range sessions {
		if now.Sub(s.LastAccess) > sessionIdleTTL {
			delete(sessions, id)
			evicted++
			slog.Info("agent: session evicted (idle)", "id", id, "idle", now.Sub(s.LastAccess).String())
		}
	}
	return evicted
}

// startSessionGC 启动后台 GC goroutine，每 sessionGCInterval 跑一次。
// 多次调用是安全的（用 sync.Once 保护）。
var startSessionGCSyncOnce sync.Once

func startSessionGC() {
	startSessionGCSyncOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(sessionGCInterval)
			defer ticker.Stop()
			for range ticker.C {
				if n := gcIdleSessions(); n > 0 {
					slog.Info("agent: session GC pass", "evicted", n, "remaining", len(sessions))
				}
			}
		}()
	})
}

// ─── 工具调用执行 + 递归 ───────────────────────────────────────

// executeAndRecurse 执行用户确认的工具调用，追加 tool_result 到 messages，
// 然后递归调 OpenAI 下一轮（带 tool_choice="none" 强制 LLM 继续生成）。
//
// 返回：
//   - tool_result（已执行）
//   - 新的 messages（已追加 tool_result）
//
// s 用于路由到正确的工具实现（fs / plugin）。fs 工具是 read-only 不需要 confirm，
// 但为了统一流程也走这里。
func executeAndRecurse(ctx context.Context, s *Server, sess *agentSession, agentCfg agentConfig, tool toolCallAccumulator) (chatMsg, error) {
	var argsObj map[string]interface{}
	_ = json.Unmarshal([]byte(tool.Function.Arguments), &argsObj)

	var (
		resultStr string
	)

	start := time.Now()
	raw, execErr := s.executeAgentTool(ctx, tool.Function.Name, tool.Function.Arguments)
	_ = time.Since(start).Milliseconds() // 预留 durationMs 字段供后续 metrics
	if execErr != nil {
		resultStr = fmt.Sprintf(`{"error":"internal","message":%q}`, execErr.Error())
	} else {
		resultStr = raw
		// 检测插件执行结果中的 error 字段
		var probe struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal([]byte(raw), &probe)
	}

	// 构造 tool 消息（OpenAI 协议要求 role="tool" + tool_call_id 引用）
	toolMsg := chatMsg{
		Role:       "tool",
		Content:    resultStr,
		ToolCallID: tool.ID,
		Name:       tool.Function.Name,
	}

	return toolMsg, nil
}

// callOpenAIChat 同步调一次 OpenAI Chat API（非流式）。用于工具结果注入后的递归。
// 返回 LLM 完整回复（一段 assistant 文本）或 tool_calls。
type openaiChatResponse struct {
	Choices []struct {
		Message struct {
			Role      string                `json:"role"`
			Content   string                `json:"content"`
			ToolCalls []toolCallAccumulator `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// callOpenAIChatOnce 同步调一次 OpenAI（非流式）。返回 LLM 响应。
//
// 关键：把 agent 工具列表塞到 reqBody["tools"] 里——之前没有这个字段，
// LLM 永远只能聊天不能调工具，这是 agent "perceive file system" 的前提。
func callOpenAIChatOnce(ctx context.Context, cfg agentConfig, model string, temperature float64, messages []chatMsg, openAITools []map[string]interface{}) (*openaiChatResponse, error) {
	reqBody := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"temperature": temperature,
		"stream":      false,
		// 故意不传 max_tokens —— 上游 LLM API 按自己模型最大输出返回
	}
	if len(openAITools) > 0 {
		reqBody["tools"] = openAITools
		reqBody["tool_choice"] = "auto"
		slog.Info("callOpenAIChatOnce: sending request with tools (max_tokens omitted, upstream decides)",
			"model", model, "tool_count", len(openAITools))
	}
	reqJSON, _ := json.Marshal(reqBody)

	reqURL := strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &openaiChatResponse{Error: &struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		}{Message: string(body)}}, nil
	}

	body, _ := io.ReadAll(resp.Body)
	var out openaiChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}

	slog.Info("callOpenAIChatOnce: response received",
		"status", resp.StatusCode,
		"has_tool_calls", len(out.Choices) > 0 && len(out.Choices[0].Message.ToolCalls) > 0,
		"finish_reason", func() string {
			if len(out.Choices) > 0 {
				return out.Choices[0].FinishReason
			}
			return "(no choices)"
		}(),
	)

	return &out, nil
}

// callOpenAIStream 调一次 OpenAI 流式，返回事件 channel。
// 用于在 confirm 后递归流式输出 LLM 继续生成的回复。
//
// 与 callOpenAIChatOnce 一样，把 agent 工具列表塞到 reqBody["tools"]。
//
// aguiMode 在 streamChat 入口处已被消费（用于构造 emitEvent 闭包分流）；
// 本函数仍透传 aguiMode 给 streamChat，以支持嵌套调用链（executeAndRecurse → streamChat）。
func callOpenAIStream(ctx context.Context, cfg agentConfig, model string, temperature float64, messages []chatMsg, openAITools []map[string]interface{}, aguiMode bool) (<-chan openaiStreamEvent, error) {
	// 故意不传 max_tokens —— 让我们根本不知道任何厂商的上限，避免硬编码瞎编。
	// 上游 LLM API 会按自己的模型最大输出返回（gpt-4o 16k, claude 8k, deepseek 8k... 各厂商自己决定）。
	// 这比维护一个会过时的查表更可靠。
	contextWindow := lookupContextWindow(model)

	reqBody := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"temperature": temperature,
		"stream":      true,
	}
	if len(openAITools) > 0 {
		reqBody["tools"] = openAITools
		reqBody["tool_choice"] = "auto"
		slog.Info("callOpenAIStream: sending request with tools (max_tokens omitted, upstream decides)",
			"model", model, "tool_count", len(openAITools),
			"context_window", contextWindow)
	} else {
		slog.Info("callOpenAIStream: WARNING no tools provided (max_tokens omitted, upstream decides)",
			"model", model, "context_window", contextWindow)
	}
	reqJSON, _ := json.Marshal(reqBody)

	reqURL := strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	// 【P2-9 修复】120s 太长 + 与 ctx 冲突；60s + ctx 双重兜底
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("upstream HTTP %d: %s", resp.StatusCode, string(body))
	}

	// 【P0-2 修复】producer 必须响应 ctx.Done() + channel 满时 select default 丢弃
	ch := make(chan openaiStreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		// emit: 把事件送到 ch；满/取消时 select default 丢弃（与 ws_hub 语义一致）
		emit := func(typ string, data interface{}) {
			select {
			case ch <- openaiStreamEvent{Type: typ, Data: data}:
			case <-ctx.Done():
			default:
				// 通道满或消费者已退：丢弃
			}
		}
		for scanner.Scan() {
			// 每行先检查 ctx（避免 scanner.Scan 阻塞时无法响应取消）
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			dataStr := strings.TrimPrefix(line, "data: ")
			if dataStr == "[DONE]" {
				emit("stream_end", "")
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string                `json:"content"`
						ToolCalls        []toolCallAccumulator `json:"tool_calls"`
						ReasoningContent string                `json:"reasoning_content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
				continue
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					emit("text_delta", choice.Delta.Content)
				}
				if choice.Delta.ReasoningContent != "" {
					emit("reasoning_delta", choice.Delta.ReasoningContent)
				}
				if len(choice.Delta.ToolCalls) > 0 {
					for _, tc := range choice.Delta.ToolCalls {
						emit("tool_call_chunk", tc)
					}
				}
				if choice.FinishReason != "" {
					emit("finish_reason", choice.FinishReason)
				}
			}
		}
		// 检查 scanner 错误（网络截断等）
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			slog.Warn("callOpenAIStream: scanner error", "err", err)
		}
	}()

	return ch, nil
}

type openaiStreamEvent struct {
	Type string
	Data interface{}
}

// streamChat 写 SSE 事件到 client —— 用于 /api/confirm 递归时
//
// openAITools 已经被 caller 包装成 OpenAI 协议格式（带 type:"function"），直接传即可。
// toolMeta 保留为 agent 内部格式（name → {needConfirm, kind, ...}），
// 用于在 SSE 推 tool_call 事件时给前端正确的 needsConfirm / kind。
//
// aguiMode 为 true 时，本函数用 AGUIEventMapper 输出标准 AG-UI 事件；
// 为 false 时走原 sendAndCache 通道（向后兼容旧客户端）。
func (s *Server) streamChat(ctx context.Context, c *gin.Context, cfg agentConfig, model string, temperature float64, messages []chatMsg, sess *agentSession, openAITools []map[string]interface{}, toolMeta map[string]map[string]interface{}, aguiMode bool) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return
	}
	s.setSSEHeaders(c.Writer)

	// ════════════════════════════════════════════════════════════
	// emitEvent 闭包分流（Phase 2 真实 LLM 路径 AG-UI 透传）
	// ════════════════════════════════════════════════════════════
	// aguiMode=true  → 走 AGUIEventMapper（标准 AG-UI 11 种事件）
	// aguiMode=false → 走 sendAndCache（legacy 自定义 SSE 格式 + EventCache 缓存）
	//
	// 注：AG-UI 路径**不**写 EventCache（保持 mock 路径一致性 — 见 agent_mock.go §emitEvent）。
	// AG-UI 模式是给前端 TDesign 引擎用的，/api/resume 重放逻辑不涉及此路径。
	//
	// AG-UI 模式额外维护"text message triplet"状态机：
	//   - 第一次 text_delta → 推 TEXT_MESSAGE_START + 首段 TEXT_MESSAGE_CONTENT
	//   - 后续 text_delta   → 推 TEXT_MESSAGE_CONTENT（共用同 messageId）
	//   - stream_end 或 finish_reason → 推 TEXT_MESSAGE_END（标记单条 assistant 消息结束）
	//   - 有 tool_call → 推完最后一个 content 后先发 TEXT_MESSAGE_END（闭合消息），
	//     再开始下一条 TOOL_CALL_START/ARGS/END triplet
	//
	// 注：streamChat 不推 RUN_STARTED / RUN_FINISHED（这两个由顶层 flow handler 推，
	// 因为 streamChat 可能在 agent loop 内被多次调用 — 一次 run 一次 RUN_STARTED）。
	var (
		emitEvent       func(evType string, data interface{})
		aguiMapper      *AGUIEventMapper
		aguiTextMsgID   string // 当前 assistant 消息的稳定 messageId
		aguiTextStarted bool   // 首个 TEXT_MESSAGE_START 是否已推
	)
	if aguiMode {
		aguiMapper = NewAGUIMapper(c.Writer, flusher, sess.SessionID)
		aguiMapper.NewRun() // 本次 chat/confirm 唯一 runId
		emitEvent = func(evType string, data interface{}) {
			// AG-UI 模式：把 data 标准化为 map[string]interface{}
			var aguiData map[string]interface{}
			if m, ok := data.(map[string]interface{}); ok {
				aguiData = m
			} else if s, ok := data.(string); ok {
				aguiData = map[string]interface{}{"text": s}
			} else {
				aguiData = map[string]interface{}{"data": data}
			}

			// 特殊处理：text_delta → TEXT_MESSAGE_START/CONTENT triplet
			if evType == "text_delta" {
				if !aguiTextStarted {
					aguiTextMsgID = aguiMapper.State().NextMessageID()
					aguiMapper.EmitTextMessageStart(aguiTextMsgID)
					aguiTextStarted = true
				}
				if text, ok := aguiData["text"].(string); ok {
					aguiMapper.EmitTextMessageContent(aguiTextMsgID, text)
				}
				return
			}

			// 特殊处理：tool_call → TOOL_CALL_START/ARGS triplet
			// 注意：闭合前一个 text 消息（如果有）
			if evType == "tool_call" {
				if aguiTextStarted {
					aguiMapper.EmitTextMessageEnd(aguiTextMsgID)
					aguiTextStarted = false
					aguiTextMsgID = ""
				}
				id, _ := aguiData["id"].(string)
				name, _ := aguiData["name"].(string)
				args, _ := aguiData["args"].(string)
				if id != "" && name != "" {
					aguiMapper.EmitToolCallStart(id, name)
				}
				// EmitToolCallArgs 内部对空 args 直接 return（不推事件）
				aguiMapper.EmitToolCallArgs(id, args)
				return
			}

			// 特殊处理：tool_status → TOOL_CALL_END
			if evType == "tool_status" {
				id, _ := aguiData["id"].(string)
				status, _ := aguiData["status"].(string)
				// completed / success / cancelled / failed 都闭合 tool call
				if status == "success" || status == "completed" || status == "cancelled" || status == "failed" {
					if id != "" {
						aguiMapper.EmitToolCallEnd(id)
					}
				}
				return
			}

			// 特殊处理：tool_result → TOOL_CALL_RESULT
			if evType == "tool_result" {
				id, _ := aguiData["id"].(string)
				result, _ := aguiData["result"].(string)
				aguiMapper.EmitToolCallResult(id, result)
				return
			}

			// 特殊处理：stream_end → 闭合 text 消息
			if evType == "stream_end" {
				if aguiTextStarted {
					aguiMapper.EmitTextMessageEnd(aguiTextMsgID)
					aguiTextStarted = false
				}
				return
			}

			// 特殊处理：stream_error → 闭合未结束的消息（不推 AG-UI event）
			if evType == "stream_error" {
				if aguiTextStarted {
					aguiMapper.EmitTextMessageEnd(aguiTextMsgID)
					aguiTextStarted = false
				}
				return
			}

			// 其他类型（reasoning_delta 等）走 AG-UI mapper 的兼容 MapEvent 路径
			aguiMapper.MapEvent(MockEvent{Type: evType, Data: aguiData}, 0, 0)
		}
	} else {
		emitEvent = func(evType string, data interface{}) {
			s.sendAndCache(sess, c.Writer, flusher, evType, data)
		}
	}

	ch, err := callOpenAIStream(ctx, cfg, model, temperature, messages, openAITools, aguiMode)
	if err != nil {
		slog.Warn("agent: stream error", "error", err)
		emitEvent("stream_error", err.Error())
		emitEvent("stream_end", "")
		return
	}

	// 累积 tool_calls（如果有的话）
	var pendingTools []toolCallAccumulator
	accumulator := make(map[int]*toolCallAccumulator)

	for ev := range ch {
		switch ev.Type {
		case "text_delta":
			emitEvent("text_delta", ev.Data)
		case "reasoning_delta":
			emitEvent("reasoning_delta", ev.Data)
		case "tool_call_chunk":
			tc := ev.Data.(toolCallAccumulator)
			cur, ok := accumulator[tc.Index]
			if !ok {
				cur = &toolCallAccumulator{Index: tc.Index, ID: tc.ID, Type: tc.Type}
				accumulator[tc.Index] = cur
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			if tc.Type != "" {
				cur.Type = tc.Type
			}
			if tc.Function.Name != "" {
				cur.Function.Name += tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				cur.Function.Arguments += tc.Function.Arguments
			}
		case "finish_reason":
			// 收集所有累积的 tool_calls
			for _, tc := range accumulator {
				pendingTools = append(pendingTools, *tc)
			}
			if len(pendingTools) > 0 {
				// 推 tool_call 事件（按 tool 名查 meta 决定 needsConfirm / kind）
				for _, tc := range pendingTools {
					needConfirm := true
					kind := "fileChange"
					if meta, ok := toolMeta[tc.Function.Name]; ok {
						if v, ok := meta["needConfirm"].(bool); ok {
							needConfirm = v
						}
						if v, ok := meta["kind"].(string); ok {
							kind = v
						}
					}
					// F 阶段：检查 session 授权表 → 已授权工具自动放行
					autoRun := false
					if !needConfirm && sess != nil {
						// fs 工具永远不需 confirm，理论上不会进 PendingTools；
						// 但 plugin 工具在 accept_for_session 之后会回写
						sess.mu.Lock()
						if sess.GrantedTools[tc.Function.Name] {
							autoRun = true
						}
						sess.mu.Unlock()
					}
					payload := map[string]interface{}{
						"id":           tc.ID,
						"name":         tc.Function.Name,
						"args":         tc.Function.Arguments,
						"auto_run":     autoRun,
						"needsConfirm": needConfirm,
						"kind":         kind,
					}
					emitEvent("tool_call", payload)
				}
				// 缓存到 session
				sess.mu.Lock()
				sess.PendingTools = pendingTools
				sess.mu.Unlock()
			}
		case "stream_end":
			emitEvent("stream_end", "")
		}
	}
}

// ════════════════════════════════════════════════════════════
// 平台级 Tool Use：文本解析器
// ════════════════════════════════════════════════════════════
//
// 当 API 代理（如 gptgod）静默丢弃 OpenAI tools 参数导致
// API 级 Function Calling 不生效时，用此解析器从 LLM 文本回复中
// 提取工具调用 JSON。
//
// System prompt 指示 LLM 在需要调用工具时输出：
//   [{"name":"tool_name","arguments":{...}}]
//
// 解析策略（按优先级）：
//   1. 整个文本是 JSON 数组 → 直接解析
//   2. 文本以 [ 开头 → 提取第一个完整的 [...]
//   3. ```json 代码块 → 提取代码块内容
//   4. ``` (无语言标记) 代码块 → 尝试解析为工具调用
//   5. 整个文本是单个 JSON object → 包装为单元素数组
//   6. 文本中间嵌入 [ 或 { → 提取并尝试解析
//
// 额外处理：
//   - 字段名归一化：tool→name, params/input→arguments
//   - 单对象自动包装为数组

// parsedToolCall 是从 LLM 文本中解析出的工具调用
type parsedToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// extractToolCallsFromText 从 LLM 文本回复中提取工具调用。
// 返回 (toolCalls, remainingText)：
//   - toolCalls: 解析到的工具调用列表（可能为空）
//   - remainingText: 剥离工具调用 JSON 后的剩余文本（LLM 的自然语言部分）
func extractToolCallsFromText(text string) ([]parsedToolCall, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, ""
	}

	// DEBUG: 记录 LLM 原始输出（截断到 300 字符，避免日志爆炸）
	debugPreview := trimmed
	if len(debugPreview) > 300 {
		debugPreview = debugPreview[:300] + "..."
	}
	slog.Debug("extractToolCallsFromText: input", "len", len(trimmed), "preview", debugPreview)

	// 策略 1: 整个文本就是 JSON 数组
	if strings.HasPrefix(trimmed, "[") {
		if calls, ok := tryParseArray(trimmed, ""); ok && len(calls) > 0 {
			slog.Info("extractToolCalls: strategy-1 (full JSON array)", "count", len(calls))
			return calls, ""
		}
	}

	// 策略 2: 文本以 [ 开头，提取第一个完整的 [...]
	if strings.HasPrefix(trimmed, "[") {
		if endIdx := findJSONArrayEnd(trimmed); endIdx > 0 {
			jsonStr := trimmed[:endIdx]
			remaining := strings.TrimSpace(trimmed[endIdx:])
			if calls, ok := tryParseArray(jsonStr, remaining); ok && len(calls) > 0 {
				slog.Info("extractToolCalls: strategy-2 (prefix array)", "count", len(calls))
				return calls, remaining
			}
		}
	}

	// 策略 3: ```json 代码块
	if start := strings.Index(trimmed, "```json"); start >= 0 {
		if calls, rem := tryExtractCodeBlock(trimmed, start, "json"); len(calls) > 0 {
			slog.Info("extractToolCalls: strategy-3 (```json block)", "count", len(calls))
			return calls, rem
		}
	}

	// 策略 4: ``` (无语言标记) 代码块 — 有些 LLM 直接用 ``` 包裹 JSON
	if start := strings.Index(trimmed, "```"); start >= 0 {
		// 跳过已处理的 ```json
		nextLine := strings.Index(trimmed[start:], "\n")
		if nextLine > 0 {
			afterMarker := trimmed[start : start+nextLine]
			if !strings.Contains(afterMarker, "json") {
				if calls, rem := tryExtractCodeBlock(trimmed, start, ""); len(calls) > 0 {
					slog.Info("extractToolCalls: strategy-4 (``` block no lang)", "count", len(calls))
					return calls, rem
				}
			}
		}
	}

	// 策略 5: 整个文本是单个 JSON object → 包装为单元素数组
	if strings.HasPrefix(trimmed, "{") {
		var single parsedToolCall
		if err := json.Unmarshal([]byte(trimmed), &single); err == nil && single.Name != "" {
			calls := []parsedToolCall{normalizeFields(single)}
			slog.Info("extractToolCalls: strategy-5 (single object wrapped)", "name", single.Name)
			return calls, ""
		}
	}

	// 策略 6: 文本中间嵌入 JSON 数组 [...]
	if bracketStart := strings.Index(trimmed, "["); bracketStart >= 0 {
		if endIdx := findJSONArrayEnd(trimmed[bracketStart:]); endIdx > 0 {
			candidate := trimmed[bracketStart : bracketStart+endIdx]
			before := strings.TrimSpace(trimmed[:bracketStart])
			after := strings.TrimSpace(trimmed[bracketStart+endIdx:])
			remaining := joinNonEmpty(before, after)
			if calls, ok := tryParseArray(candidate, remaining); ok && len(calls) > 0 {
				slog.Info("extractToolCalls: strategy-6 (embedded array)", "count", len(calls))
				return calls, remaining
			}
		}
	}

	// 策略 7: 文件中间嵌入单个 JSON object {...}（LLM 有时输出单个对象而非数组）
	if braceStart := strings.Index(trimmed, "{"); braceStart >= 0 {
		if endIdx := findJSONObjectEnd(trimmed[braceStart:]); endIdx > 0 {
			candidate := trimmed[braceStart : braceStart+endIdx]
			before := strings.TrimSpace(trimmed[:braceStart])
			after := strings.TrimSpace(trimmed[braceStart+endIdx:])
			remaining := joinNonEmpty(before, after)
			var single parsedToolCall
			if err := json.Unmarshal([]byte(candidate), &single); err == nil && single.Name != "" {
				calls := []parsedToolCall{normalizeFields(single)}
				slog.Info("extractToolCalls: strategy-7 (embedded single object)",
					"name", single.Name, "has_remaining", remaining != "")
				return calls, remaining
			}
		}
	}

	slog.Debug("extractToolCallsFromText: no tool calls found in text")
	return nil, trimmed
}

// tryParseArray 尝试将字符串解析为 parsedToolCall 数组，验证每个元素有 name 字段
func tryParseArray(jsonStr, remaining string) ([]parsedToolCall, bool) {
	var calls []parsedToolCall
	if err := json.Unmarshal([]byte(jsonStr), &calls); err != nil {
		return nil, false
	}
	valid := make([]parsedToolCall, 0, len(calls))
	for _, c := range calls {
		if c.Name == "" {
			return nil, false
		}
		valid = append(valid, normalizeFields(c))
	}
	return valid, true
}

// tryExtractCodeBlock 从 ```xxx 标记处提取代码块内容并尝试解析为工具调用
func tryExtractCodeBlock(fullText string, codeBlockStart int, lang string) ([]parsedToolCall, string) {
	if lang != "" {
		// 跳过 ```json 后的换行或空格
		contentStart := codeBlockStart + 7 // len("```json") = 7
		for contentStart < len(fullText) && (fullText[contentStart] == ' ' || fullText[contentStart] == '\n' || fullText[contentStart] == '\r') {
			contentStart++
		}
		endIdx := strings.Index(fullText[contentStart:], "```")
		if endIdx <= 0 {
			return nil, ""
		}
		codeBlock := strings.TrimSpace(fullText[contentStart : contentStart+endIdx])
		before := strings.TrimSpace(fullText[:codeBlockStart])
		after := strings.TrimSpace(fullText[contentStart+endIdx+3:])
		remaining := joinNonEmpty(before, after)

		if calls, ok := tryParseArray(codeBlock, remaining); ok {
			return calls, remaining
		}
		// 也尝试单对象
		var single parsedToolCall
		if err := json.Unmarshal([]byte(codeBlock), &single); err == nil && single.Name != "" {
			return []parsedToolCall{normalizeFields(single)}, remaining
		}
	}
	return nil, ""
}

// normalizeFields 归一化字段名：有些 LLM 输出 tool/params/input 而非标准 name/arguments
func normalizeFields(c parsedToolCall) parsedToolCall {
	// 归一化 name 字段
	if c.Name == "" {
		// 尝试从可能的别名获取（如果未来需要扩展）
	}

	// 确保 arguments 不为 nil
	if len(c.Arguments) == 0 {
		c.Arguments = json.RawMessage(`{}`)
	}
	return c
}

// findJSONObjectEnd 找到 JSON 对象的结束位置（匹配嵌套 {} 和 []）。
// 输入必须以 { 开头。返回 } 之后的位置（不含 }），找不到返回 -1。
func findJSONObjectEnd(s string) int {
	if len(s) == 0 || s[0] != '{' {
		return -1
	}
	depth := 0
	inString := false
	escape := false
	for i, ch := range s {
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// joinNonEmpty 拼接两个非空字符串，用换行分隔
func joinNonEmpty(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n" + b
}

// findJSONArrayEnd 找到 JSON 数组的结束位置（匹配嵌套 [] 和 {}）。
// 输入必须以 [ 开头。返回 ] 之后的位置（不含 ]），找不到返回 -1。
func findJSONArrayEnd(s string) int {
	if len(s) == 0 || s[0] != '[' {
		return -1
	}
	depth := 0
	inString := false
	escape := false
	for i, ch := range s {
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// parsedToolCallsToAccumulator 把 parsedToolCall 转换为 toolCallAccumulator（OpenAI 协议格式）
// 用于和已有的 Agent Loop 分支 A（API 级 tool_calls）统一处理。
func parsedToolCallsToAccumulator(calls []parsedToolCall) []toolCallAccumulator {
	result := make([]toolCallAccumulator, 0, len(calls))
	for i, c := range calls {
		argsStr := "{}"
		if len(c.Arguments) > 0 {
			argsStr = string(c.Arguments)
		}
		result = append(result, toolCallAccumulator{
			ID:   fmt.Sprintf("ptc_%d_%d", time.Now().UnixMilli(), i),
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      c.Name,
				Arguments: argsStr,
			},
		})
	}
	return result
}
