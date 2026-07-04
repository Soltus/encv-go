# 修复计划：信息/流式播放/解密三大功能适配 alist_encrypt 插件

## 问题概述

用户反馈：长按 `hyYGPCwJPQ3+xrdAvfnn2.bin` 后，「信息」「流式预览」「解密」三个操作均未正确适配 alist_encrypt 加密文件。

经过深度代码追踪，发现三个功能各有不同层面的缺陷：

---

## 问题 1：流式播放（actions.ts → ArtPlayerView.vue）— **完全不可用**

### 根因：路由参数不匹配

**发送端** [actions.ts:27](app/encv-mobile/src/features/alist-encrypt/actions.ts#L27)：
```typescript
router.push({ path: '/player', query: { streamUrl: url, name: decodedName } })
// 发送参数: { streamUrl: "/api/alist-encrypt/stream?path=...&password=...", name: "CAD放样.mp4" }
```

**接收端** [ArtPlayerView.vue:332-333](app/encv-mobile/src/views/ArtPlayerView.vue#L332-L333)：
```typescript
filePath.value = (route.query.path as string) || ''     // ← 读 'path'，但收到的是 'streamUrl'！
fileName.value = (route.query.name as string) || ''       // ← name 正确 ✅
```

**streamUrl 计算** [ArtPlayerView.vue:95-98](app/encv-mobile/src/views/ArtPlayerView.vue#L95-L98)：
```typescript
const streamUrl = computed(() => {
  if (!filePath.value) return ''                          // filePath 为空 → 返回空字符串！
  return getFileStreamUrl(filePath.value)                   // 用空路径生成 /stream?path= → 无效URL
})
```

**结果链路**：
```
点击"流式预览" → 输入密码 → promptPassword() ✅
  → setSessionPassword() ✅
  → loadDecodedName() ✅ 
  → getStreamUrl(f, password) → 生成正确的 alist-encrypt-stream URL ✅
  → router.push('/player', { streamUrl: url, name: decodedName })  ← 参数名错误!
    → ArtPlayerView: filePath='' (因为读的是 route.query.path)
      → streamUrl computed = '' (因为 filePath 为空)
        → initArtPlayer(): "播放地址为空" ❌
```

### 修复方案

**修改 actions.ts**：将 `streamUrl` 改为 `path`，让 Player 通过路径自行构建正确的 stream URL：

```typescript
// 修改前（错误）
router.push({ path: '/player', query: { streamUrl: url, name: decodedName } })

// 修改后（正确）
router.push({
  path: '/player',
  query: {
    path: f.path,              // ← 传递原始文件路径
    name: decodedName,         // ← 解码后的文件名
    alistPassword: password,  // ← 传递密码（alist_encrypt 专用）
    isAlistStream: 'true',    // ← 标记为 alist_encrypt 流
  }
})
```

**修改 ArtPlayerView.vue**：支持 `isAlistStream` 标记，使用 `getAlistEncryptStreamUrl`：

```typescript
onMounted(() => {
  filePath.value = (route.query.path as string) || ''
  fileName.value = (route.query.name) || ''
  const isAlistStream = route.query.isAlistStream === 'true'
  const alistPwd = (route.query.alistPassword as string) || ''
  
  // 覆盖 streamUrl computed：如果是 alist_encrypt 流，使用专用 URL
  if (isAlistStream && filePath.value && alistPwd) {
    alistStreamUrl.value = getAlistEncryptStreamUrl({ path: filePath.value, password: alistPwd })
  }
})
```

在 template 中使用 `alistStreamUrl || streamUrl` 作为播放地址。

---

## 问题 2：信息页（FileInfo.vue）— **缺少加密标识**

### 当前状态

| 功能 | 状态 | 说明 |
|------|------|------|
| API 调用 `/api/file/info` | ✅ 正常 | 路径解析已修复 |
| 基本文件信息展示 | ✅ 正常 | 名称、大小、MIME 等 |
| 解码后文件名 | ⚠️ 有条件 | 仅当 `isAlistEncrypted()`=true 且 decode API 成功时显示 |
| **加密标识/Badge** | ❌ 缺失 | `info.is_encrypted` 对 .bin 文件始终为 false |
| **文件类型提示** | ❌ 缺失 | 用户无法区分这是加密视频还是随机二进制 |

### 根因分析

[FileInfo.vue:58-61](app/encv-mobile/src/views/FileInfo.vue#L58-L61) 的加密标识依赖后端返回的 `is_encrypted` 字段：
```html
<div class="info-row" v-if="info.is_encrypted">
  <span class="info-label">{{ t('files.encrypted') }}</span>
  <ion-badge color="warning">Yes</ion-badge>
</div>
```

但后端 `GetFileInfo()` [mobile_service.go:439](internal/service/mobile_service.go#L439) 设置 `IsEncrypted: false` 作为默认值，只对 ENCV container 格式设为 true。alist_encrypt 的 `.bin` 文件不被识别为加密文件。

### 修复方案

**在 FileInfo.vue 中增加 Alist-Encrypt 专属检测和展示**：

1. 添加 `isAlistEnc` ref 标志（基于 `isAlistEncrypted(fileItem)` 判断）
2. 在基本信息卡片中增加 Alist-Encrypt 加密标识行：
```html
<div class="info-row" v-if="isAlistEnc">
  <span class="info-label">Alist-Encrypt</span>
  <ion-badge color="danger">加密</ion-badge>
</div>
```
3. 将「原始文件名」从底部弱显示提升为更显眼的位置（解码名称行加粗、变色）
4. 如果检测到是视频类加密文件（通过 decodedName 后缀判断），额外显示文件类型提示

---

## 问题 3：解密（useNewTaskModal）— **缺少密码传递**

### 当前状态

| 功能 | 状态 | 说明 |
|------|------|------|
| 打开任务弹窗 | ✅ 正常 | `openNewTask(f.path, 'decrypt')` 成功打开 |
| 插件预测 | ✅ 正常 | `doPredict()` 应能识别 alist_encrypt |
| 任务创建 | ✅ 正常 | `createTask()` API 调用正确 |
| **密码预填** | ❌ 缺失 | 用户需重新输入密码 |

### 根因

[actions.ts:35-38](app/encv-mobile/src/features/alist-encrypt/actions.ts#L35-L38) 的解密 handler：
```typescript
handler: async (f: FileItem) => {
  const { openNewTask } = useNewTaskModal()
  openNewTask(f.path, 'decrypt')   // ← 只传了路径，没有传密码！
}
```

而流式预览的 handler 在同文件中会先调用 `promptPassword()` 获取密码，但解密操作没有复用这个密码。

### 修复方案

**方案：利用 session password 缓存**

如果用户之前已经输入过密码（通过流式预览或文件列表的密码缓存），自动从 `getSessionPassword(f.path)` 读取并预填。

修改 `actions.ts` 的解密 handler：
```typescript
handler: async (f: FileItem) => {
  // 尝试从 session 缓存获取已有密码
  let password = getSessionPassword(f.path)
  
  // 如果没有缓存的密码，弹出输入框
  if (!password) {
    password = await promptPassword(f.name)
    if (password == null) return
    setSessionPassword(f.path, password)
  }
  
  const { openNewTask } = useNewTaskModal()
  
  // 注意：openNewTask 不直接支持预填密码，
  // 需要通过 state.secondaryPassword 或 extraValues 传入
  openNewTask(f.path, 'decrypt')
  
  // 通过 eventBus 或 setTimeout 在 modal 打开后设置密码值
  // （具体实现取决于 NewTaskModal 是否支持外部注入初始密码）
}
```

**备选方案（更简洁）**：如果 `useNewTaskModal.openNewTask` 支持额外的 initialPassword 参数，直接扩展其接口。否则在 `NewTaskModal.vue` 的 `onMounted` 中检查 session password 并自动填充。

---

## 修改文件清单

| # | 文件 | 修改类型 | 优先级 |
|---|------|---------|--------|
| 1 | `app/encv-mobile/src/features/alist-encrypt/actions.ts` | 重写 router.push 参数 + 密码传递 | P0 |
| 2 | `app/encv-mobile/src/views/ArtPlayerView.vue` | 支持 isAlistStream 标记 + alistStreamUrl | P0 |
| 3 | `app/encv-mobile/src/views/FileInfo.vue` | 增加 Alist-Encrypt 加密标识 + 优化解码名展示 | P1 |
| 4 | `app/encv-mobile/src/components/NewTaskModal.vue` 或 `useNewTaskModal.ts` | 支持初始密码预填（可选） | P2 |

---

## 验证步骤

1. **流式预览验证**：
   - 长按 `hyYGPCwJPQ3+xrdAvfnn2.bin` → 点击「流式预览」
   - 输入密码 `8682268` → 确认
   - 应跳转到 Player 页面并开始播放 CAD放样.mp4
   - Player 标题栏应显示 `CAD放样.mp4`（解码后文件名）

2. **信息页验证**：
   - 长按同一文件 → 点击「信息」
   - 应看到红色 "Alist-Encrypt 加密" badge
   - 底部应显示 "原始文件名: CAD放样.mp4"

3. **解密验证**：
   - 长按同一文件 → 点击「解密」
   - 如已输入过密码，应自动填充；否则弹出密码框
   - 任务创建成功后应在 Tasks tab 显示解密任务

4. **回归验证**：
   - 普通 .mp4 文件的播放不受影响
   - ENCV 容器文件（.sccgt）的信息页和预览正常
   - 目录的长按菜单不受影响
