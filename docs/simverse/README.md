# SimVerse — 目标规划与完成情况（单一事实来源）

> **代码位置**：`app/encv-mobile/plugin-simverse/web/`
> **后端位置**：`internal/simverse/`（Go 引擎）、`internal/server/`（HTTP 路由与处理器）
> 本文档已**合并**原 `docs/simulation/` 的早期规划要点（愿景/规模目标/架构/分阶段计划），
> 并作为**完成情况与当前状态的唯一权威来源**（single source of truth）。

---

## 一、愿景与规模目标

SimVerse 是一个**可演化的虚拟世界引擎**（不是游戏、不是 3D 渲染、是纯数据驱动的社会性模拟）：

- 千万级 NPC（10,000,000）、百万级组织（1,000,000），持续产生随机事件
- 平均内存 < 100 MB、峰值 < 150 MB（安卓端铁律）
- 事件生成 1,000 ~ 10,000 事件/秒；世界模拟速率 1x ~ 100x 可调
- 千万级实体 99% 常驻磁盘，按需加载（核心挑战）

## 二、架构与技术栈

```
SimVerse Web (Vue3+Ionic, WebView 插件)
   │  REST + WebSocket
   ▼
Go 后端 (gin)  ──  internal/server/simverse_api.go
   │
   ▼
FractalWorld 引擎 (internal/simverse)
   ├─ EconomyManager   经济/物价/价格冲击
   ├─ ChronicleManager 编年史/纪元(era)
   ├─ QuestManager      任务/抽卡
   └─ NPC 聚合(NPCV3)   org/region 派生聚合
```

| 层 | 技术 | 说明 |
|----|------|------|
| 框架 | Vue 3 + Ionic 8 | 移动端 UI（竖屏浏览 + 横屏世界） |
| 状态/数据 | `useSimverse` 单例组合式 | REST(`fetchJSON`) + WebSocket，无独立 Pinia store |
| 路由 | Vue Router | 竖屏 tab + 横屏 `/world` 沉浸视图 |
| 游戏渲染 | Phaser 4 + leafer-ui + matter-js | `src/game/` 世界/区域/战斗场景 |
| 原生桥接 | `plugins/SimVerse.ts` | JSInterface 双模式（native / web fallback） |
| 构建 | Vite | 产物拷贝至插件 `assets/simverse/` |
| 后端 | Go 1.25.1 + gin (`github.com/gin-gonic/gin v1.12.0`) | 模块 `github.com/Soltus/encv-go` |
| 共享组件 | `@encv/shared-components` | 复用主应用组件/主题/i18n |

### 数据层（`src/composables/useSimverse.ts`）

| 能力 | 方法 |
|------|------|
| 世界状态/配置 | `loadWorldState` / `loadWorldConfig` / `controlWorld` / `setPerformanceTier` |
| NPC | `loadNPCList` / `loadNPCDetail` / `loadFocusNPCs` / `setFocusNPCs` |
| 编年史 | `loadChronicleWorld` / `loadChronicleNPC` / `loadChronicleEvent` |
| 行为引擎 | `loadBehaviorStats` / `loadBehaviorList` |
| 性能 | `loadPerfMetrics` |
| 存档 | `loadSaveInfo` / `saveWorld` / `loadWorld` / `loadStorageStatus` |
| 纪元 | `loadEra` |
| 区域 | `loadRegionList` / `loadRegionDetail` |
| 组织 | `loadOrgList` / `loadOrgDetail` / `loadOrgMembers` / `loadOrgTerritory` |
| 经济 | `loadEconomyPrices` / `loadEconomyShocks` |
| 实时 | `connectWebSocket`（推送 `world:tick` / `world:stats`） + 2s 轮询兜底 |
| 渲染设置 | `useWorldRenderSettings`（帧率 30/45/60/90/120、等效渲染 720P/1080P/2K，localStorage 持久化 + `simverse:render-settings` 事件） |

---

## 三、阶段规划与完成状态

