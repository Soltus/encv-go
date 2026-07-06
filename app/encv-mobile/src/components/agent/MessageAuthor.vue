<!--
  MessageAuthor - 消息作者头
  参照 codex_web .messageAuthor：
  - 28px 圆形头像 + label + meta（可选）
  - grid 布局：28px icon | minmax(0,1fr) text | 8px gap
-->
<template>
  <div class="messageAuthor">
    <div class="avatar" :class="`avatar_${variant}`">
      <ion-icon :icon="icon" class="avatarIcon"></ion-icon>
    </div>
    <div class="authorText">
      <div class="authorName">{{ label }}</div>
      <div v-if="meta" class="authorMeta">{{ meta }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component } from "vue";
import { computed } from "vue";

const props = defineProps<{
  icon: Component | string;
  label: string;
  meta?: string;
  /** 控制头像底色变体（可选） */
  variant?: "default" | "streaming" | "error" | "tool";
}>();

const _variant = computed(() => props.variant || "default");
</script>

<style scoped>
/* ── 参照 codex_web .messageAuthor ─────────────────────────── */
.messageAuthor {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 28px;
  margin-bottom: 4px;
}

/* 参照 codex_web .avatar: 28px 圆形 */
.avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: rgba(var(--ion-color-primary-rgb), 0.10);
  color: var(--ion-color-primary);
}

.avatar_streaming {
  background: rgba(var(--ion-color-primary-rgb), 0.16);
  animation: authorAvatarPulse 1.6s ease-in-out infinite;
}

.avatar_error {
  background: rgba(var(--ion-color-danger-rgb), 0.12);
  color: var(--ion-color-danger);
}

.avatar_tool {
  background: rgba(139, 92, 246, 0.12);
  color: #8b5cf6;
}

.avatarIcon {
  font-size: 14px;
}

.authorText {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
  min-width: 0;
}

.authorName {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-text-color);
}

.authorMeta {
  font-size: 11.5px;
  color: var(--encv-text-secondary, var(--ion-color-medium));
  margin-top: 1px;
}

@keyframes authorAvatarPulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.55; }
}
</style>
