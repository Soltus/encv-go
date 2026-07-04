# Plan：Sparse Container 前端 UI + V3/V4 残留清零（最终收尾）

> **生成时间**：2026-06-11
> **上一轮 plan**：[webdav-auth-v3v4-cleanup-128gb-test.md](file:///workspace/.trae/documents/webdav-auth-v3v4-cleanup-128gb-test.md) 已批准
> **本轮范围**：Task 1/2/3 已完成的全部收尾——主要是 Task 2 的最后 2 处残留硬编码 + Task 3 的前端 UI 4 件套
> **不再扩展**：用户上一轮已批准"完全替换为常量+enum" + "狗屁不通 128GB 方案"——本 plan 不重新讨论方案选择,只补齐工程落地的最后一块

---

## 0. 当前状态盘点

### ✅ Task 1（WebDAV Basic Auth）已全部完成
- `useWebDavAutomationTests.ts:35-37` 常量、:44-58 `loadWebDavCreds`、:62 `buildAuthHeaders`、:343 注入 11 个 helper
- `WebDavAutomationTestsDetail.vue:52-95` 折叠账号配置面板 + `IonInput` × 2
- `api/encv.ts:1007-1025` `fetchWebDavLocalInfo` / `WebDavLocalInfo`
- `mobile_api.go:635-` `handleWebDavLocalInfoGin` + `server.go:312` `/api/webdav/local-info` 路由
- `i18n/tasks.ts:179-193/373-388` 16 个 `devtools.webdavAuth.*` key
- 后端实测：`GET /api/webdav/local-info` 200 OK、`OPTIONS /webdav/` 带 `admin:123456` → 200

### ⚠️ Task 2（V3/V4 → ECv3/ECv4 enum）剩 2 处残留
- ✅ `constants/containerVersion.ts:1-87` 全部 helper + 常量
- ✅ `internal/v2/types/container.go:88-119` `IsRecommendedVersion` / `IsDeprecatedVersion` / `GetVersionStatus`
- ✅ 12+ 文件 14/14 vitest 测试通过
- ❌ **[FilePreview.vue:99](file:///workspace/app/encv-mobile/src/views/FilePreview.vue#L99)** `V{{ containerInfo.version ?? '?' }}` → 改 `formatContainerVersion(containerInfo.version)`
- ❌ **[FileInfo.vue:140](file:///workspace/app/encv-mobile/src/views/FileInfo.vue#L140)** `V{{ containerData.version ?? '?' }}` → 改 `formatContainerVersion(containerData.version)`

### 🔵 Task 3（ECv4 128GB Sparse Container）后端完成 / 前端未做

**后端（已完成 + 通过测试）**：
- ✅ `internal/v2/testutil/sparse_container.go` 415 行：Config / Result / `WriteSparseVirtualContainer` / `ReadSparseContainerEdgeProbe` / `CleanupSparseContainer` / `VerifySparseReadSafe` / `DefaultSparseConfig`(100×128GB)
- ✅ `internal/v2/testutil/sparse_container_test.go` 4 个测试（5×100MB、100×128GB physical<1MB、defaults、JSON serialize）全 PASS
- ✅ `mobile_api.go:1030-` `handleSparseContainerWriteGin` / `:1090-` ProbeGin / `:1125-` CleanupGin
- ✅ `server.go:332-334` `/api/dev/sparse-container` 三路由注册
- ✅ 实测：5×1GB → virtual=5GB, physical=4KB；100×128GB → virtual=12.8TB, physical=16KB, isSparse=true；probe fragment 0 → bytesRead=1024, heapInUse=3536KB

**前端（未做，4 件套全缺）**：
- ❌ `app/encv-mobile/src/api/sparseContainer.ts`（API client） — 不存在
- ❌ `app/encv-mobile/src/views/SparseContainerTestDetail.vue`（UI 视图） — 不存在
- ❌ `router/index.ts:107` 之后无 `settings/devtools/sparse-container-test` 路由
- ❌ `i18n/tasks.ts` 无 `devtools.sparseContainer.*` key
- ❌ `DevToolsDetail.vue:62-68` 后无 ECv4 容量边界测试 entry

---

## 1. 架构决策（不重提方案选择）

### 1.1 前端 UI 形态：参考 `WebDavAutomationTestsDetail.vue` 模式
- 同样一个独立 `*Detail.vue` 视图，挂在 `/settings/devtools/sparse-container-test`
- 顶部 ion-toolbar + 后退按钮（与 webdav-tests 一致）
- 主体 3 个区块：
  1. **配置区**（3 个 `ion-input`）：`fragmentCount` / `fragmentSizeGB` / `physicalChunkMB`
  2. **操作区**（3 个 `ion-button`）：写容器 / 探测 fragment 0 / 清理
  3. **结果区**（实时表格）：声称 vs 物理（main file 实际字节、total 占用、isSparse 布尔、heapInUse 峰值、probe 耗时）

### 1.2 真机降级（不实际写 128GB）
- 在页面 onMounted 阶段调 `navigator.storage.estimate()` → 拿到 `quota` / `usage`
- 计算 `proposedBytes = fragmentCount * fragmentSizeGB * 1024^3`
- 若 `proposedBytes > quota * 0.5` 或 `fragmentSizeGB > 4`（uint32 main file 上限的 32 倍，物理分片在 .part 文件才安全），弹 `alertController` 二次确认 + 显示警告
- 用 `@capacitor/core` 的 `Capacitor.isNativePlatform()` 判断是真机 vs 浏览器；真机默认 physicalChunkMB=30（最小值），浏览器默认 0（不分片，纯 sparse main file）
- 写完后必弹 cleanup 提醒（避免 12.8TB sparse 元数据长期占用 inode）

### 1.3 路由位置
- 路径：`/settings/devtools/sparse-container-test`
- 跟随 `WebDavAutomationTestsDetail.vue` 的命名风格（kebab-case + 名词）
- 放在 `router/index.ts:107` `webdav-tests` 路由之后

### 1.4 i18n key 命名
- 顶层 `devtools.sparseContainer.*`，与 `devtools.webdavAuth.*`、`devtools.webdavTests` 平级
- 双语（中英）共 14 个 key，详见 §3.4

---

## 2. 待修改 / 新建文件清单

| # | 路径 | 类型 | 变更内容 |
|---|------|------|---------|
| 1 | `app/encv-mobile/src/views/FilePreview.vue` | 修改 | 第 99 行 `V{{ ... }}` → `formatContainerVersion(...)` + import |
| 2 | `app/encv-mobile/src/views/FileInfo.vue` | 修改 | 第 140 行 `V{{ ... }}` → `formatContainerVersion(...)` + import |
| 3 | `app/encv-mobile/src/api/sparseContainer.ts` | **新建** | 4 个 API function + 4 个 interface（write / probe / cleanup / health） |
| 4 | `app/encv-mobile/src/views/SparseContainerTestDetail.vue` | **新建** | 主视图：配置 / 操作 / 结果 / 真机降级 |
| 5 | `app/encv-mobile/src/router/index.ts` | 修改 | 加 `settings/devtools/sparse-container-test` 路由 |
| 6 | `app/encv-mobile/src/views/DevToolsDetail.vue` | 修改 | 加 ECv4 容量边界测试 entry（在 `webdavTests` 之后） + 路由跳转函数 |
| 7 | `app/encv-mobile/src/i18n/tasks.ts` | 修改 | 加 14 个 `devtools.sparseContainer.*` key（zh + en） |

---

## 3. 实施步骤（按依赖顺序）

### Step 1：清零 V3/V4 残留（FilePreview + FileInfo）

#### 1.1 `app/encv-mobile/src/views/FilePreview.vue`
- 找 `<script setup lang="ts">` 块，**import**：`import { formatContainerVersion } from '@/constants/containerVersion'`
- 第 99 行 `<span class="info-value">V{{ containerInfo.version ?? '?' }}</span>` 改为：
  ```html
  <span class="info-value">{{ formatContainerVersion(containerInfo.version) || '?' }}</span>
  ```

#### 1.2 `app/encv-mobile/src/views/FileInfo.vue`
- 同上，import + 第 140 行改 `formatContainerVersion(containerData.version) || '?'`

#### 1.3 验证
```bash
cd /workspace/app/encv-mobile && npx vitest run --reporter=basic
# 14/14 仍应通过
```
```bash
grep -rn "V{{.*version" /workspace/app/encv-mobile/src
# 应无任何命中
```
```bash
grep -rn "formatContainerVersion" /workspace/app/encv-mobile/src
# 应增加 2 处（FilePreview + FileInfo）
```

---

### Step 2：新建 `app/encv-mobile/src/api/sparseContainer.ts`

**4 个 interface + 4 个 function**（参考 `encv.ts:1007-1025` `fetchWebDavLocalInfo` 风格）：

```ts
import { getApiBaseUrl } from './getApiBaseUrl'  // 视项目实际路径调整

export interface SparseContainerRequest {
  outputDir: string         // e.g. '/tmp/encv-sparse-test'
  baseName: string          // e.g. 'huge-100x128gb'
  fragmentCount: number     // 100
  fragmentSizeGB: number    // 128
  physicalChunkMB: number   // 0=仅 main file; ≥30=生成 .part 文件
  cipherMode: number        // 0=AES-128-CTR, 1=AES-256-CTR
  containerType: number     // 1=video / 2=audio / ...
}

export interface SparseContainerResponse {
  virtualTotalBytes: number
  physicalMainBytes: number
  physicalUsedBytes: number
  manifestSizeBytes: number
  fragmentCount: number
  fragmentSizeBytes: number
  isSparse: boolean
  mainFilePath: string
  partFilePattern: string
  durationMs: number
}

export interface SparseContainerProbeRequest {
  mainPath: string
  fragmentIdx: number
  fragmentSizeGB: number
}

export interface SparseContainerProbeResponse {
  bytesRead: number
  heapInUseKB: number
  physicalSize: number
  virtualSize: number
  durationMs: number
  seekMs: number
  readMs: number
}

export interface SparseContainerCleanupRequest {
  outputDir: string
  baseName: string
}

export interface SparseContainerCleanupResponse {
  removedFiles: string[]
  removedBytes: number
  durationMs: number
}

export async function writeSparseContainer(
  req: SparseContainerRequest,
): Promise<SparseContainerResponse> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/dev/sparse-container`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!response.ok) {
    let detail = ''
    try { detail = (await response.json())?.error || '' } catch {}
    throw new Error(detail || `HTTP error! status: ${response.status}`)
  }
  return response.json() as Promise<SparseContainerResponse>
}

export async function probeSparseContainer(
  req: SparseContainerProbeRequest,
): Promise<SparseContainerProbeResponse> {
  const baseUrl = getApiBaseUrl()
  const params = new URLSearchParams({
    mainPath: req.mainPath,
    fragmentIdx: String(req.fragmentIdx),
    fragmentSizeGB: String(req.fragmentSizeGB),
  })
  const response = await fetch(`${baseUrl}/api/dev/sparse-container/probe?${params}`)
  if (!response.ok) {
    let detail = ''
    try { detail = (await response.json())?.error || '' } catch {}
    throw new Error(detail || `HTTP error! status: ${response.status}`)
  }
  return response.json() as Promise<SparseContainerProbeResponse>
}

export async function cleanupSparseContainer(
  req: SparseContainerCleanupRequest,
): Promise<SparseContainerCleanupResponse> {
  const baseUrl = getApiBaseUrl()
  const params = new URLSearchParams({ outputDir: req.outputDir, baseName: req.baseName })
  const response = await fetch(`${baseUrl}/api/dev/sparse-container?${params}`, { method: 'DELETE' })
  if (!response.ok) {
    let detail = ''
    try { detail = (await response.json())?.error || '' } catch {}
    throw new Error(detail || `HTTP error! status: ${response.status}`)
  }
  return response.json() as Promise<SparseContainerCleanupResponse>
}
```

**注意**：`getApiBaseUrl` 实际路径以项目为准（`/workspace/app/encv-mobile/src/api/` 目录只有 `encv.ts` + `mockGenerator.ts`，可能通过 `import { getApiBaseUrl } from './encv'` 或独立文件，executor 实施时按项目实际 import 路径调整）。

---

### Step 3：新建 `app/encv-mobile/src/views/SparseContainerTestDetail.vue`

**结构**（参考 `WebDavAutomationTestsDetail.vue:1-100`）：

#### 3.1 Template（4 个 section）

```html
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

      <!-- ② 配置区 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.sparseContainer.config') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-label position="stacked">{{ t('devtools.sparseContainer.fragmentCount') }}</ion-label>
          <ion-input v-model="cfg.fragmentCount" type="number" :placeholder="String(DEFAULT_FRAGMENT_COUNT)"></ion-input>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">{{ t('devtools.sparseContainer.fragmentSizeGB') }}</ion-label>
          <ion-input v-model="cfg.fragmentSizeGB" type="number" :placeholder="String(DEFAULT_FRAGMENT_SIZE_GB)"></ion-input>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">{{ t('devtools.sparseContainer.physicalChunkMB') }}</ion-label>
          <ion-input v-model="cfg.physicalChunkMB" type="number" :placeholder="String(DEFAULT_PHYSICAL_CHUNK_MB)"></ion-input>
          <ion-text color="medium" class="hint">
            <small>{{ t('devtools.sparseContainer.physicalChunkMBHint') }}</small>
          </ion-text>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">{{ t('devtools.sparseContainer.outputDir') }}</ion-label>
          <ion-input v-model="cfg.outputDir" :placeholder="DEFAULT_OUTPUT_DIR"></ion-input>
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
        <ion-item button :disabled="!lastResult || isCleaning" color="danger" @click="handleCleanup">
          <ion-icon :icon="trashOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.cleanup') }}</h3>
            <p>{{ t('devtools.sparseContainer.cleanupHint') }}</p>
          </ion-label>
          <ion-spinner v-if="isCleaning" slot="end" name="dots"></ion-spinner>
        </ion-item>
      </ion-list>

      <!-- ④ 结果区（表格：声称 vs 物理） -->
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
            <p>{{ formatBytes(lastResult.virtualTotalBytes) }} ({{ lastResult.fragmentCount }} × {{ formatBytes(lastResult.fragmentSizeBytes) }})</p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.physicalMain') }}</h3>
            <p>{{ formatBytes(lastResult.physicalMainBytes) }} (main file apparent size)</p>
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
      </ion-list>
    </ion-content>
  </ion-page>
</template>
```

#### 3.2 Script（关键 state + handlers）

```ts
<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonContent, IonList, IonListHeader, IonItem, IonLabel, IonInput,
  IonButton, IonIcon, IonText, IonSpinner, IonBadge, alertController,
} from '@ionic/vue'
import {
  createOutline, searchOutline, trashOutline,
  informationCircleOutline, warningOutline,
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { showToast } from '@/composables/useToast'
import { Capacitor } from '@capacitor/core'  // 若不可用则用 isNative() 替代
import {
  writeSparseContainer, probeSparseContainer, cleanupSparseContainer,
  type SparseContainerResponse, type SparseContainerProbeResponse,
} from '@/api/sparseContainer'

const { t } = useI18n()

// 默认值 — 跟 Go testutil.DefaultSparseConfig 对齐
const DEFAULT_FRAGMENT_COUNT = 100
const DEFAULT_FRAGMENT_SIZE_GB = 128
const DEFAULT_PHYSICAL_CHUNK_MB = 0
const DEFAULT_OUTPUT_DIR = '/tmp/encv-sparse-test'

const cfg = ref({
  fragmentCount: DEFAULT_FRAGMENT_COUNT,
  fragmentSizeGB: DEFAULT_FRAGMENT_SIZE_GB,
  physicalChunkMB: DEFAULT_PHYSICAL_CHUNK_MB,
  outputDir: DEFAULT_OUTPUT_DIR,
  baseName: 'huge-100x128gb',
})
const isWriting = ref(false)
const isProbing = ref(false)
const isCleaning = ref(false)
const lastResult = ref<SparseContainerResponse | null>(null)
const lastProbe = ref<SparseContainerProbeResponse | null>(null)
const storageEstimate = ref<{ quota: number; usage: number } | null>(null)

const proposedBytes = computed(() => cfg.value.fragmentCount * cfg.value.fragmentSizeGB * 1024 ** 3)

const isHighRisk = computed(() => {
  if (!storageEstimate.value) return proposedBytes.value > 1024 ** 4  // > 1TB always risky
  return proposedBytes.value > storageEstimate.value.quota * 0.5
})

const sparseRatioText = computed(() => {
  if (!lastResult.value) return ''
  const { virtualTotalBytes, physicalUsedBytes } = lastResult.value
  if (physicalUsedBytes === 0) return '∞'
  const ratio = virtualTotalBytes / physicalUsedBytes
  if (ratio > 1e6) return `${(ratio / 1e6).toFixed(2)}M× (${formatBytes(virtualTotalBytes)} / ${formatBytes(physicalUsedBytes)})`
  if (ratio > 1e3) return `${(ratio / 1e3).toFixed(2)}K× (${formatBytes(virtualTotalBytes)} / ${formatBytes(physicalUsedBytes)})`
  return `${ratio.toFixed(2)}× (${formatBytes(virtualTotalBytes)} / ${formatBytes(physicalUsedBytes)})`
})

function formatBytes(n: number | undefined | null): string {
  if (n == null) return '?'
  if (n >= 1024 ** 4) return `${(n / 1024 ** 4).toFixed(2)} TB`
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(2)} GB`
  if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(2)} MB`
  if (n >= 1024) return `${(n / 1024).toFixed(2)} KB`
  return `${n} B`
}

