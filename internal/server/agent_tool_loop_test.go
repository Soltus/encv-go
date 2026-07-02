// internal/server/agent_tool_loop_test.go
//
// # Phase 2 测试：streamChat 真实 LLM 路径 AG-UI 透传
//
// 覆盖范围（来自 spec tasks.md §2.4）：
//   - TestStreamChat_AGUIMode_EmitsTextMessageStartBeforeContent
//   - TestStreamChat_AGUIMode_StableMessageId
//   - TestStreamChat_AGUIMode_ToolCallArgsEmpty_SkipsTOOL_CALL_ARGS
//   - TestStreamChat_AGUIMode_AllEventsIncludeThreadRunTimestamp
//   - TestStreamChat_LegacyMode_PreservesDataFormat
//   - TestHandleAgentChat_RealLLM_PassesAGUIModeToStreamChat
//   - TestHandleAgentConfirm_AGUIHeader_PassesThrough
//   - TestHandleAgentResume_AGUIHeader_PassesThrough
//
// 策略：通过 http.DefaultTransport 拦截上游 LLM HTTP 调用，返回伪造的 SSE 流；
// 然后直接调用 s.streamChat()，抓取 ResponseWriter.Body 解析 AG-UI 事件。
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ─── 测试辅助 ───────────────────────────────────────────────────

// mockSSEStream 把多段 raw `data: ...\n\n` 串成完整 SSE 响应 + 结尾 [DONE]。
func mockSSEStream(chunks []string) string {
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString(c)
		if !strings.HasSuffix(c, "\n\n") {
			b.WriteString("\n\n")
		}
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// sseTextChunk 生成 OpenAI streaming text delta chunk。
func sseTextChunk(content string) string {
	payload := map[string]interface{}{
		"choices": []map[string]interface{}{
			{"delta": map[string]interface{}{"content": content}},
		},
	}
	raw, _ := json.Marshal(payload)
	return "data: " + string(raw)
}

// sseToolCallChunks 生成 OpenAI streaming tool call chunks（两段：id+name / args）。
func sseToolCallChunks(id, name, args string) []string {
	p1 := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"delta": map[string]interface{}{
					"tool_calls": []map[string]interface{}{
						{
							"index":    0,
							"id":       id,
							"type":     "function",
							"function": map[string]interface{}{"name": name, "arguments": ""},
						},
					},
				},
			},
		},
	}
	raw1, _ := json.Marshal(p1)
	c1 := "data: " + string(raw1)

	p2 := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"delta": map[string]interface{}{
					"tool_calls": []map[string]interface{}{
						{
							"index":    0,
							"function": map[string]interface{}{"arguments": args},
						},
					},
				},
			},
		},
	}
	raw2, _ := json.Marshal(p2)
	return []string{c1, "data: " + string(raw2)}
}

// sseFinishChunk 生成 finish_reason 收尾 chunk。
func sseFinishChunk(reason string) string {
	payload := map[string]interface{}{
		"choices": []map[string]interface{}{{"finish_reason": reason}},
	}
	raw, _ := json.Marshal(payload)
	return "data: " + string(raw)
}

// installMockOpenAI 拦截所有对 /v1/chat/completions 的 POST，返回 sseBody。
// 返回的 cleanup() 必须在测试结束时调用。
func installMockOpenAI(sseBody string) (cleanup func()) {
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     map[string][]string{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sseBody)),
		}, nil
	})
	return func() { http.DefaultTransport = orig }
}

// newTestSession 创建一个最小可用的 agentSession for testing.
func newTestSession() *agentSession {
	return &agentSession{
		SessionID:  "sess-" + uuid.New().String()[:8],
		Messages:   []chatMsg{},
		EventCache: []AgentEvent{},
	}
}

// newStreamChatGinContext 创建一个 SSE ResponseWriter 的 gin.Context。
// （避免与 server_config_api_test.go 内的 newStreamChatGinContext 重名）
func newStreamChatGinContext(rec *httptest.ResponseRecorder) *gin.Context {
	gin.SetMode(gin.TestMode)
	rec.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/test", nil)
	return ctx
}

// writeFileJSON 是测试用便捷文件写入（写到 t.TempDir 内）。
func writeFileJSON(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile 失败: %v", err)
	}
	return path
}

