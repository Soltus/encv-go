package server

// agent_confirm.go — 拆分自 agent_api.go

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type confirmRequest struct {
	SessionId  string `json:"sessionId"`
	ToolCallId string `json:"toolCallId"`
	Decision   string `json:"decision"` // accept | decline | cancel | accept_for_session
	DeviceId   string `json:"deviceId"` // 设备指纹（用于 API Key 解密 + 系统提示词）
}

func (s *Server) handleAgentConfirm(c *gin.Context) {
	// ① 解析请求体
	var body confirmRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}

	// ② 决策白名单校验
	switch body.Decision {
	case "accept", "decline", "cancel", "accept_for_session":
		// pass
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_decision", "message": "decision 必须是 accept / decline / cancel / accept_for_session"})
		return
	}

	// ════════════════════════════════════════════════════════════
	// AG-UI 协议模式检测（Phase 2 真实 LLM 路径透传）
	// ════════════════════════════════════════════════════════════
	// 当请求携带 X-Agent-Protocol: agui header 或 ?protocol=agui query 时，
	// 递归 streamChat 输出标准 AG-UI 格式。
	aguiMode := c.GetHeader("X-Agent-Protocol") == "agui" || c.Request.URL.Query().Get("protocol") == "agui"

	// ③ session 必须存在
	sessID := body.SessionId
	if sessID == "" {
		sessID = "default"
	}
	sessionMu.RLock()
	sess, ok := sessions[sessID]
	sessionMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session_not_found", "message": "未找到会话，请重新发起对话"})
		return
	}

	// ④ Flusher 检测
	flusher, okFlusher := c.Writer.(http.Flusher)
	if !okFlusher {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sse_not_supported"})
		return
	}
	s.setSSEHeaders(c.Writer)

	// ⑤ cancel：立即终止，不执行、不递归
	if body.Decision == "cancel" {
		s.sendAndCache(sess, c.Writer, flusher, "stream_end", "")
		return
	}

	// ⑥ accept / decline：必须找到对应的 tool_call
	sess.mu.Lock()
	var tool *toolCallAccumulator
	for i := range sess.PendingTools {
		if sess.PendingTools[i].ID == body.ToolCallId {
			tool = &sess.PendingTools[i]
			break
		}
	}
	if tool == nil {
		sess.mu.Unlock()
		slog.Warn("agent: confirm tool not found", "session", sessID, "toolCallId", body.ToolCallId)
		s.sendAndCache(sess, c.Writer, flusher, "stream_error", "tool_call_not_found: "+body.ToolCallId)
		s.sendAndCache(sess, c.Writer, flusher, "stream_end", "")
		return
	}

	// ⑦ 构造 assistant 消息（带 tool_calls 数组）
	//     OpenAI 协议要求 assistant 消息带 tool_calls 数组，否则 LLM 不知道上轮调用了哪些 tool。
	//     这里把所有 pending tool_calls 都放进去（即使有些用户没确认，也保留引用给 LLM），
	//     对应的 tool 消息只针对用户 confirm 的那一个。
	//     注：toolCallAccumulator.Index 在序列化时已用 omitempty 跳过（OpenAI 协议不需要）。
	allToolCalls := make([]toolCallAccumulator, len(sess.PendingTools))
	copy(allToolCalls, sess.PendingTools)
	assistantMsg := chatMsg{
		Role:      "assistant",
		Content:   "", // assistant 决策后 content 通常为空（被 tool_calls 替代）
		ToolCalls: allToolCalls,
	}
	// 拷贝 tool 引用到本地（在 lock 内完成，避免 unlock 后 dangling）
	toolCopy := *tool
	sess.mu.Unlock()

	// ⑧ accept / accept_for_session → 真实执行；decline → 注入 cancelled 假结果
	var toolMsg chatMsg
	if body.Decision == "accept" || body.Decision == "accept_for_session" {
		// 读取 agent 配置（用 deviceId 派生 API Key 解密 + 系统提示词）
		cfg := s.readAgentConfig(body.DeviceId)
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.openai.com"
		}
		toolMsg, _ = executeAndRecurse(c.Request.Context(), s, sess, cfg, toolCopy)
		// 推 tool_status: completed 给前端
		statusMsg := "completed"
		if body.Decision == "accept_for_session" {
			// 同时记录到 session 授权表
			sess.mu.Lock()
			sess.GrantedTools[toolCopy.Function.Name] = true
			sess.mu.Unlock()
			statusMsg = "completed_granted" // 前端可显示不同文案
		}
		s.sendAndCache(sess, c.Writer, flusher, "tool_status", map[string]interface{}{
			"id":     toolCopy.ID,
			"status": statusMsg,
			"result": toolMsg.Content,
		})
	} else {
		// decline: 构造 cancelled 假结果
		toolMsg = chatMsg{
			Role:       "tool",
			Content:    `{"cancelled": true, "reason": "user_declined"}`,
			ToolCallID: toolCopy.ID,
			Name:       toolCopy.Function.Name,
		}
		s.sendAndCache(sess, c.Writer, flusher, "tool_status", map[string]interface{}{
			"id":     toolCopy.ID,
			"status": "cancelled",
			"result": "user_declined",
		})
	}

	// ⑨ 把 assistant + tool 消息追加到 session.messages
	sess.mu.Lock()
	sess.Messages = append(sess.Messages, assistantMsg, toolMsg)
	// 清空 pending tools
	sess.PendingTools = nil
	// 标记递归 chat 进行中（让 resume 能感知）
	sess.InProgress = true
	sess.mu.Unlock()

	// ⑨½ 递归 chat 结束/panic 时清 InProgress
	defer func() {
		sess.mu.Lock()
		sess.InProgress = false
		sess.mu.Unlock()
	}()

	// ⑩ 递归下一轮 chat（流式）
	cfg := s.readAgentConfig(body.DeviceId)
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	// 注入 system prompt（与 handleAgentChat 一致，空配置时用默认）
	finalMessages := sess.Messages
	systemPrompt := cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultAgentSystemPrompt
	}
	finalMessages = make([]chatMsg, 0, len(sess.Messages)+1)
	finalMessages = append(finalMessages, chatMsg{Role: "system", Content: systemPrompt})
	finalMessages = append(finalMessages, sess.Messages...)
	// 递归时也要把工具列表塞回去——LLM 可能在后续轮次继续调工具
	agentTools := s.ListAgentTools()
	openAITools := agentToolsToOpenAITools(agentTools)
	toolMeta := make(map[string]map[string]interface{}, len(agentTools))
	for _, t := range agentTools {
		if n, ok := t["name"].(string); ok {
			toolMeta[n] = t
		}
	}
	s.streamChat(c.Request.Context(), c, cfg, sess.LastModel, sess.LastTemperature, finalMessages, sess, openAITools, toolMeta, aguiMode)
}

