# 修复计划：MPV 播放器 + ArtPlayer 布局 + 全屏退出

## 问题 1：退出全屏没有恢复屏幕方向

### 根因

`MpvPlayerModule.setFullscreen(false)` 设置 `SCREEN_ORIENTATION_SENSOR_PORTRAIT`，但这是**强制竖屏**，不是恢复到自由旋转。对于 overlay 模式（在 MainActivity 上层），退出全屏后应该恢复 `SCREEN_ORIENTATION_UNSPECIFIED`（跟随系统），否则用户在非全屏时也无法旋转手机。

### 修复

`MpvPlayerModule.kt` 第 349 行：
```kotlin
// 退出全屏：恢复自由旋转
act.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
```

---

## 问题 2：MPV 播放器多个问题

### 2A：视频正常播放（能听到声音）但被遮挡不可见

### 根因

`PlayerOverlayManager.createLynxView()` 中，overlay 布局的视图层级是：
1. `overlayLayout`（FrameLayout，MATCH_PARENT，黑色背景）
2. `MpvSurfaceView`（添加到 overlayLayout，index 0）
3. `LynxView`（添加到 overlayLayout，MATCH_PARENT）

**问题**：LynxView 的背景设为透明（`setBackgroundColor(0)`），但 LynxView 内部的 `PlayerContainer` CSS 设置了 `background-color: #000000`（黑色不透明），完全遮住了底层的 MpvSurfaceView。

**修复**：将 `PlayerContainer` 的背景改为透明，让 MPV SurfaceView 透过 LynxView 可见：
```css
.PlayerContainer {
  background-color: transparent;  /* 让 MPV SurfaceView 透过 */
}
```

### 2B：点击进度条位置无法正确跳转

### 根因

`ProgressBar.tsx` 的 `handleTrackTap` 使用 `e.detail?.clientX ?? e.clientX ?? 0` 获取点击坐标。在 Lynx 中，触摸事件的坐标在 `e.detail` 中，但字段名可能不是 `clientX`。需要检查 Lynx 事件对象的结构。

Lynx 的 `bindtap` 事件对象结构是 `e.detail` 包含 `{ clientX, clientY, pageX, pageY }` 等。但 `getBoundingClientRect` 在 Lynx 中可能不可用或返回不准确的值。

**修复**：使用 `e.detail.screenX` 或 `e.detail.pageX` 作为备选，并添加 `getBoundingClientRect` 不可用时的 fallback（使用 `trackEl.offsetWidth` 和事件坐标计算）：

```typescript
const handleTrackTap = useCallback(
  (e: any) => {
    const trackEl = trackRef.current
    if (!trackEl) return
    const tapX = e.detail?.clientX ?? e.detail?.pageX ?? e.clientX ?? 0
    const rect = trackEl.getBoundingClientRect?.()
    let ratio: number
    if (rect && rect.width > 0) {
      ratio = Math.max(0, Math.min(1, (tapX - rect.left) / rect.width))
    } else {
      const trackWidth = trackEl.offsetWidth ?? 1
      const tapLocalX = e.detail?.localX ?? tapX
      ratio = Math.max(0, Math.min(1, tapLocalX / trackWidth))
    }
    onSeek(ratio)
  },
  [onSeek]
)
```

### 2C：前进后退 10 秒按钮会卡一秒左右

### 根因

`handleSeekRelative` 调用 `NativeModules.MpvPlayerModule.seekTo(newPos, () => {})`，然后立即 `setPosition(newPos)` 乐观更新位置。但 MPV 的 seek 操作本身需要时间（特别是精确 seek），在 seek 完成前 MPV 不会更新画面。

问题在于 seek 使用的是 `absolute` 模式（`MPVLib.command(arrayOf("seek", positionSec.toString(), "absolute"))`），这是精确 seek，会卡顿。应该使用 `absolute+keyframes` 模式来快速跳转：

```kotlin
MPVLib.command(arrayOf("seek", positionSec.toString(), "absolute+keyframes"))
```

### 2D：播放异常卡在 loading 状态

### 根因

`PlayerControls.tsx` 第 48 行：`const isError = error && state !== 'loading'`，当 state 是 `loading` 时即使有 error 也不显示错误界面。但更根本的问题是：

