# preview-management 详情

> 本文件为 [preview-management.md](../rules/preview-management.md) 的详情文档。
>
> 索引位于 [`.trae/rules/preview-management.md`](../rules/preview-management.md)（170 行）。本文件汇总索引未包含的实战案例、详细根因链路、完整命令清单与故障排查表。

---

## 一、四大反模式

### 反模式 A：`sleep N` (N>60s) 阻塞会话

**症状**：用 `RunCommand blocking=true` 跑 `sleep 86400` / `tail -f xxx` / `node server.js` 占位 → 工具阻塞等命令完成 → 浪费 token 配额。

**禁止**：
- ❌ `sleep 86400`、`sleep 999999`、`sleep infinity` —— 任何 > 60s 的 sleep 都不允许出现在 blocking 命令中
- ❌ `tail -f /var/log/xxx` 阻塞等待
- ❌ `node server.js` 阻塞运行（无 daemon 化）
- ❌ **`python3 -c "..."` / `node -e "..."` 跑交互式协议测试**（如 WS upgrade）

**正确做法**：
- ✅ `pm2 start xxx.js --name xxx` 守护（fork 模式，立刻 daemon 化返回）
- ✅ `pm2 logs <name>` 短时查看（30s 内 kill）
- ✅ **协议探测走持久化脚本**（写到 `scripts/`，下次直接 `bash scripts/probe-ws.sh`）

### 反模式 B：`nohup xxx > /tmp/log 2>&1 &` 启动后台进程

**症状**：bash 父进程退出后，子进程变孤儿由 init 收养 → 无人监管、死了无人重启、日志散落、端口冲突无法溯源。

**禁止**：
- ❌ `nohup node server.js > /tmp/x.log 2>&1 &`
- ❌ `nohup ./node_modules/.bin/vite ... &`
- ❌ `setsid xxx &`

**正确做法**：
- ✅ 全部走 `pm2 start <script> --name <name>`，自带日志文件 + 自动重启 + 内存监控

### 反模式 C：阻塞 + web_server 类型假装常驻（2026-06-10 已废止）

**历史根因（已废止）**：之前认为"OpenPreview 工具需要 web_server 类型命令作为 command_id 来源"。

**2026-06-10 真相**：OpenPreview 工具本身就是端口注册入口，**不需要**任何阻塞 web_server 进程作为"来源"。`command_id` 只需取**任一已运行** RunCommand 的 id，工具不强制要求 web_server 类型。

**禁止**：
- ❌ `while true; do curl ...; sleep 5; done` (blocking, command_type=web_server) 当作"OpenPreview 来源"
- ❌ `node server.js` (blocking, command_type=web_server) 把 RunCommand 自身当 server

### 反模式 D：`pnpm build` + `pnpm preview` 制造孤儿前端（2026-06-07 mobile mock preset 验证事故）

**症状**：擅自 `pnpm build` + `pnpm preview --port 4173` → vite preview 是**纯静态服务**，不走 vite dev 插件链（@ionic/vue / Capacitor polyfill / HMR）→ 前端能打开但**调不到任何 `/api/`** → "无错白屏 + 工具调用失败"。

**根因（连环误判）**：
1. 错把 `pnpm build` 当成"重建前端"标准做法 — 实际 `build` 产物是给 **Android 离线包**用的
2. 错把 vite `preview` 当成"预览链接"通用方案 — vite preview 是纯静态服务，**绕开项目管控链路**

**禁止**：
- ❌ `cd app/encv-mobile && pnpm build`（除非用户明确说"打 Android 包 / Capacitor sync"）
- ❌ `cd app/encv-mobile && pnpm preview`（vite 自带静态预览，绕开项目管控）
- ❌ 任何 `vite --port 4xxx` / `vite preview --port 4xxx`（非 8100 端口都脱离项目管控）
- ❌ 任何 `npx serve dist` / `python3 -m http.server dist` / `npx http-server dist`

