<template>
  <ion-app>
    <div v-if="serviceGuardBlocked" class="service-guard-blocked">
      <div class="guard-content">
        <ion-icon :icon="warningOutline" class="guard-icon"></ion-icon>
        <h2>{{ t('app.serviceGuardTitle') }}</h2>
        <p class="guard-message">{{ t('app.serviceGuardMessage') }}</p>
        <code class="guard-detail">{{ serviceGuardDetail }}</code>
        <pre v-if="serviceGuardHint" class="guard-hint">{{ serviceGuardHint }}</pre>
        <ion-button @click="retryServiceGuard" class="guard-retry-btn">
          <ion-icon :icon="refreshOutline" slot="start"></ion-icon>
          {{ t('app.serviceGuardRetry') }}
        </ion-button>
      </div>
    </div>
    <!--
      Vue 渲染错误边界（防御性 UI：子组件 render 崩溃时显示错误页而不是空白）
      onErrorCaptured 捕获子组件的错误 → 置 rootError=true → 渲染 fallback
      注意：service-guard-blocked 优先（service guard 是更上游的拦截）
    -->
    <div v-else-if="rootError" class="root-error-fallback error-state">
      <div class="error-content">
        <ion-icon :icon="bugOutline" class="error-icon"></ion-icon>
        <h2>组件渲染错误</h2>
        <p class="error-message">某个子组件崩溃了。请截图上报给开发者。</p>

        <!--
          🆕 错误 UI 拆两栏（2026-06-08 重构）：
            - 上（红色）error-detail-panel：详细摘要 + 上下文（错误类型 / info / 时间戳 /
              浏览器模式 / baseUrl 状态），一眼看出错误场景
            - 下（灰色）error-stack-panel：原始堆栈 + 沙箱排错辅助信息链接
          拆两栏但**不**显示 message 和 stack 的内容重叠：
            - error-detail 显示 "summary + trace"（来自 splitErrorMessage）
            - error-stack 显示 "raw stack + 沙箱文档链接"（独立信息）
        -->

        <!-- 上：红色 detail 区域（详细信息） -->
        <section class="error-detail-panel">
          <header class="error-detail-header">
            <ion-icon :icon="alertCircleOutline" class="error-detail-icon"></ion-icon>
            <span class="error-detail-title">错误摘要</span>
            <button class="copy-btn" @click="copyErrorSummary" aria-label="复制摘要">
              <ion-icon :icon="copyOutline"></ion-icon>
            </button>
          </header>
          <div class="error-detail-body">
            <div class="error-summary-line">
              <span class="error-summary-label">类型</span>
              <code class="error-summary-value">{{ rootErrorSummary }}</code>
            </div>
            <div class="error-summary-line">
              <span class="error-summary-label">触发阶段</span>
              <code class="error-summary-value">{{ rootErrorInfo }}</code>
            </div>
            <div class="error-summary-line" v-if="rootErrorTime">
              <span class="error-summary-label">时间</span>
              <code class="error-summary-value">{{ rootErrorTime }}</code>
            </div>
            <div class="error-summary-line">
              <span class="error-summary-label">运行模式</span>
              <code class="error-summary-value">{{ rootErrorContext.mode }}</code>
            </div>
            <div class="error-summary-line" v-if="rootErrorContext.location">
              <span class="error-summary-label">当前 origin</span>
              <code class="error-summary-value">{{ rootErrorContext.location }}</code>
            </div>
            <!-- 探测链 trace（来自 useApiBaseProbe " | trace: " 拆分） -->
            <div v-if="rootErrorDetails" class="error-trace-block">
              <div class="error-trace-label">探测链 trace</div>
              <pre class="error-trace-body">{{ rootErrorDetails }}</pre>
            </div>
          </div>
        </section>

        <!-- 下：灰色 stack 区域（原始堆栈 + 排错文档） -->
        <section class="error-stack-panel">
          <header class="error-stack-header">
            <ion-icon :icon="codeSlashOutline" class="error-stack-icon"></ion-icon>
            <span class="error-stack-title">原始堆栈</span>
            <button class="copy-btn" @click="copyErrorStack" aria-label="复制堆栈">
              <ion-icon :icon="copyOutline"></ion-icon>
            </button>
          </header>
          <pre v-if="rootErrorStack" class="error-stack-body">{{ rootErrorStack }}</pre>
          <p v-else class="error-stack-empty">（无堆栈）</p>

          <footer class="error-stack-footer">
            <div class="error-stack-hints">
              <strong>排错指引</strong>
              <ul>
                <li v-if="rootErrorContext.mode === 'mock-browser'">
                  <span class="hint-tag">preview-only</span>
                  你正在 <code>mock 浏览器</code>（trae 网关层 mock）里查看，<strong>API 调用不通是预期</strong>。
                  trae 网关层（沙箱外）拦截 <code>/api/*</code> 和 <code>/health</code> 返回 <code>401 missing session token</code>，沙箱内不可绕过。
                  完整功能请在 <strong>Android 真机</strong> 或 <strong>本地 dev</strong>（<code>localhost:16666</code>）测试。
                </li>
                <li>沙箱外网访问限制见 <code>trae_web_sandbox_network.md §9.1</code>（mock 浏览器无 Network 面板）</li>
                <li>401 区分铁律：<code>text/html</code> = trae 网关，<code>application/json</code> = 业务端（见 <code>§9.1.2</code>）</li>
                <li>把以上"原始堆栈"和"错误摘要"完整截图给开发者</li>
              </ul>
            </div>
          </footer>
        </section>

        <ion-button @click="reloadPage" class="error-reload-btn">
          <ion-icon :icon="refreshOutline" slot="start"></ion-icon>
          重新加载
        </ion-button>
      </div>
    </div>
    <ion-router-outlet v-else />
    <!--
      🆕 2026-07-02 A5：三管齐下错误捕获浮窗
      - 监听 errorStore.latestError + showOverlay
      - 显示最近一个错误的卡片（点击展开 / 关闭按钮 dismiss）
      - 与 App.vue 顶层 onErrorCaptured 错误页（rootError fallback）不冲突：
        rootError 是渲染错误熔断器；ErrorCaptureOverlay 是底层错误通知
    -->
    <ErrorCaptureOverlay />
  </ion-app>
</template>

<script setup lang="ts">
import { onErrorCaptured, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  warningOutline,
  refreshOutline,
  bugOutline,
  alertCircleOutline,
  copyOutline,
  codeSlashOutline,
} from "ionicons/icons";
import type { ServiceGuardResult } from "@/api/encv";
import { checkServiceGuard } from "@/api/encv";
import { autoInitVConsole } from "@/composables/useDevTools";
import { registerFileFeature } from "@/composables/useFileFeatures";
import { hijackConsole } from "@/composables/useFrontendLogs";
import { initHighRefreshRate } from "@/composables/useHighRefreshRate";
import { useI18n } from "@/composables/useI18n";
import { useRealtimeTransport } from "@/composables/useRealtimeTransport";
import { useTheme } from "@/composables/useTheme";
import { createAlistEncryptFeature } from "@/features/alist-encrypt";
import { isNative, requestNotificationPermission, requestStoragePermission } from "@/plugins/GoProcess";
import ErrorCaptureOverlay from "@/components/shared/ErrorCaptureOverlay.vue";

const { initTheme, detectP3Support } = useTheme();
const { t } = useI18n();
const transport = useRealtimeTransport();
const { connect, disconnect } = transport;
const router = useRouter();

const serviceGuardBlocked = ref(false);
const serviceGuardDetail = ref("");
const serviceGuardHint = ref("");

// ============ Vue 错误边界 ============
const rootError = ref(false);
const rootErrorSummary = ref("");
const rootErrorDetails = ref("");
const rootErrorInfo = ref(""); // 触发阶段：'mounted hook' / 'render function' / ...
const rootErrorTime = ref(""); // ISO 时间戳，方便用户截图给 agent 时同步时间
const rootErrorStack = ref(""); // 原始堆栈（独立显示在下方 stack panel）
const rootErrorContext = ref<{ mode: string; location: string }>({
  mode: "unknown",
  location: "",
});

/**
 * 把 `err.message` 拆成 "summary + details" 两部分，避免 UI 上下两栏内容重复。
 *
 * 拆分规则（匹配 useApiBaseProbe throw 的格式）：
 *   "all-candidates-failed | trace: [1] skip | [1.5] result: ok=false err=... | [4] all-failed"
 *     ↳ summary  = "all-candidates-failed"
 *     ↳ details  = "[1] skip | [1.5] result: ok=false err=... | [4] all-failed"
 *
 * 没找到 " | trace: " 标记时，details 为空（普通错误只显示 summary）。
 */