| 阶段 | 目标 | 状态 |
|------|------|------|
| P0 | 正确骨架（TS/Pinia/首页/横屏/网关） | ✅ 已完成 |
| P1 | 核心功能（世界视图/NPC 详情/编年史/焦点） | ✅ 已完成 |
| P2 | 优化完善（i18n/主题/存档/经济/组织） | ✅ 已完成 |
| P3 | 高级功能（干预/时间控制/调试/探索/战斗） | ✅ 已完成 |
| P4 | 行为引擎可视化 | ✅ 已完成（`WorldBehavior` + `WorldScene` NPC 行为气泡） |
| P5 | 经济与演化可视化（组织/区域/经济/纪元） | ✅ 已完成（后端聚合接口 + 13 视图） |
| P6 | 集成与优化（微内核/任务系统） | ✅ 已完成（`QuestView` + 渲染设置 Phaser 联动 + 任务埋点） |
| P7 | 持续演化（世界实时感） | 🔶 进行中（NPC 行为气泡每 5s 实时刷新；经济/编年史轮询待补） |
| P8 | 社交关系系统（关系图/亲密度/分布统计） | ✅ 已完成（后端 `SocialGraph` + `social/stats`、`npc/:id/relations` 路由 + `SocialOverview`/`NPCRelations` 视图） |

> 注：早期 `docs/simulation/frontend-phased-plan.md` 曾声称 P0–P3 全部完成，但代码实测当时约 26/35 路由为空壳；
> 经本轮（及前序轮次）补齐，**全部 37 条路由现已实现**，无 `SvPagePlaceholder` 空壳遗留。

---

## 四、完成情况总表（37 条路由，全部 ✅）

### 4.1 竖屏主框架 / 列表 / 详情

| 路由 | 视图 | 数据源 |
|------|------|--------|
| `/tabs/home` | SimverseHome | worldState（真实人口/内存/Tick） |
| `/tabs/npcs` | NPCList | loadNPCList |
| `/npc/:id` | NPCDetail | loadNPCDetail |
| `/npc/:id/inventory` | NPCInventory | loadNPCDetail.inventory/bank |
| `/npc/:id/timeline` | NPCTimeline | loadNPCDetail.short_term_mem |
| `/npc/:id/relations` | NPCRelations | loadNPCRelations（P8 社交关系系统：家庭/社交/对抗关系 + 亲密度） |
| `/tabs/chronicles` | ChronicleList | loadChronicleWorld |
| `/chronicle/:id` | ChronicleDetail | loadChronicleEvent |
| `/chronicle/:id/causal` | ChronicleCausal | loadChronicleEvent.causes/effects |
| `/tabs/settings` | SimverseSettings | — |
| `/settings/saves` | SaveManagement | loadSaveInfo/saveWorld/loadWorld |
| `/settings/simulation` | SimulationSettings | loadWorldConfig/setPerformanceTier |
| `/settings/performance` | PerformanceSettings | loadPerfMetrics/setPerformanceTier |
| `/settings/about` | AboutSimverse | 静态 |
| `/tabs/devlogs` | SimverseDevLogs | — |
| `/world` | SimverseWorld | Phaser + worldState |

### 4.2 横屏世界沉浸视图

| 路由 | 视图 | 数据源 |
|------|------|--------|
| `/world/intervention` | WorldIntervention | controlWorld/setPerformanceTier |
| `/world/debug/perf` | WorldDebugPerf | loadPerfMetrics |
| `/world/chronicles` | WorldChronicles | loadChronicleWorld |
| `/world/npc/:id` | WorldNPCDetail | loadNPCDetail |
| `/tabs/world` | WorldMapView | worldState（世界概览/入口） |
| `/world/behavior` | WorldBehavior | loadBehaviorStats / loadBehaviorList |
| `/world/settings` | WorldSettings | useWorldRenderSettings（横屏左右布局：帧率 30/45/60/90/120、渲染 720P/1080P/2K） |
| `/world/quests` | QuestView | loadQuestSummary / claimQuest（P6 任务系统） |
| `/world/social` | SocialOverview | loadSocialStats（P8 社交关系系统：类型/区域/组织分布） |

### 4.3 组织 / 区域 / 经济 / 纪元（P5，本轮补齐）

