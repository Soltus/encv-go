# Checklist

## 容器版本选择机制

- [x] 版本常量和状态枚举定义完成（ContainerV2/V3/V4, VersionStatus）
- [x] 插件接口扩展支持版本选择方法（SupportedContainerVersions/DefaultContainerVersion/ValidateVersion）
- [x] 所有现有插件实现版本方法的默认返回值
- [x] 配置 schema 更新，支持 default_container_version 和 strict_deprecated_version
- [x] 配置加载逻辑正确读取默认版本和严格模式标志
- [x] 版本验证逻辑正确处理 v2 弃用警告

## 密码错误感知机制

- [x] PasswordHint 计算算法实现（HMAC-SHA256, 16 字节输出）
- [x] PasswordHint 验证算法实现
- [x] V4 Header 结构更新包含 PasswordHint 字段（总大小保持 2048 字节）
- [x] Header 读写函数正确处理 PasswordHint 字段
- [x] HeaderCRC32 计算包含 PasswordHint 字段
- [x] 加密流程正确计算并写入 PasswordHint
- [x] 解密流程首先验证 PasswordHint
- [x] PasswordHint 不匹配时立即返回 ErrWrongPassword 错误
- [x] ErrWrongPassword / ErrDataCorrupted / ErrDeprecatedVersion 错误类型定义

## API 层适配

- [x] MobileTask 结构体正确传递 ContainerVersion
- [x] /api/container/versions 端点可用并返回正确的版本列表及状态
- [x] 加密 API 接受可选 version 参数
- [x] 未提供 version 时使用配置中的默认值
- [x] 错误响应包含明确的错误码（WRONG_PASSWORD / DATA_CORRUPTED）

## 前端 UI 实现

- [x] ContainerVersionSelector 组件渲染版本列表
- [x] v2 显示为灰色禁用状态并附带"已弃用"标签
- [x] v4 标记为"推荐"且默认选中
- [x] 版本选择结果正确传递给加密 API 调用
- [x] 密码错误时显示友好提示："密码可能错误"
- [x] 数据损坏时显示不同提示："数据已损坏"
- [x] v2 弃用操作显示警告提示

## 测试验证

- [x] PasswordHint 单元测试通过（计算、验证、碰撞概率）
- [x] 版本状态映射单元测试通过
- [x] 插件接口版本方法单元测试通过
- [x] 配置加载单元测试通过
- [x] v4 容器创建时 PasswordHint 正确写入（集成测试）
- [x] 正确密码解密 v4 容器成功（集成测试）
- [x] 错误密码解密 v4 容器返回 ErrWrongPassword（集成测试）
- [x] v2/v3 容器向后兼容性验证通过（无 PasswordHint 检查）
- [x] 前端版本选择 UI 交互测试通过
