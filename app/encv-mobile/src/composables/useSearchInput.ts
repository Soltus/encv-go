/**
 * useSearchInput.ts — contenteditable 搜索框 + span units 管理
 *
 * 2026-07-02 重写：替换原 <ion-searchbar> + 外部 overlay div 的"高亮在输入框外"反模式
 *
 * 设计原则（用户强反馈）：
 *   1. 输入框是 <div contenteditable>，不是 ion-searchbar
 *   2. 高亮 span 在 input 内部（用户输入的文字也是 span）
 *   3. 插入按钮插入 symbol span（&/｜/￢）到光标位置，不是字符串 " AND "
 *   4. 序列化时把所有 span 拼回 query 字符串
 *   5. 输入连续文本时按空格自动 split 为新 text span
 *
 * span unit 类型（data-kind）：
 *   - 'text'    普通文本（用户键入）
 *   - 'op'      操作符（data-op=AND|OR|NOT|__qmark__|__regex_prefix__）
 *                显示为 & / ｜ / ￢ / 「 / 」 / regex:
 *
 * 与 Go 端 FTS5 的对应关系（serializeQueryDiv 拼回）：
 *   - text span    → 直接拼接
 *   - op AND/OR/NOT → ' AND ' / ' OR ' / ' NOT '
 *   - op __qmark__  → '"' (phrase 引号)
 *   - op __regex__  → 'regex:' (regex 前缀)
 *
 * 公开 API（与 useFilesView 集成）：
 *   - queryInputRef:  绑到 <div contenteditable>
 *   - queryValue:     当前 query 字符串（响应式）
 *   - onQueryInput:   @input 处理器
 *   - onQueryKeydown: @keydown 处理器
 *   - insertSymbol:   插入 symbol span 到当前光标位置
 *   - clearInput:     清空
 *   - syncFromExternal: 外部值变化时同步到 div
 */

import { onBeforeUnmount, onMounted, type Ref, ref, watch } from "vue";

export type SpanKind = "text" | "op";

export type OpKey = "AND" | "OR" | "NOT" | "__phrase_open__" | "__phrase_close__" | "__regex_prefix__";

export interface OpSymbol {
  /** 显示字符（全角） */
  display: string;
  /** 序列化到后端的对应 token */
  serialize: string;
  /** 视觉 class（CSS 用） */
  cls: string;
}

export const OP_SYMBOLS: Record<string, OpSymbol> = {
  AND: { display: "＆", serialize: " AND ", cls: "syntax-op" },
  OR: { display: "｜", serialize: " OR ", cls: "syntax-or" },
  NOT: { display: "￢", serialize: " NOT ", cls: "syntax-not" },
  __phrase_open__: { display: "「", serialize: '"', cls: "syntax-phrase" },
  __phrase_close__: { display: "」", serialize: '"', cls: "syntax-phrase" },
  __regex_prefix__: { display: "regex:", serialize: "regex:", cls: "syntax-regex" },
};

/**
 * 从 div 的 children 重建 query 字符串。
 *
 * 健壮性兜底：除了 span children，孤儿 text node 也按 text 处理。
 * （正常 input 事件后 wrapOrphanTextNodes 会把所有 text node 包成 span，
 * 但保留兜底可以防止 wrap 失败时丢内容）
 */
export function serializeQueryDiv(div: HTMLElement): string {
  const parts: string[] = [];
  for (const child of Array.from(div.childNodes)) {
    if (child.nodeType === Node.ELEMENT_NODE) {
      const el = child as HTMLElement;
      const kind = el.dataset.kind as SpanKind | undefined;
      if (kind === "op") {
        const opKey = el.dataset.op || "AND";
        const sym = OP_SYMBOLS[opKey];
        parts.push(sym ? sym.serialize : "");
      } else {
        // text span（或无 data-kind 的元素）
        parts.push(el.textContent || "");
      }
    } else if (child.nodeType === Node.TEXT_NODE) {
      // 孤儿 text node 兜底
      parts.push(child.textContent || "");
    }
  }
  return parts.join("").replace(/\s+/g, " ").trim();
}

/**
 * 把光标位置的纯文本节点（没被 span 包裹的）包成 text span。
 */
