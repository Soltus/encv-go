# MPV 播放器插件状态缺失自动 fallback 方案

## 问题分析

### 核心问题
用户在以下情况下点击视频播放导致白屏无反馈：
1. MPV 插件未安装
2. MPV 插件已安装但未启用
3. MPV 插件已启用但未加载到内存（ComboLite 两阶段模型）

### 根因分析

**饱和调试原则违反**：整个调用链没有状态检查和日志输出：

1. **Files.vue L459-484 `playMedia()`**：
   - 直接调用 `openPlayer()` 不检查插件可用性
   - 无任何状态验证

2. **PlayerEntry.kt L55-84 `startMpvPlayer()`**：
   - 调用 `ensurePluginLoaded()` 但异常被静默吞掉
   - `createProxyIntent()` 直接 startActivity，不验证插件是否真正加载

3. **PluginLifecycleEngine.kt L143-151 `ensurePluginLoaded()`**：
   - 失败时静默吞异常，无日志
   - 不返回布尔值，调用方无法知道结果

4. **EncvComboLiteHost.kt L17-18 `isPluginAvailable()`**：
   - 只检查 `enabled` 状态，不检查是否已加载到内存

### 白屏原因
`EncvHostActivity` 启动后 ProxyManager 找不到插件 Activity，因为插件未加载，导致渲染空白。

## 修复方案：自动 Fallback + 状态显示

### 核心思路
- **不使用 Toast 提示**（违反用户原则）
- **播放时自动 fallback**：MPV 不可用 → 自动切换到 Artplayer（内置播放器）
- **设置页面状态显示**：让用户知道 MPV 插件当前状态
- **日志输出**：关键路径必须有 Log.i 输出

### 修复层级

| 层级 | 文件 | 修复内容 |
|------|------|---------|
| 前端 UI | Settings.vue | 显示 MPV 插件状态（已安装/未安装/已禁用） |
| 前端逻辑 | Files.vue | 播放前检查可用性，不可用则 fallback 到 Artplayer |
| Kotlin 面 | EncvComboLiteHost.kt | 完善状态查询 API |
| Kotlin 引擎 | PluginLifecycleEngine.kt | `ensurePluginLoaded()` 返回布尔值 + 日志 |

### 详细修复步骤

#### Step 1: EncvComboLiteHost.kt — 完善状态查询

```kotlin
// 当前 L17-18 只检查 enabled
fun isPluginAvailable(pluginId: String): Boolean =
    getInstalledPlugins().any { it.id == pluginId && it.enabled }

// 修复：检查完整状态（installed + enabled + loaded）
fun isPluginAvailable(pluginId: String): Boolean {
    if (!PluginLifecycleEngine.isInitialized()) return false
    val state = getPluginInfo(pluginId)
    return state != null && state.installed && state.enabled && PluginLifecycleEngine.isPluginLoaded(pluginId)
}

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
```

#### Step 2: PluginLifecycleEngine.kt — ensurePluginLoaded 返回布尔值 + 日志

```kotlin
// 当前 L143-151 静默吞异常
fun ensurePluginLoaded(pluginId: String) {
    if (!PluginManager.isInitialized) return
    try {
        if (PluginManager.getPluginInfo(pluginId) == null) {
            runBlocking { launchPlugin(pluginId) }
        }
    } catch (e: Exception) { } // 无日志
}

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

#### Step 3: PlayerEntry.kt — 启动前检查，失败返回布尔让调用方 fallback

```kotlin
// 当前 L55-84 静默吞异常
private fun startMpvPlayer(...) {
    try {
        EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
        val intent = EncvComboLiteHost.createProxyIntent(...)
        context.startActivity(intent)
    } catch (e: Exception) {
        Log.e(TAG, "Failed to start MPV player plugin", e)
        Toast.makeText(...) // ← 删除 Toast
    }
}

