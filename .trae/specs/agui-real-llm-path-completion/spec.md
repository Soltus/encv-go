# AG-UI 统一协议改造 Spec — 三引擎共享 AG-UI 数据通路

## Why

`multi-engine-chat-architecture` spec 实现了一个"多引擎架构"，但**底层协议不统一**：

| 引擎 | 数据流 |
|------|--------|
| Default (Ionic) | 后端自定义 SSE 事件 → `useAgent.processSSE` 解析 → `Message[]` |
| CopilotKit 风格 | 后端自定义 SSE 事件 → `useAgent.processSSE` 解析 → `Message[]`（**同一份数据**）|
| TDesign Chat | 后端 AG-UI（仅 mock 模式）→ TDesign `<Chatbot>` 内部解析器（**完全绕过 useAgent**）|

**两个核心症状**：

1. **TDesign 在 mock 模式也完全无法渲染**：
   - 当前 mock 路径确实走 `AGUIEventMapper` 输出 AG-UI 事件
   - 但 TDesign `<Chatbot>` 的 AG-UI 解析器期望完整的事件 shape（含 `role: 'assistant' / 'user'`、`messageId`、`threadId` 等字段），我们简化的 mapper 输出不满足
   - 结果：TDesign 拿到 `event: TEXT_MESSAGE_CONTENT\ndata: {"messageId":"msg_xxx","delta":"..."}` 等不完整事件 → 解析器报错或静默忽略 → UI 不渲染
   - **mock 模式 ≠ TDesign 能用**，TDesign 在所有场景下都是死的

2. **协议分裂导致无法扩展**：
   - Default + CopilotKit 用自定义事件（`text_delta` / `tool_call` / `tool_result`）
   - TDesign 用 AG-UI（`TEXT_MESSAGE_CONTENT` / `TOOL_CALL_START` 等）
   - 未来要加 A2UI / MCP / 其他新协议 → 引擎会再次分裂
   - 用户切引擎时无消息状态连贯保证（不同协议的状态机不同步）

**新方案核心价值**：

1. **AG-UI 成为统一标准协议** — 后端 `X-Agent-Protocol: agui` header / `?protocol=agui` query 触发 AG-UI 输出（**默认行为**）；自定义 SSE 标记为 `legacy` 仅作 fallback
2. **useAgent 升级为 AG-UI 客户端** — `processSSE` 改造为协议无关的事件分发器，根据响应头选择 AG-UI parser 或 legacy parser；两种 parser 都归一化输出到同一个 `Message[]` reactive state
3. **三引擎共享数据源** — Default / CopilotKit / TDesign 全部从 `useAgent` 的 `Message[]` 读取数据，只在**渲染层**用不同视觉
4. **TDesign 改造为"渲染器"而非"客户端"** — 删除 `<Chatbot>` + `chatServiceConfig`，改用 TDesign 视觉组件（`<t-chat-item>` / `<t-chat-content>` / `<t-chat-thinking>` 等）渲染同一份 `Message[]`
5. **A2UI / 新协议可插拔** — 未来新增协议只需加一个 parser + 后端 mapper，前端三引擎无需改动

---

## What Changes

### 后端

- `internal/server/agent_agui_adapter.go` — `AGUIEventMapper` 升级事件 shape（补齐 TDesign 解析器要求的字段：稳定 `messageId` / `runId` / `role` / `threadId`）
- `internal/server/agent_tool_loop.go` — `streamChat` 函数签名加 `aguiMode bool` 参数 + 内部 `emitEvent` 闭包分流
- `internal/server/agent_api.go` — `handleAgentChat` / `handleAgentConfirm` / `handleAgentResume` 全部透传 `aguiMode`（覆盖真实 LLM 路径）
- `internal/server/agent_agui_adapter.go` — 新增 `AGUIThreadState` 辅助类，生成稳定的 `threadId` / `runId` / `messageId`

### 前端

