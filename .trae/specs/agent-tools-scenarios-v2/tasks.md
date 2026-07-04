# Tasks

有序、可验证的工作项；每项都对应具体文件 / 函数 / 测试。

## Task Dependencies

- T1 → T2 → T3（工具注册中心是基础）
- T1 → T4（search_files 是核心工具）
- T1 → T5 → T6 → T7（其他工具都依赖 registry）
- T1..T7 → T8（剧本引擎需要 registry 派发）
- T8 → T9（v2 剧本依赖引擎）
- T9 → T10（i18n 同步需要先确定事件 payload 形状）
- T9 → T11（前端解析需要事件形状）
- T11 → T12（前端组件依赖 useAgent 暴露的 ref）
- T12 → T13（集成到 AgentChat）
- T1..T13 → T14（端到端测试）
- T14 → T15（性能压测）

---

## T1. 工具注册中心 ToolRegistry

**目标**: 启动时统一注册所有工具，MockEngine 与真实 LLM 路径共用一份 handler 代码

- [x] **T1.1** 新建 `internal/tools/registry.go`
  - `ToolHandler` 类型（ctx + args + deps → result + error）
  - `ToolDef` 结构（name / description / args_schema / handler / requires_confirm / readonly / kind）
  - `ToolRegistry` 类型（Register / Get / List）
  - `GlobalRegistry` 全局变量
  - `ToolDeps` 依赖注入（mountManager / config / logger）
- [x] **T1.2** 迁移 v1 三个工具到新 registry
  - `list_mounts` / `list_files` / `read_file` 重新实现为 ToolDef
  - handler 内部继续调原有逻辑
- [x] **T1.3** 修改 `internal/server/agent_api.go` 的 `executeAgentTool`
  - 改为 `GlobalRegistry.Get(name).Handler(ctx, args, deps)` 派发
  - 删掉硬编码 if-else
- [x] **T1.4** 单元测试（5+）
  - `TestRegistry_Register_AndGet`
  - `TestRegistry_DuplicateName_Error`
  - `TestRegistry_List_ReturnsAllSorted`
  - `TestRegistry_UnknownName_ReturnsFalse`
  - `TestRegistry_Dispatch_viaExecuteReal`

✅ **验收**: `go test ./internal/tools/... -run TestRegistry -v` 全过

---

## T2. search_files 工具（核心）

**目标**: 递归 + glob + regex + 复合布尔查询 AST

- [x] **T2.1** 新建 `internal/tools/search_files.go`
  - `SearchFilesArgs` 参数结构（mount_id / rel_path / recursive / max_results / expression）
  - `SearchFilesResult` 结果结构（total / truncated / matches / scanned_limited）
  - `SearchFiles` 主函数（递归遍历 + AST 求值）
- [x] **T2.2** 实现 AST 节点类型（11 种 leaf + 3 种复合）
  - `name_glob` / `name_regex` / `content_regex`
  - `size_gt` / `size_lt` / `size_eq` / `mtime_after` / `mtime_before`
  - `ext_eq` / `path_contains` / `path_not_contains`
  - `and` / `or` / `not`
- [x] **T2.3** 实现 `evalExpr(ast, file)` 递归求值
  - 短路求值
  - 未知节点类型 → 错误
- [x] **T2.4** glob 编译
  - `*.mp4` → 正则（`^[^/]*\.mp4$`）
  - `**` → 跨目录
  - `?` → 单字符
- [x] **T2.5** content_regex 性能约束
  - 文件 > 10MB → 跳过
  - 累计扫描文件数 > 50000 → 截断
- [x] **T2.6** 单元测试（15+）
  - 11 种 leaf 各一个
  - AND/OR/NOT 短路
  - 嵌套
  - max_results 截断
  - 50000 上限
  - 未知类型容错

✅ **验收**: `go test ./internal/tools/... -run TestSearchFiles -v` 全过

---

## T3. get_metadata 工具

**目标**: 单文件元信息 + ffprobe 媒体探测

