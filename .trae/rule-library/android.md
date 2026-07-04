# Android 构建系统规则（详情）

> **本文件为 [android.md](../rules/android.md) 的详情文档**。包含完整 §五 gomobile + sqlite 选型铁律（约 50 行对比表 + 验证命令 + 历史踩坑）以及更长版的「为什么」和「应急回退」段落。
>
> 索引文件保持精简，本文件按需加载。

---

## 一、依赖仓库顺序铁律

### 1.1 阿里云不能代理 Gradle Plugin Portal 的根因

Gradle Plugin Portal 使用特殊的 **Plugin Marker POM** 格式：

```
请求: io.github.lnzz123:combolite-aar2apk:1.1.1
  → Portal 返回 Marker POM:
    <groupId>io.github.lnzz123</groupId>
    <artifactId>combolite-aar2apk</artifactId>
    <version>1.1.1</version>
    → 其中 <dependencies> 指向实际插件:
      <groupId>io.github.lnzz123</groupId>
      <artifactId>combolite-aar2apk.gradle.plugin</artifactId>
      <version>1.1.1</version>

阿里云 gradle-plugin 镜像:
  → 只缓存标准 Maven 坐标 (group:artifact:version)
  → 不理解 Plugin Marker POM 的间接引用机制
  → 返回 404 或空结果
```

### 1.2 `pluginManagement` 仓库顺序

**核心原则：官方源优先，镜像源兜底。**

Gradle 解析 plugin 时按 `pluginManagement.repositories` 列表**顺序搜索**，找到第一个匹配即停止。阿里云镜像无法代理以下源的元数据格式：

| 源 | 阿里云能否代理 | 说明 |
|----|--------------|------|
| `google()` | ✅ 能 | Android 库（AndroidX 等） |
| `mavenCentral()` | ⚠️ 部分 | 标准库（kotlin-reflect 等），但版本可能滞后 |
| **`gradlePluginPortal()`** | **❌ 不能** | **Plugin Marker POM 格式不同** |
| **`plugins.gradle.org/m2/`** | N/A | Plugin Portal 的直接 Maven 仓库 |

