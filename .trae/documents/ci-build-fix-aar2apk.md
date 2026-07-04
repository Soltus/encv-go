# CI 构建修复计划 — aar2apk Gradle 插件解析失败

## 问题分析

CI 日志显示唯一错误：Gradle 配置阶段找不到 aar2apk 插件：

```
Could not find io.github.lnzz123.combolite-aar2apk:io.github.lnzz123.combolite-aar2apk.gradle.plugin:1.1.0.
Searched in:
  - https://dl.google.com/dl/android/maven2/...
  - https://repo.maven.apache.org/maven2/...
```

**根因**：aar2apk 插件发布在 **Gradle Plugin Portal**（`plugins.gradle.org`），而非 Google Maven 或 Maven Central。当前 `android/build.gradle` 的 `buildscript.repositories` 中只有 `google()` 和 `mavenCentral()`，缺少 `gradlePluginPortal()`。

Gradle Plugin Portal 页面确认：https://plugins.gradle.org/plugin/io.github.lnzz123.combolite-aar2apk/1.1.0
- 当前版本 1.1.0 可用，最新版本 1.1.1
- 正确的 `buildscript` 方式需要 `gradlePluginPortal()` 仓库

## 修复步骤

### 步骤 1：修改 `android/build.gradle`（根项目）

**文件**：`/workspace/app/encv-mobile/android/build.gradle`

1. 在 `buildscript.repositories` 中添加 `gradlePluginPortal()`
2. 升级 aar2apk 版本到 1.1.1（最新稳定版）

修改前：
```groovy
buildscript {
    repositories {
        google()
        mavenCentral()
    }
    dependencies {
        classpath "org.jetbrains.kotlin:kotlin-gradle-plugin:2.1.0"
        classpath 'com.android.tools.build:gradle:8.13.0'
        classpath 'com.google.gms:google-services:4.4.4'
        classpath "io.github.lnzz123.combolite-aar2apk:io.github.lnzz123.combolite-aar2apk.gradle.plugin:1.1.0"
    }
}
```

修改后：
```groovy
buildscript {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
    dependencies {
        classpath "org.jetbrains.kotlin:kotlin-gradle-plugin:2.1.0"
        classpath 'com.android.tools.build:gradle:8.13.0'
        classpath 'com.google.gms:google-services:4.4.4'
        classpath "io.github.lnzz123.combolite-aar2apk:io.github.lnzz123.combolite-aar2apk.gradle.plugin:1.1.1"
    }
}
```

### 步骤 2：删除 job_logs/ 和 job_logs.zip

按用户要求，修复完成后删除：
- `/workspace/job_logs/` 目录
- `/workspace/job_logs.zip` 文件

## 影响范围

- 仅修改 `android/build.gradle` 根项目文件
- 添加 `gradlePluginPortal()` 仓库 + 升级 aar2apk 版本
- 不涉及 Kotlin 代码、Go 代码或前端代码变更
- CI 工作流无需修改

## 验证

修复后 CI 应能：
1. 成功解析 aar2apk Gradle 插件
2. 成功编译 `:plugin-mpv-player:compileDebugKotlin`
3. 成功执行 `:plugin-mpv-player:buildDebugPluginApk`
4. 成功构建 Debug APK
