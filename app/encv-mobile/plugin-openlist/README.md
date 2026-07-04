# plugin-openlist

ComboLite 插件：在 Android 设备上以本地服务形式运行 OpenList，端口 5244，数据目录位于 App 沙箱内。

> **本模块属于 OpenList fork 集成的客户端；fork 侧工作流（clone / push / frontend pin / i18n overlay / 沙箱 GITHUB_TOKEN 推送 / Phase 27 交叉编译流程）见 [`app/openlist/README.md`](../../openlist/README.md)。**

> **架构基线**：Phase 26+ 弃用 gomobile bind 嵌入进程模式（`openlist.aar` + `libgojni.so`），改用 host app 的 `EncvGoService` 模式（`ProcessBuilder` 启 native binary）。Phase 27 由 CI 交叉编译 `Hi-Sillot/OpenList` fork dev 分支为 `libopenlist.so`（arm64-v8a only），通过 AGP 标准 jniLibs 流程打包进本插件 APK。详见 [`/workspace/.trae/specs/build-openlist-fork-as-android-native/spec.md`](../../../.trae/specs/build-openlist-fork-as-android-native/spec.md) §5.4-5.6。

## 模块结构

```
plugin-openlist/
├── build.gradle.kts                       # abiFilters = ["arm64-v8a"] + jniLibs.srcDirs("src/main/jniLibs")
├── src/main/
│   ├── AndroidManifest.xml
│   ├── jniLibs/
│   │   └── arm64-v8a/
│   │       └── libopenlist.so             # ← Phase 27 CI 编译产物（git ignore, 每次 build 覆盖）
│   └── java/com/encvgo/plugin/openlist/
│       ├── OpenListPluginEntry.kt         # IPluginEntryClass 实现，onLoad 启 Service
│       ├── OpenListService.kt             # 前台 Service + WakeLock + 5 分钟 db sync + 端口冲突检测
│       ├── OpenListNativeService.kt       # Phase 26+ ProcessBuilder 启 libopenlist.so + 状态广播
│       └── OpenListConfig.kt              # SharedPreferences 持久化（port / dataDir / adminPassword）
└── README.md
```

> **Phase 26+ 删 `OpenListBridge.kt` + `libs/openlist.aar` 占位**：gomobile bind 入口已废弃。

## 依赖

- `compileOnly(libs.combolite.core)` — ComboLite 核心运行时（宿主提供）
- `implementation("androidx.localbroadcastmanager:localbroadcastmanager:1.1.0")` — 进程内事件广播
- `compileOnly("androidx.compose.runtime:runtime")` — 仅用于 `IPluginEntryClass.Content()` 的 `@Composable` 注解；本插件不写任何 Compose UI

> **Phase 27 删 `implementation(files("libs/openlist.aar"))`**：AAR 是 gomobile bind 时代的 in-process Go 库产物，Phase 26 重构后不再需要。

## 关键设计

### 1. 端口冲突解决

`OpenListService.startupSequence()` 在启动前以 **2 秒超时** `connect(127.0.0.1:5244)`：

- 连接成功 → 端口被占用，停止前台服务并 `LocalBroadcastManager` 发送 `BROADCAST_PORT_CONFLICT`（`EXTRA_CONFLICT_PORT`）
- 连接失败/超时 → 端口空闲，正常启动

### 2. 数据目录

默认 `${filesDir}/openlist/data`，即 App 私有目录：

```
/data/data/com.encvgo.plugin.openlist/files/openlist/data/
├── config.json
├── data.db                 # glebarez/sqlite (Phase 27 fork 切到 pure-Go)
├── data.db-wal
├── data.db-shm
├── log/
└── ...
```

可通过 `OpenListConfig.save(context, port, dataDir, password)` 修改。

### 3. 后台保活

- `START_STICKY` — 系统回收后自动重建
- `PARTIAL_WAKE_LOCK` 标签 `openlist::Service` — 防止 CPU 休眠
- `startForeground(FOREGROUND_ID, ...)` + 通知渠道 `openlist_server`，`IMPORTANCE_LOW`
- 5 分钟 Handler 定时 `OpenListNativeService.forceDbSync()`（若 fork 暴露），合并 SQLite WAL

### 4. 广播协议

| Action | 触发方 | 携带 extras | 用途 |
|--------|--------|------------|------|
| `BROADCAST_STATUS_CHANGED` | Service / NativeService | `port`, `running` | 运行状态变更 |
| `BROADCAST_PORT_CONFLICT` | Service | `conflict_port` | 端口被占 |
| `BROADCAST_LOG` | NativeService | `level`, `time`, `log` | OpenList 子进程 stdout/stderr 转发 |
| `BROADCAST_PROCESS_EXIT` | NativeService | `code` 或 `reason` | OpenList 子进程异常退出 |

### 5. libopenlist.so 启进程流（Phase 27 核心）

`OpenListNativeService.findExecutableBinary()`：

```kotlin
val nativeLibDir = applicationInfo.nativeLibraryDir  // /data/app/.../lib/arm64-v8a
val binary = File(nativeLibDir, "libopenlist.so")    // AGP jniLibs 打包的文件名
binary.setExecutable(true)   // ← jniLibs 默认非可执行
binary.setReadable(true)
binary.setWritable(true)
return binary
```

随后 `ProcessBuilder`：

