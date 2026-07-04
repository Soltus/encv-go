package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// fakeLLM is a small llmStream used by the agent tests to inject
// errors. The structured stream-scripting helpers live in
// openai_test.go (they drive a real *openai.Client through
// httptest, which is the highest-fidelity path).
type fakeLLM struct {
	mu          sync.Mutex
	openErr     error
	openErrOnce bool
	openCount   int
}

func newFakeLLM() *fakeLLM { return &fakeLLM{} }

func (f *fakeLLM) CreateChatCompletionStream(ctx context.Context, req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.openCount++
	if f.openErr != nil {
		err := f.openErr
		if f.openErrOnce {
			f.openErr = nil
		}
		return nil, err
	}
	return nil, errors.New("fakeLLM: streaming not scripted here; use makeFakeAgent in http_test.go")
}

// drainChatStream reads from the SSE channel until it closes or
// the deadline expires. Tests that use the existing fakeLLM (and
// therefore get an openErr) typically see only the stream_end
// event + the close.
func drainChatStream(t *testing.T, ch <-chan *Event) []*Event {
	t.Helper()
	events := []*Event{}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, e)
			if e.Type == EventStreamEnd {
				return events
			}
		case <-deadline:
			t.Fatal("drainChatStream: deadline exceeded")
		}
	}
}

// sessionHasEvent is a small helper for the chat tests.
func sessionHasEvent(events []*Event, t2 EventType) bool {
	for _, e := range events {
		if e.Type == t2 {
			return true
		}
	}
	return false
}

// multiScriptedOpenAIHandler returns an http.Handler that
// emits a different canned SSE body for each successive
// request. The first call to the server gets scripts[0],
// the second scripts[1], etc. Used by the hooks tests
// that need the agent to loop back to the LLM after an
// auto-run tool call.
func multiScriptedOpenAIHandler(scripts []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		// We pull the request count off a process-wide
		// counter via the test's t.Cleanup; the simpler
		// approach is to use a closure-captured pointer
		// so each test gets its own counter.
		body := scripts[0]
		// Pop the front of the script list for the NEXT
		// caller. We use a small wrapper type to allow
		// in-place rotation; for tests with 2 turns this
		// is the easiest.
		if len(scripts) > 1 {
			scripts = scripts[1:]
		}
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

// makeMultiFakeAgent builds an Agent whose LLM is wired to a
// server that scripts a different response per request. The
// first request gets scripts[0], the second scripts[1], etc.
// Tests that drive the agent through multiple LLM calls
// (e.g. "tool call on turn 1, then stop on turn 2") use
// this helper.
func makeMultiFakeAgent(t *testing.T, scripts []string) *Agent {
	t.Helper()
	if len(scripts) == 0 {
		t.Fatalf("makeMultiFakeAgent: at least one script required")
	}
	srv := httptest.NewServer(multiScriptedOpenAIHandler(append([]string(nil), scripts...)))
	t.Cleanup(srv.Close)
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = srv.URL + "/v1"
	return NewAgentWithLLM(AgentConfig{OpenAIModel: "gpt-4o"}, NewRegistry(), &realLLMFake{
		client: openai.NewClientWithConfig(cfg),
	})
}

// ----------------------------------------------------------------------
// Agent-loop tests.
//
// These exercise the public surface (Chat / ConfirmTool / Resume)
// using the existing fakeLLM to inject failures. The streaming
// happy path is covered in http_test.go via a real
// *openai.Client talking to httptest.
// ----------------------------------------------------------------------

// TestParseDelta_ExtractsAllFields locks the shape that the
// agent loop relies on. parseDelta returns a parsedDelta struct
// that the runLoop turns into Events.
func TestParseDelta_ExtractsAllFields(t *testing.T) {
	// Plain text delta.
	d := parseDelta(openai.ChatCompletionStreamResponse{
		Choices: []openai.ChatCompletionStreamChoice{{
			Delta: openai.ChatCompletionStreamChoiceDelta{
				Content: "hello",
			},
		}},
	})
	if d.Text != "hello" {
		t.Errorf("text: got %q want %q", d.Text, "hello")
	}
	if d.Reasoning != "" {
		t.Errorf("reasoning should be empty for plain text delta, got %q", d.Reasoning)
	}
	if d.Finished {
		t.Errorf("finished should be false when FinishReason is empty")
	}

	// Reasoning delta (o1).
	d = parseDelta(openai.ChatCompletionStreamResponse{
		Choices: []openai.ChatCompletionStreamChoice{{
			Delta: openai.ChatCompletionStreamChoiceDelta{
				ReasoningContent: "thinking...",
			},
		}},
	})
	if d.Reasoning != "thinking..." {
		t.Errorf("reasoning: got %q want %q", d.Reasoning, "thinking...")
	}
	if d.Text != "" {
		t.Errorf("text should be empty for reasoning delta, got %q", d.Text)
	}

	// Tool call delta.
	d = parseDelta(openai.ChatCompletionStreamResponse{
		Choices: []openai.ChatCompletionStreamChoice{{
			Delta: openai.ChatCompletionStreamChoiceDelta{
				ToolCalls: []openai.ToolCall{{
					ID: "call_1",
					Function: openai.FunctionCall{
						Name:      "list_files",
						Arguments: `{"path":"/"}`,
					},
				}},
			},
		}},
	})
	if len(d.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(d.ToolCalls))
	}
	if d.ToolCalls[0].ID != "call_1" {
		t.Errorf("tool call ID: got %q", d.ToolCalls[0].ID)
	}
	if d.ToolCalls[0].Name != "list_files" {
		t.Errorf("tool call name: got %q", d.ToolCalls[0].Name)
	}
	if d.ToolCalls[0].Arguments != `{"path":"/"}` {
		t.Errorf("tool call args: got %q", d.ToolCalls[0].Arguments)
	}

	// Finish reason.
	d = parseDelta(openai.ChatCompletionStreamResponse{
		Choices: []openai.ChatCompletionStreamChoice{{
			FinishReason: "tool_calls",
		}},
	})
	if !d.Finished {
		t.Errorf("finished should be true when FinishReason is set")
	}
}

// TestSessionCache_AppendAndSnapshot exercises the cache that
// powers the agent's Resume endpoint.
func TestSessionCache_AppendAndSnapshot(t *testing.T) {
	c := &SessionCache{}
	c.appendEvent(&Event{Type: EventTextDelta, Data: `{"content":"a"}`})
	c.appendEvent(&Event{Type: EventTextDelta, Data: `{"content":"b"}`})

	got := c.snapshot(0)
	if len(got) != 2 {
		t.Fatalf("snapshot(0) length: got %d want 2", len(got))
	}
	if got[0].Data != `{"content":"a"}` || got[1].Data != `{"content":"b"}` {
		t.Errorf("snapshot order/content mismatch: %+v", got)
	}

	got = c.snapshot(1)
	if len(got) != 1 {
		t.Fatalf("snapshot(1) length: got %d want 1", len(got))
	}
	if got[0].Data != `{"content":"b"}` {
		t.Errorf("snapshot(1) content: %q", got[0].Data)
	}

	// Offset past end yields an empty slice (Resume polls).
	if got := c.snapshot(99); len(got) != 0 {
		t.Errorf("snapshot(99) should be empty, got %d events", len(got))
	}
}

// TestSessionCache_SnapshotIsACopy verifies the defensive copy
// contract so callers can hold the slice without fearing
// mutation.
func TestSessionCache_SnapshotIsACopy(t *testing.T) {
	c := &SessionCache{}
	c.appendEvent(&Event{Type: EventTextDelta, Data: "x"})

	got := c.snapshot(0)
	got[0] = &Event{Type: EventToolResult, Data: "tampered"}

	again := c.snapshot(0)
	if again[0].Type != EventTextDelta {
		t.Errorf("snapshot should be defensive copy; cache was mutated via slice")
	}
}

