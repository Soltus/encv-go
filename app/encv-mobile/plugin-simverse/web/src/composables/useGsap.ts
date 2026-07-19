/**
 * Unified GSAP entry composable.
 *
 * Registers GSAP browser-only plugins (`ScrollTrigger`, `Flip`,
 * `MotionPathPlugin`) exactly once per module load, with an SSR-safe guard so
 * plugin registration is skipped when running on the server.
 *
 * Consumers should import {@link useGsap} (or the named plugin re-exports)
 * from this module instead of touching `gsap` directly, so that plugin
 * registration stays idempotent and centrally controlled.
 */
import { gsap } from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { Flip } from "gsap/Flip";
import { MotionPathPlugin } from "gsap/MotionPathPlugin";

/** Module-level flag ensuring plugin registration is idempotent. */
let pluginsRegistered = false;

/**
 * Register GSAP browser-only plugins exactly once.
 *
 * Safe to call from any context: SSR environments are skipped and repeat
 * calls are no-ops once registration has run.
 */
function registerPlugins(): void {
  if (pluginsRegistered) return;
  if (typeof window === "undefined") return;
  gsap.registerPlugin(ScrollTrigger, Flip, MotionPathPlugin);
  pluginsRegistered = true;
}

// Register on module load (no-op on the server).
registerPlugins();

/**
 * Readonly bundle of GSAP core + registered plugins returned by
 * {@link useGsap}.
 */
export interface GsapBundle {
  /** GSAP core namespace. */
  readonly gsap: typeof gsap;
  /** ScrollTrigger plugin (registered). */
  readonly ScrollTrigger: typeof ScrollTrigger;
  /** Flip plugin (registered). */
  readonly Flip: typeof Flip;
  /** MotionPathPlugin (registered). */
  readonly MotionPathPlugin: typeof MotionPathPlugin;
}

/** Frozen, shareable bundle of GSAP references. */
const bundle: GsapBundle = Object.freeze({
  gsap,
  ScrollTrigger,
  Flip,
  MotionPathPlugin,
});

export { gsap, ScrollTrigger, Flip, MotionPathPlugin };

/**
 * Composable returning the shared GSAP bundle.
 *
 * Ensures plugins are registered (idempotent, SSR-safe) before handing back
 * the cached references. Always returns the same frozen {@link GsapBundle}
 * instance, so callers can safely destructure without re-registering.
 *
 * @returns Frozen `{ gsap, ScrollTrigger, Flip, MotionPathPlugin }` bundle.
 */
export function useGsap(): GsapBundle {
  registerPlugins();
  return bundle;
}
