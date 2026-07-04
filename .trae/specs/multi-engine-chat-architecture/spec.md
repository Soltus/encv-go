# Agent Chat 多渲染引擎架构重构 Spec

## Why

当前 Agent Chat UI 是一套**硬编码的 Ionic/Vue 组件实现**（AssistantMessage / OperationCard / FileListCard 等），与后端 SSE 事件格式强耦合。这导致：

1. **无法切换渲染风格** —— 用户只能看到一种 UI 表现，无法体验不同框架的交互范式
2. **缺乏协议标准化** —— 后端事件是自定义格式（text_delta / tool_call / tool_result），不兼容业界标准 AG-UI 协议
3. **无法复用生态组件** —— CopilotKit 的 Generative UI、TDesign Chat 的开箱即用工具调用卡片、A2UI 的声明式 Surface 均无法接入
4. **技术债务积累** —— 每次新增功能都直接修改现有组件，没有抽象层隔离变化

## What Changes

### 核心架构：三层抽象 + 运行时切换

```
┌─────────────────────────────────────────────────────┐
│                  AgentChat.vue                       │
│  ┌───────────┐  ┌──────────┐  ┌──────────────────┐  │
│  │ 引擎选择器 │→│ 渲染适配器 │→│ 组件实现层         │  │
│  │ (UI Switch)│  │ (Adapter) │  │ (Implementation)  │  │
│  └───────────┘  └──────────┘  ├──────────────────┤  │
│                              │ ● Default (Ionic)   │  ← 当前实现，保留为默认
│                              │ ● CopilotKit 风格    │  ← 新增：React-like 交互
│                              │ ● TDesign Chat      │  ← 新增：腾讯设计语言
│                              └──────────────────┘  │
└─────────────────────────────────────────────────────┘
           ↑                        ↑              ↑
     用户实时切换               协议转换          组件渲染
```

### 变更清单

- **新增 `ChatEngine` 抽象接口** —— 定义统一的聊天渲染契约（消息列表/发送/状态/工具调用）
- **新增 `DefaultEngineAdapter`** —— 包装现有 Ionic 组件实现为 ChatEngine 接口（零行为变更）
- **新增 `CopilotKitStyleEngine`** —— 模仿 CopilotKit 交互范式的 Vue 实现（Generative UI 卡片流式展开、状态共享面板、Suggestions chip bar）
- **新增 `TDesignEngine`** —— 封装 `@tdesign-vue-next/chat` 组件库（ChatBot + AG-UI 协议适配）
- **新增后端 AG-UI 协议输出层** —— 在现有 SSE 事件之上增加 AG-UI 标准事件映射（可选开启）
- **新增运行时引擎切换器** —— Settings 页面或 AgentChat 内嵌切换按钮，无刷新切换
- **保留所有现有组件不变** —— 作为 DefaultEngine 的内部实现

### 关键技术决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 默认引擎 | Default (Ionic) | 零迁移成本，用户无感知 |
| CopilotKit 实现 | **Vue 原生模仿**，非 React 嵌入 | Capacitor/Vue 项目嵌入 React 过重；模仿其交互范式即可 |
| TDesign 实现 | **真实集成** `@tdesign-vue-next/chat` | Vue3 原生组件库，可直接使用 |
| AG-UI 协议 | **后端双模式输出**（自定义事件 + AG-UI 事件） | 不破坏现有前端，TDesign 引擎走 AG-UI 路径 |
| A2UI 支持 | **作为未来扩展点**，不在本轮实现 | A2UI 需要 LLM 输出结构化 JSONL，当前 mock 场景不适用 |
| 切换方式 | localStorage 持久化 + reactive 响应式 | 无需刷新页面 |

### AG-UI 事件映射表

| 当前自定义事件 | AG-UI 标准事件 | 说明 |
|---------------|----------------|------|
| `stream_start` | `RUN_STARTED` | 生命周期开始 |
| `stream_end` | `RUN_FINISHED` | 生命周期结束 |
| `text_delta` | `TEXT_MESSAGE_CONTENT` | 流式文本增量 |
| `tool_call` | `TOOL_CALL_START` + `TOOL_CALL_ARGS` | 工具调用开始+参数 |
| `tool_status(running)` | —（隐含在 TOOL_CALL_START 中） | — |
| `tool_status(success)` | `TOOL_CALL_END` | 工具调用结束 |
| `tool_result` | `TOOL_CALL_RESULT` | 工具执行结果 |
| `text_delta_templated` | `TEXT_MESSAGE_CONTENT`（预处理后） | 模板在服务端渲染后推送 |
| （无） | `STATE_SNAPSHOT` / `STATE_DELTA` | 新增：共享状态同步 |

### CopilotKit 风格引擎特性（Vue 模仿实现）

