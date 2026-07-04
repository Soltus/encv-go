# 修复 DevLogs 自动滚动

## Summary

把 DevLogs 面板的"时间窗启发式"自动滚动（`isUserScrolling` + 1500ms timeout）替换为工业标准 **pinned-to-bottom 模式**：用 `nearBottom` 几何判定取代时间窗；用户上滑期间累积 unreadCount 并显示浮动「↓ N 条新日志」按钮；保留底栏 `ion-toggle` 但语义改为"硬性覆盖"；彻底修复 3 个竞态：用户主动滑动、tab 切回、App 从后台恢复。

## Current State Analysis

### 文件：[`DevLogs.vue`](file:///workspace/app/encv-mobile/src/views/DevLogs.vue)

**当前实现** (lines 193-248)：
```ts
let isUserScrolling = false
let userScrollTimer: ReturnType<typeof setTimeout> | null = null

function onContentScroll() {
  isUserScrolling = true
  if (userScrollTimer) clearTimeout(userScrollTimer)
  userScrollTimer = setTimeout(() => { isUserScrolling = false }, 1500)
  // ...
}

async function scrollToBottom() {
  if (!autoScroll.value || isUserScrolling) return
  const el = await getScrollEl()
  if (el) el.scrollTop = el.scrollHeight
}

watch([filteredFrontend, filteredBackend], () => {
  nextTick(() => scrollToBottom())
}, { deep: true })
```

### 已知 bug（按用户实测）

| # | 现象 | 根因 |
|---|------|------|
| 1 | 新日志出现时**完全不滚到底** | `scrollTop = scrollHeight` 触发的程序化滚动会回调 `ionScroll` → 把 `isUserScrolling` 置 true → 接下来 1500ms 内所有新日志的 `scrollToBottom()` 都被 `if (isUserScrolling) return` 拦截。即使是"不滚期间"也会被新日志的连续程序化滚动永久占满。 |
| 2 | `getScrollElement()` 异步 + `nextTick` 不 await | `nextTick(() => scrollToBottom())` 调度的是函数引用，返回的 Promise 被丢弃 → `scrollTop` 写入可能晚于下一个 watcher 触发。 |
| 3 | `nextTick` 在 `<ion-content>` shadow DOM 完成布局前就执行 | 写入 `scrollTop` 时 `scrollHeight` 尚未包含新行 → 滚不到真正的底部。 |
| 4 | 用户上滑阅读 1.5s 后被强制弹回底部 | `isUserScrolling` 计时器到期后任何新日志都会 jump-cut 阅读位置。 |
| 5 | tab 切回 / App 从后台恢复时无重置 | 离开期间日志累积 → 切回瞬间跳到底部，无视用户原本的滚动位置。 |
| 6 | tab 切换后 `onMounted` 不重跑（Ionic keep-alive） | `nextId` 计数器、状态机、scroll 监听都假设"挂载一次 = 唯一一次"。 |
| 7 | 底栏 `ion-toggle` autoScroll 在 `false` 之外**没意义** | 当 `false` 时所有自动滚被禁；当 `true` 时**仍然被** `isUserScrolling` 抢走控制权——用户感知不到区别。 |

### 同仓库已验证的正确实现

