# Plugin Integration Guide

> **Audience**: encv-go maintainers who want to wire a new
> plugin into the in-process Go agent.

The agent (`/workspace/agent`) treats the seven encv-go
plugins as ordinary tools it can hand to the LLM. The
contract between the agent and a plugin is deliberately small
— a `PluginAdapter` interface plus an adapter implementation
in the demo binary.

## The 7 plugins

| Plugin | Container ext | PasswordStrategy | Extra fields |
|--------|---------------|------------------|--------------|
| `video` | `.sccgv` | global | `stream_preset`, `video_bitrate`, `audio_bitrate` |
| `audio` | `.sccga` | global | `audio_bitrate`, `sample_rate` |
| `image` | `.sccgi` | global | `quality`, `max_dimension` |
| `wps`   | `.sccgwps` | global | `compress_images`, `fn_rounds` |
| `pdf`   | `.sccgpdf` | global | `quality`, `fn_rounds` |
| `text`  | `.sccgt`  | global | `fn_rounds` |
| `alistencrypt` | (none — uses OpenList `/api/ext/*`) | independent | — |

> The `alistencrypt` plugin is **skipped** by `scanPluginTools`
> because its functionality is exposed through the OpenList
> tool set (`write_file` + the `/api/ext/*` extension
> endpoints).

## The PluginAdapter interface

`/workspace/agent/plugin_scanner.go` declares:

```go
type PluginAdapter interface {
    Name() string
    GetContainerExtension() string
    GetTaskOptions() PluginTaskOptions
    SetTaskExtraFields(map[string]string)
    PreEncryptProcessor(ctx, inputPath, inputRootDir, outputDir string) error
    Encrypt(reader io.Reader) (*EncryptionResult, error)
    PostEncryptProcessor(ctx, result *EncryptionResult) (string, error)
    CanDecrypt(containerPath string) bool
    PreDecryptProcessor(ctx, containerPath, outputDir string) error
    Decrypt(ctx, containerPath, outputDir string) (string, error)
    PostDecryptProcessor(ctx, containerPath string) error
}
```

This mirrors the methods in
`internal/v2/plugins/interfaces/interfaces.go`. The `*PluginTaskOptions`
return shape is in `types.go` and maps 1:1 onto
`interfaces.TaskOptions`.

## Wiring a plugin in `cmd/agent-demo`

`scanPluginTools(plugins)` consumes a slice of `PluginAdapter`
and emits two `ToolDefinition`s per plugin: `<name>_encrypt`
and `<name>_decrypt`. The demo wraps the real encv-go
`interfaces.Plugin` into a small adapter:

```go
// cmd/agent-demo/plugin_adapter.go (production only)
import (
    "github.com/encv/agent"
    "github.com/encv/internal/v2/plugins"
    "github.com/encv/internal/v2/plugins/interfaces"
)

type encvPluginAdapter struct{ p interfaces.Plugin }

func (a *encvPluginAdapter) Name() string             { return a.p.Name() }
func (a *encvPluginAdapter) GetContainerExtension() string { return a.p.GetContainerExtension() }
func (a *encvPluginAdapter) GetTaskOptions() agent.PluginTaskOptions {
    opts := a.p.GetTaskOptions()
    return agent.PluginTaskOptions{
        PasswordStrategy:      mapPwdStrategy(opts.PasswordStrategy),
        SupportVersionSelect:  opts.SupportVersionSelect,
        SupportedVersions:     opts.SupportedVersions,
        DefaultVersion:        opts.DefaultVersion,
        ExtraFields:           mapFields(opts.ExtraFields),
    }
}
// ... and the encrypt / decrypt methods follow the same pattern.
```

The demo's `registerPluginTools` then:

```go
adapters := make([]agent.PluginAdapter, 0, len(plugins.Plugins))
for _, p := range plugins.Plugins {
    adapters = append(adapters, &encvPluginAdapter{p: p})
}
tools, lookup, err := agent.ScanPluginTools(adapters)
if err != nil {
    log.Fatalf("scan plugins: %v", err)
}
for _, t := range tools {
    reg.Register(/* extract name from t.Schema */, t.Schema, t.Handler, t.NeedConfirm, t.Kind)
}
```

## Schema generation

`buildPluginSchema` (in `plugin_scanner.go`) emits a JSON-Schema
object suitable for OpenAI function calling. The schema
includes:

- `input_paths` (array of strings, required)
- `output_path` (string, required)
- `extra_fields` (object — per-plugin fields)
- `password` (string, **only** if
  `PasswordStrategy == PasswordIndependent`)
- `version` (integer, **only** if
  `SupportVersionSelect == true`, with `enum` populated from
  `SupportedVersions`)

The description is composed in Chinese and mentions the
plugin name, container extension, and password policy.

## Handler contract

`makePluginEncryptHandler(p)` returns a `func(args string) (string, error)`.
The handler:

1. Unmarshals `args` into `PluginInput{InputPaths, OutputPath, ExtraFields, Password, Version}`.
2. Calls `p.SetTaskExtraFields(ExtraFields)`.
3. For each inputPath, runs the three-step encrypt flow
   (PreEncrypt → Encrypt → PostEncrypt).
4. Returns a JSON payload of shape `PluginOutput{OutputPaths, DurationMs, Plugin, Operation}`.

The decrypt handler is symmetric and adds the `CanDecrypt`
self-check (spec §3.4) which, on failure, returns a
`container_format_mismatch` payload with a `suggested_tool`
field derived from the file extension.

## Adding a new plugin

1. Implement the plugin in `internal/v2/plugins/<name>/`,
   following the existing video / audio / etc. layout.
2. Register it in `internal/v2/plugins/registry.go`'s
   `Plugins` slice.
3. The agent's `scanPluginTools` will pick it up
   automatically on next start. No code change is needed in
   the agent core.
4. Add unit tests in `plugin_scanner_test.go` that exercise
   the new schema fragment.
5. Update the table at the top of this document.

## End-to-end test

The demo binary runs on `:5245`. After start:

```bash
# 1. List registered tools:
curl -s http://localhost:5245/api/agent/tools | jq

# 2. Trigger a turn that asks the agent to encrypt a file:
curl -N -X POST http://localhost:5245/api/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id": "smoke-1",
    "messages": [{"role":"user","content":"Encrypt /tmp/in.txt as video"}]
  }'

# 3. Confirm the tool call:
curl -N -X POST http://localhost:5245/api/confirm \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id": "smoke-1",
    "tool_call_id": "<id from the SSE stream>",
    "decision": "accept_for_session"
  }'
```

If the underlying encv-go plugin is not wired, the
`plugin_bridge_not_wired` error is returned and the SSE
stream terminates cleanly — the failure is a single
well-typed JSON line, not a 500.