- [x] **T3.1** 新建 `internal/tools/get_metadata.go`
  - `GetMetadataArgs`（mount_id / rel_path / include_hash bool）
  - `GetMetadataResult`（基础字段 + 视频/音频字段）
- [x] **T3.2** 基础字段（所有文件）
  - path / size / mtime / mode / mime / extension / is_hidden / is_symlink / is_dir
- [x] **T3.3** 视频字段（ffprobe）
  - duration / width / height / codec / bitrate / frame_rate / has_audio
- [x] **T3.4** 音频字段（ffprobe）
  - duration / bitrate / sample_rate / channels / codec / has_cover_art
- [x] **T3.5** ffprobe 容错
  - 缺失 → 跳过视频/音频字段
  - 超时（5s）→ 跳过 + slog.Warn
  - 整体不失败
- [x] **T3.6** 单元测试（5+）
  - 视频文件成功
  - 音频文件成功
  - 无 ffprobe 容错
  - ffprobe 超时
  - sha256 按需

✅ **验收**: `go test ./internal/tools/... -run TestGetMetadata -v` 全过

---

## T4. read_file v2 增强

**目标**: 分页 / 范围 / 二进制检测

- [x] **T4.1** 新建 `internal/tools/read_file_v2.go`（独立于 v1 read_file，不破坏兼容）
  - `ReadFileV2Args`（mount_id / rel_path / start_line / end_line / max_bytes）
  - `ReadFileV2Result`（path / total_lines / total_bytes / lines / binary / truncated / encoding）
- [x] **T4.2** 实现分页逻辑
  - start_line / end_line 范围读取
  - start_line 越界 → 错误
  - end_line 越界 → clamp
- [x] **T4.3** 二进制检测
  - 前 1KB 扫描非 UTF-8 字符 → binary: true
  - 返回 content_base64 + content_truncated
  - 加 warning
- [x] **T4.4** max_bytes 截断
  - 默认 1MB
  - 超过 → 返回截断内容 + truncated: true
- [x] **T4.5** 单元测试（5+）
  - 范围读取
  - max_bytes 截断
  - 二进制检测
  - start_line 越界
  - end_line 自动 clamp

✅ **验收**: `go test ./internal/tools/... -run TestReadFileV2 -v` 全过

---

## T5. 写入工具（edit_metadata / batch_rename / delete_file）

**目标**: 全部 requires_confirm，写操作有回滚

- [x] **T5.1** 新建 `internal/tools/edit_metadata.go`
  - `EditMetadataArgs`（mount_id / rel_path / metadata: {title/artist/comment}）
  - 写 ID3 / MP4 atoms（封装 ffmpeg 调用）
  - 备份原文件 → 写新文件 → 失败回滚
  - requires_confirm: true
- [x] **T5.2** 新建 `internal/tools/batch_rename.go`
  - `BatchRenameArgs`（mount_id / pattern / replacement / dry_run）
  - dry_run: true → 仅返回变更预览
  - 真实执行：每个文件先备份再改 → 任一失败回滚
  - requires_confirm: true
- [x] **T5.3** 新建 `internal/tools/delete_file.go`
  - `DeleteFileArgs`（mount_id / rel_path / mode: "trash"|"hard"）
  - 默认 trash（移至 mount.trash_path）
  - hard 模式 → 二次确认（前端弹窗）
  - requires_confirm: true
- [x] **T5.4** 单元测试（6+）
  - edit_metadata 写入 ID3
  - batch_rename dry_run 不真实改
  - batch_rename 回滚
  - delete_file 走 trash
  - delete_file hard 二次确认
  - 所有写入工具 requires_confirm=true 标记正确

✅ **验收**: `go test ./internal/tools/... -run TestEditMetadata|TestBatchRename|TestDeleteFile -v` 全过

---

## T6. command_run 受限 shell

**目标**: 工具白名单 + 沙箱路径 + 超时 + 输出截断

