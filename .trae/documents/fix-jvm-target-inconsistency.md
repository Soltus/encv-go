# 修复 compileDebugKotlin JVM target 不一致错误

## 问题
```
Inconsistent JVM-target compatibility detected
for tasks 'compileDebugJavaWithJavac' (21) and 'compileDebugKotlin' (17)
```

Capacitor 默认使用 **Java 21**，但我们注入的 `kotlinOptions.jvmTarget` 是 **17**，导致不一致。

## 修复方案

### 文件：`app/encv-mobile/scripts/post-cap-sync.mjs`

将步骤 7 的 `kotlinOptions.jvmTarget` 从 `"17"` 改为 `"21"`，与 Capacitor 默认的 Java 版本保持一致。

同时将 `compileOptions` 从 `VERSION_1_8` 升级到 `VERSION_21`（保持一致）：

```groovy
// compileOptions: VERSION_1_8 → VERSION_21
compileOptions {
    targetCompatibility JavaVersion.VERSION_21
    sourceCompatibility JavaVersion.VERSION_21
}

// kotlinOptions: "17" → "21"
tasks.withType(org.jetbrains.kotlin.gradle.tasks.KotlinCompile).configureEach {
    kotlinOptions {
        jvmTarget = "21"
    }
}
```

## 验证
- CI 构建通过（不再有 JVM-target 不一致错误）
- APK 中包含 `GoProcessPlugin.class`
- logcat 搜索 `ENCV-go` 能看到诊断日志
- GoProcess 插件方法不再返回 UNIMPLEMENTED
