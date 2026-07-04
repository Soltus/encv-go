# ComboLite 模块完整性审计 Spec

## Why

当前应用已集成 ComboLite Core（`combolite-core:2.0.0` + `aar2apk:1.1.1`），但通过反射方式调用 API，存在多个关键问题：
1. **GoProcessPlugin.kt 用错误的模式获取 PluginManager 实例**（对 `object` 单例调用了不存在的 `getInstance(Context)` 方法）
2. **installPlugin 方法实际定义在 InstallerManager 上而非 PluginManager**，反射搜索路径错误
3. **ProxyManager 未配置**（无 HostActivity/ServicePool 注册），插件 Activity/Service 无法启动
4. **ResourceManager 未使用**，插件资源加载链路缺失
5. **DependencyManager 为 internal 类**，只能通过 PluginManager 间接访问

用户要求克隆源码并逐一核对 5 大管理器的集成状态。

## What Changes

- 本 spec **纯审计性质**，不做代码修改
- 输出完整的「源码 vs 现状」对比矩阵
- 标识每个 Manager 的：正确用法、当前用法、差距、修复建议

## Impact

- Affected code:
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — 插件安装（3 处反射调用全部有误）
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvApplication.kt` — 初始化（部分正确）
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerEntry.kt` — 插件查询（基本正确）
- Affected specs: `eval-combolite-mkv-ffmpeg-plugins`（MPV 插件化方案依赖正确的 ComboLite 集成）

---

## ADDED Requirements

### Requirement 1: ComboLite 5 大管理器完整审计

基于克隆的源码（`/tmp/ComboLite`），逐一审计以下 5 个管理器：

#### 1.1 PluginManager（中心协调器）

| 属性 | 值 |
|------|-----|
| 包路径 | `com.combo.core.runtime.PluginManager` |
| 类型 | Kotlin `object`（单例） |
| 获取方式 | 直接使用 `PluginManager`（**不是** `getInstance(context)`） |
| Maven 归属 | `combolite-core` |

核心 API：
- `initialize(context: Application, onSetup: (() -> Unit)?)` — 初始化框架
- `installerManager: InstallerManager` — 属性暴露
- `resourcesManager: PluginResourcesManager` — 属性暴露
- `proxyManager: ProxyManager` — 属性暴露
- `authorizationManager: AuthorizationManager` — 属性暴露
- `launchPlugin(pluginId): Boolean` — 启动插件
- `unloadPlugin(pluginId)` — 卸载插件
- `loadEnabledPlugins(): Int` — 批量加载
- `getPluginInfo(id): LoadedPluginInfo?` — 查询插件信息
- `getAllInstallPlugins(): List<PluginInfo>` — 所有已安装插件
- `setPluginEnabled(id, enabled): Boolean` — 启用/禁用
- `getInterface(ifaceClass, className): T?` — 跨插件接口获取
- `isInitialized: Boolean` — 是否已初始化

**当前应用用法审计**：

| 使用点 | 方式 | 正确？ |
|--------|------|--------|
| EncvApplication.kt L24 | `PluginManager.setValidationStrategy(Insecure)` | ✅ 直接引用 object |
| PlayerEntry.kt L44 | `com.combo.core.runtime.PluginManager` + `.isInitialized` / `.getPluginInfo()` | ✅ 直接引用 object |
| GoProcessPlugin.kt L375-377 | `Class.forName("...PluginManager").getMethod("getInstance", Context::class.java).invoke(null, context)` | ❌ **严重错误**：对 `object` 调用不存在的 `getInstance(Context)` 方法 |

**问题详情**：PluginManager 是 Kotlin `object` 单例，没有 `getInstance(Context)` 工厂方法。反射代码虽然不会崩溃（catch 了 Exception），但返回的 `pm` 始终为 null → 永远走 fallback 分支。

#### 1.2 InstallerManager（安装/更新/校验）

