<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.database') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <ion-list>
        <ion-list-header>
          <ion-label>当前状态</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-label>运行引擎</ion-label>
          <ion-note slot="end">
            <ion-badge :color="engineBadgeColor">
              {{ dbInfo?.engine || '—' }}
            </ion-badge>
          </ion-note>
        </ion-item>
        <ion-item>
          <ion-label>并发度</ion-label>
          <ion-note slot="end">{{ dbInfo?.concurrency || '—' }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>任务总数</ion-label>
          <ion-note slot="end">{{ dbInfo?.taskCount ?? '—' }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>校准数据</ion-label>
          <ion-note slot="end">{{ dbInfo?.hasCalibration ? '已校准' : '未校准' }}</ion-note>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>引擎切换</ion-label>
          <ion-badge slot="end" color="warning" class="scope-badge">
            <span class="scope-text">需重启</span>
          </ion-badge>
        </ion-list-header>
        <ion-item>
          <ion-select
            :value="configEngine"
            @ionChange="handleEngineChange"
            label="存储引擎"
            label-placement="stacked"
            interface="action-sheet"
            mode="ios"
          >
            <ion-select-option value="memory">内存模式（不持久化）</ion-select-option>
            <ion-select-option value="sqlite">SQLite（推荐，稳定可靠）</ion-select-option>
            <ion-select-option value="turso">Turso / LibSQL（高性能，MVCC 并发）</ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item v-if="configEngine !== dbInfo?.engine && dbInfo" class="engine-mismatch-item">
          <ion-label class="ion-text-wrap">
            <p class="mismatch-warning">
              <ion-icon :icon="warningOutline" class="warn-icon"></ion-icon>
              配置值「{{ configEngine }}」与当前运行引擎「{{ dbInfo.engine }}」不一致，
              重启应用后生效。
            </p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p class="ion-text-wrap" style="font-size: 0.8em; color: var(--ion-color-medium);">
              切换引擎后需要重启应用生效。数据不会自动迁移，
              请先导出数据，切换引擎后再导入。
            </p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>导入 / 导出</ion-label>
        </ion-list-header>
        <ion-item button @click="handleExportDatabase" :disabled="dbLoading">
          <ion-icon :icon="downloadOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>导出数据库</h3>
            <p>下载为 JSON 文件，可用于备份或迁移</p>
          </ion-label>
          <ion-spinner v-if="dbLoading" slot="end" name="crescent"></ion-spinner>
        </ion-item>
        <ion-item button @click="triggerImportFile" :disabled="dbLoading">
          <ion-icon :icon="cloudUploadOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>导入数据库</h3>
            <p>从 JSON 文件导入，将清空现有数据</p>
          </ion-label>
        </ion-item>
        <input
          ref="importFileInput"
          type="file"
          accept=".json"
          style="display: none"
          @change="handleImportFileSelected"
        />
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>本地备份</ion-label>
        </ion-list-header>
        <ion-item button @click="handleBackupDatabase" :disabled="dbLoading">
          <ion-icon :icon="saveOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>立即备份</h3>
            <p>备份到本地 .encv-backups 目录</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label color="danger">跨引擎迁移</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p class="ion-text-wrap" style="font-size: 0.85em; color: var(--ion-color-medium);">
              支持 SQLite ↔ Turso ↔ LibSQL 之间的数据迁移。
              迁移步骤：先从旧引擎导出 JSON → 切换到新引擎 → 导入 JSON。
            </p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonButton,
  IonContent, IonList, IonListHeader, IonItem, IonIcon, IonLabel,
  IonBadge, IonSpinner, IonNote, IonSelect, IonSelectOption,
  IonBackButton, alertController,
} from '@ionic/vue'
import {
  downloadOutline, cloudUploadOutline, saveOutline, warningOutline,
} from 'ionicons/icons'
import { useConfig } from '@/composables/useConfig'
import {
  getDatabaseInfo, exportDatabase, importDatabase, backupDatabase,
} from '@/api/encv'
import { showToast } from '@/composables/useToast'

const { t } = useI18n()
const { getFieldValue, setFieldValue, saveConfig } = useConfig()

const dbInfo = ref<any>(null)
const dbLoading = ref(false)
const importFileInput = ref<HTMLInputElement | null>(null)

const configEngine = computed(() => getFieldValue(['database', 'engine']) as string || 'memory')

const engineBadgeColor = computed(() => {
  const eng = dbInfo.value?.engine
  if (eng === 'turso' || eng === 'libsql') return 'success'
  if (eng === 'sqlite') return 'primary'
  return 'medium'
})

onMounted(() => {
  loadDatabaseInfo().catch(() => {})
})

async function loadDatabaseInfo() {
  try {
    dbInfo.value = await getDatabaseInfo()
  } catch (e) {
    console.warn('[DatabaseDetail] loadDatabaseInfo failed:', e)
  }
}

async function handleEngineChange(event: Event) {
  const target = event.target as HTMLIonSelectElement
  const newEngine = target.value as string
  setFieldValue(['database', 'engine'], newEngine)
  try {
    await saveConfig()
    showToast({ message: `已切换到 ${newEngine} 引擎，重启后生效`, color: 'warning' })
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    showToast({ message: '保存失败: ' + msg, color: 'danger' })
  }
}

async function handleExportDatabase() {
  try {
    dbLoading.value = true
    await exportDatabase()
    showToast({ message: '数据库导出成功', color: 'success' })
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e)
    showToast({ message: '导出失败: ' + msg, color: 'danger' })
  } finally {
    dbLoading.value = false
  }
}

function triggerImportFile() {
  importFileInput.value?.click()
}

async function handleImportFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  const confirmed = confirm('导入将清空所有现有数据，确定要继续吗？此操作不可撤销！')
  if (!confirmed) {
    input.value = ''
    return
  }

  try {
    dbLoading.value = true
    const result = await importDatabase(file)
    showToast({ message: `导入成功：${result.imported.tasks} 个任务`, color: 'success' })
    await loadDatabaseInfo()
    window.dispatchEvent(new CustomEvent('tasks:reload'))
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e)
    showToast({ message: '导入失败: ' + msg, color: 'danger' })
  } finally {
    dbLoading.value = false
    input.value = ''
  }
}

async function handleBackupDatabase() {
  try {
    dbLoading.value = true
    const result = await backupDatabase()
    showToast({ message: `备份成功：${result.name}`, color: 'success' })
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e)
    showToast({ message: '备份失败: ' + msg, color: 'danger' })
  } finally {
    dbLoading.value = false
  }
}
</script>

<style scoped>
.engine-mismatch-item {
  --background: var(--ion-color-warning-50, #fff8e1);
}

.mismatch-warning {
  font-size: 0.85em;
  color: var(--ion-color-warning);
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
}

.warn-icon {
  font-size: 1.1em;
  flex-shrink: 0;
}

.scope-badge {
  font-size: 0.7em;
}

.scope-text {
  font-weight: 500;
}
</style>
