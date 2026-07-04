// internal/server/agent_context_usage_test.go
//
// /api/agent/context-usage 端点测试
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// makeToolCall 帮助函数：构造一个含指定 name/arguments 的 toolCallAccumulator
func makeToolCall(name, args string) toolCallAccumulator {
	tc := toolCallAccumulator{ID: "tc-1", Type: "function"}
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}

// ─── Token 估算 ──────────────────────────────────────────────

func TestEstimateStringTokens_Empty(t *testing.T) {
	if got := estimateStringTokens(""); got != 0 {
		t.Errorf("空字符串 token = %d, want 0", got)
	}
}

func TestEstimateStringTokens_ASCII(t *testing.T) {
	// 4 ASCII chars ≈ 1 token
	got := estimateStringTokens("test")
	if got <= 0 || got > 2 {
		t.Errorf("'test' (4 chars) token = %d, want ~1", got)
	}
}

func TestEstimateStringTokens_CJK(t *testing.T) {
	// 6 CJK chars ≈ 4 tokens (6 / 1.5)
	got := estimateStringTokens("你好世界啊")
	if got < 3 || got > 5 {
		t.Errorf("6 CJK chars token = %d, want ~4", got)
	}
}

func TestEstimateStringTokens_Mixed(t *testing.T) {
	// "hi 你好" = 2 ASCII + 1 space + 2 CJK ≈ 0.5 + 1.3 ≈ 2 tokens
	got := estimateStringTokens("hi 你好")
	if got < 1 || got > 4 {
		t.Errorf("'hi 你好' token = %d, want ~2", got)
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	if got := estimateTokens(nil); got != 0 {
		t.Errorf("空 messages token = %d, want 0", got)
	}
}

func TestEstimateTokens_AccountsForContent(t *testing.T) {
	msgs := []chatMsg{
		{Role: "user", Content: "hello world"},
		{Role: "assistant", Content: "你好世界"},
	}
	got := estimateTokens(msgs)
	// 至少 4 (role1) + 3 (hello world) + 4 (role2) + 3 (你好世界) = ~12
	if got < 8 {
		t.Errorf("token = %d, want >= 8", got)
	}
}

func TestEstimateTokens_AccountsForToolCalls(t *testing.T) {
	msgs := []chatMsg{
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("read_file", `{"path":"/foo"}`),
		}},
	}
	got := estimateTokens(msgs)
	if got < 4 {
		t.Errorf("token 应包含 tool call args, got %d", got)
	}
}

// ─── 模型 context window 查表 ───────────────────────────────

