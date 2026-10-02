<!--
  PeerPairingPanel — 双端互联配对面板（spec desktop-web-android-pairing P2b）

  职责：
    - 生成一次性配对票据（120s）→ 渲染二维码（二维码依赖可用时）
    - 等待安卓端扫码（轮询配对状态）
    - 配对成功后显示 **SAS 6 位安全码** + 设备信息，要求人工核对后才可信任

  边界：
    - 二维码内容 = { v, hub, pairingId, psk, exp }，**不含任何内网地址**
      （R3：https 页面请求 http 内网地址会被浏览器按混合内容拦截）
    - psk 只在内存流转，不写 localStorage
    - 二维码渲染走**动态 import('qrcode')**：依赖未安装时降级为"手动输入配对码"，
      绝不自己实现 QR 编码（正确性风险）
-->
<template>
  <div class="peerPanel">
    <div v-if="phase === 'idle' || phase === 'expired'" class="panelIdle">
      <p class="panelHelp">{{ t('peers.help') }}</p>
      <ion-button expand="block" :disabled="busy" @click="generate">
        {{ phase === 'expired' ? t('peers.regenerate') : t('peers.generate') }}
      </ion-button>
      <p v-if="phase === 'expired'" class="panelExpired">{{ t('peers.expired') }}</p>
    </div>

    <div v-else-if="phase === 'waiting'" class="panelWaiting">
      <div class="qrBox">
        <canvas v-show="qrReady" ref="canvasRef" class="qrCanvas"></canvas>
        <div v-if="!qrReady" class="qrFallback">
          <p class="qrFallbackHint">{{ t('peers.qrUnavailable') }}</p>
          <code class="qrCodeText">{{ pairingCode }}</code>
          <p v-if="qrDiag" class="qrDiag">{{ qrDiag }}</p>
          <ion-button size="small" fill="outline" @click="copyCode">{{ copied ? t('peers.copied') : t('peers.copyCode') }}</ion-button>
        </div>
      </div>
      <p class="panelMeta">
        <span>{{ t('peers.waiting') }}</span>
        <span class="panelCountdown">{{ t('peers.expiresIn', { sec: remain }) }}</span>
      </p>
      <ion-button size="small" fill="clear" @click="generate">{{ t('peers.regenerate') }}</ion-button>
    </div>

    <div v-else class="panelPaired">
      <p class="pairedTitle">{{ t('peers.paired', { name: paired?.name || paired?.platform || 'device' }) }}</p>

      <div v-if="phase === 'paired'" class="sasBox">
        <p class="sasTitle">{{ t('peers.sasTitle') }}</p>
        <div class="sasCode">{{ paired?.sas }}</div>
        <p class="sasHelp">{{ t('peers.sasHelp') }}</p>
        <div class="sasActions">
          <ion-button size="small" @click="confirmTrust">{{ t('peers.confirm') }}</ion-button>
          <ion-button size="small" color="danger" fill="outline" @click="rejectPairing">{{ t('peers.reject') }}</ion-button>
        </div>
      </div>

      <p v-else class="confirmedText">{{ t('peers.confirmed') }}</p>
    </div>

    <p v-if="lastError" class="panelError">{{ lastError }}</p>
  </div>
</template>

<script setup lang="ts">
import { IonButton } from "@ionic/vue";
import { computed, onUnmounted, ref } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { createPairingTicket, fetchPeers, unpairPeer, usePeerLink, waitForPairing } from "@encv/shared-components/composables/usePeerLink";

const { t } = useI18n();
const { ticket, paired, busy, lastError } = usePeerLink();

type Phase = "idle" | "waiting" | "paired" | "confirmed" | "expired";
const phase = ref<Phase>("idle");
const remain = ref(0);
const qrReady = ref(false);
const qrDiag = ref("");
const copied = ref(false);
const canvasRef = ref<HTMLCanvasElement | null>(null);

let countdownTimer: ReturnType<typeof setInterval> | null = null;

/** 二维码负载：只含会合点与票据秘密，绝不含内网地址 */
const pairingCode = computed(() => {
  const tk = ticket.value;
  if (!tk) return "";
  return JSON.stringify({ v: 2, hub: tk.hub, pairingId: tk.pairingId, psk: tk.psk, exp: tk.expiresIn });
});

function clearTimers() {
  if (countdownTimer) {
    clearInterval(countdownTimer);
    countdownTimer = null;
  }
}

