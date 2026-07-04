# 集成 Bugly SDK（免费版 · 崩溃上报）

## 技术选型

| 项目 | 选择 | 理由 |
|------|------|------|
| SDK | **Bugly 免费版** (`com.tencent.bugly:crashreport`) | 用户指定官方文档 v=1.0.0（经典版） |
| 版本 | `latest.release` | 自动解析最新版，当前 4.x |
| 初始化 API | `CrashReport.initCrashReport(context, appId, debugMode)` | 仅需 APP_ID，无需 APP_KEY |
| NDK 崩溃 | 内置支持（4.0.0+ 合并为单 AAR） | 项目有 Go/MPV/FFmpeg 等 native 库 |

## 执行步骤

### Step 1: 添加版本到 libs.versions.toml

**文件**: `app/encv-mobile/android/gradle/libs.versions.toml`

```diff
 [versions]
+bugly = "latest.release"

 [libraries]
+bugly-crashreport = { group = "com.tencent.bugly", name = "crashreport", version.ref = "bugly" }
```

### Step 2: 添加依赖 + BuildConfig 到 app/build.gradle.kts

**文件**: `app/encv-mobile/android/app/build.gradle.kts`

```diff
 dependencies {
     ...
+    implementation(libs.bugly.crashreport)
 }
```

在 `defaultConfig` 中添加 BuildConfig 字段（从 GitHub Secret 注入）：

```diff
 defaultConfig {
     applicationId = "com.encvgo.app"
     minSdk = ...
+    buildConfigField("String", "BUGLY_APP_ID", "\"${System.getenv("BUGLY_APP_ID") ?: ""}\"")
 }
```

> `buildConfig = true` 已在 L49 开启。

### Step 3: 创建 EncvApplication.kt 并初始化 Bugly

**文件**: `app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvApplication.kt`（**新建**）

Manifest 已声明 `android:name=".EncvApplication"` 但该类只存在于 `android-overlay/` 模块中。创建此文件同时：
- 修复潜在的 ClassNotFoundException 启动崩溃
- 提供最早的 Bugly 初始化入口（Application.onCreate 比 MainActivity 更早）

```kotlin
package com.encvgo.app

import android.app.Application
import com.tencent.bugly.crashreport.CrashReport
import android.util.Log

class EncvApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        initBugly()
    }

    private fun initBugly() {
        try {
            val appId = BuildConfig.BUGLY_APP_ID
            if (appId.isEmpty()) {
                Log.w("ENCV-Bugly", "BUGLY_APP_ID not configured, skipping")
                return
            }
            CrashReport.initCrashReport(applicationContext, appId, false)
            Log.i("ENCV-Bugly", "Bugly initialized: appId=$appId")
        } catch (e: Exception) {
            Log.e("ENCV-Bugly", "Failed to initialize Bugly", e)
        }
    }
}
```

### Step 4: 创建 proguard-rules.pro

**文件**: `app/encv-mobile/android/app/proguard-rules.pro`（**新建**，release 构建引用但文件不存在）

```
# Bugly 混淆保留
-dontwarn com.tencent.bugly.**
-keep public class com.tencent.bugly.**{*;}
```

### Step 5: 更新 CI Workflow 传递 Secret

**文件**: `.github/workflows/android.yml`

在构建步骤注入环境变量：

```yaml
      - name: Build ${{ inputs.version && 'Release' || 'Debug' }} APK
        env:
          BUGLY_APP_ID: ${{ secrets.BUGLY_APP_ID }}
        run: |
          cd app/encv-mobile
          ...
```

### Step 6: 清理日志文件

```bash
rm -rf /workspace/job_logs /workspace/job_logs.zip
```

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `android/gradle/libs.versions.toml` | 修改 | +bugly 版本 + crashreport 库声明 |
| `android/app/build.gradle.kts` | 修改 | +依赖 + `buildConfigField("BUGLY_APP_ID")` |
| `android/app/src/main/java/.../EncvApplication.kt` | **新建** | Application 子类 + Bugly 初始化 |
| `android/app/proguard-rules.pro` | **新建** | `-keep com.tencent.bugly.**` |
| `.github/workflows/android.yml` | 修改 | 构建步骤加 `env: BUGLY_APP_ID` |

## Secret 数据流

```
GitHub Secrets (BUGLY_APP_ID)
  → CI workflow env (secrets.BUGLY_APP_ID)
    → Gradle System.getenv("BUGLY_APP_ID")
      → buildConfigField → BuildConfig.BUGLY_APP_ID
        → EncvApplication.onCreate() → CrashReport.initCrashReport(ctx, appId, false)
```

## NDK Native Crash

项目 native 库清单：`libencv-go.so`(Go) / `libmpv.so`(MPV) / `libffmpeg.so` / `libffprobe.so`(FFmpeg)

Bugly 免费版 4.0.0+ 已内置 NDK 崩溃捕获（jar+so 合并为单一 AAR），无需额外添加 `nativecrashreport` 依赖。abiFilters 已设为 `arm64-v8a`，与所有 native 库一致。

## 权限检查

所需权限已在现有 Manifest 中声明：

| 权限 | 已有？ |
|------|--------|
| `INTERNET` | ✅ L3 |
| `ACCESS_NETWORK_STATE` | ❌ 需添加 |
| `ACCESS_WIFI_STATE` | ❌ 需添加 |

需在 `AndroidManifest.xml` 补充缺失权限。
