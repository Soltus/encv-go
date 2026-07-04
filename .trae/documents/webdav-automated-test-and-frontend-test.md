# WebDAV 自动化测试 + 前端测试连接 + Android Mock 测试

## 背景

WebDAV 服务反复出问题，之前已发现并修复了多个关键 bug（gin Any() 不含 PROPFIND、路由冲突、loggingResponseWriter 缺 Flusher 等）。Go 后端自动化测试已通过 10 个用例，但还需要：
1. 补全后端 test-local API（已写但未注册路由、缺 import）
2. 前端添加 WebDAV 测试连接按钮，让用户能直观看到诊断信息
3. Android 端 Mock 测试，验证 GoProcessPlugin 和 EncvGoService 的核心逻辑

## 步骤

### 步骤 1：补全后端 test-local API

**文件：`/workspace/internal/server/mobile_api.go`**
- 在 import 中添加 `"bytes"`（`handleTestLocalWebDAVGin` 使用了 `bytes.NewBufferString`）

**文件：`/workspace/internal/server/server.go`**
- 在路由注册区域添加：`r.GET("/api/webdav/test-local", s.handleTestLocalWebDAVGin)`

**验证：** `go build ./...`

### 步骤 2：前端 WebDAV 测试连接按钮

**文件：`/workspace/app/encv-mobile/src/api/encv.ts`**
- 添加 `testLocalWebDAV()` 函数，调用 `GET /api/webdav/test-local`
- 返回类型：`{ available: boolean; url?: string; authRequired?: boolean; details?: { propfindRoot: string; authWorks: string; dirReadable: string }; error?: string }`

**文件：`/workspace/app/encv-mobile/src/views/ServerDetail.vue`**
- 在 WebDAV 服务地址行（`serviceUrls` 中 webdav 项）旁边添加"测试连接"按钮
- 点击后调用 `testLocalWebDAV()`，显示结果 toast：
  - 成功：显示各检查项状态（propfindRoot/authWorks/dirReadable）
  - 失败：显示具体错误信息
- 测试中显示 loading 状态

**文件：`/workspace/app/encv-mobile/src/composables/useI18n.ts`**
- 添加 i18n 键：
  - `settings.testWebdavConnection` / `settings.webdavTestSuccess` / `settings.webdavTestFailed`
  - 中英文双语

**验证：** `vue-tsc --noEmit && vite build`

### 步骤 3：Android Mock 测试

**文件：`/workspace/app/encv-mobile/android/app/build.gradle`**
- 添加测试依赖：
  - `testImplementation "org.mockito:mockito-core:5.8.0"`
  - `testImplementation "org.mockito.kotlin:mockito-kotlin:5.2.1"`
  - `testImplementation "org.robolectric:robolectric:4.11.1"`（可选，用于 Android Context mock）
  - `testImplementation "androidx.test:core:1.5.0"`
  - `testImplementation "org.jetbrains.kotlin:kotlin-test:2.1.0"`

**新建：`/workspace/app/encv-mobile/android/app/src/test/java/com/encvgo/app/GoProcessPluginTest.kt`**
- 纯 JVM 单元测试（不需要 Android 设备）
- 测试用例：
  1. `test_getStatus_returnsRunningState` — 验证 getStatus 返回 EncvGoService.isRunning 和 lastKnownPort
  2. `test_stop_resolvesImmediately` — 验证 stop() 立即 resolve，不依赖 broadcast
  3. `test_isStandaloneMode_nonPlayerActivity` — 验证非 PlayerActivity 时 standalone=false
  4. `test_checkPermissions_allGranted` — 验证权限检查返回正确状态
  5. `test_setScreenOrientation_validValues` — 验证方向设置不抛异常

**新建：`/workspace/app/encv-mobile/android/app/src/test/java/com/encvgo/app/EncvGoServiceTest.kt`**
- 纯 JVM 单元测试
- 测试用例：
  1. `test_createIntent_containsCorrectAction` — 验证 createIntent 设置了正确的 action
  2. `test_createIntent_containsSource` — 验证 source extra
  3. `test_readConfigPort_validConfig` — 验证从 JSON 读取端口
  4. `test_readConfigPort_missingConfig` — 验证无配置时返回默认端口
  5. `test_checkHealth_invalidPort` — 验证无效端口返回 false
  6. `test_resetStateForStart_clearsState` — 验证状态重置
  7. `test_companionProperties_defaultValues` — 验证静态属性默认值

**新建：`/workspace/app/encv-mobile/android/app/src/test/java/com/encvgo/app/GoBackendModuleTest.kt`**
- 纯 JVM 单元测试
- 测试用例：
  1. `test_getStreamUrl_validPort` — 验证生成正确的 stream URL
  2. `test_getStreamUrl_invalidPort` — 验证端口无效时返回错误
  3. `test_getStreamUrl_external` — 验证 external stream URL 格式
  4. `test_companionEventConstants` — 验证事件常量值

**验证：** `cd android && ./gradlew testDebugUnitTest`

### 步骤 4：Go 后端 WebDAV test-local 集成测试

**文件：`/workspace/internal/server/webdav_test.go`**
- 添加测试用例：
  1. `TestWebDAV_TestLocalAPI_Enabled` — 验证 WebDAV 启用时 test-local API 返回 available=true，且 propfindRoot=ok
  2. `TestWebDAV_TestLocalAPI_Disabled` — 验证 WebDAV 未启用时返回 available=false
  3. `TestWebDAV_TestLocalAPI_AuthDetails` — 验证 authRequired 和 authWorks 详情

**验证：** `go test ./internal/server/ -run TestWebDAV_TestLocalAPI -v`

### 步骤 5：最终验证

1. `go build ./...` — Go 编译通过
2. `go test ./internal/server/ -v` — Go 测试全部通过
3. `cd app/encv-mobile && vue-tsc --noEmit && vite build` — 前端构建通过
4. `cd app/encv-mobile/android && ./gradlew testDebugUnitTest` — Android 单元测试通过

## 关键设计决策

1. **Android 测试用纯 JVM 单元测试而非 instrumented test**：GoProcessPlugin 和 EncvGoService 的核心逻辑（状态管理、配置读取、URL 生成）不依赖真实 Android 框架，用 Mockito mock Context 即可。Instrumented test 需要连接设备/模拟器，不适合 CI 自动化。

2. **前端测试按钮放在 ServerDetail.vue**：WebDAV 服务地址已经在此页面展示，测试按钮放在旁边最直观。不在 Settings.vue 的配置区域是因为配置区域是通用 schema 驱动的，不适合加特殊按钮。

3. **test-local API 返回详细诊断信息**：不仅返回成功/失败，还返回 propfindRoot/authWorks/dirReadable 三个子项状态，方便定位具体问题。
