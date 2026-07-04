package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	oa "github.com/sashabaranov/go-openai"

	"github.com/encv/agent/ai"
	"github.com/encv/agent/ai/anthropic"
	"github.com/encv/agent/ai/gemini"
	aiprovopenai "github.com/encv/agent/ai/openai"
)

// TestProviderInterfaceImplementations is the compile-time
// lock: every concrete backend the agent ships MUST satisfy
// [ai.Provider]. Adding a new backend (e.g. a self-hosted
// llama.cpp shim) without the appropriate methods is a build
// failure, not a runtime panic.
func TestProviderInterfaceImplementations(t *testing.T) {
	var _ ai.Provider = (*aiprovopenai.Provider)(nil)
	var _ ai.Provider = (*anthropic.Provider)(nil)
	var _ ai.Provider = (*gemini.Provider)(nil)
}

// TestProviderNames verifies the Name() contract: every
// backend returns a non-empty, lowercase, whitespace-free
// identifier. The string ends up in log lines and metrics
// labels, so drift on it would be visible.
func TestProviderNames(t *testing.T) {
	cases := []struct {
		name string
		p    ai.Provider
		want string
	}{
		{"openai", aiprovopenai.New(nil), "openai"},
		{"anthropic", anthropic.New("", ""), "anthropic"},
		{"gemini", gemini.New("", ""), "gemini"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.p.Name()
			if got != c.want {
				t.Errorf("Name() = %q, want %q", got, c.want)
			}
			if got == "" {
				t.Errorf("Name() is empty")
			}
			if strings.ToLower(got) != got {
				t.Errorf("Name() = %q is not lowercase", got)
			}
			if strings.ContainsAny(got, " \t\n") {
				t.Errorf("Name() = %q contains whitespace", got)
			}
		})
	}
}

// TestOpenAIProvider_StreamChat_NilClient locks the "no
// client wired" failure mode. A Provider that is happy to
// start a stream without a real client is a bug — the
// failure should surface synchronously so the caller can
// surface it to the user.
func TestOpenAIProvider_StreamChat_NilClient(t *testing.T) {
	p := aiprovopenai.New(nil)
	ch, err := p.StreamChat(context.Background(), ai.StreamRequest{Model: "gpt-4"})
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}
	if ch != nil {
		t.Errorf("expected nil channel, got %v", ch)
	}
}

// TestOpenAIProvider_StreamChat_MissingModel locks the
// "no model" failure mode. The openai library's underlying
// API rejects empty model strings, and surfacing the error
// at the Provider boundary (rather than mid-stream) is the
// expected contract.
func TestOpenAIProvider_StreamChat_MissingModel(t *testing.T) {
	p := aiprovopenai.New(oa.NewClient("test-key"))
	_, err := p.StreamChat(context.Background(), ai.StreamRequest{})
	if err == nil {
		t.Fatal("expected error for empty model, got nil")
	}
}

// TestOpenAIProvider_StreamChat_ChannelCloses pins the
// channel lifecycle: the returned channel must be non-nil
// and must close (within a reasonable timeout) when the
// upstream server is silent. A Provider that returns a
// never-closing channel would deadlock the agent loop.
func TestOpenAIProvider_StreamChat_ChannelCloses(t *testing.T) {
	// Silent server: accepts the request, then
	// immediately closes the connection without
	// emitting any data. The openai library treats
	// the empty stream as a clean end-of-stream.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// No Flush, no chunks. The client reads EOF
		// and the provider closes the channel.
	}))
	defer srv.Close()

	cfg := oa.DefaultConfig("test-key")
	cfg.BaseURL = srv.URL
	p := aiprovopenai.New(oa.NewClientWithConfig(cfg))
	ch, err := p.StreamChat(context.Background(), ai.StreamRequest{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("StreamChat open failed: %v", err)
	}
	if ch == nil {
		t.Fatal("StreamChat returned nil channel")
	}

	// The channel should close within a short
	// window. A Provider that hangs is a test
	// failure; we use a 5s ceiling (well above the
	// httptest server's own 100ms-ish round-trip)
	// so transient slowness does not flake the
	// test.
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline.C:
			t.Fatal("channel did not close within 5s")
		}
	}
}

