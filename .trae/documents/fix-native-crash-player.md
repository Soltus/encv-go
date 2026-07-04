# 修复播放页面 Native Crash（闪退）

## 问题分析

### logcat 关键线索

```
03:48:39.640  No implementation found for DevToolLifecycle.nativeSyncStateToNative  ← DevTool native 库未加载
03:48:40.114  libEGL eglQueryStringImpl display is EGL_NO_DISPLAY                  ← EGL 初始化异常
03:48:51.149  lynx_inspector_owner_native_glue.cc(53)] ptr is null                 ← Inspector 空指针！
03:48:51.208  lynx_inspector_owner_native_glue.cc(53)] ptr is null                 ← Inspector 空指针！
03:48:51.210  lynx_inspector_owner_native_glue.cc(53)] ptr is null                 ← Inspector 空指针！
03:48:51.217  layout_context.cc(549)] DestroyLayoutNodeBeforeRemoveFromParent      ← 布局引擎异常
03:48:51.433  libcrashpad_handler_trampoline.so  ← Native Crash！进程被杀
03:49:21.457  com.encvgo.app  BBinder_init  ← 应用重启（新 PID）
```

### 根因定位

**主因：Lynx DevTool/Inspector 在 native 库缺失时被启用，导致 native 空指针崩溃。**

证据链：
1. `EncvApplication.initLynxEnv()` 调用 `enableDevtool(BuildConfig.DEBUG)` — debug 构建时 DevTool 被启用
2. logcat 明确报 `No implementation found for DevToolLifecycle.nativeSyncStateToNative` — DevTool 的 JNI native 实现不存在
3. 紧接着 `lynx_inspector_owner_native_glue.cc(53)] ptr is null` — Inspector 尝试访问空指针
4. 0.2 秒后 `libcrashpad_handler_trampoline.so` 触发 — SIGSEGV 导致进程被杀
5. **无 Java 堆栈** — 纯 native crash，进程直接被内核终止

**次因：MPV 初始化线程安全问题（之前已部分修复，但 `@LynxMethod play()` 仍有隐患）**

- `play()` 在 Lynx JS 线程调用 `ensureMpvInitialized()`
- `ensureMpvInitialized()` 检测到非主线程后直接 return → `mpvInitialized` 仍为 false
- `play()` 返回 "MPV not initialized" → 播放失败（不会 crash，但功能不可用）

### 为什么 DevTool native 库缺失？

Gradle 依赖中已包含 `lynx-devtool:3.7.0` 和 `lynx-service-devtool:3.7.0`（Java 类存在），但 JNI native 实现未加载。可能原因：
- DevTool native .so 未被正确打包进 APK（与 `abiFilters 'arm64-v8a'` 或 AAR 解包有关）
- DevTool native 库需要显式 `System.loadLibrary()` 但 Lynx SDK 未自动调用

## 修复方案

### Step 1：安全启用 Lynx DevTool（修复 crash 主因）

**文件**：`EncvApplication.kt`

**方案**：在启用 DevTool/Debug/LogBox 前，检查 native 库是否可用。不可用则跳过，避免 inspector 空指针崩溃。

```kotlin
private fun initLynxEnv(app: Application) {
    try {
        LynxEnv.inst().init(app, null, null, null)
        val devtoolAvailable = try {
            System.loadLibrary("lynx_devtool")
            true
        } catch (_: UnsatisfiedLinkError) {
            false
        }
        if (devtoolAvailable && BuildConfig.DEBUG) {
            LynxEnv.inst().enableLynxDebug(true)
            LynxEnv.inst().enableDevtool(true)
            LynxEnv.inst().enableLogBox(true)
        }
    } catch (e: Exception) {
        Log.e(TAG, "initLynxEnv failed", e)
    }
}
```

> 注意：`lynx_devtool` 是推测的 so 名，实际名需要检查 APK 内的 .so 文件确认。如果无法确认，最安全的做法是**始终不启用 DevTool**（对播放器功能无影响）。

### Step 2：Activity.onCreate() 中预初始化 MPV（修复线程安全）

**文件**：`PlayerActivityLynx.kt`

在 `onCreate()` 中、`createLynxView()` 之前，在主线程上初始化 MPV 引擎。这样当 JS 调用 `play()` 时，MPV 已经就绪。

```kotlin
override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    // ...
    setContentView(R.layout.lynx_player_activity)
    rootLayout = findViewById(R.id.lynx_player_root)

    // 预初始化 MPV（主线程）
    MpvPlayerModule.preInit(this)

    resolveFileInfo(intent)
    createLynxView()
    handleBackend()
}
```

**文件**：`MpvPlayerModule.kt`

添加 `preInit()` 静态方法，在主线程上提前加载 native 库和初始化 MPV 引擎：

```kotlin
companion object {
    // ...
    @Volatile
    private var preInitialized = false

    fun preInit(context: android.content.Context) {
        if (preInitialized) return
        if (Looper.myLooper() != Looper.getMainLooper()) {
            LogRelay.get().relay(TAG, "error", "preInit: must be on main thread!")
            return
        }
        try {
            val configDir = context.filesDir.absolutePath + "/mpv"
            val cacheDir = context.cacheDir.absolutePath + "/mpv"
            java.io.File(configDir).mkdirs()
            java.io.File(cacheDir).mkdirs()
            MPVLib.create(context)
            MPVLib.setOptionString("config", "yes")
            MPVLib.setOptionString("config-dir", configDir)
            for (opt in arrayOf("gpu-shader-cache-dir", "icc-cache-dir")) {
                MPVLib.setOptionString(opt, cacheDir)
            }
            MPVLib.setOptionString("vo", "gpu")
            MPVLib.setOptionString("hwdec", "auto")
            MPVLib.init()
            MPVLib.setOptionString("force-window", "no")
            MPVLib.setOptionString("idle", "once")
            preInitialized = true
            LogRelay.get().relay(TAG, "info", "preInit: MPV engine initialized on main thread")
        } catch (e: Exception) {
            LogRelay.get().relay(TAG, "error", "preInit: failed: ${e.message}")
        }
    }
}
```

