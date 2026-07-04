# codex_web 差距分析 + 后续任务规划

## Why

`go-in-process-agent` spec 已完成设计（spec.md 945 行 + tasks.md 401 行 + checklist.md 333 行），但实际编码只完成 5% 关键路径：
- ✅ Go agent 库骨架（types/registry/agent/openai/http）+ OpenList 客户端
- ✅ Vue 基础组件（14 个全部创建）
- ✅ AI 单轮对话（OpenAI 兼容 API + SSE 流 + Markdown 渲染）
- ❌ **未实现**：ApprovalCard 4 决策流程、工具调用执行、插件工具桥接、OpenList 真实联调、断点续传、虚拟列表、4 决策撤销

本 spec **不是新增功能**，而是把 `/spec` 阶段已经规划但尚未落地的需求拆分成可执行的实施任务，让 spec 从"文档"变成"代码"。

## What Changes

按依赖顺序和价值密度，把 `go-in-process-agent/tasks.md` 中**未完成项**拆成 7 个 phase。每个 phase 都有 user-visible 产出（不是内部重构）。

### Phase A: 工具调用最小闭环（最关键，让 AI 真正能做事）  ✅ 已完成（agent library）

- 后端：实现 `/api/chat` 解析 OpenAI 返回的 `tool_calls`，路由到注册表执行
- 后端：实现 4 决策 ConfirmTool + `/api/confirm`（accept / accept_for_session / decline / cancel）
- 前端：`useAgent.confirmTool` 实现 + `ApprovalCard` 4 按钮事件绑定
- 前端：tool_call 事件渲染 ApprovalCard
- 前端：tool_result 事件渲染到消息列表

### Phase B: 插件工具桥接（让 AI 真正操作文件）  ✅ 已完成（encv-go 主后端）

- 后端：`scanPluginTools` + `makePluginEncryptHandler` + `makePluginDecryptHandler`（spec 写完但未实现）
- 后端：plugin 12 个工具自动注册到 demo
- 前端：UI 提示"已运行 video_encrypt"等

### Phase C: OpenList 真实联调（让 AI 看到远端文件）  ⛔ SKIPPED（用户决策）

- 后端：OpenListClient 8 个端点真实调通（list_files / read_file / write_file / delete_file / rename / exec_command / get_storage_info / search_files）
- 后端：把 OpenList 端点作为 8 个 readOnly/fileChange/command 工具注册
- 前端：AI 输入"列文件"→ 真实调 OpenList → 文件列表展示

### Phase D: 断点续传（生产可用性）  ✅ 后端完成 / ⏳ 前端未开始

- 后端：`/api/resume` + `agent.Resume(sessionID, offset)` 实现（spec 有，代码无）
- 前端：`useAgent.resume()` mount 时从 localStorage 自动恢复
- 前端：刷新页面 5 秒内追平进度

### Phase E: 长会话虚拟化（性能）  ⏳ 未开始

- 前端：实现 `renderTurnItems` 累积 + `MessageVirtualList` vue-virtual-scroller 集成
- 前端：阈值判断 `messages.length > 120`

### Phase F: 4 决策完整文案（UX 收口）  ✅ 后端白名单完成 / ⏳ 前端未开始

- 后端：decision 字段白名单校验（拒绝非 4 值）
- 前端：4 决策按钮文案 + i18n（批准 / 本轮批准 / 拒绝 / 拒绝并停止）
- 前端：按钮点击"处理中"状态 + 禁用其他按钮

### Phase G: 端到端联调 + 测试（验收）  ⏳ 未开始

- `ecosystem.config.cjs` 加 agent-demo
- preview-gateway `/agent-api/*` upstream 配通
- 19 个验收场景（spec Phase 8.1.1-19）逐项跑通
- `go test -race ./agent/...` + `pnpm test` 全绿

---

## Impact

- Affected specs: `go-in-process-agent`（本 spec 是它的实施分片）
- Affected code:
  - `/workspace/agent/*.go`（Phase A/C/D）
  - `/workspace/internal/server/agent_api.go`（Phase A 集成）
  - `/workspace/app/encv-mobile/src/composables/useAgent.ts`（Phase A/D/F）
  - `/workspace/app/encv-mobile/src/components/agent/*.vue`（Phase A/B/E/F）
  - `/workspace/app/encv-mobile/src/views/AgentChat.vue`（Phase A/B/E）
  - `/workspace/app/preview-gateway/src/server.ts`（Phase G）

## ADDED Requirements

### Requirement: 工具调用最小闭环

