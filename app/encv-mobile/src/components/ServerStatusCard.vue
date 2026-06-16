<!--
  ServerStatusCard.vue — 后端状态卡片（🆕 2026-06-15 v2 还原 = 4-pill grid "银行卡通"）

  单一职责：**只回答一组问题**——后端健康度 + 关键事实
  设计铁律（用户 2026-06-15 怒批"我之前的新卡片"后重设计）：
    1. **4-pill grid 银行卡通**——不是单行 dot+label+reason
    2. **transport 旁带图标**：WebSocket ↔ pulse 图标 / HTTP Polling ↔ wifi 图标 / Native bridge ↔ swap 图标
    3. **0 banner 0 pulse 0 animation 0 旁注**——只有 4 个色块 pill
    4. **点整张卡跳详情页**（如果有 clickable）

  Pill 布局（2x2 网格）：
    ┌──────────────┬──────────────┐
    │ ●  在线      │ 📶 HTTP Poll │
    ├──────────────┼──────────────┤
    │ ⏱  42 ms     │ v2.3.0       │
    └──────────────┴──────────────┘

  使用：
    <ServerStatusCard :clickable="true" @click="goServerDetail" />
-->
<template>
  <component
    :is="clickable ? 'button' : 'div'"
    class="server-status-card"
    :class="[`is-${state}`, { clickable }]"
    :aria-label="ariaLabel"
    :type="clickable ? 'button' : undefined"
    @click="clickable && $emit('click', $event)"
  >
    <!-- Pill 1：状态 -->
    <div class="pill pill_state" :class="`is-${state}`">
      <div class="pillIcon" aria-hidden="true">
        <span class="stateDot" :class="`is-${state}`" />
      </div>
      <div class="pillText">
        <div class="pillLabel">{{ t('serverStatus.label.state') }}</div>
        <div class="pillValue">{{ stateText }}</div>
      </div>
    </div>

    <!-- Pill 2：传输（带图标） -->
    <div class="pill pill_transport" :class="`transport-${transportMode}`">
      <div class="pillIcon" aria-hidden="true">
        <ion-icon :icon="transportIcon" />
      </div>
      <div class="pillText">
        <div class="pillLabel">{{ t('serverStatus.label.transport') }}</div>
        <div class="pillValue">{{ transportText }}</div>
      </div>
    </div>

    <!-- Pill 3：延迟 -->
    <div class="pill pill_latency" :class="latencyClass">
      <div class="pillIcon" aria-hidden="true">
        <ion-icon :icon="timerIcon" />
      </div>
      <div class="pillText">
        <div class="pillLabel">{{ t('serverStatus.label.latency') }}</div>
        <div class="pillValue">{{ latencyText }}</div>
      </div>
    </div>

    <!-- Pill 4：版本 / 实例 ID -->
    <div class="pill pill_version">
      <div class="pillIcon" aria-hidden="true">
        <ion-icon :icon="pricetagIcon" />
      </div>
      <div class="pillText">
        <div class="pillLabel">{{ t('serverStatus.label.version') }}</div>
        <div class="pillValue factMono" :title="backendInstanceId">
          {{ versionText }}
        </div>
      </div>
    </div>
  </component>
</template>

<script setup lang="ts">
/**
 * 🆕 2026-06-15 v2 还原 = 4-pill grid "银行卡通"
 * 之前我误把它简化为 v3 单行 dot+label+reason，被用户怒批"丑死了"
 * 正确版本：4 个 pill 排成 2x2 网格，每 pill = icon + label + value
 * transport pill 旁带 wifi / pulse / swap 图标
 *
 * Pill 1: 状态（state dot + 在线/离线/检查中）
 * Pill 2: 传输（wifi/pulse/swap 图标 + HTTP Polling/WebSocket/Native bridge）
 * Pill 3: 延迟（timer 图标 + 42 ms / —）
 * Pill 4: 版本（pricetag 图标 + vX.Y.Z 或 instance id 前 8 位）
 */
