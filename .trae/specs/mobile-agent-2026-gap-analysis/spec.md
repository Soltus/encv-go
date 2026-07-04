# 移动端 AI Agent 2026 现代化差距分析与初步完善方案

## Why

encv-mobile 的 AI 助手后端 (`/workspace/agent/`) 与 Vue 渲染壳 (`/workspace/app/encv-mobile/src/`) 截至 2026-06 已经把 2025 年的「Chat + Tool Call + Approval 4 决策 + SSE 断点续传 + 虚拟列表」闭环跑通（参见已完成的 `.trae/specs/go-in-process-agent/checklist.md` 与 `.trae/specs/codex-web-gap-analysis/` 的 Phase A-G 实施分片）。

但 2026 年的 AI agent 主流规范（参考 `earendil-works/pi` 与 `shopkeeper2020/codex_web` 两个仓库的实际代码）已经演进到：
- **可扩展的 agent harness**（hooks、durable session、compaction、skills、plans、tool policy、provider 路由）
- **多端实时同步**（Web + Desktop + VS Code 共享 thread，owner/follower 模型）
- **编码 IDE 级别的 Composer 体验**（slash 菜单、plan 模式、permission 模式、steer/queue 双轨、图片附件、行内文件引用识别）
- **两级操作分组 + 活跃态归一化**（running / editing / thinking / in_progress 统一收敛）

本 spec 不替代现有 `go-in-process-agent` 与 `codex-web-gap-analysis`（它们记录的是已完成的 2025 闭环），而是把它们推进到 2026 标准的初步完善路线图。

**严禁猜测**：本文所有文件名、函数名、状态名、事件名、URL 路径均来自 `/tmp/ai-agent-research/pi-repo/` 与 `/tmp/ai-agent-research/codex-web-repo/` 的实际代码，以及 `/workspace/` 下已存在的文件。

## What Changes

### A. 借鉴 pi 的 agent harness 能力

| 改动 | 参考来源 | 现状 |
|------|---------|------|
| A1. 在 agent 库新增 **hooks 系统**（`session_start` / `turn_start` / `turn_end` / `pre_tool_call` / `post_tool_call` / `session_shutdown`） | `pi-repo/packages/agent/docs/hooks.md` | ❌ 无 |
| A2. 实现 **durable session**：events 落 JSONL + 进程重启可恢复 | `pi-repo/packages/agent/docs/agent-harness.md`（`durableHarness`） | ⚠️ 内存 cache，重启即丢（见 `go-in-process-agent/checklist.md` 已知限制） |
| A3. 实现 **compaction**：context 超阈值时自动压缩 | `pi-repo/packages/agent/docs/compactions.md` | ❌ 无 |
| A4. 实现 **skills 注册表**：扫描 `~/.encv/skills/*/SKILL.md` | `pi-repo/packages/agent/docs/skills.md`（仿 Claude Code） | ❌ 无 |
| A5. 实现 **plan/todo 工具**（`write_todos`） | `pi-repo/packages/agent/docs/plans.md` | ❌ 无 |
| A6. 实现 **system prompt per-session override** | `pi-repo/packages/agent/docs/system-prompts.md` | ⚠️ 仅有全局默认（`agent_settings.system_prompt`） |
| A7. 实现 **tool policy**（`readonly` / `write` / `all`）按 session 限制工具 | `pi-repo/packages/agent/docs/tools-policy.md` | ⚠️ 仅有 `enabled_tools` 白名单 |
| A8. 拆分 **ai provider 抽象层**（OpenAI / Anthropic / Gemini） | `pi-repo/packages/ai/`（`packages/ai/README.md`、`packages/ai/anthropic.ts`、`packages/ai/openai.ts`） | ❌ 仅 OpenAI |

### B. 借鉴 codex_web 的多端实时同步模型

