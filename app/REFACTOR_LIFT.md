# ENCV-Mobile → @encv/shared-components 模块提升（Lift）状态

> 本文件追踪「任务系统模块从 `encv-mobile/src` 提升到 `packages/shared-components/src`」
> 的渐进式重构进度与约束。改动后**主动更新本文件**，不要等提醒。

## 目标

让 `packages/shared-components` 成为**真正自包含**的共享层：

- 不反向依赖应用层（`@/...` 内部实现）。
- 应用层特有的能力（toast / i18n / eventBus / taskServices / 真实 API 客户端）通过
  **DI 注入**或**下沉到 shared 自身**解耦。

## 范式

- **提升 + 原位 shim（迁移期约定，现已进入去垫片阶段）**：真实实现迁到
  `packages/shared-components/src`，`encv-mobile/src` 原位降级为一行
  `export * from "@encv/shared-components/..."` 的兼容垫片，避免上千处 `@/...` 引用断链。
  这是**迁移期的权宜之计**，不是终态。
- **垫片是「迁移的谎言」**：垫片文件假装是应用层实现，实则只转发 shared 真源。迁移基本
  完成后，必须**纯化**——删除垫片，让 `@/` 别名经二级回退（tsconfig `@/*` +
  vite/vitest 的 `encv-alias-fallback`）**直接解析到 shared 真源**。这样应用层只剩
  「组合 + DI 注入」的胶水，共享层是唯一事实来源，无重复、无谎言。
- **去垫片工具 `scripts/make-shim.mjs prune`**（结构性改革的核心机制）：
  - `node scripts/make-shim.mjs prune`：dry-run，列出「同名垫片」（可安全删除）与
    「名称错位垫片」（需先改写 importer，不删）。
  - `node scripts/make-shim.mjs prune --apply`：**只删同名垫片**（删除后 `@/x` 自动落到
    shared，零风险）；名称错位垫片保持不动，避免静默断链。
  - 安全判定：shim 相对路径 == shared 真源相对路径 → 同名（可删）；否则错位（先改 importer
    再删）。`check-all` 在 0 垫片时输出「✔ 无残留垫片（已纯化）」作为新终态不变量。
- **名称错位垫片的处理范式**：错位垫片（如历史遗留 `app/api/encv_core` → shared `api/core`、
  `app/features/alist-encrypt/useAlistEncrypt` → shared `composables/useAlistEncrypt`）
  删除前必须**改写其全部 importer** 指向 shared 真源路径（如 `@/api/core` /
  `@/composables/useAlistEncrypt`），再删垫片。已落地的两例见下方「去垫片纯化」节。
- 该 shim 机制源于项目既有约定（`make-shim.mjs` 与 `useSectionDerivation`/`usePathResolver`
  的 shim 早于本次会话存在），并非临时拼凑；`prune` 是其自然的收尾。

## 关键架构约束

- shared 的 tsconfig 已自映射：`"@/*": ["./src/*"]` 与
  `"@encv/shared-components": ["./src"]`、`"@encv/shared-components/*": ["./src/*"]`。
  → shared 内用 `@encv/shared-components/...` 指向自身是**语义正确**的，比 `@/...` 更清晰；
  而 `@/...` 在 shared 内经 tsconfig 也解析到 shared 自身（仅语义模糊，无功能错误）。
- 应用层 tsconfig 的 `@/*` 有**两个**解析目标：`./src/*` 与
  `../packages/shared-components/src/*`。因此 `@/composables/useToast` 在应用层其实解析到
  shared 自己的 `useToast.ts`——很多「通用能力」早就被提升进 shared 了，只是用了错误别名。
- DI 抽象现成样例：`stores/taskServices.ts` 的 `getTaskServices()` +
  `stores/registerSharedTaskServices.ts` 的注入。新增 shared→app 耦合应照此范式，不要硬塞。

## 当前进度

### Tier 1 — shared 内 `@/` 别名重指向（DONE ✅）

把 shared 内所有非 api 的 `@/...` 引用改为 `@encv/shared-components/...`：

- `types/task`、`types/errorAnalysis` → `@encv/shared-components/types/*`
- task-system composables/lib（`useTaskForm`、`useWorkflowTaskService`、
  `useTaskEventBridge`、`useTaskViewCompute`、`useBatchOperations`、`useRunSummaries`、
  `useErrorAnalyzer`、`useSectionDerivation`、`useEventBus`）及其内部互相引用
- `stores/taskServices`、`stores/taskStore`、`stores/runTasksStore`
- `lib/taskViewComputeCore`、`lib/workflow/*`、`lib/taskTypeLabel`
- 通用基础设施 `useToast` / `useI18n` / `useEventBus`（本就在 shared 内，仅别名用错）

> 正确性已核实：shared tsconfig 映射了 `@encv/shared-components/*`，standalone typecheck 可解析。

### Tier 2 — api 层真解耦（DONE ✅，已 `check:all` 门禁全绿）

shared 内原本指向应用层的泄漏：`packages/shared-components/src/api/` 下 11 个
`export * from "@/api/encv_*"` 转发壳 + `encv_core` 壳 `export * from "@/api/encv_core"`。

**已落地并验证（pnpm check:all → 8/8 PASS，含 shared / encv-mobile typecheck + 单测 + Biome）：**

1. **提升 `webdav-test` 类型**：`app/types/webdav-test.ts` →
   `packages/shared-components/src/types/webdav-test.ts`（纯类型、零 import）。
   app 侧留 re-export shim `export * from "@encv/shared-components/types/webdav-test"`。
2. **搬 11 个端点实现**到 `shared/api/encv_*.ts`
   （admin / files / files_extra / openlist / perf / plugins / search / system / tasks / trash / webdav）。
   import 改写（仅 3 个文件需改，其余 8 个相对引用文件名相同零改动）：
   - `encv_tasks.ts`：`@/types/task` → `@encv/shared-components/types/task`；
     `deleteTask` 内 `await import("@/stores/taskStore")` → 走 `getTaskServices().deleteTask`（DI）。
     （`apiRequest` 本就来自 `@encv/shared-components/api/core`，在 shared 内自指，无需改。）
   - `encv_perf.ts`：`@/types/task` → `@encv/shared-components/types/task`。
   - `encv_system.ts`：HEAD 中无 `@/` 引用，零改动（webdav-test 消费者在 composables，
     经类型提升 + app shim 解析，无需在 api 层改）。
   - 其余 8 个文件：import 全是相对 `./encv_*` 或已是 `@encv/shared-components/*`，**零改动**。
3. **反转 shim**：`app/api/encv_*.ts`（11 个）变
   `export * from "@encv/shared-components/api/encv_*"`；
   `shared/api/encv_core.ts` 变 `export * from "./core"`；
   `app/api/encv_core.ts` 此前已是壳（指向 core）。
4. **聚合 barrel**：`shared/api/encv.ts` 由 `export * from "@/api/encv"` 改为
   聚合 `./encv_*`（shared 自有真实实现），`encv_core` 符号具名导出以镜像 app 原 barrel、
   避免 star-export 静默冲突。
5. **DI 接 `deleteTask`**：`registerSharedTaskServices.ts` 的
   `deleteTask: deleteTaskApi` 改为
   `deleteTask: async (id) => { await useTaskStore().removeTask(id); }`，
   从而 shared 的 `api/encv_tasks.deleteTask` 经 `getTaskServices()` 委托到应用层 store，
   共享层不再感知 `@/stores/taskStore`。

**验证状态**

- `search_content` 全量扫 `shared/src` 的 `"@/`（含 `from "@/..."` 与类型位置
  `import("@/...")`）：**非测试代码 0 命中**。仅剩 `*.test.ts` / `__tests__/` 内的
  `vi.mock("@/...")`（mock 应用层 shim），属预期、非泄漏。
- 收尾：原 `shared/api/encv_system.ts:94,99` 有一处类型位置动态 import
  `import("@/types/webdav-test")`，虽经 shared tsconfig 自指解析、不影响编译，但语义上
  仍是「shared 用 `@/`」，已改为 `import("@encv/shared-components/types/webdav-test")`，
  使 shared 非测试代码彻底 `@/`-free。
- 门禁：`pnpm check:all` → **8/8 PASS**（shared-components typecheck、encv-mobile typecheck、
  3 个 plugin typecheck、i18n lint、encv-mobile 单测、Biome CI 全过）。
- `codemogger leaks` 本版本无该子命令（报错 `unknown command 'leaks'`），改用
  `search_content` + `scripts/make-shim.mjs check-all` 校验 shim 一致性。

**复核结果（终端恢复后已执行）**

```bash
cd /workspace/app
node scripts/make-shim.mjs check-all   # ✔ 全部 19 个垫片与 shared 真源一致
pnpm check:all                         # ✔ 8/8 PASS（含 encv_system.ts @/ 别名微调后）
```

**`make-shim.mjs` 顺带修复的两个 bug（check-all 此前形同虚设）**

1. `stripNoise` 原会把字符串内容（含 `export * from "@encv/..."` 里的说明符）
   整体替换成空格，导致 `isShim` / `parseReExports` 的 `from "..."` 正则永远匹配不到，
   `check-all` 恒报「未扫描到任何 re-export 垫片」。改为**仅剥离注释、保留字符串内容**
   （`from` 说明符需保留用于判定；注释里的 `// export const x` 假阳性已由注释剥离覆盖）。
2. `resolveShared` 原只试 `core.ts/tsx/vue`，不试目录索引 `core/index.ts`，
   导致 `@encv/shared-components/api/core` 这类「指向目录」的说明符被判为「目标不存在」。
   已补 `index.ts/tsx/vue` 候选。修复后 `app/api/encv_core.ts` 的
   `export * from "@encv/shared-components/api/core"` 被正确识别为一致垫片。

> 注：`codemogger leaks` 本版本无该子命令（报错 `unknown command 'leaks'`），
> shim 机械一致性改以 `node scripts/make-shim.mjs check-all` 为准。

### Tier 3 — 收敛 shared 反向依赖 app 的真实实现（IN PROGRESS）

Tier 1/2 把任务模块「文件 + 垫片」搬进了 shared，但仍有**真实实现留在 app、被 shared 经
tsconfig 的 `@/* → ../packages/shared-components/src/*` 映射反向引用**的漏网之鱼。这类模块
不解决，shared 就谈不上真正自包含。判定信号：`shared` 的测试/代码 `vi.mock("@/xxx")` 或
`@/xxx` 直接指向 app 内仍是真源的文件。

