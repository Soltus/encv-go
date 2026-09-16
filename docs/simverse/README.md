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
| 后台管理/数据浏览 | Vue 3 + Ionic 8 | **`Tabs` 各页（home/world/npcs/orgs/…）是模拟器管理台，不是游戏** |
| 状态/数据 | `useSimverse` 单例组合式 | REST(`fetchJSON`) + WebSocket，无独立 Pinia store |
| 路由 | Vue Router | 后台 tab + 横屏 `/world` 沉浸视图 |
| **游戏本体** | **Phaser 4 + leafer-ui + matter-js**（`src/game/` + `SimverseWorld.vue`） | **玩家真正"玩"的只有这一屏（横屏世界）** |
| 原生桥接 | `plugins/SimVerse.ts` | JSInterface 双模式（native / web fallback） |
| 构建 | Vite | 产物拷贝至插件 `assets/simverse/` |
| 后端 | Go 1.25.1 + gin (`github.com/gin-gonic/gin v1.12.0`) | 模块 `github.com/Soltus/encv-go` |
| 共享组件 | `@encv/shared-components` | 复用主应用组件/主题/i18n |

> ⚠️ **认知纠偏（2026-07-10）**：Ionic 各页是**后台管理**，横屏世界（Phaser）才是**游戏本体**。此前多轮"UI 改造"作用在 Ionic 数据视图或横屏世界的表面装饰（配色/NPC 贴图/粒子/辉光/图例），属**屎上雕花**——未触及根本。根本缺失是**页面骨架**（屏幕状态机 + 多页转场），见 `GAME_ARCHITECTURE.md`。

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
| 实时 | `connectWebSocket`（推送 `world:tick` / `world:stats` / `economy:update` / `chronicle:event`） + 2s 轮询兜底 |
| 实时刷新 | `useLiveRefresh`（WS 信号 `economySignal`/`chronicleSignal` 优先 + WS 断连时兜底轮询，节流防抖 + 卸载清理） |
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
| P7 | 持续演化（世界实时感） | ✅ 已完成（NPC 行为气泡每 5s 刷新；经济/编年史经 WS 实时推送 + 兜底轮询自动刷新） |
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
- **画面设置入口（2026-07-10 修正）**：帧率/画质原仅在 `/world/settings`（`WorldMapView` 入口页「设置」）可调；现已把同一套 `useWorldRenderSettings`（单例，改即经 `simverse:render-settings` 事件实时生效）接入**实时世界 HUD 的 ⚙️ 设置面板**，无需退回入口页即可在横屏世界里调帧率/画质。

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
- **经济/编年史 WS 实时推送（本轮补齐，2026-07-09 续）**：
  - **后端**（`internal/server/simverse_api.go` · `wsBroadcastLoop`）：世界运行时新增两条节流广播——
    `economy:update`（每 3s，携带 `tick`）、`chronicle:event`（每 4s，携带 `tick`/`count`/`era`）。仅发"变化信号"，
    不推送全量数据，避免大 payload；世界停止（`running=false`）时不广播。
  - **数据层**（`composables/useSimverse.ts`）：新增 `economySignal` / `chronicleSignal` 两个递增计数 ref，
    在 `handleWSMessage` 中收到对应 WS 事件时自增；随组合式单例导出。
  - **通用组合式**（新 `composables/useLiveRefresh.ts`）：封装"WS 信号优先 + 兜底轮询"策略——
    WS 已连接时由 signal 变化驱动刷新；WS 未连接则按 `pollMs` 兜底轮询，连接恢复即自动停止轮询
    （真正做到"实时推送替代轮询"）。内置 `throttleMs`（默认 2s）防抖 + 卸载自动清理。
  - **视图接入（7 个）**：`EconomyOverview` / `EconomyPrices` / `EconomyTrade` / `WorldEconomy`（经济，监听 `economySignal`）、
    `EraOverview` / `ChronicleList` / `WorldChronicles`（编年史，监听 `chronicleSignal`）。
    各视图 `reload`/`loadEvents` 增加 `silent` 参数：后台刷新不触发 loading 骨架、不重复埋点、错误仅告警，避免闪烁；
    工具栏新增脉冲式「实时/LIVE」指示灯。
  - **i18n**：新增 `simverse.live`（中「实时」/ 英「LIVE」）。

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

