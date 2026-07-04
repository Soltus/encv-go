# Agent Tools & Scenarios v2 Spec

## Why

现有 `agent-mock-mode` 已完成基础 12 个线性剧本 + `execute_real` 真实工具执行，但有四大缺口：

1. **工具集单薄** — 仅有 `list_mounts` / `list_files` / `read_file` 三个基础工具，**不支持**递归搜索、正则表达式、逻辑运算符（AND/OR/NOT），无法应对现实需求（如「找所有 > 100MB 且不在 subtitles 目录的 mp4」）
2. **数据真实度不足** — 多数剧本的 `tool_result` 是硬编码 JSON，与真实文件系统脱节，无法用于真机演示与截图
3. **剧本是单次线性流** — 推完 step1→step2→…→stream_end 即结束，**没有真正的分支选择**，无法表达「选 A 走加密 / 选 B 走解密」
4. **缺少多轮推进** — 用户必须等流结束再发新消息，无法在剧本中途与 assistant 反复交互（"再缩小一点时间范围" / "换一个 mount"）

新方案核心价值：
- **工具集现代化** — 补齐真实 LLM 助手所需的工具集（搜索/正则/逻辑/写操作/受限 shell），不再"工具不够用只能硬编码"
- **数据真实化** — 通过 `execute_real` 直接读真实 sandbox 文件，前端体验与生产一致
- **交互真实化** — 分支选择 + 多轮对话，让 mock 真正模拟"用户在和 AI 反复迭代"
- **完全向后兼容** — 现有 12 个剧本继续工作，新能力是**叠加**而不是**替换**

---

## What Changes

### 新增

**后端 - 工具层（`internal/tools/`）**
- `internal/tools/registry.go` — 工具注册中心（ToolRegistry）
  - 统一管理 tool 名 → handler / schema / 权限标签
  - 启动时把所有 v1 + v2 工具注册到全局
- `internal/tools/search_files.go` — 递归搜索工具 `search_files`
  - 递归遍历 mount 下的目录树
  - 支持 glob（`*` / `**` / `?`）
  - 支持 content regex（按文件内容匹配）
  - 支持 name regex（按文件名匹配）
  - 支持 size / mtime / extension 过滤
  - **支持布尔表达式**：AND / OR / NOT（带括号优先级）
- `internal/tools/get_metadata.go` — 元数据工具 `get_metadata`
  - 单文件：size / mtime / mime / extension
  - 视频：duration（ffprobe 包装）/ width / height / codec
  - 音频：duration / bitrate / sample_rate
  - 文件：sha256 哈希（按需）
- `internal/tools/read_file_v2.go` — `read_file` 增强
  - 支持 `start_line` / `end_line`（分页读取）
  - 支持 `max_bytes`（大文件截断）
  - 二进制检测（返回前 N 字节 + 提示）
  - UTF-8 校验失败时返回 base64 + warning
- `internal/tools/edit_metadata.go` — 写入元数据工具
  - 写 title / artist / album / comment（ID3 / MP4 atoms）
  - 需要 `requires_confirm: true`（权限拦截）
- `internal/tools/batch_rename.go` — 批量重命名
  - 支持 `pattern` (regex) + `replacement` (含 $1/$2)
  - 支持 dry-run 模式（先返回预览，用户确认后才真改）
- `internal/tools/delete_file.go` — 删除文件
  - 硬删除 vs 移到回收站（mount 配置决定）
  - requires_confirm
- `internal/tools/command_run.go` — 受限 shell
  - 工具白名单（默认 `ffprobe` / `du` / `wc` / `find` / `stat` / `mediainfo`）
  - 超时（默认 5s）
  - 输出截断（最大 8KB）

**后端 - 剧本引擎 v2（`internal/server/`）**
- `internal/server/agent_mock_v2.go` — 剧本 v2 引擎
  - `MockScenario` 加 `Branches []Branch` / `Rounds int` / `RoundContext map[string]any`
  - `MockStep` 加 `BranchID string` / `RoundIdx int` / `PauseForUser bool`
  - 新增 `mock_branch_choice` 事件类型：剧本推选项列表给前端
  - 新增 `mock_round_state` 事件：报告当前 round 进度
  - 新增 `MockScenario.Resume(ctx, userText, roundCtx) error` — 用户回复后恢复剧本
  - 状态机：`stream_start` → `round_state` → steps → (分支选择 → 等待 user_text → 恢复) → ... → `stream_end`
- `internal/server/agent_mock_v2_scenarios.go` — 8 个 v2 剧本
  - `search_recursive_mp4` — 演示 search_files 递归+glob+regex
  - `search_logical_query` — 演示 AND/OR/NOT 复合查询
  - `search_content_regex` — 演示 content 正则
  - `edit_metadata_wizard` — 演示 4 轮多轮对话（选文件→选字段→输入新值→确认）
  - `batch_rename_with_preview` — 演示 dry-run + 用户确认
  - `branch_encrypt_or_decrypt` — 演示 mock_branch_choice（3 选 1）
  - `branch_video_or_audio` — 演示多分支 + 跨分支多轮
  - `command_run_ffprobe` — 演示受限 shell + 真实输出