- [x] **T6.1** 新建 `internal/tools/command_run.go`
  - `CommandRunArgs`（mount_id / command / args[] / timeout_sec）
  - `CommandRunResult`（stdout / stderr / exit_code / output_truncated）
- [x] **T6.2** 白名单校验
  - 默认白名单：ffprobe / ffmpeg / du / wc / find / stat / mediainfo / file
  - 黑名单：rm / mv / cp / chmod / chown / dd / mkfs / shutdown / reboot
  - 用户可在 `agent_settings.tool_whitelist` 追加
- [x] **T6.3** 路径沙箱
  - 任何参数含 `..` → 拒绝
  - 任何参数含 `/etc` / `/usr` / `/var` → 拒绝
  - 仅允许 mount_id → 真实路径映射下的文件
- [x] **T6.4** 执行约束
  - timeout 默认 5s
  - 输出 > 8KB → 截断 + truncated: true
  - exit code != 0 → isError: true + stderr
- [x] **T6.5** 单元测试（6+）
  - 允许命令成功
  - 黑名单命令被拒
  - 路径越权被拒
  - 超时被中断
  - 输出截断
  - 沙箱外路径被拒

✅ **验收**: `go test ./internal/tools/... -run TestCommandRun -v` 全过

---

## T7. 工具权限模型

**目标**: 统一处理 requires_confirm / readonly / kind 标签

- [x] **T7.1** 修改 `internal/server/agent_api.go` 的 mock 流
  - 遇到 `requires_confirm: true` 工具 → 推 `tool_status {status: pending_confirm}`
  - 暂停剧本（不继续推）
  - 前端 confirmTool(id, approve/reject) → 恢复 / 取消
- [x] **T7.2** 验证所有工具标签正确
  - 8 个 v1 + v2 工具全部 `requires_confirm` 字段正确
  - readonly 字段正确
  - kind 字段正确
- [x] **T7.3** 单元测试（3+）
  - requires_confirm 阻塞
  - 拒绝后跳过
  - readonly 不允许外部强行设置 false

✅ **验收**: `go test ./internal/tools/... -run TestRequiresConfirm -v` 全过

---

## T8. 剧本 v2 引擎（分支 + 多轮）

**目标**: 状态机支持 branch / round / pause / resume

- [x] **T8.1** 新建 `internal/server/agent_mock_v2.go`
  - `Branch` 结构（ID / Label / Description / Icon / TriggerKeywords / TriggerRegex / OnMatch / InitialStepID）
  - `MockScenario` 加 `Branches []Branch` / `Rounds int` / `RoundContext map[string]any`
  - `MockStep` 加 `BranchID string` / `RoundIdx int` / `PauseForUser bool` / `SetContext` / `UseContext`
- [x] **T8.2** 实现 `MockEngineV2`
  - `Run(ctx, scenario, eventWriter)` 启动
  - `Resume(ctx, userText, roundCtx)` 用户回复恢复
  - `PickBranch(ctx, branchID)` 分支选择
  - `ApproveTool(ctx, toolCallID)` / `RejectTool(ctx, toolCallID)` 工具权限
- [x] **T8.3** 状态机事件
  - 新增事件类型 `mock_branch_choice` / `mock_round_state`
  - 与 v1 事件类型共存
  - 序列化 JSON 形状
- [x] **T8.4** 状态机实现
  - round 推进
  - 分支匹配（精确 / 关键词 / 正则 / 不匹配重提示）
  - 暂停 vs 继续
  - 取消与超时
- [x] **T8.5** 与 v1 兼容
  - 现有 12 个剧本走 v1 路径（线性流）
  - v2 剧本走 v2 路径（branch + round）
  - 切换依据：`MockScenario.Rounds > 0 || len(Branches) > 0`
- [x] **T8.6** 单元测试（15+）
  - 分支选择 4 用例（暂停 / 关键词 / 正则 / 重提示）
  - 多轮推进 4 用例
  - round context 读写
  - 超时取消
  - pause/resume
  - 工具权限阻塞
  - 工具权限拒绝
  - 8 个 v2 剧本可启动
  - 4 轮剧本可完成
  - 批改名 dry_run→执行
  - 分支跳转子剧本
  - ffprobe 真实输出
  - context 模板渲染
  - 事件 data shape
  - stream_end 清空状态
  - v1 兼容