**正确做法（用户说"重建前端 / 给我预览链接"时）**：
- ✅ **不** build：vite dev (8100) 跑源码 + HMR
- ✅ **不**启新进程：preview-gateway (16666) 已在 pm2 守护
- ✅ Preview 链接直接用：**http://localhost:16666/**
- ✅ 调 `OpenPreview(command_id=<任一已运行 RunCommand id>, preview_url="http://localhost:16666/")`

---

## 二、pm2 联动启动标准流程（方案 C：网关合一，2026-06-08 大改）

### 2.1 沙箱 dev 服务拓扑

**pm2 监管 2 个 app**（统一入口）：

| pm2 app | 端口 | 必备 | 角色 |
|---------|------|------|------|
| `preview-gateway` | :16666 | ✅ | 统一预览网关，**唯一对外入口 + 唯一进程管理者** |
| `openpreview-stub` | :15003 | ✅ | OpenPreview web_server command_id 源 |

**gateway 内部子进程**（由 `preview-gateway` 内部 `child_process.spawn` 管理，**不需独立 pm2 app**）：

| 子进程 | 端口 | 默认 | 角色 | 开关 env |
|--------|------|------|------|----------|
| `encv-go` (air) | :2025 | ✅ 启用 | Go 后端（mobile overlay 关键） | `SPAWN_GO=0` 关闭 |
| `encv-mobile-vite` | :8100 | ✅ 启用 | 主 app Vite（被 :16666/ 代理） | `SPAWN_VITE=0` 关闭 |
| `plugin-openlist-vite` | :5174 | ❌ 按需 | OpenList 管理 UI Vite | `SPAWN_PLUGIN_VITE=1` 启用 |
| `openlist` | :5244 | ❌ 按需 | OpenList 真实 fork Go 服务 | `SPAWN_OPENLIST=1` 启用 |

**为什么只有 2 个 pm2 app**：子进程死 → gateway 死 → pm2 重启整套（避免"vite 死、Go 活、gateway 200、用户白屏"鬼状态）

### 2.2 启动标准命令（**只此一条**）

```bash
which pm2 || npm install -g pm2              # 装 pm2（一次性）
pm2 start /workspace/ecosystem.config.cjs    # 一行启动全部
pm2 list                                     # 看到 2 个 online
curl -s http://localhost:16666/__gateway/health | jq .ok   # true
curl -sI http://localhost:16666/                          # HTTP/1.1 200 OK
curl -s http://localhost:16666/api/service-guard | jq '.context.envDevPreview'  # true
```

**按需启用 plugin-vite / openlist**：
```bash
SPAWN_PLUGIN_VITE=1 pm2 restart preview-gateway
SPAWN_OPENLIST=1    pm2 restart preview-gateway
```

### 2.3 完整 pm2 命令参考

| 场景 | 命令 |
|------|------|
| 查看状态 | `pm2 list` / `pm2 show preview-gateway` |
| 查看日志 | `pm2 logs preview-gateway --lines 100` / `pm2 logs encv-go --lines 50` |
| 实时跟踪（30s 内 kill） | `pm2 logs preview-gateway` （**不**用 blocking `tail -f`） |
| 重启 | `pm2 restart preview-gateway` / `pm2 reload preview-gateway`（零停机） |
| 停止 | `pm2 stop preview-gateway` |
| 持久化 | `pm2 save`（开机/重启自动恢复） |
| 重启沙箱后恢复 | `pm2 resurrect` |
| 清理僵尸 | `pm2 kill && pm2 start /workspace/ecosystem.config.cjs` |

**`optionalDown` 含义**：在 ecosystem.config.cjs 中标记为按需启动的 app，pm2 启动时**不自动拉起**，需要时手动 `pm2 start <name>` 或带 env 重启。

---

## 三、OpenPreview 激活（拿到外网链接）

### 3.1 原理

```
外网用户浏览器
  ↓
:16000 (agent-tool-host)  ← 唯一外网入口
  ↓ 内部 preview-proxy
:16666 (preview-gateway)   ← 沙箱内统一入口
  ↓
:8100 (encv-mobile-vite)   ← 主 app
```

