<template>
  <div class="step-inline-timeline">
    <!-- 空状态：step 无 phase 且无 startedAt -->
    <div v-if="entries.length === 0" class="step-inline-timeline__empty">
      {{ t('tasks.timeline') }}: —
    </div>

    <!-- 时间线卡片列表 -->
    <div v-else class="step-inline-timeline__list">
      <UnifiedTimelineCard
        v-for="entry in entries"
        :key="entry.id"
        :entry="entry"
        :highlight="entry.isHighlight === true"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import UnifiedTimelineCard from "@/components/shared/UnifiedTimelineCard.vue";
import { formatDateTime, formatDuration } from "@/composables/useDateFormat";
import { useI18n } from "@/composables/useI18n";
import { Phase, type StepRun, type StepStatus, type UnifiedTimelineEntry } from "@/lib/workflow/types";

const props = defineProps<{
  step: StepRun;
}>();

const { t } = useI18n();

// ==================== Phase 映射表 ====================

/** 裸 phase 字符串 → Phase 枚举（兼容后端推送的字符串值） */
const PHASE_MAP: Record<string, Phase> = {
  created: Phase.Created,
  analyzing: Phase.Analyzing,
  initializing: Phase.Initializing,
  preprocessing: Phase.Preprocessing,
  encrypting: Phase.Encrypting,
  decrypting: Phase.Decrypting,
  packing: Phase.Packing,
  verifying: Phase.Verifying,
  completed: Phase.Completed,
  // 兼容旧值
  done: Phase.Completed,
};

/** Phase 枚举 → i18n key */
const PHASE_I18N_KEY: Record<Phase, string> = {
  [Phase.Created]: "tasks.timelineCreated",
  [Phase.Analyzing]: "tasks.phaseAnalyzing",
  [Phase.Initializing]: "tasks.phaseInitializing",
  [Phase.Preprocessing]: "tasks.phasePreprocessing",
  [Phase.Encrypting]: "tasks.phaseEncrypting",
  [Phase.Decrypting]: "tasks.phaseDecrypting",
  [Phase.Packing]: "tasks.phasePacking",
  [Phase.Verifying]: "tasks.phaseVerifying",
  [Phase.Completed]: "tasks.phaseCompleted",
};

/** 根据 phase 字符串获取 i18n 标签 */
function getPhaseLabel(phase: string): string {
  const phaseEnum = PHASE_MAP[phase];
  if (!phaseEnum) return phase;
  return t(PHASE_I18N_KEY[phaseEnum]);
}

/** 把 phase 字符串转为 Phase 枚举（未知值降级为 Created） */
function toPhase(phase: string | undefined | null): Phase {
  if (!phase) return Phase.Created;
  return PHASE_MAP[phase] ?? Phase.Created;
}

// ==================== 耗时计算 ====================

/** 计算两个 ISO 时间字符串之间的毫秒数（无效返回 0） */
function calcDurationMs(startedAt?: string, completedAt?: string): number {
  if (!startedAt || !completedAt) return 0;
  const start = new Date(startedAt).getTime();
  const end = new Date(completedAt).getTime();
  if (isNaN(start) || isNaN(end) || end < start) return 0;
  return end - start;
}

// ==================== 终态 / 状态映射 ====================

/** 终态 step 状态集合 */
const TERMINAL_STATUS: Set<StepStatus> = new Set(["success", "failure", "cancelled", "skipped", "timed_out"]);

/** 失败类状态集合（用于标记最后一个时间线条目为 failure） */
const FAILURE_STATUS: Set<StepStatus> = new Set(["failure", "cancelled", "timed_out"]);

// ==================== 内部构建类型 ====================

/**
 * 内部条目（带原始耗时毫秒数，用于计算最长耗时高亮）
 * 不暴露给 UnifiedTimelineCard，仅用于 computed 内部排序
 */
interface InternalTimelineEntry extends UnifiedTimelineEntry {
  _durationMs?: number;
}

// ==================== 时间线构建 ====================

/**
 * 从 StepRun 派生 UnifiedTimelineEntry[]
 *
 * 派生逻辑：
 *   1. 始终推送 "Started" 事件（phase=Created, time=startedAt）—— 若 startedAt 存在
 *   2. 当前 phase 事件（phase=step.phase, progress/speed/eta/duration）—— 若 phase 存在且不为 'completed'
 *   3. "Completed" 事件（phase=Completed, time=completedAt, duration）—— 若 completedAt 存在
 *   4. 失败/取消态：标记最后一个事件为 failure，附加 error 信息
 *   5. 计算最长耗时 phase 并高亮
 *
 * 特殊场景：
 *   - step 无 phase 且无 startedAt → 空状态
 *   - step 只有 phase（无 startedAt）→ 仅 1 个 phase 条目
 *   - step 有 startedAt + phase（运行中）→ 2 个条目（Started + Current phase）
 *   - step 有 startedAt + completedAt（已完成）→ 2-3 个条目（Started + Completed，可能含中间 phase）
 */
