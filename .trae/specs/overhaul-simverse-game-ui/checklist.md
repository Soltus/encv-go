# SimVerse 游戏 UI 彻底重构 Checklist

> 验收清单：每条必须可观察、可验证。子任务对应 `tasks.md` 的 P0–P5。

## P0 骨架重构与 GSAP 基础设施

- [x] `plugin-simverse/web/package.json` 的 `dependencies` 中包含 `"gsap": "catalog:"`，`pnpm install` 后 `node_modules/gsap` 存在
- [x] `src/composables/useGsap.ts` 存在，导出 `gsap`/`ScrollTrigger`/`Flip`/`MotionPath` 的安全引用，插件注册幂等（重复调用不报错）
- [x] `src/composables/useSceneTransition.ts` 存在，导出 `recordState`/`playTransition`/`reverseTransition`/`transitionToScene` 方法
- [x] `src/composables/useRouteTransition.ts` 存在，导出 `onEnter`/`onLeave` 用于 vue-router 过渡
- [x] `SimverseWorld.vue` 屏幕状态机已扩展（world/focus/event/intervene/character/gacha），能区分「HUD 场景」vs「详情路由」两类目标
- [x] HUD 场景（focus/gacha/intervene/character/event）使用 `useSceneTransition` 驱动过渡，无 `<ion-modal>`/侧边弹窗 DOM
- [x] 详情页通过 `router.push` 导航到独立路由（npc/chronicles/economy/quest/org/training/inventory/explore/battle/profile/settings），不挤在 SimverseWorld 单页面内
- [x] `router/index.ts` 中新增的 7 条详情路由均带 `meta: { transition: "gsap" }` 标记，RouteMeta 类型已扩展
- [x] `src/views/simverse-world/` 目录存在，包含 `_tokens.scss`/`_mixins.scss`/`SimverseWorld.scss` 三个基础文件 + 10 个场景模块
- [x] `_tokens.scss` 将 daisyUI 令牌映射为 SCSS 变量（如 `$color-primary: var(--color-primary)`），并派生横屏世界专属语义令牌
- [x] `_mixins.scss` 至少包含玻璃拟态（glass-panel）、霓虹辉光（neon-glow）、扫描线（scanlines）等 6 个 mixin
- [x] `SimverseWorld.vue` 改用 `<style scoped lang="scss">` 并 `@use` 模块化 SCSS，能通过 vite 编译

## P1 视觉与动效

- [x] 原 `SimverseWorld.css`（3741 行）已删除，功能拆分到 `simverse-world/` 下至少 10 个模块化 SCSS 文件（`_hud`/`_top-bar`/`_bottom-bar`/`_side-panel`/`_gacha`/`_event`/`_intervene`/`_character`/`_focus`/`_common`）
- [x] `SimverseWorld.vue` 模板中所有自定义类替换为 daisyUI 组件类 / `.ui-*` 语义类 / Tailwind 工具类
- [x] 模板中无任何 `.sv-card`/`.sv-btn`/`.sv-badge`/`.sv-grid` 等 `.sv-*` 类引用
- [x] 所有 CSS `@keyframes`（`bgDrift`/`valuePop`/`runPulse`/`pulse`/`float`/`spin`）已删除，功能由 GSAP timeline 替代
- [x] 所有 Vue `<transition>`（`bottom-bar`/`more-pop`/`gacha-modal`/`gacha-flash`/`event-page`/`intervene-page`/`character-page`/`ticker`）改为 GSAP 驱动的 v-show/v-if
- [x] 抽卡场景使用 `MotionPath` 控制光柱轨迹，至少 1 条自定义路径动效
- [x] 资源数值变化时有 GSAP 驱动的滚动 + 缩放反馈（不再用 `valuePop` keyframe）
- [x] 主世界背景有 GSAP 驱动的缓动（不再用 `bgDrift` keyframe）
- [x] 屏幕震动效果存在（gsap.to body shake 或等效实现）
- [ ] 配色完全沿用工作区 daisyUI `encv`/`encv-dark` 主题令牌，未引入新的色值变量
- [ ] 主题切换（亮/暗）时 SimverseWorld 视图跟随切换无样式断层

## P2 首页与 Tabs

