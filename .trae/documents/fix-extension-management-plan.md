# 修复计划：扩展管理 3 个问题

> 根因诊断来自用户提供的饱和调试输出 + AAR 反编译证据 + 全栈代码审查

---

## 问题总览与根因

### 问题 1：禁用/启用后状态不更新，toast 不规范

**现象**：点击禁用按钮后 toast 提示"已禁用"，但 UI 上扩展仍然显示为已安装+已启用状态。

**根因链**：
1. `checkInstalledPlugins()` (GoProcessPlugin.kt L443) 只返回 `{pluginId: true}` —— **不含 enabled 状态**
2. `loadExtensions()` (ExtensionsPage.vue L184) **硬编码** `enabled: true`
3. `handleToggleEnabled()` 调用 native `togglePluginEnabled()` 后调用 `loadExtensions()` 刷新
4. 但 `loadExtensions()` 无法获取真实 enabled 状态 → **UI 永远显示 enabled=true**

**修复方向**：`checkInstalledPlugins` 需要返回每个插件的完整状态（含 `enabled`、`versionName`），前端据此更新 UI。

### 问题 2：安装成功后弹窗提示"请重启应用"

**现象**：安装 MPV 插件成功后弹窗显示 `"请重启应用以完成插件加载"`（i18n key: `extensions.installHint`）

**根因**：
- ComboLite demo 中 `installPlugin()` 后不需要重启
- 安装完成后只需调用 `loadEnabledPlugins()` 即可立即加载新插件到内存
- 当前代码缺少 install 后的 load 步骤，所以错误地提示需要重启

**修复方向**：
- 删除/修改 i18n 的 `installHint` 文案（去掉重启提示）
- `executeComboLiteInstall()` 成功后自动调用 `loadEnabledPlugins()`
- 前端安装成功提示改为即时可用

### 问题 3：MPV 播放器白屏（最关键）

**饱和调试输出关键发现**：
```
Phase 1: getAllInstallPlugins() → com.encvgo.plugin.mpv(v0.0.0,enabled=true) ✅ 已安装
Phase 4: getPluginInfo('com.encvgo.plugin.mpv') → ❌ null （未加载！）
```

**根因链**（AAR 反编译证实）：

| API | 返回类型 | 含义 | 诊断结果 |
|-----|---------|------|---------|
| `getAllInstallPlugins()` | `List<PluginInfo>` | 从 XML 读取所有**已安装**插件 | ✅ 找到 mpv |
| `getPluginInfo(id)` | `LoadedPluginInfo?` | 仅返回**已加载到内存**的插件 | ❌ null |

**ComboLite 两阶段模型**：
```
installPlugin() → APK 复制 + 签名校验 + XML 持久化 = "已安装"
loadEnabledPlugins() / launchPlugin() = ClassLoader 创建 + IPluginEntryClass 实例化 = "已加载"
```

当前代码只做了 install，**从未调用 load**。因此：
1. `EncvHostActivity` (BaseHostActivity) 代理启动 Activity 时，ProxyManager 在内存中找不到该插件的 ClassLoader → 白屏
2. `PlayerEntry.isMpvAvailable()` 用 `getPluginInfo()` 检查 → 返回 null → 误判为不可用
3. `startMpvPlayer()` 直接发 Intent 给 EncvHostActivity，但插件未加载 → 代理失败

**修复方向**：
- `executeComboLiteInstall()` 成功后 → 自动 `loadEnabledPlugins()`
- `startMpvPlayer()` 启动前 → 确保 plugin 已 loaded（未 loaded 则先 `launchPlugin()`）
- `isMpvAvailable()` 改用 `getAllInstallPlugins()` 检查（检查 installed 而非 loaded）

---

## 实施步骤

### Step 1: 扩展 `checkInstalledPlugins` 返回完整插件状态

**文件**: [GoProcessPlugin.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt)

**改动**: `checkInstalledPlugins()` 方法 (L436-L455)

当前：
```kotlin
for (plugin in plugins) {
    result.put(plugin.id, true)  // 只存 boolean
}
```

改为：对每个已安装插件，返回一个包含 `enabled`、`versionName` 的子对象：
```kotlin
for (plugin in plugins) {
    val info = JSObject().apply {
        put("installed", true)
        put("enabled", plugin.enabled)
        put("versionName", plugin.versionName)
    }
    result.put(plugin.id, info)
}
```

**AAR 依据**：`PluginInfo` 有 `getId(): String`, `getEnabled(): boolean`, `getVersionName(): String` — 全部 verified。

### Step 2: 安装成功后自动加载插件

**文件**: [GoProcessPlugin.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt)

**改动**: `executeComboLiteInstall()` 方法 (L1156-L1190)

在 InstallResult.Success 分支中，resolve 之前添加 `loadEnabledPlugins()` 调用：
```kotlin
is InstallResult.Success -> {
    // 安装后立即加载插件到内存（无需重启！）
    try {
        val loadedCount = PluginManager.loadEnabledPlugins()
        Log.i(TAG, "ComboLite install + load: $loadedCount plugins loaded")
    } catch (e: Exception) {
        Log.w(TAG, "post-install loadEnabledPlugins failed", e)
    }
    // ... resolve ...
}
```

**AAR 依据**：`public final Object loadEnabledPlugins(Continuation<? super Integer>)` — suspend, 返回 Int (loaded count)