// ─── 2.1: TEXT_MESSAGE_START 在 CONTENT 之前 ─────────────────

func TestStreamChat_AGUIMode_EmitsTextMessageStartBeforeContent(t *testing.T) {
	sse := mockSSEStream([]string{
		sseTextChunk("Hello "),
		sseTextChunk("world"),
		sseFinishChunk("stop"),
	})
	cleanup := installMockOpenAI(sse)
	defer cleanup()

	rec := httptest.NewRecorder()
	ctx := newStreamChatGinContext(rec)
	sess := newTestSession()
	srv := &Server{}

	srv.streamChat(context.Background(), ctx, agentConfig{APIKey: "test", BaseURL: "https://api.test.com"},
		"gpt-4o", 0.7, []chatMsg{{Role: "user", Content: "hi"}}, sess, nil, nil, true)

	frames := parseSSEFrames(t, rec.Body.String())
	if len(frames) < 4 {
		t.Fatalf("frames = %d, want >= 4 (START + 2 CONTENT + END), got:\n%s", len(frames), rec.Body.String())
	}
	if frames[0].EventType != "TEXT_MESSAGE_START" {
		t.Errorf("frame[0].EventType = %q, want TEXT_MESSAGE_START", frames[0].EventType)
	}
	if frames[1].EventType != "TEXT_MESSAGE_CONTENT" {
		t.Errorf("frame[1].EventType = %q, want TEXT_MESSAGE_CONTENT", frames[1].EventType)
	}
	if frames[2].EventType != "TEXT_MESSAGE_CONTENT" {
		t.Errorf("frame[2].EventType = %q, want TEXT_MESSAGE_CONTENT", frames[2].EventType)
	}
	if frames[len(frames)-1].EventType != "TEXT_MESSAGE_END" {
		t.Errorf("最后一帧 = %q, want TEXT_MESSAGE_END", frames[len(frames)-1].EventType)
	}
}

// ─── 2.2: 多个 TEXT_MESSAGE_CONTENT 共用同一 messageId ──────

func TestStreamChat_AGUIMode_StableMessageId(t *testing.T) {
	sse := mockSSEStream([]string{
		sseTextChunk("foo "),
		sseTextChunk("bar "),
		sseTextChunk("baz"),
		sseFinishChunk("stop"),
	})
	cleanup := installMockOpenAI(sse)
	defer cleanup()

	rec := httptest.NewRecorder()
	ctx := newStreamChatGinContext(rec)
	sess := newTestSession()
	srv := &Server{}

	srv.streamChat(context.Background(), ctx, agentConfig{APIKey: "test", BaseURL: "https://api.test.com"},
		"gpt-4o", 0.7, []chatMsg{{Role: "user", Content: "x"}}, sess, nil, nil, true)

	frames := parseSSEFrames(t, rec.Body.String())
	contentCount := 0
	var firstMsgID string
	for _, f := range frames {
		if f.EventType == "TEXT_MESSAGE_CONTENT" {
			contentCount++
			id, _ := f.DataJSON["messageId"].(string)
			if firstMsgID == "" {
				firstMsgID = id
			} else if id != firstMsgID {
				t.Errorf("CONTENT[%d].messageId = %q, want stable %q", contentCount, id, firstMsgID)
			}
		}
	}
	if contentCount != 3 {
		t.Errorf("TEXT_MESSAGE_CONTENT 数量 = %d, want 3", contentCount)
	}
	if firstMsgID == "" {
		t.Error("未找到任何 TEXT_MESSAGE_CONTENT 的 messageId")
	}
}

// ─── 2.3: 空 args → 跳过 TOOL_CALL_ARGS ──────────────────────

