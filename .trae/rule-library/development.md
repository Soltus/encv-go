# 开发环境铁律（详情）

> **本文件为 [development.md](../rules/development.md) 的详情文档**。包含 1.2 详细 ❌/✅ mock 完整代码对比、5.2.1 start-preview.sh 完整铁律（沙箱端口身份、6 步脚本行为、service-guard 根因清单）、§六 WAF 双重编码完整 4 处后端实现 + 6.8 字符表、§七 solid-icons 完整 7.3 命名映射 + 7.5 验证脚本、§八 HMR 噪音过滤 isHmrWsNoise 完整代码。

---

## 一、严禁 mock 大量 handle

### 1.1 数量红线

| Mock 端点数量 | 判定 | 后果 |
|-------------|------|------|
| 2-5 个 | ✅ 允许 | 仅覆盖前端开发阻塞点（如登录态、基础配置） |
| 6-10 个 | ⚠️ 警告 | 必须附迁移计划，限期替换为真实 API |
| **> 10 个** | **❌ 违规** | **立即重构或删除** |

### 1.2 禁止实现的 mock 逻辑

**以下业务逻辑禁止在 mock 中实现**：

| 禁止 mock 的逻辑 | 原因 |
|-----------------|------|
| 文件搜索递归遍历 | 边界条件（符号链接、权限、深层嵌套）与真实文件系统差异巨大 |
| 任务状态机（pending→running→completed） | 异步时序、并发竞争、失败重试无法模拟 |
| 加密/解密流程 | 密码学操作的正确性必须在真实环境验证 |
| 插件安装/卸载生命周期 | ComboLite 的类加载、签名校验、资源合并无法伪造 |
| WebSocket 消息广播 | 连接管理、断线重连、消息顺序保证 |

**❌ 错误（过度 mock）**：
```typescript
// mock/server.ts — 15 个 handler，包含完整业务逻辑
app.get('/api/files/list', (req, res) => {
  const dir = req.query.path as string
  // ❌ 递归遍历模拟 — 与真实 fs 操作行为不一致
  const files = walkDirectory(dir, { recursive: true, followSymlinks: false })
  res.json({ files })
})

app.post('/api/tasks/create', (req, res) => {
  // ❌ 完整状态机模拟 — 无法复现真实的协程调度
  const task = { id: uuid(), status: 'running', progress: 0 }
  setTimeout(() => { task.status = 'completed' }, 3000)
  res.json(task)
})

app.get('/api/plugins/installed', (req, res) => {
  // ❌ 硬编码插件列表 — 新增插件后此处过时
  res.json([{ id: 'mpv-player', version: '1.0.0', enabled: true }])
})
```

**✅ 正确（最小 mock 集合）**：
```typescript
// mock/server.ts — 3 个核心端点，仅解决前端开发阻塞
app.get('/api/config/schema', (_req, res) => {
  // ✅ 静态 JSON 快照 — schema 变更频率极低
  res.json(require('./fixtures/config-schema.json'))
})

app.get('/api/auth/status', (_req, res) => {
  // ✅ 固定已登录状态 — 前端路由守卫需要
  res.json({ authenticated: true, user: { role: 'admin' } })
})

app.post('/api/health', (_req, res) => {
  // ✅ 心跳检测 — 前端断线重连需要
  res.json({ status: 'ok', timestamp: Date.now() })
})
```

### 1.3 推荐替代方案

| 场景 | 替代方案 | 示例 |
|------|---------|------|
| 需要 API 返回数据 | **测试数据 fixture 文件** | `test/fixtures/api-responses/*.json` |
| 需要验证前端渲染 | **真实后端 + 测试数据库** | `go run ./cmd/encv/ serve --config test-config.json` |
| 需要测试异常场景 | **后端注入故障模式** | 环境变量 `ENCV_FAULT_INJECTION=slow_api:500ms` |
| 需要独立前端开发 | **Vite proxy 到真实后端** | `vite.config.ts` proxy 配置（见 §五） |

---

## 二、严禁阻塞式服务启动

### 2.1 核心规则

**Go 后端服务必须在后台运行，不得占用当前终端。**

### 2.2 错误示例（阻塞终端）

