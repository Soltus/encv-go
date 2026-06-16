<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.mountsTitle') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="openEditor(null)" :disabled="loading">
            <ion-icon :icon="addOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="mounts-content">
      <!-- 顶部说明卡 -->
      <div class="intro-card">
        <ion-icon :icon="serverOutline" class="intro-icon"></ion-icon>
        <div class="intro-text">
          <h4>{{ t('settings.mountsIntro') }}</h4>
          <p>{{ t('settings.mountsIntroHelp') }}</p>
        </div>
      </div>

      <!--
        🆕 2026-06-16：mount 启动期错误 banner（不再静默）
        历史：后端 MigrateFromServingDir 失败 → fmt.Fprintf(stderr) → DevLogs 看不到 → 用户不知道 mount 为何缺失
        现在：s.mountBootstrapErrors → /api/mounts 响应 bootstrap_errors → 这里顶部 banner 直接展示
      -->
      <div v-if="bootstrapErrors.length > 0" class="bootstrap-error-banner" role="alert">
        <div class="bootstrap-error-header">
          <ion-icon :icon="alertCircleOutline" color="danger" class="bootstrap-error-icon"></ion-icon>
          <div class="bootstrap-error-title-block">
            <div class="bootstrap-error-title">
              Mount 启动失败（{{ bootstrapErrors.length }}）
            </div>
            <div class="bootstrap-error-subtitle">
              mount registry 启动时遇到问题，下列挂载点可能未就绪
            </div>
          </div>
          <button class="bootstrap-error-close" @click="bootstrapErrors = []" aria-label="关闭">×</button>
        </div>
        <ul class="bootstrap-error-list">
          <li v-for="(msg, i) in bootstrapErrors" :key="i" class="bootstrap-error-item">
            <pre class="bootstrap-error-message">{{ msg }}</pre>
          </li>
        </ul>
      </div>

      <!--
        🆕 2026-06-16：操作错误 banner（create / update / delete / toggle 失败持久展示）
        历史：showToast 2.5s 一闪就消失 → 用户看不到根因
        现在：operationError 内联展示，附带 hint 提示排查
      -->
      <div v-if="operationError" class="bootstrap-error-banner" role="alert">
        <div class="bootstrap-error-header">
          <ion-icon :icon="alertCircleOutline" color="danger" class="bootstrap-error-icon"></ion-icon>
          <div class="bootstrap-error-title-block">
            <div class="bootstrap-error-title">
              {{ operationError.title }}
            </div>
            <div class="bootstrap-error-subtitle">
              {{ operationError.subtitle }}
            </div>
          </div>
          <button class="bootstrap-error-close" @click="operationError = null" aria-label="关闭">×</button>
        </div>
        <pre class="bootstrap-error-message">{{ operationError.message }}</pre>
        <div v-if="operationError.hint" class="bootstrap-error-hint">
          <strong>💡 排查:</strong> {{ operationError.hint }}
        </div>
      </div>

      <!-- 加载 / 错误状态 -->
      <div v-if="loading && mounts.length === 0" class="status-block">
        <ion-spinner name="crescent"></ion-spinner>
        <span>{{ t('common.loading') }}</span>
      </div>

      <div v-else-if="loadError" class="status-block status-block--error" role="alert">
        <ion-icon :icon="alertCircleOutline" color="danger"></ion-icon>
        <span>{{ loadError }}</span>
        <ion-button size="small" fill="outline" @click="loadAll">{{ t('common.retry') }}</ion-button>
      </div>

      <div v-else-if="mounts.length === 0" class="status-block status-block--empty">
        <ion-icon :icon="folderOpenOutline"></ion-icon>
        <span>{{ t('settings.mountsEmpty') }}</span>
      </div>

      <!-- 挂载点卡片列表 -->
      <div v-else class="mount-list">
        <article
          v-for="m in mounts"
          :key="m.id"
          class="mount-card"
          :class="{ 'mount-card--disabled': !m.enabled }"
        >
          <header class="mount-card-header">
            <div class="mount-identity">
              <div class="mount-name-row">
                <h3 class="mount-name">{{ m.name }}</h3>
                <ion-badge v-if="m.name === 'primary'" color="warning" class="preset-badge">
                  {{ t('settings.mountPreset') }}
                </ion-badge>
              </div>
              <code class="mount-path">{{ m.mount_path }}</code>
            </div>
            <div class="mount-flags">
              <ion-badge :color="driverColor(m.driver)">{{ m.driver }}</ion-badge>
              <ion-badge v-if="m.read_only" color="medium">RO</ion-badge>
              <ion-badge v-else color="success">RW</ion-badge>
            </div>
          </header>

          <dl class="mount-meta">
            <div class="meta-row">
              <dt>{{ t('settings.mountRootPath') }}</dt>
              <dd>
                <code class="root-path">{{ m.root_path || '—' }}</code>
              </dd>
            </div>
            <div v-if="m.driver_config && Object.keys(m.driver_config).length > 0" class="meta-row">
              <dt>{{ t('settings.mountDriverConfig') }}</dt>
              <dd>
                <pre class="driver-config">{{ formatDriverConfig(m.driver_config) }}</pre>
              </dd>
            </div>
          </dl>

          <footer class="mount-card-footer">
            <div class="toggle-group">
              <ion-toggle
                :checked="m.enabled"
                @ionChange="(e: CustomEvent) => handleToggleEnabled(m, e.detail.checked === true)"
                :disabled="togglingId === m.id"
              >
                {{ t('settings.mountEnabled') }}
              </ion-toggle>
            </div>
            <div class="action-group">
              <ion-button size="small" fill="outline" @click="openResolve(m)">
                <ion-icon :icon="searchOutline" slot="start"></ion-icon>
                {{ t('settings.mountResolve') }}
              </ion-button>
              <ion-button size="small" fill="outline" @click="openEditor(m)">
                <ion-icon :icon="createOutline" slot="start"></ion-icon>
                {{ t('common.edit') }}
              </ion-button>
              <ion-button
                size="small"
                fill="outline"
                color="danger"
                :disabled="m.name === 'primary'"
                @click="confirmDelete(m)"
              >
                <ion-icon :icon="trashOutline" slot="start"></ion-icon>
                {{ t('common.delete') }}
              </ion-button>
            </div>
          </footer>
        </article>
      </div>

      <!-- ============== Add/Edit Modal ============== -->
      <ion-modal :is-open="editorOpen" @didDismiss="closeEditor" :backdrop-dismiss="false">
        <ion-header>
          <ion-toolbar>
            <ion-title>
              {{ editing ? t('settings.mountEditTitle') : t('settings.mountAddTitle') }}
            </ion-title>
            <ion-buttons slot="end">
              <ion-button @click="closeEditor">{{ t('common.cancel') }}</ion-button>
              <ion-button :disabled="!canSave" @click="handleSave">
                {{ t('common.save') }}
              </ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content class="editor-content">
          <ion-list>
            <ion-item>
              <ion-input
                v-model="form.name"
                :label="t('settings.mountFieldName')"
                label-placement="stacked"
                :placeholder="t('settings.mountFieldNameHint')"
                :clear-input="true"
                autocapitalize="off"
                autocorrect="off"
                :spellcheck="false"
              ></ion-input>
            </ion-item>
            <ion-item>
              <ion-input
                v-model="form.mount_path"
                :label="t('settings.mountFieldMountPath')"
                label-placement="stacked"
                :placeholder="t('settings.mountFieldMountPathHint')"
                :clear-input="true"
                autocapitalize="off"
                autocorrect="off"
                :spellcheck="false"
              ></ion-input>
            </ion-item>
            <ion-item>
              <ion-select
                v-model="form.driver"
                :label="t('settings.mountFieldDriver')"
                label-placement="stacked"
                interface="action-sheet"
                mode="ios"
                :disabled="!!(editing && editing.name === 'primary')"
              >
                <ion-select-option v-for="d in availableDrivers" :key="d" :value="d">
                  {{ tDriver(d) }}
                </ion-select-option>
              </ion-select>
            </ion-item>
            <ion-item>
              <ion-toggle v-model="form.enabled">
                {{ t('settings.mountFieldEnabled') }}
              </ion-toggle>
            </ion-item>
            <ion-item>
              <ion-toggle v-model="form.read_only">
                {{ t('settings.mountFieldReadOnly') }}
              </ion-toggle>
            </ion-item>
          </ion-list>

          <div v-if="saveError" class="inline-error-card" role="alert">
            <ion-icon :icon="alertCircleOutline" color="danger"></ion-icon>
            <pre class="inline-error-message">{{ saveError }}</pre>
          </div>
        </ion-content>
      </ion-modal>

      <!-- ============== Resolve Modal (debug) ============== -->
      <ion-modal :is-open="resolveOpen" @didDismiss="closeResolve" :backdrop-dismiss="true">
        <ion-header>
          <ion-toolbar>
            <ion-title>{{ t('settings.mountResolveTitle') }}</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="closeResolve">{{ t('common.close') }}</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content class="resolve-content">
          <div class="resolve-intro">
            <p class="resolve-hint">
              {{ t('settings.mountResolveHint', { mountPath: resolving?.mount_path ?? '' }) }}
            </p>
          </div>
          <ion-list>
            <ion-item>
              <ion-input
                v-model="resolveSubPath"
                :label="t('settings.mountResolveSubPath')"
                label-placement="stacked"
                :placeholder="t('settings.mountResolveSubPathHint')"
                :clear-input="true"
                autocapitalize="off"
                autocorrect="off"
                :spellcheck="false"
                @keyup.enter="runResolve"
              ></ion-input>
            </ion-item>
            <ion-item lines="none">
              <ion-button :disabled="!resolveSubPath || resolving === null" @click="runResolve">
                <ion-icon :icon="searchOutline" slot="start"></ion-icon>
                {{ t('settings.mountResolveAction') }}
              </ion-button>
            </ion-item>
          </ion-list>

          <div v-if="resolveError" class="inline-error-card" role="alert">
            <ion-icon :icon="alertCircleOutline" color="danger"></ion-icon>
            <pre class="inline-error-message">{{ resolveError }}</pre>
          </div>

          <div v-if="resolveResult" class="resolve-result">
            <h4>{{ t('settings.mountResolveResultTitle') }}</h4>
            <dl class="resolve-dl">
              <div><dt>virtual_path</dt><dd><code>{{ resolveResult.virtual_path }}</code></dd></div>
              <div><dt>abs_path</dt><dd><code>{{ resolveResult.abs_path }}</code></dd></div>
              <div><dt>rel_path</dt><dd><code>{{ resolveResult.rel_path }}</code></dd></div>
              <div><dt>mount_name</dt><dd><code>{{ resolveResult.mount_name }}</code></dd></div>
            </dl>
          </div>
        </ion-content>
      </ion-modal>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonButton, IonBackButton,
  IonContent, IonList, IonItem, IonInput, IonSelect, IonSelectOption, IonToggle,
  IonBadge, IonIcon, IonSpinner, IonModal,
  alertController,
} from '@ionic/vue'
import {
  addOutline, alertCircleOutline, createOutline, folderOpenOutline,
  searchOutline, serverOutline, trashOutline,
} from 'ionicons/icons'
import {
  listMounts, createMount, updateMount, deleteMount, resolveMountPath,
  type Mount, type MountInput, type ResolveMountResponse,
  MOUNT_DRIVERS,
} from '@/api/encv'
import { useI18n } from '@/composables/useI18n'
import { showToast } from '@/composables/useToast'

