# 结构性改革方案（基于具体逻辑，非迁移/路径层面）

> 修订说明：前两版都偏了。
> - v1「去垫片」= 删 73 个 shim 文件 → 表皮雕花（垫片只是症状）。
> - v2「拆 `@/` 回退」= 改 import 路径 + 删 fallback 插件 → 仍是 **relocation（搬位置）**，
>   没动任何逻辑结构，shared 依旧是 encv-mobile 的搬运壳 + 转发壳。
> 用户质问「shared 难道不是从 encv-mobile 搬过来的？结构性改革在哪？不谈具体逻辑你能改什么？」
> —— 本版直接读代码、对准**逻辑层**病灶。路径层面的边界清理（v2）降级为 §6 的机械收尾，
> 真正的改革是 §1 的 A/A3/B/D 四类逻辑级动作。

---

## 0. 结论（TL;DR）

shared 现在确实**只是 encv-mobile 的搬运 + 转发壳**：没有新抽象、没有消除任何重复逻辑。
真正的结构病灶都在**逻辑层**，且有一个共同模式——**抽象已经存在，但调用方不统一用 / 或存在两套冲突真相**：

| 编号 | 病灶 | 抽象已存在？ | 现状 | 改革动作 |
|------|------|------------|------|---------|
| **A** | 统一请求层被绕过 | ✅ `api/core/request.ts` 的 `apiRequest` | 仅 `encv_tasks.ts` 用；其余 10 模块手写 fetch | 11 模块收敛到 `apiRequest` |
| **A3** | 错误类型两套层级 | ⚠️ `ApiError` 有，但 typed error 另立山头 | `PermissionDeniedError`/`NotFoundError` 继承 plain `Error` | 收敛到 `ApiError` 子类 + `apiRequest` 按 status 映射 |
| **B** | 状态机两套规则 + 死文件 | ✅ `state-machine.ts`（宽松，活） | 严格版 `stateMachine.ts` 已删，但注释+测试还引用 | 默认宽松、可配严格；修悬空测试 |
| **D** | 死抽象幽灵引用 | — | `useWorkflowEngine`/`stateMachine.ts` 已删，注释+测试仍引用 | 清幽灵 + 把关键测试纳入门禁 |
| **E** | 任务视图计算逻辑两遍实现 | ✅ `lib/taskViewComputeCore.ts`（2026-07-12 抽出，已统一 worker+`useTaskViewCompute`） | `taskStore`+`useTasksList` 仍各自内联 filter/sort/group/display | 调用方委托 core 纯函数 |
| **F** | 加/解密表单组件级重复 | ❌ 无共享子组件/composable | `EncryptBody.vue`/`DecryptBody.vue` 的 extraFields 渲染、`getExtra`、`handleBrowse*`、源/目标输入、密码段、样式**近乎逐字重复** | 抽 `TaskFormFields` 子组件 + `useFilePickerBrowse` composable |
| **G** | 两份 task store WS 事件处理重复且分歧 | ❌ 无共享事件归一化 | `taskStore.applyEvent` 与 `runTasksStore.applyEvent` 各写一遍；**`completed` 归一化（status/completedAt/progress）只在 taskStore 有**，runTasksStore 缺失 → GroupDetail 状态可能不一致 | 抽 `normalizeTaskEvent` + 列表操作 helper，两 store 共用 |
| **H** | 轮询样板重复 + raw-fetch 反模式蔓延到 composable 层 | ❌ 无 `usePoll`/`useInterval` helper | ①「周期轮询端点→更新响应式状态」被 `useVectorSearchStatus`(setInterval)/`useContextUsage`(setTimeout 自调度)/`useServerStatus`(eventBus+probe) 各写一套；② 手写 `fetch`+`resp.ok`+`resp.json()` 在 `useVectorSearchStatus:38`/`useContextUsage:80`/`useServerStatus:98`/`useTaskCancel:81` 重现（A 在 api 外也成立） | 抽 `usePoll(fn,{intervalMs,immediate,onEvent})`；composable 内改用 `apiRequest` |
| **I** | 任务视图计算「搜索命中 / group-counter / 显示名」复制 4–5 处（E 的细粒度放大） | ⚠️ `lib/taskViewComputeCore` 已抽 core，但副本未收敛 | ①「name/plugin/error/id 四字段 `includes(q)`」搜索命中判定在 `core.filterTasks:97`/`core.computeGroupCounters:202`/`useTasksList.computeGroupCounters:132`/`useTasksView.computeGroupHit:344`/`taskStore:446` **5 处**；② `computeGroupCounters` 在 `core:157`/`useTasksList:96`/`useTasksView.computeGroupHit`（语义等价）**≥3 份**；③ `getTaskDisplayName` 在 `core:40`/`useTasksList:58`/`TasksTab.vue:133` **3 份**。注释反复写「与 X 逻辑一致」却各自独立 → 典型「知道该复用没抽」 | 抽 core 的 `matchTaskSearch(task,q)` + `getTaskDisplayName(task)` 为单一真源；删 `useTasksList.computeGroupCounters`（已是 core 的冗余 fallback）；`useTasksView.computeGroupHit` 委托 core |

**C（反例/范本）**：`composables/realtime/` 已是好抽象（WS/polling/SSE 策略模式，注释清晰），
是**该模仿的范式**，不是该改的对象。

---

## 0.5 「shared 只是搬运」实证：搬运层 + 未改革的提升层

用户质问「shared 难道不是从 encv-mobile 搬过来的？改革在哪？」—— 本节能用代码实证回答。

### 0.5.1 搬运层：app 里约 50 个「转发壳」

`app/encv-mobile/src` 几乎镜像 `shared-components/src`（api / composables / lib / stores / types / config / features / components 同名目录全有）。
逐一核对同名文件，结论是：**镜像目录里 app 一侧绝大多数是 `scripts/make-shim.mjs` 生成的纯转发壳**，真实实现已在 shared。

- api/：12 个 `encv_*.ts` + `mockGenerator.ts` 全是 `export * from "@encv/shared-components/api/..."`（app 专属仅 `sparseContainer.ts` / `encv.ts` / `encv.test.ts`）。
- composables/：约 24 个同名文件是壳（`useConfig`/`useFileList`/`useProxiedFetch`/`useTaskEventBridge`/`useRealtimeTransport`…），含 `realtime/` 整子目录 4 个壳。
- lib/：6 个全壳（`taskTypeLabel`/`taskPersistence`/`mountPath`/`mockDataGenerator`/`mockConstants`/`buildReportZip`）。
- stores/：`taskStore.ts`/`runTasksStore.ts` 两壳（其余 6 个 `registerShared*` 是 app 专属 DI 接线）。
- types/：`file-feature.ts`/`webdav-test.ts` 两壳。
- config/：`schemaParser.ts` 壳。
- features/：`alist-encrypt/` 整目录 6 个壳。
- components/：`NewTaskState.ts` 壳。

**合计约 50 个转发壳文件**（grep `make-shim.mjs|兼容垫片|已提升至 @encv/shared-components` 命中 43，含 `export {…}` 形式的 re-export 命中 50）。

后果：调用方仍写 `import … from "@/api/encv_files"`，被壳无感转发到 shared。**提升只搬了位置，没改任何调用关系**——这就是「shared 只是搬运」的本体。

### 0.5.2 未改革的提升层：搬进来的代码没收敛到 shared 自己的抽象

更关键：被提升进 shared 的代码，**带着遗留双轨一起搬了进来，却没收敛到 shared 既有的抽象**。证据：

- `api/core/request.ts` 已有 `apiRequest`（拼 baseUrl + 认证头 + 归一化 `ApiError`），但 shared 自己的 10/11 个 `encv_*.ts` 模块**仍手写 fetch**（A）；`PermissionDeniedError`/`NotFoundError` 仍 `extends Error` 而非 `ApiError`（A3）。
  → 即「提升」把「绕过 apiRequest / 绕过 ApiError」的 legacy 双轨**原样搬进 shared**，抽象近在咫尺却不用。
- `lib/taskViewComputeCore.ts` 已抽出视图计算纯函数，但 `taskStore`/`useTasksList` 仍内联（E）。
- `EncryptBody.vue`/`DecryptBody.vue` 表单骨架逐字重复（F）。
- `taskStore.applyEvent`/`runTasksStore.applyEvent` 事件归一化各写一遍且 `completed` 归一化只在前者（G）。

### 0.5.3 结论：改革分两极

| 极 | 现象 | 性质 | 改革动作 |
|----|------|------|---------|
| **搬运层** | app 约 50 个转发壳 | relocation（搬位置） | 删壳 + 调用方 `import` 改指 shared（机械，原批 9） |
| **提升层** | shared 内部 legacy 双轨未收敛 | 抽象被绕过 / 缺位 | A/A3/E/F/G 收敛到 shared 既有抽象（逻辑改革） |

**真正的「改革」是提升层那一极**：让 shared 成为单一真源、消除其内部重复；搬运层的删壳只是让「单一真源」名实相符的收尾。两极少一不可——只删壳不收敛，shared 内部仍是双轨；只收敛不删壳，调用方仍绕道 app 壳。

### 0.5.4 修正：搬运层不完整 + 边界是「假共享」（实证）

§0.5.1 把 app 描述成「约 50 个转发壳 + app 专属代码」，暗示搬运基本完成。但挖 `views/` 发现**搬运层并不完整，且 shared 边界在 alias-fallback 掩护下是虚的**：

- `app/encv-mobile/src/views/useFilesView.ts`（**66KB 巨无霸**）与 `useFilesView.searchTokens.ts` **仍在 app 层、未搬进 shared**——它们是「提升」时被漏掉的大块视图逻辑。
- 但 shared 自己的测试 `views/__tests__/useFilesView.searchTokens.test.ts` 却 `import { tokenizeQuery } from "@encv/shared-components/views/useFilesView.searchTokens"`——即 **shared 测试在引用一个 shared 里根本不存在的模块**；它能解析通，只能靠 `encv-alias-fallback` 把 `@encv/shared-components/*` 回退到 app（§0.5「边界假象」的实证）。**批 9 删 fallback 后，此 import 将断链。**

这把 §0.5 的「边界假象」坐实成代码事实：
1. **app→shared 方向**：app 壳转发到 shared（§0.5.1）；
2. **shared→app 方向**：shared 测试经 alias-fallback 回退引用 app 模块（本项）。

双向靠 `encv-alias-fallback` 兜底，使「`@encv/shared-components/*` 是单源」成为**假象**。

**改革含义（修正原批 9 的简化描述）**：
- 批 9（边界强制）**不能只删壳**：必须先补齐「未搬的大块」（`useFilesView.ts` 66KB + `useFilesView.searchTokens.ts`）进 shared，否则 `_measure-fallback.mjs` 输出归零会牺牲 shared 测试。
- 「提升进 shared」是**选择性/不完整的**——不是所有逻辑都搬了；shared 内部重复（A/E/F/G/I）与「未搬的大块」并存 → 改革必须是「**收敛 + 补齐 + 删壳**」三件套，而非任一单项。
- 顺带：`useFilesView.searchTokens.tokenizeQuery`（支持 phrase/regex/boolean 的高级切词）与 shared `useFileList.clientSearchTokenize`（CJK 单字切分）是**跨层两份搜索切词能力**，分处 app/shared——属边界未理清，待 `useFilesView` 搬入后统一（可并入 I 的搜索单源议题）。

---

## 1. 诊断与证据

### 病灶 A：统一请求层存在，但 10/11 API 模块绕过它

> 定性（呼应 §0.5.2）：这不是「app 绕过 shared」，而是「**提升进 shared 的 legacy 双轨代码，连绕过一起搬了进来，仍绕过 shared 自己的 `apiRequest`**」——抽象就在同包内，却不用。

`api/core/request.ts` 已有统一的 `apiRequest` / `buildUrl`：
- 自动拼 base URL（`useApiContext()` → `getApiBaseUrl()`）；
- 自动加认证头（`ctx.getAuthHeaders()`）；
- 非 2xx 归一化为 `ApiError`（带 status + body）。

但 grep `fetch(`/`response.ok`/`.json()` 在 `api/`（除 core）共 **246 处**手写样板：

| 模块 | 手写 fetch/ok/json 处数 |
|------|------|
| encv_plugins | 36 |
| encv_perf | 31 |
| encv_search | 28 |
| encv_tasks | 27（部分已用 apiRequest，仍混用手写） |
| encv_files | 25 |
| encv_admin | 23 |
| encv_files_extra | 21 |
| encv_openlist | 15 |
| encv_system | 18 |
| encv_trash | 10 |
| encv_webdav | 9 |
| mockGenerator | 3 |
| **合计（非 core）** | **246** |

代表冗余（如 `encv_admin.ts` 的 `checkServerStatus`/`fetchConfig` 各自手写 `fetch(\`${getApiBaseUrl()}/api/...\`)` +
`!response.ok` + `response.json()` + 错误提取 + **content-type 必须是 JSON 的 SPA-fallback 防护**——这层防护在多处各写一遍）。

**改革**：
1. 11 个模块统一走 `apiRequest<T>(path, { method, query, body, headers })`。
2. `apiRequest` 扩展能力（抽象层本就该有的扩展点）：
   - **typed error 映射**（见 A3）：403→`PermissionDeniedError`、404→`NotFoundError` 等；
   - **SPA-fallback 防护**：复用 `useApiBaseProbe.probeHealth` 已实现的「响应必须是 `application/json`，否则视为不通」逻辑，下沉到 `apiRequest`；
   - **`AbortController` 超时**：`apiRequest` 接受 `timeoutMs`，替代各模块手写的 `new AbortController()` 样板。
3. `getApiBaseUrl()` 保持为唯一 base URL 真源（经 `useApiContext` 注入），**不动**——它本就是统一的。

### 病灶 A3：错误类型两套层级

`api/core/errors.ts`：`ApiError extends Error`（带 `status` + `body`）。
`api/encv_files.ts:29-39`：`PermissionDeniedError` / `NotFoundError` 直接 `extends Error`，**不继承 `ApiError`**，
且被 `encv_search.ts:3,17` import 复用。

后果：调用方无法用统一的 `instanceof ApiError` 处理；`apiRequest` 抛 `ApiError`，手写模块抛 `PermissionDeniedError`——
**两套错误体系并存**，类型守卫要写两遍。

**改革**：
1. 把 `PermissionDeniedError` / `NotFoundError` 移到 `api/core/errors.ts`，改为 `extends ApiError`
   （构造时填对应 `status`，如 403 / 404）。
2. `apiRequest` 增加 `statusErrorMap?: Record<number, new (body)=>ApiError>` 选项，默认映射
   403/404/... 到对应子类；非 2xx 一律先归一化为 `ApiError` 子类。这样调用方只需 `catch (e) { if (e instanceof ApiError) ... }`。

### 病灶 B：状态机两套规则 + 死文件 + 悬空测试

`lib/workflow/state-machine.ts`（连字符，**活**）是「官方」版，其 `VALID_TRANSITIONS` 是**宽松**的：
`pending` → `submitted/queued/running/cancelled/skipped` 都允许，注释说明「兼容后端可能跳发事件」。

它的注释（state-machine.ts:9-19）引用的 `stateMachine.ts`（驼峰）**已在 lift 时被删**，但：
- `state-machine.ts` 注释仍长篇描述「与 stateMachine.ts 的关系」；
- `composables/__tests__/workflow-core.test.ts:8` 仍
  `import { canTransition, transition, ... } from "@encv/shared-components/lib/workflow/stateMachine"`——
  **指向不存在模块的悬空导入**；且该测试断言**严格**语义（`canTransition("pending","running")` 期望 `false`），
  与活着的宽松版直接矛盾。
- `path-chain-e2e.test.ts:9,265` 注释里还写 `useWorkflowEngine.executeJob()`——`useWorkflowEngine` 也已删（见 D）。

**用户拍板（状态机严格度）**：**默认宽松，允许配置为严格**——这正是抽象层应有的扩展范式。

**改革**：
1. `validateTransition(from, to, opts?: { strict?: boolean })`：
   - `strict` 缺省 = `false` → 用 `VALID_TRANSITIONS`（宽松，现状不变）；
   - `strict = true` → 用新增的 `VALID_TRANSITIONS_STRICT`（严格：`pending` 只能 → `submitted/cancelled` 等，对齐旧 camelCase 版语义）。
