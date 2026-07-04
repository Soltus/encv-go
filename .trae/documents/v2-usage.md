# Agent Tools & Scenarios v2 — User Guide

> **v2 工具系统 / 分支剧本 / 多轮交互** — 用户文档
> 适用于后端 agent 服务 v2 能力（搜索 / 写入 / 受限 shell / 剧本分支 / 剧本多轮）。
>
> **Spec**：`/workspace/.trae/specs/agent-tools-scenarios-v2/spec.md`
> **Tasks**：`/workspace/.trae/specs/agent-tools-scenarios-v2/tasks.md`
> **Code root**：`internal/tools/` · `internal/server/agent_mock_v2*.go` · `app/encv-mobile/src/components/agent/MockBranchChoiceBar.vue`

---

## 1. 概览 (Overview)

### 中文

v2 在现有 12 个线性 mock 剧本 + 3 个 v1 工具的基础上，**叠加**（不替换）以下能力：

| 维度 | v1 | v2 |
|------|----|----|
| 工具数量 | 3（`list_mounts` / `list_files` / `read_file`） | 7（`search_files` / `get_metadata` / `read_file_v2` / `edit_metadata` / `batch_rename` / `delete_file` / `command_run`） |
| 搜索能力 | 文件名精确匹配 | 递归 + glob + regex + **AND / OR / NOT** 复合布尔表达式 |
| 文件读取 | 全文一次 | 分页（`start_line` / `end_line`）+ 二进制检测 |
| 元数据 | 无 | 基础字段 + 视频/音频 ffprobe + 可选 SHA-256 |
| 写入工具 | 无 | `edit_metadata` / `batch_rename` / `delete_file`，全部 `requires_confirm` |
| 受限 shell | 无 | `command_run`，白名单 + 黑名单 + 路径沙箱 + 5s 超时 + 8KB 截断 |
| 剧本 | 12 个线性 | 12 个 v1 + **8 个 v2**（含分支 / 多轮 / dry-run / 真实工具） |
| 状态机 | 一次性流 | **分支选择** + **多轮推进** + round context 跨轮变量 |
| 调度 | `executeFSTool` 硬编码 if-else | **`GlobalRegistry` 统一派发**（Mock / 真实 LLM 共用一份 handler） |
| 向后兼容 | — | v1 剧本 + v1 工具继续工作，零破坏 |

### English

v2 **adds** (does not replace) the following on top of v1's 12 linear mock scenarios + 3 v1 tools:

- **Unified `ToolRegistry` dispatch** — Mock engine and real-LLM path share the same `Handler` implementations.
- **7 new tools** covering file search (with boolean AST), metadata (with ffprobe), paginated/binary-aware read, metadata editing, batch rename, deletion, and a restricted shell.
- **8 new mock scenarios** demonstrating recursive search, compound queries, content regex, 4-round metadata wizard, dry-run rename, branch selection (encrypt/decrypt), multi-branch (video/audio/other), and real `ffprobe` via `command_run`.
- **State machine v2** — scripts can declare `Branches` and `Rounds`, pause mid-stream for user input, and resume via `mode: mock_resume`.
- **Round context** — variables (`set_context` / `use_context`) flow across rounds with `{{ key }}` template substitution.
- **Full backward compatibility** — all 12 v1 scenarios keep working without modification.

---

## 2. 工具清单 (Tool Catalog)

后端 `internal/tools/` 目录下 7 个 v2 工具 + 3 个 v1 工具（v1 走 `agent_fs_bridge.executeFSTool` 旧路径，**不**经过 `GlobalRegistry`）。

