# 长期记忆（跨会话稳定事实）

## 架构权威文档（encv-mobile / shared-components 重构）
- **权威文档：`docs/migration-task-system.md`**（状态：执行中；2026-07-12 从 `app/docs/` 迁到仓库根 `/docs/` 统一文档位置，`app/docs` 目录已删）。核心设计 = **拆分抽象与实现**：
  - `@encv/shared-components` = 纯抽象层/库，只依赖 vue/pinia/ionic/通用第三方，**不依赖** `@/config` `@/constants` `@/router` 等应用上下文。
  - `encv-mobile` = 应用层，提供共享抽象所需的**注入上下文**（base URL/认证/容器版本等，经 `registerSharedTaskServices` 注入），组合 shared 抽象 + 应用专属业务。
  - 关键原则（§0）：抽象进 shared = **重写逻辑 + 依赖注入解耦**，不是搬运文件。
- **`docs/shared-components-boundary-spec.md` 是过时稿，与权威设计直接相反**（它主张"encv 业务全搬回 encv-mobile"，会摧毁已落地的 DI 共享抽象层）。**不要执行它的 Phase 1/2/3（搬 stores/api 出 shared）**。读它前先确认是否该被废弃/改写指向 migration-task-system.md。
- 已落地事实（2026-07-10~11）：Phase 1 ✅（shared/api/core 基座 + types/task.ts 领域类型）；Phase 2 store ✅（taskStore/runTasksStore 提升进 shared/stores，DI 解耦，encv-mobile 留 re-export 垫片）；非任务 api（shared/api/encv_*.ts 非任务域）仍为"暂存残留"待 Phase 6/A-ext 清理回退；`useEventBus` shared 孤儿副本待删（保留 encv-mobile canonical）。
- Module G（通用 composables 去重）：encv-mobile 原有多份与 shared 重复的 composables。**2026-07-11 已完成 G-1/G-2**：encv-mobile/src/composables 下 useToast/useClipboard/useDateFormat/relativeTime/activeStatus（re-export 垫片）+ useSearchInput（与 shared 逐字节相同）已删除，全靠 `@/*` 别名二级回退到 shared（tsconfig `"@/*":["./src/*","../packages/shared-components/src/*"]`）。剩余 G-3 VirtualLogList.vue、G-4 useEventBus。**⚠️ useEventBus 结论反转**：encv-mobile 的 useEventBus.ts 已不存在，11 处 `@/composables/useEventBus` 引用回退到 shared 副本=事实 canonical，故**保留 shared 的 useEventBus.ts，禁止删除**（原文档"删 shared 孤儿"指令有害）。**2026-07-12 已修正迁移文档**：原文档 §5/Phase6/目录树/IN-SCOPE 表/Module G 目标等 7 处仍写"删 shared 孤儿副本、保留 encv-mobile canonical / 待 Phase 6 删除 / 应清理"，与 §8.1.1 ② 的已验证反转直接矛盾（即实验报告 gap F 文档↔代码漂移）；已统一改为"shared 副本为事实 canonical、保留、禁止删除"，文档现已自洽。
- 删除前务必事实核查：① 确认 tsconfig `@/*` 有 shared 回退；② grep 确认无相对路径引用（`from "./..."`/`"../..."`）；③ 逐文件比对与 shared 等价。本会话已验证这套方法安全。
- **⚠️ 关键坑（2026-07-11 实测）：tsconfig 的 `@/*` 二级回退 ≠ Vite/vitest 运行时解析**。Vite/vitest 默认不读 tsconfig paths。`vite.config.ts` 有 `encv-alias-fallback` 插件（本地优先+shared 次之）故 dev/build 正常；但 `vitest.config.ts` 的 `resolve.alias['@']` 仅指向本地 src、无 shared 回退，且原 `plugins:[vue()]` 无该插件 → 测试环境 `@/composables/useX` 解析不到 shared。**修复**：提取共享插件 `encv-mobile/vite-plugins/encv-alias-fallback.ts`（`enforce:'pre'`）加入 vitest.config.ts 的 `plugins`。删除本地 composables 副本后，测试文件若用相对路径导入被删模块（如 `activeStatus.test.ts` 的 `./activeStatus`）必须改 `@/` 别名。
- **⚠️ vitest `@/` 解析失败的完整根因链与正确修复（2026-07-11，四次迭代才定位）**：
  - 现象：`pnpm check:all` 的 encv-mobile 单测多 suite FAIL，`Failed to resolve import "@/composables/useX"`（本地 composables 副本已按 Module G 删除、应回退 shared；后期连本地真实存在的文件如 `renderTurnItems`/`state-machine` 也解析不了）。
  - **决定性根因（第四次才定位）**：测试跑在 `'fast'` **project** 下，而 `plugins` 只配在**根配置**。`vitest`/`vite 8` 的 **projects 模式【不继承】根配置的 `plugins`**——每个 project 是独立 Vite 配置，必须各自带 `plugins`。于是 `'fast'` project 里没有任何插件，`@/...` 无人解析 → 全部失败（与"没加插件"完全一样；连本地文件都解析不了是此根因的判定特征）。
  - 前三次是干扰项（已排除）：① 最初以为"缺 `@/` fallback 插件"→ 加了插件但没用；② 误判"vitest 打包使插件 `__dirname` 指向 config 目录"→ 改显式传 `roots`（健壮性改进，非决定性）；③ 误判"test.alias/resolve.alias 的 `'@': SRC_DIR` 抢先解析"→ 移除 `'@'` 别名、去 `enforce`（与 vite.config 对齐，仍非决定性，因为根插件压根没被 project 调用）。
  - **对照 `vite.config.ts`**：它是**单配置、无 projects**，根 `plugins` 直接生效，所以 dev/build 一直正常。vitest 用 projects 才暴露差异。
  - **正确修复**：抽 `BASE_PLUGINS = [vue(), encvAliasFallback({ roots: [SRC_DIR, SHARED_SRC] })]`，**根配置 + 每个 project（fast/isolated）都引用同一份**。`encvAliasFallback` 仍保留"调用方显式传 `roots`"（不用插件自身 `__dirname`）的健壮性；并在 `resolveId` 解析失败时 `console.error('[encv-alias-fallback] NOT FOUND', source, dirs)` 便于诊断。
  - **通用教训**：① vitest 用 projects 时，**插件/alias 必须写到每个 project 配置里**，不能只写根配置；② 给 vitest 加 `@/` 多路径 fallback 时**不要同时在 alias 设 `'@'`**；③ 插件内**不要用自身 `__dirname`** 定位项目根（vitest 打包后指向 config 目录），由调用方显式传 roots。
