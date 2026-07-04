# Tasks.vue 任务组聚合重构（单次测试 → 单组 + 插件下聚合子任务）

> **目标**：在 Tasks.vue 实现用户要求的展示层级
> 1. **单次测试所有任务聚合为一组** —— 一次「运行全部测试」触发的 N 个 task → 算 1 个 group card
> 2. **单个插件任务下面聚合子任务** —— 每个 plugin 作为 sub-section header，下挂该 plugin 的所有 task 卡片
>
> **核心 bug**：当前 `useAutomationTests.runTests()` 在 for 循环内**每个 task 都用 `Date.now()` 生成不同的 runId**，导致 Tasks.vue 永远看不到 run 分组。

---

## 一、当前状态分析（Phase 1 探索结果）

### 1.1 关键 bug：runTests 每次循环都生成新 runId

`/workspace/app/encv-mobile/src/composables/useAutomationTests.ts:343`：

```typescript
for (const spec of specs) {
  // ...
  recordTriggeredBy(task.id, 'automation', `at-${Date.now().toString(36)}-${spec.id}`)
  //                                                  ^^^^^^^^^^^^^^^^ 每次循环都不同
}
```

**症状**：一次 runTests 提交 200 个 case → 200 个不同的 runId → Tasks.vue 的 `getRunIdForTask(taskId)` 各自独立 → 没有任何 task 能聚成 1 个 group。

**对照参考**：`useWorkflowEngine.runWorkflow` 已经做对了（L249 共享 `run.id` 传给 `executeJob`，L387 `_runId = _runIdOverride || jobRun.id`，L398 调 `recordTriggeredBy(task.id, 'automation', _runId)`）。

### 1.2 当前 Tasks.vue 显示逻辑（v3 终版，按 runId 分组）

文件 `/workspace/app/encv-mobile/src/views/Tasks.vue:466-539`：

- GROUP_FOLD_THRESHOLD=2（≥2 个 task 的 group 折叠成 1 张卡片）
- DisplayItem union：`{ kind: 'group' | 'task' }`
- group 内部展开时按 filteredTasks 顺序插入 N 张 task card（**扁平**展示，无 2 级嵌套）
- **缺**：plugin 级别的 sub-section header

### 1.3 EncvTask 已有 pluginName 字段

`/workspace/app/encv-mobile/src/api/encv.ts:445`：

```typescript
export interface EncvTask {
  // ...
  pluginName?: string  // ← 可用，按 pluginName 分桶
}
```

后端 `createTask` 已经会传 `pluginName`（L496），所有 task 都有这个字段。

### 1.4 i18n key 现状

`/workspace/app/encv-mobile/src/i18n/tasks.ts` 已有：
- `tasks.triggeredBy_automation` / `tasks.triggeredBy_ai_agent`
- `tasks.tasksCount`（"个任务"）
- `tasks.expand` / `tasks.collapse`

需要新增：
- `tasks.pluginsCount`（"个插件"）
- `tasks.pluginSubSection`（"插件 · 24 个任务" 或 "AList 加密 · 4 个任务"）

---

## 二、关键决策（已基于用户指示确定）

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 嵌套层级 | **2 级**：Run group card → plugin sub-section → N 张 task card | 用户原话「单个插件任务下面聚合子任务」明确 2 级，plugin 是 section 不是 sub-group |
| plugin sub-section 折叠 | **跟随 Run 展开**（无独立折叠） | 实现简单，符合 group 语义；如未来需要可独立加 `expandedPluginKeys` |
| 单次测试的 runId | **fix bug**：runTests 入口生成 1 个 runId，循环内复用 | 是当前 Tasks.vue 看不到分组的根因，必须修 |
| DisplayItem 扩展 | 加 `{ kind: 'plugin_section'; ... }` | 既要 Render plugin 段头，又要在段内塞 task 卡片 |
| 聚合阈值 | 保留 GROUP_FOLD_THRESHOLD=2；plugin 段**不设阈值**（段头始终存在） | user task 永远单独展示的现有规则不变 |
| 现有 group card UI | **保留**（4px border + icon-bubble + summary badges + chevron） | 已有精美设计，不动 |
| 兼容旧数据 | **不兼容** | 跟 v3 终版决策一致，没有 runId 的 task 单条展示 |

---

## 三、具体改动

