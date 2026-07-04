# 测试基础设施规范（详情）

> **本文件为 [test.md](../rules/test.md) 的详情文档**。包含 Mock 文件系统详细规范（子目录支持、插件列表、6 类 Mock 端点）、5 个后门函数的完整实现细节、T1-T10 详细测试场景、5 条禁止事项。

---

## 一、Mock 文件系统（Vite 中间件层）

### 1.1 实现位置

```
app/encv-mobile/
├── vite.config.ts          ← 引入 mock 插件（仅 DEV 模式）
├── mock/
│   ├── index.ts            ← Vite 插件入口，注册中间件
│   ├── file-system.ts      ← Mock 文件系统数据定义
│   └── handlers.ts         ← API 请求处理器
```

### 1.2 激活方式

| 方式 | 触发条件 | 作用域 |
|------|---------|--------|
| **自动激活** | `import.meta.env.DEV === true` 且后端离线（`/health` 返回非 200） | 开发环境默认 |
| **手动强制** | URL 参数 `?__mock=1` 或 localStorage key `encv_mock_enabled=true` | 覆盖自动检测 |
| **禁用** | URL 参数 `?__mock=0` 或 localStorage key `encv_mock_enabled=false` | 强制走真实 API |

**检测优先级**：URL 参数 > localStorage > 自动检测

### 1.3 Mock 数据规范

#### 1.3.1 根目录文件列表（`GET /api/files?path=/`）

```typescript
interface MockFileItem {
  name: string
  path: string
  isDirectory: boolean
  isEncrypted?: boolean
  size?: number
  modified?: string
}
```

**必须覆盖的文件类型**：

| 类别 | 文件名示例 | 特征字段 |
|------|-----------|---------|
| 普通视频 | `sample.mp4`, `movie.mkv` | `isDirectory=false` |
| 普通音频 | `music.mp3`, `podcast.flac` | — |
| 普通图片 | `photo.jpg`, `screenshot.png` | — |
| 普通文档 | `report.pdf`, `notes.txt` | — |
| **加密文件（alist-encrypt）** | `secret.ae`（后缀来自配置） | 后缀匹配 `plugin_settings.alist_encrypt.suffix` |
| **ENCV 容器文件** | `video.sccgv`（后缀来自插件配置） | `isEncrypted=true` |
| 子目录 | `Movies/`, `Documents/` | `isDirectory=true` |

#### 1.3.2 子目录支持

- `/Movies/` → 包含容器格式视频文件
- `/Documents/` → 包含普通文档和加密文档
- 支持任意深度的目录导航（至少 2 层）

#### 1.3.3 插件列表（`GET /api/plugins`）

```typescript
interface MockPluginMeta {
  name: string
  supportedExtensions: string[]
  supportedMimePrefixes: string[]
  containerExtension: string
  taskOptions: TaskOptions
}
```

**必须返回的插件**：

| 插件名 | containerExtension | 用途 |
|--------|-------------------|------|
| `video` | `.sccgv` | 视频加密容器 |
| `image` | `.sccgi` | 图片加密容器 |
| `audio` | `.sccga` | 音频加密容器 |
| `text` | `.sccgt` | 文本加密容器 |
| `wps` | `.sccgwps` | WPS 加密容器 |
| `pdf` | `.sccgpdf` | PDF 加密容器 |

#### 1.3.4 其他 Mock 端点

| 端点 | Mock 响应 |
|------|----------|
| `GET /health` | `{ status: 'ok' }` （当 mock 激活时） |
| `GET /api/config` | 含 `plugin_settings.alist_encrypt.suffix` 的完整配置 |
| `GET /api/tasks` | `[]` （空任务列表） |
| `POST /api/tasks` | 返回模拟任务对象 |
| `GET /api/files/tags` | `[]` （空标签列表） |
| `GET /api/permissions` | `{ storage: true }` |

### 1.4 配置读取协议