- 删除 Module G 本地副本后的标准复验：`cd /workspace/app && pnpm check:all`（用户在自己终端跑；encv-mobile typecheck + 单测应全绿）。

## 工具/环境注意
- `execute_command` 在工具后端偶发 10s 启动超时（报 "failed to start within 10 seconds"，连 `mv`/`echo` 都失败），与 shell 配置无关，**且是瞬时的**：单条命令超时后**直接重试原命令**即可恢复（重试 2–3 次仍失败再判定环境失联）。**严禁用 `echo`/空命令做"探测"**——探测本身也会超时、且对恢复毫无意义，纯属浪费轮次。文件编辑/读取/搜索类工具完全不受影响，可并行使用。
- **门禁命令（含 `pnpm check:all`、构建、`make-shim`）由 agent 自己跑**，不要甩给用户。长耗时命令加超时参数（如 `timeout 600 ...`），超时即重试或报告失败，不要无脑阻塞。
- **⚠️ `read_lints` 绝不是门禁（2026-07-14 用户纠正 + 13:19 实测）**：`read_lints` 只承载 **TypeScript 语言服务 (tsserver) 诊断（Source: ts）**——能抓语法/类型红线（实测：用户在 `useTasksView.ts` L23 故意写非法字符，`read_lints` 报 `[ERROR] Line 23: Invalid character. (Source: ts, Code: 1127)`）。但 **`read_lints` 不接 Biome 诊断源——全程无任何 `Source: biome`**；Biome 的 lint 规则（noAssignInExpressions/useConst/noUselessElse 等）**不会**出现在 read_lints 里（故此前那些只有 Biome warning 的文件 read_lints 恒为 0，与文件是否打开无关）。**查 Biome 只能靠 `biome ci <path>` CLI（终端可用时）或用户 VS Code 自身红线**。`read_lints` 也不跑完整 `tsc --noEmit` 工程构建 / 单测，故仍**不能当「绿」的证据**。真实门禁是 **`node scripts/check-all.mjs`**（在 `app/` 根目录跑），报告写 `app/check-report.md`，逐套件完整日志在 `app/check-logs/<slug>.log`（如 `encv-mobile-unit-tests.log` 含每个测试文件 `✓/❯` 通过行，是确认某测试「是否被收集+通过」的权威来源）。**所有重构批次一律以 check-all.mjs 真实 PASS/FAIL 为准，不得用 read_lints 冒充门禁。**
- **Biome 配置环境事实（2026-07-14）**：biome 配置已迁到仓库根 `/workspace/biome.jsonc`（原在 `app/biome.jsonc`，用户判定根目录才正确，已删除 app 那份）。真实 IDE 设置 `.vscode/settings.json` 有 `"biome.enabled": true`（VS Code 侧 Biome 已启用，用户那边应能看到内联报错/Problems）。**`.ide/settings.json` 是镜像拷贝源，不被任何 IDE 实时读取**（改它无即时作用，需部署/同步到真实位置才生效）。终端在本环境常不可用 → Biome 查错依赖用户侧 VS Code 红线，或终端可用时 `biome ci`。
- 超时命令必须加超时参数（如 curl 加 `--max-time`）。
- **codemogger 自动重索引（✅ 2026-07-12 已落地并端到端验证）**：codemogger 索引是静态快照，`references`/`context`/`search` 只读快照、原生不自动重索引；"边改边差"会读到旧代码。方案 = **不靠 agent 自觉，把"查询前自动增量重索引"下沉为工具强制**：
  - **前提已确认**：`index` 原生就是**基于内容 hash 的增量**（`cli.mjs` `getFileHash`→`storedHash===file.hash` 则 `skipped++`，只重解析/重嵌入变更文件，`removeStaleFiles` 清理已删文件）。所以查询前重索引对未变文件很廉价（只重 hash，不重嵌入）。**不是 mtime，是内容 hash，更可靠**。watch 守护进程不采用（长驻进程按"环境保持"规矩绝不能 kill，新增风险）。
  - **实现**：外层 shim `app/codemogger-patch/codemogger-shim`（`apply.sh` 用 `install` 覆盖到 `$(command -v codemogger)`）。shim 对 `references|context|search` 三个读命令**先跑 `index <dir>` 增量刷新**再 `exec` 真 CLI；`index/list/mcp/help` 直接透传。真 CLI 经 `readlink -f` 解析（bin 被 shim 覆盖后回退硬编码 `/usr/local/lib/node_modules/codemogger/dist/cli.mjs`）。开关 `CODEMOGGER_NO_AUTOINDEX=1` 透传、`CODEMOGGER_CLI=<path>` 显式指定真 CLI；解析 `--db`/`--db=` 定位库目录。
  - **配套 stale-imports 补丁**（`patches/codemogger+0.1.5+stale-imports.patch`）：原 `removeStaleFiles` 只删 `chunks`/`indexed_files`，不删打补丁新增的 `imports` 表 → 删文件后仍出现在 `references`。补丁在按文件删除循环里补 `DELETE FROM imports`。apply.sh 以 `--forward` fuzz 应用（主补丁改了上下文，fuzz 1 正常）。
  - **端到端实测（/tmp fixture）**：新增引用文件后**不手动 index** 直接 `references` → 自动变 2 处；删该文件后直接 `references` → 自动回落 1 处。证明"新鲜度 + stale 清理"双向生效。
  - **⚠️ monorepo 用法坑（2026-07-12 实战发现）**：`references`/`context`/`search` 无 `--db` 时 db 取 `CWD/.codemogger/index.db`（`resolveDbPath` 默认 `projectDbPath(process.cwd())`），而 `index <dir>` 的 db 取 `<dir>/.codemogger/index.db`、codebase root 也记 `<dir>` 绝对路径。故**必须在 codebase 根目录（如 `app/encv-mobile/src`）下跑所有命令**，shim 自动重索引的 `dir="."` 才会正确指向该 codebase；若在 `/workspace` 跑会查到空/错 codebase（fixture 在 /tmp/cmtest 因 CWD==索引目录掩盖了此坑）。等价做法：读命令显式带 `--db app/encv-mobile/src/.codemogger/index.db`，shim 据 `--db` 反解 `dir` 并正确增量重索引。索引范围选 `app/encv-mobile/src`（本簇与下游消费方都在内，远小于整 `app/`；walker 自动忽略 node_modules/.git 且只解析有语言配置文件，svg 等跳过）；`.vue`/`.ts`/`.tsx` 均支持。
  - **⚠️ `references` 三大用法坑（2026-07-12 实战踩坑，"输出不对劲"根因）**：
    1. **`references <符号>` 是精确符号名匹配**（查 `imports.name` 字段），符号名错就空/漏。真实导出常与臆想不同：`useRunSummaries.ts` 导出 `useRunSummaries` **和** `useRunSummariesSingleton`（useTasksList 实际用后者）；`lib/taskTypeLabel.ts` **根本无 `taskTypeLabel` 符号**，导出的是 `getTaskTypeLabel`/`getTaskTypeMeta`/`getTaskTypeIcon` 等一族 → `references taskTypeLabel` 恒空。**波及面分析改用 `references <@/别名模块> --module` 按模块查最全**（不管导入哪个符号，只要 import 该模块就命中）。
    2. **`references <file> --file` 必须传绝对路径**（`imports.file_path` 存绝对路径），传相对路径 `composables/x.ts` → 空。用 `"$(pwd)/composables/x.ts"`。
    3. **只解析静态 import**，动态 `import()`/字符串引用查不到；`--module` 返回的行数含同文件多符号重复，去重看唯一文件。
  - **useTasksList 依赖簇波及面实测结论**（详见 `docs/migration-task-system.md` Phase 3 §"codemogger 实测波及面"表）：`useTaskViewCompute`（`?worker` 钉子）下游**仅簇内 useTasksList**→ 钉子影响面最小可优先动；`useTaskCancel` 是**未接线孤儿**（0 下游）应从簇提升中剔除；需改 import 的生产文件 7 个（GroupDetail.vue 最重，引 3 个簇成员）。
  - **三坑已用 shim 缓解（2026-07-12，`codemogger-shim` 已重写 + `apply.sh` 第3步 install 生效）**：坑1（符号名精确匹配→空）shim 在裸 `references <sym>` 无结果且 codebase 根下存在同名 `<sym>.{ts,tsx,vue}` 时，自动以 `--module "@/relpath"` 回退重查（模块级匹配与符号名无关，最全，stderr 提示）；坑2（`--file` 需绝对路径）shim 检测 `--file` 且 target 相对时改写为绝对路径；坑3（只解析静态 import，动态 `import()` 不可见）**不写 patch**（改 cli.mjs walker 成本高、Vue 动态 import 少），靠 grep 复核。`CODEMOGGER_EXPERIMENT.md` §8.3.1 已记三坑根因与 shim/patch 决策表。
  - **完整安装顺序**（重装 codemogger 后必做，因补丁是打进易失的全局安装、非改仓库源码）：`npm i -g codemogger@0.1.5` → `cd app/codemogger-patch && ./apply.sh`（装 kotlin grammar + 主补丁 + stale-imports + shim）→ **手动补 context 补丁**（apply.sh 未含，README 定为第二阶段）：`cd /usr/local/lib && patch -p1 --forward < .../codemogger+0.1.5+context.patch`。四命令 index/search/references/context 齐全即成功。
  - ⚠️ **两层区分（易混）**：`patches/*.patch` 文件在仓库里是**耐久的源真相**，重装/重装系统都不丢；但 `apply.sh` 把补丁打进的是**全局安装目录的单体文件** `/usr/local/lib/node_modules/codemogger/dist/cli.mjs` 并用 shim 覆盖 `/usr/local/bin/codemogger` bin 软链——这两处都是 codemogger 包自己的文件，`npm install -g codemogger` 解压 tarball 会**原样覆盖 cli.mjs、重建指向原始 cli.mjs 的 bin 软链**，故"应用后的修改"在重装 codemogger 时被抹掉。所以"重装即失效"指**已应用的修改**，不是 patch 文件；`apply.sh` 就是幂等重放器，重装后跑一次即可恢复。详见 `app/codemogger-patch/README.md` 与 `/workspace/CODEMOGGER_EXPERIMENT.md`。

