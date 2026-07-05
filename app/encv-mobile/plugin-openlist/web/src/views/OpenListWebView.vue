<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/home" />
        </ion-buttons>
        <ion-title>OpenList Web UI</ion-title>
        <ion-buttons slot="end">
          <!-- 状态指示器（点一下重试）-->
          <ion-button @click="reload" :disabled="state === 'probing'">
            <ion-icon
              :icon="stateIcon"
              :color="stateColor"
              slot="icon-only"
            />
          </ion-button>
          <ion-button @click="openExternal" v-if="!isSandbox && state === 'connected'">
            <ion-icon :icon="openOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>

      <!-- 状态条（带连接状态 + 最后一次错误） -->
      <ion-toolbar
        v-if="state === 'error' || state === 'timeout' || state === 'probing'"
        :color="stateColor"
        class="status-bar"
      >
        <ion-title size="small" class="status-title">
          <ion-spinner
            v-if="state === 'probing'"
            name="crescent"
            class="status-spinner"
          />
          <ion-icon
            v-else-if="state === 'error'"
            :icon="alertCircleOutline"
            class="status-icon-inline"
          />
          <ion-icon
            v-else-if="state === 'timeout'"
            :icon="timerOutline"
            class="status-icon-inline"
          />
          {{ stateText }}
        </ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- iframe 永远在 DOM 里（让浏览器开始加载），覆盖层控制视觉 -->
      <iframe
        v-show="state === 'connected' || state === 'loading'"
        :src="iframeUrl"
        class="openlist-iframe"
        :class="{ 'iframe-loading': state === 'loading' }"
        @error="onError"
        @load="onIframeLoad"
        ref="frameRef"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
      ></iframe>

      <!--
        防御性状态 UI 覆盖层
        覆盖在 iframe 之上，遮挡加载/错误/超时/探测的视觉空白
      -->
      <div
        v-if="state !== 'connected' && state !== 'loading'"
        class="state-overlay"
      >
        <!-- 探测中 -->
        <div v-if="state === 'probing'" class="state-card state-probing">
          <ion-spinner name="crescent" class="state-spinner" />
          <p class="state-title">正在连接 OpenList 后端…</p>
          <p class="state-hint">127.0.0.1:5244（{{ isSandbox ? 'Vite proxy' : 'localhost' }}）</p>
        </div>

        <!-- 错误：连接失败/502/404 -->
        <div v-else-if="state === 'error'" class="state-card state-error">
          <ion-icon :icon="cloudOfflineOutline" class="state-icon" />
          <p class="state-title">OpenList 后端未运行</p>
          <p class="state-hint">{{ lastError || '后端不可达（连接被拒绝或 502）' }}</p>

          <div class="state-command-block">
            <p class="state-hint-small">在另一个终端启动 OpenList 后端：</p>
            <code class="state-cmd">bash scripts/dev-openlist.sh</code>
          </div>

          <div v-if="retryCount > 0" class="state-retry-info">
            已重试 {{ retryCount }} 次
          </div>

          <div class="state-actions">
            <ion-button @click="reload" color="primary">
              <ion-icon :icon="refreshOutline" slot="start" />
              重试
            </ion-button>
            <ion-button @click="copyCommand" fill="clear" size="default">
              <ion-icon :icon="copyOutline" slot="start" />
              复制启动命令
            </ion-button>
          </div>
        </div>

        <!-- 超时：连接慢/被防火墙挡 -->
        <div v-else-if="state === 'timeout'" class="state-card state-timeout">
          <ion-icon :icon="timerOutline" class="state-icon" />
          <p class="state-title">连接超时</p>
          <p class="state-hint">OpenList 后端响应超过 5 秒</p>

          <div class="state-actions">
            <ion-button @click="reload" color="primary">
              <ion-icon :icon="refreshOutline" slot="start" />
              再试一次
            </ion-button>
          </div>
        </div>
      </div>

      <!-- 调试面板（devtools 风格，初始隐藏，点开折叠） -->
      <div v-if="debugOpen" class="debug-panel">
        <div class="debug-header">
          <span>Debug · {{ debugEntries.length }} entries</span>
          <button class="debug-close" @click="debugOpen = false">×</button>
        </div>
        <div class="debug-list">
          <div
            v-for="(e, i) in debugEntries"
            :key="i"
            class="debug-entry"
            :class="'debug-' + e.level"
          >
            <span class="debug-ts">{{ e.ts }}</span>
            <span class="debug-level">{{ e.level.toUpperCase() }}</span>
            <span class="debug-msg">{{ e.msg }}</span>
            <span v-if="e.data" class="debug-data">{{ e.data }}</span>
          </div>
        </div>
      </div>
    </ion-content>

    <!-- 调试面板触发按钮（右下角悬浮，dev 模式可见）-->
    <ion-fab v-if="isSandbox" vertical="bottom" horizontal="end" slot="fixed">
      <ion-fab-button size="small" @click="debugOpen = !debugOpen">
        <ion-icon :icon="bugOutline" />
      </ion-fab-button>
    </ion-fab>
  </ion-page>
