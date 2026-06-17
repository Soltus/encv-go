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
      :key="getKey(items[vItem.index])"
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
      <slot :item="items[vItem.index]" :index="vItem.index" />
    </div>
  </div>
</template>

<script setup lang="ts" generic="T extends { key: string }">
import { computed, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'

interface Props {
  /** 显示项数组（异构：group / sub_section_header / task） */
  items: readonly T[]
  /** 滚动容器（ion-content 的 .inner-scroll） */
  scrollEl: HTMLElement | null
  /** 单条 item 估计高度（px），measureElement 会用实际高度覆盖 */
  estimateSize?: number
  /** 视口外额外预渲染条数（上下各 overscan 条）— 加大可减少快速滚动白屏 */
  overscan?: number
  /** 自定义 key（默认取 item.key） */
  getKey?: (item: T) => string | number
}

const props = withDefaults(defineProps<Props>(), {
  // v3：从 80 改为 120（接近 task card 实际高度，减少首次渲染 totalSize 偏差）
  estimateSize: 120,
  // v3：从 20 降到 10（20 对异构列表过大，增加 RO 测量负担；10 足以覆盖快速滚动）
  overscan: 10,
  getKey: (item: T) => item.key,
})

const virtualizerOptions = computed(() => ({
  count: props.items.length,
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
 *
 * 为什么用函数包装而不是直接 :ref="virtualizer.value.measureElement"：
 *   Vue 的 :ref 绑定函数会在元素 mount/unmount 时调用，传 Element | ComponentPublicInstance | null。
 *   virtualizer.measureElement 已处理 null（卸载时清理 ResizeObserver），
 *   但 Vue ref callback 的类型签名是 (el: Element | ComponentPublicInstance | null) => void，
 *   直接绑 virtualizer 实例方法会丢失 this 上下文（measureElement 内部读 this.options）。
 *   用箭头函数包装保持 this 绑定。
 *
 *   模板内 :ref 只会绑定到原生 HTML 元素（<div>），不会是组件实例，
 *   所以运行时 el 只会是 Element | null；类型签名兼容 Vue 的 VNodeRef 即可。
 */
function setItemRef(el: Element | unknown | null): void {
  if (el instanceof HTMLElement) {
    virtualizer.value.measureElement(el)
  } else {
    // el 为 null（unmount）或非 HTMLElement（理论不会发生）→ 传 null 让 virtualizer 清理
    virtualizer.value.measureElement(null)
  }
}

// 修复：scrollEl 首次为 null 时 virtualizer 返回空 items → 列表全空白
//   根因：Ionic ion-content 的 .inner-scroll 在 shadow DOM 内，onMounted 时可能还没 ready
//   ensureScrollEl() 需要 querySelector shadow root → 首次渲染时 scrollEl=null
//   useVirtualizer 拿到 null getScrollElement → getVirtualItems()=[] → v-for 不渲染
//   修复：watch scrollEl 从 null → 非 null 时触发 measure() 重新计算可见项
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
 * 让 virtualizer 立刻测量 + 渲染首屏 items。
 *
 * 为什么需要这个：watch scrollEl 在 oldEl=null → newEl=non-null 时触发 measure()，
 * 但 Tasks.vue 的 ensureScrollEl 内部 scrollEl.value = el 是同步赋值 → 父级能 watch 到。
 * 然而父级在 onMounted 多次 retry 拿到 el 后，仍可能因为 Vue 调度时序问题 watch 没及时触发
 * （例如 nextTick 之间的竞争）。父级主动调 forceMeasure 兜底。
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
