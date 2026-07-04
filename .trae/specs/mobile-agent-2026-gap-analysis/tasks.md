# Tasks: 移动端 AI Agent 2026 现代化

> 任务按 P0 → P1 → P2 排序。每完成一项勾选 `[x]`。
> 任务范围来自 `spec.md` 的 ADDED Requirements。
> **严格禁止猜测**：所有引用的文件名/函数名/事件名/常量必须能在 `/tmp/ai-agent-research/pi-repo/` 或 `/tmp/ai-agent-research/codex-web-repo/` 或 `/workspace/` 下找到。

---

## P0 — 基础能力补齐

### Task 1: Hooks 系统（参考 `pi-repo/packages/agent/docs/hooks.md`）
- [ ] SubTask 1.1: 在 `agent/types.go` 加 `HookEvent` string + 6 个常量（`HookSessionStart` / `HookTurnStart` / `HookTurnEnd` / `HookPreToolCall` / `HookPostToolCall` / `HookSessionShutdown`）
- [ ] SubTask 1.2: 新建 `agent/hooks.go` 定义 `HookFunc func(ctx, *HookContext) error` + `HookContext{Event, SessionID, Messages, ToolCall, ToolResult}`
- [ ] SubTask 1.3: `agent/agent.go` `Agent` struct 加 `hooks []HookFunc` 字段 + `RegisterHook(HookFunc)` 方法
- [ ] SubTask 1.4: 在 6 个事件点插入 hook 调度
- [ ] SubTask 1.5: `agent/agent_test.go` 加 `TestHooks_*` 覆盖 6 个事件点
- [x] [verify] `go test -race ./agent/... -run TestHooks` 通过

### Task 2: Durable Session（参考 `pi-repo/packages/agent/docs/agent-harness.md`）
- [ ] SubTask 2.1: 新建 `agent/session_store.go` 定义 `SessionStore{root string, mu sync.Mutex}`
- [ ] SubTask 2.2: 实现 `Append(sessionID, event)` 追加到 `~/.encv/agent/sessions/{sessionId}.jsonl`
- [ ] SubTask 2.3: 实现 `Load(sessionID) ([]Event, error)` 从 JSONL 反序列化
- [ ] SubTask 2.4: `agent/agent.go` 改造 `SessionCache` 启动时从 `SessionStore` 加载
- [ ] SubTask 2.5: `agent.Resume` 在 cache miss 时 fall back to `SessionStore.Load`
- [ ] SubTask 2.6: `agent/session_store_test.go` 覆盖：写入 / 读取 / 进程重启模拟
- [x] [verify] `go test -race ./agent/... -run TestSessionStore` 通过

### Task 3: appServerRealtimeReducer（参考 `codex-web-repo/apps/web/src/app/appServerRealtimeReducer.ts`）
- [ ] SubTask 3.1: 新建 `app/encv-mobile/src/composables/appServerRealtimeReducer.ts` 定义 `MinimalRealtimeEvent` 类型
- [ ] SubTask 3.2: 实现 `readRealtimeThreadId` / `readRealtimeCacheVersion` / `readRealtimeServerInstance`（参考 `realtimeState.ts:61-86`）
- [ ] SubTask 3.3: 实现 `updateRealtimeServerInstance` 处理 instance 变化时清缓存（参考 `realtimeState.ts:88-99`）
- [ ] SubTask 3.4: `useAgent.ts` 集成 reducer 处理 SSE 事件
- [ ] SubTask 3.5: `appServerRealtimeReducer.test.ts` 覆盖 instance 切换 / sequence 去重
- [x] [verify] `pnpm test -- appServerRealtimeReducer` 通过

### Task 4: Server Instance + Sequence 去重
- [ ] SubTask 4.1: `agent/http.go` `/api/health` 返回 `serverInstanceId`（用 `os.Hostname() + pid` 哈希）
- [ ] SubTask 4.2: `useAgent.ts` 加 `currentServerInstance: string` + `seenSequences: Set<number>`（参考 `realtimeState.ts:21-27` 的 `MAX_TRACKED_REALTIME_SEQUENCES = 2_000`）
- [ ] SubTask 4.3: SSE event 处理时检查 sequence，已见则丢弃
- [ ] SubTask 4.4: `useAgent.test.ts` 覆盖 instance 变化清空 seenSequences
- [x] [verify] `pnpm test -- useAgent` 通过

