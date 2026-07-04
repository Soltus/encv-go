<template>
  <div class="job-card" :class="`job-card--${job.status}`">
    <!-- 头部：Job 名称 + 状态 + matrix 标签 -->
    <header class="job-card__head">
      <div class="job-card__head-l">
        <span class="job-card__job-id">{{ job.jobDefId }}</span>
        <h4 class="job-card__name">{{ jobName }}</h4>
        <div v-if="job.matrixVars && Object.keys(job.matrixVars).length > 0" class="job-card__matrix-tags">
          <span
            v-for="(val, key) in job.matrixVars"
            :key="key"
            class="matrix-tag"
          >{{ key }}={{ val }}</span>
        </div>
      </div>
      <div class="job-card__head-r">
        <StepMiniBadge :status="job.status" :show-name="false" />
        <span v-if="job.conclusion" class="job-card__conclusion">{{ job.conclusion.toUpperCase() }}</span>
      </div>
    </header>

    <!-- 进度条 -->
    <div v-if="job.steps.length > 0" class="job-card__progress">
      <div class="progress-track">
        <div
          class="progress-fill"
          :class="`progress-fill--${fillColor}`"
          :style="{ width: progressPct + '%' }"
        ></div>
      </div>
      <span class="progress-text">{{ completedSteps }}/{{ totalSteps }}</span>
    </div>

    <!-- Steps 列表（紧凑） -->
    <div v-if="expanded || job.steps.length <= 6" class="job-card__steps">
      <StepMiniBadge
        v-for="step in job.steps"
        :key="step.id"
        :status="step.status"
        :name="stepName(step)"
        :show-name="true"
      />
    </div>
    <div v-else class="job-card__steps-collapsed">
      <StepMiniBadge
        v-for="step in visibleSteps"
        :key="step.id"
        :status="step.status"
        :name="stepName(step)"
        :show-name="true"
      />
      <button class="more-btn" @click="expanded = true">+{{ hiddenCount }} more</button>
    </div>

    <!-- 耗时 -->
    <div v-if="job.durationMs" class="job-card__dur">
      {{ formatDur(job.durationMs) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { JobRun, StepRun } from "@/lib/workflow/types";
import StepMiniBadge from "./StepMiniBadge.vue";

const props = defineProps<{
  job: JobRun;
  /** 用于查找 step 定义名称的映射 */
  stepNames?: Map<string, string>;
  /** Job 显示名称（覆盖 jobDefId） */
  displayName?: string;
}>();

const expanded = ref(false);

const jobName = computed(() => props.displayName ?? props.job.jobDefId);

const totalSteps = computed(() => props.job.steps.length);
const completedSteps = computed(
  () =>
    props.job.steps.filter(
      s =>
        s.status === "success" || s.status === "failure" || s.status === "cancelled" || s.status === "skipped" || s.status === "timed_out"
    ).length
);
const progressPct = computed(() => {
  if (totalSteps.value === 0) return 0;
  return Math.round((completedSteps.value / totalSteps.value) * 100);
});

const fillColor = computed(() => {
  if (props.job.status === "failure") return "fail";
  if (props.job.status === "success") return "pass";
  if (props.job.status === "running") return "run";
  if (props.job.status === "cancelled") return "cancel";
  return "default";
});

const visibleSteps = computed(() => props.job.steps.slice(0, 5));
const hiddenCount = computed(() => Math.max(0, props.job.steps.length - 5));

function stepName(step: StepRun): string {
  return props.stepNames?.get(step.stepDefId) ?? step.stepDefId;
}

function formatDur(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.round((ms % 60000) / 1000)}s`;
}
</script>

<style scoped>
.job-card {
  background: #F4EFE6;
  border: 1px solid #D4C9B5;
  border-radius: 4px;
  margin: 0 16px 10px 16px;
  font-family: 'Times New Roman', Georgia, serif;
  overflow: hidden;
}
.job-card--failure { border-left: 3px solid #8B1E3F; }
.job-card--success { border-left: 3px solid #1B4332; }
.job-card--running { border-left: 3px solid #1565C0; }
.job-card--cancelled { border-left: 3px solid #616161; }

.job-card__head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 10px 14px;
  gap: 8px;
}

.job-card__head-l {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.job-card__job-id {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 8px;
  letter-spacing: 0.2em;
  color: #6B5D4C;
}

.job-card__name {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: #1A1A1A;
  letter-spacing: -0.01em;
}

.job-card__matrix-tags {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 2px;
}

.matrix-tag {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  background: #EDE5D2;
  color: #4A3F2E;
  padding: 1px 5px;
  border-radius: 2px;
}

.job-card__head-r {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.job-card__conclusion {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 8px;
  letter-spacing: 0.18em;
  color: #6B5D4C;
}

/* 进度条 */
.job-card__progress {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 14px 6px;
}

.progress-track {
  flex: 1;
  height: 3px;
  background: #D4C9B5;
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s ease;
}
.progress-fill--pass { background: #1B4332; }
.progress-fill--fail { background: #8B1E3F; }
.progress-fill--run { background: #1565C0; animation: progress-pulse 1.5s ease-in-out infinite; }
.progress-fill--cancel { background: #616161; }
.progress-fill--default { background: #C9BBA1; }

@keyframes progress-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.6; }
}

.progress-text {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 10px;
  color: #6B5D4C;
  min-width: 36px;
  text-align: right;
}

/* Steps */
.job-card__steps {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 6px 14px 10px;
  border-top: 1px solid #EDE5D2;
}

.job-card__steps-collapsed {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  padding: 6px 14px 10px;
  border-top: 1px solid #EDE5D2;
}

.more-btn {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  color: #2B3A67;
  background: none;
  border: none;
  cursor: pointer;
  padding: 2px 4px;
}
.more-btn:hover { text-decoration: underline; }

.job-card__dur {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  color: #8B7355;
  padding: 4px 14px 8px;
  text-align: right;
}
</style>
