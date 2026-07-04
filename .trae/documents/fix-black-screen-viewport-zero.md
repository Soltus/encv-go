# 修复 Lynx 播放器黑屏 — 视口尺寸为 0（使用 setScreenSize 方案）

## 根因确认

### logcat 关键证据
```
[Layout] UpdateViewport :size: 0.0, 0.0; mode: 0, 0   ← 🔴 视口 = 0×0
```

### 完整成功链路（全部成功，但内容不可见）
- `onPageStart` ✅ → bundle 加载 84909 bytes ✅ → `onLoadSuccess` ✅ → `onRuntimeReady` ✅ → `onFirstScreen` ✅
- 无任何 JS 错误、无 onLoadFailed、无 onReceivedJSError

### 为什么视口是 0×0
`addView()` 后立即调用 `renderTemplateUrl()`，此时 Android 还未对 LynxView 做 measure/layout，Lynx 读到的画布为 0×0。

## 修复方案：使用 `setScreenSize()` 预设视口（符合 IFR）

LynxViewBuilder 提供两个 API 可在 build 前预设尺寸：

| API | 用途 |
|-----|------|
| `viewBuilder.setScreenSize(widthPx, heightPx)` | 设置屏幕逻辑像素尺寸 |
| `viewBuilder.setPresetMeasuredSpec(widthSpec, heightSpec)` | 预设 MeasureSpec |

**使用 `setScreenSize()` 是官方推荐方案**，文档说明：
> "When the given screen size is not set, Lynx will be initialized with screen metrics from activity context or real screen metrics."

但在我们的场景中，从 context 获取屏幕指标可能失败（因为 LynxView 还没 attach 到 window），所以需要显式设置。

## 修复步骤

### Step 1：在 viewBuilder.build() 之前调用 setScreenSize()

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt`

在 `createLynxView()` 中，`viewBuilder.registerModule()` 之后、`viewBuilder.build(this)` 之前添加：

```kotlin
val displayMetrics = resources.displayMetrics
val screenWidth = displayMetrics.widthPixels
val screenHeight = displayMetrics.heightPixels
Log.d(TAG, "createLynxView: screen size ${screenWidth}x${screenHeight}")
viewBuilder.setScreenSize(screenWidth, screenHeight)
Log.d(TAG, "createLynxView: screenSize set")
```

这样 Lynx 在初始化时就知道了正确的屏幕尺寸，IFR 可以正常工作，不需要延迟渲染。

### Step 2：保留 `<page style={{ flex: 1 }}>` 作为兜底防御

**文件**：`lynx-player/src/App.tsx`

保持上一次的修改不变：
```tsx
<page style={{ flex: 1 }}>
```

### Step 3：保留 PlayerContainer 调试背景色验证渲染

**文件**：`lynx-player/src/App.css`

```css
.PlayerContainer {
  background-color: #001030;  /* 深蓝色便于区分 */
}
```

## 预期效果

- `setScreenSize()` 让 Lynx 在 JS 引擎启动前就知道正确的视口尺寸
- `[Layout] UpdateViewport` 日志应显示正确的屏幕尺寸
- 所有基于 flex 的元素正确计算尺寸
- 用户能看到播放器 UI
- 不影响 IFR 性能（尺寸在 build 阶段就确定了）
