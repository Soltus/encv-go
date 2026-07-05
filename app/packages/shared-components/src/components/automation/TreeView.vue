<template>
  <div class="tree">
    <!-- 搜索/过滤 + 工具栏扩展 -->
    <div v-if="searchable" class="tree__toolbar">
      <input
        v-model="searchQuery"
        type="text"
        placeholder="Filter steps..."
        class="tree__search"
      />
      <span class="tree__count">{{ filteredNodes.length }} nodes</span>
      <slot name="toolbar" />
    </div>

    <!-- 树形列表 -->
    <div class="tree__list">
      <div
        v-for="node in filteredNodes"
        :key="node.id"
        class="tree__node-wrap"
      >
        <!-- 父节点（job / 分组节点） -->
        <button
          :class="[
            'tree__node',
            'tree__node--parent',
            `tree__node--${node.status}`,
            { 'tree__node--expanded': expandedSet.has(node.id) },
          ]"
          @click="toggleNode(node)"
        >
          <ion-icon
            :icon="expandedSet.has(node.id) ? chevronDown : chevronForward"
            class="tree__arrow"
          />
          <slot name="node-icon" :node="node">
            <PhaseIcon v-if="node.phase" :phase="node.phase" />
            <StepMiniBadge v-else :status="node.status" :show-name="false" />
          </slot>
          <span class="tree__label">{{ node.label }}</span>
          <slot name="node-meta" :node="node">
            <span v-if="node.meta" class="tree__meta">{{ node.meta }}</span>
          </slot>
          <ion-icon
            v-if="node.errorHint"
            :icon="closeCircleOutline"
            class="tree__error-hint-icon"
          />
          <!-- 进度 / 速率 / 耗时（字段缺失时隐藏） -->
          <span v-if="node.progress != null" class="tree__progress">{{ node.progress }}%</span>
          <span v-if="node.duration" class="tree__duration">{{ node.duration }}</span>
          <span v-if="node.speed" class="tree__speed">
            <ion-icon :icon="flashOutline" class="tree__speed-icon" />{{ node.speed }}
          </span>
        </button>

        <!-- 子节点 -->
        <div
          v-if="expandedSet.has(node.id) && node.children?.length"
          class="tree__children"
        >
          <div
            v-for="child in node.children"
            :key="child.id"
            class="tree__child-wrap"
          >
            <button
              :class="[
                'tree__node',
                'tree__node--child',
                `tree__node--${child.status}`,
                { 'tree__node--selected': expandedDetailSet.has(child.id) },
              ]"
              @click="selectNode(child)"
            >
              <span class="tree__indent"></span>
              <slot name="node-icon" :node="child">
                <PhaseIcon v-if="child.phase" :phase="child.phase" />
                <StepMiniBadge v-else :status="child.status" :show-name="false" />
              </slot>
              <span class="tree__label">{{ child.label }}</span>
              <slot name="node-meta" :node="child">
                <span v-if="child.meta" class="tree__meta">{{ child.meta }}</span>
              </slot>
              <ion-icon
                v-if="child.errorHint"
                :icon="closeCircleOutline"
                class="tree__error-hint-icon"
              />
              <span v-if="child.progress != null" class="tree__progress">{{ child.progress }}%</span>
              <span v-if="child.duration" class="tree__duration">{{ child.duration }}</span>
              <span v-if="child.speed" class="tree__speed">
                <ion-icon :icon="flashOutline" class="tree__speed-icon" />{{ child.speed }}
              </span>
            </button>
            <!-- 子节点展开详情（slot-based） -->
            <div
              v-if="expandedDetailSet.has(child.id)"
              class="tree__child-detail"
            >
              <slot name="node-detail" :node="child" />
            </div>
          </div>
        </div>
      </div>

      <!-- 空状态 -->
      <div v-if="filteredNodes.length === 0" class="tree__empty">
        No nodes found
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { isPhase, type StepStatus, type UnifiedTreeNode, type WorkflowRun } from "@encv/shared-components/lib/workflow/types";
import { computed, ref, watch } from "vue";