onMounted(async () => {
  // 真机降级：拉 storage estimate + 调整默认值
  if (typeof navigator !== 'undefined' && navigator.storage && navigator.storage.estimate) {
    try {
      const est = await navigator.storage.estimate()
      storageEstimate.value = { quota: est.quota ?? 0, usage: est.usage ?? 0 }
    } catch { /* 浏览器可能无 StorageManager */ }
  }
  // 真机：physicalChunkMB 强制 ≥30（避免 .part 文件过小）
  const isNative = typeof Capacitor !== 'undefined' && Capacitor.isNativePlatform?.()
  if (isNative && cfg.value.physicalChunkMB === 0) {
    cfg.value.physicalChunkMB = 30
  }
})

async function confirmIfHighRisk(): Promise<boolean> {
  if (!isHighRisk.value) return true
  const alert = await alertController.create({
    header: t('devtools.sparseContainer.highRiskTitle'),
    message: t('devtools.sparseContainer.highRiskMessage', {
      proposed: formatBytes(proposedBytes.value),
      quota: formatBytes(storageEstimate.value?.quota ?? 0),
    }),
    buttons: [
      { text: t('common.cancel'), role: 'cancel' },
      { text: t('common.confirm'), role: 'confirm' },
    ],
  })
  await alert.present()
  const { role } = await alert.onDidDismiss()
  return role === 'confirm'
}

