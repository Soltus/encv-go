# Combolite 源码全面研究与 MPV 扩展安装问题诊断计划

## 一、研究目标

用户反馈 **MPV 扩展（基于 ComboLite 框架的插件化播放器）无法安装**。需要：
1. 克隆 Combolite 源码到临时文件夹（不在项目路径内）✅ 已完成
2. 全面研读源码，理解框架的工作机制、打包流程、安装流程、API 用法 ✅ 已完成
3. 对比当前项目 `plugin-mpv-player` 的实现与框架要求，找出所有不匹配之处 ✅ 已完成
4. 诊断安装失败的根本原因并给出修复方案 ← 当前步骤

---

## 二、Combolite 框架核心机制（基于源码分析）

### 2.1 框架架构

```
┌──────────────────────────────────────────────────────┐
│                   Host App (宿主)                      │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────┐   │
│  │BaseHostApp  │→ │PluginManager │  │ProxyManager│   │
│  │(Application)│  │ (单例对象)    │  │(组件代理)   │   │
│  └─────────────┘  └──────┬───────┘  └─────┬──────┘   │
│                          │                │          │
│  ┌───────────────────────▼────────────────▼────────┐ │
│  │              InstallerManager (安装器)            │ │
│  │  installPlugin() → 验证签名 → 解析Manifest        │ │
│  │  → 复制APK → 解压so → 创建类索引 → 写plugins.xml  │ │
│  └─────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
                           ↓ 安装/加载
┌──────────────────────────────────────────────────────┐
│              Plugin APK (插件)                        │
│  ┌────────────┐  ┌───────────┐  ┌────────────────┐  │
│  │IPluginEntry│  │BasePlugin │  │  jniLibs (.so) │  │
│  │  Class     │  │ Activity  │  │  libmpv/ffmpeg  │  │
│  │(入口接口)   │  │(可选)     │  │                │  │
│  └────────────┘  └───────────┘  └────────────────┘  │
│  AndroidManifest.xml:                                │
│    package="com.encvgo.plugin.mpv"  ← pluginId      │
│    versionCode="1"                                   │
│    versionName="1.0"                                 │
│    <meta-data name="plugin.entryClass"               │
│               value="...MpvPluginEntry" />  ← 必须！ │
└──────────────────────────────────────────────────────┘
```

### 2.2 完整生命周期（源码追踪）

```
[构建阶段] Gradle 构建
  aar2apk 插件:
    1. assemble plugin module → 生成 .aar
    2. ConvertAarToApkTask: .aar → .apk（注入 AndroidManifest，签名）
    3. 输出到 build/outputs/plugin-apks/{debug|release}/
    4. packagePlugins enabled → 自动复制到 assets/{pluginsDir}/

[运行阶段 - Debug]
  BaseHostApplication.onCreate()
    → super.onCreate() [内部调用 PluginManager.initialize()]
      → onFrameworkSetup() 回调:
        → PluginManager.proxyManager.setHostActivity(...)
        → PluginManager.setValidationStrategy(...)

  BaseHostActivity.onCreate()
    → installPluginsFromAssetsForDebug("debug_plugins")
      → 遍历 assets 中所有 .apk
      → PluginManager.installerManager.installPlugin(apk, forceOverwrite=true)
        → validateAndParseConfig(): 解析 Manifest 获取 package/entryClass
        → checkSignatureAndAuthorize(): 签名校验
        → copyPluginApk() + extractNativeLibs() + createClassIndex()
        → 更新 plugins.xml
    → PluginManager.loadEnabledPlugins()
      → 加载 plugins.xml 中所有 enabled 插件
      → PluginClassLoader 加载 DEX
    → PluginManager.launchPlugin(pluginId)
      → 实例化 IPluginEntryClass（通过反射调用 entryClass）
      → 调用 onLoad(context)
      → 调用 Content() 获取 Compose UI

[运行阶段 - Release]
  用户手动安装 APK 文件
  → PluginManager.installerManager.installPlugin(apkFile)
  → 同上...
```

### 2.3 关键 API 签名（来自源码）

