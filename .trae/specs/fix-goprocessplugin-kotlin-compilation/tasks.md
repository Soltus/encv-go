# Tasks

- [x] Task 1: 修复 libs.versions.toml Kotlin 版本不一致
  - [x] SubTask 1.1: 将 `kotlin = "2.1.0"` 改为 `kotlin = "2.3.21"`，与 build.gradle.kts 一致

- [x] Task 2: 修复 GoProcessPlugin.kt 编译错误
  - [x] SubTask 2.1: 添加 `import android.content.BroadcastReceiver` 和 `import android.content.Context`
  - [x] SubTask 2.2: 合并两个 companion object，将 REQUEST_CODE_PLUGIN_PICK 移入主 companion object
  - [x] SubTask 2.3: 修复 nullable 安全调用（`path.isEmpty()` → `path.isNullOrEmpty()`，`path.removePrefix` → `path!!.removePrefix`）

- [ ] Task 3: CI 验证
  - [ ] SubTask 3.1: 触发 CI 构建，确认 Kotlin 编译通过
  - [ ] SubTask 3.2: 确认 Release APK 生成成功

# Task Dependencies

- Task 2 依赖 Task 1（Kotlin 版本统一后才能正确验证编译错误修复）
- Task 3 依赖 Task 1、2

# 备注

- 根因是半截升级：build.gradle.kts 升到 2.3.21 但 libs.versions.toml 漏了
- Kotlin 2.3.x 对 nullable 类型推断更严格，暴露了之前被宽松处理掩盖的类型安全问题
- job_logs.zip 和解压文件已清理
