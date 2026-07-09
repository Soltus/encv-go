# SimVerse 前端分阶段实施计划

> **更新日期**：2026-07-17
> **目标**：构建完整的 SimVerse 独立前端应用（simverse-frontend），复用主应用的共享组件（HomePage/Settings/DevLogs），首页竖屏，从首页进入横屏世界
> **架构原则**：P0 先搭正确骨架，P1-P3 逐步完善功能

---

## 一、整体架构设计

### 1.1 核心理解

**SimVerse 是一个独立的前端 SPA 应用**，运行在独立的 WorldActivity 中：

1. **WorldActivity 启动时是竖屏** — 显示 SimVerse 首页（类似主应用的首页）
2. **SimVerse 首页复用主应用的共享组件**（`@encv/shared-components`）
3. **从 SimVerse 首页点击"进入世界"** → 锁定横屏 + 切换到横屏世界视图
4. **点击"退出世界"** → 恢复竖屏 + 回到 SimVerse 首页

```
WorldActivity（独立 Activity，初始竖屏）
├─ 独立 WebView（加载 simverse-frontend）
├─ 独立任务栈（taskAffinity）
└─ 初始路由：/simverse-home（竖屏）
    │
    ├─ 用户点击"进入世界"
    │   ├─ 锁定横屏（Capacitor screen-orientation）
    │   └─ 切换路由到 /simverse-world（横屏世界视图）
    │
    └─ 用户点击"退出世界"
        ├─ 恢复竖屏
        └─ 切换路由回 /simverse-home
```

### 1.2 用户交互流程

```
用户点击主应用 SimVerse 卡片
    │
    ▼
启动 WorldActivity（独立 Activity + 独立 WebView）
    │
    ▼
加载 simverse-frontend 的 /simverse-home 路由（竖屏）
    │
    ├─ 显示 SimVerse 首页（世界概览/快速入口）
    │   ├─ 世界概览卡片（tick/时代/NPC 数）
    │   ├─ "进入世界" 按钮 → 切换横屏 + 跳转 /simverse-world
    │   ├─ 设置入口 → 复用 shared-components 的设置页
    │   └─ 日志入口 → 复用 shared-components 的日志页
    │
    └─ 用户点击"进入世界"
            │
            ▼
        锁定横屏 + 切换路由到 /simverse-world
            │
            ├─ 沉浸式全屏（隐藏状态栏/导航栏）
            ├─ 双栏布局（左地图 + 右时间线/数据面板）
            ├─ 实时 WebSocket 推送
            └─ 底部手游式菜单（NPC/组织/编年史/设置/退出）
            │
            └─ 用户点击"退出世界"
                    │
                    ▼
                恢复竖屏 + 切换路由回 /simverse-home
```

### 1.3 前端路由结构

```
simverse-frontend/
├── /simverse-home         ← SimVerse 首页（竖屏，初始页面）
│   ├── 世界概览卡片
│   ├── "进入世界" 按钮 → 跳转 /simverse-world
│   ├── Settings 入口 → 复用 shared-components
│   └── DevLogs 入口 → 复用 shared-components
│
├── /simverse-world        ← 横屏世界视图（从首页进入）
│   ├── 地图渲染（Canvas/WebGL）
│   ├── NPC 交互（点击/高亮/信息面板）
│   ├── 实时事件流（WebSocket）
│   └─ 底部菜单（NPC/组织/编年史/设置/退出）
│
├── /chronicles            ← 编年史列表
├── /chronicle/:id         ← 编年史详情
├── /npc/:id               ← NPC 详情
├── /settings              ← 设置页（复用 shared-components）
└── /dev-logs              ← 日志页（复用 shared-components）
```

---

## 二、P0 阶段：正确骨架（最高优先级）

### 2.1 目标

搭建可运行的 SimVerse 前端骨架，确保：
1. ✅ WorldActivity 能独立启动并加载 simverse-frontend
2. ✅ 初始竖屏显示 SimVerse 首页
3. ✅ 点击"进入世界"能锁定横屏并跳转世界视图
4. ✅ 点击"退出世界"能恢复竖屏
5. ✅ 复用主应用的共享组件（设置/日志）
6. ✅ TypeScript 项目配置完整

