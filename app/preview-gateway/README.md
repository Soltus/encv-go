# preview-gateway

> **沙箱统一预览网关**：单端口 `:16666` 接管 encv-mobile / plugin-openlist-web / encv-go 跨上游转发，让 vite 退回纯净 SPA dev server。

---

## 一、它在生态里的位置

```
外网用户 / 本地 dev / agent-browser
  ↓
:16666  (preview-gateway, 唯一对外入口)
  ├── /             → :8100  encv-mobile Vite (纯净 SPA)
  ├── /openlist-ui/ → :5174  plugin-openlist-web Vite
  ├── /openlist/    → :2025  encv-go (Go) → :5244  OpenList fork
  ├── /api          → :2025  encv-go
  ├── /p/           → :2025  encv-go
  ├── /play         → :2025  encv-go
  └── /__gateway/health   ← 自带健康检查

升级链路（vite HMR）:
  :16666 ws upgrade → :8100 ws (主 app HMR)
  :16666 ws upgrade → :5174 ws (plugin HMR)

外网入口（Trae IDE 集成）:
  :16000 (OpenPreview / agent-tool-host)
    ↓ 首次用 OpenPreview(preview_url="http://localhost:16666/") 注册后
  :16666 (preview-gateway)
```

**核心收益**：
- 外网 / 本地 / agent-browser 全部走单端口 `:16666`
- vite :8100 是纯净 SPA dev server，零反向代理胶水
- 网关层 `changeOrigin: false` 透传 Origin 头，Vite 默认 `cors: true` reflect Origin 天然通过 CORS

---

## 二、为什么独立项目

之前 vite 内部承担了 4 个上游的反向代理职责（`/api` → :2025、`/openlist/` → :2025、`/openlist-ui/*` 静态重写 + 代理），单点职责过重，配置胶水累积。本 spec 把这部分职责拆出到独立 Node 项目：

| 关注点 | 之前（vite 承担） | 现在（preview-gateway 承担） |
|--------|------------------|----------------------------|
| 跨上游路由 | `vite.config.ts` 的 `server.proxy` 块 | `server.ts` 的 `UPSTREAMS` 列表 |
| `/openlist-ui/*` 静态重写 | `openlistUiProxy` plugin 60 行 | `:5174` plugin-openlist-web 自己用 `VITE_BASE=/openlist-ui/` 处理 |
| CORS `'*'` 硬编码 | vite config | 不需要（Vite 默认 reflect Origin） |
| HMR WebSocket | vite 自处理 | 网关 `upgrade` 事件 → `proxy.ws()` |

---

## 三、路由表

| 路径 | 目标 | 用途 |
|------|------|------|
| `/` | `http://127.0.0.1:8100` | encv-mobile Vite SPA（默认 fallthrough） |
| `/openlist-ui` | `http://127.0.0.1:5174` | plugin-openlist-web Vite（OpenList 管理 UI） |
| `/openlist/` | `http://127.0.0.1:2025` | encv-go → OpenList 运行时数据路径 |
| `/api` | `http://127.0.0.1:2025` | encv-go API（service-guard / config / 上传等） |
| `/p/` | `http://127.0.0.1:2025` | encv-go 公开路径 |
| `/play` | `http://127.0.0.1:2025` | encv-go 媒体播放路径 |
| `/__gateway/health` | 内置 | 并发探测所有 upstream 返回 200 / 503 |
| `/__gateway` | 内置 | 静态 banner 端点 |

匹配顺序：先匹配具体路径，否则 fallthrough 到 `/`（encv-mobile）。

---

## 四、端口决策 `:16666`

| 候选 | 状态 | 原因 |
|------|------|------|
| `:16000` | ❌ 占用 | `agent-tool-host` (pid 821) 监听；与 OpenPreview 工具冲突 |
| `:5173` | ❌ 已废弃 | vite 老端口；语义冲突（vite 应在 :8100） |
| `:8100` | ❌ 占用 | encv-mobile Vite；不能让网关抢占 |
| `:16666` | ✅ 选用 | 用户决策"好记"；独立、易记、不与现有冲突 |

---

## 五、配置

通过环境变量：

| 变量 | 默认 | 说明 |
|------|------|------|
| `PORT` | `16666` | 监听端口 |
| `HOST` | `0.0.0.0` | 监听地址 |

