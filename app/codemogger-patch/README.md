# codemogger patch package (v0.1.5)

Self-contained patch for [`codemogger@0.1.5`](https://github.com/glommer/codemogger)
that bundles all experimental changes made to the global CLI install.

## Contents

```
codemogger-patch/
├── apply.sh                         # idempotent installer
├── README.md                        # this file
├── patches/
│   └── codemogger+0.1.5.patch       # git/UNIX diff of dist/cli.mjs
└── vendor/
    └── tree-sitter-kotlin/          # @tree-sitter-grammars/tree-sitter-kotlin@1.1.0
                                      # (only the .wasm grammar, prebuilds/src stripped)
```

## What the patch changes

All changes live in `dist/cli.mjs` (the bundled CLI). They are captured as a
unified diff against the pristine `codemogger@0.1.5` tarball.

> **Version note.** The published *dist* of codemogger may report itself as
> `0.2.0` while the source / npm registry latest is `0.1.5` — a packaging
> quirk on the package's side, not a doc error. The patch is keyed to the
> `0.1.5` *source* `cli.mjs`, which is exactly what `npm install codemogger`
> serves today, so it applies cleanly.

### 0. Vue (`.vue`) language support
- Adds a `VUE` language config (`.vue`) that **reuses the already-bundled
  `tree-sitter-typescript` wasm** (zero new dependencies).
- `chunkFile` detects `config.isVue` and first runs `extractVueScript(content)`:
  it regex-extracts every `<script>…</script>` block and back-fills it into a
  blank-line matrix at the **original line offsets**, so each chunk's
  `startLine`/`endLine` match the real `.vue` file. The result is then parsed
  by the TS grammar exactly like a `.ts` file.
- Verified in the experiment: a real `MpvAbout.vue` (`<script setup>`) indexed
  `4 chunks` with correct line numbers; `keyword "mpvVersion"` and
  `semantic "display software version info"` both hit.

### 1. Kotlin language support
- Adds a `KOTLIN` language config (`.kt` / `.kts`) using
  `@tree-sitter-grammars/tree-sitter-kotlin`'s `tree-sitter-kotlin.wasm`.
- Registers `KOTLIN` in the `LANGUAGES` array and `EXT_MAP`.
- Maps Kotlin node types (`object_declaration`, `property_declaration`,
  `type_alias`, `secondary_constructor`) to chunk kinds.
- Adds an `extractName` branch for `property_declaration` (top-level
  `val`/`const` have no `name` field — the identifier is pulled from the
  nested `variable_declaration` / `simple_identifier`).

### 2. Full-text search over code bodies
- Adds a `body` column to the `chunks` table.
- Extends the FTS5 table `fts_<id>` to index `name, signature, body` with
  weights `name=5.0, signature=3.0, body=1.0`.
- New `normalizeBodyForFts()` splits camelCase / PascalCase / snake_case /
  kebab-case identifiers and strips comments, so keyword search matches
  tokens *inside* identifiers (e.g. searching `validation` hits
  `orderValidationResult`).
- `makeChunk` now emits `body: normalizeBodyForFts(snippet)` and the
  upsert/FTS-populate SQL threads it through.

### 3. Impact analysis: the `references` command
Addresses the "blind-men" gap — semantic/keyword search finds *what* a symbol
is, but not *who depends on it*. The patch builds an import graph at index time
and exposes it through a new `references` command.

- New `imports` table `(codebase_id, file_path, module, name)` capturing every
  cross-file `import` / `export … from` edge (symbol + module specifier).
- `chunkFile` now also runs `extractImports(tree)` (tree-sitter walk over
  `import_statement` / `export_statement`) and returns `{ chunks, imports }`.
- Index flow calls `store.batchUpsertImports(...)` after chunk upsert.
- New CLI command:

  ```bash
  codemogger references <target>            # files that import symbol <target>
  codemogger references <target> --module   # treat <target> as a module specifier
  codemogger references <file>   --file     # list the imports OF <file>
  codemogger references <target> --format json
  ```

  - **symbol mode** → "blast radius" of a symbol (e.g. `useTaskStore` → 9
    importers in this repo).
  - **`--module`** → boundary view: who imports a given module alias
    (`@/stores/taskStore` → 6 files).
  - **`--file`** → reverse coupling: what a file itself depends on
    (`taskStore.ts` → pinia, vue, `@/types/task`, `./taskServices`, …).

> Limitation: `--module` matches the import specifier **exactly** (alias
> differences like `@/stores/taskStore` vs `@encv/shared-components/stores/taskStore`
> are distinct rows). Symbol mode is alias-agnostic. **This gap is closed by the
> `impact` subcommand added in `codemogger-shim` (see §5).**

### 4. Context comprehension command (second-stage patch)
Addresses gap **D** — once you've located a chunk, you still can't see its
neighbours or the shape of the whole file (e.g. a 618-line `taskStore.ts`
collapsed into one chunk). This is delivered as a **separate, second-stage
patch** (`patches/codemogger+0.1.5+context.patch`) that must be applied *after*
the main patch above.

- Store gains `listChunksByFile(codebaseId, filePath)` — `file_path = ? OR
  file_path LIKE ?`, so a suffix/partial path expands to the whole file's chunk
  outline — and `findChunksByName(codebaseId, name)`.
- New CLI command:

  ```bash
  codemogger context useTaskStore          # symbol -> outline of its whole file, hit chunk marked <<<
  codemogger context useTaskStore --expand # same, but also prints each chunk's snippet body
  codemogger context stores/taskStore.ts  # file path (exact or suffix LIKE) -> that file's chunk outline
  ```

- Symbol mode: `findChunksByName` → for each hit file `listChunksByFile` →
  outline with the hit chunk tagged `<<<`. File mode: straight
  `listChunksByFile`. No re-index needed (data is already in `chunks`).

> Status: the `context` command is delivered as this second-stage patch
> (`codemogger+0.1.5+context.patch`), regenerated to match the main-patched
> `cli.mjs` exactly (the earlier draft had stale hunk line numbers that failed
> to apply past hunk 1). It inserts `Store.listChunksByFile` /
> `findChunksByName`, the two `CodeIndex` wrappers, and the `context` CLI
> command. Apply it *after* the main patch.

### 5. Refactoring helpers (added in `codemogger-shim`)

The shim wraps the real CLI and (per §0–§4) auto-reindexes before read
queries and mitigates three UX pitfalls. On top of that it adds two
subcommands that turn codemogger's import graph into a **refactoring
assistant** for the `encv-mobile` → `@encv/shared-components` lift:

```bash
# impact: alias-agnostic blast radius of a module/symbol.
# Closes the §3 limitation — it queries BOTH "@/x" and
# "@encv/shared-components/x" and merges the importer lists, so a
# lift/decouple never silently misses one alias form.
codemogger impact @/api/encv
codemogger impact @encv/shared-components/api/encv
codemogger impact useTaskStore          # bare symbol -> alias-agnostic

# leaks: list every shared->app reverse `@/` import under a dir
# (default: indexed root). Directly verifies the
# "shared layer must NOT depend on the app layer" invariant
# after a decoupling pass.
codemogger leaks packages/shared-components/src
```

- `impact` is what you run **before** moving/decoupling a module: it tells
  you the full set of callers across both alias spellings, so you can size
  the change and not break a hidden importer.
- `leaks` is what you run **after** a decoupling pass: an empty result
  means the shared package is clean (no `@/` pointing at the app layer).

## MCP server (terminal-free queries)

`codemogger` can also be exposed to CodeBuddy as an **MCP server**, so the
agent queries the code-graph (references / context / search / impact / leaks) as
MCP *tools* — no `execute_command` round-trip, no terminal flakiness.

### Files
- `mcp-server.mjs` — Node stdio JSON-RPC server wrapping `./codemogger-shim`.
  Exposes 7 tools:

  | tool | purpose |
  |------|---------|
  | `codemogger_references` | who imports a symbol/module/file (mode: symbol\|module\|file) |
  | `codemogger_context` | chunk outline of a symbol's file / a file path (`expand` for bodies) |
  | `codemogger_search` | **multi-root hybrid** search: codemogger FTS over each root in `.codemogger.json` (labeled + ordered by weight) ∪ **per-root** grep complement over the configurable general-knowledge file set (docs + config + source codemogger's FTS can't parse) — backend/config roots get first-class labeled blocks; concept-aware |
  | `codemogger_grep` | direct grep over the configurable general-knowledge file set in `.codemogger.json` `grep.include` (default: go/md/json/yaml/kt/ts/vue/css/scss/less/Dockerfile/toml) — surfaces backend, config, doc & style facts the structured index misses. Optional `dir` scopes to a single folder (sets `CODEMOGGER_GREP_ROOT`) — the MCP-native replacement for terminal `grep -r <path>`. Supports `|` as OR for multiple literals (e.g. `foo|bar`) |
  | `codemogger_impact` | alias-agnostic blast radius (queries both `@/` and `@encv/...`) |
  | `codemogger_leaks` | reverse shared→app `@/` imports under a dir |
  | `codemogger_index` | **multi-root** (re)index: indexes every root in `.codemogger.json` into its own `<root>/.codemogger/index.db` |
  | `codemogger_list` | list indexed codebases |

### Register (CodeBuddy reads `~/.codebuddy/mcp.json`)
```json
{
  "mcpServers": {
    "codemogger": {
      "command": "node",
      "args": ["/abs/path/to/codemogger-patch/mcp-server.mjs"],
      "env": { "CODEMOGGER_ROOT": "/abs/path/to/encv-mobile" }
    }
  }
}
```

Idempotent one-liner (run from inside `codemogger-patch/`):
```bash
mkdir -p ~/.codebuddy && cat > ~/.codebuddy/mcp.json <<'JSON'
{
  "mcpServers": {
    "codemogger": {
      "command": "node",
      "args": ["$(pwd)/mcp-server.mjs"],
      "env": { "CODEMOGGER_ROOT": "$(pwd)/../encv-mobile" }
    }
  }
}
JSON
```

### Env (all optional)
- `CODEMOGGER_ROOT` — codebase root to index (default `/workspace/app/encv-mobile`)
- `CODEMOGGER_SHIM` — path to the shim (default: `./codemogger-shim` beside the server)
- `CODEMOGGER_AUTOINDEX` — `"1"` lets the shim auto-reindex before every read
  query (default **off**, to avoid MCP-request timeouts; call `codemogger_index`
  explicitly when fresh data is needed)

### Why terminal-free
The shim's `impact`/`leaks` turn codemogger into a refactoring assistant
(e.g. sizing the blast radius of the `@/api/encv` → `@encv/shared-components/api/encv`
lift, or verifying the shared layer has no reverse `@/` deps after a decouple).
Wrapping them as MCP tools lets the agent run these checks without spawning a
shell — robust against the flaky `execute_command` channel.

## 6. Multi-root search + concept glossary (workspace `.codemogger.json`)

A single codemogger root is unsuitable for this workspace: it mixes the primary
frontend (`encv-mobile`), a shared frontend package (`packages/shared-components`),
a Go backend (`internal`), and **managed sibling projects** (`combolite` Kotlin,
`openlist` Go+TS, `preview-gateway` Go) that all need to be searchable. codemogger
itself only parses TS/Vue/Kotlin, so the Go roots can't contribute to its FTS — but
they must still be retrievable.

### `.codemogger.json` (workspace root)
Declares the roots (with weights) and a concept glossary:

```json
{
  "roots": [
    { "path": "app/encv-mobile",                "weight": 1.0 },
    { "path": "app/packages/shared-components", "weight": 1.0 },
    { "path": "internal",                       "weight": 0.9 },
    { "path": "app/openlist",                   "weight": 0.8 },
    { "path": "app/combolite",                  "weight": 0.7 },
    { "path": "app/preview-gateway",            "weight": 0.6 }
  ],
  "concepts": {
    "SPA":        ["index.html", "Vite", "WebView", "loadUrl", "assetBasePath", "file:///android_asset", "encv-go"],
    "servingDir": ["servingDir", "Server.Dir", "cfg.Server.Dir"],
    "docker":     ["Dockerfile", "docker-compose", "image:", "container:"],
    "config":     ["ENCV_", "CODEMOGGER_", "vite.config", "biome.jsonc", "pnpm-workspace"]
  },
    "grep": {
    "include": ["*.go","*.md","*.json","*.jsonc","*.yaml","*.yml","*.kt","*.kts",
                "*.ts","*.tsx","*.vue","*.css","*.scss","*.less",
                "Dockerfile","*.toml","*.sh","*.gradle","*.gradle.kts"],
    "exclude": ["node_modules",".git",".codemogger",".agents",".codebuddy",".trae",
                "dist","build",".next",".output","coverage"]
  }
}
```

- `weight` orders and labels the per-root FTS blocks (higher = more primary).
- `concepts` maps an architectural term to the **precise tokens** that locate it.
  This cuts substring noise — a bare `SPA` query would otherwise match
  `xcworkspace`; expanding to `Vite`/`WebView`/`loadUrl`/`assetBasePath` pinpoints
  the SPA-serving architecture instead.
- `grep` declares the **general-knowledge retrieval set** — docs + config files
  that codemogger's FTS can't index but the agent must be able to search. Two keys:
  - `include` — glob patterns (e.g. `*.go`, `*.md`, `*.json`, `*.yaml`, `*.yml`,
    `Dockerfile`, `*.toml`, `*.sh`, `*.kt`, `*.ts`, `*.vue`, `*.css`, `*.scss`,
    `*.less` …). Drives both `search`'s grep complement and the standalone `grep` tool.
  - `exclude` — directory names to skip (build output, tooling dirs, VCS, …).
  - Omit the whole `grep` block to fall back to built-in defaults (same set above).

### How `search` / `index` behave
- `codemogger index` → loops every root, building `<root>/.codemogger/index.db`
  each (codemogger has no native multi-root mode). Missing dirs are skipped.
- `codemogger search <q>`:
  1. If `<q>` matches a `concepts` key → grep each precise term **per root**
     (labeled `[root · w=…]` blocks). The bare-query grep is skipped to avoid
     substring noise. Because it's per-root, backend/config roots also contribute
     concept hits with proper attribution.
  2. Otherwise → codemogger FTS over each root's DB (labeled `[root · w=…]`,
     ordered by weight desc), **then** a **per-root** grep complement over the
     `grep.include` general-knowledge file set. The complement is per-root (not a
     flat wasted grep): each root emits its own labeled, weight-ordered block, so
     `internal`/`openlist`/`combolite` get FIRST-CLASS visibility for Go/config/doc
     hits — exactly the architectural facts codemogger's FTS can't reach.
