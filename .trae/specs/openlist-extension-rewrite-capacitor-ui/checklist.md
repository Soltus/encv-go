# Checklist

## Phase 0: ComboLite 合规修复（与 UI 无关）

- [ ] IPluginEntryClass 接口契约已确认
- [ ] MpvPluginEntry vs OpenListPluginEntry 差距已分析
- [ ] OpenListPluginEntry.pluginModule = emptyList()
- [ ] OpenListPluginEntry.onLoad 初始化 OpenListBridge
- [ ] OpenListPluginEntry.onUnload shutdown Bridge + Service
- [ ] Content() 用 OpenListEmbedWebView 替代 Compose UI
- [ ] StatusCard / ControlCard / ConfigCard / InfoGrid / formatFileSize 已删除
- [ ] build.gradle.kts 移除 compose plugin / buildFeatures / dependencies
- [ ] plugin-openlist 编译通过
- [ ] combolite-host 编译通过

## Phase 1: 嵌入式 WebView + JS-Native 桥接

- [ ] OpenListEmbedWebView.kt 已创建（@Composable + AndroidView）
- [ ] OpenListPluginJSInterface.kt 已创建（@JavascriptInterface 暴露 start/stop/getStatus/setPassword/readConfig/writeConfig/getVersion）
- [ ] OpenListWebViewClient.kt 已创建
- [ ] plugin-openlist 编译通过

## Phase 2: Monorepo 改造（pnpm workspace）