| Name (kind) | readonly | requires_confirm | Kind | One-line description |
|-------------|:---:|:---:|------|----------------------|
| `list_mounts` ⭐ v1 | ✅ | ❌ | `fileRead` | 列出所有挂载点 |
| `list_files` ⭐ v1 | ✅ | ❌ | `fileRead` | 列出单目录内容 |
| `read_file` ⭐ v1 | ✅ | ❌ | `fileRead` | 读取完整文件（旧路径，不分页） |
| **`search_files`** | ✅ | ❌ | `fileRead` | 递归 + glob + regex + AND/OR/NOT 复合查询 |
| **`get_metadata`** | ✅ | ❌ | `metadata` | 单文件元信息 + 视频/音频 ffprobe + 可选 SHA-256 |
| **`read_file_v2`** | ✅ | ❌ | `fileRead` | 分页读取（`start_line`/`end_line`/`max_bytes`）+ 二进制检测 |
| **`edit_metadata`** | ❌ | ✅ | `fileChange` | 写 title / artist / comment（ID3 / MP4 atoms），自动备份+失败回滚 |
| **`batch_rename`** | ❌ | ✅ | `fileChange` | regex + replacement，含 `dry_run` 预览，失败全量回滚 |
| **`delete_file`** | ❌ | ✅ | `fileChange` | 默认走 `trash` 目录；`mode=hard` 二次确认 |
| **`command_run`** | ✅ | ❌ | `command` | 受限 shell：白名单 + 黑名单 + 路径沙箱 + 5s 超时 + 8KB 截断 |

⭐ = v1 工具，保留兼容，不在 `GlobalRegistry` 中注册。

### 按能力分组 (Grouped by capability)

#### 📖 文件读取 (fileRead)
- `list_mounts` / `list_files` / `read_file` ⭐ v1
- `search_files` — **核心新工具**，详见 §3
- `read_file_v2` — 分页 / 二进制检测

#### 🔍 元数据 (metadata)
- `get_metadata` — 基础字段 + 视频/音频 ffprobe

#### ✏️ 写入 (fileChange) — 全部 `requires_confirm: true`
- `edit_metadata` — 媒体元数据
- `batch_rename` — 批量改名 + dry_run
- `delete_file` — 删除 / 回收站

#### 🖥️ Shell (command) — 受限
- `command_run` — 白名单 + 沙箱 + 超时 + 截断

---

## 3. `search_files` 深度使用 (Deep Dive)

> 实现：`internal/tools/search_files.go` · 编译时 `compileExpr` 把 AST 预编译成闭包，遍历时短路求值。

### 参数 (Args)

```json
{
  "mount_id":   "string (required)",
  "rel_path":   "string (default '/')",
  "recursive":  "bool (default true)",
  "max_results": "int (default 200, max 1000)",
  "expression": { /* AST，见下文 */ }
}
```

### AST 节点参考 (Node Reference)

#### 叶子节点 (11 种)

| type | value 类型 | 含义 |
|------|------------|------|
| `name_glob` | `string` | 文件名 glob（`*` / `**` / `?`） |
| `name_regex` | `string` | 文件名正则（Go RE2） |
| `content_regex` | `string` | 文件内容正则（>10MB 文件跳过） |
| `size_gt` | `int` (bytes) | `size > N` |
| `size_lt` | `int` (bytes) | `size < N` |
| `size_eq` | `int` (bytes) | `size == N` |
| `mtime_after` | `string` (RFC3339 或 `YYYY-MM-DD`) | `mtime > ts` |
| `mtime_before` | `string` | `mtime < ts` |
| `ext_eq` | `string` (大小写不敏感，不含 `.`) | 扩展名精确匹配 |
| `path_contains` | `string` | 相对路径**包含**子串 |
| `path_not_contains` | `string` | 相对路径**不包含**子串 |

#### 复合节点 (3 种)

| type | 子节点字段 | 含义 |
|------|-----------|------|
| `and` | `children: [<expr>, ...]` | 全部子表达式满足（短路：第一个 false 即返回） |
| `or` | `children: [<expr>, ...]` | 任一子表达式满足（短路：第一个 true 即返回） |
| `not` | `child: <expr>` | 子表达式不满足 |

### 表达式示例 (Examples)

> 所有示例假定 mount_id = `"local"`, recurse = true（默认）。

#### ① 找所有 mp4（仅 glob）

```json
{
  "expression": {"type": "name_glob", "value": "*.mp4"}
}
```

#### ② 找 > 100MB 的视频

```json
{
  "expression": {
    "and": [
      {"type": "ext_eq", "value": "mp4"},
      {"type": "size_gt", "value": 104857600}
    ]
  }
}
```

#### ③ 找含 `ERROR.*timeout` 的日志

```json
{
  "expression": {
    "and": [
      {"type": "content_regex", "value": "ERROR.*timeout"},
      {"type": "ext_eq", "value": "log"}
    ]
  }
}
```

#### ④ 文件名正则：`studio_*.mp4`