| 改动 | 参考来源 | 现状 |
|------|---------|------|
| B1. 引入 **`appServerRealtimeReducer`** 模式：前端 reducer 统一处理实时事件 | `codex-web-repo/apps/web/src/app/appServerRealtimeReducer.ts`（已克隆确认存在） | ❌ 无 reducer，前端直接消费 SSE |
| B2. 引入 **`realtimeState.ts`** helper：`readRealtimeThreadId` / `readRealtimeCacheVersion` / `readRealtimeServerInstance` / 序列号去重 | `codex-web-repo/apps/web/src/app/realtimeState.ts` | ❌ 无 |
| B3. 后端新增 **server instance id** 概念（`/api/health` 返回 `serverInstanceId`） | `codex-web-repo/apps/web/src/app/realtimeState.ts:77-86`（`readRealtimeServerInstance`） | ❌ 无 |
| B4. 前端 store 加 **sequence 去重**（`MAX_TRACKED_REALTIME_SEQUENCES = 2_000`） | `codex-web-repo/apps/web/src/app/realtimeState.ts:27` | ❌ 无 |

> **注**：encv-mobile 不需要像 codex_web 那样三端共享（没有官方 Desktop），但 reducer 模式 + server instance 去重仍可借鉴用于多 tab / 多设备场景。

### C. 借鉴 codex_web 的 Composer 增强

| 改动 | 参考来源 | 现状 |
|------|---------|------|
| C1. **slash 菜单**：输入 `/` 开头弹命令面板（功能 + skills 分组） | `codex-web-repo/apps/web/src/app/components/Composer.tsx:62-73`（`SlashMenuItem`） + `docs/implementation_status.md` 2026-05-31 段 | ❌ 无 |
| C2. **plan mode toggle**（Composer 底栏） | `codex-web-repo/apps/web/src/app/components/Composer.tsx` + `docs/implementation_status.md` 2026-05-31 段（"目标"模式） | ❌ 无 |
| C3. **permission mode switcher**（default / auto-review / full-access） | `codex-web-repo/apps/web/src/app/components/Composer.tsx`（`PermissionMode` 类型）+ `apps/server/src/turnRoutes.ts` 文档 | ⚠️ 仅有 backend flag，没有 UI 切换 |
| C4. **steer / queue 双轨**（active turn 时：引导当前 vs 排队下一条） | `codex-web-repo/apps/web/src/app/components/Composer.tsx:55`（`SendOptions.mode: "start" \| "steer"`） | ❌ 无，streaming 时直接 disable input |
| C5. **image attachment**：图片缩略图行 + 普通文件卡片，位于 textarea 上方 | `codex-web-repo/apps/web/src/app/components/Composer.tsx:75-78`（`ComposerDraft` 含 `attachments`） + `docs/implementation_status.md` 2026-05-30 段 | ❌ 无 |
| C6. **attachment 在 steer 路径自动转 queue** | `codex-web-repo/docs/implementation_status.md` 2026-05-30 段 | ❌ 无 |
| C7. **default reasoning 显示为用户语言**（"xhigh" → "超高"） | `codex-web-repo/docs/implementation_status.md` 2026-05-30 段 | ❌ 无 |

### D. 借鉴 codex_web 的消息渲染增强