## 任务系统 lift 重构状态补充（2026-07-13）

- **`lib/workflow/types` 真源分歧已调和（REFACTOR_LIFT.md #16）**：app 版 417 行 vs shared 版 291 行，现已统一——shared 为唯一真源（含 `UnifiedTreeNode`/`isUnifiedTreeNode`/`TestCaseSpec`/`TestCaseResult`/`ALL_PHASES`/`isPhase`/`WORKFLOW_STORE_KEY`/`isUnifiedTimelineEntry`），app 原位为 `export * from "@encv/shared-components/lib/workflow/types"` 垫片。
  - ⚠️ **#16 调和漏搬坑（2026-07-13 修复）**：app 原版 417 行有 `isUnifiedTimelineEntry` 类型守卫函数，shared 291 行版只搬了 `UnifiedTimelineEntry` 接口 + `isUnifiedTreeNode` 守卫、独漏该函数，导致 `unified-types.test.ts` 报 `isUnifiedTimelineEntry is not a function` + `TS2724`。已补回 `packages/shared-components/src/lib/workflow/types.ts`。**教训**：调和 `lib/workflow/types` 这类「app 版比 shared 版多 N 行」的真源分歧时，必须逐符号 diff 两侧 `export` 清单，不能只搬文档里列举的「知名类型」——函数/守卫极易漏搬，且门禁（单测 + typecheck）才会暴露。