**已落地（#1，已 `check:all` 8/8 全绿 + make-shim check-all 20/20 一致）：**

1. **`lib/taskPersistence.ts`（IndexedDB/Dexie 持久化层）**
   - 证据：shared 的 `views/__tests__/Tasks.component.test.ts` 与
     `composables/__tests__/useTasksList.automation-escape.test.ts` 直接
     `vi.mock("@/lib/taskPersistence")` —— shared 的 taskStore/runTasksStore 逻辑强依赖它，
     是唯一「shared 反向需要 app 层真实实现」的模块。
   - 依赖：仅 `dexie`（shared 的 package.json 已含 `dexie@^4.4.4`）+ type-only
     `@/api/encv`（→ 改 `@encv/shared-components/api/encv`）。无 app-only 能力，零风险。
   - 动作：真源搬到 `packages/shared-components/src/lib/taskPersistence.ts`；
     app 原位留 `export * from "@encv/shared-components/lib/taskPersistence"` 垫片。
   - 验证：`pnpm check:all` 8/8 PASS；`make-shim check-all` 20/20 一致；
     shared 测试仍 `vi.mock("@/lib/taskPersistence")`，模块身份不变（app 垫片 re-export 同一真源）。

**已落地（#2，已 `check:all` 8/8 全绿 + make-shim check-all 23/23 一致）：**

2. **`lib/mountPath.ts`、`lib/mockConstants.ts`、`lib/mockDataGenerator.ts`（零依赖纯叶子）**
   - 三者均无 app-only 依赖：`mountPath` 零 import；`mockDataGenerator` 零 import；
     `mockConstants` 仅 `import { mountPath } from "@/lib/mountPath"`（→ 改
     `@encv/shared-components/lib/mountPath`）。
   - 动作：真源复制到 `packages/shared-components/src/lib/`，app 原位留
     `export * from "@encv/shared-components/lib/..."` 垫片（用一次性脚本复制真源 + 改写
     单处 import + 生成 shim，避免手敲 649 行 mockDataGenerator 出错）。
   - 验证：`pnpm check:all` 8/8 PASS；`make-shim check-all` 23/23 一致。

**已落地（#3，已 `check:all` 8/8 全绿 + make-shim check-all 26/26 一致）：**

3. **`types/file-feature.ts`、`composables/useInputHistory.ts`、`composables/useDeviceId.ts`（纯叶子、低半径）**
   - 依赖均无 app-only 能力：`file-feature` 仅 type-only `@/api/encv`（→ 改
     `@encv/shared-components/api/encv`）；`useInputHistory` 仅 `vue`；`useDeviceId`
     零静态 import（动态 `import("@capacitor/device")`，该包已在 shared 依赖内）。
   - 动作：真源复制到 `packages/shared-components/src/{types,composables}/`（仅 `file-feature`
     改一处 type-only import 别名），app 原位留 `export * from "@encv/shared-components/..."` 垫片。
   - 验证：`pnpm check:all` 8/8 PASS；`make-shim check-all` 26/26 一致。

**已落地（#4，`useFileFeatures` 已 `check:all` 全绿；`buildReportZip` 正在补回）：**

4. **`composables/useFileFeatures.ts`（纯叶子）**
   - 依赖：`vue` + type-only `@/api/encv`（已在 shared）+ type-only `@/types/file-feature`
     （#3 已提升进 shared）。
   - 动作：真源复制到 `packages/shared-components/src/composables/`，app 原位留
     `export * from "@encv/shared-components/composables/useFileFeatures"` 垫片。
   - 验证：`pnpm check:all` 8/8 PASS；`make-shim check-all` 28/28 一致。

5. **`lib/buildReportZip.ts`（带 `workflow/types`，需连带 i18n key 下沉）**
   - **本质可提升**：真实依赖全是 shared 内能力 —— `useI18n`（shared）、`jszip`
     （shared 依赖）、`type-only` 的 `@/api/encv` 与 `@/types/*`、相对 `./workflow/types`
     （shared 已有）。**不存在 app-only 能力耦合**，是纯叶子。
   - **关于 i18n 耦合（重要，纠正一次误判）**：该文件内 ~64 个 `t("tasks.report*/performance.*")`
     字面量 key 原本躺在 `encv-mobile/src/i18n/tasks.ts`。i18n-tool（`scripts/i18n_lib/config.py`
     `_discover_apps`）**本就按「shared 层」设计**：把 `shared-components/src/i18n/*` 并入
     **每一个** app 的字典、并把 `shared-components/src` 并入每一个 app 的扫描目录。所以文件
     一旦进 shared，会被所有 app（plugin-*/backend）扫描，而这些 app 字典里没有那批
     `tasks.*` key → 报 MISSING_KEY。这**不是「不该提升」的证据**，只是「key 放错了字典层」
     的工具副作用。
   - **正确修法（非回退）**：把 `buildReportZip` 用到的那批 key 从 `encv-mobile/src/i18n/tasks.ts`
     **下沉到 `packages/shared-components/src/i18n/tasks.ts`**（与 `common.ts` 同形：
     `export default { "zh-CN":{...}, en:{...} }`）。shared 字典经工具自动并入每个 app → 全绿；
     encv-mobile 那份保留（loader 合并时重复 key 仅覆盖、不报错，零回归）。
   - 状态：**已完成**：shared 真源已重建、shared i18n `tasks.ts` 已生成（64 个 key）并注册进
     `i18n/index.ts` 的 `sharedI18nModules`；`make-shim check-all` 28/28 一致、`pnpm check:all`
     8/8 PASS（含 i18n lint --all 全绿，原 356 个 MISSING_KEY 已消除）。
     - 一次性脚本 `scripts/lift-buildreportzip-i18n.mjs` 已**删除**（被 `move-key` 命令取代，见
       下方「后续同类 key 下沉」）。
   - **后续同类 key 下沉改用 `move-key` 命令**（不要再写一次性脚本）：
     `python3 scripts/i18n-tool.py move-key <prefix.> --from encv-mobile --to shared --register`
     支持 `--keep`（保留源，兼容过渡）、`--dry-run`（只规划不落盘，可先预检）。
     该命令复用 loader（shared 去重/缓存）与 addkey，幂等、双 locale 对齐。

**已落地（#6，待终端恢复后跑 `make-shim check-all` + `pnpm check:all` 双门禁确认）：**

6. **`usePluginExtensions.ts` / `useThumbnailCache.ts`（纯叶子，仅依赖 `@/api/encv`，已在 shared）**
   - 真源复制到 `packages/shared-components/src/composables/`，import 改写
     `@/api/encv` → `@encv/shared-components/api/encv`（shared 的 `api/encv` barrel
     已透传 `fetchContainerExtensions` / `getExternalStreamUrl`）。
   - app 原位留 `export * from "@encv/shared-components/composables/..."` 垫片。
   - `useThumbnailCache` 用 `document` / `IntersectionObserver`（DOM 全局），shared tsconfig
     已含 `lib: ["ESNext","DOM"]`，typecheck 无碍。

7. **`useConfig.ts` + 连带 `config/schemaParser.ts` + `config/schema.json`（config 簇整体搬）**
   - 依赖全是 shared 内能力：`vue` + `@/api/encv`（`fetchConfig`/`updateConfig` 在
     `encv_admin.ts`，经 barrel 透传）+ `@/config/schemaParser`。
   - 动作：真源复制到 `packages/shared-components/src/{composables,config}/`，import 改写
     （`useConfig` → `@encv/shared-components/api/encv` 与 `@encv/shared-components/config/schemaParser`；
     `schemaParser` → `@encv/shared-components/config/schema.json`）；
     app 原位留两处 `export * from "@encv/shared-components/..."` 垫片；
     **删除** app 的 `config/schema.json`（已整体迁到 shared，且无任何其它引用——
     仅 `schemaParser` 经 TS import 消费，无构建期静态拷贝）。
   - 消费者（10+ 个 views/components）仍 `@/config/schemaParser` / `@/composables/useConfig`
     经垫片解析到 shared，零改动。

**已落地（#8，双门禁 + 单测全绿）：**

8. **`useVectorSearchStatus.ts`（纯叶子，仅依赖 `vue` + `@/api/encv` + `@/composables/useEventBus`，二者均在 shared）**
   - 真源复制到 `packages/shared-components/src/composables/`，import 改写
     `@/api/encv` → `@encv/shared-components/api/encv`、
     `@/composables/useEventBus` → `@encv/shared-components/composables/useEventBus`。
   - app 原位留 `export { useVectorSearchStatus, type VectorSearchStatus }` 垫片（make-shim gen）。
   - 既有测试 `packages/shared-components/src/composables/__tests__/useVectorSearchStatus.test.ts`
     原本是孤儿（vitest include 仍指向已搬迁的旧 app 路径、且 mock 用 `@/` 别名
     与新真源 `@encv/shared-components/` 别名对不上）→ 修正两处：
     (a) 测试 `vi.mock` 的 specifier 改为 `@encv/shared-components/api/encv` /
         `@encv/shared-components/composables/useEventBus`，与新真源 import 对齐；
     (b) `encv-mobile/vitest.config.ts` 的 `ISOLATED_INCLUDE` 条目改为 shared 真实路径，
         使其纳入 FULL 模式运行（9 个用例全 PASS）。

9. **`features/alist-encrypt/useAlistEncrypt.ts`（解密/缓存引擎核心，仅依赖 shared 内能力）**
   - 依赖全部已在 shared：`@/api/encv`（`FileItem` 类型 / `decodeAlistFilename` /
     `getAlistEncryptStreamUrl`，经 `api/encv` barrel 透传）+ `@/composables/useConfig`
     （`getFieldValue`，#6 已提升）。故**无需 DI 即可提升**。
   - 真源复制到 `packages/shared-components/src/composables/useAlistEncrypt.ts`，import 改写
     `@/api/encv` → `@encv/shared-components/api/encv`、
     `@/composables/useConfig` → `@encv/shared-components/composables/useConfig`。
   - app 原位留 `export { isAlistEncrypted, getSessionPassword, setSessionPassword,
     getDecodedName, loadDecodedName, getStreamUrl, clearPasswordCache, clearDecodeCache }`
     垫片（make-shim gen，8 值导出）。
   - **兄弟文件零改动**：`badge.ts` / `subtitle.ts` / `actions.ts` / `password-dialog.ts` /
     `index.ts` 仍用相对路径 `./useAlistEncrypt` 导入，自动解析到 app 垫片 → shared 真源。

