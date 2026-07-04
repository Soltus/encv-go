# 播放器覆盖层方案：在 Capacitor 主页面内嵌入 MPV + Lynx 播放器

## 背景

用户要求：主流播放器没有 Activity 跳转，主应用是 Capacitor 界面，应该做到附上 MPV 播放器。

当前架构：`GoProcess.openInPlayer()` → 启动 `PlayerActivity` → 跳转到 `PlayerActivityLynx`（独立 Activity）。

目标架构：`GoProcess.openPlayer()` → 在 `MainActivity` 内动态叠加播放器覆盖层（MpvSurfaceView + LynxView），不跳转 Activity。

## 架构设计

```
MainActivity (Capacitor BridgeActivity)
├── WebView (Capacitor)
└── PlayerOverlay (FrameLayout, 动态添加/移除)
    ├── MpvSurfaceView (index 0, 视频渲染层)
    └── LynxView (index 1, UI 控件层, 透明背景)
```

### 关键原则

1. **MpvSurfaceView 在底层**（先 addView），LynxView 在上层（后 addView），LynxView 背景透明
2. **PlayerOverlayManager** 管理覆盖层的创建/销毁，持有 MpvPlayerModule 和 LynxView 的引用
3. **GoProcessPlugin** 新增 `openPlayer()`/`closePlayer()` 方法，委托给 PlayerOverlayManager
4. **前端** 调用 `GoProcess.openPlayer()` 替代 `GoProcess.openInPlayer()`
5. **关闭播放器** 由 Lynx JS 端调用 `GoBackendModule.closePlayer()` → PlayerOverlayManager.removeOverlay()

## 实施步骤

### 步骤 1：完成 ReactLynx 播放器 JS 端

**1.1 创建 `ProgressBar.tsx`**

从 Vue 版 `ProgressBar.vue` 移植为 React 组件：
- `useRef` 替代 `ref`
- `bindtap` 替代 `@tap`
- `props` 替代 `defineProps`/`defineEmits`
- `handleTrackTap` 通过 `e.detail.clientX` 计算点击位置比例
- `clampedProgress` 限制 0-1 范围

文件：`/workspace/app/encv-mobile/lynx-player/src/player/ProgressBar.tsx`

**1.2 创建 `player.css`**

从 Vue 版 `PlayerControls.vue` 和 `ProgressBar.vue` 的 `<style scoped>` 合并移植。
所有 CSS 类已在 Vue 版中验证过，使用 `display: flex`，配合 `defaultDisplayLinear: false` 配置。

文件：`/workspace/app/encv-mobile/lynx-player/src/player/player.css`

**1.3 修改 `PlayerApp.tsx` 中 `closePlayer` 调用**

当前 `handleBack` 调用 `GoBackendModule.closePlayer()`，但 `GoBackendModule` 中没有 `closePlayer` 方法。
需要新增 `@LynxMethod closePlayer()`，该方法通过回调通知 Android 端移除覆盖层。

### 步骤 2：构建验证 JS 端

```bash
cd lynx-player && npx rspeedy build
```

确认 `dist/player.lynx.bundle` 生成成功。

### 步骤 3：创建 PlayerOverlayManager

新建 `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerOverlayManager.kt`

职责：
- `showOverlay(activity: MainActivity, filePath, fileName, mimeType, isExternal)` — 创建 FrameLayout 覆盖层，添加 MpvSurfaceView + LynxView
- `hideOverlay()` — 移除覆盖层，释放 MPV/Lynx 资源
- 内部复用 `PlayerActivityLynx` 中的逻辑：
  - `MpvPlayerModule.preInit()`
  - `LynxViewBuilder` 构建 LynxView，注册 MpvPlayerModule/GoBackendModule/LogBridgeModule
  - `PlayerTemplateProvider` 加载 bundle
  - `buildInitDataJson()` 构建初始化数据
  - `positionUpdateRunnable` 定时更新进度

关键差异（vs PlayerActivityLynx）：
- 不使用 `setContentView()`，而是动态创建 FrameLayout 并 `addView` 到 `MainActivity` 的 `decorView`
- `MpvPlayerModule` 的 `activity` 引用需要适配（不再从 LynxContext 获取 Activity，而是直接传入 MainActivity）
- 覆盖层需要 `MATCH_PARENT` 布局参数，覆盖在 WebView 之上

