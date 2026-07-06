<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button @click="goBack" fill="clear">
            <ion-icon :icon="arrowBack" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
        <ion-title>{{ fileName || 'Player' }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading" class="loading-state">
        <ion-spinner name="crescent" class="loading-spinner"></ion-spinner>
        <p>{{ t('player.loading') }}</p>
      </div>

      <div v-else-if="error" class="error-state">
        <ErrorStateCard
          :error-type="errorType"
          :title="error"
          :details="errorDetails"
          @retry="retryPlay"
        />
      </div>

      <div v-else class="player-container">
        <div v-if="!playerError" ref="artContainer" class="video-player"></div>

        <div v-if="playerError" class="player-error">
          <ErrorStateCard
            :error-type="playerErrorType"
            :title="t('player.playError')"
            :details="playerErrorDetails"
            @retry="retryPlay"
          />
        </div>

        <div v-if="!playerError && !isFullscreen" class="video-info">
          <h3>{{ fileName }}</h3>
          <div v-if="mediaInfo.duration || mediaInfo.resolution" class="media-meta">
            <ion-chip v-if="mediaInfo.resolution" outline>
              <ion-icon :icon="resize" slot="start"></ion-icon>
              {{ mediaInfo.resolution }}
            </ion-chip>
            <ion-chip v-if="mediaInfo.duration" outline>
              <ion-icon :icon="time" slot="start"></ion-icon>
              {{ mediaInfo.duration }}
            </ion-chip>
          </div>
          <p v-if="filePath" class="video-path">{{ filePath }}</p>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  arrowBack,
  resize,
} from "ionicons/icons";

import { getAlistEncryptStreamUrl, getFileStreamUrl } from "@/api/encv";
import type { ErrorDetailItem, ErrorType } from "@/components/ErrorStateCard.vue";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import { isNative } from "@/plugins/GoProcess";
import type Artplayer from "artplayer";
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

const TAG = "[ArtPlayer]";

const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const loading = ref(true);
const error = ref("");
const errorType = ref<ErrorType>("unknown");
const errorDetails = ref<ErrorDetailItem[]>([]);
const filePath = ref("");
const fileName = ref("");
const overrideStreamUrl = ref("");
const alistPath = ref("");
const alistPassword = ref("");
const playerError = ref(false);
const playerErrorMsg = ref("");
const playerErrorType = ref<ErrorType>("playback_failed");
const playerErrorDetails = ref<ErrorDetailItem[]>([]);
const isFullscreen = ref(false);
const mediaInfo = ref({ duration: "", resolution: "" });
const artContainer = ref<HTMLDivElement | null>(null);
let art: Artplayer | null = null;
let initRetryCount = 0;
const MAX_INIT_RETRY = 4;

const streamUrl = computed(() => {
  if (overrideStreamUrl.value) return overrideStreamUrl.value;
  // ★ alist-encrypt 流式预览：直接从 path + password 构造 URL，避免 query 二次编码
  if (alistPath.value && alistPassword.value) {
    return getAlistEncryptStreamUrl({ path: alistPath.value, password: alistPassword.value });
  }
  if (!filePath.value) return "";
  return getFileStreamUrl(filePath.value);
});

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function goBack() {
  destroyArtPlayer();
  router.back();
}

function hideNativeControls() {
  if (!art?.video) return;
  art.video.removeAttribute("controls");
  art.video.controls = false;
  art.video.setAttribute("playsinline", "");
  art.video.setAttribute("webkit-playsinline", "");
  art.video.setAttribute("x5-playsinline", "");
  art.video.setAttribute("x5-video-player-type", "h5");
}

async function handleFullscreenEnter() {
  isFullscreen.value = true;
  if (!isNative()) return;
  try {
    const { StatusBar } = await import("@capacitor/status-bar");
    const { ScreenOrientation } = await import("@capacitor/screen-orientation");
    await StatusBar.hide();
    const video = art?.video;
    if (video?.videoWidth && video?.videoHeight) {
      const ratio = video.videoWidth / video.videoHeight;
      if (ratio > 1.3) {
        await ScreenOrientation.lock({ orientation: "landscape" });
      } else if (ratio < 0.77) {
        await ScreenOrientation.lock({ orientation: "portrait" });
      } else {
        await ScreenOrientation.lock({ orientation: "landscape" });
      }
    } else {
      await ScreenOrientation.lock({ orientation: "landscape" });
    }
  } catch (e) {
    console.debug(TAG, "handleFullscreenEnter error:", e);
  }
}

