# 修复五个 Bug + 新增开发者选项计划

## Bug 1: 服务器地址端口叠加 `http://127.0.0.1:2025:2025`

### 根因
[ServerDetail.vue:152](file:///workspace/app/encv-mobile/src/views/ServerDetail.vue#L152) 用字符串替换从 `serverUrl` 提取 host，再拼接配置中的端口：
```js
const host = serverUrl.value.replace(/^https?:\/\//, '').replace(/\/.*$/, '')
// serverUrl = "http://127.0.0.1:2025" → host = "127.0.0.1:2025" → "http://127.0.0.1:2025:2025"
```
这是应试思维——用正则从字符串中提取 host，而实际上数据已经可以直接获取。

### 深层问题
配置中的端口（`server.port`、`admin.port`、`webdav.port`）是**配置值**，不是实际运行端口。当端口被占用时，后端 `StartGinWithRetry` 会自动递增端口（如 2025→2026），但配置不会更新。

### 架构发现
**所有服务（HTTP、Admin、WebDAV、OpenList）都在同一个端口上运行**，使用同一个 Gin 引擎。不同服务只是路径不同：
- 主服务：`/`
- 管理后台：`/admin`、`/login`、`/p`（文件管理代理）
- WebDAV：`/webdav/`（可配置）
- OpenList：`/openlist/`

`Admin.Port` 在后端代码中**根本没被使用**（冗余字段），`Webdav.Port` 只用来判断 WebDAV 是否启用（>0 表示启用）。

### 修复方案
**直接用 `serverUrl` 作为基础 URL**，不同服务只是追加路径。`serverUrl` 已经由 `useServerStatus` 更新为实际运行端口：

```js
const serviceUrls = computed<ServiceUrl[]>(() => {
  if (!configData.value || !serverOnline.value) return []
  const result: ServiceUrl[] = []
  const cfg = configData.value
  const baseUrl = serverUrl.value  // 已包含实际端口，如 http://127.0.0.1:2026

  // 主服务
  result.push({
    label: t('settings.httpServerSettings'),
    url: baseUrl,
    icon: cloudOutline,
  })

  // 管理后台（同端口，/admin 路径）
  const adminCfg = cfg.admin as Record<string, unknown> | undefined
  if (adminCfg && adminCfg.password !== undefined) {
    result.push({
      label: t('settings.adminServerSettings'),
      url: `${baseUrl}/admin`,
      icon: shieldCheckmark,
    })
  }

  // WebDAV（同端口，webdav.root 路径）
  const webdavCfg = cfg.webdav as Record<string, unknown> | undefined
  if (webdavCfg && typeof webdavCfg.port === 'number' && webdavCfg.port > 0) {
    const root = typeof webdavCfg.root === 'string' ? webdavCfg.root : '/webdav/'
    result.push({
      label: t('settings.webdavServerSettings'),
      url: `${baseUrl}${root}`,
      icon: globeOutline,
    })
  }

  return result
})
```

### 附带修复
后端 `handleRemoteInfoGin`（[mobile_api.go:261](file:///workspace/internal/server/mobile_api.go#L261)）也有同样的 bug，用 `cfg.Webdav.Port` 构建 URL。应改为使用实际运行端口。

### 修改文件
- `app/encv-mobile/src/views/ServerDetail.vue` — `serviceUrls` 计算属性重写
- `internal/server/mobile_api.go` — `handleRemoteInfoGin` 中 WebDAV URL 使用实际端口

---

## Bug 2: 内置 MPV 播放失败路径为空

### 根因分析
调用链路：`Files.vue:191` → `openPlayer(file.path, ...)` → `GoProcessPlugin:168` → `PlayerOverlayManager.showOverlay(path, ...)` → `buildInitDataJson()` → LynxView `initData.filePath` → `PlayerApp.tsx:38` → `GoBackendModule.getStreamUrl(path, ...)` → `http://127.0.0.1:$port/stream?path=$path`

**问题1**：`file.path` 是文件系统绝对路径如 `/storage/emulated/0/Movies/test.mp4`，传给后端 `/stream` 端点时，后端根据自身 `server.dir` 配置做路径映射，绝对路径可能无法正确映射。

**问题2**：`PlayerApp.tsx:258` 的 `useEffect` 中 `if (filePath)` 判断，当 initData 传递失败时 `filePath` 为空字符串，静默跳过不显示错误。

### 修复方案
1. **MPV 模式下直接传流 URL**：在 `Files.vue` 的 `playMedia()` 中，MPV 模式使用 `getFileStreamUrl(file.path)` 获取流 URL，传给 `openPlayer()` 作为 streamUrl 参数。`GoBackendModule.getStreamUrl()` 不再需要，直接使用前端传入的 URL。

2. **空路径时显示错误状态**：`PlayerApp.tsx` 中，当 `filePath` 为空时，设置 `playerState` 为 `'error'`。

### 修改文件
- `app/encv-mobile/src/views/Files.vue` — `playMedia()` MPV 分支改用流 URL
- `app/encv-mobile/src/plugins/GoProcess.ts` — `openPlayer()` 增加 streamUrl 参数
- `app/encv-mobile/src/plugins/web.ts` — 接口同步
- `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — 接收 streamUrl
- `app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerOverlayManager.kt` — 传递 streamUrl
- `app/encv-mobile/lynx-player/src/player/PlayerApp.tsx` — 使用 streamUrl 直接播放 + 空路径错误提示

---

## Bug 3: 内置 MPV 播放失败样式没有垂直居中

### 根因
[player.css:33-39](file:///workspace/app/encv-mobile/lynx-player/src/player/player.css#L33-L39) 中 `.ErrorContainer` 缺少 `flex: 1`，容器没有高度，垂直居中无效。

### 修复方案
添加 `flex: 1` 和 `flex-direction: column`：
```css
.ErrorContainer {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  padding: 24px;
  width: 100%;
  flex: 1;
}
```

### 修改文件
- `app/encv-mobile/lynx-player/src/player/player.css` 第 33-39 行

---

## Bug 4: 弹窗确认按钮文字显示 `COMMON.CONFIRM`

### 根因
[useI18n.ts](file:///workspace/app/encv-mobile/src/composables/useI18n.ts) 中没有定义 `common.confirm` 和 `common.cancel` 键，但以下位置使用了：
- [Files.vue:592](file:///workspace/app/encv-mobile/src/views/Files.vue#L592) — `t('common.confirm')`
- [Files.vue:654](file:///workspace/app/encv-mobile/src/views/Files.vue#L654) — `t('common.confirm')`
- [DevLogs.vue:274](file:///workspace/app/encv-mobile/src/views/DevLogs.vue#L274) — `t('common.cancel')`
- [DevLogs.vue:276](file:///workspace/app/encv-mobile/src/views/DevLogs.vue#L276) — `t('common.confirm')`

### 修复方案
在 `useI18n.ts` 的两个语言对象中添加 `common.confirm` 和 `common.cancel` 键：
- `zh-CN`: `'common.confirm': '确认'`, `'common.cancel': '取消'`
- `en`: `'common.confirm': 'Confirm'`, `'common.cancel': 'Cancel'`

### 修改文件
- `app/encv-mobile/src/composables/useI18n.ts` — 添加 common 命名空间键

---

## Bug 5: 深色模式下文件长按操作文字灰色可见度差

### 根因
[variables.css:146-150](file:///workspace/app/encv-mobile/src/theme/variables.css#L146-L150) 已定义了部分 ActionSheet 深色模式变量，但覆盖不完整。Ionic ActionSheet 的按钮文字在深色模式下可能使用了默认的灰色。

### 修复方案
1. 先尝试添加 Ionic 官方支持的 CSS 变量覆盖（activated/hover/focused 状态）
2. 如果变量不够，添加直接的 CSS 选择器覆盖确保所有按钮文字白色可见

```css
body.dark {
  --ion-action-sheet-button-color: #ffffff;
  --ion-action-sheet-button-color-activated: #cccccc;
  --ion-action-sheet-button-color-hover: #e0e0e0;
}
```

### 修改文件
- `app/encv-mobile/src/theme/variables.css` — 深色模式 ActionSheet 样式覆盖

---

## Bug 6: WebDAV 测试连接结果总是连接成功

### 根因
[mobile_service.go:228-246](file:///workspace/internal/service/mobile_service.go#L228-L246) 中 `TestWebDAV` 方法不检查 HTTP 响应状态码，401/403/404 都返回 nil（成功）。

### 修复方案
检查 HTTP 响应状态码，非 2xx 返回具体错误：
```go
resp, err := client.Do(httpReq)
if err != nil {
    return err
}
defer resp.Body.Close()

if resp.StatusCode >= 200 && resp.StatusCode < 300 {
    return nil
}
return fmt.Errorf("连接失败: HTTP %d", resp.StatusCode)
```

### 修改文件
- `internal/service/mobile_service.go` 第 228-246 行

---

## 新需求: 设置界面增加开发者选项

### 方案
vConsole 是一个轻量级的移动端调试工具，完全兼容 Capacitor WebView 环境。在设置界面增加「开发者选项」区域，包含启用 vConsole 的开关。

### 实施步骤
1. 安装 vconsole 依赖：`npm install vconsole`
2. 在 `useI18n.ts` 中添加开发者选项相关 i18n 键
3. 创建 `src/composables/useDevTools.ts` — 管理 vConsole 实例的创建/销毁，localStorage 持久化开关状态
4. 在 `Settings.vue` 中添加「开发者选项」区域，包含 vConsole 开关
5. 在 `App.vue` 的 `onMounted` 中检查 vConsole 开关状态，如果启用则初始化

### 修改/新增文件
- `app/encv-mobile/package.json` — 添加 vconsole 依赖
- `app/encv-mobile/src/composables/useDevTools.ts` — 新建，vConsole 管理
- `app/encv-mobile/src/composables/useI18n.ts` — 添加开发者选项 i18n 键
- `app/encv-mobile/src/views/Settings.vue` — 添加开发者选项区域
- `app/encv-mobile/src/App.vue` — 初始化 vConsole

---

## 实施顺序

1. Bug 1（端口叠加）— 用 `serverUrl` 直接构建 URL，不用配置端口
2. Bug 4（COMMON.CONFIRM）— 添加 i18n 键
3. Bug 3（MPV 错误样式）— CSS 修复
4. Bug 5（深色模式文字）— CSS 变量覆盖
5. Bug 6（WebDAV 测试连接）— 后端逻辑修复
6. Bug 2（MPV 播放路径为空）— 前后端多文件修改
7. 新需求（开发者选项 + vConsole）

## 构建验证

修改完成后运行：
```bash
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit && npx vite build
```
