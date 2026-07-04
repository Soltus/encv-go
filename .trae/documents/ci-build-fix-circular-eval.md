# CI 构建修复：aar2apk signing 循环引用 + MPV 扩展未构建

## 问题分析

### 错误 1：循环引用导致整个 Gradle 构建崩溃

```
Circular evaluation detected: property 'keystorePath'
   -> property 'keystorePath'
```

**根因**：`build.gradle.kts` 中顶层变量名 `keystorePath` / `keystorePassword` / `keyAlias` / `keyPassword` 与 aar2apk DSL `signing { }` 块内的同名属性冲突。

在 `signing { keystorePath.set(keystorePath) }` 中：
- `keystorePath`（左侧，DSL 属性）= aar2apk signing 的 `Property<String>` 对象
- `keystorePath`（右侧，本意引用局部变量）= 在 DSL 作用域内被解析为同一个 `Property<String>` 对象
- 结果：`property.set(property)` → Gradle 检测到循环引用

**影响范围**：这个循环引用不仅导致 `convert_plugin-mpv-player_debug` 失败，还导致 `assembleDebug` 也失败（`BUILD FAILED in 2s`）。整个 Gradle 构建崩溃，**Debug APK 和 MPV 扩展 APK 都没有生成**。

### 错误 2：CI 工作流吞掉了错误，没有中止

步骤 27（Package MPV plugin as APK）使用了：
- `|| echo "⚠️ Plugin APK packaging skipped"` — 把 Gradle 失败吞掉
- `continue-on-error: true` — 步骤标记为成功，CI 继续运行

这导致 MPV 扩展构建失败但 CI 不报错，后续步骤继续运行在错误状态上。

## Keystore 路径验证

CI 中 keystore 文件路径验证：
- CI 步骤 22 在 `app/encv-mobile/keystore/release.jks` 创建/缓存 keystore（密码 `encv2025`，别名 `encvrelease`）
- `build.gradle.kts` 默认路径 `rootProject.file("../keystore/release.jks")` 中 `rootProject` = `app/encv-mobile/android/`
- 解析后 = `app/encv-mobile/keystore/release.jks` ✅ 路径正确

## 修复步骤

### 1. 修复 build.gradle.kts 中的循环引用

文件：`app/encv-mobile/android/build.gradle.kts`

重命名局部变量，避免和 DSL 属性名冲突：

```kotlin
val ksPath = localProps.getProperty("aar2apk.keystorePath")
    ?: System.getenv("AAR2APK_KEYSTORE_PATH")
    ?: rootProject.file("../keystore/release.jks").absolutePath
val ksPassword = localProps.getProperty("aar2apk.keystorePassword")
    ?: System.getenv("AAR2APK_KEYSTORE_PASSWORD")
    ?: "encv2025"
val ksAlias = localProps.getProperty("aar2apk.keyAlias")
    ?: System.getenv("AAR2APK_KEY_ALIAS")
    ?: "encvrelease"
val ksKeyPassword = localProps.getProperty("aar2apk.keyPassword")
    ?: System.getenv("AAR2APK_KEY_PASSWORD")
    ?: "encv2025"

aar2apk {
    modules {
        module(":plugin-mpv-player")
    }
    signing {
        keystorePath.set(ksPath)
        keystorePassword.set(ksPassword)
        keyAlias.set(ksAlias)
        keyPassword.set(ksKeyPassword)
    }
}
```

### 2. 修复 CI 工作流：步骤 27 不应吞掉错误

文件：`.github/workflows/android.yml`

当前步骤 27 的问题：
- `|| echo "⚠️ Plugin APK packaging skipped"` 掩盖了真正的构建失败
- `continue-on-error: true` 让 CI 继续运行

修改策略：去掉 `|| echo` 和 `continue-on-error`，让 Gradle 失败直接中止 CI。但保留 `continue-on-error: true` 以防 aar2apk 插件本身有兼容性问题（首次集成），改为在步骤内检测失败并输出明确的错误信息：

```yaml
      - name: Package MPV plugin as APK (debug)
        continue-on-error: true
        run: |
          cd app/encv-mobile/android
          if ! ./gradlew convert_plugin-mpv-player_debug --stacktrace 2>&1; then
            echo "::error::MPV plugin APK packaging failed! Check the Gradle output above for details."
            exit 1
          fi
          
          PLUGIN_APK=$(find build -name "plugin-mpv-player-debug.apk" -type f 2>/dev/null | head -1)
          if [ -n "$PLUGIN_APK" ] && [ -f "$PLUGIN_APK" ]; then
            echo "✅ Plugin APK generated:"
            ls -lh "$PLUGIN_APK"
            mkdir -p app/src/main/assets/plugins
            cp "$PLUGIN_APK" app/src/main/assets/plugins/mpv-player.apk
            echo "✅ Copied to host assets/plugins/"
          else
            echo "::warning::Plugin APK not found after successful build"
            find build/outputs -name "*.apk" 2>/dev/null | head -5 || echo "No APKs found"
          fi
```

关键改动：
- 用 `if ! ./gradlew ...; then echo "::error::" ; exit 1; fi` 替代 `|| echo "⚠️"`
- `::error::` 会在 GitHub Actions UI 中显示为红色错误标记
- `exit 1` 确保步骤失败（配合 `continue-on-error: true` 不会中止整个 workflow，但会标记为失败）

### 3. 清理日志文件

删除 `job_logs/` 目录和 `job_logs.zip`。

## 影响范围

- 修改 2 个文件：
  1. `app/encv-mobile/android/build.gradle.kts` — 4 个局部变量重命名
  2. `.github/workflows/android.yml` — 步骤 27 错误处理改进
- 风险：极低
