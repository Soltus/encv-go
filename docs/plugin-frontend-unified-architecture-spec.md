# 插件前端统一架构 Spec

> 日期：2026-07-07
> 状态：待确认
> 涉及项目：plugin-openlist/web、plugin-mpv-player/web、plugin-simverse（已存在，作为参考）

---

## 一、背景与目标

### 1.1 现状

目前三个插件的前端状态不一致：

| 插件 | 前端位置 | 架构 | i18n | vite 插件 | 共享组件 |
|------|---------|------|------|-----------|---------|
| **plugin-openlist** | `plugin-openlist/web/` | Vue 3 + Ionic Vue | ❌ 缺失（显示 `[MISSING: xxx]`） | ❌ 缺失 vueComponentCheck / i18nOptimize | ✅ 部分接入 |
| **plugin-simverse** | `simverse-frontend/`（独立项目） | Vue 3 + Ionic Vue + Pinia | ✅ 已接入 | ⚠️ 仅有 vueComponentCheck | ✅ 已接入 |
| **plugin-mpv-player** | ❌ 无前端（纯 Compose） | Jetpack Compose | N/A | N/A | N/A |

### 1.2 目标

1. **plugin-openlist/web 补全**：接入 i18n 系统 + vueComponentCheck 插件 + i18nOptimize 插件
2. **plugin-mpv-player 新建前端**：在 `plugin-mpv-player/web/` 新建 Vue 3 + Ionic Vue 前端项目，复用骨架、i18n、共享组件、自定义 vite 插件
3. **统一架构**：三个插件前端遵循相同的技术栈和项目结构
4. **保留 Compose**：MPv 视频播放页面继续使用 Jetpack Compose（WebView 不适合视频播放场景），前端用于设置页、播放列表、日志等非实时渲染页面

### 1.3 混合架构说明（MPV 插件）

MPV 插件采用 **WebView + Compose 混合架构**：

