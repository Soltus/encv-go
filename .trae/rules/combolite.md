# ComboLite 集成规范

> **核心原则：0 Hook、0 反射。** ComboLite 是完全基于 Android 官方公开 API 构建的框架。
> **任何对 ComboLite API 使用反射的代码都是错误的，说明没有理解框架设计。**
> **ComboLite 内部使用 kotlin-reflect（`::function.javaMethod`）做权限检查，因此宿主构建必须保证字节码与 @Metadata 注解的一致性。**

> **完整内容 + R8 破坏机制 + 实战案例**：[详情文档](../rule-library/combolite.md)

---

## 一、铁律（违反 = 严重错误）

### 1.1 禁止反射调用 ComboLite API

**❌ 错误（幻觉代码）**：
```kotlin
// PluginManager 是 Kotlin object 单例，没有 getInstance(Context) 方法！
val pm = Class.forName("com.combo.core.runtime.PluginManager")
    .getMethod("getInstance", Context::class.java)
    .invoke(null, context)

// installPlugin 定义在 InstallerManager 上，不在 PluginManager 上！
val method = pm.javaClass.methods.find { it.name == "installPlugin" && it.parameterCount == 2 }
method.invoke(pm, apkFile, true)

// getAllInstallPlugins 是直接方法，不需要反射猜测方法名
val pluginsMethod = pm.javaClass.methods.find {
    it.name == "getInstalledPlugins" || it.name == "getLoadedPlugins" || ...
}
```

**✅ 正确（直接引用）**：
```kotlin
import com.combo.core.runtime.PluginManager

// object 单例，直接使用
if (PluginManager.isInitialized) {
    // 通过属性访问子管理器
    val result = PluginManager.installerManager.installPlugin(apkFile, true)
    val plugins = PluginManager.getAllInstallPlugins()
    val info = PluginManager.getPluginInfo("mpv-player")
}
```

### 1.2 禁止 Hook 任何系统服务

ComboLite 的核心价值就是 **0 Hook**。如果发现代码中有：
- `XposedHelpers.findAndHookMethod`
- `Instrumentation` 替换
- AMS/PMS 代理
- ClassLoader `pathList` 修改

**立即删除**，这不是 ComboLite 的用法。

### 1.3 禁止在 release 构建中启用 R8/ProGuard

> **ComboLite 框架内部使用 `::function.javaMethod`（kotlin-reflect API）做 @RequiresPermission 权限检查。**
> **R8 会破坏 ComboLite 类的字节码与 `@Metadata` 注解之间的一致性，导致 kotlin-reflect 无法解析函数签名。**
> **ComboLite 官方 demo (`app/build.gradle.kts`) 明确设置 `isMinifyEnabled = false`。**

**受影响的 4 个类**（全部使用 `::function.javaMethod`）：

| 类 | 使用 javaMethod 的方法 |
|---|------|
| `PluginManager` | `setValidationStrategy`, `loadEnabledPlugins`, `launchPlugin`, `unloadPlugin`, `setPluginEnabled` |
| `InstallerManager` | `installPlugin`, `uninstallPlugin` |
| `PluginCrashHandler` | `setGlobalClashCallback`, `setClashCallback` |
| `AuthorizationManager` | `setAuthorizationHandler` |

**R8 破坏机制（2026-05-28 实测确认）**：
```
@Metadata 存在且描述原始名称:
  mv=[2,2,0], k=1

但 R8 将方法重命名:
  setValidationStrategy → a()

kotlin-reflect 解析时:
  PluginManager.class -> lookupMethod "setValidationStrategy" 
  → 在重命名后的字节码里找不到这个名字
  → 抛 NullPointerException 或 IllegalArgumentException
  → @RequiresPermission 权限检查失败
  → installPlugin 永远报 "no suitable plugin"
```

> 完整 R8 破坏链路 + 重命名日志 + 实测堆栈 → 详见 [详情文档 §1.3](../rule-library/combolite.md#13-r8-破坏机制)

---

## 二、isMinifyEnabled / isShrinkResources 硬耦合

| 选项 | 作用对象 | 对 ComboLite 的风险 | 约束 |
|------|---------|-------------------|------|
| `isMinifyEnabled` | **DEX 字节码** | **致命**：破坏 `@Metadata ↔ 字节码` 一致性 | **必须 `false`** |
| `isShrinkResources` | **resources.arsc** | 理论上安全（只读 DEX），但 AGP 强制耦合 | **必须 `false`** |

**为什么 `isShrinkResources=true` 不能单独使用**：

AGP 源码级硬约束（`AndroidResourcesCreationConfigImpl.kt:91`）：
```kotlin
if (!buildType.isMinifyEnabled && androidResources.shrink) {
    issueReporter.reportError("Removing unused resources requires unused code shrinking to be turned on.")
}
```

**结论**：由于 ComboLite 要求 `isMinifyEnabled=false`，`isShrinkResources` 也必须为 `false`。两者是 AGP 强耦合的。

---

## 三、build.gradle.kts 标准配置

```kotlin
android {
    buildTypes {
        release {
            isMinifyEnabled = false      // ← ComboLite 强制要求
            isShrinkResources = false    // ← AGP 强制耦合（同上必须 false）
        }
        debug {
            isMinifyEnabled = false
            isShrinkResources = false
        }
    }
}
```

**完整 isMinifyEnabled 调试经验 + 实测对比 + 何时可临时开启** → 详见 [详情文档 §三](../rule-library/combolite.md)

---

## 四、关键 API 速查

| API | 用途 | 备注 |
|------|------|------|
| `PluginManager.isInitialized` | 检查 ComboLite 是否已初始化 | 必须在 init 完成后调用 |
| `PluginManager.installerManager` | 访问子管理器 | 通过属性而非 `getInstallerManager()` |
| `PluginManager.installerManager.installPlugin(apk, isUpdate)` | 安装/更新插件 | 第二个参数 bool |
| `PluginManager.getAllInstallPlugins()` | 获取已安装插件 | 不要用 `getInstalledPlugins` 等假名 |
| `PluginManager.getPluginInfo("name")` | 获取单个插件信息 | |
| `PluginManager.launchPlugin("name")` | 启动插件 | |

**API 误用历史 + 实例代码 + 调试技巧** → 详见 [详情文档 §四](../rule-library/combolite.md#四api-使用陷阱实战)

---

## 五、引用其他规则

- [android.md](./android.md) — SQLite / LibSQL 选型、仓库顺序铁律
- [combolite-core 升级注意事项](./android.md#二版本管理)

> 拆分：2026-06-11
