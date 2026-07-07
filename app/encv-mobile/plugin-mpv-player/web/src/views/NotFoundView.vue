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
          mpv-player/web 没有匹配 <code>{{ attemptedPath }}</code> 的路由。
        </p>

        <div class="nf-routes">
          <p class="nf-routes-title">可用路由：</p>
          <ul class="nf-routes-list">
            <li v-for="r in availableRoutes" :key="r.path">
              <code>/mpv-ui/#{{ r.path }}</code>
              <span class="nf-route-desc">— {{ r.desc }}</span>
            </li>
          </ul>
        </div>

        <details class="nf-debug">
          <summary>🔧 诊断信息（开发者）</summary>
          <div class="nf-debug-body">
            <div class="nf-debug-row">
              <span class="nf-debug-label">location.pathname:</span>
              <code>{{ pathname }}</code>
            </div>
            <div class="nf-debug-row">
              <span class="nf-debug-label">location.hash:</span>
              <code>{{ hash || '(空)' }}</code>
            </div>
            <div class="nf-debug-row">
              <span class="nf-debug-label">router base:</span>
              <code>{{ routerBase }}</code>
            </div>
            <div class="nf-debug-row">
              <span class="nf-debug-label">router currentRoute.path:</span>
              <code>{{ currentRoute }}</code>
            </div>
          </div>
        </details>

        <div class="nf-actions">
          <ion-button @click="goHome" color="primary" expand="block">
            <ion-icon :icon="homeOutline" slot="start" />
            返回 /home
          </ion-button>
          <ion-button @click="reload" fill="outline" expand="block">
            <ion-icon :icon="refreshOutline" slot="start" />
            重新加载
          </ion-button>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { alertCircleOutline, helpCircleOutline, homeOutline, refreshOutline } from "ionicons/icons";
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();

const attemptedPath = computed(() => route.fullPath || "/");
const currentRoute = computed(() => route.path);
const pathname = computed(() => (typeof window !== "undefined" ? window.location.pathname : "(n/a)"));
const hash = computed(() => (typeof window !== "undefined" ? window.location.hash : ""));
const routerBase = computed(() => (router.options.history as any).state?.base || (router.options.history as any).base || "(unknown)");

const availableRoutes = [
  { path: "/", desc: "重定向到 /home" },
  { path: "/home", desc: "主页（最近播放 + 媒体库 + 播放列表）" },
  { path: "/settings", desc: "设置" },
  { path: "/devlogs", desc: "MPV 日志" },
  { path: "/about", desc: "关于 MPV" },
];

function goHome() {
  router.replace("/home");
}

function reload() {
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
  max-width: 520px;
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

.nf-routes-list {
  list-style: none;
  padding: 0;
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
}
.nf-routes-list li {
  word-break: break-all;
}
.nf-routes-list code {
  font-family: ui-monospace, Menlo, monospace;
  background: var(--ion-background-color);
  padding: 1px 5px;
  border-radius: 3px;
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

.nf-actions {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}
</style>
