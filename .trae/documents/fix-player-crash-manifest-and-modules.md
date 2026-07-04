# 修复播放器闪退问题

## 根因分析

经过深入审查所有关键源码文件，发现以下崩溃原因（按严重程度排序）：

### 🔴 P0：AndroidManifest 未注册 PlayerActivityLynx（最可能的闪退根因）

`PlayerActivity.onCreate()` 通过 `Intent(this, PlayerActivityLynx::class.java)` 启动 Lynx 播放器 Activity，但 **AndroidManifest.xml 中只注册了 `.PlayerActivity`，没有注册 `.PlayerActivityLynx` 和 `.PlayerActivityCapacitor`**。

Android 要求所有 Activity 必须在 Manifest 中声明，否则 `startActivity()` 会抛出 `ActivityNotFoundException` 导致应用崩溃。PlayerActivity.onCreate() 中没有任何 try-catch 保护。

**文件**：`android-overlay/app/src/main/AndroidManifest.xml`

### 🔴 P1：GoBackendModule 非安全类型强转

```kotlin
private val lynxContext = context as LynxContext  // 非安全！
```

如果传入的 context 不是 LynxContext（例如在某些生命周期场景下），会抛出 ClassCastException 导致崩溃。应改为 `context as? LynxContext`。

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/GoBackendModule.kt` 第 27 行

### 🟡 P2：MpvPlayerModule 获取 Activity 方式不正确

```kotlin
private val activity = lynxContext?.context as? Activity
```

`LynxContext` 继承自 `LynxBaseContext`，提供了专用的 `getActivity()` 方法返回 `Activity?`。应该使用 `lynxContext?.activity` 而不是 `lynxContext?.context as? Activity`，因为 `getContext()` 返回的是 base context，不一定是 Activity。

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt` 第 35 行

### 🟡 P3：Lynx CSS 不支持 `position: absolute`

Lynx 使用 Flexbox 布局系统，**不支持 `position: absolute`**。当前 CSS 中 `.LoadingIndicator` 和 `.ErrorOverlay` 使用了 `position: absolute`，会导致布局异常或渲染错误。

需要改用 Lynx 兼容的布局方式（Flexbox 居中、`display: relative` + `relative-center` 等）。

**文件**：`lynx-player/src/App.css`

### 🟡 P4：PlayerActivity 路由缺少 crash 防御

`PlayerActivity.onCreate()` 中 `startActivity(targetIntent)` 没有 try-catch，如果目标 Activity 未注册或其他原因导致启动失败，会直接崩溃。

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/PlayerActivity.kt`

### 🟢 P5：MPVLib.create() 传入 Application context

```kotlin
MPVLib.create(app)  // app = act.application
```

mpv-android 官方实现传入的是 Activity context。使用 Application context 可能导致 Surface 渲染问题。应改为传入 Activity context。

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt` 第 84 行

## 修复步骤

### Step 1：AndroidManifest 注册 PlayerActivityLynx 和 PlayerActivityCapacitor

在 `AndroidManifest.xml` 的 `<application>` 标签内，`.PlayerActivity` 声明之后添加：

```xml
<activity
    android:name=".PlayerActivityLynx"
    android:exported="false"
    android:launchMode="standard"
    android:documentLaunchMode="always"
    android:taskAffinity="com.encvgo.app.player.task"
    android:theme="@style/AppTheme.NoActionBar"
    android:configChanges="orientation|keyboardHidden|keyboard|screenSize|locale|smallestScreenSize|screenLayout|uiMode"
    android:label="ENCV Player" />

<activity
    android:name=".PlayerActivityCapacitor"
    android:exported="false"
    android:launchMode="standard"
    android:documentLaunchMode="always"
    android:taskAffinity="com.encvgo.app.player.task"
    android:theme="@style/AppTheme.NoActionBar"
    android:configChanges="orientation|keyboardHidden|keyboard|screenSize|locale|smallestScreenSize|screenLayout|uiMode"
    android:label="ENCV Player" />
```

### Step 2：修复 GoBackendModule 非安全强转

将 `context as LynxContext` 改为 `context as? LynxContext`，并在使用 `lynxContext` 的地方添加空安全处理：

```kotlin
private val lynxContext = context as? LynxContext
```

`dispatchReady` 和 `dispatchError` 方法中也需要添加 `lynxContext?.` 空安全调用。

### Step 3：修复 MpvPlayerModule Activity 获取方式

将：
```kotlin
private val activity = lynxContext?.context as? Activity
```
改为：
```kotlin
private val activity = lynxContext?.activity
```

### Step 4：修复 Lynx CSS 兼容性

将 `position: absolute` 的布局改为 Flexbox 兼容方式：

- `.LoadingIndicator`：移除 `position: absolute` + `transform`，改用父容器 Flexbox 居中
- `.ErrorOverlay`：移除 `position: absolute`，改用 Flexbox 全屏覆盖

具体方案：
- `.PlayerContainer` 改为 `display: flex; flex-direction: column; justify-content: center; align-items: center;`
- `.LoadingIndicator` 移除 `position: absolute` 和 `transform`
- `.ErrorOverlay` 移除 `position: absolute`，改用 `position: fixed` 或 Flexbox 全屏

### Step 5：PlayerActivity 路由添加 crash 防御

在 `PlayerActivity.onCreate()` 中为 `startActivity()` 添加 try-catch：

```kotlin
try {
    startActivity(targetIntent)
} catch (e: Exception) {
    Log.e(TAG, "Failed to start player activity", e)
    finish()
}
```

### Step 6：MPVLib.create() 使用 Activity context

将 `ensureMpvInitialized()` 中的：
```kotlin
val app = act.application
MPVLib.create(app)
```
改为：
```kotlin
MPVLib.create(act)
```

## 预期效果

- Step 1 修复后，PlayerActivityLynx 能正常启动，不再因 ActivityNotFoundException 闪退
- Step 2-3 修复后，Module 初始化不再因类型转换异常崩溃
- Step 4 修复后，Lynx UI 渲染正常，不会因不支持的 CSS 属性出错
- Step 5 修复后，即使 Activity 启动失败也不会导致应用崩溃
- Step 6 修复后，mpv 渲染上下文正确，Surface 能正常绑定
