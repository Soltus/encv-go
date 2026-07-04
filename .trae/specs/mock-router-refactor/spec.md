# Mock 架构重构 Spec

## Why

当前 mock 方式存在 3 个结构性缺陷：

1. **逐端点 handler 反模式** — 每个 API 手写一个 handler 函数（30+ 个），新增前端调用必漏 → 刚才 `/api/file` 遗漏导致 txt 预览不可用
2. **双数据源分裂** — `file-system.ts` 硬编码内存假数据（Movies/Documents/Music），`handlers.ts` 又从 `__mock_data__/` 扫描真实文件，两者不一致且同时维护
3. **依赖外部 Go 后端** — vite proxy 指向 `127.0.0.1:2026`（无人监听），mock 未命中时 fallback 到 proxy → 连接拒绝 → "后端崩溃"

**正确做法**：mock 应是**自包含的通用 REST 代理层**，基于 `__mock_data__/` 真实文件系统，零依赖外部后端。

## What Changes

- **删除** `handlers.ts` 中全部 30+ 个逐端点 handler
- **重写** 为一个通用路由分发器（router pattern），按 URL path + method 自动分发
- **统一数据源**：删除 `file-system.ts` 的硬编码 DEFAULT_FILES/MOVIES 等，全部走 `__mock_data__/` 文件扫描
- **修复** vite.config.ts proxy 配置（端口 + bypass），确保 mock 完全接管时不穿透到外部
- **保留** MOCK_PLUGINS / MOCK_SUFFIX 等纯配置常量（移到 config 层）

### 核心设计：Router Pattern

```
请求进入 → parseUrl() → routeMatch()
  ├─ /api/files*          → fileSystemHandler()     ← 统一文件操作路由
  │   ├─ GET    ?path=/   → listDir()
  │   ├─ GET    /stream   → sseStream()
  │   ├─ GET    /plugin-stream → pluginFilter()
  │   ├─ POST   /mkdir    → createDir()
  │   ├─ DELETE ?path=..  → deleteFile()
  │   └─ POST   (body)    → rename/copy/move
  ├─ /api/file*           → fileContentHandler()    ← 文件内容读取/写入
  │   ├─ GET              → readFileContent()
  │   ├─ PATCH            → renameOriginalName()
  │   └─ POST             → renameFile()
  ├─ /api/config*         → staticJsonHandler()      ← 固定 JSON 响应
  ├─ /api/plugins*        → staticJsonHandler()
  ├─ /api/tasks*          → taskMockHandler()
  ├─ /health              → { status: 'ok' }
  ├─ /stream*             → staticFileHandler()       ← 直接 serve 文件二进制
  ├─ /preview*            → staticFileHandler()
  └─ * (其他)             → next() 或友好 404 JSON
```

## Impact

- Affected code: `mock/index.ts`, `mock/handlers.ts`, `mock/file-system.ts`, `vite.config.ts`
- 不影响: 前端业务代码、Go 后端代码、generate-mock-files.ts 脚本

## ADDED Requirements

### Requirement: 通用路由分发器

Mock 中间件 SHALL 提供单一路由入口函数 `dispatchRequest(req, res)`，根据 URL path 前缀和 HTTP method 自动分发到对应处理函数。

#### Scenario: 未知 API 路径自动兜底
- **WHEN** 前端发起任意 `/api/*` 请求但无专门 handler 匹配
- **THEN** 返回 `{ error: "not implemented in mock", path: "..." }` JSON + 501 状态码
- **AND** 绝不调用 `next()` 穿透到 proxy（避免连接拒绝）

### Requirement: 统一文件系统后端

所有涉及文件列表/内容/搜索的操作 SHALL 基于 `__mock_data__/` 目录的真实文件系统。
- `scanRealFiles(dir)` 作为唯一数据源函数
- 删除 `file-system.ts` 中硬编码的 `DEFAULT_FILES`, `MOVIES_FILES`, `DOCUMENTS_FILES`, `MUSIC_FILES`
- 保留 `FILE_MAP` 仅作为运行时缓存（lazy populate）

