<template>
  <div class="openlist-log-list">
    <div v-if="logs.length === 0" class="empty-state">暂无日志</div>
    <ion-list v-else class="log-list" ref="listRef">
      <ion-item v-for="(log, idx) in logs" :key="idx" :class="['log-item', `level-${log.level}`]">
        <span class="log-timestamp">{{ formatTime(log.timestamp) }}</span>
        <span class="log-level">{{ log.level.toUpperCase() }}</span>
        <span class="log-message">{{ log.message }}</span>
      </ion-item>
    </ion-list>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { IonList, IonItem } from '@ionic/vue'
import type { OpenListLog } from './index'

const props = defineProps<{
  logs: OpenListLog[]
  maxLength?: number
  autoScroll?: boolean
}>()

const listRef = ref<InstanceType<typeof IonList> | null>(null)
const maxLen = props.maxLength ?? 500
const autoScroll = props.autoScroll ?? true

watch(
  () => props.logs.length,
  async () => {
    if (autoScroll) {
      await nextTick()
      const el = listRef.value?.$el as HTMLElement | undefined
      el?.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
    }
  },
)

function formatTime(ts: number): string {
  const d = new Date(ts)
  return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}:${d.getSeconds().toString().padStart(2, '0')}`
}

// 修剪超出 maxLength 的日志
if (props.logs.length > maxLen) {
  props.logs.splice(0, props.logs.length - maxLen)
}
</script>

<style scoped>
.openlist-log-list {
  background: var(--ion-background-color, #1e1e1e);
  border-radius: 8px;
  overflow: hidden;
  margin: 8px 12px;
}
.empty-state {
  padding: 16px;
  text-align: center;
  font-size: 12px;
  color: var(--ion-color-medium);
}
.log-list {
  max-height: 300px;
  overflow-y: auto;
  background: transparent;
}
.log-item {
  --background: transparent;
  --border-color: rgba(255, 255, 255, 0.05);
  font-family: monospace;
  font-size: 12px;
  --padding-start: 8px;
  --padding-end: 8px;
  --inner-padding-end: 0;
}
.log-timestamp {
  color: var(--ion-color-medium);
  margin-right: 8px;
  font-size: 11px;
}
.log-level {
  font-weight: 700;
  margin-right: 8px;
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 3px;
}
.log-message {
  flex: 1;
  word-break: break-word;
  font-size: 12px;
}
.log-item.level-info .log-level { color: var(--ion-color-primary); }
.log-item.level-warn .log-level { color: var(--ion-color-warning); }
.log-item.level-error .log-level { color: var(--ion-color-danger); }
.log-item.level-debug .log-level { color: var(--ion-color-medium); }
</style>