```json
{
  "expression": {"type": "name_regex", "value": "studio_.*\\.mp4"}
}
```

#### ⑤ AND(size > 1MB, ext = log, mtime after 2025-01-01)

```json
{
  "expression": {
    "and": [
      {"type": "size_gt",  "value": 1048576},
      {"type": "ext_eq",   "value": "log"},
      {"type": "mtime_after", "value": "2025-01-01T00:00:00Z"}
    ]
  }
}
```

#### ⑥ 嵌套 OR + NOT：2024 之后的大视频，**排除** trash 目录

```json
{
  "expression": {
    "and": [
      {"type": "ext_eq", "value": "mp4"},
      {"type": "size_gt", "value": 104857600},
      {"type": "mtime_after", "value": "2024-01-01T00:00:00Z"},
      {"type": "not", "child": {"type": "path_contains", "value": "trash"}}
    ]
  }
}
```

#### ⑦ OR(size > 1GB, mtime in last 7 days)

```json
{
  "expression": {
    "or": [
      {"type": "size_gt", "value": 1073741824},
      {"type": "mtime_after", "value": "2026-06-01T00:00:00Z"}
    ]
  }
}
```

#### ⑧ 跨目录 glob：`Movies/**/*.mkv`

```json
{
  "expression": {"type": "name_glob", "value": "**/*.mkv"}
}
```

#### ⑨ 复合：找带字幕的 mp4

```json
{
  "expression": {
    "and": [
      {"type": "ext_eq", "value": "mp4"},
      {"type": "path_contains", "value": "subs"}
    ]
  }
}
```

#### ⑩ NOT chain：找 2025 年不是空的小文件

```json
{
  "expression": {
    "and": [
      {"type": "mtime_after",  "value": "2025-01-01T00:00:00Z"},
      {"type": "mtime_before", "value": "2026-01-01T00:00:00Z"},
      {"type": "size_lt",       "value": 1048576},
      {"type": "not", "child": {"type": "size_eq", "value": 0}}
    ]
  }
}
```

### 性能约束 (Limits)

| 约束 | 值 | 行为 |
|------|----|----|
| `MaxFilesScanned` | **50000** 文件 | 超过则设 `scanned_limited: true` 并立即返回 partial result |
| `MaxContentRegexSize` | **10 MB** | 超过此大小的文件跳过 `content_regex` 扫描 |
| `max_results` | 默认 **200**，最大 **1000** | 超过则 `truncated: true`，`total` 仍反映真实命中数 |

### 返回结果

```json
{
  "total": 42,
  "truncated": false,
  "scanned_limited": false,
  "matches": [
    {
      "path": "Movies/studio_video_1762.mp4",
      "size": 554000000,
      "mtime": "2026-01-15T10:30:00Z",
      "ext":  "mp4"
    }
  ]
}
```

---

## 4. `command_run` 安全模型 (Security Model)

> 实现：`internal/tools/command_run.go` · 在 `internal/config/config.go::Agent.ToolWhitelist` 集中配置。

### 白名单 (Whitelist, 默认)

| 命令 | 用途 |
|------|------|
| `ffprobe` | 视频/音频元数据探测 |
| `ffmpeg` | 媒体转码 / 元数据写入 |
| `du` | 磁盘占用 |
| `wc` | 字节/行/字数统计 |
| `find` | 文件查找（仅在沙箱内有效） |
| `stat` | 文件状态 |
| `mediainfo` | 媒体信息 |
| `file` | 类型识别 |

### 黑名单 (Blacklist, 永远拒绝)

```
rm / mv / cp / chmod / chown / dd / mkfs / shutdown / reboot / halt / poweroff
```

> **注意**：白名单/黑名单**叠加生效**——黑名单命令即使在白名单内也直接拒绝。

### 执行约束

| 约束 | 值 |
|------|----|
| 默认 timeout | **5 秒**（可通过 `timeout_sec` 覆盖，上限 30） |
| 输出截断 | **8 KB**（stdout 和 stderr 分别截断） |
| 退出码非 0 | `isError: true` + stderr 写入 result |
| 路径越权 | 任何参数含 `..` / `/etc/` / `/usr/` / `/var/` / `/boot/` / `/sys/` / `/proc/` **直接拒绝** |
| 沙箱范围 | `mount_id` 通过 `ResolveMount` 映射到真实目录，**仅**在映射下允许 |

