# plugin-openlist/web 共享组件适配记录

> 日期：2026-07-07
> 状态：已完成核心适配，页面可正常渲染

## 一、完成的工作

### 1.1 android.yml 三插件独立开关（已完成）

将单一 `skip_plugin` 开关拆分为三个独立开关：
- `skip_mpv_plugin` - MPV 播放器插件
- `skip_openlist_plugin` - OpenList 插件
- `skip_simverse_plugin` - SimVerse 插件

每个插件的构建步骤都使用各自的条件判断，避免一个插件构建失败影响整个 CI。

### 1.2 setup-sandbox-env.sh 环境依赖（已完成）

在当前环境运行 setup-sandbox-env.sh 安装依赖，不影响 CI。

### 1.3 plugin-openlist/web 适配 pnpm 工作区共享组件

#### 配置变更

**vite.config.ts**：
- 新增 `@encv/shared-components` alias，指向 `../../../../app/packages/shared-components/src`

**tsconfig.json**：
- 新增 `@encv/shared-components/*` 和 `@encv/shared-components` 路径映射

**package.json**：
- 已有 `"@encv/shared-components": "workspace:*"` 依赖

#### 关键修复：Ionic 组件注册

**问题**：Ionic Vue 组件（`ion-app`, `ion-page` 等）不显示，控制台报 "Failed to resolve component" 警告。

**根因**：`@ionic/vue` 的 `IonicVue` 插件在 CE 构建模式下不全局注册 Vue 组件，只初始化 Web Components。

**解决方案**：参考 encv-mobile 主应用，在 main.ts 中调用 `registerIonicComponents()` 手动注册所有 Ionic Vue 组件。

```typescript
import { registerIonicComponents } from "@encv/shared-components/composables/useIonicAutoRegister";

const app = createApp(App);
app.use(IonicVue);
app.use(router);

const { registered: ionicRegistered } = registerIonicComponents(app);
console.log(`[ionic] Registered ${ionicRegistered.length} Ionic Vue components`);
```

### 1.4 页面骨架复用

#### 设置页面（OpenListSettings.vue）

使用 `@encv/shared-components` 的 Settings 系列组件：
- `SettingsPage` - 页面骨架（header + content + 返回按钮）
- `SettingsGroup` - 设置分组（带标题的列表）
- `SettingsItem` - 设置项（图标 + 标题 + 描述 + 点击）

**导入方式**：直接从组件文件导入（避免 barrel index.ts 的导出错误）
```typescript
import SettingsPage from "@encv/shared-components/components/settings/SettingsPage.vue";
import SettingsGroup from "@encv/shared-components/components/settings/SettingsGroup.vue";
import SettingsItem from "@encv/shared-components/components/settings/SettingsItem.vue";
```

#### DevLogs 页面（OpenListDevLogs.vue）

新增 devlogs 页面，使用 `DevLogsViewer` 共享组件：
- 前端/后端日志 tab 切换
- 日志级别过滤
- 搜索功能
- 自动滚动、复制、清除按钮

**路由**：`/devlogs`

**导入方式**：
```typescript
import DevLogsViewer from "@encv/shared-components/components/DevLogsViewer.vue";
```

#### 主页（OpenListHome.vue）

- 新增日志查看入口按钮（documentTextOutline 图标）
- 修复图标导入（从 ionicons/icons 显式导入）
- 修复函数名（去掉下划线前缀，与模板一致）

## 二、浏览器验证结果

### 2.1 主页（/home）

✅ 正常显示：
- 顶部导航栏（OpenList 标题 + PREVIEW 徽章）
- 工具栏 5 个图标按钮（密码、配置、日志、Web UI、设置）
- 沙箱 Preview 模式提示卡片
- 实时状态显示

### 2.2 设置页（/settings）

✅ 正常显示：
- 返回按钮 + "设置" 标题
- PREVIEW BUILD 横幅
- "基本信息"分组（版本、数据目录、监听端口）
- "操作"分组（打开 Web UI、返回主页、返回 ENCV 主页面）
- SettingsPage/SettingsGroup/SettingsItem 共享组件正常工作

### 2.3 DevLogs 页（/devlogs）

✅ 结构正常，i18n 文本待完善：
- 标题 "OpenList 日志"
- 前端/后端 tab 切换
- 日志级别过滤器
- 自动滚动、复制、清除按钮
- 搜索框
- 状态行 "前端日志 0 条"

⚠️ 已知问题：i18n key 显示为 `[MISSING: devlogs.xxx]`，需要初始化 i18n 字典。

## 三、已知问题和后续工作

### 3.1 类型错误（vue-tsc）

存在较多类型错误，大部分是原代码就有的问题（其他文件的函数名、图标导入等）。本次修改引入的新文件类型基本正确。

### 3.2 i18n 缺失

DevLogsViewer 和 SettingsPage 等共享组件使用 `useI18n`，但 plugin-openlist/web 没有初始化 i18n 字典，导致显示 `[MISSING: xxx]`。

**后续可优化**：初始化 i18n 或传入默认文本。

### 3.3 SettingsItem description 不支持模板插值

SettingsItem 的 `description` prop 是纯字符串，不支持 `{{ port || 5244 }}` 插值。需要使用 slot 或其他方式传递。

## 四、文件变更清单

### 修改的文件

1. `.github/workflows/android.yml` - 三插件独立开关
2. `app/encv-mobile/plugin-openlist/web/vite.config.ts` - shared-components alias
3. `app/encv-mobile/plugin-openlist/web/tsconfig.json` - shared-components 路径映射
4. `app/encv-mobile/plugin-openlist/web/src/main.ts` - registerIonicComponents
5. `app/encv-mobile/plugin-openlist/web/src/App.vue` - 图标导入 + 函数名修复
6. `app/encv-mobile/plugin-openlist/web/src/views/OpenListHome.vue` - 图标导入 + 函数名 + devlogs 入口
7. `app/encv-mobile/plugin-openlist/web/src/views/OpenListSettings.vue` - 改用 Settings 共享组件
8. `app/encv-mobile/plugin-openlist/web/src/router/index.ts` - 新增 devlogs 路由

### 新增的文件

1. `app/encv-mobile/plugin-openlist/web/src/views/OpenListDevLogs.vue` - DevLogs 页面
