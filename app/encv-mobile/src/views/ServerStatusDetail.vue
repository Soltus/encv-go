<!--
  ServerStatusDetail.vue — 服务器状态详情页（🆕 2026-06-15 v4 专用页）

  单一职责：**只显示后端状态的全部信息**——其它什么都不做。
  - 不管 URL 配置 → ServerUrlDetail.vue
  - 不管日志 → DevLogs.vue
  - 不管 agent / webdav / http / admin 等子服务 → 那些都在 ServerDetail.vue 列表里
  - 此页面 = 100% 专注：online? version? instance_id? port? latency? transport? 上次检查时间? 上次错误?

  用户 2026-06-15 怒批"绕了几轮找不到北"后的设计铁律：
    1. **第一眼就看出状态**——上方大色块 + 大字「在线 / 离线 / 检查中」+ 原因
    2. **下面紧接分块的「事实表」**——每一项都是 label : value 一行，不再有「为什么这里有 latency」
    3. **0 装饰 0 动画 0 旁注**——不做"等宽字体网格"也不做"dot pulse"也不做"顶部 banner"
    4. **顶部刷新按钮**——重连一次后所有事实自动更新
    5. **回退按钮**——返回 ServerDetail.vue 列表

  命名一致：XxxDetail.vue 的命名约定（MountsDetail / AppearanceDetail / EngineDetail 等），
  此页就叫 ServerStatusDetail，绝对不会跟 ServerUrlDetail / ServerDetail 混淆。
-->
<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/server"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('serverStatusDetail.title') || '服务器状态' }}</ion-title>
        <ion-buttons slot="end">
          <ion-button
            :disabled="status.isRestarting.value"
            @click="status.manualReconnect()"
            :title="t('serverStatusDetail.refresh') || '刷新'"
          >
            <ion-spinner v-if="status.isRestarting.value" name="crescent" />
            <ion-icon v-else :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="statusDetailContent">
      <!-- ① 状态大色块：第一眼就看到 -->
      <section class="statusHero" :class="`is-${state}`" role="status" :aria-label="stateText">
        <div class="statusHeroDot" :class="`is-${state}`" aria-hidden="true" />
        <div class="statusHeroText">
          <div class="statusHeroLabel">{{ stateText }}</div>
          <div v-if="reason" class="statusHeroReason">{{ reason }}</div>
          <div v-else class="statusHeroReason">{{ stateSubtitle }}</div>
        </div>
      </section>

      <!-- ② 事实表：label : value 形式，每项一行，0 解释 -->
      <section class="factTable" aria-label="服务器事实">
        <div class="factRow">
          <div class="factLabel">{{ t('serverStatusDetail.instanceId') || '实例 ID' }}</div>
          <div class="factValue factMono">{{ status.backendInstanceId.value || '—' }}</div>
        </div>
        <div class="factRow">
          <div class="factLabel">{{ t('serverStatusDetail.version') || '版本' }}</div>
          <div class="factValue factMono">{{ status.backendVersion.value || '—' }}</div>
        </div>
        <div class="factRow">
          <div class="factLabel">{{ t('serverStatusDetail.port') || '端口' }}</div>
          <div class="factValue">{{ status.backendPort.value ? `:${status.backendPort.value}` : '—' }}</div>
        </div>
        <div class="factRow">
          <div class="factLabel">{{ t('serverStatusDetail.transport') || '传输' }}</div>
          <div class="factValue">
            <span class="transportTag" :class="`transport-${status.transportMode.value}`">
              {{ transportLabel }}
            </span>
          </div>
        </div>
        <div class="factRow">
          <div class="factLabel">{{ t('serverStatusDetail.latency') || '延迟' }}</div>
          <div class="factValue">
            {{ status.latencyMs.value > 0 ? `${status.latencyMs.value} ms` : '—' }}
          </div>
        </div>
        <div class="factRow">
          <div class="factLabel">{{ t('serverStatusDetail.lastChecked') || '上次检测' }}</div>
          <div class="factValue">
            {{ status.lastCheckedAt.value ? formatLastChecked(status.lastCheckedAt.value) : '—' }}
          </div>
        </div>
        <div v-if="!status.isOnline.value && status.lastError.value" class="factRow factRow_error">
          <div class="factLabel">{{ t('serverStatusDetail.lastError') || '上次错误' }}</div>
          <div class="factValue">{{ status.lastError.value }}</div>
        </div>
        <div v-if="status.isOnline.value && status.isSandboxBrowser.value" class="factRow factRow_warning">
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
      </section>

      <!-- ③ 底部：相关跳转（避免此页变孤立） -->
      <section class="relatedActions">
        <ion-button expand="block" fill="outline" @click="goServerUrl">
          <ion-icon :icon="globeOutline" slot="start" />
          {{ t('serverStatusDetail.goServerUrl') || '管理服务器地址' }}
        </ion-button>
      </section>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
