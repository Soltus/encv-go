# SimVerse 游戏 UI 彻底重构 Spec

## Why

plugin-simverse 横屏世界当前是「一个 HUD 所有界面全弹窗」的堆叠结构：NPC/编年史/经济/任务/抽卡/干预/化身等所有功能都挤在 `SimverseWorld.vue` 单页面内以侧边面板/模态弹窗展示，界面切换体验差，「怎么改配色都是垃圾」。样式层全是纯 CSS + 自造 `.sv-*` 类系统（共 ~4000 行），未落地工作区 daisyUI v5 + SCSS 样式栈；动效全是 CSS `@keyframes` + Vue `<transition>`，缺乏游戏级时序动效；Phaser 画布内部渲染与 UI 割裂。

要让 simverse「彻底像个正经游戏」，必须**先重构界面切换架构**（拆掉全弹窗 HUD），再迁移样式栈、引入 GSAP 动效、改造 Phaser 视觉与镜头。配色匹配 encv-mobile 主题（现有紫色系 daisyUI 令牌），不自创调色板。

## What Changes

### 架构层
- **拆掉全弹窗 HUD**：`SimverseWorld.vue` 内的侧边面板/模态弹窗（NPC/编年史/经济/任务/组织/训练/背包/抽卡/干预/化身）改为**混合架构**：
  - **HUD 场景**（单页面，GSAP Flip 驱动）：主世界（Phaser + 顶栏资源条 + 底部操作条）、NPC 焦点、抽卡、干预、化身等「轻交互」场景在单页面内用 GSAP Flip 做场景过渡（面板→全屏变形）
  - **详情路由**（vue-router 导航）：NPC 详情、编年史详情、经济详情、组织详情、区域详情等「深度页」拆为独立路由，用 GSAP 做页面过渡
- 屏幕状态机从 `world/focus/event/intervene/character` 扩展为更细粒度的场景状态，由 GSAP Flip 驱动过渡

