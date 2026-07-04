# 修复 Gradle 构建失败：PREFER_SETTINGS 与 Capacitor 插件仓库冲突

## 问题分析

Gradle 构建失败，错误信息：
```
Build was configured to prefer settings repositories over project repositories
but repository 'Google' was added by build file '../node_modules/@capacitor/android/capacitor/build.gradle'
```

### 根因

`settings.gradle.kts` 第 15 行设置了：
```kotlin
repositoriesMode.set(RepositoriesMode.PREFER_SETTINGS)
```

`PREFER_SETTINGS` 模式下，Gradle **允许**项目级仓库声明但会发出 WARNING。然而 Capacitor 插件的 `build.gradle`（位于 `node_modules/` 下，由 `npx cap sync` 生成）声明了自己的 `repositories` 块（`google()`、`mavenCentral()`、`flatDir`），这些声明与 settings 级仓库冲突。

**关键问题**：错误日志中显示的是 `✖ Running Gradle build - failed!`，但 `PREFER_SETTINGS` 本身只产生 WARNING 不会导致构建失败。需要确认是否有其他隐藏错误（如 `FAIL_ON_PROJECT_REPOS` 被某处设置），或者这些 WARNING 被后续的 CI 脚本当作错误处理。

### 受影响的 Capacitor 插件

| 插件 | 路径 | 冲突仓库 |
|------|------|----------|
| `@capacitor/android` | `node_modules/@capacitor/android/capacitor/build.gradle` | Google, MavenRepo |
| `capacitor-cordova-android-plugins` | `capacitor-cordova-android-plugins/build.gradle` | Google, MavenRepo, flatDir |
| `@capacitor/screen-orientation` | `node_modules/@capacitor/screen-orientation/android/build.gradle` | Google, MavenRepo |
| `@capacitor/status-bar` | `node_modules/@capacitor/status-bar/android/build.gradle` | Google, MavenRepo |

### 解决方案

**方案 A（推荐）：将 `PREFER_SETTINGS` 改为 `PREFER_PROJECT`**

将 `settings.gradle.kts` 中的 `repositoriesMode` 从 `PREFER_SETTINGS` 改为 `PREFER_PROJECT`。这允许项目级仓库声明优先，同时保留 settings 级仓库作为后备。这是最简单的修复，且与 Capacitor 生态兼容。

```kotlin
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.PREFER_PROJECT)
    ...
}
```

**方案 B：在 post-cap-sync.mjs 中自动移除插件 build.gradle 的 repositories 块**

在 `post-cap-sync.mjs` 中添加逻辑，自动从 Capacitor 插件的 `build.gradle` 中移除 `repositories {}` 块（因为 settings.gradle.kts 已声明了所有需要的仓库）。但这需要修改 `node_modules` 中的文件，每次 `npm install` 后都要重新处理。

**方案 C：移除 dependencyResolutionManagement 中的 repositoriesMode 设置**

完全移除 `repositoriesMode` 行，让 Gradle 使用默认行为（允许项目级和 settings 级仓库共存）。

### 推荐方案

**方案 A**（`PREFER_PROJECT`），理由：
1. 最小改动，只改一行
2. 与 Capacitor 生态兼容（Capacitor 插件普遍声明自己的 repositories）
3. settings 级仓库仍作为后备，确保所有依赖都能解析
4. flatDir 在 settings 级已正确配置（指向 `capacitor-cordova-android-plugins/src/main/libs` 和 `app/libs`），项目级的 flatDir 声明不会造成问题

## 实施步骤

1. 修改 `settings.gradle.kts` 第 15 行：`PREFER_SETTINGS` → `PREFER_PROJECT`
2. 验证 `capacitor-cordova-android-plugins/build.gradle` 中的 flatDir 路径与 settings 级配置一致
3. 触发 CI 构建验证
