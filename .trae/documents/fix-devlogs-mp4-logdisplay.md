# 修复 Logcat 独立窗口 + MP4 播放 + DevLogs 日志显示

## 问题 1：Logcat 直接附加到主 Activity，没有作为独立窗口打开

### 根因

项目使用 `com.github.getActivity:Logcat:13.0` 库。该库的 `LogcatActivity` 默认没有在 AndroidManifest 中声明 `launchMode="singleInstance"`，导致它被附加到主 Activity 的 task 中，在主窗口内打开而不是独立窗口。

### 方案

在 AndroidManifest.xml 中显式声明 `LogcatActivity`，设置 `launchMode="singleInstance"` 使其作为独立窗口打开：

```xml
<activity
    android:name="com.hjq.logcat.LogcatActivity"
    android:configChanges="orientation|screenSize|keyboardHidden"
    android:launchMode="singleInstance"
    android:screenOrientation="portrait"
    android:theme="@style/Theme.AppCompat.Light.NoActionBar"
    tools:node="replace" />
```

关键点：
- `launchMode="singleInstance"`：使 LogcatActivity 在独立的 task 中打开，不附加到主 Activity
- `tools:node="replace"`：覆盖 Logcat 库自身 manifest 中的默认声明
- 需要在 `<manifest>` 标签添加 `xmlns:tools="http://schemas.android.com/tools"` 命名空间

## 问题 2：无法播放普通 MP4 文件

### 根因（从 logcat 确认）

```
[Detector] FAILED: Footer magic number mismatch. Expected: ENVC, Got: ����
Container is invalid and not cached, rebuilding as last resort
ERROR: failed to get a readable container path for '...mp4'
```

`/stream` 端点（`handleStreamRequest`）总是调用 `serveEncryptedFile()`，对所有文件都尝试 ENCV 容器检测和解密。普通 MP4 不是 ENCV 容器，检测失败后 rebuild 也失败 → HTTP 500。

### 方案

在 `handleStreamRequest` 中，先检测文件是否为 ENCV 容器。如果不是，直接用 `http.ServeFile` 提供原始文件：

```go
_, err := detector.DetectContainer(cleanedFilePath)
if err != nil {
    slog.Info("File is not an ENCV container, serving raw file", "path", cleanedFilePath)
    http.ServeFile(w, r, cleanedFilePath)
    return
}
s.serveEncryptedFile(w, r, cleanedFilePath)
```

## 问题 3：DevLogs 没看到更多前后端日志

### 根因

**后端日志未到达前端**：`WSLogHandler` 发送 `{type:"log", level:"info", message:"..."}`，但 `useWebSocket.ts` 解析为 `{type, data}`，`data` 字段为 undefined。DevLogs 的 `onWsMessage` 收到 `{type:"log", data:undefined}`，`data.type` 不是 `"log"`，走了 fallback。

**前端日志缺失**：`hijackConsole()` 只在 DevLogs 页面激活（`onMounted/onUnmounted`），离开页面就失效。

### 方案

**A. 修复 WSLogHandler 消息格式**：改为 `{type: "log", data: {level, message, timestamp}}`

**B. 修复 DevLogs onWsMessage**：适配新消息格式，从 `data.data` 中提取 level/message/timestamp

**C. 前端日志全局收集**：将 `hijackConsole()` 从 DevLogs 移到 App.vue，使用全局 composable `useFrontendLogs` 持久化收集

## 文件变更清单

| 文件 | 变更 |
|------|------|
| `app/encv-mobile/android-overlay/app/src/main/AndroidManifest.xml` | 添加 `LogcatActivity` 声明（singleInstance）+ tools 命名空间 |
| `internal/server/server_handle.go` | `handleStreamRequest` 添加非加密文件降级 |
| `internal/server/ws_log_handler.go` | 消息格式改为 `{type:"log", data:{level,message,timestamp}}` |
| `app/encv-mobile/src/views/DevLogs.vue` | 适配新消息格式；从全局读取前端日志 |
| `app/encv-mobile/src/composables/useFrontendLogs.ts` | 新建：全局前端日志收集器 |
| `app/encv-mobile/src/App.vue` | 使用全局前端日志收集器 |
