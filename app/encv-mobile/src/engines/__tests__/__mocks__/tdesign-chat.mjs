/**
 * tdesign-chat.mjs - 测试环境 stub for @tdesign-vue-next/chat
 *
 * 原因：TDesign chat 包在 pnpm 严格模式下，Node.js 原生 resolver 无法解析
 * （包只声明 module 字段，无 main 字段）。在生产构建（Vite）中工作正常，
 * 但在 Vitest 测试环境会报 "Failed to resolve import" 错误。
 *
 * 本 stub 暴露 ChatList / ChatItem / ChatThinking 占位组件，足够支撑
 * TDesignChatView 组件的渲染测试。具体渲染样式由 E2E 验证。
 */
import { defineComponent, h } from 'vue'

const Stub = defineComponent({
  name: 'StubChat',
  props: ['data', 'role', 'name', 'datetime', 'avatar', 'content'],
  setup(props, { slots }) {
    return () =>
      h('div', { class: 'td-chat-stub', 'data-role': props.role ?? 'unknown' }, [
        props.content ?? (slots.content ? '[slot]' : ''),
      ])
  },
})

export const ChatList = { ...Stub, name: 'ChatList' }
export const ChatItem = { ...Stub, name: 'ChatItem' }
export const ChatThinking = { ...Stub, name: 'ChatThinking' }

export default { ChatList, ChatItem, ChatThinking }