✅ **验收**: `go test ./internal/server/... -run TestMockEngineV2 -v` 全过

---

## T9. 8 个 v2 剧本

**目标**: 覆盖搜索/正则/逻辑/写操作/分支/多轮全部新能力

- [x] **T9.1** 新建 `internal/server/agent_mock_v2_scenarios.go`
  - `search_recursive_mp4` — 递归 + glob *.mp4 + size > 100MB
  - `search_logical_query` — AND (size_gt + mtime_after + ext_eq)
  - `search_content_regex` — content_regex "ERROR.*timeout"
  - `edit_metadata_wizard` — 4 轮（选文件→选字段→输入→确认）
  - `batch_rename_with_preview` — dry_run → 确认 → 执行
  - `branch_encrypt_or_decrypt` — 3 选 1
  - `branch_video_or_audio` — 多分支 + 跨分支多轮
  - `command_run_ffprobe` — 受限 shell
- [x] **T9.2** 每个剧本 1 个集成测试（8+）
  - 验证 Run() 启动 + 关键 step 出现
  - 验证分支 / 多轮的关键转换
- [x] **T9.3** 真实 mount 集成测试
  - 准备 sandbox 目录（10+ 文件：mp4/srt/log/bin）
  - search_recursive_mp4 真实命中
  - edit_metadata_wizard 真实写入

✅ **验收**: 8 个剧本均能 Run + 关键 step 验证通过

---

## T10. 配置 schema 增量

**目标**: tool_whitelist / sandbox_paths / mock_round_timeout 字段

- [x] **T10.1** 修改 `internal/config/config.go`
  - `AgentSettings` 加 `ToolWhitelist []string` / `SandboxPaths map[string]string` / `MockRoundTimeoutSec int` / `MockRoundPauseEnabled bool`
- [x] **T10.2** 修改 `internal/config/schema.json`
  - 加 4 个新字段 + 默认值 + description
- [x] **T10.3** 修改 `app/encv-mobile/src/views/Settings.vue` 渲染
  - tool_whitelist → 多行 tag input
  - sandbox_paths → key-value 编辑器
  - mock_round_timeout_sec → number input
  - mock_round_pause_enabled → 开关
- [x] **T10.4** 单元测试（3+）
  - 字段反序列化
  - 默认值正确
  - 校验范围（10-600）

✅ **验收**: 启动时加载 config.json 4 个字段均有值，Settings.vue 渲染正常

---

## T11. 前端 useAgent 解析新事件

**目标**: 暴露 mockBranchChoices / mockRoundState / pickMockBranch / sendMockRoundResponse

- [x] **T11.1** 修改 `app/encv-mobile/src/composables/useAgent.ts`
  - `MockBranch` 类型（id / label / icon / description）
  - `mockBranchChoices = ref<MockBranch[]>([])` / `mockBranchPrompt = ref('')` / `mockRoundState = ref(...)` / `mockScenarioPaused = computed(...)`
  - `pickMockBranch(branchId)` → send(branchId, { mode: 'mock_resume', scenario: ... })
  - `sendMockRoundResponse(userText)` → send(userText, { mode: 'mock_resume' })
  - `case 'mock_branch_choice'` / `case 'mock_round_state'` 解析
  - `case 'stream_end'` 清空 mockBranchChoices + mockRoundState = null
- [x] **T11.2** 单元测试（5+）
  - 解析 mock_branch_choice
  - 解析 mock_round_state
  - stream_end 清空
  - pickMockBranch 调用 send
  - sendMockRoundResponse 调用 send

✅ **验收**: `npx vitest run src/composables/__tests__/useAgent.test.ts -v` 全过

---

## T12. MockBranchChoiceBar 组件