import { computed } from 'vue'
import { IonIcon } from '@ionic/vue'
import {
  wifiOutline,        // HTTP Polling
  pulseOutline,       // WebSocket
  swapHorizontalOutline, // Native bridge
  timerOutline,       // Latency
  pricetagOutline,    // Version
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { useServerStatus } from '@/composables/useServerStatus'

defineProps<{
  /** 卡片可点击（外层 router 跳转） */
  clickable?: boolean
}>()
defineEmits<{ (e: 'click', ev: MouseEvent): void }>()

const { t } = useI18n()
const {
  isOnline, isRestarting, lastError, transportMode,
  latencyMs, backendVersion, backendInstanceId,
} = useServerStatus()

const state = computed<'online' | 'offline' | 'checking'>(() => {
  if (isRestarting.value) return 'checking'
  return isOnline.value ? 'online' : 'offline'
})

const stateText = computed(() => {
  switch (state.value) {
    case 'online': return t('serverStatus.online') || '在线'
    case 'offline': return t('serverStatus.offline') || '离线'
    case 'checking': return t('serverStatus.checking') || '检查中…'
  }
})

// Transport
const transportIcon = computed(() => {
  switch (transportMode.value) {
    case 'ws': return pulseOutline
    case 'http-poll': return wifiOutline
    case 'native-bridge': return swapHorizontalOutline
    default: return wifiOutline
  }
})

const transportText = computed(() => {
  switch (transportMode.value) {
    case 'ws': return 'WebSocket'
    case 'http-poll': return 'HTTP Polling'
    case 'native-bridge': return 'Native bridge'
    default: return '—'
  }
})

// Latency
const latencyText = computed(() => {
  if (state.value !== 'online') return '—'
  return latencyMs.value > 0 ? `${latencyMs.value} ms` : '—'
})

const latencyClass = computed(() => {
  if (state.value !== 'online') return 'is-idle'
  if (latencyMs.value <= 0) return 'is-idle'
  if (latencyMs.value < 100) return 'is-fast'
  if (latencyMs.value < 500) return 'is-ok'
  return 'is-slow'
})

// Version / instance id
const versionText = computed(() => {
  if (backendVersion.value) return backendVersion.value
  if (backendInstanceId.value) return backendInstanceId.value.slice(0, 8)
  return '—'
})

const ariaLabel = computed(() => {
  const parts = [stateText.value, transportText.value, latencyText.value, versionText.value]
  if (lastError.value) parts.push(lastError.value)
  return parts.filter(Boolean).join('，')
})

// 图标变量（模板引用）
const timerIcon = timerOutline
const pricetagIcon = pricetagOutline
</script>

<style scoped>
/* ============================================================
   ServerStatusCard v2 = 4-pill grid 银行卡通
   设计目标：
     · 2x2 grid 4 个 pill（icon + label + value）
     · 整张卡圆角、padding、内边距像银行卡
     · 0 动画 0 banner 0 旁注
     · transport pill 边色按 transport 类型变化
     · state pill dot 颜色按 state 变化
   100% CSS variables — 0 硬编码颜色
   ============================================================ */
.server-status-card {
  /* Bank card 外观 */
  display: grid;
  grid-template-columns: 1fr 1fr;
  grid-template-rows: auto auto;
  gap: 0;
  width: 100%;
  padding: 14px 16px;
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.18));
  border-left-width: 5px;
  border-radius: 12px;
  background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.04));
  color: var(--ion-text-color);
  font: inherit;
  text-align: left;
  cursor: default;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

/* 整体 state 边框色（左侧粗线 = 整体状态指示） */
.server-status-card.is-online { border-left-color: var(--ion-color-success, #2dd55b); }
.server-status-card.is-offline { border-left-color: var(--ion-color-danger, #eb445a); }
.server-status-card.is-checking { border-left-color: var(--ion-color-warning, #ffc409); }

/* 可点击态 */
.server-status-card.clickable {
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.server-status-card.clickable:hover {
  background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 6%, var(--encv-bg-elevated, #fff));
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.08);
}
.server-status-card.clickable:focus-visible {
  outline: 2px solid var(--ion-color-primary, #3880ff);
  outline-offset: 2px;
}

/* ============ Pill 单元（4 个 1:1 结构） ============ */
.pill {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  min-width: 0;
}

.pillIcon {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: var(--encv-bg-elevated-strong, rgba(127, 127, 127, 0.12));
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.85));
}
.pillIcon ion-icon { font-size: 18px; }
.pillText { display: flex; flex-direction: column; gap: 1px; min-width: 0; flex: 1; }
.pillLabel {
  font-size: 10.5px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  line-height: 1.1;
}
.pillValue {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--ion-text-color);
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.factMono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12.5px; }

/* ============ Pill 1：状态 ============ */
.pill_state .pillIcon { background: var(--ion-color-medium-tint, rgba(127, 127, 127, 0.15)); }
.pill_state .stateDot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--ion-color-medium);
}
.pill_state.is-online .pillIcon { background: color-mix(in srgb, var(--ion-color-success, #2dd55b) 20%, transparent); }
.pill_state.is-online .stateDot { background: var(--ion-color-success, #2dd55b); }
.pill_state.is-offline .pillIcon { background: color-mix(in srgb, var(--ion-color-danger, #eb445a) 20%, transparent); }
.pill_state.is-offline .stateDot { background: var(--ion-color-danger, #eb445a); }
.pill_state.is-checking .pillIcon { background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 20%, transparent); }
.pill_state.is-checking .stateDot { background: var(--ion-color-warning, #ffc409); opacity: 0.85; }

/* ============ Pill 2：传输（带图标） ============ */
.pill_transport.transport-ws .pillIcon { background: color-mix(in srgb, var(--ion-color-success, #2dd55b) 20%, transparent); color: var(--ion-color-success); }
.pill_transport.transport-http-poll .pillIcon { background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 22%, transparent); color: var(--ion-color-warning-shade, #b48a00); }
.pill_transport.transport-native-bridge .pillIcon { background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 20%, transparent); color: var(--ion-color-primary); }

/* ============ Pill 3：延迟 ============ */
.pill_latency.is-fast .pillIcon { background: color-mix(in srgb, var(--ion-color-success, #2dd55b) 20%, transparent); color: var(--ion-color-success); }
.pill_latency.is-ok .pillIcon { background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 20%, transparent); color: var(--ion-color-warning-shade, #b48a00); }
.pill_latency.is-slow .pillIcon { background: color-mix(in srgb, var(--ion-color-danger, #eb445a) 20%, transparent); color: var(--ion-color-danger); }

/* ============ 减弱动画 ============ */
@media (prefers-reduced-motion: reduce) {
  .server-status-card { transition: none; }
}
</style>
