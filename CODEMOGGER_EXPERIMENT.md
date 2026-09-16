# codemogger 实验报告

> 环境：Node v22.20.0，npm 全局安装 `codemogger@0.2.0`（发布版 dist，非源码 0.1.5）
> 日期：2026-07-11
> 目的：按文档全局安装并测试 codemogger 在工作区项目的索引/检索效果，并尝试扩展 Vue 支持

## 1. 安装与基本使用

```bash
npm install -g codemogger
codemogger index <dir>            # db 落到 <dir>/.codemogger/index.db
codemogger search "<query>" --mode semantic|keyword|hybrid --db <path>
```

- 嵌入模型 `all-MiniLM-L6-v2`（384 维，q8 量化，本地 CPU，首跑下载 ~22MB）。
- 支持 14 种语言：Rust, C, C++, Go, Python, Zig, Java, Scala, JS, TS, TSX, PHP, Ruby, C#。
- **扩展后额外支持**（见 §4 / §5）：`.vue`（复用 TS wasm 抽 `<script>`）、`.kt`/`.kts`（Kotlin）。
- **原生不支持**：任意非配置扩展名文件直接跳过（如 `.rs` 之外、`.swift`、`.dart` 等）。

## 2. 关键结论（已用干净实验验证）

### 2.1 上一轮"检索塌缩"是假象，已推翻
- 上一轮报告"所有查询塌缩到 `core.ts:212 tryRefreshToken`""TS name 退化成 Function"均为**不可靠结果**。
- 根因：上一轮搜索指向的 `/workspace/app/.codemogger` 当时并不存在（索引产物已丢失），且引用的 `tryRefreshToken` 函数在 `/workspace/app` 中**根本不存在**（grep 零命中）。
- 结论：codemogger 在 TS 大库上**索引与检索均正常**，无系统性塌缩 bug。

### 2.2 TS 名称提取完全正常
对 `/workspace/app`（真实 uni-app 项目）重新索引：
- 总 chunks **2223**，全部嵌入（2223/2223），TS 1986。
- **named 2172 / anonymous 仅 51（2.3% 空名）**。
- 类型分布：function 982、variable 580、interface 342、method 153、type 122、class 18…
- 说明箭头函数 `export const x = () => {}`、class、interface、enum 等名称提取均正确。

### 2.3 `--db` 选项实际生效
`resolveDbPath()` 先检查 `program.opts().db`，再回退 `cwd/.codemogger`。因此可用
`codemogger --db /abs/path/index.db search ...` 精确指定库，或进入项目目录后用默认库。

### 2.4 semantic 搜索（强项）
自然语言/意图查询表现好，结果多样且语义相关，优于 grep（无需知道标识符）：
- "refresh auth token when expired" → persistBackendIdentity / resetServerUrl / useLiveRefresh
- "upload image with concurrency dedupe" → MAX_CONCURRENT / uploadFile / concurrencyModule

### 2.5 keyword 搜索的真实局限（非 bug，是设计）
- **只索引 `name` + `signature`（首行），不索引代码正文**。
  - grep 在 28 个文件正文找到 "refresh"，FTS 仅命中 1 个（函数名/首行含 refresh 的 `useLiveRefresh`）。
  - `logout` 在 FTS 中 0 命中（无函数名含 logout）。
- **FTS 不分拆 camelCase**：查 "use" 返回 0（无名为 `use` 的块），必须输入完整标识符（如 `persistBackendIdentity` 精准命中）。
- 结论：keyword 模式 ≈ "按完整标识符找定义"，**不是全文代码搜索**，正文覆盖率远不如 grep。

### 2.6 hybrid 正常
"user login authentication" → WebDavAuth / basicAuthHeader / sessionPasswords（RRF 融合排序合理）。

## 3. 与 grep 的适用边界

| 场景 | codemogger | grep |
|---|---|---|
| 按意图发现"做什么的代码" | ✅ semantic 强 | ❌ 需猜标识符 |
| 按已知函数/类名精确定位定义 | ✅ keyword（需完整标识符） | ✅ 但会返回所有出现处 |
| 在代码正文中搜某个词/字符串 | ❌ 不索引正文 | ✅ 全覆盖 |
| 驼峰前缀/子串搜索 | ❌ 不分拆 | ✅ `-i` 子串即可 |
| Vue/Kotlin 项目 | ✅ 已扩展支持（见 §4/§5） | ✅ |

