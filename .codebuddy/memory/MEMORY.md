# 长期记忆（跨会话稳定事实）

## 架构权威文档（encv-mobile / shared-components 重构）
- **权威文档：`app/docs/migration-task-system.md`**（状态：执行中）。核心设计 = **拆分抽象与实现**：
  - `@encv/shared-components` = 纯抽象层/库，只依赖 vue/pinia/ionic/通用第三方，**不依赖** `@/config` `@/constants` `@/router` 等应用上下文。
  - `encv-mobile` = 应用层，提供共享抽象所需的**注入上下文**（base URL/认证/容器版本等，经 `registerSharedTaskServices` 注入），组合 shared 抽象 + 应用专属业务。
  - 关键原则（§0）：抽象进 shared = **重写逻辑 + 依赖注入解耦**，不是搬运文件。
- **`docs/shared-components-boundary-spec.md` 是过时稿，与权威设计直接相反**（它主张"encv 业务全搬回 encv-mobile"，会摧毁已落地的 DI 共享抽象层）。**不要执行它的 Phase 1/2/3（搬 stores/api 出 shared）**。读它前先确认是否该被废弃/改写指向 migration-task-system.md。
- 已落地事实（2026-07-10~11）：Phase 1 ✅（shared/api/core 基座 + types/task.ts 领域类型）；Phase 2 store ✅（taskStore/runTasksStore 提升进 shared/stores，DI 解耦，encv-mobile 留 re-export 垫片）；非任务 api（shared/api/encv_*.ts 非任务域）仍为"暂存残留"待 Phase 6/A-ext 清理回退；`useEventBus` shared 孤儿副本待删（保留 encv-mobile canonical）。
- Module G（通用 composables 去重）：encv-mobile 原有多份与 shared 重复的 composables。**2026-07-11 已完成 G-1/G-2**：encv-mobile/src/composables 下 useToast/useClipboard/useDateFormat/relativeTime/activeStatus（re-export 垫片）+ useSearchInput（与 shared 逐字节相同）已删除，全靠 `@/*` 别名二级回退到 shared（tsconfig `"@/*":["./src/*","../packages/shared-components/src/*"]`）。剩余 G-3 VirtualLogList.vue、G-4 useEventBus。**⚠️ useEventBus 结论反转**：encv-mobile 的 useEventBus.ts 已不存在，11 处 `@/composables/useEventBus` 引用回退到 shared 副本=事实 canonical，故**保留 shared 的 useEventBus.ts，禁止删除**（原文档"删 shared 孤儿"指令有害）。
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
- `execute_command` 在工具后端偶发 10s 启动超时（连 `mv`/`echo` 都失败），与 shell 配置无关；需跑的命令（如 `pnpm check:all`/`pnpm typecheck`）交给用户在自己终端跑并贴回。文件编辑/读取/搜索类工具完全不受影响。
- 超时命令必须加超时参数（如 curl 加 `--max-time`）。
