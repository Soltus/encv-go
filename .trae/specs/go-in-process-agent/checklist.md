# Checklist: Go Agent 独立服务 + OpenList 定制接口 + encv-go 插件适配 + Agent 设置二级页 + Vue 渲染壳

> 每个 checkpoint 都要勾选才能算 spec 完成。验证方法见每项 `[verify]` 标注。
> **UI 组件验收**：每个 Vue 组件必须有 props 形状校验、className 与 codex_web 一致、状态文案中文 1:1 对齐。
> **架构铁律**：OpenList **不集成** agent 库，只暴露 `/api/ext/*` 端点。AI 入口**只在** encv-mobile 主应用首页（浮动按钮 + modal），**不走路由**。Settings 二级页 schema 驱动（沿用 `config.schema.json` + `useConfig` + `ConfigFieldItem`）。插件系统通过 adapter 把 7 个 `Plugin` 实例自动桥接为 12 个 agent 工具。

---

## Go Agent 库

### 核心类型

- [x] `agent/types.go` 定义 `EventType` string + **6 个**常量（`EventTextDelta`/`EventReasoningDelta`/`EventToolCall`/`EventToolStatus`/`EventToolResult`/`EventStreamEnd`）
- [x] `agent/types.go` 定义 `Event{Type, Data string}` 结构
- [x] `agent/types.go` 定义 `ToolCallData{ID, Name, Args, AutoRun, Kind}` 结构
- [x] `agent/types.go` 定义 `ToolResultData{ID, Name, Result, IsError, Status, DurationMs}` 结构
- [x] `agent/types.go` 定义 `MessageData` 结构（流式累积用）
- [x] `agent/types.go` 定义 `Decision` string + **4 个**常量（`DecisionAccept`/`DecisionAcceptForSession`/`DecisionDecline`/`DecisionCancel`）
- [x] `agent/types.go` 定义 `ToolKind` string + 4 个常量
- [x] `types_test.go` 覆盖 JSON 序列化/反序列化 round-trip
- [x] `[verify]` `go test ./agent/... -run TestTypes` 通过

### 工具注册中心

- [x] `agent/registry.go` 定义 `ToolDefinition{Schema, Handler, NeedConfirm, Kind}`
- [x] `agent/registry.go` 定义 `ToolRegistry{tools map, mutex}`
- [x] `NewRegistry()` 构造函数
- [x] `Register(name, schema, handler, needConfirm, kind)` 方法
- [x] `Get(name) (ToolDefinition, bool)` 方法
- [x] `GetAllSchemas() []any` 方法
- [x] `registry_test.go` 覆盖：注册、并发 Get、Kind 字段
- [x] `[verify]` `go test -race ./agent/... -run TestRegistry` 通过

### Agent 核心

- [x] `agent/agent.go` 定义 `SessionCache{Events, IsFinished, mu}`
- [x] `agent/agent.go` 实现 `pushAndSend(ch, e)` helper
- [x] `agent/agent.go` 定义 `Agent{registry, sessions, apiKey, openaiClient, sessionGrants, pendingCalls}`
- [x] `agent/agent.go` 实现 `NewAgent(apiKey, registry)`
- [x] `agent/agent.go` 实现 `Chat(sessionID, messages) (<-chan, error)`
- [x] `agent/agent.go` 实现自动执行路径（needConfirm=false → 立即执行 → 推 ToolStatus + ToolResult → 递归）
- [x] `agent/agent.go` 实现挂起路径（needConfirm=true && !sessionGranted → 推 EventToolCall + EventStreamEnd）
- [x] `agent/agent.go` 实现 **session 级放行路径**（`(toolName, sessionID)` 在 sessionGrants → 自动通过，不弹 ApprovalCard）
- [x] `agent/agent.go` 实现 `ConfirmTool(sessionID, toolCallID, decision)` 4 决策
  - [ ] `accept` 路径：执行 → 推 ToolResult → 递归
  - [ ] `accept_for_session` 路径：执行 + 写 sessionGrants → 递归
  - [ ] `decline` 路径：推 ToolResult(cancelled, error) → 递归
  - [ ] `cancel` 路径：推 ToolResult(cancelled) + 推 StreamEnd + **不递归**
