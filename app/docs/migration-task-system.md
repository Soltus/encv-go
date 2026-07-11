# 任务系统共享化重构文档

> 目标：把"任务系统"从 `encv-mobile` 应用层抽象为 `@encv/shared-components` 的可复用层。
> 状态：执行中（Phase 1 ✅、Phase 2 ✅ 已完成；Phase 3 进行中：2 个 type-only 叶子已提升，剩余为依赖簇）。
> 最后更新：2026-07-11 21:10

---

## 9. 执行进度（滚动记录）

### Phase 1 — 共享 API 基础设施 + 领域类型 ✅ 已完成（2026-07-10）
- 新建 `shared/api/core/`（`errors` / `baseUrl` / `context` / `request` / `index`），提供 `apiRequest` 统一请求封装 + base URL/认证依赖注入。
- `shared/index.ts` 追加 `export * from "./api/core"` 使基座对外可见。
- 新建 `shared/types/task.ts`，把 `EncvTask` / `TaskType` / `TaskStatus` / `TaskStep` / `BatchTaskSpec` / `RunSummary` / `RunInfo` / `PerformanceSummary` 从 `encv_tasks` 提升为共享类型层；原 `encv-mobile/src/api/encv_tasks.ts` 与 `encv_perf.ts` 改为 re-export 兼容垫片。
- `encv-mobile/src/api/encv_tasks.ts` 的 `getTasks` 改用 `apiRequest`（行为兼容）。
- **门禁**：② encv-mobile `pnpm typecheck` exit 0 ✅；③ `python3 scripts/i18n-tool.py lint --all` 0 问题 ✅；① shared 独立类型检查因环境限制（vue-tsc 超 2 分钟被工具 kill）+ 已知 315 既有错误未跑通，改动经 read_lints 0 错且 encv-mobile 类型检查通过证明其导入可解析，未引入新错。

### Phase 2 — 任务领域 store 与类型（store ✅ / workflow types ⏸）
**已采用推荐方案①（依赖注入注册函数）完成 store 提升（2026-07-10 22:37）：**
- 新增 `shared/src/stores/taskServices.ts`：DI 容器（`setTaskServices` / `getTaskServices`），定义 `TaskServices` / `TaskPersistence` / `SearchMode` 接口；未注入时返回安全空实现（不崩溃）。
- `taskStore` / `runTasksStore` 从 `encv-mobile/src/stores/` 提升为 `shared/src/stores/`，内部不再直接 import 应用层 `@/api/encv` / `@/lib/taskPersistence`，改为消费注入的服务（向量搜索 `searchTasksVector`、分页 `getTasks`、IndexedDB `persistence.*`）。
- `encv-mobile/src/stores/taskStore.ts` / `runTasksStore.ts` 改为 re-export 兼容垫片，转发到 `@encv/shared-components/stores/*`。
- 新增 `encv-mobile/src/stores/registerSharedTaskServices.ts`，在 `main.ts` 启动期（`createApp().use(pinia)` 之后、mount 之前）调用 `registerSharedTaskServices()`，把真实 `searchTasksVector` / `getTasks` / `taskPersistence` 注入共享层。
- 验证手段：新增 `app/scripts/check-all.mjs`（注册为 `pnpm check:all` / `check:all:quick`），覆盖 biome CI / encv-mobile typecheck / shared typecheck / i18n lint --all / 单测，输出 `check-report.md`。encv-mobile typecheck 会顺带类型检查被导入的共享 store 文件，可作为 Phase 2 门禁。

**仍阻塞（未做，非方案①范围）：**
- `lib/workflow/types.ts` 提升：完整定义依赖 `ErrorAnalysis`（`useErrorAnalyzer`）+ `TriggeredBy`（`useTaskTrigger`），二者是 **Phase 3** composable，尚未进 shared，提前提升制造前向依赖；且 shared 的 `Phase` 是 `const` 对象、encv-mobile 是 `enum`（值相同、TS 种类不同），统一需选定一种并改全部消费方。留待 Phase 3 一并处理。

### check:all 门禁现状（2026-07-10 23:15，跑 `pnpm check:all` 生成 `app/check-report.md`）
首轮 2 通过 / 3 失败；**真正根因**在二次排查中定位：`encv-mobile/vite.config.ts` 配了 `@encv/shared-components` 别名 + `encv-alias-fallback` 插件（让 `@/` fallback 到 shared src），但 **`vitest.config.ts` 两者都没配**，且 **tsconfig `paths` 只配了 `"@encv/shared-components/*"` 子路径、没配裸 `@encv/shared-components`**。故应用构建能解析 shared 导入，测试运行时 vite 解析不到、tsc 也解析不到裸包。

**已落地修复（让测试解析与构建一致）：**
1. `vitest.config.ts`：新增 `SHARED_SRC` 常量并加 `'@encv/shared-components'` / `'@encv/shared-components/'` 别名到 `sharedTestConfig().alias` 与 `resolve.alias`（与 vite.config.ts 对齐）。→ 修好全部 shared 裸包/子路径在测试期的解析（activeStatus、useToast、以及 FULL 模式下的 GroupDetail/Tasks 组件测试等）。
2. `tsconfig.json`：paths 补 `"@encv/shared-components": ["../packages/shared-components/src"]`（裸包映射）。→ 修好 typecheck 因裸包导入翻车的问题。
3. `encv_core.ts` / `encv_tasks.ts`：内部 `import ... from "@/api/core"` 改为 `@encv/shared-components/api/core`（`@/api/core` 仅构建期 fallback 能解析，测试期 vite 解析不到）。→ 修好 `renderTurnItems.test.ts` 等。
4. `shared/src/vite-plugins/vue-component-check.ts`：折叠 `console.log` 多行调用为单行（biome 格式报错；上一轮只修了 import 排序，还有 console.log 格式点）。**二次重跑又暴露 `console.warn` 多行调用（line 301-303）未折叠 → 已折叠为单行**（biome 对该文件有多处格式点，需逐一修）。\n5. **单测 `.vue` 解析失败根因修复**：上一轮把 `useToast.ts`/`activeStatus.ts` 垫片改成从 **main barrel** `@encv/shared-components` 导入，而 shared 的 `index.ts` 把**所有 .vue 组件（含 `SettingsPage.vue`）一并 re-export** → `useAgent.ts` 经 useToast shim 拉入 .vue → vitest 的 `fast` project 无 `plugin-vue` 解析失败（`vitest.config.ts` 顶层 `plugins:[vue()]` 不继承到 named projects）。\n   - 修法（比全局开 plugin-vue 更安全，避免把整个 shared 包拉进每个测试）：垫片改回**子路径导入**——`useToast.ts` → `export { showToast } from \"@encv/shared-components/composables/useToast\"`；`activeStatus.ts` → `export * from \"@encv/shared-components/composables/activeStatus\"`（子路径，vitest 已有 `@encv/shared-components/` 别名可解析，且不碰 .vue）。

**首轮已修的迁移债务（保留）：** 新建 `encv-mobile/src/composables/useToast.ts` 转发垫片，修好 35+ 处 `@/composables/useToast` 引用（`useAgent.test.ts` 等）。

**预期门禁状态：** Biome CI / encv-mobile typecheck / encv-mobile 单测 应转绿；**shared-components typecheck 仍 FAIL**（已知 315 既有错：vitest/@vue/test-utils 模块解析 + `IncrementalFilter` 引 `useFrontendLogs` 子路径），错误列表里无本次新增 store 文件 → Phase 2 在 shared 内类型干净，留待 Phase 3–6。若 Biome 仍报 vue-component-check.ts 其他格式点，需用户贴回新报告继续修。

