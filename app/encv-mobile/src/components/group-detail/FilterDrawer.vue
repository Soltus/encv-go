<script setup lang="ts">
/**
 * 筛选 drawer（ion-modal 内嵌）
 *
 * 三个多选分组：状态 / 任务类型 / 插件
 * 双向绑定到父组件的数组（与 useTaskFilter 形状一致）
 */
import { computed } from 'vue'
import { useI18n } from '@/composables/useI18n'

interface Props {
  status: string[]
  taskType: string[]
  plugin: string[]
  availablePlugins: string[]
}
const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:status', v: string[]): void
  (e: 'update:taskType', v: string[]): void
  (e: 'update:plugin', v: string[]): void
  (e: 'reset'): void
  (e: 'apply'): void
}>()

const { t } = useI18n()

const STATUSES = ['pending', 'running', 'completed', 'failed', 'cancelled', 'cancelling']
const TASK_TYPES = ['encrypt', 'decrypt', 'move', 'copy', 'rename', 'delete']

function toggleArray(arr: string[], key: string, emitKey: 'status' | 'taskType' | 'plugin') {
  const idx = arr.indexOf(key)
  const next = idx === -1 ? [...arr, key] : arr.filter((k) => k !== key)
  emit(`update:${emitKey}` as any, next)
}

function isChecked(arr: string[], key: string): boolean {
  return arr.includes(key)
}

const hasAny = computed(() => props.status.length > 0 || props.taskType.length > 0 || props.plugin.length > 0)
</script>

<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button fill="clear" @click="emit('reset')">
            {{ t('tasks.filterReset') }}
          </ion-button>
        </ion-buttons>
        <ion-title>{{ t('tasks.filterTitle') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button fill="clear" strong @click="emit('apply')">
            {{ t('tasks.filterApply') }}
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding">
      <div class="filter-group">
        <h3 class="filter-group-title">{{ t('tasks.filterStatusTitle') }}</h3>
        <div class="filter-chips">
          <ion-chip
            v-for="s in STATUSES"
            :key="s"
            :color="isChecked(status, s) ? 'primary' : 'medium'"
            @click="toggleArray(status, s, 'status')"
          >
            <ion-icon v-if="isChecked(status, s)" :icon="'checkmark-circle'"></ion-icon>
            <ion-label>{{ t(`tasks.status.${s}`) }}</ion-label>
          </ion-chip>
        </div>
      </div>

      <div class="filter-group">
        <h3 class="filter-group-title">{{ t('tasks.filterTaskTypeTitle') }}</h3>
        <div class="filter-chips">
          <ion-chip
            v-for="tt in TASK_TYPES"
            :key="tt"
            :color="isChecked(taskType, tt) ? 'primary' : 'medium'"
            @click="toggleArray(taskType, tt, 'taskType')"
          >
            <ion-icon v-if="isChecked(taskType, tt)" :icon="'checkmark-circle'"></ion-icon>
            <ion-label>{{ t(`tasks.type.${tt}`) }}</ion-label>
          </ion-chip>
        </div>
      </div>

      <div class="filter-group" v-if="availablePlugins.length > 0">
        <h3 class="filter-group-title">{{ t('tasks.filterPluginTitle') }}</h3>
        <div class="filter-chips">
          <ion-chip
            v-for="p in availablePlugins"
            :key="p"
            :color="isChecked(plugin, p) ? 'primary' : 'medium'"
            @click="toggleArray(plugin, p, 'plugin')"
          >
            <ion-icon v-if="isChecked(plugin, p)" :icon="'checkmark-circle'"></ion-icon>
            <ion-label>{{ p }}</ion-label>
          </ion-chip>
        </div>
      </div>

      <div v-if="!hasAny" class="filter-empty">
        <ion-icon :icon="'funnel-outline'" size="large" color="medium"></ion-icon>
        <p>{{ t('tasks.filterEmpty') }}</p>
      </div>
    </ion-content>
  </ion-page>
</template>

<style scoped>
.filter-group {
  margin-bottom: 24px;
}
.filter-group-title {
  font-size: 14px;
  font-weight: 600;
  margin: 8px 0 12px 0;
  color: var(--ion-color-dark);
}
.filter-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.filter-chips ion-chip {
  margin: 0;
  cursor: pointer;
}
.filter-empty {
  text-align: center;
  padding: 48px 16px;
  color: var(--ion-color-medium);
}
.filter-empty p {
  margin-top: 8px;
  font-size: 13px;
}
</style>
