# Capacitor / Ionic Vue 架构铁律

> **核心原则：modalController.create() 是全局 overlay，不依赖任何组件生命周期。**
> **任何跨 tab 的 eventBus 依赖都是定时炸弹。**
> **native 模式下 SSE 请求必须显式声明 `Accept: text/event-stream`。**

> **完整内容 + 11 章节实战案例**：[详情文档](../rule-library/capacitor.md)

---

## 一、Modal 架构（反复踩坑）

### 1.1 禁止 inline `<ion-modal :is-open>` 用于跨 tab 操作

**❌ 错误**（inline modal 绑定在 Tasks 组件 DOM 树内）：tab 未激活时 `onMounted` 未执行 → 事件丢失；切回时 modal 闪烁/卡顿。

**✅ 正确（composable + `modalController.create()`）**：
```typescript
export function useNewTaskModal() {
  async function openNewTask(sourcePath?: string, taskType?: 'encrypt' | 'decrypt') {
    const modal = await modalController.create({ component: NewTaskModal, componentProps: { /* ... */ } })
    await modal.present()  // ← 挂载在 <body> 根节点，与 tab 无关
  }
  return { openNewTask }
}
```

### 1.2 `componentProps` 是静态快照 — 必须用 Reactive State Object

`componentProps` 在 `create()` 时被一次性快照传入子组件。后续对原始对象的修改不会自动反映到子组件。

**✅ 正确**：
```typescript
const state = reactive<NewTaskState>({ sourcePath: '', candidates: [], /* ... */ })
const modal = await modalController.create({ component: NewTaskModal, componentProps: { state } })

// 子组件读取（双源 fallback，兼容测试场景）
const src = computed(() => props.state?.sourcePath ?? props.sourcePath ?? '')
const cands = computed(() => props.state?.candidates ?? props.candidates ?? [])
```

### 1.3 Modal 必须秒开 — 不要在 present() 前阻塞

**✅ 先 present，后异步加载**：
```typescript
const modal = await modalController.create({ component: NewTaskModal, componentProps: { state } })
await modal.present()
if (sourcePath) await doPredict(sourcePath, state.taskType)  // 后台渐进加载
```

| | 阻塞式 | 秒开式 |
|---|--------|--------|
| 点击→看到 modal | ~800-1500ms | <100ms |
| 用户感知 | "卡了" | "秒开 + 数据加载中" |

### 1.4 `modalController.create()` 不需要 router.push 切 tab

**❌ 错误**：`router.push({ path: '/tabs/tasks' })` 紧跟 modal 打开 → 上下文丢失、位置重置。
**✅ 正确**：直接 `openNewTask(...)`，全局 overlay 与当前 tab 无关。

---

## 二、eventBus 跨组件通信铁律

**致命反模式**：
```
Files.vue ──emit('open-new-task')──> 🌀 [Tasks.vue 未挂载 = 无监听器]
→ 事件永久丢失（用户首次在 Files tab 操作）
```

**正确架构**：

| 通信场景 | 推荐方式 | 原因 |
|---------|---------|------|
| 跨 tab 的操作触发 | **直接 import composable 调用** | 不依赖目标组件生命周期 |
| 同组件内（parent ↔ child） | props / emits / v-model | Vue 原生响应式 |
| 自消费事件（任务通知 / WS 消息） | eventBus | 同组件内收发 |

**已消除的不安全事件**：`open-new-task`（跨 tab，已改用 composable 直接调用）。**安全事件清单**：`task:*` / `file:change` / `ws:message` / `server:status`。

---

## 三、Tab 切换稳定性

**关键行为**：`<ion-router-outlet>` 用 `keep-alive` 缓存 tab，但 `onMounted` / `onUnmounted` 只触发一次；切 tab 不会重新触发。**数据获取放在 `onIonViewWillEnter`**（而非 `onMounted`）。

**Tab 切换影响矩阵**：

| 功能 | 是否受影响 | 原因 |
|------|----------|------|
| `modalController.create()` / `alertController` / composable 直接调用 / WebSocket / eventBus 自消费 | ❌ 不影响 | 模块级或全局 |
| inline ion-modal | ⚠️ 受影响 | DOM 树绑定在 tab 内 |
| 跨 tab eventBus | ⚠️ 受影响 | 目标组件可能未挂载 |

---