const { t } = useI18n()

// ================== 状态 ==================
const mounts = ref<Mount[]>([])
const drivers = ref<string[]>([...MOUNT_DRIVERS])
const loading = ref(false)
const loadError = ref('')
const togglingId = ref<string | null>(null)

// 🆕 2026-06-16：mount 启动期错误（从 /api/mounts 响应 bootstrap_errors 字段读取，不再静默）
const bootstrapErrors = ref<string[]>([])

// 🆕 2026-06-16：操作错误内联 banner（create/update/delete/toggle 失败持久展示）
interface OperationError {
  title: string
  subtitle: string
  message: string
  hint?: string
}
const operationError = ref<OperationError | null>(null)

// Editor
const editorOpen = ref(false)
const editing = ref<Mount | null>(null)
const form = ref<MountInput>({
  name: '',
  mount_path: '',
  driver: 'local',
  enabled: true,
  read_only: false,
})
const saveError = ref('')

// Resolve
const resolveOpen = ref(false)
const resolving = ref<Mount | null>(null)
const resolveSubPath = ref('')
const resolveResult = ref<ResolveMountResponse | null>(null)
const resolveError = ref('')

// ================== 计算属性 ==================
const availableDrivers = computed(() => (drivers.value.length > 0 ? drivers.value : [...MOUNT_DRIVERS]))

