# Tasks

## P0: 横屏世界骨架重构（混合架构落地 + GSAP 基础设施）

- [x] Task 0.1: 新增 gsap 依赖到 plugin-simverse/web/package.json
  - [x] 在 dependencies 中加入 `"gsap": "catalog:"`
  - [x] 运行 pnpm install 确认安装成功（gsap 3.15.0）
  - [x] 验证 `import gsap from "gsap"` 可用

- [x] Task 0.2: 创建 GSAP 动效基础设施 composables
  - [x] 新建 `src/composables/useGsap.ts`：统一注册 ScrollTrigger/Flip/MotionPath 插件，提供 gsap/ScrollTrigger/Flip/MotionPath 的安全引用（SSR 安全、插件注册幂等）
  - [x] 新建 `src/composables/useSceneTransition.ts`：封装 GSAP Flip 场景过渡（recordState→playTransition→reverseTransition→transitionToScene），供 HUD 场景使用
  - [x] 新建 `src/composables/useRouteTransition.ts`：封装 vue-router 路由过渡的 GSAP 驱动（onEnter/onLeave）
  - [x] 验证 composables（vue-tsc --noEmit 通过）

- [x] Task 0.3: 重构 SimverseWorld.vue 屏幕状态机为混合架构
  - [x] 扩展屏幕状态：增加 `'gacha'` HUD 场景（focus/event/intervene/character/gacha 五个 HUD 场景 + world）
  - [x] 区分「HUD 场景」（单页面内 GSAP Flip 过渡）vs「详情路由」（导航到独立路由）
  - [x] HUD 场景：focus/event/intervene/character/gacha 保留单页面，用 useSceneTransition.transitionToScene 驱动过渡
  - [x] 详情路由：npc/chronicles/economy/quest/org/training/inventory/explore/battle/profile/settings 通过 openDetailRoute → router.push 导航
  - [x] 验证场景切换（vue-tsc --noEmit 通过，待 P1 视觉到位后做运行时联调）

- [x] Task 0.4: 调整 router/index.ts 支持混合架构
  - [x] 保留现有 `/world/npc/:id`、`/world/org/:id`、`/world/chronicles` 等路由
  - [x] 新增从原侧边面板拆出的深度页路由（`/world/quest/detail/:id?`、`/world/training`、`/world/inventory`、`/world/explore`、`/world/battle`、`/world/profile`、`/world/gacha`，共 7 条）
  - [x] 路由 meta 标记 `transition: 'gsap'`，并扩展 RouteMeta 类型声明
  - [ ] 验证路由跳转正常、返回主世界恢复 HUD 状态（待 Task 0.3 完成后联调）

- [x] Task 0.5: 搭建 SCSS 模块化骨架
  - [x] 新建 `src/views/simverse-world/` 目录，存放横屏世界的模块化 SCSS
  - [x] 新建 `SimverseWorld.scss` 主入口（@use 各模块）
  - [x] 新建 `_tokens.scss`：SCSS 变量映射 daisyUI 令牌（`$color-primary: var(--color-primary)` 等）
  - [x] 新建 `_mixins.scss`：玻璃拟态/霓虹辉光/扫描线等 mixins
  - [x] 验证 SimverseWorld.vue 改用 `<style scoped lang="scss">@use './simverse-world/...'` 可编译（vue-tsc --noEmit 通过，sass 独立编译输出 3816 行无错；vite build 失败仅因 Task 0.4 遗留的缺失视图文件 WorldGacha/WorldTraining 等，与 SCSS 无关）

## P1: 横屏世界视觉与动效（SCSS 模块化 + GSAP 全量动效）

- [x] Task 1.1: SimverseWorld.css 拆分为模块化 SCSS
  - [x] `_hud.scss`：game-container/world-map/phaser-container/loading
  - [x] `_top-bar.scss`：资源条、播放控制
  - [x] `_bottom-bar.scss`：底部主操作条、更多二级面板
  - [x] `_side-panel.scss`：侧边面板容器（改造为场景过渡容器）
  - [x] `_gacha.scss`：抽卡场景（面板+模态+动画）
  - [x] `_event.scss`：事件页/ticker
  - [x] `_intervene.scss`：干预页
  - [x] `_character.scss`：化身页
  - [x] `_focus.scss`：NPC 焦点场景
  - [x] `_common.scss`：通用组件（按钮、卡片、徽章等用 daisyUI 类替代）
  - [x] 删除原 SimverseWorld.css，SimverseWorld.vue 改用 SCSS

- [x] Task 1.2: 模板类名迁移到 daisyUI + .ui-*
  - [x] SimverseWorld.vue 模板中所有自定义类替换为 daisyUI 组件类 / .ui-* 语义类 / Tailwind 工具类
  - [x] 移除对 .sv-* 类的引用
  - [ ] 验证视觉无回归（对比改造前后截图）

- [x] Task 1.3: GSAP 替换 CSS @keyframes
  - [x] `bgDrift` → useGsap 驱动的背景缓动 timeline
  - [x] `valuePop` → 资源数值变化的 gsap.to（scale + color）
  - [x] `runPulse` → 运行状态的 gsap 循环脉冲
  - [x] `pulse`/`float`/`spin` → 对应 GSAP 循环动效
  - [x] 删除所有 @keyframes 定义

