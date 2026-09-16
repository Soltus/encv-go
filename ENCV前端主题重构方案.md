# ENCV 前端主题重构方案：以 daisyUI + GSAP 重塑视觉与主题体系

> 范围：以共享包 `@encv/shared-components` 为主，覆盖 `encv-mobile` 及插件 web 应用。
> 参考：Obsidian 主题机制（变量覆盖 + 片段）+ 思源笔记（SiYuan）主题机制（清单 + CSS + 可选 JS 钩子）。
> 已安装参考 skill：`gsap-core` / `gsap-react` / `gsap-scrolltrigger` / `daisyui`（经 app-dev MCP 注册，编写实现时可 `use_skill` 调阅官方用法）。

---

## 0. 目标与原则

**目标**

1. 用 **daisyUI v5**（Tailwind v4 组件/工具层）+ **GSAP**（动效层）重塑前端视觉，主应用 `encv-mobile` 与插件 web 应用共用一套主题。
2. 建立**可定制、可运行时切换、可用户扩展**的主题体系（对标 Obsidian / 思源笔记）。
3. `encv-mobile` 现存的"旧主题色实现"彻底焕新，但保留其独有的高级特效（P3 色域、vivid 滤镜、背景模糊、渐变背景、护眼预设）。

**原则**

- **令牌驱动（Token-driven）**：所有颜色/圆角/间距来自 CSS 变量，禁止在组件里硬编码色值。
- **daisyUI 为主题事实源**：Ionic 等原生组件向下消费 daisyUI 令牌，单一调色板。
- **主题 = 声明式资产**：一个主题 = 清单 + 令牌 CSS（+ 可选 JS 钩子），可加载、可卸载、可分发。
- **特效作为叠加层**：用户自定义主色 / 渐变 / 模糊 / vivid / P3 是"叠加在主题之上的覆盖层"，不破坏主题本身。
- **动效尊重无障碍**：GSAP 动画一律遵守 `prefers-reduced-motion` 及现有 `vividMode` / `isP3Supported` 标志。

---

## 1. 现状剖析（为什么要重构）

### 1.1 三套并行、彼此脱节的样式来源

| 来源 | 引入位置 | 现状 |
| --- | --- | --- |
| `theme/variables.css` | `encv-mobile/src/main.ts:34` | Ionic 亮/暗变量**写死**（`:root` + `body.dark`），主色 `#4f8cff`（蓝） |
| `styles/daisyui.css` | 仅 `plugin-*/web/src/main.ts`（simverse / openlist / mpv） | daisyUI v5，主色 `#8b5cf6`（紫），`encv`/`encv-dark` 两主题 |
| `useTheme.ts` 命令式补丁 | `encv-mobile/src/App.vue:373` `initTheme()` | 运行时 `setProperty('--ion-color-primary', …)` 覆盖根变量 |

**问题**：主应用 `encv-mobile` **根本没引入 `daisyui.css`**，所以 daisyUI 组件/令牌只在插件 web 应用生效；主应用显示蓝色主调，插件显示紫色主调 —— 视觉不一致，正是"焕然一新"要解决的。

### 1.2 `useTheme.ts` 是命令式"补丁"引擎（旧主题色实现的核心）

`src/composables/useTheme.ts` 直接对 `:root` 写属性：

```ts
root.style.setProperty("--ion-color-primary", color);
root.style.setProperty("--ion-color-primary-rgb", rgb);
root.style.setProperty("--ion-color-primary-shade", darker(color, 10));
// …自带 hexToRgb / lighter / darker 运算
```

它与 CSS 变量系统割裂：主题无法"接管"它，改一个色要手写 6 个派生变量；Ionic 与 daisyUI 两套变量互相不知对方存在。

### 1.3 daisyUI 已脚手架但未在主应用启用、且只有 2 个主题

`styles/daisyui.css` 已用 daisyUI v5 写法（`@plugin "daisyui"` + `@plugin "daisyui/theme"`），并定义了 `.encv-card` / `.encv-panel` / `.encv-btn` 共享类。但：

- 主应用未引入 → 共享类与令牌在主应用不可用；
- 仅 `encv` / `encv-dark` 两主题，无"用户可选主题 / 可安装主题"机制；
- 旧 7 色预设（Blue/Purple/…）只是 `useTheme` 里的 hex 数组，与 daisyUI 主题没有钩稽关系。

### 1.4 值得**保留**的高级能力（不要丢掉）

`AppearanceDetail.vue` + `useTheme.ts` 已实现的独特能力，应被新体系**吸收并升华**：

- **P3 广色域强制**（`--encv-color-gamut: display-p3` + `encv-force-p3` 类）
- **vivid 模式**（CSS `contrast() saturate()` 滤镜 + 强度）
- **背景模糊**（backdrop 模糊）
- **渐变背景**（多 stop 线性渐变）
- **护眼 / 暗 / 亮 / 渐变** 四类背景预设 + 自定义取色器

---

## 2. 目标架构（参考 Obsidian + 思源笔记）

### 2.1 三层令牌模型（primitive → semantic → component）

对标 Obsidian `:root` 变量覆盖与思源 `theme.css` 的令牌体系：

```
primitive（调色板原始值，按主题覆盖）
        ↓
semantic（角色令牌：主色/背景/表面/文本/边框/成功/警告/危险）
        ↓
component（daisyUI --color-* / Ionic --ion-color-* 由 semantic 桥接而来）
```

语义令牌契约（节选）：

| Semantic 令牌 | 含义 | 桥接到 |
| --- | --- | --- |
| `--encv-color-primary` | 主色 | `--color-primary`（daisyUI）、`--ion-color-primary`（Ionic） |
| `--encv-color-bg` | 页面背景 | `--color-base-100`、`--ion-background-color` |
| `--encv-color-surface` | 卡片/浮层 | `--color-base-200`、`--ion-card-background` |
| `--encv-color-text` | 正文 | `--color-base-content`、`--ion-text-color` |
| `--encv-color-border` | 描边 | `--color-base-300`、`--encv-border-color` |
| `--encv-color-success/warn/danger` | 语义状态色 | daisyUI / Ionic 对应变量 |

