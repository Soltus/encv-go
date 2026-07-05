<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.sparseContainer.title') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- ① 真机降级提示 -->
      <div v-if="storageEstimate" class="storage-banner">
        <ion-icon :icon="informationCircleOutline" class="banner-icon"></ion-icon>
        <div class="banner-text">
          <div>
            <strong>{{ t('devtools.sparseContainer.quota') }}:</strong>
            {{ formatBytes(storageEstimate.quota) }} |
            <strong>{{ t('devtools.sparseContainer.used') }}:</strong>
            {{ formatBytes(storageEstimate.usage) }}
          </div>
          <div v-if="isHighRisk" class="warning-text">
            <ion-icon :icon="warningOutline" color="warning"></ion-icon>
            {{ t('devtools.sparseContainer.highRiskWarning') }}
          </div>
        </div>
      </div>

      <!-- ② 配置区 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.sparseContainer.config') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-label position="stacked">{{ t('devtools.sparseContainer.fragmentCount') }}</ion-label>
          <ion-input
            v-model="cfg.fragmentCount"
            type="number"
            inputmode="numeric"
            :placeholder="String(DEFAULT_FRAGMENT_COUNT)"
          ></ion-input>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">{{ t('devtools.sparseContainer.fragmentSizeGB') }}</ion-label>
          <ion-input
            v-model="cfg.fragmentSizeGB"
            type="number"
            inputmode="numeric"
            :placeholder="String(DEFAULT_FRAGMENT_SIZE_GB)"
          ></ion-input>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">{{ t('devtools.sparseContainer.physicalChunkMB') }}</ion-label>
          <ion-input
            v-model="cfg.physicalChunkMB"
            type="number"
            inputmode="numeric"
            :placeholder="String(DEFAULT_PHYSICAL_CHUNK_MB)"
          ></ion-input>
        </ion-item>
        <ion-item lines="none" class="hint-row">
          <small>{{ t('devtools.sparseContainer.physicalChunkMBHint') }}</small>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">{{ t('devtools.sparseContainer.outputDir') }}</ion-label>
          <ion-input
            v-model="cfg.outputDir"
            :placeholder="DEFAULT_OUTPUT_DIR"
          ></ion-input>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">Base name</ion-label>
          <ion-input
            v-model="cfg.baseName"
            placeholder="huge-100x128gb"
          ></ion-input>
        </ion-item>
      </ion-list>

      <!-- ③ 操作区 -->
      <ion-list>
        <ion-item button :disabled="isWriting" @click="handleWrite">
          <ion-icon :icon="createOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.write') }}</h3>
            <p>{{ t('devtools.sparseContainer.writeHint') }}</p>
          </ion-label>
          <ion-spinner v-if="isWriting" slot="end" name="dots"></ion-spinner>
        </ion-item>
        <ion-item button :disabled="!lastResult || isProbing" @click="handleProbe">
          <ion-icon :icon="searchOutline" slot="start" color="secondary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.probe') }}</h3>
            <p>{{ t('devtools.sparseContainer.probeHint') }}</p>
          </ion-label>
          <ion-spinner v-if="isProbing" slot="end" name="dots"></ion-spinner>
        </ion-item>
        <ion-item button :disabled="!lastResult || isCleaning" @click="handleCleanup">
          <ion-icon :icon="trashOutline" slot="start" color="danger"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.cleanup') }}</h3>
            <p>{{ t('devtools.sparseContainer.cleanupHint') }}</p>
          </ion-label>
          <ion-spinner v-if="isCleaning" slot="end" name="dots"></ion-spinner>
        </ion-item>
      </ion-list>

      <!-- ④ 结果区（声称 vs 物理） -->
      <ion-list v-if="lastResult">
        <ion-list-header>
          <ion-label>{{ t('devtools.sparseContainer.lastResult') }}</ion-label>
          <ion-badge slot="end" :color="lastResult.isSparse ? 'success' : 'danger'">
            {{ lastResult.isSparse ? 'SPARSE' : 'NON-SPARSE' }}
          </ion-badge>
        </ion-list-header>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.virtualTotal') }}</h3>
            <p>
              {{ formatBytes(lastResult.virtualTotalBytes) }}
              ({{ lastResult.fragmentCount }} × {{ formatBytes(lastResult.fragmentSizeBytes) }})
            </p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.physicalMain') }}</h3>
            <p>{{ formatBytes(lastResult.physicalMainBytes) }} (apparent size)</p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.physicalUsed') }}</h3>
            <p>{{ formatBytes(lastResult.physicalUsedBytes) }} (du/blocks actual)</p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.manifestSize') }}</h3>
            <p>{{ formatBytes(lastResult.manifestSizeBytes) }}</p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.sparseRatio') }}</h3>
            <p>{{ sparseRatioText }}</p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.duration') }}</h3>
            <p>{{ lastResult.durationMs }} ms</p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.mainFilePath') }}</h3>
            <p><code>{{ lastResult.mainFilePath }}</code></p>
          </ion-label>
        </ion-item>
        <ion-item v-if="lastResult.partFilePattern" lines="none">
          <ion-label>
            <h3>Part files</h3>
            <p><code>{{ lastResult.partFilePattern }}</code></p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- ⑤ probe 结果 -->
      <ion-list v-if="lastProbe">
        <ion-list-header>
          <ion-label>{{ t('devtools.sparseContainer.probeResult') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.bytesRead') }}</h3>
            <p>{{ formatBytes(lastProbe.bytesRead) }} in {{ lastProbe.durationMs }} ms</p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.heapInUse') }}</h3>
            <p>{{ lastProbe.heapInUseKB }} KB</p>
          </ion-label>
        </ion-item>
        <ion-item lines="none">
          <ion-label>
            <h3>Seek / Read</h3>
            <p>{{ lastProbe.seekMs }} ms / {{ lastProbe.readMs }} ms</p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { alertController } from "@ionic/vue";
import { computed, onMounted, ref } from "vue";
import {
  cleanupSparseContainer,
  probeSparseContainer,
  type SparseContainerProbeResponse,
  type SparseContainerResponse,
  writeSparseContainer,
} from "@/api/sparseContainer";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import { isNative } from "@/plugins/GoProcess";

const { t } = useI18n();

// 默认值与 Go testutil.DefaultSparseConfig 对齐
// 🆕 2026-06-11 v3 修正：物理分片 = 128GB (131072 MB) — 真机 ECv4 实际场景
//   沙箱仍可用 0 (sparse main file) 验证，但真机要测的就是 100 个 128G .part 物理分片
const DEFAULT_FRAGMENT_COUNT = 100;
const DEFAULT_FRAGMENT_SIZE_GB = 128;
const DEFAULT_PHYSICAL_CHUNK_MB = 131072; // 128GB = 128 × 1024 MB
const DEFAULT_OUTPUT_DIR = "/tmp/encv-sparse-test";

const cfg = ref({
  fragmentCount: DEFAULT_FRAGMENT_COUNT,
  fragmentSizeGB: DEFAULT_FRAGMENT_SIZE_GB,
  physicalChunkMB: DEFAULT_PHYSICAL_CHUNK_MB,
  outputDir: DEFAULT_OUTPUT_DIR,
  baseName: "huge-100x128gb",
});

const isWriting = ref(false);
const isProbing = ref(false);
const isCleaning = ref(false);
const lastResult = ref<SparseContainerResponse | null>(null);
const lastProbe = ref<SparseContainerProbeResponse | null>(null);
const storageEstimate = ref<{ quota: number; usage: number } | null>(null);

const proposedBytes = computed(() => Number(cfg.value.fragmentCount || 0) * Number(cfg.value.fragmentSizeGB || 0) * 1024 ** 3);

const isHighRisk = computed(() => {
  // 浏览器 / Capacitor web fallback 都没有 quota 字段：> 1TB 视为高风险
  if (!storageEstimate.value?.quota) {
    return proposedBytes.value > 1024 ** 4;
  }
  return proposedBytes.value > storageEstimate.value.quota * 0.5;
});

const _sparseRatioText = computed(() => {
  if (!lastResult.value) return "";
  const { virtualTotalBytes, physicalUsedBytes } = lastResult.value;
  if (physicalUsedBytes === 0) return `∞ (${formatBytes(virtualTotalBytes)} / 0 B)`;
  const ratio = virtualTotalBytes / physicalUsedBytes;
  const left = formatBytes(virtualTotalBytes);
  const right = formatBytes(physicalUsedBytes);
  if (ratio > 1e6) return `${(ratio / 1e6).toFixed(2)}M× (${left} / ${right})`;
  if (ratio > 1e3) return `${(ratio / 1e3).toFixed(2)}K× (${left} / ${right})`;
  return `${ratio.toFixed(2)}× (${left} / ${right})`;
});

function formatBytes(n: number | string | undefined | null): string {
  const v = Number(n);
  if (!Number.isFinite(v) || v < 0) return "?";
  if (v >= 1024 ** 4) return `${(v / 1024 ** 4).toFixed(2)} TB`;
  if (v >= 1024 ** 3) return `${(v / 1024 ** 3).toFixed(2)} GB`;
  if (v >= 1024 ** 2) return `${(v / 1024 ** 2).toFixed(2)} MB`;
  if (v >= 1024) return `${(v / 1024).toFixed(2)} KB`;
  return `${v} B`;
}

onMounted(async () => {
  // 真机降级：拉 storage estimate
  if (typeof navigator !== "undefined" && navigator.storage?.estimate) {
    try {
      const est = await navigator.storage.estimate();
      storageEstimate.value = { quota: est.quota ?? 0, usage: est.usage ?? 0 };
    } catch {
      // 浏览器可能无 StorageManager
    }
  }
  // 沙箱（isNative=false）：用 sparse main file 验证（physicalChunkMB=0，物理占用 ~16KB）
  // 真机（isNative=true）：默认 128GB 物理分片（131072 MB）—— 这就是 ECv4 真机要测的
  if (!isNative() && Number(cfg.value.physicalChunkMB) === 131072) {
    cfg.value.physicalChunkMB = 0;
  }
});

async function confirmIfHighRisk(): Promise<boolean> {
  if (!isHighRisk.value) return true;
  const alert = await alertController.create({
    header: t("devtools.sparseContainer.highRiskTitle"),
    message: t("devtools.sparseContainer.highRiskMessage", {
      proposed: formatBytes(proposedBytes.value),
      quota: formatBytes(storageEstimate.value?.quota ?? 0),
    }),
    buttons: [
      { text: t("common.cancel"), role: "cancel" },
      { text: t("common.confirm"), role: "confirm" },
    ],
  });
  await alert.present();
  const { role } = await alert.onDidDismiss();
  return role === "confirm";
}

async function _handleWrite() {
  if (isWriting.value) return;
  if (!(await confirmIfHighRisk())) return;
  isWriting.value = true;
  lastProbe.value = null;
  try {
    lastResult.value = await writeSparseContainer({
      outputDir: cfg.value.outputDir,
      baseName: cfg.value.baseName,
      fragmentCount: Number(cfg.value.fragmentCount),
      fragmentSizeGB: Number(cfg.value.fragmentSizeGB),
      physicalChunkMB: Number(cfg.value.physicalChunkMB),
      cipherMode: 0,
      containerType: 1,
    });
    showToast({
      message: t("devtools.sparseContainer.writeSuccess", {
        virtual: formatBytes(lastResult.value.virtualTotalBytes),
        physical: formatBytes(lastResult.value.physicalUsedBytes),
      }),
      duration: 3000,
      color: "success",
    });
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({
      message: `${t("devtools.sparseContainer.writeFailed")}: ${detail}`,
      duration: 4000,
      color: "danger",
    });
  } finally {
    isWriting.value = false;
  }
}

async function _handleProbe() {
  if (isProbing.value || !lastResult.value) return;
  isProbing.value = true;
  try {
    lastProbe.value = await probeSparseContainer({
      mainPath: lastResult.value.mainFilePath,
      fragmentIdx: 0,
      fragmentSizeGB: Number(cfg.value.fragmentSizeGB),
    });
    showToast({
      message: t("devtools.sparseContainer.probeSuccess"),
      duration: 2000,
      color: "success",
    });
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({
      message: `${t("devtools.sparseContainer.probeFailed")}: ${detail}`,
      duration: 4000,
      color: "danger",
    });
  } finally {
    isProbing.value = false;
  }
}

async function _handleCleanup() {
  if (isCleaning.value || !lastResult.value) return;
  const alert = await alertController.create({
    header: t("devtools.sparseContainer.cleanupConfirm"),
    message: t("devtools.sparseContainer.cleanupConfirmMessage", {
      path: lastResult.value.mainFilePath,
    }),
    buttons: [
      { text: t("common.cancel"), role: "cancel" },
      { text: t("common.confirm"), role: "confirm" },
    ],
  });
  await alert.present();
  const { role } = await alert.onDidDismiss();
  if (role !== "confirm") return;
  isCleaning.value = true;
  try {
    await cleanupSparseContainer({
      outputDir: cfg.value.outputDir,
      baseName: cfg.value.baseName,
    });
    showToast({
      message: t("devtools.sparseContainer.cleanupSuccess"),
      duration: 2000,
      color: "success",
    });
    lastResult.value = null;
    lastProbe.value = null;
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({
      message: `${t("devtools.sparseContainer.cleanupFailed")}: ${detail}`,
      duration: 4000,
      color: "danger",
    });
  } finally {
    isCleaning.value = false;
  }
}
</script>

<style scoped>
.storage-banner {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 16px;
  margin: 12px 16px 0;
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  border-radius: 10px;
  font-size: 12px;
  line-height: 1.5;
}
.banner-icon {
  font-size: 20px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
  margin-top: 2px;
}
.banner-text { flex: 1; min-width: 0; }
.warning-text {
  color: var(--ion-color-warning-shade, #b36b00);
  margin-top: 6px;
  display: flex;
  align-items: center;
  gap: 4px;
  font-weight: 500;
}
.hint-row {
  --padding-start: 16px;
  --padding-end: 16px;
  --inner-padding-end: 0;
}
.hint-row small {
  font-size: 11px;
  color: var(--ion-color-medium);
  line-height: 1.4;
}
ion-input { --background: transparent; }
code {
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
  font-size: 11px;
  word-break: break-all;
}
</style>