后端 SHALL 解析 OpenAI `tool_calls` 字段并路由到注册表执行，前端 SHALL 渲染 ApprovalCard 收集 4 决策。

#### Scenario: OpenAI 返回 tool_calls → 后端路由执行
- **WHEN** OpenAI 流式响应包含 `delta.ToolCalls` 数组
- **THEN** agent 累积 `tool_calls`，流结束时一次性发送 tool_call 事件
- **AND** `registry.Get(name)` 找到 → 同步执行 → 推 `EventToolResult`
- **AND** `registry.Get(name)` 未找到 → 推 `EventToolResult{IsError: true, Result: "{\"error\":\"tool_not_found\"}"}`

#### Scenario: needConfirm=true → 挂起等前端决策
- **WHEN** tool_call 的 `def.NeedConfirm == true` 且 `(toolName, sessionID)` 不在 `sessionGrants`
- **THEN** 推 `EventToolCall` (AutoRun=false) + `EventStreamEnd`
- **AND** 不执行 Handler，不追加 tool_result

#### Scenario: 前端 4 决策 confirm
- **WHEN** 用户点击 ApprovalCard 4 按钮之一
- **THEN** `useAgent.confirmTool(toolCallId, decision)` fetch POST `/api/confirm` body `{sessionId, toolCallId, decision}`
- **AND** `decision` ∈ `accept` / `accept_for_session` / `decline` / `cancel`（白名单）
- **AND** 按钮点击后立即显示"处理中" + 禁用其他 3 个按钮

#### Scenario: accept 决策恢复执行
- **WHEN** 后端收到 decision=accept
- **THEN** 执行 Handler → 推 EventToolResult → 递归下一轮 LLM
- **AND** 返回 SSE 流到前端

#### Scenario: accept_for_session 写 sessionGrants
- **WHEN** decision=accept_for_session
- **THEN** **同时**执行 Handler **且**写 `sessionGrants[(toolName, sessionID)] = true`
- **AND** 后续同类 tool_call 自动通过，不弹 ApprovalCard

#### Scenario: decline 不执行 + 推 cancelled + 递归
- **WHEN** decision=decline
- **THEN** 不执行 Handler
- **AND** 推 `EventToolResult{Status: cancelled, IsError: true, Result: "{\"error\":\"user_rejected\"}"}`
- **AND** 递归下一轮 LLM（让 LLM 知道用户拒绝）

#### Scenario: cancel 不执行 + 推 cancelled + 推 StreamEnd + 不递归
- **WHEN** decision=cancel
- **THEN** 不执行 Handler
- **AND** 推 EventToolResult(cancelled) + EventStreamEnd
- **AND** **不递归**（本轮结束）

### Requirement: 插件工具桥接

后端 SHALL 自动把 7 个 ENCV 插件（video/audio/image/wps/pdf/text/alistencrypt）扫描为 12 个 agent 工具（alistencrypt 跳过）。

#### Scenario: scanPluginTools 产出 12 个工具
- **WHEN** `agent-demo` 启动
- **THEN** 遍历 `plugins.Plugins`（7 个）→ 每个插件产出 2 个 tool（`{name}_encrypt` + `{name}_decrypt`）
- **AND** 跳过 alistencrypt（OpenList 端已暴露）
- **AND** 最终 6 插件 × 2 = 12 个工具

#### Scenario: 工具 schema 动态生成
- **WHEN** 生成 `{plugin}_encrypt` 工具 schema
- **THEN** 包含字段：
  - `input_paths: array<string>` required
  - `output_path: string` required
  - `extra_fields: object` 来自 `plugin.GetTaskOptions().ExtraFields`
  - `password: string` 按 `PasswordStrategy` 决定 required/optional/hidden
  - `version: integer` 按 `SupportVersionSelect` 决定 required/hidden

#### Scenario: encrypt handler 执行加密
- **WHEN** LLM 调用 `video_encrypt` 带 args
- **THEN** handler 解析 args → `plugin.SetTaskExtraFields` → `PreEncryptProcessor` → `plugin.Encrypt(reader)` → `PostEncryptProcessor`
- **AND** 返回 `{output_path, duration_ms, file_size}` JSON

#### Scenario: 容器自检失败
- **WHEN** LLM 调用 `video_decrypt(path)` 但 path 指向 `.pdf`
- **THEN** handler 调 `plugin.CanDecrypt(path)` 返回 false
- **AND** 包装为 `{error: "container_format_mismatch", suggested_tool: "pdf_decrypt"}` + IsError=true

### Requirement: OpenList 真实联调