> **关键倒转**：daisyUI v5 的主题提供 `--color-*`，我们让 Ionic 与自有语义令牌**向下消费** daisyUI 令牌（`--ion-color-primary: var(--color-primary)`）。这样切 `data-theme` 一处，Ionic 原生组件、daisyUI 组件、业务组件全部同步换肤。

### 2.2 主题 = 清单 + 令牌 CSS + 可选 JS（思源笔记模型）

每个主题是一个目录资产：

```
src/theme/themes/<theme-id>/
  theme.json      # 清单：{ "id", "name", "author", "mode": "light|dark|any", "version" }
  theme.css       # 仅覆盖本主题的 primitive/semantic 令牌（如 --color-primary、--color-base-100）
  theme.js        # 可选：export function mount() / unmount() —— 跑 GSAP 入场动画、动态行为
```

- **内置主题**：`encv`（亮）、`encv-dark`（暗，跟随 `prefers-color-scheme`）。旧 7 色预设 + 渐变**降级为主色覆盖层**（见 2.4），不再各自成"主题"。
- **用户可安装主题**：`src/theme/user-themes/<id>/` + 注册表（`registry.ts` 扫描清单），对标思源"集市 / bazaar"与 Obsidian 社区主题。
- **Snippets（片段）**：`src/theme/snippets/*.css` 提供 Obsidian 式的**局部覆盖**（只改某个组件/某类卡片），可独立开关，不打主题。

### 2.3 运行时切换（daisyUI 原生 `data-theme`）

- 激活主题 = 在 `<html>` 上设置 `data-theme="<id>"`（daisyUI v5 原生支持多主题共存 + 按元素 `data-theme`）。
- `useTheme` 不再手写 `--ion-color-primary`，改为：`setTheme(id)` → 写 `document.documentElement.dataset.theme = id` + 持久化。
- 暗色策略：默认 `encv-dark` 跟随系统；用户可在 Appearance 里强制亮/暗（一个 `--encv-force-dark` 类）。

### 2.4 特效作为「叠加层」而非「主题」

保留 1.4 的全部高级能力，但实现方式改为对语义令牌/独立变量的覆盖（不再入侵 `--ion-color-*`）：

- **自定义主色**：仅覆盖 `--color-primary`（daisyUI v5 用 `color-mix` 自动派生 content/shade/tint，无需手写 6 个变量）。`useTheme` 只需算一个对比色给 `--color-primary-content`。
- **渐变背景**：用一个固定定位的 `--encv-bg-gradient` 背景层 div（而非 `body.backgroundImage` hack），`opacity` 受模糊/暗度控制。
- **背景模糊**：浮层/面板 `backdrop-filter: blur(var(--encv-bg-blur))`。
- **vivid / P3**：维持现有 `--encv-vivid-filter`、`--encv-color-gamut` 变量与 `encv-force-p3` 类，纳入新令牌层。

### 2.5 动效层 GSAP（焕然一新的手感来源）

`src/motion/` 是统一的动效中枢：所有 GSAP 动画都从这里发起，受 `guard.ts` 闸门统一管控（见 2.6）。下面是按交互场景分类的**动效全集**（Phase 3 落地清单），覆盖从路由转场到微交互、从手势到引导的完整手感。

#### 2.5.1 路由 / 页面转场
- **页面进入**：`ion-page` 进场 = 背景 `surface` 微缩放（1.02→1）+ 内容 `opacity 0→1 / y 12→0`，时长 320ms，ease `power2.out`。
- **页面离开**：反向 `y 0→-8 / opacity→0`，与进入拼成一条 timeline，避免"跳变"。
- **共享元素转场**：列表卡片 → 详情页封面，用 `Flip` 插件做尺寸/位置 morph（见 3.4 `flip.ts`）。
- **模态页栈**：`push` 叠加一层 `x 100%→0` 滑入，`pop` 滑出，遮罩 `opacity` 同步。

#### 2.5.2 内容进入与滚动揭示
- **列表 stagger 入场**：`ion-item` 逐条 `y 16→0 / opacity 0→1`，stagger 0.04s，scrollTrigger 视口触发。
- **长列表/时间线揭示**：`gsap-scrolltrigger` 驱动，`timeline-*` 卡片进入视口时从左/下揭示（对标现有 `timeline-*` 令牌）。
- **分节标题视差**：滚动时 header 背景图 `yPercent` 慢速位移 + 标题 `opacity/scale` 渐隐（sticky 头图视差）。
- **无限滚动 loading**：底部 spinner 入场上浮 + 旋转（配合骨架）。

#### 2.5.3 微交互（高频点击/悬浮）
- **按钮按压**：`:active` 时 `scale 0.96` 弹性回弹（`back.out(2)`）。
- **卡片悬浮**：hover 抬升 `y -4` + 阴影通过 `--encv-shadow` 变量过渡（`gsap.to` 代理 CSS 变量）。
- **卡片点击波纹**：pointer 位置 `scale 0→1 / opacity→0` 圆形 ripple（替代 Ionic 默认 ripple，更顺）。
- **开关 Toggle**：滑块 `x` 位移 + 轨道 `backgroundColor` 过渡 + knob 微弹。
- **取色器**：色相环拖动时 `gsap.quickTo` 实时更新 `--color-primary`，零卡顿。
- **分段控件 Segmented**：下划线/滑块在选项间 `x` 滑动（morph 指示条）。
- **复选框**：对勾 SVG `strokeDashoffset` 绘制（drawSVG 思路，手写实线路径动画）。
- **磁性按钮**：鼠标靠近 `x/y` 朝指针微移（`magnetic.ts`，follow 指针，离开归位）。

#### 2.5.4 浮层（弹窗 / BottomSheet / Drawer / Toast / Tooltip）
- **Alert/Dialog**：遮罩 `opacity 0→1` + 面板 `scale 0.95→1 / y 8→0`，260ms。
- **BottomSheet**：从底部 `y 100%→0`，`ease back.out(1.4)` 微弹；拖拽把手可 `dragToDismiss`。
- **Drawer 侧栏**：`x -100%→0` 滑入，遮罩同步。
- **Toast/Snackbar**：底部 `y 20→0 / opacity` 入，停留 2.5s 后 `y 20` 出。
- **Tooltip/Popover**：`scale 0.9→1 / opacity`，跟随锚点，箭头方向自适应。
- **Context Menu**：菜单项 `stagger y 6→0 / opacity`（级联微动）。

