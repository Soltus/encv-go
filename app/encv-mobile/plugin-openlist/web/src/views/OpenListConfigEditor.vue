<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/home" />
        </ion-buttons>
        <ion-title>
          <span>config.json 编辑器</span>
          <span v-if="isDevPreview" class="preview-mini-chip">🔥 PREVIEW</span>
        </ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!--
        dev preview 模式防御性 UI：
        - 后端 config 需 admin auth，沙箱下不实现完整 auth flow
        - 提示用户去 :5244 backend 的 admin Web UI 编辑
        - 真机模式：仍然可读 / 写（gomobile 直接 file io）
      -->
      <div v-if="isDevPreview" class="dev-preview-banner">
        <div class="dev-preview-banner-row">
          <span class="banner-icon">🛠</span>
          <span class="banner-title">沙箱 Preview 模式</span>
        </div>
        <p class="dev-preview-text">
          config.json 写入需要 <code>admin</code> 鉴权，沙箱下不实现完整 auth 流程。
          <br />
          推荐：到 <code>:5244</code> 的 OpenList Admin Web UI 登录 <code>admin/admin</code> 编辑。
        </p>
        <ion-button
          expand="block"
          color="primary"
          class="ion-margin-top"
          @click="openAdminWebUi"
        >
          <ion-icon :icon="openOutline" slot="start" />
          打开 :5244 Admin Web UI
        </ion-button>
        <ion-button
          expand="block"
          color="medium"
          fill="outline"
          class="ion-margin-top"
          @click="loadAsReadOnly"
        >
          只读模式（GET /api/admin/config，需要 token）
        </ion-button>
      </div>

      <div class="editor-container" v-if="!isDevPreview || readOnlyMode">
        <textarea
          v-model="content"
          class="json-editor"
          :class="{ 'has-error': hasError, 'readonly': readOnlyMode }"
          spellcheck="false"
          :readonly="readOnlyMode"
          @input="onInput"
        ></textarea>
        <div v-if="hasError" class="error-text">
          JSON 错误：{{ error }}
        </div>
        <div v-else class="success-text">JSON 有效</div>
        <div v-if="readOnlyMode" class="readonly-banner">
          ⓘ 只读模式 — 沙箱 preview 下显示后端真实 config.json，不可保存
        </div>
      </div>
    </ion-content>

    <ion-footer v-if="!isDevPreview">
      <ion-toolbar>
        <ion-buttons slot="end">
          <ion-button @click="discard" color="medium">取消</ion-button>
          <ion-button @click="showSaveOptions" :disabled="hasError || isSaving">
            <ion-spinner v-if="isSaving" name="crescent" />
            <span v-else>保存</span>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-footer>
  </ion-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  IonPage,
  IonHeader,
  IonToolbar,
  IonButtons,
  IonButton,
  IonBackButton,
  IonTitle,
  IonContent,
  IonFooter,
  IonSpinner,
  IonIcon,
  modalController,
} from '@ionic/vue'
import { openOutline } from 'ionicons/icons'
import { OpenListNative, logBuffer } from '@/plugins/openlist-native'
import SaveOptionsDialog from '@/components/SaveOptionsDialog.vue'

const router = useRouter()

const content = ref('')
const hasError = ref(false)
const error = ref('')
const isSaving = ref(false)
const isDevPreview = ref(false)
const readOnlyMode = ref(false)
let debounceTimer: ReturnType<typeof setTimeout> | null = null

onMounted(async () => {
  isDevPreview.value = !window.OpenListNative
  if (isDevPreview.value) {
    // dev preview：不自动加载，避免无 token 报 401 弹错
    logBuffer.info('沙箱 preview 模式：config 需 admin token，请到 :5244 Admin Web UI 编辑')
    return
  }
  content.value = OpenListNative.readConfig()
  validate()
})

function onInput() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(validate, 300)
}

function validate() {
  if (!content.value.trim()) {
    hasError.value = true
    error.value = '内容为空'
    return
  }
  try {
    JSON.parse(content.value)
    hasError.value = false
    error.value = ''
  } catch (e: any) {
    hasError.value = true
    error.value = e.message
  }
}

async function showSaveOptions() {
  const modal = await modalController.create({
    component: SaveOptionsDialog,
  })
  await modal.present()
  const { data } = await modal.onDidDismiss()
  if (data === 'saveOnly' || data === 'saveAndRestart') {
    await doSave(data === 'saveAndRestart')
  }
}