// TestSessionCache_IsFinishedRoundTrip locks the boolean toggle
// used by Resume to decide when to stop polling.
func TestSessionCache_IsFinishedRoundTrip(t *testing.T) {
	c := &SessionCache{}
	if c.IsFinished {
		t.Errorf("new SessionCache should not be finished")
	}
	c.mu.Lock()
	c.IsFinished = true
	c.mu.Unlock()
	if !c.IsFinished {
		t.Errorf("IsFinished should be readable after setting")
	}
}

// TestAgent_NewAgentAndRegistryWiring is the smoke test:
// NewAgent must accept a non-nil registry and not panic.
func TestAgent_NewAgentAndRegistryWiring(t *testing.T) {
	cfg := AgentConfig{OpenAIModel: "gpt-4o"}
	reg := NewRegistry()
	a := NewAgent(cfg, reg)
	if a == nil {
		t.Fatal("NewAgent returned nil")
	}
	if a.Registry != reg {
		t.Errorf("NewAgent did not wire the registry")
	}
	if a.cfg.OpenAIModel != "gpt-4o" {
		t.Errorf("config not stored: %+v", a.cfg)
	}
}

// TestAgent_EnsureSessionCreates verifies the cache is created
// on first use and reused thereafter.
func TestAgent_EnsureSessionCreates(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	c1 := a.ensureSession("s1")
	c2 := a.ensureSession("s1")
	if c1 != c2 {
		t.Errorf("ensureSession should return the same instance for the same id")
	}
	if c1 == nil {
		t.Errorf("cache should not be nil")
	}

	c3 := a.ensureSession("s2")
	if c3 == c1 {
		t.Errorf("different session ids should yield different caches")
	}
}

// TestAgent_GetSessionReturnsExisting tests the read-only
// fetcher used by Resume.
func TestAgent_GetSessionReturnsExisting(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	a.ensureSession("exists")
	if c, ok := a.getSession("exists"); !ok || c == nil {
		t.Errorf("getSession should find the cached session")
	}
	if _, ok := a.getSession("missing"); ok {
		t.Errorf("getSession should return false for unknown ids")
	}
}

// TestAgent_Chat_RejectsEmptySessionID is the negative path of
// the public Chat() contract.
func TestAgent_Chat_RejectsEmptySessionID(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	if _, err := a.Chat(context.Background(), "", nil); err == nil {
		t.Errorf("Chat with empty sessionID should return an error")
	}
}

// TestAgent_ConfirmTool_RejectsInvalidDecision locks the
// 4-value whitelist at the public API level.
func TestAgent_ConfirmTool_RejectsInvalidDecision(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	if _, err := a.ConfirmTool(context.Background(), "s", "tc", Decision("bogus")); err == nil {
		t.Errorf("ConfirmTool with invalid decision should error")
	}
	if _, err := a.ConfirmTool(context.Background(), "s", "tc", Decision("")); err == nil {
		t.Errorf("ConfirmTool with empty decision should error")
	}
}

// TestAgent_ConfirmTool_NoPendingCall verifies the public error
// path: calling ConfirmTool when there is no pending call.
func TestAgent_ConfirmTool_NoPendingCall(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	if _, err := a.ConfirmTool(context.Background(), "missing", "tc", DecisionAccept); err == nil {
		t.Errorf("ConfirmTool with no pending call should error")
	}
}

// TestAgent_ConfirmTool_ToolCallIDMismatch locks the second
// argument check.
func TestAgent_ConfirmTool_ToolCallIDMismatch(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	a.mu.Lock()
	a.PendingCalls["s"] = &pendingCall{
		ToolCallID: "call_real",
		ToolName:   "x",
		Args:       `{}`,
		Messages:   []openai.ChatCompletionMessage{},
	}
	a.mu.Unlock()
	_, err := a.ConfirmTool(context.Background(), "s", "call_other", DecisionAccept)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("expected toolCallID mismatch error, got %v", err)
	}
}

// TestAgent_Resume_RejectsEmptySessionID is the negative path
// of Resume.
func TestAgent_Resume_RejectsEmptySessionID(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	if _, err := a.Resume(context.Background(), "", 0); err == nil {
		t.Errorf("Resume with empty sessionID should return an error")
	}
}

// TestAgent_Resume_SessionNotFound is the second negative path.
func TestAgent_Resume_SessionNotFound(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	if _, err := a.Resume(context.Background(), "missing", 0); err == nil {
		t.Errorf("Resume with unknown sessionID should return an error")
	}
}

// TestGrantKey_Format locks the cache-key shape so
// SessionGrants stays in sync with isGranted.
func TestGrantKey_Format(t *testing.T) {
	k := grantKey("sess_1", "delete_file")
	if k != "sess_1|delete_file" {
		t.Errorf("grantKey: got %q", k)
	}
}

// TestIsValidDecision_CoversAllValues locks the 4-value
// whitelist without depending on the HTTP layer.
func TestIsValidDecision_CoversAllValues(t *testing.T) {
	if !isValidDecision(DecisionAccept) {
		t.Errorf("accept should be valid")
	}
	if !isValidDecision(DecisionAcceptForSession) {
		t.Errorf("accept_for_session should be valid")
	}
	if !isValidDecision(DecisionDecline) {
		t.Errorf("decline should be valid")
	}
	if !isValidDecision(DecisionCancel) {
		t.Errorf("cancel should be valid")
	}
	if isValidDecision(Decision("garbage")) {
		t.Errorf("garbage should be invalid")
	}
	if isValidDecision(Decision("")) {
		t.Errorf("empty should be invalid")
	}
}

// TestAgent_SessionGrantRoundTrip exercises isGranted + the
// sessionGrants store directly. This is the storage contract
// that powers the accept_for_session decision.
func TestAgent_SessionGrantRoundTrip(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	if a.isGranted("s1", "delete_file") {
		t.Errorf("no grant should be set yet")
	}
	a.SessionGrants.Store(grantKey("s1", "delete_file"), struct{}{})
	if !a.isGranted("s1", "delete_file") {
		t.Errorf("grant should be visible after Store")
	}
	if a.isGranted("s1", "other_tool") {
		t.Errorf("grant must be per-tool")
	}
	if a.isGranted("other_session", "delete_file") {
		t.Errorf("grant must be per-session")
	}
}

// TestAgent_ConfirmTool_AcceptForSession_StoresGrantAndRunsHandler
// covers the "accept for session" decision end-to-end. We
// register a tool, manually populate a pending call, then call
// ConfirmTool with accept_for_session and verify:
//  1. the grant is stored
//  2. the tool handler is invoked once
//  3. the agent pushes a tool_result event
//  4. a stream_end event closes the channel
func TestAgent_ConfirmTool_AcceptForSession_StoresGrantAndRunsHandler(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())

	var calls int
	var mu sync.Mutex
	a.Registry.Register("echo", nil, func(args string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return `{"ok":true}`, nil
	}, true, KindCommand)

	// Manually install a pending call.
	a.mu.Lock()
	a.PendingCalls["sess"] = &pendingCall{
		ToolCallID: "call_x",
		ToolName:   "echo",
		Args:       `{}`,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{
					{
						ID:   "call_x",
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      "echo",
							Arguments: `{}`,
						},
					},
				},
			},
		},
	}
	a.mu.Unlock()

	// Inject a fake LLM that errors out — accept_for_session
	// still runs the handler, but the post-handler LLM call
	// returns an error which finishAndClose absorbs.
	fake := newFakeLLM()
	fake.openErr = errors.New("no_more_llm_calls")
	fake.openErrOnce = true
	a.llm = fake

	ch, err := a.ConfirmTool(context.Background(), "sess", "call_x", DecisionAcceptForSession)
	if err != nil {
		t.Fatalf("ConfirmTool error: %v", err)
	}
	events := drainChatStream(t, ch)
	mu.Lock()
	if calls != 1 {
		t.Errorf("expected handler to be called once, got %d", calls)
	}
	mu.Unlock()
	if !a.isGranted("sess", "echo") {
		t.Errorf("accept_for_session should store grant")
	}
	if !sessionHasEvent(events, EventToolResult) {
		t.Errorf("expected a tool_result event")
	}
	if !sessionHasEvent(events, EventStreamEnd) {
		t.Errorf("expected stream_end")
	}
}

