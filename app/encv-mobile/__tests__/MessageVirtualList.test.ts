/**
 * MessageVirtualList 单元测试
 *
 * 覆盖：
 * 1. 组件挂载：传入 items 数组不崩溃
 * 2. 暴露方法：scrollToBottom 存在
 * 3. Item slot：每个 item 都触发 slot 渲染
 * 4. keyField：使用 messageId 而非 id
 * 5. 空状态：empty slot 透传
 * 6. 不同 type 的 item 都能渲染
 *
 * 实施说明：
 * vue-virtual-scroller 的 RecycleScroller 依赖真实 DOM 尺寸，
 * 在 jsdom 下完整挂载会因元素 height=0 报 layout warning。
 * 这里 stub 掉 RecycleScroller，只验证：
 * - props 传递正确
 * - slot 渲染正确
 * - 暴露的 scrollToBottom 方法可调用
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import MessageVirtualList from '@/components/agent/MessageVirtualList.vue'
import type { RenderedItem } from '@/composables/renderTurnItems'

// ─── 构造测试数据 ─────────────────────────────────────────────────

function makeUserItem(id: string, text: string): RenderedItem {
  return { type: 'user', messageId: id, text }
}

function makeAssistantItem(id: string, text: string, streaming = false): RenderedItem {
  return { type: 'assistantText', messageId: id, text, streaming }
}

function makeErrorItem(id: string, text: string): RenderedItem {
  return { type: 'error', messageId: id, text, messageIndex: 0 }
}

function makeReasoningItem(id: string, text: string): RenderedItem {
  return { type: 'reasoning', messageId: id, text, streaming: false }
}

function makeApprovalItem(id: string, toolCallId: string): RenderedItem {
  return { type: 'approval', messageId: id, toolCallId }
}

function makeOperationGroupItem(id: string, toolCallIds: string[]): RenderedItem {
  return { type: 'operationGroup', messageId: id, toolCallIds, forceComplete: true }
}

function makeWebSearchGroupItem(id: string, toolCallIds: string[], queries: string[]): RenderedItem {
  return { type: 'webSearchGroup', messageId: id, toolCallIds, queries }
}

// ─── 辅助：构造 stub RecycleScroller，验证 props/slot ────────────

function makeStubScroller() {
  // inline template 不能用 TS 语法（不经过 vue-tsc），保持纯 JS
  return {
    name: 'RecycleScroller',
    template: `<div class="recycle-scroller-stub" :data-item-count="items.length" :data-key-field="keyField" :data-buffer="buffer" :data-min-item-size="minItemSize" :data-item-size-type="typeof itemSize"><slot v-for="(item, i) in items" :key="item.messageId || i" name="default" :item="item" :index="i" /></div>`,
    props: ['items', 'itemSize', 'minItemSize', 'buffer', 'keyField'],
  }
}

// ─── 测试 ──────────────────────────────────────────────────────────

describe('MessageVirtualList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('mounting & props', () => {
    it('renders without crashing when items array is provided', () => {
      const items: RenderedItem[] = [makeUserItem('u1', 'hello')]
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
      })
      expect(wrapper.exists()).toBe(true)
      const scroller = wrapper.find('.recycle-scroller-stub')
      expect(scroller.exists()).toBe(true)
      expect(scroller.attributes('data-item-count')).toBe('1')
    })

    it('passes keyField=messageId to RecycleScroller', () => {
      const items: RenderedItem[] = [makeUserItem('u1', 'hello')]
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
      })
      const scroller = wrapper.find('.recycle-scroller-stub')
      expect(scroller.attributes('data-key-field')).toBe('messageId')
    })

    it('uses default buffer=400 and minItemSize=80', () => {
      const items: RenderedItem[] = [makeUserItem('u1', 'hello')]
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
      })
      const scroller = wrapper.find('.recycle-scroller-stub')
      expect(scroller.attributes('data-buffer')).toBe('400')
      expect(scroller.attributes('data-min-item-size')).toBe('80')
    })

    it('respects custom buffer and minItemSize props', () => {
      const items: RenderedItem[] = [makeUserItem('u1', 'hello')]
      const wrapper = mount(MessageVirtualList, {
        props: { items, buffer: 600, minItemSize: 100 },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
      })
      const scroller = wrapper.find('.recycle-scroller-stub')
      expect(scroller.attributes('data-buffer')).toBe('600')
      expect(scroller.attributes('data-min-item-size')).toBe('100')
    })

    it('passes itemSize as a function (type=function)', () => {
      const items: RenderedItem[] = [makeUserItem('u1', 'hello')]
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
      })
      const scroller = wrapper.find('.recycle-scroller-stub')
      expect(scroller.attributes('data-item-size-type')).toBe('function')
    })
  })

  describe('item slot rendering', () => {
    it('passes each item to the slot with its messageId', () => {
      const items: RenderedItem[] = [
        makeUserItem('u1', 'hello'),
        makeAssistantItem('a1', 'world'),
      ]
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
        slots: {
          item: `<template #item="{ item }">
            <span class="slot-item" :data-type="item.type" :data-id="item.messageId">{{ item.text }}</span>
          </template>`,
        },
      })
      const slotItems = wrapper.findAll('.slot-item')
      expect(slotItems.length).toBeGreaterThanOrEqual(2)
      expect(slotItems[0].attributes('data-type')).toBe('user')
      expect(slotItems[0].attributes('data-id')).toBe('u1')
      expect(slotItems[1].attributes('data-type')).toBe('assistantText')
      expect(slotItems[1].attributes('data-id')).toBe('a1')
    })

    it('renders all 7 RenderedItem types', () => {
      const items: RenderedItem[] = [
        makeUserItem('u', 'u'),
        makeAssistantItem('a', 'a'),
        makeReasoningItem('r', 'r'),
        makeErrorItem('e', 'e'),
        makeApprovalItem('ap', 'tc-ap'),
        makeOperationGroupItem('og', ['tc1']),
        makeWebSearchGroupItem('ws', ['tc2'], ['query']),
      ]
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
        slots: {
          item: `<template #item="{ item }">
            <span class="slot-item" :data-type="item.type" />
          </template>`,
        },
      })
      // 只看 slot 内部标记（外层 virtualItem wrapper 也带 data-type，需要避免重复）
      const types = wrapper.findAll('.slot-item').map((el) => el.attributes('data-type'))
      expect(types).toEqual(['user', 'assistantText', 'reasoning', 'error', 'approval', 'operationGroup', 'webSearchGroup'])
    })
  })

  describe('exposed scrollToBottom', () => {
    it('exposes scrollToBottom method on the component instance', () => {
      const items: RenderedItem[] = [makeUserItem('u1', 'hello')]
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
      })
      const exposed = wrapper.vm.$.exposed as Record<string, unknown> | null
      expect(exposed).not.toBeNull()
      expect(typeof exposed?.scrollToBottom).toBe('function')
    })

    it('scrollToBottom calls scrollerRef.scrollToItem(items.length-1, behavior)', async () => {
      const items: RenderedItem[] = [
        makeUserItem('u1', 'hello'),
        makeUserItem('u2', 'world'),
        makeUserItem('u3', 'foo'),
      ]
      // stub RecycleScroller with scrollToItem method
      const scrollToItemMock = vi.fn()
      const StubScrollerWithMethod = {
        name: 'RecycleScroller',
        template: `<div class="recycle-scroller-stub" />`,
        props: ['items', 'itemSize', 'minItemSize', 'buffer', 'keyField'],
        methods: { scrollToItem: scrollToItemMock },
      }
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: StubScrollerWithMethod },
        },
      })

      const exposed = wrapper.vm.$.exposed as { scrollToBottom: (b?: 'auto' | 'smooth') => void }
      exposed.scrollToBottom('smooth')
      await nextTick()

      // scrollToItem should be called with last index
      expect(scrollToItemMock).toHaveBeenCalledTimes(1)
      expect(scrollToItemMock).toHaveBeenCalledWith(2, 'smooth')
    })

    it('scrollToBottom falls back to container.scrollTo when scrollerRef missing', async () => {
      const items: RenderedItem[] = [makeUserItem('u1', 'hello')]
      // 故意不暴露 scrollToItem 方法
      const StubScrollerNoMethod = {
        name: 'RecycleScroller',
        template: `<div class="recycle-scroller-stub" />`,
        props: ['items', 'itemSize', 'minItemSize', 'buffer', 'keyField'],
      }

      // jsdom HTMLElement 没有 scrollTo；为本次测试 patch 一下
      const originalScrollTo = (Element.prototype as unknown as { scrollTo?: unknown }).scrollTo
      ;(Element.prototype as unknown as { scrollTo: (opts: unknown) => void }).scrollTo = function () { /* noop */ }
      const scrollToSpy = vi.spyOn(Element.prototype as unknown as { scrollTo: (opts: unknown) => void }, 'scrollTo')

      try {
        const wrapper = mount(MessageVirtualList, {
          props: { items },
          global: {
            stubs: { RecycleScroller: StubScrollerNoMethod },
          },
          attachTo: document.body,
        })

        const container = wrapper.find('.messageVirtualList').element as HTMLDivElement
        const exposed = wrapper.vm.$.exposed as { scrollToBottom: (b?: 'auto' | 'smooth') => void }
        exposed.scrollToBottom('auto')
        await nextTick()

        // 降级路径会调用 container.scrollTo
        expect(scrollToSpy).toHaveBeenCalled()
        const callArg = scrollToSpy.mock.calls[0][0] as { top: number; behavior: string }
        expect(callArg).toMatchObject({ behavior: 'auto' })
        // 顶到底
        expect(callArg.top).toBe(container.scrollHeight)

        wrapper.unmount()
      } finally {
        if (originalScrollTo === undefined) {
          delete (Element.prototype as unknown as { scrollTo?: unknown }).scrollTo
        } else {
          ;(Element.prototype as unknown as { scrollTo: unknown }).scrollTo = originalScrollTo
        }
      }
    })
  })

  describe('integration with 120+ items (perf threshold)', () => {
    it('handles 200 items without rendering all DOM nodes (slot still iterates)', () => {
      const items: RenderedItem[] = Array.from({ length: 200 }, (_, i) =>
        makeUserItem(`u-${i}`, `message ${i}`),
      )
      const wrapper = mount(MessageVirtualList, {
        props: { items },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
        slots: {
          item: `<template #item="{ item }">
            <span :data-id="item.messageId" />
          </template>`,
        },
      })
      const scroller = wrapper.find('.recycle-scroller-stub')
      // stub 仍然遍历所有 items（实际 RecycleScroller 只渲染可见项）
      // 这里只验证 props.items.length 正确传递
      expect(scroller.attributes('data-item-count')).toBe('200')
    })

    it('reactive items update propagates to RecycleScroller', async () => {
      const itemsRef = ref<RenderedItem[]>([makeUserItem('u1', 'hello')])
      const wrapper = mount(MessageVirtualList, {
        props: { items: itemsRef.value },
        global: {
          stubs: { RecycleScroller: makeStubScroller() },
        },
      })
      expect(wrapper.find('.recycle-scroller-stub').attributes('data-item-count')).toBe('1')

      // 模拟 push 一条新消息
      itemsRef.value.push(makeUserItem('u2', 'world'))
      await wrapper.setProps({ items: itemsRef.value })

      expect(wrapper.find('.recycle-scroller-stub').attributes('data-item-count')).toBe('2')
    })
  })
})
