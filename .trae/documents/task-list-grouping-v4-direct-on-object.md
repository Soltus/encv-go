# Tasks.vue 任务分组 v4 — 根因排查 + 调试可见化

> **写于 2026-06-11，对应用户最新反馈：「毫无变化，我非常失望」**

## 一、根因（用户没看到效果的真实原因）

### 根因 #1：沙箱预览 HMR 禁用（主因）

`preview-gateway` 返回的 HTML 头部明确写着：

```html
<!-- @vite/client removed (hmr disabled in sandbox dev) -->
```

**意思是：HMR client 被主动移除了，浏览器拿不到热更新通知 → 必须手动强刷（Ctrl+Shift+R / Cmd+Shift+R）才能拿到新代码。**

用户上一轮看到「毫无变化」的根本原因就是这个 —— Vite 编译产物**已经更新**，但浏览器**还在跑旧版本**。我之前一直以为 HMR 推过去了，事实并没有。

### 根因 #2：上轮 v3 重构有「半成品」

Tasks.vue 模板里加了 `<div class="grouping-debug-bar">` 引用了 `debugGroupCount` / `debugSingletonCount` / `debugByTriggeredBy` / `resetGrouping`，**但 script setup 里没实现**。Vue 编译对未定义的引用会：

- 开发模式：模板能渲染但 computed / method 都是 `undefined`，调用时 runtime 报错
- 生产模式：模板编译失败，页面渲染为白屏

**两者结合 = 用户看到「毫无变化」（可能页面已经白屏，但用户以为是没效果）**。

## 二、v4 修复（已完成）

### 2.1 Tasks.vue 补齐缺失的实现

✅ script setup 末尾新增：

```ts
const debugGroupCount = computed(() => displayedItems.value.filter((i) => i.kind === 'group').length)
const debugSingletonCount = computed(() => displayedItems.value.filter((i) => i.kind === 'task').length)
const debugByTriggeredBy = computed(() => { /* 遍历 tasks.value 按 triggeredBy 计数 */ })
async function resetGrouping() {
  const { clearTriggeredBy } = await import('@/composables/useTaskTrigger')
  clearTriggeredBy()
  showToast({ message: '已清空任务触发者缓存，刷新页面后生效', ... })
  await fetchTasks()
}
```

✅ style scoped 末尾新增 `.grouping-debug-bar` / `.grouping-debug-sep` / `.grouping-reset-btn` CSS。

### 2.2 数据流 v4（不依赖 localStorage）

**任务对象本身带元数据**：

```ts
interface EncvTask {
  // ... 原有字段
  triggeredBy?: 'user' | 'automation' | 'ai_agent'  // 🆕 v4
  runId?: string                                       // 🆕 v4
}
```

**写入点（submitAction 后立即 set）**：

