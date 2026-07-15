# ENCV 主题开发指南（Phase 5）

本文件说明如何为 ENCV 开发「用户主题」与「CSS Snippets」——参考 Obsidian
（`manifest.json` + `theme.css`）与思源笔记（`theme.json` + `theme.css` + 可选
`theme.js`）的社区扩展范式。

## 1. 三层令牌模型

主题只覆盖 **语义令牌**，不碰组件。令牌链：

```
primitive（primitive 色）→ semantic（--color-* / --radius-*）→ component（组件以令牌自包含样式，**无共享类词汇**）
```

- 业务/组件直接以设计令牌（`--color-*` / `--radius-*`）自包含样式（各组件 scoped `<style>`），
  不再依赖任何品牌化共享类（`.encv-*` 已于 2026-07-16 废除，见 §6.4）。
- 调色板单一来源：`packages/shared-components/src/theme/palette.css`。
- 兼容基线：**Chromium 114+**，仅用 hex + `color-mix()` + 标准自定义属性，
  禁止 `light-dark()` / `allow-discrete` / `field-sizing`（见主方案 §2.8）。

## 2. 用户主题（User Theme）

### 2.1 encv-mobile 主应用（纯 CSS 路径，推荐起点）

主应用走 `theme-core.css`（纯 CSS，无 Tailwind 构建管线），用户主题即一组
`[data-theme="<id>"]` 的纯 CSS 覆盖，**运行时切换 `data-theme` 即换肤，零构建依赖**。

1. 在 `theme/user-themes.css` 复制一个 `[data-theme="..."]` 块，补全 **全部 19 色**
   （base-100/200/300/content、primary(+content)、secondary(+content)、accent(+content)、
   neutral(+content)、info(+content)、success(+content)、warning(+content)、error(+content)）
   + 形状令牌（`--radius-selector/--radius-field/--radius-box`）。颜色可用 hex / OKLCH。
2. 在 `composables/useUserThemes.ts` 的 `USER_THEMES` 注册表登记 `{ id, nameKey }`。
3. 在 `i18n/settings.ts` 加 `nameKey` 的 zh-CN / en 双语文案。
4. 运行 `app_check_all` 验证。

切换逻辑见 `useUserThemes.applyTheme(id)`：写 `documentElement[data-theme]` + 持久化；
默认主题 `encv` 移除属性以回落 `:root`。

### 2.2 插件 web 应用（daisyUI 编译期路径）

插件 web 走 `@plugin "daisyui/theme"`，daisyUI 主题是「声明式 + 编译期」，运行时**不能**
动态新增 `data-theme` 名。用户主题必须在构建期注册进 config：

- 在 `styles/daisyui.css` 的 `@plugin "daisyui/theme"` 块追加主题（与 palette.css 的
  `--color-*` 值保持一致），或在 `vite-plugins/daisy-ui.ts` 扫描 `theme/user-themes/`
  目录生成对应块（CI 拼装）。
- 运行时只切换 `data-theme="<已注册名>"`（与思源「集市」式分发 + 构建期打包一致）。

## 3. CSS Snippets（局部覆盖）

Snippet = 一段可热开关的局部 CSS，注入到 `<head>` 的
`<style data-encv-snippet="<id>">`，关闭即移除。适合微调对比度、间距、圆角等，
不影响整体主题。

- 注册：在 `composables/useSnippets.ts` 的 `SNIPPETS` 登记 `{ id, labelKey, css }`
  （CSS 字符串可内联，或来自 `theme/snippets/*.ts`，生产环境来自
  `user-themes/<id>/theme.css` / 远程清单）。
- 开关：`<ion-toggle @ionChange="toggle(id)">`；状态经 `useSnippets.isEnabled(id)` 绑定，
  持久化键 `encv-snippets`。
- 注入的 `<style>` 在 DOM 中位于样式表之后、优先级高于内建规则，实现「局部覆盖」语义。

## 4. 可选 theme.js 钩子

参考思源，主题可带一个 `theme.js`，在挂载/卸载时执行 JS（如 `installMotion()` 触发
主题光泽过渡，见 `motion/ambient.ts` 的 `applyThemeGlow`）。主应用当前通过
`useUserThemes.applyTheme` 直接切 `data-theme`，如需钩子可在此处扩展。

## 5. 验收

