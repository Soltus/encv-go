# 诊断并修复：GoProcessPlugin 仍未编译进 APK

## 已确认事实

| 项 | 状态 |
|---|------|
| post-cap-sync.mjs 执行 | ✅ 有 overlay 日志 |
| MainActivity.kt 复制 | ✅ |
| GoProcessPlugin.kt 复制 | ✅ |
| 类声明验证通过 | ✅ (只有 1 个) |
| Gradle 构建成功 | ✅ (APK 14MB) |
| **GoProcessPlugin.class 在 APK 中** | ❌ 不存在 |

## 推测根因

**最可能：Gradle 缓存/增量编译导致 Kotlin 编译被跳过**

`cap sync android` 每次运行时可能不会完全清理旧的构建产物。如果之前某次构建时 `.kt` 文件不存在，Gradle 的增量编译缓存会标记这些文件为"不需要重新编译"。即使后来文件被复制了，Gradle 也可能跳过它们。

### 验证方法

在 CI 的构建步骤前添加诊断：
```bash
# 1. 确认源文件存在
echo "=== Kotlin source files ==="
find app/encv-mobile/android/app/src/main/java -name "*.kt" -exec echo "FOUND: {}" \;

# 2. 强制清理后构建（关键！）
cd app/encv-mobile/android && ./gradlew clean && ./gradlew assembleDebug

# 3. 构建后检查编译输出
echo "=== Compiled classes ==="
find app/encv-mobile/android/app/build/tmp/kotlin-classes -name "GoProcessPlugin*" -exec echo "COMPILED: {}" \;
```

## 修复方案

### 文件：`.github/workflows/android.yml`

在构建步骤中添加 `./gradlew clean`：

```yaml
- name: Build DEBUG APK
  run: |
    cd app/encv-mobile/android
    chmod +x gradlew
    # 强制清理，确保 Kotlin 源文件变更被检测到
    ./gradlew clean
    ./gradlew assembleDebug
```

### 文件：`scripts/post-cap-sync.mjs`（可选增强）

在 overlay 复制后添加诊断日志：

```javascript
// 验证文件内容不为空
for (const f of ['MainActivity.kt', 'GoProcessPlugin.kt']) {
  const dest = join(JAVA_DIR, f)
  if (existsSync(dest)) {
    const stat = statSync(dest)
    console.log(`  ${f}: ${stat.size} bytes ✓`)
  }
}
```

## 为什么需要 clean

Capacitor 的 `cap sync` 会生成/覆盖 Android 项目文件，但不执行 `gradle clean`。这导致：

1. 第一次构建：无 .kt 文件 → Gradle 记录"无需编译 Kotlin"
2. post-cap-sync 复制 .kt 文件
3. 第二次构建：Gradle 增量编译 → 检查缓存 → 认为 Kotlin 无变化 → 跳过编译

`./gradlew clean` 清除所有构建缓存，强制完整重新编译。

## 验证标准

1. CI 日志显示 `./gradlew clean` 执行成功
2. APK 中包含 `GoProcessPlugin.class`
3. logcat 显示 `ENCV-go` tag 日志
4. GoProcess 插件不再返回 UNIMPLEMENTED
