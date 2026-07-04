# 修复计划：`Could not find method kotlinOptions()` 构建错误

## 问题分析

### 错误信息
```
A problem occurred evaluating project ':app'.
> Could not find method kotlinOptions() for arguments [build_22v70s...] 
  on extension 'android' of type com.android.build.gradle.internal.dsl.BaseAppModuleExtension
```

### 根因
Capacitor 8 使用 **新的 `plugins { }` DSL 语法**（不是旧的 `apply plugin:`），而我们的 `post-cap-sync.mjs` 用字符串替换方式添加 `kotlin-android` 插件和 `kotlinOptions` 块，导致：

1. **`kotlin-android` 插件未正确应用** — 字符串替换 `'com.android.application'` → 添加 `'kotlin-android'` 在新 DSL 格式中是非法语法，导致 `kotlin-android` 插件根本没有被加载
2. **`kotlinOptions()` 方法不存在** — 因为 `kotlin-android` 插件没有生效，`android {}` 扩展上就没有 `kotlinOptions()` 方法
3. **正则匹配可能失败** — 步骤 2c 的 regex `/compileOptions\s*\{[^}]*\}/` 用 `[^}]*` 无法正确匹配多行内容

## 修复方案

### 文件：`/workspace/app/encv-mobile/scripts/post-cap-sync.mjs`

#### 修改 1：修正 `kotlin-android` 插件添加方式（步骤 1）

**当前代码（有问题）：**
```javascript
c = c.replace(
  "'com.android.application'",
  "'com.android.application'\n    'kotlin-android'"
)
```
只对旧格式 `apply plugin: 'xxx'` 有效。对新格式 `plugins { id 'xxx' }` 会破坏语法。

**修改为：** 同时兼容新旧两种格式：
```javascript
// 兼容 plugins { id '...' } 新格式 和 apply plugin: '...' 旧格式
if (!c.includes('kotlin-android') && !c.includes('org.jetbrains.kotlin.android')) {
  if (c.match(/plugins\s*\{/)) {
    // 新 DSL 格式: 在 plugins 块中添加 id 'kotlin-android'
    c = c.replace(
      /plugins\s*\{/,
      "plugins {\n    id 'kotlin-android'"
    )
  } else if (c.includes("'com.android.application'")) {
    // 旧 apply 格式
    c = c.replace(
      "'com.android.application'",
      "'com.android.application'\n    'kotlin-android'"
    )
  }
}
```

#### 修改 2：去掉 `kotlinOptions`，改用安全方式设置 JVM target（步骤 2c）

**当前代码（有问题）：**
```javascript
// 2c. kotlinOptions (required for Kotlin compilation)
if (!c.includes('kotlinOptions')) {
  c = c.replace(
    /compileOptions\s*\{[^}]*\}/,
    "compileOptions {...}\n\n    kotlinOptions {\n        jvmTarget = '17'\n    }"
  )
}
```

问题：
- `[^}]*` 正则无法匹配多行 `compileOptions` 块
- 即使匹配成功，如果 `kotlin-android` 没有正确应用，`kotlinOptions` 也会报错
- 在 AGP 8.x + Kotlin 2.x 中推荐使用新的 `kotlin { compilerOptions {} }` 块

**修改为：** 完全删除步骤 2c（`kotlinOptions`），改用 **顶层 `kotlin` 配置块**（Kotlin 1.9+/AGP 8.x 推荐方式）：
```javascript
// 替代 kotlinOptions: 使用顶层 kotlin {} 块（Kotlin 1.9+, AGP 8.x 推荐方式）
if (!c.includes('compilerOptions') && !c.includes('jvmTarget')) {
  // 在文件末尾的 } 之前插入 kotlin 配置块
  c = c.replace(
    /^(\s*)}$/m,
    `$1kotlin {\n$1    compilerOptions {\n$1        jvmTarget = "17"\n$1    }\n$1}`
  )
}
```

这样生成的 Gradle 代码：
```groovy
kotlin {
    compilerOptions {
        jvmTarget = "17"
    }
}
```

这是 Kotlin 1.9+ 和 AGP 8.x 的标准做法，不依赖 `kotlinOptions` 在 `android {}` 内部。

#### 修改 3：简化 compileOptions 版本为 1_8（步骤 2b）

将 `compileOptions` 从 `VERSION_17` 改回 `VERSION_1_8`（或 `11`），因为：
- Capacitor 8 默认 targetSdk 是 34/35，compileOptions 设 17 可能与其他依赖不兼容
- Kotlin 的 `jvmTarget` 已经在 `kotlin { compilerOptions {} }` 中设为 17
- Java source/target compatibility 保持 1_8 更安全

```javascript
"compileOptions {\n        targetCompatibility JavaVersion.VERSION_1_8\n        sourceCompatibility JavaVersion.VERSION_1_8\n    }\n\n    defaultConfig {"
```

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `scripts/post-cap-sync.mjs` | 1. 修正 kotlin-android 插件添加逻辑（兼容新旧 DSL）<br>2. 删除 kotlinOptions 步骤，改用顶层 `kotlin { compilerOptions {} }`<br>3. compileOptions 版本回退到 VERSION_1_8 |

## 验证方式
1. 本地运行 `cd app/encv-mobile && npx cap sync android` 确认脚本执行无报错
2. 检查生成的 `android/app/build.gradle` 中 `plugins {}` 包含 `id 'kotlin-android'`
3. 检查生成的 `build.gradle` 末尾有 `kotlin { compilerOptions { jvmTarget = "17" } }`
4. TypeScript 编译：`npx vue-tsc --noEmit`
