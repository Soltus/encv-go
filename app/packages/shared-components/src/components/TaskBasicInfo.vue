<template>
  <!-- 🆕 2026-06-11 v5：商业级 breadcrumb 卡片 — 反映 task 在 group → section 架构中的位置 -->
  <div class="detail-section task-hierarchy-card">
    <div class="section-title">
      <ion-icon :icon="gitBranchOutline" class="section-title-icon"></ion-icon>
      {{ t('tasks.hierarchy') }}
    </div>
    <nav class="hierarchy-breadcrumb" :aria-label="t('tasks.hierarchy')">
      <ol class="breadcrumb-list">
        <!-- 0️⃣ 根层：Tasks / 任务中心（永远显示，作为最外层容器） -->
        <li class="breadcrumb-item breadcrumb-root">
          <span class="breadcrumb-icon-bubble breadcrumb-tone-root">
            <ion-icon :icon="listOutline" class="breadcrumb-icon"></ion-icon>
          </span>
          <span class="breadcrumb-label">{{ t('tasks.tasksRoot') }}</span>
        </li>

        <!-- 1️⃣ 触发者层：user / automation / ai_agent -->
        <li class="breadcrumb-item breadcrumb-trigger" v-if="triggeredBy !== 'user'">
          <ion-icon :icon="chevronForward" class="breadcrumb-sep"></ion-icon>
          <span class="breadcrumb-icon-bubble" :class="`trigger-tone-${triggeredBy}`">
            <ion-icon :icon="triggeredByIcon" class="breadcrumb-icon"></ion-icon>
          </span>
          <span class="breadcrumb-label">{{ t('tasks.triggeredBy_' + triggeredBy) }}</span>
        </li>

        <!-- 2️⃣ Workflow Run 层（只有 non-user task 才有 runId） -->
        <li v-if="runId" class="breadcrumb-item breadcrumb-run">
          <ion-icon :icon="chevronForward" class="breadcrumb-sep"></ion-icon>
          <span class="breadcrumb-icon-bubble breadcrumb-tone-automation">
            <ion-icon :icon="cogOutline" class="breadcrumb-icon"></ion-icon>
          </span>
          <span class="breadcrumb-label">
            <span class="breadcrumb-label-prefix">{{ t('tasks.runLabel') }}</span>
            <span class="breadcrumb-run-id">#{{ runId.slice(0, 8) }}</span>
          </span>
        </li>

        <!-- 3️⃣ Section 层：plugin / type / category / none -->
        <li class="breadcrumb-item breadcrumb-section">
          <ion-icon :icon="chevronForward" class="breadcrumb-sep"></ion-icon>
          <span class="breadcrumb-icon-bubble" :class="`section-tone-${sectionMeta.dimension}`">
            <ion-icon :icon="sectionIcon" class="breadcrumb-icon"></ion-icon>
          </span>
          <span class="breadcrumb-label">
            <span class="breadcrumb-label-prefix">{{ sectionDimensionLabel }}</span>
            <span class="breadcrumb-section-name">{{ sectionMeta.label }}</span>
          </span>
        </li>

        <!-- 4️⃣ Task 层（最右 = 当前） -->
        <li class="breadcrumb-item breadcrumb-task">
          <ion-icon :icon="chevronForward" class="breadcrumb-sep"></ion-icon>
          <span class="breadcrumb-icon-bubble breadcrumb-tone-task">
            <ion-icon :icon="documentTextOutline" class="breadcrumb-icon"></ion-icon>
          </span>
          <span class="breadcrumb-label">
            <span class="breadcrumb-label-prefix">{{ t('tasks.taskLabel') }}</span>
            <span class="breadcrumb-task-id">#{{ task.id.slice(0, 8) }}</span>
          </span>
        </li>
      </ol>
    </nav>
  </div>

  <div class="detail-section">
    <div class="section-title">
      <ion-icon :icon="informationCircleOutline" class="section-title-icon"></ion-icon>
      {{ t('tasks.basicInfo') }}
    </div>
    <div class="info-grid">
      <div class="info-item">
        <span class="info-label">{{ t('tasks.taskId') }}</span>
        <span class="info-value task-id-value" @click="copyTaskId" title="Click to copy">
          <span class="task-id-mono">{{ task.id }}</span>
          <ion-icon :icon="copyOutline" class="copy-icon"></ion-icon>
        </span>
      </div>
      <div class="info-item">
        <span class="info-label">{{ t('tasks.fileName') }}</span>
        <span class="info-value file-name">{{ fileName }}</span>
      </div>
      <div class="info-item">
        <span class="info-label">{{ t('tasks.taskType') }}</span>
        <ion-badge :color="task.type === 'encrypt' ? 'primary' : 'warning'" class="info-badge">
          {{ task.type === 'encrypt' ? t('tasks.encrypt') : t('tasks.decrypt') }}
        </ion-badge>
      </div>
      <div class="info-item" v-if="task.pluginName">
        <span class="info-label">{{ t('tasks.handledBy') }}</span>
        <ion-badge color="primary" class="info-badge">
          <ion-icon :icon="extensionPuzzle" class="badge-icon"></ion-icon>
          {{ task.pluginName }}
        </ion-badge>
      </div>
      <div class="info-item" v-if="task.containerVersion">
        <span class="info-label">{{ t('tasks.containerVersion') }}</span>
        <span class="info-value container-version-pill">{{ formatContainerVersion(task.containerVersion) }}</span>
      </div>
      <div class="info-item" v-if="runId">
        <span class="info-label">{{ t('tasks.workflowRunId') }}</span>
        <span class="info-value task-id-mono clickable" @click="copyRunId" title="Click to copy">
          {{ runId }}
          <ion-icon :icon="copyOutline" class="copy-icon"></ion-icon>
        </span>
      </div>
    </div>
  </div>

  <!-- 🆕 Task 18：加解密参数区块（cipherMode + compressionMode + extraFields） -->
  <!-- 后端 Task 16 持久化，前端 Task 17 接口扩展，这里展示回显 -->
  <!-- v-if="hasCryptoParams"：旧任务（Task 16 之前）没有这 2 字段，不显示空区块 -->
  <div class="detail-section" v-if="hasCryptoParams">
    <div class="section-title">
      <ion-icon :icon="lockClosedOutline" class="section-title-icon"></ion-icon>
      {{ t('tasks.cryptoParams') }}
    </div>
    <div class="info-grid">
      <div class="info-item" v-if="task.cipherMode !== undefined">
        <span class="info-label">{{ t('tasks.cipherMode') }}</span>
        <ion-badge :color="task.cipherMode === 1 ? 'secondary' : 'primary'" class="info-badge">
          {{ task.cipherMode === 1 ? t('tasks.cipherMode256') : t('tasks.cipherMode128') }}
        </ion-badge>
      </div>
      <div class="info-item" v-if="task.compressionMode">
        <span class="info-label">{{ t('tasks.compressionMode') }}</span>
        <ion-badge
          :color="task.compressionMode === 'zstd' ? 'success' : 'medium'"
          class="info-badge"
        >
          {{ task.compressionMode === 'zstd' ? 'Zstd' : t('tasks.compressionNone') }}
        </ion-badge>
      </div>
      <!-- extraFields：自定义参数（如 plugin_password 等不固定字段） -->
      <template v-if="task.extraFields && Object.keys(task.extraFields).length > 0">
        <div class="info-item" v-for="(value, key) in task.extraFields" :key="key">
          <span class="info-label">{{ formatExtraFieldLabel(String(key)) }}</span>
          <span class="info-value extra-field-value">{{ formatExtraFieldValue(value) }}</span>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { IonBadge, IonIcon } from "@ionic/vue";
