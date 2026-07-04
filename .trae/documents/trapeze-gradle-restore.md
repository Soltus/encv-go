# 计划：恢复 Trapeze Gradle 操作（精细匹配方案）

## 问题分析

上一轮 CI 失败的根本原因不是"Trapeze 精细匹配脆弱"，而是 **trapeze.yaml 中的 `target` 路径与 Capacitor 8 生成的 build.gradle 实际结构不匹配**：

1. Capacitor 8 的 `app/build.gradle` 使用 `apply plugin: 'com.android.application'` 旧格式，**没有 `plugins {}` 块**
2. Trapeze 的 `insertProperties`/`insertFragment` 在找不到 `target` 节点时直接抛错，不会自动创建

### Trapeze 源码验证

`gradle-file.js` L141-142:
```js
if (!found.length) {
    throw new Error('Unable to find method in Gradle file to inject');
}
```

**Trapeze 不支持自动创建不存在的块**。但我们可以：
1. 先用 `insertFragment` 在根节点注入 `plugins { id 'kotlin-android' }` 块
2. 或者用 `insertFragment` 在 `buildscript.dependencies` 中注入 classpath

### Capacitor 8 是否支持 `plugins {}` DSL？

查看 Capacitor 8 的 [android-template/app/build.gradle](https://raw.githubusercontent.com/ionic-team/capacitor/main/android-template/app/build.gradle)：
- 仍使用 `apply plugin:` 格式
- 没有 `plugins {}` 块
- 但 Gradle 允许两种格式共存（`plugins {}` + `apply plugin:`）

**结论**：可以在 Capacitor 生成的 build.gradle 中添加 `plugins {}` 块，不会冲突。

## 实施方案

### 核心策略：两阶段 Gradle 配置

1. **预注入阶段**（sync-native.mjs）：用简单的字符串操作注入 `plugins {}` 块和 `buildscript.dependencies` classpath，确保 Trapeze 能找到这些节点
2. **Trapeze 阶段**（trapeze.yaml）：利用 Trapeze 的精细匹配完成所有其他 Gradle 操作

### Step 1: sync-native.mjs 增加 Gradle 预注入

在 `sync-native.mjs` 的 Gradle patching 部分，只做两件事：

```js
// 1. Root build.gradle: 注入 kotlin-gradle-plugin classpath（如果不存在）
patchFile(join(ANDROID_DIR, 'build.gradle'), (c) => {
  if (!c.includes('kotlin-gradle-plugin')) {
    c = c.replace(
      /dependencies\s*\{/,
      "dependencies {\n        classpath \"org.jetbrains.kotlin:kotlin-gradle-plugin:2.1.0\""
    )
  }
  if (!c.includes('jitpack.io')) {
    c = c.replace(
      /allprojects\s*\{\s*repositories\s*\{/,
      "allprojects {\n    repositories {\n        maven { url 'https://jitpack.io' }"
    )
  }
  return c
})

// 2. App build.gradle: 注入 plugins {} 块（如果不存在）
patchFile(join(ANDROID_DIR, 'app', 'build.gradle'), (c) => {
  if (!c.includes('kotlin-android') && !c.includes('org.jetbrains.kotlin.android')) {
    c = "plugins {\n    id 'kotlin-android'\n}\n\n" + c
  }
  return c
})
```

### Step 2: trapeze.yaml 恢复 Gradle 操作

现在 `plugins {}` 块已存在，Trapeze 可以找到目标节点：

```yaml
platforms:
  android:
    gradle:
      # plugins 块现在已存在（由 sync-native.mjs 预注入）
      - file: app/build.gradle
        target:
          plugins:
        insert: |
          id 'kotlin-android'

      # dependencies 注入
      - file: app/build.gradle
        target:
          dependencies:
        insert: |
          implementation "org.jetbrains.kotlin:kotlin-stdlib:2.1.0"
          debugImplementation 'com.github.getActivity:Logcat:13.0'

      # android.defaultConfig 注入
      - file: app/build.gradle
        target:
          android:
            defaultConfig:
        insert: |
          buildConfigField "boolean", "USE_LYNX_PLAYER", "true"
          ndk {
              abiFilters 'arm64-v8a'
          }

      # android 块注入
      - file: app/build.gradle
        target:
          android:
        insert: |
          compileOptions {
              targetCompatibility JavaVersion.VERSION_21
              sourceCompatibility JavaVersion.VERSION_21
          }
          buildFeatures {
              buildConfig = true
          }

      # ... 其他操作

    copy:
      # ... 文件拷贝操作（不变）
```

### Step 3: sync-native.mjs 移除其余 Gradle patching

只保留 `plugins {}` 预注入和 `buildscript.dependencies` classpath 注入，其余全部交给 Trapeze。

### Step 4: 验证

1. `cap add android` → 生成原始 build.gradle
2. `sync-native.mjs` → 预注入 `plugins {}` + classpath
3. `trapeze --diff` → 预览所有 Gradle 修改
4. `trapeze` → 执行修改

## 关键变更文件

| 文件 | 变更 |
|------|------|
| `trapeze.yaml` | 恢复所有 `gradle` 操作（之前被移除） |
| `scripts/sync-native.mjs` | Gradle patching 精简为只做预注入（plugins + classpath） |

## 风险评估

- **`plugins {}` 与 `apply plugin:` 共存**：Gradle 官方支持，无风险
- **Trapeze 重复注入**：`insertFragment` 每次运行都会注入，需确保 `sync-native.mjs` 的预注入只在节点不存在时执行（已有 `includes()` 检查）
- **Trapeze 幂等性**：Trapeze 的 `insert` 操作**不是幂等的**（每次运行都会追加），但 `sync-native.mjs` 的预注入是幂等的。解决方案：CI 每次从 `cap add android` 开始，生成全新文件，不存在重复问题
