<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>
          <span class="home-title">OpenList</span>
          <span v-if="isDevPreview" class="preview-mini-chip">🔥 PREVIEW</span>
          <span v-if="!isDevPreview" class="version-mini">v{{ version }}</span>
        </ion-title>
        <ion-buttons slot="end">
          <ion-button @click="openPasswordDialog" title="设置管理员密码">
            <ion-icon :icon="keyOutline" slot="icon-only" />
          </ion-button>
          <ion-button @click="goToConfig" title="编辑 Config">
            <ion-icon :icon="codeSlashOutline" slot="icon-only" />
          </ion-button>
          <ion-button @click="goToDevLogs" title="日志查看">
            <ion-icon :icon="documentTextOutline" slot="icon-only" />
          </ion-button>
          <ion-button @click="goToWebView" title="OpenList Web UI">
            <ion-icon :icon="globeOutline" slot="icon-only" />
          </ion-button>
          <ion-button @click="goToSettings" title="设置">
            <ion-icon :icon="settingsOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!--
        复用本地 components-shared 共享状态卡
      -->
      <OpenListStatusCard :runtime="runtime" />

      <!--
        沙箱 dev preview 模式防御性提示：
        - backend :5244 是独立进程（不是 Capacitor 内嵌），UI 启停按钮无效
        - 用卡片提示用户到终端控制
        - 后端真实运行状态从 /__openlist-health 探测（实时）
      -->
      <div v-if="isDevPreview" class="dev-preview-notice">
        <div class="dev-preview-notice-row">
          <span class="notice-icon">🛠</span>
          <span class="notice-title">沙箱 Preview 模式</span>
        </div>
        <div class="dev-preview-notice-text">
          OpenList 后端在沙箱下由 <code>/tmp/openlist</code> 独立进程提供（<code>:5244</code>），
          <strong>UI 启停按钮不可用</strong>。
          <br />
          终端控制：<code>bash scripts/dev-openlist.sh</code> 启停。
          <br />
          实时状态：<span :class="backendOnline ? 'ok-text' : 'error-text'">
            <strong>{{ backendOnline ? '● 在线' : '● 离线' }}</strong>
          </span>
          <span v-if="backendOnline" class="muted small">（{{ backendLatency }}ms）</span>
        </div>
      </div>

      <!-- 复用本地 components-shared 共享日志列表 -->
      <OpenListLogList :logs="logs" />

      <!--
        启动 / 停止 FAB
        - 真机模式：正常启停（gomobile 内嵌 OpenList backend）
        - dev preview 模式：隐藏（不可用，由 :5244 进程控制）
      -->
      <ion-fab
        v-if="!isDevPreview"
        vertical="bottom"
        horizontal="end"
        slot="fixed"
      >
        <ion-fab-button
          :color="runtime.running ? 'danger' : 'primary'"
          @click="toggleService"
          :disabled="isControlling"
        >
          <ion-spinner v-if="isControlling" name="crescent" />
          <ion-icon v-else :icon="runtime.running ? powerOutline : playOutline" />
        </ion-fab-button>
      </ion-fab>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { modalController } from "@ionic/vue";
import {
  codeSlashOutline,
  documentTextOutline,
  globeOutline,
  keyOutline,
  playOutline,
  powerOutline,
  settingsOutline,
} from "ionicons/icons";
import { onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import PwdEditDialog from "@/components/PwdEditDialog.vue";
import type { OpenListLog, OpenListRuntime } from "@/components-shared";
import { logBuffer, OpenListNative } from "@/plugins/openlist-native";

const router = useRouter();

const runtime = ref<OpenListRuntime>({
  running: false,
  port: 0,
  pid: 0,
  dataSizeBytes: 0,
  lastError: "",
  lastUpdateTs: 0,
  dataDir: "",
  isInstalled: true,
});
const version = ref("unknown");
const isControlling = ref(false);
const logs = ref<OpenListLog[]>([]);

const isDevPreview = ref(false);
const backendOnline = ref(false);
const backendLatency = ref(0);

let refreshTimer: ReturnType<typeof setInterval> | null = null;
let healthTimer: ReturnType<typeof setInterval> | null = null;
let unsubscribeLog: (() => void) | null = null;

onMounted(async () => {
  isDevPreview.value = !window.OpenListNative;

  // 订阅日志流
  unsubscribeLog = logBuffer.subscribe(all => {
    logs.value = [...all];
  });

  // 初始刷新
  await refreshStatus();
  version.value = OpenListNative.getVersion();

  // 真机模式：定时刷新 runtime 状态
  if (!isDevPreview.value) {
    refreshTimer = setInterval(refreshStatus, 3000);
  } else {
    // dev preview 模式：定时探测 :5244 health
    await probeBackend();
    healthTimer = setInterval(probeBackend, 5000);
  }

  // 记录启动日志
  logBuffer.info("OpenList Web UI 已启动");
  if (isDevPreview.value) {
    logBuffer.info("当前为沙箱 dev preview 模式，启停由 :5244 进程控制");
    if (backendOnline.value) {
      logBuffer.info(`backend :5244 在线 (${backendLatency.value}ms)`);
    } else {
      logBuffer.warn("backend :5244 离线");
    }
  } else if (runtime.value.running) {
    logBuffer.info(`后端运行中，端口 ${runtime.value.port}`);
  } else {
    logBuffer.info("后端未运行");
  }
});

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
  if (healthTimer) {
    clearInterval(healthTimer);
    healthTimer = null;
  }
  if (unsubscribeLog) {
    unsubscribeLog();
    unsubscribeLog = null;
  }
});

