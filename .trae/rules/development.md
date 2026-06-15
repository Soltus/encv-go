# 开发环境铁律（来自实战踩坑）

> **核心原则：开发环境必须与生产环境保持一致的运行路径。**
> **Mock 是技术债务的源头——宁可多花 5 分钟启动真实后端，也不要花 2 天调试 mock 与真实 API 的行为差异。**

> **完整内容 + 8 章节实战案例**：[详情文档](../rule-library/development.md)

## 一、严禁 mock 大量 handle（违反 = 严重错误）

### 1.1 数量红线

| Mock 端点数量 | 判定 | 后果 |
|-------------|------|------|
| 2-5 个 | ✅ 允许 | 仅覆盖前端开发阻塞点 |
| 6-10 个 | ⚠️ 警告 | 必须附迁移计划 |
| **> 10 个** | **❌ 违规** | **立即重构或删除** |

### 1.2 禁止在 mock 中实现的逻辑

| 禁止 | 原因 |
|------|------|
| 文件搜索递归遍历 | 边界条件（symlink / 权限 / 深层嵌套）与真实 fs 差异巨大 |
| 任务状态机 | 异步时序、并发竞争、失败重试无法模拟 |
| 加密/解密流程 | 密码学操作必须在真实环境验证 |
| 插件安装/卸载生命周期 | ComboLite 类加载、签名校验无法伪造 |
| WebSocket 消息广播 | 连接管理、断线重连、消息顺序保证 |

