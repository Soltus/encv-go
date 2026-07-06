# SimVerse 前端 P0 规划：文字图标版 UI 框架 + 共享组件抽取

> **版本**：v0.1
> **日期**：2026-07-06
> **目标**：
> 1. SimVerse 前端实现「文字 + 图标」版 UI 框架，至少 40+ 页面骨架
> 2. 进一步抽取共享组件，encv-mobile 代码量减少 60% 以上
> 3. 复用 shared-components 的 Settings / DevLogs / HomePage 等成熟组件

---

## 一、现状分析

### 1.1 代码量统计

| 项目 | 文件数 | 代码行数 | 页面数 | 说明 |
|------|--------|---------|--------|------|
| encv-mobile | 287 | 89,435 | 43 | 主应用，业务逻辑重 |
| shared-components | 287 | 88,486 | 44 | 共享组件库，已抽取大部分 |
| simverse-frontend | 14 | 3,960 | 6 | SimVerse 前端，骨架期 |

### 1.2 encv-mobile 独有文件

encv-mobile 已大部分抽取到 shared-components，独有文件仅 4 个：
- `App.vue` - 应用入口
- `main.ts` - 启动文件
- `router/index.ts` - 路由配置
- `components/DebugConsole.vue` - 调试控制台

**结论**：encv-mobile → shared-components 的抽取已基本完成（~98% 文件已迁移）。
下一步重点是 **simverse-frontend 的页面框架搭建 + 复用 shared-components**。

---

## 二、P0 目标：40+ 页面骨架

### 2.1 设计原则

1. **文字 + 图标优先**：不做复杂 Canvas/WebGL 渲染，先用 Ionic 组件 + ionicons 搭骨架
2. **二次元手游风格**：参考《公主连结》《蔚蓝档案》等手游 UI，卡片式布局 + 渐变配色
3. **横屏世界 + 竖屏详情**：世界视图横屏，其他页面竖屏（从世界进入的详情页也横屏）
4. **可复用优先**：能复用 shared-components 的就复用，不重复造轮子
5. **类型安全**：全量 TypeScript，`npm run typecheck` 零错误

### 2.2 页面分类与清单（共 48 页）

#### 📱 竖屏页面（32 页）— 从首页/标签页进入

| 分类 | 页面 | 路由 | 优先级 | 复用组件 |
|------|------|------|--------|---------|
| **首页 Tab** | 世界概览首页 | `/tabs/home` | P0 | HomePage（改造） |
| | 世界状态卡片 | — | P0 | ServerStatusCard（改造） |
| | 快速入口网格 | — | P0 | 新组件 |
| **世界 Tab** | 世界地图（文字版） | `/tabs/world` | P0 | 新组件 |
| | 区域列表 | `/tabs/regions` | P1 | 新组件 |
| | 区域详情 | `/region/:id` | P1 | 新组件 |
| **NPC Tab** | NPC 列表 | `/tabs/npcs` | P0 | TaskVirtualList（改造） |
| | NPC 详情 | `/npc/:id` | P0 | 新组件 |
| | NPC 属性面板 | — | P1 | 新组件 |
| | NPC 关系网 | `/npc/:id/relations` | P1 | 新组件 |
| | NPC 时间线 | `/npc/:id/timeline` | P1 | UnifiedTimelineCard |
| | NPC 背包/财产 | `/npc/:id/inventory` | P2 | 新组件 |
| **组织 Tab** | 组织列表 | `/tabs/orgs` | P1 | 新组件 |
| | 组织详情 | `/org/:id` | P1 | 新组件 |
| | 组织成员 | `/org/:id/members` | P1 | 新组件 |
| | 组织领地 | `/org/:id/territory` | P2 | 新组件 |
| **编年史 Tab** | 编年史列表 | `/tabs/chronicles` | P0 | DevLogs（改造） |
| | 事件详情 | `/chronicle/:id` | P0 | ChronicleDetail |
| | 时代概览 | `/era/:id` | P1 | 新组件 |
| | 因果链追踪 | `/chronicle/:id/causal` | P2 | 新组件 |
| **经济 Tab** | 经济概览 | `/tabs/economy` | P1 | 新组件 |
| | 物价指数 | `/economy/prices` | P2 | 新组件 |
| | 贸易路线 | `/economy/trade` | P2 | 新组件 |
| **设置 Tab** | 设置首页 | `/tabs/settings` | P0 | Settings（复用） |
| | 性能设置 | `/settings/performance` | P0 | 新组件 |
| | 模拟设置 | `/settings/simulation` | P0 | 新组件 |
| | 存档管理 | `/settings/saves` | P0 | 新组件 |
| | 关于 | `/settings/about` | P1 | AboutDetail（复用） |
| **日志 Tab** | 日志首页 | `/tabs/devlogs` | P0 | DevLogs（复用） |
| | 前端日志 | — | P0 | VirtualLogList（复用） |
| | 后端日志 | — | P0 | VirtualLogList（复用） |
| | 筛选器 | — | P0 | FilterDropdown（复用） |

