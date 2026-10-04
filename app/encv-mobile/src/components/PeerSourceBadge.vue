<!--
  PeerSourceBadge —— 搜索命中的**来源徽章**（spec desktop-web-android-pairing P3）
  语义：远端命中永远是"跨端引用"，必须一眼看出它不属于本端。
-->
<template>
  <span class="srcBadge" :class="source === 'local' ? 'srcBadgeLocal' : 'srcBadgePeer'">
    <ion-icon :icon="source === 'local' ? desktopIcon : phoneIcon" class="srcIcon"></ion-icon>
    <span class="srcText">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { IonIcon } from "@ionic/vue";
import { desktopOutline, phonePortraitOutline } from "ionicons/icons";
import { computed } from "vue";

const props = defineProps<{
  source: "local" | "peer";
  peerName?: string;
  peerPlatform?: string;
}>();

const desktopIcon = desktopOutline;
const phoneIcon = phonePortraitOutline;

const label = computed(() => {
  if (props.source === "local") return "本机";
  const plat = props.peerPlatform && props.peerPlatform !== "android" ? props.peerPlatform : "安卓";
  return props.peerName ? `${plat}·${props.peerName}` : `${plat}·对端`;
});
</script>

<style scoped>
.srcBadge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 6px;
  border-radius: 999px;
  font-size: 0.7rem;
  line-height: 1.5;
  white-space: nowrap;
}

.srcBadgeLocal {
  background: var(--color-base-200);
  color: var(--color-base-content);
}

.srcBadgePeer {
  background: var(--color-primary);
  color: var(--color-primary-content);
}

.srcIcon {
  font-size: 0.8rem;
}

.srcText {
  font-weight: 600;
}
</style>
