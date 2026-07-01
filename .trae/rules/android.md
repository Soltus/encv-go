# Android 构建系统规则

> **核心原则**：Gradle 仓库解析采用「短路求值」策略——源的可信度必须与排列优先级成正比（官方源优先，镜像源兜底）；AGP 强制 `isMinifyEnabled` 与 `isShrinkResources` 硬耦合。

> **完整内容 + 历史踩坑**：[详情文档](../rule-library/android.md)

---

## 一、依赖仓库顺序铁律（必读）

### 1.1 `pluginManagement` 仓库顺序

**核心原则：官方源优先，镜像源兜底。**

Gradle 解析 plugin 时按 `pluginManagement.repositories` 列表**顺序搜索**，找到第一个匹配即停止。阿里云镜像无法代理以下源的元数据格式：

| 源 | 阿里云能否代理 | 说明 |
|----|--------------|------|
| `google()` | ✅ 能 | Android 库（AndroidX 等） |
| `mavenCentral()` | ⚠️ 部分 | 标准库（kotlin-reflect 等），但版本可能滞后 |
| **`gradlePluginPortal()`** | **❌ 不能** | **Plugin Marker POM 格式不同** |
| **`plugins.gradle.org/m2/`** | N/A | Plugin Portal 的直接 Maven 仓库 |

**✅ 正确配置**（settings.gradle.kts）：
```kotlin
pluginManagement {
    repositories {
        mavenCentral()           // ① 标准 Maven Central
        google()                 // ② Android 官方
        gradlePluginPortal()     // ③ Gradle 插件门户（必须靠前！）
        maven { url = uri("https://plugins.gradle.org/m2/") }  // ④ 直接 URL fallback
        maven { url = uri("https://maven.aliyun.com/repository/google") }
        maven { url = uri("https://maven.aliyun.com/repository/central") }
        maven { url = uri("https://maven.aliyun.com/repository/gradle-plugin") }
        maven { url = uri("https://maven.aliyun.com/repository/public") }
        maven { url = uri("https://mirrors.tencent.com/nexus/repository/maven-tencent/") }
    }
}
```

**❌ 错误配置**：
```kotlin
pluginManagement {
    repositories {
        mavenCentral()
        maven { url = uri("https://maven.aliyun.com/repository/google") }  // 先搜镜像
        maven { url = uri("https://maven.aliyun.com/repository/gradle-plugin") }  // 无法代理 plugin portal
        // ... 更多镜像 ...
        google()              // ← 太晚！
        gradlePluginPortal()  // ← 最晚！镜像已耗尽超时预算
    }
}
```

### 1.2 `dependencyResolutionManagement` 仓库顺序

与 `pluginManagement` 类似，但额外注意加 `jitpack.io`：

```kotlin
dependencyResolutionManagement {
    repositories {
        google()                    // AndroidX / Google 库
        mavenCentral()              // Kotlin stdlib / kotlin-reflect / 第三方库
        maven { url = uri("https://jitpack.io") }  // ComboLite 等发布在 JitPack 的库
        maven { url = uri("https://maven.aliyun.com/repository/google") }
        if (System.getenv("CI") == null) {
            maven { url = uri("https://mirrors.tencent.com/nexus/repository/maven-public/") }
        }
        maven { url = uri("https://mirrors.tencent.com/repository/maven-tencent/") }
        maven { url = uri("https://maven.aliyun.com/repository/public") }
        mavenCentral()
    }
}
```

### 1.3 ComboLite 依赖坐标参考

| 依赖 | Group ID | Artifact ID | 版本 | 仓库 |
|------|----------|-------------|------|------|
| combolite-core | `io.github.lnzz123` | `combolite-core` | 2.0.2+ | Maven Central |
| aar2apk (Gradle Plugin) | `io.github.lnzz123.combolite-aar2apk` | (plugin id) | 1.1.1 | Gradle Plugin Portal (`plugins.gradle.org`) |

---

## 二、版本管理

| 依赖 | 版本 | 用途 |
|------|------|------|
| combolite (core) | 2.0.2 | ComboLite 核心 runtime |
| combolite-aar2apk | 1.1.1 | AAR→APK 转换 Gradle 插件 |

**升级注意事项**：
- **combolite-core 升级时必须同步检查**：新版本是否引入了新的 `::function.javaMethod` 使用点（需要 R8 保持禁用）
- **aar2apk 插件升级时必须检查**：是否引入了新的 buildType 或 DSL 变更
- **两者版本独立**：core 和 aar2apk 有独立的版本号体系，不需要保持一致