import {
  chevronForward,
  cogOutline,
  copyOutline,
  documentTextOutline,
  ellipsisHorizontalCircleOutline,
  extensionPuzzle,
  folderOutline,
  gitBranchOutline,
  hardwareChipOutline,
  informationCircleOutline,
  listOutline,
  lockClosedOutline,
  person,
  swapVertical,
} from "ionicons/icons";
import { computed } from "vue";
import type { EncvTask } from "@encv/shared-components/api/encv";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { type SectionDimension, useSectionDerivation } from "@encv/shared-components/composables/useSectionDerivation";
import { showToast } from "@encv/shared-components/composables/useToast";
import { formatContainerVersion } from "@encv/shared-components/constants/containerVersion";

const props = defineProps<{ task: EncvTask }>();
const { t } = useI18n();

async function copyTaskId() {
  try {
    await navigator.clipboard.writeText(props.task.id);
    showToast({ message: t("tasks.idCopied"), duration: 1500, color: "success" });
  } catch {
    showToast({ message: t("tasks.idCopyFailed"), duration: 1500, color: "danger" });
  }
}

async function copyRunId() {
  if (!runId.value) return;
  try {
    await navigator.clipboard.writeText(runId.value);
    showToast({ message: t("tasks.runIdCopied"), duration: 1500, color: "success" });
  } catch {
    showToast({ message: t("tasks.runIdCopyFailed"), duration: 1500, color: "danger" });
  }
}

