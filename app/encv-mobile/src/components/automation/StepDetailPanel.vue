<template>
  <div class="panel" v-if="stepRun">
    <!-- 头部 -->
    <header class="panel__head">
      <span class="panel__step-name">{{ stepName }}</span>
      <StepMiniBadge :status="stepRun.status" :show-name="false" />
    </header>

    <!-- §1 Diagnosis（仅失败时） -->
    <section v-if="stepRun.errorAnalysis" class="panel__section">
      <h4 class="panel__section-title">§1 DIAGNOSIS</h4>
      <div class="panel__diag-row">
        <span class="panel__cat-tag">{{ stepRun.errorAnalysis.category.toUpperCase() }}</span>
        <span class="panel__phase-tag">PHASE: {{ stepRun.errorAnalysis.phase.toUpperCase() }}</span>
      </div>
      <p class="panel__summary"><em>{{ stepRun.errorAnalysis.summary }}</em></p>
      <p class="panel__tech">{{ stepRun.errorAnalysis.technicalExplanation }}</p>
    </section>

    <!-- §2 Error Chain -->
    <section v-if="stepRun.errorAnalysis?.chain.length" class="panel__section">
      <h4 class="panel__section-title">§2 ERROR CHAIN</h4>
      <ErrorChainNode :chain="stepRun.errorAnalysis.chain" />
    </section>

    <!-- §3 Remediation -->
    <section v-if="stepRun.errorAnalysis?.fixes.length" class="panel__section">
      <h4 class="panel__section-title">§3 REMEDIATION</h4>
      <ol class="panel__fixes">
        <li v-for="(fix, i) in stepRun.errorAnalysis.fixes" :key="i" class="panel__fix">
          <div class="panel__fix-num">{{ i + 1 }}</div>
          <div class="panel__fix-body">
            <div class="panel__fix-title">{{ fix.title }}</div>
            <div class="panel__fix-detail">{{ fix.detail }}</div>
            <pre v-if="fix.codeHint" class="panel__fix-code">{{ fix.codeHint }}</pre>
          </div>
        </li>
      </ol>
    </section>

    <!-- §4 Metadata -->
    <section class="panel__section">
      <h4 class="panel__section-title">§4 METADATA</h4>
      <dl class="panel__meta">
        <dt>STEP ID</dt><dd>{{ stepRun.id }}</dd>
        <dt>TASK ID</dt><dd>{{ stepRun.taskId ?? '—' }}</dd>
        <dt>STATUS</dt><dd>{{ stepRun.status.toUpperCase() }}</dd>
        <dt>DURATION</dt><dd>{{ formatDur(stepRun.durationMs) }}</dd>
      </dl>

      <!-- 加密选型参数（从 matrixVars 或推断） -->
      <div v-if="hasEncryptionParams" class="panel__enc-params">
        <h5 class="panel__enc-title">ENCRYPTION PARAMETERS</h5>
        <div class="panel__enc-grid">
          <div v-if="encryptionParams.cipher" class="panel__enc-cell panel__enc-cell--accent">
            <span class="panel__enc-l">CIPHER</span>
            <span class="panel__enc-v">{{ encryptionParams.cipher }}</span>
          </div>
          <div v-if="encryptionParams.compress" class="panel__enc-cell panel__enc-cell--accent">
            <span class="panel__enc-l">COMPRESSION</span>
            <span class="panel__enc-v">{{ encryptionParams.compress }}</span>
          </div>
          <div v-if="encryptionParams.version" class="panel__enc-cell">
            <span class="panel__enc-l">VERSION</span>
            <span class="panel__enc-v">v{{ encryptionParams.version }}</span>
          </div>
          <div v-if="encryptionParams.plugin" class="panel__enc-cell">
            <span class="panel__enc-l">PLUGIN</span>
            <span class="panel__enc-v">{{ encryptionParams.plugin }}</span>
          </div>
        </div>
      </div>

      <dl class="panel__meta" v-if="stepRun.matrixVars && Object.keys(stepRun.matrixVars).length > 0">
        <dt>MATRIX VARS</dt><dd><code>{{ JSON.stringify(stepRun.matrixVars) }}</code></dd>
      </dl>
    </section>

    <!-- §5 Raw Error -->
    <section v-if="stepRun.error" class="panel__section">
      <h4 class="panel__section-title">§5 RAW ERROR</h4>
      <pre class="panel__raw">{{ stepRun.error }}</pre>
    </section>

    <!-- Job context -->
    <section class="panel__section panel__section--job">
      <h4 class="panel__section-title">JOB CONTEXT</h4>
      <dl class="panel__meta">
        <dt>JOB</dt><dd>{{ jobRun.jobDefId }}</dd>
        <dt>CONCLUSION</dt><dd>{{ jobRun.conclusion ?? '—' }}</dd>
        <dt>JOB STEPS</dt><dd>{{ completedInJob }}/{{ jobRun.steps.length }}</dd>
      </dl>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { JobRun, StepRun } from "@/lib/workflow/types";
import { computed } from "vue";

const props = defineProps<{
  stepRun: StepRun;
  jobRun: JobRun;
}>();

const _stepName = computed(() => props.stepRun.stepDefId);

