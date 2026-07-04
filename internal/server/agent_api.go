// internal/server/agent_api.go — 精简后保留：
//   - const block (cryptoPassphrase / cryptoSalt)
//   - registerAgentRoutes（路由注册表）
//   - handleAgentResetKey（清空 openai_api_key）
//   - buildChatCompletionsURL（OpenAI 兼容 URL 拼接）
//
// 其余 handler / type / func 已拆分到：
//   - agent_crypto.go      API Key 加解密（deriveKey / EncryptApiKey / DecryptApiKey / scrypt）
//   - agent_config.go      agentConfig + 读/写/解析
//   - agent_models.go      /api/models + 模型排序
//   - agent_keys.go        /api/encrypt-key + /api/decrypt-key + /test
//   - agent_chat.go        /api/chat + tool call accumulator
//   - agent_confirm.go     /api/confirm + /api/resume
//   - agent_sse.go         SSE header/event/sendAndCache
//   - agent_mock_extras.go chatMsg / handleMockResume / handleAgentMockPresets + 工具函数
//   - agent_stats.go       AgentEvent + maxEventID
//
// internal/server/agent_api.go
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// ─── API Key 加密/解密（防止 config.user.json 明文暴露） ──────
// 使用 AES-256-CBC + scrypt 密钥派生（与 Node.js agent-stub 兼容）
// 存储格式: enc:<base64(iv:ciphertext_with_pkcs7_padding)>

const (
	cryptoPassphrase = "encv-agent-key-v1"
	cryptoSalt       = "encv-mobile-salt-2024"
)

func (s *Server) registerAgentRoutes(r *gin.Engine) {
	r.GET("/api/models", s.handleAgentModels)
	r.POST("/api/encrypt-key", s.handleAgentEncryptKey)
	r.POST("/api/decrypt-key", s.handleAgentDecryptKey)
	r.POST("/api/agent/reset-key", s.handleAgentResetKey)
	r.GET("/api/agent/context-usage", s.handleAgentContextUsage)
	r.GET("/api/agent/mock/presets", s.handleAgentMockPresets)
	r.GET("/test", s.handleAgentTest)
	r.POST("/test", s.handleAgentTest)
	r.POST("/api/chat", s.handleAgentChat)
	r.POST("/api/confirm", s.handleAgentConfirm)
	r.POST("/api/agent/branch-pick", s.handleAgentBranchPick) // 剧本外置 spec：预设选项 chip
	r.POST("/api/resume", s.handleAgentResume)

	slog.Info("Agent API routes registered (integrated into encv-go)")
}

func (s *Server) handleAgentResetKey(c *gin.Context) {
	if s.configPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "config path not available"})
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	// 1. 读现有 config
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		slog.Warn("agent: reset-key cannot read config", "path", s.configPath, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot_read_config", "detail": err.Error()})
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Warn("agent: reset-key cannot parse config", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_config", "detail": err.Error()})
		return
	}

	// 2. 拿 agent_settings 块
	agentRaw, ok := raw["agent_settings"]
	if !ok {
		// agent_settings 块本就不存在 → 没东西可清，直接成功
		slog.Info("agent: reset-key no-op (agent_settings absent)")
		c.JSON(http.StatusOK, gin.H{"reset": false, "reason": "no agent_settings block"})
		return
	}
	var agent map[string]interface{}
	if err := json.Unmarshal(agentRaw, &agent); err != nil {
		slog.Warn("agent: reset-key cannot parse agent_settings", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_agent_settings", "detail": err.Error()})
		return
	}

	// 3. 记录原值（仅长度，不打内容，避免日志泄露密文）
	prev, _ := agent["openai_api_key"].(string)
	prevLen := len(prev)

	// 4. 清空
	agent["openai_api_key"] = ""
	newAgent, _ := json.Marshal(agent)
	raw["agent_settings"] = newAgent

	// 5. 写回（保留缩进风格）
	indented, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal_failed", "detail": err.Error()})
		return
	}
	if err := os.WriteFile(s.configPath, append(indented, '\n'), 0644); err != nil {
		slog.Error("agent: reset-key write config failed", "path", s.configPath, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write_failed", "detail": err.Error()})
		return
	}

	slog.Info("agent: reset-key cleared openai_api_key from config", "path", s.configPath, "prev_len", prevLen)
	c.JSON(http.StatusOK, gin.H{
		"reset":   true,
		"prevLen": prevLen,
		"message": "openai_api_key has been cleared. Please re-enter the key in AI Settings.",
	})
}

// ─── GET /api/models — 从供应商获取模型列表 ─────────────────

// buildChatCompletionsURL 拼接 OpenAI 兼容 chat completions 端点 URL。
//
// 关键修复：base URL 已经包含 /v1 后缀时不能重复拼接。
// 之前无条件 + "/v1/chat/completions" 导致 base_url="https://api.openai.com/v1" 时
// 实际请求 URL 变成 "https://api.openai.com/v1/v1/chat/completions" → 上游 404 / EOF。
//
// 规则：
//   - 去掉尾部 /
//   - 去掉已存在的 /v1 后缀（不区分大小写，兼容 https://api.openai.com/V1 等）
//   - 拼接标准 /v1/chat/completions
//
// 用例：
//
//	"https://api.openai.com/v1"   → "https://api.openai.com/v1/chat/completions"
//	"https://api.openai.com/v1/"  → "https://api.openai.com/v1/chat/completions"
//	"https://api.openai.com"      → "https://api.openai.com/v1/chat/completions"
//	"https://api.openai.com/V1"   → "https://api.openai.com/v1/chat/completions"

func buildChatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	// 去掉已存在的 /v1 后缀（不区分大小写，避免 https://api.openai.com/V1 的边界情况）
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		base = strings.TrimRight(base[:len(base)-3], "/")
	}
	return base + "/v1/chat/completions"
}
