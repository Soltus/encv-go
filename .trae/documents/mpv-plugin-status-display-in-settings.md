# MPV 播放器状态显示方案（遵循铁律）

## 铁律约束

1. **严禁自动 fallback**：MPV 不可用时不能自动切换到 Artplayer
2. **严禁 Toast 提示**：不能用 Toast 临时提示

## 正确做法

在设置页面播放器选项旁显示插件状态标签，让用户明确知道当前状态并主动选择。

## 修复方案

### 核心思路

- **设置页面状态显示**：MPV 播放器选项旁显示状态标签（未安装/已禁用/已加载）
- **选项禁用**：MPV 未就绪时禁用该选项，防止用户误选
- **日志输出**：关键路径必须有 Log.i 输出，便于调试

### 修复层级

| 层级 | 文件 | 修复内容 |
|------|------|---------|
| 前端 UI | Settings.vue | 显示 MPV 插件状态 + 禁用未就绪选项 |
| Kotlin 面 | EncvComboLiteHost.kt | 完善状态查询 API |
| Kotlin 引擎 | PluginLifecycleEngine.kt | `ensurePluginLoaded()` 返回布尔值 + 日志 |

### 详细修复步骤

#### Step 1: EncvComboLiteHost.kt — 完善状态查询

```kotlin
// 新增：获取完整状态（供前端显示）
fun getPluginFullState(pluginId: String): PluginFullState {
    if (!PluginLifecycleEngine.isInitialized()) {
        return PluginFullState(id = pluginId, status = "framework_not_ready")
    }
    val state = getPluginInfo(pluginId)
    if (state == null) {
        return PluginFullState(id = pluginId, status = "not_installed")
    }
    if (!state.enabled) {
        return PluginFullState(id = pluginId, status = "disabled", name = state.name)
    }
    val loaded = PluginLifecycleEngine.isPluginLoaded(pluginId)
    return PluginFullState(
        id = pluginId,
        status = if (loaded) "ready" else "not_loaded",
        name = state.name,
        version = state.versionName
    )
}

// 修复：检查完整状态（installed + enabled + loaded）
fun isPluginAvailable(pluginId: String): Boolean {
    if (!PluginLifecycleEngine.isInitialized()) return false
    val state = getPluginInfo(pluginId)
    return state != null && state.installed && state.enabled && PluginLifecycleEngine.isPluginLoaded(pluginId)
}
```

#### Step 2: PluginLifecycleEngine.kt — ensurePluginLoaded 返回布尔值 + 日志

```kotlin
// 修复：返回布尔值 + 日志
fun ensurePluginLoaded(pluginId: String): Boolean {
    if (!PluginManager.isInitialized) {
        Log.w(TAG, "ensurePluginLoaded($pluginId): PluginManager not initialized")
        return false
    }
    return try {
        if (PluginManager.getPluginInfo(pluginId) != null) {
            Log.i(TAG, "ensurePluginLoaded($pluginId): already loaded")
            true
        } else {
            Log.i(TAG, "ensurePluginLoaded($pluginId): loading...")
            val success = runBlocking { launchPlugin(pluginId) }
            Log.i(TAG, "ensurePluginLoaded($pluginId): load result=$success")
            success
        }
    } catch (e: Exception) {
        Log.e(TAG, "ensurePluginLoaded($pluginId): failed", e)
        false
    }
}

// 新增：检查是否已加载
fun isPluginLoaded(pluginId: String): Boolean {
    if (!PluginManager.isInitialized) return false
    return PluginManager.getPluginInfo(pluginId) != null
}
```

#### Step 3: GoProcessPlugin.kt — 新增状态查询 API

```kotlin
@PluginMethod
fun getPluginFullState(call: PluginCall) {
    val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
    val state = EncvComboLiteHost.getPluginFullState(pluginId)
    call.resolve(JSObject().apply {
        put("id", state.id)
        put("status", state.status)
        put("name", state.name ?: "")
        put("version", state.version ?: "")
    })
}
```

#### Step 4: GoProcess.ts — 前端 API