/**
 * 通用树形视图组件（UnifiedTreeView）
 *
 * 设计目标：解耦 TreeView 与特定业务字段（stepName / jobDisplayName 等），
 * 接收 UnifiedTreeNode[] 作为通用数据契约，由调用方负责从 StepRun / JobRun
 * 等领域模型转换为本接口。
 *
 * 兼容模式：当传入 workflowRun + stepNames + jobDisplayNames 时，内部自动
 * 派生为 UnifiedTreeNode[]（保留对 PluginTestsDetail.vue 旧调用方的兼容）。
 */
const props = withDefaults(
  defineProps<{
    /** 通用树节点数组（推荐用法） */
    nodes?: UnifiedTreeNode[];
    /** 兼容字段：从 WorkflowRun 派生 nodes（当 nodes 未传时启用） */
    workflowRun?: WorkflowRun;
    /** 兼容字段：stepDefId → 显示名映射 */
    stepNames?: Map<string, string>;
    /** 兼容字段：jobDefId → 显示名映射 */
    jobDisplayNames?: Map<string, string>;
    /** 是否显示搜索框（默认 true） */
    searchable?: boolean;
    /** 默认展开有失败子节点的父节点（默认 true） */
    defaultExpandFailed?: boolean;
  }>(),
  {
    nodes: () => [],
    searchable: true,
    defaultExpandFailed: true,
  }
);

const emit = defineEmits<{
  (e: "select-node", node: UnifiedTreeNode): void;
  (e: "toggle-node", node: UnifiedTreeNode, expanded: boolean): void;
}>();

const searchQuery = ref("");
/** 跟踪展开的父节点 */
const expandedSet = ref<Set<string>>(new Set());
/** 跟踪展开详情的子节点 */
const expandedDetailSet = ref<Set<string>>(new Set());

// ==================== 兼容派生：WorkflowRun → UnifiedTreeNode[] ====================

/** 把毫秒耗时格式化为人类可读字符串 */
function formatDurationMs(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  const m = Math.floor(ms / 60000);
  const s = Math.floor((ms % 60000) / 1000);
  return `${m}m${s}s`;
}

/** 终态 step 状态集合（用于计算 job 完成数） */
const TERMINAL_STEP_STATUS: Set<StepStatus> = new Set(["success", "failure", "cancelled", "skipped", "timed_out"]);

/** 从 WorkflowRun 派生 UnifiedTreeNode[]（兼容旧调用方） */
function deriveNodesFromWorkflowRun(
  run: WorkflowRun,
  stepNames?: Map<string, string>,
  jobDisplayNames?: Map<string, string>
): UnifiedTreeNode[] {
  return run.jobs.map(job => {
    const completedCount = job.steps.filter(s => TERMINAL_STEP_STATUS.has(s.status)).length;
    const meta = `${completedCount}/${job.steps.length}` + (job.conclusion ? ` · ${job.conclusion}` : "");
    return {
      id: job.id,
      label: jobDisplayNames?.get(job.jobDefId) ?? job.jobDefId,
      status: job.status,
      meta,
      children: job.steps.map(step => ({
        id: step.id,
        label: stepNames?.get(step.stepDefId) ?? step.stepDefId,
        status: step.status,
        progress: step.progress,
        phase: isPhase(step.phase) ? step.phase : undefined,
        speed: step.speed,
        eta: step.eta,
        duration: step.durationMs != null ? formatDurationMs(step.durationMs) : undefined,
        errorHint: step.error ? "error" : undefined,
      })),
    };
  });
}

/** 实际渲染用的节点数组：优先用 nodes prop，否则从 workflowRun 派生 */
const resolvedNodes = computed<UnifiedTreeNode[]>(() => {
  if (props.nodes && props.nodes.length > 0) return props.nodes;
  if (props.workflowRun) {
    return deriveNodesFromWorkflowRun(props.workflowRun, props.stepNames, props.jobDisplayNames);
  }
  return [];
});

// ==================== 默认展开策略 ====================

