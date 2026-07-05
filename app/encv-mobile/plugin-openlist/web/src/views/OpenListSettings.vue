<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <!--
            用 ion-button + @click router.push 替代 ion-back-button
            原因：ion-back-button 在 vue-router 4 hash 模式下 default-href 行为不可靠
            （Ionic 8 ion-back-button 内部可能走 history.back()，popstate 不触发 hashchange）
            router.push('/home') 显式 hash 跳，hashchange 事件一定触发
          -->
          <ion-button @click="goBackToHome" title="返回 home">
            <ion-icon slot="icon-only" :icon="chevronBackOutline" />
          </ion-button>
        </ion-buttons>
        <ion-title>设置</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!--
        沙箱 dev preview 专属横幅（醒目 badge）
        仅在 window.OpenListNative 不存在时显示（即 vite dev 沙箱预览模式）
      -->
      <div v-if="isDevPreview" class="preview-banner">
        <div class="preview-banner-row">
          <span class="preview-icon">🔥</span>
          <span class="preview-tag">PREVIEW BUILD</span>
          <span class="preview-tag-sub">沙箱开发预览</span>
        </div>
        <div class="preview-banner-text">
          <strong>当前为沙箱 dev preview 模式</strong>，<code>window.OpenListNative</code> 不存在。
          <br />
          后端由 <code>/tmp/openlist</code> 独立进程提供（<code>:5244</code>），
          <strong>不能通过本界面启停</strong>（需在终端 <code>start-preview.sh</code> 控制）。
          <br />
          所有数据（密码、Config、版本）都通过 <code>http://127.0.0.1:5244/api/*</code> 直访 backend（撤销 /openlist-spa/ 路由改造）。
        </div>
        <details class="preview-debug">
          <summary>🔧 路由诊断（点击展开）</summary>
          <div class="debug-section">
            <div><strong>location.hash</strong>: <code>{{ currentHash }}</code></div>
            <div><strong>已注册 routes</strong>: <code>{{ registeredRoutes.join(', ') }}</code></div>
            <div><strong>logBuffer 最近 5 条</strong>:</div>
            <ul class="debug-log">
              <li v-for="(line, i) in recentLogs" :key="i" v-html="line"></li>
            </ul>
          </div>
        </details>
      </div>

      <ion-list>
        <!-- OpenList 版本：dev preview 模式从 backend 拿真版本，叠加炫酷 Preview Build 标识 -->
        <ion-item>
          <ion-label>
            <h2>OpenList 版本</h2>
            <p class="version-line">
              <span v-if="loadingVersion" class="muted">正在探测 :5244…</span>
              <span v-else-if="versionError" class="error-text">
                ✗ {{ versionError }}
              </span>
              <span v-else>
                <span class="version-value">v{{ realVersion || version }}</span>
                <span v-if="isDevPreview" class="preview-chip">🔥 PREVIEW</span>
                <span v-if="isDevPreview" class="preview-chip-sub">vite dev 实时刷新</span>
              </span>
            </p>
          </ion-label>
        </ion-item>

        <ion-item>
          <ion-label>
            <h2>数据目录</h2>
            <p class="mono-text">{{ dataDir || '(未配置)' }}</p>
            <p v-if="isDevPreview" class="muted small">
              沙箱模式：当前 <code>:5244</code> backend 数据目录由 <code>/tmp/openlist-data</code> 决定
            </p>
          </ion-label>
        </ion-item>

        <ion-item>
          <ion-label>
            <h2>监听端口</h2>
            <p>
              <span class="mono-text">{{ port || '(未知)' }}</span>
              <span v-if="isBackendReachable" class="ok-text"> ● 在线</span>
              <span v-else class="error-text"> ● 离线</span>
            </p>
            <p v-if="isDevPreview" class="muted small">
              沙箱模式：health 由 <code>/__openlist-health</code> Node middleware 探测
            </p>
          </ion-label>
        </ion-item>

        <ion-item button @click="openWebUi" :disabled="!isBackendReachable">
          <ion-icon :icon="globeOutline" slot="start" />
          <ion-label>
            <h2>打开 Web UI</h2>
            <p class="mono-text">http://127.0.0.1:{{ port || 5244 }}</p>
          </ion-label>
        </ion-item>

        <ion-item button @click="goHome">
          <ion-icon :icon="homeOutline" slot="start" />
          <ion-label>
            <h2>返回主页</h2>
            <p class="muted">plugin-openlist Capacitor UI</p>
          </ion-label>
        </ion-item>

        <!--
          强力的"返回 ENCV 主页面"入口
          dev 沙箱：window.location.origin + '/tabs/remote'（encv-mobile 主 tab）
          真机：不显示（真机在 Android WebView 内部，外部导航无意义）
        -->
        <ion-item
          v-if="isDevPreview"
          button
          @click="goBackToEncvMain"
          class="back-to-encv-item"
        >
          <ion-icon :icon="arrowBackOutline" slot="start" />
          <ion-label>
            <h2 class="back-to-encv-label">返回 ENCV 主页面</h2>
            <p class="muted small">
              离开 dev preview，回到 ENCV Capacitor app
            </p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { logBuffer, OpenListNative } from "@/plugins/openlist-native";

