# Kotlin 编译彻底修复 Spec

## Why
本次会话反复出现 Kotlin 编译错误（val/var 混用、suspend 调用上下文、硬编码包名、泛型推断失败等），每次修复后仍有新错误，浪费 CI 时间。需要一次性彻底审计所有 .kt 文件，消除全部编译错误和警告。

## What Changes
- 审计 `:combolite-host` 模块所有 .kt 文件（6 个源文件）
- 审计 `:app` 模块所有新建/修改的 .kt 文件（7 个源文件）
- 修复所有违反 [kotlin-android.md](.trae/rules/kotlin-android.md) 规则的代码
- 消除所有硬编码项目包名调用
- 确保 suspend 函数调用正确使用 runBlocking 或 GlobalScope.launch
- 确保 val/var 使用正确
- 确保 import 完整且无冗余

## Impact
- Affected specs: `fix-goprocessplugin-kotlin-compilation`（未完成的 Task 3）
- Affected code:
  - `combolite-host/src/main/java/com/encvgo/combolite/**/*.kt`（6 文件）
  - `android/app/src/main/java/com/encvgo/app/*.kt`（7 文件）

## ADDED Requirements

### Requirement: Kotlin 编译零错误
所有 .kt 文件 SHALL 通过 Kotlin 2.3.21 编译，无任何错误。

#### Scenario: CI 构建
- **WHEN** CI 执行 `./gradlew assembleRelease`
- **THEN** `:combolite-host:compileReleaseKotlin` 和 `:app:compileReleaseKotlin` 任务成功完成，exit code 0

### Requirement: Kotlin 编译零警告（可选）
所有 .kt 文件 SHOULD 无编译警告（如 unchecked cast、deprecated API）。

#### Scenario: CI 构建
- **WHEN** CI 执行 `./gradlew assembleRelease`
- **THEN** 日志中无 `warning:` 关键字（或仅有已知不可消除的第三方库警告）

## MODIFIED Requirements

### Requirement: 禁止硬编码项目包名（强化）
代码中 **SHALL NOT** 出现任何 `com.encvgo.xxx` 全限定名作为调用目标，必须通过 import 引用。

### Requirement: Suspend 函数调用规范（强化）
所有 suspend 函数调用 **SHALL** 在以下三种上下文之一：
1. 另一个 suspend 函数内
2. `runBlocking { }` 内
3. `GlobalScope.launch(Dispatchers.IO) { }` 内

## REMOVED Requirements
无（本次为纯修复，不删除功能）