func TestLookupContextWindow_KnownModels(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-4o", 128000},
		{"gpt-4-turbo", 128000},
		{"gpt-3.5-turbo", 16385},
		{"claude-3-5-sonnet", 200000},
		{"deepseek-chat", 64000},
		{"qwen-plus", 131072},
		{"glm-4-plus", 128000},
		{"o1", 200000},
	}
	for _, tt := range tests {
		if got := lookupContextWindow(tt.model); got != tt.want {
			t.Errorf("lookupContextWindow(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestLookupContextWindow_HeuristicMatch(t *testing.T) {
	if got := lookupContextWindow("custom-128k-model"); got != 128000 {
		t.Errorf("heuristic 128k = %d, want 128000", got)
	}
	if got := lookupContextWindow("custom-32k"); got != 32000 {
		t.Errorf("heuristic 32k = %d, want 32000", got)
	}
	if got := lookupContextWindow("custom-1m-model"); got != 1000000 {
		t.Errorf("heuristic 1m = %d, want 1000000", got)
	}
}

func TestLookupContextWindow_Default(t *testing.T) {
	if got := lookupContextWindow("unknown-model-xyz"); got != 8192 {
		t.Errorf("default = %d, want 8192", got)
	}
	if got := lookupContextWindow(""); got != 8192 {
		t.Errorf("empty model = %d, want 8192", got)
	}
}

// ─── Todos 抽取 ──────────────────────────────────────────────

func TestParseTodosJSON_Array(t *testing.T) {
	js := `[{"content":"task 1","status":"in_progress"},{"content":"task 2","status":"completed"}]`
	todos := parseTodosJSON(js)
	if len(todos) != 2 {
		t.Fatalf("todos 数 = %d, want 2", len(todos))
	}
	if todos[0].Content != "task 1" || todos[0].Status != "in_progress" {
		t.Errorf("todos[0] = %+v, want {task 1, in_progress}", todos[0])
	}
}

func TestParseTodosJSON_Wrapped(t *testing.T) {
	js := `{"todos":[{"content":"x","status":"pending"}]}`
	todos := parseTodosJSON(js)
	if len(todos) != 1 {
		t.Fatalf("wrapped todos 数 = %d, want 1", len(todos))
	}
}

func TestParseTodosJSON_Empty(t *testing.T) {
	if got := parseTodosJSON(""); got != nil {
		t.Errorf("空应返回 nil, got %v", got)
	}
	if got := parseTodosJSON("not-json"); got != nil {
		t.Errorf("非法 JSON 应返回 nil, got %v", got)
	}
	if got := parseTodosJSON("[]"); got != nil {
		t.Errorf("空数组应返回 nil, got %v", got)
	}
}

func TestParseTodosJSON_ActiveFormFallback(t *testing.T) {
	js := `[{"active_form":"do something","status":"in_progress"}]`
	todos := parseTodosJSON(js)
	if len(todos) != 1 || todos[0].Content != "do something" {
		t.Errorf("active_form fallback 失败: %+v", todos)
	}
}

func TestParseTodosJSON_DefaultStatus(t *testing.T) {
	js := `[{"content":"x"}]`
	todos := parseTodosJSON(js)
	if todos[0].Status != "pending" {
		t.Errorf("缺 status 应默认为 pending, got %s", todos[0].Status)
	}
}

func TestExtractTodos_FromLatestPlan(t *testing.T) {
	msgs := []chatMsg{
		{Role: "user", Content: "do x"},
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("write_todos", `[{"content":"old task","status":"completed"}]`),
		}},
		{Role: "assistant", Content: "thinking"},
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("write_todos", `[{"content":"new task","status":"in_progress"}]`),
		}},
	}
	todos := extractTodos(msgs)
	if len(todos) != 1 || todos[0].Content != "new task" {
		t.Errorf("应取最近的 todos, got %+v", todos)
	}
}

func TestExtractTodos_NoPlan(t *testing.T) {
	msgs := []chatMsg{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	if got := extractTodos(msgs); got != nil {
		t.Errorf("无 plan 应返回 nil, got %+v", got)
	}
}

func TestExtractTodos_AcceptsMultipleNames(t *testing.T) {
	for _, name := range []string{"write_todos", "set_plan", "plan_update", "todos", "update_todos"} {
		msgs := []chatMsg{{
			Role: "assistant",
			ToolCalls: []toolCallAccumulator{
				makeToolCall(name, `[{"content":"x","status":"pending"}]`),
			},
		}}
		todos := extractTodos(msgs)
		if len(todos) != 1 {
			t.Errorf("tool name %q 应被识别为 plan, got %v", name, todos)
		}
	}
}

// ─── 引用文件抽取 ────────────────────────────────────────────

func TestExtractReferencedFiles_Empty(t *testing.T) {
	if got := extractReferencedFiles(nil); got != nil {
		t.Errorf("空应返回 nil, got %v", got)
	}
}

func TestExtractReferencedFiles_ReadFile(t *testing.T) {
	msgs := []chatMsg{
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("read_file", `{"mount_id":"serving","rel_path":"/a.txt"}`),
		}},
	}
	refs := extractReferencedFiles(msgs)
	if len(refs) != 1 {
		t.Fatalf("refs 数 = %d, want 1", len(refs))
	}
	if refs[0].Path != "/a.txt" || refs[0].MountID != "serving" || refs[0].ViaTool != "read_file" {
		t.Errorf("ref = %+v", refs[0])
	}
}

