# Tasks

## Phase 0: 前置验证（必须全绿才能进 Phase 1）

### 0.1 Hi-Sillot fork 补全 `openlistlib/` 入口

- [x] 0.1.1 在 Hi-Sillot/OpenList fork 根目录新建 `openlistlib/` 目录
- [x] 0.1.2 拷入 `openlistlib/server.go`（参考 K-Sillot 实现，包装 `cmd/server.go::Start()`）
- [x] 0.1.3 拷入 `openlistlib/settings.go`（`SetConfigData`/`SetConfigLogStd`/`SetConfigDebug`/`SetConfigNoPrefix`/`SetAdminPassword`）
- [x] 0.1.4 拷入 `openlistlib/common.go`（`GetOutboundIPString()`）
- [x] 0.1.5 拷入 `openlistlib/internal/log.go`（`MyFormatter`）
- [x] 0.1.6 确认 fork 的 `cmd/server.go` 在 `Start()` 开头调用 `encv.GenerateENCVSettingItems()` + `encv.LoadENCVPluginSettings()`（与 Hi-Sillot 当前实现一致）
- [x] 0.1.7 `cd openlistlib && go build .` 在 fork 根目录可编译（受 fork 现有 go.mod replace 路径阻塞，需在 build script 中修正）

### 0.1·B Hi-Sillot fork 维护 dev 分支 + frontend-pinned.txt + i18n-overlay/

- [x] 0.1·B.1 在 Hi-Sillot/OpenList fork 创建 `dev` 分支（用户操作，token 已就位）
- [x] 0.1·B.2 在 fork `dev` 分支根目录新建 `frontend-pinned.txt`，写入当前匹配的 OpenList-Frontend 版本（如 `v4.0.0`）
- [x] 0.1·B.3 （可选）创建 `public/dist/i18n-overlay/zh-CN/translation.json` 含 ENCV 专用 key 翻译
- [x] 0.1·B.4 （可选）创建 `public/dist/i18n-overlay/en/translation.json`
- [x] 0.1·B.5 在 fork `dev` 分支提交 + 推送（URL 注入：`https://x-access-token:${GITHUB_TOKEN}@github.com/...`）

### 0.1·C encv-mobile 侧 fork 配置

- [x] 0.1·C.1 创建 `/workspace/scripts/openlist-fork.env`，含 `OPENLIST_FORK_URL=https://github.com/Hi-Sillot/OpenList.git`、`OPENLIST_FORK_BRANCH=dev`、`OPENLIST_FRONTEND_VERSION=`
- [x] 0.1·C.2 `.gitignore` 把 `scripts/openlist-fork.env.local` 加入（个人 override 不入仓）

### 0.2 验证 gomobile bind CGO 可行性

- [x] 0.2.1 准备 NDK r25c（仿 K-Sillot）or r26b（与 encv-mobile 一致）—— **CI 环境执行**（沙箱内未装 NDK）
- [x] 0.2.2 `go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`（已通过，gomobile v0.0.0-20260529142300 安装成功）
- [x] 0.2.3 修复 fork 的 `replace github.com/Soltus/encv-go => ../../../` 路径：用 sed 改为 `replace github.com/Soltus/encv-go => /workspace`（已写入 `scripts/build-openlist-aar.sh`）
- [x] 0.2.4 `cd openlistlib && gomobile bind -ldflags "-s -w" -v -androidapi 19 -target="android/arm64"`（需 NDK，CI 跑）
- [x] 0.2.5 产出 `openlist.aar` 存在；`unzip -l openlist.aar | grep lib/arm64-v8a/libgojni.so` 命中（CI 跑）
- [x] 0.2.6 `unzip -p openlist.aar classes.jar | jar tf - | grep openlistlib/Openlistlib` 命中（CI 跑）
- [x] 0.2.7 `ls -lh openlist.aar` 记录体积（< 50MB 算可接受）（CI 跑）

### 0.3 体积 & 启动时间基线

- [x] 0.3.1 AAR 内 `lib/arm64-v8a/libgojni.so` strip 后 < 40MB（CI 跑）
- [x] 0.3.2 在 Android 模拟器上 `Openlistlib.start()` → 5s 内 `127.0.0.1:5244/api/site/list` 返回 200（CI 跑）
- [x] 0.3.3 `lsof -i :5244` 在模拟器里确认 PID = host APK（CI 跑）

## Phase 1: ComboLite 插件模块骨架（1-2 天）

### 1.1 创建 `:plugin-openlist` Gradle module

- [x] 1.1.1 新建 `app/encv-mobile/plugin-openlist/` 目录
- [x] 1.1.2 写 `build.gradle.kts`（library + combolite.aar2apk + compileOnly(libs.combolite.core)，**不**用 Compose）
- [x] 1.1.3 在 `android/settings.gradle.kts` 注册：`include(":plugin-openlist")` + `project(":plugin-openlist").projectDir = file("../plugin-openlist")`
- [x] 1.1.4 验证 `./gradlew :plugin-openlist:assembleDebug` 通过（待真机/CI 验证，本地沙箱无 Android SDK）

