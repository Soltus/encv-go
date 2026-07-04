# MPV 播放器插件状态检查缺失导致白屏问题修复计划

## 问题分析

### 核心问题

用户在以下三种情况下选择 MPV 播放视频均导致白屏，没有任何提示：
1. **MPV 插件未安装** — 用户从未安装 mpv-plugin
2. **MPV 插件已安装但未启用** — 用户禁用了插件
3. **MPV 插件已启用但未加载** — 插件启用但未加载到内存（ComboLite 两阶段模型）

### 根因分析

**违反饱和调试原则**：整个调用链没有任何状态检查和错误提示：

1. **前端 Files.vue L459-485 `playMedia()`**：
   - 直接调用 `openPlayer()` 而不检查插件是否可用
   - 没有任何错误处理或用户提示

2. **Kotlin PlayerEntry.kt L55-84 `startMpvPlayer()`**：
   - 调用 `ensurePluginLoaded()` 但不检查返回结果（该方法静默吞掉异常）
   - `createProxyIntent()` 后直接 `startActivity()`，不验证插件是否真正加载

3. **PluginLifecycleEngine.kt L143-151 `ensurePluginLoaded()`**：
   - 失败时静默吞掉异常，不返回状态
   - 无日志记录

4. **EncvComboLiteHost.kt L17-18 `isPluginAvailable()`**：
   - 只检查 `enabled` 状态，不检查是否已加载到内存

### 白屏原因

`EncvHostActivity` 启动后，ProxyManager 找不到插件 Activity（因为插件未加载），导致渲染空白界面。

---

## 修复方案

### 修复层级

| 层级 | 文件 | 修复内容 |
|------|------|---------|
| 前端 | Files.vue | 播放前检查插件可用性，不可用时提示并引导用户 |
| Kotlin 门面 | PlayerEntry.kt | 启动前检查插件状态，失败时 Toast 提示 |
| Kotlin 引擎 | PluginLifecycleEngine.kt | `ensurePluginLoaded()` 返回布尔值 + 日志 |
| Kotlin API | EncvComboLiteHost.kt | `isPluginAvailable()` 检查完整状态 |

### 详细修复步骤

#### Step 1: EncvComboLiteHost.kt — 完善状态检查

```kotlin
// 当前 L17-18 只检查 enabled
fun isPluginAvailable(pluginId: String): Boolean =
    getInstalledPlugins().any { it.id == pluginId && it.enabled }

// 修复：检查 installed + enabled + loaded
fun isPluginAvailable(pluginId: String): Boolean {
    if (!PluginLifecycleEngine.isInitialized()) return false
    val state = getPluginInfo(pluginId)
    return state != null && state.installed && state.enabled && PluginLifecycleEngine.isPluginLoaded(pluginId)
}

// 新增：检查插件是否已加载到内存
fun isPluginLoaded(pluginId: String): Boolean = PluginLifecycleEngine.isPluginLoaded(pluginId)
```

#### Step 2: PluginLifecycleEngine.kt — ensurePluginLoaded 返回状态

```kotlin
// 当前 L143-151 静默吞掉异常
fun ensurePluginLoaded(pluginId: String) {
    if (!PluginManager.isInitialized) return
    try {
        if (PluginManager.getPluginInfo(pluginId) == null) {
            runBlocking { launchPlugin(pluginId) }
        }
    } catch (e: Exception) {}
}

// 修复：返回布尔值 + 日志
fun ensurePluginLoaded(pluginId: String): Boolean {
    if (!PluginManager.isInitialized) {
        Log.w(TAG, "ensurePluginLoaded: PluginManager not initialized")
        return false
    }
    return try {
        if (PluginManager.getPluginInfo(pluginId) != null) {
            Log.i(TAG, "ensurePluginLoaded: $pluginId already loaded")
            true
        } else {
            Log.i(TAG, "ensurePluginLoaded: loading $pluginId...")
            val success = runBlocking { launchPlugin(pluginId) }
            Log.i(TAG, "ensurePluginLoaded: $pluginId load result=$success")
            success
        }
    } catch (e: Exception) {
        Log.e(TAG, "ensurePluginLoaded: $pluginId failed: ${e.message}")
        false
    }
}

// 新增：检查插件是否已加载
fun isPluginLoaded(pluginId: String): Boolean {
    if (!PluginManager.isInitialized) return false
    return PluginManager.getPluginInfo(pluginId) != null
}
```

#### Step 3: PlayerEntry.kt — 启动前检查 + Toast 提示