func TestExtractReferencedFiles_ListFiles(t *testing.T) {
	msgs := []chatMsg{
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("list_files", `{"mount_id":"webdav","rel_path":"/sub"}`),
		}},
	}
	refs := extractReferencedFiles(msgs)
	if len(refs) != 1 {
		t.Fatalf("refs 数 = %d, want 1", len(refs))
	}
	if refs[0].Path != "/sub" {
		t.Errorf("path = %q, want /sub", refs[0].Path)
	}
}

func TestExtractReferencedFiles_ListFilesDefaultRoot(t *testing.T) {
	msgs := []chatMsg{
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("list_files", `{"mount_id":"serving"}`),
		}},
	}
	refs := extractReferencedFiles(msgs)
	if len(refs) != 1 || refs[0].Path != "/" {
		t.Errorf("rel_path 缺省应默认为 '/', got %+v", refs[0])
	}
}

func TestExtractReferencedFiles_DedupAndRecency(t *testing.T) {
	msgs := []chatMsg{
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("read_file", `{"mount_id":"serving","rel_path":"/a.txt"}`),
		}},
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("read_file", `{"mount_id":"serving","rel_path":"/b.txt"}`),
		}},
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("read_file", `{"mount_id":"serving","rel_path":"/a.txt"}`),
		}},
	}
	refs := extractReferencedFiles(msgs)
	if len(refs) != 2 {
		t.Fatalf("refs 数 = %d, want 2 (a, b)", len(refs))
	}
	if refs[0].Path != "/a.txt" {
		t.Errorf("最近引用的 a.txt 应排第一, got %+v", refs[0])
	}
}

func TestExtractReferencedFiles_IgnoresPluginTools(t *testing.T) {
	msgs := []chatMsg{
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("video_encrypt", `{"input":"/x.mp4","output":"/y.encv"}`),
		}},
	}
	if got := extractReferencedFiles(msgs); got != nil {
		t.Errorf("plugin 工具不应被记录为引用文件, got %+v", got)
	}
}

func TestReadPathFromToolArgs_UnknownTool(t *testing.T) {
	if got := readPathFromToolArgs("unknown_tool", `{"foo":1}`); got != nil {
		t.Errorf("未知工具应返回 nil, got %+v", got)
	}
}

func TestReadPathFromToolArgs_InvalidJSON(t *testing.T) {
	if got := readPathFromToolArgs("read_file", "not-json"); got != nil {
		t.Errorf("非法 args 应返回 nil, got %+v", got)
	}
}

func TestReadPathFromToolArgs_MissingRelPath(t *testing.T) {
	if got := readPathFromToolArgs("read_file", `{"mount_id":"serving"}`); got != nil {
		t.Errorf("缺 rel_path 应返回 nil, got %+v", got)
	}
}

// ─── v2 工具抽取 ──────────────────────────────────────────

func TestReadPathFromToolArgs_V2_ReadFileV2(t *testing.T) {
	got := readPathFromToolArgs("read_file_v2", `{"mount_id":"serving","rel_path":"/videos/clip.mp4","start_line":10,"end_line":50}`)
	if got == nil || got.Path != "/videos/clip.mp4" || got.MountID != "serving" {
		t.Errorf("read_file_v2 应抽取到 serving+/videos/clip.mp4, got %+v", got)
	}
}

func TestReadPathFromToolArgs_V2_GetMetadata(t *testing.T) {
	got := readPathFromToolArgs("get_metadata", `{"mount_id":"serving","rel_path":"/photos/img.jpg","include_hash":true}`)
	if got == nil || got.Path != "/photos/img.jpg" {
		t.Errorf("get_metadata 应抽取到 /photos/img.jpg, got %+v", got)
	}
}

func TestReadPathFromToolArgs_V2_SearchFiles_DefaultRoot(t *testing.T) {
	// search_files 不传 rel_path 时默认 "/"（与 list_files 行为一致）
	got := readPathFromToolArgs("search_files", `{"mount_id":"serving","recursive":true,"expression":{"op":"name_glob","value":"*.mp4"}}`)
	if got == nil || got.Path != "/" {
		t.Errorf("search_files 缺省 rel_path 应退化为 /, got %+v", got)
	}
}

