# Plan: 撤销 /openlist-spa subpath 代理，OpenList 回到原始环境 /

> **用户反馈（核心）**：
> - `/openlist-spa/` 是之前为 ENCV iframe 嵌入加的路径前缀 hack，**不可靠**
> - OpenList 必须跑在**原始环境**（`http://127.0.0.1:5244/`），不能再做 subpath 路由改造
> - 沙箱 dev 用 `bash scripts/dev-openlist.sh` 启真实 OpenList fork（Hi-Sillot）on 5244
> - 保留 `/openlist-ui-proxy` 作为主 encv-mobile Vite (8100) 的开发辅助（独立使用场景）

---

## 一、Current State（基于 Phase 1 探索）

### 1.1 三套 subpath 代理叠加（产生混乱的根源）

| # | 文件 | proxy 路径 | 上游 | 服务对象 | 状态 |
|---|------|-----------|------|---------|------|
| **A** | `app/encv-mobile/vite.config.ts:33-152` | `/openlist-ui/*` | `http://127.0.0.1:5244` (sirv dist + api rewrite) | encv-mobile 主 Vite (8100) | **保留**（开发辅助） |
| **B** | `app/encv-mobile/plugin-openlist/web/vite.config.ts:97-105` | `/openlist-spa/*` | `http://127.0.0.1:5244` (rewrite strip prefix) | plugin-openlist Vite (5174) | **撤销** |
| **C** | `app/encv-mobile/plugin-openlist/web/vite.config.ts:32-73` | `/__openlist-health` | 自定义中间件 Node 直连 5244 | plugin-openlist Vite (5174) | **撤销** |

### 1.2 引用关系

| 文件 | 引用 | 用途 |
|------|------|------|
| `app/encv-mobile/plugin-openlist/web/src/views/OpenListWebView.vue:206-219` | `isSandbox ? '/openlist-spa/...' : 'http://127.0.0.1:5244/...'` | iframe URL 分支 |
| `app/encv-mobile/plugin-openlist/web/src/views/OpenListWebView.vue:325-340, 360` | `fetch('/__openlist-health', ...)` | 后端探活 |
| `app/encv-mobile/plugin-openlist/web/vite.config.ts:6-23` | 注释说明 /openlist-spa 设计 | — |
| `app/encv-mobile/scripts/dev-openlist-web.sh:5-18` | 注释 "不依赖 OpenList(5244)，不依赖 Android WebView" | — |

### 1.3 OpenList 真实环境

- `app/openlist/Hi-Sillot-OpenList/`（fork）+ `public/dist/`（前端 dist，~5MB）
- `bash scripts/dev-openlist.sh` 启动：自动下 dist（本地 or 远程） + 写 `config.json` 启用 `dist_dir` + `go run . --data ./data`
- 服务于 `http://127.0.0.1:5244/`
- CORS: `internal/conf/config.go:222` 默认 `AllowOrigins: ["*"]` → 跨域 iframe + fetch 无障碍
- SPA 内部 axios baseURL 是相对路径，**不需要** subpath 注入

### 1.4 已对齐的 prod 模式（参考）

- Android WebView 加载 `file:///android_asset/openlist/index.html`（构建期打入 plugin APK assets）
- `LocalOpenListStatusCard.vue:196` 已经是 `window.open('http://127.0.0.1:${port}/#/login', '_system')` 直访 5244
- 真机上 plugin-openlist Content() 内部 WebView 也是直访 127.0.0.1:5244
- prod 路径**没有 subpath 改造** — 是 OpenList 原始环境

---

## 二、Proposed Changes

### 2.1 撤销 `plugin-openlist/web/vite.config.ts` 中的 subpath 代理

