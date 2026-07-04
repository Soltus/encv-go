# Tasks: Go Agent 独立服务 + OpenList 定制接口 + encv-go 插件适配 + Vue 渲染壳（encv-mobile 主应用首页）

> 实施顺序：先 Go 库（自底向上）→ encv-go 插件系统适配 → HTTP/SSE → 演示程序 → Agent 设置 UI（schema）→ Vue 渲染壳（先原子组件 → 组合组件 → 顶层视图）→ 集成 demo + 验证。
> **UI 组件命名/Props 必须与 codex_web 1:1 对齐**，便于后续组件库合并。
> **架构铁律**：OpenList **不集成** agent 库，只暴露 `/api/ext/*` 端点。AI 入口**只在** encv-mobile 主应用首页（浮动按钮 + modal），**不走路由**。Settings 二级页 schema 驱动（沿用 `config.schema.json` + `useConfig` + `ConfigFieldItem`）。

---

## Phase 1: Go Agent 库骨架

### Task 1.1: 创建 agent 顶层 Go module

- [x] SubTask 1.1.1: 在 `/workspace/agent/` 创建 `go.mod`，module path 暂定 `github.com/encv/agent`
- [x] SubTask 1.1.2: 初始化 go.sum 空白文件，`go mod tidy` 一次确保格式
- [x] SubTask 1.1.3: 写 `agent/README.md`（库定位、使用示例、API 一览）

### Task 1.2: 实现核心类型 (types.go)

- [x] SubTask 1.2.1: 定义 `EventType` string + **6 个**常量（`EventTextDelta` / `EventReasoningDelta` / `EventToolCall` / `EventToolStatus` / `EventToolResult` / `EventStreamEnd`）
- [x] SubTask 1.2.2: 定义 `Event` 结构（Type + Data string JSON）
- [x] SubTask 1.2.3: 定义 `ToolCallData{ID, Name, Args, AutoRun, Kind}` 结构
- [x] SubTask 1.2.4: 定义 `ToolResultData{ID, Name, Result, IsError, Status, DurationMs}` 结构
- [x] SubTask 1.2.5: 定义 `MessageData` 结构（Content / Reasoning / ToolCalls / ToolResults）—— 用于流式累积
- [x] SubTask 1.2.6: 定义 `Decision` string + **4 个**常量（`DecisionAccept` / `DecisionAcceptForSession` / `DecisionDecline` / `DecisionCancel`）
- [x] SubTask 1.2.7: 定义 `ToolKind` string + 4 个常量（`KindCommand` / `KindFileChange` / `KindReadOnly` / `KindUnknown`）
- [x] SubTask 1.2.8: 写 `types_test.go` 覆盖 JSON 序列化/反序列化

### Task 1.3: 实现工具注册中心 (registry.go)

- [x] SubTask 1.3.1: 定义 `ToolDefinition{Schema, Handler, NeedConfirm, Kind}` 结构
- [x] SubTask 1.3.2: 定义 `ToolRegistry{tools map, mutex}`
- [x] SubTask 1.3.3: 实现 `NewRegistry()` 构造函数
- [x] SubTask 1.3.4: 实现 `Register(name, schema, handler, needConfirm, kind)` 方法
- [x] SubTask 1.3.5: 实现 `Get(name) (ToolDefinition, bool)` 方法
- [x] SubTask 1.3.6: 实现 `GetAllSchemas() []any` 方法
- [x] SubTask 1.3.7: 写 `registry_test.go` 覆盖：注册、并发 Get、Schema 列表、Kind 字段

---

## Phase 2: Agent 核心逻辑

### Task 2.1: 实现 SessionCache

- [x] SubTask 2.1.1: 在 `agent.go` 内定义 `SessionCache` 结构（Events / IsFinished / mu）
- [x] SubTask 2.1.2: 实现 `pushAndSend(ch, e)` 辅助方法
- [x] SubTask 2.1.3: 实现 `markFinished()` 辅助方法

### Task 2.2: 实现 Agent.Chat 流式入口

- [x] SubTask 2.2.1: 定义 `Agent` 结构（registry / sessions / apiKey / openaiClient / **sessionGrants**）
- [x] SubTask 2.2.2: 实现 `NewAgent(apiKey, registry) *Agent`
- [x] SubTask 2.2.3: 实现 `Chat(sessionID, messages) (<-chan, error)`
- [x] SubTask 2.2.4: 实现 `streamOpenAI(...)` 内部辅助
  - 调用 OpenAI streaming API（带 tool schemas）
  - 解析 delta → Event（含 `reasoning_delta`）
  - 处理 tool_calls：自动执行 / 挂起 / **session 级放行**

### Task 2.3: 实现 Agent.ConfirmTool（4 决策）

- [x] SubTask 2.3.1: 在 Agent 上加 `pendingCalls map[string]pendingCall` 字段
- [x] SubTask 2.3.2: 实现 `ConfirmTool(sessionID, toolCallID, decision) (<-chan, error)`
  - `accept` → 执行 + 推 ToolResult + 递归
  - `accept_for_session` → 执行 + **写 sessionGrants** + 递归
  - `decline` → 推 ToolResult(cancelled) + 递归
  - `cancel` → 推 ToolResult(cancelled) + 推 StreamEnd + **不递归**
- [x] SubTask 2.3.3: 写 `confirm_test.go` 覆盖 4 条决策路径

### Task 2.4: 实现 Agent.Resume

- [x] SubTask 2.4.1: 实现 `Resume(sessionID, offset) (<-chan, error)`
  - 50ms polling 等流
  - 追到 IsFinished 推 EventStreamEnd

### Task 2.5: OpenAI 客户端集成 (openai.go)

- [x] SubTask 2.5.1: 引入 `github.com/sashabaranov/go-openai`
- [x] SubTask 2.5.2: 实现 `createChatCompletionStream(messages, tools)`
- [x] SubTask 2.5.3: 实现 `parseDelta(delta) (text, reasoning, toolCalls, isFinished)`
- [x] SubTask 2.5.4: 写 `openai_test.go` mock server