func TestReadPathFromToolArgs_V2_EditMetadata(t *testing.T) {
	got := readPathFromToolArgs("edit_metadata", `{"mount_id":"serving","rel_path":"/songs/a.mp3","metadata":{"title":"x"}}`)
	if got == nil || got.Path != "/songs/a.mp3" {
		t.Errorf("edit_metadata 应抽取到 /songs/a.mp3, got %+v", got)
	}
}

func TestReadPathFromToolArgs_V2_DeleteFile(t *testing.T) {
	got := readPathFromToolArgs("delete_file", `{"mount_id":"serving","rel_path":"/old/file.txt","mode":"trash"}`)
	if got == nil || got.Path != "/old/file.txt" {
		t.Errorf("delete_file 应抽取到 /old/file.txt, got %+v", got)
	}
}

func TestReadPathFromToolArgs_V2_BatchRename(t *testing.T) {
	got := readPathFromToolArgs("batch_rename", `{"mount_id":"serving","rel_path":"/photos","pattern":"(.*)\\.JPG","replacement":"$1.jpg","dry_run":true}`)
	if got == nil || got.Path != "/photos" {
		t.Errorf("batch_rename 应抽取到 /photos, got %+v", got)
	}
}

func TestReadPathFromToolArgs_V2_CommandRun_Skipped(t *testing.T) {
	// command_run 的 path 散布在 args[] 数组中，无法静态抽取 → 应返回 nil
	if got := readPathFromToolArgs("command_run", `{"mount_id":"serving","command":"ffprobe","args":["-v","error","/videos/x.mp4"]}`); got != nil {
		t.Errorf("command_run 应跳过（无法静态抽 path）, got %+v", got)
	}
}

func TestExtractReferencedFiles_V2Mixed(t *testing.T) {
	// 真实 v2 混合场景：search_files → read_file_v2 → get_metadata
	msgs := []chatMsg{
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("search_files", `{"mount_id":"serving","rel_path":"/videos","recursive":true,"expression":{"op":"name_glob","value":"*.mp4"}}`),
		}},
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("read_file_v2", `{"mount_id":"serving","rel_path":"/videos/clip.mp4"}`),
		}},
		{Role: "assistant", ToolCalls: []toolCallAccumulator{
			makeToolCall("get_metadata", `{"mount_id":"serving","rel_path":"/videos/clip.mp4"}`),
		}},
	}
	refs := extractReferencedFiles(msgs)
	if len(refs) != 2 {
		// search_files /videos + read_file_v2 & get_metadata 都指向 /videos/clip.mp4 → 去重后 2 条
		t.Fatalf("refs 数 = %d, want 2 (search root + file), got %+v", len(refs), refs)
	}
	// 最近一次引用是 /videos/clip.mp4（via get_metadata），应排第一
	if refs[0].Path != "/videos/clip.mp4" {
		t.Errorf("最近引用应是 /videos/clip.mp4, got %+v", refs[0])
	}
	if refs[0].ViaTool != "get_metadata" {
		t.Errorf("viaTool 应是 get_metadata, got %+v", refs[0])
	}
}

// ─── containsAny ────────────────────────────────────────────

func TestContainsAny(t *testing.T) {
	if !containsAny("hello128k", "128k") {
		t.Error("应找到 128k")
	}
	if containsAny("hello", "128k") {
		t.Error("不应找到")
	}
	if containsAny("", "128k") {
		t.Error("空字符串应返回 false")
	}
}

// ─── HTTP Handler ────────────────────────────────────────────

func resetSessionsForTest() {
	sessionMu.Lock()
	sessions = make(map[string]*agentSession)
	sessionMu.Unlock()
}

func TestHandleAgentContextUsage_NoSession(t *testing.T) {
	resetSessionsForTest()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv := &Server{}
	r.GET("/ctx", srv.handleAgentContextUsage)

	req := httptest.NewRequest("GET", "/ctx?sessionId=missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body 非 JSON: %v\n%s", err, w.Body.String())
	}
	if body["note"] == nil {
		t.Errorf("缺 note 字段: %s", w.Body.String())
	}
	usage, ok := body["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺 usage 字段: %s", w.Body.String())
	}
	if usage["tokens"].(float64) != 0 {
		t.Errorf("无 session 时 tokens 应为 0, got %v", usage["tokens"])
	}
}

