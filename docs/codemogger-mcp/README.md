# codemogger MCP（环境镜像版）参考手册

> 面向**提供环境镜像的项目**（codemogger MCP 由镜像部署在 `/scripts/codemogger-mcp/`，
> 由 MCP 客户端以 stdio 子进程托管），不是给某个业务仓库的安装说明。
> 所有结论均在 2026-09-18 于工作区 `/workspace` 实测复核（见 `05-验证与回归.md`）。

## 一句话结论

codemogger MCP = **发现（semantic 检索）+ 理解（引用/上下文/波及面/关联图）** 的只读检索面；
设计为「镜像自包含 + 全局 CLI 打补丁 + shim 包裹 + 每工具一个文件的 MCP server」。

## 文档导航

| 文档 | 适用读者 / 场景 |
|---|---|
| [01-架构与部署.md](./01-架构与部署.md) | 构建镜像、排部署故障、改 server 本身的人 |
| [02-工具参考.md](./02-工具参考.md) | 日常使用者：10 个工具的参数、返回、示例 |
| [03-能力边界与已知坑.md](./03-能力边界与已知坑.md) | **上台 Entity/写代码前必读**：单 root vs 多 root、索引新鲜度、keyword 模式限制 |
| [04-补丁与 CLI 增强.md](./04-补丁与 CLI 增强.md) | 维护 `patches/`、升级 codemogger 版本、向上游贡献 |
| [05-验证与回归.md](./05-验证与回归.md) | 上线/改动后的验证矩阵与实测数据（含「先红后绿」对照） |
| [06-索引成本与调优.md](./06-索引成本与调优.md) | 索引耗时/体积实测、忽略机制、裁剪方案与镜像侧改进建议 |

## 最短路径（新会话上手）

```text
1) codemogger_list                      → 若返回 "No files indexed"，先 index（见 §索引）
2) codemogger_index                     → 多 root 全量索引（慢，几分钟级）
3) codemogger_search "<自然语言/标识符>"  → 跨仓库发现
4) codemogger_references <sym>          → 影响面（谁 import 了它）
5) codemogger_context <sym|file>        → 展开整文件大纲再动手改
```

## 三条最容易踩、且代价最高的规则

1. **单 root 工具 vs 多 root 配置是两套逻辑**
   `references` / `context` / `impact` / `leaks` / `list` / `related` 读的是
   **`CODEMOGGER_ROOT/.codemogger/index.db`**（单一库）；`search` / `grep` / `index`
   走的是工作区 `.codemogger.json` 的**多 root**。因此 `CODEMOGGER_ROOT` 必须指向
   **一个已经被索引过的 root**，否则前六个工具全部静默返回空（见 03 §1）。
2. **shim 的 `index` 在有 `.codemogger.json` 时永远走多 root 循环**，显式传 `<dir>/--db`
   会被忽略 → `CODEMOGGER_ROOT` 指向的那个库无法通过 MCP 工具刷新（03 §1.3）。
3. **索引是会漂移的快照**，shim 默认**不**自动重索引；代码变更后必须显式
   `codemogger_index`（03 §2）。

## 与「工作区实验版」的关系（历史）

仓库内曾经有一套同源自建的实验版 `app/codemogger-patch/`（cli.mjs 补丁 + shim +
单体 `mcp-server.mjs`），2026-09-18 已退役删除。镜像版是它的**超集**：

| 维度 | 仓库实验版（已删） | 镜像版（现行） |
|---|---|---|
| 三个 cli.mjs 补丁 | 有 | **同一份**（字节一致） |
| 工具数 | 9 | **10**（多 `codemogger_related` 多跳关联图） |
| server 结构 | 单体 `mcp-server.mjs`（412 行） | `index.mjs`（222 行）+ `lib/` + `tools/` 每工具一文件 |
| 参数健壮性 | 无 | `lib/args.mjs`：别名归一 + 未知参数拒绝（带 did-you-mean） |
| 进程治理 | 无锁 | 接管式单例锁 `/tmp/codemogger-mcp.lock` + stdin 关闭自然退出 |
| 注册方式 | `cp` 整文件覆盖 `/root/.codebuddy/mcp.json` | `register.mjs` 合并写入（不破坏其它 server 注册） |
| arm64 构建 | 会挂（node-gyp） | `force-wasm-grammars.mjs` 改为 WASM-only |
| 读前重索引 | 每次 read 前重建索引（慢 + SQLite 多进程 WAL 竞态） | 仅在库文件不存在时建一次 |

迁移记录见 [05-验证与回归.md §4](./05-验证与回归.md)。
