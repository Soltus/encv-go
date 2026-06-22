<template>
  <div class="pipeline-tab">
    <!-- 报告头部 -->
    <TestReportHeader
      v-if="reportHeaderProps"
      v-bind="reportHeaderProps"
    />

    <!-- 🆕 2026-06-22 Q4：视图模式 toggle（折叠 vs 展开） -->
    <div class="pipeline-view-toggle">
      <ion-segment v-model="viewMode" mode="md">
        <ion-segment-button value="collapsed">
          <ion-icon :icon="reorderFourOutline" slot="start"></ion-icon>
          <ion-label>{{ t('tasks.pipelineViewCollapsed') }}</ion-label>
        </ion-segment-button>
        <ion-segment-button value="expanded">
          <ion-icon :icon="gitNetworkOutline" slot="start"></ion-icon>
          <ion-label>{{ t('tasks.pipelineViewExpanded') }}</ion-label>
        </ion-segment-button>
      </ion-segment>
    </div>

    <div v-if="jobs.length === 0" class="empty-state">
      <ion-icon :icon="gitNetworkOutline" class="empty-icon" color="medium"></ion-icon>
      <h3>{{ t('tasks.groupDetail.noJobs') }}</h3>
      <p>{{ t('tasks.groupDetail.noJobsDesc') }}</p>
    </div>
    <div v-else class="pipeline-tab__list">
      <!-- 折叠模式：仅显示 job 摘要 + 失败 step 数 -->
      <div v-if="viewMode === 'collapsed'">
        <ion-card v-for="job in jobs" :key="job.id" class="job-summary-card" button @click="toggleJob(job.id)">
          <ion-card-content>
            <div class="job-summary-row">
              <ion-icon
                :icon="expandedJobs.has(job.id) ? chevronDown : chevronForward"
                size="small"
              ></ion-icon>
              <span class="job-name">{{ getJobDisplayName(job) }}</span>
              <ion-badge :color="jobStatusColor(job)" class="job-status-badge">
                {{ jobStatusText(job) }}
              </ion-badge>
            </div>
          </ion-card-content>
        </ion-card>
      </div>
      <!-- 展开模式：完整 DAG 树 -->
      <div v-else>
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
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { IonIcon, IonSegment, IonSegmentButton, IonLabel, IonCard, IonCardContent, IonBadge } from '@ionic/vue'
import { gitNetworkOutline, reorderFourOutline, chevronDown, chevronForward } from 'ionicons/icons'
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

// 🆕 Q4：视图模式（默认折叠）
const viewMode = ref<'collapsed' | 'expanded'>('collapsed')
const expandedJobs = ref<Set<string>>(new Set())

function toggleJob(jobId: string) {
  const next = new Set(expandedJobs.value)
  if (next.has(jobId)) next.delete(jobId)
  else next.add(jobId)
  expandedJobs.value = next
}

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

function jobStatusColor(job: JobRun): string {
  const failed = job.steps.filter((s) => s.status === 'failure').length
  if (failed > 0) return 'danger'
  const running = job.steps.filter((s) => s.status === 'running' || s.status === 'queued').length
  if (running > 0) return 'primary'
  return 'success'
}

function jobStatusText(job: JobRun): string {
  const failed = job.steps.filter((s) => s.status === 'failure').length
  const passed = job.steps.filter((s) => s.status === 'success').length
  return `${passed}/${job.steps.length}${failed > 0 ? ` (${failed} ✗)` : ''}`
}
</script>

<style scoped>
.pipeline-view-toggle {
  padding: 8px 12px;
  background: var(--ion-color-light);
}
.job-summary-card {
  margin: 4px 8px;
  cursor: pointer;
}
.job-summary-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.job-name {
  flex: 1;
  font-size: 14px;
  font-weight: 500;
}
.job-status-badge {
  font-size: 11px;
}
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
