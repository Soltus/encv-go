<template>
  <div class="dossier" :class="{ 'dossier--all-passed': passed === total, 'dossier--all-failed': failed === total && total > 0 }">
    <!-- Top eyebrow -->
    <div class="dossier__eyebrow">
      <span class="dossier__eyebrow-line">— AUTOMATED TEST DOSSIER —</span>
    </div>

    <!-- Title row -->
    <div class="dossier__title">
      <div class="dossier__case-num">DOSSIER №{{ runId }}</div>
      <div class="dossier__verdict-stamp" :class="verdictClass">
        <span>{{ verdictText }}</span>
      </div>
    </div>

    <!-- Metadata grid -->
    <div class="dossier__meta">
      <div class="meta-cell">
        <div class="meta-cell__label">OPENED</div>
        <div class="meta-cell__value">{{ formattedOpenedAt }}</div>
      </div>
      <div class="meta-cell">
        <div class="meta-cell__label">DURATION</div>
        <div class="meta-cell__value">{{ formattedDuration }}</div>
      </div>
      <div class="meta-cell">
        <div class="meta-cell__label">PLATFORM</div>
        <div class="meta-cell__value">{{ platform }}</div>
      </div>
      <div class="meta-cell">
        <div class="meta-cell__label">EXAMINER</div>
        <div class="meta-cell__value">encv-automation</div>
      </div>
    </div>

    <!-- Tallies -->
    <div class="dossier__tallies">
      <div class="tally tally--total">
        <div class="tally__num">{{ total }}</div>
        <div class="tally__label">EXAMINED</div>
      </div>
      <div class="tally tally--pass" :class="{ 'tally--active': passed > 0 }">
        <div class="tally__num">{{ passed }}</div>
        <div class="tally__label">PASSED</div>
      </div>
      <div class="tally tally--fail" :class="{ 'tally--active': failed > 0 }">
        <div class="tally__num">{{ failed }}</div>
        <div class="tally__label">FAILED</div>
      </div>
      <div v-if="pending > 0" class="tally tally--pending" :class="{ 'tally--active': true }">
        <div class="tally__num">{{ pending }}</div>
        <div class="tally__label">PENDING</div>
      </div>
      <div class="tally tally--skip" v-if="skipped > 0">
        <div class="tally__num">{{ skipped }}</div>
        <div class="tally__label">SKIPPED</div>
      </div>
    </div>

    <!-- 🆕 2026-06-10 增强运行感：总进度条 + IN PROGRESS pulse -->
    <div class="dossier__progress" :class="{ 'dossier__progress--running': pending > 0 }">
      <div class="dossier__progress-label">
        <span class="dossier__progress-text">
          <span v-if="pending > 0" class="dossier__progress-spinner" aria-hidden="true">◉</span>
          {{ completedText }}
        </span>
        <span class="dossier__progress-pct">{{ totalProgressPct }}%</span>
      </div>
      <div class="dossier__progress-track">
        <div class="dossier__progress-fill" :style="{ width: totalProgressPct + '%' }"></div>
      </div>
    </div>

    <!-- Pass rate bar -->
    <div class="dossier__bar">
      <div class="dossier__bar-label">
        PASS RATE <span class="dossier__bar-pct">{{ passRate }}%</span>
      </div>
      <div class="dossier__bar-track">
        <div class="dossier__bar-fill" :style="{ width: passRate + '%' }" :class="{ 'dossier__bar-fill--low': passRate < 50 }"></div>
      </div>
    </div>

    <!-- Subtitle / tag line -->
    <div v-if="failed > 0" class="dossier__subtitle">
      <em>{{ verdictSubtitle }}</em>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  runId: string;
  openedAt: string;
  durationMs: number;
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  pending: number;
  platform: string;
}>();

const _formattedOpenedAt = computed(() => {
  if (!props.openedAt) return "—";
  const d = new Date(props.openedAt);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
});

const _formattedDuration = computed(() => {
  if (props.durationMs < 1000) return `${props.durationMs}ms`;
  if (props.durationMs < 60_000) return `${(props.durationMs / 1000).toFixed(1)}s`;
  return `${Math.floor(props.durationMs / 60_000)}m ${Math.floor((props.durationMs % 60_000) / 1000)}s`;
});

const _passRate = computed(() => {
  // 只统计已完成的任务（排除 pending）
  const finished = props.passed + props.failed;
  if (finished === 0) return 0;
  return Math.round((props.passed / finished) * 100);
});

// 🆕 2026-06-10 增强运行感：总进度条（completed / total）— 用户能看到
// 整个 run 的推进节奏，而不是只看 pass rate
const _totalProgressPct = computed(() => {
  if (props.total === 0) return 0;
  const finished = props.passed + props.failed + (props.skipped ?? 0);
  return Math.round((finished / props.total) * 100);
});
const _completedText = computed(() => {
  const finished = props.passed + props.failed + (props.skipped ?? 0);
  if (props.pending > 0) return `${finished} / ${props.total} EXECUTED`;
  return `${finished} / ${props.total} DONE`;
});

const _verdictClass = computed(() => {
  if (props.total === 0) return "";
  // 有 pending 时，即使全部通过也显示 partial（因为还没跑完）
  if (props.pending > 0) return "verdict-stamp--partial";
  if (props.failed === 0) return "verdict-stamp--pass";
  if (props.passed === 0) return "verdict-stamp--fail";
  return "verdict-stamp--partial";
});

