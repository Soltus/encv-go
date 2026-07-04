# 修复 sandbox preview 启动配置：ENCV_DEV_PREVIEW / ENCV_MOBILE 环境丢失

> 用户反馈：「/plan 配置还是有问题，没有自动补全环境就启动了导致挂载路径有误，为什么之前能生效的 mock 模式现在又没生效？」

## 1. 现象与根因

### 1.1 症状
- 浏览器通过 `http://localhost:16666/` 打开主 app
- 前端 `useConfig` / `useApiClient` 调用 `/api/service-guard`
- 返回 `envDevPreview: false` / `envMobile: false` / `servingDir: "/workspace"`（或 config.user.json 中 `server.dir` 原始值）
- mobile overlay 未触发 → 前端「Files」tab 看到的是 `/workspace` 根目录（`.md` / `.gitignore` 等），看不到 `01-plain-media/` mock 数据

### 1.2 根因链路
`.air.toml` → `.air-run.sh` → `./tmp/encv start` 启动链路上，**mobile overlay 触发条件是 env 变量 `ENCV_MOBILE=1` 或 `ENCV_DEV_PREVIEW=1`**（见 `internal/config/config.go:292-294`）。

`ecosystem.config.cjs` 的 `start-preview` app 配置：

```js
{
  name: 'start-preview',
  script: PREVIEW_SCRIPT,                // = /workspace/app/encv-mobile/scripts/start-preview.sh
  interpreter: 'bash',
  env: {
    PATH: process.env.PATH,
    // 让 mock 生成走沙箱 fallback（脚本默认 /storage/emulated/0，已存在）
    ENCV_MOCK_ROOT: '/storage/emulated/0',
  },
}
```

`ENCV_DEV_PREVIEW` 和 `ENCV_MOBILE` **都没有注入**。注释里说"start-preview.sh 内部已经设了"，但 `start-preview.sh:115` 用的是 bash inline env：

```bash
ENCV_DEV_PREVIEW=1 air &
```

这个 inline env 只对 `air` 那个进程有效。**问题**：
- pm2 启 start-preview.sh 时是 fork 出来的 bash，bash 启动 air（inline env 在 air 上）
- air 调 `go build -o ./tmp/encv ./cmd/encv`（build 步骤不需要 env）
- air 跑完 build 后调 `.air-run.sh` → `exec ./tmp/encv start "$@"`
- 实际 `./tmp/encv` 是 air 拉起的子进程，**会继承 air 的 env**（含 inline 的 `ENCV_DEV_PREVIEW=1`）
- 平时应该 work，**但是**：
  - air 0.x 某些版本在 rebuild（监听 .go 文件改动）时**不会**把 inline env 透传给新启动的 encv 二进制（这是 air 行为变化点）
  - pm2 重启 start-preview（`max_restarts: 5` 触发后）走的是 ecosystem 里的 env，inline env 失效
  - 用户手工 `pm2 restart start-preview` 也会让 inline env 失效

**结论**：inline env 是不稳定传递，pm2 必须直接注入。**需要 pm2 启动时强制设这两个 env**。

### 1.3 之前为何生效
- 之前用户都是直接 `bash scripts/start-preview.sh` 跑前台脚本（脚本内部 inline env 在 `air` 上，air fork `./tmp/encv` 时透传）
- 切换到 pm2 监管后，pm2 走 ecosystem 配置的 env，**覆盖**了 inline env 的作用（因为 `./tmp/encv` 进程被 air 启动时 air 进程的 env 来源 = pm2 给 bash 进程的 env，而 inline env 是 bash 自己的，不传给 `./tmp/encv` 实际不一定）

### 1.4 路径相关副作用
- `config.user.json` 的 `server.dir` 字段不存在（看到 admin / default_container_version / log / mobile / output_path / password / plugin_settings，**没有** server 段）
- 没有 mobile overlay 时，`config.DefaultConfig().Server.Dir = "./"`（`config.go:90`），`finalize()` 把它替换为 `os.Getwd()` = `/workspace`
- 这就是为什么前端 Files tab 看到 `/workspace` 的根目录