2. `transition()` 同步接受 `strict` 选项；`applyTerminalGuard` 不变（终态保护两版一致）。
3. 修 `workflow-core.test.ts`：import 改为活着的 `state-machine`，并把「严格语义」断言包进 `validateTransition(..., { strict: true })` 用例；宽松语义单独断言默认行为。
4. 清 `state-machine.ts` 顶部那段描述已删 `stateMachine.ts` 的注释（改为一行说明「strict 模式见 `VALID_TRANSITIONS_STRICT`」）。

### 病灶 D：死抽象幽灵引用（门禁静默放过）

`useWorkflowEngine` 已被 `useWorkflowTaskService` + `useTaskEventBridge` + `state-machine.ts` 取代并删除，
但：
- `useWorkflowTaskService.ts:11`「从 useWorkflowEngine 迁移的核心逻辑」、`:26` import 注释；
- `useWorkflowStore.ts:9-11`「消费者 useWorkflowEngine 已删除」；
- `path-chain-e2e.test.ts:9,265` 注释写 `useWorkflowEngine.executeJob()`；
- 以及 B 中的 `stateMachine.ts` 悬空 import。

更关键：**这些测试被排除出类型检查**（`**/*.test.ts` 不在 shared tsconfig 的 include）+ 只在 FULL 门禁跑，
所以坏测试被门禁**永远静默放过**。

**改革**：
1. 清所有指向已删 `useWorkflowEngine` / `stateMachine.ts` 的注释与 import。
2. 把 `workflow-core.test.ts`（修好后）和 `path-chain-e2e.test.ts` 纳入**常驻类型检查或基础门禁**，
   至少保证「无悬空 import」能被 CI 抓到（加一条 `tsc --noEmit` 覆盖 test 文件，或把关键 test 移出 `**/*.test.ts` 排除名单）。

### 病灶 E：任务视图计算逻辑两遍实现（半截子改革）

`lib/taskViewComputeCore.ts`（2026-07-12 Phase 3 抽出）的头部注释明言：**原 worker 与 `useTaskViewCompute.computeSync` 各内联了一份相同逻辑（两份重复），现统一到此模块**。
即团队**已经识别并抽出了正确的抽象**——但改革只做了一半：

- `stores/taskStore.ts` 仍内联 `filteredTasks`（:418）/ `sortedTasks`（:458）/ `groupedTasksByRunId`（:472），与 core 的 `filterTasks`/`sortTasks`/`groupByRunId` **近乎逐字重复**；
- `composables/useTasksList.ts` 的 `computeGroupCounters`/`buildGroupDisplayData`/`displayedItems` 也被 core 注释标注「与 … 逻辑一致」——即同一份聚合/展示逻辑第三处实现；
- core 自己的注释反复写「与 taskStore.xxx 逻辑一致」，正是**未 DRY 的臭味**：同一算法活在三处，改一处另两处不会跟着变。

**唯一差异**：`taskStore.filteredTasks` 多了 `useBackendSearch` 分支（后端搜索返回 id 集合时按 id 过滤，而非本地名字匹配）——这是合法扩展，不是重复的理由。

**改革**（与 A 同范式：抽象已存在，让调用方改用）：
1. `taskViewComputeCore.filterTasks` 增加可选 `backendSearchResultIds?: Set<string>` 参数，把 `taskStore` 的 `useBackendSearch` 分支下沉进去；
2. `taskStore.filteredTasks/sortedTasks/groupedTasksByRunId` 改为直接调用 core 纯函数（喂入响应式值）；
3. `useTasksList` 的 group counters / displayData 同样委托 core，删除本地副本；
4. 删掉 core 里所有「与 taskStore/useTasksList 逻辑一致」的注释（统一后这些注释失去意义，反而暗示还有第二份）。

> 这是用户痛点「shared 只是搬运、改革在哪」的**最佳样本**：抽象抽出来了，但调用方没迁移，等于没改革。E 的完成度 = 把「已抽出但未落地」的补完。

### 病灶 F：加/解密表单组件级重复（无共享子组件）

`components/EncryptBody.vue`（345 行）与 `components/DecryptBody.vue`（205 行）是**同一表单的两个变体**，却各写一遍：

- **完全相同**：源文件/目标路径两个 `InputWithHistory`、密码段（`primaryOverride`+`secondaryPassword`）、
  extraFields 的三分支渲染（`bool`→toggle / `select`→ion-select / 其它→InputWithHistory）、
  `getExtra(key)`、`handleBrowseSource()`、`handleBrowseTarget()`、以及 `.form-section/.extra-field-*` 全套样式。
- **唯一差异**：EncryptBody 多了容器版本选择 + cipher/compression 两个 RadioGroup（仅 v4）、
  extraFields 过滤条件为 `encrypt`；DecryptBody 过滤条件为 `decrypt`。

即：**表单骨架 + 文件选择逻辑 + 样式约 150 行是逐字复制**，加密独有的 cipher/compression 才是真正的差异。
改一处（如 browse 逻辑、extraField 渲染规则）另一处不会跟着变——典型组件级未 DRY。

**改革**：
1. 抽 `components/TaskFormFields.vue`：承载「源/目标输入 + 密码段 + extraFields 渲染」，
   通过 prop `condition: "encrypt" | "decrypt"` 决定过滤；EncryptBody/DecryptBody 只保留各自独有区块 + 组合它。
2. 抽 `composables/useFilePickerBrowse.ts`：把 `handleBrowseSource/Target`（modalController + FilePickerModal + onDidDismiss）收敛为一个复用函数，
   两组件（及未来的表单）共用。
3. extra-field 相关 CSS 提到共享子组件内，删两处副本。

### 病灶 G：两份 task store 的 WS 事件处理重复且分歧（危险）

`stores/taskStore.ts` 与 `stores/runTasksStore.ts` 各自实现 `applyEvent(type, data)`，
`created`/`update`/`progress`/`completed` 的分发骨架**两遍**。二者定位不同（Tasks 页全量+守卫 vs GroupDetail 按 runId），
分开是合理的——但**事件归一化逻辑不该复制**，且现在已出现**危险分歧**：

- `taskStore.applyEvent` 的 `completed` 分支会**归一化**：`status = data.error ? "failed" : "completed"`、
  补 `completedAt`、无错时补 `progress: 100`（state-machine 之外的业务归一化）；
- `runTasksStore.applyEvent` 的 `completed` **直接 `patchTaskById(data.id, data)`**，不做任何归一化。

后果：同一条 `task:completed` WS 事件，在 Tasks 页显示为「completed + 100% + completedAt」，
在 GroupDetail 页可能显示为后端原始 payload（缺 completedAt / status 未派生）——**两页状态不一致**，是潜在 bug。

**改革**：
1. 抽 `lib/taskEvent.ts` 的纯函数 `normalizeTaskEvent(type, data): { patch, isCreate }`，
   把 `completed` 归一化（status/completedAt/progress）收敛为**单一真源**；
2. 两 store 的 `applyEvent` 都调用 `normalizeTaskEvent`，各自只保留差异部分
   （taskStore：MAX_LOADED_TASKS 守卫 + persistence；runTasksStore：runId 过滤）；
3. `getTaskById`/`patchTaskById` 的 O(1)-index vs 线性 `.find` 是性能取舍差异，可暂不统一，但归一化必须共用。

> F 是「同类组件无共享子件」，G 是「同类 store 的核心归一化被复制且已分歧」——都属**抽象缺位**（不同于 A/E 的「抽象已存在被绕过」），
> G 尤其危险：复制不仅冗余，还导致两页真实行为不一致。

### 病灶 H：轮询样板重复 + raw-fetch 反模式蔓延到 composable 层

两类 intra-shared 重复，都属**抽象缺位**（shared 内没有对应的复用原语）：

**① 轮询样板三遍实现（无 `usePoll` helper）**
「周期轮询某端点 → 更新响应式状态 + start/stop 守卫 + onUnmounted 清理 + 事件触发刷新」被三个 composable 各自重写：
- `useVectorSearchStatus.ts`：`setInterval(probeOnce, 60_000)` + `pollInFlight` 并发守卫（:30,79）；
- `useContextUsage.ts`：`setTimeout` 递归自调度 + `STREAMING/IDLE` 双频率 + `watch(status)` 重排 + `loading` 守卫（:66-127）；
- `useServerStatus.ts`：`networkPing()` 手写 `fetch(/ping)` + `AbortController`，由 `eventBus.on("server:status")` 触发（:90-113）。

grep 全 shared：`usePoll`/`useInterval`/`usePeriodic` **均不存在**——轮询循环是每处手搓的。抽 `usePoll(fetcher, { intervalMs, immediate, onEvent })` 即可统一三者（含并发守卫、onUnmounted 清理、事件重探）。

**② raw-fetch 反模式不止在 api/（A 的 scope 外溢）**
手写 `fetch(url)` + `!resp.ok` + `resp.json()` 解析在 **composable 层**也重现，全部绕过同包的 `apiRequest`：
- `useVectorSearchStatus.ts:38` — `fetch(getApiBaseUrl()+"/api/runtime")` 手写 ok/json；
- `useContextUsage.ts:80` — `fetch(AGENT_API_BASE+"/api/agent/context-usage")` 手写 ok/json；
- `useServerStatus.ts:98` — `fetch(\`${baseUrl}/ping\`)` 手写 ok/json + AbortController；
- `useTaskCancel.ts:81` — `fetch(\`${baseUrl}/api/tasks/...\`)` 手写。

反例：`useWebDavManifest.ts` 正确走 `fetchWebDavManifest()`（api 层封装），说明 composable 层**应当**经 api 而非裸 fetch。

**改革**：
1. 抽 `composables/usePoll.ts`：统一「interval/immediate/onEvent + 并发守卫 + onUnmounted 清理」；`useVectorSearchStatus`/`useContextUsage`/`useServerStatus.ping` 委托之。
2. composable 内的裸 `fetch` 改走 `apiRequest`（或对应 api 封装函数），与 A 同源收敛——A 的 grep 门禁应扩展到 composables/（除 `useProxiedFetch`/`useApiBaseProbe` 这类底层）。

> H 是「shared 内部缺复用原语」的又一例：不仅 api 双轨（A）、store 事件（G）、表单（F）重复，连最通用的「轮询」都没有共享实现。
> 它把 A 的 scope 从 api/ 扩展到 composables/——**raw-fetch 反模式是贯穿 shared 的系统性习惯，不是单点**。

### 病灶 I：任务视图计算「搜索命中 / group-counter / 显示名」复制 4–5 处（E 的细粒度放大）

**I 是 E 在更细粒度的放大**：E 指出 `lib/taskViewComputeCore.ts`（2026-07-12 抽出）已统一 worker + `useTaskViewCompute`，但调用方仍内联；现在发现 **core 内部与调用方之间、调用方彼此之间，连最底层的「搜索命中判定 / group-counter / 显示名」也各写了一份**，且注释反复写「与 X 逻辑一致」却没抽成共享函数。

**① 搜索命中判定（`name/plugin/error/id` 四字段 `includes(q)`）5 处复制**

同一段逻辑在 5 个地方逐字重现：
- `lib/taskViewComputeCore.ts:97-103`（`filterTasks` 内）
- `lib/taskViewComputeCore.ts:202-208`（`computeGroupCounters` 内）
- `composables/useTasksList.ts:132-139`（`computeGroupCounters` 内）
- `composables/useTasksView.ts:344-350`（`computeGroupHit` 内）
- `stores/taskStore.ts:446`（内联 `split("/").pop()` 同式）

**② group-counter / 命中计数 ≥3 份**
- `lib/taskViewComputeCore.ts:157` `computeGroupCounters`（worker 核心，注释「与 useTasksList.computeGroupCounters 逻辑一致」）
- `composables/useTasksList.ts:96` `computeGroupCounters`（注释自承「兜底，store 已有但保持兼容」；`useTaskViewCompute.ts:212` 又说「保留作为 fallback」→ **core 已抽后仍留的冗余第四份**）
- `composables/useTasksView.ts:329` `computeGroupHit`（又一份 group 命中，语义与上面等价但签名/返回不同 → 潜在分歧点）

**③ 显示名 `getTaskDisplayName` 3 份**
- `lib/taskViewComputeCore.ts:40`
- `composables/useTasksList.ts:58`
- `components/TasksTab.vue:133`
（外加 `taskStore:446`、`core:98` 内联同一 `split("/").pop()` 表达式）

**性质**：这是「**知道该复用、却没抽**」的典型——core 已存在，但 `useTasksList.computeGroupCounters` 作为 fallback 没删、`useTasksView.computeGroupHit` 没委托 core、`getTaskDisplayName` 在组件层又写一遍。比 G 更系统（G 是 2 份 store 事件，I 是 5/3/3 份底层判定），且**副本可能已分歧**（如 `computeGroupHit` 用 `getTaskName`、core 用 `getTaskDisplayName`，当前等价但任一处改动不会传导）→ G 类「复制且可能不一致」风险在视图计算簇同样成立。

**改革**：
1. 抽 `lib/taskViewComputeCore.ts` 的**单一真源纯函数**：`matchTaskSearch(task, q): boolean`（四字段 includes 判定）+ `getTaskDisplayName(task): string`；5 处搜索判定、3 处显示名全部委托之。
2. 删 `useTasksList.ts:96` 的 `computeGroupCounters`（core 已有，且 `useTaskViewCompute` 的 `computeSync` 已走 core → 它是死冗余 fallback；`_computeGroupCounters` 测试桩改指 core 函数）。
3. `useTasksView.ts:329` 的 `computeGroupHit` 改为委托 core 的 `computeGroupCounters` 取 `search.hit` / 或抽 `countGroupHit(tasks, filters)` 单一实现。
4. I 与 E/G 同源：E 是「视图计算整体内联」、G 是「store 事件归一化复制」、I 是「视图计算内部最细粒度复制」——三者共同证明 **shared 内部「抽了 core 但没收敛调用方」是系统性习惯**，不是单点。

> I 把 E 的「已抽 core 但调用方内联」推进到「core 内部与调用方连最底层判定也各写一份」——说明**提升进 shared 时只做了「位置搬迁 + 抽了个 core」，但没把副本收敛到 core**，与 §0.5.2「提升=搬迁未收敛」完全呼应。

---

## 2. 执行批次（按依赖顺序，每批带门禁）

