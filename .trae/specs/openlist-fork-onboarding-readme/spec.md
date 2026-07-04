# app/openlist 跨 Fork 工作流与新会话自助 README

## Why

`/workspace/app/openlist/` 是 encv-mobile 与 Hi-Sillot/OpenList fork 协作的总入口目录，但当前 `README.md` 只有 5 行 GitHub 链接（`https://github.com/Hi-Sillot/OpenList` 等），**新会话启动后完全无法自助**：必须先由用户口头介绍 fork 关系、gomobile AAR 架构、frontend pin 机制、沙箱 GITHUB_TOKEN 推送绕过方法，工作才能开始。

并行地，沙箱环境存在一个**反复踩坑的隐性缺陷**：`GITHUB_TOKEN` 已 export 在 shell env 中，但 `git push` 仍报 `fatal: could not read Username for 'https://github.com': terminal prompts disabled`——因为 `git` 不会自动把环境变量当凭证使用。沙箱既无 `~/.netrc` 也无 `~/.gitconfig`，必须显式 `git -c http.extraHeader="Authorization: Bearer ${GITHUB_TOKEN}" push ...` 才能推送。这条经验**必须写进 README**，否则每次新会话都浪费 15-30 分钟试错。

## What Changes

### 一、改写 `app/openlist/README.md` 为完整工作流入口

当前 5 行 → 改写为含 10 个章节、约 350-450 行的「新会话自助手册」：

| 章节 | 必含要点 |
|------|----------|
| 1. 文档目的与适用场景 | "读到本文件即了解 fork 协作全貌，无需再问背景" |
| 2. 项目背景 | ENCV 容器加密 + OpenList 文件管理 + encv-go 反代的端到端架构图 |
| 3. 五层 fork 关系图 | encv-mobile ←→ Hi-Sillot/OpenList ←→ OpenListTeam/OpenList + OpenList-Frontend + OpenList-Desktop + OpenList-Mobile 各起什么作用 |
| 4. Hi-Sillot fork 维护工作流 | `dev` 分支（默认）+ `encv-v*.*.*` tag 流程 + 推送绕过 + 推送命令模板 |
| 5. gomobile bind AAR 架构 | OpenList 在主进程内运行、共享 JVM、不走 sidecar、为什么要 in-process |
| 6. encv-go 集成点 | `internal/encv/init.go` + `server/handles/down_ext.go::handleEncvPreviewFromLink` + 三个 init 顺序 |
| 7. frontend-pinned.txt 同步机制 | 为什么不能用 `releases/latest`、4 级 pin 优先级、`public/dist/VERSION` 写入 |
| 8. i18n overlay 机制 | `public/dist/i18n-overlay/<lang>/translation.json` 用 jq 合并到 `assets/<lang>.json` |
| 9. 构建脚本索引 | 指向 `scripts/build-openlist-aar.sh` + `scripts/README.md` + `plugin-openlist/README.md` |
| 10. 沙箱 GITHUB_TOKEN 推送工作流 | 根因 + 4 种解决方案（URL 注入 / extraHeader / netrc / SSH）+ 命令模板 |
| 11. 故障排查 checklist | 端口冲突、gomobile init 失败、AAR 缺类、frontend tag 404 等 8+ 场景 |
| 12. 相关文档双向链接 | 现有 5 个 spec + 3 个 README + 1 个 build script 入口 |

### 二、在 `scripts/build-openlist-aar.sh` 注入 GITHUB_TOKEN 自动 fallback

在 `git clone` 阶段**自动**把 fork URL 改写为 `https://x-access-token:${GITHUB_TOKEN}@github.com/...` 形式（URL 注入走 HTTP Basic Auth，GitHub 接受 PAT 作为 password），确保 fork 克隆与后续 push 不因 401/403 失败；输出日志 `[INFO] using GITHUB_TOKEN for fork auth`（token 仅日志化首 4 字符 `ghp_****`）。当用户**未 export** GITHUB_TOKEN 时，fallback 到匿名 clone 并打 `[WARN]`，不 fail。**注**：`git -c http.extraHeader=Authorization: Bearer ...` 看似等价但 push 时 GitHub 会返回 `invalid credentials`，**不可靠**。

### 三、（可选）`scripts/openlist-fork-helper.sh` wrapper

