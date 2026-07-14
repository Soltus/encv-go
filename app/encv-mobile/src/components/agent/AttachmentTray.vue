<!--
  AttachmentTray - Composer 附件展示行

  Task 12：
  - 位于 textarea 上方
  - image 类型：48x48 缩略图
  - file  类型：文件卡片（文件名 / 大小 / 关闭按钮）
  - 不显示时整体 v-if，不占空间
-->
<template>
  <div v-if="attachments && attachments.length > 0" class="attachmentTray">
    <div class="attachmentTrayRow">
      <div
        v-for="att in attachments"
        :key="att.id"
        class="attachmentItem"
        :class="`attachmentItem-${att.kind}`"
      >
        <!-- 图片：缩略图 -->
        <template v-if="att.kind === 'image'">
          <div class="attachmentThumb">
            <img :src="att.dataUrl" :alt="att.name" class="attachmentThumbImg" />
          </div>
          <div class="attachmentMeta attachmentMeta-image">
            <span class="attachmentName" :title="att.name">{{ att.name }}</span>
            <span class="attachmentSize">{{ formatSize(att.sizeBytes) }}</span>
          </div>
        </template>
        <!-- 文件：卡片 -->
        <template v-else>
          <ion-icon :icon="documentIcon" class="attachmentFileIcon" />
          <div class="attachmentMeta">
            <span class="attachmentName" :title="att.name">{{ att.name }}</span>
            <span class="attachmentSize">{{ formatSize(att.sizeBytes) }}</span>
          </div>
        </template>

        <button
          type="button"
          class="attachmentRemove"
          :aria-label="t('agent.removeAttachment')"
          :title="t('agent.removeAttachment')"
          @click="onRemove(att.id)"
        >
          <ion-icon :icon="closeIcon" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { closeOutline, documentTextOutline } from "ionicons/icons";
import type { Attachment } from "@/composables/useAttachments";
import { useI18n } from "@encv/shared-components/composables/useI18n";

defineProps<{
  attachments: Attachment[];
  onRemove: (id: string) => void;
}>();

const { t } = useI18n();

const closeIcon = closeOutline;
const documentIcon = documentTextOutline;

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<style scoped>
.attachmentTray {
  padding: 4px 0 6px;
  border-bottom: 1px dashed rgba(var(--ion-color-medium-rgb), 0.18);
  margin-bottom: 4px;
}

.attachmentTrayRow {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: stretch;
}

.attachmentItem {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  border-radius: 10px;
  background: rgba(var(--ion-color-medium-rgb), 0.1);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  max-width: 220px;
  min-width: 0;
  position: relative;
}

.attachmentItem-image {
  /* 图片：缩略图 + 标题两行式 */
  flex-direction: row;
  align-items: center;
  padding: 4px 8px 4px 4px;
}

.attachmentThumb {
  width: 36px;
  height: 36px;
  border-radius: 6px;
  overflow: hidden;
  flex-shrink: 0;
  background: rgba(var(--ion-color-medium-rgb), 0.15);
  display: flex;
  align-items: center;
  justify-content: center;
}

.attachmentThumbImg {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.attachmentFileIcon {
  font-size: 22px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.attachmentMeta {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}

.attachmentMeta-image {
  /* 图片模式下不撑满，让缩略图主导 */
  flex: 0 1 auto;
  max-width: 140px;
}

.attachmentName {
  font-size: 12px;
  font-weight: 500;
  color: var(--ion-text-color);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.attachmentSize {
  font-size: 10.5px;
  color: var(--ion-text-color-step-400, #888);
  line-height: 1.2;
}

.attachmentRemove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: 0;
  border-radius: 50%;
  background: rgba(var(--ion-color-medium-rgb), 0.2);
  color: var(--ion-text-color-step-500, #666);
  cursor: pointer;
  padding: 0;
  flex-shrink: 0;
  font-size: 12px;
  transition: background 0.12s;
}

.attachmentRemove:hover {
  background: var(--ion-color-danger);
  color: #fff;
}

.attachmentRemove ion-icon {
  font-size: 12px;
}
</style>