## 4. 尝试扩展：Vue 支持（已实现并通过验证）

### 4.1 方案
`.vue` 是 SFC（HTML 外壳 + `<script>` + `<style>`）。codemogger 无 Vue 语法包，但 `.vue`
的 `<script>` 本质是 TS/JS。因此：
1. 新增 `VUE` 语言配置：扩展名 `.vue`，**复用 `tree-sitter-typescript` 的 wasm**（零新依赖），
   `topLevelNodes`/`splitNodes` 沿用 TS，`isVue: true` 作为标记。
2. `chunkFile` 入口检测 `config.isVue`，先调用 `extractVueScript(content)`：用正则抽取所有
   `<script>...</script>` 块，按**原始行号偏移**回填到空白行矩阵中（非 script 部分置空），
   再交给 TS 解析器正常分块。这样 chunk 的 `startLine/endLine` 与原始 `.vue` 文件一致。

### 4.2 ⚠️ 关键陷阱：cli.mjs 是打包单体
`npm -g` 安装的 `codemogger` 可执行文件 = `/usr/local/lib/node_modules/codemogger/dist/cli.mjs`，
它是**把 `languages`/`treesitter`/`walker`/`store` 等全部内联进来的单文件 bundle**（注释
`// src/chunk/languages.ts` 等可证）。因此：
- 直接改 `dist/chunk/languages.js`、`dist/chunk/treesitter.js` **对 CLI 无效**（CLI 用的是
  cli.mjs 内联的未修补副本）。单测 `import './dist/chunk/treesitter.js'` 能生效，会让人误以为
  已修好，但 `codemogger index` 仍 0 文件。
- **必须改 `dist/cli.mjs` 里内联的对应代码**：`VUE` 配置块、`LANGUAGES` 数组、`var ... VUE ...`
  声明、`extractVueScript` 函数、`chunkFile` 的 `isVue` 分支。

> 注意：以上改动落在全局 `node_modules`，重装即失效。若要上游贡献，应改源码
> `src/chunk/languages.ts` + `src/chunk/treesitter.ts`，由构建重新生成 cli.mjs。

### 4.3 验证结果
- 自构 fixture（`UserCard.vue`，含 `<script setup lang="ts">` + interface/function/arrow/class）：
  `Indexed 1 file → 5 chunks`，行号 L11-15/L17-21/L23-25/L27-33/L35-35 与原文件一致；
  semantic "refresh auth token when expired" → tryRefreshToken(0.44)/AuthService/logoutUser；
  keyword "tryRefreshToken" 精准命中 L17-21。
- 真实文件 `/workspace/app/.../MpvAbout.vue`（49 行，`<script setup>`）：
  `Indexed 1 file → 4 chunks`（`mpvVersion`/`pluginVersion`/`licenseUrl`/`{ t }`，行号 33-37）；
  keyword "mpvVersion" 命中 L35-35；semantic "display software version info" →
  pluginVersion/mpvVersion。

### 4.4 已知局限
- 仅索引 `<script>` 块，**`<template>` 中的逻辑（如 `@click` 处理函数引用）不索引**；
  纯模板组件不会被检索到。
- `<style>` 不索引（符合预期）。
- 若 `<script>` 用 `lang="js"` 仍以 TS 语法解析（TS 语法兼容 JS，通常无碍）。

## 5. 尝试扩展：Kotlin 支持（已实现并通过验证）

### 5.1 方案与依赖
codemogger 无 Kotlin 语法包。经检索 npm，可用 `@tree-sitter-grammars/tree-sitter-kotlin@1.1.0`
（该包**自带 `tree-sitter-kotlin.wasm`**，正是 web-tree-sitter `Language.load` 所需；另一候选
`tree-sitter-kotlin@0.3.8` 不含 wasm，不可用）。

1. 把该包装进全局 codemogger 的 `node_modules`：`npm install --no-save @tree-sitter-grammars/tree-sitter-kotlin@1.1.0`
   （装在 `codemogger/node_modules/` 下，`_require.resolve("@tree-sitter-grammars/tree-sitter-kotlin/tree-sitter-kotlin.wasm")` 即可解析）。