- **`Phase` 表示统一决策（长期有效）**：shared 用 **const 对象 + 联合类型**（`export const Phase = {...} as const; export type Phase = ...`），**不用 enum**。理由：grep 全仓无 `Phase[...]` 反向映射 / `Object.keys(Phase)` / `instanceof Phase` 等 enum-only 用法，`Phase.Created` 值访问 / `Record<Phase,string>` / `toPhase():Phase` 在 union 形式下完全等价，且 const-object 更 library-friendly（无 enum 运行时）。**未来提升任何用到 `Phase` 的模块，统一用 const-object 形式，勿 reintroduce enum。**
- **`TaskTimeline.vue` / `TaskDetailModal.vue` 已提升进 shared**（`shared/components/`，import 全用 `@encv/shared-components/...`），app 副本删除、`components.d.ts` 改指 shared 路径、`tasks.*` i18n key 已在 `shared/i18n/tasks.ts` 双 locale 齐备。
- ⚠️ **并行编辑风险**：长任务重构中用户会**并行修改同一批文件**（REFACTOR_LIFT.md / components.d.ts / 删 app 副本 / 给 shared 加类型或 i18n key）。表现：我对这些文件 `replace_in_file` 报 not found / `delete_file` 报 ENOENT（用户已做）。**对策：动文件前先 `read_file` 确认当前状态；replace 报 not found 时先核验是否被用户并行处理，勿强行覆盖破坏用户成果。** 验证类命令（`pnpm check:all` / `make-shim` / `codemogger`）交用户在本地终端跑（工具后端偶发 10s 启动超时，文件编辑/搜索类工具不受影响）。
- **codemogger-patch 已安装（2026-07-13）**：`npm i -g codemogger@0.1.5` → `cd app/codemogger-patch && ./apply.sh`（kotlin grammar + 主补丁 + stale-imports + shim）→ 手动 `cd /usr/local/lib && patch -p1 --forward < .../codemogger+0.1.5+context.patch`。四命令 `index/search/references/context` 齐全；索引用本地 onnxruntime 嵌入（无需 API key）。索引范围用 `app/encv-mobile/src`（在 codebase 根目录下跑命令，或读命令带 `--db app/encv-mobile/src/.codemogger/index.db`）。`references`/`context`/`search` 由 shim 自动增量重索引。
- **任务系统 lift 组件层已全部完成（#11–#19，2026-07-13）**：`automation/*` 9 组件 + `group-detail/PipelineTab` 已于 #19 提升进 shared（`PipelineTab` 顺带把 `tasks.pipelineEmpty` 沉入 shared i18n）。至此所有任务系统相关组件/composables/lib/api/常量/i18n 均在 shared 并留 app 垫片，shared 非测试代码 `@/`-free。**`pnpm check:all` 已 8/8 全绿（2026-07-13 收尾）**。

