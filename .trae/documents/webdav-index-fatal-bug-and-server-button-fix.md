# 修复 WebDAV 复用索引致命bug + 设置界面启动按钮问题

## 问题总览

1. **WebDAV 复用索引致命bug**：搜索无结果、索引统计自相矛盾、第三方客户端挂载失败、devlogs无日志
2. **设置界面启动按钮问题**：启动后需手动刷新、成功后多出一个按钮

---

## 问题 1：WebDAV 复用索引致命bug

### 1a. 搜索存在的文件显示无结果

**根因**：`canUseWebdavIndex()` 返回 true 时，`handleSearchFilesGin` 只搜索 WebDAV 索引。但 WebDAV 索引只包含加密容器解密后的虚拟文件（如 `video.mp4`），不包含普通文件（如普通的 `.mp4`、`.srt`、`.mkv` 等）。

**根因链**：
- `buildInitialIndex()` 第 261 行 `if !fs.containerExtensions[ext] { return nil }` 跳过所有非容器文件
- `addOrUpdateEntry()` 第 337 行同样跳过非容器文件
- `SearchInIndex()` 只搜索 `fileInfoMap`，而 `fileInfoMap` 只有容器虚拟文件

**修复方案**：搜索时合并两个索引的结果：
1. 始终使用 MobileService 索引搜索（包含所有文件）
2. 当 WebDAV 索引可用时，额外搜索 WebDAV 索引（容器虚拟文件）
3. 从 MobileService 结果中过滤掉容器物理文件（加密文件名如 `.sccgv`），用 WebDAV 虚拟文件替代
4. 为 `IndexProvider` 接口添加 `IsContainerExtension(filename string) bool` 方法用于过滤

**修改文件**：
- `internal/webdav/fs_v2.go`：`IndexProvider` 接口添加 `IsContainerExtension` 方法
- `internal/server/mobile_api.go`：`handleSearchFilesGin` 合并搜索逻辑

### 1b. 索引统计为空但显示已就绪

**根因**：`handleIndexStatsGin` 在 `canUseWebdavIndex()` 为 true 时直接返回 WebDAV 索引统计。如果目录中没有加密容器，WebDAV 索引的 TotalFiles=0、TotalDirs=0，但 Source="webdav"。前端 Settings.vue 根据 `source === 'webdav'` 显示"WebDAV 索引已就绪"，与空数据自相矛盾。

**修复方案**：
1. 始终构建 MobileService 索引（移除 `handleIndexStatsGin` 和 `handleIndexRebuildGin` 中的 WebDAV 短路逻辑）
2. 合并两个索引的统计：TotalFiles = MobileService 普通文件数 + WebDAV 虚拟文件数，Containers = WebDAV 容器数
3. 前端 CacheDetail.vue 和 Settings.vue 的索引状态行基于实际文件数量判断，而非仅依赖 source

**修改文件**：
- `internal/server/mobile_api.go`：`handleIndexStatsGin` 合并统计逻辑
- `internal/server/mobile_api.go`：`handleIndexRebuildGin` 始终重建 MobileService 索引
- `app/encv-mobile/src/views/CacheDetail.vue`：适配合并后的统计数据
- `app/encv-mobile/src/views/Settings.vue`：索引状态行逻辑修正

### 1c. 第三方应用挂载 WebDAV 显示路径不存在

**可能原因**：
1. 路由 `/webdav/*path` 不匹配 `/webdav`（无尾部斜杠），WebDAV 客户端可能请求无斜杠的路径
2. WebDAV 请求没有被日志记录，无法排查问题

**修复方案**：
1. 添加 `/webdav` → `/webdav/` 的重定向
2. 为 WebDAV handler 添加请求日志中间件（使用 slog）

**修改文件**：
- `internal/server/server.go`：添加重定向路由 + WebDAV 日志中间件

### 1d. devlogs 没有显示日志

**根因**：WebDAV handler 通过 `gin.WrapH` 包装，不经过 Gin 中间件链。`goWebdav.Handler` 内部使用标准库日志，不经过 slog，因此日志不会推送到 WebSocket/devlogs。

**修复方案**：添加 WebDAV 请求日志中间件，在 WebDAV handler 前后记录请求信息（方法、路径、状态码、耗时），使用 slog 输出。

**修改文件**：
- `internal/server/server.go`：添加 `webdavLoggingMiddleware`

---

## 问题 2：设置界面启动后端按钮问题

### 2a. 启动后端需要手动刷新才显示在线状态

