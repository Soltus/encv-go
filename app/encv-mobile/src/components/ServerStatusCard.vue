<!--
  ServerStatusCard.vue — 后端状态卡片（🆕 2026-06-15 彻底重构 v3）

  单一职责：**只回答一个问题**——后端是否在线？
  - 其它一切（version / instance_id / port / latency / transport / 上次检查时间）→ ServerSettings 详情页
  - 卡片**绝不再内嵌**多 pill / 多 grid / 复杂动画 / 横幅

  设计铁律（用户 2026-06-15 怒批后重设计）：
    1. 一眼看出状态（红/绿/黄 + 文字 + 原因）
    2. 没有 banner / 没有 pulse 动画 / 没有 monospace grid
    3. 没有「latency 96ms」「HTTP polling」之类需要解释才知道的元数据
    4. 点击进入详情页（如果有 clickable）

  使用：
    <ServerStatusCard :clickable="true" @click="goServerDetail" />
-->
<template>
  <button
    v-if="clickable"
    type="button"
    class="server-status-card clickable"
    :class="`is-${state}`"
    :aria-label="ariaLabel"
    @click="$emit('click')"
  >
    <span class="dot" :class="`is-${state}`" aria-hidden="true" />
    <span class="label">{{ stateText }}</span>
    <span v-if="reason" class="reason">{{ reason }}</span>
  </button>
  <div
    v-else
    class="server-status-card"
    :class="`is-${state}`"
    role="status"
    :aria-label="ariaLabel"
  >
    <span class="dot" :class="`is-${state}`" aria-hidden="true" />
    <span class="label">{{ stateText }}</span>
    <span v-if="reason" class="reason">{{ reason }}</span>
  </div>
</template>

<script setup lang="ts">
/**
 * 🆕 2026-06-15 v3 重构：
 *   旧版 700+ 行（含 pulse / banner / 4-pill grid / monospace / instance-changed 动画）
 *   改完还有 90+ 行：只保留 3 个 state + 1 个 reason + click
 *
 * 状态机：online | offline | checking
 * 原因：在线时无；离线时显示 lastError（带 ws/http/timeout 等短前缀）
 *        探测中显示「…」
 */
import { computed } from 'vue'
import { useI18n } from '@/composables/useI18n'
import { useServerStatus } from '@/composables/useServerStatus'

defineProps<{
  /** 卡片可点击（外层 router 跳转） */
  clickable?: boolean
}>()
defineEmits<{ (e: 'click'): void }>()

const { t } = useI18n()
const { isOnline, isRestarting, lastError, transportMode } = useServerStatus()

const state = computed<'online' | 'offline' | 'checking'>(() => {
  if (isRestarting.value) return 'checking'
  return isOnline.value ? 'online' : 'offline'
})

const stateText = computed(() => {
  switch (state.value) {
    case 'online':
      return t('serverStatus.online') || '在线'
    case 'offline':
      return t('serverStatus.offline') || '离线'
    case 'checking':
      return t('serverStatus.checking') || '检查中…'
  }
})

const reason = computed(() => {
  if (state.value === 'online') return ''
  if (state.value === 'checking') return ''
  // offline — 显示 lastError；如果 lastError 为空则补一个 transport 提示
  if (lastError.value) return lastError.value
  if (transportMode.value === 'http-poll') {
    return t('serverStatus.sandboxPollingHint') || '沙箱环境使用 HTTP 轮询'
  }
  return t('serverStatus.noDetail') || '无法连接后端'
})

const ariaLabel = computed(() => {
  const bits: string[] = [stateText.value]
  if (reason.value) bits.push(reason.value)
  return bits.join('，')
})
</script>

<style scoped>
/* ============================================================
   ServerStatusCard v3
   设计目标：1 行 + 1 圆点 + 1 文字 + 1 原因
   100% CSS variables — 0 硬编码颜色
   ============================================================ */
.server-status-card {
  /* Layout */
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 14px;
  border: 1px solid var(--ion-color-medium-shade, #74788c);
  border-left-width: 4px;
  border-radius: 8px;
  background: var(--ion-background-color, #fff);
  color: var(--ion-text-color, #000);
  font: inherit;
  text-align: left;
  cursor: default;
  transition: border-color 0.2s ease, background-color 0.2s ease;
}

/* 可点击态 */
.server-status-card.clickable {
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.server-status-card.clickable:hover {
  background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 6%, var(--ion-background-color, #fff));
}
.server-status-card.clickable:focus-visible {
  outline: 2px solid var(--ion-color-primary, #3880ff);
  outline-offset: 2px;
}

/* ============ 状态色（仅边框 + 圆点） ============ */
.server-status-card.is-online {
  border-left-color: var(--ion-color-success, #2dd55b);
  border-color: color-mix(in srgb, var(--ion-color-success, #2dd55b) 30%, var(--ion-color-medium-shade, #74788c));
}
.server-status-card.is-offline {
  border-left-color: var(--ion-color-danger, #eb445a);
  border-color: color-mix(in srgb, var(--ion-color-danger, #eb445a) 35%, var(--ion-color-medium-shade, #74788c));
}
.server-status-card.is-checking {
  border-left-color: var(--ion-color-warning, #ffc409);
  border-color: color-mix(in srgb, var(--ion-color-warning, #ffc409) 35%, var(--ion-color-medium-shade, #74788c));
}

/* ============ 圆点（静态、无 pulse 动画） ============ */
.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}
.dot.is-online {
  background: var(--ion-color-success, #2dd55b);
}
.dot.is-offline {
  background: var(--ion-color-danger, #eb445a);
}
.dot.is-checking {
  background: var(--ion-color-warning, #ffc409);
  opacity: 0.7;
}

/* ============ 文字 ============ */
.label {
  font-size: 14px;
  font-weight: 600;
  color: var(--ion-text-color, #000);
  white-space: nowrap;
}
.reason {
  font-size: 12px;
  color: color-mix(in srgb, var(--ion-text-color, #000) 60%, transparent);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ============ 减弱动画 ============ */
@media (prefers-reduced-motion: reduce) {
  .server-status-card {
    transition: none;
  }
}
</style>
