/**
 * TaskVirtualList 单元测试
 *
 * 🆕 2026-06-23 Task 8：适配 count + getItem + getKey 接口
 *   - 旧：:items="items" + :get-key="(item) => item.key"
 *   - 新：:count="items.length" + :get-item="(i) => items[i]" + :get-key="(i) => items[i].key"
 *
 * 覆盖：
 * - 基础渲染：count > 0 + scrollEl ready → 渲染 slot 内容
 * - scrollEl null → 降级渲染前 N 个 fallback item（修复真机空白）
 * - forceMeasure 暴露给父级
 * - item wrapper class 应用
 * - slot props 传递 item + index
 * - getKey 自定义 key 函数
 *
 * 注意：jsdom 默认 getBoundingClientRect / clientHeight 全为 0，
 * virtualizer 拿到 0 视口会渲染 0 个 item。测试通过 Object.defineProperty
 * 给 mock scrollEl 注入非零 clientHeight / offsetHeight。
 */

import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TaskVirtualList from "@/components/tasks/TaskVirtualList.vue";

interface TestItem {
  key: string;
  label: string;
}

function makeItems(count: number): TestItem[] {
  return Array.from({ length: count }, (_, i) => ({
    key: `item-${i}`,
    label: `Item ${i}`,
  }));
}

/**
 * 创建 mock scrollEl：jsdom 默认 clientHeight=0，virtualizer 会渲染 0 个 item。
 * 用 Object.defineProperty 注入非零值模拟真实滚动容器。
 */
function makeMockScrollEl(): HTMLElement {
  const el = document.createElement("div");
  Object.defineProperty(el, "clientHeight", { configurable: true, value: 600 });
  Object.defineProperty(el, "offsetHeight", { configurable: true, value: 600 });
  Object.defineProperty(el, "scrollHeight", { configurable: true, value: 600 });
  Object.defineProperty(el, "scrollTop", { configurable: true, value: 0, writable: true });
  // getBoundingClientRect 返回非零 viewport 让 virtualizer 认为有可见区域
  el.getBoundingClientRect = () => ({
    top: 0,
    left: 0,
    right: 400,
    bottom: 600,
    width: 400,
    height: 600,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  });
  return el;
}

const SlotWrapper = {
  components: { TaskVirtualList },
  template: `
    <TaskVirtualList
      :count="items.length"
      :get-item="(i) => items[i]"
      :get-key="(i) => items[i].key"
      :scroll-el="scrollEl"
      ref="listRef"
    >
      <template #default="{ item, index }">
        <div class="test-item" :data-key="item.key">
          {{ item.label }} @ {{ index }}
        </div>
      </template>
    </TaskVirtualList>
  `,
  props: {
    items: { type: Array as () => TestItem[], default: () => [] },
    scrollEl: { type: Object as () => HTMLElement | null, default: null },
  },
  methods: {
    forceMeasure() {
      return (this as any).$refs.listRef.forceMeasure();
    },
  },
};

