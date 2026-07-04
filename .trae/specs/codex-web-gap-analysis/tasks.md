# Tasks: codex_web 差距实施分片

> **本文件是 `go-in-process-agent/tasks.md` 的实施切片**，按 Phase 顺序细化到可验证单元。每个 task 都有 user-visible 产出 + verify 步骤。
> 实施顺序：A → B/C（并行）→ D → E → F → G

---

## Phase A: 工具调用最小闭环（最关键，让 AI 真正能做事）

### Task A.1: 后端解析 OpenAI tool_calls

- [ ] **A.1.1**: 在 `agent/openai.go` 的 `createChatCompletionStream` 累积 `delta.ToolCalls` → 写入 `accumulatedToolCalls []openai.ToolCall`
- [ ] **A.1.2**: 流结束时若有 tool_calls → 调 `agent.executeToolCalls(messages, toolCalls, sessionID)` → 推 `EventToolCall` × N
- [ ] **A.1.3**: `executeToolCalls` 按 tool_name 查 registry，**自动执行**（NeedConfirm=false）或**挂起**（NeedConfirm=true 且 session 未放行）
- [ ] **A.1.4**: 自动执行路径：同步跑 Handler → 推 `EventToolStatus{running}` → 推 `EventToolResult{Status: success|failed}` → 追加 tool_result 到 messages → **递归**调 OpenAI 下一轮
- [ ] **A.1.5**: 挂起路径：推 `EventToolCall` (AutoRun=false) → **不执行** Handler → **不追加** tool_result → 推 `EventStreamEnd` 结束当前流
- [ ] **A.1.6**: **verify**: `agent_test.go` 新增 `TestChat_ToolCall_AutoRun` 和 `TestChat_ToolCall_NeedConfirm_Hold` — mock OpenAI server 返回 `tool_calls` → 验证 channel 收到正确事件序列

### Task A.2: 后端 ConfirmTool + /api/confirm 4 决策

- [ ] **A.2.1**: 实现 `agent.ConfirmTool(sessionID, toolCallID, decision string) (<-chan, error)` — 4 决策分发：
  - `accept` → 执行 → 推 ToolResult → 递归
  - `accept_for_session` → **同时**执行 **且**写 `sessionGrants[(toolName, sessionID)] = true` → 递归
  - `decline` → **不执行** → 推 ToolResult(cancelled, IsError=true, Result=`{"error":"user_rejected"}`) → 递归
  - `cancel` → **不执行** → 推 ToolResult(cancelled) + 推 StreamEnd → **不递归**
- [ ] **A.2.2**: 实现 `HandleConfirm(w, r)` in `agent/http.go` — 解析 body `{sessionId, toolCallId, decision}` → **白名单校验** decision（拒绝非 4 值）→ 调 ConfirmTool → 写 SSE
- [ ] **A.2.3**: 暴露 `/api/confirm` 路由到 `internal/server/agent_api.go` (在 `registerAgentRoutes` 加 `r.POST("/api/confirm", ...)`)
- [ ] **A.2.4**: **verify**: `agent_test.go` 新增 `TestConfirmTool_4_Decisions` — 4 决策各跑一次，断言：
  - `accept` → 1 ToolResult + 1 次递归（再次调 OpenAI）
  - `accept_for_session` → sessionGrants map 包含 key
  - `decline` → ToolResult(cancelled, IsError=true)
  - `cancel` → StreamEnd 且无递归

### Task A.3: 前端 useAgent.confirmTool

- [ ] **A.3.1**: 在 `app/encv-mobile/src/composables/useAgent.ts` 实现 `confirmTool(toolCallId: string, decision: Decision)` 函数
- [ ] **A.3.2**: 函数内部：fetch POST `/api/chat` 改为 body `{sessionId, toolCallId, decision}` → 路由到 `/api/confirm` → 复用 `processSSE` 解析响应
- [ ] **A.3.3**: 状态机：`status = 'streaming'` → 等待 SSE 结束 → `status = 'idle'`
- [ ] **A.3.4**: **verify**: `useAgent.test.ts` 新增 4 个测试覆盖 4 决策 → mock fetch 返回 SSE → 断言状态机 + 事件分发