**文件**：[vite.config.ts](file:///workspace/app/encv-mobile/plugin-openlist/web/vite.config.ts)

**改动**：
- 删除 `openlistHealthPlugin()` 函数定义（lines 25-73）
- 删除 `plugins: [vue(), openlistHealthPlugin()]` 中的 `openlistHealthPlugin()`（line 79）
- 删除 `server.proxy: { '/openlist-spa': ... }` 整个 block（lines 97-105）
- 顶部注释（lines 5-23）改写：移除"Proxy 设计"小节，改为说明本 Vite 仅服务 plugin 管理 UI，OpenList Web UI 通过直访 127.0.0.1:5244 加载

**Why**：
- subpath 代理引入了 HTML 改写、base_path 注入、path rewrite 等多处 hack
- 真实 fork 已经在 5244 root path 跑通（CORS=*），直访即可
- 撤销后 plugin Vite 职责单一：只服务 plugin 管理 UI（OpenListHome / OpenListSettings / OpenListConfigEditor / OpenListWebView）

**How（diff 形态）**：

```typescript
// 删
function openlistHealthPlugin(): Plugin { ... }

// 顶部注释改写
/**
 * plugin-openlist/web Vite 配置
 *
 * 职责单一：仅服务 plugin 管理 UI（OpenListHome / Settings / ConfigEditor / WebView）
 *
 * 不再做 OpenList 后端代理（撤销原因：subpath 改造不可靠，
 * 沙箱 dev 直接通过 http://127.0.0.1:5244/ 访问 OpenList 真实前端，
 * 与 prod 模式对齐——OpenList 跑在原始环境 /）。
 *
 * 沙箱 dev 启动 OpenList 后端：
 *   Terminal 2: bash scripts/dev-openlist.sh
 *
 * 沙箱 dev 启动本 Vite（plugin 管理 UI）：
 *   Terminal 3: bash scripts/dev-openlist-web.sh
 *
 * Production（Android WebView）：
 *   - WebView 加载 file:///android_asset/openlist/index.html（plugin-openlist/src/main/assets/openlist/）
 *   - iframe 内部直访 http://127.0.0.1:5244/（与本机 OpenList 进程同设备）
 */
```

```typescript
// 删 server.proxy 整段
// server: {
//   port: 5174,
//   strictPort: false,
//   proxy: {
//     '/openlist-spa': { ... }
//   }
// }

// 改为
server: {
  port: 5174,
  strictPort: false,
},
```

### 2.2 简化 `OpenListWebView.vue` 的 iframe URL 和探活

**文件**：[OpenListWebView.vue](file:///workspace/app/encv-mobile/plugin-openlist/web/src/views/OpenListWebView.vue)

**改动**：
- 删除 `isSandbox` computed（line 211）— 不再有 dev/prod 区分
- 简化 `iframeUrl` computed（lines 213-219）— 始终为 `http://127.0.0.1:${port || 5244}/#/login`
- 删除 `checkHealth()` 函数中的 `/__openlist-health` 调用（lines 325-340）— 改为直访 `${baseUrl}/api/public/settings`（CORS 允许）
- 简化 `probeBackend()`（lines 348-393）— 用 `fetch(${baseUrl}/api/public/settings, { signal })` 替代 `/__openlist-health`
- 注释（lines 206-210）改写：移除 "Vite proxy /openlist-spa" 字样
- `reload()` 函数（lines 440-455）— 简化，直接重新加载 iframe src
- `openExternal()` 函数（lines 457-460）— 已经是直访 5244，无需改
- `copyCommand()` 中的提示文案（line 87, 463）— 改为 `bash scripts/dev-openlist.sh`

**Why**：
- 与 `LocalOpenListStatusCard.vue:196` 已有的 prod 直访模式对齐
- 不再依赖 `/__openlist-health`（删除后无上游依赖）
- 探活改用 OpenList 自身的 `/api/public/settings` 端点（CORS=*，最权威）

**How（关键 diff）**：

```typescript
// 删
const isSandbox = computed(() => import.meta.env.DEV)

const iframeUrl = computed(() => {
  const hash = '#/login'
  if (isSandbox.value) {
    return `/openlist-spa/${hash}`
  }
  return `http://127.0.0.1:${port.value || 5244}/${hash}`
})

// 改为
const iframeUrl = computed(() =>
  `http://127.0.0.1:${port.value || 5244}/#/login`
)
```

```typescript
// 删 checkHealth() 中的 Vite middleware 调用
async function checkHealth(): Promise<HealthResult> {
  try {
    const res = await fetch('/__openlist-health', { cache: 'no-store' })
    ...
  }
}