**✅ 正确配置**：
```kotlin
// settings.gradle.kts
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
// gradlePluginPortal() 放在末尾 → 阿里云先返回空 → 可能超时
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

### 1.3 `dependencyResolutionManagement` 仓库顺序

与 `pluginManagement` 类似，但额外注意：

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

### 1.4 ComboLite 依赖坐标参考

| 依赖 | Group ID | Artifact ID | 版本 | 仓库 |
|------|----------|-------------|------|------|
| combolite-core | `io.github.lnzz123` | `combolite-core` | 2.0.2+ | Maven Central |
| aar2apk (Gradle Plugin) | `io.github.lnzz123.combolite-aar2apk` | (plugin id) | 1.1.1 | Gradle Plugin Portal (`plugins.gradle.org`) |

---

## 二、版本管理

### 2.1 当前依赖版本（libs.versions.toml）

| 依赖 | 版本 | 用途 |
|------|------|------|
| combolite (core) | 2.0.2 | ComboLite 核心 runtime |
| combolite-aar2apk | 1.1.1 | AAR→APK 转换 Gradle 插件 |

### 2.2 升级注意事项

- **combolite-core 升级时必须同步检查**：新版本是否引入了新的 `::function.javaMethod` 使用点（需要 R8 保持禁用）
- **aar2apk 插件升级时必须检查**：是否引入了新的 buildType 或 DSL 变更
- **两者版本独立**：core 和 aar2apk 有独立的版本号体系，不需要保持一致

---

## 三、AGP 构建选项约束

### 3.1 isMinifyEnabled 与 isShrinkResources 硬耦合

**AGP（Android Gradle Plugin）在配置阶段强制检查：`isShrinkResources=true` 必须配合 `isMinifyEnabled=true`。**

```kotlin
// AGP 源码: AndroidResourcesCreationConfigImpl.kt:91
if (!buildType.isMinifyEnabled && androidResources.shrink) {
    issueReporter.reportError(
        "Removing unused resources requires unused code shrinking to be turned on."
    )
}
```

**原因**：ResourceShrinker 的依赖图分析需要 R8/ProGuard 先生成完整的类→资源映射文件。

**对本项目的影响**：ComboLite 要求 `isMinifyEnabled=false`（R8 破坏 kotlin-reflect @Metadata），因此 `isShrinkResources` **也必须为 false**。两者无法独立配置。

| 配置 | isMinifyEnabled | isShrinkResources | 结果 |
|------|----------------|-------------------|------|
| A | `false` | `false` | ✅ 正常（本项目必须用此） |
| B | `true` | `true` | ❌ ComboLite 崩溃（@Metadata 被破坏） |
| C | `false` | `true` | ❌ AGP EvalException（CI 实测确认） |
| D | `true` | `false` | ⚠️ 技术可行但无意义（代码 shrink 不 shrink resource） |

### 错误 D：「单独开启 isShrinkResources」

> **症状**：`EvalIssueException: Removing unused resources requires unused code shrinking to be turned on.`
> **根因**：AGP 源码级硬约束，无法绕过
> **修复**：两者同时为 `false`（ComboLite 项目），或同时为 `true`（非 ComboLite 项目）

---

## 四、常见错误模式

### 错误 A：「pluginManagement 中 gradlePluginPortal 放在末尾」

> **症状**：CI 构建 `Plugin [id: 'xxx', version: 'x.y.z'] was not found in any of the following sources`
> **根因**：阿里云镜像先被搜索且返回空/超时，gradlePluginPortal 来不及 fallback
> **修复**：将 `google()` 和 `gradlePluginPortal()` 移到列表前 3 位

### 错误 B：「遗漏 jitpack.io」

> **症状**：某些第三方库（如 ComboLite）在 dependencyResolutionManagement 中找不到
> **根因**：ComboLite 发布在 JitPack 上，不在 Maven Central
> **修复**：`dependencyResolutionManagement.repositories` 中添加 `maven { url = uri("https://jitpack.io") }`

### 错误 C：「find path 排除 build.gradle.kts 文件」

> **症状**：CI guard 从未真正检查过构建配置文件
> **根因**：`find ... -not -path "*build*"` 把 `build.gradle.kts` 也排除了（文件名含 "build"）
> **修复**：使用 `-not -path "*/build/*"` （只排除目录）

---

## 五、SQLite / LibSQL 选型铁律

> **2026-07-01 更新：本项目已不再使用 gomobile。**
> 
> 当前架构：Go 后端以独立可执行文件（`libencv-go.so`）运行，通过 ProcessBuilder 启动，
> 不走 gomobile bind / JNI 桥接路径。
> 
> gomobile 历史文档见 [§六](#六gomobile--sqlite-选型铁律历史)。

### 5.1 引擎对比

| 引擎 | 实现方式 | Android 支持 | 向量搜索 | 说明 |
|------|---------|------------|---------|------|
| **glebarez/sqlite** | 纯 Go transpile | ✅ 原生支持 | ❌ 不支持 | 轻量、零依赖 |
| **libsql** | CGO + 原生 .so | ✅ 需 NDK 编译 | ✅ 支持 | Turso 官方 SQLite fork，高性能 |
| **turso** | purego | ❌ 移动端不支持 | ✅ 支持 | 桌面端可用 |

### 5.2 铁律

1. **SHALL** 默认使用 `glebarez/sqlite`（纯 Go，零依赖）
2. **SHALL** 如需向量搜索：桌面端用 `turso`（purego），移动端用 `libsql`（CGO）
3. **SHALL NOT** 移动端使用 turso（purego 方案不兼容 Android）
4. **SHALL NOT** 使用 `mattn/go-sqlite3` / `gorm.io/driver/sqlite`（CGO 版本）

### 5.3 LibSQL Android 集成

LibSQL 通过 CGO 动态链接原生库，产物放在 `jniLibs/arm64-v8a/libsql_experimental.so`。

**架构说明**：
- Go 后端是独立可执行 ELF（不是共享库），通过 ProcessBuilder 启动
- CGO 在 Go 二进制内动态链接 libsql_experimental.so
- .so 文件随 APK 打包在 jniLibs/ 目录，运行时从 nativeLibraryDir 加载

**关键文件**：
- 编译脚本：`scripts/build-libsql-android.sh`
- 驱动代码：`pkg/libsql/driver.go`
- 预编译库目录：`pkg/libsql/libs/android_arm64/`
- 存储实现：`pkg/tasksystem/store/libsql/libsql.go`

**CI 构建流程**：
1. 下载预编译的 libsql 原生库（或从源码编译）
2. 复制到 `jniLibs/arm64-v8a/`
3. Go 编译时设置 `CGO_ENABLED=1` + NDK clang
4. APK 打包时自动包含 jniLibs 里的 .so

### 5.4 Turso vs LibSQL 关系

> **重要：Turso 已脱离 libsql 成为独立的 SQLite fork 路线。**

| 维度 | LibSQL | Turso |
|------|--------|-------|
| 定位 | SQLite fork，开源免费 | 独立 SQLite fork，云服务 + 本地 |
| Android 支持 | ✅ 官方 libsql-android | ❌ 无官方移动端 SDK |
| Go 驱动 | CGO（自研） | purego（tursogo） |
| 向量搜索 | ✅ | ✅ |

---

## 六、gomobile + sqlite 选型铁律（历史）

> **⚠️ 已废弃：本项目不再使用 gomobile。保留此节供历史参考。**

### 5.1 为什么 mattn/go-sqlite3 在 gomobile 路径下是雷

`github.com/mattn/go-sqlite3` 是 **CGO 绑定驱动**——通过 `#cgo` 指令桥接 C 语言版的 `sqlite3.c`：