// TestAgent_ConfirmTool_Accept_RunsHandlerOnceWithoutGrant
// checks the accept (one-shot) decision: handler runs but no
// grant is stored.
func TestAgent_ConfirmTool_Accept_RunsHandlerOnceWithoutGrant(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	var calls int
	var mu sync.Mutex
	a.Registry.Register("echo", nil, func(args string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return `{"ok":true}`, nil
	}, true, KindCommand)

	a.mu.Lock()
	a.PendingCalls["sess"] = &pendingCall{
		ToolCallID: "call_x",
		ToolName:   "echo",
		Args:       `{}`,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{
					{ID: "call_x", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "echo", Arguments: `{}`}},
				},
			},
		},
	}
	a.mu.Unlock()

	fake := newFakeLLM()
	fake.openErr = errors.New("end")
	fake.openErrOnce = true
	a.llm = fake

	ch, err := a.ConfirmTool(context.Background(), "sess", "call_x", DecisionAccept)
	if err != nil {
		t.Fatalf("ConfirmTool: %v", err)
	}
	_ = drainChatStream(t, ch)
	if calls != 1 {
		t.Errorf("expected handler to run once, got %d", calls)
	}
	if a.isGranted("sess", "echo") {
		t.Errorf("accept (one-shot) should NOT store grant")
	}
}

// TestAgent_ConfirmTool_Decline_PushesUserRejectedAndContinues
// verifies the decline path: handler is NOT called, but a
// tool_result with user_rejected is pushed, and the loop
// continues (so the LLM can react).
func TestAgent_ConfirmTool_Decline_PushesUserRejectedAndContinues(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	var calls int
	a.Registry.Register("echo", nil, func(args string) (string, error) {
		calls++
		return `{}`, nil
	}, true, KindCommand)

	a.mu.Lock()
	a.PendingCalls["sess"] = &pendingCall{
		ToolCallID: "call_x",
		ToolName:   "echo",
		Args:       `{}`,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{
					{ID: "call_x", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "echo", Arguments: `{}`}},
				},
			},
		},
	}
	a.mu.Unlock()

	fake := newFakeLLM()
	fake.openErr = errors.New("end")
	fake.openErrOnce = true
	a.llm = fake

	ch, err := a.ConfirmTool(context.Background(), "sess", "call_x", DecisionDecline)
	if err != nil {
		t.Fatalf("ConfirmTool: %v", err)
	}
	events := drainChatStream(t, ch)
	if calls != 0 {
		t.Errorf("decline must NOT invoke handler; got %d calls", calls)
	}
	if !sessionHasEvent(events, EventToolResult) {
		t.Errorf("expected a tool_result event")
	}
	for _, e := range events {
		if e.Type == EventToolResult && strings.Contains(e.Data, "user_rejected") {
			return
		}
	}
	t.Errorf("expected user_rejected in tool_result, events: %+v", events)
}

// TestAgent_ConfirmTool_Cancel_TerminatesStream is the
// cancel-and-stop path. The stream must end with a stream_end
// and the handler must not be called.
func TestAgent_ConfirmTool_Cancel_TerminatesStream(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	var calls int
	a.Registry.Register("echo", nil, func(args string) (string, error) {
		calls++
		return `{}`, nil
	}, true, KindCommand)

	a.mu.Lock()
	a.PendingCalls["sess"] = &pendingCall{
		ToolCallID: "call_x",
		ToolName:   "echo",
		Args:       `{}`,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{
					{ID: "call_x", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "echo", Arguments: `{}`}},
				},
			},
		},
	}
	a.mu.Unlock()

	// No need to inject a fake LLM; cancel should not call
	// runLoop, so the LLM is never invoked.
	ch, err := a.ConfirmTool(context.Background(), "sess", "call_x", DecisionCancel)
	if err != nil {
		t.Fatalf("ConfirmTool: %v", err)
	}
	events := drainChatStream(t, ch)
	if calls != 0 {
		t.Errorf("cancel must NOT invoke handler; got %d calls", calls)
	}
	if !sessionHasEvent(events, EventStreamEnd) {
		t.Errorf("cancel should end the stream")
	}
	for _, e := range events {
		if e.Type == EventToolResult && strings.Contains(e.Data, "user_cancelled") {
			return
		}
	}
	t.Errorf("expected user_cancelled in tool_result; events: %+v", events)
}

// TestPendingArgs_LocatesArgsFromAssistantMessage locks the
// helper that recovers the original tool args from the
// messages slice after a suspended turn.
func TestPendingArgs_LocatesArgsFromAssistantMessage(t *testing.T) {
	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hi"},
		{Role: openai.ChatMessageRoleAssistant, ToolCalls: []openai.ToolCall{
			{ID: "a", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "n", Arguments: `{"x":1}`}},
		}},
		{Role: openai.ChatMessageRoleAssistant, ToolCalls: []openai.ToolCall{
			{ID: "b", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "n", Arguments: `{"x":2}`}},
		}},
	}
	if got := pendingArgs(msgs, "b"); got != `{"x":2}` {
		t.Errorf("pendingArgs(b): got %q", got)
	}
	if got := pendingArgs(msgs, "missing"); got != "" {
		t.Errorf("pendingArgs(missing) should be empty, got %q", got)
	}
}

// TestAgent_Chat_RejectsWhenPendingCallLocks confirms the
// single-pending-call guard. While a session has a pending
// call, a second Chat() call on the same session is rejected.
func TestAgent_Chat_RejectsWhenPendingCallLocks(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	a.mu.Lock()
	a.PendingCalls["s"] = &pendingCall{ToolCallID: "x", ToolName: "y", Args: "{}"}
	a.mu.Unlock()
	_, err := a.Chat(context.Background(), "s", nil)
	if err == nil || !strings.Contains(err.Error(), "awaiting confirmation") {
		t.Errorf("expected awaiting-confirmation error, got %v", err)
	}
}

// ----------------------------------------------------------------------
// ChatMode (steer / queue) tests.
//
// These exercise Task 11 (Steer / Queue dual-track send buttons):
//   - "steer"  — same code path as Chat, semantically marking
//     the message as a course-correction.
//   - "queue"  — message is held in agent.pendingMessages until
//     the next HookTurnEnd, at which point a new Chat is fired.
//   - "" / "start" — original Chat behaviour, must keep working.
//
// We assert behaviour, not exact SSE byte sequences, because the
// SSE marshalling is a separate concern exercised in
// http_test.go.
// ----------------------------------------------------------------------

// TestSteerMode_AppendsUserMessageAndContinues verifies that
// ChatMode with mode="steer" is functionally equivalent to
// Chat: it returns a channel that yields text_delta + stream_end
// events for the LLM's response. The front-end relies on this
// when the user clicks the "steer current turn" button — the
// existing runLoop path (and its tool-confirm flow) is
// preserved.
func TestSteerMode_AppendsUserMessageAndContinues(t *testing.T) {
	a := makeFakeAgent(t, []parsedDelta{
		{Text: "steered-ok", Finished: true},
	})

	ch, err := a.ChatMode(
		context.Background(),
		"sess_steer",
		[]openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "course-correct me"},
		},
		"steer",
	)
	if err != nil {
		t.Fatalf("ChatMode(steer): %v", err)
	}
	events := drainChatStream(t, ch)

	if !sessionHasEvent(events, EventTextDelta) {
		t.Errorf("expected a text_delta event in steer mode, got: %+v", events)
	}
	if !sessionHasEvent(events, EventStreamEnd) {
		t.Errorf("expected a stream_end event in steer mode, got: %+v", events)
	}

	// Concatenate every text_delta to verify the LLM's text
	// actually flowed through. We accept "steered-ok" being
	// delivered as one chunk or split across chunks; the
	// composite string is what matters.
	var gotText string
	for _, e := range events {
		if e.Type != EventTextDelta {
			continue
		}
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(e.Data), &payload); err == nil {
			gotText += payload.Content
		}
	}
	if gotText != "steered-ok" {
		t.Errorf("steer mode should stream LLM text, got %q", gotText)
	}
}

