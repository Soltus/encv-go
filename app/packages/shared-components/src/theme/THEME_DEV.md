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

### 6.4 决策（2026-07-16）：废除共享品牌类词汇，组件以令牌自包含样式 ~~（已被 §6.5 + §6.7 推翻）~~

> ⚠️ **本节决策已被推翻**：续27（§6.5）恢复稳定的语义全局类 `.ui-*`（SiYuan 式主题自由度），续32（§6.7）进一步用 **SCSS 程序化生成** `.ui-*` 词汇、`sass-embedded` 为**必需**依赖（原「devDep 随后可清理」作废）。共享「语义表面」类词汇 `.ui-*` 是正确形态（无品牌前缀、全局、专为被主题覆写而生），不是 §6.4 所批的「品牌化类词汇 `.encv-*`」。详见 §6.7。

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

### 6.7 续32（05:5x）：SCSS 生成表面词汇 + 分层主题（推翻 §6.4，sass-embedded 必需）

- **背景**：§6.4 把「品牌共享类词汇 `.encv-*`」连同「稳定语义全局类」一并废了，组件改 scoped + 字面量，
  结果主题只能换色板、且 scoped 的 `[data-v-x]`（0,2,0）封死用户覆写（比废除前还退步，见 §6.5）。
  §6.5 恢复 `.ui-*` 语义钩子，但初版 `surface.css` 是**手写重复 CSS** + 全局/scoped 特异性拉扯，维护性差、
  且 `sass:color` 完全没用上——违反「项目自身 + 用户主题都不妥协 / 用满 SCSS 高级特性」。
- **新架构（问题解决 + 用满 SCSS 高级特性）**：把 `surface.css` 重写为 SCSS 模块系统
  （`@use`+`@forward` 现代模块边界），并用满 maps / `@each` / `@function` / `@mixin` / `@if` / `sass:color`：
  - `_palette`：项目默认调色板具体值（sass:color 输入）。
  - `_config`：所有生成配置（tint 配方 / 变体 map / 档位）**单一事实来源**——加一个变体 = map 加一行，O(1)。
  - `_color-fns`：sass:color 封装，**编译期 `compiled-*` / 运行时 `runtime-*` 双路径**。
  - `_theme-tokens`：`@each` 遍历调色板，用 `sass:color` 在**构建期**算出 `--tint-*`（边框/背景/前景三件套）
    + `--color-*-hover/-active`（明暗派生）+ `--tint-base-content-soft-*`；另发全局交互令牌
    （`--elevation-*`/`--lift-hover`/`--press-scale`/`--ring-focus`）。
  - `_mixins`：`surface-fill` / `pill-base` / `surface-tint` / `neutral-surface`——**统一 tint 机制**
    （消灭旧版「mixin + neutral-surface 函数」两套 tint）；`pill-base` 不再重复声明 `gap`/`font-weight`，
    交给消费方单次声明（修复坏味道）。
  - `_surfaces`：`@each` 驱动生成全部 `.ui-*`，基态与变体**共用同一套 tint 算法**，无 `@if 100%` 魔法分支。
  - `_user-theme`：用户主题作者 API（`@mixin surface` + `@function blend`），让构建期用户主题也复用 SCSS
    颜色力（解决「用户主题能力不打折」的另一半）。
- **分层（项目自身 + 用户主题都不妥协）**：
  - 项目主题：【构建期】用真实色值 + `sass:color` 算出 `--tint-*` 具体颜色 → 项目自身拿到完整 SCSS 颜色力。
  - 用户主题：【运行时】注入覆写 `--color-*`；`--tint-*` 未定义时表面自动回退到 `runtime color-mix(var())`
    → 整套表面随用户调色板自动换肤，零折扣。派生令牌只定义在 `[data-theme=encv]`/`[data-theme=encv-dark]`，
    故默认（`:root` 无 attr）走运行时回退，与旧 `surface.css` 行为一致，**零视觉回归**。
  - ⚠️ **上述「官方主题编译期特权」已于续35（§6.9）被推翻**：主题 ≠ 调色板，官方与第三方必须同待遇。
    `_theme-palette.scss` 已删除，所有主题的 tint/hover/active 改由 `:root` 的 `color-mix(var(--color-*))`
    **运行时统一派生**（见 §6.9）。本节保留仅作溯源。
- **纪律（本仓强制规则 `.codebuddy/rules/文档同步.mdc`）**：本次改动同步修订本文档——§6.4 标注「已被推翻」，
  新增本节记录决策；门禁只验代码不验文档，文档正确性由本次提交保证。
- **验证**：`app_check_all`（含单测）→ 8 PASS / 0 FAIL；新增 `surface.test.ts` 编译期断言
  （`--tint-*` 为具体色非 var、hover/active 派生、交互令牌全局可用、表面消费 `--tint-*` 且保留运行时回退、
  基态/变体单一 map 驱动、暗色主题同编译）；vite build 重建 `frontend-deps.json`（sass-embedded 恢复为必需）。
- **re-surface 候选全部完成（续33）**：§6.6 列的 4 个候选均已挂 `.ui-*`，并 Reverse 了 §6.4 式内联 `color-mix`
  （「组件自包含、不依赖共享类词汇」已推翻）：
  - 用户气泡（`UserMessageBubble.vue`）→ `.ui-bubble.ui-bubble--user`（续31 已挂）。
  - 调试 chip（`AgentDebugPanel.vue` 的 `ui-chip ui-chip--neutral`）→ 续31 已挂。
  - 附件 tray（`AttachmentTray.vue` 的 `.attachmentItem`）→ 续33 挂 `.ui-card--subtle`（scoped 仅留布局/尺寸，
    表面随主题翻转；缩略图/移除按钮等子元素仍是组件细节）。
  - 输入 toggle（`CollapsedMessageToggle.vue`）→ 续33 挂 `.ui-chip` / `.ui-chip--neutral`
    （active||expanded=主色 chip，否则中性 chip；scoped 仅留布局 + 活跃脉冲动画，悬停/按下沿用 `.ui-chip:hover/:active`）。
- **词汇扩展评估（续33，结论：暂不新增）**：
  - `.ui-list` / `.ui-tooltip` / `.ui-scroll`：无可复用的通用消费者（popover 是 bespoke ion-popover，滚动容器布局特异性强）→ 不新增。
  - `.ui-divider`：唯一 divider 形态消费者 `ContextCompactionDivider.vue` 是 bespoke（两侧渐变线 + 居中文字），
    通用 `.ui-divider`(<hr> 形态) 套不上 → 不新增；该组件保持自包含。
  - 现有词汇（`.ui-chip`/`.ui-badge`/`.ui-card`/`.ui-panel`/`.ui-button`/`.ui-toggle`/`.ui-bubble`/`.ui-header`/`.ui-input`）
    已覆盖绝大多数形态；`FileReferenceChip.vue` 已用 `.ui-chip.ui-chip--mono`，`StatusBadge.vue`/`BlockHeader.vue`
    为 bespoke（改挂会改观感）列为**未来机会性 re-surface 候选**，不强制。
  - 决策纪律：新增词汇 = `_config` 的 map 加一行 + `_surfaces` 生成 + 本文件补契约；当前无强需求，避免早产抽象。

### 6.8 续34：官方主题扩展（rose / ocean / forest / midnight）（⚠️ 已被续35 §6.9 推翻）

- **⚠️ 本节决策已废除（续35 §6.9 推翻）**：「官方主题 = 编译期派生令牌特权」违背「主题 ≠ 调色板 /
  官方 == 第三方」原则，且导致第三方主题（sunset/mint）连 `--color-primary-hover` 都 undefined（按钮 hover 失效）。
  正确做法见 §6.9：所有主题统一走运行时 `color-mix(var())` 派生，官方与第三方零特权差异。
- **背景（溯源）**：§6.5/§6.7 把「表面词汇」升级为可被用户主题覆写的全局 `.ui-*`，但**官方主题本身**只有
  `encv`（亮紫）/ `encv-dark`（暗紫）两个。用户要求「实现多个官方主题」，于是续34 新增 4 个官方主题，
  并错误地把它们塞进编译期 `_theme-palette.scss`（赋予第三方拿不到的派生令牌特权）。续35 已纠正。
- **官方 vs 示例用户主题（分层，项目自身 + 用户主题都不妥协）**：
  - **官方（builtIn=true）**：调色板同时进 `_theme-palette.scss`（→ `_theme-tokens` 编译 `--tint-*`/
    `--color-*-hover/-active` 具体色）+ `palette.css`（基色 `[data-theme="<id>"]` 块）。
    故官方主题拿到**完整 SCSS 颜色力**，与 `encv`/`encv-dark` 完全平级。
  - **示例用户主题（sunset/mint）**：仅 `user-themes.css` 基色块 + 注册（非 builtIn），
    走运行时 `var()` 回退路径（证明用户主题能力不打折）；不进 `_theme-palette`，故无编译派生令牌。
- **官方主题清单（6 个）**：`encv`/`encv-dark`（紫）、`rose`（亮·玫瑰）、`ocean`（暗·海洋）、
  `forest`（亮·森林）、`midnight`（暗·午夜）。亮/暗由 `color-scheme` 声明；暗色基色加深、content 提亮。
