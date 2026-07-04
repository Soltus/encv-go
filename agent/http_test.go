package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// realLLMFake is a thin llmStream that wraps a real
// *openai.Client pointed at an httptest server. It is the
// cleanest way to exercise the agent's full Chat/Resume/Confirm
// loop from tests: the openai library's stream handling is
// well-tested, and the test gets to drive the *contents* of
// the stream without reimplementing the SSE parser.
type realLLMFake struct {
	client *openai.Client
}

// CreateChatCompletionStream delegates to the underlying
// client. Errors propagate unchanged.
func (r *realLLMFake) CreateChatCompletionStream(ctx context.Context, req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return r.client.CreateChatCompletionStream(ctx, req)
}

// scriptedOpenAIHandler returns an http.Handler that emits a
// canned SSE response. The body is a string of
// "data: {...}" chunks. The final chunk is always
// "data: [DONE]".
func scriptedOpenAIHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, line := range strings.Split(body, "\n") {
			if line == "" {
				continue
			}
			_, _ = io.WriteString(w, line+"\n")
		}
		if flusher != nil {
			flusher.Flush()
		}
	})
}

// buildChatCompletionStreamBody composes the SSE body for a
// sequence of parsed deltas. It mirrors the chunks the real
// OpenAI API emits.
func buildChatCompletionStreamBody(deltas []parsedDelta) string {
	lines := []string{}
	for _, d := range deltas {
		chunk := map[string]any{
			"id":      "x",
			"object":  "chat.completion.chunk",
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{}}},
		}
		delta := chunk["choices"].([]map[string]any)[0]["delta"].(map[string]any)
		if d.Text != "" {
			delta["content"] = d.Text
		}
		if d.Reasoning != "" {
			delta["reasoning_content"] = d.Reasoning
		}
		if len(d.ToolCalls) > 0 {
			calls := []map[string]any{}
			for _, tc := range d.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			delta["tool_calls"] = calls
		}
		if d.Finished {
			chunk["choices"].([]map[string]any)[0]["finish_reason"] = "stop"
		}
		b, _ := json.Marshal(chunk)
		lines = append(lines, "data: "+string(b))
	}
	lines = append(lines, "data: [DONE]", "")
	return strings.Join(lines, "\n")
}

// makeFakeAgentWithBody creates an Agent whose LLM is wired to
// a real *openai.Client pointed at an httptest server that
// returns the supplied body. Tests can pre-script the stream
// to drive the agent's Chat loop without hitting OpenAI.
func makeFakeAgentWithBody(t *testing.T, body string) *Agent {
	t.Helper()
	srv := httptest.NewServer(scriptedOpenAIHandler(body))
	t.Cleanup(srv.Close)
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = srv.URL + "/v1"
	a := NewAgentWithLLM(AgentConfig{OpenAIModel: "gpt-4o"}, NewRegistry(), &realLLMFake{
		client: openai.NewClientWithConfig(cfg),
	})
	return a
}

// makeFakeAgent is a convenience wrapper that builds the SSE
// body from a slice of parsedDelta.
func makeFakeAgent(t *testing.T, deltas []parsedDelta) *Agent {
	t.Helper()
	return makeFakeAgentWithBody(t, buildChatCompletionStreamBody(deltas))
}

// drainSSE reads the SSE body into a slice of {type, data}
// pairs. Tests use this to assert on the events the handler
// emits.
func drainSSE(t *testing.T, body io.Reader) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ---- HTTP handler tests ----