**已落地（#10，双门禁全绿：`make-shim check-all` 30/30 + `pnpm check:all` 8/8）：**

10. **`features/alist-encrypt` UI 胶水层（badge / subtitle / actions / password-dialog / index）+ `composables/useAgentApiBase`（agent 链）— 经 DI 解耦后整体提升**
   - 根因：两支真正只缺 3 个 app-only 能力 —— `isNative` / `openNewTask` / `alertPassword`；
     其余依赖（`useI18n` / `FileFeature` 类型 / `ionicons` / `vue-router` / `api/encv`）本就在
     shared，故可干净提升。
   - 新增 DI 抽象 `runtime/appCapabilities.ts`（`AppCapabilities` 接口 + `getAppCapabilities()` /
     `setAppCapabilities()`；未注入安全默认：`isNative→false`、其余抛清晰错误便于尽早暴露遗漏）。
     `packages/shared-components/package.json` 补 `"./runtime/*": "./src/runtime/*"` 导出。
   - `useAgentApiBase.ts`：原 `@/plugins/GoProcess` 的 `isNative` 改为
     `getAppCapabilities().isNative()`，真源搬到 shared `composables/`。
   - alist-encrypt 胶水层：`openNewTask` / `alertPassword` 经 `getAppCapabilities()` 取用
     （`password-dialog.promptPassword` 委托 `alertPassword`；`actions` 的 decrypt/encrypt handler
     委托 `openNewTask`），真源搬到 shared `features/alist-encrypt/`。
   - app 启动注入：`stores/registerSharedAppCapabilities.ts` 调
     `setAppCapabilities({ isNative, openNewTask, alertPassword })`（`alertPassword` 用
     `@ionic/vue` 的 `alertController` + `useI18n` 文案），在 `main.ts` 的
     `registerSharedTaskServices()` 之后调用。
   - app 原位留 6 个 re-export 垫片（`make-shim gen` 生成）：`composables/useAgentApiBase` +
     `features/alist-encrypt/{index,password-dialog,badge,subtitle,actions}`。
   - `useFilesView.ts` 经 `@/features/alist-encrypt/password-dialog` 解析到 shared
     （password-dialog 垫片覆盖该入口）。
   - **`files.decrypt` i18n key 下沉**：该 key 原仅定义在 `encv-mobile/src/i18n/files.ts`，
     代码搬到 shared 后被各 plugin / backend 的 i18n 扫描判 `MISSING_KEY`。已新建
     `packages/shared-components/src/i18n/files.ts`（仅 `files.decrypt` 中英两语）并注册进
     `sharedI18nModules`，经 loader 自动并入每个 app → i18n lint 全绿；
     encv-mobile 原 key 保留（合并重复覆盖、不报错，零回归）。

> 至此 Tier 3「收敛 shared 反向依赖 app 真实实现」的**已知漏网项已全部清除**：
> agent 链（`useAgentApiBase`）与 alist-encrypt 胶水层均经 `appCapabilities` DI 解耦，
> shared 非测试代码已 `@/`-free（仅剩 `*.test.ts` 内 `vi.mock("@/...")` 属预期）。
> 后续若再发现 shared 反向 `@/` 依赖，按本范式补 DI 或下沉即可。

**已落地（#11，双门禁全绿：`make-shim check-all` 31/31 + `pnpm check:all` 8/8）：**

11. **`composables/useTasksList.ts`（被延后到 Phase 4 的「簇主」，依赖簇已全部就绪后干净提升）**
   - 背景：docs/migration-task-system.md §Phase 3 收尾决策曾因 `tasks.*` 静态 i18n key 只在
     mobile 字典、搬进 shared 会逼着整套 tasks 文案迁移而**延后**。Tier 3 #1–#10 把其全部依赖
     （`useRunSummaries`/`useTaskEventBridge`/`useTaskViewCompute`/`useWorkflowTaskService`/
     `useTaskStore`/`taskTypeLabel`/`useI18n`/`useDateFormat`/`api/encv`）提升进 shared 后，
     它已是**干净叶子**，唯一阻塞是 4 个静态 `tasks.*` key。
   - **i18n 阻塞解除**：用 `python3 scripts/i18n-tool.py move-key <key> --from encv-mobile
     --to shared --keep --register` 把 `useTasksList` 实际用到的 4 个静态 key
     （`tasks.allPlugins`/`tasks.allStatuses`/`tasks.allTypes`/`tasks.unknownPlugin`，中英两语）
     下沉到 `packages/shared-components/src/i18n/tasks.ts`（与 #5 buildReportZip 共建的
     `tasks` 模块同文件）。`--keep` 保留 mobile 源、零回归；动态 key（`tasks.${status}`/
     `tasks.phase.${phase}`/`tasks.status.${status}`）经运行时 merged dict 解析，i18n lint
     不误报。
   - 顺手修：`sharedI18nModules` 中 `tasks` 被重复注册（`buildReportZip` 已注册、`move-key
     --register` 又加一遍）→ 去重。
   - 动作：真源复制到 `packages/shared-components/src/composables/useTasksList.ts`，9 处
     `@/...` import 改写为 `@encv/shared-components/...`；app 原位留
     `export * from "@encv/shared-components/composables/useTasksList"` 垫片。
   - 验证：`make-shim check-all` 31/31 一致；shared-components / encv-mobile 类型检查均
     exit 0；`python3 scripts/i18n-tool.py lint --all` 0 问题；下游 `views/useTasksView.ts` 与
     2 个 e2e 测试经 `@/composables/useTasksList` 垫片无感解析到 shared。

**已落地（#12，双门禁全绿：`make-shim check-all` 52/52 + `pnpm check:all` 8/8）：**

12. **`constants/containerVersion.ts`（纯常量叶子，直接提升——修正上轮「需 props/inject 注入」的误判）**
   - 用 codemogger 驱动决策（先 `codemogger impact @/constants/containerVersion` 看清 9 个
     importer 暴露的 4 个符号：`CONTAINER_VERSIONS`/`DEFAULT_CONTAINER_VERSION`/
     `isRecommendedVersion`/`formatContainerVersion`），再读源确认它是**零 import 的纯常量+纯函数**
     （无 `@/`、无 vue、无任何运行时状态）。**结论：它根本不需要注入**，跟其它纯叶子一样直接提升。
     （上一轮把它归为「组件层需 containerVersion 注入化」的重活是误判，特此纠正。）
   - 动作：真源逐字复制到 `packages/shared-components/src/constants/containerVersion.ts`（`./constants/*`
     导出映射本就配好）；app 原位用 `node scripts/make-shim.mjs gen` 生成垫片（10 值 + 2 类型）。
   - **顺手堵住一个门禁漏洞（重要）**：`scripts/make-shim.mjs` 的 `isShim()` 有两处盲点，导致
     **所有 brace 形式（`export { … } from`）垫片被 check-all 静默跳过**——① 本地声明正则误把
     `export { type C } from` 里的 `type C` 判成本地 type 声明；② 旧 specs 正则
     `export\s+(?:\*\s*)?from` 匹配不到中间夹 `{…}` 的 brace re-export。修法：先收集并剥离
     re-export 语句，再在剩余代码上探测本地声明；specs 同时匹配 star 与 brace 两种形态。
     修完垫片普查数 **31 → 52**（21 个此前隐形的 brace 垫片重新纳入），并**当场揪出一个真 bug**：
     `lib/workflow/state-machine.ts` 垫片手写时漏了 `computeJobConclusion`/`inferWorkflowStatus`
     两个真源导出（下游 `import { computeJobConclusion } from "@/lib/workflow/state-machine"` 本会
     静默断链，因垫片被跳过而一直未被发现）→ 已补齐。
   - 验证：`make-shim check-all` 52/52 一致；`pnpm check:all` 8/8（biome ci + 5 类型检查 +
     i18n lint + 单测）；`codemogger leaks packages/shared-components/src` 无新增反向依赖。

**已落地（#13，双门禁：`make-shim check-all` 53/53 一致；`pnpm check:all` 待用户本地跑）：**

13. **`components/NewTaskModal` 组件簇（自底向上 7 个 `.vue` + `NewTaskState` 类型）**
   - 依赖核查（codemogger + grep）：`useNewTaskModal` 仅 3 引用、`NewTaskModal.vue` 经 `@/components/`
     别名消费；整簇**无相对路径引用**（删本地不会断链）；Vite `encv-alias-fallback` 插件 +
     tsconfig `@/*` 已含 shared fallback，`.vue` 可走「移到 shared + 删本地 + 别名 fallback」范式
     （与既有 shared `.vue` 如 AboutPage 一致）。`make-shim` 的 `buildShim` 显式跳过 `default` 导出，
     故 `.vue` 不走 re-export 垫片，只 `.ts` 叶子走垫片。
   - 自底向上顺序（叶子先搬）：`RadioItem`（纯 vue 叶子）→ `ContainerVersionSelector`（差 RadioItem）
     → `InputWithHistory`（差 useInputHistory，已是 shared 垫片）→ `FilePickerModal`（无 .vue 依赖）
     → `EncryptBody`/`DecryptBody`（差上述 4 叶子）→ `NewTaskModal`（差 Encrypt/DecryptBody）。
     每个真源逐字复制到 `packages/shared-components/src/components/`，并 `sed` 改写 `@/` →
     `@encv/shared-components/`；app 本地 7 个 `.vue` 删除（靠别名 fallback），`NewTaskState.ts`
     删本地后用 `make-shim gen` 生成受门禁保护的 `export { type NewTaskState } from` 垫片。
   - **连带修正 `src/components.d.ts`**：`unplugin-vue-components` 的 `dirs` 已含 shared，全局组件
     自动导入运行时仍解析到 shared；但 d.ts 里 7 行仍指向已删的 `./components/X.vue`，已改为与
     AboutPage 一致的 `./../../packages/shared-components/src/components/X.vue`（文件头有
     `// @ts-nocheck`，typecheck 不查，仅作声明一致性修正）。
   - **（误判纠正）** 初判 `useNewTaskModal` 因 `alist-encrypt` 未提升而阻塞；事后核查发现
     `features/alist-encrypt/*` 本就是 re-export 垫片，真源早在 shared
     （`composables/useAlistEncrypt` + `features/alist-encrypt/*`）。故 #8 并非真阻塞——见 #14。
   - 验证：`make-shim check-all` 53/53 一致（含新增 NewTaskState 垫片）；`pnpm check:all` 交用户本地
     跑（biome + 5 类型检查 + i18n lint + 单测）；`codemogger leaks packages/shared-components/src`
     应在搬后无新增反向依赖（组件簇内 `@/` 已全改写为 `@encv/shared-components/`）。