const canSave = computed(() => {
  const f = form.value
  if (!f.name.trim()) return false
  if (!f.mount_path.trim()) return false
  if (!f.driver) return false
  if (!f.mount_path.startsWith('/')) return false
  return true
})

// ================== 数据加载 ==================
async function loadAll() {
  loading.value = true
  loadError.value = ''
  try {
    const resp = await listMounts()
    mounts.value = resp.mounts ?? []
    drivers.value = resp.drivers?.length ? resp.drivers : [...MOUNT_DRIVERS]
    // 🆕 2026-06-16：把后端返回的 mount 启动期错误推到 UI（不再静默）
    bootstrapErrors.value = resp.bootstrap_errors ?? []
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : String(e)
    mounts.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadAll()
})

// ================== 工具函数 ==================
function driverColor(d: string): string {
  if (d === 'local') return 'primary'
  if (d === 'appdata') return 'success'
  if (d === 'sandbox') return 'warning'
  return 'medium'
}

function tDriver(d: string): string {
  return t(`settings.mountDriver_${d}` as any, { default: d })
}

function formatDriverConfig(cfg: Record<string, unknown>): string {
  return Object.entries(cfg)
    .map(([k, v]) => `${k} = ${typeof v === 'string' ? v : JSON.stringify(v)}`)
    .join('\n')
}