### Task A.4: 前端 ApprovalCard 事件绑定

- [ ] **A.4.1**: 在 `app/encv-mobile/src/views/AgentChat.vue` 引入 `ApprovalCard` 组件
- [ ] **A.4.2**: 收到 `tool_call` 事件时，定位到对应 assistant 消息，渲染 `<ApprovalCard :toolCall="..." :onDecide="confirmTool" />`
- [ ] **A.4.3**: ApprovalCard 内 4 按钮 click → 调 `onDecide(toolCallId, 'accept' | 'accept_for_session' | 'decline' | 'cancel')`
- [ ] **A.4.4**: 收到 `tool_result` 事件时，定位到对应 tool_call，渲染结果（成功绿色 / 失败红色 / 取消灰色）
- [ ] **A.4.5**: **verify**: 浏览器跑通：输入"用 video 插件加密 foo.mp4" → 看到 ApprovalCard 4 按钮 → 点击"批准" → 真实产生 .encv 容器

---

## Phase B: 插件工具桥接（让 AI 真正操作文件）

> **状态（2026-06-06）**：✅ 已完成。实施位置 `/workspace/internal/server/agent_plugin_bridge.go`（12 个工具元信息 + executePluginTool + runPluginEncrypt/Decrypt）+ `/workspace/internal/server/agent_plugin_bridge_test.go`（17 个单测全过）。
> 极简版取舍：不实现 ExtraFields/PasswordStrategy/SupportVersionSelect 动态 schema（spec 写得很细，本阶段先用固定 input_path+output_dir schema）。

### Task B.1: agent plugin_scanner.go

- [x] **B.1.1**: 实现 `scanPluginTools(plugins []Plugin) []ToolDefinition` — 遍历 plugins，跳过 alistencrypt，每个产 2 个 tool
  - **实际**：`pluginOpsByName` + `ListPluginTools` 在 `agent_plugin_bridge.go` 实现
- [x] **B.1.2**: 工具命名：`{PluginName()}_encrypt` / `{PluginName()}_decrypt`
- [⚠️] **B.1.3**: 工具 schema 动态生成：input_paths / output_path / extra_fields / password (按 PasswordStrategy) / version (按 SupportVersionSelect)
  - **实际**：简化为 `input_path+output_dir`（encrypt）/ `container_path+output_dir`（decrypt）。ExtraFields 后续 phase 扩展
- [x] **B.1.4**: 工具 description 用中文：`video_encrypt`: "使用 video 插件将视频文件加密为 .encv 容器"
- [x] **B.1.5**: **verify**: `plugin_scanner_test.go` mock 7 个插件 → 验证产出 12 个 + 字段正确
  - **实际**：`TestListPluginTools_Contains12Tools` + `TestListPluginTools_NoDuplicateNames` + `TestListPluginTools_SkipsAlistencrypt` 在 `agent_plugin_bridge_test.go`

### Task B.2: agent plugin_tool_handler.go

- [x] **B.2.1**: 实现 `makePluginEncryptHandler(plugin Plugin) func(string) (string, error)` — 流程：
  1. 解析 args JSON
  2. `plugin.SetTaskExtraFields(extraFields)`（N/A：本阶段未实现 extraFields 透传）
  3. `plugin.PreEncryptProcessor(index, inputPath, inputRoot, outputDir)`（封装在 `plugins.EncryptFileWithPlugin` 内）
  4. 打开 inputPath → `plugin.Encrypt(reader)` → `*crypto.EncryptionResult`
  5. `plugin.PostEncryptProcessor(result)` → 拿到 output_path
  6. 返回 `{output_path, duration_ms, file_size}` JSON
  - **实际**：`runPluginEncrypt` 调 `plugins.FindEncryptingPlugin` + `plugins.EncryptFileWithPlugin`，返回 `{plugin, op, input, output}` JSON
