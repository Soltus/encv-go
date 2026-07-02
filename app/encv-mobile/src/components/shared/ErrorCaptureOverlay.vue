<template>
  <!--
    错误捕获浮窗（2026-07-02 A5 三管齐下）
    显示在屏幕底部（避开 tab bar），点击展开详情，点击关闭按钮消失
  -->
  <Transition name="error-fade">
    <div v-if="errorStore.showOverlay.value && errorStore.latestError.value" class="error-overlay" @click="onClick">
      <div class="error-overlay-header">
        <ion-icon :icon="alertCircle" class="error-overlay-icon" color="danger"></ion-icon>
        <span class="error-overlay-title">{{ title }}</span>
        <button class="error-overlay-close" type="button" @click.stop="errorStore.dismissOverlay()">×</button>
      </div>
      <div class="error-overlay-body">
        <code class="error-overlay-msg">{{ truncate(errorStore.latestError.value.message, 200) }}</code>
        <div v-if="errorStore.latestError.value.componentName" class="error-overlay-meta">
          组件: {{ errorStore.latestError.value.componentName }}
        </div>
        <div class="error-overlay-meta">
          路径: {{ errorStore.latestError.value.url || '/' }}
        </div>
        <div class="error-overlay-meta">
          来源: {{ sourceLabel }} · {{ formatTime(errorStore.latestError.value.timestamp) }}
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { alertCircle } from 'ionicons/icons'
import { errorStore } from '@/composables/useErrorCapture'

const title = computed(() => {
  const src = errorStore.latestError.value?.source
  if (src === 'vue') return 'Vue 渲染错误'
  if (src === 'promise') return '未捕获的异步错误'
  if (src === 'console') return '底层错误（控制台）'
  return '应用错误'
})

const sourceLabel = computed(() => {
  const src = errorStore.latestError.value?.source
  if (src === 'vue') return 'Vue'
  if (src === 'promise') return 'Promise'
  if (src === 'console') return 'console'
  return 'window'
})

function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, n) + '…' : s
}

function formatTime(ts: number) {
  const d = new Date(ts)
  return d.toLocaleTimeString()
}

function onClick() {
  // 点击展开：可以跳转到 DevTools → 错误查看器
  // 现在简化：仅 console.warn
  console.warn('[ErrorCapture] 点击错误卡片 → 查看完整堆栈:', errorStore.latestError.value)
}
</script>

<style scoped>
.error-overlay {
  position: fixed;
  left: 8px;
  right: 8px;
  bottom: 80px; /* 避开 tab bar */
  z-index: 9999;
  background: var(--ion-color-light, #ffffff);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.2);
  border-left: 4px solid var(--ion-color-danger, #eb445a);
  overflow: hidden;
  cursor: pointer;
  max-width: 600px;
  margin: 0 auto;
}

.error-overlay-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--ion-color-danger-tint, #fbd6d6);
  color: var(--ion-color-danger-shade, #b00020);
  font-weight: 600;
  font-size: 0.9em;
}

.error-overlay-icon {
  font-size: 1.2em;
}

.error-overlay-title {
  flex: 1;
}

.error-overlay-close {
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: inherit;
  font-size: 1.4em;
  line-height: 1;
  cursor: pointer;
}

.error-overlay-body {
  padding: 8px 12px;
  font-size: 0.85em;
  line-height: 1.4;
}

.error-overlay-msg {
  display: block;
  font-family: 'SFMono-Regular', Consolas, monospace;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--ion-text-color, #333);
}

.error-overlay-meta {
  margin-top: 4px;
  font-size: 0.8em;
  color: var(--ion-color-medium, #666);
}

.error-fade-enter-active,
.error-fade-leave-active {
  transition: opacity 0.2s, transform 0.2s;
}

.error-fade-enter-from,
.error-fade-leave-to {
  opacity: 0;
  transform: translateY(20px);
}
</style>