function splitErrorMessage(msg: string): { summary: string; details: string } {
  const MARKER = " | trace: ";
  const idx = msg.indexOf(MARKER);
  if (idx < 0) return { summary: msg, details: "" };
  return {
    summary: msg.slice(0, idx),
    details: msg.slice(idx + MARKER.length),
  };
}

/**
 * 探测当前运行模式 + location，给错误 UI 提供上下文
 * 模式：capacitor / browser-dev / browser-prod / mock-browser（trae 模拟）
 *
 * 关键判定：
 *   - mock-browser 的最可靠特征 = host 含 'agent-sandbox'（trae 给每个 agent
 *     沙箱分配的预览域名唯一特征：run-agent-<id>-<hash>-preview.agent-sandbox-
 *     <region>-<gw>.trae.cn）。UA 含 'Trae'/'Volo' 是辅助信号但不可靠
 *     （mock 浏览器可能伪装 UA）
 *   - 优先级：mock-browser > capacitor > browser-dev > browser-prod
 */
function detectErrorContext(): { mode: string; location: string } {
  let mode = "unknown";
  if (typeof window !== "undefined") {
    const proto = window.location.protocol;
    const host = window.location.host || "";
    const ua = navigator.userAgent || "";
    const isTraeMockHost = /agent-sandbox.*\.trae\.cn$/i.test(host) || /run-agent-.*\.trae\.cn$/i.test(host);
    const isCapacitor = proto === "capacitor:" || proto === "file:" || proto === "cdvfile:";
    const uaHintsMock = ua.includes("Trae") || ua.includes("Volo");
    if (isTraeMockHost || (uaHintsMock && proto === "https:")) {
      mode = "mock-browser";
    } else if (isCapacitor) {
      mode = "capacitor";
    } else if (proto === "http:" || proto === "https:") {
      mode = "browser-dev";
    }
  }
  const location = typeof window !== "undefined" ? window.location.origin : "";
  return { mode, location };
}

