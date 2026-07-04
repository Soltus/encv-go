# Plan: 用 Capacitor 官方机制替代正则替换 build.gradle

## 核心发现（来自源码分析）

通过阅读 Capacitor 8 CLI 源码 (`node_modules/@capacitor/cli/dist/android/update.js`)，确认了以下关键事实：

### `cap sync` 实际上只改这些文件

| 文件 | 行为 | 我们能否安全修改？ |
|------|------|------------------|
| `android/app/build.gradle` | ❌ **不触碰** | ✅ 安全，cap sync 不动它 |
| `android/build.gradle` (root) | ❌ **不触碰** | ✅ 安全 |
| `android/variables.gradle` | ❌ 只读取不写入 | ✅ 安全 |
| `android/gradle.properties` | ❌ 完全不动 | ✅ **最安全** |
| `android/settings.gradle` | ❌ 不触碰 | ✅ 安全 |
| `android/capacitor.settings.gradle` | ⚠️ 每次覆盖 | ❌ |
| `android/app/capacitor.build.gradle` | ⚠️ 每次覆盖 | ❌ |

**结论：`app/build.gradle` 在 `cap add android` 后生成一次，之后 `cap sync` 不会重新生成它！**

这意味着我们之前所有的正则替换方案都是基于一个错误的前提 —— 以为 cap sync 会覆盖 build.gradle。

## 正确方案：Gradle 原生配置机制

### 方案架构

```
┌─────────────────────────────────────────────────────┐
│  android/gradle.properties                          │
│  （Capacitor 完全不碰这个文件）                       │
│                                                     │
│  ENCV_STORE_FILE=../keystore/release.jks            │
│  ENCV_STORE_PASSWORD=encv2025                       │
│  ENCV_KEY_ALIAS=encvrelease                         │
│  ENCV_KEY_PASSWORD=encv2025                         │
└─────────────────────────────────────────────────────┘
                        ↓ 被引用
┌─────────────────────────────────────────────────────┐
│  android/app/build.gradle                           │
│  （Capacitor 生成后不再改动）                        │
│                                                     │
│  // 唯一需要注入的内容：                             │
│  apply from: '../encv-release.gradle'               │
│  // 或直接在 release {} 里引用签名                    │
└─────────────────────────────────────────────────────┘
                        ↓ 引用
┌─────────────────────────────────────────────────────┐
│  android/encv-release.gradle                        │
│  （我们的文件，从 overlay 复制过来）                  │
│                                                     │
│  android {                                          │
│    signingConfigs {                                 │
│      release {                                      │
│        storeFile file(ENCV_STORE_FILE)             │
│        storePassword ENCV_STORE_PASSWORD            │
│        keyAlias ENCV_KEY_ALIAS                      │
│        keyPassword ENCV_KEY_PASSWORD                │
│      }                                             │
│    }                                                │
│    ndk { abiFilters 'arm64-v8a' }                  │
│  }                                                  │
│  android.buildTypes.release {                       │
│    minifyEnabled true                               │
│    shrinkResources true                             │
│    signingConfig signingConfigs.release              │
│  }                                                  │
└─────────────────────────────────────────────────────┘
```

### 为什么这次一定可行

1. **`gradle.properties` 是标准 Gradle 机制** — 所有 Android 项目都用它管理签名，不是 hack
2. **`apply from` 是标准 Gradle 机制** — 官方推荐的外部配置方式
3. **两个文件都不被 cap sync 触及** — 从源码证实
4. **不需要任何正则** — 只需追加一行 `apply from`

## 实施步骤

### Step 1: 创建 `android-overlay/gradle.properties`
- 包含签名相关属性（storeFile、passwords、alias）
- 这个文件不会被 cap sync 触碰

### Step 2: 简化 `post-cap-sync.mjs`
- 移除所有正则替换逻辑
- 只做两件事：
  1. 在 `app/build.gradle` 的 `plugins {}` 后追加 `apply from: '../encv-release.gradle'`
  2. 设置 versionCode/versionName（简单字符串替换，有幂等检查）

### Step 3: 完善 `android-overlay/encv-release.gradle`
- 使用 `${project.property('ENCV_xxx')}` 引用 gradle.properties 的值
- 包含 signingConfigs、ndk abiFilters、buildTypes.release 配置
- 这是纯 Gradle DSL，不需要任何 hack

### Step 4: 更新 CI workflow
- keystore 准备好后，将 `gradle.properties` 复制到 `android/`
- 将 `encv-release.gradle` 复制到 `android/`
- 运行 `cap sync`（不会影响上述文件）
- 直接 `./gradlew assembleRelease`

### Step 5: 清理测试文件
- 删除 `scripts/test-patch.mjs`
- 更新 `solo.md` 记录正确方案

## 风险评估

| 风险 | 可能性 | 缓解措施 |
|------|--------|---------|
| 未来 Capacitor 版本改变模板中 build.gradle 结构 | 低 | `apply from` 只要求 plugins 块存在，位置灵活 |
| `gradle.properties` 属性名冲突 | 极低 | 使用 `ENCV_` 前缀避免冲突 |
| `apply from` 路径问题 | 低 | 相对路径 `../encv-release.gradle` 固定不变 |
