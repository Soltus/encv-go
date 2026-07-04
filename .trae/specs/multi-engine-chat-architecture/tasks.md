# Tasks

## Phase 1: 基础抽象层（引擎接口 + 切换机制）

- [ ] Task 1.1: 定义 `ChatEngine` TypeScript 接口和 `EngineRenderProps` / `EngineInputProps` 类型
  - [ ] 1.1.1 在 `composables/chatEngine.ts` 中定义 ChatEngine interface（id, name, renderMessages, renderInput, onSend, onStop, destroy）
  - [ ] 1.1.2 定义 EngineContext 类型（共享状态：messages, status, eventLog, sendMessage, stopGeneration 等）
  - [ ] 1.1.3 定义 EngineRegistry 映射表类型（Map<string, () => ChatEngine>）

- [ ] Task 1.2: 实现运行时引擎切换器 composable
  - [ ] 1.2.1 创建 `composables/useChatEngine.ts` —— useChatEngine() 返回 { currentEngine, switchEngine, engineList, supportsA2UI }
  - [ ] 1.2.2 引擎选择持久化到 localStorage('encv-chat-engine')
  - [ ] 1.2.3 响应式切换：shallowRef<ChatEngine> + destroy/实例化生命周期
  - [ ] 1.2.4 引擎加载失败时自动 fallback 到 'default'

- [ ] Task 1.3: 实现 AgentChat.vue 宿主容器改造
  - [ ] 1.3.1 将现有模板内容提取为 DefaultEngine 的 renderMessages() 返回值
  - [ ] 1.3.2 AgentChat 改为动态 component 渲染：`<component :is="currentEngine.renderMessages(engineCtx)" />`
  - [ ] 1.3.3 添加内嵌引擎切换器 UI（顶部右侧小型下拉按钮）
  - [ ] 1.3.4 验证：切换回 default 引擎时 UI 与改造前完全一致

## Phase 2: DefaultEngineAdapter（包装现有实现）

- [ ] Task 2.1: 创建 DefaultEngineAdapter
  - [ ] 2.1.1 创建 `engines/defaultEngine.ts` —— 实现 ChatEngine 接口
  - [ ] 2.1.2 内部复用现有 renderTurnItems.ts 的 renderTurnItems() / renderAgentFlow()
  - [ ] 2.1.3 内部复用现有所有 agent 子组件（AssistantMessage, OperationCard, FileListCard 等）
  - [ ] 2.1.4 renderMessages() 返回包含完整消息列表的 VNode（Fragment 或虚拟滚动容器）
  - [ ] 2.1.5 注册到 EngineRegistry，id = 'default', name = 'Ionic 默认'

## Phase 3: CopilotKitStyleEngine（Vue 模仿实现）

- [ ] Task 3.1: 创建 CopilotKit 风格布局组件
  - [ ] 3.1.1 创建 `components/agent/copilotkit/CopilotKitStyleChat.vue` —— 主容器
  - [ ] 3.1.2 布局差异：左侧固定宽头像区(48px) + 右侧更宽内容区 + 工具调用卡片渐变边框
  - [ ] 3.1.3 消息出现/消失过渡动画（Ionic Animation API 或 Vue Transition）

- [ ] Task 3.2: 实现 Suggestions Chip Bar
  - [ ] 3.2.1 复用现有 MockPreset 数据源，渲染为底部水平滚动 chip 条
  - [ ] 3.2.2 chip 点击触发发送预设文本（与现有 Preset 行为一致）

- [ ] Task 3.3: 实现增强 Markdown 渲染
  - [ ] 3.3.1 在 CopilotKit 风格中使用更强的 markdown 渲染（代码块语法高亮、Mermaid 支持）
  - [ ] 3.3.2 可选：集成 `shiki` 或 `highlight.js` 用于代码高亮

- [ ] Task 3.4: 注册 CopilotKitStyleEngine 到 EngineRegistry
  - [ ] 3.4.1 创建 `engines/copilotkitStyleEngine.ts`
  - [ ] 3.4.2 id = 'copilotkit-style', name = 'CopilotKit 风格'
  - [ ] 3.4.3 supportsA2UI = false（本轮不支持）

## Phase 4: TDesignEngine（真实集成 @tdesign-vue-next/chat）

- [ ] Task 4.1: 安装和配置 TDesign Chat 依赖
  - [ ] 4.1.1 执行 `pnpm add @tdesign-vue-next/chat@alpha`
  - [ ] 4.1.2 在 main.ts 或按需引入 TDesign Chat 组件
  - [ ] 4.1.3 配置主题色覆盖（TDesign 默认蓝 → 项目 primary 色）