onErrorCaptured((err: any, _instance: unknown, info: string) => {
  // 防止无限递归：如果已经是 error 状态，不再捕获（fallback 自己崩了）
  if (rootError.value) return false;

  console.error("[App] Vue error captured:", err, "| info:", info);
  rootError.value = true;
  // 🆕 拆 message：summary 简短显示，details（trace）作为 hint 单独成行
  // —— 避免 UI 上下两栏内容重复
  const rawMsg = err?.message || String(err) || "Unknown render error";
  const { summary, details } = splitErrorMessage(rawMsg);
  rootErrorSummary.value = summary;
  rootErrorDetails.value = details;
  rootErrorInfo.value = info || "unknown";
  rootErrorTime.value = new Date().toISOString();
  rootErrorStack.value = err?.stack || "";
  rootErrorContext.value = detectErrorContext();
  // 不阻止冒泡：让 Vue 仍然 console.error（包含完整 stack），方便 DevTools 调试
  return false;
});

/**
 * Task 9: 错误状态页 viewport meta 锁死
 *
 * 关键认知：android webview 默认 user-scalable=yes 时
 *   - 双指捏合 → 整页缩放（破坏错误页布局）
 *   - 双击 → 放大（阻碍用户复制错误详情）
 *
 * 解决：index.html 已默认锁死 user-scalable=no + maximum-scale=1.0，
 *       正常页 / 错误页都保持此设置（不再做动态切换——避免与默认行为冲突）。
 */

