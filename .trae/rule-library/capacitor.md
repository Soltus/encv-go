# Capacitor / Ionic Vue 架构铁律（详情）

> **本文件为 [capacitor.md](../rules/capacitor.md) 的详情文档**。包含 11 章节全部实战案例：Reactive State Object 完整代码、6 Layer 饱和调试攻击、Ionic 8 toggle 暗黑模式 8.1-8.7 全部细节、useProxiedFetch SSE 修复清单、主题色系统完整架构、7 个预设色板 + 对比色算法。

---

## 一、Modal 架构

### 1.1 禁止 inline `<ion-modal :is-open>` 用于跨 tab 操作

**❌ 错误（Ionic Vue 8 已知 bug）**：
```vue
<!-- Tasks.vue — inline modal 绑定在 Tasks 组件 DOM 树内 -->
<ion-modal :is-open="showNewTaskModal">
  <NewTaskModal @close="showNewTaskModal = false" />
</ion-modal>
```

**症状**：
- 在 Files tab 通过 eventBus 发送 `open-new-task` 事件
- Tasks.vue 的 `onMounted` 未执行（tab 未激活）→ 监听器未注册 → **事件丢失**
- 用户必须先手动切到 Tasks tab 再回来，modal 才能工作
- Tab 切换时 overlay 可能闪烁、卡顿、或不渲染

**根因**：inline `<ion-modal :is-open>` 的 overlay 挂载在父组件的 DOM 树内。当父组件所在的 tab 非活跃时，Ionic 的路由 outlet 可能会销毁/隐藏该 DOM 子树，导致：
1. overlay 元素被从 document 中移除或隐藏
2. `is-open` 变更无法触发正确的动画/显示逻辑
3. 即使 `isOpen = true`，用户也看不到任何东西

**✅ 正确（全局 overlay 模式）**：
```typescript
// useNewTaskModal.ts — composable 封装
import { modalController } from '@ionic/vue'

export function useNewTaskModal() {
  async function openNewTask(sourcePath?: string, taskType?: 'encrypt' | 'decrypt') {
    const modal = await modalController.create({
      component: NewTaskModal,
      componentProps: { /* ... */ },
    })
    await modal.present()  // ← 挂载在 <body> 根节点，与 tab 无关
  }
  return { openNewTask }
}
```

### 1.2 `modalController.create()` 的 componentProps 是静态快照

**关键认知**：`componentProps` 在 `create()` 时被**一次性快照**传入子组件。后续对原始对象的修改**不会自动反映**到子组件。

**❌ 错误（扁平 props 快照断裂）**：
```typescript
const modal = await modalController.create({
  component: NewTaskModal,
  componentProps: {
    sourcePath: initialSourcePath,          // ← 值快照！
    candidates: [],                         // ← 空数组快照！
    onUpdateSourcePath: async (v) => {
      // 这里更新的是闭包变量，但 NewTaskModal 收到的 props.candidates 不会变
      await doPredict(v)
      // candidates.value 更新了，但 props 不变！
    },
  },
})
```

**✅ 正确（Reactive State Object 模式）**：
```typescript
const state = reactive<NewTaskState>({
  sourcePath: initialSourcePath || '',
  candidates: [],
  // ... 所有状态字段
})

const modal = await modalController.create({
  component: NewTaskModal,
  componentProps: {
    state,  // ← 传入 reactive 对象引用！子组件通过 computed 读取最新值
    onUpdateSourcePath: async (v) => {
      state.sourcePath = v        // ← 直接修改对象属性
      await doPredict(v)
      syncState()                // ← 同步内部数据到 state 对象
    },
  },
})
```

**子组件读取模式（双源 fallback）**：
```typescript
// NewTaskModal.vue
const props = defineProps<{ state?: NewTaskState; sourcePath?: string }>()

// 优先读 state（modalController 场景），fallback 到扁平 props（测试场景）
const src = computed(() => props.state?.sourcePath ?? props.sourcePath ?? '')
const cands = computed(() => {
  const arr = props.state?.candidates ?? props.candidates
  return Array.isArray(arr) ? arr : []
})
```

### 1.3 Modal 必须秒开——不要在 present() 前阻塞等待异步数据

**❌ 错误（阻塞式打开）**：
```typescript
async function openNewTask(sourcePath?: string) {
  const state = reactive({...})

  if (sourcePath) {
    await doPredict(sourcePath, 'encrypt')  // ← 500ms 防抖 + API 调用！
    syncState()                              // ← 用户要等 ~1s 才看到 modal
  }

  const modal = await modalController.create({ component: NewTaskModal, componentProps: { state } })
  await modal.present()  // ← 才到这里才展示
}
```

