# 铁律规则更新 Spec（基于实际代码审计）

## Why

当前 `mock/handlers.ts` 存在 **620 行代码、20+ 个 API 端点的全量 mock 实现**，违反了最小化原则。同时开发流程中存在前台阻塞式服务启动的问题，导致开发体验差。

## What Changes

### 核心问题定位

1. **`mock/handlers.ts` 过度工程化**（620 行，20+ handler）：
   - `fileSystemHandler`: 10 个文件系统 API（files, stream, mkdir, search, exists, tags...）
   - `fileContentHandler`: 7 个文件内容 API（file, rename, copy, move...）
   - `staticJsonHandler`: **20+ 个静态 JSON API**（config, plugins, permissions, container versions, ffmpeg-status, build-info, webdav test, alist-encrypt...）
   - `taskMockHandler`: 4 个任务 API（predict, CRUD, cancel, retry）
   - `staticFileHandler`: 静态文件服务
   - `debugControlHandler`: 调试控制 API

2. **Vite 配置已正确**：
   - ✅ 前端端口: `5173`
   - ✅ 后端代理: `http://127.0.0.1:2025`
   - ✅ 包含完整代理路径: `/api`, `/health`, `/stream`, `/decrypt`, `/preview`, `/ws`

3. **需要规范化的流程**：
   - Go 服务应使用 `go run` 直接运行（非编译后执行）
   - 服务必须在后台运行（禁止前台阻塞终端）

---

## ADDED Requirements

### Requirement 1: 最小化 Mock Handler 原则

**现状**: `mock/handlers.ts` 实现了 **620 行、40+ 个 API 端点**的全量 mock，包括：
- 完整的文件系统 CRUD（10 个端点）
- 文件内容管理（7 个端点）
- 静态配置 API（20+ 个端点）
- 任务管理系统（4 个端点）
- 调试控制接口（5 个端点）

**铁律**:
- ❌ **禁止**: Mock 数量 > 10 个 API 端点
- ❌ **禁止**: Mock 实现完整的业务逻辑（如文件搜索递归遍历、任务状态机等）
- ✅ **允许**: 仅 mock 前端开发必需的最小集合（2-5 个核心端点）
- ✅ **推荐**: 使用真实后端 + 测试数据文件替代 mock

#### Scenario: 当前错误的 Mock 模式

```typescript
// ❌ 错误：mock/handlers.ts 实现了 40+ 个端点
function staticJsonHandler(pathname: string): boolean {
  switch (pathname) {
    case '/api/config': return json(res, { ... })           // 端点 1
    case '/api/plugins': return json(res, { ... })          // 端点 2
    case '/api/permissions': return json(res, { ... })      // 端点 3
    case '/api/container/versions': return json(res, {...}) // 端点 4
    case '/api/config/schema': return json(res, {})         // 端点 5
    case '/api/index/stats': return json(res, {...})        // 端点 6
    case '/api/remote/info': return json(res, {...})        // 端点 7
    case '/api/webdav/test-local': return json(res, {...})  // 端点 8
    case '/api/webdav/test': return json(res, {...})        // 端点 9
    case '/api/ffmpeg-status': return json(res, {...})      // 端点 10
    case '/api/build-info': return json(res, {...})         // 端点 11
    case '/api/plugins/container-extensions': return json(res,{...}) // 端点 12
    case '/api/file/text-preview-exts': return json(res,{...})      // 端点 13
    case '/api/alist-encrypt/decode-filename': ...          // 端点 14
    case '/api/alist-encrypt/stream': ...                   // 端点 15
    case '/api/index/rebuild': ...                          // 端点 16
    case '/api/index/clear': ...                            // 端点 17
    // ... 还有 POST/PUT/DELETE 变体 + 其他 handler
  }
}
```

#### Scenario: 正确的最小化 Mock 模式

```typescript
// ✅ 正确：仅保留 3-5 个核心端点
export function createMinimalHandlers(base: string) {
  const dispatchRequest = (req, res) => {
    const pathname = new URL(req.url).pathname

    // 核心端点 1: 健康检查（用于验证后端是否存活）
    if (pathname === '/health') {
      return json(res, { status: 'ok' })
    }

    // 核心端点 2: 基础配置（仅返回空结构，让前端自行处理默认值）
    if (pathname === '/api/config') {
      return json(res, {})
    }

    // 核心端点 3: 插件列表（使用真实数据文件）
    if (pathname === '/api/plugins') {
      const plugins = JSON.parse(fs.readFileSync('__mock_data__/plugins.json'))
      return json(res, { plugins })
    }

    // 所有其他请求 → 501 Not Implemented（强制走真实后端）
    res.statusCode = 501
    res.end(JSON.stringify({ error: 'use real backend', path: pathname }))
  }

  return { dispatchRequest }
}
```

### Requirement 2: 禁止前台阻塞式服务

