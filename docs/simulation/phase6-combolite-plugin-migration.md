# Phase 6 — ComboLite 插件化迁移

> **阶段目标**：将 SimVerse 从独立前端应用完整迁移为 ComboLite 插件的 WebView 前端，保留所有功能并适配插件架构
> **状态**：🔄 进行中
> **开始日期**：2026-07-08
> **前置依赖**：Phase 0-4（前端骨架 + 核心功能）

---

## 一、迁移背景

### 1.1 为什么迁移

| 维度 | 独立应用（旧） | ComboLite 插件（新） |
|------|--------------|-------------------|
| **安装方式** | 单独安装 APK | 主应用内插件市场一键安装 |
| **更新方式** | 应用商店更新 | 插件热更新，不重启主应用 |
| **资源共享** | 各自打包 | 复用主应用的后端服务、数据库、组件库 |
| **包体积** | 完整 APK (~30MB+) | 插件 APK (~0.5MB) + 前端资源 |
| **生命周期** | 独立进程 | 主应用进程内，共享 ClassLoader |

### 1.2 迁移范围

```
simverse-frontend/                    plugin-simverse/web/
├── src/                              ├── src/
│   ├── views/            ──迁移──▶  │   ├── views/
│   ├── components/       ──迁移──▶  │   ├── components/
│   ├── composables/      ──迁移──▶  │   ├── composables/
│   ├── router/           ──迁移──▶  │   ├── router/
│   ├── plugins/          ──改造──▶  │   ├── plugins/
│   │   └── SimVerse.ts  (Capacitor) │   │   └── SimVerse.ts (JSInterface)
│   ├── stores/           ──保留──▶  │   ├── stores/
│   └── main.ts           ──迁移──▶  │   └── main.ts
└── package.json          ──简化──▶  └── package.json
```

---

## 二、迁移进度追踪

### 2.1 文件级迁移清单