**目标**: 暂停状态下渲染分支 chip + 轮次进度

- [x] **T12.1** 新建 `app/encv-mobile/src/components/agent/MockBranchChoiceBar.vue`
  - props: branches / prompt / roundState
  - emits: pick(branch) / type(text)
  - 模板：scenario 名 + round 进度 + prompt + chip 列表 + textarea
- [x] **T12.2** 暗黑模式适配
  - 与 MockPresetBar 一致（半透明 primary tint）
  - 4px-12px padding
- [x] **T12.3** 单元测试（4+）
  - 渲染 chip 列表
  - 点击 chip 触发 pick
  - 键入文本触发 type
  - round 进度显示

✅ **验收**: 组件测试全过，视觉与 MockPresetBar 一致

---

## T13. 集成到 AgentChat

**目标**: 在输入框上方挂 MockBranchChoiceBar

- [x] **T13.1** 修改 `app/encv-mobile/src/views/AgentChat.vue`
  - import + 引入 MockBranchChoiceBar
  - 在 footerInputRow 上方挂载
  - 绑定 mockBranchChoices / mockRoundState
  - 绑定 pick / type 事件
- [x] **T13.2** 暂停状态提示
  - 发送按钮在 user_text 空时显示「点击 chip 继续」hint
  - 历史按钮在暂停时仍可点（用户可能想保存当前进度）

✅ **验收**: 真实页面下，触发 mock_branch_choice 后输入框上方出现 chip 条，点击后剧本推进

---

## T14. i18n 增量

**目标**: 10+ 新 key 双语

- [x] **T14.1** 修改 `app/encv-mobile/src/i18n/agent.ts`
  - 加 10 个 key（branchChoicePrompt / roundProgress / roundPausedHint / toolDenied / toolRequiresConfirm / batchRenamePreview / batchRenameConfirm / editMetadataTitle / commandTimeout / commandDenied）
  - zh-CN + en 双语
- [x] **T14.2** 修改 `app/encv-mobile/src/i18n/settings.ts`
  - 加 5 个 key（toolWhitelist / toolWhitelistHelp / sandboxPaths / sandboxPathsHelp / mockRoundTimeout）
  - zh-CN + en 双语
- [x] **T14.3** 单元测试（1+）
  - 所有 key 在两个 locale 都存在

✅ **验收**: i18n 测试全过，UI 文案全部翻译

---

## T15. 端到端集成测试

**目标**: 真实 mount + 真实剧本 + 真实分支

- [x] **T15.1** 准备 sandbox 目录
  - [x] 创建 12+ 测试文件：mp4(3) / srt(2) / log(3) / json(2) / png(1) / 隐藏文件(1)
  - [x] 通过 t.TempDir() 隔离，t.Cleanup 自动删除
  - [x] 3 个 MP4 文件大小：150MB / 50MB / 200MB（fake ftyp + sparse）
  - [x] 2 个 SRT / 3 个 LOG（logs/error.log 含 "ERROR.*timeout" 匹配）
  - [x] 1 个 PNG（fake header + IHDR/IDAT/IEND）
  - [x] 1 个 .secret.txt 隐藏文件
  - 实现位置：`internal/server/agent_e2e_test.go::setupSandboxDir`
- [x] **T15.2** E2E: search_files 真实 mount
  - [x] 配置 mountResolver 把 "sandbox" 映射到 t.TempDir()
  - [x] 通过 tools.GlobalRegistry.Dispatch 派发 search_files
  - [x] AST：`and(name_glob=*.mp4, size_gt=100MB)`
  - [x] 验证 total=2（vacation_2024.mp4 150MB + old_video.mp4 200MB，clip001.mp4 50MB 排除）
  - [x] 额外 subtest：content_regex `ERROR.*timeout` + ext_eq=log
  - 实现位置：`TestE2E_SearchFiles_RealMount` / `TestE2E_SearchFiles_ContentRegex`