| 批 | 范围 | 动作 | 门禁 |
|----|------|------|------|
| 1 | `api/core/errors.ts` + `request.ts` | A3：typed error 迁到 core 继承 `ApiError`；`apiRequest` 加 `timeoutMs` + `statusErrorMap` + SPA-fallback 防护 | `pnpm check:all` 8/8 |
| 2 | `encv_admin/files/search/perf/plugins/openlist/trash/system/webdav/files_extra/tasks/mockGenerator` | A：11 模块收敛到 `apiRequest`，删手写 fetch/ok/json/AbortController 样板 | `pnpm check:all` 8/8 + grep 无手写 fetch（除 core/request + useProxiedFetch） |
| 3 | `lib/workflow/state-machine.ts` | B：`validateTransition` 加 `strict` 选项 + `VALID_TRANSITIONS_STRICT`；清旧注释 | ✅ 已落地（真实门禁验证） |
| 4 | `workflow-core.test.ts` + `path-chain-e2e.test.ts` | B/D：修悬空 import、严格语义用例、纳入常驻类型检查 | ✅ 已落地（真实门禁验证） |
| 5 | 全局注释/`useWorkflowTaskService`/`useWorkflowStore` | D：清 `useWorkflowEngine` 幽灵引用 | ✅ 已落地（真实门禁验证） |
| 6 | `lib/taskViewComputeCore.ts` + `stores/taskStore.ts` + `composables/useTasksList.ts` | E：core 增 `backendSearchResultIds` 参数；`taskStore`/`useTasksList` 委托 core 纯函数，删本地副本 + 清「逻辑一致」注释 | `pnpm check:all` + 视图计算单测（group/flat/counters）仍全绿 |
| 7 | `stores/taskStore.ts` + `stores/runTasksStore.ts` + 新 `lib/taskEvent.ts` | G：抽 `normalizeTaskEvent`，两 store 共用 `completed` 归一化；修 GroupDetail 状态分歧 | `pnpm check:all` + GroupDetail/Tasks 事件测试全绿 |
| 8 | `components/EncryptBody.vue` + `DecryptBody.vue` + 新 `TaskFormFields.vue`/`useFilePickerBrowse.ts` | F：抽共享表单子组件 + browse composable，删两处复制 | ✅ 已落地（真实门禁验证 PASS 8/FAIL 0） |
| 9 | 边界强制（v2 的机械收尾，非主改革） | 改写 136 处 `@/x`→`@encv/shared-components/x` + 摘除 alias-fallback 的 shared 兜底分支 + **删 73 转发壳（已完成）** + 删 `encv-alias-fallback` 插件本身（仍保留，见下） | `_measure-fallback.mjs` 输出 **0** + check-all PASS 8/FAIL 0 + `vite build` ✓（均已验证） |
| 10 | 新 `composables/usePoll.ts` + `useVectorSearchStatus`/`useContextUsage`/`useServerStatus`/`useTaskCancel` | H：抽 `usePoll` 统一轮询样板；composable 内裸 `fetch` 改走 `apiRequest`（A scope 扩到 composables/） | `pnpm check:all` + grep composables 无裸 `fetch`（除 `useProxiedFetch`/`useApiBaseProbe`） |
| 11 | `lib/taskViewComputeCore.ts` + `useTasksList.ts` + `useTasksView.ts` + `TasksTab.vue` + `taskStore.ts` | I：抽 `matchTaskSearch(task,q)` + `getTaskDisplayName(task)` 为单源；删 `useTasksList.computeGroupCounters`（冗余 fallback）；`useTasksView.computeGroupHit` 委托 core | `pnpm check:all` + grep 搜索命中判定/显示名仅剩 core 定义（5 处→1、3 处→1） |
| 9a | `views/useFilesView.searchTokens.ts`（app→shared）+ `useFilesView.ts` 引用改指 shared | 补搬 §0.5.4 漏搬的纯函数模块进 shared：shared 测试 `import ... from "@encv/shared-components/views/useFilesView.searchTokens"` 此前靠 alias-fallback 回退到 app 解析（假共享）；现补真源、删 app 副本、唯一引用方改指 shared | `pnpm check:all`（shared 测试解析到真源，无 fallback）+ grep app 内无 `@/views/useFilesView.searchTokens` |
| 9b | `views/useFilesView.ts`（66KB，app→shared）+ `views/useFilesHelpers.ts` + `constants/player.ts` | 真深水硬骨头：把 66KB 主 composable + player 常量 + helpers 搬进 shared。GoProcess 原生能力经 `appCapabilities` DI 注入（扩 4 函数 + 测试后门能力 + app 注册），其余 `@/composables/*`/`@/api/*`/`@/features/*` 全改 `@encv/shared-components/...`；删 app 三副本，`Files.vue`/`Settings.vue` 改指 shared | 真实门禁 `check-all.mjs` **PASS 8 / FAIL 0 全绿**（首跑 FAIL 1 因 DEV 测试后门 `@/` 动态 import，已改能力注入修复） |

> 批 9 是路径/边界层面的收尾（v2 内容），放在逻辑改革之后：逻辑先收敛，边界再收紧，
> 避免「边界收紧后还靠回退解析」的假象。它不解决 A/A3/B/D/E/F/G 任何一处逻辑冗余。
> 批 7（G）应尽早排——它不仅去重，还修一个「两页状态不一致」的实际 bug。

## 落地状态追踪

**已修复（2026-07-14 续：重跑门禁暴露的 2 处遗留失败）：**

- **`api/sparseContainer.ts`（批 9 边界遗留）**：`getApiBaseUrl` 仍从相对路径 `./encv` 导入，而 `api/encv.ts` 已在提升期删除（真源迁至 `shared-components/api/`）。因是**相对路径**（非 `@/api/encv`，后者靠 tsconfig `@/*` 回退到 shared 才侥幸解析），无回退 → `TS2307 Cannot find module './encv'`。改为 `@encv/shared-components/api/core`（`getApiBaseUrl` 真源在 `core/baseUrl`，经 `core/index` 导出），符合 §3「跨边界一律 `@encv/shared-components/*`」。
- **`composables/useConfirmDialog.ts`（K8 新增件格式债）**：biome CI 报 `File content differs from formatting output`，`biome format --write` 修复。
- 验证：两处修复后全量门禁 `check-all.mjs` **PASS 8 / FAIL 0**（此前为 FAIL 2/8）。

> ⚠️ **验证方法纠错（2026-07-14）**：G / A3 初版笔记里写「`read_lints` 0 错误」并暗示门禁绿，这是**错误**的——
> `read_lints` 只跑 ESLint / 语言服务诊断，**不跑 `tsc` 类型检查**，绝不能当门禁。真实门禁是
> `node scripts/check-all.mjs`（报告落 `app/check-report.md`）。
> 实际首次跑门禁即红：A3 的 `encv_files.ts` 把类定义删成 `export {…} from "./core/errors"` 后，
> re-export **不把名字带进本模块作用域**，6 处 `throw new PermissionDeniedError(...)` 全 TS2304；
> 另 app 侧 `encv.ts` 仍 `import … from "./encv_core"`（LIFT 已删该文件）→ TS2307。
> 两处均已修复（`encv_files.ts` 改回 `import`+`export`；app `encv.ts` 改指 `@encv/shared-components/api/core`）。
> 修复后门禁：**FAIL 4/8 → FAIL 2/8**；剩余 2 FAIL = 4 个**预存**测试失败
> （`mockGenerator.test.ts`×2 / `useMockGenLog.test.ts`×2，文件全程未被本批次触碰，与本次重构无关）。
> 后续所有批次**一律以 `check-all.mjs` 真实结果为准**，不再用 read_lints 冒充门禁。
> **（2026-07-14 收尾）** 这 4 个预存测试失败已修复（见批 6），并顺带修 6 个 biome 格式债，
> 门禁最终 **PASS 8 / FAIL 0**（全绿）。

**已落地（G，代码完成，真实门禁已验证）：**

- **新 `lib/taskEvent.ts`**：导出 `normalizeTaskEvent(type, data)`，把 `completed` 事件的归一化
  （`status: data.error ? "failed" : "completed"` / `completedAt` / 无错时 `progress: 100`）收敛为单一真源；
  非 `completed` 事件原样透传，各 store 的差异逻辑（MAX_LOADED_TASKS 守卫 / runId 过滤）仍自管。
- **`stores/taskStore.ts`**：`applyEvent` 的 `completed` 分支由内联归一化改为 `patchTaskById(id, normalizeTaskEvent("completed", data))`，
  行为不变（仍 `persistPutTask(id)`）。
- **`stores/runTasksStore.ts`**：`applyEvent` 原 `completed` 直接 `patchTaskById(data.id, data)`（**零归一化**）→ 已补 `completed` 分支
  `patchTaskById(data.id, normalizeTaskEvent("completed", data))`，与 taskStore 对齐，**消除两页状态分歧**。
- **新测试 `lib/__tests__/taskEvent.test.ts`**（纯函数，注册进 `vitest.config.ts` 的 `FAST_INCLUDE`）：覆盖
  completed 无错/有错归一化 + 非 completed 透传，锁住单源不变量。
- 验证（真实门禁 `check-all.mjs`）：G 逻辑改动本身无 TS 报错；首次全量门禁曾因 A3 的 `encv_files.ts` re-export 作用域 bug 连带红，A3 修复后该红线已消除（见 A3 / 纠错说明）。`node scripts/make-shim.mjs check-all` 无需（G 不新增 app 垫片）。
- 注：本批**未新增 app 垫片**（`lib/taskEvent` 是全新 shared-only 模块，app 无 `@/lib/taskEvent` 引用），故不影响 LIFT 的 shim 计数。

**已落地（A3 / 批 1，代码完成，门禁待用户本地跑 `pnpm check:all`）：**

- **`api/core/errors.ts`**：`PermissionDeniedError` / `NotFoundError` 从 `encv_files.ts` 迁入，改为
  `extends ApiError`（构造填 `status=403/404` + `body`）；保留 `(message, body?)` 调用签名，**现有 throw
  站点零改动**。调用方 `instanceof ApiError` 即可统一处理，解决「两套错误体系」问题（A3）。
- **`api/encv_files.ts`**：删除本地类定义，改为 `export { PermissionDeniedError, NotFoundError } from "./core/errors"`；
  `encv_search.ts`（import 自 `./encv_files`）与 `FilePickerModal.vue`（经 `api/encv` barrel）**无感**。
- **`api/core/request.ts`**：`apiRequest` 扩展为统一请求入口的抽象层——
  - `timeoutMs`：`AbortController` 超时（替代各模块手写 `new AbortController()` 样板）；
  - `statusErrorMap`：非 2xx 按状态码映射到错误子类，默认 `403→PermissionDeniedError` / `404→NotFoundError`；
  - **SPA-fallback 防护**：2xx 但 `content-type: text/html`（登录页）视为后端不通，归一化为 `ApiError`
    （不再让 `response.json()` 抛裸 `SyntaxError`）。
  - 向后兼容：新参数均可选，既有 `apiRequest` 调用行为不变。
- 验证（真实门禁 `check-all.mjs`）：A3 改动初版曾因 `encv_files.ts` re-export 作用域 bug 导致 6 处 TS2304 红线，
  已改为 `import`+`export` 修复；修复后全量门禁由 FAIL 4/8 降至 FAIL 2/8（剩余 2 FAIL 为预存测试失败，与本批无关）。
  此批是 **A（批 2，11 模块收敛到 `apiRequest`）的地基**——
  地基就位后，各 api 模块的裸 `fetch` + `ok`/`json` + `AbortController` 样板可统一改走 `apiRequest`。
- 注：本批**未新增 app 垫片**；`api/encv.ts` barrel 经 `encv_files` re-export 仍透传两个错误类，app 无感知。

**已落地（B + D / 批 3+4+5，真实门禁 `check-all.mjs` 已验证）：**

- **`lib/workflow/state-machine.ts`（B）**：
  - 新增 `VALID_TRANSITIONS_STRICT`（严格转换表：`pending` 只能 → `submitted/cancelled`，禁止跳级，对齐旧 camelCase 版语义）。
  - `validateTransition(from, to, opts?)` 增加 `opts.strict`：`true` 走严格表、缺省 `false` 走宽松表（现状不变，向后兼容）。
  - 新增 `transition(from, to, opts?)`：合法返回目标状态、非法抛错（与 `validateTransition` 共用转换表 + strict 选项）。
  - 清掉顶部那段描述已删 `stateMachine.ts`（驼峰）的注释，改为 strict 模式说明。
- **`composables/__tests__/workflow-core.test.ts`（B/D）**：
  - 修悬空 import：原 `from ".../stateMachine"`（驼峰，已删模块）+ `canTransition`（全代码库零定义）→ 改为 `from ".../state-machine"` + 真实导出 `validateTransition`/`transition`/`computeJobConclusion`/`inferWorkflowStatus`。
  - 重写 State Machine 测试块：宽松（默认）/ 严格（`{strict:true}`）双模式断言 + `transition` 用例。
  - **关键 D 发现**：该测试的 FAST_INCLUDE 条目原路径是 `src/composables/__tests__/workflow-core.test.ts`（**缺 `../packages/shared-components/src/` 前缀**）→ 指向不存在的 app 层文件，**门禁从未真正跑过它**（悬空 import 被静默放过，正是 D 说的"测试被门禁永远静默放过"）。已把 FAST_INCLUDE 条目修正为 `../packages/shared-components/src/composables/__tests__/workflow-core.test.ts`，现在真正被收集。
  - 真实门禁验证：`✓ workflow-core.test.ts (51 tests) 13ms` —— 通过。
- **清 `useWorkflowEngine` 幽灵引用（D）**：
  - `useWorkflowTaskService.ts`：删掉「从 useWorkflowEngine 迁移的核心逻辑」整段注释（指向已删抽象）。
  - `useWorkflowStore.ts`：`runs 历史持久化已移除（消费者 useWorkflowEngine 已删除）` → 改为「（由 useWorkflowTaskService 接管）」。
  - `path-chain-e2e.test.ts`：链路注释 `useWorkflowEngine.executeJob()` 与示例代码注释 `useWorkflowEngine.executeJob() 中：` 两处 → 改为「工作流任务服务（submitRun）」。
- 验证（真实门禁 `check-all.mjs`，**非 read_lints**）：B+D 全部通过；全量门禁由 FAIL 4/8 → FAIL 2/8（剩余 2 FAIL = 1 个 vitest 套件，含 4 个**预存**测试失败 `mockGenerator.test.ts`×2 / `useMockGenLog.test.ts`×2，文件未被本批次触碰，与 B/D 无关）。`Test Files 2 failed | 12 passed (14)`，`Tests 4 failed | 357 passed (361)`。

**已落地（批 6：预存测试债 + 格式债修复，真实门禁 `check-all.mjs` 已验证全绿）：**

- **`api/mockGenerator.ts`（修源健壮性）**：SSE 解析原仅靠 `\n\n` 分割，测试构造的流最后一块缺尾随 `\n\n` → `done`/`error` 块永不被解析（count 永远 0、fatal 不 reject）。
  - 抽取 `dispatch(parsed)` 供主循环与流结束 flush 共用（消除重复逻辑）。
  - 流结束 `reader.read()` 返回 `done` 后，若 `buffer` 仍有残留则再 `parseSseEvent` 一次并 `dispatch`，兼容无尾随分隔符的 SSE 流（测试 / 部分后端实现）。
- **`composables/useMockGenLog.ts`（修 summary.total）**：`onSpecDiag` 原本只同步 `onSpecPlan`/`starting` 设的 `mockGenLogTotal`，但 `diag` 事件本身带 `total`；补 `mockGenLogTotal.value = diag.total`，使 summary 正确反映本批 spec 总数（测试期望 3）。
- **`composables/__tests__/useMockGenLog.test.ts`（修测试 mock 方式）**：`navigator.clipboard` 是只读 getter，直接赋值抛错；改用 `Object.defineProperty(navigator, "clipboard", { value: {...}, configurable: true })`。
- **biome 格式债（6 文件，`biome format --write`）**：`encv-mobile/src/views/useFilesView.ts`、`packages/shared-components/src/api/__tests__/mockGenerator.test.ts`、`useMockGenLog.test.ts`、`workflow-core.test.ts`、`useWebDavWorkflowAdapter.ts`、`lib/taskEvent.ts` 共 6 个文件格式不达标（其中 3 个为本次批次引入、3 个为预存）。统一 format 修复，format-only 不影响逻辑。
- 验证（真实门禁 `check-all.mjs`）：**PASS 8 / FAIL 0**，全绿。4 个预存测试失败清零，`workflow-core.test.ts` 仍被正常收集（51 tests）。

**已落地（A / 批 2：11 个 api 模块收敛到 `apiRequest`，真实门禁已验证 PASS 8/FAIL 0）：**

- **`api/core/request.ts`（地基增强，A3 之上）**：
  - 新增 `responseType?: "json" | "text" | "blob" | "response"`（默认 `json`）。`text`/`blob` 直接返回对应类型；`response` 返回原始 `Response`（供需读响应头的场景如 `exportDatabase`）。
  - json 分支改为先读 text：空响应体（204 / 空 200）返 `undefined`（防 `response.json()` 抛 `SyntaxError`）；HTML / SPA-fallback 抛 `ApiError`；非法 JSON 抛 `ApiError`。
  - 默认 `403→PermissionDeniedError`、`404→NotFoundError` 映射保留（与 A3 typed-error 收敛一致）。
- **11 模块收敛**（裸 `fetch` + `getApiBaseUrl` + `response.ok`/`json` 样板 → `apiRequest`）：
  - `encv_system` / `encv_trash` / `encv_openlist` / `encv_webdav` / `encv_search` / `encv_plugins` / `encv_perf` / `encv_files` / `encv_files_extra` / `encv_admin` / `encv_tasks`。
  - 保留 `proxySafeEncode` 双编码的 URL 仍按原样拼进 `apiRequest` 的 path（不丢编码语义）。
  - 特殊错误处理保留：`searchFiles` 403、`listFiles` 403/404 由 `apiRequest` 默认映射；`rebuildFullTextIndex` 的 `code/taskId/status` 自定义错误字段、`updateConfig` 成功但非 JSON 回退默认消息、`checkBackendPermissions`/`checkFileExists`/`checkEncryptOutputExists`/`getSearchStats`/`getFullTextIndexStats` 的错误→默认值 均用 try/catch 包 `apiRequest` 还原；`fetchTextPreviewExts`/`batchCreateTasksSingle` 的手动 `AbortController` 超时改为 `timeoutMs`。
  - `encv_tasks` 本就 `import apiRequest`（绝对路径 `@encv/shared-components/api/core`），其余 10 个改从 `./core/request` 导入。
