# CI 构建修复计划 — plugin-mpv-player 项目目录未设置

## 问题分析

CI 有 3 个相关错误，全部指向同一根因：

1. **Step 26**: `Cannot locate tasks that match ':plugin-mpv-player:compileDebugKotlin'`
2. **Step 27**: `Cannot locate tasks that match ':plugin-mpv-player:buildDebugPluginApk'`
3. **Step 29**: `Task with name 'assembleDebug' not found in project ':plugin-mpv-player'`

**根因**：`settings.gradle.kts` 中 `include(":plugin-mpv-player")` 没有设置 `projectDir`。Gradle 默认在 `android/plugin-mpv-player/` 下查找模块，但实际目录在 `app/encv-mobile/plugin-mpv-player/`（即 `../plugin-mpv-player` 相对于 `android/`）。

由于找不到 `build.gradle.kts`，Kotlin/Android/aar2apk 插件都没有被应用到该模块，所以所有任务都不存在。

### 次要问题

- `aaptOptions` 在 AGP 8.x 中已废弃，应改为 `androidResources {}`
- `dependencyResolutionManagement` 的 `PREFER_SETTINGS` 模式对 Capacitor 子项目产生仓库警告（不影响构建，暂不处理）

## 修复步骤

### 步骤 1：修改 `settings.gradle.kts` — 添加 plugin-mpv-player 的 projectDir

**文件**：`/workspace/app/encv-mobile/android/settings.gradle.kts`

在 `include(":plugin-mpv-player")` 之后添加：
```kotlin
project(":plugin-mpv-player").projectDir = file("../plugin-mpv-player")
```

### 步骤 2：修改 `app/build.gradle.kts` — 修复 aaptOptions 废弃警告

**文件**：`/workspace/app/encv-mobile/android/app/build.gradle.kts`

将 `aaptOptions { ignoreAssetsPattern = "..." }` 改为 `androidResources { ignoreAssetsPattern = "..." }`

### 步骤 3：删除 job_logs/ 和 job_logs.zip

按用户要求，修复完成后删除。

## 影响范围

- `settings.gradle.kts`：添加 1 行 projectDir 设置
- `app/build.gradle.kts`：修复 1 处废弃 API
- CI 工作流无需修改（Step 26 的文件检查路径虽然不精确，但不影响构建）
