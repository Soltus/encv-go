// Package stubprovider 提供「模拟的用户配置 agent 服务商」：
// 一个 OpenAI 兼容的 chat/completions 替身服务。
//
// 与 mock 剧本的本质区别（这是本包存在的唯一理由）：
//
//	mock 剧本     —— 连**工具结果**都是写死的 ⇒ 跑完什么都证明不了（假剧本）
//	stub provider —— 只替代"模型大脑"：说什么、决定调哪个工具由脚本给；
//	                 但工具调用是真的、工具结果来自真实执行、
//	                 SSE / 投影 / 确认 / 前端渲染全是真的
//
// 因此它的正确用法是：把 agent 配置里的 openai_base_url 指向它
// （agent 本来就支持用户自填服务商），演示跑的就是真实链路，
// 只有"思考"部分是确定性的替身。
//
// 启用必须显式：ENCV_STUB_PROVIDER=1。默认不开 —— 替身大脑不能悄悄冒充真实模型。
package stubprovider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// ToolCall 是替身大脑决定发起的一次真实工具调用。
//
// Args 是**过滤条件**（如 mount_id / rel_path），不是数据：
// 真正的数据由工具真实执行后返回，替身不参与。
type ToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// Turn 是一轮模型输出：一段文本，或若干工具调用（或两者皆有）。
type Turn struct {
	Text      string     `json:"text,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Script 是一个替身剧本序列。Turns[i] 在第 i 个 assistant 回合输出。
type Script struct {
	ID    string `json:"id"`
	Turns []Turn `json:"turns"`
}

// Provider 是替身服务商本体（并发安全）。
type Provider struct {
	mu        sync.Mutex
	scripts   map[string]*Script
	defaultID string
	requests  int
}

// New 构造 Provider 并注册内置脚本。
func New() *Provider {
	p := &Provider{scripts: map[string]*Script{}}
	p.mustRegister(&Script{
		ID: "fs_overview",
		Turns: []Turn{
			{Text: "我先看看有哪些挂载点。\n", ToolCalls: []ToolCall{{Name: "list_mounts"}}},
			{Text: "再看根目录下有什么。\n", ToolCalls: []ToolCall{{Name: "list_files", Args: map[string]any{"mount_id": "serving", "rel_path": "/"}}}},
			{Text: "以上列表由**真实工具**返回：挂载点、文件名、大小都来自真实文件系统。替身的只有「我该说什么、该调哪个工具」，不含任何结果数据。\n"},
		},
	})
	p.mustRegister(&Script{
		ID: "storage",
		Turns: []Turn{
			{Text: "查一下磁盘空间。\n", ToolCalls: []ToolCall{{Name: "get_storage_info"}}},
			{Text: "容量数字来自真实磁盘统计。\n"},
		},
	})
	p.defaultID = "fs_overview"
	return p
}

func (p *Provider) mustRegister(s *Script) {
	if err := p.RegisterScript(s); err != nil {
		panic(err)
	}
}

// RegisterScript 注册/覆盖一个脚本。
func (p *Provider) RegisterScript(s *Script) error {
	if s == nil || strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("stubprovider: script id required")
	}
	if len(s.Turns) == 0 {
		return fmt.Errorf("stubprovider: script %q has no turns", s.ID)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scripts[s.ID] = s
	return nil
}

// SetDefault 设置未显式指定脚本时的默认脚本。
func (p *Provider) SetDefault(id string) {
	p.mu.Lock()
	p.defaultID = id
	p.mu.Unlock()
}

// Scripts 返回已注册脚本 ID 列表（供 /api/runtime 与跑批使用）。
func (p *Provider) Scripts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.scripts))
	for id := range p.scripts {
		out = append(out, id)
	}
	return out
}

// RequestCount 返回已处理的请求数（集成测试用它证明"第二轮真的发生了"）。
func (p *Provider) RequestCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

// Handler 返回 net/http handler（挂到任意路径前缀下）。
func (p *Provider) Handler() http.Handler { return http.HandlerFunc(p.ServeHTTP) }

func (p *Provider) resolveScriptID(model string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.HasPrefix(model, "stub:") {
		return strings.TrimPrefix(model, "stub:")
	}
	return p.defaultID
}

func (p *Provider) lookup(id string) (*Script, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests++
	s, ok := p.scripts[id]
	return s, ok
}

// ServeHTTP 处理 POST /v1/chat/completions（流式与非流式）。
func (p *Provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":{"message":"stubprovider: POST only"}}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"stubprovider: bad request body"}}`, http.StatusBadRequest)
		return
	}

	scriptID := p.resolveScriptID(req.Model)
	sc, ok := p.lookup(scriptID)
	if !ok {
		// fail closed：未知脚本直接报错，绝不悄悄 fallback 到别的剧本
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": fmt.Sprintf("stubprovider: unknown script %q (registered: %v)", scriptID, p.Scripts()),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// 回合推进：已存在的 assistant 消息数 = 已经输出过的回合数
	turnIdx := 0
	for _, m := range req.Messages {
		if m.Role == "assistant" {
			turnIdx++
		}
	}
	if turnIdx >= len(sc.Turns) {
		turnIdx = len(sc.Turns) - 1
	}
	turn := sc.Turns[turnIdx]

	if !req.Stream {
		p.writeNonStream(w, turn)
		return
	}
	p.writeStream(w, turn, turnIdx)
}

func (p *Provider) writeNonStream(w http.ResponseWriter, turn Turn) {
	toolCalls := make([]map[string]any, 0, len(turn.ToolCalls))
	for i, tc := range turn.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Args)
		if string(argsJSON) == "null" {
			argsJSON = []byte("{}")
		}
		toolCalls = append(toolCalls, map[string]any{
			"id":   fmt.Sprintf("call_stub_%d", i),
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": string(argsJSON),
			},
		})
	}
	finish := "stop"
	if len(toolCalls) > 0 {
		finish = "tool_calls"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{
			"message":       map[string]any{"role": "assistant", "content": turn.Text, "tool_calls": toolCalls},
			"finish_reason": finish,
		}},
	})
}

func (p *Provider) writeStream(w http.ResponseWriter, turn Turn, turnIdx int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	write := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	// 文本分片：模拟真实模型的流式吐字（前端/投影走的是真实流式通道）
	for _, chunk := range splitText(turn.Text) {
		write(map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"content": chunk},
		}}})
	}

	// 工具调用：只给"调什么 + 过滤条件"，结果由真实执行产生
	for i, tc := range turn.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Args)
		if string(argsJSON) == "null" {
			argsJSON = []byte("{}")
		}
		write(map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": i,
				"id":    fmt.Sprintf("call_stub_%d_%d", turnIdx, i),
				"type":  "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": string(argsJSON),
				},
			}}},
		}}})
	}

	finish := "stop"
	if len(turn.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	write(map[string]any{"choices": []any{map[string]any{
		"delta":         map[string]any{},
		"finish_reason": finish,
	}}})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// splitText 把文本切成小块，避免一次性吐完（让流式通道真的被用到）。
func splitText(s string) []string {
	if s == "" {
		return nil
	}
	runes := []rune(s)
	out := make([]string, 0, len(runes)/6+1)
	for i := 0; i < len(runes); i += 6 {
		end := i + 6
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
	}
	return out
}
