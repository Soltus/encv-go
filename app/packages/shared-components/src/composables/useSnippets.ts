import { ref } from "vue";
import { comfyContrastCss } from "../theme/snippets/comfy-contrast";
import { surfaceOverrideCss } from "../theme/snippets/surface-override";

/**
 * Phase 5 · CSS Snippets 热开关（局部覆盖，零后端）。
 *
 * Snippet = 一段可注入 <head> 的局部 CSS 覆盖，运行时开关、独立持久化。
 * 生产环境的片段应来自 `user-themes/<id>/theme.css` 或远程清单（见 THEME_DEV.md），
 * 此处以注册表 + 内联字符串演示同一注入/持久化通道。
 */

export interface SnippetMeta {
  id: string;
  /** i18n 标签键；同名 + "Help" 后缀为说明键。 */
  labelKey: string;
  css: string;
}

/** Snippet 注册表。新增片段：在此登记 + 提供 CSS 字符串（或远程加载）。 */
export const SNIPPETS: SnippetMeta[] = [
  { id: "comfy-contrast", labelKey: "settings.snippetComfyContrast", css: comfyContrastCss },
  { id: "surface-override", labelKey: "settings.snippetSurfaceOverride", css: surfaceOverrideCss },
];

const SNIPPET_KEY = "encv-snippets";

const enabled = ref<string[]>([]);

function injectCss(id: string, css: string) {
  if (typeof document === "undefined") return;
  const elId = `encv-snippet-${id}`;
  if (document.getElementById(elId)) return;
  const el = document.createElement("style");
  el.id = elId;
  el.setAttribute("data-encv-snippet", id);
  el.textContent = css;
  document.head.appendChild(el);
}

function removeCss(id: string) {
  if (typeof document === "undefined") return;
  const el = document.getElementById(`encv-snippet-${id}`);
  if (el) el.remove();
}

function readStored(): string[] {
  if (typeof localStorage === "undefined") return [];
  const raw = localStorage.getItem(SNIPPET_KEY);
  if (!raw) return [];
  try {
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? arr.filter((s: unknown): s is string => typeof s === "string" && SNIPPETS.some(x => x.id === s)) : [];
  } catch {
    return [];
  }
}

// 模块加载即水合：恢复已开启的片段。
const stored = readStored();
stored.forEach(id => {
  const meta = SNIPPETS.find(s => s.id === id);
  if (meta) {
    injectCss(id, meta.css);
    enabled.value = [...enabled.value, id];
  }
});

export function useSnippets() {
  function isEnabled(id: string): boolean {
    return enabled.value.includes(id);
  }

  function toggle(id: string) {
    const meta = SNIPPETS.find(s => s.id === id);
    if (!meta) return;
    if (enabled.value.includes(id)) {
      enabled.value = enabled.value.filter(x => x !== id);
      removeCss(id);
    } else {
      enabled.value = [...enabled.value, id];
      injectCss(id, meta.css);
    }
    if (typeof localStorage !== "undefined") {
      localStorage.setItem(SNIPPET_KEY, JSON.stringify(enabled.value));
    }
  }

  return { snippets: SNIPPETS, isEnabled, toggle };
}
