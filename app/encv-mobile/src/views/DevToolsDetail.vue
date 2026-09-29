<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.title') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.debugTools') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="bugOutline" slot="start"></ion-icon>
          <ion-toggle :checked="vconsoleEnabled" @ionChange="handleVConsoleToggle">{{ t('devtools.vconsole') }}</ion-toggle>
        </ion-item>
        <!-- 🆕 2026-06-17：vConsole 之外的所有日志相关设置合并到「日志设置」三级页面 -->
        <ion-item button @click="goLogSettings" detail>
          <ion-icon :icon="terminal" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.logSettings') }}</h3>
            <p>{{ t('devtools.logSettingsDesc') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 自动化测试：生产构建也可访问（与沙箱预览的 isDev 限制不同） -->
      <ion-list>
        <!-- 🆕 2026-06-17：section header 变可点击入口，子项（plugin / webdav / sparse）整体搬到 AutomationTestsHub -->
        <ion-item button detail @click="goAutomationHub" class="section-entry">
          <ion-icon :icon="flaskOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.automationTests') }}</h3>
            <p>{{ t('devtools.automationTestsHint') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 容器预览页：资源由**后端**从可写数据目录提供，整包替换即可更新，不必换 APK -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.previewAssets') }}</ion-label>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.previewAssetsDesc') }}</p>
        <ion-item button detail @click="handleOpenPreviewAssets">
          <ion-icon :icon="eyeOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.previewAssetsOpen') }}</h3>
            <p>{{ t('devtools.previewAssetsOpenDesc') }}</p>
          </ion-label>
        </ion-item>
        <!-- 复制地址而不是"用浏览器打开"：真机上唤起外部浏览器时好时坏，
             失败就是"点了没反应"；复制出来粘到任意浏览器都能开 -->
        <ion-item button detail @click="handleCopyPreviewAssetsUrl">
          <ion-icon :icon="copyOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.previewAssetsCopyUrl') }}</h3>
            <p>{{ t('devtools.previewAssetsCopyUrlDesc') }}</p>
          </ion-label>
        </ion-item>
        <ion-item button detail @click="handleUpdatePreviewAssets" :disabled="previewBusy !== ''">
          <ion-icon :icon="cloudDownloadOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.previewAssetsUpdate') }}</h3>
            <p>{{ t('devtools.previewAssetsUpdateDesc') }}</p>
          </ion-label>
          <ion-spinner v-if="previewBusy === 'update'" slot="end" name="crescent"></ion-spinner>
        </ion-item>
        <ion-item button detail @click="handleImportPreviewAssets" :disabled="previewBusy !== ''">
          <ion-icon :icon="archiveOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.previewAssetsImport') }}</h3>
            <p>{{ t('devtools.previewAssetsImportDesc') }}</p>
          </ion-label>
          <ion-spinner v-if="previewBusy === 'import'" slot="end" name="crescent"></ion-spinner>
        </ion-item>
        <ion-item v-if="previewMessage">
          <ion-label class="ion-text-wrap">
            <p :style="{ color: previewOk ? 'var(--ion-color-success)' : 'var(--ion-color-danger)' }">
              {{ previewMessage }}
            </p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 沙箱预览：dev 专属入口，生产构建整段 v-if false 移除 -->
      <ion-list v-if="isDev">
        <ion-list-header>
          <ion-label>{{ t('devtools.sandboxPreview') }}</ion-label>
          <ion-badge slot="end" color="warning" class="scope-badge scope-dev">
            <ion-icon :icon="bugOutline" class="scope-badge-icon"></ion-icon>
            <span class="scope-text">DEV</span>
          </ion-badge>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.sandboxPreviewHint') }}</p>
        <ion-item button detail @click="openPreviewOpenList">
          <ion-icon :icon="eyeOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.previewOpenList') }}</h3>
            <p>{{ t('devtools.previewOpenListDesc') }}</p>
          </ion-label>
        </ion-item>
        <ion-item button detail @click="openPreviewOpenListPlugin">
          <ion-icon :icon="extensionPuzzleOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.previewOpenListLive') }}</h3>
            <p>{{ t('devtools.previewOpenListLiveDesc') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <!-- 🆕 2026-06-17：section header 变可点击入口，原型卡片循环整体搬到 ComposePrototypesHub -->
        <ion-item button detail @click="goComposePrototypesHub" class="section-entry">
          <ion-icon :icon="extensionPuzzleOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.composePrototypes') }}</h3>
            <p>{{ t('devtools.composePrototypesHint') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 插件生命周期状态：数据来自 /api/plugins 的 state/error/disposable 字段。
           用途是排障——「某个插件为什么不可用」过去只能翻后端日志，
           现在在开发者选项里直接可见（失败插件会带出 error 原因）。 -->
      <ion-list>
        <ion-list-header>
          <ion-label>插件状态</ion-label>
        </ion-list-header>
        <ion-item v-if="pluginLoadError">
          <ion-label color="danger">
            <h3>加载插件状态失败</h3>
            <p>{{ pluginLoadError }}</p>
          </ion-label>
        </ion-item>
        <ion-item v-for="p in plugins" :key="p.name">
          <ion-label>
            <h3>{{ p.name }}</h3>
            <p v-if="p.error" class="plugin-error">{{ p.error }}</p>
            <p v-else class="plugin-hint">{{ p.containerExtension }}</p>
          </ion-label>
          <ion-badge slot="end" :color="stateColor(p.state)">{{ p.state ?? "unknown" }}</ion-badge>
          <ion-note v-if="p.disposable" slot="end" class="plugin-disposable">可回收</ion-note>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>模拟世界</ion-label>
        </ion-list-header>
        <ion-item button detail @click="goChronicle">
          <ion-icon :icon="bookOutline" slot="start" color="tertiary"></ion-icon>
          <ion-label>
            <h3>编年史</h3>
            <p>世界历史事件时间线</p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  archiveOutline,
  bookOutline,
  bugOutline,
  cloudDownloadOutline,
  extensionPuzzleOutline,
  eyeOutline,
  copyOutline,
  flaskOutline,
  terminal,
} from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { fetchPlugins, type PluginMeta } from "@encv/shared-components/api/encv";
import { apiRequest } from "@encv/shared-components/api/core/request";
import { useDevTools } from "@encv/shared-components/composables/useDevTools";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";
import { copyPreviewAssetsUrl, openPreviewAssets, pickPreviewAssetsZip } from "@/plugins/GoProcess";

const { t } = useI18n();
const router = useRouter();
const { vconsoleEnabled, toggleVConsole } = useDevTools();

// 插件生命周期状态（pending/loading/active/failed/disposing/disposed）。
// 失败插件的 error 字段直接展示在列表里，省去翻后端日志。
const plugins = ref<PluginMeta[]>([]);
const pluginLoadError = ref("");

onMounted(async () => {
  try {
    plugins.value = await fetchPlugins();
  } catch (e) {
    pluginLoadError.value = e instanceof Error ? e.message : String(e);
  }
  // APK 内不自带预览页资源（这是热更新的前提），所以第一次进来要明说"还没装"，
  // 否则用户点「打开预览页」只会看到一个空页面。
  try {
    const v = await apiRequest<{ installed: boolean }>("/api/preview-assets/version");
    if (!v.installed) {
      previewOk.value = false;
      previewMessage.value = "预览页资源尚未安装（APK 内不自带）：请先「更新资源（远端）」或「导入资源包（zip）」";
    }
  } catch {
    // 查询失败不阻塞页面，用户点按钮时会拿到真实错误
  }
});

function stateColor(state: PluginMeta["state"]) {
  switch (state) {
    case "active":
      return "success";
    case "failed":
      return "danger";
    case "loading":
    case "disposing":
      return "warning";
    // pending / disposed / 未知状态一律中性色
    default:
      return "medium";
  }
}

function goLogSettings() {
  router.push("/tabs/settings/devtools/log-settings");
}

// 🆕 2026-06-17：自动化测试总览入口（原 section 内 3 个 ion-item 已整体搬到 AutomationTestsHub）
function goAutomationHub() {
  router.push("/tabs/settings/devtools/automation-hub");
}

// 🆕 2026-06-17：Compose UI 原型总览入口（原 prototype 卡片循环已整体搬到 ComposePrototypesHub）
function goComposePrototypesHub() {
  router.push("/tabs/settings/devtools/compose-prototypes-hub");
}

function goChronicle() {
  router.push("/tabs/settings/chronicle");
}

// ─────────── 容器预览页（资源由后端托管，可整包替换） ───────────
//
// 三个动作对应三种场景：
//   - 打开：独立全屏 Activity 加载 /preview-assets/（APK 里不带任何页面资源）
//   - 更新：按配置里的 preview.assets_url 拉最新 zip（不传 url 即走配置）
//   - 导入：用户在设备上选一个 zip（没有远端地址时也能装）
const previewBusy = ref<"" | "update" | "import">("");
const previewMessage = ref("");
const previewOk = ref(false);

async function handleOpenPreviewAssets() {
  const r = await openPreviewAssets();
  if (!r.opened) {
    // showToast 收的是 ToastOptions（{ message, color }），不是裸字符串
    await showToast({ message: `打开预览页失败：${r.error ?? "未知错误"}`, color: "danger" });
  }
}

async function handleCopyPreviewAssetsUrl() {
  const r = await copyPreviewAssetsUrl();
  await showToast({
    message: r.copied ? `已复制预览页地址：${r.url}` : `复制失败：${r.error ?? "未知错误"}`,
    color: r.copied ? "success" : "danger",
  });
}

async function handleUpdatePreviewAssets() {
  previewBusy.value = "update";
  previewMessage.value = "";
  try {
    // 不传 url：后端会用配置里的 preview.assets_url
    await apiRequest("/api/preview-assets/update", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    });
    previewOk.value = true;
    previewMessage.value = "预览页资源已更新";
  } catch (e) {
    previewOk.value = false;
    previewMessage.value = e instanceof Error ? e.message : String(e);
  } finally {
    previewBusy.value = "";
  }
}

async function handleImportPreviewAssets() {
  const picked = await pickPreviewAssetsZip();
  if (!picked.path) return; // 用户取消，不提示
  previewBusy.value = "import";
  previewMessage.value = "";
  try {
    await apiRequest("/api/preview-assets/import", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: picked.path }),
    });
    previewOk.value = true;
    previewMessage.value = `已导入 ${picked.name ?? picked.path}`;
  } catch (e) {
    previewOk.value = false;
    previewMessage.value = e instanceof Error ? e.message : String(e);
  } finally {
    previewBusy.value = "";
  }
}