**后端 - 配置**
- `internal/config/config.go` 新增字段
  - `AgentSettings.ToolWhitelist []string`（默认 ffprobe/du/wc/find/stat/mediainfo）
  - `AgentSettings.SandboxPaths map[string]string`（mount_id → 真实目录映射）
  - `AgentSettings.MockRoundTimeoutSec int`（默认 60）
  - `AgentSettings.MockRoundPauseEnabled bool`（默认 true，是否允许剧本暂停等用户）

### 修改

- `internal/server/agent_api.go` — 注册 v2 工具到 ToolRegistry
- `internal/server/server.go` — `NewServer` 注入 ToolRegistry 到 MockEngine 的 `realExecutor`
- `internal/config/schema.json` — 加 `tool_whitelist` / `sandbox_paths` / `mock_round_timeout_sec` 字段
- `app/encv-mobile/src/composables/useAgent.ts` — 解析新事件
  - `case 'mock_branch_choice':` → 推 `mockBranchChoices` ref
  - `case 'mock_round_state':` → 推 `mockRoundState` ref
  - 新增 `pickMockBranch(branchId)` 函数
  - 新增 `sendMockRoundResponse(userText)` 函数（在剧本暂停时把用户消息送回）
- `app/encv-mobile/src/components/agent/MockBranchChoiceBar.vue`（新建）— 分支选择 chip 列表组件
- `app/encv-mobile/src/views/AgentChat.vue` — 集成 MockBranchChoiceBar
- `app/encv-mobile/src/i18n/agent.ts` — 新增 i18n key

### 不影响

- 现有 12 个 mock 剧本继续工作（向后兼容）
- 现有 `execute_real` 机制（仅扩展支持新工具，自动注册到 ToolRegistry）
- 真实 LLM 路径（继续走 callOpenAIStream，工具集变更不影响）
- 现有 MockPreset / MockPresetBar（v1 的 chip 列表组件继续用）

---

## ADDED Requirements

### Requirement: 工具注册中心 ToolRegistry

`internal/tools/registry.go` SHALL 提供全局工具注册表，启动时把所有 v1 + v2 工具注册进去，MockEngine 和真实 LLM 路径**共用同一个注册表**。

#### Scenario: 注册表接口

```go
type ToolHandler func(ctx context.Context, args json.RawMessage, deps *ToolDeps) (ToolResult, error)

type ToolDef struct {
    Name             string   // 工具 ID（如 "search_files"）
    Description      string   // 给 LLM 看的功能描述
    ArgsSchema       string   // JSON Schema 描述参数
    Handler          ToolHandler
    RequiresConfirm  bool     // 是否需要用户确认
    ReadOnly         bool     // 是否只读（写操作工具 = false）
    Kind             string   // "fileRead" | "fileChange" | "command" | "metadata"
}

var GlobalRegistry = &ToolRegistry{tools: map[string]*ToolDef{}}

func (r *ToolRegistry) Register(def *ToolDef) error
func (r *ToolRegistry) Get(name string) (*ToolDef, bool)
func (r *ToolRegistry) List() []*ToolDef
```

- **WHEN** 服务启动（`NewServer`）
- **THEN** 依次注册 8 个内置工具（list_mounts / list_files / read_file / get_metadata / search_files / edit_metadata / batch_rename / delete_file / command_run）
- **AND** v1 的 3 个旧工具（list_mounts / list_files / read_file）保留兼容

#### Scenario: execute_real 通过 ToolRegistry 派发

- **WHEN** 剧本某 step 标 `execute_real: true`，且 tool_call 名字在 ToolRegistry 中
- **THEN** MockEngine 调用 `registry.Get(name).Handler(ctx, args, deps)` 而非硬编码的 if-else 分支
- **AND** 返回的 `ToolResult.Result` 字段写入 `tool_result` 事件的 data
- **AND** Handler 内部错误 → `isError: true` + 错误消息

---

### Requirement: `search_files` 工具（核心）

`search_files` SHALL 支持递归遍历 + glob + regex + 复合布尔查询。

#### Scenario: 工具参数 schema

```json
{
  "mount_id": "string (required)",
  "rel_path": "string (default '/')",
  "recursive": "bool (default true)",
  "max_results": "int (default 200)",
  "expression": {
    "type": "object",
    "description": "复合布尔查询 AST",
    "anyOf": [
      {"$ref": "#/definitions/and"},
      {"$ref": "#/definitions/or"},
      {"$ref": "#/definitions/not"},
      {"$ref": "#/definitions/leaf"}
    ]
  }
}
```

#### Scenario: 叶子节点类型（leaf）

- `{"type": "name_glob", "value": "*.mp4"}` — 文件名 glob
- `{"type": "name_regex", "value": "studio_.*\\.mp4"}` — 文件名正则
- `{"type": "content_regex", "value": "error.*timeout"}` — 文件内容正则
- `{"type": "size_gt", "value": 104857600}` — size > 100MB
- `{"type": "size_lt", "value": 1048576}` — size < 1MB
- `{"type": "size_eq", "value": 0}` — size = 0（空文件）
- `{"type": "mtime_after", "value": "2026-01-01T00:00:00Z"}` — mtime > ts
- `{"type": "mtime_before", "value": "2026-12-31T23:59:59Z"}` — mtime < ts
- `{"type": "ext_eq", "value": "mp4"}` — extension 精确匹配（不区分大小写）
- `{"type": "path_contains", "value": "subtitles"}` — 路径包含子串
- `{"type": "path_not_contains", "value": "trash"}` — 路径**不**包含子串

