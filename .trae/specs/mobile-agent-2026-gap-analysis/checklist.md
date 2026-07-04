# Checklist: 移动端 AI Agent 2026 现代化

> 每个 checkpoint 必须有 `[verify]` 标注的验证方法。
> **核心原则**：本 spec 是 additive，不修改 `go-in-process-agent` 与 `codex-web-gap-analysis` 已完成项。
> **严格禁止猜测**：所有引用的文件名/函数名必须能在 `/tmp/ai-agent-research/pi-repo/` 或 `/tmp/ai-agent-research/codex-web-repo/` 或 `/workspace/` 下找到。

---

## P0 验收

### Hooks 系统

- [x] `agent/types.go` 加 `HookEvent` string + 6 个常量（`HookSessionStart`/`HookTurnStart`/`HookTurnEnd`/`HookPreToolCall`/`HookPostToolCall`/`HookSessionShutdown`）
- [x] `agent/hooks.go` 定义 `HookFunc` + `HookContext{Event, SessionID, Messages, ToolCall, ToolResult}`
- [x] `agent/agent.go` `Agent` struct 加 `hooks []HookFunc` + `RegisterHook(HookFunc)`
- [x] 6 个事件点全部插入 hook 调度
- [x] `agent/agent_test.go` `TestHooks_*` 覆盖 6 事件点
- [x] [verify] `cd /workspace && go test -race ./agent/... -run TestHooks` 通过

### Durable Session

- [x] `agent/session_store.go` 定义 `SessionStore{root, mu}` + `Append` + `Load`
- [x] 写入路径 `~/.encv/agent/sessions/{sessionId}.jsonl`
- [x] `agent/agent.go` `SessionCache` 启动时自动 load
- [x] `agent.Resume` 在 cache miss 时 fall back to `SessionStore.Load`
- [x] 磁盘写入失败时不 panic，仅 warning
- [x] `agent/session_store_test.go` 覆盖写入 / 读取 / 重启模拟
- [x] [verify] `cd /workspace && go test -race ./agent/... -run TestSessionStore` 通过

### appServerRealtimeReducer

- [x] `app/encv-mobile/src/composables/appServerRealtimeReducer.ts` 定义 `MinimalRealtimeEvent`
- [x] 实现 `readRealtimeThreadId` / `readRealtimeCacheVersion` / `readRealtimeServerInstance`（参考 `codex-web-repo/apps/web/src/app/realtimeState.ts:61-86`）
- [x] 实现 `updateRealtimeServerInstance`（参考 `realtimeState.ts:88-99`）
- [x] `useAgent.ts` 集成 reducer 处理 SSE
- [x] `appServerRealtimeReducer.test.ts` 覆盖 instance 切换 / sequence 去重
- [x] [verify] `cd /workspace/app/encv-mobile && pnpm test -- appServerRealtimeReducer` 通过

### Server Instance + Sequence 去重

- [x] `agent/http.go` `/api/health` 返回 `serverInstanceId`（`os.Hostname() + pid` 哈希）
- [x] `useAgent.ts` 加 `currentServerInstance` + `seenSequences: Set<number>`（参考 `realtimeState.ts:21-27` 的 `MAX_TRACKED_REALTIME_SEQUENCES = 2_000`）
- [x] SSE event 处理时检查 sequence，已见则丢弃
- [x] instance 变化时清空 seenSequences
- [x] [verify] `pnpm test -- useAgent` 通过 + 浏览器实测 agent 重启后无重复事件

### 两级操作分组

- [x] review `components/agent/GroupedOperationMessage.vue`（154 行）确认现状
- [x] 改造为两层：外层 `OperationGroupSummary` + 内层 `OperationItemDetail`
- [x] review `components/agent/FileChangeSummaryMessage.vue`（179 行）确认现状
- [x] 改造为两级：外层"已编辑 N 个文件" + 内层 diff
- [x] 加常量 `OPERATION_COLLAPSE_INITIAL_COUNT = 3`（参考 `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:70` `FILE_CHANGE_INITIAL_ROW_COUNT = 3`）
- [x] [verify] 浏览器实测：触发 5 条 command + 3 个 file change → 双层折叠生效

### 活跃态归一化

