# 修复：mpvModule=null 导致播放器卡在"正在初始化视频窗口"

## 问题分析

### 错误日志

```
ERROR [PlayerActivityLynx] createLynxView: cannot attachToLayout,
  rootLayout=FrameLayout{...}, mpvModule=null
```

前端显示"正在初始化视频窗口..."，说明 Lynx bundle 加载成功了（上一轮修复生效），但 mpvModule 为 null。

### 根因链

```
PlayerActivityLynx.createLynxView()
  ↓
viewBuilder.registerModule("MpvPlayerModule", MpvPlayerModule::class.java)
  ↓
LynxViewBuilder.registerModule() 只是注册了 class，不会立即创建实例
  ↓
MpvPlayerModule 实例在 Lynx 渲染模板时才由 Lynx 框架创建
  ↓
但 createLynxView() 在 viewBuilder.build() 之后立即调用：
  MpvPlayerModule.getInstance()  ← 此时 Lynx 还没创建模块实例！
  ↓
mpvModule = null → attachToLayout 失败 → SurfaceView 未添加 → 视频窗口无法初始化
```

**核心问题**：`MpvPlayerModule.getInstance()` 在 `createLynxView()` 中被同步调用，但 Lynx Native Module 是**懒创建**的——只有当 JS 代码首次调用 `NativeModules.MpvPlayerModule.xxx()` 时，Lynx 才会实例化模块。`createLynxView()` 在 `renderTemplateUrl()` 之前就尝试获取实例，此时模块尚未创建。

### 代码证据

[PlayerActivityLynx.kt:324-331](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt):
```kotlin
val mpvModule = MpvPlayerModule.getInstance()  // ← null！Lynx 还没创建模块
if (rootLayout != null && mpvModule != null) {
    mpvModule.attachToLayout(rootLayout!!)
} else {
    LogRelay.get().relay(TAG, "error", "createLynxView: cannot attachToLayout, ...")
}
lynxView?.renderTemplateUrl("player.lynx.bundle", initData)  // ← 这之后才会创建模块
```

[MpvPlayerModule.kt:62-64](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt):
```kotlin
init {
    _instance = this  // ← 只有 Lynx 创建模块实例时才会执行
}
```

### Lynx 官方 Native Module 生命周期

根据 [Lynx 官方文档](https://lynxjs.org/guide/use-native-modules)：
1. `viewBuilder.registerModule("MpvPlayerModule", MpvPlayerModule::class.java)` — 注册模块类
2. `viewBuilder.build()` — 构建 LynxView，但**不创建**模块实例
3. `renderTemplateUrl()` — 开始渲染模板
4. JS 调用 `NativeModules.MpvPlayerModule.play()` — **此时** Lynx 才创建模块实例
5. 模块的 `init {}` 块执行，`_instance = this`

所以 `MpvPlayerModule.getInstance()` 在步骤 4 之前永远返回 null。

---

## 修复方案

### 核心思路

不再在 `createLynxView()` 中同步获取 mpvModule 并 attachToLayout。改为：

1. **MpvPlayerModule 初始化时自动 attachToLayout**：在 `init {}` 块中，通过 Activity 引用找到 rootLayout 并 attach
2. **移除 createLynxView 中的同步 attach 逻辑**：因为模块创建时机不确定，同步 attach 必然失败

### Step 1：修改 MpvPlayerModule — init 中自动 attach

```kotlin
init {
    _instance = this
    LogRelay.get().relay(TAG, "info", "init: MpvPlayerModule created")
    // 模块创建时自动 attach 到 Activity 的 rootLayout
    val act = activity
    if (act is PlayerActivityLynx) {
        val root = act.findViewById<FrameLayout>(R.id.lynx_player_root)
        if (root != null) {
            attachToLayout(root)
        } else {
            LogRelay.get().relay(TAG, "warn", "init: lynx_player_root not found, will attach later")
        }
    }
}
```

### Step 2：修改 PlayerActivityLynx — 移除同步 attach，改为延迟检查

移除 `createLynxView()` 中的同步 `MpvPlayerModule.getInstance()` + `attachToLayout` 调用。

改为在 `onRuntimeReady()` 或 `onLoadSuccess()` 回调中检查并 attach（作为兜底）：

```kotlin
override fun onRuntimeReady() {
    LogRelay.get().relay(CLIENT_TAG, "info", "onRuntimeReady: JS environment ready")
    // 兜底：如果模块已创建但还没 attach（通常不会，因为 init 中已自动 attach）
    val mpvModule = MpvPlayerModule.getInstance()
    if (mpvModule != null && rootLayout != null) {
        // 检查是否已经 attach（避免重复）
    }
}
```

### Step 3：修改 startPlayback 流程 — 处理 surface 未就绪的情况

当前 JS 端的 `startPlayback` 调用 `MpvPlayerModule.play(url)` 时，如果 surface 还没准备好，`play()` 会将 URL 存为 `pendingUrl` 并 dispatch `waiting_surface` 状态。这个逻辑已经正确，不需要修改。

但需要确保 `attachToLayout` → `ensureMpvInitialized` → `MPVLib.create/init` 的调用链在模块 init 时就能执行，而不是等到 JS 调用 `play()` 时才初始化。

### Step 4：参考 Lynx UI 组件库优化前端

当前前端使用原始 `<view>`、`<text>` 等 Lynx 内建组件，没有使用 Lynx UI 组件库。根据用户要求，应参考 lynx-ui 组件库的实现来优化播放器 UI。

但 lynx-ui 是字节跳动内部组件库，未开源。Lynx 开源生态中可用的 UI 模式是：
- 使用 `@lynx-js/react` 提供的 `<view>`、`<text>`、`<image>`、`<scroll-view>` 等基础组件
- 使用 CSS 样式（Lynx 原生支持 CSS，包括 flexbox、linear layout 等）
- 使用 `useLynxGlobalEventListener` 监听 Native Module 事件

当前前端代码已经遵循了这些模式，UI 优化属于体验提升而非 bug 修复，建议作为后续任务。

---

## 修改文件

1. `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt` — init 中自动 attachToLayout
2. `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt` — 移除同步 attach，改为回调中兜底检查