### Phase 1：修复 useAutomationTests.runTests 共享 runId

**文件**：[useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts)

**改动**：
- L309 `runTests` 入口生成 1 个 `runId`（循环外，1 次）
- L343 循环内复用这个 runId，删 `Date.now()`

```typescript
// ❌ 当前：每个 task 1 个不同 runId（破坏聚合）
recordTriggeredBy(task.id, 'automation', `at-${Date.now().toString(36)}-${spec.id}`)

// ✅ 修复：1 个 run 1 个 runId，所有 task 共享
const runId = `at-${Date.now().toString(36)}`  // 🆕 循环外生成
// ... 循环内
recordTriggeredBy(task.id, 'automation', runId)  // 🆕 共享
```

**为什么这个改动是关键**：Tasks.vue 的 `getRunIdForTask` 是按 taskId 索引到 runId 的；只有同 runId 的 task 才会被归到同一 group。fix 完这一步，Tasks.vue 的 group 聚合才会真正生效。

### Phase 2：Tasks.vue DisplayItem 扩展 + 2 级嵌套聚合

**文件**：[Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue)

#### 2.1 类型扩展（L449-451）

```typescript
// 🆕 2026-06-10 修复：2 级嵌套（Run → plugin → task）
type DisplayItem =
  | { kind: 'group'; key: string; groupKey: string; tone: 'automation' | 'ai_agent'; tasks: EncvTask[]; summary: { ... } }
  | { kind: 'plugin_section'; key: string; pluginName: string; tasks: EncvTask[]; subSummary: { passed: number; failed: number; running: number; pending: number; percent: number } }
  | { kind: 'task'; key: string; task: EncvTask }
```

#### 2.2 displayedItems computed 重构（L466-539）

核心算法：

```
输入：filteredTasks
输出：DisplayItem[]

1. O(n) 单次扫描：
   - triggeredBy === 'user' 或无 runId → singletonTasks（走单条）
   - 有 runId → groupsByRun[runId] 内按 pluginName 进一步分桶
     → groupsByRun[runId].plugins[pluginName] = EncvTask[]

2. group 按最早 createdAt 倒序排

3. 输出顺序：
   - singletonTasks → kind:'task'（按 filteredTasks 顺序）
   - 每个 group：
     - if g.tasks.length < 2 → 内部按 plugin 拆 N 个 kind:'task'（不显示 plugin header）
     - else if 未 expanded → 1 张 kind:'group'（保留现有 group card）
     - else expanded → N 个 kind:'plugin_section'（每个 plugin 一个）
                  → 每个 plugin_section 内部 M 张 kind:'task' 卡片
```

#### 2.3 模板新增 plugin_section kind 分支（L145-308 ion-list 模板）

```vue
<!-- 🆕 plugin sub-section header（在 task 卡片前） -->
<div v-if="item.kind === 'plugin_section'" class="plugin-sub-section" :class="`plugin-tone-${item.tone}`">
  <div class="plugin-sub-icon">
    <ion-icon :icon="extensionPuzzle"></ion-icon>
  </div>
  <div class="plugin-sub-info">
    <span class="plugin-sub-name">{{ item.pluginName }}</span>
    <span class="plugin-sub-count">· {{ item.tasks.length }} {{ t('tasks.tasksCount') }}</span>
  </div>
  <div class="plugin-sub-badges">
    <ion-badge v-if="item.subSummary.passed > 0" color="success" class="status-badge">✓ {{ item.subSummary.passed }}</ion-badge>
    <ion-badge v-if="item.subSummary.failed > 0" color="danger" class="status-badge">✗ {{ item.subSummary.failed }}</ion-badge>
    <ion-badge v-if="item.subSummary.running > 0" color="warning" class="status-badge">▶ {{ item.subSummary.running }}</ion-badge>
    <ion-badge v-if="item.subSummary.pending > 0" color="medium" class="status-badge">{{ item.subSummary.pending }}</ion-badge>
  </div>
  <div class="plugin-sub-progress-track">
    <div class="plugin-sub-progress-fill" :style="{ width: item.subSummary.percent + '%' }"></div>
  </div>
</div>
```

#### 2.4 plugin sub-section 样式（L610-1030 scoped style）

