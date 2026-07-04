<!--
  FileContentCard - 文件内容预览卡片
  渲染 read_file 工具的返回结果。data 形状：
    { content: string, mimeType?: string, size?: number, note?: string }

  设计要点：
  - 头部展示元数据（mimeType + size + 行数）
  - 正文用等宽字体 + 代码块样式（保留 markdown 格式）
  - content > 4000 字符自动折叠（展开按钮）
  - 真实数据来自 read_file handler，mock 模式下走 execute_real=true
-->
<template>
  <div class="fileContentCard">
    <div class="fileContentCardHeader">
      <ion-icon :icon="documentTextIcon" class="fileContentCardIcon" />
      <span class="fileContentCardTitle">{{ titleText }}</span>
      <span v-if="parsed.error && !rawResult" class="fileContentCardBadge fileContentCardBadge_warn">等待结果</span>
      <span v-else-if="parsed.isErrorResponse" class="fileContentCardBadge fileContentCardBadge_error">{{ errorBadgeLabel }}</span>
      <span v-else-if="parsed.error" class="fileContentCardBadge fileContentCardBadge_error">解析异常</span>
      <span v-if="dataSourceTag" class="fileContentCardSource">{{ dataSourceTag }}</span>
      <span v-if="meta.size !== undefined" class="fileContentCardMeta">{{ formatSize(meta.size) }}</span>
      <span v-if="content" class="fileContentCardLines">{{ contentLineCount }} 行</span>
      <span v-if="meta.mimeType" class="fileContentCardMime">{{ meta.mimeType }}</span>
    </div>
    <pre v-if="content" class="fileContentCardBody" :class="{ fileContentCardBody_collapsed: !expanded }"><code>{{ expanded ? content : truncatedContent }}</code></pre>
    <div v-else-if="!resultJson" class="fileContentCardEmpty">
      <ion-icon :icon="hourglassIcon" class="fileContentCardEmptyIcon" />
      <span>工具执行中…</span>
    </div>
    <div v-else-if="parsed.isErrorResponse" class="fileContentCardError">
      <ion-icon :icon="documentTextIcon" class="fileContentCardErrorIcon" />
      <span class="fileContentCardErrorMsg">{{ parsed.error }}</span>
    </div>
    <div v-else class="fileContentCardEmpty">文件内容为空</div>
    <div v-if="looksBinary" class="fileContentCardBinaryWarn">
      ⚠ 内容可能包含二进制数据，仅显示可读部分
    </div>
    <div v-if="showToggle" class="fileContentCardActions">
      <button type="button" class="fileContentCardToggle" @click="toggle">
        {{ expanded ? '收起' : `展开全部 (${content.length} 字符)` }}
      </button>
    </div>
    <details v-if="rawResult" class="fileContentCardRaw">
      <summary>查看原始数据</summary>
      <pre>{{ rawResult }}</pre>
    </details>
  </div>
</template>

<script setup lang="ts">
import { IonIcon } from "@ionic/vue";
import { documentTextOutline, hourglassOutline } from "ionicons/icons";
import { computed, ref } from "vue";
import { useI18n } from "@/composables/useI18n";

const props = defineProps<{
  /** 后端 tool_result.result 的 JSON 字符串 */
  resultJson: string;
  /** 工具执行状态 */
  status?: "pending" | "running" | "success" | "failed";
}>();

const { t } = useI18n();

const documentTextIcon = documentTextOutline;
const hourglassIcon = hourglassOutline;
const expanded = ref(false);

const COLLAPSE_THRESHOLD = 4000;

interface ParsedFile {
  content: string;
  mimeType?: string;
  size?: number;
  note?: string;
}

const parsed = computed<{ data: ParsedFile | null; error: string; isErrorResponse: boolean }>(() => {
  if (!props.resultJson) {
    return { data: null, error: "empty result", isErrorResponse: false };
  }
  try {
    const obj = JSON.parse(props.resultJson) as Partial<ParsedFile & { error?: string; message?: string }>;
    // 错误响应格式：{"error": "too_large", "message": "文件 xxx 字节 > max_bytes"}
    if (typeof obj.error === "string" && obj.error.length > 0 && !obj.content) {
      return { data: null, error: obj.message || obj.error, isErrorResponse: true };
    }
    if (typeof obj.content !== "string") {
      return { data: null, error: "missing content field", isErrorResponse: false };
    }
    return {
      data: {
        content: obj.content,
        mimeType: typeof obj.mimeType === "string" ? obj.mimeType : undefined,
        size: typeof obj.size === "number" ? obj.size : undefined,
        note: typeof obj.note === "string" ? obj.note : undefined,
      },
      error: "",
      isErrorResponse: false,
    };
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    console.debug("[FileContentCard] parse failed:", msg, props.resultJson);
    return { data: null, error: msg, isErrorResponse: false };
  }
});

const content = computed(() => parsed.value.data?.content ?? "");
const meta = computed(() => ({
  mimeType: parsed.value.data?.mimeType,
  size: parsed.value.data?.size,
}));
const rawResult = computed(() => (parsed.value.error ? props.resultJson : ""));

const showToggle = computed(() => content.value.length > COLLAPSE_THRESHOLD);
const truncatedContent = computed(() => {
  if (content.value.length <= COLLAPSE_THRESHOLD) return content.value;
  return content.value.slice(0, COLLAPSE_THRESHOLD) + "\n…";
});