**✅ 正确（先打开后填充）**：
```typescript
async function openNewTask(sourcePath?: string) {
  const state = reactive<NewTaskState>({..., candidates: [], predictedPlugin: null})

  // ① 先创建并立即展示 modal（空状态）
  const modal = await modalController.create({ component: NewTaskModal, componentProps: { state } })
  await modal.present()

  // ② 后台预测插件，完成后 reactive state 自动驱动 UI 更新
  if (sourcePath) {
    const norm = normalize(sourcePath)
    if (norm) {
      await doPredict(norm, state.taskType as 'encrypt' | 'decrypt')
      syncState()  // ← 数据到达后 UI 自动刷新
    }
  }
}
```

**用户体验对比**：

| | 阻塞式 | 秒开式 |
|---|--------|--------|
| 点击→看到 modal | ~800-1500ms | <100ms |
| 插件数据到达 | 同时出现 | modal 打开后 600ms 内渐进加载 |
| 用户感知 | "卡了" / "没反应" | "秒开 + 数据加载中" |

### 1.4 modalController.create() 不需要 router.push 切 tab

**❌ 错误（幽灵路由跳转）**：
```typescript
function handleEncryptFile(file: FileItem) {
  eventBus.emit('open-new-task', { sourcePath: path, taskType: 'encrypt' })
  router.push({ path: '/tabs/tasks' })  // ← 完全多余！modal 是全局 overlay
}
```

**后果**：用户在 Files tab 长按加密 → 路由跳到 Tasks tab → 上下文丢失、位置重置、体验割裂。

**✅ 正确**：
```typescript
// Files.vue — 直接调用 composable，不走 eventBus 中转
const { openNewTask } = useNewTaskModal()

function handleEncryptFile(file: FileItem) {
  openNewTask(resolveFileItem(file), 'encrypt')  // ← 全局 overlay，当前 tab 不变
}
```

---

## 二、eventBus 跨组件通信铁律

### 2.1 禁止跨 tab 的 eventBus 依赖

**致命反模式**：
```
Files.vue ──emit('open-new-task')──> 🌀 [Tasks.vue 未挂载 = 无监听器]
                                        ↓
用户首次打开 App → 在 Files tab → Tasks.vue onMounted 未执行
→ eventBus.on('open-new-task', handler) 从未注册 → 事件永久丢失
```

**正确架构**：

| 通信场景 | 推荐方式 | 原因 |
|---------|---------|------|
| 同组件内（parent ↔ child） | props / emits / v-model | Vue 原生响应式 |
| 跨 tab 的操作触发 | **直接 import composable 调用** | 不依赖目标组件生命周期 |
| 任务创建后通知列表刷新 | eventBus（自消费） | Tasks.vue 自己监听自己需要的事件 |
| WebSocket 消息分发 | eventBus（自消费） | DevLogs.vue 自己监听 |

### 2.2 eventBus 安全使用清单

每次使用 eventBus 前必须确认：

- [ ] **发射者和监听者在同一组件？** ✅ 安全
- [ ] **监听者的 onMounted 一定会在 emit 之前执行？** ⚠️ 危险——tab 切换顺序不可控
- [ ] **onUnmounted 会正确注销？** ✅ 必须配对
- [ ] **事件名是否全局唯一？** ✅ 使用命名空间 `domain:action`

**本项目安全事件清单**：

| 事件 | 发射者 | 监听者 | 跨 tab? | 安全 |
|------|--------|--------|---------|------|
| ~~`open-new-task`~~ | ~~Files.vue~~ | ~~Tasks.vue~~ | ~~是~~ | ~~**已消除**~~ |
| `task:update` | useNewTaskModal.onSubmit → WS | Tasks.vue | 否（同组件自消费） | ✅ |
| `task:progress` | TaskManager.worker → WS | Tasks.vue | 否 | ✅ |
| `task:created` | useNewTaskModal.onSubmit → WS | Tasks.vue | 否 | ✅ |
| `task:completed` | TaskManager.process* → WS | Tasks.vue | 否 | ✅ |
| `task:refresh` | useNewTaskModal.onSubmit | Tasks.vue | 否 | ✅ |
| `file:change` | TaskManager.process* → WS | Files.vue | 否（自消费） | ✅ |
| `ws:message` | WebSocket client | DevLogs.vue | 否（自消费） | ✅ |
| `server:status` | WebSocket client | DevLogs.vue | 否（自消费） | ✅ |

---

## 三、Tab 切换稳定性

### 3.1 Ionic Tabs 路由机制

```
/tabs/
├── home       → HomePage.vue      (lazy loaded)
├── files      → Files.vue         (lazy loaded)
├── tasks      → Tasks.vue         (lazy loaded)
├── remote     → Remote.vue        (lazy loaded)
├── settings   → Settings.vue      (lazy loaded)
└── devlogs    → DevLogs.vue       (lazy loaded)
```

