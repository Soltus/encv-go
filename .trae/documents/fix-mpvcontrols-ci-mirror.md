# MpvControls.kt CI 修复 + 本地构建（含国内镜像）

## 一、JDK 25 评估

| 项目 | 结果 |
|------|------|
| Gradle 8.14.3 + JDK 25 | ❌ 不兼容 |
| Gradle 8.14.3 + JDK 21 | ✅ 已安装 (`mise install java@temurin-21`) |

## 二、本地构建：配置阿里云镜像

### 原因
沙箱无法直接访问 `maven.google.com`、`repo.maven.org`、`plugins.gradle.org`

### 方案：settings.gradle.kts 添加阿里云镜像（参考 Sillot-KMP / 国内通用配置）

修改 [settings.gradle.kts](app/encv-mobile/android/settings.gradle.kts)：

**pluginManagement.repositories** 中在 `google()` 和 `mavenCentral()` **之前**插入：
```kotlin
maven { url = uri("https://maven.aliyun.com/repository/google") }
maven { url = uri("https://maven.aliyun.com/repository/public") }
maven { url = uri("https://maven.aliyun.com/repository/gradle-plugin") }
```

**dependencyResolutionManagement.repositories** 中同样在 `google()` 和 `mavenCentral()` **之前**插入：
```kotlin
maven { url = uri("https://maven.aliyun.com/repository/google") }
maven { url = uri("https://maven.aliyun.com/repository/public") }
```

同时创建全局 `~/.gradle/init.gradle` 作为兜底。

### 构建命令
```bash
export JAVA_HOME=/root/.local/share/mise/installs/java/21.0.2
cd /workspace/app/encv-mobile/android
gradle :plugin-mpv-player:compileReleaseKotlin --no-daemon
```

## 三、CI 错误修复（3 处，5 行改动）

### 权威依据
- Android 官方文档 developer.android.com/develop/ui/compose/state
- MpvPlayerScreen.kt（本项目 CI 通过）
- MpvProgressBar.kt（本项目 CI 通过）

### 修改文件：[MpvControls.kt](app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvControls.kt)

| # | 行号 | 操作 |
|---|------|------|
| 1 | L32 | `...icons.outlined.Subtitles` → `...icons.Icons.Outlined.Subtitles` |
| 2 | L33 | `...icons.outlined.MusicNote` → `...icons.Icons.Outlined.MusicNote` |
| 3 | L44 后 | **新增** `import androidx.compose.runtime.setValue` |
| 4 | L266 | `Icons.outlined.Subtitles` → `Icons.Outlined.Subtitles` |
| 5 | L273 | `Icons.outlined.MusicNote` → `Icons.Outlined.MusicNote` |

## 四、执行顺序

1. 配置阿里云镜像（settings.gradle.kts + init.gradle）
2. 运行本地 Gradle 编译验证
3. 修复 MpvControls.kt 的 5 处改动
4. 再次运行编译确认通过
5. 清理日志和临时文件