#### 2.5.5 导航反馈
- **Tab 切换**：底部 tab 图标 `scale` 弹 + 标签 `opacity`，指示器滑动。
- **FAB 展开**：主按钮旋转 45° + 子按钮 `stagger` 放射状展开（`scale 0→1 / rotate`）。
- **Search 展开**：搜索栏从图标态展开为全宽，`input` 聚焦淡入。
- **返回手势**：左缘 `drag` 跟手 `x`，松手判定阈值（见 2.5.9）。

#### 2.5.6 数据 / 状态反馈
- **数字滚动 count-up**：数值变化 `gsap.to({val})` 缓动 + `onUpdate` 写 DOM（统计/余额/百分比）。
- **进度条**：`scaleX 0→1` `transform-origin left`，`ease power1.inOut`，可中断续跑。
- **骨架屏 shimmer**：渐变高光 `x -100%→100%` 循环（`repeat:-1`）。
- **成功对勾绘制**：大对勾 SVG `strokeDashoffset` 一笔画出 + 外圈 `scale` 弹（`success.ts`）。
- **Confetti 成就**：粒子从中心喷射（轻量自写 physics，非外挂）。

#### 2.5.7 主题与氛围视觉
- **主题光泽过渡**：切 `data-theme` 时整页 `brightness/contrast` 微脉冲 + 主色 `hue` 扫过（`themeIntro.ts`，供 `theme.js` mount 调用）。
- **Aurora / 网格背景**：低饱和流动渐变层，`gsap` 驱动多 `background-position` 慢速漂移（`ambient.ts`，受 P3/vivid 调节）。
- **光标辉光**：跟随指针的径向光斑（`spotlight.ts`，`gsap.quickTo` 跟手，离开淡出）。
- **文字 scramble**：标题字符随机乱码→落定（`scrambleText` 思路，手写实版），用于主题名/品牌位。

#### 2.5.8 引导与空态
- **Onboarding 步骤**：步骤卡 `flip`/淡入横移，进度点 `scale` 弹。
- **新功能高亮**：目标元素 `outline` + 聚光遮罩 `opacity`，提示气泡 `stagger` 入。
- **空状态插画**：SVG 局部（云/叶）`float` 循环漂浮（`yoyo` repeat）。
- **首屏 Logo 入场**：品牌 mark `scale 0.8→1 / rotate` + slogan 字符 `stagger` 上移（splash）。

#### 2.5.9 手势驱动（跟手动画）
- **下拉刷新**：`pull-to-refresh` 下拉距离跟手，松手阈值触发 spinner 旋转 + 回弹。
- **侧滑返回**：左缘 `drag` 跟手 `x`，与下层页同步位移，松手判定。
- **拖拽排序**：列表项 `drag` 跟手 + 占位让位（reorder，`gesture.ts`）。
- **滑动消除**：行内 `x` 跟手，超阈值飞出 + 高度塌缩。

> 上述全部走 `guard.ts`：reduced-motion 时落终态、vivid 时增强幅度/粒子数、P3 时色更艳。

#### 2.5.10 落地进度（2026-07-16）

动效防腐层（`src/motion/` ACL）已建成，本日把 Phase 3 动效全集**开始接入真实 UI**（续44 同批）：

- `ExtensionsPage.vue`：页面进入转场 `usePageTransition`（作用 `ion-page.$el`）+ 列表 `useScrollReveal` stagger（已有试点，加 `ready` 闸门等待异步加载后入场）。
- `Settings.vue`：页面进入转场 `usePageTransition` + 设置分组 `useScrollReveal` stagger（作用 `ion-content.$el` 的各 `ion-list`）。
- `micro.ts` 的 `useRipple` 自动把宿主设为 `position:relative; overflow:hidden`，ripple 圆形无需额外 CSS 即可被正确裁剪（`.encv-ripple` 由 JS 内联样式驱动）。
- **契约测试** `encv-mobile/src/motion/__tests__/motion-guard.test.ts`（已入 `FAST_INCLUDE`）：锁死 `guard` 闸门口径（reduced-motion / vivid-P3 / 总闸覆盖）+ 设计令牌导出 + **指令层 7 指令的注册与 API**。这是「换 gsap+daisyui 技术栈、下游零改动」的运行时兜底——换引擎（改 `internal/index.ts` 一行）不会改变闸门口径。

#### 2.5.11 动效指令层（应用层 · 自助接入，2026-07-16 续45）
composables（usePageTransition / useScrollReveal / ...）内部用 `onMounted/onUnmounted`，必须在组件 setup 上下文调用，无法直接用于 Vue 指令。为让动效「一行接入」全应用，新增 **指令层** `packages/shared-components/src/directives/motion.ts`：
- 7 个指令：`v-reveal`（滚动揭示，支持 `{ stagger: true }`）、`v-page-transition`（页面进入淡入上移）、`v-ripple`（点击波纹）、`v-press`（按压弹性回弹）、`v-hover`（悬浮抬升）、`v-magnetic`（磁性跟手，可传 strength）、`v-count-up`（数字滚动，绑定 number）。
- 指令内**直接驱动 `motion` 引擎 + `guard` 闸门**（用指令生命周期 `mounted/unmounted/updated`，cleanup 存 `WeakMap`），不经过 composables 的 `onMounted`——因此仍受 ACL 约束、引擎可透明替换，换栈下游零改动。
- `main.ts` 已 `installMotionDirectives(app)` 全局注册，任意组件一行 `v-page-transition` 即可用。
- 已接入：`Files.vue` 的 `<ion-page v-page-transition>`、`AgentChat.vue` 模态根 `<div v-page-transition>`（高频页面/模态进入转场）。

> 接入原则（与主题 re-surface 一致）：**bespoke 交互组件（如 `ServerStatusCard` 的 3D 翻转/脉冲）保留专属动画，不套共享动效层**；通用列表/页面/卡片才接 `v-reveal`/`v-page-transition`/`v-ripple`/`v-press`。后续按本清单继续铺开（文件长列表 Files.vue 的列表项 `v-reveal`、Toast/BottomSheet 浮层、FAB 展开、数字 count-up 等）。

