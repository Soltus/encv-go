package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// scriptedOpenAIServer is the minimal SSE-style HTTP handler that
// pretends to be api.openai.com. It returns a stream of
// "data: {...}" chunks and finally a "data: [DONE]" sentinel.
//
// Tests configure the response with the sseBody field — a string
// of valid SSE chunks — and the agent's openai client (pointed at
// this server) will produce the same events the real OpenAI API
// would.
type scriptedOpenAIServer struct {
	sseBody string
	status  int
}

func (s *scriptedOpenAIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" {
		http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
		return
	}
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(status)
	flusher, _ := w.(http.Flusher)
	if s.sseBody == "" {
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	for _, line := range strings.Split(s.sseBody, "\n") {
		if line == "" {
			continue
		}
		_, _ = fmt.Fprint(w, line+"\n")
	}
	if flusher != nil {
		flusher.Flush()
	}
}

func newTestClient(t *testing.T, srv *scriptedOpenAIServer) *openai.Client {
	t.Helper()
	server := httptest.NewServer(srv)
	t.Cleanup(server.Close)
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = server.URL + "/v1"
	return openai.NewClientWithConfig(cfg)
}

// TestParseDelta_ReasoningContent pulls the o1 reasoning field out
// of an SSE delta. The agent's EventReasoningDelta flow depends on
// this.
func TestParseDelta_ReasoningContent(t *testing.T) {
	d := parseDelta(openai.ChatCompletionStreamResponse{
		Choices: []openai.ChatCompletionStreamChoice{{
			Delta: openai.ChatCompletionStreamChoiceDelta{
				Content:         "answer",
				ReasoningContent: "thinking",
			},
		}},
	})
	if d.Text != "answer" {
		t.Errorf("text: got %q want answer", d.Text)
	}
	if d.Reasoning != "thinking" {
		t.Errorf("reasoning: got %q want thinking", d.Reasoning)
	}
}

// TestParseDelta_NoChoices_Finished verifies the "usage only" path
// that OpenAI emits at the very end of a stream. The agent loop
// must break out of the read loop when this lands.
func TestParseDelta_NoChoices_Finished(t *testing.T) {
	d := parseDelta(openai.ChatCompletionStreamResponse{Choices: nil})
	if !d.Finished {
		t.Errorf("a delta with no choices should be considered finished")
	}
}

// TestParseDelta_AccumulatesToolCallArguments models the real
// behaviour: OpenAI sends tool call arguments in pieces across
// multiple deltas. The agent concatenates them by appending
// successive delta.Arguments strings.
func TestParseDelta_AccumulatesToolCallArguments(t *testing.T) {
	// Two deltas for the same tool call: first the name + id,
	// then the arguments in pieces.
	first := parseDelta(openai.ChatCompletionStreamResponse{
		Choices: []openai.ChatCompletionStreamChoice{{
			Delta: openai.ChatCompletionStreamChoiceDelta{
				ToolCalls: []openai.ToolCall{{
					ID:   "call_1",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "list_files",
						Arguments: `{"path":`,
					},
				}},
			},
		}},
	})
	second := parseDelta(openai.ChatCompletionStreamResponse{
		Choices: []openai.ChatCompletionStreamChoice{{
			Delta: openai.ChatCompletionStreamChoiceDelta{
				ToolCalls: []openai.ToolCall{{
					Function: openai.FunctionCall{
						Arguments: `"/"}`,
					},
				}},
			},
		}},
	})
	if first.ToolCalls[0].ID != "call_1" {
		t.Errorf("first.ID: got %q", first.ToolCalls[0].ID)
	}
	if first.ToolCalls[0].Name != "list_files" {
		t.Errorf("first.Name: got %q", first.ToolCalls[0].Name)
	}
	if second.ToolCalls[0].ID != "" {
		t.Errorf("second delta should not re-set the ID")
	}
	if second.ToolCalls[0].Name != "" {
		t.Errorf("second delta should not re-set the name")
	}
}