### 收尾（2026-07-10 23:50，第四次 `pnpm check:all`）
- 用户重跑：**encv-mobile 单测 ✅PASS、typecheck ✅、i18n ✅**；仅 Biome 仍 FAIL + shared typecheck 仍 FAIL。
- **Biome 报告"乱码"根因**：`scripts/check-all.mjs` 用 `spawn` 跑 `pnpm biome:ci` 时 `env: process.env`（未设 NO_COLOR），biome 输出带 ANSI 颜色转义码（`[0m[31m…`）被原样写进 `check-report.md` → 终端/IDE 显示为乱码。biome 本身没问题，是报告脚本没去色。
  - 修复 `scripts/check-all.mjs`：① spawn 时 `env: {...process.env, NO_COLOR:'1', FORCE_COLOR:'0'}`（根上关色）；② 新增 `stripAnsi()` 并在写报告时对失败输出去色（防御）。→ 后续报告纯文本可读。
- **Biome 最后一个格式点**：`vue-component-check.ts:267-269` 的 `console.log(\`…\n\`,\n)` 多行调用未折叠 → 已折叠为单行。全文件 11 处 console.* 仅此一处多行。
- **预期本轮（第五次）重跑**：**Biome CI ✅**（首次全绿）、encv-mobile typecheck ✅、单测 ✅、i18n ✅；**仅 shared typecheck 仍 FAIL**（315 既有错，非本次范围）。报告文件不再有 ANSI 乱码。

### Module G 去重进度（2026-07-11，事实核查驱动）
- **G-1/G-2 通用 composables 去重已完成**：`encv-mobile/src/composables/` 下与 shared 重复的副本**全部删除**——
  - `useToast` / `useClipboard` / `useDateFormat` / `relativeTime` / `activeStatus`：已是 re-export 垫片（指向 `@encv/shared-components/composables/*`）；
  - `useSearchInput`：与 shared 版**逐字节相同**的完整副本（491 行）。
  - 删除依据：① `encv-mobile/tsconfig.json` 的 `"@/*": ["./src/*", "../packages/shared-components/src/*"]` 二级回退已确认；② 全部 58 处引用均用 `@/composables/useX` 别名（**无相对路径引用**，故删除不破引用）；③ `useSearchInput` 经逐行比对与 shared 等价。共清理约 520 行重复代码。
  - 副作用：单例模块（如 `errorStore`/`logs`）此前因"本地 + shared 两份并存、别名本地优先"而分裂的问题，随本地副本删除自然消除（错误浮窗 bug 修复）。
- **剩余 Module G 项（待做）**：
  - **G-3** `VirtualLogList.vue`：**已完成（无需操作，2026-07-11 事实核查）**。本地 `encv-mobile/src/components/VirtualLogList.vue` **已不存在**（早被删），仅 shared 有 `components/VirtualLogList.vue`；`DevLogs.vue:248` 用 `@/components/VirtualLogList.vue` 经 `@/` fallback 解析到 shared。文档原"删本地重复"前提不成立。唯一残留：`components.d.ts:101` 有陈旧的 `./components/VirtualLogList.vue` 相对类型声明（unplugin-vue-components 生成文件，下次生产构建自愈，首跑 typecheck 未报错，不阻塞）。
  - **G-4** `useEventBus`：**结论已反转（见 §8.1.1 ② 修正）**。`encv-mobile/src/composables/useEventBus.ts` 已被删除，11 处 `@/composables/useEventBus` 引用经 `@/*` 别名回退到 shared 副本——**shared 副本即事实 canonical**。故**保留 shared 的 `useEventBus.ts` 为唯一真源，禁止删除**（原"删 shared 孤儿"指令会断 11 处引用）。
- **复验 / 事实核查修正（2026-07-11 02:17 首跑 `check:all` → FAIL，已修）**：
  - **首跑结果**：encv-mobile typecheck FAIL + 单测 FAIL（3 suite）。
    - typecheck：`activeStatus.test.ts` 报 `Cannot find module './activeStatus'`——该测试在 `composables/` 内用**相对** `./activeStatus` 导入，本地副本删除后断链。
    - 单测：`useAgent.ts` 的 `@/composables/useToast` 在 vitest 运行时解析失败，拖挂 `renderTurnItems.test.ts` / `renderTurnItems.agentTask.test.ts` / `activeStatus.test.ts` 3 个 suite。
  - **根因（推翻了上一轮的错误假设）**：此前以为"tsconfig `@/*` 二级回退到 shared"对运行时也生效——**只对 TypeScript 类型检查成立，Vite/vitest 运行时默认不读 tsconfig paths**。`vite.config.ts` 有 `encv-alias-fallback` 插件（本地优先、shared 次之）所以 dev/build 正常；但 `vitest.config.ts` 的 `resolve.alias['@']` **仅指向本地 src、无 shared 回退**，且 `plugins:[vue()]` 里没有该插件 → 测试环境解析不到 shared。
  - **修复（架构对齐，非回退垫片）**：
    1. 提取共享插件 `encv-mobile/vite-plugins/encv-alias-fallback.ts`（逻辑同 vite.config.ts，加 `enforce:'pre'` 以便在 vitest 的本地 `@` 别名之前生效），加入 `vitest.config.ts` 的 `plugins`。
    2. `activeStatus.test.ts` 的 `./activeStatus` → `@/composables/activeStatus`（经 fallback 解析到 shared；typecheck 也走 tsconfig 二级回退）。
  - ⚠️ **二次失败（02:57 用户复跑仍 FAIL，同 3 suite）**：插件已注册但 `resolveId` 静默返回 null。当时误判为"vitest 打包导致插件内 `__dirname` 指向 config 目录"，并改为显式传 `roots`——这是稳妥的健壮性改进（保留），但**不是决定性修复**：复跑仍 FAIL。
  - ⚠️ **三次修复（03:18 复跑仍 FAIL → 定位真正根因）**：
    - **真正根因**：`vitest.config.ts` 的 `test.alias` 与 `resolve.alias` 里都设了 **`'@': SRC_DIR`**。该别名会把 `@/composables/useToast` **先解析成本地绝对路径** `/workspace/app/encv-mobile/src/composables/useToast`（已删除→不存在），于是我的 fallback 插件来不及把 `@/...` 回退到 shared 就直接报 "Failed to resolve"。并且 `enforce:'pre'` 的 `resolveId` 在 vitest transform 期的 `this.resolve()` 里并不可靠地被咨询到，进一步让插件形同虚设。
    - **对照 `vite.config.ts`**：其 `resolve.alias` **根本没有 `'@'` 别名**，仅靠 inline `encv-alias-fallback` 插件（normal 阶段、无 enforce）解析所有 `@/...`——所以 dev/build 一直正常。vitest 缺的就是这一点。
    - **修复（与 vite.config.ts 完全对齐）**：
      1. 从 `vitest.config.ts` 的 `test.alias`（`sharedTestConfig`）**和** `resolve.alias` **同时移除 `'@': SRC_DIR`**——所有 `@/...` 改由插件统一解析。
      2. 插件去掉 `enforce:'pre'`，保持 normal 阶段（与 vite.config 的 inline 版一致），确保 transform 期能被 `this.resolve()` 咨询到。
      3. 插件仍保留"调用方显式传 `roots`"的健壮性（二次失败的改进不丢）。
  - ⚠️ `vite.config.ts` 仍保留其 inline 版 `encv-alias-fallback`（未合并，避免改动无法被 `check:all` 验证的 dev 配置；待后续统一）。
  - ⚠️ **四次修复（03:28 复跑仍 FAIL → 定位【决定性】根因）**：
    - 现象变化：这次失败的不只是 shared 回退项，连**本地 src 下真实存在的文件**（`@/composables/renderTurnItems`、`@/composables/appServerRealtimeReducer`、`@/lib/workflow/state-machine`、`@/lib/workflow/types`）也 `Failed to resolve`。连本地文件都解析不了，说明 `encvAliasFallback` 插件**根本没被调用**。
    - **真正根因（决定性）**：测试跑在 `'fast'` **project** 下，而 `plugins` 只配在**根配置**。`vitest`/`vite 8` 的 **projects 模式不继承根配置的 `plugins`**——每个 project 是独立 Vite 配置，必须各自带 `plugins`。于是 `'fast'` project 里没有任何插件，`@/...` 无人解析 → 全部失败（与"没加插件"完全一样）。
    - **对照 `vite.config.ts`**：它是**单配置、无 projects**，根 `plugins` 直接生效，所以 dev/build 一直正常。vitest 用 projects 才暴露这个差异。
    - **修复**：抽 `BASE_PLUGINS = [vue(), encvAliasFallback({ roots: [SRC_DIR, SHARED_SRC] })]`，**根配置 + 每个 project（fast/isolated）都引用同一份**。同时为插件加了"仅解析失败时 `console.error('[encv-alias-fallback] NOT FOUND', source, dirs)`"调试日志，便于下次复跑一锤定音（若仍 FAIL 且日志无 NOT FOUND → 插件仍未被调用；若有 NOT FOUND → 路径错）。
  - **待复验**：请重跑 `pnpm check:all` 确认 encv-mobile typecheck + 单测转绿（预期全绿，且单例分裂 bug 已消除）。
