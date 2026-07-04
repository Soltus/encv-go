# 修复 CI 构建通过但应用启动闪退

## 问题确认

用户明确反馈：**combolite host app 之前能正常启动运行**，修改后启动即崩。

## 根因：Kotlin 版本被意外升级导致 host app 运行时崩溃

### 版本变更链路

```
libs.versions.toml L3:   kotlin = "2.1.0"          ← 原始正确版本
                         ↓
libs.versions.toml L51:  kotlin-android = { ..., version.ref = "kotlin" }  → 2.1.0
                         ↓
app/build.gradle.kts L6: id("org.jetbrains.kotlin.android")  ← alias → 2.1.0

但我改了 root build.gradle.kts：
build.gradle.kts L4:     id("...kotlin.android") version "2.3.21" apply false
                                                         ↑ 根声明会覆盖所有子模块！
```

**结果**：Host app 的 Kotlin 编译器从 2.1.0 → 2.3.21，stdlib 也跟着变了，可能导致运行时不兼容。

### 为什么编译通过但运行时崩？

- Kotlin 2.1.0 和 2.3.21 的 stdlib 有内部差异
- Compose BOM 2024.06.00 是针对 Kotlin 2.0-2.1 系列测试的
- 或者某些生成的字节码/元数据在 2.3.21 下有变化导致类加载失败

## 修复方案

### Step 1: 回退 root build.gradle.kts 的 Kotlin 版本

将 root 声明改回与 catalog 一致：

```diff
- id("org.jetbrains.kotlin.android") version "2.3.21" apply false
- id("org.jetbrains.kotlin.plugin.compose") version "2.3.21" apply false
+ id("org.jetbrains.kotlin.android") version "2.1.0" apply false
+ id("org.jetbrains.kotlin.plugin.compose") version "2.1.0" apply false
```

这样 host app 继续用 2.1.0（已验证能跑），plugin-mpv-player 也用 2.1.0（CI 已通过）。

### Step 2: 清理日志文件

```bash
rm -rf /workspace/job_logs /workspace/job_logs.zip
```

## 不需要改的（排除项）

- ~~EncvApplication 缺失~~ — 用户确认之前能正常运行，说明该类要么存在要么不需要
- ~~GoProcessPlugin `!!`~~ — 只在 JS 调用 openPlayer 时触发，不影响启动
- ~~jvmTarget 变更~~ — 仅影响 plugin 模块编译，不影响 host app 运行时
