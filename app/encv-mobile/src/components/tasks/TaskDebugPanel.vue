<!--
  TaskDebugPanel — 任务"逃逸"诊断面板（真机可见版）
  2026-06-22 新增：嵌在 Tasks 页面顶部的实时诊断面板，让 user 在真机上
  直接看到逃逸 task 的数量 / 各 runId 聚合情况 / 视图状态叠加 / 时间桶分布。

  触发条件：
   - URL 加 ?debug=tasks 强制显示
   - 默认折叠收起（<details>）避免遮挡正常 task 列表
   - 真 group 数 / 伪 group 数一目了然（伪 group 红字标）

  显示内容：
   ① 顶部统计：tasks 总数 / 真 group / 伪 group（__manual__）/ displayedItems
   ② 视图状态：viewMode / sortBy / 6 类 filter / search / pin / date preset
   ③ runId 聚合：每个 runId 聚合 task 数（按 task 数降序），伪 group 标红
   ④ 时间桶分布：today / yesterday / thisWeek / thisMonth / earlier
   ⑤ 自我诊断：自动跑出关键告警（"逃逸 N 个 / 真因 = bulkSetTasks 丢 runId"）
-->
<template>
  <details class="taskDebugPanel" :open="defaultOpen">
    <summary class="taskDebugSummary">
      <ion-icon :icon="bugOutline" class="taskDebugSummaryIcon" />
      <span>任务诊断面板（task debug）</span>
      <span class="taskDebugBadge">
        {{ tasks.length }} task · {{ realGroupCount }} 真 group · <span :class="fakeGroupCount > 0 ? 'taskDebugBadge_alert' : ''">{{ fakeGroupCount }} 伪 group</span>
      </span>
    </summary>

    <div class="taskDebugBody">
      <!-- ① 顶部统计 -->
      <section class="taskDebugSection">
        <h4>① 顶部统计</h4>
        <div class="taskDebugStats">
          <span class="taskDebugChip">store.tasks: {{ tasks.length }}</span>
          <span class="taskDebugChip">displayedItems: {{ displayedItems.length }}</span>
          <span class="taskDebugChip">真 group: {{ realGroupCount }}</span>
          <span :class="fakeGroupCount > 0 ? 'taskDebugChip taskDebugChip_alert' : 'taskDebugChip'">
            伪 group (__manual__): {{ fakeGroupCount }}
          </span>
          <span class="taskDebugChip">逃逸 task 数: {{ escapeTaskCount }}</span>
        </div>
        <p v-if="fakeGroupCount > 0" class="taskDebugHint">
          ⚠️ {{ fakeGroupCount }} 个 task 失去 runId（fetchTasks 拉脏数据）→ 触发"任务逃逸"。
          修复：[taskStore.ts] bulkSetTasks 改 merge 模式（保留 prev.runId）。
        </p>
        <p v-else class="taskDebugHint_ok">✅ 无逃逸 task（merge 模式生效）</p>
      </section>

      <!-- ② 视图状态 -->
      <section class="taskDebugSection">
        <h4>② 视图状态</h4>
        <div class="taskDebugStats">
          <span class="taskDebugChip">viewMode: {{ viewMode }}</span>
          <span class="taskDebugChip">sortBy: {{ sortBy }}</span>
          <span class="taskDebugChip">search: "{{ searchQuery }}"</span>
          <span class="taskDebugChip">filterPlugins: [{{ filterPlugins.join(', ') }}]</span>
          <span class="taskDebugChip">filterTypes: [{{ filterTypes.join(', ') }}]</span>
          <span class="taskDebugChip">filterStatuses: [{{ filterStatuses.join(', ') }}]</span>
          <span class="taskDebugChip">filterTriggeredBy: [{{ filterTriggeredBy.join(', ') }}]</span>
          <span class="taskDebugChip">datePreset: {{ filterDatePreset }}</span>
          <span class="taskDebugChip">pinned: [{{ Array.from(pinnedRunIds).join(', ') }}]</span>
        </div>
      </section>

      <!-- ③ runId 聚合 -->
      <section class="taskDebugSection">
        <h4>③ runId 聚合（{{ groupedTasksByRunId.length }} 个 group）</h4>
        <div v-if="groupedTasksByRunId.length === 0" class="taskDebugHint">（空）</div>
        <ul v-else class="taskDebugRunList">
          <li
            v-for="g in sortedGroups"
            :key="g.runId"
            :class="g.isFake ? 'taskDebugRunItem taskDebugRunItem_fake' : 'taskDebugRunItem'"
          >
            <span class="taskDebugRunId">{{ g.runId }}</span>
            <span class="taskDebugRunCount">{{ g.taskCount }} task</span>
            <span class="taskDebugRunStatus">{{ g.statusSummary }}</span>
            <span v-if="g.isFake" class="taskDebugRunFakeTag">__manual__ 伪 group</span>
            <span v-else-if="pinnedSet.has(g.runId)" class="taskDebugRunPinTag">📌 pin</span>
          </li>
        </ul>
      </section>

      <!-- ④ 时间桶分布 -->
      <section class="taskDebugSection">
        <h4>④ 时间桶分布</h4>
        <div class="taskDebugStats">
          <span class="taskDebugChip">today: {{ bucketCounts.today }}</span>
          <span class="taskDebugChip">yesterday: {{ bucketCounts.yesterday }}</span>
          <span class="taskDebugChip">thisWeek: {{ bucketCounts.thisWeek }}</span>
          <span class="taskDebugChip">thisMonth: {{ bucketCounts.thisMonth }}</span>
          <span class="taskDebugChip">earlier: {{ bucketCounts.earlier }}</span>
        </div>
      </section>

      <!-- ⑤ 自我诊断 -->
      <section class="taskDebugSection">
        <h4>⑤ 自我诊断（自动跑出关键告警）</h4>
        <ul class="taskDebugDiag">
          <li v-for="(line, i) in diagnostics" :key="i" :class="`taskDebugDiag_${line.level}`">
            <span class="taskDebugDiagLevel">{{ line.level }}</span>
            {{ line.text }}
          </li>
          <li v-if="diagnostics.length === 0" class="taskDebugDiag_empty">
            <span class="taskDebugDiagLevel">empty</span>
            store 是空 / 或所有 key 都健康
          </li>
        </ul>
      </section>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IonIcon } from '@ionic/vue'