**关键约束**：Mock 层**不修改** `config.user.json`，但需要知道当前配置值。

**解决方案**：
1. Mock 层硬编码一套**默认配置**（与 config.user.json 的 schema 默认值一致）
2. 通过 `GET /api/config` 端点返回此默认配置
3. 如果前端已从真实后端获取过配置（localStorage 缓存），mock 层的配置不会覆盖它
4. **alist-encrypt 后缀**通过 URL 参数动态注入：`?__mock_suffix=.ae`

---

## 二、测试后门协议（Backdoor API）

### 2.1 后门设计原则

| 原则 | 说明 |
|------|------|
| **零生产影响** | 所有后门代码在 `import.meta.PROD` 时被 tree-shaking 移除 |
| **显式标记** | 所有后门函数以 `__test` 前缀或 `window.__ENCV_TEST` 命名空间暴露 |
| **可审计** | 后门调用全部输出 `console.warn('[TEST-BACKDOOR] ...')` 日志 |
| **权限控制** | 仅在 DEV 模式 + 特定条件下可用 |

### 2.2 全局测试命名空间

```typescript
declare global {
  interface Window {
    __ENCV_TEST: {
      simulateLongPress: (fileName: string) => Promise<void>
      simulateFileClick: (fileName: string) => Promise<void>
      getMockFiles: () => MockFileItem[]
      setMockFiles: (files: MockFileItem[]) => void
      addMockFile: (file: MockFileItem) => void
      removeMockFile: (name: string) => void
      navigateToPath: (path: string) => void
      getCurrentFiles: () => FileItem[]
      triggerActionSheet: (fileName: string) => Promise<void>
      openNewTaskModal: (sourcePath?: string, taskType?: 'encrypt' | 'decrypt') => Promise<void>
    }
  }
}
```

### 2.3 后门函数规格

#### 2.3.1 `simulateLongPress(fileName: string)`

**功能**：模拟对指定文件的 long-press 操作，弹出 action sheet。

**实现方式**：
1. 在 Files.vue 的 `setup()` 中检测 `window.__ENCV_TEST` 存在性
2. 查找 `files.value` 中 name 匹配的 FileItem
3. 调用 `handleLongPress(file)`

**使用场景**：
```javascript
// 浏览器控制台或 Puppeteer
await window.__ENCV_TEST.simulateLongPress('secret.ae')
// → 弹出 action sheet，包含"流式预览"+"解密"按钮
```

#### 2.3.2 `simulateFileClick(fileName: string)`

**功能**：模拟对指定文件的单击操作。

**使用场景**：
```javascript
await window.__ENCV_TEST.simulateFileClick('sample.mp4')
// → 触发 handleFileClick → 可能跳转播放器
```

#### 2.3.3 `getMockFiles() / setMockFiles() / addMockFile() / removeMockFile()`

**功能**：运行时读写 mock 文件列表。

**使用场景**：
```javascript
// 动态添加一个加密文件
window.__ENCV_TEST.addMockFile({
  name: 'test-secret.ae',
  path: '/test-secret.ae',
  isDirectory: false,
  size: 1024,
  modified: '2026-05-30T12:00:00Z'
})
```

#### 2.3.4 `triggerActionSheet(fileName: string)`

**功能**：`simulateLongPress` 的别名，语义更明确。

#### 2.3.5 `openNewTaskModal(sourcePath?, taskType?)`

**功能**：直接打开新建任务 modal。

**使用场景**：
```javascript
await window.__ENCV_TEST.openNewTaskModal('/test.txt', 'encrypt')
```

### 2.4 后门注册机制

**位置**：`src/composables/useTestBackdoor.ts`（新文件，仅 DEV 编译）