- [ ] pnpm-workspace.yaml 已创建（app, plugin-openlist/web, packages/*）
- [ ] 项目根 package.json 添加 packageManager
- [ ] packages/components/package.json 已创建（@encvgo/components）
- [ ] packages/components/src/index.ts 已创建
- [ ] packages/components/src/OpenListStatusCard.vue 已移植
- [ ] packages/components/src/OpenListLogList.vue 已移植
- [ ] plugin-openlist/web/package.json 已创建（@encvgo/components: workspace:*）
- [ ] plugin-openlist/web/vite.config.ts / tsconfig.json / index.html 已创建
- [ ] pnpm install 在 packages/components 和 plugin-openlist/web 成功

## Phase 3: 插件 web 项目页面

- [ ] plugin-openlist/web/src/main.ts / App.vue / router/index.ts 已创建
- [ ] OpenListHome.vue 已创建（K-Sillot OpenListScreen 复刻：AppBar + FAB + 状态卡 + 日志流）
- [ ] OpenListConfigEditor.vue 已创建（K-Sillot ConfigEditorPage 复刻：JSON 编辑 + 校验 + 备份）
- [ ] OpenListSettings.vue 已创建（简化版：版本/数据目录）
- [ ] OpenListWebView.vue 已创建（iframe 加载 OpenList SPA，可选）
- [ ] openlist-native.ts 已创建（Window.OpenListNative 类型 + 包装对象）
- [ ] pnpm run build 产出 dist/
- [ ] vue-tsc --noEmit 通过

## Phase 4: 主 app 集成

- [ ] app/src/router/index.ts 添加 /openlist 路由
- [ ] 主 app pnpm install + pnpm run build 通过
- [ ] 主 app vue-tsc --noEmit 通过

## Phase 5: 编译与部署验证

- [ ] plugin-openlist 编译通过
- [ ] combolite-host 编译通过
- [ ] app 编译通过
- [ ] 插件 web dist 产出
- [ ] 主 app dist 产出
- [ ] 沙箱预览启动正常
- [ ] 主 app /openlist 路由可访问
- [ ] 嵌入式 WebView 通过 OpenListNative JSInterface 调 OpenListBridge 成功
- [ ] 共享组件 OpenListStatusCard / OpenListLogList 在主 app 和插件都可用

## Phase 6: plugin-openlist/web 前端开发预览（沙箱浏览器版）

- [ ] scripts/dev-openlist-web.sh 已创建（端口 5174 → 5175 fallback、信号陷阱、状态报告）
- [ ] bash scripts/dev-openlist-web.sh 启动成功
- [ ] curl http://localhost:5174/ 返回 200 + HTML
- [ ] /home 路由渲染 OpenListHome（AppBar + 4 工具按钮 + StatusCard + LogList + FAB）
- [ ] /config 路由渲染 OpenListConfigEditor（JSON 编辑器）
- [ ] /settings 路由渲染 OpenListSettings（版本/数据目录）
- [ ] /webview 路由渲染 OpenListWebView（提示需 Android WebView 容器）
- [ ] OpenListStatusCard 在浏览器显示默认态（已停止）
- [ ] FAB 点击走 window.OpenListNative fallback 不报错
- [ ] HMR 验证：修改 OpenListStatusCard.vue 后浏览器自动刷新
- [ ] 与 dev-openlist.sh 的差异已在脚本注释中明确说明

## Phase 7: /webview 嵌入 OpenList 原生 SPA

- [ ] vite.config.ts 添加 /openlist-spa → http://127.0.0.1:5244 代理
- [ ] OpenListWebView.vue 始终渲染 iframe（dev 走代理，prod 直连 5244）
- [ ] 沙箱 dev 模式 onMounted 主动探测后端可达性（2s HEAD 超时）
- [ ] 后端未启动时显示降级 UI（带 bash scripts/dev-openlist.sh 命令提示 + 重试按钮）
- [ ] 真机模式不探测（始终显示 iframe）
- [ ] 顶部 toolbar 外部打开按钮仅真机模式显示
- [ ] 重启 dev server 后 /openlist-spa 代理返回 502（代理工作但后端未启）
- [ ] 浏览器访问 /webview 看到降级 UI
- [ ] 用户运行 dev-openlist.sh 后 iframe 自动加载 OpenList SPA
- [ ] vue-tsc --noEmit 通过

## Phase 8: iframe 防御性状态 UI

- [ ] IframeState 类型定义（5 态：probing/loading/connected/error/timeout）
- [ ] 探测中状态显示 spinner + 目标地址
- [ ] 加载中状态 iframe 半透明 0.6
- [ ] 已连接状态绿色对勾 + 隐藏覆盖层
- [ ] 错误状态红色 cloudOffline + lastError 详细信息
- [ ] 超时状态黄色 timer + 「再试一次」按钮
- [ ] 顶部状态条（颜色随状态变）
- [ ] 顶部 toolbar 状态按钮（点击重试，probing 时禁用）
- [ ] 「复制启动命令」按钮（navigator.clipboard + toast 反馈）
- [ ] 探测超时 5s（cors 模式，区分 timeout vs error）
- [ ] iframe sandbox 加固
- [ ] 重试计数显示（retryCount > 0 时）
- [ ] vue-tsc --noEmit 通过
- [ ] 浏览器显示「错误」状态卡（带重试 + 复制命令）

## Phase 9: 修复 iframe @load 误判 connected

- [ ] vite.config.ts 添加 /__openlist-health 中间件（Node 直连 5244 + CORS 头）
- [ ] checkHealth() 走 /__openlist-health（不再 mode: 'cors' 探测 proxy）
- [ ] probeBackend 用 Promise.race 实现 5s 前端兜底超时
- [ ] onIframeLoad 不再直接置 connected（改 verifyAfterIframeLoad 后置校验）
- [ ] SPA 内导航/刷新时 onIframeLoad 直接 return（state 已是 connected）
- [ ] verifyAfterIframeLoad 检测到 502 页面自动 transition to 'error'
- [ ] pollHealth() 10s 周期校验，后端突然挂掉自动跳回 error
- [ ] onUnmounted 清理 pollHealth timer
- [ ] 右下角 bug FAB 触发 devtools 风格调试面板
- [ ] 调试面板记录 info/warn/error/probe 四级日志
- [ ] 重启 dev server 后 /__openlist-health 返回 { alive: false, code: ECONNREFUSED }
- [ ] 浏览器实测：iframe @load 后保持 error 态（不再误判 connected）
- [ ] vue-tsc --noEmit 通过

## Phase 10: CI 适配 + 真机测试准备

- [ ] .github/workflows/android.yml 加 pnpm/action-setup@v4
- [ ] android.yml cache: 'pnpm' + cache-dependency-path: pnpm-lock.yaml
- [ ] android.yml 替换 npm install / npm run build / npx → pnpm
- [ ] android.yml pnpm cache key 包含 monorepo 三个 node_modules 路径
- [ ] .github/workflows/test.yml layer1 + layer2 同样切 pnpm
- [ ] 删除 package-lock.json
- [ ] 新建 scripts/build-plugin-openlist-web.sh（构建+校验+同步）
- [ ] vite.config.ts 加 base: './'（file:// 必需）
- [ ] router/index.ts 改 createWebHashHistory（file:// 必需）
- [ ] AndroidManifest.xml 加 usesCleartextTraffic="true"（明文 HTTP）
- [ ] AndroidManifest.xml 恢复 service / provider / meta-data（关键 PluginEntry 注册）
- [ ] CI android.yml 集成 build-plugin-openlist-web.sh --prod
- [ ] 模拟 CI：pnpm install + 双端 vue-tsc + plugin web build 全 EXIT=0
- [ ] plugin-openlist/src/main/assets/openlist/index.html 写入且 base 路径相对化
- [ ] 真机测试 checklist 9 项就绪

## Phase 11: 修复 vite build --prod 报错

- [ ] scripts/build-plugin-openlist-web.sh 默认生产（不传 vite 参数）
- [ ] scripts/build-plugin-openlist-web.sh --dev 切开发（传 --mode development）
- [ ] scripts/build-plugin-openlist-web.sh --prod 兼容旧调用（接受但不传给 vite）
- [ ] android.yml 移除 --prod
- [ ] 默认生产构建 EXIT=0
- [ ] --dev 开发构建 EXIT=0
- [ ] dev vs prod 产物大小差异验证

## Phase 12: Node.js 升级到 24 LTS

- [ ] .github/workflows/android.yml node-version: '24'
- [ ] .github/workflows/test.yml layer1 + layer2 node-version: '24'
- [ ] sandbox Node 版本 ≥ 24
- [ ] npx cap --version 在 Node 24 下正常输出 8.x
- [ ] 模拟 CI 全流程 5 步全 EXIT=0
- [ ] 仓库无其他 Node 版本约束残留（.nvmrc / Dockerfile / docs）

## Phase 13: OpenList 插件 APK 架构性构建失败修复

- [ ] 13.A `plugin-openlist/build.gradle.kts` 包含 `id("org.jetbrains.kotlin.plugin.compose")` + `buildFeatures { compose = true }` + 与 MPV 同款依赖集合
- [ ] 13.B `OpenListPluginEntry.kt` import 全为 `com.combo.core.api.*` / `com.combo.core.model.*`（无 `com.encvgo.combolite.*` 残留）
- [ ] 13.C `OpenListPluginJSInterface.kt` 调 `OpenListBridge.setAdminPassword`（无 `setAdminPwd` 残留）
- [ ] 13.D 仓库全局 Grep 验证 `setAdminPwd` / `com.encvgo.combolite.IPlugin*` / `com.encvgo.combolite.PluginContext` 残留 = 0
- [ ] 13.E `.trae/rules/verification-discipline.md` 已创建并被引用
- [ ] 13.F `job_logs.zip` 已删除，`/tmp/job_logs_inspect/` 已清空
- [ ] 13.G CI 重新触发后，下载 artifact 验证 `plugin-openlist-release.apk` 包含 `libgojni.so` + `Openlistlib*` + `assets/openlist/index.html`