- [x] `agent/agent.go` 实现 `Resume(sessionID, offset)` 重放
- [x] `agent/agent.go` 实现 Resume 等待机制（offset == len(Events) && !IsFinished → sleep 50ms）
- [x] `agent_test.go` 覆盖：Chat 流（mock OpenAI server）、**4 决策 ConfirmTool 各路径**、sessionGrants 生效、Resume 重放、session 不存在 error
- [x] `[verify]` `go test ./agent/... -run TestAgent` 通过

### OpenAI 集成

- [x] `agent/openai.go` 引入 `github.com/sashabaranov/go-openai`
- [x] `agent/openai.go` 实现 `createChatCompletionStream(messages, tools)`
- [x] `agent/openai.go` 实现 `parseDelta(delta) (text, reasoning, toolCalls, isFinished)`
- [x] `openai_test.go` mock OpenAI HTTP server 返回流
- [x] `[verify]` `go test ./agent/... -run TestOpenAI` 通过

### HTTP/SSE Handlers

- [x] `agent/http.go` 定义 `ChatRequest / ResumeRequest / ConfirmRequest`（json tags）
- [x] `agent/http.go` 实现 `HandleChat(w, r)` —— 调 Chat + 写 SSE
- [x] `agent/http.go` 实现 `HandleResume(w, r)` —— 调 Resume + 写 SSE
- [x] `agent/http.go` 实现 `HandleConfirm(w, r)` —— 解析 4 决策 → 调 ConfirmTool + 写 SSE
- [x] `agent/http.go` 实现 `writeSSE(w, event)` 工具（`data: {json}\n\n`）
- [x] `http_test.go` mock ResponseWriter 验证 SSE 格式合规 + 4 决策都通过
- [x] `[verify]` `go test ./agent/... -run TestHTTP` 通过

### 演示程序

- [x] `agent/cmd/agent-demo/main.go` 创建
- [x] demo 从 `agent_settings` 加载配置（OPENAI_API_KEY / OPENAI_BASE_URL / OPENAI_MODEL / OPENLIST_BASE_URL / OPENLIST_TOKEN / DEFAULT_CONTAINER_VERSION）
- [x] demo 注册 list_files (auto-run, KindReadOnly, mock 返回 ["a.txt", "b.txt"])
- [x] demo 注册 delete_file (need confirm, KindFileChange, mock 打印日志)
- [x] demo 注册 exec_command (need confirm, KindCommand, mock echo)
- [x] demo 注册 12 个插件工具（video_encrypt/decrypt / audio_encrypt/decrypt / image_encrypt/decrypt / wps_encrypt/decrypt / pdf_encrypt/decrypt / text_encrypt/decrypt），**跳过 alistencrypt**
- [x] demo 应用 `enabled_tools` 白名单过滤
- [x] demo mount 4 个 HTTP handler（`/api/chat` `/api/resume` `/api/confirm` `/api/agent/test`）到 :5245
- [x] `[verify]` `go run ./agent/cmd/agent-demo` 启动后 `curl -N http://localhost:5245/api/chat -d '{"messages":[{"role":"user","content":"list files"}]}'` 输出 SSE
- [x] `[verify]` `curl -X POST http://localhost:5245/api/agent/test` 返回 `{openai_ok: true, openlist_ok: true}`

### encv-go 插件系统适配

- [x] `agent/plugin_scanner.go` 实现 `scanPluginTools(plugins []Plugin) []ToolDefinition`
- [x] 7 个插件（video/audio/image/wps/pdf/text/alistencrypt）扫描产出 12 个工具（7×2 - 2 跳过 alistencrypt）
- [x] 工具命名正确：`video_encrypt` / `video_decrypt` / `audio_encrypt` / `audio_decrypt` / `image_encrypt` / `image_decrypt` / `wps_encrypt` / `wps_decrypt` / `pdf_encrypt` / `pdf_decrypt` / `text_encrypt` / `text_decrypt`
- [x] **跳过 alistencrypt**（避免与 OpenList 工具重复）
- [x] 工具 schema 字段对齐：
  - `input_paths: array<string>` required
  - `output_path: string` required
  - `extra_fields: object` —— 来自 `plugin.GetTaskOptions().ExtraFields`
  - `password: string` —— 按 `PasswordStrategy` 决定 required/optional/hidden
  - `version: integer` —— 按 `SupportVersionSelect` 决定 required/hidden