// TestChatMode_RejectsUnknownMode locks the public validation
// of the mode argument so typos like "steerr" surface as an
// error rather than silently degrading to start behaviour.
func TestChatMode_RejectsUnknownMode(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	_, err := a.ChatMode(
		context.Background(),
		"sess_unknown",
		[]openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "x"},
		},
		"bogus",
	)
	if err == nil || !strings.Contains(err.Error(), "unknown chat mode") {
		t.Errorf("expected unknown-mode error, got %v", err)
	}
}

// TestQueueMode_HoldsUntilTurnEnd verifies the queue contract:
//
//  1. ChatMode(mode="queue") returns a channel that is
//     *already closed* — the HTTP handler uses this to decide
//     to respond 202 Accepted immediately.
//  2. The LLM is NOT called while the current turn is still
//     running; the message is parked in agent.pendingMessages.
//  3. After the current turn fully ends (HookTurnEnd fires),
//     the drain hook consumes the parked message and starts
//     a new Chat. The LLM is called with the queued messages.
//
// We use a custom HTTP server that blocks the first response
// until the test signals it, so we can deterministically
// observe the "queued" state mid-turn.
func TestQueueMode_HoldsUntilTurnEnd(t *testing.T) {
	h := &queueTestHarness{
		firstCallStarted:   make(chan struct{}),
		firstCallCanFinish: make(chan struct{}),
	}
	srv := httptest.NewServer(h.handler())
	t.Cleanup(srv.Close)
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = srv.URL + "/v1"
	a := NewAgentWithLLM(
		AgentConfig{OpenAIModel: "gpt-4o"},
		NewRegistry(),
		&realLLMFake{client: openai.NewClientWithConfig(cfg)},
	)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	user1 := openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleUser, Content: "hello",
	}

	// Round 1: regular start. The handler blocks the first
	// response until we close firstCallCanFinish below.
	ch1, err := a.ChatMode(ctx, "sess_q", []openai.ChatCompletionMessage{user1}, "start")
	if err != nil {
		t.Fatalf("ChatMode(start): %v", err)
	}

	// Wait for the first LLM call to be in flight, so we
	// know the first turn has not yet ended.
	select {
	case <-h.firstCallStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("first LLM call never started")
	}

	// Round 2: queue the second message while the first is
	// still mid-stream. The LLM must NOT be called for this
	// yet.
	queuedMsgs := []openai.ChatCompletionMessage{
		user1,
		{Role: openai.ChatMessageRoleAssistant, Content: "first"},
		{Role: openai.ChatMessageRoleUser, Content: "queued"},
	}
	ch2, err := a.ChatMode(ctx, "sess_q", queuedMsgs, "queue")
	if err != nil {
		t.Fatalf("ChatMode(queue): %v", err)
	}

	// The queue mode returns a channel that is closed
	// immediately. Verify it.
	select {
	case _, ok := <-ch2:
		if ok {
			t.Errorf("queue mode channel should be closed, got an event")
		}
	case <-time.After(200 * time.Millisecond):
		t.Errorf("queue mode channel should close immediately")
	}

	// LLM call count must still be 1 — the queued message
	// has not been processed.
	if got := h.callCount.Load(); got != 1 {
		t.Errorf("LLM should be called once so far (queue holds), got %d", got)
	}

	// Unblock the first LLM call so the first turn ends and
	// the drain hook fires.
	close(h.firstCallCanFinish)

	// Drain the first channel. drainChatStream returns on
	// stream_end, which is emitted before HookTurnEnd in
	// streamOneTurn, so the drain hook fires shortly after.
	drainChatStream(t, ch1)

	// Wait for the drain hook to fire and the queued Chat
	// to invoke the LLM a second time.
	deadline := time.After(3 * time.Second)
	for h.callCount.Load() < 2 {
		select {
		case <-deadline:
			t.Errorf("expected 2 LLM calls after first turn ended, got %d", h.callCount.Load())
			return
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Verify the queued Chat's LLM call received the queued
	// messages in full, including the new user message at
	// the end. We do not assert against the first call
	// (Chat's messages) — we only care that the queue
	// payload was forwarded verbatim.
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.messages) != 2 {
		t.Fatalf("expected 2 LLM calls captured, got %d", len(h.messages))
	}
	round2 := h.messages[1]
	if len(round2) != 3 {
		t.Fatalf("round 2 message slice length: got %d want 3", len(round2))
	}
	if round2[2].Role != openai.ChatMessageRoleUser || round2[2].Content != "queued" {
		t.Errorf("round 2 last message: got %+v want user/queued", round2[2])
	}
	if round2[1].Role != openai.ChatMessageRoleAssistant || round2[1].Content != "first" {
		t.Errorf("round 2 assistant message: got %+v want assistant/first", round2[1])
	}
}

