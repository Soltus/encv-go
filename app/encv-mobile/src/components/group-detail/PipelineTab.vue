<script setup lang="ts">
import { useI18n } from "@/composables/useI18n";
import type { JobRun, WorkflowRun } from "@/lib/workflow/types";
import { computed } from "vue";

interface Props {
  run?: WorkflowRun;
  jobs: JobRun[];
  total?: number;
  passed?: number;
  failed?: number;
  pending?: number;
  skipped?: number;
  platform?: string;
}
const props = defineProps<Props>();
const emit = defineEmits<(e: "select-job", job: JobRun) => void>();

const { t } = useI18n();

const sortedJobs = computed<JobRun[]>(() => {
  return [...props.jobs].sort((a, b) => {
    // 失败/进行中 优先 → 已完成 最后
    const sa = a.status;
    const sb = b.status;
    const order: Record<string, number> = { failed: 0, running: 1, queued: 2, cancelling: 3, completed: 4, cancelled: 5 };
    return (order[sa] ?? 99) - (order[sb] ?? 99);
  });
});

const durationMs = computed<number>(() => {
  if (!props.run?.durationMs) return 0;
  return props.run.durationMs;
});
</script>

<template>
  <div class="pipeline-tab">
    <TestReportHeader
      v-if="run"
      :run-id="run.id"
      :opened-at="run.createdAt"
      :duration-ms="durationMs"
      :total="total ?? 0"
      :passed="passed ?? 0"
      :failed="failed ?? 0"
      :pending="pending ?? 0"
      :skipped="skipped ?? 0"
      :platform="platform ?? 'web'"
    />
    <div class="job-list">
      <JobPipelineCard
        v-for="job in sortedJobs"
        :key="job.id"
        :job="job"
        @click="emit('select-job', job)"
      />
      <div v-if="sortedJobs.length === 0" class="empty">
        <ion-icon :icon="'git-network-outline'" size="large" color="medium"></ion-icon>
        <p>{{ t('tasks.pipelineEmpty') }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pipeline-tab {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.job-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.empty {
  text-align: center;
  padding: 48px 16px;
  color: var(--ion-color-medium);
}
.empty p {
  margin-top: 8px;
  font-size: 13px;
}
</style>