/** 从 matrixVars 或 stepDefId 推断加密选型参数 */
const encryptionParams = computed(() => {
  const vars = props.stepRun.matrixVars;
  const id = props.stepRun.stepDefId;

  // 优先从 matrixVars 获取
  if (vars) {
    return {
      cipher: vars.cipher ? (vars.cipher === "0" ? "AES-128-GCM" : vars.cipher === "1" ? "AES-256-GCM" : `c${vars.cipher}`) : undefined,
      compress: vars.compress ? String(vars.compress).toUpperCase() : undefined,
      version: undefined, // version 通常不在 matrix 中
      plugin: vars.plugin ?? undefined,
    };
  }

  // 从 stepDefId 解析：格式 "plugin-taskType-vN-cN-compress"
  const parts = id.split("-");
  const result: Record<string, string | number | undefined> = {};
  for (let i = 0; i < parts.length; i++) {
    if (parts[i].startsWith("v") && /^\d+$/.test(parts[i].slice(1))) {
      result.version = Number(parts[i].slice(1));
    } else if (parts[i] === "0" || parts[i] === "1") {
      // cipher mode
      result.cipher = parts[i] === "0" ? "AES-128-GCM" : "AES-256-GCM";
    } else if (["none", "zstd"].includes(parts[i])) {
      result.compress = parts[i].toUpperCase();
    }
  }

  // plugin name 是第一段
  if (parts.length > 0 && !/^(encrypt|decrypt|v\d|c\d|none|zstd)$/.test(parts[0])) {
    result.plugin = parts[0];
  }

  return result;
});

const _hasEncryptionParams = computed(() => {
  const p = encryptionParams.value;
  return !!(p.cipher || p.compress || p.version || p.plugin);
});

const _completedInJob = computed(
  () => props.jobRun.steps.filter(s => ["success", "failure", "cancelled", "skipped", "timed_out"].includes(s.status)).length
);

function _formatDur(ms?: number): string {
  if (!ms) return "—";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.round((ms % 60000) / 1000)}s`;
}
</script>

<style scoped>
.panel {
  background: #F4EFE6;
  border: 1px solid #D4C9B5;
  border-radius: 4px;
  margin: 0 16px 12px;
  font-family: 'Times New Roman', Georgia, serif;
}

.panel__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: linear-gradient(180deg, #FAF6EE, #F4EFE6);
  border-bottom: 1px solid #D4C9B5;
}
.panel__step-name {
  font-weight: 700;
  font-size: 15px;
  color: #1A1A1A;
}

.panel__section {
  padding: 14px 16px;
  border-bottom: 1px solid #EDE5D2;
}
.panel__section:last-child { border-bottom: none; }
.panel__section--job { background: rgba(43, 58, 103, 0.03); }

.panel__section-title {
  margin: 0 0 8px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.24em;
  text-transform: uppercase;
  color: #1A1A1A;
  font-family: 'Times New Roman', Georgia, serif;
}

/* §1 */
.panel__diag-row {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}
.panel__cat-tag {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  letter-spacing: 0.15em;
  color: #8B1E3F;
  background: rgba(139, 30, 63, 0.08);
  padding: 2px 6px;
  border-radius: 2px;
}
.panel__phase-tag {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  letter-spacing: 0.15em;
  color: #6B5D4C;
  background: rgba(107, 93, 76, 0.08);
  padding: 2px 6px;
  border-radius: 2px;
}
.panel__summary {
  margin: 0 0 4px;
  font-size: 14px;
  font-weight: 600;
  color: #1A1A1A;
}
.panel__tech {
  margin: 0;
  font-size: 13px;
  color: #4A3F2E;
  line-height: 1.5;
}

/* §3 Fixes */
.panel__fixes {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.panel__fix {
  display: grid;
  grid-template-columns: 28px 1fr;
  gap: 10px;
  background: #FAF6EE;
  border-left: 3px solid #2B3A67;
  border-radius: 0 3px 3px 0;
  padding: 8px 10px;
}
.panel__fix-num {
  font-family: 'Times New Roman', Georgia, serif;
  font-weight: 700;
  font-size: 20px;
  color: #2B3A67;
  line-height: 1;
}
.panel__fix-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.panel__fix-title {
  font-size: 13px;
  font-weight: 600;
  color: #2B3A67;
}
.panel__fix-detail {
  font-size: 12px;
  color: #1A1A1A;
  line-height: 1.45;
}
.panel__fix-code {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  background: #1A1A1A;
  color: #E0D5BD;
  padding: 6px 8px;
  border-radius: 3px;
  margin-top: 2px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.35;
}

/* §4 Metadata */
.panel__meta { display: grid; grid-template-columns: max-content 1fr; gap: 4px 12px; margin: 0; font-size: 11px; }
.panel__meta dt { font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; font-size: 8px; letter-spacing: 0.18em; color: #6B5D4C; }
.panel__meta dd { margin: 0; font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; font-size: 11px; color: #1A1A1A; word-break: break-all; }
.panel__meta code { background: rgba(26, 26, 26, 0.04); padding: 1px 4px; border-radius: 2px; font-size: 10px; }

/* 加密选型参数区块 */
.panel__enc-params {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed #C9BBA1;
}
.panel__enc-title {
  margin: 0 0 6px;
  font-size: 9px;
  letter-spacing: 0.2em;
  color: #2B3A67;
  text-transform: uppercase;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
}
.panel__enc-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.panel__enc-cell {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 4px 8px;
  background: #FAF6EE;
  border-radius: 3px;
  border: 1px solid #EDE5D2;
}
.panel__enc-cell--accent {
  background: rgba(43, 58, 103, 0.05);
  border-color: rgba(43, 58, 103, 0.2);
}
.panel__enc-l {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 7px;
  letter-spacing: 0.18em;
  color: #6B5D4C;
}
.panel__enc-v {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  color: #1A1A1A;
}
.panel__enc-cell--accent .panel__enc-v { color: #2B3A67; }

/* §5 Raw */
.panel__raw {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  background: #1A1A1A;
  color: #E0D5BD;
  padding: 8px 10px;
  border-radius: 3px;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.45;
  max-height: 180px;
  overflow-y: auto;
}
</style>