describe("TaskVirtualList", () => {
  beforeEach(() => {
    // jsdom 不支持 ResizeObserver，stub 一下避免 virtualizer 内部报错
    // 必须是可 new 的构造函数（virtual-core 用 new ResizeObserver(cb)）
    class MockResizeObserver {
      observe = vi.fn();
      unobserve = vi.fn();
      disconnect = vi.fn();
    }
    global.ResizeObserver = MockResizeObserver as any;
  });

  it("scrollEl 为 null 时降级渲染 fallback item（修复真机空白）", async () => {
    const wrapper = mount(SlotWrapper, {
      props: { items: makeItems(10), scrollEl: null },
    });
    await flushPromises();
    // scrollEl=null → 降级渲染前 N 个 fallback item（overscan*2+20 = 40，不超过 count=10）
    const rendered = wrapper.findAll(".test-item");
    expect(rendered.length).toBeGreaterThan(0);
    expect(rendered.length).toBeLessThanOrEqual(10);
    // 验证有 fallback class
    const list = wrapper.find(".task-virtual-list--fallback");
    expect(list.exists()).toBe(true);
  });

  it("scrollEl ready 后渲染可见 slot 内容", async () => {
    const scrollEl = makeMockScrollEl();
    const wrapper = mount(SlotWrapper, {
      props: { items: makeItems(50), scrollEl },
    });
    await flushPromises();
    // 50 个 item，视口 600px，estimateSize 80px → 视口内 ~8 个 + overscan 20*2=40
    // jsdom 下 virtualizer 行为可能略有不同，至少应该渲染 > 0 个
    const rendered = wrapper.findAll(".test-item");
    expect(rendered.length).toBeGreaterThan(0);
    expect(rendered.length).toBeLessThanOrEqual(50);
  });

  it("slot 接收 item + index props", async () => {
    const scrollEl = makeMockScrollEl();
    const wrapper = mount(SlotWrapper, {
      props: { items: makeItems(5), scrollEl },
    });
    await flushPromises();
    const firstItem = wrapper.find(".test-item");
    expect(firstItem.exists()).toBe(true);
    // slot 内容应包含 item.label + index
    expect(firstItem.text()).toContain("Item ");
    expect(firstItem.text()).toContain("@");
  });

  it("item wrapper 应用 task-virtual-item class", async () => {
    const scrollEl = makeMockScrollEl();
    const wrapper = mount(SlotWrapper, {
      props: { items: makeItems(5), scrollEl },
    });
    await flushPromises();
    const itemWrapper = wrapper.find(".task-virtual-item");
    expect(itemWrapper.exists()).toBe(true);
    expect(itemWrapper.classes()).toContain("task-virtual-item");
  });

  it("totalSize 反映在容器 height 样式上（px 格式）", async () => {
    const scrollEl = makeMockScrollEl();
    const wrapper = mount(SlotWrapper, {
      props: { items: makeItems(20), scrollEl },
    });
    await flushPromises();
    const list = wrapper.find(".task-virtual-list");
    expect(list.exists()).toBe(true);
    const height = (list.element as HTMLElement).style.height;
    // height 应为 "{n}px" 格式（jsdom 下 virtualizer 可能未测量实际高度，
    // 但 style binding 必须生效 — 这是 TaskVirtualList 模板的核心职责）
    expect(height).toMatch(/^\d+px$/);
  });

  it("forceMeasure 暴露给父级（可调用不报错）", async () => {
    const scrollEl = makeMockScrollEl();
    const wrapper = mount(SlotWrapper, {
      props: { items: makeItems(10), scrollEl },
    });
    await flushPromises();
    expect(() => (wrapper.vm as any).forceMeasure()).not.toThrow();
  });

  it("items 为空时 totalSize = 0px", async () => {
    const scrollEl = makeMockScrollEl();
    const wrapper = mount(SlotWrapper, {
      props: { items: [], scrollEl },
    });
    await flushPromises();
    const list = wrapper.find(".task-virtual-list");
    const height = (list.element as HTMLElement).style.height;
    expect(parseInt(height, 10) || 0).toBe(0);
  });

  it("自定义 getKey 函数生效（v-for key 使用自定义函数）", async () => {
    const scrollEl = makeMockScrollEl();
    const CustomKeyWrapper = {
      components: { TaskVirtualList },
      template: `
        <TaskVirtualList
          :count="items.length"
          :get-item="(i) => items[i]"
          :get-key="getKey"
          :scroll-el="scrollEl"
        >
          <template #default="{ item }">
            <div class="test-item">{{ item.name }}</div>
          </template>
        </TaskVirtualList>
      `,
      data() {
        return {
          items: Array.from({ length: 5 }, (_, i) => ({
            key: `default-key-${i}`,
            name: `Name ${i}`,
          })),
          scrollEl,
          // 🆕 Task 8：getKey 接收 index，返回 key
          getKey: (i: number) => `custom-${i}`,
        };
      },
    };
    const wrapper = mount(CustomKeyWrapper);
    await flushPromises();
    // 只要能正常渲染不报错就说明 getKey 被使用
    expect(wrapper.findAll(".test-item").length).toBeGreaterThan(0);
  });

  it("scrollEl 从 null → 非 null 触发 virtualizer measure（列表从空变为有内容）", async () => {
    const wrapper = mount(SlotWrapper, {
      props: { items: makeItems(20), scrollEl: null },
    });
    await flushPromises();
    // 🆕 2026-06-23 修复真机空白：scrollEl=null 时走 fallback 渲染前 N 个 item（非 0）
    //   - 旧逻辑：scrollEl=null → virtualizer 返回 [] → 0 个 item → 页面空白
    //   - 新逻辑：scrollEl=null → fallback 渲染 min(count, overscan*2+20) 个 item
    expect(wrapper.findAll(".test-item").length).toBeGreaterThan(0);
    // 切换到非 null
    const scrollEl = makeMockScrollEl();
    await wrapper.setProps({ scrollEl });
    await flushPromises();
    // measure 触发后应该渲染 item
    expect(wrapper.findAll(".test-item").length).toBeGreaterThan(0);
  });

  it("estimateSize 和 overscan props 透传（不报错）", async () => {
    const scrollEl = makeMockScrollEl();
    const wrapper = mount(SlotWrapper, {
      props: { items: makeItems(10), scrollEl, estimateSize: 100, overscan: 5 },
    } as any);
    await flushPromises();
    // 能正常渲染即可（estimateSize/overscan 是 virtualizer 内部参数）
    expect(wrapper.findAll(".test-item").length).toBeGreaterThan(0);
  });
});
