<!--
  MountListCard - 挂载点列表结构化卡片
  渲染 list_mounts 工具的返回结果。data 形状：
    { count: number, items: [{ id, path, name }] }

  设计要点：
  - 卡片样式与 GroupedOperationMessage 整体风格一致（浅色背景 + 圆角 + 边框）
  - 暗黑模式自动跟随（用 --ion- / --encv- CSS 变量）
  - 真实数据由后端 list_mounts handler 返（mock 模式下走 execute_real=true）
  - 渲染失败时显示「数据解析失败」+ 原始 result 文本，方便定位
-->
<template>
  <div class="mountListCard">
    <div class="mountListCardHeader">
      <ion-icon :icon="folderOpenIcon" class="mountListCardIcon" />
      <span class="mountListCardTitle">{{ titleText }}</span>
      <span v-if="parsed.error && !rawResult" class="mountListCardBadge mountListCardBadge_warn">等待结果</span>
      <span v-else-if="parsed.error" class="mountListCardBadge mountListCardBadge_error">解析异常</span>
      <span v-if="dataSourceTag" class="mountListCardSource">{{ dataSourceTag }}</span>
      <span v-if="mounts.length > 0" class="mountListCardCount">{{ mounts.length }}</span>
    </div>
    <ul v-if="mounts.length > 0" class="mountListCardList">
      <li v-for="m in mounts" :key="m.id" class="mountListCardItem">
        <div class="mountListCardItemName">
          <ion-icon :icon="serverIcon" class="mountListCardItemIcon" />
          <span>{{ m.name || m.id }}</span>
        </div>
        <code class="mountListCardItemPath">{{ m.path || '/' }}</code>
      </li>
    </ul>
    <div v-else-if="!resultJson" class="mountListCardEmpty">
      <ion-icon :icon="hourglassIcon" class="mountListCardEmptyIcon" />
      <span>工具执行中…</span>
    </div>
    <div v-else class="mountListCardEmpty">未发现挂载点</div>
    <details v-if="rawResult" class="mountListCardRaw">
      <summary>查看原始数据</summary>
      <pre>{{ rawResult }}</pre>
    </details>
  </div>
</template>

<script setup lang="ts">
import { folderOpenOutline, hourglassOutline, serverOutline } from "ionicons/icons";
import { computed } from "vue";
import { useI18n } from "@/composables/useI18n";

const props = defineProps<{
  /** 后端 tool_result.result 的 JSON 字符串（list_mounts 返回值） */
  resultJson: string;
  /** 工具执行状态 */
  status?: "pending" | "running" | "success" | "failed";
}>();

const { t } = useI18n();

const _folderOpenIcon = folderOpenOutline;
const _serverIcon = serverOutline;
const _hourglassIcon = hourglassOutline;

interface Mount {
  id?: string;
  name?: string;
  path?: string;
}

const parsed = computed<{ mounts: Mount[]; error: string }>(() => {
  if (!props.resultJson) {
    return { mounts: [], error: "empty result" };
  }
  try {
    const obj = JSON.parse(props.resultJson) as {
      count?: number;
      items?: Mount[];
      mounts?: Mount[];
    };
    const arr = Array.isArray(obj.items) ? obj.items : Array.isArray(obj.mounts) ? obj.mounts : [];
    return { mounts: arr, error: "" };
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    console.debug("[MountListCard] parse failed:", msg, props.resultJson);
    return { mounts: [], error: msg };
  }
});

const _mounts = computed(() => parsed.value.mounts);
const _rawResult = computed(() => (parsed.value.error ? props.resultJson : ""));

const _titleText = computed(() => {
  if (props.status === "pending" || props.status === "running") return t("agent.toolCards.mountsTitle") || "挂载点（查询中）";
  if (parsed.value.error) return t("agent.toolCards.parseFailed") || "挂载点（数据异常）";
  return t("agent.toolCards.mountsTitle") || "挂载点";
});

const _dataSourceTag = computed(() => {
  if (!props.resultJson) return "";
  const s = props.resultJson;
  if (s.includes('"FAKE":true') || s.includes('"FAKE": true')) return "mock 数据";
  if (s.includes("studio_video_")) return "历史 mock";
  return "";
});
</script>

<style scoped>
.mountListCard {
  margin: 4px 0 6px;
  background: rgba(var(--ion-color-medium-rgb), 0.05);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 12px;
}

.mountListCardHeader {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
}

.mountListCardIcon {
  font-size: 14px;
  color: var(--ion-color-primary);
}

.mountListCardTitle {
  font-weight: 600;
  color: var(--ion-text-color);
}

.mountListCardCount {
  margin-inline-start: auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 18px;
  padding: 0 6px;
  border-radius: 9px;
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
  font-size: 10px;
  font-weight: 600;
}

.mountListCardList {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.mountListCardItem {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  background: rgba(var(--ion-color-medium-rgb), 0.04);
  border-radius: 5px;
  min-width: 0;
}

.mountListCardItemName {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-weight: 500;
  color: var(--ion-text-color);
  flex-shrink: 0;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mountListCardItemIcon {
  font-size: 12px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.mountListCardItemPath {
  flex: 1;
  min-width: 0;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 10.5px;
  color: var(--encv-text-secondary, #888);
  word-break: break-all;
  white-space: pre-wrap;
}

.mountListCardEmpty {
  padding: 8px 0;
  text-align: center;
  color: var(--encv-text-secondary, #888);
  font-size: 11.5px;
}

.mountListCardRaw {
  margin-top: 6px;
  font-size: 10.5px;
}

.mountListCardRaw summary {
  cursor: pointer;
  color: var(--encv-text-secondary, #888);
  user-select: none;
}

.mountListCardRaw pre {
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

.mountListCardBadge {
  font-size: 9px;
  padding: 1px 5px;
  border-radius: 6px;
  font-weight: 600;
  text-transform: uppercase;
}
.mountListCardBadge_warn {
  background: rgba(var(--ion-color-warning-rgb), 0.18);
  color: var(--ion-color-warning-shade);
}
.mountListCardBadge_error {
  background: rgba(var(--ion-color-danger-rgb), 0.15);
  color: var(--ion-color-danger);
}

.mountListCardSource {
  font-size: 10px;
  color: var(--encv-text-secondary, #888);
  opacity: 0.8;
}

.mountListCardEmptyIcon {
  font-size: 16px;
  margin-right: 4px;
  animation: spin 1.5s linear infinite;
}
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
</style>
