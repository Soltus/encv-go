# Tasks — Build OpenList Fork as Android Native Binary (Phase C)

> **ABI 范围决策**：**只编 arm64-v8a 一个 ABI**（4 ABI → 1 ABI 简化）

## 阶段 0: 准备与验证

### T0.1 浅克隆 Hi-Sillot/OpenList dev 分支
- [ ] 验证 `app/openlist/Hi-Sillot-OpenList/` 已 clone 到本地（dev 分支，depth 50）
- [ ] 验证 `cd app/openlist/Hi-Sillot-OpenList && git log --oneline -5` 显示 `404daf0 refactor(db): switch to glebarez/sqlite` 在 HEAD
- [ ] 验证 `cat go.mod | head -3` 显示 `go 1.25.1`
- [ ] 验证 `grep -l glebarez internal/bootstrap/db.go` 确认 glebarez/sqlite 已切换
- [ ] 验证 `cat cmd/server.go | grep 'Use:'` 显示 `server`（与 OpenListNativeService 调用一致）
- [ ] 验证 `cat go.mod | grep 'replace.*encv-go'` 显示 `replace github.com/Soltus/encv-go => ../../../`
- **Validation**: `go version` (需 ≥ 1.25.1)；`go env GOMODCACHE` 路径有效

### T0.2 fork go mod download
- [ ] `cd app/openlist/Hi-Sillot-OpenList && go mod download` 成功（验证 go.sum 完整）
- [ ] `go mod tidy` 跑过（**dry-run 模式**，不写回 go.mod，只看是否缺包）
- **Validation**: 退出码 0；无 `missing go.sum entry` 错误

## 阶段 1: 本地验证 fork 交叉编译可行性（仅 arm64-v8a）

### T1.1 arm64-v8a 编译验证
- [ ] `cd app/openlist/Hi-Sillot-OpenList`
- [ ] `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -buildmode=c-shared -o /tmp/libopenlist-arm64.so ./cmd`
- [ ] 验证退出码 0
- [ ] 验证产物 `file /tmp/libopenlist-arm64.so` 显示 `ELF 64-bit LSB shared object, ARM aarch64`
- [ ] 验证 `ls -lh /tmp/libopenlist-arm64.so` 体积在 30-60MB 范围
- **Validation**: ELF 头 magic + aarch64 arch + 体积合理

### T1.2 grep 验证 fork 无 CGO 依赖
- [ ] `grep -rn 'import "C"\|#cgo' app/openlist/Hi-Sillot-OpenList/ --include="*.go" | head` 应**无输出**（除 docstring）
- [ ] 如果有 CGO 引用，先替换为 pure-Go 等价物再继续
- **Validation**: 0 个 CGO 引用

### T1.3 abiFilters 配置调整
- [ ] `app/encv-mobile/plugin-openlist/build.gradle.kts` `defaultConfig.ndk.abiFilters` 改 `listOf("arm64-v8a")`（删除其他 3 ABI）
- [ ] 验证 `src/main/jniLibs/` 只含 `arm64-v8a/` 子目录
- **Validation**: abiFilters 列表长度 1

## 阶段 2: CI 步骤集成

### T2.1 Go 1.25.1 setup step
- [ ] `.github/workflows/android.yml` 新增 step：`- name: Setup Go 1.25.1` 用 `actions/setup-go@v5` 配 `go-version-file: app/openlist/Hi-Sillot-OpenList/go.mod`
- [ ] 验证：`go version` 输出 `go version go1.25.1 ...`
- **Validation**: CI log 含 `go1.25.1` 字样

### T2.2 fork 编译 step（替代 Phase 26 placeholder）
- [ ] `.github/workflows/android.yml` **删** step `- name: "[Phase 26] OpenList native binary placeholder"`
- [ ] 新增 step `- name: Build OpenList native libs (Go cross-compile, arm64-v8a)`：
  - `cd app/openlist/Hi-Sillot-OpenList`
  - `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -buildmode=c-shared -o /tmp/libopenlist-arm64.so ./cmd`
  - 编译失败 fail-fast (`set -e`)
- [ ] 编译产物 log：`ls -lh /tmp/libopenlist-arm64.so` + `file /tmp/libopenlist-arm64.so`
- **Validation**: CI log 含 libopenlist-arm64.so 编译记录，体积合理

### T2.3 拷贝到 plugin-openlist jniLibs step
- [ ] `.github/workflows/android.yml` 新增 step `- name: Copy libopenlist.so to plugin-openlist jniLibs`：
  - `mkdir -p app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a`
  - `cp /tmp/libopenlist-arm64.so app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a/libopenlist.so`
  - 验证 `ls -lh app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a/libopenlist.so` 显示文件
- **Validation**: CI log 含 libopenlist.so 拷贝记录

