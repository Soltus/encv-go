<!--
  ContextPopoverModal - 上下文详情底部弹出面板（modalController.create 模式）

  替代旧版 ion-popover，解决移动端（尤其是 modal 内）显示不可靠的问题。
  作为 modalController.create() 的 component 使用：
  - 全屏宽度、最大 70vh 高度
  - 从底部滑入（mobile-native feel）
  - 顶部拖拽手柄
  - 背景遮罩，点击关闭
  - 内嵌 ContextPopover 内容组件

  Props 通过 componentProps 传入 reactive state object（per workspace rules §1.2）
-->
<template>
  <ion-page>
    <ion-content :scroll-y="true" class="ctx-modal-content">
      <div class="ctx-modal-body">
        <!-- 拖拽手柄 -->
        <div class="ctx-drag-handle">
          <div class="ctx-drag-handle-bar" />
        </div>

        <!-- ContextPopover 内容 -->
        <ContextPopover
          :data="state.data"
          :loading="state.loading"
          @close="handleClose"
        />
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { modalController } from "@ionic/vue";
import ContextPopover from "@/components/agent/ContextPopover.vue";
import type { ContextUsageResponse } from "@/composables/useContextUsage";

export interface ContextPopoverState {
  data: ContextUsageResponse | null;
  loading: boolean;
}

defineProps<{
  state: ContextPopoverState;
}>();

function handleClose() {
  modalController.dismiss();
}
</script>

<style scoped>
.ctx-modal-content {
  --background: transparent;
  --padding-top: 0;
  --padding-bottom: 0;
  --padding-start: 0;
  --padding-end: 0;
}

.ctx-modal-body {
  max-height: 70vh;
  background: var(--ion-background-color);
  border-radius: 16px 16px 0 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* ── 拖拽手柄 ── */
.ctx-drag-handle {
  display: flex;
  justify-content: center;
  padding: 10px 0 4px;
  flex-shrink: 0;
  cursor: grab;
}

.ctx-drag-handle:active {
  cursor: grabbing;
}

.ctx-drag-handle-bar {
  width: 36px;
  height: 4px;
  border-radius: 2px;
  background: var(--encv-text-secondary, rgba(127,127,127,0.35));
}
</style>
