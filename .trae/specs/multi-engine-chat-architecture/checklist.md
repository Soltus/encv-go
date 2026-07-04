# Checklist

## Phase 1: 基础抽象层
- [ ] ChatEngine TypeScript 接口定义完整（id, name, renderMessages, renderInput, onSend, onStop, destroy, supportsA2UI, renderSurface?）
- [ ] EngineContext 类型包含所有共享状态字段（messages, status, eventLog, sendMessage, stopGeneration 等）
- [ ] useChatEngine() composable 实现引擎切换逻辑（localStorage 持久化 + reactive + fallback）
- [ ] AgentChat.vue 改造为动态 component 渲染宿主容器
- [ ] 内嵌引擎切换器 UI 可见且功能正常
- [ ] 切换回 default 引擎时 UI 与改造前像素级一致

## Phase 2: DefaultEngineAdapter
- [ ] DefaultEngineAdapter 实现 ChatEngine 接口全部方法
- [ ] 内部复用 renderTurnItems.ts 无修改（仅调用关系变化）
- [ ] 内部复用所有 agent 子组件无行为变更
- [ ] renderMessages() 返回正确的 VNode 结构
- [ ] 引擎注册到 EngineRegistry，id='default'

## Phase 3: CopilotKitStyleEngine
- [ ] CopilotKitStyleChat.vue 布局与 Ionic 默认有明显视觉差异（更宽内容区、渐变边框卡片）
- [ ] Suggestions Chip Bar 在底部正确显示预设操作
- [ ] chip 点击触发发送预设文本并收到响应
- [ ] 消息出现/消失有平滑过渡动画
- [ ] 引擎注册到 EngineRegistry，id='copilotkit-style'
- [ ] supportsA2UI = false

## Phase 4: TDesignEngine + AG-UI 后端
- [ ] @tdesign-vue-next/chat@alpha 安装成功且可 import
- [ ] TDesignEngine 内部使用 t-chatbot 组件渲染
- [ ] chatServiceConfig 配置正确（endpoint, protocol:'agui', stream:true）
- [ ] useAgentToolcall 注册了 list_files / read_file / shell_command / write_file 的组件映射
- [ ] 主题色覆盖生效（非 TDesign 默认蓝）
- [ ] 后端 AG-UI 协议模式可通过 X-Agent-Protocol header 或 ?protocol=agui 触发
- [ ] AGUIEventMapper 正确映射全部 7 种事件类型
- [ ] MockEngine 支持 aguiMode 参数
- [ ] go build 零错误

## Phase 5: A2UI 扩展预留
- [ ] ChatEngine interface 包含 supportsA2UI 和 renderSurface 可选字段
- [ ] 所有引擎实现设置 supportsA2UI = false
- [ ] 后端识别 X-A2UI-Version header 并记录日志

## Phase 6: 设置页集成 + 全量验证
- [ ] Settings 页面有"聊天引擎"选择器 UI
- [ ] 选择器列出 default / copilotkit-style / tdesign 三种选项
- [ ] 选择变更后引擎立即切换（< 100ms）
- [ ] DefaultEngine 流式渲染 complex_workflow 正常（6 tool_call + 动态取值）
- [ ] CopilotKitStyleEngine 流式渲染正常（新布局 + chip bar + 动画）
- [ ] TDesignEngine 流式渲染正常（TDesign 组件 + AG-UI 事件解析）
- [ ] 对话过程中切换引擎无消息丢失、无 JS 报错
- [ ] vue-tsc --noEmit 零错误
- [ ] go build ./internal/server/ 零错误
