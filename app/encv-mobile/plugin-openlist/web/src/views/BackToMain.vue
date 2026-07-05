<template>
  <!--
    "返回 ENCV 主页面"视图
    ====================
    在 plugin-openlist (vite :5174) 内**全屏 iframe** 加载 encv-mobile (vite :5173)，
    绕过 Trae 沙箱 OpenPreview 工具「单 port 限制」(trae_web_sandbox_network.md §8.4)：
    - 沙箱模式下 OpenPreview 一次只能注册一个 port，注册 plugin-openlist :5174 时
      就不能同时注册 encv-mobile :5173
    - 16000 入口代理到 5174，5173 没法被 16000 代理到
    - 如果让用户直接跳 `http://localhost:5173/...`，沙箱下 5173 不对外（sandbox 只暴露 16000）会失败
    - 如果直连模式（开发者本地），跳 5173 倒是工作，但跟沙箱不通用

    **iframe 方案的沙箱可行性**：
    - iframe 在 :5174 内部执行，发起对 127.0.0.1:5173 的请求
    - 这是 sandbox 内部端口互通（vite 监听 0.0.0.0:5173），不依赖 16000 代理
    - 用户浏览器看到的还是 16000 入口（OpenPreview 锚定 5174），iframe 内容是 5173

    **直连可行性**：host machine 上两个 vite 都监听 localhost，iframe 同样工作。

    **代价**：
    - 用户在 iframe 内跟 encv-mobile 交互后，状态不会写回 plugin-openlist（双向独立）
    - 这是合理的——plugin-openlist 是 "管理工具" 性质，不需要双向状态
  -->
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="#/home" />
        </ion-buttons>
        <ion-title>
          <span>ENCV 主页面</span>
          <span class="iframe-host-tag">via iframe :5173</span>
        </ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload" title="重新加载">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loadError" class="iframe-error">
        <div class="iframe-error-row">
          <span class="error-icon">⚠️</span>
          <span class="error-title">encv-mobile :5173 加载失败</span>
        </div>
        <div class="iframe-error-text">
          <code>{{ loadError }}</code>
          <p>
            请确认 encv-mobile vite dev server 跑在
            <code>http://127.0.0.1:5173</code>：
          </p>
          <pre>bash app/encv-mobile/scripts/start-preview.sh</pre>
        </div>
      </div>

      <iframe
        v-show="!loadError"
        ref="iframeRef"
        :src="iframeSrc"
        class="encv-iframe"
        @error="onIframeError"
        @load="onIframeLoad"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
        referrerpolicy="no-referrer-when-downgrade"
      ></iframe>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";

// 目标 encv-mobile vite dev server 直连地址。
// 用 127.0.0.1 而非 localhost：避免某些环境 DNS 解析 localhost 到 ::1 失败。
// 用 5173 是 start-preview.sh 起的端口（被占时回退 5174，但 5174 是 plugin-openlist 自身占着）
// 走 preview-gateway 统一收口 :16666（沙箱 dev 唯一对外端口）
//   /tabs/remote  → 主 app Remote tab（不再硬编码 :5173，因为主 app vite 已迁到 :8100）
const ENCV_MAIN_URL = "http://localhost:16666/tabs/remote";

const _iframeRef = ref<HTMLIFrameElement | null>(null);
const iframeSrc = ref(ENCV_MAIN_URL);
const loadError = ref("");
let probeTimer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  // 健康探测：每秒检查 iframe 是否能成功加载（避免空白卡死没反馈）
  probeTimer = setInterval(probeHealth, 2000);
});

onUnmounted(() => {
  if (probeTimer) {
    clearInterval(probeTimer);
    probeTimer = null;
  }
});

function _reload() {
  loadError.value = "";
  // 给 iframe 加 cache buster 参数强制重载
  const sep = ENCV_MAIN_URL.includes("?") ? "&" : "?";
  iframeSrc.value = `${ENCV_MAIN_URL}${sep}_t=${Date.now()}`;
}

function _onIframeError() {
  // iframe @error 不一定靠谱（sandboxed 跨源时静默），但保险起见监听
  loadError.value = "iframe 触发 error 事件（可能是 :5173 离线或 sandbox 拒访问）";
}

function _onIframeLoad() {
  // iframe @load 触发：能加载就清掉错误（即使加载的是错误页也算 load）
  // 进一步状态由 probeHealth 检查
  if (loadError.value) {
    loadError.value = "";
  }
}

/**
 * 健康探测：fetch :16666 根路径，能 200 就认为 preview-gateway 在线
 * 不阻塞渲染：失败时只是设错误标记，不抛异常
 */
async function probeHealth() {
  try {
    const res = await fetch("http://localhost:16666/", {
      method: "HEAD",
      cache: "no-store",
      signal: AbortSignal.timeout(1500),
    });
    if (!res.ok) {
      loadError.value = `HTTP ${res.status}（:5173 异常）`;
    } else if (loadError.value) {
      // 恢复了
      loadError.value = "";
    }
  } catch (e: any) {
    loadError.value = `:5173 不可达：${e?.message || e}`;
  }
}
</script>

<style scoped>
.encv-iframe {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
  background: var(--ion-background-color);
}

.iframe-host-tag {
  display: inline-block;
  margin-left: 8px;
  padding: 1px 6px;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.3px;
  color: #fff;
  background: linear-gradient(90deg, #6366f1 0%, #8b5cf6 100%);
  border-radius: 3px;
  vertical-align: middle;
}

.iframe-error {
  margin: 16px;
  padding: 16px;
  border-radius: 10px;
  background: linear-gradient(135deg,
    rgba(239, 68, 68, 0.10) 0%,
    rgba(245, 158, 11, 0.10) 100%);
  border: 1px solid rgba(239, 68, 68, 0.4);
  border-left: 4px solid #ef4444;
}
.iframe-error-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.error-icon {
  font-size: 18px;
}
.error-title {
  font-size: 14px;
  font-weight: 700;
  color: #ef4444;
}
.iframe-error-text {
  font-size: 12px;
  line-height: 1.6;
  color: var(--ion-text-color);
}
.iframe-error-text code {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
  padding: 1px 4px;
  background: rgba(0, 0, 0, 0.08);
  border-radius: 3px;
}
.iframe-error-text pre {
  margin: 6px 0 0;
  padding: 6px 8px;
  background: rgba(0, 0, 0, 0.06);
  border-radius: 4px;
  font-size: 11px;
  overflow-x: auto;
}
</style>