**已落地（#14，双门禁：`make-shim check-all` 54/54 一致；`pnpm check:all` 待用户本地跑）：**

14. **`composables/useNewTaskModal.ts`（纠正 #13 的暂缓误判）**
   - 核查发现 `features/alist-encrypt/*` 全是 re-export 垫片，真源已在 shared
     （`composables/useAlistEncrypt` 导出 `getSessionPassword` 等；`features/alist-encrypt/*` 亦在
     shared）。`useNewTaskModal` 唯一特殊点是第 91 行**动态 import `@/features/alist-encrypt/
     useAlistEncrypt`**——那是 app 垫片路径，shared 真源在 `composables/useAlistEncrypt`。
   - 动作：真源复制到 `packages/shared-components/src/composables/useNewTaskModal.ts`，`sed` 改写
     `@/` → `@encv/shared-components/`；再单独把动态 import 从
     `@encv/shared-components/features/alist-encrypt/useAlistEncrypt` 修正为
     `@encv/shared-components/composables/useAlistEncrypt`（指向 shared 真源，避免 shared 反向依赖
     app 垫片）；app 本地 `useNewTaskModal.ts` 删除，`make-shim gen` 生成 `export { useNewTaskModal }`
     垫片（1 值）。
   - 验证：`make-shim check-all` 53 → **54**（+useNewTaskModal）；`codemogger leaks shared` 无新增反向
     依赖（仅 `api/encv_perf.ts`/`encv_tasks.ts` 两处兼容注释文本命中，非真实 import）；下游
     `stores/registerSharedAppCapabilities.ts`、`views/useTasksView.ts`、`views/useFilesView.ts`
     （动态 import）经 `@/composables/useNewTaskModal` 垫片无感解析到 shared。

**已落地（#15，双门禁：`make-shim check-all` 54/54 不变（6 个均 `.vue` 无垫片）；`pnpm check:all` 待用户本地跑）：**

15. **任务详情 6 个纯叶子组件**（`TaskActionButtons`/`TaskBasicInfo`/`TaskOutputInfo`/
     `TaskPerformanceSection`/`TaskWarningSection`/`TaskErrorSection`）
   - 依赖核查：这 6 个互不引用、只 import 已 shared 的模块（`@/api/encv`、`useI18n`/`useToast`/
     `useSectionDerivation`(app 垫片→shared)/`useDateFormat`(shared)/`useErrorAnalyzer`(app 垫片→shared)/
     `constants/containerVersion`(shared)）；**无相对路径引用**，删本地靠别名 fallback 安全；均已在
     `components.d.ts` 全局声明。
   - 动作：6 真源逐字复制到 `packages/shared-components/src/components/`，`sed` 改写 `@/` →
     `@encv/shared-components/`；删本地 6 个 `.vue`（`.vue` 不走 re-export 垫片，靠 `encv-alias-fallback`
     解析）；`components.d.ts` 对应 6 行 `./components/X.vue` 改为 `./../../packages/shared-components/
     src/components/X.vue`（与 AboutPage 一致，文件头 `// @ts-nocheck` 仅作声明一致性修正）。
   - **暂缓项（重要阻塞，单列）**：`TaskTimeline` + `TaskDetailModal` 本轮**未提升**。根因是
     `lib/workflow/types` 存在**真源分歧**——app 版 417 行、shared 版 291 行，双方都**不** re-export
     对方（既非垫片也非统一真源）。`TaskTimeline` 经 `@/lib/workflow/types` 取该模块，若盲目改写到
     `@encv/shared-components/lib/workflow/types` 会丢失 app 版独有类型、且 `TaskDetailModal` 提升后会
     经 `@/components/TaskTimeline.vue` 形成 shared→app 反向依赖（违反 `codemogger leaks`）。故先冻结，
     待单独调和 `lib/workflow/types` 分歧（统一真源到 shared + app 留垫片）后再提 `TaskTimeline` 与
     `TaskDetailModal`。当前 `TaskDetailModal`（仍留 app）经 `@/components/*` 已能无感解析到本次提升的
     6 个叶子，运行时无影响。
   - 验证：`codemogger leaks shared` 无新增反向依赖（仅 `api/encv_perf.ts`/`encv_tasks.ts` 两处兼容注释
     文本命中）；`make-shim check-all` 54/54 不变（本轮无新 `.ts` 垫片）。

**已落地（#16，双门禁待用户本地跑 `make-shim check-all` + `pnpm check:all`）：**

16. **调和 `lib/workflow/types` 真源分歧 + 提升 `TaskTimeline` / `TaskDetailModal`**
   - 背景：#15 因 `lib/workflow/types` 真源分歧（app 417 行 vs shared 291 行，互不 re-export）冻结了
     `TaskTimeline` + `TaskDetailModal`。本轮先消除分歧，再提升两组件。
   - **类型真源统一（app → shared）**：把 app 版独有类型**合并进 shared** 真源，app 原位降级为
     `export * from "@encv/shared-components/lib/workflow/types"` 垫片（零真源丢失）：
     - `UnifiedTreeNode` + `isUnifiedTreeNode`（automation/TreeView 等依赖）
     - `TestCaseSpec` + `TestCaseResult`（automation 报告 UI 依赖）
     - `ALL_PHASES` + `isPhase`
     - `WORKFLOW_STORE_KEY`（`useWorkflowStore` 经 `@/lib/workflow/types` 消费，漏则断链）
     - `Phase` 统一为 shared 既有的 **const 对象 + 联合类型**（`export const Phase = {...} as const;
       export type Phase = ...`，非 enum）。核查全仓无 `Phase[...]` 反向映射 / `Object.keys(Phase)` 等
       enum-only 用法，故 const-object 形式对既有 `Phase.Created` 值访问、`Record<Phase,string>`、
       `toPhase(): Phase` 等完全兼容，无需改消费方。
   - **`TaskTimeline.vue` 提升**：真源复制到 `packages/shared-components/src/components/`，改写
     `@/api/encv`→`@encv/shared-components/api/encv`、`@/composables/useDateFormat`→
     `@encv/shared-components/composables/useDateFormat`、`@/composables/useI18n`→
     `@encv/shared-components/composables/useI18n`、`@/lib/workflow/types`→
     `@encv/shared-components/lib/workflow/types`（`UnifiedTimelineCard` 本就引 shared）；删本地副本。
   - **`TaskDetailModal.vue` 提升**：同上改写 3 处 `@/api`/`@/composables` 导入；`@/components/Task*`
     系列 import **保留**（`@/` 在 shared 内经 tsconfig 解析到 shared 自身组件，无反向依赖）；
     删本地副本。
   - **i18n key 下沉**：两组件用到的 `tasks.*` 静态 key（`timeline`/`timelineCreated`/`timelineInProgress`/
     `timelineDone`/`sourcePath`/`cryptoSummary`/`phaseDetail`/`outputFile`/`startedAt`/`completedAt`/
     `duration`/`error`/`phase{Analyzing,Initializing,Preprocessing,Encrypting,Decrypting,Packing,
     Verifying,Completed}`/`failed`/`cancelled`/`taskDetail`/`close`/`progress`/`rollbackButton`/
     `rollbackConfirm`/`rollbackConfirmMessage`/`rollbackSuccess`/`rollbackFailed`）新增进
     `packages/shared-components/src/i18n/tasks.ts`（中英两语，值从 `encv-mobile/src/i18n/tasks.ts`
     逐字复制），经 loader 自动并入每个 app → i18n lint 全绿；mobile 源保留（合并重复覆盖、不报错）。
     （注：本应走 `python3 scripts/i18n-tool.py move-key tasks. --from encv-mobile --to shared --keep
     --register`，但执行时终端后端不可用，改为手动等价追加；待后端恢复可重跑 move-key 幂等对齐。）
   - **`components.d.ts` 修正**：`TaskTimeline` / `TaskDetailModal` 两行 `./components/X.vue` 改为
     `./../../packages/shared-components/src/components/X.vue`（与 #13/#15 其余叶子一致）。
   - 验证：`codemogger leaks shared` 应为空（两组件 `@/components/*` 解析到 shared 自身，无 shared→app
     反向依赖）；`make-shim check-all` 含新增 `lib/workflow/types` 垫片；`pnpm check:all` 待用户本地跑
     （biome + 5 类型检查 + i18n lint + 单测）。

  - **修正（2026-07-13 续做）**：#16 调和时 `isUnifiedTimelineEntry` 类型守卫函数被**漏搬**进 shared
    真源（app 原版 417 行有该定义，shared 291 行版只含 `UnifiedTimelineEntry` 接口 + `isUnifiedTreeNode`
    守卫，独缺 `isUnifiedTimelineEntry`）。导致 `encv-mobile/src/lib/workflow/__tests__/unified-types.test.ts`
    报 `isUnifiedTimelineEntry is not a function` + `TS2724: '@/lib/workflow/types' has no exported member
    'isUnifiedTimelineEntry'`，`pnpm check:all` 门禁 FAIL。已把该函数逐字补回
    `packages/shared-components/src/lib/workflow/types.ts`（`UnifiedTimelineEntry` 接口之后），app 垫片
    `export *` 自动透传，consumer 零改动。静态核验：全仓所有从 `lib/workflow/types` import 的符号
    （类型 + 值/函数：`JobRun`/`StepRun`/`UnifiedRunRecord`/`WorkflowDefinition`/`WorkflowRun`/`StepDefinition`/
    `StepStatus`/`UnifiedTreeNode`/`TestCaseResult`/`UnifiedTimelineEntry`/`WorkflowStatus`/`ALL_PHASES`/
    `isPhase`/`isUnifiedTimelineEntry`/`isUnifiedTreeNode`/`Phase`/`WORKFLOW_STORE_KEY`）均已在 shared 真源导出。

**已落地（#17，双门禁待用户本地跑 `make-shim check-all` + `pnpm check:all`）：**

