<!--
  FileReferenceChip - 行内文件引用 chip
  展示 path[:line[:col]]，点击弹 popover 菜单（复制路径 / 复制相对路径 / 在 Files tab 打开）。
  参考 codex-web FileReference 组件的 popover 交互。
-->
<template>
  <span class="fileRefWrap" ref="wrapRef">
    <button
      type="button"
      class="fileRefChip ui-chip ui-chip--mono"
      :class="{ fileRefChip_open: popoverOpen }"
      :title="fullPath"
      @click.stop="togglePopover"
    >
      <ion-icon :icon="documentTextOutline" class="fileRefIcon"></ion-icon>
      <span class="fileRefLabel">{{ displayLabel }}</span>
      <span v-if="line !== undefined" class="fileRefLoc">:{{ line }}<span v-if="col !== undefined">:{{ col }}</span></span>
    </button>

    <ion-popover
      :is-open="popoverOpen"
      :event="popoverEvent"
      @didDismiss="popoverOpen = false"
      side="bottom"
      alignment="start"
      :show-backdrop="false"
      :dismiss-on-select="true"
      class="fileRefPopover"
    >
      <div class="fileRefMenu">
        <div class="fileRefMenuTitle" :title="fullPath">{{ displayLabel }}</div>
        <button type="button" class="fileRefMenuItem" @click="onCopyPath">
          <ion-icon :icon="copyOutline" class="fileRefMenuIcon"></ion-icon>
          <span>{{ t('agent.fileRefCopyPath') }}</span>
        </button>
        <button type="button" class="fileRefMenuItem" @click="onCopyRelativePath">
          <ion-icon :icon="gitBranchOutline" class="fileRefMenuIcon"></ion-icon>
          <span>{{ t('agent.fileRefCopyRelative') }}</span>
        </button>
        <button type="button" class="fileRefMenuItem" @click="onOpenInFiles">
          <ion-icon :icon="folderOpenOutline" class="fileRefMenuIcon"></ion-icon>
          <span>{{ t('agent.fileRefOpenInFiles') }}</span>
        </button>
      </div>
    </ion-popover>
  </span>
</template>

<script setup lang="ts">
import { copyOutline, documentTextOutline, folderOpenOutline, gitBranchOutline } from "ionicons/icons";
import { computed, ref } from "vue";
import { useRouter } from "vue-router";
import { copyToClipboard } from "@encv/shared-components/composables/useClipboard";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";

const props = defineProps<{
  path: string;
  line?: number;
  col?: number;
}>();

const { t } = useI18n();
const router = useRouter();

const popoverOpen = ref(false);
const popoverEvent = ref<Event | undefined>(undefined);
const wrapRef = ref<HTMLElement | null>(null);

// 显示文本：取最后一段（basename）；若与 path 相同则直接显示
const displayLabel = computed(() => {
  const segs = props.path.split(/[\\/]/).filter(Boolean);
  return segs[segs.length - 1] || props.path;
});

const fullPath = computed(() => {
  let s = props.path;
  if (props.line !== undefined) s += `:${props.line}`;
  if (props.col !== undefined) s += `:${props.col}`;
  return s;
});

// 相对路径：去掉开头的 ./ 或 ../ （简化版，相对项目根）
const relativePath = computed(() => {
  return props.path.replace(/^\.{1,2}[\\/]/, "");
});

function togglePopover(event: MouseEvent) {
  popoverEvent.value = event;
  popoverOpen.value = !popoverOpen.value;
}

async function onCopyPath() {
  const ok = await copyToClipboard(fullPath.value);
  showToast({
    message: ok ? t("agent.copied") : t("agent.copyFailed"),
    color: ok ? "success" : "danger",
    duration: 1500,
  });
  popoverOpen.value = false;
}

async function onCopyRelativePath() {
  const text =
    props.line !== undefined ? `${relativePath.value}:${props.line}${props.col !== undefined ? `:${props.col}` : ""}` : relativePath.value;
  const ok = await copyToClipboard(text);
  showToast({
    message: ok ? t("agent.copied") : t("agent.copyFailed"),
    color: ok ? "success" : "danger",
    duration: 1500,
  });
  popoverOpen.value = false;
}

function onOpenInFiles() {
  popoverOpen.value = false;
  // 跳转到 Files tab，path 作为 query param
  router.push({
    path: "/tabs/files",
    query: { path: props.path },
  });
}
</script>

<style scoped>
.fileRefWrap {
  display: inline-flex;
  position: relative;
}

/* 表面（背景/边框/圆角/内距/字体）已上提到全局 .ui-chip + .ui-chip--mono，
   供用户主题以 .ui-chip{} 覆写。本 scoped 仅保留结构/交互差异；
   :hover / .fileRefChip_open 的状态覆写仍在此（scoped 0,2,0 胜出）。 */
.fileRefChip {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  line-height: 1.4;
  cursor: pointer;
  transition: background-color 0.12s, border-color 0.12s, color 0.12s;
  max-width: 100%;
  vertical-align: baseline;
  user-select: none;
}

.fileRefChip:hover {
  background: color-mix(in srgb, var(--color-primary) 18%, transparent);
  border-color: color-mix(in srgb, var(--color-primary) 42%, transparent);
}

.fileRefChip_open {
  background: color-mix(in srgb, var(--color-primary) 22%, transparent);
  border-color: color-mix(in srgb, var(--color-primary) 55%, transparent);
}

.fileRefIcon {
  font-size: 11px;
  flex-shrink: 0;
  opacity: 0.85;
}

.fileRefLabel {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 240px;
}

.fileRefLoc {
  color: color-mix(in srgb, var(--color-primary) 85%, var(--color-black));
  opacity: 0.85;
  flex-shrink: 0;
}

/* popover 菜单样式 */
.fileRefMenu {
  min-width: 200px;
  padding: 4px 0;
  display: flex;
  flex-direction: column;
}

.fileRefMenuTitle {
  font-size: 11px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  color: var(--encv-text-secondary);
  padding: 8px 12px 6px;
  border-bottom: 1px solid color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 15%, transparent);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 280px;
}

.fileRefMenuItem {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 12px;
  background: transparent;
  border: 0;
  font-size: 13px;
  color: var(--ion-text-color);
  text-align: left;
  cursor: pointer;
  width: 100%;
}

.fileRefMenuItem:hover {
  background: color-mix(in srgb, var(--color-primary) 10%, transparent);
}

.fileRefMenuIcon {
  font-size: 15px;
  color: var(--color-primary);
  flex-shrink: 0;
}
</style>