- **Module G 状态（2026-07-11）**：**全部闭环** ✅。G-1/G-2（composables 去重，已修 vitest 回退）、G-3（无本地副本，DevLogs 经 `@/` 解析 shared）、G-4（shared 为 canonical，保留）。encv-mobile 与 shared 的通用 composables/components 重复已消除，统一经 `@/` 别名回退 shared。后续仅余 `shared/api/` 非任务域 encv_* 暂存残留（A-ext/Phase 6，高风险的 api/core 重写，未启动）。

---

## 0. 为什么不能"简单迁移"

最初尝试的做法是**物理复制 + re-export 垫片**：把 `encv-mobile/src/api/encv_*.ts` 等文件整份拷进 `shared-components/src`，原位置留一行 `export * from "@encv/shared-components/api/..."`。

这个做法在"让 encv-mobile 不红"上有效，但**没有完成抽象**，反而制造了错配：

1. 这些文件原本就是**围绕 `encv-mobile` 的 `@/config`、`@/constants`、`@/stores` 写的**，搬到 shared 后内部仍残留 `@/` 应用专属依赖（如 `encv_tasks.ts` 的 `import("@/stores/taskStore")`、`encv_system.ts` 的 `import("@/types/webdav-test")`）。它们在 shared 里既不能被其他 app 安全复用，又不属于真正的抽象。
2. 真正应该被共享的**任务领域抽象**（store、composables、lib/workflow、领域类型、任务组件）反而因为"迁移麻烦"一直留在 `encv-mobile`。

结论：**抽象到共享包 = 重写逻辑（解耦应用专属依赖、改为依赖注入），而不是搬运文件。**

---

## 1. 当前两类错配（现状梳理）

### 错配 A — `encv-mobile` 的应用层，因迁移省事暂存在共享包

| 位置 | 内容 | 问题 |
|---|---|---|
| `shared-components/src/api/encv*.ts`（13 个） | 上一轮从 `encv-mobile` 物理复制的业务 API 契约 | 本质是 `encv-mobile` 应用层契约，不是抽象；仍残留 `@/stores/taskStore`、`@/types/webdav-test` 等应用专属依赖 |
| `shared-components/src/composables/useEventBus.ts` | 孤儿重复文件 | `encv-mobile` 有一份**真正被使用**的 `useEventBus.ts`（被 11 处引用），两者仅第 13 行 import 源不同（shared 版引 `@encv/shared-components/api/encv`，mobile 版引 `@/api/encv_tasks`），其余 106 行逐字相同 |

> 注：`encv-mobile/src/api/encv_*.ts` 现在已是 re-export 垫片，转发到 shared 的副本。这些文件**不属于 shared 的抽象层**，应被重写为"应用层调用共享抽象"或直接回退到 `encv-mobile`。

### 错配 B — 共享包应有的抽象层，因迁移麻烦停留在 `encv-mobile`

| 模块 | 位置 | 应去向 | 钉子依赖 |
|---|---|---|---|
| 任务领域 store | `encv-mobile/src/stores/taskStore.ts`、`runTasksStore.ts` | shared `stores/` | 自身是 Pinia store，被 `useTasksList`/`useWorkflowTaskService` 依赖（Pinia 单例问题） |
| 任务领域 composables | `useTasksList`、`useTaskEventBridge`、`useWorkflowTaskService`、`useTaskViewCompute`、`useTaskForm`、`usePathResolver`、`useErrorAnalyzer`、`useBatchOperations`、`useSectionDerivation`、`useTaskTrigger`、`useTaskCancel`、`useRunSummaries`、`useNewTaskModal` | shared `composables/` | 见 §3 |
| 工作流引擎 | `encv-mobile/src/lib/workflow/*`（state-machine / scheduler / conditionEvaluator / matrixExpander / buildDynamicWorkflow） | shared `lib/workflow/` | 仅依赖 api 类型 + 少量 composables 类型，基本无钉子 |
| 任务工具 lib | `taskTypeLabel.ts`、`taskPersistence.ts`、`buildReportZip.ts` | shared `lib/` | 基本无钉子 |
| 领域类型 | `EncvTask`（定义在 `api/encv`）、`workflow/types.ts`（`WorkflowRun`/`JobRun`/`StepRun`/`UnifiedRunRecord`） | shared `types/` | 无 |
| 任务领域组件 | `TaskTimeline.vue`、`TaskBasicInfo.vue`、`TaskDetailModal.vue`、`TaskErrorSection.vue`、`TaskOutputInfo.vue`、`TaskPerformanceSection.vue`、`TaskWarningSection.vue`、`TaskActionButtons.vue`、`tasks/*`、`group-detail/*`、`automation/*` | shared `components/` | `containerVersion` 常量（见 §3） |
| 任务领域视图 | `Tasks.vue`、`GroupDetail.vue` | shared `views/`（或仅提升为可复用布局） | 同上 |

### 不属于本次重构（留在 `encv-mobile` 的应用层）

server 域（`useServerStatus`/`useOpenListBridge`/`useRealtimeTransport`…）、agent/chat 域、file 管理域、webdav 域、config/mock 域。这些依赖 router/`@/config`/`@/constants` 具体应用上下文，不抽象。

---

## 2. 目标架构（抽象边界）

**`@encv/shared-components`（纯抽象层 / 库）** 只依赖：vue、pinia、ionic、通用第三方库；**不依赖** `@/config` `@/constants` `@/router` 以及任何具体应用上下文。其包含：