```kotlin
// 当前 L55-84 无状态检查
private fun startMpvPlayer(...) {
    try {
        EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
        val intent = EncvComboLiteHost.createProxyIntent(...)
        context.startActivity(intent)
    } catch (e: Exception) {
        Log.e(TAG, "Failed to start MPV player plugin", e)
        Toast.makeText(context, "MPV 插件启动失败: ${e.message}", Toast.LENGTH_LONG).show()
    }
}

// 修复：先检查可用性，不可用时明确提示
private fun startMpvPlayer(...) {
    // 1. 检查 PluginManager 是否初始化
    if (!EncvComboLiteHost.isInitialized) {
        Log.w(TAG, "startMpvPlayer: ComboLite not initialized")
        Toast.makeText(context, "播放器框架未初始化，请重启应用", Toast.LENGTH_LONG).show()
        return
    }

    // 2. 检查插件是否已安装
    val pluginState = EncvComboLiteHost.getPluginInfo(PLUGIN_ID)
    if (pluginState == null || !pluginState.installed) {
        Log.w(TAG, "startMpvPlayer: MPV plugin not installed")
        Toast.makeText(context, "MPV 播放器插件未安装，请前往扩展管理安装", Toast.LENGTH_LONG).show()
        // 可选：引导用户到扩展管理页面
        return
    }

    // 3. 检查插件是否已启用
    if (!pluginState.enabled) {
        Log.w(TAG, "startMpvPlayer: MPV plugin disabled")
        Toast.makeText(context, "MPV 播放器插件已禁用，请前往扩展管理启用", Toast.LENGTH_LONG).show()
        return
    }

    // 4. 确保插件已加载
    val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
    if (!loaded) {
        Log.e(TAG, "startMpvPlayer: MPV plugin load failed")
        Toast.makeText(context, "MPV 播放器插件加载失败，请重试或重启应用", Toast.LENGTH_LONG).show()
        return
    }

    // 5. 启动播放
    try {
        val intent = EncvComboLiteHost.createProxyIntent(...)
        context.startActivity(intent)
        Log.i(TAG, "startMpvPlayer: launched successfully")
    } catch (e: Exception) {
        Log.e(TAG, "startMpvPlayer: startActivity failed", e)
        Toast.makeText(context, "MPV 播放器启动失败: ${e.message}", Toast.LENGTH_LONG).show()
    }
}
```

#### Step 4: Files.vue — 前端预检查 + 用户引导

```typescript
// 当前 L459-485 无检查
function playMedia(file: FileItem, category: string) {
    const mode = getPlayMode(mediaType)
    switch (mode) {
        case PLAY_MODE.MPV_PLUGIN:
            if (isNative()) {
                openPlayer(file.path, file.name, mimeType, PLAY_MODE.MPV_PLUGIN)
            } else {
                router.push({ path: '/player', ... })
            }
            break
    }
}

// 修复：先检查插件可用性
async function playMedia(file: FileItem, category: string) {
    const mode = getPlayMode(mediaType)
    
    if (mode === PLAY_MODE.MPV_PLUGIN && isNative()) {
        // 调用 Kotlin 检查插件状态
        const available = await checkPluginAvailable('mpv')
        if (!available) {
            showToast('MPV 播放器插件未安装或未启用，请前往扩展管理')
            // 可选：自动跳转到扩展管理
            // router.push('/tabs/extensions')
            return
        }
        openPlayer(file.path, file.name, mimeType, PLAY_MODE.MPV_PLUGIN)
    } else {
        // ... 其他模式
    }
}
```

需要在 GoProcess.ts 添加 `checkPluginAvailable()` 方法：

```typescript
// GoProcess.ts 新增
export async function checkPluginAvailable(pluginId: string): Promise<boolean> {
    try {
        const result = await GoProcess.isPluginAvailable({ pluginId })
        return result.available
    } catch (e) {
        console.error('[ENCV] checkPluginAvailable failed:', e)
        return false
    }
}
```

GoProcessPlugin.kt 添加对应 @PluginMethod：

```kotlin
@PluginMethod
fun isPluginAvailable(call: PluginCall) {
    val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
    val available = EncvComboLiteHost.isPluginAvailable(pluginId)
    call.resolve(JSObject().apply { put("available", available) })
}
```

---

## 实施步骤

### Task 1: 修改 EncvComboLiteHost.kt
- [ ] SubTask 1.1: 新增 `isPluginLoaded()` 方法
- [ ] SubTask 1.2: 修改 `isPluginAvailable()` 检查完整状态

### Task 2: 修改 PluginLifecycleEngine.kt
- [ ] SubTask 2.1: `ensurePluginLoaded()` 返回 Boolean + 日志
- [ ] SubTask 2.2: 新增 `isPluginLoaded()` 方法

### Task 3: 修改 PlayerEntry.kt
- [ ] SubTask 3.1: `startMpvPlayer()` 添加完整状态检查
- [ ] SubTask 3.2: 每个失败分支 Toast 提示用户

### Task 4: 修改 GoProcessPlugin.kt
- [ ] SubTask 4.1: 新增 `isPluginAvailable()` @PluginMethod

### Task 5: 修改 GoProcess.ts
- [ ] SubTask 5.1: 新增 `checkPluginAvailable()` 函数
- [ ] SubTask 5.2: 更新类型定义

### Task 6: 修改 Files.vue
- [ ] SubTask 6.1: `playMedia()` 添加 MPV 插件可用性预检查
- [ ] SubTask 6.2: 不可用时 Toast 提示 + 可选引导

### Task 7: 验证
- [ ] SubTask 7.1: 未安装 MPV 时播放视频 → Toast 提示
- [ ] SubTask 7.2: 已安装但禁用时播放视频 → Toast 提示
- [ ] SubTask 7.3: 已启用但未加载时播放视频 → 自动加载或提示

---

## 验收标准

1. **未安装 MPV 插件时**：播放视频显示 Toast "MPV 播放器插件未安装，请前往扩展管理安装"
2. **已安装但禁用时**：播放视频显示 Toast "MPV 播放器插件已禁用，请前往扩展管理启用"
3. **已启用但未加载时**：自动尝试加载，失败时 Toast "MPV 播放器插件加载失败，请重试或重启应用"
4. **插件正常可用时**：正常启动播放器，无白屏
5. **日志完整**：每个检查点都有 Log.i/w/e 输出，便于调试

---

## 风险评估

- **低风险**：仅添加检查逻辑和 Toast 提示，不改变正常播放流程
- **向后兼容**：不影响 Artplayer 和 External 模式
- **性能影响**：`isPluginAvailable()` 检查是内存操作，无 IO 开销