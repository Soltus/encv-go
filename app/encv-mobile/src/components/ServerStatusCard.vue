<!--
  ServerStatusCard.vue — 后端连接状态可视化卡片

  单一职责：把后端进程的健康度（online/offline/checking）+ 身份（version / instance_id /
  port / transport / latency / last check）可视化成一张可交互的卡片。**不承载任何设置项
  配置 / 表单 / 列表项 / 跳转入口**。需要这些的请用 ion-item / form。

  使用边界（避免与"设置项"卡片混淆）：
    ✅ 可用位置
      · ServerDetail.vue        详情页状态行
      · ServerStatusDetail.vue  独立诊断页（直接 useServerStatus 而非此组件）
      · DevLogs.vue             debug 工具顶栏（compact 模式）
    ❌ 不可用位置（这些是"设置"语义，放这里会让用户误以为是"系统设置建议"）
      · Settings.vue            设置首页（配置项 + 主题 + 权限）
      · ServerSettings.vue      URL 配置页（属于"配置 backend 怎么连"，不是"显示连上没"）
      · AgentSettingsDetail.vue Agent 设置详情（"AI 行为配置"，不是"后端健康度"）

  视觉：3D 实体化（perspective + 厚度 box-shadow）+ 双面翻转（点正面看诊断，点反面回到状态）。
  主题：100% CSS variables（--ion-color-* / --ion-background-color / --ion-text-color），
        0 硬编码颜色。深色模式自动适配。

  使用：
    <ServerStatusCard :compact="false" :clickable="true" @click="goServerDetail" />