---

## Phase 3: encv-go 插件系统适配 + agent 集成

### Task 3.1: 插件扫描器（plugin_scanner.go）

- [x] SubTask 3.1.1: 写 `agent/plugin_scanner.go`，输入 `plugins.Plugins []Plugin`
- [x] SubTask 3.1.2: 实现 `scanPluginTools(plugins) []ToolDefinition` —— 遍历每个 plugin，生成 2 个 ToolDefinition（encrypt + decrypt），跳过 alistencrypt
- [x] SubTask 3.1.3: 工具命名：`(plugin.Name() + "_encrypt")` / `(plugin.Name() + "_decrypt")`
- [x] SubTask 3.1.4: 工具 schema 动态生成：
  - `input_paths: array<string>` (required)
  - `output_path: string` (required)
  - `extra_fields: object` —— 来自 `plugin.GetTaskOptions().ExtraFields` 字段拼装
  - `password: string` —— 根据 `PasswordStrategy` 决定 required/optional/hidden
  - `version: integer` —— 根据 `SupportVersionSelect` 决定 required/hidden
- [x] SubTask 3.1.5: 工具 description 模板：使用 `plugin.Name() + plugin.GetContainerExtension()` 拼中文描述
- [x] SubTask 3.1.6: 写单测覆盖：7 个插件产出 12 个工具（7×2 - 2 跳过 alistencrypt），schema 字段对每个 plugin 正确

### Task 3.2: 插件工具 handler adapter

- [x] SubTask 3.2.1: 写 `agent/plugin_tool_handler.go`，实现 `makePluginEncryptHandler(plugin) func(string) (string, error)`
- [x] SubTask 3.2.2: handler 步骤：
  - 解析 args JSON → {InputPaths, OutputPath, ExtraFields, Password, Version}
  - `plugin.SetTaskExtraFields(extraFields)` 注入
  - `plugin.PreEncryptProcessor(index, inputPath, inputRoot, outputDir)`
  - 打开 inputPath → `plugin.Encrypt(reader)` → 拿到 `*crypto.EncryptionResult`
  - `plugin.PostEncryptProcessor(result)` 拿到最终输出路径
  - 返回 `{output_path, duration_ms, file_size}` JSON
- [x] SubTask 3.2.3: 写 `makePluginDecryptHandler(plugin) func(string) (string, error)`（对称：PreDecryptProcessor + CanDecrypt 自检 + 解密逻辑）
- [x] SubTask 3.2.4: 写单测：mock plugin（实现 Plugin 接口）→ 验证 handler 流程

### Task 3.3: agent demo 集成插件工具

- [x] SubTask 3.3.1: `cmd/agent-demo/main.go` 中，从 `agent_settings` 读 `enabled_tools`
- [x] SubTask 3.3.2: 调 `scanPluginTools(plugins.Plugins)` 得到 12 个工具
- [x] SubTask 3.3.3: 调 `registry.Register` 把 12 个工具 + 8 个 OpenList 工具都注册
- [x] SubTask 3.3.4: `enabled_tools` 白名单过滤（如果用户禁用了 `video_encrypt` 则不注册）
- [x] SubTask 3.3.5: 验证 demo 启动后 `/api/chat` 返回的工具列表包含 20 个

### Task 3.4: 容器格式识别自检

- [x] SubTask 3.4.1: 在 plugin_tool_handler 中，加 `plugin.CanDecrypt(path)` 自检
- [x] SubTask 3.4.2: 自检失败 → 包装为 `{error: "container_format_mismatch", suggested_tool: "<correct_plugin>_decrypt"}` 返回 IsError=true

### Task 3.5: 读取 agent_settings 配置

- [x] SubTask 3.5.1: 写 `agent/config_loader.go`，从 `~/.encv/config.user.json` 读 `agent_settings` 段
- [x] SubTask 3.5.2: 把字段映射为环境变量 / 配置结构：`OPENAI_API_KEY` / `OPENAI_BASE_URL` / `OPENAI_MODEL` / `OPENLIST_BASE_URL` / `OPENLIST_TOKEN` / `DEFAULT_CONTAINER_VERSION`
- [x] SubTask 3.5.3: 单测：mock 写入 `config.user.json` → 验证字段正确加载

---

## Phase 4: HTTP/SSE Handlers

### Task 4.1: 实现 /api/chat handler

- [x] SubTask 4.1.1: 定义 `ChatRequest{SessionID, Messages}`
- [x] SubTask 4.1.2: 实现 `HandleChat(w, r)`

### Task 4.2: 实现 /api/resume handler

- [x] SubTask 4.2.1: 定义 `ResumeRequest{SessionID, Offset}`
- [x] SubTask 4.2.2: 实现 `HandleResume(w, r)`

### Task 4.3: 实现 /api/confirm handler（4 决策）

- [x] SubTask 4.3.1: 定义 `ConfirmRequest{SessionID, ToolCallID, Decision string}`（接受 `accept`/`accept_for_session`/`decline`/`cancel`）
- [x] SubTask 4.3.2: 实现 `HandleConfirm(w, r)`：解析 decision → 调 ConfirmTool

### Task 4.4: SSE 通用工具函数

- [x] SubTask 4.4.1: 抽 `writeSSE(w, event)` 工具
- [x] SubTask 4.4.2: 写 `http_test.go` mock ResponseWriter

### Task 4.5: 实现 /api/agent/test handler（测试连接）

- [x] SubTask 4.5.1: 定义 `TestRequest{}` / `TestResponse{OpenAIOK, OpenListOK, Errors}`
- [x] SubTask 4.5.2: 实现 `HandleTest(w, r)` —— 同时 ping OpenAI (`GET /v1/models`) 和 OpenList (`GET /api/me`) → 返回 ok/fail
- [x] SubTask 4.5.3: 超时 5s / 错误包装为结构化响应

