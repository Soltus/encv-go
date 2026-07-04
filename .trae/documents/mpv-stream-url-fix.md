# 修复：MPV 播放 "Unable to get stream URL"

## 根因

`PlayerEntry.getBackendBaseUrl()` 硬编码 `"http://127.0.0.1:8899"`，实际后端端口是 `2025`。

## 排查发现的问题清单

### 问题 1（根因）：端口硬编码 8899

`PlayerEntry.kt:272` — `getBackendBaseUrl()` 返回 `"http://127.0.0.1:8899"`
实际：`EncvGoService.DEFAULT_PORT = 2025`，前端 `DEFAULT_API_BASE_URL = 'http://127.0.0.1:2025'`

### 问题 2：`resolveStreamUrl` 包含不符合规范的本地路径 fallback

当前逻辑：先检查本地文件是否存在 → 再构造 HTTP 流 URL。
正确逻辑：和前端 `getFileStreamUrl` 一视同仁，只构造 `$backendUrl/stream?path=$encodedPath`。
MPV 原生支持 HTTP 流，加密文件由后端 `/stream` 端点处理，不需要本地路径 fallback。

### 问题 3：`startPlayback` 中 `streamUrl` 校验逻辑有缺陷

```kotlin
if (streamUrl.isEmpty() || !streamUrl.startsWith("http")) {
    if (streamUrl.isEmpty()) {  // ← 只处理空，非空非 http 的路径被静默放行
        onError("Unable to get stream URL")
        return
    }
}
```

重写后 `resolveStreamUrl` 只返回 HTTP URL 或空字符串，此校验可简化。

### 问题 4：`backendUrl` 获取方式不够可靠

`MpvPlayerScreen` 和 `MpvAudioPlayerScreen` 都通过 `(context as? Activity)?.intent?.getStringExtra("backend_url")` 获取。
`MpvPlayerActivity` 已经从 `hostIntent` 读取了 `filePath` 等参数，`backend_url` 也应该在那里读取并直接传给 Compose，而不是让 Compose 自己从 Activity intent 取。

---

## 修复步骤

### Step 1：修复 `PlayerEntry.getBackendBaseUrl()` 端口

**文件**：`android/app/src/main/java/com/encvgo/app/PlayerEntry.kt`

动态读取 `EncvGoService.lastKnownPort`，回退 `2025`。

### Step 2：重写 `resolveStreamUrl` — 只构造 HTTP 流 URL

**文件**：`plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerScreen.kt`

删除所有本地路径识别逻辑，只做一件事：
- `backendUrl` 非空 → `$backendUrl/stream?path=$encodedPath`（或 external 变体）
- `backendUrl` 为空 → 返回空字符串

### Step 3：简化 `startPlayback` 校验逻辑

```kotlin
val streamUrl = resolveStreamUrl(filePath, isExternal, backendUrl)
if (streamUrl.isEmpty()) {
    onError("Unable to get stream URL")
    return
}
engine.play(streamUrl)
```

### Step 4：`MpvPlayerActivity` 直接传递 `backendUrl` 给 Compose

在 `MpvPlayerActivity.onCreate()` 中读取 `backend_url`，作为参数传给 `MpvPlayerScreen` / `MpvAudioPlayerScreen`，而不是让 Compose 从 `LocalContext.current` 取。

### Step 5：验证构建
