# 计划：彻底清理 Lynx 残留

> 目标：移除项目中所有 Lynx 框架相关残留（技能、资源、规划文档、注释），释放 ~4.8MB 仓库体积 + 109KB APK 体积，简化仓库语言统计。
>
> 前置条件：前序 PR 已完成 Lynx 主体代码移除（PlayerActivityLynx、PlayerOverlayManager、所有 LynxModule、EncvApplication 中的 Lynx 初始化、AndroidManifest 引用、Gradle 依赖全部已清）。本计划处理**剩余残留**。

---

## 一、Phase 1 探索结果

### 1.1 主体代码已无 Lynx 引用（✅ 已被前序 PR 清理）

| 文件 | 状态 |
|------|------|
| `app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvApplication.kt` | ✅ 干净（仅有 Bugly + ComboLite 初始化） |
| `app/encv-mobile/android/app/src/main/AndroidManifest.xml` | ✅ 干净（无 PlayerActivityLynx 注册） |
| `app/encv-mobile/android/app/proguard-rules.pro` | ✅ 干净（无 `-keep class com.lynx.**`） |
| `app/encv-mobile/android/gradle/libs.versions.toml` | ✅ 干净（无 Lynx 依赖） |
| `app/encv-mobile/android/app/build.gradle.kts` | ✅ 干净（无 Lynx 依赖） |
| `app/encv-mobile/capacitor.config.ts` | ✅ 干净 |
| `app/encv-mobile/package.json` | ✅ 干净（无 `@lynx-js/*` 依赖） |
| `app/encv-mobile/trapeze.yaml` | ✅ 干净 |
| `app/encv-mobile/src/**/*.{ts,vue}` | ✅ 干净（无 `lynx` / `Lynx` 引用） |
| `app/encv-mobile/plugin-mpv-player/src/**` | ✅ 干净（已迁移为 Compose + MPV） |
| `app/encv-mobile/plugin-openlist/src/**` | ✅ 干净 |
| `app/encv-mobile/android/combolite-host/src/**` | ✅ 干净 |
| `agent/**`、`cmd/**`、`internal/**`、`pkg/**`（Go 后端） | ✅ 干净 |
| `scripts/build-*.sh`、`scripts/sync-native.mjs` | ✅ 干净 |

### 1.2 剩余残留清单

#### A. Lynx 第三方技能（`app/encv-mobile/.agents/skills/`，共 4.7MB）

| 目录 | 大小 | 内容类型 | 决策 |
|------|------|----------|------|
| `lynx-devtool/` | 1.1M | Lynx DevTool CDP 桥接（含 739KB `scripts/index.mjs`） | 🗑️ 删除 |
| `lynx-trace-analysis/` | 2.0M | Lynx trace 分析（含 1.2MB `shared.bundle.cjs` + 638KB `trace_query.bundle.cjs`） | 🗑️ 删除 |
| `lynx-trace-record/` | 1.3M | Lynx trace 录制（含 1.2MB `shared.bundle.cjs`） | 🗑️ 删除 |
| `lynx-typescript/` | 16K | Lynx TypeScript 类型说明 | 🗑️ 删除 |
| `reactlynx-best-practices/` | 68K | ReactLynx 编码规范 | 🗑️ 删除 |
| `fiber-element/` | 64K | Lynx Fiber 元素 | 🗑️ 删除 |
| `habitat-usage/` | 16K | Lynx Habitat 工具 | 🗑️ 删除 |
| `debug-info-remapping/` | 20K | Lynx debug 符号映射 | 🗑️ 删除 |

#### B. 物理资产（109KB）

| 路径 | 大小 | 用途 | 决策 |
|------|------|------|------|
| `app/encv-mobile/android/app/src/main/assets/player.lynx.bundle` | 109K | 旧 Lynx 播放器 bundle（仍在 APK assets 里打包） | 🗑️ 删除（释放 APK 体积） |

#### C. 规划文档

| 路径 | 大小 | 决策 |
|------|------|------|
| `.trae/specs/lynx-native-player/` | 28K | 🗑️ 删除整个 spec 目录（spec.md + tasks.md + checklist.md） |

`.trae/documents/` 中 11 个以「lynx」为主题的历史计划：
- `lynx-player-react-sparkling-migration.md`
- `lynx-player-ui-fix.md`
- `lynx-react-to-vue-migration-analysis.md`
- `remove-lynx-plan.md`
- `fix-lynx-bundle-not-packaged.md`
- `fix-lynx-nativemodules-admin404-background-service.md`
- `fix-lynx-page-duplicate-and-thread-safety.md`
- `fix-black-screen-lynx-rendering.md`
- `encrypt-detection-task-list-lynx-player-fix.md`
- `fix-mpv-module-null-with-lynx-ui.md`
- `fix-mpv-crash-lynx-bundle-missing.md`