**❌ 错误（15 handler 含完整业务逻辑）** vs **✅ 正确（3 个核心端点）** → [详情文档 §1.2](../rule-library/development.md#12-禁止实现的-mock-逻辑)

### 1.3 推荐替代方案

| 场景 | 替代方案 |
|------|---------|
| 需要 API 返回数据 | **测试数据 fixture 文件**（`test/fixtures/*.json`） |
| 验证前端渲染 | **真实后端 + 测试数据库** |
| 测试异常场景 | **后端注入故障模式**（`ENCV_FAULT_INJECTION=slow_api:500ms`） |
| 独立前端开发 | **Vite proxy 到真实后端**（见 §五） |

## 二、严禁阻塞式服务启动

**Go 后端 / Vite dev server 严禁在当前终端前台运行；必须在 PM2 → preview-gateway 链路内管理（详见 §5.2）。**

```bash
# ❌ 错误：所有"绕过 pm2"的启动方式 — 全部非法
$ go run ./cmd/encv/ serve          # 前台占终端
$ go run ./cmd/encv/ start          # 前台占终端
$ go run ./cmd/encv/ serve > /tmp/encv-backend.log 2>&1 &  # nohup / & 后台
$ nohup go run ./cmd/encv/ start > /tmp/encv-backend.log 2>&1 &
$ tmux new-session -d -s encv 'go run ./cmd/encv/ start'  # tmux 包装
$ vite                                       # dev-start-guard 拦截
$ npm run dev                                # dev-start-guard 拦截
$ pnpm exec vite                             # dev-start-guard 拦截
$ CI=true pnpm exec vite                     # 绕过意图明确，dev-start-guard 拦截（2026-06-15 收紧）
$ PPA_SPAWNED=1 pnpm exec vite               # 绕过意图明确，dev-start-guard 拦截（2026-06-15 收紧）
$ bash -c 'go run ./cmd/encv/ start'         # bash 包装同样非法
```

**✅ 唯一合法链路**（详见 §5.2）：

```bash
$ pm2 start /workspace/ecosystem.config.cjs
# → preview-gateway(:16666) spawn 子进程：
#   - air → encv-go(:2025)
#   - vite → encv-mobile(:8100, SPAWN_VITE=1)
#   - openlist (按需)
```

**后台进程管理命令**：`lsof -i :2025 -t` / `tail -f /tmp/encv-backend.log` / `kill $(lsof -i :2025 -t)`

## 三、Go 程序直接运行规范

| 方式 | 命令 | 适用 |
|------|------|------|
| **✅ go run** | `go run ./cmd/encv/ serve` | **日常开发**（编译+执行一步） |
| **❌ go build + 执行** | `go build -o encv && ./encv serve` | **仅生产部署 / CI** |

**禁止两步法的 4 个根因**：
1. 每次代码修改都要手动 build → 容易遗漏 → 运行旧代码
2. 项目根目录散落 `encv` 二进制 → 可能被 git 意外提交
3. `./encv` 在 PATH 中优先于系统命令 → 难以排查的"修改没生效"
4. `GOOS=android go build` 产生的二进制无法在桌面运行

## 四、端口必须正确

| 服务 | 端口 | 配置位置 |
|------|------|---------|
| **Go Backend API** | **2025** | Go 代码 `Serve()` / `--port` |
| **Vite Dev Server** | **5173** | `vite.config.ts` |
| **Proxy Target** | **127.0.0.1:2025** | `vite.config.ts` proxy |

**禁止**：硬编码其他端口 / `:0` 随机端口 / 修改 `config.user.json` / API base URL 写死非标准端口

**一键检查**：
```bash
for port in 2025 5173; do
  if lsof -i :$port -t >/dev/null 2>&1; then
    echo "⚠️  Port $port in use by PID $(lsof -i :$port -t)"
  else
    echo "✅ Port $port is free"
  fi
done
```

## 五、Capacitor 预览标准化流程

### 5.1 ⚠️ 本节已废弃 — 仅作历史背景

> **唯一合法启动方式：见 §5.2 Capacitor 预览专用一键脚本（PM2 → preview-gateway）。**
> 
> **5.1 描述的"通用方式"（go run / npm run dev / npx cap serve 等）已全部被 dev-start-guard + air 配置 + ecosystem.config.cjs 收紧，2026-06-15 不再可用。**
> 
> 历史背景（保留供排错参考）：
> 
> ```
> Step 1 ──→ Step 2 ──→ Step 3（可选）
> Backend     Frontend    Capacitor
> :2025       :5173       sync/preview
> ```
> 
> 旧 Step 1（已废弃）：`go run ./cmd/encv/ serve > /tmp/encv-backend.log 2>&1 &`
> 旧 Step 2（已废弃）：`npm run dev` 启动 Vite
> 旧 Step 3（已废弃）：`npx cap sync` / `npx cap open android` / `npx cap serve`
> 
> **以上三种"通用方式"在沙箱内一律被 dev-start-guard / air 拦截。请改走 §5.2 的 pm2 路线。**

### 5.2 Capacitor 预览专用一键脚本（`scripts/start-preview.sh`）

**核心铁律**：
- servingDir 永远为 `/storage/emulated/0` 绝对路径
- **严禁任何符号链接**（mock-data 真实目录就在 `/storage/emulated/0`）
- **严禁修改 `config.user.json`**
- **后端必须用 air 监视重载**（禁止 `go build` / `go run`）
- **严禁误杀 agent-tool-host**（沙箱基础设施在 16000 端口）

**沙箱端口身份**：

| 端口 | 进程 | 身份 |
|------|------|------|
| 16000 | agent-tool-host | 公网反向代理入口 |
| 5174/5175/... | Vite | 实际 dev server（端口漂移） |
| 2025 | encv（air 监视） | Go Backend |

**激活外部访问**：
```bash
# 脚本返回后必须调用 OpenPreview
OpenPreview(command_id="<id>", preview_url="http://localhost:5174/")
# 预览 URL 用 Vite 实际端口（5174），不是 5173
```

**完整 6 步脚本行为 + 排查表 + service-guard 根因清单** → [详情文档 §五](../rule-library/development.md#五capacitor-预览标准化流程)

## 六、WAF/代理截断路径参数（⚠️ 实战踩坑！）

> **核心原则：经过 WAF/反向代理的请求中，`@` 字符会被当作 URL authority 分隔符截断。**
> **所有路径参数必须使用双重编码（double encoding）穿越代理层。**

**症状**：`curl 同样请求 → 200 OK ✅`，但 `浏览器同样请求 → 404 ❌`

**根因**：`encodeURIComponent("@")` → `%40` → WAF 解码为 `@` → 截断后丢失后续字符

**修复（双重编码）**：
```
原始: special-chars-!@#$.txt
  → 第 1 次 encodeURIComponent: special-chars-!%40%23%24.txt
  → 第 2 次 encodeURIComponent: special-chars-!%2540%2523%2524.txt
  → WAF 解码外层: special-chars-!%40%23%24.txt  (@ 仍是 %40)
  → 后端 decodeURIComponent: special-chars-!@#$.txt ✅
```

**实现**：
- **前端**：[src/api/encv.ts](file:///workspace/app/encv-mobile/src/api/encv.ts) `proxySafeEncode()` = `encodeURIComponent(encodeURIComponent(v))`
- **后端**：[internal/utils/path.go](file:///workspace/internal/utils/path.go) `DecodePathParam()` = `url.QueryUnescape(QueryUnescape(raw))`
- **应用范围**（19 处替换）：所有将路径放入 query parameter 的 API 调用

> 完整 §六（mock 层同步更新 / 已知受影响字符 / 排查方法 / 测试覆盖）→ [详情文档 §六](../rule-library/development.md#六waf代理截断路径参数-实战踩坑)

## 七、Hi-Sillot-OpenList-Frontend fork 适配

> **核心问题：pnpm 锁定的 `solid-icons@1.2.0` 只有 `TbFillXxx` / `TbOutlineXxx` 前缀变体，但 fork 1.8+ 源码使用裸 `TbCheck` 导入 → SyntaxError → #root 永远不 mount。**

**修复方案**：vite plugin 通用 import 重写（`app/openlist/Hi-Sillot-OpenList-Frontend/vite-plugins/solid-icons-compat.ts`）：
```ts
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

> 完整 §七（命名映射表 + 验证脚本 + 兼容性说明）→ [详情文档 §七](../rule-library/development.md#七hi-sillot-openlist-frontend-fork-适配solid-icons-命名兼容-实战踩坑)

## 八、vite HMR WebSocket 噪音过滤

**症状**：16000 沙箱入口不支持 WebSocket Upgrade 协议 → `@vite/client` 每秒重试 → 污染 DevLogs。

**修复**：[src/composables/useFrontendLogs.ts](file:///workspace/app/encv-mobile/src/composables/useFrontendLogs.ts) `hijackConsole()` 加 `isHmrWsNoise` 过滤：
- 匹配 `failed to connect to websocket` + `WebSocket closed without opened`
- 命中后**降级为 debug 级别**记录，不丢信息、不污染 error 流

**不做"完全关 HMR"的原因**：沙箱是只读验证场景（HMR 不可用是已知限制），但本地直连 `localhost:5173` 仍依赖 HMR。

## 九、引用其他规则

- [project_rules.md](./project_rules.md) — Mobile Overlay 机制
- [verification-discipline.md](./verification-discipline.md) — 故障排查纪律

> 拆分：2026-06-11
