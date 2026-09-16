# SimVerse 游戏本体架构认知与页面骨架方案

> 本文档用于**巩固 2026-07-10 的关键纠偏认知**，并定义横屏世界（游戏本体）应有的"页面骨架"。
> 它是后续所有玩法（P9–P14）落地的**地基**，应优先于功能开发。
> 关联：`README.md`（完成状态）、`ROADMAP.md`（P9+ 计划）。

---

## 一、核心认知（纠偏）

### 1.1 哪部分是"游戏"，哪部分是"后台"
- **Ionic 各页面（Tabs：home/world/npcs/orgs/regions/chronicles/economy/settings/devlogs + 各详情页）= 后台管理 / 数据浏览**。它们是模拟器的管理台（看配置、看 tick、看人口、看物价、看编年史），**不是游戏**。
- **横屏世界（`SimverseWorld.vue` + Phaser `WorldScene`）= 游戏本体**。玩家真正"玩"的只有这一屏。

> 之前若干轮"UI 改造"全部作用在 Ionic 数据视图或横屏世界的**表面装饰**（配色、NPC 贴图、粒子、辉光、图例、弹性滑入），本质是**屎上雕花**——根子（页面骨架）没动，所以真机预览毫无改观。

### 1.2 真正的病根：缺"页面骨架"
当前 `SimverseWorld.vue` 是**一整页永远不变的东西**：
- 一个永远在背后的 Phaser 画布；
- 一圈**永远可见、永远不长变的浮动按钮**（左菜单 8 个 + 右菜单 5 个 + 顶栏）；
- 点按钮弹出的 `side-panel` **全都是同一个通用抽屉换内容**；
- 点 NPC 弹出的 `detail-modal` 是个**小卡片浮层**，关掉又回到同一页。

**它从没"变成另一页"过。** 没有任何屏幕状态机、没有任何会变形的页面、没有任何转场。这正是"固定页面不变"——连一个能切换的屏幕状态机都没有。

> 现代手游（**哪怕 B 单人 RPG**）都是由很多"页"组成的：主菜单 → 地图 → 角色 → 战斗 → 商店 → 事件弹窗 → 设置。你玩游戏时**屏幕一直在变**，每一页是不同布局，靠转场连起来。当前世界完全没有这层结构。

### 1.3 身份定义（决定骨架怎么搭）
- **A 上帝 / 观察者（主）**：俯瞰一个自运行的活世界，聚焦 NPC、干预事件、看它演化出的编年史。核心循环 = 观察 → 聚焦/干预 → 看后果。
- **B 化身（体验）**：你作为世界里的一个角色，和 NPC 一样成长、社交、被模拟影响。核心循环 = 扮演 → 互动 → 成长。

**当前 `SimverseWorld` 同时假装是两者，且 B 部分是写死桩**——既当上帝模拟器又当英雄养成手游，两边都不成立，这就是"没有页面骨架"的深层表现。

---

## 二、真假清单（避免再雕花在桩上）

`SimverseWorld.vue` 里各面板的数据来源：

| 面板 | 数据 | 状态 |
|------|------|------|
| quest（任务） | `loadQuestData` / `useSimverse` | ✅ 真（接后端） |
| npc（NPC 列表） | `loadNPCList` | ✅ 真 |
| chronicles（编年史） | `loadChronicleWorld` | ✅ 真 |
| economy（物价/财富榜） | `loadEconomyPrices` / 财富榜 | ✅ 真 |
| settings（画面/档位） | `useWorldRenderSettings` / `worldConfig` | ✅ 真 |
| **profile（角色卡）** | `playerLevel=1` 等 `ref` 写死 | ❌ 桩 |
| **training（训练）** | `trainingModes` 写死、`playerAttack+=` 本地改 | ❌ 桩 |
| **inventory（背包）** | `inventoryItems` 写死 | ❌ 桩 |
| **gacha（抽卡）** | `pool` 写死、`Math.random` 本地抽 | ❌ 桩（未接后端） |
| **battle（战斗）** | `battleEnemies` 写死 | ❌ 桩 |
| **explore（探索）** | `exploreRegions` 写死 | ❌ 桩 |