2. 在 `cli.mjs` 内联代码加 `KOTLIN` 配置：扩展名 `.kt`/`.kts`，`topLevelNodes` =
   `["function_declaration","class_declaration","object_declaration","property_declaration","type_alias","secondary_constructor"]`，
   `splitNodes` = `["class_declaration","object_declaration"]`；并加入 `LANGUAGES` 数组与 `var` 声明列表。

### 5.2 Kotlin 节点类型实测（`@tree-sitter-grammars/tree-sitter-kotlin` 1.1.0）
用 wasm 实跑解析确认：
- `class`/`interface`/`data class`/`enum class`/`sealed class` **全部解析为 `class_declaration`**，且都带 `name` 字段 → `extractName` 的 `childForFieldName("name")` 通用分支已能取类名。
- `object Foo` / `companion object` → `object_declaration` / `companion_object`，带 `name`。
- 顶层 `fun` → `function_declaration`，带 `name`；`typealias X = Y` → `type_alias`，别名在 `type` 字段。
- **顶层 `val`/`const`（`property_declaration`）无 `name` 字段**，仅有 `variable_declaration` 子节点的 `identifier`。
  因此需在 `extractName` 增加分支：取 `property_declaration` 下首个 `identifier`/`simple_identifier` 子节点作名称。
- `nodeKind` 补充映射：`object_declaration`/`companion_object`→`object`、`property_declaration`→`variable`、
  `type_alias`→`type`、`secondary_constructor`→`constructor`（原逻辑对 Kotlin 类型会回退成裸类型名）。

### 5.3 ⚠️ 同样陷阱：cli.mjs 打包单体
与 Vue 一致，**必须改 `dist/cli.mjs` 内联代码**（KOTLIN 配置块、`LANGUAGES`、`var ... KOTLIN ...`、
`extractName` 的 `property_declaration` 分支、`nodeKind` 的 Kotlin 映射），改 `dist/chunk/*.js` 对 CLI 无效。
> 改动落在全局 `node_modules`，重装即失效；上游贡献应改源码 `src/chunk/languages.ts` +
> `src/chunk/treesitter.ts` 后重新构建。

### 5.4 验证结果
- 真实风格 fixture `Service.kt`（含 `const`/`val`/大 `class`/`interface`/`object`/`data class`/`typealias`/`fun`）：
  `Indexed 1 file → 8 chunks`；semantic "refresh auth token when expired" → `class AuthService`(0.45)/
  `class Token`/`object SystemClock`，名称与行号正确。
- 大 `class BigCalculator`（171 行）触发拆分：`Indexed → 169 chunks`，每个方法独立成块；
  keyword "gcd" → `function gcd`（L8-8）精准命中。证明 Kotlin 类内方法拆分与 keyword 检索均正常。

### 5.5 已知局限
- 仅当类/对象 **超过 150 行（`MAX_CHUNK_LINES`）** 才拆分出方法级 chunk；小类的方法名不进 FTS 索引
  （与 TS/Java 行为一致，属设计而非 bug）。
- `.kts` 脚本文件同样支持（同属 Kotlin wasm）。
- 仍不支持：`.vue` 的 `<template>` 正文、代码正文全文搜索、camelCase 子串拆分（见 §2.5）。

## 6. 全文搜索探索（FTS 索引代码正文）

### 6.1 现状（§2.5 根因定位）
通读 `src/db/schema.ts` 与 `src/db/store.ts` 确认 keyword 搜索的架构：
- FTS 表 `fts_<codebaseId>` 只建了 `chunk_id, name, signature` 三列，`USING fts (name, signature)`
  （tokenizer='default'，weights `name=5.0,signature=3.0`）。
- `populateFtsForFileSQL` 只 `SELECT id, name, signature FROM chunks` 灌入 FTS。
- `ftsSearch` 用 `fts_match(f.name, f.signature, ?1)` + `fts_score(...)` 查询。
- `snippet`（完整代码正文）只存在 `chunks` 表、仅用于结果回显，**完全不进 FTS 索引**。
  → 这就是 §2.5「keyword 不索引代码正文」「camelCase 不分拆」的根因。
- Turso/libSQL 的 `default` tokenizer 基于 unicode61，**不拆 camelCase / snake_case**
  （`tryRefreshToken` 是一个 token，`refresh` 搜不到）。