- [x] **B.2.2**: 实现 `makePluginDecryptHandler` 对称（CanDecrypt 自检 + Decrypt 流程）
  - **实际**：`runPluginDecrypt` 调 `plugins.FindDecryptingPlugin` + `plugins.DecryptContainerWithPlugin`
- [x] **B.2.3**: **verify**: `plugin_tool_handler_test.go` mock plugin → 验证 handler 流程
  - **实际**：`TestExecutePluginTool_UnknownTool` + `TestExecutePluginTool_InvalidArgsJSON` + `TestExecutePluginTool_MissingArgs`

### Task B.3: agent-demo 集成插件工具

- [x] **B.3.1**: 在 `agent/cmd/agent-demo/main.go` 调 `scanPluginTools(plugins.Plugins)` 注册 12 个插件工具
  - **实际**：encv-go 主后端 `internal/server/agent_plugin_bridge.go` 在 package init 时构建 `pluginOpsByName`，按需通过 `executePluginTool` 路由
- [⚠️] **B.3.2**: 读 `agent_settings.enabled_tools` 白名单过滤
  - **实际**：未实现（spec 提到但本阶段没做，后续 phase 加）
- [x] **B.3.3**: **verify**: demo 启动后 `curl /api/chat` 返回的工具列表包含 12 个 `*_encrypt` / `*_decrypt`
  - **实际**：`TestListPluginTools_Contains12Tools` 验证工具数量和命名

---

## Phase C: OpenList 真实联调 — ⛔ SKIPPED（用户决策）

> **状态（2026-06-06）**：⛔ **整体跳过**。用户明确指令"openlist不做，重点是encv自己的能力"（见对话历史）。
> 后续如需恢复，可单独提需求，本 spec 保持 SKIPPED 标记。

### Task C.1: OpenListClient 8 端点真实调通

- [⛔] **C.1.1-1.4**: **SKIPPED** — OpenList 8 端点不实施

### Task C.2: agent-demo 集成 OpenList 工具

- [⛔] **C.2.1-2.2**: **SKIPPED** — 8 个 OpenList 工具不注册

### Task C.3: 端到端 OpenList 联调

- [⛔] **C.3.1-3.4**: **SKIPPED** — 端到端联调不做

---

## Phase C: OpenList 真实联调

### Task C.1: OpenListClient 8 端点真实调通

- [ ] **C.1.1**: `agent/openlist_client.go` 实现 8 个方法（`ListFiles` / `ReadFile` / `WriteFile` / `DeleteFiles` / `Rename` / `ExecCommand` / `GetStorageInfo` / `SearchFiles`）
- [ ] **C.1.2**: 每个方法调 `POST OPENLIST_BASE_URL/api/ext/{name}` 携带 `Authorization: Bearer ${OPENLIST_TOKEN}`
- [ ] **C.1.3**: 错误处理：4xx/5xx → 返回包装 error
- [ ] **C.1.4**: **verify**: `openlist_client_test.go` mock OpenList server → 8 个端点 round-trip

### Task C.2: agent-demo 集成 OpenList 工具

- [ ] **C.2.1**: demo 注册 8 个 OpenList 工具：
  - `list_files` (readOnly, auto-run)
  - `read_file` (readOnly, auto-run)
  - `get_storage_info` (readOnly, auto-run)
  - `search_files` (readOnly, auto-run)
  - `write_file` (fileChange, need confirm)
  - `delete_file` (fileChange, need confirm)
  - `rename` (fileChange, need confirm)
  - `exec_command` (command, need confirm)
- [ ] **C.2.2**: **verify**: demo 启动 → 工具列表共 20 个（12 插件 + 8 OpenList）

### Task C.3: 端到端 OpenList 联调