#### Scenario: 复合节点类型

- `{"type": "and", "children": [<expr>, <expr>, ...]}` — 所有子表达式都满足
- `{"type": "or", "children": [<expr>, <expr>, ...]}` — 任一子表达式满足
- `{"type": "not", "child": <expr>}` — 子表达式不满足
- 嵌套：`{"and": [{"name_glob": "*.mp4"}, {"or": [{"size_gt": 100M}, {"mtime_after": "2026-01-01"}]}]}`

#### Scenario: 表达式求值算法

- 后端用**递归下降**实现求值器（参考 SQL WHERE 子句）
- AST 节点 → 谓词函数（返回 bool）
- 遇到未知节点类型 → 返回错误（`"unknown_expr_type: xxx"`）
- 短路求值：AND 第一个 false → 不再算后续；OR 第一个 true → 不再算后续

#### Scenario: 返回结果格式

```json
{
  "total": 42,
  "truncated": false,
  "matches": [
    {
      "path": "Movies/studio_video_1762.mp4",
      "size": 554000000,
      "mtime": "2026-01-15T10:30:00Z",
      "ext": "mp4"
    },
    ...
  ]
}
```

- `total` = 命中总数（即使被 truncated 也反映真实数量）
- `truncated` = 是否被 `max_results` 截断
- `matches` = 实际返回的条目（最多 max_results）

#### Scenario: 性能约束

- 单次搜索最多扫描 `max_files_scanned = 50000`（防止 mount 过大时阻塞）
- 超过上限 → 立即返回当前 partial result + `"scanned_limited": true` 字段
- content_regex 命中文件最大 10MB（> 10MB 文件跳过，正则不查内容）

---

### Requirement: `get_metadata` 工具

`get_metadata` SHALL 返回单文件的完整元信息。

#### Scenario: 通用元数据

所有文件都返回：
- `path` / `size` / `mtime` / `mode` / `mime`（按扩展名 + content sniffing）
- `extension`（小写，不含 `.`）
- `is_hidden` / `is_symlink` / `is_dir`

#### Scenario: 视频文件元数据

`mime` 命中 `video/*` 时调用 `ffprobe`（外部命令，需在 `tool_whitelist`）拿：
- `duration`（秒，浮点）/ `width` / `height` / `codec` / `bitrate` / `frame_rate` / `has_audio`

#### Scenario: 音频文件元数据

`mime` 命中 `audio/*` 时拿：
- `duration` / `bitrate` / `sample_rate` / `channels` / `codec` / `has_cover_art`

#### Scenario: ffprobe 失败容错

- ffprobe 缺失 → 跳过视频/音频字段，其他字段照常返回
- ffprobe 超时（5s）→ 同样跳过，记录 slog.Warn
- 不因 ffprobe 失败导致整个工具失败

---

### Requirement: `read_file` 增强

`read_file` v2 SHALL 支持分页 / 范围 / 二进制检测。

#### Scenario: 新增参数

```json
{
  "mount_id": "string (required)",
  "rel_path": "string (required)",
  "start_line": "int (optional, 1-based, default 1)",
  "end_line": "int (optional, 1-based, inclusive)",
  "max_bytes": "int (optional, default 1MB)"
}
```

#### Scenario: 返回格式

```json
{
  "path": "...",
  "total_lines": 1234,
  "total_bytes": 56789,
  "lines": ["line 1 content", "line 2 content", ...],
  "binary": false,
  "truncated": false,
  "encoding": "utf-8"
}
```

#### Scenario: 二进制文件处理

- `binary: true` → 返回 `content_base64: "..."` + `content_truncated: true`（超过 1KB 截断）
- 警告：`"Binary file detected. Use get_metadata or specialized tool."`

#### Scenario: 范围越界

- `start_line > total_lines` → 错误 `"start_line beyond end of file"`
- `end_line > total_lines` → 自动 clamp 到 total_lines
- `start_line > end_line` → 错误 `"start_line > end_line"`

---

### Requirement: 写入工具

`edit_metadata` / `batch_rename` / `delete_file` SHALL **全部**标 `requires_confirm: true`，走前端确认弹窗。

#### Scenario: `edit_metadata` 接口

```json
{
  "mount_id": "string",
  "rel_path": "string",
  "metadata": {
    "title": "...",
    "artist": "...",
    "comment": "..."
  }
}
```

- **WHEN** 用户在 AgentChat 收到 confirmation 弹窗并确认
- **THEN** 调真实 ffprobe + 写回 metadata（ffmpeg -i in -metadata key=value out）
- **AND** 工具返回 `"success": true` + 备份文件路径

#### Scenario: `batch_rename` 流程

- 第一次调用 `dry_run: true` → 返回变更预览（每个文件 old_path → new_path）
- 用户在前端确认 → 第二次调用 `dry_run: false` → 真实执行
- 失败回滚：任一文件失败 → 全部还原，记录错误

