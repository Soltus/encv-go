<!--
  ToolDetailContent - TDesign 风格工具详情渲染器

  智能解析工具参数/结果字符串，渲染为 TDesign 友好的结构化展示：
  1. 先尝试 JSON.parse
  2. 根据 shape 选择最佳渲染：
     - args（参数对象/对象数组）→ 简洁 key-value 表
     - mount list（mounts[]）→ 卡片列表（id / type / label / path）
     - file list（files[]/items[]）→ 简洁路径列表
     - file content（content/text/contentSnippet）→ 等宽字体块（带文件路径 title）
     - 普通对象 → 格式化 JSON（颜色 key/value）
     - 普通字符串 → 等宽字体块
     - 解析失败 → TDesign 配色 pre 块

  v4 设计要点：
  - 不依赖 TDesign ChatMarkdown（cherry-markdown 渲染 JSON 不友好）
  - 用 Vue 条件渲染分支，结构清晰
  - 所有列表项支持 click → 复制单条
-->
<template>
  <div class="td-tool-detail">
    <!-- 解析成功 + 对象形态 -->
    <template v-if="parsed && isObject(parsed)">
      <!-- ① mount list 渲染 -->
      <div v-if="isMountList" class="td-mt-list">
        <div
          v-for="m in mountItems"
          :key="(m as any).id"
          class="td-mt-item"
        >
          <div class="td-mt-itemHead">
            <span class="td-mt-itemIcon">📁</span>
            <span class="td-mt-itemName">{{ (m as any).id || (m as any).name || '(无 id)' }}</span>
            <span v-if="(m as any).type" class="td-mt-itemBadge">{{ (m as any).type }}</span>
          </div>
          <div v-if="(m as any).label || (m as any).path" class="td-mt-itemMeta">
            <span v-if="(m as any).label">{{ (m as any).label }}</span>
            <code v-if="(m as any).path">{{ (m as any).path }}</code>
          </div>
        </div>
      </div>

      <!-- ② file list 渲染（files[] / items[] / entries[]） -->
      <div v-else-if="isFileList" class="td-fl-list">
        <div
          v-for="(f, idx) in fileItems"
          :key="idx"
          class="td-fl-item"
        >
          <span class="td-fl-itemIcon">📄</span>
          <span class="td-fl-itemName">
            {{ (f as any).name || (f as any).path || (typeof f === 'string' ? f : JSON.stringify(f)) }}
          </span>
          <span v-if="(f as any).size != null" class="td-fl-itemSize">
            {{ formatSize((f as any).size) }}
          </span>
        </div>
      </div>

      <!-- ③ file content 渲染（read_file / cat） -->
      <div v-else-if="isFileContent" class="td-fc">
        <div v-if="(parsed as any).path" class="td-fc-path">
          <code>{{ (parsed as any).path }}</code>
        </div>
        <pre class="td-fc-body">{{ fileContentText }}</pre>
      </div>

      <!-- ④ 普通对象 → key-value 表 -->
      <div v-else class="td-kv">
        <div
          v-for="(value, key) in (parsed as Record<string, unknown>)"
          :key="String(key)"
          class="td-kv-row"
        >
          <span class="td-kv-key">{{ key }}</span>
          <span class="td-kv-val">
            <template v-if="isObject(value) || Array.isArray(value)">
              <pre class="td-kv-jsonInline">{{ JSON.stringify(value, null, 2) }}</pre>
            </template>
            <template v-else>
              {{ String(value) }}
            </template>
          </span>
        </div>
      </div>
    </template>

    <!-- 解析失败 OR 字符串形态 → 等宽 pre 块 -->
    <pre v-else class="td-raw">{{ displayText }}</pre>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

interface Props {
  /** 原始字符串（args 或 result 序列化后的 JSON 字符串） */
  raw: string;
  /** 'args' 或 'result' — 影响解析策略和样式 */
  kind: "args" | "result";
  /** 工具名（仅 result 模式使用，便于 mount list 识别） */
  toolName?: string;
}

const props = defineProps<Props>();

/** 解析结果（可能为 null 表示解析失败） */
const parsed = computed<unknown>(() => {
  if (!props.raw) return null;
  const s = String(props.raw).trim();
  if (!s) return null;
  try {
    return JSON.parse(s);
  } catch {
    return null;
  }
});

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** mount list 检测：mounts[] / mounts[].id+type */
const isMountList = computed(() => {
  if (Array.isArray(parsed.value)) {
    return (
      parsed.value.length > 0 &&
      parsed.value.every(m => isObject(m) && ("id" in m || "name" in m) && ("type" in m || "path" in m || "label" in m))
    );
  }
  if (isObject(parsed.value) && Array.isArray((parsed.value as any).mounts)) {
    return true;
  }
  return false;
});

const mountItems = computed<unknown[]>(() => {
  if (Array.isArray(parsed.value)) return parsed.value;
  if (isObject(parsed.value) && Array.isArray((parsed.value as any).mounts)) {
    return (parsed.value as any).mounts;
  }
  return [];
});

/** file list 检测：files[] / items[] / entries[] */
const isFileList = computed(() => {
  if (Array.isArray(parsed.value)) {
    return parsed.value.length > 0 && parsed.value.every(f => isObject(f) || typeof f === "string") && !isMountList.value;
  }
  for (const k of ["files", "items", "entries", "results"]) {
    if (isObject(parsed.value) && Array.isArray((parsed.value as any)[k])) {
      // 排除 count/ok 字段共存的 mount list 包装
      if (k === "files" || k === "items" || k === "entries") return true;
    }
  }
  return false;
});