---

## 三、AGP 构建选项约束

### 3.1 `isMinifyEnabled` 与 `isShrinkResources` 硬耦合

**AGP（Android Gradle Plugin）在配置阶段强制检查：`isShrinkResources=true` 必须配合 `isMinifyEnabled=true`。**

**原因**：ResourceShrinker 的依赖图分析需要 R8/ProGuard 先生成完整的类→资源映射文件。

**对本项目的影响**：ComboLite 要求 `isMinifyEnabled=false`（R8 破坏 kotlin-reflect @Metadata），因此 `isShrinkResources` **也必须为 false**。两者无法独立配置。

| 配置 | isMinifyEnabled | isShrinkResources | 结果 |
|------|----------------|-------------------|------|
| A | `false` | `false` | ✅ 正常（本项目必须用此） |
| B | `true` | `true` | ❌ ComboLite 崩溃（@Metadata 被破坏） |
| C | `false` | `true` | ❌ AGP EvalException（CI 实测确认） |
| D | `true` | `false` | ⚠️ 技术可行但无意义（代码 shrink 不 shrink resource） |

> **错误 C**：「单独开启 isShrinkResources」→ 报 `EvalIssueException: Removing unused resources requires unused code shrinking to be turned on.` → **AGP 源码级硬约束，无法绕过**

---

## 四、常见错误模式

| 错误 | 症状 | 根因 | 修复 |
|------|------|------|------|
| **A** | `Plugin [id: 'xxx', version: 'x.y.z'] was not found` | `gradlePluginPortal` 放在阿里云镜像后 → 镜像先返回空/超时，gradlePluginPortal 来不及 fallback | 将 `google()` 和 `gradlePluginPortal()` 移到列表前 3 位 |
| **B** | ComboLite 找不到 | `dependencyResolutionManagement` 没加 `jitpack.io`（ComboLite 发在 JitPack） | 加 `maven { url = uri("https://jitpack.io") }` |
| **C** | EvalIssueException | `isShrinkResources=true` 但 `isMinifyEnabled=false` | 两者同时为 `false`（ComboLite 项目） |

---

## 五、SQLite 选型铁律（libsql / glebarez）

> **2026-07-01 更新：本项目已不再使用 gomobile。**
> 
> 当前架构：Go 后端以独立可执行文件（`libencv-go.so`）运行，通过 ProcessBuilder 启动，
> 不走 gomobile bind / JNI 桥接路径。

### 5.1 引擎对比

| 引擎 | 实现方式 | Android 支持 | 向量搜索 | 说明 |
|------|---------|------------|---------|------|
| **glebarez/sqlite** | 纯 Go transpile | ✅ 原生支持 | ❌ 不支持 | 轻量、零依赖 |
| **libsql** | CGO + 原生 .so | ✅ 需 NDK 编译 | ✅ 支持 | Turso 官方 fork，高性能 |
| **turso** | purego | ❌ 移动端不支持 | ✅ 支持 | 桌面端可用 |

### 5.2 铁律

1. **SHALL** 默认使用 `glebarez/sqlite`（纯 Go，零依赖）
2. **SHALL** 如需向量搜索，桌面端用 `turso`（purego），移动端用 `libsql`（CGO）
3. **SHALL NOT** 移动端使用 turso（purego 方案不兼容 Android）
4. **SHALL NOT** 使用 `mattn/go-sqlite3` / `gorm.io/driver/sqlite`（CGO 版本）

### 5.3 LibSQL Android 集成

LibSQL 通过 CGO 动态链接原生库，产物放在 `jniLibs/arm64-v8a/libsql_experimental.so`：
- 编译脚本：`scripts/build-libsql-android.sh`
- 驱动代码：`pkg/libsql/driver.go`
- 预编译库目录：`pkg/libsql/libs/android_arm64/`

> **历史背景（已废弃）**：gomobile bind 方案详见 [详情文档 §五](../rule-library/android.md#五gomobile--sqlite-选型铁律plugin-openlist-必读-历史)

---

## 六、引用其他规则

- [combolite.md](./combolite.md) — R8 禁用铁律源头、kotlin-reflect @Metadata 破坏机制
- [capacitor.md](./capacitor.md) — Capacitor 项目构建配置参考

> 拆分：2026-06-11