| 模块 | 文件 | 状态 | 备注 |
|------|------|------|------|
| **路由** | `src/router/index.ts` | ✅ 已完成 | 两边完全一致 |
| **入口** | `src/main.ts` | ✅ 已完成 | 两边完全一致 |
| **类型定义** | `src/vite-env.d.ts` | ✅ 已完成 | 两边完全一致 |
| **共享组件** | `src/components/SvPagePlaceholder.vue` | ✅ 已完成 | 两边完全一致 |
| **i18n** | `src/i18n/simverse.ts` | ✅ 已完成 | 新增（插件架构整合到主应用 i18n 扫描） |
| **插件封装** | `src/plugins/SimVerse.ts` | ✅ 已完成 | 从 Capacitor 改为 JSInterface，双模式支持 |
| **屏幕方向** | `src/composables/useScreenOrientation.ts` | ✅ 已完成 | 从 Capacitor 改为 JSInterface |
| **数据层** | `src/composables/useSimverse.ts` | ✅ 已完成 | 两边完全一致 |
| **Ionic 自动注册** | `src/composables/useIonicAutoRegister.ts` | ✅ 已完成 | 两边完全一致 |
| **首页** | `src/views/SimverseHome.vue` | ✅ 已修复 | 修复跳转路径 /tabs/world → /world |
| **横屏世界** | `src/views/SimverseWorld.vue` | ✅ 已增强 | 新增退出按钮、插件模式横屏锁定 |
| **设置页** | `src/views/SimverseSettings.vue` | ✅ 已增强 | 新增诊断工具入口 |
| **Tab 布局** | `src/views/Tabs.vue` | ✅ 已完成 | 两边完全一致 |
| **NPC 列表** | `src/views/NPCList.vue` | ✅ 已完成 | 两边完全一致 |
| **NPC 详情** | `src/views/NPCDetail.vue` | ✅ 已完成 | 两边完全一致 |
| **NPC 关系** | `src/views/NPCRelations.vue` | ✅ 已完成 | 两边完全一致 |
| **NPC 时间线** | `src/views/NPCTimeline.vue` | ✅ 已完成 | 两边完全一致 |
| **NPC 背包** | `src/views/NPCInventory.vue` | ✅ 已完成 | 两边完全一致 |
| **组织列表** | `src/views/OrgList.vue` | ✅ 已完成 | 两边完全一致 |
| **组织详情** | `src/views/OrgDetail.vue` | ✅ 已完成 | 两边完全一致 |
| **组织成员** | `src/views/OrgMembers.vue` | ✅ 已完成 | 两边完全一致 |
| **组织领地** | `src/views/OrgTerritory.vue` | ✅ 已完成 | 两边完全一致 |
| **区域列表** | `src/views/RegionList.vue` | ✅ 已完成 | 两边完全一致 |
| **区域详情** | `src/views/RegionDetail.vue` | ✅ 已完成 | 两边完全一致 |
| **编年史列表** | `src/views/ChronicleList.vue` | ✅ 已完成 | 两边完全一致 |
| **编年史详情** | `src/views/ChronicleDetail.vue` | ✅ 已完成 | 两边完全一致 |
| **编年史因果** | `src/views/ChronicleCausal.vue` | ✅ 已完成 | 两边完全一致 |
| **时代概览** | `src/views/EraOverview.vue` | ✅ 已完成 | 两边完全一致 |
| **经济总览** | `src/views/EconomyOverview.vue` | ✅ 已完成 | 两边完全一致 |
| **物价** | `src/views/EconomyPrices.vue` | ✅ 已完成 | 两边完全一致 |
| **贸易** | `src/views/EconomyTrade.vue` | ✅ 已完成 | 两边完全一致 |
| **开发日志** | `src/views/SimverseDevLogs.vue` | ✅ 已完成 | 两边完全一致 |
| **关于** | `src/views/AboutSimverse.vue` | ✅ 已完成 | 两边完全一致 |
| **存档管理** | `src/views/SaveManagement.vue` | ✅ 已完成 | 两边完全一致 |
| **性能设置** | `src/views/PerformanceSettings.vue` | ✅ 已完成 | 两边完全一致 |
| **模拟设置** | `src/views/SimulationSettings.vue` | ✅ 已完成 | 两边完全一致 |
| **世界地图（Tab）** | `src/views/WorldMapView.vue` | ⚠️ 占位符 | 竖屏 tab 内的地图视图，当前为占位符 |
| **世界 NPC 详情** | `src/views/WorldNPCDetail.vue` | ✅ 已完成 | 两边完全一致 |
| **世界组织详情** | `src/views/WorldOrgDetail.vue` | ✅ 已完成 | 两边完全一致 |
| **世界编年史** | `src/views/WorldChronicles.vue` | ✅ 已完成 | 两边完全一致 |
| **世界经济** | `src/views/WorldEconomy.vue` | ✅ 已完成 | 两边完全一致 |
| **世界干预** | `src/views/WorldIntervention.vue` | ✅ 已完成 | 两边完全一致 |
| **世界调试-性能** | `src/views/WorldDebugPerf.vue` | ✅ 已完成 | 两边完全一致 |
| **世界调试-实体** | `src/views/WorldDebugEntities.vue` | ✅ 已完成 | 两边完全一致 |

**统计**：37/38 页面已迁移（97.4%），1 个占位符页面（WorldMapView.vue，竖屏 tab 地图视图）

### 2.2 架构改造清单

| 改造项 | 说明 | 状态 |
|--------|------|------|
| **Capacitor → JSInterface** | 屏幕方向锁定从 Capacitor 插件改为原生 JSInterface 调用 | ✅ 已完成 |
| **双模式支持** | SimVerse.ts 支持原生插件模式 + 网页开发模式 | ✅ 已完成 |
| **横屏世界视图集成** | SimverseWorld.vue 集成插件模式的横屏锁定和退出 | ✅ 已完成 |
| **诊断工具入口** | 设置页面添加诊断工具入口 | ✅ 已完成 |
| **i18n 整合** | 插件 i18n 文件被主应用扫描器发现 | ✅ 已完成 |
| **虚拟 HTTPS 域名** | WebView 使用 https://simverse-plugin.local 替代 file:// | ✅ 已完成 |
| **首页跳转修复** | "进入世界"从 /tabs/world 改为 /world | ✅ 已修复 |
| **构建产物打包** | 前端构建产物放入插件 APK assets/simverse/ | ✅ 已完成 |
| **CI 构建集成** | android.yml 中添加 simverse 前端构建步骤 | ✅ 已完成 |

---

## 三、关键设计决策

### 3.1 虚拟 HTTPS 域名（替代 file:// 协议）

**问题**：file:// 协议加载 HTML 会触发 CORS 限制，Origin 为 null，无法加载 CSS/JS 子资源。

**方案**：使用虚拟 HTTPS 域名 `https://simverse-plugin.local/`，通过 `shouldInterceptRequest` 拦截请求，从 assets 目录读取资源。

