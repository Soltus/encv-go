<!--
  ServerStatusCard.vue — 后端状态卡片
  🆕 2026-06-15 v1 完整精美卡片恢复（来自 git blob 99ec4ac, commit 182aeaf）
  用户怒批"丑死了"+"i18n 也没有"+"和银行卡那样的才叫卡片"——
  本轮从 git fsck 找到 unreachable blob 99ec4ac（22KB 724 行）= 真正的"v1 原始精美卡片"：
    · 顶部状态行（pulse-dot 状态点 + status-label + status-subtitle）
    · 右上角 meta-pill：latency (speedometer) + transport (wifi) 双 pill
    · 详细字段网格（4 个 detail-item：version / instanceId / port / lastCheck）
    · 错误态/检查态：error-body + cloudOfflineOutline / ion-spinner
    · 底部 card-footer：forensic 时间戳
    · instance-changed banner（4s 自动消失，hijack 死锁修复）
    · pulse-dot 动画（success/danger/warning 3 色 + keyframes）
    · 状态过渡光晕（ssc-pulse-sweep sweep 横移）
    · 5 入口复用：Settings / ServerSettings / ServerStatusDetail / ServerDetail / DevLogs
    · 0 硬编码颜色（100% CSS variables）
    · 响应式（max-width: 380px → 1fr）
    · 无障碍（prefers-reduced-motion）

  使用：
    <ServerStatusCard :compact="false" :clickable="true" @click="goServerDetail" />