- [ ] 用户主题可在 Appearance「主题」段切换并持久化（重启保留）。
- [ ] Snippets 可在 Appearance「片段」段热开关并持久化。
- [ ] 产物 CSS 仅面向 Chromium 114+（无越基线特性）。
- [ ] `app_check_all` 全绿。

## 6. 已知技术债与待办

### 6.1 ~~`.encv-*` 类名即契约，改名成本高~~（已废弃，2026-07-16 解决）

> ⚠️ 本节所述问题已通过「废除共享品牌类词汇、组件改以设计令牌自包含样式」根除，详见 §6.4。以下保留原分析供溯源。

- **问题（原分析）**：`.encv-*` 曾是组件层事实上的 API 契约（见 §1 令牌链末端），业务/组件直接消费类名。
  在纯 HTML/模板里 CSS 类名**没有编译期别名**：`@extend` 只是把硬名搬到 SCSS、模板仍写死
  `class="encv-chip"`；`@custom-selector` 仅分组选择器、不能重命名 HTML 里的类。故**消费方直接
  写类名字面量时，改名须逐消费方改**，爆炸半径随接入组件增长。
- **现状爆炸半径**（截至 2026-07-16）：已接入 6 个组件——`CollapsedMessageToggle` /
  `MockPresetBar` / `StatusBadge` / `V2QuickActions` / `FileChangeSummaryMessage` /
  `GroupedOperationMessage`；其余 agent 组件为 bespoke 件，暂未接入。
- **结论**：风险真实，但**可消除**（见 §6.2）。当前未消除仅因消费方直接用字面量、尚未引入
  间接层——不是 CSS 的固有死局。

### 6.2 ~~消除改名成本的可行间接层~~（已废弃，2026-07-16 推翻）

> ⚠️ 本节原推荐的「重命名间接层」（方案 A′ / 6.2.y SCSS+TS 映射）经实践被认定为**掩耳盗铃的解耦**：
> 它只是让品牌前缀可改名，并未消除「组件依赖共享类词汇」这一根本耦合。2026-07-16 改为
> **直接废除共享类词汇**（组件以令牌自包含样式），该间接层方案已撤销。保留原文供溯源。

核心思路（原分析）：把「CSS 类名字符串」与「消费方引用的稳定标识符」解耦，且**把前缀与词表收成
单一事实源**，使重命名（前缀 / 单个类名）都只动一处、并自动同步到 CSS 与 TS 两端。

> **先澄清方案 A 的不足**：原版常量表把前缀烤进每个字面量
>（`chip:"encv-chip"`、`badgeSuccess:"encv-badge-success"`…），前缀 `encv` 仍散在 N 个值 +
> N 个 CSS 选择器里——改前缀照样批改。下面方案 A′ 解决这一点。

- **方案 A′ — 单源 + 一次性代码生成（推荐，零常驻工具链、真·O(1)）**：
  单一事实源只声明「语义词」（无前缀、无样式）+ 一个前缀常量：
  ```ts
  // theme/encv-meta.ts  —— 唯一事实源
  export const ENCV_PREFIX = "encv";
  export const encvTokens = [
    "chip", "chip-primary",
    "badge-success", "badge-warning", "badge-error", "badge-neutral",
    /* …其余词汇 */
  ] as const;

  // 语义词 → TS 属性名（保持消费方 API 稳定，与 emitted class 名解耦）
  export const encvKeyMap = { chip: "chip", "chip-primary": "chipPrimary",
    "badge-success": "badgeSuccess", /* … */ } as const;
  ```
  跑**一次性生成脚本** `scripts/gen-encv.mjs`（或 SCSS `@each`）从这两样产出两份产物并提交：
  - `encv-classes.ts`：`{ chip: \`${ENCV_PREFIX}-chip\`, chipPrimary: \`${ENCV_PREFIX}-chip-primary\`, … }`
    —— 消费方 `:class="encv.chip"`（类字符串彻底不在消费方出现）。
  - `components.generated.css`：`.${ENCV_PREFIX}-chip { … }`、`.${ENCV_PREFIX}-badge-success { … }` …
    选择器全部由前缀 + 词表拼接，**不再手写任何 `.encv-*` 字面量**。
  - **改名成本**：
    - 改**前缀** `encv`→`x`：改 `ENCV_PREFIX` **一处** → 重跑脚本 → CSS + TS 同时更新；
      所有消费方 `encv.chip` 无感。**真 O(1)**。
    - 改**单个 emitted 类名** `chip`→`tag`：改 `encvTokens` 里一个词（属性名 `chip` 经 `encvKeyMap`
      保留）→ 重跑 → CSS 选择器 + TS 映射同步；消费方无感。
    - 改名**概念** `chip`→`pill`（即连 API 也换）：改 `encvKeyMap` 的键 → 消费方 `encv.chip`
      需跟着改，这属于**有意的 API 破坏性变更**（合理，应走协调 sweep）；非前缀/非 cosmetic 改名。
  - **代价**：多一层生成物（`gen-encv.mjs` 可挂 `prebuild` 或手动跑、产物入库）；但**无常驻构建插件**、
    不引预处理、不破「纯 CSS / daisyUI 无关」契约（生成的 `.css` 仍是纯 CSS、TS 部分无 `@apply`）。