**铁律**:
- ❌ **禁止**: 在终端前台直接运行 `go run ./cmd/encv/ serve` 或 `./encv-server`
- ❌ **禁止**: 任何会无限期 `block` 当前 shell 会话的命令
- ✅ **必须**: 使用后台运行模式（`&`, `nohup`, `tmux`, 或 IDE 独立终端）

#### Scenario: 错误的服务启动方式

```bash
# ❌ 错误：阻塞当前终端，无法执行其他命令
$ go run ./cmd/encv/ serve --port 2025
# 终端被占用，无法启动 Vite

# ❌ 错误：两个服务串行启动，第二个永远执行不到
$ go run ./cmd/encv/ serve --port 2025  # ← 卡在这里 forever
$ npx vite --port 5173                   # ← 永远不会执行
```

#### Scenario: 正确的后台启动方式

```bash
# ✅ 方式 1: Shell 后台运行
$ go run ./cmd/encv/ serve --port 2025 > /tmp/go-server.log 2>&1 &
$ echo "Go server PID: $!"
$ npx vite --port 5173 --host

# ✅ 方式 2: tmux 终端复用（推荐）
$ tmux new-session -d -s go-server 'go run ./cmd/encv/ serve --port 2025'
$ tmux attach -t go-server  # 可随时查看日志
# 另开一个终端运行 Vite

# ✅ 方式 3: IDE 多终端（VS Code / WebStorm）
# Terminal 1: go run ./cmd/encv/ serve --port 2025
# Terminal 2: npx vite --port 5173 --host
```

### Requirement 3: Go 程序直接运行规范

**铁律**:
- ✅ 开发环境：**必须** 使用 `go run ./cmd/encv/...` 直接运行
- ❌ **禁止**: `go build -o encv && ./encv` 两步法（除非生产部署）
- ✅ 生产环境：可使用编译后的二进制（提升启动速度）

#### Scenario: 开发环境标准流程

```bash
# ✅ 正确：开发环境一键运行
$ cd /workspace
$ go run ./cmd/encv/ serve --port 2025 &  # 自动编译+运行，利用缓存加速

# ❌ 错误：不必要的编译步骤
$ go build -o ./bin/encv ./cmd/encv/     # 浪费时间生成二进制
$ ./bin/encv serve --port 2025            # 且每次修改代码都要重新编译
```

### Requirement 4: 强制端口一致性（基于现有配置）

**当前已正确的配置**（来自 `vite.config.ts` 审计）：

| 服务 | 端口 | 配置位置 | 状态 |
|------|------|---------|------|
| **Go Backend API** | **2025** | `internal/server/server.go` | ✅ 已确认 |
| **Vite Dev Server** | **5173** | `app/encv-mobile/vite.config.ts:27` | ✅ 已确认 |
| **Proxy Target** | **127.0.0.1:2025** | `app/encv-mobile/vite.config.ts:31` | ✅ 已确认 |

**铁律**:
- ✅ 所有新代码必须使用上述端口
- ❌ **禁止**: 硬编码其他端口号（如 2026、8080、3000 等）
- ⚠️ **注意**: `mock/handlers.ts:278` 中有错误端口 `{ port: 2026 }`，需修正为 2025

#### Scenario: 端口冲突检测与修复

```bash
# ✅ 启动前检查端口占用
$ lsof -i :2025 || echo "Port 2025 is free"
$ lsof -i :5173 || echo "Port 5173 is free"

# 如果冲突，显示进程信息
$ lsof -i :2025
# COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME
# go       12345 user   3u  IPv4  12345      0t0  TCP *:2025 (LISTEN)

# 终止旧进程
$ kill 12345
```

### Requirement 5: Capacitor 预览标准化流程

**基于现有配置的正确启动顺序**：

```bash
# Step 1: 启动 Go 后端（后台，端口 2025）
cd /workspace
go run ./cmd/encv/ serve --port 2025 > /tmp/go-backend.log 2>&1 &
echo "✅ Go backend started on port 2025 (PID: $!)"

# Step 2: 启动 Vite 前端（新终端，端口 5173）
cd /workspace/app/encv-mobile
npx vite --port 5173 --host
# 输出: ➜  Local: http://localhost:5173/
#       ➜  Network: http://192.168.x.x:5173/

# Step 3: （可选） Capacitor 原生预览
npx cap sync android
npx cap open android  # 打开 Android Studio
# 或 iOS: npx cap open ios
```

**关键依赖关系**:
- Vite proxy 配置 (`vite.config.ts:30-62`) 会将 `/api/*` 转发到 `http://127.0.0.1:2025`
- 因此 **Go 后端必须先于 Vite 启动**
- WebSocket (`/ws`) 也通过 proxy 转发到 `ws://127.0.0.1:2025`

---

## MODIFIED Requirements

### Requirement: 清理 `mock/handlers.ts` 为最小化实现

**迁移方案**:

#### Phase 1: 移除过度 Mock（立即执行）

删除以下 handler 函数及其所有路由分支：

