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
        <ion-list-header>
          <ion-label>{{ t('devtools.automationTests') }}</ion-label>
          <ion-badge slot="end" color="success" class="scope-badge scope-prod">
            <ion-icon :icon="rocketOutline" class="scope-badge-icon"></ion-icon>
            <span class="scope-text">{{ t('devtools.availableInProd') }}</span>
          </ion-badge>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.automationTestsHint') }}</p>
        <ion-item button detail @click="goAutomationTests">
          <ion-icon :icon="flaskOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.automationTestsEntry') }}</h3>
            <p>{{ t('devtools.automationTestsEntryDesc') }}</p>
          </ion-label>
        </ion-item>
        <!-- 🆕 2026-06-11 v6：webdav 服务自动化测试入口 -->
        <ion-item button detail @click="goWebDavTests">
          <ion-icon :icon="cloudUploadOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.webdavTests') }}</h3>
            <p>{{ t('devtools.webdavTestsHint') }}</p>
          </ion-label>
        </ion-item>
        <!-- 🆕 2026-06-11：ECv4 容量边界测试入口（100×128GB sparse 虚拟容器） -->
        <ion-item button detail @click="goSparseContainerTest">
          <ion-icon :icon="serverOutline" slot="start" color="warning"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.title') }}</h3>
            <p>{{ t('devtools.sparseContainer.entryHint') }}</p>
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
        <ion-list-header>
          <ion-label>{{ t('devtools.composePrototypes') }}</ion-label>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.composePrototypesHint') }}</p>

        <div class="prototype-cards">
          <div
            v-for="proto in prototypes"
            :key="proto.id"
            class="prototype-card"
            @click="handlePrototypeClick(proto)"
          >
            <div class="proto-header">
              <div class="proto-icon-wrap" :style="{ background: proto.accentColor }">
                <ion-icon :icon="iconMap[proto.icon]" class="proto-icon"></ion-icon>
              </div>
              <div class="proto-title-area">
                <h3 class="proto-title">{{ proto.name }}</h3>
                <p class="proto-route">{{ proto.route }}</p>
              </div>
              <ion-icon :icon="chevronForward" class="proto-arrow"></ion-icon>
            </div>
            <div class="proto-compose-path">
              <span class="path-label">Compose</span>
              <code class="path-value">{{ proto.composePath }}</code>
            </div>
            <p class="proto-desc">{{ proto.description }}</p>
          </div>
        </div>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonContent, IonList, IonListHeader, IonItem, IonIcon, IonLabel, IonToggle,
  IonBadge,
} from '@ionic/vue'
import {
  bugOutline, chevronForward, playCircleOutline, musicalNotesOutline,
  colorPaletteOutline, settingsOutline, terminal, eyeOutline, cloudUploadOutline,
  extensionPuzzleOutline, flaskOutline, rocketOutline,
  serverOutline,  // 🆕 ECv4 容量边界测试
} from 'ionicons/icons'
import { useRouter } from 'vue-router'
import { useI18n } from '@/composables/useI18n'
import { useDevTools } from '@/composables/useDevTools'
import { getAllPrototypes } from './prototypes/registry'

const { t } = useI18n()
const router = useRouter()
const { vconsoleEnabled, toggleVConsole } = useDevTools()

function goLogSettings() {
  router.push('/tabs/settings/devtools/log-settings')
}

const prototypes = getAllPrototypes()

const iconMap: Record<string, string> = {
  'play-circle': playCircleOutline,
  'settings': settingsOutline,
  'musical-notes': musicalNotesOutline,
  'color-palette': colorPaletteOutline,
}

function handlePrototypeClick(proto: typeof prototypes[0]) {
  router.push(`/tabs/settings/devtools/prototype/${proto.id}`)
}

// 沙箱预览：强制整页跳转，绕过 Vue Router 拦截
// 为什么不用 <router-link>：<router-link> 只走 in-app 路由，/openlist-ui/ 不在路由表
// 为什么不用 <a href>：Vue Router 4 在某些 setup 下会拦截 plain <a> 点击事件，
//   导致 router 试图导航到 /openlist-ui/ 失败、渲空 <ion-router-outlet>
// 为什么不用 window.open(_, '_blank')：会破坏 OpenPreview 会话（用户需手动切回 tab）
// 为什么用 window.location.assign：触发完整页面加载，浏览器原生处理同源跳转
const isDev = import.meta.env.DEV
function openPreviewOpenList() {
  window.location.assign('/openlist-ui/')
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
function openPreviewOpenListPlugin() {
  window.location.assign('/api/preview/plugin-openlist/')
}

function goAutomationTests() {
  router.push('/tabs/settings/devtools/automation')
}

function goWebDavTests() {
  router.push('/tabs/settings/devtools/webdav-tests')
}

// 🆕 2026-06-11：ECv4 容量边界测试入口（sparse 虚拟容器，验证 physical_used ≪ virtual_total）
function goSparseContainerTest() {
  router.push('/tabs/settings/devtools/sparse-container-test')
}

function handleVConsoleToggle(event: CustomEvent) {
  toggleVConsole(event.detail.checked)
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

<style scoped>
.prototype-cards {
  padding: 0 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.prototype-card {
  background: var(--ion-card-background, var(--ion-item-background, #fff));
  border-radius: 14px;
  padding: 14px 16px;
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.06);
  border: 1px solid rgba(var(--ion-color-medium-rgb, 128, 128, 128), 0.12);
}

.prototype-card:active {
  transform: scale(0.98);
}

.proto-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.proto-icon-wrap {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.proto-icon {
  font-size: 22px;
  color: var(--ion-text-color, #333);
}

.proto-title-area {
  flex: 1;
  min-width: 0;
}

.proto-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--ion-text-color, #333);
  margin: 0;
}

.proto-route {
  font-size: 11px;
  color: var(--encv-text-secondary, #999);
  margin: 2px 0 0;
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.proto-arrow {
  font-size: 18px;
  color: var(--ion-color-medium, #999);
  flex-shrink: 0;
}

.proto-compose-path {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 10px;
  padding: 6px 10px;
  background: rgba(var(--ion-color-medium-rgb, 128, 128, 128), 0.08);
  border-radius: 8px;
}

.path-label {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--ion-color-primary, #3880ff);
  flex-shrink: 0;
}

.path-value {
  font-size: 11px;
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
  color: var(--ion-text-color, #333);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background: none;
  padding: 0;
  margin: 0;
}

.proto-desc {
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  margin: 8px 0 0;
  line-height: 1.4;
}
</style>