- **新增官方主题的「四处同步」纪律（门禁只验代码不验文档，故此处显式记录）**：
  1. `_theme-palette.scss` 的 `$palettes` 加一项（12 色，键与 `palette.css` 一致）；
  2. `palette.css` 加 `[data-theme="<id>"]` 基色块（含 `--color-*` + `--radius-*`）；
  3. `useUserThemes.ts` 的 `USER_THEMES` 加 `{ id, nameKey, builtIn: true }`；
  4. `i18n/settings.ts` 的 zh-CN + en 各加 `settings.theme<Id>` 名。
  - **契约锁**：`surface.test.ts` 新增用例断言——每个 `builtIn` 主题都编译出 `[data-theme=id]{--tint-primary-bg;--color-primary-hover}`，
    防止「只注册不进 `_theme-palette`」导致该主题 silently 退回运行时回退（视觉降级、且违背官方主题契约）。
- **选择器/Hydration**：沿用 `useUserThemes` 既有机制——默认 `encv` 移除 `data-theme` 回落 `:root`，
  其它主题 `setAttribute('data-theme', id)`；偏好存 `localStorage`，模块加载即水合。无需改 AppearanceDetail.vue
  （它已 `v-for theme in themes` 渲染 `t(theme.nameKey)`，新主题自动出现在外观页列表）。
- **注意（既有行为，非本次引入）**：暗色官方主题的「暗」由 `color-scheme: dark` 驱动原生控件，
  Ionic 侧的 `body.dark` 类由 `useTheme` 的 bgColor 逻辑控制，与 `data-theme` 切换解耦——与 `encv-dark` 既有行为一致，无回归。
- **验证**：`app_check_all`（含单测）→ 见汇总；`surface.test.ts` 新增官方主题契约用例；i18n lint 两语言键齐备。

### 6.9 续35：主题 = 声明式数据资产，官方 == 第三方（推翻 §6.7/§6.8 的编译期特权）

> ⚠️ **本节「加载方式」已被续37 §6.10 推翻**：续35/续36 把官方主题放在
> `official-themes/<id>/theme.css` 并由 `theme-core.css` **构建期 `@import`** 烤进产物——
> 这仍剥夺了「可加载 / 可卸载 / 可分发」能力，且让官方主题无法携带自有资源（图片 / 字体 / 贴纸），
> 不是真主题（Hello Kitty 主题即需此）。续37 改为**运行时资产包 + themeLoader**（见 §6.10）。
> 本节「零特权 / 官方 == 第三方 / tint·hover·active 运行时派生」结论**仍成立**，仅加载机制作废。

- **用户原则（早已确立，本次才真正落实）**：**主题 ≠ 调色板**。主方案 `ENCV前端主题重构方案.md` §2.3 / §2.7.6
  已定义「主题 = 清单 + 令牌 CSS（+ 可选 JS 钩子），可加载、可卸载、可分发」，官方与第三方**同形态**。
  续26 用户亦点破「主题 ≠ 换色板」。续34 把官方主题硬塞进编译期 `_theme-palette.scss`，给官方主题发了
  第三方主题**永远拿不到**的「编译期派生令牌」特权——直接违背该原则，故本次推翻。
- **根因 bug（顺带暴露）**：§6.7 的 `--color-*-hover/-active` 只在 `[data-theme=encv/encv-dark]` 编译块里定义；
  第三方主题（sunset/mint）未进 `_theme-palette`，其 `--color-primary-hover` 为 `undefined`，
  `.ui-button:hover` 背景失效。官方特权与第三方缺陷同源。
- **新架构（官方 == 第三方，零特权）**：
  1. **删除 `_theme-palette.scss`**（编译期主题调色板）。不再有任何主题在 SCSS 编译期被烘焙。
  2. **tint / hover / active 全部运行时派生**：`_theme-tokens.scss` 在 `:root` 用
     `color-mix(in srgb, var(--color-*), white/black)` 对每个交互色派生 hover/active；
     表面 tint 沿用 `_mixins` 的 `var(--tint-*, color-mix(var(--color-*)))` 回退。
     因 `var(--color-*)` 在元素上解析为**当前主题**的语义色，单个 `:root` 定义即对所有主题
     （官方 + 第三方）生效 → 派生结果完全一致，**零特权差异**。
  3. **唯一区别 = 分发方式，不是颜色力**：`builtIn:true` 表示随包发布 + 外观页展示 + 不可卸载；
     官方主题资产在 `official-themes/<id>/theme.css`（随包），与第三方 `user-themes/<id>/theme.css`
     同形态、同加载方式，**但（续37 前）由 `theme-core.css` 用 `@import` 引入**——
     ⚠️ 此「构建期 @import」加载方式已被续37 §6.10 推翻，改为运行时资产包 + themeLoader。
- **契约锁（反方向，防复发）**：`surface.test.ts` 断言编译产物**不含**任何
  `[data-theme=X]{ --tint-* / --color-*-hover }` 声明（即无主题被编译期烘焙），且 `:root` 提供
  `color-mix(var(--color-primary))` 派生的 hover/active。防止「某官方主题又被塞回编译期」。
- **新增官方主题的正确做法（四处同步，续36 起必须是「主题」而非「换色」）**：
  1. 建 `official-themes/<id>/theme.css`：定义**完整设计语言**——除 `--color-*` 色相外，
     必须覆写形状（`--radius-*`）、密度（`--density`）、立体感（`--elevation-*`/`--lift-hover`/`--press-scale`）、
     焦点环（`--ring-focus`）、动效（`--motion-*`）等令牌，让主题有独立「设计语言」而不仅是换色；
  2. ~~`theme-core.css` `@import "../theme/official-themes/<id>/theme.css";`~~
     ⚠️ 已废除（续37）：主题**不再**编译进 theme-core.css，改为运行时资产包。
  3. `useUserThemes.ts` 的 `USER_THEMES` 加 `{ id, nameKey, builtIn: true }`；
  4. `i18n/settings.ts` 的 zh-CN + en 加 `settings.theme<Id>` 名。
  ❌ **不再**需要、也**不应**在 `_theme-palette.scss` 登记（该文件已删除）；
  ❌ **不应**把官方主题塞进 `palette.css`（那是 encv 品牌调色板/默认基态，仅留 encv/encv-dark）。
- **验证**：`app_check_all`（含单测）→ 见汇总；官方/第三方主题均经同一运行时派生，按钮 hover 对第三方亦生效。

### 6.9.1 续36：换色 ≠ 主题（官方主题重写为设计语言资产）（⚠️ 加载方式已被续37 §6.10 推翻）

- **⚠️ 本节「加载方式」已被续37 §6.10 推翻**：续36 把官方主题资产放在
  `official-themes/<id>/theme.css` 并由 `theme-core.css` 构建期 `@import` 加载——
  这仍把主题烤进产物，不是真「可加载 / 可卸载 / 可分发」资产包。续37 改为运行时加载（见 §6.10）。
  本节「设计语言（形状/密度/立体感）而非换色」的结论仍成立并被保留。
- **用户再次强调**：「换色不是主题，能懂吗」。续35 虽消除了编译期特权，但官方主题仍只是
  「换色」——6 个主题半径全相同（`1rem/0.5rem/1rem`），表面层还有**字面量圆角**
  （`8px`/`12px`/`9999px`/`4px`）导致形状语言根本无法被主题改写。这才是「换色」的本质。
- **本次修正（让主题真成主题）**：
  1. **迁出 `palette.css`**：rose/ocean/forest/midnight 从 encv 品牌调色板移到
     `official-themes/<id>/theme.css` 独立主题资产（与 `user-themes/<id>/theme.css` 同形态、
     同 `@import` 加载），`palette.css` 仅留 encv/encv-dark 默认基态。
  2. **表面层字面量圆角 token 化**（仍成立）：`--radius-box` / `--radius-pill` / `--radius-mono`
     基线定义在 `tokens.css :root`，`_surfaces.scss` 改消费这些令牌（卡片/面板/胶囊/mono）。
     主题从此可整体改写「形状语言」，不再被硬编码卡死。
  3. **每个官方主题定义独立设计语言**（不仅是色相）：
     - `rose`：更圆润（1.5rem）+ 略宽松（density 1.05）+ 柔和彩色阴影 + 明显抬升 + 回弹动效；
     - `ocean`：科技暗色 + 利落小圆角（0.75rem）+ 紧凑（0.9）+ 青色辉光 + 敏捷动效；
     - `forest`：圆角盒（1.25rem）+ 宽松（1.12）+ 柔和绿影 + 缓入缓出；
     - `midnight`：极简暗色 + 锐利小圆角（0.375rem，胶囊也近方）+ 近扁平（无抬升）+ 细锐环 + 沉稳。
- **契约锁（防复发，反向）**：`official-themes.test.ts` 断言 4 个官方主题资产**各自定义不同的**
  `--radius-*` / `--density` / `--elevation-*`，且**不仅**含 `--color-*`（防止「又退化成换色」）；
  并断言 `palette.css` 不再含这 4 个官方主题（证明没回塞品牌调色板）。`surface.test.ts` 既有契约
  （无编译期烘焙）仍有效——这些主题资产是纯 CSS 数据，不进 `surface.scss` 编译。

### 6.10 续37：主题 = 运行时可加载的「文件夹资产包」，官方 == 第三方（附性能指标与优化）