---

## 六、启动 / 停止

通过 pm2 统一管理（见 `/workspace/ecosystem.config.cjs`）：

```bash
# 启动所有（含 preview-gateway）
pm2 start /workspace/ecosystem.config.cjs

# 单独管理 preview-gateway
pm2 status preview-gateway
pm2 logs preview-gateway
pm2 restart preview-gateway
pm2 stop preview-gateway
```

通过 `scripts/previews.sh` 包装：

```bash
bash /workspace/scripts/previews.sh start    # 启动全部
bash /workspace/scripts/previews.sh status  # 查看状态
bash /workspace/scripts/previews.sh logs    # 看日志
bash /workspace/scripts/previews.sh restart # 重启全部
```

---

## 七、外网访问（OpenPreview）

**首次**外网访问必须用 OpenPreview 工具显式注册 `:16666`：

```python
OpenPreview(
    command_id="<某个运行中的 command_id>",
    preview_url="http://localhost:16666/"
)
```

注册成功时 `agent-tool-host` 日志：

```
[PreviewManager] Port registered: 16666 (..., old_default: 5173, new_default: 16666, total_ports: 2)
[open_preview] OpenPreview registered successfully port=16666
```

之后 `curl http://localhost:16000/` 才会被转发到 `:16666` → vite。

**为什么不能自动注册**：实测发现 agent-tool-host 的 preview-proxy 内部 `:80` register 端点 `requires_auth=true`，普通 HTTP 请求 401 拒绝。只有 `OpenPreview` 工具（IDE 内部命令）能完成 register。

---

## 八、健康检查

```bash
curl -s http://localhost:16666/__gateway/health | jq .
```

返回示例：

```json
{
  "ok": true,
  "upstreams": {
    "encv-mobile":         { "url": "http://127.0.0.1:8100", "alive": true,  "latency_ms": 6 },
    "plugin-openlist-web": { "url": "http://127.0.0.1:5174", "alive": true,  "latency_ms": 11 },
    "encv-go":             { "url": "http://127.0.0.1:2025", "alive": true,  "latency_ms": 2 }
  }
}
```

`ok: false` 时检查对应 upstream 是否在跑（`ss -tlnp | grep :<port>`）。

---

## 九、故障排查

### Q1: `curl :16666/` 返回 502

上游不可达。看 `pm2 logs preview-gateway` 找到具体 upstream（`encv-mobile` / `plugin-openlist-web` / `encv-go`），然后用 `pm2 status` / `ss -tlnp` 确认对应端口在跑。

### Q2: 浏览器 ESM import() 报 "Failed to fetch dynamically imported module"

CORS 不通过。检查：

1. `curl -I http://localhost:16666/` 看响应头是否有 `Access-Control-Allow-Origin: http://localhost:16666`（vite reflect Origin）
2. 如果没有，确认你访问的是 `:16666` 而不是 `:16000`（未注册时 :16000 走 :5173 默认 upstream）
3. 如果有但浏览器仍报，检查 `:16666` 是否被中间代理（如 `agent-tool-host`）改写了 Origin

### Q3: Vite HMR 不工作

确认 ws upgrade 路径：

```bash
node -e '
const http = require("http");
const req = http.request({
  hostname: "localhost", port: 16666, path: "/?token=test",
  headers: { Connection: "Upgrade", Upgrade: "websocket",
             "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ==",
             "Sec-WebSocket-Version": "13",
             "Sec-WebSocket-Protocol": "vite-hmr" }
});
req.on("upgrade", (res) => { console.log("✅ 101 UPGRADE"); process.exit(0); });
req.on("error", (e) => { console.log("✗", e.message); process.exit(1); });
req.end();
'
```

期望 `✅ 101 UPGRADE`。

### Q4: /openlist-ui/ 看到空白页 OR 浏览器报 `[vite] failed to connect to websocket`

**空白根因**：Vite 8 (rolldown) dev 模式不基于 `<base href>` 改写 import 路径，所有内部 import 解析为**绝对根路径**（如 `/node_modules/.vite/deps/vue.js?v=hash`），不带 `/openlist-ui/` 前缀。gateway 收到 `/node_modules/...` 时 fallthrough 到 :8100 主 app → 404 → 浏览器控制台报 "Failed to fetch" → SPA 空白。