- **方案 A（原版，仅消费方 O(1)）**：`encv-classes.ts` 直接写死字面量值的常量表（见上「不足」说明）。
  只解消费方一端，前缀改名仍批改——**不作为推荐**，仅记录其思路。

- **方案 B — Vite 别名插件（模板保持字面量）**：写一个小 Vite/PostCSS 插件持有「别名 → 真名」表
  （如 `chip → encv-chip`），编译期把模板里的 `class="encv:chip"` 重写为 `class="encv-chip"`、
  并同步 `components.css` 选择器。模板零改动，但**前缀改名仍需改插件表 + CSS 选择器两处**，且引入
  非标准语法 `encv:chip` + 常驻构建插件——相较方案 A′ 无优势，**不推荐**。

### 6.2.x 契约再审视：「纯 CSS / 禁预处理」是否值得（2026-07-16 用户质疑）

- **用户两点质疑**：(1) 「纯 CSS 契约」有必要吗？(2) 现代 CSS 已原生支持嵌套。
- **澄清 native CSS 嵌套的边界**：原生嵌套能替代 SCSS 的「嵌套」能力，但**不能**替代 SCSS 的
  「选择器插值」——CSS 没有把变量拼进选择器（`.#{$prefix}-chip`）的语法。故单靠 native CSS
  **仍解不了 O(1) 前缀改名**；前缀改名要单点，必须靠预处理插值或代码生成。
- **契约其实含两条价值不同的约束**：
  1. **daisyUI 无关（禁 `@apply` / 禁 daisyUI 工具类）**——**真价值，应保留**：去掉 daisyUI 时
     `components.css` 零改动，因样式全靠裸 CSS 规则 + `--color-*` 变量，不依赖 daisyUI 组件类 API。
  2. **纯 CSS / 禁预处理**——**价值很弱**：唯一换来「换构建工具容易」，但本就用 Vite（`sass`
     一行集成）；代价却是为 O(1) 改名被迫搞 `gen-encv.mjs` 代码生成。且 `palette.css` 本就是
     daisyUI 调色板的纯 CSS 镜像（要与 `daisyui.css` 的 `@plugin` 块同步），"纯 CSS 独立"本就未彻底实现。
  **结论**：保留 (1)，放松 (2)——引入 `sass` 不破坏「daisyUI 无关」，仅放弃「零构建工具耦合」。
- **关键且不明显的推论**：即便用 SCSS，消费方那层间接仍不可省。SCSS `$prefix`+`@each` 只解决
  **CSS 端**单点；组件模板若仍写死 `class="encv-chip"` 字面量，前缀一改模板字符串就跟生成选择器
  对不上、消费方照样要改。故消费端 O(1) 仍需 `:class="encv.chip"`（TS 常量表，前缀一处）或方案 B 重写。

### 6.2.y ~~修订推荐：放松契约 → SCSS + TS 映射~~（已撤销，见 §6.4）

- **做法**（保留 daisyUI 无关、放弃纯 CSS、不引代码生成）：
  1. `components.css` → `components.scss`；顶部 `$encv-prefix: "encv";`，词表
     `$encv-tokens: (chip, chip-primary, badge-success, badge-warning, badge-error, badge-neutral);`，
     用 `@each $t in $encv-tokens { .#{$encv-prefix}-#{$t} { … } }` 生成选择器——
     **CSS 端零手写 `.encv-*` 字面量、前缀改名 = 改 `$encv-prefix` 一处**。
  2. `theme/encv-classes.ts` 持 `const PREFIX = "encv"` + token→key 映射，导出
     `encv = { chip: \`${PREFIX}-chip\`, … }`；消费方 `:class="encv.chip"`。
  3. 词表在 SCSS 与 TS 两处各列一份（**稳定、极少变动**）；前缀在两处各一处常量。
