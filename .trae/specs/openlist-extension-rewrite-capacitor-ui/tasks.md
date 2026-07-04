# Tasks

## Phase 0: ComboLite 合规修复（与 UI 无关）

- [ ] 0.1 查阅 `combolite-core` AAR 中 `IPluginEntryClass` 接口契约
- [ ] 0.2 对比 MpvPluginEntry vs OpenListPluginEntry 差距
- [ ] 0.3 重写 `plugin-openlist/OpenListPluginEntry.kt`：
  - `pluginModule = emptyList()`（与 MpvPluginEntry 一致）
  - `onLoad(context)` 初始化 OpenListBridge
  - `onUnload()` shutdown OpenListBridge + OpenListService
  - `Content()` 用 `OpenListEmbedWebView` Composable 替代 Compose UI
- [ ] 0.4 删除现有 Compose UI（StatusCard / ControlCard / ConfigCard / InfoGrid / formatFileSize）
- [ ] 0.5 瘦身 `plugin-openlist/build.gradle.kts`（移除 compose plugin / buildFeatures / dependencies）
- [ ] 0.6 `./gradlew :plugin-openlist:compileDebugKotlin` 通过
- [ ] 0.7 `./gradlew :combolite-host:compileDebugKotlin` 通过

## Phase 1: 嵌入式 WebView + JS-Native 桥接

### 1.1 新增 OpenListEmbedWebView Composable

- [ ] 1.1.1 新建 `plugin-openlist/src/main/java/.../OpenListEmbedWebView.kt`
- [ ] 1.1.2 定义 `@Composable fun OpenListEmbedWebView(containerId, initialPath)` 
- [ ] 1.1.3 用 `AndroidView(factory = { WebView })` 创建 WebView
- [ ] 1.1.4 启用 JS / DOM storage / file access
- [ ] 1.1.5 注册 `OpenListPluginJSInterface` (addJavascriptInterface)
- [ ] 1.1.6 设置 WebViewClient 处理页面加载回调

### 1.2 新增 OpenListPluginJSInterface（JS-Native 桥接）

- [ ] 1.2.1 新建 `plugin-openlist/src/main/java/.../OpenListPluginJSInterface.kt`
- [ ] 1.2.2 `@JavascriptInterface fun startOpenList(): String` → `OpenListBridge.start()`
- [ ] 1.2.3 `@JavascriptInterface fun stopOpenList(): Boolean` → `OpenListBridge.stop()`
- [ ] 1.2.4 `@JavascriptInterface fun getRuntimeStatus(): String` → `OpenListBridge.snapshot()` 返回 JSON
- [ ] 1.2.5 `@JavascriptInterface fun setAdminPassword(pwd: String): Boolean` → `OpenListBridge.setAdminPwd()`
- [ ] 1.2.6 `@JavascriptInterface fun readConfig(): String` → 读 config.json
- [ ] 1.2.7 `@JavascriptInterface fun writeConfig(content: String): Boolean` → 写 config.json (自动备份)
- [ ] 1.2.8 `@JavascriptInterface fun getVersion(): String` → 读 OpenListConfig.version

### 1.3 新增 OpenListWebViewClient

- [ ] 1.3.1 新建 `plugin-openlist/src/main/java/.../OpenListWebViewClient.kt`
- [ ] 1.3.2 处理 shouldOverrideUrlLoading（OpenList SPA 内部链接）
- [ ] 1.3.3 处理 onPageStarted / onPageFinished 回调

### 1.4 编译验证

- [ ] 1.4.1 `./gradlew :plugin-openlist:compileDebugKotlin` 通过

## Phase 2: Monorepo 改造（pnpm workspace）

### 2.1 项目根 workspace 配置

- [ ] 2.1.1 新建 `pnpm-workspace.yaml`：
  ```yaml
  packages:
    - 'app'
    - 'plugin-openlist/web'
    - 'packages/*'
  ```
- [ ] 2.1.2 修改项目根 `package.json` 添加 `"packageManager": "pnpm@9.x"`
- [ ] 2.1.3 确认项目根 `package.json` 的 workspaces 字段（如果用 npm workspaces 而非 pnpm）

### 2.2 共享组件包 @encvgo/components

- [ ] 2.2.1 新建 `packages/components/package.json`：
  - name: `@encvgo/components`
  - main: `src/index.ts`
  - exports: `./OpenListStatusCard`, `./OpenListLogList`
  - peerDependencies: vue, @ionic/vue, ionicons
- [ ] 2.2.2 新建 `packages/components/tsconfig.json`
- [ ] 2.2.3 新建 `packages/components/src/index.ts`
- [ ] 2.2.4 新建 `packages/components/src/OpenListStatusCard.vue`（从原 LocalOpenListStatusCard.vue 移植）
- [ ] 2.2.5 新建 `packages/components/src/OpenListLogList.vue`（从 OpenListPluginEntry 中日志组件移植）
- [ ] 2.2.6 验证 `pnpm install` 在 packages/components 成功

### 2.3 插件 web 项目

- [ ] 2.3.1 新建 `plugin-openlist/web/package.json`：
  - name: `@encvgo/plugin-openlist-web`
  - dependencies: `@encvgo/components: workspace:*`, vue, @ionic/vue, ionicons, vue-router, vite, @vitejs/plugin-vue
- [ ] 2.3.2 新建 `plugin-openlist/web/vite.config.ts`
- [ ] 2.3.3 新建 `plugin-openlist/web/tsconfig.json`
- [ ] 2.3.4 新建 `plugin-openlist/web/index.html`
- [ ] 2.3.5 验证 `pnpm install` 在 plugin-openlist/web 成功

## Phase 3: 插件 web 项目页面

### 3.1 入口与路由

