<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.serverTitle') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.connection') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="serverIcon" slot="start"></ion-icon>
          <ion-label class="ion-text-wrap">
            <h3>{{ t('settings.serverUrl') }}</h3>
            <p class="readonly-url" @click="copyToClipboard(serverUrl)">{{ serverUrl }}</p>
          </ion-label>
          <ion-button slot="end" fill="clear" size="small" @click="copyToClipboard(serverUrl)">
            <ion-icon :icon="copyIcon" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-item>
        <!-- 🆕 2026-06-15 v6：状态 ion-item → 内联「精美卡片」hero + 事实表
             用户怒批"ion-item 看不见在哪吗"——之前这行只是 badge + 18b967b8 + vdev
             现在这行 = ServerStatusDetail.vue 整页内容内联：
               ① 大色块 hero（在线/离线/检查中 + 原因）
               ② 事实表：instance_id / version / port / transport / latency / last_checked / last_error / sandbox / backend_changed
               ③ 右侧按钮：refresh / stop / restart
             不再跳 /settings/server/status 单独页（删路由）；用户要的是这一行本身就是"详情页" -->
        <section class="statusFactCard" :class="`is-${stateClass}`" aria-label="服务器状态">
          <div class="statusHeroRow">
            <span class="statusHeroDot" :class="`is-${stateClass}`" aria-hidden="true" />
            <div class="statusHeroText">
              <div class="statusHeroLabel">{{ stateText }}</div>
              <div class="statusHeroReason">{{ stateReason }}</div>
            </div>
            <div class="server-controls">
              <ion-button fill="outline" size="small" @click="checkServerInner">
                <ion-icon :icon="refreshIcon" slot="icon-only"></ion-icon>
              </ion-button>
              <ion-button v-if="isRestarting" fill="outline" size="small" color="medium" disabled>
                <ion-spinner slot="icon-only" name="crescent"></ion-spinner>
              </ion-button>
              <ion-button v-else-if="serverOnline" fill="outline" size="small" color="danger" @click="handleStop" :disabled="isStopping">
                <ion-spinner v-if="isStopping" slot="icon-only" name="crescent"></ion-spinner>
                <ion-icon v-else :icon="stopIcon" slot="icon-only"></ion-icon>
              </ion-button>
              <ion-button v-else fill="outline" size="small" color="warning" @click="handleRestart">
                <ion-icon :icon="playIcon" slot="icon-only"></ion-icon>
              </ion-button>
            </div>
          </div>

          <div class="factTable">
            <div class="factRow">
              <div class="factLabel">{{ t('serverStatusDetail.instanceId') || '实例 ID' }}</div>
              <div class="factValue factMono">{{ backendInstanceId || '—' }}</div>
            </div>
            <div class="factRow">
              <div class="factLabel">{{ t('serverStatusDetail.version') || '版本' }}</div>
              <div class="factValue factMono">{{ backendVersion || '—' }}</div>
            </div>
            <div class="factRow">
              <div class="factLabel">{{ t('serverStatusDetail.port') || '端口' }}</div>
              <div class="factValue">{{ backendPort ? `:${backendPort}` : '—' }}</div>
            </div>
            <div class="factRow">
              <div class="factLabel">{{ t('serverStatusDetail.transport') || '传输' }}</div>
              <div class="factValue">
                <span class="transportTag" :class="`transport-${transportMode}`">
                  {{ transportLabel }}
                </span>
              </div>
            </div>
            <div class="factRow">
              <div class="factLabel">{{ t('serverStatusDetail.latency') || '延迟' }}</div>
              <div class="factValue">{{ latencyMs > 0 ? `${latencyMs} ms` : '—' }}</div>
            </div>
            <div class="factRow">
              <div class="factLabel">{{ t('serverStatusDetail.lastChecked') || '上次检测' }}</div>
              <div class="factValue">{{ lastCheckedAt ? formatLastChecked(lastCheckedAt) : '—' }}</div>
            </div>
            <div v-if="!serverOnline && connectionError" class="factRow factRow_error">
              <div class="factLabel">{{ t('serverStatusDetail.lastError') || '上次错误' }}</div>
              <div class="factValue">{{ connectionError }}</div>
            </div>
            <div v-if="serverOnline && isSandboxBrowser" class="factRow factRow_warning">
              <div class="factLabel">{{ t('serverStatusDetail.sandboxNote') || '沙箱提示' }}</div>
              <div class="factValue">
                {{ t('serverStatus.sandboxPollingHint') || '当前为沙箱浏览器，WebSocket 已降级为 HTTP 轮询' }}
              </div>
            </div>
            <div v-if="instanceChanged" class="factRow factRow_warning">
              <div class="factLabel">{{ t('serverStatusDetail.backendChanged') || '后端变更' }}</div>
              <div class="factValue">
                {{ t('serverStatusDetail.backendChangedHint', { prev: instanceChanged.previous, curr: instanceChanged.current }) ||
                   `实例 ID 已变更 ${instanceChanged.previous} → ${instanceChanged.current}` }}
              </div>
            </div>
          </div>
        </section>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.serviceAddresses') }}</ion-label>
        </ion-list-header>
        <ion-item button @click="goHttpServer" detail>
          <ion-icon :icon="cloudOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.httpServer') }}</h3>
            <p>:{{ httpPort }} {{ rootDir }}</p>
          </ion-label>
        </ion-item>

        <ion-item button @click="goAdminServer" detail>
          <ion-icon :icon="shieldCheckmark" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.adminServer') }}</h3>
            <p>{{ adminConfigured ? t('settings.configured') : t('settings.notConfigured') }}</p>
          </ion-label>
        </ion-item>

        <ion-item button @click="goWebdavServer" detail>
          <ion-icon :icon="globeOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.webdavServer') }}</h3>
            <p>{{ webdavRoot }}{{ webdavUsername ? ' @' + webdavUsername : '' }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list v-if="isNativePlatform">
        <ion-list-header>
          <ion-label>{{ t('settings.permissions') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="notificationsIcon" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.notificationPermission') }}</h3>
            <p>{{ permNotifications ? t('settings.granted') : t('settings.denied') }}</p>
          </ion-label>
          <ion-button v-if="!permNotifications" fill="outline" size="small" @click="handleRequestNotification">
            {{ t('settings.request') }}
          </ion-button>
        </ion-item>
        <ion-item>
          <ion-icon :icon="folderOpen" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.storagePermission') }}</h3>
            <p>{{ permStorage ? t('settings.granted') : t('settings.denied') }}</p>
          </ion-label>
          <ion-button v-if="!permStorage" fill="outline" size="small" @click="handleRequestStorage">
            {{ t('settings.request') }}
          </ion-button>
        </ion-item>
        <ion-item>
          <ion-icon :icon="batteryOptimizationIcon" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.batteryOptimization') }}</h3>
            <p>{{ permBatteryOpt ? t('settings.granted') : t('settings.denied') }}</p>
          </ion-label>
          <ion-button v-if="!permBatteryOpt" fill="outline" size="small" @click="handleRequestBatteryOpt">
            {{ t('settings.request') }}
          </ion-button>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonContent, IonList, IonListHeader, IonItem, IonIcon, IonLabel,
  IonButton, alertController, IonSpinner,
} from '@ionic/vue'
import {
  server as serverIcon, refresh as refreshIcon,
  stop as stopIcon, play as playIcon,
  notifications as notificationsIcon, folderOpen,
  copy as copyIcon, shieldCheckmark, cloudOutline, globeOutline,
  batteryCharging as batteryOptimizationIcon,
} from 'ionicons/icons'
import { useServerStatus } from '@/composables/useServerStatus'
import { useI18n } from '@/composables/useI18n'
import { eventBus } from '@/composables/useEventBus'
import { showToast } from '@/composables/useToast'
import { copyToClipboard as clipboardWrite } from '@/composables/useClipboard'
import { getServerUrl, fetchConfig } from '@/api/encv'
import { isNative, requestNotificationPermission, requestStoragePermission, requestBatteryOptimization, checkPermissions } from '@/plugins/GoProcess'

const configData = ref<Record<string, unknown> | null>(null)
const {
  isOnline: serverOnline,
  lastError: connectionError,
  checkStatus,
  restartBackend,
  stopBackend,
  backendPort,
  isRestarting,
  isStopping,
  // 🆕 2026-06-15 v6：状态区"精美卡片"内联所有事实
  //   不再跳 /settings/server/status 单独页（路由删除）
  //   instanceId / version / port / transport / latency / lastCheckedAt / sandbox / backendChanged
  backendInstanceId,
  backendVersion,
  transportMode,
  latencyMs,
  lastCheckedAt,
  isSandboxBrowser,
} = useServerStatus()
const { t } = useI18n()

const serverUrl = ref(getServerUrl())
const isNativePlatform = ref(isNative())
const permNotifications = ref(false)
const permStorage = ref(false)
const permBatteryOpt = ref(false)
let permissionCheckTimer: number | null = null

const router = useRouter()

const httpPort = computed(() => (configData.value?.server as Record<string, unknown>)?.port ?? '-')
const rootDir = computed(() => (configData.value?.server as Record<string, unknown>)?.dir ?? '/')
const adminConfigured = computed(() => !!(configData.value?.admin as Record<string, unknown>)?.password)
const webdavRoot = computed(() => {
  const val = (configData.value?.webdav as Record<string, unknown>)?.root
  return typeof val === 'string' ? val : '/'
})
const webdavUsername = computed(() => (configData.value?.webdav as Record<string, unknown>)?.username ?? '')

// 🆕 2026-06-15 v6：状态"精美卡片"内联 —— 状态机/文本/原因/transport label
const stateClass = computed<'online' | 'offline' | 'checking'>(() => {
  if (isRestarting.value) return 'checking'
  return serverOnline.value ? 'online' : 'offline'
})

const stateText = computed(() => {
  switch (stateClass.value) {
    case 'online': return t('serverStatus.online') || '在线'
    case 'offline': return t('serverStatus.offline') || '离线'
    case 'checking': return t('serverStatus.checking') || '检查中…'
  }
})

const stateReason = computed(() => {
  if (stateClass.value === 'online') return t('serverStatusDetail.allOk') || '后端正常响应'
  if (stateClass.value === 'checking') return t('serverStatusDetail.probing') || '正在探测…'
  if (connectionError.value) return connectionError.value
  if (transportMode.value === 'http-poll') {
    return t('serverStatus.sandboxPollingHint') || '沙箱环境使用 HTTP 轮询'
  }
  return t('serverStatus.noDetail') || '无法连接后端'
})

const transportLabel = computed(() => {
  switch (transportMode.value) {
    case 'ws': return 'WebSocket'
    case 'http-poll': return 'HTTP polling'
    case 'native-bridge': return 'Native bridge'
    case 'unknown': return '—'
    default: return transportMode.value
  }
})

// 后端实例 ID 变更（eventBus）
const instanceChanged = ref<{ previous: string; current: string } | null>(null)
function onBackendInstanceChanged(payload: { previous: string; current: string }) {
  instanceChanged.value = payload
}
onMounted(() => eventBus.on('backend:instance-changed', onBackendInstanceChanged))
onUnmounted(() => eventBus.off('backend:instance-changed', onBackendInstanceChanged))

function formatLastChecked(d: Date | string): string {
  const dt = typeof d === 'string' ? new Date(d) : d
  if (Number.isNaN(dt.getTime())) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(dt.getHours())}:${pad(dt.getMinutes())}:${pad(dt.getSeconds())}`
}

function goHttpServer() { router.push('/tabs/settings/server/http') }
function goAdminServer() { router.push('/tabs/settings/server/admin') }
function goWebdavServer() { router.push('/tabs/settings/server/webdav') }

async function copyToClipboard(text: string) {
  const ok = await clipboardWrite(text)
  showToast({ message: ok ? t('remote.copied') : t('devlogs.copyFailed'), duration: 1000, color: ok ? 'success' : 'danger' })
}

async function refreshPermissions() {
  const perms = await checkPermissions()
  permNotifications.value = perms.notifications
  permStorage.value = perms.storage
  permBatteryOpt.value = perms.batteryOptimization
}

async function handleRequestNotification() {
  await requestNotificationPermission()
  if (permissionCheckTimer) clearTimeout(permissionCheckTimer)
  permissionCheckTimer = window.setTimeout(() => refreshPermissions(), 1000)
  setTimeout(() => refreshPermissions(), 3000)
  setTimeout(() => refreshPermissions(), 5000)
}

async function handleRequestStorage() {
  await requestStoragePermission()
  if (permissionCheckTimer) clearTimeout(permissionCheckTimer)
  permissionCheckTimer = window.setTimeout(() => refreshPermissions(), 1000)
  setTimeout(() => refreshPermissions(), 3000)
  setTimeout(() => refreshPermissions(), 5000)
}

async function handleRequestBatteryOpt() {
  await requestBatteryOptimization()
  if (permissionCheckTimer) clearTimeout(permissionCheckTimer)
  permissionCheckTimer = window.setTimeout(() => refreshPermissions(), 1000)
  setTimeout(() => refreshPermissions(), 3000)
  setTimeout(() => refreshPermissions(), 5000)
}

async function checkServerInner() {
  // 刷新按钮：只 ping 一次后端 + 弹 toast，不跳详情页
  // （防 ion-item 冒泡用 .stop，但本身也独立可用 — 跟 goServerStatusDetail 路径严格区分）
  await checkStatus()
  showToast({
    message: serverOnline.value ? t('settings.serverOnline') : t('settings.serverOffline'),
    duration: 1500,
    color: serverOnline.value ? 'success' : 'danger',
  })
}

async function handleRestart() {
  showToast({
    message: t('settings.restarting'),
    duration: 30000,
  })
  const success = await restartBackend()
  showToast({
    message: success ? t('settings.restartSuccess') : t('settings.restartFailed'),
    duration: 2000,
    color: success ? 'success' : 'danger',
  })
}

async function handleStop() {
  const alert = await alertController.create({
    header: t('settings.stopConfirm'),
    buttons: [
      { text: t('settings.cancel'), role: 'cancel' },
      {
        text: t('settings.stop'),
        role: 'destructive',
        handler: async () => {
          const success = await stopBackend()
          showToast({
            message: success ? t('settings.stopped') : t('settings.stopFailed'),
            duration: 2000,
            color: success ? 'success' : 'danger',
          })
        },
      },
    ],
  })
  await alert.present()
}

onMounted(async () => {
  if (isNativePlatform.value) {
    await refreshPermissions()
  }
  if (serverOnline.value) {
    try {
      configData.value = await fetchConfig()
    } catch {}
  }
})
</script>

<style scoped>
/* ============================================================
   🆕 2026-06-15 v6：状态区"精美卡片"内联
   替代原本 ion-item 行（badge + instance_id + version）
   现在 = 大色块 hero + 事实表 label:value 单行
   ============================================================ */
.statusFactCard {
  display: block;
  margin: 8px 14px 16px;
  padding: 0;
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.18));
  border-left-width: 5px;
  border-radius: 8px;
  background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.04));
  overflow: hidden;
}
.statusFactCard.is-online { border-left-color: var(--ion-color-success, #2dd55b); }
.statusFactCard.is-offline { border-left-color: var(--ion-color-danger, #eb445a); }
.statusFactCard.is-checking { border-left-color: var(--ion-color-warning, #ffc409); }

.statusHeroRow {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 14px 12px;
}
.statusHeroDot {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--ion-color-medium);
}
.statusHeroDot.is-online { background: var(--ion-color-success, #2dd55b); }
.statusHeroDot.is-offline { background: var(--ion-color-danger, #eb445a); }
.statusHeroDot.is-checking { background: var(--ion-color-warning, #ffc409); opacity: 0.7; }

.statusHeroText { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.statusHeroLabel { font-size: 18px; font-weight: 700; line-height: 1.15; }
.statusHeroReason { font-size: 12px; color: var(--encv-text-secondary, rgba(127, 127, 127, 0.75)); line-height: 1.35; word-break: break-word; }

.server-controls { display: flex; gap: 4px; flex-shrink: 0; }

.factTable {
  display: flex;
  flex-direction: column;
  border-top: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.12));
  background: var(--ion-background-color, #fff);
}
.factRow {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 8px 14px;
  border-bottom: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.06));
}
.factRow:last-child { border-bottom: none; }
.factRow_error { background: rgba(var(--ion-color-danger-rgb), 0.06); }
.factRow_warning { background: rgba(var(--ion-color-warning-rgb), 0.06); }

.factLabel {
  flex: 0 0 88px;
  font-size: 11px;
  font-weight: 600;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.85));
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.factValue {
  flex: 1;
  font-size: 13px;
  color: var(--ion-text-color);
  word-break: break-all;
  text-align: right;
}
.factMono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

.transportTag {
  display: inline-block;
  padding: 1px 7px;
  border-radius: 4px;
  font-size: 11.5px;
  font-weight: 500;
  background: rgba(var(--ion-color-primary-rgb), 0.14);
  color: var(--ion-color-primary);
}
.transportTag.transport-ws { background: rgba(var(--ion-color-success-rgb), 0.14); color: var(--ion-color-success); }
.transportTag.transport-http-poll { background: rgba(var(--ion-color-warning-rgb), 0.14); color: var(--ion-color-warning); }

.connection-error {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin-top: 4px;
}
.port-info {
  font-size: 12px;
  opacity: 0.7;
  margin-left: 6px;
}
.latency-info {
  font-size: 12px;
  opacity: 0.7;
  margin-left: 2px;
  color: var(--ion-color-primary-shade);
}
.transport-info {
  font-size: 12px;
  margin-left: 2px;
  font-weight: 500;
}
.transport-ws {
  color: var(--ion-color-success-shade);
}
.transport-http-poll {
  color: var(--ion-color-warning-shade);
}
.transport-native-bridge {
  color: var(--ion-color-tertiary-shade);
}
.status-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
}
.status-meta {
  font-size: 11px;
  color: var(--ion-color-medium);
  margin-top: 4px;
}
.status-warning {
  font-size: 11px;
  color: var(--ion-color-warning-shade);
  background: var(--ion-color-warning-tint);
  padding: 4px 8px;
  border-radius: 4px;
  margin-top: 6px;
  line-height: 1.4;
}
.server-controls {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}
.readonly-url {
  font-family: 'Courier New', Courier, monospace;
  font-size: 13px;
  color: var(--ion-color-primary);
  word-break: break-all;
  cursor: pointer;
  user-select: all;
}
</style>