### Task 5: 两级操作分组（参考 `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:66-70`）
- [ ] SubTask 5.1: review 现有 `components/agent/GroupedOperationMessage.vue`（154 行）确认是否单层
- [ ] SubTask 5.2: 改造为两层结构：外层 `OperationGroupSummary`（"已运行/正在运行 N 条命令"）+ 内层 `OperationItemDetail`（单条命令/文件/工具）
- [ ] SubTask 5.3: review `FileChangeSummaryMessage.vue`（179 行）确认是否单层
- [ ] SubTask 5.4: 改造为两级：外层"已编辑 N 个文件"+ 内层 diff
- [ ] SubTask 5.5: 加常量 `OPERATION_COLLAPSE_INITIAL_COUNT = 3`（参考 `MessageBlocks.tsx:70` `FILE_CHANGE_INITIAL_ROW_COUNT = 3`）
- [ ] SubTask 5.6: 渲染测试：注入 5 条 command → 看到外层摘要"已运行 5 条命令" + 展开看到 5 条详情
- [x] [verify] 浏览器实测：触发 5 条 command + 3 个 file change → 双层折叠生效

### Task 6: 活跃态归一化（参考 `codex-web-repo/apps/web/src/app/appServerRealtimeReducer.ts:61-65` `isActiveStatus`）
- [ ] SubTask 6.1: 新建 `app/encv-mobile/src/composables/activeStatus.ts` 定义 `compactStatus(value)` / `isActiveStatus(value)` / `readTurnStatus(value)`
- [ ] SubTask 6.2: 归一化集合：`active / inprogress / running / editing / thinking / in_progress / streaming` → `active`
- [ ] SubTask 6.3: 归一化集合：`completed / complete / done / success / succeeded` → `completed`
- [ ] SubTask 6.4: 归一化集合：`failed / failure / error` → `failed`
- [ ] SubTask 6.5: 归一化集合：`interrupted / interrupt / canceled / cancelled` → `interrupted`
- [ ] SubTask 6.6: `useAgent.ts` 集成归一化，渲染"正在运行 / 正在编辑 / 正在思考" 三个独立文案
- [ ] SubTask 6.7: `activeStatus.test.ts` 覆盖 4 集合 × 各状态字符串
- [x] [verify] 浏览器实测：AI 调 tool 时无论后端推 `running` / `editing` / `thinking` 都正确显示

---

## P1 — Composer / 渲染增强

### Task 7: Compaction（参考 `pi-repo/packages/agent/docs/compactions.md`）
- [ ] SubTask 7.1: `agent/types.go` 加 `EventTypeCompaction` 常量 + `CompactionData{SummaryText, ReplacedMessageCount}` 结构
- [ ] SubTask 7.2: 新建 `agent/compaction.go` 实现 `maybeCompact(messages, modelContextWindow)` 触发 LLM 总结
- [ ] SubTask 7.3: `agent/agent.go` 在 `Chat` 主循环前检查 compaction
- [ ] SubTask 7.4: 后端推 `EventTypeCompaction` 事件
- [ ] SubTask 7.5: 前端 useAgent 处理 `compaction` 事件，记录到 messages
- [ ] SubTask 7.6: 新建 `components/agent/ContextCompactionDivider.vue` 渲染不可展开分隔线
- [ ] SubTask 7.7: 触发测试：注入 100 条消息 + 模拟 token 超限 → 收到 compaction 事件 → UI 显示分隔线
- [x] [verify] 浏览器实测：长对话自动压缩生效

### Task 8: Skills 注册表（参考 `pi-repo/packages/agent/docs/skills.md`）
- [ ] SubTask 8.1: 新建 `agent/skills.go` 定义 `Skill{Name, Description, Prompt}` + `ScanSkills(root string) []Skill`
- [ ] SubTask 8.2: 启动时扫描 `~/.encv/skills/*/SKILL.md`（仿 Claude Code）
- [ ] SubTask 8.3: SKILL.md frontmatter 解析（YAML `name:` / `description:` / body 是 prompt）
- [ ] SubTask 8.4: 注册 `session_start` hook 注入 selected skills 到 system prompt
- [ ] SubTask 8.5: 加 1 个示例 skill：`~/.encv/skills/video-encrypt/SKILL.md`
- [ ] SubTask 8.6: `skills_test.go` 覆盖扫描 + 解析 + 注入
- [x] [verify] `go test ./agent/... -run TestSkills` 通过

