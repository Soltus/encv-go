/**
 * useClickOutside — 「点击目标元素外部即触发回调」的最小抽象。
 *
 * 统一替代各组件手搓的：
 *   - `document.addEventListener("mousedown", handleClickOutside)`
 *   - `document.addEventListener("keydown", handleEscape)`（closeOnEscape）
 *   - `onMounted` 注册 + `onBeforeUnmount` 清理
 *
 * 与 useDisclosure 配合即可完整收敛「手搓 dropdown / popover」样板。
 *
 * 行为契约（与 FilterDropdown 原手搓实现等价）：
 *   - 仅当 target 存在且事件目标不在 target 内时触发 handler；
 *   - handler 在 target 为 null 时静默跳过（不抛错）；
 *   - Escape 默认一并处理（closeOnEscape=true），handler 幂等（已关时再关无副作用）。
 */
import { onBeforeUnmount, onMounted, type Ref } from "vue";

export type ClickOutsideEvent = "mousedown" | "click" | "pointerdown";

export interface UseClickOutsideOptions {
  /** 触发「外部」判定的指针事件，默认 mousedown（按下即关，体验更跟手） */
  event?: ClickOutsideEvent;
  /** 是否同时响应 Escape 键关闭，默认 true */
  closeOnEscape?: boolean;
}

export function useClickOutside(
  target: Ref<HTMLElement | null | undefined>,
  handler: (e: Event) => void,
  options: UseClickOutsideOptions = {}
): void {
  const { event = "mousedown", closeOnEscape = true } = options;

  function onPointer(e: Event) {
    const el = target.value;
    if (!el) return;
    if (el.contains(e.target as Node)) return;
    handler(e);
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "Escape") handler(e);
  }

  onMounted(() => {
    document.addEventListener(event, onPointer);
    if (closeOnEscape) document.addEventListener("keydown", onKey);
  });

  onBeforeUnmount(() => {
    document.removeEventListener(event, onPointer);
    if (closeOnEscape) document.removeEventListener("keydown", onKey);
  });
}