**关键行为**：
- `<ion-router-outlet>` 使用 `keep-alive` 缓存已访问过的 tab 组件
- 但 `onMounted` / `onUnmounted` 只在首次进入和销毁时触发一次
- **切换 tab 不会重新触发 mounted/unmounted**
- `onIonViewWillEnter` / `onIonViewDidLeave` 是 Ionic 生命周期钩子，每次切 tab 都触发

### 3.2 确保稳定的最佳实践

1. **数据获取放在 `onIonViewWillEnter`**（而非 `onMounted`），确保每次切回都刷新
2. **禁止在 `onMounted` 中注册跨 tab 事件的监听器**（见 §2.1）
3. **composable 调用是安全的**——它在模块级别初始化，不依赖组件生命周期
4. **modalController.create() 是最稳定的跨 tab 操作方式**——overlay 挂载在 `<body>` 上

### 3.3 Tab 切换不影响的功能

| 功能 | 是否受 tab 影响 | 原因 |
|------|----------------|------|
| `modalController.create()` | ❌ 不影响 | overlay 在 `<body>` 根节点 |
| `alertController.create()` | ❌ 不影响 | 同上 |
| WebSocket 连接 | ❌ 不影响 | 在 App 层级管理 |
| eventBus 自消费事件 | ❌ 不影响 | 同组件内收发 |
| **composable 直接调用** | ❌ 不影响 | 模块级别初始化 |
| inline ion-modal | ⚠️ **受影响** | DOM 树绑定在 tab 组件内 |
| 跨 tab eventBus | ⚠️ **受影响** | 目标组件可能未挂载 |

---

## 四、防抖/异步时序陷阱

### 4.1 doPredict 防抖 + await 的组合爆炸

**useTaskForm.ts 的 doPredict 内部有 500ms setTimeout 防抖**。如果调用方用 `await doPredict()`：

```
t=0ms:   doPredict() → 设置 500ms timer
t=0ms:   await ??? → 需要 timer resolve + API 返回
t=500ms: timer 触发 → 开始 API 调用
t=500ms+X: API 返回 → resolve()
```

**总延迟 = 500ms(防抖) + API耗时(~100-300ms) = 600-800ms**

如果调用方还加了 `await setTimeout(600)` 固定等待（旧代码）：
- API 快时：浪费 100ms
- API 慢时：**syncState 在 API 返回前执行 → 拿到空数据**

### 4.2 正确做法

**调用方**：`await doPredict()` 即可（它返回 Promise，内部自行处理防抖+API）

**展示方**：先 present modal，再 await doPredict（见 §1.3）

---

## 五、调试检查清单

当 modal 不显示 / 数据不更新 / tab 切换异常时，按此顺序排查：

1. **modal 是否用了 `modalController.create()`？** 如果是 inline `:is-open`，改用 create
2. **是否在 present() 前有长时间 await？** 改为 present 后再异步加载数据
3. **componentProps 是否用了 reactive state object？** 如果是扁平 props，检查数据流
4. **是否有不必要的 router.push？** modalController.create 不需要切路由
5. **eventBus 是否跨 tab？** 改为直接 import composable 调用
6. **doPredict 是否被正确 await？** 检查 Promise 链完整性
7. **syncState() 是否在 API 返回后调用？** 检查时序

---

## 六、Tab 切换异常：RouterOutlet 冻结（实战踩坑！）

> **核心原则：Vue render function 崩溃会阻塞 Ionic RouterOutlet 的组件切换流程。**
> **任何子组件的未捕获渲染异常都可能导致整个 tab 切换机制瘫痪。**

### 6.1 症状

```
用户操作序列：
  首页 → Files（✅ 正常显示文件列表）
       → Tasks（❌ 内容仍显示文件列表，TabBar 高亮已切换到 Tasks）
       → Remote（❌ 内容仍显示文件列表）
       → Settings（❌ 内容仍显示文件列表）
       → DevLogs（❌ 内容仍显示文件列表）
       → Files（✅ 切回正常）

关键特征：
- URL 路由正确变化（/tabs/files → /tabs/tasks ...）
- TabBar 高亮正确切换
- 但 <ion-router-outlet> 内容区域「冻结」在离开前的组件视图
- 无白屏、无闪烁、无卡顿 —— 不是渲染慢而是**根本没切换**
```

### 6.2 根因链路

```
Files.vue 的 <IonMenu menu-id="plugin-menu"> 内：
  <IonItem v-for="plugin in plugins">
    {{ plugin.supportedExtensions.length }}   ← API 返回 null
                    ↓
         💥 Cannot read properties of null (reading 'length')
                    ↓
    Vue render function 异常（未捕获）
                    ↓
Ionic RouterOutlet 组件切换流程中断
                    ↓
目标组件(Tasks/Remote/Settings/DevLogs) 
的 onMounted/onIonViewWillEnter 永远不触发
                    ↓
内容区域「冻结」在 Files 视图
```

