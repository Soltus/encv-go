# MpvControls.kt CI 修复 + 本地构建环境

## 一、JDK 25 评估结论

| 项目 | 结果 |
|------|------|
| Gradle 8.14.3 + JDK 25 | ❌ 不兼容（报错仅输出 "25.0.2"） |
| Gradle 8.14.3 + JDK 21 | ✅ 兼容（已通过 mise 安装 `java@21.0.2`） |
| build.gradle.kts 要求 | `sourceCompatibility = VERSION_21`, `jvmTarget = "21"` |

**结论：使用 JDK 21 运行构建。** 已执行 `mise install java@temurin-21`，mise.toml 已更新为 `java@21.0.2`。

## 二、本地构建现状

Gradle 启动正常（JDK 21），但依赖解析阶段失败：
```
Plugin [id: 'org.jetbrains.kotlin.android', version: '2.1.0'] was not found
Searched: Google, MavenRepo, Gradle Central Plugin Repository
```

原因：沙箱网络无法访问 Google Maven（`maven.google.com` 连接被拒）、GitHub（需认证）。**本地完整构建暂时不可行。**

## 三、CI 错误修复（基于权威来源）

### 权威依据（3 个独立来源交叉验证）

| 来源 | 验证内容 |
|------|---------|
| **Android 官方文档** developer.android.com/develop/ui/compose/state | `by` 委托必须同时 import `getValue` + `setValue` |
| **MpvPlayerScreen.kt**（本项目已通过 CI） | L46-L53: `mutableStateOf<PlayerState/Boolean/Float>` + `by` 全部通过；有 `setValue` import |
| **MpvProgressBar.kt**（本项目已通过 CI） | L44: `mutableFloatStateOf` + `by` 通过 |
| **CI 日志 L176-178** | 精确的 3 个错误位置和消息 |

### 3 个错误及修复

#### 错误 1&2：`Icons.outlined` 大小写（L266, L273）
- **CI 输出**：`Unresolved reference 'outlined'`
- **根因**：material-icons-extended 的 Outlined 图标对象路径是 `Icons.Outlined`（**大写 O**），不是小写 `outlined`
- **修复**：
  - L32 import: `androidx.compose.material.icons.outlined.Subtitles` → `androidx.compose.material.icons.Icons.Outlined.Subtitles`
  - L33 import: `androidx.compose.material.icons.outlined.MusicNote` → `androidx.compose.material.icons.Icons.Outlined.MusicNote`
  - L266 引用: `Icons.outlined.Subtitles` → `Icons.Outlined.Subtitles`
  - L273 引用: `Icons.outlined.MusicNote` → `Icons.Outlined.MusicNote`

#### 错误 3：`MutableState<Float> has no method 'setValue'`（L303）
- **CI 输出**：`Type 'MutableState<Float>' has no method 'setValue(Nothing?, KMutableProperty0<*>, Float)', so it cannot serve as a delegate for var`
- **根因**：缺少 `import androidx.compose.runtime.setValue`
- **官方文档原文**："`by` 委托语法需要以下导入：`import androidx.compose.runtime.getValue` / `import androidx.compose.runtime.setValue`"
- **证据**：MpvPlayerScreen.kt 有 `setValue` import 且 `var volume by remember { mutableStateOf(1f) }` 编译通过
- **修复**：在 L44（`import androidx.compose.runtime.mutableStateOf`）之后添加一行：
  ```kotlin
  import androidx.compose.runtime.setValue
  ```
  **不需要修改 VolumeSliderRow 的任何逻辑代码。**

### 修改文件清单

**唯一需要修改的文件**：[MpvControls.kt](app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvControls.kt)

| 行号 | 操作 |
|------|------|
| L32 | import 改为 `...icons.Icons.Outlined.Subtitles` |
| L33 | import 改为 `...icons.Icons.Outlined.MusicNote` |
| L44 后 | **新增** `import androidx.compose.runtime.setValue` |
| L266 | `Icons.outlined.Subtitles` → `Icons.Outlined.Subtitles` |
| L273 | `Icons.outlined.MusicNote` → `Icons.Outlined.MusicNote` |

共 **5 处改动**（1 新增 + 4 修改），不动任何业务逻辑。

## 四、持久化措施

已创建 [compose-reference.md](.trae/rules/compose-reference.md)，包含：
- Android 官方 State 文档关键摘录（`by` 委托完整规则）
- Material Icons Extended 包结构（`Icons.Outlined` 大写 O）
- Material3 组件 Import 速查表
- Runtime 完整 Import 清单
- 本项目"金标准"文件索引（已通过 CI 的 MpvPlayerScreen.kt、MpvProgressBar.kt）

后续每次编写 Compose 代码前必须参照此文件。

## 五、清理

完成后删除：
- `/workspace/job_logs_extracted/`
- `/workspace/job_logs.zip`
- `/tmp/androidx/`（空壳 sparse checkout）
