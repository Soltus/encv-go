package server

// agent_mock_extras.go — 从原 agent_api.go 提取的 mock 相关声明
//（chatMsg / scenarioPickerEntry / handleMockResume / handleAgentMockPresets / lastUserTextFromLoopMessages / classifyAgentToolError / truncateForLog）

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Soltus/encv-go/internal/tools"
)

func lastUserTextFromLoopMessages(msgs []chatMsg) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

func classifyAgentToolError(err error) (code, message string) {
	if err == nil {
		return "", ""
	}
	if te := tools.AsToolError(err); te != nil {
		if te.Code != "" {
			code = te.Code
		} else {
			code = tools.CodeUnknown
		}
		if te.Message != "" {
			message = te.Message
		} else {
			message = err.Error()
		}
		return code, message
	}
	// 兜底：非 ToolError 类型 → 给通用码
	return tools.CodeExecFailed, err.Error()
}

func truncateForLog(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

type chatMsg struct {
	Role       string                `json:"role"`
	Content    string                `json:"content"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
	Name       string                `json:"name,omitempty"`
	ToolCalls  []toolCallAccumulator `json:"tool_calls,omitempty"`
}

func (s *Server) handleMockResume(
	c *gin.Context,
	body chatRequest,
	mockMode string,
	aguiMode bool,
) (bool, error) {
	scenario := lookupMockScenarioV2(body.Scenario)
	if scenario == nil {
		return false, nil
	}
	if mockMode == "off" || mockMode == "" {
		slog.Warn("agent: mock_resume called with mock_mode=off — round state may be lost",
			"scenario", body.Scenario, "session", body.SessionId)
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sse_not_supported"})
		return true, fmt.Errorf("sse not supported")
	}

	// 取出 / 创建 session（沿用现有 session 体系，断点续传 EventCache）
	sessID := body.SessionId
	if sessID == "" {
		sessID = "default"
	}
	sess := getOrCreateSession(sessID)
	sess.mu.Lock()
	sess.InProgress = true
	sess.mu.Unlock()

	// 取出 userText（从最后一条 user 消息）
	userText := lastUserTextFromLoopMessages(body.Messages)

	// 设 SSE header
	c.Header("X-Mock-Scenario", scenario.ID)
	c.Header("X-Mock-Mode", mockMode)
	if aguiMode {
		c.Header("X-Agent-Protocol", "agui")
	}
	s.setSSEHeaders(c.Writer)
	flusher.Flush()

	// 取出 / 创建 stateful v2 引擎
	eng := getOrCreateV2Engine(sessID, scenario)

	slog.Info("agent: mock_resume dispatch",
		"mode", body.Mode,
		"scenario", scenario.ID,
		"session", sessID,
		"user_text", truncateForLog(userText, 100),
		"current_round", eng.CurrentRound(),
		"branch_id", eng.CurrentBranchID())

	// Resume 推进下一轮。Resume 内部：
	//   - 把 userText 写进 roundCtx["user_text"] 和 roundCtx["round_N_user_text"]
	//   - 推 mock_round_state{phase:resumed}
	//   - 推进 round 计数
	//   - 推下一轮 steps
	//   - 若该 step 是 BranchChoice → 推 mock_branch_choice 并等待 PickBranch
	//   - 若该 step 是 PauseForUser → 推 mock_round_state{awaiting_user_input} 并等待 Resume
	//   - 若是最后一轮 → 推 stream_end
	if err := eng.Resume(c.Request.Context(), s, sess, c.Writer, flusher, userText); err != nil {
		sess.mu.Lock()
		sess.InProgress = false
		sess.mu.Unlock()
		return true, fmt.Errorf("Resume: %w", err)
	}
	sess.mu.Lock()
	sess.InProgress = false
	sess.mu.Unlock()
	return true, nil
}

type scenarioPickerEntry struct {
	ID          string `json:"id"`
	ScenarioID  string `json:"scenarioId"`
	Label       string `json:"label"`
	UserText    string `json:"userText"`
	Icon        string `json:"icon,omitempty"`
	Tooltip     string `json:"tooltip,omitempty"`
	Description string `json:"description,omitempty"`
}

func (s *Server) handleAgentMockPresets(c *gin.Context) {
	cfg := s.getAgentConfig()
	mode := cfg.MockMode

	// mock 模式关闭时返回空（前端 v-if 自然不渲染）
	if mode == "off" || mode == "" {
		c.JSON(http.StatusOK, gin.H{
			"scenario": "",
			"phase":    "off",
			"presets":  []scenarioPickerEntry{},
			"mockMode": mode,
		})
		return
	}

	// 遍历所有内置 + 自定义剧本，每个转成一个 picker entry
	allScenarios := s.mockEngine.AllScenarios()
	entries := make([]scenarioPickerEntry, 0, len(allScenarios))

	for _, sc := range allScenarios {
		// 跳过"无 Presets 字段"的剧本（理论上 12 个 builtin 都有）
		if len(sc.Presets) == 0 {
			continue
		}
		// picker 入口：取剧本第一个 Preset 的 userText 作为触发关键词
		firstPreset := sc.Presets[0]
		entries = append(entries, scenarioPickerEntry{
			ID:          "pick_" + sc.ID,
			ScenarioID:  sc.ID,
			Label:       "🎬 " + sc.ID,
			UserText:    firstPreset.UserText,
			Icon:        "🎬",
			Tooltip:     sc.Description,
			Description: sc.Description,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"scenario": "scenario_picker",
		"phase":    "picker",
		"presets":  entries,
		"mockMode": mode,
	})
}