let initialized = false;
/** 默认展开有失败子节点的父节点 */
function initExpanded(nodes: UnifiedTreeNode[]) {
  if (!props.defaultExpandFailed) return;
  const set = new Set<string>();
  for (const node of nodes) {
    if (node.children?.some(c => c.status === "failure" || c.status === "timed_out")) {
      set.add(node.id);
    }
  }
  expandedSet.value = set;
}

// 监听 resolvedNodes：首次拿到非空数据时初始化展开状态
watch(
  resolvedNodes,
  nodes => {
    if (!initialized && nodes.length > 0) {
      initExpanded(nodes);
      initialized = true;
    }
  },
  { immediate: true }
);

// ==================== 搜索过滤 ====================

const _filteredNodes = computed(() => {
  if (!searchQuery.value) return resolvedNodes.value;
  const q = searchQuery.value.toLowerCase();
  return resolvedNodes.value.filter(node => {
    // 父节点 label / meta 匹配
    if (node.label.toLowerCase().includes(q)) return true;
    if (node.meta?.toLowerCase().includes(q)) return true;
    // 子节点 label / meta 匹配
    return node.children?.some(c => c.label.toLowerCase().includes(q) || c.meta?.toLowerCase().includes(q));
  });
});

// ==================== 交互 ====================

/** 切换父节点展开状态 */
function _toggleNode(node: UnifiedTreeNode) {
  const next = new Set(expandedSet.value);
  const wasExpanded = next.has(node.id);
  if (wasExpanded) next.delete(node.id);
  else next.add(node.id);
  expandedSet.value = next;
  emit("toggle-node", node, !wasExpanded);
}

/** 子节点点击：emit select-node + 切换详情展开 */
function _selectNode(node: UnifiedTreeNode) {
  const next = new Set(expandedDetailSet.value);
  if (next.has(node.id)) next.delete(node.id);
  else next.add(node.id);
  expandedDetailSet.value = next;
  emit("select-node", node);
}
</script>

<style scoped>
.tree {
  margin: 0 16px 12px;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
}

/* ==================== 工具栏 ==================== */
.tree__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 0;
}