参考 CopilotKit v1.50 的交互范式，用 Vue/Ionic 实现等效体验：

| CopilotKit 特性 | 本项目对应实现 |
|------------------|---------------|
| `useAgent` hook | 复用现有 `useAgent.ts`，包装为统一接口 |
| `CopilotChat` 组件 | 新建 `CopilotKitStyleChat.vue` |
| Generative UI（动态组件注册） | `useComponentRegistry()` composable |
| Suggestions API（chip bar） | 复用现有 Presets 机制 |
| State 共享面板 | 新增侧滑面板展示 agent state |
| Time Travel（消息回溯） | 未来扩展点，本轮预留接口 |
| Slot-based theming | 复用现有 `useTheme.ts` |

### TDesign Engine 特性

| TDesign Chat 能力 | 集成方式 |
|-------------------|----------|
| `ChatBot` 组件 | 直接引入 `<t-chatbot>` |
| AG-UI 协议解析 | 内置支持，配置 `protocol: 'agui'` |
| `useAgentToolcall` Hook | 注册工具调用组件映射 |
| `useAgentState` Hook | 订阅状态变更 |
| Markdown 渲染 | 内置，替代当前 markdown-it |
| 思考过程（Thinking） | 映射到 AG-UI `THINKING_*` 事件 |

## Impact

- **Affected specs**: agent-mock-mode（mock 引擎需支持 AG-UI 输出模式）
- **Affected code**:
  - `app/encv-mobile/src/views/AgentChat.vue` —— 接入引擎切换逻辑
  - `app/encv-mobile/src/composables/useAgent.ts` —— 可能需要包装为 ChatEngine interface
  - `app/encv-mobile/src/composables/renderTurnItems.ts` —— 成为 DefaultEngine 的内部实现
  - `app/encv-mobile/src/components/agent/*` —— 全部保留，仅归属关系变为 DefaultEngine 内部
  - `internal/server/agent_api.go` —— 新增 AG-UI 协议输出模式
  - `internal/server/agent_mock.go` —— MockEngine 支持双模式输出
  - **新增文件**: `composables/chatEngine.ts`, `engines/defaultEngine.ts`, `engines/copilotkitStyleEngine.ts`, `engines/tdesignEngine.ts`, `components/agent/engineSwitcher.vue`

## ADDED Requirements

### Requirement: ChatEngine 抽象接口

系统 SHALL 定义统一的 `ChatEngine` TypeScript 接口，包含以下能力：

```typescript
interface ChatEngine {
  /** 引擎唯一标识 */
  readonly id: string
  /** 显示名称 */
  readonly name: string
  /** 渲染消息列表（核心方法） */
  renderMessages(props: EngineRenderProps): VNode
  /** 渲染输入区域 */
  renderInput(props: EngineInputProps): VNode
  /** 处理用户发送 */
  onSend(text: string): Promise<void>
  /** 处理停止生成 */
  onStop(): void
  /** 引擎销毁时的清理 */
  destroy(): void
}
```

#### Scenario: 引擎切换时无缝过渡

- **WHEN** 用户从 Settings 或 AgentChat 内的切换器选择不同引擎
- **THEN** 当前引擎的 `destroy()` 被调用，新引擎被实例化并渲染，消息状态通过共享 store 传递不丢失
- **AND** 切换过程 < 100ms（仅 VNode 替换，不重新请求后端）

### Requirement: DefaultEngineAdapter（现有实现包装）

系统 SHALL 将现有的 AgentChat + renderTurnItems + 所有 agent 子组件包装为 `ChatEngine` 接口的默认实现。

- **约束**: 不修改任何现有组件的行为和样式
- **约束**: DefaultEngine 作为 fallback，当其他引擎加载失败时自动回退
- **验证**: 切换回 DefaultEngine 时 UI 与重构前完全一致（像素级对比）

### Requirement: CopilotKitStyleEngine（Vue 模仿实现）

系统 SHALL 提供 CopilotKit 交互风格的 Vue 实现，包含以下特性：

1. **Generative UI 卡片流** —— tool_call 结果以可折叠卡片形式逐步展开（复用现有 OperationCard 折叠态）
2. **Suggestions Chip Bar** —— 底部预设操作按钮（复用现有 MockPreset 机制）
3. **Agent State Panel** —— 可选侧滑面板显示当前 agent 内部状态
4. **Markdown 增强** —— 支持代码块语法高亮、表格、Mermaid 图表
5. **动画过渡** —— 消息出现/消失有平滑过渡效果（Ionic Animation API）

#### Scenario: 切换到 CopilotKit 风格

