# Tasks

- [x] Task 1: 新建 PlayerActivity.kt
  - [x] SubTask 1.1: 创建 `PlayerActivity.kt`，继承 `BridgeActivity`，在 `onCreate` 中注册 `GoProcessPlugin`
  - [x] SubTask 1.2: 实现 `onCreate` 中独立启动后端逻辑：检查 `EncvGoService.isRunning`，若未运行则启动 `EncvGoService`
  - [x] SubTask 1.3: 注册 `BroadcastReceiver` 监听 `BROADCAST_BACKEND_READY` / `BROADCAST_BACKEND_STATUS`，收到后通过 `evaluateJavascript` 派发事件到前端
  - [x] SubTask 1.4: 实现 intent 文件 URI 解析（`content://` 和 `file://`），存储解析结果到伴生对象字段
  - [x] SubTask 1.5: 实现 `content://` URI 解析：通过 `ContentResolver` 查询文件名，尝试解析实际路径，失败时复制文件到缓存目录
  - [x] SubTask 1.6: WebView 加载完成后导航到 `#/standalone/player` 路由
  - [x] SubTask 1.7: 实现 `onNewIntent` 和 `onDestroy` 生命周期管理

- [x] Task 2: 扩展 GoProcessPlugin
  - [x] SubTask 2.1: 新增 `isStandaloneMode()` 方法，根据 `activity` 类型返回 standalone 状态
  - [x] SubTask 2.2: 新增 `getIntentFileInfo()` 方法，从 `PlayerActivity` 伴生对象读取文件信息并返回
  - [x] SubTask 2.3: 更新 `GoProcess.ts`，新增 `isStandaloneMode()` 和 `getIntentFileInfo()` 的 TypeScript 接口和调用封装
  - [x] SubTask 2.4: 更新 `web.ts`，新增 web 端空实现

- [x] Task 3: 修改 AndroidManifest.xml
  - [x] SubTask 3.1: 注册 `PlayerActivity` 声明，设置 `exported="true"`、`launchMode="singleTop"`
  - [x] SubTask 3.2: 添加 `ACTION_VIEW` intent-filter，支持 `video/*` MIME 类型
  - [x] SubTask 3.3: 添加 `ACTION_VIEW` intent-filter，支持 `audio/*` MIME 类型
  - [x] SubTask 3.4: 添加 `ACTION_VIEW` intent-filter，支持 `application/x-encv` MIME 类型
  - [x] SubTask 3.5: 为每个 intent-filter 配置 `file` 和 `content` scheme

- [x] Task 4: 新建 StandalonePlayer.vue
  - [x] SubTask 4.1: 创建 `StandalonePlayer.vue`，包含独立播放器 UI（无 TabBar，全屏播放区域）
  - [x] SubTask 4.2: 实现独立后端初始化逻辑：调用 `isStandaloneMode()` → `getIntentFileInfo()` → `getBackendStatus()` → 设置 API URL
  - [x] SubTask 4.3: 实现后端等待状态 UI（加载动画 + "正在启动后端..."）
  - [x] SubTask 4.4: 实现视频/音频/加密文件的播放逻辑，复用 ArtPlayer 和 `<audio>` 标签
  - [x] SubTask 4.5: 实现播放错误处理和重试逻辑
  - [x] SubTask 4.6: 监听 `encv:backend-ready` / `encv:backend-status` 事件更新后端连接状态

- [x] Task 5: 修改路由配置
  - [x] SubTask 5.1: 在 `router/index.ts` 中新增 `/standalone/player` 路由，指向 `StandalonePlayer.vue`
  - [x] SubTask 5.2: 确保独立路由不在 Tabs 子路由下，作为顶层路由存在

- [x] Task 6: 后端新增外部文件流式传输端点
  - [x] SubTask 6.1: 在 `mobile_api.go` 中注册 `GET /api/stream/external` 路由
  - [x] SubTask 6.2: 在 `mobile_service.go` 中实现外部文件流式传输：检查文件存在性、可读性、媒体类型，支持 Range 请求
  - [x] SubTask 6.3: 前端 `api/encv.ts` 新增 `getExternalStreamUrl()` 函数

- [ ] Task 7: 集成测试与验证
  - [ ] SubTask 7.1: 验证从文件管理器打开视频文件可正常播放
  - [ ] SubTask 7.2: 验证后端未运行时 PlayerActivity 可独立启动后端
  - [ ] SubTask 7.3: 验证 content:// URI 可正确解析和播放
  - [ ] SubTask 7.4: 验证加密 .encv 文件可通过 ENCV Player 打开播放
  - [ ] SubTask 7.5: 验证从 MainActivity 内部点击视频仍正常播放（不影响现有功能）

# Task Dependencies

- [Task 2] depends on [Task 1]（GoProcessPlugin 扩展依赖 PlayerActivity 伴生对象字段）
- [Task 3] depends on [Task 1]（AndroidManifest 注册依赖 PlayerActivity 类存在）
- [Task 4] depends on [Task 2]（StalonePlayer 依赖 GoProcessPlugin 新方法）
- [Task 4] depends on [Task 5]（StandalonePlayer 依赖路由配置）
- [Task 4] depends on [Task 6]（外部文件播放依赖后端新端点）
- [Task 7] depends on [Task 1, 2, 3, 4, 5, 6]（集成测试依赖所有功能完成）
- [Task 1] 和 [Task 6] 可并行开发