```bash
# ❌ 直接前台运行 — 终端被占用，无法执行其他命令
$ go run ./cmd/encv/ serve
# → 终端输出日志流，Ctrl+C 才能退出
# → 无法在同一终端启动 Vite / Capacitor / 其他命令
```

### 2.3 正确示例（后台运行）

```bash
# ✅ 方式一：& 后台运行（最简单）
$ go run ./cmd/encv/ serve > /tmp/encv-backend.log 2>&1 &
$ echo $!  # 打印 PID，用于后续监控/终止

# ✅ 方式二：nohup（断开 SSH 后仍运行）
$ nohup go run ./cmd/encv/ serve > /tmp/encv-backend.log 2>&1 &
echo "Backend PID: $!"

# ✅ 方式三：tmux/screen（推荐长期开发会话）
$ tmux new-session -d -s encv 'go run ./cmd/encv/ serve'
# 查看：tmux attach -t encv
# 分离：Ctrl+B 然后 D

# ✅ 方式四：IDE 独立终端（VS Code / GoLand）
# 在 IDE 的 Terminal 面板中新建一个独立标签页运行
# IDE 终端之间互不阻塞
```

### 2.4 后台进程管理命令

```bash
# 查看后端是否在运行
lsof -i :2025 -t

# 查看日志（实时跟踪）
tail -f /tmp/encv-backend.log

# 停止后端
kill $(lsof -i :2025 -t)

# 强制停止（如果 kill 无效）
kill -9 $(lsof -i :2025 -t)
```

---

## 三、Go 程序直接运行规范

### 3.1 核心规则

**开发环境必须使用 `go run` 一键运行，禁止 `go build` + 手动执行两步法。**

### 3.2 对比

| 方式 | 命令 | 适用场景 |
|------|------|---------|
| **✅ go run** | `go run ./cmd/encv/ serve` | **日常开发**（编译+执行一步完成，支持热重载工具） |
| **❌ go build + 执行** | `go build -o encv && ./encv serve` | **仅生产部署 / CI 构建**（需要控制输出路径和交叉编译） |

### 3.3 为什么禁止两步法

1. **额外步骤**：每次代码修改都要手动重新 build → 容易遗漏 → 运行的是旧代码
2. **二进制残留**：项目根目录散落 `encv` 二进制文件 → 可能被 git 意外提交
3. **路径污染**：`./encv` 在 PATH 中优先于系统命令 → 难以排查的"为什么我的修改没生效"
4. **交叉编译陷阱**：`GOOS=android go build` 产生的二进制无法在桌面运行 → 浪费调试时间

### 3.4 正确用法示例

```bash
# ✅ 启动后端服务
go run ./cmd/encv/ serve

# ✅ 启动 CLI 工具（如一次性任务）
go run ./cmd/encv/ encrypt --input file.sccgv --output file.sccga

# ✅ 带环境变量
ENCV_CONFIG_PATH=./dev-config.json go run ./cmd/encv/ serve

# ✅ 带 build tags（移动端 stub 编译验证）
GOOS=android go run ./cmd/encv/ serve  # 验证 android tag 分支编译通过
```

---

## 四、端口必须正确（违反 = 服务全部失联）

### 4.1 端口分配表

| 服务 | 端口 | 用途 | 配置位置 |
|------|------|------|---------|
| **Go Backend API** | **2025** | HTTP REST API + WebSocket | Go 代码 `Serve()` 或 `--port` 标志 |
| **Vite Dev Server** | **5173** | 前端 HMR + 开发服务器 | `vite.config.ts` (默认) |
| **Proxy Target** | **127.0.0.1:2025** | Vite dev proxy 转发目标 | `vite.config.ts` proxy 配置 |

### 4.2 禁止行为

- ❌ 硬编码其他端口号（如 8080、3000、4000 等 Web 框架默认端口）
- ❌ 使用 `:0` 随机端口（导致 Vite proxy 无法配置固定 target）
- ❌ 在 `config.user.json` 中修改默认端口（见 project_rules.md 配置模板保护规则）
- ❌ 前端 API base URL 写死非标准端口

### 4.3 端口冲突检测