🗑️ 全部删除（这些计划的变更已实施完成，文档本身是历史变更记录，与当前代码脱钩）

#### D. 注释引用

| 文件 | 行 | 内容 | 决策 |
|------|----|------|------|
| `scripts/ci-check-no-nodejs-crypto.sh` | 75 | `# AI agent skill 打包产物（含 lynx-devtool / lynx-trace-* 的 bundle）...` | 🔧 改写为通用说明，不再提具体 skill 名 |

#### E. 项目规则引用

| 文件 | 章节 | 内容 | 决策 |
|------|------|------|------|
| `.trae/rules/project_rules.md` | 「Skill 目录归属铁律」 | 提到「Lynx 工具链的第三方 skill 副本，含 5MB+ bundle 文件」 | 🔧 改写为「Capacitor / Ionic / etc. 第三方技能副本」（删除具体 Lynx 数量） |

#### F. 历史日志（不处理）

- `logcat.txt` 与 `logcat.txt.sccgt` — 包含 Lynx 历史日志，但属调试历史文件，不在清理范围
- `node_modules/` 下的 `node:crypto` 引用 — 已被 `.gitattributes` 排除

### 1.3 不需要变更的文件

- `.github/linguist.yml` — 已用 `app/encv-mobile/.agents/skills/**` 通配，删除子目录后规则仍生效
- `.gitattributes` — 同上
- `app/encv-mobile/.gitignore` — 已有相关规则
- 现有 5 个 `.github/workflows/*.yml` — 全部干净

---

## 二、Proposed Changes（变更清单）

### 2.1 删除 8 个 Lynx 相关技能（顺序无关，可并行）

```bash
rm -rf /workspace/app/encv-mobile/.agents/skills/lynx-devtool/
rm -rf /workspace/app/encv-mobile/.agents/skills/lynx-trace-analysis/
rm -rf /workspace/app/encv-mobile/.agents/skills/lynx-trace-record/
rm -rf /workspace/app/encv-mobile/.agents/skills/lynx-typescript/
rm -rf /workspace/app/encv-mobile/.agents/skills/reactlynx-best-practices/
rm -rf /workspace/app/encv-mobile/.agents/skills/fiber-element/
rm -rf /workspace/app/encv-mobile/.agents/skills/habitat-usage/
rm -rf /workspace/app/encv-mobile/.agents/skills/debug-info-remapping/
```

**预期**：`app/encv-mobile/.agents/skills/` 目录从 8 个技能（~4.5MB+170KB）变为 0 个（整个子目录可整体删除）。

### 2.2 删除 Lynx 物理资产

```bash
rm /workspace/app/encv-mobile/android/app/src/main/assets/player.lynx.bundle
```

**验证**：APK 解包检查 `assets/player.lynx.bundle` 不再存在（仅在 release 构建后验证；开发阶段不强制）。

### 2.3 删除 Lynx 规划文档

```bash
# Spec
rm -rf /workspace/.trae/specs/lynx-native-player/

# 11 个 Lynx 主题 plans
rm /workspace/.trae/documents/lynx-player-react-sparkling-migration.md
rm /workspace/.trae/documents/lynx-player-ui-fix.md
rm /workspace/.trae/documents/lynx-react-to-vue-migration-analysis.md
rm /workspace/.trae/documents/remove-lynx-plan.md
rm /workspace/.trae/documents/fix-lynx-bundle-not-packaged.md
rm /workspace/.trae/documents/fix-lynx-nativemodules-admin404-background-service.md
rm /workspace/.trae/documents/fix-lynx-page-duplicate-and-thread-safety.md
rm /workspace/.trae/documents/fix-black-screen-lynx-rendering.md
rm /workspace/.trae/documents/encrypt-detection-task-list-lynx-player-fix.md
rm /workspace/.trae/documents/fix-mpv-module-null-with-lynx-ui.md
rm /workspace/.trae/documents/fix-mpv-crash-lynx-bundle-missing.md
```

**保留**：`fix-black-screen-react-mount.md`、`fix-black-screen-root-render.md` 等以 MPV/Vue 为主体的文件（仅在历史背景中提及 ReactLynx，不属于 Lynx 主题）。

### 2.4 改写 CI 脚本注释（[ci-check-no-nodejs-crypto.sh:75](file:///workspace/scripts/ci-check-no-nodejs-crypto.sh#L75)）

**现状**：
```bash
# AI agent skill 打包产物（含 lynx-devtool / lynx-trace-* 的 bundle），
# 内部引用 node:crypto 用于生成 trace ID、createHash 做 URL hash 等，
# 属于 dev tool 自身能力，与 API Key 加密无关 → 排除。
```

