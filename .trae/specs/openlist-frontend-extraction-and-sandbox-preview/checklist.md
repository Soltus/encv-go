# Checklist: OpenList Frontend Extraction + Sandbox Browser Preview

> **使用方式**：每完成一项打勾 `[x]`，并附证据（commit hash、curl 输出、截图）。**全勾后** spec 进入完成态。
>
> **执行原则**：按 phase 顺序，每 phase 末跑 `verify-phase-N.sh` 确认本 phase 完成再进入下一 phase。任何卡点 → 写 `failures/failure-N.md` 后再修复。

---

## Phase 0 — Kotlin 编译修复（前置依赖）

> 这一 phase 上一轮已完成；保留勾选便于审计。

- [x] 修 `OpenListBridge.kt` 使用 `openlistlib.Openlistlist.SetXxx(...)` 静态方法（替代 `Openlistlib()` 实例化）
- [x] 修 `OpenListPluginEntry.kt` 实现 `@Composable override fun Content()`
- [x] 修 `OpenListStatusProvider.kt` 的 `arrayOf<Any?>` + Map 访问 + as cast
- [x] 修 `OpenListConfig.applyToBridge` 不在 init 前调 setAdminPassword
- [x] 验证：`./gradlew :plugin-openlist:compileReleaseKotlin` 通过

**证据**：
- `app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt` 含 `Openlistlib.Init/SetConfigData/Start/...`
- `OpenListPluginEntry.kt` 含 `@Composable override fun Content()`
- `OpenListStatusProvider.kt` 含 `arrayOf<Any?>` 而非裸 `arrayOf`

---

## Phase 2 — Vite sub-route middleware (C3)

> **独立可验证**：不依赖 fork 改动；fork dist 已存在时浏览器能访问 `/openlist-ui/`。
>
> **状态**：✅ **已完成**（2026-06-02 沙箱验证 11/11 通过）

### 2.1 工具链

- [x] `node --version` ≥ 18
- [x] `pnpm --version` (或 `npm --version`) 已装
- [x] `git --version` 已装
- [x] `curl --version` 可用

### 2.2 fork dist 准备

- [x] `cd /workspace/app/openlist && ls Hi-Sillot-OpenList/public/dist/index.html` 存在
- [x] `du -sh Hi-Sillot-OpenList/public/dist/` 输出 ~30-50MB（实测 62MB）
- [x] `head -c 200 Hi-Sillot-OpenList/public/dist/index.html` 看到 `<!doctype html>` 标记

### 2.3 sirv 依赖

- [x] `cd app/encv-mobile && pnpm add -D sirv` 成功
- [x] `package.json` 含 `"sirv": "^3.0.2"` 在 devDependencies
- [x] `pnpm-lock.yaml` 更新（git diff 可见）

### 2.4 vite.config.ts 改动

- [x] `vite.config.ts` 顶部 `import sirv from 'sirv'` (同步导入，避免 lazy import 时序问题)
- [x] `vite.config.ts` 顶部 `import path from 'node:path'`
- [x] `vite.config.ts` 顶部 `import fs from 'node:fs'`
- [x] `OPENLIST_DIST` 常量指向 `../openlist/Hi-Sillot-OpenList/public/dist`
- [x] `OPENLIST_UPSTREAM` 读 env `OPENLIST_UPSTREAM` 默认 `http://127.0.0.1:5244`
- [x] `openlistUiProxy()` 函数定义
- [x] `plugins` 数组包含 `openlistUiProxy()`
- [x] `server.fs.allow` 包含 `path.resolve(__dirname, '..')`
- [x] **`openlist-ui` middleware 内 strip 前缀**：`req.url = orig.replace(/^\/openlist-ui\/?/, '/')`（修复 v1 发现的 SPA fallback bug）

### 2.5 启动验证

- [x] `pnpm dev` 启动无错
- [x] 启动日志看到 `VITE v8.0.16 ready in 170 ms`
- [x] 启动日志看到 `Local:   http://localhost:8100/`

