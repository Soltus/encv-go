<template>
  <div class="scanPanel">
    <h3 class="scanTitle">{{ t('peers.scanTitle') || '扫码连接另一台设备' }}</h3>
    <p class="scanHelp">{{ t('peers.scanHelp') || '扫描桌面端显示的配对码；连接由本机服务保持，息屏不影响' }}</p>

    <div class="scanRow">
      <ion-button size="small" :disabled="busy" data-testid="scan-start" @click="startScan">
        {{ t('peers.scanStart') || '开始扫码' }}
      </ion-button>
      <ion-button size="small" fill="outline" :disabled="busy" data-testid="edge-refresh" @click="refreshStatus">
        {{ t('peers.edgeStatus') || '互联状态' }}
      </ion-button>
      <!-- 从相册选择：相机不可用/不想扫时的第二条通路（纯 JS 解码，WebView 与桌面浏览器同一份代码） -->
      <ion-button size="small" fill="outline" :disabled="busy" data-testid="gallery-pick" @click="pickFromGallery">
        {{ t('peers.pickFromGallery') || '从相册选择' }}
      </ion-button>
      <input
        ref="fileRef"
        class="qrFileInput"
        type="file"
        accept="image/*"
        data-testid="qr-file-input"
        @change="onFilePicked"
      />
    </div>

    <p v-if="galleryMsg" class="scanNote" data-testid="gallery-msg">{{ galleryMsg }}</p>

    <p v-if="!nativeCamera" class="scanNote" data-testid="scan-no-camera">
      {{ t('peers.scanNoCamera') || '当前环境没有相机（非原生环境），请改用下方「粘贴配对码」' }}
    </p>

    <div class="pasteRow">
      <label class="pasteLabel" for="peer-paste">{{ t('peers.pasteLabel') || '粘贴配对码' }}</label>
      <textarea
        id="peer-paste"
        v-model="pasted"
        class="pasteInput"
        rows="3"
        data-testid="paste-input"
        :placeholder="t('peers.pastePlaceholder') || '粘贴桌面端「复制配对码」得到的文本'"
        spellcheck="false"
      ></textarea>
      <ion-button size="small" :disabled="busy || !pasted.trim()" data-testid="paste-submit" @click="connectWithPasted">
        {{ t('peers.pasteSubmit') || '连接' }}
      </ion-button>
    </div>

    <!-- ⚠️ 结果/错误必须可见（禁止静默失败）：三处文案都渲染到 DOM -->
    <p v-if="okMsg" class="scanOk" data-testid="scan-ok">{{ okMsg }}</p>
    <p v-if="errMsg" class="scanErr" data-testid="scan-error">{{ errMsg }}</p>
    <p v-if="hintMsg" class="scanNote" data-testid="scan-hint">{{ hintMsg }}</p>

    <!-- 🆕 2026-10-04：扫码端的 SAS 核对区。
         桌面端点「一致，信任该设备」时手里有 6 位安全码，而手机端此前**什么都不显示**
         ⇒ 用户不知道该拿什么去比对（真机反馈：困惑）。这里把本端 Go 从 Hub 拿到的
         SAS 显示出来，并说明"只有两端一致时才在桌面点信任"。 -->
    <div v-if="sasCode" class="sasBox" data-testid="scan-sas">
      <p class="sasTitle">{{ t('peers.sasTitle') || '核对安全码' }}</p>
      <div class="sasCode" data-testid="scan-sas-code">{{ sasCode }}</div>
      <p class="sasHelp">
        {{ t('peers.sasPhoneHelp') || '请在桌面端核对这个 6 位安全码；两端一致才在桌面点「信任该设备」，不一致请立即取消配对' }}
      </p>
      <ion-button
        v-if="pairedPeerId"
        size="small"
        color="danger"
        fill="outline"
        data-testid="scan-unpair"
        @click="unpairCurrent"
      >
        {{ t('peers.unpair') || '取消配对' }}
      </ion-button>
    </div>

    <!-- 🆕 2026-10-04：Edge 侧的**持久**已连接状态。
         旧实现只在"刚配对完那一刻"用组件本地 okMsg/sasCode 显示，重进设置页（组件重建）
         就什么都不剩了 ⇒ 真机反馈"重进设置丢失状态显示"。这里改从 /edge/status 恢复。 -->
    <div v-if="linkedHub" class="linkedBox" data-testid="edge-linked">
      <p class="linkedTitle">
        {{ String(t('peers.edgeLinked') || '已连接到会合点：{host}').replace('{host}', hostOf(linkedHub)) }}
      </p>
      <ion-button size="small" fill="outline" data-testid="edge-disconnect" @click="disconnectEdge">
        {{ t('peers.edgeDisconnect') || '断开连接' }}
      </ion-button>
    </div>

    <p class="edgeStatus" data-testid="edge-status">
      {{ t('peers.edgeStatus') || '互联状态' }}：{{ edgeLabel }}
    </p>
    <!-- ⚠️ 连接失败必须可见（2026-10-04）：以前 running=true/connected=false 时只显示
         "连接中…"，用户不知道是还在连、还是根本连不上（真机反馈：困惑）。 -->
    <p v-if="edgeErr" class="scanErr" data-testid="edge-error">{{ edgeErr }}</p>
    <p v-if="edgeErr" class="scanNote" data-testid="edge-error-hint">
      {{ t('peers.edgeFailedHint') || '常见原因：会合点地址手机无法访问，或该地址不支持 WebSocket 升级（例如经某些代理/网关访问时）' }}
    </p>
    <!-- 🆕 2026-10-05：会合点**地址本身没了**（域名随容器重建就变）时，
         光说"连不上"没用 —— 用户唯一的出路是重新扫码，必须显式给出来。 -->
    <div v-if="edgeStale" class="staleBox" data-testid="edge-stale">
      <p class="scanErr">
        {{ t('peers.edgeStale') || '会合点地址可能已失效（域名可能已变更）：已重试多次仍连不上。请重新扫码连接。' }}
      </p>
      <ion-button size="small" fill="outline" data-testid="edge-forget" @click="forgetAndRescan">
        {{ t('peers.edgeForget') || '忘记这个会合点' }}
      </ion-button>
    </div>
  </div>
