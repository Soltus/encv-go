<!--
  ServerSettings - 服务器地址配置页
  位置：/settings/server（从 AgentSettingsDetail "服务器地址" 入口跳入）
  作用：手动管理 baseUrl 兜底——自动探测链失败时用户最后的逃生通道
  提供的操作：
    - 显示当前 baseUrl + 来源（loopback / LAN / 自定义）
    - "立即探测" 按钮：调 useApiBaseProbe.probe({force: true})
    - "LAN 候选" 列表：来自 /api/network/lan-access（自动探测时拉取）
    - "手动输入" 输入框：写自定义 URL
    - "恢复默认 loopback" 按钮：清 localStorage + 重探测
  失败态：显示红色 banner + 详细错误（从 lastError 拿）
-->
<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.server.title') || '服务器地址' }}</ion-title>
        <ion-buttons slot="end">
          <ion-button
            :disabled="probe.isProbing.value"
            @click="handleProbeNow"
            :title="t('settings.server.probeNow') || '立即探测'"
          >
            <ion-spinner v-if="probe.isProbing.value" name="crescent" />
            <ion-icon v-else :icon="refreshIcon" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="serverSettingsContent">
      <!-- ① 当前 baseUrl 状态卡片 -->
      <ion-card class="statusCard" :class="{ 'statusCard_offline': !server.isOnline.value, 'statusCard_probing': probe.isProbing.value }">
        <ion-card-content>
          <div class="statusHeader">
            <ion-icon
              :icon="server.isOnline.value ? serverOnlineIcon : serverOfflineIcon"
              :class="['statusIcon', server.isOnline.value ? 'statusIcon_online' : 'statusIcon_offline']"
            />
            <div class="statusTextBlock">
              <div class="statusLabel">
                {{ server.isOnline.value
                    ? (t('settings.server.online') || '在线')
                    : (t('settings.server.offline') || '离线') }}
              </div>
              <div class="statusBaseUrl" :title="currentBaseUrl">{{ currentBaseUrl }}</div>
              <div v-if="probe.lastResult.value" class="statusMeta">
                <span class="statusSourceTag" :class="`statusSourceTag_${probe.lastResult.value.source}`">
                  {{ sourceLabel(probe.lastResult.value.source) }}
                </span>
                <span class="statusLatency">{{ probe.lastResult.value.latencyMs }}ms</span>
              </div>
            </div>
          </div>
          <!-- 错误详情：探测失败时显示 -->
          <div v-if="probe.lastError.value" class="statusError">
            <ion-icon :icon="alertCircleIcon" />
            <span>{{ probe.lastError.value }}</span>
          </div>
        </ion-card-content>
      </ion-card>

      <!-- ①.5 🆕 2026-06-15：复用 ServerStatusCard
           上一张卡片显示"我连到哪里"（baseUrl + 来源），本卡片显示"我连的是谁"（instance_id + version + transport + 状态切换动画）
           两张互补：上面管 URL 选择，下面管 backend 身份确认（防劫持） -->
      <div class="statusCardSecondary">
        <ServerStatusCard :clickable="false" :hide-instance-id="false" />
      </div>

      <!-- ② 自动探测 + 重置操作 -->
      <div class="actionRow">
        <ion-button
          expand="block"
          fill="solid"
          :disabled="probe.isProbing.value"
          @click="handleProbeNow"
        >
          <ion-icon :icon="searchIcon" slot="start" />
          {{ t('settings.server.probeNow') || '立即探测' }}
        </ion-button>
        <ion-button
          expand="block"
          fill="outline"
          @click="handleReset"
        >
          <ion-icon :icon="homeIcon" slot="start" />
          {{ t('settings.server.resetToDefault') || '恢复默认 loopback' }}
        </ion-button>
      </div>

      <!-- ③ LAN 候选列表（来自 /api/network/lan-access） -->
      <div v-if="lanCandidates.length > 0" class="lanSection">
        <h3 class="sectionTitle">
          <ion-icon :icon="globeIcon" />
          <span>{{ t('settings.server.lanCandidates') || '局域网候选' }}</span>
          <span class="sectionCount">{{ lanCandidates.length }}</span>
        </h3>
        <p class="sectionHint">
          {{ t('settings.server.lanCandidatesHint') || '来自 dev 机器的 /api/network/lan-access 端点。点击立即切换。' }}
        </p>
        <ion-list class="lanList">
          <ion-item
            v-for="(c, idx) in lanCandidates"
            :key="c"
            button
            :detail="false"
            :class="{ 'lanItem_active': currentBaseUrl === c }"
            @click="handleUseCandidate(c)"
          >
            <ion-icon :icon="idx === 0 ? starIcon : wifiIcon" slot="start" :class="idx === 0 ? 'lanIcon_preferred' : ''" />
            <ion-label>
              <div class="lanUrl">{{ c }}</div>
              <div class="lanSubLabel">
                {{ idx === 0
                    ? (t('settings.server.preferred') || '推荐')
                    : (t('settings.server.alternative') || '备选') }}
              </div>
            </ion-label>
            <ion-button
              v-if="currentBaseUrl !== c"
              slot="end"
              fill="clear"
              size="small"
              @click.stop="handleUseCandidate(c)"
            >
              {{ t('settings.server.use') || '使用' }}
            </ion-button>
            <ion-icon
              v-else
              slot="end"
              :icon="checkmarkIcon"
              class="lanCheckmark"
            />
          </ion-item>
        </ion-list>
      </div>

      <!-- ④ 手动输入 URL -->
      <div class="manualSection">
        <h3 class="sectionTitle">
          <ion-icon :icon="createIcon" />
          <span>{{ t('settings.server.manual') || '手动指定' }}</span>
        </h3>
        <p class="sectionHint">
          {{ t('settings.server.manualHint') || '如自动探测和 LAN 候选都不适用，可手动输入完整 URL（http(s)://host:port）。' }}
        </p>
        <ion-item class="manualInputRow">
          <ion-input
            v-model="manualUrl"
            :placeholder="t('settings.server.manualPlaceholder') || 'http://192.168.1.x:2025'"
            autocapitalize="off"
            autocorrect="off"
            :spellcheck="false"
            :clear-input="true"
          />
        </ion-item>
        <ion-button
          expand="block"
          fill="outline"
          :disabled="!isManualValid"
          @click="handleManualSave"
        >
          {{ t('settings.server.save') || '保存并连接' }}
        </ion-button>
        <div v-if="manualError" class="manualError">
          <ion-icon :icon="alertCircleIcon" />
          <span>{{ manualError }}</span>
        </div>
      </div>

      <!-- ⑤ 调试信息（展开可看探测日志） -->
      <details class="debugSection" v-if="probe.lastResult.value">
        <summary>{{ t('settings.server.debug') || '调试日志' }}</summary>
        <pre class="debugLog">{{ probe.lastResult.value.log.join('\n') }}</pre>
      </details>
    </ion-content>

    <!-- Toast：探测 / 切换结果反馈 -->
    <ion-toast
      :is-open="toastOpen"
      :message="toastMessage"
      :duration="2000"
      :color="toastColor"
      @did-dismiss="toastOpen = false"
    />
  </ion-page>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton, IonButton, IonIcon,
  IonContent, IonCard, IonCardContent, IonList, IonItem, IonLabel, IonInput, IonSpinner, IonToast,
} from '@ionic/vue'
import {
  refreshOutline as refreshIcon,
  searchOutline as searchIcon,
  homeOutline as homeIcon,
  globeOutline as globeIcon,
  wifiOutline as wifiIcon,
  starOutline as starIcon,
  createOutline as createIcon,
  checkmarkOutline as checkmarkIcon,
  alertCircleOutline as alertCircleIcon,
  cloudOfflineOutline as serverOfflineIcon,
  cloudDoneOutline as serverOnlineIcon,
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { useServerStatus } from '@/composables/useServerStatus'
import { useApiBaseProbe, type ProbeResult } from '@/composables/useApiBaseProbe'
import { DEFAULT_API_BASE_URL, getApiBaseUrl } from '@/api/encv'
import ServerStatusCard from '@/components/ServerStatusCard.vue'

const { t } = useI18n()
const server = useServerStatus()
const probe = useApiBaseProbe()

const manualUrl = ref('')
const manualError = ref('')
const toastOpen = ref(false)
const toastMessage = ref('')
const toastColor = ref<'success' | 'danger' | 'warning'>('success')

// 当前 baseUrl：优先从 probe.lastResult 取（最新探测结果），否则读 getApiBaseUrl()
const currentBaseUrl = computed(() => {
  return probe.lastResult.value?.baseUrl ?? getApiBaseUrl() ?? DEFAULT_API_BASE_URL
})

// LAN 候选：来自 probe.lastResult.lanAccess.addresses
// + 在地址前补 port（如果后端没返 port）
const lanCandidates = computed<string[]>(() => {
  const la = probe.lastResult.value?.lanAccess
  if (!la || la.addresses.length === 0) return []
  const port = extractPort(currentBaseUrl.value) || 2025
  return la.addresses
    .filter((a) => a && a !== '127.0.0.1' && a !== '::1' && a !== 'localhost')
    .map((a) => ensureHttpPrefix(a, port))
})

function extractPort(url: string): number {
  try {
    const u = new URL(url)
    return u.port ? parseInt(u.port, 10) : 2025
  } catch {
    return 2025
  }
}

function ensureHttpPrefix(addr: string, port: number): string {
  if (/^https?:\/\//i.test(addr)) return addr
  return `http://${addr}:${port}`
}

const isManualValid = computed(() => {
  const v = manualUrl.value.trim()
  return /^https?:\/\/[^\s/$.?#].[^\s]*$/i.test(v)
})

function sourceLabel(s: ProbeResult['source']): string {
  // 覆盖 ProbeResult['source'] 的全部 4 个 union 值 + default 兜底（TS2366）
  switch (s) {
    case 'cached': return t('settings.server.sourceCached') || '已缓存'
    case 'current-origin': return t('settings.server.sourceCurrentOrigin') || '当前页面'
    case 'loopback': return t('settings.server.sourceLoopback') || 'loopback'
    case 'lan-candidate': return t('settings.server.sourceLan') || '局域网'
    default: return s
  }
}

function showToast(msg: string, color: 'success' | 'danger' | 'warning' = 'success'): void {
  toastMessage.value = msg
  toastColor.value = color
  toastOpen.value = true
}

async function handleProbeNow(): Promise<void> {
  try {
    const result = await probe.probe({ force: true })
    if (result.baseUrl) {
      await server.manualReconnect()
      showToast(
        `${t('settings.server.probeSuccess') || '已切换'}：${result.baseUrl}（${result.latencyMs}ms）`,
        server.isOnline.value ? 'success' : 'warning'
      )
    } else {
      showToast(t('settings.server.probeFailed') || '所有候选都不可达', 'danger')
    }
  } catch (e) {
    showToast(`${t('settings.server.probeError') || '探测失败'}：${e instanceof Error ? e.message : String(e)}`, 'danger')
  }
}

async function handleReset(): Promise<void> {
  try {
    const result = await probe.resetToDefault()
    await server.manualReconnect()
    showToast(`${t('settings.server.resetSuccess') || '已恢复'}：${result.baseUrl}`)
  } catch (e) {
    showToast(`${t('settings.server.probeError') || '恢复失败'}：${e instanceof Error ? e.message : String(e)}`, 'danger')
  }
}

async function handleUseCandidate(url: string): Promise<void> {
  try {
    probe.setManual(url)
    await server.manualReconnect()
    showToast(`${t('settings.server.useSuccess') || '已切换'}：${url}`, server.isOnline.value ? 'success' : 'warning')
  } catch (e) {
    showToast(`${t('settings.server.probeError') || '切换失败'}：${e instanceof Error ? e.message : String(e)}`, 'danger')
  }
}

function handleManualSave(): void {
  manualError.value = ''
  const v = manualUrl.value.trim()
  if (!isManualValid.value) {
    manualError.value = t('settings.server.manualInvalid') || 'URL 格式不正确（需 http(s)://host:port）'
    return
  }
  try {
    probe.setManual(v)
    server.manualReconnect().then(() => {
      showToast(`${t('settings.server.useSuccess') || '已切换'}：${v}`, server.isOnline.value ? 'success' : 'warning')
    })
  } catch (e) {
    manualError.value = e instanceof Error ? e.message : String(e)
  }
}

onMounted(async () => {
  // 若没有 lastResult，主动跑一次（用 cached 节流，不强制 force）
  if (!probe.lastResult.value) {
    try {
      await probe.probe()
    } catch {/* ignore — UI 仍会显示默认 loopback */}
  }
  // 初始化 manualUrl
  manualUrl.value = currentBaseUrl.value
})
</script>

<style scoped>
.serverSettingsContent {
  --padding-start: 14px;
  --padding-end: 14px;
  --padding-top: 10px;
  --padding-bottom: 30px;
}

/* ① 状态卡片 */
.statusCard {
  margin: 0 0 16px;
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.14));
}
.statusCard_offline {
  border-color: rgba(var(--ion-color-danger-rgb), 0.4);
}
.statusCard_probing {
  opacity: 0.7;
}
.statusHeader {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.statusIcon {
  font-size: 28px;
  flex-shrink: 0;
  margin-top: 2px;
}
.statusIcon_online { color: var(--ion-color-success); }
.statusIcon_offline { color: var(--ion-color-danger); }
.statusTextBlock { flex: 1; min-width: 0; }
.statusLabel {
  font-size: 12px;
  font-weight: 500;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.statusBaseUrl {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 14px;
  color: var(--ion-text-color);
  word-break: break-all;
  margin: 4px 0;
}
.statusMeta {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 11px;
  margin-top: 4px;
}
.statusSourceTag {
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 500;
}
.statusSourceTag_cached { background: rgba(var(--ion-color-medium-rgb), 0.18); color: var(--ion-color-medium); }
.statusSourceTag_loopback { background: rgba(var(--ion-color-primary-rgb), 0.18); color: var(--ion-color-primary); }
.statusSourceTag_lan-candidate { background: rgba(var(--ion-color-success-rgb), 0.18); color: var(--ion-color-success); }
.statusLatency {
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.statusError {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-top: 10px;
  padding: 8px 10px;
  background: rgba(var(--ion-color-danger-rgb), 0.1);
  border-left: 3px solid var(--ion-color-danger);
  border-radius: 4px;
  font-size: 12px;
  color: var(--ion-color-danger);
}

/* ①.5 ServerStatusCard 容器：与上方 ion-card 对齐，间距略小以体现"副卡片"层次 */
.statusCardSecondary {
  margin: 0 0 16px;
}
.statusCardSecondary :deep(.server-status-card) {
  width: 100%;
  box-shadow: none;
}

/* ② 操作行 */
.actionRow {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 20px;
}

/* ③ LAN 候选 */
.lanSection, .manualSection { margin-bottom: 20px; }
.sectionTitle {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 0 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.85));
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.sectionTitle ion-icon { font-size: 14px; color: var(--ion-color-primary); }
.sectionCount {
  margin-left: auto;
  background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.12));
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 8px;
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}
.sectionHint {
  font-size: 11.5px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  margin: 0 0 10px;
  line-height: 1.45;
}
.lanList { background: transparent; padding: 0; }
.lanItem { --background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.06)); --border-color: var(--encv-border-color, rgba(127, 127, 127, 0.14)); margin-bottom: 6px; border-radius: 6px; }
.lanItem_active { --background: rgba(var(--ion-color-primary-rgb), 0.1); --border-color: rgba(var(--ion-color-primary-rgb), 0.3); }
.lanUrl { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12.5px; font-weight: 500; }
.lanSubLabel { font-size: 10.5px; color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7)); margin-top: 2px; }
.lanIcon_preferred { color: var(--ion-color-warning); }
.lanCheckmark { color: var(--ion-color-success); font-size: 20px; }

/* ④ 手动输入 */
.manualInputRow { --background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.06)); --border-color: var(--encv-border-color, rgba(127, 127, 127, 0.14)); margin-bottom: 10px; border-radius: 6px; }
.manualError {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-top: 8px;
  padding: 6px 10px;
  background: rgba(var(--ion-color-danger-rgb), 0.1);
  border-radius: 4px;
  font-size: 11.5px;
  color: var(--ion-color-danger);
}

/* ⑤ 调试 */
.debugSection { margin-top: 20px; }
.debugSection summary {
  font-size: 11px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  cursor: pointer;
  user-select: none;
  padding: 4px 0;
}
.debugLog {
  margin: 8px 0 0;
  padding: 8px;
  background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.06));
  border-radius: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 10.5px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.85));
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