- [ ] **C.3.1**: OpenList 服务跑起来（用 Hi-Sillot/OpenList 真实环境或 mock）
- [ ] **C.3.2**: 配置 `OPENLIST_BASE_URL` + `OPENLIST_TOKEN` 到 `config.user.json`
- [ ] **C.3.3**: 浏览器 AgentChat 输入"列文件" → AI 调 `list_files` → 真实 OpenList 返回 → UI 展示文件列表
- [ ] **C.3.4**: **verify**: 文件名展示在 UI 上 + 控制台 `tool_result` 日志

---

## Phase D: 断点续传

> **状态（2026-06-06）**：后端 ✅ 已完成；前端 ⏳ 未开始。
> 实施位置：`/workspace/internal/server/agent_api.go` (handleAgentResume + sendAndCache + AgentEvent)
> + `/workspace/internal/server/agent_tool_loop.go` (agentSession.EventCache/eventIDCounter/InProgress)
> + `/workspace/internal/server/agent_plugin_bridge_test.go` (10 个 D 阶段单测全过)
> **设计取舍**：本阶段不实现"chat 期间前端断线后立即 resume 追到最新事件"（需要 pub/sub 实时推送），
> 而是采用"polling 模型"：Resume 拉取 lastEventID 之后所有事件，InProgress=true 时返回 stream_status=synced 让前端继续轮询。

### Task D.1: 后端 /api/resume

- [x] **D.1.1**: 实现事件缓存 `agentSession.EventCache []AgentEvent`（按 ID 升序）
- [x] **D.1.2**: 实现 `sendAndCache(sess, w, flusher, type, data)` — 写 SSE 同时入 EventCache，ID 单调递增
- [x] **D.1.3**: 实现 `handleAgentResume` 读 lastEventID（body 或 Last-Event-ID header）→ 重放 ID > lastEventID 的事件
- [x] **D.1.4**: 状态同步：inProgress=true → 返回 stream_status=synced；inProgress=false + 最后事件是 stream_end → 不重复推
- [x] **D.1.5**: **verify**: `TestHandleAgentResume_HTTP_ReplayEvents` / `TestHandleAgentResume_HTTP_Header_LastEventID` / `TestHandleAgentResume_HTTP_NotFound` / `TestHandleAgentResume_HTTP_InProgress_Synced` 全过

### Task D.2: 前端 useAgent.resume

- [⏳] **D.2.1-D.2.4**: **未开始** — 需前端 useAgent 集成 Last-Event-ID 协议

---

## Phase E: 长会话虚拟化

> **状态**：⏳ 未开始。本轮跳过（用户优先推进 B/D/F 收尾）。

### Task E.1: MessageVirtualList 集成 vue-virtual-scroller

- [ ] **E.1.1-1.4**: 未开始

### Task E.2: renderTurnItems 分组

- [ ] **E.2.1-2.4**: 未开始

---

## Phase F: 4 决策完整 UX

> **状态（2026-06-06）**：后端 ✅ 已完成；前端 ⏳ 未开始。
> 实施位置：`/workspace/internal/server/agent_api.go` (handleAgentConfirm 决策白名单 + GrantedTools)
> + `/workspace/internal/server/agent_tool_loop.go` (agentSession.GrantedTools map)
> + `/workspace/internal/server/agent_plugin_bridge.go` (emitToolCallEvent 检查 GrantedTools → auto_run)
> + `/workspace/internal/server/agent_plugin_bridge_test.go` (6 个 F 阶段单测全过)

### Task F.1: 4 决策按钮文案

- [⏳] **F.1.1-1.2**: **未开始** — 前端 i18n 待添加

### Task F.2: 按钮处理中态

- [⏳] **F.2.1-2.3**: **未开始** — 前端 decisionLoading 状态待实现

### Task F.3: 后端 decision 白名单