const _verdictText = computed(() => {
  if (props.total === 0) return "NO DATA";
  if (props.pending > 0) return "IN PROGRESS";
  if (props.failed === 0) return "VERIFIED";
  if (props.passed === 0) return "REJECTED";
  return "PARTIAL";
});

const _verdictSubtitle = computed(() => {
  if (props.failed === 0) return "";
  if (props.passed === 0) return "All examinations failed. Review the case files below for diagnosis.";
  return `${props.failed} examination${props.failed === 1 ? "" : "s"} failed. Inspect individual case files for error chain and remediation.`;
});
</script>

<style scoped>
.dossier {
  background: #F4EFE6;
  color: #1A1A1A;
  border: 1px solid #D4C9B5;
  border-radius: 4px;
  padding: 24px 20px 20px;
  margin: 16px;
  position: relative;
  overflow: hidden;
  font-family: 'Times New Roman', Georgia, serif;
}

/* Paper texture using subtle gradient */
.dossier::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    radial-gradient(circle at 20% 10%, rgba(139, 110, 70, 0.04) 0%, transparent 50%),
    radial-gradient(circle at 80% 90%, rgba(139, 30, 63, 0.03) 0%, transparent 50%);
  pointer-events: none;
}

.dossier__eyebrow {
  text-align: center;
  margin-bottom: 12px;
}
.dossier__eyebrow-line {
  font-family: 'Times New Roman', Georgia, serif;
  font-size: 10px;
  letter-spacing: 0.32em;
  color: #6B5D4C;
  text-transform: uppercase;
}

.dossier__title {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  padding-bottom: 16px;
  border-bottom: 1.5px solid #1A1A1A;
  margin-bottom: 16px;
}

.dossier__case-num {
  font-size: 24px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: #1A1A1A;
}

.dossier__verdict-stamp {
  display: inline-block;
  padding: 6px 14px;
  border: 2.5px solid currentColor;
  font-family: 'Times New Roman', Georgia, serif;
  font-weight: 700;
  font-size: 13px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  transform: rotate(-2.5deg);
  border-radius: 3px;
}
.verdict-stamp--pass { color: #1B4332; }
.verdict-stamp--fail { color: #8B1E3F; }
.verdict-stamp--partial { color: #B8860B; }

.dossier__meta {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px 16px;
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid #C9BBA1;
}
.meta-cell__label {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  letter-spacing: 0.2em;
  color: #6B5D4C;
  margin-bottom: 2px;
}
.meta-cell__value {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  color: #1A1A1A;
  font-weight: 500;
}

.dossier__tallies {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin-bottom: 18px;
}
.tally {
  text-align: center;
  padding: 10px 4px;
  background: #FAF6EE;
  border: 1px solid #D4C9B5;
  border-radius: 2px;
}
.tally__num {
  font-size: 24px;
  font-weight: 700;
  font-family: 'Times New Roman', Georgia, serif;
  line-height: 1;
}
.tally__label {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 8px;
  letter-spacing: 0.18em;
  color: #6B5D4C;
  margin-top: 4px;
}
.tally--total .tally__num { color: #1A1A1A; }
.tally--pass .tally__num { color: #1B4332; }
.tally--fail .tally__num { color: #5B0F1F; }
.tally--pending .tally__num { color: #B8860B; }
.tally--skip .tally__num { color: #6B5D4C; }
.tally--active {
  background: #FAEAEC;
  border-color: #8B1E3F;
}
.tally--pending.tally--active {
  background: #FFF8E1;
  border-color: #B8860B;
}

.dossier__bar {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed #C9BBA1;
}

/* 🆕 2026-06-10 总进度条 — 增强运行感 */
.dossier__progress {
  margin-bottom: 14px;
  padding-bottom: 14px;
  border-bottom: 1px dashed #C9BBA1;
}
.dossier__progress-label {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 10px;
  letter-spacing: 0.18em;
  color: #6B5D4C;
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.dossier__progress-pct {
  font-weight: 700;
  color: #1A1A1A;
}
.dossier__progress-text {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.dossier__progress-spinner {
  color: #1565C0;
  font-size: 12px;
  animation: progress-pulse 1.4s ease-in-out infinite;
}
.dossier__progress-track {
  height: 5px;
  background: #E5DCC8;
  border-radius: 3px;
  overflow: hidden;
}
.dossier__progress-fill {
  height: 100%;
  background: #1B4332;
  transition: width 0.4s ease;
}
.dossier__progress--running .dossier__progress-fill {
  background: #1565C0;
  animation: progress-shimmer 1.6s linear infinite;
}
@keyframes progress-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.85); }
}
@keyframes progress-shimmer {
  0% { box-shadow: inset 0 0 0 0 rgba(255,255,255,0); }
  50% { box-shadow: inset 0 0 8px 1px rgba(255,255,255,0.35); }
  100% { box-shadow: inset 0 0 0 0 rgba(255,255,255,0); }
}
.dossier__bar-label {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 10px;
  letter-spacing: 0.18em;
  color: #6B5D4C;
  display: flex;
  justify-content: space-between;
  margin-bottom: 6px;
}
.dossier__bar-pct {
  font-weight: 700;
  color: #1A1A1A;
}
.dossier__bar-track {
  height: 4px;
  background: #E5DCC8;
  border-radius: 2px;
  overflow: hidden;
}
.dossier__bar-fill {
  height: 100%;
  background: #1B4332;
  transition: width 0.4s ease;
}
.dossier__bar-fill--low { background: #8B1E3F; }

.dossier__subtitle {
  margin-top: 14px;
  font-size: 13px;
  color: #4A3F2E;
  line-height: 1.4;
  text-align: center;
}
</style>