### 6.3 为什么只影响从 Files 切出？

因为崩溃发生在 **Files 组件的 IonMenu 子树渲染中**。当 RouterOutlet 尝试从 Files 切换到其他组件时：
1. Vue 开始卸载/更新 Files 组件树
2. IonMenu 内的 `v-for` 触发重新渲染
3. 访问 `null.length` 抛出异常
4. 异常中断了 RouterOutlet 的切换事务
5. 目标组件永远不会挂载

切**回** Files 时正常，因为 Files 已经是当前组件，不需要切换。

### 6.4 触发崩溃的三类代码模式

| 模式 | 示例 | 修复 |
|------|------|------|
| **模板中直接访问可能为 null 的属性** | `{{ plugin.supportedExtensions.length }}` | 使用可选链 + 默认值：`{{ plugin.supportedExtensions?.length ?? 0 }}` |
| **脚本中无防御访问 API 返回值** | `if (plugin.supportedExtensions.length === 0)` | 前置 null 检查：`if (!plugin.supportedExtensions \|\| plugin.supportedExtensions.length === 0)` |
| **使用字符串字面量引用未导入的图标** | `return 'film-outline'`（但未 import filmOutline） | 导入图标变量并使用引用：`return filmOutline` |

### 6.5 排查此类问题的诊断方法：饱和调试攻击

#### 步骤 1：确认症状层级

```
Layer 1: URL 变了但内容没变？
  → 检查 Router beforeEach/afterEach 是否配对
  → 如果有 beforeEach 无 afterEach → 路由守卫卡住

Layer 2: URL 变了 + 路由正常但组件没挂载？
  → 检查目标组件 onMounted/onIonViewWillEnter 是否触发
  → 如果没触发 → RouterOutlet 切换被中断

Layer 3: 有 JS 错误？
  → 检查控制台是否有 "Cannot read properties of null/undefined"
  → 检查是否有 "Unhandled error during render function"
```

#### 步骤 2：注入全链路 error 日志（饱和调试）

将所有关键路径临时升格为 `console.error('[SAT-DBG]...')`，使其在 DevLogs 中以红色高亮显示：

```typescript
// Layer A: Tabs 核心
// Tabs.vue — ionTabsWillChange + ionTabsDidChange
function onTabsWillChange(event: CustomEvent) {
  const tab = event?.detail?.tab ?? '(unknown)'
  console.error('[SAT-DBG][Tabs] ionTabsWillChange →', tab, '| ts=', Date.now())
}

// Layer B: 页面生命周期（每个 tab 组件）
onMounted(() => {
  console.error('[SAT-DBG][ComponentName] onMounted | ts=', Date.now())
})
onIonViewWillEnter(() => {
  console.error('[SAT-DBG][ComponentName] onIonViewWillEnter | ts=', Date.now())
})

// Layer C: 通信总线
function on(event, handler) {
  console.error('[SAT-DBG][eventBus] on(', event, ') | totalListeners=', count, '| ts=', Date.now())
}
function emit(event, data) {
  console.error('[SAT-DBG][eventBus] emit(', event, ') →', listenerCount, 'listeners | ts=', Date.now())
}

// Layer D: WebSocket
function connect() {
  console.error('[SAT-DBG][WS] connect() → connecting | url=', url, '| ts=', Date.now())
}
ws.onopen = () => {
  console.error('[SAT-DBG][WS] onopen → connected | ts=', Date.now())
}

// Layer E: 路由导航守卫
router.beforeEach((to, from) => {
  console.error('[SAT-DBG][Router] beforeEach |', from.path, '→', to.path, '| ts=', Date.now())
})
router.afterEach((to, from) => {
  console.error('[SAT-DBG][Router] afterEach  |', from.path, '→', to.path, '| ts=', Date.now())
})

// Layer F: Overlay 系统
console.error('[SAT-DBG][Modal] create(ComponentName) | ts=', Date.now())
await modal.present()
modal.onDidDismiss().then(() => {
  console.error('[SAT-DBG][Modal] didDismiss() | ts=', Date.now())
})
```

#### 步骤 3：分析日志时序定位卡点

**正常的 tab 切换日志时序**（以 Home → Files 为例）：
```
[Router] beforeEach  | /tabs/home → /tabs/files     ← 路由开始切换
[Tabs]   ionTabsWillChange → files            ← Ionic tabs 感知
[Files]  onIonViewWillEnter                 ← 目标组件准备进入
[Files]  onMounted                          ← 目标组件首次挂载
[Files]  eventBus.on(file:change)           ← 事件监听注册
[Tabs]   ionTabsDidChange  → files           ← Ionic tabs 确认完成
[Router] afterEach  | /tabs/home → /tabs/files ← 路由切换完成
```