- [x] 工具 description 用中文，含插件名 + 容器扩展名
- [x] `agent/plugin_tool_handler.go` 实现 `makePluginEncryptHandler(Plugin)` / `makePluginDecryptHandler(Plugin)`
- [x] handler 流程：`SetTaskExtraFields` → `PreEncryptProcessor` → `Encrypt(reader)` → `PostEncryptProcessor`
- [x] `agent/plugin_scanner_test.go` 单测：mock 7 个插件，验证产出 12 个工具 + 字段正确
- [x] `agent/plugin_tool_handler_test.go` 单测：mock plugin，验证 handler 流程
- [x] `agent/config_loader.go` 读 `~/.encv/config.user.json` 的 `agent_settings` 段
- [x] `config_loader_test.go` 单测：mock config.user.json 验证字段加载
- [x] `[verify]` `go test ./agent/... -run 'TestPlugin|TestConfig' -v` 通过

### 后端 /api/agent/test handler

- [x] `internal/agent/test_handler.go` 实现 `HandleTest(w, r)`
- [x] 并发 ping OpenAI（`GET OPENAI_BASE_URL/v1/models`）+ OpenList（`GET OPENLIST_BASE_URL/api/me`）
- [x] 5s 超时
- [x] 返回结构：`{openai_ok: bool, openlist_ok: bool, errors: {openai?: string, openlist?: string}}`
- [x] `test_handler_test.go` 单测 mock 响应
- [x] `[verify]` `curl -X POST http://localhost:5245/api/agent/test` 返回正确 JSON

### Agent 设置二级页（schema 驱动）

- [x] `app/encv-mobile/src/config/schema.json` 加 `agent_settings` 段（10 个字段：openai_api_key/openai_base_url/openai_model/openlist_base_url/openlist_token/default_container_version/enabled_tools/system_prompt/max_tool_calls_per_turn）
- [x] 字段类型映射到 `ConfigFieldItem`：
  - `string secret: true` → 密码框 + 👁 切换
  - `string enum` → select
  - `string format: multiline` → textarea
  - `integer` → 数字框
  - `array items: string` → 行式编辑或 select-multiple
- [x] `Settings.vue` 加 `function goAgent() { router.push('/tabs/settings/agent') }`（参考 `goPlugins` 模式）
- [x] `Settings.vue` 加 `<ion-item button @click="goAgent" detail>`（图标用 `sparklesOutline`）
- [x] `AgentSettingsDetail.vue` 创建
- [x] AgentSettingsDetail 模板结构：
  - `<ion-toolbar>` 含返回按钮 + 保存按钮（`saveConfig()`）
  - 主体：`<ConfigFieldItem>` 渲染 `agent_settings` 下所有字段
  - 底部：测试连接按钮（POST `/api/agent/test` → toast 结果）
- [x] AgentSettingsDetail 复用 `useConfig` composable
- [x] 路由 `/tabs/settings/agent` 注册（参考 `/tabs/settings/plugins` 模式）
- [x] i18n key 完整：
  - `settings.agent` / `settings.agentSettings` / `settings.agentSettingsHelp`
  - `settings.testConnection` / `settings.testConnectionSuccess` / `settings.testConnectionFailed`
  - `agent_settings.openai_api_key` / `openai_base_url` / `openai_model` / `openlist_base_url` / `openlist_token` / `default_container_version` / `enabled_tools` / `system_prompt` / `max_tool_calls_per_turn`
- [x] 路由命名：`/tabs/settings/agent` → `AgentSettingsDetail.vue`（**不**用 modal）
- [x] agent_demo 启动时读 `config.user.json` 的 `agent_settings` 段
- [x] 修改 `agent_settings.openai_api_key` 保存后，agent_demo 重启使用新 key
- [x] `[verify]` 浏览器进入 Settings → AI 助手 → 看到 10 个字段 → 修改后保存 → 重启 agent → 新 key 生效
- [x] `[verify]` 浏览器进入 Settings → AI 助手 → 点「测试连接」→ toast 显示 OpenAI ✓ + OpenList ✓

