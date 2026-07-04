# 根因修复计划：新建任务 Modal 架构 + 后端 matchType 逻辑修正

> 用户反馈的两个问题本质上是架构错误和数据逻辑错误，不是 UI 层面的补丁能解决的。

---

## 问题诊断

### 问题 1：Modal 绑定 Tasks tab 是架构错误

**现状**：新建任务 modal 通过 inline `<ion-modal :is-open>` 写在 Tasks.vue 模板中。
- FAB 按钮（Tasks tab 内）→ 设 `showNewTaskModal = true` → inline modal 打开 ✅
- Files 长按（跨 tab）→ eventBus → pendingNewTask → onActivated → modalController.create() ⚠️ 两种入口走不同代码路径

**为什么错了**：新建任务是**全局操作**，不应绑定在任何 tab 页面组件上。
- 从 Files.vue 触发时，Tasks.vue 可能不在活跃 tab → Ionic overlay 创建失败
- 之前所有修复（nextTick、onActivated、pendingNewTask）都在给这个错误架构打补丁
- 双轨制（inline modal + modalController.create）增加了维护复杂度

### 问题 2：后端 FindAllEncryptingPlugins 阶段 3 逻辑过宽

**实测证据**（`POST /api/tasks/predict-plugin {"sourcePath":"/mock/video.mp4","type":"encrypt"}`）：

| 插件 | matchType | 是否合理 |
|------|-----------|---------|
| video | `extension` | ✅ 扩展名 .mp4 匹配 |
| audio | `general` | ❌ 音频插件有 audio/ MIME 前缀 |
| image | `general` | ❌ 图像插件有 image/ MIME 前缀 |
| wps | `general` | ❌ WPS 有自己的 MIME/扩展名 |
| pdf | `general` | ❌ PDF 有自己的 MIME/扩展名 |
| text | `general` | ❌ 文本插件有 text/ MIME 前缀 |
| alist_encrypt | `general` | ✅ 真正的通用插件 |