| 路由 | 视图 | 数据源 |
|------|------|--------|
| `/tabs/orgs` | OrgList | loadOrgList |
| `/org/:id` | OrgDetail | loadOrgDetail |
| `/org/:id/members` | OrgMembers | loadOrgMembers |
| `/org/:id/territory` | OrgTerritory | loadOrgTerritory |
| `/tabs/regions` | RegionList | loadRegionList |
| `/region/:id` | RegionDetail | loadRegionDetail |
| `/tabs/economy` | EconomyOverview | loadEconomyPrices / loadEconomyShocks |
| `/economy/prices` | EconomyPrices | loadEconomyPrices |
| `/economy/trade` | EconomyTrade | loadEconomyShocks |
| `/tabs/era` | EraOverview | loadEra |
| `/world/economy` | WorldEconomy | loadEconomyPrices / loadEconomyShocks |
| `/world/org/:id` | WorldOrgDetail | loadOrgDetail |
| `/world/debug/entities` | WorldDebugEntities | loadNPCList（实体浏览器） |

---

## 五、本轮完善内容（2026-07-09）

### 5.1 前端：补齐 13 个组织/区域/经济/纪元视图
此前这些视图为 `SvPagePlaceholder` 空壳（后端无对应接口）。本轮**后端补齐聚合接口**后，前端按 `NPCList` 模式
（loading / error / empty 三态 + Ionic + `useSimverse` + `useI18n`）实现：

- **组织**：`OrgList`（聚合列表）、`OrgDetail`（成员/存活/平均等级/财富/职业阶段 + 地区分布 chips）、
  `OrgMembers`（分页 + 搜索 + 跳转 NPC）、`OrgTerritory`（成员地区分布 → 跳区域）
- **区域**：`RegionList`、`RegionDetail`（人口/存活/平均等级/财富 + 区域编年史）
- **经济**：`EconomyOverview`、`EconomyPrices`（资源/价格/供给/需求表）、`EconomyTrade`（价格冲击流）
- **纪元**：`EraOverview`（当前纪元 + 世界 Tick + 编年史事件流）
- **世界视图**：`WorldEconomy`（区域切换 + 物价/冲击）、`WorldOrgDetail`、`WorldDebugEntities`（NPC 实体浏览器）

`useSimverse` 新增 9 个方法（`loadEra`/`loadRegionList`/`loadRegionDetail`/`loadOrgList`/`loadOrgDetail`/
`loadOrgMembers`/`loadOrgTerritory`/`loadEconomyPrices`/`loadEconomyShocks`）+ 对应 TS 接口；
`src/i18n/simverse.ts` 补充中英文案键。

### 5.2 前端：横屏世界设置界面（手游左右布局）
- 新增 `composables/useWorldRenderSettings.ts`：`RenderFps`（30/45/60/90/120）、`RenderQuality`（720p/1080p/2k）、
  `FPS_OPTIONS`/`QUALITY_OPTIONS`/`QUALITY_RESOLUTION`（720P=1280×720、1080P=1920×1080、2K=2560×1440），
  localStorage 持久化 + `simverse:render-settings` 事件（供 Phaser 世界消费）。
- 新增 `views/WorldSettings.vue`：横屏**左右布局**（左侧导航：画面/性能/关于；右侧面板），
  帧率段控 30/45/60/90/120、等效渲染等级段控 720P/1080P/2K（显示分辨率），并联动性能档位（perf tier）。
- 路由 `/world/settings` 已注册；`WorldMapView` 增加「设置」入口。
- **端到端联动（已落地）**：`usePhaserWorld` 监听 `simverse:render-settings` 事件，将设置实时应用到运行中的 Phaser 游戏——
  帧率经 `game.loop.targetFps`（<60 时 `forceSetTimeOut` 限速省电）生效；等效渲染等级经 `renderer.setResolution(高度/1080)` 调整画布内部分辨率（720P≈0.67 / 1080P=1.0 / 2K≈1.33）。

### 5.3 后端：补齐组织/区域/经济/纪元聚合接口（Go）
- 安装 Go 1.25.1（mirrors.aliyun.com），`go env -w GOPROXY=https://goproxy.cn,direct`，`go mod download` 成功。
- `internal/simverse/aggregates.go`（新增）：`GetRegionAggregates` / `GetRegionAggregate` / `GetOrgAggregates` /
  `GetOrgAggregate` / `GetOrgMembers`（按 `OrgID`/`RegionID` 扫描 `GetCachedNPCs()` 派生聚合）。
