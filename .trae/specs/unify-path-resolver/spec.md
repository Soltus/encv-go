# 统一路径处理委托 Spec

## Why

**predictPlugin API 持续返回 400 错误**，根因排查显示路径处理分散在项目各处（Files.vue 的 `file.path`、Tasks.vue 的路由 query 解析、useTaskForm 的 doPredict、encv.ts 的 API 调用），缺乏统一归一化层。当前路径从后端 `ListFiles` 返回后直接透传到前端各消费点，中间无任何标准化处理，导致：

1. **路径格式不一致风险**：后端 `mobile_service.go:L130-133` 构造路径时对 `/` 根路径有特殊处理（`filePath = "/" + entry.Name()`），子目录则是 `queryPath + "/" + entry.Name()`，但前端不做任何 normalize
2. **开发预览环境缺少 mock 适配**：沙箱预览环境下文件系统路径与真实设备不同，需要路径映射层
3. **桌面端/Android 端特殊路径无法统一处理**：Android `content://`、SD 卡 `/storage/emulated/0/`、桌面端 Windows `C:\` 等场景需要平台感知的路径归一化

## What Changes

### 新增：前端路径工具模块 `usePathResolver`

- 新建 `src/composables/usePathResolver.ts` — 统一路径归一化 composable
- 职责：
  - **normalize(rawPath)** → 标准化路径（去除重复 `/`、处理 `\` 转 `/`、确保绝对路径以 `/` 开头）
  - **resolveFileItem(file)** → 从 FileItem 提取并归一化 path
  - **toAPIPath(path)** → 转换为 API 请求安全格式（URL encode 已由调用层处理，此处负责内容归一化）
  - **isAbsolutePath(path)** → 判断是否为绝对路径
  - **getMockPath(platform)** → 开发预览环境返回可用的 mock 文件路径

### 修改：消费点全部走 usePathResolver

| 文件 | 当前方式 | 改为 |
|------|---------|------|
| `Files.vue` handleEncryptFile/handleDecryptFile | 直接用 `file.path` | `resolveFileItem(file)` |
| `Tasks.vue` processQueryAction | 直接用 `route.query.source as string` | `normalize(route.query.source)` |
| `useTaskForm.ts` doPredict | 直接传 sourcePath | 先 normalize 再调用 API |
| `encv.ts` predictPlugin | 直接 JSON.stringify | 入参已归一化（不改 API 层） |

### 新增：开发预览路径 mock

- `usePathResolver` 检测 `import.meta.env.DEV` + 后端 `/api/files` 可达性
- 当后端文件列表为空或不可用时，返回一组预设 mock 路径供 UI 测试
- mock 数据包含多种扩展名（`.mp4`, `.txt`, `.pdf`）以覆盖多候选选择分支

## Impact

- Affected specs: 无直接依赖其他 spec
- Affected code:
  - **新建** `src/composables/usePathResolver.ts`
  - **修改** `src/views/Files.vue` — 加密/解密按钮路径提取
  - **修改** `src/views/Tasks.vue` — processQueryAction 路径解析
  - **修改** `src/composables/useTaskForm.ts` — doPredict 入口归一化
  - **可选修改** `src/api/encv.ts` — predictPlugin 参数防御性校验

## ADDED Requirements

### Requirement: 路径归一化

系统 SHALL 提供 `usePathResolver()` composable，对所有跨组件路径进行统一归一化处理。

#### Scenario: 文件加密路径归一化
- **WHEN** 用户在 Files 页面长按文件选择"加密"
- **THEN** 传递给 Tasks 页面的路径 SHALL 经过 normalize 处理（去重 `/`、统一分隔符）
- **AND** predictPlugin API 收到的 sourcePath SHALL 是有效的归一化路径

#### Scenario: 开发预览 mock 路径
- **WHEN** 在开发环境且后端无可用的真实文件列表
- **THEN** usePathResolver SHALL 返回预设 mock 路径而非空值
- **AND** mock 路径 SHALL 包含 `.mp4` / `.txt` / `.pdf` 以覆盖不同插件候选分支

#### Scenario: FAB 新建任务（无源路径）
- **WHEN** 用户点击 FAB 按钮打开新建任务（无预填路径）
- **THEN** 不调用 predictPlugin API
- **AND** modal SHALL 正常显示空白表单等待用户输入

### Requirement: predictPlugin 400 防御

系统 SHALL 在 predictPlugin 调用前校验 sourcePath 非空且格式合法。

- **WHEN** sourcePath 为空字符串或仅含空白
- **THEN** 跳过 API 调用，不抛出 400 错误
- **WHEN** sourcePath 不以 `/` 开头（非绝对路径）
- **THEN** 自动添加 `/` 前缀后再调用

## MODIFIED Requirements

### Requirement: 路由跳转新建任务

现有 `processQueryAction()` SHALL 使用 `normalize()` 处理 `route.query.source`，确保传入 `openNewTaskModal()` 的路径已是标准格式。
