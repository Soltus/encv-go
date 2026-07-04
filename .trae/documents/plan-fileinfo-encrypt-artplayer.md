# 实施计划：文件信息修复 + 加密解密逻辑修正 + Artplayer 竖屏黑边

## 核心发现：没有 `.encv` 扩展名

加密容器的扩展名由插件定义：
- 视频：`.sccgv`、文本：`.sccgt`、PDF：`.sccgpdf`、WPS：`.sccgwps`、图片：`.sccgi`、音频：`.sccga`

**所有基于 `.encv` 扩展名判断的逻辑都是错误的。** 正确的判断方式：
- 后端：使用 `detector.DetectContainer()` 基于文件内容检测
- 前端：使用 `ListFiles` 返回的 `isEncrypted` 字段（后端已通过内容检测设置）

---

## 问题 1：加密容器文件信息页不显示容器信息和清单

### 根因

`GetFileInfo` 中 `ext == ".encv"` 永远不会命中，因为加密容器的扩展名是 `.sccgv`/`.sccgt`/`.sccgpdf` 等。导致 `IsEncvContainer` 永远为 false，`Container` 数据永远为空。

### 修复清单

#### 后端（Go）— 4 处

**1. `mobile_service.go:204` — `ReadFileContent` 中 `ext == ".encv"` 拦截**
```go
// 修复：使用内容检测替代扩展名判断
if _, detectErr := detector.DetectContainer(absPath); detectErr == nil {
    return nil, &BadRequestError{Err: errors.New("is_encv_container: use /api/file/info endpoint for metadata")}
}
```

**2. `mobile_service.go:275` — `GetFileInfo` 中 category 判断 `ext == ".encv"`**
```go
// 修复：移除 ext == ".encv"
} else if strings.HasPrefix(mimeType, "text/") || mimeType == "application/pdf" || mimeType == "application/epub+zip" {
```

**3. `mobile_service.go:291` — `GetFileInfo` 中 `ext == ".encv"` 判断容器**
```go
// 修复：使用内容检测
if _, detectErr := detector.DetectContainer(absPath); detectErr == nil {
    result.IsEncvContainer = true
    result.IsEncrypted = true
    // ... 读取容器信息
}
```

**4. `mobile_service.go:816` — `mediaExtensions` 中 `"encv": true`**
- 移除 `"encv": true`，加密容器不应通过 `StreamExternalFile` 直接读取

#### 前端（TypeScript/Vue）— 4 处

**5. `encv.ts:383` — `getFileCategory` 中 `ext === 'encv'`**
```typescript
// 修复：移除，isEncrypted 参数已处理
// 删除: if (ext === 'encv') return 'encrypted'
```

**6. `FilePreview.vue:169` — `determinePreviewType` 中 `ext === 'encv'`**
```typescript
// 修复：只依赖 category
if (category === 'encrypted') return 'container'
```

**7. `FilePreview.vue:193` — `determinePreviewType` 调用缺少 `isEncrypted` 参数**
- Files.vue 跳转预览时需传递 `isEncrypted` query 参数
- FilePreview.vue 从 route.query 读取 `isEncrypted` 并传给 `determinePreviewType`

**8. `Files.vue:652` — 解密时 `file.name.replace(/\.encv$/i, '')`**
- 后端 `processDecrypt` 中 `task.TargetPath` 是目录，输出文件名由后端从 manifest 获取
- 前端 `outputName` 只用于覆盖检查（`checkFileExists`）
- **修复**：移除 `.encv` 替换逻辑，改为从后端 `/api/file/info` 获取 `original_filename`
- 或更简方案：**跳过解密的覆盖检查**，后端会处理覆盖

#### 补充：FileInfo i18n key

**9. `useI18n.ts` — 补充缺失的 fileInfo i18n key**
```
'fileInfo.name': '文件名' / 'Name'
'fileInfo.path': '路径' / 'Path'
'fileInfo.size': '大小' / 'Size'
'fileInfo.modified': '修改时间' / 'Modified'
'fileInfo.category': '分类' / 'Category'
```

---

## 问题 2：加密解密逻辑修正

### 2.1 移除加密弹窗的密码输入框

`handleEncryptFile` 中：
- 移除 `password` 输入框
- 加密时直接使用全局密码（从 config 读取）
- 如果全局密码为空，显示错误提示并阻止操作

### 2.2 全局密码为空时加密应当失败

**前端**：加密前检查全局密码，为空则 toast 提示并 return

**后端**：`processEncrypt` 开头增加密码为空校验：
```go
password := tm.cfg.Password
if task.Password != "" {
    password = task.Password
}
if password == "" {
    tm.failTask(task.ID, "encryption requires a password: global password is empty")
    return
}
```

### 2.3 解密弹窗保留密码输入框

解密可能需要不同密码（文件可能用不同密码加密），保留密码输入框，预填全局密码。

### 2.4 解密覆盖检查修正

移除 `file.name.replace(/\.encv$/i, '')` 逻辑。后端 `processDecrypt` 的 `TargetPath` 是目录，输出文件名由后端决定。前端无法准确预知输出文件名（在 manifest 中），所以**简化为不做覆盖检查**，后端会处理。

### 2.5 新增 i18n key
```
'files.noPassword': '请先在设置中配置全局密码' / 'Please set a global password in settings first'
```

### 涉及文件
| 文件 | 改动 |
|------|------|
| `app/encv-mobile/src/views/Files.vue` | 加密弹窗移除密码框 + 空密码校验 + 解密覆盖检查修正 |
| `internal/service/task_manager.go` | `processEncrypt` 增加密码为空校验 |
| `app/encv-mobile/src/composables/useI18n.ts` | 新增 `files.noPassword` i18n key |

---

## 问题 3：竖屏比例视频 Artplayer 黑边过大

### 根因

当前 `ArtPlayerView.vue` 的视频容器逻辑：
1. 容器强制 `width: 100%`（横屏宽度）
2. `loadedmetadata` 中手动设置高度（按视频比例计算）
3. 竖屏视频（9:16）在横屏容器中，`object-fit: contain` 导致左右有大黑边

**问题**：竖屏视频应该让容器宽度适配视频比例，而不是强制占满屏幕宽度。

### 修复方案

在 `video:loadedmetadata` 事件中，区分横屏/竖屏视频：

**竖屏视频**（`videoHeight > videoWidth`）：
- 容器高度 = `window.innerHeight - 56`（占满可用高度）
- 容器宽度 = `height * videoWidth / videoHeight`（按比例计算）
- 容器居中（`margin: 0 auto`）

**横屏视频**：
- 容器宽度 = 屏幕宽度
- 容器高度 = 按比例计算（不超过 `maxHeight`）

同时移除 `initArtPlayer` 中的 `minHeight`/`maxHeight` 设置（与 `loadedmetadata` 冲突）。

### 涉及文件
| 文件 | 改动 |
|------|------|
| `app/encv-mobile/src/views/ArtPlayerView.vue` | 竖屏视频容器尺寸适配 |

---

## 执行顺序

1. **问题 1 后端**（4 处 `.encv` 硬编码修复）
2. **问题 1 前端**（4 处 `.encv` 硬编码修复 + i18n）
3. **问题 2**（加密解密逻辑修正）
4. **问题 3**（Artplayer 竖屏黑边）
5. **构建验证** — go vet + vue-tsc + vite build