## 2. Proposed Changes

### 2.1 修复 1（核心）：`ecosystem.config.cjs` 的 `start-preview` 块注入 env

**文件**：`/workspace/ecosystem.config.cjs:111-136`

**改动**：
```js
{
  name: 'start-preview',
  script: PREVIEW_SCRIPT,
  interpreter: 'bash',
  cwd: REPO_ROOT,
  env: {
    PATH: process.env.PATH,
    // ✅ 必须由 pm2 注入！之前依赖 start-preview.sh 第 115 行的 inline env
    //   `ENCV_DEV_PREVIEW=1 air &` 在 air 0.x 行为下不稳定（rebuild 时可能丢 env），
    //   pm2 直接注入到 bash 进程环境，确保 air fork ./tmp/encv 时 100% 透传。
    //   见 internal/config/config.go:292-294 ApplyMobileOverlay 触发条件。
    ENCV_DEV_PREVIEW: '1',
    ENCV_MOBILE: '1',
    ENCV_MOCK_ROOT: '/storage/emulated/0',
  },
  // ... 其余不变
}
```

**为什么三件套都要设**：
- `ENCV_DEV_PREVIEW=1` —— desktop 端移动预览主触发器（`Makefile dev-mobile` 注释里说明）
- `ENCV_MOBILE=1` —— 兜底触发器（Android 真机语义；desktop 端有它也行）
- `ENCV_MOCK_ROOT=/storage/emulated/0` —— mock 生成目标路径（`generate-mock-files.ts:8` 读这个 env）

### 2.2 修复 2（兜底）：`.air-run.sh` 显式 exec 前 export

**文件**：`/workspace/.air-run.sh`

**改动**：
```sh
#!/bin/sh
# 兜底：即使 pm2 / air 中间层丢 env，这里强制 export
# ApplyMobileOverlay 的触发条件 (internal/config/config.go:292-294)
export ENCV_DEV_PREVIEW="${ENCV_DEV_PREVIEW:-1}"
export ENCV_MOBILE="${ENCV_MOBILE:-1}"
exec ./tmp/encv start "$@"
```

**为什么**：air 在 rebuild（监听 .go 改动）时**重新拉起** `./tmp/encv`，新进程的 env = air 当时的 env。如果 air fork 时 inline env 已丢（`air start` 走的是 cgroup 隔离模式或类似），这里的 `:-1` 兜底确保 encv 一定看到这两个 env。

### 2.3 修复 3（start-preview.sh 自身）：删 inline env，依赖 pm2 注入 + .air-run.sh 兜底

**文件**：`/workspace/app/encv-mobile/scripts/start-preview.sh:115`

**改动**：
```bash
# 改前：
ENCV_DEV_PREVIEW=1 air &

# 改后（pm2 监管下不依赖 inline，裸脚本用户也走 export 在 .air-run.sh 兜底）：
air &
```

如果用户直接 `bash start-preview.sh`（不走 pm2），`.air-run.sh` 的 export 兜底会自动填上 env。

### 2.4 修复 4（自检）：service-guard 失败时 pm2 强制 restart

**文件**：`/workspace/ecosystem.config.cjs:111-136` 在 `start-preview` 块加 `pm2-runtime` 风格的 `wait_ready` 或在 `kill_timeout` 之后做 health check。

**新做法**（更简单）：在 ecosystem 块加 `pm2-health-check` 字段？pm2 不支持 health check 原生机制。改用：

**做法 A**（推荐）：在 `start-preview` 块加 `post_update` / `autorestart` 已经够用，依赖 **start-preview.sh 内部已有的 10s 自检循环**（`scripts/start-preview.sh:133-154`）。如果自检失败，脚本 exit 1 → pm2 看到非零退出码 → 触发 autorestart。

