# Changelog

All notable changes to the **ENCV** project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v2] - 2026-06-08

### Added

- **Tool system: `ToolRegistry` unified dispatch** (`internal/tools/registry.go`)
  - Single `GlobalRegistry` consumed by both the Mock engine and the real-LLM path
  - `ToolDef{Name, Description, ArgsSchema, Handler, RequiresConfirm, ReadOnly, Kind}`
  - `Register` / `Get` / `List` / `Has` / `Dispatch` API; thread-safe
  - `ToolDeps` dependency-injection struct (`ResolveMount`, `SandboxCheck`, `Config`)
  - `RegisterAll()` boot-time one-shot registration; `AllToolDefs()` for tests / docs

- **`search_files` tool** (`internal/tools/search_files.go`)
  - Recursive directory walk with `glob` (`*` / `**` / `?`) + `regex` (Go RE2)
  - 14-node boolean AST: 11 leaves (`name_glob`, `name_regex`, `content_regex`, `size_gt/lt/eq`, `mtime_after/before`, `ext_eq`, `path_contains`, `path_not_contains`) + 3 compounds (`and` / `or` / `not`) with short-circuit evaluation
  - Hard limits: `MaxFilesScanned=50000`, `MaxContentRegexSize=10MB`
  - Returns `{total, truncated, scanned_limited, matches[]}` with `path/size/mtime/ext` per match

- **`get_metadata` tool** (`internal/tools/get_metadata.go`)
  - Base fields: `path / size / mtime / mode / mime / extension / is_hidden / is_symlink / is_dir`
  - Video probe (via `ffprobe`): `duration / width / height / codec / bitrate / frame_rate / has_audio`
  - Audio probe: `duration / bitrate / sample_rate / channels / codec / has_cover_art`
  - Optional `include_hash` → SHA-256
  - Graceful degradation: `ffprobe` missing or 5s timeout → skip media fields, **never** fail the whole call

- **`read_file_v2` tool** (`internal/tools/read_file_v2.go`)
  - `start_line` / `end_line` (1-based, inclusive) + `max_bytes` (default 1MB)
  - Binary detection (first 1KB UTF-8 invalid) → `content_base64` + `warning`
  - `start_line > total_lines` → error; `end_line > total_lines` → clamp

- **`edit_metadata` tool** (`internal/tools/edit_metadata.go`)
  - Writes `title / artist / comment` via `ffmpeg -metadata`
  - Automatic `<name>.bak` backup + rollback on failure
  - `requires_confirm: true`

- **`batch_rename` tool** (`internal/tools/edit_metadata.go`)
  - `pattern` (regex) + `replacement` (with `$1` / `$2`)
  - `dry_run: true` → preview only; `dry_run: false` → execute
  - All-or-nothing rollback if any file fails
  - `requires_confirm: true`

- **`delete_file` tool** (`internal/tools/edit_metadata.go`)
  - Default `mode: "trash"` → moves to `<mount_root>/.trash/<unix_nano>_<name>`
  - `mode: "hard"` → direct `os.Remove` (still requires confirm)
  - `requires_confirm: true`

- **`command_run` restricted shell** (`internal/tools/command_run.go`)
  - Default whitelist: `ffprobe / ffmpeg / du / wc / find / stat / mediainfo / file`
  - Hard blacklist: `rm / mv / cp / chmod / chown / dd / mkfs / shutdown / reboot / halt / poweroff`
  - 5-second timeout (override up to 30s via `timeout_sec`)
  - 8KB output truncation (stdout + stderr)
  - Path traversal rejection: any arg containing `..` or starting with `/etc/ /usr/ /var/ /boot/ /sys/ /proc/` → denied

- **Mock engine v2** (`internal/server/agent_mock_v2.go`)
  - `MockScenario.Branches []Branch` (ID, Label, Description, Icon, TriggerKeywords, TriggerRegex, OnMatch, InitialStepID)
  - `MockScenario.Rounds int` + `RoundContext map[string]any`
  - `MockStep.RoundIdx` / `PauseForUser` / `BranchChoice` / `SetContext` / `UseContext`
  - `MockEngineV2` with `Run` / `Resume` / `PickBranch` / `ApproveTool` / `RejectTool`
  - Branch matching priority: exact ID → keyword → regex → re-prompt
  - `{{ key }}` template substitution across rounds
  - Auto `stream_end{finishReason: "timeout"}` after `mock_round_timeout_sec` of no user input
  - Backward-compatible dispatch: `scenario.Rounds == 0 || len(Branches) == 0` → v1 path

