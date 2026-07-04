# 修复 CI 构建失败：Kotlin 版本不一致 + GoProcessPlugin 编译错误

## Why

CI 构建在 `:app:compileReleaseKotlin` 阶段失败，17 个编译错误。根因是 Kotlin 版本升级半截——`build.gradle.kts` 已升级到 2.3.21，但 `libs.versions.toml` 还停留在 2.1.0，导致编译器插件（2.3.21）与标准库（2.1.0）版本冲突。同时 `GoProcessPlugin.kt` 存在缺失 import、重复 companion object、nullable 安全调用等基础编译错误。

## 根因分析

### 洞察：半截升级是系统性问题

Kotlin 升级只改了 `build.gradle.kts` 的硬编码版本号，没有同步更新 `libs.versions.toml` 的版本目录。这不是偶然——Gradle 项目有**两个**版本声明点：

1. **根 `build.gradle.kts`**：`id("org.jetbrains.kotlin.android") version "2.3.21"` — 控制 Kotlin 编译器插件版本
2. **`libs.versions.toml`**：`kotlin = "2.1.0"` — 控制 `kotlin-stdlib`、`kotlin-test` 等库版本

编译器 2.3.21 + 标准库 2.1.0 = 运行时类型系统不一致。Kotlin 2.3.x 对 nullable 类型的推断更严格，`getString()` 返回 `String?` 的约束在 2.3.x 下更严格执行，导致之前"恰好能编译"的代码报错。

### CI vs 本地差异的真正原因

不是 IDE 自动 import 那么简单——**Kotlin 2.3.x 的类型推断更严格**。在 2.1.0 下，某些 nullable 类型可能被宽松处理；在 2.3.21 下，编译器严格执行 `String?` 约束，暴露了之前被掩盖的类型安全问题。

## What Changes

- 修复 `libs.versions.toml` 中 Kotlin 版本：`2.1.0` → `2.3.21`，与 `build.gradle.kts` 一致
- 修复 `GoProcessPlugin.kt` 编译错误：添加缺失 import、合并重复 companion object、修复 nullable 安全调用

## Impact

- Affected code: `libs.versions.toml`、`GoProcessPlugin.kt`
- Affected specs: 无

## ADDED Requirements

### Requirement: Kotlin 版本一致性

`libs.versions.toml` 中的 `kotlin` 版本 SHALL 与 `build.gradle.kts` 中的 Kotlin 插件版本一致。

#### Scenario: 版本目录与插件版本匹配
- **WHEN** `build.gradle.kts` 声明 `org.jetbrains.kotlin.android` 版本为 `2.3.21`
- **THEN** `libs.versions.toml` 中的 `kotlin` 版本 SHALL 也为 `2.3.21`

### Requirement: GoProcessPlugin.kt 编译通过

`GoProcessPlugin.kt` SHALL 通过 Kotlin 2.3.21 编译，无编译错误。

#### Scenario: import 完整性
- **WHEN** Kotlin 编译器处理 `GoProcessPlugin.kt`
- **THEN** 所有使用的类型（`BroadcastReceiver`、`Context`）SHALL 有对应的 import 声明

#### Scenario: 单一 companion object
- **WHEN** Kotlin 编译器处理 `GoProcessPlugin.kt`
- **THEN** 类中 SHALL 只有一个 `companion object`

#### Scenario: nullable 安全调用
- **WHEN** 对 `String?` 类型调用方法
- **THEN** SHALL 使用安全调用 `?.`、非空断言 `!!.` 或 `isNullOrEmpty()`

## MODIFIED Requirements

无

## REMOVED Requirements

无
