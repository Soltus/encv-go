import { HighRefreshRate } from "@ajuarezso/capacitor-high-refresh-rate";
import { Capacitor } from "@capacitor/core";

let highRefreshInitialized = false;

/**
 * Safely enable high refresh rate (90–120Hz) via the
 * `@ajuarezso/capacitor-high-refresh-rate` plugin.
 *
 * Uses adaptive mode: 120Hz active / 60Hz idle with a 1.5s
 * idle timeout, driven by global touch/scroll/pointer activity.
 *
 * Call once at app boot. Idempotent (no-op on subsequent calls
 * and on non-native platforms).
 */
export async function initHighRefreshRate() {
  if (!Capacitor.isNativePlatform()) return;
  if (highRefreshInitialized) return;
  highRefreshInitialized = true;

  try {
    console.error("[SAT-DBG][HighRefresh] init start");

    const info = await HighRefreshRate.enable();
    console.error("[SAT-DBG][HighRefresh] enabled, info:", JSON.stringify(info));

    await HighRefreshRate.setAdaptiveMode({
      enabled: true,
      activeHz: 120,
      idleHz: 60,
      idleMs: 1500,
    });
    console.error("[SAT-DBG][HighRefresh] adaptive mode set (120/60, 1500ms)");

    // Pipe global activity events to the plugin — throttled to ~5/s.
    let lastPing = 0;
    const onActivity = () => {
      const now = performance.now();
      if (now - lastPing < 200) return;
      lastPing = now;
      HighRefreshRate.notifyActivity();
    };

    ["touchstart", "touchmove", "scroll", "wheel", "pointerdown", "pointermove"].forEach(ev =>
      window.addEventListener(ev, onActivity, { passive: true, capture: true })
    );

    console.error("[SAT-DBG][HighRefresh] activity listeners registered");
  } catch (e: any) {
    console.warn("[HighRefresh] init failed (non-fatal):", e?.message || e);
  }
}

/**
 * Get current refresh rate info for debug overlays / settings UI.
 */
export async function getHighRefreshInfo() {
  if (!Capacitor.isNativePlatform()) return null;
  try {
    return await HighRefreshRate.getInfo();
  } catch {
    return null;
  }
}
