# 修复 CI Kotlin 编译错误

## 根本原因

CI 构建失败是因为 Kotlin 编译错误。本地代码已修复，但需要确认所有修复正确且完整。

## 当前代码问题分析

### 1. EncvApplication.kt — LynxDevToolService 注册方式

**当前代码**: `LynxServiceCenter.inst().registerService(LynxDevToolService())`

**问题**: `LynxDevToolService()` 调用类构造函数，虽然理论上可行，但 Lynx 官方示例使用 `LynxDevToolService.getINSTANCE()`（Java）或 `LynxDevToolService.INSTANCE`（Kotlin）。使用官方推荐方式更安全，避免 Kotlin 编译器泛型推断问题。

**修复**: 改为 `LynxServiceCenter.inst().registerService(LynxDevToolService.INSTANCE)`

### 2. GoBackendModule.kt — JavaOnlyMap.of() 未解析

**当前代码**: 已改为 `JavaOnlyMap().apply { put(...) }`

**状态**: ✅ 已修复，无需改动

### 3. MpvPlayerModule.kt — is.xyz.mpv 语法错误 + MPVView 引用

**当前代码**: 已改为反引号 import `` `is`.xyz.mpv.MPVLib `` + 自定义 MpvSurfaceView

**状态**: ✅ 已修复，无需改动

### 4. PlayerActivityLynx.kt — getModuleByName 未解析

**当前代码**: 已改为静态持有者模式 `MpvPlayerModule.getInstance()` 等

**状态**: ✅ 已修复，无需改动

### 5. MPVLib.kt — 缺少 JNI native 方法声明

**当前代码**: MPVLib.kt 中的 `external fun` 方法（如 `create`, `init`, `destroy` 等）需要对应的 JNI 实现。这些实现在 `libplayer.so` 中。

**潜在问题**: `command(cmd: Array<out String>)` 使用了 `out` 协变，JNI 可能期望 `Array<String>`。需要检查 mpv-android 原始代码。

### 6. setup-mpv-libs.sh — .so 文件下载

**当前代码**: 从 Maven Central 下载 mpv-android-lib AAR 并提取 .so 文件

**状态**: ✅ 脚本正确，但 CI 中需要在 cap sync 之前运行

### 7. post-cap-sync.mjs — 文件复制逻辑

**当前代码**: 正确复制 overlay 文件到 android 目录

**状态**: ✅ 已修复，无需改动

## 执行计划

1. **修复 EncvApplication.kt**: `LynxDevToolService()` → `LynxDevToolService.INSTANCE`
2. **检查 MPVLib.kt**: 确认 `command` 方法签名与 JNI 匹配
3. **本地模拟验证**: 运行 `npm run build` + `npx cap sync android` 验证 post-cap-sync.mjs 正确执行
4. **提交并推送**: 确保所有修复推送到远程分支