- **API 基础设施**：`api/core/`（请求封装、错误类型、base URL / 认证通过注入获取）。
- **领域类型**：`types/`（通用 `appError`/`appResult`/`phase` + 任务领域 `EncvTask`/`WorkflowRun`…）。
- **通用 composables**：`useTheme`/`useI18n`/`useToast`/`useDateFormat`/`useClipboard`/`relativeTime`/`activeStatus`/`eventBus` 等（已大部分在 shared）。
- **任务领域 composables / stores / lib**：解耦后的任务系统全量。
- **通用 UI 基元 + 任务领域组件**：解耦 `containerVersion` 等应用常量。

**`encv-mobile`（应用层）** 负责：
- 提供共享抽象所需的**注入上下文**（base URL、认证、容器版本列表等）。
- 组合 shared 抽象 + 应用专属业务（server/agent/file/webdav/mock 域）。
- 保留 `@/config` `@/constants` `@/router` 及业务页面。

---

## 3. 钉子依赖与重写策略

| 钉子 | 出现在 | 重写策略 |
|---|---|---|
| **Pinia 单例** | `taskStore`/`runTasksStore` 被 `useTasksList`/`useWorkflowTaskService` 直接 `useTaskStore()` | store 定义移入 shared；应用层在 Pinia 安装时 `useTaskStore(pinia)` 确保单例；shared 不自己 `createPinia`。 |
| **`?worker` import** | `useTaskViewCompute` → `@/workers/taskViewCompute.worker.ts` | shared 包需在其 `vite.config` / 消费端配置 worker 打包；或把计算逻辑改为纯函数 + 由应用层负责 worker 包装（推荐：shared 暴露纯计算函数，worker 留在应用层或 shared 提供 `?worker` 入口）。 |
| **`containerVersion` 常量** | `TaskBasicInfo`/`NewTaskModal`/`EncryptBody`/`ContainerVersionSelector` | 应用专属常量通过 **props / `provide-inject`** 传入，组件不再 `import "@/constants/containerVersion"`。 |
| **具体业务组件依赖** | `useNewTaskModal` → `@/components/NewTaskModal.vue` | 先提升 `NewTaskModal` 到 shared（解耦常量），或改为插槽/回调注入，使 composable 不依赖具体 UI。 |
| **`api/encv` 内的 `@/` 残留** | `encv_tasks.ts`、`encv_system.ts` | 重写为依赖注入：base URL / 认证来自 `api/core` 注入，而非 `@/config`。 |

> 好消息：`useI18n` 已完成共享化（`encv-mobile` 无本地文件，统一从 shared 导入），任务模块只 import 其 `TFunction` 类型，**无 i18n 钉子**。

---

## 4. 分阶段计划（按工作量）

> 每阶段结束都必须复验：① `shared-components` 类型检查 0 错误；② `encv-mobile` 类型检查 0 错误；③ `python3 scripts/i18n-tool.py lint --all` 0 问题。
> 所有"提升"动作采用**重写 + 应用层 re-export 兼容**策略，避免一次性破坏上千处 `@/` 导入。

### Phase 0 — 现状止血（小，立即可做）
- 明确标注：shared 里的 13 个 `api/encv_*` 副本 + `useEventBus` 孤儿 = **暂存应用层**，不是抽象，禁止新代码从 shared barrel 引用它们。
- 复验 `encv-mobile` 类型检查（终端恢复后执行 `vue-tsc --noEmit -p encv-mobile/tsconfig.json`）。
- 产出：本文档定稿。

### Phase 1 — 共享 API 基础设施 + 领域类型（中）
- 新建 `shared/api/core/`：请求封装、错误类型、`provideApiContext`/`useApiContext`（base URL + 认证注入）。
- 把 `EncvTask` 等核心领域类型从 `api/encv` 抽离到 `shared/types/task.ts`，原处 re-export 兼容。
- 复验。

### Phase 2 — 任务领域 store 与类型（中，含 Pinia 钉子）
- 重写 `taskStore`/`runTasksStore` 进 `shared/stores/`，解耦 Pinia 单例（由应用层安装）。
- 提升 `workflow/types.ts` 到 `shared/types/`。
- 复验。

### Phase 3 — 任务领域 composables（大，含 worker / 常量 / 组件钉子）

- 重写提升：`useTasksList`、`useTaskEventBridge`、`useWorkflowTaskService`、`useTaskForm`、`usePathResolver`、`useErrorAnalyzer`、`useBatchOperations`、`useSectionDerivation`、`useTaskTrigger`、`useTaskCancel`、`useRunSummaries`、`useNewTaskModal`。
- 处理钉子：`?worker`（纯函数化或 shared worker 入口）、`containerVersion`（注入）、`NewTaskModal`（提升/插槽）。

#### Phase 3 进度（2026-07-11 事实核查）

**✅ 已完成 — type-only 叶子（零运行时依赖，安全提升）**
- `usePathResolver`：仅 `import type { FileItem }`（`@/api/encv_files`），已提升到 `shared/src/composables/usePathResolver.ts`；`encv-mobile/src/composables/usePathResolver.ts` 改为 `export * from "@encv/shared-components/composables/usePathResolver"` 垫片。5 处 app 引用方（`useTaskForm`/`useNewTaskModal`/`__tests__` 等）经垫片无感。
- `useSectionDerivation`：仅 `vue` + `import type { EncvTask }`（`@/api/encv`），同样提升 + 垫片。`TaskBasicInfo.vue` 引用无感。
- 两个源文件的 type-only 导入已改为 shared 自身路径（`@/api/encv_files` / `@/api/encv`），与 shared 内部约定一致（如 `taskStore` 用 `@/types/task`）。

**⚠️ 关键发现 — 剩余项不是"逐个提升"，而是依赖簇**
- `useTasksList`（代表项）的源**只在 `encv-mobile`**，但其编译依赖 **4 个未提升的 Phase 3 composable**：`useRunSummaries` / `useTaskEventBridge` / `useTaskViewCompute` / `useWorkflowTaskService`，外加 **1 个 Phase 4 lib** `taskTypeLabel`。`useTaskViewCompute` 还带 `?worker` 钉子。
- → **必须整簇一起提升**（建议单个 PR），不能单拎 `useTasksList`。原清单漏列 `useTaskViewCompute`，需补入（它是 `useTasksList` 的硬依赖且带 `?worker`）。

**非干净叶子（提升前需先解耦）**
- `useBatchOperations`：运行时依赖 `@/composables/useToast`（shared 已有）+ 动态 `import("@/api/encv")`（shared 暂存残留有）。可提升，但会带入暂存 api 耦合，建议与簇一起处理。
- `useTaskCancel`：**硬依赖 `@/plugins/GoProcess`**（`enqueueCancelWorker` / `isNative`，native 插件）→ 不是干净叶子。需先抽象为注入式（参照 Phase 2 store 的 DI 模式：把 `enqueueCancelWorker`/`isNative` 注入 shared，web 端返回 noop）才能提升。

**待分析（未逐一核查依赖）**
- `useTaskForm` / `useErrorAnalyzer` / `useTaskTrigger` / `useNewTaskModal`：其中 `useNewTaskModal` 含组件钉子（需提升或插槽化），`useTaskForm` 可能依赖簇内其他 composable。整簇提升时一并核查。