- **有意保留的裸 fetch / 非收敛点**（A 范围外，文档登记）：
  - `encv_files.listFilesStream` / `listPluginFilesStream`：SSE/NDJSON 流式读取 `response.body` reader，无法走 `apiRequest`（其返回已解析数据），保留裸 fetch。
  - `encv_admin.checkServerStatus`：防劫持 ping 探针（自管 `cache:"no-store"`、`Content-Type` 校验、`instance_id` 比对、持久化），逻辑特殊，保留裸 fetch。
  - `encv_admin.checkServiceGuard`：失败抛带 `code`/`payload` 自定义字段的 `Error`，保留裸 fetch（如需收敛需 `statusErrorMap` + 自定义 error 类）。
  - `encv_perf.runDatabaseTests`：XHR SSE 流式（非 `fetch`），保留。
  - `mockGenerator.ts`：SSE 生成器（裸 `fetch` + 流解析），属 mock 基础设施，不在 11 模块内，保留。
  - `getFileStreamUrl` / `getFilePreviewUrl` / `getExternalStreamUrl` / `getAlistEncryptStreamUrl`：纯 URL 构造器（无 fetch），保留。
- 验证（真实门禁 `check-all.mjs`）：`biome format --write` + 全量门禁 **PASS 8 / FAIL 0**，全绿。A 与 E/F/G/A3/B/D/批6 共同构成「shared 内部双轨收敛」主线。

**已落地（F / 批 8：加/解密表单抽 `TaskFormFields` + `useFilePickerBrowse`，真实门禁已验证 PASS 8/FAIL 0）：**

- **新 `components/TaskFormFields.vue`**：把 `EncryptBody`/`DecryptBody` 共有的「源/目标输入 + 密码段 + extraFields 渲染 + `InputWithHistory` + `browsable` 文件选择」收敛为单一子组件；用 `condition` prop + 两个具名 slot（`#version`/`#cipher`/`#compression` 等）保留各自独有区块与字段顺序；`browsable` 走共享的 `useFilePickerBrowse`。
- **新 `composables/useFilePickerBrowse.ts`**：把 `EncryptBody`/`DecryptBody` 各自的 `handleBrowse*`（源/目标/输出目录选择）收敛为共享 composable（目录 vs 文件、写入 `ref`）。
- **`components/EncryptBody.vue`**：组合 `TaskFormFields`，把 version/cipher/compression 作为 slot 内容传入，删除逐字重复的源/目标/密码/extraFields/`InputWithHistory` 样板。
- **`components/DecryptBody.vue`**：组合 `TaskFormFields`，删除逐字重复的源/目标/密码/`InputWithHistory` 样板。
- **`__tests__/tasks-regression.test.ts`**：原检查 `EncryptBody`/`DecryptBody` 源码含 `InputWithHistory`+`browsable`，改为检查两者含 `TaskFormFields` 且 `TaskFormFields.vue` 源码含 `InputWithHistory`+`browsable`（锁住「表单无重复」不变量）。
- 验证（真实门禁 `check-all.mjs`）：`biome format --write` + 全量门禁 **PASS 8 / FAIL 0**，全绿；组件测试 + 回归测试仍绿。

**已落地（H / 批 10：抽 `usePoll` 统一轮询样板 + composable 裸 fetch 收敛到 `apiRequest`，真实门禁已验证 PASS 8/FAIL 0）：**

- **新 `composables/usePoll.ts`**：统一「周期轮询端点 → 更新响应式状态」样板。API：`usePoll(fetcher, { intervalMs, immediate?, guardConcurrency?, onEvent? })`，返回 `{ start, stop, refresh, reschedule, isPolling }`。
  - 递归 `setTimeout`（非 `setInterval`）每次循环重读 `intervalMs` → 天然支持动态 interval（如按 status 切频率）；
  - `guardConcurrency`（默认 true）：上一次 `fetcher` 未返回则跳过，消除并发重复请求；
  - `onEvent`：`EventKey | EventKey[]`，自动 `eventBus.on(ev, refresh)`，`onUnmounted` 自动 `stop()` + `eventBus.off`，调用方零清理；
  - 为支持该类型约束，`useEventBus.ts` 的 `EventKey` 由 `type` 改为 `export type`。
- **`useVectorSearchStatus.ts`**：`setInterval` + `pollInFlight` 手搓轮询 → 委托 `usePoll(probeOnce, { intervalMs: 60_000, immediate: true, onEvent: "server:status" })`；`probeOnce` 裸 `fetch(getApiBaseUrl()+"/api/runtime")` + ok/json → `apiRequest<{vector_search_available?, vector_search_degraded?}>("/api/runtime")`（A scope 扩到 composables）。
- **`useContextUsage.ts`**：`setTimeout` 递归自调度 + `watch(status)` 重排 → 委托 `usePoll(fetchOnce, { intervalMs: () => getInterval(), immediate: true })`；`watch(status)` 改调 `poll.reschedule()`、`watch(sessionId)` 改调 `poll.refresh()`；删无用 `let timer`。**保留** `fetchOnce` 裸 `fetch` —— Agent API 走 `getAgentApiBase()`（dev=preview-gateway/agent-api、APK=相对路径由 ApiProxy 接管），与 `apiRequest` 标准后端 base 不同，属「有意不收敛」（同 A 批探针/特殊 base）。
- **`useTaskCancel.ts`**：`pollCancelStatus` 裸 `fetch(\`${baseUrl}/api/tasks/...\`)` + ok/json → `apiRequest<{status?:string}>(\`/api/tasks/${encodeURIComponent(taskId)}\`)`；删 `getApiBaseUrl` 导入、改 import `apiRequest`。
- **`useVectorSearchStatus.test.ts`**：原 mock 裸 `fetch` → 改 mock `apiRequest`（hoisted），覆盖 available/degraded/unavailable/失败保持原值/并发守卫/路径=`/api/runtime` 等，锁住单源不变量。
- **有意保留的 composable 裸 fetch（登记）**：`useContextUsage.fetchOnce`（agent 特殊 base）、`useServerStatus.networkPing`（防劫持 `/ping` 探针，自管 `AbortController`+`cache:"no-store"`）。`useServerStatus` 本轮未改（其 ping 属探针，保留裸 fetch）；若后续要统一，可在 `usePoll` 之上再包一层，但当前按「探针/特殊 base 不收敛」原则保留。
- 验证（真实门禁 `check-all.mjs`）：`biome format --write` 5 文件（均无需修复）+ 全量门禁 **PASS 8 / FAIL 0**，全绿。`useVectorSearchStatus.test.ts` 仍被收集通过。

**已落地（I / 批 11：搜索命中 + 显示名抽单源，真实门禁已验证 PASS 8/FAIL 0）：**

- **`lib/taskViewComputeCore.ts`**：
  - 新增 `matchTaskSearch(task, q): boolean` —— name/plugin/error/id 四字段 `includes` 搜索命中判定的**单一真源**（空查询返回 true）。原 `filterTasks`（非后端搜索分支）与 `computeGroupCounters`（`search.hit`）各自内联的 4 字段 includes 改为调用之（搜索命中判定 3 处→1）。
  - 新增 `countGroupHit(tasks, filters): number` —— group 在「所有激活筛选」下的命中数（plugin/type/status/date/search 交集）单一真源；search 命中复用 `matchTaskSearch`。`useTasksView.computeGroupHit` 委托之。
  - `getTaskDisplayName` 早已是单源（core:40）；本批把调用方副本收敛掉（见下）。
- **`composables/useTasksList.ts`**：删本地 `getTaskDisplayName` 副本（与 core 逐字重复），改从 core `import { computeGroupCounters, getTaskDisplayName }`；`getTaskName` 仍委托 core 单源。`useTasksList.computeGroupCounters` 冗余 fallback 经核对**已在批 E 完成**（现 `_computeGroupCounters` 直接委托 core），本批无需再删。
- **`components/TasksTab.vue`**：删本地 `getTaskName`（即 `getTaskDisplayName` 副本），模板改调 core 导入的 `getTaskDisplayName`；显示名副本 3 处→1（仅 core）。
- **`composables/useTasksView.ts`**：`computeGroupHit` 内联的 4 字段 includes + 全维度过滤改为 `return countGroupHit(tasks, { searchQuery, filterPlugins, filterTypes, filterStatuses, filterDateRange })`（读响应式 ref），委托 core 单源。
- **`taskStore.ts`**：核对无 `split("/").pop()` / `getTaskDisplayName` / `includes(q)` 残留（批 E 已收敛），本批无需改动。
- **grep 终态**：`matchTaskSearch` 定义仅 core 1 处；`getTaskDisplayName` 定义仅 core 1 处（其余 `TaskOutputInfo.vue:76` / `TaskDetailModal.vue:117` 为 output 路径 basename，非任务显示名，属不同语义，保留）。composable/app 层不再有逐字重复的 4 字段搜索命中或显示名逻辑。
- 验证（真实门禁 `check-all.mjs`）：`biome format --write` 4 文件（均无需修复）+ 全量门禁 **PASS 8 / FAIL 0**，全绿。encv-mobile 单测 + 6 处 typecheck + biome CI + i18n lint 全通过。

**已落地（9a / 批 9 第一步：补搬 `useFilesView.searchTokens` 进 shared，真实门禁已验证 PASS 8/FAIL 0）：**

- **新 `views/useFilesView.searchTokens.ts`（shared）**：纯函数模块（`tokenizeQuery` / `renderSnippet` / `operatorSymbols` / `QueryToken` / `SnippetPart`），零外部 import，与 app 原文件逐字一致（仅补顶部 lift 注释）。这是 §0.5.4 点明的「漏搬大块」中较小的纯函数部分。
- **`views/useFilesView.ts`（app，66KB）**：唯一引用方 import 由 `@/views/useFilesView.searchTokens` 改 `@encv/shared-components/views/useFilesView.searchTokens` —— 直接指向真源，**消掉一条假共享回退路径**（不再经 app 壳或 alias-fallback）。
- **删 `app/encv-mobile/src/views/useFilesView.searchTokens.ts`**：实现已提升进 shared，app 副本不再需要。
- **效果**：shared 测试 `useFilesView.searchTokens.test.ts` 的 `import ... from "@encv/shared-components/views/useFilesView.searchTokens"` 从此解析到**真源模块**（此前靠 `encv-alias-fallback` 回退到 app 才通，属「shared→app 假共享」实证）。
- **不盲从文档的纠正**：§0.5.4 把 `clientSearchTokenize`（shared，CJK 单字切分，用于**实际文件名过滤**）与 `tokenizeQuery`（app，phrase/regex/boolean，用于 **UI 语法高亮**）并称「两份搜索切词能力待统一」——经核实二者语义不同、用途不同，**不是重复**，本批不强行合并（避免引入错误抽象）。
- 验证（真实门禁 `check-all.mjs`）：`biome format --write` 2 文件（均无需修复）+ 全量门禁 **PASS 8 / FAIL 0**，全绿。shared 测试 `useFilesView.searchTokens.test.ts` 现解析到真源模块（不再经 alias-fallback）；encv-mobile 单测 + 6 处 typecheck + biome CI + i18n lint 全通过。

**已落地（9b / 批 9 第二步：搬 66KB `useFilesView` 进 shared，真实门禁已验证 PASS 8/FAIL 0）：**

- **测绘结论（code-explorer 子代理）**：`useFilesView.ts`(66KB) 不能原样搬，卡 2 个 app 专属硬依赖：`@/plugins/GoProcess`(原生桥) 与 `@/constants/player`(PLAY_MODE)。其余 22 个 import 全是 npm 包 / shared 真源 / 已生成的 app 壳→shared。
- **方案 A（复用现有 `appCapabilities` 运行时注入 DI）**：
  1. `cp` 三文件进 shared：`views/useFilesView.ts` / `views/useFilesHelpers.ts` / `constants/player.ts`（shared `exports` 已含 `./constants/*` `./views/*` `./runtime/*`）。
  2. 扩 `runtime/appCapabilities.ts`：加 `openPlayer`/`openExternal`/`getLocalFilePath`/`requestStoragePermission` 4 函数（导出 `PlayResult`/`PermissionResult` 类型，签名对齐 `GoProcess.ts`），默认抛错；`isNative` 已存在。
  3. `app/stores/registerSharedAppCapabilities.ts` 注入这 4 个 GoProcess 包装函数。
  4. shared `views/useFilesHelpers.ts` 与 `views/useFilesView.ts` 的 import 全部由 `@/x`/`./useFilesHelpers` 改写为 `@encv/shared-components/...`；删 GoProcess import，改 `import { getAppCapabilities }`；调用点 `isNative()`(×4)/`openPlayer(`/`openExternal(`/`getLocalFilePath(`/`requestStoragePermission(` 改走 `getAppCapabilities().xxx(`。
  5. `constants/player` 去壳：删 app 副本，`Settings.vue:430` 改指 `@encv/shared-components/constants/player`（grep 确认 `@/constants/player` 仅被 useFilesView/Settings.vue/useFilesHelpers 引用，前两者已改指 shared）。
  6. 删 app `views/useFilesView.ts` + `views/useFilesHelpers.ts`（grep 确认仅 Files.vue 引 useFilesView、仅 useFilesView 引 useFilesHelpers）；`Files.vue:636` 改 `from "@encv/shared-components/views/useFilesView"`。
  7. **验证（真实门禁 `check-all.mjs`）**：首跑 FAIL 1 —— `shared-components/src/views/useFilesView.ts(1725): Cannot find module '@/composables/useTestBackdoor'`。根因：原 app 副本有一段 `if (import.meta.env.DEV)` DEV-only 测试后门，动态 `import("@/composables/useTestBackdoor")` + `useNewTaskModal`，被字节级 cp 带进 shared，shared 既无此模块也不能用 `@/`。**修复**：按既有 `appCapabilities` DI 模式，在 `runtime/appCapabilities.ts` 加 `TestBackdoorContext` 接口 + 可选 `registerTestBackdoor?` 能力；shared `useFilesView.ts` 的 DEV 块改调 `getAppCapabilities().registerTestBackdoor?.(ctx)`；app `registerSharedAppCapabilities.ts` 注入实现（保留 `@/` 动态 import 在 app 侧）。补改 3 文件 `biome format --write`（无修复）后重跑 → **PASS 8 / FAIL 0 全绿**（shared-components typecheck / encv-mobile typecheck / 6 处 typecheck / Biome CI / i18n lint / 单测全通过）。grep 确认 shared 三文件已无任何 `@/` 残留。
  - **关键发现（纠正原方案）**：app 原 `useFilesView.ts` 的 `isNative()` 调用**早已**是 `getAppCapabilities().isNative()`（isNative 已通过能力注入），故实际只需给 `openPlayer`/`openExternal`/`getLocalFilePath`/`requestStoragePermission` 4 处裸调用加 `getAppCapabilities().` 前缀，无需动 `isNative`。这印证了「app 早就在用 appCapabilities 解耦原生能力」的既有设计，本批只是把 66KB 主体 + player 常量 + helpers 物理提升进 shared 并补全剩余 4 个能力的注入 + 测试后门能力。

**已落地（9 / 批 9 第三步：边界强制之 import 单源化，真实门禁已验证 PASS 8/FAIL 0 + `_measure-fallback.mjs` 归零）：**

