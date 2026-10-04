<!--
  PeerDegradedNotice —— P5 / Task 5.5：降级矩阵**持久性** UI（**不是 Toast**）

  为什么是常驻条幅：对端离线 / Hub 漂移 / 重启后 token 失效都是**持续状态**，
  Toast 一闪而过会让用户误以为"刚才那次失败了"，而不是"这个功能现在不能用"。
  状态恢复后自动消失（每 10s 轮询）。
-->
<template>
  <div v-if="visible.length > 0" class="pdBar" data-testid="peer-degraded">
    <div v-for="n in visible" :key="n.kind + n.peerName" class="pdRow" :data-testid="'pd-' + n.kind">
      <ion-icon :icon="warningOutline" class="pdIcon"></ion-icon>
      <span class="pdText">{{ textOf(n) }}</span>
      <button type="button" class="pdClose" @click="dismiss(n)">×</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { warningOutline } from "ionicons/icons";
import { IonIcon } from "@ionic/vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { type DegradeNotice, usePeerDegradation } from "@/composables/usePeerDegradation";

const { t } = useI18n();
const { notices } = usePeerDegradation();

/** 被手动收起的条目（同一 kind+peer 不再打扰；状态变化后 key 变了会重新出现） */
const dismissed = ref<Set<string>>(new Set());

const visible = computed(() => notices.value.filter(n => !dismissed.value.has(key(n))));

function key(n: DegradeNotice): string {
  return n.kind + "|" + n.peerName;
}

function dismiss(n: DegradeNotice) {
  dismissed.value.add(key(n));
}

function textOf(n: DegradeNotice): string {
  switch (n.kind) {
    case "peer_offline":
      return `${n.peerName} ${t("peers.degradedOffline") || "离线：跨端搜索与远程调试暂不可用"}`;
    case "hub_disconnected":
      return t("peers.degradedHubDisconnected") || "未连上互联服务（正在重连）：对端暂时看不到本设备";
    case "hub_not_paired":
      return t("peers.degradedNotPaired") || "本设备尚未连接互联服务（服务重启后需重新扫码配对）";
    case "rate_limited":
      return t("peers.degradedRateLimited") || "调用过于频繁已被限流，稍后自动恢复";
    case "circuit_open":
      return t("peers.degradedCircuitOpen") || "对端连续失败已熔断，冷却后自动重试";
    default:
      return t("peers.degradedUnknown") || "互联功能降级中";
  }
}
</script>

<style scoped>
.pdBar {
  position: sticky;
  top: 0;
  z-index: 900;
  padding: 4px 8px;
  background: rgba(255, 176, 32, 0.14);
  border-bottom: 1px solid rgba(255, 176, 32, 0.5);
}

.pdRow {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
  font-size: 0.78rem;
}

.pdIcon {
  flex-shrink: 0;
  color: #b26a00;
}

.pdText {
  flex: 1;
  min-width: 0;
}

.pdClose {
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 1rem;
  line-height: 1;
  opacity: 0.6;
}
</style>