#### Scenario: `delete_file` 流程

- 默认走 mount 配置的回收站（`mounts.json` 的 `trash_path` 字段）
- 硬删除模式（mount 配 `trash_enabled: false`）→ 二次确认弹窗

---

### Requirement: `command_run` 受限 shell

`command_run` SHALL 仅执行白名单内的命令，防止误用。

#### Scenario: 白名单配置

- 默认白名单：`ffprobe` / `ffmpeg` / `du` / `wc` / `find` / `stat` / `mediainfo` / `file`
- 用户可在 `agent_settings.tool_whitelist` 覆盖（追加）
- 黑名单（任何情况下都拒绝）：`rm` / `mv` / `cp` / `chmod` / `chown` / `dd` / `mkfs` / `shutdown` / `reboot`

#### Scenario: 执行约束

- 单次执行 timeout = 5s（默认，可在 `command_run` args 中覆盖 `timeout_sec`）
- 输出超过 8KB 截断 + 标记 `"output_truncated": true`
- 退出码非 0 → `isError: true` + stderr 内容

#### Scenario: 路径沙箱

- 任何参数含 `..` 或绝对路径 `/etc` / `/usr` / `/var` → 直接拒绝（防越权）
- 只允许 mount_id → 真实路径映射下的文件

---

### Requirement: 工具权限模型

每个 ToolDef SHALL 携带权限标签，MockEngine / 真实路径 SHALL 统一处理。

#### Scenario: 权限标签

| 标签 | 工具 | 行为 |
|------|------|------|
| `requires_confirm: true` | edit_metadata / batch_rename / delete_file | 前端必须弹出 confirm 弹窗，用户同意后才执行 |
| `requires_confirm: false` | search_files / read_file / get_metadata / list_files | 自动执行 |
| `readonly: true` | search / read / list / get_metadata | 不能修改文件系统 |
| `readonly: false` | edit_metadata / batch_rename / delete_file | 修改文件系统 |
| `kind: "command"` | command_run | 受限 shell |

#### Scenario: 权限拦截

- **WHEN** MockEngine 准备执行 `requires_confirm: true` 的工具
- **THEN** 推 `tool_status {id, status: "pending_confirm"}` 事件给前端
- **AND** 暂停剧本流（不继续推后续 step）
- **AND** 前端弹窗 → 用户点击「确认」→ 调 `confirmTool(id, "approve")` → 恢复执行
- **AND** 用户点击「拒绝」→ `confirmTool(id, "reject")` → 推 `tool_status {status: "cancelled"}` + 跳过后续

---

### Requirement: 剧本 v2 分支字段

`MockScenario` SHALL 支持 `Branches []Branch` 字段，描述剧本内可走的不同路径。

#### Scenario: Branch 数据结构

```go
type Branch struct {
    ID          string      // 全局唯一
    Label       string      // chip 上显示
    Description string      // 详细说明
    Icon        string      // 可选 emoji
    TriggerKeywords []string  // 用户输入含这些词时自动匹配
    TriggerRegex    string    // 备选正则
    OnMatch     *MockScenario  // 匹配后跳到的子剧本（独立 stream + EventCache）
    InitialStepID string       // 可选：在新 stream 中从哪个 step 开始
}
```

#### Scenario: `mock_branch_choice` 事件

- **WHEN** 剧本某 step 标 `branch_choice: true` 且有 `Branches` 字段
- **THEN** MockEngine 推 `mock_branch_choice` 事件，data 形状：
  ```json
  {
    "scenario": "branch_encrypt_or_decrypt",
    "step_id": "choose_action",
    "prompt": "请选择操作类型：",
    "branches": [
      {"id": "encrypt", "label": "加密", "icon": "🔒", "description": "..."},
      {"id": "decrypt", "label": "解密", "icon": "🔓", "description": "..."},
      {"id": "transcode", "label": "转码", "icon": "🔄", "description": "..."}
    ]
  }
  ```
- **AND** 暂停剧本，等待用户输入
- **AND** 前端渲染为 MockBranchChoiceBar（chip 列表）
- **AND** 用户点击 chip / 直接键入 → 触发 branch 选择

#### Scenario: 用户文本自动匹配 Branch

- **WHEN** 剧本在 `mock_branch_choice` 暂停状态收到 `user_text`
- **THEN** 按优先级匹配 Branch：
  1. 精确匹配 `branch.ID == userText`（如 userText = "encrypt"）
  2. 关键词匹配（任一 keyword 出现在 userText）
  3. 正则匹配
  4. 都不匹配 → 推 `mock_branch_choice` 再次提示用户
- **AND** 匹配成功 → 调 `OnMatch` 子剧本 / 跳到 `InitialStepID`

---

### Requirement: 剧本内多轮状态机

`MockScenario` SHALL 支持 `Rounds int` 字段 + `RoundContext map[string]any`，实现"剧本内部多轮对话"。

#### Scenario: Round 推进协议

