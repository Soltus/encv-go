# 项目测试编排与双端交叉验证 Spec

## Why

当前项目测试基础设施存在以下缺口：

### 现有覆盖（已具备）

| 层 | 框架 | 测试数量 | 覆盖范围 |
|---|------|---------|----------|
| Go 后端单元 | `go test` | ~45 个文件 | crypto、container(v2/v4)、reader、writer、plugins、service、server、webdav、physical |
| 前端单元 | Vitest (jsdom) | 4 个文件 | composables、FilePickerModal、files.logic、api.mock |
| CI 自动化 | GitHub Actions | android.yml | `vitest run` + `go test ./internal/service/` |
| Android | Instrumented Test | 1 个模板 | ExampleInstrumentedTest（空壳） |

### 缺失覆盖（本 spec 补全）

| # | 缺失领域 | 严重程度 | 影响 |
|---|----------|----------|------|
| T1 | **GoProcessPlugin API 契约验证** | 🔴 P0 | Capacitor 方法签名变更时无编译期保障 |
| T2 | **ComboLite 集成正确性验证** | 🔴 P0 | 反射/直接API调用错误无法被自动检测 |
| T3 | **加密 E2E 全链路（v3+v4）** | 🟠 P1 | 加密→解密→验证的完整 roundtrip |
| T4 | **前端↔Go 后端 API 对接** | 🟠 P1 | 请求/响应格式变更时的回归 |
| T5 | **Android Native↔Web Bridge 链路** | 🟡 P2 | Capacitor plugin → Go backend 调用路径 |
| T6 | **插件安装全链路（ExtensionsPage→InstallerManager）** | 🟠 P1 | UI→Native→ComboLite 的跨层集成 |
| T7 | **视频播放器启动链路（PlayerEntry→ProxyManager→Activity）** | 🟡 P2 | MPV 插件 Activity 代理启动 |
| T8 | **配置文件双端一致性** | 🟡 P2 | config.user.json 解析行为在 Go/Vue 两侧一致 |

## What Changes

### 新增测试类型定义

#### 类型 A：Go 单元测试补充（T1, T2, T3）
- 文件位置：`internal/*_test.go`
- 运行方式：`go test ./... -count=1 -timeout 300s`
- CI 集成：扩展 android.yml 的 Go test 步骤

#### 类型 B：前端 Vitest 补充（T4, T6）
- 文件位置：`app/encv-mobile/__tests__/*.test.ts`
- 运行方式：`vitest run --reporter=verbose`
- CI 集成：已在 android.yml 中

#### 类型 C：Go↔Vue API 契约测试（T4 变体）
- 文件位置：新增 `tests/api_contract/` 目录
- 运行方式：Go server 启动 + Vitest 发送 HTTP 请求验证响应格式
- 或：OpenAPI schema 验证

#### 类型 D：Android Instrumented Test 补充（T5, T7）
- 文件位置：`app/encv-mobile/android/app/src/androidTest/java/com/encvgo/app/`
- 运行方式：`./gradlew connectedAndroidTest`（需要 emulator/device）
- CI 集成：需新增 workflow 或在现有 android.yml 中添加步骤

---

## ADDED Requirements

### Requirement T1: GoProcessPlugin API 契约表

系统 SHALL 维护一个 GoProcessPlugin 的方法签名契约表，确保：
- 每个 `@PluginMethod` 标注的方法签名（参数名/类型/返回值）与前端调用点匹配
- Capacitor Plugin 注册的方法名列表完整无误
- 回调机制（startActivityResult / BroadcastReceiver / pendingCalls）的类型安全

#### Scenario T1.1: 方法签名编译期检查
- **WHEN** GoProcessPlugin.kt 中任何 `@PluginMethod` 的参数类型或名称发生变化
- **THEN** 编译失败或测试明确报错（通过反射枚举所有 @PluginMethod 并对比预期签名表）

#### Scenario T1.2: pendingCalls 键值一致性
- **WHEN** installFromPath / installPlugin 存入 pendingCalls 的 key 与 handleOnActivityResult / onReceive 取出的 key 不匹配
- **THEN** 测试检测到泄漏（pendingCall 未被消费导致内存/逻辑问题）

### Requirement T2: ComboLite API 调用规范静态检查

系统 SHALL 通过 AST 分析或反射检测确保：

#### Scenario T2.1: 零反射调用 ComboLite
- **WHEN** 扫描所有 .kt 文件
- **THEN** 不存在 `Class.forName("com.combo.core.runtime")` 或 `.getMethod("getInstance"` 或 `.invoke(pm` 模式的代码
- **AND** 存在 `com.combo.core.runtime.PluginManager.isInitialized` 直接引用

#### Scenario T2.2: InstallerManager 访问路径正确
- **WHEN** 搜索 `installPlugin` 调用
- **THEN** 调用链为 `PluginManager.installerManager.installPlugin()` 而非 `PluginManager.installPlugin()`