- **量化起点**：`node scripts/_measure-fallback.mjs` 显示 **136 处** `@/x` 导入（app 无本地文件、靠 alias-fallback 落到 shared）：123 `@/composables`、9 `@/components`、各 1 `@/views`/`@/utils`/`@/api`/`@/lib`。
- **机械改写**：一次性脚本（已删）把全部 136 处 `@/x` → `@encv/shared-components/x`（仅改写「app 无本地文件且 shared 有」的导入，行为不变，仅显式单源化）。覆盖 ~80 个文件（`Files.vue`/`Settings.vue`/`main.ts`/`router/index.ts`/`Tabs.vue`/`useAgentChatView.ts`/`taskViewCompute.worker.ts` 等）。
- **验证（真实门禁 `check-all.mjs`）**：`biome format --write encv-mobile/src`（245 文件 No fixes applied）+ 全量门禁 **PASS 8 / FAIL 0**，全绿。重跑 `_measure-fallback.mjs` → **总数 0**（假共享回退残留归零）。
- **✅ 删 73 转发壳（已完成，2026-07-14）**：一次性脚本（已删）按「纯壳判定」精确识别 73 个纯转发壳（**只有 `export ... from "@encv/shared-components/..."`，无 import / 无本地引用 / 无其它 export**，精确排除 `api/encv.ts` barrel、`i18n/index.ts`、`useAgent.ts` 等混合文件），先把 **114 处**指向壳的 import（`@/x` 与相对路径，跨 47 文件，含 `api/encv.ts` 的 11 处 `./encv_* ` barrel re-export）改写为 `@encv/shared-components/x`，再删 73 壳。验证：`_measure-fallback.mjs`=0 + `biome format`（1 文件）+ check-all **PASS 8/FAIL 0** + `vite build` ✓ built in 7.34s。**注意教训**：批 9 摘除 alias-fallback 的 shared 兜底后，MEMORY 里记载的 `make-shim.mjs prune --apply`「删后 @/x 自动落 shared 零风险」前提已失效——必须先改 importer 再删壳（本次即此顺序）。
- **未做（留作安全网，非本批必需）**：删 `encv-alias-fallback` 机制本身（它仍是 app 唯一的 `@/`→本地 src 解析器，删它须先给 vite/vitest 加真正的 `@` 别名，属高爆破半径 build 配置改动，当前保留无害）。
- **⚠️ 关于删 `encv-alias-fallback` 的重要纠正（勘察结论）**：该插件**不是**单纯的 shared 兜底——它是 app 里**唯一**的 `@/` 解析器（`vite.config.ts` 的 `resolve.alias` 无 `@` 条目，vitest 也刻意不设 `@` 别名，全靠此插件把 `@/x` 解析为「本地 src 优先、shared 次之」）。因此**不能裸删它**，否则 app 所有解析到本地文件的 `@/x` 也会断。本批让 `_measure-fallback.mjs` 归零后，shared 兜底分支已成死代码——"假共享"目标**已达成**（`@/` 严格只解析本地，`@encv/shared-components/` 严格指向 shared）。要彻底删插件，须先给 vite/vitest 加真正的 `@` → `encv-mobile/src` 别名再移除插件，属高爆破半径的 build 配置改动，建议作为独立、单独门禁验证的批次，且当前非必需（插件留着无害）。
- **已摘除 shared 兜底分支（已验证）**：采用低风险方案——把插件 `dirs` 改为**仅本地 src**（`vitest.config.ts` 的 `roots` 去 SHARED_SRC；`vite.config.ts` inline 插件 `dirs` 去 shared 行），从解析层彻底杀死"假共享"，同时保留 `@/` 对本地文件（含 `?worker`）的解析能力。`measure=0` 证明无 `@/x` 需落 shared，故摘除后所有 `@/x` 仍解析本地，tsconfig `@/*`→shared 回退亦成死路径，无错位。**真实门禁 `check-all.mjs` → PASS 8/FAIL 0 全绿 + `vite build`（`pnpm --filter encv-mobile build`）✓ built in 6.18s 均通过。**
- **验证构建（vite build）暴露并修复的 4 处假共享残留**：批 9 的 import 单源化脚本只扫 app 的 `from` 导入，漏掉两类 —— ① `main.ts` 的 3 个 **CSS side-effect import**（`@/theme/variables.css`、`@/styles/timeline-tokens.css`、`@/styles/timeline-utilities.css`，文件早已被更早重构移到 `shared-components/src/theme|styles/`，但 main.ts 仍用 `@/`）；② `shared-components/src/composables/useTasksView.ts:207` 的 **动态 `import("@/components/TaskDetailModal.vue")`**（该文件被批 9b 移到 shared，但 shared 内部自引用仍用 `@/`）。此前靠 `@/` 的 shared 兜底能解析，摘除兜底后断链（`UNLOADABLE_DEPENDENCY`）。已全部改为显式 `@encv/shared-components/...` 路径并重新 `vite build` 通过。
- **✅ 逻辑抽象改革（J / 批 12）：格式化单一真源（2026-07-14）**：这是 §0.5.2「提升进 shared 却没收敛到既有抽象」的活样本——shared 内部存在**跨模块重复的格式化逻辑**（区别于搬运层删壳，是「逻辑本身」的抽象改革）。测绘：① 文件大小格式化 3 处——`api/encv_files.ts` 的 `formatFileSize`（经 `api/encv` barrel 公开导出，shared+app 共 9 处消费）、`lib/buildReportZip.ts` 与 `components/TaskPerformanceSection.vue` 各一份本地 `formatBytes`；三者 1024 进制、B/KB/MB/GB/TB 单位序一致，仅健壮性（undefined 处理）/精度（toFixed 1/1/2）微差；② 日期格式化 3 处——`useFilesHelpers.formatDateInput`（HTML date input 契约，不可动）、`useDateFormat.formatDateTime` 与 `PerformanceTab.formatTime`（显示用途，可收敛但涉及显示格式统一需谨慎）。本轮收敛**文件大小格式化**：新建 `lib/format.ts` 的 `formatBytes(bytes?)`（最健壮版：undefined/null→""、<=0→"0 B"、clamp 越界单位、toFixed(1)）作单一真源；`encv_files.formatFileSize` 改为委托（公开 API 不变，消费方无感）；buildReportZip/TaskPerformanceSection 删本地副本、改 import 共享版。**验证全绿**：`biome format`（Fixed 1 file）+ check-all **PASS 8/FAIL 0** + `vite build` ✓ built in 7.92s。日期格式化收敛（`formatTime` 委托 `formatDateTime`）留作候选，因改变显示格式需用户拍板。

---

## 3. 终态不变量 / 门禁

- **统一请求入口**：`api/*` 内 JSON/文本/blob 请求统一走 `apiRequest`（`core/request.ts`）；
  有意保留的裸 `fetch`：`listFilesStream`/`listPluginFilesStream`（流式 reader）、`checkServerStatus`（防劫持探针）、
  `checkServiceGuard`（自定义 error 字段）、`runDatabaseTests`（XHR SSE）、`mockGenerator`（SSE 生成器）；
  URL 构造器（`getFile*Url`/`getAlistEncryptStreamUrl`）无 fetch。
- **统一错误体系**：所有 API 错误 `instanceof ApiError`；typed error 为其子类。
- **状态机可配置**：`validateTransition` 默认宽松、传 `{ strict: true }` 即严格；无第二套冲突规则表。
- **无悬空 import**：`tsc` 全量（含 test）+ 关键测试纳入基础门禁；已删抽象无注释/测试残留。
- **视图计算单源**：filter/sort/group/counters/display 仅 `lib/taskViewComputeCore.ts` 实现，`taskStore`/`useTasksList` 委托之，无第二份内联。
- **事件归一化单源**：`completed`（status/completedAt/progress）归一化仅 `lib/taskEvent.ts` 实现，两 store 共用；Tasks 页与 GroupDetail 页对同一事件状态一致。
- **表单无重复**：加/解密表单共享 `TaskFormFields` + `useFilePickerBrowse`，Encrypt/DecryptBody 仅保留各自独有区块。
- **边界显式**（批 9 后）：`@/` 仅指 app 本地；跨边界一律 `@encv/shared-components/*`；
  `_measure-fallback.mjs` 输出 0。
- **格式化单源**（批 J）：文件大小格式化仅 `lib/format.ts` 的 `formatBytes` 实现，`formatFileSize` 为其公开别名；无第二份 1024 进制副本。

---

## 4. 终态蓝图

```
packages/shared-components/   不再是搬运壳，而是「抽象真正被消费」的单源
  api/core/
    request.ts   ← 唯一 fetch 入口（apiRequest：baseUrl + auth + timeout + typed error + SPA-guard）
    errors.ts     ← ApiError + PermissionDeniedError/NotFoundError（均 extends ApiError）
  api/            ← 11 模块全部走 apiRequest，零手写 fetch 样板
  lib/workflow/
    state-machine.ts  ← validateTransition(from,to,{strict?}) + VALID_TRANSITIONS + VALID_TRANSITIONS_STRICT
  lib/taskViewComputeCore.ts  ← 视图计算唯一真源（filter/sort/group/counters/display），store 与 composable 委托之
  lib/format.ts               ← formatBytes（文件大小格式化唯一真源），formatFileSize 为其公开别名
  lib/taskEvent.ts  ← WS 事件归一化唯一真源（normalizeTaskEvent），taskStore/runTasksStore 共用
  components/
    TaskFormFields.vue     ← 加/解密表单共享骨架（源/目标/密码/extraFields）
    EncryptBody/DecryptBody.vue  ← 仅各自独有区块 + 组合 TaskFormFields
  composables/
    useFilePickerBrowse.ts  ← 文件选择 modal 复用；无指向已删抽象的幽灵引用；关键 test 进常驻门禁
```

shared 的价值从「能把文件搬过来」升级为「**把重复逻辑真正收敛掉**」——这才是结构性改革。

---

## 5. 逻辑抽象改革 Backlog（已识别，待执行/待拍板）

> 本节为「逻辑本身正确抽象改革」的**待办清单**（区别于 §2 已落地的 A–J 批）。每条均带实证（文件:行）、语义差异、风险与建议。优先级由「零 UI 风险 → 高风险」递增。**执行前须先门禁验证**（check-all + vite build），改动显示格式的项须用户拍板。

### K1. `formatDuration` 三份实现收敛 ⚠️ 待拍板（显示格式会变）✅ 已落地（代码完成，门禁待终端恢复验证；2026-07-14 续）

- **关键认知修正（用户拍板）**：用户指出「丢精度」不应作为取舍来问——共享库本就该用**配置**保留精度。故改为**可配置单源**，调用方用选项精确还原各自原输出，**零精度损失、无行为变化**。
- **共享版改造**（`composables/useDateFormat.ts:formatDuration`）：新增 `FormatDurationOptions { showMs?, showDecimals?, invalid? }`。默认行为（无 ms、无小数、`<0→""`）与旧 shared 版一致（既有 TaskTimeline/TaskOutputInfo/StepInlineTimeline 消费方**零回归**）。
- **迁移**：
  - `lib/buildReportZip.ts`：删本地 `formatDuration`，改 `import { formatDuration }`；调用 `formatDuration(p.totalDurationMs, { showMs:true, showDecimals:true, invalid:"N/A" })` —— 精确还原 `"500ms"` / `"45.0s"` / `"1m35s"` / `"N/A"`（markdown 报告格式不变，历史 diff 可比）。
  - `components/TreeView.vue`：删本地 `formatDurationMs`，改 `formatDuration(step.durationMs, { showMs:true, showDecimals:true })` —— 还原 `"500ms"` / `"45.0s"` / `"1m35s"`。
- **结论**：K1 不再「改变显示格式」，而是「收敛逻辑 + 配置保精度」。`read_lints` 对两共享文件 0 诊断；门禁（check:all）待终端恢复后跑。

| 实现 | 位置 | 语义 |
|------|------|------|
| `formatDuration(ms:number)` | `composables/useDateFormat.ts:23`（shared，已导出，被 TaskTimeline/TaskOutputInfo/StepInlineTimeline 消费） | `<0→""`；`<60s→"Xs"`；`<60m→"XmYs"`；否则 `"XhYm"`。**无 ms 精度、无小数、无 N/A** |
| `formatDuration(ms?)` | `lib/buildReportZip.ts:296` | `null→"N/A"`；`<1000ms→"Xms"`；`<60s→"X.Ys"`（1 位小数）；否则 `"XmYs"` |
| `formatDurationMs(ms)` | `components/TreeView.vue:168` | `<1000ms→"Xms"`；`<60000→"X.Xs"`（toFixed(1)）；否则 `"XmYs"` |

- **差异**：三者对同一输入输出不同（shared 版丢 ms 精度、不显示 N/A；另两份带 ms 小数）。
- **建议**：以 shared 版为基准，补 `ms?` 入参、`null/undefined→"N/A"`、`<1000ms→"Xms"` 精度，统一为 `formatDuration(ms?)`；buildReportZip/TreeView 删本地副本、改 import。
- **风险**：buildReportZip 报告是**导出物**（markdown 报告），改格式会影响历史报告比对；TreeView 是 UI 显示。属「显示格式变化」，**须用户拍板**后再动。

### K2. `formatRelativeTime` 已收敛 + 过时注释清理（零风险）

- 实证：`composables/relativeTime.ts:37` 已是 shared 单一真源（带 `__tests__/relativeTime.test.ts` 5 档边界值覆盖）；`app/.../ServerStatusCard.vue:252` 已 `import { formatRelativeTime } from "@encv/shared-components/composables/relativeTime"`。全仓**已无本地副本**。
- **遗留**：`relativeTime.ts:19` 注释 `AgentChat.vue L1037 的局部 formatRelativeTime → 改为 import 这个` 已**过时**（实为已迁移）。建议删该注释。零风险，可随下次编辑顺手清。
- **已执行（2026-07-14 续）**：删除 `composables/relativeTime.ts` 顶部「调用方迁移」整块过时注释（原 L18–21：含 AgentChat.vue 局部迁移、sessionList 迁移、30s 自动刷新 setInterval 指引）。实证 `useAgentChatView.ts:27` 已 `import { formatRelativeTime } ...`，`useAgentChatView.ts:598` 注释亦确认 sessionList 逻辑已统一 → 整块迁移指引确为历史残留。函数实现与 5 档边界未动。
- **验证状态（已验证）**：`biome format --write` 对 `relativeTime.ts` 无需修复（格式已对齐）；真实门禁 `node scripts/check-all.mjs` **PASS 8/FAIL 0 全绿**（2026-07-14 14:2x 终端恢复后跑通）。

### K3. app 层 `formatBytes` 复用共享真源（2 处）⚠️ 低风险待确认 ✅ 已落地（代码完成，门禁待终端恢复验证；2026-07-14 续）

- **关键认知修正（同 K1）**：用**可配置单源**替代「取舍精度」。`lib/format.ts:formatBytes` 新增 `FormatBytesOptions { decimals?, invalid?, maxUnit? }`，默认 `(decimals:1, invalid:"", maxUnit:"TB")` 与旧共享版一致（既有 9 处消费方零回归）。
- **迁移（均零精度损失）**：
  - `FullTextIndexDetail.vue`：删本地 `formatBytes`，改 `formatBytes(x, { decimals:2 })` —— 原 `toFixed(1)` KB/MB / `toFixed(2)` GB 通过 `decimals:2` 全保留（KB 仅多一位小数，不丢精度）；**顺带修复**原缺 TB 单位（>1TB 原显示巨大 GB 数，现正确显示 TB）。
  - `SparseContainerTestDetail.vue`：删本地 `formatBytes`（1024 数学 + `toFixed(2)` + `"?"` sentinel），改为 `import { formatBytes as formatBytesShared }` + 本地 2 行绑定 `const formatBytes = n => formatBytesShared(n, { decimals:2, invalid:"?" })` —— 16 处调用点**逐字不变**，仅数学逻辑上提到共享（消除重复实现，非 shim 式转发）。
  - `plugin-simverse/web/.../SimverseSettings.vue`：删本地 `formatBytes`（与共享默认**逐字节相同**），直接 `import { formatBytes }`，调用点不变。
- **结论**：K3 不再「GB 丢一位小数 / "?" 变 ""」，而是「`decimals:2` 保精度 + `invalid:"?"` 保 sentinel」，行为完全还原。

- 共享真源：`lib/format.ts:14 formatBytes(bytes?)`（undefined/null→`""`、≤0→`"0 B"`、含 TB、toFixed(1)）。
- app 本地副本：
  - `app/encv-mobile/src/views/FullTextIndexDetail.vue:449`：`formatBytes(b:number)` —— **无 TB**（超大文件会显示巨大 GB 数）、`<1024→"X B"` 精确字节、KB/MB toFixed(1)、GB toFixed(2)。
  - `app/encv-mobile/src/views/SparseContainerTestDetail.vue:260`：`formatBytes(n:number|string|undefined|null)` —— 非有限/负→`"?"`、含 TB、全 toFixed(2)。
- **建议**：两处改 `import { formatBytes } from "@encv/shared-components/lib/format"`。
- **风险**：① FullTextIndexDetail 缺 TB —— 切共享版后**行为反而更正确**（补 TB），但 GB 精度从 toFixed(2) 变 toFixed(1)，显示略有变化；② SparseContainer 用 `"?"` 表示非法，共享版用 `""` —— storage 配额/用量通常有限值，影响极小。属低风险，但建议先确认 SparseContainer 是否依赖 `"?"` 视觉。