/**
 * 复制到剪贴板：让用户在 mock 浏览器里一键粘贴错误摘要 / 堆栈给 agent
 * mock 浏览器无 Network 面板，截图+文本是唯一能贴出诊断信息的途径
 */
async function copyToClipboard(text: string): Promise<void> {
  if (!text) return;
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      // fallback：textarea + execCommand（兼容旧 mock 浏览器）
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.left = "-9999px";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
  } catch (e) {
    console.warn("[App] copyToClipboard failed:", e);
  }
}

function copyErrorSummary() {
  const ctx = rootErrorContext.value;
  const lines = [
    `类型: ${rootErrorSummary.value}`,
    `触发阶段: ${rootErrorInfo.value}`,
    `时间: ${rootErrorTime.value}`,
    `模式: ${ctx.mode} @ ${ctx.location}`,
    rootErrorDetails.value ? `trace: ${rootErrorDetails.value}` : "",
  ].filter(Boolean);
  copyToClipboard(lines.join("\n"));
}

function copyErrorStack() {
  const lines = [`STACK (${rootErrorSummary.value} @ ${rootErrorTime.value}):`, rootErrorStack.value];
  copyToClipboard(lines.join("\n"));
}

function reloadPage() {
  rootError.value = false;
  rootErrorSummary.value = "";
  rootErrorDetails.value = "";
  rootErrorInfo.value = "";
  rootErrorTime.value = "";
  rootErrorStack.value = "";
  if (typeof window !== "undefined") {
    router.replace("/tabs/home").catch(() => {
      window.location.reload();
    });
  }
}
// ======================================

class ServiceGuardError extends Error {
  code: string;
  payload: ServiceGuardResult;

  constructor(message: string, code: string, payload: ServiceGuardResult) {
    super(message);
    this.name = "ServiceGuardError";
    this.code = code;
    this.payload = payload;
  }
}

async function runServiceGuard(): Promise<void> {
  try {
    await checkServiceGuard();
  } catch (e: any) {
    if (e?.code === "SERVICE_GUARD_BLOCKED" || e instanceof ServiceGuardError) {
      const payload: ServiceGuardResult = e.payload || {};
      serviceGuardDetail.value = payload.detail || e.message || "Unknown guard error";
      // 2026-06-10 改造：service-guard 不再返回 hint 字段（remediation 是结构化数组，不是单 string）
      serviceGuardHint.value = "";
      serviceGuardBlocked.value = true;
      throw e;
    }
    console.warn("[App] Service guard: API error, allowing entry —", e?.message);
  }
}

async function retryServiceGuard() {
  try {
    await runServiceGuard();
    serviceGuardBlocked.value = false;
    connect();
  } catch {}
}

const FIRST_LAUNCH_KEY = "encv-first-launch-done";

async function requestEssentialPermissions() {
  if (!isNative()) return;

  const done = localStorage.getItem(FIRST_LAUNCH_KEY);
  if (done) return;

  console.info("[App] First launch, requesting essential permissions");
  const notifResult = await requestNotificationPermission();
  console.info("[App] Notification permission:", notifResult.granted ? "granted" : "denied");
  const storageResult = await requestStoragePermission();
  console.info("[App] Storage permission:", storageResult.granted ? "granted" : "denied");
  localStorage.setItem(FIRST_LAUNCH_KEY, "1");
}

async function applyScreenOrientation() {
  if (!isNative()) return;
  const orientation = localStorage.getItem("encv_screen_orientation") || "auto";
  try {
    const { ScreenOrientation } = await import("@capacitor/screen-orientation");
    if (orientation === "portrait") {
      await ScreenOrientation.lock({ orientation: "portrait" });
    } else if (orientation === "landscape") {
      await ScreenOrientation.lock({ orientation: "landscape" });
    } else {
      await ScreenOrientation.unlock();
    }
  } catch (e) {
    console.debug("[App] Failed to apply screen orientation:", e);
  }
}