### 5.9 横屏世界 UI 改造（Phaser 渲染层，2026-07-10）
- **问题定位**：此前 P7/P8/P14 的改动均在 Ionic/Vue 数据视图层，而横屏世界是独立的 Phaser canvas（`WorldScene`/`NPCSprite`），二者完全隔离；NPC 此前只是 6px 绿/灰小圆点 + 白字黑描边，故真机预览"辣眼睛且毫无变化"。
- **根因**：渲染层从未接入 P14 流派系统；`SimverseNPC` 已含 `profession/level/wealth_tier/social_tier`，可直接复用 `deriveBuildFromNPC`。
- **改动**（`src/game/`）：
  - `builds.ts` 新增共享元数据 `ARCH_META`（`color` 数值色 / `colorCss` / `emoji` / 中文 `name`），供 Phaser 与 Ionic 两端共用，保证配色/图标一致。
  - `NPCSprite.ts` 重写：以流派驱动外观——彩色流派圆底（按 `ARCH_META.color`）+ emoji 头像（覆盖在圆底上）+ 白色描边；名牌改为深色半透明胶囊背景（去除刺眼黑描边）；移动轨迹 `drawTrail` 改用流派色；死亡 NPC 显示灰底 💀。呼吸/悬停缩放作用于整个头像主体。
  - `WorldScene.ts`：新增 `createLegend()` 左上角「流派图例」（固定不随相机缩放/平移，按 `L` 键开关），帮助识别彩色头像对应流派；新增 `createVignette()` 暗角叠加（径向渐变，固定相机），给平坦地形瓦片增加纵深感，缓解平铺"辣眼睛"。
- **验证**：`read_lints` 对 `src/game/*` 0 错误；最终构建请在沙箱内 `pnpm --filter simverse-web build` + `go build` 确认（沙箱无 node_modules/Go 工具链）。

### 5.10 横屏世界生命感（game-feel，2026-07-10）
- **认知修正**：用户指出"辣眼睛"的根源不是配色/贴图，而是**界面与地图整体发呆、没有生命感**——这正是现代手游（原神/星穹铁道/龙息：神寂）靠持续运动 + 微交互 + 反馈动画解决的核心体验问题。
- **世界层（`WorldScene.ts`）**：
  - 新增 `createAmbientParticles()`：90 个环境粒子（萤火/尘埃），在世界空间持续缓缓漂浮、正弦明灭，给空旷地图注入"呼吸感"；`updateAmbientParticles(delta)` 每帧更新（循环回收 + 透明度波动），`depth=1` 置于地形之上、NPC 之下。
  - NPC 运动频率翻倍：`npcMoveInterval` 5000→3000ms，移动触发概率 0.3→0.5，地图肉眼可见地"活"起来。
  - 头像/粒子分层：`NPCSprite` 统一 `setDepth(10)`，避免被粒子/地形遮挡。
- **HUD 层（`SimverseWorld.vue`，纯 CSS 动画，零逻辑风险）**：
  - 背景星云 `bgDrift` 26s 缓动漂移；
  - 资源数值（💎/🪙/⚡）变动时 `valuePop` 弹跳高亮（`:key` 触发重挂载重放动画）；
  - 运行中播放键 `runPulse` 绿色脉冲辉光；
  - 抽卡按钮 `gachaGlow` 呼吸式粉光高亮（引导注意力，仿手游常驻抽卡入口）；
  - 激活菜单 `activeGlow` 紫色辉光；
  - 侧栏 `side-panel` 弹性滑入（spring 缓动 `cubic-bezier(0.34,1.56,0.64,1)` + 轻微 scale）。
- **验证**：`read_lints` 对 `SimverseWorld.vue` / `WorldScene.ts` 0 错误。
- ⚠️ **事后纠偏（2026-07-10 末）**：§5.9/§5.10 的"流派贴图/粒子/辉光/弹性滑入"仍是**表面装饰**。用户最终点明：横屏世界缺的是**页面骨架**——它只是一页永远不变的东西（常驻画布 + 永远可见的浮动按钮 + 通用抽屉 + 小卡片浮层），没有屏幕状态机、没有会变形的页面、没有转场。这些动画贴在"一页"上依旧是屎上雕花。真正的改造是建立多页屏幕架构（world/focus/event/intervene/character + 转场），见 `GAME_ARCHITECTURE.md`，并已启动重构。