const entries = computed<UnifiedTimelineEntry[]>(() => {
  const step = props.step;
  const result: InternalTimelineEntry[] = [];

  // 1. "Started" 事件（phase=Created）
  if (step.startedAt) {
    result.push({
      id: `started-${step.id}`,
      phase: Phase.Created,
      label: t("tasks.timelineCreated"),
      time: formatDateTime(step.startedAt),
      status: "success",
      isCurrent: false,
      hasExpandableDetail: false,
    });
  }

  // 2. 当前 phase 事件（若 phase 存在且不为 'completed'）
  const phaseStr = step.phase;
  const isCompletedPhase = phaseStr === "completed" || phaseStr === "done";
  if (phaseStr && !isCompletedPhase) {
    const phaseEnum = toPhase(phaseStr);
    const isRunning = step.status === "running";
    const isTerminal = TERMINAL_STATUS.has(step.status);
    const durationMs = step.durationMs ?? calcDurationMs(step.startedAt, step.completedAt);
    const durationStr = durationMs > 0 ? formatDuration(durationMs) : undefined;

    // 状态映射：running → running, 终态 → success, 否则 pending
    let status: StepStatus;
    if (isRunning) {
      status = "running";
    } else if (isTerminal) {
      status = "success";
    } else {
      status = "pending";
    }

    // 时间显示：运行中显示"进行中..."；终态显示耗时；否则空
    const timeStr = isRunning ? t("tasks.timelineInProgress") : isTerminal ? (durationStr ?? t("tasks.timelineDone")) : "";

    // 展开详情
    const expandDetail: UnifiedTimelineEntry["expandDetail"] = {};
    let hasExpand = false;
    if (step.startedAt) {
      expandDetail.startedAt = formatDateTime(step.startedAt);
      hasExpand = true;
    }
    if (step.completedAt) {
      expandDetail.completedAt = formatDateTime(step.completedAt);
      hasExpand = true;
    }
    if (durationStr) {
      expandDetail.duration = durationStr;
      hasExpand = true;
    }
    if (step.error) {
      expandDetail.error = step.error;
      hasExpand = true;
    }

    result.push({
      id: `phase-${phaseStr}-${step.id}`,
      phase: phaseEnum,
      label: getPhaseLabel(phaseStr),
      time: timeStr,
      duration: durationStr,
      progress: step.progress,
      speed: step.speed,
      eta: step.eta,
      status,
      isCurrent: isRunning,
      hasExpandableDetail: hasExpand,
      expandDetail: hasExpand ? expandDetail : undefined,
      _durationMs: durationMs,
    });
  }

  // 3. "Completed" 事件（若 completedAt 存在）
  if (step.completedAt) {
    const durationMs = step.durationMs ?? calcDurationMs(step.startedAt, step.completedAt);
    const durationStr = durationMs > 0 ? formatDuration(durationMs) : undefined;

    result.push({
      id: `completed-${step.id}`,
      phase: Phase.Completed,
      label: t("tasks.phaseCompleted"),
      time: formatDateTime(step.completedAt),
      duration: durationStr,
      status: "success",
      isCurrent: false,
      hasExpandableDetail: false,
      _durationMs: durationMs,
    });
  }

  // 4. 失败/取消态：标记最后一个事件为 failure，附加 error 信息
  if (FAILURE_STATUS.has(step.status)) {
    const last = result[result.length - 1];
    if (last) {
      last.status = "failure";
      if (step.error) {
        last.hasExpandableDetail = true;
        last.expandDetail = {
          ...(last.expandDetail ?? {}),
          error: step.error,
        };
      }
    }
  }

  // 5. 计算最长耗时 phase 并高亮
  let maxDurationMs = 0;
  let maxEntryId: string | null = null;
  for (const entry of result) {
    if (entry._durationMs && entry._durationMs > maxDurationMs) {
      maxDurationMs = entry._durationMs;
      maxEntryId = entry.id;
    }
  }
  if (maxEntryId) {
    const maxEntry = result.find(e => e.id === maxEntryId);
    if (maxEntry) maxEntry.isHighlight = true;
  }

  // 6. 剥离内部字段，返回 UnifiedTimelineEntry[]
  return result.map(entry => {
    const { _durationMs: _ignored, ...rest } = entry;
    void _ignored;
    return rest;
  });
});
</script>

<style scoped>
.step-inline-timeline {
  margin: 4px 0 8px;
}

.step-inline-timeline__list {
  position: relative;
  padding-left: 4px;
}

.step-inline-timeline__empty {
  padding: 8px 12px;
  font-size: 12px;
  color: var(--ion-color-medium, #92949c);
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
}

/* ==================== 暗黑模式适配（body.dark） ==================== */
:global(body.dark) .step-inline-timeline__empty {
  color: rgba(255, 255, 255, 0.5);
}
</style>
