# 修复 Release APK 前端资源缺失 + 废弃 android-overlay

## Why
Release APK 启动后 WebView 显示经典安卓小人 `net::ERR_CONNECTION_REFUSED`。**根因**：在修复签名问题的 spec（commit `d679389`）中，将 `npx cap build android --keystorepath ...` 替换为直接 `./gradlew assembleRelease`。`cap build android` 内部会调用 `cap sync`（复制 Web 资源到 assets/public/），切换到 gradlew 后这个隐式同步步骤丢失了。release 构建的 `npx cap copy android` 步骤本身就有 `if: inputs.version == ''` 的 debug-only 限制，所以 release 构建完全没有执行过任何 Web 资源同步。

**android-overlay 是过期产物**：overlay 中的 .kt 文件是旧版本（缺少 WakeLock、ensureBuildInfoExists、ENCV_LIB_DIR、mobile/recover/admin/webdav/proxy 配置字段等后续添加的功能），且残留 `config.mobile.json` 幽灵引用。CI workflow 中从未调用过 `post-cap-sync.mjs` 或 `sync-native.mjs`（grep 零结果），overlay 实际上已废弃但仍在制造混淆。

## What Changes
- **CI workflow**：移除 "Copy web assets" 步骤的 `if: inputs.version == ''` 限制，release 构建也执行 `npx cap copy android`
- **删除 `android-overlay/` 目录**：其 .kt 文件已过时（比 deployed 版本少功能），且 CI 从未调用 post-cap-sync.mjs 使用它。保留只会造成幽灵改动回归风险
- **清理 `post-cap-sync.mjs`**：该脚本引用 overlay 作为 Kotlin 源，overlay 删除后此脚本也应删除或标记废弃
- **在 `sync-native.mjs` 中添加 config.user.json 复制**：确保 `config.user.json` 进入 APK assets
- **CI 添加验证**：检查 APK 内 `public/index.html` 和 `config.user.json` 存在性

## Impact
- Affected code:
  - `.github/workflows/android.yml` — 移除 debug-only 限制
  - `app/encv-mobile/android-overlay/` — 整个目录删除
  - `app/encv-mobile/scripts/post-cap-sync.mjs` — 废弃或删除
  - `app/encv-mobile/scripts/sync-native.mjs` — 添加 config.user.json 复制
- **BREAKING**: 删除 android-overlay/ 目录后，任何依赖 post-cap-sync.mjs 的本地开发流程需改用 sync-native.mjs（当前已是 package.json 的 capacitor:copy:after hook）

## ADDED Requirements

### Requirement: Release APK 必须包含前端 Web 资源
CI release 构建必须在 Gradle 构建前执行 `npx cap copy android`，将 dist/ 复制到 Android 项目 assets/public/。

#### Scenario: Release 构建后 APK 包含 Web 资源
- **WHEN** CI 执行 release 构建（version 参数非空）
- **THEN** APK 的 `assets/public/index.html` 存在且非空
- **AND** WebView 能正常加载 `https://localhost/`

### Requirement: sync-native.mjs 必须复制 config.user.json 到 assets
`sync-native.mjs`（package.json 的 `capacitor:copy:after` hook）必须将项目根目录的 `config.user.json` 复制到 `android/app/src/main/assets/`。

#### Scenario: cap copy 执行后 assets 包含 config.user.json
- **WHEN** `npx cap copy android` 执行完毕
- **THEN** `android/app/src/main/assets/config.user.json` 存在

### Requirement: CI 验证 APK 内关键资源存在性
"Verify APK contents" 步骤必须检查 `public/index.html` 和 `config.user.json` 在 APK assets 中存在。

#### Scenario: 缺失资源导致构建失败
- **WHEN** APK 中无 `public/index.html` 或无 `config.user.json`
- **THEN** 构建失败并输出明确错误信息

## MODIFIED Requirements

### Requirement: CI release 构建流程
release 构建不再使用 `npx cap build android`（签名问题 spec 改用 gradlew），因此必须显式添加 `npx cap copy android` 步骤来补回丢失的 Web 资源同步。

## REMOVED Requirements

### Requirement: android-overlay 目录
**Reason**:
1. overlay 中 EncvGoService.kt 比 deployed 版本**少以下功能**：WakeLock、ensureBuildInfoExists()、ENCV_LIB_DIR 环境变量、mobile 配置合并、recover/default_container_version/admin/webdav/proxy 配置字段
2. overlay 残留 `config.mobile.json` 引用（幽灵改动）
3. CI workflow 从未调用 `post-cap-sync.mjs`（grep 零结果），overlay 无实际消费者
4. 当前生效的 hook 是 `sync-native.mjs`（package.json 的 `capacitor:copy:after`），它只处理 jniLibs/include/jni 不涉及 Kotlin 文件
5. 保留 overlay 会持续造成幽灵改动风险

**Migration**: 直接删除整个 `android-overlay/` 目录和 `post-cap-sync.mjs`。Kotlin 源文件以 `android/app/src/main/java/` 为唯一来源。