> 结论：以 A 为主时，**B 类桩（profile/training/inventory/gacha/battle/explore）应整体移除或折叠**，不占世界主屏；若要做 B 体验，这些必须**接到真实后端角色系统**，而不是桩。

---

## 三、目标页面骨架（屏幕状态机）

不是"一个世界页 + 一堆按钮"，而是一组**会变形的屏幕**，用转场连起来：

| 屏幕 | 触发 | 布局要点 |
|------|------|----------|
| `world` 主世界页 | 默认 | 活地图 + 极简顶栏（世界名/运行态/tick/人口）+ **底部主操作条**（聚焦/事件/干预/化身/设置）。那 13 个浮动按钮收进主操作条 + "更多"溢出。 |
| `focus` 焦点页 | 点 NPC/组织/区域 | **屏幕整体切换**：镜头推近对象，布局变成"这个对象"的上下文（身份 + 时间线 + 关系 + 对此对象干预/编入编队），带返回键。是另一页，不是通用抽屉。 |
| `event` 事件页 | 重大编年史事件 | 占满屏幕的"刚刚发生了什么"时刻，不是右上角小 ticker。 |
| `intervene` 干预页 | 主动干预模式 | 对世界动手的专属布局。 |
| `character` 化身页（B） | 化身入口 | 你在这个世界里的化身：属性/关系/任务（接真实后端）。 |
| 弹窗层 | gacha/settings/battle | 全覆盖模态。 |

**转场**：world→focus 用"镜头推近 + 内容滑入"；focus→world 用"镜头拉远 + 淡出"。屏幕必须**肉眼可见地变化**。

---

## 四、重构路线（地基优先）

1. **先建骨架（本阶段）**：引入 `screen` 状态机 + 转场框架；把"点 NPC 小卡片浮层"改为 `world → focus` 真切换；把 13 个浮动按钮收敛为底部主操作条。
2. **再填内容（P9–P14）**：把已有真数据面板（任务/NPC/编年史/经济）挂进 `world` 主操作条与 `focus` 上下文；把 B 类桩按 A/B 定位决定移除或接后端。
3. **事件/干预/化身** 作为独立屏幕逐步落地，复用 `chronicle` / `intervention` / 真实角色系统。

> 任何"UI 美化"都必须发生在骨架之上，而不是反过来。

---

## 五、落地进度（2026-07-10）

- ✅ **地基第一块：屏幕状态机**（`SimverseWorld.vue`）
  - 新增 `screen` 状态机：`world | focus | event | intervene | character`。
  - 点 NPC（`onNPCClick` / `selectNPC`）不再弹"小卡片浮层"，而是 `screen='focus'` —— 屏幕整体切换为焦点页。
  - 焦点页用 `<transition name="focus-slide">` 从右滑入 + 缩放（`cubic-bezier(0.34,1.56,0.64,1)`），关闭调用 `backToWorld()` 回到 `world` 并复位状态机。
  - 焦点页升级为近全屏页面（max-width 440 / max-height 86% / 内容可滚动），不再是从前的小卡片。
  - `read_lints` 0 错误。
- ✅ **地基第二块：底部主操作条 + 更多溢出**（同文件）
  - 移除原先散落在左右两侧的 13 个浮动按钮（`left-menu` / `right-menu` 及其全部 CSS），改为主世界 `world` 页**底部主操作条**：聚焦 / 编年史 / 经济 / 更多 / 设置。
  - "更多"溢出面板（`more-pop`）收拢二级功能：任务 / 组织 / 角色(B) / 养成(B) / 背包(B) / 探索(B) / 战斗(B) / 召唤(B)——即原 B 类体验桩，诚实标注为桩、待接后端或移除。
  - 底部条与"更多"面板均带 `<transition>` 进出动画；`screen !== 'world'` 时整体隐藏，符合"不同屏幕不同 chrome"的骨架原则。
  - 死掉的 `menu-*` / `gacha-menu-btn` / `menu-badge` CSS 与 `gachaGlow` / `activeGlow` / `badgePulse` / `sparkle` 孤儿 keyframes 一并清理。
  - `read_lints` 0 错误。
