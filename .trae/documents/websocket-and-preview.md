# WebSocket 实时通讯 & 本地预览方案

## 问题分析

### 问题 1：替代轮询 — WebSocket vs SSE

当前 `useServerStatus.ts` 使用 `setInterval` 每 10 秒轮询 `/health`，资源浪费且延迟高。

**方案对比：**

| 方案 | 实时性 | 双向 | 复杂度 | Go 后端 | 前端 |
|------|--------|------|--------|---------|------|
| 轮询（当前） | 低 | ❌ | 最低 | 无需改动 | 无依赖 |
| SSE | 高（单向） | ❌ | 低 | 简单 | EventSource 原生 |
| WebSocket | 高 | ✅ | 中 | gorilla/websocket | 原生 API |

**用户需求：后续迟早需要双向实时通讯**

这意味着：
- 远程控制（手机→Go daemon 发指令）
- 实时日志推送（Go daemon→手机）
- 任务交互（暂停/恢复/优先级调整）
- 文件操作通知（上传/下载进度）

**结论：直接上 WebSocket，一步到位**

理由：
1. 既然双向通讯是确定需求，先用 SSE 再迁移 WS 是重复工作
2. Go 后端用 `gorilla/websocket` 或 `nhooyr.io/websocket` 实现极简
3. 前端原生 `WebSocket` API 零依赖，Capacitor WebView 完全支持
4. WebSocket 连接建立后，SSE 能做的 WS 都能做，且额外支持客户端→服务端推送

### Capacitor 的 WebSocket 支持

Capacitor 本身**不提供**内置 WebSocket 插件，但有两个层面：

1. **WebView 层（推荐）**：直接使用浏览器原生 `WebSocket` API
   - Capacitor WebView 完全支持 WSS/WS 协议
   - 零额外依赖，性能最优
   - 本项目 Go daemon 运行在 localhost，`ws://127.0.0.1:2025/ws` 完全可用

2. **Native 层插件**（仅特殊场景需要）：
   - `@miaz/capacitor-websocket` — 跨平台原生 WS 实现（3 年未更新，不推荐）
   - `capacitor-foreground-websocket` — 前台服务 WS，适合后台保活场景
   - `capacitor-signalr` — SignalR 原生客户端（.NET 后端专用）

**本项目推荐 WebView 层原生 WebSocket**，原因：
- Go daemon 在 localhost，不存在跨域/证书问题
- 不需要后台保活（App 在前台时才需要实时通讯）
- 零依赖、零维护负担

### 问题 2：本地预览

当前沙箱环境完全支持：
- `vite dev` 监听 `0.0.0.0:5173`
- `OpenPreview` 工具激活端口
- 无需 Go 后端也能预览 UI（API 调用失败但页面正常渲染）

## 实现计划

### 第一步：新建 WebSocket 连接管理器

**新建文件：** `src/composables/useWebSocket.ts`

**实现细节：**
1. WebSocket 连接生命周期管理：
   - `connect()` — 建立 WS 连接到 `ws://{baseUrl}/ws`
   - `disconnect()` — 关闭连接
   - 自动重连：指数退避（1s → 2s → 4s → 8s → 最大 30s）
   - 连接状态 ref：`connecting` | `connected` | `disconnected`
2. 消息收发：
   - `send(type, data)` — 发送类型化消息
   - `onMessage(type, handler)` — 注册消息处理器
   - 消息格式：`{ type: string, data: any }`
3. 心跳机制：
   - 每 30s 发送 ping，超时 10s 无 pong 则重连
4. URL 自动从 API base URL 推导（`http://` → `ws://`，`https://` → `wss://`）

### 第二步：新建类型安全事件总线

**新建文件：** `src/composables/useEventBus.ts`

**实现细节：**
1. 轻量级泛型事件总线（无第三方依赖）
2. 类型定义：
   ```typescript
   interface EncvEvents {
     'task:update': { id: string; type: string; status: string; progress: number }
     'task:created': { id: string; type: string; sourcePath: string }
     'task:completed': { id: string; error?: string }
     'file:change': { path: string; action: 'create' | 'delete' | 'modify' }
     'server:status': { online: boolean }
     'log:message': { level: string; message: string }
   }
   ```