function wrapOrphanTextNodes(div: HTMLElement) {
  for (const node of Array.from(div.childNodes)) {
    if (node.nodeType === Node.TEXT_NODE) {
      const text = node.textContent || "";
      if (text === "") continue;
      const span = document.createElement("span");
      span.dataset.kind = "text";
      span.classList.add("syntax-text-span");
      span.textContent = text;
      node.parentNode?.replaceChild(span, node);
    }
  }
}

/**
 * 合并相邻的 text span（删一个 span 时被分割的两个 text span 需合并）。
 */
function mergeAdjacentTextSpans(div: HTMLElement) {
  const children = Array.from(div.children) as HTMLElement[];
  for (let i = 0; i < children.length - 1; i++) {
    const a = children[i];
    const b = children[i + 1];
    if (a.dataset.kind === "text" && b.dataset.kind === "text") {
      a.textContent = (a.textContent || "") + (b.textContent || "");
      div.removeChild(b);
      children.splice(i + 1, 1);
      i--;
    }
  }
}

function normalizeAfterEnter(div: HTMLElement) {
  const children = Array.from(div.childNodes);
  for (const node of children) {
    if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node as HTMLElement;
      const tag = el.tagName.toLowerCase();
      if (tag === "div" || tag === "p") {
        const textSpan = document.createElement("span");
        textSpan.dataset.kind = "text";
        textSpan.classList.add("syntax-text-span");
        textSpan.textContent = el.textContent || "";
        el.parentNode?.replaceChild(textSpan, el);
      } else if (tag === "br") {
        const brSpan = document.createElement("span");
        brSpan.dataset.kind = "text";
        brSpan.classList.add("syntax-text-span");
        brSpan.innerHTML = "<br>";
        el.parentNode?.replaceChild(brSpan, el);
      }
    }
  }
  wrapOrphanTextNodes(div);
  mergeAdjacentTextSpans(div);
}

/**
 * 计算光标在 div 中的字符偏移量（考虑所有子节点的 textContent）。
 * 用于 DOM 修改前后保存/恢复光标位置。
 */
function getCaretOffset(div: HTMLElement): number {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return 0;
  const range = sel.getRangeAt(0);
  if (!div.contains(range.endContainer)) return 0;

  const preRange = document.createRange();
  preRange.selectNodeContents(div);
  preRange.setEnd(range.endContainer, range.endOffset);
  return preRange.toString().length;
}

/**
 * 根据字符偏移量设置光标位置。
 */
function setCaretOffset(div: HTMLElement, offset: number) {
  const sel = window.getSelection();
  if (!sel) return;

  const selection = sel;
  let remaining = offset;

  function walkNodes(node: Node): boolean {
    if (node.nodeType === Node.TEXT_NODE) {
      const len = (node.textContent || "").length;
      if (remaining <= len) {
        const range = document.createRange();
        range.setStart(node, remaining);
        range.collapse(true);
        selection.removeAllRanges();
        selection.addRange(range);
        return true;
      }
      remaining -= len;
      return false;
    }
    for (const child of Array.from(node.childNodes)) {
      if (walkNodes(child)) return true;
    }
    return false;
  }

  if (!walkNodes(div)) {
    // 偏移超出范围 → 放末尾
    placeCaretAtEnd(div);
  }
}

/**
 * 把光标放到 div 末尾。
 */
function placeCaretAtEnd(div: HTMLElement) {
  const range = document.createRange();
  range.selectNodeContents(div);
  range.collapse(false);
  const sel = window.getSelection();
  if (sel) {
    sel.removeAllRanges();
    sel.addRange(range);
  }
}

