<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.logSettings') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- 日志级别：preset cards + reset-to-default + synced badge -->
      <ion-list v-if="configLoaded">
        <ion-list-header>
          <ion-label>{{ t('devtools.exportLogsDesc') }}</ion-label>
          <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
            <ion-icon :icon="cloudOutline" class="scope-badge-icon"></ion-icon>
            <span class="scope-text">{{ t('settings.synced') }}</span>
          </ion-badge>
        </ion-list-header>
        <div v-if="logLevelField && logLevelField.selectOptions && logLevelField.selectOptions.length > 2" class="log-level-card">
          <div class="field-label-row">
            <ion-icon :icon="terminal" class="field-icon"></ion-icon>
            <span class="field-label-text">{{ tField('level') }}</span>
            <span class="required-mark">*</span>
            <ion-icon :icon="cloudOutline" class="sync-indicator" :title="t('settings.synced')"></ion-icon>
            <ion-button v-if="isLogLevelCustomized" fill="clear" size="small" class="reset-btn" @click="resetLogLevelToDefault">
              <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
            </ion-button>
          </div>
          <div class="preset-cards">
            <div
              v-for="opt in logLevelField.selectOptions"
              :key="opt.value"
              class="preset-card"
              :class="{ 'preset-card-active': logLevel === opt.value }"
              @click="handleLogLevelChange(opt.value)"
            >
              <div class="preset-card-title">{{ opt.label }}</div>
              <div v-if="opt.description" class="preset-card-desc">{{ opt.description }}</div>
            </div>
          </div>
        </div>
      </ion-list>

      <!-- 导出日志：使用 config.log.level 作为阈值过滤前端日志 + 走 Go 后端 zip 流程 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.exportLogs') }}</ion-label>
        </ion-list-header>
        <ion-item button @click="handleExportLogs" detail>
          <ion-icon :icon="downloadOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.exportLogs') }}</h3>
            <p>{{ t('devtools.exportLogsDesc') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 清空日志：弹窗确认 + clearLogs() plugin API -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.clearLogs') }}</ion-label>
        </ion-list-header>
        <ion-item button @click="handleClearLogs" detail>
          <ion-icon :icon="trashOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.clearLogs') }}</h3>
            <p>{{ t('devtools.clearLogsDesc') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonContent, IonList, IonListHeader, IonItem, IonIcon, IonLabel,
  IonButton, IonBadge, alertController,
} from '@ionic/vue'
import {
  downloadOutline, trashOutline, terminal,
  cloudOutline, refreshOutline,
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { useConfig } from '@/composables/useConfig'
import { getDefaultValue } from '@/config/schemaParser'
import { showToast } from '@/composables/useToast'
import { isNative, exportLogs, clearLogs, saveDevLogs } from '@/plugins/GoProcess'
import { useFrontendLogs, type LogEntry } from '@/composables/useFrontendLogs'

const { t, tField } = useI18n()
const { schemaFields, getFieldValue, setFieldValue, saveConfig, resetFieldToDefault } = useConfig()
const { logs: frontendLogs } = useFrontendLogs()

const configLoaded = computed(() => schemaFields.value.length > 0)

const logLevel = computed(() => String(getFieldValue(['log', 'level']) ?? 'info'))

const logLevelField = computed(() => {
  const logSection = schemaFields.value.find((s) => s.key === 'log')
  if (!logSection || !logSection.properties) return null
  return logSection.properties.find((p) => p.key === 'level') || null
})

const logDefault = computed(() => {
  if (!logLevelField.value) return 'info'
  return String(getDefaultValue(logLevelField.value))
})

const isLogLevelCustomized = computed(() => logLevel.value !== logDefault.value)

function resetLogLevelToDefault() {
  if (!logLevelField.value) return
  resetFieldToDefault(['log', 'level'], logLevelField.value)
  saveLogConfig()
}

async function handleLogLevelChange(value: string) {
  setFieldValue(['log', 'level'], value)
  await saveLogConfig()
}

async function saveLogConfig() {
  try {
    await saveConfig()
    showToast({ message: t('settings.configSaved'), duration: 1500, color: 'success' })
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e)
    showToast({ message: t('settings.configSaveFailed') + ': ' + detail, duration: 3000, color: 'danger' })
  }
}

/**
 * 🆕 2026-06-17：日志级别阈值过滤导出
 *
 * 为什么不直接传 getFrontendLogsJson()：
 *   - 前端内存日志没有阈值概念（hijackConsole 收集所有 console.*）
 *   - 用户期望"导出时使用日志级别设置"，即用持久化的 config.log.level
 *   - 这里过滤后传给 saveDevLogs，进入 zip 的 frontend_logs.json
 *
 * 为什么不过滤后端：
 *   - exportLogs() 是 Go 后端 plugin API
 *   - Go 后端启动时已按 config.log.level 作为 slog 阈值
 *   - logcat buffer 和 log file 输出已天然带阈值
 *   - 前端再过滤后端 = 重复劳动
 *
 * 为什么不读 DevLogs 的 selectedLevels：
 *   - 那是 DevLogs 页面组件本地的 UI 临时筛选状态
 *   - 非持久化、不影响 export
 *   - 用户明确要求"而不是 devlogs 页面的日志级别筛选"
 */
const LEVEL_RANK: Record<string, number> = { debug: 0, info: 1, warn: 2, error: 3 }

function rankOf(level: string): number {
  return LEVEL_RANK[level] ?? 1
}

async function handleExportLogs() {
  if (!isNative()) return
  try {
    const configuredLevel = String(getFieldValue(['log', 'level']) ?? 'info')
    const threshold = rankOf(configuredLevel)
    const filteredFrontendLogs: LogEntry[] = frontendLogs.value.filter(
      (l) => rankOf(l.level) >= threshold,
    )
    await saveDevLogs(JSON.stringify(filteredFrontendLogs, null, 2))
    const result = await exportLogs()
    if (result.success) {
      showToast({ message: t('devtools.exportSuccess'), duration: 1500, color: 'success' })
    } else {
      showToast({ message: t('devtools.exportFailed'), duration: 2000, color: 'danger' })
    }
  } catch {
    showToast({ message: t('devtools.exportFailed'), duration: 2000, color: 'danger' })
  }
}

async function handleClearLogs() {
  if (!isNative()) return
  const alert = await alertController.create({
    header: t('devtools.clearLogsConfirm'),
    buttons: [
      { text: t('common.cancel'), role: 'cancel' },
      {
        text: t('common.confirm'),
        role: 'confirm',
        handler: async () => {
          const result = await clearLogs()
          if (result.success) {
            showToast({ message: t('devtools.clearSuccess'), duration: 1500, color: 'success' })
          } else {
            showToast({ message: t('devtools.clearFailed'), duration: 2000, color: 'danger' })
          }
        },
      },
    ],
  })
  await alert.present()
}
</script>

<style scoped>
.scope-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 8px;
  --padding-top: 3px;
  --padding-bottom: 3px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
}
.scope-badge-icon {
  font-size: 12px;
}
.scope-synced {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  --color: var(--ion-color-primary);
}

.log-level-card {
  padding: 12px 16px;
  border-bottom: 1px solid var(--ion-color-light-shade, #e0e0e0);
}

body.dark .log-level-card {
  border-bottom-color: #2a2a2c;
}

.field-icon {
  font-size: 18px;
  color: var(--ion-color-medium);
  flex-shrink: 0;
}

.field-label-row {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.field-label-text {
  flex: 1 1 auto;
  min-width: 0;
  font-weight: 500;
  font-size: 15px;
}

.required-mark {
  color: var(--ion-color-danger);
  margin-left: 2px;
}

.sync-indicator {
  font-size: 12px;
  color: var(--ion-color-primary);
  opacity: 0.4;
  flex-shrink: 0;
}

.reset-btn {
  --padding-start: 4px;
  --padding-end: 4px;
  min-width: 28px;
  min-height: 28px;
  margin: 0;
}

.reset-btn ion-icon {
  font-size: 16px;
}

.preset-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(80px, 1fr));
  gap: 8px;
  margin-top: 10px;
  width: 100%;
}

.preset-card {
  padding: 10px 8px;
  border: 2px solid var(--ion-color-light-shade, #e0e0e0);
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: center;
  background: var(--ion-background-color, transparent);
}

.preset-card-active {
  border-color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}

.preset-card-title {
  font-weight: 600;
  font-size: 13px;
}

.preset-card-desc {
  font-size: 11px;
  color: var(--ion-color-medium);
  margin-top: 3px;
  line-height: 1.3;
}

@media (max-width: 599px) {
  .preset-cards {
    grid-template-columns: repeat(auto-fit, minmax(70px, 1fr));
    gap: 6px;
  }
  .preset-card-title {
    font-size: 12px;
  }
  .preset-card-desc {
    font-size: 10px;
  }
}
</style>

<style>
body.dark .preset-card {
  border-color: #3a3a3c;
}

body.dark .preset-card-active {
  border-color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.12);
}
</style>
