# Tasks

## Phase 1: 后端 AG-UI 事件 shape 升级

- [x] Task 1.1: `AGUIThreadState` 辅助类
  - [x] 1.1.1 `internal/server/agent_agui_adapter.go` 新增 `AGUIThreadState` 结构体：threadId / runId / messageId 计数器
  - [x] 1.1.2 构造时接收 `sessID` → 派生稳定的 `threadId`
  - [x] 1.1.3 `NewRun()` 方法：生成新 `runId`（UUID）
  - [x] 1.1.4 `NextMessageID()` 方法：返回 `msg_<runId>_<seq>`（seq 自增）
  - [x] 1.1.5 `CurrentTimestamp()` 方法：返回 ISO 8601 毫秒时间戳

- [x] Task 1.2: `AGUIEventMapper` 升级事件输出
  - [x] 1.2.1 `NewAGUIMapper` 改为接收 `AGUIThreadState`（不再是裸 sess 字符串）
  - [x] 1.2.2 `MapEvent` 内为每个事件加 `threadId` / `runId` / `timestamp` 顶层字段
  - [x] 1.2.3 新增 `EmitTextMessageStart(messageID)` 方法 → 推 `TEXT_MESSAGE_START` 事件
  - [x] 1.2.4 `TEXT_MESSAGE_CONTENT` 改用 `EmitTextMessageContent(messageID, delta)` 方法，data 含稳定 `messageId`
  - [x] 1.2.5 新增 `EmitTextMessageEnd(messageID)` 方法 → 推 `TEXT_MESSAGE_END` 事件
  - [x] 1.2.6 `TOOL_CALL_ARGS` 改为 `EmitToolCallArgs(toolCallID, delta)`，空 args 时跳过
  - [x] 1.2.7 新增 `EmitStateSnapshot(state)` 方法 → 推 `STATE_SNAPSHOT` 事件
  - [x] 1.2.8 新增 `EmitMessagesSnapshot(messages)` 方法 → 推 `MESSAGES_SNAPSHOT` 事件
  - [x] 1.2.9 `sendAGUI` 内部为每个事件 JSON 顶层加 `type` 字段（与 `event:` 行重复保险）

- [x] Task 1.3: 单元测试
  - [x] 1.3.1 `TestAGUIEventMapper_EmitsTextMessageStartBeforeContent` — TEXT_MESSAGE_START 在 TEXT_MESSAGE_CONTENT 之前
  - [x] 1.3.2 `TestAGUIEventMapper_StableMessageId` — 同一 message 多次 TEXT_MESSAGE_CONTENT 共用同一 messageId
  - [x] 1.3.3 `TestAGUIEventMapper_EmptyArgs_SkipsTOOL_CALL_ARGS` — 空 args 时不发 TOOL_CALL_ARGS
  - [x] 1.3.4 `TestAGUIEventMapper_AllEventsIncludeThreadIdRunIdTimestamp` — 11 种事件全部含稳定字段
  - [x] 1.3.5 `TestAGUIThreadState_NextMessageID_IncrementsSeq` — seq 跨调用递增

## Phase 2: 后端 streamChat 真实 LLM 路径透传

- [x] Task 2.1: `streamChat` 签名 + 闭包
  - [x] 2.1.1 `internal/server/agent_tool_loop.go` `streamChat` 函数签名末尾加 `aguiMode bool` 参数
  - [x] 2.1.2 函数体内构造 `emitEvent` 闭包：`aguiMode=true` 走 `AGUIEventMapper`；`aguiMode=false` 走 `sendAndCache`
  - [x] 2.1.3 所有 `s.sendAndCache(...)` 调用替换为 `emitEvent(...)`
  - [x] 2.1.4 所有 `s.sendSSEEventSafe(...)` 调用替换为 `emitEvent(...)`
  - [x] 2.1.5 文本增量推送前先调 `EmitTextMessageStart` 一次，结束时调 `EmitTextMessageEnd`（aguiMode=true 时）

- [x] Task 2.2: `callOpenAIStream` 接受 aguiMode
  - [x] 2.2.1 `callOpenAIStream` 函数签名末尾加 `aguiMode bool` 参数
  - [x] 2.2.2 内部透传给 `streamChat`
  - [x] 2.2.3 `executeAndRecurse` 内部递归调 `streamChat` 时透传 aguiMode（已通过 handleAgentConfirm → streamChat 透传）

- [x] Task 2.3: `handleAgentChat` 真实 LLM 路径调用更新
  - [x] 2.3.1 `agent_api.go` line 798 检测 header/query → `aguiMode` 变量（已有）
  - [x] 2.3.2 `agent_api.go` line 900 `callOpenAIStream(...)` 末尾加 `aguiMode` 实参
  - [x] 2.3.3 mock 短路分支 line 818 `s.mockEngine.Run(...)` 的 aguiMode 变参保持不变