-->
<template>
  <div
    class="card-3d-wrapper"
    :class="[`state-${state}`, { 'is-flipped': isFlipped }]"
    role="status"
    :aria-label="ariaLabel"
  >
    <div class="card-3d-inner">
      <!-- ============ 正面：状态概览 ============ -->
      <div class="card-face card-face-front" @click="onCardClick">
        <!-- 状态行 -->
        <div class="status-row">
          <div class="status-indicator">
            <span class="pulse-dot" :class="`pulse-${state}`">
              <span class="pulse-dot-inner" />
            </span>
            <div class="status-text">
              <span class="status-label">{{ statusLabel }}</span>
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

        <!-- instance-changed banner（4s 自动消失；不进 lastError，不阻塞状态） -->
        <div v-if="instanceChangedBanner" class="instance-changed-banner" role="status">
          <ion-icon :icon="refreshCircleIcon" class="banner-icon" />
          <span class="banner-text">
            {{ t('serverStatus.instanceChangedBanner') }}
            <code class="banner-prev">{{ instanceChangedBanner.previous.slice(0, 6) }}</code>
            <ion-icon :icon="arrowForwardIcon" class="banner-arrow" />
            <code class="banner-curr">{{ instanceChangedBanner.current.slice(0, 6) }}</code>
          </span>
          <button class="banner-close" :aria-label="t('common.close')" @click.stop="instanceChangedBanner = null">×</button>
        </div>

        <!-- 详细字段网格（仅 online 状态显示） -->
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
              :title="backendInstanceId || ''"
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

        <!-- 错误态 -->
        <div v-else-if="state === 'offline'" class="error-body">
          <ion-icon :icon="cloudOfflineOutline" class="error-icon" />
          <div class="error-text">
            <span class="error-title">{{ t('serverStatus.backendOffline') }}</span>
            <span v-if="error" class="error-detail">{{ error }}</span>
          </div>
        </div>

        <!-- 检查态 -->
        <div v-else-if="state === 'checking'" class="error-body checking-body">
          <ion-spinner name="crescent" class="checking-spinner" />
          <span class="error-title">{{ t('serverStatus.checking') }}</span>
        </div>

        <!-- 翻转提示 -->
        <div class="flip-hint" aria-hidden="true">
          <ion-icon :icon="refreshIcon" class="flip-hint-icon" />
          <span class="flip-hint-text">{{ t('serverStatus.flipHint') }}</span>
        </div>
      </div>

      <!-- ============ 反面：诊断 / 操作历史 ============ -->
      <div class="card-face card-face-back" @click="onCardClick">
        <div class="back-header">
          <ion-icon :icon="pulseIcon" class="back-header-icon" />
          <span class="back-header-title">{{ t('serverStatus.diagnosticsTitle') }}</span>
        </div>

        <!-- 完整 instance_id（不再截断） -->
        <div class="back-section">
          <div class="back-label">{{ t('serverStatus.fullInstanceId') }}</div>
          <div
            class="back-value monospace"
            :class="{ 'instance-changed': instanceChanged }"
            :title="backendInstanceId || ''"
          >
            {{ backendInstanceId || '—' }}
          </div>
        </div>

        <!-- 完整 lastError 详情（仅 offline 时显示） -->
        <div v-if="state === 'offline' && error" class="back-section">
          <div class="back-label">{{ t('serverStatus.fullError') }}</div>
          <div class="back-value error-text-mono">{{ error }}</div>
        </div>

        <!-- 完整 transport 描述 -->
        <div class="back-section">
          <div class="back-label">{{ t('serverStatus.transportDesc') }}</div>
          <div class="back-value">
            <ion-icon :icon="transportIcon" class="back-transport-icon" />
            {{ transportFullLabel }}
          </div>
        </div>

        <!-- 时间戳详情 -->
        <div v-if="!compact" class="back-section">
          <div class="back-label">{{ t('serverStatus.timestamp') }}</div>
          <div class="back-value monospace">{{ lastCheckAbsolute }}</div>
        </div>

        <div class="flip-hint" aria-hidden="true">
          <ion-icon :icon="flipBackIcon" class="flip-hint-icon" />
          <span class="flip-hint-text">{{ t('serverStatus.flipBackHint') }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from '@/composables/useI18n'
import { useServerStatus } from '@/composables/useServerStatus'
import { formatRelativeTime } from '@/composables/relativeTime'
import {
  cloudOfflineOutline,
  speedometerOutline,
  wifiOutline,
  layersOutline,
  refreshCircleOutline as refreshCircleIcon,
  arrowForwardOutline as arrowForwardIcon,
  refreshOutline as refreshIcon,
  syncOutline as flipBackIcon,
  pulseOutline as pulseIcon,
} from 'ionicons/icons'
import { eventBus } from '@/composables/useEventBus'
import { IonIcon } from '@ionic/vue'

interface Props {
  /** 紧凑模式：省略反面时间戳详情 */
  compact?: boolean
  /** 卡片可点击 → 触发外部 click 事件（注意：内部翻转也走 click，但 emit 仍 fire） */
  clickable?: boolean
}
const props = withDefaults(defineProps<Props>(), {
  compact: false,
  clickable: false,
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

// —— 3D 翻转 ——
const isFlipped = ref(false)
watch(state, () => {
  // 状态切换时自动回正面（避免误导）
  isFlipped.value = false
})

// —— 脉冲 / 光泽动画 ——
const pulsing = ref(false)
watch(state, () => {
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

// —— 监听 useServerStatus 发的 'backend:instance-changed' 事件（4s banner） ——
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

// —— transport 显示 ——
const transport = computed(() => {
  const m = transportMode.value
  return m && m !== 'unknown' ? m.toUpperCase() : ''
})
const transportFullLabel = computed(() => {
  const m = transportMode.value
  switch (m) {
    case 'ws': return t('serverStatus.transportWs') || 'WebSocket (real-time push)'
    case 'http-poll': return t('serverStatus.transportHttpPoll') || 'HTTP Polling (periodic pull)'
    case 'native-bridge': return t('serverStatus.transportNativeBridge') || 'Native bridge (in-app IPC)'
    default: return t('serverStatus.transportUnknown') || 'Unknown'
  }
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
const lastCheckAbsolute = computed(() => {
  if (!lastCheckedAt.value) return '—'
  const d = lastCheckedAt.value
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
})
const lastCheckKey = ref(0)
const now = ref(Date.now())
let tickHandle: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  tickHandle = setInterval(() => {
    now.value = Date.now()
    lastCheckKey.value++
  }, 30_000)
  eventBus.on('backend:instance-changed', onInstanceChanged)
})
onUnmounted(() => {
  if (tickHandle) clearInterval(tickHandle)
  if (bannerTimer) clearTimeout(bannerTimer)
  eventBus.off('backend:instance-changed', onInstanceChanged)
})

// —— 点击：翻转（如果 clickable 也 emit 外部 click） ——
function onCardClick(event: MouseEvent) {
  // 阻止子元素点击冒泡时翻转（按钮 / pill / 链接等）
  const target = event.target as HTMLElement
  if (target.closest('button, a, .meta-pill, .flip-hint')) {
    return
  }
  isFlipped.value = !isFlipped.value
  if (props.clickable) emit('click')
}

// —— expose checkStatus to parent via defineExpose ——
defineExpose({ checkStatus })
</script>

<style scoped>
/* ============================================================
   ServerStatusCard — 3D 实体化 + 双面翻转
   100% CSS variables — 0 硬编码颜色。深色模式自动适配。
   ============================================================ */

/* ============ 3D 容器 ============ */
.card-3d-wrapper {
  --card-bg: var(--ion-background-color, #fff);
  --card-border: var(--ion-color-medium, #92949c);
  --card-text: var(--ion-text-color, #000);
  --card-text-muted: color-mix(in srgb, var(--ion-text-color, #000) 60%, transparent);
  --card-accent: var(--ion-color-primary, #3880ff);
  --card-radius: 14px;
  --transition-3d: 0.6s cubic-bezier(0.4, 0.0, 0.2, 1);
  --transition-fast: 0.3s cubic-bezier(0.4, 0, 0.2, 1);

  position: relative;
  perspective: 1200px;
  perspective-origin: 50% 30%;
  width: 100%;
  /* 3D 实体化的"厚度" — 多层 box-shadow 模拟金属质感 */
  filter: drop-shadow(0 1px 1px rgba(0, 0, 0, 0.08))
          drop-shadow(0 4px 8px rgba(0, 0, 0, 0.06))
          drop-shadow(0 8px 16px rgba(0, 0, 0, 0.04));
}

.card-3d-inner {
  position: relative;
  width: 100%;
  min-height: 140px;
  transform-style: preserve-3d;
  transition: transform var(--transition-3d);
}
.card-3d-wrapper.is-flipped .card-3d-inner {
  transform: rotateY(180deg);
}

/* ============ 双面通用 ============ */
.card-face {
  position: absolute;
  inset: 0;
  backface-visibility: hidden;
  -webkit-backface-visibility: hidden;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px;
  background: var(--card-bg);
  color: var(--card-text);
  border: 1px solid var(--card-border);
  border-left-width: 4px;
  border-radius: var(--card-radius);
  /* 内层 3D 阴影（叠加 drop-shadow 形成厚度） */
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.4),
              inset 0 -1px 0 rgba(0, 0, 0, 0.04);
  transition: border-color var(--transition-fast), background-color var(--transition-fast);
  min-height: inherit;
  cursor: pointer;
  overflow: hidden;
}
.card-face-front {
  /* 正面 z-index 高，反面旋转后 z-index 翻转 */
  transform: rotateY(0deg);
}
.card-face-back {
  transform: rotateY(180deg);
}

/* ============ 状态变体：边框 + 主题色 ============ */
.card-3d-wrapper.state-online {
  --card-accent: var(--ion-color-success, #2dd55b);
  --card-border: color-mix(in srgb, var(--ion-color-success, #2dd55b) 30%, var(--ion-color-medium, #92949c));
}
.card-3d-wrapper.state-offline {
  --card-accent: var(--ion-color-danger, #eb445a);
  --card-border: color-mix(in srgb, var(--ion-color-danger, #eb445a) 35%, var(--ion-color-medium, #92949c));
}
.card-3d-wrapper.state-checking {
  --card-accent: var(--ion-color-warning, #ffc409);
  --card-border: color-mix(in srgb, var(--ion-color-warning, #ffc409) 35%, var(--ion-color-medium, #92949c));
}
.card-face {
  border-left-color: var(--card-accent);
}

/* ============ 状态切换光泽扫过 ============ */
.card-face::before {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(105deg, transparent 30%, rgba(255, 255, 255, 0.18) 50%, transparent 70%);
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--transition-fast);
}
.card-3d-wrapper.state-online .card-face-front::before,
.card-3d-wrapper.state-offline .card-face-front::before,
.card-3d-wrapper.state-checking .card-face-front::before {
  opacity: 1;
  /* 静态光泽（不扫动，状态色已隐含光感） */
}

/* ============ 状态行（正面） ============ */
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
  inset: -4px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0;
  animation: ssc-pulse 2s ease-out infinite;
  z-index: -1;
}
.pulse-online { color: var(--ion-color-success, #2dd55b); }
.pulse-online::after { animation-name: ssc-pulse-success; }
.pulse-offline { color: var(--ion-color-danger, #eb445a); }
.pulse-offline::after { animation-name: ssc-pulse-danger; }
.pulse-checking { color: var(--ion-color-warning, #ffc409); }
.pulse-checking::after { animation-name: ssc-pulse-warning; }
@keyframes ssc-pulse-success {
  0% { transform: scale(0.8); opacity: 0.7; }
  100% { transform: scale(2); opacity: 0; }
}
@keyframes ssc-pulse-danger {
  0% { transform: scale(0.8); opacity: 0.7; }
  100% { transform: scale(2); opacity: 0; }
}
@keyframes ssc-pulse-warning {
  0% { transform: scale(0.8); opacity: 0.5; }
  100% { transform: scale(1.8); opacity: 0; }
}

/* ============ Status meta pills（latency / transport） ============ */
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
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  border-radius: 999px;
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 18%, transparent);
  color: var(--card-text);
  transition: background-color var(--transition-fast);
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

/* ============ instance-changed banner ============ */
.instance-changed-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 4px 0 0;
  padding: 7px 10px;
  background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--ion-color-warning, #ffc409) 30%, transparent);
  border-radius: 6px;
  font-size: 12px;
  color: var(--ion-color-warning-shade, #cc8a00);
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
  background: var(--ion-color-light, #f4f5f8);
  border-radius: 3px;
  color: var(--ion-text-color, #000);
}
.banner-curr {
  background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 20%, transparent);
  color: var(--ion-color-warning-shade, #cc8a00);
}
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

/* ============ Detail grid（正面） ============ */
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
  transition: background-color var(--transition-fast);
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

/* ============ Error body（正面） ============ */
.error-body {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 0;
  border-top: 1px solid color-mix(in srgb, var(--card-border) 50%, transparent);
}
.error-body.checking-body { align-items: center; }
.error-icon { font-size: 22px; color: var(--ion-color-danger, #eb445a); flex-shrink: 0; }
.checking-spinner { width: 22px; height: 22px; color: var(--ion-color-warning, #ffc409); }
.error-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.error-title { font-size: 13px; font-weight: 600; color: var(--ion-color-danger, #eb445a); }
.error-detail { font-size: 12px; color: var(--card-text-muted); word-break: break-word; }

/* ============ 反面：诊断 / 操作历史 ============ */
.back-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid color-mix(in srgb, var(--card-border) 50%, transparent);
}
.back-header-icon {
  font-size: 18px;
  color: var(--ion-color-primary, #3880ff);
}
.back-header-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--card-text);
}
.back-section {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.back-label {
  font-size: 10px;
  color: var(--card-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 500;
}
.back-value {
  font-size: 12px;
  color: var(--card-text);
  word-break: break-all;
  display: flex;
  align-items: center;
  gap: 6px;
}
.back-value.monospace {
  font-family: var(--ion-font-family-monospace, 'SF Mono', Menlo, Consolas, monospace);
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 12%, transparent);
  padding: 2px 6px;
  border-radius: 3px;
  display: inline-block;
  max-width: 100%;
  word-break: break-all;
}
.back-value.monospace.instance-changed {
  animation: ssc-instance-change 1.5s ease-out;
}
.back-value.error-text-mono {
  font-family: var(--ion-font-family-monospace, monospace);
  color: var(--ion-color-danger, #eb445a);
  background: color-mix(in srgb, var(--ion-color-danger, #eb445a) 8%, transparent);
  padding: 4px 8px;
  border-radius: 4px;
  border-left: 2px solid var(--ion-color-danger, #eb445a);
  white-space: pre-wrap;
  max-height: 80px;
  overflow-y: auto;
}
.back-transport-icon {
  font-size: 14px;
  width: 14px;
  height: 14px;
  color: var(--ion-color-primary, #3880ff);
}

/* ============ 翻转提示 ============ */
.flip-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  margin-top: auto;
  padding-top: 6px;
  font-size: 10px;
  color: var(--card-text-muted);
  opacity: 0.5;
  transition: opacity var(--transition-fast);
}
.card-3d-wrapper:hover .flip-hint { opacity: 0.9; }
.flip-hint-icon {
  font-size: 11px;
  width: 11px;
  height: 11px;
  animation: flip-hint-spin 2s linear infinite;
}
@keyframes flip-hint-spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}
.card-3d-wrapper.is-flipped .flip-hint-icon {
  animation: flip-hint-spin 2s linear infinite reverse;
}

/* ============ Compact mode ============ */
.card-3d-wrapper.is-compact .card-face {
  padding: 10px 12px;
  gap: 6px;
  min-height: 80px;
}
.card-3d-wrapper.is-compact .status-label { font-size: 13px; }
.card-3d-wrapper.is-compact .pulse-dot { width: 12px; height: 12px; }
.card-3d-wrapper.is-compact .detail-grid { display: none; }

/* ============ 响应式 ============ */
@media (max-width: 380px) {
  .detail-grid { grid-template-columns: 1fr; }
  .status-meta { flex-wrap: wrap; }
}

/* ============ 减弱动画（无障碍） ============ */
@media (prefers-reduced-motion: reduce) {
  .card-3d-inner { transition: transform 0.3s ease-out; }
  .pulse-dot::after { animation: none; }
  .detail-value.instance-changed,
  .back-value.monospace.instance-changed { animation: none; }
  .time-roll { animation: none; }
  .flip-hint-icon { animation: none; }
}
</style>
