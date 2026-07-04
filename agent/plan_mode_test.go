package agent

import (
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// ----------------------------------------------------------------------
// Task 19: Plan Mode toggle
//
// Plan mode is a per-session, per-turn flag the front-end
// sends as part of the /api/chat JSON body. When set, the
// agent appends a directive ("list steps first; wait for
// user confirmation before executing") to the system prompt
// that injectSystemPrompt prepends on every LLM call. The
// flag does NOT alter the tool registry — the same tool
// set is exposed either way; only the textual framing
// changes.
//
// These tests cover the four behaviours:
//   1. SetPlanMode / planModeFor round-trip per session.
//   2. injectSystemPrompt appends the directive when the
//      flag is on and a per-session system prompt already
//      exists.
//   3. injectSystemPrompt prepends ONLY the directive
//      (no system prompt set) when the flag is on — the
//      function must still emit a system message.
//   4. Disabling the flag on a subsequent turn restores
//      the no-flag behaviour and drops the directive
//      (no stale "true" left behind).
// ----------------------------------------------------------------------

// TestPlanMode_SetAndRead covers the basic store/load
// semantics of SetPlanMode. Toggling the flag off MUST
// remove the entry so a later planModeFor returns false.
func TestPlanMode_SetAndRead(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())

	// Default (no SetPlanMode call) → false.
	if a.planModeFor("sess-1") {
		t.Errorf("planModeFor on unset session should be false")
	}

	// Turn on.
	a.SetPlanMode("sess-1", true)
	if !a.planModeFor("sess-1") {
		t.Errorf("planModeFor after SetPlanMode(true) should be true")
	}

	// Turn off.
	a.SetPlanMode("sess-1", false)
	if a.planModeFor("sess-1") {
		t.Errorf("planModeFor after SetPlanMode(false) should be false")
	}

	// Other sessions must not be polluted.
	a.SetPlanMode("sess-2", true)
	if a.planModeFor("sess-3") {
		t.Errorf("planModeFor on unrelated session should be false")
	}

	// Empty sessionID is a no-op (defensive, mirrors
	// SetSelectedSkills).
	a.SetPlanMode("", true)
	if a.planModeFor("") {
		t.Errorf("planModeFor on empty sessionID should be false")
	}
}

// TestPlanMode_AppendsWhenSystemPromptSet covers the
// combined case: a session that has a per-session system
// prompt override AND plan mode turned on. The injected
// system message must contain BOTH halves — the original
// prompt and the appended directive — separated by a
// blank line so the LLM reads them as two paragraphs.
func TestPlanMode_AppendsWhenSystemPromptSet(t *testing.T) {
	a := NewAgent(AgentConfig{SystemPrompt: "global base prompt"}, NewRegistry())
	a.systemPromptBySession.Store("sess-pm", "session override")
	a.SetPlanMode("sess-pm", true)

	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hi"},
	}
	out := a.injectSystemPrompt("sess-pm", msgs)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(out))
	}
	if out[0].Role != openai.ChatMessageRoleSystem {
		t.Errorf("prepended message role: got %q want system", out[0].Role)
	}
	body := out[0].Content
	if !strings.Contains(body, "session override") {
		t.Errorf("system body should keep the original prompt, got %q", body)
	}
	if !strings.Contains(body, planModeInstruction) {
		t.Errorf("system body should contain the plan-mode directive, got %q", body)
	}
	// Separator: a blank line between the two halves.
	if !strings.Contains(body, "session override\n\n"+planModeInstruction) {
		t.Errorf("expected two paragraphs separated by a blank line, got %q", body)
	}
	// User message must be preserved at index 1.
	if out[1].Role != openai.ChatMessageRoleUser || out[1].Content != "hi" {
		t.Errorf("user message lost: %+v", out[1])
	}
}

// TestPlanMode_PrependsWhenNoSystemPromptSet covers the
// case where plan mode is on but the session has NO
// per-session system prompt override AND the agent's
// global SystemPrompt is also empty. injectSystemPrompt
// must STILL emit a system message — otherwise the
// directive would be silently dropped, which would defeat
// the toggle.
func TestPlanMode_PrependsWhenNoSystemPromptSet(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry()) // no global prompt
	a.SetPlanMode("sess-only-pm", true)

	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "go"},
	}
	out := a.injectSystemPrompt("sess-only-pm", msgs)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(out))
	}
	if out[0].Role != openai.ChatMessageRoleSystem {
		t.Errorf("prepended message role: got %q want system", out[0].Role)
	}
	if out[0].Content != planModeInstruction {
		t.Errorf("system body should equal the directive on its own, got %q", out[0].Content)
	}
}

// TestPlanMode_DisabledByDefault covers the "toggle off"
// path: a session that has plan mode enabled then disabled
// (and nothing else) must end up with no system message —
// matching the pre-flag behaviour exactly. This locks the
// invariant that disabling the flag leaves no stale
// directive behind.
func TestPlanMode_DisabledByDefault(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	a.SetPlanMode("sess-off", true)
	// toggle off
	a.SetPlanMode("sess-off", false)

	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hi"},
	}
	out := a.injectSystemPrompt("sess-off", msgs)
	if len(out) != 1 {
		t.Fatalf("disabled plan mode + no system prompt should leave slice unchanged, got %d messages", len(out))
	}
	if out[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("user message should be at index 0, got role %q", out[0].Role)
	}
	if strings.Contains(out[0].Content, planModeInstruction) {
		t.Errorf("disabled plan mode must not leak the directive, got %q", out[0].Content)
	}
}

// TestPlanMode_DoesNotAlterToolRegistry locks the
// constraint that plan mode is purely a system-prompt
// concern: the agent's tool set is identical regardless
// of the flag. We assert this by registering one tool,
// then checking the tool's name appears in the registry
// with plan mode off AND on.
func TestPlanMode_DoesNotAlterToolRegistry(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterTool(Tool{
		Name: "echo",
		Kind: KindReadOnly,
		Handler: func(args string) (string, error) {
			return args, nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	a := NewAgent(AgentConfig{}, reg)

	// Snapshot tools before enabling plan mode.
	before := registryNames(t, a)
	// Snapshot tools after enabling plan mode.
	a.SetPlanMode("sess-tools", true)
	after := registryNames(t, a)

	if !equalSlices(before, after) {
		t.Errorf("plan mode must not change the tool registry: before=%v after=%v", before, after)
	}
}

// registryNames is a small test helper that returns the
// list of tool names currently registered on the agent.
// The exact accessor is kept package-private to the agent
// package (a.Registry.Names); tests in the same package
// can call it directly.
func registryNames(t *testing.T, a *Agent) []string {
	t.Helper()
	return append([]string{}, a.Registry.Names()...)
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