| 属性 | 值 |
|------|-----|
| 包路径 | `com.combo.core.runtime.installer.InstallerManager` |
| 类型 | 普通 `class`（非单例，由 PluginFrameworkContext 持有） |
| 获取方式 | `PluginManager.installerManager`（属性） |
| Maven 归属 | `combolite-core` |

核心 API：
- `suspend fun installPlugin(pluginApkFile: File, forceOverwrite: Boolean = false): InstallResult`
  - 返回 `InstallResult.Success(PluginInfo)` 或 `InstallResult.Failure(reason, exception?)`
  - **完整流程**：签名验证 → 版本检查 → APK 复制 → so 解压 → 类索引创建 → 组件解析 → XML 持久化
- `suspend fun uninstallPlugin(pluginId: String): Boolean` — 事务性卸载（重命名→删除→回滚）
- `getPluginDirectory(pluginId): File` — 获取插件安装目录

**当前应用用法审计**：

| 使用点 | 方式 | 正确？ |
|--------|------|--------|
| GoProcessPlugin.kt L384/L556 | `pm.javaClass.methods.find { it.name == "installPlugin" && it.parameterCount == 2 }` | ❌ **方向性错误**：在 PluginManager 的方法列表中搜索 installPlugin，但该方法定义在 InstallerManager 上 |
| GoProcessPlugin.kt L386/L558 | `installMethod.invoke(pm, apkFile, true)` | ❌ 即使找到也不会工作（因为方法不在 PluginManager 上） |

**正确用法应该是**：
```kotlin
// 方式 A：直接访问属性（推荐，编译时安全）
val result = PluginManager.installerManager.installPlugin(apkFile, true)

// 方式 B：如果必须用反射，应该在 installerManager 上找
val im = PluginManager::class.java.getDeclaredField("installerManager")
    ?.apply { isAccessible = true }
    ?.get(/* object instance 不需要 */)
// 或者更简单：先拿到 frameworkContext 内部字段
```

#### 1.3 ResourceManager（资源加载与管理）

| 属性 | 值 |
|------|-----|
| 包路径 | `com.combo.core.runtime.resource.PluginResourcesManager` |
| 类型 | `class PluginResourcesManager(context: Application)` |
| 获取方式 | `PluginManager.resourcesManager`（属性） |
| Maven 归属 | `combolite-core` |

核心 API：
- `loadPluginResources(pluginId, pluginFile): Boolean` — 加载插件资源
- `removePluginResources(pluginId)` — 移除插件资源
- `updatePluginResources(pluginId, pluginFile): Boolean` — 更新资源
- `getResources(): Resources` — 获取合并后的资源实例
- `refreshAllResources()` — 刷新缓存
- **Android 11+** 使用官方 `ResourcesLoader` API
- **Android <11** 使用 `AssetManager.addAssetPath` 反射 API

**当前应用用法审计**：

| 使用点 | 方式 | 正确？ |
|--------|------|--------|
| （全局搜索） | 无任何引用 | ❌ **完全缺失** |

**影响**：插件的 drawable/layout/string 资源无法被宿主加载。Compose UI 插件的 `R.drawable.xxx` / `R.string.xxx` 会抛出 `Resources.NotFoundException`。

#### 1.4 ProxyManager（四大组件代理）

| 属性 | 值 |
|------|-----|
| 包路径 | `com.combo.core.proxy.ProxyManager` |
| 类型 | `class ProxyManager(context: Application)` |
| 获取方式 | `PluginManager.proxyManager`（属性） |
| Maven 归属 | `combolite-core` |

核心 API：
- `setHostActivity(hostActivityClass)` — **必须配置**：注册代理 Activity
- `setServicePool(serviceProxyPool)` — 配置代理 Service 池
- `registerStaticReceivers(pluginId, receivers)` — 注册静态广播
- `registerProviders(pluginId, providers)` — 注册 ContentProvider
- `findReceiversForIntent(intent)` — 广播分发匹配
- `acquireServiceProxy(instanceIdentifier)` — 获取 Service 代理
- `setHostProviderAuthority(authority)` — Provider 代理 Authority