</template>

<script setup lang="ts">
import { toastController } from "@ionic/vue";
import { checkmarkCircleOutline, cloudOfflineOutline, refreshOutline, timerOutline } from "ionicons/icons";
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { logBuffer, OpenListNative } from "@/plugins/openlist-native";

type IframeState = "probing" | "loading" | "connected" | "error" | "timeout";

interface DebugEntry {
  ts: string;
  level: "info" | "warn" | "error" | "probe";
  msg: string;
  data?: string;
}

const _router = useRouter();

const port = ref(0);
const state = ref<IframeState>("probing");
const lastError = ref("");
const retryCount = ref(0);
const frameRef = ref<HTMLIFrameElement | null>(null);
const _debugOpen = ref(false);
const debugEntries = ref<DebugEntry[]>([]);

const PROBE_TIMEOUT_MS = 5000;
const HEALTH_POLL_INTERVAL_MS = 10000;

let pollTimer: ReturnType<typeof setInterval> | null = null;

/**
 * 沙箱 dev / 真机 prod 区分
 *  - dev:   直访 127.0.0.1:5244（同源策略对 127.0.0.1 不严格，CORS=*）
 *  - prod:  直连 127.0.0.1:5244（同设备，OpenList 与 Capacitor 同进程域）
 */
const isSandbox = computed(() => import.meta.env.DEV);

const _iframeUrl = computed(() => {
  const hash = "#/login";
  // 走 preview-gateway 统一收口 :16666/openlist/ → :5244 OpenList upstream
  //   不再硬编码 :5244 — 沙箱 dev 唯一对外端口是 :16666（agent-tool-host :16000 代理过来）
  return `http://localhost:16666/openlist/${hash}`;
});

const _stateText = computed(() => {
  switch (state.value) {
    case "probing":
      return "连接中…";
    case "error":
      return "连接失败";
    case "timeout":
      return "连接超时";
    default:
      return "";
  }
});

const _stateColor = computed(() => {
  switch (state.value) {
    case "connected":
      return "success";
    case "error":
      return "danger";
    case "timeout":
      return "warning";
    case "probing":
      return "medium";
    default:
      return "primary";
  }
});

const _stateIcon = computed(() => {
  switch (state.value) {
    case "connected":
      return checkmarkCircleOutline;
    case "error":
      return cloudOfflineOutline;
    case "timeout":
      return timerOutline;
    case "probing":
      return refreshOutline;
    case "loading":
      return refreshOutline;
    default:
      return refreshOutline;
  }
});

// ============== 调试日志 ==============

function debug(level: DebugEntry["level"], msg: string, data?: any) {
  const ts = new Date().toISOString().split("T")[1].slice(0, 12);
  let dataStr = "";
  if (data !== undefined) {
    try {
      dataStr = typeof data === "string" ? data : JSON.stringify(data);
      if (dataStr.length > 200) dataStr = dataStr.slice(0, 200) + "…";
    } catch {
      dataStr = String(data);
    }
  }
  debugEntries.value.unshift({ ts, level, msg, data: dataStr });
  if (debugEntries.value.length > 50) debugEntries.value.length = 50;
}

// ============== 生命周期 ==============

onMounted(async () => {
  port.value = OpenListNative.getPort();
  debug("info", "onMounted", { isSandbox: isSandbox.value, port: port.value });
  if (isSandbox.value) {
    await probeBackend("initial");
  } else {
    // 真机模式：OpenList 与 Capacitor 同设备，假设后端可达
    state.value = "loading";
  }
  startHealthPolling();
});

onUnmounted(() => {
  stopHealthPolling();
});

// ============== 健康轮询 ==============

function startHealthPolling() {
  stopHealthPolling();
  pollTimer = setInterval(() => {
    pollHealth();
  }, HEALTH_POLL_INTERVAL_MS);
}

function stopHealthPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

