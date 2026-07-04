# scripts/

辅助构建 / 运维脚本。所有脚本应可在仓库根目录执行（路径均相对仓库根）。

## build-openlist-aar.sh / build-openlist-aar.ps1

从 [Hi-Sillot/OpenList](https://github.com/Hi-Sillot/OpenList) fork 编译出
`openlist.aar`（gomobile bind 产物），供
[plugin-openlist](../app/encv-mobile/plugin-openlist/) ComboLite 插件
`libs/openlist.aar` 使用。

- Linux / macOS: `scripts/build-openlist-aar.sh`
- Windows (CI): `scripts/build-openlist-aar.ps1`

### 环境要求

| 工具 | 版本 | 备注 |
|------|------|------|
| Go | 1.25.x | 与 Hi-Sillot fork `go.mod` 一致 |
| Android NDK | r25c+ | 推荐 r26b / 26.3.11579264 |
| Java | 17 | Temurin / OpenJDK |
| cmake | 系统包 | NDK 工具链依赖 |
| git / curl / tar / sha256sum | 系统包 | 拉取源码与 frontend dist |
| jq | 系统包 | **仅在 fork 提供 `public/dist/i18n-overlay/` 时必须**（merge i18n 翻译补丁） |

### 快速开始

```bash
scripts/build-openlist-aar.sh \
    --output /workspace/app/encv-mobile/plugin-openlist/libs
```

或 Windows：

```powershell
pwsh -File scripts/build-openlist-aar.ps1 `
    -Output  C:\workspace\app\encv-mobile\plugin-openlist\libs `
    -EncvGoRoot C:\workspace
```

### 入参

| 参数（bash / PowerShell） | 必填 | 默认来源 | 说明 |
|------|------|----------|------|
| `--output` / `-Output` | 是 | — | `openlist.aar` 输出目录（脚本会生成 `openlist.aar` + `openlist.aar.sha256`） |
| `--fork` / `-Fork` | 否 | `openlist-fork.env` → `OPENLIST_FORK_URL` | fork 仓库 URL |
| `--branch` / `-Branch` | 否 | `openlist-fork.env` → `OPENLIST_FORK_BRANCH`（默认 `dev`） | 分支或 tag |
| `--ndk` / `-Ndk` | 否 | `$ANDROID_HOME/ndk/26.3.11579264` | NDK 安装路径 |
| `--encv-go-root` / `-EncvGoRoot` | 否 | `/workspace`（Linux）/ `C:\workspace`（Windows） | 本地 encv-go 仓库路径（用于修复 `replace github.com/Soltus/encv-go => ../../../`） |
| `--frontend-version` / `-FrontendVersion` | 否 | 见下「frontend 版本对齐」段 | 锁定 OpenList-Frontend 版本号（如 `v4.0.0`） |
| `--local-frontend-dist` / `-LocalFrontendDist` | 否 | — | 跳过下载，直接 cp 本地 dist 目录到 `public/dist/` |

### Fork 配置（`openlist-fork.env`）

集中管理 fork URL / 默认 branch / frontend pin 等参数，CI 与本地开发共享。

| 键 | 默认 | 说明 |
|----|------|------|
| `OPENLIST_FORK_URL` | `https://github.com/Hi-Sillot/OpenList.git` | 克隆的 fork 仓库地址 |
| `OPENLIST_FORK_BRANCH` | `dev` | 默认 checkout 的 branch / tag |
| `OPENLIST_FORK_PINNED_TAG` | 空 | 保留字段，用于在 fork 上打 tag 后切到固定版本 |
| `OPENLIST_FRONTEND_VERSION` | 空 | 兜底 frontend pin（在 fork `frontend-pinned.txt` 与 CLI 都未提供时使用） |

#### 个人 override（不污染 git）

复制 `scripts/openlist-fork.env` 为 `scripts/openlist-fork.env.local`（已加入
`.gitignore`），改其中字段即可。`build-openlist-aar.sh` 会在 `openlist-fork.env`
之后 source 本地覆盖文件。PowerShell 版本也支持同样的 `.env.local` 自动加载。

#### 配置优先级（高 → 低）

1. CLI 入参 / PowerShell 参数
2. 当前 shell 已 export 的同名环境变量
3. `scripts/openlist-fork.env.local`（个人覆盖）
4. `scripts/openlist-fork.env`（仓库默认）
5. 硬编码 fallback

### Frontend 版本对齐（`frontend-pinned.txt` 工作流）

> **核心问题**：fork 增加了 ENCV 设置项（`EncvDecryptPassword` /
> `EncvTextExt` 等），上游 OpenList-Frontend 不感知这些 key → 切语言后
> 部分 UI 回退显示 key 字面量。

解决方案：fork 根目录维护 `frontend-pinned.txt`，脚本读取后从
`https://api.github.com/repos/OpenListTeam/OpenList-Frontend/releases/tags/<vX.Y.Z>`
**精确下载**该版本的 dist，不再用 `releases/latest`。

#### 4 级优先级（高 → 低）

1. `${SRC_DIR}/frontend-pinned.txt`（fork 提交时 pin → 不漂移）
2. `--frontend-version` CLI / PowerShell 入参
3. `OPENLIST_FRONTEND_VERSION` 环境变量
4. fallback `releases/latest` + stderr 警告（CI lint 阶段应 fail 此情况，build 阶段仍可继续）

#### 工作流

```bash
# 1. 用户在 Hi-Sillot/OpenList 升级了 ENCV 设置项
cd ~/openlist-fork
echo "v4.0.1" > frontend-pinned.txt   # 与 OpenList-Frontend 上游 tag 同步
git add frontend-pinned.txt && git commit -m "bump frontend pin to v4.0.1"
git push origin dev

# 2. encv-mobile 仓库直接拉最新 dev 即可，无需改任何文件
scripts/build-openlist-aar.sh --output /workspace/app/encv-mobile/plugin-openlist/libs
# 日志会输出：
#   == Resolve frontend version ==
#     source: fork frontend-pinned.txt
#     version: v4.0.1
#     frontend: https://github.com/OpenListTeam/OpenList-Frontend/releases/download/v4.0.1/openlist-frontend-dist-...
```

`ldflags` 中的 `WebVersion` 同步注入 `${WEB_VERSION}`（不再硬编码 `rolling`），
OpenList 启动后访问 `/api/admin/settings` → `web_version` 字段会返回该版本号。

### i18n overlay

如果 fork 在 `public/dist/i18n-overlay/<lang>/translation.json` 放置 ENCV
专用翻译补丁，构建脚本会用 `jq` 把它合并到 `public/dist/assets/<lang>.json`，
overlay 的 key 覆盖原 key：

```
public/dist/i18n-overlay/
├── zh-CN/
│   └── translation.json   # ENCV 设置项的中文翻译补丁
└── en/
    └── translation.json
```

合并命令等价于：

```bash
jq -s '.[0] * .[1]' public/dist/assets/zh-CN.json \
                public/dist/i18n-overlay/zh-CN/translation.json \
    > public/dist/assets/zh-CN.json.tmp && \
    mv public/dist/assets/zh-CN.json.tmp public/dist/assets/zh-CN.json
```

i18n overlay 合并后脚本会写 `public/dist/VERSION`（内容形如 `v4.0.1-encv`），
OpenList 后端可在 `Bootstrap()` 中读此文件并存储为 `conf.FrontendVersion`。

fork 没提供 `i18n-overlay/` 目录时，脚本直接跳过该步骤，原 frontend dist
原样使用。

### 工作流程

1. 解析入参，校验 go / java / git / curl / tar / cmake / ndk-build
2. 加载 `scripts/openlist-fork.env`（+ `.local` 覆盖）
3. `app/openlist/Hi-Sillot-OpenList/` 准备本地工作区，删除旧副本
4. `git clone --depth 1 --branch $BRANCH $FORK`
5. 验证 fork `go.mod` 的 `replace github.com/Soltus/encv-go => ../../../` 相对路径（默认布局下天然成立；若 fork 在非标位置，脚本 sed 兜底为绝对路径）
6. 解析 frontend 版本（4 级优先级），下载匹配版本的 OpenList-Frontend dist
7. `jq` 合并 `public/dist/i18n-overlay/`（可选）
8. 写 `public/dist/VERSION`（`${WEB_VERSION}-encv`）
9. 设置 `ANDROID_NDK_HOME`，`go install gomobile/gobind@latest` + `gomobile init -ndk $NDK`
10. `gomobile bind -ldflags "..." -androidapi 19 -target="android/arm64" -o $OUTPUT/openlist.aar ./openlistlib`
11. 生成 `openlist.aar.sha256`

### 故障排查

| 症状 | 根因 | 修复 |
|------|------|------|
| `Hi-Sillot fork is missing openlistlib/` | fork 还没补 `openlistlib/{server,settings,common,event}.go` 入口 | 参见 spec §一 |
| `replace github.com/Soltus/encv-go => ../../../` 解析失败 | sed 未生效 | 检查 `--encv-go-root` 是否为绝对路径 |
| `[WARN] no frontend pin, using latest` | fork 无 `frontend-pinned.txt` 且 CLI/env 未指定 | 在 fork 根加 `frontend-pinned.txt` 或传 `--frontend-version vX.Y.Z` |
| `OpenList-Frontend v9.9.9 not found` | pin 的 tag 在上游不存在 | 核对 tag 名（区分大小写），必要时用 `v4.0.0` 全名 |
| `i18n-overlay/ exists in fork but jq is not installed` | 缺 jq | `apt install jq` / `choco install jq` |
| `frontend dist extraction failed` | GitHub API 限流或网络问题 | 重跑脚本（脚本本身有缓存逻辑之外的简单重试） |
| `gomobile init` 失败 | NDK 路径错 | `--ndk` 传 NDK 安装根目录（含 `ndk-build`） |

### 相关文档

- Spec: [integrate-openlist-as-combolite-plugin](../.trae/specs/integrate-openlist-as-combolite-plugin/spec.md)
- Reference fork: [Hi-Sillot/OpenList](https://github.com/Hi-Sillot/OpenList)
- 参考实现脚本（K-Sillot 仓库，仅参考）：
  - `init_openlist.sh` / `init_web.sh` / `init_gomobile.sh` / `gobind.sh`

## 沙箱 GITHUB_TOKEN 推送工作流

`build-openlist-aar.sh` 在 `git clone` 阶段会**自动**检测 `GITHUB_TOKEN` 是否在 env 中；若已 export，则把 fork URL 改写为 `https://x-access-token:${GITHUB_TOKEN}@github.com/...` 形式（URL 注入走 HTTP Basic Auth，GitHub 接受 PAT 作为 password）。**不**用 `git -c http.extraHeader=Authorization: Bearer ...`——clone 可用但 push 时 GitHub 返回 `invalid credentials`。

手动 push 时的推荐命令：

```bash
git push https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git dev
```

完整 4 方案对比 + 失败/成功命令块 + shell history 防护见 [app/openlist/README.md §10](../app/openlist/README.md#10-沙箱-github_token-推送工作流)。