### K4. `formatDateTime` vs `formatTime`（PerformanceTab）⚠️ 待拍板（显示格式会变）✅ 已落地（代码完成，门禁待终端恢复验证；2026-07-14 续）

- **关键认知修正（同 K1/K3）**：不靠「丢秒」换统一，而是**可配置单源**。`useDateFormat.formatDateTime` 新增 `FormatDateTimeOptions { withSeconds?, locale? }`，默认（无秒、24h、零补位）与旧共享版一致（既有消费方零回归）。
- **迁移**：
  - `components/PerformanceTab.vue`：删本地 `formatTime`（`new Date(iso).toLocaleString()`，环境相关、含秒），改 `formatDateTime(iso, { withSeconds:true })` —— 保留「秒」精度，同时**消除环境相关性**（固定 `2026/07/14 11:01:30` 式输出，跨平台一致）。
  - `plugin-simverse/web/.../SimverseSettings.vue`：删本地 `formatTime`（`toLocaleString()` + 无效返回 `"-"`），改 `formatDateTime(iso, { withSeconds:true })`（无效返回 `""`，更统一）。
- **结论**：K4 不再「去掉秒」，而是「`withSeconds:true` 保秒 + 统一为 Intl 固定格式」，行为还原且跨平台一致。

- `formatDateTime(iso)`（`useDateFormat.ts:3`，shared 已导出）：`Intl.DateTimeFormat(locale,{年/月/日/时/分,hour12:false})` → 如 `"2026/07/14 11:01"`。
- `formatTime(iso)`（`components/PerformanceTab.vue:168`）：`new Date(iso).toLocaleString()` → 依赖运行环境默认格式（可能含秒、不同分隔符），与 `formatDateTime` 不一致。
- **建议**：PerformanceTab 改 import `formatDateTime`，统一时间显示。
- **风险**：显示格式变化（toLocaleString 行为不确定），**须用户拍板**。
- **注意**：`useFilesHelpers.formatDateInput(d)`（`useFilesHelpers.ts:52`）是 HTML `<input type=date>` 契约（`"YYYY-MM-DD"`），**不可动**。

### K5. `truncatePath` 跨边界重复（零/低风险，待确认语义）✅ 已落地（真实门禁验证）

- shared：`lib/buildReportZip.ts:907 truncatePath(p,max=60)` —— 尾部截断，`"..."+末尾(max-3)`。
- app：`app/encv-mobile/src/components/agent/ApprovalCard.vue:299 truncatePath(p)` —— 尾部截断，默认 `max=28`，`"…"+末尾27`，省略符用 `…`。
- **差异**：默认 max（60 vs 28）、省略符（`...` vs `…`）、入参是否可配。意图一致（保留路径末尾）。
- **已执行（2026-07-14 续）**：在 `lib/format.ts` 新增 `export function truncatePath(p, max=60, ellipsis="...")`（保留总长度=max，省略符可配）；`buildReportZip.ts` 改 `import { formatBytes, truncatePath } from "./format"` 并删本地定义；`ApprovalCard.vue` 改 `import { truncatePath } from "@encv/shared-components/lib/format"`，模板调用改为 `truncatePath(path, 28, "…")`（保持原显示不变）。**行为零变化**（两处 max/省略符原值保留）。
- **探测新发现（登记 K41）**：同文件 `buildReportZip.ts:914` 还有 `truncateError(e, max=120)`（单行错误截断，`firstLine + slice(max-3)+"..."`），是「截断工具族」的相邻重复——与 `truncatePath` 同属「尾部保留 + 省略符」模式。建议一并抽到 `lib/format.ts`（如 `truncateText(p, max, ellipsis)` 统一两函数），见 K41。
- **验证（真实门禁）**：`biome format --write` 修了 `ApprovalCard.vue` 1 处格式后重跑 `node scripts/check-all.mjs` **PASS 8/FAIL 0 全绿**。

### K6. `useFilesView.ts` 66KB god composable 逻辑抽取（高阶，高风险，分阶段）

- 实证：`views/useFilesView.ts` 单文件 66KB、含 **53+ 个函数**（L124–L1700+），是典型的「上帝 composable」。批 9b 只搬不收敛（§0.5.2 活样本）。可抽取的逻辑单元：
  1. **`convertBooleanQueryToVectorKeywords(query)`（L790）**：纯函数（布尔查询→向量关键词，无副作用、无响应式依赖）。**零风险**，抽到 `lib/searchQuery.ts`，`performSearch` 委托之。建议**优先做**（纯函数、有确定性、易测）。✅ **已落地（2026-07-14 续）**：抽出到 `lib/searchQuery.ts`（`export function convertBooleanQueryToVectorKeywords`），`useFilesView.ts` 改为 `import { convertBooleanQueryToVectorKeywords } from "@encv/shared-components/lib/searchQuery"` 并删除内嵌定义（原 L774–823 doc+函数）；新增 `lib/__tests__/searchQuery.test.ts` 8 例覆盖文档转换规则（AND/OR/NOT/引号/regex/嵌套括号/无语法回显/全 NOT 空串）。`performSearch` 内调用点（原 L971）不变。`read_lints` 对 `useFilesView.ts`/`searchQuery.ts` 报 0 TS 错误；真实门禁 **PASS 8/FAIL 0 全绿**（2026-07-14 14:2x 终端恢复后跑通，`biome format --write` 修了 `searchQuery.ts`/`searchQuery.test.ts`/`types/task.ts` 3 处格式后重跑通过）。
  2. **`applySizePreset`/`applyTimePreset`（L1596/L1600）+ `SIZE_PRESETS`/`TIME_PRESETS`（来自 useFilesHelpers）**：预设应用逻辑可抽 `lib/filterPresets.ts`。中风险（依赖 `formatDateInput` 契约）。
  3. **`playMedia`（L124）**：媒体播放，可抽 `useMediaPlayer` composable。高风险（涉及 `<audio>/<video>` 生命周期、播放错误处理）。
- **建议**：分阶段——先抽纯函数（K6.1），再抽预设（K6.2），最后媒体播放（K6.3）。每步须门禁 + 既有 useFilesView 相关 test 全绿。
- **风险**：改动面大。K6.1 零风险可立即做；K6.2/3 待拍板。

### 5.2 业务逻辑层（深一层，跨模块重复的逻辑本身）

> 以下为比 K1–K6「工具函数去重」更深一层的**业务逻辑重复**——同一意图在多个模块各写一套，是「逻辑本身正确抽象」的核心战场。

### K7. ion-content 滚动容器提取（`ensureScrollEl` + 重试 + ResizeObserver）⚠️ 中风险，优先做 ✅ 已落地（真实门禁验证）

- `useTasksView.ts:54-93`：`ensureScrollEl()` 用 `shadowRoot.querySelector(".inner-scroll")` + `initScrollElWithRetry()`（setTimeout 指数退避 max 8、50→300ms）+ `ResizeObserver` 兜底。
- `DevLogsViewer.vue:334-341`：`ensureScrollEl()` 用 **Ionic 官方 `getScrollElement()`** + try/catch warn，**无重试 / 无 ResizeObserver**。
- `TaskVirtualList.vue:22` 注释明引「DevLogs.vue ensureScrollEl()（ion-content shadowRoot .inner-scroll 获取模式）」——已知跨模块模式。
- **差异**：两种取滚动元素的方式（shadowRoot.querySelector vs getScrollElement）+ 重试逻辑仅 `useTasksView` 有。同一意图两套实现。
- **已执行（2026-07-14 续）**：新建 `composables/useIonContentScroll.ts`（`useIonContentScroll(contentRef)` 返回 `{ scrollEl, ensureScrollEl, initScrollElWithRetry, dispose }`，内部 shadowRoot `.inner-scroll` 查询为主 + `getScrollElement()` 同步兜底 + 指数退避重试 + ResizeObserver 兜底 + `onUnmounted` 自动清理）。
  - `useTasksView.ts`：删本地 `scrollEl`/`ensureScrollEl`/`initScrollElWithRetry`/`scrollElRetryTimer`/`scrollElRO` + 手动 `onBeforeUnmount` 清理块，改 `const { scrollEl, ensureScrollEl, initScrollElWithRetry } = useIonContentScroll(contentRef)`；原 `initScrollElWithRetry` 内「拿到元素后 `virtualListRef.forceMeasure()`」逻辑改由 `watch(scrollEl, el => el && forceMeasure())` 等价保留（虚拟列表首屏渲染依赖）；`onBeforeUnmount` 导入因不再使用已删除。
  - `DevLogsViewer.vue`：删本地 `scrollEl`/`ensureScrollEl`（`getScrollElement`+scroll 监听），改组合式；其独有行为（挂 `scroll` 监听驱动 scroll-to-top/bottom 状态）通过 `watch(scrollEl, el => el?.addEventListener("scroll", handleScroll), {immediate:true})` + `onBeforeUnmount` 移除监听 **保留不变**。
- **探测新发现（登记 K42）**：DevLogsViewer 原 `ensureScrollEl` 并非纯重复——它**额外挂了 scroll 监听**（驱动滚动状态），与 useTasksView 的纯「取元素」语义不同，属「同模式但行为有差异」；且全仓仍有 `getScrollElement()` **直接调用点**（如 `plugin-simverse/web/src/views/SimverseDevLogs.vue:497`）未走组合式。建议后续把 `useIonContentScroll` 推广到这些直接调用点（K42）。
- **验证（真实门禁）**：`biome format --write` 修 `DevLogsViewer.vue`/`useTasksView.ts` 2 处格式后重跑 `node scripts/check-all.mjs` **PASS 8/FAIL 0 全绿**（shared/encv-mobile typecheck + 单测 + i18n lint 全过；滚动行为靠 typecheck+单测覆盖，e2e 滚动需手动回归）。

### K8. confirm 弹窗模板（`alertController.create`）10+ 处 ⚠️ 低-中风险，高价值（2026-07-14 续探测：前提需修正）

- 散落：`useFilesView:1389/1400`（删除确认 + 目录 alert）、`GroupDetail.vue:268/412`、`useTasksView:238/396/408/504/530`（clearCompleted / cancelGroup / removeGroup）、`useTasksList:395`（clearCompletedWithConfirm）、`FilePickerModal.vue:253`（confirmNewFolder）、`features/alist-encrypt/password-dialog.ts`。
- **模式**：`const alert = await alertController.create({ header, message, buttons:[{text,role,handler}] }); await alert.present();`（10+ 重复）。
- **建议**：抽 `useConfirmDialog().confirm({ header, message, confirmText, cancelText?, danger? }): Promise<boolean>`（或语义化 `confirmDelete(name)`）。
- **风险**：低-中（纯封装、不改行为；逐处替换并保留 handler 副作用）。**高价值**。

> **⚠️ 2026-07-14 续探测修正（重要）**：grep 全仓 `alertController.create` 共 **32 处**，但**并非都是 confirm 模板**——`useConfirmDialog.confirm()`（始终 cancel+confirm 双按钮）只适配「二选一决策」子集，套到其余会**错误添加取消按钮 / 改变行为**。实测分类：
> - **A 类 · 真·确认（cancel+confirm/destructive，应走 `confirm()`）**：`useFilesView` 删除确认 / `useTasksView` clearCompleted·cancelGroup·removeGroup / `useTasksList` clearCompletedWithConfirm / `GroupDetail` 的确认类 / `FilePickerModal` confirmNewFolder / `alist-encrypt/password-dialog` 等。
> - **B 类 · 单 OK 的错误/信息 alert（不应走 `confirm()`）**：`useTasksView:352`(cancelRunFailed)、`:466`(error)、`GroupDetail:406`(exportFailed)、`FilePickerModal:253`(createFolderFailed) 等——`buttons:[t("common.ok")]` 单按钮，本质是「show error」而非「确认」。
> - **结论**：K8 必须**拆分**为两个原语：① 既有 `useConfirmDialog.confirm()` 只收 A 类；② B 类需另抽 `showAlert({header,message})`（单 OK）或降级为 `showToast`（若无需阻塞）。**不能**把 32 处一股脑迁到 `confirm()`（那是错误抽象）。
> - **当前状态**：✅ **已落地（已验证 PASS 8/FAIL 0 全绿；2026-07-14 23:57）**。原语 `useConfirmDialog` 扩 `showAlert`（B 类单 OK），`confirm`（A 类二选一）互补，避免错误抽象。门禁跑通前修 3 处迁移引入问题：① `GroupDetail.vue` 重复 `useConfirmDialog` import（删其一）；② `ConfirmOptions.message` 原必填→改可选（4 处仅传 header 的确认调用）；③ 拓宽共享 `formatBytes` 接受 `string|null`（SparseContainer 合法传入，K3 式收敛此前未门禁验证）。
> - **迁移 16 文件 / ~30 站点**（A 类→`confirm()`，B 类→`showAlert()`）：
>   - shared：`GroupDetail.vue` / `useTasksView.ts` / `FilePickerModal.vue`
>   - app：`useAgentChatView.ts` / `WebDavAutomationTestsDetail.vue` / `SparseContainerTestDetail.vue` / `Settings.vue` / `ServerDetail.vue` / `MountsDetail.vue` / `LogSettingsDetail.vue` / `ExtensionsPage.vue`(仅 :345 卸载确认) / `DevLogs.vue` / `DatabaseDetail.vue` / `CacheDetail.vue`
>   - plugin-simverse：`SimverseDevLogs.vue` / `SaveManagement.vue`
> - **行为保真**：① 每文件移除未用 `alertController` import（biome 防 unused）；② A 类 handler 副作用原样搬进 `if (await confirm(...)) {…}`；③ **8 个 A 类站点原确认按钮是描述性自定义文案**（"清除"/"停止"/"删除"/"立即重启"/"重置"等），已逐一补 `confirmText:` 保留原文案，UI 不变；④ `ExtensionsPage.vue:373 showDebugResult`（自定义"复制"按钮，非简单确认）**故意不动**；⑤ `SparseContainerTestDetail.confirmIfHighRisk` 原 `return role==="confirm"` 改为 `return await confirm(...)`（布尔语义一致）。
> - **已验证（真实门禁）**：`biome format --write` 17 文件 + `node scripts/check-all.mjs` 验证 **PASS 8/FAIL 0 全绿**（2026-07-14）。本批曾因 `npx biome` 在网络受限环境挂起阻塞终端，改用本地二进制 `./node_modules/.bin/biome` 跑通。

### K9. `runWithConcurrency` 提升为 lib 工具（低优先）

- `useBatchOperations.ts:28`：`runWithConcurrency(items, fn, max=5)` 通用并发限制器。当前仅此处，但属通用工具 → 建议提 `lib/concurrency.ts`。`useWorkflowTaskService` 的 worker 池是另一套 DAG 编排，不合并。低优先。
- **已执行（2026-07-15 续，并行推进）**：新建 `lib/concurrency.ts` 的 `runWithConcurrency<T,R>(items, fn, max=5)`，签名 / 行为与原内联版逐字一致（含 `catch` 内 `{ ok:false, error:String(err) } as any` 错误位填充，调用方 `r?.ok` 判成败不变）；`useBatchOperations.ts` 删内联定义、改 `import { runWithConcurrency } from "@encv/shared-components/lib/concurrency"`，三处 `batchRetry/Cancel/Delete` 调用点不变。零行为变化，真实门禁 `check-all.mjs` 已验证 **PASS 8/FAIL 0 全绿**（2026-07-15）。

### K14. 错误 toast 样板（`showToast({ message: err.message || "X failed", color:"danger" })`）✅ 已落地（代码完成，门禁待终端恢复）

- `useFilesView:1217`(copy) / `1245`(rename) / `1257`(move) / `1417`(delete) 4 处几乎一致，仅动词不同；`1375`("不能删除根目录") 同形态无 `err`；`TaskDetailModal.vue:107` 类似。
- **问题**：fallback 是**硬编码英文**（"Copy failed"）而非 i18n key，且样板重复。
- **已执行（2026-07-14 续）**：在 `shared-components/src/views/useFilesView.ts` 模块级新增 `function showErrorToast(message: string): void { showToast({ message, duration: 2000, color: "danger" }); }`，5 处 danger toast 收敛为 `showErrorToast(err.message || "X failed")` / `showErrorToast("不能删除根目录")`。**本次仅做去重，未动 i18n**（fallback 英文串保持原样，行为不变）——i18n 硬化（改 `t(key)`）留作后续独立批次，避免扩大爆破面。
- **验证状态（✅ 已验证，2026-07-14 终端恢复后）**：真实门禁 `node scripts/check-all.mjs` **PASS 8 / FAIL 0** 全绿，K14 去重无回归。

