# ENCV-go Mobile 基础功能实现 & 暗黑模式计划

## 当前状态
- APK 已成功安装，首页显示正常
- 5 个 Tab 页面均为占位符状态（Files 有基础 API 集成但功能不完整）
- API 层仅有 `listFiles`、`getFileStreamUrl`、`checkServerStatus` 三个函数
- 无暗黑模式、无主题系统、无状态持久化

## 实现计划

### 第一步：暗黑模式基础设施（Theme 系统）

**新建文件：**
- `src/theme/variables.css` — Ionic CSS 变量覆盖，定义亮色/暗色两套颜色
- `src/composables/useTheme.ts` — 主题管理 composable

**修改文件：**
- `src/main.ts` — 引入 `variables.css`
- `src/App.vue` — 在根组件应用主题 class

**实现细节：**
1. `variables.css` 使用 `:root` 定义亮色变量，使用 `body.dark` 定义暗色变量
2. `useTheme.ts` 提供：
   - `isDark` ref（响应式）
   - `toggleDark()` 方法
   - `initTheme()` 方法：读取 localStorage 持久化偏好，若未设置则跟随系统 `prefers-color-scheme`
   - 切换时在 `document.body` 上添加/移除 `dark` class
3. App.vue 的 `onMounted` 中调用 `initTheme()`

### 第二步：Files 页面 — 完整文件浏览

**修改文件：**
- `src/views/Files.vue` — 重写为完整文件浏览器
- `src/api/encv.ts` — 新增 API 接口

**实现细节：**
1. **面包屑导航**：显示当前路径，点击可跳转到任意父级
2. **目录遍历**：
   - 点击文件夹 → 进入子目录
   - 返回按钮 → 回到上级目录
3. **文件类型图标**：根据扩展名显示不同图标
   - 视频（.mp4/.mkv/.avi/.mov）→ videocam
   - 音频（.mp3/.flac/.wav/.aac）→ musical-notes
   - 图片（.jpg/.png/.gif/.webp）→ image
   - 文档（.pdf/.doc/.txt）→ document
   - 加密文件（.encv）→ lock-closed
   - 其他 → document-text
4. **文件信息**：显示文件大小（格式化）、修改时间
5. **点击文件**：
   - 视频/音频 → 路由到 Player 页面，传递文件路径
   - 其他文件 → 显示 toast 提示暂不支持
6. **下拉刷新**：`ion-refresher` 组件
7. **空状态**：目录为空时显示友好提示
8. **加载状态**：`ion-spinner` 加载指示器
9. **API 扩展**：
   - `listFiles` 返回类型修正（当前返回 `FileListResponse` 但实际赋值给 `files` 时类型不匹配）
   - 新增 `deleteFile(path)` 接口

### 第三步：Player 页面 — 视频播放器

**修改文件：**
- `src/views/Player.vue` — 完整播放器实现
- `src/router/index.ts` — Player 路由支持 query 参数 `?path=xxx`

**实现细节：**
1. 从路由 query 获取文件路径
2. HTML5 `<video>` 标签，src 指向 `getFileStreamUrl(path)`
3. 原生播放控件（`controls` 属性）
4. 自动播放（`autoplay`）
5. 文件名显示在标题栏
6. 无文件时显示提示信息，引导用户从 Files 页面选择
7. 全屏支持
8. 播放错误处理（toast 提示）

### 第四步：Tasks 页面 — 加解密任务管理

**修改文件：**
- `src/views/Tasks.vue` — 任务列表 UI
- `src/api/encv.ts` — 新增任务相关 API

**实现细节：**
1. **任务列表**：显示所有加解密任务
   - 任务名称（源文件名）
   - 任务类型（加密/解密）+ 图标
   - 任务状态（排队中/进行中/已完成/失败）
   - 进度条（进行中时显示）
2. **新建任务按钮**：`ion-fab-button`
   - 选择加密/解密
   - 选择源文件