- [x] **T15.3** E2E: edit_metadata 多轮
  - [x] 构造 4 轮剧本（Round 0 选文件 → 1 选字段 → 2 输入新值 → 3 确认 + tool_call）
  - [x] 通过 MockEngineV2.Resume 推 4 轮 user text："1" / "title" / "New Title" / "yes"
  - [x] 验证 stream_end 事件被推
  - [x] 验证 sidecar `.metadata.json` 真实写入并含 `{"title":"New Title"}`
  - 实现位置：`TestE2E_EditMetadata_4Rounds`
- [x] **T15.4** E2E: branch_choice
  - [x] 启动 `branch_encrypt_or_decrypt` 剧本
  - [x] 验证 Run 推 mock_branch_choice 事件（含 3 个 branches）
  - [x] 验证 stream_end 尚未推（branch_choice 暂停）
  - [x] PickBranch("encrypt") → 推 mock_branch_picked + stream_end{branch_terminated}
  - [x] CurrentBranchID() == "encrypt"
  - [x] 额外 subtest：keyword 匹配（"我想加密文件" → encrypt 分支）
  - 实现位置：`TestE2E_BranchChoice_Encrypt` / `TestE2E_BranchChoice_KeywordMatch`
- [x] **T15.5** E2E: command_run
  - [x] 真实 `file` 命令（白名单）：stdout 包含文件名 + exit_code=0
  - [x] 真实 `ffprobe`（白名单）：fake MP4 触发 isError=true + stderr 捕获
  - [x] 黑名单 `rm -rf /` 被拒绝（"blacklist" / "whitelist" 错误）
  - 实现位置：`TestE2E_CommandRun_RealFileCommand` / `TestE2E_CommandRun_RealFfprobe` / `TestE2E_CommandRun_BlacklistDenied`

**附 T15 unblock 改造**（spec 关键阻断修复）:
- [x] 后端 `chatRequest` struct 新增 `Mode` + `Scenario` 字段（`internal/server/agent_api.go`）
- [x] 后端 `handleMockResume` 方法：在 `handleAgentChat` 中 route `body.Mode == "mock_resume"` 到 v2 引擎
- [x] 后端 `mockV2SessionEngines` 进程级 sync.Map：`session_id → *MockEngineV2` 维持 stateful 引擎
- [x] 后端 `getOrCreateV2Engine` / `lookupMockScenarioV2` helper（`internal/server/agent_mock_v2.go`）
- [x] 前端 `send()` body 新增 `scenario` 字段（`useAgent.ts`，mode === 'mock_resume' 时透传）
- [x] 前端 mock_resume 进入前清空 `mockBranchChoices` + 把 `mockRoundState.phase` 切到 'running'
- [x] `tools.RegisterAll()` 改幂等（同名 tool 已存在则跳过）以支持 e2e + WebDAV 测试共存

**综合 e2e**:
- [x] **TestE2E_MockResume_FullLoop** — 完整跑 v1 引擎 + `search_recursive_mp4` 剧本，验证 tool_result 真实命中 sandbox MP4
- [x] **TestE2E_Sandbox_ContainsAllRequiredFiles** — sandbox 完整性自检

✅ **验收**: `go test ./internal/server/... -run TestE2E -v` 10/10 全过（0.29s）

---

## T16. 性能与压力测试

**目标**: search_files 在大目录下的稳定性

- [x] **T16.1** 50000 文件扫描测试
  - [x] 准备 50001 个测试文件（50001 触发 MaxFilesScanned=50000 截断）
  - [x] search_files 全表扫描
  - [x] 验证返回时间 < 5s（实测 ~1.2s @50001 文件）
  - [x] 验证 scanned_limited: true + Total == MaxFilesScanned
  - 实现位置：`internal/tools/search_files_bench_test.go::TestSearchFiles_LargeScan_50000Files_Under5s`
  - 跑测命令：`BENCH_SCAN=1 go test -run 'TestSearchFiles_LargeScan_50000Files_Under5s' -v ./internal/tools/`
  - 跳过条件：`testing.Short()` 或缺 `BENCH_SCAN=1`
