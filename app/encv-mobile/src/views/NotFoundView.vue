<template>
  <ion-page>
    <ion-header>
      <ion-toolbar color="warning">
        <ion-title>
          <ion-icon :icon="alertCircleOutline" class="title-icon" />
          路径未找到
        </ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="nf-card">
        <ion-icon :icon="helpCircleOutline" class="nf-icon" color="warning" />
        <h2 class="nf-title">404 · Path Not Found</h2>
        <p class="nf-hint">
          ENCV 主 app 没有匹配 <code>{{ attemptedPath }}</code> 的路由。
        </p>

        <!-- 防御性：列出所有可用路由，让开发者立即知道有哪些路径 -->
        <div class="nf-routes">
          <p class="nf-routes-title">可用路由（/tabs/ 下）：</p>
          <ul class="nf-routes-list">
            <li v-for="r in availableRoutes" :key="r.path">
              <code>{{ r.path }}</code>
              <span class="nf-route-desc">— {{ r.desc }}</span>
            </li>
          </ul>
          <p class="nf-routes-title nf-routes-title-top">顶层路由：</p>
          <ul class="nf-routes-list">
            <li v-for="r in topLevelRoutes" :key="r.path">
              <code>{{ r.path }}</code>
              <span class="nf-route-desc">— {{ r.desc }}</span>
            </li>
          </ul>
        </div>

        <!-- 防御性：诊断信息（点击展开），让开发者知道是路径写错还是导入漏了 -->
        <details class="nf-debug">
          <summary>🔧 诊断信息（开发者）</summary>
          <div class="nf-debug-body">
            <div class="nf-debug-row">
              <span class="nf-debug-label">location.pathname:</span>
              <code>{{ pathname }}</code>
            </div>
            <div class="nf-debug-row">
              <span class="nf-debug-label">route.fullPath:</span>
              <code>{{ attemptedPath }}</code>
            </div>
            <div class="nf-debug-row">
              <span class="nf-debug-label">route.path:</span>
              <code>{{ currentRoute }}</code>
            </div>
            <div class="nf-debug-row">
              <span class="nf-debug-label">route.name:</span>
              <code>{{ routeName || '(n/a)' }}</code>
            </div>
            <div class="nf-debug-hint">
              💡 如果是访问 plugin-openlist web 的路径（如 <code>/openlist-ui/home</code>），
              请走 :16666 网关的 <code>/openlist-ui/</code> 路径
              （preview-gateway :16666 转发到 :5174 plugin-openlist-web vite）。
              <br /><br />
              详见 spec <code>unify-sandbox-preview-port</code>。
            </div>
          </div>
        </details>

        <div class="nf-actions">
          <ion-button @click="goHome" color="primary" expand="block">
            <ion-icon :icon="homeOutline" slot="start" />
            返回 /tabs/home
          </ion-button>
          <ion-button @click="goExtensions" fill="outline" expand="block">
            <ion-icon :icon="appsOutline" slot="start" />
            扩展管理
          </ion-button>
          <ion-button @click="reload" fill="clear" expand="block">
            <ion-icon :icon="refreshOutline" slot="start" />
            重新加载
          </ion-button>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();

const _attemptedPath = computed(() => route.fullPath || "/");
const _currentRoute = computed(() => route.path);
const _routeName = computed(() => (route.name ? String(route.name) : ""));
const _pathname = computed(() => (typeof window !== "undefined" ? window.location.pathname : "(n/a)"));

