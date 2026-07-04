# 评估：项目 Gradle Groovy → Kotlin DSL 迁移可行性

## 结论：可行，推荐渐进式迁移

**评分：4.2/5** — 收益明确，风险可控，Capacitor 不构成阻碍。

---

## 1. 核心问题：Capacitor 是否支持 KTS？

### 结论：支持（混合模式）

Capacitor **不会阻止**项目使用 Kotlin DSL，原因如下：

1. **Gradle 原生支持混合 DSL**：同一项目中 `.gradle` 和 `.gradle.kts` 可以共存。这是 Gradle 官方明确支持的特性。

2. **Capacitor 生成的文件全部是 Groovy DSL**，且标注 `DO NOT EDIT`：
   - `capacitor.settings.gradle` — 每次 `capacitor update` 重新生成
   - `app/capacitor.build.gradle` — 每次 `capacitor update` 重新生成
   - `capacitor-cordova-android-plugins/build.gradle` — Cordova 插件构建
   - `capacitor-cordova-android-plugins/cordova.variables.gradle` — Cordova 变量
   - `node_modules/@capacitor/android/capacitor/build.gradle` — Capacitor 核心库

3. **Capacitor 8.x（当前项目使用 ^8.3.4）** 的 Android 模板仍然生成 Groovy DSL 文件，官方尚未迁移到 KTS。

4. **关键兼容点**：`settings.gradle` 中 `apply from: 'capacitor.settings.gradle'` — Groovy 的 `apply from` 可以在 KTS 中通过 `apply(from = "capacitor.settings.gradle")` 调用，**反向也兼容**。

### Capacitor 不支持的风险点

| 风险 | 严重度 | 说明 |
|------|--------|------|
| `capacitor update` 覆盖自定义修改 | 🟡 中 | Capacitor 会重新生成 `capacitor.build.gradle` 和 `capacitor.settings.gradle`，但这些文件本来就不应手动修改 |
| `capacitor add android` 重新生成项目 | 🟡 中 | 如果删除 `android/` 重新 `cap add android`，会生成 Groovy 模板；但正常开发不需要此操作 |
| Capacitor 未来版本切换 KTS | 🟢 低 | 即使 Capacitor 未来生成 KTS 文件，与现有 KTS 文件完全兼容 |

---

## 2. 项目当前 Gradle 文件清单

### 自维护文件（应迁移到 KTS）

| 文件 | 类型 | 复杂度 | 迁移优先级 |
|------|------|--------|-----------|
| `android/build.gradle` | 根项目 | 中（aar2apk + buildscript） | 🔴 高 |
| `android/app/build.gradle` | 应用模块 | 高（packagePlugins + Compose + 多依赖） | 🔴 高 |
| `android/settings.gradle` | 项目设置 | 低（include + apply from） | 🟡 中 |
| `android/variables.gradle` | 变量定义 | 低（ext 块） | 🟡 中 |
| `plugin-mpv-player/build.gradle.kts` | 插件模块 | **已是 KTS** ✅ | — |

### Capacitor 生成文件（保持 Groovy，不迁移）

| 文件 | 说明 |
|------|------|
| `android/capacitor.settings.gradle` | 自动生成，`capacitor update` 会覆盖 |
| `android/app/capacitor.build.gradle` | 自动生成，`capacitor update` 会覆盖 |
| `android/capacitor-cordova-android-plugins/build.gradle` | Cordova 插件 |
| `android/capacitor-cordova-android-plugins/cordova.variables.gradle` | Cordova 变量 |
| `node_modules/@capacitor/android/capacitor/build.gradle` | Capacitor 核心 |

---

## 3. 迁移收益

### 3.1 解决当前痛点（直接收益）

**本次 CI 构建错误就是 Groovy DSL 的典型问题**：
- `packagePlugins` 闭包内 FQCN `io.github.combolite.core.build.PackageBuildType.DEBUG` 无法解析
- 需要手动 `import com.combo.aar2apk.PackageBuildType`
- Groovy 的动态特性导致编译期无法发现此类错误

**KTS 中这些问题不存在**：
```kotlin
// KTS 中可以直接使用 FQCN 或 import，IDE 会实时提示
import com.combo.aar2apk.PackageBuildType

packagePlugins {
    buildType.set(PackageBuildType.DEBUG)  // IDE 自动补全，编译期检查
}
```

### 3.2 长期收益