---

## Phase 5: 演示程序

### Task 5.1: agent-demo 入口（独立 Go 服务，调 OpenList 定制接口 + 插件工具）

- [x] SubTask 5.1.1: 在 `cmd/agent-demo/main.go` 写演示程序（独立二进制，跑 :5245）
  - 从 `agent_settings` 加载 OpenAI/OpenList 配置（Task 3.5）
  - 注册 8 个 OpenList 工具（list_files / read_file / write_file / delete_file / rename / exec_command / get_storage_info / search_files）
  - 调 `scanPluginTools(plugins.Plugins)` 注册 12 个插件工具（Task 3.1-3.2）
  - 跳过 alistencrypt（避免与 OpenList 工具重复）
  - 应用 `enabled_tools` 白名单过滤
  - mount 4 个 HTTP handler (`/api/chat` `/api/resume` `/api/confirm` `/api/agent/test`)
  - 监听 :5245
- [x] SubTask 5.1.2: 写 Makefile 或 go run 指令
- [x] SubTask 5.1.3: 验证 `curl -N http://localhost:5245/api/chat -d '{...}'` 流式输出 SSE，工具列表包含 20 个

### Task 5.2: OpenList 定制接口契约文档（驱动外部 PR）

- [x] SubTask 5.2.1: 写 `agent/openlist_contract.md` —— 8 个端点的完整 spec（请求/响应/错误码/鉴权）
- [x] SubTask 5.2.2: 写 `agent/openlist_client.go` —— Go 客户端 + 单测（mock OpenList 响应）
- [x] SubTask 5.2.3: 写 `agent/openlist_client_test.go` —— 8 个端点 round-trip
- [x] SubTask 5.2.4: 写 PR 模板 `docs/pr-openlist-ext-api.md` —— 提交到 Hi-Sillot/OpenList 时附上的设计说明

---

## Phase 6: Agent 设置 UI（schema 驱动 + Settings 二级页）

### Task 6.1: 在 config.schema.json 添加 agent_settings 段

- [x] SubTask 6.1.1: 读 `app/encv-mobile/src/config/schema.json`，定位 `plugin_settings` 段附近
- [x] SubTask 6.1.2: 在 `properties` 顶层加 `agent_settings` 对象（10 个字段：openai_api_key / openai_base_url / openai_model / openlist_base_url / openlist_token / default_container_version / enabled_tools / system_prompt / max_tool_calls_per_turn）
- [x] SubTask 6.1.3: 字段类型映射：
  - `string secret: true` → 密码框
  - `string` → 文本框
  - `string enum: [...]` → select
  - `integer` → 数字框
  - `string format: multiline` → textarea
  - `array items: string` → 行式编辑（或 select-multiple）
- [x] SubTask 6.1.4: 验证 `useConfig.schemaFields` 自动包含 `agent_settings` 段

### Task 6.2: Settings.vue 加二级页入口（参考 goPlugins 模式）

- [x] SubTask 6.2.1: 在 `Settings.vue` `goPlugins` 附近加 `function goAgent() { router.push('/tabs/settings/agent') }`
- [x] SubTask 6.2.2: 加 `<ion-item button @click="goAgent" detail>`（图标用 `sparklesOutline`）
- [x] SubTask 6.2.3: i18n key：复用 `settings.agent` = "AI 助手" / `settings.agentSettingsHelp` = "配置 OpenAI / OpenList 接入信息"

### Task 6.3: AgentSettingsDetail.vue 二级页

- [x] SubTask 6.3.1: 创建 `app/encv-mobile/src/views/AgentSettingsDetail.vue`
- [x] SubTask 6.3.2: 模板结构（参考 `AppearanceDetail.vue` / `PluginSettings.vue`）：
  - `<ion-toolbar>` 含返回按钮 + 保存按钮（`saveConfig()`）
  - 主体：`<ConfigFieldItem>` 渲染 `agent_settings` 下所有字段
  - 底部：测试连接按钮（POST `/api/agent/test` → toast 结果）
- [x] SubTask 6.3.3: 复用 `useConfig` composable
- [x] SubTask 6.3.4: 路由 `/tabs/settings/agent` 注册

### Task 6.4: 后端 /api/agent/test handler

- [x] SubTask 6.4.1: 写 `internal/agent/test_handler.go`
- [x] SubTask 6.4.2: 读 `agent_settings` → ping OpenAI（`GET OPENAI_BASE_URL/v1/models`）+ ping OpenList（`GET OPENLIST_BASE_URL/api/me`）
- [x] SubTask 6.4.3: 并发请求，5s 超时，返回 `{openai_ok, openlist_ok, errors}`
- [x] SubTask 6.4.4: 写单测 mock OpenAI/OpenList 响应

### Task 6.5: 路由注册（沿用现有模式）

- [x] SubTask 6.5.1: 在 `app/encv-mobile/src/router/index.ts` 加 `/tabs/settings/agent` 路由
- [x] SubTask 6.5.2: 命名：参考 `/tabs/settings/plugins` 注册方式

### Task 6.6: i18n 完整化

- [x] SubTask 6.6.1: 在 `app/encv-mobile/src/i18n/settings.ts` 加 keys：
  - `settings.agent` / `settings.agentSettings` / `settings.agentSettingsHelp` / `settings.testConnection` / `settings.testConnectionSuccess` / `settings.testConnectionFailed` / `settings.testConnectionOpenAIFailed` / `settings.testConnectionOpenListFailed`
- [x] SubTask 6.6.2: 在 `app/encv-mobile/src/i18n/extensions.ts`（或新建 `agent.ts`）加 field-level keys：
  - `agent_settings.openai_api_key` / `openai_base_url` / `openai_model` / `openlist_base_url` / `openlist_token` / `default_container_version` / `enabled_tools` / `system_prompt` / `max_tool_calls_per_turn`

