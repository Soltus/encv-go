# 测试基础设施规范（Mock + 后门协议）

> **核心原则：零侵入、配置驱动、可开关。**
> **所有 mock 和后门机制不得修改 `config.user.json` 或任何生产代码逻辑路径。**

> **完整内容 + 历史踩坑**：[详情文档](../rule-library/test.md)

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

**Mock 数据规范详细**（子目录支持、插件列表、其他端点）→ 详见 [详情文档 §1.3](../rule-library/test.md#13-mock-数据规范)

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

**5 个后门函数的实现细节**（simulateLongPress / simulateFileClick / addMockFile / triggerActionSheet / openNewTaskModal） → 详见 [详情文档 §2.3](../rule-library/test.md#23-后门函数规格)

### 2.3 后门注册机制

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

### 3.2 必须覆盖的测试场景（T1-T10）

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

## 四、禁止事项（速查）

1. **❌ 禁止修改 `config.user.json`** — Mock 数据完全自包含
2. **❌ 禁止在生产构建中包含后门代码** — 使用 `import.meta.env.DEV` 守卫
3. **❌ 禁止在后门中跳过权限检查** — 后门只模拟用户操作，不绕过业务逻辑
4. **❌ 禁止硬编码具体后缀值到 spec/测试代码** — 使用 `__mock_suffix` 参数或 `TEST_SUFFIX` 注入
5. **❌ 禁止将 mock 文件提交到版本控制的 `mock/` 目录以外** — 保持隔离

---

## 五、引用其他规则

- [mock-data-architecture.md](./mock-data-architecture.md) — Mock 字节必须同源
- [development.md](./development.md) — dev mode 激活条件
- [test-master-plan.md](./test-master-plan.md) — **测试体系总纲**：Cypress 为主、Go bench 为辅、性能测试方法论

> 拆分：2026-06-11
> 更新：2026-07-01（新增 test-master-plan 引用）