- [ ] 3.1.1 新建 `plugin-openlist/web/src/main.ts`
- [ ] 3.1.2 新建 `plugin-openlist/web/src/App.vue`（ion-app 根）
- [ ] 3.1.3 新建 `plugin-openlist/web/src/router/index.ts`
  - 路由：/ → OpenListHome, /config → OpenListConfigEditor, /settings → OpenListSettings

### 3.2 页面

- [ ] 3.2.1 新建 `plugin-openlist/web/src/views/OpenListHome.vue`（K-Sillot OpenListScreen 复刻）
  - AppBar: 标题 + 工具按钮（密码/Config/快捷方式）
  - Body: OpenListStatusCard + OpenListLogList
  - FAB: Start/Stop 切换
  - onMounted: refreshStatus + setInterval 3s
  - toggleService: 调 OpenListNative.start/stop
- [ ] 3.2.2 新建 `plugin-openlist/web/src/views/OpenListConfigEditor.vue`（K-Sillot ConfigEditorPage 复刻）
  - onMounted: `OpenListNative.readConfig()` → 填充 textarea
  - JSON 校验（debounce 300ms）
  - 保存按钮：备份 + writeConfig
  - 三选项 dialog: 取消/仅保存/保存并重启
- [ ] 3.2.3 新建 `plugin-openlist/web/src/views/OpenListSettings.vue`（简化版）
  - 显示 OpenList 版本、数据目录
  - 关于按钮
- [ ] 3.2.4 新建 `plugin-openlist/web/src/views/OpenListWebView.vue`（可选，iframe 加载 OpenList SPA）

### 3.3 插件定义

- [ ] 3.3.1 新建 `plugin-openlist/web/src/plugins/openlist-native.ts`
  - 类型声明 `Window.OpenListNative`
  - 导出 `OpenListNative` 对象包装 window.OpenListNative
  - 方法: start / stop / getStatus / setPassword / readConfig / writeConfig / getVersion

### 3.4 构建配置

- [ ] 3.4.1 `plugin-openlist/web/vite.config.ts` 配置：
  - build.outDir: `dist`
  - build.assetsDir: `assets`
  - 别名 `@` → `src`
- [ ] 3.4.2 `cd plugin-openlist/web && pnpm install`
- [ ] 3.4.3 `pnpm run build` 产出 `dist/`

### 3.5 TypeScript 编译

- [ ] 3.5.1 `cd plugin-openlist/web && npx vue-tsc --noEmit` 通过 (0 errors)

## Phase 4: 主 app 集成（最小改动）

### 4.1 主 app 路由

- [ ] 4.1.1 修改 `app/src/router/index.ts`：
  - 添加 `/openlist` 路由 → `OpenListHome` (从 `@encvgo/plugin-openlist-web/views/OpenListHome.vue` 导入)
- [ ] 4.1.2 验证主 app `pnpm install` + `pnpm run build` 通过

### 4.2 TypeScript 编译

- [ ] 4.2.1 主 app `npx vue-tsc --noEmit` 通过 (0 errors)

## Phase 5: 编译与部署验证

- [ ] 5.1 `./gradlew :plugin-openlist:compileDebugKotlin` 通过
- [ ] 5.2 `./gradlew :combolite-host:compileDebugKotlin` 通过
- [ ] 5.3 `./gradlew :app:compileDebugKotlin` 通过
- [ ] 5.4 插件 web `pnpm run build` 产出 dist/
- [ ] 5.5 主 app `pnpm run build` 产出 dist/
- [ ] 5.6 沙箱预览 `scripts/start-preview.sh` 启动正常
- [ ] 5.7 主 app 路由 `/openlist` 可访问，渲染 OpenListHome
- [ ] 5.8 验证嵌入式 WebView 通过 OpenListNative JSInterface 调 OpenListBridge 成功
- [ ] 5.9 验证 OpenListStatusCard / OpenListLogList 共享组件在主 app 和插件都可用

## Phase 6: plugin-openlist/web 前端开发预览（沙箱浏览器版）

> **与既有 `scripts/dev-openlist.sh` 的关键区别**：
> - `dev-openlist.sh` 预览的是 **OpenList 原生 SPA**（Hi-Sillot-OpenList/public/dist 里的 Vue3 SPA，通过 Vite middleware 反代到 OpenList(5244)）
> - 本 Phase 预览的是 **plugin-openlist/web 的 Capacitor 多例 UI**（plugin 自己的 Vue3 + Ionic Vue 8 管理面板），不依赖 OpenList(5244)，由 `window.OpenListNative` 桥接 Android 端的 OpenListBridge
>
> 沙箱浏览器模式下 `window.OpenListNative` 不存在，所有 JS-Native 调用走 `safe(fallback, fn)` 安全 fallback → 显示「未安装/已停止」默认态。这是预期的「UI 视觉预览」目标。

- [ ] 6.1 新建 `scripts/dev-openlist-web.sh`（沿用 `start-preview.sh` 的铁律风格）
  - 默认端口 5174（plugin-openlist/web vite.config 已设）
  - 端口被占时回退到 5175
  - 启动前清理残留 vite 进程
  - 信号陷阱：Ctrl+C 时优雅 kill 子进程
  - 前台运行（保持 OpenPreview 可激活）
  - 状态报告 + OpenPreview 提示
- [ ] 6.2 `bash scripts/dev-openlist-web.sh` 启动成功
- [ ] 6.3 `curl -s http://localhost:5174/` 返回 200 + HTML
- [ ] 6.4 浏览器访问 OpenListHome（路由 `/home`）看到：
  - AppBar 标题「OpenList - v0.0.0」
  - 4 个工具按钮（密码/Config/Settings/WebView）
  - OpenListStatusCard（默认态：已停止）
  - OpenListLogList（空）
  - FAB 启动按钮（点击走 fallback 不报错）