| 改动 | 参考来源 | 现状 |
|------|---------|------|
| D1. **两级操作分组**（外层"已运行/正在运行 N 条命令"摘要 + 内层单条详情） | `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx`（`GroupedOperationItem`、`USER_MESSAGE_COLLAPSE_LINE_COUNT = 9` 等常量在 66-69 行） | ⚠️ `GroupedOperationMessage.vue` 已有（154 行），但目前是单层，需要 review 是否需要拆双层 |
| D2. **活跃态归一化**：`active / running / editing / thinking / in_progress` 全部走 active 分支 | `codex-web-repo/apps/web/src/app/appServerRealtimeReducer.ts:61-65`（`isActiveStatus`） + 78-89（`readTurnStatus`） | ⚠️ 后端只推 4 态，前端需补归一化函数 |
| D3. **context compaction** 指示：完成态显示不可展开的"上下文已自动压缩"分隔线 | `codex-web-repo/docs/implementation_status.md` 2026-05-30 段 + 2026-05-31 段 | ❌ 无 |
| D4. **行内文件引用识别**：识别 Markdown 链接 / 绝对路径 / 相对路径 / 文件名，渲染为 chip，点击打开轻量菜单（复制路径 / 在真实右侧栏打开） | `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:71-81`（`INLINE_FILE_REFERENCE_PATTERN`） + 2026-05-31 段 | ❌ 无 |
| D5. **user message 折叠阈值 9 行 / 560 字符**（已有，需核对） | `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:66-67` | ✅ `UserMessageBubble.vue` 已有（`go-in-process-agent/checklist.md` 已勾选） |
| D6. **agent task 折叠阈值 7 行 / 520 字符** | `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:68-69` | ❌ 无 agent task 消息块 |
| D7. **file change 两级折叠**（外层摘要 + 内层 diff） | `codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:70`（`FILE_CHANGE_INITIAL_ROW_COUNT = 3`） + 2026-05-31 段 | ⚠️ `FileChangeSummaryMessage.vue` 已有（179 行），需 review 是否需要两级 |
| D8. **滚动到底部按钮** + 选区保持时暂停自动滚动 | `codex-web-repo/docs/implementation_status.md` 2026-05-30 / 2026-05-31 段 | ❌ 无（`AgentChat.vue` 只有 onMainScroll） |
| D9. **实时事件合并到短 debounce 窗口**（避免 React 列表重建刷掉选区） | `codex-web-repo/docs/implementation_status.md` 2026-05-31 段 | ❌ 无 |
| D10. **side conversation / fork**：从主 thread 派生侧边聊天 | `codex-web-repo/apps/web/src/app/components/Composer.tsx` + `docs/implementation_status.md` 2026-05-30 / 2026-05-31 段 | ❌ 无 |

### E. 借鉴 codex_web 的 Diagnostics / 运维能力

| 改动 | 参考来源 | 现状 |
|------|---------|------|
| E1. **sync:doctor 脱敏诊断** 入口（CLI + 移动端 Settings 面板） | `codex-web-repo/apps/server/src/syncDoctor.ts` + `docs/implementation_status.md` 2026-05-30 段 | ❌ 无 |
| E2. **LAN access 地址枚举**（Settings → Network 显示可复制 URL） | `codex-web-repo/apps/server/src/lanAccess.ts` + 2026-05-30 段 | ❌ 无（仅 dev gateway 走 443） |

> E1/E2 优先级低，仅作为运维增强，不阻塞 P0。

### F. 借鉴 pi 的可观测性

| 改动 | 参考来源 | 现状 |
|------|---------|------|
| F1. **events.jsonl** 落盘 + 配套的 TUI trace 工具 | `pi-repo/packages/agent/docs/agent-harness.md`（"durableHarness"） | ❌ 无 |
| F2. **replay** 命令（`pi --replay events.jsonl`） | `pi-repo/packages/coding-agent/src/cli.ts` | ❌ 无 |

## Impact

### 受影响 specs
- `go-in-process-agent/checklist.md`（**保持现状**——已 100% 完成的 2025 闭环，本 spec 不重写）
- `codex-web-gap-analysis/checklist.md`（**保持现状**——2025 闭环的实施分片）
- `implement-mobile-backend-api/`（envoy-go / sqlite / gomobile 相关，不在本 spec 范围）
- **新增** `mobile-agent-2026-gap-analysis/`（本文档）

### 受影响 code
**后端 Go 库**（`/workspace/agent/`）：
- `agent/agent.go`（715 行）—— 加 hook 回调、compaction 触发、session 持久化
- `agent/types.go`（212 行）—— 加 `HookEvent` / `Skill` / `PlanStep` / `ToolPolicy` 等类型
- `agent/registry.go`（115 行）—— 加 tool policy enforcement
- `agent/openai.go`（174 行）—— 拆出 ai provider 抽象
- **新增** `agent/hooks.go` / `agent/skills.go` / `agent/compaction.go` / `agent/session_store.go` / `agent/ai/`（provider 子包）
- `agent/http.go`（319 行）—— 加 `/api/server/instance`、`/api/sync/doctor`、`/api/network/lan-access`