```
┌─────────────────────────────────────────────┐
│  MpvPluginEntry (ComboLite 插件入口)        │
├─────────────────────────────────────────────┤
│  ┌───────────────────────────────────────┐  │
│  │  WebView 界面 (适合的场景)            │  │
│  │  - 设置页 (SettingsPage)              │  │
│  │  - 播放列表 / 媒体库                  │  │
│  │  - 日志查看 (DevLogsViewer)           │  │
│  │  - 关于 / 版本信息                    │  │
│  └───────────────────────────────────────┘  │
│  ┌───────────────────────────────────────┐  │
│  │  Compose 界面 (不适合 WebView 的场景) │  │
│  │  - 视频播放页 (MpvPlayerScreen)       │  │
│  │  - 音频播放服务                       │  │
│  │  - 实时视频渲染 (SurfaceView)         │  │
│  └───────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

**决策依据**：
- ✅ **WebView 上显示 Compose**：视频播放页用 Compose，浮层/导航用 WebView，兼容性好
- ✅ **Compose 嵌入 WebView**：不推荐（WebView 里嵌入原生 View 性能差）
- ✅ **混合模式**：WebView 做管理界面，Compose 做播放核心，各司其职

---

## 二、技术栈统一

所有插件前端项目遵循以下技术栈：

| 层级 | 技术 | 来源 |
|------|------|------|
| 框架 | Vue 3 (Composition API + `<script setup>`) | 与主应用一致 |
| UI 组件库 | Ionic Vue 8 | 与主应用一致 |
| 路由 | Vue Router 4 + Ionic Router | 与主应用一致 |
| 状态管理 | Pinia（按需，简单页面不用） | simverse 已用 |
| 国际化 | `@encv/shared-components` 的 useI18n | 与主应用一致 |
| 共享组件 | `@encv/shared-components` | 工作区依赖 |
| 构建工具 | Vite 8 (rolldown) | 与主应用一致 |
| 语言 | TypeScript 5+ | 与主应用一致 |
| Lint / Format | Biome | 与主应用一致 |
| 类型检查 | vue-tsc | 与主应用一致 |

---

## 三、项目结构规范

### 3.1 plugin-openlist/web（现有，补全）

```
plugin-openlist/web/
├── index.html
├── package.json
├── vite.config.ts           # ⚠️ 需更新：加 vueComponentCheck + i18nOptimize
├── tsconfig.json
├── biome.json
├── src/
│   ├── main.ts              # ⚠️ 需更新：加 i18n 初始化
│   ├── App.vue
│   ├── vite-env.d.ts
│   ├── router/
│   │   └── index.ts
│   ├── i18n/                # 🆕 新增
│   │   └── openlist.ts      # 🆕 openlist 专属 i18n 字典
│   ├── theme/
│   │   └── variables.css
│   ├── plugins/
│   │   └── openlist-native.ts
│   ├── components/
│   ├── components-shared/
│   └── views/
│       ├── OpenListHome.vue
│       ├── OpenListSettings.vue
│       ├── OpenListDevLogs.vue    # ✅ 已有
│       ├── OpenListConfigEditor.vue
│       ├── OpenListWebView.vue
│       ├── BackToMain.vue
│       └── NotFoundView.vue
```

### 3.2 plugin-mpv-player/web（新建）

```
plugin-mpv-player/web/
├── index.html
├── package.json
├── vite.config.ts           # 参考 simverse-frontend
├── tsconfig.json
├── biome.json
├── src/
│   ├── main.ts              # IonicVue + registerIonicComponents + i18n
│   ├── App.vue
│   ├── vite-env.d.ts
│   ├── router/
│   │   └── index.ts
│   ├── i18n/
│   │   └── mpv.ts           # mpv 专属 i18n 字典
│   ├── theme/
│   │   ├── variables.css
│   │   └── mpv.css          # mpv 专属主题色
│   ├── plugins/
│   │   └── mpv-native.ts    # 与原生 Compose 交互的 JS Bridge
│   ├── composables/
│   │   └── useMpvPlayer.ts  # MPV 播放控制封装
│   ├── components/
│   └── views/
│       ├── MpvHome.vue         # 主页：最近播放 / 媒体库入口
│       ├── MpvSettings.vue     # 设置页：复用 SettingsPage 组件
│       ├── MpvDevLogs.vue      # 日志页：复用 DevLogsViewer
│       ├── MpvPlaylist.vue     # 播放列表
│       ├── MpvAbout.vue        # 关于 / 版本
│       └── NotFoundView.vue
```

### 3.3 与 Android 构建集成

参考 `plugin-simverse/build.gradle.kts` 的模式：

1. 在 `plugin-mpv-player/build.gradle.kts` 中新增 `buildMpvFrontend` task
2. 构建时自动执行 `pnpm build` 并将产物拷贝到 `src/main/assets/mpv/`
3. `mergeDebugAssets` / `mergeReleaseAssets` 依赖此 task

**保持 Compose 页面不变**：
- `MpvPlayerActivity` / `MpvPlayerScreen` — 视频播放核心，继续用 Compose
- 新增 WebView Activity 或在现有 Activity 中嵌入 WebView 展示管理界面

---

## 四、i18n 系统接入规范

### 4.1 核心机制

`@encv/shared-components` 提供的 i18n 系统：

- **基础模块**：`common`、`errors`、`settings`、`devlogs`（shared-components 内置）
- **扩展模块**：各插件自己的 i18n 字典（如 `openlist.ts`、`mpv.ts`、`simverse.ts`）
- **注册方式**：`registerI18nModule(module)` 或 `registerI18nModules([module1, module2])`
- **使用方式**：`const { t } = useI18n()`，在模板中 `{{ t('key') }}`

### 4.2 main.ts 初始化模板

```typescript
import { IonicVue } from "@ionic/vue";
import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { registerIonicComponents } from "@encv/shared-components/composables/useIonicAutoRegister";
import { useI18n, registerI18nModule } from "@encv/shared-components/composables/useI18n";
import pluginI18n from "./i18n/xxx";  // 插件专属字典