- `app/encv-mobile/src/composables/useAgent.ts` — `processSSE` 改造为协议分发器：检测 `X-Agent-Protocol: agui` 响应头 → 走 `processAGUISSE`；否则走 `processLegacySSE`（保留旧路径作 fallback）
- `app/encv-mobile/src/composables/useAGUIParser.ts`（**新增**）— AG-UI 事件 → `Message[]` 状态变更的归一化逻辑
- `app/encv-mobile/src/engines/tdesignEngine.ts` — **重写**：删除 `<Chatbot>` 集成，改为消费 `messages: readonly Message[]` 并用 TDesign 视觉组件渲染
- `app/encv-mobile/src/engines/TDesignChatView.vue` — **重写**：从 Chatbot 包装改为消息列表渲染器
- `app/encv-mobile/src/engines/copilotkitStyleEngine.ts` — **小改**：明确注释"通过 useAgent 的 AG-UI parser 获取数据"
- `app/encv-mobile/src/engines/defaultEngine.ts` — **小改**：明确注释"通过 useAgent 的 AG-UI parser 获取数据"
- `app/encv-mobile/src/composables/useAgentApiBase.ts` — 暴露 `setAgentProtocol(protocol: 'agui' | 'legacy')` 用于测试/调试
- `app/encv-mobile/src/views/AgentChat.vue` — 引擎切换时无需刷新（数据已统一在 `useAgent` 共享 store）

### Mock 模式

- `internal/server/agent_mock.go` — MockEngine 继续支持 aguiMode（不变）
- `internal/server/agent_agui_adapter.go` — AG-UI 事件 shape 升级后，mock 路径自动受益（**无需改 mock**）

### 不影响

- `internal/server/agent_mock_scenarios.go`（剧本数据不变）
- `app/encv-mobile/src/composables/useContextUsage.ts`（独立功能）
- 前端所有 agent 子组件的视觉（仅数据流路径变化）
- 后端业务逻辑（`executeAgentTool` 派发器、`streamChat` 工具循环等）

---

## ADDED Requirements

### Requirement: AG-UI 事件 shape 升级（TDesign 兼容）

`AGUIEventMapper` SHALL 输出 TDesign Chat 解析器要求的事件 shape，**所有事件** 必须包含稳定的 `threadId` / `runId` / `messageId` 等字段。

#### Scenario: 事件字段契约（升级后）

| AG-UI 事件 | 必填字段 | 备注 |
|----------|--------|------|
| `RUN_STARTED` | `threadId` / `runId` / `timestamp` | 每个 run 唯一 runId |
| `TEXT_MESSAGE_START` | `messageId` / `role: 'assistant'` / `timestamp` | **新增**：在 TEXT_MESSAGE_CONTENT 之前 |
| `TEXT_MESSAGE_CONTENT` | `messageId` / `delta` / `timestamp` | `delta` 是增量文本 |
| `TEXT_MESSAGE_END` | `messageId` / `timestamp` | **新增**：标记完整消息边界 |
| `TOOL_CALL_START` | `toolCallId` / `toolCallName` / `timestamp` | |
| `TOOL_CALL_ARGS` | `toolCallId` / `delta` / `timestamp` | `delta` 是 args 字符串 |
| `TOOL_CALL_END` | `toolCallId` / `timestamp` | |
| `TOOL_CALL_RESULT` | `toolCallId` / `content` / `timestamp` | |
| `RUN_FINISHED` | `threadId` / `runId` / `timestamp` | |
| `STATE_SNAPSHOT` | `state: object` / `timestamp` | **新增**：会话级共享状态（用于 context 同步）|
| `MESSAGES_SNAPSHOT` | `messages: Message[]` | **新增**：完整消息快照（断点续传对齐）|

**所有** 事件的 JSON 顶层必须有 `type` 字段（与 `event:` 行重复保险）。

#### Scenario: 稳定 ID 生成

- `threadId` 在整个 session 不变（来自 `sess.SessionID`）
- `runId` 在每次 `/api/chat` / `/api/confirm` 调用时生成新 UUID（同一 session 可有多个 run）
- `messageId` = `msg_<runId>_<seq>`（seq 跨 run 全局递增）
- `toolCallId` 由 LLM / mock 剧本给出（保持原值）

#### Scenario: 字段缺失 graceful 降级

- **WHEN** `AGUIEventMapper` 收到 `tool_call` 事件但 `args` 为空字符串
- **THEN** **不**发送 `TOOL_CALL_ARGS` 事件（避免空 delta 引发解析器报错）
- **AND** `TOOL_CALL_END` 仍然发出（标记 tool_call 边界）