- [ ] 6.5 访问 `/config` 看到 JSON 编辑器
- [ ] 6.6 访问 `/settings` 看到版本/数据目录占位
- [ ] 6.7 访问 `/webview` 看到「需 Android WebView 容器」提示
- [ ] 6.8 验证 HMR：修改 `OpenListStatusCard.vue` 后浏览器自动刷新

## Phase 7: /webview 嵌入 OpenList 原生 SPA（iframe + Vite proxy）

> **核心目标**：在 Capacitor 多例 UI 的 `/webview` 路由内显示 OpenList 自己的 Vue3 SPA（Hi-Sillot-OpenList-Frontend），让用户能在同一页面里既管理 OpenList 启停，又浏览 OpenList 文件管理。
>
> 沙箱浏览器模式下走 Vite proxy (`/openlist-spa/*` → `http://127.0.0.1:5244/*`)；真机 WebView 内 OpenList 与 Capacitor 同设备 → iframe 直连 5244。
>
> 仍需配合 `bash scripts/dev-openlist.sh` 启动 OpenList(5244) 后端。后端未启动时显示降级 UI「OpenList 后端未运行」+ 启动命令提示。

- [ ] 7.1 修改 `plugin-openlist/web/vite.config.ts`：
  - 添加 `server.proxy['/openlist-spa']` → `http://127.0.0.1:5244`
  - `changeOrigin: true`、`rewrite: path => path.replace(/^\\/openlist-spa/, '')`
- [ ] 7.2 重写 `plugin-openlist/web/src/views/OpenListWebView.vue`：
  - 始终渲染 iframe，`:src` 根据 `import.meta.env.DEV` 切换（dev 走代理，prod 直连）
  - 沙箱模式下 `onMounted` 主动探测 `/openlist-spa/` HEAD 请求（2s 超时）
  - 探测失败 → `showFallback=true` → 显示「OpenList 后端未运行」+ `bash scripts/dev-openlist.sh` 提示 + 重试按钮
  - 真机模式 → 直接假设可达（不探测），始终显示 iframe
  - 顶部 toolbar 增加「外部打开」按钮（仅真机模式）
- [ ] 7.3 重启 dev server：`pkill vite; bash scripts/dev-openlist-web.sh`
- [ ] 7.4 验证 `/openlist-spa/` 代理生效（curl 502 表示代理工作但后端未启）
- [ ] 7.5 验证 `/webview` 页面在浏览器中显示降级 UI（带 `bash scripts/dev-openlist.sh` 命令）
- [ ] 7.6 验证用户运行 `bash scripts/dev-openlist.sh` 后 iframe 自动加载 OpenList SPA（hash 路由 `#/login`）
- [ ] 7.7 验证 `vue-tsc --noEmit` 通过

## Phase 8: iframe 防御性状态 UI（5 态状态机）

> **核心痛点**：之前 `v-show` 简单切 fallback，探测期间 iframe 是空白（用户感知「卡死/没反应」）。Phase 8 引入显式 5 态状态机 + 状态条 + 覆盖层 + 复制命令按钮，让每种异常都有明确视觉反馈。

- [ ] 8.1 重写 `OpenListWebView.vue` 引入 `IframeState` 类型：
  - `'probing'` 探测中（spinner + 「正在连接… 127.0.0.1:5244」）
  - `'loading'` iframe 加载中（半透明 0.6）
  - `'connected'` 已连接（绿色对勾 + 隐藏覆盖层）
  - `'error'` 连接失败/502/404（红色 cloudOffline + lastError + 重试 + 复制命令）
  - `'timeout'` 超时（黄色 timer + 「再试一次」）
- [ ] 8.2 顶部 `ion-toolbar` 状态条（仅 probing/error/timeout 时显示）：
  - 颜色随状态变（medium / danger / warning）
  - 显示 icon + 状态文本
- [ ] 8.3 覆盖层 `.state-overlay` 绝对定位遮挡 iframe 空白区
  - 探测超时从 2s 提升到 5s（更宽容的慢网/慢启动）
  - `mode: 'cors'` 显式探测（不再 no-cors，区分 timeout vs error）
- [ ] 8.4 错误 UI 增加「复制启动命令」按钮（navigator.clipboard + toastController 反馈）
- [ ] 8.5 顶部 toolbar 状态按钮（点一下重试）：
  - icon 随状态变（checkmark / cloudOffline / timer / refresh）
  - 颜色随状态变（success / danger / warning / medium）
  - probing 时禁用（避免重复触发）
- [ ] 8.6 iframe sandbox 属性加固（`allow-scripts allow-same-origin allow-forms allow-popups`）
- [ ] 8.7 验证 `vue-tsc --noEmit` 通过
- [ ] 8.8 验证 `/webview` 页面在 OpenList 未启动时显示「错误」状态卡（带重试 + 复制命令按钮）

## Phase 9: 修复 iframe @load 误判 connected（健康探针 + 后置校验）

> **核心痛点**：Phase 8 实现后用户实测反馈"闪了一下又空白了，右上角显示绿色勾"。根因：
> 1. iframe 加载 502 错误页面（来自 Vite proxy）也会触发 `@load` 事件
> 2. 之前 `onLoad()` 直接 `state.value = 'connected'`，没做后置校验
> 3. 浏览器 fetch with `mode: 'cors'` 对 502 响应做 CORS 拦截 → `res.status === 0`（opaque）
>    → `0 < 500` 落入 `else` 分支 → state='loading'（误判）
> 4. iframe 显示 → @load → state='connected'（误判），但 iframe 是 502 错误页 → 视觉空白

- [ ] 9.1 vite.config.ts 新增 `openlistHealthPlugin()` 中间件：
  - 路由 `/__openlist-health`
  - Node 端 `fetch('http://127.0.0.1:5244/api/public/settings', signal: AbortSignal.timeout(3000))`
  - 响应 JSON：`{ alive, upstreamStatus, latency, target, ts, error?, code? }`
  - 强制 CORS 头（`Access-Control-Allow-Origin: *`）→ 浏览器永远能读 status
  - 区分 alive/timeout/connect-refused