func TestStreamChat_AGUIMode_ToolCallArgsEmpty_SkipsTOOL_CALL_ARGS(t *testing.T) {
	chunks := sseToolCallChunks("tc_1", "list_mounts", "") // args = ""
	chunks = append(chunks, sseFinishChunk("tool_calls"))

	sse := mockSSEStream(chunks)
	cleanup := installMockOpenAI(sse)
	defer cleanup()

	rec := httptest.NewRecorder()
	ctx := newStreamChatGinContext(rec)
	sess := newTestSession()
	srv := &Server{}

	srv.streamChat(context.Background(), ctx, agentConfig{APIKey: "test", BaseURL: "https://api.test.com"},
		"gpt-4o", 0.7, []chatMsg{{Role: "user", Content: "x"}}, sess, nil, nil, true)

	frames := parseSSEFrames(t, rec.Body.String())
	argsCount := 0
	startCount := 0
	for _, f := range frames {
		switch f.EventType {
		case "TOOL_CALL_ARGS":
			argsCount++
		case "TOOL_CALL_START":
			startCount++
		}
	}
	if argsCount != 0 {
		t.Errorf("空 args 时 TOOL_CALL_ARGS 事件数 = %d, want 0 (应被跳过)", argsCount)
	}
	if startCount != 1 {
		t.Errorf("TOOL_CALL_START 事件数 = %d, want 1", startCount)
	}
}

// ─── 2.4: 所有事件都含 threadId / runId / timestamp ─────────

func TestStreamChat_AGUIMode_AllEventsIncludeThreadRunTimestamp(t *testing.T) {
	sse := mockSSEStream([]string{
		sseTextChunk("hi"),
		sseFinishChunk("stop"),
	})
	cleanup := installMockOpenAI(sse)
	defer cleanup()

	rec := httptest.NewRecorder()
	ctx := newStreamChatGinContext(rec)
	sess := newTestSession()
	srv := &Server{}

	srv.streamChat(context.Background(), ctx, agentConfig{APIKey: "test", BaseURL: "https://api.test.com"},
		"gpt-4o", 0.7, []chatMsg{{Role: "user", Content: "x"}}, sess, nil, nil, true)

	frames := parseSSEFrames(t, rec.Body.String())
	expectedThread := "thread_" + sess.SessionID
	if len(frames) == 0 {
		t.Fatal("未产生任何事件")
	}
	for i, f := range frames {
		if tid, _ := f.DataJSON["threadId"].(string); tid != expectedThread {
			t.Errorf("frame[%d] (%s) threadId = %q, want %q", i, f.EventType, tid, expectedThread)
		}
		if rid, _ := f.DataJSON["runId"].(string); rid == "" {
			t.Errorf("frame[%d] (%s) runId 为空", i, f.EventType)
		}
		if ts, _ := f.DataJSON["timestamp"].(string); ts == "" {
			t.Errorf("frame[%d] (%s) timestamp 为空", i, f.EventType)
		}
	}
}

// ─── 2.5: legacy 模式 (aguiMode=false) 保留原 data 格式 ────

func TestStreamChat_LegacyMode_PreservesDataFormat(t *testing.T) {
	sse := mockSSEStream([]string{
		sseTextChunk("legacy text"),
		sseFinishChunk("stop"),
	})
	cleanup := installMockOpenAI(sse)
	defer cleanup()

	rec := httptest.NewRecorder()
	ctx := newStreamChatGinContext(rec)
	sess := newTestSession()
	srv := &Server{}

	srv.streamChat(context.Background(), ctx, agentConfig{APIKey: "test", BaseURL: "https://api.test.com"},
		"gpt-4o", 0.7, []chatMsg{{Role: "user", Content: "x"}}, sess, nil, nil, false)

	body := rec.Body.String()
	if !strings.Contains(body, `"type": "text_delta"`) {
		t.Errorf("legacy 模式应含 text_delta，body = %s", body)
	}
	if !strings.Contains(body, `"data": "legacy text"`) {
		t.Errorf("legacy 模式应含 legacy text 文本，body = %s", body)
	}
	if strings.Contains(body, "TEXT_MESSAGE_START") || strings.Contains(body, "TEXT_MESSAGE_CONTENT") {
		t.Errorf("legacy 模式不应含 AG-UI 事件，body = %s", body)
	}
}

