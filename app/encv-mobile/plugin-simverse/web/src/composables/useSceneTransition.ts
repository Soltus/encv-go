/**
 * GSAP Flip-powered scene transition composable for HUD scenes.
 *
 * Wraps `Flip.getState` / `Flip.from` / `Flip.to` behind a small stateful API
 * so callers can record a snapshot, mutate the DOM (e.g. swap a `data-scene`
 * attribute), then animate from the snapshot to the new state.
 *
 * SSR-safe: every method is a no-op when `window` is undefined.
 */
import { Flip, gsap } from "./useGsap";

/** Options for {@link useSceneTransition} methods. */
export interface SceneTransitionOptions {
  /** Tween duration in seconds. */
  duration?: number;
  /** GSAP ease name (e.g. `"power3.inOut"`). */
  ease?: string;
  /** Position elements absolutely during the flip (avoids reflow jumps). */
  absolute?: boolean;
  /** Apply transforms to nested children as well. */
  nested?: boolean;
  /** Include scale in the flip transform. */
  scale?: boolean;
  /** Callback fired when the transition completes. */
  onComplete?: () => void;
}

/** Sensible defaults matching the spec: 0.45s power3.inOut, absolute, nested. */
const DEFAULT_OPTS: Required<Omit<SceneTransitionOptions, "onComplete">> = {
  duration: 0.45,
  ease: "power3.inOut",
  absolute: true,
  nested: true,
  scale: false,
};

/**
 * Composable for managing Flip-based HUD scene transitions.
 *
 * Typical usage:
 * ```ts
 * const { transitionToScene } = useSceneTransition();
 * await transitionToScene(containerEl, () => {
 *   containerEl.dataset.scene = "battle";
 * });
 * ```
 *
 * @returns `{ recordState, playTransition, reverseTransition, transitionToScene }`.
 */
export function useSceneTransition() {
  let lastState: Flip.FlipState | null = null;

  /**
   * Capture a {@link Flip.FlipState} snapshot for `el`.
   *
   * No-op on the server or when `el` is null.
   */
  function recordState(el: Element | null): void {
    if (!el || typeof window === "undefined") return;
    lastState = Flip.getState(el);
  }

  /**
   * Play a forward Flip transition: animate `targets` FROM the last recorded
   * state TO their current DOM state via `Flip.from`. Clears the recorded
   * state afterwards so the next transition requires a fresh `recordState`.
   */
  function playTransition(targets: gsap.TweenTarget, options?: SceneTransitionOptions): void {
    if (typeof window === "undefined" || !lastState) return;
    const opts = { ...DEFAULT_OPTS, ...options };
    Flip.from(lastState, {
      targets: targets as gsap.DOMTarget,
      duration: opts.duration,
      ease: opts.ease,
      absolute: opts.absolute,
      nested: opts.nested,
      scale: opts.scale,
      onComplete: opts.onComplete,
    });
    lastState = null;
  }

  /**
   * Play a reverse Flip transition: animate `targets` from their current DOM
   * state back TO the last recorded state via `Flip.to`.
   */
  function reverseTransition(targets: gsap.TweenTarget, options?: SceneTransitionOptions): void {
    if (typeof window === "undefined" || !lastState) return;
    const opts = { ...DEFAULT_OPTS, ...options };
    Flip.to(lastState, {
      targets: targets as gsap.DOMTarget,
      duration: opts.duration,
      ease: opts.ease,
      absolute: opts.absolute,
      nested: opts.nested,
      scale: opts.scale,
      onComplete: opts.onComplete,
    });
  }

  /**
   * High-level helper: record state, apply a (possibly async) DOM mutation,
   * then play the forward transition. Ideal for scene switches that change a
   * `data-scene` attribute or swap child nodes.
   */
  async function transitionToScene(
    containerEl: HTMLElement | null,
    applyChange: () => void | Promise<void>,
    options?: SceneTransitionOptions,
  ): Promise<void> {
    if (!containerEl || typeof window === "undefined") return;
    recordState(containerEl);
    await applyChange();
    playTransition(containerEl, options);
  }

  return { recordState, playTransition, reverseTransition, transitionToScene };
}
