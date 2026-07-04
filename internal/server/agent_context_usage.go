// internal/server/agent_context_usage.go
//
// GET /api/agent/context-usage — 查询当前会话的上下文使用情况
//
// 设计原则：
//  1. 完全从 session.Messages 派生（无独立 state），保证与 /api/chat 流一致
//  2. 估算 token 数（char/3 启发式，对中英混合较准）
//  3. 抽取最近一次 plan tool (write_todos) 的 todos
//  4. 抽取所有 read_file / list_files 工具调用涉及的引用文件
//  5. 模型上下文窗口查表（硬编码常见模型）
//
// 返回结构（供前端 context icon 弹窗用）：
//
//	{
//	  "sessionId": "default",
//	  "model":     "gpt-4",
//	  "usage":     {"tokens": 12345, "window": 128000, "percent": 9.6},
//	  "todos":     [{"content": "…", "status": "in_progress"}, ...],
//	  "referencedFiles": [{"path": "x.txt", "mountId": "serving", "viaTool": "read_file", "lastRefAt": 12345}, ...],
//	  "compactions": 0,
//	  "updatedAt": 12345
//	}
package server

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

// ─── 模型上下文窗口查表 ──────────────────────────────────────

// modelContextWindows 常见模型的 context window（token）
// 不在表内的模型默认 8192（保守）
var modelContextWindows = map[string]int{
	// OpenAI
	"gpt-4":               8192,
	"gpt-4-32k":           32768,
	"gpt-4-turbo":         128000,
	"gpt-4-turbo-preview": 128000,
	"gpt-4o":              128000,
	"gpt-4o-mini":         128000,
	"gpt-3.5-turbo":       16385,
	"gpt-3.5-turbo-16k":   16385,
	"o1":                  200000,
	"o1-mini":             128000,
	"o1-preview":          128000,
	"o3-mini":             200000,
	// Anthropic (若供应商代理了 /v1)
	"claude-3-5-sonnet": 200000,
	"claude-3-5-haiku":  200000,
	"claude-3-opus":     200000,
	"claude-3-sonnet":   200000,
	"claude-3-haiku":    200000,
	// DeepSeek
	"deepseek-chat":     64000,
	"deepseek-coder":    64000,
	"deepseek-reasoner": 64000,
	// Qwen
	"qwen-turbo":           1000000,
	"qwen-plus":            131072,
	"qwen-max":             32768,
	"qwen2.5-72b-instruct": 131072,
	// 智谱 GLM
	"glm-4":      100000,
	"glm-4-plus": 128000,
	"glm-4-long": 1000000,
	// Moonshot
	"moonshot-v1-8k":   8000,
	"moonshot-v1-32k":  32000,
	"moonshot-v1-128k": 128000,
}

// modelMaxOutputTokens 常见模型的单次输出上限（max_completion_tokens 硬上限）
// 注意：上下文窗口 128k 不等于输出 128k。OpenAI 官方 gpt-4o 输出上限 16,384。
// 来源：各厂商公开文档（截至 2026-06）。
//   - OpenAI:  https://platform.openai.com/docs/models
//   - Anthropic: https://docs.anthropic.com/en/docs/about-claude/models
//   - DeepSeek: https://api-docs.deepseek.com/quick_start/pricing
//   - Qwen:    https://help.aliyun.com/zh/model-studio
//   - GLM:     https://open.bigmodel.cn/dev/api
//
// 不在表内默认 4096（保守，绝不超出厂商上限）。
var modelMaxOutputTokens = map[string]int{
	// OpenAI
	"gpt-4":               4096,
	"gpt-4-32k":           4096,
	"gpt-4-turbo":         4096,
	"gpt-4-turbo-preview": 4096,
	"gpt-4o":              16384,
	"gpt-4o-mini":         16384,
	"gpt-3.5-turbo":       4096,
	"gpt-3.5-turbo-16k":   4096,
	"o1":                  100000,
	"o1-mini":             65536,
	"o1-preview":          32768,
	"o3-mini":             100000,
	// Anthropic (经 OpenAI 兼容代理)
	"claude-3-5-sonnet": 8192,
	"claude-3-5-haiku":  8192,
	"claude-3-opus":     4096,
	"claude-3-sonnet":   4096,
	"claude-3-haiku":    4096,
	// DeepSeek
	"deepseek-chat":     8000,
	"deepseek-coder":    8000,
	"deepseek-reasoner": 8000,
	// Qwen
	"qwen-turbo":           8192,
	"qwen-plus":            8192,
	"qwen-max":             8192,
	"qwen2.5-72b-instruct": 8192,
	// 智谱 GLM
	"glm-4":      4096,
	"glm-4-plus": 4096,
	"glm-4-long": 4096,
	// Moonshot
	"moonshot-v1-8k":   8000,
	"moonshot-v1-32k":  32000,
	"moonshot-v1-128k": 128000,
}

