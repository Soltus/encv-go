import { registerPlugin } from "@capacitor/core";

export interface SimVersePlugin {
  openWorld(options: { worldId: string; worldName: string }): Promise<void>;
  closeWorld(): Promise<void>;
  startHeartbeat(): Promise<void>;
  stopHeartbeat(): Promise<void>;
  setWorldRunning(options: { running: boolean }): Promise<void>;
  addShortcut(): Promise<void>;
  isShortcutSupported(): Promise<{ supported: boolean }>;
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
}

const SimVerse = registerPlugin<SimVersePlugin>("SimVerse", {
  web: () => new SimVerseWeb(),
});

export async function openWorld(worldId: string = "default", worldName: string = "Default"): Promise<{ success: boolean; error?: string }> {
  try {
    await SimVerse.openWorld({ worldId, worldName });
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] openWorld failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
    return { success: false, error: e instanceof Error ? e.message : String(e) };
  }
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