async function handleWrite() {
  if (isWriting.value) return
  if (!await confirmIfHighRisk()) return
  isWriting.value = true
  lastProbe.value = null
  try {
    lastResult.value = await writeSparseContainer({
      outputDir: cfg.value.outputDir,
      baseName: cfg.value.baseName,
      fragmentCount: Number(cfg.value.fragmentCount),
      fragmentSizeGB: Number(cfg.value.fragmentSizeGB),
      physicalChunkMB: Number(cfg.value.physicalChunkMB),
      cipherMode: 0,
      containerType: 1,
    })
    showToast({
      message: t('devtools.sparseContainer.writeSuccess', {
        virtual: formatBytes(lastResult.value.virtualTotalBytes),
        physical: formatBytes(lastResult.value.physicalUsedBytes),
      }),
      duration: 3000, color: 'success',
    })
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e)
    showToast({ message: t('devtools.sparseContainer.writeFailed') + ': ' + detail, duration: 4000, color: 'danger' })
  } finally {
    isWriting.value = false
  }
}

async function handleProbe() {
  if (isProbing.value || !lastResult.value) return
  isProbing.value = true
  try {
    lastProbe.value = await probeSparseContainer({
      mainPath: lastResult.value.mainFilePath,
      fragmentIdx: 0,
      fragmentSizeGB: Number(cfg.value.fragmentSizeGB),
    })
    showToast({ message: t('devtools.sparseContainer.probeSuccess'), duration: 2000, color: 'success' })
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e)
    showToast({ message: t('devtools.sparseContainer.probeFailed') + ': ' + detail, duration: 4000, color: 'danger' })
  } finally {
    isProbing.value = false
  }
}