**做法 B**（加固）：写一个独立 health checker `pm2` app，每 30s 探测 `/api/service-guard`，若 `envDevPreview: false` 则 `pm2 restart start-preview` 并告警。

**先选 A**，B 作为后续 v2 增强（写到 V2-Backlog）。

### 2.5 修复 5（依赖完整性）：mock 数据缺失时自动生成

**文件**：`/workspace/app/encv-mobile/scripts/start-preview.sh:99-110`

`start-preview.sh` Step 2 已经在生成 mock，但**只在 `/storage/emulated/0` 存在的情况下**才验证。沙箱首次启动时该路径不存在（用户本次痛点就是这个）。

**改动**：
```bash
# 改前：
step "2/6 生成 mock 数据到 ${MOCK_DIR}"
cd "${MOBILE_DIR}"
npx tsx scripts/generate-mock-files.ts
cd "${REPO_ROOT}"

if [[ ! -d "${MOCK_DIR}/01-plain-media" ]]; then
  echo "❌ 错误：mock 生成后仍缺少 ${MOCK_DIR}/01-plain-media 标记目录" >&2
  exit 1
fi

# 改后：自动 mkdir（脚本有权在沙箱 root 下建任何路径）
if [[ ! -d "${MOCK_DIR}" ]]; then
  step "2a/6 创建 mock 根目录 ${MOCK_DIR}（沙箱首次启动）"
  mkdir -p "${MOCK_DIR}" || { echo "❌ 无法创建 ${MOCK_DIR}" >&2; exit 1; }
fi
step "2b/6 生成 mock 数据到 ${MOCK_DIR}"
cd "${MOBILE_DIR}"
npx tsx scripts/generate-mock-files.ts
cd "${REPO_ROOT}"
```

`generate-mock-files.ts:16-18` 的 `ensureDir` 用 `mkdirSync({recursive: true})` 也会自动建，但 `Step 2` 的失败检测是同步的——在脚本早期显式 mkdir 让失败点更明确。

## 3. 文件改动清单

| 文件 | 改动 | 行号 |
|------|------|------|
| `/workspace/ecosystem.config.cjs` | `start-preview` 块 env 加 `ENCV_DEV_PREVIEW: '1'`、`ENCV_MOBILE: '1'`，修正注释 | 111-136 |
| `/workspace/.air-run.sh` | exec 前 export 兜底 env | 全文 |
| `/workspace/app/encv-mobile/scripts/start-preview.sh` | Step 3 第 115 行删 inline env；Step 2 加 mkdir 兜底 | 99-110, 115 |
| `/workspace/.trae/rules/preview-management.md` | §二 拓扑表 `start-preview` 标记为"⚠️ 启动时必须设 ENCV_DEV_PREVIEW=1"；§五 自检清单加「curl :2025/api/service-guard 应返 envDevPreview:true」 | §二、§五 |

## 4. 验证步骤（按顺序）

1. **环境归零**：
   ```bash
   pm2 delete all
   pkill -f 'tmp/encv' 2>/dev/null
   pkill -f air 2>/dev/null
   sleep 1
   ```

2. **应用改动**（人工编辑 4 个文件）

3. **启动主 app 4 件套 + start-preview**：
   ```bash
   pm2 start /workspace/ecosystem.config.cjs
   sleep 6
   pm2 list
   ```
   期望：start-preview online，memory 涨到 ~100MB（air + go build + 监听）

4. **校验 mobile overlay 生效**：
   ```bash
   curl -s http://localhost:2025/api/service-guard | jq .
   ```
   期望返回：
   ```json
   {
     "ready": true,
     "context": {
       "envDevPreview": true,    // ← 关键
       "envMobile": true,        // ← 关键
       "servingDir": "/storage/emulated/0",  // ← 关键
       "servingDirExists": true
     }
   }
   ```
   - 若 `envDevPreview: false` → env 没传到 encv，重做修复 1 + 修复 2
   - 若 `servingDir: "/workspace"` → mobile overlay 没触发，重做修复 1
   - 若 `servingDirExists: false` → mock 没生成，重做修复 5