#### Scenario T2.3: ProxyManager 已配置
- **WHEN** EncvApplication 初始化完成
- **THEN** `setHostActivity(EncvHostActivity::class.java)` 在 onFrameworkSetup 中被调用

### Requirement T3: 加密 E2E 全链路测试

系统 SHALL 对 v3 和 v4 容器分别执行加密→解密→验证的完整 roundtrip：

#### Scenario T3.1: v3 容器加密 roundtrip
- **WHEN** 使用一个小型测试 MP4 文件（<5MB）执行 v3 加密
- **THEN** 输出文件可被成功解密
- **AND** 解密后文件的 MD5/SHA256 与原始文件一致（或内容字节级比对）
- **AND** ffprobe 元数据提取容错不阻塞加密（即使 ffprobe 输出异常格式）

#### Scenario T3.2: v4 容器加密 roundtrip
- **WHEN** 使用同一个测试 MP4 文件执行 v4 加密
- **THEN** 输出文件可被成功解密
- **AND** verifyContainer 不再误报 "stsz box missing"（SkipStructCheck=true 生效）
- **AND** PostEncryptProcessor 场景下 isPostEncryptVerify 标志正确传递

#### Scenario T3.3: 加密后原始文件保留
- **WHEN** 加密完成
- **THEN** 原始输入文件仍然存在且未被修改（P0 文件丢失防护验证）

### Requirement T4: 前端↔Go 后端 API 对接验证

#### Scenario T4.1: 文件列表 API 契约
- **WHEN** 前端调用 `GET /api/files/list?path=xxx`
- **THEN** Go 后端返回符合前端期望格式的 JSON（字段名、嵌套结构、空值处理）

#### Scenario T4.2: 加密 API 契约
- **WHEN** 前端调用 `POST /api/files/encrypt`（含 containerType、password 等参数）
- **THEN** 后端接受参数并返回预期的 progress/event stream 格式

#### Scenario T4.3: 预览 URL 生成
- **WHEN** 前端调用 `getFilePreviewUrl('text.html', path)` 或 `getFilePreviewUrl('pdf.html', path)`
- **THEN** 生成的 URL 能被后端 preview handler 正确路由和渲染

### Requirement T6: 插件安装全链路（前端模拟）

由于真实 ComboLite 安装需要 Android 设备，前端测试 SHALL mock Native 层：

#### Scenario T6.1: ExtensionsPage 安装流程状态机
- **WHEN** 用户点击「选择 APK」→ 选择文件 → 等待确认 → 确认安装
- **THEN** 状态转换正确：idle → picking → confirming → installing → success/error
- **AND** 120s 超时处理正确（安装确认界面使用 BroadcastReceiver 后不再超时）

#### Scenario T6.2: InstallConfirmActivity 数据传递
- **WHEN** Intent 携带 EXTRA_APK_PATH 和 EXTRA_FILE_NAME
- **THEN** Activity 正确解析并显示文件信息（图标/名称/包名/大小）
- **AND** sendBroadcast 携带正确的 request_id 和 result_code

### Requirement T7: 视频播放器启动链路验证

#### Scenario T7.1: PlayerEntry MPV Activity 启动
- **WHEN** `startMpvPlayer(context, path)` 被调用且 MPV 插件已加载
- **THEN** Intent 的 component 正确指向 `com.encvgo.plugin.mpv.MpvPlayerActivity`
- **AND** ProxyManager 将其路由到 EncvHostActivity

#### Scenario T7.2: ArtPlayer fallback
- **WHEN** MPV 插件未加载或 ActivityNotFoundException
- **THEN** 正确 fallback 到 ArtPlayer（WebView 播放器）

---

## MODIFIED Requirements

### Requirement TC1: CI 测试矩阵扩展

CI workflow SHALL 从当前的 2 步扩展为分层矩阵：

```
┌─────────────────────────────────┐
│  Layer 1: 快速反馈 (<5min)     │  ← 每次 PR 触发
│  ├─ Frontend: vitest run       │     (jsdom, 无浏览器)
│  ├─ Go: go test ./internal/v2/  │     (核心逻辑)
│  └─ Go: ComboLite 静态检查      │     (反射扫描)
├─────────────────────────────────┤
│  Layer 2: 完整回归 (<15min)      │  ← main 分支 + PR merge
│  ├─ Go: go test ./...          │     (全量后端)
│  ├─ Frontend: vitest --coverage │     (覆盖率报告)
│  └─ API 契约: handler_test      │     (HTTP 接口)
├─────────────────────────────────┤
│  Layer 3: 集成/E2E (>15min)     │  ← nightly / release tag
│  ├─ 加密 E2E roundtrip          │     (v3 + v4)
│  ├─ Android instrumented test   │     (需要 emulator)
│  └─ 双端配置一致性              │     (config.user.json)
└─────────────────────────────────┘
```