// ================== Enabled 切换 ==================
async function handleToggleEnabled(m: Mount, enabled: boolean) {
  if (togglingId.value) return
  togglingId.value = m.id
  // 乐观更新
  const original = m.enabled
  m.enabled = enabled
  try {
    await updateMount(m.id, {
      name: m.name,
      mount_path: m.mount_path,
      driver: m.driver,
      enabled,
      read_only: m.read_only,
      driver_config: m.driver_config,
    })
    showToast({ message: t('settings.mountUpdated'), duration: 1500, color: 'success' })
  } catch (e) {
    // 回滚
    m.enabled = original
    const msg = e instanceof Error ? e.message : String(e)
    // 🆕 2026-06-16：内联错误 banner（持久展示，不再依赖 toast 一闪就消失）
    operationError.value = {
      title: '挂载点切换失败',
      subtitle: `挂载点 ${m.name} (${m.mount_path}) 启用状态切换失败`,
      message: msg,
      hint: '检查 root_path 目录是否存在 + 是否有写权限（RO 标志）。后端 mount.MountRegistry.Update 也会校验必填字段。',
    }
    showToast({ message: t('settings.mountUpdateFailed') + ': ' + msg, duration: 3000, color: 'danger' })
  } finally {
    togglingId.value = null
  }
}

// ================== Editor ==================
function openEditor(target: Mount | null) {
  editing.value = target
  if (target) {
    form.value = {
      name: target.name,
      mount_path: target.mount_path,
      driver: target.driver,
      enabled: target.enabled,
      read_only: target.read_only,
      driver_config: target.driver_config,
    }
  } else {
    form.value = {
      name: '',
      mount_path: '',
      driver: availableDrivers.value[0] ?? 'local',
      enabled: true,
      read_only: false,
    }
  }
  saveError.value = ''
  editorOpen.value = true
}

function closeEditor() {
  editorOpen.value = false
  editing.value = null
}

async function handleSave() {
  if (!canSave.value) return
  saveError.value = ''
  const input: MountInput = { ...form.value }
  try {
    if (editing.value) {
      await updateMount(editing.value.id, input)
      showToast({ message: t('settings.mountUpdated'), duration: 1500, color: 'success' })
    } else {
      await createMount(input)
      showToast({ message: t('settings.mountCreated'), duration: 1500, color: 'success' })
    }
    closeEditor()
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    saveError.value = msg
    // 🆕 2026-06-16：内联错误 banner（持久展示）
    // 关闭 modal 后用户仍能看到错误根因（toast 2.5s 后消失）
    operationError.value = {
      title: editing.value ? '挂载点更新失败' : '挂载点创建失败',
      subtitle: editing.value
        ? `挂载点 ${editing.value.name} (${editing.value.mount_path}) 更新失败`
        : `新建挂载点 ${input.name} (${input.mount_path}) 失败`,
      message: msg,
      hint: editing.value
        ? '检查 root_path 是否仍可写 + driver 工厂是否支持此 mount。primary mount 的 driver 字段不能改。'
        : '检查 mount_path 路径唯一性（primary/automation/sandbox 已存在）+ driver 名拼写（local/appdata/sandbox）',
    }
    showToast({ message: t('settings.mountSaveFailed') + ': ' + msg, duration: 3000, color: 'danger' })
  }
}

