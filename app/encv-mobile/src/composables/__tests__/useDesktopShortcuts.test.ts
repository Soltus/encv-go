import { afterEach, describe, expect, it, vi } from "vitest";
import { closeTopOverlay, createDesktopShortcutsHandler, focusPageSearch, isEditableTarget } from "@/composables/useDesktopShortcuts";

// Task 1.3 桌面快捷键契约：/ 聚焦搜索、Esc 关浮层；手机端 / 输入位内绝不抢键。

function setupShell(): { search: HTMLElement } {
  document.body.innerHTML = `
    <div class="desktop-shell">
      <div class="desktop-content">
        <div class="ion-page">
          <div data-testid="search-input" tabindex="0" contenteditable="true"></div>
        </div>
      </div>
    </div>`;
  const search = document.querySelector<HTMLElement>("[data-testid='search-input']");
  if (!search) throw new Error("fixture missing search input");
  return { search };
}

function keyEvent(key: string, target?: EventTarget, init: KeyboardEventInit = {}): KeyboardEvent {
  const e = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
  if (target) Object.defineProperty(e, "target", { value: target });
  return e;
}

afterEach(() => {
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

describe("useDesktopShortcuts — `/` 聚焦搜索", () => {
  it("桌面形态：按 `/` 聚焦当前页搜索框并 preventDefault", () => {
    const { search } = setupShell();
    const handler = createDesktopShortcutsHandler({ isDesktop: () => true });
    const e = keyEvent("/");
    handler(e);
    expect(document.activeElement).toBe(search);
    expect(e.defaultPrevented).toBe(true);
  });

  it("手机形态：不响应（零行为变化）", () => {
    setupShell();
    const handler = createDesktopShortcutsHandler({ isDesktop: () => false });
    const e = keyEvent("/");
    handler(e);
    expect(document.activeElement).toBe(document.body);
    expect(e.defaultPrevented).toBe(false);
  });

  it("已在文本输入位内：不劫持，`/` 正常输入", () => {
    setupShell();
    const handler = createDesktopShortcutsHandler({ isDesktop: () => true });
    const editable = document.querySelector<HTMLElement>("[data-testid='search-input']");
    if (!editable) throw new Error("fixture missing");
    editable.focus();
    const e = keyEvent("/", editable);
    handler(e);
    expect(e.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(editable); // 焦点未被移动
  });

  it("带修饰键（Ctrl+/ 等）：不劫持", () => {
    setupShell();
    const handler = createDesktopShortcutsHandler({ isDesktop: () => true });
    const e = keyEvent("/", undefined, { ctrlKey: true });
    handler(e);
    expect(e.defaultPrevented).toBe(false);
  });

  it("当前页没有搜索框：不 preventDefault（不吞按键）", () => {
    document.body.innerHTML = `<div class="desktop-content"><div class="ion-page"></div></div>`;
    const handler = createDesktopShortcutsHandler({ isDesktop: () => true });
    const e = keyEvent("/");
    handler(e);
    expect(e.defaultPrevented).toBe(false);
  });
});

describe("useDesktopShortcuts — Esc 关浮层", () => {
  function makeOverlay(tag: string, hidden = false, dismiss = vi.fn(async () => true)): HTMLElement {
    const el = document.createElement(tag);
    if (hidden) el.classList.add("overlay-hidden");
    (el as HTMLElement & { dismiss: unknown }).dismiss = dismiss;
    document.body.appendChild(el);
    return el;
  }

  it("Esc 关闭最上层浮层（同类型取 DOM 最后一个）", () => {
    const d1 = vi.fn(async () => true);
    const d2 = vi.fn(async () => true);
    makeOverlay("ion-modal", false, d1);
    makeOverlay("ion-modal", false, d2);
    const handler = createDesktopShortcutsHandler({ isDesktop: () => true });
    handler(keyEvent("Escape"));
    expect(d2).toHaveBeenCalledTimes(1);
    expect(d1).not.toHaveBeenCalled();
  });

  it("overlay-hidden 的浮层不算打开，不误关", () => {
    const dismiss = vi.fn(async () => true);
    makeOverlay("ion-modal", true, dismiss);
    const handler = createDesktopShortcutsHandler({ isDesktop: () => true });
    handler(keyEvent("Escape"));
    expect(dismiss).not.toHaveBeenCalled();
  });

  it("不同类型浮层按 alert > modal 优先级由近到远（先关 alert 类）", () => {
    const dAlert = vi.fn(async () => true);
    const dModal = vi.fn(async () => true);
    makeOverlay("ion-modal", false, dModal);
    makeOverlay("ion-alert", false, dAlert);
    const handler = createDesktopShortcutsHandler({ isDesktop: () => true });
    handler(keyEvent("Escape"));
    expect(dAlert).toHaveBeenCalledTimes(1);
    expect(dModal).not.toHaveBeenCalled();
  });

  it("无浮层打开时 Esc 不产生副作用", () => {
    setupShell();
    const handler = createDesktopShortcutsHandler({ isDesktop: () => true });
    expect(() => handler(keyEvent("Escape"))).not.toThrow();
  });
});

describe("useDesktopShortcuts — 纯工具函数", () => {
  it("isEditableTarget：contenteditable / 文本 input 为真，按钮类为假", () => {
    const div = document.createElement("div");
    div.setAttribute("contenteditable", "true");
    expect(isEditableTarget(div)).toBe(true);
    const input = document.createElement("input");
    input.setAttribute("type", "text");
    expect(isEditableTarget(input)).toBe(true);
    const btn = document.createElement("input");
    btn.setAttribute("type", "button");
    expect(isEditableTarget(btn)).toBe(false);
    expect(isEditableTarget(document.createElement("span"))).toBe(false);
    expect(isEditableTarget(null)).toBe(false);
  });

  it("focusPageSearch：无激活页时回退全文档查询，仍可聚焦", () => {
    document.body.innerHTML = `<div data-testid="search-input" tabindex="0"></div>`;
    expect(focusPageSearch()).toBe(true);
    expect(document.activeElement?.getAttribute("data-testid")).toBe("search-input");
  });

  it("closeTopOverlay：dismiss 缺失时跳过且不抛错", () => {
    const el = document.createElement("ion-popover");
    document.body.appendChild(el);
    expect(closeTopOverlay()).toBe(false);
  });
});
