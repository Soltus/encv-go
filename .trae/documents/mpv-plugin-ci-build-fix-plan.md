# MPV 插件 CI 构建失败修复计划

## 问题诊断

### 错误 1（致命）：MpvPluginEntry.kt L23 — Null 赋值给非空类型

**CI 日志原文**：
```
e: file:///.../MpvPluginEntry.kt:23:22 Null cannot be a value of a non-null type 'MpvEngine'.
```

**根因分析**：
- [MpvPluginEntry.kt](../../app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPluginEntry.kt#L17-L26) 的 `Content()` 方法中调用 `MpvPlayerScreen(engine=null, ...)`
- [MpvPlayerScreen.kt](../../app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerScreen.kt#L38-L44) 的 `engine` 参数类型为 `MpvEngine`（非空）
- [MpvEngine.kt](../../app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvEngine.kt#L20) 构造函数：`class MpvEngine(private val context: Context)`，需要 Context 实例

**修复方案**：
在 `Content()` 中通过 `LocalContext.current` 获取 Android Context，创建真实的 `MpvEngine` 实例。这是 Compose 标准做法（参照本项目金标准文件 MpvPlayerScreen.kt L56 已使用 LocalContext）。

修改后代码逻辑：
```kotlin
@Composable
override fun Content() {
    val context = LocalContext.current
    val engine = remember { MpvEngine(context) }
    MpvPlayerScreen(
        filePath = "",
        fileName = "",
        mimeType = "",
        isExternal = false,
        engine = engine,
        onBack = {}
    )
}
```

需要新增 import：
- `androidx.compose.runtime.remember`
- `androidx.compose.ui.platform.LocalContext`

### 错误 2（警告）：AndroidManifest.xml package 属性已弃用

**CI 日志原文**：
```
package="com.encvgo.plugin.mpv" found in source AndroidManifest.xml
Setting the namespace via the package attribute in the source AndroidManifest.xml is no longer supported
Recommendation: remove package="com.encvgo.plugin.mpv" from the source AndroidManifest.xml
```

**根因分析**：
AGP 8.0+ 要求 namespace 通过 `build.gradle.kts` 的 `android.namespace` 声明，而非 Manifest 的 `package` 属性。
当前 [build.gradle.kts](../../app/encv-mobile/plugin-mpv-player/build.gradle.kts#L9) 已正确设置 `namespace = "com.encvgo.plugin.mpv"`。
但 [AndroidManifest.xml](../../app/encv-mobile/plugin-mpv-player/src/main/AndroidManifest.xml#L3) 仍保留 `package="com.encvgo.plugin.mpv"`。

**修复方案**：
从 AndroidManifest.xml `<manifest>` 标签中删除 `package`、`android:versionCode`、`android:versionName` 属性。这些属性在 AGP 8+ 中由 build.gradle.kts 管理。

修改后的 AndroidManifest.xml 只保留：
```xml
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application ...>
```

> ⚠️ 注意：Combolite 框架的 `InstallerManager.validateAndParseConfig()` 需要 Manifest 中有 `package` 作为 pluginId。需要确认移除后 Combolite 是否仍能从 build.gradle.kts namespace 正确获取。根据 AGP 8+ 行为，namespace 会自动注入最终 Manifest 的 package 字段，所以运行时不受影响。

---

## 修复步骤

### 步骤 1：修复 MpvPluginEntry.kt

**文件**：`app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPluginEntry.kt`

**操作**：
1. 新增 import：`androidx.compose.runtime.remember`、`androidx.compose.ui.platform.LocalContext`
2. 重写 `Content()` 方法体：
   - 使用 `val context = LocalContext.current` 获取 Context
   - 使用 `val engine = remember { MpvEngine(context) }` 创建引擎实例
   - 将 `engine = null` 改为 `engine = engine`

### 步骤 2：修复 AndroidManifest.xml

**文件**：`app/encv-mobile/plugin-mpv-player/src/main/AndroidManifest.xml`

**操作**：
1. 从 `<manifest>` 标签删除 `package="com.encvgo.plugin.mpv"`
2. 删除 `android:versionCode="1"`
3. 删除 `android:versionName="1.0"`

### 步骤 3：清理日志文件

按用户要求，修复完成后：
1. 删除 `/workspace/job_logs/` 目录（解压的日志）
2. 删除 `/workspace/job_logs.zip`（原始压缩包）

---

## 验证方式

修复后应满足：
1. Kotlin 编译无错误（`compileDebugKotlin` 通过）
2. AGP package 弃用警告消除
3. ndk-build libplayer.so 构建保持正常（此前已通过）