const titleText = computed(() => {
  if (props.status === "pending" || props.status === "running") return t("agent.toolCards.fileContentTitle") || "文件内容（查询中）";
  if (parsed.value.isErrorResponse) return parsed.value.error.includes("too_large") ? "文件过大" : "读取失败";
  if (parsed.value.error) return t("agent.toolCards.parseFailed") || "文件内容（数据异常）";
  return t("agent.toolCards.fileContentTitle") || "文件内容";
});

const errorBadgeLabel = computed(() => {
  const err = parsed.value.error;
  if (err?.includes("too_large")) return "过大";
  return "错误";
});

const dataSourceTag = computed(() => {
  if (!props.resultJson) return "";
  const s = props.resultJson;
  if (s.includes('"FAKE":true') || s.includes('"FAKE": true')) return "mock 数据";
  if (s.includes("studio_video_")) return "历史 mock";
  return "";
});

const contentLineCount = computed(() => {
  return content.value ? content.value.split("\n").length : 0;
});

const looksBinary = computed(() => {
  if (content.value.length < 50) return false;
  const nonPrintable = (content.value.match(/[\x00-\x08\x0b\x0c\x0e-\x1f]/g) || []).length;
  return nonPrintable > content.value.length * 0.05;
});

function toggle() {
  expanded.value = !expanded.value;
}

function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "—";
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb.toFixed(1)} MB`;
  return `${(mb / 1024).toFixed(2)} GB`;
}
</script>

<style scoped>
.fileContentCard {
  margin: 4px 0 6px;
  background: rgba(var(--ion-color-medium-rgb), 0.05);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 12px;
}

.fileContentCardHeader {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}

.fileContentCardIcon {
  font-size: 14px;
  color: var(--ion-color-primary);
}

.fileContentCardTitle {
  font-weight: 600;
  color: var(--ion-text-color);
}

.fileContentCardMeta {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 10.5px;
  color: var(--encv-text-secondary, #888);
}

.fileContentCardMime {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 8px;
  background: rgba(var(--ion-color-primary-rgb), 0.14);
  color: var(--ion-color-primary);
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
}

.fileContentCardBody {
  margin: 0;
  padding: 8px 10px;
  background: rgba(var(--ion-color-medium-rgb), 0.04);
  border-radius: 5px;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 11.5px;
  line-height: 1.55;
  color: var(--ion-text-color);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 360px;
  overflow-y: auto;
}

.fileContentCardBody code {
  font-family: inherit;
  background: transparent;
  padding: 0;
  font-size: inherit;
  color: inherit;
}

.fileContentCardBody_collapsed {
  max-height: 180px;
  overflow: hidden;
  position: relative;
}

.fileContentCardActions {
  margin-top: 4px;
  text-align: right;
}

.fileContentCardToggle {
  background: transparent;
  border: 0;
  padding: 2px 0;
  font-size: 11.5px;
  color: var(--ion-color-primary);
  cursor: pointer;
  font-family: inherit;
}

.fileContentCardToggle:hover {
  text-decoration: underline;
}

.fileContentCardEmpty {
  padding: 8px 0;
  text-align: center;
  color: var(--encv-text-secondary, #888);
  font-size: 11.5px;
}

.fileContentCardError {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  background: rgba(var(--ion-color-danger-rgb), 0.06);
  border-radius: 6px;
  font-size: 11.5px;
  color: var(--ion-color-danger, #eb445a);
}

.fileContentCardErrorIcon {
  font-size: 16px;
  flex-shrink: 0;
}

.fileContentCardErrorMsg {
  word-break: break-word;
}

.fileContentCardRaw {
  margin-top: 6px;
  font-size: 10.5px;
}

.fileContentCardRaw summary {
  cursor: pointer;
  color: var(--encv-text-secondary, #888);
  user-select: none;
}

.fileContentCardRaw pre {
  margin: 4px 0 0;
  padding: 6px 8px;
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  border-radius: 4px;
  overflow-x: auto;
  font-size: 10.5px;
  max-height: 160px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.fileContentCardBadge {
  font-size: 9px;
  padding: 1px 5px;
  border-radius: 6px;
  font-weight: 600;
  text-transform: uppercase;
}
.fileContentCardBadge_warn {
  background: rgba(var(--ion-color-warning-rgb), 0.18);
  color: var(--ion-color-warning-shade);
}
.fileContentCardBadge_error {
  background: rgba(var(--ion-color-danger-rgb), 0.15);
  color: var(--ion-color-danger);
}

.fileContentCardSource {
  font-size: 10px;
  color: var(--encv-text-secondary, #888);
  opacity: 0.8;
}

.fileContentCardLines {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 10px;
  color: var(--encv-text-secondary, #888);
}

.fileContentCardBinaryWarn {
  margin-top: 4px;
  padding: 4px 8px;
  font-size: 10.5px;
  color: var(--ion-color-warning-shade, #e68a00);
  background: rgba(var(--ion-color-warning-rgb), 0.1);
  border-radius: 4px;
}

.fileContentCardEmptyIcon {
  font-size: 16px;
  margin-right: 4px;
  animation: spin 1.5s linear infinite;
}
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
</style>
