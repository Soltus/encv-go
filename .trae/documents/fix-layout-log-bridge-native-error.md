# 修复布局 + 日志输出 + 播放报错

## 现状
- ✅ **`root.render()` 生效！UI 渲染出来了！**
- ❌ 布局：蓝色区域只在顶部（`<page>` 的 flex: 1 对根元素无效）
- ❌ 点击播放报错：Native Module 调用失败
- ❌ 前端 JS console.log 不可见
- ❌ 后端 Kotlin Log.d/e 在主应用 devlogs 中不可见

## 问题分析与修复

### 问题 1：布局未全屏

**原因**：`<page>` 是根元素没有 flex 父容器，`flex: 1` 无效。

**修复**：`<page style={{ width: '100%', height: '100%' }}>` + PlayerContainer 也设为全屏尺寸

### 问题 2 & 3 & 4：日志系统打通

#### 2A：前端日志 — 创建 LogBridge Native Module

新建 `LogBridgeModule.kt`，提供一个 `log(level, msg)` 方法，将 JS 层的日志通过 `Log.d/e/w()` 输出到 Android logcat：

```kotlin
@LynxMethod
fun log(level: String, msg: String, callback: Callback) {
    when (level) {
        "info" -> Log.i(TAG, msg)
        "error" -> Log.e(TAG, msg)
        "warn" -> Log.w(TAG, msg)
        else -> Log.d(TAG, msg)
    }
    callback.onSuccess(null)
}
```

在 JS 端创建一个简单的 logger 包装：
```tsx
const lynxLog = {
    info: (msg: string) => { console.info(msg); NativeModules.LogBridge.log('info', String(msg), () => {}); },
    error: (msg: string) => { console.error(msg); NativeModules.LogBridge.log('error', String(msg), () => {}); },
};
```

这样 JS 的 `lynxLog.info()` 会同时输出到：
- Lynx 内置 console（Lynx DevTool 可见）
- Android logcat（tag: `LogBridge`，HjqLogCat / adb logcat 可见）

#### 2B：后端日志 — 确认 TAG 可被捕获

当前后端使用的 TAG：
- `PlayerActivityLynx`
- `MpvPlayerModule`
- `GoBackendModule`
- `PlayerTemplateProvider`
- `LynxPlayerClient`

这些应该都在 logcat 中。如果 HjqLogCat 不显示，可能需要确认其过滤规则。作为兜底，也可以让关键日志通过 LogBridge 输出（但没必要，logcat 本身就应该能捕获）。

### 问题 3：播放报错 — 分步调试

在 `startPlayback()` 中每个 Native Module 调用前后加 `lynxLog`，精确定位失败点：

```tsx
const startPlayback = useCallback(async (data: InitData | undefined) => {
    if (!data) return;
    setPlayerState("loading");
    try {
      lynxLog.info("startPlayback: step1 getBackendStatus");
      const status = await new Promise<any>((resolve) => {
        NativeModules.GoBackendModule.getBackendStatus(resolve);
      });
      lynxLog.info("startPlayback: step1 result=" + JSON.stringify(status));
      
      // ... 每步都加日志 ...
    } catch (e: any) {
      lynxLog.error("startPlayback error: " + (e?.message || String(e)));
      setPlayerState("error");
      setErrorMessage(String(e?.message || e));
    }
}, []);
```

## 修复步骤

### Step 1：修复 layout（page 尺寸 + PlayerContainer 全屏）

**文件**：
- `lynx-player/src/components/AppComponent.tsx` — page 改为 width/height 100%
- `lynx-player/src/App.css` — PlayerContainer 加 width/height 100%，默认居中

### Step 2：创建 LogBridgeModule.kt

**新文件**：`android-overlay/app/src/main/java/com/encvgo/app/LogBridgeModule.kt`

简单的 Native Module，注册名 `"LogBridge"`，方法 `log(level, msg)` → `Log.d/e/w/i(TAG, msg)`

### Step 3：注册 LogBridgeModule 到 LynxViewBuilder

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt`

添加 `viewBuilder.registerModule("LogBridge", LogBridgeModule::class.java)`

### Step 4：JS 端 logger 包装 + startPlayback 分步日志

**文件**：
- `lynx-player/src/components/AppComponent.tsx` — 添加 lynxLog 工具 + startPlayback 分步日志
- `lynx-player/src/typing.d.ts` — 添加 LogBridgeModule 类型声明

### Step 5：移除调试背景色

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt` — 删除 setBackgroundColor
**文件**：`lynx-player/src/App.css` — background-color 改回 #000000

## 预期效果

- UI 全屏显示，居中的播放按钮和文件名
- 点击播放后能在 logcat 中看到精确到每一步的日志输出
- 前端和后端日志统一输出到 logcat，主应用 devlogs 能看到
- 根据日志定位具体是哪个 Native Module 调用失败并针对性修复