- `codemogger grep <q>` → direct grep over the `grep.include` set (the complement
  step, standalone; flat over the workspace unless `CODEMOGGER_GREP_ROOT` scopes it).
- `references` / `context` / `impact` / `leaks` remain **single-root** (they target
  the frontend root via `--db`); multi-root FTS + per-root grep discovery is what
  `search` adds.

### Env
- `CODEMOGGER_CONFIG` — path to `.codemogger.json` (default `/workspace/.codemogger.json`).
- `CODEMOGGER_GREP_ROOT` — override the full-repo grep tree (default: the workspace
  dir containing `.codemogger.json`).

> Without a `.codemogger.json` present, the shim falls back to the original
> single-root behavior (`CODEMOGGER_ROOT` / `--db`), so it's backward compatible.

## Applying

```bash
# default: global install at /usr/local/lib/node_modules/codemogger
cd codemogger-patch
./apply.sh                 # apply (idempotent)
./apply.sh --check         # dry-run only

# or point at a project-local install
CODEMOGGER_DIR=./node_modules/codemogger ./apply.sh
```

`apply.sh` will:
1. Copy the vendored `tree-sitter-kotlin` grammar into codemogger's
   `node_modules` (skipped if already present).
2. `patch -p1` the `dist/cli.mjs` diff (skipped if already applied).