## 去垫片纯化（结构性改革范式 · 2026-07-13）

- **垫片是「迁移的谎言」，现已进入去垫片阶段**：模块提升进 shared 后，app 原位的
  `export * / export {…} from "@encv/shared-components/..."` 垫片只是转发壳，不是真源。
  纯化目标 = **删除全部垫片**，让 `@/` 经二级回退（tsconfig `@/*` + vite/vitest 的
  `encv-alias-fallback` 插件）**直接解析到 shared 真源**。这样 shared 是唯一事实来源、
  app 只剩组合 + DI 胶水。
- **安全机制 `scripts/make-shim.mjs prune`**（已落地）：`prune`（dry-run 列出可删同名垫片 +
  需先改 importer 的错位垫片）；`prune --apply` **只删同名垫片**，错位垫片保持不动避免静默断链。
  `check-all` 在 0 垫片时输出「✔ 无残留垫片（已纯化）」。
- **⚠️ 2026-07-14 更新：73 转发壳已全部删除，`prune --apply` 的「删后 @/x 自动落 shared 零风险」前提已失效。**
  批 9 已摘除 `encv-alias-fallback` 的 shared 兜底分支（`vite.config.ts`/`vitest.config.ts` 的 `dirs`/`roots` 仅留本地 src），
  现在 `@/x` **只解析本地**，不再回退 shared。因此**删壳前必须先把所有 `@/<壳>` 与相对路径 importer 改写为
  `@encv/shared-components/<壳>`**（本次即此顺序：改 114 处 importer 跨 47 文件 → 删 73 壳，check-all PASS 8/FAIL 0 + vite build ✓）。
  纯壳判定用结构判定（只有 `export ... from "@encv/shared-components/..."`，无 import/本地引用/其它 export），
  精确排除 `api/encv.ts`(barrel)/`i18n/index.ts`/`useAgent.ts` 等混合文件。今后若再有壳需删，沿用「先改 importer 再删壳」。