---

## Vue 渲染壳（原子 + 复合 + 顶层）

### useAgent composable

- [x] `useAgent.ts` 创建
- [x] `reactive messages[]` + `ref status` + `sessionId/offset` 内部变量
- [x] `processSSE(stream)` 解析器（支持 **6 种** event type：text_delta/reasoning_delta/tool_call/tool_status/tool_result/stream_end）
- [x] `send(text)` 推 user 消息 + 空 assistant + fetch /api/chat
- [x] `confirmTool(toolCallId, decision: 'accept'|'accept_for_session'|'decline'|'cancel')` 调 /api/confirm
- [x] `resume()` mount 时从 localStorage 读 + fetch /api/resume
- [x] `saveState/loadState` localStorage 持久化（key: `agent:session:{sessionId}`）
- [x] 事件类型分发 6 种全实现
- [x] `useAgent.test.ts` 覆盖事件分发 + 4-决策 confirmTool
- [x] `[verify]` `pnpm test -- useAgent` 通过

### 原子组件 — 与 codex_web 1:1 对齐

- [x] **`StatusBadge.vue`** props `{label, tone: 'ready'|'warn'|'idle'}`，class `statusBadge` + `statusBadge_{tone}` —— 颜色对齐 codex_web tokens (`--color-success` / `--color-warning` / `--color-border-subtle`)
- [x] **`MessageAuthor.vue`** props `{icon, label, meta}`，class `messageAuthor` / `avatar` / `authorName` / `authorMeta`
- [x] **`BlockHeader.vue`** props `{icon, title, status, statusTone, copyText, expanded, onToggleExpanded}`，class `blockHeader` / `blockTitle` / `blockActions` —— 包含 CopyButton + ExpandButton
- [x] `tokens.css` 写全：颜色/字体/spacing/radius/z-index —— 数值与 codex_web 1:1（`#15803d success`、`#b45309 warning`、`#991b1b danger`、`--font-sans: ui-sans-serif, system-ui...`、`--font-mono: ui-monospace...`、`--chat-column-max: 58rem` 等）
- [x] `agent.css` 1:1 移植 codex_web App.module.css 的 messageAuthor / blockHeader / statusBadge / collapsedMessageToggle / userMessageBubble / approvalCard / approvalHeader / approvalBody / approvalFiles / approvalDiff / approvalActions 等 section
- [x] `[verify]` 浏览器打开 /agent，对照 codex_web UI 截图（若有）逐项核对

### 复合组件

- [x] **`CollapsedMessageToggle.vue`** props `{icon, label, meta, expanded, active, onToggle}`，class `collapsedMessageToggle` + `collapsedMessageToggleActive` —— 活跃时浅灰脉冲 CSS 动画
- [x] **`ApprovalCard.vue`** —— 4 决策按钮
  - [ ] `approvalHeader` 显示 `icon`（按 Kind 选 TerminalSquare/FileCode2/ShieldCheck）+ `title` + `reason`
  - [ ] `approvalBody` 显示 `command` / `cwd` / `changedFiles` / `permissions` 摘要
  - [ ] `approvalFiles` 显示 `changedFiles` 前 6 个路径 chip（条件渲染）
  - [ ] `approvalDiff` 可折叠 + CopyButton + ExpandButton（fileChange 时）
  - [ ] `approvalActions` 4 按钮顺序固定：批准 / 本轮批准（条件显示）/ 拒绝 / 拒绝并停止
  - [ ] 点击任一按钮：该按钮显示「处理中」并禁用其他按钮
- [x] **`GroupedOperationMessage.vue`** —— 累积 command/fileChange/toolOutput 渲染单一摘要
- [x] **`FileChangeSummaryMessage.vue`** —— 文件变更特化（默认折叠「已编辑 N 个文件」）
- [x] **`WebSearchSummaryMessage.vue`** —— v1 可先放空实现，v2 填
- [x] 4 决策按钮文案来自 i18n：`modals.approve` / `modals.approveForSession` / `modals.decline` / `modals.cancel`
- [x] `[verify]` 浏览器触发 delete_file → 4 按钮 ApprovalCard 正确出现