```typescript
export interface PluginFullState {
    id: string
    status: 'ready' | 'not_installed' | 'disabled' | 'not_loaded' | 'framework_not_ready' | 'error'
    name: string
    version: string
}

export async function getPluginFullState(pluginId: string): Promise<PluginFullState> {
    try {
        const result = await GoProcess.getPluginFullState({ pluginId })
        return result
    } catch (e) {
        console.error('[GoProcess] getPluginFullState failed:', e)
        return { id: pluginId, status: 'error', name: '', version: '' }
    }
}
```

#### Step 5: Settings.vue — 显示 MPV 插件状态 + 禁用未就绪选项 + 错误显示

```vue
<ion-item>
    <ion-select
        :value="videoPlayerMode"
        @ionChange="handleVideoPlayerChange"
        :label="t('settings.videoPlayer')"
        label-placement="stacked"
        interface="action-sheet"
        mode="ios"
    >
        <ion-select-option value="artplayer">{{ t('settings.builtInArtplayer') }}</ion-select-option>
        <ion-select-option 
            value="mpv-plugin" 
            :disabled="mpvPluginStatus !== 'ready'"
        >
            {{ t('settings.mpvPluginExtension') }}
            <span v-if="mpvPluginStatus === 'not_installed'" style="color: var(--ion-color-warning)">
                (未安装)
            </span>
            <span v-if="mpvPluginStatus === 'disabled'" style="color: var(--ion-color-warning)">
                (已禁用)
            </span>
            <span v-if="mpvPluginStatus === 'not_loaded'" style="color: var(--ion-color-medium)">
                (未加载)
            </span>
            <span v-if="mpvPluginStatus === 'ready'" style="color: var(--ion-color-success)">
                ✓
            </span>
            <span v-if="mpvPluginStatus === 'error'" style="color: var(--ion-color-danger)">
                (查询失败)
            </span>
            <span v-if="mpvPluginStatus === 'framework_not_ready'" style="color: var(--ion-color-warning)">
                (框架未初始化)
            </span>
        </ion-select-option>
        <ion-select-option value="external">{{ t('settings.openExternal') }}</ion-select-option>
    </ion-select>
</ion-item>
```

```typescript
const mpvPluginStatus = ref<string>('unknown')
const mpvPluginError = ref<string>('')

onMounted(async () => {
    if (isNative()) {
        try {
            const state = await getPluginFullState('com.encvgo.plugin.mpv')
            mpvPluginStatus.value = state.status
            mpvPluginError.value = ''
            console.info('[Settings] MPV plugin status:', state.status)
            
            // 扩展已启用但未加载 → 自动尝试加载
            if (state.status === 'not_loaded') {
                console.info('[Settings] MPV plugin enabled but not loaded, attempting to load...')
                const loaded = await ensurePluginLoaded('com.encvgo.plugin.mpv')
                if (loaded) {
                    mpvPluginStatus.value = 'ready'
                    console.info('[Settings] MPV plugin loaded successfully')
                } else {
                    mpvPluginStatus.value = 'load_failed'
                    mpvPluginError.value = '插件加载失败，请重启应用'
                    console.warn('[Settings] MPV plugin load failed')
                }
            }
        } catch (e: any) {
            console.error('[Settings] getPluginFullState failed:', e)
            mpvPluginStatus.value = 'error'
            mpvPluginError.value = e.message || '查询插件状态失败'
        }
    }
})
```

#### Step 6: GoProcess.ts — 补充 ensurePluginLoaded 函数

```typescript
export async function ensurePluginLoaded(pluginId: string): Promise<boolean> {
    try {
        const result = await GoProcess.ensurePluginLoaded({ pluginId })
        return result.success === true
    } catch (e) {
        console.error('[GoProcess] ensurePluginLoaded failed:', e)
        return false
    }
}
```

#### Step 7: GoProcessPlugin.kt — 补充 ensurePluginLoaded @PluginMethod

```kotlin
@PluginMethod
fun ensurePluginLoaded(call: PluginCall) {
    val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
    val success = EncvComboLiteHost.ensurePluginLoaded(pluginId)
    call.resolve(JSObject().apply { put("success", success) })
}
```

## 任务清单

