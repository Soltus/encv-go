# 修复计划：GoProcess 插件 UNIMPLEMENTED（Debug + Release 通用修复）

## 问题现状

**所有方法均返回 UNIMPLEMENTED**：`restart`、`stop`、`getStatus`、`requestNotificationPermission`、`requestStoragePermission`、`checkPermissions`

**关键信息**：debug 包也有此问题 → 排除 R8/ProGuard 原因（debug 默认 `minifyEnabled false`）

## 根因分析（多因素排查）

我们的实现与 [Capacitor 8 官方文档](https://capacitorjs.com/docs/android/custom-code) 完全一致，但仍然失败。需要系统排查以下可能原因：

### 可能原因 1：`kotlin-android` 插件未正确应用导致 Kotlin 文件未编译

`post-cap-sync.mjs` 通过字符串正则注入 `id 'kotlin-android'` 到 `plugins {}` 块。如果：
- 正则匹配失败（格式不兼容）
- 注入位置破坏了 DSL 语法
- 导致整个 `plugins {}` 块解析异常

结果：Gradle 静默忽略 kotlin-android 插件 → `.kt` 文件不被编译 → APK 中无 GoProcessPlugin.class

### 可能原因 2：`capacitor.plugins.json` 干扰自动注册

我们在 `post-cap-sync.mjs` 中向 `capacitor.plugins.json` 追加了 GoProcessPlugin 条目。在 `BridgeActivity.onStart()` 中：

```java
if (bridge == null) {
    PluginManager loader = new PluginManager(getAssets());
    try {
        bridgeBuilder.addPlugins(loader.loadPluginClasses()); // 从 JSON 加载
    } catch (PluginLoadException ex) { ... }
    this.load(); // 创建 bridge
}
```

如果 `Class.forName("com.encvgo.app.GoProcessPlugin")` 因任何原因抛出异常（被 catch 吞掉），虽然不会阻止构建，但可能影响插件注册表。

### 可能原因 3：缺少 `proguard-rules.pro`（影响 release 构建）

[encv-release.gradle](file:///workspace/app/encv-mobile/android-overlay/encv-release.gradle) 引用了不存在的 `proguard-rules.pro`。debug 不受影响，但 CI release 构建必定失败。

## 修复方案

### 修改 1：添加诊断日志到 MainActivity（定位问题）

在 `MainActivity.kt` 的 `onCreate()` 和 `startGoDaemon()` 中添加 Logcat 日志，确认：
- `registerPlugin()` 是否被执行
- 是否抛出异常
- bridge 初始化状态

```kotlin
override fun onCreate(savedInstanceState: android.os.Bundle?) {
    Log.d(TAG, "=== onCreate start ===")
    try {
        Log.d(TAG, "Registering GoProcessPlugin...")
        registerPlugin(GoProcessPlugin::class.java)
        Log.d(TAG, "GoProcessPlugin registered OK")
    } catch (e: Exception) {
        Log.e(TAG, "registerPlugin FAILED", e)
    }
    super.onCreate(savedInstanceState)
    Log.d(TAG, "=== onCreate end, starting daemon ===")
    startGoDaemon()
}
```

### 修改 2：创建 `proguard-rules.pro`（release 构建必需）

新建 `/workspace/app/encv-mobile/android-overlay/proguard-rules.pro`：

```proguard
# Keep all Capacitor plugins from being removed by R8
-keep public class * extends com.getcapacitor.Plugin { *; }

# Keep local classes
-keep class com.encvgo.app.** { *; }
```

### 修改 3：CI 工作流复制 proguard-rules.pro

在 `.github/workflows/android.yml` 的 "Apply Android overlay" 步骤中追加：

```yaml
cp app/encv-mobile/android-overlay/proguard-rules.pro \
   app/encv-mobile/android/app/proguard-rules.pro
```

### 修改 4：移除 `capacitor.plugins.json` 追加逻辑（消除干扰）

从 `post-cap-sync.mjs` 中移除步骤 "capacitor.plugins.json: register local GoProcessPlugin"。理由：
- `registerPlugin(GoProcessPlugin::class.java)` 已是官方推荐的手动注册方式
- `capacitor.plugins.json` 的自动注册机制可能与手动注册冲突
- 去掉后可排除此干扰因素

### 修改 5：验证生成的 build.gradle 正确性

在 CI 的 "Verify APK contents" 之前增加一步，检查生成的 `build.gradle` 关键内容：

```yaml
- name: Verify build.gradle
  run: |
    echo "=== kotlin-android plugin ==="
    grep -n "kotlin-android" app/encv-mobile/android/app/build.gradle || echo "MISSING: kotlin-android"
    echo "=== jvmTarget ==="
    grep -n "jvmTarget" app/encv-mobile/android/app/build.gradle || echo "MISSING: jvmTarget"
    echo "=== kotlin-stdlib ==="
    grep -n "kotlin-stdlib" app/encv-mobile/android/app/build.gradle || echo "MISSING: kotlin-stdlib"
```

## 修改文件清单

| 文件 | 操作 | 目的 |
|------|------|------|
| `android-overlay/.../MainActivity.kt` | 修改 | 添加诊断日志 |
| `android-overlay/proguard-rules.pro` | **新建** | R8 保留规则 |
| `.github/workflows/android.yml` | 修改 | 复制 proguard + 验证 build.gradle |
| `scripts/post-cap-sync.mjs` | 修改 | 移除 capacitor.plugins.json 追加 |

## 排查路径

1. 先应用修改 1（诊断日志）→ 用户重新构建 debug 包 → 查看 logcat 中 `ENCV-go` tag 的日志
2. 如果日志显示 `registerPlugin FAILED` → 修复 kotlin-android 插件注入
3. 如果日志显示 `registered OK` 但仍 UNIMPLEMENTED → 检查 bridge 初始化时序
4. 同时应用修改 2-4 → 确保 release 构建也正常
