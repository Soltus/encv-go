<!--
  AgentTaskMessage - 子任务列表（Task 22）

  用法：
    <AgentTaskMessage
      :sub-tasks="item.subTasks"
      :reasoning="item.reasoning"
    />

  渲染要点：
    - 折叠阈值：subTasks 行数 > 7 或全部 description 拼接字符数 > 520
      → 默认折叠（点击展开）
    - 任一阈值未触发 → 默认展开全部（短列表无需折叠）
    - 头部展示"子任务"标题 + 进度 (n/m) + 状态徽章
    - 状态图标：ionicons（checkmarkCircle / sync / ellipsisHorizontal / closeCircle）
    - 与 plan/operationGroup 独立渲染，不与之合并
    - 复用 BlockHeader 的视觉语言（border + 浅色背景 + primary tint）
    - streaming 态：边框高亮 + 等待图标 pulse（与 PlanBlock 一致）
    - codex_web 风格：agent task + 多子任务状态徽章 + 折叠
-->
<template>
  <div class="agent-task ui-panel" :class="{ 'is-streaming': props.streaming }">
    <div class="agentTaskHeader" @click="toggleExpanded">
      <ion-icon :icon="icon" class="agentTaskIcon" />
      <span class="agentTaskTitle">{{ t('agent.agentTask') }}</span>
      <span class="agentTaskProgress">
        {{ t('agent.subTaskProgress', { done: String(completedCount), total: String(props.subTasks.length) }) }}
      </span>
      <StatusBadge
        v-if="failedCount > 0"
        :label="t('agent.failed')"
        tone="warn"
      />
      <StatusBadge
        v-else-if="props.streaming || inProgressCount > 0"
        :label="t('agent.planStatusInProgress')"
        tone="idle"
        pulse
      />
      <StatusBadge
        v-else-if="props.subTasks.length > 0 && completedCount === props.subTasks.length"
        :label="t('agent.completed')"
        tone="ready"
      />
      <ion-icon
        :icon="expanded ? chevronUp : chevronDown"
        class="agentTaskChevron"
      />
    </div>

    <!-- 进度条 -->
    <div v-if="props.subTasks.length > 0" class="agentTaskProgressBar">
      <div class="agentTaskProgressBarTrack">
        <div class="agentTaskProgressBarFill" :style="{ width: progressPct + '%' }" />
      </div>
    </div>

    <!-- reasoning 区域：仅当存在 reasoning 文本时显示（折叠态也保留） -->
    <p v-if="reasoningText" class="agentTaskReasoning">{{ reasoningText }}</p>

    <ul v-if="expanded && props.subTasks.length > 0" class="agentTaskList" data-testid="agent-task-list">
      <li
        v-for="task in props.subTasks"
        :key="task.id"
        class="agentTaskItem"
        :class="`agentTaskItem--${task.status}`"
        :data-testid="`agent-task-item-${task.id}`"
      >
        <span class="agentTaskMarker" aria-hidden="true">
          <ion-icon
            :icon="statusIcon(task.status)"
            :class="`agentTaskMarkerIcon agentTaskMarkerIcon--${task.status}`"
          />
        </span>
        <span class="agentTaskDescription">{{ task.description }}</span>
        <span class="agentTaskStatusLabel">{{ statusLabel(task.status) }}</span>
      </li>
    </ul>
    <p v-else-if="expanded" class="agentTaskEmpty">{{ t('agent.agentTaskEmpty') }}</p>
  </div>
</template>

<script setup lang="ts">
import {
  checkmarkCircle,
  chevronDownOutline,
  chevronUpOutline,
  closeCircle,
  ellipsisHorizontalCircle,
  gitBranchOutline,
  sync,
} from "ionicons/icons";
import { computed, ref } from "vue";
import StatusBadge from "@/components/agent/StatusBadge.vue";
import type { SubTask } from "@/composables/renderTurnItems";
import { AGENT_TASK_COLLAPSE_CHAR_COUNT, AGENT_TASK_COLLAPSE_LINE_COUNT } from "@/composables/renderTurnItems";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const props = withDefaults(
  defineProps<{
    subTasks: SubTask[];
    /** 后端 SubagentDispatch 事件附带的"为什么派发子任务"高层说明 */
    reasoning?: string;
    /** 是否正在流式追加（后端持续推送 subagent 进度时为 true） */
    streaming?: boolean;
  }>(),
  {
    reasoning: undefined,
    streaming: false,
  }
);

const { t } = useI18n();

const icon = gitBranchOutline;
const chevronUp = chevronUpOutline;
const chevronDown = chevronDownOutline;

const shouldCollapse = computed(() => {
  if (props.subTasks.length > AGENT_TASK_COLLAPSE_LINE_COUNT) return true;
  const totalChars = props.subTasks.reduce((acc, t) => acc + (t.description?.length ?? 0), 0);
  return totalChars > AGENT_TASK_COLLAPSE_CHAR_COUNT;
});

// streaming 态默认展开（让用户看到"任务正在跑"的进度变化），
// 非 streaming 态按 shouldCollapse 决定初值。
const expanded = ref<boolean>(props.streaming || !shouldCollapse.value);

