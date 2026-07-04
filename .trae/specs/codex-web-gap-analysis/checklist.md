# Checklist: codex_web 差距实施分片

> 每个 checkpoint 都要勾选才能算本 spec 完成。
> **核心原则**：本 spec 不是替代 `go-in-process-agent` spec，而是把它从"文档"推进到"代码"的实施分片。

---

## Phase A: 工具调用最小闭环

### 后端 OpenAI tool_calls 解析

- [ ] `agent/openai.go` 累积 `delta.ToolCalls` 到 `accumulatedToolCalls`
- [ ] 流结束时若有 tool_calls → 调 `executeToolCalls`
- [ ] `executeToolCalls` 按 NeedConfirm 分发：auto-run / 挂起
- [ ] auto-run：推 ToolStatus(running) → 执行 Handler → 推 ToolResult → 递归下一轮 LLM
- [ ] 挂起：推 EventToolCall (AutoRun=false) + EventStreamEnd（不执行 / 不递归）
- [ ] `agent_test.go` `TestChat_ToolCall_AutoRun` 通过
- [ ] `agent_test.go` `TestChat_ToolCall_NeedConfirm_Hold` 通过

### 后端 ConfirmTool + /api/confirm 4 决策

- [ ] `agent.ConfirmTool(sessionID, toolCallID, decision)` 实现
- [ ] `accept` 路径：执行 → 推 ToolResult → 递归
- [ ] `accept_for_session` 路径：执行 **且**写 sessionGrants → 递归
- [ ] `decline` 路径：推 ToolResult(cancelled, IsError=true) → 递归
- [ ] `cancel` 路径：推 ToolResult(cancelled) + 推 StreamEnd + **不递归**
- [ ] `HandleConfirm(w, r)` 实现 + decision 白名单校验（拒绝非 4 值返回 400）
- [ ] `/api/confirm` 路由挂载到 `internal/server/agent_api.go`
- [ ] `agent_test.go` `TestConfirmTool_4_Decisions` 通过

### 前端 useAgent.confirmTool

- [ ] `useAgent.confirmTool(toolCallId, decision)` 实现
- [ ] fetch POST `/api/confirm` body `{sessionId, toolCallId, decision}`
- [ ] 状态机：`status='streaming'` → 等 SSE → `status='idle'`
- [ ] `useAgent.test.ts` 4 个测试覆盖 4 决策

### 前端 ApprovalCard 事件绑定

- [ ] AgentChat.vue 引入 ApprovalCard
- [ ] 收到 tool_call 事件 → 渲染 `<ApprovalCard :toolCall="..." :onDecide="confirmTool" />`
- [ ] 4 按钮 click → 调 `onDecide(toolCallId, decision)`
- [ ] 收到 tool_result 事件 → 渲染结果（成功/失败/取消）
- [ ] 浏览器跑通：输入"用 video 插件加密 foo.mp4" → 看到 ApprovalCard → 批准 → 产生 .encv

---

## Phase B: 插件工具桥接

### plugin_scanner.go

- [ ] `scanPluginTools(plugins []Plugin) []ToolDefinition` 实现
- [ ] 7 插件 → 12 工具（跳过 alistencrypt）
- [ ] 工具命名：`{name}_encrypt` / `{name}_decrypt`
- [ ] schema 动态生成：input_paths / output_path / extra_fields / password / version
- [ ] description 用中文
- [ ] `plugin_scanner_test.go` 通过

### plugin_tool_handler.go

- [ ] `makePluginEncryptHandler(plugin)` 实现
- [ ] encrypt 流程：SetTaskExtraFields → PreEncryptProcessor → Encrypt(reader) → PostEncryptProcessor
- [ ] `makePluginDecryptHandler` 对称实现
- [ ] CanDecrypt 自检 + 失败返回 container_format_mismatch 建议
- [ ] `plugin_tool_handler_test.go` 通过

### agent-demo 集成插件工具

- [ ] demo 调 `scanPluginTools` 注册 12 个工具
- [ ] 读 `agent_settings.enabled_tools` 白名单过滤
- [ ] demo 启动后工具列表 = 12 个 `*_encrypt` / `*_decrypt`

---

## Phase C: OpenList 真实联调

### OpenListClient 8 端点

- [ ] `agent/openlist_client.go` 实现 8 个方法（ListFiles / ReadFile / WriteFile / DeleteFiles / Rename / ExecCommand / GetStorageInfo / SearchFiles）
- [ ] 每个方法携带 `Authorization: Bearer ${OPENLIST_TOKEN}`
- [ ] 4xx/5xx → 包装 error
- [ ] `openlist_client_test.go` 8 端点 round-trip 通过

### agent-demo 集成 OpenList 工具

- [ ] demo 注册 8 个工具（readOnly auto-run × 4 + fileChange/command need-confirm × 4）
- [ ] 工具列表 = 12 插件 + 8 OpenList = 20

### 端到端联调

- [ ] OpenList 服务跑起来
- [ ] 配置 OPENLIST_BASE_URL + OPENLIST_TOKEN
- [ ] 输入"列文件" → AI 调 list_files → 真实 OpenList 返回 → UI 展示文件列表

---

## Phase D: 断点续传

### 后端 /api/resume