**IPluginEntryClass 接口** (`comboLite-core/.../api/IPluginEntryClass.kt`):
```kotlin
interface IPluginEntryClass {
    val pluginModule: List<Module>           // Koin DI 模块
    fun onLoad(context: PluginContext)       // 加载回调
    fun onUnload()                           // 卸载回调
    @Composable fun Content()                // Compose UI 入口
}
```

**InstallerManager.validateAndParseConfig() 核心校验逻辑** (L399-462):
```kotlin
// 必须从 Manifest 解析出以下字段，缺一不可：
val pluginId = packageInfo.packageName          // ← manifest 的 package 属性
val entryClass = metaData.getString("plugin.entryClass")  // ← meta-data
if (pluginId.isBlank() || entryClass.isNullOrBlank()) {
    return null  // 校验失败！安装终止
}
```

---

## 三、诊断结果：当前项目 vs 框架要求

### 🔴 P0 — 致命阻塞问题（不修复则完全无法工作）

#### 问题 1：宿主 App **完全没有初始化 Combolite 框架**

**当前状态：**
- [EncvApplication.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvApplication.kt) 继承 `Application()`（普通 Application）
- 无 `PluginManager.initialize()` 调用
- 无 proxyManager 配置
- 无 ValidationStrategy 设置
- 无 `installPluginsFromAssetsForDebug()` 调用
- 无 `loadEnabledPlugins()` / `launchPlugin()` 调用