- ✅ **地基第三块：事件页 `event`**（同文件）
  - 顶部事件跑马灯（event-ticker）改为可点击，进入**全屏事件页** `screen='event'`：标题 + 返回 + 滚动事件流（按 importance 着色），用 `<transition name="event-page">` 上滑淡入。
  - 这是从"右上角小 ticker"升级为"占满屏幕的刚刚发生了什么"时刻，落实 §1.2 / §三 的页面骨架要求。
  - `read_lints` 0 错误。
- ✅ **地基第四块：干预页 `intervene`**（同文件，全部接真实后端）
  - 新增 `screen='intervene'` 全屏页，提供**上帝视角的世界控制**：时间控制（播放/暂停 `toggleRunning`、单步 `stepOnce`、`ffSteps` 快进循环 `controlWorld("step")`）、世界快照（`saveWorld` / `loadWorld`，带成功反馈）。
  - 不造桩：`WorldAction` 仅 `start/stop/pause/resume/step`，故快进用"循环单步"实现，未虚构 speed 接口。
  - 底部主操作条新增「🎛️ 干预」入口（`screen==='intervene'` 高亮）。
  - `read_lints` 0 错误。
- ✅ **地基第五块：化身页 `character`（B 体验，诚实标注）**（同文件）
  - 新增 `screen='character'` 全屏页，进入入口收在「更多」溢出（🧑 化身）。
  - 顶部黄色 `bStubHint` 横幅明确标注"后端角色系统未接入，以下为占位"，下方展示现有 `playerLevel/diamond/gold/stamina/skills` 桩数据——**不伪装成真实系统**，符合"B 类桩诚实标注"原则。
  - `read_lints` 0 错误。
- ✅ **页面骨架已成型**：`world`（底部主操作条）/ `focus`（NPC 焦点页）/ `event`（全屏事件流）/ `intervene`（世界控制）/ `character`（B 占位）五屏 + 各自 `<transition>` 转场 + `backToWorld()` 统一返回。各屏 chrome 不同（非 world 屏隐藏底部条），屏幕肉眼可见地切换，根治"一页不变"。
- ✅ **地基第六块：焦点页升级为「对象上下文页」**（`SimverseWorld.vue`，2026-07-10 续）
  - 落实 §三 / §五 待办：`focus` 页从"小卡片"升级为**镜头推近 + 对象上下文**。
  - **镜头转场**：`selectNPC()` 进入焦点页时调用 `phaserWorld.centerOnNPC(id)`（Phaser 相机居中并放大到 1.2x），构成 `world → focus` 的"镜头推近"；`backToWorld()` 复位 `setZoom(0.5)` 回到俯瞰视角。地图点击 NPC（`onNPCClick`）也统一走 `selectNPC`。
  - **身份**：保留基础档案网格，头部新增**流派徽章**（复用 P14 `deriveBuildFromNPC` + `ARCH_META` 的 emoji/中文名/配色 + 契合度 ★），让 NPC 在焦点页即有可辨识定位。
  - **时间线**：`loadChronicleNPC(id, 30)` 拉取该 NPC 编年史事件流（接真实后端，非桩），按 importance 左侧着色。
  - **关系**：`loadNPCRelations(id)` 拉取关系网（接真实后端 `SocialGraph`），关系行可点击 → `selectNPC(rel.target)` **跳转对方焦点页**，形成关系网导航；关系类型经 `simverse.rel.*` 本地化、显示亲密度。
  - **编入编队**：焦点页操作条 `toggleSquad()` 把 NPC 加入/移出玩家编队，复用 `simverse:squad` localStorage（与 `SquadSynergy.vue` 同一持久化键，编队满 6 提示），呼应 P14 编队协同。
  - **标签切换**：`ion-segment`（`:value` + `@ionChange`，沿用项目既有模式）在 身份/时间线/关系 间切换；`focusLoading` 期间显示加载态。
  - `read_lints` 0 错误；i18n 新增 `simverse.focus.*`（身份/时间线/关系/编入编队/移出编队/编队已满/亲密度/编年史空/关系空）+ `simverse.loading`。
- ⏳ 后续（骨架之上建玩法，P9–P14）：统一转场与返回手势打磨；B 类桩（profile/training/inventory/gacha/battle/explore）按 A 为主定位决定移除或接后端真实角色系统；再叠 P9 LLM NPC、P10 活世界、P11 玩家经济、P12 季节、P13 多人。
