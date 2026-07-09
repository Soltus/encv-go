export interface SimVersePlugin {
  openWorld(options: { worldId: string; worldName: string }): Promise<void>;
  closeWorld(): Promise<void>;
  startHeartbeat(): Promise<void>;
  stopHeartbeat(): Promise<void>;
  setWorldRunning(options: { running: boolean }): Promise<void>;
  addShortcut(): Promise<void>;
  isShortcutSupported(): Promise<{ supported: boolean }>;
  lockOrientation(options: { orientation: "landscape-primary" | "portrait-primary" }): Promise<void>;
  unlockOrientation(): Promise<void>;
  showDiagnostic(): Promise<void>;
  hideSystemUI(): Promise<void>;
  showSystemUI(): Promise<void>;
}

const nativeBridge = (window as any).SimVerseNative as SimVersePlugin | null;
const isJSInterfaceMode = !!(window as any).SimVerseNative && typeof (window as any).SimVerseNative.lockOrientation === "function";

const webImpl: SimVersePlugin = {
  async openWorld(_options: { worldId: string; worldName: string }) {
    console.warn("[SimVerse] openWorld: not available in web mode");
  },
  async closeWorld() {
    console.warn("[SimVerse] closeWorld: not available in web mode");
  },
  async startHeartbeat() {
    console.warn("[SimVerse] startHeartbeat: not available in web mode");
  },
  async stopHeartbeat() {
    console.warn("[SimVerse] stopHeartbeat: not available in web mode");
  },
  async setWorldRunning(_options: { running: boolean }) {
    console.warn("[SimVerse] setWorldRunning: not available in web mode");
  },
  async addShortcut() {
    console.warn("[SimVerse] addShortcut: not available in web mode");
  },
  async isShortcutSupported() {
    return { supported: false };
  },
  async lockOrientation(_options: { orientation: "landscape-primary" | "portrait-primary" }) {
    console.warn("[SimVerse] lockOrientation: not available in web mode");
  },
  async unlockOrientation() {
    console.warn("[SimVerse] unlockOrientation: not available in web mode");
  },
  async showDiagnostic() {
    console.warn("[SimVerse] showDiagnostic: not available in web mode");
  },
  async hideSystemUI() {
    console.warn("[SimVerse] hideSystemUI: not available in web mode, trying Fullscreen API");
    try {
      if (document.documentElement.requestFullscreen) {
        await document.documentElement.requestFullscreen();
      }
    } catch (e) {
      console.warn("[SimVerse] requestFullscreen failed:", e);
    }
  },
  async showSystemUI() {
    console.warn("[SimVerse] showSystemUI: not available in web mode, trying exitFullscreen");
    try {
      if (document.exitFullscreen && document.fullscreenElement) {
        await document.exitFullscreen();
      }
    } catch (e) {
      console.warn("[SimVerse] exitFullscreen failed:", e);
    }
  },
};

function callNative<K extends keyof SimVersePlugin>(
  method: K,
  ...args: Parameters<SimVersePlugin[K]>
): ReturnType<SimVersePlugin[K]> {
  if (nativeBridge) {
    try {
      if (isJSInterfaceMode) {
        if (method === "lockOrientation") {
          const options = args[0] as { orientation: string };
          (nativeBridge as any).lockOrientation(options.orientation);
          return Promise.resolve() as any;
        }
        if (method === "unlockOrientation") {
          (nativeBridge as any).unlockOrientation();
          return Promise.resolve() as any;
        }
        if (method === "closeWorld") {
          (nativeBridge as any).closeWorld();
          return Promise.resolve() as any;
        }
        if (method === "showDiagnostic") {
          (nativeBridge as any).showDiagnostic();
          return Promise.resolve() as any;
        }
        if (method === "hideSystemUI") {
          (nativeBridge as any).hideSystemUI();
          return Promise.resolve() as any;
        }
        if (method === "showSystemUI") {
          (nativeBridge as any).showSystemUI();
          return Promise.resolve() as any;
        }
        console.warn(`[SimVerse] Method ${method} not available in JS Interface mode`);
        return (webImpl[method] as any)(...args);
      }
      if (typeof (nativeBridge as any)[method] === "function") {
        return (nativeBridge as any)[method](...args) as ReturnType<SimVersePlugin[K]>;
      }
    } catch (e) {
      console.error(`[SimVerse] Native call ${method} failed:`, e);
      throw e;
    }
  }
  return (webImpl[method] as any)(...args);
}

export const SimVerse: SimVersePlugin = {
  openWorld: (options) => callNative("openWorld", options),
  closeWorld: () => callNative("closeWorld"),
  startHeartbeat: () => callNative("startHeartbeat"),
  stopHeartbeat: () => callNative("stopHeartbeat"),
  setWorldRunning: (options) => callNative("setWorldRunning", options),
  addShortcut: () => callNative("addShortcut"),
  isShortcutSupported: () => callNative("isShortcutSupported"),
  lockOrientation: (options) => callNative("lockOrientation", options),
  unlockOrientation: () => callNative("unlockOrientation"),
  showDiagnostic: () => callNative("showDiagnostic"),
  hideSystemUI: () => callNative("hideSystemUI"),
  showSystemUI: () => callNative("showSystemUI"),
};

export function isNativePluginMode(): boolean {
  return !!(window as any).SimVerseNative;
}

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

export async function lockScreenOrientation(orientation: "landscape-primary" | "portrait-primary"): Promise<{ success: boolean }> {
  try {
    await SimVerse.lockOrientation({ orientation });
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] lockOrientation failed:", e);
    return { success: false };
  }
}

export async function unlockScreenOrientation(): Promise<{ success: boolean }> {
  try {
    await SimVerse.unlockOrientation();
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] unlockOrientation failed:", e);
    return { success: false };
  }
}

export async function showDiagnosticPanel(): Promise<{ success: boolean }> {
  try {
    await SimVerse.showDiagnostic();
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] showDiagnostic failed:", e);
    return { success: false };
  }
}

export async function hideSystemUI(): Promise<{ success: boolean }> {
  try {
    await SimVerse.hideSystemUI();
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] hideSystemUI failed:", e);
    return { success: false };
  }
}

export async function showSystemUI(): Promise<{ success: boolean }> {
  try {
    await SimVerse.showSystemUI();
    return { success: true };
  } catch (e) {
    console.error("[SimVerse] showSystemUI failed:", e);
    return { success: false };
  }
}