#### 🖥️ 横屏页面（16 页）— 从世界视图进入

| 分类 | 页面 | 路由 | 优先级 | 说明 |
|------|------|------|--------|------|
| **世界主视图** | 世界地图（横屏） | `/world` | P0 | 左地图 + 右面板 |
| | 世界控制面板 | — | P0 | 暂停/加速/保存 |
| | 实时事件流 | — | P0 | 右侧时间线 |
| **NPC 横屏详情** | NPC 详情（横屏） | `/world/npc/:id` | P1 | 横屏版 NPC 详情 |
| | NPC 属性雷达图 | — | P1 | 文字版雷达 |
| | NPC 技能树 | — | P2 | 文字版技能树 |
| **组织横屏详情** | 组织详情（横屏） | `/world/org/:id` | P1 | 横屏版组织详情 |
| | 组织结构图 | — | P2 | 文字版层级图 |
| **编年史横屏** | 编年史时间线 | `/world/chronicles` | P1 | 横向时间轴 |
| | 事件详情弹窗 | — | P0 | 复用竖屏组件 |
| **经济横屏** | 经济仪表盘 | `/world/economy` | P2 | 横屏版经济概览 |
| **干预模式** | 干预控制台 | `/world/intervention` | P2 | 管理员模式 |
| | 事件触发器 | — | P2 | 手动触发事件 |
| | 属性修改器 | — | P2 | 修改 NPC 属性 |
| **调试工具** | 性能监控 | `/world/debug/perf` | P1 | FPS/CPU/内存 |
| | 实体浏览器 | `/world/debug/entities` | P2 | 按 ID 查实体 |

### 2.3 页面实现优先级

**P0（必须完成，~20 页）**：
- 首页（世界概览）
- 横屏世界视图（地图 + 控制面板 + 事件流）
- NPC 列表 + NPC 详情
- 编年史列表 + 事件详情
- 设置页（性能/模拟/存档）
- DevLogs（复用）
- 所有 Tab 骨架

**P1（重要，~15 页）**：
- 区域列表 + 详情
- 组织列表 + 详情
- NPC 关系网 + 时间线
- 时代概览
- 经济概览
- 性能监控

**P2（锦上添花，~13 页）**：
- NPC 背包/技能树
- 组织领地/结构图
- 因果链追踪
- 物价/贸易
- 干预模式全套
- 实体浏览器

---

## 三、架构设计

### 3.1 目录结构