onMounted(async () => {
  hijackConsole();
  initTheme();
  detectP3Support();
  autoInitVConsole();
  initHighRefreshRate();
  registerFileFeature(createAlistEncryptFeature());

  if (!isNative()) {
    try {
      await runServiceGuard();
    } catch {
      return;
    }
  }

  connect();
  await requestEssentialPermissions();
  applyScreenOrientation();
});

onUnmounted(() => {
  disconnect();
});
</script>

<style scoped>
.service-guard-blocked {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  width: 100%;
  background: var(--ion-background-color);
  padding: 24px;
}

.guard-content {
  text-align: center;
  max-width: 400px;
}

.guard-icon {
  font-size: 64px;
  color: var(--ion-color-warning);
  margin-bottom: 16px;
}

.guard-content h2 {
  font-size: 20px;
  font-weight: 700;
  color: var(--ion-text-color);
  margin: 0 0 8px;
}

.guard-content p {
  font-size: 14px;
  color: var(--encv-text-secondary);
  margin: 0 0 16px;
  line-height: 1.5;
}

.guard-message {
  white-space: pre-line;
  text-align: left;
  font-size: 13px;
  max-height: 50vh;
  overflow-y: auto;
}

.guard-detail {
  display: block;
  font-size: 12px;
  color: var(--ion-color-danger);
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  border-radius: 8px;
  padding: 10px 14px;
  margin: 0 0 12px;
  text-align: left;
  word-break: break-all;
  white-space: pre-wrap;
}

.guard-hint {
  display: block;
  font-size: 11px;
  color: var(--ion-color-medium);
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-radius: 6px;
  padding: 8px 12px;
  margin: 0 0 20px;
  text-align: left;
  white-space: pre-wrap;
  font-family: monospace;
}

.guard-retry-btn {
  --border-radius: 8px;
}

/* ===== Vue 错误边界 fallback（开发期 / 生产期都生效） ===== */
.root-error-fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  width: 100%;
  background: var(--ion-background-color);
  padding: 24px;
  overflow-y: auto;
}

/* Task 9: 错误状态页 touch-action / user-select
   关键认知：android webview 默认 user-scalable=yes 时
     - 双指捏合 → 整页缩放（破坏错误页布局）
     - 双击 → 放大（阻碍用户复制错误详情）
   修复：touch-action: manipulation 禁用双击缩放 + 允许 pan/tap
        user-select: text 允许选择错误堆栈文本（用户复制给 agent 看）
   同步：viewport meta 在 index.html 已默认锁死 user-scalable=no + maximum-scale=1.0 */
.error-state {
  touch-action: manipulation;
  user-select: text;
  -webkit-user-select: text;
}

.error-content {
  text-align: center;
  max-width: 560px;
  width: 100%;
}

.error-icon {
  font-size: 64px;
  color: var(--ion-color-danger);
  margin-bottom: 16px;
}

.error-content h2 {
  font-size: 20px;
  font-weight: 700;
  color: var(--ion-text-color);
  margin: 0 0 8px;
}

.error-message {
  font-size: 14px;
  color: var(--encv-text-secondary);
  margin: 0 0 20px;
  line-height: 1.5;
}

/* ===== 上：红色 detail 区域（详细信息） ===== */
.error-detail-panel {
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  border-left: 3px solid var(--ion-color-danger);
  border-radius: 8px;
  padding: 12px 14px;
  margin: 0 0 12px;
  text-align: left;
}

.error-detail-header,
.error-stack-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid rgba(var(--ion-color-medium-rgb), 0.15);
}

.error-detail-icon {
  font-size: 16px;
  color: var(--ion-color-danger);
}

.error-detail-title,
.error-stack-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-color-danger);
  flex: 1;
}

.copy-btn {
  background: transparent;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.3);
  border-radius: 4px;
  padding: 2px 6px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  color: var(--encv-text-secondary);
  font-size: 11px;
  transition: background 0.15s;
}

.copy-btn:hover {
  background: rgba(var(--ion-color-medium-rgb), 0.1);
}

.copy-btn ion-icon {
  font-size: 14px;
}

.error-detail-body {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 12px;
}