### 示例：安全 vs 拒绝

#### ✅ 安全

```json
{
  "command": "ffprobe",
  "args": ["-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", "Movies/a.mp4"],
  "timeout_sec": 5
}
```

#### ❌ 黑名单拒绝

```json
{
  "command": "rm",
  "args": ["-rf", "Movies/a.mp4"]
}
```
→ 返回 `{"error": "command denied (blacklisted): rm"}`，`isError: true`

#### ❌ 路径越权拒绝

```json
{
  "command": "stat",
  "args": ["/etc/passwd"]
}
```
→ 返回 `{"error": "arg rejected: sensitive path prefix /etc/: /etc/passwd"}`

#### ❌ 路径遍历拒绝

```json
{
  "command": "cat",
  "args": ["../../../etc/passwd"]
}
```
→ 即便 `cat` 不在白名单也直接因黑名单失败；如换为 `find` 则因 `..` 拒绝

---

## 5. 写入工具 (Write Tools)

> **所有写入工具** `requires_confirm: true`：前端 AgentChat 收到 confirmation 弹窗 → 用户确认后 → `confirmTool(id, "accept")` → 真正执行。

### 通用流程

```
tool_call 事件 → 弹 confirm 弹窗（显示 args）→ 用户点「确认」
  → confirmTool(id, "accept") / "accept_for_session"
  → 后端 MockEngine 恢复推流 → execute_real 走真实 handler
  → tool_result 事件带 success / backup_path
  → 失败时 handler 内部已尝试回滚
```

### `edit_metadata`

```json
{
  "mount_id": "local",
  "rel_path": "Movies/a.mp4",
  "metadata": {
    "title":   "My Title",
    "artist":  "Encoder",
    "comment": "encoded at 2026-06-08"
  }
}
```

内部流程（`internal/tools/edit_metadata.go::runFfmpegMetadata`）：
1. 备份原文件到 `<name>.bak`
2. 调 `ffmpeg -y -i in -c copy -metadata key=value out`
3. 成功 → `os.Rename` 替换原文件 → 返回 `{"success": true, "backup_path": "..."}`
4. 失败 → 恢复备份

### `batch_rename`（**dry-run 必须先走**）

```json
// 第一次：dry_run=true → 返回预览
{
  "mount_id": "local",
  "rel_path": "Series/Friends",
  "pattern":     "Friends S01E(\\d+).*",
  "replacement": "Friends_S01E$1.mkv",
  "dry_run":     true
}
```

→ 返回：
```json
{
  "total": 24,
  "previews": [
    {"old": ".../Friends S01E01 720p.mkv", "new": ".../Friends_S01E01.mkv"},
    ...
  ]
}
```

用户在前端确认 → 第二次调用 `dry_run: false`：
- 每个文件先备份到 `<name>.bak`
- `os.Rename` 改名
- 任一失败 → 全量回滚（把 backups 还原到原路径）
- 返回 `{"applied": 24, "backups": ["..."], "rolled_back": false}`

### `delete_file`

```json
{
  "mount_id": "local",
  "rel_path": "tmp/old.mp4",
  "mode": "trash"   // or "hard"
}
```

- `mode=trash`（默认）：移到 `<mount_root>/.trash/<unix_nano>_<name>`
- `mode=hard`：直接 `os.Remove`（仍需前端 confirm；mount 配置 `trash_enabled: false` 时才走此模式）

---

## 6. 剧本 v2 引擎 (Scenario v2 Engine)

> 实现：`internal/server/agent_mock_v2.go` · 8 个剧本在 `internal/server/agent_mock_v2_scenarios.go`。

### 关键概念

| 字段 | 含义 |
|------|------|
| `MockScenario.Rounds` | 剧本总轮数；`> 0` 时引擎走 v2 路径 |
| `MockScenario.RoundContext` | 跨轮变量（`set_context` / `use_context`） |
| `MockScenario.Branches` | 分支数组 |
| `MockStep.RoundIdx` | step 归属 round（0-based） |
| `MockStep.PauseForUser` | 推完本 step 后暂停等用户输入 |
| `MockStep.BranchChoice` | 推 mock_branch_choice 并等待 PickBranch |
| `MockStep.SetContext` | 把 key/value 写入 roundCtx |
| `MockStep.UseContext` | 在事件 data 中用 `{{ key }}` 插值 |