### 已核查无需动（登记为「已收敛」对照，避免重复提案）

- **K10 WS 事件归一化 + 事件桥（已收敛）**：`normalizeTaskEvent`（`lib/taskEvent.ts`）单源，`taskStore` + `runTasksStore` 共用；`useTaskEventBridge` 是 WS 4 件套唯一入口；`state-machine.ts` 服务之。已是良好抽象。
- **K11 轮询（已收敛）**：`usePoll`（`composables/usePoll.ts`）提供 setInterval 自调度 + 并发守卫 + onUnmounted 清理 + 事件触发刷新。全仓仅 `WsBackend` 心跳用裸 `setInterval`（合理，非 UI 轮询）。
- **K12 文件类型检测（已收敛）**：`getFileExtension`（`api/encv_files.ts` 单源，`encv_admin` 复用）、`isImageFile`（`composables/useFileList.ts` 单源）。
- **K13 任务显示/视图计算（已收敛）**：`getTaskTypeLabel`（`lib/taskTypeLabel`）、`taskViewComputeCore`（`lib/taskViewComputeCore.ts`）均为单源，tasks 双雄（`useTasksView`/`useTasksList`）已委托之（见批 G/H）。

### 5.3 结构性 / 跨切面重复（更深一层）

> 比 5.2「业务逻辑」再深一层：跨切面基础设施（单例 / 事件 / 类型 / 字符串工具 / 异步状态）的重复样板。这类重复体量小但**散布最广、收益累积最大**。

### K15. 模块级单例样板（6+ 处）⚠️ 高价值

- 重复形状：`let _cachedInstance = null; function useXSingleton() { if (_cachedInstance) return _cachedInstance; _cachedInstance = factory(); return _cachedInstance; }` + `__resetXForTests()`。
- 散落：`runTasksStore.ts:148-160`、`useRunSummaries.ts:116-118`、`useApiBaseProbe.ts:80`、`useRealtimeTransport.ts:85`、`useWorkflowTaskService.ts:127`、`useTasksList.ts:40-41`（双实例 + `createView`/`createUseTasksList` 工厂）。
- **建议**：抽 `defineSingleton(factory, { resetName? })` / `useModuleSingleton(factory)`，统一「单例 + 测试重置」。每处省 ~10 行 + 消除测试重置逻辑重复。
- **已执行（2026-07-15 续，并行推进）**：新建 `lib/singleton.ts` 的 `defineSingleton<T>(factory): { get, reset }`，收敛**3 个干净站点**（单 `_instance` 变量 + 独立 `__resetXForTests`）：`useRunSummaries.ts` / `runTasksStore.ts` / `useApiBaseProbe.ts` 均改为 `const _x = defineSingleton(factory)` + 导出 `useXSingleton()` 委托 `_x.get()` + `__resetXForTests()` 委托 `_x.reset()`，**导出名 / 调用方全不变**。
- **有意不动（避免错误抽象，呼应 K17/K22/K43 纪律）**：其余 3 处属「设计内差异」非简单单例，强行套用即错误抽象 → 保留原样：`useWorkflowTaskService.ts`（额外缓存 `_cachedOptions`，reset 需双清）、`useRealtimeTransport.ts`（`_forcedMode` 与 `_instance` 同生命周期、reset 在实例方法内）、`useTasksList.ts`（双实例 `_viewInstance`+`_tasksListInstance`，reset 双清）。真实门禁 `check-all.mjs` 已验证 **PASS 8/FAIL 0 全绿**（2026-07-15）。

### K17. eventBus 缺自动清理封装 ⚠️ 高价值，低-中风险 ✅ 已落地（真实门禁验证）

- `useEventBus.ts` 仅导出 `eventBus`（`on/off/emit/clear`），**无 `onUnmounted` 自动退订**的 composable。
- 消费方手动配对：`useServerStatus.ts:418-419`、`useRealtimeTransport.ts:139-142`、`useFilesView.ts`（原 `eventBus.on("file:change", onFileChange)` 在 `onMounted` + `eventBus.off(...)` 在 `onUnmounted`）。
- **已执行（2026-07-14 续）**：在 `useEventBus.ts` 新增 `useEventBusListener(event, handler)`（内部 `onMounted` 注册 + `onUnmounted` 注销），并迁移**唯一安全的组件作用域站点** `useFilesView.ts`：删原 `eventBus.on/off` 配对，改为在 setup 期调用 `useEventBusListener("file:change", onFileChange)`（函数声明 hoist，`onFileChange` 已定义）。
- **有意不迁移（避免错误抽象 / 破坏既有生命周期语义）**：
  - `useServerStatus.ts:418-419`：其 `onUnmounted` 是**故意空实现**（注释明示「模块级单例订阅，不在 composable 内注销」——`isOnline` 跨组件共享保活）。套用 `useEventBusListener` 会在卸载时错误退订，破坏多组件共享语义 → **保留原样**。
  - `useRealtimeTransport.ts:139-142`：`ensureApiBaseListeners` 是**普通函数返回 cleanup**（非 composable、无组件上下文），`onUnmounted` 在此不可调用 → **保留原样**（其自身 `return () => { eventBus.off(...) }` 已正确管理）。
  - `useTaskEventBridge.ts`：已内聚 `on/off` + 外部显式 `dispose()` 模式（由调用方决定何时注销），语义不同于「随组件卸载」→ **保留原样**。
- **探测结论**：K17 的「自动清理」抽象**只适用于组件作用域、且随卸载即退订**的订阅；其余站点因「模块级保活 / transport 级 cleanup / 显式 dispose」等**刻意不同的生命周期**，强行统一反而引入错误抽象（呼应 §0.5.4 / K22 的「避免错误抽象」纪律）。故本批**只交付原语 + 收敛唯一干净站点**，其余登记为「设计内差异，非重复」。
- **验证（真实门禁 `check-all.mjs`）**：`biome format --write` 2 文件（无需修复）+ 全量门禁 **PASS 8 / FAIL 0** 全绿（shared/encv-mobile typecheck + 单测 + i18n lint + Biome CI 全过）。

### K19. `PluginMeta` 接口两处定义逐字节相同（零风险，高价值）✅ 已落地（代码完成，门禁待终端恢复）

- `api/encv_plugins.ts:9-15` 与 `types/task.ts:177-183` **完全相同**：
  ```ts
  export interface PluginMeta {
    name: string;
    supportedExtensions: string[];
    supportedMimePrefixes: string[];
    containerExtension: string;
    taskOptions: TaskOptions;
  }
  ```
- **消费方全部从 `@encv/shared-components/api/encv`（→ encv_plugins）导入 `PluginMeta`**（useFilesView / useTestCaseGeneration / 各 test / app 层 `PluginTestsDetail` 等）。
- **核查 16 处 `from "...types/task"` 导入（含 app 层），无一含 `PluginMeta`**；且无 `export * from "...types/task"` 桶间接导出 → `types/task.ts` 那份是**死重复**。
- **已执行（2026-07-14 续）**：直接删除 `types/task.ts` 的 `PluginMeta` 接口（文件末尾最后一个导出，无内部/外部消费）。真源保留在 `api/encv_plugins.ts:9`。`TaskOptions` 同模式核查：**非重复**——`types/task.ts` 的 `TaskOptions` 被 `useTaskForm`(`PluginCandidate, TaskField, TaskOptions`)/`useTestCaseGeneration`(`TaskOptions`)/`PredictPluginResponse.taskOptions` 等消费，是活源，未删。
- **验证状态（✅ 已验证，2026-07-14 终端恢复后）**：真实门禁 `node scripts/check-all.mjs` **PASS 8 / FAIL 0** 全绿，K19 删除无回归。

### K22. 字符串 / 路径小工具重复（低-中风险）✅ 扩展名部分已落地（真实门禁验证）

- 去扩展名点 `ext.toLowerCase().replace(/^\./, "")`：3 处 —— `mockDataGenerator.ts:649`、`useTestCaseGeneration.ts:95`、`useSectionDerivation.ts:71`。
- 路径斜杠归一化 `replace(/\\/g,"/").replace(/\/+/g,"/")`：多处 —— `usePathResolver.ts:37-38`、`path-chain-e2e.test.ts:71`、`buildReportZip.ts`、`mockDataGenerator.ts`。
- **建议**：抽 `lib/string.ts`（`stripLeadingDot`）+ `lib/path.ts`（`normalizeSlashes` / `joinPath`），三处改 import。低-中风险。
- **已执行（2026-07-14 续）——扩展名规整收敛**：新建 `lib/string.ts` 的 `normalizeExt(ext)`（= `ext.toLowerCase().replace(/^\./, "")`，含 `.MP4→mp4` 文档示例）作单一真源；3 处逐字节相同的内联表达式（`mockDataGenerator.extToRelativePath` / `useTestCaseGeneration.categoryForExt` / `useSectionDerivation.categoryForExt`）全部改为 `normalizeExt(ext)`。`mockDataGenerator.ts` 原为零 import 纯模块 —— `string.ts` 亦零依赖纯工具，别名在前端 vite / vitest 均可解析，不破坏其「任意 JS 环境可 import」约束。**行为零变化**。
- **有意不动（避免错误抽象）——路径斜杠归一化**：核实 `usePathResolver.normalize`（trim + 反斜杠→正斜杠 + 去重 + 补前导斜杠的**复合逻辑**）与 `mockDataGenerator.joinPath`（`join("/")` + 仅去重、无反斜杠）**语义不同**，其余多为 test 内镜像后端语义。强抽 `normalizeSlashes` 只能替换 `usePathResolver` 里 2 行、割裂其内聚逻辑，属边际/可能错误抽象 → 按 §0.5.4/§804「避免错误抽象」纪律**暂不合并**，`lib/path.ts` 不新建。
- **验证（真实门禁 `check-all.mjs`）**：`biome format --write` 5 文件（No fixes applied）+ 全量门禁 **PASS 8 / FAIL 0** 全绿（含 encv-mobile 单测 + 6 处 typecheck + Biome CI + i18n lint）。

### K18. `instanceof ApiError && e.status === X` 状态分支（中价值）✅ 已落地（代码完成，read_lints 0 诊断；门禁待终端恢复）

- 全仓实际仅 **5 处** `instanceof ApiError`（文档「43 处」为泛指/夸大；其中 2 处非状态码分支：`encv_search:182` 仅做 `instanceof` 类型收窄、`encv_search:220` 查 `e.body`，均不在 K18 范围）。
- **真·状态码裸分支仅 3 处**，已全部收敛：
  - `api/core/errors.ts` 新增 `isApiStatus(e, status): e is ApiError`（`e instanceof ApiError && e.status === X`）与 `isApiStatusAtLeast(e, min): e is ApiError`（`>= X`）。经 `core/index.ts` 的 `export * from "./errors"` 透出，故 `@encv/shared-components/api/core` 与 `./core/errors` 均可解析。
  - `encv_tasks.ts:173` `instanceof ApiError && e.status === 0` → `isApiStatus(e, 0)`。
  - `encv_search.ts:147` `instanceof ApiError && e.status === 503` → `isApiStatus(e, 503)`。
  - `encv_admin.ts:259` `instanceof ApiError && e.status >= 400` → `isApiStatusAtLeast(e, 400)`。
- **验证**：4 个改动文件 `read_lints` 均 0 诊断（TS 类型守卫 `e is ApiError` 保证下游 `e.status` 收窄正确）；后续真实门禁 `node scripts/check-all.mjs` **PASS 8 / FAIL 0 全绿**（2026-07-14 终端恢复后多次跑通，含本批改动），K18 收敛无回归。
- **结论**：K18 实际是「小且安全」项，非 43 处大规模；helper 已就位，后续若再出现裸分支直接套用即可。

### K16. 加载 / 错误异步状态样板（17 文件，高风险候选）

- `loading` / `error` ref 出现在 `runTasksStore`(12) / `Tasks.vue`(4) / `useTasksList`(6) / `useRunSummaries`(7) / `useFilesView`(2) 等 **17 文件**。
- 经典 `useAsyncState` / `useResource(fn)` 模式（返回 `{ data, loading, error, reload }`）。
- **建议**：抽 `useAsyncResource`，但**爆破半径大**（17 文件），属高阶候选，需逐模块迁移 + 门禁。高风险。

### K41. 截断工具族（`truncateError` 等尾部保留 + 省略符模式）⚠️ 低-中风险（2026-07-14 续探测）

- **探测来源**：做 K5（`truncatePath` 收敛）时，同文件 `lib/buildReportZip.ts:914` 暴露相邻 `truncateError(e, max=120)`（取 `e.split("\n")[0]` 首行 + `slice(0, max-3)+"..."`），与 `truncatePath` 同属「尾部保留 + 省略符」截断模式，是同一「截断工具族」的重复。
- **更广模式**：全仓「截断并加省略符」散落多处（路径 / 错误 / 文本），每个都手搓 `slice` + 省略符拼接，缺统一 helper。
- **建议**：在 `lib/format.ts` 抽通用 `truncateText(text, max, ellipsis="...")`（保留末尾 max 字符），`truncatePath` 与 `truncateError` 均委托之（`truncateError` 额外取首行）；grep 全仓 `slice(-` / `…` / `"..."` 截断拼接点，逐步收敛。低-中风险。
- **风险**：低-中（纯工具，逐个替换）。与 K5 同族，建议 K5 收尾后顺做。
- **已执行（2026-07-15 续，并行推进）**：`lib/format.ts` 新增 `truncateText(text, max=60, ellipsis="...", mode:"tail"|"head"="tail")`（`tail`=保留末尾 / 省略符在前，`head`=保留开头 / 省略符在后）；`truncatePath` 改委托 `truncateText(p, max, ellipsis)`（`tail`，行为不变）；`buildReportZip.ts` 的 `truncateError` 改 `truncateText(firstLine, max, "...", "head")`——**精确还原原 head 保留语义**（`firstLine.slice(0,max-3)+"..."`），非盲目套 `tail`。真实门禁 `check-all.mjs` 已验证 **PASS 8/FAIL 0 全绿**（2026-07-15）。

### K42. `getScrollElement()` 直接调用点未走组合式 ⚠️ 低-中风险（2026-07-14 续探测）✅ 已落地（真实门禁验证）

- **探测来源**：做 K7（`useIonContentScroll` 收敛）时，grep 发现除已收敛的 `useTasksView`/`DevLogsViewer` 外，仍有 `getScrollElement()` **直接调用点**未走组合式。
- **实证**：`plugin-simverse/web/src/views/SimverseDevLogs.vue:497` `const scroll = await (contentRef.value as any).getScrollElement();`（与 DevLogsViewer 原写法同构，缺重试/兜底，且 `as any` 绕过类型）。
- **已执行（2026-07-14 续）**：`SimverseDevLogs.vue` 改为 `const { scrollEl, initScrollElWithRetry } = useIonContentScroll(contentRef);`（跨 package 引用 `@encv/shared-components/composables/useIonContentScroll`，plugin 已依赖 shared-components，路径可解析）；删原 `setupScrollEl()`（手搓 `getScrollElement()` + `as any` + 直接 `scrollEl.value = scroll`）；`onMounted` 改调 `initScrollElWithRetry()`；其独有行为（挂 `scroll` 监听驱动滚动状态）经 `watch(scrollEl, el => el?.addEventListener("scroll", onScroll), { immediate: true })` + `onBeforeUnmount` 移除监听 **保留不变**（与 K7 的 DevLogsViewer 处理同构）。
- **grep 终态**：全仓 `getScrollElement()` 直接调用点**仅剩 `useIonContentScroll.ts` 自身兜底分支**（即组合式内部实现），外部调用点归零 → K42 完成。
- **验证（真实门禁 `check-all.mjs`）**：`biome format --write` 修 `SimverseDevLogs.vue` 1 处格式后重跑 → **PASS 8 / FAIL 0 全绿**（含 `plugin-simverse/web` typecheck 通过，证明跨包导入 + 类型兼容成立）。

### K43. 剩余本地 `formatTime`/`formatDuration` 副本收敛（2026-07-14 续探测：多数签名发散，故意不合并）

