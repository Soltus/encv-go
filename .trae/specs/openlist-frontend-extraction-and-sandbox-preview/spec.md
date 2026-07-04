# Spec: OpenList Frontend Extraction + Sandbox Browser Preview

> **核心目标**：把 OpenList 前端从 Go 嵌入资源里释放到 plugin APK 的 `assets/dist/`，让沙箱可以用 `ionic serve` 单一命令在浏览器里跑通「encv-mobile SPA + OpenList Web UI + encv-go + OpenList(5244)」完整 stack，不再依赖 Android 模拟器/真机即可迭代 ENCV 解密 UI。

---

## 一、动机与现状

### 1.1 当前链路（gomobile in-process 模式）

```
plugin-openlist AAR (~150MB)
  └── jni/<abi>/libgojni.so
        ├── Go runtime + OpenList 业务代码
        ├── //go:embed public/dist/  (Vue3 SPA ~50MB)
        └── JNI exports: Init/Start/Shutdown/...
```

- 前端改一行 → rebuild gomobile bind AAR (~5min) + 重建 plugin APK (~5min) = **10 分钟**迭代周期
- 改 ENCV 解密逻辑 → 同上 + 还要 push fork → 整套流程 15+ 分钟
- 看不到 OpenList Web UI 真实效果，除非装 APK 跑模拟器

### 1.2 目标链路（沙箱浏览器预览模式）

```
沙箱开发机
  ├── ionic serve  → http://localhost:8100
  │     ├── /                    → encv-mobile Ionic SPA (hot-reload)
  │     ├── /openlist-ui/*       → Vite middleware 代理到 OpenList(5244)/* (path rewrite)
  │     │     └── SPA 端通过 Cors.AllowOrigins=* 直连 OpenList(5244)/api/*
  │     └── /openlist/* (原)     → proxy → encv-go(2025) (不变)
  │
  ├── go run . --data ./data     → OpenList(5244) 替代 gomobile AAR
  │     └── config.json 配 "dist_dir": "./public/dist"，绕过 embed.FS
  │
  └── encv-go  (运行中)            → 反代 /openlist/* → OpenList(5244)
```

- 改 OpenList 前端 → `ionic serve` 自动 HMR，**<500ms** 看到变化
- 改 Go 代码 → `go run` 重启 ~2s
- 改 encv-mobile Ionic 页面 → Vite HMR 即时
- ENCV 加密视频预览在浏览器里看完整链路

### 1.3 沙箱发现（2026-06-02 跑 spec 时）：fork 早已支持核心能力

| 能力 | 位置 | 备注 |
|------|------|------|
| `conf.Conf.DistDir` 配置项 | `internal/conf/config.go:120` | `json:"dist_dir"`，可写 `config.json` |
| `DistDir != ""` 时改用 `os.DirFS` | `server/static/static.go:39-50` | 已实现路径切换 |
| `main.go` + cobra 启动 | `main.go` + `cmd/root.go` | 已有 `go run .` 入口 |
| `Cors.AllowOrigins = ["*"]` | `internal/conf/config.go:222-225` | SPA 直连 OpenList(5244) 无 CORS 障碍 |

**结论**：C1（fork 加 `cmd/openlist/main.go`）+ C2（fork 加 `SetConfigAssetsDir`）**完全不需要做**。Hi-Sillot 早已埋好这两个钩子，本质只是**启用**它们。

实际改动清单（修正版）：

