<!--
  TaskVirtualList - 任务列表虚拟滚动容器
  用 @tanstack/vue-virtual 渲染异构列表（group / sub_section_header / task 三种 kind）

  设计要点：
  - 父级（Tasks.vue）提供 scrollEl = ion-content 的 .inner-scroll 元素
  - 虚拟列表接管渲染：仅渲染可见窗口 + overscan 个 item，DOM 节点数恒定
  - measureElement 自动测量动态高度（group card / sub_section_header / task card 高度不同；
    task card 展开警告详情时高度也会变化）
  - content-visibility: auto + contain-intrinsic-size 白屏优化（快速滚动时浏览器跳过
    不可见内容的渲染，提供占位高度避免滚动条抖动）
  - 暴露 forceMeasure() 给父级兜底（ion-content .inner-scroll 异步 ready 时父级主动调一次）

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
  estimateSize: 80,
  overscan: 20,
  getKey: (item: T) => item.key,
})

const virtualizerOptions = computed(() => ({
  count: props.items.length,
  getScrollElement: () => props.scrollEl,
  estimateSize: () => props.estimateSize,
  overscan: props.overscan,
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

.task-virtual-item {
  /* 白屏优化：content-visibility: auto 让浏览器跳过不可见 item 的渲染，
     contain-intrinsic-size 提供占位高度避免滚动条抖动。
     measureElement 会用实际高度覆盖占位值。 */
  content-visibility: auto;
  contain-intrinsic-size: 80px;
}
</style>
