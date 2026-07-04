# Artplayer 覆盖播放方案

## 背景

用户要求：
1. 删除 PlayerApp.vue 及引用的 player views（独立 Capacitor 播放器模式）
2. 音频播放选择：内置 MPV、外部打开（第三方应用）
3. 视频播放选择：内置 MPV、内置 Artplayer、外部打开（第三方应用）
4. 播放方式在设置界面配置，不是每次弹出选择

## 架构设计

### 三种播放方式

| 方式 | 实现 | 适用 |
|------|------|------|
| 内置 Artplayer | 主 WebView 内路由到 `/player` 页面，Artplayer（HTML5）播放 | 仅视频 |
| 内置 MPV | PlayerOverlayManager 在 MainActivity 上叠加 MpvSurfaceView + LynxView | 视频 + 音频 |
| 外部打开 | Android Intent `ACTION_VIEW` + `createChooser` 打开第三方播放器（VLC、MX Player 等） | 视频 + 音频 |

### 外部打开方案选择

| 方案 | 机制 | 优劣 |
|------|------|------|
| `@capacitor/share` | `ACTION_SEND` | ❌ 弹出分享面板，显示社交/消息类应用，不适合打开播放器 |
| `@capacitor/app-launcher` | `ACTION_VIEW` | ⚠️ 只能打开默认应用，不能让用户选择 |
| **自定义 Plugin 方法** | `ACTION_VIEW` + `createChooser` | ✅ 弹出应用选择器，显示所有可处理该 MIME 类型的应用 |

结论：在现有 `GoProcessPlugin` 中新增 `openExternal()` 方法，使用 `Intent.ACTION_VIEW` + `Intent.createChooser`。代码量约 15 行，无需引入额外插件。

### 设置界面

在 Settings.vue 的"外观"区域下方新增"播放"设置区域：
- **视频播放方式**：下拉选择（内置 Artplayer / 内置 MPV / 外部打开）
- **音频播放方式**：下拉选择（内置 MPV / 外部打开）

存储在 localStorage，key: `encv_player_video` / `encv_player_audio`，默认值：视频 → 内置 Artplayer，音频 → 内置 MPV。

### 播放流程

用户点击媒体文件 → 读取 localStorage 中的播放偏好 → 直接使用对应方式播放

## 实施步骤

### 步骤 1：创建 Artplayer 播放器视图

新建 `/workspace/app/encv-mobile/src/views/ArtPlayerView.vue`

基于现有 `StandalonePlayer.vue` 的 Artplayer 逻辑，但改为：
- 作为主应用路由页面（不是独立 Activity）
- 从路由 query 获取文件信息（`path`、`name`）
- 使用 `getFileStreamUrl()` / `getExternalStreamUrl()` 获取流 URL
- 顶部工具栏：返回按钮 + 文件名
- 视频区域使用 Artplayer
- 支持全屏切换（通过 GoProcess.setScreenOrientation）

### 步骤 2：修改路由配置

修改 `/workspace/app/encv-mobile/src/router/index.ts`：
- `/player` 路由改为指向 `ArtPlayerView.vue`（替换 StandalonePlayer）
- 删除对 `StandalonePlayer.vue` 的引用

### 步骤 3：新增外部打开（第三方应用）

3.1 修改 `GoProcessPlugin.kt`，新增 `openExternal()` 方法：
```kotlin
@PluginMethod
fun openExternal(call: PluginCall) {
    val url = call.getString("url", "")
    val mimeType = call.getString("mimeType", "")
    if (url.isNullOrEmpty()) {
        call.reject("url is required")
        return
    }
    try {
        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(Uri.parse(url), mimeType)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        val chooser = Intent.createChooser(intent, null)
        activity.startActivity(chooser)
        call.resolve()
    } catch (e: Exception) {
        call.reject("Failed to open externally: ${e.message}")
    }
}
```

3.2 修改 `web.ts` 接口，新增 `openExternal(options: { url: string; mimeType: string }): Promise<void>`

3.3 修改 `GoProcess.ts`，新增 `openExternal(url: string, mimeType: string)` 导出函数

### 步骤 4：修改 Files.vue 播放逻辑

4.1 新增 `getPlayMode(mediaType: 'video' | 'audio')` 函数：
- 从 localStorage 读取播放偏好
- 默认值：视频 → `artplayer`，音频 → `mpv`