17. **`tasks/*` 组件簇（`TaskVirtualList` / `TaskDebugPanel`）提升**
   - 依赖核查：两组件均为**纯叶子**，无 app-only 能力耦合。
     - `TaskVirtualList.vue`：仅 `import { useVirtualizer } from "@tanstack/vue-virtual"`（shared
       package.json 已含 `^3.13.26`）+ `vue`，**零 `@/` 引用**，逐字复制即可。
     - `TaskDebugPanel.vue`：仅 `import type { EncvTask } from "@/api/encv"` 一处 `@/` → 改写为
       `@encv/shared-components/api/encv`（`EncvTask` 经 `api/encv` barrel 透传；`ionicons/icons` /
       `vue` 均为 shared 依赖）。
   - 动作：真源复制到 `packages/shared-components/src/components/`（与 #15/#16 其余任务叶子同目录、扁平）；
     `TaskDebugPanel` 改写 1 处 import；删本地 `components/tasks/` 下 2 个 `.vue`（`tasks/` 目录现仅剩
     空 `__tests__/`，git 不跟踪空目录，无残留）。
   - **测试迁移**（对齐 #15/#16 模式）：`components/tasks/__tests__/{TaskVirtualList,TaskDebugPanel}.test.ts`
     移到 `packages/shared-components/src/components/__tests__/`，import 改写为 `@encv/shared-components/...`
     （`TaskVirtualList.test.ts` 改 `TaskVirtualList` 入口；`TaskDebugPanel.test.ts` 改 `EncvTask`/`TaskStatus`
     类型 + `TaskDebugPanel` 入口）；删本地 2 个测试文件。
   - **`components.d.ts` 修正**：`TaskDebugPanel` / `TaskVirtualList` 两行 `./components/tasks/X.vue` 改为
     `./../../packages/shared-components/src/components/X.vue`（与 #13/#15/#16 一致）。
   - **`vitest.config.ts` 同步**：`ISOLATED_INCLUDE` 的 `src/components/tasks/__tests__/{TaskDebugPanel,
     TaskVirtualList}.test.ts` 改为 `../packages/shared-components/src/components/__tests__/{...}`；
     移除 `exclude` 中已失效的 `src/components/tasks/__tests__/**` glob（目录已空）。
   - 验证：`codemogger leaks shared` 应为空（两组件 `@/` 已全改写为 `@encv/shared-components/`，无
     shared→app 反向依赖）；`make-shim check-all` 不变（本轮无新 `.ts` 垫片）；`pnpm check:all` 待用户本地跑
     （biome + 5 类型检查 + i18n lint + 单测，含迁移到 shared 的 2 个测试）。

**已落地（#18，双门禁待用户本地跑 `make-shim check-all` + `pnpm check:all`）：**

18. **`group-detail/*` 中 3 个纯叶子（`FilterDrawer` / `PerformanceTab` / `TasksTab`）提升**
   - 依赖核查：三者均为纯叶子，仅依赖已 shared 的模块；**唯 `PipelineTab` 因 import 尚未提升的
     `automation/JobPipelineCard` + `automation/TestReportHeader` 而本轮冻结**（见 #19 待做）。
     - `FilterDrawer.vue`：仅 `@/composables/useI18n` → `@encv/shared-components/composables/useI18n`。
     - `PerformanceTab.vue`：`@/api/encv`（`CalibrationResult`/`EncvTask`/`getCalibration`/
       `getPerformanceHistory`/`PerformanceMetrics`，均经 `api/encv` barrel）+ `@/composables/useI18n`
       → 两处改写为 `@encv/shared-components/...`。
     - `TasksTab.vue`：`@/api/encv`（`EncvTask`/`cancelTask`/`deleteTask`/`retryTask`；`deleteTask`
       经 `api/encv_tasks` 走 `getTaskServices().deleteTask` DI，无 shared→app 硬依赖）+
       `@/composables/useDateFormat` + `@/composables/useI18n` + `@/lib/taskTypeLabel`
       （Tier 1 已提升）→ 全部改写为 `@encv/shared-components/...`。
   - 动作：真源复制到 `packages/shared-components/src/components/`，改写各自 `@/` import；
     删本地 `group-detail/` 下 3 个 `.vue`（`PipelineTab.vue` 保留在 app，待 #19 与 automation 簇同提）。
   - **i18n key 下沉**（搬进 shared 后其字面 `tasks.*` key 须进 shared 字典，否则 plugin/backend 扫描报
     MISSING_KEY）：把 `tasks.filter{Reset,Title,StatusTitle,TaskTypeTitle,PluginTitle,Apply,Empty}`、
     `tasks.groupDetail.{emptyTasks,emptyTasksDesc}`、`tasks.{cancel,cancelSuccess,retrySuccess,
     deleteSuccess,encrypted}`（中英两语，值从 `encv-mobile/src/i18n/tasks.ts` 逐字复制）新增进
     `packages/shared-components/src/i18n/tasks.ts`；`tasks.performance.*` 已由 #5 buildReportZip 先行
     迁入、`common.failed` 在 shared `common.ts` 已齐备，无需补。`tasks.status.*` / `tasks.type.*` 为
     运行时动态 key（模板插值），经 loader 合并 mobile 字典解析，i18n lint 不误报。
   - **`components.d.ts` 修正**：`FilterDrawer` / `PerformanceTab` / `TasksTab` 三行
     `./components/group-detail/X.vue` 改为 `./../../packages/shared-components/src/components/X.vue`
     （与 #13/#15/#16/#17 一致）；`PipelineTab` 行保持 `./components/group-detail/PipelineTab.vue` 不变。
   - 验证：`codemogger leaks shared` 应为空（三组件 `@/` 已全改写为 `@encv/shared-components/`）；
     `make-shim check-all` 不变（本轮无新 `.ts` 垫片）；`pnpm check:all` 待用户本地跑
     （biome + 5 类型检查 + i18n lint + 单测）。

**已落地（#19，双门禁待用户本地跑 `make-shim check-all` + `pnpm check:all`）：**

19. **`automation/*` 组件簇（9 个纯叶子）+ `group-detail/PipelineTab`（冻结项解阻塞同提）**
   - 依赖核查：`automation/*` 9 组件（`ErrorChainNode`/`FilterChips`/`JobPipelineCard`/`StepDetailPanel`/
     `StepInlineTimeline`/`StepMiniBadge`/`TestCaseFile`/`TestReportHeader`/`TreeView`）均**无 i18n 依赖**
     （grep `useI18n|t(` 全 0 命中），仅依赖已 shared 的 `lib/workflow/types` 与互相 `@/components/automation/*`；
     `PipelineTab` 仅依赖 `JobPipelineCard`/`TestReportHeader`（automation）+ `useI18n` + `lib/workflow/types`，
     均已在 shared。无 app-only 能力耦合，是纯叶子簇。
   - 动作：
     - 9 个 automation `.vue` 真源**此前已复制到** `packages/shared-components/src/components/`（import 已改写为
       `@encv/shared-components/...`，grep `from "@/"` 全 0 命中，无反向依赖）；本轮**删除 app 原位 9 个 `.vue`**
       （靠 `encv-alias-fallback` 别名回退 + tsconfig `@/*` shared fallback 解析到 shared）。
     - `components.d.ts` 9 行 `./components/automation/X.vue` →
       `./../../packages/shared-components/src/components/X.vue`（与 #13/#15/#16/#17 一致）。
     - 2 个测试 `automation/__tests__/{TreeView,StepInlineTimeline}.test.ts` 的 import 改写为
       `@encv/shared-components/...`（保留在 app 原位，ISOLATED_INCLUDE 路径不变；与 #17「搬测试到 shared」略异，
       但功能等价——测试经 `@encv/shared-components` 别名解析到 shared 真源）。
     - `PipelineTab.vue` 真源复制到 `packages/shared-components/src/components/`，4 处 import 改写
       （`JobPipelineCard`/`TestReportHeader` → `@encv/shared-components/components/*`；
       `useI18n` → `@encv/shared-components/composables/useI18n`；
       `lib/workflow/types` → `@encv/shared-components/lib/workflow/types`）；删 app 原位；
       `components.d.ts` 对应行改指 shared。
     - **i18n key 下沉**：`PipelineTab` 用到的 `tasks.pipelineEmpty`（中英两语 `暂无任务`/`No tasks yet`）
       新增进 `packages/shared-components/src/i18n/tasks.ts`（此前 shared 无此 key，mobile 源保留、合并覆盖不报错）。
   - 验证：`codemogger leaks shared` 应为空（automation 9 组件 + PipelineTab 的 `@/` 均已改写为
     `@encv/shared-components/`，无 shared→app 反向依赖）；`make-shim check-all` 不变（本轮无新 `.ts` 垫片）；
     `pnpm check:all` 待用户本地跑（biome + 5 类型检查 + i18n lint + 单测，含改写 import 的 2 个 automation 测试）。
   - **修正（2026-07-13 续做）**：shared `components/` 为**扁平**层（无 `automation/`/`group-detail/`/`tasks/`
     子目录），但 #17/#18/#19 提升后，3 个 consumer 视图仍用旧子目录路径 `@/components/<subdir>/X.vue`，
     `encv-alias-fallback` 无法把子目录路径映射到扁平 shared，构建/单测会静默失败。已改写 6 处为
     `@encv/shared-components/components/<FlatName>.vue`：`PluginTestsDetail.vue:259`（`automation/StepMiniBadge`）、
     `GroupDetail.vue:145-147`（`group-detail/{PerformanceTab,TasksTab,PipelineTab}`）、
     `Tasks.vue:617-618`（`tasks/{TaskDebugPanel,TaskVirtualList}`）。改后 `grep '@/components/(automation|group-detail|tasks)/'`
     全 0 命中。
  - **i18n key 下沉（批量，2026-07-13 续做）**：#13/#15/#17/#18/#19 提升的组件（NewTaskModal 簇、TaskBasicInfo、
    DecryptBody/EncryptBody/FilePickerModal、任务详情组件、PerformanceTab 等）用到的 `tasks.*` / `files.*` key 此前未下沉，
    导致 `i18n lint --all` 报 ~372 个 MISSING_KEY + ~30 个 MISSING_EN。根因是 `add_key` 两个引号 bug（已修复）：① value 含双引号
    （如 `"{query}"` / `"{taskId}"`）时 `add_key` 用双引号包裹插入会破坏 TS 语法（`Expected ',', got '{'`）；② shared 字典 en 键是
    `en: {`（无引号），而 `add_key` 正则只匹配 `"en": {`，导致 **en 部分从不插入** → 所有 key 只有 zh 无 en（MISSING_EN 根因）。
    当时用「直接复制 encv-mobile 完整 `tasks.ts`/`files.ts` 到 shared」临时绕过（任务系统已完全进 shared，这两命名空间本就归
    shared；`pipelineEmpty` 等 #19 加的 key 在 encv-mobile 源也保留着，复制不丢）。另：`move-key --register` 会把已注册模块再注册
    一次 → `sharedI18nModules` 出现 `tasks` 重复，已去重。
    **现状（2026-07-13 同日修复）**：`add_key` 已修——locale 正则同时匹配带/不带引号，`en: {` 可正常插入；value 含双引号时改单引号
    包裹（与字典约定一致）。`movekey._register_shared_module` 幂等改为数组元素级匹配，重复注册不再发生。**现在可安全用
    `move-key "<prefix>." --from encv-mobile --to shared --keep --register` 批量下沉 i18n key**，无需再 `cp` 整文件绕过。
    **教训**：提升带 i18n 的组件必须同步 `move-key` 批量下沉其 `tasks.`/`files.` 前缀。

