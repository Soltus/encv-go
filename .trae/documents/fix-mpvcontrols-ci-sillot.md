# MpvControls.kt CI 修复 + 本地构建（Sillot-KMP 镜像方案）

## 一、JDK 环境

| 项目 | 结果 |
|------|------|
| Gradle 8.14.3 + JDK 25 | ❌ 不兼容 |
| Gradle 8.14.3 + JDK 21 | ✅ 已安装 (`mise install java@temurin-21`) |

## 二、本地构建：Sillot-KMP 镜像

来源：[Hi-Sillot/Sillot-KMP](https://github.com/Hi-Sillot/Sillot-KMP) `settings.gradle.kts`（已写入 `.trae/rules/project_rules.md`）

修改 [settings.gradle.kts](app/encv-mobile/android/settings.gradle.kts)：
- `pluginManagement.repositories`：在 `google()` 前插入阿里云+腾讯镜像
- `dependencyResolutionManagement.repositories`：同上

## 三、CI 错误修复（5 处改动，不动逻辑）

### 权威依据
1. Android 官方文档 — `by` 委托需 `getValue` + `setValue`
2. MpvPlayerScreen.kt（CI 通过）— `mutableStateOf<Float>` + `by` 有 `setValue` import
3. CI 日志 L176-178 — 精确错误位置

### 文件：[MpvControls.kt](app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvControls.kt)

| # | 行号 | 操作 |
|---|------|------|
| 1 | L32 | `...icons.outlined.Subtitles` → `...icons.Icons.Outlined.Subtitles` |
| 2 | L33 | `...icons.outlined.MusicNote` → `...icons.Icons.Outlined.MusicNote` |
| 3 | L44 后 | **新增** `import androidx.compose.runtime.setValue` |
| 4 | L266 | `Icons.outlined.Subtitles` → `Icons.Outlined.Subtitles` |
| 5 | L273 | `Icons.outlined.MusicNote` → `Icons.Outlined.MusicNote` |

## 四、执行顺序

1. 配置镜像（settings.gradle.kts）
2. JDK 21 + Gradle 本地编译
3. 修复 MpvControls.kt
4. 再次编译验证
5. 清理日志