import "@ionic/vue/css/core.css";
import "@ionic/vue/css/normalize.css";
import "@ionic/vue/css/structure.css";
import "@ionic/vue/css/typography.css";
import "./theme/variables.css";

const app = createApp(App).use(IonicVue).use(router);

// 1. 注册 Ionic 组件（必须在 .use(IonicVue) 之后）
const { registered } = registerIonicComponents(app);
console.log(`[ionic] Registered ${registered.length} Ionic Vue components`);

// 2. 注册 i18n 模块（shared-components 已内置 common/errors/settings/devlogs）
registerI18nModule(pluginI18n);

// 3. 全局挂载 $t（可选，方便模板中直接用 $t）
const { t } = useI18n();
app.config.globalProperties.$t = t;

router.isReady().then(() => {
  app.mount("#app");
});
```

### 4.3 插件专属 i18n 字典模板

```typescript
// src/i18n/openlist.ts 或 src/i18n/mpv.ts
export default {
  "zh-CN": {
    "xxx.home.title": "标题",
    "xxx.home.subtitle": "副标题",
    "xxx.settings": "设置",
    "xxx.devLogs": "日志",
    "xxx.about": "关于",
    // ... 更多
  },
  en: {
    "xxx.home.title": "Title",
    "xxx.home.subtitle": "Subtitle",
    "xxx.settings": "Settings",
    "xxx.devLogs": "Logs",
    "xxx.about": "About",
    // ... 更多
  },
};
```

**命名约定**：key 以插件名前缀开头（`openlist.xxx` / `mpv.xxx` / `simverse.xxx`），避免命名空间冲突。

---

## 五、Vite 插件接入规范

### 5.1 必需插件列表

| 插件 | 位置 | 用途 |
|------|------|------|
| `vue()` | `@vitejs/plugin-vue` | Vue 3 SFC 支持 |
| `vueComponentCheckPlugin()` | `@encv/shared-components/vite-plugins/vue-component-check` | 检查 Vue 组件是否正确导入（防止漏 import 导致 ion-page 等问题） |
| `i18nOptimizePlugin()` | `app/encv-mobile/vite-plugins/i18n-optimize` | i18n HMR 热重载 + 构建优化 |
| `injectBaseHref()` | 各插件自己的 vite.config | 注入 base href + 删除 @vite/client（沙箱 dev） |
| `dynamicHmrHostPlugin()` | 各插件自己的 vite.config | 沙箱 dev 动态 HMR host 修复 |

### 5.2 vite.config.ts 模板

```typescript
import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import { vueComponentCheckPlugin } from '@encv/shared-components/vite-plugins/vue-component-check'
// 注意：i18nOptimizePlugin 在主应用 vite-plugins 下，需要相对路径引用
// 或者后续把它也抽到 shared-components 中