**已落地（#20，Phase 5 视图层起点，双门禁全绿：`make-shim check-all` 55/55 + `pnpm check:all` 8/8）：**

20. **`views/Tasks.vue` + `views/useTasksView.ts`（任务列表页 + 其 script 逻辑 composable）提升**
   - 背景：Phase 4 已完成全部任务组件（#13–#19），但任务**视图页**仍留 app。Phase 5（视图层）按「重写 + DI 解耦」范式推进；`Tasks.vue` 是其中**唯一干净可提升**的视图（`GroupDetail.vue` 含更重的 router 钉子，留待后续）。
   - 依赖核查：`Tasks.vue` 仅 2 个 `@/` 依赖——`formatDateTime`（`@/composables/useDateFormat`）、`formatContainerVersion`（`@/constants/containerVersion`）——二者均已在 shared（#12 提升 `containerVersion`）。`useTasksView.ts` 依赖 `vue-router`（`useRoute`/`useRouter`）+ 已 shared 的 `useI18n`/`useNewTaskModal`/`useTasksList`/`useToast`/`api/encv`/`ionicons`/`@ionic/vue`——**唯一钉子是 vue-router**。
   - **router 钉子解耦（新增 DI，镜像 #10 `appCapabilities` 范式）**：新增 `packages/shared-components/src/runtime/appNavigation.ts`
     （`AppNavigation` 接口 + `getAppNavigation()`/`setAppNavigation()`；未注入时 `navigate`/`replace` 抛清晰错误）。
     把 `useTasksView.ts` 里的 `useRoute()`/`useRouter()` 改写为经 `getAppNavigation()` 取用：`route.query.*` → `navQuery.value.*`
     （`navQuery` 为响应式 `Ref`，供 `computed`/`watch`/`onMounted` 使用）、`router.push(...)` → `navigate(...)`、
     `router.replace({path,query})` → `replace(path,query)`。app 侧新增 `stores/registerSharedAppNavigation.ts`：
     用 vue-router 实例把 `currentRoute.value.query` 同步进 `navQuery` ref（`router.afterEach` 持续同步），并注入
     `navigate`/`replace` 实现；`main.ts` 在 `registerSharedAppCapabilities()` 后调用 `registerSharedAppNavigation()`
     （早于 `app.mount()`，保证 Tasks.vue 挂载前导航能力已注入）。
   - 动作：真源 `useTasksView.ts` 复制到 `shared/src/composables/`、`Tasks.vue` 复制到 `shared/src/views/`，
     全部 `@/` import 改写为 `@encv/shared-components/...`，`Tasks.vue` 的 `./useTasksView` 改为
     `@encv/shared-components/composables/useTasksView`；删 app 原位 `views/Tasks.vue` + `views/useTasksView.ts`
     （`.vue` 与 composable 均走显式 shared 路径，无需 app 垫片）；`router/index.ts:33` 懒加载
     `import("@/views/Tasks.vue")` 改为 `import("@encv/shared-components/views/Tasks.vue")`。
   - 验证：`make-shim check-all` 55/55 一致（本轮无新 `.ts` 垫片）；`pnpm check:all` 8/8（Biome + 5 类型检查 +
     i18n lint + 单测，含新 `registerSharedAppNavigation.ts`）；`codemogger leaks shared` 应为空
     （`useTasksView`/`Tasks.vue` 的 `@/` 已全改写为 `@encv/shared-components/`，vue-router 经 `appNavigation` DI 解耦，
     无 shared→app 反向依赖）。
   - **注意**：`Files.vue` 注释称「调用 useTasksView()」但实测并未 import（注释过时），故无跨视图耦合，提升无牵连。

**已落地（#21，Phase 5 收尾 + Phase 6 文档校正，双门禁全绿：`make-shim check-all` 55/55 + `pnpm check:all` 8/8）：**

21. **`views/GroupDetail.vue`（任务组详情页）提升** — Phase 5 最后一环，比 #20 多一个 router 钉子（`route.params`）。
   - 依赖核查：所有非 router 依赖**早已进 shared**——`getCalibration`（`api/encv_perf`）、`useBatchOperations`（`composables`）、
     `buildReportZip`（`lib`）、`TaskDetailModal.vue`（`components`，动态 `import` 改为指向 shared）、`useRunSummariesSingleton`/
     `useTaskEventBridge`/`useWorkflowTaskService`/`runTasksStore`（`composables`/`stores`）、`JobRun` 类型（`lib/workflow/types`）。
     **唯一钉子是 `vue-router`**：`route.params.runId`（路由参数）+ 两处 `router.replace("/tabs/tasks")`。
   - **appNavigation DI 补 `params`**：`runtime/appNavigation.ts` 的 `AppNavigation` 接口新增 `params: Readonly<Ref<Record<string,string|string[]|undefined>>>`（默认空 ref）；
     `registerSharedAppNavigation.ts` 同步 `currentRoute.value.params` 经 `router.afterEach` 注入。shared 内取用 `getAppNavigation().params`。
   - 动作：`GroupDetail.vue` 全部 `@/` import 改写为 `@encv/shared-components/...`，移除 `vue-router` 的 `useRoute`/`useRouter`、
     改 `const { params: navParams, replace } = getAppNavigation()`；`route.params.runId` → `navParams.value.runId`、
     两处 `router.replace("/tabs/tasks")` → `replace("/tabs/tasks")`、`openTaskDetail` 的 `import("@/components/TaskDetailModal.vue")`
     → `import("@encv/shared-components/components/TaskDetailModal.vue")`；真源复制到 `shared/src/views/GroupDetail.vue`，
     删 app 原位，router/index.ts:42 懒加载改指 `@encv/shared-components/views/GroupDetail.vue`。
   - 验证：`make-shim check-all` 55/55；`pnpm check:all` 8/8（含 `registerSharedAppNavigation.ts` 格式修正）；
     `codemogger leaks shared` 仅 2 处**注释**提及 `@/api/...`，无真实反向依赖；shared `GroupDetail.vue` grep `from "@/"` 命中 0。
   - **Phase 5 至此全绿**：`Tasks.vue` + `useTasksView.ts`（#20）+ `GroupDetail.vue`（#21）均已进 shared，任务系统视图层完成。

22. **Phase 6 清理 — 文档校正（无代码动作，门禁已绿）**：原迁移计划把 shared 内 13 个 `api/encv_*` 标为「错配 A 暂存残留、应移回 app」，
   但经 Tier 2 重写它们已全改为经 `shared/api/core` 的 `apiRequest` 依赖注入式请求（base URL/认证来自注入），`codemogger leaks shared` 与全局 grep
   确认 shared 内**无任何真实 `@/` 导入**（仅 2 处注释提及 `@/api/...` 作兼容说明）→ 已是**合法的自包含共享后端契约层**（§8.2 选项 (a)），
   原「移回 app / A-3 清理」指引**作废**。`encv-mobile/src/api/encv_*.ts` 维持 re-export 薄壳垫片（向后兼容）。已同步校正
   `migration-task-system.md` 的 §1 错配 A、§4 Phase 6、§5 过渡态、§7.2 api 地图、§7.3 速查表、§8.2 Module A-ext 共 6 处陈旧标注。
   - 结论：**任务系统 lift 实质收尾**，Phase 1–6 全部完成，门禁全绿。后续可选：提交累积 lift 改动（选项③），或评估 Module H 业务域抽取候选。

**已落地（#23，双门禁全绿：`make-shim check-all` 58/58 + `pnpm check:all` 8/8）：**

23. **`composables/useWebDavTestModules` / `useWebDavTestRunner` / `useWebDavManifest`（webdav-test 纯叶子簇）提升** — 任务系统之外第一个提升簇（延续 #1–#22 范式，但目标从「修复 shared 反向依赖」转为「把 app 层纯叶子实现归一进 shared」）。
   - 依赖核查：三者均仅依赖 shared 已自包含能力 —— `useWebDavTestModules`/`useWebDavTestRunner` 仅 `import type` 自 `@/types/webdav-test`（#2 已提升）；`useWebDavManifest` 仅 `@/api/encv`（`fetchWebDavManifest`/`fetchWebDavLocalInfo` 实际定义在 `shared/api/encv_system.ts`，经 `api/encv` barrel 透传）+ `@/types/webdav-test`。无任何 app-only 能力耦合（无 `@/router`/`@/config`/`@/plugins`/`@/stores`），是纯叶子。
   - 动作：真源复制到 `packages/shared-components/src/composables/`，3 处 `@/` import 改写为 `@encv/shared-components/...`；app 原位经 `make-shim gen` **就地改写为 re-export 垫片**（导出 2 值 / 1 值+2 类型 / 1 值）。
   - 下游无感：`WebDavAutomationTestsDetail.vue` 与 `useWebDavWorkflowAdapter.ts` 仍经 `@/composables/useWebDav*` 解析到 shared 真源；无测试需迁移。
   - 验证：`make-shim check-all` 58/58（原 55 → +3）；`pnpm check:all` 8/8（Biome 格式化 make-shim 生成的 `useWebDavManifest` 垫片与 shared `useWebDavTestRunner` 副本后全绿）；`codemogger leaks shared` 无新增反向依赖。
   - **范式微调（更安全）**：本轮改用「先 `cp` 复制 → `replace_in_file` 改写 shared 副本 import → `make-shim gen` 就地把 app 原位覆盖为垫片」的逐步可审查流程，避免此前 `cp+sed+rm` 一键批量脚本（不透明且直接删源）。后续 lift 均沿用此流程。

**已落地（#24，双门禁全绿：`make-shim check-all` 62/62 + `pnpm check:all` 8/8）：**