```
simverse-frontend/src/
├── composables/
│   ├── useSimverse.ts          # API 层（已存在）
│   ├── useScreenOrientation.ts # 横屏切换（已存在）
│   ├── useIonicAutoRegister.ts # Ionic 自动注册（已存在）
│   ├── useWorldControl.ts      # 世界控制（暂停/加速/单步）
│   ├── useNPCList.ts           # NPC 列表逻辑
│   ├── useChronicleList.ts     # 编年史列表逻辑
│   └── useWorldMap.ts          # 地图渲染逻辑
├── components/
│   ├── world/                   # 世界视图组件
│   │   ├── WorldMapGrid.vue     # 地图网格（文字版）
│   │   ├── WorldControlBar.vue  # 底部控制栏
│   │   ├── WorldEventStream.vue # 事件流面板
│   │   ├── WorldStatsPanel.vue  # 统计面板
│   │   └── NPCMarker.vue        # NPC 标记
│   ├── npc/                     # NPC 组件
│   │   ├── NPCCard.vue          # NPC 卡片
│   │   ├── NPCAttributePanel.vue # 属性面板
│   │   ├── NPCStatusBadge.vue   # 状态徽章
│   │   └── NPCRelationItem.vue  # 关系项
│   ├── chronicle/               # 编年史组件
│   │   ├── ChronicleCard.vue    # 事件卡片
│   │   ├── ChronicleTimeline.vue # 时间线
│   │   └── EventTypeIcon.vue    # 事件类型图标
│   ├── org/                     # 组织组件
│   │   ├── OrgCard.vue          # 组织卡片
│   │   └── OrgMemberItem.vue    # 成员项
│   └── shared/                  # 通用组件（复用 shared-components）
├── views/
│   ├── Tabs.vue                 # 标签页（已存在）
│   ├── SimverseHome.vue         # 首页（已存在，待完善）
│   ├── SimverseWorld.vue        # 横屏世界（已存在，待重构）
│   ├── SimverseSettings.vue     # 设置（已存在，待完善）
│   ├── SimverseDevLogs.vue      # 日志（已存在，待完善）
│   ├── NPCList.vue              # NPC 列表
│   ├── NPCDetail.vue            # NPC 详情
│   ├── ChronicleList.vue        # 编年史列表
│   ├── ChronicleDetail.vue      # 事件详情（已存在）
│   ├── OrgList.vue              # 组织列表
│   ├── OrgDetail.vue            # 组织详情
│   ├── RegionList.vue           # 区域列表
│   ├── RegionDetail.vue         # 区域详情
│   ├── EconomyOverview.vue      # 经济概览
│   ├── WorldNPCDetail.vue       # 横屏 NPC 详情
│   ├── WorldOrgDetail.vue       # 横屏组织详情
│   ├── WorldChronicles.vue      # 横屏编年史
│   ├── WorldEconomy.vue         # 横屏经济
│   ├── WorldDebugPerf.vue       # 性能监控
│   ├── PerformanceSettings.vue  # 性能设置
│   ├── SimulationSettings.vue   # 模拟设置
│   ├── SaveManagement.vue       # 存档管理
│   └── EraOverview.vue          # 时代概览
├── router/
│   └── index.ts                 # 路由配置（已存在，待扩展）
├── i18n/                        # （复用 shared-components 的 i18n）
├── theme/
│   └── variables.css            # 主题变量（已存在，待完善）
├── types/
│   └── simverse.ts              # 类型定义（复用 shared-components）
├── App.vue
└── main.ts
```

### 3.2 复用策略

| 组件 | 来源 | 复用方式 | 改造量 |
|------|------|---------|--------|
| Settings 页面 | shared-components | 直接复用，替换配置项 | 小 |
| DevLogs 页面 | shared-components | 直接复用，替换日志源 | 小 |
| VirtualLogList | shared-components | 直接复用 | 无 |
| FilterDropdown | shared-components | 直接复用 | 无 |
| ServerStatusCard | shared-components | 改造为 WorldStatusCard | 中 |
| UnifiedTimelineCard | shared-components | 改造为 ChronicleTimeline | 中 |
| TaskVirtualList | shared-components | 改造为 NPCVirtualList | 中 |
| HomePage 布局 | shared-components | 参考布局，替换内容 | 中 |
| useI18n | shared-components | 直接复用 | 无 |
| useTheme | shared-components | 直接复用 | 无 |
| useToast | shared-components | 直接复用 | 无 |
| useProxiedFetch | shared-components | 直接复用 | 无 |
| useEventBus | shared-components | 直接复用 | 无 |

### 3.3 横屏世界视图布局

```
┌─────────────────────────────────────────────────────────────┐
│  🌍 SimVerse  [暂停] [1x] [10x] [保存] [加载]  ⚙️ 设置   │
├──────────────┬──────────────────────────────────────────────┤
│              │                                              │
│   地图区域    │  右侧面板（可切换 Tab）                       │
│              │                                              │
│  60% 宽度     │  📜 事件流    👥 NPC    🏢 组织    📊 统计  │
│              │                                              │
│  [网格地图]   │  ┌──────────────────────────────────────┐  │
│              │  │ [Tick 1234] 张三出生了                │  │
│  • NPC 标记   │  │ [Tick 1233] 李家商店开业             │  │
│  • 区域高亮   │  │ [Tick 1232] 降雨，农作生长 +10%      │  │
│  • 地形标识   │  │ ...                                  │  │
│              │  └──────────────────────────────────────┘  │
│              │                                              │
├──────────────┴──────────────────────────────────────────────┤
│  🏠 首页  👤 NPC  🏢 组织  📜 编年史  ⚙️ 设置  🚪 退出世界  │
└─────────────────────────────────────────────────────────────┘
```

---

## 四、UI 设计规范（文字图标版）

### 4.1 配色系统（二次元手游风）

| 用途 | 颜色 | 说明 |
|------|------|------|
| 主色 | `#7c3aed`（紫） | SimVerse 品牌色，区别于主应用的蓝 |
| 辅助色 | `#ec4899`（粉） | 强调/高亮 |
| 成功色 | `#22c55e`（绿） | 正常/存活/繁荣 |
| 警告色 | `#f59e0b`（橙） | 警告/生病/紧张 |
| 危险色 | `#ef4444`（红） | 危险/死亡/战争 |
| 信息色 | `#3b82f6`（蓝） | 信息/中性事件 |
| 背景渐变 | `from-purple-900 to-indigo-900` | 横屏世界背景 |
| 卡片背景 | `rgba(30, 27, 75, 0.8)` | 半透明卡片 |
| 边框色 | `rgba(139, 92, 246, 0.3)` | 紫色边框 |