后端 SHALL 把 OpenList 8 个端点（`/api/ext/*`）注册为 agent 工具，前端 SHALL 真实调通。

#### Scenario: OpenListClient 8 个端点
- **WHEN** agent-demo 启动
- **THEN** 注册工具：
  - `list_files` (readOnly, auto-run) → `POST /api/ext/list_files`
  - `read_file` (readOnly, auto-run) → `POST /api/ext/read_file`
  - `write_file` (fileChange, need confirm) → `POST /api/ext/write_file`
  - `delete_file` (fileChange, need confirm) → `POST /api/ext/delete_file`
  - `rename` (fileChange, need confirm) → `POST /api/ext/rename`
  - `exec_command` (command, need confirm) → `POST /api/ext/exec_command`
  - `get_storage_info` (readOnly, auto-run) → `POST /api/ext/get_storage_info`
  - `search_files` (readOnly, auto-run) → `POST /api/ext/search_files`

#### Scenario: 真实端到端列文件
- **WHEN** 用户在 AgentChat 输入 "列文件"
- **THEN** LLM 调 `list_files({path: "/"})` → OpenList 真实返回文件列表
- **AND** UI 展示「✅ list_files: 5 个文件」+ 文件名列表

### Requirement: 断点续传

#### Scenario: 刷新页面自动恢复
- **WHEN** 用户在流式输出中刷新浏览器
- **THEN** 组件 mount → `useAgent.resume()` 读 localStorage `agent:session:{sessionId}` 拿 `eventOffset`
- **AND** fetch POST `/api/resume` body `{sessionId, eventOffset}`
- **AND** 后端从 `cache.Events[offset]` 开始重放，5 秒内追平进度

#### Scenario: 会话不存在
- **WHEN** 后端 `sessions` map 中无此 sessionId
- **THEN** 返回 error，前端应清空 localStorage 并重置

### Requirement: 长会话虚拟化

#### Scenario: 超过 120 条消息触发虚拟列表
- **WHEN** `messages.length > 120`
- **THEN** 使用 `<MessageVirtualList :messages="..." :estimateSize="112" :overscan="12" />`
- **AND** 底层 vue-virtual-scroller `<RecycleScroller>`，DOM 中只渲染 ~20 个 message 节点

#### Scenario: renderTurnItems 分组
- **WHEN** 渲染消息列表
- **THEN** 累积相邻 `command` / `fileChange` / `toolOutput` 到 `operationGroup`
- **AND** 累积相邻 `webSearch` 到 `webSearchGroup`
- **AND** 遇到非操作 / 流结束时 flush group 渲染为 `<GroupedOperationMessage>`

### Requirement: 4 决策完整 UX

#### Scenario: 按钮文案
- **WHEN** 渲染 ApprovalCard
- **THEN** 4 按钮文案（中英）：
  - 批准 = `modals.approve` = "批准"
  - 本轮批准 = `modals.approveForSession` = "本轮批准"（仅 `kind !== readOnly` 时显示）
  - 拒绝 = `modals.decline` = "拒绝"
  - 拒绝并停止 = `modals.cancel` = "拒绝并停止"

#### Scenario: 按钮处理中态
- **WHEN** 用户点击任一按钮
- **THEN** 该按钮显示 spinner + 禁用其他 3 个
- **AND** 等 SSE 流返回后按钮恢复

### Requirement: 端到端联调验收

#### Scenario: 19 个验收场景
- **THEN** 完整跑通 spec Phase 8.1.1-19 全部场景
- **AND** 0 个 console error + 0 个 SSE 解析失败
- **AND** OpenList 故障隔离（OpenList 崩溃不影响 agent）
- **AND** `go test -race ./agent/...` + `pnpm test` 全绿

## REMOVED Requirements

无。`go-in-process-agent` spec 继续作为 source of truth，本 spec 是它的实施分片。

## 约束

1. **不修改 spec 文档**：本 spec 不替代 `go-in-process-agent/spec.md`，是它的子集实施计划
2. **Phase 顺序强依赖**：A → B/C 可并行 → D → E → F → G
3. **每个 Phase 必须可独立验证**：user-visible 产出 + 测试通过
4. **UI 与 codex_web 1:1 对齐**：组件命名、props、class、状态文案严格保持一致
5. **集成入口**（铁律）：AI 入口只在 encv-mobile 主应用首页（`Home.vue` 浮动按钮），不走路由
6. **OpenList 集成边界**（铁律）：OpenList 不集成 agent 库，只暴露 `/api/ext/*` 端点