- [x] Task 2.4: `handleAgentConfirm` / `handleAgentResume` 透传
  - [x] 2.4.1 `handleAgentConfirm` 函数体开头加 `aguiMode := ...` 检测
  - [x] 2.4.2 调 `s.streamChat(...)` 末尾加 `aguiMode` 实参
  - [x] 2.4.3 `handleAgentResume` 函数体开头加 `aguiMode := ...` 检测
  - [x] 2.4.4 调 `s.streamChat(...)` 末尾加 `aguiMode` 实参（resume 不直接走 streamChat，但保持检测一致）

- [x] Task 2.5: 单元测试
  - [x] 2.5.1 `TestStreamChat_AGUIMode_EmitsTextMessageStartBeforeContent` — Phase 1 升级后的 text 序列
  - [x] 2.5.2 `TestStreamChat_AGUIMode_EmitsTextMessageEndAfterStream` — 文本流结束边界
  - [x] 2.5.3 `TestStreamChat_AGUIMode_ToolCallArgsEmpty_SkipsTOOL_CALL_ARGS`
  - [x] 2.5.4 `TestStreamChat_AGUIMode_StableMessageId` — 同一 messageId 跨多次 delta
  - [x] 2.5.5 `TestStreamChat_LegacyMode_PreservesDataFormat` — aguiMode=false 与改造前字节级一致
  - [x] 2.5.6 `TestHandleAgentChat_RealLLM_PassesAGUIModeToStreamChat`
  - [x] 2.5.7 `TestHandleAgentConfirm_AGUIHeader_PassesThrough`
  - [x] 2.5.8 `TestHandleAgentResume_AGUIHeader_PassesThrough`

- [x] Task 2.6: 预存 deadlock 修复
  - [x] 2.6.1 `handleAgentConfirm` 中 `sess.mu` 在调用 `sendAndCache` 前未释放导致 self-deadlock
  - [x] 2.6.2 复制 tool 引用到本地后释放 lock，再分块按需 lock

## Phase 3: 前端 useAgent 协议分发器

- [x] Task 3.1: `useAGUIParser` 组合式
  - [x] 3.1.1 新建 `app/encv-mobile/src/composables/useAGUIParser.ts`
  - [x] 3.1.2 实现 `parseAGUIEvent(raw: string): AgentEvent | null` — 解析 `event: <type>\ndata: <json>\n\n` → 归一化为 `AgentEvent`
  - [x] 3.1.3 11 种 AG-UI 事件 → `AgentEvent` 类型的归一化映射（见 spec Requirement: useAgent 协议分发器）
  - [x] 3.1.4 实现 `processAGUISSE(stream)` — 包装 `processSSE` 的 reader 循环，逐行解析 AG-UI 事件
  - [x] 3.1.5 单元测试 `useAGUIParser.test.ts`：12+ 用例覆盖各种事件映射

- [x] Task 3.2: `useAgent.processSSE` 协议分发
  - [x] 3.2.1 `useAgent.ts` `processSSE` 改造为：先读 fetch response headers 中 `X-Agent-Protocol`
  - [x] 3.2.2 若为 `agui` → 调 `processAGUISSE` 并合并返回值
  - [x] 3.2.3 若无 header → fallback `processLegacySSE`（原 `processSSE` 逻辑，提取为独立函数）
  - [x] 3.2.4 `useAgent.send()` 始终带 `X-Agent-Protocol: agui` header（默认行为）
  - [x] 3.2.5 单元测试：mock 两种响应头 → 验证走不同 parser（由 useAGUIParser.test.ts 覆盖核心逻辑，processSSE 协议分发由集成测试验证）

- [x] Task 3.3: `useAgentApiBase` 暴露协议切换
  - [x] 3.3.1 `useAgentApiBase.ts` 新增 `AgentProtocol = 'agui' | 'legacy' | 'auto'`
  - [x] 3.3.2 新增 `setAgentProtocol(protocol)` / `getAgentProtocol()` 函数
  - [x] 3.3.3 持久化到 `localStorage('encv-agent-protocol')`
  - [x] 3.3.4 `useAgent.send()` 读 `getAgentProtocol()` 决定 header 行为

## Phase 4: TDesign 引擎改造

- [x] Task 4.1: `tdesignEngine.ts` 重写
  - [x] 4.1.1 删除 `<Chatbot>` 相关 import（`import { Chatbot } from '@tdesign-vue-next/chat'`）
  - [x] 4.1.2 删除 `chatServiceConfig` 构造
  - [x] 4.1.3 `renderMessages` 改为 `h(TDesignChatView, { ...props })` 传入完整 EngineRenderProps
  - [x] 4.1.4 `destroy` 方法清理 TDesign 视觉相关的 CSS 注入（如有）