```bash
# 一键检查所有关键端口是否被占用
check_ports() {
  for port in 2025 5173; do
    if lsof -i :$port -t >/dev/null 2>&1; then
      echo "⚠️  Port $port is in use by PID $(lsof -i :$port -t)"
      lsof -i :$port -t | xargs ps -p -o pid,command= 2>/dev/null
    else
      echo "✅ Port $port is free"
    fi
  done
}
check_ports

# 快速查找占用进程
lsof -i :2025 -i :5173

# 杀掉冲突进程（谨慎使用）
kill $(lsof -i :2025 -t) 2>/dev/null
kill $(lsof -i :5173 -t) 2>/dev/null
```

### 4.4 Vite Proxy 配置（正确示例）

```typescript
// vite.config.ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,           // ✅ 标准 Vite 端口
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:2025',  // ✅ Go Backend 标准端口
        changeOrigin: true,
        ws: true,            // ✅ WebSocket 代理
      },
      '/ws': {
        target: 'ws://127.0.0.1:2025',  // ✅ WebSocket 直连
        ws: true,
      },
    },
  },
})
```

---

## 五、Capacitor 预览标准化流程

### 5.1 完整启动序列

```
Step 1 ──→ Step 2 ──→ Step 3（可选）
Backend     Frontend    Capacitor
:2025       :5173       sync/preview
```

#### Step 1：启动 Go 后端（后台，端口 2025）

```bash
# 进入项目根目录
cd /workspace

# 后台启动 Go 后端
go run ./cmd/encv/ serve > /tmp/encv-backend.log 2>&1 &

# 验证启动成功
sleep 2
curl -s http://127.0.0.1:2025/api/health | jq .
# 预期输出: {"status":"ok","timestamp":...}
```

#### Step 2：启动 Vite 前端（端口 5173）

```bash
# 同一目录下新开终端（或使用 tmux/tab）
npm run dev
# 或
npx vite

# 预期输出:
#   VITE v5.x.x  ready in xxx ms
#   ➜  Local:   http://localhost:5173/
#   ➜  Network: use --host to expose
```

#### Step 3（可选）：Capacitor 同步 / 预览

```bash
# 同步 web 资源到 native 项目
npx cap sync

# Android 预览（需 Android Studio / SDK）
npx cap open android

# iOS 预览（需 Xcode，macOS only）
npx cap open ios

# 浏览器预览（不需要 native 工具链）
npx cap serve
# 访问 http://localhost:3333 （Capacitor 内置预览服务器）
```

### 5.2 一键启动脚本（参考）

```bash
#!/bin/bash
# start-dev.sh — 开发环境一键启动

set -e

echo "=== ENCV Development Environment ==="

# Step 1: Backend
echo "[1/2] Starting Go backend on :2025 ..."
if lsof -i :2025 -t >/dev/null 2>&1; then
  echo "  ⚠️  Port 2025 already in use, skipping"
else
  go run ./cmd/encv/ serve > /tmp/encv-backend.log 2>&1 &
  BACKEND_PID=$!
  echo "  ✅ Backend started (PID: $BACKEND_PID)"
fi

# Wait for backend ready
for i in $(seq 1 10); do
  if curl -s http://127.0.0.1:2025/api/health >/dev/null 2>&1; then
    echo "  ✅ Backend health check passed"
    break
  fi
  sleep 0.5
done

# Step 2: Frontend
echo "[2/2] Starting Vite frontend on :5173 ..."
npm run dev &
FRONTEND_PID=$!
echo "  ✅ Frontend starting (PID: $FRONTEND_PID)"

echo ""
echo "=== Ready ==="
echo "  Frontend: http://localhost:5173"
echo "  Backend:  http://localhost:2025"
echo "  Logs:     tail -f /tmp/encv-backend.log"
echo "  Stop:     kill $(lsof -i :2025 -t) $(lsof -i :5173 -t)"
```

### 5.2.1 ⚠️ 本节已废弃 — 历史背景