**前端 encv-mobile**（`/workspace/app/encv-mobile/src/`）：
- `composables/useAgent.ts`（1057 行）—— 加 server instance / sequence 去重 / steer+queue / attach / slash 菜单事件
- `views/AgentChat.vue`（1048 行）—— 加 slash 菜单、permission 切换、滚动到底部按钮、debounce 实时事件、attach 行
- `components/agent/GroupedOperationMessage.vue`（154 行）—— review 是否升级为两级
- `components/agent/FileChangeSummaryMessage.vue`（179 行）—— review 是否升级为两级
- **新增** `components/agent/SlashMenu.vue` / `PlanToggle.vue` / `PermissionModeSwitcher.vue` / `AttachmentTray.vue` / `FileReferenceChip.vue` / `ScrollToBottomButton.vue` / `ContextCompactionDivider.vue`
- **新增** `composables/appServerRealtimeReducer.ts` / `composables/serverInstanceTracker.ts` / `composables/inlineFileReference.ts`
- `i18n/agent.ts`（80+ 行）—— 扩 50+ key

**与现有架构铁律不冲突**：
- 入口仍走首页浮动按钮（`AgentEntry.vue`）+ modalController.create（`capacitor.md` 铁律 §1）
- 不加路由
- 不破坏 OpenList 集成（`go-in-process-agent/checklist.md` 已确立的 8 个 `/api/ext/*` 端点契约）

## ADDED Requirements

### Capability 1: Hooks 系统

**Requirement: agent 库应支持 hooks**

The system SHALL 提供 6 个 hook 事件点，允许外部代码注入：
- `session_start` —— session 创建时
- `turn_start` —— LLM 调用前
- `turn_end` —— LLM 调用后
- `pre_tool_call` —— 工具执行前
- `post_tool_call` —— 工具执行后
- `session_shutdown` —— session 销毁时

#### Scenario: 注入 system prompt
- **WHEN** 用户在 Settings 配置了 `agent_settings.system_prompt`
- **THEN** `session_start` hook 应自动注入该 prompt 到 messages 头部
- **AND** 现有 `agent_settings.system_prompt` 行为保持不变（向后兼容）

#### Scenario: 审计工具调用
- **WHEN** AI 调用任何 tool
- **THEN** `pre_tool_call` 和 `post_tool_call` hook 被触发
- **AND** 外部可注册 logger 审计 args + result

#### Scenario: 阻断危险工具
- **WHEN** `pre_tool_call` hook 返回 `cancel`
- **THEN** tool 不执行，返回 `cancelled` 状态给 LLM

### Capability 2: Durable Session

**Requirement: agent 库应支持 session 持久化**

The system SHALL 把 session 的所有 event 落 JSONL 到 `~/.encv/agent/sessions/{sessionId}.jsonl`。
The system SHALL 在 session cache 命中磁盘文件时自动 load。
The system SHALL 提供 `agent.Resume(sessionID)` 接受 offset 参数从 JSONL 重放。

#### Scenario: 进程重启不丢 session
- **WHEN** agent 进程崩溃后重启
- **AND** client 调用 `Resume(sessionID, 0)`
- **THEN** 从 JSONL 头部重放所有 event
- **AND** 状态与崩溃前一致

#### Scenario: 磁盘写入失败不阻塞
- **WHEN** JSONL 写入失败（磁盘满 / 权限拒绝）
- **THEN** 内存 cache 仍生效
- **AND** 记录 warning 到 stderr，不 panic

### Capability 3: Compaction

**Requirement: agent 库应支持 context 自动压缩**

The system SHALL 监控 messages 总 token 数。
The system SHALL 在 token 数超过阈值的 80% 时触发 LLM summary 压缩历史消息。

#### Scenario: 长对话自动压缩
- **WHEN** messages 总 token > 80% of model context window
- **THEN** 后台异步调 LLM 压缩老 messages
- **AND** 前端收到 `compaction` 事件
- **AND** `MessageBlocks` 渲染不可展开的"上下文已自动压缩"分隔线

### Capability 4: Skills 注册表

**Requirement: agent 库应扫描并加载 skills**

The system SHALL 启动时扫描 `~/.encv/skills/*/SKILL.md`。
The system SHALL 把每个 SKILL.md 解析为 `Skill{Name, Description, Prompt}`。
The system SHALL 在 `GetAllSchemas` 中不暴露 skill（skill 是 prompt 注入，不是 tool）。