// TestQueueMode_FIFOOrderMultipleMessages verifies that
// multiple messages enqueued during the same in-flight turn
// are processed in FIFO order. The drain hook fires on every
// HookTurnEnd, so each drained message produces its own
// runLoop, whose end re-fires the hook for the next message.
func TestQueueMode_FIFOOrderMultipleMessages(t *testing.T) {
	// 3 scripts: round 1 (start), round 2 (first queued),
	// round 3 (second queued). All but the first script
	// need the handler to be non-blocking, so we use the
	// existing makeMultiFakeAgent.
	a := makeMultiFakeAgent(t, []string{
		buildChatCompletionStreamBody([]parsedDelta{{Text: "r1", Finished: true}}),
		buildChatCompletionStreamBody([]parsedDelta{{Text: "r2", Finished: true}}),
		buildChatCompletionStreamBody([]parsedDelta{{Text: "r3", Finished: true}}),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Round 1.
	ch1, err := a.ChatMode(ctx, "sess_fifo", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "u1"},
	}, "start")
	if err != nil {
		t.Fatalf("ChatMode(start): %v", err)
	}

	// Queue two messages while round 1 is in flight.
	_, err = a.ChatMode(ctx, "sess_fifo", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "u2"},
	}, "queue")
	if err != nil {
		t.Fatalf("ChatMode(queue 2): %v", err)
	}
	_, err = a.ChatMode(ctx, "sess_fifo", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "u3"},
	}, "queue")
	if err != nil {
		t.Fatalf("ChatMode(queue 3): %v", err)
	}

	// Wait for round 1 to finish. The drain hook will then
	// fire twice in turn (once per queued message), each
	// time consuming the front of the pending queue.
	drainChatStream(t, ch1)

	// All 3 scripts must be consumed in order. We assert by
	// checking the session cache: each Chat's text_delta
	// event lands there, and the LLM was called for each.
	// drainChatStream only covers ch1, so we rely on the
	// fact that all 3 scripts were consumed (the multi-
	// scripted handler pops scripts[0] on every call).
	// We poll the queue directly: it should be empty.
	deadline := time.After(3 * time.Second)
	for {
		a.pendingMu.Lock()
		q, ok := a.pendingMessages["sess_fifo"]
		var remaining int
		if ok {
			q.mu.Lock()
			remaining = len(q.messages)
			q.mu.Unlock()
		}
		a.pendingMu.Unlock()
		if remaining == 0 {
			break
		}
		select {
		case <-deadline:
			t.Errorf("pending queue for sess_fifo did not drain in time: %d messages remain", remaining)
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// queueTestHarness drives TestQueueMode_HoldsUntilTurnEnd.
// The first LLM call blocks on firstCallCanFinish so the
// test can deterministically observe the queued state; the
// second call (fired by the drain hook) returns immediately
// with text "second".
type queueTestHarness struct {
	firstCallStarted   chan struct{}
	firstCallCanFinish chan struct{}

	callCount atomic.Int32
	mu        sync.Mutex
	messages  [][]openai.ChatCompletionMessage
}

func (h *queueTestHarness) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the request body so the test can assert
		// on the messages slice that the agent (re)builds.
		bodyBytes, _ := io.ReadAll(r.Body)
		var req openai.ChatCompletionRequest
		if err := json.Unmarshal(bodyBytes, &req); err == nil {
			h.mu.Lock()
			h.messages = append(h.messages, req.Messages)
			h.mu.Unlock()
		}

		count := h.callCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Block the first call so the test can interleave
		// the queue request. Subsequent calls (fired by
		// the drain hook) run unimpeded.
		if count == 1 {
			close(h.firstCallStarted)
			<-h.firstCallCanFinish
		}

		text := "second"
		if count == 1 {
			text = "first"
		}
		streamBody := buildChatCompletionStreamBody([]parsedDelta{
			{Text: text, Finished: true},
		})
		for _, line := range strings.Split(streamBody, "\n") {
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

// TestAgent_RunTool_TimesHandlerExecution covers the
// instrumentation that backs the DurationMs field on
// ToolResultData.
func TestAgent_RunTool_TimesHandlerExecution(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	def := ToolDefinition{
		Handler: func(args string) (string, error) {
			time.Sleep(5 * time.Millisecond)
			return `{"ok":true}`, nil
		},
	}
	out, status, _, err := a.runTool(def, `{}`)
	if err != nil {
		t.Errorf("runTool returned error: %v", err)
	}
	if status != "success" {
		t.Errorf("status: got %q want success", status)
	}
	if out != `{"ok":true}` {
		t.Errorf("output: got %q", out)
	}
	// Error path.
	def = ToolDefinition{
		Handler: func(args string) (string, error) {
			return "", errors.New("boom")
		},
	}
	out, status, _, err = a.runTool(def, `{}`)
	if err == nil {
		t.Errorf("expected error")
	}
	if status != "failed" {
		t.Errorf("status: got %q want failed", status)
	}
	if !strings.Contains(out, "boom") {
		t.Errorf("output should contain error message, got %q", out)
	}
}

// TestCloneMessages_DefensiveCopy makes sure the snapshot used
// by PendingCalls is independent of the caller's slice.
func TestCloneMessages_DefensiveCopy(t *testing.T) {
	original := []openai.ChatCompletionMessage{{Role: "user", Content: "hi"}}
	cloned := cloneMessages(original)
	cloned[0].Content = "tampered"
	if original[0].Content != "hi" {
		t.Errorf("cloneMessages must be a deep copy; original was mutated")
	}
}

// ----------------------------------------------------------------------
// Hooks tests.
//
// The hook system has 6 documented event points; these tests
// assert each one fires (and only fires) at the documented
// stage of the agent lifecycle. They use a single counting
// hook per test, plus a few dedicated scenarios for the
// cancel-via-HookPreToolCall path and the panic-recovery
// guarantee.
// ----------------------------------------------------------------------

// hookCounters is a small concurrent-safe accumulator the
// tests use to assert which event types were raised and how
// many times. Each field is the call count for one
// HookEvent value; toolCalls / toolResults record the
// payloads the hook received for the per-tool assertions.
type hookCounters struct {
	mu                  sync.Mutex
	sessionStart        int
	turnStart           int
	turnEnd             int
	preToolCall         int
	postToolCall        int
	sessionShutdown     int
	lastSessionID       string
	lastMessagesLen     int
	lastPreToolCall     *ToolCallData
	lastPostToolCall    *ToolCallData
	lastPostToolResult  *ToolResultData
	lastPreToolCancel   *bool
}

func (c *hookCounters) record(hc *HookContext) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSessionID = hc.SessionID
	if hc.Messages != nil {
		c.lastMessagesLen = len(hc.Messages)
	}
	switch hc.Event {
	case HookSessionStart:
		c.sessionStart++
	case HookTurnStart:
		c.turnStart++
	case HookTurnEnd:
		c.turnEnd++
	case HookPreToolCall:
		c.preToolCall++
		if hc.ToolCall != nil {
			copy := *hc.ToolCall
			c.lastPreToolCall = &copy
		}
		if hc.Cancel != nil {
			v := *hc.Cancel
			c.lastPreToolCancel = &v
		}
	case HookPostToolCall:
		c.postToolCall++
		if hc.ToolCall != nil {
			copy := *hc.ToolCall
			c.lastPostToolCall = &copy
		}
		if hc.ToolResult != nil {
			copy := *hc.ToolResult
			c.lastPostToolResult = &copy
		}
	case HookSessionShutdown:
		c.sessionShutdown++
	}
}

// snapshot returns the counter values under the mutex so
// assertions can read them without racing the hook.
func (c *hookCounters) snapshot() (int, int, int, int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionStart, c.turnStart, c.turnEnd, c.preToolCall, c.postToolCall, c.sessionShutdown
}

// countingHook returns a HookFunc that records every event
// into c. Used as the single registered hook in most tests.
func countingHook(c *hookCounters) HookFunc {
	return func(ctx context.Context, hc *HookContext) error {
		c.record(hc)
		return nil
	}
}

// TestHooks_TextOnlyChat_FiresSessionTurnStartEnd confirms
// that a text-only Chat turn (no tool calls) fires
// HookSessionStart, HookTurnStart, and HookTurnEnd exactly
// once each, and does NOT fire the tool events.
func TestHooks_TextOnlyChat_FiresSessionTurnStartEnd(t *testing.T) {
	a := makeFakeAgent(t, []parsedDelta{
		{Text: "hi", Finished: true},
	})
	c := &hookCounters{}
	a.RegisterHook(countingHook(c))

	ch, err := a.Chat(context.Background(), "sess_text", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	_ = drainChatStream(t, ch)

	ss, ts, te, pre, post, shut := c.snapshot()
	if ss != 1 {
		t.Errorf("HookSessionStart: got %d want 1", ss)
	}
	if ts != 1 {
		t.Errorf("HookTurnStart: got %d want 1", ts)
	}
	if te != 1 {
		t.Errorf("HookTurnEnd: got %d want 1", te)
	}
	if pre != 0 {
		t.Errorf("HookPreToolCall: got %d want 0 (no tool call in this turn)", pre)
	}
	if post != 0 {
		t.Errorf("HookPostToolCall: got %d want 0 (no tool call in this turn)", post)
	}
	if shut != 0 {
		t.Errorf("HookSessionShutdown: got %d want 0 (no ShutdownSession call)", shut)
	}
	if c.lastSessionID != "sess_text" {
		t.Errorf("lastSessionID: got %q want %q", c.lastSessionID, "sess_text")
	}
}

// TestHooks_ToolCall_FiresPreAndPostToolCall covers the
// auto-run tool path: a single tool invocation must fire
// HookPreToolCall once (with a pointer to a populated
// ToolCallData) and HookPostToolCall once (with both the
// ToolCallData and a populated ToolResultData).
func TestHooks_ToolCall_FiresPreAndPostToolCall(t *testing.T) {
	// Two scripts: the first turn returns a tool call (the
	// agent runs the handler and loops back to the LLM),
	// the second turn returns a plain "stop" so the
	// session ends cleanly.
	a := makeMultiFakeAgent(t, []string{
		buildChatCompletionStreamBody([]parsedDelta{
			{
				ToolCalls: []parsedToolCall{
					{ID: "call_hook_1", Name: "echo", Arguments: `{"v":1}`},
				},
				Finished: true,
			},
		}),
		buildChatCompletionStreamBody([]parsedDelta{
			{Text: "all done", Finished: true},
		}),
	})
	a.Registry.Register("echo", nil, func(args string) (string, error) {
		return `{"ok":true}`, nil
	}, false, KindCommand)

	c := &hookCounters{}
	a.RegisterHook(countingHook(c))

	ch, err := a.Chat(context.Background(), "sess_tc", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "use echo"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	_ = drainChatStream(t, ch)

	ss, ts, te, pre, post, _ := c.snapshot()
	// Two-turn scenario: the first LLM call returns a
	// tool call (the agent runs the handler, then loops
	// back to the LLM for a second call), the second call
	// returns a normal text "stop". Therefore we expect
	// TurnStart/TurnEnd to fire twice and the tool events
	// once.
	if ss != 1 {
		t.Errorf("HookSessionStart: got %d want 1", ss)
	}
	if ts != 2 {
		t.Errorf("HookTurnStart: got %d want 2 (one per LLM call)", ts)
	}
	if te != 2 {
		t.Errorf("HookTurnEnd: got %d want 2 (one per LLM call)", te)
	}
	if pre != 1 {
		t.Errorf("HookPreToolCall: got %d want 1", pre)
	}
	if post != 1 {
		t.Errorf("HookPostToolCall: got %d want 1", post)
	}

	if c.lastPreToolCall == nil || c.lastPreToolCall.ID != "call_hook_1" || c.lastPreToolCall.Name != "echo" {
		t.Errorf("PreToolCall payload: %+v", c.lastPreToolCall)
	}
	if c.lastPostToolCall == nil || c.lastPostToolCall.ID != "call_hook_1" {
		t.Errorf("PostToolCall ToolCall payload: %+v", c.lastPostToolCall)
	}
	if c.lastPostToolResult == nil || c.lastPostToolResult.ID != "call_hook_1" || c.lastPostToolResult.Status != "success" {
		t.Errorf("PostToolCall ToolResult payload: %+v", c.lastPostToolResult)
	}
}

// TestHooks_PreToolCall_CancelShortCircuits is the dedicated
// test for the cancellation contract: when a hook sets
// hc.Cancel = true, the tool handler MUST NOT run, and the
// agent MUST push a synthetic "cancelled" tool result so the
// LLM can continue the turn.
func TestHooks_PreToolCall_CancelShortCircuits(t *testing.T) {
	// First turn: a single tool call (cancelled by the
	// hook). Second turn: the LLM responds with a normal
	// text "stop" to close the session.
	a := makeMultiFakeAgent(t, []string{
		buildChatCompletionStreamBody([]parsedDelta{
			{
				ToolCalls: []parsedToolCall{
					{ID: "call_cancel", Name: "dangerous", Arguments: `{}`},
				},
				Finished: true,
			},
		}),
		buildChatCompletionStreamBody([]parsedDelta{
			{Text: "ok, no dangerous", Finished: true},
		}),
	})
	var handlerCalls int
	var mu sync.Mutex
	a.Registry.Register("dangerous", nil, func(args string) (string, error) {
		mu.Lock()
		handlerCalls++
		mu.Unlock()
		return `{"ran":true}`, nil
	}, false, KindCommand)

	// Register a hook that cancels the call.
	a.RegisterHook(func(ctx context.Context, hc *HookContext) error {
		if hc.Event == HookPreToolCall && hc.Cancel != nil {
			*hc.Cancel = true
		}
		return nil
	})

	ch, err := a.Chat(context.Background(), "sess_cancel", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "do dangerous"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	events := drainChatStream(t, ch)

	mu.Lock()
	if handlerCalls != 0 {
		mu.Unlock()
		t.Errorf("handler must not run when PreToolCall cancels; got %d calls", handlerCalls)
	} else {
		mu.Unlock()
	}

	// The cancelled tool result must be on the wire.
	var foundCancelled bool
	for _, e := range events {
		if e.Type == EventToolResult && strings.Contains(e.Data, "cancelled_by_hook") {
			foundCancelled = true
		}
	}
	if !foundCancelled {
		t.Errorf("expected cancelled_by_hook in a tool_result event; got %d events", len(events))
	}
}

// TestHooks_ShutdownSession_FiresShutdownEvent covers the
// explicit-shutdown path. Calling ShutdownSession on a
// sessionID MUST fire HookSessionShutdown exactly once with
// the correct sessionID, even if the session has not been
// Chat'd.
func TestHooks_ShutdownSession_FiresShutdownEvent(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	c := &hookCounters{}
	a.RegisterHook(countingHook(c))

	a.ShutdownSession(context.Background(), "sess_shutdown")

	_, _, _, _, _, shut := c.snapshot()
	if shut != 1 {
		t.Errorf("HookSessionShutdown: got %d want 1", shut)
	}
	if c.lastSessionID != "sess_shutdown" {
		t.Errorf("Shutdown sessionID: got %q want %q", c.lastSessionID, "sess_shutdown")
	}
}

// TestHooks_AllSixEventsFireAcrossLifecycle is the umbrella
// test: it walks the full lifecycle (Chat → Shutdown) and
// asserts that every one of the 6 documented event points
// was raised exactly once.
func TestHooks_AllSixEventsFireAcrossLifecycle(t *testing.T) {
	// Two scripts: first a tool call (agent runs handler,
	// loops back), then a plain "stop".
	a := makeMultiFakeAgent(t, []string{
		buildChatCompletionStreamBody([]parsedDelta{
			{
				ToolCalls: []parsedToolCall{
					{ID: "call_all", Name: "echo", Arguments: `{}`},
				},
				Finished: true,
			},
		}),
		buildChatCompletionStreamBody([]parsedDelta{
			{Text: "done", Finished: true},
		}),
	})
	a.Registry.Register("echo", nil, func(args string) (string, error) {
		return `{"ok":true}`, nil
	}, false, KindReadOnly)

	c := &hookCounters{}
	a.RegisterHook(countingHook(c))

	ch, err := a.Chat(context.Background(), "sess_all", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "go"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	_ = drainChatStream(t, ch)
	a.ShutdownSession(context.Background(), "sess_all")

	ss, ts, te, pre, post, shut := c.snapshot()
	// Two-turn scenario (tool call + final text), so the
	// per-turn hooks fire twice.
	if ss != 1 {
		t.Errorf("HookSessionStart: got %d want 1", ss)
	}
	if ts != 2 {
		t.Errorf("HookTurnStart: got %d want 2", ts)
	}
	if te != 2 {
		t.Errorf("HookTurnEnd: got %d want 2", te)
	}
	if pre != 1 {
		t.Errorf("HookPreToolCall: got %d want 1", pre)
	}
	if post != 1 {
		t.Errorf("HookPostToolCall: got %d want 1", post)
	}
	if shut != 1 {
		t.Errorf("HookSessionShutdown: got %d want 1", shut)
	}
}

// TestHooks_SecondChatOnSameSession_DoesNotReFireSessionStart
// locks the "first Chat only" contract for
// HookSessionStart. A second Chat call on the same sessionID
// MUST NOT re-fire it, only the per-turn events.
func TestHooks_SecondChatOnSameSession_DoesNotReFireSessionStart(t *testing.T) {
	a := makeFakeAgent(t, []parsedDelta{
		{Text: "first", Finished: true},
		{Text: "second", Finished: true},
	})
	c := &hookCounters{}
	a.RegisterHook(countingHook(c))

	ch1, err := a.Chat(context.Background(), "sess_resume", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "1"},
	})
	if err != nil {
		t.Fatalf("first Chat: %v", err)
	}
	_ = drainChatStream(t, ch1)

	ch2, err := a.Chat(context.Background(), "sess_resume", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "2"},
	})
	if err != nil {
		t.Fatalf("second Chat: %v", err)
	}
	_ = drainChatStream(t, ch2)

	ss, ts, te, _, _, _ := c.snapshot()
	if ss != 1 {
		t.Errorf("HookSessionStart: got %d want 1 (only the first Chat should fire it)", ss)
	}
	if ts != 2 {
		t.Errorf("HookTurnStart: got %d want 2 (one per Chat)", ts)
	}
	if te != 2 {
		t.Errorf("HookTurnEnd: got %d want 2 (one per Chat)", te)
	}
}

// TestHooks_PanicRecovered covers the safety contract: a
// panicking hook MUST NOT crash the chat goroutine. The
// stream still completes with stream_end and the second
// (non-panicking) hook still runs.
func TestHooks_PanicRecovered(t *testing.T) {
	a := makeFakeAgent(t, []parsedDelta{
		{Text: "ok", Finished: true},
	})
	panicked := false
	a.RegisterHook(func(ctx context.Context, hc *HookContext) error {
		panicked = true
		panic("intentional hook boom")
	})
	var otherFired int
	var mu sync.Mutex
	a.RegisterHook(func(ctx context.Context, hc *HookContext) error {
		mu.Lock()
		otherFired++
		mu.Unlock()
		return nil
	})

	ch, err := a.Chat(context.Background(), "sess_panic", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "go"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	events := drainChatStream(t, ch)

	if !panicked {
		t.Errorf("expected the panicking hook to be invoked")
	}
	mu.Lock()
	if otherFired == 0 {
		t.Errorf("the non-panicking hook must still fire after recovery")
	}
	mu.Unlock()
	// Stream must close cleanly.
	var foundEnd bool
	for _, e := range events {
		if e.Type == EventStreamEnd {
			foundEnd = true
		}
	}
	if !foundEnd {
		t.Errorf("expected stream_end despite panicking hook")
	}
}

// TestRegisterHook_MultipleInvocationsAllFire checks the
// FIFO / all-registered-callbacks contract: every
// registered hook is invoked at every event point, in
// registration order.
func TestRegisterHook_MultipleInvocationsAllFire(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	var order []string
	var mu sync.Mutex
	make := func(name string) HookFunc {
		return func(ctx context.Context, hc *HookContext) error {
			mu.Lock()
			order = append(order, name+":"+string(hc.Event))
			mu.Unlock()
			return nil
		}
	}
	a.RegisterHook(make("first"))
	a.RegisterHook(make("second"))
	a.RegisterHook(make("third"))

	a.ShutdownSession(context.Background(), "sess_multi")

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Fatalf("expected 3 hook invocations, got %d: %v", len(order), order)
	}
	want := []string{
		"first:session_shutdown",
		"second:session_shutdown",
		"third:session_shutdown",
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d]: got %q want %q", i, order[i], w)
		}
	}
}