---

### Requirement: streamChat 真实 LLM 路径 AG-UI 透传

`streamChat` 函数 SHALL 接受 `aguiMode bool` 参数，内部 `emitEvent` 闭包统一事件出口。

#### Scenario: 函数签名变更

**BEFORE**:
```go
func (s *Server) streamChat(ctx context.Context, c *gin.Context, cfg AgentConfig, model string, temperature float64, messages []chatMsg, sess *agentSession, openAITools []map[string]interface{}, toolMeta map[string]map[string]interface{})
```

**AFTER**:
```go
func (s *Server) streamChat(ctx context.Context, c *gin.Context, cfg AgentConfig, model string, temperature float64, messages []chatMsg, sess *agentSession, openAITools []map[string]interface{}, toolMeta map[string]map[string]interface{}, aguiMode bool)
```

#### Scenario: 闭包 emitEvent 分流

```go
var emitEvent func(evType string, data map[string]interface{})
if aguiMode {
    aguiMapper := NewAGUIMapper(c.Writer, flusher, sess.SessionID)
    emitEvent = func(evType string, data map[string]interface{}) {
        aguiMapper.MapEvent(MockEvent{Type: evType, Data: data}, 0, 0)
    }
} else {
    emitEvent = func(evType string, data map[string]interface{}) {
        s.sendAndCache(sess, c.Writer, getFlusher(c), evType, data)
    }
}
```

- `aguiMode=true` → `AGUIEventMapper.MapEvent` 转换后 flush
- `aguiMode=false` → 走原有 `sendAndCache`（legacy 自定义 SSE 格式）

#### Scenario: 单元测试

- [ ] `TestStreamChat_AGUIMode_EmitsTextMessageStartBeforeContent` — 升级后第一段文本开始前推 `TEXT_MESSAGE_START`
- [ ] `TestStreamChat_AGUIMode_EmitsTextMessageEndAfterStream` — 文本流结束后推 `TEXT_MESSAGE_END`
- [ ] `TestStreamChat_AGUIMode_ToolCallArgsEmpty_SkipsTOOL_CALL_ARGS` — 空 args 时不发 `TOOL_CALL_ARGS`
- [ ] `TestStreamChat_AGUIMode_StableMessageId` — 同一 message 多次 TEXT_MESSAGE_CONTENT 共用同一 messageId
- [ ] `TestStreamChat_LegacyMode_PreservesDataFormat` — `aguiMode=false` 输出与改造前字节级一致

---

### Requirement: handleAgentChat / handleAgentConfirm / handleAgentResume 透传 aguiMode

三个 handler SHALL 全部识别 `X-Agent-Protocol: agui` header 和 `?protocol=agui` query 并透传给 `streamChat`。

#### Scenario: 三处透传点

| Handler | 透传位置 | 备注 |
|--------|--------|------|
| `handleAgentChat` | `c.GetHeader("X-Agent-Protocol") == "agui"` → `aguiMode` 变量 → `callOpenAIStream(..., aguiMode)` | 真实 LLM 路径 |
| `handleAgentConfirm` | 同上 → `s.streamChat(..., aguiMode)` | confirm 后流 |
| `handleAgentResume` | 同上 → `s.streamChat(..., aguiMode)` | 续传流 |

#### Scenario: 真实 LLM 路径 + aguiMode=true 端到端

- **WHEN** 前端发 `POST /api/chat?protocol=agui`（真实 API 模式）
- **THEN** 响应流是 AG-UI 标准格式（`event: RUN_STARTED` / `event: TEXT_MESSAGE_START` / ...）
- **AND** TDesign 引擎 / CopilotKit 引擎 / Default 引擎都能正确解析（走 useAgent 的 AG-UI parser）

#### Scenario: legacy 模式保留

- **WHEN** 前端**不**带 AG-UI 头/query
- **THEN** 响应流是 legacy 自定义 SSE 格式（`data: {"type":"text_delta",...}`）
- **AND** 旧客户端兼容（不影响现有功能）

---

### Requirement: 前端 useAgent 协议分发器

`useAgent.processSSE` SHALL 改造为协议分发器，根据响应头选择 parser。

#### Scenario: processSSE 分流逻辑

