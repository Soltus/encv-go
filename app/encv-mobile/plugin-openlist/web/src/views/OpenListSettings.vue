<template>
  <SettingsPage :title="t('openlist.settings.title')" :show-back-button="true" @save="onSave" @reset="onReset">
    <div v-if="isDevPreview" class="preview-banner">
      <div class="preview-banner-row">
        <span class="preview-icon">🔥</span>
        <span class="preview-tag">{{ t('openlist.settings.previewBuild') }}</span>
        <span class="preview-tag-sub">{{ t('openlist.settings.sandboxDev') }}</span>
      </div>
      <div class="preview-banner-text">
        <strong>{{ t('openlist.settings.sandboxMode') }}</strong>，<code>window.OpenListNative</code> 不存在。
        <br />
        后端由 <code>/tmp/openlist</code> 独立进程提供（<code>:5244</code>），
        <strong>不能通过本界面启停</strong>（需在终端 <code>start-preview.sh</code> 控制）。
        <br />
        所有数据（密码、Config、版本）都通过 <code>http://127.0.0.1:5244/api/*</code> 直访 backend。
      </div>
    </div>

    <SettingsGroup :title="t('openlist.settings.basicInfo')">
      <SettingsItem :icon="informationCircleOutline" :title="t('openlist.settings.openlistVersion')">
        <template #default>
          <p class="version-line">
            <span v-if="loadingVersion" class="muted">{{ t('openlist.settings.probing') }}</span>
            <span v-else-if="versionError" class="error-text">
              ✗ {{ versionError }}
            </span>
            <span v-else>
              <span class="version-value">v{{ realVersion || version }}</span>
              <span v-if="isDevPreview" class="preview-chip">🔥 {{ t('openlist.home.preview') }}</span>
            </span>
          </p>
        </template>
      </SettingsItem>
      <SettingsItem :icon="folderOutline" :title="t('openlist.settings.dataDir')">
        <template #default>
          <p class="mono-text">{{ dataDir || t('openlist.settings.notConfigured') }}</p>
          <p v-if="isDevPreview" class="muted small">
            {{ t('openlist.settings.sandboxMode') }}：当前 <code>:5244</code> backend 数据目录由 <code>/tmp/openlist-data</code> 决定
          </p>
        </template>
      </SettingsItem>
      <SettingsItem :icon="rocketOutline" :title="t('openlist.settings.listenPort')">
        <template #default>
          <p>
            <span class="mono-text">{{ port || t('openlist.settings.unknown') }}</span>
            <span v-if="isBackendReachable" class="ok-text"> {{ t('openlist.home.online') }}</span>
            <span v-else class="error-text"> {{ t('openlist.home.offline') }}</span>
          </p>
          <p v-if="isDevPreview" class="muted small">
            {{ t('openlist.settings.sandboxMode') }}：health 由 <code>/__openlist-health</code> Node middleware 探测
          </p>
        </template>
      </SettingsItem>
    </SettingsGroup>

    <SettingsGroup :title="t('openlist.settings.actions')">
      <SettingsItem
        :icon="globeOutline"
        :title="t('openlist.settings.openWebUi')"
        description="http://127.0.0.1:{{ port || 5244 }}"
        :button="isBackendReachable"
        @click="openWebUi"
      />
      <SettingsItem
        :icon="homeOutline"
        :title="t('openlist.settings.backHome')"
        :description="t('openlist.settings.pluginUi')"
        button
        @click="goHome"
      />
      <SettingsItem
        v-if="isDevPreview"
        :icon="arrowBackOutline"
        :title="t('openlist.settings.backToEncv')"
        :description="t('openlist.settings.leavePreview')"
        button
        class="back-to-encv-item"
        @click="goBackToEncvMain"
      />
    </SettingsGroup>

    <SettingsGroup :title="t('settings.about')">
      <SettingsItem
        :icon="informationCircleOutline"
        :title="t('settings.about')"
        :description="`OpenList v${realVersion || version}`"
        button
        @click="goAbout"
      />
    </SettingsGroup>
  </SettingsPage>
</template>

<script setup lang="ts">
import SettingsGroup from "@encv/shared-components/components/settings/SettingsGroup.vue";
import SettingsItem from "@encv/shared-components/components/settings/SettingsItem.vue";
import SettingsPage from "@encv/shared-components/components/settings/SettingsPage.vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { arrowBackOutline, folderOutline, globeOutline, homeOutline, informationCircleOutline, rocketOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { logBuffer, OpenListNative } from "@/plugins/openlist-native";

const { t } = useI18n();

const router = useRouter();

const version = ref("unknown");
const dataDir = ref("");
const port = ref(0);

const isDevPreview = ref(false);
const loadingVersion = ref(false);
const realVersion = ref("");
const versionError = ref("");
const isBackendReachable = ref(false);

onMounted(async () => {
  isDevPreview.value = !window.OpenListNative;

  version.value = OpenListNative.getVersion();
  dataDir.value = OpenListNative.getDataDir();
  port.value = OpenListNative.getPort();

  if (isDevPreview.value) {
    await fetchRealVersion();
    await probeBackendHealth();
  }
});

function onSave() {
  logBuffer.info("保存设置");
}

function onReset() {
  logBuffer.info("重置设置");
}