// lookupContextWindow 根据模型名查 context window
// 不在表内则用启发式（名字含 128k → 128000，含 32k → 32000，含 16k → 16385，含 8k → 8192）
// 都没有则默认 8192
func lookupContextWindow(model string) int {
	if model == "" {
		return 8192
	}
	if w, ok := modelContextWindows[model]; ok {
		return w
	}
	// 启发式匹配
	lower := model
	switch {
	case containsAny(lower, "128k", "128000"):
		return 128000
	case containsAny(lower, "200k", "200000", "1m", "1M"):
		return 1000000
	case containsAny(lower, "64k", "64000"):
		return 64000
	case containsAny(lower, "32k", "32000"):
		return 32000
	case containsAny(lower, "16k", "16000"):
		return 16385
	case containsAny(lower, "8k", "8000"):
		return 8192
	}
	return 8192
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) <= len(s) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// lookupMaxOutputTokens 查模型单次输出硬上限
// 不在表内则用启发式（名字含 16k → 16384，含 8k → 8192，含 4k → 4096），都没有默认 4096
func lookupMaxOutputTokens(model string) int {
	if model == "" {
		return 4096
	}
	if v, ok := modelMaxOutputTokens[model]; ok {
		return v
	}
	lower := model
	switch {
	case containsAny(lower, "16k", "16000"):
		return 16384
	case containsAny(lower, "8k", "8000"):
		return 8192
	case containsAny(lower, "4k", "4000"):
		return 4096
	}
	return 4096
}

// ─── Token 估算 ──────────────────────────────────────────────

// estimateTokens 启发式估算 message 数组的 token 数
// 中英混合 char/3；纯 ASCII char/4
// 这是近似值——精确 tokenization 需要 tiktoken/go 库
func estimateTokens(messages []chatMsg) int {
	total := 0
	for _, m := range messages {
		total += estimateStringTokens(m.Content)
		// tool_calls args 也会被算入
		for _, tc := range m.ToolCalls {
			total += estimateStringTokens(tc.Function.Name)
			total += estimateStringTokens(tc.Function.Arguments)
		}
		// role 占 4 token（OpenAI chatml 格式）
		total += 4
	}
	return total
}

// estimateStringTokens 启发式：CJK 字符按 1.5/字符，ASCII 按 4/字符
func estimateStringTokens(s string) int {
	if s == "" {
		return 0
	}
	cjk, ascii := 0, 0
	for _, r := range s {
		if r < 0x80 {
			ascii++
		} else {
			cjk++
		}
	}
	// CJK token ≈ 1.5 chars, ASCII token ≈ 4 chars
	tokens := int(math.Round(float64(cjk)/1.5 + float64(ascii)/4.0))
	return tokens
}

// ─── Todos 抽取 ──────────────────────────────────────────────

// planTodo plan tool (write_todos) 的单条 todo
type planTodo struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending / in_progress / completed
}

