# Tasks

- [ ] Task 1: 修复 CI 工作流中 APK 选择逻辑
  - [ ] SubTask 1.1: 在 `Build Release APK` 步骤中，将 `find ... -name "*signed*.apk"` 改为 `find ... -name "app-release-signed.apk"`，消除 `unsigned` 误匹配
  - [ ] SubTask 1.2: 移除 fallback 到 any APK 的逻辑——如果签名 APK 不存在则直接报错退出（`echo "::error::Signed APK not found"; exit 1`）
  - [ ] SubTask 1.3: 在 `Verify APK contents` 步骤中同样修复 APK 选择逻辑

# Task Dependencies

无
