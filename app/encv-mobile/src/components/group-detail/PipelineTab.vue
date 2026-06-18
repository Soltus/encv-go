<template>
  <div class="pipeline-tab">
    <!-- 报告头部 -->
    <TestReportHeader
      v-if="reportHeaderProps"
      v-bind="reportHeaderProps"
    />

    <div v-if="jobs.length === 0" class="empty-state">
      <ion-icon :icon="gitNetworkOutline" class="empty-icon" color="medium"></ion-icon>
      <h3>{{ t('tasks.groupDetail.noJobs') }}</h3>
      <p>{{ t('tasks.groupDetail.noJobsDesc') }}</p>
    </div>
    <div v-else class="pipeline-tab__list">
      <JobPipelineCard
        v-for="job in jobs"
        :key="job.id"
        :job="job"
        :step-names="stepNamesMap"
        :display-name="getJobDisplayName(job)"
        @click="emit('select-job', job)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IonIcon } from '@ionic/vue'
import { gitNetworkOutline } from 'ionicons/icons'
import TestReportHeader from '@/components/automation/TestReportHeader.vue'
import JobPipelineCard from '@/components/automation/JobPipelineCard.vue'
import type { JobRun } from '@/lib/workflow/types'
import { useI18n } from '@/composables/useI18n'

const props = defineProps<{
  jobs: JobRun[]
  runId: string
  startedAt?: string
  durationMs?: number
  total: number
  passed: number
  failed: number
  pending: number
  skipped: number
}>()

const emit = defineEmits<{
  (e: 'select-job', job: JobRun): void
}>()

const { t } = useI18n()

const stepNamesMap = computed<Map<string, string> | undefined>(() => {
  const m = new Map<string, string>()
  for (const job of props.jobs) {
    for (const step of job.steps) {
      m.set(step.id, step.stepDefId)
    }
  }
  return m
})

const reportHeaderProps = computed(() => ({
  runId: props.runId.slice(0, 8),
  openedAt: props.startedAt ?? new Date().toISOString(),
  durationMs: props.durationMs ?? 0,
  total: props.total,
  passed: props.passed,
  failed: props.failed,
  skipped: props.skipped,
  pending: props.pending,
  platform: 'encv-automation',
}))

function getJobDisplayName(job: JobRun): string {
  return job.jobDefId
    .replace(/^[a-z]+-/, '')
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}
</script>

<style scoped>
.pipeline-tab__list {
  padding: 12px;
  background: transparent;
}
.empty-state {
  text-align: center;
  padding: 48px 16px;
}
.empty-icon {
  font-size: 56px;
  margin-bottom: 8px;
  display: block;
  margin-left: auto;
  margin-right: auto;
}
</style>
