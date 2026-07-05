# SimVerse 前端分阶段实施计划

> **更新日期**：2026-07-05
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

## 三、P1 阶段：核心功能（下一阶段）

### 3.1 任务清单

#### Task 8: 完善横屏世界视图

- [ ] 实现双栏布局（左地图 + 右时间线/数据面板）
- [ ] NPC 点击交互（选中/高亮/显示信息）
- [ ] 实时 WebSocket 事件推送
- [ ] 性能指标监控（FPS/CPU/内存）

#### Task 8: NPC 详情页面

- [ ] NPC 基本信息展示
- [ ] 属性面板（六维属性/技能/资源）
- [ ] 关系网可视化
- [ ] 时间线（生命事件历史）
- [ ] 进入脑内视图（分形 L1 层）

#### Task 9: 编年史系统

- [ ] 五级编年史列表（个人/家庭/组织/区域/世界）
- [ ] 事件详情弹窗（含因果链导航）
- [ ] 多维度查询（时间/实体/类型/重要性）
- [ ] 时代切换器

#### Task 10: 焦点 NPC 管理

- [ ] 焦点 NPC 列表编辑
- [ ] 焦点层级设置（player/core/near/distant）
- [ ] 推送频率调整（根据焦点层级）

---

## 四、P2 阶段：优化与完善

### 4.1 任务清单

- [ ] 国际化支持（i18n）
- [ ] 主题系统（7 色预设 + 暗黑模式）
- [ ] PWA 支持（离线缓存/安装提示）
- [ ] 移动端适配（底部抽屉/模态框）
- [ ] 错误处理与用户反馈（Toast/Notification）
- [ ] 单元测试/E2E 测试覆盖

---

## 五、P3 阶段：高级功能

### 5.1 任务清单

- [ ] 干预模式（玩家触发事件/修改属性）
- [ ] 时间控制（暂停/加速/跳转）
- [ ] 经济系统可视化
- [ ] 组织生态展示
- [ ] 区域热力图
- [ ] 脑内视图（格系统详情）

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

*文档版本：v1.1（2026-07-05 更新）*