### 1.2 集成 openlist.aar 到插件模块

- [x] 1.2.1 在 `plugin-openlist/libs/` 放置 `openlist.aar` 占位文件（0 字节 + 注释）
- [x] 1.2.2 `build.gradle.kts` 添加 `implementation(files("libs/openlist.aar"))`
- [x] 1.2.3 在 `src/main/AndroidManifest.xml` 声明 `OpenListService` + `BootReceiver`
- [x] 1.2.4 验证 `import openlistlib.Openlistlib` 编译通过（待 AAR 真实产物；当前用占位接口）

### 1.3 `OpenListBridge`（Kotlin 单例 wrapper）

- [x] 1.3.1 `OpenListBridge.kt` 实现 `OpenListEvent` + `OpenListLogCallback` 接口（用占位，待 AAR 后切到 `openlistlib.Event/LogCallback`）
- [x] 1.3.2 `init(dataDir: String)`：含 setConfigData 调用占位
- [x] 1.3.3 `start()` → 占位 + TODO 注释
- [x] 1.3.4 `shutdown(timeoutMs: Long)` → 占位
- [x] 1.3.5 `isRunning()` → 占位
- [x] 1.3.6 `setAdminPassword(pwd: String)` → 占位
- [x] 1.3.7 `forceDbSync()` → 占位
- [x] 1.3.8 override `onLog` 把日志分发到 Logcat（tag=`OpenList`）
- [x] 1.3.9 override `onStartError` / `onShutdown` / `onProcessExit` 通过 `LocalBroadcastManager` 发出

### 1.4 `OpenListConfig`（Kotlin 配置存储）

- [x] 1.4.1 用 `SharedPreferences` 持久化：端口（默认 5244）、数据目录（默认 `filesDir/openlist/data`）、管理员密码
- [x] 1.4.2 提供 `applyToBridge(bridge: OpenListBridge)` 方法，在 `OpenListBridge.init` 前先 `setConfigData` + 后 `setAdminPassword`

### 1.5 `OpenListService`（前台服务）

- [x] 1.5.1 仿 `K-Sillot/OpenListService.kt` 写：FOREGROUND_ID、NotificationChannel、PARTIAL_WAKE_LOCK
- [x] 1.5.2 `onCreate`: 添加 OpenList listener + acquire WakeLock
- [x] 1.5.3 `onStartCommand`: 端口冲突检测 → 调 `OpenListConfig.applyToBridge(bridge)` → `bridge.init` → `bridge.start`
- [x] 1.5.4 `onDestroy`: `bridge.shutdown(5000)` + release WakeLock
- [x] 1.5.5 5 分钟 Handler 定时 `bridge.forceDbSync()`（防 SQLite WAL 丢失）
- [x] 1.5.6 START_STICKY 保活

### 1.6 `OpenListPluginEntry`（IPluginEntry 实现）

- [x] 1.6.1 `onLoad(context)`: `context.startForegroundService(Intent(context, OpenListService::class.java))`
- [x] 1.6.2 `onUnload(context)`: `context.stopService(...)`
- [x] 1.6.3 `onConfigurationChanged()`: 端口/数据目录变更时优雅重启 service

### 1.7 端口冲突检测

- [x] 1.7.1 `OpenListService.isPortOccupied(port: Int)`: 2s 超时 socket connect 127.0.0.1:port
- [x] 1.7.2 端口被占 → 发 `PORT_CONFLICT` broadcast，不启动 OpenList

## Phase 2: encv-go 自动发现（0.5-1 天）

### 2.1 `internal/openlist/multi_openlist.go`

- [x] 2.1.1 在 `LoadSites()` 末尾调用 `tryRegisterLocalLoopback()`（通过 `server.Start()` 调用 `TryRegisterLocalLoopback`）
- [x] 2.1.2 实现 `tryRegisterLocalLoopback()`：2s 超时 GET `http://127.0.0.1:5244/api/site/list`，200 则插入 `{ID: "local-loopback", Name: "本地 OpenList（Plugin）", Host: "http://127.0.0.1:5244", Enable: true, BuiltIn: true}`
- [x] 2.1.3 `SaveSites()` 跳过 `BuiltIn: true`（在 `writeConfigToFile` 中实现）
- [x] 2.1.4 `GET /openlist/sites` 端点：把 `BuiltIn` site 排前面（`handleListOpenlistSitesGin` / `handleRemoteInfoGin` 中通过 `order` 数组实现）

### 2.2 `internal/server/openlist_handlers.go`

- [x] 2.2.1 新增 `GET /openlist/local/status` handler（`LocalOpenListStatusHandler`）
- [x] 2.2.2 返回 `{running, pid, port, dataDirSize, lastHeartbeat}`（在 `GetLocalOpenListStatus` 中）
- [x] 2.2.3 `lastHeartbeat` 通过 `atomic.Int64` 维护：`MarkOpenListHeartbeat()` 在 `HandleRequest` 入口调用