[`AgentChat.vue#L1254-1260`](file:///workspace/app/encv-mobile/src/views/AgentChat.vue#L1254-L1260) 已经在生产中跑过同样的 pinned 模式：

```ts
const nearBottom = ref(true)
function onMainScroll() {
  const el = mainRef.value
  if (!el) return
  const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
  nearBottom.value = distanceFromBottom < 80
}
watch(() => messages.value.length, () => {
  if (nearBottom.value) scrollToBottom()
})
```

[`ScrollToBottomButton.vue`](file:///workspace/app/encv-mobile/src/components/agent/ScrollToBottomButton.vue) 已存在的浮动按钮组件（带 unreadCount badge），可直接复用，但 DevLogs 自己已有 `ion-content` 结构，**改写为内联实现**避免耦合 AgentChat 内部接口。

## Proposed Changes

### 1. `DevLogs.vue` 脚本部分（核心重写）

#### 1.1 新增状态（替换 `isUserScrolling` / `userScrollTimer`）

```ts
// 几何判定：scrollTop + clientHeight 距 scrollHeight < 80px
const NEAR_BOTTOM_THRESHOLD_PX = 80

const nearBottom = ref(true)       // 当前是否在底部（pin 状态）
const unreadCount = ref(0)         // 用户上滑期间累积的"未读"日志条数
const hardPaused = ref(false)      // 底栏 toggle 的"硬性覆盖"：true = 即便在底部也不自动滚
const lastScrollTop = ref(0)       // 用于区分"程序化滚动"与"用户主动滚动"
```

#### 1.2 新增 `updateNearBottom()` — 单一滚动状态判定

```ts
async function updateNearBottom() {
  const el = await getScrollEl()
  if (!el) return
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight
  const wasNear = nearBottom.value
  nearBottom.value = distance < NEAR_BOTTOM_THRESHOLD_PX
  // 关键修复：用户主动上滑（从近底→远底）才累计 unreadCount
  // 程序化滚到底部（远底→近底）不累计
  if (wasNear && !nearBottom.value) {
    // 离开底部的瞬间不增加 unreadCount——等真正"有日志因不在底部而错过"时再加
  }
}
```

**重要约束**：`ionScroll` 事件对程序化滚动和用户滚动都会触发，仅靠"事件触发"无法区分。**用滚轮/触摸事件的位移方向区分**——见 1.6。

#### 1.3 新增 `scrollToBottom(smooth = false)` — 正确的滚动实现

```ts
async function scrollToBottom(smooth = false) {
  const el = await getScrollEl()
  if (!el) return
  // 关键：scrollTo 是异步的，行为由浏览器/Capacitor WebView 保证在布局后执行
  // 避免 scrollTop=scrollHeight 同步赋值的"布局未完成" race
  el.scrollTo({ top: el.scrollHeight, behavior: smooth ? 'smooth' : 'auto' })
  nearBottom.value = true   // 立即置位（程序化滚动保证最终在底）
  unreadCount.value = 0
}
```

**取消** `el.scrollTop = el.scrollHeight` 的同步赋值。

#### 1.4 改写 watcher（核心修复点）

```ts
// 监听"日志数量"和"最后一条日志的 id"——后者覆盖"原地改 message"的场景
// 不要监听整个 filteredXxx（深监听性能差且会被搜索/筛选误触发）
const lastFrontendId = computed(() => filteredFrontend.value.at(-1)?.id ?? 0)
const lastBackendId = computed(() => filteredBackend.value.at(-1)?.id ?? 0)

watch([lastFrontendId, lastBackendId], ([newFid], [oldFid]) => {
  // 只处理当前 tab 的那一个；watcher 同时监听两个 id 但只对当前 tab 生效
  const newId = activeTab.value === 'frontend' ? newFid : newIdFromBackend
  // ... 见 1.5
})
```

**问题**：单一 watcher 难以"只看当前 tab"。**改用两个独立 watcher**：

```ts
watch(lastFrontendId, (newId, oldId) => {
  if (activeTab.value !== 'frontend') return
  handleNewLog()
})
watch(lastBackendId, (newId, oldId) => {
  if (activeTab.value !== 'backend') return
  handleNewLog()
})

function handleNewLog() {
  if (hardPaused.value) return              // 硬性覆盖：不滚
  if (nearBottom.value) {
    scrollToBottom(false)                  // 平滑性交给 isAtBottom 决定
  } else {
    unreadCount.value++                    // 累积未读
  }
}
```

#### 1.5 改写 `onContentScroll` — 区分程序化 vs 用户滚动

**关键点**：`<ion-content>` 的 `ionScroll` 事件在程序化滚动和用户滚动时都会触发。**靠 `wheel` / `touchstart` / `pointerdown` 标记用户主动滚动**。

```ts
let userGestureActive = false

function onUserGestureStart() {
  userGestureActive = true
}
function onUserGestureEnd() {
  // 50ms 延迟：等最后一次惯性滚动触发的 ionScroll 处理完
  setTimeout(() => { userGestureActive = false }, 50)
}

function onContentScroll() {
  // 视觉反馈：滚动条可见
  showScrollbarTemporarily()
  // 用户手势期间：才更新 nearBottom
  if (userGestureActive) {
    void updateNearBottom()
  }
  // 程序化滚动触发的 ionScroll：保持 nearBottom=true（scrollToBottom 已设置）
}
```

模板新增 3 个监听（`@wheel` / `@touchstart` / `@pointerdown`）：

```vue
<ion-content
  ref="contentRef"
  class="log-content"
  @ionScroll="onContentScroll"
  @ionScrollEnd="onContentScrollEnd"
  @wheel.passive="onUserGestureStart"
  @touchstart.passive="onUserGestureStart"
  @pointerdown.passive="onUserGestureStart"
  @touchend.passive="onUserGestureEnd"
  @pointerup.passive="onUserGestureEnd"
  @wheel.passive="onUserGestureEnd"
>
```

> 注：实际只需要 `@wheel` + `@touchstart` + `@touchend` 三件套即可（@pointerdown/up 与 touch 重叠会重复触发，去掉以简化）。

#### 1.6 浮动按钮的 `unreadCount` 计数修正

只在"用户主动离开底部"后到"用户回到底部"之间的日志才计入 `unreadCount`：

```ts
async function updateNearBottom() {
  const el = await getScrollEl()
  if (!el) return
  const wasNear = nearBottom.value
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight
  nearBottom.value = distance < NEAR_BOTTOM_THRESHOLD_PX
  if (!wasNear && nearBottom.value) {
    // 用户主动滑回底部：清空 unread
    unreadCount.value = 0
  }
}
```

`handleNewLog` 已在 1.4 实现：用户在底部 → 自动滚 + 清空；用户不在底部 → `unreadCount++`。

#### 1.7 tab 切回 / App 后台恢复

Ionic `<ion-tabs>` 使用 `keep-alive` 缓存 → DevLogs 组件不重新挂载。需要在 `onIonViewWillEnter`（tab 切回时触发）重置：

```ts
import { onIonViewWillEnter, onIonViewDidEnter, onIonViewDidLeave } from '@ionic/vue'

onIonViewWillEnter(async () => {
  // 切回 tab 时：强制重新计算 nearBottom（DOM 已可见，scrollTop 仍是用户离开时的值）
  await nextTick()
  await updateNearBottom()
  // 切回时如果用户在底部 + autoScroll 开 + 有未读 → 滚到底部
  if (activeTab.value === 'frontend' && nearBottom.value && !hardPaused.value && unreadCount.value > 0) {
    scrollToBottom(false)
  }
  // 同上 backend
})
```

App 前后台切换（visibilitychange）：

```ts
function onVisibilityChange() {
  if (document.visibilityState === 'visible') {
    // 后台 → 前台：DOM 滚动位置可能已失效（iOS Safari background tab 会清空 layout）
    void nextTick(() => updateNearBottom())
  }
}
onMounted(() => document.addEventListener('visibilitychange', onVisibilityChange))
onUnmounted(() => document.removeEventListener('visibilitychange', onVisibilityChange))
```

#### 1.8 底栏 `ion-toggle` 改造

语义从"启用/禁用自动滚"改为"硬性覆盖"：

```vue
<ion-toggle v-model="hardPaused" :label-placement="'start'">
  {{ t('devlogs.autoScroll') }}
</ion-toggle>
```

i18n key 复用 `devlogs.autoScroll` 不变（值仍叫"自动滚动"），但 tooltip 加新 key `devlogs.autoScrollHint`：

```json
"devlogs.autoScrollHint": "关闭后新日志不会自动滚动到底部（仍可点 ↓ 按钮跳回）"
```

> 用户感知：toggle 关闭 = 硬性暂停；toggle 开启 = 智能 pinned 模式（默认）。

#### 1.9 浮动按钮（内联在 `DevLogs.vue` 模板内）

不复用 `ScrollToBottomButton.vue`（它在 AgentChat 上下文里）。**新增一段 inline 模板**：

```vue
<transition name="fade">
  <button
    v-if="!nearBottom && unreadCount > 0"
    type="button"
    class="scrollToBottomBtn"
    :title="t('devlogs.scrollToBottom')"
    @click="onJumpToBottom"
  >
    <ion-icon :icon="arrowDownOutline" class="scrollToBottomIcon" />
    <span class="scrollToBottomBadge">{{ unreadCount > 99 ? '99+' : unreadCount }}</span>
  </button>
</transition>
```

```ts
async function onJumpToBottom() {
  await scrollToBottom(true)  // smooth
}
```

样式（参考 `ScrollToBottomButton.vue` 的 z-index 50、box-shadow；位置在 content 内右下角）：

```css
.scrollToBottomBtn {
  position: absolute;
  right: 16px;
  bottom: 16px;
  z-index: 50;
  width: 40px; height: 40px;
  border: 0; border-radius: 50%;
  background: var(--ion-toolbar-background, var(--ion-background-color));
  color: var(--ion-color-primary);
  box-shadow: 0 3px 10px rgba(0,0,0,0.18), 0 1px 3px rgba(0,0,0,0.12);
  cursor: pointer;
  display: inline-flex; align-items: center; justify-content: center;
  padding: 0;
  transition: transform 0.12s, box-shadow 0.12s;
}
.scrollToBottomBtn:hover { transform: scale(1.06); }
.scrollToBottomBtn:active { transform: scale(0.94); }
.scrollToBottomIcon { font-size: 20px; }
.scrollToBottomBadge {
  position: absolute; top: -2px; right: -2px;
  min-width: 18px; height: 18px; padding: 0 5px;
  border-radius: 9px; background: var(--ion-color-danger, #eb445a);
  color: #fff; font-size: 11px; font-weight: 600;
  line-height: 18px; text-align: center;
  box-shadow: 0 0 0 2px var(--ion-toolbar-background);
}
.fade-enter-active, .fade-leave-active { transition: opacity 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
```

按钮位置用 `position: absolute` 相对 `ion-content`（ion-content 本身 `position: relative`）。

### 2. i18n 新增 key

`/workspace/app/encv-mobile/src/i18n/devlogs.ts`：

```json
"devlogs.scrollToBottom": "跳到最新日志",
"devlogs.newLogsBadge": "{count} 条新日志",
"devlogs.autoScrollHint": "关闭后新日志不会自动滚动到底部（仍可点 ↓ 按钮跳回）"
```

英文版对称添加。

### 3. 单元测试

新建 `/workspace/app/encv-mobile/__tests__/DevLogs.autoScroll.test.ts`，覆盖：

| # | 用例 | 验证点 |
|---|------|--------|
| 1 | mount 后初始 nearBottom=true | 默认状态 |
| 2 | `handleNewLog` 在 nearBottom=true 时调用 `scrollToBottom` | 不累积 unread |
| 3 | `handleNewLog` 在 nearBottom=false 时 `unreadCount++` | 累积 unread |
| 4 | 模拟 `onContentScroll` 后距离 > 阈值 → nearBottom=false | 几何判定 |
| 5 | 模拟滚回底部（距离 < 阈值）→ unreadCount=0 | 清空 |
| 6 | `hardPaused=true` → `handleNewLog` 不滚不累积 | 硬性覆盖 |
| 7 | `onJumpToBottom` → 调用 `scrollToBottom(true)` + 清空 unread | 浮动按钮 |
| 8 | 切 frontend tab → 不响应 backend 日志 | tab 隔离 |
| 9 | 切 backend tab → 不响应 frontend 日志 | tab 隔离 |
| 10 | 切回 tab（`onIonViewWillEnter`）→ 重算 nearBottom | 生命周期 |

> 由于 `ion-content` 的 `getScrollElement()` 依赖 shadow DOM 真实布局，测试中用 `vi.mock('@ionic/vue', ...)` 注入 fake `IonContent` 组件（template render `div.log-fake-scroll`，事件 emit）。

参考既有 [`FilePickerModal.test.ts`](file:///workspace/app/encv-mobile/__tests__/FilePickerModal.test.ts#L31-L42) 的 mock 模式。

## Assumptions & Decisions

| 决策 | 选择 | 理由 |
|------|------|------|
| `ion-toggle` 改名 / 保留 | 保留 + 语义改为"硬性覆盖" | 用户已选；最小 UI 变更 |
| 浮动按钮：复用 `ScrollToBottomButton.vue` 还是内联 | **内联** | 避免 DevLogs 依赖 AgentChat 内部组件；样式可独立调整 |
| 平滑滚动 | 用 `behavior: 'smooth'` 仅在用户点击按钮时 | 自动滚应瞬时（避免新日志被缓动追上），按钮点击可丝滑 |
| `nearBottom` 阈值 | 80px | 与 AgentChat 保持一致；用户感知 1-2 行日志 |
| `unreadCount` 上限 | 99+ | 防止 badge 撑大 |
| 区分"程序化 vs 用户滚动" | `wheel`/`touchstart` 标记 + `ionScroll` 检测 | 唯一可靠方式（ion-content 不暴露 gesture 标志） |
| tab 切回是否强制滚到底 | 否——尊重用户离开时的滚动位置；仅"原本在底部 + 有未读"才滚 | 工业标准（VS Code console、Postman）行为 |
| App 后台恢复是否重置 `unreadCount` | 否——只重算 nearBottom | 后台期间用户没"消费"日志 |

## Verification

### 自动化测试

```bash
cd /workspace/app/encv-mobile
npx vitest run __tests__/DevLogs.autoScroll.test.ts
```

期望：10/10 通过。

### TypeScript 编译

```bash
npx vue-tsc --noEmit
```

期望：0 错误。

### 手工验证矩阵（在 Dev Server 上）

| # | 场景 | 预期 |
|---|------|------|
| 1 | 进入 DevLogs，等待 5s，发送 10 条后端日志 | 滚到底部 |
| 2 | 滚到顶部附近（不在底部），等 10 条新日志 | 不滚；右下角显示「↓ 10 条新日志」按钮 |
| 3 | 点击「↓ 10 条新日志」按钮 | 平滑滚到底部，按钮消失 |
| 4 | 用户主动滚到底部后停留 1s，再发送日志 | 自动滚到底（nearBottom=true → 立即响应） |
| 5 | 切到 Settings tab，停留 10s，再切回 DevLogs | 不强制滚——尊重离开时的位置 |
| 6 | 切走时在底部 + autoScroll 开 + 切回时仍在底部 | 保持 pinned 状态 |
| 7 | 切到其他 App（Cmd+Tab / Home），停留 5s，回来 | nearBottom 重算，行为同 #1 |
| 8 | 关闭底栏 autoScroll toggle | 新日志不滚；浮动按钮仍可点击 |
| 9 | 打开 toggle | 恢复 pinned 模式 |
| 10 | 搜索框输入文本过滤 | 列表变化不影响 nearBottom（搜索是布局变化，不是日志新增） |

### 回归测试

- 后端日志（`useRealtimeTransport` WS 链路）推送频率正常（参考 WS 稳定性提升）
- 前端 console.hijack 收集仍工作（`useFrontendLogs` 模块级单例）
- 清空按钮、复制按钮仍正常

## 不在范围内

- 改用 vue-virtual-scroller 虚拟滚动（当前最多 2000 条，DOM 渲染 OK）
- DevTools 详情页的导出/Logcat 独立窗口
- AgentChat 联动（DevLogs 与 AgentChat 互不感知）
- i18n 翻译（只补 key，文案双语由 `useI18n` 默认占位）

## 引用

- [`DevLogs.vue` 当前实现](file:///workspace/app/encv-mobile/src/views/DevLogs.vue#L193-L248)
- [`useFrontendLogs.ts`](file:///workspace/app/encv-mobile/src/composables/useFrontendLogs.ts)
- [`AgentChat.vue` pinned 模式参考](file:///workspace/app/encv-mobile/src/views/AgentChat.vue#L1254-L1260)
- [`ScrollToBottomButton.vue` 浮动按钮样式参考](file:///workspace/app/encv-mobile/src/components/agent/ScrollToBottomButton.vue)
- [fix-devlogs-mp4-logdisplay.md](file:///workspace/.trae/documents/fix-devlogs-mp4-logdisplay.md) — 上一轮 DevLogs 修复