- [x] 新建 `app/encv-mobile/src/composables/activeStatus.ts`
- [x] 定义 `compactStatus(value)` / `isActiveStatus(value)` / `readTurnStatus(value)`
- [x] 4 个归一化集合（active / completed / failed / interrupted）
- [x] active 集合包含：`active / inprogress / running / editing / thinking / in_progress / streaming`（参考 `codex-web-repo/apps/web/src/app/appServerRealtimeReducer.ts:61-89`）
- [x] `useAgent.ts` 集成归一化
- [x] 渲染"正在运行 / 正在编辑 / 正在思考" 三个独立文案
- [x] `activeStatus.test.ts` 覆盖 4 集合 × 各状态字符串
- [x] [verify] 浏览器实测：AI 调 tool 时无论后端推 `running` / `editing` / `thinking` 都正确显示

---

## P1 验收

### Compaction

- [x] `agent/types.go` 加 `EventTypeCompaction` + `CompactionData{SummaryText, ReplacedMessageCount}`
- [x] `agent/compaction.go` 实现 `maybeCompact(messages, modelContextWindow)`
- [x] 阈值触发：token > 80% of model context window
- [x] 后端推 `EventTypeCompaction` 事件
- [x] useAgent 处理 `compaction` 事件
- [x] 新建 `components/agent/ContextCompactionDivider.vue`（参考 `codex-web-repo/docs/implementation_status.md` 2026-05-30 段 — 不可展开分隔线）
- [x] [verify] 浏览器实测：长对话自动压缩生效

### Skills 注册表

- [x] `agent/skills.go` 定义 `Skill{Name, Description, Prompt}` + `ScanSkills(root string)`
- [x] 扫描 `~/.encv/skills/*/SKILL.md`（仿 Claude Code + 参考 `pi-repo/packages/agent/docs/skills.md`）
- [x] SKILL.md frontmatter 解析（YAML `name:` / `description:` / body 是 prompt）
- [x] `session_start` hook 注入 selected skills 到 system prompt
- [x] 加 1 个示例 skill：`~/.encv/skills/video-encrypt/SKILL.md`
- [x] `skills_test.go` 覆盖扫描 + 解析 + 注入
- [x] [verify] `cd /workspace && go test ./agent/... -run TestSkills` 通过

### Plan / Todo 工具

- [x] `agent/types.go` 加 `ToolKindPlan` 常量
- [x] `agent/registry.go` 内置 `write_todos` 工具（参考 `pi-repo/packages/agent/docs/plans.md`）
- [x] schema: `[{id, status, content}]`
- [x] handler 推 `EventToolStatus` + `EventToolResult` + 内部存 todos
- [x] 新建 `components/agent/PlanBlock.vue`
- [x] `renderTurnItems.ts` 加 `type: 'plan'` 分支
- [x] AgentChat 加 plan 渲染分支
- [x] [verify] 浏览器实测：用户要求"先列文件再删除" → AI 拆 plan → UI 进度显示

### Slash 菜单

- [x] 新建 `components/agent/SlashMenu.vue`
- [x] 接收 `items: SlashMenuItem[]` + `onApply(id)`（参考 `codex-web-repo/apps/web/src/app/components/Composer.tsx:62-73` `SlashMenuItem`）
- [x] 定义 `SlashMenuItem{id, group: "功能" | "技能", label, description, icon, apply}`
- [x] AgentChat textarea 加 `@input` 监听，匹配 `/^\s*\/\S*$/` 时打开菜单
- [x] 菜单项数据源 = 后端 `/api/skills` + 静态功能列表
- [x] 键盘导航：↑↓ 移动高亮，Enter 应用，Esc 关闭
- [x] i18n key 完整：`agent.slashMenuTitle` / `agent.slashMenuFeatures` / `agent.slashMenuSkills` / `agent.slashMenuNoMatches`
- [x] [verify] 浏览器实测：Composer 输入 `/` → 看到菜单 → 选 `video-encrypt` skill → 关闭菜单

### Steer / Queue 双轨