const fileItems = computed<unknown[]>(() => {
  if (Array.isArray(parsed.value)) return parsed.value;
  if (isObject(parsed.value)) {
    for (const k of ["files", "items", "entries", "results"]) {
      if (Array.isArray((parsed.value as any)[k])) {
        return (parsed.value as any)[k];
      }
    }
  }
  return [];
});

/** file content 检测：content / text / contentSnippet 字段 */
const isFileContent = computed(() => {
  if (!isObject(parsed.value)) return false;
  const p = parsed.value as any;
  return typeof p.content === "string" || typeof p.text === "string" || typeof p.contentSnippet === "string";
});

const fileContentText = computed(() => {
  if (!isObject(parsed.value)) return "";
  const p = parsed.value as any;
  return p.content || p.text || p.contentSnippet || "";
});

/** 解析失败的展示文本（去除外层引号） */
const displayText = computed(() => {
  if (parsed.value !== null) return String(parsed.value);
  return props.raw || "";
});

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
}
</script>

<style scoped>
.td-tool-detail {
  font-size: 12px;
  color: var(--td-text-color-primary, #333);
}

/* ── key-value 表 ──────────────────────────── */
.td-kv {
  display: flex;
  flex-direction: column;
  gap: 4px;
  background: var(--td-bg-color-secondarycomponent, #f7f7f7);
  border-radius: 6px;
  padding: 8px 10px;
}
.td-kv-row {
  display: grid;
  grid-template-columns: 100px 1fr;
  gap: 8px;
  align-items: start;
  font-size: 12px;
  line-height: 1.5;
}
.td-kv-key {
  color: var(--td-brand-color, #4f8cff);
  font-weight: 500;
  font-family: 'SF Mono', Monaco, monospace;
  word-break: break-all;
}
.td-kv-val {
  color: var(--td-text-color-primary, #333);
  word-break: break-word;
  white-space: pre-wrap;
}
.td-kv-jsonInline {
  margin: 0;
  padding: 4px 6px;
  background: rgba(var(--td-brand-color-rgb, 79, 140, 255), 0.08);
  border-radius: 3px;
  font-family: 'SF Mono', Monaco, monospace;
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 200px;
  overflow-y: auto;
}

/* ── mount list ────────────────────────────── */
.td-mt-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.td-mt-item {
  padding: 8px 10px;
  background: var(--td-bg-color-secondarycomponent, #f7f7f7);
  border-left: 3px solid var(--td-brand-color, #4f8cff);
  border-radius: 4px;
}
.td-mt-itemHead {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.td-mt-itemIcon { font-size: 14px; }
.td-mt-itemName {
  font-weight: 600;
  font-family: 'SF Mono', Monaco, monospace;
  color: var(--td-text-color-primary, #333);
}
.td-mt-itemBadge {
  font-size: 10px;
  padding: 1px 6px;
  background: var(--td-brand-color, #4f8cff);
  color: #fff;
  border-radius: 3px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.td-mt-itemMeta {
  margin-top: 4px;
  font-size: 11px;
  color: var(--td-text-color-secondary, #666);
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.td-mt-itemMeta code {
  font-family: 'SF Mono', Monaco, monospace;
  font-size: 11px;
  color: var(--td-text-color-secondary, #666);
  word-break: break-all;
}

/* ── file list ─────────────────────────────── */
.td-fl-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  background: var(--td-bg-color-secondarycomponent, #f7f7f7);
  border-radius: 6px;
  padding: 4px 0;
}
.td-fl-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-size: 12px;
  color: var(--td-text-color-primary, #333);
}
.td-fl-item:hover {
  background: rgba(var(--td-brand-color-rgb, 79, 140, 255), 0.06);
}
.td-fl-itemIcon { font-size: 12px; opacity: 0.7; }
.td-fl-itemName {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: 'SF Mono', Monaco, monospace;
}
.td-fl-itemSize {
  font-size: 10px;
  color: var(--td-text-color-secondary, #999);
  flex-shrink: 0;
}

/* ── file content ──────────────────────────── */
.td-fc {
  background: var(--td-bg-color-secondarycomponent, #f7f7f7);
  border-radius: 6px;
  overflow: hidden;
}
.td-fc-path {
  padding: 6px 10px;
  font-size: 11px;
  color: var(--td-text-color-secondary, #666);
  background: rgba(var(--td-brand-color-rgb, 79, 140, 255), 0.06);
  border-bottom: 1px solid rgba(var(--td-component-stroke, 231, 231, 231), 0.5);
}
.td-fc-path code {
  font-family: 'SF Mono', Monaco, monospace;
  font-size: 11px;
}
.td-fc-body {
  margin: 0;
  padding: 8px 10px;
  font-family: 'SF Mono', Monaco, monospace;
  font-size: 11.5px;
  color: var(--td-text-color-primary, #333);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 300px;
  overflow-y: auto;
  line-height: 1.5;
}

/* ── 解析失败 / 字符串 fallback ─────────────── */
.td-raw {
  margin: 0;
  padding: 6px 8px;
  background: var(--td-bg-color-secondarycomponent, #f7f7f7);
  border-radius: 4px;
  font-family: 'SF Mono', Monaco, monospace;
  font-size: 11.5px;
  color: var(--td-text-color-primary, #333);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 200px;
  overflow-y: auto;
  line-height: 1.5;
}
</style>