func (s *Server) handleAgentResume(c *gin.Context) {
	// ① 解析请求体（lastEventID 从 body 或 SSE standard header Last-Event-ID 取）
	var body struct {
		SessionId   string `json:"sessionId"`
		LastEventID int64  `json:"lastEventId"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.LastEventID == 0 {
		// SSE 协议标准 header：Last-Event-ID（前端用 EventSource 自带支持）
		if hdr := c.GetHeader("Last-Event-ID"); hdr != "" {
			if n, err := strconv.ParseInt(hdr, 10, 64); err == nil {
				body.LastEventID = n
			}
		}
	}

	// ════════════════════════════════════════════════════════════
	// AG-UI 协议模式检测（Phase 2 真实 LLM 路径透传）
	// ════════════════════════════════════════════════════════════
	// resume 路径目前只重放 EventCache，不直接调 streamChat。
	// 但保留 aguiMode 检测以保持接口一致，并供未来重放逻辑参考。
	_ = c.GetHeader("X-Agent-Protocol") == "agui" || c.Request.URL.Query().Get("protocol") == "agui"
	// 注：resume 不走 streamChat，故暂不实际透传 aguiMode。

	// ② session 必须存在
	sessID := body.SessionId
	if sessID == "" {
		sessID = "default"
	}
	sessionMu.RLock()
	sess, ok := sessions[sessID]
	sessionMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session_not_found", "message": "未找到会话"})
		return
	}

	// ③ Flusher 检测
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sse_not_supported"})
		return
	}
	s.setSSEHeaders(c.Writer)

	// ④ 取 EventCache 副本（释放锁后遍历，避免长时间持锁）
	sess.mu.Lock()
	eventCache := make([]AgentEvent, len(sess.EventCache))
	copy(eventCache, sess.EventCache)
	inProgress := sess.InProgress
	sess.mu.Unlock()

	// ⑤ 找到 lastEventID 之后第一个事件的索引
	startIdx := 0
	for i, e := range eventCache {
		if e.ID > body.LastEventID {
			startIdx = i
			break
		}
		// 如果遍历完没找到（即 lastEventID >= 最后一个事件 ID），保持 startIdx = 0
		// 但这会导致全量重放——更合理的处理：如果 lastEventID == 最后事件 ID，startIdx = len
		if i == len(eventCache)-1 && e.ID <= body.LastEventID {
			startIdx = len(eventCache)
		}
	}

	// ⑥ 如果 lastEventID 已经到最后（startIdx == len），且 in_progress=false（已结束）→ 推 stream_end
	if startIdx >= len(eventCache) {
		if !inProgress {
			s.sendSSEEventSafe(c.Writer, flusher, "stream_end", "")
			return
		}
		// 仍 inProgress 但没有新事件 → 推 status: synced
		s.sendSSEEventSafe(c.Writer, flusher, "stream_status", map[string]interface{}{
			"status":     "synced",
			"inProgress": true,
			"maxEventId": maxEventID(eventCache),
		})
		return
	}

	// ⑦ 重放 startIdx 之后的所有事件
	slog.Info("agent: resume replay events", "session", sessID, "lastEventID", body.LastEventID, "count", len(eventCache)-startIdx)
	for _, e := range eventCache[startIdx:] {
		// SSE 标准：在 data 之前带 `id: <eventID>` 字段
		raw, _ := json.Marshal(e.Data)
		fmt.Fprintf(c.Writer, "id: %d\ndata: {\"type\": \"%s\", \"data\": %s}\n\n", e.ID, e.Type, raw)
	}
	flusher.Flush()

	// ⑧ 如果仍 inProgress，前端稍后再次 resume；如果已完成 → 推 stream_end
	if inProgress {
		s.sendSSEEventSafe(c.Writer, flusher, "stream_status", map[string]interface{}{
			"status":     "more_pending",
			"inProgress": true,
			"maxEventId": maxEventID(eventCache),
		})
	} else {
		// 检查最后一个事件是否是 stream_end
		last := eventCache[len(eventCache)-1]
		if last.Type != "stream_end" {
			s.sendSSEEventSafe(c.Writer, flusher, "stream_end", "")
		}
	}
}