func TestHandleAgentContextUsage_WithSession(t *testing.T) {
	resetSessionsForTest()
	sessionMu.Lock()
	sess := &agentSession{
		SessionID: "test-sess",
		LastModel: "gpt-4o",
		Messages: []chatMsg{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
		GrantedTools: make(map[string]bool),
	}
	sessions["test-sess"] = sess
	sessionMu.Unlock()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv := &Server{}
	r.GET("/ctx", srv.handleAgentContextUsage)

	req := httptest.NewRequest("GET", "/ctx?sessionId=test-sess", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body 非 JSON: %v\n%s", err, w.Body.String())
	}
	if body["sessionId"] != "test-sess" {
		t.Errorf("sessionId = %v, want test-sess", body["sessionId"])
	}
	if body["model"] != "gpt-4o" {
		t.Errorf("model = %v, want gpt-4o", body["model"])
	}
	usage := body["usage"].(map[string]interface{})
	if usage["window"].(float64) != 128000 {
		t.Errorf("window = %v, want 128000 (gpt-4o)", usage["window"])
	}
	if usage["tokens"].(float64) <= 0 {
		t.Errorf("tokens 应 > 0, got %v", usage["tokens"])
	}
}

func TestHandleAgentContextUsage_WithTodosAndRefs(t *testing.T) {
	resetSessionsForTest()
	sessionMu.Lock()
	sess := &agentSession{
		SessionID: "rich-sess",
		LastModel: "gpt-4o",
		Messages: []chatMsg{
			{Role: "user", Content: "encrypt my file"},
			{Role: "assistant", ToolCalls: []toolCallAccumulator{
				makeToolCall("write_todos", `[{"content":"读取 x.txt","status":"in_progress"}]`),
			}},
			{Role: "assistant", ToolCalls: []toolCallAccumulator{
				makeToolCall("read_file", `{"mount_id":"serving","rel_path":"/x.txt"}`),
			}},
		},
		GrantedTools: make(map[string]bool),
	}
	sessions["rich-sess"] = sess
	sessionMu.Unlock()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv := &Server{}
	r.GET("/ctx", srv.handleAgentContextUsage)

	req := httptest.NewRequest("GET", "/ctx?sessionId=rich-sess", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)

	todos, _ := body["todos"].([]interface{})
	if len(todos) != 1 {
		t.Errorf("todos 数 = %d, want 1", len(todos))
	}
	refs, _ := body["referencedFiles"].([]interface{})
	if len(refs) != 1 {
		t.Errorf("refs 数 = %d, want 1", len(refs))
	}
	if refs[0].(map[string]interface{})["path"] != "/x.txt" {
		t.Errorf("ref path = %v, want /x.txt", refs[0])
	}
}

func TestHandleAgentContextUsage_DefaultSessionId(t *testing.T) {
	resetSessionsForTest()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv := &Server{}
	r.GET("/ctx", srv.handleAgentContextUsage)

	req := httptest.NewRequest("GET", "/ctx", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["sessionId"] != "default" {
		t.Errorf("默认 sessionId = %v, want default", body["sessionId"])
	}
}

func TestHandleAgentContextUsage_Percent(t *testing.T) {
	resetSessionsForTest()
	sessionMu.Lock()
	sess := &agentSession{
		SessionID: "pct-sess",
		LastModel: "gpt-4o",
		Messages: []chatMsg{
			{Role: "user", Content: "hello"},
		},
		GrantedTools: make(map[string]bool),
	}
	sessions["pct-sess"] = sess
	sessionMu.Unlock()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv := &Server{}
	r.GET("/ctx", srv.handleAgentContextUsage)

	req := httptest.NewRequest("GET", "/ctx?sessionId=pct-sess", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	usage := body["usage"].(map[string]interface{})
	percent := usage["percent"].(float64)
	if percent < 0 || percent > 100 {
		t.Errorf("percent = %v, 应在 [0,100]", percent)
	}
	if percent >= 1.0 {
		t.Errorf("1 token / 128K window 应 < 1%%, got %v", percent)
	}
}
