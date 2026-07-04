# 修复 CI packageDebug OOM 失败

## 根本原因

最新 CI 日志显示 **Kotlin 编译已通过**（`compileDebugKotlin` 成功），但 `packageDebug` 阶段因 **`java.lang.OutOfMemoryError: Java heap space`** 失败。

原因：jniLibs 包含 4 个 ABI 的 mpv .so 文件（~22M），加上 Lynx SDK .so + encv-go .so（23M），总计 ~45M+ native 库。ApkFlinger 在打包时 OOM。

## 执行计划

### 1. 只保留 arm64-v8a 的 mpv .so 文件

**文件**: `scripts/setup-mpv-libs.sh`

当前脚本提取 4 个 ABI（arm64-v8a, armeabi-v7a, x86, x86_64），但 build.gradle 只配置了 `abiFilters 'arm64-v8a'`。其他 ABI 的 .so 文件白白占用空间并增加打包内存。

**修改**: 只提取 arm64-v8a

### 2. 增加 Gradle 堆内存

**文件**: `android-overlay/gradle.properties`

添加 `org.gradle.jvmargs=-Xmx4g` 增加 Gradle JVM 堆内存到 4GB（GitHub Actions runner 有 7GB RAM）。

### 3. post-cap-sync.mjs 中只复制 arm64-v8a 的 jniLibs

**文件**: `scripts/post-cap-sync.mjs`

当前代码遍历 overlay jniLibs 的所有 ABI 子目录并复制。由于 setup-mpv-libs.sh 改为只提取 arm64-v8a，这里自然只会复制 arm64-v8a。但为了安全，也加一个 ABI 过滤。

### 4. EncvApplication.kt 服务注册优化（非阻塞）

**文件**: `android-overlay/app/src/main/java/com/encvgo/app/EncvApplication.kt`

`LynxDevToolService()` 当前调用类构造函数，虽然能编译通过，但 Lynx 官方推荐 `LynxDevToolService.INSTANCE`。改为官方推荐方式更稳健。