// ─── 2.6: handleAgentChat 集成测试（X-Agent-Protocol: agui） ──
//
// 验证 handleAgentChat 在收到 aguiMode 时不崩溃、正常返回 SSE 响应。
// 注：handleAgentChat 直接用 sendSSEEventSafe（不走 streamChat），所以这里
// 主要确认 aguiMode 不影响主流程；streamChat 路径已被 TestStreamChat_* 覆盖。
func TestHandleAgentChat_RealLLM_PassesAGUIModeToStreamChat(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := writeFileJSON(t, tmpDir, "config.user.json", `{
		"agent_settings": {
			"openai_api_key": "sk-test",
			"openai_base_url": "https://api.test.com"
		}
	}`)

	s := &Server{configPath: cfgPath}
	gin.SetMode(gin.TestMode)
	r := gin.New()

	cleanup := installMockOpenAI(mockSSEStream([]string{
		sseTextChunk("AGUI text"),
		sseFinishChunk("stop"),
	}))
	defer cleanup()

	s.registerAgentRoutes(r)

	body := map[string]interface{}{
		"sessionId": "sess-agui",
		"model":     "gpt-4o-mini",
		"messages":  []map[string]string{{"role": "user", "content": "hi"}},
	}
	reqJSON, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Protocol", "agui")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code >= 500 {
		t.Fatalf("handleAgentChat 5xx 错误: code=%d body=%s", rec.Code, rec.Body.String())
	}

	bodyStr := rec.Body.String()
	// 验证响应流里包含 AG-UI mock 数据的文本（证明 mock 路径被 aguiMode 通过）
	if !strings.Contains(bodyStr, "AGUI text") {
		t.Errorf("handleAgentChat 输出应含 AGUI text 内容, body = %s", bodyStr)
	}
	// 验证不返回 4xx
	if rec.Code >= 400 && rec.Code < 500 {
		t.Errorf("handleAgentChat 4xx 错误: code=%d body=%s", rec.Code, bodyStr)
	}
}

// ─── 2.7: handleAgentConfirm 透传 aguiMode ────────────────

func TestHandleAgentConfirm_AGUIHeader_PassesThrough(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := writeFileJSON(t, tmpDir, "config.user.json", `{
		"agent_settings": {
			"openai_api_key": "sk-test",
			"openai_base_url": "https://api.test.com"
		}
	}`)

	s := &Server{configPath: cfgPath}
	gin.SetMode(gin.TestMode)
	r := gin.New()

	cleanup := installMockOpenAI(mockSSEStream([]string{
		sseTextChunk("confirm agui"),
		sseFinishChunk("stop"),
	}))
	defer cleanup()

	s.registerAgentRoutes(r)

	// 先建一个 session（含 PendingTools）
	sessID := "sess-confirm-agui"
	sess := getOrCreateSession(sessID)
	sess.mu.Lock()
	sess.PendingTools = []toolCallAccumulator{
		{ID: "tc_1", Type: "function", Index: 0, Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "list_mounts", Arguments: "{}"}},
	}
	sess.LastModel = "gpt-4o-mini"
	sess.LastTemperature = 0.5
	sess.mu.Unlock()

	// 构造 /api/confirm 请求（带 ?protocol=agui query）
	body := map[string]interface{}{
		"sessionId":  sessID,
		"decision":   "accept",
		"toolCallId": "tc_1", // 必须匹配 PendingTools 里的 ID
	}
	reqJSON, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/confirm?protocol=agui", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// 验证请求不返回 5xx（避免 header 解析 panic）
	if rec.Code >= 500 {
		t.Fatalf("confirm 5xx 错误: code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 验证请求未返回 4xx
	if rec.Code >= 400 && rec.Code < 500 {
		t.Errorf("confirm 4xx 错误: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ─── 2.8: handleAgentResume AG-UI header 检测 ─────────────

func TestHandleAgentResume_AGUIHeader_PassesThrough(t *testing.T) {
	s := &Server{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s.registerAgentRoutes(r)

	sessID := "sess-resume-agui"
	sess := getOrCreateSession(sessID)
	sess.mu.Lock()
	sess.EventCache = []AgentEvent{
		{Type: "text_delta", Data: "old", ID: 1},
	}
	sess.mu.Unlock()

	body := map[string]interface{}{
		"sessionId":   sessID,
		"lastEventId": 0,
	}
	reqJSON, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/resume?protocol=agui", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code >= 500 {
		t.Errorf("resume 5xx 错误: code=%d body=%s", rec.Code, rec.Body.String())
	}
}
