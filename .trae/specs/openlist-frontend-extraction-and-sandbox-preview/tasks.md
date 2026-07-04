# Tasks: OpenList Frontend Extraction + Sandbox Browser Preview

> **执行原则**：每完成一阶段跑 `verify-phase-N.sh` 自检脚本；任何 phase 失败 → 写 `failure-N.log` + 标注根因 + 进入修复模式（spec 不断迭代）。

## 阶段总览

| Phase | 目标 | 关键命令 | 预期时长 | 状态 |
|-------|------|----------|----------|------|
| **P0** | 修 gomobile Kotlin 编译错误 | `./gradlew :plugin-openlist:compileReleaseKotlin` | 已完成 | ✅ |
| **P1** | 写 spec + tasks + checklist | 本文件 | 已完成 | ✅ |
| **P2** | Vite sub-route middleware (C3) — 沙箱可独立验证 | `pnpm install sirv && ionic serve` | 1-2h | ✅ |
| **P2.5** | clone Hi-Sillot-OpenList-Frontend + dev-openlist.sh 优先用本地 dist | `git clone https://github.com/Hi-Sillot/OpenList-Frontend.git` | 10min | ✅ |
| **P3** | 沙箱 dev 启动脚本 (C4) | `bash scripts/dev-openlist.sh` | 30min | ✅ |
| **P4** | build script + plugin runtime (C5) | `bash scripts/build-openlist-aar.sh` + `OpenListBridge.kt` | 1-2h | ✅ 代码完成 / ⏳ 设备验证 |
| **P5** | 端到端沙箱验证（浏览器手动） | curl + 浏览器手动 | 1-2h | ⏳ 沙箱无浏览器，跳过 |
| **P6** | Capacitor live-reload 适配（可选） | `npx cap run android --livereload` | 2-3h | ⏳ P5 后 |

> **沙箱发现后的简化**：原 P3「fork PR」取消（Hi-Sillot 已有 `conf.Conf.DistDir` 等全部能力），原 P4 拆为 P3 (C4 dev 脚本) + P4 (C5 prod 路径)。spec 整体复杂度从「fork + encv-mobile 双侧」简化为「encv-mobile 仓库内单侧」。

---

## Phase 2 — Vite sub-route middleware (C3)

> **目标**：在 encv-mobile 沙箱不依赖 fork 改动的情况下，让 `ionic serve` 能提供 `/openlist-ui/` 静态服务。P2 完成后，浏览器访问 `localhost:8100/openlist-ui/` 看到 OpenList 静态资源（如果 dist/ 在本地），API 调用走 `/openlist-ui/api/*` 代理到 `OPENLIST_API_TARGET`（默认 `127.0.0.1:5244`）。
>
> **本阶段不需要 fork 改动**，可以独立验证。

### Task 2.1: 检查 fork dist 是否已存在

```bash
cd /workspace/app/openlist
ls -la Hi-Sillot-OpenList/public/dist/ 2>/dev/null && echo "DIST_FOUND" || echo "DIST_MISSING"
```