const router = useRouter();

// 防御性默认值：dev preview 模式 window.OpenListNative 不存在，OpenListNative 返 'unknown'/0/''
const version = ref("unknown");
const dataDir = ref("");
const port = ref(0);

const isDevPreview = ref(false);
const loadingVersion = ref(false);
const realVersion = ref("");
const versionError = ref("");
const isBackendReachable = ref(false);

// logBuffer 状态（用于 debug section）
const recentLogs = ref<string[]>([]);
const currentHash = ref(window.location.hash || "(empty)");
const registeredRoutes = ref<string[]>([]);

let hashPollTimer: ReturnType<typeof setInterval> | null = null;

onMounted(async () => {
  // **核心检测**：dev preview 模式 = window.OpenListNative 不存在
  // 真机模式 = window.OpenListNative 存在（OpenListPluginJSInterface 注册过）
  isDevPreview.value = !window.OpenListNative;

  // 基础数据：从 OpenListNative stub 拿（dev preview 全部 0/unknown/''）
  version.value = OpenListNative.getVersion();
  dataDir.value = OpenListNative.getDataDir();
  port.value = OpenListNative.getPort();

  // dev preview 模式：从 :5244 backend 拿真版本 + 真 health 状态
  if (isDevPreview.value) {
    await fetchRealVersion();
    await probeBackendHealth();
    // 启动路由诊断轮询
    registeredRoutes.value = router.getRoutes().map(r => r.path);
    hashPollTimer = setInterval(() => {
      currentHash.value = window.location.hash || "(empty)";
      // 同步 logBuffer 输出
      recentLogs.value = logBuffer.getAll().slice(-5).map(formatLogLine);
    }, 1000);
  }
});

onUnmounted(() => {
  if (hashPollTimer) clearInterval(hashPollTimer);
});

function formatLogLine(entry: any): string {
  const level = entry?.level?.toUpperCase() || "INFO";
  const msg = entry?.message || "";
  const color = level === "ERROR" ? "#ef4444" : level === "WARN" ? "#f59e0b" : "#22c55e";
  return `<span style="color:${color}">[${level}]</span> ${msg}`;
}

/**
 * 从 :5244 /api/public/settings 拿真版本（无需 auth）
 * 失败时显示错误，不让 UI 一直 loading
 */
async function fetchRealVersion() {
  loadingVersion.value = true;
  versionError.value = "";
  try {
    // dev preview 下 axios 直接用 http://127.0.0.1:5244/api/*（直访，无 vite proxy）
    // 但 vite proxy 会被同源策略拦（vite 起在 5174，我们从 5174 fetch 自己）—— OK 同源
    const res = await fetch("http://127.0.0.1:5244/api/public/settings", {
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    });
    if (!res.ok) {
      versionError.value = `HTTP ${res.status}`;
      return;
    }
    const data = await res.json();
    if (data?.code === 200 && data?.data?.version) {
      realVersion.value = data.data.version;
    } else {
      versionError.value = "backend 返非预期格式";
    }
  } catch (e: any) {
    versionError.value = e?.message || String(e);
  } finally {
    loadingVersion.value = false;
  }
}

/**
 * 探测 :5244 backend 是否可达（用 vite Node middleware /__openlist-health）
 */
async function probeBackendHealth() {
  try {
    const res = await fetch("/__openlist-health", {
      cache: "no-store",
      signal: AbortSignal.timeout(3500),
    });
    if (!res.ok) {
      isBackendReachable.value = false;
      return;
    }
    const data = await res.json();
    isBackendReachable.value = !!data?.alive;
  } catch {
    isBackendReachable.value = false;
  }
}

function _openWebUi() {
  // dev preview：走沙箱路径 :2025/openlist-ui/... 不行（那只能到 plugin-openlist vite）
  // 直接打开 :5244 真实 backend
  window.open(`http://127.0.0.1:${port.value || 5244}/#/login`, "_blank", "noopener");
}

function _goHome() {
  router.push("/home");
}

/**
 * toolbar 左上角返回按钮：显式 router.push 避免 ion-back-button + default-href 在
 * vue-router 4 hash 模式下行为不可靠。
 */
function _goBackToHome() {
  router.push("/home");
}

