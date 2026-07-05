<!--
  MarkdownStream - 流式 Markdown 渲染
  封装 markstream-vue 的 MarkdownRender（default export）
  正确 prop: content（不是 source！）

  关键：不用 :key 绑定 content，否则每次 text_delta 都销毁重建 → 闪烁
  markstream-vue 自身的 smooth-streaming prop 会处理增量更新
-->
<template>
  <div class="markdownStream" :class="{ markdownStream_streaming: streaming }">
    <MarkdownRender
      :content="content"
      :smooth-streaming="streaming"
      :custom-markdown-it="customizeMarkdownIt"
    />
  </div>
</template>

<script setup lang="ts">
import type { MarkdownIt } from "markstream-vue";
import "markstream-vue/index.css";

defineProps<{
  content: string;
  streaming?: boolean;
}>();

/**
 * 自定义 markdown-it 配置：启用 linkify（自动将裸 URL 转为 <a> 标签）
 * 解决 LLM 输出中 https://... 未用 [text](url) 包裹时无法点击的问题
 */
function _customizeMarkdownIt(md: MarkdownIt): MarkdownIt {
  // 启用 linkify：自动识别并链接裸 URL（https?://、ftp://、www. 开头等）
  // md.linkify 是 markdown-it 内置的 linkify 插件开关
  if ("linkify" in md && typeof (md as unknown as Record<string, unknown>).linkify === "boolean") {
    (md as unknown as { linkify: boolean }).linkify = true;
  }
  return md;
}
</script>

<style>
/* ── 全局样式：让 markstream 在 dark mode 下可读 ── */
.markdownStream {
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--ion-text-color);
  word-break: break-word;
  user-select: text;
  -webkit-user-select: text;
}

/* ── 流式输出平滑过渡（容器级） ── */
.markdownStream_streaming .markstream-vue {
  /* 高度变化时平滑过渡，消除内容追加时的跳动 */
  transition: height 0.3s cubic-bezier(0.25, 0.46, 0.45, 0.94);
}

/* ── 流式输出：新内容块淡入上滑 ── */
.markdownStream_streaming .markstream-vue > * {
  animation: streamFadeIn 0.4s ease-out both;
}

/* 错开入场时间，避免所有段落同时闪烁 */
.markdownStream_streaming .markstream-vue > *:nth-child(1)  { animation-delay: 0ms; }
.markdownStream_streaming .markstream-vue > *:nth-child(2)  { animation-delay: 50ms; }
.markdownStream_streaming .markstream-vue > *:nth-child(3)  { animation-delay: 100ms; }
.markdownStream_streaming .markstream-vue > *:nth-child(4)  { animation-delay: 150ms; }
.markdownStream_streaming .markstream-vue > *:nth-child(5)  { animation-delay: 200ms; }
.markdownStream_streaming .markstream-vue > *:nth-child(n+6) { animation-delay: 250ms; }

@keyframes streamFadeIn {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* ── 排版规则 ── */
.markdownStream p {
  margin: 6px 0;
}

.markdownStream h1,
.markdownStream h2,
.markdownStream h3,
.markdownStream h4,
.markdownStream h5,
.markdownStream h6 {
  margin: 12px 0 6px;
  font-weight: 700;
  color: var(--ion-text-color);
}
.markdownStream h1 { font-size: 18px; }
.markdownStream h2 { font-size: 16px; }
.markdownStream h3 { font-size: 15px; }
.markdownStream h4 { font-size: 14px; }
.markdownStream h5 { font-size: 13.5px; }
.markdownStream h6 { font-size: 13px; }

.markdownStream ul,
.markdownStream ol {
  padding-left: 20px;
  margin: 6px 0;
}
.markdownStream li {
  margin: 2px 0;
}

.markdownStream code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  padding: 1px 5px;
  background: rgba(var(--ion-color-medium-rgb), 0.16);
  border-radius: 4px;
}

.markdownStream pre {
  margin: 6px 0;
  padding: 8px 12px;
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  border-radius: 6px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  overflow: auto;
}
.markdownStream pre code {
  background: transparent;
  padding: 0;
  font-size: 12px;
}

.markdownStream blockquote {
  margin: 6px 0;
  padding: 4px 10px;
  border-left: 3px solid rgba(var(--ion-color-primary-rgb), 0.4);
  color: var(--encv-text-secondary);
  background: rgba(var(--ion-color-primary-rgb), 0.04);
  border-radius: 0 4px 4px 0;
}

.markdownStream a {
  color: var(--ion-color-primary);
  text-decoration: none;
}
.markdownStream a:hover {
  text-decoration: underline;
}

.markdownStream table {
  border-collapse: collapse;
  margin: 6px 0;
  font-size: 12.5px;
  width: 100%;
}
.markdownStream th,
.markdownStream td {
  padding: 4px 8px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.3);
  text-align: left;
}
.markdownStream th {
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  font-weight: 600;
}

/* 流式期间 code block 降低饱和度，避免高亮闪烁 */
.markdownStream_streaming pre {
  filter: saturate(0.85);
}
</style>
