# 修复 ComboLite 全部集成缺陷 Spec

## Why

基于 `audit-combolite-modules` 的审计结果，当前应用存在以下未修复问题：

1. **GoProcessPlugin.kt 反射获取 PluginManager 使用不存在的 `getInstance(Context)` 方法**（PluginManager 是 Kotlin `object` 单例）
2. **installPlugin 反射搜索路径错误**（在 PluginManager 上搜，但方法定义在 InstallerManager 上）——虽然 parameterCount 已从 1 改为 2，但搜索对象错误，仍然找不到方法
3. **ProxyManager 完全未配置**（无 HostActivity/ServicePool 注册），插件 Activity/Service 无法启动
4. **ResourceManager 完全未集成**

前一轮修复（fix-p0-file-loss-and-reverts）已完成的 3 项：
- ✅ FilePreview.vue 文本预览 iframe 回退
- ✅ TempFileReadCloser.Close() 移除 os.Remove
- ✅ parameterCount 1→2（但搜索路径仍错）

## What Changes

### 变更清单

1. **重写 GoProcessPlugin.kt 的反射逻辑**
   - 移除错误的 `Class.forName("...PluginManager").getMethod("getInstance", Context).invoke(null, context)`
   - 改为直接使用 `com.combo.core.runtime.PluginManager` object 引用
   - 通过 `PluginManager.installerManager` 获取 InstallerManager 实例
   - 在 InstallerManager 上调用 `installPlugin(File, Boolean)`

2. **配置 ProxyManager**
   - 在 EncvApplication 或合适位置调用 `PluginManager.proxyManager.setHostActivity(HostActivity::class.java)`
   - 声明 BaseHostActivity 子类（如 EncvHostActivity）并在 AndroidManifest.xml 中注册
   - 配置 ServicePool（如果插件需要 Service）

3. **确保 ResourceManager 集成**
   - 确认 ComboLite 内部 lifecycleManager 是否自动触发资源加载
   - 如果没有，手动在插件加载后调用 `PluginManager.resourcesManager.loadPluginResources()`

## Impact

- Affected code:
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — 核心修改：移除错误反射，改用直接 API 调用
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvApplication.kt` — 可能需要添加 ProxyManager/ResourceManager 初始化
  - `app/encv-mobile/android/app/src/main/AndroidManifest.xml` — 添加 HostActivity 声明
  - 可能新增 `EncvHostActivity.kt` — BaseHostActivity 子类
- Affected specs: `audit-combolite-modules`（本 spec 直接修复其发现的问题）

---

## ADDED Requirements

### Requirement 1: 正确的 PluginManager 访问方式

系统 SHALL 使用 Kotlin `object` 单例方式直接引用 `com.combo.core.runtime.PluginManager`，而非通过反射 `getInstance(Context)` 获取。

#### Scenario 1.1: installPlugin 正确调用
- **WHEN** 用户选择 APK 文件并触发安装
- **THEN** 系统 SHALL 通过 `PluginManager.installerManager.installPlugin(apkFile, true)` 调用安装方法
- **AND** 返回值 SHALL 为 `InstallResult.Success(pluginInfo)` 或 `InstallResult.Failure`
- **AND** 用户 SHALL 看到 ComboLite 框架的安装确认界面（而非系统安装器）

#### Scenario 1.2: checkInstalledPlugins 正确查询
- **WHEN** ExtensionsPage 加载时查询已安装插件
- **THEN** 系统 SHALL 通过 `PluginManager.getAllInstallPlugins()` 或 `PluginManager.getPluginInfo(id)` 获取列表
- **AND** 不再使用反射方式

### Requirement 2: ProxyManager 配置

系统 SHALL 配置 ProxyManager 以支持插件四大组件代理。

#### Scenario 2.1: Activity 代理
- **WHEN** 插件 APK 包含 Activity 组件（如 MpvPlayerActivity）
- **THEN** 系统 SHALL 通过 ProxyManager.setHostActivity() 注册宿主代理 Activity
- **AND** AndroidManifest.xml 中 SHALL 声明该代理 Activity
- **AND** 插件 Activity 可以通过 HostActivity 代理启动

#### Scenario 2.2: Service/Receiver/Provider 代理
- **WHEN** 插件包含 Service、BroadcastReceiver 或 ContentProvider
- **THEN** 对应的代理组件 SHALL 被 ProxyManager 正确注册和分发

### Requirement 3: ResourceManager 集成

系统 SHALL 确保 ResourceManager 在插件加载后正确加载插件资源。

#### Scenario 3.1: 插件资源可用
- **WHEN** 插件 APK 安装并加载成功
- **THEN** 插件的 drawable、layout、string 等 resources SHALL 可被宿主应用访问
- **AND** 不会抛出 Resources.NotFoundException

---

## MODIFIED Requirements

### Requirement 4: GoProcessPlugin.kt 反射代码清理

现有的三处反射代码块 SHALL 被替换为直接 API 调用：

| 位置 | 当前代码 | 替换为 |
|------|----------|--------|
| L373-397 | `Class.forName(...).getMethod("getInstance"...)` + `pm?.javaClass.methods.find { installPlugin && count==2 }` | `PluginManager.installerManager.installPlugin(apkFile, true)` |
| L537-569 | 同上（pickAndInstallPlugin 路径） | 同上 |
| L444-486 | `Class.forName(...).getMethod("getInstance"...)` + `pm?.getAllInstallPlugins()` | `PluginManager.getAllInstallPlugins()` |

**BREAKING**: 移除所有 `Class.forName` / `getMethod` / `invoke` 反射代码。改为直接 import 和调用 ComboLite API。