- [x] **T16.2** 多剧本并发测试
  - [x] 同时启动 5 个 mock 剧本流（取 mockScenariosV2[0..4]）
  - [x] 验证无 panic（goroutine recover）
  - [x] 验证 goroutine 数量 < baseline + 50（实测 delta=0）
  - [x] 验证内存增长 < 100MB（实测 +9.8KB）
  - 实现位置：`internal/server/agent_mock_v2_bench_test.go::TestMockEngineV2_ConcurrentScenarios`
  - 跑测命令：`BENCH_V2=1 go test -run 'TestMockEngineV2_ConcurrentScenarios' -v ./internal/server/`
  - 跳过条件：`testing.Short()` 或缺 `BENCH_V2=1`
- [x] **T16.3** 长时间运行稳定性
  - [x] 程序化构造 1000 轮剧本（每轮 1 个 text_delta 事件）
  - [x] 跑 1 Run + 999 Resume（交替 user A / user B 输入）
  - [x] 验证 stream_end 事件触发（1000 轮后正确收尾）
  - [x] 验证 EventCache ≥ 1000（实际 5000：含 round_state 事件）
  - [x] 验证无 goroutine 泄漏（实测 delta=0）
  - [x] 验证 HeapAlloc 增长 < 100MB（实测 GC 后反而 -438KB）
  - 实现位置：`internal/server/agent_mock_v2_bench_test.go::TestMockEngineV2_1000Rounds_NoLeak`
  - 跑测命令：`BENCH_V2=1 go test -run 'TestMockEngineV2_1000Rounds_NoLeak' -v ./internal/server/`
  - 跳过条件：`testing.Short()` 或缺 `BENCH_V2=1`
- [x] **T16.4** 可选 benchmarks（不在 -short 中跑）
  - [x] `BenchmarkSearchFiles_GlobPath_5000Files`
  - [x] `BenchmarkSearchFiles_ContentRegex_5000Files`
  - [x] `BenchmarkSearchFiles_Glob_50000Files`（需 `BENCH_SCAN=1`）
  - [x] `BenchmarkMockEngineV2_100Rounds`（需 `BENCH_V2=1`）
  - 跑测命令：`go test -bench=. -run=^$ ./internal/tools/ ./internal/server/`
- [x] **T16.5** 辅助工具
  - [x] `setupLargeDir(t, fileCount)` — 16 worker 并发写小文件（50001 文件 ~5.5s）
  - [x] `shouldRunLargeScan(t)` — 集中 skip 逻辑（-short + env var）
  - [x] `shouldRunV2Bench(t)` — 集中 skip 逻辑（-short + env var）
  - [x] `makeThousandRoundScenario(n)` — 程序化构造 n 轮合成剧本
  - [x] `humanBytes(int64)` — 字节数格式化
  - [x] `channelResponseWriter` — 最小 http.ResponseWriter + http.Flusher（满足"buffered channel as eventWriter"模式，编译期检查接口实现）

✅ **验收**:
- `go test -short ./internal/tools/ ./internal/server/` 全部通过或 skip
- `BENCH_SCAN=1 BENCH_V2=1 go test ./internal/tools/ ./internal/server/` 全量通过
- 实测性能：50001 文件扫描 1.16s（5s 限额的 23%）/ 5 并发 ~150ms / 1000 轮 1.27s

---

## T17. 文档与发布

**目标**: 用户能上手使用 v2 能力

- [x] **T17.1** README 更新（在 `.trae/documents/` 添加 v2-usage.md）
  - 工具清单 + 参数示例
  - 分支 / 多轮使用流程
  - 截图（如有）
- [x] **T17.2** 8 个 v2 剧本速查表
  - 触发关键词
  - 演示能力
  - 预期交互流程
- [x] **T17.3** CHANGELOG
  - v2 新增 8 工具 + 8 剧本 + 3 事件

✅ **验收**: `.trae/documents/v2-usage.md` + `CHANGELOG.md` 双语齐备