| 维度 | mattn/go-sqlite3（CGO） | glebarez/sqlite（pure-Go） |
|------|------------------------|---------------------------|
| 编译要求 | `CGO_ENABLED=1` + 主机 gcc/clang | `CGO_ENABLED=0` 也可 |
| 跨 ABI 稳定性 | 依赖目标平台 libc / NDK toolchain | 零系统依赖，arm64-v8a ELF 跨设备一致 |
| gomobile bind 表现 | 必须给 gomobile 配 NDK clang，否则 host gcc 产错 ELF；常见 `-fPIC` / `setresuid` / musl 报错 | 直接 `go build` 产出，零摩擦 |
| AAR 体积 | ~42 MB（带 SQLite C 静态库） | ~30 MB（纯 Go transpiled 字节码） |
| 写性能 | 100% 基准 | 70-80%（OpenList 元数据场景不可感知） |
| 与上游 OpenList API | 100% 兼容 | 100% 兼容（同 GORM Dialector 接口） |

### 5.2 铁律

> 任何走 `gomobile bind` 路径产出的 Go 代码（即 `libgojni.so`），如需 sqlite 持久化：
> 1. **SHALL** 导入 `github.com/glebarez/sqlite`
> 2. **SHALL NOT** 导入 `gorm.io/driver/sqlite`（其内部链入 mattn）
> 3. **SHALL NOT** 直接导入 `github.com/mattn/go-sqlite3`

> 非 gomobile 路径的普通 Go 二进制（encv-go 子进程、CLI 工具）目前未强制，但建议一致使用 `glebarez/sqlite` 以减少供应链碎片——见 `implement-mobile-backend-api/spec.md`「本地存储 sqlite 驱动 SHALL 使用 glebarez/sqlite」。

### 5.3 验证

```bash
# 检查 gomobile 路径下是否违规引入 mattn
cd fork && grep -rln '"github.com/mattn/go-sqlite3"\|"gorm.io/driver/sqlite"' . | head
# 应为空

# CGO_ENABLED=0 自检
cd fork && CGO_ENABLED=0 go build ./...
# 应通过（说明纯 Go）
```

### 5.4 应急回退（不应走到这一步）

若 fork 仍使用 mattn 且 gomobile 撞 NDK toolchain 兼容坑，`scripts/build-openlist-aar.sh` 内置 **B2 兜底**：

- 自动设 `CC=<NDK>/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android21-clang`
- 强制 `CGO_ENABLED=1`
- 但这只是「让 CI 跑过」，长期方案仍是切 glebarez（见 `.trae/documents/openlist-aar-sqlite-cgo-multi-solution.md` §三 B1）

### 5.5 历史踩坑

> **症状**：`gomobile bind` 报 `undefined: LogCallback`，补全后下一轮报 `# github.com/mattn/go-sqlite3` 或 `-fPIC` 失败
> **根因**：fork 用 `gorm.io/driver/sqlite` 链入 mattn CGO 库，gomobile 的 NDK toolchain 默认不开启 CGO 路径解析
> **修复**：fork 切 `glebarez/sqlite`（A1+B1 路径，spec 主推）；或脚本兜底强 CGO（A2+B2 路径，临时 CI 绿线）
