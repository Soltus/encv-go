import { Capacitor } from "@capacitor/core";

export async function copyToClipboard(text: string): Promise<boolean> {
  if (Capacitor.isNativePlatform()) {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        return true;
      }
    } catch {}
  }

  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {}
  }

  try {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.cssText =
      "position:fixed;left:0;top:0;width:2em;height:2em;padding:0;border:none;outline:none;box-shadow:none;background:transparent;opacity:0.01";
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(textarea);
    if (ok) return true;
  } catch {}

  return false;
}

export function selectAllText(el: HTMLTextAreaElement | HTMLInputElement) {
  el.focus();
  el.select();
  if (el instanceof HTMLTextAreaElement) {
    el.setSelectionRange(0, el.value.length);
  }
}