## 四、Tab 切换异常：RouterOutlet 冻结（实战踩坑！）

> **Vue render function 崩溃会阻塞 Ionic RouterOutlet 的组件切换流程。**

**症状**：URL 变了、TabBar 高亮切了，但内容区"冻结"在离开前的视图；切回原 tab 又正常。无白屏、无卡顿——根本没切换。

**根因链路**：
```
Files.vue 的 <IonMenu> 内 v-for 访问 plugin.supportedExtensions.length
  → API 返回 null → 💥 Cannot read properties of null
  → Vue render 异常（未捕获）
  → Ionic RouterOutlet 切换事务中断
  → 目标组件的 onMounted 永远不触发
```

**触发崩溃的三类代码**：

| 模式 | 修复 |
|------|------|
| 模板直接访问可能为 null 的属性 | 可选链 + 默认值：`data?.field?.subField ?? 0` |
| 脚本无防御访问 API 返回值 | 前置 null 检查 |
| 字符串字面量引用未导入的图标 | 导入图标变量并使用引用 |

> 完整饱和调试攻击（6 Layer 错误日志 + 时序分析）→ 详见 [详情文档 §六](../rule-library/capacitor.md#六tab-切换异常routeroutlet-冻结--实战踩坑)

---

## 五、ion-toggle 暗黑模式适配（实战踩坑！）

**问题一：label 不可见**（Shadow DOM 内 + `::part(label)` 在 scoped 不可靠）→ 改用 light DOM `<ion-label>` + `<ion-toggle slot="end">`。

**问题二：ON 状态手柄暗黑模式变黑**（Ionic 8 内部规则覆盖）：

**✅ 最终方案**（App.vue 非 scoped `<style>` 块）：
```css
ion-toggle {
  --track-background: #424242;
  --track-background-checked: var(--ion-color-primary);
  --handle-background: var(--ion-color-primary);
}
ion-toggle.toggle-checked::part(handle) {
  background: #ffffff;  /* 覆盖 .ion-color 上下文 */
}
```

**Ionic v5 → v8 变量名**：`--background` → `--track-background`、`--background-checked` → `--track-background-checked`。

> 完整 8.1-8.7 章节（根因链 + 必导入组件 + ExtraField 类型分支渲染）→ 详见 [详情文档 §八](../rule-library/capacitor.md#八ion-toggle-暗黑模式完整适配-实战踩坑)

---

## 六、useProxiedFetch SSE Header 铁律

> **native 模式所有走 SSE 的 fetch 必须显式声明 `Accept: text/event-stream`，否则 useProxiedFetch 走 fetchOnce() 一次性读完所有 chunk。**

**isStream 判断**（[useProxiedFetch.ts#L166-181](file:///workspace/app/encv-mobile/src/composables/useProxiedFetch.ts)）：
```typescript
const isStream = init.isStream === true
  || (headers['Accept']?.includes('text/event-stream') ?? false)
  || (headers['accept']?.includes('text/event-stream') ?? false)
```

**修复模式**：所有 `/api/chat` / `/api/confirm` / `/api/resume` 端点的 fetch headers 加 `'Accept': 'text/event-stream'`。

**dev 模式 vs native**：
- **dev（vite）**：原生 fetch 走 WebView SSE 拆分 → 流式正常
- **Android 真机**：useProxiedFetch → `streamStart` 或 `fetchOnce` → isStream 缺失走 fetchOnce 一次性读完

> 已修复端点清单 + 新增 SSE 端点 checklist + logcat 调试技巧 → 详见 [详情文档 §九](../rule-library/capacitor.md#九useproxiedfetch-流式-header-铁律-android-真机实战踩坑)

---

## 七、主题色系统（useTheme.ts）

**核心原则**：通过 JS 动态设置 `--ion-color-primary-*` CSS 变量实现运行时切换 primary 色。

**预设色板**（7 个）：`#4f8cff / #8b5cf6 / #22c55e / #f97316 / #ef4444 / #ec4899 / #14b8a6`

```typescript
function applyColor(color: string) {
  const root = document.documentElement
  root.style.setProperty('--ion-color-primary', color)
  root.style.setProperty('--ion-color-primary-contrast', getContrastColor(color))
  root.style.setProperty('--ion-color-primary-shade', darker(color, 10))
  root.style.setProperty('--ion-color-primary-tint', lighter(color, 10))
}
```

**localStorage 键**：`encv-theme-color`、`encv-theme-preference`、`encv-locale`

> 完整 10.x 章节（架构概览 + Settings.vue UI + 注意事项）→ 详见 [详情文档 §十/§十一](../rule-library/capacitor.md#十一主题色系统)

## 八、三级页面 classList 错误（实战踩坑！2026-07-03）

> **Vue SFC 未显式 import Ionic 组件 → `<ion-page>` 渲染成原生 `<ION-PAGE>` 自闭合元素，缺失 `.ion-page` class 和 z-index，页面被前一个页面覆盖。**
>
> **症状**：URL 变了，但页面内容"冻结"在离开前的视图（被前页 z-index:101 覆盖），无白屏无报错。Cypress e2e DOM log 可见 `lastChild.tagName === 'ION-PAGE'`（大写 = 未编译）。

### 8.1 根因链路

```
FullTextIndexDetail.vue <script setup> 没 import Ionic 组件
  → Vue 编译器不识别 <ion-page> 为 Ionic Vue 组件
  → 渲染成原生自定义元素 <ION-PAGE>（大写，自闭合）
  → 缺失 .ion-page class（Ionic RouterOutlet 依赖此 class 加 z-index）
  → 缺失 z-index style → 被前一个 CacheDetail（z-index:101）覆盖
  → 用户看到"页面进不去/被前页覆盖"
```

**对比验证**：`ServerDetail.vue` / `DatabaseDetail.vue` / `CacheDetail.vue` 都显式 import Ionic 组件，正常渲染。只有 `FullTextIndexDetail.vue` 漏了 import。

### 8.2 修复模式（必须显式 import）

```typescript
// ❌ 错误（修复前）：完全没 import Ionic 组件
import { ref, onMounted } from 'vue'
// <template> 里用 <ion-page> 等标签，但 Vue 编译器不识别

// ✅ 正确（修复后）：显式 import 所有用到的 Ionic 组件
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonList, IonListHeader, IonItem,
  IonLabel, IonNote, IonBadge, IonSpinner,
} from '@ionic/vue'
```

### 8.3 Cypress e2e 验证模式

```typescript
cy.get('ion-router-outlet').then(($outlet) => {
  const children = $outlet.children()
  const lastChild = children[children.length - 1]
  // 修复前是 ION-PAGE（大写 = 未编译的自定义元素）
  expect(lastChild.tagName).to.not.eq('ION-PAGE')
  // 修复后应该有 ion-page class
  expect(lastChild.className).to.include('ion-page')
  // 应该有 z-index style（被前页覆盖的根因就是没 z-index）
  const style = lastChild.getAttribute('style') || ''
  expect(style).to.include('z-index')
})
```

### 8.4 铁律

1. **SHALL** 所有三级页面（`/tabs/settings/xxx`）的 `<script setup>` 必须显式 import 所有用到的 Ionic 组件，即使 `<template>` 里只用了 `<ion-page>` 一个标签
2. **SHALL** 新建三级页面前，对照 `ServerDetail.vue` 等"金标准"文件检查 import 完整性
3. **SHALL NOT** 依赖 Vue 编译器自动识别 `<ion-xxx>` 标签（未 import 时会渲染成原生自定义元素，无样式无行为）
4. **SHALL** 三级页面 PR 必须附 Cypress e2e 验证 `tagName !== 'ION-PAGE'` + `className includes 'ion-page'` + `style includes 'z-index'`

> 完整修复链路 + Cypress e2e 测试 → [search-diagnostics-and-classlist-fix.cy.ts](file:///workspace/app/encv-mobile/cypress/e2e/search-diagnostics-and-classlist-fix.cy.ts)

## 九、调试检查清单

1. modal 用 `modalController.create()`？inline → 改 create
2. present() 前有 await？改 present 后异步
3. componentProps 用 reactive state？扁平 props → 检查数据流
4. 有无谓 router.push？去掉
5. eventBus 跨 tab？改 composable 直接调用
6. SSE 端点 fetch headers 是否带 `Accept: text/event-stream`？
7. **🆕 三级页面进不去/被前页覆盖？检查 `<script setup>` 是否显式 import Ionic 组件（`tagName === 'ION-PAGE'` = 未编译）**

> 拆分：2026-06-11
> 更新：2026-07-03（新增 §八 三级页面 classList 错误）