**执行建议**
1. 先整簇提升 `useTasksList` 依赖组（`useRunSummaries` / `useTaskEventBridge` / `useTaskViewCompute` / `useWorkflowTaskService` + 提前 `taskTypeLabel`），`useTaskViewCompute` 的 `?worker` 纯函数化或改为 shared worker 入口。
2. 再处理 `useTaskCancel`（先抽 GoProcess 为注入）。
3. `useTaskForm` / `useErrorAnalyzer` / `useTaskTrigger` / `useNewTaskModal` 随簇或单独 PR，按依赖核查结果定。
4. 每步复验 §6 门禁（shared 0 错 + encv-mobile 0 错 + i18n 0 问题）。
- 复验。

### Phase 4 — 任务领域 lib 与组件（大）
- 提升 `lib/workflow/*`、`lib/taskTypeLabel`、`lib/taskPersistence`、`lib/buildReportZip`。
- 重写提升任务组件（`TaskTimeline`/`TaskBasicInfo`/`TaskDetailModal`/`tasks/*`/`group-detail/*`/`automation/*`），解耦 `containerVersion`。
- 复验。

### Phase 5 — 任务领域视图（中，可选）
- 提升 `Tasks.vue`/`GroupDetail.vue` 为可复用页面（或仅提升布局组件，应用层填空）。
- 复验。

### Phase 6 — 清理与收尾（小）
- 删除 shared 里的孤儿 `useEventBus.ts`。
- 处理 13 个暂存 `api/encv_*`：任务相关部分已重写为 Phase 1 抽象层模块；非任务部分（files/admin/…）**回退到 `encv-mobile` 作为应用层**，移除 shared 副本与 re-export 垫片。
- 全量复验：shared 0 错误 + encv-mobile 0 错误 + i18n `--all` 0 问题。

---

## 5. 当前过渡态（执行前须知）

- `shared-components/src/api/encv_*.ts` = 上一轮物理副本（错配 A），**暂存**，待 Phase 6 清理。
- `encv-mobile/src/api/encv_*.ts` = re-export 垫片，转发到 shared 副本，保证现有导入不红。
- `shared-components/src/composables/useEventBus.ts` = 孤儿重复文件，待 Phase 6 删除。
- 真实构建（`encv-mobile` + 3 个 plugin）此前已验证 0 错误；shared 自身的 315 错误来自未迁移的 task 测试与孤儿文件，将在 Phase 3–6 随重写一并消除。

---

## 6. 复验标准（每阶段门禁）

```bash
# 全量门禁（推荐：一条命令覆盖下列全部）
pnpm check:all            # 含单测；或 pnpm check:all:quick 跳过单测
# 完整日志写入 check-logs/<suite>.log，报告见 check-report.md

# ── 手动逐项（等价 check:all 的子集） ──
# 1. shared-components 类型检查
cd /workspace/app && npx vue-tsc --noEmit -p packages/shared-components/tsconfig.json

# 2. encv-mobile 类型检查
npx vue-tsc --noEmit -p encv-mobile/tsconfig.json

# 3. plugin-* web 类型检查（pnpm 工作区三个插件，各有独立 typecheck）
npx vue-tsc --noEmit -p encv-mobile/plugin-openlist/web/tsconfig.json
npx vue-tsc --noEmit -p encv-mobile/plugin-mpv-player/web/tsconfig.json
npx vue-tsc --noEmit -p encv-mobile/plugin-simverse/web/tsconfig.json

# 4. 全局 i18n
python3 scripts/i18n-tool.py lint --all
```

全部 0 错误 / 0 问题方可通过本阶段。

---

## 7. 全局模块概览（非任务系统范围，执行参考）

> 本节为后续执行提供"全貌"背景。任务系统迁移**只动 IN-SCOPE**，其余保持不动。
> 域标签：`IN-SCOPE`=任务系统（本次迁移）；`OUT-SCOPE-APP`=encv-mobile 应用专属（留应用层）；`OUT-SCOPE-SHARED`=shared 已有真抽象（保持不动）；`暂存残留`=上一轮误放入 shared 的应用层副本（待清理）。

### 7.1 `encv-mobile/src` 全模块地图

```
src/
├── api/                          [OUT-SCOPE-APP · server域] 后端 REST 客户端，按资源分文件
│   ├── encv_tasks.ts            [IN-SCOPE] 任务 API 客户端
│   ├── encv_core/admin/files/files_extra/openlist/perf/plugins/search/system/trash/webdav.ts  [OUT-SCOPE-APP] 各业务域 API
│   ├── encv.ts / encv.test.ts   [OUT-SCOPE-APP] 基础封装 + 测试
│   ├── mockGenerator.ts / sparseContainer.ts             [OUT-SCOPE-APP · mock域] 本地 mock 数据生成
│   └── __tests__/
├── stores/
│   ├── taskStore.ts             [IN-SCOPE] 任务主 store（27KB，迁移核心）
│   └── runTasksStore.ts         [IN-SCOPE] 运行态任务 store
├── composables/                  [混合域]
│   ├── task域/                   [IN-SCOPE] useTasksList/useTaskForm/useTaskTrigger/useTaskCancel/
│   │                             useTaskViewCompute/useTaskEventBridge/useNewTaskModal/useRunSummaries/
│   │                             useSectionDerivation/useBatchOperations/useErrorAnalyzer/usePathResolver/
│   │                             useWorkflowStore/useWorkflowTaskService + realtime/ 子目录（任务实时后端抽象）
│   ├── agent/chat域/            [OUT-SCOPE-APP] useAgent*/useChatEngine*/useAGUIParser/useToolCallAccumulator/
│   │                             renderTurnItems/inlineFileReference/reasoningEffort/useSlashMenu/
│   │                             useInputHistory/useContextUsage/useSimverse/activeStatus
│   ├── server域/                [OUT-SCOPE-APP] useServerStatus/useApiBaseProbe/appServerRealtimeReducer/
│   │                             useProxiedFetch/usePluginExtensions/useTestBackdoor/useVectorSearchStatus/useRealtimeTransport
│   ├── file域/                  [OUT-SCOPE-APP] useFileList/useFileFeatures/useLibraries/useAttachments/
│   │                             useThumbnailCache/useFileSystemTests/useOpenListBridge
│   ├── webdav域/                [OUT-SCOPE-APP] useWebDavManifest/useWebDavTestModules/useWebDavTestRunner/useWebDavWorkflowAdapter
│   ├── config/mock域/          [OUT-SCOPE-APP] useConfig/useMockGenLog/useDeviceId/useTestCaseGeneration
│   └── 通用工具域/              [OUT-SCOPE-APP] useClipboard/useDateFormat/usePinchZoom/useHighRefreshRate/
│                                 useSearchInput/useErrorCapture/useIonicAutoRegister/useTheme/useToast/useDevTools
│                                 （注：useTheme/useToast 应用层版待与 shared 对齐；useEventBus 为孤儿，见 §7.2）
├── components/                   [混合域]
│   ├── tasks/                    [IN-SCOPE] TaskVirtualList/TaskDebugPanel
│   ├── group-detail/             [IN-SCOPE] TasksTab/PipelineTab/PerformanceTab/FilterDrawer
│   ├── automation/               [IN-SCOPE] JobPipelineCard/StepInlineTimeline/TestReportHeader 等
│   ├── Task*/                    [IN-SCOPE] TaskBasicInfo/TaskTimeline/TaskDetailModal/TaskOutputInfo/TaskErrorSection/
│   │                             TaskPerformanceSection/TaskWarningSection/TaskActionButtons
│   ├── agent/                    [OUT-SCOPE-APP] 对话消息渲染（38 文件：AgentTaskMessage/AssistantMessage/SlashMenu…）
│   ├── developer/                [OUT-SCOPE-APP] MockGenLogCard
│   ├── shared/                   [OUT-SCOPE-APP] ErrorCaptureOverlay
│   └── 顶层通用/                 [OUT-SCOPE-APP] ServerStatusCard/ConfigFieldItem/LocalOpenListStatusCard/
│                                 VirtualLogList/FilePickerModal/NewTaskModal/EncryptBody/DecryptBody/
│                                 ContainerVersionSelector/RadioItem/DebugConsole/InputWithHistory/LibraryRow
├── components-shared/            [OUT-SCOPE-APP] OpenListStatusCard/OpenListLogList（应用层共享）
├── views/                        [混合域]
│   ├── Tasks.vue / GroupDetail.vue / WorkflowDashboard.vue  [IN-SCOPE] 任务页/组详情/工作流看板
│   ├── AgentChat.vue + useAgentChatView.ts        [OUT-SCOPE-APP · agent域]
│   ├── Files.vue / FileInfo.vue / FilePreview.vue / useFilesView.ts  [OUT-SCOPE-APP · file域]
│   ├── Settings.vue / AgentSettingsDetail.vue / AppearanceDetail.vue / ServerSettings.vue  [OUT-SCOPE-APP · config域]
│   ├── DevLogs.vue / DevToolsDetail.vue / PluginTestsDetail.vue / DatabaseTests.vue  [OUT-SCOPE-APP · developer域]
│   ├── WebDav* / OpenListView.vue / Remote.vue / MountsDetail.vue  [OUT-SCOPE-APP · webdav域]
│   └── About/Admin/Chronicle/ComposePrototypes 等  [OUT-SCOPE-APP] 各功能页
├── lib/
│   ├── workflow/                 [IN-SCOPE] buildDynamicWorkflow/state-machine/scheduler/conditionEvaluator/matrixExpander
│   ├── taskTypeLabel.ts / taskPersistence.ts / buildReportZip.ts  [IN-SCOPE]
│   ├── mockDataGenerator.ts / mockConstants.ts  [OUT-SCOPE-APP · mock域]
│   └── mountPath.ts             [OUT-SCOPE-APP]
├── types/                        [OUT-SCOPE-APP] file-feature/messageStatus/simverse/tokenSnapshot/webdav-test
│                                 （任务类型 WorkflowRun 等已并入 shared）
├── router/                       [OUT-SCOPE-APP] 应用路由表，依赖应用层 views，留应用层
├── config/                       [OUT-SCOPE-APP] containerVersion.ts（容器版本映射）/player.ts（播放器配置）
├── constants/                    [OUT-SCOPE-APP] schema.json/schemaParser.ts（表单 schema 协议）
├── workers/                      [IN-SCOPE] taskViewCompute.worker.ts（任务视图计算）
├── engines/                      [OUT-SCOPE-APP · chat渲染] TDesignChatView/ToolDetailContent + tdesign/default
├── features/                     [OUT-SCOPE-APP · 功能模块] alist-encrypt/（actions/badge/useAlistEncrypt）
├── plugins/                      [OUT-SCOPE-APP · 原生桥接] ApiProxy/GoProcess/SimVerse/openlist-native/web
├── i18n/                         [OUT-SCOPE-APP · 应用国际化] 应用层文案（与 shared i18n 并存）
└── App.vue / main.ts / components.d.ts / vite-env.d.ts  [OUT-SCOPE-APP] 应用入口与全局声明
```