#### Scenario: 用户在 slash 菜单选 skill
- **WHEN** Composer 显示 `/skills` 菜单
- **AND** 用户选择 `video-encrypt`
- **THEN** 该 skill 的 prompt 注入到 `turn_start` hook 注入的 system prompt 尾部
- **AND** 不调用 LLM tool call（skill 是 prompt，不是 function）

### Capability 5: Plan / Todo 工具

**Requirement: agent 库应内置 `write_todos` 工具**

The system SHALL 内置 `write_todos(todos: [{id, status, content}])` 工具。
The system SHALL 把 todos 推 `EventToolStatus` + `EventToolResult`。
The system SHALL 在前端渲染独立的 plan block（不与 operationGroup 合并）。

#### Scenario: AI 拆解多步任务
- **WHEN** 用户要求"先列文件，再删除 foo.txt，再生成 .encv"
- **THEN** AI 调用 `write_todos` 拆成 3 步
- **AND** 每完成一步，更新 todo status
- **AND** 前端显示 plan 进度条

### Capability 6: Provider 抽象层

**Requirement: ai provider 应可插拔**

The system SHALL 把 `agent/openai.go` 拆为 `agent/ai/openai.go` + `agent/ai/anthropic.go` + `agent/ai/gemini.go`。
The system SHALL 定义统一 `Provider` interface：`StreamChat(messages, tools) <-chan Delta`。
The system SHALL 根据 `agent_settings.provider` 字段路由到对应实现。

#### Scenario: 用户切换到 Anthropic
- **WHEN** Settings 中 `provider` 改为 `anthropic` + 填入 `anthropic_api_key`
- **THEN** `/api/chat` 调用 Anthropic Messages API
- **AND** SSE 事件类型保持 6 种不变（与 OpenAI 相同）

### Capability 7: Slash 菜单

**Requirement: Composer 应支持 `/` 命令面板**

The system SHALL 在 `<textarea>` 内容以 `/` 开头且无其他字符时打开 slash 菜单。
The system SHALL 菜单分组为"功能"和"技能"两类。
The system SHALL 菜单项通过 `apply()` 回调触发动作（如选 skill、上传文件、切模式）。

#### Scenario: 用户在 Composer 输入 /
- **WHEN** Composer 为空，输入 `/`
- **THEN** 弹出 slash 菜单浮层在 Composer 上方
- **AND** 菜单列出所有 enabled skills + 所有功能项
- **AND** 选中项按 Enter 应用，菜单关闭

### Capability 8: Steer / Queue 双轨

**Requirement: active turn 时 Composer 应支持双发送模式**

The system SHALL 在 `status='streaming'` 时显示两个发送按钮：
- "引导当前"（steer）—— 调 `/api/chat` 的 `mode: "steer"` 字段
- "排队下一条"（queue）—— 调 `/api/chat` 后等待 stream_end 再发

The system SHALL 切换时 input 框不清空。

#### Scenario: 用户在 AI 思考中追加消息
- **WHEN** 状态为 streaming，输入"补充：使用 mp4 容器"
- **AND** 点击"引导当前"
- **THEN** AI 当前 turn 立即收到该 steer
- **AND** input 清空

#### Scenario: 用户在 AI 思考中排队消息
- **WHEN** 状态为 streaming，输入"接下来解密 .encv"
- **AND** 点击"排队下一条"
- **THEN** 消息以 placeholder 形式出现在 Composer 上方
- **AND** 当前 turn 结束后自动发送

### Capability 9: Attach 图片

**Requirement: Composer 应支持图片 + 文件附件**

The system SHALL Composer 底栏加 `+` 按钮触发文件选择器。
The system SHALL 区分 image（缩略图行）和 file（卡片行），均显示在 textarea 上方。
The system SHALL 发送时把 attachments 编码为 base64，附在 `messages[-1].content` 数组中。

#### Scenario: 用户附 1 张图片 + 1 个 .mp4
- **WHEN** Composer 拖入 `screenshot.png` + 点击 + 选 `video.mp4`
- **THEN** textarea 上方显示 1 个图片缩略图 + 1 个 .mp4 卡片
- **AND** 发送时 LLM 收到 `[image, text, file]` 3 input
- **AND** 附件从客户端清空

### Capability 10: 行内文件引用

**Requirement: MessageBlocks 应识别行内文件引用**