### 6.2 实验一：把 `snippet` 直接纳入 FTS
最小改动：FTS 表加 `snippet` 列、`USING fts (name, signature, snippet)`、`populateFtsForFileSQL`
与 `ftsSearch` 同步带 `snippet`。
- 验证：fixture 中仅在函数体注释/变量里出现的 `reconciliation` → keyword 搜索命中 `function doWork`。
- 局限：`invoiceReconciliation` 仍是单 token，`invoice` 仍搜不到（tokenizer 不拆驼峰）。

### 6.3 实验二：规范化 `body` 列（camelCase/snake 拆词）
为真正可用的「代码全文搜索」，新增 `body` 列（chunks 表与 FTS 表各一列），由
`normalizeBodyForFts(snippet)` 在分块时生成：剥离注释 → 按
`([a-z0-9])([A-Z])` / `([A-Z]+)([A-Z][a-z])` 拆驼峰、`[_-]+` 拆蛇形/连字符 → 转小写。
FTS 改为 `USING fts (name, signature, body)`（weights 加 `body=1.0`）。
- 改动点（均在 `dist/cli.mjs` 内联代码，同前陷阱）：`CREATE_CHUNKS_TABLE` 加 `body` 列、
  `createFtsTableSQL`/`createFtsIndexSQL`/`populateFtsForFileSQL`/`ftsSearch` 改用 `body`、
  `makeChunk` 产出 `body`、`batchUpsertAllFileChunks` 的 insert/upsert/run 透传 `body`、
  新增 `normalizeBodyForFts` 函数。
- 预期效果（待终端恢复后复测）：`validation` 命中 `orderValidationResult`、
  `order` 命中驼峰前缀 —— 即代码正文（含标识符内部词）可被全文检索。
- 代价：FTS 索引体积增大（每 chunk 多存一份规范化正文）；body 权重 1.0 低于 name/signature，
  避免常见词噪声淹没定义名。

### 6.4 结论
- codemogger 原生 keyword 搜索 = 「按完整标识符找定义」，非全文搜索（设计使然）。
- 通过把代码正文（规范化拆词后）纳入 FTS，可低成本升级为「代码全文搜索」，覆盖 §2.5 两大痛点。
- 仍不支持：子串/正则搜索、跨词短语邻近度（FTS 是 token 级 BM25）；这些需额外方案。

## 7. 文档待修正项（上游）
- `package.json` 声明 `@tursodatabase/database ^0.6.0`，但 README 仍写依赖钉在 `0.5.0-pre.14` 预发布版 —— 不一致。
- README/语言列表未显式列出 C#（实际 `languages.ts` 已支持）。
- 应明确文档化"`.vue` 的 `<template>` 不索引、代码正文全文搜索、camelCase 子串拆分"等限制。

## 8. 需求上下文：避免"盲人摸象"（影响分析）

### 8.1 问题
语言支持（多解析几种文件）+ 全文搜索（多命中几个 token）都只是**检索原语**。
而一个真实需求是一个带"波及面"的目标 —— 大象 = 实体全集 + 它们之间的依赖关系 +
边界规则 + 设计意图。只摸一块（一个函数定义）永远不够。

用两个真实需求案例验证（`docs/shared-components-boundary-spec.md`、
`docs/migration-task-system.md` 的 Phase 2「把 `useTaskStore` 提升进 shared、解耦 Pinia 单例」）：
- 要安全动手，agent 实际需要的上下文：✅ 定义（codemogger 能定位）❌ **全部 ~12 个消费者**
  （无引用查找）❌ **边界违例 / 钉子依赖**（无 import 关系图）❌ **"为什么"**（codemogger 读不了文档）。
- `shared-components-boundary-spec.md` 顶部自注「**已过时，与 migration 文档相反**」——
  文档自身就是失真/冲突的大象视图，agent 必须先调和文档 vs 代码，否则按错文档直接摧毁已建抽象层。

### 8.2 真实查询证据（在已索引的 `/workspace/app` 上跑）
| 查询 | codemogger 返回 | 真相 |
|---|---|---|
| keyword `taskStore` | 1 条：cypress 测试 mock | 文档叫 `taskStore`，代码标识符是 `useTaskStore` → 术语错配引到错误部位 |
| keyword `useTaskStore` | 1 条：`taskStore.ts:35-653`（618 行单 chunk） | 定义找到了，但无法展开、无调用者 |
| semantic 意图查询 | 扁平 12 条，混着 `freshModules`×4 测试 + cypress mock | 能捞出 DI 容器，但漏掉大部分消费者、无结构 |
| `grep -rn useTaskStore` | — | **11 个文件引用**（含 `main.ts`/`registerSharedTaskServices`/`useTasksList`…）|