.error-summary-line {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 4px 0;
  line-height: 1.4;
}

.error-summary-label {
  flex: 0 0 64px;
  font-size: 11px;
  color: var(--encv-text-secondary);
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.4px;
  padding-top: 1px;
}

.error-summary-value {
  flex: 1;
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11.5px;
  color: var(--ion-color-danger);
  word-break: break-all;
  white-space: pre-wrap;
}

.error-trace-block {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px dashed rgba(var(--ion-color-danger-rgb), 0.2);
}

.error-trace-label {
  font-size: 11px;
  color: var(--encv-text-secondary);
  font-weight: 600;
  margin-bottom: 4px;
}

.error-trace-body {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 10.5px;
  color: var(--ion-color-danger);
  background: rgba(var(--ion-color-danger-rgb), 0.04);
  border-radius: 4px;
  padding: 6px 8px;
  margin: 0;
  word-break: break-all;
  white-space: pre-wrap;
  max-height: 25vh;
  overflow-y: auto;
}

/* ===== 下：灰色 stack 区域（原始堆栈 + 排错文档） ===== */
.error-stack-panel {
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-left: 3px solid var(--ion-color-medium);
  border-radius: 8px;
  padding: 12px 14px;
  margin: 0 0 16px;
  text-align: left;
}

.error-stack-icon {
  font-size: 16px;
  color: var(--encv-text-secondary);
}

.error-stack-title {
  color: var(--encv-text-secondary);
}

.error-stack-body {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 10.5px;
  color: var(--encv-text-secondary);
  background: rgba(0, 0, 0, 0.04);
  border-radius: 4px;
  padding: 8px 10px;
  margin: 0 0 10px;
  word-break: break-all;
  white-space: pre-wrap;
  max-height: 30vh;
  overflow-y: auto;
  line-height: 1.45;
}

.error-stack-empty {
  font-size: 11px;
  color: var(--encv-text-secondary);
  font-style: italic;
  margin: 0 0 10px;
}

.error-stack-footer {
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.15);
  padding-top: 8px;
}

.error-stack-hints {
  font-size: 11px;
  color: var(--encv-text-secondary);
  line-height: 1.5;
}

.error-stack-hints strong {
  display: block;
  font-size: 11px;
  color: var(--encv-text-secondary);
  margin-bottom: 4px;
  font-weight: 600;
}

.error-stack-hints ul {
  margin: 0;
  padding-left: 16px;
}

.error-stack-hints li {
  margin: 2px 0;
}

.error-stack-hints code {
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  padding: 0 4px;
  border-radius: 2px;
  font-size: 10.5px;
  font-family: ui-monospace, Menlo, monospace;
}

.hint-tag {
  display: inline-block;
  background: var(--ion-color-warning);
  color: #000;
  font-size: 9.5px;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 3px;
  margin-right: 4px;
  letter-spacing: 0.4px;
  text-transform: uppercase;
  vertical-align: 1px;
}

.error-reload-btn {
  --border-radius: 8px;
  margin-top: 4px;
}
</style>

<style>
/* 通用 ion-toggle 暗黑模式适配 — 非 scoped，作用于所有 toggle */
ion-toggle {
  --track-background: #424242;
  --track-background-checked: var(--ion-color-primary);
  --handle-background: var(--ion-color-primary);
  --handle-background-checked: #ffffff;
}

/* 覆盖 ion-item 内部 .ion-color 上下文导致的 ON 状态手柄变黑 */
ion-toggle.toggle-checked::part(handle) {
  background: #ffffff;
}

/* 背景高斯模糊 + 全面透明化设计规范 */
ion-content,
ion-header,
ion-toolbar,
.encv-blur-surface {
  --backdrop-filter: blur(var(--encv-bg-blur, 0px));
  backdrop-filter: blur(var(--encv-bg-blur, 0px));
  -webkit-backdrop-filter: blur(var(--encv-bg-blur, 0px));
}

/* 瑰彩显示：CSS 滤镜增强对比度与饱和度（网页端也生效） */
ion-page {
  filter: var(--encv-vivid-filter, none);
}