#### Scenario: txt 文件预览
- **WHEN** 前端请求 `GET /api/file?path=/01-plain-media/document/notes.txt`
- **THEN** 从 `__mock_data__/01-plain-media/document/notes.txt` 读取文件内容
- **AND** 返回 `{ name, path, size, content: "<utf8>", encoding: "utf-8" }`

#### Scenario: SSE 流式文件列表
- **WHEN** 前端请求 `GET /api/files/stream?path=/01-plain-media/image`
- **THEN** 返回 SSE 格式 `data: {...}\n\n` 每个文件一行 + `data: [DONE]\n\n`

### Requirement: 二进制文件流服务

`/stream?path=` 和 `/preview/*` SHALL 直接返回 `__mock_data__/` 中文件的原始字节，带正确 MIME type 和 Content-Length header。

#### Scenario: 图片预览
- **WHEN** 前端请求 `/stream?path=/01-plain-media/image/photo.jpg`
- **THEN** 返回 JPEG 二进制数据 + `Content-Type: image/jpeg`

### Requirement: 静态 JSON 端点

以下端点 SHALL 返回固定合理的 JSON 响应（无需每次重新计算）：
- `/api/config` → 包含 password, server_dir, output_path, plugin_settings
- `/api/plugins` → MOCK_PLUGINS 数组
- `/api/permissions` → `{ storage: true }`
- `/health` → `{ status: ok }`
- `/api/container/versions` → 版本列表
- `/api/ffmpeg-status` → `{ ffmpeg_available: false }`（mock 无 ffmpeg）
- `/api/build-info` → `{ app_version: "0.0.1-mock" }`
- `/api/config/schema` → `{}`
- `/api/index/stats` → 基于 __mock_data__ 动态统计
- `/api/remote/info` → 空 WebDAV/OpenList
- `/api/webdav/test-local|test` → 不可用
- `/api/alist-encrypt/decode-filename` → 去 .ae 后缀还原
- `/api/file/text-preview-exts` → 文本扩展名列表
- `/api/plugins/container-extensions` → 容器扩展映射

### Requirement: Task 操作 Mock

- `POST /api/tasks` → 返回 `{ id: "mock-task-N", status: "queued" }`
- `GET /api/tasks` → 返回空任务列表或已创建的任务
- `POST /api/tasks/:id/cancel` → `{ ok: true }`
- `DELETE /api/tasks/:id` → `{ ok: true }`
- `POST /api/tasks/predict-plugin` → 根据 extension 匹配 MOCK_PLUGINS

### Requirement: Vite Proxy Bypass

vite.config.ts 的 proxy 配置 SHALL 在 mock 模式下自动 bypass（不转发到外部后端）。

#### Scenario: mock 模式下无外部依赖
- **WHEN** mock 中间件已注册且启用
- **THEN** 所有 `/api/*`, `/health`, `/stream`, `/ws` 请求由 mock 处理
- **AND** 不产生任何到 `127.0.0.1:2026` 的网络连接

## MODIFIED Requirements

### Requirement: 自动生成机制（已有）

保持不变：启动时检测 `__mock_data__/` 不存在则自动执行 generate-mock-files.ts。

### Requirement: __mock_control 端点（已有）

保持不变：开发调试用的动态控制端点保留。

## REMOVED Requirements

### Requirement: 逐端点 Handler 映射表
**Reason**: 30+ 个独立 handler 函数不可维护，每新增前端 API 必遗漏
**Migration**: 替换为通用 router dispatch 函数

### Requirement: 硬编码内存文件列表
**Reason**: `DEFAULT_FILES`, `MOVIES_FILES` 等与 `__mock_data__/` 真实文件不同步
**Migration**: 删除，统一使用 `scanRealFiles()` 从磁盘扫描