// 修复：返回布尔值，让调用方决定 fallback
fun startMpvPlayer(
    context: Context,
    filePath: String,
    fileName: String,
    mimeType: String,
    isExternal: Boolean
): Boolean {
    Log.i(TAG, "startMpvPlayer: filePath=$filePath")

    // 1. 检查框架初始化
    if (!EncvComboLiteHost.isInitialized) {
        Log.w(TAG, "startMpvPlayer: ComboLite not initialized")
        return false
    }

    // 2. 检查插件完整状态
    val available = EncvComboLiteHost.isPluginAvailable(PLUGIN_ID)
    if (!available) {
        Log.w(TAG, "startMpvPlayer: MPV plugin not available (state=${EncvComboLiteHost.getPluginFullState(PLUGIN_ID).status})")
        return false
    }

    // 3. 确保插件加载
    val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
    if (!loaded) {
        Log.w(TAG, "startMpvPlayer: MPV plugin load failed")
        return false
    }

    // 4. 启动播放
    return try {
        val intent = EncvComboLiteHost.createProxyIntent(
            context = context,
            pluginId = PLUGIN_ID,
            targetActivity = TARGET_ACTIVITY,
            hostActivityClass = EncvHostActivity::class.java,
            extras = mapOf(...)
        )
        context.startActivity(intent)
        Log.i(TAG, "startMpvPlayer: launched successfully")
        true
    } catch (e: Exception) {
        Log.e(TAG, "startMpvPlayer: startActivity failed", e)
        false
    }
}
```

#### Step 4: GoProcessPlugin.kt — 新增状态查询 API

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

@PluginMethod
fun isPluginAvailable(call: PluginCall) {
    val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
    val available = EncvComboLiteHost.isPluginAvailable(pluginId)
    call.resolve(JSObject().apply { put("available", available) })
}
```

#### Step 5: GoProcess.ts — 前端 API

```typescript
export async function getPluginFullState(pluginId: string): Promise<PluginFullState> {
    try {
        const result = await GoProcess.getPluginFullState({ pluginId })
        return result
    } catch (e) {
        console.error('[GoProcess] getPluginFullState failed:', e)
        return { id: pluginId, status: 'error', name: '', version: '' }
    }
}

export async function isPluginAvailable(pluginId: string): Promise<boolean> {
    try {
        const result = await GoProcess.isPluginAvailable({ pluginId })
        return result.available
    } catch (e) {
        console.error('[GoProcess] isPluginAvailable failed:', e)
        return false
    }
}
```

#### Step 6: Files.vue — 播放前检查 + 自动 fallback

```typescript
// 当前 L459-484 无检查
async function playMedia(file: FileItem, category: string) {
    const mode = getPlayMode(category)
    switch (mode) {
        case PLAY_MODE.MPV_PLUGIN:
            if (isNative()) {
                openPlayer(file, file.name, mimeType, PLAY_MODE.MPV_PLUGIN)
            } else {
                router.push({ path: '/preview', ... })
            }
            break
        // ...
    }
}

// 修复：检查可用性，不可用则自动 fallback
async function playMedia(file: FileItem, category: string) {
    const mode = getPlayMode(category)
    const mimeType = category === 'video' ? 'video/*' : 'audio/*'

    if (mode === PLAY_MODE.MPV_PLUGIN && isNative()) {
        // 检查 MPV 插件可用性
        const available = await isPluginAvailable('com.encvgo.plugin.mpv')
        if (!available) {
            console.info('[Files] MPV plugin not available, fallback to Artplayer')
            // 自动 fallback 到内置播放器，无 Toast
            router.push({
                path: '/preview',
                query: { path: file.path, name: file.name }
            })
            return
        }
        // 可用时正常播放
        openPlayer(file, file.name, mimeType, PLAY_MODE.MPV_PLUGIN)
    } else if (mode === PLAY_MODE.ARTPLAYER) {
        router.push({ path: '/preview', ... })
    } else if (mode === PLAY_MODE.EXTERNAL) {
        // ...
    }
}
```

#### Step 7: Settings.vue — 显示 MPV 插件状态

