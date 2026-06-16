# 沙箱预览服务管理铁律

> **核心原则：所有 dev/preview 进程必须由 pm2 监管，禁止在工具调用中用 blocking / nohup & / sleep 长跑方式启动。**
> **任何"等用户访问"的端口都必须先 pm2 守护，工具调用立即返回。**

> **完整内容 + 10 章节实战案例**：[详情文档](../rule-library/preview-management.md)

---

## 一、三大反模式

| 反模式 | 症状 | 根因 | 正确做法 |
|--------|------|------|----------|
| **A** `sleep N` (N>60s) blocking | 工具阻塞等命令完成 | 长 sleep 占满 token 配额 | `pm2 start xxx.js --name xxx` 守护 |
| **B** `nohup xxx > /tmp/log 2>&1 &` | bash 退出 → 子进程变孤儿 init 收养 | 无人监管、日志散落 | 全部走 `pm2 start` |
| **C** 阻塞 + `web_server` 类型假装常驻 | 浪费 1 个 web_server command_id | OpenPreview 工具**本身**就是注册入口 | 直接调 OpenPreview 工具即可 |
| **D** `pnpm build` + `pnpm preview` | "无错白屏 + 调不到 `/api/`" | vite preview 是纯静态服务，绕开管控链路 | preview-gateway (16666) → vite dev (8100) |

**禁止速查**：`sleep N>60s` / `tail -f` / `nohup &` / `setsid` / 任何 `&` / `pnpm build` / `pnpm preview` / `npx serve dist` / `http-server dist`

