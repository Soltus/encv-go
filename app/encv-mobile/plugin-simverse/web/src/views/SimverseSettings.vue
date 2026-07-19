<template>
  <SettingsPage :title="t('simverse.settings')" :show-back-button="true" :loading="loading">
    <div v-if="loading" class="loading-wrap">
      <ion-spinner name="crescent"></ion-spinner>
      <p>{{ t('settings.loading') }}</p>
    </div>

    <template v-else>
      <div class="ui-card">
        <div class="p-3">
          <div class="ui-header mb-2">{{ t('simverse.performanceTier') }}</div>
          <ion-segment :value="worldConfig?.tier_name" @ionChange="onTierChange" class="w-full">
            <ion-segment-button value="background">
              <ion-label>Background</ion-label>
            </ion-segment-button>
            <ion-segment-button value="foreground">
              <ion-label>Foreground</ion-label>
            </ion-segment-button>
            <ion-segment-button value="fg_idle">
              <ion-label>Foreground Idle</ion-label>
            </ion-segment-button>
          </ion-segment>
          <div class="space-y-1 mt-3">
            <div class="flex items-center gap-2 p-2 rounded-lg">
              <span class="text-xs font-medium">Background</span>
              <span class="text-xs text-base-content/70">后台运行，低功耗</span>
            </div>
            <div class="flex items-center gap-2 p-2 rounded-lg">
              <span class="text-xs font-medium">Foreground</span>
              <span class="text-xs text-base-content/70">前台全速，高帧率</span>
            </div>
            <div class="flex items-center gap-2 p-2 rounded-lg">
              <span class="text-xs font-medium">Foreground Idle</span>
              <span class="text-xs text-base-content/70">前台空闲，平衡模式</span>
            </div>
          </div>
        </div>
      </div>

      <div class="ui-card">
        <div class="p-3">
          <div class="ui-header mb-2">世界配置</div>
          <div class="space-y-1">
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">事件速率倍率</span>
              <span class="text-xs text-base-content/70 font-mono">{{ worldConfig?.event_rate_mul }}x</span>
            </div>
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">缓存大小</span>
              <span class="text-xs text-base-content/70 font-mono">{{ worldConfig?.cache_size }}</span>
            </div>
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">子模拟</span>
              <span class="text-xs text-base-content/70 font-mono">{{ worldConfig?.sub_sim_active ? '已启用' : '未启用' }}</span>
            </div>
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">子模拟深度</span>
              <span class="text-xs text-base-content/70 font-mono">{{ worldConfig?.sub_sim_depth }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="ui-card">
        <div class="p-3">
          <div class="ui-header mb-2">世界状态</div>
          <div class="space-y-1">
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">运行状态</span>
              <span class="ui-chip" :class="isRunning ? '!bg-success/15 !text-success' : '!bg-base-300 !text-base-content/70'">
                {{ isRunning ? 'RUNNING' : 'PAUSED' }}
              </span>
            </div>
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">当前 Tick</span>
              <span class="text-xs text-base-content/70 font-mono">{{ currentTick }}</span>
            </div>
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">NPC 数量</span>
              <span class="text-xs text-base-content/70 font-mono">{{ worldState?.npc_count }}</span>
            </div>
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">Brain 数量</span>
              <span class="text-xs text-base-content/70 font-mono">{{ worldState?.brain_count }}</span>
            </div>
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">内存占用</span>
              <span class="text-xs text-base-content/70 font-mono">{{ totalMemoryMB?.toFixed(1) }} MB</span>
            </div>
          </div>
        </div>
      </div>

      <div class="ui-card">
        <div class="p-3">
          <div class="ui-header mb-2">存档管理</div>
          <div class="space-y-1">
            <div
              class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
              @click="handleSave"
            >
              <ion-icon :icon="saveOutline" class="text-primary text-xl" />
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium">保存存档</div>
                <p v-if="saveInfo?.saved_at" class="text-xs text-base-content/70 mt-0.5">上次保存: {{ formatDateTime(saveInfo.saved_at, { withSeconds: true }) }}</p>
                <p v-else class="text-xs text-base-content/70 mt-0.5">暂无存档</p>
              </div>
              <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
            </div>

            <div
              class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
              :class="{ 'opacity-50 pointer-events-none': !saveInfo?.has_save }"
              @click="handleLoad"
            >
              <ion-icon :icon="downloadOutline" class="text-success text-xl" />
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium">读取存档</div>
                <p v-if="saveInfo?.tick" class="text-xs text-base-content/70 mt-0.5">Tick {{ saveInfo.tick }} · {{ saveInfo.npc_count }} NPCs</p>
                <p v-else class="text-xs text-base-content/70 mt-0.5">暂无存档</p>
              </div>
              <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
            </div>
          </div>
        </div>
      </div>

      <div class="ui-card">
        <div class="p-3">
          <div class="ui-header mb-2">存储状态</div>
          <div class="space-y-2">
            <div class="flex items-center justify-between p-3">
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium">{{ formatBytes(storageStatus?.used_bytes || 0) }} / {{ formatBytes(storageStatus?.total_bytes || 0) }}</div>
                <div class="text-xs text-base-content/70 mt-0.5">可用空间: {{ formatBytes(storageStatus?.available_bytes || 0) }}</div>
              </div>
            </div>
            <div class="h-2 bg-base-300 rounded-full overflow-hidden mx-3">
              <div
                class="h-full transition-all"
                :class="storageColorClass"
                :style="{ width: `${storagePercent * 100}%` }"
              ></div>
            </div>
          </div>
        </div>
      </div>

      <div class="ui-card">
        <div class="p-3">
          <div class="ui-header mb-2">{{ t('settings.about') }}</div>
          <div class="space-y-1">
            <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
              <span class="text-sm font-medium">版本</span>
              <span class="text-xs text-base-content/70 font-mono">v0.1.0</span>
            </div>
            <div
              class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
              @click="refreshAll"
            >
              <ion-icon :icon="refreshOutline" class="text-primary text-xl" />
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium">刷新数据</div>
                <div class="text-xs text-base-content/70 mt-0.5">重新加载所有配置和状态</div>
              </div>
              <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
            </div>
            <div
              v-if="isNativePlugin"
              class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
              @click="handleShowDiagnostic"
            >
              <ion-icon :icon="bugOutline" class="text-warning text-xl" />
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium">{{ t('simverse.diagnosticTools') }}</div>
                <div class="text-xs text-base-content/70 mt-0.5">{{ t('simverse.diagnosticToolsDesc') }}</div>
              </div>
              <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
            </div>
            <div
              class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
              @click="handleOpenDevLogs"
            >
              <ion-icon :icon="documentTextOutline" class="text-tertiary text-xl" />
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium">开发者日志</div>
                <div class="text-xs text-base-content/70 mt-0.5">查看前端和后端日志</div>
              </div>
              <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
            </div>
          </div>
        </div>
      </div>
    </template>
  </SettingsPage>
</template>

<script setup lang="ts">
import SettingsPage from "@encv/shared-components/components/settings/SettingsPage.vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";
import { bugOutline, chevronForwardOutline, documentTextOutline, downloadOutline, refreshOutline, saveOutline } from "ionicons/icons";
import { IonIcon, IonLabel, IonSegment, IonSegmentButton, IonSpinner } from "@ionic/vue";
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseSaveInfo, type SimverseStorageStatus, useSimverse } from "@/composables/useSimverse";
import { isNativePluginMode, showDiagnosticPanel } from "@/plugins/SimVerse";
import { formatBytes } from "@encv/shared-components/lib/format";
import { formatDateTime } from "@encv/shared-components/composables/useDateFormat";

