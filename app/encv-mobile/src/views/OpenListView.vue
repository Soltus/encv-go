<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/remote" />
        </ion-buttons>
        <ion-title>OpenList 管理</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- 嵌入式 WebView 渲染 plugin-openlist Content() 内的 OpenList UI -->
      <!--
        注意：此组件作为主 app 与 plugin-openlist 的桥接点。
        实际的 OpenList 页面通过 plugin-openlist Content() 内的嵌入式 WebView 渲染，
        主 app 这层只负责提供一个独立的 Capacitor route + 状态摘要卡片。
      -->
      <OpenListStatusCard :runtime="runtime" />

      <div class="info-box">
        <p>
          <strong>提示</strong>：OpenList 详细管理界面由 plugin-openlist 扩展通过嵌入式 WebView 提供。
          完整功能（启停控制、密码设置、Config 编辑、下载管理）请通过以下方式访问：
        </p>
        <ul>
          <li>方式 A：在 OpenList 扩展卡片中启用并通过 Content() 渲染（推荐）</li>
          <li>方式 B：通过外部 Intent 启动 plugin-openlist 独立 Activity（可选）</li>
        </ul>
      </div>

      <!--
        沙箱 dev 入口（preview-gateway 统一收口 :16666）：
        - /openlist-ui/  → plugin-openlist 独立 Vite SPA (Vue + Ionic)
        - /openlist/     → OpenList Go 后端真实管理 UI
        因为三者同源（都是 http://localhost:16666），用 <a href> 即可整页跳转
        （主 app 是 Capacitor WebView 或浏览器，router.push 不支持跨 SPA 跳转）
      -->
      <div class="quick-entries">
        <h3 class="entries-title">沙箱 dev 快速入口</h3>
        <ion-button
          expand="block"
          fill="outline"
          color="primary"
          href="/openlist-ui/"
          class="ion-margin"
        >
          <ion-icon :icon="openOutline" slot="start" />
          打开 plugin-openlist 管理 UI
        </ion-button>
        <ion-button
          expand="block"
          fill="outline"
          color="secondary"
          href="/openlist/"
          class="ion-margin"
        >
          <ion-icon :icon="globeOutline" slot="start" />
          打开 OpenList 真实管理 UI
        </ion-button>
      </div>

      <ion-button expand="block" @click="reloadStatus" class="ion-margin">
        <ion-icon :icon="refreshOutline" slot="start" />
        刷新状态
      </ion-button>

      <ion-button
        expand="block"
        :color="runtime.running ? 'danger' : 'primary'"
        @click="toggleService"
        :disabled="isControlling"
        class="ion-margin"
      >
        <ion-spinner v-if="isControlling" name="crescent" />
        <ion-icon
          v-else
          :icon="runtime.running ? powerOutline : playOutline"
          slot="start"
        />
        {{ runtime.running ? '停止 OpenList' : '启动 OpenList' }}
      </ion-button>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  globeOutline,
  openOutline,
  playOutline,
  powerOutline,
  refreshOutline,
} from "ionicons/icons";

import type { OpenListRuntime } from "@/components-shared";
import { OpenListNative } from "@/plugins/openlist-native";
import { onMounted, onUnmounted, ref } from "vue";

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

const isControlling = ref(false);
let refreshTimer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  reloadStatus();
  refreshTimer = setInterval(reloadStatus, 3000);
});

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
});

function reloadStatus() {
  runtime.value = OpenListNative.getStatus();
}

async function toggleService() {
  if (isControlling.value) return;
  isControlling.value = true;
  try {
    if (runtime.value.running) {
      OpenListNative.stopOpenList();
    } else {
      OpenListNative.startOpenList();
    }
    setTimeout(reloadStatus, 1000);
  } finally {
    isControlling.value = false;
  }
}
</script>

<style scoped>
.info-box {
  margin: 12px;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--ion-color-light);
  font-size: 12px;
  line-height: 1.6;
}
.info-box p { margin: 0 0 8px 0; }
.info-box ul { margin: 0; padding-left: 20px; }
</style>
