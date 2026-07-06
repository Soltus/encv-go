<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.serverTitle') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.connection') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="serverIcon" slot="start"></ion-icon>
          <ion-label class="ion-text-wrap">
            <h3>{{ t('settings.serverUrl') }}</h3>
            <p class="readonly-url" @click="copyToClipboard(serverUrl)">{{ serverUrl }}</p>
          </ion-label>
          <ion-button slot="end" fill="clear" size="small" @click="copyToClipboard(serverUrl)">
            <ion-icon :icon="copyIcon" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-item>
        <!-- 后端状态行：ServerStatusCard（操作按钮已内嵌到卡片内）
             点卡片空白 → 翻转看诊断；点按钮 → 触发对应 handler
             3D 实体化 + 高度自适应平滑伸缩在 ServerStatusCard 内部实现 -->
        <ServerStatusCard
          :clickable="true"
          :hide-actions="false"
          @check="checkServerInner"
          @restart="handleRestart"
          @stop="handleStop"
        />
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.serviceAddresses') }}</ion-label>
        </ion-list-header>
        <ion-item button @click="goHttpServer" detail>
          <ion-icon :icon="cloudOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.httpServer') }}</h3>
            <p>:{{ httpPort }} {{ rootDir }}</p>
          </ion-label>
        </ion-item>

        <ion-item button @click="goAdminServer" detail>
          <ion-icon :icon="shieldCheckmark" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.adminServer') }}</h3>
            <p>{{ adminConfigured ? t('settings.configured') : t('settings.notConfigured') }}</p>
          </ion-label>
        </ion-item>

        <ion-item button @click="goWebdavServer" detail>
          <ion-icon :icon="globeOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.webdavServer') }}</h3>
            <p>{{ webdavRoot }}{{ webdavUsername ? ' @' + webdavUsername : '' }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list v-if="isNativePlatform">
        <ion-list-header>
          <ion-label>{{ t('settings.permissions') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="notificationsIcon" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.notificationPermission') }}</h3>
            <p>{{ permNotifications ? t('settings.granted') : t('settings.denied') }}</p>
          </ion-label>
          <ion-button v-if="!permNotifications" fill="outline" size="small" @click="handleRequestNotification">
            {{ t('settings.request') }}
          </ion-button>
        </ion-item>
        <ion-item>
          <ion-icon :icon="folderOpen" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.storagePermission') }}</h3>
            <p>{{ permStorage ? t('settings.granted') : t('settings.denied') }}</p>
          </ion-label>
          <ion-button v-if="!permStorage" fill="outline" size="small" @click="handleRequestStorage">
            {{ t('settings.request') }}
          </ion-button>
        </ion-item>
        <ion-item>
          <ion-icon :icon="batteryOptimizationIcon" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.batteryOptimization') }}</h3>
            <p>{{ permBatteryOpt ? t('settings.granted') : t('settings.denied') }}</p>
          </ion-label>
          <ion-button v-if="!permBatteryOpt" fill="outline" size="small" @click="handleRequestBatteryOpt">
            {{ t('settings.request') }}
          </ion-button>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { fetchConfig, getServerUrl } from "@/api/encv";
import { copyToClipboard as clipboardWrite } from "@/composables/useClipboard";
import { useI18n } from "@/composables/useI18n";
import { useServerStatus } from "@/composables/useServerStatus";
import { showToast } from "@/composables/useToast";
import {
  checkPermissions,
  isNative,
  requestBatteryOptimization,
  requestNotificationPermission,
  requestStoragePermission,
} from "@/plugins/GoProcess";
import { alertController } from "@ionic/vue";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";

const configData = ref<Record<string, unknown> | null>(null);
const { isOnline: serverOnline, checkStatus, restartBackend, stopBackend } = useServerStatus();
const { t } = useI18n();

const _serverUrl = ref(getServerUrl());
const isNativePlatform = ref(isNative());
const permNotifications = ref(false);
const permStorage = ref(false);
const permBatteryOpt = ref(false);
let permissionCheckTimer: number | null = null;

const router = useRouter();

const _httpPort = computed(() => (configData.value?.server as Record<string, unknown>)?.port ?? "-");
const _rootDir = computed(() => (configData.value?.server as Record<string, unknown>)?.dir ?? "/");
const _adminConfigured = computed(() => !!(configData.value?.admin as Record<string, unknown>)?.password);
const _webdavRoot = computed(() => {
  const val = (configData.value?.webdav as Record<string, unknown>)?.root;
  return typeof val === "string" ? val : "/";
});
const _webdavUsername = computed(() => (configData.value?.webdav as Record<string, unknown>)?.username ?? "");

function _goHttpServer() {
  router.push("/tabs/settings/server/http");
}
function _goAdminServer() {
  router.push("/tabs/settings/server/admin");
}
function _goWebdavServer() {
  router.push("/tabs/settings/server/webdav");
}

async function _copyToClipboard(text: string) {
  const ok = await clipboardWrite(text);
  showToast({ message: ok ? t("remote.copied") : t("devlogs.copyFailed"), duration: 1000, color: ok ? "success" : "danger" });
}

async function refreshPermissions() {
  const perms = await checkPermissions();
  permNotifications.value = perms.notifications;
  permStorage.value = perms.storage;
  permBatteryOpt.value = perms.batteryOptimization;
}

async function _handleRequestNotification() {
  await requestNotificationPermission();
  if (permissionCheckTimer) clearTimeout(permissionCheckTimer);
  permissionCheckTimer = window.setTimeout(() => refreshPermissions(), 1000);
  setTimeout(() => refreshPermissions(), 3000);
  setTimeout(() => refreshPermissions(), 5000);
}

async function _handleRequestStorage() {
  await requestStoragePermission();
  if (permissionCheckTimer) clearTimeout(permissionCheckTimer);
  permissionCheckTimer = window.setTimeout(() => refreshPermissions(), 1000);
  setTimeout(() => refreshPermissions(), 3000);
  setTimeout(() => refreshPermissions(), 5000);
}

async function _handleRequestBatteryOpt() {
  await requestBatteryOptimization();
  if (permissionCheckTimer) clearTimeout(permissionCheckTimer);
  permissionCheckTimer = window.setTimeout(() => refreshPermissions(), 1000);
  setTimeout(() => refreshPermissions(), 3000);
  setTimeout(() => refreshPermissions(), 5000);
}

async function _checkServerInner() {
  // 刷新按钮：只 ping 一次后端 + 弹 toast
  await checkStatus();
  showToast({
    message: serverOnline.value ? t("settings.serverOnline") : t("settings.serverOffline"),
    duration: 1500,
    color: serverOnline.value ? "success" : "danger",
  });
}

async function _handleRestart() {
  showToast({
    message: t("settings.restarting"),
    duration: 30000,
  });
  const success = await restartBackend();
  showToast({
    message: success ? t("settings.restartSuccess") : t("settings.restartFailed"),
    duration: 2000,
    color: success ? "success" : "danger",
  });
}

async function _handleStop() {
  const alert = await alertController.create({
    header: t("settings.stopConfirm"),
    buttons: [
      { text: t("settings.cancel"), role: "cancel" },
      {
        text: t("settings.stop"),
        role: "destructive",
        handler: async () => {
          const success = await stopBackend();
          showToast({
            message: success ? t("settings.stopped") : t("settings.stopFailed"),
            duration: 2000,
            color: success ? "success" : "danger",
          });
        },
      },
    ],
  });
  await alert.present();
}

onMounted(async () => {
  if (isNativePlatform.value) {
    await refreshPermissions();
  }
  if (serverOnline.value) {
    try {
      configData.value = await fetchConfig();
    } catch {}
  }
});
</script>

<style scoped>
/* 状态行：ServerStatusCard 完整版（操作按钮内嵌在卡片内）
   卡片自身实现 3D 实体化 / 高度自适应 / 翻转动画 / 操作按钮
   ServerDetail 父级只负责传 @click / @check / @stop / @restart 监听 */

.connection-error {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin-top: 4px;
}
.port-info {
  font-size: 12px;
  opacity: 0.7;
  margin-left: 6px;
}
.latency-info {
  font-size: 12px;
  opacity: 0.7;
  margin-left: 2px;
  color: var(--ion-color-primary-shade);
}
.transport-info {
  font-size: 12px;
  margin-left: 2px;
  font-weight: 500;
}
.transport-ws {
  color: var(--ion-color-success-shade);
}
.transport-http-poll {
  color: var(--ion-color-warning-shade);
}
.transport-native-bridge {
  color: var(--ion-color-tertiary-shade);
}
.status-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
}
.status-meta {
  font-size: 11px;
  color: var(--ion-color-medium);
  margin-top: 4px;
}
.status-warning {
  font-size: 11px;
  color: var(--ion-color-warning-shade);
  background: var(--ion-color-warning-tint);
  padding: 4px 8px;
  border-radius: 4px;
  margin-top: 6px;
  line-height: 1.4;
}
.server-controls {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}
.readonly-url {
  font-family: 'Courier New', Courier, monospace;
  font-size: 13px;
  color: var(--ion-color-primary);
  word-break: break-all;
  cursor: pointer;
  user-select: all;
}
</style>