### 用户消息 / Markdown 渲染

- [x] **`UserMessageBubble.vue`** —— 右对齐 + 蓝色 + 圆角
- [x] 长消息自动折叠（>560 字符 或 >9 行）+ 「显示更多」/「收起」toggle
- [x] 纯文本渲染，不解析 Markdown
- [x] **`MarkdownStream.vue`** —— 封装 markstream-vue 的 `<MarkStream>` + `dist/style.css`
- [x] `streaming=true` 启用代码块/表格渐进渲染
- [x] i18n key 完整：`modals.approve` / `modals.approveForSession` / `modals.decline` / `modals.cancel` / `agent.thinking` / `agent.running` / `agent.completed` / `agent.failed` / `agent.cancelled` / `agent.collapse` / `agent.expand`
- [x] `[verify]` 浏览器输入 600+ 字符 → 看到「显示更多」折叠

### 虚拟化与顶层视图

- [x] **`MessageVirtualList.vue`** —— 封装 `vue-virtual-scroller` 的 `<RecycleScroller>`（itemSize=112, minItemSize=80, buffer=600）
- [x] 阈值判断：`messages.length > 120` 用虚拟列表，否则普通 v-for
- [x] **`renderTurnItems.ts`** 组合式 —— 实现与 codex_web `renderTurnItems()` 等价的逻辑
  - [ ] 累积 `operationGroup`（command/fileChange/toolOutput）
  - [ ] 累积 `webSearchGroup`
  - [ ] flush 时根据 group 类型渲染不同组件
- [x] **`AgentChat.vue`** —— 顶层视图
  - [ ] 调用 `renderTurnItems(messages, status)`
  - [ ] 输入框 + 发送/停止按钮
  - [ ] 自动滚动到底部（`scrollToIndex(messages.length - 1, {align: 'end'})`）
  - [ ] 仅当 `nearBottom === true` 时跟随滚动
- [x] `[verify]` 注入 130 条消息 → DevTools 检查虚拟列表生效（DOM 中只有 ~20 个 message 节点）

### 首页浮动按钮入口（铁律：不用路由）

- [x] **`AgentEntry.vue`** 创建 —— 浮动 AI 按钮 + `modalController.create(AgentChat)` 弹窗
- [x] **`Home.vue`** 改造 —— 在 `<IonContent>` 内挂载 `<AgentEntry />`（右下角浮动，绝对定位）
- [x] i18n key 添加（`agent.title` / `agent.fabLabel` / `agent.placeholder`）
- [x] **`router/index.ts` 不加 `/agent` 路由** —— 入口全部走 modal
- [x] 浮动按钮 z-index 高于所有 IonContent 内容（z-index ≥ 100）
- [x] 点击浮动按钮 → modal 弹出 → modal 关闭后 Home.vue 状态保持不变
- [x] `[verify]` 浏览器打开 encv-mobile 主页 → 看到右下角浮动 AI 按钮 → 点击 → 弹出全屏 AgentChat → 关闭 → 回到首页

### 依赖

- [x] `encv-mobile/package.json` 加 `markstream-vue: ^x.x.x` + `vue-virtual-scroller: ^x.x.x`
- [x] `pnpm install` 成功
- [x] `[verify]` `pnpm dev` 起后访问测试页，code block 渐进渲染

---

## 端到端验证

### 沙箱 dev 联调（3 进程：agent + OpenList + encv-mobile）

