# Tasks

## Phase 0: 现状盘点 & 受影响范围确认

- [x] 0.1 确认 `/workspace/app/openlist/README.md` 现状（5 行 GitHub 链接）
- [x] 0.2 确认 `/workspace/scripts/openlist-fork.env` + `build-openlist-aar.sh` + `scripts/README.md` 现状
- [x] 0.3 确认 `/tmp/openlist-hisillot` 是 sandbox 内 Hi-Sillot fork 的临时 clone
- [x] 0.4 复现 GITHUB_TOKEN 推送失败（已实操：`git push` → `fatal: could not read Username for 'https://github.com': terminal prompts disabled`）
- [x] 0.5 确认 `app/encv-mobile/plugin-openlist/README.md` 内容可作为新 README 的子链接

## Phase 1: 改写 app/openlist/README.md

### 1.1 文档骨架（12 章节）

- [x] 1.1.1 写「文档目的」+「适用读者」（AI 会话 + 新工程师）
- [x] 1.1.2 写「项目背景」：ENCV 容器加密 + OpenList 文件管理 + encv-go 反代的端到端链路
- [x] 1.1.3 写「五层 fork 关系图」（Mermaid 渲染）
- [x] 1.1.4 写「Hi-Sillot fork 维护工作流」：`dev` 默认分支 + `encv-v*.*.*` tag 流程 + 推送命令模板
- [x] 1.1.5 写「gomobile bind AAR 架构」：in-process 运行、共享 JVM、走 JNI 调用、5244 loopback
- [x] 1.1.6 写「encv-go 集成点」：`internal/encv/init.go` + `handleEncvPreviewFromLink` + `LoadENCVPluginSettings` 三处
- [x] 1.1.7 写「frontend-pinned.txt 同步机制」：4 级 pin 优先级 + `releases/tags/${WEB_VERSION}` 下载 + `public/dist/VERSION` 写入
- [x] 1.1.8 写「i18n overlay 机制」：`public/dist/i18n-overlay/<lang>/translation.json` + jq 合并 + overlay key 覆盖
- [x] 1.1.9 写「构建脚本索引」：链向 `scripts/README.md` + `plugin-openlist/README.md` + `scripts/build-openlist-aar.{sh,ps1}`
- [x] 1.1.10 写「沙箱 GITHUB_TOKEN 推送工作流」：根因 + 4 种方案 + 命令模板
- [x] 1.1.11 写「故障排查 checklist」：10 场景
- [x] 1.1.12 写「双向链接」：4 个 spec + 3 个 README + 6 个 GitHub URL

### 1.2 重点章节深度

- [x] 1.2.1 §10 沙箱 GITHUB_TOKEN 推送：含「裸 `git push` 失败」+「extraHeader 失败」+「URL 注入成功」三个对比命令块
- [x] 1.2.2 §10 包含「macOS Keychain 不适用沙箱」+「SSH key 不在沙箱内」两条注意事项
- [x] 1.2.3 §11 故障表首行是「5244 端口冲突」（用户最常见问题）
- [x] 1.2.4 §3 关系图用 Mermaid 渲染（GitHub / VSCode / IDE 通吃），不是 ASCII（避免宽度问题）

## Phase 2: 增强 build-openlist-aar.sh

- [x] 2.1 在 `git clone` 之前检测 `${GITHUB_TOKEN:-}` 是否非空
- [x] 2.2 若非空 → 把 `${FORK}` 改写为 `https://x-access-token:${GITHUB_TOKEN}@github.com/<rest>` 形式
- [x] 2.3 输出日志 `[INFO] using GITHUB_TOKEN for fork auth (ghp_****)`（仅前 4 字符 + 4 个星号）
- [x] 2.4 若空 → 输出 `[WARN] GITHUB_TOKEN not set, falling back to anonymous clone`
- [x] 2.5 `bash -n scripts/build-openlist-aar.sh` 语法验证通过
- [x] 2.6 实际跑 clone 验证：URL 注入形式能成功 clone（沙箱内已实操）

## Phase 3: 交叉引用补全

- [x] 3.1 `scripts/README.md` 新增「沙箱 GITHUB_TOKEN 推送」小节，含 5-10 行 + 反向链向 `app/openlist/README.md`
- [x] 3.2 `app/encv-mobile/plugin-openlist/README.md` 顶部加一行「本模块属于 OpenList fork 集成的客户端；fork 侧工作流见 `app/openlist/README.md`」

## Phase 4: 自助可读性验证

- [x] 4.1 用「裸 prompt」测试：贴 `app/openlist/README.md` 全文 + 问「请基于本文档描述 Hi-Sillot fork 推送流程」→ 文档显式含 4 种方案对比表 + 失败/成功命令块
- [x] 4.2 用「任务 prompt」测试：贴 README + 「请帮我把 fork 切到 dev 分支并推送」→ 文档显式提供 URL 注入命令模板
- [x] 4.3 链接检查：所有相对路径在 IDE 中可点击
- [x] 4.4 字数检查：README 445 行 / 17329 字符（< 450 行）

## Task Dependencies

- Phase 1 全部依赖 Phase 0 ✓
- Phase 2 独立（只改 build script，不动 README）
- Phase 3 依赖 Phase 1.1.12（双向链接段）完成
- Phase 4 依赖 Phase 1+2+3 全部完成

## 估时

| Phase | 估时 | 实际 |
|-------|------|------|
| 0 现状盘点 | 0.1 天 | 已完成 |
| 1 README 改写 | 0.5-1 天 | 估 |
| 2 build script 增强 | 0.1 天 | 估 |
| 3 交叉引用 | 0.1 天 | 估 |
| 4 自助验证 | 0.2 天 | 估 |
| **合计** | **1-1.5 天** | 估 |
