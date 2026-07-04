# Hi-Sillot/OpenList fork 交叉编译为 Android Native Binary (Phase C) Spec

## Why

Phase 26 重构让 `plugin-openlist` 改用 host app 的 `EncvGoService` 模式（`ProcessBuilder` 启 native binary），但 `libopenlist.so` 还没编出来。**Hi-Sillot/OpenList fork dev 分支已切到 `glebarez/sqlite`（pure-Go，CGO-free）**——这一改动让 fork 可以 **`CGO_ENABLED=0` 交叉编译为 Android native binary**，彻底摆脱 gomobile bind 链路的 NDK toolchain 兼容坑。

`integrate-openlist-as-combolite-plugin` 关注的是 gomobile bind 时代的 `openlistlib/` 入口，**与本 spec 无关**——Phase 26 之后 `openlistlib/` 目录是死代码（plugin-openlist 不再调 gomobile bind 产物），本 spec 走 **`cmd/` 入口**（`openlist server` cobra command）独立交叉编译为 `libopenlist.so`。

**ABI 范围决策**：**只编 arm64-v8a 一个 ABI**。理由：
- 现代 Android 设备（2018 年后）几乎都是 arm64-v8a
- Google Play 2019 起强制 64-bit；OpenList 主要面向 64-bit 设备
- 编译时间从 4 ABI 的 5-8 min 降到 ~2 min
- APK 体积从 4 ABI 的 ~120-200MB 降到 **~30-50MB**
- 简化：少 3 个 ABI 的 build script、jniLibs 目录、aar2apk addNativeLibs 复杂度

## What Changes

### 新增
- `app/openlist/Hi-Sillot-OpenList/`（fork clone，dev 分支，**只入 fork 源码，不入 CI 编译产物**）
- `.github/workflows/android.yml` Step：Go 1.25.1 setup + fork **arm64-v8a** 交叉编译
- `.github/workflows/android.yml` Step：拷 `libopenlist.so` 到 `plugin-openlist/src/main/jniLibs/arm64-v8a/`
- `.github/workflows/android.yml` Step：验证 `libopenlist.so` arm64-v8a（`file`/`ls -lh` 验证）
- `app/openlist/README.md` 新增「Phase C 交叉编译流程」章节

### 删/废弃
- `.github/workflows/android.yml` Step：`[Phase 26] OpenList native binary placeholder`（被实际编译 step 替代）
- `scripts/build-openlist-aar.sh`（gomobile bind 脚本，Phase 26 已废弃，本 spec 正式删除）
- `.github/workflows/android.yml` abiFilters 限制为仅 arm64-v8a（plugin-openlist `defaultConfig.ndk.abiFilters`）

### 不变
- `plugin-openlist/build.gradle.kts`（Phase 26 已设好 `jniLibs.srcDirs("src/main/jniLibs")`，本 spec 不动）
- `plugin-openlist/.../OpenListNativeService.kt`（Phase 26 已实现 ProcessBuilder 启 `libopenlist.so` 路径）
- `app/encv-mobile/fork/`（如果存在）—— 本 spec 不引入新路径，**统一用 `app/openlist/Hi-Sillot-OpenList/`**

## Impact

### Affected specs
- `integrate-openlist-as-combolite-plugin`（gomobile bind 时代）—— 文档归档
- `openlist-fork-onboarding-readme` —— 文档补充

### Affected code
- `.github/workflows/android.yml`（+2-3 steps，+Go 1.25.1 setup，+fork 编译 **~2 min** build time）
- `app/encv-mobile/plugin-openlist/build.gradle.kts`（`abiFilters` 从 4 ABI 改为 1 ABI）
- `app/openlist/README.md`（+1 章节「Phase C 交叉编译」）
- `app/openlist/Hi-Sillot-OpenList/.gitignore`（**不**入编译产物 .so / .a）
- `scripts/build-openlist-aar.sh`（**删**）

### Affected artifact
- `plugin-openlist.apk` 含 `lib/arm64-v8a/libopenlist.so`（**仅 1 个 ABI**）
- 体积：~30-50MB（单 ABI libopenlist.so）

