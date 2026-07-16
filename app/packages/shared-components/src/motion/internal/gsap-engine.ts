/**
 * gsap 实现：MotionEngine 的唯一具体实现，也是整个仓库「唯一 import gsap」的文件。
 *
 * 更换动画库时：
 *   1. 复制本文件，改用目标库实现同一 MotionEngine 接口（见 ./types.ts）；
 *   2. 让 ./index.ts 导出新的实现（例如 `export { motion } from "./anime-engine"`）。
 * composable 与所有下游应用 / 插件无需任何改动。
 */
import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { Flip } from "gsap/Flip";
import type {
  MotionEngine,
  MotionVars,
  MotionTarget,
  MotionTween,
  MotionContext,
  ScrollTriggerHandle,
  FlipState,
  ScrollTriggerConfig,
} from "./types";

/** 语义缓动 → 引擎原生缓动。新增语义缓动只需在此登记一处。 */
const EASE_MAP: Record<string, string> = {
  out: "power2.out",
  inOut: "power1.inOut",
  back: "back.out(1.6)",
  linear: "none",
  follow: "power3",
  float: "sine.inOut",
};

function withEase(vars?: MotionVars): MotionVars | undefined {
  if (!vars || typeof vars.ease !== "string") return vars;
  const mapped = EASE_MAP[vars.ease];
  return mapped ? { ...vars, ease: mapped } : vars;
}

type GsapTarget = Parameters<typeof gsap.set>[0];
function gt(t: MotionTarget | MotionTarget[]): GsapTarget {
  return (Array.isArray(t) ? t : [t]) as unknown as GsapTarget;
}
function gv(v?: MotionVars): gsap.TweenVars {
  return (v ?? {}) as unknown as gsap.TweenVars;
}

class GsapEngine implements MotionEngine {
  private installed = false;

  registerPlugins(): void {
    if (this.installed) return;
    gsap.registerPlugin(ScrollTrigger, Flip);
    this.installed = true;
  }

  set(target: MotionTarget | MotionTarget[], vars: MotionVars): void {
    gsap.set(gt(target), gv(vars));
  }

  to(target: MotionTarget | MotionTarget[], vars: MotionVars): MotionTween {
    const t = gsap.to(gt(target), gv(withEase(vars)));
    return { kill: () => t.kill() };
  }

  from(target: MotionTarget | MotionTarget[], vars: MotionVars): MotionTween {
    const t = gsap.from(gt(target), gv(withEase(vars)));
    return { kill: () => t.kill() };
  }

  fromTo(target: MotionTarget | MotionTarget[], fromVars: MotionVars, toVars: MotionVars): MotionTween {
    const t = gsap.fromTo(gt(target), gv(fromVars), gv(withEase(toVars)));
    return { kill: () => t.kill() };
  }

  context(fn: () => void, scope: Element): MotionContext {
    const ctx = gsap.context(fn, scope);
    return { revert: () => ctx.revert() };
  }

  quickTo(target: MotionTarget, prop: string, vars: MotionVars): (value: number) => void {
    return gsap.quickTo(target as Parameters<typeof gsap.quickTo>[0], prop, gv(vars)) as (value: number) => void;
  }

  delayedCall(delay: number, cb: () => void): MotionTween {
    const t = gsap.delayedCall(delay, cb);
    return { kill: () => t.kill() };
  }

  createScrollTrigger(config: ScrollTriggerConfig): ScrollTriggerHandle {
    const st = ScrollTrigger.create({
      trigger: config.trigger,
      scroller: config.scroller,
      start: config.start,
      end: config.end,
      once: config.once,
      scrub: config.scrub,
      onEnter: config.onEnter,
      onUpdate: config.onUpdate ? self => config.onUpdate!({ progress: self.progress, kill: () => self.kill() }) : undefined,
    });
    return {
      kill: () => st.kill(),
      get progress() {
        return st.progress;
      },
    };
  }

  refreshScrollTriggers(): void {
    ScrollTrigger.refresh();
  }

  flipGetState(target: MotionTarget): FlipState {
    return Flip.getState(target as Parameters<typeof Flip.getState>[0]);
  }

  flipFrom(state: FlipState, vars: MotionVars): void {
    Flip.from(state as Parameters<typeof Flip.from>[0], gv(withEase(vars)) as unknown as Parameters<typeof Flip.from>[1]);
  }
}

export const motion: MotionEngine = new GsapEngine();
