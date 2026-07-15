<!--
  ContextCompactionDivider - 不可展开的水平分隔线（Task 7）

  用法：
    <ContextCompactionDivider :text="t('agent.contextCompaction')" />

  设计要点：
    - 中间一行小字 "上下文已自动压缩"，两侧各有 1px 渐变线
    - 不可点击 / 不可展开（区别于 CollapsedMessageToggle）
    - 暗黑模式自适应：line 颜色用 CSS 变量 + 透明度，文字颜色用
      --color-base-content 跟随主题
    - 不使用 ion-icon，避免 Shadow DOM 嵌套在深色模式下的渲染
      不一致问题（参见项目 rules §六）
    - 渲染成 inline-flex 而不是 block，让它与左右消息气泡
      错开时仍能居中
-->
<template>
  <div class="ctx-compaction" role="separator" :aria-label="text">
    <span class="ctx-compaction__line ctx-compaction__line--left" />
    <span class="ctx-compaction__text">{{ text }}</span>
    <span class="ctx-compaction__line ctx-compaction__line--right" />
  </div>
</template>

<script setup lang="ts">
defineProps<{ text: string }>();
</script>

<style scoped>
.ctx-compaction {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 14px 16px;
  /* 不可点击：明确告诉浏览器我们不想接收交互 */
  user-select: none;
  -webkit-user-select: none;
}

.ctx-compaction__line {
  flex: 1 1 auto;
  height: 1px;
  background: linear-gradient(
    to right,
    transparent 0%,
    color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 35%, transparent) 100%
  );
}

.ctx-compaction__line--right {
  background: linear-gradient(
    to left,
    transparent 0%,
    color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 35%, transparent) 100%
  );
}

.ctx-compaction__text {
  flex: 0 0 auto;
  font-size: 12px;
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
  letter-spacing: 0.4px;
  white-space: nowrap;
  /* 与文字的同色系半透明背景，让分隔线在视觉上不"飘" */
  padding: 2px 8px;
  border-radius: 10px;
  background: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 6%, transparent);
}
</style>
