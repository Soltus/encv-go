/**
 * 内容进入与滚动揭示（§2.5.2）。
 *  - useScrollReveal：元素进入视口时从下方淡入，可选 children stagger。
 *  - useScrollParallax：sticky 头图 / 分节标题的慢速视差位移。
 *
 * 所有原语经 internal/ 引擎抽象，调用方不接触具体动画库（ACL）。
 */
import { nextTick, onMounted, onUnmounted, watch, type Ref } from "vue";
import { motion, type MotionTarget, type ScrollTriggerHandle } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE, getStagger } from "./tokens";

export interface ScrollRevealOptions {
  /** 子元素逐个错峰入场（容器内的直接子元素） */
  stagger?: boolean;
  /**
   * 异步列表就绪闸门：为 true 时才落初始隐藏态并建触发器。
   * 不传则按挂载时 DOM 立即处理（适用于静态列表）。
   * 解决「数据异步加载、挂载时子元素还不存在」导致不入场的问题。
   */
  ready?: Ref<boolean>;
}

export function useScrollReveal(el: Ref<HTMLElement | null>, opts: ScrollRevealOptions = {}): void {
  let observer: { disconnect(): void } | undefined;
  let disposed = false;
  const revealed = new WeakSet<object>();

  function animateIn(targets: MotionTarget | MotionTarget[], p: ReturnType<typeof getMotionProfile>): void {
    const arr = (Array.isArray(targets) ? targets : [targets]).filter(t => !revealed.has(t));
    if (arr.length === 0) return;
    arr.forEach(t => revealed.add(t));
    motion.to(arr, {
      y: 0,
      opacity: 1,
      duration: DUR.base,
      ease: EASE.out,
      stagger: opts.stagger ? getStagger() * p.intensity : undefined,
    });
  }

  function setup(): void {
    const node = el.value;
    if (!node) return;
    const p = getMotionProfile();
    if (!p.enabled) return; // 关动效：元素保持自然态（可见），不播放入场。
    const targets: MotionTarget | MotionTarget[] = opts.stagger ? Array.from(node.children) : node;
    if (Array.isArray(targets) && targets.length === 0) return; // 无子元素：等 ready 闸门触发
    // 立即落初始隐藏态，避免「滚动到才闪一下」的跳变。
    motion.set(targets, { y: 16 * p.intensity, opacity: 0 });
    // 用 IntersectionObserver 观察视口相交：与具体滚动容器无关。
    // Ionic 的 ion-content 在内部 .inner-scroll 滚动（非 window），gsap ScrollTrigger 默认
    // 以 window 为 scroller，永远收不到滚动事件 → onEnter 永不触发 → 子元素永久停在
    // opacity:0 = 整页空白但可点击。IO 在「挂载即相交」的元素上立即回调，揭示可靠发生。
    if (typeof IntersectionObserver === "undefined") {
      // 兜底：无 IO 环境直接落可见态，内容【绝不】永久隐藏（防变异形）。
      animateIn(targets, p);
      return;
    }
    const io = new IntersectionObserver(
      entries => {
        if (disposed) return;
        if (entries.some(e => e.isIntersecting)) {
          animateIn(targets, p);
          io.disconnect(); // once 语义：揭示后断开，避免重复入场
        }
      },
      // rootMargin 底部收缩 10% 视口 ≈ gsap start:"top 90%"
      { root: null, rootMargin: "0px 0px -10% 0px", threshold: 0 }
    );
    io.observe(node);
    observer = io;
  }

  onMounted(() => {
    const p = getMotionProfile();
    if (!p.enabled) return;
    if (opts.ready && !opts.ready.value) {
      watch(
        opts.ready,
        async v => {
          if (!v) return;
          await nextTick(); // 等异步列表渲染出子元素
          setup();
        },
        { once: true }
      );
    } else {
      setup();
    }
  });
  onUnmounted(() => {
    disposed = true;
    observer?.disconnect();
  });
}

export function useScrollParallax(el: Ref<HTMLElement | null>, amount = 20): void {
  let trigger: ScrollTriggerHandle | undefined;
  onMounted(() => {
    const node = el.value;
    if (!node) return;
    const p = getMotionProfile();
    if (!p.enabled) return;
    trigger = motion.createScrollTrigger({
      trigger: node,
      start: "top bottom",
      end: "bottom top",
      scrub: true,
      onUpdate: self => {
        motion.set(node, { yPercent: (self.progress - 0.5) * amount * p.intensity });
      },
    });
  });
  onUnmounted(() => trigger?.kill());
}
