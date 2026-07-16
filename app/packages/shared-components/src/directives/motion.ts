/**
 * 动效指令层（应用层 · §2.5 全集的自助接入）。
 *
 * 为什么单独一层、不复用 composables：
 *   composables（usePageTransition / useScrollReveal / ...）内部用 onMounted/onUnmounted，
 *   要求在组件 setup 上下文里调用；而 Vue 指令的 mounted(el) 没有活跃组件实例，
 *   直接调 composable 的 onMounted 会失效。因此本文件把同一套动效逻辑用「指令生命周期」
 *   （mounted / unmounted / updated）重新驱动，引擎仍走 ./motion/internal（ACL 唯一出口），
 *   闸门仍走 ./motion/guard —— 换 gsap+daisyui 技术栈时，本层与 composables 一起零改动。
 *
 * 用法（全局注册后任意组件一行接入）：
 *   <ion-page v-page-transition>
 *   <section v-reveal="{ stagger: true }">
 *   <button v-ripple v-press>
 *   <span v-count-up="12345">
 */
import type { Directive, App } from "vue";
import { motion } from "../motion/internal";
import { getMotionProfile } from "../motion/guard";
import { DUR, EASE, getStagger } from "../motion/tokens";

/** 指令级 cleanup 收集（unmounted 时统一执行）。挂 WeakMap 避免污染元素类型。 */
const cleanups = new WeakMap<HTMLElement, Array<() => void>>();
function track(el: HTMLElement, cleanup: () => void): void {
  const list = cleanups.get(el) ?? [];
  list.push(cleanup);
  cleanups.set(el, list);
}
function runCleanups(el: HTMLElement): void {
  const list = cleanups.get(el);
  if (list) for (const fn of list) fn();
  cleanups.delete(el);
}

/* ---------------- v-reveal：滚动揭示（可选子元素 stagger） ---------------- */
export const vReveal: Directive<HTMLElement, { stagger?: boolean } | undefined> = {
  mounted(el, binding) {
    const p = getMotionProfile();
    if (!p.enabled) return; // 关动效：保持自然可见态
    const stagger = binding.value?.stagger ?? false;
    const targets: HTMLElement | HTMLElement[] = stagger ? (Array.from(el.children) as HTMLElement[]) : el;
    if (Array.isArray(targets) && targets.length === 0) return;
    motion.set(targets, { y: 16 * p.intensity, opacity: 0 });
    // IntersectionObserver：与滚动容器无关，Ionic 内滚下也能可靠揭示（详见 reveal.ts 说明）。
    if (typeof IntersectionObserver === "undefined") {
      motion.to(targets, {
        y: 0,
        opacity: 1,
        duration: DUR.base,
        ease: EASE.out,
        stagger: stagger ? getStagger() * p.intensity : undefined,
      });
      return;
    }
    const io = new IntersectionObserver(
      entries => {
        if (entries.some(e => e.isIntersecting)) {
          motion.to(targets, {
            y: 0,
            opacity: 1,
            duration: DUR.base,
            ease: EASE.out,
            stagger: stagger ? getStagger() * p.intensity : undefined,
          });
          io.disconnect();
        }
      },
      { root: null, rootMargin: "0px 0px -10% 0px", threshold: 0 }
    );
    io.observe(el);
    track(el, () => io.disconnect());
  },
  unmounted(el) {
    runCleanups(el);
  },
};

/* ---------------- v-page-transition：页面进入淡入上移 ---------------- */
export const vPageTransition: Directive<HTMLElement> = {
  mounted(el) {
    const p = getMotionProfile();
    if (!p.enabled) {
      motion.set(el, { clearProps: "all" });
      return;
    }
    const ctx = motion.context(() => {
      motion.from(el, {
        y: 12 * p.intensity,
        opacity: 0,
        duration: DUR.base * p.intensity,
        ease: EASE.out,
      });
    }, el);
    track(el, () => ctx.revert());
  },
  unmounted(el) {
    runCleanups(el);
  },
};

/* ---------------- v-ripple：卡片点击波纹 ---------------- */
export const vRipple: Directive<HTMLElement> = {
  mounted(el) {
    // 宿主需相对定位 + 裁剪溢出，ripple 圆形才被正确限制在卡片内。
    if (getComputedStyle(el).position === "static") el.style.position = "relative";
    if (getComputedStyle(el).overflow === "visible") el.style.overflow = "hidden";
    const onDown = (e: PointerEvent) => {
      if (!getMotionProfile().enabled) return;
      const rect = el.getBoundingClientRect();
      const size = Math.max(rect.width, rect.height);
      const ripple = document.createElement("span");
      ripple.className = "encv-ripple";
      motion.set(ripple, {
        position: "absolute",
        left: e.clientX - rect.left - size / 2,
        top: e.clientY - rect.top - size / 2,
        width: size,
        height: size,
        borderRadius: "9999px",
        background: "currentColor",
        opacity: 0.28,
        scale: 0,
        pointerEvents: "none",
      });
      el.appendChild(ripple);
      motion.to(ripple, {
        scale: 1,
        opacity: 0,
        duration: 0.5,
        ease: EASE.out,
        onComplete: () => ripple.remove(),
      });
    };
    el.addEventListener("pointerdown", onDown);
    track(el, () => el.removeEventListener("pointerdown", onDown));
  },
  unmounted(el) {
    runCleanups(el);
  },
};

