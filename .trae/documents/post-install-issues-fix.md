# 扩展安装后三大问题修复计划

## 问题总览

| # | 问题 | 根因 | 严重度 |
|---|------|------|--------|
| 1 | 扩展管理页面安装后不更新 | 插件 ID 键名不匹配：前端查 `mpv-player`，ComboLite 返回 `com.encvgo.plugin.mpv` | 高 |
| 2 | 设置用 MPV 打开后显示空白 WebView + ERR_HTTP_RESPONSE_CODE_FAILURE | 前端 localStorage 与 Android SharedPreferences 未同步，PlayerEntry 永远走 artplayer 路径 | 严重 |
| 3 | Android data 找不到 encv 目录 | 需调查：可能是 scoped storage 限制或 app 重装后数据丢失 | 中 |

---

## 问题 1：扩展管理页面不更新

### 根因分析

**数据流**：
```
ExtensionsPage.vue → checkInstalledPlugins() → GoProcessPlugin.checkInstalledPlugins()
  → PluginManager.getAllInstallPlugins() → 返回 [PluginInfo(id="com.encvgo.plugin.mpv", ...)]
  → JSObject: {"com.encvgo.plugin.mpv": true}
```

**前端检查**（[ExtensionsPage.vue:165](file:///workspace/app/encv-mobile/src/views/ExtensionsPage.vue#L165)）：
```typescript
installed: !!installedMap['mpv-player'],  // ← 键名不匹配！
```

ComboLite 使用插件的 Android 包名（`namespace`）作为插件 ID，即 `com.encvgo.plugin.mpv`。但前端硬编码了 `mpv-player` 作为扩展 ID。

**附带问题**：[PlayerEntry.kt:11](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerEntry.kt#L11) 中 `PLUGIN_ID = "mpv-player"` 也是错误的，导致 `isMpvAvailable()` 永远返回 false。

### 修复步骤

1. **建立插件 ID 映射**（前端）：
   - 在 `ExtensionsPage.vue` 中创建映射：`{ 'mpv-player': 'com.encvgo.plugin.mpv' }`
   - 检查安装状态时使用映射后的 ID：`installedMap[pluginIdMap['mpv-player']]`

2. **修复 PlayerEntry.PLUGIN_ID**（Kotlin）：
   - 将 `PLUGIN_ID` 从 `"mpv-player"` 改为 `"com.encvgo.plugin.mpv"`
   - 这样 `isMpvAvailable()` 才能正确检测插件是否已安装

3. **统一扩展 ID 体系**：
   - 前端使用人类可读的 `mpv-player` 作为扩展 ID（用于 UI 展示）
   - 与 ComboLite 插件 ID `com.encvgo.plugin.mpv` 建立映射
   - 映射关系集中定义，避免散落各处

---

## 问题 2：MPV 打开为空白 WebView（核心问题）

### 根因分析

**完整调用链**：
```
用户在 Settings 选择 "MPV 插件扩展"
  → localStorage.setItem('encv_player_video', 'mpv-plugin')  ← 只存到 Web 存储

用户播放视频
  → Files.vue: getPlayMode() → localStorage.getItem('encv_player_video') → 'mpv-plugin'
  → Files.vue: openPlayer(path, name, mimeType)  ← 前端已决定用 MPV
  → GoProcessPlugin.openPlayer() → PlayerEntry.play(context, filePath, name, mimeType)
  → PlayerEntry.play(): SharedPreferences("encv_player_prefs").getString("video_player", "artplayer")
     ↑↑↑ 从未写入！默认值 "artplayer" ↑↑↑
  → mode = "artplayer" → startArtPlayer() → PlayerActivityCapacitor
  → PlayerActivityCapacitor.navigateToPlayer() → webView.loadUrl("https://localhost/player.html")
  → Capacitor localServer 不提供 player.html → ERR_HTTP_RESPONSE_CODE_FAILURE
```

**关键矛盾**：前端通过 `localStorage` 决定播放模式，但 `PlayerEntry.play()` 通过 `SharedPreferences` 重新决定模式——而 SharedPreferences 从未被写入，永远返回默认值 `"artplayer"`。

**`https://localhost/player.html` 错误原因**：Capacitor 的本地服务器只提供 Capacitor 应用自身的资源（`/` 路由对应 `index.html`），不提供 `player.html`。`player.html` 是 Go 后端提供的页面（`http://127.0.0.1:8899/player.html`），但 Capacitor WebView 的 `https://localhost` scheme 无法访问 Go 后端。

### 修复方案

**方案 A（推荐）：前端传递 mode 参数，PlayerEntry 不再自作主张**

1. **GoProcess.ts**：`openPlayer()` 增加 `mode` 参数
   ```typescript
   export async function openPlayer(filePath: string, name: string, mimeType: string, mode?: string): Promise<void>
   ```

2. **GoProcessPlugin.kt**：`openPlayer()` 读取 `mode` 参数并传给 `PlayerEntry.play()`
   ```kotlin
   @PluginMethod
   fun openPlayer(call: PluginCall) {
       val mode = call.getString("mode", "")  // 新增
       PlayerEntry.play(context ?: activity!!, filePath!!, name!!, mimeType!!, isExternal = false, mode = mode)
   }
   ```

3. **PlayerEntry.kt**：`play()` 增加 `mode` 参数，当 mode 非空时优先使用
   ```kotlin
   fun play(context: Context, filePath: String, fileName: String, mimeType: String = "",
            isExternal: Boolean = false, mode: String = "") {
       val effectiveMode = if (mode.isNotEmpty()) mode else {
           val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
           val rawMode = prefs.getString(PREF_KEY_VIDEO_PLAYER, "artplayer") ?: "artplayer"
           if (rawMode == "mpv") "mpv-plugin" else rawMode
       }
       when (effectiveMode) { ... }
   }
   ```

4. **Files.vue**：`playMedia()` 传递 mode 参数
   ```typescript
   case PLAY_MODE.MPV_PLUGIN:
     if (isNative()) {
       openPlayer(file.path, file.name, mimeType, PLAY_MODE.MPV_PLUGIN)  // 传递 mode
     }
   ```

5. **同样修复 `openInPlayer()`**：`PlayerActivity` → `PlayerEntry.play()` 也需要传递 mode

**方案 B（备选）：设置变更时同步 SharedPreferences**

- Settings.vue 变更时调用新的 Capacitor 方法 `setVideoPlayerMode(mode)`
- 该方法写入 `SharedPreferences("encv_player_prefs")` 的 `"video_player"` 键
- 缺点：需要新增 Capacitor 方法，且两个存储源需要保持同步

**选择方案 A 的理由**：
- 前端已经做了 mode 判断，不需要后端重复判断
- 减少存储同步的复杂度
- 更符合"调用者决定行为"的设计原则

### 额外修复：startMpvPlayer 的 fallback 行为

当前 `startMpvPlayer()` 在 catch 中 fallback 到 `startArtPlayer()`，这会掩盖真正的错误。应改为：
- catch 中记录错误日志
- 向用户显示 Toast 提示"MPV 插件启动失败"
- 不再静默 fallback 到 ArtPlayer（用户明确选择了 MPV，不应偷偷换成 ArtPlayer）

---

## 问题 3：Android data 找不到 encv 目录

### 初步分析

**应用数据存储位置**：
- Go 后端 HOME = `context.filesDir` = `/data/data/com.encvgo.app/files/`（内部存储）
- 配置文件 = `filesDir/config.user.json`
- 输出目录 = `/storage/emulated/0/encv-output`（外部存储公共目录）

**可能原因**：
1. **Android 11+ scoped storage**：`/storage/emulated/0/Android/data/com.encvgo.app/` 目录在文件管理器中不可见
2. **应用重装后数据丢失**：内部存储数据在卸载时被清除
3. **输出目录未创建**：`/storage/emulated/0/encv-output` 可能尚未被创建
4. **用户查找位置不对**：用户可能在找 `/storage/emulated/0/Android/data/com.encvgo.app/` 而不是 `/storage/emulated/0/encv-output/`

### 修复步骤

1. **在 debugInstallFlow() 中添加数据目录诊断**：
   - 输出 `filesDir` 路径
   - 输出 `getExternalFilesDir(null)` 路径
   - 检查 `/storage/emulated/0/encv-output` 是否存在
   - 列出 `filesDir` 下的文件

2. **在设置页面添加"数据目录信息"**：
   - 显示内部存储路径（filesDir）
   - 显示输出目录路径
   - 提供"打开输出目录"按钮（使用系统文件管理器 intent）

3. **确保输出目录存在**：
   - 在 `EncvGoService.startGoProcess()` 中检查并创建 `/storage/emulated/0/encv-output`

---

## 实施步骤

### Step 1：修复插件 ID 映射（问题 1）

| 文件 | 修改内容 |
|------|----------|
| `PlayerEntry.kt` | `PLUGIN_ID` 从 `"mpv-player"` 改为 `"com.encvgo.plugin.mpv"` |
| `ExtensionsPage.vue` | 添加插件 ID 映射，用 `com.encvgo.plugin.mpv` 检查安装状态 |

### Step 2：修复播放模式传递（问题 2）

| 文件 | 修改内容 |
|------|----------|
| `GoProcess.ts` | `openPlayer()` 增加 `mode` 参数 |
| `GoProcessPlugin.kt` | `openPlayer()` 读取 `mode` 参数传给 `PlayerEntry.play()` |
| `PlayerEntry.kt` | `play()` 增加 `mode` 参数，优先使用传入的 mode；移除静默 fallback |
| `Files.vue` | `playMedia()` 传递 mode 参数 |
| `PlayerActivity.kt` | 从 Intent extra 读取 mode 并传给 `PlayerEntry.play()` |
| `GoProcessPlugin.kt` | `openInPlayer()` 传递 mode 参数到 Intent extra |

### Step 3：添加数据目录诊断（问题 3）

| 文件 | 修改内容 |
|------|----------|
| `GoProcessPlugin.kt` | `debugInstallFlow()` 添加数据目录信息 |
| `EncvGoService.kt` | 启动时检查输出目录是否存在 |

### Step 4：验证

- 确认 `vue-tsc --noEmit && vite build` 通过
- 确认 Kotlin 代码无编译错误（CI 构建）