**框架要求（参考 [HostApp.kt](file:///tmp/combolite-research/app/src/main/java/com/combo/plugin/sample/HostApp.kt)）：**
```kotlin
class HostApp : BaseHostApplication(), IPluginCrashCallback {
    override fun onFrameworkSetup(): suspend () -> Unit = {
        PluginManager.proxyManager.apply {
            setHostActivity(HostActivity::class.java)
            setServicePool(listOf(...))
            setHostProviderAuthority("com.combo.plugin.sample.provider")
        }
        setValidationStrategy(ValidationStrategy.UserGrant)
    }
}
```

**影响：** 即使插件 APK 打包正确，运行时也没有任何代码去加载和启动它。**这是最根本的原因。**

---

#### 问题 2：插件 AndroidManifest.xml **缺少关键元数据**

**当前状态** ([AndroidManifest.xml](file:///workspace/app/encv-mobile/plugin-mpv-player/src/main/AndroidManifest.xml))：
```xml
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".MpvPlayerActivity" ... />
    </application>
</manifest>
```

**框架要求**（参考 [sample-plugin/home/.../AndroidManifest.xml](file:///tmp/combolite-research/sample-plugin/home/src/main/AndroidManifest.xml)）：
```xml
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.encvgo.plugin.mpv"    <!-- ← 缺失！作为 pluginId -->
    android:versionCode="1"            <!-- ← 缺失！ -->
    android:versionName="1.0">         <!-- ← 缺失！ -->

    <application
        android:label="MPV Player">
        <!-- ← 缺失！以下是必须的 meta-data -->
        <meta-data
            android:name="plugin.entryClass"
            android:value="com.encvgo.plugin.mpv.MpvPluginEntry" />
        <meta-data
            android:name="plugin.description"
            android:value="MPV media player plugin based on libmpv" />
        
        <activity android:name=".MpvPlayerActivity" ... />
    </application>
</manifest>
```

**影响：** `validateAndParseConfig()` 在 L441 会检查 `pluginId.isBlank() || entryClass.isNullOrBlank()`，两者都为空 → 返回 `null` → 安装直接报错 **"插件配置元数据验证失败"**

---

### 🟠 P1 — 高优先级问题（构建产物可能不正确）

#### 问题 3：`packagePlugins` 配置不完整

**当前状态** ([app/build.gradle.kts](file:///workspace/app/encv-mobile/android/app/build.gradle.kts))：
```kotlin
packagePlugins {
    enabled.set(true)
    // ❌ 缺少 buildType
    // ❌ 缺少 pluginsDir
}
```

**框架要求**（参考 Combolite sample [app/build.gradle.kts](file:///tmp/combolite-research/app/build.gradle.kts) L114-118）：
```kotlin
packagePlugins {
    enabled.set(true)
    buildType.set(PackageBuildType.RELEASE)  // 或 DEBUG
    pluginsDir.set("debug_plugins")            // assets 子目录名
}
```

**影响：** `buildType` 和 `pluginsDir` 有默认值（可能是 `"plugins"`），但不显式设置可能导致产物输出到意外路径或使用错误的构建类型。

---

### 🟡 P2 — 中优先级问题（功能异常）

#### 问题 4：MpvPlayerActivity 继承了错误的基类

**当前状态：** `class MpvPlayerActivity : AppCompatActivity()`
**框架建议：** 应继承 `BasePluginActivity`（如果需要在插件内部独立启动 Activity）

**注意：** 对于纯 Compose UI 插件（只使用 `IPluginEntryClass.Content()`），不一定需要 Activity。但如果 MpvPlayerActivity 是从插件内部跳转的页面，则需要继承 `BasePluginActivity`。

**实际影响取决于使用方式：** 如果 MPV 播放器界面是通过 `Content()` Compose 函数渲染的（当前 MpvPlayerScreen.kt 存在），则 Activity 基类问题可能不阻塞。但 `MpvPlayerActivity` 目前被注册在 Manifest 中，说明它可能被直接启动。

---

#### 问题 5：MpvPluginEntry.Content() 为空实现

**当前状态：**
```kotlin
@Composable
override fun Content() { }  // 空！无任何 UI
```

**框架要求：** 应返回插件的 Compose UI 内容（如 `MpvPlayerScreen()`）

**影响：** 即使插件成功加载和启动，用户看到的也是空白界面。

---

### 🔵 P3 — 低优先级 / 信息项

#### 问题 6：jniLibs 构建配置

**当前状态：** 插件模块有 `src/main/jni/` 目录（含 C++ 源码 + Android.mk/Application.mk），但未见预编译的 `.so` 文件。

**分析：** NDK 构建会在编译时生成 `.so` 并打入 APK。需要确认 `build.gradle.kts` 或 `Android.mk` 正确配置了 NDK 编译和 ABI 过滤。

#### 问题 7：JVM target 版本差异

- 项目宿主 app: `jvmTarget = JVM_21`
- Combolite sample: `jvmTarget = JVM_17`

**影响：** 可能导致编译时版本冲突警告。

---

## 四、根因分析总结

### 安装失败的完整因果链

```
P0#1 宿主未初始化 Combolite 框架
  └→ 没有 PluginManager.initialize()
    └→ 没有 installPluginsFromAssetsForDebug()
      └→ assets 中的插件 APK 从未被安装到 filesDir/plugins/
        └→ loadEnabledPlugins() 从未被调用
          └→ 插件从未被加载和启动
            └→ 【结果】MPV 扩展"无法安装"

P0#2 插件 Manifest 缺少元数据
  └→ 即使手动触发安装，validateAndParseConfig() 失败
    └→ "核心元数据 (package, entryClass) 不能为空"
      └→ 【结果】安装报错 "插件配置元数据验证失败"

P1#3 packagePlugins 配置不完整
  └→ aar2apk 可能未正确生成/放置插件 APK 到 assets
    └→ 【结果】assets 中找不到插件 APK 文件
```

**结论：存在 2 个 P0 致命问题和 1 个 P1 高优先级问题，它们共同导致 MPV 扩展无法安装。其中 P0#1（框架未初始化）是最根本的原因。**

---

## 五、修复方案清单

### Fix #1: [P0] 宿主 App 初始化 Combolite 框架

**文件：** `app/src/main/java/com/encvgo/app/EncvApplication.kt`

**修改内容：**
1. 改为继承 `BaseHostApplication`
2. 实现 `onFrameworkSetup()` 回调
3. 配置 ProxyManager（至少 setHostActivity）
4. 设置 ValidationStrategy（开发期可用 Insecure）

**文件：** `app/src/main/java/com/encvgo/app/MainActivity.kt`

**修改内容：**
1. 改为继承 `BaseHostActivity`（或确认 BridgeActivity 与 BaseHostActivity 兼容）
2. 在 onCreate 中调用 `installPluginsFromAssetsForDebug()` + `loadEnabledPlugins()` + `launchPlugin()`

> ⚠️ 注意：当前 MainActivity 继承的是 Capacitor 的 `BridgeActivity`，需要评估与 `BaseHostActivity` 的兼容性。可能需要创建单独的插件入口 Activity。

### Fix #2: [P0] 补全插件 AndroidManifest.xml 元数据

**文件：** `plugin-mpv-player/src/main/AndroidManifest.xml`

**修改内容：**
1. 添加 `package="com.encvgo.plugin.mpv"`
2. 添加 `android:versionCode="1"`
3. 添加 `android:versionName="1.0"`
4. 添加 `<meta-data android:name="plugin.entryClass" android:value="com.encvgo.plugin.mpv.MpvPluginEntry" />`
5. 添加 `<meta-data android:name="plugin.description" ... />`

### Fix #3: [P1] 补全 packagePlugins 配置

**文件：** `android/app/build.gradle.kts`

**修改内容：**
```kotlin
packagePlugins {
    enabled.set(true)
    buildType.set(PackageBuildType.DEBUG)     // 新增
    pluginsDir.set("debug_plugins")             // 新增
}
```

### Fix #4: [P2] 完善 MpvPluginEntry.Content()

**文件：** `plugin-mpv-player/.../MpvPluginEntry.kt`

**修改内容：** 在 `Content()` 中返回 `MpvPlayerScreen()` Compose UI

### Fix #5: [P2] 评估 MpvPlayerActivity 基类

**选项 A：** 如果 MPV 播放器仅通过 `Content()` 渲染，可移除 Activity 或改为 `BasePluginActivity`
**选项 B：** 如果需要独立 Activity，改为继承 `BasePluginActivity`

### Fix #6: [P3] 确认 NDK/.so 构建链

验证 `jni/Android.mk` 能正确编译出 `libmpv_jni.so` 等 .so 文件并打入插件 APK

---

## 六、实施顺序建议

```
Fix #2 (Manifest 元数据)     ← 最快见效，立即解除安装校验阻塞
    ↓
Fix #3 (packagePlugins)     ← 确保 APK 正确生成和放置
    ↓
Fix #1 (宿主框架初始化)      ← 最大工作量，需要仔细处理 Capacitor 兼容性
    ↓
Fix #4 (Content UI)         ← 让插件有实际界面
    ↓
Fix #5 (Activity 基类)      ← 根据实际需求决定
    ↓
Fix #6 (NDK/.so 验证)      ← 确认构建链完整
```

---

## 七、Combolite 源码位置参考

克隆路径：`/tmp/combolite-research/`

| 关键文件 | 路径 | 作用 |
|---------|------|------|
| IPluginEntryClass | `comboLite-core/.../api/IPluginEntryClass.kt` | 插件入口接口定义 |
| PluginManager | `comboLite-core/.../runtime/PluginManager.kt` | 核心管理器单例 |
| InstallerManager | `comboLite-core/.../runtime/installer/InstallerManager.kt` | 安装/卸载逻辑 |
| BasePluginActivity | `comboLite-core/.../component/activity/BasePluginActivity.kt` | 插件 Activity 基类 |
| BaseHostApplication | `comboLite-core/.../runtime/app/BaseHostApplication.kt` | 宿主 Application 基类 |
| BaseHostActivity | `comboLite-core/.../component/activity/BaseHostActivity.kt` | 宿主 Activity 基类 |
| Extensions (installFromAssets) | `comboLite-core/.../utils/Extensions.kt` | Debug 安装工具函数 |
| Aar2ApkPlugin | `build-logic/aar2apk/.../Aar2ApkPlugin.kt` | Gradle 插件实现 |
| Sample HostApp | `app/src/main/java/.../HostApp.kt` | 宿主集成参考 |
| Sample HostActivity | `app/src/main/java/.../HostActivity.kt` | 宿主 Activity 参考 |
| Sample PluginEntry | `sample-plugin/home/.../PluginEntryClass.kt` | 插件入口参考 |
| Sample Manifest | `sample-plugin/home/.../AndroidManifest.xml` | 插件 Manifest 模板 |
| Sample app build.gradle | `app/build.gradle.kts` | packagePlugins 配置参考 |
