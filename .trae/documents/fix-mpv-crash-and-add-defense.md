# 修复播放闪退 + 增加 Crash 防御 + ffmpeg 说明

## 闪退根本原因

对比官方 `BaseMPVView.kt`（mpv-android 源码），我们的 `MpvPlayerModule.ensureMpvInitialized()` 有 **6 个关键缺陷**：

### 缺陷 1：缺少 `config-dir` 选项（致命）

官方代码：
```kotlin
MPVLib.create(context)
MPVLib.setOptionString("config", "yes")
MPVLib.setOptionString("config-dir", configDir)  // ← 我们缺这个！
```

我们设置了 `config=yes` 但没有指定 `config-dir`。mpv 会尝试使用默认路径（在 Android 上不存在），导致崩溃。

### 缺陷 2：缺少 GPU 缓存目录选项（致命）

官方代码：
```kotlin
for (opt in arrayOf("gpu-shader-cache-dir", "icc-cache-dir"))
    MPVLib.setOptionString(opt, cacheDir)  // ← 我们缺这个！
```

mpv 的 GPU 渲染器需要 shader 缓存目录。没有它，GPU 初始化时崩溃。

### 缺陷 3：`force-window` 设置时机错误

我们：`init()` 之前设置 `force-window=no`
官方：`init()` 之后设置 `force-window=no`

`force-window` 在 `init()` 之前设置会干扰 VO 初始化。

### 缺陷 4：`idle` 值错误

我们：`idle=yes`（mpv 永远空闲）
官方：`idle=once`（mpv 空闲一次后退出，配合 playFile 逻辑）

### 缺陷 5：Surface 生命周期管理不完整

官方 `surfaceDestroyed`：
```kotlin
MPVLib.setPropertyString("vo", "null")
MPVLib.setPropertyString("force-window", "no")  // ← 我们缺这个！
MPVLib.detachSurface()
```

### 缺陷 6：无 Crash 防御

所有 `MPVLib` 调用没有 try-catch 保护，任何 JNI 异常直接导致应用崩溃。

## ffmpeg 说明

**`libmpv.so` 已静态链接 ffmpeg**。mpv-android 的构建脚本将 ffmpeg 编译进 libmpv.so，不需要单独的 ffmpeg 库。通过 mpv API（如 `MPVLib.command()`）可以访问 ffmpeg 的所有解码/滤镜功能。

如果需要直接使用 ffmpeg 命令行工具（非 mpv API），才需要单独集成 ffmpeg。

## 执行计划

### 1. 重写 `MpvPlayerModule.ensureMpvInitialized()` — 对齐官方初始化流程

**文件**: `android-overlay/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt`

修改 `ensureMpvInitialized()` 方法：
- 使用 `activity.application` 的内部存储路径作为 `configDir`
- 使用 `activity.application` 的缓存路径作为 `cacheDir`
- 在 `create()` 后、`init()` 前设置 `config-dir`、`gpu-shader-cache-dir`、`icc-cache-dir`
- 在 `init()` 后设置 `force-window=no`、`idle=once`
- 移除 `init()` 前的 `force-window` 和 `idle` 设置
- 移除 `init()` 前的 `vo=gpu` 设置（使用默认值即可）

### 2. 修复 Surface 生命周期 — 对齐官方 BaseMPVView

**文件**: `android-overlay/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt`

修改 `MpvSurfaceView` 的回调：
- `surfaceCreated`: 保持 `attachSurface` + `force-window=yes`，但修复 VO 恢复逻辑
- `surfaceChanged`: 保持 `android-surface-size` 设置
- `surfaceDestroyed`: 添加 `force-window=no` 在 `detachSurface()` 之前

### 3. 增加 Crash 防御 — 所有 MPVLib 调用加 try-catch

**文件**: `android-overlay/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt`

- `ensureMpvInitialized()`: try-catch 包裹，失败时 dispatchStateChange("error", message)
- `play()`: 已有 try-catch，但需确保 callback 始终被调用
- `pause()/resume()/seekTo()` 等: 已有 try-catch
- `getDuration()/getCurrentPosition()/isPlaying()`: 加 try-catch，失败时返回安全默认值
- `dispatchPositionUpdate()`: 加 try-catch，避免定时器回调崩溃
- `attachToLayout()/detachFromLayout()/release()`: 加 try-catch
- `MpvSurfaceView` 回调: 加 try-catch，避免 Surface 操作崩溃

### 4. 增加 `pendingUrl` 机制 — 确保 Surface 就绪后才播放

**文件**: `android-overlay/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt`

当前 `play()` 直接调用 `MPVLib.command(arrayOf("loadfile", url))`，但 Surface 可能还没准备好。
修改为：
- 如果 Surface 未创建，将 URL 存入 `pendingUrl`
- 在 `surfaceCreated` 中检查 `pendingUrl`，如果有则播放
- 这与官方 `BaseMPVView.playFile()` 的逻辑一致

### 5. PlayerActivityLynx 加 Crash 防御

**文件**: `android-overlay/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt`

- `createLynxView()`: 加 try-catch，失败时显示错误 Toast 并 finish()
- `onDestroy()`: 已有 try-catch，但确保所有清理操作安全
- `positionUpdateRunnable`: 加 try-catch，避免定时器崩溃

### 6. 修复 JS 层事件数据格式

**文件**: `lynx-player/src/App.tsx`

`sendGlobalEvent` 发送的 `JavaOnlyArray` 中第一个元素是 `JavaOnlyMap`。
`useLynxGlobalEventListener` 回调接收的 `event` 就是那个 `JavaOnlyMap`。
所以应该用 `event.state` 而不是 `event.detail.state`。

修复事件数据访问方式，并添加 fallback 和调试日志。