function toggleExpanded() {
  expanded.value = !expanded.value;
}

const completedCount = computed(() => props.subTasks.filter(s => s.status === "completed").length);
const inProgressCount = computed(() => props.subTasks.filter(s => s.status === "in_progress").length);
const failedCount = computed(() => props.subTasks.filter(s => s.status === "failed").length);
const progressPct = computed(() => {
  const total = props.subTasks.length;
  if (total === 0) return 0;
  return Math.round((completedCount.value / total) * 100);
});

const reasoningText = computed(() => {
  const r = props.reasoning;
  return typeof r === "string" && r.trim().length > 0 ? r.trim() : "";
});

function statusIcon(status: SubTask["status"]) {
  switch (status) {
    case "completed":
      return checkmarkCircle;
    case "in_progress":
      return sync;
    case "failed":
      return closeCircle;
    default:
      return ellipsisHorizontalCircle;
  }
}

function statusLabel(status: SubTask["status"]): string {
  if (status === "in_progress") return t("agent.planStatusInProgress");
  if (status === "completed") return t("agent.planStatusCompleted");
  if (status === "failed") return t("agent.failed");
  if (status === "pending") return t("agent.planStatusPending");
  return status;
}
</script>

<style scoped>
/* 表面（背景/边框/圆角/内距）已上提到全局 .ui-panel，供用户主题以 .ui-panel{} 覆写。
   本 scoped 仅保留结构/交互差异；is-streaming 的状态覆写仍在此（scoped
   特异性 0,2,0 高于 .ui-panel 的 0,1,0，故 border-color/box-shadow 生效）。 */
.agent-task {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.agent-task.is-streaming {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-primary) 85%, var(--color-white));
}

.agentTaskHeader {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  user-select: none;
  min-width: 0;
}

.agentTaskIcon {
  font-size: 16px;
  color: var(--color-primary);
  flex-shrink: 0;
}

.agentTaskTitle {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-text-color);
  flex-shrink: 0;
}

.agentTaskProgress {
  flex: 1 1 auto;
  font-size: 11px;
  color: color-mix(in srgb, var(--color-base-content) 43%, var(--color-base-100));
  font-variant-numeric: tabular-nums;
  text-align: right;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agentTaskChevron {
  font-size: 14px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
}

/* 进度条 */
.agentTaskProgressBar {
  display: flex;
  align-items: center;
  padding: 0 22px;
}

.agentTaskProgressBarTrack {
  flex: 1;
  height: 4px;
  background: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 15%, transparent);
  border-radius: 2px;
  overflow: hidden;
}

.agentTaskProgressBarFill {
  height: 100%;
  background: linear-gradient(90deg, #22c55e, #16a34a);
  border-radius: 2px;
  transition: width 0.4s ease;
}

.agentTaskReasoning {
  margin: 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--encv-text-secondary, #6b7280);
  font-style: italic;
  padding: 0 0 0 22px;
  word-break: break-word;
}

.agentTaskList {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.agentTaskItem {
  display: flex;
  align-items: baseline;
  gap: 0.625rem;
  padding: 0.25rem 0;
  font-size: 0.875rem;
  line-height: 1.4;
}

.agentTaskItem--completed .agentTaskDescription {
  text-decoration: line-through;
  color: color-mix(in srgb, var(--color-base-content) 57%, var(--color-base-100));
}

.agentTaskItem--in_progress .agentTaskDescription {
  font-weight: 600;
  color: color-mix(in srgb, var(--color-primary) 85%, var(--color-black));
}

.agentTaskItem--failed .agentTaskDescription {
  color: var(--color-error);
}

.agentTaskItem--pending .agentTaskDescription {
  color: color-mix(in srgb, var(--color-base-content) 71%, var(--color-base-100));
}

.agentTaskMarker {
  flex: 0 0 auto;
  width: 1.1rem;
  text-align: center;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.agentTaskMarkerIcon {
  font-size: 1rem;
  line-height: 1;
}

.agentTaskItem--completed .agentTaskMarkerIcon {
  color: var(--color-success);
}

.agentTaskItem--in_progress .agentTaskMarkerIcon {
  color: var(--color-primary);
  animation: agent-task-spin 1.6s linear infinite;
}

.agentTaskItem--failed .agentTaskMarkerIcon {
  color: var(--color-error);
}

.agentTaskItem--pending .agentTaskMarkerIcon {
  color: color-mix(in srgb, var(--color-base-content) 29%, var(--color-base-100));
}

@keyframes agent-task-spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}

@keyframes agent-task-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.45; }
}

.agentTaskDescription {
  flex: 1 1 auto;
  word-break: break-word;
}

.agentTaskStatusLabel {
  flex: 0 0 auto;
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: color-mix(in srgb, var(--color-base-content) 43%, var(--color-base-100));
}

.agentTaskEmpty {
  margin: 0;
  font-size: 0.85rem;
  color: color-mix(in srgb, var(--color-base-content) 43%, var(--color-base-100));
  font-style: italic;
  padding: 0 0 0 22px;
}
</style>
