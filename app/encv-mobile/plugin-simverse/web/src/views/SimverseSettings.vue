<template>
  <SettingsPage :title="t('simverse.settings')" :show-back-button="true" :loading="loading">
    <div v-if="loading" class="loading-wrap">
      <ion-spinner name="crescent"></ion-spinner>
      <p>{{ t('settings.loading') }}</p>
    </div>

    <template v-else>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('simverse.performanceTier') }}</ion-label>
        </ion-list-header>

        <ion-radio-group :value="worldConfig?.tier_name" @ionChange="onTierChange">
          <ion-item>
            <ion-radio value="background">
              <div class="tier-option">
                <div class="tier-name">Background</div>
                <div class="tier-desc">后台运行，低功耗</div>
              </div>
            </ion-radio>
          </ion-item>
          <ion-item>
            <ion-radio value="foreground">
              <div class="tier-option">
                <div class="tier-name">Foreground</div>
                <div class="tier-desc">前台全速，高帧率</div>
              </div>
            </ion-radio>
          </ion-item>
          <ion-item>
            <ion-radio value="fg_idle">
              <div class="tier-option">
                <div class="tier-name">Foreground Idle</div>
                <div class="tier-desc">前台空闲，平衡模式</div>
              </div>
            </ion-radio>
          </ion-item>
        </ion-radio-group>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>世界配置</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-label>
            <h3>事件速率倍率</h3>
            <p>{{ worldConfig?.event_rate_mul }}x</p>
          </ion-label>
          <ion-note slot="end">{{ worldConfig?.event_rate_mul }}x</ion-note>
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>缓存大小</h3>
            <p>{{ worldConfig?.cache_size }}</p>
          </ion-label>
          <ion-note slot="end">{{ worldConfig?.cache_size }}</ion-note>
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>子模拟</h3>
            <p>{{ worldConfig?.sub_sim_active ? '已启用' : '未启用' }}</p>
          </ion-label>
          <ion-toggle :checked="worldConfig?.sub_sim_active" disabled slot="end" />
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>子模拟深度</h3>
            <p>{{ worldConfig?.sub_sim_depth }}</p>
          </ion-label>
          <ion-note slot="end">{{ worldConfig?.sub_sim_depth }}</ion-note>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>世界状态</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-label>
            <h3>运行状态</h3>
            <p>{{ isRunning ? '运行中' : '已暂停' }}</p>
          </ion-label>
          <ion-badge :color="isRunning ? 'success' : 'warning'" slot="end">
            {{ isRunning ? 'RUNNING' : 'PAUSED' }}
          </ion-badge>
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>当前 Tick</h3>
            <p>{{ currentTick }}</p>
          </ion-label>
          <ion-note slot="end">{{ currentTick }}</ion-note>
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>NPC 数量</h3>
            <p>{{ worldState?.npc_count }}</p>
          </ion-label>
          <ion-note slot="end">{{ worldState?.npc_count }}</ion-note>
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>Brain 数量</h3>
            <p>{{ worldState?.brain_count }}</p>
          </ion-label>
          <ion-note slot="end">{{ worldState?.brain_count }}</ion-note>
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>内存占用</h3>
            <p>{{ totalMemoryMB?.toFixed(1) }} MB</p>
          </ion-label>
          <ion-note slot="end">{{ totalMemoryMB?.toFixed(1) }} MB</ion-note>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>存档管理</ion-label>
        </ion-list-header>

        <ion-item button @click="handleSave">
          <ion-icon :icon="saveOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>保存存档</h3>
            <p v-if="saveInfo?.saved_at">上次保存: {{ formatDateTime(saveInfo.saved_at, { withSeconds: true }) }}</p>
            <p v-else>暂无存档</p>
          </ion-label>
          <ion-icon :icon="chevronForwardOutline" slot="end"></ion-icon>
        </ion-item>

        <ion-item button @click="handleLoad" :disabled="!saveInfo?.has_save">
          <ion-icon :icon="downloadOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>读取存档</h3>
            <p v-if="saveInfo?.tick">Tick {{ saveInfo.tick }} · {{ saveInfo.npc_count }} NPCs</p>
            <p v-else>暂无存档</p>
          </ion-label>
          <ion-icon :icon="chevronForwardOutline" slot="end"></ion-icon>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>存储状态</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-label>
            <h3>存储空间</h3>
            <p>{{ formatBytes(storageStatus?.used_bytes || 0) }} / {{ formatBytes(storageStatus?.total_bytes || 0) }}</p>
          </ion-label>
          <ion-progress-bar :value="storagePercent" :color="storageColor" slot="end" class="storage-bar" />
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>可用空间</h3>
            <p>{{ formatBytes(storageStatus?.available_bytes || 0) }}</p>
          </ion-label>
          <ion-note slot="end">{{ formatBytes(storageStatus?.available_bytes || 0) }}</ion-note>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.about') }}</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-label>
            <h3>版本</h3>
            <p>Simverse v0.1.0</p>
          </ion-label>
          <ion-note slot="end">v0.1.0</ion-note>
        </ion-item>

        <ion-item button @click="refreshAll">
          <ion-icon :icon="refreshOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>刷新数据</h3>
            <p>重新加载所有配置和状态</p>
          </ion-label>
          <ion-icon :icon="chevronForwardOutline" slot="end"></ion-icon>
        </ion-item>

        <ion-item button @click="handleShowDiagnostic" v-if="isNativePlugin">
          <ion-icon :icon="bugOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('simverse.diagnosticTools') }}</h3>
            <p>{{ t('simverse.diagnosticToolsDesc') }}</p>
          </ion-label>
          <ion-icon :icon="chevronForwardOutline" slot="end"></ion-icon>
        </ion-item>

        <ion-item button @click="handleOpenDevLogs">
          <ion-icon :icon="documentTextOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>开发者日志</h3>
            <p>查看前端和后端日志</p>
          </ion-label>
          <ion-icon :icon="chevronForwardOutline" slot="end"></ion-icon>
        </ion-item>
      </ion-list>
    </template>
  </SettingsPage>
</template>

<script setup lang="ts">
import SettingsPage from "@encv/shared-components/components/settings/SettingsPage.vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";
import { bugOutline, chevronForwardOutline, documentTextOutline, downloadOutline, refreshOutline, saveOutline } from "ionicons/icons";
import { IonBadge, IonItem, IonLabel, IonList, IonListHeader, IonNote, IonProgressBar, IonRadio, IonRadioGroup, IonSpinner } from "@ionic/vue";
import { onMounted, onUnmounted, ref } from "vue";
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

ion-list {
  margin-bottom: 20px;
}

.tier-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.tier-name {
  font-size: 15px;
  font-weight: 500;
  color: var(--color-base-content);
}

.tier-desc {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
}

.storage-bar {
  width: 100px;
  margin-inline-start: 12px;
}
</style>
