// Package agent provides a Go-native agent library that bridges an LLM
// (typically OpenAI) with a thread-safe tool registry. The library exposes
// a JSON-serializable event stream that is decoupled from any specific
// transport (HTTP/SSE, in-process channel, etc.).
//
// The library is consumed by an HTTP/SSE service that fronts a Vue
// front-end. See /workspace/.trae/specs/go-in-process-agent/spec.md for
// the full architecture.
package agent

// EventType is the discriminator of an [Event] flowing through the
// agent's stream. Each value maps to a specific Data payload shape; the
// consumer decodes Data according to Type.
type EventType string

const (
	// EventTextDelta carries an incremental text fragment emitted by the
	// LLM. Multiple deltas concatenate into a complete assistant
	// message. Data is a JSON-encoded struct of shape
	// {"content": "..."}.
	EventTextDelta EventType = "text_delta"

	// EventReasoningDelta carries an incremental reasoning fragment
	// (e.g. OpenAI o1 reasoning_content). Data is a JSON-encoded struct
	// of shape {"content": "..."}.
	EventReasoningDelta EventType = "reasoning_delta"

	// EventToolCall announces a tool invocation requested by the LLM.
	// Data is a JSON-encoded [ToolCallData].
	EventToolCall EventType = "tool_call"

	// EventToolStatus is an in-flight status change for a tool call
	// (typically "running" → "success" / "failed" / "cancelled"). Data
	// is a JSON-encoded [ToolStatusData].
	EventToolStatus EventType = "tool_status"

	// EventToolResult is the final outcome of a tool execution. Data is
	// a JSON-encoded [ToolResultData].
	EventToolResult EventType = "tool_result"

	// EventStreamEnd marks the end of a streaming turn. After this
	// event the channel is closed. Data is an empty string.
	EventStreamEnd EventType = "stream_end"

	// EventCompaction signals that the agent has auto-compressed
	// the conversation history. The payload (CompactionData) carries
	// the LLM-generated summary text and the count of messages it
	// replaced. The front-end renders a non-expandable divider at
	// the position the compacted messages used to occupy so the user
	// can see "context was compressed here" without re-reading the
	// dropped text. The agent has already inserted a synthetic
	// "summary" message at the head of the conversation; the event
	// itself is purely informational.
	EventCompaction EventType = "compaction"
)

// Decision is one of four user responses to a pending tool approval.
// Values align 1:1 with the codex_web approvalDecisionSchema.
type Decision string

const (
	// DecisionAccept approves the pending tool call for this single
	// execution. Future calls of the same tool still require approval.
	DecisionAccept Decision = "accept"

	// DecisionAcceptForSession approves the pending tool call AND
	// records a session-level grant for the same (toolName, sessionID),
	// so subsequent calls of that tool in the same session auto-run
	// without prompting the user.
	DecisionAcceptForSession Decision = "accept_for_session"

	// DecisionDecline rejects the pending tool call. The agent pushes a
	// synthetic "user_rejected" tool result so the LLM can continue the
	// turn with another approach.
	DecisionDecline Decision = "decline"

	// DecisionCancel rejects the pending tool call and terminates the
	// current turn. The agent emits EventStreamEnd without recursing
	// into another LLM call.
	DecisionCancel Decision = "cancel"
)

// ToolKind classifies a tool so the front-end can pick the correct
// icon / colour in the approval card. The value is supplied by the
// caller when registering the tool.
type ToolKind string

const (
	// KindCommand marks tools that execute shell commands.
	KindCommand ToolKind = "command"

	// KindFileChange marks tools that mutate files on disk.
	KindFileChange ToolKind = "fileChange"

	// KindReadOnly marks tools that only read state without side
	// effects (e.g. list_files).
	KindReadOnly ToolKind = "readOnly"

	// KindPlan marks the built-in write_todos plan tool. The
	// front-end renders a PlanBlock for tool calls of this
	// kind; operationGroup and other groupings MUST NOT
	// coalesce plan calls into their windows.
	KindPlan ToolKind = "plan"

	// KindUnknown is the safe default when the caller cannot classify
	// the tool. Front-ends should fall back to a neutral icon.
	KindUnknown ToolKind = "unknown"
)

// Todo is one entry in the plan tool's todo list. The agent
// stores the latest list on the session cache so the front-end
// can render a stable PlanBlock while the assistant refines its
// plan across multiple tool calls.
type Todo struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Content string `json:"content"`
}

