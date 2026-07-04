# 计划：创建 GitHub Linguist / Gitattributes 规则文件以修正语言统计

> 目标：让 `github.com` 仓库页的「Languages」栏准确反映项目真实代码组成，剔除第三方 skill、dist 产物、纯文档目录造成的 JS / Markdown 虚高占比。

---

## 一、Phase 1 探索结果（Current State Analysis）

### 1.1 项目真实语言组成（基于本地统计，排除 `node_modules`、`.git`）

| 语言 | 估算文件数 | 主要分布 | 备注 |
|------|-----------|----------|------|
| Go | ~345 | `agent/`、`cmd/`、`internal/`、`pkg/`、`test_decrypt/` | 项目核心后端 |
| TypeScript / Vue / JS | ~249 | `app/encv-mobile/src/`、`app/preview-gateway/`、`app/encv-mobile/plugin-openlist/web/` | 移动端前端 |
| Kotlin | ~52 | `app/encv-mobile/android/app/`、`plugin-mpv-player/`、`plugin-openlist/`、`combolite-host/` | Android 原生层 |
| Shell | ~18 | `scripts/`、`.trae/` | 构建/工具脚本 |
| Markdown | ~839 | 全仓库广泛分布 | 含大量内部文档与第三方 skill 描述 |

### 1.2 已识别的「语言统计失真」来源

| 失真源 | 路径 | 主要失真语言 | 体量估算 | 性质 |
|--------|------|-------------|----------|------|
| Capacitor / Ionic / Lynx 第三方 skill | `.agents/skills/**` | Markdown (283 个 `.md`)+ 零散 JS | ~1.9 MB | 第三方技能定义，本项目实际未使用其代码 |
| 移动端第三方 skill | `app/encv-mobile/.agents/skills/**` | JS/MJS/CJS bundle | **~5 MB**（lynx-trace-record/analysis 各含 1.2MB `shared.bundle.cjs`，lynx-devtool 含 739KB `index.mjs`） | 第三方 Lynx 工具链脚本 |
| OpenList Web 前端 dist 产物 | `app/encv-mobile/plugin-openlist/src/main/assets/openlist/assets/**` | JS (Vite 打包) | **~1.3 MB**（`index-vjWJtKvZ.js` 单文件 1.2 MB） | 上游 OpenList 仓库构建产物，**非本项目代码** |
| Trae 内部 docs / specs / rules | `.trae/documents/**`、`.trae/specs/**`、`.trae/rules/**`、`.trae/skills/**` | Markdown (400+) | ~4.3 MB | AI agent 内部规划文档/规范/规则，**非交付代码** |
| 演示 agent 的 skill | `agent/cmd/agent-demo/scripts/skills/**` | Markdown | ~24 KB | demo 数据 |
| Capacitor Android 容器（已部分定制） | `app/encv-mobile/android/**` | Gradle/Kotlin 混合 | 大 | 既有 generated 也有定制 Kotlin，**不可整体排除**（见 §1.3） |

### 1.3 关键发现：android/ 目录**不能**整目录排除

`app/encv-mobile/android/` 同时包含：
- **生成产物**：`android/capacitor-cordova-android-plugins/`、`android/app/src/main/assets/capacitor.config.json`（见 `capacitor.settings.gradle` 第 1 行 `// DO NOT EDIT ... GENERATED`）
- **定制 Kotlin 源码**：`android/app/src/main/java/com/encvgo/app/`、`android/combolite-host/src/main/java/com/encvgo/combolite/`

若整目录 `paths-ignore` 会误伤定制 Kotlin 代码的语言识别。**方案**：
- 不动 `app/encv-mobile/android/` 的整体可见性
- 仅对其中的「纯生成」子目录用 `gitattributes` 单独标 `linguist-generated=true`（影响 diff 标注，不影响语言统计可见性）

### 1.4 现有规则文件状态

- `.github/linguist.yml` — **不存在**
- `.gitattributes` — **不存在**
- `.github/workflows/` — 已有 5 个 CI workflow（无需改动）

---

## 二、目标方案对比（用户已选：linguist.yml + .gitattributes 组合 + 激进范围）

| 方案 | 影响面 | 优点 | 缺点 |
|------|--------|------|------|
| 仅 `.github/linguist.yml` | github.com Languages 栏 | 改动最小、最符合 GitHub 官方推荐 | 不影响 git diff / blame 中的 generated 标注 |
| **linguist.yml + .gitattributes**（采用） | 语言统计 + git diff 视图 | 语言统计更准确 + IDE / GitHub 折叠 generated diff；CI 可见性更好 | 多一个文件；`.gitattributes` 通配规则需谨慎避免误伤 |

`.gitattributes` 在本计划中承担两个角色：
1. `linguist-vendored=true` — 同步给 GitHub 与 linguist.yml 效果一致（防御性双保险）
2. `linguist-generated=true` — 仅影响 diff 折叠 / blame 跳转，不影响语言统计
3. `binary` / `diff=` — 对大体积 bundle 关闭 diff（避免 PR review 卡顿）

