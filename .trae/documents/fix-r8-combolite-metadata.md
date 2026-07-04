# 修复 R8 破坏 ComboLite @Metadata 一致性导致 kotlin-reflect 全部失败

## 问题诊断（来自饱和调试日志）

### 关键证据

**所有 4 个 ComboLite 类的 `::function.javaMethod` 全部失败：**

| 类 | 方法 | 错误 |
|---|------|------|
| PluginManager | setValidationStrategy | `KotlinReflectionInternalError: Function not resolved` |
| PluginManager | loadEnabledPlugins | 同上 |
| PluginManager | launchPlugin | 同上 |
| InstallerManager | installPlugin | 同上 |
| PluginCrashHandler | setGlobalClashCallback | 同上 |
| AuthorizationManager | setAuthorizationHandler | 同上 |

**关键发现：`@Metadata` 注解存在但与实际方法不一致！**

```
@Metadata = true
mv = [2, 2, 0]     ← ComboLite 编译时用的 Kotlin 2.2.0 的元数据格式
k = 1

R8 混淆后的方法名：
[a([Application, KoinApplication]),    ← 被混淆为 'a'
 getInterfaceFromHost(...),           ← 保留原名
 initialize$default(...),             ← 保留原名
 awaitInitialization([e]),            ← 参数类型被混淆为 'e'
 ...
]
```

**validationStrategy = Strict** ← `setValidationStrategy(Insecure)` 从未成功执行！

**onFrameworkSetup 日志为空** ← EncvApplication 中的 try-catch 吞掉了错误，没有任何日志输出！

### 根因分析

**R8 在保留类的同时混淆了方法名和参数类型名**，导致：

1. `@Metadata` 注解仍然描述原始方法名（如 `setValidationStrategy`）
2. 但 R8 将方法重命名为 `a()`、参数类型从 `ValidationStrategy` 改为 `d6.j0`
3. `kotlin-reflect` 读取 `@Metadata` 后去查找名为 `setValidationStrategy` 的方法 → 找不到（已被改名为 `a`）→ 抛出 `KotlinReflectionInternalError`

**为什么 `-keep class com.combo.core.** { *; }` 没有完全生效？**
- 部分方法保留了原名（`getInterfaceFromHost`、`initialize$default` 等）
- 部分方法被混淆（`a()`、`e()`）
- 这说明 keep 规则对 public 方法有效，但对某些 internal/suspend 函数的合成方法可能不够

**ComboLite demo 为什么没问题？**
- Demo 的 `app/build.gradle.kts`: `isMinifyEnabled = false` — 根本没开 R8！

---

## 修复方案

### 方案：禁用 R8 混淆（与 ComboLite demo 保持一致）

**理由：**
- ComboLite 框架依赖 kotlin-reflect 做权限检查，而 kotlin-reflect 要求字节码与 @Metadata 完全一致
- R8 即使 keep 了类名，仍可能修改内部方法名、synthetic accessor、lambda 类等，破坏一致性
- ComboLite 官方 demo 明确设置 `isMinifyEnabled = false`，说明框架设计上就不兼容 R8
- 对于插件框架这种需要反射的场景，禁用混淆是业界常见做法

### 具体步骤

#### Step 1: 修改 app/build.gradle.kts — release 构建禁用 R8

文件：`app/encv-mobile/android/app/build.gradle.kts`

```kotlin
buildTypes {
    release {
        isMinifyEnabled = false          // ← 改为 false（原为 true）
        isShrinkResources = false         // ← 同步改为 false
        signingConfig = signingConfigs.findByName("release")
        proguardFiles(
            getDefaultProguardFile("proguard-android-optimize.txt"),
            "proguard-rules.pro"
        )
    }
}
```

#### Step 2: 清理 proguard-rules.pro — 移除不再需要的 ComboLite keep 规则

文件：`app/encv-mobile/android/app/proguard-rules.pro`

既然禁用了 R8，ComboLite 的 keep 规则就不需要了。只保留 Bugly 规则：

```proguard
# Bugly 混淆保留
-dontwarn com.tencent.bugly.**
-keep public class com.tencent.bugly.**{*;}
```

移除以下内容（不再需要）：
- `-keep class com.combo.core.** { *; }`
- `-keepattributes RuntimeVisibleAnnotations,RuntimeInvisibleAnnotations`
- `-keep class kotlin.reflect.** { *; }`
- `-dontwarn kotlin.reflect.**`

> 注意：如果未来重新启用 R8，这些规则需要加回来并加强。

#### Step 3: 增强 EncvApplication.onFrameworkSetup 日志输出

当前问题：`setValidationStrategy` 失败时被 try-catch 吞掉，日志中无任何记录。

文件：`app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvApplication.kt`

在现有的 try-catch 基础上，确保 **Log.w/i 输出始终执行**（目前代码已正确，但需确认 Log tag 可搜索）。

#### Step 4: 提交推送，CI 构建

#### Step 5: 验证清单

用户安装新构建后，依次点击 4 个诊断按钮确认：

1. **🔧 kotlin-reflect健康检查** → 所有 `::function.javaMethod` 应返回非 null，不再抛 Error
2. **🔧 ValidationStrategy状态** → `setValidationStrategy(Insecure)` 应成功，strategy 应变为 Insecure
3. **🔧 APK元数据+签名校验** → 选择 APK 后应显示完整元数据
4. **🔧 installPlugin实际调用** → 应返回 SUCCESS 或有意义的 Failure reason（而非 kotlin-reflect Error）

---

## 不采用的其他方案及原因

### 方案 B：加强 ProGuard 规则（keep 所有成员包括 synthetic）
- **问题**：R8 对 suspend 函数生成的 `$default`、`$lambda`、continuation 参数类型的处理非常复杂，即使加了 keep 规则也可能遗漏
- **风险**：每次 ComboLite 更新都可能引入新的需要 keep 的方法/类
- **维护成本高**：需要持续同步 ComboLite 内部实现变化

### 方案 C：将 ComboLite 编译为 Kotlin 2.3.21 匹配项目版本
- **问题**：无法控制第三方 AAR 的编译器版本；ComboLite POM 声明的 kotlin-reflect:2.2.0 运行时会被 Gradle 解析为 2.3.21（version alignment），但 AAR 内的字节码 @Metadata 已经是 2.2.0 格式
- **且根本问题不是版本不匹配，而是 R8 破坏了字节码与 @Metadata 的一致性**
