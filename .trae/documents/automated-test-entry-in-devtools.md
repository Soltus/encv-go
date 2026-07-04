# 开发者选项新增自动化测试入口（生产可用）

## 1. Summary

在"开发者选项"页面新增一个**生产构建也可访问**的「自动化测试」入口。提供：

1. **Mock 数据生成 / 重置** — 一键生成或清空测试用文件，dev 模式写到 `<project>/__mock_data__/01-*` 目录；真机自动改写到 `/storage/emulated/0/encv-automation/01-*` 命名空间（与真实用户数据完全隔离）。
2. **自动化测试运行器** — 通过**动态笛卡尔积**生成测试用例（不再硬编码），覆盖后端 API 任务流：每个 plugin 的 `taskType × version × cipherMode × compressionMode` 全组合。
3. **真机安全边界** — 在 `usePathResolver` 新增 `withSafetyBoundary()` 拦截器，临时屏蔽 `/storage/emulated/0` 真实挂载点，强制改写到 `encv-automation` 命名空间。开发与生产行为一致。
4. **任务触发者标签** — 任务卡片 + 详情新增"触发者"badge（用户 / 自动化 / AI 智能体），用于追溯是哪个入口创建的任务。前端用 localStorage 维护 `taskId → triggeredBy` 映射（不依赖后端透传，避免越界修改 Go 后端）。

**生产可用**：与现有"沙箱预览"区块（`v-if="isDev"`）不同，本入口**不**加 `isDev` 限制——任何安装到真机的用户也能点开用于自助问题定位（但有安全边界保护）。

---

## 2. Current State Analysis

### 2.1 现有开发者选项结构

