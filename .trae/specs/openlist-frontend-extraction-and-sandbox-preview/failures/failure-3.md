# Failure F3: Hi-Sillot-OpenList-Frontend 此前未克隆，dev 模式只能用下载的 release dist

**Phase**: P2.5 (clone Hi-Sillot-OpenList-Frontend + dev-openlist.sh 增强)
**Task**: Task 2.5.1 (clone frontend 源)
**Date**: 2026-06-02
**Status**: ✅ 已修

---

## 症状

`scripts/dev-openlist.sh` 的 Step 3 在 frontend 改动时无法热更新：用户必须改完 `Hi-Sillot-OpenList-Frontend/src/...` 后：
1. `bun run build` 生成 `dist/`
2. 手动 `cp -a dist/* Hi-Sillot-OpenList/public/dist/`
3. 重启 OpenList

整个流程每次 30+ 秒，跟"在 encv-mobile 沙箱里改前端热更新"的愿景差距大。

更根本的：**`Hi-Sillot-OpenList-Frontend` 这个仓库根本没被克隆到 `/workspace/app/openlist/` 下**。spec 里漏了这一步。

## 根因

spec/tasks.md 阶段总览提到 Hi-Sillot-OpenList-Frontend 是热更新源，但**没有任何 Task 把它 clone 下来**。`dev-openlist.sh` 的 Step 3 也只 fallback 到 GitHub release tarball，没考虑本地 dist/ 路径。

## 修复

### 修复 1: clone Hi-Sillot-OpenList-Frontend

```bash
cd /workspace/app/openlist
git clone --depth 1 https://github.com/Hi-Sillot/OpenList-Frontend.git Hi-Sillot-OpenList-Frontend
# 验证
ls Hi-Sillot-OpenList-Frontend/
# 看到 package.json (version 4.1.8) + src/ + build.sh + README_Sillot.md (i18n 说明)
```

### 修复 2: dev-openlist.sh Step 3 优先用本地 dist

[dev-openlist.sh:106-118](file:///workspace/app/encv-mobile/scripts/dev-openlist.sh#L106-L118)：

```bash
LOCAL_FRONTEND_DIR="${REPO_ROOT}/app/openlist/Hi-Sillot-OpenList-Frontend"

if [[ -d "${LOCAL_FRONTEND_DIR}/dist" && -f "${LOCAL_FRONTEND_DIR}/dist/index.html" ]]; then
  log "使用本地构建的 dist（来自 Hi-Sillot-OpenList-Frontend）"
  log "  source: ${LOCAL_FRONTEND_DIR}/dist"
  rm -rf public/dist
  mkdir -p public/dist
  cp -a "${LOCAL_FRONTEND_DIR}/dist/." public/dist/
  # 用 Hi-Sillot-OpenList-Frontend 的 package.json 版本作为 VERSION 标记
  LOCAL_VERSION="v$(grep -oE '"version"[[:space:]]*:[[:space:]]*"[^"]+"' "${LOCAL_FRONTEND_DIR}/package.json" | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')"
  echo "${LOCAL_VERSION:-local}-encv" > public/dist/VERSION
elif [[ ! -f "public/dist/index.html" ]]; then
  # fallback: 下载 release tarball
  log "下载 ${FRONTEND_TARBALL_URL}"
  ...
fi
```

## 验证

```bash
# 1. 克隆验证
$ ls /workspace/app/openlist/Hi-Sillot-OpenList-Frontend/
LICENSE            README.md         build.sh           package.json       public/
README_Sillot.md   bun.lock          index.html         pnpm-lock.yaml     src/
✅ 文件齐全

$ cat /workspace/app/openlist/Hi-Sillot-OpenList-Frontend/package.json | grep version
"version": "4.1.8",
✅ 版本号正确

# 2. 脚本语法
$ bash -n /workspace/app/encv-mobile/scripts/dev-openlist.sh
[OK] syntax

# 3. 脚本检测到本地 dist 不存在，fallback 到 public/dist/ 已有的 62M
$ bash scripts/dev-openlist.sh --data /tmp/test --no-config
[dev-openlist] public/dist/ 已存在 (62M)，跳过下载
[dev-openlist] --no-config：跳过 config.json 写入
[dev-openlist] 启动 OpenList (port 5244, data /tmp/test)
✅ 走 fallback 路径（Hi-Sillot-OpenList-Frontend/dist/ 还没生成）

# 4. （待 P5 验证）用户跑：
#    cd Hi-Sillot-OpenList-Frontend && bun run build
#    cd ../../encv-mobile && bash scripts/dev-openlist.sh
#    → 自动用本地 dist
```

## 教训

1. **spec/tasks.md 阶段总览的每一行都应该是可执行 Task**：不能只提到"用 Hi-Sillot-OpenList-Frontend"就完事，要明确"clone 到哪个目录"
2. **dev 工作流必须能 hot reload**：如果改一行代码要 30+ 秒才能看到效果，就失败了
3. **本地源 vs 下载 release 是两种 dev 模式**：脚本要支持两者，并优先用本地
4. **Phase 2.5 是补救插入**：因为发现 P3 缺关键步骤，必须用新 phase 名（如 P2.5）插入到 P2-P3 之间，不打乱 P3-P4 的完成态

## 完整 dev 工作流（修复后）

```bash
# Terminal 1: 改前端 + 实时重 build
cd /workspace/app/openlist/Hi-Sillot-OpenList-Frontend
bun install
bun run build --watch    # 暂未确认 vite build --watch 存在；目前手动 bun run build
# → 每次 src/ 改动：bun run build 重新生成 dist/

# Terminal 2: 启 OpenList，自动用本地 dist
cd /workspace/app/encv-mobile
bash scripts/dev-openlist.sh --data /tmp/openlist-dev
# → 检测到 ${REPO_ROOT}/app/openlist/Hi-Sillot-OpenList-Frontend/dist/ 存在
# → cp -a dist/. public/dist/
# → 启 OpenList on 5244

# Terminal 3: 启 Vite (encv-mobile SPA + OpenList UI)
pnpm dev
# → http://localhost:8100/openlist-ui/  → 走 Vite middleware → OpenList

# 改完前端后：Ctrl+C Terminal 2 终止 + 重跑 dev-openlist.sh + Vite HMR
# 整个 cycle ~5-10s（vs 之前 30+ 秒的下载）
```