**根因代码** ([registry.go:468-488](internal/v2/plugins/registry.go#L468-L488))：

```go
// 阶段 3: 通用插件 (P1) — 当前逻辑太宽松！
for _, p := range Plugins {
    if !p.ShouldProcess(inputPath) { continue }
    // 只要 ShouldProcess=true 且未被包含 → 全部标记为 general
    // 但 audio/image/pdf/text 的 ShouldProcess 也返回 true！
    candidates = append(candidates, PluginCandidate{
        Plugin: p, Name: p.Name(), MatchType: "general", Priority: 1,
    })
}
```

**各插件的 ShouldProcess 实现**：
| 插件 | ShouldProcess | 支持的类型声明 |
|------|---------------|--------------|
| alist_encrypt | `return true` | **无**（nil + nil）→ 真通用 |
| audio | `return true` | audio/ MIME + mp3/flac/ogg... |
| video | 排除字幕 | video/ MIME + mp4/mkv... |
| image | （待确认） | image/ MIME |
| text | （待确认） | text/ MIME |
| pdf | （待确认） | application/pdf |
| wps | （待确认） | WPS 特定类型 |

**本质矛盾**：audio 插件声明了自己只处理 `audio/` MIME 类型，但 `ShouldProcess` 却对所有路径返回 `true`。阶段 3 不检查类型声明，只要 ShouldProcess=true 就当通用插件加入。

---

## 修复方案

### Step 1：修正后端 FindAllEncryptingPlugins 阶段 3 逻辑

**文件**：`internal/v2/plugins/registry.go` — `FindAllEncryptingPlugins()` 函数

**修改内容**：阶段 3 只加入**真正的通用插件**——即不声明任何 MIME 前缀和扩展名的插件。

```go
// --- 阶段 3: 仅限真正的通用插件 (P1) ---
// 条件：ShouldProcess=true 且 未声明任何 MIME 前缀 且 未声明任何扩展名
// 这样只有 alist-encrypt（nil + nil + always true）会被纳入
for _, p := range Plugins {
    if !p.ShouldProcess(inputPath) { continue }

    hasMimePrefixes := len(p.SupportedMimePrefixes()) > 0
    hasExtensions := len(p.SupportedExtensions()) > 0
    // 声明了特定类型的插件不应该在阶段 3 出现
    // 它们没在阶段 1-2 匹配到 = 这个文件不是它们能处理的类型
    if hasMimePrefixes || hasExtensions { continue }

    alreadyIncluded := false
    for _, c := range candidates {
        if c.Name == p.Name() { alreadyIncluded = true; break }
    }
    if !alreadyIncluded {
        candidates = append(candidates, PluginCandidate{
            Plugin: p, Name: p.Name(), MatchType: "general", Priority: 1,
        })
    }
}
```

**预期效果**（对 /mock/video.mp4）：
| 插件 | matchType | 说明 |
|------|-----------|------|
| video | `extension` | .mp4 扩展名匹配 |
| alist_encrypt | `general` | 真正的通用插件 |

从 7 个候选 → 2 个候选。

**验证方法**：
1. Go 编译通过
2. curl 测试 predictPlugin API 确认返回正确的 matchType
3. 更新 `plugin_candidates_test.go` 测试用例

### Step 2：将新建任务 Modal 从 Tasks.vue 解耦为全局入口

**核心原则**：所有新建任务入口统一使用 `modalController.create({ component: NewTaskModal })`，移除 Tasks.vue 中的 inline `<ion-modal>`。

#### 2a. 新建全局 composable：`useNewTaskModal.ts`

**新文件**：`src/composables/useNewTaskModal.ts`

职责：
- 封装 `modalController.create({ component: NewTaskModal })` 调用
- 管理 NewTaskModal 所需的所有状态（taskType, sourcePath, targetPath, candidates, ...）
- 处理回调桥接（modal 内交互 ↔ 外部状态）
- 提供 `openNewTask(sourcePath?, taskType?)` 统一入口函数

接口设计：
```typescript
export function useNewTaskModal() {
  function openNewTask(sourcePath?: string, taskType?: 'encrypt' | 'decrypt'): Promise<void>
  // 内部管理：candidates, selectedPluginIndex, doPredict, ...
  return { openNewTask }
}
```

#### 2b. 重构 Tasks.vue

**删除**：
- inline `<ion-modal :is-open="showNewTaskModal">...</ion-modal>` 整个模板块
- `showNewTaskModal` ref
- `openPendingNewTaskViaController()` 函数
- pendingNewTask 相关逻辑
- NewTaskModal 导入

**修改 FAB 入口**：
```typescript
import { useNewTaskModal } from '@/composables/useNewTaskModal'
const { openNewTask } = useNewTaskModal()

function showNewTaskSheet() {
  openNewTask()  // 无参数 = 空白新建任务
}
```

**修改 eventBus 监听**：
```typescript
onMounted(() => {
  eventBus.on('open-new-task', (data) => {
    openNewTask(data.sourcePath, data.taskType)
  })
})
```

注意：eventBus 回调中直接调用 `openNewTask()` 即可，不再需要 pendingNewTask + onActivated，
因为 `modalController.create()` 在 document root 层级创建 overlay，不依赖任何 tab 状态。

#### 2c. Files.vue 无需改动

Files.vue 已经通过 `eventBus.emit('open-new-task', {...}) + router.push('/tabs/tasks')` 发送请求。
重构后 Tasks.vue 的 eventBus handler 直接调用 `openNewTask()`，自动处理 modal 弹出。

#### 2d. processQueryAction（直链访问）也统一

```typescript
function processQueryAction() {
  if (route.query.action === 'new') {
    // 收集参数后直接调用 openNewTask()
    const sourcePath = route.query.source as string
    const taskType = (route.query.type || 'encrypt') as TaskType
    router.replace({ path: '/tabs/tasks', query: {} })
    openNewTask(sourcePath, taskType)
  }
}
```

### Step 3：清理遗留代码

- 删除 Tasks.vue 中所有 inline modal 相关的状态变量和函数
- 删除 NewTaskModal.vue 中不再需要的部分（如有）
- 确认 `useTaskForm.ts` 的 doPredict/candidates 仍然被 useNewTaskModal 使用

---

## 验证清单

### 后端验证
- [ ] Go 编译零错误
- [ ] `curl -X POST /api/tasks/predict-plugin {"sourcePath":"/mock/video.mp4"}` 只返回 video(extension) + alist_encrypt(general)
- [ ] `curl -X POST /api/tasks/predict-plugin {"sourcePath":"/mock/doc.txt"}` 返回 text(mime/extension) + alist_encrypt(general)
- [ ] `curl -X POST /api/tasks/predict-plugin {"sourcePath":"/mock/report.pdf"}` 返回 pdf(mime/extension) + alist_encrypt(general)
- [ ] plugin_candidates_test.go 测试更新并全通过

### 前端验证
- [ ] vue-tsc 零错误
- [ ] 全部测试通过（191 个）
- [ ] FAB 按钮 → 弹出空白新建任务 modal
- [ ] FAB → 选路径 → 自动显示对应插件（如选 .mp4 显示 video ★）
- [ ] Files 页长按加密 → 自动跳转 tasks tab + 弹出预填路径的新建任务 modal
- [ ] Files 页长按解密 → 同上
- [ ] 多候选时显示选择器，单候选时显示提示
- [ ] 只有 alist_encrypt 标记为"(通用)"，其余标记为 ★
- [ ] modal 关闭后 tab 路由正常工作

---

## 影响范围

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/v2/plugins/registry.go` | **修改** | FindAllEncryptingPlugins 阶段 3 增加类型声明检查 |
| `internal/v2/plugins/plugin_candidates_test.go` | **修改** | 更新测试用例适配新的过滤逻辑 |
| `src/composables/useNewTaskModal.ts` | **新建** | 全局新建任务 modal composable |
| `src/views/Tasks.vue` | **大幅简化** | 删除 inline modal，改用 useNewTaskModal |
| `src/components/NewTaskModal.vue` | **微调** | 已有回调 props 保持不变 |
| `src/views/Files.vue` | **无需改动** | eventBus.emit 保持不变 |
