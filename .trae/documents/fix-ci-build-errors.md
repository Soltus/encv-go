# CI 构建失败修复计划

## 错误分析

CI 构建失败的根本原因是 **Kotlin 编译错误**，共 6 个问题需要修复：

### 错误 1: `EncvApplication.kt:42` — LynxDevToolService 注册方式
```
Cannot infer type for this parameter. Please specify it explicitly.
```
**原因**: `LynxDevToolService` 是 Kotlin `object` 单例，注册方式与 `LynxLogService`/`LynxHttpService` 相同，直接传对象引用即可。

**修复**: 确认 `LynxServiceCenter.inst().registerService(LynxDevToolService)` 语法正确（官方 demo 确认此方式）。如果 CI 仍然报错，可能是 `lynx-service-devtool` 依赖未正确注入到 build.gradle。

### 错误 2: `GoBackendModule.kt:131,138` — `dispatchEvent` 未解析
```
Unresolved reference 'dispatchEvent'
```
**原因**: `LynxContext` 没有 `dispatchEvent` 方法。正确的方法名是 `sendGlobalEvent`，签名：`sendGlobalEvent(String name, JavaOnlyArray params)`。

**修复**: 
- 将 `lynxContext.dispatchEvent(eventName, data)` 改为 `lynxContext.sendGlobalEvent(eventName, params)`
- 使用 `JavaOnlyArray` 构建参数：`val params = JavaOnlyArray(); params.pushMap(JavaOnlyMap.of("key", value))`
- 需要添加 import：`com.lynx.react.bridge.JavaOnlyArray` 和 `com.lynx.react.bridge.JavaOnlyMap`

### 错误 3: `GoProcessPlugin.kt:140-142` — `PlayerActivity.intentFilePath` 等未解析
```
Unresolved reference 'intentFilePath'
```
**原因**: `PlayerActivity` 现在是路由器，不再有 `companion object` 中的静态字段。这些字段在 `PlayerActivityCapacitor` 的 `companion object` 中。

**修复**: 将 `PlayerActivity.intentFilePath` 改为 `PlayerActivityCapacitor.intentFilePath` 等。

### 错误 4: `MpvPlayerModule.kt` — mpv-android-lib 包名错误
```
Unresolved reference 'io' (import io.github.abdallahmehiz.mpvlib.*)
```
**原因**: mpv-android-lib 的实际包名是 `is.xyz.mpv`，不是 `io.github.abdallahmehiz.mpvlib`。类名是 `MPVView` 和 `MPVLib`。

**修复**: 
- 将 `import io.github.abdallahmehiz.mpvlib.MPVLib` 改为 `import is.xyz.mpv.MPVLib`
- 将 `import io.github.abdallahmehiz.mpvlib.MPVView` 改为 `import is.xyz.mpv.MPVView`
- MPVView 是 `internal` 类，无法直接在外部使用。需要改用 `BaseMPVView` 或直接使用 `MPVLib` API

### 错误 5: `PlayerActivityCapacitor.kt` — 重复 companion object
```
Conflicting declarations: companion object
Only one companion object is allowed per class.
```
**原因**: `PlayerActivityCapacitor` 有两个 `companion object`。

**修复**: 将两个 companion object 合并为一个。

### 错误 6: `PlayerActivityLynx.kt:219-220,258-259` — LynxViewBuilder API 错误
```
Unresolved reference 'config'
Unresolved reference 'getModule'
```
**原因**: 
1. `LynxViewBuilder` 没有 `config` 属性。`registerModule` 方法直接在 `LynxViewBuilder` 上（继承自 `LynxBaseConfigurator`），签名是 `registerModule(String name, Class<? extends LynxModule> module)`
2. `LynxContext` 没有 `getModule()` 方法。Native Module 不需要从 Native 端获取实例，JS 端通过 `NativeModules` 全局对象直接调用

**修复**:
- 将 `viewBuilder.config.registerModule(MpvPlayerModule::class.java)` 改为 `viewBuilder.registerModule("MpvPlayerModule", MpvPlayerModule::class.java)`
- 移除 `lynxContext.getModule()` 调用
- MpvPlayerModule 和 GoBackendModule 需要在 Activity 层面持有引用，而不是通过 LynxContext 获取

## 修复步骤

### Step 1: 修复 `PlayerActivityCapacitor.kt` — 合并 companion object
将两个 companion object 合并为一个，把 `TAG` 和 `intentFilePath/FileName/FileMimeType` 放在一起。

### Step 2: 修复 `GoProcessPlugin.kt` — 引用正确的静态字段
将 `PlayerActivity.intentFilePath` 改为 `PlayerActivityCapacitor.intentFilePath`。

### Step 3: 修复 `EncvApplication.kt` — LynxDevToolService 注册
确认 `LynxServiceCenter.inst().registerService(LynxDevToolService)` 语法正确。如果 CI 报类型推断错误，可能需要显式指定泛型类型。

### Step 4: 修复 `MpvPlayerModule.kt` — mpv-android-lib 包名 + API 重构
- 修改 import 为 `is.xyz.mpv.MPVLib` 和 `is.xyz.mpv.BaseMPVView`
- 由于 `MPVView` 是 `internal` 类，需要改为使用 `BaseMPVView` 或直接通过 `MPVLib` API 控制
- 重构 MpvPlayerModule：不持有 MPVView 引用，改为通过 PlayerActivityLynx 间接控制

### Step 5: 修复 `GoBackendModule.kt` — sendGlobalEvent API
- 将 `lynxContext.dispatchEvent(eventName, data)` 改为 `lynxContext.sendGlobalEvent(eventName, params)`
- 使用 `JavaOnlyArray` + `JavaOnlyMap` 构建参数

### Step 6: 修复 `PlayerActivityLynx.kt` — registerModule + 移除 getModule
- 将 `viewBuilder.config.registerModule()` 改为 `viewBuilder.registerModule("ModuleName", ModuleClass::class.java)`
- 移除 `lynxContext.getModule()` 调用
- 在 Activity 层面直接创建和持有 MpvPlayerModule/GoBackendModule 实例
- Module 需要接收 LynxContext 参数，但 LynxContext 在 LynxView 构建后才可用

### Step 7: 更新 `post-cap-sync.mjs` — 确认依赖注入
确认所有 Gradle 依赖正确注入，包括 `lynx-service-devtool:3.7.0`。

### Step 8: 验证构建
重新运行 `npm run build`（lynx-player）和检查所有 Kotlin 文件的一致性。
