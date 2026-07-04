package agent

import (
	"context"
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

// HookFunc is the callback signature for the agent's hook
// system. It receives a context (typically the same context
// the chat goroutine is running under) and a HookContext
// describing the event being raised.
//
// Returning an error does NOT abort the main flow — the
// return value is reserved for future use (e.g. surfacing
// audit failures) and is currently ignored. Panics ARE
// recovered inside the dispatcher and logged to stderr so a
// buggy hook cannot crash the agent.
//
// Hooks are invoked synchronously in registration order on
// the chat goroutine. They MUST NOT block for long
// durations; long-running side work (network calls, disk
// I/O) should be dispatched onto a background goroutine
// inside the hook body. Hooks MUST NOT call back into the
// agent's public API (e.g. Chat / ConfirmTool) on the same
// session, or they will deadlock the worker goroutine.
type HookFunc func(ctx context.Context, hc *HookContext) error

// HookContext is the payload passed to every hook
// invocation. Only the fields relevant to the current
// [HookEvent] are populated; others are zero values.
//
// Cancel is meaningful only for [HookPreToolCall]: the hook
// sets the bool it points to true to abort the tool
// execution, after which the agent synthesises a
// "cancelled" tool result so the LLM can continue the turn.
//
// Messages is a snapshot of the conversation at the moment
// the hook is raised. The slice is the live slice (not a
// copy); hooks that need to keep the data past the
// callback should clone it.
type HookContext struct {
	Event      HookEvent
	SessionID  string
	Messages   []openai.ChatCompletionMessage
	ToolCall   *ToolCallData
	ToolResult *ToolResultData
	Cancel     *bool

	// SelectedSkills is populated on the [HookSessionStart]
	// event and lists the skill names the front-end asked
	// the agent to activate for this session. The default
	// skill-injection hook (registered automatically by
	// NewAgent when any skill is loaded) reads this field
	// and appends the matching skill prompts to the
	// session's per-session system prompt override. The
	// field is empty for the other five hook points and on
	// sessions where the front-end did not request a skill.
	SelectedSkills []string
}

// registerHook appends h to the agent's hook list. It is
// exported via Agent.RegisterHook and safe to call from any
// goroutine, including concurrently with hook dispatch.
//
// Order of invocation matches registration order (FIFO).
// The slice is grown under a write lock; dispatch snapshots
// hooksMu and the hooks slice back the public
// [Agent.RegisterHook] declared in agent.go. Dispatch
// snapshots the slice under a read lock to avoid blocking
// registration during long hook chains.

// runHooks invokes every registered hook on the supplied
// HookContext. The hook slice is snapshotted under the
// mutex so concurrent RegisterHook calls do not race with
// dispatch.
//
// Panics are recovered and logged to stderr with the
// offending event name. Returned errors are intentionally
// ignored — the contract is that hooks cannot influence the
// main flow (except via HookPreToolCall.Cancel).
func (a *Agent) runHooks(ctx context.Context, hc *HookContext) {
	a.hooksMu.RLock()
	snapshot := make([]HookFunc, len(a.hooks))
	copy(snapshot, a.hooks)
	a.hooksMu.RUnlock()

	for _, h := range snapshot {
		func(h HookFunc) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "agent: hook panic recovered for event %q: %v\n", hc.Event, r)
				}
			}()
			// Errors are intentionally swallowed. The
			// contract documented on HookFunc reserves the
			// return value for future use.
			_ = h(ctx, hc)
		}(h)
	}
}