```css
/* 🆕 2026-06-10：plugin sub-section（2 级嵌套 group） */
.plugin-sub-section {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px 6px 48px;  /* 左侧 48px 缩进（对应 group card 左侧 4px border + 40px icon） */
  background: rgba(79, 140, 255, 0.04);
  border-bottom: 1px solid rgba(79, 140, 255, 0.1);
  font-size: 13px;
}
.plugin-sub-section.plugin-tone-ai_agent {
  background: rgba(139, 92, 246, 0.04);
  border-bottom-color: rgba(139, 92, 246, 0.1);
}
.plugin-sub-icon {
  width: 24px; height: 24px;
  display: flex; align-items: center; justify-content: center;
  background: rgba(79, 140, 255, 0.15);
  border-radius: 50%;
  font-size: 14px;
  color: var(--ion-color-primary);
}
.plugin-tone-ai_agent .plugin-sub-icon {
  background: rgba(139, 92, 246, 0.15);
  color: var(--ion-color-secondary);
}
.plugin-sub-info {
  display: flex; align-items: baseline; gap: 4px; flex: 1;
}
.plugin-sub-name { font-weight: 600; color: var(--ion-color-dark); }
.plugin-sub-count { font-size: 12px; color: var(--encv-text-secondary); }
.plugin-sub-badges { display: flex; gap: 4px; }
.plugin-sub-progress-track {
  position: absolute; left: 48px; right: 12px; bottom: 2px;
  height: 2px; background: rgba(0,0,0,0.04); border-radius: 1px; overflow: hidden;
}
.plugin-sub-progress-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--ion-color-primary), var(--ion-color-primary-shade));
}
.plugin-tone-ai_agent .plugin-sub-progress-fill {
  background: linear-gradient(90deg, var(--ion-color-secondary), var(--ion-color-secondary-shade));
}
```

注意：plugin sub-section 必须是 `display: block` 而不是 ion-item（避免 sliding / detail 等干扰），所以用 `div` + 自定义样式 + 绝对定位的 progress track（始终在底部）。

### Phase 3：辅助函数 buildPluginSectionItem

**文件**：[Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue)

加在 buildGroupItem 旁边（L541-568）：

```typescript
/**
 * 构造 plugin sub-section item（2 级嵌套内的段头）
 * 跟 group card 类似但更轻量（不折叠、无 chevron、无滑动）
 */
function buildPluginSectionItem(
  pluginName: string,
  tasks: EncvTask[],
  tone: 'automation' | 'ai_agent',
): DisplayItem {
  let passed = 0, failed = 0, running = 0, pending = 0
  for (const t of tasks) {
    if (t.status === 'completed') passed++
    else if (t.status === 'failed') failed++
    else if (t.status === 'running' || t.status === 'cancelling') running++
    else pending++
  }
  const finished = passed + failed
  const percent = tasks.length > 0 ? Math.round((finished / tasks.length) * 100) : 0
  return {
    kind: 'plugin_section',
    key: `plugin-${pluginName}-${tone}`,
    pluginName,
    tasks,
    subSummary: { passed, failed, running, pending, percent },
  }
}
```

### Phase 4：i18n 新增 key