</template>

<script setup lang="ts">
// PeerScanPanel.vue —— P2b Task 2.6：扫码端（安卓）扫桌面端二维码 → 本端 Go 作为 Edge 连 Hub
//
// 链路：相机（或粘贴）→ parsePairingQR → POST /api/peerlink/edge/pair（本端 Go 后端）
//       → Go 侧 Edge 主动出网连 Hub 并常驻（不在 WebView 里，息屏/重建不断链）。
// ⚠️ psk 只在内存流转（局部变量传参），**不落 localStorage**（与 usePeerLink 同一纪律）。
import { IonButton } from "@ionic/vue";
import { Capacitor } from "@capacitor/core";
import { onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  type EdgeStatus,
  fetchEdgeStatus,
  PairEdgeError,
  pairAsEdge,
  parsePairingQR,
} from "@encv/shared-components/composables/usePeerLink";
import { ScanError, scanOnce } from "@/peerlink/barcodeScanner";
import { QRDecodeError, decodeQRFromImageFile } from "@/peerlink/qrFromImage";
import { stopEdge, unpairPeer } from "@encv/shared-components/composables/usePeerLink";

const { t } = useI18n();

const fileRef = ref<HTMLInputElement | null>(null);
const galleryMsg = ref("");
const busy = ref(false);
const pasted = ref("");
const okMsg = ref("");
const errMsg = ref("");
// 失败之后的"下一步该做什么"（2026-10-05：只说失败不说办法，用户只能瞎重试）
const hintMsg = ref("");
const sasCode = ref("");
const pairedPeerId = ref("");
// 已连接的会合点（从 /edge/status 恢复 ⇒ 重进页面也不会丢）
const linkedHub = ref("");
const nativeCamera = Capacitor.isNativePlatform();
const edge = ref<EdgeStatus | null>(null);