3. **任务操作**：
   - 取消进行中的任务
   - 重试失败的任务
   - 清除已完成的任务
4. **空状态**：无任务时显示引导
5. **API 扩展**：
   - `getTasks()` — 获取任务列表
   - `createTask(type, sourcePath)` — 创建任务
   - `cancelTask(id)` — 取消任务
   - `retryTask(id)` — 重试任务

### 第五步：WebDAV 页面 — WebDAV 配置

**修改文件：**
- `src/views/WebDAV.vue` — WebDAV 配置 UI
- `src/api/encv.ts` — 新增 WebDAV API

**实现细节：**
1. **WebDAV 服务器配置表单**：
   - 服务器地址（URL）
   - 用户名
   - 密码（密码输入框 + 显示/隐藏切换）
   - 挂载路径
2. **连接测试按钮**：验证 WebDAV 连接
3. **保存配置**：持久化到 localStorage
4. **WebDAV 状态指示**：已连接/未连接
5. **已保存的 WebDAV 配置列表**：支持多个 WebDAV 服务器
6. **API 扩展**：
   - `testWebDAV(config)` — 测试连接
   - `saveWebDAVConfig(config)` — 保存配置
   - `getWebDAVConfigs()` — 获取配置列表

### 第六步：Settings 页面 — 应用设置

**修改文件：**
- `src/views/Settings.vue` — 完整设置页面

**实现细节：**
1. **外观设置**：
   - 暗黑模式开关（`ion-toggle`，绑定 `useTheme` 的 `isDark`）
2. **服务器设置**：
   - 服务器地址（默认 `http://127.0.0.1:2025`）
   - 服务器状态指示（在线/离线 + 重试按钮）
3. **关于**：
   - 应用名称 + 版本号
   - ENCV-go 引擎版本
   - GitHub 仓库链接
4. **危险操作**：
   - 清除缓存
   - 重置设置

### 第七步：全局增强

**修改文件：**
- `src/api/encv.ts` — 服务器地址可配置化
- `src/composables/useServerStatus.ts` — 服务器状态轮询 composable

**新建文件：**
- `src/composables/useServerStatus.ts`

**实现细节：**
1. 服务器地址从 localStorage 读取，默认 `http://127.0.0.1:2025`
2. `useServerStatus` 提供：
   - `isOnline` ref
   - `checkStatus()` 方法
   - 可选的定时轮询（在 App.vue 中启用）
3. 所有 API 调用使用可配置的 base URL

## 文件变更总览

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新建 | `src/theme/variables.css` | 亮色/暗色主题 CSS 变量 |
| 新建 | `src/composables/useTheme.ts` | 主题管理 composable |
| 新建 | `src/composables/useServerStatus.ts` | 服务器状态 composable |
| 修改 | `src/main.ts` | 引入 theme CSS |
| 修改 | `src/App.vue` | 应用主题 class + 服务器状态检测 |
| 修改 | `src/views/Files.vue` | 完整文件浏览器 |
| 修改 | `src/views/Player.vue` | HTML5 视频播放器 |
| 修改 | `src/views/Tasks.vue` | 任务管理 UI |
| 修改 | `src/views/WebDAV.vue` | WebDAV 配置 UI |
| 修改 | `src/views/Settings.vue` | 完整设置页面 |
| 修改 | `src/api/encv.ts` | 扩展 API + 可配置 base URL |
| 修改 | `src/router/index.ts` | Player 路由支持 query 参数 |

## 执行顺序

1. 暗黑模式基础设施（variables.css + useTheme + main.ts + App.vue）
2. API 层扩展（encv.ts 可配置化 + 新接口）
3. useServerStatus composable
4. Files.vue 完整重写
5. Player.vue 播放器实现
6. Tasks.vue 任务管理
7. WebDAV.vue 配置页面
8. Settings.vue 设置页面
9. 本地构建验证（`npm run build`）