**文件**：[tasks.ts](file:///workspace/app/encv-mobile/src/i18n/tasks.ts)

中英文各加：

```typescript
// 🆕 2026-06-10：2 级嵌套 plugin sub-section
'tasks.pluginSection': '插件',  // 中文
'tasks.pluginSection': 'Plugin',  // 英文
```

（如果 `tasksCount` 已经够用，可以不加新 key，直接复用 `· N {{ t('tasks.tasksCount') }}`）

---

## 四、代码改动汇总

| 文件 | 改动 | 行数估计 |
|------|------|---------|
| `useAutomationTests.ts` | 1 处：runTests 入口生成 runId + 循环内复用 | ~3 行 |
| `Tasks.vue` | 1 处类型扩展 + 1 处 displayedItems 重构 + 1 处模板新增 plugin_section 分支 + 1 处 buildPluginSectionItem + 1 处 CSS | ~80 行 |
| `tasks.ts` (i18n) | 可选 1 行新 key | 0-2 行 |

**总计**：~85 行代码改动

---

## 五、验证步骤

### 5.1 单元 / 集成测试

由于 `useAutomationTests` 和 `Tasks.vue` 都是 Vue 3 composable + 组件，建议手动验证为主：

1. **后端先起**：`cd /workspace && pm2 list` 确认 backend / preview-gateway 都 healthy
2. **mobile overlay 起**：`pm2 restart preview-gateway` 确认 `ENCV_MOBILE=1` 生效
3. **生成 mock 数据**：在 AutomationTestsDetail.vue 点「生成 Mock」按钮（X-Confirm-Mock-Mutation header 自动带）
4. **加载 plugins**：在同页点「加载插件列表」
5. **运行 workflow**：点「Run Workflow」（现在底层是 `useWorkflowEngine.runWorkflow`，已经传对 runId）
6. **切到 Tasks tab**：
   - 应该看到 1 张 group card（automation 蓝色，标题「自动化 · N 个任务」）
   - 点 chevron 展开
   - 应该看到 N 个 plugin sub-section header（每个 plugin 1 个，名字 + count + 4 个 badge + 2px progress track）
   - 每个 plugin section 下面是该 plugin 的所有 task 卡片
7. **运行"全部测试"**（如果还有走 useAutomationTests.runTests 的入口）：
   - 现在应该聚合为 1 张 group（修 bug 后生效）

### 5.2 视觉验证

- group card 左侧 4px 蓝色 border（automation）/ 紫色 border（ai_agent）
- group card 内部每个 plugin sub-section：48px 缩进 + 24×24 icon bubble + plugin name + count + badges
- plugin sub-section 之间有 1px 分隔线（不重叠）
- task 卡片（ion-item-sliding）在 plugin sub-section 下方，按 filteredTasks 顺序

### 5.3 折叠交互验证

- 折叠 group card → 整个 run 折叠，只看到外层 group card
- 展开 group card → 看到所有 plugin sub-section + 所有 task 卡片
- plugin sub-section **不**独立折叠（决策点 2）

### 5.4 性能验证

- 200 case 的测试 → 1 张 group card + N 个 plugin sub-section（典型 7 个 plugin）+ 200 张 task 卡片
- 滚动性能：ion-list 虚拟滚动已启用，无明显卡顿
- WS 事件实时性：4 件套监听已就绪（useTaskEventBridge），task 状态变化 → group summary 实时更新

### 5.5 边界场景

| 场景 | 预期行为 |
|------|---------|
| 1 个 plugin、1 个 task | 走 singletonTasks（< GROUP_FOLD_THRESHOLD），不显示 group card |
| 1 个 plugin、10 个 task | 1 张 group card + 1 个 plugin sub-section + 10 张 task 卡片 |
| 7 个 plugin、各 20 个 task | 1 张 group card + 7 个 plugin sub-section + 140 张 task 卡片 |
| 混 user task 和 automation task | user task 单条展示，automation task 聚 1 个 group，**不混** |
| 跨 run 的 task | 每个 run 各自 1 张 group（已通过 runId 隔离） |
| 无 pluginName 的 task | plugin sub-section 用 `'(unknown plugin)'` 兜底，单独成 1 段 |

---

## 六、风险与回退

### 风险

| 风险 | 缓解 |
|------|------|
| 改了 runTests 共享 runId 破坏 AI agent 入口的 runId 隔离 | `runTests` 是自动化测试入口，AI agent 走 `useWorkflowEngine` 已有独立 runId，不冲突 |
| 2 级嵌套让 DOM 体积变大 | ion-list 虚拟滚动天然只渲染可视区，200+ 卡片无压力 |
| plugin sub-section 视觉太抢眼 | CSS 用 0.04 alpha 浅背景 + 1px 边线，不抢 task 卡片焦点 |

### 回退

- Phase 1（runId fix）是**独立**的；即使 Tasks.vue 暂不重构，至少 runId 已经正确，group 折叠功能可工作
- Phase 2-4 是 Tasks.vue 内的纯展示改动，不影响任何数据流；可单独 revert

---

## 七、执行顺序（建议）

1. **Phase 1**（fix runId）— 1 处改动，3 行
2. **Phase 2.1-2.3**（Tasks.vue 改造）— 主工作量
3. **Phase 3**（buildPluginSectionItem 辅助）
4. **Phase 4**（i18n，可选）
5. 跑一次完整 automation test，截图前后对比

预计总改动：~85 行，1 个文件主改（Tasks.vue）+ 1 个文件 3 行 fix（useAutomationTests.ts）+ 可选 i18n。
