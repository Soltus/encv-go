<!--
  OperationCard - 单条工具调用卡片（agent 流式时间轴模式）

  与 GroupedOperationMessage 不同：
  - GroupedOperationMessage：聚合 N 个 tool_call 到一个折叠 group（旧模式）
  - OperationCard：渲染**单个** tool_call，紧跟其 toolResultCard（新模式）

  设计要点：
  - 折叠态（默认）：单行图标+名称+状态badge，点击展开
  - 展开态：完整卡片（图标+名称+状态+参数+结果slot）
  - streaming 时强制展开（用户需看到实时进度）

  Task 4 增量：
  - 错误详情可折叠（status === 'failed' 时）
  - 「复制错误」按钮（status === 'failed' 时）
  - 卡片底部显示「耗时 1.2s」（基于 duration_ms / startedAt+finishedAt）
-->
<template>
  <div class="operationCard" :class="{
    operationCard_streaming: streaming,
    operationCard_collapsed: isCollapsed && !streaming,
    operationCard_failed: toolCall.status === 'failed',
  }">
    <!-- 折叠/展开 切换头部（始终显示） -->
    <div class="operationCardHead" @click="toggleCollapse">
      <ion-icon :icon="toolIcon" class="operationCardIcon" />
      <span class="operationCardName">{{ toolCall.name || t('agent.tool.unknown') }}</span>
      <span
        v-if="isV2Tool"
        class="operationCardV2Tag"
        :title="t('agent.v2BadgeTitle')"
      >{{ t('agent.v2Badge') }}</span>
      <StatusBadge
        :label="statusLabel"
        :tone="statusTone"
        :pulse="streaming || toolCall.status === 'running' || toolCall.status === 'pending'"
        class="operationCardBadge"
      />
      <ion-icon :icon="isCollapsed ? chevronDownOutline : chevronUpOutline" class="operationCardToggle" />
    </div>

    <!-- 展开内容（折叠时隐藏） -->
    <div v-show="!isCollapsed || streaming" class="operationCardBody">
      <div v-if="toolCall.args" class="operationCardArgs">
        <code>{{ truncateArgs(toolCall.args) }}</code>
      </div>
      <!-- ToolResultCard 插槽：AgentChat 按 name 分发到 MountListCard / FileListCard / FileContentCard -->
      <div v-if="$slots.result" class="operationCardResult">
        <slot name="result" />
      </div>

      <!-- Task 4：错误详情（仅 failed 状态显示） -->
      <div v-if="toolCall.status === 'failed' && toolCall.errorMessage" class="operationCardError">
        <div class="operationCardErrorHeader" @click.stop="toggleErrorDetails">
          <ion-icon :icon="alertCircleOutline" class="operationCardErrorIcon" />
          <span class="operationCardErrorLabel">
            {{ toolCall.errorCode ? `[${toolCall.errorCode}] ` : '' }}{{ toolCall.errorMessage }}
          </span>
          <ion-button
            v-if="toolCall.errorMessage"
            fill="clear"
            size="small"
            class="operationCardErrorCopyBtn"
            :title="t('agent.copy')"
            @click.stop="copyError"
          >
            <ion-icon :icon="copyOutline" slot="icon-only" />
          </ion-button>
          <ion-icon
            :icon="showErrorDetails ? chevronUpOutline : chevronDownOutline"
            class="operationCardErrorToggle"
          />
        </div>
        <div v-if="showErrorDetails" class="operationCardErrorDetails">
          <div v-if="toolCall.errorCode" class="operationCardErrorRow">
            <span class="operationCardErrorKey">code</span>
            <code class="operationCardErrorVal">{{ toolCall.errorCode }}</code>
          </div>
          <div class="operationCardErrorRow">
            <span class="operationCardErrorKey">message</span>
            <pre class="operationCardErrorVal">{{ toolCall.errorMessage }}</pre>
          </div>
          <div v-if="toolCall.output !== undefined" class="operationCardErrorRow">
            <span class="operationCardErrorKey">output</span>
            <pre class="operationCardErrorVal">{{ formatErrorOutput(toolCall.output) }}</pre>
          </div>
        </div>
      </div>

      <!-- Task 4：耗时（终态时显示） -->
      <div v-if="durationMs !== null" class="operationCardDuration">
        <span>{{ t('agent.toolDuration', { ms: formatDuration(durationMs) }) }}</span>
        <span v-if="durationMs > LONG_DURATION_MS" class="operationCardDurationWarn">
          {{ t('agent.toolDurationLong') }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ToolCall } from "@/composables/useAgent";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import {
  alertCircleOutline,
  chevronDownOutline,
  chevronUpOutline,
  copyOutline,
  documentTextOutline,
  ellipsisHorizontalCircleOutline,
  eyeOutline,
  searchOutline,
  terminalOutline,
} from "ionicons/icons";
import { computed, ref } from "vue";