```kotlin
val process = ProcessBuilder(
    binary.absolutePath,
    "server",
    "--port", "5244",
    "--data", dataDir
).start()
```

子进程启动后调 `cmd/server.go::Start()`：

```go
func Start() {
    // 1. 注册 ENCV 设置项
    encv.GenerateENCVSettingItems()
    encv.LoadENCVPluginSettings()
    encvPlugins.InitializeWithSettings()

    // 2. 上游 bootstrap
    bootstrap.InitOfflineDownloadTools()
    bootstrap.LoadStorages()
    bootstrap.InitTaskManager()

    // 3. 起 server
    r := gin.New()
    server.Init(r)
    r.Run("127.0.0.1:5244")
}
```

### 6. ABI 范围（arm64-v8a only）

`build.gradle.kts`：

```kotlin
ndk {
    abiFilters += listOf("arm64-v8a")  // Phase 27: 限制为仅 arm64-v8a
}
jniLibs.srcDirs("src/main/jniLibs")
```

**影响**：
- ✅ 编译时间从 4 ABI 的 5-8 min 降到 **~2 min**
- ✅ APK 体积从 4 ABI 的 ~120-200MB 降到 **~30-50MB**
- ⚠️ armeabi-v7a / x86 / x86_64 设备**无法运行**本插件

### 7. 权限

| 权限 | 用途 |
|------|------|
| `INTERNET` / `ACCESS_NETWORK_STATE` / `ACCESS_WIFI_STATE` | OpenList 网络访问 |
| `FOREGROUND_SERVICE` | 前台 Service |
| `FOREGROUND_SERVICE_DATA_SYNC` | Android 14+ 前台服务类型 |
| `POST_NOTIFICATIONS` | Android 13+ 通知权限 |
| `WAKE_LOCK` | PARTIAL_WAKE_LOCK |
| `RECEIVE_BOOT_COMPLETED` | 开机自启（暂未实现 BroadcastReceiver，后续任务扩展） |

## 编译（Phase 27 全 CI 流程）

```bash
# 1) push 到 trae/solo-agent-WAmQzy 分支即可触发 CI
git push origin trae/solo-agent-WAmQzy

# 2) CI 在 .github/workflows/android.yml 跑以下 steps:
#    ① Setup Go 1.25.1 (from app/openlist/Hi-Sillot-OpenList/go.mod)
#    ② Build OpenList native libs (arm64-v8a, CGO+NDK)
#       - CGO_ENABLED=1 GOOS=android GOARCH=arm64
#       - CC=<NDK>/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android24-clang
#       - go build -o /tmp/libopenlist-arm64.so ./
#    ③ Copy libopenlist.so to jniLibs
#       - cp /tmp/libopenlist-arm64.so app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a/libopenlist.so
#    ④ ./gradlew :plugin-openlist:assembleRelease (aar2apk 任务 addNativeLibs 打包)
#
# 3) 产物路径
#    app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk
#    ├── AndroidManifest.xml
#    ├── classes.dex
#    ├── lib/arm64-v8a/libopenlist.so    ← Phase 27 编译产物
#    └── ...
#
# 4) 安装到设备
adb install -r app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk
```

> **本地无 Go 工具链也能跑 build**——CI 负责交叉编译 + 拷贝 `libopenlist.so`，本地只需 `./gradlew :plugin-openlist:assembleRelease`（但需要先有 `libopenlist.so`，否则 plugin APK 会缺 native binary）。
>
> 如需本地 dev 模式跑 OpenList（不依赖 APK），用 [`app/encv-mobile/scripts/dev-openlist.sh`](../scripts/dev-openlist.sh) —— 它直接 `go run . server`，不依赖 jniLibs 流程。

## 当前状态

- ✅ 插件框架：完整骨架，`IPluginEntryClass` 实现 + 前台 Service + 配置持久化
- ✅ 端口冲突检测 + 广播转发
- ✅ WakeLock / 5 分钟 db sync / `START_STICKY`
- ✅ Phase 26 重构：删除 `OpenListBridge.kt` + `libs/openlist.aar`，改用 `OpenListNativeService` + `ProcessBuilder`
- ✅ Phase 27 集成：CI 交叉编译 fork 为 `libopenlist.so`（arm64-v8a only）→ AGP jniLibs 流程
- ⚠️  BootReceiver 未实现（`RECEIVE_BOOT_COMPLETED` 权限已声明，待后续任务）
- ⚠️  端到端设备验证待 T4.x（沙盒无 Android 设备，需 CI runner 真机或模拟器）

## 故障排查

Phase 27 后常见故障见 [`app/openlist/README.md` §12](../../openlist/README.md#12-故障排查-checklist)。本插件特有：

| 症状 | 根因 | 修复 |
|------|------|------|
| `libopenlist.so` not found in nativeLibraryDir | CI 编译 step ③ 没跑 / 路径错 | 看 `app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a/` 是否存在 `libopenlist.so` |
| `binary.canExecute() == false` | jniLibs 默认 0644 | `OpenListNativeService.findExecutableBinary()` 必须 `setExecutable(true)` |
| OpenList 启动后 5s 内 5244 不响应 | ffmpeg dlopen 失败 / port conflict / dataDir 权限 | `adb logcat -s OpenListNativeService`；先单跑 `dev-openlist.sh` 验证 |