### 步骤 4：修改 GoBackendModule 新增 closePlayer

在 `GoBackendModule` 中新增 `@LynxMethod closePlayer()`：
- 通知 `PlayerOverlayManager.hideOverlay()`
- 暂停 MPV 播放
- 移除覆盖层

### 步骤 5：修改 GoProcessPlugin

新增两个 Capacitor Plugin 方法：

```kotlin
@PluginMethod
fun openPlayer(call: PluginCall) {
    val path = call.getString("path", "")
    val name = call.getString("name", "")
    val mimeType = call.getString("mimeType", "")
    // 委托给 PlayerOverlayManager.showOverlay()
}

@PluginMethod
fun closePlayer(call: PluginCall) {
    // 委托给 PlayerOverlayManager.hideOverlay()
}
```

保留 `openInPlayer()` 方法不变（向后兼容，仍跳转 Activity）。

### 步骤 6：前端 Capacitor 调用

**6.1 修改 `GoProcessPlugin` 接口**

在 `web.ts` 的 `GoProcessPlugin` 接口新增：
```typescript
openPlayer(options: { path: string; name: string; mimeType: string }): Promise<void>
closePlayer(): Promise<void>
```

**6.2 修改 `GoProcess.ts`**

新增 `openPlayer()` 和 `closePlayer()` 导出函数。

**6.3 修改 `Files.vue`**

将 `openInPlayer()` 调用替换为 `openPlayer()`。

**6.4 修改 `HomePage.vue`**

将 `openPlayerHome()` 替换为 `openPlayer()`（不带文件参数，打开播放器首页）。

### 步骤 7：MpvPlayerModule 适配

当前 `MpvPlayerModule` 从 `LynxContext.activity` 获取 Activity 引用。
在覆盖层模式下，LynxContext 的 activity 可能不是 MainActivity。

解决方案：在 `PlayerOverlayManager` 创建 LynxView 时，确保 LynxView 的 LynxContext 关联到 MainActivity。
如果 LynxContext 无法获取正确的 Activity，需要在 MpvPlayerModule 中增加一个显式设置 Activity 的方法。

### 步骤 8：构建验证

```bash
cd android && ./gradlew assembleDebug
```

### 步骤 9：清理

- `PlayerActivityLynx.kt` — 保留但不再作为主要入口（外部 Intent 打开视频仍需要）
- `PlayerActivity.kt` — 保留（处理外部 Intent）
- `PLAYER_ACTIVITY_FIX.md` — 删除（过时文档）

## 文件变更清单

| 操作 | 文件 |
|------|------|
| 新建 | `lynx-player/src/player/ProgressBar.tsx` |
| 新建 | `lynx-player/src/player/player.css` |
| 修改 | `lynx-player/src/player/PlayerApp.tsx` — closePlayer 调用 |
| 新建 | `android/.../PlayerOverlayManager.kt` |
| 修改 | `android/.../GoBackendModule.kt` — 新增 closePlayer |
| 修改 | `android/.../GoProcessPlugin.kt` — 新增 openPlayer/closePlayer |
| 修改 | `android/.../MpvPlayerModule.kt` — 适配覆盖层模式 |
| 修改 | `src/plugins/web.ts` — 新增接口方法 |
| 修改 | `src/plugins/GoProcess.ts` — 新增导出函数 |
| 修改 | `src/views/Files.vue` — 使用 openPlayer |
| 修改 | `src/views/HomePage.vue` — 使用 openPlayer |
| 删除 | `PLAYER_ACTIVITY_FIX.md` |

## 风险与注意事项

1. **SurfaceView 层级**：SurfaceView 是特殊 View，必须在 LynxView 之下（先 addView），否则会遮挡 UI
2. **LynxView 透明**：必须 `setBackgroundColor(0)` 确保透明
3. **生命周期**：覆盖层在 MainActivity 内，Activity 销毁时必须清理
4. **全屏切换**：`MpvPlayerModule.setFullscreen()` 通过 Activity 的 systemUiVisibility 控制，在覆盖层模式下同样适用
5. **返回键**：需要拦截 MainActivity 的返回键，如果覆盖层显示则关闭覆盖层而非退出 Activity
6. **MPV 单例**：`MpvPlayerModule` 使用 `_instance` 单例，覆盖层和 Activity 模式不能同时存在