提供 `fork-clone` / `fork-push` / `fork-status` 三个 subcommand 封装 GITHUB_TOKEN 注入逻辑，让用户**不需要记住** `git -c http.extraHeader=...` 的繁琐语法。脚本检测 `~/.netrc` / `~/.gitconfig` 优先级，决定用 env 注入还是 helper。

## Impact

### Affected docs
- `/workspace/app/openlist/README.md`（5 行 → 完整手册）
- `/workspace/scripts/README.md`（新增章节：「沙箱 GITHUB_TOKEN 推送」交叉引用）
- `/workspace/app/encv-mobile/plugin-openlist/README.md`（新增章节：「参见 app/openlist/README.md」反向链接）

### Affected scripts
- `/workspace/scripts/build-openlist-aar.sh`（git clone 加 GITHUB_TOKEN 注入）

### Affected specs
- `integrate-openlist-as-combolite-plugin`（README 是其交付物的一部分，**无需修改 spec**，但 checklist 需更新「文档」段勾选状态）

### 不影响
- encv-go Go 代码（`internal/openlist/`, `internal/server/openlist_handlers.go`）
- encv-mobile Capacitor / Vue 代码
- ComboLite 框架
- iOS 端（仍不在范围）

## ADDED Requirements

### Requirement: app/openlist/README.md 包含 10 个标准章节

`/workspace/app/openlist/README.md` SHALL 至少包含：① 文档目的 ② 项目背景 ③ 五层 fork 关系图 ④ Hi-Sillot fork 维护工作流 ⑤ gomobile bind AAR 架构 ⑥ encv-go 集成点 ⑦ frontend-pinned.txt 同步机制 ⑧ i18n overlay 机制 ⑨ 构建脚本索引 ⑩ 沙箱 GITHUB_TOKEN 推送工作流 ⑪ 故障排查 checklist ⑫ 双向链接。

#### Scenario: 新会话冷启动可直接工作
- **WHEN** 新 AI 会话把 `app/openlist/README.md` 纳入上下文
- **THEN** 不需要再向用户问任何 fork / 架构 / 推送 / frontend pin 背景问题，就能直接进入实施阶段

#### Scenario: README 自含 fork 关系图
- **WHEN** 阅读第 3 章
- **THEN** 至少有一张 ASCII / Mermaid 关系图清晰说明 encv-mobile、Hi-Sillot/OpenList、OpenListTeam/OpenList、OpenList-Frontend、OpenList-Desktop、OpenList-Mobile 各自角色

### Requirement: 沙箱 GITHUB_TOKEN 推送工作流文档化

README §10 SHALL 显式记录：
- 根因：`GITHUB_TOKEN` 在 env 中存在但 git 不会自动当凭证使用
- 现象：`fatal: could not read Username for 'https://github.com': terminal prompts disabled`
- 解决方案 1（**推荐**）：URL 注入 `git push https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git <branch>`（GitHub 接受 Basic Auth with PAT）
- 解决方案 2（**不可靠**）：`git -c http.extraHeader="Authorization: Bearer ${GITHUB_TOKEN}" ...` —— clone 可用但 push 报 `invalid credentials`
- 解决方案 3：写 `~/.netrc` 一劳永逸（沙箱仅本次会话有效）
- 解决方案 4：换 SSH 协议（沙箱无 ssh-agent，**不可行**）
- shell history 防护：`set +o history` 或 `GITHUB_TOKEN="$(cat ~/.encv-token)"` 内联

#### Scenario: 用户按 README 步骤推送成功
- **WHEN** 用户复制 README §10 提供的 URL 注入命令并替换分支名
- **THEN** push 成功，无 401/403/username 提示

#### Scenario: 用户 export 了 GITHUB_TOKEN 但用了 extraHeader 推
- **WHEN** 跑 `git -c http.extraHeader="Authorization: Bearer ${GITHUB_TOKEN}" push origin dev`
- **THEN** 失败信息与 README §10.3 「失败 2」一字不差：`remote: invalid credentials` + `fatal: Authentication failed for '...'`

### Requirement: build-openlist-aar.sh 在 git clone 阶段自动注入 GITHUB_TOKEN