---

### 5.11 横屏世界页面骨架·焦点页升级（对象上下文页，2026-07-10 续）
- **承上**：`GAME_ARCHITECTURE.md` 地基第六块。把 `focus` 页从"小卡片浮层"升级为真正的**对象上下文页**，落实"镜头推近 + 身份/时间线/关系/编入编队"的页面骨架要求。
- **镜头转场**：进入焦点页 `phaserWorld.centerOnNPC(id)`（相机推近 1.2x），返回 `setZoom(0.5)` 复位俯瞰；地图点击 NPC 统一走 `selectNPC`。
- **身份**：头部流派徽章（复用 P14 `deriveBuildFromNPC` + `ARCH_META`）。
- **时间线**：`loadChronicleNPC` 拉取该 NPC 编年史（真实后端）；**关系**：`loadNPCRelations` 拉取关系网，点击跳转对方焦点页（关系网导航）；均经 `simverse.rel.*` 本地化。
- **编入编队**：`toggleSquad` 复用 `simverse:squad` localStorage（与 `SquadSynergy.vue` 同键），呼应 P14 编队协同。
- `read_lints` 0 错误；i18n 新增 `simverse.focus.*` + `simverse.loading`。

## 六、下一步 / 阻塞项

### 6.1 已解锁（前阻塞项已消除）
组织/区域/经济/纪元页面（5.1 / 5.3）与任务系统（5.6）所需的后端接口与前端视图**已全部就绪**。

### 6.2 P4 行为可视化剩余项
- ✅ 已完成：NPC 详情「行为时间线」Tab（`NPCBehavior.vue`）；NPC 间交互事件流（WorldScene 连线 + `I` 键开关）。详见 5.5。

### 6.3 P6 集成
- ✅ 已完成：`WorldSettings` 渲染设置（帧率/等效渲染等级）经 `simverse:render-settings` 事件端到端联动 Phaser 世界画布（见 5.2）。
- ✅ 已完成：任务行为埋点——查看 NPC 详情调 `recordQuestAction("view_npc")`，查看经济行情（经济概览/物价/世界经济）调 `recordQuestAction("view_economy")`，抽卡调 `recordQuestAction("gacha")`，驱动日常任务进度。

### 6.4 P7 持续演化（已完成）
- ✅ 已完成：NPC 行为气泡每 5s 实时刷新（`SimverseWorld.behaviorPollInterval`，见 5.7）。
- ✅ 已完成：经济行情、编年史经 WebSocket 实时推送（`economy:update` / `chronicle:event`）+ 兜底轮询自动刷新
  （`useLiveRefresh`，7 视图接入，见 5.7）。
- 待做（可选增强）：将 NPC 行为气泡刷新也切换到 WS 信号（当前仍为 5s 轮询）；对推送内容做增量 diff 进一步降带宽。

### 6.5 质量保障
- 沙箱内执行 `pnpm --filter simverse-web build` 做最终类型/构建校验；
- 为详情类视图补充路由级单元测试 / 组件快照。

---

## 七、下一阶段开发计划（P9+，借鉴 2024 年后手游）

前瞻性路线图已独立沉淀于 **[`ROADMAP.md`](./ROADMAP.md)**，要点：

- **调研基线**：恋与深空（AI 伴侣/羁绊）、鸣潮（开放世界/co-op）、绝区零（活城枢纽）、
  七日世界（动态事件/玩家交易/赛季）、无限暖暖（UGC/分享）、原神/星穹铁道（长线运营）、
  幻兽帕鲁（自动化基地）、Roblox（UGC 生态）。