| 收益 | 说明 |
|------|------|
| **类型安全** | 编译期发现错误，而非运行时 |
| **IDE 支持** | 代码补全、跳转定义、重构、内联文档 |
| **ComboLite 官方示例全部使用 KTS** | 复制粘贴无需转换 |
| **AGP 新特性优先支持 KTS** | Android Gradle Plugin 新 API 优先在 KTS 中提供 |
| **Version Catalog 集成** | `libs.plugins.combolite.aar2apk` 类型安全引用 |
| **统一语言栈** | Kotlin 代码 + Kotlin 构建脚本，减少上下文切换 |

---

## 4. 迁移方案

### 方案 A：渐进式迁移（推荐）

分步迁移，每步可独立验证：

#### 第 1 步：`settings.gradle` → `settings.gradle.kts`

最简单的文件，风险最低：

```kotlin
// settings.gradle.kts
include(":app")
include(":capacitor-cordova-android-plugins")
include(":plugin-mpv-player")

project(":capacitor-cordova-android-plugins").projectDir = file("./capacitor-cordova-android-plugins/")

apply(from = "capacitor.settings.gradle")
```

#### 第 2 步：`variables.gradle` → 集成到 Version Catalog

将 `ext { ... }` 变量迁移到 `gradle/libs.versions.toml`：

```toml
[versions]
minSdk = "24"
compileSdk = "36"
targetSdk = "36"
androidxActivity = "1.11.0"
androidxAppCompat = "1.7.1"
# ...
```

#### 第 3 步：`build.gradle`（根）→ `build.gradle.kts`

```kotlin
plugins {
    id("io.github.lnzz123.combolite-aar2apk") version "1.1.1"
}

aar2apk {
    modules {
        module(":plugin-mpv-player")
    }
}
```

注意：KTS 中 aar2apk 插件通过 `plugins {}` 块应用，不再需要 `buildscript { classpath ... }`。

#### 第 4 步：`app/build.gradle` → `app/build.gradle.kts`

最复杂的文件，需要仔细转换：

```kotlin
import com.combo.aar2apk.PackageBuildType

plugins {
    id("com.android.application")
    id("kotlin-android")
    id("io.github.lnzz123.combolite-aar2apk")
}

android {
    namespace = "com.encvgo.app"
    compileSdk = 36
    // ...
}

packagePlugins {
    enabled.set(true)
    val isReleaseBuild = gradle.startParameter.taskNames.any { it.lowercase().contains("release") }
    buildType.set(if (isReleaseBuild) PackageBuildType.RELEASE else PackageBuildType.DEBUG)
    pluginsDir.set("plugins")
}

dependencies {
    implementation(libs.combolite.core)
    // ...
}
```

### 方案 B：一次性全量迁移

风险较高，不推荐。所有 `.gradle` 文件同时改为 `.gradle.kts`，调试困难。

---

## 5. 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| Capacitor `apply from` 在 KTS 中不兼容 | 低 | 高 | 已验证 `apply(from = "...")` 语法兼容 Groovy 文件 |
| `capacitor update` 覆盖 settings | 中 | 中 | `capacitor.settings.gradle` 是独立文件，`apply from` 不受影响 |
| KTS 语法转换错误 | 中 | 低 | 渐进式迁移，每步验证构建 |
| `variables.gradle` 的 `ext {}` 在 KTS 中不可用 | 确定 | 低 | 迁移到 Version Catalog（`libs.versions.toml`） |
| CI 环境兼容性 | 低 | 高 | CI 使用相同 Gradle wrapper，KTS 无额外依赖 |

---

## 6. 建议执行顺序

如果决定迁移，建议按以下顺序执行（每步完成后提交并验证 CI）：

1. ✅ `plugin-mpv-player/build.gradle.kts` — 已完成
2. `settings.gradle` → `settings.gradle.kts`
3. 创建 `gradle/libs.versions.toml` Version Catalog
4. `build.gradle`（根）→ `build.gradle.kts`
5. `app/build.gradle` → `app/build.gradle.kts`
6. 删除 `variables.gradle`（变量已迁移到 Version Catalog）

---

## 7. 不迁移的替代方案

如果决定暂不迁移，当前 Groovy DSL 的问题可以通过以下方式缓解：

- 在 Groovy 文件顶部添加 `import` 语句（本次修复方案）
- 使用 `@SuppressWarnings` 抑制警告
- 在 CI 中增加 Gradle 语法检查

**但这些只是权宜之计**，根本解决方案是迁移到 KTS。
