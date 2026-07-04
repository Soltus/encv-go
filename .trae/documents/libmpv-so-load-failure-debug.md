# 饱和调试：libmpv.so 加载失败 (UnsatisfiedLinkError)

## 根因分析

### 错误堆栈
```
java.lang.UnsatisfiedLinkError: dlopen failed: library "libmpv.so" not found
  at is.xyz.mpv.MPVLib.<clinit>(MPVLib.kt:17)
  at com.encvgo.plugin.mpv.MpvEngine.initialize(MpvEngine.kt:185)
  at com.encvgo.plugin.mpv.MpvPlayerActivity.onCreate(MpvPlayerActivity.kt:40)
  at com.combo.core.component.activity.BaseHostActivity.onCreate(BaseHostActivity.kt:82)
  at com.encvgo.app.EncvHostActivity.onCreate(EncvHostActivity.kt:13)
```

### 根因：Java ClassLoader 父委托机制 + 宿主模块残留 MPVLib.kt

**`is.xyz.mpv.MPVLib` 同时存在于两个模块：**

| 模块 | 文件路径 | 编译？ | 含 so？ |
|------|---------|--------|---------|
| `:app` | `android/app/src/main/java/is/xyz/mpv/MPVLib.kt` | ✅ 是（在 src/main/java 中） | ❌ 无（jniLibs 不存在） |
| `:plugin-mpv-player` | `plugin-mpv-player/src/main/java/is/xyz/mpv/MPVLib.kt` | ✅ 是 | ✅ 有（jniLibs/arm64-v8a/ 含 11 个 .so） |

**致命链路（Java ClassLoader 父委托）：**

```
MpvPlayerActivity.onCreate()  ← 由 PluginClassLoader 加载
  → MpvEngine.initialize()
    → MPVLib.create(context)   ← 触发 MPVLib 类初始化
      → JVM: loadClass("is.xyz.mpv.MPVLib")
        → PluginClassLoader.loadClass()
          → ① 先问 parent（宿主 PathClassLoader）
          → ② 宿主 APK 中有 is.xyz.mpv.MPVLib → 命中！
          → ③ 返回宿主的 MPVLib 类（由宿主 ClassLoader 加载）
      → MPVLib.<clinit>() 执行 init 块
        → System.loadLibrary("mpv")
          → 使用调用者类（MPVLib）的 ClassLoader = 宿主 PathClassLoader
          → 宿主 PathClassLoader 只搜索宿主 APK 的 native lib 目录
          → 宿主 APK 中没有 libmpv.so → UnsatisfiedLinkError!
```

**如果删除宿主的 MPVLib.kt 后的正确链路：**

```
MpvPlayerActivity.onCreate()  ← 由 PluginClassLoader 加载
  → MpvEngine.initialize()
    → MPVLib.create(context)   ← 触发 MPVLib 类初始化
      → JVM: loadClass("is.xyz.mpv.MPVLib")
        → PluginClassLoader.loadClass()
          → ① 先问 parent（宿主 PathClassLoader）
          → ② 宿主 APK 中没有 is.xyz.mpv.MPVLib → ClassNotFoundException
          → ③ PluginClassLoader.findClass() → 从插件 DEX 加载
          → ④ 返回插件的 MPVLib 类（由 PluginClassLoader 加载）
      → MPVLib.<clinit>() 执行 init 块
        → System.loadLibrary("mpv")
          → 使用调用者类（MPVLib）的 ClassLoader = PluginClassLoader
          → PluginClassLoader.findLibrary("mpv")
            → 搜索 librarySearchPath = plugins/$pluginId/lib/arm64-v8a/
            → libmpv.so 存在（InstallerManager.extractNativeLibs 解压）→ ✅ 找到！
```

### 证据链

1. **宿主 MPVLib.kt 文件头部注释**：`// MIGRATED to :plugin-mpv-player module — kept for reference only` — 说明已迁移但未删除
2. **宿主 JNI 目录 MIGRATED.txt**：`NOT compiled — this is the overlay variant, no externalNativeBuild configured` — JNI 源码未编译但 Kotlin 代码仍在编译
3. **宿主 jniLibs 目录不存在**：`android/app/src/main/jniLibs/` → Glob 返回空
4. **插件 jniLibs 完整**：`plugin-mpv-player/src/main/jniLibs/arm64-v8a/` 含 11 个 .so 文件
5. **app 模块无其他代码引用 MPVLib**：Grep 确认只有 `MPVLib.kt` 自身
6. **combolite-host 模块无 MPVLib 引用**：Grep 确认
7. **ComboLite PluginClassLoader 源码**：继承 DexClassLoader，构造时传入 `librarySearchPath`，`findClass` 先走父委托再 fallback
8. **PluginLifecycleManager.loadPlugin()**：构建 `nativeLibDir = File(pluginInstallDir, "lib/$abi")`，传给 PluginClassLoader
9. **InstallerManager.extractNativeLibs()**：从 APK 解压 `lib/` 下的 so 到 `plugins/$pluginId/lib/$abi/`

---

## 修复步骤

### Step 1：删除宿主模块残留的 MPVLib.kt

**文件**：`android/app/src/main/java/is/xyz/mpv/MPVLib.kt`

这是根因文件。删除后，PluginClassLoader 的父委托将找不到 `is.xyz.mpv.MPVLib`，从而正确地从插件 DEX 加载。

### Step 2：删除宿主模块残留的 JNI 源码目录

**目录**：`android/app/src/main/jni/`

包含 MIGRATED.txt + 完整 JNI 源码（main.cpp, event.cpp 等），均未编译（无 externalNativeBuild 配置）。这些文件仅作"参考"保留，实际已全部迁移到 plugin-mpv-player 模块。删除可避免混淆。

### Step 3：验证构建

运行 Gradle 构建确认：
- 宿主 APK 不再包含 `is.xyz.mpv.MPVLib` 类
- 插件 APK 仍包含 `is.xyz.mpv.MPVLib` 类和所有 .so 文件

### Step 4：添加 ClassLoader 诊断日志（防御性）

在 `MpvPlayerActivity.onCreate()` 中添加诊断日志，记录 `MPVLib` 的 ClassLoader 类型，以便未来快速定位类似问题：

```kotlin
val cl = MPVLib::class.java.classLoader
Log.i(TAG, "MPVLib ClassLoader: ${cl?.javaClass?.name}")
```

---

## 影响范围

- **删除的文件**：`android/app/src/main/java/is/xyz/mpv/MPVLib.kt` + `android/app/src/main/jni/` 整个目录
- **无破坏性影响**：已确认 app 模块中无其他代码引用 `is.xyz.mpv.MPVLib`
- **构建影响**：宿主 APK 体积可能略微减小（少了一个未使用的类）
- **运行时影响**：插件加载 MPVLib 时将正确使用 PluginClassLoader，native library 搜索路径生效

---

## 风险评估

| 风险 | 可能性 | 影响 | 缓解 |
|------|--------|------|------|
| 删除后编译失败 | 极低 | 无（无引用） | Grep 已确认 |
| 插件 MPVLib 仍加载失败 | 低 | 播放器不可用 | 诊断日志 + librarySearchPath 检查 |
| 其他类存在类似父委托问题 | 低 | 需逐个排查 | 目前只发现 MPVLib 一个冲突类 |