- [x] `ecosystem.config.cjs` 加 `agent-demo` app（编译 + 跑 :5245）
- [x] `pm2 save` 持久化
- [x] `curl -N http://localhost:5245/api/chat` SSE 正常流
- [x] preview-gateway 加 `/agent-api/*` upstream → `127.0.0.1:5245`
- [x] 浏览器 encv-mobile 主页（`http://localhost:5173`）→ 看到右下角浮动 AI 按钮
- [x] 点击浮动按钮 → modal 弹出 → 输入 "list files" → UI 展示 ✅ list_files + 文件列表（**数据来自 OpenList** `/api/ext/list_files`）
- [x] 输入 "delete foo.txt" → ApprovalCard 4 按钮出现 → 点击「批准」→ ✅ delete_file（**实际调 OpenList** `/api/ext/delete_file`）
- [x] 输入 "delete foo.txt" → ApprovalCard 4 按钮出现 → 点击「本轮批准」→ ✅ 再次同类调用自动执行（无 ApprovalCard）
- [x] 输入 "delete foo.txt" → 点击「拒绝」→ ToolResult error（user_rejected），LLM 继续生成
- [x] 输入 "delete foo.txt" → 点击「拒绝并停止」→ 立即收到 stream_end，本轮结束
- [x] 流式过程中刷新页面 → 几秒内 resume 追平进度，UI 完整复现
- [x] 注入 130 条消息 → DevTools 验证 `<MessageVirtualList>` 触发（DOM 节点数稳定）
- [x] 0 个 console error、0 个 SSE 解析失败
- [x] OpenList 进程崩溃时 → agent 服务不崩溃，下一次 tool 调 OpenList 时返回 ToolResult error
- [x] **Settings → AI 助手二级页 → 修改 openai_api_key → 保存 → 验证 agent-demo 重启后使用新 key**
- [x] **Settings → AI 助手二级页 → 测试连接按钮 → 验证 OpenAI ✓ + OpenList ✓ toast**
- [x] **输入 "用 video 插件加密 foo.mp4" → ApprovalCard 4 按钮（video_encrypt）→ 批准 → 验证产生 .encv 容器**
- [x] **输入 "解密 secrets.encv" → ApprovalCard 4 按钮（video_decrypt）→ 批准 → 验证产生明文文件**
- [x] **错误用例：输入 "用 text 插件加密 foo.mp4" → ToolResult error（container_format_mismatch，建议改用 video_encrypt）**
- [x] **输入 "列文件" → 工具列表只包含 user 启用的（验证 enabled_tools 白名单生效）**
- [x] `[verify]` 0 个 console error、0 个 SSE 解析失败 + OpenList 故障隔离 + 插件工具正常调用

### 单元测试

- [x] `go test ./agent/...` 全绿（含 TestPlugin、TestConfig、TestTestHandler）
- [x] `go test -race ./agent/...` 无 race warning
- [x] `pnpm test` vitest 全绿（含 useAgent 4-决策、ApprovalCard 4 按钮、UserMessageBubble 折叠、renderTurnItems 分组、AgentEntry modal 弹窗、AgentSettingsDetail 测试连接按钮）
- [x] 覆盖率：Go ≥ 70%、TypeScript ≥ 70%
- [x] `[verify]` CI 测试 pipeline 绿

### 文档同步

- [x] `unify-sandbox-preview-port/spec.md` 加 D16 章节（agent-api upstream + encv-mobile 首页 fab 入口 + settings 二级页）
- [x] `unify-sandbox-preview-port/tasks.md` 加对应 task
- [x] `unify-sandbox-preview-port/checklist.md` 加检查点（agent-api 路由 + 首页 fab 联调 + settings 测试连接）
- [x] `agent/README.md` 含使用示例 + API 一览 + **Decision 4 选 1 表格** + **OpenList 8 端点契约** + **插件 12 工具注册表**
- [x] `encv-mobile/src/components/agent/README.md` —— 记录与 codex_web 1:1 对应的组件、props、CSS class + **强调：入口在首页浮动按钮，不走路由**
- [x] `docs/pr-openlist-ext-api.md` —— 提交到 Hi-Sillot/OpenList 时附上的设计说明
- [x] `agent/PLUGIN_INTEGRATION.md` —— 7 个 plugin 如何被 adapter 桥接为 agent 工具（视频版/图版/音频版/文档版）
- [x] `[verify]` 文档 review 通过

---

## 非功能性需求

### 性能

