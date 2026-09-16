import { registerPlugin } from "@capacitor/core";

export interface SimVersePlugin {
  openWorld(options: { worldId: string; worldName: string; themeCss?: string }): Promise<void>;
  closeWorld(): Promise<void>;
  startHeartbeat(): Promise<void>;
  stopHeartbeat(): Promise<void>;
  setWorldRunning(options: { running: boolean }): Promise<void>;
  addShortcut(): Promise<void>;
  isShortcutSupported(): Promise<{ supported: boolean }>;
  debugSimVerseFlow(): Promise<{ debugLog: string }>;
}

class SimVerseWeb implements SimVersePlugin {
  async openWorld(_options: { worldId: string; worldName: string }): Promise<void> {
    console.warn("[SimVerse] openWorld: not available in web mode");
  }
  async closeWorld(): Promise<void> {
    console.warn("[SimVerse] closeWorld: not available in web mode");
  }
  async startHeartbeat(): Promise<void> {
    console.warn("[SimVerse] startHeartbeat: not available in web mode");
  }
  async stopHeartbeat(): Promise<void> {
    console.warn("[SimVerse] stopHeartbeat: not available in web mode");
  }
  async setWorldRunning(_options: { running: boolean }): Promise<void> {
    console.warn("[SimVerse] setWorldRunning: not available in web mode");
  }
  async addShortcut(): Promise<void> {
    console.warn("[SimVerse] addShortcut: not available in web mode");
  }
  async isShortcutSupported(): Promise<{ supported: boolean }> {
    return { supported: false };
  }
  async debugSimVerseFlow(): Promise<{ debugLog: string }> {
    return { debugLog: "[SimVerse] debugSimVerseFlow: not available in web mode" };
  }
}

const SimVerse = registerPlugin<SimVersePlugin>("SimVerse", {
  web: () => new SimVerseWeb(),
});

export async function openWorld(worldId: string = "default", worldName: string = "Default", themeCss?: string): Promise<{ success: boolean; error?: string }> {
  try {
    // 主应用外观设置桥接：未显式传入时，抓取当前主题的已解析 CSS 变量块一并交给插件，
    // 使主应用外观设置在独立插件 WebView 内生效。
    const css = themeCss ?? captureThemeCssVars();
    await SimVerse.openWorld({ worldId, worldName, themeCss: css });
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] openWorld failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
    return { success: false, error: e instanceof Error ? e.message : String(e) };
  }
}

/**
 * 抓取主应用当前主题的【已解析 CSS 变量】为 `:root{...}` 块。
 * 插件是独立 WebView，不共享主应用 window / CSS 变量；把当前主题变量作为
 * `window.__ENCV_THEME__.css` 注入插件（见 SimVerseWebViewClient.injectTheme），
 * 即可让主应用外观设置（主题/取色）在插件内生效。无 document 时返回空串（不报错）。
 */
function captureThemeCssVars(): string {
  if (typeof document === "undefined") return "";
  const cs = getComputedStyle(document.documentElement);
  const lines: string[] = [];
  for (const name of cs) {
    if (name.startsWith("--")) {
      const val = cs.getPropertyValue(name);
      if (val) lines.push(`${name}: ${val.trim()};`);
    }
  }
  return `:root{${lines.join("")}}`;
}

export async function closeWorld(): Promise<{ success: boolean }> {
  try {
    await SimVerse.closeWorld();
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] closeWorld failed:", e);
    return { success: false };
  }
}

export async function startSimverseHeartbeat(): Promise<{ success: boolean }> {
  try {
    await SimVerse.startHeartbeat();
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] startHeartbeat failed:", e);
    return { success: false };
  }
}

export async function stopSimverseHeartbeat(): Promise<{ success: boolean }> {
  try {
    await SimVerse.stopHeartbeat();
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] stopHeartbeat failed:", e);
    return { success: false };
  }
}

export async function setSimverseWorldRunning(running: boolean): Promise<{ success: boolean }> {
  try {
    await SimVerse.setWorldRunning({ running });
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] setWorldRunning failed:", e);
    return { success: false };
  }
}

export async function addWorldShortcut(): Promise<{ success: boolean }> {
  try {
    await SimVerse.addShortcut();
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] addShortcut failed:", e);
    return { success: false };
  }
}

export async function isWorldShortcutSupported(): Promise<boolean> {
  try {
    const result = await SimVerse.isShortcutSupported();
    return result.supported;
  } catch (e) {
    console.error("[SimVerse] isShortcutSupported failed:", e);
    return false;
  }
}

export async function debugSimVerseFlow(): Promise<{ debugLog: string }> {
  try {
    const result = await SimVerse.debugSimVerseFlow();
    return result;
  } catch (e) {
    console.error("[SimVerse] debugSimVerseFlow failed:", e);
    return { debugLog: e instanceof Error ? `${e.name}: ${e.message}` : String(e) };
  }
}
