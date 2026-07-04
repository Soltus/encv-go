# Simverse 前端独立重构计划

## 背景与问题

当前 Simverse 前端代码与主应用 `encv-mobile` 混在一起，缺乏清晰的边界：
- `SimverseWorld.vue` 等视图直接放在 `encv-mobile/src/views/`
- `useSimverse.ts`、`useWorldRenderer.ts` 等组合式函数混杂在主应用目录
- `simverse-world.html` 通过 `encv-mobile/vite.config.ts` 的 `rollupOptions.input` 作为多入口构建，并非独立包
- 主应用路由 `/simverse/world` 直接加载 Simverse 世界视图，Simverse 没有独立的 Tab 导航和首页

## 目标架构

```
app/
├── pnpm-workspace.yaml
├── encv-mobile/          # 主应用：去掉 SimVerse 专属代码，依赖 shared-components
├── simverse-frontend/    # SimVerse 独立前端：自有构建、路由、Tab 导航，依赖 shared-components
└── packages/
    └── shared-components/ # 共享页面包：HomePage、Settings、DevLogs、Tabs + 基础工具
```

### 应用行为

**主应用 (`encv-mobile`)**
- 保留 HomePage，其中 SimVerse 卡片点击 → 打开 SimVerse Activity（原生）或跳转 SimVerse 应用 URL（Web）
- 不再包含 `SimverseWorld.vue`、独立入口 HTML、SimVerse 路由
- Settings、DevLogs、Tabs 等页面继续使用共享组件

**SimVerse 应用 (`simverse-frontend`)**
- 独立 Vite 构建，独立 `index.html`
- 自有 Ionic App + Vue Router，底部 Tab 导航：Home / Settings / DevLogs
- Home Tab 复用共享 `HomePage.vue`（带有 SimVerse 卡片），点击卡片 → 进入 `/world`（横屏/全屏）
- Settings Tab 复用共享 `Settings.vue`
- DevLogs Tab 复用共享 `DevLogs.vue`
- `/world` 路由加载 `SimverseWorld.vue`（从主应用迁移过来）

**共享包 (`shared-components`)**
- 导出共享页面组件（Vue SFC）和组合式函数/工具
- 内部使用相对路径互相引用，消费端通过包名导入
- 包含自身需要的 API、子组件、工具，做到自包含

## 模块拆分策略

### shared-components 需包含的内容

共享页面组件：
- `HomePage.vue`（增加 `simverseAction` prop 控制卡片点击行为）
- `Settings.vue` + 其直接子组件
- `DevLogs.vue` + 其直接子组件
- `Tabs.vue`

基础组合式函数（现成，可直接迁移）：
- `useI18n.ts`
- `useEventBus.ts`
- `useTheme.ts`
- `useClipboard.ts`
- `useToast.ts`

基础插件/工具：
- `plugins/SimVerse.ts`（原生桥接）
- `plugins/GoProcess.ts`（`isNative` 等工具）

API 与数据层（被 Settings/DevLogs 依赖）：
- `api/encv.ts`（核心 API 封装）
- `composables/useConfig.ts`
- `composables/useServerStatus.ts`
- `composables/useFrontendLogs.ts`
- `composables/useRealtimeTransport.ts`
- `composables/useFileFeatures.ts`

子组件：
- `components/ConfigFieldItem.vue`
- `components/FilePickerModal.vue`
- `components/shared/FilterDropdown.vue`
- `components/VirtualLogList.vue`

工具与常量：
- `config/schemaParser.ts`
- `constants/player.ts`
- `features/alist-encrypt.ts`
- `utils/IncrementalFilter.ts`

样式与主题：
- `theme/variables.css`
- `styles/timeline-*.css`
- i18n 消息文件

### simverse-frontend 需包含的内容

自有文件（新建）：
- `package.json`
- `vite.config.ts`（独立构建，端口不同，无 HMR guard 插件等主应用专属插件）
- `index.html`
- `src/main.ts`
- `src/App.vue`
- `src/router/index.ts`（Tab 路由：/tabs/home、/tabs/settings、/tabs/devlogs，以及 /world）

从主应用迁移：
- `src/views/SimverseWorld.vue`
- `src/composables/useSimverse.ts`
- `src/composables/useWorldRenderer.ts`

### encv-mobile 需移除的内容

- `src/views/SimverseWorld.vue`
- `src/composables/useSimverse.ts`
- `src/composables/useWorldRenderer.ts`
- `src/simverse-world-main.ts`
- `simverse-world.html`
- `vite.config.ts` 中的 `simverse-world` 入口
- `router/index.ts` 中的 `/simverse/world` 路由
- `src/plugins/SimVerse.ts`（迁移到 shared）