- **WHEN** 用户选择 "CopilotKit 风格" 引擎
- **THEN** 消息区域切换为新布局：左侧头像固定宽、右侧内容区更宽、工具调用卡片带渐变边框
- **AND** 底部出现 Suggestions 水平滚动条
- **AND** 发送消息后的流式渲染体验与 CopilotKit React 版一致（打字机效果 + 卡片依次展开）

### Requirement: TDesignEngine（真实集成）

系统 SHALL 集成 `@tdesign-vue-next/chat` 组件库作为可选渲染引擎。

1. **依赖安装**: `pnpm add @tdesign-vue-next/chat@alpha`
2. **AG-UI 模式**: 配置 `protocol: 'agui'` 让 TDesign 自动解析标准事件
3. **后端适配**: 当引擎为 TDesign 时，后端 SSE 输出切换为 AG-UI 格式
4. **工具调用映射**: 通过 `useAgentToolcall` 注册 list_files → MountListCard、read_file → FileContentCard 等
5. **样式融合**: TDesign 组件继承项目主题色变量（覆盖 TDesign 默认蓝为项目 primary 色）

#### Scenario: 切换到 TDesign 引擎

- **WHEN** 用户选择 "TDesign Chat" 引擎
- **THEN** 整个聊天区域替换为 TDesign ChatBot 组件
- **AND** 消息气泡样式遵循 TDesign 设计规范（圆角、阴影、间距）
- **AND** 工具调用以 TDesign 内置的 Activity 组件渲染
- **AND** Markdown 由 TDesign ChatMarkdown 组件渲染（内置代码高亮）

### Requirement: AG-UI 后端双模式输出

后端 MockEngine 和真实 Agent handler SHALL 支持双模式 SSE 输出：

- **模式 A（默认）**: 保持现有自定义事件格式（text_delta / tool_call / tool_result）—— 供 DefaultEngine 和 CopilotKitStyleEngine 使用
- **模式 B（AG-UI）**: 输出标准 AG-UI 事件（RUN_STARTED / TEXT_MESSAGE_CONTENT / TOOL_CALL_*）—— 供 TDesignEngine 使用

#### Scenario: TDesign 引擎请求 AG-UI 格式

- **WHEN** 前端请求头包含 `X-Agent-Protocol: agui` 或 query 参数 `?protocol=agui`
- **THEN** 后端 `handleAgentChat` 将事件转换为 AG-UI 格式后 flush
- **AND** 两种模式共享同一个 MockEngine.Run() 执行逻辑，仅在输出层做格式映射

### Requirement: 运行时引擎切换器

系统 SHALL 提供以下两种切换入口：

1. **AgentChat 内嵌微型切换器** —— 聊天区域顶部右侧的小型下拉/图标按钮
2. **Settings 页面选项** —— Agent 设置区域的"聊天引擎"选项

切换 SHALL：
- 使用 `localStorage('encv-chat-engine')` 持久化选择
- 使用 Vue `reactive/shallowRef` 实现响应式切换（无需刷新）
- 显示每种引擎的预览缩略图（可选）

#### Scenario: 用户切换引擎

- **GIVEN** 用户正在 AgentChat 页面查看对话
- **WHEN** 用户点击引擎切换器并选择 "TDesign Chat"
- **THEN** 当前视图立即切换为 TDesign 渲染（< 100ms）
- **AND** 已有的消息历史通过协议适配层重新渲染
- **AND** 后续新消息使用新引擎的渲染路径

### Requirement: A2UI 扩展预留

系统 SHALL 在 ChatEngine 接口中预留 A2UI 相关扩展点：

- `ChatEngine.supportsA2UI: boolean` —— 声明是否支持 A2UI Surface 渲染
- `ChatEngine.renderSurface(surfaceId: string, payload: unknown): VNode` —— 未来渲染 A2UI Surface
- 后端预留 `X-A2UI-Version` 请求头识别

> **注意**: 本轮不实现 A2UI 渲染器，仅预留接口。A2UI 需要 LLM 输出结构化 JSONL（beginRendering / surfaceUpdate / dataModelUpdate），当前 mock 场景暂不需要。

## MODIFIED Requirements

### Requirement: useAgent Composable（适配层）

现有 `useAgent.ts` SHALL 保持不变，但新增一个工厂函数 `createChatEngine(engineId)` 用于获取对应的引擎实例。useAgent 的核心状态（messages / status / eventLog）作为**共享数据源**被所有引擎读取。

### Requirement: AgentChat.vue（宿主容器）

AgentChat.vue 从"直接渲染组件"改造为"引擎宿主"：

- 模板中不再直接 import AssistantMessage / OperationCard 等子组件
- 改为 `<component :is="currentEngine.renderMessages(props)" />` 动态渲染
- 保留输入框区域（或委托给 engine.renderInput）
- 保留工具栏和设置入口