- [ ] Task 1: EncvComboLiteHost.kt — 完善状态查询 API
  - [ ] SubTask 1.1: 新增 `getPluginFullState()` 方法
  - [ ] SubTask 1.2: 新增 `PluginFullState` 数据类
  - [ ] SubTask 1.3: 修复 `isPluginAvailable()` 检查完整状态
  - [ ] SubTask 1.4: 新增 `ensurePluginLoaded()` 公开方法（返回 Boolean）
- [ ] Task 2: PluginLifecycleEngine.kt — ensurePluginLoaded 返回布尔值 + 日志
  - [ ] SubTask 2.1: `ensurePluginLoaded()` 返回 Boolean
  - [ ] SubTask 2.2: 添加 Log.i 日志输出
  - [ ] SubTask 2.3: 新增 `isPluginLoaded()` 方法
- [ ] Task 3: GoProcessPlugin.kt — 新增状态查询 API
  - [ ] SubTask 3.1: 新增 `getPluginFullState()` @PluginMethod
  - [ ] SubTask 3.2: 新增 `ensurePluginLoaded()` @PluginMethod
- [ ] Task 4: GoProcess.ts — 前端 API 封装
  - [ ] SubTask 4.1: 新增 `PluginFullState` 类型定义（含 error/load_failed 状态）
  - [ ] SubTask 4.2: 新增 `getPluginFullState()` 函数（含错误处理）
  - [ ] SubTask 4.3: 新增 `ensurePluginLoaded()` 函数
- [ ] Task 5: Settings.vue — 显示 MPV 插件状态 + 禁用未就绪选项 + 错误显示 + 自动加载
  - [ ] SubTask 5.1: 新增 `mpvPluginStatus` ref + `mpvPluginError` ref
  - [ ] SubTask 5.2: onMounted 时查询插件状态（try-catch 错误处理）
  - [ ] SubTask 5.3: MPV 选项显示所有状态标签（error/framework_not_ready/load_failed）
  - [ ] SubTask 5.4: MPV 未就绪时禁用选项
  - [ ] SubTask 5.5: 扩展已启用但未加载时自动尝试加载

## 验证标准

1. **设置页面**：MPV 播放器选项旁显示所有状态（未安装/已禁用/未加载/查询失败/框架未初始化/加载失败）
2. **选项禁用**：MPV 未就绪时选项禁用，用户无法选择
3. **状态准确**：状态与实际插件状态一致
4. **错误显示**：调用失败时显示错误状态标签，不隐藏错误
5. **自动加载**：插件已启用但未加载时，自动尝试加载（这是正常流程，不是 fallback）
6. **日志输出**：关键路径有 Log.i 输出，便于调试
7. **无白屏**：用户选择 MPV 时若未就绪，选项已禁用不会触发

## 状态流转图

```
用户打开设置页面
    ↓
查询 getPluginFullState('com.encvgo.plugin.mpv')
    ↓
┌─────────────────────────────────────────────────────┐
│ 返回状态                                             │
├─────────────────────────────────────────────────────┤
│ framework_not_ready → 禁用选项 + 显示"(框架未初始化)" │
│ not_installed       → 禁用选项 + 显示"(未安装)"      │
│ disabled            → 禁用选项 + 显示"(已禁用)"      │
│ not_loaded          → 自动尝试加载                   │
│     ├─ 成功 → ready → 启用选项 + 显示"✓"            │
│     └─ 失败 → load_failed → 禁用 + 显示"(加载失败)" │
│ ready               → 启用选项 + 显示"✓"            │
│ error (调用失败)    → 禁用选项 + 显示"(查询失败)"    │
└─────────────────────────────────────────────────────┘
```

## 铁律合规说明

1. **严禁 fallback**：插件不可用时禁用选项，用户必须主动选择其他播放器
2. **严禁 Toast**：所有状态通过选项旁的标签显示，持久可见
3. **自动加载不是 fallback**：插件已启用但未加载时自动加载，是正常初始化流程，不是切换播放器

## 风险评估

- **低风险**：仅添加状态显示和选项禁用，不改变播放逻辑
- **向后兼容**：Artplayer 模式完全不受影响
- **性能影响**：状态查询仅在设置页面加载时执行一次