### 2.6 动效设计令牌（缓动 / 时长 / 强度）

集中定义，组件不再各写魔法数：

```css
:root {
  --motion-ease-out: cubic-bezier(0.22, 1, 0.36, 1);     /* power2.out 等价 */
  --motion-ease-in-out: cubic-bezier(0.65, 0, 0.35, 1);  /* power1.inOut */
  --motion-ease-back: cubic-bezier(0.34, 1.56, 0.64, 1); /* back.out */
  --motion-dur-fast: 160ms;
  --motion-dur-base: 320ms;
  --motion-dur-slow: 520ms;
  --motion-stagger: 0.04s;
  --motion-intensity: 1;        /* vivid/P3 时上调到 1.3 */
}
```

`guard.ts` 读 `--motion-intensity` + `matchMedia('(prefers-reduced-motion)')` + `vividMode` 计算 `MotionProfile`：`{ enabled, intensity, respectsReduced }`，所有动画工厂读取后缩放时长/位移。

> **gsap 赋能主题（运行时读令牌）**：上述 `--motion-*` 令牌的「单一真源」在 `theme/tokens.css`，
> 纯 CSS 动画直接 `var()` 消费；GSAP（JS 引擎）侧经 `src/motion/theme-read.ts` 在运行时读取根节点
> `--motion-dur-*` / `--motion-stagger` / `--motion-intensity` 计算值（`tokens.ts` 的 `DUR` getter、
> `getStagger()`、`guard.ts` 的 `intensity` 均走此路径）。因此**主题 / 用户片段覆写这些令牌会同时作用于
> 纯 CSS 与 GSAP 动画**（250ms 节流缓存，主题切换调 `invalidateMotionTokenCache()` 即时生效），
> 消费方零改动——主题真能定制动效节奏（见 THEME_DEV.md §6.16）。

---

### 2.7 daisyUI v5 主题与组件体系（落地细节）

> 基于已安装的 **daisyUI 5** 官方 skill（`config` / `colors` / `usage`）。本项目已用 `tailwindcss ^4.1` + `daisyui ^5` + `@tailwindcss/vite ^4.1`，**无需重装**，缺的是体系化落地。

#### 2.7.1 双插件结构（当前写法已正确，保留）
```css
@import "tailwindcss";
@plugin "daisyui" {                 /* 只声明「启用哪些主题」 */
  themes: encv --default, encv-dark --prefersdark;
  /* 可选：root / include / exclude / prefix / logs */
}
@plugin "daisyui/theme" {           /* 真正的颜色令牌 */
  name: "encv";  default: true;  color-scheme: light;
  --color-primary: #8b5cf6;  /* 全部 19 个 --color-* */
}
```
- `@plugin "daisyui"` 是"开关+清单"，`@plugin "daisyui/theme"` 是"调色板"。本项目已正确分离。
- config 进阶项：`root: ":root"`（主题挂哪里）、`include`/`exclude`（剔除不需要的样式，如 `exclude: rootscrollgutter, checkbox`）、`prefix: daisy-`（给所有 daisyUI 类加前缀避免与别的 UI 库冲突——**启用需把 `btn`→`daisy-btn` 全量改写，慎重**）、`logs: false`。

#### 2.7.2 自定义主题必填令牌（全量 19 色）
每个 `@plugin "daisyui/theme"` **必须**提供全部 19 个 `--color-*`：`base-100/200/300/content`、`primary(+content)`、`secondary(+content)`、`accent(+content)`、`neutral(+content)`、`info(+content)`、`success(+content)`、`warning(+content)`、`error(+content)`。颜色可用 **hex / OKLCH / 任意 CSS 颜色**。本项目 `encv` 现用 hex（最保守，见 2.8）；新主题推荐 OKLCH（更广色域，Chromium 111+ 已支持，114 可用）。

#### 2.7.3 语义色使用铁律（避免暗色不可读）
1. daisyUI 把语义色加进 Tailwind 调色板，`bg-primary` 等随主题变化；**不要**用 `dark:` 前缀（daisyUI 已按 `data-theme` 自动换色）。
2. 页面大面积用 `base-*`（base-100 背景、base-200/300 做层次），`primary` 只用于**每页一个**最重要元素。
3. **禁止**用 Tailwind 具名色做文本（如 `text-gray-800`）——暗色下 `bg-base-100` 是深色，灰字会不可读；改用 `text-base-content`。
4. 仅"必须脱离主题"的特例（图表、固定品牌 SVG）才用具名色。

#### 2.7.4 形状 / 深度令牌（"焕然一新"的开关）
除颜色外每个主题设：`--radius-selector`(checkbox/toggle/badge)、`--radius-field`(button/input/select/tab)、`--radius-box`(card/modal/alert)，取值 `0 / 0.25 / 0.5 / 1 / 2rem`；`--size-selector`/`--size-field`(默认 0.25rem)；`--border`(默认 1px)；`--depth`(0|1 阴影/3D 浮雕)；`--noise`(0|1 颗粒噪点)。把全局圆角从 Ionic 4px 拉到 daisyUI 1rem，整体风格即转向。

#### 2.7.5 组件层与共享类
- `@layer components` 已有 `.encv-card/.encv-panel/.encv-btn`（正确）。Phase 4 补充 `.encv-input/.encv-badge/.encv-modal` 等，收敛散落 scoped 样式。
- **theme-controller**：daisyUI 提供 `<input class="theme-controller" type="radio" value="encv-dark">` 零 JS 切主题；我们保留 `useTheme.setTheme()` 写 `data-theme` 的**受控**方案（便于叠加层 + 持久化 + 预览），不采用 uncontrolled 的 theme-controller。

