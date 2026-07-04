package agent

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// ---- TestPermissionMode_* — Task 20 (Permission Mode Switcher) ----
//
// These tests lock the wire contract for the three
// PermissionMode tiers the front-end can select:
//
//   - PermissionDefault ("default")     : use the tool's
//     registered NeedConfirm value as-is. A tool that
//     opted into NeedConfirm pauses the stream and the
//     agent enters "pending confirmation" (PendingCalls
//     has an entry, the channel stays open without
//     closing). Audit (HookPreToolCall / HookPostToolCall)
//     is NOT fired because the tool did not run.
//
//   - PermissionAutoReview ("auto-review"): force auto-run
//     for every tool the LLM requests. The pending-confirmation
//     path is skipped (PendingCalls stays empty) and the
//     tool handler runs. The audit chain still fires — the
//     same HookPreToolCall / HookPostToolCall sequence as
//     a "default" run on a read-only tool.
//
//   - PermissionFullAccess ("full-access"): same wire
//     behaviour as auto-review (force auto-run, audit
//     fired). The visible "no ApprovalCard rendered"
//     contract is a front-end concern, not an agent-core
//     concern, so the agent-side test cannot tell the
//     two apart — we assert the same shape and trust the
//     UI to differentiate.
//
// The tests use the small helpers from http_test.go
// (makeFakeAgent / makeMultiFakeAgent / buildChatCompletionStreamBody)
// plus a tiny per-test counting hook so we can assert
// "audit still fired" independently of "PendingCalls was
// populated".

// permModeCountingHook is a thin shim around countingHook
// that only fires the events the Task 20 contract cares
// about: HookPreToolCall, HookPostToolCall,
// HookSessionStart, HookTurnStart, HookTurnEnd. The
// existing countingHook (agent_test.go) tracks more events
// but lacks per-event guards, so we re-declare a focused
// version here.
type permModeCountingHook struct {
	mu         sync.Mutex
	sessionID  string
	pre        int32
	post       int32
	lastPreID  string
	lastPreNm  string
	lastPostID string
}

func (h *permModeCountingHook) onPre(sessID, id, name string) {
	atomic.AddInt32(&h.pre, 1)
	h.mu.Lock()
	h.lastPreID = id
	h.lastPreNm = name
	h.sessionID = sessID
	h.mu.Unlock()
}

func (h *permModeCountingHook) onPost(sessID, id string) {
	atomic.AddInt32(&h.post, 1)
	h.mu.Lock()
	h.lastPostID = id
	h.mu.Unlock()
}

// registerPermModeHook attaches a Hook that increments
// the audit counters for the events the Task 20 contract
// tracks. The function is a tiny shim over
// Agent.RegisterHook; it exists so the assertions in the
// four TestPermissionMode_* tests stay readable.
func registerPermModeHook(a *Agent, h *permModeCountingHook) {
	a.RegisterHook(func(ctx context.Context, hc *HookContext) error {
		switch hc.Event {
		case HookPreToolCall:
			if hc.ToolCall != nil {
				h.onPre(hc.SessionID, hc.ToolCall.ID, hc.ToolCall.Name)
			}
		case HookPostToolCall:
			if hc.ToolCall != nil {
				h.onPost(hc.SessionID, hc.ToolCall.ID)
			}
		}
		return nil
	})
}

// scriptCallOnce builds the SSE script the agent will
// receive across LLM calls: the first call returns a
// single tool call (named toolName with the given id and
// args), subsequent calls return a plain "stop" text
// reply. The number of SSE bodies returned is 1 + followUps.
func scriptCallOnce(toolName, toolID, args string, followUps int) []string {
	first := buildChatCompletionStreamBody([]parsedDelta{
		{
			ToolCalls: []parsedToolCall{
				{ID: toolID, Name: toolName, Arguments: args},
			},
			Finished: true,
		},
	})
	out := make([]string, 0, 1+followUps)
	out = append(out, first)
	for i := 0; i < followUps; i++ {
		out = append(out, buildChatCompletionStreamBody([]parsedDelta{
			{Text: "done", Finished: true},
		}))
	}
	return out
}

