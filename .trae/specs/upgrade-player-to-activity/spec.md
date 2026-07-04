# 播放器升级为独立 Activity + 注册第三方打开方式 Spec

## Why

当前播放器作为 Tabs 内的子路由存在，无法被外部应用调用，且生命周期完全依赖 MainActivity。将播放器升级为独立 Activity 后：1）可注册为第三方视频/音频/加密文件的打开方式（ENCV Player），让用户从文件管理器等应用直接调起播放；2）独立 Activity 拥有自己的窗口和返回栈，支持画中画、分屏等系统级能力；3）独立获取配置与内核交互，不依赖 MainActivity 是否已启动。

## What Changes

- 新建 `PlayerActivity.kt`，继承 `BridgeActivity`，独立注册 `GoProcessPlugin`，独立监听后端状态广播
- 新建 `StandalonePlayer.vue`，无 TabBar 的独立播放器页面，包含后端启动等待状态、错误处理
- 新增 Vue 路由 `/standalone/player`，渲染 `StandalonePlayer.vue`
- 扩展 `GoProcessPlugin`，新增 `isStandaloneMode()` 和 `getIntentFileInfo()` 方法
- 扩展 `GoProcess.ts` / `web.ts`，新增对应的 TypeScript 接口
- 修改 `AndroidManifest.xml`，注册 `PlayerActivity` 及其 intent-filter（video/audio/encv MIME 类型）
- 修改 `EncvGoService`，新增 `/api/stream/external` 端点支持流式传输任意可访问路径的文件
- 修改 `PlayerActivity` 的通知点击行为，指向 PlayerActivity 自身

## Impact

- Affected specs: `preview-artplayer-logging`（Player.vue 逻辑不变，新增 StandalonePlayer.vue 为独立页面）
- Affected code:
  - `android-overlay/app/src/main/java/com/encvgo/app/PlayerActivity.kt`（新建）— 独立播放器 Activity
  - `android-overlay/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — 新增 isStandaloneMode / getIntentFileInfo
  - `android-overlay/app/src/main/AndroidManifest.xml` — 注册 PlayerActivity + intent-filter
  - `src/views/StandalonePlayer.vue`（新建）— 独立播放器页面
  - `src/router/index.ts` — 新增 /standalone/player 路由
  - `src/plugins/GoProcess.ts` — 新增 isStandaloneMode / getIntentFileInfo
  - `src/plugins/web.ts` — 新增 web 端空实现
  - `internal/server/mobile_api.go` — 新增 /api/stream/external 端点
  - `internal/service/mobile_service.go` — 新增外部文件流式传输逻辑

## ADDED Requirements

### Requirement: PlayerActivity 独立启动与后端交互

系统 SHALL 提供 `PlayerActivity`，可独立于 `MainActivity` 启动，并自主完成与 Go 后端的交互。

#### Scenario: 后端已运行时启动
- **WHEN** PlayerActivity 启动且 `EncvGoService.isRunning == true`
- **THEN** 直接从 `EncvGoService.lastKnownPort` 获取端口，设置 API 基础 URL，通知前端播放

#### Scenario: 后端未运行时启动
- **WHEN** PlayerActivity 启动且 `EncvGoService.isRunning == false`
- **THEN** 启动 `EncvGoService`（ACTION_START），显示加载状态，注册 BroadcastReceiver 等待 `BROADCAST_BACKEND_READY`
- **WHEN** 收到 `BROADCAST_BACKEND_READY`
- **THEN** 获取端口，设置 API 基础 URL，通知前端播放

#### Scenario: 后端启动失败
- **WHEN** PlayerActivity 等待后端就绪超时（15 秒）或收到错误广播
- **THEN** 显示错误提示，提供重试按钮

#### Scenario: 后端状态通知
- **WHEN** PlayerActivity 收到后端状态广播
- **THEN** 通过 `bridge.webView.evaluateJavascript` 派发 `encv:backend-ready` / `encv:backend-status` 事件到前端

### Requirement: 第三方文件打开（ENCV Player）

系统 SHALL 将 PlayerActivity 注册为视频、音频、加密文件的第三方打开方式，显示名称为 "ENCV Player"。

#### Scenario: 从文件管理器打开视频文件
- **WHEN** 用户在文件管理器中点击 mp4/mkv/avi/mov/webm 等视频文件，选择 "ENCV Player"
- **THEN** PlayerActivity 启动，接收 `ACTION_VIEW` intent，解析 `content://` 或 `file://` URI 为文件路径，开始播放

#### Scenario: 从文件管理器打开音频文件
- **WHEN** 用户点击 mp3/flac/wav 等音频文件，选择 "ENCV Player"
- **THEN** PlayerActivity 启动并播放音频

#### Scenario: 打开 .encv 加密文件
- **WHEN** 用户点击 .encv 文件，选择 "ENCV Player"
- **THEN** PlayerActivity 启动，通过后端解密流式播放