**WS 失败根因（三个隐藏）**：
1. **vite 5+ `server.allowedHosts` 默认锁 localhost** → 外部 Host 头（如 `run-agent-...trae.cn`）直接 403 拒绝
2. **vite `server.hmr.host` 默认派生自 `server.host='0.0.0.0'`** → 退回 `localhost`，沙箱外浏览器连不上沙箱内 `localhost`
3. **vite 8 WS upgrade 监听器要求 `Sec-WebSocket-Protocol: vite-hmr`**（`@vite/client` 实际会带，curl 测试要手动加）

**修复（五层链 + 三件套）**：见 §十一（D11 + D12 五层修复）和 §十（D14 HMR 修复三件套）。

**快速验证**：

```bash
# 1) 入口 HTML 应注入 Set-Cookie
curl -sI http://localhost:16666/openlist-ui/ | grep -i set-cookie
# 期望：__plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600

# 2) HTML 应包含 /openlist-ui/@vite/client (注入的 base 前缀)
curl -s http://localhost:16666/openlist-ui/ | grep -oE '/openlist-ui/@vite'
# 期望：/openlist-ui/@vite

# 3) 子资源带 cookie → 200 text/javascript；不带 → fallthrough text/html
curl -sI -H "Cookie: __plugin_spa=1" http://localhost:16666/src/views/OpenListHome.vue | head -3
# 期望：Content-Type: text/javascript
curl -sI http://localhost:16666/src/views/OpenListHome.vue | head -3
# 期望：Content-Type: text/html (主 app fallback)

# 4) HMR config 应指向外部 host (D14 修复)
curl -s -H "Host: run-agent-test.trae.cn:16666" \
  http://localhost:16666/openlist-ui/@vite/client \
  | grep -E "(socketHost|hmrPort) ="
# 期望：socketHost = ${"run-agent-test.trae.cn" || ...}:16666/
#       hmrPort = 16666

# 5) WS 升级连通 (D14 修复)
node -e "
const http = require('http');
const req = http.request({
  host:'127.0.0.1', port:16666, path:'/?token=test', method:'GET',
  headers:{
    'Connection':'Upgrade','Upgrade':'websocket',
    'Sec-WebSocket-Key':'dGhlIHNhbXBsZSBub25jZQ==',
    'Sec-WebSocket-Version':'13',
    'Sec-WebSocket-Protocol':'vite-hmr',  // 关键！
    'Cookie':'__plugin_spa=1',
  },
});
req.on('upgrade',(r)=>console.log('UPGRADE',r.statusCode));
req.on('response',(r)=>console.log('RESP',r.statusCode));
req.on('error',(e)=>console.log('ERR',e.message));
req.setTimeout(3000,()=>{console.log('TIMEOUT');process.exit(0)});
req.end();
"
# 期望：UPGRADE 101
```

确认 plugin-openlist-web :5174 启动时设了 `VITE_BASE=/openlist-ui/`，并启用了 `allowedHosts: true` + `dynamicHmrHostPlugin`：

```bash
grep -E "VITE_BASE|allowedHosts|dynamicHmrHost" /workspace/app/encv-mobile/plugin-openlist/web/vite.config.ts
# 期望：process.env.VITE_BASE || '/openlist-ui/'
#       allowedHosts: true
#       dynamicHmrHostPlugin() 在 plugins 数组中
```

### Q5: 撤销 `cors: '*'` 后 :16666 仍 CORS 通过？

Vite 8 默认 `cors: true` 会 reflect Origin。preview-gateway `changeOrigin: false` 透传 Origin 头，Vite 看到 `Origin: http://localhost:16666`，回 `Access-Control-Allow-Origin: http://localhost:16666`，浏览器 CORS 匹配，天然通过。

---

## 十、沙箱 dev HMR 修复（D14）

> **核心问题**：沙箱 dev 浏览器跑在外部 trae.cn 域名（如 `run-agent-...trae.cn:16666`），vite dev server 跑在沙箱内 `:5174` / `:8100`。浏览器无法直连 `localhost:5174`，HMR 报 `[vite] failed to connect to websocket`。

### 10.1 三个隐藏根因