### 分支匹配优先级

`MockEngineV2.PickBranch` 按以下顺序尝试匹配：

1. **精确匹配**：`branch.ID == userText`（如 userText = `"encrypt"`）
2. **关键词匹配**：任一 `TriggerKeywords` 出现在 userText（大小写不敏感）
3. **正则匹配**：`TriggerRegex` 编译后 `MatchString(userText)`
4. **都不匹配** → 重新推 `mock_branch_choice` 并等待用户再选

### 示例演练 1：`edit_metadata_wizard`（4 轮）

```
Round 0（选文件）：
  step 1: text_delta "请选择要编辑的文件："
  step 2: text_delta "1) Movies/a.mp4\n2) Movies/b.mp4"
  step 3: mock_presets 列出 a/b 选项
  step 4: PauseForUser=true
  step 5: SetContext.selected_file = "Movies/a.mp4"

[用户点 chip "Movies/a.mp4" → 触发 sendMockRoundResponse]
  → Resume → 推进到 Round 1

Round 1（选字段）：
  step 1: text_delta "你想编辑哪个字段？"
  step 2: mock_presets 列出 title/year/genre
  step 3: PauseForUser
  step 4: SetContext.selected_field = "title"

[用户选 "title"]

Round 2（输入新值）：
  step 1: text_delta "请输入新值："
  step 2: PauseForUser
  step 3: SetContext.new_value = "My New Title"

[用户键入 "My New Title"]

Round 3（确认 + 执行）：
  step 1: text_delta "将编辑 Movies/a.mp4 的 title 字段，值为「My New Title」。"
  step 2: tool_call edit_metadata {requires_confirm=true}
  [用户点确认 → 真实 ffmpeg 写入]
  step 3: tool_result (真实结果)
  step 4: text_delta "✓ 已更新 title 字段。"
  step 5: stream_end
```

### 示例演练 2：`branch_encrypt_or_decrypt`（3 选 1 分支）

```
Round 0:
  step 1: stream_start {scenario: "branch_encrypt_or_decrypt"}
  step 2: text_delta "请选择操作类型："
  step 3: mock_branch_choice
         {
           "branches": [
             {"id": "encrypt", "label": "🔒 加密", "icon": "🔒"},
             {"id": "decrypt", "label": "🔓 解密", "icon": "🔓"},
             {"id": "cancel",  "label": "✗ 取消",  "icon": "✗"}
           ]
         }
  step 4: PauseForUser
```

[用户点 chip / 键入 `encrypt`]

→ `MockEngineV2.PickBranch("encrypt")`：
1. 匹配 branch `encrypt`（精确）
2. 写入 `roundCtx.branch_id = "encrypt"`
3. 推 `mock_branch_picked`
4. 如果有 `OnMatch` 子剧本 → 跳到子剧本 Run；否则 `stream_end {finishReason: branch_terminated}`

### 暂停 / 取消 / 超时

| 行为 | 触发 | 后端响应 |
|------|------|---------|
| 暂停 | step `pause_for_user=true` | 推 `mock_round_state{phase: awaiting_user_input}` 后停止推事件，**SSE 保持** |
| 恢复 | 用户发 user_text（chip / 输入） | `mode: mock_resume` → `Resume()` → 继续推下一 round |
| 取消 | 用户关 modal | 推 `stream_end{finishReason: cancelled}` |
| 超时 | `mock_round_timeout_sec` 秒内无 user_text | 推 `stream_end{finishReason: timeout}`（默认 60s，可配 10-600） |

---

## 7. 8 个 v2 剧本速查表 (Scenario Cheatsheet)

> 8 个 v2 剧本全部注册在 `internal/server/agent_mock_v2_scenarios.go::mockScenariosV2`。
> 在 mock_mode=builtin 时由 `MockEngine.Match` 关键词匹配自动触发。