// TestPermissionMode_Default_RequiresConfirmation locks
// the legacy behaviour: with PermissionMode = "default"
// and a tool that registered NeedConfirm = true, the
// agent MUST pause the stream (PendingCalls is populated,
// the channel stays open), the tool handler MUST NOT
// run, and the audit chain MUST NOT fire (PreToolCall /
// PostToolCall counters stay at zero).
func TestPermissionMode_Default_RequiresConfirmation(t *testing.T) {
	a := makeMultiFakeAgent(t, scriptCallOnce("dangerous", "call_pm_default", `{}`, 1))

	var handlerCalls int32
	a.Registry.Register("dangerous", nil, func(args string) (string, error) {
		atomic.AddInt32(&handlerCalls, 1)
		return `{"ran":true}`, nil
	}, true /* NeedConfirm */, KindCommand)

	h := &permModeCountingHook{}
	registerPermModeHook(a, h)

	// The legacy code path: do NOT call SetSessionPermissionMode.
	// permissionModeFor returns PermissionDefault.
	ch, err := a.Chat(context.Background(), "sess_pm_default", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "do dangerous"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// Drain the stream up to the first non-text event
	// (the tool_call that will suspend). We use a short
	// soft deadline: the test passes if the channel is
	// still open after the deadline, because the
	// pending-confirmation path keeps the goroutine alive
	// and the channel open (it is closed only by
	// ConfirmTool / CancelTool / Resume).
	events := drainChatStream(t, ch)
	if len(events) == 0 {
		t.Fatal("expected at least one event (tool_call) before suspension, got 0")
	}
	// The runLoop closes the channel via
	// finishAndClose AFTER anySuspended flips, so the
	// final event is stream_end. The tool_call
	// suspension event is the one before it.
	var toolCallData string
	for _, ev := range events {
		if ev.Type == EventToolCall {
			toolCallData = string(ev.Data)
		}
	}
	if toolCallData == "" {
		t.Fatalf("expected an EventToolCall in the stream, got events: %+v", events)
	}
	if !strings.Contains(toolCallData, `"auto_run":false`) {
		t.Errorf("expected auto_run=false in tool_call event, got %s", toolCallData)
	}

	// Confirm the suspension side-effect: PendingCalls
	// holds an entry, the handler has NOT been called, and
	// the audit counters are zero.
	a.mu.Lock()
	pending, ok := a.PendingCalls["sess_pm_default"]
	a.mu.Unlock()
	if !ok {
		t.Fatal("PendingCalls should have an entry for sess_pm_default")
	}
	if pending.ToolCallID != "call_pm_default" || pending.ToolName != "dangerous" {
		t.Errorf("pending call mismatch: %+v", pending)
	}
	if got := atomic.LoadInt32(&handlerCalls); got != 0 {
		t.Errorf("tool handler MUST NOT run in default mode + NeedConfirm=true, got %d calls", got)
	}
	if got := atomic.LoadInt32(&h.pre); got != 0 {
		t.Errorf("HookPreToolCall MUST NOT fire in default mode + NeedConfirm=true, got %d", got)
	}
	if got := atomic.LoadInt32(&h.post); got != 0 {
		t.Errorf("HookPostToolCall MUST NOT fire in default mode + NeedConfirm=true, got %d", got)
	}
}

// TestPermissionMode_AutoReview_AutoRuns locks the
// auto-review semantics: a tool that registered
// NeedConfirm = true MUST auto-run when the session's
// permission mode is "auto-review". PendingCalls stays
// empty, the tool handler runs, the audit chain fires
// (PreToolCall + PostToolCall both at 1).
func TestPermissionMode_AutoReview_AutoRuns(t *testing.T) {
	a := makeMultiFakeAgent(t, scriptCallOnce("dangerous", "call_pm_auto", `{}`, 1))

	var handlerCalls int32
	a.Registry.Register("dangerous", nil, func(args string) (string, error) {
		atomic.AddInt32(&handlerCalls, 1)
		return `{"ok":true}`, nil
	}, true /* NeedConfirm */, KindCommand)

	h := &permModeCountingHook{}
	registerPermModeHook(a, h)

	a.SetSessionPermissionMode("sess_pm_auto", PermissionAutoReview)
	t.Cleanup(func() { a.ClearSessionPermissionMode("sess_pm_auto") })

	ch, err := a.Chat(context.Background(), "sess_pm_auto", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "do dangerous"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	events := drainChatStream(t, ch)
	if len(events) == 0 {
		t.Fatal("expected events, got 0")
	}

	// The tool_call event MUST carry auto_run=true
	// (the autoRun override in streamOneTurn).
	var sawToolCall bool
	for _, ev := range events {
		if ev.Type == EventToolCall && strings.Contains(string(ev.Data), `"auto_run":true`) {
			sawToolCall = true
		}
	}
	if !sawToolCall {
		t.Errorf("expected EventToolCall with auto_run=true, events: %+v", events)
	}

	// No suspension: PendingCalls MUST be empty.
	a.mu.Lock()
	_, suspended := a.PendingCalls["sess_pm_auto"]
	a.mu.Unlock()
	if suspended {
		t.Error("PendingCalls MUST be empty in auto-review mode")
	}

	// Tool handler ran once, audit chain fired.
	if got := atomic.LoadInt32(&handlerCalls); got != 1 {
		t.Errorf("tool handler should have run exactly once, got %d", got)
	}
	if got := atomic.LoadInt32(&h.pre); got != 1 {
		t.Errorf("HookPreToolCall should fire once, got %d", got)
	}
	if got := atomic.LoadInt32(&h.post); got != 1 {
		t.Errorf("HookPostToolCall should fire once, got %d", got)
	}
}

// TestPermissionMode_FullAccess_AutoRuns locks the
// full-access semantics: identical to auto-review on the
// agent side (force auto-run, audit fired). The wire
// contract is "no ApprovalCard rendered" — that is a
// front-end concern and is not observable here.
func TestPermissionMode_FullAccess_AutoRuns(t *testing.T) {
	a := makeMultiFakeAgent(t, scriptCallOnce("dangerous", "call_pm_full", `{}`, 1))

	var handlerCalls int32
	a.Registry.Register("dangerous", nil, func(args string) (string, error) {
		atomic.AddInt32(&handlerCalls, 1)
		return `{"ok":true}`, nil
	}, true /* NeedConfirm */, KindCommand)

	h := &permModeCountingHook{}
	registerPermModeHook(a, h)

	a.SetSessionPermissionMode("sess_pm_full", PermissionFullAccess)
	t.Cleanup(func() { a.ClearSessionPermissionMode("sess_pm_full") })

	ch, err := a.Chat(context.Background(), "sess_pm_full", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "do dangerous"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	events := drainChatStream(t, ch)
	if len(events) == 0 {
		t.Fatal("expected events, got 0")
	}

	// Same wire invariants as auto-review.
	a.mu.Lock()
	_, suspended := a.PendingCalls["sess_pm_full"]
	a.mu.Unlock()
	if suspended {
		t.Error("PendingCalls MUST be empty in full-access mode")
	}
	if got := atomic.LoadInt32(&handlerCalls); got != 1 {
		t.Errorf("tool handler should have run exactly once, got %d", got)
	}
	if got := atomic.LoadInt32(&h.pre); got != 1 {
		t.Errorf("HookPreToolCall should fire once, got %d", got)
	}
	if got := atomic.LoadInt32(&h.post); got != 1 {
		t.Errorf("HookPostToolCall should fire once, got %d", got)
	}
}

// TestPermissionMode_Default_ReadOnlyToolStillAutoRuns locks
// the constraint that the new modes do NOT regress the
// legacy read-only path: a tool that registered
// NeedConfirm = false MUST auto-run regardless of
// permission mode (default / auto-review / full-access
// all behave identically for read-only tools). The
// audit chain MUST fire.
func TestPermissionMode_Default_ReadOnlyToolStillAutoRuns(t *testing.T) {
	a := makeMultiFakeAgent(t, scriptCallOnce("read", "call_pm_read", `{}`, 1))

	var handlerCalls int32
	a.Registry.Register("read", nil, func(args string) (string, error) {
		atomic.AddInt32(&handlerCalls, 1)
		return `{"ok":true}`, nil
	}, false /* NeedConfirm */, KindReadOnly)

	h := &permModeCountingHook{}
	registerPermModeHook(a, h)

	// No SetSessionPermissionMode: legacy default path.
	ch, err := a.Chat(context.Background(), "sess_pm_read", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "read something"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	_ = drainChatStream(t, ch)

	a.mu.Lock()
	_, suspended := a.PendingCalls["sess_pm_read"]
	a.mu.Unlock()
	if suspended {
		t.Error("read-only tool MUST NOT enter PendingCalls in default mode")
	}
	if got := atomic.LoadInt32(&handlerCalls); got != 1 {
		t.Errorf("read-only tool handler should have run once, got %d", got)
	}
	if got := atomic.LoadInt32(&h.pre); got != 1 {
		t.Errorf("HookPreToolCall should fire once, got %d", got)
	}
	if got := atomic.LoadInt32(&h.post); got != 1 {
		t.Errorf("HookPostToolCall should fire once, got %d", got)
	}
}

// TestPermissionMode_Helpers_RoundTrip covers the
// store/read helpers in isolation (no agent loop). It
// locks three contracts:
//
//  1. SetSessionPermissionMode("s", "default") is the
//     no-op sentinel — the per-session entry is
//     deleted, not stored, so a subsequent
//     permissionModeFor("s") returns PermissionDefault.
//  2. SetSessionPermissionMode("s", "auto-review") is
//     stored verbatim and permissionModeFor("s")
//     returns PermissionAutoReview.
//  3. ClearSessionPermissionMode wipes the entry so a
//     subsequent permissionModeFor returns
//     PermissionDefault.
//
// Unknown / empty sessionIDs are no-ops and unknown
// modes fall back to PermissionDefault at lookup time.
func TestPermissionMode_Helpers_RoundTrip(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())

	// 1. "default" deletes the entry.
	a.SetSessionPermissionMode("s1", PermissionDefault)
	if got := a.permissionModeFor("s1"); got != PermissionDefault {
		t.Errorf("after default: got %q want %q", got, PermissionDefault)
	}

	// 2. "auto-review" stored verbatim.
	a.SetSessionPermissionMode("s2", PermissionAutoReview)
	if got := a.permissionModeFor("s2"); got != PermissionAutoReview {
		t.Errorf("after auto-review: got %q want %q", got, PermissionAutoReview)
	}

	// 3. Clear wipes.
	a.ClearSessionPermissionMode("s2")
	if got := a.permissionModeFor("s2"); got != PermissionDefault {
		t.Errorf("after clear: got %q want %q", got, PermissionDefault)
	}

	// 4. Empty sessionID is a no-op.
	a.SetSessionPermissionMode("", PermissionAutoReview)
	if got := a.permissionModeFor(""); got != PermissionDefault {
		t.Errorf("empty sessionID: got %q want %q", got, PermissionDefault)
	}

	// 5. IsValidPermissionMode accepts the three
	//    documented constants. Empty / unknown values
	//    are intentionally NOT considered "valid" — the
	//    HTTP layer normalises them to PermissionDefault
	//    before calling SetSessionPermissionMode, so
	//    permissionModeFor is the only path that ever
	//    sees a non-constant. The negative cases confirm
	//    the validator is strict.
	cases := map[string]bool{
		"default":     true,
		"auto-review": true,
		"full-access": true,
		"":            false,
		"unknown":     false,
		"AUTO-REVIEW": false, // case-sensitive on purpose
	}
	for in, want := range cases {
		if got := IsValidPermissionMode(PermissionMode(in)); got != want {
			t.Errorf("IsValidPermissionMode(%q) = %v want %v", in, got, want)
		}
	}
}