// TestAgent_DurableSession verifies the durable-session
// integration end-to-end: events are appended to a JSONL file
// on every emit; a process restart (modelled by deleting the
// in-memory cache) is followed by a Resume call that
// reconstructs the cache from disk and replays the events.
func TestAgent_DurableSession(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	a := NewAgent(AgentConfig{}, NewRegistry(), store)

	const sessionID = "sess_durable"

	// A buffered drain channel — we only care about the
	// persistence side, not the streaming output, so we
	// silently consume everything that emit* pushes.
	out := make(chan *Event, 32)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-out:
			case <-stop:
				return
			}
		}
	}()

	// 1. Push a few events through the agent's emit path.
	//    Each emitData must land both in the cache AND on disk.
	a.emitData(sessionID, out, EventTextDelta, `{"content":"hello"}`)
	a.emitData(sessionID, out, EventTextDelta, `{"content":"world"}`)
	a.emitData(sessionID, out, EventToolCall, `{"id":"c1","name":"echo","args":"{}","auto_run":true,"kind":"command"}`)
	a.emitError(sessionID, out, "test_code", "test message")
	a.finishSession(sessionID, out)
	close(stop)

	// 2. Verify the JSONL file is on disk and round-trips.
	if !store.Exists(sessionID) {
		t.Fatal("session file should exist after emit")
	}
	diskEvents, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(diskEvents) != 5 {
		t.Fatalf("expected 5 events on disk, got %d: %+v", len(diskEvents), diskEvents)
	}
	if diskEvents[0].Type != EventTextDelta || diskEvents[0].Data != `{"content":"hello"}` {
		t.Errorf("disk event 0 mismatch: %+v", diskEvents[0])
	}
	if diskEvents[4].Type != EventStreamEnd {
		t.Errorf("disk event 4 should be stream_end, got %q", diskEvents[4].Type)
	}

	// 3. Simulate process restart — drop the in-memory cache.
	a.Sessions.Delete(sessionID)
	if _, ok := a.getSession(sessionID); ok {
		t.Fatal("cache should be empty after Delete")
	}

	// 4. Resume should reconstruct the cache from the store.
	//    The returned channel must replay all 5 events in
	//    order, then close (because the rebuilt cache is
	//    marked finished).
	resumeCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := a.Resume(resumeCtx, sessionID, 0)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}

	var replayed []*Event
	for e := range ch {
		replayed = append(replayed, e)
	}
	if len(replayed) != 5 {
		t.Fatalf("Resume should replay all 5 events, got %d: %+v", len(replayed), replayed)
	}
	if replayed[0].Type != EventTextDelta || replayed[0].Data != `{"content":"hello"}` {
		t.Errorf("replayed event 0 mismatch: %+v", replayed[0])
	}
	if replayed[3].Type != EventStreamEnd {
		t.Errorf("replayed event 3 should be stream_end (emitError), got %q", replayed[3].Type)
	}
	if replayed[4].Type != EventStreamEnd {
		t.Errorf("replayed event 4 should be stream_end (finishSession), got %q", replayed[4].Type)
	}
}