3. `on<K>(event: K, handler)` / `off<K>(event: K, handler)` / `emit<K>(event: K, data)`

### 第三步：重构 useServerStatus

**修改文件：** `src/composables/useServerStatus.ts`

**实现细节：**
1. 移除 `setInterval` 轮询
2. WS 连接成功时 → `isOnline = true`
3. WS 断开时 → `isOnline = false`
4. 监听 `server:status` 事件更新状态
5. 保留 `checkStatus()` 作为手动检查的兜底（首次启动 WS 未连接时使用）
6. 首次加载时用 HTTP `/health` 检查，WS 连接后切换到 WS 驱动

### 第四步：App.vue 初始化 WebSocket

**修改文件：** `src/App.vue`

**实现细节：**
1. 在 `onMounted` 中初始化 WebSocket 连接
2. 将 WS 消息分发到事件总线
3. `onUnmounted` 中断开连接

### 第五步：Tasks 页面接入实时更新

**修改文件：** `src/views/Tasks.vue`

**实现细节：**
1. 监听 `task:update` 事件，实时更新对应任务的状态和进度
2. 监听 `task:created` 事件，新任务自动出现在列表
3. 监听 `task:completed` 事件，更新完成状态
4. 保留下拉刷新作为兜底

### 第六步：Files 页面接入文件变更通知

**修改文件：** `src/views/Files.vue`

**实现细节：**
1. 监听 `file:change` 事件，当当前目录下文件变化时自动刷新列表
2. 保留下拉刷新作为兜底

### 第七步：API 层增加 WebSocket URL 工具

**修改文件：** `src/api/encv.ts`

**实现细节：**
1. 新增 `getWebSocketUrl()` 函数
2. 将 `http://` 替换为 `ws://`，`https://` 替换为 `wss://`
3. 路径为 `/ws`

### 第八步：本地预览

**操作步骤：**
1. 修改 `vite.config.ts`，添加 `host: '0.0.0.0'`
2. 运行 `npm run dev` 启动 Vite 开发服务器
3. 使用 `OpenPreview` 激活预览
4. 浏览器中查看所有页面效果

### 第九步：构建验证

1. 运行 `npm run build` 确保 TypeScript 无错误
2. 确认 dist 产物正常

## 文件变更总览

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新建 | `src/composables/useWebSocket.ts` | WebSocket 连接管理器 |
| 新建 | `src/composables/useEventBus.ts` | 类型安全事件总线 |
| 修改 | `src/composables/useServerStatus.ts` | WS 驱动状态 + HTTP 兜底 |
| 修改 | `src/App.vue` | 初始化 WS + 事件分发 |
| 修改 | `src/views/Tasks.vue` | 接入 task 实时更新 |
| 修改 | `src/views/Files.vue` | 接入 file 变更通知 |
| 修改 | `src/api/encv.ts` | 新增 WS URL 工具函数 |
| 修改 | `vite.config.ts` | 添加 host: '0.0.0.0' |

## 执行顺序

1. 新建事件总线 `useEventBus.ts`
2. 新建 WebSocket 管理器 `useWebSocket.ts`
3. 扩展 API 层 `encv.ts`（WS URL 函数）
4. 重构 `useServerStatus.ts`
5. 修改 `App.vue`（初始化 WS + 事件分发）
6. 修改 `Tasks.vue`（接入实时更新）
7. 修改 `Files.vue`（接入文件变更通知）
8. 修改 `vite.config.ts`（添加 host）
9. 启动 dev server + OpenPreview 预览
10. 构建验证

## 未来扩展

WebSocket 双向通讯为以下场景预留了通道：
- **远程控制**：前端发送 `{ type: "command", data: { action: "encrypt", path: "..." } }`
- **实时日志**：Go daemon 推送 `{ type: "log:message", data: { level: "info", message: "..." } }`
- **进度推送**：Go daemon 推送 `{ type: "task:update", data: { id: "...", progress: 45 } }`
- **文件监控**：Go daemon 推送 `{ type: "file:change", data: { path: "...", action: "modify" } }`
