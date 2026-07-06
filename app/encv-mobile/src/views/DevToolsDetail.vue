<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.title') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.debugTools') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="bugOutline" slot="start"></ion-icon>
          <ion-toggle :checked="vconsoleEnabled" @ionChange="handleVConsoleToggle">{{ t('devtools.vconsole') }}</ion-toggle>
        </ion-item>
        <!-- 🆕 2026-06-17：vConsole 之外的所有日志相关设置合并到「日志设置」三级页面 -->
        <ion-item button @click="goLogSettings" detail>
          <ion-icon :icon="terminal" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.logSettings') }}</h3>
            <p>{{ t('devtools.logSettingsDesc') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 自动化测试：生产构建也可访问（与沙箱预览的 isDev 限制不同） -->
      <ion-list>
        <!-- 🆕 2026-06-17：section header 变可点击入口，子项（plugin / webdav / sparse）整体搬到 AutomationTestsHub -->
        <ion-item button detail @click="goAutomationHub" class="section-entry">
          <ion-icon :icon="flaskOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.automationTests') }}</h3>
            <p>{{ t('devtools.automationTestsHint') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 沙箱预览：dev 专属入口，生产构建整段 v-if false 移除 -->
      <ion-list v-if="isDev">
        <ion-list-header>
          <ion-label>{{ t('devtools.sandboxPreview') }}</ion-label>
          <ion-badge slot="end" color="warning" class="scope-badge scope-dev">
            <ion-icon :icon="bugOutline" class="scope-badge-icon"></ion-icon>
            <span class="scope-text">DEV</span>
          </ion-badge>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.sandboxPreviewHint') }}</p>
        <ion-item button detail @click="openPreviewOpenList">
          <ion-icon :icon="eyeOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.previewOpenList') }}</h3>
            <p>{{ t('devtools.previewOpenListDesc') }}</p>
          </ion-label>
        </ion-item>
        <ion-item button detail @click="openPreviewOpenListPlugin">
          <ion-icon :icon="extensionPuzzleOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.previewOpenListLive') }}</h3>
            <p>{{ t('devtools.previewOpenListLiveDesc') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <!-- 🆕 2026-06-17：section header 变可点击入口，原型卡片循环整体搬到 ComposePrototypesHub -->
        <ion-item button detail @click="goComposePrototypesHub" class="section-entry">
          <ion-icon :icon="extensionPuzzleOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.composePrototypes') }}</h3>
            <p>{{ t('devtools.composePrototypesHint') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>模拟世界</ion-label>
        </ion-list-header>
        <ion-item button detail @click="goChronicle">
          <ion-icon :icon="bookOutline" slot="start" color="tertiary"></ion-icon>
          <ion-label>
            <h3>编年史</h3>
            <p>世界历史事件时间线</p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useDevTools } from "@/composables/useDevTools";
import { useI18n } from "@/composables/useI18n";
import { useRouter } from "vue-router";

const { t } = useI18n();
const router = useRouter();
const { vconsoleEnabled, toggleVConsole } = useDevTools();

function _goLogSettings() {
  router.push("/tabs/settings/devtools/log-settings");
}

// 🆕 2026-06-17：自动化测试总览入口（原 section 内 3 个 ion-item 已整体搬到 AutomationTestsHub）
function _goAutomationHub() {
  router.push("/tabs/settings/devtools/automation-hub");
}

// 🆕 2026-06-17：Compose UI 原型总览入口（原 prototype 卡片循环已整体搬到 ComposePrototypesHub）
function _goComposePrototypesHub() {
  router.push("/tabs/settings/devtools/compose-prototypes-hub");
}

function _goChronicle() {
  router.push("/tabs/settings/chronicle");
}

// 沙箱预览：强制整页跳转，绕过 Vue Router 拦截
// 为什么不用 <router-link>：<router-link> 只走 in-app 路由，/openlist-ui/ 不在路由表
// 为什么不用 <a href>：Vue Router 4 在某些 setup 下会拦截 plain <a> 点击事件，
//   导致 router 试图导航到 /openlist-ui/ 失败、渲空 <ion-router-outlet>
// 为什么不用 window.open(_, '_blank')：会破坏 OpenPreview 会话（用户需手动切回 tab）
// 为什么用 window.location.assign：触发完整页面加载，浏览器原生处理同源跳转
const _isDev = import.meta.env.DEV;
function _openPreviewOpenList() {
  window.location.assign("/openlist-ui/");
}

// 跳 :5174 plugin-openlist 管理 UI（OpenListHome/Settings/ConfigEditor）
// 与现有 /openlist-ui/ 入口的区别：openlist-ui 是 dev 沙箱代理，plugin-openlist
// 是 Capacitor OpenList plugin 自身的管理 UI（独立前端，不在 encv-mobile 内）
//
// 为什么跳 /api/preview/plugin-openlist/（encv-go 后端相对路径）而不是 :5174：
// - 独立后端协调：encv-go 后端 reverse proxy 该路径到 127.0.0.1:5174
//   （见 internal/server/mobile_api.go handlePluginOpenlistProxyGin）
// - 不依赖 vite：vite.config.ts 的 openlist-ui-proxy 只能代理 :5244（OpenList 真实前端），
//   不能代理 :5174（plugin-openlist 是另一个独立 vite 进程）
// - 不依赖 OpenPreview 会话：跳相对路径不破坏当前 OpenPreview 工具锚定的 :5173
// - Capacitor native 端 127.0.0.1 指向设备本身，跳绝对 URL 不可达
//   走 encv-go 后端相对路径，由后端内部处理上游转发
// - 跟 openPreviewOpenList（/openlist-ui/）保持同一种风格：相对路径 + 整页跳转
function _openPreviewOpenListPlugin() {
  window.location.assign("/api/preview/plugin-openlist/");
}

function _handleVConsoleToggle(event: CustomEvent) {
  toggleVConsole(event.detail.checked);
}
</script>

<style scoped>
.section-hint {
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  margin: 0 16px 8px;
  line-height: 1.5;
}

.scope-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 8px;
  --padding-top: 3px;
  --padding-bottom: 3px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
}
.scope-badge-icon {
  font-size: 12px;
}
.scope-synced {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  --color: var(--ion-color-primary);
}
.scope-dev {
  --background: rgba(var(--ion-color-warning-rgb), 0.18);
  --color: var(--ion-color-warning-shade);
}
.scope-prod {
  --background: rgba(var(--ion-color-success-rgb), 0.16);
  --color: var(--ion-color-success-shade);
}
</style>