4.2 修改 `handleFileClick()`：
- 媒体文件根据 `getPlayMode()` 直接使用对应方式播放
- `artplayer` → 路由到 `/player?path=xxx&name=xxx`
- `mpv` → 调用 `GoProcess.openPlayer()`
- `external` → 调用 `GoProcess.openExternal(getExternalStreamUrl(path), mimeType)`

4.3 长按菜单也同步修改

### 步骤 5：修改 HomePage.vue

播放器入口按钮改为路由到 `/player`（Artplayer 页面），不再调用 `openPlayer()`。

### 步骤 6：在 Settings.vue 添加播放设置

在"外观"区域下方新增"播放"设置区域：
- 视频播放方式：`ion-select`，选项为"内置 Artplayer"/"内置 MPV"/"外部打开"
- 音频播放方式：`ion-select`，选项为"内置 MPV"/"外部打开"

使用 localStorage 存储，key: `encv_player_video` / `encv_player_audio`。

### 步骤 7：添加 i18n 键

在 `useI18n.ts` 中新增：
- `settings.player` — 播放
- `settings.videoPlayer` — 视频播放方式
- `settings.audioPlayer` — 音频播放方式
- `settings.builtInArtplayer` — 内置 Artplayer
- `settings.builtInMpv` — 内置 MPV
- `settings.openExternal` — 外部打开

### 步骤 8：删除旧文件

删除以下文件：
- `/workspace/app/encv-mobile/src/PlayerApp.vue`
- `/workspace/app/encv-mobile/src/views/StandalonePlayer.vue`
- `/workspace/app/encv-mobile/src/views/PlayerSettings.vue`
- `/workspace/app/encv-mobile/src/player-main.ts`
- `/workspace/app/encv-mobile/src/router/player.ts`
- `/workspace/app/encv-mobile/player.html`

### 步骤 9：修改 Vite 配置

修改 `/workspace/app/encv-mobile/vite.config.ts`：
- 移除 `player.html` 的多入口构建配置

### 步骤 10：构建验证

- `npx vue-tsc --noEmit && npx vite build`（前端构建）
- `npx rspeedy build`（Lynx 播放器 bundle，内置 MPV 仍需要）

## 文件变更清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `src/views/ArtPlayerView.vue` | Artplayer 播放器视图 |
| 修改 | `src/router/index.ts` | `/player` 路由指向 ArtPlayerView |
| 修改 | `src/views/Files.vue` | 播放逻辑（根据设置选择方式） |
| 修改 | `src/views/HomePage.vue` | 播放器入口 |
| 修改 | `src/views/Settings.vue` | 新增播放方式设置 |
| 修改 | `src/composables/useI18n.ts` | 新增播放设置 i18n 键 |
| 修改 | `src/plugins/GoProcess.ts` | 新增 openExternal 函数 |
| 修改 | `src/plugins/web.ts` | 新增 openExternal 接口 |
| 修改 | `android/.../GoProcessPlugin.kt` | 新增 openExternal 方法 |
| 修改 | `vite.config.ts` | 移除 player.html 多入口 |
| 删除 | `src/PlayerApp.vue` | 独立播放器入口 |
| 删除 | `src/views/StandalonePlayer.vue` | 独立播放器视图 |
| 删除 | `src/views/PlayerSettings.vue` | 播放器设置 |
| 删除 | `src/player-main.ts` | 独立播放器 JS 入口 |
| 删除 | `src/router/player.ts` | 独立播放器路由 |
| 删除 | `player.html` | 独立播放器 HTML |

## 注意事项

1. **Artplayer 在 Capacitor WebView 中的兼容性**：Capacitor WebView 基于 Chrome/WebView，Artplayer 的 HTML5 播放完全兼容
2. **外部打开需要 HTTP 流 URL**：第三方播放器需要可访问的 HTTP URL，使用 `getExternalStreamUrl()` 生成
3. **MPV 覆盖层与 Artplayer 互斥**：同一时间只能使用一种内置播放方式
4. **Lynx 播放器 JS 端保留**：内置 MPV 仍需要 LynxView 作为 UI 控件层
5. **PlayerActivity 保留**：外部 Intent 打开视频仍需要 PlayerActivity