**异常信号（本案例实际日志）**：
```
[Router] beforeEach  | /tabs/files → /tabs/tasks   ✅ 正常
[Tabs]   ionTabsWillChange → (unknown)        ✅ 正常
[Tabs]   ionTabsDidChange  → (unknown)        ✅ 正常
[Router] afterEach  | /tabs/files → /tabs/tasks   ✅ 正常
    ⚠️ [Tasks] onMounted                      ❌ 缺失！
    ⚠️ [Tasks] eventBus.on ×5                ❌ 缺失！
    💥 Cannot read properties of null (reading 'length')  ← 崩溃点！
```

#### 步骤 4：根据缺失的日志段定位根因

| 缺失的日志段 | 含义 | 下一步 |
|-------------|------|--------|
| `[Tabs]` WillChange/DidChange 缺失 | Tabs 事件未发射或 handler 崩溃 | 检查 Tabs.vue 事件处理器是否防御了 `event.detail` 为 undefined |
| `[Component]` onMounted 缺失 | 组件从未挂载 | 检查前一个组件是否有 render function error |
| `[eventBus]` on 缺失 | 事件监听器未注册 | 检查组件 onMounted 是否执行 |
| beforeEach 后无 afterEach | 路由导航卡死 | 检查路由守卫是否有异常抛出 |
| `null reading 'length'` 错误 | **本案例的直接原因** | 定位到具体行号，添加 null 安全访问 |

### 6.6 预防规则

1. **所有 API 返回值的属性访问必须使用可选链**：`data?.field?.subField ?? fallback`
2. **图标名称必须使用导入的变量引用**，禁止字符串字面量（除非是动态计算的）
3. **v-for 渲染的数据源必须有空值保护**：`v-if="Array.isArray(items)"` 或 computed 中过滤
4. **Ionicons 图标必须先 import 再使用**——Vue 编译期无法检测运行时的图标解析失败
5. **子组件的 render error 会向上传播影响父级**（包括 RouterOutlet），不是局部问题

---

## 七、控制台日志降噪规范

### 7.1 日志级别使用铁律

| 级别 | 使用场景 | 用户可见性 | DevLogs 显示 |
|------|---------|-----------|-------------|
| `console.error` | **真正的错误**（API 失败、不可恢复的异常） | 🔴 红色高亮 | 始终显示 |
| `console.warn` | **可忽略的预期内情况**（fallback 路径、非关键功能降级） | 🟡 黄色 | 可过滤 |
| `console.debug` | **开发调试信息**（API 请求/响应、状态变更） | ⚪ 灰色 | 默认隐藏 |
| `console.info` | **业务关键节点**（任务创建、权限结果） | ⚪ 灰色 | 默认显示 |
| `console.log` | 兼容性输出（hijackConsole 重定向为 info） | ⚪ 灰色 | 默认显示 |

### 7.2 本项目已降噪的模式

| 原始代码 | 降级后 | 原因 |
|---------|--------|------|
| `console.warn('[Files] Unknown play mode')` | `console.debug(...)` | 有 artplayer fallback，非错误 |
| `console.warn('Failed to apply screen orientation')` | `console.debug(...)` | 非 native 平台预期行为 |
| `console.warn('[Settings] MPV plugin load failed')` | `console.debug(...)` | 插件未安装的正常状态 |
| `console.warn('[ArtPlayer] handleFullscreenEnter error')` | `console.debug(...)` | StatusBar API 在 web 端不可用 |
| API 层全部 `console.debug` | 保持不变 | 请求/响应详情仅在开发时需要 |
| `console.error('[API] xxx failed')` | **保持 error** | 真正的网络/服务端错误 |

### 7.3 禁止的模式

- ❌ 用 `console.error` 输出调试信息（会污染生产环境错误日志）
- ❌ 用 `console.warn` 输出可通过代码逻辑处理的路径（应使用 debug）
- ❌ 在循环/高频回调中使用任何级别的 console（性能杀手）

---

## 八、ion-toggle 暗黑模式完整适配（实战踩坑！）

> **核心原则：`ion-toggle` 在暗黑模式下有两大问题——label 文字不可见 + 轨道/手柄颜色异常。**
> **两者根因不同，需要分别解决。**

### 8.1 问题一：label 文字在暗黑模式下不可见

**根因**：`:label` prop 渲染在 Shadow DOM 内部，`::part(label)` 穿透在 `<style scoped>` 中不可靠。

**❌ 错误（label 在 Shadow DOM 内）**：
```vue
<ion-toggle :label="t(field.label)" justify="space-between" />
```

**✅ 正确（light DOM 模式）**：
```vue
<ion-item lines="none">
  <ion-label>{{ t(field.label) }}</ion-label>
  <ion-toggle slot="end" :checked="..." @ionChange="..." />
  <ion-note slot="helper">{{ t(field.help) }}</ion-note>
</ion-item>
```