- [ ] 9.2 OpenListWebView.vue `probeBackend` 改用 `/__openlist-health`：
  - Promise.race 实现 5s 前端兜底超时
  - 拿到 alive=true → state='loading'（**不再直接 connected**，等 iframe @load）
  - 拿到 alive=false → state='error'（带 error + code 详细信息）
- [ ] 9.3 `onIframeLoad()` **关键修复**：
  - 不再直接置 connected
  - 改为调 `verifyAfterIframeLoad()` → 再发一次 health check
  - alive=true → state='connected'（确认 iframe 加载的是真 SPA）
  - alive=false → state='error'（iframe 加载的是 502 错误页，自动回退到错误状态卡）
  - SPA 内导航/刷新（state 已是 connected）→ onIframeLoad 直接 return，零开销
- [ ] 9.4 加 `pollHealth()` 周期性校验（10s 一次）：
  - 仅在 state='connected' 时执行
  - 后端突然挂掉 → 自动 transition to 'error'
  - 组件 unmount 时清理 timer
- [ ] 9.5 加 devtools 风格调试面板（右下角 bug FAB 触发）：
  - 记录 `info/warn/error/probe` 四级日志
  - 每次关键事件（onMounted/probe/iframe @load/verify/poll）都打点
  - JSON 序列化 + 200 字符截断
  - 仅 sandbox dev 模式可见
- [ ] 9.6 验证重启 dev server 后 `/__openlist-health` 返回正确 JSON
- [ ] 9.7 验证 iframe @load 后 state='error'（不再误判 connected）
- [ ] 9.8 验证 `vue-tsc --noEmit` 通过

## Phase 10: CI 适配 + 真机测试准备（workspace 协议 + assets 同步 + manifest 修复）

> **核心痛点**：CI 报 `npm error code EUNSUPPORTEDPROTOCOL: workspace:*`，
> 因为仓库里同时存在 `pnpm-workspace.yaml`（用 `workspace:` 协议）但 CI 用 npm。
> 此外还有多个真机测试前必须修复的隐患（vite base、router 模式、cleartext、assets 同步）。

- [ ] 10.1 **CI 全面切到 pnpm**
  - `.github/workflows/android.yml` 加 `pnpm/action-setup@v4`（version: 9）
  - 替换 `cache: 'npm'` → `cache: 'pnpm'`
  - 替换 `cache-dependency-path: package-lock.json` → `pnpm-lock.yaml`
  - 替换 `npm install` → `pnpm install --frozen-lockfile`
  - 替换 `npm run build` → `pnpm run build`
  - 替换 `npx vitest` → `pnpm exec vitest`
  - 替换 `npm-main-*` cache key → `pnpm-main-*`（含 monorepo 三个 node_modules 路径）
  - `.github/workflows/test.yml` 同样改造（layer1 + layer2 各加 pnpm setup）
- [ ] 10.2 **删 `package-lock.json`**（pnpm-lock.yaml 是唯一真源）
- [ ] 10.3 **新建 `scripts/build-plugin-openlist-web.sh`**
  - pnpm install → 校验 @encvgo/components 链接 → pnpm exec vite build
  - 校验 dist/index.html 无绝对路径（grep 拒绝 `/`）
  - 同步 dist/ → `plugin-openlist/src/main/assets/openlist/`
  - 输出大小 + 下一步 gradle 命令
- [ ] 10.4 **`vite.config.ts` 加 `base: './'`**（file:// 加载必需）
  - 默认 `/` 在 `file:///android_asset/openlist/` 下 404
  - 注释强调原因
- [ ] 10.5 **`router/index.ts` 改 hash 模式**
  - `createWebHistory()` → `createWebHashHistory()`
  - file:// 协议不支持 history.pushState
  - 即使支持，非根路径刷新会 404（无服务端路由）
  - hash 模式天然兼容 file:// + 刷新友好
- [ ] 10.6 **`AndroidManifest.xml` 加 `usesCleartextTraffic="true"`**
  - OpenList 是 127.0.0.1:5244 明文 HTTP
  - Android 9+ 默认禁止明文 HTTP，必须显式开
  - 同时恢复 service / provider / meta-data（之前误删）
- [ ] 10.7 **CI workflow 加 plugin web 构建步骤**
  - 在主 app `pnpm run build` 之后
  - 执行 `bash scripts/build-plugin-openlist-web.sh --prod`
  - 必须在 Gradle build 之前（assets 资源必须就位）
- [ ] 10.8 **模拟 CI 流程**（本地验证）
  - pnpm install --frozen-lockfile ✓
  - vue-tsc --noEmit 主 app ✓
  - vue-tsc --noEmit plugin web ✓
  - bash scripts/build-plugin-openlist-web.sh ✓ → assets/openlist/index.html 写入
- [ ] 10.9 **真机测试 checklist 准备**
  - [ ] Android 9+ 设备（明文 HTTP 兼容测试）
  - [ ] `adb install app-debug.apk && adb install plugin-openlist-debug.apk`
  - [ ] 主 app 启动 → 设置 → 扩展管理 → 看到 OpenList 插件
  - [ ] 启用 OpenList 扩展 → 嵌入式 WebView 渲染（file:///android_asset/openlist/）
  - [ ] WebView 内点「启动并加载」→ OpenList 服务启动（127.0.0.1:5244）
  - [ ] 主 app 的 OpenList 管理面板（/openlist）能通过 ContentProvider 读状态
  - [ ] 沙箱 dev preview: http://localhost:5174/webview 能看 Capacitor UI + iframe 嵌入 OpenList SPA

## Phase 11: 修复 vite build --prod 报错（Vite 8 不支持 --prod）