async function pollHealth() {
  // 仅在 connected 状态做后置校验：若后端突然挂掉，自动跳回 error
  if (state.value !== "connected") return;
  debug("probe", "pollHealth (background)");
  const result = await checkHealth();
  if (!result.alive) {
    debug("error", "pollHealth: backend gone", result);
    state.value = "error";
    lastError.value = `后端已离线：${result.error || "unknown"}`;
  }
}

// ============== 探测 / 健康检查 ==============

interface HealthResult {
  alive: boolean;
  error?: string;
  code?: string;
  upstreamStatus?: number;
  latency?: number;
  ts: number;
}

async function checkHealth(): Promise<HealthResult> {
  try {
    const res = await fetch("/__openlist-health", {
      cache: "no-store",
    });
    const data = (await res.json()) as HealthResult;
    return data;
  } catch (e: any) {
    return {
      alive: false,
      error: e?.message || String(e),
      code: "FETCH_FAILED",
      ts: Date.now(),
    };
  }
}

/**
 * 沙箱后端可达性探测（带超时）
 * - 成功（alive=true）→ state=loading（等 iframe @load 进一步验证）
 * - alive=false 且 error=timeout → state=timeout
 * - alive=false 其它 → state=error
 */
async function probeBackend(reason: string = "manual") {
  state.value = "probing";
  lastError.value = "";
  debug("probe", `probeBackend (${reason})`);

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS);

  let result: HealthResult;
  try {
    // 带超时的健康检查（middleware 自带 3s 超时，但前端再加一层保险）
    const r = await Promise.race([
      fetch("/__openlist-health", { cache: "no-store", signal: controller.signal }),
      new Promise<never>((_, reject) => {
        setTimeout(() => reject(new DOMException("probe timeout", "AbortError")), PROBE_TIMEOUT_MS);
      }),
    ]);
    result = (await (r as Response).json()) as HealthResult;
    clearTimeout(timer);
  } catch (e: any) {
    clearTimeout(timer);
    if (e?.name === "AbortError" || e?.message?.includes("timeout")) {
      state.value = "timeout";
      lastError.value = `超过 ${PROBE_TIMEOUT_MS}ms 未响应`;
      debug("warn", "probeBackend timeout");
      return;
    }
    state.value = "error";
    lastError.value = String(e?.message || e);
    debug("error", "probeBackend fetch failed", e?.message);
    return;
  }

  debug("probe", "health result", result);

  if (result.alive) {
    state.value = "loading";
    logBuffer.info(`OpenList 后端已连接 (${result.latency}ms, status=${result.upstreamStatus})`);
  } else if (result.error === "timeout") {
    state.value = "timeout";
    lastError.value = `超过 ${PROBE_TIMEOUT_MS}ms 未响应`;
  } else {
    state.value = "error";
    lastError.value = `${result.error || "unknown"}${result.code ? " (" + result.code + ")" : ""}`;
  }
}

// ============== iframe 事件 ==============

function _onIframeLoad() {
  debug("info", "iframe @load fired", {
    currentState: state.value,
    src: frameRef.value?.src?.slice(0, 80),
  });

  // 关键修复：iframe @load 不直接置 connected
  // 原因：iframe 可能加载 502 错误页面（来自 Vite proxy）也会触发 @load
  // 必须重新 health 校验才能确认是真 SPA 加载完成
  if (state.value === "connected") {
    // 已经是 connected（用户在 SPA 内导航/刷新），不重复验证
    return;
  }

  // 立即做一次 health check 验证
  verifyAfterIframeLoad();
}

async function verifyAfterIframeLoad() {
  debug("probe", "verifyAfterIframeLoad (post @load)");
  const result = await checkHealth();
  if (result.alive) {
    state.value = "connected";
    debug("info", "iframe verified, state=connected", result);
  } else {
    // iframe @load 触发了，但 health 不通过 → iframe 是 502 错误页
    state.value = "error";
    lastError.value = `iframe 加载但后端不健康：${result.error}`;
    debug("error", "iframe loaded 502 page", result);
  }
}

function _onError() {
  debug("error", "iframe @error fired");
  logBuffer.error("iframe 加载失败");
  if (isSandbox.value) {
    state.value = "error";
    lastError.value = "iframe 加载失败";
  }
}

// ============== 用户操作 ==============

function _reload() {
  retryCount.value++;
  if (isSandbox.value) {
    probeBackend("manual");
  } else {
    const frame = frameRef.value;
    if (frame) {
      state.value = "loading";
      const oldSrc = frame.src;
      frame.src = "about:blank";
      nextTick(() => {
        frame.src = oldSrc;
      });
    }
  }
}