```
Round 0: stream_start → round_state{0} → steps[0..N] → (可能 mock_branch_choice)
        ↓ 用户回复
Round 1: round_state{1} → steps[N+1..M] → ...
        ↓ ...
Round R-1: round_state{R-1} → steps[last] → stream_end → mock_presets_clear
```

#### Scenario: RoundContext 跨轮变量

- **WHEN** Round K 的某 step 标 `set_context: {"user_choice": "encrypt"}`
- **THEN** 写入 `RoundContext["user_choice"] = "encrypt"`
- **AND** Round K+1 的 step 可通过 `use_context: "user_choice"` 读取
- **AND** 读取方式：在 step 的 event data 模板中用 `{{ ctx.user_choice }}` 插值

#### Scenario: 状态机事件 `mock_round_state`

```json
{
  "scenario": "edit_metadata_wizard",
  "round_idx": 1,
  "total_rounds": 4,
  "phase": "awaiting_user_input",
  "context": {
    "selected_file": "Movies/a.mp4"
  }
}
```

- `phase` 枚举：`"running"` / `"awaiting_user_input"` / `"awaiting_branch_choice"`
- 前端 `useAgent` 推送到 `mockRoundState` ref
- AgentChat 可选择性显示 round 进度条（header 或 footer）

#### Scenario: 暂停 vs 继续

- **WHEN** step 标 `pause_for_user: true` 且 status==idle
- **THEN** 推 `mock_round_state {phase: "awaiting_user_input"}` 后停止推事件
- **AND** SSE 连接保持打开（不关闭 stream）
- **AND** 用户发送新 user_text → MockEngine 调 `Resume(ctx, userText, roundCtx)` → 继续推下一 round

#### Scenario: 取消与超时

- 用户点取消 / 关掉 modal → 推 `stream_end {finishReason: "cancelled"}`
- `mock_round_timeout_sec` (默认 60s) 超过无 user_text → 自动推 `stream_end {finishReason: "timeout"}`

---

### Requirement: 8 个 v2 剧本

新增 8 个 v2 剧本，覆盖搜索/正则/逻辑/写操作/分支/多轮全部新能力。

#### Scenario: 剧本清单

| ID | 触发关键词 | 演示能力 |
|----|----------|---------|
| `search_recursive_mp4` | "找视频" / "所有视频" | search_files 递归 + glob *.mp4 + size > 100MB |
| `search_logical_query` | "大视频" + "2025" | 复合 AND (size_gt + mtime_after + ext_eq) |
| `search_content_regex` | "日志报错" | content_regex "ERROR.*timeout" |
| `edit_metadata_wizard` | "改标题" | 4 轮：选文件 → 选字段 → 输入新值 → 确认写入 |
| `batch_rename_with_preview` | "批量改名" | dry_run 预览 → 用户确认 → 真实执行 |
| `branch_encrypt_or_decrypt` | "加密或解密" | mock_branch_choice 3 选 1 |
| `branch_video_or_audio` | "媒体信息" | 多分支（视频走 ffprobe / 音频走 ffprobe / 其他走 get_metadata） |
| `command_run_ffprobe` | "ffprobe 一下" | command_run 受限 shell 真实输出 |

#### Scenario: 高级剧本示例 — `edit_metadata_wizard`

```
Round 0:
  step 1: text_delta "我来帮你修改元数据。"
  step 2: tool_call search_files {expression: {and: [name_glob "*.mp4", size_gt 0]}}
  step 3: tool_result (execute_real, 真实搜索) → 列出候选
  step 4: mock_round_state {phase: awaiting_user_input} (列出候选 + 暂停)
  step 5: set_context.selected_file = user_text

Round 1 (用户说 "第一个"):
  step 1: text_delta "好的，你想改哪些字段？"
  step 2: mock_branch_choice [title / artist / comment]
  step 3 (用户选 "title"):

Round 2:
  step 1: text_delta "请输入新的标题："
  step 2: mock_round_state {phase: awaiting_user_input}
  step 3: set_context.new_title = user_text

Round 3:
  step 1: text_delta "准备写入...\n文件: {{ctx.selected_file}}\n新标题: {{ctx.new_title}}"
  step 2: tool_call edit_metadata {requires_confirm: true}
  step 3 (用户确认):
  step 4: tool_result (execute_real, 真实写入)
  step 5: text_delta "写入完成。"
  step 6: stream_end
```

---

### Requirement: 前端 useAgent 解析新事件

`useAgent` SHALL 解析 `mock_branch_choice` / `mock_round_state` 事件并暴露 ref。

#### Scenario: 事件解析

```typescript
type AgentEvent = ... | {type: 'mock_branch_choice', data: {scenario, stepId, prompt, branches: MockBranch[]}}
                      | {type: 'mock_round_state', data: {scenario, roundIdx, totalRounds, phase, context: Record<string, any>}}

interface MockBranch {
  id: string
  label: string
  icon?: string
  description?: string
}

const mockBranchChoices = ref<MockBranch[]>([])
const mockBranchPrompt = ref('')
const mockRoundState = ref<{roundIdx: number, totalRounds: number, phase: string, context: any} | null>(null)
const mockScenarioPaused = computed(() =>
  mockRoundState.value?.phase === 'awaiting_user_input' ||
  mockRoundState.value?.phase === 'awaiting_branch_choice'
)
```