// ================== Delete ==================
async function confirmDelete(m: Mount) {
  if (m.name === 'primary') return
  const alert = await alertController.create({
    header: t('settings.mountDeleteTitle'),
    message: t('settings.mountDeleteConfirm', { name: m.name, path: m.mount_path }),
    buttons: [
      { text: t('common.cancel'), role: 'cancel' },
      {
        text: t('common.delete'),
        role: 'destructive',
        handler: async () => {
          try {
            await deleteMount(m.id)
            showToast({ message: t('settings.mountDeleted'), duration: 1500, color: 'success' })
            await loadAll()
          } catch (e) {
            const msg = e instanceof Error ? e.message : String(e)
            // 🆕 2026-06-16：内联错误 banner（持久展示）
            operationError.value = {
              title: '挂载点删除失败',
              subtitle: `挂载点 ${m.name} (${m.mount_path}) 删除失败`,
              message: msg,
              hint: 'primary mount 不可删（后端 ErrPrimaryProtected 保护）。其他 mount 删除会校验 root_path 引用计数。',
            }
            showToast({ message: t('settings.mountDeleteFailed') + ': ' + msg, duration: 3000, color: 'danger' })
          }
        },
      },
    ],
  })
  await alert.present()
}

// ================== Resolve (debug) ==================
function openResolve(m: Mount) {
  resolving.value = m
  resolveSubPath.value = ''
  resolveResult.value = null
  resolveError.value = ''
  resolveOpen.value = true
}

function closeResolve() {
  resolveOpen.value = false
  resolving.value = null
  resolveResult.value = null
  resolveError.value = ''
  resolveSubPath.value = ''
}

async function runResolve() {
  if (!resolving.value || !resolveSubPath.value) return
  resolveError.value = ''
  resolveResult.value = null
  try {
    resolveResult.value = await resolveMountPath(resolving.value.id, resolveSubPath.value)
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    resolveError.value = msg
  }
}
</script>

<style scoped>
.mounts-content {
  --background: var(--ion-background-color);
}

.intro-card {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin: 12px 16px 8px;
  padding: 14px 16px;
  background: rgba(var(--ion-color-primary-rgb), 0.06);
  border-radius: 12px;
  border-left: 3px solid var(--ion-color-primary);
}
.intro-icon {
  font-size: 24px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
  margin-top: 2px;
}
.intro-text h4 {
  margin: 0 0 4px;
  font-size: 15px;
  font-weight: 600;
}
.intro-text p {
  margin: 0;
  font-size: 13px;
  color: var(--ion-text-secondary);
  line-height: 1.5;
}

.status-block {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 48px 24px;
  color: var(--ion-text-secondary);
  text-align: center;
}
.status-block ion-icon {
  font-size: 36px;
  opacity: 0.6;
}
.status-block--error {
  color: var(--ion-color-danger);
}
.status-block--empty {
  font-style: italic;
  opacity: 0.7;
}

.mount-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 8px 16px 24px;
}

.mount-card {
  background: var(--ion-background-color);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.2);
  border-radius: 14px;
  padding: 14px 16px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: opacity 0.2s;
}
.mount-card--disabled {
  opacity: 0.55;
}

.mount-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.mount-identity {
  min-width: 0;
  flex: 1;
}
.mount-name-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.mount-name {
  margin: 0;
  font-size: 17px;
  font-weight: 600;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  word-break: break-all;
}
.preset-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
}
.mount-path {
  display: inline-block;
  margin-top: 4px;
  font-size: 12px;
  color: var(--ion-text-secondary);
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  word-break: break-all;
}
.mount-flags {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-end;
  flex-shrink: 0;
}

