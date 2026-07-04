# 修复计划：GoProcess 插件 UNIMPLEMENTED — 最终修复

## 根因分析

### 真正的根因：R8 代码混淆剥离了插件类

这是一个 **Capacitor 已知问题**（[capacitor-plugins#1024](https://github.com/ionic-team/capacitor-plugins/issues/1024)，[官方 Troubleshooting](https://capacitorjs.com/docs/v3/android/troubleshooting)）：

当 `build.gradle` 设置了 `minifyEnabled true` 时，**R8 会移除所有 `extends com.getcapacitor.Plugin` 的子类**，因为它们看起来像是"未被直接使用"的代码。

我们的 [encv-release.gradle](file:///workspace/app/encv-mobile/android-overlay/encv-release.gradle#L23-L26)：
```groovy
release {
    minifyEnabled true          // ← 开启 R8 混淆
    shrinkResources true        // ← 开启资源压缩
    proguardFiles getDefaultProguardFile('proguard-android-optimize.txt'), 'proguard-rules.pro'  // ← 引用了不存在的文件！
}
```

**`proguard-rules.pro` 文件在 android-overlay 目录中不存在！** R8 只使用了默认规则，没有保留任何 Capacitor 插件类。

### 为什么之前的修复都没用

| 尝试 | 原因 |
|------|------|
| JS 端 `registerPlugin` 格式修正 | JS 格式正确但原生端找不到插件类 |
| `GoProcessWeb extends WebPlugin` | Web fallback 不影响 Android 原生查找 |
| `capacitor.plugins.json` 追加 | JSON 注册成功，但 R8 把编译后的 .class 删了 |
| 去掉 `androidx.core` 依赖 | 解决了编译问题，但 R8 仍然剥离类 |
| `kotlin-android` 插件格式修正 | 解决了 Gradle 构建问题，但 R8 仍然剥离类 |

## 修复方案

### 1. 创建 `proguard-rules.pro`（核心修复）

在 `/workspace/app/encv-mobile/android-overlay/` 下创建 `proguard-rules.pro`：

```proguard
# Keep all Capacitor plugins from being removed by R8
-keep public class * extends com.getcapacitor.Plugin { *; }

# Keep our local GoProcessPlugin specifically
-keep class com.encvgo.app.GoProcessPlugin { *; }
-keep class com.encvgo.app.MainActivity { *; }
```

### 2. 更新 CI 工作流 — 复制 proguard-rules.pro

在 `.github/workflows/android.yml` 的 "Apply Android overlay" 步骤中添加复制 `proguard-rules.pro`：
```yaml
cp app/encv-mobile/android-overlay/proguard-rules.pro \
   app/encv-mobile/android/app/proguard-rules.pro
```

### 3. 在 MainActivity.onCreate() 中添加诊断日志（可选但推荐）

确认 `registerPlugin()` 是否真的被执行：
```kotlin
override fun onCreate(savedInstanceState: android.os.Bundle?) {
    Log.d(TAG, "Registering GoProcessPlugin...")
    try {
        registerPlugin(GoProcessPlugin::class.java)
        Log.d(TAG, "GoProcessPlugin registered successfully")
    } catch (e: Exception) {
        Log.e(TAG, "Failed to register GoProcessPlugin", e)
    }
    super.onCreate(savedInstanceState)
    startGoDaemon()
}
```

## 修改文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `android-overlay/proguard-rules.pro` | **新建** | R8 保留规则，防止插件类被剥离 |
| `.github/workflows/android.yml` | **修改** | 复制 proguard-rules.pro 到构建目录 |
| `android-overlay/.../MainActivity.kt` | **可选修改** | 添加诊断日志 |

## 验证方式

1. 重新触发 CI 构建
2. 检查 release APK 中包含 `com/encvgo/app/GoProcessPlugin.class`
3. 安装后调用 GoProcess 各方法不再返回 UNIMPLEMENTED