- [x] **F.3.1**: `handleAgentConfirm` 决策白名单 ∈ {`accept`, `accept_for_session`, `decline`, `cancel`} — 400 拒绝其他
- [x] **F.3.2**: `accept_for_session` 决策：执行工具 + 写入 `sess.GrantedTools[toolName] = true`
- [x] **F.3.3**: `emitToolCallEvent` 检查 `sess.GrantedTools` → `auto_run=true` / `needsConfirm=false` 推给前端
- [x] **F.3.4**: **verify**: `TestHandleAgentConfirm_HTTP_InvalidDecision` / `TestHandleAgentConfirm_HTTP_CancelDecision` / `TestEmitToolCallEvent_*` / `TestGrantedTools_*` 全过

---

## Phase G: 端到端联调 + 测试

### Task G.1: 沙箱 dev 三进程联调

- [ ] **G.1.1**: `ecosystem.config.cjs` 加 `agent-demo` app
- [ ] **G.1.2**: `pm2 save` 持久化
- [ ] **G.1.3**: preview-gateway `/agent-api/*` upstream → `127.0.0.1:5245`

### Task G.2: 19 个验收场景

- [ ] **G.2.1**: spec Phase 8.1.1-19 全部跑通：
  - 输入"列文件" → 真实 OpenList 文件列表
  - 输入"删除 foo.txt" → ApprovalCard 4 按钮 → 4 个决策各跑一次
  - 流式刷新 → resume 5 秒内追平
  - 130 条消息 → 虚拟列表生效
  - "用 video 插件加密" → 真实产生 .encv
  - OpenList 崩溃 → agent 优雅降级
  - 0 个 console error
- [ ] **G.2.2**: **verify**: 完整跑通

### Task G.3: 单元测试

- [ ] **G.3.1**: `go test -race ./agent/...` 全绿
- [ ] **G.3.2**: `pnpm test` vitest 全绿（含 useAgent 4 决策、ApprovalCard 4 按钮、MessageVirtualList、renderTurnItems）
- [ ] **G.3.3**: 覆盖率 Go ≥ 70% + TS ≥ 70%

---

# Task Dependencies

| Phase / Task | Depends on |
|--------------|-----------|
| A.1 (后端 tool_call 解析) | go-in-process-agent Phase 1-2 (已完成) |
| A.2 (后端 ConfirmTool) | A.1 |
| A.3 (前端 confirmTool) | A.2 |
| A.4 (前端 ApprovalCard 渲染) | A.3 |
| B.1 (plugin_scanner) | go-in-process-agent Phase 1.3 (已完成) |
| B.2 (plugin_tool_handler) | B.1 |
| B.3 (demo 集成插件) | B.1, B.2 |
| C.1 (OpenListClient) | go-in-process-agent Phase 5.2 (已完成) |
| C.2 (demo 集成 OpenList) | C.1 |
| C.3 (端到端联调) | C.2 + OpenList 服务跑通 |
| D.1 (后端 /api/resume) | go-in-process-agent Phase 2.4 (已完成) |
| D.2 (前端 resume) | D.1 |
| E.1 (MessageVirtualList) | go-in-process-agent Phase 7.6 (包已存在) |
| E.2 (renderTurnItems) | E.1 |
| F.1-F.3 (4 决策 UX) | A.4 |
| G.1 (三进程) | A 全部 + B 全部 + C 全部 + D 全部 |
| G.2 (19 验收) | G.1 + E 全部 + F 全部 |
| G.3 (单测) | 全部 |

# 可并行任务

- **Phase B**（plugin_scanner / handler / demo 集成）**与 Phase C**（OpenList 联调）**完全并行**（不同模块）
- **Phase E**（虚拟化）**与 Phase F**（4 决策 UX）**可并行**
- Phase A 必须先于 B/C/D/E/F/G

# 估算

- Phase A: 2 天（核心闭环，最难）
- Phase B: 1 天（已有 spec，机械实现）
- Phase C: 1.5 天（依赖 OpenList 服务）
- Phase D: 0.5 天
- Phase E: 0.5 天
- Phase F: 0.5 天
- Phase G: 1.5 天（联调+测试）

**总计：~7.5 天**