关键：`useTaskStore` 的波及面是 11 个文件，codemogger 只显示 1 个 → 以为在改孤立 store，
实际会牵动 10 个文件。这是"盲人摸象"的代码版。

> 附带踩坑：给 `cli.mjs` 加 `body` 列改了 schema，但旧 `index.db` 是改前建的，结果 `index`/`search`
> **双双报错 `no such column: body`**。索引是会漂移的快照，无版本/迁移 —— 这也是"上下文新鲜度"缺口（见 §8.4 G）。

### 8.3 已实现的修复：`references` 命令（覆盖缺口 A/C）
在 `dist/cli.mjs` 内联代码新增**导入图**（index 时构建），并暴露 `references` 子命令：
- 新 `imports` 表 `(codebase_id, file_path, module, name)` 记录每条跨文件
  `import` / `export ... from` 边（符号 + 模块说明符）。
- `chunkFile` 额外跑 `extractImports(tree)`（tree-sitter 遍历 `import_statement` /
  `export_statement`），返回 `{ chunks, imports }`；index 流程在 chunk upsert 后调
  `store.batchUpsertImports(...)`。
- 新 CLI 命令（`patch` 包已含，见 `app/codemogger-patch/`）：

  ```bash
  codemogger references useTaskStore                  # 9 个导入该符号的文件 = 波及面
  codemogger references "@/stores/taskStore" --module   # 6 个按该模块别名导入的文件 = 边界视图
  codemogger references <taskStore.ts> --file        # 该文件自身的 11 个依赖（pinia/vue/@/types/task…）
  ```

- 实测：`references useTaskStore` → 9 importers；`references <taskStore.ts> --file` →
  11 个自身依赖。grep 全量 24 文件含该串（含定义/re-export/注释），`references` 精准隔离 9 个真实导入点。
- 局限：`--module` 按模块说明符**精确**匹配（别名差异如 `@/stores/taskStore` vs
  `@encv/shared-components/stores/taskStore` 是不同行）；符号模式与别名无关。

### 8.3.1 `references` 三坑与 shim 层缓解（2026-07-12 实战）

真实影响面分析时 `references` 连踩三坑，根因与缓解分层如下：

| 坑 | 根因 | 能否 patch cli.mjs 解决 | 实际缓解方式 |
|---|---|---|---|
| **1. 符号名精确匹配** | `references <sym>` 精确匹配 `imports.name`；猜错名即空（如真实导出 `useRunSummariesSingleton`/`getTaskTypeLabel`，臆想名 `useRunSummaries`/`taskTypeLabel` 恒空） | 可（改 SQL 为 LIKE/模糊），但会放大噪声 | **shim 启发式回退**：裸 `references <sym>` 无结果且 codebase 根下存在同名 `<sym>.{ts,tsx,vue}` 时，自动以 `--module "@/relpath"` 重查（模块级匹配与符号名无关，最全）。stderr 提示已回退 |
| **2. `--file` 需绝对路径** | `imports.file_path` 存绝对路径，传相对路径 `composables/x.ts` 静默零命中 | 可（shim 内转即可，无需动上游） | **shim 直接转**：检测到 `--file` 且 target 为相对路径时，改写为绝对路径再调真实 CLI |
| **3. 只解析静态 import** | walker 的 `extractImports` 仅遍历静态 `import`/`export ... from`，动态 `import()`/字符串说明符不可见 | **能**（需改 cli.mjs 的 walker 解析动态 import），但成本高、Vue 动态 import 少、收益低 | **不写 patch**：shim 不掩盖；调用方对动态引用用 grep 复核（如 `useTaskCancel` 即靠 grep 发现是未接线孤儿） |

> 坑 1/2 在 **shim 层**解决，不污染上游 `cli.mjs`（升级不冲突、patch 仍干净）；坑 3 是解析器限制，留给上游或按需单独 patch。

