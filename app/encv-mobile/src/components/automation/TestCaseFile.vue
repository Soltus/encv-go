<template>
  <div class="casefile" :class="`casefile--${result.status}`">
    <!-- 头部：case 编号 + 状态印章 -->
    <header class="casefile__head" @click="toggle">
      <div class="casefile__head-l">
        <div class="casefile__case-num">CASE №{{ paddedIdx }}</div>
        <div class="casefile__id">{{ result.spec.id }}</div>
        <div class="casefile__meta">
          <span class="casefile__meta-cell">
            <span class="casefile__meta-l">PLUGIN</span>
            <span class="casefile__meta-v">{{ result.spec.pluginName }}</span>
          </span>
          <span class="casefile__meta-cell">
            <span class="casefile__meta-l">TASK</span>
            <span class="casefile__meta-v">{{ result.spec.taskType }}</span>
          </span>
          <span class="casefile__meta-cell">
            <span class="casefile__meta-l">VER</span>
            <span class="casefile__meta-v">v{{ result.spec.version }}</span>
          </span>
          <!-- 加密选型：cipher mode -->
          <span v-if="result.spec.cipherMode !== undefined && result.spec.cipherMode !== null" class="casefile__meta-cell casefile__meta-cell--accent">
            <span class="casefile__meta-l">CIPHER</span>
            <span class="casefile__meta-v">
              {{ result.spec.cipherMode === 0 ? 'AES-128-GCM' : 'AES-256-GCM' }}
            </span>
          </span>
          <!-- 加密选型：compression mode -->
          <span v-if="result.spec.compressionMode" class="casefile__meta-cell casefile__meta-cell--accent">
            <span class="casefile__meta-l">COMPRESS</span>
            <span class="casefile__meta-v casefile__meta-v--compress">
              {{ result.spec.compressionMode.toUpperCase() }}
            </span>
          </span>
          <span v-if="result.durationMs" class="casefile__meta-cell">
            <span class="casefile__meta-l">DUR</span>
            <span class="casefile__meta-v">{{ result.durationMs }}ms</span>
          </span>
        </div>
      </div>
      <div class="casefile__head-r">
        <div class="casefile__stamp" :class="`casefile__stamp--${result.status}`">
          {{ statusText }}
        </div>
        <div v-if="result.spec.expectedBehavior === 'might-fail'" class="casefile__expected">
          MIGHT-FAIL
        </div>
        <div v-if="isExpandable" class="casefile__caret" :class="{ 'casefile__caret--open': expanded }">
          ▾
        </div>
      </div>
    </header>

    <!-- 折叠的详情 -->
    <div v-if="expanded" class="casefile__body">
      <!-- 错误分类 + 技术说明 -->
      <section v-if="result.errorAnalysis" class="casefile__section">
        <h4 class="casefile__section-title">
          <span class="casefile__section-num">§1</span> DIAGNOSIS
        </h4>
        <div class="casefile__diag">
          <div class="casefile__cat" :style="{ borderColor: catMeta.color, color: catMeta.color }">
            <span class="casefile__cat-label">CATEGORY</span>
            <span class="casefile__cat-val">{{ catMeta.label }}</span>
          </div>
          <div class="casefile__cat" :style="{ borderColor: '#1A1A1A', color: '#1A1A1A' }">
            <span class="casefile__cat-label">PHASE</span>
            <span class="casefile__cat-val">{{ result.errorAnalysis.phase.toUpperCase() }}</span>
          </div>
        </div>
        <p class="casefile__summary">
          <em>{{ result.errorAnalysis.summary }}</em>
        </p>
        <p class="casefile__tech">{{ result.errorAnalysis.technicalExplanation }}</p>
      </section>

      <!-- 错误链路 -->
      <section v-if="result.errorAnalysis" class="casefile__section">
        <h4 class="casefile__section-title">
          <span class="casefile__section-num">§2</span> ERROR CHAIN
        </h4>
        <ErrorChainNode :chain="result.errorAnalysis.chain" />
      </section>

      <!-- 修复建议 -->
      <section v-if="result.errorAnalysis && result.errorAnalysis.fixes.length" class="casefile__section">
        <h4 class="casefile__section-title">
          <span class="casefile__section-num">§3</span> REMEDIATION
        </h4>
        <ol class="casefile__fixes">
          <li v-for="(fix, i) in result.errorAnalysis.fixes" :key="i" class="casefile__fix">
            <div class="casefile__fix-num">{{ String(i + 1).padStart(2, '0') }}</div>
            <div class="casefile__fix-body">
              <div class="casefile__fix-title">{{ fix.title }}</div>
              <div class="casefile__fix-detail">{{ fix.detail }}</div>
              <pre v-if="fix.codeHint" class="casefile__fix-code">{{ fix.codeHint }}</pre>
              <a v-if="fix.docUrl" :href="fix.docUrl" target="_blank" rel="noopener" class="casefile__fix-doc">
                {{ fix.docUrl }} ↗
              </a>
            </div>
          </li>
        </ol>
      </section>

      <!-- 提交快照 -->
      <section class="casefile__section">
        <h4 class="casefile__section-title">
          <span class="casefile__section-num">§4</span> SUBMITTED PAYLOAD
        </h4>
        <dl class="casefile__payload">
          <dt>SOURCE</dt>
          <dd class="casefile__path">{{ result.submittedSourcePath || result.spec.sourcePath }}</dd>
          <dt>SUBMITTED AT</dt>
          <dd class="casefile__mono">{{ result.submittedAt || '—' }}</dd>
          <dt>TASK ID</dt>
          <dd class="casefile__mono">{{ result.taskId || '— (not created)' }}</dd>
          <dt>EXPECTED</dt>
          <dd class="casefile__mono">{{ result.spec.expectedBehavior }}</dd>
        </dl>
      </section>

      <!-- 原始错误 -->
      <section v-if="result.error" class="casefile__section">
        <h4 class="casefile__section-title">
          <span class="casefile__section-num">§5</span> RAW ERROR
        </h4>
        <pre class="casefile__raw">{{ result.error }}</pre>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { CATEGORY_META } from "@/composables/useErrorAnalyzer";
