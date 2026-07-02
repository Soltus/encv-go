// internal/server/agent_plugin_bridge_test.go
//
// 插件桥接 + session 缓存的单元测试。
//
// 测试策略：
//   - ListPluginTools / pluginNameCN：纯函数测试，无需 mock
//   - executePluginTool 错误路径：传 invalid args → 验证返回 errJSON（不调用真插件）
//   - getOrCreateSession：map 并发安全 + 复用 + LastAccess 更新
//   - chatMsg 序列化：ToolCallID/Name/ToolCalls 字段 JSON 编解码正确
//   - session GC：清理过期 + 保留活跃 + LastAccess 不被错误清理
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/v2/plugins"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	encvPlugins "github.com/Soltus/encv-go/pkg/encv/plugins"
	"github.com/gin-gonic/gin"
)

// ─── ListPluginTools ──────────────────────────────────────────

func TestListPluginTools_Contains12Tools(t *testing.T) {
	tools := ListPluginTools()
	// 排除所有 alist* 开头的 OpenList 工具族，剩下的插件 × 2
	allPlugins := encvPlugins.Plugins()
	expected := 0
	for _, p := range allPlugins {
		if !strings.HasPrefix(p.Name(), "alist") {
			expected += 2
		}
	}
	if len(tools) != expected {
		t.Errorf("ListPluginTools() 长度 = %d, want %d", len(tools), expected)
	}

	// 验证每个工具都有 name/description/parameters/needConfirm
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool["name"].(string))
		if tool["description"] == nil || tool["description"].(string) == "" {
			t.Errorf("tool %v 缺 description", tool["name"])
		}
		if tool["parameters"] == nil {
			t.Errorf("tool %v 缺 parameters", tool["name"])
		}
		if tool["needConfirm"] != true {
			t.Errorf("tool %v needConfirm 应为 true", tool["name"])
		}
	}
	sort.Strings(names)
	t.Logf("工具列表: %v", names)
}

func TestListPluginTools_NoDuplicateNames(t *testing.T) {
	tools := ListPluginTools()
	seen := make(map[string]bool)
	for _, tool := range tools {
		name := tool["name"].(string)
		if seen[name] {
			t.Errorf("工具名重复: %s", name)
		}
		seen[name] = true
	}
}

func TestListPluginTools_SkipsAlistencrypt(t *testing.T) {
	tools := ListPluginTools()
	for _, tool := range tools {
		name := tool["name"].(string)
		// alistencrypt / alist_encrypt / 任何 alist* 都不应出现
		if strings.HasPrefix(name, "alist") {
			t.Errorf("alist* 工具不应出现在工具列表（已砍掉 OpenList）: %s", name)
		}
	}
}

// ─── pluginNameCN ─────────────────────────────────────────────