#### Scenario: 选 Branch / 送 user_text

```typescript
function pickMockBranch(branchId: string): void {
  send(branchId, { mode: 'mock_resume', scenario: currentMockScenario.value })
  // 后端 MockEngine.Resume 收到 "encrypt" → 跳 OnMatch
}

function sendMockRoundResponse(userText: string): void {
  send(userText, { mode: 'mock_resume' })
  // 后端 MockEngine.Resume 推进下一 round
}
```

#### Scenario: 状态机

- 收到 `mock_branch_choice` → `mockBranchChoices` 填充 + `mockRoundState.phase = awaiting_branch_choice`
- 收到 `mock_round_state` → `mockRoundState` 更新
- 收到 `stream_end` → 清空 `mockBranchChoices` + `mockRoundState = null`
- 用户点 chip → `pickMockBranch(id)` → 触发 send → 后端 Resume

---

### Requirement: MockBranchChoiceBar 组件

`MockBranchChoiceBar.vue` SHALL 是 AgentChat 输入框上方的分支选择 chip 列表。

#### Scenario: 组件契约

```vue
<MockBranchChoiceBar
  v-if="mockScenarioPaused"
  :branches="mockBranchChoices"
  :prompt="mockBranchPrompt"
  :round-state="mockRoundState"
  @pick="(branch) => pickMockBranch(branch.id)"
  @type="(text) => sendMockRoundResponse(text)"
/>
```

- 仅在 `mockScenarioPaused === true` 时渲染
- header 显示：🧪 + scenario 名 + 当前 round（如 "Round 2/4"）
- 提示语：`{{ prompt }}`
- chip 列表：水平滚动，每个 chip 含 icon + label + 可选 description
- chip 下方有 textarea（用户也可直接键入文本而非点 chip）
- 暗黑模式：与 MockPresetBar 一致（半透明 primary tint）

#### Scenario: 暂停状态禁用输入

- **WHEN** `mockScenarioPaused === true`
- **THEN** 底部 `footerInputRow` 的 textarea 仍可用（用户可键入自由文本）
- **AND** 发送按钮在用户未输入时显示「点击 chip 继续」hint

---

### Requirement: 配置 schema 增量

`internal/config/schema.json` SHALL 新增以下字段：

```json
{
  "agent_settings": {
    "properties": {
      "tool_whitelist": {
        "type": "array",
        "items": {"type": "string"},
        "default": ["ffprobe", "ffmpeg", "du", "wc", "find", "stat", "mediainfo", "file"],
        "description": "agent.toolWhitelist"
      },
      "sandbox_paths": {
        "type": "object",
        "description": "agent.sandboxPaths",
        "default": {}
      },
      "mock_round_timeout_sec": {
        "type": "number",
        "min": 10,
        "max": 600,
        "default": 60,
        "description": "agent.mockRoundTimeout"
      },
      "mock_round_pause_enabled": {
        "type": "bool",
        "default": true,
        "description": "agent.mockRoundPauseEnabled"
      }
    }
  }
}
```

#### Scenario: 字段渲染

- `tool_whitelist` → 多行 tag input，每行一个命令
- `sandbox_paths` → key-value 编辑器（mount_id → 路径）
- `mock_round_timeout_sec` → number input
- `mock_round_pause_enabled` → 开关

---

### Requirement: i18n 增量

`app/encv-mobile/src/i18n/agent.ts` 新增：

| key | zh-CN | en |
|-----|-------|---|
| `agent.branchChoicePrompt` | 请选择操作： | Choose an action: |
| `agent.roundProgress` | 第 {round}/{total} 轮 | Round {round}/{total} |
| `agent.roundPausedHint` | 点击 chip 继续或键入文本 | Click a chip or type to continue |
| `agent.toolDenied` | 工具被拒绝 | Tool denied |
| `agent.toolRequiresConfirm` | 工具需要确认 | Tool requires confirmation |
| `agent.batchRenamePreview` | 改名预览（{count} 个文件） | Rename preview ({count} files) |
| `agent.batchRenameConfirm` | 确认改名 | Confirm rename |
| `agent.editMetadataTitle` | 修改元数据 | Edit metadata |
| `agent.commandTimeout` | 命令执行超时 | Command timeout |
| `agent.commandDenied` | 命令不在白名单 | Command not in whitelist |

`app/encv-mobile/src/i18n/settings.ts` 新增：

| key | zh-CN | en |
|-----|-------|---|
| `settings.toolWhitelist` | 工具白名单 | Tool whitelist |
| `settings.toolWhitelistHelp` | 受限 shell 工具允许的命令 | Allowed shell commands |
| `settings.sandboxPaths` | 沙箱路径映射 | Sandbox path mapping |
| `settings.sandboxPathsHelp` | mount_id → 真实目录 | mount_id → real directory |
| `settings.mockRoundTimeout` | 多轮暂停超时 | Round pause timeout |

---

### Requirement: 单元测试覆盖

#### Scenario: 工具层测试（30+ 用例）

