# 修复 Lynx 播放器黑屏 — React 组件未正确渲染

## 最新 logcat 分析

### 好消息 ✅
- `setScreenSize(1260, 2800)` **生效了**
- `UpdateViewport :size: 1260.0, 2800.0; mode: 1, 1` — **视口尺寸正确**
- 所有回调成功：`onPageStart` → `onLoadSuccess` → `onFirstScreen` → `onRuntimeReady`
- 无 JS 错误、无 onLoadFailed
- Bundle 加载正常（84912 bytes）
- `TemplateAssembler::OnScreenMetricsSet width: 1260 height: 2800`

### 🔴 关键发现：JS console 输出完全缺失

App.tsx 中有这些日志代码：
```tsx
console.info("mpv:state-change", JSON.stringify(event));  // 第27行
console.info("Backend ready, port:", event?.port);        // 第43行
```

**logcat 中完全没有这些输出！** 这意味着：
1. `useLynxGlobalEventListener` 回调从未触发 → 说明全局事件没发出来（因为还没到播放阶段）— 这个可以理解
2. 但更关键的是：**React 组件的 useEffect 可能执行时崩溃了**

### 根因假设：useEffect 中立即调用 Native Module 导致 React 组件挂载失败

```tsx
// App.tsx 第46-51行
useEffect(() => {
    if (initData) {
      setFileName(initData.fileName || "Unknown");
      startPlayback(initData);  // ← ⚠️ 立即调用 Native Modules！
    }
}, [initData]);
```

`startPlayback()` 内部会同步/异步调用多个 Native Module：
1. `GoBackendModule.getBackendStatus()`
2. `GoBackendModule.startBackend()`
3. `GoBackendModule.getStreamUrl()`
4. `MpvPlayerModule.play()`

**如果在 React 组件首次渲染时就调用这些 Native Module，而此时 LynxContext 或 Module 还没有完全初始化好，可能会抛出异常。** 异常可能导致 React 组件树渲染失败（渲染为空），但 Lynx 模板层面仍然报告 onFirstScreen/onLoadSuccess（因为它只关心模板解析，不关心 React 组件状态）。

## 修复方案

### Step 1：让初始 UI 为纯静态展示，不依赖 Native Module

修改 App.tsx，移除 useEffect 中的自动播放逻辑，改为：

```tsx
// 删除自动 startPlayback 的 useEffect
// 改为只在用户点击播放按钮时才触发 Native Module 调用

const handlePlayPause = useCallback(() => {
    if (playerState === "playing") {
      NativeModules.MpvPlayerModule.pause(() => {});
      setPlayerState("paused");
    } else if (playerState === "paused") {
      NativeModules.MpvPlayerModule.resume(() => {});
      setPlayerState("playing");
    } else {
      // idle → 开始播放（首次点击时才调用 Native Modules）
      startPlayback(currentInitData);
    }
}, [playerState]);
```

同时保留 initData 设置文件名的逻辑（这是纯本地操作，不涉及 Native Module）。

### Step 2：给 LynxView 设置可见背景色用于调试

在 PlayerActivityLynx.kt 中恢复调试背景色（确认 LynxView 本身是否可见）：

```kotlin
lynxView?.setBackgroundColor(android.graphics.Color.parseColor("#CC0010"))
```

### Step 3：在 PlayerControls idle 状态下确保有可见元素

当前 idle 状态应该显示居中的 ▶ 按钮 + 文件名。确认 PlayerControls.tsx 的 idle 分支正确。

## 预期效果

- Step 1 后，组件挂载时不调用任何 Native Module，React 组件树能正常渲染
- 用户应该能看到深蓝色背景 (#001030) + 居中的播放按钮 + 文件名
- 点击播放按钮后才开始与 Native Layer 交互
- 如果能看到 UI，说明问题确实是 Native Module 在初始化阶段调用导致的
