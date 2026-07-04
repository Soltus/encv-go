<!--
  ApprovalCard - 审批卡（4 决策按钮组）
  参照 codex_web ApprovalCard
  4 按钮顺序固定：批准 / 本轮批准 / 拒绝 / 拒绝并停止
  按 Kind 选图标：command → TerminalSquare / fileChange → FileCode2 / readOnly → ShieldCheck / unknown → HelpCircle
  本轮批准 (accept_for_session) 仅在 toolCall.kind !== 'readOnly' 时显示
-->
<template>
  <div class="approvalCard">
    <!-- Header -->
    <div class="approvalHeader">
      <div class="approvalHeaderLeft">
        <ion-icon :icon="kindIcon" class="approvalKindIcon" />
        <div class="approvalHeaderText">
          <div class="approvalTitle">{{ titleText }}</div>
          <div v-if="reasonText" class="approvalReason">{{ reasonText }}</div>
        </div>
      </div>
    </div>

    <!-- Body: command / cwd / changedFiles / permissions 摘要 -->
    <div v-if="bodySummary.length > 0" class="approvalBody">
      <div v-for="row in bodySummary" :key="row.label" class="approvalBodyRow">
        <span class="approvalBodyLabel">{{ row.label }}</span>
        <span class="approvalBodyValue">{{ row.value }}</span>
      </div>
    </div>

    <!-- Files chips -->
    <div v-if="filesChips.length > 0" class="approvalFiles">
      <div v-for="path in filesChips" :key="path" class="approvalFileChip" :title="path">
        {{ truncatePath(path) }}
      </div>
      <div v-if="extraFilesCount > 0" class="approvalFileChip approvalFileChip_more">
        +{{ extraFilesCount }}
      </div>
    </div>

    <!-- Diff 区（fileChange） -->
    <div v-if="diffText" class="approvalDiff">
      <div class="approvalDiffHeader" @click="diffExpanded = !diffExpanded">
        <span>{{ diffExpanded ? t('agent.collapse') : t('agent.expand') }}</span>
        <ion-icon :icon="diffExpanded ? chevronUp : chevronDown" />
      </div>
      <pre v-if="diffExpanded" class="approvalDiffContent">{{ diffText }}</pre>
    </div>

    <!-- Actions: 4 决策按钮 -->
    <div class="approvalActions">
      <button
        type="button"
        class="approvalBtn approvalBtn_accept"
        :disabled="disabled"
        :class="{ approvalBtn_processing: processingDecision === 'accept' }"
        @click="handleDecide('accept')"
      >
        <span v-if="processingDecision === 'accept'" class="approvalBtnSpinner" />
        {{ t('modals.approve') }}
      </button>

      <button
        v-if="canShowSessionGrant"
        type="button"
        class="approvalBtn approvalBtn_acceptSession"
        :disabled="disabled"
        :class="{ approvalBtn_processing: processingDecision === 'accept_for_session' }"
        @click="handleDecide('accept_for_session')"
      >
        <span v-if="processingDecision === 'accept_for_session'" class="approvalBtnSpinner" />
        {{ t('modals.approveForSession') }}
      </button>

      <button
        type="button"
        class="approvalBtn approvalBtn_decline"
        :disabled="disabled"
        :class="{ approvalBtn_processing: processingDecision === 'decline' }"
        @click="handleDecide('decline')"
      >
        <span v-if="processingDecision === 'decline'" class="approvalBtnSpinner" />
        {{ t('modals.decline') }}
      </button>

      <button
        type="button"
        class="approvalBtn approvalBtn_cancel"
        :disabled="disabled"
        :class="{ approvalBtn_processing: processingDecision === 'cancel' }"
        @click="handleDecide('cancel')"
      >
        <span v-if="processingDecision === 'cancel'" class="approvalBtnSpinner" />
        {{ t('modals.cancel') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { IonIcon } from "@ionic/vue";
import {
  chevronDownOutline,
  chevronUpOutline,
  codeSlashOutline,
  helpCircleOutline,
  listOutline,
  searchOutline,
  shieldCheckmarkOutline,
  terminalOutline,
} from "ionicons/icons";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import type { Decision, ToolCall, ToolKind } from "@/composables/useAgent";
import { useI18n } from "@/composables/useI18n";

// 模板用 chevronUp/chevronDown 引用，必须从 import 别名重绑定，否则
// 模板引用未定义变量（vue-tsc 报 chevronUp/Down 不在 template scope）。
const chevronUp = chevronUpOutline;
const chevronDown = chevronDownOutline;

const props = defineProps<{
  toolCall: ToolCall;
  onDecide: (toolCallId: string, decision: Decision) => void;
  isProcessing: boolean;
}>();

const { t } = useI18n();
const processingDecision = ref<Decision | null>(null);
const diffExpanded = ref(false);
let safetyTimer: number | null = null;

const MAX_FILE_CHIPS = 6;

const kindIcon = computed(() => {
  const map: Record<ToolKind, typeof terminalOutline> = {
    command: terminalOutline,
    fileChange: codeSlashOutline,
    readOnly: shieldCheckmarkOutline,
    webSearch: searchOutline,
    plan: listOutline,
    unknown: helpCircleOutline,
  };
  return map[props.toolCall.kind] || helpCircleOutline;
});

const titleText = computed(() => {
  const kindMap: Record<ToolKind, string> = {
    command: t("agent.tool.command"),
    fileChange: t("agent.tool.fileChange"),
    readOnly: t("agent.tool.readOnly"),
    webSearch: t("agent.tool.webSearch"),
    plan: t("agent.plan"),
    unknown: t("agent.tool.unknown"),
  };
  const kindLabel = kindMap[props.toolCall.kind] || props.toolCall.kind;
  return `${kindLabel}：${props.toolCall.name}`;
});

const reasonText = computed(() => {
  // 解析 args 拿第一个 string 字段作为 "reason" 提示
  try {
    const parsed = JSON.parse(props.toolCall.args);
    if (parsed && typeof parsed === "object") {
      const reasonField = (parsed as Record<string, unknown>).reason;
      if (typeof reasonField === "string" && reasonField.trim()) return reasonField;
    }
  } catch {
    // ignore
  }
  return "";
});

interface SummaryRow {
  label: string;
  value: string;
}

const bodySummary = computed<SummaryRow[]>(() => {
  const rows: SummaryRow[] = [];
  let args: Record<string, unknown> = {};
  try {
    const parsed = JSON.parse(props.toolCall.args);
    if (parsed && typeof parsed === "object") args = parsed as Record<string, unknown>;
  } catch {
    // ignore
  }

  if (typeof args.command === "string" && args.command.trim()) {
    rows.push({ label: "Command", value: args.command });
  }
  if (typeof args.cwd === "string" && args.cwd.trim()) {
    rows.push({ label: "CWD", value: args.cwd });
  }
  if (Array.isArray(args.permissions) && args.permissions.length > 0) {
    rows.push({ label: "Permissions", value: (args.permissions as unknown[]).map(String).join(", ") });
  }
  return rows;
});

const filesChips = computed<string[]>(() => {
  let args: Record<string, unknown> = {};
  try {
    const parsed = JSON.parse(props.toolCall.args);
    if (parsed && typeof parsed === "object") args = parsed as Record<string, unknown>;
  } catch {
    return [];
  }
  const candidates: string[] = [];
  if (Array.isArray(args.changedFiles)) {
    for (const f of args.changedFiles) {
      if (typeof f === "string") candidates.push(f);
      else if (f && typeof f === "object" && typeof (f as Record<string, unknown>).path === "string") {
        candidates.push((f as Record<string, string>).path);
      }
    }
  } else if (Array.isArray(args.input_paths)) {
    for (const p of args.input_paths) {
      if (typeof p === "string") candidates.push(p);
    }
  } else if (typeof args.path === "string") {
    candidates.push(args.path);
  }
  return candidates.slice(0, MAX_FILE_CHIPS);
});

const extraFilesCount = computed(() => {
  let args: Record<string, unknown> = {};
  try {
    const parsed = JSON.parse(props.toolCall.args);
    if (parsed && typeof parsed === "object") args = parsed as Record<string, unknown>;
  } catch {
    return 0;
  }
  let total = 0;
  if (Array.isArray(args.changedFiles)) total = args.changedFiles.length;
  else if (Array.isArray(args.input_paths)) total = args.input_paths.length;
  else if (typeof args.path === "string") total = 1;
  return Math.max(0, total - MAX_FILE_CHIPS);
});

const diffText = computed(() => {
  let args: Record<string, unknown> = {};
  try {
    const parsed = JSON.parse(props.toolCall.args);
    if (parsed && typeof parsed === "object") args = parsed as Record<string, unknown>;
  } catch {
    return "";
  }
  if (typeof args.diff === "string") return args.diff;
  if (typeof args.content === "string" && props.toolCall.kind === "fileChange") {
    return args.content;
  }
  return "";
});

const canShowSessionGrant = computed(() => props.toolCall.kind !== "readOnly");

const disabled = computed(() => props.isProcessing || processingDecision.value !== null);

/**
 * 当父组件通知流结束（isProcessing 变 false）时，
 * 同步清空本地 processingDecision，让按钮立即恢复可点击。
 *
 * 5s safetyTimer 保留作为挂起兜底：若 SSE 网络异常挂起导致 isProcessing 永远为 true，
 * 也能保证按钮最终恢复，避免永久 disabled 的 UX 陷阱。
 */
watch(
  () => props.isProcessing,
  (curr, prev) => {
    if (prev === true && curr === false) {
      processingDecision.value = null;
      if (safetyTimer !== null) {
        window.clearTimeout(safetyTimer);
        safetyTimer = null;
      }
    }
  }
);

onBeforeUnmount(() => {
  if (safetyTimer !== null) {
    window.clearTimeout(safetyTimer);
    safetyTimer = null;
  }
});

function handleDecide(decision: Decision) {
  if (disabled.value) return;
  processingDecision.value = decision;
  try {
    props.onDecide(props.toolCall.id, decision);
  } finally {
    // safetyTimer：网络挂起兜底 5s 强制清空，防止按钮永久 disabled
    if (safetyTimer !== null) window.clearTimeout(safetyTimer);
    safetyTimer = window.setTimeout(() => {
      if (processingDecision.value === decision) processingDecision.value = null;
      safetyTimer = null;
    }, 5000);
  }
}

function truncatePath(p: string): string {
  if (p.length <= 28) return p;
  return "…" + p.slice(p.length - 27);
}
</script>

<style scoped>
.approvalCard {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  background: rgba(255, 196, 9, 0.08);
  border: 1px solid rgba(255, 196, 9, 0.32);
  border-radius: 10px;
  margin: 8px 0;
}

.approvalHeader {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.approvalHeaderLeft {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.approvalKindIcon {
  font-size: 18px;
  color: var(--ion-color-warning-shade, #e0ac08);
  flex-shrink: 0;
}

.approvalHeaderText {
  min-width: 0;
}

.approvalTitle {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--ion-text-color);
  word-break: break-word;
}

.approvalReason {
  font-size: 12px;
  color: var(--encv-text-secondary);
  margin-top: 2px;
  word-break: break-word;
}

.approvalBody {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px;
  background: rgba(var(--ion-background-color-rgb), 0.4);
  border-radius: 6px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.16);
}

.approvalBodyRow {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12px;
  line-height: 1.45;
}

.approvalBodyLabel {
  flex-shrink: 0;
  font-weight: 600;
  color: var(--encv-text-secondary);
  min-width: 70px;
}

.approvalBodyValue {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  color: var(--ion-text-color);
  word-break: break-all;
  white-space: pre-wrap;
}

.approvalFiles {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.approvalFileChip {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  color: var(--ion-color-primary);
  border-radius: 10px;
  font-size: 11px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.approvalFileChip_more {
  background: rgba(var(--ion-color-medium-rgb), 0.18);
  color: var(--encv-text-secondary);
}

.approvalDiff {
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.2);
  border-radius: 6px;
  background: rgba(var(--ion-background-color-rgb), 0.5);
  overflow: hidden;
}

.approvalDiffHeader {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  font-size: 11.5px;
  color: var(--encv-text-secondary);
  cursor: pointer;
  user-select: none;
}

.approvalDiffHeader ion-icon {
  font-size: 13px;
}

.approvalDiffContent {
  margin: 0;
  padding: 8px 12px;
  font-size: 11.5px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  line-height: 1.45;
  color: var(--ion-text-color);
  background: rgba(var(--ion-background-color-rgb), 0.5);
  max-height: 280px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.approvalActions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px;
}

.approvalBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid transparent;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
  transition: filter 0.15s, transform 0.05s, opacity 0.15s;
  min-height: 32px;
  line-height: 1.2;
}

.approvalBtn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.approvalBtn:active:not(:disabled) {
  transform: scale(0.97);
}

.approvalBtn_accept {
  grid-column: 1 / -1;
  background: var(--ion-color-primary);
  color: var(--ion-color-primary-contrast, #fff);
}

.approvalBtn_acceptSession {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  color: var(--ion-color-primary);
  border-color: rgba(var(--ion-color-primary-rgb), 0.3);
}

.approvalBtn_decline {
  background: rgba(var(--ion-color-medium-rgb), 0.18);
  color: var(--ion-text-color);
  border-color: rgba(var(--ion-color-medium-rgb), 0.3);
}

.approvalBtn_cancel {
  background: rgba(var(--ion-color-danger-rgb), 0.12);
  color: var(--ion-color-danger);
  border-color: rgba(var(--ion-color-danger-rgb), 0.3);
}

.approvalBtn_processing {
  filter: brightness(0.92);
}

.approvalBtnSpinner {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid currentColor;
  border-top-color: transparent;
  animation: approvalBtnSpin 0.7s linear infinite;
}

@keyframes approvalBtnSpin {
  to { transform: rotate(360deg); }
}
</style>
