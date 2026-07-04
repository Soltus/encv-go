# Tasks

## Phase 1: 容器版本选择机制（后端）

- [x] Task 1: 定义版本常量和状态枚举
  - [x] SubTask 1.1: 在 `internal/v2/types/container.go` 中定义容器版本常量 `ContainerV2=2`, `ContainerV3=3`, `ContainerV4=4`
  - [x] SubTask 1.2: 定义版本状态类型 `VersionStatus` (deprecated/stable/recommended)
  - [x] SubTask 1.3: 实现 `GetVersionStatus(version int) VersionStatus` 函数
  - [x] SubTask 1.4: 定义错误类型 `ErrWrongPassword`, `ErrDataCorrupted`, `ErrDeprecatedVersion`

- [x] Task 2: 扩展插件接口支持版本选择
  - [x] SubTask 2.1: 在 Plugin 接口中添加 `SupportedContainerVersions() []int` 方法
  - [x] SubTask 2.2: 添加 `DefaultContainerVersion() int` 方法
  - [x] SubTask 2.3: 添加 `ValidateVersion(version int) error` 方法
  - [x] SubTask 2.4: 为所有现有插件实现这些方法的默认返回值

- [x] Task 3: 更新配置系统支持默认版本
  - [x] SubTask 3.1: 更新 `config.schema.json`，新增 `default_container_version` 和 `strict_deprecated_version` 字段
  - [x] SubTask 3.2: 在 `internal/config/config.go` 中读取并暴露这些配置项
  - [x] SubTask 3.3: 实现版本验证逻辑，根据 `strict_deprecated_version` 决定是否阻止 v2

## Phase 2: 密码错误感知机制（v4 容器）

- [x] Task 4: 实现 PasswordHint 计算和验证
  - [x] SubTask 4.1: 在 `internal/v2/crypto/` 中创建 `password_hint.go`
  - [x] SubTask 4.2: 实现 `CalculatePasswordHint(password string, salt []byte) ([16]byte, error)` — 使用 HMAC-SHA256
  - [x] SubTask 4.3: 实现 `VerifyPasswordHint(hint [16]byte, password string, salt []byte) bool` — 验证密码是否正确
  - [x] SubTask 4.4: 编写单元测试验证 PasswordHint 的计算和碰撞概率

- [x] Task 5: 更新 V4 Header 结构支持 PasswordHint
  - [x] SubTask 5.1: 更新 `internal/v2/types/header_v4.go`，在 Header 中添加 `PasswordHint [16]byte` 字段
  - [x] SubTask 5.2: 调整 SpecialID 大小从 2000 减少至 1992 以保持 Header 总大小 2048 不变
  - [x] SubTask 5.3: 更新 `WriteHeaderV4` 和 `ReadHeaderV4` 函数处理新字段
  - [x] SubTask 5.4: 更新 HeaderCRC32 计算，包含 PasswordHint 字段

- [x] Task 6: 集成 PasswordHint 到加密/解密流程
  - [x] SubTask 6.1: 在加密流程中（PostEncryptProcessor），计算并写入 PasswordHint 到 Header
  - [x] SubTask 6.2: 在解密流程中（PreDecryptProcessor 或 Decrypt 开头），首先验证 PasswordHint
  - [x] SubTask 6.3: 若验证失败，立即返回 `ErrWrongPassword` 错误，不继续解密
  - [x] SubTask 6.4: 更新视频插件和其他插件的 Decrypt 方法集成此逻辑

## Phase 3: API 层适配

- [x] Task 7: 扩展移动端 API 支持版本信息
  - [x] SubTask 7.1: 更新 `MobileTask` 结构体，确保 `ContainerVersion` 字段正确传递
  - [x] SubTask 7.2: 新增 `/api/container/versions` 端点，返回支持的版本列表及状态
  - [x] SubTask 7.3: 在加密 API 中接受可选的 `version` 参数，若未提供则使用默认值
  - [x] SubTask 7.4: 错误响应中包含明确的错误码（WRONG_PASSWORD / DATA_CORRUPTED）

- [x] Task 8: 前端 API 类型更新
  - [x] SubTask 8.1: 在 `app/encv-mobile/src/api/encv.ts` 中定义 `ContainerVersion` 接口
  - [x] SubTask 8.2: 新增 `fetchContainerVersions()` API 函数
  - [x] SubTask 8.3: 更新加密任务创建接口，支持传入 `version` 参数

## Phase 4: 前端 UI 实现

- [x] Task 9: 容器版本选择组件
  - [x] SubTask 9.1: 创建 `ContainerVersionSelector.vue` 组件（或内联到 Files.vue）
  - [x] SubTask 9.2: 渲染版本列表：v2(deprecated/disabled), v3(stable), v4(recommended/default)
  - [x] SubTask 9.3: 实现版本选择的视觉反馈（推荐标签、弃用灰色样式）
  - [x] SubTask 9.4: 将选择结果传递给加密 API 调用

- [x] Task 10: 密码错误提示优化
  - [x] SubTask 10.1: 在 `Tasks.vue` 和 `Files.vue` 中识别 `ErrWrongPassword` 类型的错误
  - [x] SubTask 10.2: 对密码错误显示友好的用户提示："密码可能错误"
  - [x] SubTask 10.3: 对数据损坏显示不同的提示："数据已损坏"
  - [x] SubTask 10.4: 对 v2 弃用警告显示提示信息

## Phase 5: 测试与验证

- [x] Task 11: 单元测试
  - [x] SubTask 11.1: 测试 PasswordHint 计算/验证的正确性
  - [x] SubTask 11.2: 测试版本状态映射逻辑
  - [x] SubTask 11.3: 测试插件接口的版本方法默认实现
  - [x] SubTask 11.4: 测试配置加载和默认值回退

- [x] Task 12: 集成测试
  - [x] SubTask 12.1: 测试使用 v4 创建容器时 PasswordHint 正确写入
  - [x] SubTask 12.2: 测试使用正确密码解密 v4 容器成功
  - [x] SubTask 12.3: 测试使用错误密码解密 v4 容器返回 ErrWrongPassword
  - [x] SubTask 12.4: 测试 v2/v3 容器的向后兼容性（无 PasswordHint 检查）
  - [x] SubTask 12.5: 测试前端版本选择 UI 的交互和禁用状态

# Task Dependencies

- Task 2 depends on Task 1（插件接口扩展依赖版本常量定义）
- Task 3 depends on Task 1（配置系统依赖版本常量和状态）
- Task 4 depends on Task 1（PasswordHint 依赖错误类型定义）
- Task 5 depends on Task 4（Header 结构依赖 PasswordHint 实现）
- Task 6 depends on Task 4, Task 5（集成依赖 PasswordHint 和 Header 更新）
- Task 7 depends on Task 2, Task 6（API 层依赖插件接口和解密流程）
- Task 8 depends on Task 7（前端 API 类型依赖后端 API）
- Task 9 depends on Task 8（UI 组件依赖 API 类型）
- Task 10 depends on Task 8（错误提示依赖 API 错误码）
- Task 11 depends on Task 1-6（单元测试依赖核心功能完成）
- Task 12 depends on Task 7-10（集成测试依赖全部功能完成）

# Parallelizable Work

- Task 1 + Task 4 可并行（版本常量和 PasswordHint 算法独立）
- Task 9 + Task 10 可并行（UI 组件独立）
- Task 11 可与 Task 7-10 并行（单元测试不依赖前端）
