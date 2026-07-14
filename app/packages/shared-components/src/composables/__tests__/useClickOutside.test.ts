/**
 * useClickOutside 单测（happy-dom 环境）
 *
 * 覆盖：
 * 1. 点击目标外部 → handler 触发
 * 2. 点击目标内部 → handler 不触发
 * 3. Escape 键 → handler 触发（默认 closeOnEscape=true）
 * 4. closeOnEscape=false → Escape 不触发
 * 5. unmount 后监听移除 → 不再触发
 * 6. 与 useDisclosure 集成：外部点击关闭 isOpen
 */
import { createApp, defineComponent, h, ref } from "vue";
import { describe, expect, it, vi } from "vitest";
import { useClickOutside } from "../useClickOutside";
import { useDisclosure } from "../useDisclosure";

/** 在真实组件实例里跑 composable，以触发 onMounted / onBeforeUnmount */
function mountComposable<T>(factory: () => T): { result: T; unmount: () => void } {
  let result!: T;
  const Comp = defineComponent({
    setup() {
      result = factory();
      return () => h("div");
    },
  });
  const el = document.createElement("div");
  document.body.appendChild(el);
  const app = createApp(Comp);
  app.mount(el);
  return {
    result,
    unmount: () => app.unmount(),
  };
}

function fireMouseDown(target: HTMLElement) {
  target.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
}
function fireEscape() {
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
}

describe("useClickOutside", () => {
  it("点击目标外部触发 handler", () => {
    const target = ref<HTMLElement>(document.createElement("div"));
    document.body.appendChild(target.value);
    const handler = vi.fn();
    const { unmount } = mountComposable(() => {
      useClickOutside(target, handler);
      return null;
    });

    const outside = document.createElement("div");
    document.body.appendChild(outside);
    fireMouseDown(outside);

    expect(handler).toHaveBeenCalledTimes(1);
    unmount();
  });

  it("点击目标内部不触发 handler", () => {
    const target = ref<HTMLElement>(document.createElement("div"));
    document.body.appendChild(target.value);
    const handler = vi.fn();
    mountComposable(() => {
      useClickOutside(target, handler);
      return null;
    });

    fireMouseDown(target.value);
    expect(handler).not.toHaveBeenCalled();
  });

  it("Escape 键触发 handler（默认 closeOnEscape=true）", () => {
    const target = ref<HTMLElement>(document.createElement("div"));
    const handler = vi.fn();
    mountComposable(() => {
      useClickOutside(target, handler);
      return null;
    });

    fireEscape();
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it("closeOnEscape=false 时 Escape 不触发", () => {
    const target = ref<HTMLElement>(document.createElement("div"));
    const handler = vi.fn();
    mountComposable(() => {
      useClickOutside(target, handler, { closeOnEscape: false });
      return null;
    });

    fireEscape();
    expect(handler).not.toHaveBeenCalled();
  });

  it("unmount 后监听移除，不再触发", () => {
    const target = ref<HTMLElement>(document.createElement("div"));
    document.body.appendChild(target.value);
    const handler = vi.fn();
    const { unmount } = mountComposable(() => {
      useClickOutside(target, handler);
      return null;
    });
    unmount();

    const outside = document.createElement("div");
    document.body.appendChild(outside);
    fireMouseDown(outside);
    expect(handler).not.toHaveBeenCalled();
  });

  it("与 useDisclosure 集成：外部点击关闭 isOpen", () => {
    const target = ref<HTMLElement>(document.createElement("div"));
    document.body.appendChild(target.value);
    const { result, unmount } = mountComposable(() => {
      const d = useDisclosure(true);
      useClickOutside(target, () => d.close());
      return d;
    });
    expect(result.isOpen.value).toBe(true);

    const outside = document.createElement("div");
    document.body.appendChild(outside);
    fireMouseDown(outside);
    expect(result.isOpen.value).toBe(false);
    unmount();
  });
});