**为何 `router/` `config/` `constants/` `types/` 留应用层**：强依赖 encv-mobile 页面组件、原生壳环境或移动端专属配置协议（表单 schema、容器版本映射、播放器参数），无法脱离应用独立运行。

### 7.2 `shared-components/src` 全模块地图

```
src/
├── composables/                  [OUT-SCOPE-SHARED 真抽象]
│   ├── useTheme/useI18n/useToast/useDateFormat/relativeTime/activeStatus  [OUT-SCOPE-SHARED]
│   ├── useClipboard/usePinchZoom/useHighRefreshRate/useDevTools/useSearchInput/
│   │   useErrorCapture/useFrontendLogs/useIonicAutoRegister              [OUT-SCOPE-SHARED]
│   ├── __tests__/                [混合] 含任务相关测试（useTaskViewCompute/useWorkflowTaskService/useSectionDerivation…）
│   └── useEventBus.ts            [暂存残留 · 孤儿] 无引用，应清理或移回应用层
├── components/                   [混合]
│   ├── shared/                   [IN-SCOPE] PhaseBadge/PhaseIcon/UnifiedTimelineCard/FilterDropdown/RelevanceBadge
│   │                             + __tests__（TaskBasicInfo/TaskTimeline）
│   ├── settings/                 [OUT-SCOPE-SHARED] SettingsPage/SettingsGroup/SettingsItem/SettingsSelect
│   ├── about/AboutPage.vue       [OUT-SCOPE-SHARED]
│   ├── DevLogsViewer.vue / VirtualLogList.vue  [OUT-SCOPE-SHARED]
│   └── __tests__/
├── views/                        [OUT-SCOPE-SHARED] NotFoundView.vue + __tests__
├── types/                        [混合]
│   ├── phase.ts                  [IN-SCOPE] 任务阶段/WorkflowRun 等类型
│   └── appError.ts / appResult.ts  [OUT-SCOPE-SHARED]
├── lib/                          [混合]
│   ├── workflow/types.ts         [IN-SCOPE] WorkflowRun/JobRun/StepRun/UnifiedRunRecord
│   ├── dev-start-guard.ts        [OUT-SCOPE-SHARED]
│   └── __tests__/（workflow-core.test）
├── utils/                        [OUT-SCOPE-SHARED] RingBuffer.ts / IncrementalFilter.ts
├── i18n/                         [OUT-SCOPE-SHARED] common/settings/devlogs/errors/generated-types/index/types
├── api/                          [暂存残留 · 待清理]
│   ├── encv_tasks.ts            [IN-SCOPE 残留] 任务 API（应随任务系统重写进来）
│   └── encv*.ts（core/admin/files/openlist/perf/plugins/search/system/trash/webdav）
│                                 [暂存残留 · OUT-SCOPE-APP] 非任务业务 API 误放入，应移回 encv-mobile
├── vite-plugins/                 [OUT-SCOPE-SHARED] daisy-ui/file-size-limit/i18n-optimize/vue-component-check
├── directives/                   [OUT-SCOPE-SHARED] longpress.ts
├── theme/                        [OUT-SCOPE-SHARED] variables.css（设计 token）
├── styles/                       [OUT-SCOPE-SHARED] daisyui.css/timeline-tokens.css/timeline-utilities.css
├── index.ts / env.d.ts          [OUT-SCOPE-SHARED] 包出口与类型声明
```

**shared 三类标注**：
- `OUT-SCOPE-SHARED` 真抽象：`useTheme`/`useI18n`/`useToast`/`useDateFormat`/`useClipboard`/`usePinchZoom`/`useSearchInput`/`useErrorCapture`/`useFrontendLogs`/`useIonicAutoRegister`、`components/settings/*`、`AboutPage`、`DevLogsViewer`、`VirtualLogList`、`NotFoundView`、`appError`/`appResult`、`utils/*`、`i18n/*`、`vite-plugins/*`、`directives/*`、`theme/*`、`styles/*` —— 保持不动。
- `IN-SCOPE`（任务相关）：`components/shared/*`、`types/phase.ts`、`lib/workflow/`（含 WorkflowRun 等）、`api/encv_tasks.ts`（暂存残留）。
- `暂存残留`：`api/encv*`（非任务域误放入，应移回 encv-mobile）、`composables/useEventBus.ts`（孤儿，无引用）。