```typescript
async function processSSE(stream: ReadableStream<Uint8Array> | null) {
  // 1. 从 fetch response.headers 读取 X-Agent-Protocol
  // 2. if 'agui' → processAGUISSE()
  // 3. else → processLegacySSE()
  // 两种 parser 内部都把事件归一化到 handleAgentEvent(event)
  //   - 归一化后事件类型保持与现有 useAgent 一致 (text_delta, tool_call 等)
  //   - 但来源是 AG-UI parser / legacy parser 不影响下游
}
```

#### Scenario: AG-UI 事件归一化

`processAGUISSE` SHALL 把 AG-UI 事件归一化为 `useAgent` 内部的 `AgentEvent`：

| AG-UI 事件 | 归一化为 `AgentEvent.type` | 提取字段 |
|----------|---------------------------|---------|
| `RUN_STARTED` | `stream_start` | `{ runId, threadId, protocol: 'agui' }` |
| `TEXT_MESSAGE_START` | （无内部对应，作为 meta）| `{ messageId }` |
| `TEXT_MESSAGE_CONTENT` | `text_delta` | `{ text: data.delta, messageId }` |
| `TEXT_MESSAGE_END` | （标记完整消息）| `{ messageId }` |
| `TOOL_CALL_START` | `tool_call` | `{ id, name, kind: 'unknown' }` |
| `TOOL_CALL_ARGS` | `tool_call_args` | `{ id, argsDelta: data.delta }`（**新增**内部类型）|
| `TOOL_CALL_END` | `tool_status` | `{ id, status: 'success' }` |
| `TOOL_CALL_RESULT` | `tool_result` | `{ id, result: data.content }` |
| `RUN_FINISHED` | `stream_end` | `{ runId, threadId }` |
| `STATE_SNAPSHOT` | `state_snapshot` | `{ state: data.state }`（**新增**）|
| `MESSAGES_SNAPSHOT` | `messages_snapshot` | `{ messages: data.messages }`（**新增**，断点续传对齐）|

#### Scenario: legacy parser 保留

`processLegacySSE` SHALL 保留原有 `processSSE` 的所有逻辑（不改），作为 `X-Agent-Protocol` 头缺失时的 fallback。

#### Scenario: 协议协商

- **WHEN** `useAgent.send()` 发请求
- **THEN** 默认**始终**带 `X-Agent-Protocol: agui` header（因为后端已实现）
- **AND** 后端若不支持 AG-UI（极端情况）→ 响应头无 `X-Agent-Protocol` → 前端 fallback 到 legacy parser
- **AND** 旧后端（无 AG-UI 改造）→ 响应流是 `data: {"type":"text_delta",...}` → `X-Agent-Protocol` 头不返回 → 前端走 legacy parser

---

### Requirement: 新增 useAGUIParser 组合式

`useAGUIParser.ts` SHALL 封装 AG-UI 事件解析逻辑。

#### Scenario: 导出函数

```typescript
// useAGUIParser.ts
export function useAGUIParser(): {
  parseAGUIEvent(raw: string): AgentEvent | null
  processAGUISSE(stream: ReadableStream<Uint8Array>): Promise<{ received, streamEnded, morePending }>
}
```

- `parseAGUIEvent` — 解析单条 `event: <type>\ndata: <json>\n\n` → 归一化为 `AgentEvent`
- `processAGUISSE` — 包装 `processSSE` 的 reader 循环，逐行解析 AG-UI 事件

#### Scenario: 单元测试

- [ ] `TestParseAGUIEvent_TEXT_MESSAGE_CONTENT_ToTextDelta` — 验证归一化正确
- [ ] `TestParseAGUIEvent_TOOL_CALL_ARGS_AccumulatesArgs` — 多次 ARGS 事件累积到同一 tool_call
- [ ] `TestProcessAGUISSE_FullRunLifecycle` — 完整 RUN_STARTED → TEXT_MESSAGE_* → TOOL_CALL_* → RUN_FINISHED 解析

---

### Requirement: TDesign 引擎从 Chatbot 改为渲染器

`tdesignEngine.ts` SHALL **删除** `<Chatbot>` 集成，改为消费 `messages: readonly Message[]` 并用 TDesign 视觉组件渲染。

#### Scenario: 新版 tdesignEngine.ts