1. **`fileSystemHandler`** (94-193 行):
   - 删除: `/api/files` (GET/POST/PATCH/DELETE)
   - 删除: `/api/files/stream` (SSE)
   - 删除: `/api/files/plugin-stream` (SSE)
   - 删除: `/api/files/mkdir` (POST)
   - 删除: `/api/files/search` (GET) — 含递归目录遍历逻辑
   - 删除: `/api/files/exists` (GET)
   - 删除: `/api/files/encrypt-output-exists` (GET)
   - 删除: `/api/files/tags` (GET/POST/DELETE)

2. **`fileContentHandler`** (195-267 行):
   - 删除: `/api/file` (GET/POST/PATCH/DELETE)
   - 删除: `/api/file/rename` (POST/PATCH)
   - 删除: `/api/file/copy` (POST)
   - 删除: `/api/file/move` (POST)

3. **`staticJsonHandler`** 精简 (269-356 行):
   - **保留**: `/health` (1 个)
   - **保留**: `/api/config` (1 个，但返回空对象 `{}`)
   - **保留**: `/api/plugins` (1 个，从 JSON 文件读取)
   - **删除**: 其余 17+ 个端点（container-versions, schema, index/stats, remote/info, webdav/*, ffmpeg-status, build-info, file/text-preview-exts, alist-encrypt/*, index/rebuild, index/clear）

4. **`taskMockHandler`** (358-427 行):
   - **全部删除**: `/api/tasks/predict-plugin`, `/api/tasks` CRUD, cancel, retry

5. **`staticFileHandler`** (429-447 行):
   - **删除**: `/stream`, `/api/stream/external` 静态文件服务

6. **`debugControlHandler`** (449-481 行):
   - **全部删除**: `/__mock_control` 调试接口

7. **`dispatchRequest` 中的特殊路由** (509-612 行):
   - **删除**: `/decrypt` 处理
   - **删除**: `/api/file/info` 处理（含 container 类型推断逻辑）
   - **删除**: `/preview/*` PDF 预览 HTML 生成

#### Phase 2: 替换为最小化 Handler

替换后的 `mock/handlers.ts` 应 ≤ 50 行：

```typescript
import type { Connect } from 'vite'
import * as fs from 'fs'
import * as path from 'path'

const MOCK_DATA_ROOT = path.resolve(__dirname, '../__mock_data__')

function json(res: Connect.ServerResponse, data: unknown, status = 200): void {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json')
  res.end(JSON.stringify(data))
}

export function createHandlers(base: string): { dispatchRequest: Connect.NextHandleFunction } {
  const dispatchRequest: Connect.NextHandleFunction = (req, res) => {
    const url = new URL(req.url || '', `http://localhost${base}`)
    const pathname = url.pathname

    if (pathname === '/health') {
      return json(res, { status: 'ok' })
    }

    if (pathname === '/api/config') {
      return json(res, {}) // 返回空配置，前端使用默认值
    }

    if (pathname === '/api/plugins') {
      const pluginsPath = path.join(MOCK_DATA_ROOT, 'plugins.json')
      if (fs.existsSync(pluginsPath)) {
        const plugins = JSON.parse(fs.readFileSync(pluginsPath, 'utf-8'))
        return json(res, { plugins })
      }
      return json(res, { plugins: [] })
    }

    // 所有其他请求 → 501（强制走真实后端）
    res.statusCode = 501
    res.setHeader('Content-Type', 'application/json')
    res.end(JSON.stringify({ error: 'not implemented in mock', path: pathname }))
  }

  return { dispatchRequest }
}
```

#### Phase 3: 更新引用链

1. **`mock/index.ts`**: 移除对已删除函数的 import
2. **`mock/file-system.ts`**: 如果仅被 handlers.ts 使用，可考虑删除或标记为 deprecated
3. **`vite.config.ts`**: 无需修改（已正确配置 proxy fallback）

---

## REMOVED Requirements

无（这是新增规则和重构任务）

---

## Implementation Notes

### 文件变更清单

| 文件 | 操作 | 影响行数 |
|------|------|---------|
| `app/encv-mobile/mock/handlers.ts` | **重写** (620→50 行) | -570 行 |
| `app/encv-mobile/mock/index.ts` | 更新 import | ~10 行 |
| `app/encv-mobile/mock/file-system.ts` | 标记 deprecated 或删除 | ~100 行 |
| `.trae/rules/development.md` | 新建或追加铁律 | +80 行 |

### 验证标准

- [ ] `mock/handlers.ts` 总行数 < 100 行（含空行和注释）
- [ ] Mock API 端点数量 ≤ 3 个（health, config, plugins）
- [ ] `go run ./cmd/encv/ serve --port 2025` 能正常启动
- [ ] `npx vite --port 5173` 能正常启动并代理到 2025
- [ ] 前端页面能通过真实后端加载（非 mock 数据）
- [ ] 无硬编码的错误端口号（如 2026）
- [ ] Git 提交信息清晰说明："refactor: minimize mock handlers from 40+ to 3 endpoints"
