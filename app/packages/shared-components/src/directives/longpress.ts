import type { Directive } from "vue";

export const vLongpress: Directive<HTMLElement, () => void> = {
  mounted(el, binding) {
    if (typeof binding.value !== "function") return;
    let pressTimer: ReturnType<typeof setTimeout> | null = null;
    let longPressed = false;

    const start = () => {
      longPressed = false;
      if (pressTimer !== null) return;
      pressTimer = setTimeout(() => {
        longPressed = true;
        binding.value();
        pressTimer = null;
      }, 500);
    };

    const cancel = () => {
      if (pressTimer !== null) {
        clearTimeout(pressTimer);
        pressTimer = null;
      }
    };

    const onClick = (e: Event) => {
      if (longPressed) {
        e.preventDefault();
        e.stopPropagation();
        longPressed = false;
      }
    };

    const onContextMenu = (e: Event) => {
      if (longPressed) {
        e.preventDefault();
      }
    };

    el.addEventListener("touchstart", start, { passive: true });
    el.addEventListener("touchend", cancel);
    el.addEventListener("touchmove", cancel);
    el.addEventListener("touchcancel", cancel);
    el.addEventListener("mousedown", start);
    el.addEventListener("mouseup", cancel);
    el.addEventListener("mouseleave", cancel);
    el.addEventListener("click", onClick, true);
    el.addEventListener("contextmenu", onContextMenu);

    (el as any)._longpress_cleanup = () => {
      el.removeEventListener("touchstart", start);
      el.removeEventListener("touchend", cancel);
      el.removeEventListener("touchmove", cancel);
      el.removeEventListener("touchcancel", cancel);
      el.removeEventListener("mousedown", start);
      el.removeEventListener("mouseup", cancel);
      el.removeEventListener("mouseleave", cancel);
      el.removeEventListener("click", onClick, true);
      el.removeEventListener("contextmenu", onContextMenu);
    };
  },
  unmounted(el) {
    (el as any)._longpress_cleanup?.();
  },
};