### 4.2 卡片样式规范

```css
/* 通用卡片 */
.sv-card {
  background: rgba(30, 27, 75, 0.8);
  border: 1px solid rgba(139, 92, 246, 0.3);
  border-radius: 12px;
  backdrop-filter: blur(10px);
  padding: 16px;
}

/* 卡片标题 */
.sv-card-title {
  font-size: 16px;
  font-weight: 600;
  color: #e9d5ff;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 卡片内容文字 */
.sv-card-text {
  font-size: 14px;
  color: #c4b5fd;
  line-height: 1.6;
}
```

### 4.3 图标使用规范

| 类别 | 图标名（ionicons） | 用途 |
|------|-------------------|------|
| 世界 | `globe-outline` / `planet-outline` | 世界/地图 |
| NPC | `person-outline` / `people-outline` | NPC/人物 |
| 组织 | `business-outline` / `flag-outline` | 组织/势力 |
| 事件 | `newspaper-outline` / `time-outline` | 编年史/事件 |
| 经济 | `cash-outline` / `trending-up-outline` | 经济/贸易 |
| 战斗 | `sword-outline` / `shield-outline` | 冲突/战争 |
| 生命 | `heart-outline` / `pulse-outline` | 健康/状态 |
| 时间 | `play-outline` / `pause-outline` | 控制按钮 |
| 设置 | `settings-outline` | 设置 |
| 保存 | `save-outline` | 存档 |

### 4.4 NPC 状态标识

| 状态 | 图标 | 颜色 | 说明 |
|------|------|------|------|
| 存活 | `heart` | 绿色 | 正常存活 |
| 生病 | `medkit` | 黄色 | 生病状态 |
| 受伤 | `bandage` | 橙色 | 受伤状态 |
| 死亡 | `skull` | 红色 | 已死亡 |
| 忙碌 | `hourglass` | 蓝色 | 正在执行任务 |
| 休眠 | `moon` | 灰色 | 冷数据/休眠 |

---

## 五、encv-mobile 精简计划

### 5.1 现状

encv-mobile 目前 89,435 行，shared-components 88,486 行。
encv-mobile 独有文件仅 4 个（App.vue, main.ts, router, DebugConsole.vue）。

但实际代码量并未减少多少，因为：
1. encv-mobile 还在引用自己 src 下的文件（没有完全切换到 shared-components）
2. 测试文件还在 encv-mobile 里

### 5.2 精简目标

**encv-mobile 代码量减少 60% 以上**（从 ~90K 行降到 ~35K 行以下）

### 5.3 精简方案

| 步骤 | 内容 | 预计减少行数 |
|------|------|-------------|
| 1 | views/ 目录全部从 shared-components 导入，删除本地副本 | ~15K |
| 2 | components/ 目录全部从 shared-components 导入，删除本地副本 | ~25K |
| 3 | composables/ 目录全部从 shared-components 导入，删除本地副本 | ~30K |
| 4 | api/ 目录全部从 shared-components 导入，删除本地副本 | ~10K |
| 5 | i18n/ 目录全部从 shared-components 导入，删除本地副本 | ~2K |
| 6 | lib/ 目录全部从 shared-components 导入，删除本地副本 | ~5K |
| 7 | types/ 目录全部从 shared-components 导入，删除本地副本 | ~1K |
| **合计** | | **~88K 行** |

encv-mobile 最终保留：
- `App.vue` — 应用入口（~50 行）
- `main.ts` — 启动文件（~100 行）
- `router/index.ts` — 路由配置（~500 行）
- `components/DebugConsole.vue` — 调试专用（~200 行）
- `__tests__/` — 测试文件（保留，约 ~10K 行）
- 其他配置文件

**最终 encv-mobile src 代码量：~11K 行（减少 87%）** ✅ 远超 60% 目标

### 5.4 实施步骤

1. **确认 shared-components 导出完整性** — 检查 `index.ts` 是否导出了所有需要的组件/composable
2. **修改 encv-mobile 引用路径** — 把所有 `@/xxx` 改为 `@encv/shared-components/xxx`
3. **删除 encv-mobile 重复文件** — 确认无引用后删除
4. **验证构建** — `npm run build` + `npm run typecheck` 全通过
5. **验证运行** — 启动应用确认功能正常