shim 安装/复测（在 codebase 根下跑，否则 shim 自动重索引会建错库）：
```bash
# 安装 shim 覆盖 codemogger bin（apply.sh 第 3 步等价）
install -m 0755 app/codemogger-patch/codemogger-shim "$(command -v codemogger)"

cd /workspace/app/encv-mobile/src
codemogger references useRunSummaries          # 坑1：原空 → 回退 @/composables/useRunSummaries 命中
codemogger references taskTypeLabel            # 坑1：原空 → 回退 @/lib/taskTypeLabel 命中（含 getTaskTypeLabel 引用）
codemogger references composables/useTasksList.ts --file   # 坑2：相对路径 → 自动转绝对，命中 19 条依赖
```

### 8.4 仍缺的能力（7 项缺口）
1. **A. 引用/反向查找（影响分析）** —— ✅ 已由 `references` 部分覆盖（符号/模块/文件三向）。
2. **B. 范围/模块查询** —— 按目录/包聚合"IN-SCOPE 全集"，而非一次一个实体。
3. **C. 依赖/边界图** —— ✅ `--module` 提供单模块边界视图；缺"动 X 会断什么"的传递闭包。
4. **D. 上下文展开** —— ✅ 已实现 `context` 命令（见 §8.6）：符号 → 展开到整文件（命中 chunk 标 `<<<`）+ `--expand` 打印每个 chunk 正文；文件路径（精确或后缀 LIKE）→ 该文件 chunk 大纲。
5. **E. 跨 chunk 聚合 / "大象形状"** —— 按模块/文件分组、画调用图，而非扁平相关度列表。
6. **F. 文档↔代码对齐** —— 把需求文档的 IN-SCOPE 列表/钉子/阶段链接到代码实体；检测文档漂移。
7. **G. 索引新鲜度/版本** —— schema 漂移问题；agent 应知道索引是否新鲜、schema 是否兼容。

### 8.5 今天 agent 如何用现有工具"不摸象"（实用流程）
1. **先调和文档**：读 `migration-task-system.md` §2/§7/§9，注意 `shared-components-boundary-spec.md`
   已过时；抽出 IN-SCOPE 实体清单 + 钉子。
2. **定位定义**：对每个实体 semantic+keyword（警惕术语错配：`taskStore`≠`useTaskStore`）。
3. **影响面用 `references`**（替代 grep）：`codemogger references useTaskStore` 直接给波及面。
4. **边界违例**：在 shared 内 `grep -rn "@/config\|@/constants\|@/router" packages/shared-components/src`。
5. **编辑前展开**：把定位到的 chunk 拉成完整文件再看。

### 8.6 已实现的修复：`context` 命令（覆盖缺口 D）
定位到一个 chunk 后，仍看不到它的"邻居"与"整文件形状"——尤其 618 行的 `taskStore.ts`
被压成 1 个 chunk 时，无法展开。新增 `context` 子命令（index 时数据已就绪，无需重索引）：

```bash
codemogger context useTaskStore          # 符号 → 其所在整文件的 chunk 大纲，命中处标 <<<
codemogger context useTaskStore --expand # 同上，另打印每个 chunk 的正文 snippet
codemogger context stores/taskStore.ts  # 文件路径（精确或后缀 LIKE）→ 该文件 chunk 大纲
```

- store 新增 `listChunksByFile`（`file_path = ? OR file_path LIKE ?`，支持后缀模糊）、
  `findChunksByName`；CodeIndex 包一层 `getDefaultCodebaseId` 解析当前库。
- 符号模式：先 `findChunksByName` 拿到命中 chunk，再对每个命中文件 `listChunksByFile`
  展开整文件，命中 chunk 打 `<<<` 标记 —— 即"从符号展开到整文件上下文"。
- 实测：`context useTaskStore` → `taskStore.ts` 的 8 个 chunk 大纲（types/variables/大 store），
  `useTaskStore` 标在 `L35-653 <<<`；`context useTasksList --expand` → 11 个 chunk 各自正文。
- 局限：符号模式按 `name` 精确匹配（不含正文/签名模糊）；展开整文件对超大文件会很长，
  可后续加 `--max-lines` / 只展开命中 chunk 附近 N 行窗口。

结论：**codemogger = 发现（discovery），references/context/文档 = 理解（comprehension）**。
单靠哪个都会盲人摸象；组合 + 文档的范围清单才拼出整头大象。
