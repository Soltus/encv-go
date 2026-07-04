# Tasks

- [x] Task 1: CI workflow 移除 cap copy 的 debug-only 限制
  - [x] 修改 `.github/workflows/android.yml` 第176-183行：移除 "Copy web assets to Android project" 步骤的 `if: inputs.version == ''` 条件
  - [x] 更新步骤注释：说明 release 构建也需要 cap copy（因为已改用 gradlew 直接构建，不再有 cap build 的隐式 sync）
  - [x] 验证：release 构建路径不再跳过此步骤

- [x] Task 2: 删除 android-overlay/ 目录（过期产物）
  - [x] 删除整个 `app/encv-mobile/android-overlay/` 目录
  - [x] 删除 `app/encv-mobile/scripts/post-cap-sync.mjs`（唯一消费者，引用 overlay 作为 Kotlin 源）
  - [x] 验证：`grep -r "config.mobile" app/encv-mobile/` 应无结果（幽灵引用彻底清除）

- [x] Task 3: 在 sync-native.mjs 中添加 config.user.json 复制逻辑
  - [x] 在 `scripts/sync-native.mjs` 末尾添加：从项目根目录 `../../../config.user.json` 复制到 `android/app/src/main/assets/config.user.json`
  - [x] 验证：执行 `cd app/encv-mobile && node scripts/sync-native.mjs` 后 `android/app/src/main/assets/config.user.json` 存在

- [x] Task 4: CI 添加 APK 内关键资源存在性验证
  - [x] 在 "Verify APK contents" 步骤中添加：
    - 检查 `public/index.html` 存在：`unzip -l "$APK_PATH" | grep "public/index.html"`，缺失则 error + exit 1
    - 检查 `config.user.json` 存在：`unzip -l "$APK_PATH" | grep "config.user.json"`，缺失则 warning（首次启动会 fallback 生成）

# Task Dependencies
- [Task 2] 无依赖，可并行执行 ✅
- [Task 3] 无依赖，可并行执行 ✅
- [Task 4] 依赖 [Task 1]（验证在 cap copy 添加后才有效）✅