**当前应用用法审计**：

| 使用点 | 方式 | 正确？ |
|--------|------|--------|
| （全局搜索） | 无任何引用 | ❌ **完全缺失** |
| AndroidManifest.xml | 未声明 BaseHostActivity/BaseHostService 子类 | ❌ **未配置代理组件** |

**影响**：
- 插件 Activity 无法启动（没有 HostActivity 代理）
- 插件 Service 无法运行（没有 ServicePool）
- 插件 BroadcastReceiver 无法接收广播
- 插件 ContentProvider 无法处理 URI 请求
- **结论：即使 APK 安装成功，插件的功能组件也完全不可用**

#### 1.5 DependencyManager（依赖关系图 + 类索引）

| 属性 | 值 |
|------|-----|
| 包路径 | `com.combo.core.runtime.loader.DependencyManager` |
| 类型 | `internal class`（**包级私有，不可直接访问**） |
| 获取方式 | 仅通过 PluginManager 间接转发 |
| Maven 归属 | `combolite-core` |

间接可用 API（通过 PluginManager 转发）：
- `getPluginDependenciesChain(pluginId): List<String>` — 递归查询依赖
- `getPluginDependentsChain(pluginId): List<String>` — 递归查询被依赖

内部职责：
- 维护正向/反向依赖图（ConcurrentHashMap）
- 类查找时的动态依赖记录（A 加载 B 的类 → 记录 A→B 依赖）
- DFS 递归遍历依赖树
- 插件卸载时清理依赖关系

**当前应用用法审计**：

| 使用点 | 方式 | 正确？ |
|--------|------|--------|
| （全局搜索） | 无直接引用 | ⚠️ 合理（internal 类），但也没有使用 PluginManager 转发的方法 |

**影响**：低优先级。只有多插件互相依赖场景才需要。

---

## 审计总结表

| # | Manager | 类型 | 当前状态 | 严重程度 | 能否工作 |
|---|---------|------|----------|----------|---------|
| 1 | **PluginManager** | `object` 单例 | EncvApplication/PlayerEntry 用法正确；GoProcessPlugin 反射方式错误 | 🟡 中 | 部分（初始化和查询 OK，安装路径错误） |
| 2 | **InstallerManager** | `class` | **从未被正确调用**——在 PluginManager 上搜 installPlugin 找不到 | 🔴 **P0** | ❌ 安装完全不工作 |
| 3 | **ResourceManager** | `class` | **完全未集成** | 🔴 **P0** | ❌ 插件资源不可用 |
| 4 | **ProxyManager** | `class` | **完全未集成**——未注册 HostActivity/ServicePool | 🔴 **P0** | ❌ 插件组件不可启动 |
| 5 | **DependencyManager** | `internal class` | 未直接使用（合理） | 🟢 低 | N/A（单插件不需要） |

## 关键发现

### 发现 A：GoProcessPlugin.kt 的反射链路全错

```
当前错误链路：
  Class.forName("PluginManager")           ← ✅ 类名正确
    .getMethod("getInstance", Context)      ← ❌ object 没有 getInstance(Context)!
    .invoke(null, context)                 ← 返回 null（被 catch 吞掉）
  pm?.javaClass.methods.find { name == "installPlugin" && count == 2 }  ← ❌ 在 PluginManager 自身方法中找 installPlugin，但它定义在 InstallerManager 上
  
结果：pm == null → 走 fallback ACTION_INSTALL_PACKAGE（系统安装器，非 ComboLite）
```

### 发现 B：ComboLite 安装确认界面的前提条件