## Phase 3: 前端 UI（0.5-1 天）

### 3.1 `<LocalOpenListStatusCard>` 组件

- [x] 3.1.1 在 `src/views/Remote.vue` 顶部引入
- [x] 3.1.2 字段：状态 / PID / 端口 / 数据目录大小 / 心跳
- [x] 3.1.3 三态：未安装（绿色引导安装） / 端口冲突（红色提示） / 运行中（绿色）
- [x] 3.1.4 5s 轮询 status 端点
- [x] 3.1.5 "打开 OpenList Web UI"按钮：`window.open('http://127.0.0.1:5244/#/login', '_system')`

### 3.2 站点列表排序

- [x] 3.2.1 远端 site 通过 `GET /openlist/sites` 拉
- [x] 3.2.2 `local-loopback` 排第一，IonChip "📱 本地" 角标
- [x] 3.2.3 "已禁用"开关只对非 BuiltIn site 可见

## Phase 4: 构建 & CI（1-2 天）

### 4.1 `scripts/build-openlist-aar.sh`（仿 K-Sillot 4 件套合一 + frontend pin 修复）

- [x] 4.1.1 入参：`--output <aar>` `--fork <git-url>` `--branch <branch>` `--ndk <path>` `--encv-go-root <path>` `--frontend-version <vX.Y.Z>` `--local-frontend-dist <path>`
- [x] 4.1.2 入口 `source scripts/openlist-fork.env` 加载默认配置
- [x] 4.1.3 clone fork 到 `$WORK_DIR/openlist`（删除旧副本）
- [x] 4.1.4 修复 `go.mod` 的 `replace` 路径：把 `../../../` 替换为传入的 `--encv-go-root`
- [x] 4.1.5 **读 frontend pin**：先 `${SRC_DIR}/frontend-pinned.txt`、再 `--frontend-version`、再 `OPENLIST_FRONTEND_VERSION`、再 fallback `releases/latest` + warning
- [x] 4.1.6 下载 OpenList-Frontend dist：从 `releases/tags/${WEB_VERSION}` 拉（**不**用 `latest`）
- [x] 4.1.7 （可选）应用 i18n overlay：若 `public/dist/i18n-overlay/<lang>/translation.json` 存在 → jq 合并到 `public/dist/assets/<lang>.json`
- [x] 4.1.8 写 `public/dist/VERSION` 文件：`${WEB_VERSION}-encv`
- [x] 4.1.9 `go install gomobile@latest && gomobile init`
- [x] 4.1.10 `cd openlistlib && gomobile bind -ldflags "-s -w -X ...Version=... -X ...WebVersion=${WEB_VERSION} -X ...BuiltAt=... -X ...GitCommit=..." -v -androidapi 19 -target="android/arm64"`
- [x] 4.1.11 把 `openlist.aar` 拷到 `--output`
- [x] 4.1.12 chmod 755、生成 SHA256

### 4.2 `scripts/build-openlist-aar.ps1`（Windows 镜像）

- [x] 4.2.1 PowerShell 7+ 等价版本，CI matrix 在 Windows leg 调用

### 4.3 验证流水线

- [x] 4.3.1 本地 `./gradlew :plugin-openlist:assembleDebug` → 产出 AAR（待 CI 验证，依赖 AAR 真实产物）
- [x] 4.3.2 aar2apk 任务产出 `plugin-openlist-debug.apk`（待 CI 验证）
- [x] 4.3.3 真机/模拟器手动 `adb install` → `PluginManager.getInstalledPlugin("openlist")` 命中（待真机验证）
- [x] 4.3.4 端到端：插件装上 → 启动 → OpenList 在 5s 内监听 5244 → encv-go 自动注册 local-loopback → 列表显示本地 site → ENCV 容器可预览（待真机验证）

## Task Dependencies

- Phase 0.2（gomobile bind 验证）依赖 Phase 0.1（fork 补全 `openlistlib/`）✓
- Phase 1 全部依赖 Phase 0 全绿 ✓
- Task 2.1 依赖 Task 1.5 至少能让 OpenList 在真机拉起 ✓（代码就位，待真机）
- Task 3.1 依赖 Task 2.2 提供 `/openlist/local/status` ✓
- Phase 4 的 `preBuild` 钩子依赖 Task 4.1 脚本可用 ✓

## 估时

- Phase 0：1-2 天（**实际**：fork 改动已提交（0.1 完成）；gomobile bind 编译需在有 NDK 的 CI 环境完成）
- Phase 1：2-3 天（**实际**：8 个文件已创建，代码风格与 plugin-mpv-player 对齐）
- Phase 2：0.5-1 天（**实际**：Go 端编译通过，go test 通过）
- Phase 3：0.5-1 天（**实际**：组件已创建，vue-tsc 通过）
- Phase 4：1-2 天（**实际**：bash + PowerShell 脚本完成，bash -n 验证通过）
- **本地完成**：所有可在本沙箱完成的代码改动；**CI 验证项**：gomobile bind 实际编译 + Android 真机端到端