## ADDED Requirements

### Requirement: fork dev 分支的纯 Go arm64-v8a 交叉编译可行性
The CI system SHALL be able to build `Hi-Sillot/OpenList` fork's dev branch for **arm64-v8a only** using pure-Go cross-compilation (no CGO toolchain required).

#### Scenario: arm64-v8a build succeeds
- **WHEN** CI runs `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -buildmode=c-shared -o libopenlist-arm64.so ./cmd` in `app/openlist/Hi-Sillot-OpenList/`
- **THEN** the build succeeds without NDK toolchain
- **AND** produces a valid Android shared library (`libopenlist-arm64.so`)
- **AND** `file libopenlist-arm64.so` reports `ELF 64-bit LSB shared object, ARM aarch64`

#### Scenario: glebarez/sqlite 不触发 CGO
- **WHEN** Go linker resolves `github.com/glebarez/sqlite v1.11.0` (the dep fork uses per `go.mod:44`)
- **THEN** no CGO toolchain is invoked (pure-Go implementation, see `.trae/rules/android.md` §五 选型铁律)

### Requirement: plugin-openlist.apk 含 libopenlist.so
The plugin-openlist.apk SHALL contain `lib/arm64-v8a/libopenlist.so` (single ABI).

#### Scenario: AGP jniLibs 流程正常打包
- **WHEN** `libopenlist.so` copied to `app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a/`
- **THEN** `./gradlew convert_plugin-openlist_release` (aar2apk task) succeeds
- **AND** the output APK's `lib/arm64-v8a/` contains `libopenlist.so` (Phase 26 jniLibs 流程天然支持)

#### Scenario: ProcessBuilder 启 libopenlist.so 成功
- **WHEN** plugin-openlist APK installed on Android device
- **THEN** `OpenListNativeService.start()` invokes `ProcessBuilder(applicationInfo.nativeLibraryDir + "/lib/arm64-v8a/libopenlist.so", "server", "--port", "5244", "--data", dataDir)` 
- **AND** the Go process runs and starts listening on `127.0.0.1:5244`
- **AND** `OpenListEmbedWebView` loads `http://127.0.0.1:5244/` and displays OpenList web UI

### Requirement: fork 的 monorepo replace 指令兼容
The `replace github.com/Soltus/encv-go => ../../../` directive in fork's `go.mod` SHALL resolve correctly in CI.

#### Scenario: encv-go 仓库根可被 Go 找到
- **WHEN** CI checkout puts `Hi-Sillot-OpenList` at `/workspace/app/openlist/Hi-Sillot-OpenList/`
- **THEN** `../../../` resolves to `/workspace/` which contains encv-go's `pkg/encv/plugins/` etc.
- **AND** `go build` succeeds with `encv.LoadENCVPluginSettings` symbol resolution

#### Scenario: fork 编译产物含 ENCV 集成代码
- **WHEN** `libopenlist.so` is built
- **THEN** it contains references to `cv.encv.LoadENCVPluginSettings` and `encvPlugins.InitializeWithSettings` (per `cmd/server.go:51-58`)
- **AND** runtime: `libopenlist.so server` initializes ENCV plugins before OpenList server starts

### Requirement: Phase 26 placeholder step 替换
The CI workflow SHALL replace the Phase 26 placeholder step with the actual fork build steps.

#### Scenario: placeholder step 替换为真实编译
- **WHEN** this spec is implemented
- **THEN** `[Phase 26] OpenList native binary placeholder` step is removed from `.github/workflows/android.yml`
- **AND** in its place, 2-3 new steps are added: `Setup Go`, `Build OpenList native libs (arm64-v8a)`, `Copy libopenlist.so to jniLibs`

#### Scenario: 旧 gomobile 脚本彻底删
- **WHEN** this spec is implemented
- **THEN** `scripts/build-openlist-aar.sh` is removed (git rm)
- **AND** no other workflow step references this script

### Requirement: plugin-openlist 仅打 arm64-v8a 一个 ABI
The plugin-openlist build configuration SHALL be restricted to arm64-v8a only.