async function handleFullscreenExit() {
  isFullscreen.value = false;
  if (!isNative()) return;
  try {
    const { ScreenOrientation } = await import("@capacitor/screen-orientation");
    const { StatusBar, Style } = await import("@capacitor/status-bar");
    await ScreenOrientation.lock({ orientation: "portrait" });
    await StatusBar.show();
    await StatusBar.setStyle({ style: Style.Default });
  } catch (e) {
    console.debug(TAG, "handleFullscreenExit error:", e);
  }
}

/**
 * 构建错误详情数组 —— 给 ErrorStateCard 渲染调试信息
 * 包含：URL、文件路径、媒体类型猜测、当前网络状态、readyState、错误阶段
 */
function buildPlayerErrorDetails(networkState?: number, readyState?: number, src?: string, stage?: string): ErrorDetailItem[] {
  const items: ErrorDetailItem[] = [];
  if (fileName.value) items.push({ label: "文件名", value: fileName.value });
  if (filePath.value) items.push({ label: "容器路径", value: filePath.value });
  items.push({ label: "媒体类型", value: detectMediaType(filePath.value) });
  if (streamUrl.value) items.push({ label: "流地址", value: streamUrl.value, copyable: true });
  if (src) items.push({ label: "video.src", value: src });
  if (networkState !== undefined) {
    items.push({ label: "networkState", value: networkStateToText(networkState) });
  }
  if (readyState !== undefined) {
    items.push({ label: "readyState", value: readyStateToText(readyState) });
  }
  if (stage) items.push({ label: "错误阶段", value: stage });
  items.push({ label: "时间戳", value: new Date().toISOString() });
  return items;
}

/**
 * 推断错误类型
 * - networkState=3 NETWORK_NO_SOURCE → network_error（HTML fallback / 404 / 网关拦截）
 * - readyState=0 + network error → network_error
 * - mp4 解码错 → format_error
 * - trae 网关 401/HTML 响应 → gateway_error
 */
function detectErrorType(networkState?: number, readyState?: number): ErrorType {
  if (networkState === 3 || (readyState === 0 && networkState === 3)) {
    return "network_error";
  }
  if (networkState === 2) return "format_error";
  if (readyState === 0) return "network_error";
  return "playback_failed";
}

function networkStateToText(n: number): string {
  return (
    ["0 NETWORK_EMPTY（尚未初始化）", "1 NETWORK_IDLE（空闲）", "2 NETWORK_LOADING（加载中）", "3 NETWORK_NO_SOURCE（找不到资源）"][n] ||
    `${n} (未知)`
  );
}

function readyStateToText(r: number): string {
  return (
    [
      "0 HAVE_NOTHING（无数据）",
      "1 HAVE_METADATA（有元数据）",
      "2 HAVE_CURRENT_DATA（当前帧可用）",
      "3 HAVE_FUTURE_DATA（未来帧可用）",
      "4 HAVE_ENOUGH_DATA（数据充足）",
    ][r] || `${r} (未知)`
  );
}

function detectMediaType(path: string): string {
  if (!path) return "未知";
  const ext = path.split(".").pop()?.toLowerCase() || "";
  const map: Record<string, string> = {
    mp4: "video/mp4",
    m4v: "video/x-m4v",
    webm: "video/webm",
    mov: "video/quicktime",
    mkv: "video/x-matroska",
    avi: "video/x-msvideo",
    ts: "video/mp2t",
    mp3: "audio/mpeg",
    m4a: "audio/mp4",
    aac: "audio/aac",
    wav: "audio/wav",
    flac: "audio/flac",
    ogg: "audio/ogg",
  };
  return map[ext] || `${ext || "(无后缀)"} (未识别)`;
}

