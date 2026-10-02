/**
 * 桌面快捷键（spec desktop-web-android-pairing Task 1.3）：
 *   `/`  聚焦当前页搜索框（Files 页 `data-testid="search-input"`；已在输入框内 / 有浮层打开时不抢）
 *   Esc  关闭最上层 Ionic 浮层（alert / action-sheet / loading / picker / popover / modal）
 *
 * 仅桌面形态生效（isDesktop 注入，默认 useFormFactor）；手机端零行为变化。
 * isDesktop 可注入 + 处理器可独立创建，便于单测（无需 stub 视口 / 挂载组件）。
 */
import { onMounted, onUnmounted } from "vue";
import { useFormFactor } from "@encv/shared-components/composables/useFormFactor";

const EDITABLE_TAGS = new Set(["INPUT", "TEXTAREA", "SELECT"]);
/** 这些 input 类型不算"文本输入位"，`/` 仍可聚焦搜索 */
const NON_TEXT_INPUT_TYPES = new Set(["button", "submit", "reset", "checkbox", "radio", "file", "image", "range"]);

/** 由近到远尝试关闭的浮层选择器（后挂载的在 DOM 后面，取同类的最后一个 = 最上层） */
const OVERLAY_SELECTORS = ["ion-alert", "ion-action-sheet", "ion-loading", "ion-picker", "ion-popover", "ion-modal"] as const;

type Dismissible = { dismiss?: (data?: unknown, role?: string) => Promise<boolean> };

export function isEditableTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el.isContentEditable) return true;
  if (!EDITABLE_TAGS.has(el.tagName)) return false;
  const type = (el.getAttribute("type") ?? "").toLowerCase();
  return !NON_TEXT_INPUT_TYPES.has(type);
}

/** 聚焦当前激活页的搜索框；找不到（当前页无搜索框）返回 false 不抢焦点 */
export function focusPageSearch(root: ParentNode = document): boolean {
  const page = root.querySelector(".desktop-content .ion-page:not(.ion-page-hidden)") ?? root;
  const target = page.querySelector<HTMLElement>("[data-testid='search-input']");
  if (!target) return false;
  target.focus();
  return true;
}

/** 关闭最上层未隐藏的 Ionic 浮层；没有可关的返回 false */
export function closeTopOverlay(root: ParentNode = document): boolean {
  for (const sel of OVERLAY_SELECTORS) {
    const overlays = [...root.querySelectorAll(sel)].filter(el => !el.classList.contains("overlay-hidden")) as HTMLElement[];
    const top = overlays.at(-1);
    if (!top) continue;
    const dismiss = (top as HTMLElement & Dismissible).dismiss;
    if (typeof dismiss === "function") {
      void dismiss.call(top, undefined, "desktop-esc");
      return true;
    }
  }
  return false;
}

export interface DesktopShortcutsOptions {
  isDesktop?: () => boolean;
}

/** 创建 keydown 处理器（纯函数，便于单测） */
export function createDesktopShortcutsHandler(options: DesktopShortcutsOptions = {}): (e: KeyboardEvent) => void {
  const isDesktop = options.isDesktop ?? (() => useFormFactor().isDesktop.value);
  return (e: KeyboardEvent) => {
    if (!isDesktop()) return;
    if (e.key === "Escape") {
      closeTopOverlay();
      return;
    }
    if (e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return;
    if (isEditableTarget(e.target)) return; // 已在文本输入位：让 "/" 正常输入
    if (closeTopOverlay()) return; // 有浮层打开时不抢焦点
    if (focusPageSearch()) e.preventDefault();
  };
}

/** Tabs.vue 桌面壳接入点：全局 keydown（处理器内部自行判断桌面形态，手机端零行为变化） */
export function useDesktopShortcuts(options: DesktopShortcutsOptions = {}): void {
  const onKeyDown = createDesktopShortcutsHandler(options);
  onMounted(() => document.addEventListener("keydown", onKeyDown));
  onUnmounted(() => document.removeEventListener("keydown", onKeyDown));
}