- **改名成本**：改**前缀** = 改 SCSS `$encv-prefix` 一处 + TS `PREFIX` 一处 = **2 个单点编辑，
  无任何 N 处批改**；改单类名 = 改对应词（属性名经映射保留）→ 各一处；改概念 = 有意破坏性变更走 sweep。
- **对比**：比方案 A′ 更简单（无 `gen-encv.mjs`、无 `components.generated.css` 生成物），且彻底
  消除「批量替换字符串」；代价仅是新增 `sass` devDep 与放弃「纯 CSS」字样（daisyUI 无关仍保住）。
- **决策（修订）**：推荐 **6.2.y**（放松纯 CSS 契约、用 SCSS `$prefix`+`@each` + TS `encv.chip` 映射）。
  先付一次性迁移（6 组件 `class="encv-*"` → `:class="encv.*"`；`components.css` 选择器改由 SCSS
  `@each` 生成），之后前缀/单类名改名均 2 单点编辑、零批改。原 6.2 方案 A′ 留作「坚持零预处理」时的备选。

### 6.2.z 预处理器选型：SCSS vs LESS（2026-07-16 评估结论 = SCSS）

- **工程事实**：构建工具 **Vite 8.1.4**，`less`/`sass`/`sass-embedded` 均为其 optional peerDep
  （即两者都零配置原生支持），当前**都未安装**（干净选型）。
- **需求**：`$prefix` + 遍历 token 词表 + 选择器插值生成 `.encv-*`；§6.3 前瞻可能要 token→值映射。
- **结论：选 SCSS（装 `sass-embedded` 而非纯 JS `sass`）**，理由按权重：
  1. **正中需求**：`@each $t in $tokens { .#{$prefix}-#{$t} {} }` 是 SCSS 一等语法；LESS 需
     `each()`（Less 3.7+）或递归 mixin+guard，明显更绕。
  2. **§6.3 前瞻**：SCSS 有真 Map `(key: value)`+`map.get`；LESS 无真 Map（只能 detached ruleset
     模拟），是硬伤。
  3. **可持续性**：Dart Sass 是官方参考实现、活跃迭代、事实标准；LESS 社区动能下滑（Bootstrap 5
     已弃 Less 转 Sass）。
  4. **生态/团队**：本栈已是 Tailwind v4 + daisyUI v5 现代前端，SCSS 资料/熟悉度远胜 LESS。
  5. **性能**：`sass-embedded`（Dart Sass native embedded 协议）编译快，Vite 官方推荐优于纯 JS `sass`。
- **LESS 唯一相对优势**：语法更贴近 CSS/JS、入门略平缓；但"遍历+插值+映射"这类程序化生成恰是其短板。
- **注意点**：SCSS 旧 JS API 已弃用 → 用 `sass-embedded`/现代 API；`@import` 迁 `@use`/`@forward`。

### 6.3 待办：将 tint 色调 token 提升至 palette.css（方案 C）

- **目标**：消除 `theme/components.css` 中的 `#000`/`#fff` 字面量与 8 段近重复 badge/chip
  色调块；并让 `theme/user-themes.css` 的任意用户主题**免费**获得正确徽章/胶囊配色
  （当前 `color-mix(... #000/#fff)` 写死「亮=黑字/暗=白字」，用户主题若改 `--color-success`
  等会对比失效）。
- **做法**：
  1. 在 `palette.css` 的 `[data-theme="encv"]`（light）与 `[data-theme="encv-dark"]`（dark）
     两块各补一组 tint token（`#000`/`#fff` 仅在此出现一次，按 `color-scheme` 分明暗）：
     - `--encv-badge-success-bg/fg`、`--encv-badge-warning-bg/fg`、
       `--encv-badge-error-bg/fg`、`--encv-badge-neutral-bg/fg`
     - `--encv-chip-primary-bg/fg`
  2. `components.css` 退化为纯 `var()` 引用，删除 `body.dark` 分叉与所有
     `color-mix(... #000/#fff)` 体（共 8 段 badge + chip-primary 若干段）。
  3. **不改类名、不引入预处理、不破坏「daisyUI 无关 / 纯 CSS」契约**；`palette.css`
     本就是与 daisyUI 同步的单一来源（主方案 §2.7），改动同步该块即可。