import { bugOutline } from 'ionicons/icons'
import type { EncvTask } from '@/api/encv'

const props = defineProps<{
  tasks: EncvTask[]
  displayedItems: any[]
  groupedTasksByRunId: Array<{ key: string; runId: string; tasks: EncvTask[]; startedAt: string }>
  viewMode: string
  sortBy: string
  searchQuery: string
  filterPlugins: string[]
  filterTypes: string[]
  filterStatuses: string[]
  filterTriggeredBy: string[]
  filterDatePreset: string
  pinnedRunIds: Set<string>
  defaultOpen?: boolean
}>()

// ============ 派生统计 ============
const realGroupCount = computed(() =>
  props.groupedTasksByRunId.filter((g) => !g.runId.startsWith('__manual__')).length,
)
const fakeGroupCount = computed(() =>
  props.groupedTasksByRunId.filter((g) => g.runId.startsWith('__manual__')).length,
)
const escapeTaskCount = computed(() =>
  props.groupedTasksByRunId
    .filter((g) => g.runId.startsWith('__manual__'))
    .reduce((acc, g) => acc + g.tasks.length, 0),
)
const pinnedSet = computed(() => props.pinnedRunIds)

const sortedGroups = computed(() => {
  return props.groupedTasksByRunId
    .map((g) => {
      const isFake = g.runId.startsWith('__manual__')
      // 状态汇总
      const statusMap: Record<string, number> = {}
      for (const t of g.tasks) {
        statusMap[t.status] = (statusMap[t.status] ?? 0) + 1
      }
      const statusSummary = Object.entries(statusMap)
        .map(([s, n]) => `${s}:${n}`)
        .join(' / ') || '—'
      return { runId: g.runId, taskCount: g.tasks.length, statusSummary, isFake }
    })
    .sort((a, b) => {
      if (a.isFake !== b.isFake) return a.isFake ? 1 : -1
      return b.taskCount - a.taskCount
    })
})