#### 2.7.6 把 `themes/` 目录接入 config（用户可安装主题）
- 内置主题写死在 `daisyui.css` 的 `@plugin "daisyui/theme"` 块（2.7.1）。
- 用户主题（`src/theme/user-themes/<id>/theme.css`）走**构建期拼接**：在 `vite-plugins/daisy-ui.ts` 扫描这些目录，生成对应的 `@plugin "daisyui/theme"` 块（或在 CI 生成 `themes:` 列表）。
- ⚠️ **重要**：daisyUI 主题是"声明式 + 编译期"，运行时不能动态新增 `data-theme` 名。用户主题必须在构建期注册进 config，运行时只是切换 `data-theme="<已注册名>"`（这正是思源"集市"式分发 + 构建期打包的折中）。
- 🆕 **续37（主应用 encv-mobile 纯 CSS 路径，已落地）**：主应用不走 daisyUI 编译期拼接，
  而是把主题实现为**运行时可加载的文件夹资产包**（`public/themes/<id>/theme.json` + `theme.css`
  + 可选 `theme.js` / `assets/`），由 `themeLoader`（`packages/shared-components/src/theme/themeLoader.ts`）
  运行时注入 `<link>` 并切 `[data-theme]`，**官方 == 第三方同形态、同加载机制**（仅 `builtIn` 区分预装），
  真正满足 §2.3「可加载 / 可卸载 / 可分发」。配套性能指标（load/switch/cache/FOUC）+ 优化
  （预加载 / 去重缓存 / LRU 卸载 / 防 FOUC）。详见 `THEME_DEV.md` §6.10。
  ⚠️ 因此主应用路径**不再**把任何主题 `@import` 进 `theme-core.css`（那会剥夺运行时装卸能力）。

### 2.8 浏览器兼容性：Chromium 114 基线验证

项目要求 **Chromium 114+（2023-04-11 功能冻结）**。Tailwind v4 / daisyUI v5 的官方基线正是 **Chrome 111+**（需支持 `color-mix()` / CSS nesting / OKLCH / `@property` / cascade layers），**114 ≥ 111 → 核心能力全部可用，无需 CSS polyfill。**

#### 2.8.1 关键 CSS 特性对照（Chromium 114）
| 特性 | 首次可用 | 114 可用 | 用途 |
| --- | --- | --- | --- |
| OKLCH 颜色 | 111 | ✅ | daisyUI 主题色（hex 亦可） |
| `color-mix()` | 111 | ✅ | daisyUI 派生 shade / `primary-content` 自动推算 |
| CSS nesting | 112 | ✅ | Tailwind v4 产物 |
| `@property` | 85 | ✅ | 动画/自定义属性 |
| `:has()` | 105 | ✅ | daisyUI 部分选择器 |
| cascade layers `@layer` | 99 | ✅ | Tailwind v4 分层 |
| `backdrop-filter` | 76 | ✅ | 背景模糊特效 |
| container queries | 105 | ✅ | 响应式组件 |
| `light-dark()` | 123 | ❌ | daisyUI **不依赖**（用 `data-theme`） |
| `transition-behavior: allow-discrete` | 117 | ❌ | 仅影响 `display`/`overlay` 过渡，非核心 |
| `color()` / `field-sizing: auto` | 123 / 123 | ❌ | daisyUI 默认不用 |

#### 2.8.2 结论与约束
- **核心可用**：daisyUI 组件、主题切换、`color-mix` 自动派生、`primary-content` 免手写、OKLCH、背景模糊全部在 114 正常。本项目 `encv` 主题用 **hex**（零风险），即使换 OKLCH 仍兼容 114。
- **自定义 CSS 禁令**：写 `tokens.css` / `theme.css` / 特效层时，避免 `light-dark()`、`transition-behavior: allow-discrete`、`field-sizing: auto`、`color()` 等 117/123 才有的特性；若必须用，提供 `@supports` 兜底或仅作渐进增强（不影响主流程）。
- **Tailwind v4 不降级**：oxide 引擎输出现代 CSS 原样，**不转译旧语法**。兼容性完全取决于"浏览器 ≥ 基线"，我们已满足，故不引入 autoprefixer / 降级插件。
- **JS 侧**：Chromium 114 支持 ES2022 / 动态 `import` / Top-level await；Vite 8 默认 `build.target` 足够，可显式设 `build.target: 'chrome114'` 收紧（可选）。

#### 2.8.3 落地动作
1. 在共享包 `package.json` 加 `"browserslist": ["Chrome >= 114"]` 记录意图（Tailwind v4 不读它，但供将来引入 lightningcss / `@vitejs/plugin-legacy` 时统一口径）。
2. `vite-plugins/daisy-ui.ts` 维持 `@tailwindcss/vite`；不引入降级插件（114 无需）。
3. 主题开发指南写明红线：只用 114 支持的特性。
4. （可选）CI 加一条扫描产物 CSS 是否出现 `light-dark(` / `allow-discrete` 的防回归检查。

---

### 2.9 plugin-simverse 横竖屏适配（横屏世界锁定 + 竖屏为主形态）

> 探查现状：`SimverseWorld.vue` 仅 native 模式 `lockScreenOrientation("landscape-primary")`；`SimVerseActivity` 仅 `configChanges="orientation|screenSize|keyboardHidden"`，无 `screenOrientation`；世界 HUD 为横屏形态（`.world-map{padding:60px 100px 70px}`、300px side-panel、居中 bottom-bar）。
>
> **两种形态的定位（修正）**：
> - **横屏世界 = landscape-locked 游戏**（仅 simverse 的"世界"页）。它是游戏，**锁定横屏、不存在竖屏形态** —— 无需为竖屏设计世界 UI，正确做法是锁屏而非响应式重排。
> - **竖屏 = 所有应用与插件的主要形态**（encv-mobile 主应用、simverse 除"世界"外的全部页面、openlist/mpv 等插件 web 应用）。portrait-first，需做 **phone + pad 双屏适配**。

#### 2.9.1 横屏世界（landscape-locked game）特殊适配
- **锁定横屏**：进入世界 `lockScreenOrientation("landscape-primary")`（native）；web 端 `Screen Orientation API` 若不支持（桌面/部分 pad）→ 兜底"请旋转设备"遮罩（GSAP 摇摇动画）或 letterbox 缩放，**不强制旋转、不阻塞**。
- **沉浸**：锁定后 `hideSystemUI()`（状态栏/导航栏），离开/unmount 恢复 `showSystemUI()`（现有逻辑保留）。
- **性能档**：横屏自动 `fg_idle`（高帧/高分辨率，复用 `WorldSettings` / `QUALITY_RESOLUTION`）。
- **Phaser 适配**：`Scale.RESIZE`（`RegionScene.ts` `this.scale`），`onResize` 重算相机 `scrollX/Y` 与 NPC/建筑密度，避免尺寸变化越界。
- **`AndroidManifest.xml` 不硬锁 `screenOrientation`**：否则剥夺"竖屏为主形态"。横屏仅由 web 在进入世界时申请、退出时 `unlockOrientation`。
- **无竖屏世界布局**：世界本身只有横屏一套 UI。