- **同名 vs 错位判定**：shim 相对路径 == shared 真源相对路径 → 同名（可删）；否则错位。
  已知错位两例（#35 已手动清理）：`api/encv_core`→shared `api/core`（importer
  `FullTextIndexDetail.vue:256` 改 `@/api/core`）、`features/alist-encrypt/useAlistEncrypt`→
  shared `composables/useAlistEncrypt`（importers `useFilesView.ts:100`/`FileInfo.vue:205` 改
  `@/composables/useAlistEncrypt`）。清理后 `grep` 两路径全仓 0 命中。
- **后续**：终端恢复后跑 `node scripts/make-shim.mjs prune --apply` + `node scripts/make-shim.mjs
  check-all` + `pnpm check:all`（8/8）。若 check:all 报「找不到导出」，说明有未预见的错位垫片，
  回到 `prune` dry-run 列出的错位项按 #35 范式改 importer 再删（删除均 git 可还原，勿盲目回滚）。
- **✅ 逻辑抽象改革（批 J / 2026-07-14）：格式化单一真源**：shared 内文件大小格式化 3 处重复
  （`api/encv_files.formatFileSize` 经 `api/encv` barrel 公开导出 + `lib/buildReportZip.formatBytes` +
  `components/TaskPerformanceSection.formatBytes`）已收敛到 **`lib/format.ts` 的 `formatBytes(bytes?)`**（1024 进制、
  B/KB/MB/GB/TB、undefined→""、<=0→"0 B"、clamp 越界、toFixed(1)）。`formatFileSize` 是其公开别名（委托，消费方无感）。
  门禁 PASS 8/FAIL 0 + vite build ✓。日期格式化（`useDateFormat.formatDateTime` vs `PerformanceTab.formatTime`）
  与 `formatDateInput`（HTML date input 契约，不可动）仍分散，收敛 `formatTime→formatDateTime` 待用户拍板（涉及显示格式统一）。
- ⚠️ **`encv-alias-fallback` 插件是路径拼接式回退**：`@/<rel>` 先试 `encv-mobile/src/<rel>` 再试
  `packages/shared-components/src/<rel>`（含 `.ts/.vue/index` 候选）。删除同名垫片后 `@/x` 直接落
  shared；但**名称错位垫片删除前必须改 importer**，否则插件拼不出 shared 真源路径会断链。
- ✅ **`add_key` 两个引号 bug + `move-key` 重复注册缺陷（2026-07-13 踩坑，已于同日修复）**：原 `scripts/i18n_lib/addkey.py` 的 `add_key` 在批量下沉 i18n key 时会破坏 shared 字典——① value 含双引号（如 `"{query}"`）时双引号包裹插入 → TS 语法破坏（`Expected ',', got '{'`）；② shared 字典 en 键是 `en: {`（无引号），正则只匹配 `"en": {` → **en 部分从不插入**（MISSING_EN 根因）。**已修复**：`insert_key_into_section` 的 locale 正则改为 `["']?{locale}["']?` 同时匹配带/不带引号；value 含双引号时改用单引号包裹（与字典现有约定一致），同时含单双引号则转义双引号。另 `movekey.py` 的 `_register_shared_module` 原幂等判断只看 `, {module}]` 字面量、且数组末元素后无 `]` 会误判 → **已修复为数组元素级匹配** `(^|,\s*){module}(\s*,|\s*\]|\s*$)`，重复注册（如 `tasks` 出现两次）不再发生。**现在可安全用 `move-key "<prefix>." --from encv-mobile --to shared --keep --register` 批量下沉 i18n key**，无需再 `cp` 整文件绕过。
- **⚠️ flat-shared + 子目录 consumer 导入坑（2026-07-13 实测修复）**：shared `components/` 是**扁平**的（无 `automation/`/`group-detail/`/`tasks/` 子目录），但 #17/#18/#19 把这几个子目录组件提升进 shared 扁平层后，**consumer 视图仍用 `@/components/<subdir>/X.vue` 旧路径**——`encv-alias-fallback` 只做「本地 src → shared 同路径」精确回退，无法把 `@/components/group-detail/X` 映射到扁平的 `shared/components/X`，导致解析失败。已修复的 6 处：`PluginTestsDetail.vue:259`（`automation/StepMiniBadge`）、`GroupDetail.vue:145-147`（`group-detail/{PerformanceTab,TasksTab,PipelineTab}`）、`Tasks.vue:617-618`（`tasks/{TaskDebugPanel,TaskVirtualList}`）——全部改写为 `@encv/shared-components/components/<FlatName>.vue`。**今后提升带子目录的组件时，必须同步改写所有 consumer 的 import 到扁平 shared 路径**，否则构建/单测静默失败（`pnpm check:all` 才暴露）。`agent/`、`developer/`、`shared/` 子目录仍在 app 或 shared 中保留，其 `@/components/<subdir>/` 导入正常。