> **核心痛点**：CI 报 `CACError: Unknown option --prod`，因为 Vite 8 移除了 Webpack 风格的 `--prod` 参数。
> - Vite 8 默认 `vite build` = 生产（NODE_ENV=production）
> - 开发模式用 `vite build --mode development`
> - `--prod` 是 Webpack 遗留，Vite 直接 reject

- [ ] 11.1 **重写 `scripts/build-plugin-openlist-web.sh`**：
  - 默认生产（不传 vite 额外参数）
  - `--dev` 切开发（传 `--mode development`）
  - `--prod` 兼容旧调用（接受但不传给 vite）
  - 错误参数直接 exit 1 + 提示用法
- [ ] 11.2 **CI workflow 移除 `--prod`**：
  - `android.yml` 改为 `bash scripts/build-plugin-openlist-web.sh`
- [ ] 11.3 验证默认生产构建 EXIT=0
- [ ] 11.4 验证 `--dev` 开发构建 EXIT=0
- [ ] 11.5 验证 dev 产物 vs prod 产物大小差异（dev 更大含 sourcemap）

## Phase 12: Node.js 升级到 24 LTS（Capacitor 8 要求 ≥22）

> **核心痛点**：CI 报 `[fatal] The Capacitor CLI requires NodeJS >=22.0.0`
> 根因：Phase 10 改 CI 时把所有 `node-version: '20'` 写死了（当时是为了 pnpm 9 兼容而保守选择 20）
> 实际 Capacitor 8.3.4 要求 Node ≥ 22，且当前最新 LTS 是 24（v24.x 已于 2025-05 正式 LTS）。
>
> 升级 Node 24 的好处：
> - Capacitor 8 CLI 直接兼容（≥ 22 即可）
> - 更好的 V8 引擎性能（vue-tsc 编译更快）
> - pnpm 10 完整支持
> - Node 22/24 内置 fetch / test runner / 等现代 API

- [ ] 12.1 `.github/workflows/android.yml`：`node-version: '20'` → `'24'`
- [ ] 12.2 `.github/workflows/test.yml` layer1 + layer2：两处都 `20` → `24`
- [ ] 12.3 验证 sandbox 已装 Node 24.15.0（`node --version`）
- [ ] 12.4 验证 `npx cap --version` 在 Node 24 下正常（输出 8.x）
- [ ] 12.5 模拟 CI 全流程：pnpm install + vue-tsc × 2 + plugin web build + npx cap --version 全 EXIT=0
- [ ] 12.6 验证仓库无其他 Node 版本约束（.nvmrc / Dockerfile / docs）

## Task Dependencies

- Phase 0 → ... → Phase 11 → Phase 12

## Phase 13: OpenList 插件 APK 架构性构建失败修复

> **核心原则：mpv 与 openlist 功能形态不同，不应锁镜。正确做法是直接读 combolite-core 2.0.2 源码确认架构契约，再针对 OpenList 实际需要裁剪 deps。**
>
> **直接读源码发现的真实架构契约**（`/tmp/combolite-src` 从 Maven Central 拉取的 `combolite-core-2.0.2-sources.jar`）：
>
> | 接口 | 强契约点 | openlist 用法 |
> |------|---------|-------------|
> | `IPluginEntryClass.pluginModule: List<Module>` | 必须实现，类型来自 `org.koin.core.module.Module` | `emptyList()` |
> | `IPluginEntryClass.onLoad(context: PluginContext)` | 必须 | 初始化 Bridge + Config |
> | `IPluginEntryClass.onUnload()` | 必须 | shutdown Service + Bridge |
> | `IPluginEntryClass.Content()` | **必须 + 必须是 `@Composable`** | Composable 包装 AndroidView 宿主 WebView |
> | `IPluginActivity` / `IPluginService` / `IPluginReceiver` | 可选 meta-data | **openlist 不用**（Service/Provider 都是普通 Android 组件，自己管生命周期） |
>
> **ClassLoader 拓扑**（combolite-core/PluginLifecycleManager.kt:224）：
> ```
> pluginClassLoader.parent = host's classLoader
> ```
> → host 已 implementation 的所有依赖（combolite-core / core-ktx / compose-ui / koin-core runtime），插件**用 compileOnly 就够**（运行时由 parent classloader 解析）。
> → host **没有**的依赖（localbroadcastmanager / openlist-classes.jar / koin-core 类型），插件**必须 implementation** 打到 APK 里。
>
> **MPV vs OpenList 真实差异**（不要锁镜）：
>
> | 维度 | MPV | OpenList |
> |------|-----|----------|
> | Content 内部 | Composable + Material3 widget + icons | Composable + `AndroidView`(WebView) |
> | 多余组件 | 无 | Service + ContentProvider + LocalBroadcastManager |
> | Material3 依赖 | 必须 | **不要** |
> | icons-extended | 必须 | **不要** |
> | activity-compose | 必须 | **不要** |
> | appcompat | 必须 | **不要** |
> | localbroadcastmanager | 不要 | **必须**（host 没有） |
> | openlist-classes.jar | 不要 | **必须**（gomobile 产物） |