ion-content {
  --background: var(--ion-background-color);
  background: var(--ion-background-color);
}

ion-toolbar {
  --background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.85);
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.85);
}

body.dark ion-toolbar {
  --background: rgba(26, 26, 26, 0.85);
  background: rgba(26, 26, 26, 0.85);
}

/* ===== Tab bar 半透明毛玻璃 + 微光动画 ===== */
ion-tab-bar {
  --background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.78);
  --color: var(--ion-text-color);
  --color-selected: var(--ion-color-primary);
  --border: none;
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.78);
  backdrop-filter: blur(20px) saturate(1.8);
  -webkit-backdrop-filter: blur(20px) saturate(1.8);
  border-top: 1px solid rgba(var(--ion-text-color-rgb), 0.08);
  box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.04);
  position: relative;
  overflow: hidden;
}

body.dark ion-tab-bar {
  --background: rgba(26, 26, 26, 0.85);
  background: rgba(26, 26, 26, 0.85);
}

ion-tab-bar::before {
  content: '';
  position: absolute;
  top: 0;
  left: -50%;
  width: 50%;
  height: 100%;
  background: linear-gradient(90deg,
    transparent,
    rgba(255, 255, 255, 0.08),
    transparent);
  animation: encvTabBarShine 4s ease-in-out infinite;
  pointer-events: none;
  z-index: 0;
}

ion-tab-bar > * {
  position: relative;
  z-index: 1;
}

ion-tab-button {
  --background: transparent;
  --background-focused: rgba(var(--ion-color-primary-rgb), 0.12);
  --background-hover: rgba(var(--ion-color-primary-rgb), 0.06);
  --color: var(--ion-color-medium);
  --color-selected: var(--ion-color-primary);
  transition: color 0.2s ease, transform 0.2s ease;
  background: transparent;
  font-weight: 500;
}

ion-tab-button.tab-selected {
  transform: translateY(-1px);
  font-weight: 600;
}

ion-tab-button ion-icon {
  transition: transform 0.25s cubic-bezier(0.34, 1.56, 0.64, 1);
}

ion-tab-button.tab-selected ion-icon {
  transform: scale(1.15);
  filter: drop-shadow(0 0 4px rgba(var(--ion-color-primary-rgb), 0.4));
}

@keyframes encvTabBarShine {
  0% { left: -50%; }
  100% { left: 100%; }
}

/* ===== 核心透明化规范：ion-item / ion-list / 卡片 ===== */
ion-list {
  --background: transparent;
  background: transparent;
  --border-color: transparent;
  border: none;
  padding: 0;
}

ion-item {
  --background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.55);
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.55);
  backdrop-filter: blur(var(--encv-bg-blur, 8px));
  -webkit-backdrop-filter: blur(var(--encv-bg-blur, 8px));
  --border-color: rgba(var(--ion-text-color-rgb), 0.06);
  --inner-border-width: 0;
}

body.dark ion-item {
  --background: rgba(30, 30, 30, 0.6);
  background: rgba(30, 30, 30, 0.6);
  --border-color: rgba(255, 255, 255, 0.04);
}

ion-list-header {
  --background: transparent;
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.3);
}

body.dark ion-list-header {
  background: rgba(30, 30, 30, 0.3);
}

.home-card {
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.6) !important;
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}

body.dark .home-card {
  background: rgba(30, 30, 30, 0.65) !important;
}

/* 输入框清空按钮：圆边框 + 半透明背景 */
ion-input .input-clear-icon {
  background: rgba(var(--ion-text-color-rgb), 0.08);
  border: 1.5px solid rgba(var(--ion-text-color-rgb), 0.15);
  border-radius: 50%;
  width: 20px;
  height: 20px;
  font-size: 12px;
  color: var(--ion-color-medium);
}
ion-input .input-clear-icon:hover {
  background: rgba(var(--ion-text-color-rgb), 0.14);
  border-color: rgba(var(--ion-text-color-rgb), 0.25);
}