## 项目 skill 注册约定（2026-07-15）
- 项目内 skill 真源在 .agents/skills(capawesome/ionic/skill-creator)、.trae/skills(cypress/ffmpeg)、agent/.../video-encrypt。
- 注册清单 skills-lock.json(agentskills.io 规范)的 skillPath 必须指向真实目录(.agents/skills/...)，曾错误地写 skills/ 导致失效。
- 让 CodeBuddy 识别：在 .codebuddy/skills/<name> 建符号链接到上述真源(单一真源，无复制)。**已确认 CodeBuddy 正常识别 .codebuddy/skills 下的符号链接 skill**（用户 2026-07-15 验证）。
- **skill 由 MCP 管理（scripts/skill-manager.mjs，app-dev MCP 的 9 个 app_skill_* 工具）**：多 skill 路径列表(skillPaths，默认 .codebuddy/.agents/.trae skills)持久化于 .codebuddy/skill-registry.json；路径可增删、skill 可 CRUD(add 三法 import/npx/git)、全程 fs.watch 监视(跟随符号链接、抗原子保存)。扫描/列表/增删改均走此模块，是 skill 生命周期的权威。注意：扫描时跟随符号链接，故 .codebuddy/skills/* 符号链接也能被识别(共 31 个 skill)。
- app_exec MCP 安全门禁规则位于 scripts/app-dev-guard.mjs，server 热更新(监听目录按 basename 过滤，抗原子保存)，改规则即时生效无需重启；`app_guard_reload` 工具可手动重载并返回规则数。

## MCP 注册单一真源（2026-07-15）
- 权威文件：**/workspace/.codebuddy/mcp.json**（`mcpServers` 含 codemogger/app-dev/web-fetch，绝对 /workspace 路径）。.cnb.yml 流水线「构建 codemogger-patch 并注册 MCP」stage 用 `cp /workspace/.codebuddy/mcp.json /root/.codebuddy/mcp.json` 覆盖用户级注册，去掉原内联 printf 块。**改 MCP 只改项目文件 + 重跑流水线**，不要直接改 /root/.codebuddy/mcp.json（会被下次流水线覆盖）。
- 注册后开新对话生效（同会话内工具列表缓存需刷新），不必重启 IDE。

## web-fetch MCP（2026-07-15）
- 高级 web_fetch 替代：`scripts/web-fetch-mcp.mjs`（stdio MCP server，注册名 `web-fetch`）。能力：retry(指数退避+Retry-After)、内容嗅探(magic bytes 复核声明的 content-type)、SPA 检测+可选 headless 渲染(puppeteer/playwright)、代理(HTTP/HTTPS CONNECT 隧道，零依赖)。核心函数导出，main 守卫启动 server，可 `node -e "import('/workspace/scripts/web-fetch-mcp.mjs')"` 单测。门禁/构建类走 app-dev，网页抓取走 web-fetch。

