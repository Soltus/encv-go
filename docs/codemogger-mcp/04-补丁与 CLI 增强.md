# 04 · 补丁与 CLI 增强

codemogger 上游不支持 Vue/Kotlin、keyword 不索引代码正文、没有引用图。
镜像版通过**给已安装的 CLI 打补丁 + shim 包裹**补齐这些能力，不 fork 上游。

## 0. 必须先知道：`cli.mjs` 是打包单体

`npm -g` 安装的 `codemogger` = `/usr/local/lib/node_modules/codemogger/dist/cli.mjs`，
它把 `languages`/`treesitter`/`walker`/`store` 等全部内联进了单文件 bundle
（注释里能看到 `// src/chunk/languages.ts` 等标记）。

⇒ 改 `dist/chunk/*.js` **对 CLI 完全无效**（CLI 用的是 cli.mjs 内联的未修补副本）。
单测里 `import "./dist/chunk/treesitter.js"` 能生效，会让人误以为改好了，
但 `codemogger index` 仍然 0 文件。所有补丁必须落在 `dist/cli.mjs`。

## 1. 补丁清单（`/scripts/codemogger-mcp/patches/`）

### `codemogger+0.1.5.patch`（主补丁）

| 改动 | 说明 |
|---|---|
| **Vue 支持** | 新增 `VUE` 语言配置（`.vue`），**复用已内置的 `tree-sitter-typescript` wasm**（零新依赖）。`chunkFile` 检测 `config.isVue` → 先用 `extractVueScript(content)` 正则抽所有 `<script>` 块，**按原始行号偏移回填**到空白行矩阵（非 script 部分置空）→ 交 TS 解析器正常分块。因此 chunk 的 startLine/endLine 与真实 `.vue` 文件一致 |
| **Kotlin 支持** | 依赖 `@tree-sitter-grammars/tree-sitter-kotlin@1.1.0`（自带 wasm；另一个候选 `tree-sitter-kotlin@0.3.8` 不含 wasm，不可用）。节点映射：`class/interface/data class/enum class/sealed class` 都解析为 `class_declaration` 且带 `name`；`object`/`companion object` → `object_declaration`/`companion_object`；顶层 `val`/`const` 是 `property_declaration` **没有 name 字段**，需在 `extractName` 增加分支取首个 `identifier`/`simple_identifier` |
| **全文检索** | `chunks` 表加 `body` 列；FTS5 表改为 `fts(name, signature, body)`，权重 `name=5.0 / signature=3.0 / body=1.0`；新增 `normalizeBodyForFts(snippet)`：剥注释 → 按 `([a-z0-9])([A-Z])`、`([A-Z]+)([A-Z][a-z])` 拆驼峰、`[_-]+` 拆蛇形 → 转小写；`makeChunk` 产出 `body`，insert/upsert/FTS populate 透传 |
| **引用图** | 新增 `imports(codebase_id, file_path, module, name)` 表；`chunkFile` 额外跑 `extractImports(tree)`（遍历 `import_statement`/`export_statement`）→ `{chunks, imports}`；索引流程在 chunk upsert 后 `batchUpsertImports` |
| `references` 命令 | symbol / `--module` / `--file` 三向查询 |

### `codemogger+0.1.5+context.patch`（第二阶段，**必须在主补丁之后**）

给 `Store` 增加 `listChunksByFile`（`file_path = ? OR LIKE ?` → 支持后缀模糊）与
`findChunksByName`，并在 CodeIndex 包一层，新增 `context` 命令（符号 → 整文件大纲，命中标 `<<<`）。

> 这条曾经踩过：早先把 context 逻辑写进主补丁 draft，hunk 行号对不上，第 1 个 hunk 之后就 apply 失败。
> 现在拆成独立补丁、并在 `apply.sh` 里做 dry-run 判断幂等。重复应用会留下 `cli.mjs.rej`
> （无害，可清理；2026-09-18 在本环境观察到一份，已随恢复 pristine 处理）。

### `codemogger+0.1.5+stale-imports.patch`

索引时清理失效的 import 边（重建 `-wal` 场景下的陈旧关系）。

## 2. `apply.sh`（幂等安装器）

```bash
cd /scripts/codemogger-mcp      # 或设置 CODEMOGGER_DIR 指向项目内安装
./apply.sh                      # 应用
CODEMOGGER_DIR=./node_modules/codemogger ./apply.sh
```

流程：
1. `force-wasm-grammars.mjs` 把 TS 语法包改成 WASM-only（存在才执行）。
2. vendor Kotlin wasm 到 `codemogger/node_modules/@tree-sitter-grammars/tree-sitter-kotlin/`。
3. 主补丁 `patch -p1`（已应用的 hunk 自动跳过）。
4. `context` 补丁：`--dry-run` 判断是否可应用，不能则打印 `[skip] already applied`。
5. 用 shim 覆盖 `codemogger` bin（`install -m 0755`）。

> **易混**：仓库里的 `patches/*.patch` 是耐久的源真相；被打进 **全局安装目录的 cli.mjs** 才是易失的
> —— `npm install -g codemogger` 解压 tarball 会原样覆盖 cli.mjs、重建指向原始 cli.mjs 的 bin。
> 所以「重装即失效」指**已应用的修改**，重跑一次 `apply.sh` 即恢复。

## 3. shim 层（`codemogger-shim`）

不污染上游、升级不冲突的一层，做四件事：

1. **自动建库**：库文件不存在时先 `index` 再执行读命令（只建一次：每次重建会与紧随其后的查询
   抢同一个 SQLite 库，报多进程 WAL 错误，也更慢）。
2. **多 root / 概念感知**：读 `CODEMOGGER_CONFIG`（默认 `/workspace/.codemogger.json`）的 roots、
   concepts、grep include/exclude；`index` 循环各 root，`search` 逐 root 出 FTS 块 + grep 补集。
3. **`references` 两坑缓解**（见 03 §4）。
4. **`impact` / `leaks` / `grep` / `css-source`** 子命令（重构三件套 + 泛知识检索 + CSS 溯源）。

## 4. 向上游贡献（正道）

以上所有逻辑若要进上游，改的是**源码**（重编后才生效）：

| 目标 | 源码位置 |
|---|---|
| Vue / Kotlin 语言支持 | `src/chunk/languages.ts`、`src/chunk/treesitter.ts` |
| 全文检索（body 列） | `src/db/schema.ts`、`src/db/store.ts` |
| 引用图 / context | 新增 `src/db/*`、`src/commands/*` |

## 5. 上层质检项（给上游的文档待修正）

- `package.json` 声明 `@tursodatabase/database ^0.6.0`，README 仍写依赖钉在 `0.5.0-pre.14` 预发布版（不一致）。
- README/语言列表未列 C#（`languages.ts` 实际已支持）。
- 文档应明确「`<template>` 不索引」「keyword 默认不索引正文」「camelCase 不拆分」等限制
  （镜像版的补丁只解决后两条，且**额外信息补给了 keyword 模式**）。
