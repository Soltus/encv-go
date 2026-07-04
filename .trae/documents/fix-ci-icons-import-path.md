# 修复 CI 构建：MpvControls.kt Icons Outlined 导入路径错误

## CI 错误分析（job_logs/build/23_*.txt）

### 环境
- Gradle 9.0.0（从腾讯云镜像下载）
- JDK 21（CI: `/opt/hostedtoolcache/Java_Temurin-Hotspot_jdk/21.0.11-10/x64`）
- Kotlin 2.3.21（build.gradle.kts root）
- AGP 8.13.0

### 编译错误（仅 4 个，全是 MpvControls.kt）

```
e: MpvControls.kt:32:55 Unresolved reference 'Subtitles'
e: MpvControls.kt:33:55 Unresolved reference 'MusicNote'
e: MpvControls.kt:267:50 Unresolved reference 'Subtitles'
e: MpvControls.kt:274:50 Unresolved reference 'MusicNote'
```

### 根因

**Import 路径写错了。** Material Icons Extended 的包结构是：

```
androidx.compose.material.icons/
├── filled/     → import androidx.compose.material.icons.filled.XXX    → 使用 Icons.Filled.XXX
├── outlined/   → import androidx.compose.material.icons.outlined.XXX  → 使用 Icons.Outlined.XXX  ← 小写 o！
├── rounded/
└── sharp/
```

当前代码（**错误**）：
```kotlin
import androidx.compose.material.icons.Icons.Outlined.Subtitles   // ❌ 不存在这个路径
import androidx.compose.material.icons.Icons.Outlined.MusicNote   // ❌
```

正确写法：
```kotlin
import androidx.compose.material.icons.outlined.Subtitles        // ✅ 小写 outlined
import androidx.compose.material.icons.outlined.MusicNote        // ✅
```

对比同文件中已正确工作的 Filled 图标导入：
```kotlin
import androidx.compose.material.icons.filled.PlayArrow          // ✅ 小写 filled → Icons.Filled.PlayArrow
import androidx.compose.material.icons.filled.VolumeUp           // ✅
```

### 次要问题：jvmTarget="25" vs CI 的 JDK 21

CI 运行在 JDK 21 上，但 build.gradle.kts 设了 `jvmTarget="25"` + `sourceCompatibility=VERSION_25`。
本次 CI 未报此错（可能 Kotlin 2.3.21 支持交叉编译到 JVM 25），但为保险应与 CI JDK 版本对齐。

---

## 修复步骤

### Step 1: 修复 MpvControls.kt 的 Outlined 图标 import（2 处）

**文件**: `app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvControls.kt`

**L32-33** — 将错误的 import 路径改为正确的小写 `outlined`：

```diff
- import androidx.compose.material.icons.Icons.Outlined.Subtitles
- import androidx.compose.material.icons.Icons.Outlined.MusicNote
+ import androidx.compose.material.icons.outlined.Subtitles
+ import androidx.compose.material.icons.outlined.MusicNote
```

L267 和 L274 的使用处 `Icons.Outlined.Subtitles` / `Icons.Outlined.MusicNote` **不需要改**——引用方式是对的，只是 import 路径错了。

### Step 2: 修复 compose-reference.md 中的错误文档

**文件**: `.trae/rules/compose-reference.md` L93-94

```diff
- import androidx.compose.material.icons.Icons.Outlined.Subtitles   // ✅ 正确
- import androidx.compose.material.icons.Icons.Outlined.MusicNote   // ✅ 正确
+ import androidx.compose.material.icons.outlined.Subtitles         // ✅ 正确
+ import androidx.compose.material.icons.outlined.MusicNote         // ✅ 正确
```

### Step 3: jvmTarget 与 CI JDK 对齐（可选，降低风险）

**文件**: `app/encv-mobile/plugin-mpv-player/build.gradle.kts`

CI 用 JDK 21，将 jvmTarget 从 25 改为 21：

```diff
  compileOptions {
-     sourceCompatibility = JavaVersion.VERSION_25
-     targetCompatibility = JavaVersion.VERSION_25
+     sourceCompatibility = JavaVersion.VERSION_21
+     targetCompatibility = JavaVersion.VERSION_21
  }

  kotlin {
      compilerOptions {
-         jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.fromTarget("25"))
+         jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.fromTarget("21"))
      }
  }
```

> 注：如果用户坚持要 Java 25 兼容，可保留 25（本次 CI 未因此报错）。

### Step 4: 清理日志文件

删除解压的日志目录和原始 zip：
```bash
rm -rf /workspace/job_logs /workspace/job_logs.zip
```

---

## 验证方式

修复后推送到 CI，检查 step 23 `compileReleaseKotlin` 是否通过。预期结果：
- `compileReleaseKotlin` 无 `e:` 错误
- BUILD SUCCESSFUL