export function useSearchInput(options: { externalQuery?: Ref<string>; onChange?: (query: string) => void } = {}) {
  const queryInputRef = ref<HTMLElement | null>(null);
  const queryValue = ref("");

  let syncing = false;

  function syncFromDiv() {
    if (syncing) return;
    if (!queryInputRef.value) return;
    const newVal = serializeQueryDiv(queryInputRef.value);
    if (newVal !== queryValue.value) {
      queryValue.value = newVal;
      options.onChange?.(newVal);
      if (options.externalQuery) {
        syncing = true;
        options.externalQuery.value = newVal;
        queueMicrotask(() => {
          syncing = false;
        });
      }
    }
  }

  function onQueryInput(_e: Event) {
    if (!queryInputRef.value) return;
    const div = queryInputRef.value;
    // 🆕 修复光标定位：DOM 修改前保存偏移量
    const caretOffset = getCaretOffset(div);
    wrapOrphanTextNodes(div);
    mergeAdjacentTextSpans(div);
    // 🆕 修复光标定位：DOM 修改后恢复偏移量
    setCaretOffset(div, caretOffset);
    syncFromDiv();
  }

  function onQueryKeydown(e: KeyboardEvent) {
    if (!queryInputRef.value) return;

    if (e.key === "Enter") {
      const div = queryInputRef.value;
      const beforeOffset = getCaretOffset(div);
      requestAnimationFrame(() => {
        if (!queryInputRef.value) return;
        normalizeAfterEnter(queryInputRef.value);
        const newOffset = beforeOffset + 1;
        setCaretOffset(queryInputRef.value, newOffset);
        syncFromDiv();
      });
      return;
    }

    if (e.key === "Backspace" || e.key === "Delete") {
      const div = queryInputRef.value;
      const beforeOffset = getCaretOffset(div);
      requestAnimationFrame(() => {
        if (!queryInputRef.value) return;
        mergeAdjacentTextSpans(queryInputRef.value);
        const afterOffset = getCaretOffset(queryInputRef.value);
        setCaretOffset(queryInputRef.value, Math.max(0, Math.min(afterOffset, beforeOffset)));
        syncFromDiv();
      });
    }
  }

  /**
   * 插入 symbol span 到当前光标位置。
   *
   *  - kind='op' → op span (data-op=opKey)
   *  - kind='text' → text span
   *
   *  phrase 特殊：连续插入「 」两个 op span + 中间插入空 text span 占位
   *  regex 特殊：插入 'regex:' op span
   */
  function insertSymbol(opKey: string) {
    if (!queryInputRef.value) return;
    const div = queryInputRef.value;
    div.focus();
    wrapOrphanTextNodes(div);

    const sym = OP_SYMBOLS[opKey];
    if (!sym) return;

    function makeOpSpan() {
      const span = document.createElement("span");
      span.dataset.kind = "op";
      span.dataset.op = opKey;
      span.contentEditable = "false";
      span.classList.add("syntax-op-span", sym.cls);
      span.textContent = sym.display;
      return span;
    }

    function insertAtCaret(node: Node) {
      const sel = window.getSelection();
      if (sel && sel.rangeCount > 0) {
        const range = sel.getRangeAt(0);
        if (div.contains(range.commonAncestorContainer)) {
          range.deleteContents();

          // 🐛 2026-07-02 修复：range.insertNode 会把节点插到 span 内部
          //   （光标在 span text node 里时），导致节点嵌套层级错乱。
          //   修复：找到光标对应的 div 直接子节点，用 insertBefore 在正确层级插入。
          let targetChild: Node | null = null;
          let offsetInChild = 0;

          // 向上找到 div 的直接子节点
          let cur: Node | null = range.startContainer;
          while (cur && cur.parentNode !== div) {
            cur = cur.parentNode;
          }
          if (cur) {
            targetChild = cur;
            // 计算在 targetChild 内的字符偏移（粗略：如果 startContainer 是 text node 且在 targetChild 内）
            if (range.startContainer.nodeType === Node.TEXT_NODE) {
              // 遍历 targetChild 的文本内容累计长度
              let textLen = 0;
              const walker = document.createTreeWalker(targetChild, NodeFilter.SHOW_TEXT);
              let tn: Node | null;
              while ((tn = walker.nextNode())) {
                if (tn === range.startContainer) {
                  textLen += range.startOffset;
                  break;
                }
                textLen += (tn.textContent || "").length;
              }
              offsetInChild = textLen;
            } else if (range.startContainer === targetChild) {
              offsetInChild = 0;
            } else {
              offsetInChild = (targetChild.textContent || "").length;
            }
          }

          if (targetChild) {
            const childLen = (targetChild.textContent || "").length;
            if (offsetInChild === 0) {
              // 在 targetChild 前面插入
              div.insertBefore(node, targetChild);
            } else if (offsetInChild >= childLen) {
              // 在 targetChild 后面插入
              div.insertBefore(node, targetChild.nextSibling);
            } else {
              // 在中间 → 拆分 text span
              if (targetChild.nodeType === Node.ELEMENT_NODE && (targetChild as HTMLElement).dataset.kind === "text") {
                const text = targetChild.textContent || "";
                const beforeText = text.slice(0, offsetInChild);
                const afterText = text.slice(offsetInChild);
                const beforeSpan = document.createElement("span");
                beforeSpan.dataset.kind = "text";
                beforeSpan.classList.add("syntax-text-span");
                beforeSpan.textContent = beforeText;
                const afterSpan = document.createElement("span");
                afterSpan.dataset.kind = "text";
                afterSpan.classList.add("syntax-text-span");
                afterSpan.textContent = afterText;
                div.insertBefore(beforeSpan, targetChild);
                div.insertBefore(node, targetChild);
                div.insertBefore(afterSpan, targetChild);
                div.removeChild(targetChild);
              } else {
                // op span 等不可拆分 → 在后面插入
                div.insertBefore(node, targetChild.nextSibling);
              }
            }
          } else {
            // 没找到（空 div）→ 追加
            div.appendChild(node);
          }

          // 设置光标到插入节点之后
          const r2 = document.createRange();
          r2.setStartAfter(node);
          r2.collapse(true);
          sel.removeAllRanges();
          sel.addRange(r2);
          return true;
        }
      }
      // 光标不在 div 内 → 追加到末尾
      div.appendChild(node);
      placeCaretAtEnd(div);
      return false;
    }

    if (opKey === "__phrase_open__") {
      // 「 + 空 text + 」+ 光标回到 text 中间
      const openSpan = makeOpSpan();
      const textSpan = document.createElement("span");
      textSpan.dataset.kind = "text";
      textSpan.classList.add("syntax-text-span");
      textSpan.textContent = "";
      const closeSpan = makeOpSpan();
      closeSpan.dataset.op = "__phrase_close__";
      closeSpan.classList.add("syntax-op-span", OP_SYMBOLS.__phrase_close__.cls);
      closeSpan.textContent = OP_SYMBOLS.__phrase_close__.display;

      insertAtCaret(openSpan);
      // insertAtCaret 保证 openSpan 是 div 的直接子节点
      const ref = openSpan.nextSibling;
      div.insertBefore(textSpan, ref);
      div.insertBefore(closeSpan, textSpan.nextSibling);
      // 光标到 text 中间
      const r2 = document.createRange();
      r2.selectNodeContents(textSpan);
      r2.collapse(true);
      window.getSelection()?.removeAllRanges();
      window.getSelection()?.addRange(r2);
    } else {
      const span = makeOpSpan();
      insertAtCaret(span);
    }

    syncFromDiv();
  }

  function clearInput() {
    if (!queryInputRef.value) return;
    queryInputRef.value.innerHTML = "";
    queryValue.value = "";
    options.onChange?.("");
    if (options.externalQuery) {
      syncing = true;
      options.externalQuery.value = "";
      queueMicrotask(() => {
        syncing = false;
      });
    }
    placeCaretAtEnd(queryInputRef.value);
  }

  /**
   * 外部值变化时同步到 div。
   */
  watch(
    () => options.externalQuery?.value,
    newVal => {
      if (syncing) return;
      if (!queryInputRef.value) return;
      const cur = serializeQueryDiv(queryInputRef.value);
      if (cur === newVal) return;
      syncing = true;
      if (!newVal) {
        queryInputRef.value.innerHTML = "";
      } else {
        queryInputRef.value.innerHTML = "";
        const span = document.createElement("span");
        span.dataset.kind = "text";
        span.classList.add("syntax-text-span");
        span.textContent = newVal;
        queryInputRef.value.appendChild(span);
      }
      queryValue.value = newVal || "";
      syncing = false;
    }
  );

  onMounted(() => {
    if (options.externalQuery?.value && queryInputRef.value) {
      const span = document.createElement("span");
      span.dataset.kind = "text";
      span.classList.add("syntax-text-span");
      span.textContent = options.externalQuery.value;
      queryInputRef.value.appendChild(span);
      queryValue.value = options.externalQuery.value;
    }
  });

  onBeforeUnmount(() => {
    // 清理（如有需要）
  });

  return {
    queryInputRef,
    queryValue,
    onQueryInput,
    onQueryKeydown,
    insertSymbol,
    clearInput,
  };
}
