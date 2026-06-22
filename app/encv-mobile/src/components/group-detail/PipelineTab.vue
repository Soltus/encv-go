<script setup lang="ts">
/**
 * 🆕 2026-06-22 v3：PipelineTree 完全重写（朴素 ul/li 缩进树）
 *
 * 用户的"完全重写"决策：
 *   1. 去掉我之前加的折叠/展开 toggle
 *   2. 去掉 ion-card 包装
 *   3. 去掉 icon-bubble / 渐变色 / 边框装饰
 *   4. 去掉 status badge chip
 *
 * 设计：
 *   - 一棵递归 ul/li 树
 *   - 每行 = 节点名 + 状态文字（紧凑，不抢戏）
 *   - 缩进 16px/层
 *   - 不用 ion-* 组件，只用原生元素
 */
import { computed } from 'vue'
import type { JobRun, StepRun } from '@/lib/workflow/types'

interface Props {
  jobs: JobRun[]
  steps?: Record<string, StepRun[]>  // jobId -> steps（可选；如果 run 走 buildPipelineTree 派生）
}

const props = defineProps<Props>()

interface TreeNode {
  id: string
  label: string
  status: string
  children: TreeNode[]
}

const root = computed<TreeNode[]>(() => {
  // 从 jobs / steps 派生
  return props.jobs.map((job) => {
    const childSteps = (props.steps && props.steps[job.id]) || (job as any).steps || []
    return {
      id: job.id,
      label: job.id,
      status: job.status,
      children: childSteps.map((s: any) => ({
        id: s.id,
        label: s.id,
        status: s.status,
        children: [],
      })),
    }
  })
})

function statusLabel(s: string): string {
  switch (s) {
    case 'completed':
    case 'success': return '✓'
    case 'failed':
    case 'failure': return '✗'
    case 'running': return '…'
    case 'pending':
    case 'queued': return '·'
    case 'cancelling': return '↯'
    case 'cancelled': return '×'
    default: return '·'
  }
}
</script>

<template>
  <ul class="pipeline-tree">
    <PipelineNode v-for="node in root" :key="node.id" :node="node" :status-label="statusLabel" />
  </ul>
</template>

<script lang="ts">
import { defineComponent, h } from 'vue'
// 子节点递归组件（原生 render，避免额外 SFC 文件）
const PipelineNode = defineComponent({
  name: 'PipelineNode',
  props: {
    node: { type: Object as () => any, required: true },
    statusLabel: { type: Function as any, required: true },
  },
  setup(props) {
    return () => {
      const n = props.node
      const sym = props.statusLabel(n.status)
      const row = h('div', { class: 'pt-row' }, [
        h('span', { class: `pt-status pt-status-${n.status || 'pending'}` }, sym),
        h('span', { class: 'pt-label' }, n.label),
      ])
      if (!n.children || n.children.length === 0) {
        return h('li', { class: 'pt-leaf' }, row)
      }
      return h('li', { class: 'pt-branch' }, [
        row,
        h('ul', { class: 'pt-children' },
          n.children.map((c: any) => h(PipelineNode, { node: c, statusLabel: props.statusLabel, key: c.id }))
        ),
      ])
    }
  },
})

export default {
  components: { PipelineNode },
}
</script>

<style scoped>
.pipeline-tree {
  list-style: none;
  margin: 0;
  padding: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  font-size: 13px;
  line-height: 1.5;
  color: #222;
}

.pipeline-tree :deep(.pt-children) {
  list-style: none;
  margin: 0;
  padding-left: 16px;
  border-left: 1px solid #e5e5e5;
}

.pipeline-tree :deep(.pt-row) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
}

.pipeline-tree :deep(.pt-status) {
  display: inline-block;
  width: 14px;
  text-align: center;
  font-size: 12px;
  font-weight: 600;
}

.pipeline-tree :deep(.pt-status-completed),
.pipeline-tree :deep(.pt-status-success) {
  color: #2e7d32;
}
.pipeline-tree :deep(.pt-status-failed),
.pipeline-tree :deep(.pt-status-failure) {
  color: #c62828;
}
.pipeline-tree :deep(.pt-status-running) {
  color: #1565c0;
}
.pipeline-tree :deep(.pt-status-pending),
.pipeline-tree :deep(.pt-status-queued) {
  color: #9e9e9e;
}
.pipeline-tree :deep(.pt-status-cancelling),
.pipeline-tree :deep(.pt-status-cancelled) {
  color: #ef6c00;
}

.pipeline-tree :deep(.pt-label) {
  color: #333;
}
</style>