---

## 三、Proposed Changes（变更清单）

### 3.1 新建 `.github/linguist.yml`

**作用**：github.com 仓库页 Languages 栏的「白名单/黑名单」配置。

**内容结构**：

```yaml
# .github/linguist.yml
# 控制 github.com 仓库页「Languages」统计的可见性。
# 仅列在 paths-ignore 中的路径会被剔除；不在此文件中的目录保持原样。

version: 1.0

# 关闭自动 vendored 推断（避免 Capacitor 误判）
vendored: false
auto: false

# 优先识别（防止默认推断把 Vue/TS 误归为 HTML）
typescript: true
vue: true
kotlin: true
go: true

paths-ignore:
  # === 第三方 skill 定义（纯 markdown + 偶发脚本片段） ===
  - .agents/skills/**
  - app/encv-mobile/.agents/skills/**
  - .trae/skills/**
  - agent/cmd/agent-demo/scripts/skills/**

  # === 上游构建产物 / dist 输出 ===
  - app/encv-mobile/plugin-openlist/src/main/assets/openlist/assets/**

  # === Trae AI agent 内部文档与规范（不是交付代码） ===
  - .trae/documents/**
  - .trae/specs/**
  - .trae/rules/**
  - .trae/scripts/**

  # === 资源/数据/锁文件（不计入语言） ===
  - app/encv-mobile/data/**
  - **/*.png
  - **/*.svg
  - **/*.ttf
  - **/*.woff2
  - **/*.db
  - **/*.jar
  - **/*.aar
  - **/*.lock
  - app/encv-mobile/pnpm-lock.yaml
  - pnpm-lock.yaml
  - app/encv-mobile/package-lock.json
  - package-lock.json
  - **/*.sccgt
  - *.tar.gz

# 不使用 — 但显式声明避免 GitHub 默认行为
generated:
  - .github/workflows/**
```

**为何把 `pnpm-lock.yaml` 等也列入**：
锁文件、字体、二进制 SQLite DB 不应进入语言统计；`.sccgt` 是 SCC 历史归档临时文件（项目根有两份 `*.sccgt`）。

### 3.2 新建 `.gitattributes`

**作用**：双层保险（同步 `paths-ignore`）+ diff 折叠 + 二进制识别。

**内容结构**：

```gitattributes
# .gitattributes
# 同步 linguist.yml 规则，并补充 diff / blame 行为配置。

# === 1. 同步语言统计排除（与 .github/linguist.yml 保持一致） ===
# GitHub 优先读取 linguist.yml；此处作为 IDE / 工具链 fallback
/agents/skills/**                                    linguist-vendored
app/encv-mobile/.agents/skills/**                    linguist-vendored
/.trae/skills/**                                     linguist-vendored
/agent/cmd/agent-demo/scripts/skills/**              linguist-vendored
app/encv-mobile/plugin-openlist/src/main/assets/openlist/assets/**  linguist-vendored
/.trae/documents/**                                  linguist-vendored
/.trae/specs/**                                      linguist-vendored
/.trae/rules/**                                      linguist-vendored
/.trae/scripts/**                                    linguist-vendored

# === 2. 标记生成产物（不计入语言统计；diff 折叠；blame 跳转上一提交） ===
/app/encv-mobile/android/capacitor-cordova-android-plugins/**  linguist-generated
/app/encv-mobile/android/capacitor.settings.gradle             linguist-generated
/app/encv-mobile/android/app/src/main/assets/capacitor.config.json linguist-generated
/.github/workflows/**                                   linguist-generated

# === 3. 二进制文件关闭 diff（避免 PR review 卡顿） ===
app/encv-mobile/plugin-openlist/src/main/assets/openlist/assets/** binary
**/*.png  binary
**/*.jpg  binary
**/*.jar  binary
**/*.aar  binary
**/*.db   binary
**/*.sccgt binary

# === 4. 大体积 JS bundle（Vite 产物）：关闭 diff，使用上一提交归因 ===
app/encv-mobile/plugin-openlist/src/main/assets/openlist/assets/*.js diff=skip
app/encv-mobile/.agents/skills/**/*.bundle.cjs          diff=skip
app/encv-mobile/.agents/skills/**/*.bundle.mjs          diff=skip
```

**关于 `linguist-vendored` vs `linguist-generated` 区别**：
- `linguist-vendored` — **不计入**语言统计，且搜索默认不索引
- `linguist-generated` — **不计入**语言统计，但搜索仍索引
- 在本项目里，**第三方 skill 与 dist 产物**应同时满足「不计入 + 不参与代码搜索」 → 用 `linguist-vendored`
- **CI workflow 文件**用 `linguist-generated`（仍允许搜索定位）

### 3.3 不需要变更的文件