- [x] Task 4.2: `TDesignChatView.vue` 重写为消息列表渲染器
  - [x] 4.2.1 删除 `<Chatbot>` 模板节点
  - [x] 4.2.2 改为 `<ChatList>` + `<ChatItem v-for="msg in messages">` 列表渲染
  - [x] 4.2.3 user 消息 vs assistant 消息分别用 TDesign 视觉样式
  - [x] 4.2.4 流式状态用 TDesign `<ChatThinking>` 组件（`status === 'streaming'` 时显示）
  - [x] 4.2.5 工具调用用 TDesign 风格操作卡（包装现有 `tool_calls: ToolCall[]`）
  - [x] 4.2.6 主题色覆盖：使用 TDesign CSS 变量（`--td-brand-color` / `--td-bg-color-container` 等），由 useTheme 注入的 primary 色驱动（占位未做实时联动，预留 CSS 变量可被外部主题覆盖）

- [x] Task 4.3: `main.ts` 清理
  - [x] 4.3.1 移除 `import TDesignChat from '@tdesign-vue-next/chat'`
  - [x] 4.3.2 移除 `import '@tdesign-vue-next/chat/es/style/index.css'`
  - [x] 4.3.3 移除 `app.use(TDesignChat)`

- [x] Task 4.4: 单元测试
  - [x] 4.4.1 `tdesignEngine.test.ts` — 验证 `renderMessages` 接收 EngineRenderProps 并渲染 TDesignChatView
  - [x] 4.4.2 `TDesignChatView.test.ts` — 验证 props 透传 + 流式状态显示

## Phase 5: Default / CopilotKit 注释 + 视觉验证

- [x] Task 5.1: 注释更新
  - [x] 5.1.1 `defaultEngine.ts` 顶部注释明确"数据通过 useAgent 的 AG-UI parser 归一化获取"
  - [x] 5.1.2 `copilotkitStyleEngine.ts` 顶部注释同上
  - [x] 5.1.3 `tdesignEngine.ts` 顶部注释明确"通过 useAgent 共享数据，TDesign 仅作为渲染层"

- [ ] Task 5.2: 视觉一致性回归（需要人工 E2E 验证，本任务仅做契约层注释；不阻塞自动化）
  - [ ] Default 引擎：与改造前 UI 像素级一致
  - [ ] CopilotKit 风格引擎：与改造前 UI 像素级一致
  - [ ] TDesign 风格引擎：能感受到 TDesign 视觉特征（圆角、阴影、组件风格）

## Phase 6: 全量回归验证

- [ ] Task 6.1: 编译 + 类型检查
  - [ ] 6.1.1 后端：`go build ./cmd/encv` 0 错误
  - [ ] 6.1.2 后端：`go vet ./internal/server/...` 0 警告
  - [ ] 6.1.3 前端：`npx vue-tsc --noEmit` 0 错误
  - [ ] 6.1.4 前端：`npx vite build` 0 错误

- [ ] Task 6.2: 单测全跑
  - [ ] 6.2.1 后端：`go test ./internal/server/...` 全跑 0 回归
  - [ ] 6.2.2 前端：`npm test` 全跑 0 回归

- [ ] Task 6.3: 端到端验证（按 spec 验证步骤 9-15）
  - [ ] 6.3.1 DefaultEngine + 真实 API → UI 正确渲染
  - [ ] 6.3.2 CopilotKit 引擎 + 真实 API → UI 渲染
  - [ ] 6.3.3 TDesign 引擎 + 真实 API → UI 用 TDesign 视觉组件渲染（**TDesign 死场景被修复**）
  - [ ] 6.3.4 TDesign 工具调用显示为 TDesign 风格操作卡
  - [ ] 6.3.5 引擎切换无消息丢失
  - [ ] 6.3.6 mock 模式回归
  - [ ] 6.3.7 TDesign 资源清理

# Task Dependencies
- [Task 1.x] → [Task 2.x]（后端 AG-UI shape 必须先升级再透传）
- [Task 1.x] → [Task 3.1]（前端 parser 依赖后端 shape 契约）
- [Task 3.1] → [Task 3.2]（parser 实现后再接入 useAgent）
- [Task 3.2] + [Task 4.x] 可并行（TDesign 改造与 useAgent 分发独立，但都依赖 Phase 1 shape）
- [Phase 5] 依赖 [Phase 3] + [Phase 4] 完成
- [Phase 6] 必须在所有阶段完成后执行