- **验收**：`app_check_all` 全绿；新增用户主题时徽章/胶囊自动跟随其调色板。

### 6.4 决策（2026-07-16）：废除共享品牌类词汇，组件以令牌自包含样式

- **背景**：§6.2 的「重命名间接层」（SCSS `$prefix` + `@each` + TS `encv.chip` 映射）落地后，
  被认定为**掩耳盗铃的解耦**——它仅让前缀可改名，组件仍强耦合于一个品牌化共享类词汇
  （`.encv-*` / 改名的 `.ui-*`），耦合本质未变；且前缀改名对「换肤无关紧要、对用户主题无意义」。
- **真问题**：组件不应依赖任何「主题拥有的类名字典」。唯一稳定且中性的契约是**设计令牌层**
  （`--color-*` / `--radius-*`），它与品牌、与 daisyUI 均无关。
- **做法**：
  1. 删除 `theme/components.scss`（品牌化共享类词汇）与 `theme/ui-classes.ts`（改名间接层）；
     `theme-core.css` / `daisyui.css` 移除对其 `@import`；`sass-embedded` devDep 随后可清理。
  2. 各消费组件（`V2QuickActions` / `MockPresetBar` / `GroupedOperationMessage` /
     `FileChangeSummaryMessage` / `CollapsedMessageToggle` / `StatusBadge`）将 chip / 徽章色调等
     **以 scoped 样式 + 设计令牌内联表达**，组件自包含、零共享类依赖。
  3. `BackToMain.vue` 的 `encv-iframe` 改名局部 `openlistFrame`（插件本地 one-off，本就非共享词汇）。
- **结果**：不再有「类名字典」这一契约，谈不上改名成本；前缀/单类名改名问题**从架构上消失**。
  视觉一致性由各组件直接复用同一组令牌保证（同一 `--color-primary` 等），而非靠共享类强制。
- **与 §6.3 的关系（续 25，02:38）**：上述 6 个组件现已把 tint 的前景改为朝
  `var(--color-base-content)`、背景改为朝 `var(--color-base-100)` 派生，并**删除 `body.dark` 分支**，
  明暗随主题自动翻转，任意用户主题免费获得正确对比——因此这 6 个组件**不再需要** §6.3 的
  「tint 提级 palette.css」方案 C。但搜索发现**另有约 13 个组件**（`SlashMenu` / `PlanBlock` /
  `OperationCard` / `MountListCard` / `MockBranchChoiceBar` / `FileReferenceChip` / `FileListCard` /
  `FileContentCard` / `ApprovalCard` / `AgentTaskMessage` / `ErrorMessage` / `AgentDebugPanel` 等）
  仍硬编码 `color-mix(... 85%, #000/#fff)` 作为前景对比，不随主题翻转（暗色用户主题下会黑字压暗底）。
  该 sweep 与本次 abolition 同一根因，建议统一改为朝 `base-content`/`base-100` 派生后一并消除。

### 6.5 续27（02:5x）：主题 = 稳定语义全局类 + 令牌两层契约（达 SiYuan 同级且更优）

- **反思**：续24 的 abolition 把「品牌共享类词汇 `.encv-*`」连同「稳定语义定位器」一起废了，
  组件改 `<style scoped>` + 字面量。这导致主题**只能换色/圆角**（令牌覆盖得到的维度），
  且 `<style scoped>` 给选择器加 `[data-v-x]`（specificity 0,2,0），用户写 `.xxx{}`（0,1,0）
  **永远赢不了**——比废除前（至少共享 `.encv-*` 可全局覆盖）还退步。续26 用户点破：
  **主题 ≠ 换色板**，思源笔记能让用户用极简选择器任意改任意元素，我们要达到同级甚至更好。
- **根因确认**：SiYuan 主题强大，是因为 DOM 上焊着**稳定的、语义的、全局的 CSS 类名**
  （`.b3-list` / `.protyle-wysiwyg` …），主题就是一份普通 CSS，靠后加载的级联天然胜出，无需 `!important`。
  我们的死穴正是「无稳定语义全局钩子 + scoped 封死级联」。