/* ---------------- v-press：按钮按压弹性回弹 ---------------- */
export const vPress: Directive<HTMLElement> = {
  mounted(el) {
    const onDown = () => {
      if (getMotionProfile().enabled) motion.to(el, { scale: 0.96, duration: 0.08, ease: EASE.out });
    };
    const onUp = () => {
      if (getMotionProfile().enabled) motion.to(el, { scale: 1, duration: 0.24, ease: EASE.back });
    };
    el.addEventListener("pointerdown", onDown);
    el.addEventListener("pointerup", onUp);
    el.addEventListener("pointerleave", onUp);
    track(el, () => {
      el.removeEventListener("pointerdown", onDown);
      el.removeEventListener("pointerup", onUp);
      el.removeEventListener("pointerleave", onUp);
    });
  },
  unmounted(el) {
    runCleanups(el);
  },
};

/* ---------------- v-hover：悬浮抬升 ---------------- */
export const vHover: Directive<HTMLElement> = {
  mounted(el) {
    const onEnter = () => {
      if (getMotionProfile().enabled) motion.to(el, { y: -4, duration: DUR.fast, ease: EASE.out });
    };
    const onLeave = () => {
      if (getMotionProfile().enabled) motion.to(el, { y: 0, duration: DUR.fast, ease: EASE.out });
    };
    el.addEventListener("pointerenter", onEnter);
    el.addEventListener("pointerleave", onLeave);
    track(el, () => {
      el.removeEventListener("pointerenter", onEnter);
      el.removeEventListener("pointerleave", onLeave);
    });
  },
  unmounted(el) {
    runCleanups(el);
  },
};

/* ---------------- v-magnetic：磁性按钮（鼠标跟手微移） ---------------- */
export const vMagnetic: Directive<HTMLElement, number | undefined> = {
  mounted(el, binding) {
    const strength = binding.value ?? 0.3;
    const xTo = motion.quickTo(el, "x", { duration: DUR.fast, ease: EASE.out });
    const yTo = motion.quickTo(el, "y", { duration: DUR.fast, ease: EASE.out });
    const onMove = (e: PointerEvent) => {
      if (!getMotionProfile().enabled) return;
      const rect = el.getBoundingClientRect();
      const mx = e.clientX - (rect.left + rect.width / 2);
      const my = e.clientY - (rect.top + rect.height / 2);
      xTo(mx * strength);
      yTo(my * strength);
    };
    const onLeave = () => {
      xTo(0);
      yTo(0);
    };
    el.addEventListener("pointermove", onMove);
    el.addEventListener("pointerleave", onLeave);
    track(el, () => {
      el.removeEventListener("pointermove", onMove);
      el.removeEventListener("pointerleave", onLeave);
    });
  },
  unmounted(el) {
    runCleanups(el);
  },
};

/* ---------------- v-count-up：数字滚动 ---------------- */
export const vCountUp: Directive<HTMLElement, number | { to: number; dur?: number }> = {
  mounted(el, binding) {
    runCountUp(el, binding.value);
  },
  updated(el, binding) {
    if (binding.value === binding.oldValue) return;
    runCountUp(el, binding.value);
  },
};
function runCountUp(el: HTMLElement, val: number | { to: number; dur?: number } | undefined): void {
  if (val == null) return;
  const to = typeof val === "number" ? val : val.to;
  const dur = typeof val === "number" ? 0.8 : (val.dur ?? 0.8);
  const p = getMotionProfile();
  if (!p.enabled) {
    el.textContent = to.toLocaleString();
    return;
  }
  const obj = { v: 0 };
  motion.to(obj, {
    v: to,
    duration: dur * p.intensity,
    ease: EASE.inOut,
    onUpdate: () => (el.textContent = Math.round(obj.v).toLocaleString()),
  });
}

/** 批量注册（main.ts 一行接入，全应用自助可用） */
export function installMotionDirectives(app: App): void {
  app.directive("reveal", vReveal);
  app.directive("page-transition", vPageTransition);
  app.directive("ripple", vRipple);
  app.directive("press", vPress);
  app.directive("hover", vHover);
  app.directive("magnetic", vMagnetic);
  app.directive("count-up", vCountUp);
}

export const motionDirectives = {
  reveal: vReveal,
  "page-transition": vPageTransition,
  ripple: vRipple,
  press: vPress,
  hover: vHover,
  magnetic: vMagnetic,
  "count-up": vCountUp,
};
