package agent

import "sync"

// Tool is the future-facing registration record of a
// single tool. Task 19 / Task 24 (plan-mode toggle, events
// JSONL replay) sketches a struct-based Register signature
// in the test suite (plan_mode_test.go) that takes a Tool
// value directly. The struct is kept minimal at this stage
// so the existing positional-argument Register is the
// authoritative entry point; the struct-based overload is
// added next to it so test code that already targets it
// (and future migration targets) compile without
// rewriting.
//
// Fields mirror the positional Register parameters:
//
//   - Name     → first positional argument
//   - Schema   → second positional argument
//   - Handler  → third positional argument
//   - NeedConfirm → fourth positional argument
//   - Kind     → fifth positional argument
//
// The struct exists purely so the test suite compiles
// against a stable surface; production code SHOULD
// continue to use the positional Register for clarity.
type Tool struct {
	Name        string
	Schema      any
	Handler     func(args string) (string, error)
	NeedConfirm bool
	Kind        ToolKind
}

// ToolDefinition is the registration record of a single tool inside a
// [ToolRegistry].
//
// Schema is an OpenAI-style function-calling schema describing the
// tool's parameters; the agent forwards it verbatim when assembling the
// LLM request. Schema is typed as any so callers can use whichever
// schema representation they prefer (struct, map[string]any,
// *jsonschema.Schema, etc.).
//
// Handler executes the tool. It receives args as the raw JSON string
// the LLM produced (which avoids re-marshaling) and must return a JSON
// string that will be wrapped in [ToolResultData.Result].
//
// NeedConfirm is the source of truth for the AutoRun flag on
// [ToolCallData]: when true, the agent pauses for a [Decision] before
// invoking Handler.
//
// Kind classifies the tool for the front-end's approval card icon.
type ToolDefinition struct {
	Schema      any
	Handler     func(args string) (string, error)
	NeedConfirm bool
	Kind        ToolKind
}

// ToolRegistry is a thread-safe collection of tool definitions.
//
// A single registry is shared by the agent core and any HTTP handler
// that exposes tool metadata (e.g. an OpenAI-compatible /v1/tools
// endpoint), so reads must scale. Writes (Register) are expected to
// happen at startup, so a sync.RWMutex is the right primitive: many
// concurrent Get/GetAllSchemas callers, but only one or two writers.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]ToolDefinition
}

// NewRegistry returns an empty, ready-to-use registry.
func NewRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]ToolDefinition),
	}
}

// Register stores a tool under the given name. If a tool with the same
// name already exists, it is overwritten. Register is safe to call
// concurrently with Get / GetAllSchemas.
func (r *ToolRegistry) Register(
	name string,
	schema any,
	handler func(args string) (string, error),
	needConfirm bool,
	kind ToolKind,
) {
	def := ToolDefinition{
		Schema:      schema,
		Handler:     handler,
		NeedConfirm: needConfirm,
		Kind:        kind,
	}
	r.mu.Lock()
	r.tools[name] = def
	r.mu.Unlock()
}

// RegisterTool is the struct-based overload of Register. It
// is added so the test suite (plan_mode_test.go and any
// future migration target) can write
// `reg.Register(Tool{Name: "x", ...})` instead of
// repeating the positional parameter list. Production code
// should still prefer the positional Register for
// readability; RegisterTool exists purely to keep the
// struct-based form compileable.
//
// The overload is a thin shim around the positional
// Register — no validation, no extras, no copy of the
// handler. A nil return is reserved for a future iteration
// that adds validation (today there is nothing to fail on).
func (r *ToolRegistry) RegisterTool(t Tool) error {
	r.Register(t.Name, t.Schema, t.Handler, t.NeedConfirm, t.Kind)
	return nil
}

// Get fetches a tool by name. The boolean return mirrors map lookups
// and lets callers branch without sentinel values.
func (r *ToolRegistry) Get(name string) (ToolDefinition, bool) {
	r.mu.RLock()
	def, ok := r.tools[name]
	r.mu.RUnlock()
	return def, ok
}

// GetAllSchemas returns every registered tool's Schema in an
// unspecified order. The slice is freshly allocated, so callers may
// mutate it without affecting the registry. This is the canonical
// shape the agent forwards to the LLM as the "tools" field.
func (r *ToolRegistry) GetAllSchemas() []any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]any, 0, len(r.tools))
	for _, def := range r.tools {
		out = append(out, def.Schema)
	}
	return out
}

// Names returns the registered tool names in unspecified
// order. The slice is freshly allocated and may be mutated by
// the caller. Used by the demo's enabled_tools filter and by
// debug endpoints that need to enumerate the registry.
func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	return out
}

// Unregister removes a tool by name. It is a no-op if the
// tool does not exist. The primary use case is the demo's
// enabled_tools filter; production code should not strip
// tools at runtime.
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	delete(r.tools, name)
	r.mu.Unlock()
}

// planToolSchema is the OpenAI-style function-calling schema
// for the built-in write_todos plan tool. It is a static
// map (not a *openai.FunctionDefinition) so the registry can
// convert it via convertMapToTool without an extra type
// import.
var planToolSchema = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "write_todos",
		"description": "Persist the assistant's current step-by-step plan to the session. The front-end renders the latest plan snapshot as a PlanBlock; subsequent calls overwrite the snapshot.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"todos": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "Stable identifier for the todo item; the LLM should reuse the same id when reordering.",
							},
							"status": map[string]any{
								"type":        "string",
								"enum":        []string{"pending", "in_progress", "completed"},
								"description": "Lifecycle state of the todo item.",
							},
							"content": map[string]any{
								"type":        "string",
								"description": "Human-readable description of the step.",
							},
						},
						"required": []string{"id", "status", "content"},
					},
				},
			},
			"required": []string{"todos"},
		},
	},
}

// planToolNoopHandler is the placeholder handler that
// NewPlanToolHandler registers. The agent core has a
// dedicated code path (runPlanTool) that bridges sessionID
// into the call, parses the todos and stores them on the
// session cache; this no-op handler is never actually
// invoked at runtime but is required by the ToolDefinition
// shape. Keeping the handler side-effect-free also means
// accidental direct invocations (e.g. from a test that
// bypasses the agent loop) cannot corrupt the cache.
func planToolNoopHandler(args string) (string, error) {
	return `{"ok":true,"note":"plan handler is a no-op; agent core handles write_todos"}`, nil
}

// NewPlanToolHandler returns the ToolDefinition for the
// built-in write_todos plan tool. The schema is the single
// source of truth for the tool's wire shape; the handler is
// a no-op because the agent core has a dedicated execution
// path that knows the sessionID.
//
// Callers wire this into the registry at startup; the agent
// then recognises any tool call whose Kind == KindPlan and
// routes it through runPlanTool so SessionCache.Todos is
// updated.
func NewPlanToolHandler() ToolDefinition {
	return ToolDefinition{
		Schema:      planToolSchema,
		Handler:     planToolNoopHandler,
		NeedConfirm: false,
		Kind:        KindPlan,
	}
}