**根因**：`useServerStatus.ts` 的 `handleRestart()` 成功后（第 86-91 行），只在失败时设 `isRestarting.value = false`。成功时不更新 `isRestarting` 和 `isOnline`，完全依赖 native bridge 事件（`encv:backend-ready`）。如果事件延迟，UI 不更新。

**修复方案**：`handleRestart` 成功后：
1. 立即设 `isRestarting.value = false`
2. 主动调用 `getBackendStatus()` 检查后端状态，更新 `isOnline`
3. 如果后端已在线，连接 WebSocket

### 2b. 启动成功后多出一个按钮

**根因**：ServerDetail.vue 按钮逻辑：
- 刷新按钮：始终显示
- 停止按钮：`v-if="serverOnline"` — 在线时显示
- 启动按钮：`v-if="!serverOnline && !isRestarting"` — 离线且非重启中
- 加载中按钮：`v-if="isRestarting"` — 重启中

当 native bridge 更新 `isOnline=true` 但 `isRestarting` 仍为 true 时，停止按钮和加载中按钮同时显示 = 3 个按钮。

**修复方案**：
1. 修复 `handleRestart` 成功后立即重置 `isRestarting`（见 2a）
2. 按钮逻辑改为互斥：停止按钮条件加 `&& !isRestarting`
3. 预期行为：启动按钮 → 加载中 → 停止按钮（同一位置替换）

**修改文件**：
- `app/encv-mobile/src/composables/useServerStatus.ts`：修复 `handleRestart` 状态管理
- `app/encv-mobile/src/views/ServerDetail.vue`：修复按钮互斥逻辑

---

## 实施步骤

### 步骤 1：修复 WebDAV 索引搜索和统计逻辑（Go 后端）

1. `internal/webdav/fs_v2.go`：
   - `IndexProvider` 接口添加 `IsContainerExtension(filename string) bool`
   - 实现 `IsContainerExtension` 方法

2. `internal/server/mobile_api.go`：
   - `handleSearchFilesGin`：移除 WebDAV 索引短路，改为合并搜索
     - 始终搜索 MobileService 索引
     - 当 WebDAV 索引可用时，额外搜索 WebDAV 索引
     - 从 MobileService 结果中过滤容器物理文件（用 `IsContainerExtension` 判断）
     - 合并结果
   - `handleIndexStatsGin`：移除 WebDAV 索引短路，改为合并统计
     - 始终获取 MobileService 统计
     - 当 WebDAV 索引可用时，额外获取 WebDAV 统计
     - 合并：TotalFiles = MobileService普通文件 + WebDAV虚拟文件，Containers = WebDAV容器数
     - 确保 MobileService 索引已构建（如果为空则触发构建）
   - `handleIndexRebuildGin`：移除 WebDAV 索引短路，始终重建 MobileService 索引

### 步骤 2：添加 WebDAV 日志和路由修复（Go 后端）

1. `internal/server/server.go`：
   - 添加 `/webdav` → `/webdav/` 重定向路由
   - 添加 `webdavLoggingMiddleware` 函数，记录 WebDAV 请求（方法、路径、耗时）
   - 在 WebDAV handler 前应用日志中间件

### 步骤 3：修复设置界面启动按钮（前端）

1. `app/encv-mobile/src/composables/useServerStatus.ts`：
   - `handleRestart` 成功后：设 `isRestarting.value = false`，调用 `getBackendStatus()` 更新状态
   - 成功后如果后端在线：设 `isOnline.value = true`，连接 WebSocket

2. `app/encv-mobile/src/views/ServerDetail.vue`：
   - 停止按钮条件改为 `v-if="serverOnline && !isRestarting"`
   - 确保加载中和停止按钮互斥

### 步骤 4：修复前端索引显示（前端）

1. `app/encv-mobile/src/views/Settings.vue`：
   - 索引状态行逻辑：当 TotalFiles=0 时不显示"已就绪"，显示"无数据"或"未索引"

2. `app/encv-mobile/src/views/CacheDetail.vue`：
   - 适配合并后的统计数据格式
   - 当 source="webdav" 时，显示容器数量和普通文件数量分开

### 步骤 5：验证

1. Go 编译通过
2. 前端构建通过（`vue-tsc --noEmit && vite build`）
3. 搜索功能测试：搜索普通文件和容器虚拟文件都能找到
4. 索引统计测试：显示正确的文件数量
5. WebDAV 客户端挂载测试
6. 启动按钮测试：启动 → 加载中 → 停止（无多余按钮）