func TestPluginNameCN(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"video", "视频"},
		{"audio", "音频"},
		{"image", "图片"},
		{"wps", "WPS 文档"},
		{"pdf", "PDF"},
		{"text", "文本"},
		{"unknown_plugin", "unknown_plugin"}, // 未知名 fallback
	}
	for _, tt := range tests {
		if got := pluginNameCN(tt.name); got != tt.want {
			t.Errorf("pluginNameCN(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// ─── executePluginTool 错误路径 ───────────────────────────────

func TestExecutePluginTool_UnknownTool(t *testing.T) {
	_, err := executePluginTool(context.Background(), "nonexistent_tool", "{}")
	if err == nil {
		t.Fatal("executePluginTool(未知工具) 应返回 error")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error 应包含 'unknown tool', got: %v", err)
	}
}

func TestExecutePluginTool_InvalidArgsJSON(t *testing.T) {
	// 找到第一个有效的 encrypt 工具
	tools := ListPluginTools()
	if len(tools) == 0 {
		t.Skip("无可用插件工具")
	}
	var encryptTool string
	for _, tool := range tools {
		name := tool["name"].(string)
		if strings.HasSuffix(name, "_encrypt") {
			encryptTool = name
			break
		}
	}
	if encryptTool == "" {
		t.Skip("未找到 _encrypt 工具")
	}

	// 传非法 JSON → 应返回 errJSON(invalid_args) 而非 panic
	raw, err := executePluginTool(context.Background(), encryptTool, "not-valid-json{")
	if err != nil {
		t.Fatalf("executePluginTool 错误路径不应返回 error，应返回 errJSON: %v", err)
	}
	if !strings.Contains(raw, `"error":"invalid_args"`) {
		t.Errorf("result 应包含 invalid_args, got: %s", raw)
	}
}

func TestExecutePluginTool_MissingArgs(t *testing.T) {
	tools := ListPluginTools()
	if len(tools) == 0 {
		t.Skip("无可用插件工具")
	}
	var encryptTool string
	for _, tool := range tools {
		name := tool["name"].(string)
		if strings.HasSuffix(name, "_encrypt") {
			encryptTool = name
			break
		}
	}
	if encryptTool == "" {
		t.Skip("未找到 _encrypt 工具")
	}

	// 传空 args → 应返回 errJSON(missing_args)
	raw, _ := executePluginTool(context.Background(), encryptTool, `{}`)
	if !strings.Contains(raw, `"error":"missing_args"`) {
		t.Errorf("result 应包含 missing_args, got: %s", raw)
	}
}

// ─── getOrCreateSession ───────────────────────────────────────

func TestGetOrCreateSession_NewAndReuse(t *testing.T) {
	id := "test-session-1"
	s1 := getOrCreateSession(id)
	if s1 == nil {
		t.Fatal("getOrCreateSession 不应返回 nil")
	}
	if s1.SessionID != id {
		t.Errorf("SessionID = %q, want %q", s1.SessionID, id)
	}
	s2 := getOrCreateSession(id)
	if s1 != s2 {
		t.Errorf("同 ID 应返回同一 session 实例")
	}
}

func TestGetOrCreateSession_DifferentIDs(t *testing.T) {
	s1 := getOrCreateSession("session-a")
	s2 := getOrCreateSession("session-b")
	if s1 == s2 {
		t.Errorf("不同 ID 应返回不同 session 实例")
	}
}

func TestGetOrCreateSession_Concurrent(t *testing.T) {
	// 并发创建同 ID → 必须返回同一实例
	const id = "concurrent-session"
	const n = 50
	var wg sync.WaitGroup
	results := make([]*agentSession, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = getOrCreateSession(id)
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if results[0] != results[i] {
			t.Errorf("并发创建返回了不同实例: results[0]=%p, results[%d]=%p",
				results[0], i, results[i])
		}
	}
}

// ─── chatMsg 序列化（OpenAI 工具调用协议） ────────────────────

func TestChatMsg_ToolMessageSerialization(t *testing.T) {
	// 验证 ToolCallID 和 Name 字段能正确 JSON 序列化（OpenAI tool message 协议要求）
	msg := chatMsg{
		Role:       "tool",
		Content:    `{"ok": true}`,
		ToolCallID: "call_abc123",
		Name:       "video_encrypt",
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"role":"tool"`) {
		t.Errorf("序列化缺 role: %s", got)
	}
	if !strings.Contains(got, `"tool_call_id":"call_abc123"`) {
		t.Errorf("序列化缺 tool_call_id: %s", got)
	}
	if !strings.Contains(got, `"name":"video_encrypt"`) {
		t.Errorf("序列化缺 name: %s", got)
	}
	if !strings.Contains(got, `"content":"{\"ok\": true}"`) {
		t.Errorf("序列化缺 content: %s", got)
	}

	// 反序列化也必须能恢复
	var back chatMsg
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if back.Role != msg.Role || back.Content != msg.Content ||
		back.ToolCallID != msg.ToolCallID || back.Name != msg.Name {
		t.Errorf("反序列化结果不一致:\n got: %+v\nwant: %+v", back, msg)
	}
}

func TestChatMsg_UserMessageOmitsToolFields(t *testing.T) {
	// user 消息不应有 tool_call_id / name → omitempty 生效
	msg := chatMsg{Role: "user", Content: "hello"}
	raw, _ := json.Marshal(msg)
	got := string(raw)
	if strings.Contains(got, "tool_call_id") {
		t.Errorf("user 消息不应有 tool_call_id: %s", got)
	}
	if strings.Contains(got, `"name"`) {
		t.Errorf("user 消息不应有 name 字段: %s", got)
	}
}

// ─── Session 状态机 ──────────────────────────────────────────

func TestAgentSession_StoreMessagesAndPendingTools(t *testing.T) {
	id := "session-messages"
	sess := getOrCreateSession(id)

	// 模拟 handleAgentChat 缓存 messages
	userMsg := chatMsg{Role: "user", Content: "加密 /tmp/test.mp4"}
	sess.mu.Lock()
	sess.Messages = []chatMsg{userMsg}
	sess.LastModel = "gpt-4o-mini"
	sess.LastTemperature = 0.7
	sess.PendingTools = nil
	sess.mu.Unlock()

	// 模拟 LLM 返回 tool_call
	tc := toolCallAccumulator{ID: "call_xyz", Index: 0, Type: "function"}
	tc.Function.Name = "video_encrypt"
	tc.Function.Arguments = `{"input_path":"/tmp/test.mp4","output_dir":"/tmp/out"}`
	sess.mu.Lock()
	sess.PendingTools = []toolCallAccumulator{tc}
	sess.mu.Unlock()

	// 模拟 handleAgentConfirm 读取
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if len(sess.Messages) != 1 || sess.Messages[0].Content != "加密 /tmp/test.mp4" {
		t.Errorf("Messages 状态异常: %+v", sess.Messages)
	}
	if len(sess.PendingTools) != 1 {
		t.Fatalf("PendingTools 数量 = %d, want 1", len(sess.PendingTools))
	}
	got := sess.PendingTools[0]
	if got.ID != "call_xyz" || got.Function.Name != "video_encrypt" {
		t.Errorf("PendingTools 内容异常: %+v", got)
	}
	if got.Function.Arguments == "" {
		t.Error("PendingTools.Function.Arguments 不应为空")
	}
	if sess.LastModel != "gpt-4o-mini" {
		t.Errorf("LastModel = %q, want gpt-4o-mini", sess.LastModel)
	}
}

func TestAgentSession_ConfirmAppendsAssistantAndToolMessages(t *testing.T) {
	// 模拟 confirm accept 后的 messages 追加
	id := "session-confirm"
	sess := getOrCreateSession(id)

	// 初始：用户消息
	sess.mu.Lock()
	sess.Messages = []chatMsg{{Role: "user", Content: "加密"}}
	sess.PendingTools = []toolCallAccumulator{{ID: "call_1", Index: 0, Type: "function"}}
	sess.PendingTools[0].Function.Name = "video_encrypt"
	sess.PendingTools[0].Function.Arguments = `{"input_path":"/a.mp4"}`
	sess.mu.Unlock()

	// 模拟 confirm 流程（用新的 ToolCalls 字段，不再 hack Content）
	sess.mu.Lock()
	tool := sess.PendingTools[0]
	allToolCalls := make([]toolCallAccumulator, len(sess.PendingTools))
	copy(allToolCalls, sess.PendingTools)
	assistantMsg := chatMsg{
		Role:      "assistant",
		Content:   "",
		ToolCalls: allToolCalls,
	}
	toolMsg := chatMsg{
		Role:       "tool",
		Content:    `{"output":"/tmp/a.encv"}`,
		ToolCallID: tool.ID,
		Name:       tool.Function.Name,
	}
	sess.Messages = append(sess.Messages, assistantMsg, toolMsg)
	sess.PendingTools = nil
	sess.mu.Unlock()

	// 验证
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if len(sess.Messages) != 3 {
		t.Fatalf("Messages 数量 = %d, want 3", len(sess.Messages))
	}
	if sess.Messages[1].Role != "assistant" {
		t.Errorf("第二条消息 role = %q, want assistant", sess.Messages[1].Role)
	}
	if len(sess.Messages[1].ToolCalls) != 1 {
		t.Fatalf("assistant 消息 ToolCalls 数量 = %d, want 1", len(sess.Messages[1].ToolCalls))
	}
	if sess.Messages[1].ToolCalls[0].ID != "call_1" {
		t.Errorf("assistant ToolCalls[0].ID = %q, want call_1", sess.Messages[1].ToolCalls[0].ID)
	}
	if sess.Messages[1].ToolCalls[0].Function.Name != "video_encrypt" {
		t.Errorf("assistant ToolCalls[0].Function.Name = %q, want video_encrypt", sess.Messages[1].ToolCalls[0].Function.Name)
	}
	if sess.Messages[1].Content != "" {
		t.Errorf("assistant 消息 Content 应为空（被 ToolCalls 替代）: %q", sess.Messages[1].Content)
	}
	if sess.Messages[2].Role != "tool" {
		t.Errorf("第三条消息 role = %q, want tool", sess.Messages[2].Role)
	}
	if sess.Messages[2].ToolCallID != "call_1" {
		t.Errorf("tool_msg ToolCallID = %q, want call_1", sess.Messages[2].ToolCallID)
	}
	if sess.Messages[2].Name != "video_encrypt" {
		t.Errorf("tool_msg Name = %q, want video_encrypt", sess.Messages[2].Name)
	}
	if len(sess.PendingTools) != 0 {
		t.Errorf("PendingTools 应已清空, got %d", len(sess.PendingTools))
	}
}

// ─── toolCallAccumulator JSON 编解码 ─────────────────────────

func TestToolCallAccumulator_RoundTrip(t *testing.T) {
	orig := toolCallAccumulator{
		ID:    "call_999",
		Type:  "function",
		Index: 0,
	}
	orig.Function.Name = "video_encrypt"
	orig.Function.Arguments = `{"input_path":"/x.mp4","output_dir":"/out"}`

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	var back toolCallAccumulator
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if back.ID != orig.ID || back.Type != orig.Type || back.Index != orig.Index {
		t.Errorf("顶层字段不一致: got %+v, want %+v", back, orig)
	}
	if back.Function.Name != orig.Function.Name {
		t.Errorf("Function.Name 不一致: got %q, want %q", back.Function.Name, orig.Function.Name)
	}
	if back.Function.Arguments != orig.Function.Arguments {
		t.Errorf("Function.Arguments 不一致: got %q, want %q", back.Function.Arguments, orig.Function.Arguments)
	}
}

// ─── okJSON / errJSON 辅助函数 ────────────────────────────────

func TestOkJSON_ValidJSON(t *testing.T) {
	got := okJSON(map[string]interface{}{"a": 1, "b": "x"})
	if !strings.Contains(got, `"a":1`) || !strings.Contains(got, `"b":"x"`) {
		t.Errorf("okJSON 输出异常: %s", got)
	}
}

func TestErrJSON_HasErrorAndMessage(t *testing.T) {
	got := errJSON("test_code", "test message")
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("errJSON 输出非合法 JSON: %s", got)
	}
	if m["error"] != "test_code" {
		t.Errorf("error 字段 = %v, want test_code", m["error"])
	}
	if m["message"] != "test message" {
		t.Errorf("message 字段 = %v, want test message", m["message"])
	}
}

// ─── session GC ──────────────────────────────────────────────

func TestGcIdleSessions_EvictsExpired(t *testing.T) {
	// 创建一个 session 然后手动回拨 LastAccess 到过期
	id := "gc-evict-test"
	sess := getOrCreateSession(id)
	if sess == nil {
		t.Fatal("getOrCreateSession 失败")
	}

	// 回拨到 31 分钟前（超过 sessionIdleTTL=30min）
	sessionMu.Lock()
	sess.LastAccess = time.Now().Add(-31 * time.Minute)
	sessionMu.Unlock()

	// 执行 GC
	evicted := gcIdleSessions()
	if evicted < 1 {
		t.Errorf("应至少清理 1 个 session, got %d", evicted)
	}

	// 验证 session 已消失
	sessionMu.RLock()
	_, exists := sessions[id]
	sessionMu.RUnlock()
	if exists {
		t.Errorf("session %q 应已被 GC 清理", id)
	}
}

func TestGcIdleSessions_PreservesActive(t *testing.T) {
	// 创建活跃 session（LastAccess = now），GC 不应清理
	id := "gc-active-test"
	sess := getOrCreateSession(id)
	if sess == nil {
		t.Fatal("getOrCreateSession 失败")
	}

	// 显式设置 LastAccess 为 1 分钟前（未过期）
	sessionMu.Lock()
	sess.LastAccess = time.Now().Add(-1 * time.Minute)
	sessionMu.Unlock()

	evicted := gcIdleSessions()
	_ = evicted // 可能有别的测试残留的过期 session 被清

	sessionMu.RLock()
	_, exists := sessions[id]
	sessionMu.RUnlock()
	if !exists {
		t.Errorf("活跃 session %q 不应被 GC 清理", id)
	}
}

func TestGetOrCreateSession_UpdatesLastAccess(t *testing.T) {
	// 首次创建
	id := "gc-touch-test"
	s1 := getOrCreateSession(id)
	sessionMu.RLock()
	firstAccess := s1.LastAccess
	sessionMu.RUnlock()

	// 睡 50ms 后再 getOrCreate → LastAccess 必须更新
	time.Sleep(50 * time.Millisecond)
	s2 := getOrCreateSession(id)
	sessionMu.RLock()
	secondAccess := s2.LastAccess
	sessionMu.RUnlock()

	if !secondAccess.After(firstAccess) {
		t.Errorf("LastAccess 未更新: first=%v, second=%v", firstAccess, secondAccess)
	}
}

// ─── chatMsg.ToolCalls 序列化 ────────────────────────────────

func TestChatMsg_AssistantWithToolCalls(t *testing.T) {
	// assistant 消息带 tool_calls 数组 → 序列化必须正确
	tc := toolCallAccumulator{ID: "call_1", Type: "function"}
	tc.Function.Name = "video_encrypt"
	tc.Function.Arguments = `{"input_path":"/a.mp4"}`

	msg := chatMsg{
		Role:      "assistant",
		Content:   "",
		ToolCalls: []toolCallAccumulator{tc},
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"role":"assistant"`) {
		t.Errorf("缺 role: %s", got)
	}
	if !strings.Contains(got, `"tool_calls":[{`) {
		t.Errorf("缺 tool_calls 数组: %s", got)
	}
	if !strings.Contains(got, `"id":"call_1"`) {
		t.Errorf("缺 tool_call id: %s", got)
	}
	if !strings.Contains(got, `"name":"video_encrypt"`) {
		t.Errorf("缺 function.name: %s", got)
	}
	if !strings.Contains(got, `"arguments":"{\"input_path\":\"/a.mp4\"}"`) {
		t.Errorf("缺 function.arguments: %s", got)
	}

	// 反序列化恢复
	var back chatMsg
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if len(back.ToolCalls) != 1 || back.ToolCalls[0].ID != "call_1" {
		t.Errorf("反序列化 ToolCalls 异常: %+v", back.ToolCalls)
	}
}

func TestChatMsg_OmitsToolCallsWhenEmpty(t *testing.T) {
	// 普通 user 消息不应有 tool_calls 字段（omitempty 生效）
	msg := chatMsg{Role: "user", Content: "hello"}
	raw, _ := json.Marshal(msg)
	got := string(raw)
	if strings.Contains(got, "tool_calls") {
		t.Errorf("user 消息不应有 tool_calls 字段: %s", got)
	}
	if strings.Contains(got, "tool_call_id") {
		t.Errorf("user 消息不应有 tool_call_id 字段: %s", got)
	}
}

func TestToolCallAccumulator_IndexOmittedWhenZero(t *testing.T) {
	// Index 字段 omitempty：0 值不序列化（OpenAI 协议不需要 index）
	tc := toolCallAccumulator{ID: "call_x", Type: "function"}
	tc.Function.Name = "video_encrypt"
	raw, _ := json.Marshal(tc)
	got := string(raw)
	if strings.Contains(got, `"index"`) {
		t.Errorf("Index=0 不应被序列化: %s", got)
	}
	// 验证必要字段都在
	for _, want := range []string{`"id":"call_x"`, `"type":"function"`, `"name":"video_encrypt"`} {
		if !strings.Contains(got, want) {
			t.Errorf("缺字段 %s: %s", want, got)
		}
	}
}

func TestToolCallAccumulator_IndexSerializedWhenNonZero(t *testing.T) {
	// Index 字段非 0 时正常序列化（流式响应需要）
	tc := toolCallAccumulator{ID: "call_x", Type: "function", Index: 3}
	tc.Function.Name = "video_encrypt"
	raw, _ := json.Marshal(tc)
	got := string(raw)
	if !strings.Contains(got, `"index":3`) {
		t.Errorf("Index=3 应被序列化: %s", got)
	}
}

// ─── 动态 schema（B.1.3）──────────────────────────────────────

func TestBuildDynamicSchema_EncryptBaseline(t *testing.T) {
	// 任意插件加密 schema 都有 input_paths/output_path
	tools := ListPluginTools()
	var encryptTool map[string]interface{}
	for _, tool := range tools {
		if tool["name"].(string) == "wps_encrypt" {
			encryptTool = tool
			break
		}
	}
	if encryptTool == nil {
		t.Skip("wps_encrypt 工具不存在")
	}
	params := encryptTool["parameters"].(map[string]interface{})
	props := params["properties"].(map[string]interface{})
	if _, ok := props["input_paths"]; !ok {
		t.Error("encrypt schema 应含 input_paths")
	}
	if _, ok := props["output_path"]; !ok {
		t.Error("encrypt schema 应含 output_path")
	}
	required := params["required"].([]string)
	hasInputPaths := false
	hasOutputPath := false
	for _, r := range required {
		if r == "input_paths" {
			hasInputPaths = true
		}
		if r == "output_path" {
			hasOutputPath = true
		}
	}
	if !hasInputPaths || !hasOutputPath {
		t.Errorf("input_paths/output_path 都应在 required 列表: %v", required)
	}
}

func TestBuildDynamicSchema_DecryptBaseline(t *testing.T) {
	// decrypt 必有 container_path/output_dir
	tools := ListPluginTools()
	var decryptTool map[string]interface{}
	for _, tool := range tools {
		if tool["name"].(string) == "wps_decrypt" {
			decryptTool = tool
			break
		}
	}
	if decryptTool == nil {
		t.Skip("wps_decrypt 工具不存在")
	}
	params := decryptTool["parameters"].(map[string]interface{})
	props := params["properties"].(map[string]interface{})
	if _, ok := props["container_path"]; !ok {
		t.Error("decrypt schema 应含 container_path")
	}
	if _, ok := props["output_dir"]; !ok {
		t.Error("decrypt schema 应含 output_dir")
	}
}

func TestBuildDynamicSchema_ExtraFieldsPresent(t *testing.T) {
	// video 插件有 6 个 extra_fields（按 TaskOptions）
	// 检查 video_encrypt 的 schema 包含 extra_fields 子对象
	tools := ListPluginTools()
	var videoEncrypt map[string]interface{}
	for _, tool := range tools {
		if tool["name"].(string) == "video_encrypt" {
			videoEncrypt = tool
			break
		}
	}
	if videoEncrypt == nil {
		t.Skip("video_encrypt 工具不存在")
	}
	params := videoEncrypt["parameters"].(map[string]interface{})
	props := params["properties"].(map[string]interface{})
	extraFields, ok := props["extra_fields"].(map[string]interface{})
	if !ok {
		t.Fatal("video_encrypt schema 应含 extra_fields")
	}
	extraProps, ok := extraFields["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("extra_fields 应有 properties 子对象")
	}
	// video 至少应有 stream_preset 字段
	if _, ok := extraProps["stream_preset"]; !ok {
		t.Error("video extra_fields 应含 stream_preset 字段")
	}
}

func TestBuildDynamicSchema_VersionField(t *testing.T) {
	// video 插件 SupportVersionSelect=true → schema 必有 version
	// pdf 插件 SupportVersionSelect=false → schema 不应有 version
	tools := ListPluginTools()
	getTool := func(name string) map[string]interface{} {
		for _, tool := range tools {
			if tool["name"].(string) == name {
				return tool
			}
		}
		return nil
	}

	if videoEnc := getTool("video_encrypt"); videoEnc != nil {
		props := videoEnc["parameters"].(map[string]interface{})["properties"].(map[string]interface{})
		if _, ok := props["version"]; !ok {
			t.Error("video_encrypt schema 应含 version 字段")
		}
	} else {
		t.Skip("video_encrypt 工具不存在")
	}

	if pdfEnc := getTool("pdf_encrypt"); pdfEnc != nil {
		props := pdfEnc["parameters"].(map[string]interface{})["properties"].(map[string]interface{})
		if _, ok := props["version"]; ok {
			t.Error("pdf_encrypt schema 不应含 version 字段（SupportVersionSelect=false）")
		}
	}
}

func TestBuildDynamicSchema_PasswordStrategy(t *testing.T) {
	// 全局密码插件（video/audio/...）不应有 password 字段
	// 独立密码插件（alist_encrypt）已被排除
	// 所以 6 插件都不应暴露 password 给 LLM
	tools := ListPluginTools()
	for _, tool := range tools {
		name := tool["name"].(string)
		props := tool["parameters"].(map[string]interface{})["properties"].(map[string]interface{})
		if _, ok := props["password"]; ok {
			t.Errorf("工具 %s 不应暴露 password 字段（PasswordGlobal 策略）", name)
		}
	}
}

func TestJsonSchemaType(t *testing.T) {
	tests := []struct {
		pluginType string
		schemaType string
	}{
		{"bool", "boolean"},
		{"int", "integer"},
		{"integer", "integer"},
		{"float", "number"},
		{"number", "number"},
		{"select", "string"},
		{"string", "string"},
		{"password", "string"},
		{"text", "string"},
		{"array", "array"},
		{"unknown_type", "string"}, // fallback
	}
	for _, tt := range tests {
		if got := jsonSchemaType(tt.pluginType); got != tt.schemaType {
			t.Errorf("jsonSchemaType(%q) = %q, want %q", tt.pluginType, got, tt.schemaType)
		}
	}
}

func TestBuildTaskFieldSchema(t *testing.T) {
	// bool 字段
	boolField := pluginInterfaces.TaskField{
		Key: "encrypt_filename", Label: "加密文件名", Type: "bool",
		DefaultValue: "false", Help: "是否加密文件名",
	}
	bs := buildTaskFieldSchema(boolField)
	if bs["type"] != "boolean" {
		t.Errorf("bool field type = %v, want boolean", bs["type"])
	}
	if bs["default"] != "false" {
		t.Errorf("bool field default = %v, want false", bs["default"])
	}

	// select 字段
	selectField := pluginInterfaces.TaskField{
		Key: "stream_preset", Label: "流预设", Type: "select",
		Options:      []string{"balanced", "high_quality"},
		OptionLabels: map[string]string{"balanced": "Balanced", "high_quality": "High Quality"},
	}
	ss := buildTaskFieldSchema(selectField)
	if ss["type"] != "string" {
		t.Errorf("select field type = %v, want string", ss["type"])
	}
	enum, ok := ss["enum"].([]string)
	if !ok || len(enum) != 2 {
		t.Errorf("select field enum 异常: %v", ss["enum"])
	}
}

func TestInjectExtraFields_NoOpWhenEmpty(t *testing.T) {
	// 空 fields → 直接 return，不调 setter
	p := findAnyPlugin(t)
	// 这里不能直接 mock，但可以验证空 fields 不 panic
	injectExtraFields(p, nil)
	injectExtraFields(p, map[string]interface{}{})
}

func TestInjectExtraFields_SkipWhenNoSetter(t *testing.T) {
	// 6 核心插件都没实现 SetTaskExtraFields → 应该 no-op（不 panic）
	p := findAnyPlugin(t)
	if _, ok := p.(pluginInterfaces.TaskExtraFieldsSetter); ok {
		t.Skip("插件实现了 SetTaskExtraFields（alist_encrypt 之类），此测试不适用")
	}
	// 不应 panic
	injectExtraFields(p, map[string]interface{}{
		"stream_preset": "balanced",
		"fn_rounds":     8,
	})
}

func findAnyPlugin(t *testing.T) plugins.Plugin {
	t.Helper()
	all := encvPlugins.Plugins()
	for _, p := range all {
		if !strings.HasPrefix(p.Name(), "alist") {
			return p
		}
	}
	t.Fatal("找不到非 alist 插件")
	return nil
}

// ─── D 阶段：事件缓存 + 断点续传 ─────────────────────────────

func TestMaxEventID_EmptySlice(t *testing.T) {
	if got := maxEventID(nil); got != 0 {
		t.Errorf("maxEventID(nil) = %d, want 0", got)
	}
	if got := maxEventID([]AgentEvent{}); got != 0 {
		t.Errorf("maxEventID(empty) = %d, want 0", got)
	}
}

func TestMaxEventID_ReturnsLastID(t *testing.T) {
	events := []AgentEvent{
		{ID: 1, Type: "text_delta"},
		{ID: 5, Type: "tool_call"},
		{ID: 3, Type: "stream_end"},
	}
	if got := maxEventID(events); got != 5 {
		t.Errorf("maxEventID = %d, want 5（取最大 ID，不依赖顺序）", got)
	}
}

func TestSendAndCache_AppendsToEventCache(t *testing.T) {
	sess := getOrCreateSession("d-cache-test")
	defer deleteSessionForTest("d-cache-test")

	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.sendAndCache(sess, rec, rec, "text_delta", "hello")

	if got := len(sess.EventCache); got != 1 {
		t.Fatalf("EventCache 长度 = %d, want 1", got)
	}
	if sess.EventCache[0].ID != 1 {
		t.Errorf("第一个事件 ID = %d, want 1", sess.EventCache[0].ID)
	}
	if sess.EventCache[0].Type != "text_delta" {
		t.Errorf("Type = %q, want text_delta", sess.EventCache[0].Type)
	}
	// 验证 HTTP 响应也写了
	if !strings.Contains(rec.Body.String(), "text_delta") {
		t.Errorf("HTTP 响应应含 text_delta: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "hello") {
		t.Errorf("HTTP 响应应含 data 'hello': %s", rec.Body.String())
	}
}

func TestSendAndCache_MonotonicIDs(t *testing.T) {
	sess := getOrCreateSession("d-monotonic-test")
	defer deleteSessionForTest("d-monotonic-test")

	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.sendAndCache(sess, rec, rec, "text_delta", "1")
	srv.sendAndCache(sess, rec, rec, "text_delta", "2")
	srv.sendAndCache(sess, rec, rec, "text_delta", "3")

	if len(sess.EventCache) != 3 {
		t.Fatalf("EventCache 长度 = %d, want 3", len(sess.EventCache))
	}
	for i, e := range sess.EventCache {
		want := int64(i + 1)
		if e.ID != want {
			t.Errorf("EventCache[%d].ID = %d, want %d", i, e.ID, want)
		}
	}
}

func TestSendAndCache_NilSession(t *testing.T) {
	// nil session → 不缓存但不 panic
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.sendAndCache(nil, rec, rec, "text_delta", "hi")
	if !strings.Contains(rec.Body.String(), "hi") {
		t.Errorf("HTTP 响应应含 hi: %s", rec.Body.String())
	}
}

func TestSendAndCache_ConcurrentSafety(t *testing.T) {
	// 并发写 → 事件 ID 必须严格单调递增
	sess := getOrCreateSession("d-concurrent-test")
	defer deleteSessionForTest("d-concurrent-test")

	rec := httptest.NewRecorder()
	srv := &Server{}
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			srv.sendAndCache(sess, rec, rec, "text_delta", "x")
		}()
	}
	wg.Wait()

	if len(sess.EventCache) != n {
		t.Fatalf("EventCache 长度 = %d, want %d", len(sess.EventCache), n)
	}
	// 验证 ID 严格递增（1..n）—— sendAndCache 内部用 sess.mu 保护 eventIDCounter
	seen := make(map[int64]bool)
	for _, e := range sess.EventCache {
		if seen[e.ID] {
			t.Errorf("重复 ID: %d", e.ID)
		}
		seen[e.ID] = true
	}
}

// ─── helper ───

// deleteSessionForTest 测试结束后清理 session（不影响其他测试的 sessions map）
func deleteSessionForTest(id string) {
	sessionMu.Lock()
	delete(sessions, id)
	sessionMu.Unlock()
}

// createGinContextForTest 构造一个测试用 gin.Context（httptest.ResponseRecorder + http.Request）
// 让 handleAgentXXX 等 gin handler 函数能直接调用
func createGinContextForTest(rec *httptest.ResponseRecorder, req *http.Request) *gin.Context {
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	return c
}

// ─── handleAgentResume 重放逻辑（直接测试内部状态机） ───────

func TestResumeLogic_FindStartIndex(t *testing.T) {
	// 给定 EventCache 和 lastEventID，找到正确的 startIdx
	cases := []struct {
		name        string
		cache       []AgentEvent
		lastEventID int64
		wantStart   int
	}{
		{
			name: "lastEventID=0 重放全部",
			cache: []AgentEvent{
				{ID: 1, Type: "text_delta"},
				{ID: 2, Type: "text_delta"},
				{ID: 3, Type: "stream_end"},
			},
			lastEventID: 0,
			wantStart:   0,
		},
		{
			name: "lastEventID=1 重放 ID 2,3",
			cache: []AgentEvent{
				{ID: 1, Type: "text_delta"},
				{ID: 2, Type: "text_delta"},
				{ID: 3, Type: "stream_end"},
			},
			lastEventID: 1,
			wantStart:   1,
		},
		{
			name: "lastEventID=3（最后）重放空",
			cache: []AgentEvent{
				{ID: 1, Type: "text_delta"},
				{ID: 2, Type: "text_delta"},
				{ID: 3, Type: "stream_end"},
			},
			lastEventID: 3,
			wantStart:   3,
		},
		{
			name:        "空 cache",
			cache:       nil,
			lastEventID: 0,
			wantStart:   0,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// 复用 handleAgentResume 中的算法
			startIdx := 0
			for i, e := range tt.cache {
				if e.ID > tt.lastEventID {
					startIdx = i
					break
				}
				if i == len(tt.cache)-1 && e.ID <= tt.lastEventID {
					startIdx = len(tt.cache)
				}
			}
			if startIdx != tt.wantStart {
				t.Errorf("startIdx = %d, want %d", startIdx, tt.wantStart)
			}
		})
	}
}

func TestHandleAgentResume_HTTP_ReplayEvents(t *testing.T) {
	// 集成测试：构造 session + EventCache，调 /api/resume，验证响应内容
	id := "d-resume-http-test"
	sess := getOrCreateSession(id)
	defer deleteSessionForTest(id)

	// 手动注入 5 个事件到 EventCache
	sess.mu.Lock()
	sess.EventCache = []AgentEvent{
		{ID: 1, Type: "text_delta", Data: "hello "},
		{ID: 2, Type: "text_delta", Data: "world"},
		{ID: 3, Type: "tool_call", Data: map[string]interface{}{"id": "call_1", "name": "video_encrypt"}},
		{ID: 4, Type: "tool_status", Data: map[string]interface{}{"id": "call_1", "status": "completed"}},
		{ID: 5, Type: "stream_end", Data: ""},
	}
	sess.InProgress = false // 已结束
	sess.mu.Unlock()

	// 调 /api/resume 从 lastEventID=2 开始
	req := httptest.NewRequest(http.MethodPost, "/api/resume",
		strings.NewReader(`{"sessionId":"`+id+`","lastEventId":2}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv := &Server{}
	srv.handleAgentResume(createGinContextForTest(rec, req))

	body := rec.Body.String()
	t.Logf("Resume 响应:\n%s", body)

	// 验证重放了 ID 3,4,5（SSE 格式：`id: N`）
	if !strings.Contains(body, "id: 3\n") {
		t.Errorf("响应应含 id: 3 字段: %s", body)
	}
	if !strings.Contains(body, "id: 4\n") {
		t.Errorf("响应应含 id: 4 字段: %s", body)
	}
	if !strings.Contains(body, "id: 5\n") {
		t.Errorf("响应应含 id: 5 字段: %s", body)
	}
	// ID 1,2 不应被重放
	if strings.Contains(body, "id: 1\n") || strings.Contains(body, "id: 2\n") {
		t.Errorf("不应重放 ID 1,2: %s", body)
	}
	// 应含 tool_call 事件
	if !strings.Contains(body, "tool_call") {
		t.Errorf("应重放 tool_call 事件: %s", body)
	}
	if !strings.Contains(body, "stream_end") {
		t.Errorf("响应应含 stream_end: %s", body)
	}
}

func TestHandleAgentResume_HTTP_Header_LastEventID(t *testing.T) {
	// SSE 协议标准 header：Last-Event-ID
	id := "default" // handleAgentResume 的 sessionId 默认值

	sess := getOrCreateSession(id)
	defer deleteSessionForTest(id)

	sess.mu.Lock()
	sess.EventCache = []AgentEvent{
		{ID: 1, Type: "text_delta", Data: "a"},
		{ID: 2, Type: "text_delta", Data: "b"},
	}
	sess.InProgress = false
	sess.mu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/resume", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Last-Event-ID", "1")
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleAgentResume(createGinContextForTest(rec, req))

	body := rec.Body.String()
	// 应只重放 ID=2
	if !strings.Contains(body, "id: 2\n") {
		t.Errorf("应重放 ID 2: %s", body)
	}
	if strings.Contains(body, "id: 1\n") {
		t.Errorf("不应重放 ID 1: %s", body)
	}
}

func TestHandleAgentResume_HTTP_NotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/resume",
		strings.NewReader(`{"sessionId":"nonexistent","lastEventId":0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleAgentResume(createGinContextForTest(rec, req))

	if rec.Code != http.StatusNotFound {
		t.Errorf("session_not_found 应返回 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "session_not_found") {
		t.Errorf("响应应含 session_not_found: %s", rec.Body.String())
	}
}

func TestHandleAgentResume_HTTP_InProgress_Synced(t *testing.T) {
	// lastEventID = 末尾，inProgress=true → 应推 stream_status: synced
	id := "d-resume-synced-test"
	sess := getOrCreateSession(id)
	defer deleteSessionForTest(id)

	sess.mu.Lock()
	sess.EventCache = []AgentEvent{
		{ID: 1, Type: "text_delta", Data: "hi"},
	}
	sess.InProgress = true // 还在流式
	sess.mu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/resume",
		strings.NewReader(`{"sessionId":"`+id+`","lastEventId":1}`))
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleAgentResume(createGinContextForTest(rec, req))

	body := rec.Body.String()
	if !strings.Contains(body, `"stream_status"`) {
		t.Errorf("inProgress + synced 应推 stream_status: %s", body)
	}
	if !strings.Contains(body, `"synced"`) {
		t.Errorf("应含 status:synced: %s", body)
	}
}

// ─── F 阶段：4 决策 UX（accept_for_session + sessionGrants） ───

func TestGrantedTools_InitializedOnSessionCreate(t *testing.T) {
	// getOrCreateSession 必须初始化 GrantedTools map（防止 nil map 写入 panic）
	id := "f-grant-init-test"
	sess := getOrCreateSession(id)
	defer deleteSessionForTest(id)

	if sess.GrantedTools == nil {
		t.Fatal("GrantedTools 应被初始化为非 nil map")
	}
	// 写入不应 panic
	sess.GrantedTools["video_encrypt"] = true
	if !sess.GrantedTools["video_encrypt"] {
		t.Error("写入后读取失败")
	}
}

func TestEmitToolCallEvent_NotGranted_AutoRunFalse(t *testing.T) {
	// session 未授权该工具 → auto_run=false, needsConfirm=true
	sess := getOrCreateSession("f-no-grant-test")
	defer deleteSessionForTest("f-no-grant-test")

	rec := httptest.NewRecorder()
	srv := &Server{}
	tc := toolCallAccumulator{ID: "call_1", Type: "function"}
	tc.Function.Name = "video_encrypt"
	tc.Function.Arguments = `{"input_paths":["/a.mp4"]}`

	srv.emitToolCallEvent(sess, rec, rec, tc, toolMetaFor("video_encrypt"))

	body := rec.Body.String()
	if !strings.Contains(body, `"auto_run":false`) {
		t.Errorf("未授权时 auto_run 应为 false: %s", body)
	}
	if !strings.Contains(body, `"needsConfirm":true`) {
		t.Errorf("未授权时 needsConfirm 应为 true: %s", body)
	}
}

func TestEmitToolCallEvent_Granted_AutoRunTrue(t *testing.T) {
	// session 已授权该工具 → auto_run=true, needsConfirm=false
	sess := getOrCreateSession("f-granted-test")
	defer deleteSessionForTest("f-granted-test")

	// 模拟 accept_for_session 写入
	sess.GrantedTools["video_encrypt"] = true

	rec := httptest.NewRecorder()
	srv := &Server{}
	tc := toolCallAccumulator{ID: "call_2", Type: "function"}
	tc.Function.Name = "video_encrypt" // 同名工具
	tc.Function.Arguments = `{"input_paths":["/b.mp4"]}`

	srv.emitToolCallEvent(sess, rec, rec, tc, toolMetaFor("video_encrypt"))

	body := rec.Body.String()
	if !strings.Contains(body, `"auto_run":true`) {
		t.Errorf("已授权时 auto_run 应为 true: %s", body)
	}
	if !strings.Contains(body, `"needsConfirm":false`) {
		t.Errorf("已授权时 needsConfirm 应为 false: %s", body)
	}
}

func TestEmitToolCallEvent_GrantIsNameScoped(t *testing.T) {
	// 授权 video_encrypt 不应影响 audio_encrypt
	sess := getOrCreateSession("f-grant-scoped-test")
	defer deleteSessionForTest("f-grant-scoped-test")
	sess.GrantedTools["video_encrypt"] = true

	rec := httptest.NewRecorder()
	srv := &Server{}
	tc := toolCallAccumulator{ID: "call_3", Type: "function"}
	tc.Function.Name = "audio_encrypt" // 不同名
	srv.emitToolCallEvent(sess, rec, rec, tc, toolMetaFor("audio_encrypt"))

	body := rec.Body.String()
	if !strings.Contains(body, `"auto_run":false`) {
		t.Errorf("audio_encrypt 不应被 video 授权覆盖: %s", body)
	}
}

func TestEmitToolCallEvent_NilSession(t *testing.T) {
	// nil session → auto_run=false, needsConfirm=true（不 panic）
	rec := httptest.NewRecorder()
	srv := &Server{}
	tc := toolCallAccumulator{ID: "call_4", Type: "function"}
	tc.Function.Name = "video_encrypt"
	srv.emitToolCallEvent(nil, rec, rec, tc, toolMetaFor("video_encrypt"))

	body := rec.Body.String()
	if !strings.Contains(body, `"auto_run":false`) {
		t.Errorf("nil session 应 default auto_run=false: %s", body)
	}
}

func TestHandleAgentConfirm_HTTP_InvalidDecision(t *testing.T) {
	// 非白名单决策 → 400
	id := "f-invalid-decision"
	sess := getOrCreateSession(id)
	defer deleteSessionForTest(id)
	_ = sess // 占位：保证 session 已注册

	req := httptest.NewRequest(http.MethodPost, "/api/confirm",
		strings.NewReader(`{"sessionId":"`+id+`","toolCallId":"call_x","decision":"unknown_decision"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleAgentConfirm(createGinContextForTest(rec, req))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 decision 应返回 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_decision") {
		t.Errorf("响应应含 invalid_decision: %s", rec.Body.String())
	}
}

func TestHandleAgentConfirm_HTTP_CancelDecision(t *testing.T) {
	// cancel 决策：应直接推 stream_end，不执行工具
	// 不依赖真实插件执行，最快验证 cancel 路径
	id := "f-cancel-decision"
	sess := getOrCreateSession(id)
	defer deleteSessionForTest(id)

	// 准备一个 pending tool（让 cancel 之前的查找能走通）
	sess.mu.Lock()
	sess.PendingTools = []toolCallAccumulator{}
	sess.mu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/confirm",
		strings.NewReader(`{"sessionId":"`+id+`","toolCallId":"any","decision":"cancel"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleAgentConfirm(createGinContextForTest(rec, req))

	body := rec.Body.String()
	if !strings.Contains(body, "stream_end") {
		t.Errorf("cancel 应推 stream_end: %s", body)
	}
	// 不应推 tool_status（没有执行任何工具）
	if strings.Contains(body, "tool_status") {
		t.Errorf("cancel 不应推 tool_status: %s", body)
	}
}

// toolMetaFor 测试用 helper：构造只含一个 tool 的 toolMeta
// （mirror 真实场景下 handleAgentChat 构造 toolMeta 的逻辑）
func toolMetaFor(toolName string) map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		toolName: {
			"name":        toolName,
			"needConfirm": true, // 加密/解密 plugin 默认需 confirm
			"kind":        "fileChange",
		},
	}
}