**首次访问 :16666** → agent-tool-host 内部 `:80` register 端点 `requires_auth=true`，**普通 HTTP 请求 401 拒绝**。只有 `OpenPreview` 工具能完成 register。

### 3.2 标准激活流程（2026-06-10 重写：零阻塞）

```bash
# 1. 确认沙箱内端口在线
pm2 list                              # 2 个 online
curl -sI http://localhost:16666/      # 200 OK
curl -s http://localhost:16666/__gateway/health | jq .ok  # true

# 2. 直接调 OpenPreview 工具（零阻塞）
OpenPreview(
  command_id="<任一已运行 RunCommand 的 id>",
  preview_url="http://localhost:16666/"
)

# 3. 验证
curl -sI http://127.0.0.1:16000/      # 期望 200（不再 400）
```

**为什么不需要阻塞进程**：OpenPreview 工具是 IDE 提供的**注册工具**，调用即在 agent-tool-host 内部建立路由，不依赖任何"长跑命令占位"。

### 3.3 OpenPreview 浏览器下的服务器状态检测

> trae 反代 `:16000` **不支持 WebSocket upgrade**（实测：WS handshake → 502）。
> OpenPreview 浏览器下用 `new WebSocket('wss://...')` → 1006 异常关闭 → 误显"离线"。