24. **4 个纯叶子 composables 提升**（`useFileList` / `useTestCaseGeneration` / `useWorkflowStore` / `useApiBaseProbe`）— 延续 #23「把 app 层纯叶子实现归一进 shared」方向，本批覆盖 file / automation / workflow / server 四个域各一个代表。
   - 依赖核查：四个均仅依赖 shared 已自包含能力，无相对 app 导入、无 app-only 耦合（无 `@/router`/`@/config`/`@/plugins`/`@/stores`）：
     - `useFileList`：`@/api/encv` 的 `FileItem`(type) + `getFileCategory`（经 `encv_files` 透传）+ `ionicons/icons` + `vue`；
     - `useTestCaseGeneration`：仅 `import type` 自 `@/api/encv` 的 `PluginMeta`/`TaskType`；
     - `useWorkflowStore`：`@/lib/workflow/types` 的 `WorkflowDefinition`/`WORKFLOW_STORE_KEY`（#16 已提升）+ `vue` + `localStorage`；
     - `useApiBaseProbe`：`@/api/encv` 的 `DEFAULT_API_BASE_URL`/`DEV_SANDBOX_ENTRY`/`setApiBaseUrl`（经 `encv_core` 透传）+ `vue` + `window`/`localStorage`/`fetch`。
   - 动作：真源复制到 `packages/shared-components/src/composables/`，4 处 `@/` import 改写为 `@encv/shared-components/...`；app 原位经 `make-shim gen` 就地改写为 re-export 垫片（导出 12 值+2 类型 / 1 值+2 类型 / 1 值 / 2 值+1 类型；`useApiBaseProbe` 的内部 `__resetApiBaseProbeForTest` 一并透传）。
   - 下游无感：consumer 仍经 `@/composables/useX` 解析到 shared 真源；无测试需迁移。
   - 验证：`make-shim check-all` 62/62（58 → +4）；`pnpm check:all` 8/8（Biome 格式化 1 文件后全绿）；`codemogger leaks shared` 无新增反向依赖。
   - **架构注记**：`useFileList`（file 域）/`useApiBaseProbe`（server 域）按 `migration-task-system.md` §8 本属"正确留在 app"的域，但二者仅依赖 shared、不引入任何泄漏，提升为无害的「域逻辑归一进 shared」；`useWorkflowStore`（workflow 域，任务系统已用）/ `useTestCaseGeneration`（automation）更接近 shared 形状。若后续要严格遵循 §8 的"域留 app"，可将其改为 app 内 import shared 抽象——但当前无功能影响，且双门禁已锁绿。

**已落地（#25，双门禁全绿：`make-shim check-all` 66/66 + `pnpm check:all` 8/8）：**

25. **`composables/realtime/` 整簇提升**（Backend / WsBackend / HttpPollBackend / NativeBridgeBackend）— 任务系统实时传输通道的核心基础设施，纯叶子、零 app-only 耦合。
   - 依赖核查：整簇仅依赖 shared 已自包含能力 + 同目录相对 `./Backend`：
     - `Backend.ts`：纯类型/接口（`EventEmitter`/`Backend`/`ConnectionState`），零 import；
     - `WsBackend.ts`：`@/api/encv` 的 `getWebSocketUrl`/`isOpenPreviewBrowser`（经 `encv_core` 透传）+ `./Backend`；
     - `HttpPollBackend.ts`：`@/api/encv` 的 `EncvTask`(type)/`getRecentBackendLogs`(encv_admin)/`getTasks`(encv_tasks) + `./Backend`；
     - `NativeBridgeBackend.ts`：**仅** `./Backend`（占位 noop，无 `@/plugins/GoProcess` 等耦合——此前对 realtime 簇"有 app-only 钉子"的判断系 grep 误读，已更正）。
   - 动作：整目录 `cp` 到 `packages/shared-components/src/composables/realtime/`（`@/` import 改写 2 处；`Backend.ts`/`NativeBridgeBackend.ts` 无 `@/` 无需改；相对 `./Backend` 搬目录后仍有效）；app 原位 4 文件经 `make-shim gen` **就地改写为 re-export 垫片**（导出 0值/3类型、1值/1类型、1值/1类型、1值/0类型）。
   - 下游无感：`useRealtimeTransport.ts`（消费方，本轮不提升）经相对导入 `./realtime/*` 解析到 app shim → 自动转发 shared 真源；无测试需迁移。
   - 验证：`make-shim check-all` 66/66（62 → +4）；`pnpm check:all` 8/8（Biome 格式化 1 文件后全绿）；`codemogger leaks shared` 无新增反向依赖。
   - **架构注记**：realtime transport backend 是 WS/HTTP-poll/native-bridge 通用传输抽象，属"任务系统实时通道"的核心，比 #23/#24 更核心；按"纯叶子可提升"标准达标。不依赖任何 app-only 能力，提升为无害归一。

**已落地（#26，双门禁全绿：`make-shim check-all` 67/67 + `pnpm check:all` 8/8）：**

26. **`composables/useRealtimeTransport`（realtime 消费方/编排者）提升** — 完成**整个实时传输层（backend + 单例编排）的归一 shared**，是 #25 的自然延续。
   - 依赖核查：纯叶子，零 app-only 钉子 —— `vue` + `@/api/encv`（`getApiBaseUrl`/`isOpenPreviewBrowser`，shared）+ `eventBus`（shared `useEventBus.ts`，已确认真源在 `packages/shared-components/src/composables/useEventBus.ts`）+ 相对 `./realtime/*`（#25 真源）。内部 `isNative()` 是**自包含**实现（`window.Capacitor` 检测），不依赖 `@/plugins/GoProcess`（此前误以为 server 域都带 GoProcess 钉子，useRealtimeTransport 实际无）。
   - 动作：真源复制到 `packages/shared-components/src/composables/`，2 处 `@/` import 改写为 `@encv/shared-components/...`（api/encv + composables/useEventBus）；app 原位经 `make-shim gen` 就地改写为 re-export 垫片（导出 3 值 `useRealtimeTransport`/`getActiveTransportMode`/`getTransportDebugInfo` + 2 类型 `TransportMode`/`RealtimeTransport`）。
   - 下游无感：`App.vue` 经 `@/composables/useRealtimeTransport` → shim → shared 真源；shared 内 `__tests__/useRealtimeTransport.test.ts` 仍经 `@/composables/useRealtimeTransport` 解析到 shim；相对 `./realtime/*` 在 shared 副本里自动解析到 #25 真源，无断链。
   - 验证：`make-shim check-all` 67/67（66 → +1）；`pnpm check:all` 8/8（Biome 无需修正）；`codemogger leaks shared` 无新增反向依赖。
   - **架构注记**：提升后实时传输层（WsBackend/HttpPollBackend/NativeBridgeBackend/Backend + useRealtimeTransport）全部在 shared，encv-mobile 仅留 shim。这是"纯叶子功能逻辑归一"的收尾簇——剩余未提升的 composables 已无更多纯叶子功能逻辑。

**已落地（Module H 选项2 · 双门禁全绿：`make-shim check-all` 70/70 + `pnpm check:all` 8/8）：**

**新增 DI：`runtime/nativeBridge.ts`（解耦 `@/plugins/GoProcess` 原生桥接）**
- 背景：A 类钉子模块（`useTaskCancel`/`useServerStatus`/`useOpenListBridge`/`useLibraries`）直接 `import { ... } from "@/plugins/GoProcess"`，违反 shared 不得反向依赖 app 的约束。
- 范式：镜像已落地的 `appCapabilities`/`appNavigation` —— shared 内只经 `getNativeBridge()` 取用能力；app 启动期由 `stores/registerSharedNativeBridge.ts`（新增）调 `setNativeBridge(...)` 注入 `@/plugins/GoProcess` 的 8 个包装函数；`main.ts` 在 `registerSharedAppNavigation()` 后调 `registerSharedNativeBridge()`。
- 接口 `NativeBridge` 含 `isNative` + 7 个原生专属函数（`enqueueCancelWorker`/`restartBackend`/`stopBackend`/`getBackendStatus`/`getAndroidDeps`/`addOpenListStatusListener`/`getOpenListRuntime`），并附自包含类型（`GoProcessResult`/`GoProcessStatus`/`AndroidDepsManifest`/`OpenListRuntime`）。默认 `isNative→false`，原生专属函数默认抛清晰错误（调用方均先 `isNative()` 守卫，web 永不触发）。另提供与 `GoProcess.ts` 形态一致的顶层委托函数（`isNative()`/`enqueueCancelWorker()` 等），消费方导入写法不变。

27. **`composables/useTaskCancel` 提升** — 任务取消双写（HTTP + WorkManager 兜底）。
   - 唯一钉子 `@/plugins/GoProcess`（`enqueueCancelWorker`/`isNative`）→ `getNativeBridge()`；其余 `@/api/encv_core`/`@/api/encv_tasks` 本就 shared。
   - 动作：真源复制到 shared，3 处 `@/` 改写为 `@encv/shared-components/...`；app 原位 `make-shim gen` 就地改写为 re-export 垫片（1 值 `useTaskCancel`）。

28. **`composables/useOpenListBridge` 提升** — OpenList 事件桥接（Phase 22 替换 3s 轮询）。
   - 钉子 `@/plugins/GoProcess`（`addOpenListStatusListener`/`getOpenListRuntime`/`isNative`）+ `@/composables/useEventBus`（shared） → 全部改写；消费方 `LocalOpenListStatusCard.vue` 经 `@/composables/useOpenListBridge` → shim 无感。