async function handleCleanup() {
  if (isCleaning.value || !lastResult.value) return
  const alert = await alertController.create({
    header: t('devtools.sparseContainer.cleanupConfirm'),
    message: t('devtools.sparseContainer.cleanupConfirmMessage', { path: lastResult.value.mainFilePath }),
    buttons: [
      { text: t('common.cancel'), role: 'cancel' },
      { text: t('common.confirm'), role: 'confirm' },
    ],
  })
  await alert.present()
  const { role } = await alert.onDidDismiss()
  if (role !== 'confirm') return
  isCleaning.value = true
  try {
    await cleanupSparseContainer({ outputDir: cfg.value.outputDir, baseName: cfg.value.baseName })
    showToast({ message: t('devtools.sparseContainer.cleanupSuccess'), duration: 2000, color: 'success' })
    lastResult.value = null
    lastProbe.value = null
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e)
    showToast({ message: t('devtools.sparseContainer.cleanupFailed') + ': ' + detail, duration: 4000, color: 'danger' })
  } finally {
    isCleaning.value = false
  }
}
</script>
```

#### 3.3 Style（最简,沿用 webdav-tests 的 scope 风格）

```css
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
.banner-icon { font-size: 20px; color: var(--ion-color-primary); flex-shrink: 0; }
.banner-text { flex: 1; }
.warning-text { color: var(--ion-color-warning); margin-top: 4px; display: flex; align-items: center; gap: 4px; }
.hint { font-size: 11px; padding: 4px 16px 0; }
ion-input { --background: transparent; }
code { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 11px; }
</style>
```

---

### Step 4：注册路由 `router/index.ts`

**位置**：[`router/index.ts:107`](file:///workspace/app/encv-mobile/src/router/index.ts#L107) 之后（在 `webdav-tests` 路由之后）。

**Edit**：
```ts
{
  // 🆕 2026-06-11：ECv4 容量边界测试（100×128GB sparse 虚拟容器）
  path: 'settings/devtools/sparse-container-test',
  component: () => import('@/views/SparseContainerTestDetail.vue'),
},
```

---

### Step 5：DevTools 入口 `DevToolsDetail.vue`

#### 5.1 加 entry（在 [`webdavTests`](file:///workspace/app/encv-mobile/src/views/DevToolsDetail.vue#L62-L68) 之后）

在 ion-item 块后追加：
```html
<!-- 🆕 2026-06-11：ECv4 容量边界测试入口 -->
<ion-item button detail @click="goSparseContainerTest">
  <ion-icon :icon="serverOutline" slot="start" color="warning"></ion-icon>
  <ion-label>
    <h3>{{ t('devtools.sparseContainer.title') }}</h3>
    <p>{{ t('devtools.sparseContainer.entryHint') }}</p>
  </ion-label>