- **用户点破的根因（第四次纠正）**：前几版把主题实现成「填 `--color-*` 令牌槽」（换色）→
  退一步成「填设计语言令牌」（色+形状+密度+立体感），但**本质仍是编译进 `theme-core.css` 的 CSS 块**：
  1. 主题被构建期 `@import` 烤进产物 → 用户**无法加载 / 卸载 / 分发**一个主题（违背方案文档 §2.3）；
  2. 主题不能携带自有资源（图片 / 字体 / 贴纸）→ **Hello Kitty 主题根本做不了**（只能换色）。
  真正的主题（思源集市式）= **一份能运行时加载、能用自己的选择器覆写真实组件、能 url() 自有资源 /
  字体、能 theme.js 钩子装饰的独立样式表**。令牌只是「懒人快捷层」，不是主题本身。
- **新架构（落实方案文档 §2.3 / 对比表「theme.json + theme.css + 可选 theme.js，可加载/卸载/分发」）**：
  1. **主题 = 文件夹资产包**：`public/themes/<id>/theme.json` + `theme.css` + 可选 `theme.js` / `assets/`。
     官方 6 个（encv / encv-dark / rose / ocean / forest / midnight）+ 示例第三方（sunset / mint）**同形态**。
  2. **运行时加载，不再编译进产物**：`theme-core.css` **不再 `@import` 任何主题**；`palette.css` 仅留
     `:root` 的 encv 默认基态（无 `data-theme` 块）。主题由 `themeLoader`（`theme/themeLoader.ts`）
     在运行时 `document.createElement('link')` 注入 `<link rel="stylesheet">` 并切 `[data-theme]`。
  3. **官方 == 第三方，唯一区别 = 预装（builtIn）**：
     - `builtIn:true`（官方）：随包发布于 `public/themes/`、启动期**预加载**、**不可卸载**；
     - `builtIn:false`（第三方）：来自用户空间 / 集市（远程 URL），**可卸载**、LRU 回收。
     两者走**同一套 `themeLoader` 注入/切换代码**，颜色力与加载机制零差异。
  4. **资源包能力（真主题的证明）**：`rose/theme.css` 用 `url("data:image/svg+xml,...")` 给 `body` 加
     极淡点纹，演示「主题能自带装饰资源」。Hello Kitty 主题即靠此贴贴纸、换背景图案、`@font-face` 换字体、
     `::before` 加装饰——全部无需编译期支持。**新增官方/第三方主题 = 复制 `public/themes/<id>/` 文件夹**。
- **themeLoader 性能指标（getThemePerf / window.__encvThemePerf）**：
  - 每主题 `loadMs`（注入→onload）、`cached`（去重命中）、`loadedAt`；
  - 全局 `lastSwitchMs`（切换耗时）、`cacheHits`（去重命中次数）、`loaded`（常驻 link 数）、
    `active`（当前主题）、`firstPaintMs`（首帧）、`thirdPartyLoaded`（LRU 下第三方常驻数）。
- **themeLoader 优化（对应指标）**：
  1. **预加载**：`preloadOfficial()` 启动期并行注入官方主题 `<link>`，切换零等待 → 压低 `lastSwitchMs`；
  2. **去重缓存**：同主题只注入一次，重复激活命中缓存（`cacheHits++`，`loadMs=0`）→ 切回已加载主题零成本；
  3. **LRU 卸载**：第三方超 `MAX_THIRD_PARTY_LINKS=4` 自动回收 `<link>` + `unmount` → 控制内存；
  4. **防 FOUC**：先 `await` CSS `onload` 再置 `[data-theme]`，确保样式到位才生效；默认主题回落 `:root`；
  5. **安全网**：个别环境 `link` 事件不触发时 2s 超时兜底，避免永久挂起。
- **契约锁（防复发，反向）**：`official-themes.test.ts` 现断言——
  - 8 个主题都是 `themes/<id>/{theme.json,theme.css}` 文件夹资产；官方 `builtIn=true`、第三方 `false`；
  - 官方主题定义**互不相同**的 `--radius-box` / `--density`（防止退化成换色）；
  - 所有主题**不含** `--color-primary-hover` / `--tint-*`（无编译期特权回潮）；
  - `rose` 含 `url()`（资源包能力）；
  - `palette.css` **不含**任何 `[data-theme]` 块（主题已迁出，零编译特权）；
  - `themeLoader`：注入 `<link>`、默认回落 `:root`、去重缓存命中、`unloadTheme` 仅卸第三方、预加载后切回零成本。
- **新增主题（官方 / 第三方完全一致）**：
  1. `public/themes/<id>/theme.json`（id 匹配、builtIn）+ `theme.css`（可含 url() 资源 / @font-face / 装饰）；
  2. `useUserThemes.ts` 的 `USER_THEMES` 加 `{ id, nameKey, builtIn }`（第三方可带 `url` 远程地址）；
  3. `i18n/settings.ts` 加 `settings.theme<Id>` 名。
  ❌ **不再** `@import` 进 `theme-core.css`（那会剥夺「可加载/卸载/分发」并造特权，正是续37 推翻的）。
- **验证**：`app_check_all`（含单测）→ 见汇总；`official-themes.test.ts` 覆盖资源包 + loader 全契约；
  i18n lint 两语言键齐备。

### 6.11 续38：主题系统对用户真正可用（可视预览 + 集市安装 + URL 分发 + 管理 + 性能可见）

> 前几版（到续37）只把**引擎**做对了：运行时资产包 + loader + 指标都在，但**用户面**仍是
> 「一行纯文本列表 + 勾选」，既看不出主题的「设计语言」，也无法安装/卸载/分发——「可加载/可卸载/
> 可分发」对终端用户只是内部能力，不是体验。用户第五次纠正「远远不够」点破此缺口。

- **外观页（AppearanceDetail.vue）主题区重做为三块**：
  1. **可视实时预览网格**：每个主题一张卡片，卡片上 `:data-theme="<id>"` 让该主题 CSS 作用域落到卡片，
     **直接渲染真实语义组件**（`.ui-bubble--user` 气泡 / `.ui-chip` 胶囊 / `.ui-panel` 面板 / 主·辅·强调·底色色块），
     因此**主题的「组件覆写」也原样可见**——neon 的发光描边、paper 的衬线字体、各主题的 `--radius-box` 圆角差异
     都能在预览里一眼看出，而非仅靠名字。
     ⚠️ 修订（续39）：初版预览用的是占位 `.pv-bubble`/`.pv-chip`（只显示令牌颜色，不显示主题对组件的覆写），
     已改为真实语义组件类（占位样式删除），每个卡片自身 `data-theme` 作用域内继承主题，覆写即生效。
     卡片文字统一用「该主题自身」令牌（`color: var(--color-base-content)`），保证暗/亮卡片上都可读。切回即 applyTheme。
  2. **主题集市（Bazaar）**：`BAZAAR` 注册表列出可安装条目（示例 `neon` / `paper`，本地资源包），
     一键 `installFromBazaar` → 进用户空间 → 出现在网格。证明「集市式分发」。
  3. **从链接安装（可分发）**：粘贴远程 `theme.css` URL → `installTheme({ id, url })` 注册为第三方，
     loader 按远程 URL 注入 `<link>`。这是「主题可跨设备/跨用户分发」的可运行证据。
  4. **管理**：每个非官方主题显示卸载按钮 → `unloadTheme` 移除 `<link>` + 删持久化；官方 `builtIn` 不可卸载。
  5. **性能可见**：外观页底部展示 `themeLoader` 实时指标（切换耗时 `lastSwitchMs` / 去重命中 `cacheHits`
     / 已加载 `loaded`），让「性能指标与优化」从代码承诺变成用户可观测项。
- **useUserThemes 新增运行时能力**：
  - `installTheme(meta)` / `installFromBazaar(entry)` / `uninstallTheme(id)`：运行期增删主题；
  - 已安装主题持久化到 `localStorage["encv-installed-themes"]`，`initUserThemes()` 启动期水合；
  - `ensureThemeLoaded(id)`：安装/进页面时预热 `<link>`，保证可视预览即带正确样式；
  - `themePerf`（响应式）：暴露 `getThemePerf()` 快照，apply/install/uninstall 后刷新。
- **契约锁（防复发，反向）**：`official-themes.test.ts` 新增 `useUserThemes` 描述块断言——
  - 集市条目可安装并进入 `allThemes`、持久化到 localStorage；
  - 卸载第三方主题同步移除注册 + 删持久化 + loader 卸载 `<link>`；
  - 从 URL 安装的主题按远程地址注入 `<link>`（可分发证据）；
  - 官方主题 `builtIn` 为真、`uninstall` 静默忽略（不可卸载）。
- **新增主题（用户侧，完全自服务）**：
  - 集市新增条目：`BAZAAR` 加 `{ id, nameKey, descKey }`（资源在 `public/themes/<id>/`）；
  - 或用户自己从 URL 装任意远程主题——**无需改任何编译配置 / 无需发版**。
- **验证**：`app_check_all`（含单测）→ 9 PASS；i18n lint 两语言键齐备（bazaar/install/perf 等）。

### 6.12 续40：Ionic 半透明层跟随任意主题（换肤不再是「半成品」）