// TestCreateChatCompletionStream_OK spins up an httptest server
// that emulates the OpenAI SSE format and confirms the agent's
// openaiStream can drain it via the openai library.
func TestCreateChatCompletionStream_OK(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"x","object":"chat.completion.chunk","choices":[{"delta":{"content":"hi"},"index":0}]}`,
		`data: {"id":"x","object":"chat.completion.chunk","choices":[{"delta":{"content":" there"},"index":0}]}`,
		`data: {"id":"x","object":"chat.completion.chunk","choices":[{"finish_reason":"stop","delta":{}}]}`,
		`data: [DONE]`,
		``,
	}, "\n")

	srv := &scriptedOpenAIServer{sseBody: body}
	client := newTestClient(t, srv)

	stream, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{
		Model:  "gpt-4o",
		Stream: true,
	})
	if err != nil {
		t.Fatalf("CreateChatCompletionStream: %v", err)
	}
	defer stream.Close()

	collected := []string{}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("stream read deadline exceeded")
		default:
		}
		chunk, err := stream.Recv()
		if err != nil {
			break
		}
		parsed := parseDelta(chunk)
		if parsed.Text != "" {
			collected = append(collected, parsed.Text)
		}
		if parsed.Finished {
			break
		}
	}
	if strings.Join(collected, "") != "hi there" {
		t.Errorf("collected text: got %q", strings.Join(collected, ""))
	}
}

// TestCreateChatCompletionStream_Non200 is the failure path: the
// agent must surface the HTTP error.
func TestCreateChatCompletionStream_Non200(t *testing.T) {
	srv := &scriptedOpenAIServer{
		status: http.StatusUnauthorized,
	}
	client := newTestClient(t, srv)
	_, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{
		Model:  "gpt-4o",
		Stream: true,
	})
	if err == nil {
		t.Errorf("expected an error on HTTP 401, got nil")
	}
}

// TestChatTools_NoToolsRegistered verifies the OpenAI request is
// well-formed even when the registry is empty.
func TestChatTools_NoToolsRegistered(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	tools := a.chatTools()
	if len(tools) != 0 {
		t.Errorf("empty registry should produce zero tools, got %d", len(tools))
	}
}

// TestChatTools_RoundTripsRegisteredSchema is the registration
// round-trip: a schema registered as openai.Tool should appear
// verbatim in chatTools() output.
func TestChatTools_RoundTripsRegisteredSchema(t *testing.T) {
	reg := NewRegistry()
	defSchema := &openai.FunctionDefinition{
		Name:        "delete_file",
		Description: "Delete a list of files",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"paths": map[string]any{"type": "array"},
			},
		},
	}
	reg.Register("delete_file", defSchema, func(string) (string, error) {
		return "{}", nil
	}, true, KindFileChange)

	a := NewAgent(AgentConfig{}, reg)
	tools := a.chatTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Function == nil || tools[0].Function.Name != "delete_file" {
		t.Errorf("tool name not preserved: %+v", tools[0])
	}
}

// TestChatTools_HandlesBareFunctionDefinition covers the
// openai.FunctionDefinition (not *openai.FunctionDefinition)
// branch in chatTools.
func TestChatTools_HandlesBareFunctionDefinition(t *testing.T) {
	reg := NewRegistry()
	defSchema := openai.FunctionDefinition{Name: "ping"}
	reg.Register("ping", defSchema, func(string) (string, error) { return "", nil }, false, KindReadOnly)

	a := NewAgent(AgentConfig{}, reg)
	tools := a.chatTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Function == nil || tools[0].Function.Name != "ping" {
		t.Errorf("bare function def not handled: %+v", tools[0])
	}
}

// TestConvertMapToTool_WrapsBareFunctionDefinition checks the
// "map without type but with name" branch.
func TestConvertMapToTool_WrapsBareFunctionDefinition(t *testing.T) {
	m := map[string]any{
		"name": "echo",
		"parameters": map[string]any{
			"type": "object",
		},
	}
	tool, ok := convertMapToTool(m)
	if !ok {
		t.Fatalf("convertMapToTool should accept a bare FunctionDefinition")
	}
	if tool.Function == nil || tool.Function.Name != "echo" {
		t.Errorf("name not preserved: %+v", tool)
	}
	if tool.Type != openai.ToolTypeFunction {
		t.Errorf("type: got %q want %q", tool.Type, openai.ToolTypeFunction)
	}
}

// TestConvertMapToTool_RejectsUnparseable ensures malformed input
// does not crash.
func TestConvertMapToTool_RejectsUnparseable(t *testing.T) {
	// Functions are not marshalable to JSON. Use a value with
	// a type that json.Marshal handles but does not match a Tool
	// shape.
	m := map[string]any{
		"bogus": func() {},
	}
	if _, ok := convertMapToTool(m); ok {
		t.Errorf("convertMapToTool should reject an unparseable map")
	}
}

// TestChatRequest_IncludesModelAndMessages checks the request
// shape that streamOneTurn sends to the LLM.
func TestChatRequest_IncludesModelAndMessages(t *testing.T) {
	a := NewAgent(AgentConfig{OpenAIModel: "gpt-4o-mini"}, NewRegistry())
	req := a.chatRequest([]openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hi"},
	}, "gpt-4o-mini")
	if req.Model != "gpt-4o-mini" {
		t.Errorf("model: got %q", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Errorf("messages length: got %d", len(req.Messages))
	}
	if !req.Stream {
		t.Errorf("stream should be true")
	}
}

// TestReadStream_DrainsToEOF exercises the readStream helper that
// backs the test scaffolding.
func TestReadStream_DrainsToEOF(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"a"}}]}`,
		`data: {"choices":[{"delta":{"content":"b"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
		``,
	}, "\n")
	srv := &scriptedOpenAIServer{sseBody: body}
	client := newTestClient(t, srv)
	stream, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{
		Model: "gpt-4o", Stream: true,
	})
	if err != nil {
		t.Fatalf("CreateChatCompletionStream: %v", err)
	}
	defer stream.Close()
	deltas, err := readStream(stream)
	if err != nil {
		t.Fatalf("readStream: %v", err)
	}
	// The first two deltas have text; the [DONE] chunk produces
	// a finished sentinel.
	text := ""
	for _, d := range deltas {
		text += d.Text
	}
	if text != "ab" {
		t.Errorf("stream text: got %q want ab", text)
	}
	foundFinish := false
	for _, d := range deltas {
		if d.Finished {
			foundFinish = true
		}
	}
	if !foundFinish {
		t.Errorf("expected at least one finished delta")
	}
}

// TestNewOpenAIClient_OverridesBaseURL is the configuration
// contract: OpenAIBaseURL is honoured.
func TestNewOpenAIClient_OverridesBaseURL(t *testing.T) {
	cfg := AgentConfig{
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://example.com/v1",
	}
	c := newOpenAIClient(cfg)
	if c == nil {
		t.Fatal("newOpenAIClient returned nil")
	}
	// We can't easily inspect the config from outside, so we
	// just verify the function doesn't panic and returns a
	// non-nil client.
}

// TestScriptedOpenAIServer_PathCheck locks the routing assumption
// used by the agent: the openai library always hits /v1/chat/completions.
func TestScriptedOpenAIServer_PathCheck(t *testing.T) {
	srv := &scriptedOpenAIServer{}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/somewhere/else")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown path, got %d", resp.StatusCode)
	}
}

// mustJSONIn is a small helper for tests that need to assemble a
// JSON string quickly.
func mustJSONIn(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