#### 2.9.2 竖屏：所有应用/插件的主要形态（phone + pad 双屏）
这是适配重点。主应用与所有插件页面默认 portrait，按设备形态分流：

- **phone 竖屏（窄，<600px）**：单栏流式 + 底部 Tab 条 + 抽屉面板 + `env(safe-area-inset-*)` 处理刘海/圆角。simverse 的非世界页面（`NPCList` / `ChronicleList` / `EconomyOverview` / `QuestView` / `WorldSettings` 等 `.vue`）统一走此形态。
- **pad 竖屏（宽，≥768px）**：master-detail 双栏 —— 左列表/缩略、右详情常驻（`selectNPC` 即右栏更新，复用现有 `focus` 状态），无需抽屉。
- **断点（container queries，Chromium 105+ ✅，见 2.8）**：`≤600px` phone、`601–1023px` pad 竖屏双栏、`≥1024px` 大屏。用 container queries 而非仅 viewport 媒体查询，便于插件内嵌。
- **与 daisyUI 协同**：竖屏布局复用 `.encv-card` / `.encv-panel` 与 `base-*` 令牌，双栏下组件自动重排；列表/网格用 `grid` + `auto-fill/minmax` 自适应列数。

#### 2.9.3 响应式令牌与断点
```ts
// useSimverseLayout.ts：世界恒 landscape；其余页面恒 portrait-*（不随世界切换）
export type FormFactor = "landscape" | "portrait-phone" | "portrait-pad";
export function useSimverseLayout(isWorld: boolean) {
  const formFactor = ref<FormFactor>("landscape");
  function compute() {
    if (isWorld) { formFactor.value = "landscape"; }   // 世界：永远横屏
    else {
      const w = window.innerWidth, h = window.innerHeight;
      formFactor.value = (h >= w && w >= 768) ? "portrait-pad" : "portrait-phone";
    }
    document.documentElement.dataset.formFactor = formFactor.value;
  }
  onMounted(() => { compute(); window.addEventListener("resize", compute); });
  onUnmounted(() => window.removeEventListener("resize", compute));
  return { formFactor };
}
```
```css
:root { --app-gap: 8px; }
[data-form-factor="portrait-phone"] { --app-gap: 6px; }            /* 单栏紧凑 */
[data-form-factor="portrait-pad"]   { /* 双栏由布局容器 grid 控制 */ }
```

#### 2.9.4 兼容性（Chromium 114）
`orientation` 媒体查询、`matchMedia`、`Screen Orientation API`、Fullscreen、`env(safe-area-inset-*)`、`100dvh`（现 `SimverseWorld.css` 已用）、container queries（105+）**114 全部支持**；web 端 `lockOrientation` 不支持时走 2.9.1 兜底。

---

## 3. 技术落地

### 3.1 令牌契约：`src/theme/tokens.css`（新建，单一事实源）

```css
/* 语义令牌默认（亮） */
:root {
  --encv-color-primary: var(--color-primary, #4f8cff);
  --encv-color-bg: var(--color-base-100, #ffffff);
  --encv-color-surface: var(--color-base-200, #f3f4f6);
  --encv-color-text: var(--color-base-content, #1f2937);
  --encv-color-border: var(--color-base-300, #e5e7eb);
  /* 桥接：Ionic 消费 daisyUI 令牌 */
  --ion-color-primary: var(--color-primary);
  --ion-color-primary-content: var(--color-primary-content);
  --ion-background-color: var(--color-base-100);
  --ion-text-color: var(--color-base-content);
  --ion-card-background: var(--color-base-200);
  --ion-item-background: var(--color-base-100);
  /* 特效层 */
  --encv-bg-blur: 12px;
  --encv-vivid-filter: none;
  --encv-color-gamut: srgb;
}
```

主题 `theme.css` 只需覆盖 primitive，例如 `encv-dark/theme.css`：

```css
[data-theme="encv-dark"] {
  --color-primary: #9c6df7;
  --color-base-100: #0f172a;
  --color-base-200: #1e293b;
  --color-base-300: #334155;
  --color-base-content: #e2e8f0;
}
```

### 3.2 重写 `useTheme.ts`（从补丁引擎 → 主题切换 + 叠加层）

- 删除 `hexToRgb / lighter / darker` 手写运算（改由 daisyUI `color-mix` 与 CSS 处理）。
- 新增 `setTheme(id)`（写 `data-theme` + 持久化）、`setPrimaryColor(hex)`（仅覆盖 `--color-primary` + 算 `--color-primary-content`）、`setGradient(colors)`（写 `--encv-bg-gradient` 背景层）、`setBgBlur` / `setVivid` / `setP3`（维持，挂在语义变量上）。
- 启动 `initTheme()`：读持久化 → 应用 `data-theme` + 叠加层变量；保留 P3/vivid 侦测。

### 3.3 daisyUI 全量启用（落地细节见 2.7）

- `encv-mobile/src/main.ts` 追加 `import "@encv/shared-components/styles/daisyui.css";`（与插件 web 应用对齐，统一调色板）。
- 主题采用"双插件"结构（2.7.1）：`@plugin "daisyui"` 列清单、`@plugin "daisyui/theme"` 给令牌；`encv`/`encv-dark` 已就位，新增主题按 2.7.2 全量 19 色 + 2.7.4 形状令牌定义。
- 用户可安装主题经构建期拼接注入 config（2.7.6），运行时只切 `data-theme`。
- 语义色铁律（2.7.3）：禁用 `dark:` 前缀、文本用 `text-base-content`、大面积用 `base-*`。
- **兼容性红线（2.8）**：产物只面向 Chromium 114+，不使用 `light-dark()` / `allow-discrete` / `field-sizing` 等 117/123 特性；encv 主题用 hex（零风险）。
- 业务组件把硬编码 scoped 样式逐步迁到 `.encv-card` / `.encv-panel` 与 `bg-base-*` / `text-base-content` 工具类。