async function doSave(restart: boolean) {
  isSaving.value = true
  try {
    const ok = OpenListNative.writeConfig(content.value)
    logBuffer[ok ? 'info' : 'error'](ok ? 'config.json 已保存' : '保存失败')
    if (ok && restart) {
      logBuffer.info('重启 OpenList...')
      OpenListNative.stopOpenList()
      setTimeout(() => {
        OpenListNative.startOpenList()
        router.back()
      }, 1500)
    } else if (ok) {
      router.back()
    }
  } catch (e: any) {
    logBuffer.error(`保存异常: ${e?.message || e}`)
  } finally {
    isSaving.value = false
  }
}

function discard() {
  router.back()
}

/**
 * dev preview 模式：以只读方式从 :5244 加载 config
 * OpenList admin API 实际路径是 /api/admin/setting/get（不是 /api/admin/config）
 * 失败时显示错误，但不让 UI 崩溃
 */
async function loadAsReadOnly() {
  readOnlyMode.value = true
  content.value = ''
  hasError.value = false
  error.value = ''

  const token = localStorage.getItem('openlist-token') || ''
  try {
    // OpenList v4 admin settings API: /api/admin/setting/list
    const res = await fetch('http://127.0.0.1:5244/api/admin/setting/list', {
      cache: 'no-store',
      signal: AbortSignal.timeout(5000),
      headers: token ? { Authorization: token } : {},
    })
    if (!res.ok) {
      const errBody = await res.text().catch(() => '')
      if (res.status === 401) {
        logBuffer.warn('admin setting 需要 token：先在 :5244 Admin Web UI 用 admin/admin 登录')
        error.value = '401 未授权（需要在 :5244 Admin Web UI 登录后把 token 粘到 localStorage）'
      } else {
        error.value = `HTTP ${res.status}: ${errBody.slice(0, 100)}`
      }
      hasError.value = true
      return
    }
    const data = await res.json()
    content.value = JSON.stringify(data?.data ?? data, null, 2)
    validate()
    logBuffer.info('已加载 :5244 admin setting (只读模式)')
  } catch (e: any) {
    error.value = e?.message || String(e)
    hasError.value = true
    logBuffer.error(`加载 admin setting 失败: ${error.value}`)
  }
}

function openAdminWebUi() {
  // 沙箱下直接打 :5244 backend 的 Web UI（dev preview 用户已经在 sandbox 内）
  // 真实 port 用 vite proxy rewrite 后打到 5244，但这里直接打开 5244 避开 vite proxy
  window.open('http://127.0.0.1:5244/#/login', '_blank', 'noopener')
}
</script>

<style scoped>
.editor-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 8px;
}
.json-editor {
  flex: 1;
  width: 100%;
  font-family: monospace;
  font-size: 12px;
  border: 1px solid var(--ion-color-medium);
  border-radius: 6px;
  padding: 8px;
  background: var(--ion-background-color, #1e1e1e);
  color: var(--ion-text-color);
  resize: none;
  outline: none;
}
.json-editor.has-error {
  border-color: var(--ion-color-danger);
}
.json-editor.readonly {
  background: rgba(0, 0, 0, 0.04);
  color: var(--ion-color-medium);
  cursor: not-allowed;
}
.error-text {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin-top: 4px;
  font-family: monospace;
}
.success-text {
  color: var(--ion-color-success);
  font-size: 12px;
  margin-top: 4px;
}
.readonly-banner {
  margin-top: 8px;
  padding: 8px 10px;
  font-size: 11px;
  color: var(--ion-color-medium);
  background: rgba(0, 0, 0, 0.05);
  border-radius: 4px;
}

/* === dev preview banner === */
.dev-preview-banner {
  margin: 12px;
  padding: 14px 16px;
  border-radius: 10px;
  background: linear-gradient(135deg,
    rgba(245, 158, 11, 0.12) 0%,
    rgba(239, 68, 68, 0.10) 100%);
  border: 1px solid rgba(245, 158, 11, 0.4);
  border-left: 4px solid #f59e0b;
}
.dev-preview-banner-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.banner-icon {
  font-size: 18px;
}
.banner-title {
  font-size: 13px;
  font-weight: 700;
  color: #f59e0b;
  letter-spacing: 0.5px;
}
.dev-preview-text {
  font-size: 12px;
  line-height: 1.6;
  color: var(--ion-text-color);
  margin: 0;
}
.dev-preview-text code {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
  padding: 1px 4px;
  background: rgba(0, 0, 0, 0.08);
  border-radius: 3px;
}
.preview-mini-chip {
  display: inline-block;
  margin-left: 8px;
  padding: 1px 6px;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.5px;
  color: #fff;
  background: linear-gradient(90deg, #f97316 0%, #ec4899 100%);
  border-radius: 3px;
  vertical-align: middle;
}
</style>