- [x] `useAgent.ts` `SendOptions` 加 `mode: "start" | "steer" | "queue"`（参考 `codex-web-repo/apps/web/src/app/components/Composer.tsx:55`）
- [x] `agent/http.go` `ChatRequest` 接受 `mode` 字段
- [x] 后端 steer 路径：调 LLM with current messages + new user message
- [x] 后端 queue 路径：缓存到 `pendingMessages[sessionID]`，stream_end 时自动发送
- [x] AgentChat 在 `status='streaming'` 时显示双按钮
- [x] i18n key：`agent.steer` / `agent.queue` / `agent.queuedHint`
- [x] [verify] 浏览器实测：streaming 时输入 → 双按钮 → steer 立即被 AI 接收

### Attach 图片

- [x] `useAgent.ts` `ComposerDraft` 加 `attachments: Attachment[]`（参考 `codex-web-repo/apps/web/src/app/components/Composer.tsx:75-78`）
- [x] `Attachment{Id, Name, MimeType, SizeBytes, DataUrl, Kind: "image" | "file"}`
- [x] 新建 `components/agent/AttachmentTray.vue`
- [x] 区分 image（缩略图行）和 file（卡片行），均显示在 textarea 上方
- [x] AgentChat 加 `+` 按钮触发 file picker
- [x] 发送时 attachments 编入 `messages[-1].content` 数组
- [x] 触发 steer 路径时若含 attach → 自动转 queue（参考 `docs/implementation_status.md` 2026-05-30 段）
- [x] [verify] 浏览器实测：附 1 张图 + 1 个 .mp4 → textarea 上方显示 → 发送 → AI 收到

### 行内文件引用

- [x] 新建 `composables/inlineFileReference.ts`
- [x] 定义 `FILE_REFERENCE_EXTENSIONS` 列表（参考 `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:72`）
- [x] 实现 `parseFileReferences(text): {start, end, path, line, col}[]`
- [x] 新建 `components/agent/FileReferenceChip.vue` props `{path, line, col}`
- [x] AssistantMessage 解析消息文本，识别到的 path 替换为 chip
- [x] chip click 弹轻量菜单：复制路径 / 复制相对路径 / 在 Files tab 打开
- [x] i18n key 完整
- [x] [verify] 浏览器实测：AI 回复含 `src/main.go:42` → 渲染为蓝色 chip → 点击弹菜单

### 滚动到底部按钮

- [x] 新建 `components/agent/ScrollToBottomButton.vue`
- [x] AgentChat 监听 `onMainScroll` 判断 `nearBottom`
- [x] 选区存在时暂停自动滚动（参考 `codex-web-repo/docs/implementation_status.md` 2026-05-31 段）
- [x] [verify] 浏览器实测：阅读旧消息时按钮出现 → 点击回到最新

### 实时事件 Debounce

- [x] `useAgent.ts` 加 `flushTimer: number | null`
- [x] SSE 事件累积到 50ms 短 debounce 窗口再 setState
- [x] 选区存在时跳过 flush
- [x] [verify] 浏览器实测：长消息流式输出时用户可选中文本不被刷掉

---

## P2 验收

### System Prompt per-session override

- [x] `agent_settings` schema 加 `session_overrides: { sessionId: { system_prompt } }`
- [x] `session_start` hook 优先用 session override，其次全局
- [x] [verify] 修改 session override → 下一轮 LLM 收到新 prompt

### Tool Policy

- [x] `agent/registry.go` 加 `Policy{ToolName, Allowed: "readonly" | "write" | "all"}`（参考 `pi-repo/packages/agent/docs/tools-policy.md`）
- [x] `agent/agent.go` `SessionCache` 加 `toolPolicy`
- [x] 工具执行前检查 policy，违反返回 error
- [x] [verify] 切 readonly → fileChange 工具拒绝

### Provider 抽象层

- [x] 新建 `agent/ai/provider.go` 定义 `Provider interface`（参考 `pi-repo/packages/ai/README.md`）
- [x] `agent/openai.go` 移到 `agent/ai/openai.go`
- [x] 新建 `agent/ai/anthropic.go`
- [x] 新建 `agent/ai/gemini.go`
- [x] `agent/agent.go` `NewAgent` 路由 provider
- [x] [verify] 切换 provider → 走对应 API

### Plan Mode Toggle