const props = defineProps<{
  toolCall: ToolCall;
  /** 流式状态（running/pending 时显示脉冲动画 + 强制展开） */
  streaming?: boolean;
}>();

const { t } = useI18n();

// 折叠态：默认折叠（有内容时），streaming 强制展开
const isCollapsed = ref(true);
// 错误详情展开态：默认折叠
const showErrorDetails = ref(false);

function toggleCollapse() {
  if (!props.streaming) {
    isCollapsed.value = !isCollapsed.value;
  }
}

function toggleErrorDetails() {
  showErrorDetails.value = !showErrorDetails.value;
}

/**
 * 复制错误：拼接 errorCode + errorMessage，方便用户贴到 issue / 群聊。
 * 失败时弹 toast（不静默吞错）。
 */
async function copyError() {
  if (!props.toolCall.errorMessage) return;
  const text = props.toolCall.errorCode ? `[${props.toolCall.errorCode}] ${props.toolCall.errorMessage}` : props.toolCall.errorMessage;
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      // fallback：textarea + execCommand
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    showToast({ message: t("agent.copied"), color: "success" });
  } catch (e) {
    console.warn("[OperationCard] copy error failed:", e);
    showToast({ message: t("agent.copyFailed"), color: "danger" });
  }
}

/** 按 kind 选图标 */
const toolIcon = computed(() => {
  switch (props.toolCall.kind) {
    case "command":
      return terminalOutline;
    case "fileChange":
      return documentTextOutline;
    case "readOnly":
      return eyeOutline;
    case "webSearch":
      return searchOutline;
    default:
      return ellipsisHorizontalCircleOutline;
  }
});

/** ToolStatus → StatusBadge tone 映射 */
const statusTone = computed<"ready" | "warn" | "idle">(() => {
  switch (props.toolCall.status) {
    case "success":
      return "ready";
    case "failed":
    case "cancelled":
      return "warn";
    default:
      // pending / running / 其他 → idle（pulse 动画提示进行中）
      return "idle";
  }
});

/** ToolStatus → 状态文案（覆盖 raw 英文 tag，状态语义化） */
const statusLabel = computed(() => {
  switch (props.toolCall.status) {
    case "pending":
      return t("agent.toolStatusPending");
    case "running":
      return t("agent.toolStatusRunning");
    case "success":
      return t("agent.toolStatusSuccess");
    case "failed":
      return t("agent.toolStatusFailed");
    case "cancelled":
      return t("agent.toolStatusCancelled");
    default:
      return props.toolCall.status;
  }
});

/**
 * 计算耗时（毫秒）。优先级：
 *  1. 外部 tool_result.duration_ms（后端实测值，**最准**）
 *  2. 本地 finishedAt - startedAt（前端估算）
 *  3. finishedAt - Date.now() 的一个保守 fallback（不推荐，仅作为视觉占位）
 * 返回 null 表示「非终态 / 不可计算」，不显示耗时行。
 */
const durationMs = computed<number | null>(() => {
  const tc = props.toolCall;
  // 优先：用最近一条 tool_result 的 duration_ms（来自 useAgent.m.tool_results）
  // 这里用 props 没传 result，所以只能从 tc 自身找。
  // 实际上后端在 tool_result 中带了 duration_ms，但 ToolCall 类型没存。
  // 退而求其次：finishedAt - startedAt
  if (tc.status === "pending" || tc.status === "running") return null;
  if (tc.startedAt !== undefined && tc.finishedAt !== undefined) {
    return Math.max(0, tc.finishedAt - tc.startedAt);
  }
  // 单点 finishedAt 但缺 startedAt：无法计算（避免显示 0s 误导）
  return null;
});

/** 超过该阈值时显示"耗时较长"红色提示（spec: 5s） */
const LONG_DURATION_MS = 5_000;

/** 毫秒 → "1.2s" / "850ms" 友好格式 */
function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const s = ms / 1000;
  if (s < 10) return `${s.toFixed(2)}s`;
  if (s < 60) return `${s.toFixed(1)}s`;
  const m = Math.floor(s / 60);
  const rem = Math.round(s % 60);
  return `${m}m ${rem}s`;
}

/** 错误 output 可能为对象/字符串，统一格式化 */
function formatErrorOutput(output: unknown): string {
  if (output === null || output === undefined) return "(empty)";
  if (typeof output === "string") return output;
  try {
    return JSON.stringify(output, null, 2);
  } catch {
    return String(output);
  }
}

function truncateArgs(args: string): string {
  if (!args || args.length <= 120) return args || "";
  return args.slice(0, 120) + "…";
}

/** v2 工具名集合：7 个 v2 工具 + 它们的 v1 名称（向后兼容） */
const V2_TOOL_NAMES = new Set<string>([
  "search_files",
  "read_file_v2",
  "get_metadata",
  "edit_metadata",
  "batch_rename",
  "delete_file",
  "command_run",
]);