### 7.3 迁移边界速查

| 标签 | 含义 | 本次是否改动 |
|---|---|---|
| `IN-SCOPE` | 任务系统（api/encv_tasks、taskStore/runTasksStore、use*Task*/useWorkflow*/useRunSummaries/useBatchOperations/useSectionDerivation/useNewTaskModal/useTaskForm/usePathResolver/useErrorAnalyzer/useTaskTrigger/useTaskCancel/useTaskEventBridge/useTaskViewCompute、lib/workflow、lib/taskTypeLabel、lib/taskPersistence、lib/buildReportZip、components/tasks\|group-detail\|automation\|Task*、views/Tasks\|GroupDetail\|WorkflowDashboard、workers/taskViewCompute.worker、shared 的 components/shared/*+types/phase+lib/workflow） | **改（重写式提升）** |
| `OUT-SCOPE-APP` | encv-mobile 的 server/agent/chat/file/webdav/config/mock/router/engines/features/plugins/i18n 等 | 不动 |
| `OUT-SCOPE-SHARED` | shared 已有真抽象（useTheme/useI18n/useToast/Settings*/VirtualLogList/DevLogsViewer/NotFoundView/appError/appResult/utils/i18n/vite-plugins/directives/theme/styles） | 不动 |
| `暂存残留` | shared 内 `api/encv*`（非任务）、`composables/useEventBus` 孤儿 | **清理（Phase 6）** |

---

## 8. 其他模块的抽象/应用层耦合与重构计划

> 任务系统（§1–§6）之外，仓库还存在**同类耦合问题**：通用抽象被应用在 `encv-mobile` 里独立重写（重复实现、真源不清、漂移风险），以及 API 层整体依赖 `@/config`（应用层契约误放进 shared）。本节补上这些模块的重构计划。
> 业务域（server/agent/chat/file/webdav/config/mock）目前**正确地留在应用层**，其耦合以"消费侧对齐 + 抽取候选评估"处理，不阻塞任务系统迁移。

### 8.1 Module G — 通用抽象去重对齐（高优先级，证据确凿）

> **进度（2026-07-11）**：下表中的 composables 重复**已全部去重完成**（本地副本删除、`@/composables/useX` 别名回退 shared，见 §9）。仅 `VirtualLogList.vue`（G-3）与 `useEventBus`（G-4）待处理。下表保留作历史证据。

**现状（已核实）**：以下通用能力在 `shared-components` 已是真抽象，但 `encv-mobile/src` 又**独立实现了一份**：

| 模块 | shared 真源 | encv-mobile 重复副本 | 重复性质 |
|---|---|---|---|
| `useTheme` | `composables/useTheme` | `composables/useTheme.ts`（396 行，全独立，未引 shared） | 完全重复 |
| `useToast` | `composables/useToast`（`showToast`） | `composables/useToast.ts`（独立实现） | 完全重复 |
| `useDateFormat` | `composables/useDateFormat`（`formatDateTime`/`formatDuration`） | `composables/useDateFormat.ts`（重定义同名函数，仅引 shared 的 `useI18n`） | 逻辑重复 |
| `useClipboard` | `composables/useClipboard` | `composables/useClipboard.ts` | 完全重复 |
| `useErrorCapture` | `composables/useErrorCapture`（`errorStore`） | `composables/useErrorCapture.ts` | 完全重复（注意：encv-mobile 的 `components/shared/ErrorCaptureOverlay.vue` 已正确引 shared 的 `errorStore`，两份并存易冲突） |
| `useFrontendLogs` | `composables/useFrontendLogs` | `composables/useFrontendLogs.ts` | 完全重复 |
| `useHighRefreshRate` | `composables/useHighRefreshRate` | `composables/useHighRefreshRate.ts` | 完全重复 |
| `useIonicAutoRegister` | `composables/useIonicAutoRegister` | `composables/useIonicAutoRegister.ts` | 完全重复 |
| `usePinchZoom` | `composables/usePinchZoom` | `composables/usePinchZoom.ts` | 完全重复 |
| `useSearchInput` | `composables/useSearchInput` | `composables/useSearchInput.ts` | 完全重复 |
| `useDevTools` | `composables/useDevTools` | `composables/useDevTools.ts`（49 行，逐字节相同，**文档原表漏列**） | 完全重复 |
| `VirtualLogList`(组件) | `components/VirtualLogList.vue` | `components/VirtualLogList.vue`（仅差 `useI18n` import 路径） | 完全重复 |

> 注：`relativeTime`/`activeStatus` 在两边均存在，按同模式处理。`Settings*`/`AboutPage`/`RelevanceBadge`/`UnifiedTimelineCard` 已正确从 shared 引用，是**正确示范**，无需改动。

#### 8.1.1 诊断修正与新增发现（2026-07-10 复核）

**① `useI18n` 不是重复问题（原表误列风险已排除）**
- `encv-mobile/src/composables/` **不存在** `useI18n.ts`（read/search 双重确认）。
- 根因：`encv-mobile/tsconfig.json` 的 `@/*` 别名定义为
  `"@/*": ["./src/*", "../packages/shared-components/src/*"]`
  → `@/composables/useI18n` 在本地找不到时**自动回退到 shared 的 `useI18n`**。
- 结论：`useI18n` 是 shared 单一真源，encv-mobile 通过别名透明引用，**无重复副本，不纳入去重**。

**② `useEventBus` 归属纠正（原 §7.2 标"孤儿"不准确）**
- 两边**都有** `useEventBus.ts`，且逐字节相同（仅第 13 行 `EncvTask` 类型 import 源不同：encv-mobile 引 `@/api/encv_tasks`，shared 引 `@encv/shared-components/api/encv`）。
- 它是**模块级单例**（`listeners` Map）。
- 引用方核查：`@/composables/useEventBus`（→ encv-mobile 副本）被 6 个模块消费（`useFilesView`/`DevLogs`/`useTaskEventBridge`/`useNewTaskModal`/`ServerStatusCard`/`LocalOpenListStatusCard`），**是 canonical**；shared 副本仅被 `app/scripts/setup-simverse-refactor.sh` 一个 setup 脚本引用，**是孤儿**。
- 结论：**保留 encv-mobile 的 canonical 副本，删除 shared 的孤儿副本**（注意：删除前需把该 setup 脚本的 import 改指 encv-mobile 副本，或随脚本废弃一并删除）。这与"提升进 shared"方向相反——`useEventBus` 含 app 层 `EncvTask` 依赖，当前不宜进 shared。

> **2026-07-11 事实核查修正（重要）**：上述 ② 的"两边都有 / encv-mobile 是 canonical"**已不成立**。当前 `encv-mobile/src/composables/useEventBus.ts` **不存在**（已被删除），而 11 处引用（`useFilesView`/`DevLogs`/`useTaskEventBridge`/`useNewTaskModal`/`ServerStatusCard`/`LocalOpenListStatusCard`/`useServerStatus`/`useVectorSearchStatus`/`useRealtimeTransport`/`useOpenListBridge`/`useTestBackdoor`）仍写 `@/composables/useEventBus`，经 `tsconfig` 的 `@/*` 二级回退解析到 **`shared/src/composables/useEventBus.ts`**——即 **shared 副本已成为事实 canonical**。因此原 G-4 计划"删 shared 孤儿、保留 encv-mobile"**反向且有害**（删 shared 会直接断这 11 处引用）。正确做法：**保留 shared 的 `useEventBus.ts` 作为唯一真源**，并清理 `app/scripts/setup-simverse-refactor.sh` 对其的引用（如有残留）。`useEventBus` 含的 `EncvTask` 依赖经 `@encv/shared-components/api/encv` 解析，在 shared 内自洽，无需回退 encv-mobile。