**(1) vite 5+ `server.allowedHosts` 默认白名单**：

vite 5+ 引入安全特性，默认 `server.allowedHosts` 锁 `localhost / 127.0.0.1`，外部 Host 头直接 403。

**修复**：
```ts
// app/encv-mobile/{plugin-openlist/web/,}vite.config.ts
server: {
  allowedHosts: true,  // 允许所有 Host
}
```

**(2) vite `server.hmr.host` 派生自 `server.host`**：

`server.host='0.0.0.0'` → HMR host 退回 `localhost` → 沙箱外浏览器连不上沙箱内 `localhost`。

**修复**（`dynamicHmrHostPlugin`）：

`enforce: 'pre'` + `transform` 钩子拦截 `@vite/client`，替换四个占位符：

| 占位符 | 修复前 | 修复后 |
|--------|--------|--------|
| `__HMR_HOSTNAME__` | `"localhost"` | auto-detected `req.headers.host` 剥端口 |
| `__HMR_PORT__` | `5174` | `16666` |
| `__HMR_PROTOCOL__` | `"ws"` | `"wss"` 当 Referer 是 `https://`，否则 `"ws"` |
| `__HMR_BASE__` | `"/"` | `"/"`（保持） |

**为什么 `enforce: 'pre'`？** vite 8 内部用 `code.replace('__HMR_HOSTNAME__', ...)` 替换占位符，`enforce: 'pre'` 保证我们的 transform 先替换，vite 内部再 replace 时找不到占位符就跳过。

**auto-detect 流程**：
```
1. 浏览器 GET http://run-agent-...trae.cn:16666/openlist-ui/
2. 网关剥前缀 → :5174 vite
3. vite 中间件检测 req.headers.host = 'run-agent-...trae.cn:16666'
4. vite 响应 HTML（含 <script src="/openlist-ui/@vite/client">）
5. 浏览器 GET /openlist-ui/@vite/client → 网关 → :5174 vite
6. transform @vite/client → dynamicHmrHostPlugin 替换占位符
7. 浏览器 HMR client 连接 ws://run-agent-...trae.cn:16666/?token=...
8. 网关 WS upgrade → cookie 路由到 :5174 → vite 接受
```

**(3) vite 8 WS upgrade 要求 `Sec-WebSocket-Protocol: vite-hmr`**：

vite 8 在 `if (wsServer)` 分支只对带 `vite-hmr` / `vite-ping` subprotocol 的 upgrade 调 `handleUpgrade`。`@vite/client` 实际会带，curl 测试要手动加。

### 10.2 三件套修复清单

| # | 文件 | 改动 | 作用 |
|---|------|------|------|
| 1 | `app/encv-mobile/plugin-openlist/web/vite.config.ts` | `server.allowedHosts: true` | 让 vite 接受外部 Host |
| 2 | `app/encv-mobile/plugin-openlist/web/vite.config.ts` | `dynamicHmrHostPlugin` (enforce:'pre') | 替换 @vite/client 占位符 |
| 3 | `app/encv-mobile/vite.config.ts` | 同 1 + 2（端口 16666） | 主 app 同上 |

### 10.3 env var 覆盖

`dynamicHmrHostPlugin` 优先级：**env > auto-detect > fallback**

| env | 默认值 | 说明 |
|-----|--------|------|
| `HMR_HOST` | auto-detected | 外部 host（不含端口） |
| `HMR_PROTOCOL` | auto from Referer | `ws` 或 `wss` |
| `HMR_CLIENT_PORT` | `16666` | 浏览器连的端口 |

### 10.4 完整验证命令