const fileName = computed(() => {
  const parts = props.task.sourcePath.split("/");
  return parts[parts.length - 1] || props.task.sourcePath;
});

// 🆕 2026-06-18 Task 18：crypto params 区块显示判定 + extraFields 格式化
// 旧任务（Task 16 之前）没有这 3 个字段 → 不显示空区块
const hasCryptoParams = computed(() => {
  const task = props.task;
  return (
    (task.cipherMode !== undefined && task.cipherMode !== null) ||
    !!task.compressionMode ||
    !!(task.extraFields && Object.keys(task.extraFields).length > 0)
  );
});

// extraField key → 显示标签：snake_case → Title Case（如 plugin_password → Plugin Password）
// 已知 key 走 i18n（如 plugin_password → tasks.pluginPassword），未知 key 退化到 Title Case
const EXTRA_FIELD_LABEL_I18N: Record<string, string> = {
  pluginPassword: "tasks.pluginPassword",
  streamPreset: "tasks.streamPreset",
  encryptFilename: "tasks.encryptFilename",
  fnRounds: "tasks.fnRounds",
  fnCharset: "tasks.fnCharset",
  fnDeconfuse: "tasks.fnDeconfuse",
  fnStructured: "tasks.fnStructured",
  encodeFilename: "tasks.encodeFilename",
  encType: "tasks.encType",
};

function formatExtraFieldLabel(key: string): string {
  // 1) 直接命中 i18n 表
  const directKey = EXTRA_FIELD_LABEL_I18N[key];
  if (directKey) return t(directKey);
  // 2) camelCase → snake_case 后再查
  const snakeKey = key.replace(/([A-Z])/g, "_$1").toLowerCase();
  const snakeLookup = Object.keys(EXTRA_FIELD_LABEL_I18N).find(k => k.replace(/([A-Z])/g, "_$1").toLowerCase() === snakeKey);
  if (snakeLookup) return t(EXTRA_FIELD_LABEL_I18N[snakeLookup]);
  // 3) 退化：snake_case → Title Case
  return key.replace(/[_-]/g, " ").replace(/\b\w/g, c => c.toUpperCase());
}

// extraField value → 显示值：bool 字符串 → ✓/✗；密码类 key → 脱敏（•••••）
function formatExtraFieldValue(value: string): string {
  if (value === undefined || value === null) return "";
  const v = String(value);
  // bool 字符串
  if (v === "true") return "✓";
  if (v === "false") return "✗";
  // 密码类（key 在调用方决定，这里只看 value 长度，长字符串疑似密码 → 脱敏）
  // 注意：密码脱敏由调用方决定（这里只做通用长字符串截断）
  if (v.length > 32) return v.slice(0, 8) + "…" + v.slice(-4);
  return v;
}