#### Scenario: content:// URI 解析
- **WHEN** PlayerActivity 收到 `content://` URI
- **THEN** 使用 `ContentResolver` 查询 `OpenableColumns` 获取文件名，尝试通过 `/proc/self/fd` 或 `MediaStore` 解析为实际文件路径；若无法解析，则将文件复制到应用缓存目录后使用缓存路径

#### Scenario: file:// URI 解析
- **WHEN** PlayerActivity 收到 `file://` URI
- **THEN** 直接使用 URI 路径作为文件路径

### Requirement: GoProcessPlugin 扩展方法

系统 SHALL 在 GoProcessPlugin 中新增 `isStandaloneMode()` 和 `getIntentFileInfo()` 方法。

#### Scenario: isStandaloneMode
- **WHEN** 前端调用 `GoProcess.isStandaloneMode()`
- **THEN** 在 PlayerActivity 中返回 `{ standalone: true }`，在 MainActivity 中返回 `{ standalone: false }`

#### Scenario: getIntentFileInfo
- **WHEN** 前端调用 `GoProcess.getIntentFileInfo()`
- **THEN** 返回 `{ path: string, name: string, mimeType: string }` 从 PlayerActivity 的启动 intent 中解析的文件信息
- **WHEN** 不在 PlayerActivity 或无文件 intent
- **THEN** 返回 `{ path: '', name: '', mimeType: '' }`

### Requirement: StandalonePlayer 独立播放器页面

系统 SHALL 提供 `StandalonePlayer.vue`，作为 PlayerActivity 的独立播放器页面，无 TabBar，独立初始化后端连接。

#### Scenario: 页面加载与后端连接
- **WHEN** StandalonePlayer 挂载
- **THEN** 调用 `isStandaloneMode()` 确认独立模式，调用 `getIntentFileInfo()` 获取文件信息，调用 `getBackendStatus()` 检查后端状态
- **WHEN** 后端未就绪
- **THEN** 显示加载动画和 "正在启动后端..." 提示
- **WHEN** 后端就绪
- **THEN** 设置 API 基础 URL，开始播放

#### Scenario: 视频播放
- **WHEN** 文件为视频类型且后端就绪
- **THEN** 使用 ArtPlayer 播放，URL 为 `/stream?path=...`（后端目录内文件）或 `/api/stream/external?path=...`（外部文件）

#### Scenario: 音频播放
- **WHEN** 文件为音频类型且后端就绪
- **THEN** 使用原生 `<audio>` 标签播放

#### Scenario: 加密文件播放
- **WHEN** 文件为 .encv 加密文件且后端就绪
- **THEN** 使用 ArtPlayer 播放，URL 为 `/stream?path=...`（后端自动解密流式传输）

#### Scenario: 播放错误处理
- **WHEN** 播放失败
- **THEN** 显示错误信息和重试按钮

#### Scenario: 返回退出
- **WHEN** 用户按返回键
- **THEN** PlayerActivity finish()，返回调用方应用

### Requirement: 外部文件流式传输端点

系统 SHALL 在 Go 后端新增 `/api/stream/external` 端点，支持流式传输任意可访问路径的文件。

#### Scenario: 请求外部文件流
- **WHEN** 前端请求 `GET /api/stream/external?path=/storage/emulated/0/Download/video.mp4`
- **THEN** 后端检查文件是否存在且可读，检查文件类型是否为媒体文件（video/audio），返回流式响应（支持 Range 请求）

#### Scenario: 文件不存在或不可读
- **WHEN** 请求的文件路径不存在或无读取权限
- **THEN** 返回 404 或 403 错误

#### Scenario: 非媒体文件
- **WHEN** 请求的文件不是视频/音频类型
- **THEN** 返回 415 Unsupported Media Type 错误

### Requirement: PlayerActivity WebView 路由导航

系统 SHALL 在 PlayerActivity 启动后，将 WebView 导航到独立播放器路由。

#### Scenario: WebView 加载完成后导航
- **WHEN** PlayerActivity 的 WebView 页面加载完成
- **THEN** 通过 `bridge.webView.evaluateJavascript` 导航到 `#/standalone/player` 路由

#### Scenario: 带文件参数导航
- **WHEN** PlayerActivity 有文件 intent 数据
- **THEN** 文件信息通过 `getIntentFileInfo()` 插件方法传递给前端，不通过 URL 参数传递（避免路径编码问题）

## MODIFIED Requirements

### Requirement: AndroidManifest.xml Activity 注册

`AndroidManifest.xml` SHALL 新增 `PlayerActivity` 声明，包含 `ACTION_VIEW` intent-filter 支持 video/*、audio/* 和 application/x-encv MIME 类型，以及 file 和 content scheme。

### Requirement: GoProcessPlugin

`GoProcessPlugin` SHALL 新增 `isStandaloneMode()` 和 `getIntentFileInfo()` 两个 `@PluginMethod`，供前端判断当前运行模式和获取外部文件信息。

## REMOVED Requirements

无移除的需求。