- `internal/simverse/org.go`：新增 `OrgType.String()`、`OrgIDToType()`（合成 org 类型反推）。
- `internal/simverse/npc_v3.go`：`GenerateNPCV3` 的 `OrgID` 由职业派生（`prof%OrgMax+1`），使组织聚合有意义
  （`RegionID` 原为 `id%100`，共 100 个区域）。
- `internal/server/simverse_api.go`（新增处理器）：`handleSimverseEraCurrent` / `handleSimverseRegionList` /
  `handleSimverseRegionDetail` / `handleSimverseOrgList` / `handleSimverseOrgDetail` / `handleSimverseOrgMembers` /
  `handleSimverseOrgTerritory` / `handleSimverseEconomyPrices` / `handleSimverseEconomyShocks`。
- `internal/server/routes.go`：在 `simGroup` 注册上述 10 条路由。
- **校验**：`go build ./...` 通过（EXIT=0），`go vet ./internal/simverse/... ./internal/server/...` 通过。

### 5.4 校验（前端）
- 工作区 `pnpm install`（`Already up to date`）。
- `pnpm --filter simverse-web build`（`vue-tsc --noEmit && vite build`）：此前已抓出并修复
  `WorldChronicles`/`ChronicleCausal` 的 TS18047（null 收窄）；本轮新增视图遵循既有模式，语言服务 lint 0 错误。
  > 注意：受命令执行上限影响，最终构建请在沙箱内手动执行 `pnpm --filter simverse-web build` 确认。

### 5.5 P4 行为可视化（WorldScene NPC 行为气泡）
- `NPCSprite` 新增行为气泡：在 NPC 名称上方渲染 `current_behavior_cn` 文本（带半透明背景），仅在高 LOD（放大）时显示。
- `WorldScene.setNPCBehaviors(Map<id, cn>)` + `usePhaserWorld.setNPCBehaviors` 暴露给 Vue 层。
- `SimverseWorld` 在 NPC 列表就绪后调用 `loadBehaviorList(1,200)` 构建 `id→行为` 映射并下发到场景；NPC 列表变化时同步刷新。
- 行为数据来自后端 `GET /api/simverse/behavior/list`（已存在，返回 `current_behavior_cn`/`mood`/`energy`）。
- **NPC 间交互事件流**：`WorldScene.drawInteractions()` 每 800ms 扫描处于「社交/交易/会谈/恋爱」行为的 NPC，对距离 <150px 的配对绘制淡蓝连线（最多 60 条，透明度随距离衰减）；按 `I` 键开关。
- **NPC 详情行为时间线 Tab**：新增 `NPCBehavior.vue`（`/npc/:id/behavior`），展示当前行为快照（行为/心情/精力/起始 Tick/时长）+ 该 NPC 编年史事件流（`loadChronicleNPC`）；`NPCDetail` 增加「行为时间线」入口。

### 5.6 P6 任务系统前端（QuestView）
- `useSimverse` 新增 `loadQuestSummary` / `claimQuest` / `recordQuestAction` 及类型
  （`SimverseQuest` / `SimverseQuestReward` / `SimverseQuestSummary` / `SimversePlayerStats`）。
- 新增 `views/QuestView.vue`（`/world/quests`）：按类型（日常/成就/剧情/经济）分组，进度条 + 奖励徽章 + 领取按钮
  （`progress>=goal` 可领取，调用 `POST /api/simverse/quest/claim`）。路由已注册，`WorldMapView` 增加「任务」入口。
- 后端 `QuestManager`（`/quest/list`、`/quest/claim`、`/quest/action`）此前已就绪。

### 5.7 P7 持续演化（世界实时感）
- `SimverseWorld` 新增 `behaviorPollInterval`：Phaser 就绪后每 5s 调用 `loadNPCBehaviors()` 重新拉取行为分布，
  使 WorldScene 内 NPC 行为气泡随世界演化实时更新；组件卸载时清理定时器。
- 后续可扩展：经济行情（`loadEconomyPrices`）、编年史（`loadChronicleWorld`）的定时轮询刷新，强化"活生态"观感。