**代码位置**：
- 原生：`plugin-simverse/src/main/java/.../SimVerseEmbedWebView.kt`
- 前端：无感知（正常使用相对路径）

**好处**：
- ✅ 正常的 Origin（`https://simverse-plugin.local`）
- ✅ 支持相对路径资源加载
- ✅ 后端 CORS 可精确控制允许的域名

### 3.2 JSInterface 双模式适配

**问题**：前端开发时在浏览器中运行，没有原生 JSInterface；在插件 WebView 中运行时，需要调用原生方法。

**方案**：SimVerse.ts 插件封装层提供双模式：

```typescript
// 检测是否运行在原生插件模式
const nativeBridge = (window as any).SimVerseNative as SimVersePlugin | null;

// 网页开发模式的 fallback 实现
const webImpl: SimVersePlugin = {
  async openWorld(_options) { console.warn("[SimVerse] not available in web mode"); },
  async closeWorld() { /* ... */ },
  // ...
};

// 调用时优先使用原生，fallback 到 web 实现
function callNative<K extends keyof SimVersePlugin>(method: K, ...args) {
  if (nativeBridge && typeof (nativeBridge as any)[method] === "function") {
    return (nativeBridge as any)[method](...args);
  }
  return (webImpl[method] as any)(...args);
}
```

**已替换的 Capacitor 插件**：

| 功能 | Capacitor 插件 | JSInterface 方法 |
|------|--------------|-----------------|
| 屏幕方向锁定 | `@capacitor/screen-orientation` | `lockOrientation()` / `unlockOrientation()` |
| 打开/关闭世界 | （自定义） | `openWorld()` / `closeWorld()` |
| 心跳服务 | （自定义） | `startHeartbeat()` / `stopHeartbeat()` |
| 添加桌面快捷方式 | （自定义） | `addShortcut()` / `isShortcutSupported()` |
| 诊断面板 | （自定义） | `showDiagnostic()` |

### 3.3 前端构建与插件打包分离

**原则**：Gradle 不负责前端构建，前端构建由 CI 或开发者手动执行。

**原因**：
1. Gradle 执行 npm/pnpm 需要 Node.js 环境，构建机可能没有
2. 前端构建是独立的工程，不应与 Android 构建耦合
3. 调试时可以单独改前端，不用重新编译 Kotlin

**工作流**：

```bash
# 1. 前端开发（热更新）
cd plugin-simverse/web
pnpm dev

# 2. 前端构建（产出 dist/）
pnpm build

# 3. 拷贝到 assets 目录（CI 或手动）
cp -r dist/* ../src/main/assets/simverse/

# 4. 构建插件 APK
cd ../..
./gradlew :plugin-simverse:assembleRelease
```

---

## 四、横屏世界视图的特殊处理

世界视图是横屏的，需要特殊处理：

1. **进入时锁定横屏**：`onMounted` 时调用 `lockScreenOrientation("landscape-primary")`
2. **退出时恢复竖屏**：`onUnmounted` 时调用 `unlockScreenOrientation()`
3. **底部菜单加退出按钮**：调用 `closeWorld()` 关闭 Activity
4. **双模式兼容**：网页开发模式下用 `window.history.back()` 替代

---

## 五、待办事项（后续优化）

### P0 — 核心功能

| 任务 | 说明 | 优先级 |
|------|------|--------|
| WorldMapView 竖屏地图 | 竖屏 tab 内的地图视图（当前是占位符） | 中 |
| 世界视图内的子页面路由 | /world/npc/:id 等横屏内的子页面 | 中 |
| 真实后端 API 联调 | 当前 useSimverse 可能用的是 mock 数据 | 高 |

### P1 — 体验优化

| 任务 | 说明 | 优先级 |
|------|------|--------|
| WebView 性能优化 | 开启硬件加速、离屏渲染等 | 中 |
| 横竖屏切换动画 | 平滑过渡动画 | 低 |
| 沉浸式模式 | 隐藏状态栏和导航栏 | 中 |

---

## 六、验证清单

- ✅ TypeScript 类型检查通过
- ✅ Vite 构建成功
- ✅ 所有 37 个页面组件正确导入
- ✅ i18n 词条完整（中英文）
- ✅ 双模式（原生/网页）功能正常
- ⏳ 真机测试：横屏锁定
- ⏳ 真机测试：退出世界关闭 Activity
- ⏳ 真机测试：诊断面板
- ⏳ 真机测试：后端 API 连通性

---

*文档版本：v0.1*
*创建日期：2026-07-08*