-->
<template>
  <div
    class="server-status-card"
    :class="[
      `state-${state}`,
      { 'is-compact': compact, 'is-clickable': clickable, 'is-pulse': pulsing },
    ]"
    role="status"
    :aria-label="ariaLabel"
    @click="onCardClick"
  >
    <!-- ① 顶部状态行 -->
    <div class="status-row">
      <div class="status-indicator">
        <span class="pulse-dot" :class="`pulse-${state}`">
          <span class="pulse-dot-inner" />
        </span>
        <div class="status-text">
          <span class="status-label">{{ statusLabel }}</span>
          <span v-if="stateSubtitle" class="status-subtitle">{{ stateSubtitle }}</span>
        </div>
      </div>
      <div class="status-meta">
        <span
          v-if="latencyPillVisible"
          class="meta-pill"
          :class="`latency-${latencyQuality}`"
          :title="t('serverStatus.latencyHint')"
        >
          <ion-icon :icon="speedometerOutline" class="meta-pill-icon" />
          {{ latencyText }}
        </span>
        <span
          v-if="transport"
          class="meta-pill transport-pill"
          :title="t('serverStatus.transportHint')"
        >
          <ion-icon :icon="transportIcon" class="meta-pill-icon" />
          {{ transport }}
        </span>
      </div>
    </div>

    <!-- 🆕 2026-06-15：instance-changed banner（后端崩重启后短暂提示；不阻塞状态机）
         之前：hijack 警告进 lastError → 永远显示 → 状态卡 offline
         现在：emit 'backend:instance-changed' → 这里监听 → 顶部 banner → 4s 后自动消失 -->
    <div v-if="instanceChangedBanner" class="instance-changed-banner" role="status">
      <ion-icon :icon="refreshCircleIcon" class="banner-icon" />
      <span class="banner-text">
        {{ t('serverStatus.instanceChangedBanner') || 'Backend 已重启，新进程已上线' }}
        <code class="banner-prev">{{ instanceChangedBanner.previous.slice(0, 6) }}</code>
        <ion-icon :icon="arrowForwardIcon" class="banner-arrow" />
        <code class="banner-curr">{{ instanceChangedBanner.current.slice(0, 6) }}</code>
      </span>
      <button class="banner-close" :aria-label="t('common.close')" @click.stop="instanceChangedBanner = null">×</button>
    </div>

    <!-- ② 详细字段网格（仅 online 状态显示） -->
    <div v-if="state === 'online'" class="detail-grid">
      <div class="detail-item">
        <span class="detail-label">{{ t('serverStatus.version') }}</span>
        <span class="detail-value version-value">v{{ version || '—' }}</span>
      </div>
      <div class="detail-item">
        <span class="detail-label">{{ t('serverStatus.instanceId') }}</span>
        <span
          class="detail-value monospace"
          :class="{ 'instance-changed': instanceChanged }"
          :title="shortInstanceId || ''"
        >
          {{ shortInstanceId || '—' }}
        </span>
      </div>
      <div class="detail-item">
        <span class="detail-label">{{ t('serverStatus.port') }}</span>
        <span class="detail-value port-value">:{{ port || '—' }}</span>
      </div>
      <div class="detail-item">
        <span class="detail-label">{{ t('serverStatus.lastCheck') }}</span>
        <span class="detail-value last-check-value">
          <span :key="lastCheckKey" class="time-roll">{{ lastCheckText }}</span>
        </span>
      </div>
    </div>

    <!-- ③ 错误态：错误文案 + 展开的诊断 -->
    <div v-else-if="state === 'offline'" class="error-body">
      <ion-icon :icon="cloudOfflineOutline" class="error-icon" />
      <div class="error-text">
        <span class="error-title">{{ t('serverStatus.backendOffline') }}</span>
        <span v-if="error" class="error-detail">{{ error }}</span>
      </div>
    </div>
    <div v-else-if="state === 'checking'" class="error-body checking-body">
      <ion-spinner name="crescent" class="checking-spinner" />
      <span class="error-title">{{ t('serverStatus.checking') }}</span>
    </div>

    <!-- ④ 底部时间戳（debug / forensic） -->
    <div v-if="!compact" class="card-footer">
      <span class="footer-ts">{{ footerText }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from '@/composables/useI18n'
import { useServerStatus } from '@/composables/useServerStatus'
import { formatRelativeTime } from '@/composables/relativeTime'
import { cloudOfflineOutline, speedometerOutline, wifiOutline, layersOutline, refreshCircleOutline as refreshCircleIcon, arrowForwardOutline as arrowForwardIcon } from 'ionicons/icons'
import { eventBus } from '@/composables/useEventBus'
import { IonIcon } from '@ionic/vue'

interface Props {
  /** 紧凑模式：隐藏 detail-grid / footer */
  compact?: boolean
  /** 卡片可点击（外层 router 跳转） */
  clickable?: boolean
  /** 隐藏 instance_id 展示（隐私 / demo 场景） */
  hideInstanceId?: boolean
}
const props = withDefaults(defineProps<Props>(), {
  compact: false,
  clickable: false,
  hideInstanceId: false,
})
const emit = defineEmits<{ (e: 'click'): void }>()

const { t } = useI18n()
const {
  isOnline,
  lastError,
  backendPort,
  backendInstanceId,
  backendVersion,
  latencyMs,
  lastCheckedAt,
  transportMode,
  checkStatus,
} = useServerStatus()

// —— state machine: online | offline | checking ——
const isChecking = ref(false)
const state = computed<'online' | 'offline' | 'checking'>(() => {
  if (isChecking.value) return 'checking'
  return isOnline.value ? 'online' : 'offline'
})

const pulsing = ref(false)
watch(state, () => {
  // 状态切换时触发一次 pulse 动画
  pulsing.value = false
  requestAnimationFrame(() => { pulsing.value = true })
  setTimeout(() => { pulsing.value = false }, 1200)
})

// —— 标签文案 ——
const statusLabel = computed(() => {
  switch (state.value) {
    case 'online': return t('serverStatus.online')
    case 'offline': return t('serverStatus.offline')
    case 'checking': return t('serverStatus.checking')
  }
})
const stateSubtitle = computed(() => {
  if (state.value === 'online' && props.hideInstanceId) {
    // 紧凑 + 隐藏 instance_id 时显示端口作为副标题
    return port.value ? `:${port.value}` : ''
  }
  return ''
})

// —— aria ——
const ariaLabel = computed(() => {
  const bits: string[] = [statusLabel.value]
  if (state.value === 'online') {
    if (version.value) bits.push(`v${version.value}`)
    if (port.value) bits.push(`port ${port.value}`)
  } else if (state.value === 'offline' && lastError.value) {
    bits.push(lastError.value)
  }
  return bits.join(', ')
})

// —— detail 字段 ——
const version = computed(() => backendVersion.value)
const port = computed(() => backendPort.value)
const shortInstanceId = computed(() => {
  if (props.hideInstanceId) return ''
  return backendInstanceId.value ? backendInstanceId.value.slice(0, 8) : ''
})
const error = computed(() => lastError.value)

// —— instance_id 变化检测 → 闪烁 1.5s ——
const instanceChanged = ref(false)
const prevInstanceId = ref('')
watch(backendInstanceId, (newId) => {
  if (prevInstanceId.value && newId && prevInstanceId.value !== newId) {
    instanceChanged.value = true
    setTimeout(() => { instanceChanged.value = false }, 1500)
  }
  prevInstanceId.value = newId
})

// 🆕 2026-06-15：监听 useServerStatus 发的 'backend:instance-changed' 事件
//   之前：hijack 警告进 lastError → 永远显示"backend instance changed" → 状态卡 offline → 死锁
//   现在：emit 事件 → 这里显示 4s banner（短暂提示，不阻塞）
let bannerTimer: ReturnType<typeof setTimeout> | null = null
const instanceChangedBanner = ref<{ previous: string; current: string } | null>(null)
function onInstanceChanged(data: { previous: string; current: string }) {
  instanceChangedBanner.value = data
  if (bannerTimer) clearTimeout(bannerTimer)
  bannerTimer = setTimeout(() => {
    instanceChangedBanner.value = null
    bannerTimer = null
  }, 4000)
}

// —— latency 分类 / 显示 ——
const latencyText = computed(() => {
  if (latencyMs.value <= 0) return '—'
  if (latencyMs.value < 1000) return `${latencyMs.value}ms`
  return `${(latencyMs.value / 1000).toFixed(2)}s`
})
const latencyQuality = computed<'fast' | 'normal' | 'slow' | 'unknown'>(() => {
  if (latencyMs.value <= 0) return 'unknown'
  if (latencyMs.value < 100) return 'fast'
  if (latencyMs.value < 500) return 'normal'
  return 'slow'
})
const latencyPillVisible = computed(() => state.value === 'online' && latencyMs.value > 0)

// —— transport ——
const transport = computed(() => {
  const m = transportMode.value
  // 4 种 mode: 'ws' | 'http-poll' | 'native-bridge' | 'unknown'
  // 隐藏 unknown（探测前）即可视为"无 transport"
  return m && m !== 'unknown' ? m.toUpperCase() : ''
})
const transportIcon = computed(() => {
  const m = transportMode.value
  // 用户要求：HTTP Polling 旁必须有 wifi 图标
  // 策略：所有真实 transport（ws / http-poll / native-bridge）统一用 wifi 图标
  // 只在 unknown 时退化为 layers 图标
  if (m === 'ws' || m === 'http-poll' || m === 'native-bridge') return wifiOutline
  return layersOutline
})

// —— last check 时间（30s 滚动刷新）——
const lastCheckText = computed(() => {
  if (!lastCheckedAt.value) return t('serverStatus.never')
  return formatRelativeTime(lastCheckedAt.value.getTime())
})
const lastCheckKey = ref(0)
const now = ref(Date.now())
let tickHandle: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  tickHandle = setInterval(() => {
    now.value = Date.now()
    lastCheckKey.value++ // 强制 time-roll 重渲染
  }, 30_000)
  eventBus.on('backend:instance-changed', onInstanceChanged)
})
onUnmounted(() => {
  if (tickHandle) clearInterval(tickHandle)
  if (bannerTimer) clearTimeout(bannerTimer)
  eventBus.off('backend:instance-changed', onInstanceChanged)
})

