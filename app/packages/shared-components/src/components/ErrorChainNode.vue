<template>
  <div class="chain">
    <div
      v-for="(step, idx) in chain"
      :key="step.phase"
      class="chain__node"
      :class="`chain__node--${step.severity}`"
    >
      <!-- 阶段连接线 -->
      <div class="chain__rail" :class="{
        'chain__rail--failed': step.severity === 'error',
        'chain__rail--info': step.severity === 'info',
      }">
        <div class="chain__rail-dot">
          <span class="chain__rail-glyph">{{ getGlyph(step, idx) }}</span>
        </div>
        <div v-if="idx !== chain.length - 1" class="chain__rail-line"></div>
      </div>

      <!-- 阶段内容 -->
      <div class="chain__body">
        <div class="chain__header">
          <span class="chain__phase-tag">PHASE {{ idx + 1 }}/{{ chain.length }}</span>
          <span class="chain__title">{{ step.title }}</span>
          <span v-if="step.severity === 'error'" class="chain__status">FAILED</span>
        </div>
        <div class="chain__detail">{{ step.detail }}</div>
        <div v-if="step.severity === 'error'" class="chain__failed-hint">
          <em>— failure originated here —</em>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ErrorChainStep } from "@encv/shared-components/composables/useErrorAnalyzer";

defineProps<{
  chain: ErrorChainStep[];
}>();

function getGlyph(step: ErrorChainStep, _idx: number): string {
  if (step.severity === "error") return "✕";
  if (step.severity === "warning") return "!";
  // info: 前置阶段用 '✓'，未到达阶段用 '○'
  // 通过"detail 是否包含"未到达""判断
  if (step.detail.includes("未到达")) return "○";
  return "✓";
}
</script>

<style scoped>
.chain {
  display: flex;
  flex-direction: column;
  gap: 0;
  font-family: 'Times New Roman', Georgia, serif;
}

.chain__node {
  display: grid;
  grid-template-columns: 36px 1fr;
  gap: 0;
  min-height: 60px;
}

.chain__rail {
  display: flex;
  flex-direction: column;
  align-items: center;
  position: relative;
}

.chain__rail-dot {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: 'Times New Roman', Georgia, serif;
  font-weight: 700;
  font-size: 13px;
  flex-shrink: 0;
  border: 1.5px solid;
  background: #FAF6EE;
  z-index: 1;
}

.chain__rail-line {
  flex: 1;
  width: 1.5px;
  background: #C9BBA1;
  margin-top: -1px;
  margin-bottom: -1px;
  min-height: 16px;
}

.chain__rail--failed .chain__rail-dot {
  background: #8B1E3F;
  border-color: #5B0F1F;
  color: #F4EFE6;
}
.chain__rail--info .chain__rail-dot {
  background: #FAF6EE;
  border-color: #C9BBA1;
  color: #1B4332;
}

.chain__rail--failed .chain__rail-line {
  background: repeating-linear-gradient(
    to bottom,
    #8B1E3F 0,
    #8B1E3F 4px,
    transparent 4px,
    transparent 8px
  );
}

.chain__body {
  padding: 4px 0 14px 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.chain__header {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}

.chain__phase-tag {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  letter-spacing: 0.2em;
  color: #6B5D4C;
  text-transform: uppercase;
}

.chain__title {
  font-size: 15px;
  font-weight: 600;
  color: #1A1A1A;
  letter-spacing: 0.01em;
}

.chain__status {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  letter-spacing: 0.2em;
  color: #F4EFE6;
  background: #8B1E3F;
  padding: 2px 6px;
  border-radius: 2px;
  margin-left: auto;
}

.chain__detail {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  color: #4A3F2E;
  word-break: break-all;
  background: rgba(244, 239, 230, 0.5);
  padding: 6px 8px;
  border-left: 2px solid #C9BBA1;
  border-radius: 0 2px 2px 0;
}

.chain__node--error .chain__detail {
  background: rgba(139, 30, 63, 0.06);
  border-left-color: #8B1E3F;
  color: #2D0815;
}

.chain__failed-hint {
  font-size: 11px;
  color: #8B1E3F;
  font-style: italic;
  margin-top: 2px;
}
</style>