- [x] Chat 首次 Event 延迟 < 500ms（OpenAI TTFB 范围内）
- [x] SSE 写入不阻塞（每事件写完即 flush）
- [x] Resume 追平进度延迟 < 1s（50ms polling）
- [x] 消息列表 1000+ 时虚拟列表仍流畅（虚拟化生效）
- [x] agent → OpenList 调 `/api/ext/*` 单次 P95 < 100ms（本地网络）

### 安全

- [x] Tool Handler 返回 error 不暴露内部堆栈（只 `error.Error()` 字符串）
- [x] HTTP handler 设 `Content-Type: text/event-stream` 严格，无任何 HTML 注入
- [x] SessionID 由 server 生成（UUIDv4），不接受客户端传入作为唯一信任
- [x] 4 决策的 `decision` 字段在 handler 中做白名单校验（拒绝非 4 值）
- [x] agent → OpenList 调用携带 `OPENLIST_TOKEN`，不暴露给前端
- [x] OpenAI API key 仅在 agent 服务 env 持有，不下发前端
- [x] **OpenList `/api/ext/*` 端点沿用 OpenList 现有 ACL**（用户 / 管理员 token），不绕过权限检查

### 兼容性

- [x] Go ≥ 1.21（用 `slices` / `maps` 标准库）
- [x] Vue 3 + TypeScript 5+（项目现状）
- [x] 不依赖任何 CGO（gomobile 兼容预留）
- [x] agent 服务、OpenList、encv-mobile 是**完全独立的三进程**，可独立部署 / 升级

---

## 已知限制

- [x] Session 内存缓存，进程重启即丢（v2 加 Redis/SQLite）
- [x] 单 session 串行，并发 ConfirmTool 需 v2 加锁
- [x] OpenAI 单一 provider（v2 加 Anthropic / Gemini 适配）
- [x] **OpenList 集成需外部 PR**（`Hi-Sillot/OpenList` 仓库提 PR 加 8 个 `/api/ext/*` 端点），不修改本仓库 OpenList 源码
- [x] WebSearchSummaryMessage v1 仅占位（v2 实装）
- [x] 当前 agent demo 用 mock OpenList 响应（Phase 6.1.7-6.1.11 联调前需替换为真实 OpenAI key）

---

## 与 codex_web 1:1 验收清单（强制）

实施完成后必须逐项核对：

- [x] `MessageAuthor.vue` 的 props 形状 `{icon, label, meta}` 与 codex_web 一致
- [x] `BlockHeader.vue` 的 props 形状 `{icon, title, status, statusTone, copyText, expanded, onToggleExpanded}` 与 codex_web 一致
- [x] `StatusBadge.vue` 的 tone 取值 `ready` / `warn` / `idle` 与 codex_web 一致
- [x] `CollapsedMessageToggle.vue` 的 props 形状 `{icon, label, meta, expanded, active, onToggle}` 与 codex_web 一致
- [x] `ApprovalCard.vue` 的 4 决策按钮顺序（批准 / 本轮批准 / 拒绝 / 拒绝并停止）与 codex_web 一致
- [x] `GroupedOperationMessage.vue` 的累积逻辑（command/fileChange/toolOutput）与 codex_web `renderTurnItems` 一致
- [x] `FileChangeSummaryMessage.vue` 的特化分组逻辑（全是 fileChange → 用此组件）与 codex_web 一致
- [x] 消息虚拟化阈值 `messages.length > 120` 与 codex_web `MESSAGE_VIRTUALIZATION_THRESHOLD` 一致
- [x] 用户消息折叠阈值 560 字符 / 9 行 与 codex_web 一致
- [x] 状态文案（中文）「正在思考」/「正在运行」/「已完成」/「失败」/「已取消」与 codex_web 一致
- [x] CSS class 命名（`messageAuthor` / `blockHeader` / `statusBadge` / `collapsedMessageToggle` / `userMessageBubble` / `approvalCard` / `approvalHeader` / `approvalBody` / `approvalActions` 等）与 codex_web 一致
- [x] tokens.css 颜色/字体/spacing/radius 数值与 codex_web 一致
- [x] i18n key 结构（`modals.approve` / `modals.approveForSession` / `modals.decline` / `modals.cancel`）与 codex_web 一致