#### Scenario: abiFilters 限定为 arm64-v8a
- **WHEN** Phase C is implemented
- **THEN** `plugin-openlist/build.gradle.kts` `defaultConfig.ndk.abiFilters` is `listOf("arm64-v8a")` (was `listOf("arm64-v8a", "armeabi-v7a", "x86", "x86_64")` in Phase 26)
- **AND** `src/main/jniLibs/` only contains `arm64-v8a/` subdirectory

## MODIFIED Requirements

### Requirement: plugin-openlist/build.gradle.kts 显式声明 jniLibs 流程 (Phase 26)
**Was**: `jniLibs.srcDirs("src/main/jniLibs")` declared + `abiFilters += listOf("arm64-v8a", "armeabi-v7a", "x86", "x86_64")` (4 ABI).
**Now**: `jniLibs.srcDirs("src/main/jniLibs")` declared + `abiFilters += listOf("arm64-v8a")` (1 ABI only).

**Modified file**: `app/encv-mobile/plugin-openlist/build.gradle.kts` — 4 行 abiFilters 列表改 1 行.

## REMOVED Requirements

### Requirement: gomobile bind AAR 产物依赖
**Reason**: Phase 26 重构决定完全弃用 gomobile bind 嵌入进程模式，改用 host app 的 ProcessBuilder 模式启独立 Go native binary 进程。gomobile bind 产物 (`openlist.aar` / `openlist-classes.jar` / `libgojni.so` 150MB) 已被删除，本 spec 不再需要构建它们。

**Migration**: 不需要迁移（Phase 26 已经在 commit `60f5891` 完成删除）。本 spec 进一步删除 `scripts/build-openlist-aar.sh` 脚本。

## 已知风险与缓解

| 风险 | 缓解 |
|------|------|
| **fork 编译失败**（`go build` 撞 fork 内部 Go 错误，与 glebarez/sqlite 无关） | 阶段 1 单独跑 `go build ./cmd` 验证；失败先修 fork，再继续 |
| **fork replace 指令解析失败**（CI runner checkout path 不是 `/workspace`） | 阶段 1 在 `app/openlist/Hi-Sillot-OpenList/` 下跑 `go build` 验证 replace 解析 |
| **Go 1.25.1 在 actions/setup-go 不支持** | 用 `actions/setup-go@v5` + `go-version-file: app/openlist/Hi-Sillot-OpenList/go.mod` 自动取版本 |
| **CGO_ENABLED=0 撞 fork 内部 cgo 依赖** | grep 验证 fork 无 `import "C"`；有则先转 pure-Go 等价物 |
| **aar2apk 任务 addNativeLibs 失败** | 已 Phase 26 验证 jniLibs 流程工作；不需额外处理 |
| **armeabi-v7a / x86 / x86_64 设备无法运行** | 接受——现代 Android 设备几乎都是 arm64-v8a；如有需求可后续扩展 |
| **fork 与 encv-go 强耦合**（go.mod replace） | 接受——fork 设计如此；不引入 monorepo 重构 |

## 验证清单

1. ✅ Hi-Sillot/OpenList dev 分支 clone 到 `app/openlist/Hi-Sillot-OpenList/`
2. ✅ CI 装 Go 1.25.1（从 fork 的 go.mod 读版本）
3. ✅ CI 跑 `go build -buildmode=c-shared ./cmd` 成功（仅 arm64-v8a）
4. ✅ `libopenlist.so` 拷到 `plugin-openlist/src/main/jniLibs/arm64-v8a/`
5. ✅ `./gradlew convert_plugin-openlist_release` 成功
6. ✅ plugin-openlist.apk 含 `lib/arm64-v8a/libopenlist.so`
7. ✅ ProcessBuilder 启 `libopenlist.so server` 监听 5244
8. ✅ OpenListEmbedWebView 加载 `http://127.0.0.1:5244/` 显示 OpenList web UI
9. ✅ 删 `scripts/build-openlist-aar.sh`
10. ✅ 删 `.github/workflows/android.yml` Phase 26 placeholder step
11. ✅ `plugin-openlist/build.gradle.kts` `abiFilters` 改为仅 arm64-v8a
