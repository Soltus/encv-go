# MPV 播放器 ComboLite 插件化 — 收尾实施计划（v2）

## 概述

基于已完成的 Phase 1-4 基础设施，本计划覆盖 4 项收尾工作：
1. CI 工作流适配（MPV 插件构建集成到 Android CI，含 debug/release 双变体）
2. 主应用首页增加**播放器扩展**入口 + 二级页面（明确区分两种"插件"概念）
3. 设置页播放方式适配（MPV 插件选项）
4. 剩余 TODO 实现（MpvEngine 接入、流地址解析、全屏、安全区域）

---

## Task 1: CI 工作流适配

### 现状分析

当前 [android.yml](file:///workspace/.github/workflows/android.yml) 关键步骤：
- L116-117: `setup-mpv-libs.sh` — 下载 mpv .so → 已改为输出到 `plugin-mpv-player/src/main/jniLibs/`
- L119-120: `build-ffmpeg-android.sh` — 编译 Go 后端 FFmpeg
- L182-187: 复制 libencv-go.so 到 Host jniLibs
- L189-218: Verify native libraries — 检查 libmpv/libplayer/FFmpeg 系列
- L246: `./gradlew assembleDebug`
- L272: APK 验证 — 检查 APK 中包含 mpv/ffmpeg .so

### 插件 APK 构建的难点分析

#### 难点 A: aar2apk 需要 root build.gradle.kts 配置（项目用 Groovy DSL）

ComboLite 的 `aar2apk` 插件要求在 **root `build.gradle.kts`** 中声明模块：

```kotlin
// build.gradle.kts (root) — 但本项目用的是 build.gradle (Groovy)!
aar2apk {
    modules { module(":plugin-mpv-player") }
    signing { ... }
}
```

**问题**: 当前 root 是 [build.gradle (Groovy)](file:///workspace/app/encv-mobile/android/build.gradle)，不是 kts。

**解决方案**: 在 Groovy root build.gradle 中用同样的语法配置。Gradle 支持在 `.gradle` 文件中使用 Kotlin DSL 风格的 block（因为 aar2apk 插件注册的是 Gradle Extension，与 DSL 语言无关）：

```groovy
// android/build.gradle (root, Groovy)
buildscript { ... }
apply plugin: 'io.github.lnzz123.combolite-aar2apk'  // 或在 plugins block 中

aar2apk {
    modules {
        module(':plugin-mpv-player')
    }
}
```

> 如果 aar2apk 插件严格要求 kts，则需将 root build.gradle 迁移为 kts，或将 aar2apk 配置提取为独立的 `aar2apk.settings.gradle.kts` 文件。

#### 难点 B: Debug vs Release 双变体签名

Host App 有两个构建变体：
| 变体 | minify | proguard | 签名 |
|------|--------|----------|------|
| **debug** | false | 无 | debug keystore |
| **release** | true | 有 | release keystore (`encv-release.gradle`) |

ComboLite 插件 APK 也需要对应签名：
- **Debug 插件 APK**: 使用 debug keystore（自动），用于开发调试
- **Release 插件 APK**: 必须使用 **与宿主相同的签名**（否则无法通过签名校验安装）

**CI 环境中签名处理**:
```yaml
# CI 中从 secrets 获取 keystore
- name: Setup release keystore for plugin APK
  run: |
    echo "${{ secrets.ANDROID_KEYSTORE_BASE64 }}" | base64 -d > app/encv-mobile/android/release.keystore
    # 配置 aar2apk signing（写入 local.properties 或 gradle.properties）
    echo "COMBOLITE_KEYSTORE_PATH=release.keystore" >> app/encv-mobile/android/local.properties
    echo "COMBOLITE_KEYSTORE_PASSWORD=${{ secrets.KEYSTORE_PASSWORD }}" >> app/encv-mobile/android/local.properties
```

#### 难点 C: 插件 APK 与 Host APK 的构建顺序

正确顺序：
1. 先编译插件 Library module（`:plugin-mpv-player:compileDebugKotlin` + ndk-build 编译 libplayer.so）
2. aar2apk 将编译产物打包为插件 APK
3. （可选）将插件 APK 复制到 Host assets 目录（`packagePlugins` 自动集成模式）
4. 编译 Host APK（assembleDebug / assembleRelease）

CI 步骤设计需反映此顺序。

### 具体修改方案

#### 1.1 修改 root build.gradle — 添加 aar2apk 配置

**文件**: `/workspace/app/encv-mobile/android/build.gradle`

在文件末尾添加：

```groovy
// ComboLite Plugin Configuration
buildscript {
    dependencies {
        classpath 'io.github.lnzz123.combolite-aar2apk:io.github.lnzz123.combolite-aar2apk.gradle.plugin:1.1.0'
    }
}

apply plugin: 'io.github.lnzz123.combolite-aar2apk'

aar2apk {
    modules {
        module(':plugin-mpv-player')
    }
}
```

> 注意：classpath 可能在之前的 Phase 1 中已添加过，检查是否重复。

#### 1.2 修改 app/build.gradle — 添加 packagePlugins 配置

**文件**: `/workspace/app/encv-mobile/android/app/build.gradle`

在 `android {}` block 后、`dependencies {}` 前：

```groovy
// ComboLite: auto-integrate plugin APKs into host assets during development
packagePlugins {
    enabled.set(true)
    buildType.set(io.github.combolite.core.build.PackageBuildType.DEBUG)
    pluginsDir.set("plugins")
}
```

Release 构建时不启用 packagePlugins（release 应该预置或远程下载插件）。

#### 1.3 CI 工作流：新增插件构建步骤

**文件**: `.github/workflows/android.yml`

在现有 `Build native libraries` 和 `Build Android App` 之间插入：

```yaml
# === Build MPV Player Plugin ===
- name: Build MPV player plugin (JNI + Kotlin compile)
  run: |
    cd app/encv-mobile/android
    
    # Step 1: ndk-build libplayer.so (if not already built by setup-mpv-libs step)
    if [ ! -f "plugin-mpv-player/src/main/jniLibs/arm64-v8a/libplayer.so" ]; then
      echo "Building libplayer.so via ndk-build..."
      bash ../scripts/build-player-so.sh || echo "⚠️ libplayer.so build failed"
    fi
    
    # Step 2: Compile plugin Kotlin code
    ./gradlew :plugin-mpv-player:compileDebugKotlin --stacktrace 2>&1
    echo "=== Plugin module compile result: $? ==="

- name: Package MPV plugin as APK (debug)
  run: |
    cd app/encv-mobile/android
    # aar2apk task: converts library AAR to installable APK
    ./gradlew :plugin-mpv-player:buildDebugPluginApk --stacktrace 2>&1 || echo "⚠️ Plugin APK packaging skipped"
    
    # Verify output
    if [ -f "build/outputs/plugin-apks/debug/plugin-mpv-player-debug.apk" ]; then
      echo "✅ Plugin APK generated:"
      ls -lh build/outputs/plugin-apks/debug/
      
      # Copy to host assets for auto-integration
      mkdir -p app/src/main/assets/plugins
      cp build/outputs/plugin-apks/debug/plugin-mpv-player-debug.apk \
         app/src/main/assets/plugins/mpv-player.apk
      echo "✅ Plugin APK copied to host assets/plugins/"
    else
      echo "⚠️ Plugin APK not found at expected path"
      # List what was actually produced
      find build/outputs -name "*.apk" 2>/dev/null || echo "No APK outputs found"
    fi
  continue-on-error: true

- name: Verify plugin APK contents
  if: always()
  run: |
    PLUGIN_APK="app/encv-mobile/android/build/outputs/plugin-apks/debug/plugin-mpv-player-debug.apk"
    if [ -f "$PLUGIN_APK" ]; then
      echo "=== Plugin APK size ==="
      ls -lh "$PLUGIN_APK"
      echo "=== Plugin APK contains .so files ==="
      unzip -l "$PLUGIN_APK" | grep "\.so" | head -20
      echo "=== Total .so count in plugin ==="
      unzip -l "$PLUGIN_APK" | grep -c "\.so" || echo "0"
    else
      echo "⏭️ Plugin APK not available, skipping verification"
    fi
```

#### 1.4 修改 Verify native libraries 步骤

**文件**: `.github/workflows/android.yml` (L189-218)

改造为区分 Host vs Plugin .so：

```yaml
- name: Verify native libraries
  run: |
    echo "═══ HOST APP jniLibs (Go backend only) ═══"
    HOST_LIBS="app/encv-mobile/android/app/src/main/jniLibs/arm64-v8a"
    for lib in libencv-go.so libffmpeg.so libffprobe.so; do
      if [ -f "$HOST_LIBS/$lib" ]; then
        echo "  ✅ $lib $(ls -lh "$HOST_LIBS/$lib" | awk '{print $5}')"
      else
        echo "  ⚠️ $lib absent"
      fi
    done

    echo ""
    echo "═══ MPV PLAYER PLUGIN jniLibs ═══"
    PLUGIN_LIBS="app/encv-mobile/plugin-mpv-player/src/main/jniLibs/arm64-v8a"
    if [ -d "$PLUGIN_LIBS" ]; then
      echo "  Plugin .so files:"
      ls -lh "$PLUGIN_LIBS/"*.so 2>/dev/null | awk '{print "  ✅ "$NF" ("$5")"}'
      REQUIRED="libmpv.so libplayer.so libavcodec.so libavformat.so libavutil.so libswresample.so libswscale.so"
      for lib in $REQUIRED; do
        [ -f "$PLUGIN_LIBS/$lib" ] && echo "  ✅ $lib present" || echo "  ❌ $lib MISSING!"
      done
    else
      echo "  ⚠️ Plugin jniLibs dir not found"
    fi

    echo ""
    echo "═══ EXPECTED SEPARATION ═══"
    echo "  Host APK should contain: libencv-go.so (+ optional libffmpeg/libffprobe)"
    echo "  Host APK should NOT contain: libmpv.so, libplayer.so, libav*.so (MPV's)"
    echo "  Plugin APK should contain: all MPV/FFmpeg .so files"
```

#### 1.5 修改 APK 验证步骤

**文件**: `.github/workflows/android.yml` (L249-277)

```yaml
- name: Verify final APK
  run: |
    echo "═══ HOST APK analysis ═══"
    unzip -l "$APK_PATH" | grep -E "\.so$" | head -20
    echo "---"
    if unzip -l "$APK_PATH" | grep -q "libmpv\.so\|libplayer\.so"; then
      echo "❌ FAIL: Host APK still contains MPV .so (should be in plugin only)"
      exit 1
    else
      echo "✅ PASS: Host APK excludes MPV .so"
    fi
    if unzip -l "$APK_PATH" | grep -q "libencv-go"; then
      echo "✅ PASS: Host APK contains Go backend binary"
    else
      echo "❌ FAIL: libencv-go.so missing from Host APK"
    fi
```

---

## Task 2: 主应用首页「播放器扩展」+ 二级页面

### ⚠️ 重要：术语规范 — 消除"插件"歧义

项目中存在 **两套完全不同的"插件"体系**：

| 体系 | 技术层 | 用户可见性 | 示例 | 存储位置 |
|------|--------|-----------|------|---------|
| **ENCV 容器格式插件** | Go 后端 | 开发者可见 | video, audio, image, pdf | Go 代码 + config.user.json `plugin_settings` |
| **ComboLite 功能扩展** | Android APK 层 | **用户可见** | MPV 播放器 | Android PluginManager |

**命名规范**：
- 对用户 UI：ComboLite 统称为 **「功能扩展」** 或 **「扩展」**，不叫"插件"
- ENCV 插件保持叫 **「插件」**（仅开发者/设置页高级区域出现）
- MPV 播放器在 UI 中称为 **「MPV 播放器扩展」**

这样彻底消除歧义：
- 「设置 → 插件设置」→ ENCV 容器格式插件（已有）
- 「首页 → 扩展管理」→ ComboLite 功能扩展（新增）

### 方案

#### 2.1 修改 HomePage.vue — 新增「扩展管理」卡片

**文件**: `/workspace/app/encv-mobile/src/views/HomePage.vue`

在现有 4 个卡片后新增第 5 个卡片（注意 grid 布局调整）：

```html
<!-- 扩展管理 -->
<div class="home-card extensions-card" @click="handleOpenExtensions">
  <ion-icon :icon="layersOutline" class="card-icon extensions-icon"></ion-icon>
  <div class="card-info">
    <h3>{{ t('home.extensions') }}</h3>
    <p>{{ t('home.extensionsDesc') }}</p>
  </div>
</div>
```

grid 布局从 2×2 调整为：Player 卡片保持横跨两列，其余 4 卡片（Files/Tasks/Remote/Extensions）排成 2×2。

#### 2.2 新建 ExtensionsPage.vue — 功能扩展管理页（本地安装）

**文件**: `/workspace/app/encv-mobile/src/views/ExtensionsPage.vue`（新建）

这是全新的二级页面，**不是改造 PluginSettings.vue**（PluginSettings 保持不变，继续服务 ENCV 插件）。

**核心交互：系统文件选择器 + 本地安装**

```
┌─────────────────────────────┐
│ ← 扩展管理                  │
├─────────────────────────────┤
│                             │
│ ┌─────────────────────────┐ │
│ │ 🎬 MPV 播放器            │ │
│ │                         │ │
│ │ 高性能原生视频/音频播放  │ │
│ │ 支持 MKV/FLV/ASS 字幕   │ │
│ │                         │ │
│ │ 📦 约 35 MB             │ │
│ │                         │ │
│ │ [未安装]                 │ │ ← 状态显示
│ └─────────────────────────┘ │
│                             │
│     ┌───────────────────┐   │
│     │ 📂 从文件安装扩展  │   │ ← 主操作按钮
│     └───────────────────┘   │
│                             │
│ 💡 选择 .apk 文件即可安装   │
│    可通过 USB/网盘传入手机   │
└─────────────────────────────┘

安装中状态：
┌─────────────────────────────┐
│ ← 扩展管理                  │
├─────────────────────────────┤
│  📦 正在安装 MPV 播放器...  │
│  ████████░░░░░  65%        │
│  正在校验签名...            │
└─────────────────────────────┘

已安装状态：
┌─────────────────────────────┐
│ ← 扩展管理                  │
├─────────────────────────────┤
│ ┌─────────────────────────┐ │
│ │ 🎬 MPV 播放器  ✅ 已安装│ │
│ │                         │ │
│ │ v1.0 · 约 35 MB         │ │
│ │                         │ │
│ │ [禁用]  [卸载]          │ │
│ └─────────────────────────┘ │
└─────────────────────────────┘
```

**前端核心逻辑**：

```typescript
import { layersOutline, filmOutline, addOutline } from 'ionicons/icons'
import { Capacitor } from '@capacitor/core'
import { PlayerBridgeModule } from '@/plugins/GoProcess'

interface ExtensionInfo {
  id: string
  name: string
  icon: string
  description: string
  installed: boolean
  enabled: boolean
  sizeDisplay: string
  version?: string
}

const extensions = ref<ExtensionInfo[]>([])
const installing = ref(false)
const installProgress = ref(0)
const installMessage = ref('')
const installError = ref('')

async function loadExtensions() {
  if (!Capacitor.isNativePlatform()) return

  const result = await PlayerBridgeModule.getExtensionStatus()
  // 返回 { mpvPlayer: { installed, enabled, version } }
  extensions.value = [
    {
      id: 'mpv-player',
      name: t('extensions.mpvPlayer'),
      icon: 'film-outline',
      description: t('extensions.mpvPlayerDesc'),
      installed: result?.mpvPlayer?.installed ?? false,
      enabled: result?.mpvPlayer?.enabled ?? false,
      sizeDisplay: '~35 MB',
      version: result?.mpvPlayer?.version,
    },
  ]
}

async function handleInstallFromFile() {
  try {
    // 调用原生方法打开系统文件选择器
    const result = await PlayerBridgeModule.pickAndInstallPlugin({
      mimeType: 'application/vnd.android.package-archive',
      title: t('extensions.selectApk')
    })
    
    if (result.success) {
      await loadExtensions() // 刷新状态
    } else {
      installError.value = result.error || t('extensions.installFailed')
    }
  } catch (e: any) {
    installError.value = e.message || t('extensions.installFailed')
  }
}

async function handleUninstall(id: string) {
  const result = await PlayerBridgeModule.uninstallPlugin({ pluginId: id })
  if (result.success) {
    await loadExtensions()
  }
}
```

**PlayerBridgeModule 新增方法**（Kotlin 侧）：

```kotlin
// PlayerBridgeModule.kt — 新增方法

@LynxMethod
fun getExtensionStatus(): Map<String, Any> {
    val pm = try { PluginManager.getInstance(context) } catch (e: Exception) { null }
    val mpvPlugin = pm?.getInstalledPlugin("mpv-player")
    return mapOf(
        "mpvPlayer" to mapOf(
            "installed" to (mpvPlugin != null),
            "enabled" to (mpvPlugin?.enabled == true),
            "version" to (mpvPlugin?.versionName ?: "")
        )
    )
}

@LynxMethod
fun pickAndInstallPlugin(options: Map<String, String>): Map<String, Any> {
    // 1. 启动系统文件选择器 (Intent.ACTION_GET_CONTENT)
    // 2. 用户选择 .apk 文件后获取 URI
    // 3. 复制到应用内部存储
    // 4. 调用 InstallerManager.installPlugin(file)
    // 5. 返回结果
}
```

**Kotlin 侧文件选择器实现**：

```kotlin
@LynxMethod
fun pickAndInstallPlugin(options: Map<String, String>): Map<String, Any> {
    return try {
        val intent = Intent(Intent.ACTION_GET_CONTENT).apply {
            type = "application/vnd.android.package-archive"
            addCategory(Intent.CATEGORY_OPENABLE)
            putExtra(Intent.EXTRA_TITLE, options["title"] ?: "Select plugin APK")
        }
        
        // 通过 Activity Result API 获取文件
        // 注意：LynxModule 中需要用 startActivityForResult 或
        // 使用 registerForActivityResult (需要 Activity 引用)
        // 实际实现可能需要在 Activity 层处理回调
        
        // 简化方案：先检查是否有预置的 APK 在 assets/downloads/
        // 或者使用 ComboLite 的 installPluginsFromAssetsForDebug 作为备选
        
        mapOf("success" to true)
    } catch (e: Exception) {
        mapOf("success" to false, "error" to (e.message ?: "Unknown error"))
    }
}
```

> **注意**: 从 LynxModule 中启动 Activity 并获取结果需要特殊处理。实际有两种方案：
> 
> **方案 A（推荐）**: 在 MpvPlayerActivity / 主 Activity 中注册一个 `ActivityResultCaller`，通过全局事件/LynxEvent 触发文件选择。PlayerBridgeModule 发送事件 → Activity 收到 → 启动选择器 → 回调结果 → PlayerBridgeModule 调用 `InstallerManager.installPlugin()`。
> 
> **方案 B（简化）**: 先支持从 **应用内部 assets/downloads/** 目录安装（用户将 .apk 文件放入该目录），后续迭代再接入系统文件选择器。这避免了 `startActivityForResult` 的复杂性。

**i18n 键值更新**：

中文：
```
'home.extensions': '扩展管理',
'home.extensionsDesc': '管理和配置播放器等功能扩展',
'extensions.title': '扩展管理',
'extensions.mpvPlayer': 'MPV 播放器',
'extensions.mpvPlayerDesc': '高性能原生视频/音频播放器，支持 MKV/FLV/ASS 字幕等格式',
'extensions.installed': '已安装',
'extensions.notInstalled': '未安装',
'extensions.enable': '启用',
'extensions.disable': '禁用',
'extensions.uninstall': '卸载',
'extensions.sizeHint': '约 {size}',
'extensions.hint': '扩展按需安装可减小应用体积',
'extensions.comingSoon': '敬请期待',
'extensions.selectApk': '选择扩展安装包 (.apk)',
'extensions.installing': '正在安装...',
'extensions.installSuccess': '安装成功',
'extensions.installFailed': '安装失败',
'extensions.uninstallConfirm': '确定要卸载此扩展吗？',
'extensions.uninstallSuccess': '卸载完成',
'extensions.installedVersion': '版本 {v}',
'extensions.noExtensions': '暂无可用扩展',
'extensions.installFromLocal': '从文件安装',
'extensions.installFromLocalHint': '选择 .apk 文件进行本地安装',
```

#### 2.3 注册路由

**文件**: `/workspace/app/encv-mobile/src/router/index.ts`

```typescript
{
  path: '/tabs/extensions',
  component: () => import('@/views/ExtensionsPage.vue'),
  meta: { title: 'extensions.title' }
}
```

#### 2.4 注册 PlayerBridgeModule 到 LynxViewBuilder

找到 LynxViewBuilder 的 `registerModule` 调用位置（搜索 Kotlin 代码），添加：
```kotlin
viewBuilder.registerModule(PlayerBridgeModule::class.java)
```

#### 2.5 更新 typing.d.ts

```typescript
declare let NativeModules: {
  // ...existing modules...
  PlayerBridgeModule: {
    playFile: (filePath: string, fileName: string, mimeType: string) => Promise<boolean>
    playFileExternal: (filePath: string, fileName: string, mimeType: string) => Promise<boolean>
    isMpvAvailable: () => Promise<boolean>
    getExtensionStatus: () => Promise<{ mpvPlayer: { installed: boolean, enabled: boolean, version: string } }>
    pickAndInstallPlugin: (options: { mimeType?: string, title?: string }) => Promise<{ success: boolean, error?: string }>
    uninstallPlugin: (options: { pluginId: string }) => Promise<{ success: boolean, error?: string }>
  }
}
```

#### 2.6 i18n 键值

中文：
```
'home.extensions': '扩展管理',
'home.extensionsDesc': '管理和配置播放器等功能扩展',
'extensions.title': '扩展管理',
'extensions.mpvPlayer': 'MPV 播放器',
'extensions.mpvPlayerDesc': '高性能原生视频/音频播放器，支持 MKV/FLV/ASS 字幕等格式',
'extensions.installed': '已安装',
'extensions.notInstalled': '未安装',
'extensions.enable': '启用',
'extensions.disable': '禁用',
'extensions.uninstall': '卸载',
'extensions.sizeHint': '约 {size}',
'extensions.hint': '扩展按需安装可减小应用体积',
'extensions.comingSoon': '敬请期待',
```

英文对应翻译。

---

## Task 3: 设置页播放方式适配

### 现状分析

[Settings.vue](file:///workspace/app/encv-mobile/src/views/Settings.vue) L33-81：

**视频播放选项** (localStorage key `encv_player_video`):
- `artplayer` → 内置 Artplayer（默认）
- `mpv` → 内置 MPV（旧路径）
- `external` → 外部打开

**音频播放选项** (localStorage key `encv_player_audio`):
- `mpv` → 内置 MPV（默认）
- `external` → 外部打开

### 方案

#### 3.1 修改 Settings.vue — 视频选项更新

将 `value="mpv"` 改为 `value="mpv-plugin"`，文案改为「MPV 播放器（扩展）」：

```html
<ion-select-option value="artplayer">{{ t('settings.builtInArtplayer') }}</ion-select-option>
<ion-select-option value="mpv-plugin">{{ t('settings.mpvPluginExtension') }}</ion-select-option>
<ion-select-option value="external">{{ t('settings.openExternal') }}</ion-select-option>
```

新增 i18n 键：
```
'settings.mpvPluginExtension': 'MPV 播放器（需安装扩展）',
```

**向后兼容**：如果旧 localStorage 值是 `"mpv"`，读取时映射为 `"mpv-plugin"`。

#### 3.2 PlayerEntry 适配 — 读取用户选择并路由

**文件**: `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerEntry.kt`

```kotlin
fun play(context: Context, filePath: String, fileName: String, 
         mimeType: String, isExternal: Boolean = false) {
    val prefs = context.getSharedPreferences("encv_player_prefs", Context.MODE_PRIVATE)
    // 向后兼容：旧值 "mpv" 映射为新值 "mpv-plugin"
    val rawMode = prefs.getString("video_player", "artplayer") ?: "artplayer"
    val mode = if (rawMode == "mpv") "mpv-plugin" else rawMode

    when (mode) {
        "mpv-plugin" -> {
            val pm = try { PluginManager.getInstance(context) } catch (e: Exception) { null }
            val mpvPlugin = pm?.getInstalledPlugin("mpv-player")
            if (mpvPlugin != null && mpvPlugin.enabled) {
                startMpvPlayer(context, filePath, fileName, mimeType, isExternal, pm, mpvPlugin)
            } else {
                // Toast 提示 + 降级到 ArtPlayer
                android.widget.Toast.makeText(
                    context, 
                    context.getString(R.string.mpv_plugin_not_available),
                    android.widget.Toast.LENGTH_SHORT
                ).show()
                startArtPlayer(context, filePath, fileName)
            }
        }
        "external" -> openExternal(context, filePath)
        else -> startArtPlayer(context, filePath, fileName) // default: artplayer
    }
}
```

---

## Task 4: 完成剩余 TODO 工作

### 4.1 createMpvEngine() — MpvEngine 构造 + SurfaceView 接入

**文件**: [MpvPlayerActivity.kt:54-56](file:///workspace/app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerActivity.kt#L54-L56)

实现 MpvEngine 创建 + 事件监听 + SurfaceView 桥接。

### 4.2 resolveStreamUrl() — Go 后端流地址 HTTP 解析

**文件**: [MpvPlayerScreen.kt:244-246](file:///workspace/app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerScreen.kt#L244-L246)

通过 Intent 接收 `backend_url` 参数（由 PlayerEntry 传入），HTTP HEAD 请求验证可达性后返回 URL 给 MPV loadfile。

配套：PlayerEntry.startMpvPlayer() Bundle 新增 `backend_url` 字段。

### 4.3 全屏切换 — Activity 方向 + WindowInsetsController

**文件**: [MpvPlayerScreen.kt:181-183](file:///workspace/app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerScreen.kt#L181-L183)

Activity requestedOrientation 切换 + hideSystemUi/showSystemUi 辅助函数。

### 4.4 WindowInsetsSafeTop/Bottom — Compose systemBars

**文件**: [MpvPlayerScreen.kt:249-252](file:///workspace/app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerScreen.kt#L249-L252)

删除硬编码 0 函数，改用 `Modifier.windowInsetsPadding(WindowInsets.systemBars)`。

---

## 任务依赖关系

```
Task 1 (CI + aar2apk 配置) ─────────┐
                                    │ (并行)
Task 2 (首页扩展卡片 + ExtensionsPage) ├──┐
                                    │   │
Task 3 (设置页 mpv-plugin 选项) ────┤   │
                                    │   │
Task 4 (4个 TODO 实现) ─────────────┘   │
  ├── 4.1 createMpvEngine()            │
  ├── 4.2 resolveStreamUrl() ← 依赖 Task 3 的 backend_url 传递
  ├── 4.3 全屏切换                    │
  └── 4.4 WindowInsets                │
                                        │
Task 1-3 并行 → Task 4 最后收尾         │
```

## 文件变更清单

| 操作 | 文件 | 说明 |
|------|------|------|
| **修改** | `android/build.gradle` | aar2apk plugin + modules 配置 |
| **修改** | `android/app/build.gradle` | packagePlugins 配置 |
| **修改** | `.github/workflows/android.yml` | 插件构建步骤 + Host/Plugin .so 分离验证 |
| **修改** | `src/views/HomePage.vue` | 新增「扩展管理」卡片 + grid 调整 |
| **新建** | `src/views/ExtensionsPage.vue` | 功能扩展管理二级页面 |
| **修改** | `src/router/index.ts` | /tabs/extensions 路由 |
| **修改** | `src/views/Settings.vue` | mpv → mpv-plugin 选项 |
| **修改** | `src/composables/useI18n.ts` | 扩展相关 i18n 键 |
| **修改** | `app/.../PlayerEntry.kt` | 读取 video_player pref + 路由 |
| **修改** | `plugin-mpv-player/.../MpvPlayerActivity.kt` | createMpvEngine() 实现 |
| **修改** | `plugin-mpv-player/.../MpvPlayerScreen.kt` | resolveStreamUrl() + 全屏 + insets |
| **修改** | `lynx-player/src/typing.d.ts` | PlayerBridgeModule 类型 |
| **修改** | Kotlin LynxViewBuilder | registerModule(PlayerBridgeModule) |
