# 彻底修复 MainActivity 重复声明 + GoProcessPlugin 编译错误

## 问题根因

### 错误 1: `checkPermissions` 需要 override (已修)
- `GoProcessPlugin.checkPermissions()` 隐藏了父类 `Plugin.checkPermissions()` 方法
- 修复：添加 `override` 修饰符 ✅

### 错误 2: MainActivity 重复声明 (本次重点)
```
e: MainActivity.kt:14:7 Redeclaration:
   class MainActivity : BridgeActivity
   class MainActivity : BridgeActivity
```

**根因推测**：`rm -f` 可能因路径/缓存问题未生效，或 `cap sync` 后续步骤重新生成

## 彻底修复方案

### 文件: `.github/workflows/android.yml`

将简单的 `rm -f` 替换为**三重保险删除策略**：

```bash
# 三重保险：删除所有可能的默认 MainActivity
find app/encv-mobile/android -name "MainActivity.kt" -type f -delete 2>/dev/null || true
rm -rf app/encv-mobile/android/app/src/main/java/com/encvgo/app/
mkdir -p app/encv-mobile/android/app/src/main/java/com/encvgo/app
# 复制自定义文件
cp ...MainActivity.kt ...
cp ...GoProcessPlugin.kt ...

# 验证：确保只有一个 MainActivity 类声明
grep -c "class MainActivity" app/encv-mobile/android/app/src/main/java/com/encvgo/app/MainActivity.kt
```

### 关键改进点

| 改进 | 说明 |
|------|------|
| `find ... -delete` | 删除 android 目录下**所有** MainActivity.kt，不遗漏 |
| `rm -rf` 整个包目录 | 连同可能存在的缓存/编译产物一起删 |
| 事后验证 | `grep -c` 确认只有一个类声明，构建前就暴露问题 |

## 验证标准

1. CI 构建通过，无 Kotlin 编译错误
2. APK 中包含 `GoProcessPlugin.class`
3. logcat 搜索 `ENCV-go` 能看到诊断日志
4. GoProcess 插件方法正常工作（不再 UNIMPLEMENTED）