### 样式层
- **废弃 `.sv-*` 自造类系统**：[theme/simverse.css](file:///workspace/app/encv-mobile/plugin-simverse/web/src/theme/simverse.css)（253 行）整体废弃，功能由 daisyUI 组件类（`.card/.btn/.badge/.modal/.tabs/.menu`）+ 共享 [surface.scss](file:///workspace/app/packages/shared-components/src/theme/surface.scss) 的 `.ui-*` 语义类承接
- **SimverseWorld.css → 模块化 SCSS**：3741 行纯 CSS 重写为模块化 SCSS（按场景拆分：`_hud.scss`/`_top-bar.scss`/`_bottom-bar.scss`/`_side-panel.scss`/`_gacha.scss`/`_event.scss`/`_intervene.scss` 等），用 SCSS 变量/mixins + daisyUI 令牌（`var(--color-primary)` 等）
- **配色匹配 encv-mobile 主题**：不自创赛博朋克调色板，沿用工作区 [daisyui.css](file:///workspace/app/packages/shared-components/src/styles/daisyui.css) 的 `encv`/`encv-dark` 主题令牌；视觉风格靠霓虹辉光/玻璃拟态/扫描线等质感实现，不靠换色
- **整个 plugin-simverse 全部迁移**：40+ 个 `.vue` 视图全部从纯 CSS / `.sv-*` 类迁移到 daisyUI + `.ui-*` 语义类

### 动效层
- **GSAP 全量替换**：所有 CSS `@keyframes`（`bgDrift`/`valuePop`/`runPulse`/`pulse`/`float`/`spin`）和 Vue `<transition>`（`bottom-bar`/`more-pop`/`gacha-modal`/`gacha-flash`/`event-page`/`intervene-page`/`character-page`/`ticker`）改用 GSAP timeline
- **引入 GSAP 插件**：
  - `ScrollTrigger`：侧边面板滚动触发、编年史时间轴滚动动效
  - `Flip`：HUD 场景过渡（面板→全屏变形）、屏幕状态机切换
  - `MotionPath`：NPC 标记移动、抽卡光柱轨迹、资源数字飞入
- **新增游戏级动效**：镜头推进、粒子效果、连击数字、屏幕震动、抽卡光柱
- **简单 hover/active 保留 CSS**：交互态反馈不强制 GSAP

### Phaser 层
- **视觉质感**：`game/*.ts` 中 NPC sprite 配色、地形色调、建筑样式调整为与 encv-mobile 紫色主题一致的质感
- **镜头动效**：`game/*.ts` 的 camera 逻辑加入 GSAP 驱动的镜头缓动（场景切换时推近/拉远、NPC 选中时 centerOn 缓动）

### 交付层
- **分阶段交付**：
  - P0：横屏世界骨架重构（混合架构落地 + GSAP 基础设施）
  - P1：横屏世界视觉与动效（SCSS 模块化 + GSAP 全量动效）
  - P2：首页与 Tabs
  - P3：NPC/编年史系列
  - P4：经济/区域/设置
  - P5：Phaser 视觉与镜头

## Impact

- **Affected code**:
  - [plugin-simverse/web/src/](file:///workspace/app/encv-mobile/plugin-simverse/web/src/) 全部（40+ 视图 + 主题 + composables）
  - [plugin-simverse/web/src/game/](file:///workspace/app/encv-mobile/plugin-simverse/web/src/game/) 12 个 Phaser 场景文件（P5 视觉与镜头）
  - [plugin-simverse/web/package.json](file:///workspace/app/encv-mobile/plugin-simverse/web/package.json)（新增 gsap 依赖）
  - [plugin-simverse/web/src/router/index.ts](file:///workspace/app/encv-mobile/plugin-simverse/web/src/router/index.ts)（新增详情路由）
- **Affected specs**: 无直接关联 spec
- **BREAKING**:
  - `.sv-*` 类名系统废弃，所有模板类名重写
  - `SimverseWorld.css` 废弃，改为模块化 SCSS
  - `theme/simverse.css` 废弃
  - 横屏世界屏幕状态机结构变更（扩展场景状态）
  - 部分原侧边面板功能改为路由页面

## ADDED Requirements

### Requirement: 混合架构界面切换

系统 SHALL 在横屏世界内采用「HUD 场景 + 详情路由」混合架构：
- HUD 场景（单页面，GSAP Flip 驱动）：主世界、NPC 焦点、抽卡、干预、化身等轻交互场景
- 详情路由（vue-router）：NPC 详情、编年史详情、经济详情、组织详情、区域详情等深度页

#### Scenario: HUD 场景过渡
- **WHEN** 用户在主世界点击 NPC 焦点按钮
- **THEN** 侧边面板通过 GSAP Flip 变形为全屏角色焦点场景，无弹窗感
- **AND** 场景过渡有镜头推进感（Phaser camera 缓动 + UI 元素 Flip）

#### Scenario: 详情路由导航
- **WHEN** 用户在 NPC 焦点场景点击「查看详情」
- **THEN** 导航到 `/world/npc/:id` 独立路由页面
- **AND** 页面过渡用 GSAP 驱动（非默认 vue-router 过渡）

#### Scenario: 返回主世界
- **WHEN** 用户从详情页或焦点场景返回主世界
- **THEN** GSAP Flip 反向播放，场景平滑回退
- **AND** Phaser camera 拉远恢复全局视角

### Requirement: daisyUI + SCSS 样式栈

系统 SHALL 将整个 plugin-simverse 的样式迁移到 daisyUI v5 组件类 + 共享 surface.scss 的 `.ui-*` 语义类，废弃 `.sv-*` 自造类。

#### Scenario: 样式类使用
- **WHEN** 渲染任意 plugin-simverse 视图
- **THEN** 模板使用 daisyUI 组件类（`.card/.btn/.badge/.modal/.tabs/.menu`）或 `.ui-*` 语义类
- **AND** 不再使用 `.sv-card/.sv-btn/.sv-badge` 等自造类

#### Scenario: 配色一致性
- **WHEN** encv-mobile 主题切换（亮/暗）
- **THEN** plugin-simverse 所有视图跟随切换
- **AND** 使用工作区 daisyUI `encv`/`encv-dark` 主题令牌

### Requirement: GSAP 动效系统

系统 SHALL 用 GSAP（含 ScrollTrigger/Flip/MotionPath）替换所有 CSS `@keyframes` 和 Vue `<transition>` 动效。

#### Scenario: 抽卡翻牌动效
- **WHEN** 用户触发抽卡
- **THEN** GSAP timeline 驱动卡牌翻转、光柱、粒子、屏幕震动
- **AND** 使用 MotionPath 控制光柱轨迹

#### Scenario: 面板场景过渡
- **WHEN** 侧边面板变形为全屏场景
- **THEN** GSAP Flip 记录 first/last 状态并平滑过渡
- **AND** 过渡期间 Phaser camera 同步缓动

#### Scenario: 数值跳动
- **WHEN** 资源数值变化（钻石/金币/体力）
- **THEN** GSAP 驱动数字滚动 + 缩放反馈
- **AND** 不再使用 CSS `valuePop` keyframe

#### Scenario: 滚动触发
- **WHEN** 侧边面板或编年史时间轴滚动
- **THEN** ScrollTrigger 触发元素入场动效

### Requirement: Phaser 视觉与镜头动效

系统 SHALL 改造 `game/*.ts` 的 NPC sprite 配色、地形色调、建筑样式，并加入 GSAP 驱动的镜头缓动。

#### Scenario: NPC 选中镜头推近
- **WHEN** 用户选中某 NPC
- **THEN** Phaser camera 用 GSAP 缓动推近到该 NPC
- **AND** UI 侧同步触发焦点场景过渡

#### Scenario: 视觉质感一致性
- **WHEN** 渲染 Phaser 画布
- **THEN** NPC sprite 配色、地形色调、建筑样式与 encv-mobile 紫色主题质感一致
- **AND** 不出现 UI 与画布视觉割裂

## MODIFIED Requirements

### Requirement: SimverseWorld.vue 结构

`SimverseWorld.vue`（1821 行）从「单页面 + 全弹窗 HUD」重构为「混合架构 HUD 场景容器」：
- 保留：Phaser 容器、顶栏资源条、底部主操作条、屏幕状态机
- 改造：侧边面板/模态弹窗 → GSAP Flip 场景过渡
- 拆出：深度页 → 独立路由
- 屏幕状态机扩展更细粒度场景状态

### Requirement: 路由结构

[router/index.ts](file:///workspace/app/encv-mobile/plugin-simverse/web/src/router/index.ts) 新增/调整路由：
- 保留现有 `/world/npc/:id`、`/world/org/:id`、`/world/chronicles` 等
- 新增从原侧边面板拆出的深度页路由
- 路由过渡改用 GSAP 驱动

### Requirement: 依赖配置

[package.json](file:///workspace/app/encv-mobile/plugin-simverse/web/package.json) 新增：
- `gsap`（catalog 引用，`^3.13.0`）
- 确认 `sass` 已可用（工作区已有）

## REMOVED Requirements

### Requirement: `.sv-*` 自造类系统
**Reason**: 与 daisyUI 组件类 + `.ui-*` 语义类功能重叠，维护两套类系统成本高，且 `.sv-*` 未接入工作区主题令牌
**Migration**: 所有 `.sv-card`→`.card`/`.ui-card`，`.sv-btn`→`.btn`/`.ui-button`，`.sv-badge`→`.badge`/`.ui-badge`，`.sv-grid`→Tailwind grid 工具类

### Requirement: 纯 CSS `@keyframes` 动效
**Reason**: 缺乏时序控制、无法与场景过渡联动、维护分散
**Migration**: `bgDrift`→GSAP timeline，`valuePop`→GSAP 数值动效，`runPulse`→GSAP 状态脉冲，`pulse`/`float`/`spin`→GSAP 循环动效

### Requirement: `SimverseWorld.css` 单文件
**Reason**: 3741 行单文件不可维护，无法模块化复用
**Migration**: 拆分为 `SimverseWorld.scss` + 按场景模块化 `_hud.scss`/`_top-bar.scss`/`_bottom-bar.scss`/`_side-panel.scss`/`_gacha.scss`/`_event.scss`/`_intervene.scss` 等

### Requirement: `theme/simverse.css`
**Reason**: 自造设计令牌与 daisyUI 主题令牌重复
**Migration**: 功能并入 daisyUI 主题令牌 + SCSS 变量