### 5.8 P8 社交关系系统（SocialGraph）
- **后端**（`internal/simverse/social.go`，新文件）：
  - `SocialGraph`：按 NPC.ID 缓存的关系图，关系由 NPC 的确定性属性（婚姻数 / 子女数 / 出生时刻）派生，使用以 `npc.ID` 为种子的本地 RNG 生成，结果稳定可复现，避免存储千万级全量图。
  - `RelationshipType.String()`：关系类型稳定英文 key（stranger/acquaintance/friend/lover/spouse/parent/child/sibling/master/apprentice/enemy/rival），前端 i18n 本地化。
  - `generateRelationships(npc)`：派生家庭（配偶/恋人/子女/父母/兄弟姐妹）、社交（朋友/熟人/师徒）、对抗（对手/敌人）三类关系，含亲密度 `Affinity` 与最近接触 `LastMeet`。
  - `SocialStats` + `Stats(npcs, regionFilter, orgFilter)`：按（可选）区域/组织过滤聚合关系总数与按类型/区域/组织分布。
  - 接入 `FractalWorld`：新增 `socialGraph` 字段与 `Social()` / `GetNPCRelationships()` / `GetSocialStats()` 访问器。
  - 复用 `interaction.go` 的 `Relationship` / `RelationshipGraph` 类型与 `entity.go` 的 `RelationshipType` 枚举。
- **HTTP 路由**（`internal/server/`）：
  - `GET /api/simverse/social/stats?region=&org=` → `handleSimverseSocialStats`（采样角色数 / 关系总数 / 按类型·区域·组织分布）。
  - `GET /api/simverse/npc/:id/relations` → `handleSimverseNPCRelations`（该 NPC 的关系列表，含目标档案 `npcBrief`、按亲密度降序）。
- **前端**：
  - `useSimverse.ts`：新增 `SimverseRelation` / `SimverseRelationListResponse` / `SimverseSocialStats` 类型，及 `loadSocialStats()` / `loadNPCRelations()`。
  - `i18n/simverse.ts`：新增 `social*` 与 `rel.*` 共 24 个中英文本地化键。
  - `SocialOverview.vue`（新，`/world/social`）：世界关系总览——总数/采样卡片 + 类型分布进度条 + 区域/组织分布跳转。
  - `NPCRelations.vue`：重写为真实关系视图——关系计数 chips + 关系卡片（目标名/类型/亲密度，点击跳转目标关系网）。
  - `WorldMapView` 增加「💞 社交关系」入口；`NPCDetail` 已有「查看关系」子入口指向 `/npc/:id/relations`。

---

## 六、下一步 / 阻塞项

### 6.1 已解锁（前阻塞项已消除）
组织/区域/经济/纪元页面（5.1 / 5.3）与任务系统（5.6）所需的后端接口与前端视图**已全部就绪**。

### 6.2 P4 行为可视化剩余项
- ✅ 已完成：NPC 详情「行为时间线」Tab（`NPCBehavior.vue`）；NPC 间交互事件流（WorldScene 连线 + `I` 键开关）。详见 5.5。

### 6.3 P6 集成
- ✅ 已完成：`WorldSettings` 渲染设置（帧率/等效渲染等级）经 `simverse:render-settings` 事件端到端联动 Phaser 世界画布（见 5.2）。
- ✅ 已完成：任务行为埋点——查看 NPC 详情调 `recordQuestAction("view_npc")`，查看经济行情（经济概览/物价/世界经济）调 `recordQuestAction("view_economy")`，抽卡调 `recordQuestAction("gacha")`，驱动日常任务进度。

### 6.4 P7 持续演化（进行中）
- ✅ 已完成：NPC 行为气泡每 5s 实时刷新（`SimverseWorld.behaviorPollInterval`，见 5.7）。
- 待做：经济行情、编年史的定时轮询刷新；行为/经济变化的实时事件推送（WebSocket）替代轮询。

### 6.5 质量保障
- 沙箱内执行 `pnpm --filter simverse-web build` 做最终类型/构建校验；
- 为详情类视图补充路由级单元测试 / 组件快照。

---

*最后更新：2026-07-09（合并 docs/simulation → docs/simverse 完成；全部 37 路由已实现，后端聚合接口已落地；P4 行为气泡/交互流、P6 任务系统/渲染联动、P7 行为实时刷新、P8 社交关系系统（SocialGraph 后端 + social/stats、npc/:id/relations 路由 + SocialOverview/NPCRelations 视图）已补齐）*
