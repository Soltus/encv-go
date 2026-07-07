<script setup lang="ts">
import { useI18n } from "@/composables/useI18n";
import BlockHeader from "@/components/agent/BlockHeader.vue";
import {
  checkboxOutline,
  checkmarkCircle,
  ellipsisHorizontalCircle,
  sync,
} from "ionicons/icons";
import { computed } from "vue";

export interface PlanTodo {
  id: string;
  status: "pending" | "in_progress" | "completed" | string;
  content: string;
}

const props = withDefaults(
  defineProps<{
    todos: PlanTodo[];
    streaming?: boolean;
  }>(),
  {
    streaming: false,
  }
);

const { t } = useI18n();

// Split todos by status so completed items sit at the bottom
// of the list (they read as a log of what's been done) and
// the in_progress / pending items are at the top where the
// user's eye lands first. This is a presentational choice
// only — the underlying id+status+content is preserved so
// the LLM's notion of ordering can be reconstructed by id.
const orderedTodos = computed(() => {
  const inProgress = props.todos.filter(x => x.status === "in_progress");
  const pending = props.todos.filter(x => x.status === "pending");
  const completed = props.todos.filter(x => x.status === "completed");
  const unknown = props.todos.filter(x => x.status !== "in_progress" && x.status !== "pending" && x.status !== "completed");
  return [...inProgress, ...pending, ...unknown, ...completed];
});

const completedCount = computed(() => props.todos.filter(x => x.status === "completed").length);
const inProgressCount = computed(() => props.todos.filter(x => x.status === "in_progress").length);
const progressPct = computed(() => {
  const total = props.todos.length;
  if (total === 0) return 0;
  return Math.round((completedCount.value / total) * 100);
});

function statusLabel(status: string): string {
  if (status === "in_progress") return t("agent.planStatusInProgress");
  if (status === "completed") return t("agent.planStatusCompleted");
  if (status === "pending") return t("agent.planStatusPending");
  return status;
}

function statusIcon(status: string) {
  if (status === "completed") return checkmarkCircle;
  if (status === "in_progress") return sync;
  return ellipsisHorizontalCircle;
}
</script>

<template>
  <div class="plan-block" :class="{ 'is-streaming': props.streaming }">
    <BlockHeader
      :icon="checkboxOutline"
      :title="t('agent.plan')"
      :badge="props.streaming ? t('agent.streaming') : undefined"
      :expanded="true"
    />
    <!-- 进度条（仅当有 todo 时显示） -->
    <div v-if="orderedTodos.length > 0" class="plan-progress" aria-label="Plan progress">
      <div class="plan-progress-bar">
        <div class="plan-progress-fill" :style="{ width: progressPct + '%' }" />
      </div>
      <span class="plan-progress-text">
        <span class="plan-progress-count">{{ completedCount }}</span>
        <span class="plan-progress-sep">/</span>
        <span class="plan-progress-total">{{ orderedTodos.length }}</span>
        <span v-if="inProgressCount > 0" class="plan-progress-hint">
          （{{ inProgressCount }} {{ t('agent.planStatusInProgress') }}）
        </span>
      </span>
    </div>
    <ol v-if="orderedTodos.length > 0" class="plan-list" data-testid="plan-list">
      <li
        v-for="todo in orderedTodos"
        :key="todo.id"
        class="plan-item"
        :class="`plan-item--${todo.status}`"
        :data-testid="`plan-item-${todo.id}`"
      >
        <span class="plan-marker" aria-hidden="true">
          <ion-icon
            :icon="statusIcon(todo.status)"
            :class="`plan-marker-icon plan-marker-icon--${todo.status}`"
          />
        </span>
        <span class="plan-content">{{ todo.content }}</span>
        <span class="plan-status">{{ statusLabel(todo.status) }}</span>
      </li>
    </ol>
    <p v-else class="plan-empty">{{ t('agent.planEmpty') }}</p>
  </div>
</template>

<style scoped>
.plan-block {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem 0.875rem;
  border: 1px solid var(--ion-color-step-200, #e4e4e7);
  border-radius: 0.5rem;
  background: var(--ion-color-step-50, #fafafa);
}

.plan-block.is-streaming {
  border-color: var(--ion-color-primary);
  box-shadow: 0 0 0 1px var(--ion-color-primary-tint, rgba(79, 140, 255, 0.3));
}

/* 进度条 */
.plan-progress {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
  color: var(--encv-text-secondary);
}

.plan-progress-bar {
  flex: 1;
  height: 4px;
  background: rgba(var(--ion-color-medium-rgb), 0.15);
  border-radius: 2px;
  overflow: hidden;
}

.plan-progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #22c55e, #16a34a);
  border-radius: 2px;
  transition: width 0.4s ease;
}

.plan-progress-text {
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
  white-space: nowrap;
}

.plan-progress-count {
  font-weight: 600;
  color: var(--ion-color-success, #16a34a);
}

.plan-progress-sep,
.plan-progress-total {
  color: var(--encv-text-secondary);
}

.plan-progress-hint {
  margin-left: 4px;
  color: var(--ion-color-primary, #4f8cff);
  font-weight: 500;
}

.plan-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.plan-item {
  display: flex;
  align-items: baseline;
  gap: 0.625rem;
  padding: 0.25rem 0;
  font-size: 0.875rem;
  line-height: 1.4;
}

.plan-item--completed .plan-content {
  text-decoration: line-through;
  color: var(--ion-color-step-600, #6b7280);
}

.plan-item--in_progress .plan-content {
  font-weight: 600;
  color: var(--ion-color-primary-shade, #1d4ed8);
}

.plan-item--pending .plan-content {
  color: var(--ion-color-step-700, #374151);
}

.plan-item--unknown .plan-content {
  color: var(--ion-color-step-700, #374151);
}

.plan-marker {
  flex: 0 0 auto;
  width: 1.1rem;
  text-align: center;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.plan-marker-icon {
  font-size: 1rem;
  line-height: 1;
}

.plan-item--completed .plan-marker-icon {
  color: var(--ion-color-success, #16a34a);
}

.plan-item--in_progress .plan-marker-icon {
  color: var(--ion-color-primary, #4f8cff);
  animation: plan-marker-spin 1.6s linear infinite;
}

.plan-item--pending .plan-marker-icon {
  color: var(--ion-color-step-400, #9ca3af);
}

.plan-item--unknown .plan-marker-icon {
  color: var(--ion-color-step-400, #9ca3af);
}

@keyframes plan-marker-spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}

@keyframes plan-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.45; }
}

.plan-content {
  flex: 1 1 auto;
  word-break: break-word;
}

.plan-status {
  flex: 0 0 auto;
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ion-color-step-500, #6b7280);
}

.plan-empty {
  margin: 0;
  font-size: 0.85rem;
  color: var(--ion-color-step-500, #6b7280);
  font-style: italic;
}
</style>