// TestOpenAIProvider_StreamChat_EmitsDelta exercises the
// happy path against a real openai-shaped SSE server. The
// test asserts that the Provider translates the wire format
// into a non-empty Content / FinishReason sequence on the
// returned channel. We use atomic counters to verify the
// provider actually emitted at least one delta of each
// kind — a Provider that swallows chunks would not be
// caught by the "channel closes" test above.
func TestOpenAIProvider_StreamChat_EmitsDelta(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, line := range []string{
			`data: {"id":"x","object":"chat.completion.chunk","choices":[{"delta":{"content":"hi"},"index":0}]}`,
			`data: {"id":"x","object":"chat.completion.chunk","choices":[{"delta":{"content":" there"},"index":0}]}`,
			`data: {"id":"x","object":"chat.completion.chunk","choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
			`data: [DONE]`,
		} {
			_, _ = w.Write([]byte(line + "\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	cfg := oa.DefaultConfig("test-key")
	cfg.BaseURL = srv.URL
	p := aiprovopenai.New(oa.NewClientWithConfig(cfg))
	ch, err := p.StreamChat(context.Background(), ai.StreamRequest{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("StreamChat open failed: %v", err)
	}

	var (
		gotText      strings.Builder
		gotFinish    string
		deltaCount   int
		finishSeen   bool
	)
	for d := range ch {
		deltaCount++
		if d.Content != "" {
			gotText.WriteString(d.Content)
		}
		if d.FinishReason != "" {
			gotFinish = d.FinishReason
			finishSeen = true
		}
	}

	if deltaCount == 0 {
		t.Fatal("provider emitted zero deltas")
	}
	if gotText.String() != "hi there" {
		t.Errorf("concatenated content = %q, want %q", gotText.String(), "hi there")
	}
	if !finishSeen {
		t.Error("provider never sent a FinishReason")
	}
	if gotFinish != "stop" {
		t.Errorf("FinishReason = %q, want %q", gotFinish, "stop")
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Error("test server never received a request")
	}
}

// TestAnthropicProvider_StreamChat_EmptyAPIKey locks the
// skeleton contract: a Provider without credentials returns
// a non-nil, well-behaved (immediately-closed) channel and
// no error. The agent loop's "channel closed" branch is the
// canonical "stream done" path; the skeleton must trigger
// that branch without raising a flag.
func TestAnthropicProvider_StreamChat_EmptyAPIKey(t *testing.T) {
	p := anthropic.New("", "claude-3-5-sonnet-20240620")
	ch, err := p.StreamChat(context.Background(), ai.StreamRequest{Model: "claude-3-5-sonnet-20240620"})
	if err != nil {
		t.Fatalf("expected nil err for skeleton, got %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	for range ch {
		// Skeleton emits no deltas; the loop
		// should exit immediately on the
		// closed-channel signal.
	}
}

// TestAnthropicProvider_StreamChat_MissingModel locks the
// "no model" failure mode. Unlike the empty-API-key path,
// a missing model is a programmer error: the agent should
// fail fast rather than silently no-op.
func TestAnthropicProvider_StreamChat_MissingModel(t *testing.T) {
	p := anthropic.New("sk-test", "")
	_, err := p.StreamChat(context.Background(), ai.StreamRequest{})
	if err == nil {
		t.Fatal("expected error for empty model, got nil")
	}
}

// TestAnthropicProvider_BuildRequest_Shape exercises the
// wire-format projection: system messages hoist to the
// top-level "system" field, user/assistant messages pass
// through, tools are encoded with input_schema. This is
// the lock for the Anthropic protocol adapter; a future
// change that re-encodes these fields is a test failure.
func TestAnthropicProvider_BuildRequest_Shape(t *testing.T) {
	p := anthropic.New("sk-test", "claude-3-5-sonnet-20240620")
	p.MaxTokens = 0 // exercise the default

	tools := []ai.ToolSpec{{
		Name:        "get_weather",
		Description: "Get the weather for a city.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
	}}
	got, err := p.BuildRequest(ai.StreamRequest{
		Model:    "claude-3-5-sonnet-20240620",
		Messages: []ai.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "What is the weather in Tokyo?"},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got.Model != "claude-3-5-sonnet-20240620" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-3-5-sonnet-20240620")
	}
	if got.System != "You are helpful." {
		t.Errorf("System = %q, want %q (hoisted from messages)", got.System, "You are helpful.")
	}
	if got.MaxTokens != anthropic.DefaultMaxTokens {
		t.Errorf("MaxTokens = %d, want %d (default)", got.MaxTokens, anthropic.DefaultMaxTokens)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("Messages = %d entries, want 1 (system hoisted out)", len(got.Messages))
	}
	if got.Messages[0].Role != "user" || got.Messages[0].Content != "What is the weather in Tokyo?" {
		t.Errorf("Messages[0] = %+v, want user/Tokyo", got.Messages[0])
	}
	if len(got.Tools) != 1 {
		t.Fatalf("Tools = %d entries, want 1", len(got.Tools))
	}
	if got.Tools[0].Name != "get_weather" {
		t.Errorf("Tools[0].Name = %q, want %q", got.Tools[0].Name, "get_weather")
	}
	if string(got.Tools[0].InputSchema) == "" || string(got.Tools[0].InputSchema) == "{}" {
		t.Errorf("Tools[0].InputSchema = %q, want the supplied schema", string(got.Tools[0].InputSchema))
	}
}

// TestGeminiProvider_StreamChat_EmptyAPIKey mirrors the
// Anthropic skeleton test. The Gemini provider's no-credentials
// path is identical by design: a closed channel, no error.
func TestGeminiProvider_StreamChat_EmptyAPIKey(t *testing.T) {
	p := gemini.New("", "gemini-1.5-pro")
	ch, err := p.StreamChat(context.Background(), ai.StreamRequest{Model: "gemini-1.5-pro"})
	if err != nil {
		t.Fatalf("expected nil err for skeleton, got %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	for range ch {
	}
}

// TestGeminiProvider_StreamChat_MissingModel mirrors
// Anthropic's "no model" failure mode: a programmer error
// surfaces synchronously.
func TestGeminiProvider_StreamChat_MissingModel(t *testing.T) {
	p := gemini.New("test-key", "")
	_, err := p.StreamChat(context.Background(), ai.StreamRequest{})
	if err == nil {
		t.Fatal("expected error for empty model, got nil")
	}
}

// TestGeminiProvider_BuildRequest_Shape exercises the
// wire-format projection: system messages hoist to a
// top-level systemInstruction object, user/assistant
// messages map to contents with the Gemini role names
// ("user" / "model"), tools encode as
// functionDeclarations, and generationConfig carries
// maxOutputTokens / temperature.
func TestGeminiProvider_BuildRequest_Shape(t *testing.T) {
	p := gemini.New("test-key", "gemini-1.5-pro")

	got, err := p.BuildRequest(ai.StreamRequest{
		Model: "gemini-1.5-pro",
		Messages: []ai.Message{
			{Role: "system", Content: "Be concise."},
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
		},
		Tools: []ai.ToolSpec{{
			Name:        "echo",
			Description: "echo input",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
		MaxTokens:   256,
		Temperature: 0.5,
	})
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got.SystemInstruction == nil {
		t.Fatal("SystemInstruction is nil, want hoisted system message")
	}
	if len(got.SystemInstruction.Parts) != 1 || got.SystemInstruction.Parts[0].Text != "Be concise." {
		t.Errorf("SystemInstruction = %+v, want {Be concise.}", got.SystemInstruction)
	}
	if len(got.Contents) != 2 {
		t.Fatalf("Contents = %d entries, want 2 (user + assistant)", len(got.Contents))
	}
	if got.Contents[0].Role != "user" {
		t.Errorf("Contents[0].Role = %q, want %q", got.Contents[0].Role, "user")
	}
	if got.Contents[1].Role != "model" {
		t.Errorf("Contents[1].Role = %q, want %q (assistant remapped to model)", got.Contents[1].Role, "model")
	}
	if len(got.Tools) != 1 || len(got.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("Tools = %+v, want 1 functionDeclaration", got.Tools)
	}
	if got.Tools[0].FunctionDeclarations[0].Name != "echo" {
		t.Errorf("FunctionDecl.Name = %q, want %q", got.Tools[0].FunctionDeclarations[0].Name, "echo")
	}
	if got.GenerationConfig == nil {
		t.Fatal("GenerationConfig is nil, want MaxOutputTokens / Temperature forwarded")
	}
	if got.GenerationConfig.MaxOutputTokens != 256 {
		t.Errorf("MaxOutputTokens = %d, want 256", got.GenerationConfig.MaxOutputTokens)
	}
	if got.GenerationConfig.Temperature != 0.5 {
		t.Errorf("Temperature = %v, want 0.5", got.GenerationConfig.Temperature)
	}
}

// TestStreamRequest_ZeroValue is a tiny guardrail: an
// [ai.StreamRequest] zero value must be a usable value
// (not a typed nil that breaks later reflection). The
// openai provider's StreamChat test for "missing model"
// exercises this indirectly; this test makes the
// intent explicit.
func TestStreamRequest_ZeroValue(t *testing.T) {
	var req ai.StreamRequest
	if req.Model != "" {
		t.Errorf("zero-value Model = %q, want empty", req.Model)
	}
	if req.Messages != nil {
		t.Errorf("zero-value Messages = %v, want nil", req.Messages)
	}
	if req.Tools != nil {
		t.Errorf("zero-value Tools = %v, want nil", req.Tools)
	}
}

// TestDelta_ZeroValue mirrors TestStreamRequest_ZeroValue
// for the [ai.Delta] side of the contract. A zero Delta
// must be the "no payload" sentinel; a Provider that
// emits a zero Delta for "I have nothing to say" should
// be the canonical no-op path.
func TestDelta_ZeroValue(t *testing.T) {
	var d ai.Delta
	if d.Content != "" || d.Reasoning != "" || d.FinishReason != "" {
		t.Errorf("zero-value Delta has unexpected content: %+v", d)
	}
	if d.ToolCall != nil {
		t.Errorf("zero-value Delta has a non-nil ToolCall: %+v", d.ToolCall)
	}
}

// errProvider is a test-only [ai.Provider] that returns
// the configured error from StreamChat. It exists so
// future tests can inject a deterministic failure without
// standing up an httptest server.
type errProvider struct {
	name string
	err  error
}

func (p *errProvider) Name() string { return p.name }
func (p *errProvider) StreamChat(ctx context.Context, req ai.StreamRequest) (<-chan ai.Delta, error) {
	return nil, p.err
}

// TestErrProvider_SatisfiesInterface is a compile-time
// guard for the test-only errProvider. If this test file
// no longer compiles after a future refactor, the agent
// loop's error-handling assumptions are at risk.
func TestErrProvider_SatisfiesInterface(t *testing.T) {
	var _ ai.Provider = &errProvider{err: errors.New("test")}
}
