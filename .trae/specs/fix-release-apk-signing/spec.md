# 修复 Release APK 签名缺失（第三次回归）

## Why

Release APK 构建成功但 `apksigner verify` 报 `DOES NOT VERIFY / Missing META-INF/MANIFEST.MF`，这是**第三次**出现此问题。根因是 `build.gradle.kts`（Kotlin DSL）中缺少 `signingConfigs` 块，而 `post-cap-sync.mjs` 只操作 Groovy DSL 的 `build.gradle`，完全忽略了 `.kts` 文件。Capacitor CLI 的 `--keystorepath` 参数在 Kotlin DSL 项目中无法正确注入签名配置。

## 根因分析

### 洞察：Groovy/Kotlin DSL 双轨制导致的系统性盲区

项目从 Capacitor 脚手架继承了 Groovy DSL 的 `build.gradle`，后来迁移到 Kotlin DSL 的 `build.gradle.kts`。但 `post-cap-sync.mjs` 仍然只操作 `build.gradle`（Groovy），导致：

1. **签名配置从未注入到实际构建文件**：`post-cap-sync.mjs` 第 240 行 `if (version && c.includes('minifyEnabled false'))` 检查的是 Groovy 语法，但 `build.gradle.kts` 使用 `isMinifyEnabled = true`（Kotlin DSL 语法），条件永远不匹配
2. **Capacitor CLI 的 `--keystorepath` 参数无效**：Capacitor CLI 通过 Gradle 属性注入签名配置，但 Kotlin DSL 项目需要在 `build.gradle.kts` 中显式声明 `signingConfigs` 块才能使用这些属性
3. **`assembleRelease` 成功但不签名**：没有 `signingConfig` 的 release 构建类型会生成未签名的 APK，Gradle 不报错

### 为什么反复回归

| 次数 | 尝试的修复 | 为什么失败 |
|------|-----------|-----------|
| 第1次 | `apply from: '../encv-release.gradle'` | Gradle 8 属性加载时序问题，signingConfig 不生效 |
| 第2次 | `post-cap-sync.mjs` 注入到 `build.gradle` | 只操作 Groovy DSL，项目实际用 `.kts` |
| 第3次 | 同上 | 同上，问题从未被真正修复 |

### 防回归范式

**核心原则：签名配置必须直接写入 `build.gradle.kts`，不依赖任何运行时注入机制。**

理由：
- `build.gradle.kts` 是版本控制的源文件，修改可追溯
- 不依赖 `post-cap-sync.mjs` 的字符串匹配逻辑（脆弱）
- 不依赖 Capacitor CLI 的 `--keystorepath` 参数（对 Kotlin DSL 无效）
- 不依赖 `apply from` 外部文件（Gradle 8 时序问题）

## What Changes

- 在 `build.gradle.kts` 中添加 `signingConfigs` 块和 `signingConfig` 引用
- 移除 `post-cap-sync.mjs` 中对 `build.gradle`（Groovy）的签名注入逻辑（已无用）
- 移除 CI 工作流中 `npx cap build` 的 `--keystorepath` 等参数（对 Kotlin DSL 无效）
- 添加 CI 构建后签名验证步骤（防回归）

## Impact

- Affected code: `app/encv-mobile/android/app/build.gradle.kts`、`app/encv-mobile/scripts/post-cap-sync.mjs`、`.github/workflows/android.yml`
- Affected specs: 无

## ADDED Requirements

### Requirement: Release APK 签名配置直接写入 build.gradle.kts

`build.gradle.kts` SHALL 在 `android {}` 块中声明 `signingConfigs` 和 `buildTypes.release.signingConfig`，确保 `assembleRelease` 自动签名。

#### Scenario: signingConfigs 声明
- **WHEN** `build.gradle.kts` 被 Gradle 解析
- **THEN** `android.signingConfigs.release` SHALL 存在，包含 `storeFile`、`storePassword`、`keyAlias`、`keyPassword`

#### Scenario: release 构建类型引用签名配置
- **WHEN** 执行 `assembleRelease`
- **THEN** `buildTypes.release.signingConfig` SHALL 引用 `signingConfigs.release`

#### Scenario: APK 签名验证
- **WHEN** `assembleRelease` 完成
- **THEN** `apksigner verify` SHALL 通过，APK 包含 `META-INF/MANIFEST.MF`

### Requirement: 签名凭据通过环境变量或文件引用

签名凭据 SHALL NOT 硬编码在 `build.gradle.kts` 中。使用以下方式之一：
- 从环境变量读取（`System.getenv()`）
- 从 `keystore.properties` 文件读取
- 从 Gradle 属性读取

**推荐方案**：使用 `keystore.properties` 文件（与 Android 官方文档一致），CI 中动态生成该文件。

### Requirement: CI 构建后签名验证（防回归）

CI 工作流 SHALL 在 Release APK 构建后执行 `apksigner verify`，验证失败则构建失败。

#### Scenario: 签名验证失败
- **WHEN** `apksigner verify` 返回非零退出码
- **THEN** CI 构建 SHALL 失败并输出明确错误信息

### Requirement: 移除无效的签名注入逻辑

`post-cap-sync.mjs` 中对 Groovy `build.gradle` 的签名注入逻辑 SHALL 被移除或标记为废弃，因为项目已迁移到 Kotlin DSL。

## MODIFIED Requirements

无

## REMOVED Requirements

### Requirement: 通过 post-cap-sync.mjs 注入签名配置到 build.gradle

**Reason**: 项目已迁移到 Kotlin DSL（`build.gradle.kts`），Groovy DSL 的 `build.gradle` 不再使用。字符串匹配注入方式脆弱且已三次失败。
**Migration**: 签名配置直接写入 `build.gradle.kts`，通过 `keystore.properties` 文件读取凭据。