**已修复**（`a8c4e7d`）：
- **[useWebSocket.ts](file:///workspace/app/encv-mobile/src/composables/useWebSocket.ts) `connect()`**：sandbox 浏览器下不连 WS
- **[useServerStatus.ts](file:///workspace/app/encv-mobile/src/composables/useServerStatus.ts)**：新增 `transportMode` / `latencyMs` / `isSandboxBrowser`
- **[ServerDetail.vue](file:///workspace/app/encv-mobile/src/views/ServerDetail.vue)**：状态 ion-item 美化

**3 档调试链路**：
1. **OpenPreview 浏览器** —— 仅 fetch /api/*，实时功能不可用
2. **沙箱本地 `:16666`** —— 完整 API + WS
3. **APK 真机 + adb reverse** —— 完整 + 真实性能

### 3.4 mock 浏览器约束（沙箱架构硬约束）

> **用户通过 OpenPreview 拿到的预览浏览器是 agent-tool-host 提供的"模拟浏览器"**——沙箱架构硬约束，**无法升级为完整 Chrome DevTools**。

| 能/不能 | 详情 |
|---------|------|
| ✅ 能刷新页面 / 查看 console 日志 | DevLogs tab 可见 |
| ❌ **没有完整 DevTools** | 不能装 React/Vue Devtools |
| ❌ **没有 Network 面板** | 看不到 fetch / CORS 错（**最痛**） |

### 3.5 错误模式表

| 错误现象 | 根因 | 修复 |
|---------|------|------|
| `HTTP 401` 首次访问 :16666 | agent-tool-host `:80` register 端点未 register | 调 `OpenPreview(command_id=..., preview_url=...)` 完成 register |
| `port already registered` | 同一端口被前一次 OpenPreview 占用 | 先调 `pm2 restart` 或重启沙箱 |
| `HTTP 400` 调 OpenPreview 后 | preview_url 不匹配沙箱内端口 | 确保 `preview_url` 走 `localhost:16666` |
| `HTTP 502` WS 握手 | trae 反代不支持 WS upgrade | 见 §3.3 已修复 |
| `WebSocket 1006` 异常关闭 | 同上，OpenPreview 浏览器下不连 WS | `a8c4e7d` 已修复 |

---

## 四、禁止命令清单（速查）

| 模式 | 反例 | 替代 |
|------|------|------|
| `sleep N` (N>60s) blocking | `sleep 86400` | `pm2 start` 后立刻返回 |
| `tail -f xxx` blocking | `tail -f /tmp/x.log` | `pm2 logs xxx --lines 100` |
| `nohup xxx &` | `nohup node s.js &` | `pm2 start s.js --name xxx` |
| `setsid xxx` | `setsid vite &` | `pm2 start vite --name xxx` |
| `node server.js` blocking | 直接跑 node | `pm2 start server.js --name xxx` |
| 任何 `&` 启后台 | `cmd &` | `pm2 start` |
| `while true; curl; sleep` | OpenPreview 来源占位 | **直接调 OpenPreview 工具** |
| **`pnpm build`**（误当作 dev 预览） | `pnpm build` | vite dev (8100) 跑源码 + HMR |
| **`pnpm preview`** | `pnpm preview --port 4173` | preview-gateway (16666) → vite dev (8100) |
| **`npx serve dist` / `http-server dist`** | 绕开 API 反代 | preview-gateway 是唯一入口 |

---

## 五、env 注入铁律（2026-06-05 mobile overlay 触发失败事故后写入）

> **核心原则：`ApplyMobileOverlay` 由 `ENCV_MOBILE=1` 或 `ENCV_DEV_PREVIEW=1` 触发，缺失则 servingDir 退回 `/workspace`（用户看到 `.md` / `.gitignore`，不是 mock 媒体）。**

### 5.1 三层注入（缺一不可）

| 层 | 文件 | 作用 |
|----|------|------|
| **L1 pm2 → gateway** | `ecosystem.config.cjs` `preview-gateway` 块 `env` | pm2 fork 时注入到 gateway Node 进程 |
| **L2 gateway → air 子进程** | `app/preview-gateway/src/server.ts` `buildChildSpecs()` | gateway `child_process.spawn` air 时透传 env |
| **L3 air → encv 传递** | `.air-run.sh` `export ${X:-1}` 兜底 | air rebuild 重启 `./tmp/encv` 时不会丢 env |

**数据流**：
```
ecosystem.config.cjs (L1: ENCV_DEV_PREVIEW=1)
  → pm2 start → preview-gateway 进程 (Node, process.env)
  → buildChildSpecs() spread process.env + defaults (L2)
  → air 子进程 (bash, env in air shell)
  → .air-run.sh (export ${X:-1} 兜底 L3)
  → exec ./tmp/encv start (Go, env in os.Getenv)
  → config.Load() → ApplyMobileOverlay 触发
```

**为什么 L2 显式 spread 不省略**：`process.env` 在 Node 里有但**不会自动**被子进程继承 — 必须用 `spawn(cmd, args, { env: ... })` 显式传递。

### 5.2 自检命令

```bash
curl -s http://localhost:16666/api/service-guard | jq '.context.envDevPreview'  # true
curl -s http://localhost:16666/api/service-guard | jq '.context.servingDir'    # /storage/emulated/0
ls /storage/emulated/0/01-plain-media/ | head                                  # mock 数据落地
curl -s http://localhost:16666/__gateway/health | jq '.children[].name'        # encv-go + encv-mobile-vite
```

**自检失败排查表**：

| 自检结果 | 根因 | 修复 |
|---------|------|------|
| `envDevPreview: false` | L1 未设 / L2 spread 漏 | 检查 `ecosystem.config.cjs` + `buildChildSpecs()` |
| `servingDir: /workspace` | `ApplyMobileOverlay` 未触发 | 检查 `ENCV_*` env 链 |
| `__gateway/health.children = []` | 子进程未起 | `pm2 restart preview-gateway` |
| mock 数据未落地 | 模拟数据生成器未跑 | 调 `curl :16666/api/mock/reset`（或启动时自动跑） |

### 5.3 绝对禁止

- ❌ 移除 `.air-run.sh` 的 `export ${X:-1}` 兜底（air rebuild 丢 env）
- ❌ 在 `ecosystem.config.cjs` `preview-gateway` 块 env 里**不设** `ENCV_DEV_PREVIEW` / `ENCV_MOBILE`
- ❌ 在 `src/children.ts` `buildChildSpecs` 里删掉 `{ ...process.env, ENCV_*: ... }` 显式 spread
- ❌ 让 `./tmp/encv` 直接以 pm2 启动，绕过 air 监视
- ❌ 复活 start-preview.sh 里的 inline env 注入（方案 C 已删）

---

## 六、强制自检清单

每次启动 dev 服务前必须确认：

- [ ] pm2 已装（`which pm2`）—— 未装则 `npm install -g pm2`
- [ ] ecosystem.config.cjs 已存在 + `preview-gateway/dist/server.js` 已构建
- [ ] 启动命令是 `pm2 start /workspace/ecosystem.config.cjs`，**不是** `nohup` / `&` / `sleep`
- [ ] `pm2 list` 看到 **2 个** app `online`（preview-gateway + openpreview-stub）
- [ ] `curl :16666/__gateway/health | jq .ok` = **true**（必检 upstream 全 alive）
- [ ] `curl :16666/` 返回 200
- [ ] `curl :16666/api/service-guard | jq .context.envDevPreview` = **true**
- [ ] `curl :16666/api/service-guard | jq .context.servingDir` = **/storage/emulated/0**
- [ ] `pm2 save` 持久化

**当用户说"重建前端 / 给我预览链接 / dev server 起一下"时**：

- [ ] 我**没有**在用 `pnpm build` 吗？
- [ ] 我**没有**在用 `pnpm preview` / `vite preview` / `npx serve dist` 吗？
- [ ] 我要启的端口在 §二 拓扑表吗？
- [ ] `pm2 list` 显示都 online 吗？
- [ ] 如果都已 online，我**根本不需要启任何进程** — 直接给链接 http://localhost:16666/
- [ ] OpenPreview 调用过了吗？

---

## 七、go run 沙箱路径（2026-06-10 切换）

> **结论**：go run **完全可行**，关键在 tee 解决「编译沉默」+ zombie killer 全方位。

```toml
# .air.toml
[build]
  pre_cmd = ["mkdir -p tmp && go build -o ./tmp/encv-go-check ./cmd/encv 2>&1 | tee ./tmp/encv-go-build.log; true"]
  cmd = "go run ./cmd/encv start 2>&1 | tee ./tmp/encv-go-run.log"
  bin = "./tmp/encv start"
  delay = 5000         # 5s 防 pipe 死锁误判
  grace_delay = 10000  # 10s 冷编空间
```

| 问题 | 根因 | 解决 |
|------|------|------|
| **go run 编译沉默** | Go 编译期 stdout/stderr 被吞 | `tee ./tmp/encv-go-run.log` |
| **沙箱冷编 5+ 分钟** | go mod 40+ 模块 + 沙箱 CPU 慢 | `delay=5000` + `readyTimeoutMs=600000` |
| **encv 启动需 start 子命令** | `./tmp/encv` 裸跑 → help 后 exit 0 | `go run ./cmd/encv start` |
| **Zombie 累积** | pm2 SIGKILL → air/go run orphan | `kill-orphan-children.sh` 14 步清理 |

---

## 八、沙箱 Zombie 强杀（14 步 + 报告）

> **沙箱特有**：`init(1)` 不主动 reap orphan + go build/run 经常卡 Sl 状态（pipe 死锁，0% CPU 永远不醒）。

```bash
bash /workspace/scripts/kill-orphan-children.sh         # 静默
bash /workspace/scripts/kill-orphan-children.sh --report # 报告残留
```

**14 步覆盖**：
1. pm2 全杀
2. air / air_sh 残留
3. go build 残留
4. go run 残留
5. go test 残留
6. Go compile worker（compile-commands）
7. encv 进程（含 `./tmp/encv`）
8. vite 全家（encv-mobile-vite / plugin-openlist-vite）
9. preview-gateway 残留
10. openlist 残留
11. PPID=1 的孤儿进程
12. Sl 状态 0% CPU 进程
13. Go 工具链（gopls / dlv）
14. **stat=Z 真 zombie**（不可杀，记录上报 init）+ pnpm 清理 + 二次扫描

**集成点**：
- `setup-sandbox-env.sh` 步骤 pre-0 + 末尾 --report
- `previews.sh` restart / kill
- 用户手动 debug

---

## 九、验证实测

```bash
# 启动前
bash /workspace/scripts/kill-orphan-children.sh --report | tee /tmp/orphan-pre.log

# 启动
pm2 start /workspace/ecosystem.config.cjs
sleep 30
bash /workspace/scripts/kill-orphan-children.sh --report | tee /tmp/orphan-post.log

# diff 应为空
diff /tmp/orphan-pre.log /tmp/orphan-post.log
```

---

## 十、DOM 锚定教训（2026-06-10 用户痛批后写入）

> **核心原则：用户发的 DOM 节点自带完整属性（class / slot / 子元素 / 文本），先全字段匹配再下手；不要只看 class 名推断。**

### 10.1 反模式：靠"语义猜"找文件

**症状**：用户发 `ion-item` 包含 `<h3>状态</h3>` + `<ion-badge color="danger">离线</ion-badge>` → 直觉以为是 `Settings.vue` → 但实际是 `ServerDetail.vue`（详情二级页面）。

**根因**：
- 用户发的 DOM 节点是**正在显示的**视图，需要找的是**当前**路由匹配的组件
- 只看 "settings.xxx" 翻译键字符串（i18n 复用）→ 误以为是 Settings.vue 顶层

### 10.2 DOM 锚定 checklist

- [ ] **路由推断**：当前页 URL path → 对应 vue 组件
- [ ] **唯一 class / slot 锚定**：`class="server-controls"` / `slot="end"` 等独有属性
- [ ] **完整文本锚定**：`<h3>状态</h3>` + 兄弟节点（不能只看 h3）
- [ ] **不要靠** `t('settings.xxx')` 推断组件位置

### 10.3 修复本案例的查找手法

```bash
# 1. 锚定独有 class / slot
grep -rln "server-controls" /workspace/app/encv-mobile/src/views/   # ServerDetail.vue
grep -rln "connection-error" /workspace/app/encv-mobile/src/      # ServerDetail.vue + useServerStatus.ts

# 2. 锚定完整文本
grep -rln "状态.*h3\|<h3>.*状态" /workspace/app/encv-mobile/src/views/  # ServerDetail.vue

# 3. 路由路径推断
grep -rln "tabs/settings/server\b" /workspace/app/encv-mobile/src/router/  # ServerDetail 路由
```

### 10.4 历史踩坑案例（3 条 2026-06-10 实战）

| # | 症状 | 错误猜测 | 真实文件 | 锚定要素 |
|---|------|----------|---------|---------|
| 1 | `<h3>状态</h3>` + `ion-badge color="danger"` | `Settings.vue` | `ServerDetail.vue` | 路由 `tabs/settings/server/:id` + `class="server-controls"` |
| 2 | `<ion-item>` 含"连接超时" | `Connection.vue` | `useServerStatus.ts`（composable） | 文件路径 = composable 而非 component |
| 3 | `<div class="queue-list">` 含"待处理" | `Tasks.vue` | `Tasks.vue`（猜对，但**差点猜错**） | 实际是 Tasks.vue 内的 `<QueueList>` 子组件 |

### 10.5 绝对不能做

- ❌ **只**根据 i18n key 字符串（`t('settings.xxx')`）推断组件位置
- ❌ **只**根据 h1/h2/h3 文本节点推断
- ❌ 跳过路由推断直接猜
- ❌ 在没找到唯一 class/slot 锚定前就修改代码

---

## 十一、相关 spec 文档

- [plan-fix-sandbox-preview-env-injection.md](file:///workspace/.trae/documents/plan-fix-sandbox-preview-env-injection.md) — env 注入专项 spec
- [preview-management-pm2-pipeline.md](file:///workspace/.trae/documents/) — pm2 pipeline 详细
- [backend-debug-and-dev-preview-plan.md](file:///workspace/.trae/documents/backend-debug-and-dev-preview-plan.md) — dev preview 整体规划

> 拆分：2026-06-11