---

## Phase 7: Vue 渲染壳（对齐 codex_web 组件）

### Task 7.1: useAgent composable

- [x] SubTask 7.1.1: 在 `composables/useAgent.ts` 创建
- [x] SubTask 7.1.2: 定义 `reactive messages[]` / `ref status` / `let sessionId` / `let eventOffset`
- [x] SubTask 7.1.3: 实现 `processSSE(stream)` 解析器（支持 6 种 event type）
- [x] SubTask 7.1.4: 实现 `send(text)` 发起对话
- [x] SubTask 7.1.5: 实现 `confirmTool(toolCallId, decision: Decision)` 4 决策
- [x] SubTask 7.1.6: 实现 `resume()` mount 时自动续传
- [x] SubTask 7.1.7: 实现 `saveState/loadState` localStorage
- [x] SubTask 7.1.8: 写 unit test

### Task 7.2: 原子组件（MessageAuthor / BlockHeader / StatusBadge）

- [x] SubTask 7.2.1: **`StatusBadge.vue`** — props `{label, tone: 'ready'|'warn'|'idle'}`，对应 CSS `.statusBadge` `.statusBadge_ready/warn/idle`
- [x] SubTask 7.2.2: **`MessageAuthor.vue`** — props `{icon, label, meta}`，class `messageAuthor` `avatar` `authorName` `authorMeta`
- [x] SubTask 7.2.3: **`BlockHeader.vue`** — props `{icon, title, status, statusTone, copyText, expanded, onToggleExpanded}`，class `blockHeader` `blockTitle` `blockActions`
- [x] SubTask 7.2.4: 写 `tokens.css`（颜色/字体/spacing/radius）—— 复用 codex_web token 数值
- [x] SubTask 7.2.5: 写 `agent.css`（messageAuthor / blockHeader / statusBadge）—— 1:1 移植 codex_web App.module.css 对应部分

### Task 7.3: 复合组件（ApprovalCard / CollapsedMessageToggle / GroupedOperationMessage）

- [x] SubTask 7.3.1: **`CollapsedMessageToggle.vue`** — props `{icon, label, meta, expanded, active, onToggle}`，class `collapsedMessageToggle` + `collapsedMessageToggleActive`
- [x] SubTask 7.3.2: **`ApprovalCard.vue`** — 4 决策按钮 + `approvalHeader` + `approvalBody` + `approvalFiles` + `approvalDiff` + `approvalActions`
  - 按钮顺序固定：批准 / 本轮批准（条件显示） / 拒绝 / 拒绝并停止
  - 点击 → 按钮显示「处理中」并禁用其他按钮
- [x] SubTask 7.3.3: **`GroupedOperationMessage.vue`** — 累积 command/fileChange/toolOutput，渲染摘要
- [x] SubTask 7.3.4: **`FileChangeSummaryMessage.vue`** — 文件变更特化分组
- [x] SubTask 7.3.5: **`WebSearchSummaryMessage.vue`** — Web 搜索特化分组（v1 可先放空实现）

### Task 7.4: 用户消息 / Markdown 渲染

- [x] SubTask 7.4.1: **`UserMessageBubble.vue`** — 右对齐蓝色气泡 + 长文本自动折叠（>560 字符 或 >9 行）
- [x] SubTask 7.4.2: **`MarkdownStream.vue`** — 封装 markstream-vue 的 `<MarkStream>` + `dist/style.css`
- [x] SubTask 7.4.3: 添加 i18n key（`modals.approve`/`modals.approveForSession`/`modals.decline`/`modals.cancel`/`agent.thinking`/`agent.running`/`agent.completed`/`agent.failed`/`agent.cancelled`/`agent.collapse`/`agent.expand`）

### Task 7.5: 虚拟化与顶层视图

- [x] SubTask 7.5.1: **`MessageVirtualList.vue`** — 封装 `vue-virtual-scroller` 的 `<RecycleScroller>`（itemSize=112, minItemSize=80, buffer=600）
- [x] SubTask 7.5.2: 阈值判断：`messages.length > 120` 用虚拟列表，否则普通 v-for
- [x] SubTask 7.5.3: **`renderTurnItems.ts`** 组合式 — 实现与 codex_web `renderTurnItems()` 等价的逻辑
  - 累积 `operationGroup`（command/fileChange/toolOutput）
  - 累积 `webSearchGroup`
  - flush 时根据 group 类型渲染不同组件
- [x] SubTask 7.5.4: **`AgentChat.vue`** — 顶层视图
  - 调用 `renderTurnItems(messages, status)`
  - 输入框 + 发送/停止按钮
  - 自动滚动到底部（`scrollToIndex(messages.length - 1, {align: 'end'})`）

### Task 7.6: 集成入口（首页浮动按钮 + 依赖）

- [x] SubTask 7.6.1: `encv-mobile/package.json` 加 `markstream-vue` + `vue-virtual-scroller` 依赖
- [x] SubTask 7.6.2: `pnpm install` 验证
- [x] SubTask 7.6.3: **`AgentEntry.vue`** 创建 —— 浮动 AI 按钮 + `modalController.create(AgentChat)` 弹窗
- [x] SubTask 7.6.4: **`Home.vue`** 改造 —— 在 `<IonContent>` 内挂载 `<AgentEntry />`（右下角浮动）
- [x] SubTask 7.6.5: i18n key 添加（`agent.title` / `agent.fabLabel` / `agent.placeholder`）
- [x] SubTask 7.6.6: **不使用路由**——确认 `router/index.ts` 不加 `/agent` 路由（避免跳页）

---

## Phase 8: 端到端验证

### Task 8.1: 沙箱 dev 联调（3 进程：agent + OpenList + encv-mobile）

