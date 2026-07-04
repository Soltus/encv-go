# CI 构建修复计划 — packagePlugins 闭包内 FQCN 解析失败 + release 变体适配

## 问题分析

CI 错误（`app/build.gradle` line 65）：

```
Could not get unknown property 'io' for extension 'packagePlugins' of type com.combo.aar2apk.PackagePluginsExtension.
```

**根因**：`packagePlugins` 闭包内使用了 Java 完全限定类名 `io.github.combolite.core.build.PackageBuildType.DEBUG`，但 Groovy DSL 的闭包作用域中，`io` 被当作 `packagePlugins` 扩展的属性查找，而不是 Java 包名前缀。

**ComboLite 官方示例**（`app/build.gradle.kts`）的正确做法：
```kotlin
import com.combo.aar2apk.PackageBuildType  // 顶部 import

packagePlugins {
    buildType.set(PackageBuildType.RELEASE)  // 使用短名
}
```

注意：官方示例的 `PackageBuildType` 包名是 `com.combo.aar2apk.PackageBuildType`，不是我们之前写的 `io.github.combolite.core.build.PackageBuildType`。

### Release 变体适配

当前 `packagePlugins.buildType` 硬编码为 `DEBUG`，但 CI 有两种构建模式：
- **Debug 构建**：`./gradlew assembleDebug` → 插件应为 `PackageBuildType.DEBUG`
- **Release 构建**：`npx cap build android` → 插件应为 `PackageBuildType.RELEASE`

在 Groovy DSL 中，可以通过检查 Gradle 启动参数中的 task 名称来动态判断构建类型：
```groovy
def isReleaseBuild = gradle.startParameter.taskNames.any { it.toLowerCase().contains('release') }
buildType.set(isReleaseBuild ? PackageBuildType.RELEASE : PackageBuildType.DEBUG)
```

## 修复步骤

### 步骤 1：修改 `app/build.gradle`

**文件**：`/workspace/app/encv-mobile/android/app/build.gradle`

1. 在文件顶部添加 import 语句
2. 修改 `packagePlugins` 块，使用 import 后的短名 + 动态判断构建类型

修改前：
```groovy
plugins {
    id 'kotlin-android'
}

apply plugin: 'com.android.application'
apply plugin: 'io.github.lnzz123.combolite-aar2apk'

// ... android { ... } ...

packagePlugins {
    enabled.set(true)
    buildType.set(io.github.combolite.core.build.PackageBuildType.DEBUG)
    pluginsDir.set("plugins")
}
```

修改后：
```groovy
import com.combo.aar2apk.PackageBuildType

plugins {
    id 'kotlin-android'
}

apply plugin: 'com.android.application'
apply plugin: 'io.github.lnzz123.combolite-aar2apk'

// ... android { ... } ...

packagePlugins {
    enabled.set(true)
    def isReleaseBuild = gradle.startParameter.taskNames.any { it.toLowerCase().contains('release') }
    buildType.set(isReleaseBuild ? PackageBuildType.RELEASE : PackageBuildType.DEBUG)
    pluginsDir.set("plugins")
}
```

### 步骤 2：删除 job_logs/ 和 job_logs.zip

按用户要求，修复完成后删除：
- `/workspace/job_logs/` 目录
- `/workspace/job_logs.zip` 文件

## 影响范围

- 仅修改 `app/build.gradle` 文件
- 添加 import + 修改 buildType 引用方式 + 动态判断构建类型
- 不涉及 Kotlin/Go/前端代码变更
- CI 工作流无需修改

## 验证

修复后 CI 应能：
1. 成功解析 `packagePlugins` DSL 块（import 短名）
2. Debug 构建时插件打包为 debug 变体
3. Release 构建时插件打包为 release 变体
4. 成功编译 `:plugin-mpv-player:compileDebugKotlin`
5. 成功执行 `:plugin-mpv-player:buildDebugPluginApk`（或 release 对应任务）
6. 成功构建 Debug/Release APK