> 续38/续39 把用户面与预览 fidelity 做对了，但核验桥接时发现**真正的换肤缺口**：
> `bridge.css` 的实体色用 `var(--color-*)` 间接（任意主题跟随），但 **`-rgb` 三元组硬编码成
> encv 紫**（lines 25/33/41/…）。Ionic 的半透明层（`rgba(var(--ion-color-primary-rgb), a)`：
> 进度条 / 焦点环 / overlay / ripple / 选中态）因此【只认 encv 紫】——切到 rose/ocean/neon
> 后实体色换了、半透明层却残留紫，换肤「半成品」。

- **修复（themeLoader.syncIonicRgb）**：每次 `activateTheme` 置位 `[data-theme]` 后，读取当前主题
  已落到 `<html>` 的 `--color-*`，用 `hexToRgbTriple()` 派生并写回 `--ion-color-*-rgb`
  （primary/secondary/tertiary=accent/success/warning/danger/medium/base-100=light）+
  `--ion-color-primary-contrast-rgb` / `--ion-background-color-rgb` / `--ion-text-color-rgb`。
  主题无关：官方 / 第三方 / 远程 URL 主题全部生效；写在内联 `<html>` 上（特异性高于桥接 `:root`），
  覆盖硬编码兜底值。`bridge.css` 的硬编码 -rgb 降级为「JS 未跑时的兜底」（注释同步更新）。
- **价值**：切主题后整个 Ionic App（含半透明层、进度、焦点、overlay）真正统一换肤，不再紫绿混杂。
- **契约锁（反向）**：`official-themes.test.ts` 新增 `themeLoader.syncIonicRgb` 描述块断言——
  - 当前 `--color-primary=#e11d48`(rose) → `--ion-color-primary-rgb` 派生为 `225, 29, 72`、
    contrast/background/text -rgb 同步；
  - 即使 bridge.css 硬编码 `--ion-color-primary-rgb: 139, 92, 246`(encv 紫) 已存在，
    syncIonicRgb 仍覆盖为 neon 绿 `57, 255, 20`，证明任意主题不会残留紫色半透明层。
- **验证**：`app_check_all` → **9 PASS / 0 FAIL**（Biome/各包 typecheck/i18n lint/单测/vite build 全绿）。

### 6.13 续41：切换主题卸载上一主题的 JS 装饰（不残留 / 不丢挂载）

> 续40 把颜色换肤做对了，但核验「真主题能力（theme.js 运行时装饰）」时发现**切换生命周期 bug**：
> `activateTheme` 调 `ensureJs` 在激活时 `mount()` 一次，但【切走时不调上一主题的 `unmount()`】
> （只有 `unloadTheme`/LRU 回收才调）。于是 kitty 这类挂 `<body>` 的装饰（固定角标贴纸）
> 切到 rose 后【永久残留】；更糟的是 `jsCache` 仍持有 kitty，切回时 `ensureJs` 的
> `jsCache.has` 守卫会使它【不再重新 mount】——不刷新就再也拿不回装饰。

- **修复（activateTheme 内）**：置新主题前，若 `prevId` 存在且 ≠ 新主题，取其 `jsCache` 中的
  `unmount` 并调用、随后 `jsCache.delete(prevId)`。这样：① 切走即卸载上一主题装饰（不残留/堆积）；
  ② 删掉缓存使【切回时能重新 mount】（不丢挂载能力）。同主题重复激活走 `prevId === src.id` 守卫，
  幂等（不重复挂载、不误卸载）。
- **测试桩（__stubJsModule）**：为在 jsdom 下对「动态 import」做确定性单测，新增 `__stubJsModule(id, mod)`
  注入假模块、`ensureJs` 优先用桩、`resetThemeLoaderForTest` 一并清桩，避免跨用例污染。
- **契约锁**：`official-themes.test.ts` 新增 `themeLoader 切换生命周期` 描述块断言——
  - 切到 kitty 装饰存在；切到无 JS 的 rose 后装饰被卸载；
  - 离开再切回 kitty → 装饰重新挂载（证明 jsCache 清除生效）；
  - 重复激活同一主题 → 仅挂载一次（幂等）。
- **验证**：`app_check_all` → **9 PASS / 0 FAIL**（Biome 用 `pnpm exec biome check --write` 修，非 app_format）。

### 6.14 续42：theme.json 成为运行时活清单（清单驱动分发，非死数据）

> 续37–41 把「运行时资产包 + loader + 装饰生命周期」都做对了，但核验「可分发」时发现：
> 每个主题都随包放着 `theme.json`（架构文档也称主题为「theme.json + theme.css 文件夹资产包」），
> **但运行时从不 fetch/parse 它 —— 是彻头彻尾的死数据**。后果：① 元信息（名字/作者/版本/是否带 js/资源）
> 全在 TS 注册表 `USER_THEMES` 里硬编码，与磁盘清单**两处真相、可静默漂移**；② 从 URL 分发主题时
> 无法发现主题自身元信息，必须让用户手填 id + **直连 theme.css**（不是「指向文件夹即可安装」的真分发）。

- **修复（让清单成为活契约）**：
  1. `themeLoader.ts` 新增 `ThemeManifest` 接口 + `fetchThemeManifest(folderOrJsonUrl)`：接受主题
     **文件夹 URL**（如 `/themes/rose` 或 `https://x/themes/rose`）或直接的 `theme.json` URL，
     fetch 并解析清单，把相对 `css`/`js` 按清单所在目录**解析为绝对 URL**（绝对 / data: 原样保留），
     校验 `id`、HTTP 失败即抛错。这是对齐 Obsidian / 思源集市「指向清单即可安装」的分发范式。
  2. `useUserThemes.ts`：`UserThemeMeta` 增 `jsUrl`（清单声明的 theme.js 绝对 URL），`toSource` 优先用它
     （否则回退本地 `js:true` 推导）；新增 `installThemeFromUrl(folderOrJsonUrl)`（async）——读清单自动
     发现 id/名字/CSS/JS 并安装，不再要用户手填 id 或直连 css。安装经 `ThemeStorage` 端口让【后端】把远程
     主题拉取到【本地同一目录】（详见 §6.15），前端从同源 `/themes/<id>/` 加载，本地优先、不热链 CDN。
  3. `AppearanceDetail.vue` 的「从链接安装」改为：非 `.css` 结尾走 `installThemeFromUrl`（清单路径），
     `.css` 结尾保留旧直连回退；加安装中/错误态（`installing` i18n 键 + 错误 `ion-note`）。
     placeholder 文案改为「主题文件夹 / theme.json 地址」。
- **契约锁（防复发）**：`official-themes.test.ts` 新增三块——
  - **防漂移**：每个本地主题 `theme.json` 的 `builtIn`/`js` 与 TS 注册表 `USER_THEMES` 一致；
  - `fetchThemeManifest`：文件夹 URL / theme.json URL / 绝对 css 直连 / 缺 id / HTTP 404 各分支（mock fetch）；
  - `installThemeFromUrl`：读远程清单派生 id/名字，经 `ThemeStorage` 端口拉取到后端本地同一目录，
    同源 `/themes/<id>/` 注入 `<link>`（本地优先，非热链远程 CDN）。
- **说明**：内置主题随包发布，其 TS 注册表仍是启动期零 fetch 的权威（避免 N 次启动请求），
  清单一致性由防漂移测试保证；**远程/集市分发**才走 `fetchThemeManifest` 活清单路径。
- **验证**：`app_check_all` → **9 PASS / 0 FAIL**（Biome 用 `pnpm exec biome check --write` 修，非 app_format）。

### 6.15 续43（修订）：本地优先 / 云拉取落本地同一目录 —— 框架端口化，存储后端是 Go

> 续42 的 `installThemeFromUrl` 初版只把远程 css 当 `<link>` **热链**（运行时依赖 CDN、断网即失效、与内置
> 「两套来源」）。用户要求：**本地优先，云拉取也下载到本地同一目录**。初稿曾错用 `@capacitor/filesystem`
> 把主题字节存进设备文件系统——但本工程后端是 **Go（encv-go）**，主题「本地同一目录」=
> **Go 后端托管的同源 `/themes/<id>/`**，不是设备文件系统；且「字节下载到哪、怎么存」是【应用层职责】，
> 不应耦合进 `shared-components` 框架（违背「避免框架与应用耦合」）。故推倒重来如下。

- **架构（框架 ↔ 后端解耦）**：
  - `theme/themeStorage.ts`（框架）：只定义【端口（抽象）`ThemeStorage`】——
    `pullToLocal(req)` / `removeLocal(id)`，以及 `setThemeStorage` / `getThemeStorage` 注入点。
    默认 `sameOriginThemeStorage` 不下载、不存字节（单测 / 后端已就绪场景），保证框架零后端依赖、零 Capacitor 依赖。
  - `encv-mobile/src/stores/registerSharedThemeStorage.ts`（应用）：注入 `ThemeStorage` 的
    **Go 后端适配器**——`pullToLocal` → `POST {getAgentApiBase()}/api/themes/pull`、
    `removeLocal` → `DELETE {getAgentApiBase()}/api/themes/<id>`。沿用 `getAgentApiBase()`
    三态拼装（dev 网关 `/agent-api`、native 相对路径、web SPA 绝对 URL），与项目既有的
    `setApiProxy` / `getApiProxy` 解耦范式完全一致。`main.ts` 启动期调用 `registerSharedThemeStorage()`。