- [x] SubTask 8.1.1: `ecosystem.config.cjs` 加 `agent-demo` app（编译 + 跑 :5245）
- [x] SubTask 8.1.2: `pm2 save` 持久化
- [x] SubTask 8.1.3: `curl -N http://localhost:5245/api/chat` SSE 正常流
- [x] SubTask 8.1.4: preview-gateway 加 `/agent-api/*` upstream → `127.0.0.1:5245`
- [x] SubTask 8.1.5: 浏览器 encv-mobile 主页 → 看到右下角浮动 AI 按钮
- [x] SubTask 8.1.6: 点击浮动按钮 → 弹出 AgentChat modal（全屏 overlay）
- [x] SubTask 8.1.7: 输入 "list files" → UI 展示 ✅ list_files + 文件列表（数据来自 OpenList /api/ext/list_files）
- [x] SubTask 8.1.8: 输入 "delete foo.txt" → **ApprovalCard 4 按钮** 出现 → 点击「批准」→ UI 展示 ✅ delete_file
- [x] SubTask 8.1.9: 同上但点击「本轮批准」→ 验证 sessionGrants 生效（第二次同类调用直接执行）
- [x] SubTask 8.1.10: 同上但点击「拒绝」→ UI 展示 ToolResult error（user_rejected），LLM 继续
- [x] SubTask 8.1.11: 同上但点击「拒绝并停止」→ UI 立即收到 stream_end，本轮结束
- [x] SubTask 8.1.12: 流式过程中刷新页面 → 几秒内 resume 追平进度
- [x] SubTask 8.1.13: 注入 130 条消息 → 验证 `<MessageVirtualList>` 触发
- [x] SubTask 8.1.14: 验证 0 个 console error、0 个 SSE 解析失败
- [x] SubTask 8.1.15: 验证 OpenList 进程崩溃时，agent 服务优雅降级（tool 调 OpenList 失败 → ToolResult error，不连带 agent 崩溃）
- [x] SubTask 8.1.16: **Settings → AI 助手二级页 → 修改 openai_api_key → 保存 → 验证 agent-demo 重启后使用新 key**
- [x] SubTask 8.1.17: **Settings → AI 助手二级页 → 测试连接按钮 → 验证 OpenAI ✓ + OpenList ✓ toast**
- [x] SubTask 8.1.18: **输入 "用 video 插件加密 foo.mp4" → ApprovalCard 4 按钮（video_encrypt）→ 批准 → 验证产生 .encv 容器**

### Task 8.2: 单元测试

- [x] SubTask 8.2.1: `go test ./agent/...` 全绿
- [x] SubTask 8.2.2: `go test -race ./agent/...` 无 race warning
- [x] SubTask 8.2.3: `pnpm test` vitest 全绿（含 useAgent + 各 Vue 组件 + AgentSettingsDetail）
- [x] SubTask 8.2.4: 覆盖率：Go ≥ 70%、TypeScript ≥ 70%

### Task 8.3: 文档同步

- [x] SubTask 8.3.1: 在 `unify-sandbox-preview-port/spec.md` 加 D16 章节（agent-api upstream + encv-mobile 首页 fab 入口 + settings 二级页）
- [x] SubTask 8.3.2: 在 `unify-sandbox-preview-port/tasks.md` 加对应 task
- [x] SubTask 8.3.3: 在 `unify-sandbox-preview-port/checklist.md` 加检查点
- [x] SubTask 8.3.4: `agent/README.md` 含使用示例 + API 一览 + **Decision 4 选 1 表格** + **OpenList 8 端点契约说明** + **插件 12 工具注册表**
- [x] SubTask 8.3.5: 写 `encv-mobile/src/components/agent/README.md` —— 记录与 codex_web 1:1 对应的组件、props、CSS class，**强调：入口在首页浮动按钮，不走路由**
- [x] SubTask 8.3.6: 写 `docs/pr-openlist-ext-api.md` —— 提交到 Hi-Sillot/OpenList 时附上的设计说明（含 8 个端点契约 + 鉴权 + 错误码），便于跨项目复用
- [x] SubTask 8.3.7: 写 `agent/PLUGIN_INTEGRATION.md` —— 说明 7 个 plugin 如何被 adapter 桥接为 agent 工具，便于未来新增 plugin 时参考

---

## Phase 9: Agent 本地 fs 工具（取代 OpenList 外部依赖）

> **背景**：原计划让 agent 通过 HTTP 调 OpenList 的 `/api/ext/*` 端点。但用户反馈"和 openlist 八竿子打不着，现在完全不要考虑 openlist 都能力"，所以本 phase 把 fs 能力直接做进 encv-go 自身（in-process）。OpenList 相关代码（`agent/openlist_client.go` 等）已撤销。

### Task 9.1: agent_fs_bridge.go 实现

- [x] SubTask 9.1.1: 创建 `internal/server/agent_fs_bridge.go`
- [x] SubTask 9.1.2: 实现 `mountInfo` 结构（id/type/public_path/description/available）
- [x] SubTask 9.1.3: 实现 `(s *Server) ListFSMounts()` —— 枚举 `servingDir` / `webdavDir`
- [x] SubTask 9.1.4: 实现 `(s *Server) resolveMount(mountID)` —— 物理根路径解析
- [x] SubTask 9.1.5: 实现 5 个 schema：`fsToolListMountsSchema` / `ListFiles` / `ReadFile` / `StatFile` / `GetStorageInfo`
- [x] SubTask 9.1.6: 实现 `(s *Server) ListFSTools()` —— 5 个工具元信息
- [x] SubTask 9.1.7: 实现 `(s *Server) executeFSTool(ctx, name, args)` 派发器
- [x] SubTask 9.1.8: 实现 5 个 handler：`fsListMounts` / `fsListFiles` / `fsReadFile` / `fsStatFile` / `fsGetStorageInfo`
- [x] SubTask 9.1.9: 实现辅助函数：`statFS`（syscall.Statfs 跨平台）、`detectContainerEntry`（读 4 字节 ENCV magic）、`base64Encode`（手写避免引 base64 包）
- [x] SubTask 9.1.10: 所有路径走 `utils.SafeResolveToAbsPath` 沙箱化，越界返回 `path_forbidden`

