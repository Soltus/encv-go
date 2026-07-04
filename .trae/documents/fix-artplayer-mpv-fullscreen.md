# 修复 ArtPlayer 功能 + 全屏处理 + MPV 错误处理 + 外部打开

## 问题 1: ArtPlayer 高级控件缺失

### 根因
ArtPlayer 的很多功能默认关闭。当前配置只启用了基础功能（autoplay/autoSize/fullscreen/miniProgressBar），
`setting: false` 导致设置面板（字幕轨/播放速度/宽高比/翻转）不渲染。

### 修复方案
启用 ArtPlayer 移动端完整功能集：setting/playbackRate/aspectRatio/flip/lock/autoOrientation/autoPlayback/subtitleOffset/fastForward。
给 `.video-player` 容器添加 `position: relative; overflow: hidden;`。

## 问题 2: 全屏进入/退出

### 修复方案
安装 `@capacitor/status-bar` + `@capacitor/screen-orientation`。
进入全屏：`StatusBar.hide()` + `ScreenOrientation.lock(LANDSCAPE)`
退出全屏：`StatusBar.show()` + `ScreenOrientation.lock(PORTRAIT)`
`onBeforeUnmount` 恢复状态栏和方向。

## 问题 3: 设置添加屏幕方向选项

自动/竖屏锁定/横屏锁定，存储到 localStorage。

## 问题 4: MPV 播放错误处理

### 根因
错误信息太简略，没有上下文、分类、DevLogs 推送。

### 修复方案
1. 所有错误路径推送 `lynxLog.error()` 到 LogBridgeModule
2. 错误信息包含 initData 完整 JSON
3. MPV 错误分类映射
4. PlayerControls 增加错误类型标签

## 问题 5: 外部打开路径无效（核心架构修正）

### 根因
当前思路：Android 侧 `resolveFileInfo()` 尝试解析 URI → 得到不完整路径 → 传给 Go 后端 → Go 后端找不到文件。

**正确思路**：Go 后端知道自己的 serving 目录，应该由 Go 后端来处理路径解析。
Android 侧只需要把原始路径传给 Go 后端，Go 后端自己尝试解析。

用户实际错误：`/api/stream/external?path=/123云盘/xxx.mp4`
后端返回：`stat /123云盘/xxx.mp4: no such file or directory`

路径 `/123云盘/xxx.mp4` 不是绝对路径，但 Go 后端的 `StreamExternalFile` 要求绝对路径。
实际上这个路径是相对于 serving 目录的，Go 后端应该用 `SafeURLToAbsPath(s.servingDir, path)` 解析。

### 修复方案

#### Go 后端 `StreamExternalFile` 修改
当路径不是绝对路径，或绝对路径不存在时，尝试用 `SafeURLToAbsPath(s.servingDir, filePath)` 解析：
```go
func (s *MobileService) StreamExternalFile(w http.ResponseWriter, r *http.Request, filePath string) error {
    if filePath == "" {
        return &BadRequestError{Err: errors.New("'path' query parameter is required")}
    }

    absPath := filepath.Clean(filePath)

    // 如果不是绝对路径，尝试用 servingDir 解析
    if !filepath.IsAbs(absPath) {
        resolved, err := utils.SafeURLToAbsPath(s.servingDir, filePath)
        if err == nil {
            absPath = resolved
        } else {
            return &BadRequestError{Err: fmt.Errorf("path is not absolute and cannot be resolved: %s", filePath)}
        }
    }

    // 如果绝对路径不存在，尝试用 servingDir 解析
    info, err := os.Stat(absPath)
    if err != nil && os.IsNotExist(err) {
        resolved, resolveErr := utils.SafeURLToAbsPath(s.servingDir, filePath)
        if resolveErr == nil {
            if resolvedInfo, statErr := os.Stat(resolved); statErr == nil {
                absPath = resolved
                info = resolvedInfo
                err = nil
            }
        }
    }

    if err != nil {
        if os.IsNotExist(err) {
            return &NotFoundError{Err: fmt.Errorf("file not found: %s (also tried servingDir)", absPath)}
        }
        return &ForbiddenError{Err: err}
    }

    // ... 后续逻辑不变
}
```

#### Android 侧简化
`PlayerActivityLynx.resolveFileInfo()` 不再需要猜测路径前缀。
对于 `content://` URI，仍然需要 `copyContentToCache`（因为 Go 后端无法访问 content://）。
对于 `file://` URI，直接传 `uri.path` 给 Go 后端，让 Go 后端解析。
`Uri.encode(path, "/")` 保留 `/` 不编码。

## 实施步骤

### Step 1: 安装 Capacitor 插件
```bash
npm install @capacitor/status-bar @capacitor/screen-orientation
npx cap sync android
```

### Step 2: Go 后端 StreamExternalFile 修改
1. 非绝对路径时用 `SafeURLToAbsPath(s.servingDir, path)` 解析
2. 绝对路径不存在时也尝试 servingDir 解析
3. 改进错误信息

### Step 3: ArtPlayerView.vue — 启用完整功能 + 全屏处理
1. 启用 setting/playbackRate/aspectRatio/flip/lock/autoOrientation/autoPlayback/subtitleOffset/fastForward
2. `.video-player` 添加 `position: relative; overflow: hidden;`
3. 全屏处理改用 @capacitor/status-bar + @capacitor/screen-orientation
4. `onBeforeUnmount` 恢复状态栏和方向

### Step 4: PlayerApp.tsx — 增强 MPV 错误处理
1. 所有错误路径推送 lynxLog.error() 到 LogBridgeModule
2. 错误信息包含 initData 完整内容
3. MPV 错误分类映射

### Step 5: PlayerControls.tsx — 增强错误显示
1. 错误类型标签
2. 分层显示

### Step 6: PlayerActivityLynx.kt — 简化路径处理
1. `file://` URI 直接传 `uri.path`，不再猜测前缀
2. `content://` URI 仍用 `copyContentToCache`
3. `Uri.encode(path, "/")` 保留 `/`
4. 等待后端端口就绪

### Step 7: Settings.vue — 添加屏幕方向选项

### Step 8: App.vue — 初始化屏幕方向

### Step 9: 构建验证
```bash
vue-tsc --noEmit && vite build
```
