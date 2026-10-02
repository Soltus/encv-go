<!--
  RemoteApprovalPrompt —— P4：**执行端**远程 Agent 审批弹窗
  (spec desktop-web-android-pairing Task 4.2 / 4.5)

  设计要点：
   - 一次只弹一条（其余排队），避免弹窗堆叠遮挡。
   - 默认**每次都问**；「信任此设备」= 进程级信任，**本端服务重启后失效**（文案明说）。
   - 破坏性工具即使已信任也强制确认 —— 信任按钮仍在（本次放行），但明说下次仍会问。
   - 挂起超时由后端自动拒绝（90s），这里只把倒计时显示出来；超时后弹窗自然消失。
-->
<template>
  <div v-if="current" class="raOverlay" data-testid="remote-approval">
    <div class="raCard" role="alertdialog" aria-modal="true">
      <div class="raHeader">
        <span class="raTitle">{{ t('peers.remoteApprovalTitle') || '远程调试请求' }}</span>
        <span v-if="queued > 0" class="raQueued">+{{ queued }}</span>
      </div>

      <div class="raRow">
        <span class="raLabel">{{ t('peers.fromDevice') || '来自设备' }}</span>
        <span class="raValue" data-testid="ra-peer">{{ current.peerName || current.peerId }}</span>
      </div>
      <div class="raRow">
        <span class="raLabel">{{ t('peers.tool') || '操作' }}</span>
        <span class="raValue raTool" data-testid="ra-tool">{{ current.tool }}</span>
      </div>

      <div v-if="current.destructive" class="raWarn" data-testid="ra-destructive">
        {{ t('peers.destructiveHint') || '破坏性操作：即使已信任该设备，下次仍会询问' }}
      </div>

      <div class="raFooter">
        <span class="raCountdown" data-testid="ra-countdown">
          {{ t('peers.autoDeclineIn') || '超时自动拒绝' }} {{ countdown }}s
        </span>
      </div>

      <div class="raActions">
        <button type="button" class="raBtn raBtn_decline" data-testid="ra-decline" :disabled="busy" @click="onDecide('decline')">
          {{ t('modals.decline') || '拒绝' }}
        </button>
        <button type="button" class="raBtn raBtn_once" data-testid="ra-once" :disabled="busy" @click="onDecide('accept')">
          {{ t('peers.allowOnce') || '允许一次' }}
        </button>
        <button type="button" class="raBtn raBtn_trust" data-testid="ra-trust" :disabled="busy" @click="onDecide('trust_device')">
          {{ t('peers.trustDevice') || '信任此设备' }}
        </button>
      </div>
      <p class="raHint">{{ t('peers.trustHint') || '信任仅在本端服务运行期间有效，重启后需重新授权' }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { type RemoteDecision, useRemoteApproval } from "@/composables/useRemoteApproval";

const { t } = useI18n();
const { current, queued, busy, decide } = useRemoteApproval();

const now = ref(Date.now());
let timer: ReturnType<typeof setInterval> | null = null;

function ensureTimer(on: boolean) {
  if (on && !timer) {
    timer = setInterval(() => {
      now.value = Date.now();
    }, 1000);
  } else if (!on && timer) {
    clearInterval(timer);
    timer = null;
  }
}

watch(
  () => !!current.value,
  on => ensureTimer(on),
  { immediate: true }
);
onUnmounted(() => ensureTimer(false));

const countdown = computed(() => {
  const c = current.value;
  if (!c) return 0;
  const ms = new Date(c.expiresAt).getTime() - now.value;
  return Math.max(0, Math.ceil(ms / 1000));
});

async function onDecide(d: RemoteDecision) {
  const c = current.value;
  if (!c) return;
  await decide(c.callId, d);
}
</script>

<style scoped>
.raOverlay {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.45);
}

.raCard {
  width: min(420px, 92vw);
  padding: 16px 18px;
  border-radius: 14px;
  background: var(--panel-bg, #fff);
  color: var(--fg-main);
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.28);
}

.raHeader {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.raTitle {
  font-size: 1rem;
  font-weight: 700;
}

.raQueued {
  font-size: 0.75rem;
  opacity: 0.7;
}

.raRow {
  display: flex;
  gap: 8px;
  margin: 4px 0;
  font-size: 0.85rem;
}

.raLabel {
  width: 68px;
  flex-shrink: 0;
  opacity: 0.65;
}

.raValue {
  word-break: break-all;
}

.raTool {
  font-weight: 600;
}

.raWarn {
  margin: 8px 0;
  padding: 6px 8px;
  border-radius: 8px;
  font-size: 0.78rem;
  background: rgba(255, 90, 60, 0.12);
  color: var(--color-danger, #e0503f);
}

.raFooter {
  display: flex;
  justify-content: flex-end;
  margin-top: 8px;
}

.raCountdown {
  font-size: 0.72rem;
  opacity: 0.6;
}

.raActions {
  display: flex;
  gap: 8px;
  margin-top: 10px;
}

.raBtn {
  flex: 1;
  padding: 8px 6px;
  border: 1px solid transparent;
  border-radius: 8px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
}

.raBtn:disabled {
  opacity: 0.55;
  cursor: default;
}

.raBtn_decline {
  background: transparent;
  border-color: var(--color-danger, #e0503f);
  color: var(--color-danger, #e0503f);
}

.raBtn_once {
  background: var(--color-primary);
  color: #fff;
}

.raBtn_trust {
  background: transparent;
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.raHint {
  margin: 8px 0 0;
  font-size: 0.7rem;
  opacity: 0.6;
}
</style>