func TestHandleChat_MethodNotAllowed(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	req := httptest.NewRequest("GET", "/api/chat", nil)
	rec := httptest.NewRecorder()
	a.HandleChat(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
	if rec.Header().Get("Allow") != "POST" {
		t.Errorf("expected Allow: POST header")
	}
}

func TestHandleChat_InvalidJSON(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	a.HandleChat(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleChat_MissingSessionID(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	body := `{"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleChat(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleChat_PendingCallConflict(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	a.mu.Lock()
	a.PendingCalls["sess"] = &pendingCall{ToolCallID: "x", ToolName: "y", Args: "{}"}
	a.mu.Unlock()
	body := `{"session_id":"sess","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleChat(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestHandleChat_TextOnly exercises the happy path of a turn
// that produces only text. The fake LLM emits two text deltas
// and a stop. The handler must stream them as SSE and finish
// with a stream_end event.
func TestHandleChat_TextOnly(t *testing.T) {
	a := makeFakeAgent(t, []parsedDelta{
		{Text: "hello "},
		{Text: "world", Finished: true},
	})

	body, _ := json.Marshal(ChatRequest{
		SessionID: "sess_1",
		Messages: []map[string]any{
			{"role": "user", "content": "hi"},
		},
	})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleChat(rec, req)

	events := drainSSE(t, rec.Body)
	types := make([]string, 0, len(events))
	for _, e := range events {
		if t, ok := e["type"].(string); ok {
			types = append(types, t)
		}
	}
	if !containsString(types, string(EventTextDelta)) {
		t.Errorf("expected text_delta event, got types: %v", types)
	}
	if !containsString(types, string(EventStreamEnd)) {
		t.Errorf("expected stream_end event, got types: %v", types)
	}
}

func TestHandleResume_MethodNotAllowed(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	req := httptest.NewRequest("GET", "/api/resume", nil)
	rec := httptest.NewRecorder()
	a.HandleResume(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405")
	}
}

func TestHandleResume_SessionNotFound(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	body, _ := json.Marshal(ResumeRequest{SessionID: "missing", Offset: 0})
	req := httptest.NewRequest("POST", "/api/resume", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleResume(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleResume_ReplaysEvents(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	// Pre-populate the session.
	c := a.ensureSession("sess_1")
	c.appendEvent(&Event{Type: EventTextDelta, Data: `{"content":"a"}`})
	c.appendEvent(&Event{Type: EventStreamEnd, Data: ""})
	c.mu.Lock()
	c.IsFinished = true
	c.mu.Unlock()

	body, _ := json.Marshal(ResumeRequest{SessionID: "sess_1", Offset: 0})
	req := httptest.NewRequest("POST", "/api/resume", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleResume(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"text_delta"`) {
		t.Errorf("expected text_delta in replayed events")
	}
}

func TestHandleConfirm_MethodNotAllowed(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	req := httptest.NewRequest("GET", "/api/confirm", nil)
	rec := httptest.NewRecorder()
	a.HandleConfirm(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405")
	}
}

func TestHandleConfirm_InvalidDecision(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	body, _ := json.Marshal(ConfirmRequest{
		SessionID:  "s",
		ToolCallID: "t",
		Decision:   "bogus",
	})
	req := httptest.NewRequest("POST", "/api/confirm", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleConfirm(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid decision, got %d", rec.Code)
	}
}

func TestHandleConfirm_RejectsEmptySession(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	body, _ := json.Marshal(ConfirmRequest{SessionID: "", ToolCallID: "t", Decision: "accept"})
	req := httptest.NewRequest("POST", "/api/confirm", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleConfirm(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleConfirm_CancelPath(t *testing.T) {
	a := makeFakeAgent(t, nil) // empty stream is fine for cancel
	a.mu.Lock()
	a.PendingCalls["s"] = &pendingCall{
		ToolCallID: "t",
		ToolName:   "x",
		Args:       "{}",
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{
					{ID: "t", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "x", Arguments: "{}"}},
				},
			},
		},
	}
	a.mu.Unlock()
	body, _ := json.Marshal(ConfirmRequest{SessionID: "s", ToolCallID: "t", Decision: "cancel"})
	req := httptest.NewRequest("POST", "/api/confirm", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleConfirm(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestWriteSSE_NilEventIsNoOp(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := writeSSE(rec, nil); err != nil {
		t.Errorf("writeSSE(nil) should not error, got %v", err)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("writeSSE(nil) should write nothing")
	}
}

func TestWriteSSE_FormatsCorrectly(t *testing.T) {
	rec := httptest.NewRecorder()
	ev := &Event{Type: EventTextDelta, Data: `{"content":"hi"}`}
	if err := writeSSE(rec, ev); err != nil {
		t.Errorf("writeSSE: %v", err)
	}
	out := rec.Body.String()
	if !strings.HasPrefix(out, "data: ") || !strings.HasSuffix(out, "\n\n") {
		t.Errorf("SSE frame shape: %q", out)
	}
	// The frame is JSON-of-JSON: the outer object wraps
	// {type, data} where data is the already-stringified
	// payload. We assert on the structural pieces, not on
	// the exact escape sequence.
	if !strings.Contains(out, `"text_delta"`) {
		t.Errorf("SSE frame content: %q", out)
	}
	if !strings.Contains(out, `content`) || !strings.Contains(out, `hi`) {
		t.Errorf("SSE frame payload not preserved: %q", out)
	}
}

func TestConvertMessages_AcceptsCommonShapes(t *testing.T) {
	in := []map[string]any{
		{"role": "system", "content": "you are x"},
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "", "tool_calls": []any{
			map[string]any{
				"id":   "c1",
				"type": "function",
				"function": map[string]any{
					"name":      "list_files",
					"arguments": `{"path":"/"}`,
				},
			},
		}},
		{"role": "tool", "content": "result", "name": "list_files", "tool_call_id": "c1"},
	}
	msgs, err := convertMessages(in)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("length: %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("role[0]: %q", msgs[0].Role)
	}
	if msgs[2].Role != "assistant" {
		t.Errorf("role[2]: %q", msgs[2].Role)
	}
	if len(msgs[2].ToolCalls) != 1 {
		t.Errorf("tool_calls[2]: %d", len(msgs[2].ToolCalls))
	}
	if msgs[2].ToolCalls[0].Function.Name != "list_files" {
		t.Errorf("tool_calls[2].name: %q", msgs[2].ToolCalls[0].Function.Name)
	}
	if msgs[3].ToolCallID != "c1" {
		t.Errorf("tool_call_id[3]: %q", msgs[3].ToolCallID)
	}
}

func TestConvertMessages_MissingRole(t *testing.T) {
	in := []map[string]any{{"content": "hi"}}
	_, err := convertMessages(in)
	if err == nil || !strings.Contains(err.Error(), "missing role") {
		t.Errorf("expected missing role error, got %v", err)
	}
}

func TestConvertMessages_RejectsMalformedToolCall(t *testing.T) {
	in := []map[string]any{
		{"role": "assistant", "tool_calls": []any{"not a map"}},
	}
	_, err := convertMessages(in)
	if err == nil {
		t.Errorf("expected error for non-map tool call")
	}
}

func TestConvertMessages_StructuredContentStringified(t *testing.T) {
	in := []map[string]any{
		{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": "hi"},
		}},
	}
	msgs, err := convertMessages(in)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if !strings.Contains(msgs[0].Content, "text") {
		t.Errorf("structured content not stringified: %q", msgs[0].Content)
	}
}

func TestHandleHealth_OK(t *testing.T) {
	a := NewAgent(AgentConfig{OpenAIAPIKey: "k"}, NewRegistry())
	rec := httptest.NewRecorder()
	a.HandleHealth(rec, httptest.NewRequest("GET", "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Errorf("ok flag: %v", body)
	}
	// Task 4.1: /api/health must carry serverInstanceId so the
	// front-end can detect server restarts and clear stale SSE
	// sequence tracking state.
	id, ok := body["serverInstanceId"].(string)
	if !ok || id == "" {
		t.Errorf("serverInstanceId missing or empty: %v", body)
	}
	// The id reported over HTTP must match the one held by the
	// Agent (single source of truth).
	if a.ServerInstanceId() == "" {
		t.Errorf("Agent.ServerInstanceId() should not be empty")
	}
	if id != a.ServerInstanceId() {
		t.Errorf("serverInstanceId mismatch: http=%q agent=%q", id, a.ServerInstanceId())
	}
	// The id must be stable across multiple calls within the
	// same process — it is generated once at NewAgent time and
	// never re-rolled.
	rec2 := httptest.NewRecorder()
	a.HandleHealth(rec2, httptest.NewRequest("GET", "/api/health", nil))
	var body2 map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &body2); err != nil {
		t.Fatal(err)
	}
	if body2["serverInstanceId"] != id {
		t.Errorf("serverInstanceId not stable across calls: %q -> %q", id, body2["serverInstanceId"])
	}
	// It must also contain the host + pid so log readers can
	// correlate it with a real OS process. We embed os.Getpid()
	// and the host name; pid is enough to assert without
	// making the test environment-specific.
	wantPid := fmt.Sprintf("-%d-", os.Getpid())
	if !strings.Contains(id, wantPid) {
		t.Errorf("serverInstanceId %q should embed pid %d", id, os.Getpid())
	}
}

// TestServerInstanceId_DistinctAcrossAgents makes sure two
// agents built in quick succession receive distinct ids. We use
// nanosecond precision in generateServerInstanceId so the two
// calls — even when only nanoseconds apart — always differ.
func TestServerInstanceId_DistinctAcrossAgents(t *testing.T) {
	a1 := NewAgent(AgentConfig{}, NewRegistry())
	a2 := NewAgent(AgentConfig{}, NewRegistry())
	if a1.ServerInstanceId() == a2.ServerInstanceId() {
		t.Errorf("two NewAgent calls should produce distinct ids, got %q twice", a1.ServerInstanceId())
	}
	if a1.ServerInstanceId() == "" || a2.ServerInstanceId() == "" {
		t.Errorf("serverInstanceId must be non-empty: a1=%q a2=%q", a1.ServerInstanceId(), a2.ServerInstanceId())
	}
}

func TestWriteJSONError_ShapeIsConsistent(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSONError(rec, http.StatusBadRequest, "boom")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error"`) || !strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("body: %s", rec.Body.String())
	}
}

// TestPickFirst_SkipsEmptyStrings locks the helper used by
// HandleTest to merge body + config values.
func TestPickFirst_SkipsEmptyStrings(t *testing.T) {
	if got := pickFirst("", "a", "b"); got != "a" {
		t.Errorf("pickFirst: %q", got)
	}
	if got := pickFirst("", "", ""); got != "" {
		t.Errorf("pickFirst all empty: %q", got)
	}
	if got := pickFirst("x", "y"); got != "x" {
		t.Errorf("pickFirst: %q", got)
	}
}

// containsString is a small local helper that mirrors the one
// in config_loader_test.go but is not exported.
func containsString(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

// ---- test handler tests ----

// TestHandleTest_BothEndpointsOK sets up two httptest servers
// that mimic OpenAI and OpenList success responses, then
// verifies the handler reports both as ok.
func TestHandleTest_BothEndpointsOK(t *testing.T) {
	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer openai.Close()
	openlist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	}))
	defer openlist.Close()

	a := NewAgent(AgentConfig{
		OpenAIAPIKey:    "k",
		OpenAIBaseURL:   openai.URL,
		OpenListBaseURL: openlist.URL,
		OpenListToken:   "t",
	}, NewRegistry())

	body, _ := json.Marshal(TestRequest{})
	req := httptest.NewRequest("POST", "/api/agent/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp TestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OpenAIOk {
		t.Errorf("OpenAI: %s", resp.OpenAIError)
	}
	if !resp.OpenListOk {
		t.Errorf("OpenList: %s", resp.OpenListError)
	}
}

// TestHandleTest_OpenAIUnauthorized exercises the 401 path.
func TestHandleTest_OpenAIUnauthorized(t *testing.T) {
	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer openai.Close()
	openlist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer openlist.Close()

	a := NewAgent(AgentConfig{
		OpenAIAPIKey:    "bad",
		OpenAIBaseURL:   openai.URL,
		OpenListBaseURL: openlist.URL,
		OpenListToken:   "t",
	}, NewRegistry())
	body, _ := json.Marshal(TestRequest{})
	req := httptest.NewRequest("POST", "/api/agent/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleTest(rec, req)
	var resp TestResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.OpenAIOk {
		t.Errorf("openai should NOT be ok on 401")
	}
	if !resp.OpenListOk {
		t.Errorf("openlist should still be ok, got %+v", resp)
	}
	if !strings.Contains(resp.OpenAIError, "401") {
		t.Errorf("openai error: %q", resp.OpenAIError)
	}
}

// TestHandleTest_BothMissingKey exercises the "no key
// configured" path.
func TestHandleTest_BothMissingKey(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	body, _ := json.Marshal(TestRequest{})
	req := httptest.NewRequest("POST", "/api/agent/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleTest(rec, req)
	var resp TestResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.OpenAIOk || resp.OpenListOk {
		t.Errorf("expected both not-ok, got %+v", resp)
	}
	if resp.Errors["openai"] == "" || resp.Errors["openlist"] == "" {
		t.Errorf("expected both error messages, got %+v", resp.Errors)
	}
}

// TestHandleTest_BodyOverridesConfig confirms that a freshly
// typed key in the request body is used instead of the
// configured one.
func TestHandleTest_BodyOverridesConfig(t *testing.T) {
	gotToken := ""
	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer openai.Close()
	openlist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer openlist.Close()

	a := NewAgent(AgentConfig{
		OpenAIAPIKey:    "config-key",
		OpenAIBaseURL:   openai.URL,
		OpenListBaseURL: openlist.URL,
		OpenListToken:   "config-tok",
	}, NewRegistry())
	body, _ := json.Marshal(TestRequest{
		OpenAIKey:     "body-key",
		OpenListToken: "body-tok",
	})
	req := httptest.NewRequest("POST", "/api/agent/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.HandleTest(rec, req)
	if !strings.HasSuffix(gotToken, "body-key") {
		t.Errorf("expected body-key in Authorization, got %q", gotToken)
	}
}

// TestHandleTest_5sTimeout exercises the 5s context.Timeout
// assertion by pointing at an unresponsive server.
func TestHandleTest_5sTimeout(t *testing.T) {
	// Server that hangs forever. The handler must time out
	// after 5s.
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer hang.Close()
	a := NewAgent(AgentConfig{
		OpenAIAPIKey:    "k",
		OpenAIBaseURL:   hang.URL,
		OpenListBaseURL: hang.URL,
		OpenListToken:   "t",
	}, NewRegistry())
	body, _ := json.Marshal(TestRequest{})
	req := httptest.NewRequest("POST", "/api/agent/test", bytes.NewReader(body))

	start := time.Now()
	rec := httptest.NewRecorder()
	a.HandleTest(rec, req)
	elapsed := time.Since(start)

	// We accept anything between 4 and 7 seconds to avoid
	// flakiness.
	if elapsed > 7*time.Second {
		t.Errorf("test handler took too long: %v", elapsed)
	}
	if elapsed < 4*time.Second {
		t.Errorf("test handler did not wait for timeout: %v", elapsed)
	}
}

// TestHandleTest_GETAlsoSupported ensures the GET form of the
// endpoint works for debugging.
func TestHandleTest_GETAlsoSupported(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	rec := httptest.NewRecorder()
	a.HandleTest(rec, httptest.NewRequest("GET", "/api/agent/test", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET should be supported, got %d", rec.Code)
	}
}

func TestHandleTest_RejectsOtherMethods(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	rec := httptest.NewRecorder()
	a.HandleTest(rec, httptest.NewRequest("DELETE", "/api/agent/test", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// TestStreamSSE_ContextCancellation confirms that a cancelled
// request context makes the handler return promptly.
func TestStreamSSE_ContextCancellation(t *testing.T) {
	a := makeFakeAgent(t, []parsedDelta{{Text: "hi", Finished: true}})
	body, _ := json.Marshal(ChatRequest{
		SessionID: "s",
		Messages:  []map[string]any{{"role": "user", "content": "hi"}},
	})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	cancel() // cancel before invoking; handler should exit fast
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.HandleChat(rec, req)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("HandleChat did not return after context cancel")
	}
}
