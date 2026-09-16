<!--
  MessageVirtualList - 虚拟滚动消息列表
  封装 vue-virtual-scroller 的 RecycleScroller，用于长会话（>120 条）性能优化
  - 接收 RenderedItem[]，按类型估算 itemSize
  - keyField=messageId 与 renderTurnItems 对齐
  - 暴露 scrollToBottom(behavior) 方法供 AgentChat 调用
  - item slot 留给父组件填充分发逻辑（UserMessageBubble / AssistantMessage 等）
-->
<template>
  <div class="messageVirtualList" ref="containerRef">
    <component
      :is="RecycleScroller"
      ref="scrollerRef"
      class="virtualScroller"
      :items="items"
      :item-size="getItemSize"
      :min-item-size="minItemSize"
      :buffer="buffer"
      key-field="messageId"
      @scroll="onScroll"
    >
      <template #default="{ item, index }">
        <div
          class="virtualItem"
          :data-index="index"
          :data-type="(item as RenderedItem).type"
        >
          <slot name="item" :item="item" :index="index" />
        </div>
      </template>
      <template #empty>
        <slot name="empty" />
      </template>
    </component>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import type { RecycleScrollerExposed } from "vue-virtual-scroller";
import { RecycleScroller } from "vue-virtual-scroller";
import "vue-virtual-scroller/dist/vue-virtual-scroller.css";
import type { RenderedItem } from "@/composables/renderTurnItems";

const props = withDefaults(
  defineProps<{
    items: RenderedItem[];
    /** 最小行高（默认 80px） */
    minItemSize?: number;
    /** 预渲染缓冲区（像素），默认 400 */
    buffer?: number;
  }>(),
  {
    minItemSize: 80,
    buffer: 400,
  }
);

const scrollerRef = ref<RecycleScrollerExposed | null>(null);
const containerRef = ref<HTMLDivElement | null>(null);

/**
 * 按 item type 估算高度
 * RecycleScroller 是固定行高模式（itemSize），不能完全精确但对滚动位置影响小
 * - user / error: 短气泡 80px
 * - assistantText / reasoning: 中等 120px
 * - approval: 表单卡 160px
 * - operationGroup: 列表卡 200px
 * - webSearchGroup: 搜索卡 140px
 */
function getItemSize(item: RenderedItem): number {
  switch (item.type) {
    case "user":
      return 80;
    case "assistantText":
      return 120;
    case "reasoning":
      return 100;
    case "error":
      return 80;
    case "approval":
      return 160;
    case "operationGroup":
      return 200;
    case "webSearchGroup":
      return 140;
    default:
      return 120;
  }
}

/**
 * 滚到列表底部
 * @param behavior 'auto' | 'smooth'
 */
function scrollToBottom(behavior: "auto" | "smooth" = "smooth") {
  if (
    scrollerRef.value &&
    typeof (scrollerRef.value as unknown as { scrollToItem?: (n: number, b?: ScrollBehavior) => void }).scrollToItem === "function"
  ) {
    (scrollerRef.value as unknown as { scrollToItem: (n: number, b?: ScrollBehavior) => void }).scrollToItem(
      props.items.length - 1,
      behavior
    );
  } else if (containerRef.value) {
    // 降级：直接滚到 container 底部
    const el = containerRef.value;
    el.scrollTo({ top: el.scrollHeight, behavior });
  }
}

function onScroll(_e: Event) {
  // 子组件可监听 scroll 事件以实现"是否接近底部"判断
}

defineExpose({ scrollToBottom });
</script>

<style scoped>
.messageVirtualList {
  position: relative;
  flex: 1;
  min-height: 0;
  height: 100%;
  width: 100%;
}

.virtualScroller {
  height: 100%;
  width: 100%;
}

.virtualItem {
  padding: 0;
  box-sizing: border-box;
}
</style>
