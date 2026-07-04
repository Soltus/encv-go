# 文件长按菜单增强 + 侧边抽屉 + 标签系统 Spec

## Why

当前 encv-mobile 文件界面长按菜单仅有"删除"和"加密/解密"两个操作，缺少文件管理基础功能（重命名、复制、移动、分享、标签）。同时文件页面没有插件分类入口，用户无法按插件类型快速筛选文件。

## What Changes

### 1. 文件长按菜单扩展
- 新增：重命名（调用已有后端 `POST /api/file/rename`）
- 新增：复制（新增后端 API `POST /api/file/copy`）
- 新增：移动（新增后端 API `POST /api/file/move`）
- 新增：分享（使用 `@capacitor/share` 原生分享能力）
- 新增：标签（基于文件哈希，存储于后端数据库，需新增标签 CRUD API）

### 2. 文件页面侧边抽屉
- 左侧抽屉根据 `plugins.Plugins` 动态生成入口列表
- 每个入口显示插件名称、图标、支持的后缀名数量
- 点击进入二级页面，复用现有 `searchFiles` 检索缓存
- 二级页显示该插件处理的：未加密源文件 + 已加密容器文件

### 3. 后端新增 API
- `POST /api/file/copy` — 文件复制
- `POST /api/file/move` — 文件移动
- `GET /api/plugins` — 返回所有已注册插件元信息（名称/扩展名/MIME前缀/容器扩展名）
- `GET /api/files/tags` — 获取文件标签列表
- `POST /api/files/tags` — 为文件添加/移除标签
- `GET /api/files?tag=xxx` — 按标签筛选文件

### 4. Mock 测试完善
- 后端新增 handler 的单元测试（copy/move/tag handlers）
- 前端 Files.vue 长按菜单交互逻辑的组件测试

## Impact

- Affected specs: 无前置 spec 依赖
- Affected code:
  - `app/encv-mobile/src/views/Files.vue` — 长按菜单 + 侧边抽屉
  - `app/encv-mobile/src/api/encv.ts` — 新增 API 函数
  - `internal/server/admin_handlers.go` — copy/move handler
  - `internal/server/mobile_api.go` — plugins 元数据 + tags API
  - `internal/server/server.go` — 路由注册
  - `internal/server/*_test.go` — 新增测试

## ADDED Requirements

### Requirement: 文件重命名功能
系统 SHALL 提供文件重命名功能，调用已有的 `POST /api/file/rename` 接口。
#### Scenario: 用户重命名文件
- **WHEN** 用户在文件列表长按某文件并选择"重命名"
- **THEN** 弹出输入框预填当前文件名，用户确认后调用后端 API 完成重命名并刷新列表

### Requirement: 文件复制功能
系统 SHALL 提供文件复制功能，通过新增 `POST /api/file/copy` 后端接口实现。
#### Scenario: 用户复制文件到同目录
- **WHEN** 用户选择"复制"
- **THEN** 系统自动生成 `{原名}_copy.{ext}` 形式的目标路径并调用后端完成复制

### Requirement: 文件移动功能
系统 SHALL 提供文件移动功能，通过新增 `POST /api/file/move` 后端接口实现。
#### Scenario: 用户移动文件到目标目录
- **WHEN** 用户选择"移动"
- **THEN** 弹出目录选择器（复用文件浏览），选择目标目录后调用后端完成移动

### Requirement: 文件分享功能
系统 SHALL 使用 `@capacitor/share` 原生分享插件实现文件分享。
#### Scenario: 用户分享文件
- **WHEN** 用户选择"分享"且设备为原生环境
- **THEN** 调用 Share.share() 触发系统分享面板；Web 环境降级为下载链接提示

### Requirement: 文件标签系统
系统 SHALL 基于文件内容哈希（SHA256）提供标签管理功能，标签存储于后端内存数据库。
#### Scenario: 用户为文件添加标签
- **WHEN** 用户选择"标签"并在弹窗中输入标签名
- **THEN** 系统计算文件哈希，将标签关联存储到后端，后续可通过标签筛选文件
#### Scenario: 按标签筛选文件
- **WHEN** 用户在侧边栏或搜索中选择某个标签
- **THEN** 文件列表仅显示带有该标签的文件

### Requirement: 插件侧边抽屉
系统 SHALL 在文件页面提供左侧抽屉，动态展示所有已注册插件入口。
#### Scenario: 用户打开插件抽屉
- **WHEN** 用户点击文件页面左上角插件按钮或左滑打开抽屉
- **THEN** 抽屉展示所有插件列表（名称+图标+支持格式数），点击进入该插件的文件检索二级页
#### Scenario: 用户查看视频插件关联文件
- **WHEN** 用户点击"视频"插件入口
- **THEN** 进入二级页，分别展示未加密的视频文件和已加密的视频容器文件

## MODIFIED Requirements

### Requirement: 文件长按 ActionSheet
现有的 `showFileActions` 函数 SHALL 扩展为包含全部6个操作项（删除、加密/解密、重命名、复制、移动、分享、标签）。

### Requirement: 后端路由
server.go 的路由组 SHALL 新增 copy、move、plugins、tags 相关路由。

## REMOVED Requirements

无
