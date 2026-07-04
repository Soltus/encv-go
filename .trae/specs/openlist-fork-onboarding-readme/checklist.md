# Checklist

## Phase 0: 现状盘点

- [x] `/workspace/app/openlist/README.md` 旧版是 5 行 GitHub 链接（已改写为 12 章节手册）
- [x] `/workspace/scripts/openlist-fork.env` 现状：4 个 key（URL/BRANCH/PINNED_TAG/FRONTEND_VERSION）
- [x] `/workspace/scripts/build-openlist-aar.sh` 已增强：GITHUB_TOKEN 自动 URL 注入
- [x] `/workspace/scripts/README.md` 现状：含 fork 配置 / frontend 版本对齐 / i18n overlay 三段
- [x] `/tmp/openlist-hisillot` 存在，origin 是 `https://github.com/Hi-Sillot/OpenList.git`
- [x] 沙箱 GITHUB_TOKEN 推送 root cause 已诊断：`git` 不读 env 当凭证；`extraHeader` 在 push 时被 GitHub 拒（`invalid credentials`）；**正确解是 URL 注入**

## Phase 1: app/openlist/README.md 改写

### 12 个章节齐全

- [x] §1 文档目的 + 适用读者
- [x] §2 项目背景（端到端架构 + 关键路径）
- [x] §3 五层 fork 关系图（Mermaid 渲染）
- [x] §4 Hi-Sillot fork 维护工作流（dev / tag / push）
- [x] §5 gomobile bind AAR 架构（in-process）
- [x] §6 encv-go 集成点（3 个 init 函数）
- [x] §7 frontend-pinned.txt 同步机制（4 级 pin）
- [x] §8 i18n overlay 机制（jq 合并）
- [x] §9 构建脚本索引
- [x] §10 沙箱 GITHUB_TOKEN 推送工作流（**4 方案** + 命令模板）
- [x] §11 故障排查 checklist（**10 场景**）
- [x] §12 双向链接（4 spec + 3 README + 6 GitHub URL）

### 重点章节深度

- [x] §10 包含「裸 `git push` 失败」+「extraHeader 失败」+「URL 注入成功」三个对比命令块
- [x] §10 包含「macOS Keychain 不适用沙箱」说明
- [x] §10 包含「SSH key 不在沙箱内」说明
- [x] §10 包含「shell history 防护」提示
- [x] §11 故障表首行是「5244 端口冲突」
- [x] §3 关系图用 Mermaid 渲染
- [x] §6 引用的 `internal/encv/init.go` 路径与实际位置一致
- [x] §7 引用的 `frontend-pinned.txt` 路径与 fork 实际结构一致
- [x] §8 引用的 `public/dist/i18n-overlay/` 路径与 fork 实际结构一致

### 字数 / 行数

- [x] 总行数 = 445 行（≤ 450 阈值）
- [x] 总字符数 = 17,329（≤ 30,000 阈值）

## Phase 2: build-openlist-aar.sh GITHUB_TOKEN 注入

- [x] `git clone` 前检测 `${GITHUB_TOKEN:-}` 是否非空
- [x] 非空时 fork URL 改写为 `https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git`
- [x] 日志输出 `[INFO] using GITHUB_TOKEN for fork auth (ghp_****)`（仅前 4 字符 + 4 星号）
- [x] 空时输出 `[WARN] GITHUB_TOKEN not set, falling back to anonymous clone (will fail on private repos)`
- [x] 日志**不**含完整 token 字符串（`bash -x` 调试时也不会泄露）
- [x] `bash -n scripts/build-openlist-aar.sh` exit 0
- [x] 沙箱内实际跑：`GITHUB_TOKEN=xxx` 时 clone 成功（URL 注入形式已实操验证）
- [x] 沙箱内实际跑：无 GITHUB_TOKEN 时脚本不 crash，只打 `[WARN]`

## Phase 3: 交叉引用

- [x] `scripts/README.md` 新增「沙箱 GITHUB_TOKEN 推送」小节，5-10 行 + 反向链向 `app/openlist/README.md`
- [x] `app/encv-mobile/plugin-openlist/README.md` 顶部新增一行反向链接：「fork 侧工作流见 `app/openlist/README.md`」
- [x] `integrate-openlist-as-combolite-plugin/checklist.md` 文档段勾选「`app/openlist/README.md` 改写为 12 章节新会话自助手册」

## Phase 4: 自助可读性验证

### 文档自含（无需会话外部上下文）

- [x] §10 提供完整命令模板（clone / commit / push 三步）
- [x] §10 显式对比 4 种方案的优缺点
- [x] §11 故障表覆盖 10 个场景
- [x] §12 双向链接覆盖 4 个 spec + 3 个 README + 6 个 GitHub URL

### 链接可达性

- [x] `file:///workspace/.trae/specs/integrate-openlist-as-combolite-plugin/spec.md` 存在
- [x] `file:///workspace/scripts/README.md` 存在
- [x] `file:///workspace/app/encv-mobile/plugin-openlist/README.md` 存在
- [x] 6 个 GitHub URL 拼写正确（手工验证）
- [x] 相对路径在 IDE / GitHub 中可点击

## 验证最终检查

- [x] 12 个 H2 章节标题层级清晰（H1 一个「app/openlist — Hi-Sillot OpenList Fork 协作入口」）
- [x] Mermaid 图块语法正确（graph TB / subgraph / 节点标签）
- [x] URL 注入命令沙箱内实操成功（`git clone https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git` 命中）

## 兼容性

- [x] 不修改 `integrate-openlist-as-combolite-plugin` spec 主体
- [x] 不修改 encv-go Go 代码
- [x] 不修改 encv-mobile Capacitor / Vue 代码
- [x] 不修改 ComboLite 框架
- [x] 不影响 iOS 端（用户已排除）

## 不在本次范围

- 自动写 `~/.netrc`（仅当次会话有效，写了也无用）
- SSH key 自动生成（沙箱无 ssh-agent）
- 修改 `integrate-openlist-as-combolite-plugin` spec 主体
- 跨会话 context 持久化（依赖 README 即可）
