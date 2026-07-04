# 修复：加密视频回滚 ArtPlayer + MPV 加载状态转圈圈

## 问题分析

### 问题 1：加密视频回滚到 ArtPlayer

用户已设置播放器为 MPV，但加密视频仍然回滚到 ArtPlayer。有两条回滚路径：

**路径 A：`handleLongPress` 未识别加密媒体类别**

[Files.vue:867-881](src/views/Files.vue) — 当 `getFileCategory` 返回 `'encrypted-video'` 或 `'encrypted-audio'` 时：
- 不匹配 `category === 'encrypted'`（L841），落入 `else` 分支
- `isMedia = category === 'video' || category === 'audio'` → **false**（因为 category 是 `'encrypted-video'` 不是 `'video'`）
- 按钮文本变成"预览"而非"播放"，点击后路由到 `/tabs/preview`
- 预览页检测到 `containerType === 'video'` 后重定向到 ArtPlayer（`/player`）

**路径 B：`FilePreview.vue` 硬编码重定向到 ArtPlayer**

[FilePreview.vue:296-305](src/views/FilePreview.vue) — 加密文件预览页检测到 `containerType === 'video'` 或 `'audio'` 时：
```typescript
case 'video':
case 'audio':
    router.push({ path: '/player', query: { path, name: fileName.value } })
```
直接跳转 ArtPlayer，完全忽略用户播放器偏好设置。

**修复**：
1. `handleLongPress` 中 `isMedia` 判断增加 `category.startsWith('encrypted-')` 前缀匹配
2. `FilePreview.vue` 中加密视频/音频改用 `openPlayer()` 走原生 MPV 播放器

### 问题 2：加载状态一直转圈圈

**根因链**：

1. `MpvPlayerActivity.onCreate()` L47: `engine.initialize()` 内部调用 `notifyState(State.MpvReady)`
2. L83: `engine.stateListener` 在 `initialize()` **之后**才设置 → `MpvReady` 事件丢失
3. `attachSurfaceView()` 永远不执行 → `MpvSurfaceView` 永远不创建 → `surfaceReady` 永远为 false
4. `MpvPlayerScreen.LaunchedEffect` 调用 `startPlayback()` → `engine.play(url)` → `surfaceReady=false` → `pendingUrl` 被设置但永远不被消费
5. `MpvPlayerScreen` 没有订阅 `engine.stateListener`，engine 异步状态变化无法传递到 Compose UI
6. 结果：Compose UI 停留在 `PlayerState.Loading`，永远转圈圈

**修复**：
- `MpvPlayerActivity.onCreate()` 中 `setContent` 后直接调用 `attachSurfaceView()`，不依赖 `stateListener` 回调
- `MpvPlayerScreen` 和 `MpvAudioPlayerScreen` 添加 `DisposableEffect(engine)` 订阅 `engine.stateListener`

---

## 实施步骤

### Step 1：Files.vue 第 317 行加密徽章修复

文件：`src/views/Files.vue`

```diff
- <ion-badge v-if="file.isEncrypted || getFileCategory(file.name, file.isEncrypted) === 'encrypted'" color="warning" slot="end">
+ <ion-badge v-if="file.isEncrypted || getFileCategory(file.name, file.isEncrypted).startsWith('encrypted')" color="warning" slot="end">
```

### Step 2：handleLongPress 识别加密媒体类别

文件：`src/views/Files.vue`

1. `else if (category === 'encrypted')` 改为 `else if (category.startsWith('encrypted-'))`，覆盖 `'encrypted'`、`'encrypted-video'`、`'encrypted-audio'`、`'encrypted-image'`
2. `else` 分支中 `isMedia` 判断增加加密媒体前缀匹配：

```typescript
} else {
    const isMedia = category === 'video' || category === 'audio' || category === 'encrypted-video' || category === 'encrypted-audio'
    buttons.push({
        text: isMedia ? t('files.play') : t('files.preview'),
        icon: isMedia ? videocam : image,
        handler: () => {
            if (isMedia) {
                const playCategory = category.startsWith('encrypted-') ? category.replace('encrypted-', '') : category
                playMedia(file, playCategory)
            } else {
                router.push({
                    path: '/tabs/preview',
                    query: { path: file.path, name: file.name, isEncrypted: String(!!file.isEncrypted) },
                })
            }
        },
    })
```

### Step 3：FilePreview.vue 加密视频/音频改用原生 MPV

文件：`src/views/FilePreview.vue`

将 `containerType === 'video' | 'audio'` 的硬编码 ArtPlayer 跳转改为调用 `openPlayer()`：

```typescript
case 'video':
case 'audio':
    if (isNative()) {
        const mimeType = containerType === 'video' ? 'video/*' : 'audio/*'
        openPlayer(path, fileName.value, mimeType)
    } else {
        router.push({ path: '/player', query: { path, name: fileName.value } })
    }
    loading.value = false
    return
```

需要 import `openPlayer` 和 `isNative` from `@/plugins/GoProcess`。

### Step 4：MpvPlayerActivity — Surface 直接挂载

文件：`plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerActivity.kt`

核心改动：
1. 删除 `stateListener` 回调方式挂载 Surface 的代码（L80-95）
2. 在 `setContent` 之后直接调用 `attachSurfaceView()`

```kotlin
// 直接挂载 Surface，不依赖 stateListener 回调
if (!audioMode) {
    val decorView = host.window?.decorView as? ViewGroup
    if (decorView != null) {
        val contentRoot = decorView.findViewById<ViewGroup>(android.R.id.content)
        if (contentRoot != null) {
            engine.attachSurfaceView(contentRoot)
        }
    }
}
```

### Step 5：MpvPlayerScreen — 订阅 engine.stateListener

文件：`plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerScreen.kt`

在现有 `DisposableEffect(Unit)` 之前添加：

```kotlin
DisposableEffect(engine) {
    val listener: (MpvEngine.State) -> Unit = { state ->
        when (state) {
            is MpvEngine.State.Playing -> playerState = PlayerState.Playing
            is MpvEngine.State.Paused -> playerState = PlayerState.Paused
            is MpvEngine.State.AudioOnly -> playerState = PlayerState.AudioOnly
            is MpvEngine.State.Ended -> playerState = PlayerState.Ended
            is MpvEngine.State.Error -> playerState = PlayerState.Error(classifyError(state.message), state.message)
            is MpvEngine.State.SurfaceReady -> { }
            is MpvEngine.State.WaitingSurface -> { }
            is MpvEngine.State.MpvReady -> { }
        }
    }
    engine.stateListener = listener
    onDispose {
        engine.stateListener = null
    }
}
```

同时修改 `startPlayback` 函数：删除 L298 的 `onStateChange(PlayerState.Loading)` 重复调用，状态由 `stateListener` 驱动。

### Step 6：MpvAudioPlayerScreen — 订阅 engine.stateListener

文件：`plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvAudioPlayerScreen.kt`

同样添加 `DisposableEffect(engine)` 订阅 `engine.stateListener`。

修改 `LaunchedEffect(filePath)` 中的播放逻辑：不再手动设置 `playerState = PlayerState.AudioOnly`，由 `stateListener` 驱动。

### Step 7：验证构建

```bash
cd /workspace/app/encv-mobile && ./gradlew :plugin-mpv-player:compileDebugKotlin
```