// 时间桶
function dateSectionKey(date: string): string {
  const d = new Date(date)
  const now = new Date()
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const yesterdayStart = todayStart - 86400000
  const weekStart = todayStart - 7 * 86400000
  const monthStart = new Date(now.getFullYear(), now.getMonth(), 1).getTime()
  const ts = d.getTime()
  if (ts >= todayStart) return 'today'
  if (ts >= yesterdayStart) return 'yesterday'
  if (ts >= weekStart) return 'thisWeek'
  if (ts >= monthStart) return 'thisMonth'
  return 'earlier'
}
const bucketCounts = computed(() => {
  const out = { today: 0, yesterday: 0, thisWeek: 0, thisMonth: 0, earlier: 0 }
  for (const t of props.tasks) {
    const k = dateSectionKey(t.createdAt)
    out[k as keyof typeof out]++
  }
  return out
})

// 自我诊断
type Diag = { level: 'ok' | 'warn' | 'error' | 'info'; text: string }
const diagnostics = computed<Diag[]>(() => {
  const out: Diag[] = []
  if (props.tasks.length === 0) {
    out.push({ level: 'info', text: 'store.tasks 是空（首次进入或已清空）' })
    return out
  }
  // 1. 逃逸检测
  if (fakeGroupCount.value > 0) {
    out.push({
      level: 'error',
      text: `检测到 ${fakeGroupCount.value} 个 __manual__ 伪 group（${escapeTaskCount.value} 个 task 失去 runId）。`
        + '真因：fetchTasks 返回的 task.runId 字段是空字符串 → Go json omitempty → 前端 merge 模式未生效。',
    })
  } else if (escapeTaskCount.value > 0) {
    out.push({ level: 'warn', text: `有 ${escapeTaskCount.value} 个逃逸 task 但未聚合到伪 group（异常状态）` })
  } else {
    out.push({ level: 'ok', text: '无逃逸 task（merge 模式生效，prev.runId 保留成功）' })
  }
  // 2. runId 缺失统计
  const taskWithRunId = props.tasks.filter((t) => t.runId && !t.runId.startsWith('__manual__')).length
  const taskWithoutRunId = props.tasks.length - taskWithRunId
  if (taskWithoutRunId > 0) {
    out.push({ level: 'warn', text: `store 里 ${taskWithoutRunId}/${props.tasks.length} task 没有 runId（merge 模式会保留 prev）` })
  }
  // 3. triggeredBy 缺失
  const taskWithTriggeredBy = props.tasks.filter((t) => t.triggeredBy).length
  if (taskWithTriggeredBy < props.tasks.length) {
    out.push({ level: 'warn', text: `store 里 ${props.tasks.length - taskWithTriggeredBy} task 缺 triggeredBy（默认为 user）` })
  }
  // 4. viewMode + sortBy 一致性
  if (props.viewMode === 'group' && props.groupedTasksByRunId.length > 0 && props.displayedItems.length === 0) {
    out.push({ level: 'error', text: 'group 模式有 group 数据但 displayedItems.length=0（filter 把所有 group 过滤了）' })
  }
  // 5. displayedItems 比例
  const dateCount = props.displayedItems.filter((it: any) => it.kind === 'date').length
  const groupCount = props.displayedItems.filter((it: any) => it.kind === 'group').length
  const taskCount = props.displayedItems.filter((it: any) => it.kind === 'task').length
  out.push({
    level: 'info',
    text: `displayedItems 拆解：date=${dateCount} / group=${groupCount} / task=${taskCount}（合计 ${dateCount + groupCount + taskCount}）`,
  })
  // 6. 时间桶异常
  if (bucketCounts.value.earlier > bucketCounts.value.today * 5) {
    out.push({ level: 'warn', text: `earlier 桶有 ${bucketCounts.value.earlier} task（远多于 today ${bucketCounts.value.today}）→ 时间分布异常` })
  }
  return out
})
</script>