// extractTodos 从 messages 中抽出最近一次 plan tool call 的 todos
// 优先取 tool result（最权威），fallback 到 tool call args
func extractTodos(messages []chatMsg) []planTodo {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role != "assistant" {
			continue
		}
		for _, tc := range m.ToolCalls {
			// plan tool name 约定为 write_todos（来自 codex-web）
			// 也兼容 set_plan / plan_update
			if !isPlanToolName(tc.Function.Name) {
				continue
			}
			// 尝试解析 args
			if todos := parseTodosJSON(tc.Function.Arguments); len(todos) > 0 {
				return todos
			}
		}
	}
	return nil
}

func isPlanToolName(name string) bool {
	switch name {
	case "write_todos", "set_plan", "plan_update", "todos", "update_todos":
		return true
	}
	return false
}

// parseTodosJSON 解析 [{"content": "...", "status": "..."}, ...] 形式
// 也兼容 {todos: [...]} 包装
func parseTodosJSON(s string) []planTodo {
	if s == "" {
		return nil
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return todosFromArray(arr)
	}
	var wrapped struct {
		Todos []map[string]interface{} `json:"todos"`
	}
	if err := json.Unmarshal([]byte(s), &wrapped); err == nil {
		return todosFromArray(wrapped.Todos)
	}
	return nil
}

func todosFromArray(arr []map[string]interface{}) []planTodo {
	if len(arr) == 0 {
		return nil
	}
	out := make([]planTodo, 0, len(arr))
	for _, item := range arr {
		content, _ := item["content"].(string)
		if content == "" {
			if activeForm, _ := item["active_form"].(string); activeForm != "" {
				content = activeForm
			}
		}
		status, _ := item["status"].(string)
		if status == "" {
			status = "pending"
		}
		if content != "" {
			out = append(out, planTodo{Content: content, Status: status})
		}
	}
	return out
}

// ─── 引用文件抽取 ────────────────────────────────────────────

// referencedFile 一个被工具调用引用的文件
type referencedFile struct {
	Path      string `json:"path"`
	MountID   string `json:"mountId"`
	ViaTool   string `json:"viaTool"`
	LastRefAt int64  `json:"lastRefAt"`
}

// extractReferencedFiles 扫描所有 tool_call.args，提取 read_file / list_files 涉及的路径
// 同路径保留最近一次引用
func extractReferencedFiles(messages []chatMsg) []referencedFile {
	byPath := make(map[string]referencedFile)
	// 按时间顺序：索引小的更早，索引大的更晚
	for idx, m := range messages {
		if m.Role != "assistant" {
			continue
		}
		for _, tc := range m.ToolCalls {
			ref := readPathFromToolArgs(tc.Function.Name, tc.Function.Arguments)
			if ref == nil {
				continue
			}
			key := ref.MountID + "|" + ref.Path
			byPath[key] = referencedFile{
				Path:      ref.Path,
				MountID:   ref.MountID,
				ViaTool:   tc.Function.Name,
				LastRefAt: int64(idx),
			}
		}
	}
	if len(byPath) == 0 {
		return nil
	}
	out := make([]referencedFile, 0, len(byPath))
	for _, v := range byPath {
		out = append(out, v)
	}
	// 按 lastRefAt 降序
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastRefAt > out[j].LastRefAt
	})
	return out
}