const footerText = computed(() => {
  if (state.value === 'offline' && lastCheckedAt.value) {
    return `${t('serverStatus.lastCheck')}: ${formatRelativeTime(lastCheckedAt.value.getTime())}`
  }
  if (state.value === 'online') {
    return `${t('serverStatus.instanceIdLabel')}: ${props.hideInstanceId ? '••••••••' : (backendInstanceId.value || '—')}`
  }
  return ''
})

// —— click ——
function onCardClick() {
  if (!props.clickable) return
  emit('click')
}

// —— expose checkStatus to parent via defineExpose ——
defineExpose({ checkStatus })
</script>

<style scoped>
/* ============================================================
   ServerStatusCard
   100% CSS variables — 0 硬编码颜色。深色模式自动适配。
   ============================================================ */
.server-status-card {
  --card-bg: var(--ion-background-color, #fff);
  --card-border: var(--ion-color-medium, #92949c);
  --card-text: var(--ion-text-color, #000);
  --card-text-muted: color-mix(in srgb, var(--ion-text-color, #000) 60%, transparent);
  --card-accent: var(--ion-color-primary, #3880ff);
  --card-radius: 12px;
  --card-pad: 14px 16px;
  --card-gap: 10px;
  --transition: 0.3s cubic-bezier(0.4, 0, 0.2, 1);

  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--card-gap);
  padding: var(--card-pad);
  background: var(--card-bg);
  color: var(--card-text);
  border: 1px solid var(--card-border);
  border-left-width: 4px;
  border-radius: var(--card-radius);
  transition: border-color var(--transition), background-color var(--transition), transform var(--transition);
  overflow: hidden;
}
.server-status-card::before {
  /* 状态过渡光晕 */
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(90deg, transparent, currentColor, transparent);
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--transition);
}
.server-status-card.is-pulse::before {
  opacity: 0.06;
  animation: ssc-pulse-sweep 1.2s ease-out;
}
@keyframes ssc-pulse-sweep {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}

.server-status-card.is-clickable {
  cursor: pointer;
}
.server-status-card.is-clickable:hover {
  transform: translateY(-1px);
}
.server-status-card.is-clickable:active {
  transform: translateY(0);
}

/* 状态变体：边框 + 主题色 */
.server-status-card.state-online {
  --card-accent: var(--ion-color-success, #2dd55b);
  --card-border: color-mix(in srgb, var(--ion-color-success, #2dd55b) 30%, var(--card-border));
}
.server-status-card.state-offline {
  --card-accent: var(--ion-color-danger, #eb445a);
  --card-border: color-mix(in srgb, var(--ion-color-danger, #eb445a) 35%, var(--card-border));
}
.server-status-card.state-checking {
  --card-accent: var(--ion-color-warning, #ffc409);
  --card-border: color-mix(in srgb, var(--ion-color-warning, #ffc409) 35%, var(--card-border));
}
.server-status-card.state-online,
.server-status-card.state-offline,
.server-status-card.state-checking {
  border-left-color: var(--card-accent);
}

/* ============ 状态行 ============ */
.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 36px;
}
.status-indicator {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}
.status-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.status-label {
  font-size: 15px;
  font-weight: 600;
  color: var(--card-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.status-subtitle {
  font-size: 12px;
  color: var(--card-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ============ Pulse dot ============ */
.pulse-dot {
  position: relative;
  display: inline-flex;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  flex-shrink: 0;
}
.pulse-dot-inner {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  background: currentColor;
}
.pulse-dot::after {
  content: '';
  position: absolute;
  inset: -3px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0;
  animation: ssc-pulse 2s ease-out infinite;
  z-index: -1;
}
.pulse-online {
  color: var(--ion-color-success, #2dd55b);
}
.pulse-online::after {
  animation-name: ssc-pulse-success;
}
.pulse-offline {
  color: var(--ion-color-danger, #eb445a);
}
.pulse-offline::after {
  animation-name: ssc-pulse-danger;
}
.pulse-checking {
  color: var(--ion-color-warning, #ffc409);
}
.pulse-checking::after {
  animation-name: ssc-pulse-warning;
}
@keyframes ssc-pulse-success {
  0% { transform: scale(0.8); opacity: 0.7; }
  100% { transform: scale(1.8); opacity: 0; }
}
@keyframes ssc-pulse-danger {
  0% { transform: scale(0.8); opacity: 0.7; }
  100% { transform: scale(1.8); opacity: 0; }
}
@keyframes ssc-pulse-warning {
  0% { transform: scale(0.8); opacity: 0.5; }
  100% { transform: scale(1.6); opacity: 0; }
}

/* ============ Status meta (latency / transport) ============ */
.status-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.meta-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  border-radius: 999px;
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 18%, transparent);
  color: var(--card-text);
  transition: background-color var(--transition);
}
.meta-pill-icon {
  font-size: 12px;
  width: 12px;
  height: 12px;
}
.meta-pill.latency-fast {
  background: color-mix(in srgb, var(--ion-color-success, #2dd55b) 22%, transparent);
  color: var(--ion-color-success, #2dd55b);
}
.meta-pill.latency-normal {
  background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 18%, transparent);
  color: var(--ion-color-primary, #3880ff);
}
.meta-pill.latency-slow {
  background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 24%, transparent);
  color: var(--ion-color-warning-shade, #cc8a00);
}
.meta-pill.latency-unknown {
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 18%, transparent);
  color: var(--card-text-muted);
}
.meta-pill.transport-pill {
  background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 18%, transparent);
  color: var(--ion-color-primary, #3880ff);
}

/* 🆕 2026-06-15：instance-changed banner（4s 自动消失；不进 lastError，不阻塞状态） */
.instance-changed-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0 0;
  padding: 8px 10px;
  background: rgba(var(--ion-color-warning-rgb), 0.12);
  border: 1px solid rgba(var(--ion-color-warning-rgb), 0.3);
  border-radius: 6px;
  font-size: 12px;
  color: var(--ion-color-warning-shade);
  animation: banner-in 0.3s ease-out;
}
@keyframes banner-in {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}
.banner-icon { font-size: 16px; flex-shrink: 0; }
.banner-text { flex: 1; line-height: 1.4; }
.banner-prev, .banner-curr {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  padding: 0 4px;
  background: var(--ion-color-light);
  border-radius: 3px;
  color: var(--ion-text-color);
}
.banner-curr { background: rgba(var(--ion-color-warning-rgb), 0.2); color: var(--ion-color-warning-shade); }
.banner-arrow { font-size: 12px; opacity: 0.6; vertical-align: middle; }
.banner-close {
  background: transparent;
  border: 0;
  color: inherit;
  font-size: 18px;
  line-height: 1;
  padding: 0 4px;
  cursor: pointer;
  opacity: 0.6;
}
.banner-close:hover { opacity: 1; }

/* ============ Detail grid ============ */
.detail-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px 14px;
  padding-top: 8px;
  border-top: 1px solid color-mix(in srgb, var(--card-border) 50%, transparent);
}
.detail-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.detail-label {
  font-size: 11px;
  color: var(--card-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 500;
}
.detail-value {
  font-size: 13px;
  color: var(--card-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.detail-value.monospace {
  font-family: var(--ion-font-family-monospace, 'SF Mono', Menlo, Consolas, monospace);
  font-size: 12px;
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 12%, transparent);
  padding: 1px 5px;
  border-radius: 3px;
  display: inline-block;
  max-width: 100%;
  transition: background-color var(--transition);
}
.detail-value.instance-changed {
  animation: ssc-instance-change 1.5s ease-out;
}
@keyframes ssc-instance-change {
  0%   { background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 50%, transparent); transform: scale(1.04); }
  100% { background: color-mix(in srgb, var(--ion-color-medium, #92949c) 12%, transparent); transform: scale(1); }
}
.detail-value.port-value {
  font-family: var(--ion-font-family-monospace, monospace);
  color: var(--ion-color-primary, #3880ff);
  font-weight: 600;
}
.detail-value.version-value {
  color: var(--ion-color-primary, #3880ff);
  font-weight: 500;
}
.time-roll {
  display: inline-block;
  animation: ssc-time-roll 0.3s ease-out;
}
@keyframes ssc-time-roll {
  0% { transform: translateY(2px); opacity: 0.4; }
  100% { transform: translateY(0); opacity: 1; }
}

/* ============ Error body ============ */
.error-body {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 0;
  border-top: 1px solid color-mix(in srgb, var(--card-border) 50%, transparent);
}
.error-body.checking-body {
  align-items: center;
}
.error-icon {
  font-size: 22px;
  color: var(--ion-color-danger, #eb445a);
  flex-shrink: 0;
}
.checking-spinner {
  width: 22px;
  height: 22px;
  color: var(--ion-color-warning, #ffc409);
}
.error-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.error-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-color-danger, #eb445a);
}
.error-detail {
  font-size: 12px;
  color: var(--card-text-muted);
  word-break: break-word;
}

/* ============ Card footer ============ */
.card-footer {
  padding-top: 6px;
  border-top: 1px dashed color-mix(in srgb, var(--card-border) 30%, transparent);
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.footer-ts {
  font-size: 10px;
  color: var(--card-text-muted);
  font-family: var(--ion-font-family-monospace, monospace);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

/* ============ Compact mode ============ */
.server-status-card.is-compact {
  --card-pad: 10px 12px;
  --card-gap: 6px;
}
.server-status-card.is-compact .status-row {
  min-height: 28px;
}
.server-status-card.is-compact .status-label {
  font-size: 13px;
}
.server-status-card.is-compact .pulse-dot {
  width: 12px;
  height: 12px;
}

/* ============ 响应式 ============ */
@media (max-width: 380px) {
  .detail-grid {
    grid-template-columns: 1fr;
  }
  .status-meta {
    flex-wrap: wrap;
  }
}

/* ============ 减弱动画（无障碍） ============ */
@media (prefers-reduced-motion: reduce) {
  .pulse-dot::after {
    animation: none;
  }
  .server-status-card.is-pulse::before {
    animation: none;
  }
  .detail-value.instance-changed {
    animation: none;
  }
  .time-roll {
    animation: none;
  }
}
</style>