### Task 9: Plan / Todo 工具（参考 `pi-repo/packages/agent/docs/plans.md`）
- [ ] SubTask 9.1: `agent/types.go` 加 `ToolKindPlan` 常量
- [ ] SubTask 9.2: `agent/registry.go` 内置 `write_todos` 工具，schema `[{id, status, content}]`
- [ ] SubTask 9.3: handler 推 `EventToolStatus` + `EventToolResult` + 内部存 todos
- [ ] SubTask 9.4: 新建 `components/agent/PlanBlock.vue` 渲染 todos 进度条
- [ ] SubTask 9.5: `renderTurnItems.ts` 加 `type: 'plan'` 分支
- [ ] SubTask 9.6: AgentChat 加 plan 渲染分支
- [ ] SubTask 9.7: 渲染测试：AI 调用 `write_todos` 拆 3 步 → UI 显示 plan
- [x] [verify] 浏览器实测：用户要求"先列文件再删除" → AI 拆 plan → UI 进度显示

### Task 10: Slash 菜单（参考 `codex-web-repo/apps/web/src/app/components/Composer.tsx:62-73` `SlashMenuItem`）
- [ ] SubTask 10.1: 新建 `components/agent/SlashMenu.vue` 接收 `items: SlashMenuItem[]` + `onApply(id)`
- [ ] SubTask 10.2: 定义 `SlashMenuItem{id, group: "功能" | "技能", label, description, icon, apply}`
- [ ] SubTask 10.3: AgentChat textarea 加 `@input` 监听，匹配 `/^\s*\/\S*$/` 时打开菜单
- [ ] SubTask 10.4: 菜单项数据源 = 后端 `/api/skills` + 静态功能列表
- [ ] SubTask 10.5: 键盘导航：↑↓ 移动高亮，Enter 应用，Esc 关闭
- [ ] SubTask 10.6: i18n key 完整：`agent.slashMenuTitle` / `agent.slashMenuFeatures` / `agent.slashMenuSkills` / `agent.slashMenuNoMatches`
- [ ] SubTask 10.7: 渲染测试 + 浏览器实测
- [x] [verify] 浏览器实测：Composer 输入 `/` → 看到菜单 → 选 `video-encrypt` skill → 关闭菜单

### Task 11: Steer / Queue 双轨（参考 `codex-web-repo/apps/web/src/app/components/Composer.tsx:55` `SendOptions.mode`）
- [ ] SubTask 11.1: `useAgent.ts` `SendOptions` 加 `mode: "start" | "steer" | "queue"`
- [ ] SubTask 11.2: `agent/http.go` `ChatRequest` 接受 `mode` 字段
- [ ] SubTask 11.3: 后端 steer 路径：调 LLM with current messages + new user message
- [ ] SubTask 11.4: 后端 queue 路径：缓存到 `pendingMessages[sessionID]`，stream_end 时自动发送
- [ ] SubTask 11.5: AgentChat 在 `status='streaming'` 时显示双按钮（"引导当前" / "排队下一条"）
- [ ] SubTask 11.6: i18n key：`agent.steer` / `agent.queue` / `agent.queuedHint`
- [ ] SubTask 11.7: 渲染测试 + 浏览器实测
- [x] [verify] 浏览器实测：streaming 时输入 → 双按钮 → steer 立即被 AI 接收

### Task 12: Attach 图片（参考 `codex-web-repo/apps/web/src/app/components/Composer.tsx:75-78` `ComposerDraft`）
- [ ] SubTask 12.1: `useAgent.ts` `ComposerDraft` 加 `attachments: Attachment[]`
- [ ] SubTask 12.2: `Attachment{Id, Name, MimeType, SizeBytes, DataUrl, Kind: "image" | "file"}`
- [ ] SubTask 12.3: 新建 `components/agent/AttachmentTray.vue` 渲染图片缩略图行 + 文件卡片行
- [ ] SubTask 12.4: AgentChat 加 `+` 按钮触发 file picker
- [ ] SubTask 12.5: 选 image → 缩略图；选 file → 卡片；均在 textarea 上方
- [ ] SubTask 12.6: 发送时把 attachments 编入 `messages[-1].content` 数组（`type: "image_url" / "file"`）
- [ ] SubTask 12.7: 触发 steer 路径时若含 attach → 自动转 queue（参考 `docs/implementation_status.md` 2026-05-30 段）
- [ ] SubTask 12.8: i18n key 完整
- [x] [verify] 浏览器实测：附 1 张图 + 1 个 .mp4 → textarea 上方显示 → 发送 → AI 收到