async function initArtPlayer() {
  console.info(TAG, "initArtPlayer called, retry=", initRetryCount);
  const { default: Artplayer } = await import("artplayer");

  // ★ 关键：artContainer 必须有真实 DOM 节点。Vue 嵌套 v-if + nextTick 偶发失败，
  // 这里加重试机制确保万无一失。
  if (!artContainer.value) {
    if (initRetryCount < MAX_INIT_RETRY) {
      initRetryCount++;
      const delay = [50, 150, 350, 800][initRetryCount - 1] ?? 500;
      console.warn(TAG, `artContainer is null, retry ${initRetryCount}/${MAX_INIT_RETRY} in ${delay}ms`);
      await new Promise(r => setTimeout(r, delay));
      return initArtPlayer();
    }
    // 实在拿不到 → 显示错误
    console.error(TAG, "initArtPlayer: artContainer is null after retries, cannot init");
    playerError.value = true;
    playerErrorType.value = "init_failed";
    playerErrorMsg.value = "播放器容器未就绪";
    playerErrorDetails.value = buildPlayerErrorDetails(undefined, undefined, undefined, "init:artContainer-null");
    return;
  }

  if (!streamUrl.value) {
    console.error(TAG, "initArtPlayer: streamUrl is empty, cannot init");
    playerError.value = true;
    playerErrorType.value = "init_failed";
    playerErrorMsg.value = "播放地址为空";
    playerErrorDetails.value = buildPlayerErrorDetails(undefined, undefined, undefined, "init:empty-streamUrl");
    return;
  }

  const containerWidth = artContainer.value.clientWidth || window.innerWidth;
  artContainer.value.style.minHeight = "200px";
  artContainer.value.style.maxHeight = `${window.innerHeight - 56}px`;
  console.info(TAG, "container size:", containerWidth, "| streamUrl:", streamUrl.value);

  try {
    art = new Artplayer({
      container: artContainer.value,
      url: streamUrl.value,
      autoplay: true,
      autoSize: true,
      autoMini: true,
      mutex: true,
      playsInline: true,
      theme: "#ffad00",
      volume: 0.7,
      fullscreen: true,
      fullscreenWeb: !isNative(),
      miniProgressBar: true,
      setting: true,
      playbackRate: true,
      aspectRatio: true,
      flip: true,
      lock: true,
      autoOrientation: true,
      autoPlayback: true,
      subtitleOffset: true,
      fastForward: true,
      hotkey: true,
      gesture: true,
      moreVideoAttr: {
        controls: false,
        preload: "metadata",
        playsInline: true,
      },
    });
    console.info(TAG, "Artplayer instance created successfully, id:", art.id);
  } catch (e: any) {
    console.error(TAG, "Artplayer constructor failed:", e?.message || String(e));
    playerError.value = true;
    playerErrorType.value = "init_failed";
    playerErrorMsg.value = `ArtPlayer 初始化失败: ${e?.message || String(e)}`;
    playerErrorDetails.value = buildPlayerErrorDetails(undefined, undefined, undefined, `init:throw:${e?.name || "Error"}`);
    return;
  }

  art.on("ready", () => {
    console.info(TAG, "Artplayer ready event fired");
    hideNativeControls();
  });

  art.on("video:loadedmetadata", () => {
    const video = art?.video;
    console.info(
      TAG,
      "video:loadedmetadata, videoWidth:",
      video?.videoWidth,
      "videoHeight:",
      video?.videoHeight,
      "duration:",
      video?.duration
    );
    if (video) {
      if (video.videoWidth && video.videoHeight) {
        mediaInfo.value.resolution = `${video.videoWidth}×${video.videoHeight}`;
        const ratio = video.videoHeight / video.videoWidth;
        const containerWidth = artContainer.value?.clientWidth || window.innerWidth;
        const naturalHeight = Math.round(containerWidth * ratio);
        const maxHeight = window.innerHeight - 56;
        const finalHeight = Math.min(naturalHeight, maxHeight);
        if (artContainer.value) {
          artContainer.value.style.height = `${finalHeight}px`;
        }
      }
      if (video.duration && Number.isFinite(video.duration)) {
        mediaInfo.value.duration = formatDuration(video.duration);
      }
    }
    hideNativeControls();
  });

  art.on("video:play", () => {
    console.info(TAG, "video:play");
    hideNativeControls();
  });

  art.on("video:playing", () => {
    console.info(TAG, "video:playing");
    hideNativeControls();
  });

  art.on("fullscreen", (state: boolean) => {
    if (state) {
      handleFullscreenEnter();
    } else {
      handleFullscreenExit();
    }
  });

  art.on("error", () => {
    const video = art?.video;
    const networkState = video?.networkState;
    const readyState = video?.readyState;
    const src = video?.src;
    const currentSrc = video?.currentSrc;
    console.error(
      TAG,
      "Artplayer error event, networkState:",
      networkState,
      "readyState:",
      readyState,
      "src:",
      src,
      "currentSrc:",
      currentSrc
    );
    playerError.value = true;
    playerErrorType.value = detectErrorType(networkState, readyState);
    playerErrorMsg.value = `播放失败 (network=${networkState ?? "?"}, ready=${readyState ?? "?"})`;
    playerErrorDetails.value = buildPlayerErrorDetails(networkState, readyState, currentSrc || src, "play:error-event");
    showToast({ message: t("player.playFailed", { name: fileName.value }), duration: 3000, color: "danger" });
  });

  art.on("destroy", () => {
    console.info(TAG, "Artplayer destroy event");
  });

  nextTick(() => {
    hideNativeControls();
  });

  setTimeout(() => {
    hideNativeControls();
  }, 500);

  setTimeout(() => {
    hideNativeControls();
  }, 2000);
}