### Task 9.2: agent_plugin_bridge.go 工具聚合

- [x] SubTask 9.2.1: 新增 `executeAgentTool` 方法（plugin 派发 + fs 派发）
- [x] SubTask 9.2.2: 新增 `(s *Server) ListAgentTools()` —— 合并 `ListPluginTools()` + `s.ListFSTools()`
- [x] SubTask 9.2.3: 新增 `agentToolsToOpenAITools` —— 转 OpenAI `{type:"function", function:{name, description, parameters}}` 格式

### Task 9.3: OpenAI 请求 reqBody.tools 字段（关键修复）

- [x] SubTask 9.3.1: `callOpenAIChatOnce` 新增 `openAITools []map[string]interface{}` 参数，非空时写入 `reqBody.tools` + `reqBody.tool_choice = "auto"`
- [x] SubTask 9.3.2: `callOpenAIStream` 同步加 `openAITools` 参数
- [x] SubTask 9.3.3: `streamChat` 新增 `openAITools` + `toolMeta` 参数
- [x] SubTask 9.3.4: `executeAndRecurse` 改为方法（或接受 `s *Server`），调用 `s.executeAgentTool` 代替原 `executePluginTool`
- [x] SubTask 9.3.5: `handleAgentChat` 构造 `agentTools` / `openAITools` / `toolMeta` 并透传到所有下游调用
- [x] SubTask 9.3.6: `handleAgentConfirm` 递归 `streamChat` 调用也透传 `openAITools` + `toolMeta`
- [x] SubTask 9.3.7: `emitToolCallEvent` 新增 `toolMeta` 参数，从 `toolMeta[tc.Function.Name]` 读 `needConfirm` + `kind`

### Task 9.4: fs 工具测试

- [x] SubTask 9.4.1: 创建 `internal/server/agent_fs_bridge_test.go`
- [x] SubTask 9.4.2: 覆盖 `ListFSMounts` / `resolveMount` 4+4 场景
- [x] SubTask 9.4.3: 覆盖 5 个 fs tool 的 happy + error 路径
- [x] SubTask 9.4.4: 专门覆盖 `path_forbidden` —— 用 `subdir/../../etc` 真正逃出 baseDir
- [x] SubTask 9.4.5: 覆盖 `agentToolsToOpenAITools` 格式正确性
- [x] SubTask 9.4.6: 覆盖 `executeAgentTool` 三路派发（plugin / fs / unknown）
- [x] SubTask 9.4.7: `go test ./internal/server/...` 全绿

### Task 9.5: 清理与回归

- [x] SubTask 9.5.1: 确认 `agent/openlist_client.go` / `agent/cmd/agent-demo/main.go` 已撤销 OpenList 工具注册
- [x] SubTask 9.5.2: 确认 `agent/openlist_client_test.go` 通过（pre-existing test 损坏已修复）
- [x] SubTask 9.5.3: `go build ./...` 无新错误（`cmd/bench-report` 的 Windows syscall 错误是 pre-existing，与本 phase 无关）
- [x] SubTask 9.5.4: `go test ./internal/server/...` 全绿

---

## Phase 10: Context 图标 + 时间线混合内容（codex_web 风格升级）

