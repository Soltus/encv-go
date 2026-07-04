# agent — Go in-process agent library

`agent` is a self-contained Go module that bridges an LLM
(typically OpenAI) with a thread-safe registry of Go-native tools.
It exposes a JSON-serializable event stream that any transport
(HTTP/SSE, in-process channel, IPC, …) can forward to a front-end.

The full design lives at
`/workspace/.trae/specs/go-in-process-agent/spec.md`; this README only
covers the **Phase 1** skeleton: the event / decision / kind type
contract and the tool registry.

---

## Status

| Phase | Scope | Status |
|-------|-------|--------|
| 1     | Core types + `ToolRegistry` | ✅ done |
| 2     | Agent core (`Chat` / `ConfirmTool` / `Resume` + `SessionCache`) | pending |
| 3     | OpenAI streaming client | pending |
| 4     | HTTP/SSE handlers (`/api/chat` / `/api/resume` / `/api/confirm`) | pending |
| 5     | OpenList custom-API client | pending |
| 6     | `cmd/agent-demo` reference service | pending |

---

## Module layout

```
agent/
├── go.mod             # module github.com/encv/agent (Go ≥ 1.21)
├── README.md          # this file
├── types.go           # Event / EventType / Decision / ToolKind / payloads
├── registry.go        # ToolRegistry + ToolDefinition
├── types_test.go      # JSON round-trip + enum contract
└── registry_test.go   # register / concurrent get / schema list / kind
```

---

## Type contract (Phase 1)

| Symbol            | Wire form (JSON)         | Purpose |
|-------------------|--------------------------|---------|
| `EventType`       | `text_delta` / `reasoning_delta` / `tool_call` / `tool_status` / `tool_result` / `stream_end` | discriminator on `Event.Type` |
| `Decision`        | `accept` / `accept_for_session` / `decline` / `cancel` | 4-decision user response |
| `ToolKind`        | `command` / `fileChange` / `readOnly` / `unknown`     | approval-card icon hint |
| `Event`           | `{type, data}` where `data` is a JSON-encoded string  | atomic stream event |
| `ToolCallData`    | `{id, name, args, auto_run, kind}`                    | payload of `EventToolCall` |
| `ToolResultData`  | `{id, name, result, is_error, status, duration_ms}`   | payload of `EventToolResult` (`status ∈ success \| failed \| cancelled \| running`) |
| `ToolStatusData`  | `{id, status}`                                         | payload of `EventToolStatus` |
| `MessageData`     | in-memory only                                         | accumulator for a single assistant turn |

The wire format is intentionally a quoted JSON string for `Event.Data`
(so the agent can remain transport-agnostic); the front-end decodes
based on `Event.Type`.

---

## ToolRegistry

```go
type ToolDefinition struct {
    Schema      any
    Handler     func(args string) (string, error)
    NeedConfirm bool
    Kind        ToolKind
}

func NewRegistry() *ToolRegistry
func (r *ToolRegistry) Register(name string, schema any,
    handler func(args string) (string, error), needConfirm bool, kind ToolKind)
func (r *ToolRegistry) Get(name string) (ToolDefinition, bool)
func (r *ToolRegistry) GetAllSchemas() []any
```

- `Schema` is `any` so callers can pass any representation
  (`map[string]any`, `*jsonschema.Schema`, a Go struct, …).
- `Handler` receives the raw JSON args string from the LLM and
  returns a raw JSON result string.
- `NeedConfirm` is the source of truth for the `AutoRun` flag emitted
  on `ToolCallData`.
- All methods are safe for concurrent use (writer uses `Lock`,
  readers use `RLock`).

---

## Usage (preview of Phase 2+ API)

```go
registry := agent.NewRegistry()
registry.Register("list_files",
    map[string]any{ /* OpenAI function-calling schema */ },
    func(args string) (string, error) { /* call OpenList */ return "...", nil },
    false, // NeedConfirm
    agent.KindReadOnly,
)

if def, ok := registry.Get("list_files"); ok {
    out, err := def.Handler(`{"path":"/"}`)
    _ = out
    _ = err
}
```

---

## Test & verify

```bash
cd agent
go mod tidy

# ✅ 模块化（沙箱推荐）：单包跑
bash ../scripts/test-go.sh .

# ✅ CI 模式：全包跑
ENCV_TEST_FULL=1 bash ../scripts/test-all-go.sh
```

The Phase 1 tests must keep coverage ≥ 70 % on `types.go` and
`registry.go` combined.