**改后**：
```bash
# AI agent skill 打包产物（.agents/skills/ 与 .trae/skills/ 下的 .mjs/.cjs bundle），
# 内部引用 node:crypto 用于生成 trace ID、createHash 做 URL hash 等，
# 属于 dev tool 自身能力，与 API Key 加密无关 → 排除。
```

### 2.5 改写项目规则（[project_rules.md:411-441](file:///workspace/.trae/rules/project_rules.md#L411-L441)）

**修改点 1**：删除 Lynx 引用
```diff
- - **SHALL NOT** 向 `.agents/skills/**` 提交 first-party 技能定义 — 该目录是 Trae IDE / Capacitor / Ionic / Lynx 等第三方技能的存储位置（语言统计排除 + 搜索不索引）
+ - **SHALL NOT** 向 `.agents/skills/**` 提交 first-party 技能定义 — 该目录是 Trae IDE / Capacitor / Ionic 等第三方技能的存储位置（语言统计排除 + 搜索不索引）
```

```diff
- - **SHALL NOT** 向 `app/encv-mobile/.agents/skills/**` 提交 first-party 脚本 — 该目录是 Lynx 工具链的第三方 skill 副本，含 5MB+ bundle 文件
+ - **SHALL NOT** 向 `app/encv-mobile/.agents/skills/**` 提交 first-party 脚本 — 该目录是 Capacitor / Lynx 工具链等第三方 skill 副本目录，体积庞大（3MB+ bundle）
```

**修改点 2**：标题区对 lynx 残留目录的描述改为空
```diff
- > **`.agents/skills/` 与 `plugin-openlist/src/main/assets/` 已被 `.github/linguist.yml` 与 `.gitattributes` 标记为 `linguist-vendored`/`linguist-generated`，提交到这两处的代码不会出现在仓库 Languages 栏。**
+ > **`.agents/skills/`（含 `.trae/skills/` 与 `app/encv-mobile/.agents/skills/`）与 `plugin-openlist/src/main/assets/` 已被 `.github/linguist.yml` 与 `.gitattributes` 标记为 `linguist-vendored`/`linguist-generated`，提交到这两处的代码不会出现在仓库 Languages 栏。**
```

### 2.6 不需要变更

- `.github/linguist.yml` 保持不变（`app/encv-mobile/.agents/skills/**` 通配仍有效）
- `.gitattributes` 保持不变
- 所有 first-party 源码保持不变

---

## 三、Assumptions & Decisions（已锁定）

1. **不再重新引入 Lynx** — 用户明确「lynx 已不再使用」，未来不应再向仓库添加 Lynx 相关资产。`project_rules.md` 的修改保留「禁止 first-party 资源放入 `.agents/skills/`」的规则，作为防御性约束。

2. **保留历史日志** — `logcat.txt`、`logcat.txt.sccgt` 是调试用历史归档，不在清理范围。

3. **保留混合主题文档** — `fix-mpv-*` 系列文档虽然提及 Lynx，但核心话题是 MPV 播放器问题（lynx 仅作为历史背景），不删除。

4. **保留 Capacitor / Ionic 第三方技能** — `.agents/skills/capacitor-*`、`.agents/skills/ionic-*` 等 23 个非 Lynx 技能**不动**，是 IDE 工具链功能，与本任务无关。

5. **`black-screen-*` 系列文档** — 保留 `fix-black-screen-react-mount.md`、`fix-black-screen-root-render.md`、`fix-black-screen-viewport-zero.md`、`fix-black-screen-add-debug-client.md`（它们核心话题是 Vue/渲染根因，lynx 仅是历史上下文）。仅 `fix-black-screen-lynx-rendering.md` 属于 Lynx 主题，删除。

6. **未变更项目代码结构** — 本计划**只删除文件 + 改 2 个小注释**，不动任何源码 / 依赖 / CI / 配置。

---

## 四、Verification（验证步骤）

按以下顺序验证：

1. **本地搜索残留**：
   ```bash
   # 应该零结果（排除已识别的历史日志与 spec 文件名残留）
   grep -ri "lynx" /workspace --include="*.ts" --include="*.vue" --include="*.kt" --include="*.go" \
     --include="*.gradle*" --include="*.toml" --include="*.json" --include="*.xml" --include="*.pro" \
     --include="*.yaml" --include="*.yml" 2>/dev/null \
     | grep -v node_modules | grep -v ".sccgt" | grep -v "logcat.txt"
   # 预期：仅剩 scripts/ci-check-no-nodejs-crypto.sh 注释（已改写为通用说明）
   #       + .trae/documents/ 中非 lynx 主题但提及 lynx 的 mpv/black-screen 文档
   ```

2. **目录存在性**：
   ```bash
   for d in lynx-devtool lynx-trace-analysis lynx-trace-record lynx-typescript \
            reactlynx-best-practices fiber-element habitat-usage debug-info-remapping; do
     [ -d "/workspace/app/encv-mobile/.agents/skills/$d" ] && echo "❌ $d still exists" || echo "✅ $d removed"
   done
   # 预期：8 行 ✅
   ```

3. **bundle 文件检查**：
   ```bash
   [ -f /workspace/app/encv-mobile/android/app/src/main/assets/player.lynx.bundle ] \
     && echo "❌ bundle still exists" || echo "✅ bundle removed"
   ```

4. **spec 检查**：
   ```bash
   [ -d /workspace/.trae/specs/lynx-native-player ] \
     && echo "❌ spec still exists" || echo "✅ spec removed"
   ```

5. **11 个文档删除确认**：
   ```bash
   for f in lynx-player-react-sparkling-migration lynx-player-ui-fix \
            lynx-react-to-vue-migration-analysis remove-lynx-plan \
            fix-lynx-bundle-not-packaged fix-lynx-nativemodules-admin404-background-service \
            fix-lynx-page-duplicate-and-thread-safety fix-black-screen-lynx-rendering \
            encrypt-detection-task-list-lynx-player-fix fix-mpv-module-null-with-lynx-ui \
            fix-mpv-crash-lynx-bundle-missing; do
     [ -f "/workspace/.trae/documents/$f.md" ] && echo "❌ $f.md still exists" || echo "✅ $f.md removed"
   done
   # 预期：11 行 ✅
   ```

6. **CI 脚本无 lynx 字面引用**：
   ```bash
   grep -n "lynx" /workspace/scripts/ci-check-no-nodejs-crypto.sh
   # 预期：无输出
   ```

7. **project_rules.md 已更新**：
   ```bash
   grep -n "lynx" /workspace/.trae/rules/project_rules.md
   # 预期：无输出（或仅 "Capacitor / Lynx" 历史说明残留）
   ```

8. **体积对比**：
   ```bash
   du -sh /workspace/app/encv-mobile/.agents/skills/
   # 清理前：4.5M+
   # 清理后：应大幅减少（删除 4.5MB+lynx 技能）
   ```

9. **gitattributes / linguist.yml 仍正确标记剩余目录**（防御性）：
   ```bash
   git check-attr -a /workspace/app/encv-mobile/.agents/skills/capacitor-angular/SKILL.md
   # 预期：linguist-vendored: set（规则不变）
   ```

10. **CI 不受影响**：
    ```bash
    grep -l "lynx" /workspace/.github/workflows/*.yml
    # 预期：无输出
    ```

---

## 五、风险与回退

| 风险 | 概率 | 缓解 |
|------|------|------|
| 删除的 skill 某天需要重新引入 | 低 | Skill 仓库（Trae / Capacitor / Lynx 各自维护）可通过 `git clone --depth 1` 重新拉取；不需要时无需回退 |
| 11 个 plan 含历史变更记录，删除后无法溯源 | 低 | 所有变更均已 commit 入 git 历史（`git log -- <file>` 可回溯） |
| APK 仍打包 player.lynx.bundle（同步问题） | 极低 | 下次 `cd app/encv-mobile && npx cap sync android` 后会被 Capacitor 同步；assets 目录在 sync 阶段会被覆盖 |
| 用户后续手动添加 Lynx 资源 | 中 | `project_rules.md` 的「Skill 目录归属铁律」仍生效；新增的 `linguist.yml` 通配规则仍兜底 |
| git 历史保留但本地副本丢失 | 极低 | `git log` + `git show <commit>` 可恢复任何已删除文件 |

---

## 六、执行清单（提交顺序）

1. 删除 8 个 skill 目录（`rm -rf`）
2. 删除 player.lynx.bundle
3. 删除 `.trae/specs/lynx-native-player/`
4. 删除 11 个 lynx 主题 plan
5. 改写 `scripts/ci-check-no-nodejs-crypto.sh:75` 注释
6. 改写 `.trae/rules/project_rules.md` 中 2 处 lynx 引用
7. 运行 Verification 步骤 1-10
8. 提交（commit message：`chore(repo): remove all Lynx-related residuals (skills, bundle, spec, plans)`）

---

## 七、清理后仓库语言统计预期变化

按之前 plan 估算：
- 仓库体积减少 ~4.8MB（4.5MB lynx 技能 + 109KB bundle + 28KB spec + 11 个 md 文档）
- APK 体积减少 ~109KB（`player.lynx.bundle` 不再打包）
- GitHub Languages 栏：JS 行数占比进一步降低（删除 2 个大 bundle）
- `.trae/documents/` 数量：-11（54 → 43）
