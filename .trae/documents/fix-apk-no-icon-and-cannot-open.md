# 修复 APK 无 Icon 和无法打开问题

## 问题分析

从 CI 日志和源码分析，发现两个关键问题：

### 问题 1：AndroidManifest.xml 缺少 MAIN/LAUNCHER intent-filter（根本原因）

`android-overlay/app/src/main/AndroidManifest.xml` 中的 `<activity>` 声明**缺少**启动器 intent-filter：

```xml
<activity
    android:name=".MainActivity"
    android:exported="true"
    android:launchMode="singleTop">
    <!-- 只有 encvgo:// scheme 和 com.encvgo.action.* 的 intent-filter -->
    <!-- ❌ 缺少 MAIN/LAUNCHER intent-filter！ -->
</activity>
```

正常的 Capacitor 应用，Activity 应该包含：
```xml
<intent-filter>
    <action android:name="android.intent.action.MAIN" />
    <category android:name="android.intent.category.LAUNCHER" />
</intent-filter>
```

**这是"没有 icon"和"安装后无法打开"的根本原因**——Android 系统找不到 LAUNCHER Activity，所以不会在桌面创建图标，也无法从启动器打开 App。

### 问题 2：DEX 中 Kotlin 类未找到（次要问题）

CI 验证步骤显示 `❌ Not found in DEX strings`，虽然 `.class` 文件存在于 `kotlin-classes` 目录。这可能是因为：
- `strings` 命令搜索 DEX 的方式不够可靠（R8/DEX 优化可能压缩类名）
- 或者 Kotlin 代码确实没被正确打包到 DEX 中

但构建日志显示 `compileDebugKotlin` 成功执行，且 `.class` 文件存在，所以这更可能是验证脚本的问题而非真正的构建问题。**需要进一步确认**。

### 问题 3：capacitor.plugins.json 为空数组

`capacitor.plugins.json: []` — 这意味着 `GoProcessPlugin` 没有被 Capacitor 插件系统注册。但 MainActivity 中通过 `registerPlugin(GoProcessPlugin::class.java)` 手动注册了，所以这不影响功能。

## 修复计划

### Step 1：删除指定文件
- 删除 `/workspace/.trae/documents/job-logs.txt`
- 删除 `/workspace/.trae/documents/logcat.txt`
- 删除 `/workspace/job-logs.txt`

### Step 2：修复 AndroidManifest.xml — 添加 MAIN/LAUNCHER intent-filter

在 `android-overlay/app/src/main/AndroidManifest.xml` 的 `<activity>` 标签内，添加 MAIN/LAUNCHER intent-filter，使其成为可从桌面启动的应用。

修改后的 Activity 部分应为：
```xml
<activity
    android:name=".MainActivity"
    android:exported="true"
    android:launchMode="singleTop"
    android:theme="@style/AppTheme.NoActionBar"
    android:configChanges="orientation|keyboardHidden|keyboard|screenSize|locale|smallestScreenSize|screenLayout|uiMode">
    
    <intent-filter>
        <action android:name="android.intent.action.MAIN" />
        <category android:name="android.intent.category.LAUNCHER" />
    </intent-filter>
    
    <!-- 保留现有的 scheme 和 action intent-filters -->
    <intent-filter>
        <action android:name="android.intent.action.VIEW" />
        <category android:name="android.intent.category.DEFAULT" />
        <category android:name="android.intent.category.BROWSABLE" />
        <data android:scheme="encvgo" />
    </intent-filter>
    
    <intent-filter>
        <action android:name="com.encvgo.action.START" />
        <action android:name="com.encvgo.action.RESTART" />
        <action android:name="com.encvgo.action.STOP" />
        <action android:name="com.encvgo.action.STATUS" />
        <category android:name="android.intent.category.DEFAULT" />
    </intent-filter>
</activity>
```

### Step 3：验证
- 确认修改后的 AndroidManifest.xml 格式正确
- 确认 MAIN/LAUNCHER intent-filter 在第一个位置（Android 系统优先使用第一个 intent-filter 作为启动入口）