保留但修改：
- `src/views/HomePage.vue` → 改为从 `@encv/shared-components` 导入，调整 SimVerse 卡片点击为打开 SimVerse 应用

## 关键设计决策

### 1. 共享包内部引用方式

`shared-components` 内部所有文件之间使用 **相对路径** 互相引用（如 `import { useI18n } from '../composables/useI18n'`），不依赖 `@/` 别名。这样消费应用的 Vite 别名配置不会影响共享包内部的模块解析。

### 2. 消费端引用方式

两个应用通过包名引用：
```ts
import HomePage from '@encv/shared-components/views/HomePage.vue'
import { useI18n } from '@encv/shared-components'
```

需在 `package.json` 中配置 `exports` 字段实现精确映射。

### 3. HomePage.vue 的 SimVerse 卡片行为参数化

当前 `HomePage.vue` 中 `handleOpenSimverse` 在原生环境调用 `openWorld()`，Web 环境 `router.push("/simverse/world")`。

改为：
- 新增可选 prop：
  ```ts
  const props = defineProps<{
    simverseAction?: 'open-app' | 'enter-world'
  }>()
  ```
- 主应用传入 `simverseAction="open-app"` → 打开 SimVerse Activity / 跳转 SimVerse URL
- SimVerse 应用传入 `simverseAction="enter-world"`（或默认）→ 进入世界视图
- 或通过更通用的 `onSimverseClick` 事件/slot，由父级决定行为

### 4. Vite 配置对齐

- `simverse-frontend` 的 Vite 配置与 `encv-mobile` 类似（Vue 插件、相同 alias 规则、HMR 动态 host 插件），但：
  - 不需要 `devStartGuard`（该包不是主应用守护目标）
  - 不需要 `frontendDepsManifestPlugin`
  - 端口不同（如 `:8200`，preview-gateway 需配置对应 upstream）
  - 不需要多入口（仅单页 `index.html`）

### 5. Go 后端最小改动

后端 `internal/server/simverse_api.go` 中 API 路由 `/api/simverse/*` 保持不变。

新增：为 `simverse-frontend` 的构建产物提供静态文件服务，例如路由 `/simverse/` → 指向 `simverse-frontend/dist/`。

产物目录规划：
- `encv-mobile/dist/` → 根路径 `/`
- `simverse-frontend/dist/` → 子路径 `/simverse/`

可通过嵌入或文件系统挂载实现。

### 6. 原生 Capacitor 入口

当前 `simverse-world.html` 作为 Capacitor 的独立 Activity 入口。重构后：
- `simverse-frontend` 的 `index.html` 成为新的独立 Activity 入口
- 原生桥接 `plugins/SimVerse.ts` 中 `openWorld` 不再直接打开世界，而是打开 SimVerse 应用（Tab 导航页）
- 或者保留不同原生 API：`openSimverseApp()` 打开 Tab 导航，`openWorld()` 直接打开世界（在 SimVerse 应用内部或有快捷方式时使用）

为简化，本次重构：**统一改为打开 SimVerse 应用首页**（Tab 导航）。世界入口由 SimVerse 应用内的 HomePage 卡片提供。

## 实施步骤（按依赖顺序）

### Phase 0：准备与 workspace 配置
1. 更新 `app/pnpm-workspace.yaml`，添加 `packages/*` 和 `simverse-frontend`
2. 创建 `app/packages/shared-components/package.json`
3. 创建 `app/simverse-frontend/package.json`
4. 在两个新包的 `package.json` 中声明依赖（与 `encv-mobile` 相同的 Vue/Ionic 等版本）

### Phase 1：迁移基础共享件到 shared-components
5. 迁移 `useI18n.ts`、`useEventBus.ts`、`useTheme.ts`、`useClipboard.ts`、`useToast.ts`
6. 迁移 `plugins/SimVerse.ts`、`plugins/GoProcess.ts`
7. 迁移 `api/encv.ts`
8. 迁移 `config/schemaParser.ts`、`constants/player.ts`、`features/alist-encrypt.ts`、`utils/IncrementalFilter.ts`
9. 迁移子组件：`ConfigFieldItem.vue`、`FilePickerModal.vue`、`FilterDropdown.vue`、`VirtualLogList.vue`
10. 迁移 `useConfig.ts`、`useServerStatus.ts`、`useFrontendLogs.ts`、`useRealtimeTransport.ts`、`useFileFeatures.ts`
11. 迁移样式与主题文件：`theme/variables.css`、`styles/timeline-*.css`、i18n 文件
12. 创建 `shared-components/src/index.ts` barrel export
13. 在 `shared-components/package.json` 中配置 `exports` 字段