/**
 * 🆕 2026-06-15 v4：
 *   - 用户多次反馈"绕了几轮找不到北"——之前 ServerStatusCard 在 DevLogs/ServerSettings/AgentSettingsDetail 都用，混在一起
 *   - 抽到独立页面 + 路由 /tabs/settings/server/status，命名 ServerStatusDetail.vue 与 ServerSettings.vue 严格区分
 *   - 状态用大色块、其它信息用事实表 label:value 单行，0 装饰 0 动画 0 banner
 *   - 引入 useServerStatus() 直接拿所有事实（instanceId/version/port/transport/latency/lastError/lastCheckedAt/isOnline/isChecking/transportMode）
 *   - 监听 backend:instance-changed 事件：显示「后端实例已变更」事实行
 */
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton, IonButton, IonIcon,
  IonContent, IonSpinner,
} from '@ionic/vue'
import { refreshOutline, globeOutline } from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { useServerStatus } from '@/composables/useServerStatus'
import { eventBus } from '@/composables/useEventBus'

const { t } = useI18n()
const router = useRouter()
const status = useServerStatus()

// 状态计算
const state = computed<'online' | 'offline' | 'checking'>(() => {
  if (status.isRestarting.value) return 'checking'
  return status.isOnline.value ? 'online' : 'offline'
})

const stateText = computed(() => {
  switch (state.value) {
    case 'online': return t('serverStatus.online') || '在线'
    case 'offline': return t('serverStatus.offline') || '离线'
    case 'checking': return t('serverStatus.checking') || '检查中…'
  }
})

const stateSubtitle = computed(() => {
  if (state.value === 'online') return t('serverStatusDetail.allOk') || '后端正常响应'
  if (state.value === 'checking') return t('serverStatusDetail.probing') || '正在探测…'
  return t('serverStatusDetail.connectFailed') || '无法连接后端'
})

const reason = computed(() => {
  if (state.value === 'online') return ''
  if (state.value === 'checking') return ''
  if (status.lastError.value) return status.lastError.value
  if (status.transportMode.value === 'http-poll') {
    return t('serverStatus.sandboxPollingHint') || '沙箱环境使用 HTTP 轮询'
  }
  return t('serverStatus.noDetail') || '无法连接后端'
})

const transportLabel = computed(() => {
  switch (status.transportMode.value) {
    case 'ws': return 'WebSocket'
    case 'http-poll': return 'HTTP polling'
    case 'native-bridge': return 'Native bridge'
    case 'unknown': return '—'
    default: return status.transportMode.value
  }
})

// 后端实例 ID 变更（监听 eventBus 拿到 latest 一次）
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

function goServerUrl() {
  router.push('/tabs/settings/server-url')
}
</script>

<style scoped>
/* ============================================================
   ServerStatusDetail v4 — 重构到「无法混淆」
   设计：① 大色块 hero ② 事实表 ③ 跳转按钮
   0 硬编码颜色，全部 CSS variables
   ============================================================ */

.statusDetailContent {
  --padding-start: 14px;
  --padding-end: 14px;
  --padding-top: 12px;
  --padding-bottom: 30px;
}

/* ① 大色块 hero */
.statusHero {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 22px 18px;
  border-radius: 10px;
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.18));
  margin-bottom: 18px;
  background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.06));
}
.statusHero.is-online { border-color: var(--ion-color-success); border-left-width: 5px; }
.statusHero.is-offline { border-color: var(--ion-color-danger); border-left-width: 5px; }
.statusHero.is-checking { border-color: var(--ion-color-warning); border-left-width: 5px; }

.statusHeroDot {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--encv-text-secondary, rgba(127, 127, 127, 0.5));
}
.statusHeroDot.is-online { background: var(--ion-color-success); }
.statusHeroDot.is-offline { background: var(--ion-color-danger); }
.statusHeroDot.is-checking { background: var(--ion-color-warning); }

.statusHeroText { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.statusHeroLabel { font-size: 22px; font-weight: 700; line-height: 1.1; }
.statusHeroReason { font-size: 13px; color: var(--encv-text-secondary, rgba(127, 127, 127, 0.75)); line-height: 1.4; word-break: break-word; }

/* ② 事实表：label : value 形式 */
.factTable {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.14));
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 18px;
  background: var(--encv-bg-base, var(--ion-background-color, #fff));
}
.factRow {
  display: flex;
  align-items: baseline;
  gap: 12px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.08));
}
.factRow:last-child { border-bottom: none; }
.factRow_error { background: rgba(var(--ion-color-danger-rgb), 0.06); }
.factRow_warning { background: rgba(var(--ion-color-warning-rgb), 0.06); }

.factLabel {
  flex: 0 0 96px;
  font-size: 12px;
  font-weight: 600;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.85));
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.factValue {
  flex: 1;
  font-size: 13.5px;
  color: var(--ion-text-color);
  word-break: break-all;
  text-align: right;
}
.factMono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
}

.transportTag {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 4px;
  font-size: 11.5px;
  font-weight: 500;
  background: rgba(var(--ion-color-primary-rgb), 0.14);
  color: var(--ion-color-primary);
}
.transportTag.transport-websocket { background: rgba(var(--ion-color-success-rgb), 0.14); color: var(--ion-color-success); }
.transportTag.transport-http-poll { background: rgba(var(--ion-color-warning-rgb), 0.14); color: var(--ion-color-warning); }

/* ③ 跳转 */
.relatedActions { margin-top: 6px; }
</style>