import type { TestCaseResult } from "@/lib/workflow/types";
import { computed, ref } from "vue";

const props = defineProps<{
  result: TestCaseResult;
  index: number;
}>();

const expanded = ref(props.result.status === "failed" || props.result.status === "passed");

const _paddedIdx = computed(() => String(props.index + 1).padStart(3, "0"));

const _statusText = computed(() => {
  switch (props.result.status) {
    case "passed":
      return "VERIFIED";
    case "failed":
      return "REJECTED";
    case "skipped":
      return "OMITTED";
    case "running":
      return "IN PROGRESS";
    case "pending":
      return "QUEUED";
    default:
      return "—";
  }
});

const _isExpandable = computed(() => true);

const _catMeta = computed(() => {
  const cat = props.result.errorAnalysis?.category ?? "unknown";
  return CATEGORY_META[cat];
});

function _toggle() {
  expanded.value = !expanded.value;
}
</script>

<style scoped>
.casefile {
  background: #F4EFE6;
  color: #1A1A1A;
  border: 1px solid #D4C9B5;
  border-radius: 4px;
  margin: 0 16px 12px 16px;
  overflow: hidden;
  font-family: 'Times New Roman', Georgia, serif;
  position: relative;
}

.casefile--failed {
  border-color: #8B1E3F;
  border-width: 1.5px;
  box-shadow: inset 3px 0 0 #8B1E3F;
}
.casefile--passed {
  border-color: #1B4332;
  box-shadow: inset 3px 0 0 #1B4332;
}
.casefile--skipped {
  opacity: 0.65;
  border-style: dashed;
}
.casefile--running {
  border-color: #B8860B;
  box-shadow: inset 3px 0 0 #B8860B;
}
.casefile--pending {
  border-color: #C9BBA1;
}

/* head */
.casefile__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 16px;
  cursor: pointer;
  user-select: none;
  background: linear-gradient(180deg, #FAF6EE 0%, #F4EFE6 100%);
  transition: background 0.15s ease;
}
.casefile__head:hover {
  background: #FAF6EE;
}
.casefile__head:active {
  background: #EDE5D2;
}

.casefile__head-l {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.casefile__case-num {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 10px;
  letter-spacing: 0.2em;
  color: #6B5D4C;
}

.casefile__id {
  font-size: 16px;
  font-weight: 700;
  color: #1A1A1A;
  word-break: break-all;
  font-family: 'Times New Roman', Georgia, serif;
  letter-spacing: -0.01em;
}

.casefile__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  margin-top: 4px;
}

.casefile__meta-cell {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
}

.casefile__meta-l {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 8px;
  letter-spacing: 0.18em;
  color: #6B5D4C;
}