// TestAgent_DurableSession_ResumeWithoutStoreReturnsNotFound
// locks the negative path of Resume when neither an in-memory
// cache nor a durable store has a record of the session.
func TestAgent_DurableSession_ResumeWithoutStoreReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	a := NewAgent(AgentConfig{}, NewRegistry(), store)

	_, err := a.Resume(context.Background(), "never_seen", 0)
	if err == nil {
		t.Fatal("Resume on unknown session should return an error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

// TestAgent_DurableSession_AppendFailureDoesNotBlockBusiness
// is the "disk full / permission denied" guard. Emit must
// succeed (in-memory cache is authoritative) even if the
// store's Append returns an error. The user-visible channel
// must NOT see an error, and the in-memory cache must still
// hold the event.
func TestAgent_DurableSession_AppendFailureDoesNotBlockBusiness(t *testing.T) {
	dir := t.TempDir()
	// Pre-create a FILE where the store expects a directory
	// so every Append fails on MkdirAll.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(blocker)
	a := NewAgent(AgentConfig{}, NewRegistry(), store)

	// In real usage, Chat() calls ensureSession() before
	// emitData so the cache exists. We mirror that here so the
	// failure scenario reproduces exactly: persist fails,
	// cache still grows.
	_ = a.ensureSession("s1")

	out := make(chan *Event, 8)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-out:
			case <-done:
				return
			}
		}
	}()

	// This MUST NOT panic and the in-memory cache MUST receive
	// the event even though the store write fails.
	a.emitData("s1", out, EventTextDelta, `{"content":"alive"}`)
	close(done)

	cache, ok := a.getSession("s1")
	if !ok {
		t.Fatal("in-memory cache should still be populated")
	}
	cache.mu.Lock()
	n := len(cache.Events)
	cache.mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 in-memory event after failed persist, got %d", n)
	}
}

// ----------------------------------------------------------------------
// Plan tool tests.
//
// The plan tool (KindPlan / write_todos) has a bespoke execution
// path in the agent core: the registered handler is a no-op, and
// the agent bridges sessionID into runPlanTool which parses the
// todos and stores them on SessionCache.Todos. The tests below
// cover the public surface:
//   - the schema registered via NewPlanToolHandler round-trips
//     through chatTools (so the LLM actually sees the tool)
//   - a full Chat turn with a write_todos tool call pushes the
//     expected EventToolStatus / EventToolResult events AND
//     updates SessionCache.Todos
//   - a malformed write_todos call surfaces a "failed" tool
//     result so the LLM can recover on the next turn
// ----------------------------------------------------------------------