// 改为（直访 OpenList 自身端点）
async function checkHealth(): Promise<HealthResult> {
  const base = `http://127.0.0.1:${port.value || 5244}`
  const target = `${base}/api/public/settings`
  const start = Date.now()
  try {
    const res = await fetch(target, { cache: 'no-store' })
    const elapsed = Date.now() - start
    return {
      alive: res.ok,
      upstreamStatus: res.status,
      latency: elapsed,
      ts: elapsed,
    }
  } catch (e: any) {
    return {
      alive: false,
      error: e?.message || String(e),
      code: e?.cause?.code || e?.code,
      ts: Date.now(),
    }
  }
}
```

> ⚠️ 注意：直访 fetch 会有 CORS preflight（GET 不一定有，HEAD 不一定），OpenList 默认 `AllowOrigins: ["*"]` 应能放行。若浏览器拒绝，加 `mode: 'cors'`（默认就是）并观察 Network 面板。

### 2.3 更新 `dev-openlist-web.sh` 文档注释

**文件**：[dev-openlist-web.sh](file:///workspace/app/encv-mobile/scripts/dev-openlist-web.sh)

**改动**：
- 顶部 "重要区别" 注释（lines 5-18）改写：
  - 明确本脚本**只服务 plugin 管理 UI**（5174 端口）
  - 删去 "不依赖 OpenList(5244)" 的措辞 — 改为 "不代理 OpenList(5244)，iframe 直访"
  - 增加 "若要测 OpenList Web UI，请另起 terminal 跑 `bash scripts/dev-openlist.sh`"

**Why**：让脚本使用者清楚知道：plugin Vite (5174) 和 OpenList 后端 (5244) 是**两个独立进程**，分别启动。

### 2.4 不变的部分（明确范围）

- **保留** `app/encv-mobile/vite.config.ts:33-152` 的 `/openlist-ui-proxy` middleware — 主 encv-mobile Vite (8100) 的开发辅助，与 plugin 侧独立
- **保留** `LocalOpenListStatusCard.vue:196` 的 `127.0.0.1:5244/#/login` 直访
- **保留** `scripts/dev-openlist.sh` 启动真实 OpenList fork 的流程
- **保留** `scripts/build-plugin-openlist-web.sh` 的 build + assets 同步流程（prod 不变）
- **保留** 根 vite.config.ts 中的 proxy `/openlist/` → encv-go(2025)（encv-go 反代 OpenList 的 runtime 路径，不属于 subpath 改造）

---

## 三、Assumptions & Decisions

| # | 假设/决策 | 取值 | 理由 |
|---|---------|------|------|
| **D1** | 沙箱浏览器能直访 `http://127.0.0.1:5244` | 假设成立 | sandbox 默认网络允许 loopback；prod 模式已验证同模式工作 |
| **D2** | OpenList CORS `AllowOrigins: ["*"]` 在 dev 直访下放行 | 假设成立 | Hi-Sillot fork `internal/conf/config.go:222` 默认值；如不放行，dev script 加 `--allow-cors` 即可 |
| **D3** | OpenList 默认端口 5244 | 保留 | 与 prod 行为一致；如 fork 配置改端口，需 `OPENLIST_PORT` env 传递 |
| **D4** | 不撤销 `/openlist-ui-proxy` | 保留 | 用户明确指示"保留主 app 开发辅助" |
| **D5** | 探活端点用 `/api/public/settings` | 新选择 | OpenList 标准端点，不需 auth；`/api/ping` 也可但语义弱 |
| **D6** | 不动 prod 路径（plugin APK assets） | 保留 | prod 已经直访 5244，无 subpath 改造，无关本次撤销 |
| **D7** | 不删 plugin-openlist/web 的 Vite 5174 端口 | 保留 | plugin 管理 UI（OpenListHome / Settings / ConfigEditor）仍需在浏览器内 HMR 迭代 |
| **D8** | 撤销后 `LocalOpenListStatusCard` 的 `openWebUi()` 文案不变 | 保留 | 它已经直访 5244，跟随新方案自然一致 |

---

## 四、Verification（实施后跑）

### V1：plugin Vite 启动无 proxy
```bash
bash scripts/dev-openlist-web.sh
# 期望：vite 启动日志中不出现 "openlist-ui-proxy" / "openlist-spa" 字样
# 期望：curl http://localhost:5174/openlist-spa/  → 404（Vite 找不到路由）
```

### V2：iframe 直访 5244
```bash
# 终端 1
bash scripts/dev-openlist.sh
# 等待 "start HTTP server @ 0.0.0.0:5244" 日志

# 终端 2
bash scripts/dev-openlist-web.sh

# 浏览器（或 OpenPreview）
open http://localhost:5174/webview
# 期望：iframe 加载 http://127.0.0.1:5244/#/login 显示 OpenList 登录页
# 期望：Network 面板 iframe 内部请求全是 127.0.0.1:5244 域（无 5174 域子请求）
```

### V3：OpenList 登录链路
```bash
# 在 iframe 内手动输入 admin / 密码（取决于 fork 默认配置）
# 期望：登录成功跳转到 /home（OpenList 自己的路由），UI 正常渲染
# 期望：Network 面板无 CORS 错误
```