修改 `ensureMpvInitialized()` 以利用预初始化结果：

```kotlin
private fun ensureMpvInitialized() {
    if (mpvInitialized) return
    if (Looper.myLooper() != Looper.getMainLooper()) {
        if (preInitialized) {
            mpvInitialized = true
            MPVLib.addObserver(eventObserver)
            MPVLib.observeProperty("pause", MpvFormat.MPV_FORMAT_FLAG)
            MPVLib.observeProperty("idle", MpvFormat.MPV_FORMAT_FLAG)
            dispatchStateChange("mpv_ready")
            return
        }
        LogRelay.get().relay(TAG, "error", "ensureMpvInitialized: not on main thread and not pre-initialized")
        return
    }
    // 主线程路径：如果已预初始化，只需注册 observer
    if (preInitialized) {
        mpvInitialized = true
        MPVLib.addObserver(eventObserver)
        MPVLib.observeProperty("pause", MpvFormat.MPV_FORMAT_FLAG)
        MPVLib.observeProperty("idle", MpvFormat.MPV_FORMAT_FLAG)
        dispatchStateChange("mpv_ready")
        return
    }
    // 未预初始化，在主线程上完整初始化
    // ... 原有逻辑
}
```

### Step 3：修复 `@LynxMethod play()` 线程安全

**文件**：`MpvPlayerModule.kt`

`play()` 在 JS 线程被调用。如果 MPV 已预初始化，`ensureMpvInitialized()` 可以在非主线程完成（只注册 observer）。如果未预初始化，需要通过 `mainHandler.post` 延迟初始化。

```kotlin
@LynxMethod
fun play(url: String, callback: Callback) {
    LogRelay.get().relay(TAG, "info", "play: url=$url, surfaceReady=$surfaceReady, mpvInitialized=$mpvInitialized, preInitialized=$preInitialized")
    try {
        ensureMpvInitialized()
        if (!mpvInitialized) {
            if (!preInitialized) {
                mainHandler.post {
                    try {
                        ensureMpvInitialized()
                        if (mpvInitialized && surfaceReady) {
                            MPVLib.command(arrayOf("loadfile", url))
                        } else if (mpvInitialized) {
                            pendingUrl = url
                            dispatchStateChange("waiting_surface")
                        }
                    } catch (e: Exception) {
                        LogRelay.get().relay(TAG, "error", "play delayed init failed: ${e.message}")
                    }
                }
                callback.invoke("MPV initializing, will play when ready")
                return
            }
            callback.invoke("MPV not initialized")
            return
        }
        if (surfaceReady) {
            MPVLib.command(arrayOf("loadfile", url))
        } else {
            pendingUrl = url
            dispatchStateChange("waiting_surface")
        }
        callback.invoke(true)
    } catch (e: Exception) {
        LogRelay.get().relay(TAG, "error", "play failed: ${e.message}")
        dispatchStateChange("error", "Play failed: ${e.message}")
        callback.invoke(e.message)
    }
}
```

### Step 4：Activity 驱动 MPV SurfaceView 生命周期

**文件**：`PlayerActivityLynx.kt`

在 `onCreate()` 中预初始化 MPV 后，直接在主线程创建并添加 SurfaceView，不再依赖 Lynx 回调：

```kotlin
override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    // ...
    setContentView(R.layout.lynx_player_activity)
    rootLayout = findViewById(R.id.lynx_player_root)

    MpvPlayerModule.preInit(this)
    attachMpvSurface()  // 主线程直接 attach

    resolveFileInfo(intent)
    createLynxView()
    handleBackend()
}

private fun attachMpvSurface() {
    val mpvModule = MpvPlayerModule.getInstance() ?: return
    if (mpvModule.isAttached()) return
    val root = rootLayout ?: return
    mpvModule.attachToLayout(root)
}
```

`tryAttachMpvModule()` 保留作为兜底，但正常流程不再依赖它。

### Step 5：release 时清理 preInit 状态

**文件**：`MpvPlayerModule.kt`

```kotlin
fun release() {
    // ... 原有清理逻辑
    preInitialized = false
    mpvInitialized = false
    surfaceReady = false
    pendingUrl = null
    mpvSurfaceView = null
    _instance = null
}
```

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `EncvApplication.kt` | DevTool 启用前检查 native 库可用性 |
| `MpvPlayerModule.kt` | 添加 `preInit()`、修改 `ensureMpvInitialized()`、修复 `play()` 线程安全、release 清理 |
| `PlayerActivityLynx.kt` | `onCreate()` 中调用 `preInit()` + `attachMpvSurface()` |

## 风险评估

- **Step 1 是关键修复**：不修复则 debug 构建必现 crash
- **Step 2-4 是防御性修复**：即使 Step 1 修复了 crash，MPV 线程安全问题仍可能导致播放失败
- **Step 5 是清理**：防止 Activity 销毁后状态残留