const triggeredBy = computed(() => props.task.triggeredBy ?? "user");
const runId = computed(() => props.task.runId);
const triggeredByIcon = computed(() => {
  const v = triggeredBy.value;
  return v === "automation" ? cogOutline : v === "ai_agent" ? hardwareChipOutline : person;
});

// 🆕 2026-06-11 v5：section 维度元数据（与 Tasks.vue deriveSubSection 保持一致）
// 🆕 2026-06-18 Task 6：派生逻辑已抽取到 @/composables/useSectionDerivation。
//   TaskBasicInfo 是单 task 组件，dimension 由 props.task 字段决定（per-component），
//   适合用 useSectionDerivation(dimension) composable 包裹。
//   'none' 维度的 label 用 i18n 覆盖（保持原行为：t('tasks.sectionOther')）。
const sectionDimension = computed<SectionDimension>(() => (props.task.pluginName ? "plugin" : "none"));
const { derive } = useSectionDerivation(sectionDimension);
const sectionMeta = computed(() => {
  const meta = derive(props.task);
  if (meta.dimension === "none") {
    return { ...meta, label: t("tasks.sectionOther") };
  }
  return meta;
});
const sectionIcon = computed(() => {
  // 当前 sectionMeta 仅返回 'plugin' | 'none' 两个维度（type / category 留给 Tasks.vue 派生）
  // 为兼容历史 case 分支不报错，这里也覆盖 'type' | 'category'
  const dim = sectionMeta.value.dimension as "plugin" | "type" | "category" | "none";
  switch (dim) {
    case "plugin":
      return extensionPuzzle;
    case "type":
      return swapVertical;
    case "category":
      return folderOutline;
    default:
      return ellipsisHorizontalCircleOutline;
  }
});
const sectionDimensionLabel = computed(() => {
  const dim = sectionMeta.value.dimension as "plugin" | "type" | "category" | "none";
  switch (dim) {
    case "plugin":
      return t("tasks.dimensionPlugin");
    case "type":
      return t("tasks.dimensionType");
    case "category":
      return t("tasks.dimensionCategory");
    default:
      return t("tasks.dimensionNone");
  }
});
</script>

<style scoped>
.detail-section {
  margin-bottom: 24px;
}

.section-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--ion-text-color);
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
  letter-spacing: -0.01em;
}
.section-title-icon {
  font-size: 16px;
  color: var(--ion-color-primary);
}

/* 🆕 2026-06-11 v5：商业级 breadcrumb */
.task-hierarchy-card {
  background: linear-gradient(135deg, rgba(79, 140, 255, 0.04), rgba(139, 92, 246, 0.04));
  border: 1px solid rgba(79, 140, 255, 0.12);
  border-radius: 12px;
  padding: 14px 16px 16px;
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.02), 0 2px 6px -2px rgba(0, 0, 0, 0.04);
}

.hierarchy-breadcrumb {
  margin: 0;
  padding: 0;
}
.breadcrumb-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 0;
  font-size: 12px;
  line-height: 1.4;
}
.breadcrumb-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px 4px 4px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid rgba(0, 0, 0, 0.06);
  transition: background 0.15s ease, border-color 0.15s ease;
}
.breadcrumb-item:hover {
  background: rgba(255, 255, 255, 0.95);
  border-color: rgba(0, 0, 0, 0.12);
}
/* 最右 = 当前 task 高亮 */
.breadcrumb-task {
  background: rgba(79, 140, 255, 0.1);
  border-color: rgba(79, 140, 255, 0.2);
}