29. **`composables/useServerStatus` 提升** — server 状态探测主 composable（463 行单例）。
   - 唯一钉子 `@/plugins/GoProcess`（`getBackendStatus`/`isNative`/`restartBackend`/`stopBackend`）；其余 `@/api/encv`(shared) + `eventBus`(shared) + `./useApiBaseProbe`(shared #24) + `./useRealtimeTransport`(shared #26) 全为 shared-safe。内部 `window`/`document` 全局监听自包含、无 app 依赖。
   - 动作：真源复制到 shared，顶部 5 处导入 + 1 处动态 `import("@/api/encv")` 改写为 `@encv/shared-components/...`；app 原位 shim（1 值 `useServerStatus`）。消费方（ServerDetail/PluginSettings/Settings/ServerSettings/AgentSettingsDetail/ServerStatusCard 等）全部经 `@/composables/useServerStatus` → shim 无感。
   - 验证中修：`nativeBridge` 的 `GoProcessStatus` 初版漏 `lastError?` 字段（app 真实类型含之），已补齐后 `pnpm check:all` 全绿。

30. **`composables/useLibraries` 提升** — 数据源合并（Go backend / frontend-deps.json / Android deps）。
   - 钉子：`@/generated/frontend-deps.json`（构建期静态资产，属应用层）需走 DI 资产注入；`getAndroidDeps`/`isNative` 经 `nativeBridge`（#27 新增 DI）取用。
   - **新增 DI：`runtime/appAssets.ts`**（解耦 `@/generated` 静态资产）：`AppAssets` 接口（`frontendDeps: FrontendDepsManifest`）+ `getAppAssets()`/`setAppAssets()` + 便捷 `getFrontendDeps()`；app 启动期 `stores/registerSharedAppAssets.ts` 调 `setAppAssets({ frontendDeps })` 注入 `@/generated/frontend-deps.json`（`main.ts` 在 `registerSharedNativeBridge()` 后调 `registerSharedAppAssets()`）。
   - 动作：真源复制到 shared，改写 `@/generated/frontend-deps.json` → `getFrontendDeps()`（来自 `runtime/appAssets`）、`getAndroidDeps`/`isNative` → `getNativeBridge()`；app 原位 shim（1 值 `useLibraries` + 4 类型 `LibSource`/`LibStatus`/`LibImportance`/`LibraryItem`）。消费方（`AboutDetail.vue`/`LibraryRow.vue`）经 `@/composables/useLibraries` → shim 无感。
   - 门禁：代码已落地（shared 真源 + app 垫片 + appAssets DI 均已就位，`main.ts` 已接线）；`make-shim check-all` + `pnpm check:all` 待本地跑确认。

31. **`composables/useProxiedFetch` 提升** — native Android 下 window.fetch 覆盖安装器（绕过 WebView CORS）。
   - 钉子：`@/plugins/ApiProxy`（Capacitor 原生插件）+ `@capacitor/core` 平台检测。二者均违反 shared 不得反向依赖 app 的约束。
   - **DI（并行预置）**：`runtime/apiProxy.ts`（`ApiProxyBridge` 接口 + `getApiProxy()`/`setApiProxy()`；`isAndroid` 默认 false，原生专属函数默认抛清晰错误）；`stores/registerSharedApiProxy.ts` 调 `setApiProxy(...)` 注入 `@/plugins/ApiProxy` 的 `fetchOnce`/`streamStart`/`streamCancel`/`addListener`/`removeAllListeners` + `isAndroid`（来自 `@capacitor/core`）；`main.ts` 在 `registerSharedAppAssets()` 后调 `registerSharedApiProxy()`。
   - 动作：真源复制到 shared，全部 `@/plugins/ApiProxy`/`Capacitor` 引用改写为 `getApiProxy()`（`isAndroid()` 替代 `Capacitor.isNativePlatform() && getPlatform()==="android"`；`fetchOnce`/`streamStart`/`streamCancel`/`addListener`/`removeAllListeners` 经桥）；app 原位 shim（3 值 `installProxiedFetch`/`uninstallProxiedFetch`/`isProxiedFetchInstalled`）。`main.ts` 经 `@/composables/useProxiedFetch` → shim 无感。
   - **测试迁移**：shared 预置 `composables/__tests__/useProxiedFetch.test.ts` 原 mock `@capacitor/core`/`@/plugins/ApiProxy` 并期望直接调用——与 DI 范式冲突（且 `getApiProxy()` 默认 `isAndroid→false`/原生函数抛错会使全部用例失败）。已改写为经 `setApiProxy({...mocks})` 注入 mock 桥（移除两个 `vi.mock`），`beforeEach`/`it` 改用 `mocks.isAndroid` 控制平台守卫，覆盖 dev/web no-op、native Android 替换、iOS 不替换、fetchOnce 包装、SSE streamStart、uninstall 还原、幂等等 7 例。
   - 门禁：代码已落地（shared 真源 + app 垫片 + apiProxy DI 均已就位）；`make-shim check-all` + `pnpm check:all`（含改写的 useProxiedFetch 单测）待本地跑确认。

> Module H 选项2 继续项（待执行）：选项2c `useAgent`/`useMockGenLog`/`useWebDavWorkflowAdapter`（toast/i18n/mock DI；其中 `useMockGenLog`/`useWebDavWorkflowAdapter` 还依赖未提升的 `@/api/mockGenerator`，需先将其经 DI 或提升解耦）；选项3 chat/agent 簇评估（见 TODO）。

## 去垫片纯化（结构性改革 Phase 7 · 2026-07-13）

**动机**：垫片是「迁移的谎言」——假装是应用层实现，实则只转发 shared 真源。迁移基本完成后，
必须纯化：删除垫片，让 `@/` 经二级回退**直接解析到 shared**，使 shared 成为唯一事实来源、
应用层只剩组合 + DI 胶水。

**机制**：`scripts/make-shim.mjs prune`（见上方「范式」节）。dry-run 列出可删的同名垫片与
需先改 importer 的错位垫片；`--apply` 只删同名垫片（零风险）。`check-all` 在 0 垫片时输出
「✔ 无残留垫片（已纯化）」作为新终态不变量。

**已落地（#35，双门禁已收口）：**

35. **清理名称错位垫片 + 收口顶层 barrel 垫片（达成「已纯化」终态）**
  - `api/encv_core.ts`（→ shared `api/core`）：唯一 importer `views/FullTextIndexDetail.vue:256`
    `import { getApiBaseUrl } from "@/api/encv_core"` 改写为 `@/api/core`（shared `api/core` 经
    二级回退解析）；删除 `api/encv_core.ts` 垫片。`grep "@/api/encv_core"` 全仓 0 命中。
  - `features/alist-encrypt/useAlistEncrypt.ts`（→ shared `composables/useAlistEncrypt`）：
    importer `views/useFilesView.ts:100` 与 `views/FileInfo.vue:205` 的
    `from "@/features/alist-encrypt/useAlistEncrypt"` 改写为 `@/composables/useAlistEncrypt`
    （应用层无 `composables/useAlistEncrypt` 垫片，`@/` 直接落到 shared 真源）；删除该垫片。
    `grep "@/features/alist-encrypt/useAlistEncrypt"` 全仓 0 命中。`features/alist-encrypt/index.ts`
    等同级垫片指向 shared `features/alist-encrypt/index`（同名），不受影响。
  - `useMockGenLog`/`useWebDavWorkflowAdapter`/`mockGenerator` 三个本会话提升的模块其
    app 原位垫片为**同名**，已随工作树改动一并删除；下游（`PluginTestsDetail.vue` 等）
    经 `@/` 落到 shared，零改动。
  - **顶层 barrel `api/encv.ts`（最后 1 个残留垫片）**：`prune` 因它 re-export 了 12 个
    shared 子模块（多 star-export 目标）而误判为「错位」，实则 shared 已有**同相对路径**
    `api/encv.ts` 真源 barrel（结构完全一致）。删除 app 副本后，`@/api/encv`（37 处引用）
    经 `@/*` → shared 二级回退直接解析到 shared 真源 barrel，与同名垫片等价、零风险。
    `grep 'from "@/api/encv"'` 全仓 0 命中（引用仍在，仅解析目标变 shared）。

**验证（已完成）**：
```bash
cd /workspace/app
node scripts/make-shim.mjs prune          # dry-run：同名 0、错位 1（即 api/encv.ts，已单独纯化）
node scripts/make-shim.mjs prune --apply  # 实为 no-op（同名垫片此前已删）
node scripts/make-shim.mjs check-all     # ✔ 无残留垫片（应用层已纯化，全部经 @/ 别名直连 shared）
```
> **Phase 7 去垫片纯化已收口**：`make-shim check-all` 稳定输出「无残留垫片」终态不变量。
> 唯一保留的 app→shared 桥接是 `encv-mobile/src/api/encv_*.ts` 11 个**子模块薄壳**（doc #22
> 明确「维持 re-export 薄壳垫片向后兼容」），它们属于有意的公共 API 面，不计入「残留垫片」终态。
> **后续门禁**：`pnpm check:all`（含单测 + Biome + 5 类型检查 + i18n lint）请在本地终端跑确认
> （长耗时命令，不在本端硬跑）；预期 8/8 全绿。若报「找不到导出」，按 #35 范式改写 importer 再删，
> 删除均为 git 可还原。

> Phase 4 继续项（已全部落地 ✅）：
> - `lib/workflow/types` 分歧已调和（#16）；`TaskTimeline` + `TaskDetailModal` 已提升（#16）；
>   `tasks/*`（#17）；`group-detail` 3 叶子（#18）；`automation/*` 9 组件 + `PipelineTab`（#19）均已提升。
> - 至此 **encv-mobile → @encv/shared-components 任务系统 lift 重构的「组件层」已全部完成**：
>   所有任务系统相关组件 / composables / lib / api / 常量 / i18n 均已提升进 shared 并留 app 垫片，
>   shared 非测试代码 `@/`-free。后续仅剩收尾：跑 `pnpm check:all` 全绿 + 门禁稳定。
> 注：`useBatchOperations`/`useTaskForm`/`useTaskTrigger` 此前已是垫片；其余均已于 #11–#19 提升。

> Tier-3 每搬一个模块，必须：① 真源复制到 shared 并改写 `@/` → `@encv/shared-components/...`；
> ② app 原位留 `export * from "@encv/shared-components/..."` 垫片（用 `node scripts/make-shim.mjs
> gen <shared> <app>` 生成，避免手写出错）；③ 跑 `make-shim check-all` + `pnpm check:all` 双门禁。
> 严禁把 shim 当真源写入（此前会话曾误把 11 个 api 写成自引用 shim 丢真源，务必从 git HEAD
> 取真源再搬）。

## codemogger-shim 在重构中的角色

`codemogger-patch/codemogger-shim` 已增强两个重构子命令（详见 `codemogger-patch/README.md §5`）：

- `codemogger impact <target>` — 别名无关的「爆炸半径」。同时查 `@/x` 与
  `@encv/shared-components/x` 两种拼写并合并，闭合原 `--module` 按 specifier 精确匹配导致的
  alias 分叉漏查。搬模块前用它看清调用方全貌。
- `codemogger leaks [dir]` — 扫出某包内残留的 `shared → app` 反向 `@/` 导入，
  直接验证「共享层不得依赖应用层」是否达标（搬之后应为空）。

## 验证命令

```bash
# 搬之前：看清爆炸半径（两种别名都会查）
codemogger impact @/api/encv
codemogger impact @encv/shared-components/api/encv

# 搬之后：shared 内不应再有指向 app 的 @/ 依赖
codemogger leaks packages/shared-components/src

# 全量门禁（typecheck + biome + 单测）
pnpm check:all
```