.casefile__meta-v {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  color: #1A1A1A;
  font-weight: 500;
}
/* 加密选型参数高亮 */
.casefile__meta-cell--accent {
  background: rgba(43, 58, 103, 0.06);
  border-radius: 3px;
  padding: 2px 6px !important;
  border: 1px solid rgba(43, 58, 103, 0.15);
}
.casefile__meta-cell--accent .casefile__meta-l { color: #2B3A67; }
.casefile__meta-cell--accent .casefile__meta-v { color: #2B3A67; font-weight: 700; }
.casefile__meta-v--compress {
  font-size: 9px;
  letter-spacing: 0.08em;
}

.casefile__head-r {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  flex-shrink: 0;
  margin-left: 12px;
}

.casefile__stamp {
  padding: 4px 10px;
  border: 2px solid currentColor;
  border-radius: 2px;
  font-family: 'Times New Roman', Georgia, serif;
  font-weight: 700;
  font-size: 10px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  transform: rotate(-2deg);
  white-space: nowrap;
}
.casefile__stamp--passed { color: #1B4332; }
.casefile__stamp--failed { color: #8B1E3F; }
.casefile__stamp--skipped { color: #6B5D4C; }
.casefile__stamp--running { color: #B8860B; }
.casefile__stamp--pending { color: #6B5D4C; }

.casefile__expected {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 8px;
  letter-spacing: 0.18em;
  color: #8B7355;
  border: 1px solid #8B7355;
  padding: 1px 5px;
  border-radius: 1px;
}

.casefile__caret {
  font-size: 14px;
  color: #6B5D4C;
  margin-top: 2px;
  transition: transform 0.2s ease;
}
.casefile__caret--open {
  transform: rotate(180deg);
}

/* body */
.casefile__body {
  border-top: 1px solid #D4C9B5;
  padding: 16px 16px 18px 16px;
  background: #FAF6EE;
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.casefile__section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.casefile__section-title {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 0;
  font-family: 'Times New Roman', Georgia, serif;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.24em;
  color: #1A1A1A;
  text-transform: uppercase;
  border-bottom: 1px solid #C9BBA1;
  padding-bottom: 4px;
}

.casefile__section-num {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  color: #8B1E3F;
  letter-spacing: 0.1em;
}

/* §1 diagnosis */
.casefile__diag {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.casefile__cat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 10px;
  border: 1.5px solid;
  border-radius: 2px;
  min-width: 80px;
}
.casefile__cat-label {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 8px;
  letter-spacing: 0.2em;
  opacity: 0.75;
}
.casefile__cat-val {
  font-family: 'Times New Roman', Georgia, serif;
  font-weight: 700;
  font-size: 14px;
  letter-spacing: 0.04em;
}

.casefile__summary {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: #1A1A1A;
  line-height: 1.3;
}
.casefile__tech {
  margin: 0;
  font-size: 13px;
  color: #4A3F2E;
  line-height: 1.5;
}

/* §3 fixes */
.casefile__fixes {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  counter-reset: fix;
}
.casefile__fix {
  display: grid;
  grid-template-columns: 32px 1fr;
  gap: 12px;
  background: #F4EFE6;
  border-left: 3px solid #2B3A67;
  border-radius: 0 3px 3px 0;
  padding: 10px 12px;
}
.casefile__fix-num {
  font-family: 'Times New Roman', Georgia, serif;
  font-weight: 700;
  font-size: 22px;
  color: #2B3A67;
  line-height: 1;
}
.casefile__fix-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.casefile__fix-title {
  font-size: 14px;
  font-weight: 600;
  color: #2B3A67;
  letter-spacing: 0.01em;
}
.casefile__fix-detail {
  font-size: 13px;
  color: #1A1A1A;
  line-height: 1.5;
}
.casefile__fix-code {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  background: #1A1A1A;
  color: #E0D5BD;
  padding: 8px 10px;
  border-radius: 3px;
  margin: 4px 0 0 0;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.4;
}
.casefile__fix-doc {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  color: #2B3A67;
  text-decoration: underline;
  word-break: break-all;
}

/* §4 payload */
.casefile__payload {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 6px 16px;
  margin: 0;
  font-size: 12px;
}
.casefile__payload dt {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  letter-spacing: 0.2em;
  color: #6B5D4C;
  padding-top: 2px;
}
.casefile__payload dd {
  margin: 0;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  color: #1A1A1A;
  word-break: break-all;
}
.casefile__mono { font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; }
.casefile__path {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  color: #1A1A1A;
  background: rgba(26, 26, 26, 0.04);
  padding: 3px 6px;
  border-radius: 2px;
  word-break: break-all;
}

/* §5 raw */
.casefile__raw {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  background: #1A1A1A;
  color: #E0D5BD;
  padding: 10px 12px;
  border-radius: 3px;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.5;
  max-height: 200px;
  overflow-y: auto;
}
</style>