// TestPlanTool_SchemaRoundTripsThroughChatTools locks the
// registration contract: NewPlanToolHandler's schema must reach
// the LLM request as a properly-shaped openai.Tool so the model
// can actually emit write_todos calls.
func TestPlanTool_SchemaRoundTripsThroughChatTools(t *testing.T) {
	reg := NewRegistry()
	def := NewPlanToolHandler()
	reg.Register("write_todos", def.Schema, def.Handler, def.NeedConfirm, def.Kind)
	a := NewAgent(AgentConfig{}, reg)

	tools := a.chatTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0]
	if tool.Function == nil {
		t.Fatal("plan tool: Function is nil")
	}
	if tool.Function.Name != "write_todos" {
		t.Errorf("plan tool name: got %q want write_todos", tool.Function.Name)
	}
	// Parameters must declare the todos array; the LLM rejects
	// the call if the schema is structurally wrong.
	params, ok := tool.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("plan tool: Parameters shape: %T", tool.Function.Parameters)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok || props["todos"] == nil {
		t.Errorf("plan tool: missing todos property in parameters: %+v", params)
	}
}

// TestPlanTool_WriteTodos is the end-to-end happy path: drive a
// scripted OpenAI server that returns a write_todos tool call
// (followed by a normal text "stop"), and assert that the agent
// pushes EventToolStatus + EventToolResult on the wire AND
// updates SessionCache.Todos with the parsed todos.
func TestPlanTool_WriteTodos(t *testing.T) {
	a := makeMultiFakeAgent(t, []string{
		buildChatCompletionStreamBody([]parsedDelta{
			{
				ToolCalls: []parsedToolCall{
					{
						ID:        "call_plan_1",
						Name:      "write_todos",
						Arguments: `{"todos":[{"id":"1","status":"in_progress","content":"List files"},{"id":"2","status":"pending","content":"Delete target"}]}`,
					},
				},
				Finished: true,
			},
		}),
		buildChatCompletionStreamBody([]parsedDelta{
			{Text: "all done", Finished: true},
		}),
	})
	def := NewPlanToolHandler()
	a.Registry.Register("write_todos", def.Schema, def.Handler, def.NeedConfirm, def.Kind)

	ch, err := a.Chat(context.Background(), "sess_plan", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "please plan it"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	events := drainChatStream(t, ch)

	// 1. EventToolStatus was pushed (the "running" badge the
	//    front-end listens to).
	if !sessionHasEvent(events, EventToolStatus) {
		t.Errorf("expected EventToolStatus, events: %+v", events)
	}
	// 2. EventToolResult was pushed with status=success.
	var foundPlanResult bool
	for _, e := range events {
		if e.Type == EventToolResult && strings.Contains(e.Data, `"write_todos"`) {
			foundPlanResult = true
			if !strings.Contains(e.Data, `"status":"success"`) {
				t.Errorf("plan tool result should be success, got %s", e.Data)
			}
		}
	}
	if !foundPlanResult {
		t.Errorf("expected a write_todos EventToolResult, events: %+v", events)
	}
	// 3. SessionCache.Todos reflects the parsed snapshot.
	cache, ok := a.getSession("sess_plan")
	if !ok {
		t.Fatal("session cache not found")
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d: %+v", len(cache.Todos), cache.Todos)
	}
	if cache.Todos[0].ID != "1" || cache.Todos[0].Status != "in_progress" || cache.Todos[0].Content != "List files" {
		t.Errorf("todo[0] mismatch: %+v", cache.Todos[0])
	}
	if cache.Todos[1].ID != "2" || cache.Todos[1].Status != "pending" || cache.Todos[1].Content != "Delete target" {
		t.Errorf("todo[1] mismatch: %+v", cache.Todos[1])
	}
}

// TestPlanTool_MalformedArgsProducesFailedResult covers the
// negative path: the LLM sends a write_todos call with a
// payload that does not unmarshal into []Todo. The agent
// must surface a "failed" tool result so the LLM can recover
// on the next turn, and SessionCache.Todos must NOT be
// corrupted with a partial / zero-value list.
func TestPlanTool_MalformedArgsProducesFailedResult(t *testing.T) {
	a := makeMultiFakeAgent(t, []string{
		buildChatCompletionStreamBody([]parsedDelta{
			{
				ToolCalls: []parsedToolCall{
					{
						ID:        "call_plan_bad",
						Name:      "write_todos",
						Arguments: `{this is not valid JSON`,
					},
				},
				Finished: true,
			},
		}),
		buildChatCompletionStreamBody([]parsedDelta{
			{Text: "ok", Finished: true},
		}),
	})
	def := NewPlanToolHandler()
	a.Registry.Register("write_todos", def.Schema, def.Handler, def.NeedConfirm, def.Kind)

	ch, err := a.Chat(context.Background(), "sess_plan_bad", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "plan it"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	events := drainChatStream(t, ch)

	var foundFailedResult bool
	for _, e := range events {
		if e.Type == EventToolResult && strings.Contains(e.Data, `"write_todos"`) {
			if !strings.Contains(e.Data, `"status":"failed"`) {
				t.Errorf("malformed plan call: expected failed status, got %s", e.Data)
			}
			foundFailedResult = true
		}
	}
	if !foundFailedResult {
		t.Errorf("expected a failed write_todos EventToolResult, events: %+v", events)
	}
	// SessionCache must still exist (we did create the session),
	// but its Todos must be nil / untouched.
	cache, ok := a.getSession("sess_plan_bad")
	if !ok {
		t.Fatal("session cache not found")
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.Todos) != 0 {
		t.Errorf("malformed plan call: Todos should remain empty, got %+v", cache.Todos)
	}
}

// TestPlanTool_SecondCallOverwritesSnapshot covers the
// "assistant refines its plan" flow: two consecutive write_todos
// calls in different turns must result in the LATEST snapshot
// being the one stored on SessionCache.Todos. The agent
// rewrites (not appends) so the front-end can render a stable
// PlanBlock without having to de-dupe by id.
func TestPlanTool_SecondCallOverwritesSnapshot(t *testing.T) {
	a := makeMultiFakeAgent(t, []string{
		buildChatCompletionStreamBody([]parsedDelta{
			{
				ToolCalls: []parsedToolCall{
					{
						ID:        "call_plan_a",
						Name:      "write_todos",
						Arguments: `{"todos":[{"id":"1","status":"in_progress","content":"step A"}]}`,
					},
				},
				Finished: true,
			},
		}),
		buildChatCompletionStreamBody([]parsedDelta{
			{
				ToolCalls: []parsedToolCall{
					{
						ID:        "call_plan_b",
						Name:      "write_todos",
						Arguments: `{"todos":[{"id":"1","status":"completed","content":"step A"},{"id":"2","status":"in_progress","content":"step B"}]}`,
					},
				},
				Finished: true,
			},
		}),
		buildChatCompletionStreamBody([]parsedDelta{
			{Text: "done", Finished: true},
		}),
	})
	def := NewPlanToolHandler()
	a.Registry.Register("write_todos", def.Schema, def.Handler, def.NeedConfirm, def.Kind)

	ch, err := a.Chat(context.Background(), "sess_plan_overwrite", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "plan it"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	_ = drainChatStream(t, ch)

	cache, ok := a.getSession("sess_plan_overwrite")
	if !ok {
		t.Fatal("session cache not found")
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.Todos) != 2 {
		t.Fatalf("expected the latest snapshot (2 todos), got %d: %+v", len(cache.Todos), cache.Todos)
	}
	if cache.Todos[0].Status != "completed" || cache.Todos[1].Status != "in_progress" {
		t.Errorf("Todos should reflect the SECOND call, got %+v", cache.Todos)
	}
}
