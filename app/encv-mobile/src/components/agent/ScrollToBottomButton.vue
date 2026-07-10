<!--
  ScrollToBottomButton - 浮动"滚动到底部"按钮
  Task 14：阅读旧消息时显示，点击跳回最新消息；unreadCount > 0 时显示数字 badge
  - 父组件传入 visible / onClick / unreadCount
  - 自身只负责按钮的展示与点击回调，滚动逻辑由父组件实现
  - z-index < 100，不挡浮动 AI 入口（AgentEntry 用 999）
-->
<template>
  <button
    v-if="visible"
    type="button"
    class="scrollToBottomBtn"
    :title="t('agent.scrollToBottom')"
    :aria-label="t('agent.scrollToBottom')"
    @click="onClick"
  >
    <ion-icon :icon="arrowDownIcon" class="scrollToBottomIcon" />
    <span v-if="unreadCount > 0" class="scrollToBottomBadge">{{ unreadCount }}</span>
  </button>
</template>

<script setup lang="ts">
import { arrowDownOutline } from "ionicons/icons";
import { useI18n } from "@/composables/useI18n";

withDefaults(
  defineProps<{
    visible: boolean;
    onClick: () => void;
    unreadCount?: number;
  }>(),
  {
    unreadCount: 0,
  }
);

const { t } = useI18n();
const arrowDownIcon = arrowDownOutline;
</script>

<style scoped>
.scrollToBottomBtn {
  position: absolute;
  right: 16px;
  bottom: 16px;
  /* z-index 留出余量：浮动 AI 入口 999，本按钮 < 100 不挡 */
  z-index: 50;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border: 0;
  border-radius: 50%;
  background: var(--ion-toolbar-background, var(--ion-background-color));
  color: var(--ion-color-primary);
  box-shadow: 0 3px 10px rgba(0, 0, 0, 0.18), 0 1px 3px rgba(0, 0, 0, 0.12);
  cursor: pointer;
  padding: 0;
  transition: transform 0.12s, box-shadow 0.12s;
}

.scrollToBottomBtn:hover {
  transform: scale(1.06);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.22), 0 2px 5px rgba(0, 0, 0, 0.16);
}

.scrollToBottomBtn:active {
  transform: scale(0.94);
}

.scrollToBottomIcon {
  font-size: 20px;
}

.scrollToBottomBadge {
  position: absolute;
  top: -2px;
  right: -2px;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--ion-color-danger, #eb445a);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
  text-align: center;
  box-shadow: 0 0 0 2px var(--ion-toolbar-background, var(--ion-background-color));
}

body.dark .scrollToBottomBtn {
  background: var(--ion-toolbar-background, var(--ion-background-color));
  color: var(--ion-color-primary);
  box-shadow: 0 3px 12px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(var(--ion-color-primary-rgb), 0.3);
}
</style>