async function refreshStatus() {
  try {
    runtime.value = OpenListNative.getStatus();
  } catch (e: any) {
    logBuffer.error(`refreshStatus 异常: ${e?.message || e}`);
  }
}

/**
 * 探测 :5244 backend（用 vite Node middleware /__openlist-health）
 * 永远 try-catch，绝不抛出（防御性，避免 unhandled rejection 击穿 SPA）
 */
async function probeBackend() {
  try {
    const res = await fetch("/__openlist-health", {
      cache: "no-store",
      signal: AbortSignal.timeout(3500),
    });
    if (!res.ok) {
      backendOnline.value = false;
      return;
    }
    const data = await res.json();
    backendOnline.value = !!data?.alive;
    backendLatency.value = data?.latency ?? 0;

    // 同步 runtime 状态（用 health 探测代替 OpenListNative.getStatus）
    if (backendOnline.value) {
      runtime.value.running = true;
      runtime.value.port = 5244;
      runtime.value.lastUpdateTs = Date.now();
      runtime.value.lastError = "";
    } else {
      runtime.value.running = false;
      runtime.value.port = 0;
      runtime.value.lastError = "backend 离线";
      runtime.value.lastUpdateTs = Date.now();
    }
  } catch (e: any) {
    backendOnline.value = false;
    runtime.value.running = false;
    runtime.value.lastError = e?.message || "探测失败";
    runtime.value.lastUpdateTs = Date.now();
  }
}

async function toggleService() {
  if (isDevPreview.value) {
    logBuffer.warn("沙箱 preview 模式，启停由 :5244 进程控制（不在 UI 范围）");
    return;
  }
  if (isControlling.value) return;
  isControlling.value = true;
  try {
    if (runtime.value.running) {
      logBuffer.info("正在停止 OpenList...");
      const ok = OpenListNative.stopOpenList();
      logBuffer[ok ? "info" : "error"](ok ? "已停止" : "停止失败");
    } else {
      logBuffer.info("正在启动 OpenList...");
      const port = OpenListNative.startOpenList();
      if (port > 0) {
        logBuffer.info(`已启动，端口 ${port}`);
      } else {
        logBuffer.error("启动失败");
      }
    }
    setTimeout(refreshStatus, 1000);
  } catch (e: any) {
    logBuffer.error(`toggleService 异常: ${e?.message || e}`);
  } finally {
    isControlling.value = false;
  }
}

async function openPasswordDialog() {
  let modal: any;
  try {
    modal = await modalController.create({
      component: PwdEditDialog,
      componentProps: {
        onConfirm: async (password: string) => {
          logBuffer.info("设置管理员密码...");
          if (isDevPreview.value) {
            logBuffer.warn("沙箱 preview 模式，密码设置需直接改 :5244 sqlite db 或用 OpenList admin UI");
            return;
          }
          const ok = OpenListNative.setPassword(password);
          logBuffer[ok ? "info" : "error"](ok ? "密码已设置" : "设置失败");
        },
      },
    });
    await modal.present();
  } catch (e: any) {
    logBuffer.error(`密码对话框打开失败: ${e?.message || e}`);
  }
}

function goToConfig() {
  router.push("/config");
}
function goToDevLogs() {
  router.push("/devlogs");
}
function goToWebView() {
  router.push("/webview");
}
function goToSettings() {
  router.push("/settings");
}
</script>

<style scoped>
ion-fab {
  margin-bottom: env(safe-area-inset-bottom, 0);
}

/* === 标题里的 Preview/Version 小徽章 === */
.home-title {
  font-weight: 600;
}
.preview-mini-chip {
  display: inline-block;
  margin-left: 8px;
  padding: 1px 6px;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.5px;
  color: #fff;
  background: linear-gradient(90deg, #f97316 0%, #ec4899 100%);
  border-radius: 3px;
  vertical-align: middle;
}
.version-mini {
  margin-left: 8px;
  font-size: 11px;
  color: var(--ion-color-medium);
  font-weight: normal;
}

/* === dev preview 提示卡片 === */
.dev-preview-notice {
  margin: 12px;
  padding: 12px 14px;
  border-radius: 10px;
  background: linear-gradient(135deg,
    rgba(245, 158, 11, 0.12) 0%,
    rgba(239, 68, 68, 0.10) 100%);
  border: 1px solid rgba(245, 158, 11, 0.4);
  border-left: 4px solid #f59e0b;
}
.dev-preview-notice-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.notice-icon {
  font-size: 18px;
}
.notice-title {
  font-size: 13px;
  font-weight: 700;
  color: #f59e0b;
  letter-spacing: 0.5px;
}
.dev-preview-notice-text {
  font-size: 12px;
  line-height: 1.6;
  color: var(--ion-text-color);
}
.dev-preview-notice-text code {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
  padding: 1px 4px;
  background: rgba(0, 0, 0, 0.08);
  border-radius: 3px;
}
.muted {
  color: var(--ion-color-medium);
}
.small {
  font-size: 10px;
}
.ok-text {
  color: var(--ion-color-success);
}
.error-text {
  color: var(--ion-color-danger);
}
</style>