| 剧本 ID | 触发关键词 | 演示能力 | 预期交互流程 |
|---------|-----------|---------|-------------|
| `search_recursive_mp4` | `search_recursive_mp4` | 递归 + glob `*.mp4` + size > 100MB | 单轮：text → tool_call(search_files) → tool_result → text → stream_end |
| `search_logical_query` | `search_logical_query` | 复合 AND（size_gt + mtime_after + ext_eq） | 单轮：text → tool_call(AND AST) → tool_result → text → stream_end |
| `search_content_regex` | `search_content_regex` | content_regex `ERROR.*timeout` | 单轮：text → tool_call(content + ext) → tool_result → text → stream_end |
| `edit_metadata_wizard` | `edit_metadata_wizard` | **4 轮**多轮（选文件→选字段→输入新值→确认） | Round 0-3 见 §6 示例演练 1 |
| `batch_rename_with_preview` | `batch_rename_with_preview` | dry_run 预览 → 用户确认 → 真实执行 | Round 0: dry_run 预览 + confirm presets；Round 1: 真实执行 |
| `branch_encrypt_or_decrypt` | `branch_encrypt_or_decrypt` | **3 选 1**分支（encrypt / decrypt / cancel） | Round 0: text + mock_branch_choice；用户选 → PickBranch |
| `branch_video_or_audio` | `branch_video_or_audio` | **3 分支**（video / audio / other） | Round 0: text + mock_branch_choice（按文件类型走不同 ffprobe 路径） |
| `command_run_ffprobe` | `command_run_ffprobe` | **受限 shell** + 真实 ffprobe 输出 | 单轮：text → tool_call(command_run) → tool_result(stdout JSON) → text |

> **8 个剧本演示了什么**：1 递归 + 1 复合布尔 + 1 内容正则 + 1 多轮向导 + 1 dry-run + 1 三选一 + 1 多分支 + 1 受限 shell。覆盖了 v2 的全部新能力。

---

## 8. `MockBranchChoiceBar` UI

> 文件：`app/encv-mobile/src/components/agent/MockBranchChoiceBar.vue` · 集成在 `AgentChat.vue` 的 footerInputRow 上方。

### 视觉（无实际截图，文字描述）

```
┌──────────────────────────────────────────────┐
│ 🧪 edit_metadata_wizard       Round 2/4     │  ← header：scenario badge + 轮次
│ 你想编辑哪个字段？                            │  ← prompt（来自 mock_branch_choice.prompt）
│                                              │
│ ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│ │ title    │  │ year     │  │ genre    │   │  ← chip 列表：icon + label + description
│ │ 标题     │  │ 年份     │  │ 类型     │   │     横向滚动
│ └──────────┘  └──────────┘  └──────────┘   │
│                                              │
│ 点击 chip 继续或键入文本                     │  ← hint（i18n key: agent.roundPausedHint）
└──────────────────────────────────────────────┘
```

### 显示时机

`MockBranchChoiceBar` 仅在 `mockScenarioPaused === true` 时渲染：

```typescript
const mockScenarioPaused = computed(() => {
  const phase = mockRoundState.value?.phase
  return phase === 'awaiting_user_input' || phase === 'awaiting_branch_choice'
})
```

| 触发条件 | phase | 显示内容 |
|---------|-------|---------|
| `mock_branch_choice` 事件到达 | `awaiting_branch_choice` | 完整 chip 列表 |
| `mock_round_state{phase: awaiting_user_input}` 到达 | `awaiting_user_input` | prompt + hint（无 chip，让用户键入文本） |
| `stream_end` 到达 | `null` | **不显示**（清空 `mockRoundState`） |

### 暗黑模式

- 容器：`rgba(var(--ion-color-primary-rgb), 0.10)` + `backdrop-filter: blur(12px)` + 圆角 12px
- chip 暗色态：`rgba(var(--ion-color-primary-rgb), 0.18)` 背景
- 完全跟随当前主题色（Settings → Theme Color 可即时切换）

### 用户交互

| 用户行为 | 前端动作 | 后端事件流 |
|---------|---------|-----------|
| 点击 chip | 触发 `pick(branch)` → `pickMockBranch(branch.id)` | `send(branchId, {mode: "mock_resume", scenario: ...})` |
| 在输入框键入文本 | 触发 `type(text)` → `sendMockRoundResponse(text)` | `send(userText, {mode: "mock_resume", scenario: ...})` |

`scenario` 字段**必须**带上——后端 `MockEngineV2` 据此找到正确的状态机。

---

## 9. 配置 (Configuration)

> 文件：`internal/config/config.go::Agent` · `internal/config/schema.json`。
> 4 个新字段，渲染在 `app/encv-mobile/src/views/Settings.vue` 的「Agent 高级设置」分组。