</ion-item>
```

#### 5.2 加跳转函数（在 [`goWebDavTests`](file:///workspace/app/encv-mobile/src/views/DevToolsDetail.vue#L292-L294) 之后）
```ts
function goSparseContainerTest() {
  router.push('/tabs/settings/devtools/sparse-container-test')
}
```

#### 5.3 加 icon import（[line 180-186](file:///workspace/app/encv-mobile/src/views/DevToolsDetail.vue#L180) 区域）
```ts
import {
  bugOutline, downloadOutline, readerOutline, trashOutline,
  chevronForward, playCircleOutline, musicalNotesOutline,
  colorPaletteOutline, settingsOutline, terminal, documentText,
  cloudOutline, refreshOutline, eyeOutline, cloudUploadOutline,
  extensionPuzzleOutline, flaskOutline, rocketOutline,
  serverOutline,  // 🆕
} from 'ionicons/icons'
```

#### 5.4 加 i18n key 引用检查
确保 i18n key 引用全部走 `t('devtools.sparseContainer.*')`，不在模板里硬编码字符串。

---

### Step 6：i18n keys `i18n/tasks.ts`

**位置**：[`webdavAuth.*`](file:///workspace/app/encv-mobile/src/i18n/tasks.ts#L179-L193) 块之后追加。

#### 6.1 中文（`zh` 块，line 193 之后）

```ts
'devtools.sparseContainer.title': 'ECv4 容量边界测试',
'devtools.sparseContainer.entryHint': '写入 100×128GB sparse 虚拟容器,验证 physical_used ≪ virtual_total（避免实际占用 12.8TB）',
'devtools.sparseContainer.config': '配置',
'devtools.sparseContainer.fragmentCount': '分片数',
'devtools.sparseContainer.fragmentSizeGB': '每片大小 (GB)',
'devtools.sparseContainer.physicalChunkMB': '物理分片 (MB)',
'devtools.sparseContainer.physicalChunkMBHint': '0=仅 main file; ≥30=生成 .part 物理分片',
'devtools.sparseContainer.outputDir': '输出目录',
'devtools.sparseContainer.write': '写入 sparse 容器',
'devtools.sparseContainer.writeHint': '声称 vs 物理: 12.8TB 虚拟 / 16KB 实际',
'devtools.sparseContainer.probe': '探测 fragment 0',
'devtools.sparseContainer.probeHint': '读 1 个 fragment,记录耗时 + 内存峰值',
'devtools.sparseContainer.cleanup': '清理产物',
'devtools.sparseContainer.cleanupHint': '删除 main file + .part 文件,释放 inode',
'devtools.sparseContainer.lastResult': '最近结果',
'devtools.sparseContainer.virtualTotal': '虚拟总大小',
'devtools.sparseContainer.physicalMain': '物理 main file',
'devtools.sparseContainer.physicalUsed': '实际占用 (du)',
'devtools.sparseContainer.manifestSize': 'Manifest 大小',
'devtools.sparseContainer.sparseRatio': '稀疏比',
'devtools.sparseContainer.duration': '耗时',
'devtools.sparseContainer.mainFilePath': '主文件路径',
'devtools.sparseContainer.probeResult': '探测结果',
'devtools.sparseContainer.bytesRead': '读取字节',
'devtools.sparseContainer.heapInUse': '堆内存峰值',
'devtools.sparseContainer.quota': '存储配额',
'devtools.sparseContainer.used': '已用',
'devtools.sparseContainer.highRiskWarning': '高风险: 声称大小超过配额 50%',
'devtools.sparseContainer.highRiskTitle': '确认写入',
'devtools.sparseContainer.highRiskMessage': '声称 {proposed} / 配额 {quota}。sparse 不实际占用但请确认是真测试而非误输入。',
'devtools.sparseContainer.writeSuccess': '写入完成: 虚拟 {virtual} / 实际 {physical}',
'devtools.sparseContainer.writeFailed': '写入失败',
'devtools.sparseContainer.probeSuccess': '探测完成',
'devtools.sparseContainer.probeFailed': '探测失败',
'devtools.sparseContainer.cleanupConfirm': '确认清理',
'devtools.sparseContainer.cleanupConfirmMessage': '即将删除: {path}',
'devtools.sparseContainer.cleanupSuccess': '清理完成',
'devtools.sparseContainer.cleanupFailed': '清理失败',
```

#### 6.2 英文（`en` 块，line 388 之后）

```ts
'devtools.sparseContainer.title': 'ECv4 Capacity Boundary Test',
'devtools.sparseContainer.entryHint': 'Write a 100×128GB sparse virtual container to verify physical_used ≪ virtual_total (avoid actually occupying 12.8TB)',
'devtools.sparseContainer.config': 'Configuration',
'devtools.sparseContainer.fragmentCount': 'Fragment count',
'devtools.sparseContainer.fragmentSizeGB': 'Fragment size (GB)',
'devtools.sparseContainer.physicalChunkMB': 'Physical chunk (MB)',
'devtools.sparseContainer.physicalChunkMBHint': '0=main file only; ≥30=generate .part files',
'devtools.sparseContainer.outputDir': 'Output dir',
'devtools.sparseContainer.write': 'Write sparse container',
'devtools.sparseContainer.writeHint': 'Claimed vs physical: 12.8TB virtual / 16KB actual',
'devtools.sparseContainer.probe': 'Probe fragment 0',
'devtools.sparseContainer.probeHint': 'Read 1 fragment, record duration + heap peak',
'devtools.sparseContainer.cleanup': 'Cleanup artifacts',
'devtools.sparseContainer.cleanupHint': 'Delete main file + .part files, free inode',
'devtools.sparseContainer.lastResult': 'Last result',
'devtools.sparseContainer.virtualTotal': 'Virtual total',
'devtools.sparseContainer.physicalMain': 'Physical main file',
'devtools.sparseContainer.physicalUsed': 'Actual used (du)',
'devtools.sparseContainer.manifestSize': 'Manifest size',
'devtools.sparseContainer.sparseRatio': 'Sparse ratio',
'devtools.sparseContainer.duration': 'Duration',
'devtools.sparseContainer.mainFilePath': 'Main file path',
'devtools.sparseContainer.probeResult': 'Probe result',
'devtools.sparseContainer.bytesRead': 'Bytes read',
'devtools.sparseContainer.heapInUse': 'Heap peak',
'devtools.sparseContainer.quota': 'Storage quota',
'devtools.sparseContainer.used': 'Used',
'devtools.sparseContainer.highRiskWarning': 'High risk: claimed size > 50% of quota',
'devtools.sparseContainer.highRiskTitle': 'Confirm write',
'devtools.sparseContainer.highRiskMessage': 'Claimed {proposed} / quota {quota}. Sparse does not actually occupy space, but please confirm this is a real test.',
'devtools.sparseContainer.writeSuccess': 'Write complete: virtual {virtual} / actual {physical}',
'devtools.sparseContainer.writeFailed': 'Write failed',
'devtools.sparseContainer.probeSuccess': 'Probe complete',
'devtools.sparseContainer.probeFailed': 'Probe failed',
'devtools.sparseContainer.cleanupConfirm': 'Confirm cleanup',
'devtools.sparseContainer.cleanupConfirmMessage': 'About to delete: {path}',
'devtools.sparseContainer.cleanupSuccess': 'Cleanup complete',
'devtools.sparseContainer.cleanupFailed': 'Cleanup failed',
```

**注意**：`useI18n` 的 `t(key, params)` 是否支持 `{proposed}` 占位符视项目实际实现，executor 实施时若不支持则改用 `t(key) + ' (' + formatBytes(proposedBytes.value) + ')'` 拼接。

---

## 4. 验证步骤

### 4.1 编译 + 类型检查
```bash
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit
# 应 0 error（Vite 沙箱项目无 vue-tsc 时用 npx vite build 看编译产物）
```

### 4.2 测试套件
```bash
cd /workspace/app/encv-mobile && npx vitest run --reporter=basic
# 14/14 仍应通过（V3/V4 替换不破坏现有 case）
```

### 4.3 后端 Go 测试
```bash
cd /workspace && go test ./internal/v2/testutil/... -v -run Sparse
# 4/4 PASS
```

### 4.4 端到端流程（浏览器强制刷新）
1. 启动后端：`./tmp/encv-new &`（pid 304495 在 :2025，含 4 个新 endpoint）
2. 启动 vite：`cd app/encv-mobile && npm run dev`（:5173）
3. 浏览器 `Ctrl+Shift+R` 强制刷新
4. 路径：`设置 → 开发者工具 → ECv4 容量边界测试`
5. 看到 3 个 input（100/128/0），点「写入 sparse 容器」→ 实际 `du -h /tmp/encv-sparse-test/huge-100x128gb.sccg` 应为 ~16K（声称 12.8TB）
6. 点「探测 fragment 0」→ 结果区显示 heapInUse < 5MB、durationMs < 100
7. 点「清理产物」→ `ls /tmp/encv-sparse-test/` 应无 sparse 残留

### 4.5 真机降级（Capacitor build 验证）
- 修改 `physicalChunkMB` 默认值为 0 跑一次 → 实际物理 ~16KB ✓
- 切到 30 → 实际物理 = (FragmentCount × 30MB) ≈ 3GB（用于真机 .part 文件场景）
- 高风险场景：把 `fragmentSizeGB` 改成 1024 + `fragmentCount` 100 → proposedBytes = 100TB > quota → 弹二次确认

### 4.6 残留扫描
```bash
grep -rn "V{{.*[Cc]ontainer.*[Vv]ersion" /workspace/app/encv-mobile/src
# 应 0 命中