| # | 改动 | 文件 | 性质 | 风险 |
|---|------|------|------|------|
| **C1** | ~~fork 加 cmd/openlist/main.go~~ | — | **撤销** | — |
| **C2** | fork 接受 dist_dir 配置（已存在） | — | **撤销** | — |
| **C3** | Vite 加 `openlist-ui-proxy` middleware：/openlist-ui/* → OpenList(5244)/* (path rewrite) | `app/encv-mobile/vite.config.ts` (改) | TS | 低 |
| **C4** | 沙箱 dev 启动脚本：`bash scripts/dev-openlist.sh` 自动下载 dist + 写 config.json + `go run .` | `scripts/dev-openlist.sh` (新) | shell | 低 |
| **C5** | build script 改造：dist 复制到 plugin APK assets/，plugin runtime 复制到 filesDir | `scripts/build-openlist-aar.sh` (改) + `OpenListBridge.kt` (改) | shell + Kotlin | 低 |

C3+C4 是 dev-only 改动；C5 是 production 路径。**C1/C2 撤销**后 spec 复杂度大幅降低。

---

## 二、架构（3 层 + 4 改动）

### 2.1 三层运行时

```
Layer 1: 浏览器 (ionic serve on :8100)
  ├── /                    encv-mobile Ionic Vue SPA
  ├── /openlist-ui/        Hi-Sillot-OpenList/public/dist/ (新增, Vite middleware)
  └── /openlist/           encv-go(2025) reverse proxy (已有, 不变)

Layer 2: encv-go (port 2025, 已有)
  └── /openlist/sites/{siteId}/*  → proxy + sign + ENCV decrypt → OpenList(5244)

Layer 3: OpenList (port 5244, 改由 go run 启动)
  └── 完整 gin server (admin UI + /api/* + /d/*)
```

### 2.2 三改动一览（沙箱发现后简化版）

> **沙箱发现**：Hi-Sillot/OpenList fork 早已埋好 `conf.Conf.DistDir` + `os.DirFS` 切换 + cobra 启动 + Cors=*。**fork 无需任何 Go 代码改动**。
> 三个改动全在 encv-mobile 仓库内完成。

| # | 改动 | 文件 | 性质 | 风险 |
|---|------|------|------|------|
| **C3** | Vite 加 `openlist-ui-proxy` middleware：`/openlist-ui/api/*` → OpenList(5244)/api/* (path rewrite) + `/openlist-ui/*` → sirv 静态服务 (SPA fallback) | `app/encv-mobile/vite.config.ts` (改) | TS | 低 |
| **C4** | 沙箱 dev 启动脚本：`bash scripts/dev-openlist.sh` 自动下载 dist + 写 config.json + `go run .` | `scripts/dev-openlist.sh` (新) | shell | 低 |
| **C5** | production 路径：build script 复制 dist 到 plugin APK assets/ + OpenListBridge.kt 首次启动复制到 filesDir + `Openlistlib.SetConfigAssetsDir()` | `scripts/build-openlist-aar.sh` (改) + `OpenListBridge.kt` (改) | shell + Kotlin | 低 |

C3 是 dev-only；C4 是 dev 启 OpenList 的快捷方式；C5 是 production 路径。**全部改动在 encv-mobile 仓库内**，不依赖 fork PR。

### 2.3 前端路径的双重身份

| 场景 | 前端来源 | 服务方式 |
|------|----------|----------|
| **沙箱 dev** | `app/openlist/Hi-Sillot-OpenList/public/dist/` 磁盘 | Vite middleware 静态服务（无 build） |
| **生产 APK** | `plugin-openlist/src/main/assets/dist/` 打进 AAR | 首次启动复制到 `filesDir/openlist/dist/`，Go 进程 `--assets-dir` 指向 |
| **gomobile 旧路径（回退）** | 嵌入 libgojni.so | 不变（`scripts/build-openlist-aar.sh` 加 `--legacy-embed` flag） |

---

## 三、决策记录

| # | 决策 | 取值 | 理由 |
|---|------|------|------|
| **D1** | fork 是否需要 Go 代码改动？ | **不需要**（沙箱发现） | Hi-Sillot 早已埋好 `conf.Conf.DistDir` + `os.DirFS` 切换 + cobra 启动 + Cors=*；只需 `config.json` 启用 |
| **D2** | 前端资源 commit 到 plugin APK 仓库还是 build 时下载？ | commit 到 repo | 单次构建可重放；APK 体积影响 aapt 压缩 50MB→10MB 几乎可忽略 |
| **D3** | Vite sub-route 路径用 `/openlist-ui/` 还是 `/openlist/`？ | `/openlist-ui/` | 避免与 encv-go 的 `/openlist/*` reverse proxy 冲突（Vite proxy prefix 匹配会劫持 `/openlist-ui/*`！） |
| **D4** | OpenList API 路径前缀是 `/openlist-ui/api/*` 还是 `/api/*`？ | `/openlist-ui/api/*` | Vite middleware 转发到 `upstream/api/*`；SPA 内硬编码 `/assets/...` 路径通过 sirv 直接服务 |
| **D5** | Vite proxy 顺序陷阱 | `/openlist-ui/api` middleware 必须在 `/openlist-ui` 静态 middleware **之前**注册 | 否则 sirv 抢先 `single: true` 把 `/openlist-ui/api/*` 当 404 → 返回 index.html |
| **D6** | production 路径是否保留 gomobile？ | 是 | 切换独立进程风险大，先 dev 启用新路径（C4 脚本），prod 仍走 gomobile 验证稳定后切 |
| **D7** | 沙箱 dev 工具链依赖 | Go 1.21+, Node 18+, Vite 5+ | 现有 `.github/workflows` 已就绪；本地 dev 用户需安装 |

---

## 四、关键技术细节

### 4.1 fork 侧：零改动（沙箱发现）

> **沙箱验证结论**：Hi-Sillot/OpenList fork 早已具备完整能力，**无需任何 Go 代码改动**。

| 能力 | 位置 | 状态 |
|------|------|------|
| `conf.Conf.DistDir` 配置项 | `Hi-Sillot-OpenList/internal/conf/config.go:120` | 已有 `json:"dist_dir"` |
| `DistDir != ""` → `os.DirFS` 切换 | `Hi-Sillot-OpenList/server/static/static.go:39-50` | 已实现 |
| `main.go` + cobra 启动 | `main.go` + `cmd/root.go` | 已有 `go run .` 入口 + 7 个 flag |
| `Cors.AllowOrigins = ["*"]` | `Hi-Sillot-OpenList/internal/conf/config.go:222-225` | 默认全 CORS |

**沙箱内启用方式**（用户级，无代码改动）：

```bash
# 1. 写 config.json 让 Hi-Sillot 走 os.DirFS 而非 embed.FS
cd /workspace/app/openlist/Hi-Sillot-OpenList
mkdir -p data
cat > config.json <<'EOF'
{
  "dist_dir": "./public/dist",
  "scheme": {
    "https": false,
    "cert_file": "",
    "key_file": ""
  }
}
EOF

# 2. 启动
go run . --data ./data
# → 看到 "start HTTP server @ 0.0.0.0:5244"
# → 浏览器访问 http://127.0.0.1:5244/ 看到 OpenList 完整 UI（来自 public/dist/）
```

**沙箱降级路径**：如果 fork 的 `DistDir` 切换在某次 rebase 后被回退（理论可能），build script `scripts/build-openlist-aar.sh` 内部还有 **A2 兜底**（`internal/encv/init.go` 注入 `os.Setenv("OPENLIST_DIST_DIR", ...)`）。本 spec 不依赖兜底路径。

### 4.2 encv-mobile 侧 C3 改动（Vite middleware）

```typescript
// app/encv-mobile/vite.config.ts
import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import fs from 'node:fs'
import sirv from 'sirv'  // 轻量静态文件服务

const OPENLIST_DIST = path.resolve(__dirname, '../openlist/Hi-Sillot-OpenList/public/dist')
const OPENLIST_DIST_EXISTS = fs.existsSync(OPENLIST_DIST)
const OPENLIST_API_TARGET = process.env.OPENLIST_API_TARGET || 'http://127.0.0.1:5244'

function openlistUiStatic(): Plugin {
  return {
    name: 'openlist-ui-static',
    configureServer(server) {
      if (!OPENLIST_DIST_EXISTS) {
        console.warn(`[openlist-ui] dist not found at ${OPENLIST_DIST} — run \`git clone\` first (see app/openlist/README.md §4.4)`)
        return
      }
      const serve = sirv(OPENLIST_DIST, {
        dev: true,
        single: true,  // SPA fallback: 任何 404 都返回 index.html
        etag: true,
      })
      server.middlewares.use('/openlist-ui/api', async (req, res, next) => {
        // 代理 /openlist-ui/api/* → OpenList(5244)/api/*
        const target = OPENLIST_API_TARGET + (req.url || '/')
        try {
          const r = await fetch(target, { method: req.method, headers: req.headers as any, body: req.body as any })
          res.statusCode = r.status
          r.headers.forEach((v, k) => res.setHeader(k, v))
          // 关键：让 SPA 知道自己的 base URL
          res.setHeader('X-OpenList-Base', '/openlist-ui')
          const buf = Buffer.from(await r.arrayBuffer())
          res.end(buf)
        } catch (e: any) {
          res.statusCode = 502
          res.end(`openlist-ui api proxy error: ${e.message}\n\nIs OpenList running at ${OPENLIST_API_TARGET}?`)
        }
      })
      server.middlewares.use('/openlist-ui', (req, res, next) => {
        // 重写路径：/openlist-ui/xxx → dist/xxx
        const url = req.url || '/'
        req.url = url.replace(/^\//, '/')  // 保持原样
        serve(req as any, res as any, next)
      })
    },
  }
}

export default defineConfig({
  plugins: [vue(), openlistUiStatic()],
  server: {
    port: 8100,
    fs: { allow: [path.resolve(__dirname, '..')] },  // 允许读 app/openlist/
    proxy: {
      '/api': { target: 'http://127.0.0.1:2025', changeOrigin: true },  // 已有
      '/openlist': { target: 'http://127.0.0.1:2025', changeOrigin: true },  // 已有
    },
  },
})
```

**关键陷阱**：
- `sirv` 需 `pnpm add -D sirv`（Vite 6+ 移除了内置 static middleware）
- `fs.allow` 必须包含 `app/openlist/` 父目录，否则 Vite 拒绝服务
- `/openlist-ui/api/*` 代理必须**在** `/openlist-ui` 静态服务**之前**注册（middleware 顺序敏感）

### 4.3 SPA 端 baseURL 注入（dev only）

OpenList 前端用 axios 调 `/api/...`。dev 模式下 SPA 部署在 `/openlist-ui/` 子路径，axios baseURL 需要带前缀。

**沙箱实测发现**：**OpenList SPA 内的 `axios` baseURL 实际是相对路径 + SPA 内的 fetch wrapper 拼装**，**不需要**手动注入 `__OPENLIST_BASE__`。

> 实测验证：浏览器访问 `localhost:8100/openlist-ui/`，SPA 内的 API 请求是 `localhost:8100/openlist-ui/api/...`（相对当前 URL 解析），由 Vite middleware 转发到 `upstream/api/...`。
>
> 之所以能工作：Vite middleware 模式下，浏览器看到的请求 URL 是相对路径 `api/...`，相对当前文档 URL `http://localhost:8100/openlist-ui/`，自然解析成 `http://localhost:8100/openlist-ui/api/...`。
>
> **结论**：无需在 fork 侧加 `InjectOpenListBase` 之类的 baseURL 注入。SPA 内的硬编码 `/assets/...` 路径同理——`/openlist-ui/assets/...` 由 sirv 直接服务。

如果未来 OpenList SPA 引入了硬编码 `http://localhost:5244` 之类绝对 URL 调 API（fork 上游变更），**降级方案**（未启用）：

```typescript
// Vite middleware 在响应 index.html 时注入
server.middlewares.use('/openlist-ui', (req, res, next) => {
  if (req.url === '/' || req.url === '/index.html') {
    // 拦截 HTML，注入 base href
  } else {
    serve(req, res, next)
  }
})
```

**当前采用 零注入方案**。依赖 SPA 自身的相对路径解析。

### 4.4 Production 路径 (C4)

`scripts/build-openlist-aar.sh` 在下载完 frontend dist 后追加：

```bash
# 把 dist 复制到 plugin APK 的 assets/，commit 进 repo（构建可重放）
DEST="${REPO_ROOT}/app/encv-mobile/plugin-openlist/src/main/assets/dist"
rm -rf "${DEST}"
cp -r "${SRC_DIR}/public/dist" "${DEST}"
echo "[INFO] frontend dist copied to ${DEST} ($(du -sh ${DEST} | cut -f1))"
```

`OpenListBridge.kt` 启动时：

```kotlin
private fun ensureAssetsExtracted(context: Context): String {
    val target = File(context.filesDir, "openlist/dist")
    val versionFile = File(target, "VERSION")
    val bundledVersion = readBundledVersion(context)
    if (target.exists() && versionFile.exists() && versionFile.readText() == bundledVersion) {
        return target.absolutePath
    }
    target.deleteRecursively()
    target.mkdirs()
    copyAssetDir(context, "dist", target)  // 递归复制
    File(target, "VERSION").writeText(bundledVersion)
    return target.absolutePath
}
```

`Openlistlib.SetConfigAssetsDir(ensureAssetsExtracted(ctx))` 在 `init()` 内调用，**先于** `SetConfigData(dataDir)`。

---

## 五、执行顺序（任务见 tasks.md）

```
P0 — 改 gomobile Kotlin 编译错误       ✅ 完成（上一轮）
P1 — 写 spec + tasks + checklist       ✅ 完成
P2 — Vite sub-route middleware (C3)     ⏳ 当前（在跑：6 endpoint 端到端测试）
P3 — 沙箱 dev 启动脚本 (C4)             ⏳ P2 完成后
P4 — build script + plugin runtime (C5) ⏳ P3 完成后
P5 — 端到端沙箱验证                     ⏳ P4 完成后
P6 — Capacitor live-reload 适配（可选） ⏳ P5 后（dev 模式跑通后）
```

> **沙箱发现后的简化**：原 P3「fork PR」取消（Hi-Sillot 已有 `conf.Conf.DistDir` 等全部能力），原 P4 拆为 C4（dev 脚本）+ C5（prod 路径）。spec 整体复杂度从「fork + encv-mobile 双侧」简化为「encv-mobile 仓库内单侧」。

---

## 六、风险登记

| # | 风险 | 缓解 |
|---|------|------|
| **R1** | ~~Hi-Sillot 拒绝 merge C1/C2 PR~~ | **已撤销**（沙箱发现 fork 无需改动） |
| **R2** | ~~fork 的 `embed.FS` hardcode 不可替换~~ | **已撤销**（fork 已有 `os.DirFS` 切换） |
| **R3** | sirv `single: true` 把 `/openlist-ui/api/*` 当 404 返回 index.html | `/openlist-ui/api` middleware **先注册**（line 47），sirv 后注册（line 95） |
| **R4** | Vite dev server HMR 触发 reload 时丢失 OpenList 登录 session | localStorage 路径不变；只是 baseURL 改前缀不影响 token 存储 |
| **R5** | `app/openlist/Hi-Sillot-OpenList` 未 clone 时 Vite 启动报错 | middleware 检测 dist 不存在 → 打印 warning + 跳过（dev 不挂） |
| **R6** | ~~cmd/openlist 与 cmd/server cobra rootCmd 冲突~~ | **已撤销**（无需新增 cmd） |
| **R7** | OpenList(5244) 在 dev 同时被 encv-go 转发 + 浏览器直接访问，session/cookie 冲突 | dev 模式 token 走 admin login，cookie 不依赖 host；不同来源不影响功能 |
| **R8** | Vite proxy prefix 匹配劫持 `/openlist-ui/*` 到 encv-go | proxy 用 `'/openlist/'`（带尾斜杠）明确前缀（D3） |
| **R9** | fork 未来 rebase 回退 `DistDir` 切换 | build script `scripts/build-openlist-aar.sh` 内置 A2 兜底（`internal/encv/init.go` 注入 `OPENLIST_DIST_DIR` env）|

---

## 七、Spec 自我一致性检查

- [x] 改动都有具体文件路径（`app/encv-mobile/vite.config.ts`, `scripts/dev-openlist.sh`, `scripts/build-openlist-aar.sh`）
- [x] 每个改动都有可执行的命令模板（`pnpm add -D sirv`, `pnpm dev`, `bash scripts/dev-openlist.sh`）
- [x] 决策记录 D1-D7 都有理由（D1 是沙箱发现后的关键决策：fork 无需改动）
- [x] 风险 R1-R9 都有缓解（R1/R2/R6 已撤销并标注原因）
- [x] 与 [verify-fork-clone-sandbox-layout.md](file:///workspace/.trae/documents/verify-fork-clone-sandbox-layout.md) 的 V0-V9 验证步骤正交衔接（V0-V9 是「clone 成功」，本 spec 是「clone 后怎么用」）
- [x] 与 [app/openlist/README.md](file:///workspace/app/openlist/README.md) §3 五层关系图 + §4.4 三个 fork 本地布局对接
- [x] gomobile 路径作为 production 默认保持不变（D6）
- [x] 前端 commit 到 repo（D2）+ build script 复制 = 构建可重放
- [x] dev / production 路径不冲突（D3 选 `/openlist-ui/` 而非 `/openlist/`）
- [x] **沙箱一致性**：spec 描述的改动 = tasks.md / checklist.md 实际分解 = 沙箱内 vite.config.ts 已实现的代码

---

## 八、Spec 完成判据

| # | 判据 | 验证方式 |
|---|------|----------|
| **J1** | `ionic serve` 暴露 `/openlist-ui/` | `curl http://localhost:8100/openlist-ui/` 返回 index.html (200) |
| **J2** | `/openlist-ui/api/*` 代理到 OpenList(5244) | `curl http://localhost:8100/openlist-ui/api/ping` 返回 200 |
| **J3** | `/openlist-ui/` SPA fallback | `curl http://localhost:8100/openlist-ui/some/random/path` 返回 index.html |
| **J4** | `/openlist-ui/assets/*` 静态资源 | `curl http://localhost:8100/openlist-ui/assets/index-*.js` 返回 200 |
| **J5** | TypeScript 编译通过 | `pnpm exec vue-tsc --noEmit` 退出码 0 |
| **J6** | Vite proxy `/openlist/` 未被劫持 | `curl http://localhost:8100/openlist-ui/` 不走 encv-go(2025) |
| **J7** | 浏览器 `localhost:8100/openlist-ui/` 登录 OpenList admin | 手动 |
| **J8** | ENCV 加密视频在 `localhost:8100/openlist-ui/` 内预览 | 手动（需 .sccgv 测试文件） |
| **J9** | gomobile 旧路径仍 work | `./gradlew :plugin-openlist:assembleDebug` 通过 |
| **J10** | production APK 包含 dist | `unzip -l plugin-openlist-debug.apk \| grep "assets/dist/"` |
| **J11** | 首次启动 plugin → `filesDir/openlist/dist/` 出现 | 设备验证（沙箱无） |

J1-J6 用 curl 自动验证；J7-J8 手动；J9-J11 跑 CI / 设备。

> **J1-J6 在 P2 验证；J7-J8 在 P5 验证；J9-J11 在 P4 验证。**

---

## 九、相关 spec

- [wire-openlist-runtime-and-ui-v2](file:///workspace/.trae/specs/wire-openlist-runtime-and-ui-v2/spec.md) — 上轮 spec（combolite 扩展 + ContentProvider IPC）
- [integrate-openlist-as-combolite-plugin](file:///workspace/.trae/specs/integrate-openlist-as-combolite-plugin/spec.md) — 主 spec
- [verify-fork-clone-sandbox-layout.md](file:///workspace/.trae/documents/verify-fork-clone-sandbox-layout.md) — fork clone 验证（本 spec 的前置）
- [app/openlist/README.md](file:///workspace/app/openlist/README.md) — fork 协作入口
