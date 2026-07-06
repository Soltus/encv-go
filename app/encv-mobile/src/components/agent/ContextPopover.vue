<!--
  ContextPopover - 上下文详情弹窗内容

  分 3 个区段：
  1. Usage 区：进度条 + 数字（tokens / window / percent）+ model
  2. Todos 区：当前 plan 的步骤列表（含状态徽章）
  3. Referenced Files 区：最近工具调用引用的文件

  视觉设计参考 codex_web：
  - 顶部 progress bar 渐变（绿→黄→红）
  - 每区独立 card 样式
  - 状态徽章：pending/idle/warn/ready 三种 tone
  - 文件行等宽字体 + truncate
-->
<template>
  <div class="ctx-popover">
    <header class="ctx-header">
      <ion-icon :icon="layersIcon" class="ctx-header-icon" />
      <div class="ctx-header-text">
        <h3>上下文使用</h3>
        <p v-if="data?.model">{{ data.model }} · session {{ data.sessionId }}</p>
        <p v-else>{{ loading ? '加载中…' : '暂无数据' }}</p>
      </div>
      <ion-button
        fill="clear"
        size="small"
        class="ctx-close"
        aria-label="关闭"
        @click="$emit('close')"
      >
        <ion-icon :icon="closeIcon" />
      </ion-button>
    </header>

    <!-- ─── ① Usage 进度条 ─────────────────────────── -->
    <section v-if="data && data.usage" class="ctx-section">
      <div class="ctx-usage">
        <div class="ctx-usage-numbers">
          <span class="ctx-usage-pct" :class="getToneClass(data.usage.percent ?? 0)">
            {{ (data.usage.percent ?? 0).toFixed(1) }}%
          </span>
          <span class="ctx-usage-detail">
            {{ formatTokens(data.usage.tokens ?? 0) }} / {{ formatTokens(data.usage.window ?? 0) }} tokens
          </span>
        </div>
        <div class="ctx-usage-bar">
          <div
            class="ctx-usage-fill"
            :class="getToneClass(data.usage.percent ?? 0)"
            :style="{ width: Math.min(100, data.usage.percent ?? 0) + '%' }"
          />
        </div>
        <div v-if="data.compactions > 0" class="ctx-usage-compact">
          <ion-icon :icon="compressIcon" />
          <span>已自动压缩 {{ data.compactions }} 次</span>
        </div>
      </div>
    </section>

    <!-- ─── ② Todos 任务列表 ────────────────────────── -->
    <section v-if="data && Array.isArray(data.todos) && data.todos.length > 0" class="ctx-section">
      <h4 class="ctx-section-title">
        <ion-icon :icon="checkboxIcon" />
        <span>任务列表</span>
        <span class="ctx-section-count">{{ data.todos.length }}</span>
      </h4>
      <ul class="ctx-todos">
        <li
          v-for="(t, i) in data.todos"
          :key="i"
          class="ctx-todo"
          :class="`ctx-todo_${t.status}`"
        >
          <ion-icon
            :icon="todoIcon(t.status)"
            class="ctx-todo-icon"
          />
          <span class="ctx-todo-content">{{ t.content }}</span>
          <span
            v-if="t.status !== 'pending'"
            class="ctx-todo-badge"
            :class="`ctx-todo-badge_${t.status}`"
          >{{ todoStatusLabel(t.status) }}</span>
        </li>
      </ul>
    </section>

    <!-- ─── ③ Referenced Files 引用文件 ─────────────── -->
    <section v-if="data && Array.isArray(data.referencedFiles) && data.referencedFiles.length > 0" class="ctx-section">
      <h4 class="ctx-section-title">
        <ion-icon :icon="documentIcon" />
        <span>引用的文件</span>
        <span class="ctx-section-count">{{ data.referencedFiles.length }}</span>
      </h4>
      <ul class="ctx-refs">
        <li
          v-for="(f, i) in data.referencedFiles"
          :key="`${i}-${f.path}`"
          class="ctx-ref"
        >
          <ion-icon :icon="documentTextIcon" class="ctx-ref-icon" />
          <span class="ctx-ref-path" :title="f.path">{{ f.path }}</span>
          <span class="ctx-ref-via">via {{ f.viaTool }}</span>
        </li>
      </ul>
    </section>

    <!-- 无数据 fallback -->
    <section
      v-else-if="!loading && data"
      class="ctx-section ctx-empty"
    >
      <ion-icon :icon="informationCircleIcon" class="ctx-empty-icon" />
      <p>该 session 还没有任何 tool 调用</p>
    </section>

    <!-- 加载中 -->
    <section v-else-if="loading" class="ctx-section ctx-empty">
      <ion-spinner name="dots" />
      <p>加载上下文使用情况…</p>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { ContextUsageResponse } from "@/composables/useContextUsage";
import { checkmarkCircle as completedIcon, sync as inProgressIcon, ellipsisHorizontal as pendingIcon } from "ionicons/icons";

defineProps<{
  data: ContextUsageResponse | null;
  loading: boolean;
}>();

defineEmits<{ close: [] }>();

// ─── 计算属性 ──────────────────────────────────────────────

function _getToneClass(percent: number): string {
  if (percent >= 90) return "ctx-tone-danger";
  if (percent >= 70) return "ctx-tone-warn";
  return "ctx-tone-ok";
}

// ─── 工具函数 ──────────────────────────────────────────────