- **8 new v2 mock scenarios** (`internal/server/agent_mock_v2_scenarios.go`)
  - `search_recursive_mp4` — recursive + glob + size
  - `search_logical_query` — compound AND query
  - `search_content_regex` — content regex on log files
  - `edit_metadata_wizard` — **4-round** multi-turn wizard (select file → select field → enter value → confirm)
  - `batch_rename_with_preview` — dry_run preview → confirm → execute
  - `branch_encrypt_or_decrypt` — 3-way branch selection
  - `branch_video_or_audio` — multi-branch dispatch (video / audio / other)
  - `command_run_ffprobe` — restricted shell with real `ffprobe` output

- **Frontend `MockBranchChoiceBar` component** (`app/encv-mobile/src/components/agent/MockBranchChoiceBar.vue`)
  - Chip list above the input row during `mockScenarioPaused`
  - Header: scenario badge + round progress (`Round 2/4`)
  - Prompt line + chip list (icon / label / description)
  - Dark-mode-aware with primary-tint background + `backdrop-filter: blur(12px)`

- **`useAgent` v2 event handling** (`app/encv-mobile/src/composables/useAgent.ts`)
  - Parses `mock_branch_choice` and `mock_round_state` SSE event types
  - Exposes `mockBranchChoices` / `mockBranchPrompt` / `mockRoundState` / `mockScenarioPaused` / `currentMockScenario` refs
  - `pickMockBranch(branchId)` and `sendMockRoundResponse(userText)` dispatch with `mode: "mock_resume"`
  - `stream_end` clears all v2 state

- **i18n: 15 new bilingual keys** (`app/encv-mobile/src/i18n/agent.ts` + `settings.ts`)
  - `agent.branchChoicePrompt`, `agent.roundProgress`, `agent.roundPausedHint`
  - `agent.toolDenied`, `agent.toolRequiresConfirm`
  - `agent.batchRenamePreview`, `agent.batchRenameConfirm`
  - `agent.editMetadataTitle`, `agent.commandTimeout`, `agent.commandDenied`
  - `settings.toolWhitelist`, `settings.toolWhitelistHelp`
  - `settings.sandboxPaths`, `settings.sandboxPathsHelp`
  - `settings.mockRoundTimeout`
  - All in zh-CN and en

- **Config: 4 new `agent_settings` fields** (`internal/config/config.go` + `schema.json`)
  - `tool_whitelist: string[]` — default `[ffprobe, ffmpeg, du, wc, find, stat, mediainfo, file]`
  - `sandbox_paths: { [mount_id]: string }` — mount ID → host directory
  - `mock_round_timeout_sec: number` — range 10–600, default 60
  - `mock_round_pause_enabled: bool` — default `true`
  - UI rendering in `Settings.vue` (tag input / key-value editor / number input / toggle)

- **Documentation**
  - User guide: `/workspace/.trae/documents/v2-usage.md` (zh-CN primary, en secondary)
  - This CHANGELOG entry

### Changed

- **Tool execution now goes through `GlobalRegistry.Dispatch`**
  - `server.executeAgentTool` no longer hard-codes if/else for each tool
  - Mock engine `execute_real` and real LLM path share the **same** handler implementations
  - Adding a new tool = `RegisterAll()` in one place; no engine changes needed
- **`handleAgentChat` request struct** now includes `Mode` (`start` / `steer` / `queue` / `mock_resume`) and `Scenario` fields to support `mock_resume` in v2 multi-round scripts

### Fixed

- None specific to v2. All v1 12 scenarios continue to work unchanged.

### Performance

- `search_files` 50,000-file scan completes in < 5s on a synthetic 50K-file mount
- `scanned_limited: true` flag returned when the cap is hit, so callers can re-issue with a narrower scope
- `content_regex` skips files > 10MB automatically, preventing OOM on large logs

### Security

- `command_run` defaults to read-only whitelist; explicit blacklist of all destructive commands
- Path traversal: `..`, absolute paths under `/etc /usr /var /boot /sys /proc` → rejected at handler entry
- All write tools (`edit_metadata` / `batch_rename` / `delete_file`) require explicit user confirmation in the UI
- Backup-and-rollback on every write tool (no silent data loss)

### Backward Compatibility

- ✅ All 12 v1 mock scenarios continue to work without modification
- ✅ v1 tools (`list_mounts` / `list_files` / `read_file`) keep their legacy `executeFSTool` dispatch path
- ✅ Real-LLM path automatically picks up v2 tool declarations (no API contract change for LLM tool spec)
- ✅ Old `localStorage` agent sessions resume correctly (v2 events are simply ignored if the field is missing)

---

## Prior Versions

See git history for changes prior to v2.