- **本地优先语义（Go 后端视角，前端永远从同源 `/themes/<id>/` 加载）**：
  - **存储根 = 数据目录，绝非 servingDir**：用户安装的远程主题落盘到 `themeDataPath()` 派生目录
    （`internal/server` 的 `themeDataPath()`，与 `kernelDataPath` / `simverseDataPath` 同脉络，
    复用 `mountRegistryDataPath` 的目录派生逻辑，见 `server.go`）：
    - Android：`<ENCV_APP_FILES_DIR>/.encv/themes`（**app 私有 files 目录**——可写、不污染媒体视图；
      servingDir 在 Android 上是打包的私有只读资产，写不进去也不该混）。
    - 桌面：Linux/Mac `$XDG_DATA_HOME/encv(-dev)/themes`、Windows `%LOCALAPPDATA%\encv(-dev)\themes`。
    - 优先级：`ENCV_THEMES_DIR`（显式）> 派生默认值。
    - **严禁**把用户数据写进 `servingDir`（静态 web 根）—— 用户于 2026-07-16 明确纠正过此错误初稿。
  - 内置主题：随包发布在 `servingDir/themes/<id>/`，由 `/themes/*` 路由回退服务。
  - 云拉取主题：前端安装时 `getThemeStorage().pullToLocal({ id, sourceUrl, manifest })` →
    **Go 后端把远程主题下载到【数据目录】`themes/<id>/`**（与内置同形态、同加载机制，但物理位置不同）。
    前端不热链远程 CDN —— 同源即本地优先、离线可用。
  - `GET /themes/*` 静态路由（local-first）：先查数据目录（用户主题），**再回退 `servingDir/themes`**（内置），
    两侧均做路径穿越防护；用户主题优先覆盖同名内置。
  - `toSource` 据此：已落地（`meta.local`）的主题一律从 `/themes/<id>/theme.css` 解析；`js` 同理解析
    `/themes/<id>/theme.js`；远程 `jsUrl` 仅作非本地兜底的遗留兼容。
- **裸 .css 直链回退**：`installThemeFromCssLink(cssUrl)` 走 `pullToLocal({ cssOnly: true })`，
  同样拉取到后端本地同一目录，本地优先；id 由文件名推导（无清单元信息）。
- **契约锁（续42 已建 + 本轮修正）**：`installThemeFromUrl` 单测断言改为——清单派生 id/名字后，
  注入的 `<link>` 为**同源 `/themes/<id>/theme.css`**（本地优先），而非远程 CDN URL。
- **废弃说明**：续43 初稿 `themeStore.ts`（Capacitor + localStorage 存主题字节 + data: URL 注入）**已废除**，
  被本节的 `themeStorage.ts`【端口】方案取代——主题字节存哪、怎么存，全归 Go 后端，框架不持有。
- **验证**：`app_check_all` → **9 PASS / 0 FAIL**（见上）。

### 6.16 续44/45：gsap 赋能主题 —— GSAP 运行时读取主题的 --motion-* 令牌

> 续44/45（§2.5.10 / §2.5.11 / §2.6）把动效防腐层 + 指令层接入真实 UI，但核验「主题 ≠ 换色」时发现
> **最后一块短板**：`theme/tokens.css` 早已定义 `--motion-dur-*` / `--motion-stagger` / `--motion-intensity`
> （见 §2.6），但此前**只有纯 CSS 动画**消费它们；GSAP（JS 动画引擎）侧的时长 / stagger / 强度是写死的
> JS 常量（见 `motion/tokens.ts` 旧版）。于是**主题 / 用户片段覆写 `--motion-*` 令牌对 GSAP 动画毫无影响
> —— 主题能换色、换形、换密度，却换不了动效节奏**，与「主题 ≠ 换色」原则相悖。

- **修复（让 GSAP 也读主题令牌，gsap 赋能主题）**：新增 `motion/theme-read.ts` 运行时读取层——
  1. 在运行时读取 `documentElement` 上 `--motion-*` 的**计算值**（带 250ms 节流缓存，按变量名缓存，
     避免磁性跟手等高频路径每帧 `getComputedStyle` 触发回流风暴；主题切换最多 250ms 内反映）；
  2. `motion/tokens.ts` 的 `DUR.fast/base/slow` getter、`getStagger()`，`motion/guard.ts` 的
     `intensity` 全部改为经 `readMotionSeconds()` / `readMotionNumber()` 实时读令牌，读不到 / 非法时
     回退常量默认（`DUR_FALLBACK` = 0.16/0.32/0.52s、`STAGGER` = 0.04s、`intensity` = vivid?1.3:1）。
  3. 导出 `invalidateMotionTokenCache()`：主题切换（切 `data-theme` / 注入用户片段）后可显式调用，
     立即让 GSAP 读到新令牌（否则最多 250ms 后自动生效）。
- **结果**：覆写 `--motion-*` 令牌**同时作用于纯 CSS 动画（直接 `var()`）与 GSAP 动画（运行时读取）**，
  二者表现一致，消费方零改动——**主题真能定制动效节奏**。
- **主题作者公开 API（在主题 `theme.css` 覆写即可，无需改代码）**：
  - `--motion-dur-fast / --motion-dur-base / --motion-dur-slow`：时长（支持 `ms` / `s`）；
  - `--motion-stagger`：列表 / 级联入场的基础节奏（会再乘 `intensity`）；
  - `--motion-intensity`：强度（0.8 克制 / 1 默认 / 1.5 张扬；`encv-vivid` / `encv-p3` 根类自动 1.3；
    `prefers-reduced-motion` 下 `tokens.css` 置 0 → `guard` 直接关动画落终态）；
  - `--motion-ease-*`：缓动（目前仅供纯 CSS 动画；GSAP 缓动走语义键 + 引擎 `EASE_MAP`，见 tokens.ts）。
- **已落地样例（证明主题已真能定制动效）**：
  - `ocean` 覆写 `--motion-dur-fast: 140ms`（比默认 160ms 更敏捷）；
  - `midnight` 覆写 `--motion-dur-fast: 200ms`（沉稳、略慢）；
  - `rose` / `kitty` 覆写 `--motion-ease-back`（明显回弹）；`forest` 覆写 `--motion-ease-out`（缓入缓出柔）。
  这些令牌现在对 GSAP 与 CSS 动画**同时生效**。
- **契约锁（防复发，反向）**：`motion-guard.test.ts` 新增 `motion tokens — gsap 赋能主题` 描述块断言——
  - `DUR.fast/base/slow` 从 `--motion-dur-*`（ms/s 皆可）实时读取，未覆写回退默认；
  - `getStagger()` 从 `--motion-stagger` 读取，缺失回退默认；
  - `intensity` 从 `--motion-intensity` 读取（主题可克制 0.8 / 张扬 1.5）；
  - 令牌 `<=0` 但被 `setMotionDisabled(false)` 强制开启时，`intensity` 钳制为正（避免「开启却零幅度」矛盾）。
- **纪律（本仓强制规则 `.codebuddy/rules/文档同步.mdc`）**：本次补本文档 §6.16；门禁只验代码不验文档，
  文档正确性由本次提交保证。
- **验证**：`app_check_all` → **9 PASS / 0 FAIL**（含 `motion-guard.test.ts` 该描述块）；`app_check_all`
  经 MCP 通道（`app_check_all`）跑，未碰裸终端。

### 6.17 续48：臻彩显示（Vivid / P3 宽色域）真正生效（修复三处历史失效）

> 用户纠正：外观里的「臻彩显示」此前**形同虚设**——宣传的提色 / 宽色域效果从未真实出现。
> 经「先红后绿」复现（见 `encv-mobile/src/motion/__tests__/vivid.test.ts`，修复前 5/6 RED）→ 定位三处根因。

- **根因（诊断）**：
  1. **默认恒等滤镜**：旧 `syncVividFilter` 把 intensity 直接当 `--encv-vivid-filter` 拼成
     `contrast(intensity/100) saturate(...)`，默认 intensity=100 → `contrast(1) saturate(1)` = **恒等**，
     肉眼零变化。
  2. **`.encv-vivid` 根类从未添加**：`applyVividMode` 只切 `vividMode.value` 状态，**从没**给
     `document.documentElement` 加 `.encv-vivid` 类 → `motion/guard.ts` / `tokens.css` 里
     `encv-vivid` 根类驱动的动效强度 1.3 boost **永不触发**（guard/tokens 侧已就绪，业务侧没接上）。
  3. **P3 写无效属性**：旧 `applyP3Mode` 写 `--encv-color-gamut: p3` 之类的「属性」，
     `color-gamut` 是**只读媒体特性（media feature）**，**不是作者可设的 CSS 属性** → 写出来无任何效果，
     宽色域从未生效。
  4. **滤镜选择器命中不到页面（2026-07-17 二次复现）**：`vivid.css` 滤镜规则写 `.encv-vivid ion-page`，
     但 Ionic Vue 在 CE 注册模式（`registerIonicComponents`）下把 `<ion-page>` 渲染为 `<div class="ion-page">`
     （**TAG 是 div，不是 ion-page**）。真实浏览器复现：`getComputedStyle(.ion-page).filter` **恒为 `none`**——
     即便 `.encv-vivid` 类与 `--encv-vivid-amount`（gsap 确实写入了 0.33）都正确，滤镜也从不生效。
  5. **P3 交换被内联压过（2026-07-17 二次复现）**：`@media (color-gamut: p3) :root.encv-p3 { --color-primary:
     var(--color-primary-p3, var(--color-primary)) }` 是普通作者规则，而 `applyColor` 把 `--color-primary`
     以**内联**写在 `:root` 上；内联优先级高于作者规则（无 `!important`）→ P3 屏下品牌主色恒为内联 srgb 值，
     宽色域从不生效。真实浏览器复现：`withoutImportant` 时 computed `--color-primary` = 内联值；且原回退写
     `var(--color-primary)` 是**自引用**（无效声明→令牌丢失）。仅 `--color-primary` 被内联（secondary/accent 未内联，
     故不受影响），但 primary 是主色，视觉上 P3「没效果」。
  6. **刷新后 vivid 失效（2026-07-17 二次复现）**：`initTheme` 读 localStorage 的 vivid 偏好后只置
     `vividMode.value`，**从未调用 `applyVividMode`** → 刷新后 `.encv-vivid` 根类丢失、滤镜不挂（须手动再开关一次才恢复）。