### 3.4 GSAP 动效层：`src/motion/`（动效全集的落地）

每个模块导出一个 Vue composable / 工厂函数，内部读 `MotionProfile`（见 2.6）：

```
src/motion/
  index.ts          # 统一导出 + installMotion()（注册插件/全局）
  guard.ts          # MotionProfile：reduced-motion / vivid / 强度 闸门
  tokens.ts         # ease/dur 常量（与 2.6 CSS 令牌对应）
  transition.ts     # usePageTransition()：路由进入/离开 timeline
  reveal.ts         # useScrollReveal()：scrolltrigger 揭示 + 分节视差
  flip.ts           # useSharedElement()：Flip 共享元素转场
  micro.ts          # usePress()/useHover()/useRipple()/useToggle()/useSegmented()/useCheckbox()
  magnetic.ts       # useMagnetic()：磁性按钮跟手
  overlay.ts        # useDialog()/useSheet()/useDrawer()/useToast()/useTooltip()/useMenu()
  nav.ts            # useTabSwitch()/useFab()/useSearchExpand()/useBackGesture()
  data.ts           # useCountUp()/useProgress()/useShimmer()/useSuccess()/useConfetti()
  ambient.ts        # useThemeIntro()/useAurora()/useSpotlight()/useScramble()
  guide.ts          # useOnboarding()/useFeatureHint()/useEmptyState()/useSplash()
  gesture.ts        # usePullRefresh()/useSwipeBack()/useDragSort()/useSwipeDismiss()
  registry.ts       # 全局动画注册表（命名动画可热禁用 / 性能采样）
```

各模块要点与骨架：

```ts
// guard.ts —— 所有动效的唯一闸门
export interface MotionProfile {
  enabled: boolean;        // reduced-motion => false
  intensity: number;       // vivid/P3 时 1.3，否则 1
  respectsReduced: boolean;
}
export function getMotionProfile(): MotionProfile {
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const vivid = document.documentElement.classList.contains("encv-vivid");
  return { enabled: !reduced, intensity: vivid ? 1.3 : 1, respectsReduced: reduced };
}
// 调用方：若 !profile.enabled 直接 set 终态 return，绝不 gsap.from

// transition.ts —— 路由转场
export function usePageTransition(el: Ref<HTMLElement>) {
  const p = getMotionProfile();
  onMounted(() => {
    if (!p.enabled) return finalState(el);
    gsap.from(el.value, { y: 12 * p.intensity, opacity: 0,
      duration: 0.32, ease: "power2.out" });
  });
}

// reveal.ts —— 滚动揭示 + 视差
export function useScrollReveal(el: Ref<HTMLElement>, opts = {}) {
  const p = getMotionProfile();
  ScrollTrigger.create({
    trigger: el.value, start: "top 90%", once: true,
    onEnter: () => gsap.from(el.value, {
      y: 16 * p.intensity, opacity: 0,
      duration: 0.4, ease: "power2.out",
      stagger: 0.04 * p.intensity, ...opts,
    }),
  });
}

// micro.ts —— 高频微交互收敛到一个工厂
export function useRipple(el: Ref<HTMLElement>) {
  el.value.addEventListener("pointerdown", (e) => {
    if (!getMotionProfile().enabled) return;
    const r = document.createElement("span");
    r.className = "encv-ripple";
    gsap.fromTo(r, { scale: 0, opacity: 0.5 },
      { scale: 1, opacity: 0, duration: 0.5, ease: "power2.out" });
  });
}

// data.ts —— 数字滚动
export function useCountUp(el: Ref<HTMLElement>, to: number, dur = 0.8) {
  const p = getMotionProfile();
  const o = { v: 0 };
  if (!p.enabled) { el.value.textContent = to.toLocaleString(); return; }
  gsap.to(o, { v: to, duration: dur * p.intensity, ease: "power1.out",
    onUpdate: () => (el.value.textContent = Math.round(o.v).toLocaleString()) });
}

// ambient.ts —— 主题光泽过渡
export function useThemeIntro() {
  const p = getMotionProfile();
  if (!p.enabled) return;
  gsap.fromTo(document.documentElement,
    { filter: "brightness(1.15)" },
    { filter: "brightness(1)", duration: 0.5, ease: "power2.out" });
}
```

> 实现前先 `use_skill gsap-core`（核心 API / ease / timeline）、`use_skill gsap-react`（Vue `useGSAP` 生命周期与自动清理）、`use_skill gsap-plugins`（Flip / ScrollTrigger / Draggable 注册）、`use_skill gsap-scrolltrigger`（滚动揭示）、`use_skill gsap-performance`（transform/will-change 优化）。

### 3.5 Appearance UI 演进为「主题 / 主色 / 特效」三段

`AppearanceDetail.vue` 重组为三块（现有控件几乎全部复用）：

1. **主题（Theme）**：列出内置 + 用户安装主题（卡片式，实时预览），对标 Obsidian/思源主题切换。
2. **主色（Accent）**：旧 7 色预设 + 自定义取色器 → 走 `setPrimaryColor`（叠加层）。
3. **特效（Effects）**：背景（亮/护眼/暗/渐变 + 模糊）、vivid、P3 —— 原样保留。

---

## 4. 目标目录结构

```
packages/shared-components/src/
  theme/
    tokens.css              # 语义令牌契约 + Ionic/daisyUI 桥接（新建）
    variables.css           # 旧：迁移后仅保留兜底/兼容，逐步废弃
    themes/
      encv/theme.json + theme.css [+ theme.js]
      encv-dark/theme.json + theme.css [+ theme.js]
      <user-installed>/…    # 用户可安装主题（集市式）
    snippets/               # Obsidian 式局部覆盖片段
    registry.ts             # 内置 + 用户主题注册表（扫描清单）
  motion/                   # GSAP 动效层（新建，见 3.4 全集）
    index.ts guard.ts tokens.ts
    transition.ts reveal.ts flip.ts micro.ts magnetic.ts
    overlay.ts nav.ts data.ts ambient.ts guide.ts gesture.ts registry.ts
  composables/
    useTheme.ts             # 重写为：切 data-theme + 叠加层（保留 P3/vivid/模糊/渐变）
  styles/
    daisyui.css             # 主题从 themes/ 注册；保留 .encv-* 共享类
```

