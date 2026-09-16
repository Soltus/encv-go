/**
 * GSAP-powered vue-router route transition composable.
 *
 * Exposes `onEnter` / `onLeave` hooks suitable for `<Transition>` JavaScript
 * hooks. Enter fades + slides the element up; leave fades + slides it up and
 * away.
 *
 * SSR-safe: hooks call `done()` immediately when `window` is undefined so the
 * transition resolves synchronously on the server.
 */
import { gsap } from "./useGsap";

/** Options for {@link useRouteTransition}. */
export interface RouteTransitionOptions {
  /** Tween duration in seconds. */
  duration?: number;
  /** GSAP ease name (e.g. `"power2.out"`). */
  ease?: string;
  /** Starting vars for the enter tween (defaults to fade + slide up). */
  enterFrom?: gsap.TweenVars;
  /** Ending vars for the enter tween (defaults to opacity 1, y 0). */
  enterTo?: gsap.TweenVars;
  /** Ending vars for the leave tween (defaults to fade + slide up). */
  leaveTo?: gsap.TweenVars;
}

/** Sensible defaults: 0.35s power2.out, fade + 24px slide. */
const DEFAULT_OPTS: Required<RouteTransitionOptions> = {
  duration: 0.35,
  ease: "power2.out",
  enterFrom: { opacity: 0, y: 24 },
  enterTo: { opacity: 1, y: 0 },
  leaveTo: { opacity: 0, y: -24 },
};

/**
 * Composable returning `<Transition>` JavaScript hooks wired to GSAP.
 *
 * Usage:
 * ```vue
 * <Transition :css="false" @enter="onEnter" @leave="onLeave">
 *   <RouterView />
 * </Transition>
 * ```
 *
 * @param options Optional overrides merged onto {@link DEFAULT_OPTS}.
 * @returns `{ onEnter, onLeave }` hooks.
 */
export function useRouteTransition(options?: RouteTransitionOptions) {
  const opts = { ...DEFAULT_OPTS, ...options };

  /**
   * `<Transition>` `@enter` hook. Fades + slides the element into place using a
   * gsap timeline. Calls `done()` immediately on the server.
   */
  function onEnter(el: Element, done: () => void): void {
    if (typeof window === "undefined") {
      done();
      return;
    }
    gsap
      .timeline({ onComplete: done })
      .fromTo(el, opts.enterFrom, { ...opts.enterTo, duration: opts.duration, ease: opts.ease });
  }

  /**
   * `<Transition>` `@leave` hook. Fades + slides the element out using a gsap
   * timeline. Calls `done()` immediately on the server.
   */
  function onLeave(el: Element, done: () => void): void {
    if (typeof window === "undefined") {
      done();
      return;
    }
    gsap
      .timeline({ onComplete: done })
      .to(el, { ...opts.leaveTo, duration: opts.duration, ease: opts.ease });
  }

  return { onEnter, onLeave };
}