- **修复（让臻彩真生效，零行为回归）**：
  1. `vividAmountFromIntensity(intensity)`：50→0（近关）、100→**0.34**（默认即可见）、200→1（最浓）。
     不再恒等。
  2. `applyVividMode('on')` 现在给根元素加 `.encv-vivid` 类（'off' 移除）；`syncVividFilter` 用
     `MotionEngine`（gsap）把 `--encv-vivid-amount` 平滑过渡到目标值（reduced-motion 下直接落终态，
     走 `getMotionProfile()` 闸门——ACL 解耦，换 anime.js 下游零改动）。
  3. P3 改为「`.encv-p3` 根类 + `@media (color-gamut: p3)`」控制：`applyP3Mode` 只加 / 去 `.encv-p3` 类，
     不再写无效属性；`vivid.css` 在真 P3 屏下把 `--color-primary/-secondary/-accent` 替换成对应
     `-p3` 令牌（`color(display-p3 ...)`）。
  4. `applyColor` 内用 `hexToP3Token(color)`（任意有效 hex → `color(display-p3 r g b)`，
     srgb 归一化值直接塞入更宽基色 = 更艳）写 `--color-primary-p3`；**不再写死内置 7 色**——
     自定义取色 / 远程主题色也全自动派生。非法 hex `removeProperty` → 回退 srgb，不报错。
     `palette.css` 补 `--color-secondary-p3` / `--color-accent-p3` 默认值。
  5. `App.vue` 删除失效的 `ion-page { filter: var(--encv-vivid-filter, none) }` 与整个
     `@media (color-gamut: p3)` / `.encv-force-p3` 无效块（改注释说明已迁 `vivid.css`）。
    6. `theme-core.css` / `daisyui.css` 均 `@import "../theme/vivid.scss"`。
  7. **（修复 4）滤镜选择器改为 `.encv-vivid ion-page, .encv-vivid .ion-page`**（两条并存，兼容
     `<ion-page>` TAG 与 `<div class="ion-page">` 两种 DOM 形态）。真实浏览器验证修复后
     `getComputedStyle(.ion-page).filter` 含 `contrast`/`saturate`（修复前恒 `none`）。
  8. **（修复 5）P3 交换规则加 `!important`** 越过 `applyColor` 内联覆盖；`--color-primary` 回退改用
     `applyColor` 新存的 `--color-primary-srgb`（srgb 基色），**严禁** `var(--color-primary)` 自引用；
     secondary/accent 标 `!important` 仅为防御一致；`--material-bg-active` 同理 `!important`。
  9. **（修复 6）`initTheme` 末尾改调 `applyVividMode(vividMode.value)`**（重新挂 `.encv-vivid` 根类 + 滤镜），
     修复刷新后类丢失。
- **关键认知（长期）**：`color-gamut` / `prefers-color-scheme` 等是**媒体特性、只读**，绝不能作为
  作者属性写在 `style` / 内联 / `setProperty` 上——要「响应式宽色域」只能靠 `@media (color-gamut: p3)`
  + 根类切换。这是本 bug 的隐蔽根因，下次动色彩 / 屏幕能力特性务必记住。
  - **CE 注册模式 DOM 形态（2026-07-17）**：`registerIonicComponents` 把 `<ion-*>` 渲染为 `<div class="ion-*">`
    （TAG 是 div，不是自定义元素名）。**任何依赖 `<ion-*>` TAG 的 CSS 选择器都会命中不到**——一律改用
    `.ion-*` 类选择器（不止 `ion-page`，`ion-content` 等若用 TAG 选择器同理要改类）。本环境真实浏览器复现过。
  - **内联 CSS 变量交换必须 `!important`**：当某 CSS 变量被 JS 以 `setProperty` **内联**写在 `:root` 上，
    普通作者规则（含 `@media` 块）优先级低于内联、**永远压不过**；要拿它做主题/媒体交换只能在该规则上加
    `!important`。回退值**不能**写 `var(--同名)`（自引用→无效声明→令牌丢失），应另存一份非循环基色变量。
- **契约锁（防复发，先红后绿）**：`encv-mobile/src/motion/__tests__/vivid.test.ts`（已入 `vitest.config.ts`
  FAST_INCLUDE）断言——开 vivid 加 `.encv-vivid` 根类、默认强度 100 即 `amount>0`、关 vivid 移除类且
  amount 归零、开 P3 加 `.encv-p3` 且不再写 `--encv-color-gamut`、任意（含非内置）色自动派生 `display-p3` 令牌、
  非法色回退 srgb、**`initTheme` 重新应用 `.encv-vivid` 根类**（修复刷新丢失）。
  - **真实浏览器回归（2026-07-17）**：`encv-mobile/test-visual/vivid-diag.visual.ts`（Playwright，3 用例）
    在 test-visual 挂载壳跑**真实主题 CSS + 真实 gsap**，断言：① 开启 vivid 后 `getComputedStyle(.ion-page).filter`
    含 `contrast`/`saturate`（修复前恒 `none`）；② 真实样式表里 `@media (color-gamut:p3) :root.encv-p3` 规则带
    `!important` 且回退 `--color-primary-srgb`；③ 级联复现：无 `!important` 时内联压过作者规则、有 `!important`
    时覆盖内联。这是「先红后绿」的权威证据（修复前 `realRuleFilter==='none'`）。
  - **编译期契约锁（2026-07-17）**：`src/theme/__tests__/vividScss.test.ts` 新增断言——compiled `vivid.css`
    含 `.encv-vivid .ion-page` 选择器、P3 块含 `!important` 与 `--color-primary-srgb`、且不再自引用
    `var(--color-primary)`。

### 6.17b 臻彩显示二次优化（2026-07-17，实测生效后）：滤镜拆分 + 明暗分调 + P3 自动化
- **用户实测「臻彩显示」已生效**，提出三项优化并全部落地、真实浏览器回归（见下）：
  1. **色彩滤镜与对比度拆开调节**：原单一 `vividIntensity` 滑块同时驱动 contrast + saturate。
     拆为 `vividSaturation`（色彩浓度）+ `vividContrast`（对比度）两个独立滑块（50..200，默认 100），
     对应 CSS 变量 `--encv-vivid-sat` / `--encv-vivid-contrast`（0..1），由 `syncVividFilter` 经 gsap
     分别平滑过渡。`useTheme` 导出 `setVividSaturation` / `setVividContrast`，localStorage 键
     `encv-vivid-mode-saturation` / `encv-vivid-mode-contrast`。旧的 `vividIntensity` / `--encv-vivid-amount` 已移除。
  2. **删除 P3「自动/始终开启/关闭」选项组**：该选择组冗余（"on"/"auto" 在 `@media (color-gamut:p3)` 下
     行为等价，区别只在 "off"）。UI 移除 `p3Modes` 卡片，`initTheme` 改为始终 `applyP3Mode("auto")`
     （`.encv-p3` 常驻，真实 P3 屏由媒体查询决定，srgb 屏仅强化 vivid 饱和）。`setP3Mode`/`p3Mode` 仍导出供测试。
  3. **明暗场景分别调优**：新增暗色专属规则 `:root.encv-vivid body.dark .ion-page`（specificity 高于亮色规则），
     暗色下对比度增益收敛 `*0.12`（避免暗部过硬）、色彩浓度增益加大 `*0.72`（让颜色更跳）；
     亮色规则 `*0.25` / `*0.45`（避免过艳刺眼）。选择器用 `:root.encv-vivid body.dark` 精确命中
     「html 带 .encv-vivid 且 body 带 .dark」的暗色臻彩态（CE 模式 `body.dark` 在 `html.encv-vivid` 之内）。
- **验证（先红后绿）**：
  - 真实浏览器 `encv-mobile/test-visual/vivid-diag.visual.ts`（Playwright，4 用例）全绿：① 开启 vivid 后
    `.ion-page` filter 含 contrast/saturate；② 暗色专属规则下 `saturate` 增益 > 亮色、`contrast` 增益 < 亮色
    （真实 computed 断言）；③ P3 `!important` + `--color-primary-srgb`；④ 级联复现内联压过/!important 覆盖。
  - 编译期 `vividScss.test.ts` 新增：滤镜拆为 `--encv-vivid-sat`/`--encv-vivid-contrast` 两条独立变量；
    暗色 `body.dark` 规则存在且含 `0.72`(浓度)/`0.12`(对比度)，亮色含 `0.45`/`0.25`。
  - 单测 `vivid.test.ts` 新增「浓度与对比度可独立调节」；`app_check_all` 全绿（9 PASS / 0 FAIL）。