- [ ] 13.1 `plugin-openlist/build.gradle.kts` 重写：承接契约（compose plugin + buildFeatures.compose）+ 最小 deps（composet-bom + compose-ui + localbroadcastmanager + openlist-classes.jar + combolite-core compileOnly + koin-core compileOnly + core-ktx compileOnly）；**不**引 material3 / icons / activity-compose / appcompat
- [ ] 13.2 `OpenListPluginEntry.kt` import 修复：`com.encvgo.combolite.IPlugin*` → `com.combo.core.api.IPlugin*` / `com.combo.core.model.PluginContext`
- [ ] 13.3 `OpenListPluginJSInterface.kt:99`：`setAdminPwd` → `setAdminPassword`（编译期因 Bridge 是 `object` 会立刻报 NoSuchMethodError 之外的 unresolved reference）
- [ ] 13.4 全局 Grep 验证 `setAdminPwd` / `com.encvgo.combolite.IPlugin*` / `com.encvgo.combolite.PluginContext` 残留 0 处
- [ ] 13.5 `OpenListPluginEntry` 与 `IPluginEntryClass` 4 契约点对齐：pluginModule=onLoad=onUnload=@Composable Content()=已实现
- [ ] 13.6 `AndroidManifest.xml` 验证：未注册 IPluginActivity/IPluginService/IPluginReceiver meta-data（用普通 Service/Provider 即可）
- [ ] 13.7 新增 `.trae/rules/verification-discipline.md`（防幻觉 + 本地工具优先 + CI 诊断纪律）
- [ ] 13.8 `.trae/rules/combolite.md` 记录 IPluginEntryClass 实际接口契约（pluginModule/onLoad/onUnload/Content + 必须 @Composable + Koin Module 类型）+ ClassLoader 拓扑（plugin.parent = host）
- [ ] 13.9 清理 `job_logs.zip` + `/tmp/job_logs_inspect/`（用户明确要求）
- [ ] 13.10 推送分支到 `trae/solo-agent-WAmQzy`，等 CI 跑通，下载 artifact 验证 `plugin-openlist-release.apk` 包含 `libgojni.so` + `Openlistlib*` + `assets/openlist/index.html`

## Phase 14: 沙箱 kotlinc 验证发现的语法/抽象/Composable 错误修复

> **核心痛点**：CI 报 21 个 `unresolved reference` + 4 个 `syntax/abstract/composable` error。
> 根因不是缺依赖，而是 `OpenListBridge.kt:128` 写了 `dist/* to` 触发 Kotlin 嵌套块注释
> （`/* /* */ */` 语义），吞掉后续 330 行 → 所有 21 个 unresolved 都是级联。
>
> Phase 14 的关键认知：**沙箱无 android.jar / compose.jar / combolite-core.jar，**
> **unresolved reference 数字大但都是预期的**——沙箱只能抓 parse/syntax 错
> （这正是 CI log 的核心 ~60% 错误）。

- [ ] 14.1 修 `OpenListBridge.kt:128` 的 `dist/* to` 嵌套块注释触发器 → 改为 `dist contents`
- [ ] 14.2 删 `OpenListConfig.kt` 的幻觉方法调用 `bridge.setPort()` / `bridge.setDataDir()`
- [ ] 14.3 给 `OpenListEmbedWebView.kt` 的 Composable 加 `@Composable` 注解
- [ ] 14.4 修 `OpenListPluginEntry.kt` 的 `context.applicationContext` → `context.application`（PluginContext 是 data class）
- [ ] 14.5 补 `plugin-openlist/build.gradle.kts` 的 `compose.foundation` + `compose.foundation.layout` 依赖
- [ ] 14.6 沙箱验证：grep "Syntax error\|abstract member\|Composable invocations" 应为 0
- [ ] 14.7 沙箱 unresolved reference 数字大但都是预期第三方类型（android.* / com.combo.* / openlistlib.* / compose.*）
- [ ] 14.8 重写 `OpenListBridge.applyToBridge` 仅做日志（gomobile 暴露的是 `Openlistlib.setConfigData()` 静态方法）

## Phase 15: 沙箱 kotlinc 自动化准备（工程化）

> **核心痛点**：每次新沙箱会话 /tmp 被清空，kotlin 编译器需要重装。
> 手动 curl 4 个 jar + 写 wrapper 易错，且后续 AI 不知道有现成 wrapper，必须重做。
>
> **解决方案**：仿照 `app/encv-mobile/scripts/start-preview.sh` 的铁律风格
> 写一个一键脚本 `/workspace/.trae/scripts/setup-kotlinc.sh`。

- [ ] 15.1 **创建** `/workspace/.trae/scripts/setup-kotlinc.sh`
  - `set -euo pipefail` 严格模式
  - 6 步骤：前置检查 → 创建 KOTLIN_HOME → 拉 4 jar → 写 wrapper → 验证 → 状态报告
  - 自动 `grep` `libs.versions.toml` 的 `kotlin = "X.Y.Z"`（不硬编码版本）
  - monorepo 多种布局兼容（app/encv-mobile/android/gradle/、android/gradle/、gradle/）
  - 100% 走 Maven Central，**绝不**走 GitHub Releases / Google CDN / JetBrains CDN
  - 4 个 jar：`kotlin-compiler-embeddable` + `kotlin-stdlib` + `kotlin-reflect` + `kotlinx-coroutines-core-jvm:1.10.2`
  - 写 `/usr/local/bin/kotlinc-<version>` 包装脚本（exec java -cp ... K2JVMCompiler）
  - 跑 `kotlinc-<version> -version` 验证
  - 退出码：0=OK / 1=前置缺 / 2=网络失败 / 3=版本校验失败
  - 幂等：jar 已有且 size 合理（compiler ≥50MB，其余 ≥100KB）就 skip
- [ ] 15.2 **运行验证**：`bash /workspace/.trae/scripts/setup-kotlinc.sh` → 6 步骤全过 + EXIT=0
- [ ] 15.3 **再次运行验证幂等**：直接返回 `[skip] 已有` + EXIT=0
- [ ] 15.4 **更新 `.trae/rules/trae_web_sandbox_network.md`**
  - §七：CDN 阻断清单（GitHub Releases / dl.google.com / download.jetbrains.com 全部 ❌）
  - §八：kotlinc 一键拉取方案（指向 setup 脚本 + 脚本设计要点 + 调试流程）
  - §九：跨文档引用表