.tree__search {
  flex: 1;
  padding: 6px 10px;
  border: 1px solid #D4C9B5;
  border-radius: 3px;
  font-family: inherit;
  font-size: 12px;
  background: #FAF6EE;
  color: #1A1A1A;
}
.tree__search::placeholder { color: #A09580; }
.tree__search:focus {
  outline: none;
  border-color: #2B3A67;
}

.tree__count {
  font-size: 10px;
  color: #6B5D4C;
  white-space: nowrap;
}

/* ==================== 树形列表 ==================== */
.tree__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.tree__node-wrap {
  border: 1px solid #EDE5D2;
  border-radius: 3px;
  overflow: hidden;
}

/* 节点 */
.tree__node {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 7px 10px;
  background: none;
  border: none;
  text-align: left;
  cursor: pointer;
  font-family: inherit;
  font-size: 12px;
  color: #1A1A1A;
  transition: background 0.1s ease;
}
.tree__node:hover { background: rgba(43, 58, 103, 0.04); }

.tree__node--parent {
  background: #FAF6EE;
  font-weight: 600;
  border-bottom: 1px solid #EDE5D2;
}
.tree__node--expanded.tree__node--parent {
  border-bottom-color: transparent;
}

.tree__node--child {
  padding-left: 24px;
  font-weight: 400;
  font-size: 11px;
}
.tree__node--child:hover { background: rgba(43, 58, 103, 0.06); }
.tree__node--selected {
  background: rgba(43, 58, 103, 0.1);
  border-left: 2px solid #2B3A67;
}

/* 状态色（左边框微弱提示） */
.tree__node--failure { border-left: 2px solid rgba(136, 14, 79, 0.4); }
.tree__node--success { border-left: 2px solid rgba(27, 94, 32, 0.3); }
.tree__node--running { border-left: 2px solid rgba(21, 101, 192, 0.4); }
.tree__node--timed_out { border-left: 2px solid rgba(230, 81, 0, 0.4); }

.tree__arrow {
  font-size: 12px;
  color: #8B7355;
  flex-shrink: 0;
}
.tree__indent {
  width: 14px;
  flex-shrink: 0;
}

.tree__label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tree__meta {
  font-size: 9px;
  color: #8B7355;
  flex-shrink: 0;
}

.tree__error-hint {
  color: #8B1E3F;
  font-size: 11px;
  flex-shrink: 0;
}

.tree__error-hint-icon {
  font-size: 12px;
  color: #8B1E3F;
  flex-shrink: 0;
}

.tree__progress {
  font-size: 10px;
  color: #1565C0;
  font-weight: 600;
  flex-shrink: 0;
}

.tree__duration {
  font-size: 10px;
  color: #6B5D4C;
  flex-shrink: 0;
}

.tree__speed {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 10px;
  color: #1565C0;
  flex-shrink: 0;
}

.tree__speed-icon {
  font-size: 11px;
  flex-shrink: 0;
}

/* 子节点容器 */
.tree__children {
  display: flex;
  flex-direction: column;
}

/* 子节点展开详情 */
.tree__child-detail {
  padding: 8px 12px 12px 28px;
  background: rgba(43, 58, 103, 0.02);
  border-top: 1px dashed #EDE5D2;
  animation: tree-detail-enter 0.2s ease;
}

@keyframes tree-detail-enter {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.tree__empty {
  padding: 20px;
  text-align: center;
  color: #8B7355;
  font-size: 13px;
}

/* ==================== 暗黑模式适配（body.dark）—— 保留档案主题，非纯黑 ==================== */
:global(body.dark) .tree {
  color: #E0E0E0;
}

:global(body.dark) .tree__search {
  background: #1A1D21;
  border-color: rgba(255, 255, 255, 0.10);
  color: #E0E0E0;
}
:global(body.dark) .tree__search::placeholder {
  color: rgba(255, 255, 255, 0.4);
}
:global(body.dark) .tree__search:focus {
  border-color: var(--ion-color-primary, #4f8cff);
}

:global(body.dark) .tree__count {
  color: #8B95A5;
}

:global(body.dark) .tree__node-wrap {
  border-color: rgba(255, 255, 255, 0.08);
}

:global(body.dark) .tree__node {
  color: #E0E0E0;
}
:global(body.dark) .tree__node:hover {
  background: rgba(255, 255, 255, 0.04);
}

:global(body.dark) .tree__node--parent {
  background: #1A1D21;
  border-bottom-color: rgba(255, 255, 255, 0.08);
}

:global(body.dark) .tree__node--child:hover {
  background: rgba(255, 255, 255, 0.06);
}
:global(body.dark) .tree__node--selected {
  background: rgba(var(--ion-color-primary-rgb, 79, 140, 255), 0.15);
  border-left-color: var(--ion-color-primary, #4f8cff);
}

:global(body.dark) .tree__arrow {
  color: #8B95A5;
}

:global(body.dark) .tree__meta {
  color: #8B95A5;
}

:global(body.dark) .tree__error-hint-icon {
  color: var(--tl-state-failed);
}

:global(body.dark) .tree__progress {
  color: var(--ion-color-primary, #4f8cff);
}

:global(body.dark) .tree__duration {
  color: #8B95A5;
}

:global(body.dark) .tree__speed {
  color: var(--ion-color-primary, #4f8cff);
}

:global(body.dark) .tree__child-detail {
  background: rgba(255, 255, 255, 0.02);
  border-top-color: rgba(255, 255, 255, 0.08);
}

:global(body.dark) .tree__empty {
  color: #8B95A5;
}

/* 暗黑模式下状态色边框 */
:global(body.dark) .tree__node--failure {
  border-left-color: rgba(var(--tl-state-failed-rgb), 0.5);
}
:global(body.dark) .tree__node--success {
  border-left-color: rgba(var(--tl-state-completed-rgb), 0.4);
}
:global(body.dark) .tree__node--running {
  border-left-color: rgba(var(--tl-state-analyzing-rgb), 0.5);
}
:global(body.dark) .tree__node--timed_out {
  border-left-color: rgba(var(--tl-state-preprocessing-rgb), 0.5);
}
</style>