```vue
<ion-item>
    <ion-select
        :value="videoPlayerMode"
        @ionChange="handleVideoPlayerChange"
        :label="t('settings.videoPlayer')"
        ...
    >
        <ion-select-option value="artplayer">{{ t('settings.builtInArtplayer') }}</ion-select-option>
        <ion-select-option value="mpv-plugin" :disabled="mpvPluginStatus !== 'ready'">
            {{ t('settings.mpvPluginExtension') }}
            <span v-if="mpvPluginStatus === 'not_installed'" style="color: var(--ion-color-warning)">
                (未安装)
            </span>
            <span v-if="mpvPluginStatus === 'disabled'" style="color: var(--ion-color-warning)">
                (已禁用)
            </span>
            <span v-if="mpvPluginStatus === 'not_loaded'" style="color: var(--ion-color-warning)">
                (未加载)
            </span>
        </ion-select-option>
        <ion-select-option value="external">{{ t('settings.openExternal') }}</ion-select-option>
    </ion-select>
</ion-item>
```

```typescript
const mpvPluginStatus = ref('unknown')

onMounted(async () => {
    if (isNative()) {
        const state = await getPluginFullState('com.encvgo.plugin.mpv')
        mpvPluginStatus.value = state.status
    }
})
```

## 任务清单

- [ ] Task 1: EncvComboLiteHost.kt — 完善状态查询 API
  - [ ] SubTask 1.1: 新增 `getPluginFullState()` 方法
  - [ ] SubTask 1.2: 修复 `isPluginAvailable()` 检查完整状态
- [ ] Task 2: PluginLifecycleEngine.kt — ensurePluginLoaded 返回布尔值 + 日志
  - [ ] SubTask 2.1: `ensurePluginLoaded()` 返回 Boolean
  - [ ] SubTask 2.2: 添加 Log.i 日志输出
  - [ ] SubTask 2.3: 新增 `isPluginLoaded()` 方法
- [ ] Task 3: PlayerEntry.kt — 启动前检查，失败返回布尔
  - [ ] SubTask 3.1: `startMpvPlayer()` 返回 Boolean
  - [ ] SubTask 3.2: 删除 Toast 提示
  - [ ] SubTask 3.3: 添加完整状态检查和日志
- [ ] Task 4: GoProcessPlugin.kt — 新增状态查询 API
  - [ ] SubTask 4.1: 新增 `getPluginFullState()` @PluginMethod
  - [ ] SubTask 4.2: 新增 `isPluginAvailable()` @PluginMethod
- [ ] Task 5: GoProcess.ts — 前端 API 封装
  - [ ] SubTask 5.1: 新增 `getPluginFullState()` 函数
  - [ ] SubTask 5.2: 新增 `isPluginAvailable()` 函数
- [ ] Task 6: Files.vue — 播放前检查 + 自动 fallback
  - [ ] SubTask 6.1: `playMedia()` 检查 MPV 可用性
  - [ ] SubTask 6.2: 不可用时自动 fallback 到 Artplayer
- [ ] Task 7: Settings.vue — 显示 MPV 插件状态
  - [ ] SubTask 7.1: 新增 `mpvPluginStatus` ref
  - [ ] SubTask 7.2: onMounted 时查询插件状态
  - [ ] SubTask 7.3: MPV 选项显示状态标签

## 验证标准

1. **MPV 未安装时点击视频**：自动打开 Artplayer 播放（无 Toast，无白屏）
2. **MPV 已禁用时点击视频**：自动打开 Artplayer 播放（无 Toast，无白屏）
3. **MPV 未加载时点击视频**：自动加载或 fallback 到 Artplayer
4. **设置页面**：MPV 播放器选项旁显示状态（未安装/已禁用/已加载）
5. **日志输出**：每个关键路径有 Log.i 输出，便于调试

## 风险评估

- **低风险**：仅添加状态检查和 fallback 逻辑，不影响正常播放流程
- **向后兼容**：Artplayer 模式完全不受影响
- **性能影响**：`isPluginAvailable()` 检查是内存操作，无 IO 开销