- [x] `useAgent.ts` 加 `planMode: boolean` ref
- [x] AgentChat Composer 底栏加"目标" toggle
- [x] 开启时 `/api/chat` 带 `planMode: true` → 后端注入 plan-aware system prompt
- [x] [verify] 切 plan mode → AI 拆 step-by-step

### Permission Mode Switcher

- [x] `useAgent.ts` `PermissionMode: "default" | "auto-review" | "full-access"`
- [x] 后端接受 `permissionMode` 字段
- [x] default → needConfirm=true；auto-review → 自动执行；full-access → 跳过 approval
- [x] [verify] 切 full-access → 不弹 ApprovalCard

### Reasoning i18n

- [x] i18n 加 `agent.reasoningEffort.low` / `medium` / `high` / `xhigh` 翻译
- [x] 后端返回的 `reasoningEffort` 在前端翻译（参考 `codex-web-repo/docs/implementation_status.md` 2026-05-30 段）
- [x] [verify] `xhigh` 显示为"超高"

### Agent Task Message

- [x] `MessageItem` 加 `type: 'agentTask'`（参考 `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:43-44` `AgentTaskItem`）
- [x] 折叠阈值常量 `AGENT_TASK_COLLAPSE_LINE_COUNT = 7` / `AGENT_TASK_COLLAPSE_CHAR_COUNT = 520`（参考 `MessageBlocks.tsx:68-69`）
- [x] 新建 `AgentTaskMessage.vue`
- [x] [verify] AI 拆 subagent → 看到 agent task 块

### Side Conversation / Fork

- [x] `useAgent.ts` `Session` 加 `parentSessionId?`
- [x] `agent/agent.go` `NewSession(parentID)` 派生
- [x] AgentChat 加 "分叉此会话" 按钮
- [x] [verify] 分叉后两个 session 独立运行

### Events JSONL + Replay

- [x] Task 2 的 `SessionStore` 已实现，扩展为 JSONL
- [x] `agent/cmd/agent-demo/main.go` 加 `--replay {sessionId}`
- [x] [verify] 跑 `--replay` 看到历史 events 顺序重放

### Sync Doctor

- [x] 新建 `agent/sync_doctor.go`（参考 `codex-web-repo/apps/server/src/syncDoctor.ts`）
- [x] 加 `/api/sync/doctor` 端点
- [x] AgentSettingsDetail 加 "运行 sync 诊断" 按钮
- [x] [verify] 移动端跑诊断 → 看到脱敏 JSON

### LAN Access

- [x] 新建 `agent/lan_access.go`（参考 `codex-web-repo/apps/server/src/lanAccess.ts`）
- [x] 加 `/api/network/lan-access` 端点
- [x] AgentSettingsDetail Network 面板展示
- [x] [verify] 显示 `http://192.168.x.x:5245/`

---

## 端到端联调

### 三进程跑通

- [x] `agent-demo` 进程在 :5245 跑起来
- [x] OpenList 进程跑起来
- [x] `pnpm dev` 启动 encv-mobile
- [x] 浏览器实测所有 P0 + P1 验收项

### 单测覆盖率

- [x] `cd /workspace && go test -race ./agent/...` 全绿
- [x] `cd /workspace/app/encv-mobile && pnpm test` vitest 全绿
- [x] Go 覆盖率 ≥ 70%
- [x] TS 覆盖率 ≥ 70%

### 性能

- [x] Chat 首次 Event 延迟 < 500ms
- [x] Resume 追平进度延迟 < 1s（50ms polling 保留）
- [x] 1000+ 消息虚拟列表仍流畅
- [x] agent → OpenList `/api/ext/*` P95 < 100ms

### 兼容性

- [x] 不破坏现有 `agent_settings` 字段
- [x] 不破坏现有 SSE 6 种 event 协议（text_delta / reasoning_delta / tool_call / tool_status / tool_result / stream_end）
- [x] 不破坏 12 个插件工具 + 8 个 OpenList 工具注册表
- [x] 不破坏 `app/encv-mobile/src/config/schema.json` 现有 schema

### 文档同步

- [x] `agent/README.md` 增补 Hooks / Skills / Plan / Provider 章节
- [x] `app/encv-mobile/src/components/agent/README.md` 增补 SlashMenu / PlanBlock / FileReferenceChip 等组件
- [x] i18n key 全量翻译（zh-CN / en）