```typescript
export function createTDesignEngine(): ChatEngine {
  return {
    id: 'tdesign',
    name: 'TDesign Chat',
    description: '腾讯 TDesign 视觉风格（基于 useAgent 共享数据）',
    supportsA2UI: false,

    renderMessages(props: EngineRenderProps): VNode {
      return h(TDesignChatView, { ...props })
    },

    destroy(): void { /* 清理 */ },
  }
}
```

#### Scenario: TDesignChatView.vue 改造

`TDesignChatView.vue` SHALL 用 TDesign UI 组件渲染消息列表：

```vue
<template>
  <div class="tdesign-chat-container">
    <ChatList>
      <ChatItem
        v-for="msg in messages"
        :key="msg.id"
        :role="msg.role"
        :content="msg.content"
        :avatar="msg.role === 'user' ? userAvatar : botAvatar"
      />
      <ChatThinking v-if="status === 'streaming'" content="正在思考..." />
    </ChatList>
  </div>
</template>
```

- **删除** `<Chatbot>` 和 `chatServiceConfig`（不再自行发 SSE）
- **使用** `messages: readonly Message[]` prop（与 Default / CopilotKit 同一份数据）
- **保留** TDesign 视觉风格（圆角、阴影、间距等 TDesign 设计 token）
- **支持** TDesign 主题色覆盖（沿用现有 `useTheme.ts` 的 primary 色）

#### Scenario: TDesign 工具调用渲染

- **WHEN** 渲染 tool_call 消息
- **THEN** 使用 TDesign `<t-chat-toolcall>` 或自定义 TDesign 风格的操作卡（不调 TDesign 的工具调用框架，仅视觉）
- **AND** 工具结果用 `<t-chat-content>` 渲染 JSON / markdown

#### Scenario: 删除 main.ts 中 TDesign Chat 全局注册

- **WHEN** 改造完成
- **THEN** `main.ts` 移除 `import TDesignChat from '@tdesign-vue-next/chat'` + `app.use(TDesignChat)` + CSS import
- **理由**：不再使用 TDesign 的 Chatbot / SSE 框架
- **保留**：TDesign 其他基础组件（按钮、列表、头像、图标等）继续使用

---

### Requirement: Default / CopilotKit 注释明确"数据来自 useAgent AG-UI parser"

两个引擎 SHALL 在源码顶部加注释，明确**数据通过 useAgent 的 AG-UI parser 归一化获取**。

#### Scenario: defaultEngine.ts 注释

```typescript
/**
 * defaultEngine.ts - Ionic 默认渲染引擎
 *
 * 数据源：通过 useAgent 的 AG-UI parser（composables/useAGUIParser.ts）
 *        解析后端 AG-UI SSE 事件流，归一化为 messages: readonly Message[]。
 * 渲染层：本引擎用 Ionic 组件渲染 Message[]。
 *
 * 协议：AG-UI（与 CopilotKit 风格 / TDesign 引擎共享同一份数据）
 */
```

#### Scenario: copilotkitStyleEngine.ts 注释

```typescript
/**
 * copilotkitStyleEngine.ts - CopilotKit 风格渲染引擎
 *
 * 数据源：通过 useAgent 的 AG-UI parser 归一化获取 Message[]。
 * 渲染层：本引擎用 CopilotKit 风格 Vue 组件渲染 Message[]。
 * 协议：AG-UI（与 Default / TDesign 引擎共享同一份数据）
 */
```

> **关键认知**：用户原话"CopilotKit 也应当遵循 AG-UI"——CopilotKit 引擎本来就和 Default 共用 `useAgent` 的 `Message[]`（无独立 SSE 消费），所以"遵循 AG-UI"实际是要求 `useAgent` 改为 AG-UI parser（由本次改造满足），引擎层只需在注释中明确这一点。

---

### Requirement: useAgentApiBase 暴露 setAgentProtocol

`useAgentApiBase.ts` SHALL 暴露 `setAgentProtocol(protocol)` 用于测试 / 调试 / 引擎切换时协商。

#### Scenario: API

```typescript
// useAgentApiBase.ts
export type AgentProtocol = 'agui' | 'legacy' | 'auto'

export function setAgentProtocol(protocol: AgentProtocol): void
export function getAgentProtocol(): AgentProtocol
```