- [ ] 15.5 **更新 `.trae/rules/verification-discipline.md` §7**
  - 顶部加 "Kotlin 编译器准备已工程化" 提示 → 指向 setup 脚本
  - §7.4 加 "推荐：一键脚本" + 列出 3 个 `禁止 curl` 的 CDN
  - "手动做法" 标注 "不推荐，仅当脚本失败时备查"
- [ ] 15.6 **更新 spec 本文件**（tasks.md Phase 15）记录新工作流

## Phase 16: Gradle Kotlin DSL 脚本编译错误修复（污染 mpv 构建）

> **症状（CI 实际）**：
> - step 31 `:plugin-mpv-player:convert_plugin-mpv-player_release` → BUILD FAILED in 14s
> - step 32 `Package MPV plugin as APK` → BUILD FAILED in 1s
> - step 36 `OpenList plugin APK (aar2apk)` → BUILD FAILED in 1s
> - step 38 `Release APK build (composite)` → 看似 OK 但实际没生成 openlist APK
> - **终态**：`openlist-release.apk` 没产物，主 app APK 只含 mpv
>
> **根因（build log line 14102-14104）**：
> ```
> e: file:///.../plugin-openlist/build.gradle.kts:81:52
>     Unresolved reference 'foundation'.
> e: file:///.../plugin-openlist/build.gradle.kts:82:52
>     Unresolved reference 'foundation'.
> ```
> `plugin-openlist/build.gradle.kts:81-82` 在 Phase 14 修复时加了
> `implementation(libs.compose.foundation)` + `implementation(libs.compose.foundation.layout)`，
> 但**忘了在 `libs.versions.toml` 声明对应 library 别名**。
> Gradle Kotlin DSL 脚本编译期就崩了（不是 source 编译期，是 build script 本身），
> 因为 plugin-openlist 是同一个 multi-project 的子项目，
> 任何子项目配置期失败都会让 `:plugin-mpv-player:convert_*` 也跟着死。
>
> **为什么污染 mpv**：
> `convert_plugin-mpv-player_release` 任务需要解析整个 dependency graph，
> Gradle 必然先 evaluate 所有子项目的 `build.gradle.kts`。
> 任意一个子项目脚本编译失败 → 整个 configuration phase 退出码非 0。
> 修复 openlist 后,mpv 自动恢复。

- [ ] 16.1 修 `libs.versions.toml` 缺 2 个 alias：
  - `compose-foundation = { group = "androidx.compose.foundation", name = "foundation" }`
  - `compose-foundation-layout = { group = "androidx.compose.foundation", name = "foundation-layout" }`
  - 不带 `version.ref`（由 compose-bom 2024.06.00 管版本）
- [ ] 16.2 沙箱验证：写脚本遍历所有 `build.gradle.kts` 提取 `libs.X.Y` 引用，
       对照 toml alias（toml `-` → 访问器 `.`），**总失败数 0**
- [ ] 16.3 沙箱验证：toml 23 个 lib alias + 5 个 plugin alias **全部唯一**（python tomllib 解析）
- [ ] 16.4 沙箱验证：`/usr/local/bin/kotlinc-2.3.21` 跑 plugin-openlist 所有 .kt 文件 0 syntax / 0 abstract / 0 @Composable 错
- [ ] 16.5 推送分支到 `trae/solo-agent-WAmQzy`，CI 跑通，下载 artifact 验证 openlist + mpv 双 APK 都生成

## Phase 17: 8 个 unresolved reference（gomobile 命名 + Map property）

> **症状（CI 实际）**：
> ```
> e: OpenListBridge.kt:306:29 Unresolved reference 'forceDbSync'.
> e: OpenListPluginJSInterface.kt:66:41 Unresolved reference 'running'.
> e: OpenListPluginJSInterface.kt:67:42 Unresolved reference 'running'.
> e: OpenListPluginJSInterface.kt:67:60 Unresolved reference 'port'.
> e: OpenListPluginJSInterface.kt:68:37 Unresolved reference 'pid'.
> e: OpenListPluginJSInterface.kt:69:47 Unresolved reference 'dataSizeBytes'.
> e: OpenListPluginJSInterface.kt:70:43 Unresolved reference 'lastError'.
> e: OpenListPluginJSInterface.kt:71:46 Unresolved reference 'lastUpdateTs'.
> ```
>
> **不要再猜——按用户指示「利用 CI 没有沙箱限制的优势诊断」**：
> 1. clone 真 fork `https://github.com/Hi-Sillot/OpenList@dev`（沙箱能 clone）
> 2. 读 fork `openlistlib/server.go` 找到实际 Go 函数名 `ForceDBSync`
> 3. 读 gomobile 源码 `cmd/gobind/gen.go:527 lowerFirst` 找命名规则
> 4. 用 Go 在沙箱里跑 `lowerFirst("ForceDBSync")` 实证

- [ ] 17.1 **Bug 1**：`OpenListBridge.kt:306` 调用 `Openlistlib.forceDbSync()` → `Openlistlib.forceDBSync()`
  - 根因：gomobile `lowerFirst("ForceDBSync")` = `"forceDBSync"`（保留 DBSync 子词大写，只动首字符）
  - 头注释 line 25, 36 也一并改 `forceDbSync` → `forceDBSync`
  - **沙箱实证**：`go run /tmp/lowerFirst.go` 输出 `forceDBSync`
- [ ] 17.2 **Bug 2**：`OpenListPluginJSInterface.kt:66-71` 用 `snapshot.running` 等 property 访问
  - 根因：`OpenListBridge.snapshot()` 返回 `Map<String, Any?>`，Kotlin Map 没有 `.running` 这种 property
  - 修复：仿 [OpenListStatusProvider.kt:93-98](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListStatusProvider.kt) 改 `snapshot["running"] as? Boolean`
  - 注意 key 用 snake_case（`data_size_bytes` / `last_error` / `last_update_ts`），JSON 输出再用 camelCase