### 8.2 问题二：ON 状态手柄在暗黑模式下变黑

**根因链路**：
```
toggle 在 ion-item 内部 → 获得 .ion-color 类
→ Ionic 8 内部规则：:host(.ion-color.toggle-checked) .toggle-inner{background:var(--ion-color-base)}
→ 暗黑模式下 --ion-color-base = #121212（深色）
→ ON 状态手柄被覆盖为黑色，--handle-background-checked CSS 变量失效
```

**关键认知**：
1. **Ionic 8 的 CSS 变量名与 v5 不同**——用错变量名等于没设置
2. **CSS 变量对 ON 状态手柄可能不生效**——被 `.ion-color.toggle-checked` 内部规则覆盖
3. **`::part(handle)` 必须在非 scoped 样式中使用**
4. **样式必须放在 App.vue 的非 scoped `<style>` 块中**

### 8.3 Ionic 版本差异（v5 vs v8 变量名）

| 功能 | Ionic v5（❌ 过时） | Ionic 8 ✅ |
|------|---------------------|-------------|
| 轨道背景 (OFF) | `--background` | **`--track-background`** |
| 轨道背景 (ON) | `--background-checked` | **`--track-background-checked`** |
| 手柄背景 (OFF) | `--handle-background` | `--handle-background`（相同） |
| 手柄背景 (ON) | `--handle-background-checked` | **`--handle-background-checked`**（相同） |

### 8.4 ✅ 最终解决方案（已验证有效）

**文件位置**：App.vue — 非 scoped `<style>` 块

```css
/* 方案 A: CSS 变量控制轨道 + OFF 手柄 */
ion-toggle {
  --track-background: #424242;
  --track-background-checked: var(--ion-color-primary);
  --handle-background: var(--ion-color-primary);
}

/* 方案 B: ::part() 穿透控制 ON 手柄（覆盖 .ion-color 上下文变黑） */
ion-toggle.toggle-checked::part(handle) {
  background: #ffffff;
}
```

| 状态 | 轨道 | 手柄 |
|------|------|------|
| **OFF** | `#424242` 灰色 | 主题蓝色 |
| **ON** | 主题蓝色 | **白色** |

### 8.5 禁止的做法

| 做法 | 原因 |
|------|------|
| 用 v5 变量名 (`--background`) | Ionic 8 不识别，静默忽略 |
| 在 `<style scoped>` 中用 `::part()` | 与 `[data-v-xxx]` 冲突，浏览器忽略 |
| 在 `variables.css` 中用 `::part()` | Vite 可能不正确处理 |
| 用 `!important` | 违反规范，且可能被 `contain: strict` 阻断 |

### 8.6 必须导入的组件清单

```typescript
import {
  IonContent, IonHeader, IonPage, IonToolbar, IonTitle,
  IonButtons, IonButton,
  IonItem, IonLabel, IonSelect, IonSelectOption,
  IonInput, IonIcon, IonSpinner,
  IonToggle, IonNote, modalController,
} from '@ionic/vue'
```

### 8.7 ExtraField 类型分支渲染

```vue
<!-- bool: ion-label + toggle slot="end" -->
<ion-item v-if="field.type === 'bool'" lines="none">
  <ion-label>{{ t(field.label) }}</ion-label>
  <ion-toggle slot="end" :checked="..." @ionChange="..." />
  <ion-note slot="helper">{{ t(field.help) }}</ion-note>
</ion-item>

<!-- select: ion-select label prop（无 Shadow DOM 问题） -->
<ion-item v-else-if="field.type === 'select'" lines="none">
  <ion-select :label="t(field.label)" interface="action-sheet">...</ion-select>
</ion-item>

<!-- string/password: ion-input label prop -->
<ion-item v-else lines="none">
  <ion-input :label="t(field.label)" :type="field.type" />
</ion-item>
```

## 九、useProxiedFetch 流式 Header 铁律（Android 真机实战踩坑！）

> **核心原则：native 模式下所有走 SSE 的 fetch 必须显式声明 `Accept: text/event-stream`，否则 useProxiedFetch 走 fetchOnce() 一次性读完所有 chunk，processLegacySSE reader 同步消费 → 没有逐字流式效果。**

### 9.1 症状

```
dev 模式（vite）：        agent 流式输出正常（原生 fetch 走 WebView SSE 拆分）
Android 真机（WebView）：agent 整体工作正常，但**没有流式效果**
                          — 用户看到的是一次性完整回复，不是逐字打字机效果
```

User 报告原文：「实测安卓真机是正常的，但是安卓真机 agent 流式输出没有生效！」

### 9.2 根因链路