- `auto`（默认）→ 总是带 `X-Agent-Protocol: agui` header
- `agui` → 强制 AG-UI（同 auto）
- `legacy` → 不带 header（用于回滚调试）

#### Scenario: useAgent 集成

- **WHEN** `useAgent.send()` 发起请求
- **THEN** 读 `getAgentProtocol()` 决定是否带 `X-Agent-Protocol: agui` header
- **AND** `setAgentProtocol('legacy')` 可在 DevTools 手动切回 legacy 模式排查

---

## MODIFIED Requirements

### Requirement: useAgent.processSSE 协议分发

**BEFORE**: 单一 parser 解析自定义 SSE 事件
**AFTER**: 协议分发器，根据响应头选 AG-UI / legacy parser

#### Scenario: 兼容性

- `agui` 协议 → 新增 `processAGUISSE` 路径
- 无 `X-Agent-Protocol` 响应头 → fallback 到 `processLegacySSE`（旧逻辑不变）
- 归一化后的 `AgentEvent` 类型契约**不变**（下游 `handleAgentEvent` 0 改动）

### Requirement: AGUIEventMapper 事件 shape

**BEFORE**: 7 种事件（`RUN_STARTED` / `TEXT_MESSAGE_CONTENT` / `TOOL_CALL_START` / `TOOL_CALL_ARGS` / `TOOL_CALL_END` / `TOOL_CALL_RESULT` / `RUN_FINISHED`）
**AFTER**: 11 种事件（新增 `TEXT_MESSAGE_START` / `TEXT_MESSAGE_END` / `STATE_SNAPSHOT` / `MESSAGES_SNAPSHOT`），所有事件含稳定 `threadId` / `runId` / `messageId` / `timestamp`

### Requirement: tdesignEngine.ts 集成方式

**BEFORE**: 使用 TDesign `<Chatbot>` + `chatServiceConfig`（自消费 SSE，绕过 useAgent）
**AFTER**: 渲染 `messages: readonly Message[]`（与其他引擎共享 useAgent 数据）

---

## REMOVED Requirements

无（不删除任何现有能力；只是数据流路径统一到 AG-UI）

---

## 约束与限制

1. **AG-UI 优先，legacy 兜底** — 前端默认带 `X-Agent-Protocol: agui`；后端无 AG-UI 实现时（旧版本）→ 响应头无该字段 → 前端 fallback legacy parser
2. **useAgent 内部 `AgentEvent` 契约不变** — 不管是 AG-UI parser 还是 legacy parser，输出都给到 `handleAgentEvent(event)`，下游 0 改动
3. **三引擎共享 `useAgent` 共享 store** — 引擎切换时**不丢消息**（同份 `messages: readonly Message[]`）
4. **TDesign 视觉保留** — 切换到 TDesign 引擎时仍能感受到 TDesign 设计语言（圆角、阴影、间距、组件风格）
5. **TDesign 工具调用用视觉层** — 不调 TDesign 的工具调用框架（`<t-chat-toolcall>` 等），仅用 TDesign 视觉风格包装现有 `tool_calls: ToolCall[]` 数据
6. **`@tdesign-vue-next/chat` 依赖可保留或移除** — 改造后仅用 TDesign 基础组件（按钮、列表、头像、图标），可以改为 `@tdesign-vue-next` 主包（不带 chat 子包）；本 spec 阶段**保留**依赖，迁依赖单独 spec
7. **A2UI 扩展点保留** — `ChatEngine.supportsA2UI: false`（本轮不实现 A2UI 渲染器），未来 A2UI 协议加进 useAGUIParser 即可

---

## 与现有 spec 的关系

| 现有 spec | 影响 |
|----------|------|
| `multi-engine-chat-architecture` | **本 spec 是其 Phase 4 + Phase 5 的彻底补齐 + 简化** — 删 TDesign Chatbot 集成，三引擎统一数据源 |
| `agent-mock-mode` | 不受影响（mock 路径已支持 aguiMode，升级后事件 shape 自动受益） |
| `go-in-process-agent` | 不受影响（前端 useAgent 走 AG-UI，事件归一化后下游不变） |
| `multi-engine-chat-architecture` Task 4.3 | 任务范围扩展（不再仅 mock 路径，覆盖真实 LLM + confirm + resume） |