### Phase 2：迁移共享页面到 shared-components
14. 迁移 `Tabs.vue`，确保内部引用使用相对路径
15. 迁移 `HomePage.vue`
    - 更新所有内部 `@/` 引用为相对路径或包名引用
    - 修改 `handleOpenSimverse`，使其行为可配置（prop 或 emit 事件）
16. 迁移 `Settings.vue`，更新所有 `@/` 引用
17. 迁移 `DevLogs.vue`，更新所有 `@/` 引用

### Phase 3：创建 simverse-frontend 应用
18. 创建 `simverse-frontend/index.html`
19. 创建 `simverse-frontend/vite.config.ts`
20. 创建 `simverse-frontend/src/main.ts`
21. 创建 `simverse-frontend/src/App.vue`（轻量 App shell，不需要 service guard）
22. 创建 `simverse-frontend/src/router/index.ts`
    - `/tabs/home` → HomePage（传入 `simverseAction="enter-world"`）
    - `/tabs/settings` → Settings.vue
    - `/tabs/devlogs` → DevLogs.vue
    - `/world` → SimverseWorld.vue
23. 从 `encv-mobile` 迁移：
    - `SimverseWorld.vue` → `simverse-frontend/src/views/SimverseWorld.vue`
    - `useSimverse.ts` → `simverse-frontend/src/composables/useSimverse.ts`
    - `useWorldRenderer.ts` → `simverse-frontend/src/composables/useWorldRenderer.ts`
    - 更新迁移文件的内部引用路径

### Phase 4：改造主应用 encv-mobile
24. 在 `encv-mobile/package.json` 中添加 `@encv/shared-components` workspace 依赖
25. 修改 `encv-mobile/src/views/HomePage.vue`
    - 删除本地 HomePage.vue（已在 shared）
    - 在主应用对应路由中改为导入共享 HomePage，传入 `simverseAction="open-app"`
    - 或者调整 `handleOpenSimverse` 在主应用上下文中打开 SimVerse 应用
26. 移除 `encv-mobile` 中的 SimVerse 专属文件（SimverseWorld.vue、useSimverse.ts、useWorldRenderer.ts、simverse-world-main.ts、simverse-world.html）
27. 修改 `encv-mobile/vite.config.ts`
    - 从 `rollupOptions.input` 中移除 `simverse-world` 入口
    - 添加 `@shared` alias 指向 `../packages/shared-components/src`（如果消费端需要直接引用）
28. 修改 `encv-mobile/src/router/index.ts`，移除 `/simverse/world` 路由
29. 调整 `encv-mobile` 中所有被迁移到 shared 的文件的导入路径（改为从 `@encv/shared-components` 导入）

### Phase 5：后端最小适配
30. 在 Go 后端新增静态文件路由，将 `/simverse/` 映射到 `simverse-frontend/dist/`
31. 确保构建产物整合：capacitor copy/sync 将 `simverse-frontend/dist` 作为额外 web asset 复制到 Android assets 目录

### Phase 6：构建验证
32. 在 `app/` 目录执行 `pnpm install` 更新 workspace 依赖
33. 验证 `shared-components` 的 barrel export 正确
34. 尝试构建 `encv-mobile`：`pnpm --filter encv-mobile build`
35. 尝试构建 `simverse-frontend`：`pnpm --filter simverse-frontend build`
36. 修复构建错误和类型错误

## 风险与回退

- **依赖项过多**：Settings.vue / DevLogs.vue 的依赖链较长（API、子组件、工具）。如果迁移导致牵一发而动全身，可考虑不在 shared 中放完整页面，而是只共享设计系统级组件（TabsLayout、样式），让两个应用各自维护页面实现。
- **i18n 键冲突**：共享包中的页面使用了 `t('settings.title')`、`t('devlogs.title')` 等键。由于这些键在两个应用中语义相同，共享没有问题。
- **类型引用循环**：`useEventBus.ts` 引用了 `EncvTask` 类型（来自 `@/api/encv`）。迁移时需确保类型定义一并迁移。

## 确认问题

在开始执行前，请确认以下细节：

1. SimVerse 应用中的 Settings 和 DevLogs 与主应用的是否需要 **完全一致**（包括所有子页面、功能），还是只需要外观一致的简化版？这决定了是否需要迁移全部依赖链。
2. 主应用首页 SimVerse 卡片在原生端点击后，是直接打开 SimVerse 应用的 Tab 首页，还是仍希望保留一个"直接进世界"的快捷方式？
3. `simverse-frontend` 的 Vite dev server 端口期望是多少？（建议 8200，与主应用 8100 区分）
4. 后端对 `/simverse/` 静态文件的服务方式是否有偏好（Go embed / 文件系统 / 代理）？