.mount-meta {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.meta-row {
  display: flex;
  gap: 8px;
  font-size: 12px;
  align-items: baseline;
}
.meta-row dt {
  flex: 0 0 70px;
  color: var(--ion-text-secondary);
  font-weight: 500;
}
.meta-row dd {
  margin: 0;
  flex: 1;
  min-width: 0;
  word-break: break-all;
}
.root-path {
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11.5px;
  color: var(--ion-text-color);
}
.driver-config {
  margin: 0;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  background: rgba(var(--ion-color-medium-rgb), 0.1);
  padding: 6px 8px;
  border-radius: 6px;
  white-space: pre-wrap;
  max-height: 80px;
  overflow-y: auto;
}

.mount-card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.15);
}
.toggle-group {
  display: flex;
  align-items: center;
}
.action-group {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.editor-content,
.resolve-content {
  --background: var(--ion-background-color);
}

.resolve-intro {
  padding: 12px 16px 0;
}
.resolve-hint {
  margin: 0;
  font-size: 13px;
  color: var(--ion-text-secondary);
  line-height: 1.5;
}

.resolve-result {
  margin: 16px;
  padding: 12px 16px;
  background: rgba(var(--ion-color-success-rgb), 0.06);
  border-radius: 10px;
  border-left: 3px solid var(--ion-color-success);
}
.resolve-result h4 {
  margin: 0 0 8px;
  font-size: 14px;
  font-weight: 600;
}
.resolve-dl {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.resolve-dl > div {
  display: flex;
  gap: 8px;
  font-size: 12px;
}
.resolve-dl dt {
  flex: 0 0 90px;
  color: var(--ion-text-secondary);
  font-family: 'SF Mono', Menlo, Consolas, monospace;
}
.resolve-dl dd {
  margin: 0;
  flex: 1;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  word-break: break-all;
}

.inline-error-card {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  margin: 12px 16px;
  padding: 10px 12px;
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  border-radius: 8px;
  border-left: 3px solid var(--ion-color-danger);
}
.inline-error-card ion-icon {
  flex-shrink: 0;
  font-size: 20px;
  margin-top: 2px;
}
.inline-error-message {
  margin: 0;
  font-size: 12px;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--ion-color-danger);
}

/* ========== 🆕 2026-06-16：mount 启动期错误 banner / 操作错误 banner ========== */
.bootstrap-error-banner {
  margin: 10px 16px 4px;
  padding: 12px 14px;
  background: linear-gradient(180deg, rgba(220, 38, 38, 0.10) 0%, rgba(220, 38, 38, 0.04) 100%);
  border-left: 4px solid var(--ion-color-danger);
  border-radius: 10px;
  box-shadow: 0 2px 6px rgba(220, 38, 38, 0.12);
}
.bootstrap-error-header {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
.bootstrap-error-icon {
  font-size: 24px;
  flex-shrink: 0;
  margin-top: 2px;
}
.bootstrap-error-title-block {
  flex: 1;
  min-width: 0;
}
.bootstrap-error-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--ion-color-danger-shade);
  line-height: 1.3;
}
.bootstrap-error-subtitle {
  font-size: 11.5px;
  color: var(--ion-color-medium);
  margin-top: 2px;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
}
.bootstrap-error-close {
  background: none;
  border: none;
  font-size: 24px;
  line-height: 1;
  color: var(--ion-color-medium);
  cursor: pointer;
  padding: 0 4px;
  flex-shrink: 0;
}
.bootstrap-error-close:hover {
  color: var(--ion-color-danger);
}
.bootstrap-error-list {
  list-style: none;
  margin: 10px 0 0 34px;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.bootstrap-error-item {
  margin: 0;
}
.bootstrap-error-message {
  margin: 10px 0 0 34px;
  padding: 10px 12px;
  background: rgba(0, 0, 0, 0.04);
  border-radius: 6px;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ion-color-dark);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 200px;
  overflow-y: auto;
}
.bootstrap-error-hint {
  margin: 10px 0 0 34px;
  padding: 8px 10px;
  background: rgba(59, 130, 246, 0.08);
  border-left: 3px solid var(--ion-color-primary);
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ion-color-dark);
}
</style>