export default defineConfig({
  base: process.env.VITE_BASE || './',
  plugins: [
    vueComponentCheckPlugin({
      dev: process.env.NODE_ENV !== 'production',
      failOnError: process.env.NODE_ENV === 'production',
    }),
    vue(),
    // ... 其他插件
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
      '@encv/shared-components': path.resolve(__dirname, '../../../../app/packages/shared-components/src'),
    },
  },
  server: {
    port: XXXX,  // 各插件不同端口
    host: '0.0.0.0',
    allowedHosts: true,
    hmr: false,  // 沙箱 dev 禁用 HMR
    fs: {
      allow: [
        path.resolve(__dirname),
        path.resolve(__dirname, '..', '..', '..'),  // workspace root
        // ... 更多
      ],
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
```

### 5.3 端口分配

| 项目 | 端口 | 说明 |
|------|------|------|
| encv-mobile 主应用 | 8100 | 通过 preview-gateway :16666 访问 |
| simverse-frontend | 8200 | |
| plugin-openlist/web | 5174 | |
| **plugin-mpv-player/web** | **5175** | 🆕 新增 |

### 5.4 i18nOptimizePlugin 的归属问题

**现状**：`i18nOptimizePlugin` 在 `app/encv-mobile/vite-plugins/i18n-optimize.ts`，是主应用私有的。

**问题**：其他插件前端也想用，但文件不在 shared-components 里。

**方案选择**：

| 方案 | 说明 | 优缺点 |
|------|------|--------|
| **A. 相对路径引用** | 各插件 vite.config 用相对路径 `../../../../app/encv-mobile/vite-plugins/i18n-optimize` 引用 | ✅ 不用移动代码<br>❌ 路径深，耦合主应用结构 |
| **B. 抽到 shared-components** | 把 `i18n-optimize.ts` 移到 `packages/shared-components/src/vite-plugins/` | ✅ 干净，所有插件共享<br>❌ 需要移动文件，主应用也要改 import |
| **C. 各项目自己复制一份** | 每个插件前端都有自己的 i18n-optimize | ❌ 代码重复，维护成本高 |

**推荐方案 B**：将 `i18nOptimizePlugin` 抽到 `@encv/shared-components/vite-plugins/` 下，与 `vueComponentCheckPlugin` 放一起。主应用和所有插件都从 shared-components 导入。

---

## 六、共享组件复用规范

### 6.1 必用共享组件

所有插件前端应优先使用以下共享组件（`@encv/shared-components`）：

| 组件 | 用途 | 路径 |
|------|------|------|
| `SettingsPage` | 设置页骨架（header + 返回按钮 + 保存/重置） | `components/settings/SettingsPage.vue` |
| `SettingsGroup` | 设置分组（带标题的列表） | `components/settings/SettingsGroup.vue` |
| `SettingsItem` | 设置项（图标 + 标题 + 描述 + 点击） | `components/settings/SettingsItem.vue` |
| `DevLogsViewer` | 日志查看器（tab + 过滤 + 搜索 + 自动滚动） | `components/DevLogsViewer.vue` |
| `VirtualLogList` | 虚拟滚动日志列表 | `components/VirtualLogList.vue` |

### 6.2 必用 Composables

| Composable | 用途 |
|------------|------|
| `useI18n` | 国际化 |
| `useToast` | Toast 提示 |
| `useClipboard` | 剪贴板 |
| `useDateFormat` | 日期格式化 |
| `useFrontendLogs` | 前端日志 |
| `useEventBus` | 事件总线 |
| `registerIonicComponents` | Ionic 组件自动注册 |
| `useTheme` | 主题色切换 |

### 6.3 导入方式

**组件直接从文件导入**（避免 barrel index.ts 的导出错误）：
```typescript
import SettingsPage from "@encv/shared-components/components/settings/SettingsPage.vue";
```

**Composables 从 barrel 导入**：
```typescript
import { useI18n, registerI18nModule } from "@encv/shared-components/composables/useI18n";
```

---

## 七、页面骨架规范

所有插件前端至少包含以下页面：

| 页面 | 路由 | 复用组件 | 说明 |
|------|------|---------|------|
| 主页 | `/` 或 `/home` | - | 插件入口，展示核心功能入口 |
| 设置页 | `/settings` | `SettingsPage` / `SettingsGroup` / `SettingsItem` | 插件配置 |
| 日志页 | `/devlogs` | `DevLogsViewer` | 前端 + 后端日志查看 |
| 关于页 | `/about` | `SettingsPage` 风格 | 版本信息、开源协议等 |
| 404 页 | `/:pathMatch(.*)*` | - | 未找到页面 |

---

## 八、实施步骤

### Phase 1：plugin-openlist/web 补全

1. 新增 `src/i18n/openlist.ts` 字典
2. 更新 `src/main.ts`：初始化 i18n
3. 更新 `vite.config.ts`：加 `vueComponentCheckPlugin`
4. 更新 `vite.config.ts`：加 `i18nOptimizePlugin`（从 shared-components 导入）
5. 更新页面：把硬编码中文文本替换为 `t()` 调用
6. vue-tsc + biome 检查
7. 浏览器验证

### Phase 2：i18nOptimizePlugin 抽到 shared-components

1. 将 `app/encv-mobile/vite-plugins/i18n-optimize.ts` 移到 `packages/shared-components/src/vite-plugins/`
2. 更新主应用 `vite.config.ts` 的导入路径
3. 更新 simverse-frontend `vite.config.ts`：加上 i18nOptimizePlugin
4. 验证主应用和 simverse 构建正常

### Phase 3：plugin-mpv-player/web 新建

1. 初始化项目结构（package.json / vite.config.ts / tsconfig.json / biome.json）
2. 基础脚手架：main.ts / App.vue / router / theme
3. i18n 初始化 + mpv 专属字典
4. 页面骨架：Home / Settings / DevLogs / About / NotFound
5. 与原生交互的 JS Bridge（mpv-native.ts）
6. Android 构建集成（build.gradle.kts 加 buildMpvFrontend task）
7. vue-tsc + biome 检查
8. 浏览器验证

### Phase 4：四项目联调验证

1. 启动四个前端项目（主应用 + 三个插件）
2. 浏览器分别打开，验证：
   - 页面正常渲染
   - i18n 文本正确显示（无 `[MISSING: xxx]`）
   - 设置页共享组件正常
   - DevLogsViewer 正常
   - 无控制台错误

---

## 九、验收标准

### 9.1 plugin-openlist/web

- ✅ i18n 正常工作，所有页面无 `[MISSING: xxx]`
- ✅ vueComponentCheckPlugin 正常运行（dev 模式控制台有检查输出）
- ✅ i18nOptimizePlugin 正常运行（i18n 文件修改 HMR 生效）
- ✅ vue-tsc 无新增类型错误
- ✅ biome lint/format 通过
- ✅ 浏览器三个页面（home/settings/devlogs）正常渲染

### 9.2 plugin-mpv-player/web

- ✅ 项目能正常启动（vite dev）
- ✅ 基础页面骨架完整（home/settings/devlogs/about/notfound）
- ✅ i18n 正常工作
- ✅ 共享组件正常复用（SettingsPage / DevLogsViewer）
- ✅ vueComponentCheckPlugin + i18nOptimizePlugin 接入
- ✅ vue-tsc 无类型错误
- ✅ biome lint/format 通过
- ✅ Android 构建集成（buildMpvFrontend task 能正常构建）
- ✅ 浏览器所有页面正常渲染

### 9.3 架构一致性

- ✅ 三个插件前端技术栈一致
- ✅ 共享组件 / composables / vite 插件都从 `@encv/shared-components` 导入
- ✅ 项目结构遵循统一规范
- ✅ i18n key 命名遵循前缀约定

---

## 十、风险与注意事项

### 10.1 i18nOptimizePlugin 迁移风险

将 `i18n-optimize.ts` 从主应用移到 shared-components 时，需要确保：
- 不破坏主应用的现有功能
- simverse-frontend 也能正常使用
- 插件引用路径正确

**缓解措施**：先移文件 + 改主应用导入，验证主应用构建正常后，再给插件用。

### 10.2 MPV 插件 WebView 与 Compose 交互

需要设计 JS Bridge 让 WebView 能控制原生播放器：
- 播放/暂停/停止
- 跳转到指定时间
- 获取播放状态
- 打开播放页（跳转到 Compose Activity）

**缓解措施**：参考 plugin-openlist 的 `openlist-native.ts` 模式，设计 `mpv-native.ts`。

### 10.3 沙箱 dev HMR 问题

所有插件前端在沙箱 dev 环境下：
- 禁用 HMR（`hmr: false`）
- 删除 `@vite/client` 脚本
- 使用 `dynamicHmrHostPlugin` 作为备选（如果未来 HMR 可用）

---

## 十一、问题讨论点

需要用户确认的决策点：

1. **i18nOptimizePlugin 归属**：是否同意抽到 shared-components？（推荐方案 B）
2. **MPV 前端路由**：MPv 插件前端的页面结构是否符合预期？（Home / Settings / DevLogs / About + Playlist）
3. **MPV 原生交互**：JS Bridge 需要哪些 API？还是先只做静态页面，后续再加？
4. **Android 集成方式**：MPV 插件的 WebView 是新建 Activity 还是在现有 MpvPlayerActivity 中嵌入？