// readPathFromToolArgs 从工具 args 抽取 (mountId, path) 二元组
// 覆盖 v1 + v2 工具集。任何以 (mount_id, rel_path) 为参数的 fs 工具都会被记录。
func readPathFromToolArgs(toolName, argsJSON string) *struct {
	MountID string
	Path    string
} {
	// v1 工具（保留兼容）
	switch toolName {
	case "read_file", "stat_file":
		var args struct {
			MountID string `json:"mount_id"`
			RelPath string `json:"rel_path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err == nil && args.RelPath != "" {
			return &struct {
				MountID string
				Path    string
			}{MountID: args.MountID, Path: args.RelPath}
		}
	case "list_files":
		var args struct {
			MountID string `json:"mount_id"`
			RelPath string `json:"rel_path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
			p := args.RelPath
			if p == "" {
				p = "/"
			}
			return &struct {
				MountID string
				Path    string
			}{MountID: args.MountID, Path: p}
		}
	}
	// v2 工具：6 个共享 (mount_id, rel_path) 契约的工具
	switch toolName {
	case "read_file_v2", "get_metadata", "edit_metadata", "delete_file":
		var args struct {
			MountID string `json:"mount_id"`
			RelPath string `json:"rel_path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err == nil && args.RelPath != "" {
			return &struct {
				MountID string
				Path    string
			}{MountID: args.MountID, Path: args.RelPath}
		}
	case "search_files":
		var args struct {
			MountID string `json:"mount_id"`
			RelPath string `json:"rel_path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
			p := args.RelPath
			if p == "" {
				p = "/"
			}
			return &struct {
				MountID string
				Path    string
			}{MountID: args.MountID, Path: p}
		}
	case "batch_rename":
		// batch_rename 的语义是对 mount 下 root 起作用，pattern 是模板；记录 root 即可
		var args struct {
			MountID string `json:"mount_id"`
			RelPath string `json:"rel_path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err == nil && args.RelPath != "" {
			return &struct {
				MountID string
				Path    string
			}{MountID: args.MountID, Path: args.RelPath}
		}
	}
	// command_run / batch_rename_v2-without-relpath / 其他无法静态抽取的工具 → 跳过
	return nil
}

// ─── Compactions 计数 ────────────────────────────────────────

// countCompactions 扫描 EventCache，统计 type=context_compaction 的事件数
func countCompactions(sess *agentSession) int {
	if sess == nil {
		return 0
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	n := 0
	for _, ev := range sess.EventCache {
		if ev.Type == "context_compaction" {
			n++
		}
	}
	return n
}

// ─── HTTP Handler ────────────────────────────────────────────

// handleAgentContextUsage GET /api/agent/context-usage?sessionId=default
func (s *Server) handleAgentContextUsage(c *gin.Context) {
	sessionId := c.Query("sessionId")
	if sessionId == "" {
		sessionId = "default"
	}

	// deviceId 用于确认 session 的归属（虽然目前 session 索引不区分 device）
	_ = c.Query("deviceId")

	// 获取 session
	sessionMu.RLock()
	sess, ok := sessions[sessionId]
	sessionMu.RUnlock()

	if !ok {
		// 无 session：返回零状态
		// 仍然用真实激活模型查表，避免前端显示 8192 误导用户
		activeModel := s.resolveActiveModel(c.Query("deviceId"))
		c.JSON(http.StatusOK, gin.H{
			"sessionId":       sessionId,
			"model":           activeModel,
			"usage":           gin.H{"tokens": 0, "window": lookupContextWindow(activeModel), "percent": 0.0},
			"todos":           []planTodo{},
			"referencedFiles": []referencedFile{},
			"compactions":     0,
			"updatedAt":       time.Now().UnixMilli(),
			"note":            "无活动 session（已使用激活模型 " + activeModel + " 查表）",
		})
		return
	}

	sess.mu.Lock()
	msgs := append([]chatMsg(nil), sess.Messages...) // 拷贝一份避免持锁过久
	model := sess.LastModel
	sess.mu.Unlock()

	tokens := estimateTokens(msgs)
	window := lookupContextWindow(model)
	percent := 0.0
	if window > 0 {
		percent = float64(tokens) / float64(window) * 100
		percent = math.Round(percent*10) / 10 // 1 位小数
	}
	todos := extractTodos(msgs)
	refs := extractReferencedFiles(msgs)
	compactions := countCompactions(sess)

	c.JSON(http.StatusOK, gin.H{
		"sessionId":       sessionId,
		"model":           model,
		"usage":           gin.H{"tokens": tokens, "window": window, "percent": percent},
		"todos":           todos,
		"referencedFiles": refs,
		"compactions":     compactions,
		"updatedAt":       time.Now().UnixMilli(),
	})
}