### `agent_settings.tool_whitelist`

- **类型**：`string[]`
- **默认**：`["ffprobe", "ffmpeg", "du", "wc", "find", "stat", "mediainfo", "file"]`
- **说明**：`command_run` 允许的命令名（黑名单永远生效：`rm` / `mv` / `cp` / `chmod` / `chown` / `dd` / `mkfs` / `shutdown` / `reboot` 等）。
- **UI**：多行 tag input，每行一个命令。
- **示例**：
  ```json
  "tool_whitelist": ["ffprobe", "ffmpeg", "du", "wc", "find", "stat", "mediainfo", "file", "jq"]
  ```

### `agent_settings.sandbox_paths`

- **类型**：`{ [mount_id: string]: string }`
- **默认**：`{}`
- **说明**：mount_id → 真实主机目录的映射。`search_files` / `get_metadata` / `command_run` / `read_file_v2` 等需要访问物理文件系统的工具都走此映射。
- **UI**：key-value 编辑器（左列 mount_id，右列绝对路径）。
- **示例**：
  ```json
  "sandbox_paths": {
    "local": "/home/user/Videos",
    "sandbox": "/tmp/agent-v2-sandbox"
  }
  ```

### `agent_settings.mock_round_timeout_sec`

- **类型**：`number`
- **范围**：`10` ~ `600`
- **默认**：`60`
- **说明**：v2 多轮剧本在 `pause_for_user=true` 步骤暂停时，等待用户输入的最大秒数。超时后后端推 `stream_end{finishReason: "timeout"}`。
- **UI**：number input（min=10, max=600）。

### `agent_settings.mock_round_pause_enabled`

- **类型**：`boolean`
- **默认**：`true`
- **说明**：是否允许剧本 mid-scenario 暂停等用户输入。`false` → 剧本忽略 `pause_for_user` 标记，自动跑完（适合自动化测试 / CI 录制）。
- **UI**：开关 (`<ion-toggle>`)。

### 配置示例

```json
{
  "agent_settings": {
    "mock_mode": "builtin",
    "mock_speed": 1.0,
    "tool_whitelist": ["ffprobe", "ffmpeg", "du", "wc", "find", "stat", "mediainfo", "file"],
    "sandbox_paths": {
      "local": "/data/media"
    },
    "mock_round_timeout_sec": 60,
    "mock_round_pause_enabled": true
  }
}
```

---

## 10. English Summary

### Overview (English)

v2 **adds** 7 new tools and 8 new mock scenarios on top of v1's 12 linear scenarios. The most important architectural change is the **`ToolRegistry`** — a unified dispatch table in `internal/tools/registry.go` that the Mock engine and the real-LLM path both consume. The 7 v2 tools (`search_files`, `get_metadata`, `read_file_v2`, `edit_metadata`, `batch_rename`, `delete_file`, `command_run`) are registered at server startup via `tools.RegisterAll()`. The 3 v1 tools (`list_mounts`, `list_files`, `read_file`) keep their legacy `executeFSTool` path to preserve backward compatibility — registering them with the same names in `GlobalRegistry` would conflict and is explicitly avoided.

The flagship tool is `search_files`, which combines recursive directory walk, glob, regex, and a 14-node boolean AST (11 leaves + 3 compound: `and` / `or` / `not` with short-circuit evaluation). Hard limits (50,000 files / 10 MB content regex) protect against pathological mounts. `command_run` is a restricted shell with a whitelist (`ffprobe` / `ffmpeg` / `du` / `wc` / `find` / `stat` / `mediainfo` / `file`), a hard blacklist (`rm` / `mv` / `cp` / `chmod` / `chown` / `dd` / `mkfs` / etc.), path-traversal rejection, 5-second timeout, and 8-KB output truncation. All write tools (`edit_metadata` / `batch_rename` / `delete_file`) declare `requires_confirm: true` and back up + roll back on failure.

The mock engine v2 (`internal/server/agent_mock_v2.go`) adds a state machine with `Branches`, `Rounds`, and `RoundContext`. Branch matching tries exact-ID first, then keyword, then regex, then re-prompts. Pause/resume is wired through a `mode: "mock_resume"` POST whose body carries the `scenario` ID. Two new SSE event types (`mock_branch_choice`, `mock_round_state`) drive the new `MockBranchChoiceBar` component on the frontend.

