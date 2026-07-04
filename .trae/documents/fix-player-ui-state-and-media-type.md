# 修复播放器 UI 状态 + 区分视频/音频控件 + 弹幕架构规划

## 问题分析

### 问题 1："正在初始化视频窗口..." 永远覆盖视频上层 + 没有播放控件

**根因**：`setErrorMessage("正在初始化视频窗口...")` 把 loading 状态当成了 error。`PlayerControls` 中 `error` 检查优先于 `state` 检查（第 49 行 `if (error)` 先于 `if (state === 'loading')`），所以一旦 `errorMessage` 被设置，UI 就永远显示错误界面。后续 `"playing"` 事件虽然更新了 `playerState`，但 `errorMessage` 从未被清除。

### 问题 2：需要区分视频/音频显示不同控件

当前 `initData` 包含 `mimeType` 但未利用。需要根据媒体类型显示不同布局。

### 问题 3：60fps 密集弹幕可行性

**结论：完全可行，但必须用 mpv 内置的 libass 渲染，不能用 Lynx/JS 渲染弹幕。**

技术依据：
- mpv 深度集成 libass，原生支持 ASS 字幕格式，包括 `\move()` 移动动画、`\t()` 渐变、`\fad()` 淡入淡出等弹幕所需的全部特效
- mpv-android 已支持 libass 渲染样式化字幕（README 明确列出 "libass support for styled subtitles"）
- libass 渲染在 mpv 的 GPU 管线内完成，与视频帧合成，不经过 JS 线程，零额外延迟
- 抖音/B站等弹幕方案也是原生渲染（Canvas/TextureView），不走 JS 层

**弹幕架构方案**：
1. 后端将弹幕数据转换为 ASS 字幕格式（.ass 文件或实时注入）
2. mpv 通过 `sub-file` 选项加载 ASS 字幕
3. 弹幕渲染完全由 libass + mpv GPU 管线完成，60fps 无压力
4. Lynx 层只负责控件 UI（播放/暂停/进度条），不参与弹幕渲染

**实时弹幕注入**：mpv 提供 `sub-add` 命令和 `observe_property("secondary-sub-text")` 等接口，可以通过 `MPVLib.command()` 动态添加字幕轨道。对于实时弹幕，后端可以将弹幕流实时转为 ASS 事件并通过 mpv 的 `sub-file` 管道注入。

## 修复方案

### Step 1：修复 JS 端事件处理逻辑（核心修复）

**文件**：`AppComponent.tsx`

关键修改：
- `"waiting_surface"` 事件：只设置 `playerState("loading")`，**不设置 errorMessage**
- `"surface_ready"` 事件：清除 errorMessage
- `"playing"` / `"paused"` 事件：清除 errorMessage
- `startPlayback()` 中：`setPlayerState("loading")` 时也清除 errorMessage

### Step 2：修复 PlayerControls loading 状态显示

**文件**：`PlayerControls.tsx`

loading 状态添加 "正在加载..." 文字提示，确保 error 和 loading 互斥。

### Step 3：区分视频/音频，传递 mediaType

**文件**：`PlayerActivityLynx.kt` — `buildInitDataJson()` 添加 `mediaType` 字段
**文件**：`MpvPlayerModule.kt` — `MPV_EVENT_FILE_LOADED` 中检测 video width/height，纯音频时隐藏 SurfaceView 并派发 `"audio_only"` 事件

### Step 4：前端根据 mediaType 显示不同控件

**文件**：`AppComponent.tsx` — 添加 `mediaType` 状态
**文件**：`PlayerControls.tsx` — 拆分 VideoControls 和 AudioControls

**VideoControls**（现有布局）：
```
┌─────────────────────┐
│  ✕  文件名      ⤢   │  ← TopBar（返回+标题+全屏）
│       ⏸             │  ← CenterArea（播放/暂停）
│  0:00 ━━━━━━ 3:45   │  ← BottomBar（进度条）
└─────────────────────┘
```

**AudioControls**（新增）：
```
┌─────────────────────┐
│  ✕  文件名          │  ← TopBar（返回+标题，无全屏）
│    ┌───────────┐    │
│    │  🎵       │    │  ← 居中封面占位
│    └───────────┘    │
│  0:00 ━━━━━━ 3:45   │  ← 进度条
│       ⏸             │  ← 播放/暂停
└─────────────────────┘
```

### Step 5：CSS 样式更新

**文件**：`App.css` — 添加音频模式样式（AudioCoverContainer、AudioCover 等）

### Step 6：重建 Lynx bundle + 同步到 Android assets

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `AppComponent.tsx` | 修复事件处理 + 添加 mediaType 状态 |
| `PlayerControls.tsx` | 拆分 VideoControls/AudioControls + 修复 loading |
| `App.css` | 添加音频模式样式 |
| `PlayerActivityLynx.kt` | buildInitDataJson 添加 mediaType |
| `MpvPlayerModule.kt` | audio_only 事件 + 隐藏 SurfaceView |