---

## 验证步骤

### 后端验证

1. **类型检查** — `go build ./cmd/encv` 0 错误
2. **单元测试** — `go test ./internal/server/... -run TestAGUI -v` 全部通过
3. **集成测试** — `go test ./internal/server/... -run TestStreamChat -v` 全部通过
4. **真实 LLM + AG-UI** — 启动服务 → 关闭 mock → 浏览器 Network 面板发 `?protocol=agui` → 验证响应流含 `event: RUN_STARTED` / `event: TEXT_MESSAGE_START` / `event: TEXT_MESSAGE_CONTENT` / `event: TOOL_CALL_START` / `event: RUN_FINISHED`

### 前端验证

5. **类型检查** — `npx vue-tsc --noEmit` 0 错误
6. **构建** — `npx vite build` 0 错误
7. **AG-UI parser 单元测试** — `npm test -- useAGUIParser` 全部通过
8. **useAgent 协议分发** — 手动 mock 一个后端返回 `X-Agent-Protocol: agui` 头 → 验证 useAgent 走 AG-UI parser；返回自定义事件无该头 → 验证 fallback legacy parser

### 端到端验证

9. **DefaultEngine** — 关闭 mock → 真实 API → 发送消息 → UI 正确渲染（`Message[]` 来自 AG-UI parser）
10. **CopilotKit 风格** — 切到 copilotkit-style 引擎 → 真实 API → 发送消息 → UI 用 CopilotKit 风格渲染（同份数据）
11. **TDesign 风格** — 切到 tdesign 引擎 → 真实 API → 发送消息 → UI 用 TDesign 视觉组件渲染（同份数据）
12. **TDesign 工具调用** — 触发 `list_files` → TDesign 风格操作卡显示 → 工具结果用 TDesign 视觉渲染
13. **引擎切换无消息丢失** — 对话进行中切换 Default → TDesign → CopilotKit → Default → 验证消息历史不丢
14. **mock 模式回归** — 打开 mock 模式 → 真实 LLM 路径不再走 mock → 验证 TDesign 仍能渲染（mock 路径的 AG-UI 输出也能被 TDesign 消费）
15. **TDesign 资源清理** — 切换出 TDesign 引擎 → 验证 `<t-chat-*>` 组件 unmounted

---

## 端点契约总结

| 端点 | 触发 AG-UI 的方式 | 输出格式（aguiMode=true）| 输出格式（aguiMode=false/legacy）|
|------|----------------|------------------------|-------------------------------|
| `POST /api/chat` | `X-Agent-Protocol: agui` header 或 `?protocol=agui` query | AG-UI 11 种事件 | 自定义 SSE（默认 fallback）|
| `POST /api/confirm` | 同上 | AG-UI | 自定义 SSE |
| `POST /api/resume` | 同上 | AG-UI | 自定义 SSE |
| `GET /api/agent/context-usage` | 不变 | 不变 | 不变 |

## 关键文件 / 函数

- `internal/server/agent_agui_adapter.go` — `AGUIEventMapper` 升级 + `AGUIThreadState` 新增
- `internal/server/agent_tool_loop.go` — `streamChat` / `callOpenAIStream` 接受 aguiMode
- `internal/server/agent_api.go` — 三个 handler 透传 aguiMode
- `app/encv-mobile/src/composables/useAgent.ts` — `processSSE` 协议分发
- `app/encv-mobile/src/composables/useAGUIParser.ts`（**新增**）— AG-UI 事件归一化
- `app/encv-mobile/src/engines/tdesignEngine.ts` — 改为渲染器
- `app/encv-mobile/src/engines/TDesignChatView.vue` — 用 TDesign 视觉组件渲染 Message[]
- `app/encv-mobile/src/engines/defaultEngine.ts` / `copilotkitStyleEngine.ts` — 加注释
- `app/encv-mobile/src/composables/useAgentApiBase.ts` — 暴露 `setAgentProtocol`
- `app/encv-mobile/src/main.ts` — 移除 `TDesignChat` 全局注册
- 测试文件：`agent_agui_adapter_test.go` / `useAGUIParser.test.ts` / `tdesignEngine.test.ts`