> **Optional second stage — `context` command.** After `apply.sh` succeeds,
> apply the supplementary patch from the `node_modules` parent of your
> codemogger install (same working dir `apply.sh` uses):
>
> ```bash
> cd "$(dirname "$(dirname "$CODEMOGGER_DIR")")"   # node_modules parent
> patch -p1 --forward < /path/to/codemogger-patch/patches/codemogger+0.1.5+context.patch
> ```
>
> It must run *after* the main patch (it edits the already-patched
> `cli.mjs`). Re-test `codemogger context <symbol>` before relying on it.

> Note: changes are to the **bundled** `dist/cli.mjs`. Reinstalling or
> upgrading codemogger overwrites it — re-run `apply.sh` afterwards. For an
> upstream fix, port the logic to `src/` (languages, treesitter, db/schema,
> db/store) and rebuild.

## Reverting

```bash
cd /usr/local/lib            # node_modules parent
patch -p1 -R < /path/to/codemogger-patch/patches/codemogger+0.1.5.patch
# then remove the vendored grammar if desired:
rm -rf /usr/local/lib/node_modules/codemogger/node_modules/@tree-sitter-grammars/tree-sitter-kotlin
```

## Verification (as tested)

- `Service.kt` → 8 chunks indexed; large class (>150 lines) → method-level
  split (169 chunks); keyword `gcd` matched.
- Keyword `validation` matched `orderValidationResult` after the `body`
  normalization change.
- `codemogger references useTaskStore` → 9 importers; `references <taskStore.ts>
  --file` → 11 of its own dependencies. Confirms the import graph is populated
  after a full re-index.