```bash
# A) @vite/client 接受外部 Host
curl -s -H "Host: run-agent-test.trae.cn:16666" http://127.0.0.1:5174/@vite/client \
  | grep -E "(socketHost|hmrPort|socketProtocol) ="
# 期望：socketHost = ${"run-agent-test.trae.cn" || ...}:16666/

# B) 经 gateway 完整链路
curl -s -H "Host: run-agent-test.trae.cn:16666" \
  http://localhost:16666/openlist-ui/@vite/client \
  | grep -E "(socketHost|hmrPort) ="

# C) WS 升级（带 vite-hmr subprotocol）
node -e "
const http = require('http');
const req = http.request({
  host:'127.0.0.1', port:16666, path:'/?token=test', method:'GET',
  headers:{
    'Connection':'Upgrade','Upgrade':'websocket',
    'Sec-WebSocket-Key':'dGhlIHNhbXBsZSBub25jZQ==',
    'Sec-WebSocket-Version':'13',
    'Sec-WebSocket-Protocol':'vite-hmr',
    'Cookie':'__plugin_spa=1',
  },
});
req.on('upgrade',(r)=>console.log('UPGRADE',r.statusCode));
req.on('response',(r)=>console.log('RESP',r.statusCode));
req.on('error',(e)=>console.log('ERR',e.message));
req.setTimeout(3000,()=>{console.log('TIMEOUT');process.exit(0)});
req.end();
"
# 期望：UPGRADE 101
```

### 10.5 已知限制

1. **auto-detect 一次性**：第一个请求的 Host 锁定后不变。多域名场景需每个域名单独启动 vite。
2. **HTTPS 推断 protocol 靠 Referer**：referer 不可靠时用 `HMR_PROTOCOL=wss` env 覆盖。
3. **`__WS_TOKEN__` 不能改**：vite 启动时随机生成，gateway 透传即可。

---

## 十一、沙箱 dev 资源路由：Cookie 标记机制

> **核心问题**：Vite 8 (rolldown) dev 模式不基于 `<base href>` 改写 import 路径。访问 `/openlist-ui/` 时，入口 HTML 的相对路径 `./src/main.ts` 解析为 `/openlist-ui/src/main.ts`（基于 base），但 main.ts 内部 `import 'vue'` 被 vite dev 解析为 `/node_modules/.vite/deps/vue.js?v=hash`（**绝对根路径，不带 /openlist-ui 前缀**）。浏览器请求 `http://localhost:16666/node_modules/...` → gateway fallthrough 到 :8100 (encv-mobile) → 404 → SPA 空白。

### 11.1 为什么单一机制不够

| 机制 | 单独作用 | 失败原因 |
|------|---------|---------|
| `<base href="/openlist-ui/">` | 相对路径变 base-prefixed | Vite 8 dev 模式内部 import 不基于 base 改写 |
| `server.origin: 'http://...:16666/openlist-ui'` | 让 vite 输出完整 origin 路径 | Vite 8 仍输出绝对根路径（实测） |
| `enforce:'pre' + transform` 改 .ts/.vue 内路径 | 拦截 import 重写 | main.ts 入口文件不被 user transform 处理 |
| preview-gateway Referer 路由 | 用 referer 判定 plugin SPA 来源 | Trae IDE 沙箱 Chrome referer 为空 |

### 11.2 五层修复链（D11 + D12）

**Layer 1 — `VITE_BASE` 环境变量**（scripts/dev-openlist-web.sh + ecosystem.config.cjs）：

```bash
export VITE_BASE="/openlist-ui/"
```

**Layer 2 — vite.config.ts `injectBaseHref` plugin（order: 'post'）**：

```ts
function injectBaseHref(href: string): Plugin {
  return {
    name: 'inject-base-href',
    transformIndexHtml: {
      order: 'post',  // 必须在 Vite 注入 @vite/client 之后
      handler(html) {
        let result = html
        // 1) 占位符 → <base> 标签
        result = result.replace(
          '<!--VITE-BASE-HREF-PLACEHOLDER-->',
          `<base href="${href}" />`,
        )
        // 2) @vite/client 路径加前缀
        if (result.includes('<script type="module" src="/@vite/client">')) {
          result = result.replace(
            '<script type="module" src="/@vite/client">',
            `<script type="module" src="${href.replace(/\/$/, '')}/@vite/client">`,
          )
        }
        return result
      },
    },
  }
}
```

**Layer 3 — vite.config.ts `fs.allow` 扩展**：

```ts
server: {
  fs: {
    allow: [
      path.resolve(__dirname),
      path.resolve(__dirname, '..', '..', '..'),
      path.resolve('/workspace/app/encv-mobile'),
      path.resolve('/workspace/app/encv-mobile/node_modules'),
      path.resolve('/workspace'),
    ],
  },
}
```

**Layer 4 — preview-gateway `pathRewrite` 剥前缀**：