- **新契约（两层）**，落地于 `theme/surface.css`：
  1. **全局语义「表面」类**（无 scoped）：`.ui-chip` / `.ui-badge`（含 `--success/--warning/--error/--neutral`）
     / `.ui-card` / `.ui-panel` / `.ui-button` / `.ui-toggle` / `.ui-bubble` / `.ui-header` / `.ui-input`。
     这些是公开主题 API（不轻易改名），消费令牌定义默认外观。
  2. **令牌层**（续26 维度补齐）：`tokens.css` 新增 `--font-sans/--font-mono`、`--text-xs…--text-xl`、
     `--leading-*`、`--weight-*`、`--space-1…--space-8`、`--density` 倍率、`--pad-chip-*`/`--gap-chip`/`--pad-btn-*`/`--pad-card-*`。
     **易用路径**：只想换字/密度/字距的主题，覆写 `--text-sm` / `--density` / `--pad-chip-y` 即一次性生效
     （比 SiYuan 逐项写 CSS 更省 → 超越）。
- **为什么用户能「任意改」且不用 `!important`**：`surface.css` 全局、选择器不带 `[data-v-x]`；
  用户主题/片段经 `useUserThemes`/`useSnippets` 在**运行时** `document.head.appendChild` 注入，
  天然晚于打包 CSS；与原规则 specificity 相同（0,1,0）时后加载者胜出 → `.ui-chip { ... }` 直接生效。
  这就是 SiYuan 式自由度。
- **定位器地图（locator map，主题作者照此 targeting）**：

  | 类 | 主题化目标 | 常用可覆写属性 |
  |---|---|---|
  | `.ui-chip` | 胶囊 / 标签按钮（主色 tint） | color / background / border / border-radius / padding / font-size / gap |
  | `.ui-badge` + `--success/--warning/--error/--neutral` | 状态徽章 | color / background / border-color / border-radius |
  | `.ui-card` | 实体卡片表面 | background / border / border-radius / padding |
  | `.ui-panel` | 次级面板表面 | background / border / padding |
  | `.ui-button` | 实心主色按钮 | background / color / border / padding / border-radius |
  | `.ui-toggle` | 开关行 | color / gap |
  | `.ui-bubble` | 对话气泡 | background / color / border / border-radius / padding |
  | `.ui-header` | 区域标题 | font-size / font-weight / color / gap |
  | `.ui-input` | 输入框 | background / color / border / border-radius / padding |

- **与 abolition 的关系（修正）**：abolition 仍正确——杀掉了**品牌耦合**的 `.encv-*` 词汇；
  但稳定语义钩子不该被一并杀掉。`.ui-*` 层是**正确形态**的钩子：语义/角色化、全局、专为被主题覆写而生。
  组件 scoped 样式只保留**结构/interaction**（布局、hover transform 等），**视觉表现一律交给 `.ui-*`
  全局类**，否则 scoped 的 `[data-v-x]` 会再次封死用户覆写。
- **已落地样例（可一键验证）**：
  - `MockPresetBar.vue` 的 chip 已挂 `ui-chip`，视觉从 scoped 块上提到 `surface.css`（证明 parity）。
  - `theme/snippets/surface-override.ts`：注册为可热开关片段 `surface-override`，
    内容 `.ui-chip { border-radius:2px; border-color:#ef4444; background:#fff7ed; color:#9a3412; font-weight:700; padding:6px 14px }`
    —— 开启即见胶囊变方橙粗体，**零组件改动、零 `!important`**。
- **下一步（待确认范围）**：把 §6.4 列的 ~13 个组件 + 其余对话组件，按「视觉上提 `.ui-*` / 字面量改消费令牌」
  分批 re-surface；并决定 `.ui-*` 词汇是否要进一步扩展（如 `.ui-list`/`.ui-divider`/`.ui-tooltip`）。

### 6.6 续28（03:2x）：#fff/#000 硬编码清零 + 主聊天图元 re-surface 落地

- **#fff/#000 硬编码清零**：基础令牌层 / snippet 主题文件 / `useTheme.ts` 调色板数据 /
  `plugin-simverse/.../game/*.ts`（Phaser canvas 不吃 CSS 变量）/ 一次性迁移脚本**不动**。
  仅组件 `.vue` 的 `<style>` 块内字面量路由到中性令牌：
  - `palette.css` 新增全局 `--color-white:#ffffff` / `--color-black:#000000`（默认纯白黑，零行为变化，主题可覆写）。
  - 临时脚本仅替换 `.vue` 的 `<style>` 块（避开 `<template>` 的 SVG `fill` 属性、`<script>` 字符串，
    那里 `var()` 不生效），`#fff`/`#ffffff`→`var(--color-white)`、`#000`/`#000000`→`var(--color-black)`，
    负向预查避免误伤 `#fff7ed`/`#000000aa` 等长 hex。**281 处 / 67 文件**已路由，门禁 8 PASS。
  - 余下 2 个 `.vue`（`TestReportHeader.vue`/`StepMiniBadge.vue`）里是 `#FFF8E1`/`#FFF3E0` 浅 tint，非纯白黑，正确保留。