const { t } = useI18n();
const {
  worldState,
  worldConfig,
  isRunning,
  currentTick,
  totalMemoryMB,
  loadWorldState,
  loadWorldConfig,
  setPerformanceTier,
  loadSaveInfo,
  saveWorld,
  loadWorld: loadWorldFromSave,
  loadStorageStatus,
  init,
  cleanup,
} = useSimverse();

const loading = ref(true);
const saveInfo = ref<SimverseSaveInfo | null>(null);
const storageStatus = ref<SimverseStorageStatus | null>(null);

const storagePercent = ref(0);
const storageColor = ref("primary");

const storageColorClass = computed(() => {
  if (storagePercent.value > 0.9) return "bg-error";
  if (storagePercent.value > 0.7) return "bg-warning";
  return "bg-primary";
});

const isNativePlugin = isNativePluginMode();

const router = useRouter();

function handleOpenDevLogs() {
  router.push("/tabs/devlogs");
}

let pollInterval: number | null = null;

async function refreshAll() {
  loading.value = true;
  try {
    const [state, config, save, storage] = await Promise.all([loadWorldState(), loadWorldConfig(), loadSaveInfo(), loadStorageStatus()]);
    saveInfo.value = save;
    storageStatus.value = storage;
    updateStorageStatus();
  } catch (e) {
    showToast({ message: "加载失败: " + String(e), color: "danger" });
  } finally {
    loading.value = false;
  }
}

async function refreshState() {
  try {
    await loadWorldState();
  } catch {}
}

function updateStorageStatus() {
  if (!storageStatus.value?.total_bytes) return;
  const used = storageStatus.value.used_bytes || 0;
  const total = storageStatus.value.total_bytes;
  storagePercent.value = Math.min(used / total, 1);

  if (storagePercent.value > 0.9) {
    storageColor.value = "danger";
  } else if (storagePercent.value > 0.7) {
    storageColor.value = "warning";
  } else {
    storageColor.value = "primary";
  }
}

async function onTierChange(e: any) {
  const tier = e.detail.value;
  try {
    const result = await setPerformanceTier(tier);
    if (result) {
      showToast({ message: "性能等级已切换为 " + result.tier_name });
    }
  } catch (e) {
    showToast({ message: "切换失败: " + String(e), color: "danger" });
  }
}

async function handleSave() {
  try {
    const result = await saveWorld();
    if (result) {
      saveInfo.value = result;
      showToast({ message: "存档保存成功" });
    }
  } catch (e) {
    showToast({ message: "保存失败: " + String(e), color: "danger" });
  }
}

async function handleLoad() {
  try {
    const result = await loadWorldFromSave();
    if (result) {
      saveInfo.value = result;
      showToast({ message: "存档读取成功" });
      await refreshState();
    }
  } catch (e) {
    showToast({ message: "读取失败: " + String(e), color: "danger" });
  }
}

async function handleShowDiagnostic() {
  try {
    await showDiagnosticPanel();
  } catch (e) {
    showToast({ message: "打开诊断工具失败: " + String(e), color: "danger" });
  }
}

onMounted(async () => {
  await init();
  await refreshAll();
  pollInterval = window.setInterval(() => {
    refreshState();
  }, 3000);
});

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval);
  cleanup();
});
</script>

<style scoped lang="scss">
.loading-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: var(--color-base-content);
  opacity: 0.7;
  gap: 12px;

  p {
    margin: 0;
    font-size: 14px;
  }
}
</style>
