# Tasks

- [x] Task 1: 在 build.gradle.kts 中添加签名配置
  - [x] SubTask 1.1: 添加 `keystoreProperties` 加载逻辑（带 `exists()` 守卫）
  - [x] SubTask 1.2: 添加 `signingConfigs` 块（带 `containsKey("storeFile")` 守卫）
  - [x] SubTask 1.3: 在 `buildTypes.release` 中添加 `signingConfig = signingConfigs.findByName("release")`

- [x] Task 2: 修改 CI 工作流
  - [x] SubTask 2.1: 构建前动态生成 `keystore.properties` 文件
  - [x] SubTask 2.2: 使用 `./gradlew assembleRelease` 替代 `npx cap build --keystorepath`
  - [x] SubTask 2.3: `apksigner verify` 失败时 `exit 1`（防回归）

- [x] Task 3: 清理 post-cap-sync.mjs
  - [x] SubTask 3.1: 移除签名注入逻辑（step 8）
  - [x] SubTask 3.2: Kotlin 版本 `2.1.0` → `2.3.21`（4 处）

- [x] Task 4: 添加 .gitignore 规则
  - [x] SubTask 4.1: `keystore.properties` 加入 `.gitignore`

- [ ] Task 5: CI 验证
  - [ ] SubTask 5.1: 触发 CI 构建，确认 `apksigner verify` 通过

# Task Dependencies

- Task 2 依赖 Task 1
- Task 5 依赖所有前置任务