/**
 * 从 :5244 /api/public/settings 拿真版本（无需 auth）
 * 失败时显示错误，不让 UI 一直 loading
 */
async function fetchRealVersion() {
  loadingVersion.value = true;
  versionError.value = "";
  try {
    // dev preview 下 axios 直接用 http://127.0.0.1:5244/api/*（直访，无 vite proxy）
    // 但 vite proxy 会被同源策略拦（vite 起在 5174，我们从 5174 fetch 自己）—— OK 同源
    const res = await fetch("http://127.0.0.1:5244/api/public/settings", {
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    });
    if (!res.ok) {
      versionError.value = `HTTP ${res.status}`;
      return;
    }
    const data = await res.json();
    if (data?.code === 200 && data?.data?.version) {
      realVersion.value = data.data.version;
    } else {
      versionError.value = "backend 返非预期格式";
    }
  } catch (e: any) {
    versionError.value = e?.message || String(e);
  } finally {
    loadingVersion.value = false;
  }
}

/**
 * 探测 :5244 backend 是否可达（用 vite Node middleware /__openlist-health）
 */
async function probeBackendHealth() {
  try {
    const res = await fetch("/__openlist-health", {
      cache: "no-store",
      signal: AbortSignal.timeout(3500),
    });
    if (!res.ok) {
      isBackendReachable.value = false;
      return;
    }
    const data = await res.json();
    isBackendReachable.value = !!data?.alive;
  } catch {
    isBackendReachable.value = false;
  }
}

function openWebUi() {
  window.open(`http://127.0.0.1:${port.value || 5244}/#/login`, "_blank", "noopener");
}

function goHome() {
  router.push("/home");
}

function goBackToEncvMain() {
  logBuffer.info("[OpenListSettings] goBackToEncvMain → /back-to-main");
  const hasRoute = router.getRoutes().some(r => r.path === "/back-to-main");
  if (hasRoute) {
    router.push("/back-to-main");
  } else {
    logBuffer.warn("[OpenListSettings] /back-to-main 未注册，fallback 直接跳 :5173");
    window.location.assign("http://127.0.0.1:5173/tabs/remote");
  }
}

function goAbout() {
  router.push("/settings/about");
}
</script>

<style scoped>
.mono-text {
  font-family: monospace;
  font-size: 12px;
  word-break: break-all;
}
.muted {
  color: var(--ion-color-medium);
  font-size: 11px;
}
.small {
  font-size: 11px;
}
.error-text {
  color: var(--ion-color-danger);
}
.ok-text {
  color: var(--ion-color-success);
}

/* === 沙箱 dev preview 横幅 === */
.preview-banner {
  margin: 12px;
  padding: 14px 16px;
  border-radius: 12px;
  background: linear-gradient(135deg,
    rgba(99, 102, 241, 0.18) 0%,
    rgba(168, 85, 247, 0.18) 50%,
    rgba(236, 72, 153, 0.18) 100%);
  border: 1px solid rgba(99, 102, 241, 0.4);
  box-shadow: 0 2px 12px rgba(99, 102, 241, 0.15);
  backdrop-filter: blur(4px);
}
.preview-banner-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.preview-icon {
  font-size: 22px;
  filter: drop-shadow(0 0 6px rgba(255, 150, 50, 0.6));
  animation: pulse-flame 1.8s ease-in-out infinite;
}
@keyframes pulse-flame {
  0%, 100% { transform: scale(1) rotate(-3deg); }
  50% { transform: scale(1.15) rotate(3deg); }
}
.preview-tag {
  display: inline-block;
  padding: 3px 10px;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 1.5px;
  color: var(--color-white);
  background: linear-gradient(135deg, #6366f1 0%, #a855f7 50%, #ec4899 100%);
  border-radius: 6px;
  box-shadow: 0 1px 4px rgba(168, 85, 247, 0.4);
}
.preview-tag-sub {
  font-size: 11px;
  color: var(--ion-color-medium);
}
.preview-banner-text {
  font-size: 12px;
  line-height: 1.65;
  color: var(--ion-text-color);
}
.preview-banner-text code {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
  padding: 1px 4px;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 3px;
}

/* === 版本行 + 炫酷 Preview 徽章 === */
.version-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.version-value {
  font-family: monospace;
  font-weight: 600;
}
.preview-chip {
  display: inline-block;
  padding: 2px 8px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 1px;
  color: var(--color-white);
  background: linear-gradient(90deg, #f97316 0%, #ef4444 50%, #ec4899 100%);
  border-radius: 4px;
  box-shadow: 0 0 8px rgba(239, 68, 68, 0.4);
  animation: pulse-glow 2s ease-in-out infinite;
}
@keyframes pulse-glow {
  0%, 100% { box-shadow: 0 0 4px rgba(239, 68, 68, 0.4); }
  50% { box-shadow: 0 0 12px rgba(239, 68, 68, 0.8); }
}
.preview-chip-sub {
  font-size: 10px;
  color: var(--ion-color-medium);
}

/* === 返回 ENCV 主页面按钮 === */
.back-to-encv-item {
  --background: rgba(99, 102, 241, 0.1);
  --border-color: rgba(99, 102, 241, 0.3);
  margin: 12px 0;
  border-radius: 8px;
}
.back-to-encv-label {
  color: var(--ion-color-primary);
  font-weight: 600;
}
</style>