- **纪律（`.codebuddy/rules/文档同步.mdc`）**：本次补本文档 §6.17；门禁只验代码不验文档，文档正确性由本次提交保证。
- **验证**：`app_check_all` → **9 PASS / 0 FAIL**（含 `vivid.test.ts`）；Biome 用 `pnpm exec biome check --write`
  实写（非 `app_format`）。

### 6.18 外观即「表面材质（surface material）」—— 统一主题 / 动态背景 / 模糊 / P3

> 2026-07-16 续49 用户指出：动态渐变背景（BG_PRESETS + `applyBgColor`）与后来的主题系统是**共存却冲突**的平行子系统；
> 高斯模糊（`--encv-bg-blur` 全局开关）不该是全局开关，而应交给主题控制（参考 iOS / 鸿蒙流行的**液态玻璃**）。
> 加上 §6.17 暴露的「背景/渐变拿不到真·P3 宽色域」缺口，外观层需要一次整合。

- **被取代的旧做法（已废除，已被本 § 取代）**：
  - ❌ 「**动态背景/渐变是独立于主题的平行系统**」（`BG_PRESETS` + `applyBgColor` 直接写 `--ion-background-color` / `body` 渐变）。
    它和主题写的 `--color-base-100` 叠成两套背景、深浅色多处各自处理。→ 背景/渐变**改为主题的属性**（见材质契约）。
  - ❌ 「**高斯模糊是全局开关**」（`applyBgBlur` → `--encv-bg-blur`，UI 上一个独立滑块）。
    实测它**不全局**：只有 `App.vue` + `HomePage` header 读 `--encv-bg-blur`，`codemogger_grep`（`format:json`、`dir` 限定 `src`）证实约 27 处组件硬编码 `backdrop-filter: blur(8/12/20px)` 根本不读它（另共享主题层 `NewTaskModal.vue` / `timeline-utilities.css` 各 1 处）。
    → 模糊**改为主题材质的一部分**（`--material-blur`），所有磨砂面统一读它。
  - ❌ 「**背景/渐变不享 P3 宽色域**」（§6.17 遗留缺口）。→ 背景也带 `-p3` 孪生令牌，随主题在 `@media (color-gamut: p3)` 下切换。
- **核心模型：一个主题 = 一套表面材质（material）**。外观 UI 只选「一个主题」，不再有独立的背景/模糊全局开关。
  主题（官方或预设）在 `theme.css` 里声明材质契约令牌；缺失时回落到全局默认材质。
- **材质契约（surface material tokens）**：
  ```css
  --material-bg:        <color | gradient>;   /* 原 BG_PRESETS → 主题属性；默认回落 --color-base-100 */
  --material-bg-p3:     <display-p3 gradient>;  /* 背景的宽色域孪生，关掉 §6.17 缺口 */
  --material-blur:      <px>;                    /* 原 --encv-bg-blur → 主题决定；0 = 关闭磨砂 */
  --material-saturate:  <number>;               /* 液态玻璃的饱和增强（叠加在 vivid 之上） */
  --material-tint:      <rgba>;                 /* 磨砂填充色（半透明） */
  --material-highlight: <color>;                /* 镜面高光描边（液态玻璃的灵魂边） */
  ```
- **液态玻璃（liquid glass）实现要点**（能力设备上的默认材质）：
  磨砂面 = 半透明 `var(--material-tint)` 填充 + `backdrop-filter: blur(var(--material-blur)) saturate(var(--material-saturate))`
  + 1px `inset` 高光描边（`box-shadow: inset 0 0 0 1px var(--material-highlight)`）。
  其**前提是背景够鲜艳**（故背景与模糊必须同主题自洽）；低配 / `prefers-reduced-motion` 主题给 `--material-blur: 0` + 不透明回落。
- **分期落地（按性价比）**：
  1. **本设计文档**（先钉事实，再动码）——即本 §。✅ 完成。
  2. `--encv-bg-blur` → `--material-blur`：
     - ✅ **已新增 canonical 材质令牌**：`--material-blur` 现作为材质令牌存在（钳制 0..40px 的语义由主题/使用者约定），
       `--encv-bg-blur` 保留为兼容别名（迁移期不破坏现有读者）。**2026-07-17 重要变更**：外观页的
       「背景模糊」独立设置项（原 `setBgBlur` / `bgBlur` / 滑块 UI）已**整体移除**——模糊不再作为全局开关，
       完全由主题材质 `--material-blur` 控制；`:root` 在 `variables.css` 里给出默认 `12px`，主题可在自身 `theme.css`
       覆写。`useTheme` 不再导出 `setBgBlur` / `bgBlur`（契约锁见 `surfaceMaterial.test.ts`：断言二者已 `undefined`）。
     - ✅ **已全量接入（2026-07-17）**：用 `codemogger_grep`（`format:json`、`dir` 限定 `src`）拿到的清单，
       把 `encv-mobile/src` 内约 27 处裸 `backdrop-filter: blur(<px>)` 全部改为 `blur(var(--material-blur, <原px>))`
       （含 `App.vue` 的液态玻璃 `saturate(1.8)` 站点 792 行、`.home-card` / `.player-card`；`HomePage.vue` /
       `ServerStatusCard.vue` 说明注释不计入），并把原读 `--encv-bg-blur` 的读者（App.vue / HomePage / NewTaskModal）
       统一到 `--material-blur`。共享主题层 `NewTaskModal.vue`、`timeline-utilities.css` 一并迁移。
       新增**回归锁**「全仓无裸 `blur(<px>)` 字面量」：`walk()` 扫描两套 `src` 树（剥离 `/* */` 与 `//` 注释避免
       ServerStatusCard.vue 注释误报），发现裸字面量即红。已做红验证（临时 `__blur_lock_scratch.vue` 的 `blur(5px)`
       被准确抓出 → 红；删除后转绿）。→ 模糊现已真正全局一致、由主题控制。
  3. ✅ **背景预设并入主题材质（2026-07-17）**：`applyBgColor` 现写 canonical `--material-bg`
     （纯色直接写；渐变写 `linear-gradient`）。`--ion-background-color` 与 `document.body` 不再写死
     字面量，统一改读中转变量 `--material-bg-active`（`vivid.css` 定义，默认回落
     `--material-bg` → `--color-base-100`）。`BG_PRESETS` 仍作为预设列表存在（未删，避免动 UI 选择器），
     但**写入路径已并入材质契约**——即「一个主题=一套表面材质」的写侧已落地。`bridge.css` 亮/暗块
     的 `--ion-background-color` 默认也改为 `var(--material-bg-active, var(--color-base-100))`。
  4. ✅ **补 `--material-bg-p3` 关掉 P3 背景缺口（2026-07-17）**：`applyBgColor` 对渐变预设用
     `hexToP3Token` 把每个停色转 `color(display-p3 ...)` 写 `--material-bg-p3`；`vivid.css` 的
     `@media (color-gamut: p3) { :root.encv-p3 }` 内把 `--material-bg-active` 换成
     `var(--material-bg-p3, var(--material-bg, var(--color-base-100)))`。**关键设计**：P3 切换作用在
     `--material-bg-active`（此变量只由 stylesheet 定义、从不被 `applyBgColor` 内联设置），故媒体查询
     可覆盖它——规避 §6.17 中 `--color-primary` 被内联设置、媒体查询可能压不住的潜在失效。纯色预设不写
     `-p3` 孪生（srgb 纯色已够艳，缺口本在渐变）。契约锁：`surfaceMaterial.test.ts` 的「背景并入材质令牌」
     描述块（断言 `--material-bg` / `--material-bg-p3` / 绘制面读 `--material-bg-active`；先红后绿：
     回退 `applyBgColor` 写字面量即 3 断言全红）。
  5. 液态玻璃高光描边（`--material-highlight`）作为默认材质变体接入。
- **对 codemogger 的要求（实践中完善）**：本次重构依赖「跨文件找 CSS 自定义属性 / `backdrop-filter` 字面量」，
  `references`/`impact` 只认 JS symbol、抓不到 CSS var；必须以 `codemogger_grep` 兜底。已给 `codemogger_grep`/`codemogger_search`
  补 `format: "json"` 输出（见 `codemogger-patch/mcp-server.mjs`），使迁移清单可机读、可脚本化替换。
- **纪律（`.codebuddy/rules/文档同步.mdc`）**：本次补本文档 §6.18；门禁只验代码不验文档，文档正确性由本次提交保证。

### 6.18.1 续51（2026-07-17）：主题「声明」调色板 + per-theme 主色/背景定制（不再固定全局预设）

> 用户优化外观（4 项）：① 去掉背景模糊设置项（见 §6.18 第 2 点）；② 主题卡片加「自定义」按钮，
> 背景色/主题色不再是固定的全局预设，改为**按主题声明的调色板**驱动；③ 所有主题据此各自实现可定制；
> ④ 充分利用 gsap + daisyUI + scss，框架层（composable）与 UI 解耦。

- **旧做法（已废除）**：外观页主色/背景色用**一套固定全局预设**（`THEME_PRESETS` / `BG_PRESETS` 的分类卡片），
  用户改的色是「全局」的，切主题不会回落、也记不住「每个主题各自的偏好」。