### Step 3: 修复 `PlayerEntry.isMpvAvailable()` 和 `startMpvPlayer()`

**文件**: [PlayerEntry.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerEntry.kt)

**3a. `isMpvAvailable()` (L49-58)**:

当前用 `getPluginInfo()` 检查 → 只能检测 loaded 状态。改为用 `getAllInstallPlugins()` 检查 installed 状态：
```kotlin
fun isMpvAvailable(context: Context): Boolean {
    return try {
        val pm = com.combo.core.runtime.PluginManager
        if (!pm.isInitialized) return false
        pm.getAllInstallPlugins().any { it.id == PLUGIN_ID && it.enabled }
    } catch (e: Exception) {
        Log.w(TAG, "isMpvAvailable check failed", e)
        false
    }
}
```

**3b. `startMpvPlayer()` (L60-86)**:

在启动 Intent 前，确保插件已加载。如果 `getPluginInfo` 返回 null（未加载），先调用 `launchPlugin()`：
```kotlin
private fun startMpvPlayer(...) {
    try {
        val pm = com.combo.core.runtime.PluginManager
        // 确保插件已加载到内存
        if (pm.getPluginInfo(PLUGIN_ID) == null) {
            Log.i(TAG, "MPV plugin not loaded, launching...")
            runBlocking { pm.launchPlugin(PLUGIN_ID) }
        }
        // ... 创建 Intent 启动 EncvHostActivity ...
    }
}
```

**注意**：`launchPlugin()` 是 suspend 函数，在非协程上下文中需用 `runBlocking`。或者改用 GlobalScope.launch + 回调方式。考虑到 `startMpvPlayer` 是同步调用的简单函数，`runBlocking` 是合理选择。

**AAR 依据**：
- `launchPlugin(String): Object` (suspend, returns Boolean)
- `getAllInstallPlugins(): List<PluginInfo>` — 含 `.id`, `.enabled`

### Step 4: 前端适配新的 `checkInstalledPlugins` 返回格式

**文件**: [ExtensionsPage.vue](app/encv-mobile/src/views/ExtensionsPage.vue)

**4a. `loadExtensions()` (L170-193)**:

适配新的返回格式（从 `{id: boolean}` 变为 `{id: {installed, enabled, versionName}}`）：
```typescript
async function loadExtensions() {
  const installedMap = Capacitor.isNativePlatform() ? await checkInstalledPlugins() : {}
  const mpvInfo = installedMap[COMBOLITE_PLUGIN_ID_MAP['mpv-player']]
  extensions.value = [{
    id: 'mpv-player',
    name: t('extensions.mpvPlayer'),
    description: t('extensions.mpvPlayerDesc'),
    installed: !!mpvInfo?.installed,
    enabled: mpvInfo?.enabled ?? false,  // ← 从 backend 获取真实状态
    sizeDisplay: '~35 MB',
    versionName: mpvInfo?.versionName ?? '',
  }]
}
```

**4b. 安装成功提示 (L206-212)**:

去掉"请重启"文案，改为即时可用：
```typescript
if (result.success) {
  showToast({ message: t('extensions.installSuccess'), duration: 2000, color: 'success' })
  await loadExtensions()
}
```
删除原来的 `alertController.create` 弹窗（或改为简洁的成功 toast）。

### Step 5: 更新 i18n 文案

**文件**: [useI18n.ts](app/encv-mobile/src/composables/useI18n.ts)

修改 `extensions.installHint`：
- zh-CN: `'请重启应用以完成插件加载'` → `'插件已就绪，可以立即使用'`
- en: `'Restart the app to complete plugin loading'` → `'Plugin ready for immediate use'`

或者在 Step 4b 中不再引用此 key（直接用 toast 替代弹窗）。

### Step 6: 更新 TS 接口类型

**文件**: [web.ts](app/encv-mobile/src/plugins/web.ts)

更新 `checkInstalledPlugins` 返回类型：
```typescript
// Before:
checkInstalledPlugins(): Promise<Record<string, boolean>>
// After:
checkInstalledPlugins(): Promise<Record<string, { installed: boolean; enabled: boolean; versionName: string }>>
```

---

## 文件变更清单

| 文件 | 改动内容 |
|------|---------|
| `app/.../GoProcessPlugin.kt` | 1. `checkInstalledPlugins` 返回完整状态 2. `executeComboLiteInstall` 成功后 auto-load |
| `app/.../PlayerEntry.kt` | 1. `isMpvAvailable` 用 getAllInstallPlugins 2. `startMpvPlayer` 启动前 ensure loaded |
| `app/.../ExtensionsPage.vue` | 1. `loadExtensions` 读取真实 enabled 状态 2. 安装成功去重启提示 |
| `app/.../useI18n.ts` | 修改 `installHint` 文案 |
| `app/.../web.ts` | 更新 `checkInstalledPlugins` 返回类型 |

## 验证方法

1. 安装 MPV 插件 → 应无"重启"提示 → 立即可用
2. 禁用 MPV → UI 显示"已禁用" → 再次启用 → UI 显示"已启用"
3. 打开视频选 MPV 模式 → 正常播放（非白屏）
4. 🔧 生命周期诊断 → Phase 4 `getPluginInfo` 应返回 non-null