function _goBackToEncvMain() {
  // 跳到 BackToMain 视图（plugin-openlist 内嵌全屏 iframe 加载 encv-mobile :5173）。
  // 为什么不直接 window.location.href 跳 :5173：
  //   - Trae 沙箱 OpenPreview 单 port 限制 (trae_web_sandbox_network.md §8.4)：
  //     OpenPreview 注册了 :5174，16000 入口只能代理 :5174，:5173 在沙箱内对外不可达
  //   - 直接跳 `http://localhost:5173/tabs/remote` 在沙箱下会失败
  //   - 在 :5174 内嵌 iframe 加载 :5173 走的是 sandbox 内部端口互通（vite 监听 0.0.0.0），
  //     不依赖 16000 代理
  // 为什么不调 OpenPreview 切换到 :5173：
  //   - OpenPreview 是 AI agent 工具，前端无法调用
  //   - 即使能调用，多次注册会 last-write-wins 覆盖
  logBuffer.info("[OpenListSettings] goBackToEncvMain → /back-to-main");
  // 检查路由是否注册（如果 plugins/router 还没初始化好，fallback 跳绝对 URL）
  const hasRoute = router.getRoutes().some(r => r.path === "/back-to-main");
  if (hasRoute) {
    router.push("/back-to-main");
  } else {
    // Fallback：直接跳 :5173（直连模式 OK；沙箱模式会失败但允许尝试）
    logBuffer.warn("[OpenListSettings] /back-to-main 未注册，fallback 直接跳 :5173");
    window.location.assign("http://127.0.0.1:5173/tabs/remote");
  }
}
</script>

<style scoped>
.mono-text {
  font-family: monospace;
  font-size: 12px;
  word-break: break-all;
}
.muted {
  color: var(--ion-color-medium);
  font-size: 11px;
}
.small {
  font-size: 11px;
}
.error-text {
  color: var(--ion-color-danger);
}
.ok-text {
  color: var(--ion-color-success);
}

/* === 沙箱 dev preview 横幅 === */
.preview-banner {
  margin: 12px;
  padding: 14px 16px;
  border-radius: 12px;
  background: linear-gradient(135deg,
    rgba(99, 102, 241, 0.18) 0%,
    rgba(168, 85, 247, 0.18) 50%,
    rgba(236, 72, 153, 0.18) 100%);
  border: 1px solid rgba(99, 102, 241, 0.4);
  box-shadow: 0 2px 12px rgba(99, 102, 241, 0.15);
  backdrop-filter: blur(4px);
}
.preview-banner-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.preview-icon {
  font-size: 22px;
  filter: drop-shadow(0 0 6px rgba(255, 150, 50, 0.6));
  animation: pulse-flame 1.8s ease-in-out infinite;
}
@keyframes pulse-flame {
  0%, 100% { transform: scale(1) rotate(-3deg); }
  50% { transform: scale(1.15) rotate(3deg); }
}
.preview-tag {
  display: inline-block;
  padding: 3px 10px;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 1.5px;
  color: #fff;
  background: linear-gradient(135deg, #6366f1 0%, #a855f7 50%, #ec4899 100%);
  border-radius: 6px;
  box-shadow: 0 1px 4px rgba(168, 85, 247, 0.4);
}
.preview-tag-sub {
  font-size: 11px;
  color: var(--ion-color-medium);
}
.preview-banner-text {
  font-size: 12px;
  line-height: 1.65;
  color: var(--ion-text-color);
}
.preview-banner-text code {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
  padding: 1px 4px;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 3px;
}

/* === 版本行 + 炫酷 Preview 徽章 === */
.version-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.version-value {
  font-family: monospace;
  font-weight: 600;
}
.preview-chip {
  display: inline-block;
  padding: 2px 8px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 1px;
  color: #fff;
  background: linear-gradient(90deg, #f97316 0%, #ef4444 50%, #ec4899 100%);
  border-radius: 4px;
  box-shadow: 0 0 8px rgba(239, 68, 68, 0.4);
  animation: pulse-glow 2s ease-in-out infinite;
}
@keyframes pulse-glow {
  0%, 100% { box-shadow: 0 0 4px rgba(239, 68, 68, 0.4); }
  50% { box-shadow: 0 0 12px rgba(239, 68, 68, 0.8); }
}
.preview-chip-sub {
  font-size: 10px;
  color: var(--ion-color-medium);
}

/* === 返回 ENCV 主页面按钮 === */
.back-to-encv-item {
  --background: rgba(99, 102, 241, 0.1);
  --border-color: rgba(99, 102, 241, 0.3);
  margin: 12px 0;
  border-radius: 8px;
}
.back-to-encv-label {
  color: var(--ion-color-primary);
  font-weight: 600;
}
</style>
