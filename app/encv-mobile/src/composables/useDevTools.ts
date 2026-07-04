import { ref } from "vue";

const VCONSOLE_KEY = "encv_vconsole_enabled";

const vconsoleEnabled = ref(localStorage.getItem(VCONSOLE_KEY) === "true");
let vconsoleInstance: any = null;

export function useDevTools() {
  function toggleVConsole(enabled: boolean) {
    vconsoleEnabled.value = enabled;
    localStorage.setItem(VCONSOLE_KEY, String(enabled));
    if (enabled) {
      initVConsole();
    } else {
      destroyVConsole();
    }
  }

  return {
    vconsoleEnabled,
    toggleVConsole,
  };
}

export async function initVConsole() {
  if (vconsoleInstance) return;
  try {
    const VConsole = (await import("vconsole")).default;
    vconsoleInstance = new VConsole();
  } catch (e) {
    console.error("[ENCV] Failed to init vConsole:", e);
  }
}

export function destroyVConsole() {
  if (vconsoleInstance) {
    try {
      vconsoleInstance.destroy();
    } catch {}
    vconsoleInstance = null;
  }
}

export function autoInitVConsole() {
  if (localStorage.getItem(VCONSOLE_KEY) === "true") {
    initVConsole();
  }
}