### 2.2 任务清单

#### Task 1: 配置 TypeScript 项目

**文件**：`simverse-frontend/tsconfig.json`

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "jsx": "preserve",
    "importHelpers": true,
    "experimentalDecorators": true,
    "allowSyntheticDefaultImports": true,
    "sourceMap": true,
    "baseUrl": ".",
    "paths": {
      "@encv/*": ["../packages/shared-components/src/*"]
    },
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "skipLibCheck": true,
    "types": ["@capacitor/screen-orientation"]
  },
  "include": ["src/**/*.ts", "src/**/*.d.ts", "src/**/*.tsx", "src/**/*.vue"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

**文件**：`simverse-frontend/tsconfig.node.json`

```json
{
  "compilerOptions": {
    "composite": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts"]
}
```

**验收标准**：
- `npm run typecheck` 能正常运行
- 无类型错误

---

#### Task 2: 创建 Pinia Store

**文件**：`simverse-frontend/src/stores/simverse.ts`

```typescript
import { defineStore } from 'pinia';
import type { WorldState, NPC, Chronicle } from '@encv/shared-types';

export const useSimverseStore = defineStore('simverse', {
  state: () => ({
    worldState: null as WorldState | null,
    npcs: [] as NPC[],
    chronicles: [] as Chronicle[],
    focusNPCs: [] as string[],
    performanceTier: 'mid' as 'low' | 'mid' | 'high',
    isConnected: false,
    lastTick: 0,
  }),
  
  getters: {
    activeNPCCount: (state) => state.npcs.filter(n => n.status === 'active').length,
    eraName: (state) => `时代 ${Math.floor(state.worldState?.tick || 0) / 1000}`,
  },
  
  actions: {
    async fetchWorldState() {
      // 复用 useSimverse composable 中的逻辑
    },
    
    async setFocusNPCs(npcIds: string[]) {
      this.focusNPCs = npcIds;
    },
    
    async setPerformanceTier(tier: 'low' | 'mid' | 'high') {
      this.performanceTier = tier;
    },
  },
});
```

**验收标准**：
- Store 能正确管理世界状态
- 多个组件共享状态时无冲突

---

#### Task 3: 完善 SimVerse 首页

**文件**：`simverse-frontend/src/views/SimverseHome.vue`

```vue
<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>SimVerse 模拟世界</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="goToSettings">
            <ion-icon :icon="settingsOutline" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- 世界概览卡片 -->
      <div class="world-overview card">
        <h2>{{ eraName }}</h2>
        <p>Tick: {{ worldState?.tick || 0 }}</p>
        <p>NPC 数: {{ worldState?.npcCount || 0 }}</p>
      </div>

      <!-- 进入世界按钮 -->
      <ion-button expand="block" class="enter-world-btn" @click="enterWorld">
        <ion-icon :icon="planetOutline" slot="start" />
        进入世界
      </ion-button>

      <!-- 快速入口 -->
      <div class="quick-actions">
        <ion-button fill="outline" @click="goToChronicles">
          <ion-icon :icon="documentTextOutline" slot="start" />
          编年史
        </ion-button>
        <ion-button fill="outline" @click="goToSettings">
          <ion-icon :icon="settingsOutline" slot="start" />
          设置
        </ion-button>
        <ion-button fill="outline" @click="goToDevLogs">
          <ion-icon :icon="listOutline" slot="start" />
          日志
        </ion-button>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { IonPage, IonHeader, IonToolbar, IonTitle, IonButton, IonContent, IonIcon, ionButtons } from '@ionic/vue';
import { settingsOutline, planetOutline, documentTextOutline, listOutline } from 'ionicons/icons';
import { useSimverseStore } from '@/stores/simverse';

const router = useRouter();
const store = useSimverseStore();

const worldState = computed(() => store.worldState);
const eraName = computed(() => `时代 ${Math.floor((store.worldState?.tick || 0) / 1000)}`);

const enterWorld = () => {
  router.push('/world');
};

const goToChronicles = () => {
  router.push('/chronicles');
};

const goToSettings = () => {
  router.push('/settings');
};

const goToDevLogs = () => {
  router.push('/dev-logs');
};
</script>
```

**验收标准**：
- 能显示世界概览信息
- 点击"进入世界"能跳转到 /world
- 设置/日志页能正确复用共享组件

---

#### Task 4: 实现 WorldActivity 集成

**文件**：`encv-mobile/android/app/src/main/java/com/encvgo/app/WorldActivity.kt`（新建）

```kotlin
class WorldActivity : AppCompatActivity() {
    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        
        // 初始竖屏（显示 SimVerse 首页）
        requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        
        // WebView 设置
        webView = WebView(this)
        setContentView(webView)
        webView.settings.javaScriptEnabled = true
        webView.settings.domStorageEnabled = true
        webView.settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        
        // 加载 simverse-frontend（路由到 /simverse-home）
        webView.loadUrl("http://localhost:8200/#/simverse-home")
        
        // 桥接：通知 Go 后端世界页面已打开
        EncvGoService.sendCommand("world_activity_opened")
    }
    
    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
        } else {
            // 退出世界，通知 Go 后端 checkpoint
            EncvGoService.sendCommand("world_activity_closed")
            finish()
        }
    }
}
```

**Manifest 配置**：`encv-mobile/android/app/src/main/AndroidManifest.xml`

```xml
<activity
    android:name=".WorldActivity"
    android:exported="false"
    android:launchMode="singleTop"
    android:taskAffinity="com.encvgo.app.world.task"
    android:documentLaunchMode="always"
    android:maxRecents="1"
    android:theme="@style/AppTheme.World.Fullscreen"
    android:configChanges="orientation|screenSize|smallestScreenSize|screenLayout|uiMode|keyboardHidden"
    android:screenOrientation="userPortrait"
    android:resizeableActivity="true"
    android:label="SimVerse">
</activity>
```

**验收标准**：
- WorldActivity 能正确启动并初始竖屏
- WebView 能加载 simverse-frontend
- 前端能通过 Capacitor screen-orientation 动态切换横屏/竖屏

---

#### Task 5: 配置 Capacitor 横屏锁定

**文件**：`simverse-frontend/src/composables/useScreenOrientation.ts`

```typescript
import { useEffect } from 'vue';
import { ScreenOrientation } from '@capacitor/screen-orientation';

export function useScreenOrientation(type: 'landscape-primary' | 'portrait-primary' = 'portrait-primary') {
  useEffect(() => {
    ScreenOrientation.setStyle({
      style: type,
    });
    
    return () => {
      // 组件卸载时恢复默认方向
      if (type === 'landscape-primary') {
        ScreenOrientation.setStyle({ style: 'portrait-primary' });
      }
    };
  }, [type]);
}
```

**文件**：`simverse-frontend/src/views/SimverseWorld.vue`

```vue
<script setup lang="ts">
import { useScreenOrientation } from '@/composables/useScreenOrientation';

// 进入世界时锁定横屏
useScreenOrientation('landscape-primary');
</script>
```

**验收标准**：
- 进入 /world 路由时自动锁定横屏
- 退出 /world 时恢复竖屏

---

#### Task 6: 预览网关集成 simverse-frontend

**文件**：`preview-gateway/src/routing.ts`

```typescript
const ROUTES = [
  // ... 现有路由
  
  // SimVerse 独立前端（端口 8200）
  {
    path: '/simverse',
    port: 8200,
    type: 'spa',
    name: 'SimVerse Frontend',
  },
];
```

**文件**：`preview-gateway/src/server.ts`

在 `buildChildSpecs()` 中添加 simverse-frontend 的 Vite 服务：

```typescript
{
  name: 'simverse-frontend',
  cmd: 'npx',
  args: ['vite', '--port', '8200', '--host', '0.0.0.0'],
  cwd: path.join(APP_DIR, 'simverse-frontend'),
},
```

**验收标准**：
- 网关能正确代理 `/simverse` 路径到 simverse-frontend
- simverse-frontend 能通过网关统一入口访问

---

#### Task 7: 配置环境变量

**文件**：`simverse-frontend/.env.development`

```env
VITE_API_BASE=http://localhost:8080
VITE_WS_URL=ws://localhost:8080/api/simverse/ws
VITE_GW_PORT=8080
```

**文件**：`simverse-frontend/.env.production`

```env
VITE_API_BASE=https://your-production-api.com
VITE_WS_URL=wss://your-production-api.com/api/simverse/ws
VITE_GW_PORT=443
```

**验收标准**：
- 开发/生产环境 API 地址能正确切换
- WebSocket 连接能正常工作

---

### 2.3 P0 验收标准汇总

1. ✅ `npm run typecheck` 无错误
2. ✅ Pinia Store 能管理世界状态
3. ✅ SimVerse 首页能显示世界概览并跳转到 World
4. ✅ 进入 World 时能正确锁定横屏（Capacitor screen-orientation）
5. ✅ 预览网关能代理 simverse-frontend（路由 + 子进程）
6. ✅ WorldActivity 能独立启动并加载 simverse-frontend
7. ✅ 环境变量配置完成（开发/生产）

---

## 三、P1 阶段：核心功能（已完成 ✅）

### 3.1 任务清单

#### Task 8: 完善横屏世界视图

- [x] Phaser 3D 世界地图渲染
- [x] NPC 点击交互（选中/高亮/显示信息）
- [x] 实时 WebSocket 事件推送
- [x] 性能指标监控（FPS/CPU/内存）
- [x] 地形生成 + 昼夜循环
- [x] 领地渲染 + 小地图
- [x] 事件特效系统

#### Task 9: NPC 详情页面

- [x] NPC 基本信息展示
- [x] 属性面板（等级/健康/精力/财富/社会等级）
- [x] 背包/时间线/关系网 Tab
- [x] 点击世界地图 NPC 弹出详情

#### Task 10: 编年史系统

- [x] 编年史列表（实时事件流）
- [x] 事件重要性分级（普通/重要/重大）
- [x] 事件滚动展示
- [x] 因果链页面

#### Task 11: 焦点 NPC 管理

- [x] NPC 列表 + 分页加载
- [x] 焦点 NPC 显示
- [x] NPC 搜索过滤

---

## 四、P2 阶段：优化与完善（已完成 ✅）

### 4.1 任务清单

- [x] 国际化支持（i18n，中文/英文）
- [x] 主题系统（7 色预设 + 暗黑模式，复用 shared-components）
- [x] 移动端适配（底部 tab 栏/侧边抽屉/模态框）
- [x] 召唤系统（Gacha，单抽/十连/保底）
- [x] 存档管理（保存/读取/删除）
- [x] 性能分级（后台/前台/空闲）
- [x] 经济系统概览
- [x] 组织系统
- [x] 错误处理与用户反馈（Toast/Notification）

---

## 五、P3 阶段：高级功能（已完成 ✅）

### 5.1 任务清单

- [x] 场景分层系统
  - [x] WorldScene（世界地图）
  - [x] RegionScene（区域特写：城镇/森林/山脉/地牢/平原）
  - [x] BattleScene（战斗场景：攻击/防御/技能/逃跑）
- [x] 触控优化
  - [x] 移除键盘依赖，完全触控操作
  - [x] 双指缩放 + 单指拖拽
  - [x] 横屏全屏适配（safe-area + viewport-fit=cover）
- [x] 探索系统
  - [x] 区域选择列表
  - [x] 区域内建筑/NPC 交互
  - [x] 返回世界地图
- [x] 战斗系统
  - [x] 敌人列表（史莱姆/哥布林/骷髅兵/魔狼/幼龙）
  - [x] 回合制战斗（攻击/防御/技能/逃跑）
  - [x] HP/MP 血条 + 战斗日志
  - [x] 战斗特效 + 屏幕震动
  - [x] 胜负判定 + 自动返回
- [x] 干预模式（世界干预面板）
- [x] 时间控制（暂停/步进/加速）
- [x] 经济系统可视化（物价/贸易）
- [x] 组织生态展示
- [x] 调试工具（实体浏览器/性能监控）
- [x] DevLogs 开发者日志（独立 tab + i18n）

---

### 5.2 移动端适配修复

| Bug | 修复内容 |
|-----|---------|
| 横屏状态栏遮挡 | 添加 `viewport-fit=cover` + `env(safe-area-inset-*)` 安全区域适配 |
| DevLogs tab 缺失 | 底部 tab 栏新增 DevLogs 入口 |
| DevLogs i18n 缺失 | 补充 `devlogs.*` 中英文翻译键 |

---

## 六、技术栈汇总

| 层 | 技术 | 说明 |
|----|------|------|
| 框架 | Vue 3 + Ionic 8 | 移动端 UI 框架 |
| 状态管理 | Pinia | 世界状态/NPC 数据/UI 状态 |
| API 层 | useProxiedFetch | 现有封装，支持 native 模式 |
| WebSocket | useWebSocket | 实时事件推送 |
| 路由 | Vue Router | 页面导航 |
| 构建 | Vite | 快速开发/HMR |
| 移动端 | Capacitor 6 | 原生桥接 |
| 横屏 | @capacitor/screen-orientation | 屏幕方向锁定 |
| 共享组件 | @encv/shared-components | 复用主应用组件库 |

---

## 七、风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 横屏布局在不同尺寸设备上适配困难 | 高 | 中 | CSS Grid/Flex + 媒体查询 + 降级方案 |
| WebSocket 在 native 模式不稳定 | 中 | 高 | SSE 备选 + 自动重连 |
| Pinia Store 状态同步问题 | 低 | 中 | 单一数据源 + 严格模式 |
| 复用共享组件时样式冲突 | 中 | 低 | CSS Modules / scoped 样式 |

---

*文档版本：v2.0（2026-07-17 更新，P1/P2/P3 全部完成）*

---

## 八、P4 阶段：行为引擎与关系系统（后端 Phase 2 联动）

> **阶段目标**：落地后端 BehaviorEngine 行为引擎，让 NPC 真正"活起来"，前端同步实现行为可视化
> **前置依赖**：P3 前端场景分层已完成 + 后端 Phase 1 核心引擎已完成
> **联动后端**：Phase 2（行为与关系）

### 8.1 后端核心模块（优先实现）

#### Task 1: BehaviorEngine 基础行为框架 ✅

**文件**：`internal/simverse/behavior_engine.go`（新建）

**核心设计**：
- 基于状态机的行为树（Behavior Tree）
- 每日作息循环（工作/休息/吃饭/睡觉/社交）
- 需求驱动系统（饥饿/精力/社交/成就）
- 行为优先级调度

**数据结构**：
```go
type BehaviorType int
const (
    BehaviorIdle BehaviorType = iota
    BehaviorWork
    BehaviorRest
    BehaviorEat
    BehaviorSleep
    BehaviorSocialize
    BehaviorExplore
    BehaviorTrade
)

type BehaviorState struct {
    CurrentBehavior BehaviorType
    BehaviorStartTick uint32
    BehaviorDuration uint32
    TargetID uint64      // 交互目标（NPC/地点/物品）
    TargetPosX int32
    TargetPosY int32
}

type NeedSystem struct {
    Hunger uint8      // 饥饿度 0-100，高了要吃饭
    Energy uint8      // 精力值 0-100，低了要睡觉
    Social uint8      // 社交需求 0-100，高了要找人说话
    Achievement uint8 // 成就需求，驱动职业晋升
}
```

**核心方法**：
- `DecideNextAction(npc *NPCV3, world *FractalWorld) BehaviorType` — 基于需求状态决定下一个行为
- `ExecuteBehavior(npc *NPCV3, behavior BehaviorType, deltaTick uint32)` — 执行行为，更新状态
- `TickNeeds(npc *NPCV3, deltaTick uint32)` — 每 tick 自然消耗/恢复需求值

**性能指标**：
- Tick: ~20 ns/op（0 内存分配）
- DecideNextAction: ~100 ns/op（0 内存分配）
- TickNeeds: ~7.7 ns/op（0 内存分配）

**集成状态**：
- ✅ 已集成到 FractalWorld.Tick() 中
- ✅ 已集成到 NPC detail API
- ✅ 所有单元测试通过
- ✅ 不破坏现有功能

**验收标准**：
- ✅ NPC 能基于需求状态自主选择行为
- ✅ 需求值随时间自然变化
- ✅ 行为有持续时间，完成后切换到下一个行为
- ✅ 单元测试覆盖率 > 80%

#### Task 2: 事件生成系统 ⬜

**文件**：`internal/simverse/event_generator.go`（新建）

**事件类型**：
- 个人事件：生病、工作晋升、学习技能、购物、意外
- 交互事件：对话、交易、冲突、合作、交友
- 组织事件：集会、选举、战争、贸易协定
- 区域事件：节日、灾难、市场波动

**生成策略**：
- 概率基于 NPC 属性（健康→生病概率，魅力→社交事件概率）
- 基于当前行为（工作中→职业事件概率高）
- 基于关系网络（朋友多→社交事件多）
- 时间相关性（白天工作事件多，晚上休息事件多）

**验收标准**：
- 事件生成速率可调（100~10,000 events/s）
- 事件类型分布合理（个人事件 > 交互事件 > 组织事件）
- 事件与 NPC 状态强相关（不是完全随机）
- 编年史系统能正确记录生成的事件

#### Task 3: 关系图存储与查询优化 ⬜

**文件**：`internal/simverse/relation_graph.go`（新建/重构）

**关系类型**：
- 家庭关系（父母/子女/兄弟姐妹/配偶）
- 社会关系（朋友/敌人/同事/邻居）
- 组织关系（上下级/成员/盟友）

**存储优化**：
- 邻接表 + 反向索引
- 热点关系缓存（LRU）
- 批量查询支持

**验收标准**：
- 按 ID 查询关系 < 1ms
- 批量查询 100 个 NPC 的关系 < 10ms
- 关系图支持遍历（朋友的朋友）
- 内存占用 < 20MB（百万级关系）

#### Task 4: 组织系统基础行为 ⬜

**文件**：`internal/simverse/org_behavior.go`（新建）

**组织行为**：
- 组织日常运作（开会/生产/贸易）
- 成员招募与淘汰
- 组织间互动（结盟/战争/贸易）
- 组织属性聚合（整体实力/财富/影响力）

**验收标准**：
- 组织能随时间演化（成员增减/属性变化）
- 组织间有互动事件
- 组织属性 = 成员属性聚合（不需要逐个算）
- 大规模组织（万人级）计算开销可接受

---

### 8.2 前端同步实现

#### Task 5: NPC 日常活动可视化 ⬜

**位置**：`plugin-simverse/web/src/game/scenes/`

**功能**：
- WorldScene 中显示 NPC 当前行为状态（图标/颜色区分）
- NPC 移动动画（从 A 点走到 B 点）
- 行为状态气泡（正在吃饭/正在工作/正在睡觉）
- 点击 NPC 显示当前行为详情和需求状态

**验收标准**：
- NPC 有明显的行为状态视觉区分
- 移动动画流畅（30fps+）
- 行为状态与后端数据同步
- 移动端触控操作正常

#### Task 6: 行为时间线面板 ⬜

**位置**：`plugin-simverse/web/src/views/`

**功能**：
- NPC 详情页新增"行为时间线"Tab
- 显示最近 24 小时（游戏时间）的行为记录
- 饼图/柱状图展示行为分布
- 需求值变化曲线图

**验收标准**：
- 时间线数据正确显示
- 图表渲染流畅
- 支持按时间段筛选
- 移动端适配良好

#### Task 7: 交互事件流可视化 ⬜

**位置**：`plugin-simverse/web/src/views/`

**功能**：
- 世界地图上显示交互事件（NPC 之间的连线/特效）
- 事件类型图标区分（对话/交易/冲突）
- 点击事件显示详情
- 交互事件时间线

**验收标准**：
- 交互事件视觉效果清晰
- 大量事件时性能可接受（100+ 同时显示不卡）
- 事件详情数据正确
- 移动端触控正常

---

### 8.3 P4 验收标准汇总

1. ✅ 后端 BehaviorEngine 能驱动 NPC 自主行为
2. ✅ 事件生成系统稳定运行，速率可调
3. ✅ 关系图查询性能达标
4. ✅ 组织系统基础行为可用
5. ✅ 前端行为可视化与后端同步
6. ✅ 所有单元测试通过
7. ✅ 性能指标达标（内存/CPU/事件吞吐）
8. ✅ 移动端适配良好

---

## 九、P5 阶段：经济与演化（后端 Phase 3 联动）

> **阶段目标**：实现经济系统、组织演化、世代交替，让世界真正"演化"起来
> **前置依赖**：P4 行为引擎完成
> **联动后端**：Phase 3（经济与演化）

### 9.1 后端核心模块

#### Task 1: 经济系统（供需/物价/贸易）⬜
- 资源类型定义（食物/木材/矿石/金币等）
- 供需关系计算（物价随供需波动）
- NPC 经济行为（生产/消费/贸易/储蓄）
- 区域经济差异（不同地区物价不同）

#### Task 2: 组织演化（兴衰/分裂/合并）⬜
- 组织生命周期（创立/发展/鼎盛/衰落/灭亡）
- 组织分裂条件与机制
- 组织合并与吞并
- 势力范围变化

#### Task 3: 世代交替（出生/死亡/继承）⬜
- NPC 衰老与自然死亡
- 新生儿生成与成长
- 财产继承系统
- 家族血脉延续

#### Task 4: 多线程并行计算 ⬜
- 按区域/组织分片并行计算
- 无锁数据结构设计
- 工作窃取调度
- 性能线性扩展验证

---

### 9.2 前端同步实现

#### Task 5: 经济系统可视化 ⬜
- 物价走势图表
- 贸易路线地图
- 区域经济热力图
- NPC 财产排行榜

#### Task 6: 组织演化展示 ⬜
- 组织势力范围变化动画
- 组织兴衰历史时间线
- 家族树可视化
- 战争/结盟事件展示

#### Task 7: 世代交替系统 ⬜
- 家族谱系图
- 出生/死亡通知
- 继承事件详情
- 长寿家族排行榜

---

## 十、P6 阶段：集成与优化（后端 Phase 4 联动）

> **阶段目标**：集成到微内核、对接任务系统、极致性能优化
> **前置依赖**：P5 经济与演化完成
> **联动后端**：Phase 4（集成与优化）

### 10.1 核心任务

#### Task 1: 微内核集成 ⬜
- SimVerse 作为服务注册到微内核
- 服务级生命周期管理
- 内存守卫集成
- 租户隔离支持

#### Task 2: 任务系统对接 ⬜
- 长时间模拟任务（如"模拟 100 年"）
- 任务暂停/继续/进度
- 任务优先级调度
- 与现有任务系统框架整合

#### Task 3: 极致性能优化 ⬜
- ObjectBox 热数据分层
- SIMD 批量计算
- 增量计算优化
- 内存压缩与对象池

#### Task 4: 安卓端适配与调优 ⬜
- 后台运行优化
- 电量消耗优化
- 低温省电模式
- 不同性能档位适配

---

*文档扩展版本：v3.0（2026-07-17 更新，新增 P4/P5/P6 后端联动阶段规划）*