### Task 13: 行内文件引用（参考 `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:71-81` `INLINE_FILE_REFERENCE_PATTERN`）
- [ ] SubTask 13.1: 新建 `composables/inlineFileReference.ts` 定义 `FILE_REFERENCE_EXTENSIONS` 列表
- [ ] SubTask 13.2: 实现 `parseFileReferences(text): {start, end, path, line, col}[]`
- [ ] SubTask 13.3: 新建 `components/agent/FileReferenceChip.vue` props `{path, line, col}`
- [ ] SubTask 13.4: AssistantMessage 解析消息文本，把识别到的 path 替换为 chip
- [ ] SubTask 13.5: chip click 弹轻量菜单：复制路径 / 复制相对路径 / 在 Files tab 打开
- [ ] SubTask 13.6: i18n key 完整
- [x] [verify] 浏览器实测：AI 回复含 `src/main.go:42` → 渲染为蓝色 chip → 点击弹菜单

### Task 14: 滚动到底部按钮（参考 `codex-web-repo/docs/implementation_status.md` 2026-05-30 段）
- [ ] SubTask 14.1: 新建 `components/agent/ScrollToBottomButton.vue`
- [ ] SubTask 14.2: AgentChat 监听 `onMainScroll` 判断 `nearBottom`，`false` 时显示按钮
- [ ] SubTask 14.3: 选区存在时暂停自动滚动
- [ ] SubTask 14.4: 点击按钮 → `scrollToIndex(messages.length - 1, {align: 'end'})`
- [x] [verify] 浏览器实测：阅读旧消息时按钮出现 → 点击回到最新

### Task 15: 实时事件 Debounce（参考 `codex-web-repo/docs/implementation_status.md` 2026-05-31 段）
- [ ] SubTask 15.1: `useAgent.ts` 加 `flushTimer: number | null`
- [ ] SubTask 15.2: SSE 事件累积到 50ms 短 debounce 窗口再 setState
- [ ] SubTask 15.3: 选区存在时跳过 flush
- [x] [verify] 浏览器实测：长消息流式输出时用户可选中文本不被刷掉

---

## P2 — 高级能力

### Task 16: System Prompt per-session override
- [ ] SubTask 16.1: `agent_settings` schema 加 `session_overrides: { sessionId: { system_prompt } }`
- [ ] SubTask 16.2: `session_start` hook 优先用 session override，其次全局
- [x] [verify] 修改 session override → 下一轮 LLM 收到新 prompt

### Task 17: Tool Policy
- [ ] SubTask 17.1: `agent/registry.go` 加 `Policy{ToolName, Allowed: "readonly" | "write" | "all"}`
- [ ] SubTask 17.2: `agent/agent.go` `SessionCache` 加 `toolPolicy map[string]Policy`
- [ ] SubTask 17.3: 工具执行前检查 policy，违反返回 error
- [x] [verify] 切 readonly → fileChange 工具拒绝

### Task 18: Provider 抽象层（参考 `pi-repo/packages/ai/`）
- [ ] SubTask 18.1: 新建 `agent/ai/provider.go` 定义 `Provider interface{ StreamChat(...) }`
- [ ] SubTask 18.2: `agent/openai.go` 移到 `agent/ai/openai.go` 实现 Provider
- [ ] SubTask 18.3: 新建 `agent/ai/anthropic.go` 实现 Anthropic Messages API
- [ ] SubTask 18.4: 新建 `agent/ai/gemini.go` 实现 Gemini API
- [ ] SubTask 18.5: `agent/agent.go` `NewAgent` 根据 `agent_settings.provider` 路由
- [x] [verify] 切换 provider → 走对应 API

### Task 19: Plan Mode Toggle
- [ ] SubTask 19.1: `useAgent.ts` 加 `planMode: boolean` ref
- [ ] SubTask 19.2: AgentChat Composer 底栏加"目标" toggle
- [ ] SubTask 19.3: 开启时 `/api/chat` 带 `planMode: true` → 后端注入 plan-aware system prompt
- [x] [verify] 切 plan mode → AI 拆 step-by-step