1. `mpv:state-change` 事件中，`error` 字段可能和 `state` 字段同时存在，但代码先检查 `state` 再检查 `error`，某些错误状态可能被忽略
2. 没有超时机制：如果 MPV 一直不发送状态变化，UI 会永远卡在 loading
3. `audio_only` 的判断过于激进：`videoWidth == 0 || videoHeight == 0` 在视频元数据还没加载完时就会触发

### 修复

**PlayerApp.tsx**：
1. 添加 loading 超时机制（15 秒后如果还在 loading 则显示错误）
2. 修改 `mpv:state-change` 处理逻辑，确保 error 字段优先处理

**MpvPlayerModule.kt**：
3. `MPV_EVENT_FILE_LOADED` 中增加延迟检查视频尺寸，避免元数据未就绪时误判为 audio_only

**PlayerControls.tsx**：
4. loading 状态也应该显示取消/返回按钮，不能让用户卡死

### 2E：音频检测容易误触发

### 根因

`MpvPlayerModule.kt` 第 88-91 行：
```kotlin
val videoWidth = try { MPVLib.getPropertyInt("width") ?: 0 } catch (_: Exception) { 0 }
val videoHeight = try { MPVLib.getPropertyInt("height") ?: 0 } catch (_: Exception) { 0 }
val isAudioOnly = videoWidth == 0 || videoHeight == 0
```

`MPV_EVENT_FILE_LOADED` 时，视频的 width/height 属性可能还未就绪（返回 0），导致视频被误判为纯音频。

**修复**：添加延迟二次检查，在 FILE_LOADED 后延迟 500ms 再次检查视频尺寸：

```kotlin
MpvEvent.MPV_EVENT_FILE_LOADED -> {
    mainHandler.postDelayed({
        val videoWidth = try { MPVLib.getPropertyInt("width") ?: 0 } catch (_: Exception) { 0 }
        val videoHeight = try { MPVLib.getPropertyInt("height") ?: 0 } catch (_: Exception) { 0 }
        val isAudioOnly = videoWidth == 0 || videoHeight == 0
        if (isAudioOnly) {
            mainHandler.post { mpvSurfaceView?.visibility = View.GONE }
            dispatchStateChange("audio_only")
        } else {
            mainHandler.post { mpvSurfaceView?.visibility = View.VISIBLE }
            dispatchStateChange("playing")
        }
    }, 500)
}
```

---

## 问题 3：ArtPlayer 非全屏播放高度被限制死

### 根因

`ArtPlayerView.vue` 第 182-183 行：
```javascript
const containerHeight = Math.round(containerWidth * 9 / 16)
artContainer.value.style.height = `${containerHeight}px`
```

硬编码 16:9 比例计算高度，竖屏视频（9:16）时高度只有宽度的 56.25%，导致控件显示不全。

### 修复

1. 不预设固定高度，让 ArtPlayer 的 `autoSize` 功能根据视频实际比例自适应
2. 设置 `max-height` 限制不超出可视范围
3. 在 `video:loadedmetadata` 事件中根据视频实际比例调整容器高度

```javascript
// 初始化时不设固定高度，设最小高度避免闪烁
artContainer.value.style.minHeight = '200px'
artContainer.value.style.maxHeight = `${window.innerHeight - 56}px` // 减去 header 高度

// 在 loadedmetadata 中根据视频比例调整
art.on('video:loadedmetadata', () => {
  const video = art?.video
  if (video && video.videoWidth && video.videoHeight) {
    const ratio = video.videoHeight / video.videoWidth
    const containerWidth = artContainer.value?.clientWidth || window.innerWidth
    const naturalHeight = Math.round(containerWidth * ratio)
    const maxHeight = window.innerHeight - 56
    const finalHeight = Math.min(naturalHeight, maxHeight)
    if (artContainer.value) {
      artContainer.value.style.height = `${finalHeight}px`
    }
  }
})
```

---

## 修改文件清单

| 文件 | 修改内容 |
|------|----------|
| `MpvPlayerModule.kt` | 退出全屏恢复 UNSPECIFIED；seek 改用 keyframes；FILE_LOADED 延迟检查视频尺寸 |
| `PlayerApp.tsx` | 添加 loading 超时；优化 mpv:state-change 错误处理优先级 |
| `PlayerControls.tsx` | loading 状态添加返回按钮；修复 isError 判断 |
| `ProgressBar.tsx` | 修复 Lynx 事件坐标获取，添加 fallback |
| `player.css` | PlayerContainer 背景改为透明 |
| `ArtPlayerView.vue` | 移除硬编码 16:9 高度，改为自适应 + maxHeight 限制 |