> **唯一合法启动方式**：[workspace rules §5.2 Capacitor 预览专用一键脚本（`scripts/previews.sh`）](../rules/development.md#五capacitor-预览标准化流程)。
> 本节描述的 `app/encv-mobile/scripts/start-preview.sh` 已被 `scripts/previews.sh` 取代（2026-06-08 改造为 PM2 → preview-gateway 链路）。
> 历史背景保留供排错参考（如需找旧脚本，文件仍在 `app/encv-mobile/scripts/start-preview.sh`，但**不要在新工作中使用**）。

### 5.2.1 Capacitor 预览专用一键启动（`scripts/start-preview.sh`）⚠️ 已废弃

**适用场景**：浏览器沙箱预览、Capacitor 开发模式。脚本整合 mock 数据生成 + 后端 air 监视 + Vite 前端为一条命令。

**铁律**：
- servingDir 永远为 `/storage/emulated/0` 绝对路径（设计预期，脚本自建真实目录）
- **严禁任何符号链接**（mock-data 真实目录就在 `/storage/emulated/0`）
- **严禁修改 `config.user.json`**（保持 `mobile.server.dir` 为绝对 Android 路径）
- **后端必须用 air 监视重载**（禁止 `go build` / `go run`）
- **严禁误杀 agent-tool-host**（沙箱基础设施在 16000 端口，反向代理到 Vite）
- 脚本保持前台运行（便于 OpenPreview 激活），脚本退出时优雅停止所有子进程

**前置条件**：
- `air` 在 PATH 中（mise 安装的 Go 1.25.1 自带：`/root/.local/share/mise/installs/go/1.25.1/bin/air`）
- 前端依赖已安装：`cd app/encv-mobile && npm install`（脚本会自动检测并安装）

**使用方式**：
```bash
cd /workspace
bash app/encv-mobile/scripts/start-preview.sh
```

**脚本行为**：
1. Step 0: 杀掉残留的 air / encv / vite 进程（精确按进程名匹配 + `lsof -ti :2025` 按端口兜底，绝不杀 agent-tool-host）
2. Step 1: 确保 `node_modules` 就绪（缺失则自动 `pnpm install --no-frozen-lockline`，支持 workspace 成员）
3. Step 2: 运行 `npx tsx scripts/generate-mock-files.ts` 生成 mock 数据到 `/storage/emulated/0/01-plain-media/...`
4. Step 3: 启动 `ENCV_DEV_PREVIEW=1 air` 监视 `./cmd/encv/`（air 自动重建并重启后端）+ **verify `/api/service-guard` 返回 `servingDir=/storage/emulated/0`，否则 exit 1**
5. Step 4: 启动 Vite（`--host 0.0.0.0 --port 5174 --strictPort`）
6. Step 5: 启动 OpenList 前端 dev server (`:3000`, `OPENLIST_PREVIEW_BASE="/openlist-ui/"`) — 沙箱预览 OpenList UI 用
7. Step 6/7: 状态报告 + 保持前台 / 退出 detach

**激活外部访问**：
- 脚本返回 `command_id` 后，**必须调用 `OpenPreview(command_id="<id>", preview_url="http://localhost:5174/")`**
- **预览 URL 用 Vite 实际端口（5174）**，不是 5173（5173 在沙箱中无服务监听，agent-tool-host 实际占 16000）

**沙箱端口身份**（铁律）：

| 端口 | 进程 | 身份 |
|------|------|------|
| 16000 | agent-tool-host | 公网反向代理入口（`PREVIEW_PROXY_PUBLIC_PORT`） |
| 5174/5175/... | Vite | 实际 dev server（端口漂移，agent-tool-host 反代到此） |
| 2025 | encv（air 监视） | Go Backend |

**禁止做法**：
- ❌ `ln -sfn` 任何路径"桥接" mock-data（mock-data 实际位置就是 `/storage/emulated/0`）
- ❌ 修改 `config.user.json` 的 `mobile.server.dir`
- ❌ 使用 `go build` / `go run`（必须用 air 监视）
- ❌ `lsof -i :5173 | xargs kill`（会误杀 agent-tool-host 沙箱基础设施）
- ❌ 跳过 mock 生成直接启动（会触发 service guard）

### 5.3 常见问题排查

| 症状 | 可能原因 | 排查命令 |
|------|---------|---------|
| 前端 API 请求 404/502 | 后端未启动或端口错误 | `curl http://127.0.0.1:2025/api/health` |
| 前端 API 请求跨域 CORS 错误 | Vite proxy 未配置或 target 端口错误 | 检查 `vite.config.ts` proxy.target |
| WebSocket 连接失败 | proxy 未开启 `ws: true` 或后端未监听 `/ws` | `curl -i -N -H "Connection: Upgrade" http://127.0.0.1:2025/ws` |
| Capacitor 预览空白页 | 未执行 `cap sync` 或 web 资源过期 | `npx cap sync && npx cap serve` |
| 端口被占用 | 上次开发会话未清理 | `lsof -i :2025 -i :5173` + `kill` |
| 修改 Go 代码后前端无变化 | 后端是旧进程（go run 不会自动重启） | 重启后端：先 kill 旧进程再 `go run` |
| Vite HMR 不生效 | 文件保存事件未触发（某些远程文件系统） | 触发一次 touch 或重启 Vite |
| **service-guard BLOCKED：`server.dir missing "01-plain-media"`，列出 `.md` 文件** | **mobile overlay 未生效**：`server.dir` 留在默认的 `/` → 解析为 `/workspace` → 看到 workspace 根目录文件。常见原因：手工 `tmp/encv start` 没设 `ENCV_DEV_PREVIEW=1`，或 start-preview.sh Step 0 没杀掉残留进程 | `curl -s http://127.0.0.1:2025/api/service-guard` 看 `servingDir` 字段；应该是 `/storage/emulated/0` 而不是 `/` 或 `/workspace` |

### 5.3.1 ⚠️ service-guard 失败根因清单（2026-06-04 实战踩坑）

> **`/api/service-guard` 报告 `server.dir missing "01-plain-media"` 是 mobile overlay 没生效的标志**。
> 任何路径下看到此错误，都按"mobile overlay 未触发"处理，不要去改 `config.user.json`。

| 触发场景 | 根因 | 修复 |
|----------|------|------|
| 手工启动 `tmp/encv start` / `tmp/encv` | 缺 `ENCV_DEV_PREVIEW=1` 环境变量，config overlay 不被加载 | 用 `start-preview.sh` 启动；或手工起时 `export ENCV_DEV_PREVIEW=1` |
| `start-preview.sh` 启动后服务 guard 失败 | Step 0 漏杀旧 `tmp/encv` 进程（`pkill -f '^./tmp/encv'` 不匹配 `/workspace/tmp/encv start`） | 2026-06-04 修复：Step 0 改用 `lsof -ti :2025` 按端口兜底杀进程；Step 3 启动 air 后必须 verify `/api/service-guard` 的 `servingDir == /storage/emulated/0`，否则 `exit 1` |
| mock-data 不在 `/storage/emulated/0/01-plain-media` | 跳过了 `npx tsx scripts/generate-mock-files.ts` 步骤 | 重跑 start-preview.sh（Step 2 自动生成） |
| 改了 `config.user.json` 的 `server.dir` 或 `mobile.server.dir` | 违反 start-preview.sh §5.2.1 铁律 | `git checkout config.user.json` 还原 |

### 5.4 开发环境健康检查

```bash
#!/bin/bash
# check-dev.sh — 快速诊断开发环境状态

echo "=== Development Environment Health Check ==="
echo ""

# Backend
if curl -sf http://127.0.0.1:2025/api/health >/dev/null 2>&1; then
  echo "✅ Backend (:2025) — healthy"
else
  echo "❌ Backend (:2025) — not responding"
fi

# Frontend
if curl -sf http://127.0.0.1:5173 >/dev/null 2>&1; then
  echo "✅ Frontend (:5173) — running"
else
  echo "⚠️  Frontend (:5173) — not detected (may be on different port)"
fi

# Proxy connectivity
if curl -sf http://127.0.0.1:5173/api/health >/dev/null 2>&1; then
  echo "✅ Vite Proxy → Backend — connected"
else
  echo "⚠️  Vite Proxy → Backend — cannot verify (frontend may not be running)"
fi

echo ""
echo "All ports:"
lsof -i :2025 -i :5173 2>/dev/null || echo "  (no processes found)"
```

---

## 六、WAF/代理截断路径参数（⚠️ 实战踩坑！）

> **核心原则：经过 WAF/反向代理的请求中，`@` 字符会被当作 URL authority 分隔符截断。**
> **所有路径参数必须使用双重编码（double encoding）穿越代理层。**

### 6.1 症状

```
用户操作：点击文件 `special-chars-!@#$%^&*()_+.txt` 预览
预期行为：前端 fetch /decrypt?file=... → 后端返回文件内容
实际行为：HTTP 404 — file not found

关键矛盾：
  curl 同样请求 → 200 OK ✅
  浏览器同样请求 → 404 ❌
```

### 6.2 根因链路

```
前端: encodeURIComponent("special-chars-!@#$%^&*()_+.txt")
  → "special-chars-!%40%23%24%25%5E%26*()_%2B.txt"
    ↓ 发送到 Vite dev server

Vite proxy 转发到后端（正常）
  ↓

用户浏览器环境: Android WebView / com.xunlei.browser（迅雷浏览器）
  带有大量 WAF/proxy header:
    x-alb-waf-requestid, x-clb-cluster, x-envoy-external-address, ...
  ↓

WAF/中间代理处理 query string:
  发现 %40 → 解码为 @ → 当作 URL authority 分隔符
  → 截断 @ 之后的所有字符
  → 实际到达后端的 filePath = "special-chars-!" （不完整！）
    ↓

后端: 在 mock_data 目录查找 "special-chars-!.txt" → 404 Not Found
```

**证据**（mock 层 404 响应体中的 debug 信息）：

```json
{
  "error": "file not found",
  "debug": {
    "receivedFilePath": "/04-boundary-test/special-chars-!@",
    "resolvedAbsPath": "/workspace/app/encv-mobile/__mock_data__/04-boundary-test/special-chars-!@",
    "siblings": ["special-chars-!@#$%^&*()_+.txt", ...]
  }
}
```

`siblings` 列表中存在完整文件名，但 `receivedFilePath` 在 `@` 处被截断。

### 6.3 修复方案：双重编码（Double Encoding）

**原理**：编码两次，WAF 只解码外层一次，内层 `%40` 安全通过。

```
原始路径: special-chars-!@#$%^&*()_+.txt
  ↓ 第 1 次 encodeURIComponent
单层编码: special-chars-!%40%23%24%25%5E%26*()_%2B.txt
  ↓ 第 2 次 encodeURIComponent（proxySafeEncode）
双重编码: special-chars-!%2540%2523%2524%25255E%2526*()_%252B.txt
  ↓ WAF/代理解码外层
WAF 输出: special-chars-!%40%23%24%25%5E%26*()_%2B.txt  (@ 仍是 %40！)
  ↓ 后端 decodeURIComponent（第二次解码）
最终结果: special-chars-!@#$%^&*()_+.txt  ✅ 完整恢复
```

### 6.4 实现细节

#### 前端（TypeScript）— `proxySafeEncode()`

```typescript
// src/api/encv.ts
export function proxySafeEncode(value: string): string {
  return encodeURIComponent(encodeURIComponent(value))
}
```

**应用范围**（19 处替换）：所有将路径放入 query parameter 的 API 调用。

| 文件 | 替换数 | 涉及端点 |
|------|--------|---------|
| [api/encv.ts](app/encv-mobile/src/api/encv.ts) | 14 | listFiles, stream, plugin-stream, deleteFile, readFileContent, checkFileExists, getFileStreamUrl, getFilePreviewUrl, getExternalStreamUrl, listFilesByTag, getAlistEncryptStreamUrl |
| [views/FilePreview.vue](app/encv-mobile/src/views/FilePreview.vue) | 2 | decrypt, api/file/info |
| [views/FileInfo.vue](app/encv-mobile/src/views/FileInfo.vue) | 1 | api/file/info |

#### Go 后端 — `DecodePathParam()`

```go
// internal/utils/path.go
func DecodePathParam(raw string) string {
    s, err := url.QueryUnescape(raw)
    if err != nil { return raw }
    s2, err := url.QueryUnescape(s)
    if err != nil { return s }
    return s2
}
```

**应用范围**（4 处）：

| 文件 | 函数 | 端点 |
|------|------|------|
| [server_handle.go](internal/server/server_handle.go) | handleStreamRequest | /stream?path= /stream?file= |
| [openlist_handlers.go](internal/server/openlist_handlers.go) | handler | /openlist/sites/:siteId/decrypt?file= |
| [openlist_middleware.go](internal/server/openlist_middleware.go) | OpenlistSiteMiddleware | /openlist/sites/:siteId/decrypt?file= |

#### Mock 层同步更新

```typescript
// app/encv-mobile/mock/handlers.ts
let filePath = url.searchParams.get('file') || url.searchParams.get('path') || ''
try { filePath = decodeURIComponent(filePath) } catch {}
```

### 6.5 为什么只对 path 参数双重编码

| 参数类型 | 是否需要双重编码 | 原因 |
|---------|-----------------|------|
| **path / file**（文件路径） | **✅ 必须** | 用户可控，可能包含 `@#` 等特殊字符 |
| password（加密密码） | 可选 | 通常不含特殊字符，但建议保持一致 |
| tag（标签名） | 可选 | 通常为 ASCII 字母数字 |
| extensions（扩展名列表） | 不需要 | 固定格式 `.ext1,.ext2` |

### 6.6 测试覆盖

| 测试文件 | 用例数 | 覆盖场景 |
|---------|--------|---------|
| [path_test.go](internal/utils/path_test.go) | 24 (15+8+1) | DecodePathParam 双重解码 + RoundTrip 编码往返验证 |
| [proxy-safe-encode.test.ts](app/encv-mobile/__tests__/proxy-safe-encode.test.ts) | 8 | proxySafeEncode 双重编码 + Unicode + 特殊字符 + 空值 |

### 6.7 排查此类问题的诊断方法

当出现「curl 正常但浏览器失败」的矛盾时：

1. **Mock 层拦截**：在 Vite middleware 中直接处理请求，打印 `req.url` 和解析后的参数
2. **响应体带 debug 信息**：404 时返回 `{ error, debug: { receivedFilePath, resolvedAbsPath, siblings } }`
3. **前端错误详情按钮**：FilePreview.vue 的 Show Details 展开响应体 JSON
4. **对比法**：同一 URL 用 curl 和浏览器分别测试，对比差异

### 6.8 已知受影响的特殊字符

以下字符在 URL query string 中有特殊含义，必须被正确编码：

| 字符 | URL 含义 | 单层编码 | 双重编码后 WAF 解码结果 |
|------|---------|----------|---------------------|
| `@` | **authority 分隔符** ⚠️ | `%40` | `%40`（安全） |
| `#` | fragment 分隔符 | `%23` | `%23`（安全） |
| `$` | 无特殊含义 | `%24` | `%24`（安全） |
| `%` | 编码前缀 | `%25` | `%25`（安全） |
| `^` | 无特殊含义 | `%5E` | `%5E`（安全） |
| `&` | query 分隔符 | `%26` | `%26`（安全） |
| `+` | 空格替代 | `%2B` | `%2B`（安全） |
| `()` | 无特殊含义 | 不编码 | `()`（安全） |
| `!` | 无特殊含义 | 不编码 | `!`（安全） |

**其中 `@` 是唯一确认会被迅雷浏览器/WAF 截断的字符。** 其他字符虽然理论上也可能被某些代理误处理，但双重编码方案统一保护了所有特殊字符。

---

## 七、Hi-Sillot-OpenList-Frontend fork 适配：solid-icons 命名兼容（⚠️ 实战踩坑！）

### 7.1 症状

打开 `/openlist-ui/` 时，#root 一直空、只有注入的「返回 ENCV」按钮可见。debug pane 抓到：

```
[err] SyntaxError: The requested module '.../solid-icons@1.2.0_.../solid-icons/tb/index.js' does not provide an export named 'TbCheck' @ .../FolderTree.tsx:7:15
```

页面整体 JS module graph 解析失败 → Solid App 永远不 mount。

### 7.2 根因

| 维度 | 说明 |
|------|------|
| **fork 的源码约定** | `solid-icons` 1.8+：`import { TbCheck, TbX, TbFile } from "solid-icons/tb"`（无前缀 = 填充变体） |
| **本工作区实际安装** | pnpm 锁定 `solid-icons@1.2.0`，**只有** `TbFillXxx` / `TbOutlineXxx` 前缀变体 |
| **类型签名** | fork 的 `node_modules/.../tb/index.d.ts` 不会缺（npm 把上游 d.ts 一并装下来），所以 TS 编译过 |
| **运行时** | 浏览器 ESM 解析真实 JS module 时发现导出列表对不上 → throw SyntaxError → 整个 module graph 中断 |

### 7.3 命名映射

| fork 写的（1.8+） | 1.2.0 实际提供 |
|------------------|---------------|
| `TbCheck` | `TbFillCircleCheck` / `TbOutlineCheck` |
| `TbX` | `TbOutlineX` |
| `TbFile` | `TbFillFile` / `TbOutlineFile` |
| `TbFolder` | `TbFillFolder` / `TbOutlineFolder` |
| `TbArchive` / `TbRefresh` / `TbCopy` / `TbLink` / `TbSelector` / `TbPlus` / `TbCheckbox` / `TbExternalLink` / `TbFileArrowRight` | `TbOutline${Xxx}` 全部存在 |

> 其他包（`bi` / `ai` / `io` / `ri` / `cg` / `fa` / `fi` / `bs` / `im` / `si`）在 1.2.0 里都齐全，不受影响。

### 7.4 修复方案：vite plugin 通用 import 重写

**文件**：`app/openlist/Hi-Sillot-OpenList-Frontend/vite-plugins/solid-icons-compat.ts`

```ts
const TB_IMPORT_RE =
  /import\s*\{\s*([^}]+?)\s*\}\s*from\s*(["'])solid-icons\/tb\2/g

// 裸 Tb* → 改写为 TbOutlineXxx as TbXxx
// 已带 Fill/Outline 前缀 → 保持
```

**接入位置**：`vite.config.ts` 必须在 `solidPlugin()` **之前**，`enforce: "pre"`：

```ts
plugins: [
  solidIconsCompat(),  // ← 必须最先
  solidPlugin(),
  ...
]
```

### 7.5 验证

```bash
# 1. 重启 openlist vite
pkill -f 'Hi-Sillot-OpenList-Frontend.*vite'
cd app/openlist/Hi-Sillot-OpenList-Frontend
setsid nohup env OPENLIST_PREVIEW_BASE="/openlist-ui/" OPENLIST_NO_HMR=1 \
  pnpm dev --host 127.0.0.1 --port 3000 --strictPort \
  </dev/null >/tmp/encv-openlist.log 2>&1 &

# 2. 验证重写生效
curl -s 'http://127.0.0.1:3000/openlist-ui/src/components/FolderTree.tsx' \
  | grep 'solid-icons/tb'
# 期望：import { TbOutlineX as TbX, TbOutlineCheck as TbCheck } from "..."

# 3. 浏览器打开 /openlist-ui/，debug pane 应该消失、#root 应该 mount
```

### 7.6 兼容性

- 如果未来 `solid-icons` 升到 ≥ 1.8，本插件变成 no-op（重写后的名字在 1.8+ 也都存在）
- TS 类型可能仍报缺导出，但 fork 是 .tsx + vite-plugin-solid，类型不参与运行
- 不影响 production build：plugin 走 vite.transform，prod 也生效，但 prod 永远命中"已是正确前缀"分支

---

## 八、vite HMR WebSocket 噪音过滤（16000 沙箱预览专用）

### 8.1 症状

encv-mobile 预览控制台持续刷：
```
[vite] failed to connect to websocket (Error: WebSocket closed without opened.)
    at Object.connect (/@vite/client:892:13)
```

不影响应用运行（HMR 是开发辅助，非核心功能），但会污染 DevLogs。

### 8.2 根因

`16000` 沙箱入口（agent-tool-host）不支持 WebSocket Upgrade 协议。`@vite/client` 启动时尝试连 `ws://.../{__hmr__token__}`，连接被代理中断，浏览器每秒重试一次。

### 8.3 修复

`app/encv-mobile/src/composables/useFrontendLogs.ts` 的 `hijackConsole()` 增加 `isHmrWsNoise` 过滤：

```ts
console.error = (...args) => {
  saved.error(...args)  // 原生 console.error 仍输出到 DevTools
  if (isHmrWsNoise(args)) {
    addLog('debug', ['[HMR WS sandbox noise] ' + args[0]])
    return
  }
  addLog('error', args)
}
```

`isHmrWsNoise` 匹配 `failed to connect to websocket` 和 `WebSocket closed without opened`。命中后**降级为 debug 级别**记录，不丢信息、不污染 error 流。

### 8.4 为什么不做"完全关 HMR"

| 做法 | 优劣 |
|------|------|
| 完全关 HMR（`hmr: false`） | 噪音彻底消失，但本地直连 `localhost:5173` 也失去 HMR |
| **只过滤日志（推荐）** | **DevLogs 干净 + 本地直连 HMR 仍可用** |

沙箱预览是只读验证场景，HMR 不可用是已知限制；本地开发仍依赖 HMR。