- [ ] 17.3 沙箱验证：`/usr/local/bin/kotlinc-2.3.21` 跑 fix 后
  - 0 syntax / 0 abstract / 0 @Composable 错
  - 0 unresolved `forceDbSync`（错名）—— 原 1 个
  - 0 unresolved `running` / `port` / `pid` / `dataSizeBytes` / `lastError` / `lastUpdateTs` —— 原 7 个
  - 剩 458 个 unresolved 全是预期第三方（android.* / com.combo.* / openlistlib.* / compose.* / koin.*）
- [ ] 17.4 全仓库 grep `Openlistlib.<method>` 7 处调用，验证命名都正确（`setConfigData/init/start/shutdown/isRunning/setAdminPassword/forceDBSync`）
- [ ] 17.5 **Phase 18 风险登记**（不修，仅登记）：
  - [build-openlist-aar.sh:381](file:///workspace/scripts/build-openlist-aar.sh) A2 fallback 仅在 fork 缺 `openlistlib/event.go` 时注入
  - Hi-Sillot/OpenList@`404daf0` **已自带** event.go（`OnProcessExit(code int)`）→ A2 跳过 → gomobile 生成 `onProcessExit(int)`
  - 但 [OpenListBridge.kt:420](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt) 实现 `override fun onProcessExit(code: Long)` —— **下一轮 AAR 重建时会爆**
  - 解决：把 `code: Long` 改 `code: Int`，或固定 fork 某个不带 event.go 的老 commit
- [ ] 17.6 推送分支到 `trae/solo-agent-WAmQzy`，CI 跑通

## Phase 18: android.yml 加固（5 个守卫 + 1 个缓存）

> **背景**：Phase 16-17 暴露 CI 反馈链太长：
> - Phase 16 错（`libs.compose.foundation` 缺 toml alias）→ 3 min Gradle 配置期才报
> - Phase 17 错（8 个 unresolved）→ ~3 min `:plugin-openlist:compileReleaseKotlin` 才报
> - AAR 重建（3-5 min）每次都重跑，git diff 一行也能触发全量 gomobile bind
>
> **用户反馈**：「你没改 android.yml 啊，没有主动诊断效率太低了」—— 必须把
> `verification-discipline.md` §7.7-7.8 的规则**落到 CI**才能复利。
>
> **5 个守卫 + 1 个缓存**（按 ROI 排序）：

- [ ] 18.1 **Guard A — TOML alias guard**（§7.7，最早期抓手，0 网络，< 1s）
  - 在 "R8/ProGuard Guard" **之前** 跑（早于所有 Gradle 调用）
  - bash 遍历所有 `build.gradle.kts` 提取 `libs.X.Y` 引用，对照 toml alias
  - 0 失败则继续，1+ 失败则 `::error::` + exit 1
  - **沙箱验证**：删 `compose-foundation` 2 行 → guard 抓出 2 错（plugin-openlist/build.gradle.kts:81-82）
- [ ] 18.2 **Guard B — kotlinc pre-flight**（§7.8，~10s，抓真 unresolved）
  - 在 "Build OpenList plugin APK" 步骤**最开头**（AAR 已抽好）
  - 调 `bash .trae/scripts/setup-kotlinc.sh` 拉 kotlinc 工具
  - kotlinc 跑 plugin 全部 .kt 配 classpath = `openlist-classes.jar + android.jar + combolite-core.jar`
  - 只对**已知 gomobile + Map 字段名**做 unresolved 过滤（`grep -E "Unresolved reference '(forceDbSync|forceDBSync|...|port|pid|...)"`）
  - 找不到 classes.jar → `::warning::` 跳过（fail-safe，不阻塞 CI）
- [ ] 18.3 **Guard C — Map<String, Any?> property guard**（Phase 17 类抓错，< 1s）
  - 同样在 "Build OpenList plugin APK" 步骤最开头，Guard B 之前
  - grep `snapshot\.(running|port|pid|dataSizeBytes|lastError|lastUpdateTs)` → 0 匹配则通过
  - **沙箱验证**：改回 `snapshot.running` → guard 抓出 3 错（line 65, 71, 72）
- [ ] 18.4 **Cache A — openlist.aar + openlist-classes.jar 缓存**（节省 3-5 min）
  - 在 "Build OpenList AAR" **之前** 用 `actions/cache@v4`
  - key = `openlist-aar-{hash(build-openlist-aar.sh, plugin-openlist/build.gradle.kts)}-v2`
  - restore-keys = `openlist-aar-` 兜底
  - 命中 → 跳过 `Build OpenList AAR` 整个 step（`if: steps.cache-openlist-aar.outputs.cache-hit != 'true'`）
- [ ] 18.5 **Guard D — gofmt**（Go 改必跑，< 1s）
  - 在 "Run Go unit tests" 步骤**最开头**（go test 之前先 lint）
  - `gofmt -l ./internal ./cmd` → 有文件列出则 `::error::` + exit 1
- [ ] 18.6 **元教训**：写好的 guard 自己也要 smoke test 验证
  - Guard A 第一版有盲点：`find android ...` 漏掉了 `plugin-openlist/`（不在 android/ 下）
  - 只在 22 个 refs 上跑（漏 14 个），破坏 toml 后没抓到错
  - 修：改 `find . -name "build.gradle.kts"` → 36 refs，破坏后精准抓 2 错
  - 教训：**任何新增的"自动化守卫"都必须经过 "故意坏掉" 的反向测试**，否则可能比没 guard 还糟
  - 见 [verification-discipline.md §7.9](file:///workspace/.trae/rules/verification-discipline.md)「守卫也要被 test 验证」
- [ ] 18.7 推送分支到 `trae/solo-agent-WAmQzy`，CI 跑通，5 个守卫都 PASS