```typescript
// useTestBackdoor.ts — 仅在 import.meta.env.DEV 时被导入
export function useTestBackdoor(files: Ref<FileItem[]>, options: {
  onLongPress: (file: FileItem) => Promise<void>
  onClick: (file: FileItem) => Promise<void>
  navigateTo: (path: string) => void
}) {
  if (import.meta.env.DEV) {
    window.__ENCV_TEST = {
      simulateLongPress: async (name: string) => {
        const file = files.value.find(f => f.name === name)
        if (!file) throw new Error(`[TEST-BACKDOOR] File not found: ${name}`)
        console.warn(`[TEST-BACKDOOR] simulateLongPress(${name})`)
        await options.onLongPress(file)
      },
      // ... 其他方法
    }
  }
  return window.__ENCV_TEST
}
```

**Files.vue 集成**：
```vue
<script setup lang="ts">
// 仅 DEV 模式导入（PROD 时被消除）
if (import.meta.env.DEV) {
  const { useTestBackdoor } = await import('@/composables/useTestBackdoor')
  useTestBackdoor(files, {
    onLongPress: handleLongPress,
    onClick: handleFileClick,
    navigateTo: navigateTo,
  })
}
</script>
```

---

## 三、浏览器自动化测试协议

### 3.1 agent-browser 测试标准流程

每个测试场景必须遵循以下步骤：

```
Step 1: 导航到 localhost:5173?__mock=1&__mock_suffix=.ae
Step 2: 等待页面稳定（ion-content 可见）
Step 3: 执行操作（点击/长按/导航）
Step 4: 截图验证 UI 状态
Step 5: 检查 console 日志（error/warn 数量）
Step 6: 清理状态（关闭 modal/sheet）
```

### 3.2 必须覆盖的测试场景

| # | 场景 | 操作 | 验证点 |
|---|------|------|--------|
| T1 | 根目录文件列表渲染 | 访问 `/tabs/files` | 显示 mock 文件（含各类型） |
| T2 | 普通文件长按菜单 | `simulateLongPress('sample.mp4')` | 出现播放+预览+加密操作 |
| T3 | alist-encrypt 加密文件长按 | `simulateLongPress('secret.ae')` | 出现流式预览+解密 |
| T4 | ENCV 容器文件长按 | `simulateLongPress('video.sccgv')` | 出现解密（container 模式） |
| T5 | 目录导航 | 点击子目录 | 文件列表更新为子目录内容 |
| T6 | 插件 tab 过滤 | 切换到 plugin view | 只显示匹配扩展名的文件 |
| T7 | 新建任务 modal | `openNewTaskModal('/test', 'encrypt')` | Modal 打开且表单预填充 |
| T8 | Tab 切换稳定性 | Home→Files→Tasks→Settings→Files | 无 RouterOutlet 冻结 |
| T9 | 控制台日志清洁度 | 执行 T1-T8 后统计 | 0 error, warn 可接受 |
| T10 | 动态添加 mock 文件 | `addMockFile()` 后刷新 | 新文件出现在列表中 |

### 3.3 测试结果判定标准

| 级别 | 标准 | 处理方式 |
|------|------|---------|
| ✅ PASS | 0 error + 功能正常 | 记录通过 |
| ⚠️ WARN | 有 warn 但功能正常 | 记录并 LOW 优先级修复 |
| ❌ FAIL | error 或功能异常 | 阻塞，必须修复后重新测试 |

---

## 四、禁止事项

1. **❌ 禁止修改 `config.user.json`** — Mock 数据完全自包含
2. **❌ 禁止在生产构建中包含后门代码** — 使用 `import.meta.env.DEV` 守卫
3. **❌ 禁止在后门中跳过权限检查** — 后门只模拟用户操作，不绕过业务逻辑
4. **❌ 禁止硬编码具体后缀值到 spec/测试代码** — 使用 `__mock_suffix` 参数或 `TEST_SUFFIX` 注入
5. **❌ 禁止将 mock 文件提交到版本控制的 `mock/` 目录以外** — 保持隔离