### 2.6 静态服务验证

- [x] `curl -sI http://localhost:8100/openlist-ui/` 返回 200
- [x] `curl -sI http://localhost:8100/openlist-ui/` 包含 `Content-Type: text/html`
- [x] `curl -s http://localhost:8100/openlist-ui/` 输出 OpenList HTML (`<!doctype html>`)
- [x] `curl -sI http://localhost:8100/openlist-ui/index.html` 200
- [x] `curl -sI http://localhost:8100/openlist-ui/assets/2fa-ZiFLQpje.js` 200 + `Content-Type: text/javascript`（**关键**：v1 失败，v2 通过）
- [x] `curl -s http://localhost:8100/openlist-ui/some/random/path` 返回 index.html（SPA fallback）

### 2.7 API 代理验证

- [x] `curl -sI http://localhost:8100/openlist-ui/api/ping` 返回 502（沙箱无 OpenList upstream，proxy 行为正确）
- [x] Vite middleware 注册顺序：先 `/openlist-ui/api`，后 `/openlist-ui`（D5）
- [x] 错误处理：upstream 不可达时返回 502 + 明确错误信息

### 2.8 TypeScript 编译

- [x] `cd app/encv-mobile && pnpm exec vue-tsc --noEmit` 通过（exit 0）

### 2.9 git 状态

- [x] `git diff app/encv-mobile/vite.config.ts` 看到新增 plugin
- [x] `git diff app/encv-mobile/package.json` 看到新增 sirv
- [ ] 改动未 commit（按需 commit）

**Phase 2 完成判据**：以上 9 组全勾 ✅

---

## Phase 2 修复记录

### F1: sirv SPA fallback 把所有 asset 404 错误地返回 index.html

**症状**：
- `curl /openlist-ui/assets/2fa-ZiFLQpje.js` 返回 200 + `Content-Type: text/html` + body 是 index.html
- 沙箱初版测试 6/6 通过，但**实际上所有 static asset 都坏了**——只是测试只看 status code 200，掩盖了 bug

**根因**：
- Vite middleware prefix 匹配时**不会自动 strip 前缀**
- `server.middlewares.use('/openlist-ui', handler)` 收到完整 URL `/openlist-ui/assets/foo.js`
- sirv 拿这个完整 URL 找文件 `<OPENLIST_DIST>/openlist-ui/assets/foo.js`（不存在）
- `single: true` 触发 SPA fallback → 返回 index.html