### T2.4 旧 gomobile 脚本彻底删
- [ ] `git rm scripts/build-openlist-aar.sh`（或直接 `rm` 删文件）
- [ ] `grep -rn "build-openlist-aar\|gomobile" .github/workflows/android.yml` 应**无输出**（除注释）
- **Validation**: 0 个 gomobile 引用

## 阶段 3: plugin-openlist.aar build 验证

### T3.1 aar2apk 流程集成
- [ ] CI 跑 `./gradlew -PincludePlugins=true "convert_plugin-openlist_release" --stacktrace`
- [ ] 验证退出码 0
- [ ] 验证产物：`ls -lh app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk`
- [ ] 验证 `unzip -l app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk | grep libopenlist` 显示 `lib/arm64-v8a/libopenlist.so`（**1 行**）
- **Validation**: plugin APK 含单 ABI libopenlist.so

### T3.2 APK 体积审计
- [ ] 对比 Phase 26 缺 libopenlist.so 的 APK 体积 vs Phase C 含 libopenlist.so 的 APK 体积
- [ ] 期望增量：~30-50MB（单 ABI libopenlist.so）
- [ ] **不要**走 splits / app bundle（保留 fat APK 简化 aar2apk 流程）
- **Validation**: APK 体积增加量在预期范围

## 阶段 4: 端到端验证（真机/模拟器，可选）

### T4.1 设备安装 plugin-openlist.apk
- [ ] `adb install -r app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk`
- [ ] 验证安装成功
- **Validation**: `adb shell pm list packages | grep openlist` 显示包名

### T4.2 ProcessBuilder 启 libopenlist.so
- [ ] 在 host app 启动 OpenList plugin
- [ ] logcat 过滤 `OpenList-Native` tag：
  - `located libopenlist.so | path=/data/app/.../lib/arm64-v8a/libopenlist.so | size=...`
  - `starting OpenList server | port=5244`
  - `[openlist/stdout] data_size=...`（周期性）
- [ ] 验证 `adb shell netstat -tlnp | grep 5244` 显示 OpenList 进程监听 5244
- **Validation**: OpenList Go server 成功启动监听 5244

### T4.3 WebView 加载 OpenList web UI
- [ ] 在 host app 进入 OpenList plugin tab
- [ ] 验证 WebView 加载 `http://127.0.0.1:5244/` 显示 OpenList web UI（登录页）
- [ ] 输入 admin password 进入 dashboard
- **Validation**: OpenList web UI 正常渲染 + 可登录

## 阶段 5: 文档更新

### T5.1 app/openlist/README.md 新增章节
- [ ] 在「Hi-Sillot fork 维护工作流」之后新增「Phase C 交叉编译」章节
- [ ] 内容：Go 1.25.1 装、arm64-v8a build mode、jniLibs 拷贝、CI 集成点、**只编 arm64-v8a 不编其他 ABI 的原因**
- **Validation**: 文档 ≥ 50 行新增

### T5.2 plugin-openlist/README.md 更新
- [ ] 删 `implementation(files("libs/openlist.aar"))` 那行（Phase 26 已删 .aar 但 README 没更新）
- [ ] 加 Phase 26/Phase C 架构说明（libopenlist.so via jniLibs + arm64-v8a only）
- **Validation**: README 与实际构建配置一致

# Task Dependencies

- T0.1 → T0.2 → T1.1 → T1.2 → T1.3 → T2.1 → T2.2 → T2.3 → T3.1 → T3.2 → T4.1 → T4.2 → T4.3
- T2.4 独立（删旧脚本），可与 T1.x 并行
- T5.1 / T5.2 独立（文档更新），可在 T2.x 之后任意时刻

# 关键决策点

- **T1.1 失败时**：fork 内部 Go 错误（与 glebarez 无关）→ 提交 issue 给 Hi-Sillot/OpenList fork；不要硬改 fork 源码（避免与上游 drift）
- **T1.2 撞 CGO**：fork 有 `import "C"` → 评估是否必要；非必要则先转 pure-Go 等价物再继续
- **T2.1 失败时**：GitHub Actions runner 不支持 Go 1.25.1 → fallback 用 `actions/setup-go@v5` + `go-version: '1.25.1'` 手动写版本
- **T3.1 失败时**：aar2apk 任务 addNativeLibs 没把 jniLibs 拷进 APK → 检查 `src/main/jniLibs` 路径是否与 plugin-openlist 的 `applicationId` 包名一致

# Phase D 后可考虑（不包含在本 spec）

- **D.1** 多 ABI 扩展：如有 armeabi-v7a / x86 / x86_64 设备需求，把 T1.3/T2.2/T2.3 扩展为 for 循环编 4 ABI
- **D.2** fork upstream rebase：定期 rebase Hi-Sillot/OpenList fork 到 OpenListTeam/OpenList main，吸收上游特性
- **D.3** Host app GoProcessPlugin 已有 OpenListNativeService.statusListener 反射的完全迁移（去掉反射，改用 ContentProvider）