```ts
const UPSTREAMS: Upstream[] = [
  {
    match: '/openlist-ui',
    target: 'http://127.0.0.1:5174',
    pathRewrite: (p) => p.replace(/^\/openlist-ui(?=\/|$)/, '') || '/',
  },
]
```

**Layer 5 — preview-gateway `Set-Cookie` 注入 + Cookie 路由**：

```ts
// 注入：HTML 响应时
proxy.on('proxyRes', (proxyRes, _req) => {
  const ct = proxyRes.headers['content-type']
  if (ct && /text\/html/i.test(String(ct))) {
    const cookieLine = '__plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600'
    const existing = proxyRes.headers['set-cookie']
    proxyRes.headers['set-cookie'] = existing
      ? (Array.isArray(existing) ? [...existing, cookieLine] : [String(existing), cookieLine])
      : [cookieLine]
  }
})

// 路由：pickUpstream 末尾的 fallback
if (cookie && /(?:^|;\s*)__plugin_spa=1/.test(cookie)) {
  return UPSTREAMS.find((u) => u.match === '/openlist-ui') ?? DEFAULT_UPSTREAM
}
```

### 11.3 完整请求流

```
1. 用户 GET http://localhost:16666/openlist-ui/
   ↓ gateway pickUpstream: url 匹配 /openlist-ui → :5174
   pathRewrite: /openlist-ui/ → /
   ↓ proxy 到 :5174
   vite 返回 HTML（已含 <base href="/openlist-ui/"> + 改过的 @vite/client）
   ↓ proxyRes: text/html → Set-Cookie: __plugin_spa=1
   浏览器收到 HTML + cookie

2. 浏览器执行 <script src="/openlist-ui/src/main.ts"> (相对路径，基于 base)
   ↓ gateway pickUpstream: url 匹配 → :5174
   pathRewrite: /openlist-ui/src/main.ts → /src/main.ts
   ↓ proxy 到 :5174
   vite 返回 main.ts（内部 import 是绝对根路径）

3. 浏览器执行 main.ts → import 'vue' 解析为 /node_modules/.vite/deps/vue.js
   ↓ gateway pickUpstream: url 不匹配任何 prefix，但 cookie 命中 → :5174
   ↓ proxy 到 :5174
   vite 返回 vue.js ✓
```

### 11.4 完整验证命令

```bash
# A) 入口 HTML 注入验证
curl -sI http://localhost:16666/openlist-ui/ | grep -i set-cookie
# 期望：__plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600

curl -s http://localhost:16666/openlist-ui/ | grep -oE '/openlist-ui/@vite|src/main\.ts'
# 期望：/openlist-ui/@vite  src/main.ts

# B) 子资源 cookie 路由
curl -sI -H "Cookie: __plugin_spa=1" http://localhost:16666/src/views/OpenListHome.vue | head -3
# 期望：HTTP/1.1 200 OK / Content-Type: text/javascript

curl -sI http://localhost:16666/src/views/OpenListHome.vue | head -3
# 期望：HTTP/1.1 200 OK / Content-Type: text/html (主 app SPA fallback)

# C) 端到端 6+ 场景
echo "==== 主 app / 子资源 / plugin SPA / health / 端口监听 ===="
curl -sI http://localhost:16666/ | head -1                                # 200 (fallthrough)
curl -sI -H "Cookie: __plugin_spa=1" http://localhost:16666/src/main.ts | head -1  # 200 (主 app)
curl -sI http://localhost:16666/openlist-ui/ | head -1                    # 200 (plugin)
curl -s http://localhost:16666/__gateway/health | jq .ok                  # true
ss -tlnp | grep -E "(:16666|:8100|:5174|:2025|:5244)" | wc -l             # 5
```

### 11.5 为什么不用 Referer

- Trae IDE 沙箱 Chrome 默认 `referrer-policy = strict-origin-when-cross-origin`
- `document.referrer` 在 network requests 中**显示为空**
- Referer 方案在普通浏览器可用，但对沙箱 dev 不可靠
- Cookie 是显式由 gateway 注入的，更可控

### 11.6 重启后是否要重新注入 Cookie