### V4：主 app Vite 仍可用 /openlist-ui/
```bash
# 终端 1
bash scripts/dev-openlist.sh

# 终端 2
bash scripts/start-preview.sh

# 浏览器（或 OpenPreview）
open http://localhost:8100/openlist-ui/
# 期望：仍能看到 OpenList 真实前端（与本次撤销无关，验证回归通过）
```

### V5：prod build 回归
```bash
cd app/encv-mobile/android
./gradlew :plugin-openlist:assembleDebug
# 期望：build 成功，APK 包含 assets/openlist/index.html
unzip -l plugin-openlist/build/outputs/apk/debug/*.apk | grep "assets/openlist/index.html"
# 期望：命中（prod 路径未受影响）
```

### V6：grep 回归（确保无遗漏）
```bash
# 全仓 grep 残余的 /openlist-spa 引用
cd /workspace
grep -rln "openlist-spa" --include="*.ts" --include="*.vue" --include="*.sh" --include="*.md" \
  app/ scripts/ 2>/dev/null
# 期望：仅在 .trae/documents/ 历史 spec 命中，无源码命中

# 全仓 grep /__openlist-health 引用
grep -rln "__openlist-health" --include="*.ts" --include="*.vue" --include="*.sh" --include="*.md" \
  app/ scripts/ 2>/dev/null
# 期望：无任何命中
```

---

## 五、Risk

| # | 风险 | 缓解 |
|---|------|------|
| **R1** | 沙箱浏览器对 127.0.0.1:5244 跨域限制（如 strict-origin policy） | 多数浏览器允许 loopback 跨域；如限制，加 CORS headers（fork 默认已 *） |
| **R2** | 直访 fetch 在 iframe 内的相对路径解析异常 | OpenList SPA 内部用相对路径 `/api/...`，与 origin 127.0.0.1:5244 配套，无 subpath 歧义 |
| **R3** | 用户在浏览器跑 `bash scripts/dev-openlist-web.sh` 但忘记启 5244 后端 | OpenListWebView 错误态 UI 已显示 "请在另一终端启动 OpenList 后端" + 复制按钮（已实现） |
| **R4** | OpenList 端口被其他进程占用 | `dev-openlist.sh` 启动时若 5244 占用会报 `bind: address already in use`；OpenListWebView 的 `isPortOccupied` 已能识别 |
| **R5** | 撤销后某个旧测试场景依赖 `/openlist-spa/` 路径 | V6 grep 兜底；如有命中，迁移到 127.0.0.1:5244 直访 |
| **R6** | 撤销后 `/openlist-ui-proxy` 仍保留造成认知混乱 | 文档注释（OpenListWebView.vue 顶部）明确"两个独立场景：主 app Vite 8100/openlist-ui/ 是开发辅助；plugin WebView 直访 5244 是对齐 prod 的正式路径" |

---

## 六、Sequence（执行顺序）

1. 改 `vite.config.ts`（plugin-openlist/web）— 删 proxy + health plugin
2. 改 `OpenListWebView.vue` — 删 isSandbox 分支 + 改用直访 /api/public/settings
3. 改 `scripts/dev-openlist-web.sh` 注释
4. 跑 V1-V6 全部 verification
5. 如有遗留 grep 命中，单独处理

预计改动量：
- vite.config.ts: -40 行（删 health plugin + proxy 块）
- OpenListWebView.vue: -30 行（删 health 调用 + 简化分支）/+ 20 行（直访 fetch）
- dev-openlist-web.sh: 注释改写 ~10 行
- 总计：~60 行 diff，3 个文件

---

## 七、相关文档

- `.trae/specs/openlist-frontend-extraction-and-sandbox-preview/spec.md` — 描述 `/openlist-ui/` 主 app 路径，本计划不撤销
- `.trae/specs/wire-openlist-runtime-and-ui-v2/spec.md` — OpenList 跨进程 IPC（ContentProvider），与本计划正交
- `.trae/specs/openlist-extension-rewrite-capacitor-ui/spec.md` — plugin 嵌入式 WebView 架构，本计划仅触及 plugin 侧 dev 工具链
- `app/encv-mobile/scripts/dev-openlist.sh` — 启真实 OpenList fork on 5244（**保留且为本计划基础**）
- `app/encv-mobile/src/components/LocalOpenListStatusCard.vue:196` — 已对齐的 prod 直访模式（参考）