grep -rn "V{{.*containerData.version\|V{{.*containerInfo.version" /workspace/app/encv-mobile/src
# 应 0 命中（FilePreview + FileInfo 已修）

grep -rn "formatContainerVersion" /workspace/app/encv-mobile/src
# 应 ≥ 7 处（Tasks.vue + TaskBasicInfo.vue + FilePreview.vue + FileInfo.vue + ContainerVersionSelector.vue + AutomationTestsDetail.vue + useAutomationTests.ts）
```

### 4.7 路由验证
```bash
grep -n "sparse-container-test" /workspace/app/encv-mobile/src/router/index.ts
# 应命中 1 次（路由注册）

grep -n "goSparseContainerTest" /workspace/app/encv-mobile/src/views/DevToolsDetail.vue
# 应命中 1 次（跳转函数）
```

### 4.8 i18n 完整性
```bash
grep -c "devtools.sparseContainer" /workspace/app/encv-mobile/src/i18n/tasks.ts
# 应 = 72（36 key × 2 语言）
```

---

## 5. 关键文件路径速查

| 类别 | 路径 |
|------|------|
| 后端 sparse 写出 | [/workspace/internal/v2/testutil/sparse_container.go](file:///workspace/internal/v2/testutil/sparse_container.go) |
| 后端 sparse handler | [/workspace/internal/server/mobile_api.go](file:///workspace/internal/server/mobile_api.go#L1030) |
| 后端 sparse 路由 | [/workspace/internal/server/server.go](file:///workspace/internal/server/server.go#L332-L334) |
| 前端 API client（新建） | /workspace/app/encv-mobile/src/api/sparseContainer.ts |
| 前端视图（新建） | /workspace/app/encv-mobile/src/views/SparseContainerTestDetail.vue |
| 前端路由 | /workspace/app/encv-mobile/src/router/index.ts |
| 前端 DevTools 入口 | /workspace/app/encv-mobile/src/views/DevToolsDetail.vue |
| 前端 i18n | /workspace/app/encv-mobile/src/i18n/tasks.ts |
| 残留硬编码 #1 | [/workspace/app/encv-mobile/src/views/FilePreview.vue](file:///workspace/app/encv-mobile/src/views/FilePreview.vue#L99) |
| 残留硬编码 #2 | [/workspace/app/encv-mobile/src/views/FileInfo.vue](file:///workspace/app/encv-mobile/src/views/FileInfo.vue#L140) |
| 容器版本常量 | [/workspace/app/encv-mobile/src/constants/containerVersion.ts](file:///workspace/app/encv-mobile/src/constants/containerVersion.ts) |
| 后端版本 helper | [/workspace/internal/v2/types/container.go](file:///workspace/internal/v2/types/container.go#L88) |

---

## 6. 假设与决策

### 6.1 假设
- **`getApiBaseUrl` 实际路径**：项目有 `api/encv.ts` 和 `api/mockGenerator.ts`，大概率通过 `import { getApiBaseUrl } from './getApiBaseUrl'` 或 `'./encv'` 引入。executor 实施时按实际项目结构 import。
- **`@capacitor/core` 依赖**：项目其他位置大概率已用（参考 `DevToolsDetail.vue:193 isNative()` from `GoProcess` plugin），若未装则改用 `isNative()` 判断。
- **`useI18n` 占位符**：若不支持 `{key}` 模板，executor 把 message 改成字符串拼接。
- **后端 304495 在 :2025 跑**：若已退出则需重启 `/tmp/encv-new` 进程。

### 6.2 决策
- **UI 形态**：参考 `WebDavAutomationTestsDetail.vue` 模式（ion-page + ion-list + ion-item button）—— 已与项目风格一致
- **路由位置**：`/settings/devtools/sparse-container-test`，跟随 webdav-tests 命名
- **i18n 命名**：`devtools.sparseContainer.*` 顶层 key，36 个 key × 2 语言
- **真机降级**：默认 physicalChunkMB=30（真机）vs 0（浏览器），不阻塞用户手动改回
- **不做 SSR / 后台 worker**：单页面 + 点击触发，不引入额外复杂度

### 6.3 不做的事（明确划界）
- **不改 Go 后端**：sparse_container.go / mobile_api.go / server.go 已是终态
- **不改 ECv4 容器格式**：V4 header 2048B + manifest uint64 是协议层事实，不重新讨论
- **不写额外文档**：本 plan 文档已涵盖实施 + 验证
- **不引入新依赖**：不装 `pretty-bytes` 等库，formatBytes 手写（< 30 行）
- **不写自动化测试**：本任务为 dev tool，配套 4 个 Go test 已覆盖核心逻辑

---

## 7. 风险与回退

| 风险 | 概率 | 缓解 |
|------|------|------|
| `@capacitor/core` 未装 | 低 | fallback 到 `isNative()` from `GoProcess` plugin |
| `useI18n` 不支持 `{placeholder}` | 中 | 改字符串拼接 |
| Vite 沙箱 HMR 不刷新 | 中 | 强制刷新 `Ctrl+Shift+R` |
| 真机 storage.estimate 无 `quota` 字段 | 低 | 默认 0 → 高风险分支退化为「always warn」 |
| 后端 304495 进程被 kill | 中 | 重新跑 `/tmp/encv-new`（编译产物已有） |
| 12.8TB sparse 写出后忘了 cleanup | 低 | UI 弹 cleanup 提醒 + toast 写成功文案后追加「记得清理」 |

---

## 8. 完成判定（DoD）

- [ ] FilePreview.vue:99 改完，`grep "V{{.*containerInfo.version" src/` 0 命中
- [ ] FileInfo.vue:140 改完，`grep "V{{.*containerData.version" src/` 0 命中
- [ ] `sparseContainer.ts` 创建，4 个 function + 4 个 interface
- [ ] `SparseContainerTestDetail.vue` 创建，配置/操作/结果/真机降级 4 区齐全
- [ ] `router/index.ts` 加 `sparse-container-test` 路由
- [ ] `DevToolsDetail.vue` 加 entry + `goSparseContainerTest` + `serverOutline` icon
- [ ] `i18n/tasks.ts` 加 36 个 `devtools.sparseContainer.*` key
- [ ] 浏览器 `Ctrl+Shift+R` 强制刷新后能跳转到 `/tabs/settings/devtools/sparse-container-test`
- [ ] 100×128GB 写入后 `du -h` < 1MB
- [ ] fragment 0 probe 后 `heapInUseKB` < 10000
- [ ] cleanup 后目录为空
- [ ] 14/14 vitest 测试仍 PASS

---

**Plan 状态**：等待用户批准后立即进入执行阶段。**批准后无需再次确认**——executor 直接按 Step 1→6 顺序落地。