### Tool Catalog (English)

| Name | readonly | requires_confirm | Kind | Description |
|------|:---:|:---:|------|-------------|
| `list_mounts` ⭐ v1 | ✅ | ❌ | `fileRead` | List all mounts |
| `list_files` ⭐ v1 | ✅ | ❌ | `fileRead` | List single directory |
| `read_file` ⭐ v1 | ✅ | ❌ | `fileRead` | Read entire file (legacy path, no pagination) |
| `search_files` | ✅ | ❌ | `fileRead` | Recursive + glob + regex + AND/OR/NOT AST |
| `get_metadata` | ✅ | ❌ | `metadata` | File metadata + ffprobe video/audio + optional SHA-256 |
| `read_file_v2` | ✅ | ❌ | `fileRead` | Paginated (`start_line` / `end_line` / `max_bytes`) + binary detection |
| `edit_metadata` | ❌ | ✅ | `fileChange` | Write title/artist/comment, with backup + rollback |
| `batch_rename` | ❌ | ✅ | `fileChange` | regex + replacement, with `dry_run` + rollback on failure |
| `delete_file` | ❌ | ✅ | `fileChange` | Default trash; `mode=hard` requires double confirmation |
| `command_run` | ✅ | ❌ | `command` | Restricted shell: whitelist + blacklist + path sandbox + 5s timeout + 8KB truncation |

### 8 New Mock Scenarios (English)

| Scenario | Demonstrates |
|----------|-------------|
| `search_recursive_mp4` | Recursive + glob + size > 100MB |
| `search_logical_query` | Compound AND (size + mtime + ext) |
| `search_content_regex` | `content_regex` against log files |
| `edit_metadata_wizard` | **4 rounds**: select file → select field → enter value → confirm |
| `batch_rename_with_preview` | `dry_run` preview → user confirmation → real execution |
| `branch_encrypt_or_decrypt` | **3-way branch** (encrypt / decrypt / cancel) |
| `branch_video_or_audio` | **3 branches** (video / audio / other), each takes a different ffprobe path |
| `command_run_ffprobe` | Restricted shell + real `ffprobe` stdout |

### Configuration (English)

Four new fields in `agent_settings`:
- `tool_whitelist: string[]` — additional allowed commands for `command_run` (default: `ffprobe`, `ffmpeg`, `du`, `wc`, `find`, `stat`, `mediainfo`, `file`)
- `sandbox_paths: { [mount_id]: absolute_path }` — mount ID → host directory mapping
- `mock_round_timeout_sec: number` — pause-for-user timeout (10–600, default 60)
- `mock_round_pause_enabled: boolean` — whether scripts may pause mid-stream (default `true`; set `false` for auto-pilot / CI)

### Frontend Integration (English)

The Vue composable `useAgent` (in `app/encv-mobile/src/composables/useAgent.ts`) parses two new SSE event types and exposes four reactive refs:
- `mockBranchChoices: MockBranch[]` — chip list
- `mockBranchPrompt: string` — current step prompt
- `mockRoundState: MockRoundState | null` — round + phase + context
- `mockScenarioPaused: ComputedRef<boolean>` — derived from `phase` ∈ {`awaiting_user_input`, `awaiting_branch_choice`}

Two helper functions dispatch user actions back to the engine with `mode: "mock_resume"`:
- `pickMockBranch(branchId)` — for chip clicks
- `sendMockRoundResponse(userText)` — for free-text replies

The `MockBranchChoiceBar.vue` component renders the chip list above the input row with primary-tint background, backdrop blur, and dark-mode-aware colors. The `stream_end` event clears all v2 state so the bar disappears cleanly when the session ends.

---

## See Also

- **Spec**: `/workspace/.trae/specs/agent-tools-scenarios-v2/spec.md` (full requirements)
- **Tasks**: `/workspace/.trae/specs/agent-tools-scenarios-v2/tasks.md` (17 tasks, 89 sub-items)
- **Changelog**: `/workspace/.trae/documents/CHANGELOG.md` (v2 release notes)
- **Tool registry**: `/workspace/internal/tools/registry.go`
- **Mock engine v2**: `/workspace/internal/server/agent_mock_v2.go`
- **Frontend bar**: `/workspace/app/encv-mobile/src/components/agent/MockBranchChoiceBar.vue`
