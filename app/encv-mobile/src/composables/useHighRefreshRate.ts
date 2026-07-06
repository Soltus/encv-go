import { Capacitor } from "@capacitor/core";

let highRefreshInitialized = false;
let HighRefreshRate: any = null;

async function loadHighRefreshRate() {
  if (HighRefreshRate) return HighRefreshRate;
  try {
    const mod = await import(/* @vite-ignore */ "@ajuarezso/capacitor-high-refresh-rate");
    HighRefreshRate = mod.HighRefreshRate;
    return HighRefreshRate;
  } catch {
    return null;
  }
}

export async function initHighRefreshRate() {
  if (!Capacitor.isNativePlatform()) return;
  if (highRefreshInitialized) return;
  highRefreshInitialized = true;

  try {
    const HRR = await loadHighRefreshRate();
    if (!HRR) return;

    console.error("[SAT-DBG][HighRefresh] init start");

    const info = await HRR.enable();
    console.error("[SAT-DBG][HighRefresh] enabled, info:", JSON.stringify(info));

    await HRR.setAdaptiveMode({
      enabled: true,
      activeHz: 120,
      idleHz: 60,
      idleMs: 1500,
    });
    console.error("[SAT-DBG][HighRefresh] adaptive mode set (120/60, 1500ms)");

    let lastPing = 0;
    const onActivity = () => {
      const now = performance.now();
      if (now - lastPing < 200) return;
      lastPing = now;
      HRR.notifyActivity();
    };

    ["touchstart", "touchmove", "scroll", "wheel", "pointerdown", "pointermove"].forEach(ev =>
      window.addEventListener(ev, onActivity, { passive: true, capture: true })
    );

    console.error("[SAT-DBG][HighRefresh] activity listeners registered");
  } catch (e: any) {
    console.warn("[HighRefresh] init failed (non-fatal):", e?.message || e);
  }
}

export async function getHighRefreshInfo() {
  if (!Capacitor.isNativePlatform()) return null;
  try {
    const HRR = await loadHighRefreshRate();
    if (!HRR) return null;
    return await HRR.getInfo();
  } catch {
    return null;
  }
}
