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
      <!-- 后端健康度摘要：让用户改 URL 前先看到当前连接状态
           此页面语义 = "URL 配置"，不是"状态详情"
           所以这里卡片不可点（避免和 ServerDetail 状态行的可点行为混淆） -->
      <ServerStatusCard :clickable="false" />

      <!-- ① 当前 baseUrl + source 摘要（ion-note 风格小 chip，提示"我连到哪里"） -->
      <div class="statusSubline">
        <code class="statusSublineUrl" :title="currentBaseUrl">{{ currentBaseUrl }}</code>
        <span v-if="probe.lastResult.value" class="statusSublineSource" :class="`statusSourceTag_${probe.lastResult.value.source}`">
          {{ sourceLabel(probe.lastResult.value.source) }}
        </span>
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
        <div class="lanList">
          <button
            v-for="addr in lanCandidates"
            :key="addr"
            class="lanItem"
            type="button"
            @click="handleUseLanAddress(addr)"
          >
            <code class="lanAddr">{{ addr }}</code>
            <ion-icon :icon="checkmarkIcon" class="lanUseIcon" />
          </button>
        </div>
      </div>

      <!-- ④ 手动输入 baseUrl -->
      <div class="manualSection">
        <h3 class="sectionTitle">
          <ion-icon :icon="createIcon" />
          <span>{{ t('settings.server.manualUrl') || '手动输入' }}</span>
        </h3>
        <p class="sectionHint">
          {{ t('settings.server.manualUrlHint') || '适用于后端不在探测范围内的场景。点击下方按钮应用。' }}
        </p>
        <div class="manualInputRow">
          <input
            v-model="manualUrl"
            type="text"
            class="manualInput"
            :placeholder="t('settings.server.manualUrlPlaceholder') || 'http://192.168.x.x:2025'"
            spellcheck="false"
            @keyup.enter="handleUseManual"
          />
          <ion-button
            fill="solid"
            :disabled="!isManualValid"
            @click="handleUseManual"
          >
            {{ t('settings.server.use') || '使用' }}
          </ion-button>
        </div>
        <p v-if="manualError" class="manualError">{{ manualError }}</p>
      </div>

      <!-- ⑤ Toast 反馈（probe 成功 / 失败 / 切换结果） -->
      <ion-toast
        :is-open="toastOpen"
        :message="toastMessage"
        :color="toastColor"
        :duration="1600"
        @didDismiss="toastOpen = false"
      />
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton, IonButton, IonIcon,
  IonContent, IonSpinner, IonToast,
} from '@ionic/vue'
import {
  refresh as refreshIcon,
  searchOutline as searchIcon,
  homeOutline as homeIcon,
  globeOutline as globeIcon,
  createOutline as createIcon,
  checkmarkOutline as checkmarkIcon,
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

async function handleUseLanAddress(addr: string): Promise<void> {
  try {
    manualUrl.value = addr
    manualError.value = ''
    probe.setManual(addr)
    server.manualReconnect().then(() => {
      showToast(`${t('settings.server.useSuccess') || '已切换'}：${addr}`, server.isOnline.value ? 'success' : 'warning')
    })
  } catch (e) {
    manualError.value = e instanceof Error ? e.message : String(e)
  }
}

async function handleUseManual(): Promise<void> {
  if (!isManualValid.value) {
    manualError.value = t('settings.server.manualUrlInvalid') || 'URL 格式无效'
    return
  }
  try {
    const v = manualUrl.value.trim()
    manualError.value = ''
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

/* 🆕 2026-06-15：详情页状态卡片 = ServerStatusCard
   - 全宽容器，间距与 ion-card 视觉一致 */
.serverSettingsContent > .server-status-card {
  margin-bottom: 8px;
}

/* ① 状态卡片下方一行：baseUrl + source tag（紧凑信息条） */
.statusSubline {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 16px;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.85));
  flex-wrap: wrap;
}
.statusSublineUrl {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  background: var(--ion-color-light);
  color: var(--ion-color-light-contrast, #000);
  padding: 2px 6px;
  border-radius: 4px;
  word-break: break-all;
}
.statusSublineSource {
  font-size: 11px;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--ion-color-medium-tint, rgba(127, 127, 127, 0.15));
  color: var(--ion-color-medium-shade, #74788c);
}
.statusSourceTag_loopback { background: var(--ion-color-primary-tint); color: var(--ion-color-primary-shade); }
.statusSourceTag_lan-candidate { background: var(--ion-color-success-tint); color: var(--ion-color-success-shade); }
.statusSourceTag_current-origin { background: var(--ion-color-warning-tint); color: var(--ion-color-warning-shade); }
.statusSourceTag_cached { background: var(--ion-color-medium-tint); color: var(--ion-color-medium-shade); }

/* ② 探测 / 重置按钮 */
.actionRow {
  display: flex;
  gap: 8px;
  margin-bottom: 18px;
}
.actionRow ion-button { flex: 1; }

/* 通用 section 标题 */
.sectionTitle {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  margin: 0 0 4px;
  color: var(--ion-text-color);
}
.sectionTitle ion-icon { font-size: 18px; }
.sectionCount {
  margin-left: auto;
  font-size: 11px;
  background: var(--ion-color-primary-tint);
  color: var(--ion-color-primary-shade);
  padding: 1px 6px;
  border-radius: 8px;
  font-weight: 500;
}
.sectionHint {
  font-size: 12px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.75));
  margin: 0 0 10px;
  line-height: 1.4;
}

/* ③ LAN 候选列表 */
.lanSection { margin-bottom: 18px; }
.lanList {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.lanItem {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.2));
  border-radius: 6px;
  background: var(--ion-background-color, #fff);
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: background-color 0.15s ease;
}
.lanItem:hover { background: var(--ion-color-light); }
.lanItem:focus-visible { outline: 2px solid var(--ion-color-primary); }
.lanAddr {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  word-break: break-all;
  color: var(--ion-text-color);
}
.lanUseIcon {
  font-size: 18px;
  color: var(--ion-color-success);
  flex-shrink: 0;
}

/* ④ 手动输入 */
.manualSection { margin-bottom: 24px; }
.manualInputRow {
  display: flex;
  gap: 8px;
  align-items: stretch;
}
.manualInput {
  flex: 1;
  font: inherit;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  padding: 8px 10px;
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.3));
  border-radius: 6px;
  background: var(--ion-background-color, #fff);
  color: var(--ion-text-color);
  outline: none;
  min-width: 0;
}
.manualInput:focus { border-color: var(--ion-color-primary); }
.manualError {
  font-size: 12px;
  color: var(--ion-color-danger);
  margin: 6px 0 0;
}
</style>