.breadcrumb-icon-bubble {
  width: 18px;
  height: 18px;
  border-radius: 5px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  font-size: 11px;
  color: white;
  background: var(--ion-color-medium);
}
.breadcrumb-icon-bubble.breadcrumb-tone-automation {
  background: linear-gradient(135deg, #5b9dff, #2f7ce0);
}
.breadcrumb-icon-bubble.breadcrumb-tone-task {
  background: linear-gradient(135deg, #2f7ce0, #1e5aa0);
}
.breadcrumb-icon-bubble.breadcrumb-tone-root {
  background: linear-gradient(135deg, #4a5568, #2d3748);
}
.breadcrumb-icon-bubble.trigger-tone-automation {
  background: linear-gradient(135deg, #5b9dff, #2f7ce0);
}
.breadcrumb-icon-bubble.trigger-tone-ai_agent {
  background: linear-gradient(135deg, #b388ff, #7c4dff);
}
.breadcrumb-icon-bubble.section-tone-plugin {
  background: linear-gradient(135deg, #5b9dff, #2f7ce0);
}
.breadcrumb-icon-bubble.section-tone-type {
  background: linear-gradient(135deg, #ffb74d, #f57c00);
}
.breadcrumb-icon-bubble.section-tone-category {
  background: linear-gradient(135deg, #66bb6a, #388e3c);
}
.breadcrumb-icon-bubble.section-tone-none {
  background: linear-gradient(135deg, #bdbdbd, #9e9e9e);
}
.breadcrumb-icon {
  font-size: 11px;
  color: white;
}
.breadcrumb-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--ion-color-dark);
  font-weight: 500;
}
.breadcrumb-label-prefix {
  color: var(--encv-text-secondary);
  font-size: 10px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.breadcrumb-run-id,
.breadcrumb-section-name,
.breadcrumb-task-id {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-weight: 600;
  font-size: 11px;
  color: var(--ion-color-dark);
}
.breadcrumb-task-id {
  color: var(--ion-color-primary);
}

.breadcrumb-sep {
  font-size: 14px;
  color: var(--ion-color-medium);
  margin: 0 2px;
  flex-shrink: 0;
}

/* 🆕 2026-06-11 v5：basic info 商业级 */
.info-grid {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 10px 16px;
  align-items: center;
  background: var(--ion-color-light, #f8f9fa);
  border-radius: 10px;
  padding: 14px 16px;
  border: 1px solid rgba(0, 0, 0, 0.04);
}

.info-item {
  display: contents;
}

.info-label {
  font-size: 11px;
  color: var(--ion-color-medium);
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.info-value {
  font-size: 13px;
  font-weight: 500;
  justify-self: start;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.info-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-weight: 600;
}

.file-name {
  word-break: break-all;
  max-width: 220px;
}

.task-id-mono {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  color: var(--encv-text-secondary);
  word-break: break-all;
}

.task-id-value {
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  word-break: break-all;
}
.task-id-value:hover .task-id-mono {
  color: var(--ion-color-primary);
}

.clickable {
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
  transition: background 0.15s ease;
}
.clickable:hover {
  background: rgba(79, 140, 255, 0.06);
}

.copy-icon {
  font-size: 12px;
  opacity: 0.5;
  flex-shrink: 0;
  transition: opacity 0.15s ease;
}
.task-id-value:hover .copy-icon,
.clickable:hover .copy-icon {
  opacity: 1;
}

.container-version-pill {
  display: inline-flex;
  align-items: center;
  background: linear-gradient(135deg, rgba(79, 140, 255, 0.12), rgba(79, 140, 255, 0.06));
  color: var(--ion-color-primary);
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 6px;
  border: 1px solid rgba(79, 140, 255, 0.18);
}

.badge-icon {
  font-size: 11px;
}

/* 🆕 2026-06-18 Task 18：crypto params 区块 extraField 值样式 */
.extra-field-value {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  color: var(--ion-color-dark);
  background: rgba(0, 0, 0, 0.04);
  padding: 2px 6px;
  border-radius: 4px;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.triggered-by-icon {
  font-size: 11px;
  margin-right: 3px;
  vertical-align: middle;
}
</style>