The system SHALL 解析消息正文中的 `path:line:col` 模式（参考 `INLINE_FILE_REFERENCE_PATTERN`）。
The system SHALL 把识别到的 path 渲染为可点击 chip（`FileReferenceChip.vue`）。
The system SHALL 点击 chip 弹出轻量菜单：复制路径 / 在 Files tab 打开。

#### Scenario: AI 回复包含 `src/main.go:42`
- **WHEN** assistant 消息含 `src/main.go:42`
- **THEN** 该片段渲染为蓝色 chip
- **AND** 点击 chip 弹菜单
- **AND** 选"在 Files tab 打开"→ 切到 Files tab 并高亮该文件

### Capability 11: Server Instance + Sequence 去重

**Requirement: 前端应跟踪 server instance 与 sequence 防重**

The system SHALL `useAgent.ts` 启动时调 `/api/health` 取得 `serverInstanceId`。
The system SHALL 维护 `Set<number>` seenSequences，最大 2000 条。
The system SHALL 收到 sequence 已见过的 event 时丢弃。

#### Scenario: server 重启后避免重复
- **WHEN** agent 进程重启，serverInstanceId 变化
- **THEN** seenSequences 集合清空
- **AND** 重新从 Resume 拉取

### Capability 12: Sync Doctor（运维）

**Requirement: 移动端 Settings 应提供 sync:doctor 入口**

The system SHALL Settings → 诊断 → 加 "运行 sync 诊断" 按钮。
The system SHALL POST `/api/sync/doctor` 启动脱敏诊断。
The system SHALL 展示结果（脱敏 JSON），可复制完整 JSON。

> 优先级 P2，仅运维增强。

## MODIFIED Requirements

### Requirement: useAgent 事件分发（已有）
**现状**：`useAgent.ts` 处理 6 种 SSE event（text_delta / reasoning_delta / tool_call / tool_status / tool_result / stream_end）。
**改后**：增加 `compaction` event；`tool_call` 内部区分 `plan` kind（`write_todos`）；`tool_status` 增加 `running` / `editing` / `thinking` / `in_progress` 子状态。

### Requirement: ApprovalCard 4 决策（已有）
**现状**：批准 / 本轮批准 / 拒绝 / 拒绝并停止。
**改后**：在卡片头部加 `running` / `editing` 状态徽标；按钮在 pre_tool_call 触发时显示 spinner。

### Requirement: AgentChat 顶层视图（已有）
**现状**：渲染 messages + 输入框。
**改后**：增加 slash 菜单 / permission mode 切换 / steer/queue 双轨 / 滚动到底部按钮 / 实时事件 debounce。

## REMOVED Requirements

无（本 spec 是 additive，不移除现有功能）。

## 实施分片

| 阶段 | 范围 | 估时（参考） | 价值密度 |
|------|------|-------------|---------|
| **P0** | A1 hooks + A2 durable session + B1 reducer + B3 server instance + D1 两级操作分组 + D2 活跃态归一化 | ~3 周 | ⭐⭐⭐⭐⭐ |
| **P1** | A3 compaction + A4 skills + A5 plan tool + C1 slash 菜单 + C4 steer/queue + C5 attach + D3 compaction 指示 + D4 文件引用 + D8 滚动到底部 | ~4 周 | ⭐⭐⭐⭐ |
| **P2** | A6 system prompt per-session + A7 tool policy + A8 provider 抽象 + C2 plan mode + C3 permission switcher + C6 steer→queue 自动转 + C7 i18n reasoning + D5/D6/D7 渲染细化 + D9 debounce + D10 side conversation + F1 events.jsonl + E1 sync doctor + E2 LAN access | ~6 周 | ⭐⭐⭐ |

> **不估时**——只列范围与依赖。具体排期进入 `tasks.md` 后再细化。

## 验收门槛

- Go 单测覆盖率 ≥ 70%（参考 `go-in-process-agent/checklist.md`）
- Vue 单测覆盖率 ≥ 70%
- 0 console error
- 三进程（agent + OpenList + encv-mobile）端到端跑通
- 浏览器实测：移动端主页 → 浮动按钮 → slash 菜单 → 选 skill → 输入 → steer/queue → 批准 → 工具执行结果展示