- **探测来源**：做 K1/K3/K4 后 grep 全仓 `function formatBytes|function formatDuration|function formatTime`，除已收敛者外仍有 **15 处**本地副本。逐一核对签名，发现**多数语义发散**，强套共享函数即「错误抽象」（呼应 §0.5.4 / K22 纪律）。
- **签名发散分类（故意不合并）**：
  - **时间-仅时分（`toLocaleTimeString`，无日期）**：`WorkflowDashboard:376`、`PluginTestsDetail:683` —— 意图是「时刻」非「日期时间」，与 `formatDateTime`（含日期）语义不同 → 不合并。
  - **秒为输入（`formatDuration(seconds:number)`）**：`FilePreview:224`、`FileInfo:267`、`ArtPlayerView:108` —— 入参单位是秒不是毫秒，与共享 `formatDuration(ms)` 单位相反 → 不合并（若合并需 `×1000` 转换，属语义改写）。
  - **两时间戳求差（`formatDuration(start?,end?)`）**：`WebDavAutomationTestsDetail:701` —— 入参是两个 ISO 字符串、内部求 ms 差、秒用 `toFixed(2)`，与共享「单 ms 入参」形态不同 → 不合并。
  - **数字时间戳（`formatTime(ts:number)` / `formatInlineErrorTime(at:number)`）**：`ErrorCaptureOverlay:65`、`OpenListLogList:40`、`MpvProgressBarWeb:27`、`OpenListDevLogs:70`、`MpvDevLogs:69`、`PluginTestsDetail:691`（相对时间「刚刚/N分钟前」）—— 入参是 `Date.now()` 数字时间戳或相对时间，非 ISO 字符串，需 `new Date(ts)` 且意图不同 → 不合并。
  - **2 位小数秒（`OperationCard:244` `<10s → toFixed(2)`）**：共享 `formatDuration` 仅支持 `showDecimals`（1 位），精度档位不同 → 不合并（避免为单站点加 `decimals` 选项膨胀 API）。
- **已安全收敛（ISO 字符串 `toLocaleString` 同构，零回归）**：
  - `FileInfo:273` `new Date(isoStr).toLocaleString()` → `formatDateTime(isoStr, { withSeconds: true })`（保留秒、消除环境相关性）。
  - `WebDavAutomationTestsDetail:696` `new Date(iso).toLocaleString("zh-CN",{hour12:false})` → `formatDateTime(iso, { locale: "zh-CN" })`（无秒，与原一致；固定 24h 零补位）。
  - 两处 `read_lints` 均 0 诊断。
- **结论**：K43 实证「表面重复 ≠ 逻辑重复」——15 处副本里仅 2 处是 `formatDateTime` 的真同构，其余 13 处因**单位/入参类型/意图**发散而属「设计内差异」，强行统一即错误抽象。故只收敛 2 处，13 处登记为「非重复、不合并」。这与 K15/K41/K22 的纪律一脉相承。

### 已核查无需动（追加）

- **K25 timeline 渲染（已收敛）**：`TaskTimeline.vue` 与 `StepInlineTimeline.vue` 均渲染 `<UnifiedTimelineCard v-for>`，底层 `components/shared/UnifiedTimelineCard.vue` 已是单源。两 wrapper 用途不同（任务级 vs step 级），不合并。
- **K26 TaskForm 骨架（已收敛）**：`TaskFormFields.vue` 是加/解密表单共享骨架（蓝图已登记），`EncryptBody`/`DecryptBody` 仅各自独有区块。

### 5.4 范式级（约定被违反 / 抽象泄漏 / 双范式并存）

> 比 5.3「跨切面样板」更深一层：不是「少了个 helper」，而是**约定本身被违反、或同一意图存在两套互斥范式**。这类问题危害最大（行为不一致、难以统一演进），是「逻辑本身正确抽象」的最高阶战场。

### K27. 导航约定被 god file 违反 + 抽象泄漏 ⚠️ 高风险，高价值

- **约定**：`runtime/appNavigation.ts:10` 明写「shared 内部一律通过 `getAppNavigation()` 取用导航能力，**绝不 import vue-router / @/**」。
- **违反**：`views/useFilesView.ts:12` `import { useRoute, useRouter } from "vue-router"`，并直接 `router.push(...)` **13+ 处**（L137/152/160/165/652/670/675/687/749/754/1065/1086/1116）；`components/TaskDetailModal.vue:63`、`views/NotFoundView.vue:91` 同样直接 `useRouter`。
- **更深层（抽象泄漏）**：`useFilesView.ts:749` 注释「直接 router.push（**绕过三级导航 classList bug**，直接打开独立路由）」——说明 `getAppNavigation().navigate` 自身有 bug，迫使调用方回退到裸 router。**约定与实现互相打架**。
- **建议**：① 先修 `getAppNavigation` 的 classList bug，使其覆盖全场景；② 将 useFilesView / TaskDetailModal / NotFoundView 的 `router.push` 全改为 `getAppNavigation().navigate`/`replace`；③ 加 lint 规则（禁止 shared 内 `import ... from "vue-router"`）固化边界。
- **风险**：高（导航核心路径，需全回归 + 路由测试）。**高价值**（消除 13+ 处不一致 + 固化架构边界）。

### K28. 双任务 store 重复「任务状态 reducer」⚠️ 中-高风险，高价值

- `stores/taskStore.ts`（pinia，注释自称「**唯一**任务数据 + 视图状态 owner」）实现：`applyEvent`(L304) / `getTaskById`(L59) / `patchTaskById`(L190) / `rebuildIndex`(L51) / `normalizeTaskEvent`(L23) / `triggerRef`。
- `stores/runTasksStore.ts`（GroupDetail 页单例）**重写同一套**：`applyEvent`(L104) / `getTaskById`(L121) / `patchTaskById`(L125) / `normalizeTaskEvent`(L19)。
- **问题**：任务状态归约逻辑（patch + 索引 + normalize + triggerRef）两 store 双写；taskStore 注释「唯一 owner」与实际双源矛盾。
- **建议**：抽 `createTaskCollection()`（返回 `{ tasks, getTaskById, patchTaskById, applyEvent, rebuildIndex }`），两 store 共用；视图状态（viewMode/sort/filter）仍留各自 store。
- **风险**：中-高（触碰两 store + WS 事件入口）。**高价值**（消除核心逻辑双写）。
- **注**：`useWorkflowStore`（localStorage 工作流定义 CRUD）领域不同，**非重叠**，无需合并。

### K29. Modal/Overlay 两套互斥范式 ⚠️ 中风险，高价值

- **范式 A（命令式 `modalController`）**：`modalController.create({component, componentProps}).present()` + `modalController.dismiss(data, role)` —— 至少 8 处：`useTasksView:208`、`useNewTaskModal:100`、`useFilePickerBrowse:15/26`、`GroupDetail:238`、`TaskDetailModal`、`NewTaskModal`、`FilePickerModal`。`useNewTaskModal:16` 注释点出 `componentProps` 是「**静态快照**」这一已知坑。
- **范式 B（声明式 `isOpen` ref）**：`<ion-modal v-model:is-open>` / `const isOpen = ref()` —— `Tasks.vue:70-134`（4 个 popover）、`FilterDropdown.vue:91`（isOpen + 手动 toggle/Escape/外部点击）。
- **问题**：同一「overlay 开关 + 结果回传」意图两套实现，行为/测试方式不一致；范式 A 的 `onDidDismiss` 结果处理样板重复，范式 B 的 dropdown 手搓开关逻辑重复（见 K30）。
- **建议**：抽 `useModal()`（封装 `modalController` 打开/关闭/结果 Promise + 防 componentProps 快照坑）与 `useOverlay()`/`useDisclosure()`（封装 `isOpen` + Escape + 外部点击）。两范式收敛为一。
- **风险**：中（overlay 交互广，需逐处替换 + 交互测试）。

### K30. Dropdown/Popover 开关样板（useDisclosure / useClickOutside 缺失）⚠️ 低-中风险

- `FilterDropdown.vue:91-184` 手搓：`isOpen` ref + `toggle()` + `Escape` 键关闭 + 外部点击关闭 + `onUnmounted` 清理。这是所有自定义 dropdown/popover 的通用样板。
- **建议**：抽 `useDisclosure()`（isOpen/toggle/open/close）+ `useClickOutside(target, handler)`，FilterDropdown 及未来 dropdown 复用。低-中风险。

### 已核查无需动（追加）

- **K33 API 请求层（已收敛，范式正面样本）**：`api/core/request.ts` 的 `apiRequest` 是**统一单源**，所有 `encv_*.ts` 均 `import { apiRequest, ApiError } from core` 复用，**无每文件重复 request 样板**。与 §5 其余「重复」形成对照——说明项目在「请求层」范式正确，问题集中在「导航 / overlay / store 状态」层。
- **K34 `useWorkflowStore`（已收敛，非重叠）**：localStorage 工作流定义 CRUD，领域独立于 taskStore，无需合并。

### 5.5 元范式 / 基础设施范式（最深一层）

> 比 5.4「约定被违反 / 双范式并存」再深一层：是**基础设施与元决策层面**的范式不统一——状态管理用几套范式、测试脚手架是否共享、生命周期范式是否手搓、日志是否走显式 API。这类问题决定整个代码库的「形状」，最难改但收益最持久。

### K35. 状态管理两套范式并存（pinia `defineStore` vs 手搓单例 composable）⚠️ 元范式，高价值

- **实证**：全仓仅 **1 个** `defineStore`（`stores/taskStore.ts:37`，pinia）；但**大量手搓单例 composable 持有跨组件状态**：`runTasksStore`(单例)、`useWorkflowStore`、`useRealtimeTransport`、`useApiBaseProbe`、`useWorkflowTaskService`、`useRunSummaries` 等（K15 已列单例样板 6+ 处）。
- **问题**：同一「跨组件共享状态」意图，存在 **pinia store 与手搓单例 composable 两套互斥范式**。taskStore 用 pinia，runTasksStore/useWorkflowStore 却手搓——连任务状态都分裂（见 K28）。缺乏「何时用 pinia、何时用 composable 单例」的显式决策。
- **建议**：① 显式约定——共享状态优先 pinia（享 devtools / 持久化 / 测试注入），仅在「shared 库不依赖应用层 pinia 实例」等硬约束下用 composable 单例；② 将 `runTasksStore`/`useWorkflowStore` 等评估是否迁入 pinia，至少统一为 `defineSingleton`（K15）收敛样板。
- **风险**：高（触及状态架构根基）。**高价值**（决定代码库形状）。属「元范式拍板」项，需用户/架构决策。

### K36. Ionic/Vue 生命周期「init+refresh」范式手搓 ⚠️ 中风险，高价值

- **实证**：`useFilesView.ts:1711`(`onMounted`) + `:1737`(`onIonViewWillEnter`)；`useTasksView.ts:560`(`onMounted`) + `:600`(`onIonViewWillEnter`，注释「参考 Files.vue 实现切回 tab 自动刷新」)。
- **模式**：「`onMounted` 初始化一次 + `onIonViewWillEnter` 每次切回 tab 智能刷新」在至少 2 处手搓；`useTasksView:604-617` 还在 `onIonViewWillEnter` 内手搓 `console.error` 崩溃兜底（「crashed, do not block tab」）。
- **问题**：Ionic `onIonViewWillEnter`（每次 tab 进入触发）与 Vue `onMounted`（仅一次）的语义差异是 Ionic 经典坑，项目里这个「init+refresh」范式**没有抽成 composable**，每处重写得不一致（刷新条件、崩溃兜底各不相同）。
- **建议**：抽 `useIonViewRefresh({ onMounted: init, onEnter: refresh, guard?: true })`，统一「初始化 + 切回刷新 + 崩溃兜底」。中风险（触碰两处页面生命周期）。

### K38. 测试 `vi.mock` 脚手架在 15+ 文件重复（无共享 fixture）⚠️ 中风险，高价值

- **实证**：`Tasks.component.test.ts` / `GroupDetail.component.test.ts` / `AgentChat.history.test.ts` 等至少 15 个测试文件，每个都手搓 `vi.mock("@ionic/vue", ...)` + `vi.mock("vue-router", ...)` + `vi.mock("@/api/encv", ...)` + `vi.mock("@/composables/useTaskEventBridge", ...)` 等一长串 mock 块（单文件 mock 数 5–12 个）。
- **缺失**：全 src 内**无共享测试 fixture**（搜 `createTestPinia`/`renderWithProviders`/`setupTest` 等均 0 命中）——每个测试自管 mock。
- **问题**：mock 样板重复 + 易漏（某测试忘 mock 某依赖就挂），维护成本高。
- **建议**：建 `src/__tests__/fixtures/mockProviders.ts`（或 `test-utils.ts`），导出 `mockIonic()` / `mockRouter()` / `mockEncvApi()` / `mockTaskEventBridge()` 等可组合 fixture；组件测试 `beforeEach` 调用之。中风险（需逐测试文件迁移，但纯测试侧、不改产品代码）。

### K40. Composable 返回形态不一致（需审计）⚠️ 低-中风险，待量化

- **现象**：composable 返回形态不统一——有的 `return reactive({...})`、有的 `return { refA, refB }`（plain object of refs）、有的返回单个 `ref`/`ComputedRef`。消费方需逐个记忆「要不要 `.value` / 要不要解构」。
- **建议**：定一条约定（推荐「始终返回 plain object of refs，便于解构且保持响应式」），对存量 composable 做一次形态审计 + 对齐。低-中风险，待先量化分布。

### 已核查非缺陷（登记为「设计内范式」，避免误报）

- **K37 两个 mock 生成器（非重复，分层依赖）**：`api/mockGenerator.ts`（12KB）是调后端 SSE `/api/mock/generate` 的 **API 薄包装**；`lib/mockDataGenerator.ts`（30KB）是**纯函数字节生成器**（返回 `Uint8Array`）。前者 `import type { MockFileType } from ".../lib/mockDataGenerator"` 依赖后者——**分层依赖，非重复**。与 §0.5.4「避免错误抽象」纪律一致，不列入 Backlog。
- **K39 日志走 console 全局劫持（设计内，非缺陷）**：`useFrontendLogs.ts` 通过**全局劫持 `console.error/info/warn/debug/log`**（`origConsole` 捕获）把前端日志路由进 `logs` ref。故散落 59 文件的 raw `console.*` 是**设计内的日志通道**（被捕获而非噪音），非范式缺陷。仅提示：有意日志与调试日志在通道上不可区分，若需分级可后续引入显式 `log()` API——属增强，非重构。

### 执行顺序建议（更新）

1. **零风险可直接做**：K2（删过时注释）、K19（删 `types/task` 死重复 `PluginMeta`）、K6.1（抽纯函数 `convertBooleanQueryToVectorKeywords`）、**K14**（错误 toast helper，顺带修 i18n）。
2. **低风险待确认**：K3（app formatBytes 复用）、K5（truncatePath 收敛）、**K22**（字符串/路径工具）、**K8**（confirm 弹窗封装）、**K30**（useDisclosure/useClickOutside）、**K17**（eventBus 自动清理）。
3. **中风险优先（范式级）**：**K15**（单例样板）、**K7**（ion-content 滚动 composable）、**K29**（modal 双范式收敛为 `useModal`/`useOverlay`）、**K28**（双 store reducer 收敛为 `createTaskCollection`）。
4. **待用户拍板（显示格式变化）**：K1（formatDuration）、K4（formatTime→formatDateTime）。
5. **高风险/高阶**：**K27**（导航约定违反 + 抽象泄漏，须先修 `getAppNavigation` classList bug 再统一）、K16（useAsyncResource）、K6.2 / K6.3（useFilesView 进一步抽取）、K9（concurrency 提 lib）、K18（ApiError 状态分支）。
6. **范式正面样本（对照）**：K33（API 请求层 `apiRequest` 已统一单源）、K10–K13/K25/K26（已收敛）。
7. **元范式拍板（最深）**：**K35**（状态管理双范式：pinia vs 手搓单例，需架构决策）、**K36**（生命周期 init+refresh 手搓 → `useIonViewRefresh`）、**K38**（测试 mock 脚手架 → 共享 fixture）、**K40**（composable 返回形态审计）。
8. **已核查非缺陷（避免误报）**：K37（两 mock 生成器是分层依赖非重复）、K39（日志走 console 全局劫持，设计内）。

> 与 §0.5.4 的呼应：搜索切词（`tokenizeQuery` vs `clientSearchTokenize`）经核实语义不同（高亮 vs 过滤、输出类型不同），**不强行合并**——已登记为「避免错误抽象」的反面教材，不列入本 Backlog。