5. **校验 mock 数据落地**：
   ```bash
   ls /storage/emulated/0/01-plain-media/ | head -5
   ```
   期望：列出 `01.mp4` `02.mp3` `03.png` `04.pdf` `05.txt` 等

6. **校验前端**：
   - 浏览器 `http://localhost:16666/`
   - Files tab → 看到 `01-plain-media/` `02-encrypted-media/` 等子目录（不是 `/workspace` 的 `.md`）
   - 切到 Tasks tab → 不报 "service-guard BLOCKED"

7. **校验 OpenPreview 外网入口**：点 OpenPreview 任务链点 → 打开外网 URL → 看到同样 Files 列表

8. **校验 air rebuild 时 env 不丢**：
   ```bash
   # 故意改一个 .go 文件触发 air rebuild
   echo '// test' >> /workspace/internal/server/server.go
   sleep 5
   curl -s http://localhost:2025/api/service-guard | jq '.context.envDevPreview'
   ```
   期望：`true`（air rebuild 后 env 仍保留，验证 `.air-run.sh` 兜底生效）

9. **持久化**：
   ```bash
   pm2 save
   ```
   期望：写入 `/root/.pm2/dump.pm2`

## 5. Assumptions & Decisions

| 假设 | 决策 | 备选 |
|------|------|------|
| `.air.toml` 用 `./.air-run.sh` 作为 entrypoint | **保留**（不改成直接 `./tmp/encv`）—— 留出 env 注入点 | 直接 exec encv |
| `start-preview.sh` 仍然由 pm2 监管 | **保留**（pm2 一键启停 + 日志 + autorestart） | 改用 `pm2 start air` 直接管 air |
| `start-preview.sh` 里的 `ENCV_MOCK_ROOT` 兜底 | **保留** | 完全靠 pm2 注入 |
| 修复 4 选 A（依赖 start-preview.sh 内部 10s 自检）而非 B（独立 health checker） | **简化 + 复用现有逻辑** | v2 再上 health checker |
| mock 数据路径固定 `/storage/emulated/0` | **保留**（spec 定义 + 服务端 ReadFile 假设） | 用户可设 `ENCV_MOCK_ROOT` 覆盖 |

## 6. 风险与回退

| 风险 | 缓解 |
|------|------|
| `.air-run.sh` export 兜底会让**桌面端正常模式**意外触发 mobile overlay | `export ... :-1` 模式：只有当外部没设时才填 1；用户想走桌面端时显式 `unset ENCV_DEV_PREVIEW && air` 即可 |
| pm2 inject `ENCV_MOBILE=1` 可能影响 Android 真机构建（虽然 build 时不读） | Android 端只走 `ENCV_MOBILE=1`，不影响；Capacitor 自己注入 |
| 修改 `.air-run.sh` 破坏 desktop 端 `make dev-backend` | `make dev-backend` 是直接 `go run` 不经 air 启动路径，零影响 |
| mock `mkdir -p /storage/emulated/0` 失败（沙箱权限） | 已有 `exit 1` 兜底 + 日志明确 |

## 7. 不在本次范围

- 改造为 `pm2-runtime` Docker 化（v2 议题）
- Redis 替代内存 SessionCache（v2）
- mock 数据生成器性能优化（v2）
- preview-gateway 加 `/agent-api/*` → :5245 路由（另一个 spec）

## 8. 预计改动量

- 4 个文件，最小化改动
- 预计 30-60 分钟：10 分钟改 + 5 分钟跑通 + 10 分钟回归 + 5 分钟自检 + 10 分钟文档同步