onUnmounted(clearTimers);

async function renderQR(text: string) {
  qrReady.value = false;
  try {
    // 动态依赖：未安装（或加载失败）时降级为手动输入，不自己实现编码
    const mod = (await import("qrcode")) as {
      toCanvas?: (c: HTMLCanvasElement, t: string, o?: unknown) => Promise<unknown> | unknown;
      default?: { toCanvas?: (c: HTMLCanvasElement, t: string, o?: unknown) => Promise<unknown> | unknown };
    };
    const toCanvas = mod?.toCanvas ?? mod?.default?.toCanvas;
    if (!toCanvas || !canvasRef.value) {
      qrDiag.value = `qrcode module shape: keys=${Object.keys(mod ?? {}).join(",")} default=${typeof mod?.default}`;
      return;
    }
    await toCanvas(canvasRef.value, text, { width: 220, margin: 1 });
    qrReady.value = true;
    qrDiag.value = "";
  } catch (e) {
    // ⚠️ 不吞错误：渲染失败要可见（今天 toCanvas 静默失败就是被这里吞了才查了半天）
    qrReady.value = false;
    qrDiag.value = e instanceof Error ? `${e.name}: ${e.message}` : String(e);
  }
}

async function generate() {
  clearTimers();
  phase.value = "waiting";
  qrReady.value = false;
  copied.value = false;
  try {
    const tk = await createPairingTicket();
    remain.value = tk.expiresIn || 120;
    countdownTimer = setInterval(() => {
      remain.value -= 1;
      if (remain.value <= 0) {
        clearTimers();
        if (phase.value === "waiting") phase.value = "expired";
      }
    }, 1000);
    await renderQR(pairingCode.value);

    const info = await waitForPairing(tk.pairingId);
    clearTimers();
    phase.value = "paired";
    void info;
  } catch (e) {
    clearTimers();
    if (e instanceof Error && e.message.includes("timeout")) {
      phase.value = "expired";
    } else {
      phase.value = "idle";
    }
  }
}

async function copyCode() {
  try {
    await navigator.clipboard.writeText(pairingCode.value);
    copied.value = true;
  } catch {
    copied.value = false;
  }
}

function confirmTrust() {
  // ⚠️ 前端"信任"只是本次交互确认；真正的 trust_device（进程内存、重启失效）在 P4 落地
  phase.value = "confirmed";
  // 配对完成后刷新设备列表（否则列表停留在"暂无已配对设备"）
  void fetchPeers().catch(() => undefined);
}

async function rejectPairing() {
  const id = paired.value?.peerId;
  if (id) {
    await unpairPeer(id).catch(() => undefined);
  }
  phase.value = "idle";
}
</script>

<style scoped>
.peerPanel {
  padding: 8px 0;
}

.panelHelp,
.panelMeta,
.sasHelp {
  color: var(--color-base-content);
  opacity: 0.75;
  font-size: var(--text-sm, 0.875rem);
  margin: 6px 0;
}

.panelExpired,
.panelError {
  color: var(--color-error, #b91c1c);
  font-size: var(--text-sm, 0.875rem);
}

.qrBox {
  display: flex;
  justify-content: center;
  padding: 12px;
  background: var(--color-base-100);
  border: 1px solid var(--color-base-300);
  border-radius: var(--radius-box, 0.75rem);
}

.qrCanvas {
  width: 220px;
  height: 220px;
}

.qrFallback {
  text-align: center;
  max-width: 260px;
}

.qrCodeText {
  display: block;
  font-size: 0.7rem;
  word-break: break-all;
  margin: 8px 0;
}

.panelCountdown {
  margin-left: 8px;
  font-variant-numeric: tabular-nums;
}

.pairedTitle {
  font-weight: 600;
  margin: 4px 0;
}

.sasBox {
  border: 1px solid var(--color-base-300);
  border-radius: var(--radius-box, 0.75rem);
  padding: 12px;
  text-align: center;
}

.sasTitle {
  font-weight: 600;
  margin: 0 0 6px;
}

.sasCode {
  font-size: 2rem;
  letter-spacing: 0.35em;
  font-variant-numeric: tabular-nums;
  font-family: ui-monospace, monospace;
  margin: 4px 0 8px;
}

.sasActions {
  display: flex;
  gap: 8px;
  justify-content: center;
}

.confirmedText {
  color: var(--color-success, #15803d);
  font-weight: 600;
}
</style>