const edgeLabel = ref(t("peers.edgeIdle") || "未连接（服务重启后需重新扫码）");
const edgeErr = ref("");
// 会合点地址已失效（连不上且重试多次）⇒ 必须给用户"重新扫码"这条出路
const edgeStale = ref(false);
// 配对后的状态巡查：连不上时不能一直停在"连接中…"，要让用户看到真实结果
let pollTimer: ReturnType<typeof setInterval> | null = null;

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

/** 配对后轮询 Edge 状态，直到连上 / 报错 / 超时（60s） */
function startPollingUntilSettled() {
  stopPolling();
  let waited = 0;
  pollTimer = setInterval(async () => {
    await refreshStatus();
    waited += 3000;
    const st = edge.value;
    const settled = (st && st.connected) || !!edgeErr.value || !(st && st.running) || waited >= 60_000;
    if (settled) stopPolling();
  }, 3000);
}

/** 取会合点的主机名（回显给用户看），解析失败则原样返回 */
function hostOf(hubUrl: string): string {
  try {
    return new URL(hubUrl).host;
  } catch {
    return hubUrl;
  }
}

/** 取消配对（两端安全码不一致时给用户的退出路径） */
async function unpairCurrent() {
  const id = pairedPeerId.value;
  if (!id) return;
  busy.value = true;
  try {
    await unpairPeer(id);
    sasCode.value = "";
    pairedPeerId.value = "";
    okMsg.value = t("peers.unpaired") || "已解除配对";
    await refreshStatus();
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}

function errTextFor(reason: string, detail?: string): string {
  switch (reason) {
    case "not_json":
      return t("peers.codeInvalid") || "配对码无法识别（不是有效的配对码）";
    case "missing_field":
      return t("peers.codeMissingField") || "配对码缺少必需字段（hub / pairingId / psk）";
    case "bad_hub":
      return t("peers.codeBadHub") || "配对码里的会合点地址不被允许（必须 https，或本机回环）";
    case "expired":
      return t("peers.codeExpired") || "配对码已过期，请在桌面端刷新后重新扫码";
    // 🆕 2026-10-05：后端把失败原因码带回来了（此前一律"连接失败"，
    //    用户拿着过期/用过的码反复扫，界面却没有任何"刷新二维码"的引导）。
    //    这三种都是**用户自己能解决**的：回桌面端点一下"重新生成"再来。
    case "ticket_expired":
      return t("peers.codeTicketExpired") || "配对码已过期，请在桌面端刷新二维码后重新扫描";
    case "ticket_used":
      return t("peers.codeTicketUsed") || "这个配对码已经被用过（一张码只能配一次），请在桌面端刷新二维码后重新扫描";
    case "ticket_not_found":
      return t("peers.codeTicketNotFound") || "配对码无效（桌面端可能已重启），请在桌面端刷新二维码后重新扫描";
    default:
      return String(t("peers.pairFailed") || "连接失败：{detail}").replace("{detail}", detail ?? reason);
  }
}

/** 解析 → 调 /edge/pair。扫码与粘贴走同一条路径（相机不可用时也能连通）。 */
async function connectWithText(text: string) {
  busy.value = true;
  okMsg.value = "";
  errMsg.value = "";
  sasCode.value = "";
  pairedPeerId.value = "";
  try {
    const parsed = parsePairingQR(text);
    if (!parsed.ok) {
      errMsg.value = errTextFor(parsed.reason);
      return;
    }
    const res = await pairAsEdge({
      hub: parsed.payload.hub,
      pairingId: parsed.payload.pairingId,
      psk: parsed.payload.psk,
      name: "Android",
    });
    // ⚠️ 不要回显裸 URL（真机截图里是一长串 http://127.0.0.1:8100/api/peerlink，
    //    对用户毫无意义）⇒ 只显示会合点主机名。
    okMsg.value = String(t("peers.pairOk") || "已连接：{hub}").replace("{hub}", hostOf(res.hub));
    sasCode.value = res.sas || "";
    pairedPeerId.value = res.peerId || "";
    await refreshStatus();
    // 配对完立刻开始巡查：连上 / 失败 / 60s 超时后停止，别让用户盯着"连接中…"
    startPollingUntilSettled();
  } catch (e) {
    // ⚠️ 优先用后端给的原因码：一律显示"连接失败"会让"码过期了"与"连不上会合点"
    //    在用户看来毫无区别（真机反馈：只能反复重试同一个注定失败的动作）。
    if (e instanceof PairEdgeError) {
      errMsg.value = errTextFor(e.reason, e.message);
      // 票据类失败额外给一条"怎么做"的提示，而不是只说"失败了"
      hintMsg.value = e.refreshQr ? t("peers.refreshQrHint") || "解决办法：回到桌面端点「重新生成」，再扫新码" : "";
    } else {
      errMsg.value = errTextFor("unknown", e instanceof Error ? e.message : String(e));
      hintMsg.value = "";
    }
  } finally {
    busy.value = false;
  }
}

async function startScan() {
  try {
    const text = await scanOnce();
    await connectWithText(text);
  } catch (e) {
    busy.value = false;
    if (e instanceof ScanError) {
      errMsg.value = `${e.reason}: ${e.message}`;
    } else {
      errMsg.value = e instanceof Error ? e.message : String(e);
    }
  }
}

function connectWithPasted() {
  void connectWithText(pasted.value);
}

/**
 * 断开本端 Edge（手机侧解除与对端的连接）。
 * 手机是 Edge，`/peers` 只有 Hub 才有 ⇒ 这里不能用 unpairPeer(peerId)，
 * 必须走 `/edge/stop`（2026-10-04 补的封装）。
 */
async function disconnectEdge() {
  busy.value = true;
  try {
    const ok = await stopEdge();
    if (!ok) throw new Error("edge/stop failed");
    sasCode.value = "";
    pairedPeerId.value = "";
    linkedHub.value = "";
    okMsg.value = t("peers.edgeDisconnected") || "已断开连接";
    await refreshStatus();
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}

/** 打开系统相册/文件选择器（原生与桌面浏览器都是这个 file input）。 */
function pickFromGallery() {
  galleryMsg.value = t("peers.pickFromGalleryHint") || "请选择含有配对码二维码的截图";
  fileRef.value?.click();
}

/** 选图 → 解码 → 走**同一条** connectWithText（与扫码、粘贴共享连通逻辑）。 */
async function onFilePicked(e: Event) {
  const input = e.target as HTMLInputElement | null;
  const file = input?.files?.[0];
  if (!file) return;
  busy.value = true;
  okMsg.value = "";
  errMsg.value = "";
  try {
    const text = await decodeQRFromImageFile(file);
    galleryMsg.value = String(t("peers.galleryDecoded") || "已从图片识别到配对码：{len} 字符").replace("{len}", String(text.length));
    await connectWithText(text);
  } catch (err) {
    // ⚠️ 失败必须可见（不静默）：解码失败/图片不对都渲染出来
    const msg = err instanceof QRDecodeError || err instanceof Error ? err.message : String(err);
    galleryMsg.value = "";
    errMsg.value = String(t("peers.galleryDecodeFailed") || "从图片识别二维码失败：{detail}").replace("{detail}", msg);
    busy.value = false;
  } finally {
    // 允许重复选同一张图（否则第二次选同一文件不触发 change）
    if (input) input.value = "";
  }
}

/**
 * 忘记这个会合点（2026-10-05）：清掉落盘的 Edge 会话，让用户能干净地重新扫码。
 *
 * 为什么需要它：地址失效后如果不主动忘记，下次启动又会自动连那个已经不存在的地址，
 * 用户将**永远**卡在"连不上" —— 这正是"持久化的坏值没有自愈路径"的典型形态。
 */
async function forgetAndRescan() {
  busy.value = true;
  try {
    await stopEdge();
    linkedHub.value = "";
    edgeStale.value = false;
    edgeErr.value = "";
    okMsg.value = t("peers.edgeForgotten") || "已忘记该会合点，请重新扫码连接";
    await refreshStatus();
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}

async function refreshStatus() {
  try {
    const st = await fetchEdgeStatus();
    edge.value = st;
    if (!st.running) {
      edgeLabel.value = t("peers.edgeIdle") || "未连接（服务重启后需重新扫码）";
      edgeErr.value = "";
      linkedHub.value = "";
      edgeStale.value = false;
    } else if (st.connected) {
      edgeStale.value = false;
      edgeLabel.value = t("peers.edgeConnected") || "已连上会合点";
      edgeErr.value = "";
      linkedHub.value = st.hub || "";
    } else if (st.lastErr) {
      linkedHub.value = "";
      // 连不上：把原因摆出来（禁止用"连接中…"掩盖失败）
      edgeLabel.value = t("peers.edgeFailed") || "连接失败";
      edgeErr.value = String(t("peers.edgeFailedDetail") || "连不上会合点：{detail}（已重试 {n} 次）")
        .replace("{detail}", st.lastErr)
        .replace("{n}", String(st.attempts ?? 0));
    } else {
      edgeLabel.value = t("peers.edgeRunning") || "连接中…";
      edgeErr.value = "";
      // 连不上且已重试多次 ⇒ 这已经不是"还在连"，而是"地址大概没了"
      edgeStale.value = st.stale === true;
    }
  } catch (e) {
    edgeLabel.value = e instanceof Error ? e.message : String(e);
    edgeErr.value = "";
  }
}

onMounted(() => {
  void refreshStatus();
});

onUnmounted(() => {
  stopPolling();
});
</script>

<style scoped>
.scanPanel {
  padding: 8px 0 12px;
}

.scanTitle {
  font-size: 0.95rem;
  font-weight: 600;
  margin: 0 0 4px;
}

.scanHelp,
.scanNote {
  font-size: 0.75rem;
  opacity: 0.75;
  margin: 4px 0;
}

.scanRow,
.pasteRow {
  display: flex;
  gap: 8px;
  align-items: center;
  margin: 6px 0;
}

/* file input 只作为"选图"的载体（按钮由 ion-button 承担），视觉上隐藏但仍可点击/可被测试选中 */
.qrFileInput {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.pasteRow {
  flex-direction: column;
  align-items: stretch;
}

.pasteLabel {
  font-size: 0.75rem;
  opacity: 0.8;
}

.pasteInput {
  width: 100%;
  padding: 8px 10px;
  font-family: ui-monospace, monospace;
  font-size: 0.72rem;
  border: 1px solid var(--color-base-300);
  border-radius: var(--radius-field, 0.5rem);
  background: var(--color-base-100);
  color: var(--color-base-content);
  resize: vertical;
}

.scanOk {
  font-size: 0.78rem;
  color: var(--color-success, #16a34a);
  margin: 6px 0;
  word-break: break-all;
}

.scanErr {
  font-size: 0.78rem;
  color: var(--color-error, #b91c1c);
  margin: 6px 0;
  word-break: break-all;
}

.edgeStatus {
  font-size: 0.75rem;
  opacity: 0.8;
  margin: 6px 0 0;
}

/* 已连接（持久）：与 .sasBox 同款容器，但语义是"当前连着谁" */
.linkedBox {
  margin: 10px 0;
  padding: 10px 12px;
  border: 1px solid var(--color-base-300);
  border-radius: var(--radius-field, 0.5rem);
}

.linkedTitle {
  font-size: 0.8rem;
  font-weight: 600;
  margin: 0 0 8px;
  word-break: break-all;
}

/* 扫码端 SAS 核对区：与桌面端 PeerPairingPanel 的 .sasBox 同款视觉，
   让用户在两端看到的是"同一个东西" */
.sasBox {
  margin: 10px 0;
  padding: 10px 12px;
  border: 1px dashed var(--color-base-300);
  border-radius: var(--radius-field, 0.5rem);
}

.sasTitle {
  font-size: 0.8rem;
  font-weight: 600;
  margin: 0 0 6px;
}

.sasCode {
  font-family: ui-monospace, monospace;
  font-size: 1.9rem;
  font-weight: 700;
  letter-spacing: 0.35em;
  margin: 4px 0 6px;
}

.sasHelp {
  font-size: 0.72rem;
  opacity: 0.8;
  margin: 0 0 8px;
}
</style>