### Requirement TC3: 专用测试 CI 工作流文件

系统 SHALL 创建 `.github/workflows/test.yml` 作为**专门的完整测试工作流**（区别于 `android.yml` 的构建导向），包含以下特性：

#### TC3.1: 触发条件
- **手动触发**：`workflow_dispatch`（可指定 branch / 跳过特定层）
- **PR 触发**：`pull_request`（仅运行 Layer 1 快速测试）
- **Push 触发**：`push` to main（运行 Layer 1 + Layer 2）
- **定时触发**：`schedule` nightly（UTC 04:00 = 北京时间中午，运行全量 Layer 1-3）

#### TC3.2: 工作流结构

```
test.yml
├── jobs:
│   ├── layer1-quick-tests        # Matrix: [frontend, go-core, combolite-check]
│   │   ├── frontend-vitest       # npx vitest run --reporter=verbose
│   │   ├── go-core-test          # go test ./internal/v2/... -count=1 -timeout 120s
│   │   └── combolite-static      # go test ./internal/service/ -run TestComboLite -v
│   │   └── outputs: layer1-result # 汇总 pass/fail + 耗时
│   │
│   ├── layer2-full-regression    # needs: layer1
│   │   ├── go-full-test         # go test ./internal/... -count=1 -timeout 300s
│   │   │   ├── -coverprofile=cover.out
│   │   │   └── coverage report upload
│   │   ├── frontend-coverage    # npx vitest run --coverage
│   │   ├── api-contract-test    # go test ./internal/server/... -run TestAPIContract
│   │   └── outputs: layer2-result
│   │
│   ├── layer3-e2e-integration   # needs: layer2
│   │   ├── encryption-e2e       # go test ./internal/v2/plugins/video/... -run TestEncryptionE2E
│   │   │   ├── v3-roundtrip
│   │   │   ├── v4-roundtrip
│   │   │   ├── file-retention-check
│   │   │   └── ffprobe-tolerance
│   │   ├── android-instrumented  # ./gradlew connectedAndroidTest (if emulator available)
│   │   └── config-consistency    # 双端 config.user.json 解析一致性
│   │   └── outputs: layer3-result
│   │
│   └── test-summary             # needs: all layers
│       ├── aggregate all layer results
│       ├── generate test report markdown
│       └── comment on PR / upload artifact
```

#### TC3.3: 关键实现细节

**缓存策略**：
- Go module cache (`go.sum`)
- npm cache (`package-lock.json`)
- Gradle cache
- FFmpeg build intermediates

**条件跳过**：
```yaml
# 可通过 workflow_dispatch input 跳过特定层
skip-layer1: ${{ github.event.inputs.skip_layer1 || 'false' }}
skip-e2e: ${{ github.event.inputs.skip_e2e || 'false' }}
```

**超时控制**：
- Layer 1 每个 job timeout-minutes: 10
- Layer 2 每个 job timeout-minutes: 20
- Layer 3 每个 job timeout-minutes: 30

**产物上传**：
- Layer 2: coverage report（HTML + lcov）
- Layer 3: E2E test logs（加密 roundtrip 详细输出）
- 全部层: test-results JUnit XML 格式（GitHub PR checks 集成）

**失败策略**：
- Layer 1 失败 → 阻断后续层（`if: always()` 但 summary 标记 fail）
- Layer 2/3 失败 → 不阻断（`continue-on-error: true`），但 summary 中标记

#### TC3.4: 与现有 android.yml 的关系
- `android.yml` **保持不变**（构建导向，保留现有测试步骤作为构建前检查）
- `test.yml` **独立存在**（纯测试导向，更完整的矩阵和报告）
- 两者共享缓存 key 前缀避免重复下载

### Requirement TC2: Makefile 测试目标

Makefile SHALL 定义清晰的测试入口：

```makefile
test-quick:       # Layer 1: 前端 + 核心 Go + 静态检查
test-full:        # Layer 2: 全量 + 覆盖率 + API 契约
test-e2e:         # Layer 3: 加密 roundtrip + 集成
test-android:     # Android instrumented (需要设备)
```

## Impact

- Affected code:
  - 新增测试文件（Go _test.go、前端 .test.ts、可能的 Android instrumented tests）
  - 可能新增 `tests/api_contract/` 目录
  - 修改 `.github/workflows/android.yml`（扩展测试矩阵）
  - 可能新增 `Makefile` test 目标
- Affected specs:
  - `fix-all-combolite-integration-defects`（T2 ComboLite 静态检查覆盖其修复）
  - `fix-three-runtime-defects`（T3 加密 E2E 覆盖 stsz/ffprobe 修复）
  - `replicate-combolite-install-confirm-ui`（T6 安装全链路覆盖 Broadcast 重构）