要看到「ComboLite 同款安装确认界面」，需要满足：
1. ✅ `EncvApplication` 继承 `BaseHostApplication`（已满足）
2. ✅ `PluginManager.initialize()` 已执行（由 BaseHostApplication 触发）
3. ❌ 通过 `PluginManager.installerManager.installPlugin(file, true)` 调用（当前代码走的是反射 fallback）
4. ❌ `AuthorizationManager` 的 handler 已配置（Insecure 模式下跳过，当前是 Insecure）

### 发现 C：MPV 插件 Activity 能启动的前提

`PlayerEntry.startMpvPlayer()` 直接 `setClassName(context, "com.encvgo.plugin.mpv.MpvPlayerActivity")` 启动 Activity。这要求：
1. 该 Activity 必须在某个 APK 的 Manifest 中声明
2. 如果是插件 APK，必须通过 ProxyManager 的 HostActivity 代理启动
3. **当前 ProxyManager 未配置 → 即使 MPV 插件 APK 存在，Activity 也无法启动**

---

## 后续修复建议（不属于本 spec 范围，供参考）

1. **P0-立即**：重写 GoProcessPlugin.kt 的安装逻辑，使用 `PluginManager.installerManager.installPlugin()` 直接调用
2. **P0-立即**：配置 ProxyManager（`setHostActivity` + `setServicePool` + Manifest 声明代理组件）
3. **P1-短期**：确保 ResourceManager 在插件加载后被自动调用（ComboLite 内部 lifecycleManager 应该会处理）
4. **P2-中期**：移除所有反射代码，改为直接 import ComboLite API

---

## 附录 A：安装确认界面布局对比

### A.1 ComboLite 官方 `InstallPermissionScreen`（Compose Material3）

源码位置：[InstallPermissionScreen.kt](/tmp/ComboLite/comboLite-core/src/main/kotlin/com/combo/core/ui/InstallPermissionScreen.kt)

触发条件：`InstallerManager.checkSignatureAndAuthorize()` → 签名不匹配 + `ValidationStrategy.UserGrant` → 启动 `AuthorizationActivity`

```
┌──────────────────────────────────────────────┐
│  ←  (TopAppBar, navigationIcon=ArrowBack)   │
├──────────────────────────────────────────────┤
│                                              │
│  ┌──────┐  ┌─────────────────────────────┐  │
│  │ ICON │  │ Plugin Name                  │  │
│  │ 56dp │  │ 版本 x.x.x                   │  │
│  │圆角12│  └─────────────────────────────┘  │
│  └──────┘                                    │
│                                              │
│         (Spacer 24dp)                         │
│  ┌── ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─┐   │
│  │ ⚠️ 该插件的数字签名与本应用不一致    │   │ ← tertiaryContainer 圆角10
│  │   可能存在未知风险，是否继续安装？  │   │   着色 tertiary
│  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─┘   │
│                                              │
│         (Spacer 24dp)                         │
│  ┌ InfoRow: 插件 ID    com.example.plugin ─┐ │
│  ├ InfoRow: 描述       这是MPV播放器插件  ─┤ │
│  └ InfoRow: 数字签名   a1b2c3d4e5...     ─┘ │
│                                              │
│              (Spacer flex=1)                 │
│  ┌──────────────────────────────────────┐   │
│  │           [ 仍然安装  ]               │   │ ← PrimaryButton fillMaxWidth
│  └──────────────────────────────────────┘   │
│            [ 取消 ]                        │   ← TextButton
│                                              │
└──────────────────────────────────────────────┘

组件清单：
- Scaffold + TopAppBar（透明背景）
- Image（插件图标，56dp 圆角12）+ Text 列（名称 + 版本）
- 警告横幅（tertiaryContainer 背景 + Info 图标 + 警告文字）
- InfoRowStyled × 3（键值对信息行）
- PrimaryButton「仍然安装」（fillMaxWidth）
- TextButton「取消」
```

### A.2 当前应用 `ExtensionsPage.vue` 安装流程（Ionic Vue）