---

## 六、P0 实施计划（20 页骨架 + 精简）

### 6.1 任务分解（按优先级）

#### Phase 1：基础框架（~2 天）

| 任务 | 内容 | 产出 |
|------|------|------|
| T1 | 扩展路由配置 | 40+ 路由定义，Tab 结构完整 |
| T2 | 主题系统完善 | 紫色系二次元主题，CSS 变量完整 |
| T3 | 布局组件封装 | SvCard / SvPanel / SvButton 等基础组件 |
| T4 | 确认 shared-components 导出 | index.ts 补充缺失导出 |
| T5 | i18n simverse 模块完善 | 40+ 页面的文案 key |

#### Phase 2：竖屏页面骨架（~3 天）

| 任务 | 内容 | 页面数 |
|------|------|--------|
| T6 | 首页完善 | 世界概览卡片 + 快速入口网格 | 1 |
| T7 | NPC 模块 | 列表页 + 详情页 + 属性面板组件 | 3 |
| T8 | 编年史模块 | 列表页 + 详情页（复用）+ 事件卡片组件 | 2 |
| T9 | 设置模块 | 性能设置 + 模拟设置 + 存档管理 | 3 |
| T10 | DevLogs 完善 | 前端/后端日志切换，复用 VirtualLogList | 1 |
| T11 | 组织模块骨架 | 列表页 + 详情页 | 2 |
| T12 | 区域模块骨架 | 列表页 + 详情页 | 2 |
| T13 | 经济模块骨架 | 概览页 | 1 |

#### Phase 3：横屏世界视图（~2 天）

| 任务 | 内容 | 组件数 |
|------|------|--------|
| T14 | 横屏布局框架 | 左地图 + 右面板 + 底部菜单 | 布局 |
| T15 | 地图网格组件 | 文字版网格地图 + NPC 标记 | 2 |
| T16 | 控制面板组件 | 暂停/加速/保存/加载按钮 | 1 |
| T17 | 事件流面板 | 实时事件列表 + Tab 切换 | 1 |
| T18 | 统计面板 | 世界状态数据展示 | 1 |
| T19 | 横屏详情页骨架 | NPC/组织/编年史横屏页 | 3 |

#### Phase 4：encv-mobile 精简（~2 天）

| 任务 | 内容 | 验证 |
|------|------|------|
| T20 | shared-components index.ts 全量导出 | 所有组件/composable 可导入 |
| T21 | encv-mobile 改为引用 shared-components | 所有 import 路径替换 |
| T22 | 删除 encv-mobile 重复文件 | 删除后 typecheck 通过 |
| T23 | 构建验证 + 运行验证 | 双端正常 |

#### Phase 5：验证与优化（~1 天）

| 任务 | 内容 |
|------|------|
| T24 | typecheck 零错误 |
| T25 | 40+ 页面路由全部可达 |
| T26 | 横屏/竖屏切换正常 |
| T27 | 响应式适配（不同屏幕尺寸） |

### 6.2 验收标准

1. **页面数量**：至少 40 个路由可达的页面骨架
2. **代码质量**：`npm run typecheck` 零错误
3. **视觉效果**：紫色系二次元风格，卡片式布局，基础美观
4. **复用率**：simverse-frontend 60% 以上组件复用 shared-components
5. **encv-mobile 精简**：代码量从 ~90K 行降到 ~35K 行以下（减少 60%+）
6. **构建通过**：`npm run build` 成功
7. **路由完整**：所有页面都能通过路由访问

---

## 七、风险与应对

| 风险 | 概率 | 影响 | 应对措施 |
|------|------|------|---------|
| shared-components 导出不全 | 中 | 高 | 提前梳理导出清单，逐个补全 |
| encv-mobile 精简后构建失败 | 中 | 高 | 逐个模块替换+验证，不一次性全删 |
| 40+ 页面开发量过大 | 中 | 中 | 页面骨架用模板生成，只保证能进，不做功能 |
| 横屏适配问题多 | 高 | 中 | 用 CSS Grid + 相对单位，降级方案完善 |
| i18n key 爆炸 | 低 | 低 | 先只做中文，i18n 骨架留着后续补 |

---

## 八、参考文件

- [00-overview.md](./00-overview.md) — SimVerse 总览
- [frontend-phased-plan.md](./frontend-phased-plan.md) — 旧版前端计划
- [p0-completion-report.md](./p0-completion-report.md) — 旧版 P0 完成报告

---

*文档版本：v0.1*
*创建日期：2026-07-06*