.player-card {
  background: linear-gradient(135deg, rgba(var(--ion-color-primary-rgb), 0.12), rgba(var(--ion-color-primary-rgb), 0.04)) !important;
  backdrop-filter: blur(12px);
}

/* P3 瑰彩显示：增强颜色饱和度与对比度 */
@media (color-gamut: p3) {
  :root {
    color-scheme: light dark;
  }
  ion-card,
  .preset-card,
  .config-field,
  .task-card,
  .theme-color-picker {
    --encv-color-gamut: p3;
  }
  .p3-enhanced ion-icon {
    color: color(display-p3 1 0 0);
  }
  .p3-enhanced .preset-card-active {
    background: color(display-p3 var(--ion-color-primary-rgb) / 0.08);
  }
}

/* 强制 P3 模式：当用户手动开启时，通过 CSS 变量应用 display-p3 色域 */
:root {
  --encv-color-gamut: srgb;
}

/* 当 --encv-color-gamut 为 display-p3 时，强制使用 P3 色彩空间渲染关键元素 */
@supports (color: color(display-p3 1 0 0)) {
  :root:has([style*="--encv-color-gamut: display-p3"]) ion-page,
  :root[style*="--encv-color-gamut: display-p3"] ion-page {
    color-gamut: display-p3;
  }

  :root:has([style*="--encv-color-gamut: display-p3"]) *,
  :root[style*="--encv-color-gamut: display-p3"] * {
    color-gamut: display-p3;
  }
}

/* 降级方案：不支持 :has() 时，用 class 方式触发 */
.encv-force-p3 {
  color-gamut: display-p3 !important;
}
.encv-force-p3 * {
  color-gamut: display-p3 !important;
}

/* ============================================
   ENCV Toast 系统 — 顶部展示 + 堆叠 + Ionic 官方动画
   完全不覆盖 Ionic overlay 布局，只调整视觉外观
   ============================================ */
.encv-toast {
  --background: transparent;
  --box-shadow: none;
  --color: var(--ion-text-color);
  --border-radius: 14px;
}

.encv-toast .toast-wrapper {
  border-radius: 14px;
  padding: 12px 16px;
  display: flex;
  align-items: center;
  gap: 10px;
  box-shadow:
    0 4px 24px rgba(0, 0, 0, 0.12),
    0 1px 4px rgba(0, 0, 0, 0.06);
  margin: 6px 16px 0;
  max-width: 380px;
}

.encv-toast .toast-message {
  font-size: 13.5px;
  font-weight: 500;
  letter-spacing: 0.01em;
  flex: 1;
  color: inherit;
  line-height: 1.4;
}

.encv-toast .toast-button {
  --padding-start: 6px;
  --padding-end: 6px;
  --border-radius: 50%;
  min-width: 28px;
  min-height: 28px;
  font-size: 15px;
  color: var(--ion-color-medium);
  margin-left: 2px;
  flex-shrink: 0;
}

.encv-toast--primary {
  --background: rgba(var(--ion-color-primary-rgb), 0.92);
  --color: #ffffff;
}
body.dark .encv-toast--primary {
  --background: rgba(var(--ion-color-primary-rgb), 0.88);
}

.encv-toast--success {
  --background: rgba(34, 197, 94, 0.92);
  --color: #ffffff;
}

.encv-toast--danger,
.encv-toast--error {
  --background: rgba(239, 68, 68, 0.92);
  --color: #ffffff;
}

.encv-toast--warning {
  --background: rgba(245, 158, 11, 0.92);
  --color: #1a1a1a;
}

.encv-toast--medium {
  --background: rgba(115, 115, 128, 0.9);
  --color: #ffffff;
}

/* ============================================
   Context Popover 底部面板（modalController.create 模式）
   全宽 + 从底部滑入 + 圆角顶部
   ============================================ */
.context-popover-modal .modal-wrapper {
  align-items: flex-end;
}

.context-popover-modal ion-modal {
  --border-radius: 16px 16px 0 0;
  --height: auto;
  --max-height: 70vh;
  box-shadow: 0 -4px 24px rgba(0, 0, 0, 0.15);
}
</style>
