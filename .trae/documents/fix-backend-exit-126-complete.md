# 修复 Android 后端 exit code 126 — 完整方案

## 问题现状

授予通知权限后，后端依旧启动失败，logcat 显示 `Backend exited with code: 126`。

Exit code 126 = "command found but not executable"，即二进制文件被找到但无法执行。

## 根因分析

之前的方案（Steps 1-4）已将 Go binary 作为 `libencv-go.so` 打包到 `jniLibs/arm64-v8a/`，并通过 `nativeLibraryDir` 查找执行。但仍有以下未完成/遗漏的问题：

### 关键遗漏 1：AGP 8.0+ 的 `useLegacyPackaging` 默认值

**AGP 8.0+ 默认 `useLegacyPackaging=false`**，这意味着 `.so` 文件以未压缩方式存储在 APK 中，安装时 **不会提取到 `nativeLibraryDir`**。即使 manifest 声明了 `extractNativeLibs="true"`，Gradle 的 `useLegacyPackaging=false` 会覆盖它。

结果：`applicationInfo.nativeLibraryDir` 下没有 `libencv-go.so`，`findExecutableBinary()` 的首选路径失败，降级到 `filesDir` 降级路径，仍然被 W^X 阻止 → exit 126。

**修复**：在 `build.gradle` 中显式设置 `useLegacyPackaging = true`。

### 关键遗漏 2：`post-cap-sync.mjs` 未处理 jniLibs

`cap sync` 会重新生成 Android 项目，可能清除手动创建的 `jniLibs` 目录。虽然 CI 在 sync 之后复制 `.so` 文件，但 `post-cap-sync.mjs` 作为 afterSync 钩子应确保目录结构存在，并在 `build.gradle` 中声明 `jniLibs.srcDirs`。

### 关键遗漏 3：CI 验证步骤过时

`Verify APK contents` 步骤仍检查 `assets/encv-go`，而非 `lib/arm64-v8a/libencv-go.so`。无法验证 `.so` 文件是否正确打包。

### 关键遗漏 4：降级路径 `copyBinaryFromAssets()` 已失效

Go binary 不再放在 `assets/` 目录，`copyBinaryFromAssets()` 会抛异常。降级路径完全无效。

## 修复步骤

### Step 1：修改 `post-cap-sync.mjs` — 添加 jniLibs 目录和 build.gradle 补丁

在 `post-cap-sync.mjs` 中添加：

1. **确保 jniLibs 目录存在**（cap sync 后目录可能被清除）：
```javascript
const jniLibsDir = join(ANDROID_DIR, 'app', 'src', 'main', 'jniLibs', 'arm64-v8a')
mkdirSync(jniLibsDir, { recursive: true })
console.log('  ensured jniLibs/arm64-v8a directory exists')
```

2. **在 build.gradle 补丁中添加 `jniLibs.srcDirs`**：
```javascript
if (!c.includes('jniLibs.srcDirs')) {
    c = c.replace(
        /android\s*\{/,
        "android {\n    sourceSets {\n        main {\n            jniLibs.srcDirs = ['src/main/jniLibs']\n        }\n    }"
    )
}
```

3. **在 build.gradle 补丁中添加 `useLegacyPackaging = true`**（**最关键**）：
```javascript
if (!c.includes('useLegacyPackaging')) {
    c = c.replace(
        /android\s*\{/,
        "android {\n    packaging {\n        jniLibs {\n            useLegacyPackaging = true\n        }\n    }"
    )
}
```

### Step 2：修改 `EncvGoService.kt` — 增强日志和修复降级路径

1. **增强 `findExecutableBinary()` 的日志**，输出 `nativeLibraryDir` 路径和文件状态：
```kotlin
private fun findExecutableBinary(): File? {
    val nativeLibDir = applicationInfo.nativeLibraryDir
    Log.i(TAG, "nativeLibraryDir: $nativeLibDir")

    val nativeBinary = File(nativeLibDir, "libencv-go.so")
    Log.i(TAG, "Checking native binary: exists=${nativeBinary.exists()}, canExecute=${nativeBinary.canExecute()}, path=${nativeBinary.absolutePath}")

    if (nativeBinary.exists() && nativeBinary.canExecute()) {
        Log.i(TAG, "Using binary from nativeLibraryDir: ${nativeBinary.absolutePath}")
        return nativeBinary
    }

    // 如果 nativeLibraryDir 查找失败，列出目录内容用于诊断
    val libDir = File(nativeLibDir)
    if (libDir.exists()) {
        libDir.listFiles()?.forEach { f ->
            Log.i(TAG, "  lib dir entry: ${f.name} exe=${f.canExecute()}")
        }
    } else {
        Log.w(TAG, "nativeLibraryDir does not exist: $nativeLibDir")
    }

    // 降级路径（仅 Android 9 及以下有效）
    // ... 保持现有降级逻辑不变，但添加警告日志
    Log.w(TAG, "nativeLibraryDir lookup failed, falling back to filesDir (may fail on Android 10+)")
    ...
}
```

2. **修复 `copyBinaryFromAssets()` 的异常处理**：当前 `assets/encv-go` 不再存在，需要处理 `IOException`：
```kotlin
private fun copyBinaryFromAssets(dest: File) {
    dest.parentFile?.mkdirs()
    try {
        assets.open(BINARY_NAME).use { input ->
            FileOutputStream(dest).use { output ->
                val buffer = ByteArray(8192)
                var len: Int
                while (input.read(buffer).also { len = it } != -1) {
                    output.write(buffer, 0, len)
                }
            }
        }
    } catch (e: Exception) {
        Log.w(TAG, "Binary not found in assets (expected on Android 10+ with jniLibs packaging)", e)
    }
}
```

### Step 3：修改 CI workflow — 更新验证步骤

1. **更新 `Verify APK contents` 步骤**，检查 `lib/arm64-v8a/libencv-go.so` 而非 `assets/encv-go`：
```yaml
echo "=== Go binary in APK ==="
unzip -l "$APK_PATH" | grep -E "libencv-go|lib/arm64" || echo "❌ libencv-go.so NOT in APK!"
```

2. **添加 `useLegacyPackaging` 验证**：
```yaml
echo "=== useLegacyPackaging ==="
grep -n "useLegacyPackaging" app/encv-mobile/android/app/build.gradle || echo "⚠️ WARNING: useLegacyPackaging not set!"
```

### Step 4：验证

1. 确认 `go build ./cmd/encv/` 编译通过
2. 确认 `post-cap-sync.mjs` 正确添加 `jniLibs.srcDirs` 和 `useLegacyPackaging`
3. 确认 CI 验证步骤检查 `lib/arm64-v8a/libencv-go.so`
4. 确认 `EncvGoService.kt` 有足够的诊断日志

## 文件变更清单

| 文件 | 变更 |
|------|------|
| `app/encv-mobile/scripts/post-cap-sync.mjs` | 添加 jniLibs 目录、`jniLibs.srcDirs`、`useLegacyPackaging` |
| `app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/EncvGoService.kt` | 增强日志、修复降级路径异常处理 |
| `.github/workflows/android.yml` | 更新验证步骤 |
