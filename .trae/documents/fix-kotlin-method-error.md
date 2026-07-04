# 修复计划：`Could not find method kotlin()` 构建错误

## 问题分析

### 错误信息
```
Could not find method kotlin() for arguments [...] 
on project ':app' of type org.gradle.api.Project.
```
发生在 `build.gradle` 第 66 行。

### 根因

上一次修复中，我将 `kotlinOptions` 替换为顶层 `kotlin {}` 块：
```groovy
kotlin {                          // ❌ 这是 Kotlin DSL (.gradle.kts) 语法！
    compilerOptions {
        jvmTarget = "17"
    }
}
```

但 Capacitor 生成的 `build.gradle` 是 **Groovy DSL**，不是 Kotlin DSL。**`kotlin {}` 在 Groovy DSL 中不存在**——它是 Kotlin DSL 的专属语法。

### 正确的 Groovy DSL 方式

在 Groovy DSL 中设置 Kotlin JVM target 有两种方式：

| 方式 | 语法 | 条件 |
|------|------|------|
| A: `android.kotlinOptions` | `android { kotlinOptions { jvmTarget = '17' } }` | 需要 `kotlin-android` 插件正确应用 |
| B: `tasks.withType(KotlinCompile)` | `tasks.withType(KotlinCompile) { ... }` | 最通用，不依赖插件状态 |

**方式 B 是最安全的选择**，因为它：
- 不依赖 `kotlin-android` 插件是否被正确应用
- 不依赖 AGP 版本
- 兼容新旧两种 Gradle 插件格式
- 是 Groovy DSL 原生支持的写法

## 修复方案

### 文件：`/workspace/app/encv-mobile/scripts/post-cap-sync.mjs`

将步骤 7（当前有问题的 `kotlin {}` 块）替换为：

```javascript
// 7. Kotlin JVM target (Groovy DSL 兼容写法)
// 使用 tasks.withType(KotlinCompile) 设置 jvmTarget
// 这是 Groovy DSL 中设置 Kotlin 编译参数的标准方式
if (!c.includes('jvmTarget') && c.includes('kotlin-android')) {
  c += `
tasks.withType(org.jetbrains.kotlin.gradle.tasks.KotlinCompile).configureEach {
    kotlinOptions {
        jvmTarget = "17"
    }
}
`
}
```

生成的 Gradle 代码：
```groovy
tasks.withType(org.jetbrains.kotlin.gradle.tasks.KotlinCompile).configureEach {
    kotlinOptions {
        jvmTarget = "17"
    }
}
```

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `scripts/post-cap-sync.mjs` | 步骤 7：删除 `kotlin {}` 块 → 改用 `tasks.withType(KotlinCompile)` |

## 验证方式
1. 本地运行 `cd app/encv-mobile && npx cap sync android` 确认脚本执行无报错
2. 检查生成的 `android/app/build.gradle` 末尾包含 `tasks.withType(...KotlinCompile)...`
3. TypeScript 编译：`npx vue-tsc --noEmit`
