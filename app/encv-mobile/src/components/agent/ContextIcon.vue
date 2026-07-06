<!--
  ContextIcon - 顶部 header 内的上下文使用图标按钮

  行为：
  - 显示当前会话的上下文使用百分比（X.X%）+ 压缩次数徽章
  - 点击 → 通过 modalController.create() 弹出底部面板（ContextPopoverModal）
  - 高负载时（>=80%）图标 + 文字变橙红色，提示接近上限

  设计要点：
  - 紧凑：button 内只放 icon + 百分比 + 可选徽章
  - 暗黑/亮色模式兼容：颜色全部走 CSS 变量
  - disabled 状态：拉取失败/未就绪时显示「—」占位

  架构（per workspace rules §1.1）：
  - 使用 modalController.create() 替代 ion-popover
  - 底部滑入式面板，全宽 + max 70vh
  - 通过 reactive state object 传递数据（per §1.2）
-->
<template>
  <ion-button
    fill="clear"
    size="small"
    class="context-icon-btn"
    :class="[
      toneClass,
      { 'context-icon-btn_compact': compact },
    ]"
    :aria-label="ariaLabel"
    @click="openPopover"
  >
    <ion-icon :icon="layersIcon" slot="start" class="context-icon-svg" />
    <span v-if="!compact" class="context-icon-text">{{ percentText }}</span>
    <ion-badge
      v-if="compactions > 0"
      class="context-icon-badge"
      :color="compactions >= 3 ? 'warning' : 'medium'"
    >×{{ compactions }}</ion-badge>
  </ion-button>
</template>

<script setup lang="ts">
import type { ContextUsageResponse } from "@/composables/useContextUsage";
import { modalController } from "@ionic/vue";
import { computed, reactive } from "vue";
import type { ContextPopoverState } from "./ContextPopoverModal.vue";
import ContextPopoverModal from "./ContextPopoverModal.vue";

const props = defineProps<{
  /** null = 尚未拉到数据；非 null = 已就绪 */
  data: ContextUsageResponse | null;
  loading?: boolean;
  /** 紧凑模式：只显示图标，不显示百分比 */
  compact?: boolean;
}>();

const _ariaLabel = computed(() => {
  if (!props.data) return "上下文使用（加载中）";
  return `上下文使用 ${props.data.usage.percent.toFixed(1)}%`;
});

const _percentText = computed(() => {
  if (!props.data) return "—";
  return props.data.usage.percent.toFixed(1) + "%";
});

const _compactions = computed(() => props.data?.compactions ?? 0);

const _toneClass = computed(() => {
  if (!props.data) return "tone-idle";
  const p = props.data.usage.percent;
  if (p >= 90) return "tone-danger";
  if (p >= 70) return "tone-warn";
  return "tone-ok";
});

/**
 * 打开底部弹出面板（modalController.create 模式）
 * per workspace rules §1.1 + §1.2: 使用 reactive state object 传递数据
 */
async function _openPopover() {
  const state: ContextPopoverState = reactive({
    data: props.data,
    loading: props.loading ?? false,
  });

  const modal = await modalController.create({
    component: ContextPopoverModal,
    componentProps: { state },
    cssClass: "context-popover-modal",
    backdropDismiss: true,
    showBackdrop: true,
  });
  await modal.present();
}
</script>

<style scoped>
.context-icon-btn {
  --padding-start: 6px;
  --padding-end: 6px;
  --color: var(--ion-color-primary);
  font-size: 11.5px;
  position: relative;
}

.context-icon-svg {
  font-size: 14px;
  margin-inline-end: 2px;
}

.context-icon-text {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}

.context-icon-badge {
  font-size: 9px;
  margin-inline-start: 4px;
  padding: 1px 4px;
  min-width: 16px;
}

/* tone: ok（< 70%） */
.tone-ok {
  --color: var(--ion-color-primary);
}

/* tone: warn（70% - 90%） */
.tone-warn {
  --color: #f59e0b;
}

/* tone: danger（>= 90%） */
.tone-danger {
  --color: #ef4444;
}

.tone-idle {
  --color: var(--encv-text-secondary);
}

/* 紧凑模式：去掉文字 */
.context-icon-btn_compact .context-icon-text {
  display: none;
}

.context-icon-btn_compact .context-icon-svg {
  font-size: 16px;
  margin-inline-end: 0;
}
</style>