- **新模型：每个主题声明自己的调色板（palette）**，取色器据此驱动；用户可**按主题**改主色/背景并持久化覆盖。
  - **声明位置（双写、互锁）**：
    1. TS 单一来源：`useUserThemes.ts` 的 `USER_THEMES` / `BAZAAR` 每项带 `palette: ThemePalette`
       （`{ primary?, bg?, presets?: { primary?: string[], bg?: string[] } }`，颜色均为 `#rrggbb`）。
    2. 忠实镜像：`public/themes/<id>/theme.json` 的 `palette` 字段（由 `scripts/patch-theme-palettes.mjs`
       批量写入 **public + dist + android 三处**，共 33 个 theme.json）。`themeLoader.ts` 的 `parsePalette`
       负责校验并把声明带进 `ThemeManifest.palette`。
  - **取色器驱动源**：`useUserThemes.getThemePalette(id)` → 返回该主题声明的 `ThemePalette`（内置注册表优先，
    其次已安装主题）。`AppearanceDetail.vue` 用 `activeThemePalette`（`useTheme` 暴露的 computed）渲染
    `activePrimaryPresets` / `activeBgPresets`，**不再引用固定全局 `THEME_PRESETS` / `BG_PRESETS`**。
  - **per-theme 覆盖（持久化）**：`useTheme` 新增 `THEME_CUSTOM_KEY = "encv-theme-custom"`（结构
    `Record<themeId, { primary?, bg? }>`，`bg: string | null`，`null` = 用主题默认）。API：
    `setThemePrimary(color)` / `setThemeBg(color|null)` / `resetThemePrimary()` / `resetThemeBg()`。
    切主题时 `useTheme` 内 `watch(activeThemeId, reapplyActiveTheme)` 重放该主题的覆盖（或回落主题声明默认），
    `initTheme` 末也调用一次（并把旧全局 `COLOR_KEY` / `BG_COLOR_KEY` 迁移成「当前主题的覆盖」，保留用户历史选择）。
  - **框架解耦**：颜色/背景的**写入**只在 composable 内（`applyColor` / `applyBgColor` 写 `--color-primary` /
    `--material-bg` 等语义令牌），UI 组件只调 API；gsap 过渡只在 `applyColor` 内（见下），不污染组件。
- **gsap 平滑过渡主色（视觉优化 ④）**：`applyColor` 在写入终态 hex 的同时，若 `getMotionProfile().enabled`
  且 `motion.to` 可用，用 gsap 把代理对象 `{r,g,b}` 从 `prev` 补间到 `target`，`onUpdate` 逐帧把
  `--color-primary` 覆写为 `rgb(...)`（终态 = 目标色）。reduced-motion / 无 gsap 时直接落终态 hex，
  保证测试/无动效环境也能拿到正确终值。背景色**不**走 gsap（直接写 hex，终态即 hex）。
- **UI（AppearanceDetail.vue）**：
  - 每个主题卡片加「自定义」按钮（`.theme-customize`，`colorPaletteOutline` 图标）+ 已定制标记
    （`.theme-customized-mark`，`sparklesOutline`，当该主题有覆盖时显示）。点按钮 = `applyTheme(id)` +
     滚动到取色器（`#theme-customize` / `#theme-primary`）。
  - 背景色 / 主题色两块改为**按当前主题声明的预设**渲染色板（纯色 swatch，不再分类卡片），并显示当前/覆盖态、
    提供原生 `<input type=color>` 自定义 + 一键复位。两块头部 badge 显示当前主题名（取代原来的 Dark/Light）。
- **防复发（契约锁）**：
  - `themeCustomization.test.ts`：断言 `setThemePrimary`/`setThemeBg` 写入响应式真值 + 持久化覆盖；
    切主题后 per-theme 覆盖**互相隔离并随主题重放**；`resetThemePrimary/Bg` 回落主题声明默认；
    `getThemePalette` 对各主题返回声明预设。**契约锁**：遍历 `USER_THEMES`，断言每项 `palette`
    与 `public/themes/<id>/theme.json` 的 `palette.primary` / `presets.primary` **逐字节一致**（TS 源 = 镜像源）。
  - 真实浏览器门禁：`test-visual/appearance-customize.visual.ts`（Playwright 挂 `AppearanceDetail`）
    断言「真实 DOM 有 `.theme-customize` 按钮」「改主色 → `--color-primary` 真实变为 `rgb(...)` 且 localStorage 记录覆盖」
    「改背景 → `--material-bg` 真实变化」「切主题不串色（per-theme 隔离）」。
- **纪律（`.codebuddy/rules/文档同步.mdc`）**：本次补本文档 §6.18.1；门禁只验代码不验文档，文档正确性由本次提交保证。

### 6.19 续50：CSS 产物 → SCSS 源溯源（codemogger css-source）+ vivid P3 孪生 SCSS 化

> 2026-07-17 用户要求：增强 codemogger，使其能「通过 CSS 产物溯源到 SCSS 源」；溯源能力到位后，
> 放心用 SCSS 高级能力（@function / @mixin / @each / @use）重写主题，不再怕「生成的规则找不到出处」。

- **codemogger `css-source`（CSS 产物溯源）**：新增 `codemogger css-source <file.css>` 子命令 + MCP
  `codemogger_css_source`。读取 CSS 同目录的 `*.css.map`（Sass/Vite 产出），手工解码 base64-VLQ
  source map（纯 Node，无新依赖），把「CSS 第 N 行」映射回「`.scss` partial 的 file:line:col + 片段」。
  两种模式：① 无 `--line` → 按源汇总（每个 `.scss` 源贡献了多少生成行，含样例映射）；
  ② `--line N` → 单行溯源。`--json` 机器可读。**关键点**：`@mixin`/`@function`/`@each` 生成的规则
  在 scss 中不以字面量出现，但 Sass 仍把它们精确记入 source map，故溯源对此类规则完全有效——
  这正是「放心用 SCSS 高级能力」的安全网。
  - 实现：`codemogger-patch/css-source.mjs`（VLQ 解码器）+ `codemogger-shim` 的 `css-source` 分支
    + `mcp-server.mjs` 的 `codemogger_css_source` 工具（含别名 `file`/`path`/`f`、`line`/`l`）。
  - Vite 产物注意：Vite 8(rolldown) 的 `build.cssSourcemap` 在本环境**不实际产出** `.css.map`，
    故另起专用编译步骤产出可溯源产物（见下），`vite.config.ts` 仍保留 `css.preprocessorOptions.scss.sourceMap`
    + `build.cssSourcemap: true`（意图正确、环境修复后即生效）。
- **vivid P3 孪生 SCSS 化（尽情用 SCSS 高级能力）**：
  - `vivid.css` → `vivid.scss`：`theme-core.css` 与 `daisyui.css` 的 `@import` 改为 `../theme/vivid.scss`。
  - 新增 `_vivid-p3.scss` partial（现代 `@use "sass:color"` + `@use "sass:math"`）：
    - `@function p3($hex)`：用 `color.channel()` 取归一化 sRGB 通道 + `math.round` 匹配 JS `toFixed(4)`，
      拼 `color(display-p3 ...)` 令牌（语义与 `useTheme.hexToP3Token` 完全一致，保证默认/自定义主题在 P3 屏视觉统一）。
      ⚠️ 本 sass-embedded 构建未暴露 `color.display-p3()` 构造器，故用 `color.channel` + 手工拼装。
    - `$encv-brand` map：encv 品牌色（primary/secondary/accent）的 sRGB 单一来源，键与 `palette.css` 一致。
    - `@mixin emit-p3-twins`：`@each $role, $hex in $encv-brand` 生成 `--color-#{$role}-p3: #{p3($hex)};`。
  - `palette.css` 里手写的两处 `--color-*-p3: color(display-p3 ...)` 字面量**已删除**（单一事实源迁到 SCSS，
    消除与基色 hex 的漂移）；encv 默认 `primary` 现也编译出 `-p3` 孪生（此前仅有 JS 运行时补，P3 覆盖更完整）。
  - 作用域：孪生生成在 `:root, [data-theme="encv"]`（与 palette.css 基色块对齐），仅 encv 默认主题生效；
    自定义/远程主题色仍由 JS 写 `--color-primary-p3` 内联覆盖，secondary/accent 回落 srgb（与既有设计一致）。
- **可溯源产物生成器**：`packages/shared-components/scripts/build-theme-scss.mjs`（npm `build:scss`）用
  `sass-embedded` 编译 `surface.scss` / `vivid.scss` → `src/theme/.dist/*.css` + `.css.map`
  （`.dist` 已 gitignore）。`codemogger css-source src/theme/.dist/vivid.css` 即可把
  `--color-secondary-p3: color(display-p3 ...)` 精确溯源到 `_vivid-p3.scss:42:5` 的 `@each` 循环体。
- **契约锁（`src/theme/__tests__/vividScss.test.ts`，3 用例，先红后绿）**：
  ① 编译 `vivid.scss` 断言 primary/secondary/accent 的 `-p3` 孪生值（naive 归一化匹配 `hexToP3Token`）；
  ② 断言 `palette.css` 不再含手写 `-p3` 字面量（防回潮）；
  ③ 解码 source map 断言生成的 `--color-secondary-p3` 行**精确溯源到 `_vivid-p3.scss`**（与 css-source 同源校验）。
- **验证**：`app_check_all` 全绿（9 PASS）；`codemogger css-source` 对 `surface.scss`/`vivid.scss` 产物均验证回源。