/** 当前 toolCall 是否是 v2 工具（用于显示 v2 badge） */
const isV2Tool = computed(() => V2_TOOL_NAMES.has(props.toolCall.name));
</script>

<style scoped>
.operationCard {
  margin: 3px 0 5px;
  background: rgba(var(--ion-color-medium-rgb), 0.05);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.16);
  border-radius: 7px;
  padding: 6px 10px;
  font-size: 11.5px;
  transition: border-color 0.2s;
}

.operationCard_streaming {
  border-color: rgba(var(--ion-color-primary-rgb), 0.35);
  animation: opPulse 2s ease-in-out infinite;
}

@keyframes opPulse {
  0%, 100% { border-color: rgba(var(--ion-color-primary-rgb), 0.25); }
  50% { border-color: rgba(var(--ion-color-primary-rgb), 0.55); }
}

.operationCardHead {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  user-select: none;
  -webkit-tap-highlight-color: transparent;
}

.operationCardHead:active {
  opacity: 0.7;
}

.operationCardIcon {
  font-size: 14px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.operationCardName {
  font-weight: 600;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  color: var(--ion-text-color);
  flex-shrink: 0;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* v2 工具标签：在工具名右侧展示一个小 pill，让用户一眼看出这次是 v2 调用 */
.operationCardV2Tag {
  display: inline-block;
  font-size: 9px;
  padding: 1px 5px;
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
  border-radius: 4px;
  font-weight: 600;
  letter-spacing: 0.05em;
  line-height: 1.3;
  flex-shrink: 0;
  cursor: help;
}

.operationCardBadge {
  margin-inline-start: auto;
  flex-shrink: 0;
}

.operationCardToggle {
  font-size: 12px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.45));
  flex-shrink: 0;
  margin-left: 2px;
  transition: transform 0.2s;
}

.operationCard_collapsed .operationCardToggle {
  /* 折叠时箭头向下 */
}

/* 展开内容区 */
.operationCardBody {
  margin-top: 4px;
}

.operationCardArgs {
  padding-left: 20px; /* 缩进与图标对齐 */
}

.operationCardArgs code {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 10.5px;
  color: var(--encv-text-secondary, #888);
  word-break: break-all;
  white-space: pre-wrap;
}

.operationCardResult {
  margin-top: 4px;
}

/* ─── Task 4: 错误详情块（仅 failed 状态） ──────────────────── */
.operationCard_failed {
  border-color: rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.4);
  background: rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.05);
}

.operationCardError {
  margin-top: 6px;
  border-top: 1px dashed rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.25);
  padding-top: 4px;
}

.operationCardErrorHeader {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 4px;
  cursor: pointer;
  user-select: none;
  -webkit-tap-highlight-color: transparent;
  border-radius: 4px;
}

.operationCardErrorHeader:hover {
  background: rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.08);
}

.operationCardErrorIcon {
  font-size: 13px;
  color: var(--ion-color-danger, #ef4444);
  flex-shrink: 0;
}

.operationCardErrorLabel {
  font-size: 10.5px;
  color: var(--ion-color-danger-shade, #c53030);
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.operationCardErrorCopyBtn {
  --padding-start: 4px;
  --padding-end: 4px;
  height: 20px !important;
  width: 20px !important;
  margin: 0;
  flex-shrink: 0;
  color: var(--ion-color-danger, #ef4444);
}

.operationCardErrorCopyBtn ion-icon {
  font-size: 12px;
}

.operationCardErrorToggle {
  font-size: 11px;
  color: var(--encv-text-secondary, #888);
  flex-shrink: 0;
  margin-left: 2px;
  transition: transform 0.2s;
}

.operationCardErrorDetails {
  margin-top: 4px;
  padding: 4px 6px;
  background: rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.06);
  border-radius: 4px;
  border-left: 2px solid var(--ion-color-danger, #ef4444);
}

.operationCardErrorRow {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  margin-bottom: 2px;
  font-size: 10.5px;
}

.operationCardErrorRow:last-child {
  margin-bottom: 0;
}

.operationCardErrorKey {
  font-weight: 600;
  color: var(--ion-color-danger-shade, #c53030);
  flex-shrink: 0;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
}

.operationCardErrorVal {
  flex: 1;
  min-width: 0;
  word-break: break-all;
  white-space: pre-wrap;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  color: var(--ion-text-color);
  margin: 0;
}

/* ─── Task 4: 耗时显示（卡片底部） ──────────────────────────── */
.operationCardDuration {
  margin-top: 4px;
  padding-left: 20px; /* 与 operationCardArgs 缩进对齐 */
  font-size: 9.5px;
  color: var(--encv-text-secondary, #888);
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  display: flex;
  align-items: center;
  gap: 6px;
}

.operationCardDurationWarn {
  color: var(--ion-color-warning-shade, #e0ac08);
  font-weight: 600;
}

body.dark .operationCardErrorDetails {
  background: rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.1);
}
</style>