function destroyArtPlayer() {
  if (art) {
    console.info(TAG, "destroyArtPlayer: destroying art instance");
    art.destroy();
    art = null;
  }
}

function retryPlay() {
  console.info(TAG, "retryPlay called");
  playerError.value = false;
  playerErrorMsg.value = "";
  playerErrorDetails.value = [];
  mediaInfo.value = { duration: "", resolution: "" };
  initRetryCount = 0;
  destroyArtPlayer();
  nextTick(() => initArtPlayer());
}

async function startPlayback() {
  console.info(TAG, "startPlayback called, filePath:", filePath.value, "streamUrl:", streamUrl.value);
  if (!filePath.value) {
    console.error(TAG, "startPlayback: filePath is empty");
    return;
  }
  loading.value = false;
  playerError.value = false;
  playerErrorMsg.value = "";
  playerErrorDetails.value = [];
  mediaInfo.value = { duration: "", resolution: "" };
  await nextTick();
  console.info(TAG, "startPlayback: nextTick done, artContainer:", artContainer.value ? "exists" : "null");
  initArtPlayer();
}

onMounted(() => {
  filePath.value = (route.query.path as string) || "";
  fileName.value = (route.query.name as string) || "";

  const qsu = route.query.streamUrl as string;
  if (qsu) overrideStreamUrl.value = qsu;

  alistPath.value = (route.query.alistPath as string) || "";
  alistPassword.value = (route.query.alistPassword as string) || "";

  console.info(
    TAG,
    "onMounted: filePath=",
    filePath.value,
    "fileName=",
    fileName.value,
    "overrideStreamUrl=",
    overrideStreamUrl.value,
    "alistPath=",
    alistPath.value,
    "hasPassword=",
    !!alistPassword.value
  );

  if (!filePath.value && !overrideStreamUrl.value && !alistPath.value) {
    error.value = "No file provided";
    errorType.value = "init_failed";
    errorDetails.value = [
      { label: "错误阶段", value: "mount:no-input" },
      { label: "route.query", value: JSON.stringify(route.query) },
    ];
    loading.value = false;
    return;
  }

  startPlayback();
});

onBeforeUnmount(async () => {
  console.info(TAG, "onBeforeUnmount");
  destroyArtPlayer();
  if (isNative()) {
    try {
      const { ScreenOrientation } = await import("@capacitor/screen-orientation");
      const { StatusBar, Style } = await import("@capacitor/status-bar");
      await ScreenOrientation.lock({ orientation: "portrait" });
      await StatusBar.show();
      await StatusBar.setStyle({ style: Style.Default });
    } catch {}
  }
});
</script>

<style scoped>
:deep(video) {
  outline: none !important;
}

:deep(video::-webkit-media-controls) {
  display: none !important;
}

:deep(video::-webkit-media-controls-enclosure) {
  display: none !important;
}

:deep(video::-webkit-media-controls-panel) {
  display: none !important;
}

:deep(.art-video-player) {
  --art-control-height: 44px;
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 24px;
  text-align: center;
  color: var(--encv-text-secondary);
}

.loading-spinner {
  width: 48px;
  height: 48px;
  margin-bottom: 16px;
}

.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100%;
  padding: 24px 16px;
  background: linear-gradient(180deg, transparent 0%, var(--ion-background-color, #fff) 100%);
}

.player-container {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.video-player {
  width: 100%;
  background: #000;
  position: relative;
  overflow: hidden;
}

.video-info {
  padding: 16px;
}

.media-meta {
  display: flex;
  gap: 8px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.video-path {
  font-size: 12px;
  color: var(--encv-text-secondary);
  word-break: break-all;
  margin-top: 8px;
}

.player-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 50vh;
  padding: 24px 16px;
  width: 100%;
}
</style>