- [x] `TestSearchFiles_NameGlob` — 验证 `*.mp4` 命中
- [x] `TestSearchFiles_NameRegex` — 验证 `studio_.*\.mp4`
- [x] `TestSearchFiles_ContentRegex` — 验证日志 ERROR 匹配
- [x] `TestSearchFiles_SizeGt` — 验证 size > 100MB
- [x] `TestSearchFiles_SizeEqZero` — 验证空文件
- [x] `TestSearchFiles_MtimeAfter` — 验证 mtime 时间窗
- [x] `TestSearchFiles_ExtEq` — 验证扩展名匹配（不区分大小写）
- [x] `TestSearchFiles_PathContains` / `PathNotContains`
- [x] `TestSearchFiles_And_ShortCircuit` — AND 第一个 false 后续不执行
- [x] `TestSearchFiles_Or_ShortCircuit` — OR 第一个 true 后续不执行
- [x] `TestSearchFiles_Not` — NOT 取反
- [x] `TestSearchFiles_Nested` — 嵌套 AND/OR/NOT
- [x] `TestSearchFiles_MaxResults` — 截断
- [x] `TestSearchFiles_RecursiveLimit` — 50000 文件上限
- [x] `TestSearchFiles_UnknownExprType` — 错误类型容错
- [x] `TestGetMetadata_Video` — ffprobe 调用
- [x] `TestGetMetadata_Audio`
- [x] `TestGetMetadata_NoFFprobe` — 缺失容错
- [x] `TestGetMetadata_FFprobeTimeout` — 超时容错
- [x] `TestReadFileV2_RangeLines` — start_line / end_line
- [x] `TestReadFileV2_MaxBytes`
- [x] `TestReadFileV2_Binary` — 二进制检测
- [x] `TestReadFileV2_StartLineBeyondEnd`
- [x] `TestCommandRun_AllowedCommand`
- [x] `TestCommandRun_DeniedCommand` — `rm` 被拒
- [x] `TestCommandRun_Timeout`
- [x] `TestCommandRun_OutputTruncate`
- [x] `TestCommandRun_PathTraversal_Denied`
- [x] `TestEditMetadata_WritesID3`
- [x] `TestBatchRename_DryRun` — 不真实改文件
- [x] `TestBatchRename_RollbackOnFailure`
- [x] `TestDeleteFile_TrashPath`
- [x] `TestDeleteFile_HardDelete_WithDoubleConfirm`

#### Scenario: 剧本 v2 引擎测试（20+ 用例）

- [x] `TestMockEngineV2_BranchChoice_PausesUntilUserPicks`
- [x] `TestMockEngineV2_BranchChoice_KeywordMatch`
- [x] `TestMockEngineV2_BranchChoice_RegexMatch`
- [x] `TestMockEngineV2_BranchChoice_NoMatchRePrompts`
- [x] `TestMockEngineV2_MultiRound_AdvancesOnUserText`
- [x] `TestMockEngineV2_RoundContext_SetAndUse`
- [x] `TestMockEngineV2_RoundTimeout_CancelsStream`
- [x] `TestMockEngineV2_PauseForUser_ResumesOnSend`
- [x] `TestMockEngineV2_ToolRequiresConfirm_BlocksUntilApproved`
- [x] `TestMockEngineV2_ToolRequiresConfirm_RejectionCancelsStep`
- [x] `TestMockEngineV2_SearchRecursive_8ScenariosRun`
- [x] `TestMockEngineV2_EditMetadataWizard_4Rounds`
- [x] `TestMockEngineV2_BatchRename_DryRunThenExecute`
- [x] `TestMockEngineV2_Branch_EncryptOrDecrypt_DispatchesToSubScenario`
- [x] `TestMockEngineV2_CommandRun_FFprobeRealOutput`
- [x] `TestMockEngineV2_ContextTemplate_Renders`
- [x] `TestMockEngineV2_BranchEvent_DataShape`
- [x] `TestMockEngineV2_RoundStateEvent_DataShape`
- [x] `TestMockEngineV2_StreamEndClearsBranchChoices`
- [x] `TestMockEngineV2_CompatWithV1Scenarios` — v1 剧本继续工作

#### Scenario: 端到端测试

- [x] `TestE2E_SearchFiles_RealMount` — 真实 mount 递归搜索
- [x] `TestE2E_EditMetadata_MultiRound` — 4 轮 + 真实写入
- [x] `TestE2E_BranchChoice_UserPicksEncrypt` — 触发 OnMatch 子剧本
- [x] `TestE2E_CommandRun_FFprobeOutputsJSON`

---

## MODIFIED Requirements

### Requirement: handleAgentChat 工具注册路径变更

**BEFORE**: MockEngine 的 `execute_real` 通过 `server.executeAgentTool` 硬编码 if-else 分发
**AFTER**: 通过 `ToolRegistry.Get(name).Handler(ctx, args, deps)` 派发

#### Scenario: 旧工具兼容

- v1 的 3 个工具（list_mounts / list_files / read_file）迁移到新 ToolRegistry 但 handler 实现保持兼容
- 现有 12 个剧本不需要任何修改

#### Scenario: 新工具自动可用