- [Settings.vue](file:///workspace/app/encv-mobile/src/views/Settings.vue#L281-L292)：底部"开发者选项"区块，goDevTools 跳转 `/tabs/settings/devtools`
- [DevToolsDetail.vue](file:///workspace/app/encv-mobile/src/views/DevToolsDetail.vue)：
  - 区块 1：debug tools（vConsole、日志导出/查看/清理）— 生产可用
  - 区块 2：**沙箱预览**（line 45-68，`<ion-list v-if="isDev">`）— dev 专属，Preview OpenList / plugin-openlist
  - 区块 3：日志设置（level/file，云端同步）
  - 区块 4：Compose Prototypes
- [useDevTools.ts](file:///workspace/app/encv-mobile/src/composables/useDevTools.ts)：仅封装 vConsole 开关（24 行），无扩展点

### 2.2 现有 Mock 数据生成脚本

- [generate-mock-files.ts](file:///workspace/app/encv-mobile/scripts/generate-mock-files.ts#L8)：`MOCK_ROOT = process.env.ENCV_MOCK_ROOT || '/storage/emulated/0'`
- 695+ 行：parseArgs、ensureDir、createJPEG/PNG/MP4/MKV/MP3/FLAC/PDF、AENC、SCCV4 容器生成
- **已支持 `genType = 'all' | 'plain' | 'ae' | 'container' | 'boundary'` 四类**
- **已支持 `--dir` 自定义输出目录**
- 关键限制：
  - 强依赖 `child_process`（ffmpeg spawn）和 `fs` 模块 → **不能直接在前端 `import`**
  - 入口是 `main()` CLI 函数，无 export 暴露
  - 不提供 `reset()` 操作（只生成不清理）

### 2.3 现有 createTask API 签名

[useNewTaskModal.ts](file:///workspace/app/encv-mobile/src/composables/useNewTaskModal.ts#L153-L166) 调用 10 个参数：

```typescript
createTask(
  type: TaskType,             // 'encrypt' | 'decrypt'
  sourcePath: string,
  targetPath?: string,
  password?: string,
  version?: number,
  pluginName?: string,
  extraFields?: Record<string, string>,
  secondaryPassword?: string,
  cipherMode?: number,        // v4 only
  compressionMode?: 'none' | 'zstd',  // v4 only
)
```

[api/encv.ts](file:///workspace/app/encv-mobile/src/api/encv.ts#L418-L456) 定义 `EncvTask` 类型，**未含 triggeredBy 字段**。

### 2.4 现有 TaskOptions / 动态配置

[useTaskForm.ts](file:///workspace/app/encv-mobile/src/composables/useTaskForm.ts#L91-L103)：`versionOptions` 来自 `taskOptions.supportedVersions`（后端 API 返回）：

```typescript
interface TaskOptions {
  passwordStrategy: 'global' | 'independent' | 'none'
  supportVersionSelect: boolean
  supportedVersions: number[] | null  // ← 动态来源
  defaultVersion: number
  extraFields: TaskField[]
}
```

[EncryptBody.vue](file:///workspace/app/encv-mobile/src/components/EncryptBody.vue) 已有 cipher mode（0/1）和 compression（none/zstd）的 v4 专属选择控件——这些都是后端 `taskOptions` 驱动，前端 UI 不硬编码。

### 2.5 现有 usePathResolver 缺口

[usePathResolver.ts](file:///workspace/app/encv-mobile/src/composables/usePathResolver.ts)：
- 有 `normalize`（路径规范化）、`isAbsolutePath`、`getMockPaths`（dev 专用）
- **完全无真机安全边界**：生产构建调用 `getMockPaths()` 返回 null，所有路径走真实 `/storage/emulated/0/...`
- **无自动化测试名空间隔离**：直接用真实路径作为 source/target

### 2.6 现有 task UI 标签体系

[Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue#L152-L162)：task 卡片已有 badges：
- `task.id`（monospace，灰色）
- `status-badge`（按 status 着色）
- `task.type`（encrypt/decrypt 文字）
- `plugin-badge`（pluginName，primary 色）

新增 `triggeredBy-badge` 与现有风格一致。

### 2.7 现有 i18n 命名空间

`devtools`（`debugTools` / `vconsole` / `exportLogs` / `openLog` / `clearLogs` / `sandboxPreview`），需要追加 `automationTests` 子命名空间。

---

## 3. Proposed Changes

### 3.1 路由与入口（**生产可用**）

#### 3.1.1 新增路由

**文件**：[src/router/index.ts](file:///workspace/app/encv-mobile/src/router/index.ts) L97 后追加：

```typescript
{
  path: 'settings/devtools/automation',
  component: () => import('@/views/AutomationTestsDetail.vue'),
},
```

**关键决策**：路由不加 `meta.requiresDev` 或 `beforeEnter` 拦截。**生产可访问**。

#### 3.1.2 DevToolsDetail.vue 新增入口项

**文件**：[src/views/DevToolsDetail.vue](file:///workspace/app/encv-mobile/src/views/DevToolsDetail.vue) line 13-42（debug tools 区块后）插入新区块：

```vue
<ion-list>
  <ion-list-header>
    <ion-label>{{ t('devtools.automationTests') }}</ion-label>
    <ion-badge slot="end" color="success" class="scope-badge scope-prod">
      <ion-icon :icon="rocketOutline" class="scope-badge-icon"></ion-icon>
      <span class="scope-text">{{ t('devtools.availableInProd') }}</span>
    </ion-badge>
  </ion-list-header>
  <p class="section-hint">{{ t('devtools.automationTestsHint') }}</p>
  <ion-item button detail @click="goAutomationTests">
    <ion-icon :icon="flaskOutline" slot="start"></ion-icon>
    <ion-label>
      <h3>{{ t('devtools.automationTestsEntry') }}</h3>
      <p>{{ t('devtools.automationTestsEntryDesc') }}</p>
    </ion-label>
  </ion-item>
</ion-list>
```

**新增脚本逻辑**：
```typescript
function goAutomationTests() {
  router.push('/tabs/settings/devtools/automation')
}
```

**新增 import**：`flaskOutline`, `rocketOutline` from `ionicons/icons`；`useRouter` 已存在。

**新增 style**：`.scope-prod` badge 配色（success green，区别 dev-warning yellow）。

### 3.2 Mock 数据生成器模块化（**前后端共用**）

#### 3.2.1 提取纯函数到 `src/lib/mockDataGenerator.ts`

**新建**：[src/lib/mockDataGenerator.ts](file:///workspace/app/encv-mobile/src/lib/mockDataGenerator.ts)

将 [generate-mock-files.ts](file:///workspace/app/encv-mobile/scripts/generate-mock-files.ts) 的所有 `create*` 函数（`createJPEG`, `createPNG`, `createValidMP4`, `createValidMKV`, `createValidMP3`, `createValidFLAC`, `createPDF`, `createAENC`, `createSCCV*`, `createBoundaryFiles` 等）**逐字搬过来**，去 CLI 入口（`parseArgs` / `main`），改为 `export function`：

```typescript
// 文件头不依赖 child_process / fs
export function createJPEG(): Uint8Array { /* ... */ }
export function createPNG(): Uint8Array { /* ... */ }
export function createValidMP4(): Uint8Array { /* ... */ }
// ... 全部 export

export type MockFileType = 'all' | 'plain' | 'ae' | 'container' | 'boundary'

export interface MockFileSpec {
  relativePath: string
  data: Uint8Array
  size: number
}

export interface GenerateOptions {
  root: string           // 真实磁盘路径
  type?: MockFileType    // 默认 'all'
  writeToDisk?: (path: string, data: Uint8Array) => Promise<void>  // 抽象 IO
  onProgress?: (spec: MockFileSpec) => void
}

export async function generateMockFiles(opts: GenerateOptions): Promise<{ count: number; totalSize: number }> {
  const specs = collectSpecs(opts.type ?? 'all')
  let count = 0
  let totalSize = 0
  for (const spec of specs) {
    const fullPath = `${opts.root}/${spec.relativePath}`
    if (opts.writeToDisk) {
      await opts.writeToDisk(fullPath, spec.data)
    }
    count++
    totalSize += spec.size
    opts.onProgress?.(spec)
  }
  return { count, totalSize }
}

export function collectSpecs(type: MockFileType): MockFileSpec[] {
  // 把原本散落在 main() 里的所有 writeBuffer/writeString 调用
  // 转换为 collect-返回数组的纯函数
  // 顺序保持一致：plain → ae → container → boundary
}
```

**关键设计**：
- **不依赖 `fs` / `child_process`** — 通过 `writeToDisk` 回调抽象（CLI 版本传 `fs.writeFile`，前端用 fetch → 后端）
- **纯函数 `create*` 全部 export** — 单元测试可独立验证
- **不依赖 `process.env`** — `root` 通过 `opts.root` 传入

#### 3.2.2 修改 generate-mock-files.ts 调用新模块

**修改**：[scripts/generate-mock-files.ts](file:///workspace/app/encv-mobile/scripts/generate-mock-files.ts)

整段 `main()` 替换为：
```typescript
import { generateMockFiles } from '../src/lib/mockDataGenerator'

async function main(): Promise<void> {
  parseArgs()
  await generateMockFiles({
    root,
    type: genType as any,
    writeToDisk: async (p, data) => {
      fs.mkdirSync(path.dirname(p), { recursive: true })
      fs.writeFileSync(p, Buffer.from(data))
    },
    onProgress: (spec) => console.log('  ✅ ' + spec.relativePath),
  })
  printSummary()
}
```

**CLI 行为保持不变**：仍接受 `--dir` / `--type` / `ENCV_MOCK_ROOT`，输出格式一致。

#### 3.2.3 真机 IO：通过后端 API 写入

**新建**：[src/api/mockGenerator.ts](file:///workspace/app/encv-mobile/src/api/mockGenerator.ts)

```typescript
export async function generateMockFilesViaBackend(opts: {
  root: string
  type?: 'all' | 'plain' | 'ae' | 'container' | 'boundary'
  onProgress?: (spec: { relativePath: string; size: number }) => void
}): Promise<{ count: number; totalSize: number }> {
  // 1. 调 POST /api/mock/generate (后端实现，详见 §3.3)
  // 2. SSE 流式返回每个生成的文件路径 + 大小
  // 3. onProgress 回调更新 UI
}

export async function resetMockFilesViaBackend(root: string): Promise<{ removed: number }> {
  // POST /api/mock/reset { root }
  // 只清空 root 下的文件，不动 /storage/emulated/0/ 其他目录
}
```

### 3.3 后端 Mock 生成 API（Go 新增端点）

**新建**：[internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) + 注册路由

```go
// POST /api/mock/generate
// body: { root: string, type: "all"|"plain"|"ae"|"container"|"boundary" }
// response: SSE stream of { relativePath, size } events
//
// 实现要点：
// - 在 root 下创建 01-plain-media, 02-alist-encrypt, 03-encv-containers, 04-boundary-test 目录
// - 复用 Go 版本的纯函数：jpeg.Minimal, png.Encode, mpeg.MinimalFrame, ...
// - 安全：root 必须在白名单内（开发配置或 env 指定），不允许任意写入
// - 通过 serveWs 风格的 SSE writer 流式返回进度
```

**白名单校验**：
- 允许的 root 前缀：
  - dev: `<project>/__mock_data__/`
  - 真机: `/storage/emulated/0/encv-automation/`（如果 EncvMockRoot 配置项设置）
- 其他路径 → 403 ForbiddenError

**路由注册**：[internal/server/server.go](file:///workspace/internal/server/server.go) 在 r.Group("/api/...") 加 `r.POST("/api/mock/generate", s.handleMockGenerate)` + `r.POST("/api/mock/reset", s.handleMockReset)`。

### 3.4 真机安全边界：withSafetyBoundary

**修改**：[src/composables/usePathResolver.ts](file:///workspace/app/encv-mobile/src/composables/usePathResolver.ts) L38 后追加：

```typescript
const REAL_STORAGE_ROOT = '/storage/emulated/0'
const SAFETY_NAMESPACE = 'encv-automation'

function withSafetyBoundary(rawPath: string, opts?: { forceAutomation?: boolean }): string {
  const normalized = normalize(rawPath)
  if (!normalized) return ''
  
  // dev 模式：直接返回原路径（vite 走 mock 路径 /mock/*）
  if (import.meta.env.DEV) return normalized
  
  // 真机：如果路径以 /storage/emulated/0/ 开头（且非 encv-automation 内部）
  // 自动改写到 /storage/emulated/0/encv-automation/<原路径>
  if (opts?.forceAutomation || normalized.startsWith(REAL_STORAGE_ROOT + '/')) {
    if (normalized.startsWith(REAL_STORAGE_ROOT + '/' + SAFETY_NAMESPACE)) {
      return normalized  // 已经是 automation 命名空间内
    }
    // 把 /storage/emulated/0/foo.txt → /storage/emulated/0/encv-automation/foo.txt
    const rel = normalized.slice(REAL_STORAGE_ROOT.length)
    return `${REAL_STORAGE_ROOT}/${SAFETY_NAMESPACE}${rel}`
  }
  
  return normalized
}
```

**导出更新**：
```typescript
return {
  normalize,
  resolveFileItem,
  isAbsolutePath,
  getMockPaths,
  withSafetyBoundary,  // ← 新增
}
```

**使用方式**：
- `useNewTaskModal.ts` 在 `createTask` 调用前对 `sourcePath` / `targetPath` 调 `withSafetyBoundary`
- `useAutomationTests.ts` 调 `withSafetyBoundary(sourcePath, { forceAutomation: true })` 强制改写

**关键不变量**：
- 自动化测试**永远**走 encv-automation 命名空间
- 用户手动创建任务**默认**走原路径（如果用户在真实 /Download 创建则尊重用户意图）
- 自动化测试入口无论路径如何都强制改写（`forceAutomation: true`）
- 真机 release 构建也生效（`!import.meta.env.DEV` 永远 true）

### 3.5 自动化测试 Composable

**新建**：[src/composables/useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts)

```typescript
import { ref, computed } from 'vue'
import { createTask, fetchPlugins, type PluginMeta, type TaskType, type EncvTask } from '@/api/encv'
import { usePathResolver } from '@/composables/usePathResolver'

export type TriggeredBy = 'user' | 'automation' | 'ai_agent'

export interface TestCaseSpec {
  id: string                     // 唯一 id
  taskType: TaskType             // encrypt | decrypt
  pluginName: string
  sourcePath: string             // 自动化 mock 文件路径
  version: number
  cipherMode?: number            // v4 only
  compressionMode?: 'none' | 'zstd'  // v4 only
  expectedBehavior: 'success' | 'might-fail'  // v2 已废弃可能失败
}

export interface TestCaseResult {
  spec: TestCaseSpec
  status: 'pending' | 'running' | 'passed' | 'failed' | 'skipped'
  taskId?: string
  error?: string
  durationMs?: number
}

export function useAutomationTests() {
  const { withSafetyBoundary } = usePathResolver()
  const plugins = ref<PluginMeta[]>([])
  const isLoadingPlugins = ref(false)
  const isRunning = ref(false)
  const progress = ref({ total: 0, completed: 0, passed: 0, failed: 0 })
  const results = ref<TestCaseResult[]>([])
  const lastError = ref<string | null>(null)
  
  /**
   * 动态笛卡尔积生成测试用例：
   * plugins (n) × taskType (2) × versions (per-plugin) × cipherMode (v4:2) × compression (v4:2)
   *
   * 不硬编码 cipher mode / compression / version——从 PluginMeta.taskOptions 动态派生：
   *   - 如果 plugin.supportVersionSelect + supportedVersions, version ∈ supportedVersions
   *   - 如果 version=4, cipherMode ∈ [0, 1]（AES-128/256）
   *   - 如果 version=4, compressionMode ∈ ['none', 'zstd']
   *   - 否则 (v2/v3) cipherMode/compressionMode 省略
   */
  async function loadPlugins(): Promise<void> {
    isLoadingPlugins.value = true
    try {
      plugins.value = await fetchPlugins()
    } catch (e) {
      lastError.value = e instanceof Error ? e.message : String(e)
    } finally {
      isLoadingPlugins.value = false
    }
  }
  
  function generateTestCases(opts: {
    sourceFile: string           // mock 源文件路径
    includeDeprecated?: boolean  // 默认 true：包含 v2/v3 测试（用于回归）
  }): TestCaseSpec[] {
    const cases: TestCaseSpec[] = []
    for (const plugin of plugins.value) {
      const opts = plugin.taskOptions
      if (!opts) continue
      const versions: number[] = opts.supportVersionSelect && opts.supportedVersions
        ? opts.supportedVersions
        : [opts.defaultVersion]
      
      for (const taskType of ['encrypt', 'decrypt'] as const) {
        for (const version of versions) {
          if (!includeDeprecated && version <= 3) continue
          
          const isV4 = version === 4
          const cipherModes = isV4 && taskType === 'encrypt' ? [0, 1] : [undefined]
          const compressionModes = isV4 && taskType === 'encrypt' ? ['none' as const, 'zstd' as const] : [undefined]
          
          for (const cipherMode of cipherModes) {
            for (const compressionMode of compressionModes) {
              cases.push({
                id: `${plugin.name}-${taskType}-v${version}` +
                    (cipherMode !== undefined ? `-c${cipherMode}` : '') +
                    (compressionMode !== undefined ? `-${compressionMode}` : ''),
                taskType,
                pluginName: plugin.name,
                sourcePath: opts.includeDeprecated
                  ? sourceFile
                  : sourceFile,
                version,
                cipherMode,
                compressionMode,
                expectedBehavior: version <= 3 ? 'might-fail' : 'success',
              })
            }
          }
        }
      }
    }
    return cases
  }
  
  async function runTests(specs: TestCaseSpec[]): Promise<void> {
    isRunning.value = true
    progress.value = { total: specs.length, completed: 0, passed: 0, failed: 0 }
    results.value = []
    
    for (const spec of specs) {
      const result: TestCaseResult = { spec, status: 'running' }
      results.value = [...results.value, result]
      const start = Date.now()
      
      try {
        // 真机安全边界：强制改写到 encv-automation
        const safeSource = withSafetyBoundary(spec.sourcePath, { forceAutomation: true })
        const task = await createTask(
          spec.taskType,
          safeSource,
          undefined,                            // targetPath 让后端决定
          'automation-test-pwd',                // 全局 password
          spec.version,
          spec.pluginName,
          {},
          undefined,
          spec.cipherMode,
          spec.compressionMode,
        )
        recordTriggeredBy(task.id, 'automation')  // ← 触发者标签登记
        result.taskId = task.id
        result.status = 'pending'                // 任务已提交，等 WS 回调
        result.durationMs = Date.now() - start
        progress.value.passed++
      } catch (e) {
        result.status = 'failed'
        result.error = e instanceof Error ? e.message : String(e)
        result.durationMs = Date.now() - start
        progress.value.failed++
      }
      
      progress.value.completed++
    }
    
    isRunning.value = false
  }
  
  return {
    plugins,
    isLoadingPlugins,
    isRunning,
    progress,
    results,
    lastError,
    loadPlugins,
    generateTestCases,
    runTests,
  }
}
```

### 3.6 自动化测试详情页

**新建**：[src/views/AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue)

```vue
<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.automationTests') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- Mock 数据管理区 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.mockDataManager') }}</ion-label>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.mockDataManagerHint') }}</p>
        
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.mockRoot') }}</h3>
            <p><code>{{ mockRoot }}</code></p>
          </ion-label>
        </ion-item>
        
        <ion-item button @click="handleGenerateMock" :disabled="isGenerating">
          <ion-icon :icon="addCircleOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.generateMock') }}</h3>
            <p>{{ t('devtools.generateMockDesc') }}</p>
          </ion-label>
          <ion-spinner v-if="isGenerating" slot="end" name="dots"></ion-spinner>
        </ion-item>
        
        <ion-item button @click="handleResetMock" :disabled="isResetting">
          <ion-icon :icon="trashOutline" slot="start" color="danger"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.resetMock') }}</h3>
            <p>{{ t('devtools.resetMockDesc') }}</p>
          </ion-label>
          <ion-spinner v-if="isResetting" slot="end" name="dots"></ion-spinner>
        </ion-item>
        
        <div v-if="mockStats" class="mock-stats-card">
          <div class="stat-row">
            <span>{{ t('devtools.fileCount') }}</span><span>{{ mockStats.count }}</span>
          </div>
          <div class="stat-row">
            <span>{{ t('devtools.totalSize') }}</span><span>{{ humanSize(mockStats.totalSize) }}</span>
          </div>
        </div>
      </ion-list>
      
      <!-- 自动化测试运行器 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.testRunner') }}</ion-label>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.testRunnerHint') }}</p>
        
        <ion-item button @click="handleLoadPlugins" :disabled="isLoadingPlugins">
          <ion-icon :icon="syncOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.loadPlugins') }}</h3>
            <p>{{ plugins.length > 0 ? `${plugins.length} ${t('devtools.pluginsLoaded')}` : t('devtools.notLoaded') }}</p>
          </ion-label>
        </ion-item>
        
        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.testCases') }}</h3>
            <p>{{ testCaseCount }} {{ t('devtools.testCasesGenerated') }}</p>
          </ion-label>
        </ion-item>
        
        <ion-item button @click="handleRunTests" :disabled="isRunning || testCaseCount === 0" detail>
          <ion-icon :icon="playCircleOutline" slot="start" color="success"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.runAllTests') }}</h3>
            <p>{{ t('devtools.runAllTestsDesc') }}</p>
          </ion-label>
        </ion-item>
        
        <div v-if="progress.total > 0" class="progress-card">
          <ion-progress-bar :value="progress.completed / progress.total" />
          <div class="progress-stats">
            <span>{{ progress.completed }} / {{ progress.total }}</span>
            <span color="success">{{ progress.passed }} ✓</span>
            <span color="danger">{{ progress.failed }} ✗</span>
          </div>
        </div>
        
        <ion-list v-if="results.length > 0" class="results-list">
          <ion-item v-for="r in results" :key="r.spec.id">
            <ion-icon :icon="getResultIcon(r)" :color="getResultColor(r)" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ r.spec.id }}</h3>
              <p class="result-meta">
                <ion-badge :color="getResultColor(r)" size="small">{{ r.status }}</ion-badge>
                <span v-if="r.durationMs">{{ r.durationMs }}ms</span>
                <span v-if="r.taskId" class="task-id-ref">#{{ r.taskId.slice(0, 6) }}</span>
              </p>
              <p v-if="r.error" class="error-text">{{ r.error }}</p>
            </ion-label>
          </ion-item>
        </ion-list>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
// ... 实现细节
// 调用 useAutomationTests() 提供的 loadPlugins / generateTestCases / runTests
// 调 usePathResolver.withSafetyBoundary 处理路径
// 调 useMockGenerator.generateMockFilesViaBackend / resetMockFilesViaBackend
</script>
```

### 3.7 任务触发者标签

#### 3.7.1 触发者持久化

**新建**：[src/composables/useTaskTrigger.ts](file:///workspace/app/encv-mobile/src/composables/useTaskTrigger.ts)

```typescript
import type { TriggeredBy } from '@/composables/useAutomationTests'

const STORAGE_KEY = 'encv_task_triggered_by'

interface TriggeredByMap {
  [taskId: string]: { triggeredBy: TriggeredBy; recordedAt: string }
}

function readMap(): TriggeredByMap {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}')
  } catch {
    return {}
  }
}

function writeMap(m: TriggeredByMap): void {
  // 限制最多 500 条，防止 localStorage 撑爆
  const entries = Object.entries(m).sort(
    (a, b) => b[1].recordedAt.localeCompare(a[1].recordedAt)
  ).slice(0, 500)
  const trimmed = Object.fromEntries(entries)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(trimmed))
}

export function recordTriggeredBy(taskId: string, triggeredBy: TriggeredBy): void {
  const m = readMap()
  m[taskId] = { triggeredBy, recordedAt: new Date().toISOString() }
  writeMap(m)
}

export function getTriggeredBy(taskId: string): TriggeredBy {
  const m = readMap()
  return m[taskId]?.triggeredBy ?? 'user'
}

export function clearTriggeredBy(): void {
  localStorage.removeItem(STORAGE_KEY)
}
```

**为什么是前端 localStorage 而非后端透传**：
- 当前 `POST /api/tasks` 不接受 `triggeredBy` 字段
- 修改 Go 后端 `task` struct 属于越界（不在开发者选项 / 移动端 scope 内）
- localStorage 方案：100% 前端实现，立即可用，0 后端侵入
- 已知限制：清理 localStorage / 换设备后丢失——但这是辅助标识，不是核心功能

#### 3.7.2 useNewTaskModal 登记触发者

**修改**：[src/composables/useNewTaskModal.ts](file:///workspace/app/encv-mobile/src/composables/useNewTaskModal.ts) L167 后：

```typescript
await createTask(/* ... */)
await modal.dismiss()
showToast({ ... })
eventBus.emit('task:refresh', {})
// ← 新增：用户手动创建的任务登记为 'user'
recordTriggeredBy(task.id, 'user')  // 需先从 createTask 返回值取 task.id
```

**问题**：[useNewTaskModal.ts:153](file:///workspace/app/encv-mobile/src/composables/useNewTaskModal.ts#L153) `createTask(...)` 返回值未使用。修改为：
```typescript
const task = await createTask(/* ... */)
recordTriggeredBy(task.id, 'user')
```

#### 3.7.3 Tasks.vue 任务卡片显示触发者 badge

**修改**：[src/views/Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue) L160 后（在 `plugin-badge` 之后）：

```vue
<ion-badge :color="getTriggeredByColor(task.id)" class="triggered-by-badge">
  <ion-icon :icon="getTriggeredByIcon(task.id)" class="triggered-by-icon"></ion-icon>
  {{ t(`tasks.triggeredBy_${getTriggeredBy(task.id)}`) }}
</ion-badge>
```

**新增 import**：`getTriggeredBy`, `recordTriggeredBy` from `@/composables/useTaskTrigger`

**新增 helpers**：
```typescript
function getTriggeredBy(taskId: string): TriggeredBy {
  return getTriggeredBy(taskId)
}
function getTriggeredByColor(taskId: string): string {
  const t = getTriggeredBy(taskId)
  return t === 'automation' ? 'primary' : t === 'ai_agent' ? 'secondary' : 'medium'
}
function getTriggeredByIcon(taskId: string): string {
  return /* robotOutline | personOutline | hardwareChipOutline */
}
```

**新增 style**：`.triggered-by-badge { font-size: 10px; }`，`.triggered-by-icon { font-size: 11px; }`

#### 3.7.4 TaskDetailModal 显示触发者

**修改**：[src/components/TaskDetailModal.vue](file:///workspace/app/encv-mobile/src/components/TaskDetailModal.vue)（需要先 Read 确认结构）

在 task info 区域加一行：
```vue
<div class="detail-row">
  <span class="detail-label">{{ t('tasks.triggeredBy') }}</span>
  <ion-badge :color="getTriggeredByColor(task.id)">
    <ion-icon :icon="getTriggeredByIcon(task.id)"></ion-icon>
    {{ t(`tasks.triggeredBy_${getTriggeredBy(task.id)}`) }}
  </ion-badge>
</div>
```

### 3.8 i18n 键

**修改**：[src/composables/useI18n.ts](file:///workspace/app/encv-mobile/src/composables/useI18n.ts) 的 `devtools` / `tasks` 命名空间追加：

```typescript
// devtools
automationTests: '自动化测试',
automationTestsHint: '后端 API 任务流自动化测试，可在发行版本使用',
automationTestsEntry: '运行自动化测试',
automationTestsEntryDesc: '生成测试用例并提交到后端执行',
availableInProd: '生产可用',
mockDataManager: 'Mock 数据',
mockDataManagerHint: '生成或重置用于自动化测试的 Mock 文件。dev 写到 __mock_data__/，真机改写到 /storage/emulated/0/encv-automation/',
mockRoot: 'Mock 根目录',
generateMock: '生成 Mock 数据',
generateMockDesc: '创建 01-plain-media / 02-alist-encrypt / 03-encv-containers / 04-boundary-test',
resetMock: '重置 Mock 数据',
resetMockDesc: '清空上述目录中的所有文件，不影响其他目录',
fileCount: '文件数',
totalSize: '总大小',
testRunner: '测试运行器',
testRunnerHint: '动态生成测试用例（plugin × type × version × cipher × compression）',
loadPlugins: '加载 plugins',
pluginsLoaded: '个 plugin 已加载',
notLoaded: '未加载',
testCases: '测试用例',
testCasesGenerated: '个用例（已动态生成）',
runAllTests: '运行全部测试',
runAllTestsDesc: '逐个提交任务，触发者标签 = 自动化',

// tasks
triggeredBy: '触发者',
triggeredBy_user: '用户',
triggeredBy_automation: '自动化',
triggeredBy_ai_agent: 'AI 智能体',
```

### 3.9 单元测试

**新建**：[src/composables/__tests__/useAutomationTests.test.ts](file:///workspace/app/encv-mobile/src/composables/__tests__/useAutomationTests.test.ts)

覆盖：
- `generateTestCases` 笛卡尔积正确性
  - 单 plugin + 单一 version → 用例数 = 2 (encrypt + decrypt)
  - v4 plugin + encryption → 用例数 = 2 (cipher=0,1) × 2 (compression=none,zstd) × 2 (encrypt,decrypt) = 8
  - v2/v3 plugin（无 v4）→ cipherMode/compressionMode 全为 undefined
  - 不传 includeDeprecated → v2/v3 跳过
- `withSafetyBoundary` 路径改写
  - dev 模式 → 路径不变
  - 真机 /storage/emulated/0/Download/foo.txt → /storage/emulated/0/encv-automation/Download/foo.txt
  - 真机 /storage/emulated/0/encv-automation/foo.txt → 已是命名空间内，不变
  - 非 /storage/emulated/0 路径 → 不变
  - `forceAutomation: true` → 任意路径都强制改写

**新建**：[src/composables/__tests__/useTaskTrigger.test.ts](file:///workspace/app/encv-mobile/src/composables/__tests__/useTaskTrigger.test.ts)

覆盖：
- `recordTriggeredBy` + `getTriggeredBy` roundtrip
- localStorage 容量上限 500 条（模拟 600 次写入，验证只保留最新 500）
- `clearTriggeredBy` 清空

**新建**：[src/lib/__tests__/mockDataGenerator.test.ts](file:///workspace/app/encv-mobile/src/lib/__tests__/mockDataGenerator.test.ts)

覆盖：
- `collectSpecs('plain').length > 0` 且相对路径含 `01-plain-media`
- `collectSpecs('ae').length > 0` 且扩展名 `.ae`
- `createJPEG()` 头 2 字节 = `0xFF 0xD8`
- `createPNG()` 前 8 字节是 PNG signature
- `generateMockFiles({ writeToDisk: vi.fn() })` 调用的 IO 次数 = specs.length

### 3.10 E2E 验证

**新建**：[src/views/__tests__/AutomationTestsDetail.test.ts](file:///workspace/app/encv-mobile/src/views/__tests__/AutomationTestsDetail.test.ts)（如果现有视图测试存在则同模式）

或**手动验证清单**：
1. 真机 (release APK) 进入 Settings → 开发者选项 → 自动化测试
2. 点"生成 Mock" → Toast 成功 → adb shell ls /storage/emulated/0/encv-automation/01-plain-media/ 看到 photo.jpg 等
3. 点"重置 Mock" → 文件被清空
4. 点"加载 plugins" → 显示 plugin 数
5. 点"运行全部测试" → 进度条推进 → Tasks tab 看到 N 个新任务
6. Tasks tab 卡片显示 "自动化" badge（primary 色 + robot icon）
7. 自动化测试即使 source 是 `/storage/emulated/0/Download/real.txt` 也被改写到 `/storage/emulated/0/encv-automation/Download/real.txt`，**用户真实数据完全不动**

---

## 4. Assumptions & Decisions

### 4.1 关键决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 入口生产可用性 | **是** | 用户原话"在发行版本中也是可用的"，且安全边界足够（`withSafetyBoundary` 强制改写） |
| Mock 生成复用方式 | **提取为 `src/lib/mockDataGenerator.ts` 纯函数模块** | 前后端共用；不依赖 fs/child_process；CLI 端通过回调写盘 |
| 路径安全改写时机 | **`usePathResolver.withSafetyBoundary` 集中拦截** | 单一拦截点，避免散落在 useNewTaskModal / useAutomationTests 各处 |
| 触发者字段存储 | **前端 localStorage** | 不越界修改 Go 后端 task struct；100% 前端实现；任务被删除后条目自然失效 |
| 触发者默认值 | **`'user'`** | 任何未登记的任务 ID 都视为用户手动创建，向后兼容旧任务 |
| 动态组合实现 | **`fetchPlugins()` × `taskOptions.supportedVersions` × v4 cipher/compression 笛卡尔积** | 零硬编码；新增 plugin / 新 version 自动覆盖 |
| 自动化 mock 源文件 | **默认用 `01-plain-media/video/sample.mp4`** | 跨 plugin 通用的有效文件；mp4 是大多数 plugin 支持的格式 |
| 真机 mock 写入路径 | **`/storage/emulated/0/encv-automation/`** | 用户原话"真机用 encv-automation"；与 `withSafetyBoundary` 行为对齐 |
| 后端 mock API 实现 | **Go 新增 `/api/mock/generate` (SSE) + `/api/mock/reset`** | 前端无法直写磁盘到真机；通过后端走服务进程的权限更稳定 |
| 真机 IO 安全 | **后端白名单校验 root 前缀** | dev 允许 `__mock_data__/`；真机只允许 `/storage/emulated/0/encv-automation/` |
| TaskDetailModal 触发者 badge | **同 Tasks.vue 卡片样式** | 视觉一致；用户原话"任务卡片和详情新增触发者标签" |

### 4.2 风险与缓解

| 风险 | 缓解 |
|------|------|
| localStorage 撑爆（500 条限制） | 写入时按 `recordedAt` 排序裁剪；500 条是 500 字符 × 几百字节 ~ 几百 KB |
| 用户误用自动化测试入口损坏数据 | `withSafetyBoundary` 强制改写 + 后端 root 白名单，双重保险 |
| 后端 mock API 被滥用 | root 前缀白名单 + Go 后端单测覆盖白名单逻辑 |
| v2/v3 测试用例大多会失败（已废弃） | `expectedBehavior: 'might-fail'` 标记；进度条不把 v2/v3 计入 failed |
| `useAutomationTests` 调用频次失控 | 进度条 + "运行中"禁用按钮；用户主动取消需 stop flag（v1 暂不实现） |
| `fetchPlugins` 在 release 不可用？ | `fetchPlugins` 走 `/api/plugins`，无 isDev 限制，生产可用 |

### 4.3 不做的事

- ❌ 不修改 Go 后端 `task` struct（v1 用 localStorage 方案）
- ❌ 不在 useNewTaskModal 加 isDev 限制（开发者选项要生产可用）
- ❌ 不实现 stop/cancel 自动化测试运行（v1 简单轮询，进度可见即可）
- ❌ 不实现测试用例的导入/导出（v1 用默认动态组合）
- ❌ 不集成 CI（v1 是手动工具入口；后端 mock API 单测覆盖）
- ❌ 不修改 EncvTask 类型（前端触发者标识完全在 useTaskTrigger 内部）

---

## 5. Verification

### 5.1 单元测试

```bash
cd /workspace/app/encv-mobile
pnpm test:run
```

预期：
- 现有 776/776 仍全绿
- 新增 ≥ 15 个用例（useAutomationTests × 6、useTaskTrigger × 3、mockDataGenerator × 5、usePathResolver 安全边界已含 × 5）
- 总数 ≥ 791

### 5.2 类型检查

```bash
pnpm exec vue-tsc --noEmit
```

预期：0 错误。

### 5.3 构建

```bash
pnpm build
```

预期：vite build 成功，生成 production bundle 含 `AutomationTestsDetail` chunk。

### 5.4 真机 smoke test

```bash
# 1. 生成 release APK
cd /workspace/app/encv-mobile/android
./gradlew assembleRelease

# 2. 安装到真机
adb install -r app/build/outputs/apk/release/app-release.apk

# 3. 启动 app，进入 Settings → 开发者选项 → 自动化测试

# 4. 点"生成 Mock"
adb shell ls /storage/emulated/0/encv-automation/01-plain-media/
# 预期：photo.jpg, screenshot.png, sample.mp4, ...

# 5. 点"重置 Mock"
adb shell ls /storage/emulated/0/encv-automation/
# 预期：No such file or directory

# 6. 重新生成，点"加载 plugins"（应显示 ≥ 3 个 plugin）
# 7. 点"运行全部测试"
# 8. 切到 Tasks tab
# 9. 卡片显示 "🤖 自动化" badge (primary 色)

# 10. 验证安全边界：在测试中故意设 source='/storage/emulated/0/Download/important.jpg'
adb shell ls /storage/emulated/0/Download/important.jpg
# 预期：文件仍然存在（用户数据未损坏）
adb shell ls /storage/emulated/0/encv-automation/Download/
# 预期：important.jpg 出现在这里
```

### 5.5 CLI 验证（generate-mock-files.ts 重构后不破坏 CLI）

```bash
cd /workspace/app/encv-mobile
ENCV_MOCK_ROOT=/tmp/encv-test npx tsx scripts/generate-mock-files.ts
ls -la /tmp/encv-test/01-plain-media/
file /tmp/encv-test/01-plain-media/video/sample.mp4
file /tmp/encv-test/01-plain-media/image/photo.jpg
# 预期：照片、mp4 全部正常生成
```

---

## 6. 实现顺序

按依赖关系排序（可并行批次已标注）：

### Batch A（并行）
1. §3.2 提取 `src/lib/mockDataGenerator.ts` + §3.2.2 改写 generate-mock-files.ts
2. §3.4 `usePathResolver.withSafetyBoundary` + §3.9 usePathResolver 测试补充
3. §3.7.1 `useTaskTrigger` composable + 测试

### Batch B（依赖 A）
4. §3.5 `useAutomationTests` composable + 测试
5. §3.6 `AutomationTestsDetail.vue` 视图（mock 生成 + 测试运行器）
6. §3.7.2 useNewTaskModal 集成 recordTriggeredBy
7. §3.7.3 Tasks.vue + §3.7.4 TaskDetailModal 触发者 badge

### Batch C（依赖 B）
8. §3.1 路由 + DevToolsDetail.vue 入口
9. §3.3 后端 `/api/mock/generate` / `/api/mock/reset` Go 端点 + 注册 + 单测
10. §3.8 i18n 键（中英文）
11. §3.2.3 `api/mockGenerator.ts` 前端调用后端

### Batch D（最终验证）
12. §5 全部验证步骤

预计新增/修改文件：
- 新建 5 个：`src/lib/mockDataGenerator.ts`、`src/api/mockGenerator.ts`、`src/composables/useAutomationTests.ts`、`src/composables/useTaskTrigger.ts`、`src/views/AutomationTestsDetail.vue`
- 新建 4 个测试：`mockDataGenerator.test.ts`、`useAutomationTests.test.ts`、`useTaskTrigger.test.ts`、补充 `usePathResolver` 测试
- 修改 7 个：`scripts/generate-mock-files.ts`、`src/composables/usePathResolver.ts`、`src/composables/useNewTaskModal.ts`、`src/views/Tasks.vue`、`src/views/DevToolsDetail.vue`、`src/components/TaskDetailModal.vue`、`src/router/index.ts`
- 新建/修改 Go 后端 2 个：`internal/server/mock_generator.go`、`internal/server/server.go`（路由注册）+ `internal/server/mock_generator_test.go`
- 修改 i18n 1 个：`src/composables/useI18n.ts`

总计：5 新文件 + 7 修改 + 4 测试 + 3 Go 文件 = 19 处变更