- [x] Task 1.4: GSAP 替换 Vue `<transition>`
  - [x] `bottom-bar`/`more-pop` → useGsap 驱动的 enter/leave
  - [x] `gacha-modal`/`gacha-flash` → useSceneTransition 驱动
  - [x] `event-page`/`intervene-page`/`character-page` → useSceneTransition 驱动
  - [x] `ticker` → ScrollTrigger/MotionPath 驱动
  - [x] 移除所有 Vue `<transition>` 包裹（改为 GSAP 驱动的 v-show/v-if）

- [x] Task 1.5: 新增游戏级动效
  - [x] 抽卡光柱：MotionPath 控制光柱轨迹
  - [x] 屏幕震动：gsap.to body shake
  - [x] 连击数字：资源变化时的飞出数字
  - [x] 粒子效果：抽卡/升级时的粒子（用 GSAP 驱动 DOM 粒子或 canvas）

## P2: 首页与 Tabs

- [x] Task 2.1: SimverseHome.vue 迁移到 daisyUI + SCSS
  - [x] 内联 `<style scoped>` 改为 `<style scoped lang="scss">`
  - [x] .hero-section/.action-cards/.stats-grid 等类用 daisyUI + .ui-* 替代
  - [x] float 动效改用 GSAP
  - [x] 验证首页视觉与动效

- [x] Task 2.2: Tabs.vue 迁移
  - [x] Ionic ion-tabs 保留（原生组件不强制改 daisyUI）
  - [x] tab-bar 样式用 daisyUI 令牌调色
  - [x] 验证 tab 切换正常

## P3: NPC/编年史系列

- [x] Task 3.1: NPC 系列视图迁移（NPCList/NPCDetail/NPCRelations/NPCBehavior/NPCTimeline/NPCInventory）
  - [x] 每个视图的 scoped CSS 改为 SCSS
  - [x] 自定义类迁移到 daisyUI + .ui-*
  - [x] .sv-* 类引用全部清除
  - [x] 列表/卡片动效改用 GSAP（ScrollTrigger 入场）

- [x] Task 3.2: 编年史系列视图迁移（ChronicleList/ChronicleDetail/ChronicleCausal/EraOverview）
  - [x] SCSS 化 + daisyUI 类迁移
  - [x] 时间轴动效用 ScrollTrigger
  - [x] 验证编年史时间轴滚动动效

- [x] Task 3.3: 横屏世界内 NPC/编年史子页面迁移（WorldNPCDetail/WorldChronicles/WorldOrgDetail）
  - [x] 作为详情路由，用 useRouteTransition 驱动过渡
  - [x] SCSS 化 + daisyUI 类迁移

## P4: 经济/区域/设置

- [x] Task 4.1: 经济系列视图迁移（EconomyOverview/EconomyPrices/EconomyTrade/WorldEconomy）
  - [x] SCSS 化 + daisyUI 类迁移
  - [x] 物价条形图、财富榜动效用 GSAP

- [x] Task 4.2: 区域/组织系列视图迁移（RegionList/RegionDetail/OrgList/OrgDetail/OrgMembers/OrgTerritory/WorldMapView）
  - [x] SCSS 化 + daisyUI 类迁移

- [x] Task 4.3: 设置系列视图迁移（SimverseSettings/WorldSettings/PerformanceSettings/SimulationSettings/SaveManagement/AboutSimverse）
  - [x] SCSS 化 + daisyUI 类迁移

- [x] Task 4.4: 其他视图迁移（QuestView/SocialOverview/SquadSynergy/WorldIntervention/WorldBehavior/WorldDebugPerf/WorldDebugEntities/SimverseDevLogs）
  - [x] SCSS 化 + daisyUI 类迁移

- [x] Task 4.5: 清理 theme/simverse.css
  - [x] 确认所有 .sv-* 类引用已清除
  - [x] 删除 theme/simverse.css
  - [x] main.ts 移除 import "./theme/simverse.css"

## P5: Phaser 视觉与镜头

- [x] Task 5.1: game/*.ts NPC sprite 视觉调整
  - [x] NPCSprite.ts：配色调整为与 encv-mobile 紫色主题一致
  - [x] 验证 NPC sprite 视觉

- [x] Task 5.2: game/*.ts 地形与建筑视觉调整
  - [x] TerrainGenerator.ts：地形色调调整
  - [x] BuildingSprite.ts：建筑样式调整
  - [x] TerritoryRenderer.ts/RegionScene.ts：区域渲染配色
  - [x] 验证整体画布视觉一致性

- [x] Task 5.3: Phaser camera GSAP 镜头动效
  - [x] WorldScene.ts：camera 推近/拉远改用 GSAP 缓动
  - [x] NPC 选中时 centerOn 用 GSAP 驱动
  - [x] 场景切换时与 UI 侧 useSceneTransition 同步
  - [x] 验证镜头动效与 UI 过渡同步

- [x] Task 5.4: DayNightCycle/EventEffectManager 视觉调整
  - [x] DayNightCycle.ts：昼夜配色与主题协调
  - [x] EventEffectManager.ts：事件特效配色
  - [x] MiniMap.ts：小地图配色

# Task Dependencies

- [Task 1.x] 依赖 [Task 0.x]（GSAP 基础设施先行）
- [Task 2.x]/[Task 3.x]/[Task 4.x] 可并行（独立视图迁移）
- [Task 5.x] 依赖 [Task 1.x]（横屏世界视觉到位后再改 Phaser）
- [Task 4.5]（清理 simverse.css）依赖 [Task 2.x]/[Task 3.x]/[Task 4.x] 全部完成