function _openExternal() {
  const url = `http://127.0.0.1:${port.value || 5244}/`;
  window.open(url, "_blank");
}

async function _copyCommand() {
  const cmd = "bash scripts/dev-openlist.sh";
  try {
    await navigator.clipboard.writeText(cmd);
    const toast = await toastController.create({
      message: "启动命令已复制到剪贴板",
      duration: 1800,
      position: "bottom",
    });
    await toast.present();
  } catch {
    logBuffer.warn("剪贴板复制失败");
  }
}
</script>

<style scoped>
.openlist-iframe {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
  background: #fff;
}
.iframe-loading {
  opacity: 0.6;
}

.state-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--ion-background-color, #ffffff);
  z-index: 10;
  padding: 24px;
}

.state-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  max-width: 360px;
  gap: 8px;
}

.state-spinner {
  width: 48px;
  height: 48px;
  margin-bottom: 12px;
  color: var(--ion-color-medium);
}

.state-icon {
  font-size: 56px;
  margin-bottom: 8px;
}
.state-error .state-icon { color: var(--ion-color-danger); }
.state-timeout .state-icon { color: var(--ion-color-warning); }

.state-title {
  font-size: 17px;
  font-weight: 600;
  margin: 0;
  color: var(--ion-text-color, #000);
}
.state-hint {
  font-size: 13px;
  color: var(--ion-color-medium);
  margin: 0 0 4px 0;
  line-height: 1.5;
  word-break: break-word;
}
.state-hint-small {
  font-size: 12px;
  color: var(--ion-color-medium);
  margin: 0 0 4px 0;
}

.state-command-block {
  margin: 12px 0 4px 0;
  padding: 12px 16px;
  background: var(--ion-color-light);
  border-radius: 8px;
  width: 100%;
  box-sizing: border-box;
}

.state-cmd {
  display: inline-block;
  padding: 6px 10px;
  background: var(--ion-background-color, #fff);
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
  border-radius: 4px;
  font-family: monospace;
  font-size: 12px;
  color: var(--ion-text-color, #000);
  user-select: all;
  word-break: break-all;
}

.state-retry-info {
  font-size: 11px;
  color: var(--ion-color-medium);
  margin-top: 4px;
}

.state-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: center;
  margin-top: 12px;
}

.status-bar {
  --background: var(--ion-color-light);
  --color: var(--ion-color-medium);
}
.status-bar[color="danger"] {
  --background: var(--ion-color-danger);
  --color: #fff;
}
.status-bar[color="warning"] {
  --background: var(--ion-color-warning);
  --color: #000;
}

.status-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
}
.status-spinner {
  width: 14px;
  height: 14px;
}
.status-icon-inline {
  font-size: 14px;
}

/* 调试面板（devtools 风格） */
.debug-panel {
  position: absolute;
  bottom: 16px;
  left: 16px;
  width: 80%;
  max-width: 480px;
  max-height: 50%;
  background: #1e1e1e;
  color: #d4d4d4;
  border-radius: 6px;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  z-index: 100;
  box-shadow: 0 4px 16px rgba(0,0,0,0.4);
  display: flex;
  flex-direction: column;
}
.debug-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 10px;
  border-bottom: 1px solid #333;
  background: #252526;
  border-radius: 6px 6px 0 0;
}
.debug-close {
  background: none;
  border: none;
  color: #999;
  font-size: 18px;
  cursor: pointer;
  padding: 0 4px;
}
.debug-close:hover { color: #fff; }
.debug-list {
  overflow-y: auto;
  padding: 4px 0;
  flex: 1;
}
.debug-entry {
  padding: 3px 10px;
  display: flex;
  gap: 6px;
  border-bottom: 1px solid #2a2a2a;
  word-break: break-all;
}
.debug-entry:hover { background: #2a2a2a; }
.debug-ts { color: #858585; flex-shrink: 0; }
.debug-level {
  flex-shrink: 0;
  font-weight: 600;
  width: 42px;
}
.debug-info .debug-level { color: #4ec9b0; }
.debug-warn .debug-level { color: #dcdcaa; }
.debug-error .debug-level { color: #f48771; }
.debug-probe .debug-level { color: #9cdcfe; }
.debug-msg { color: #d4d4d4; }
.debug-data { color: #858585; margin-left: 6px; }
</style>