- `app/encv-mobile/.gitignore` — 不需要改
- `.gitignore`（根）— 不需要改
- 现有 `app/encv-mobile/android/.gitignore` — 不需要改

---

## 四、Assumptions & Decisions（已锁定的决策）

1. **不整体排除 `app/encv-mobile/android/`** — 因为含定制 Kotlin（GoProcessPlugin、PluginLifecycleEngine 等），整体排除会误伤这些 Kotlin 文件的语言识别。仅对其中的「纯生成」子目录用 `linguist-generated` 标 diff 行为。

2. **保留 Capacitor Java/Kotlin 文件的语言统计** — 定制 Kotlin 是项目交付代码（player / plugin / combolite-host）。

3. **`pnpm-lock.yaml` 与 `package-lock.json` 列入 ignore** — 锁文件不应计入语言统计（GitHub 默认就会，但显式列出以防误判）。

4. **不修改 `.github/workflows/` 下的 YAML 自身** — 仅把它们标记为 `linguist-generated`（CI 配置不应被用户误以为是项目主代码语言）。

5. **`.trae/scripts/setup-kotlinc.sh` 等脚本** — 既然 `.trae/scripts/**` 已整目录排除，此脚本不计入语言统计。决策依据：`.trae/` 全树是 AI agent 工作产物，**非项目交付物**。

6. **未变更项目代码结构** — 本计划**只新增两个规则文件**，不动任何源码 / 依赖 / CI。

---

## 五、Verification（验证步骤）

按以下顺序验证：

1. **本地检查 `.gitattributes` 语法**：
   ```bash
   git check-attr -a -- app/encv-mobile/.agents/skills/lynx-trace-record/scripts/shared.bundle.cjs
   # 预期输出包含 linguist-vendored 与 diff=skip
   ```

2. **本地检查 `.github/linguist.yml` 语法**（YAML lint）：
   ```bash
   python3 -c "import yaml; yaml.safe_load(open('.github/linguist.yml'))"
   # 预期无错误
   ```

3. **行数预估对比（提交 PR 前预览）**：
   ```bash
   # 不带 linguist 规则的当前估算
   cloc . --not-match-d='(^|/)(\.agents|node_modules|\.git|dist|build)/'
   # 应用规则后预期效果（提交 PR 后 GitHub 自动重算）
   ```
   预期：JS 行数占比从 ~15% 下降到 < 8%，Kotlin 占比从 < 5% 上升到 7-8%（因为去除了 JS 噪声）。

4. **提交 PR 后在 GitHub 仓库页 Languages 栏肉眼对比**：
   - 提交到任一 fork 分支
   - 打开 PR 后查看文件变更统计
   - 合并到主分支后查看「Languages」栏
   - 预期：OpenList 资产目录的 JS 比例消失，Capacitor 第三方 skill 的 .mjs/.cjs 不再计入

5. **回归验证**：
   - 打开任意一个被标记 `linguist-vendored` 的文件（如 `lynx-trace-record/scripts/shared.bundle.cjs`），确认仓库内搜索能搜到但 Languages 栏不计
   - 在 `app/encv-mobile/android/app/src/main/java/com/encvgo/app/MainActivity.kt` 上做一次 blame，确认定制 Kotlin 仍能正常 blame（**未受 `linguist-generated` 误伤**）

---

## 六、风险与回退

| 风险 | 概率 | 缓解 |
|------|------|------|
| `.gitattributes` 通配规则过宽，误伤 `app/encv-mobile/src/**` 下的 first-party 文件 | 低 | 验证步骤 1+5 会在 PR 前/后回归验证；若误伤，立即 revert 整个 PR |
| GitHub 缓存导致 Languages 栏 5-10 分钟才更新 | 中 | 在 PR 描述中提示 reviewer「合并后等待 5-10 分钟再看 Languages 栏」 |
| `.trae/` 整目录排除影响 IDE 索引 | 低 | `linguist-vendored` 仅影响 GitHub 搜索与语言统计，不影响本地 IDE 索引 |
| 用户后续向 `app/encv-mobile/.agents/skills/` 添加 first-party skill | 低 | 在 `.trae/rules/` 中追加一条铁律：first-party skill 必须放到 `app/encv-mobile/scripts/agents-skills/` 或 `.trae/skills/`，不得直接放进 `.agents/skills/` |

---

## 七、执行清单（提交顺序）

1. 创建 `.github/linguist.yml`（路径：`.github/linguist.yml`）
2. 创建 `.gitattributes`（路径：`.gitattributes`）
3. 运行 Verification 步骤 1+2
4. 提交两个文件，附 commit message：`chore(repo): add linguist + gitattributes to exclude 3rd-party skills and dist outputs from language stats`
5. （可选）追加一条规则到 `.trae/rules/project_rules.md`：禁止 first-party 资源放进 `.agents/skills/` 或 `plugin-openlist/src/main/assets/`