<style scoped>
.taskDebugPanel {
  margin: 8px 12px;
  border: 1px solid var(--ion-color-primary-tint, #4f8cff);
  border-radius: 8px;
  background: var(--ion-color-light, #f4f5f8);
  font-size: 12px;
  font-family: ui-monospace, 'SF Mono', Menlo, monospace;
}
.taskDebugSummary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  user-select: none;
  font-weight: 600;
  color: var(--ion-color-primary);
}
.taskDebugSummaryIcon { font-size: 16px; }
.taskDebugBadge {
  margin-left: auto;
  font-size: 11px;
  color: var(--ion-color-medium);
  font-weight: 400;
}
.taskDebugBadge_alert {
  color: var(--ion-color-danger);
  font-weight: 700;
}
.taskDebugBody {
  padding: 8px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.taskDebugSection {
  border-top: 1px dashed var(--ion-color-medium-tint, #c8c8d0);
  padding-top: 8px;
}
.taskDebugSection h4 {
  margin: 0 0 6px 0;
  font-size: 12px;
  color: var(--ion-color-primary-shade);
  font-weight: 700;
}
.taskDebugStats {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.taskDebugChip {
  display: inline-block;
  padding: 2px 6px;
  background: var(--ion-color-light-shade, #e6e7eb);
  color: var(--ion-color-dark);
  border-radius: 4px;
  font-size: 11px;
  line-height: 1.4;
}
.taskDebugChip_alert {
  background: var(--ion-color-danger-tint, #f5b3b3);
  color: var(--ion-color-danger-shade, #b30000);
  font-weight: 700;
}
.taskDebugRunList {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 200px;
  overflow-y: auto;
}
.taskDebugRunItem {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 6px;
  background: var(--ion-color-light-shade, #e6e7eb);
  border-radius: 3px;
  font-size: 11px;
}
.taskDebugRunItem_fake {
  background: var(--ion-color-danger-tint, #f5b3b3);
  color: var(--ion-color-danger-shade, #b30000);
  font-weight: 600;
}
.taskDebugRunId {
  font-weight: 600;
  flex: 0 0 auto;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.taskDebugRunCount {
  font-weight: 700;
  color: var(--ion-color-primary);
}
.taskDebugRunStatus {
  color: var(--ion-color-medium-shade);
  font-size: 10px;
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.taskDebugRunFakeTag {
  font-size: 10px;
  color: var(--ion-color-danger);
  font-weight: 700;
}
.taskDebugRunPinTag {
  font-size: 10px;
  color: var(--ion-color-primary);
}
.taskDebugDiag {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.taskDebugDiag li {
  display: flex;
  gap: 6px;
  padding: 2px 4px;
  border-radius: 3px;
  font-size: 11px;
  line-height: 1.5;
}
.taskDebugDiag_ok { background: rgba(34, 197, 94, 0.1); }
.taskDebugDiag_warn { background: rgba(249, 115, 22, 0.1); }
.taskDebugDiag_error { background: rgba(239, 68, 68, 0.15); color: var(--ion-color-danger-shade, #b30000); }
.taskDebugDiag_info { background: rgba(79, 140, 255, 0.05); }
.taskDebugDiagLevel {
  flex: 0 0 auto;
  padding: 0 4px;
  border-radius: 2px;
  font-size: 9px;
  font-weight: 700;
  text-transform: uppercase;
  height: 16px;
  line-height: 16px;
  background: var(--ion-color-medium);
  color: white;
}
.taskDebugDiag_ok .taskDebugDiagLevel { background: var(--ion-color-success); }
.taskDebugDiag_warn .taskDebugDiagLevel { background: var(--ion-color-warning); }
.taskDebugDiag_error .taskDebugDiagLevel { background: var(--ion-color-danger); }
.taskDebugDiag_info .taskDebugDiagLevel { background: var(--ion-color-primary); }
.taskDebugDiag_empty { color: var(--ion-color-medium); font-style: italic; }
.taskDebugHint {
  margin: 4px 0 0 0;
  font-size: 11px;
  color: var(--ion-color-warning-shade);
  background: var(--ion-color-warning-tint);
  padding: 4px 6px;
  border-radius: 3px;
  line-height: 1.5;
}
.taskDebugHint_ok {
  margin: 4px 0 0 0;
  font-size: 11px;
  color: var(--ion-color-success-shade);
  background: var(--ion-color-success-tint);
  padding: 4px 6px;
  border-radius: 3px;
}
</style>