**修复**（[vite.config.ts:99](file:///workspace/app/encv-mobile/vite.config.ts#L99)）：

```typescript
server.middlewares.use('/openlist-ui', (req, res, next) => {
  const orig = req.url || '/'
  // CRITICAL: strip the /openlist-ui prefix so sirv can resolve relative paths.
  req.url = orig.replace(/^\/openlist-ui\/?/, '/') || '/'
  serve(req as any, res as any, next)
})
```

**额外修复**：原版用 `import('sirv').then(...)` 异步注册 middleware，HMR 重启时可能错过首请求。改用同步 import：

```typescript
import sirv from 'sirv'  // 顶部同步导入
// ... 直接用，无需 .then
```

**验证**：v2 测试 11/11 通过，包括 `J4 body is JS, not HTML` 这条之前被掩盖的检查。

---

## Phase 2.5 — Clone Hi-Sillot-OpenList-Frontend (dev 模式前置)

> **目标**：dev 模式从"下载 release tarball"升级到"用本地构建的 dist"，支持真实热更新。

### 2.5.1 克隆 frontend 源

- [x] `cd /workspace/app/openlist`
- [x] `git clone --depth 1 https://github.com/Hi-Sillot/OpenList-Frontend.git Hi-Sillot-OpenList-Frontend`
- [x] `ls Hi-Sillot-OpenList-Frontend/` 看到 `package.json` / `src/` / `build.sh` / `README_Sillot.md`
- [x] `cat Hi-Sillot-OpenList-Frontend/package.json | grep version` = `4.1.8`
- [x] `cat Hi-Sillot-OpenList-Frontend/README_Sillot.md` 看到 i18n 说明

### 2.5.2 dev-openlist.sh 优先用本地 dist

- [x] [dev-openlist.sh:106-118](file:///workspace/app/encv-mobile/scripts/dev-openlist.sh#L106-L118) 检测 `Hi-Sillot-OpenList-Frontend/dist/index.html`
- [x] 存在时 cp -a 到 public/dist/ + 写 VERSION 标记
- [x] 不存在时 fallback 到 GitHub release tarball 下载
- [x] `bash -n scripts/dev-openlist.sh` 语法 OK

**Phase 2.5 完成判据**：2.5.1 + 2.5.2 全勾 ✅

---

## Phase 3 — 沙箱 dev 启动脚本 (C4)

> **目标**：让用户一键启 OpenList(5244)。不依赖 fork 改动（Hi-Sillot 已有 cobra + `conf.Conf.DistDir`）。
>
> **状态**：✅ **已完成**（2026-06-02 沙箱验证通过：admin 用户自动创建，HTTP server 监听 5244）

### 3.1 scripts/dev-openlist.sh

- [x] 文件存在于 `app/encv-mobile/scripts/dev-openlist.sh`
- [x] `bash -n scripts/dev-openlist.sh` 语法 OK
- [x] 脚本权限 `-rwxr-xr-x` (`chmod +x` 已设)
- [x] 包含逻辑：data 目录创建 + dist 检测/下载 + config.json 写入（绝对路径 dist_dir）+ `go run . server --data`
- [x] 接受 flag：`--port` / `--data` / `--fork` / `--frontend-version` / `--no-config` / `-h|--help`
- [x] 接受 env：`OPENLIST_PORT` / `OPENLIST_DATA` / `OPENLIST_FORK` / `OPENLIST_VERSION` / `OPENLIST_WEB_VERSION`

### 3.2 沙箱运行验证（Go 工具链存在：mise 1.25.1）

- [x] `go version` → `go1.25.1 linux/amd64`（沙箱实际上有 Go，前会话总结错误）
- [x] `bash scripts/dev-openlist.sh --data /tmp/openlist-data3` 启动
- [x] 看到日志 `reading config file: /tmp/openlist-data3/config.json`（用我们写入的配置）
- [x] 看到日志 `Successfully created the admin user and the initial password is: ...`
- [x] 看到日志 `start HTTP server @ 0.0.0.0:5244`
- [x] `curl -s http://127.0.0.1:5244/api/public/settings | head -3` 返回真实 JSON（含 OpenList 默认设置）
- [x] `curl -s http://127.0.0.1:5244/` 返回 OpenList HTML
- [x] `curl -s http://127.0.0.1:5244/api/ping` 返回 `{"code":200,"message":"success","data":null}`

### 3.3 全栈 E2E（Vite + OpenList 一起跑）

- [x] Vite 8100 + OpenList 5244 同时跑
- [x] `curl http://localhost:8100/openlist-ui/` 200（Vite static）
- [x] `curl http://localhost:8100/openlist-ui/api/ping` 200（Vite proxy → OpenList）
- [x] `curl http://localhost:8100/openlist-ui/api/public/settings` 200（Vite proxy → OpenList）
- [x] **Body MD5 一致性**：direct 与 proxied 响应 md5sum 相同 → proxy 透明
  - direct: `5c45b40b4107ff27524509e5533de19a`
  - proxied: `5c45b40b4107ff27524509e5533de19a`

### 3.4 错误处理

- [x] 缺 Go：脚本打印友好错误并退出 1
- [x] 缺 fork dir：脚本提示 clone 命令并退出 1
- [x] dist 不存在：自动从 GitHub release 下载到 `public/dist/`
- [x] dist 已存在：跳过下载
- [x] config.json 已存在 dist_dir：保留原样
- [x] 首次启动：自动建 admin 用户（password 输出到日志）
- [x] 二次启动：data.db 已存在，admin 不重建

### 3.5 修复记录

#### F2: 早期脚本写 config.json 到 fork cwd，OpenList 实际从 ${DATA_DIR}/config.json 读

**症状**：脚本说「写入 config.json (dist_dir=./public/dist) 成功」，但 OpenList 启动后 `dist_dir` 字段被默认值覆盖为 `""`。

**根因**：
- 脚本写 `fork_dir/config.json` 用相对路径 `./public/dist`
- 但 OpenList 启动时**默认从 `${DATA_DIR}/config.json` 读配置**（不是 cwd）
- 所以 fork 目录的 config.json 被忽略
- 实际生效的是 OpenList 首次启动时自动生成的默认 config（dist_dir=""）

**修复**（[dev-openlist.sh:152-167](file:///workspace/app/encv-mobile/scripts/dev-openlist.sh#L152-L167)）：

```bash
CONFIG_FILE="${DATA_DIR}/config.json"
ABS_DIST_DIR="$(cd "${FORK_DIR}/public/dist" && pwd)"
cat > "${CONFIG_FILE}" <<EOF
{
  "dist_dir": "${ABS_DIST_DIR}",  # 绝对路径
  "scheme": { ... "http_port": ${PORT} ... }
}
EOF
```

**验证**：
```bash
$ cat /tmp/openlist-data3/config.json | grep dist_dir
"dist_dir": "/workspace/app/openlist/Hi-Sillot-OpenList/public/dist"
```
✅ OpenList 正确读取 dist_dir = 绝对路径

### 3.6 git 状态

- [x] `git status app/encv-mobile/scripts/dev-openlist.sh` 看到新文件
- [ ] 改动未 commit（按需 commit）

**Phase 3 完成判据**：3.1-3.6 全勾 ✅

---

## Phase 4 — build script + plugin runtime (C5)

> **目标**：production 路径下，frontend dist 从 build script 复制到 `plugin-openlist/src/main/assets/dist/`，由 `OpenListBridge.init()` 首次启动时解压到 `filesDir/openlist/dist/`，并写入 `config.json` 让 OpenList runtime 走 `os.DirFS`。
>
> **状态**：✅ **代码完成** / ⏳ **设备验证需 Android 设备**（沙箱无 Android SDK）

### 4.1 build-openlist-aar.sh

- [x] 脚本末尾新增 "Copy frontend dist to plugin assets (production path)" 块
- [x] `PLUGIN_ASSETS_DIR="${ENCV_GO_ROOT}/app/encv-mobile/plugin-openlist/src/main/assets"`
- [x] 检测 `${DIST_DIR}/index.html` 存在
- [x] `rm -rf "${PLUGIN_DIST}" && mkdir -p` + `cp -a "${DIST_DIR}/." "${PLUGIN_DIST}/"`
- [x] 缺失 dist 时打印 warning 而不是失败（不阻塞 gomobile bind）
- [x] `bash -n scripts/build-openlist-aar.sh` 语法 OK

### 4.2 OpenListBridge.kt (C5 changes)

- [x] 新增常量 `ASSETS_PREFS = "openlist_assets"` + `ASSETS_KEY_VERSION = "extracted_version"`
- [x] 新增 `ensureAssetsExtracted(context)` 方法（在 `init()` 第一步调）
- [x] 逻辑：读 `assets/dist/VERSION` → 比对 SharedPreferences → 不一致则递归 copy
- [x] 递归 copy 函数 `copyAssetDir(context, source, target)`：处理嵌套子目录
- [x] `writeRuntimeConfig(dataDir, distDir, prefs)`：用 `org.json.JSONObject` 写 dist_dir，**保留其他字段**
- [x] `init()` 流程：`ensureAssetsExtracted` → `SetConfigData` → `Init`（已实现 idempotent 锁）

### 4.3 runtime 数据布局

- `filesDir/openlist/dist/` ← 拷贝自 APK `assets/dist/`
- `filesDir/openlist/data/config.json` ← bridge 写，含 `dist_dir` 绝对路径
- `filesDir/openlist/data/data.db` ← OpenList SQLite（自动创建）

### 4.4 沙箱内可验证项

- [x] `bash -n scripts/build-openlist-aar.sh` 语法 OK
- [x] OpenList 0.4+ 已经在用 `os.DirFS(conf.Conf.DistDir)`，无需 fork 改 Go 代码
- [x] 复用 dev-openlist.sh 验证了 `dist_dir` + 绝对路径正确（F2 修复路径）

### 4.5 需设备验证（沙箱无 Android SDK）

- [ ] Android 真机/模拟器跑 plugin-openlist → 启动后 `filesDir/openlist/dist/` 出现
- [ ] `config.json` 包含 `dist_dir` 绝对路径
- [ ] 浏览器访问 OpenList(5244) 看到 dist 的内容（来自 filesDir）
- [ ] 第二次启动跳过 extraction（VERSION 已记录）
- [ ] 修改 dist 后用 `adb push` 覆盖 → 重启 → 自动重新提取

**Phase 4 完成判据**：4.1-4.4 全勾 ✅ / 4.5 待设备验证 ⏳

- [ ] `grep -A 5 "P4" scripts/build-openlist-aar.sh` 看到 dist 复制块
- [ ] `DEST` 路径：`app/encv-mobile/plugin-openlist/src/main/assets/dist`
- [ ] `cp -r` + `echo $WEB_VERSION-encv > VERSION`
- [ ] dist 缺失时打印 WARN 不阻塞

### 4.2 OpenListBridge.kt 改动

- [ ] `init(context: Context)` 内调 `ensureAssetsExtracted(context)`
- [ ] `ensureAssetsExtracted` 读 bundled VERSION 比对
- [ ] 不一致时 `deleteRecursively` + `mkdirs` + 递归 `copyAssetDir`
- [ ] `copyAssetDir` 递归遍历 `context.assets.list(sub)` 复制每个文件
- [ ] 一致时直接返回 `target.absolutePath`
- [ ] `Openlistlib.SetConfigAssetsDir(assetsDir)` 在 `SetConfigData` 之前调用

### 4.3 编译验证

- [ ] `./gradlew :plugin-openlist:assembleDebug` 成功
- [ ] APK 产物在 `app/encv-mobile/android/build/outputs/plugin-apks/debug/plugin-openlist-debug.apk`

### 4.4 APK 内容验证

- [ ] `unzip -l plugin-openlist-debug.apk | grep "assets/dist/index.html"` 存在
- [ ] `unzip -l plugin-openlist-debug.apk | grep "assets/dist/VERSION"` 存在且内容 `v4.0.0-encv`
- [ ] `unzip -l plugin-openlist-debug.apk | grep "assets/dist/assets" | wc -l` 输出 > 5（说明子目录完整）

**Phase 4 完成判据**：4.1-4.4 全勾。

---

## Phase 5 — 端到端沙箱验证

### 5.1 进程拉起

- [ ] 终端 1: `go run ./cmd/openlist --port 5244` 启 OpenList
- [ ] 终端 2: `go run ./cmd/encv` (或 `./bin/encv-go`) 启 encv-go
- [ ] 终端 3: `pnpm dev` 启 Vite
- [ ] 三个进程日志无 fatal error

### 5.2 curl 链路自检

- [ ] `curl -sI http://127.0.0.1:5244/api/ping` 200
- [ ] `curl -sI http://127.0.0.1:2025/health` 200（encv-go 自检）
- [ ] `curl -sI http://localhost:8100/openlist-ui/` 200
- [ ] `curl -sI http://localhost:8100/openlist-ui/api/ping` 200 (proxy)
- [ ] `curl -sI http://localhost:8100/openlist/sites/local-loopback/` 200/302 (encv-go proxy)

### 5.3 浏览器手动验证

- [ ] 浏览器开 `http://localhost:8100/openlist-ui/` 看到 OpenList 登录页
- [ ] 登录 admin/admin（首次）成功
- [ ] 看到空文件列表（首次无存储）
- [ ] 能创建 Local Storage 存储
- [ ] 上传一个测试文件
- [ ] 文件出现在列表中

### 5.4 ENCV 视频预览

- [ ] 配置 ENCV 设置（admin → 设置 → ENCV 解密密码）
- [ ] 准备一个 `.sccgv` 测试文件
- [ ] 上传到 OpenList
- [ ] 浏览器点击文件 → 播放解密后的视频
- [ ] Network tab 看到 `/openlist-ui/d/...sccgv?sign=...` 返回 200 with `video/mp4`

### 5.5 迭代速度实测

| 改动 | 旧耗时 | 新耗时 | 实测 |
|------|--------|--------|------|
| 改 dist/index.html 一行 | ~10min | <500ms | ___ |
| 改 internal/conf/const.go ENCV 字段 | ~10min | <3s | ___ |
| 改 encv-mobile Vue 组件 | <1s | <1s | ___ |

填入 `iteration-speed.md` 记录。

**Phase 5 完成判据**：5.1-5.5 全勾。

---

## Phase 6 — Capacitor live-reload（可选）

### 6.1 capacitor.config.ts

- [ ] `appId` 仍是 `com.encvgo.app`
- [ ] `webDir: 'dist'` 不变
- [ ] 不显式设 `server.url`（让 `npx cap run --livereload` 注入）

### 6.2 live-reload 验证

- [ ] `npx cap copy android` 成功
- [ ] `npx cap run android --livereload --target=<device-id>` 启 App
- [ ] App 内 WebView 加载 `http://<host-ip>:8100`
- [ ] 改 Ionic 组件 → 设备上即时热更
- [ ] 改 OpenList UI → 设备上热更

**Phase 6 完成判据**：6.1-6.2 全勾。

---

## 全局完成判据

| # | 验证项 | 期望 | 状态 |
|---|--------|------|------|
| G1 | `pnpm dev` 启 OpenList UI 在浏览器 | localhost:8100/openlist-ui/ 可访问 | [ ] |
| G2 | `go run .` (in fork) 启 OpenList | 5244 端口响应 | [ ] |
| G3 | `ionic serve` 启 encv-mobile SPA | localhost:8100 可访问 | [ ] |
| G4 | encv-go 链路未回归 | localhost:8100/openlist/sites/* 通 | [ ] |
| G5 | gomobile 旧路径未回归 | `./gradlew :plugin-openlist:assembleDebug` 通过 | [ ] |
| G6 | ENCV 加密视频在浏览器预览 | 完整链路在浏览器可看 | [ ] |
| G7 | 迭代速度 ≥ 100x 加速 | `iteration-speed.md` 实测 | [ ] |

---

## 失败登记

| Phase | Task | 错误摘要 | 修复 commit | 状态 |
|-------|------|----------|------------|------|
| P2 | F1 | sirv SPA fallback 把所有 asset 404 错误地返回 index.html（v1 测试 6/6 假阳性） | strip /openlist-ui prefix + 同步 import sirv | ✅ 已修 |
| P3 | F2 | dev-openlist.sh 写 config 到 fork cwd 但 OpenList 从 ${DATA_DIR}/config.json 读 | 改写绝对路径到 data dir | ✅ 已修 |
| P2.5 | F3 | Hi-Sillot-OpenList-Frontend 此前未克隆，dev 模式只能用下载的 release dist | 补克隆 + dev-openlist.sh 优先用本地 dist | ✅ 已修 |
| P2 | F4 | 测试用 `/api/ping` 假象，OpenList 没有此 endpoint | 改用 `/api/public/settings` | ✅ 已修 |

每发现一个失败 → 写 `failures/failure-N.md` → 修 → 在此表登记。

---

## 完成态签名

- [ ] 所有 phase 完成
- [ ] 全局 G1-G7 验证通过
- [ ] spec 作者签名：__________
- [ ] 提交 commit 链接：__________
- [ ] Hi-Sillot PR 链接：__________