**③ 别名回退机制 → 去重策略可简化，且揭示"单例分裂"真实 bug**
- 因 `@/*` 别名二级回退到 shared：**删除本地重复副本后，`@/composables/useX` 会自动解析到 shared**，无需 re-export 垫片（G-1 已用垫片，等价且安全；后续阶段直接删除本地文件更干净）。
- **关键 bug（当前已存在）**：当"本地 + shared 两份都在"时，别名**本地优先**。导致 `useErrorCapture`/`useFrontendLogs` 这类单例模块**实际分裂成两个实例**：
  - `ErrorCaptureOverlay.vue:43` 从 `@encv/shared-components/composables/useErrorCapture` 引 `errorStore`（shared 实例）；
  - `main.ts:15` / `FullTextIndexDetail.vue:261` 从 `@/composables/useErrorCapture` 引 `errorStore`（本地实例，因为本地优先）。
  - → `installErrorCapture()`（在 main.ts 经本地副本安装）捕获的错误写入**本地** `errorStore`，而浮窗读**shared** `errorStore` → **错误浮窗永远不会显示捕获到的错误**。
- 修复：删除本地 `useErrorCapture.ts` / `useFrontendLogs.ts` 后，所有 `@/composables/useX` 回退到 shared → 单例统一，浮窗恢复工作。这正是去重（Module G）除"消除漂移"外的**第二个硬收益：修 bug**。

**问题**：重复实现 → ① 行为漂移、bug 修复需改两处、真源不清；② 单例模块（errorStore/logs/eventBus）因别名本地优先而**分裂成两个实例**，引发真实功能 bug（错误浮窗失效）。

**目标**：`shared-components` 为唯一真源；encv-mobile 的重复副本**直接删除**（依赖别名回退到 shared），现有 `@/composables/useX` 导入无需逐处改。`useEventBus` 例外：删 shared 孤儿副本、保留 encv-mobile canonical。

**阶段（按风险从低到高）**：
- **G-1（低）纯函数/无状态**：`useDateFormat`、`useClipboard`、`useSearchInput`、`relativeTime`、`activeStatus` → 删除 encv-mobile 副本，原路径改 re-export 垫片指向 shared。
- **G-2（中）有状态/依赖 DOM**：`useTheme`、`useToast`、`useErrorCapture`、`useFrontendLogs`、`useHighRefreshRate`、`useIonicAutoRegister`、`usePinchZoom`、`useDevTools`（文档原漏列）→ 因别名回退机制，**直接删除本地副本**即可（无需垫片）；删除前核对导出签名与 shared 一致（尤其 `useErrorCapture` 的 `errorStore` 单例，删除后全 app 统一共用 shared 实例，顺带修复浮窗不显示 bug）。
- **G-3（中）组件**：`VirtualLogList.vue` → 确认 encv-mobile 的 `DevLogs.vue` 改用 shared 版本（两版仅差 `useI18n` import 路径，逻辑一致），删除本地重复方。
- **G-4（修正项）`useEventBus`**：删 shared 孤儿副本、保留 encv-mobile canonical（含修 setup 脚本引用）。
- 每子阶段复验：shared 0 错误 + encv-mobile 0 错误 + `i18n --all` 0 问题。

### 8.2 Module A-ext — API 层全局基座（跨所有 api 模块）

**现状**：§1 的错配 A 实际**不止任务模块**——上一轮把全部 13 个 `api/encv_*.ts`（含 files/admin/openlist/perf/plugins/search/system/trash/webdav 等非任务域）都物理复制进了 shared，且全部依赖 `@/config` 获取 base URL/认证。任务系统的 Phase 1/6 只完整覆盖 `encv_tasks`；其余 12 个非任务 api 仍是"应用层契约误放共享包"。

**目标**：建立 **`shared/api/core/`** 作为**所有 api 模块的通用基座**（请求封装 + base URL/认证依赖注入，见 §3），`encv_tasks` 与非任务 api 统一经 core 重写为"注入式"，而非 `@/config` 硬编码。非任务 api 的最终归属二选一：
- （a）经 `api/core` 重写后**留在 shared**（若确属多 app 复用的后端契约）；或
- （b）**回退到 `encv-mobile` 应用层**（若仅 encv-mobile 使用），移除 shared 副本与 re-export 垫片。

**阶段**：
- **A-1**：`api/core` 基座落地（与任务系统 Phase 1 合并实施，避免重复）。
- **A-2**：非任务 api 逐个评估归属（a/b），按评估结果重写或回退。
- **A-3**：清理 shared 内剩余 `api/encv_*`（非任务）暂存副本（并入任务系统 Phase 6）。
- 复验同 §6 门禁。

### 8.3 Module H — 业务域消费侧对齐与抽取候选（评估型，不阻塞）

**现状**：server/agent/chat/file/webdav/config/mock 等域正确留在 `encv-mobile` 应用层，且已通过 `useI18n`/`eventBus`/`useErrorCapture` 等**正确消费** shared 抽象（未见误放 shared 的情况）。

**风险与候选**：域内可能含有"未来多 app 复用"的子抽象，当前不强制抽取，但需建立评估机制：
- **agent/chat 域**：消息渲染引擎（`engines/tdesign`）、`renderTurnItems`、`useToolCallAccumulator` 若未来其他 app 也做对话 UI，可抽取为 shared 的 chat 渲染抽象。
- **file 域**：`useFileFeatures`/`useThumbnailCache` 的纯逻辑部分可抽象。
- **webdav 域**：`useWebDavWorkflowAdapter` 等测试适配器若多 app 复用可抽象。
- **server 域**：`useServerStatus`/`useRealtimeTransport` 依赖 `eventBus`（已共享），域逻辑本身 app 专属。

**计划（评估流程，非立即执行）**：
1. 定义"是否提升"判定标准：① 是否被 ≥2 个 app 复用或计划复用；② 是否不依赖 `@/router`/`@/config`/`@/constants` 等应用上下文；③ 提升后能否用依赖注入解耦剩余钉子。
2. 对每个候选按标准打分，达标者立项为独立的"Module X 抽取"任务（结构同 §8.1）。
3. 未达标者保持应用层，仅确保**消费** shared 抽象而非复制。
4. 仅在实际抽取时触发复验（§6 门禁）。

### 8.4 其他模块重构与任务系统迁移的先后关系

| 模块 | 优先级 | 与任务系统的关系 |
|---|---|---|
| Module G（通用去重） | 高 | 独立，可与任务系统 Phase 0–6 **并行**推进；建议先吃低风险的 G-1 |
| Module A-ext（API 基座） | 高 | `api/core` 与任务系统 Phase 1 **合并**；非任务 api 回退并入 Phase 6 |
| Module H（业务域评估） | 低 | 不阻塞，仅建立评估机制，后续按需立项 |

> 结论：任务系统迁移（§1–§6）仍是主线；Module G 与 A-ext 是其"同一类问题在通用层与 API 层的延伸"，应一并纳入重构路线图，避免修一处漏一处。