// 防御性：列出真实路由（与 src/router/index.ts 保持同步），开发者立刻知道有哪些路径可用
const _availableRoutes = [
  { path: "/tabs/home", desc: "首页" },
  { path: "/tabs/files", desc: "文件管理" },
  { path: "/tabs/tasks", desc: "任务" },
  { path: "/tabs/remote", desc: "远端连接" },
  { path: "/tabs/settings", desc: "设置" },
  { path: "/tabs/extensions", desc: "扩展管理" },
  { path: "/tabs/openlist", desc: "OpenList 管理（主 app 桥接）" },
  { path: "/tabs/settings/server", desc: "服务器设置" },
  { path: "/tabs/settings/server/http", desc: "HTTP 服务器" },
  { path: "/tabs/settings/server/admin", desc: "Admin 服务器" },
  { path: "/tabs/settings/server/webdav", desc: "WebDAV 服务器" },
  { path: "/tabs/settings/engine", desc: "【已废弃】加密引擎（迁移到 /tabs/settings/about/engine）" },
  { path: "/tabs/settings/about", desc: "关于" },
  { path: "/tabs/settings/about/engine", desc: "FFmpeg 引擎详情（三级）" },
  { path: "/tabs/settings/cache", desc: "缓存" },
  { path: "/tabs/settings/plugins", desc: "插件设置" },
  { path: "/tabs/settings/devtools", desc: "开发者工具" },
  { path: "/tabs/settings/appearance", desc: "外观" },
  { path: "/tabs/devlogs", desc: "开发日志" },
  { path: "/tabs/preview", desc: "文件预览" },
  { path: "/tabs/file-info", desc: "文件信息" },
];
const _topLevelRoutes = [
  { path: "/", desc: "重定向到 /tabs/home" },
  { path: "/player", desc: "播放器（ArtPlayer）" },
  { path: "/tabs/*", desc: "Tabs 内嵌路由" },
];

function _goHome() {
  router.replace("/tabs/home");
}
function _goExtensions() {
  router.replace("/tabs/extensions");
}
function _reload() {
  if (typeof window !== "undefined") {
    window.location.reload();
  }
}
</script>

<style scoped>
.title-icon {
  vertical-align: middle;
  margin-right: 6px;
  font-size: 20px;
}

.nf-card {
  max-width: 600px;
  margin: 32px auto;
  padding: 24px 20px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.nf-icon {
  font-size: 72px;
  margin-bottom: 8px;
}

.nf-title {
  font-size: 22px;
  font-weight: 700;
  margin: 0;
  color: var(--ion-color-warning-shade);
}

.nf-hint {
  font-size: 14px;
  line-height: 1.6;
  color: var(--ion-color-medium);
  margin: 0 0 8px 0;
  word-break: break-word;
}

.nf-hint code {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 12px;
  padding: 1px 5px;
  background: var(--ion-color-light);
  border-radius: 3px;
}

.nf-routes {
  width: 100%;
  text-align: left;
  margin: 12px 0;
  padding: 12px 16px;
  background: var(--ion-color-light);
  border-radius: 8px;
  box-sizing: border-box;
}

.nf-routes-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--ion-color-medium-shade);
  margin: 0 0 8px 0;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.nf-routes-title-top {
  margin-top: 12px;
}

.nf-routes-list {
  list-style: none;
  padding: 0;
  margin: 0;
  font-size: 11.5px;
  line-height: 1.7;
  columns: 2;
  column-gap: 16px;
}
.nf-routes-list li {
  word-break: break-all;
  break-inside: avoid;
}
.nf-routes-list code {
  font-family: ui-monospace, Menlo, monospace;
  background: var(--ion-background-color);
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 10.5px;
}
.nf-route-desc {
  color: var(--ion-color-medium);
  margin-left: 4px;
}

.nf-debug {
  width: 100%;
  text-align: left;
  background: #1e1e1e;
  color: #d4d4d4;
  border-radius: 6px;
  padding: 8px 12px;
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
}
.nf-debug summary {
  cursor: pointer;
  color: #4ec9b0;
  font-weight: 600;
  padding: 4px 0;
  user-select: none;
}
.nf-debug-body {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid #333;
}
.nf-debug-row {
  display: flex;
  gap: 8px;
  padding: 3px 0;
  align-items: baseline;
}
.nf-debug-label {
  color: #9cdcfe;
  flex-shrink: 0;
  min-width: 200px;
}
.nf-debug-row code {
  color: #ce9178;
  word-break: break-all;
}
.nf-debug-hint {
  margin-top: 8px;
  padding: 6px 8px;
  background: #252526;
  border-left: 3px solid #f59e0b;
  border-radius: 3px;
  color: #d4d4d4;
  line-height: 1.5;
}
.nf-debug-hint code {
  background: #1e1e1e;
  padding: 0 4px;
  border-radius: 2px;
  color: #ce9178;
}

.nf-actions {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}
</style>
