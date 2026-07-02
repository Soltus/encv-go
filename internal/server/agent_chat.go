package server

// agent_chat.go — 拆分自 agent_api.go

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type chatRequest struct {
	SessionId   string    `json:"sessionId"`
	Model       string    `json:"model"`
	Temperature float64   `json:"temperature"`
	Messages    []chatMsg `json:"messages"`
	DeviceId    string    `json:"deviceId"`
	Mode        string    `json:"mode,omitempty"`     // start / steer / queue / mock_resume
	Scenario    string    `json:"scenario,omitempty"` // mock_resume 时携带的剧本 ID
}

func (s *Server) handleAgentChat(c *gin.Context) {
	// ① 解析请求体（必须在 WriteHeader 之前）
	var body chatRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}

	// ①½ 启动后台 session GC（幂等）
	startSessionGC()

	// A2UI 协议版本识别（预留，本轮不处理）
	if a2v := c.GetHeader("X-A2UI-Version"); a2v != "" {
		slog.Info("agent: A2UI protocol requested", "version", a2v, "session", body.SessionId)
		// 未来：根据 version 选择不同的 Surface 渲染策略
	}

	// ② 读取 agent 配置（API Key / Base URL / System Prompt）
	cfg := s.readAgentConfig(body.DeviceId)
	if cfg.APIKey == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no_api_key", "message": "未配置 API Key，请在 AI 设置中填写"})
		return
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	model := body.Model
	if model == "" {
		// 跟随用户在 AI 设置中选的激活模型，而不是写死 gpt-4o-mini
		if cfg.OpenAIModel != "" {
			model = cfg.OpenAIModel
		} else {
			model = "gpt-4o"
		}
	}

	// ③ 防御：空消息
	if len(body.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty_messages"})
		return
	}

	// ③½ 注入系统提示词（从配置读取，前端无需关心）
	//     配置为空时使用内置默认 prompt（强制 list_mounts + 禁止编造路径）
	finalMessages := body.Messages
	systemPrompt := cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultAgentSystemPrompt
	}
	finalMessages = make([]chatMsg, 0, len(body.Messages)+1)
	finalMessages = append(finalMessages, chatMsg{Role: "system", Content: systemPrompt})
	finalMessages = append(finalMessages, body.Messages...)

	// ③¾ 缓存 session：messages（不含 system）+ model + temperature
	//     —— confirm 时按 sessionId 取回继续对话
	sessID := body.SessionId
	if sessID == "" {
		sessID = "default"
	}
	sess := getOrCreateSession(sessID)
	sess.mu.Lock()
	sess.Messages = append([]chatMsg{}, body.Messages...) // 存用户原始 messages（不含 system）
	sess.LastModel = model
	sess.LastTemperature = body.Temperature
	sess.PendingTools = nil // 新一轮开始，清空旧的 pending
	sess.InProgress = true  // 标记流式生成中
	sess.mu.Unlock()

	// ════════════════════════════════════════════════════════════
	// ③⁴ Mock 模式短路（核心测试 / CI / 离线开发路径）
	// ════════════════════════════════════════════════════════════
	//
	// 触发条件：config.user.json 的 agent_settings.mock_mode 设为 "builtin" 或 "custom"。
	//   - builtin 模式无匹配 → Match 内部 fallback 到 default_friendly（不会落到这里）
	//   - custom 模式无匹配 → Match 返回 nil → 继续走真实 OpenAI
	// 短路后完全不调用 OpenAI/gptgod，0 token 消耗。
	//
	// 必须放在 session 缓存之后（需要 sess 写入 EventCache），
	// 必须放在 callOpenAIStream 之前（避免无谓的 API 请求）。
	agentCfg := s.getAgentConfig()
	mockMode := strings.ToLower(strings.TrimSpace(agentCfg.MockMode))

	// ════════════════════════════════════════════════════════════
	// AG-UI 协议模式检测（Phase 4）
	// ════════════════════════════════════════════════════════════
	// 当请求携带 X-Agent-Protocol: agui header 或 ?protocol=agui query 时，
	// 后端使用 AGUIEventMapper 输出标准 AG-UI 格式事件，而非自定义 SSE 格式。
	// 前端 TDesignEngine 通过此协议与后端通信。
	aguiMode := c.GetHeader("X-Agent-Protocol") == "agui" || c.Request.URL.Query().Get("protocol") == "agui"

	// ════════════════════════════════════════════════════════════
	// ③⁴ ⅰ mock_resume 路径（Task 11 / T15 unblock）
	// ════════════════════════════════════════════════════════════
	//
	// 当 body.Mode == "mock_resume" 且 body.Scenario 非空时，前端要求在已
	// 暂停的多轮 / 分支剧本上继续推事件。
	//   - 这里跳过 Match 关键词匹配（避免"开始"等 userText 重新匹配到首轮剧本）
	//   - 用 body.Scenario 在 mockScenariosV2 中查找剧本定义
	//   - 用 per-session v2 engine map 取出 / 创建一个 stateful 引擎
	//   - 调 engine.Resume 让 userText 进入 roundCtx 并推下一轮
	//
	// mock_resume 模式在 mock_mode = "off" 时也允许走（保留路径给未来扩展），
	// 但实际只有 mock_mode != "off" 才有可恢复的 v2 引擎。
	if strings.EqualFold(body.Mode, "mock_resume") && body.Scenario != "" {
		if handled, err := s.handleMockResume(c, body, mockMode, aguiMode); handled {
			if err != nil {
				slog.Warn("agent: mock_resume failed", "scenario", body.Scenario, "error", err)
			}
			return
		}
		// handled=false 表示找不到 v2 剧本 → 落到下方 default 流程（让前端知道是 404）
		c.JSON(http.StatusNotFound, gin.H{
			"error":    "mock_resume_scenario_not_found",
			"scenario": body.Scenario,
			"hint":     "v2 剧本必须存在于 mockScenariosV2 且 mode 为 builtin/custom",
		})
		return
	}

	if mockMode != "" && mockMode != "off" {
		userText := lastUserTextFromLoopMessages(body.Messages)
		scenario := s.mockEngine.Match(userText, mockMode)
		// v2 场景在 mockEngine.builtinScenarios 之外（保持 v1 builtin=12 不变），
		// 这里手动检查并补充
		if scenario == nil {
			for _, sc := range mockScenariosV2 {
				if sc.Rounds > 0 || len(sc.Branches) > 0 {
					if matchScenarioSimple(userText, sc) {
						scenario = sc
						break
					}
				}
			}
		}
		if scenario != nil {
			c.Header("X-Mock-Scenario", scenario.ID)
			c.Header("X-Mock-Mode", mockMode)
			if aguiMode {
				c.Header("X-Agent-Protocol", "agui")
			}

			flusher, _ := c.Writer.(http.Flusher)
			s.setSSEHeaders(c.Writer)
			slog.Info("agent: mock mode short-circuit",
				"mode", mockMode,
				"scenario", scenario.ID,
				"user_text", truncateForLog(userText, 100),
				"speed", agentCfg.MockSpeed,
				"agui_mode", aguiMode)
			// v2 场景（带 Rounds/Branches）走 MockEngineV2 路径
			if scenario.Rounds > 0 || scenario.TotalRounds > 0 || len(scenario.Branches) > 0 {
				v2 := NewMockEngineV2()
				if err := v2.Run(c.Request.Context(), s, sess, c.Writer, flusher, scenario,
					agentCfg.MockSpeed, aguiMode); err != nil {
					slog.Warn("agent: mock v2 engine run failed", "scenario", scenario.ID, "error", err)
				}
			} else {
				if err := s.mockEngine.Run(c.Request.Context(), s, sess, c.Writer, flusher, scenario,
					agentCfg.MockSpeed, true /* mockFlag */, aguiMode); err != nil {
					slog.Warn("agent: mock engine run failed", "scenario", scenario.ID, "error", err)
				}
			}
			sess.mu.Lock()
			sess.InProgress = false
			sess.mu.Unlock()
			return
		}
		// builtin 模式无匹配 → Match 内部已 fallback 到 default_friendly，不会到这里
		// custom 模式无匹配 → Match 返回 nil，落到这里 → 继续走真实 OpenAI
		if mockMode == "custom" {
			slog.Info("agent: custom mock no match, falling through to real API",
				"user_text", truncateForLog(userText, 200))
		}
	}

	// ④ Flusher 检测
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sse_not_supported"})
		return
	}

	// ④½ 提前设置 SSE headers —— 让客户端立即建立连接，
	//     Agent Loop 期间可通过同一连接推送进度事件（thinking / tool_executed）
	s.setSSEHeaders(c.Writer)
	// 初始注释确认 SSE 连接已建立
	c.Writer.Write([]byte(": agent loop starting\n\n"))
	flusher.Flush()

	// ⑤ 构建 OpenAI 兼容请求
	//
	// 关键：把 agent 工具列表（plugin 加密解密 + fs 只读）发给 LLM。
	// 之前这里没 "tools" 字段，agent 实际根本无法调任何工具——这是让 LLM "perceive
	// the mounted file system" 的真正入口。
	agentTools := s.ListAgentTools()
	openAITools := agentToolsToOpenAITools(agentTools)
	toolMeta := make(map[string]map[string]interface{}, len(agentTools))
	for _, t := range agentTools {
		if n, ok := t["name"].(string); ok {
			toolMeta[n] = t
		}
	}

	// ════════════════════════════════════════════════════════════
	// 阶段 1: Agent Tool Loop（参照 OpenAI Agents SDK 模式）
	// ════════════════════════════════════════════════════════════
	//
	// 核心循环：非流式调 LLM → 如果返回 tool_calls → 自动执行只读工具
	//           → 注入结果 → 继续循环 → 直到 LLM 返回纯文本
	//
	// 客户端无感知：工具执行完全在服务端完成，客户端只收到最终流式文本。
	// 只有需要用户确认的工具（加密/解密等）才会中断循环并推 approval 事件。
	// ════════════════════════════════════════════════════════════
	const maxAgentLoopRounds = 5
	var (
		loopMessages       = finalMessages // 循环内的 messages（包含 system + 历史对话）
		pendingTools       []toolCallAccumulator
		finalAssistantText string // LLM 最终文本回复
		autoToolExecuted   bool   // 是否有工具被执行过
		textSeq            int    // 全局 seq 计数器（跨 round 递增，供 text_delta 使用）
		reasoningSeq       int    // 全局 seq 计数器（跨 round 递增，供 reasoning_delta 使用）
	)

	for round := 0; round < maxAgentLoopRounds; round++ {
		slog.Info("agent: loop round",
			"round", round+1,
			"max_rounds", maxAgentLoopRounds,
			"messages_count", len(loopMessages),
			"tools_count", len(openAITools))

		// 推送循环进度事件（客户端可显示"正在思考..."或轮次指示器）
		s.sendSSEEventSafe(c.Writer, flusher, "stream_status", map[string]interface{}{
			"status":  "thinking",
			"round":   round + 1,
			"message": fmt.Sprintf("正在调用 LLM (第 %d/%d 轮)...", round+1, maxAgentLoopRounds),
		})

		// ═════════════════════════════════════════════════════
		// 流式调用 LLM —— 文本实时转发给客户端，tool_calls 累积后处理
		// ═════════════════════════════════════════════════════
		streamCh, err := callOpenAIStream(c.Request.Context(), cfg, model, body.Temperature, loopMessages, openAITools, aguiMode)
		if err != nil {
			slog.Warn("agent: loop stream failed", "round", round+1, "error", err)
			s.sendSSEEventSafe(c.Writer, flusher, "stream_error", map[string]interface{}{
				"code":    "llm_request_failed",
				"message": err.Error(),
				"round":   round + 1,
			})
			s.sendSSEEventSafe(c.Writer, flusher, "stream_end", "")
			return
		}

		// 读取流式事件：累积 tool_calls / 智能缓冲文本
		var (
			roundTextContent string
			roundToolCalls   []toolCallAccumulator
			tcAccumulator    = make(map[int]*toolCallAccumulator)
			finishReason     string
			gotToolCalls     bool

			// ═══ 平台级 Tool Use 智能缓冲 ═══
			// 问题：如果 LLM 输出工具调用 JSON（如 [{"name":"list_mounts",...}]），
			//       之前的代码会通过 text_delta 实时把原始 JSON 推送给用户。
			//       用户看到的是裸 JSON 而非工具执行结果。
			//
			// 解决：前 bufSizeLimit 字符进入缓冲区，检测是否像工具调用 JSON：
			//   - 以 [ 或 { 开头 + 包含 "name" 字段 → 进入"疑似工具调用"模式
			//     → 继续缓冲所有后续文本，不转发给客户端
			//     → 流结束后用 extractToolCallsFromText 解析
			//     → 解析成功 → 执行工具，JSON 永远不暴露给用户
			//     → 解析失败 → 补发所有缓冲的文本（降级为普通文本）
			//   - 不像工具调用 → 立即转发已缓冲的部分 + 切回实时模式
			textBuf           []string // 缓冲的 text_delta chunks
			bufMode           = true   // 是否在缓冲模式（前 N 字符）
			suspectedToolCall = false  // 是否检测到可能是工具调用
		)
		const bufSizeLimit = 60 // 缓冲阈值（字符数）

		// containsEmbeddedToolCallPattern 在任意位置扫描工具调用 JSON 特征。
		//
		// 参考 LobeChat 的协议级分离思路：LobeChat 通过 chunkType='text'/'tools_calling'
		// 在协议层面分离文本和工具调用。由于 gptgod 代理不发送标准 tool_call_chunk 事件，
		// 我们需要在文本层做更精确的启发式检测来模拟同样的效果。
		//
		// 此函数用于：
		//   1) looksLikeToolCheck 策略 2 — 嵌入式检测（中文正文后接 JSON）
		//   2) 实时模式的二次检测 — bufMode 已释放后发现后续 chunk 出现工具调用特征
		containsEmbeddedToolCallPattern := func(s string) bool {
			if len(s) < 20 {
				return false
			}
			return strings.Contains(s, `[{"name"`) ||
				strings.Contains(s, `{"name":"`) ||
				strings.Contains(s, `"function":`) ||
				strings.Contains(s, `"arguments":`)
		}

		// splitTextIntoChunks 将文本按字符数分割为等长大块。
		// 用于 Branch B 成功解析工具调用后，将 remainingText 分块作为 text_delta 补发给前端，
		// 模拟 LobeChat stream_chunk chunkType='text' 的增量推送效果。
		splitTextIntoChunks := func(text string, chunkSize int) []string {
			runes := []rune(text)
			if len(runes) <= chunkSize {
				return []string{text}
			}
			var chunks []string
			for i := 0; i < len(runes); i += chunkSize {
				end := i + chunkSize
				if end > len(runes) {
					end = len(runes)
				}
				chunks = append(chunks, string(runes[i:end]))
			}
			return chunks
		}

		// truncateStr 截断字符串到指定长度（用于日志预览）
		truncateStr := func(s string, maxLen int) string {
			if len(s) <= maxLen {
				return s
			}
			return s[:maxLen] + "..."
		}

		// looksLikeToolCall 检查累积文本是否看起来像工具调用 JSON
		looksLikeToolCall := func(s string) bool {
			trimmed := strings.TrimSpace(s)
			if len(trimmed) < 3 {
				return false
			}
			// 策略 1：文本以 [ 或 { 开头（原有逻辑 — 处理以 JSON 开头的回复）
			if (trimmed[0] == '[' || trimmed[0] == '{') &&
				strings.Contains(trimmed, `"name"`) {
				return true
			}
			// ★ 策略 2：文本任意位置嵌入工具调用 JSON 特征（新增）
			//    处理「中文正文 + 后接工具调用 JSON」的嵌入式场景
			return containsEmbeddedToolCallPattern(trimmed)
		}

		// flushBuffer 把缓冲的文本一次性转发给客户端（当确定不是工具调用时调用）
		// 架构升级：text_delta / reasoning_delta 事件从裸字符串改为 {seq, text} 结构，
		// 前端按 seq 排序渲染，解决 SSE chunk 乱序/丢包问题。
		flushBuffer := func() {
			for _, chunk := range textBuf {
				textSeq++
				s.sendAndCache(sess, c.Writer, flusher, "text_delta",
					map[string]interface{}{"seq": textSeq, "text": chunk})
			}
			textBuf = nil
		}

		for ev := range streamCh {
			switch ev.Type {
			case "text_delta":
				if textChunk, ok := ev.Data.(string); ok && textChunk != "" {
					roundTextContent += textChunk

					if suspectedToolCall {
						// 已确认为疑似工具调用 → 继续缓冲，不转发
						textBuf = append(textBuf, textChunk)
					} else if bufMode && len(roundTextContent) < bufSizeLimit {
						// 缓冲阶段：积累足够样本再判断
						textBuf = append(textBuf, textChunk)
						// 积累到一定量后判断
						if len(roundTextContent) >= bufSizeLimit || looksLikeToolCall(roundTextContent) {
							if looksLikeToolCall(roundTextContent) {
								suspectedToolCall = true
								slog.Info("agent: detected suspected tool call JSON, buffering",
									"prefix_len", len(roundTextContent),
									"preview", roundTextContent[:min(80, len(roundTextContent))])
							} else {
								// 不像工具调用 → 立即释放缓冲区，切回实时模式
								flushBuffer()
								bufMode = false
							}
						}
					} else {
						// 正常实时模式或缓冲已释放
						// ★ 二次检测（参考 LobeChat chunkType 分发思路）：
						//   如果累积文本中出现嵌入式工具调用特征，立即重新进入缓冲模式，
						//   防止工具调用 JSON 作为普通 text_delta 泄漏给前端。
						//   注意：只在 roundTextContent 上做检测（O(n) 字符串搜索），
						//   不对每个 chunk 做（避免性能问题）。
						if !suspectedToolCall && containsEmbeddedToolCallPattern(roundTextContent) {
							slog.Info("agent: mid-stream embedded tool call detected, re-buffering",
								"text_len", len(roundTextContent),
								"chunk_preview", truncateStr(textChunk, 40))
							suspectedToolCall = true
							bufMode = true
							textBuf = append(textBuf, textChunk)
						} else {
							textSeq++
							s.sendAndCache(sess, c.Writer, flusher, "text_delta",
								map[string]interface{}{"seq": textSeq, "text": textChunk})
						}
					}
				}
			case "reasoning_delta":
				if textChunk, ok := ev.Data.(string); ok && textChunk != "" {
					reasoningSeq++
					s.sendAndCache(sess, c.Writer, flusher, "reasoning_delta",
						map[string]interface{}{"seq": reasoningSeq, "text": textChunk})
				}
			case "tool_call_chunk":
				gotToolCalls = true
				tc := ev.Data.(toolCallAccumulator)
				cur, ok := tcAccumulator[tc.Index]
				if !ok {
					cur = &toolCallAccumulator{Index: tc.Index, Type: "function"}
					tcAccumulator[tc.Index] = cur
				}
				if tc.ID != "" {
					cur.ID = tc.ID
				}
				if tc.Type != "" {
					cur.Type = tc.Type
				}
				cur.Function.Name += tc.Function.Name
				cur.Function.Arguments += tc.Function.Arguments
			case "finish_reason":
				if s, ok := ev.Data.(string); ok {
					finishReason = s
				}
			case "stream_end":
				// 正常结束
			}
		}

		// 检测输出被 token 限制截断
		if finishReason == "length" {
			slog.Warn("agent: LLM response truncated (finish_reason=length)",
				"round", round+1,
				"text_len", len(roundTextContent),
				"text_tail", roundTextContent[max(0, len(roundTextContent)-100):])
		}

		// 收集所有累积的完整 tool_calls
		if gotToolCalls {
			for _, tc := range tcAccumulator {
				if tc.Function.Name != "" {
					roundToolCalls = append(roundToolCalls, *tc)
				}
			}
		}

		// ── 分支 A: LLM 返回了 tool_calls ──
		if gotToolCalls && len(roundToolCalls) > 0 {
			slog.Info("agent: loop tool_calls received (stream)",
				"round", round+1,
				"tool_count", len(roundToolCalls),
				"finish_reason", finishReason)

			// ★ 如果有缓冲区残留的前置文本（工具调用之前的正常正文），立即发送。
			// 这些文本在 tool_call_chunk 事件到达之前就已经产生，是安全的自然语言内容。
			// 参考 LobeChat stream_start 重置 accumulatedContent 的思路：
			// 在进入工具调用处理之前，先确保前置文本已正确投递给客户端。
			if len(textBuf) > 0 {
				flushBuffer()
				slog.Info("agent: Branch A flushed pre-tool-call buffered text",
					"buf_chunks", len(textBuf))
				textBuf = nil
			}

			// 把 assistant 的 tool_calls 消息追加到历史
			loopMessages = append(loopMessages, chatMsg{
				Role:      "assistant",
				Content:   roundTextContent,
				ToolCalls: roundToolCalls,
			})

			allAutoExecuted := true
			for _, tc := range roundToolCalls {
				needConfirm := true
				if meta, ok := toolMeta[tc.Function.Name]; ok {
					if v, ok := meta["needConfirm"].(bool); ok {
						needConfirm = v
					}
				}

				// ★ 无论是否需要确认，都向前端推送 tool_call 事件
				// 这样前端才能渲染 GroupedOperationMessage 等结构化组件
				s.emitToolCallEvent(sess, c.Writer, flusher, tc, toolMeta)

				if needConfirm {
					pendingTools = append(pendingTools, tc)
					allAutoExecuted = false
				} else {
					start := time.Now()
					result, execErr := s.executeAgentTool(
						c.Request.Context(), tc.Function.Name, tc.Function.Arguments)
					slog.Info("agent: loop tool executed",
						"name", tc.Function.Name,
						"duration_ms", time.Since(start).Milliseconds(),
						"has_error", execErr != nil)
					s.sendSSEEventSafe(c.Writer, flusher, "stream_status", map[string]interface{}{
						"status":      "tool_executed",
						"tool_name":   tc.Function.Name,
						"round":       round + 1,
						"duration_ms": time.Since(start).Milliseconds(),
					})
					// 推送 tool_status 事件，让前端 GroupedOperationMessage 更新状态徽章
					// 关键改动：异常时也推 tool_status { status: "error" } 而非 success
					// （参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §tool_status 同步）
					statusVal := "success"
					if execErr != nil {
						statusVal = "error"
					}
					s.sendAndCache(sess, c.Writer, flusher, "tool_status", map[string]interface{}{
						"id":     tc.ID,
						"status": statusVal,
					})
					// 推 tool_result 事件（带 isError / errorCode / errorMessage），
					// 让前端 useAgent 在收到此事件时把 tool_call.status 置为 error。
					// （参考 spec §tool_result 事件带 isError 字段）
					if execErr != nil {
						errCode, errMsg := classifyAgentToolError(execErr)
						result = fmt.Sprintf(`{"error":"tool_execution_failed","code":%q,"message":%q,"detail":%q}`,
							errCode, errMsg, execErr.Error())
						s.sendAndCache(sess, c.Writer, flusher, "tool_result", map[string]interface{}{
							"id":           tc.ID,
							"name":         tc.Function.Name,
							"result":       result,
							"isError":      true,
							"status":       "failed",
							"errorCode":    errCode,
							"errorMessage": errMsg,
						})
					} else {
						s.sendAndCache(sess, c.Writer, flusher, "tool_result", map[string]interface{}{
							"id":      tc.ID,
							"name":    tc.Function.Name,
							"result":  result,
							"isError": false,
							"status":  "success",
						})
					}
					loopMessages = append(loopMessages, chatMsg{
						Role: "tool", Content: result,
						ToolCallID: tc.ID, Name: tc.Function.Name,
					})
					autoToolExecuted = true
				}
			}

			if !allAutoExecuted {
				slog.Info("agent: loop exiting — tools need user confirmation",
					"pending_count", len(pendingTools))
				break
			}

			loopMessages = append(loopMessages, chatMsg{
				Role:    "user",
				Content: "[工具执行结果已注入。请基于以上结果回答用户的原始问题。]",
			})
			continue
		}

		// ── 分支 B: LLM 返回了纯文本（无 API 级 tool_calls）──
		//     注意：如果 suspectedToolCall=true，文本可能还在缓冲区中未转发！
		//     需要在 extractToolCallsFromText 结果出来后决定：丢弃（是工具调用）或补发（普通文本）

		// DEBUG: 记录 LLM 实际返回内容（截断到 500 字符），用于诊断工具调用为何不触发
		textPreview := roundTextContent
		if len(textPreview) > 500 {
			textPreview = textPreview[:500] + "...(truncated)"
		}
		slog.Info("agent: loop got text response",
			"round", round+1,
			"finish_reason", finishReason,
			"text_len", len(roundTextContent),
			"text_preview", textPreview,
			"suspected_tool_call", suspectedToolCall,
			"buf_mode", bufMode,
			"buf_len", len(textBuf))
		finalAssistantText = roundTextContent

		// 平台级 Tool Use：尝试从文本中解析工具调用 JSON（应对 API 代理丢弃 tools 参数的情况）
		parsedCalls, remainingText := extractToolCallsFromText(finalAssistantText)
		if len(parsedCalls) > 0 {
			// ★★ 工具调用成功解析 ★★
			// 如果文本在缓冲区中 → 丢弃缓冲区，用户永远看不到原始 JSON
			if suspectedToolCall || bufMode {
				slog.Info("agent: discarding buffered tool call JSON — user will not see raw JSON",
					"buf_size", len(textBuf))
				textBuf = nil // 丢弃缓冲区
				suspectedToolCall = false
				bufMode = false
			}

			slog.Info("agent: loop parsed tool calls from text (platform-level Tool Use) ★★ 工具调用成功解析 ★★",
				"round", round+1,
				"parsed_count", len(parsedCalls),
				"remaining_len", len(remainingText),
				"tool_names", func() (names []string) {
					ns := make([]string, len(parsedCalls))
					for i, c := range parsedCalls {
						ns[i] = c.Name
					}
					return ns
				}())

			accums := parsedToolCallsToAccumulator(parsedCalls)
			loopMessages = append(loopMessages, chatMsg{
				Role: "assistant", Content: finalAssistantText,
			})

			allAutoExecuted := true
			for _, tc := range accums {
				needConfirm := true
				if meta, ok := toolMeta[tc.Function.Name]; ok {
					if v, ok := meta["needConfirm"].(bool); ok {
						needConfirm = v
					}
				}

				// ★ 向前端推送 tool_call 事件（与 API 级 tool_calls 路径一致）
				s.emitToolCallEvent(sess, c.Writer, flusher, tc, toolMeta)

				if needConfirm {
					pendingTools = append(pendingTools, tc)
					allAutoExecuted = false
				} else {
					start := time.Now()
					result, execErr := s.executeAgentTool(
						c.Request.Context(), tc.Function.Name, tc.Function.Arguments)
					slog.Info("agent: loop parsed tool executed",
						"name", tc.Function.Name,
						"duration_ms", time.Since(start).Milliseconds(),
						"has_error", execErr != nil)
					s.sendSSEEventSafe(c.Writer, flusher, "stream_status", map[string]interface{}{
						"status":      "tool_executed",
						"tool_name":   tc.Function.Name,
						"round":       round + 1,
						"duration_ms": time.Since(start).Milliseconds(),
					})
					// 推送 tool_status 事件（平台级 Tool Use 路径）
					// 关键改动：异常时推 tool_status { status: "error" } 而非 success
					// （参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §tool_status 同步）
					parsedStatusVal := "success"
					if execErr != nil {
						parsedStatusVal = "error"
					}
					s.sendAndCache(sess, c.Writer, flusher, "tool_status", map[string]interface{}{
						"id":     tc.ID,
						"status": parsedStatusVal,
					})
					// 推 tool_result 事件（带 isError / errorCode / errorMessage），
					// 让前端 useAgent 在收到此事件时把 tool_call.status 置为 error。
					// （参考 spec §tool_result 事件带 isError 字段）
					if execErr != nil {
						errCode, errMsg := classifyAgentToolError(execErr)
						result = fmt.Sprintf(`{"error":"tool_execution_failed","code":%q,"message":%q,"detail":%q}`,
							errCode, errMsg, execErr.Error())
						s.sendAndCache(sess, c.Writer, flusher, "tool_result", map[string]interface{}{
							"id":           tc.ID,
							"name":         tc.Function.Name,
							"result":       result,
							"isError":      true,
							"status":       "failed",
							"errorCode":    errCode,
							"errorMessage": errMsg,
						})
					} else {
						s.sendAndCache(sess, c.Writer, flusher, "tool_result", map[string]interface{}{
							"id":      tc.ID,
							"name":    tc.Function.Name,
							"result":  result,
							"isError": false,
							"status":  "success",
						})
					}
					loopMessages = append(loopMessages, chatMsg{
						Role: "tool", Content: result,
						ToolCallID: tc.ID, Name: tc.Function.Name,
					})
					autoToolExecuted = true
				}
			}
			if !allAutoExecuted {
				break
			}
			loopMessages = append(loopMessages, chatMsg{
				Role:    "user",
				Content: "[工具执行结果已注入。请基于以上结果回答用户的原始问题。]",
			})

			// ★ 补发 remainingText（参考 LobeChat stream_chunk chunkType='text' 的增量模式）
			// extractToolCallsFromText 返回的 remainingText 是剥离工具调用 JSON 后的
			// 自然语言部分。LobeChat 不需要这个步骤因为它的协议天然分离（chunkType 区分），
			// 但我们的架构决定了必须手动补发，否则 JSON 之后的正文会丢失 → 回答截断。
			if remainingText != "" {
				chunks := splitTextIntoChunks(remainingText, 100)
				for _, ch := range chunks {
					textSeq++
					s.sendAndCache(sess, c.Writer, flusher, "text_delta",
						map[string]interface{}{"seq": textSeq, "text": ch})
				}
				slog.Info("agent: Branch B remaining text sent to client",
					"remaining_len", len(remainingText), "chunks", len(chunks))
			}

			continue
		}
		// 分支 B 也没有解析到工具调用 → LLM 输出了纯文本回复（非工具调用）
		// 如果之前在缓冲模式 → 需要补发缓冲的文本给客户端
		if suspectedToolCall || bufMode {
			slog.Info("agent: flushing buffered text — was suspected tool call but parsing failed",
				"buf_size", len(textBuf))
			flushBuffer()
			suspectedToolCall = false
			bufMode = false
		}

		slog.Info("agent: loop no tool calls found — LLM returned plain text response",
			"round", round+1,
			"auto_tool_executed", autoToolExecuted,
			"text_len", len(finalAssistantText))
		break
	}

	// 更新 session 的 messages（用于后续 resume/confirm）
	sess.mu.Lock()
	sess.Messages = append([]chatMsg{}, body.Messages...) // 用户原始消息
	// 把循环中产生的所有 assistant/tool 消息也存一份（简化：只存最后几轮的关键内容）
	if len(pendingTools) > 0 {
		sess.PendingTools = pendingTools
	}
	sess.mu.Unlock()

	// ════════════════════════════════════════════════════════════
	// 阶段 2: 流式输出给客户端
	// ════════════════════════════════════════════════════════════
	// （SSE headers 已在阶段 1 之前设置，无需重复）
	defer func() {
		sess.mu.Lock()
		sess.InProgress = false
		sess.mu.Unlock()
	}()

	// 2a: 有待确认工具 → 推送 tool_call 事件 + stream_end
	if len(pendingTools) > 0 {
		for _, tc := range pendingTools {
			s.emitToolCallEvent(sess, c.Writer, flusher, tc, toolMeta)
		}
		s.sendAndCache(sess, c.Writer, flusher, "stream_end", "")
		slog.Info("agent: chat completed (pending approval)",
			"pending_tools", len(pendingTools),
			"auto_executed", autoToolExecuted)
		return
	}

	// 2b: 有最终文本 → 文本已在 Agent Loop 中通过 streaming 实时发送
	//     这里只需确保 stream_end 被发出（作为安全兜底）
	if finalAssistantText != "" {
		s.sendAndCache(sess, c.Writer, flusher, "stream_end", "")
		slog.Info("agent: chat completed (text streamed in real-time)",
			"chars", len(finalAssistantText),
			"loop_rounds_executed", autoToolExecuted)
		return
	}

	// 2c: 兜底——LLM 未返回任何文本（finalAssistantText 为空）
	//     发送一个 text_delta 提示事件，避免前端显示"服务端返回空回复"
	textSeq++ // fallback 也递增 seq，保持全局唯一
	s.sendSSEEventSafe(c.Writer, flusher, "text_delta",
		map[string]interface{}{"seq": textSeq, "text": "（AI 助手未生成有效回复，可能需要换个问题或检查 API Key 配置）"})
	s.sendAndCache(sess, c.Writer, flusher, "stream_end", "")
	slog.Warn("agent: chat completed with no output (empty finalAssistantText)",
		"rounds", autoToolExecuted)
}

type openaiToolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (s *Server) emitToolCallEvent(sess *agentSession, w http.ResponseWriter, flusher http.Flusher, tc toolCallAccumulator, toolMeta map[string]map[string]interface{}) {
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
	// F 阶段：检查 session 授权表 → 已授权工具自动放行（auto_run=true）
	autoRun := false
	if needConfirm && sess != nil {
		sess.mu.Lock()
		autoRun = sess.GrantedTools[tc.Function.Name]
		sess.mu.Unlock()
	}

	payload := map[string]interface{}{
		"id":           tc.ID,
		"name":         tc.Function.Name,
		"args":         tc.Function.Arguments,
		"auto_run":     autoRun, // F 阶段：true=前端不弹 ApprovalCard 直接放行
		"needsConfirm": needConfirm && !autoRun,
		"kind":         kind,
	}
	s.sendAndCache(sess, w, flusher, "tool_call", payload)
}