- [ ] `agent.Resume(sessionID, offset)` 实现
- [ ] 从 `cache.Events[offset:]` 重放到 channel
- [ ] 等待机制：50ms polling
- [ ] 追到 IsFinished → 推 EventStreamEnd
- [ ] `HandleResume(w, r)` 实现
- [ ] `/api/resume` 路由挂载
- [ ] `agent_test.go` `TestResume_Replay` 通过

### 前端 useAgent.resume

- [ ] `useAgent.resume()` 实现
- [ ] localStorage 读 `{sessionId, eventOffset, messages, status}`
- [ ] status==='streaming' → fetch /api/resume → processSSE
- [ ] 组件 mount 时自动调
- [ ] 浏览器跑通：流式中刷新 → 5 秒内追平

---

## Phase E: 长会话虚拟化

### MessageVirtualList 集成

- [ ] `package.json` 加 `vue-virtual-scroller: ^2.0.0-beta.8`
- [ ] `pnpm install` 成功
- [ ] `MessageVirtualList.vue` 封装 `<RecycleScroller>` (itemSize=112, minItemSize=80, buffer=600)
- [ ] 阈值判断：`messages.length > 120` 用虚拟列表

### renderTurnItems 分组

- [ ] `renderTurnItems.ts` 实现
- [ ] 累积 operationGroup (command/fileChange/toolOutput) + webSearchGroup
- [ ] flush 按 group 类型渲染
- [ ] 注入 130 条消息 → DevTools 验证虚拟列表生效（DOM 中 ~20 个节点）

---

## Phase F: 4 决策完整 UX

### 按钮文案

- [ ] i18n key 完整：approve / approveForSession / decline / cancel
- [ ] ApprovalCard 用 i18n 文案

### 按钮处理中态

- [ ] `decisionLoading: ref<Decision | null>(null)`
- [ ] 点击 → 显示 spinner + 禁用其他 3 个
- [ ] SSE 流结束（tool_result）→ 清空 loading

### 后端 decision 白名单

- [ ] `HandleConfirm` 入口校验 decision ∈ 4 值 → 拒绝返回 400

---

## Phase G: 端到端联调 + 测试

### 沙箱三进程

- [ ] `ecosystem.config.cjs` 加 `agent-demo` app
- [ ] `pm2 save` 持久化
- [ ] preview-gateway `/agent-api/*` upstream → :5245

### 19 个验收场景

- [ ] 8.1.1: `curl -N http://localhost:5245/api/chat` SSE 正常流
- [ ] 8.1.2: OpenAI + OpenList 测试连接 OK
- [ ] 8.1.3: 浏览器 encv-mobile 主页 → 浮动 AI 按钮可见
- [ ] 8.1.4: 点击 → modal 弹出 → 全屏 AgentChat
- [ ] 8.1.5: 输入"列文件" → 真实 OpenList 文件列表展示
- [ ] 8.1.6: 输入"删除 foo.txt" → ApprovalCard 4 按钮 → 批准 → 真实 delete_file
- [ ] 8.1.7: 本轮批准 → 第二次同类调用自动执行（无 ApprovalCard）
- [ ] 8.1.8: 拒绝 → ToolResult error（user_rejected），LLM 继续
- [ ] 8.1.9: 拒绝并停止 → 立即收到 stream_end，本轮结束
- [ ] 8.1.10: 流式刷新 → resume 5 秒内追平
- [ ] 8.1.11: 130 条消息 → 虚拟列表生效
- [ ] 8.1.12: 0 个 console error + 0 个 SSE 解析失败
- [ ] 8.1.13: OpenList 崩溃 → agent 优雅降级
- [ ] 8.1.14: Settings → AI 助手 → 修改 openai_api_key → 重启 agent → 新 key 生效
- [ ] 8.1.15: Settings → AI 助手 → 测试连接 → OpenAI ✓ + OpenList ✓
- [ ] 8.1.16: "用 video 插件加密 foo.mp4" → ApprovalCard (video_encrypt) → 批准 → 产生 .encv
- [ ] 8.1.17: "解密 secrets.encv" → ApprovalCard (video_decrypt) → 批准 → 产生明文
- [ ] 8.1.18: 错误用例："用 text 插件加密 foo.mp4" → container_format_mismatch 建议
- [ ] 8.1.19: 工具列表只包含 user 启用的（验证 enabled_tools 白名单）

### 单元测试

- [ ] `go test -race ./agent/...` 全绿
- [ ] `pnpm test` vitest 全绿
- [ ] 覆盖率 Go ≥ 70% + TS ≥ 70%

---

## 实施节奏建议

| 阶段 | 时长 | 价值密度 |
|------|------|---------|
| Phase A | 2d | ⭐⭐⭐⭐⭐（核心闭环） |
| Phase B | 1d | ⭐⭐⭐⭐（让 AI 真正操作文件） |
| Phase C | 1.5d | ⭐⭐⭐（让 AI 看到远端） |
| Phase D | 0.5d | ⭐⭐⭐（生产可用） |
| Phase E | 0.5d | ⭐⭐（性能） |
| Phase F | 0.5d | ⭐⭐（UX 收口） |
| Phase G | 1.5d | ⭐⭐⭐⭐（验收） |
| **总计** | **7.5d** | |

**推荐实施顺序**：A → B+C（并行）→ D → E+F（并行）→ G