## 前端主题重构（ENCV 共享包 / daisyUI / Ionic 桥接 · 2026-07-15）
- 权威方案文档：`/workspace/ENCV前端主题重构方案.md`（daisyUI v5 + GSAP 重塑，共享包为主；Phase 0–5）。
- **⚠️ vite 8 (rolldown) 打包 vite.config 的 ESM interop 坑（长期）**：把 `@tailwindcss/vite` 接入 encv-mobile vite 配置（`daisyUiPlugin()`，来自 `packages/shared-components/src/vite-plugins/daisy-ui.ts`）时，rolldown 把该依赖默认导出 interop 弄坏 → 构建报 `tailwindcss_vite_..._index_mjs.default is not a function`。运行时 `import('@tailwindcss/vite').default` 确为 function（node 验证过），纯属配置打包 interop。**影响**：主应用无法经 Tailwind 管线接入 daisyUI。**已采用替代**：encv-mobile 改引纯 CSS 入口 `packages/shared-components/src/styles/theme-core.css`（`@import tokens.css + palette.css + bridge.css + components.css`，不依赖 Tailwind）统一调色板；插件 web 仍用 `daisyui.css`(@plugin daisyui/theme 经 Tailwind)。**后续要让主应用用 Tailwind 工具类，须先解决此 interop**（daisy-ui.ts 做 default 兜底，或 vite 配置 external 化 @tailwindcss/vite），勿重复踩坑。
- **调色板单一来源现状**：插件走 `daisyui.css` 的 `@plugin "daisyui/theme"`(encv/encv-dark)；主应用走 `palette.css`(纯 CSS 等价，值须与前者同步)。`bridge.css` 把 Ionic `--ion-color-*` 桥接到 daisyUI `--color-*`，主应用与插件共用。
- **🔒 技术栈解耦 ACL（2026-07-15 落地，app_check_all 全绿）**：设计目标「换 gsap+daisyui，下游应用/插件零改动」。
  - 动效：gsap 收敛进唯一 `packages/shared-components/src/motion/internal/gsap-engine.ts`（全仓唯一 `import gsap`）。对外契约 `src/motion/internal/types.ts` 的 `MotionEngine` 接口（引擎无关类型，无动画库 import）。12 个 composable + guard + index 全部经 `import { motion } from "./internal"`，公共签名仅用我们的类型（无 `gsap.TweenVars`/`ScrollTrigger`/`Flip` 泄漏）。`tokens.ts` 的 `EASE` 为语义键，由引擎 `EASE_MAP` 映射。`internal/index.ts` 的 `export { motion }` 是「换库唯一开关」。
  - **noop 引擎 + 全局开关（2026-07-15 续7 加）**：`motion/internal/noop-engine.ts` 实现 `MotionEngine` 全 no-op（直接落终态），桶导出 `noopMotion`（改 `internal/index.ts` 一行即全局换 no-op）。`motion/guard.ts` 新增运行时总闸 `setMotionDisabled(bool|null)`/`getMotionDisabled()`（null=跟随系统 reduced-motion；true=强制全关；false=强制开），`getMotionProfile().enabled` 据此算。**注意与 `registry.ts` 的 `setMotionEnabled(name, enabled)`（按命名动画开关）同名冲突——全局开关必须叫 `setMotionDisabled`**，否则 TS2308。
  - 主题：稳定视觉词汇 = CSS 变量(`palette.css`) + `.encv-*` 组件/工具类(`theme/components.css`，纯 CSS、零 `@apply`、只吃令牌)；daisyui 的 `@plugin` 块是唯一切换点。`daisyui.css` 已移除泄漏 daisyui 类的 `@layer components` 块。
  - **useTheme 解耦（2026-07-15 续7）**：`applyColor` 不再手搓 Ionic `--ion-color-primary*` 的 shade/tint（原 lighter/darker 数学已删），改为只写 daisyUI 语义令牌 `--color-primary` + JS 补 `--ion-color-primary`/`-rgb`/contrast/`-contrast-rgb`；shade/tint 由 `bridge.css` 的 `color-mix(var(--color-primary)...)` 自动派生。新增语义别名 `setPrimaryColor`(=setThemeColor)。换主色时 Ionic 与 daisyUI 组件共用同一派生链。
  - 换栈步骤：(a) 新增一个实现 `MotionEngine` 的文件，改 `internal/index.ts` 一行；(b) 重写 `daisyui.css` 的 `@plugin` 块（palette.css/components.css 不动）。下游零改动。
- Phase 0/1/ACL/noop 引擎/useTheme 解耦已落地，`app_check_all` 全绿。Phase 3 动效试点已接通首个真实下游 `encv-mobile/src/views/ExtensionsPage.vue`（`useScrollReveal` 对 `.extensions-list` 错峰淡入，含 `ready` 异步闸门）。Phase 4=组件迁移到 .encv-*/bg-base-* + Appearance 重组；Phase 5=用户主题/Snippets 闭环。
