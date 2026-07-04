# 修复 ArtPlayer 未加载 + 添加调试日志

## 问题分析

用户反馈：ArtPlayer 没有加载，播放的是 WebView 默认播放器。

### 根因分析

经过深入代码审查，发现以下问题：

#### 1. ArtPlayer 初始化缺乏错误处理和日志（主因）
`ArtPlayerView.vue` 的 `initArtPlayer()` 函数：
- `new Artplayer({...})` 没有 try-catch 包裹，如果构造函数抛异常会被吞掉
- 如果 `artContainer.value` 为 null 或 `streamUrl.value` 为空，函数静默返回，不设置任何错误状态
- 没有任何 console.log 调试信息，无法判断初始化走到哪一步

#### 2. 混合内容可能被阻止
Capacitor 配置 `androidScheme: 'https'`，WebView 页面 URL 为 `https://localhost/...`。
但 `getFileStreamUrl()` 返回 `http://127.0.0.1:PORT/stream?path=...`。
从 `https://` 页面加载 `http://` 资源属于混合内容，Android WebView 默认可能阻止。
虽然 `network_security_config.xml` 允许明文流量，但 WebView 的 `setMixedContentMode` 未设置。

#### 3. ArtPlayer 的 `controls: false` 时序问题
ArtPlayer 通过 `moreVideoAttr: { controls: false }` 设置 video 元素不显示原生控件，
但代码中还有 `nextTick` 里手动 `removeAttribute('controls')` 的冗余操作。
如果 ArtPlayer 初始化失败或延迟，video 元素可能短暂显示原生控件。

#### 4. PlayerActivityLynx.buildInitDataJson 使用 `filePath` 而非 `streamUrl`
`PlayerActivityLynx.kt` 第 377 行 `put("filePath", intentFilePath)`，
但 `PlayerApp.tsx` 第 38 行读取 `initData.streamUrl`。
这导致从外部打开文件时 Lynx 播放器 streamUrl 为空。

## 修复方案

### Step 1: ArtPlayerView.vue — 添加完整调试日志和错误处理

1. 在 `initArtPlayer()` 入口添加日志，打印 `artContainer`、`streamUrl` 状态
2. 用 try-catch 包裹 `new Artplayer(...)`，捕获构造函数异常
3. 在 Artplayer 事件回调中添加日志（ready、error、video:loadedmetadata 等）
4. 在 `startPlayback()` 中添加日志
5. 如果 `artContainer` 或 `streamUrl` 为空，设置明确的错误状态而非静默返回
6. 在 `onMounted` 中添加日志

### Step 2: ArtPlayerView.vue — 修复原生控件显示问题

1. 在 Artplayer 配置中显式设置 `moreVideoAttr: { controls: false, preload: 'metadata', playsinline: true, 'webkit-playsinline': true }`
2. 监听 Artplayer 的 `ready` 事件，在 ready 后确保移除原生 controls
3. 添加 CSS `:deep(video) { outline: none; }` 避免焦点边框

### Step 3: Android WebView — 设置 MixedContentMode

在 `MainActivity.kt` 中，Capacitor Bridge 初始化后设置 WebView 的 mixedContentMode：
```kotlin
bridge.webView.settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
```

### Step 4: PlayerActivityLynx.kt — 修复 initData 字段名

将 `buildInitDataJson()` 中的 `put("filePath", intentFilePath)` 改为 `put("streamUrl", streamUrl)`，
其中 `streamUrl` 需要根据后端端口构建流 URL（与 PlayerOverlayManager 一致）。

### Step 5: 构建验证

运行 `vue-tsc --noEmit && vite build` 确保前端构建通过。