// 沙箱预览：强制整页跳转，绕过 Vue Router 拦截
// 为什么不用 <router-link>：<router-link> 只走 in-app 路由，/openlist-ui/ 不在路由表
// 为什么不用 <a href>：Vue Router 4 在某些 setup 下会拦截 plain <a> 点击事件，
//   导致 router 试图导航到 /openlist-ui/ 失败、渲空 <ion-router-outlet>
// 为什么不用 window.open(_, '_blank')：会破坏 OpenPreview 会话（用户需手动切回 tab）
// 为什么用 window.location.assign：触发完整页面加载，浏览器原生处理同源跳转
const isDev = import.meta.env.DEV;
function openPreviewOpenList() {
  window.location.assign("/openlist-ui/");
}

// 跳 :5174 plugin-openlist 管理 UI（OpenListHome/Settings/ConfigEditor）
// 与现有 /openlist-ui/ 入口的区别：openlist-ui 是 dev 沙箱代理，plugin-openlist
// 是 Capacitor OpenList plugin 自身的管理 UI（独立前端，不在 encv-mobile 内）
//
// 为什么跳 /api/preview/plugin-openlist/（encv-go 后端相对路径）而不是 :5174：
// - 独立后端协调：encv-go 后端 reverse proxy 该路径到 127.0.0.1:5174
//   （见 internal/server/mobile_api.go handlePluginOpenlistProxyGin）
// - 不依赖 vite：vite.config.ts 的 openlist-ui-proxy 只能代理 :5244（OpenList 真实前端），
//   不能代理 :5174（plugin-openlist 是另一个独立 vite 进程）
// - 不依赖 OpenPreview 会话：跳相对路径不破坏当前 OpenPreview 工具锚定的 :5173
// - Capacitor native 端 127.0.0.1 指向设备本身，跳绝对 URL 不可达
//   走 encv-go 后端相对路径，由后端内部处理上游转发
// - 跟 openPreviewOpenList（/openlist-ui/）保持同一种风格：相对路径 + 整页跳转
function openPreviewOpenListPlugin() {
  window.location.assign("/api/preview/plugin-openlist/");
}

function handleVConsoleToggle(event: CustomEvent) {
  toggleVConsole(event.detail.checked);
}
</script>

<style scoped>
.section-hint {
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  margin: 0 16px 8px;
  line-height: 1.5;
}

.plugin-error {
  color: var(--ion-color-danger, #eb445a);
  white-space: normal;
}

.plugin-hint {
  font-size: 12px;
  opacity: 0.7;
}

.plugin-disposable {
  font-size: 11px;
  margin-inline-start: 6px;
}

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
  --background: color-mix(in srgb, var(--color-primary) 12%, transparent);
  --color: var(--color-primary);
}
.scope-dev {
  --background: color-mix(in srgb, var(--color-warning) 18%, transparent);
  --color: color-mix(in srgb, var(--color-warning) 85%, var(--color-black));
}
.scope-prod {
  --background: color-mix(in srgb, var(--color-success) 16%, transparent);
  --color: color-mix(in srgb, var(--color-success) 85%, var(--color-black));
}
</style>
