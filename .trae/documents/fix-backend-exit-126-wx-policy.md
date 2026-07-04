# 修复 Android 10+ W^X 策略导致后端 exit code 126

## 问题根因

**Android 10+ 的 W^X（Write XOR Execute）安全策略**禁止从 app 的可写目录（`filesDir`、`cacheDir`、`externalFilesDir`）执行二进制文件。

当前流程：
1. Go binary 打包在 `assets/encv-go`
2. 运行时从 assets 复制到 `filesDir`（`/data/data/com.encvgo.app/files/encv-go`）
3. `setExecutable(true)` 设置权限
4. 通过 `ProcessBuilder("/system/bin/sh", "-c", binary.absolutePath)` 执行

**问题**：即使 `chmod +x` 成功，SELinux/W^X 策略仍然阻止从 `/data/data/` 执行。`sh -c` 调用 `execve()` 时返回 EACCES，shell 报告 exit code 126。

## 解决方案

将 Go binary 作为 **JNI native library** 打包（命名为 `libencv-go.so`），放在 APK 的 `jniLibs/arm64-v8a/` 目录下。Android 安装 APK 时会自动将 native libs 提取到 `/data/app/com.encvgo.app/lib/arm64/` 目录，该目录允许执行。

通过 `applicationInfo.nativeLibraryDir` 获取 native lib 的路径，直接执行。

### 为什么这能工作

- Android 系统将 APK 中的 `lib/*/lib*.so` 文件视为合法的 native code
- 安装时提取到 `/data/app/<pkg>/lib/arm64/`，该目录受 SELinux 保护且允许执行
- `android:extractNativeLibs="true"` 确保文件被提取（而非直接从 APK 中加载）
- 不需要 `sh -c`，可以直接 `exec()` 执行

## 修复步骤

### Step 1：修改 CI workflow — 将 Go binary 放入 jniLibs

修改 `.github/workflows/android.yml` 中的 "Copy assets" 步骤：

```yaml
- name: Copy assets, overlay files, and release config
  run: |
    mkdir -p app/encv-mobile/android/app/src/main/assets
    # Go binary as JNI native library (not in assets)
    mkdir -p app/encv-mobile/android/app/src/main/jniLibs/arm64-v8a
    cp app/encv-mobile/encv-go-arm64 app/encv-mobile/android/app/src/main/jniLibs/arm64-v8a/libencv-go.so
    # Config file still in assets
    cp app/encv-mobile/assets/config.mobile.json app/encv-mobile/android/app/src/main/assets/config.mobile.json
    ...
```

关键点：
- 文件名必须以 `lib` 开头、`.so` 结尾（Android 要求的 JNI lib 命名规范）
- 目录结构 `jniLibs/<abi>/lib*.so`
- 不再复制到 `assets/` 目录

### Step 2：修改 AndroidManifest.xml — 添加 extractNativeLibs

在 `<application>` 标签上添加 `android:extractNativeLibs="true"`：

```xml
<application
    android:allowBackup="true"
    android:extractNativeLibs="true"
    android:icon="@mipmap/ic_launcher"
    ...>
```

这确保 Android 在安装 APK 时将 `.so` 文件提取到文件系统（而非保持在 APK 内），从而可以被执行。

### Step 3：修改 EncvGoService.kt — 使用 nativeLibraryDir

重写 `findExecutableBinary()` 方法：

```kotlin
private fun findExecutableBinary(): File? {
    // 优先使用 nativeLibraryDir（Android 10+ W^X 兼容）
    val nativeLibDir = File(applicationInfo.nativeLibraryDir)
    val nativeBinary = File(nativeLibDir, "libencv-go.so")
    if (nativeBinary.exists() && nativeBinary.canExecute()) {
        Log.i(TAG, "Using binary from nativeLibraryDir: ${nativeBinary.absolutePath}")
        return nativeBinary
    }

    // 降级：从 assets 复制到 filesDir（仅适用于 Android 9 及以下）
    val candidateDirs = listOf(
        filesDir to "filesDir",
        cacheDir to "cacheDir",
        getExternalFilesDir(null) to "externalFilesDir",
    )
    for ((dir, name) in candidateDirs) {
        if (dir == null) continue
        val binary = File(dir, BINARY_NAME)
        if (!binary.exists()) {
            copyBinaryFromAssets(binary)
        }
        binary.setReadable(true)
        binary.setExecutable(true)
        binary.setWritable(true)
        if (binary.canExecute()) {
            Log.i(TAG, "Using binary from $name: ${binary.absolutePath}")
            return binary
        }
    }
    return null
}
```

### Step 4：修改 startGoProcess — 直接执行而非 sh -c

```kotlin
val shellCommand = "${binary.absolutePath} start"
Log.i(TAG, "Starting backend: $shellCommand")

goProcess = ProcessBuilder(binary.absolutePath, "start").apply {
    environment()["ENCV_CONFIG_PATH"] = configPath
    environment()["ENCV_MOBILE"] = "1"
    environment()["HOME"] = filesDir.absolutePath
    redirectErrorStream(true)
    directory(filesDir)
}.start()
```

关键改动：
- `ProcessBuilder(binary.absolutePath, "start")` 替代 `ProcessBuilder("/system/bin/sh", "-c", shellCommand)`
- 直接执行 binary，不经过 shell，避免 shell 对 exit code 的包装
- nativeLibraryDir 下的文件本身就有执行权限，不需要 `sh -c`

### Step 5：修改 post-cap-sync.mjs — 添加 jniLibs 目录

在 post-cap-sync 脚本中，确保 `jniLibs` 目录不被 cap sync 清除：

```javascript
// 确保 jniLibs 目录存在
const jniLibsDir = join(ANDROID_DIR, 'app', 'src', 'main', 'jniLibs', 'arm64-v8a')
mkdirSync(jniLibsDir, { recursive: true })
console.log(`  ensured jniLibs/arm64-v8a directory exists`)
```

注意：实际的 `.so` 文件在 CI 中复制，不在 post-cap-sync 中。这里只是确保目录结构存在。

### Step 6：修改 app/build.gradle — 确保 extractNativeLibs 生效

在 `post-cap-sync.mjs` 的 build.gradle 补丁中，添加 `packagingOptions` 确保 `.so` 文件不被压缩：

```javascript
if (!c.includes('jniLibs')) {
    c = c.replace(
        /android\s*\{/,
        "android {\n    sourceSets {\n        main {\n            jniLibs.srcDirs = ['src/main/jniLibs']\n        }\n    }"
    )
}
```

### Step 7：验证

- 确认 `go build ./cmd/encv/` 编译通过
- 确认 `jniLibs/arm64-v8a/libencv-go.so` 在 CI 中正确创建
- 确认 `applicationInfo.nativeLibraryDir` 在 Android 10+ 上返回有效路径