- [ ] Task 4.2: 创建 TDesignEngine 包装器
  - [ ] 4.2.1 创建 `engines/tdesignEngine.ts`
  - [ ] 4.2.2 内部使用 `<t-chatbot>` 组件作为渲染核心
  - [ ] 4.2.3 配置 chatServiceConfig：endpoint 指向当前后端、protocol: 'agui'、stream: true
  - [ ] 4.2.4 通过 useAgentToolcall 注册工具调用映射：
    - list_files → 复用 MountListCard/FileListCard（包装为 TDesign 兼容格式）
    - read_file → 复用 FileContentCard
    - shell_command → 新建 TerminalOutputCard
    - write_file → 新建 WriteConfirmCard
  - [ ] 4.2.5 id = 'tdesign', name = 'TDesign Chat'

- [ ] Task 4.3: 后端 AG-UI 协议输出模式
  - [ ] 4.3.1 在 `agent_api.go` handleAgentChat 中检测请求协议模式（header X-Agent-Protocol 或 query ?protocol=agui）
  - [ ] 4.3.2 创建 `agent_agui_adapter.go` —— AGUIEventMapper 结构体，将内部事件映射为 AG-UI 格式
  - [ ] 4.3.3 映射规则：
    - stream_start → RUN_STARTED (JSON)
    - text_delta → TEXT_MESSAGE_CONTENT (delta 字段)
    - tool_call → TOOL_CALL_START + TOOL_CALL_ARGS
    - tool_status(success) → TOOL_CALL_END
    - tool_result → TOOL_CALL_RESULT
    - stream_end → RUN_FINISHED
  - [ ] 4.3.4 MockEngine.Run() 增加 aguiMode bool 参数，控制输出格式
  - [ ] 4.3.5 go build + vue-tsc 验证通过

## Phase 5: A2UI 扩展预留

- [ ] Task 5.1: ChatEngine 接口扩展 A2UI 预留字段
  - [ ] 5.1.1 ChatEngine interface 新增 `readonly supportsA2UI: boolean`
  - [ ] 5.1.2 ChatEngine interface 新增 `renderSurface?(surfaceId: string, payload: unknown): VNode`
  - [ ] 5.1.3 所有已实现引擎设置 supportsA2UI = false

- [ ] Task 5.2: 后端 A2UI 识别预留
  - [ ] 5.2.1 handleAgentChat 识别 `X-A2UI-Version` 请求头（仅记录日志，不处理）
  - [ ] 5.2.2 MockEngine 预留 A2UI 输出分支（注释占位）

## Phase 6: 设置页集成 + 全量验证

- [ ] Task 6.1: Settings 页面添加引擎选择器
  - [ ] 6.1.1 在 Agent 相关设置区域增加"聊天引擎"选项（ion-select 或自定义选择器）
  - [ ] 6.1.2 显示每种引擎的名称和简短描述
  - [ ] 6.1.3 选择变更时调用 switchEngine()

- [ ] Task 6.2: 全量验证
  - [ ] 6.2.1 DefaultEngine: 发送 "帮我全面分析" 触发 complex_workflow，验证 UI 与改造前一致
  - [ ] 6.2.2 CopilotKitStyleEngine: 切换后发送同一条消息，验证新布局+动画+chip bar
  - [ ] 6.2.3 TDesignEngine: 切换后发送消息，验证 TDesign 组件正确渲染 + AG-UI 协议解析
  - [ ] 6.2.4 引擎切换：在对话过程中切换引擎，验证无消息丢失、无状态异常
  - [ ] 6.2.5 vue-tsc --noEmit 零错误
  - [ ] 6.2.6 go build ./internal/server/ 零错误
  - [ ] 6.2.7 浏览器端到端验证三种引擎的流式渲染效果

# Task Dependencies
- [Task 1.1] → [Task 1.2] → [Task 1.3] → [Task 2.1]（基础层必须先完成）
- [Task 2.1] 和 [Task 3.x] 可并行（DefaultEngine 和 CopilotKit 风格独立实现）
- [Task 4.1] → [Task 4.2] → [Task 4.3]（TDesign 依赖链）
- [Task 5.x] 可与 Phase 4 并行（纯预留工作）
- [Task 6.x] 必须在所有引擎实现完成后执行