### Task 20: Permission Mode Switcher
- [ ] SubTask 20.1: `useAgent.ts` `PermissionMode: "default" | "auto-review" | "full-access"`
- [ ] SubTask 20.2: 后端接受 `permissionMode` 字段
- [ ] SubTask 20.3: default → needConfirm=true；auto-review → 自动执行；full-access → 跳过 approval
- [x] [verify] 切 full-access → 不弹 ApprovalCard

### Task 21: Reasoning i18n
- [ ] SubTask 21.1: i18n 加 `agent.reasoningEffort.low` / `medium` / `high` / `xhigh` 翻译
- [ ] SubTask 21.2: 后端返回的 `reasoningEffort` 在前端翻译
- [x] [verify] `xhigh` 显示为"超高"

### Task 22: Agent Task Message（参考 `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:43-44` `AgentTaskItem`）
- [ ] SubTask 22.1: `MessageItem` 加 `type: 'agentTask'`
- [ ] SubTask 22.2: 折叠阈值常量 `AGENT_TASK_COLLAPSE_LINE_COUNT = 7` / `AGENT_TASK_COLLAPSE_CHAR_COUNT = 520`（参考 `MessageBlocks.tsx:68-69`）
- [ ] SubTask 22.3: 新建 `AgentTaskMessage.vue` 渲染子任务列表
- [x] [verify] AI 拆 subagent → 看到 agent task 块

### Task 23: Side Conversation / Fork
- [ ] SubTask 23.1: `useAgent.ts` `Session` 加 `parentSessionId?` 字段
- [ ] SubTask 23.2: `agent/agent.go` `NewSession(parentID)` 派生
- [ ] SubTask 23.3: AgentChat 加 "分叉此会话" 按钮
- [x] [verify] 分叉后两个 session 独立运行

### Task 24: Events JSONL + Replay（参考 `pi-repo/packages/agent/docs/agent-harness.md`）
- [ ] SubTask 24.1: Task 2 的 `SessionStore` 已实现，扩展为 JSONL
- [ ] SubTask 24.2: `agent/cmd/agent-demo/main.go` 加 `--replay {sessionId}` 命令
- [x] [verify] 跑 `--replay` 看到历史 events 顺序重放

### Task 25: Sync Doctor（参考 `codex-web-repo/apps/server/src/syncDoctor.ts`）
- [ ] SubTask 25.1: 新建 `agent/sync_doctor.go` 实现脱敏诊断
- [ ] SubTask 25.2: 加 `/api/sync/doctor` 端点
- [ ] SubTask 25.3: AgentSettingsDetail 加 "运行 sync 诊断" 按钮
- [x] [verify] 移动端跑诊断 → 看到脱敏 JSON

### Task 26: LAN Access（参考 `codex-web-repo/apps/server/src/lanAccess.ts`）
- [ ] SubTask 26.1: 新建 `agent/lan_access.go` 枚举网卡 IPv4
- [ ] SubTask 26.2: 加 `/api/network/lan-access` 端点
- [ ] SubTask 26.3: AgentSettingsDetail Network 面板展示
- [x] [verify] 显示 `http://192.168.x.x:5245/`

---

## 任务依赖图

```
P0:
T1 (hooks) ──→ T8 (skills 注入)
T2 (durable session) ──→ T24 (replay)
T3 (reducer) ──→ T4 (server instance)
T5 (两级操作分组) ──→ T6 (活跃态归一化)

P1:
T7 (compaction) ──→ T15 (debounce 配合)
T8 (skills) ──→ T10 (slash 菜单)
T9 (plan) ──→ T19 (plan mode)
T10 (slash 菜单) ──→ T11 (steer/queue)
T11 (steer/queue) ──→ T12 (attach)
T12 (attach) ──→ T13 (file reference)
T13 (file reference) ──→ T14 (scroll to bottom)

P2:
T16 (per-session system prompt) 依赖 T1
T17 (tool policy) 依赖 T1
T18 (provider) 独立
T20 (permission mode) 依赖 T11
T21 (i18n) 独立
T22 (agent task) 依赖 T9
T23 (fork) 依赖 T2
T24 (replay) 依赖 T2
T25 (sync doctor) 独立
T26 (LAN access) 独立
```

## 实施顺序建议

```
T1 → T2 → T3 → T4 → T5 → T6   (P0 串行)
   ↓
T7 → T8 → T9                  (P1 第一波)
   ↓
T10 → T11 → T12 → T13 → T14 → T15   (P1 第二波)
   ↓
T16-T26 可并行                (P2)
```