- `Max-Age=3600`：1 小时后 cookie 过期，需重新访问 `/openlist-ui/` 入口
- `pm2 restart preview-gateway --update-env` 后已发出的 cookie 仍然有效（cookie 存浏览器，不在 gateway）
- 关闭浏览器 tab 重新打开 → cookie 仍有效（Path=/，同源）
- 清空浏览器 cookie 后 → 下次访问 `/openlist-ui/` 时 Set-Cookie 重新注入

---

## 十二、防御性 UI 设计

> **核心原则**：白屏 = 失败。任何路径不匹配 / 组件渲染异常 / 资源加载失败都必须显示**可诊断的 UI**。

### 12.1 三层防护架构（D13）

```
Layer 1: vue-router catch-all（路由级）
  任何未匹配路径 → :pathMatch(.*)* → NotFoundView
  - 列出所有已知路由
  - 显示 location.pathname / router base / currentRoute
  - 「返回 /home」「重新加载」按钮
  - <details> 折叠 debug 面板

Layer 2: onErrorCaptured（组件级）
  父组件捕获子组件 render 异常 → 显示局部错误 UI
  - 错误标题 + 图标
  - 错误堆栈（折叠）
  - 「重新加载」「返回」按钮
  - return false 阻止冒泡

Layer 3: rootError fallback（应用级）
  App.vue 顶层 catch → 整 app 替换为错误屏
  - 大图标 + 「应用启动失败」
  - 错误堆栈（可复制）
  - 「重新加载」按钮
```

### 12.2 文件清单

| 文件 | 作用 |
|------|------|
| `app/encv-mobile/src/views/NotFoundView.vue` | 主 app 404 页面（Ionic UI） |
| `app/encv-mobile/src/router/index.ts` | 主 app catch-all 路由 |
| `app/encv-mobile/src/App.vue` | 主 app onErrorCaptured + rootError |
| `app/encv-mobile/plugin-openlist/web/src/views/NotFoundView.vue` | plugin SPA 404 页面（纯 Vue） |
| `app/encv-mobile/plugin-openlist/web/src/router/index.ts` | plugin catch-all + base='/openlist-ui/' |
| `app/encv-mobile/plugin-openlist/web/src/App.vue` | plugin onErrorCaptured + rootError |

### 12.3 为什么"白屏 = 失败"

| 场景 | 不防御的后果 | 防御后 |
|------|------------|-------|
| 用户手输 `/tabs/typo` | 整个 SPA 空白 | 显示 404 + 路由列表 + 返回按钮 |
| 沙箱 vite HMR 断开 | dev mode 整个 SPA 卡死 | 重新加载按钮可恢复 |
| `OpenListView` 组件 import 失败 | /tabs/openlist 空白 | rootError 屏 + 堆栈 + 重新加载 |
| 用户手输 `/openlist-ui/#/zzz` | 空白 | 显示 404 + 已知路由 |
| 上游 :5174 vite 挂掉 | 入口 HTML 502 → 浏览器空白 | preview-gateway 已返 502 JSON（仍可诊断） |

### 12.4 禁止的反模式

| 反模式 | 后果 |
|--------|------|
| ❌ inline `<ion-modal :is-open="showModal">` 用于跨 tab 操作 | tab 非活跃时 modal 不渲染 |
| ❌ 把 catch-all 路由放在 routes 列表第一项 | 永远匹配不到其他路由 |
| ❌ `onErrorCaptured` 不 `return false` | 异常继续冒泡到根，被 Vue 默认 handler 吞掉 |
| ❌ `rootError` 状态用 `let` 不用 `ref` | Vue 不会响应式更新 |
| ❌ NotFoundView 不导出 debug 信息 | 出了问题无法诊断 |

---

## 十三、相关 spec

- 主 spec: [`.trae/specs/unify-sandbox-preview-port/spec.md`](file:///workspace/.trae/specs/unify-sandbox-preview-port/spec.md) — 完整决策、风险、J1-J14 验证
- 上游 spec: [`.trae/specs/openlist-frontend-extraction-and-sandbox-preview/`](file:///workspace/.trae/specs/openlist-frontend-extraction-and-sandbox-preview/spec.md) — OpenList 前端抽取
- 运行时 spec: [`.trae/specs/wire-openlist-runtime-and-ui-v2/`](file:///workspace/.trae/specs/wire-openlist-runtime-and-ui-v2/spec.md) — OpenList 运行时 + UI 集成
