<!--
  TaskVirtualList - 任务列表虚拟滚动容器
  用 @tanstack/vue-virtual 渲染异构列表（group / sub_section_header / task 三种 kind）

  设计要点：
  - 父级（Tasks.vue）提供 scrollEl = ion-content 的 .inner-scroll 元素
  - 虚拟列表接管渲染：仅渲染可见窗口 + overscan 个 item，DOM 节点数恒定
  - measureElement 自动测量动态高度（group card / sub_section_header / task card 高度不同；
    task card 展开警告详情时高度也会变化）
  - useAnimationFrameWithResizeObserver: true 让 RO 回调在 rAF 中执行，
    避免 RO 与 Vue patch 调度竞争导致测量时序不确定
  - 暴露 forceMeasure() 给父级兜底（ion-content .inner-scroll 异步 ready 时父级主动调一次）

  ⚠️ 2026-06-18 v3 修复：移除 content-visibility: auto + contain-intrinsic-size
    根因：content-visibility 让浏览器对不在视口的元素跳过渲染，用 80px 占位。
    ResizeObserver 测量时读到 80px 占位值（而非实际高度），写入 itemSizeCache。
    后续 measureElement 同步路径发现缓存有值就直接返回 80px，不再读 DOM → 缓存中毒自我强化。
    getTotalSize() = Σ(被污染的 80px) → 容器高度错误 → calculateRange() 计算错误窗口 → 白屏。
    "空白高度不固定"是因为 group/sub_section/task 高度差异大，与 80px 占位的差值正负不一。

  参考：VirtualLogList.vue（固定行高，不带 measureElement）；
        DevLogs.vue ensureScrollEl()（ion-content shadowRoot .inner-scroll 获取模式）
-->
<template>
  <div
    class="task-virtual-list"
    :style="{ height: `${totalSize}px`, position: 'relative', width: '100%' }"
  >
    <div
      v-for="vItem in virtualItems"
      :key="getKey(vItem.index)"
      :ref="setItemRef"
      :data-index="vItem.index"
      :style="{
        position: 'absolute',
        top: 0,
        left: 0,
        width: '100%',
        transform: `translateY(${vItem.start}px)`,
      }"
      class="task-virtual-item"
    >
      <slot :item="getItem(vItem.index)" :index="vItem.index" />
    </div>
  </div>
</template>

<script setup lang="ts" generic="T">
import { computed, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'

/**
 * 🆕 2026-06-23 Task 8：重构为 count + getItem 接口
 *
 * 旧设计：接收 items 数组（O(N) 遍历）
 *   - 父级构建 displayedItems computed 时遍历整个 store（1000+ task）
 *   - 切换 viewMode/sortBy 时 computed 重新遍历 → 卡顿
 *
 * 新设计：接收 count + getItem(index) + getKey(index)
 *   - virtualizer 只调 getItem 获取可见窗口的 ~20 个 item
 *   - 切换 viewMode 时只更新 count + getItem 引用
 *   - virtualizer 复用 measure cache（key 稳定）
 *   - 消除 O(N) 遍历，主线程不卡顿
 */
interface Props {
  /** 显示项总数（virtualizer 用此值计算滚动范围） */
  count: number
  /** 按 index 获取 item（virtualizer 只对可见窗口调用，O(1)） */
  getItem: (index: number) => T
  /** 按 index 获取 key（用于 virtualizer measure cache 复用，key 稳定） */
  getKey: (index: number) => string | number
  /** 滚动容器（ion-content 的 .inner-scroll） */
  scrollEl: HTMLElement | null
  /** 单条 item 估计高度（px），measureElement 会用实际高度覆盖 */
  estimateSize?: number
  /** 视口外额外预渲染条数（上下各 overscan 条）— 加大可减少快速滚动白屏 */
  overscan?: number
}

const props = withDefaults(defineProps<Props>(), {
  // v3：从 80 改为 120（接近 task card 实际高度，减少首次渲染 totalSize 偏差）
  estimateSize: 120,
  // v3：从 20 降到 10（20 对异构列表过大，增加 RO 测量负担；10 足以覆盖快速滚动）
  overscan: 10,
})

const virtualizerOptions = computed(() => ({
  count: props.count,
  getScrollElement: () => props.scrollEl,
  estimateSize: () => props.estimateSize,
  overscan: props.overscan,
  // v3：启用 rAF 调度 RO 回调，避免与 Vue patch 竞争导致测量时序不确定
  useAnimationFrameWithResizeObserver: true,
}))

const virtualizer = useVirtualizer(virtualizerOptions)

const virtualItems = computed(() => virtualizer.value.getVirtualItems())
const totalSize = computed(() => virtualizer.value.getTotalSize())

/**
 * measureElement ref callback — 交给 virtualizer 自动测量每个 item 的实际高度。
 * virtualizer 内部用 ResizeObserver 监听元素尺寸变化（如 task card 展开警告详情），
 * 读 data-index 属性把测量结果关联到对应 index。
 */
function setItemRef(el: Element | unknown | null): void {
  if (el instanceof HTMLElement) {
    virtualizer.value.measureElement(el)
  } else {
    virtualizer.value.measureElement(null)
  }
}

// 修复：scrollEl 首次为 null 时 virtualizer 返回空 items → 列表全空白
watch(
  () => props.scrollEl,
  (newEl, oldEl) => {
    if (!oldEl && newEl) {
      virtualizer.value.measure()
    }
  },
)

/**
 * 暴露给父级 Tasks.vue 用 — 当 .inner-scroll 异步 ready 时父级主动调一次
 */
function forceMeasure(): void {
  virtualizer.value.measure()
}

defineExpose({ forceMeasure })
</script>

<style scoped>
.task-virtual-list {
  display: block;
  /* contain: layout paint 让虚拟列表容器成为包含上下文，
     避免内部绝对定位 item 影响外部布局 */
  contain: layout paint;
}

/* v3：移除 content-visibility: auto + contain-intrinsic-size
   原因：与 measureElement 动态高度测量冲突，导致 ResizeObserver 测到占位高度，
   污染 itemSizeCache，引发 getTotalSize() / calculateRange() 全链路错误 → 白屏。
   虚拟滚动本身已只渲染可见 + overscan 个 item，不需要浏览器再跳过渲染。 */
.task-virtual-item {
  /* 无 content-visibility — 让 measureElement 拿到准确的 offsetHeight */
}
</style>