- 8 个 v2 工具（search_files / get_metadata / read_file_v2 / edit_metadata / batch_rename / delete_file / command_run）注册到 GlobalRegistry
- 现有 12 个剧本**无需改动**即可在 step 中调新工具（通过 name 匹配）
- 建议在剧本中加新 step 演示新工具（这就是 v2 剧本的目的）

---

### Requirement: 真实 LLM 路径工具声明

**BEFORE**: 真实 OpenAI 调用时 tools 数组只有 v1 工具
**AFTER**: 真实 OpenAI 调用时 tools 数组包含 v1 + v2 全部 11 个工具

#### Scenario: 工具声明格式

```json
{
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "search_files",
        "description": "递归搜索文件，支持 glob/正则/逻辑运算",
        "parameters": {
          "type": "object",
          "properties": {
            "mount_id": {"type": "string"},
            "rel_path": {"type": "string"},
            "expression": {"type": "object", "description": "AST"},
            ...
          }
        }
      }
    },
    ...
  ]
}
```

- 真实 LLM 现在**能**用 search_files / get_metadata / command_run
- handler 与 MockEngine 共用 → 行为一致

---

## REMOVED Requirements

无（仅新增 + 增量修改，不删除任何现有能力）

---

## 约束与限制

1. **完全向后兼容** — 现有 12 个 mock 剧本 + `execute_real` + MockPreset 必须继续工作
2. **search_files 性能** — 50000 文件上限 + 10MB content 上限，防止 mount 过大时阻塞
3. **受限 shell** — 任何情况下 `rm` / `mv` / `shutdown` 等黑名单命令被拒
4. **路径沙箱** — 任何参数含 `..` / `/etc` / `/usr` 等敏感路径直接拒绝
5. **分支/多轮超时** — `mock_round_timeout_sec` 默认 60s，超过自动 stream_end
6. **工具权限** — `requires_confirm: true` 的工具必须前端确认才能执行
7. **ToolRegistry 单一源** — MockEngine 和真实 LLM 路径**共用**同一个注册表，handler 同一份代码
8. **i18n 同步** — 10+ 个新 key 必须 zh-CN + en 双语

---

## 与现有 spec 的关系

| 现有 spec | 关系 |
|----------|------|
| `agent-mock-mode` | **基础** — 本 spec 在其之上叠加 v2 能力（branching / multi-round / new tools） |
| `agui-real-llm-path-completion` | **受益** — 真实 LLM 路径自动获得 v2 工具声明 |
| `mock-router-refactor` | 无关 — 前端 dev mock 路由 |
| `go-in-process-agent` | **修改点** — `executeAgentTool` 改为派发到 ToolRegistry |

---

## 验证步骤

1. **工具层单元测试** — `go test ./internal/tools/... -v` 30+ 全部通过
2. **剧本 v2 单元测试** — `go test ./internal/server/... -run TestMockEngineV2 -v` 20+ 全部通过
3. **端到端测试** — `go test ./internal/server/... -run TestE2E -v` 4+ 全部通过
4. **类型检查** — `go build ./cmd/encv` 0 错误
5. **前端类型** — `vue-tsc --noEmit` 0 错误
6. **前端构建** — `vite build` 0 错误
7. **集成验证** — 启动服务 → 切 mock_mode=builtin → 提问"找视频" → 验证：
   - 触发 `search_recursive_mp4` 剧本
   - 工具调用显示 `search_files` 卡片 + 参数（glob/regex/布尔表达式可视化）
   - 真实 mount 下的文件被搜索（不是硬编码）
8. **多轮验证** — 提问"改标题" → 4 轮交互完成真实 metadata 写入
9. **分支验证** — 提问"加密或解密" → chip 选择 → 跳转到对应子剧本

---

## 关键文件 / 函数

| 文件 | 关键类型/函数 |
|------|--------------|
| `internal/tools/registry.go` | `ToolRegistry` / `ToolDef` / `Register` / `Get` / `List` |
| `internal/tools/search_files.go` | `SearchFiles` / `evalExpr` / 11 种叶子 + 3 种复合节点 |
| `internal/tools/get_metadata.go` | `GetMetadata` / `probeMedia` (ffprobe 包装) |
| `internal/tools/read_file_v2.go` | `ReadFileV2` / `detectBinary` |
| `internal/tools/edit_metadata.go` | `EditMetadata` / `writeID3` |
| `internal/tools/batch_rename.go` | `BatchRename` / `dryRun` / `rollback` |
| `internal/tools/delete_file.go` | `DeleteFile` / `trashPath` |
| `internal/tools/command_run.go` | `CommandRun` / `whitelist` / `sandboxPath` |
| `internal/server/agent_mock_v2.go` | `MockEngineV2` / `Branch` / `Resume` / `runRounds` |
| `internal/server/agent_mock_v2_scenarios.go` | 8 个 v2 剧本 |
| `app/encv-mobile/src/composables/useAgent.ts` | `mockBranchChoices` / `mockRoundState` / `pickMockBranch` / `sendMockRoundResponse` |
| `app/encv-mobile/src/components/agent/MockBranchChoiceBar.vue` | 分支 chip + 轮次进度 + 自由键入 |
| `app/encv-mobile/src/views/AgentChat.vue` | 集成 MockBranchChoiceBar |
