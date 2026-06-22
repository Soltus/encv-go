<script setup lang="ts">
/**
 * 🆕 2026-06-22 Q4：筛选 drawer（ion-modal 内嵌）
 *
 * 三个多选分组：状态 / 任务类型 / 插件
 * 双向绑定到父组件的 Set
 */
import { computed } from 'vue'
import { useI18n } from '@/composables/useI18n'

interface Props {
  status: Set<string>
  taskType: Set<string>
  plugin: Set<string>
  availablePlugins: string[]
}
const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:status', v: Set<string>): void
  (e: 'update:taskType', v: Set<string>): void
  (e: 'update:plugin', v: Set<string>): void
  (e: 'reset'): void
  (e: 'apply'): void
}>()

const { t } = useI18n()

// 6 个状态 + 12 种 taskType（6 主类型 + 6 rollback_*）
const STATUSES = ['pending', 'running', 'completed', 'failed', 'cancelled', 'cancelling']
const TASK_TYPES = ['encrypt', 'decrypt', 'move', 'copy', 'rename', 'delete']

function toggleSet(s: Set<string>, key: string, emitKey: 'status' | 'taskType' | 'plugin') {
  const next = new Set(s)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  emit(`update:${emitKey}` as any, next)
}

function isChecked(s: Set<string>, key: string): boolean {
  return s.has(key)
}

const hasAny = computed(() => props.status.size > 0 || props.taskType.size > 0 || props.plugin.size > 0)
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
      <!-- 状态 -->
      <div class="filter-group">
        <h3 class="filter-group-title">{{ t('tasks.filterStatusTitle') }}</h3>
        <div class="filter-chips">
          <ion-chip
            v-for="s in STATUSES"
            :key="s"
            :color="isChecked(status, s) ? 'primary' : 'medium'"
            @click="toggleSet(status, s, 'status')"
          >
            <ion-icon v-if="isChecked(status, s)" :icon="'checkmark-circle'"></ion-icon>
            <ion-label>{{ t(`tasks.status.${s}`) }}</ion-label>
          </ion-chip>
        </div>
      </div>

      <!-- 任务类型 -->
      <div class="filter-group">
        <h3 class="filter-group-title">{{ t('tasks.filterTaskTypeTitle') }}</h3>
        <div class="filter-chips">
          <ion-chip
            v-for="tt in TASK_TYPES"
            :key="tt"
            :color="isChecked(taskType, tt) ? 'primary' : 'medium'"
            @click="toggleSet(taskType, tt, 'taskType')"
          >
            <ion-icon v-if="isChecked(taskType, tt)" :icon="'checkmark-circle'"></ion-icon>
            <ion-label>{{ t(`tasks.type.${tt}`) }}</ion-label>
          </ion-chip>
        </div>
      </div>

      <!-- 插件 -->
      <div class="filter-group" v-if="availablePlugins.length > 0">
        <h3 class="filter-group-title">{{ t('tasks.filterPluginTitle') }}</h3>
        <div class="filter-chips">
          <ion-chip
            v-for="p in availablePlugins"
            :key="p"
            :color="isChecked(plugin, p) ? 'primary' : 'medium'"
            @click="toggleSet(plugin, p, 'plugin')"
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