function _formatTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + "M";
  if (n >= 1_000) return (n / 1_000).toFixed(1) + "K";
  return String(n);
}

function _todoIcon(status: string) {
  switch (status) {
    case "completed":
      return completedIcon;
    case "in_progress":
      return inProgressIcon;
    default:
      return pendingIcon;
  }
}

function _todoStatusLabel(status: string): string {
  switch (status) {
    case "completed":
      return "已完成";
    case "in_progress":
      return "进行中";
    default:
      return "待办";
  }
}
</script>

<script lang="ts">
// 让 template 可以直接调 getToneClass
</script>

<style scoped>
.ctx-popover {
  width: 100%;
  max-height: inherit;
  overflow-y: auto;
  background: var(--ion-background-color);
  color: var(--ion-text-color);
  font-size: 13px;
}

/* ─── Header ───────────────────────────────── */
.ctx-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 14px 10px;
  border-bottom: 1px solid var(--encv-border-color, rgba(127,127,127,0.2));
}

.ctx-header-icon {
  font-size: 18px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.ctx-header-text {
  flex: 1;
  min-width: 0;
}

.ctx-header-text h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--ion-text-color);
}

.ctx-header-text p {
  margin: 2px 0 0;
  font-size: 11px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.7));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ctx-close {
  --color: var(--encv-text-secondary, rgba(127,127,127,0.7));
  margin: 0;
}

/* ─── Section ──────────────────────────────── */
.ctx-section {
  padding: 10px 14px;
  border-bottom: 1px solid var(--encv-border-color, rgba(127,127,127,0.1));
}

.ctx-section:last-of-type {
  border-bottom: 0;
}

.ctx-section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 0 6px;
  font-size: 11.5px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--encv-text-secondary, rgba(127,127,127,0.7));
}

.ctx-section-title ion-icon {
  font-size: 13px;
}

.ctx-section-count {
  margin-left: auto;
  background: var(--encv-bg-elevated, rgba(127,127,127,0.1));
  color: var(--encv-text-secondary, rgba(127,127,127,0.7));
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 8px;
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}

/* ─── Usage Bar ────────────────────────────── */
.ctx-usage {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ctx-usage-numbers {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.ctx-usage-pct {
  font-size: 22px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.01em;
}

.ctx-usage-pct.ctx-tone-ok    { color: #22c55e; }
.ctx-usage-pct.ctx-tone-warn  { color: #f59e0b; }
.ctx-usage-pct.ctx-tone-danger{ color: #ef4444; }

.ctx-usage-detail {
  font-size: 11.5px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.7));
  font-variant-numeric: tabular-nums;
}

.ctx-usage-bar {
  width: 100%;
  height: 6px;
  background: var(--encv-bg-elevated, rgba(127,127,127,0.15));
  border-radius: 3px;
  overflow: hidden;
}

.ctx-usage-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.4s ease;
}

.ctx-usage-fill.ctx-tone-ok    { background: linear-gradient(90deg, #22c55e, #16a34a); }
.ctx-usage-fill.ctx-tone-warn  { background: linear-gradient(90deg, #eab308, #f59e0b); }
.ctx-usage-fill.ctx-tone-danger{ background: linear-gradient(90deg, #f97316, #ef4444); }

.ctx-usage-compact {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.7));
  margin-top: 2px;
}

.ctx-usage-compact ion-icon {
  font-size: 12px;
}

/* ─── Todos ────────────────────────────────── */
.ctx-todos {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ctx-todo {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  background: var(--encv-bg-elevated, rgba(127,127,127,0.06));
  border-radius: 6px;
  font-size: 12.5px;
}

.ctx-todo-icon {
  font-size: 14px;
  flex-shrink: 0;
}

.ctx-todo_completed .ctx-todo-icon { color: #22c55e; }
.ctx-todo_in_progress .ctx-todo-icon { color: #3b82f6; }
.ctx-todo_pending .ctx-todo-icon { color: var(--encv-text-secondary, rgba(127,127,127,0.6)); }

.ctx-todo_completed .ctx-todo-content {
  text-decoration: line-through;
  opacity: 0.7;
}

.ctx-todo-content {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ctx-todo-badge {
  font-size: 9.5px;
  padding: 1px 5px;
  border-radius: 8px;
  font-weight: 500;
  flex-shrink: 0;
  letter-spacing: 0.02em;
}

.ctx-todo-badge_completed { background: rgba(34, 197, 94, 0.15); color: #16a34a; }
.ctx-todo-badge_in_progress { background: rgba(59, 130, 246, 0.15); color: #2563eb; }

/* ─── Refs ─────────────────────────────────── */
.ctx-refs {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 180px;
  overflow-y: auto;
}

.ctx-ref {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  border-radius: 4px;
  font-size: 11.5px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.ctx-ref:hover {
  background: var(--encv-bg-elevated, rgba(127,127,127,0.06));
}

.ctx-ref-icon {
  font-size: 12px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.6));
  flex-shrink: 0;
}

.ctx-ref-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--ion-text-color);
}

.ctx-ref-via {
  font-size: 10px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.6));
  flex-shrink: 0;
  font-family: inherit;
  font-style: italic;
}

/* ─── Empty ────────────────────────────────── */
.ctx-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 20px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.6));
  text-align: center;
}

.ctx-empty-icon {
  font-size: 24px;
  opacity: 0.5;
}

.ctx-empty p {
  margin: 0;
  font-size: 12px;
}
</style>