```
useAgent.send() / confirmTool() / runResumeChain() 三处的 fetch headers
  → 没传 Accept: text/event-stream
  → useProxiedFetch.proxiedFetch() 的 isStream 判断：
       isStream = init.isStream === true
         || headers['Accept']?.includes('text/event-stream')
         || headers['accept']?.includes('text/event-stream')
  → 三个条件全 false → isStream=false
  → 走 ApiProxy.fetchOnce() 分支：
       new Response(body, ...)  ← body 是完整字符串，不是 ReadableStream
  → processLegacySSE reader.read() 一次读完所有 chunk
  → 没有「逐字 enqueue」的流式效果
```

### 9.3 修复模式

**❌ 错误（fetch headers 缺 Accept）**：
```typescript
const response = await fetch(`${getAgentBase()}/api/chat`, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    ...(sendAGUIHeader ? { 'X-Agent-Protocol': 'agui' } : {}),
  },
  body: JSON.stringify({...}),
})
```

**✅ 正确（加 Accept: text/event-stream）**：
```typescript
const response = await fetch(`${getAgentBase()}/api/chat`, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'text/event-stream',  // ← 必传！触发 ApiProxy.streamStart
    ...(sendAGUIHeader ? { 'X-Agent-Protocol': 'agui' } : {}),
  },
  body: JSON.stringify({...}),
})
```

### 9.4 当前已修复的 SSE 端点

| 端点 | 文件位置 | 状态 |
|------|---------|------|
| `/api/chat` | useAgent.send() L2162-2175 | ✅ 已加 Accept |
| `/api/confirm` | useAgent.confirmTool() L2421-2431 | ✅ 已加 Accept |
| `/api/resume` | useAgent.runResumeChain() L2521-2531 | ✅ 已加 Accept |

### 9.5 新增 SSE 端点时的 checklist

任何新增的 SSE/streaming 端点必须检查：

- [ ] fetch headers 是否带 `Accept: text/event-stream`？
- [ ] 后端是否返回 `Content-Type: text/event-stream; charset=utf-8`？
- [ ] dev 模式下原生 fetch 能处理（WebView 自带 SSE 拆分）？
- [ ] Android 真机走 `useProxiedFetch` 触发 `streamStart` 路径？