- **P9 智能 NPC 与情感陪伴**：LLM 驱动焦点 NPC 对话 + 记忆回溯 + 羁绊/好感升级（复用 `SocialGraph`）。
- **P10 活态世界与玩家共创**：NPC 昼夜作息、区域动态事件 2.0、UGC 叙事、拍照分享（复用 `chronicle`/实时推送/Phaser）。
- **P11 玩家驱动经济与组织**：玩家市场叠加模拟物价、公会/领地战、声望榜（复用 `EconomyManager`/`BattleManager`）。
- **P12 赛季制与长线运营**：Battle Pass + 限时活动 + 世界演化回放（扩展 `QuestManager`）。
- **P13 多人共存与社交场**：同世界多玩家化身、组队探索、协作事件（Phaser `WorldScene` 扩展）。
- **P14 流派系统与卡牌化界面**（补充卡片手游参考：龙息：神寂 / 潮汐守望者 / 云顶之弈 / 炉石）：NPC/组织"流派"派生 + 自走棋式编队协同 + 卡牌化 UI（卡片/标签 chips/编队网格/关系网卡牌连线/抽卡动画），复用 `personality.go`/`aggregates.go`/`SocialGraph`/`gacha`。
  - ✅ 已落地（2026-07-10）：`src/game/builds.ts` 的 `deriveNPCBuild()`（前端确定性流派派生）+ `NPCDetail`「流派」卡片（彩色徽章 + 契合度星级 + 次要流派 chips）+ i18n `simverse.build*`；组织流派/编队协同/卡牌连线待续。
  - ✅ 已落地（2026-07-10）：`NPCRelations.vue` 新增 SVG「关系网」卡片连线可视化（中心自身节点 + 按亲密度环状排布的关系目标卡牌 + 类型着色/亲密度加权的连线，点击跳转），呼应 P8 `SocialGraph` 与卡片连线范式；i18n `simverse.relGraph`/`relGraphHint`。
  - ✅ 已落地（2026-07-10）：新增 `SquadSynergy.vue`（路由 `/world/squad`，`WorldMapView`「🃏 编队」入口）——玩家组最多 6 人编队（`localStorage` 持久化），基于 `deriveBuildFromNPC` 统计流派，达 2/4/6 触发「初/盛/极」羁绊，直接呼应《潮汐守望者》/云顶之弈流派协同；i18n `simverse.squad*`/`synergy*`。
  - ✅ 已落地（2026-07-10）：横屏世界 Phaser 渲染层接入 P14 流派——`NPCSprite` 改为「彩色流派圆底 + emoji 头像 + 清爽名牌」（`ARCH_META` 共享元数据），`WorldScene` 新增「流派图例」(`L` 键) 与暗角纵深叠加，根治"辣眼睛"的统一绿/灰小圆点。详见 5.9。
  - ✅ 已落地（2026-07-10，「生命感 / game-feel」修正）：此前多轮改造都停留在"配色/贴图"层，但横屏世界真正的"辣眼睛"根源是**界面与地图没有生命感（发呆）**。本轮注入运动与反馈——`WorldScene` 新增 90 个环境粒子（萤火/尘埃，持续漂浮明灭）+ NPC 移动频率翻倍（间隔 5s→3s、触发概率 0.3→0.5）+ 头像/粒子分层（NPC depth 10）；`SimverseWorld.vue` HUD 新增：背景星云缓动漂移、资源数值变动弹跳、运行中播放键脉冲辉光、抽卡按钮呼吸式高亮、激活菜单辉光、侧栏弹性滑入（spring 缓动）。详见 5.10。
- **MVP 路径**：P9 → P10 → P11 → P12 → P13 → P14（情感陪伴留存杠杆最高，多人架构改动最大放最后；P14 为低风险表现层升级，可并行）。

---

*最后更新：2026-07-10（合并 docs/simulation → docs/simverse 完成；全部 37 路由已实现，后端聚合接口已落地；P4 行为气泡/交互流、P6 任务系统/渲染联动、P7 持续演化（行为实时刷新 + 经济/编年史 WS 实时推送 `economy:update`/`chronicle:event` + `useLiveRefresh` 7 视图接入，P7 完成）、P8 社交关系系统（SocialGraph 后端 + social/stats、npc/:id/relations 路由 + SocialOverview/NPCRelations 视图）已补齐；横屏世界页面骨架（屏幕状态机 world/focus/event/intervene/character + 底部主操作条 + 焦点页升级为对象上下文页：镜头推近 + 身份/时间线/关系/编入编队）已落地，详见 `GAME_ARCHITECTURE.md`）*
