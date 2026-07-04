# CI构建报错修复 + MPV播放器UI完善计划

## 问题分析（基于 job_logs.zip 实际日志）

### 问题1：CI构建失败 — `Unresolved reference 'PlayerOverlayManager'`（5处）

| 位置 | 行号 | 调用 |
|------|------|------|
| [GoProcessPlugin.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt) | L194 | `openPlayer()` 中 |
| [GoProcessPlugin.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt) | L208 | `closePlayer()` 中 |
| [MainActivity.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/MainActivity.kt) | L64 | `onDestroy()` 中 |
| [MainActivity.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/MainActivity.kt) | L73-74 | `onBackPressed()` 中 |

**根因**：`PlayerOverlayManager` 类从未创建。项目已重构为独立 Activity 方案，应使用已有的 [PlayerEntry.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerEntry.kt) 统一启动。

### 问题2：Files.vue 播放器模式值残留

Settings.vue 存储值为 `mpv-plugin`，但 Files.vue 的 `getPlayMode()` 只认旧值 `mpv`。导致选择MPV插件后永远 fallback 到 ArtPlayer。

### 问题3：MPV播放器UI功能缺失

MpvEngine 已实现以下底层能力但 **UI层未暴露**：
- ❌ 音量控制（`setVolume`/`getVolume` 已有）
- ❌ 字幕开关（`toggleSubtitleVisibility` 已有）
- ❌ 音轨切换（`cycleAudioTrack` 已有）
- ❌ 字幕轨切换（`cycleSubtitleTrack` 已有）
- ❌ 画面无额外操作按钮（只有倍速+全屏）

---

## 修复步骤

### Step 1: 修复 GoProcessPlugin.kt — PlayerEntry 替换 PlayerOverlayManager

**文件**: `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt`

**openPlayer()** (L177-201)：删除 `PlayerOverlayManager` + MainActivity 类型检查，改为直接调用 `PlayerEntry.play()`
**closePlayer()** (L204-214)：MPV 是独立 Activity，改为 no-op resolve

### Step 2: 修复 MainActivity.kt — 移除 PlayerOverlayManager 引用

**文件**: `app/encv-mobile/android/app/src/main/java/com/encvgo/app/MainActivity.kt`

- **onDestroy()** (L64)：删除 `PlayerOverlayManager.getInstance().hideOverlay()`
- **onBackPressed()** (L73-74)：删除 `PlayerOverlayManager` 判断块，MPV独立Activity不需要拦截返回键

### Step 3: 修复 Files.vue — mpv → mpv-plugin 值对齐

**文件**: `app/encv-mobile/src/views/Files.vue`

- `getPlayMode()` (L170-175)：校验列表加入 `'mpv-plugin'`，默认值 `'mpv'` 改为 `'mpv-plugin'`
- `playMedia()` switch (L183-203)：`case 'mpv':` → `case 'mpv-plugin':`

### Step 4: 完善MPV播放器UI — MpvControls.kt 增强

**文件**: `app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvControls.kt`

在 `VideoPlaybackLayout` 的 BottomBar 区域增加：

**4a. 音量控制**
- 底部栏新增音量 Slider 或 IconButton（静音/取消静音 + 滑块）
- 回调: `onVolumeChange: (Float) -> Unit`
- 使用 MpvEngine 的 `setVolume`/`getVolume`

**4b. 字幕+音轨按钮**
- 底部栏新增字幕图标按钮（toggle subtitle visibility）
- 底部栏新增音轨图标按钮（cycle audio track）
- 回调: `onToggleSubtitle: () -> Unit`, `onCycleAudio: () -> Unit`

**4c. MpvControls composable 签名扩展**
```kotlin
fun MpvControls(
    ...existing params...,
    volume: Float = 1f,
    onVolumeChange: (Float) -> Unit,
    onToggleSubtitle: () -> Unit,
    onCycleAudio: () -> Unit
)
```

**4d. VideoPlaybackLayout BottomBar 扩展**
在 SpeedChip 和 FullscreenButton 之间添加：音量滑块行 + 字幕/音轨按钮行

**4e. MpvPlayerScreen.kt 接线**
将新回调连接到 MpvEngine 对应方法

### Step 5: 前端构建验证
```bash
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit && npm run build
```

### Step 6: 清理日志
- 删除 `job_logs_extracted/`
- 删除 `job_logs.zip`
- 删除 `.trae/documents/job_logs.zip`

---

## 变更文件清单

| # | 文件 | 变更 |
|---|------|------|
| 1 | `android/.../GoProcessPlugin.kt` | openPlayer→PlayerEntry.play；closePlayer→no-op |
| 2 | `android/.../MainActivity.kt` | 移除5处PlayerOverlayManager引用 |
| 3 | `src/views/Files.vue` | mpv→mpv-plugin值对齐 |
| 4 | `plugin-mpv-player/.../MpvControls.kt` | 新增音量/字幕/音轨UI控件 |
| 5 | `plugin-mpv-player/.../MpvPlayerScreen.kt` | 接线新回调到MpvEngine |