**判断流式分发的代码位置**：[useProxiedFetch.ts#L166-181](file:///workspace/app/encv-mobile/src/composables/useProxiedFetch.ts)

```typescript
const isStream = init.isStream === true
  || (headers['Accept']?.includes('text/event-stream') ?? false)
  || (headers['accept']?.includes('text/event-stream') ?? false)
```

### 9.6 dev 模式 vs native 模式对比

| 模式 | fetch 实现 | isStream 缺失影响 | 现象 |
|------|----------|------------------|------|
| dev（vite） | 原生 fetch（useProxiedFetch 不安装） | 无影响（WebView 自带 SSE 拆分） | 流式正常 |
| Android 真机 | useProxiedFetch → ApiProxy.streamStart/fetchOnce | **致命**（走 fetchOnce 一次性读完） | 无流式 |
| Web 生产 | 原生 fetch | 无影响 | 流式正常 |

### 9.7 单元测试覆盖

- [src/composables/__tests__/useProxiedFetch.test.ts](file:///workspace/app/encv-mobile/src/composables/__tests__/useProxiedFetch.test.ts) L"SSE 请求（Accept: text/event-stream）走 ApiProxy.streamStart" 用例覆盖了此判断逻辑

### 9.8 调试技巧

流式失效时在 Android 真机 logcat 查：

```bash
adb logcat | grep -E "useProxiedFetch|ApiProxy"
# 应看到 installProxiedFetch 启动信息 + stream:data 事件流
# 若只有 streamStart 一次、立即 stream:end → 走的是 fetchOnce 路径
```

---

## 十、跨层参考

| 主题 | 文档位置 |
|------|---------|
| **WAF/代理截断 `@` 字符 → 双重编码方案** | [development.md §六](development.md#六waf代理截断路径参数实战踩坑) |
| 配置合并加载（Default → user → dev） | [config.go](internal/config/config.go) |
| **Mobile Overlay 机制（mobile→顶层映射）** | [project_rules.md §Mobile Overlay 机制](project_rules.md#mobile-overlay-机制核心架构) |
| **主题色系统** | [useTheme.ts](app/encv-mobile/src/composables/useTheme.ts) |

---

## 十一、主题色系统

> **核心原则：通过 JS 动态设置 `--ion-color-primary-*` CSS 变量实现运行时切换 primary 色。**
> **不依赖 Ionic 的 CSS 自定义属性文件生成，完全运行时驱动。**

### 10.1 架构概览

```
useTheme.ts (composable)
├── THEME_PRESETS: ThemePreset[]     ← 7 个预设颜色
├── currentColor: Ref<string>        ← 当前激活颜色响应式引用
├── applyColor(color)                ← 核心：动态设置所有 --ion-color-primary-* 变量
├── setThemeColor(color)             ← 公开 API：调用 applyColor
├── initTheme()                      ← 初始化：读取 localStorage + prefers-color-scheme
└── localStorage('encv-theme-color') ← 持久化存储

Settings.vue (UI)
├── 预设圆点选择器 (.color-dot × 7)   ← 点击即切换
├── 自定义颜色输入 (<input type="color">)  ← 取色器 + HEX 显示
└── 实时预览 (.color-hex)             ← 显示当前 HEX 值
```

### 10.2 预设色板

| 名称 | 色值 | 视觉 |
|------|------|------|
| Blue | `#4f8cff` | 🔵 默认 |
| Purple | `#8b5cf6` | 🟣 |
| Green | `#22c55e` | 🟢 |
| Orange | `#f97316` | 🟠 |
| Red | `#ef4444` | 🔴 |
| Pink | `#ec4899` | 💗 |
| Teal | `#14b8a6` | 🔵 |

### 10.3 核心算法：applyColor()

```typescript
function applyColor(color: string) {
  const root = document.documentElement
  const rgb = hexToRgb(color)
  const contrast = getContrastColor(color)

  root.style.setProperty('--ion-color-primary', color)
  root.style.setProperty('--ion-color-primary-rgb', rgb)
  root.style.setProperty('--ion-color-primary-contrast', contrast)
  root.style.setProperty('--ion-color-primary-contrast-rgb', hexToRgb(contrast))
  root.style.setProperty('--ion-color-primary-shade', darker(color, 10))
  root.style.setProperty('--ion-color-primary-tint', lighter(color, 10))
}
```

**自动生成的衍生变量**：

| 变量 | 算法 | 用途 |
|------|------|------|
| `--ion-color-primary` | 原始色值 | 按钮、FAB、链接、toggle ON 轨道 |
| `--ion-color-primary-rgb` | `r, g, b` 格式 | rgba() 内联使用 |
| `--ion-color-primary-contrast` | 基于亮度反色 | 按钮文字（白底黑字/黑底白字） |
| `--ion-color-primary-shade` | 暗 10% | 按压态 |
| `--ion-color-primary-tint` | 亮 10% | hover 态 |

### 10.4 对比色算法

```typescript
function getContrastColor(hex: string): string {
  const luminance = (0.299 * R + 0.587 * G + 0.114 * B) / 255
  return luminance > 0.5 ? '#000000' : '#ffffff'
}
```

使用 ITU-R BT.601 亮度公式判断，确保按钮文字在任意背景色上可读。

### 10.5 localStorage 键名规范

| 键名 | 类型 | 值示例 | 用途 |
|------|------|--------|------|
| `encv-theme-preference` | string | `'dark'` / `'light'` | 暗黑模式偏好 |
| `encv-theme-color` | string | `'#8b5cf6'` | 自定义主题色 |
| `encv-locale` | string | `'zh-CN'` / `'en'` | 语言偏好 |

### 10.6 Settings.vue UI 结构

```vue
<ion-item lines="full">
  <ion-icon :icon="colorPaletteOutline" slot="start"></ion-icon>
  <ion-label>
    <h3>{{ t('settings.themeColor') }}</h3>
    <p>{{ t('settings.themeColorHelp') }}</p>
  </ion-label>
</ion-item>
<div class="theme-color-picker">
  <div class="preset-colors">
    <button v-for="preset in THEME_PRESETS"
      class="color-dot" :class="{ active: currentColor === preset.value }"
      :style="{ backgroundColor: preset.value }"
      @click="setThemeColor(preset.value)" />
  </div>
  <div class="custom-color-row">
    <label>{{ t('settings.customColor') }}</label>
    <input type="color" :value="currentColor" @input="setThemeColor(...)" />
    <span class="color-hex">{{ currentColor.toUpperCase() }}</span>
  </div>
</div>
```

**交互行为**：
- 点击预设圆点 → 即时切换，选中项放大 1.15x + 加深边框
- 使用取色器选择自定义色 → 即时应用 + HEX 实时显示
- 刷新页面后从 localStorage 恢复上次选择的颜色

### 10.7 注意事项

1. **`applyColor()` 设置在 `document.documentElement` 上**——全局生效，影响所有组件
2. **与 ion-toggle 联动**——toggle 的 `--track-background-checked` 和 `--handle-background` 引用 `var(--ion-color-primary)`，换色后自动跟随
3. **暗黑/亮色模式下均有效**——颜色变量独立于 dark mode class
4. **不支持 CSS 变量的组件需单独处理**——如原生 `<input type="color">` 的样式需要手动覆盖 webkit 伪元素