> 完整反模式详细（含反模式 D 2026-06-07 4 步连环误判实战）→ [详情文档 §一](../rule-library/preview-management.md#一四大反模式)

---

## 二、pm2 启动标准流程（方案 C：网关合一）

**pm2 监管 2 个 app**（统一入口）：

| pm2 app | 端口 | 角色 |
|---------|------|------|
| `preview-gateway` | :16666 | 统一预览网关，**唯一对外入口** |
| `openpreview-stub` | :15003 | OpenPreview web_server command_id 源 |

**gateway 内部子进程**（由 gateway 内部 `child_process.spawn` 管理，**不需独立 pm2 app**）：

| 子进程 | 端口 | 默认 | 开关 env |
|--------|------|------|----------|
| `encv-go` (air) | :2025 | ✅ | `SPAWN_GO=0` |
| `encv-mobile-vite` | :8100 | ✅ | `SPAWN_VITE=0` |
| `plugin-openlist-vite` | :5174 | ❌ | `SPAWN_PLUGIN_VITE=1` |
| `openlist` | :5244 | ❌ | `SPAWN_OPENLIST=1` |

**为什么只有 2 个 pm2 app**：子进程死 → gateway 死 → pm2 重启整套（避免"vite 死、Go 活、gateway 200、用户白屏"鬼状态）

**启动标准命令**（**只此一条**）：
```bash
which pm2 || npm install -g pm2
pm2 start /workspace/ecosystem.config.cjs
pm2 list                              # 2 个 online
curl -s :16666/__gateway/health | jq .ok   # true
curl -sI :16666/                               # 200
curl -s :16666/api/service-guard | jq '.context.envDevPreview'   # true
```

> 完整 pm2 命令参考（logs / save / resurrect / reload）+ optionalDown 含义 → [详情文档 §二](../rule-library/preview-management.md#二pm2-联动启动标准流程)

---

## 三、OpenPreview 激活

**原理**（4 层）：

```
外网用户 → :16000 (agent-tool-host) → :16666 (preview-gateway) → :8100 (encv-mobile-vite)
```

**首次访问 :16666** → agent-tool-host 内部 `:80` register 端点 `requires_auth=true`，**普通 HTTP 请求 401 拒绝**。只有 `OpenPreview` 工具能完成 register。

**标准激活流程**（2026-06-10 重写：零阻塞）：

```bash
# 1. 确认端口在线
pm2 list && curl -sI :16666/        # 200 OK

# 2. 直接调 OpenPreview 工具（零阻塞）
OpenPreview(command_id="<任一已运行 RunCommand 的 id>", preview_url="http://localhost:16666/")

# 3. 验证外网入口
curl -sI http://127.0.0.1:16000/    # 200（不再 400）
```

**WS 兼容性警告**：trae 反代 `:16000` **不支持 WebSocket upgrade**（WS → 502）。OpenPreview 浏览器下用 `new WebSocket('wss://...')` → 1006 异常关闭 → 误显"离线"。`a8c4e7d` 已修复（sandbox 浏览器不连 WS）。

**3 档调试链路**：①OpenPreview 浏览器（仅 fetch /api/*）②沙箱本地 `:16666`（完整 API+WS）③APK 真机 + adb reverse（完整 + 真实性能）

> 错误模式表（401 / port already registered / 400 / 502 / WebSocket error）+ DOM 锚定教训 → [详情文档 §三/§十](../rule-library/preview-management.md#三openpreview-激活)

---

## 四、env 注入铁律（3 层缺一不可）

> `ApplyMobileOverlay` 由 `ENCV_MOBILE=1` 或 `ENCV_DEV_PREVIEW=1` 触发，缺失则 servingDir 退回 `/workspace`（用户看到 `.md`/`.gitignore`）。

| 层 | 文件 | 作用 |
|----|------|------|
| **L1** pm2 → gateway | `ecosystem.config.cjs` `preview-gateway` 块 `env` | pm2 fork 注入到 gateway Node 进程 |
| **L2** gateway → air | `app/preview-gateway/src/server.ts` `buildChildSpecs()` | spawn 时显式 spread env（**process.env 不会自动继承**） |
| **L3** air → encv | `.air-run.sh` `export ${X:-1}` 兜底 | air rebuild 重启 `./tmp/encv` 不丢 env |

**自检**：
```bash
curl -s :16666/api/service-guard | jq '.context.envDevPreview'   # true
curl -s :16666/api/service-guard | jq '.context.servingDir'     # /storage/emulated/0
```

**绝对禁止**：移除 L3 兜底 / 不设 L1 `ENCV_*` env / 删 L2 显式 spread / 复活 previews.sh inline env 注入

> 完整数据流 + 自检失败排查表 → [详情文档 §五](../rule-library/preview-management.md#五env-注入铁律)

---

## 五、go run 沙箱路径

```toml
# .air.toml
[build]
  pre_cmd = ["mkdir -p tmp && go build -o ./tmp/encv-go-check ./cmd/encv 2>&1 | tee ./tmp/encv-go-build.log; true"]
  cmd = "go run ./cmd/encv start 2>&1 | tee ./tmp/encv-go-run.log"
  bin = "./tmp/encv start"
  delay = 5000
  grace_delay = 10000
```

| 问题 | 解决 |
|------|------|
| go run 编译沉默 | `tee ./tmp/encv-go-run.log` |
| 沙箱冷编 5+ 分钟 | `delay=5000` + `readyTimeoutMs=600000` |
| `./tmp/encv` 裸跑 help 后 exit 0 | `go run ./cmd/encv start` |
| Zombie 累积 | `bash /workspace/scripts/kill-orphan-children.sh`（14 步清理） |

---

## 六、强制自检清单

启动 dev 服务前必检：pm2 已装 / `pm2 start` 一行 / `pm2 list` 2 个 online / `:16666/__gateway/health` ok / `:16666/` 200 / `envDevPreview=true` / `servingDir=/storage/emulated/0` / `pm2 save`。

用户说"重建前端 / 给我预览链接"时：❌不用 `pnpm build` / ❌不用 `pnpm preview` / ✅端口在 §二 拓扑表 / ✅`pm2 list` 都 online / ✅**根本不需要启任何进程** — 直接给链接 `http://localhost:16666/` / ✅OpenPreview 调用过了。

---

## 七、DOM 锚定教训

> 用户发的 DOM 节点自带完整属性（class / slot / 子元素 / 文本），**先全字段匹配再下手**；不要只看 class 名推断。

- 锚定**路由** → 推断组件
- 锚定**唯一 class / slot**（`server-controls` / `slot="end"`）
- 锚定**完整文本**（`<h3>状态</h3>` + 兄弟节点）
- **不要靠** `t('settings.xxx')` 推断位置

```bash
grep -rln "server-controls" /workspace/app/encv-mobile/src/views/   # ServerDetail.vue
grep -rln "状态.*h3\|<h3>.*状态" /workspace/app/encv-mobile/src/views/
```

> 3 条 2026-06-10 实战踩坑 + 完整查找手法 → [详情文档 §十](../rule-library/preview-management.md#十dom-锚定教训)

---

## 八、相关文件

| 文件 | 作用 |
|------|------|
| [/workspace/ecosystem.config.cjs](file:///workspace/ecosystem.config.cjs) | pm2 配置（4 主 + 3 辅） |
| [/workspace/.air.toml](file:///workspace/.air.toml) | air 配置（go run + tee log） |
| [/workspace/scripts/previews.sh](file:///workspace/scripts/previews.sh) | pm2 启停包装 |
| [/workspace/scripts/kill-orphan-children.sh](file:///workspace/scripts/kill-orphan-children.sh) | 沙箱 zombie 强杀 14 步 |
| [/workspace/app/preview-gateway/README.md](file:///workspace/app/preview-gateway/README.md) | 网关 + 路由 + 健康检查 |
| [/workspace/internal/config/config.go](file:///workspace/internal/config/config.go) | `ApplyMobileOverlay`（L292-294） |

> 拆分：2026-06-11