> **背景**：当前 AI 输出只渲染了纯 markdown 卡片，没有 timeline 式的混合内容（md 渲染 → 已编辑 2 个文件 → 读取 2 个文件 → md 渲染 → 思考 → 搜索找到 15 行 → ...）。本 phase 把 6 个已有组件按 codex_web 风格升级（可点击 header + chevron + StatusBadge 替换文本），并在会话顶部加 Context 图标（点击弹 popover 看任务列表 + 上下文占用 + 引用文件）。
>
> **参考仓库**：[codex_web](https://github.com/shopkeeper2020/codex_web) 的 `apps/web/src/app/components/MessageBlocks.tsx`

### Task 10.1: 后端 `/api/agent/context-usage` 端点

- [x] SubTask 10.1.1: 创建 `internal/server/agent_context_usage.go`
- [x] SubTask 10.1.2: 实现 `modelContextWindows` map（gpt-4o→128000, claude-3-5-sonnet→200000, deepseek-chat→64000, qwen-plus→131072, o1→200000, glm-4-plus→128000 等）
- [x] SubTask 10.1.3: 实现 `lookupContextWindow(model string) int` —— 表查 + 启发式（128k→128000, 32k→32000, 1m→1000000）+ 默认 8192
- [x] SubTask 10.1.4: 实现 `estimateStringTokens(s string) int` —— `CJK字符/1.5 + ASCII字符/4`
- [x] SubTask 10.1.5: 实现 `estimateTokens(messages []chatMsg) int` —— 累加 content + tool_calls args + 每个 role 4 token
- [x] SubTask 10.1.6: 实现 `planTodo{content, status}` struct + `isPlanToolName(name)`（接受 write_todos/set_plan/plan_update/todos/update_todos）
- [x] SubTask 10.1.7: 实现 `parseTodosJSON(s)` —— 支持 `[]` 和 `{todos:[...]}` 两种格式
- [x] SubTask 10.1.8: 实现 `todosFromArray(arr)` —— 处理 `active_form` fallback + 默认 status
- [x] SubTask 10.1.9: 实现 `extractTodos(messages)` —— 反向扫描取最近一次 plan 工具调用的结果
- [x] SubTask 10.1.10: 实现 `referencedFile{path, mountId, viaTool, lastRefAt}` struct
- [x] SubTask 10.1.11: 实现 `readPathFromToolArgs(toolName, argsJSON)` —— 仅 fs 工具（read_file/list_files/stat_file）返回 `{mountId, path}`
- [x] SubTask 10.1.12: 实现 `extractReferencedFiles(messages)` —— dedup by path + sort by recency
- [x] SubTask 10.1.13: 实现 `countCompactions(sess)` —— 扫 `EventCache` 中 `type:"context_compaction"` 计数
- [x] SubTask 10.1.14: 实现 `handleAgentContextUsage(c *gin.Context)` —— 返回 6 字段响应
- [x] SubTask 10.1.15: 在 `agent_api.go` 注册 `r.GET("/api/agent/context-usage", s.handleAgentContextUsage)`

### Task 10.2: context_usage 端点测试

- [x] SubTask 10.2.1: 创建 `internal/server/agent_context_usage_test.go`
- [x] SubTask 10.2.2: 覆盖 `estimateStringTokens` —— empty / ASCII / CJK / mixed
- [x] SubTask 10.2.3: 覆盖 `estimateTokens` —— empty / with content / with tool_calls
- [x] SubTask 10.2.4: 覆盖 `lookupContextWindow` —— known / heuristic / default
- [x] SubTask 10.2.5: 覆盖 `parseTodosJSON` —— array / wrapped / empty / active_form fallback / default status
- [x] SubTask 10.2.6: 覆盖 `extractTodos` —— from latest / no plan / accepts multiple tool names
- [x] SubTask 10.2.7: 覆盖 `extractReferencedFiles` —— empty / read_file / list_files / list_files default root / dedup+recency / ignores plugin tools
- [x] SubTask 10.2.8: 覆盖 `readPathFromToolArgs` —— unknown / invalid JSON / missing rel_path
- [x] SubTask 10.2.9: 覆盖 `containsAny` helper
- [x] SubTask 10.2.10: 覆盖 `handleAgentContextUsage` —— no session / with session / with todos+refs / default sessionId / percent 计算
- [x] SubTask 10.2.11: `go test ./internal/server/...` 全绿（33+ 测试）

### Task 10.3: 前端 `useContextUsage` composable

- [x] SubTask 10.3.1: 创建 `app/encv-mobile/src/composables/useContextUsage.ts`
- [x] SubTask 10.3.2: 定义 `ContextUsage` / `ContextTodo` / `ContextReferencedFile` / `ContextUsageResponse` 类型
- [x] SubTask 10.3.3: 实现 `useContextUsage({sessionId, status})` composable
- [x] SubTask 10.3.4: streaming/confirming → 5s 间隔，idle/error → 30s 间隔
- [x] SubTask 10.3.5: 错误静默：拉取失败仅 `console.debug`
- [x] SubTask 10.3.6: `watch(() => opts.status.value)` 切换频率；`watch(() => opts.sessionId.value)` 触发拉取（**仅在 timer != null 时**）
- [x] SubTask 10.3.7: `onUnmounted` 清理 timer + watch
- [x] SubTask 10.3.8: 导出 `start()` / `stop()` / `refresh()` / `data` / `loading` / `lastFetchedAt`

### Task 10.4: ContextIcon + ContextPopover 组件

- [x] SubTask 10.4.1: 创建 `app/encv-mobile/src/components/agent/ContextIcon.vue`
  - header 按钮：`<IonButton fill="clear" size="small">` + `<IonIcon :icon="layersOutline" />` + 百分比文本
  - 4 tone 配色（`tone-ok` / `tone-warn` / `tone-danger` / `tone-idle`）
  - compression badge：`<div v-if="compactions > 0">` 红点 + 数字
  - 点击触发 `<IonPopover :is-open="show" :event="event" @didDismiss="show=false">`
  - prop `compact?: boolean` 控制大小
- [x] SubTask 10.4.2: 创建 `app/encv-mobile/src/components/agent/ContextPopover.vue`
  - 3 section: `usage-section` / `todos-section` / `files-section`
  - Usage 段：`<div class="usageBar">` gradient 进度条 + 数值（`formatTokens` 1.2K/1.5M）
  - Todos 段：`<ul class="todoList">` 每项含 status icon + label + 进度条
  - Files 段：`<ul class="fileList">` 每项含 path + mountId + viaTool + 相对时间
  - 空状态：`<div class="emptyState">{{ t('context.emptyTodos') }}</div>`
  - 加载中：`<IonSpinner v-if="loading" />`

### Task 10.5: AgentChat 集成 Context 图标

- [x] SubTask 10.5.1: 修改 `app/encv-mobile/src/views/AgentChat.vue`
- [x] SubTask 10.5.2: import `ContextIcon` + `import { onUnmounted }` from vue
- [x] SubTask 10.5.3: useAgent() 解构增加 `contextUsage`
- [x] SubTask 10.5.4: `onMounted(() => contextUsage.start())` + `onUnmounted(() => contextUsage.stop())`
- [x] SubTask 10.5.5: 在 header 标题与新会话按钮之间插入 `<ContextIcon :data="contextUsage.data.value" :loading="contextUsage.loading.value" />`

### Task 10.6: 6 个组件按 codex_web 风格重做

- [x] SubTask 10.6.1: **`GroupedOperationMessage.vue`** —— header 改为 `<button>` + chevron + 整体可折叠 + per-item 可展开
- [x] SubTask 10.6.2: **`FileChangeSummaryMessage.vue`** —— header 改为 `<button>` + chevron + 整体可折叠
- [x] SubTask 10.6.3: **`ReasoningMessage.vue`** —— `t('agent.reasoning')` + StatusBadge（idle+pulse when streaming）+ meta text `t('agent.thinkingMeta')`
- [x] SubTask 10.6.4: **`WebSearchSummaryMessage.vue`** —— `t('agent.webSearch')` + StatusBadge with total hits + per-query hit count badge
- [x] SubTask 10.6.5: **`PlanBlock.vue`** —— ionicons (checkmarkCircle/sync/ellipsisHorizontalCircle) 替代字符 + `plan-progress` 进度条 + `2/5 (1 进行中)` 计数
- [x] SubTask 10.6.6: **`AgentTaskMessage.vue`** —— ionicons + `agentTaskProgressBar` + 状态徽章
- [x] SubTask 10.6.7: **`StatusBadge.vue`** —— 新增 `pulse?: boolean` prop + `statusBadge_pulse` CSS animation

### Task 10.7: useAgent 集成 + 测试修复

- [x] SubTask 10.7.1: 修改 `useAgent.ts` —— 导入 `useContextUsage`，不自动 start（让 AgentChat 视图管理 lifecycle）
- [x] SubTask 10.7.2: 在 useAgent return 中暴露 `contextUsage`
- [x] SubTask 10.7.3: 修改 `useAgent.test.ts` —— 给 `/api/agent/context-usage` 加默认 mock 返回空 session 数据
- [x] SubTask 10.7.4: 修复 `useContextUsage` 的 sessionId watch 在 `timer == null` 时不发请求（避免 useAgent 初始化时偷偷 fetch）

### Task 10.8: i18n + 清理

- [x] SubTask 10.8.1: 在 `app/encv-mobile/src/i18n/agent.ts` 加 keys：
  - `agent.thinkingMeta` = "正在推理…" / "Thinking…"
  - `agent.thought` = "已思考" / "Thought"
  - `agent.reasoning` = "推理" / "Reasoning"
  - `agent.webSearch` = "搜索" / "Search"
  - `agent.queries` = "个查询" / "queries"
  - `agent.query` = "个查询" / "query"
- [x] SubTask 10.8.2: 移除 `app/encv-mobile/src/views/AgentChat.vue` 残留的 `console.log` / 调试代码

### Task 10.9: 全栈验证

- [x] SubTask 10.9.1: `go test ./internal/server/...` 全绿（30+ context_usage 测试 + 原有回归）
- [x] SubTask 10.9.2: 前端 vitest 55 个 useAgent 测试 + 全套 547 个测试（3 个 pre-existing 失败与本 phase 无关）
- [x] SubTask 10.9.3: `go build ./...` 无新错误
- [x] SubTask 10.9.4: 文档同步（spec.md §Context 图标 + tasks.md Phase 10）完成

---

# Task Dependencies

| Task | Depends on |
|------|-----------|
| 1.1 (Go module) | — |
| 1.2 (types) | 1.1 |
| 1.3 (registry) | 1.2 |
| 2.1 (SessionCache) | 1.2 |
| 2.2 (Chat) | 1.3, 2.1, 2.5 |
| 2.3 (ConfirmTool) | 2.2 |
| 2.4 (Resume) | 2.1 |
| 2.5 (OpenAI 集成) | 1.3 |
| 3.1 (plugin_scanner) | 1.3 |
| 3.2 (plugin_tool_handler) | 3.1 |
| 3.3 (agent demo 集成插件) | 3.1, 3.2 |
| 3.4 (容器自检) | 3.2 |
| 3.5 (config_loader) | — |
| 4.1 (/chat) | 2.2 |
| 4.2 (/resume) | 2.4 |
| 4.3 (/confirm) | 2.3 |
| 4.4 (SSE 工具) | 1.2 |
| 4.5 (/api/agent/test) | 3.5 |
| 5.1 (demo) | 1.3, 2.x, 3.x, 4.x, 5.2 |
| 5.2 (OpenList 契约) | — |
| 6.1 (schema 加 agent_settings) | — |
| 6.2 (Settings.vue 入口) | 6.1 |
| 6.3 (AgentSettingsDetail.vue) | 6.1, 6.2 |
| 6.4 (后端 /api/agent/test) | 3.5, 4.5 |
| 6.5 (路由注册) | 6.3 |
| 6.6 (i18n) | 6.1, 6.3 |
| 7.1 (useAgent) | — (前端独立) |
| 7.2 (原子组件) | — |
| 7.3 (复合组件) | 7.2 |
| 7.4 (User/Markdown) | 7.2 |
| 7.5 (虚拟化/顶层) | 7.2, 7.3, 7.4, 7.1 |
| 7.6 (首页入口) | 7.5 |
| 8.1 (联调) | 5.1, 5.2, 6.x, 7.6 |
| 8.2 (单测) | 全部 |
| 8.3 (文档) | 8.1 |

# 可并行任务

- **Phase 1** (1.1/1.2/1.3) 与 **Phase 7.1** (useAgent) 可完全并行
- **Phase 3** (plugin scanner / handler / demo 集成) 与 **Phase 6** (Settings UI) 可并行（都依赖 Phase 1.3）
- **Phase 7.2 / 7.3 / 7.4** 三个组件子阶段可内部并行（不同文件）
- **Phase 2**（Agent 核心）与 **Phase 7.2/7.3**（UI 原子/复合组件）可并行

---

# 估算

- Phase 1: 0.5 天（types + registry）
- Phase 2: 2.5 天（Agent 核心 + 4-决策 + session grants + OpenAI 集成）
- Phase 3: 1.5 天（plugin_scanner + handler adapter + demo 集成 + config_loader）
- Phase 4: 0.5 天（HTTP/SSE + /api/agent/test）
- Phase 5: 0.5 天（demo + OpenList 契约文档）
- Phase 6: 1.5 天（schema 加 agent_settings + Settings.vue 入口 + AgentSettingsDetail.vue + 后端 test handler + 路由 + i18n）
- Phase 7: 2 天（Vue 渲染壳：原子组件 0.5d + 复合组件 0.5d + User/Markdown 0.5d + 虚拟化/顶层 0.5d）
- Phase 8: 1.5 天（联调 + 测试 + 文档 + plugin 集成文档）

**总计：~10.5 天**（比原估算 +3.5 天，因新增 plugin 系统适配 12 工具 + agent_settings schema + Settings 二级页 + /api/agent/test + PLUGIN_INTEGRATION 文档）
