/**
 * useDisclosure — 开关状态的最小抽象。
 *
 * 覆盖所有「开/关/切换」意图（dropdown、popover、sheet、drawer 等），
 * 避免每个组件手搓 `const isOpen = ref(false)` + `open/close/toggle` 样板。
 *
 * 与 useClickOutside 配合可完整替代「手搓 dropdown」的样板
 * （isOpen + toggle + 外部点击关闭 + Escape 关闭 + onMounted/onBeforeUnmount 清理）。
 */
import { ref, type Ref } from "vue";

export interface UseDisclosureReturn {
  /** 当前是否打开 */
  isOpen: Ref<boolean>;
  open: () => void;
  close: () => void;
  toggle: () => void;
}

export function useDisclosure(initial = false): UseDisclosureReturn {
  const isOpen = ref(initial);
  function open() {
    isOpen.value = true;
  }
  function close() {
    isOpen.value = false;
  }
  function toggle() {
    isOpen.value = !isOpen.value;
  }
  return { isOpen, open, close, toggle };
}