`scripts/build-openlist-aar.sh` SHALL 在 `git clone` Hi-Sillot fork 之前检测 `GITHUB_TOKEN` 是否 export；若有，则把 fork URL `https://github.com/Hi-Sillot/OpenList.git` 改写为 `https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git`（URL 注入走 HTTP Basic Auth），并在日志输出 `[INFO] using GITHUB_TOKEN for fork auth (ghp_****)`（仅前 4 字符 + `****`）。

#### Scenario: GITHUB_TOKEN 已 export
- **WHEN** 用户跑 `scripts/build-openlist-aar.sh --output ...`
- **THEN** 日志第 N 行含 `[INFO] using GITHUB_TOKEN for fork auth (ghp_****)`；`git clone` 的 URL 含 `x-access-token:` 前缀

#### Scenario: GITHUB_TOKEN 未 export
- **WHEN** 用户未 export GITHUB_TOKEN
- **THEN** 日志第 N 行含 `[WARN] GITHUB_TOKEN not set, falling back to anonymous clone (will fail on private repos)`；fork 是 public 仓库时仍能 clone（dev 分支可访问）；fork 切到 private 后会 401

### Requirement: README 包含故障排查 checklist

README §11 SHALL 包含至少 8 个常见场景的「症状 → 根因 → 修复」表格：
1. 端口 5244 冲突
2. `gomobile init` 失败
3. AAR 缺 `openlistlib.Openlistlib` 类
4. OpenList-Frontend tag 404
5. `replace github.com/Soltus/encv-go => ../../../` 解析失败
6. `OpenListBridge.start()` 后 5s 内 5244 不响应
7. i18n overlay jq 合并失败
8. gomobile bind 后 AAR 体积 > 50MB

#### Scenario: 用户遇到其中任一症状
- **WHEN** 用户在 README §11 检索关键词（如「5244」「gomobile」「AAR 体积」）
- **THEN** 找到对应行，按「修复」列操作可恢复

### Requirement: 双向链接覆盖核心 spec / 文档

README §12 SHALL 至少包含以下交叉引用（确保 0 个死链）：
- [`.trae/specs/integrate-openlist-as-combolite-plugin/spec.md`](file:///workspace/.trae/specs/integrate-openlist-as-combolite-plugin/spec.md)（主 spec）
- [`scripts/README.md`](file:///workspace/scripts/README.md)（构建脚本说明）
- [`app/encv-mobile/plugin-openlist/README.md`](file:///workspace/app/encv-mobile/plugin-openlist/README.md)（插件模块说明）
- 5 个 GitHub 仓库 URL（Hi-Sillot/OpenList 等）
- K-Sillot/OpenList-Mobile 参考仓库

#### Scenario: 链接可达性
- **WHEN** 用户在 IDE 中点击 README 任意链接
- **THEN** 目标文件存在且相对路径正确

## MODIFIED Requirements

无（这是新增文档工作流 spec，不修改既有功能 spec）。

## REMOVED Requirements

无。

---

## 风险与决策点

| 编号 | 风险 | 决策依据 |
|------|------|---------|
| R1 | 改写后的 README 过长（> 500 行）让会话 context 膨胀 | 上限 450 行；超过则拆分为 `app/openlist/docs/{architecture,fork-workflow,push-workflow}.md` |
| R2 | GITHUB_TOKEN 日志化泄露完整 token | 严格只输出 `ghp_****` 前 4 字符；任何带 `${GITHUB_TOKEN}` 的完整字符串禁止进 log |
| R3 | build-openlist-aar.sh 注入 GITHUB_TOKEN 改变默认行为 | 仅在 env 存在时注入；不破坏未 export GITHUB_TOKEN 的用户路径 |
| R4 | wrapper 脚本（`openlist-fork-helper.sh`）可能重复造轮子 | **可选**，本期不强制；用户可选用 git alias 替代 |
| R5 | README 描述的「fork 关系图」与实际工作流漂移 | 引用 spec + script 作为单一事实源；图每季度审查一次 |

## 不在本次范围

- 修改 `integrate-openlist-as-combolite-plugin` spec 主体（仅 README 文档化工作）
- 修改 ComboLite 框架
- 重写 gomobile bind 流程（仅补 GITHUB_TOKEN 注入）
- iOS 端（用户已排除）
- SSH key 配置自动化
- 跨会话 context 持久化机制（依赖 README 即可，足够轻量）
