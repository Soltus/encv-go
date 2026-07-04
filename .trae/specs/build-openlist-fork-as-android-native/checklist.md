# Checklist — Build OpenList Fork as Android Native Binary (Phase C)

> **ABI 范围决策**：**只编 arm64-v8a 一个 ABI**（4 ABI → 1 ABI 简化）

## 阶段 0: 准备与验证

- [ ] `app/openlist/Hi-Sillot-OpenList/` 目录存在，含 fork 源码
- [ ] `git log --oneline -5` HEAD 为 `404daf0 refactor(db): switch to glebarez/sqlite`
- [ ] `go.mod` 第 3 行是 `go 1.25.1`
- [ ] `internal/bootstrap/db.go:15` 引用 `github.com/glebarez/sqlite`
- [ ] `cmd/server.go:14` 含 `Use: "server"`（cobra command）
- [ ] `cmd/server.go:48` 调 `cv.encv.LoadENCVPluginSettings()`
- [ ] `go.mod` 含 `replace github.com/Soltus/encv-go => ../../../`
- [ ] `go mod download` 退出码 0
- [ ] `grep -rn 'import "C"\|#cgo' app/openlist/Hi-Sillot-OpenList/ --include="*.go"` 无输出

## 阶段 1: 本地编译验证（arm64-v8a only）

- [ ] `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -buildmode=c-shared -o /tmp/libopenlist-arm64.so ./cmd` 成功
- [ ] `file /tmp/libopenlist-arm64.so` 显示 `ARM aarch64`
- [ ] 产物体积在 30-60MB 范围
- [ ] **不**编 armeabi-v7a / x86 / x86_64（仅 arm64-v8a）
- [ ] `plugin-openlist/build.gradle.kts` `defaultConfig.ndk.abiFilters` 是 `listOf("arm64-v8a")`（长度 1）
- [ ] `src/main/jniLibs/` 只含 `arm64-v8a/` 子目录

## 阶段 2: CI 步骤集成

- [ ] `.github/workflows/android.yml` 删 `Restore OpenList AAR from cache` step
- [ ] `.github/workflows/android.yml` 删 `Build OpenList AAR (gomobile bind)` step
- [ ] `.github/workflows/android.yml` 删 `Extract OpenList AAR for plugin packaging` step
- [ ] `.github/workflows/android.yml` 删 `[Phase 26] OpenList native binary placeholder` step
- [ ] `.github/workflows/android.yml` 新增 `Setup Go 1.25.1` step (actions/setup-go@v5, go-version-file)
- [ ] `.github/workflows/android.yml` 新增 `Build OpenList native libs (Go cross-compile, arm64-v8a)` step
- [ ] `.github/workflows/android.yml` 新增 `Copy libopenlist.so to plugin-openlist jniLibs` step
- [ ] `scripts/build-openlist-aar.sh` 已 `git rm`
- [ ] `grep -rn "build-openlist-aar\|gomobile" .github/workflows/android.yml` 无输出

## 阶段 3: plugin-openlist.aar build 验证

- [ ] `./gradlew -PincludePlugins=true "convert_plugin-openlist_release" --stacktrace` 退出码 0
- [ ] `app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk` 存在
- [ ] `unzip -l plugin-openlist-release.apk | grep libopenlist` 显示 **1 行**（仅 `lib/arm64-v8a/libopenlist.so`）
- [ ] APK 体积增量在 ~30-50MB 范围（单 ABI libopenlist.so）

## 阶段 4: 端到端验证（设备，可选）

- [ ] `adb install -r` plugin-openlist.apk 成功
- [ ] host app 启动 OpenList plugin 触发 `OpenListNativeService.start()`
- [ ] logcat 含 `OpenList-Native` tag：
  - `located libopenlist.so | path=/data/app/.../lib/arm64-v8a/libopenlist.so`
  - `starting OpenList server | port=5244`
- [ ] `adb shell netstat -tlnp | grep 5244` 显示监听
- [ ] WebView 加载 `http://127.0.0.1:5244/` 显示 OpenList web UI
- [ ] admin password 登录成功

## 阶段 5: 文档更新

- [ ] `app/openlist/README.md` 新增「Phase C 交叉编译」章节（≥ 50 行）
- [ ] `plugin-openlist/README.md` 删 `implementation(files("libs/openlist.aar"))` 引用
- [ ] `plugin-openlist/README.md` 加 Phase 26/Phase C 架构说明（libopenlist.so via jniLibs + arm64-v8a only）

## 不变项（回归测试）

- [ ] `plugin-openlist/build.gradle.kts` `jniLibs.srcDirs("src/main/jniLibs")` 仍存在（**不**为 Phase C 删除）
- [ ] `OpenListNativeService.kt` 仍是 Phase 26 仿 EncvGoService 模式（**不**为 Phase C 再改）
- [ ] host app `GoProcessPlugin.kt` classloader 反射类名仍是 `OpenListNativeService`（**不**为 Phase C 再改）
- [ ] `.github/workflows/android.yml` step 顺序：Go setup → fork 编译 → jniLibs 拷贝 → gradle aar2apk → apk 验证