- ✅ `DIST_FOUND` → 进入 2.2
- ❌ `DIST_MISSING` → 先按 [verify-fork-clone-sandbox-layout.md §4.3 §4.5](file:///workspace/.trae/documents/verify-fork-clone-sandbox-layout.md) clone 三个 fork

### Task 2.2: 安装 sirv 依赖

```bash
cd /workspace/app/encv-mobile
pnpm add -D sirv
# 或 npm: npm install --save-dev sirv
```

**预期**：`package.json` 里增加 `"sirv": "^3.x.x"`；`pnpm-lock.yaml` 更新。

**失败处理**：
- `pnpm not found` → `npm install -g pnpm` 或改用 npm
- `EACCES` → 沙箱无 root，用 `pnpm add -D sirv --shamefully-hoist` 或换 npm

### Task 2.3: 改 vite.config.ts 加 openlist-ui-static plugin

修改 `/workspace/app/encv-mobile/vite.config.ts`：

```typescript
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import fs from 'node:fs'
import sirv from 'sirv'

const OPENLIST_DIST = path.resolve(__dirname, '../openlist/Hi-Sillot-OpenList/public/dist')
const OPENLIST_DIST_EXISTS = fs.existsSync(OPENLIST_DIST)
const OPENLIST_API_TARGET = process.env.OPENLIST_API_TARGET || 'http://127.0.0.1:5244'

// ... 保留原有 plugins 数组，新增 openlist-ui-static
```

完整代码见 [spec.md §4.2](file:///workspace/.trae/specs/openlist-frontend-extraction-and-sandbox-preview/spec.md)。

### Task 2.4: 验证 Vite 启动不报错

```bash
cd /workspace/app/encv-mobile
# 静默启动 5s，看启动日志
timeout 8 pnpm dev 2>&1 | head -30 || true
```

**预期输出关键行**：
```
VITE v5.x.x  ready in xxx ms
➜  Local:   http://localhost:8100/
[openlist-ui] dist found at /workspace/app/openlist/Hi-Sillot-OpenList/public/dist
```

**失败处理**：
- `Cannot find module 'sirv'` → Task 2.2 没装成功
- `[openlist-ui] dist not found` → Task 2.1 失败，按 verify-fork-clone-sandbox-layout.md §4.3 重 clone
- `EACCES /vite.config` → 改文件权限

### Task 2.5: 验证 `/openlist-ui/` 静态服务

```bash
# 假设 Task 2.4 已启 ionic serve 在后台（这里独立起一个做 curl 测试）
cd /workspace/app/encv-mobile
nohup pnpm dev > /tmp/vite-dev.log 2>&1 &
VITE_PID=$!
sleep 5

# 验证根路径
curl -sI http://localhost:8100/openlist-ui/ | head -3
# 期望: HTTP/1.1 200 OK
#       Content-Type: text/html

# 验证 dist 内具体文件
curl -sI http://localhost:8100/openlist-ui/index.html | head -3
# 期望: HTTP/1.1 200 OK
#       Content-Type: text/html

# 验证 SPA fallback（任意路径返回 index.html）
curl -s http://localhost:8100/openlist-ui/some/random/path | head -5
# 期望: <!DOCTYPE html> ...

# 清理
kill $VITE_PID 2>/dev/null
```

### Task 2.6: 验证 `/openlist-ui/api/*` 代理

```bash
# 先起一个假 OpenList 在 5244 模拟
nohup python3 -m http.server 5244 --directory /tmp/fake-openlist > /tmp/fake-openlist.log 2>&1 &
FAKE_PID=$!
echo '{"success":true,"data":{"ping":"pong"}}' > /tmp/fake-openlist/api/ping.json
sleep 1

# 再起 Vite
cd /workspace/app/encv-mobile
nohup pnpm dev > /tmp/vite-dev.log 2>&1 &
VITE_PID=$!
sleep 5

# 代理测试
curl -s http://localhost:8100/openlist-ui/api/ping.json
# 期望: {"success":true,"data":{"ping":"pong"}}

# 清理
kill $VITE_PID $FAKE_PID 2>/dev/null
rm -rf /tmp/fake-openlist
```

**失败处理**：
- 502 错误 → OpenList(5244) 不在跑，dev 模式可不阻塞开发
- 404 → middleware 顺序错，API 路径被 sirv 抢先
- CORS 错 → 不应出现（Vite proxy 转发保留 origin）

### Task 2.7: 跑 TypeScript 编译检查

```bash
cd /workspace/app/encv-mobile
pnpm exec vue-tsc --noEmit
```

**预期**：无错误退出。

**已知可能错**：
- `Cannot find module 'sirv'` → 检查 `pnpm add -D sirv` 是否完成
- `Type 'X' is not assignable to type 'Y'` → 检查 vite.config.ts 类型注解

### Phase 2 完成判据

- [x] sirv 装好
- [x] vite.config.ts 改完
- [x] `pnpm dev` 启动无错
- [x] `/openlist-ui/` 200 OK
- [x] `/openlist-ui/api/ping.json` 代理通
- [x] `vue-tsc --noEmit` 通过

---

## Phase 3 — 沙箱 dev 启动脚本 (C4)

> **目标**：写一个 `scripts/dev-openlist.sh`，把「下载 dist + 写 config.json + go run .」三步封装成单条命令，让用户一键启 OpenList(5244)。
>
> **依赖 P2 通过**（Vite middleware 已能服务 dist + 代理 API）。
>
> **不依赖 fork 改动**（Hi-Sillot 已有 cobra 入口 + `conf.Conf.DistDir` 配置）。

### Task 3.1: 写 dev-openlist.sh

新增 `app/encv-mobile/scripts/dev-openlist.sh`：

```bash
#!/usr/bin/env bash
# scripts/dev-openlist.sh
# 一键启动 OpenList(5244) dev 模式
# 用法： bash scripts/dev-openlist.sh [--port 5244] [--data ./data]

set -euo pipefail

PORT=5244
DATA_DIR="./data"
FORK_DIR="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}/../openlist/Hi-Sillot-OpenList"
OPENLIST_VERSION="${OPENLIST_VERSION:-4.1.8}"
WEB_VERSION="${WEB_VERSION:-v${OPENLIST_VERSION}}"

while [[ $# -gt 0 ]]; do
  case $1 in
    --port)   PORT="$2"; shift 2 ;;
    --data)   DATA_DIR="$2"; shift 2 ;;
    --fork)   FORK_DIR="$2"; shift 2 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

cd "${FORK_DIR}"
echo "[dev-openlist] Working dir: $(pwd)"

# 1. 确保 data 目录
mkdir -p "${DATA_DIR}"

# 2. 确保 dist 存在
if [[ ! -f "public/dist/index.html" ]]; then
  echo "[dev-openlist] dist not found, downloading OpenList-Frontend ${WEB_VERSION}..."
  TMP=$(mktemp -d)
  curl -fsSL -o "${TMP}/frontend.tar.gz" \
    "https://github.com/OpenListTeam/OpenList-Frontend/releases/download/${WEB_VERSION}/openlist-frontend-dist-${OPENLIST_VERSION}.tar.gz"
  tar -xzf "${TMP}/frontend.tar.gz" -C "${TMP}"
  mkdir -p public/dist
  # tar 解压到当前目录的子集（实际 release 是 tar -czf 整个 dist/）
  if [[ -d "${TMP}/dist" ]]; then
    cp -r "${TMP}/dist/." public/dist/
  fi
  echo "${WEB_VERSION}-encv" > public/dist/VERSION
  rm -rf "${TMP}"
fi

# 3. 写 config.json 启用 dist_dir
if [[ ! -f "config.json" ]]; then
  cat > config.json <<EOF
{
  "dist_dir": "./public/dist",
  "scheme": {
    "https": false,
    "cert_file": "",
    "key_file": ""
  }
}
EOF
  echo "[dev-openlist] config.json created with dist_dir=./public/dist"
fi

# 4. 启动
echo "[dev-openlist] Starting OpenList on :${PORT}, data=${DATA_DIR}"
exec go run . --data "${DATA_DIR}"
```

### Task 3.2: 沙箱内运行验证（无 Go 工具链时跳过）

```bash
cd /workspace/app/encv-mobile
chmod +x scripts/dev-openlist.sh
# 如果沙箱有 go 工具链：
bash scripts/dev-openlist.sh --port 5244 --data /tmp/openlist-data &
sleep 5
curl -sI http://127.0.0.1:5244/api/ping
# 期望: 200
```

**沙箱可能失败**：
- `go: command not found` → 沙箱无 Go 工具链，本地用户可跑
- `curl: (6) Could not resolve host` → 网络问题，dist 已在本地
- `public/dist/index.html not found` → 跳过下载，用本地 dist 启动

### Task 3.3: 验证脚本不破坏现有 build

```bash
# 检查脚本语法（不需要真跑）
bash -n scripts/dev-openlist.sh && echo "[OK] syntax check"
# 检查文件权限
ls -la scripts/dev-openlist.sh
# 期望: -rwxr-xr-x
```

### Phase 3 完成判据

- [ ] `scripts/dev-openlist.sh` 创建
- [ ] `bash -n scripts/dev-openlist.sh` 语法 OK
- [ ] 脚本可执行权限设置
- [ ] 沙箱内有 Go 时能启 OpenList（沙箱无 Go 可跳过）

---

## Phase 4 — build script + plugin runtime (C5)

> **目标**：production 路径下，frontend dist 从 build script 复制到 `plugin-openlist/src/main/assets/dist/`，由 `OpenListBridge.init()` 首次启动时解压到 `filesDir/openlist/dist/`，传给 `Openlistlib.SetConfigAssetsDir(...)`。
>
> **依赖 P2 通过**（Vite middleware 在沙箱可独立验证前端可达）。
>
> **不依赖 fork 改动**。

### Task 4.1: build-openlist-aar.sh 复制 frontend

修改 `scripts/build-openlist-aar.sh` 在 frontend dist 下载完成后追加：

```bash
# === P4: 把 frontend dist 复制到 plugin APK assets ===
DEST="${REPO_ROOT}/app/encv-mobile/plugin-openlist/src/main/assets/dist"
if [[ -d "${SRC_DIR}/public/dist" ]]; then
    rm -rf "${DEST}"
    cp -r "${SRC_DIR}/public/dist" "${DEST}"
    echo "[INFO][P4] frontend dist copied to ${DEST} ($(du -sh ${DEST} | cut -f1))"
    # 写 VERSION 文件
    echo "${WEB_VERSION}-encv" > "${DEST}/VERSION"
else
    echo "[WARN][P4] ${SRC_DIR}/public/dist not found, assets/dist will be empty"
fi
```

### Task 4.2: OpenListBridge 加 ensureAssetsExtracted

修改 `app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt`：

```kotlin
// 加在 init() 内，SetConfigData 之前
val assetsDir = ensureAssetsExtracted(context)
if (assetsDir.isNotEmpty()) {
    Openlistlib.SetConfigAssetsDir(assetsDir)
}

// 加 helper
private fun ensureAssetsExtracted(context: Context): String {
    val target = File(context.filesDir, "openlist/dist")
    val versionFile = File(target, "VERSION")
    val bundledVersion = try {
        context.assets.open("dist/VERSION").bufferedReader().readText().trim()
    } catch (e: Throwable) { "" }
    if (target.exists() && versionFile.exists() && versionFile.readText().trim() == bundledVersion) {
        return target.absolutePath
    }
    target.deleteRecursively()
    target.mkdirs()
    copyAssetDir(context, "dist", target.absolutePath)
    return target.absolutePath
}

private fun copyAssetDir(context: Context, srcAssetPath: String, destPath: String) {
    val assets = context.assets.list(srcAssetPath) ?: return
    val dest = File(destPath)
    if (!dest.exists()) dest.mkdirs()
    for (asset in assets) {
        val sub = if (srcAssetPath.isEmpty()) asset else "$srcAssetPath/$asset"
        val outFile = File(dest, asset)
        // 简化：假设非目录（dist/ 顶层是文件）；递归调用
        try {
            context.assets.open(sub).use { input ->
                outFile.outputStream().use { input.copyTo(it) }
            }
        } catch (e: Throwable) {
            // 是目录
            copyAssetDir(context, sub, outFile.absolutePath)
        }
    }
}
```

### Task 4.3: 验证编译

```bash
cd /workspace/app/encv-mobile/android
./gradlew :plugin-openlist:assembleDebug 2>&1 | tail -30
```

**预期**：`BUILD SUCCESSFUL`。

**关键 API 修正**：确保 `openlistlib.Openlistlib.SetConfigAssetsDir(String)` 是 gomobile 生成的 static method（与 `SetConfigData` 同形）。

### Task 4.4: 验证 APK 包含 dist

```bash
find . -name "plugin-openlist-*.apk" -path "*/outputs/*"
APK=$(find . -name "plugin-openlist-*.apk" -path "*/outputs/*" | head -1)
unzip -l "$APK" | grep -E "assets/dist|index.html" | head
# 期望: assets/dist/index.html, assets/dist/assets/...
```

### Phase 4 完成判据

- [ ] `bash scripts/build-openlist-aar.sh` 跑通
- [ ] `./gradlew :plugin-openlist:assembleDebug` 成功
- [ ] APK 内含 `assets/dist/`
- [ ] 设备上首次启 plugin → `filesDir/openlist/dist/` 出现

---

## Phase 5 — 端到端沙箱验证

> **目标**：浏览器 `localhost:8100/openlist-ui/` 看到 OpenList 完整 UI，能登录 admin，能看到文件列表。

### Task 5.1: 启三个进程

```bash
# 终端 1: OpenList
cd /workspace/app/openlist/Hi-Sillot-OpenList
go run ./cmd/openlist --port 5244 --data ./data --log-std
# 等待看到 "start HTTP server @ :5244"

# 终端 2: encv-go (按现有方式)
cd /workspace
./bin/encv-go -port 2025  # 或 go run ./cmd/encv

# 终端 3: encv-mobile dev server
cd /workspace/app/encv-mobile
pnpm dev
# 等待看到 "Local: http://localhost:8100/"
```

### Task 5.2: curl 链路自检

```bash
# 直接访问 OpenList
curl -sI http://127.0.0.1:5244/api/ping
# 期望: 200

# 浏览器内访问的路径
curl -sI http://localhost:8100/openlist-ui/
# 期望: 200, text/html

curl -sI http://localhost:8100/openlist-ui/api/ping
# 期望: 200 (proxy 到 OpenList(5244))

# encv-go 链路（既有，不应回归）
curl -sI http://localhost:8100/openlist/sites/local-loopback/d/test.sccgv
# 期望: 200/302/401（取决于 token 配置）
```

### Task 5.3: 浏览器手动验证

打开 `http://localhost:8100/openlist-ui/`，期待看到：
- OpenList 登录页（admin 账号）
- 登录后看到文件列表
- 能创建存储、浏览、上传

### Task 5.4: ENCV 视频预览链路

如果有 .sccgv 测试文件：
1. 配置 ENCV 存储（`internal/conf/const.go` 里的密码）
2. 访问 `/openlist-ui/d/path/to/test.sccgv`
3. 期待：解密后流回浏览器，可播放

### Task 5.5: 修改迭代速度实测

| 改动 | 旧耗时 | 新耗时 |
|------|--------|--------|
| 改 OpenList 前端一行（`/index.html` 标题） | ~10min | <500ms |
| 改 `internal/conf/const.go` 一个 ENCV 字段 | ~10min | <3s（go run 重启） |
| 改 encv-mobile Vue 组件 | <1s | <1s |

记录在 `iteration-speed.md`。

### Phase 5 完成判据

- [ ] 三个进程都能启
- [ ] curl 自检全过
- [ ] 浏览器看到 OpenList 完整 UI
- [ ] admin 登录可用
- [ ] 文件浏览可用
- [ ] 迭代速度达预期

---

## Phase 6 — Capacitor live-reload 适配（可选）

> **目标**：让 Android WebView 也走 Vite dev server（`npx cap run android`），真实设备上也能 hot-reload。
>
> **依赖**：P5 全通。
> **可选性**：纯 dev 体验优化，不影响功能。

### Task 6.1: 配 Capacitor server.url

修改 `app/encv-mobile/capacitor.config.ts` 加 dev 配置：

```typescript
import { CapacitorConfig } from '@capacitor/cli'

const config: CapacitorConfig = {
  appId: 'com.encvgo.app',
  appName: 'encv-go',
  webDir: 'dist',
  // dev mode only: cap copy then cap run will use this
  server: {
    androidScheme: 'https',
    // 留空让 npx cap run --livereload 注入
  },
  // ...
}
```

### Task 6.2: 启 live-reload 跑通

```bash
cd /workspace/app/encv-mobile
npx cap copy android
cd android
./gradlew assembleDebug
cd ..
npx cap run android --livereload --target=<device>
# 期望: WebView 加载 host:8100，改 Ionic 组件即时热更
```

### Phase 6 完成判据

- [ ] Android WebView 加载 `http://<host>:8100`
- [ ] 改 Ionic 组件即时热更
- [ ] 改 OpenList UI 也热更（Vite middleware 触发 SPA reload）

---

## 失败处理模板

```bash
# 记录失败
mkdir -p .trae/specs/openlist-frontend-extraction-and-sandbox-preview/failures
cat > .trae/specs/openlist-frontend-extraction-and-sandbox-preview/failures/failure-<N>.md <<EOF
# Failure <N>: <title>

**Phase**: P2/P3/P4/P5/P6
**Task**: Task 2.x / 3.x / ...
**Date**: $(date -I)

## 复现命令

\`\`\`bash
<exact commands>
\`\`\`

## 实际输出

\`\`\`
<stderr + stdout>
\`\`\`

## 根因

<analysis>

## 修复

<solution>
EOF
```

---

## 自检脚本（每 phase 末跑）

```bash
# verify-phase-N.sh 模板
PHASE="${1:-2}"
echo "=== Verifying Phase $PHASE ==="
case $PHASE in
    2)
        cd /workspace/app/encv-mobile
        pnpm exec vue-tsc --noEmit && echo "[OK] vue-tsc passed"
        grep -q "openlist-ui-static" vite.config.ts && echo "[OK] vite plugin registered"
        [[ -d "node_modules/sirv" ]] && echo "[OK] sirv installed"
        ;;
    3)
        cd /workspace/app/openlist/Hi-Sillot-OpenList
        [[ -f "cmd/openlist/main.go" ]] && echo "[OK] cmd/openlist/main.go exists"
        grep -q "SetConfigAssetsDir" openlistlib/settings.go && echo "[OK] SetConfigAssetsDir declared"
        ;;
    4)
        grep -q "frontend dist copied" scripts/build-openlist-aar.sh && echo "[OK] build script updated"
        grep -q "ensureAssetsExtracted" app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt && echo "[OK] bridge updated"
        ;;
    3)
        # Phase 3 = C4 dev script
        [[ -f "app/encv-mobile/scripts/dev-openlist.sh" ]] && echo "[OK] dev-openlist.sh exists"
        bash -n app/encv-mobile/scripts/dev-openlist.sh && echo "[OK] syntax OK"
        ;;
    5)
        curl -sf http://localhost:8100/openlist-ui/ > /dev/null && echo "[OK] /openlist-ui/ 200"
        curl -sf http://localhost:8100/openlist-ui/api/ping > /dev/null && echo "[OK] /openlist-ui/api/ping 200"
        ;;
esac
echo "=== Phase $PHASE verification complete ==="
```