- [x] `SimverseHome.vue` 改用 `<style scoped lang="scss">`，自定义类迁移到 daisyUI + `.ui-*`
- [x] `SimverseHome.vue` 的 `float` 动效改用 GSAP，无 `@keyframes float`
- [x] `Tabs.vue` 的 tab-bar 样式使用 daisyUI 令牌调色，Ionic `ion-tabs` 组件保留
- [x] 首页与 Tabs 视觉与改造前功能等价，无交互回归

## P3 NPC/编年史系列

- [x] `NPCList.vue`/`NPCDetail.vue`/`NPCRelations.vue`/`NPCBehavior.vue`/`NPCTimeline.vue`/`NPCInventory.vue` 全部 SCSS 化
- [x] 上述 6 个视图中 `.sv-*` 类引用全部清除
- [x] NPC 列表入场动效用 `ScrollTrigger` 驱动
- [x] `ChronicleList.vue`/`ChronicleDetail.vue`/`ChronicleCausal.vue`/`EraOverview.vue` 全部 SCSS 化
- [x] 编年史时间轴滚动用 `ScrollTrigger` 触发入场动效
- [x] `WorldNPCDetail.vue`/`WorldChronicles.vue`/`WorldOrgDetail.vue` 作为详情路由，使用 `useRouteTransition` 驱动过渡

## P4 经济/区域/设置

- [x] `EconomyOverview.vue`/`EconomyPrices.vue`/`EconomyTrade.vue`/`WorldEconomy.vue` 全部 SCSS 化 + daisyUI 类迁移
- [x] 经济条形图/财富榜动效用 GSAP 驱动
- [x] `RegionList.vue`/`RegionDetail.vue`/`OrgList.vue`/`OrgDetail.vue`/`OrgMembers.vue`/`OrgTerritory.vue`/`WorldMapView.vue` 全部 SCSS 化 + daisyUI 类迁移
- [x] `SimverseSettings.vue`/`WorldSettings.vue`/`PerformanceSettings.vue`/`SimulationSettings.vue`/`SaveManagement.vue`/`AboutSimverse.vue` 全部 SCSS 化 + daisyUI 类迁移
- [x] `QuestView.vue`/`SocialOverview.vue`/`SquadSynergy.vue`/`WorldIntervention.vue`/`WorldBehavior.vue`/`WorldDebugPerf.vue`/`WorldDebugEntities.vue`/`SimverseDevLogs.vue` 全部 SCSS 化 + daisyUI 类迁移
- [x] 全仓库搜索 `.sv-` 类引用为 0（`grep -r "\.sv-" src/` 无结果）
- [x] `theme/simverse.css` 文件已删除
- [x] `main.ts` 中 `import "./theme/simverse.css"` 语句已移除

## P5 Phaser 视觉与镜头

- [x] `game/NPCSprite.ts` 中 NPC 配色与 encv-mobile 紫色主题质感一致
- [x] `game/TerrainGenerator.ts` 中地形色调与主题协调
- [x] `game/BuildingSprite.ts` 中建筑样式视觉升级
- [x] `game/TerritoryRenderer.ts`/`game/RegionScene.ts` 中区域渲染配色与主题一致
- [x] `game/WorldScene.ts` 中 camera 推近/拉远改用 GSAP 缓动（非 Phaser 默认 tween）
- [x] NPC 选中时 `centerOn` 由 GSAP 驱动，与 UI 侧 `useSceneTransition` 同步触发
- [x] `game/DayNightCycle.ts`/`game/EventEffectManager.ts`/`game/MiniMap.ts` 配色与主题协调
- [ ] UI 与画布无视觉割裂（截图对比验证）

## 全局质量门

- [x] `pnpm --filter simverse-web build` 构建通过（exit 0，2.07s，无 TypeScript / SCSS 编译错误）
- [x] `pnpm --filter simverse-web typecheck`（`vue-tsc --noEmit`）通过 — 本 package 无独立 lint 脚本，typecheck 即类型层 lint
- [ ] 横屏世界在浏览器与 Android WebView 中均能正常加载、场景切换无白屏/卡死
- [ ] 浏览器控制台无 GSAP/Phaser/Vue 相关报错
- [ ] 主题切换（亮↔暗）所有视图跟随切换，无遗漏
- [x] 全仓库 `@keyframes` 数量较改造前显著减少（仅保留必要的纯装饰动画）
- [x] 全仓库 `.sv-` 类引用为 0
- [x] 全仓库 `SimverseWorld.css` 文件不存在，所有横屏世界样式来自 `simverse-world/` 模块化 SCSS