---

## 5. 分阶段迁移计划

> 每阶段结束用 `app_check_all`（lint + typecheck + i18n + build）门禁，确保零回归。

**Phase 0 — 地基**
- 新增 `gsap` 依赖（shared-components `package.json`）；确认 `@tailwindcss/vite` + `daisyui` 已在（已是，见 2.7）。
- 新建 `src/theme/tokens.css` 与 `src/motion/`。
- 兼容性基线（2.8）：在共享包 `package.json` 加 `"browserslist": ["Chrome >= 114"]`；`vite-plugins/daisy-ui.ts` 维持 `@tailwindcss/vite`，不引入降级插件。可选收紧 `build.target: 'chrome114'`。
- 风险：无。验证：`app_check_all`。

**Phase 1 — 统一 daisyUI + 桥接令牌**
- `encv-mobile/src/main.ts` 引入 `daisyui.css`；`tokens.css` 桥接 Ionic←daisyUI。
- 把 `variables.css` 中 Ionic 变量改为从 `tokens.css` 别名（兼容旧组件不破）。
- 风险：主应用主色由蓝变紫（daisyUI encv 主题），属预期"焕新"；若需保持蓝，调 `encv` 主题 `--color-primary`。
- 验证：`app_check_all` + 人工核对暗/亮。

**Phase 2 — 重写主题引擎**
- 重写 `useTheme.ts`：`setTheme(data-theme)` + `setPrimaryColor`（仅覆盖 `--color-primary`）+ 叠加层变量；删 hex 手写运算。
- 内置主题落地为 `themes/encv`、`themes/encv-dark` 的 `theme.json`+`theme.css`；`registry.ts` 注册。
- 风险：Appearance UI 需适配新 API（微调调用）。验证：`typecheck` + `AppearanceDetail` 交互。

**Phase 3 — GSAP 动效全集**
- 落地 `src/motion/` 全部模块：`transition`(路由转场) / `reveal`(滚动揭示+视差) / `flip`(共享元素) / `micro`(按压/悬浮/波纹/开关/分段/复选) / `magnetic`(磁性按钮) / `overlay`(弹窗/BottomSheet/Drawer/Toast/Tooltip/菜单) / `nav`(Tab/FAB/搜索展开/返回手势) / `data`(数字滚动/进度/骨架/成功对勾/Confetti) / `ambient`(主题光泽/Aurora/光标辉光/文字scramble) / `guide`(Onboarding/高亮/空态/首屏) / `gesture`(下拉刷新/侧滑返回/拖拽排序/滑动消除)。
- `guard.ts` + `tokens.ts` 统一闸门（reduced-motion 落终态、vivid/P3 调强度），`registry.ts` 全局注册可热禁用。
- 风险：动画性能/回归面最大；先挑 3–4 个高频视图（列表/详情/设置/弹窗）试点，用 `prefers-reduced-motion` 真机 QA。验证：构建 + 模拟器手感 QA + `app_check_all`。

**Phase 4 — 组件迁移与 Appearance 重组**
- 共享组件 scattered scoped 样式迁到 `.encv-*` / `bg-base-*` 工具类（搜索 `--ion-color-primary` 等约 96+ 处，批量替换 + 抽查）。
- `AppearanceDetail.vue` 重组为「主题/主色/特效」三段；新增用户主题列表 + Snippets 开关。
- 风险：视觉回归面最大；逐视图核对。验证：`app_check_all` + 截图比对。

**Phase 5 — 可扩展性闭环**
- 用户主题加载/卸载 API（从 `user-themes/` + 远程清单）、Snippets 热开关。
- 文档：主题开发指南（写 `theme.json` + `theme.css` + 可选 `theme.js`）。

---

## 6. 参考对照

| 能力 | Obsidian | 思源笔记（SiYuan） | 本方案 |
| --- | --- | --- | --- |
| 主题形态 | `manifest.json` + `theme.css` | `theme.json` + `theme.css`(+`theme.less`) + 可选 `theme.js` | 同思源：`theme.json`+`theme.css`+可选 `theme.js` |
| 换肤机制 | 覆盖 `:root`/`.theme-dark` 变量，运行时 `setConfig('cssTheme')` | `appearance/themes/<name>`，`mount/unmount` | `data-theme` 切换 + `theme.js` 钩子 |
| 局部覆盖 | CSS Snippets | 片段 | `src/theme/snippets/*` |
| 扩展分发 | 社区主题市场 | 集市（Bazaar） | `user-themes/` + 注册表 |
| 暗色 | `.theme-dark` / 系统 | `mode: dark` | `encv-dark`（`--prefersdark`） |

**官方用法参考（本项目已注册 skill）**：实现 Phase 3 动效前先 `use_skill gsap-core`（核心 API）、`use_skill gsap-react`（Vue 生命周期/清理）、`use_skill gsap-scrolltrigger`（滚动揭示）；daisyUI 主题/令牌写法参考 `use_skill daisyui`。

---

## 7. 验收清单

- [ ] 主应用 `encv-mobile` 与插件 web 应用主色/暗亮**一致**（单一调色板）。
- [ ] 内置 `encv` / `encv-dark` 可运行时切换，Ionic + daisyUI + 业务组件同步换肤。
- [ ] 旧 7 色预设 + 渐变 + 模糊 + vivid + P3 **全部保留可用**。
- [ ] 用户可安装主题 + Snippets 局部覆盖可加载/卸载。
- [ ] GSAP 动效全集（路由转场/滚动揭示/微交互/浮层/导航/数据反馈/氛围/引导/手势）生效，且 `reduced-motion` 下直接落终态、vivid/P3 下增强。
- [ ] `app_check_all` 全绿（Biome / typecheck / i18n / 构建）。
- [ ] 产物 CSS 仅面向 Chromium 114+：无 `light-dark(` / `allow-discrete` / `field-sizing` 等越基线特性（兼容性红线 2.8）。
