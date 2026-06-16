<!--
  ServerStatusDetail.vue — 后端状态详情页

  这个页面的存在意义 = 展示 ServerStatusCard 装不下的信息：
    ✅ 延迟趋势图（最近 60 次探测，SVG 折线图）
    ✅ 状态切换时间线（最近 10 次 online ↔ offline）
    ✅ 操作历史（最近 30 条 check / start / stop / restart / ping）
    ✅ 网络诊断（主动 ping baseUrl 测 RTT，裸 ping 不改 isOnline）
    ✅ 应用元信息（App 启动时间 / 平台 / User-Agent / apiBaseUrl）

  跟 ServerStatusCard 的边界：
    ❌ 不再列 fact table（version / port / instanceId / transport / latency）
       这些在卡片正面 + 反面已经有，这里重复展示无价值
    ❌ 不再有大色块 hero
       卡片的状态点 + label 已是 0→1 摘要，详情页承接"1→N"展开

  使用：/tabs/settings/server/status（从 ServerStatusCard 点击进入）
-->
<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button :default-href="'/tabs/settings/server'" />
        </ion-buttons>
        <ion-title>{{ t('serverStatusDetail.title') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="onRefresh">
            <ion-icon :icon="refreshIcon" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="statusDetailContent">
      <!-- ============ ① 当前状态摘要（迷你版 ServerStatusCard）============ -->
      <section class="stateSummary" :class="`is-${state}`">
        <span class="stateDot" :class="`is-${state}`" />
        <div class="stateText">
          <div class="stateLabel">{{ stateLabel }}</div>
          <div class="stateSub">{{ stateSubText }}</div>
        </div>
        <div class="stateQuickFacts">
          <span class="quickFact">
            <ion-icon :icon="timerIcon" />
            {{ latencyText }}
          </span>
          <span class="quickFact" v-if="transport">
            <ion-icon :icon="wifiIcon" />
            {{ transport }}
          </span>
        </div>
      </section>

      <!-- ============ ② 延迟趋势图（SVG，0 依赖）============ -->
      <section class="detailSection">
        <div class="sectionHeader">
          <ion-icon :icon="trendingUpIcon" />
          <span class="sectionTitle">{{ t('serverStatusDetail.latencyTrend') }}</span>
          <span class="sectionMeta">{{ latencyHistory.length }} {{ t('serverStatusDetail.samples') }}</span>
        </div>
        <div class="trendChart">
          <svg
            v-if="latencyHistory.length > 0"
            class="trendSvg"
            :viewBox="`0 0 ${trendWidth} ${trendHeight}`"
            preserveAspectRatio="none"
            aria-label="latency trend"
          >
            <!-- 网格 -->
            <g class="trendGrid">
              <line v-for="y in 4" :key="`g${y}`" :x1="0" :x2="trendWidth" :y1="(trendHeight * y) / 4" :y2="(trendHeight * y) / 4" />
            </g>
            <!-- 折线 -->
            <polyline
              v-if="trendPoints"
              class="trendLine"
              :points="trendPoints"
            />
            <!-- 数据点 -->
            <circle
              v-for="(p, i) in trendDataPoints"
              :key="`p${i}`"
              :cx="p.x"
              :cy="p.y"
              r="2"
              class="trendDot"
            />
          </svg>
          <div v-else class="trendEmpty">
            <ion-icon :icon="hourglassIcon" />
            <span>{{ t('serverStatusDetail.noData') }}</span>
          </div>
        </div>
        <div v-if="latencyHistory.length > 0" class="trendLegend">
          <span class="legendItem">
            <span class="legendSwatch legendSwatchMin" />
            {{ minLatency }}ms
          </span>
          <span class="legendItem">
            <span class="legendSwatch legendSwatchAvg" />
            {{ avgLatency }}ms
          </span>
          <span class="legendItem">
            <span class="legendSwatch legendSwatchMax" />
            {{ maxLatency }}ms
          </span>
        </div>
      </section>

      <!-- ============ ③ 状态切换时间线 ============ -->
      <section class="detailSection">
        <div class="sectionHeader">
          <ion-icon :icon="timeIcon" />
          <span class="sectionTitle">{{ t('serverStatusDetail.timeline') }}</span>
          <span class="sectionMeta">{{ stateHistory.length }} {{ t('serverStatusDetail.records') }}</span>
        </div>
        <div v-if="stateHistory.length === 0" class="emptyState">
          <ion-icon :icon="hourglassIcon" />
          <span>{{ t('serverStatusDetail.noData') }}</span>
        </div>
        <ul v-else class="timeline">
          <li
            v-for="(item, i) in stateHistory"
            :key="item.at + '-' + i"
            class="timelineItem"
            :class="`is-${item.state}`"
          >
            <span class="timelineDot" :class="`is-${item.state}`" />
            <div class="timelineBody">
              <div class="timelineRow">
                <span class="timelineState">{{ t(`serverStatus.${item.state}`) }}</span>
                <span class="timelineTime">{{ formatTime(item.at) }}</span>
              </div>
              <div v-if="item.reason" class="timelineReason">{{ item.reason }}</div>
            </div>
          </li>
        </ul>
      </section>

      <!-- ============ ④ 操作历史 ============ -->
      <section class="detailSection">
        <div class="sectionHeader">
          <ion-icon :icon="listIcon" />
          <span class="sectionTitle">{{ t('serverStatusDetail.actions') }}</span>
          <span class="sectionMeta">{{ actionHistory.length }} {{ t('serverStatusDetail.records') }}</span>
        </div>
        <div v-if="actionHistory.length === 0" class="emptyState">
          <ion-icon :icon="hourglassIcon" />
          <span>{{ t('serverStatusDetail.noData') }}</span>
        </div>
        <ul v-else class="actionLog">
          <li
            v-for="(item, i) in actionHistory"
            :key="item.at + '-' + i"
            class="actionLogItem"
            :class="item.success ? 'is-success' : 'is-failed'"
          >
            <ion-icon
              :icon="actionIcon(item.action)"
              class="actionLogIcon"
              :class="item.success ? 'is-success' : 'is-failed'"
            />
            <div class="actionLogBody">
              <div class="actionLogRow">
                <span class="actionLogName">{{ t(`serverStatusDetail.action.${item.action}`) }}</span>
                <span class="actionLogTime">{{ formatTime(item.at) }}</span>
              </div>
              <div v-if="item.detail" class="actionLogDetail">{{ item.detail }}</div>
            </div>
          </li>
        </ul>
      </section>

      <!-- ============ ⑤ 网络诊断 ============ -->
      <section class="detailSection">
        <div class="sectionHeader">
          <ion-icon :icon="networkIcon" />
          <span class="sectionTitle">{{ t('serverStatusDetail.networkDiag') }}</span>
        </div>
        <div class="networkBox">
          <div class="networkRow">
            <div class="networkLabel">{{ t('serverStatusDetail.apiBaseUrl') }}</div>
            <div class="networkValue monospace">{{ metrics.apiBaseUrl || '—' }}</div>
          </div>
          <ion-button
            fill="outline"
            size="default"
            expand="block"
            :disabled="pinging"
            @click="onPing"
          >
            <ion-spinner v-if="pinging" slot="start" name="crescent" />
            <ion-icon v-else :icon="pulseIcon" slot="start" />
            {{ pinging ? t('serverStatusDetail.pinging') : t('serverStatusDetail.pingTest') }}
          </ion-button>
          <div v-if="lastPing" class="pingResult" :class="lastPing.ok ? 'is-success' : 'is-failed'">
            <ion-icon
              :icon="lastPing.ok ? checkmarkIcon : closeIcon"
              class="pingResultIcon"
            />
            <div class="pingResultText">
              <div class="pingResultTitle">
                {{ lastPing.ok ? t('serverStatusDetail.pingOk') : t('serverStatusDetail.pingFailed') }}
                <span class="pingResultMs">{{ lastPing.ms }}ms</span>
              </div>
              <div v-if="lastPing.error" class="pingResultError">{{ lastPing.error }}</div>
            </div>
          </div>
        </div>
      </section>

      <!-- ============ ⑥ 应用元信息 ============ -->
      <section class="detailSection">
        <div class="sectionHeader">
          <ion-icon :icon="phonePortraitIcon" />
          <span class="sectionTitle">{{ t('serverStatusDetail.appInfo') }}</span>
        </div>
        <div class="metaGrid">
          <div class="metaCell">
            <div class="metaLabel">{{ t('serverStatusDetail.appUptime') }}</div>
            <div class="metaValue">{{ appUptime }}</div>
          </div>
          <div class="metaCell">
            <div class="metaLabel">{{ t('serverStatusDetail.appPlatform') }}</div>
            <div class="metaValue">{{ metrics.platform }}</div>
          </div>
          <div class="metaCell metaCellFull">
            <div class="metaLabel">{{ t('serverStatusDetail.appNative') }}</div>
            <div class="metaValue">
              <span class="badge" :class="metrics.isNative ? 'is-yes' : 'is-no'">
                {{ metrics.isNative ? t('serverStatusDetail.yes') : t('serverStatusDetail.no') }}
              </span>
              <span v-if="metrics.isSandboxBrowser" class="badge is-sandbox">
                {{ t('serverStatusDetail.sandboxBrowser') }}
              </span>
            </div>
          </div>
          <div class="metaCell metaCellFull">
            <div class="metaLabel">{{ t('serverStatusDetail.appUA') }}</div>
            <div class="metaValue monospace small">{{ shortUA }}</div>
          </div>
        </div>
      </section>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from '@/composables/useI18n'
import { useServerStatus } from '@/composables/useServerStatus'
import {
  refreshOutline as refreshIcon,
  timerOutline as timerIcon,
  wifiOutline as wifiIcon,
  trendingUpOutline as trendingUpIcon,
  hourglassOutline as hourglassIcon,
  timeOutline as timeIcon,
  listOutline as listIcon,
  gitNetworkOutline as networkIcon,
  pulseOutline as pulseIcon,
  checkmarkCircleOutline as checkmarkIcon,
  closeCircleOutline as closeIcon,
  phonePortraitOutline as phonePortraitIcon,
  cloudUploadOutline as checkActionIcon,
  refreshCircleOutline as restartActionIcon,
  stopCircleOutline as stopActionIcon,
  playCircleOutline as playActionIcon,
  linkOutline as reconnectActionIcon,
} from 'ionicons/icons'

const { t } = useI18n()
const {
  isOnline,
  latencyMs,
  transportMode,
  lastCheckedAt,
  backendInstanceId,
  backendVersion,
  checkStatus,
  latencyHistory,
  stateHistory,
  actionHistory,
  metrics,
  recordAction,
  networkPing,
} = useServerStatus()

// —— 状态摘要 ——
const state = computed<'online' | 'offline' | 'checking'>(() => {
  if (isChecking.value) return 'checking'
  return isOnline.value ? 'online' : 'offline'
})
const isChecking = ref(false)
const stateLabel = computed(() => {
  switch (state.value) {
    case 'online': return t('serverStatus.online')
    case 'offline': return t('serverStatus.offline')
    case 'checking': return t('serverStatus.checking')
  }
})
const stateSubText = computed(() => {
  if (state.value === 'online' && backendVersion.value) {
    return `v${backendVersion.value} · ${shortId(backendInstanceId.value)}`
  }
  if (state.value === 'offline') {
    return lastCheckedAt.value
      ? `${t('serverStatus.lastCheck')}: ${formatTime(lastCheckedAt.value.getTime())}`
      : t('serverStatus.backendOffline')
  }
  return ''
})
function shortId(id: string) {
  return id ? id.slice(0, 8) : '—'
}

// —— latency / transport 摘要 ——
const latencyText = computed(() => {
  if (latencyMs.value <= 0) return '—'
  if (latencyMs.value < 1000) return `${latencyMs.value}ms`
  return `${(latencyMs.value / 1000).toFixed(2)}s`
})
const transport = computed(() => {
  const m = transportMode.value
  return m && m !== 'unknown' ? m.toUpperCase() : ''
})

// —— 趋势图数据 ——
const trendWidth = 320
const trendHeight = 80
const minLatency = computed(() => latencyHistory.value.length > 0 ? Math.min(...latencyHistory.value.map(p => p.ms)) : 0)
const maxLatency = computed(() => latencyHistory.value.length > 0 ? Math.max(...latencyHistory.value.map(p => p.ms)) : 0)
const avgLatency = computed(() => {
  if (latencyHistory.value.length === 0) return 0
  return Math.round(latencyHistory.value.reduce((sum, p) => sum + p.ms, 0) / latencyHistory.value.length)
})
const trendPoints = computed(() => {
  if (latencyHistory.value.length < 2) return ''
  const arr = latencyHistory.value
  const max = Math.max(maxLatency.value, 1)
  const stepX = trendWidth / (arr.length - 1)
  return arr.map((p, i) => {
    const x = i * stepX
    // y 0 在顶部 = 趋势图底部，对应 max 延迟
    const y = trendHeight - (p.ms / max) * (trendHeight - 4) - 2
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
})
const trendDataPoints = computed(() => {
  if (latencyHistory.value.length < 2) return []
  const arr = latencyHistory.value
  const max = Math.max(maxLatency.value, 1)
  const stepX = trendWidth / (arr.length - 1)
  return arr.map((p, i) => {
    const x = i * stepX
    const y = trendHeight - (p.ms / max) * (trendHeight - 4) - 2
    return { x, y }
  })
})

// —— 网络 ping ——
const pinging = ref(false)
const lastPing = ref<{ ok: boolean; ms: number; error?: string } | null>(null)
async function onPing() {
  pinging.value = true
  try {
    lastPing.value = await networkPing()
  } finally {
    pinging.value = false
  }
}

// —— 操作 history 图标映射 ——
function actionIcon(action: string) {
  switch (action) {
    case 'check': return checkActionIcon
    case 'restart': return restartActionIcon
    case 'stop': return stopActionIcon
    case 'start': return playActionIcon
    case 'reconnect': return reconnectActionIcon
    case 'ping': return pulseIcon
    default: return checkActionIcon
  }
}

// —— 时间格式 ——
function formatTime(ts: number) {
  const d = new Date(ts)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

// —— App uptime ——
const now = ref(Date.now())
setInterval(() => { now.value = Date.now() }, 1000)
const appUptime = computed(() => {
  const s = Math.floor((now.value - metrics.value.appStartTime) / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${sec}s`
  return `${sec}s`
})
const shortUA = computed(() => {
  const ua = metrics.value.userAgent
  if (ua.length <= 80) return ua
  return ua.slice(0, 80) + '…'
})

// —— refresh ——
async function onRefresh() {
  isChecking.value = true
  recordAction('check', true, t('serverStatusDetail.manualTrigger'))
  try {
    await checkStatus()
  } finally {
    isChecking.value = false
  }
}

onMounted(() => {
  // 进来时跑一次 check（确保 history 有点）
  if (latencyHistory.value.length === 0) {
    onRefresh()
  }
})
</script>

<style scoped>
/* ============================================================
   ServerStatusDetail — 详情页（卡片装不下的信息）
   100% CSS variables — 0 硬编码颜色
   ============================================================ */
.statusDetailContent {
  --background: var(--ion-background-color, #fff);
}

/* ============ 状态摘要条（迷你卡片） ============ */
.stateSummary {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 14px 14px 6px;
  padding: 12px 14px;
  background: var(--ion-background-color, #fff);
  border: 1px solid var(--ion-color-medium-shade, #747484);
  border-left-width: 4px;
  border-radius: 12px;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.4);
}
.stateSummary.is-online { border-left-color: var(--ion-color-success, #2dd55b); }
.stateSummary.is-offline { border-left-color: var(--ion-color-danger, #eb445a); }
.stateSummary.is-checking { border-left-color: var(--ion-color-warning, #ffc409); }
.stateDot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  flex-shrink: 0;
}
.stateDot.is-online { background: var(--ion-color-success, #2dd55b); box-shadow: 0 0 8px color-mix(in srgb, var(--ion-color-success, #2dd55b) 60%, transparent); }
.stateDot.is-offline { background: var(--ion-color-danger, #eb445a); }
.stateDot.is-checking { background: var(--ion-color-warning, #ffc409); }
.stateText { flex: 1; min-width: 0; }
.stateLabel { font-size: 14px; font-weight: 600; color: var(--ion-text-color, #000); }
.stateSub { font-size: 11px; color: color-mix(in srgb, var(--ion-text-color, #000) 55%, transparent); margin-top: 1px; }
.stateQuickFacts { display: flex; gap: 6px; flex-shrink: 0; }
.quickFact {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  font-weight: 600;
  color: var(--ion-color-primary, #3880ff);
  background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 12%, transparent);
  padding: 2px 7px;
  border-radius: 999px;
}
.quickFact ion-icon { font-size: 11px; width: 11px; height: 11px; }

/* ============ section 通用 ============ */
.detailSection {
  margin: 14px;
  background: var(--ion-background-color, #fff);
  border: 1px solid color-mix(in srgb, var(--ion-color-medium, #92949c) 40%, transparent);
  border-radius: 12px;
  overflow: hidden;
}
.sectionHeader {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 6%, transparent);
  border-bottom: 1px solid color-mix(in srgb, var(--ion-color-medium, #92949c) 30%, transparent);
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-text-color, #000);
}
.sectionHeader ion-icon {
  font-size: 16px;
  color: var(--ion-color-primary, #3880ff);
}
.sectionMeta {
  margin-left: auto;
  font-size: 11px;
  font-weight: 500;
  color: color-mix(in srgb, var(--ion-text-color, #000) 55%, transparent);
}
.sectionTitle { flex: 1; }

.emptyState {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 24px 16px;
  color: color-mix(in srgb, var(--ion-text-color, #000) 45%, transparent);
  font-size: 12px;
}
.emptyState ion-icon { font-size: 24px; opacity: 0.5; }

/* ============ 趋势图 ============ */
.trendChart {
  position: relative;
  padding: 12px 14px;
  background: var(--ion-background-color, #fff);
}
.trendSvg {
  display: block;
  width: 100%;
  height: 80px;
}
.trendGrid line {
  stroke: color-mix(in srgb, var(--ion-color-medium, #92949c) 25%, transparent);
  stroke-width: 0.5;
  stroke-dasharray: 2 3;
}
.trendLine {
  fill: none;
  stroke: var(--ion-color-primary, #3880ff);
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
  filter: drop-shadow(0 1px 1px color-mix(in srgb, var(--ion-color-primary, #3880ff) 40%, transparent));
}
.trendDot {
  fill: var(--ion-color-primary, #3880ff);
  opacity: 0.7;
}
.trendEmpty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 20px 0;
  color: color-mix(in srgb, var(--ion-text-color, #000) 45%, transparent);
  font-size: 12px;
}
.trendEmpty ion-icon { font-size: 24px; opacity: 0.5; }
.trendLegend {
  display: flex;
  gap: 12px;
  padding: 6px 14px 12px;
  font-size: 11px;
  color: color-mix(in srgb, var(--ion-text-color, #000) 55%, transparent);
  font-variant-numeric: tabular-nums;
}
.legendItem { display: inline-flex; align-items: center; gap: 4px; }
.legendSwatch {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 2px;
}
.legendSwatchMin { background: var(--ion-color-success, #2dd55b); }
.legendSwatchAvg { background: var(--ion-color-primary, #3880ff); }
.legendSwatchMax { background: var(--ion-color-warning, #ffc409); }

/* ============ 时间线 ============ */
.timeline {
  list-style: none;
  margin: 0;
  padding: 6px 14px 12px;
}
.timelineItem {
  position: relative;
  display: flex;
  gap: 10px;
  padding: 6px 0;
}
.timelineItem::before {
  content: '';
  position: absolute;
  left: 5px;
  top: 18px;
  bottom: -6px;
  width: 1px;
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 30%, transparent);
}
.timelineItem:last-child::before { display: none; }
.timelineDot {
  width: 11px;
  height: 11px;
  border-radius: 50%;
  margin-top: 5px;
  flex-shrink: 0;
}
.timelineDot.is-online { background: var(--ion-color-success, #2dd55b); }
.timelineDot.is-offline { background: var(--ion-color-danger, #eb445a); }
.timelineDot.is-checking { background: var(--ion-color-warning, #ffc409); }
.timelineBody { flex: 1; min-width: 0; }
.timelineRow {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.timelineState { font-size: 13px; font-weight: 600; color: var(--ion-text-color, #000); }
.timelineTime {
  font-size: 11px;
  color: color-mix(in srgb, var(--ion-text-color, #000) 55%, transparent);
  font-family: var(--ion-font-family-monospace, monospace);
  font-variant-numeric: tabular-nums;
}
.timelineReason {
  font-size: 11px;
  color: color-mix(in srgb, var(--ion-text-color, #000) 65%, transparent);
  margin-top: 1px;
}

/* ============ 操作历史 ============ */
.actionLog {
  list-style: none;
  margin: 0;
  padding: 6px 14px 12px;
}
.actionLogItem {
  display: flex;
  gap: 10px;
  padding: 6px 0;
  align-items: flex-start;
}
.actionLogIcon {
  font-size: 18px;
  margin-top: 2px;
  flex-shrink: 0;
}
.actionLogIcon.is-success { color: var(--ion-color-success, #2dd55b); }
.actionLogIcon.is-failed { color: var(--ion-color-danger, #eb445a); }
.actionLogBody { flex: 1; min-width: 0; }
.actionLogRow {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.actionLogName { font-size: 13px; font-weight: 500; color: var(--ion-text-color, #000); }
.actionLogTime {
  font-size: 11px;
  color: color-mix(in srgb, var(--ion-text-color, #000) 55%, transparent);
  font-family: var(--ion-font-family-monospace, monospace);
  font-variant-numeric: tabular-nums;
}
.actionLogDetail {
  font-size: 11px;
  color: color-mix(in srgb, var(--ion-text-color, #000) 60%, transparent);
  margin-top: 1px;
  font-family: var(--ion-font-family-monospace, monospace);
  word-break: break-all;
}

/* ============ 网络诊断 ============ */
.networkBox {
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.networkRow {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.networkLabel {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: color-mix(in srgb, var(--ion-text-color, #000) 55%, transparent);
  font-weight: 500;
}
.networkValue {
  font-size: 12px;
  color: var(--ion-text-color, #000);
  word-break: break-all;
}
.pingResult {
  display: flex;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  align-items: flex-start;
}
.pingResult.is-success {
  background: color-mix(in srgb, var(--ion-color-success, #2dd55b) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--ion-color-success, #2dd55b) 30%, transparent);
}
.pingResult.is-failed {
  background: color-mix(in srgb, var(--ion-color-danger, #eb445a) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--ion-color-danger, #eb445a) 30%, transparent);
}
.pingResultIcon { font-size: 20px; flex-shrink: 0; }
.pingResult.is-success .pingResultIcon { color: var(--ion-color-success, #2dd55b); }
.pingResult.is-failed .pingResultIcon { color: var(--ion-color-danger, #eb445a); }
.pingResultText { flex: 1; min-width: 0; }
.pingResultTitle {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-text-color, #000);
  display: flex;
  gap: 8px;
  align-items: center;
}
.pingResultMs {
  font-family: var(--ion-font-family-monospace, monospace);
  font-size: 12px;
  color: var(--ion-color-primary, #3880ff);
  font-variant-numeric: tabular-nums;
}
.pingResultError {
  font-size: 11px;
  color: var(--ion-color-danger, #eb445a);
  margin-top: 2px;
  font-family: var(--ion-font-family-monospace, monospace);
}

/* ============ 应用元信息 ============ */
.metaGrid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px 14px;
  padding: 12px 14px;
}
.metaCell {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.metaCellFull { grid-column: 1 / -1; }
.metaLabel {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: color-mix(in srgb, var(--ion-text-color, #000) 55%, transparent);
  font-weight: 500;
}
.metaValue {
  font-size: 12px;
  color: var(--ion-text-color, #000);
  word-break: break-all;
  display: flex;
  gap: 4px;
  align-items: center;
  flex-wrap: wrap;
}
.metaValue.monospace {
  font-family: var(--ion-font-family-monospace, monospace);
}
.metaValue.small { font-size: 11px; }
.badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 999px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.badge.is-yes { background: color-mix(in srgb, var(--ion-color-success, #2dd55b) 22%, transparent); color: var(--ion-color-success, #2dd55b); }
.badge.is-no { background: color-mix(in srgb, var(--ion-color-medium, #92949c) 22%, transparent); color: color-mix(in srgb, var(--ion-text-color, #000) 60%, transparent); }
.badge.is-sandbox { background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 22%, transparent); color: var(--ion-color-warning-shade, #cc8a00); }

/* ============ 响应式 ============ */
@media (max-width: 380px) {
  .stateSummary { flex-wrap: wrap; }
  .stateQuickFacts { width: 100%; justify-content: flex-end; }
  .metaGrid { grid-template-columns: 1fr; }
}
</style>