- **主聊天图元 re-surface（证明契约可扩展，非仅 MockPresetBar）**：
  - `AgentTaskMessage.vue` 根面板 → 挂 `ui-panel`；scoped 的 `background/border/radius/padding` **上提**到
    全局 `.ui-panel`（默认对齐原外观：base-100 + base-200 + 0.5rem + 0.625/0.875，零视觉回退）。
    `.is-streaming` 状态覆写仍留 scoped（0,2,0 胜出）。
  - `FileReferenceChip.vue` 文件引用 chip → 挂 `ui-chip ui-chip--mono`；scoped 表面属性上提。
    新增 `.ui-chip--mono` 变体（等宽 + 4px 圆角 + 紧凑内距），保留代码类 chip 外观，主色 tint 仍由 `.ui-chip` 提供。
    `:hover`/`.fileRefChip_open` 状态覆写留 scoped。
  - 关键纪律：**组件 scoped 只留结构/interaction 与状态覆写，可主题化表面一律交给 `.ui-*`**，
    否则 scoped 的 `[data-v-x]`（0,2,0）会再次封死用户覆写。
- **验证**：`app_check_all`（noTests）→ 8 PASS / 0 FAIL（两次：硬编码清零、re-surface 各一次）。
- **续29（03:5x）：卡片/浮层 re-surface 落地 + `--overlay` 变体**
  - `ErrorStateCard.vue` 的 `.error-details` → 挂 `ui-card`；scoped 表面（背景/圆角/描边）**上提**到全局 `.ui-card`
    （retune 默认对齐原外观：base-100 + base-200 + 12px，零回退）；`overflow:hidden` 留 scoped（功能裁剪）。
  - `FileListCard.vue` / `FileContentCard.vue` 文件卡 → 挂 `ui-card--subtle`；scoped 表面（背景/描边/圆角/内距）上提
    到全局 `.ui-card--subtle`（新增变体，浅染 base-content 5%/18% + 8px 圆角 + 8/10 内距，对齐原外观零回退）。
  - `SlashMenu.vue` 浮层 → 挂 `ui-panel--overlay`（**新增变体**：背景用 `ion-background-color` + 25% 描边 + 12px 圆角 +
    投影 + 文本色，对齐原 fixed 弹层外观）；scoped 仅留定位（`position:fixed`/`left`/`bottom`/`transform`/`z-index` 等结构）+ 字号。
    定位属浮层专属、非通用表面，刻意**不**上提。
  - **纪律巩固**：不同表面家族（实色卡 / 浅染卡 / 浮层）用 `--变体` 区分，全局默认各对齐原组件，挂接零回退。
  - `AgentDebugPanel.vue` 暂缓：debug 专用（虚线 warning 边框 + warning 浅染），表面特殊且低可见度，留待评估
    （其内 `.agentDebugChip` 可后续路由到 `.ui-chip`，但强调/状态变体多，单独一轮做）。
  - **验证**：`app_check_all`（noTests）→ 8 PASS / 0 FAIL（续29 一次）。
- **下一轮待办（re-surface 候选，按主聊天可见度排序）**：
  1. 真正对话气泡组件（`UserMessageBubble.vue` 用户气泡 `#f0f1f3` 硬编码 + `@media dark` 切换）→ 先 token 化再挂 `.ui-bubble`/`.ui-bubble--user`（**需先解耦暗色硬编码**，本轮未做）
  2. `AgentDebugPanel.vue` 内 `.agentDebugChip` → `.ui-chip`（强调/状态变体单独处理）
  3. `AttachmentTray.vue` 内 chip → `.ui-chip`
  4. 输入框（搜索/命令）→ `.ui-input`；开关行 → `.ui-toggle`
  5. 词汇扩展评估：`.ui-list`/`.ui-divider`/`.ui-tooltip`/`.ui-scroll` 是否必要。