// Event is the atomic unit emitted on the agent's event channel.
//
// Data is intentionally a JSON-encoded string (rather than an any) so
// the wire format is unambiguous: the consumer always parses a string
// into the concrete payload type advertised by Type.
type Event struct {
	Type EventType `json:"type"`
	Data string    `json:"data"`
}

// ToolCallData is the payload of [EventToolCall].
//
// Args is a JSON-encoded object whose schema is the tool's
// OpenAI-style function-calling schema. AutoRun is computed by the
// agent as !NeedConfirm at the moment the tool is registered.
type ToolCallData struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Args    string   `json:"args"`
	AutoRun bool     `json:"auto_run"`
	Kind    ToolKind `json:"kind"`
}

// ToolResultData is the payload of [EventToolResult].
//
// Result is a JSON-encoded string whose structure is tool-defined.
// Status is one of "success" | "failed" | "cancelled" | "running".
type ToolResultData struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Result     string `json:"result"`
	IsError    bool   `json:"is_error"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
}

// ToolStatusData is the payload of [EventToolStatus]. It is a
// lightweight update for a tool that is already in-flight.
//
// Status is one of "running" | "success" | "failed" | "cancelled".
type ToolStatusData struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CompactionData is the payload of [EventCompaction].
//
// The agent estimates the total token count of the conversation
// before each LLM turn. When the count crosses the threshold
// (default 80% of the model context window), it asks the LLM
// itself to summarise the older messages and replaces them in
// place with a single "summary" message. This event is then
// pushed to the front-end so it can render a divider at the
// compaction boundary.
//
// SummaryText is the LLM-generated summary the agent just
// inserted at the head of the messages slice.
//
// ReplacedMessageCount is the number of older messages that
// were dropped from the conversation. It is purely advisory
// (used by the UI for the divider label / log) and is NOT used
// by the front-end to recover the dropped text — that text is
// gone from the conversation once the event is emitted.
//
// TriggeredAtMs is the unix-millisecond timestamp at which the
// compaction was emitted, included so DevLogs can correlate
// the event with other agent logs.
type CompactionData struct {
	SummaryText          string `json:"summary_text"`
	ReplacedMessageCount int    `json:"replaced_message_count"`
	TriggeredAtMs        int64  `json:"triggered_at_ms"`
}

// MessageData is the in-memory accumulator for a single assistant
// turn. It is not wire-serialised; the agent uses it to fold deltas
// (text / reasoning) and tool events into one cohesive message before
// the next LLM call.
type MessageData struct {
	Content     string
	Reasoning   string
	ToolCalls   []ToolCallData
	ToolResults []ToolResultData
}

// PasswordStrategy is the agent-package mirror of
// encv-go's interfaces.PasswordStrategy. It exists so the agent
// module can stay self-contained and importable as a plain Go
// library without dragging in encv-go's internal packages.
type PasswordStrategy string

const (
	// PasswordGlobal — the agent uses the global password from
	// AgentConfig (default for video/audio/image/wps/pdf/text).
	PasswordGlobal PasswordStrategy = "global"
	// PasswordIndependent — the LLM must supply the password
	// per call (default for alist_encrypt).
	PasswordIndependent PasswordStrategy = "independent"
	// PasswordNone — the plugin does not require a password.
	PasswordNone PasswordStrategy = "none"
)

// PluginTaskField describes one extra input field a plugin expects
// (resolution, codec, fn_rounds, etc.). The agent maps this to a
// JSON-Schema property in the tool schema.
type PluginTaskField struct {
	Key          string
	Label        string
	Type         string
	Required     bool
	DefaultValue string
	Help         string
	Options      []string
	OptionLabels map[string]string
	Condition    string
}

// PluginTaskOptions is the agent-package mirror of
// interfaces.TaskOptions.
type PluginTaskOptions struct {
	PasswordStrategy     PasswordStrategy
	SupportVersionSelect bool
	SupportedVersions    []int
	DefaultVersion       int
	ExtraFields          []PluginTaskField
}

// HookEvent is the discriminator of the 6 hook points the
// agent exposes to user-supplied callbacks. The value is a
// stable wire string; persist it in your audit log so callers
// can correlate events across agent versions.
type HookEvent string

const (
	// HookSessionStart fires when a brand-new session is
	// created — i.e. the first Chat call on a sessionID
	// that has no pre-existing SessionCache. Use it to
	// inject per-session system prompt overrides, register
	// skills, or audit "session opened" events.
	HookSessionStart HookEvent = "session_start"

	// HookTurnStart fires immediately before the agent
	// calls the LLM (one fire per turn, including resumed
	// turns after auto-run tool loops).
	HookTurnStart HookEvent = "turn_start"

	// HookTurnEnd fires after the LLM stream ends and any
	// tool-call processing is complete, but before the
	// terminal EventStreamEnd is pushed. Use it to capture
	// assistant outputs for analytics or compaction.
	HookTurnEnd HookEvent = "turn_end"

	// HookPreToolCall fires before an auto-run tool
	// handler is invoked. Set hc.Cancel to true to abort
	// the execution; the agent then synthesises a
	// "cancelled" tool result so the LLM can continue
	// the turn.
	HookPreToolCall HookEvent = "pre_tool_call"

	// HookPostToolCall fires after a tool handler has
	// returned. The hook receives both the original tool
	// call and the result so loggers / policy enforcers
	// can audit the call.
	HookPostToolCall HookEvent = "post_tool_call"

	// HookSessionShutdown fires when an explicit
	// ShutdownSession call is made on the agent. Use it to
	// flush durable state, close databases, or push a
	// final audit event.
	HookSessionShutdown HookEvent = "session_shutdown"
)

// AgentConfig is the top-level configuration consumed by NewAgent.
// Field names are snake_case on the wire (see config_loader.go) but
// idiomatic Go here.
type AgentConfig struct {
	// OpenAI side
	OpenAIAPIKey  string
	OpenAIBaseURL string
	OpenAIModel   string

	// OpenList side
	OpenListBaseURL string
	OpenListToken   string

	// Plugin / agent behaviour
	DefaultContainerVersion int
	EnabledTools            []string
	SystemPrompt            string
	MaxToolCallsPerTurn     int
	// GlobalPassword is the fallback password for plugins whose
	// PasswordStrategy == "global" (video/audio/image/wps/pdf/text).
	GlobalPassword string

	// SkillsDir is the on-disk directory ScanSkills walks at
	// NewAgent time. The expected layout is
	// "<SkillsDir>/<skill-name>/SKILL.md", one folder per
	// skill. An empty string defaults to "$HOME/.encv/skills"
	// (or "./.encv/skills" if $HOME is unset). A missing
	// directory is not an error: NewAgent continues with
	// zero skills.
	SkillsDir string
}

// PermissionMode is the per-session tool-confirmation tier.
// Task 20 (Permission Mode Switcher) exposes three values
// to the front-end: PermissionDefault (the legacy
// "respect NeedConfirm" behaviour), PermissionAutoReview
// (force auto-run but still emit tool_call + tool_result
// events for visual review), and PermissionFullAccess
// (force auto-run, no ApprovalCard at all). The constants
// are declared alongside the type so the wire format and
// the in-memory representation stay in lockstep.
type PermissionMode string

const (
	// PermissionDefault preserves the legacy behaviour:
	// a tool that registered NeedConfirm=true still asks
	// for confirmation, a tool that did not still auto-runs.
	PermissionDefault PermissionMode = "default"
	// PermissionAutoReview forces every tool to auto-run
	// while still emitting the tool_call + tool_result
	// events on the wire. The front-end renders a
	// non-modal review badge in place of the ApprovalCard.
	PermissionAutoReview PermissionMode = "auto-review"
	// PermissionFullAccess forces every tool to auto-run
	// AND tells the front-end to skip rendering an
	// ApprovalCard for the duration of the session. The
	// tool_call / tool_result events still surface so
	// DevLogs can audit the execution.
	PermissionFullAccess PermissionMode = "full-access"
)

// IsValidPermissionMode reports whether m is one of the
// three documented PermissionMode values. The HTTP layer
// uses it to coerce unknown / empty values to
// PermissionDefault rather than 400ing — see
// ChatRequest.PermissionMode for the rationale.
func IsValidPermissionMode(m PermissionMode) bool {
	switch m {
	case PermissionDefault, PermissionAutoReview, PermissionFullAccess:
		return true
	default:
		return false
	}
}