源码位置：[ExtensionsPage.vue](app/encv-mobile/src/views/ExtensionsPage.vue)

当前实际安装路径：`handleInstallFromFile()` → `pickAndInstallPlugin()` → **GoProcessPlugin 反射失败** → fallback 到系统 `ACTION_INSTALL_PACKAGE`

```
═══ ExtensionsPage（扩展管理页）═══════════════

┌──────────────────────────────────────────────┐
│  ←  Extensions          (ion-toolbar)      │
├──────────────────────────────────────────────┤
│                                              │
│  ┌──────────────────────────────────────┐   │
│  │ 🎬 MPV Player        [未安装] badge │   │ ← ion-card
│  │                                        │   │
│  │ 高性能原生播放器，支持MKV/ASS/FLV    │   │
│  │ 大小: ~35 MB                          │   │
│  │                                        │   │
│  │ [+ 从本地安装]  (primary button)     │   │
│  └──────────────────────────────────────┘   │
│                                              │
│  ┌──────────────────────────────────────┐   │
│  │     [ ☁ 选择 APK 文件 ]             │   │ ← outline block button
│  └──────────────────────────────────────┘   │
│     选择本地 APK 文件以安装扩展           │   ← install-hint
│                                              │
└──────────────────────────────────────────────┘

点击"选择APK"后：

  pickAndInstallPlugin()
    ↓
  ACTION_GET_CONTENT (application/vnd.android.package-archive)
    ↓
  用户选择文件 → 复制到 cache/plugin_install/
    ↓
  installFromPath() → 反射找 installPlugin...
    ↓ ❌ 找不到（方法在 InstallerManager 上）
  fallback: ACTION_INSTALL_PACKAGE (系统安装器)
    ↓
  系统弹出 Google Play 风格的"是否安装应用?"对话框
    ↓
  回调: result = { success:true, method:"system", pending:true }
    ↓
┌──────────────────────┐
│ ⚠️ alertController   │
│                      │
│ 安装成功!            │
│ plugin-name.apk      │
│ 请在系统对话框中完成  │
│ 安装，然后返回此处。  │
│                      │
│        [ 确定 ]      │
└──────────────────────┘
```

### A.3 差距对比表

| 维度 | ComboLite 官方 | 当前应用 |
|------|---------------|---------|
| **界面框架** | Compose Material3 Scaffold | Ionic Vue ion-page |
| **图标展示** | ✅ 从 APK 内提取 56dp 圆角图标 | ❌ 无（只有 filmOutline 通用图标） |
| **版本显示** | ✅ 从 APK 读取 versionName | ❌ 硬编码 "~35 MB" |
| **描述信息** | ✅ 从 APK meta-data 读取 | ✅ 有（但硬编码在代码中） |
| **签名验证** | ✅ 显示 SHA-256 + 风险警告 | ❌ 完全没有 |
| **插件 ID** | ✅ 显示 packageName | ❌ 没有显示 |
| **确认按钮** | 「仍然安装」主按钮 + 取消链接 | ❌ 无确认步骤（直接调系统安装器） |
| **交互时机** | 安装前确认（可拦截/拒绝） | 安装后提示（已无法撤销） |
| **视觉风格** | Material3 原生 Compose | Ionic 卡片 + alert 弹窗 |

### A.4 结论

当前应用的"安装确认"本质上是 **安装后通知弹窗**（alertController），而非真正的 **安装前确认界面**。用户从未看到 ComboLite 的 `InstallPermissionScreen`，原因是：

1. GoProcessPlugin 反射获取 PluginManager 实例失败（`getInstance` 不存在）
2. 即使获取成功，`installPlugin` 方法在 `InstallerManager` 上不在 `PluginManager` 上
3. 因此永远 fallback 到系统 `ACTION_INSTALL_PACKAGE`
4. 系统安装器有自己的简单确认对话框，但那是 Android 系统级的，不是 ComboLite 框架提供的