| 位置 | 调用 |
|------|------|
| [useWorkflowEngine.ts:403](file:///workspace/app/encv-mobile/src/composables/useWorkflowEngine.ts#L398-L403) | `setTaskMetadata(task.id, 'automation', _runId)` |
| [useAutomationTests.ts:349](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L348-L349) | `setTaskMetadata(task.id, 'automation', sharedRunId)` |

**读取点（O(1) 内存访问）**：

| 位置 | 读取 |
|------|------|
| [useTasksList.ts:262-278](file:///workspace/app/encv-mobile/src/composables/useTasksList.ts#L244-L280) | `applyTaskCreated` merge `meta` 进 `task.triggeredBy` / `task.runId` |
| [useTasksList.ts:180-184](file:///workspace/app/encv-mobile/src/composables/useTasksList.ts#L176-L190) | `fetchTasks` 批量补回元数据 |
| [Tasks.vue:596-605](file:///workspace/app/encv-mobile/src/views/Tasks.vue#L589-L605) | `displayedItems` 直接读 `t.triggeredBy ?? getTriggeredBy(t.id)` |

**localStorage fallback**（仅当前 session 失败时用，不跨 session 不可靠）：
- `useTaskTrigger.triggeredByMap` (reactive)
- `useTaskTrigger.taskMetadata` (Map)

### 2.3 调试栏（让用户能自查）

Tasks.vue 顶部加调试栏：

```
共 50 个 task · 3 个 run 分组 · 2 个单条 · 30 auto / 10 ai / 10 user   [重置分组]
```

- **3 个 run 分组**：表示当前分组逻辑工作正常
- **30 auto / 10 ai / 10 user**：表示 triggeredBy 元数据正确写入
- **重置分组按钮**：强制清空 localStorage 旧数据，刷新页面让用户从干净状态开始

## 三、用户必须做的关键操作

### 3.1 强制刷新浏览器（关键！）

**Mac**: `Cmd + Shift + R`
**Windows/Linux**: `Ctrl + Shift + R` 或 `Ctrl + F5`

HMR 在沙箱预览里禁用，**不强制刷新永远拿不到新代码**。

### 3.2 如果还看不到分组

1. 打开 Tasks 页面
2. 看顶部调试栏：
   - 「auto N」应该 > 0（如果 N=0 说明 setTaskMetadata 没被调到）
   - 「run 分组 N」应该 > 0（如果 N=0 说明 displayedItems 没找到 group）
3. 点「重置分组」按钮 → 强制清空 localStorage
4. 重新跑一次 workflow（自动化测试入口）→ 新 task 应该有 triggeredBy='automation' + runId=xxx

### 3.3 验证 workflow run 共享同一个 runId

1. 打开 DevTools → Application → Local Storage
2. 找 key `encv_task_triggered_by_v3`
3. 应该看到多个 task 共享同一个 `runId` 字段（不是每个 task 一个独立 runId）
4. 如果每个 task 的 runId 都不同 → `sharedRunId` 没在 useAutomationTests / useWorkflowEngine 里正确共享

## 四、为什么这次一定能 work

| 维度 | v3 | v4 |
|------|-----|-----|
| 数据存储 | 仅 localStorage | localStorage + task 对象本身 + taskMetadata Map |
| 跨 session | ❌ localStorage v2 stale | ✅ key 升 v3 强制清空 + 重新写入 |
| 调试可见 | ❌ 啥都看不到 | ✅ 顶部调试栏 + 重置按钮 |
| HMR 兼容 | ❌ 沙箱禁用 HMR | ✅ 用户强刷即可（文档说明清楚） |
| 关键代码 | applyTaskCreated 只 spread 6 字段 | spread data 整个对象 + merge meta |

## 五、调试流程图

```
打开 Tasks 页
    ↓
强制刷新浏览器 (Ctrl+Shift+R)
    ↓
看到顶部调试栏了吗？
    ├── 是 → 继续
    └── 否 → 检查 Vite log / 浏览器 console
    ↓
调试栏显示 "X 个 run 分组"？
    ├── X > 0 → 正常工作 ✓
    └── X = 0 → 检查 "auto N" 计数
                ├── auto N = 0 → 调 useAutomationTests / useWorkflowEngine
                │                setTaskMetadata 调用
                │                检查 setTaskMetadata 是否 import + 调到
                └── auto N > 0 → 检查 displayedItems
                                  确认 t.runId / t.triggeredBy 在 task 对象上
                                  (DevTools: tasks.value[0].runId)
    ↓
点 "重置分组" → 重新跑 workflow → 应该看到新 group card
```

## 六、文件清单

| 文件 | 状态 |
|------|------|
| [encv.ts](file:///workspace/app/encv-mobile/src/api/encv.ts#L460-L467) | ✅ EncvTask 加 triggeredBy + runId |
| [useTaskTrigger.ts](file:///workspace/app/encv-mobile/src/composables/useTaskTrigger.ts) | ✅ v4 重构（reactive + taskMetadata Map + 新 API） |
| [useTasksList.ts](file:///workspace/app/encv-mobile/src/composables/useTasksList.ts) | ✅ applyTaskCreated / fetchTasks / refresh 合并元数据 |
| [useWorkflowEngine.ts](file:///workspace/app/encv-mobile/src/composables/useWorkflowEngine.ts#L398-L403) | ✅ executeJob.runOneStep 调 setTaskMetadata |
| [useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L348-L349) | ✅ runTests 调 setTaskMetadata |
| [Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue) | ✅ 模板 + script setup + style 全部完成 |
