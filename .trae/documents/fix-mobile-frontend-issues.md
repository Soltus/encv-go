# ENCV-Mobile 前端问题修复计划

## 问题总览

5 个需要修复的问题，涉及 7 个文件：

| # | 问题 | 涉及文件 | 优先级 |
|---|------|---------|--------|
| 1 | 后端连不上，无辅助信息 | `useServerStatus.ts`, `Settings.vue`, `useWebSocket.ts`, `encv.ts` | 高 |
| 2 | 日志级别 sheet 暗黑模式 + 外观/语言统一组件 | `Settings.vue`, `variables.css` | 高 |
| 3 | GitHub 链接不正确 | `Settings.vue` | 中 |
| 4 | 配置保存/WebDAV 测试失败时日志输出详细信息 | `useConfig.ts`, `Settings.vue`, `WebDAV.vue`, `encv.ts` | 高 |
| 5 | 设置项必填/可选区分 | `Settings.vue`, `useI18n.ts` | 中 |

---

## 问题 1：后端连不上，无辅助信息

### 根因分析

1. **Settings 页面只显示 "在线/离线" badge**，没有任何诊断信息（连接地址、失败原因）
2. **WebSocket 连接失败时静默重连**，用户完全不知道发生了什么
3. **`checkServerStatus()` 只返回 boolean**，丢失了所有错误详情
4. **App.vue 和 useServerStatus 都调用 `connect()`**，存在重复初始化

### 修复方案

**encv.ts**：
- `checkServerStatus()` 改为返回 `{ online: boolean, error?: string }`，捕获具体错误信息

**useServerStatus.ts**：
- 新增 `lastError` ref 记录最近一次连接失败的错误信息
- `checkStatus()` 保存错误详情到 `lastError`

**useWebSocket.ts**：
- `ws.onerror` / `ws.onclose` 中通过 eventBus 发送 `server:connection-error` 事件

**Settings.vue**：
- 连接区域显示当前 serverUrl
- 离线时显示错误原因（如 "网络错误"、"连接被拒绝" 等）

---

## 问题 2：日志级别 sheet 暗黑模式 + 外观/语言统一组件

### 根因分析

1. **日志级别** ion-select 使用了 `mode="ios"`，但**语言** ion-select 没有 `mode="ios"` → 风格不统一
2. **暗黑模式下 action-sheet 背景是白色** → 这是 `variables.css` 中缺少 overlay 组件暗黑变量的 bug

Ionic 的 overlay 组件（action-sheet、alert、popover）的暗黑模式依赖 CSS 变量。当前 `variables.css` 的 `body.dark` 只定义了基础颜色变量（`--ion-background-color` 等），但缺少 overlay 相关变量：
- `--ion-overlay-background-color`
- action-sheet 专用变量（`--ion-action-sheet-background` 等）

### 修复方案

**统一所有 ion-select 使用 `mode="ios"` + `interface="action-sheet"`**，保持风格一致。

**修复暗黑模式**：在 `variables.css` 的 `body.dark` 中添加 overlay 相关变量：

```css
body.dark {
  /* 现有变量... */

  /* Overlay 暗黑模式 */
  --ion-overlay-background-color: #1e1e1e;

  /* Action Sheet 暗黑模式 */
  --ion-action-sheet-background: #1e1e1e;
  --ion-action-sheet-button-background: #2a2a2a;
  --ion-action-sheet-button-color: #ffffff;
  --ion-action-sheet-destructive-color: #ff4961;

  /* Alert 暗黑模式 */
  --ion-alert-background: #1e1e1e;
  --ion-alert-color: #ffffff;
}
```

---

## 问题 3：GitHub 链接不正确

### 修复方案

`Settings.vue` 第336行：`https://github.com/encv-go` → `https://github.com/Soltus/encv-go`

---

## 问题 4：配置保存/WebDAV 测试失败时日志输出详细信息

### 根因分析

当前所有错误处理都是 `catch {}` 静默吞掉错误或只显示通用 toast：

1. `useConfig.ts loadConfig()`：`catch {}` — 完全静默
2. `useConfig.ts saveConfig()`：`catch (error) { throw error }` — 不记录
3. `Settings.vue handleSaveConfig()`：只显示 "保存配置失败"
4. `WebDAV.vue testConfig()/testConnection()`：只显示 "连接失败"
5. `encv.ts testWebDAVConnection()`：`catch { return false }` — 丢失错误
6. `encv.ts updateConfig()`：只抛出 HTTP status

### 修复方案

**encv.ts**：
- `testWebDAVConnection()` 失败时抛出含响应体/状态码的 Error
- `updateConfig()` 失败时读取响应体并包含在 Error message 中
- `fetchConfig()` 失败时同上

**useConfig.ts**：
- `loadConfig()` catch 中 `console.error` 记录详细错误
- `saveConfig()` catch 中 `console.error` 记录详细错误再 throw

**Settings.vue**：
- `handleSaveConfig()` 的 catch 中显示具体错误信息

**WebDAV.vue**：
- `testConfig()/testConnection()` 的 catch 中显示具体错误信息

---

## 问题 5：设置项必填/可选区分

### 根因分析

`schemaParser.ts` 已解析 `required` 字段到 `FieldDef.required`，但 `Settings.vue` 渲染时未使用。

### 修复方案

**Settings.vue**：
- ion-input label 后添加红色 `*` 标记（当 `field.required` 为 true）
- ion-toggle 不需要标记（boolean 无所谓必填）

**useI18n.ts**：
- 不需要额外 i18n key，`*` 是通用标记

---

## 实施步骤

### Step 1: 修复 GitHub 链接
- 文件：`Settings.vue`
- 改动：URL 修正

### Step 2: 统一 select 组件 + 修复暗黑模式
- 文件：`Settings.vue`（统一 `mode="ios"` + `interface="action-sheet"`）
- 文件：`variables.css`（添加 overlay 暗黑变量）

### Step 3: 设置项必填/可选区分
- 文件：`Settings.vue`

### Step 4: API 层错误信息增强
- 文件：`encv.ts`（详细错误信息）
- 文件：`useConfig.ts`（console.error 记录）
- 文件：`Settings.vue`（显示具体错误）
- 文件：`WebDAV.vue`（显示具体错误）

### Step 5: 后端连接诊断信息增强
- 文件：`encv.ts`（checkServerStatus 返回错误详情）
- 文件：`useServerStatus.ts`（lastError）
- 文件：`useWebSocket.ts`（eventBus 发送错误）
- 文件：`Settings.vue`（显示连接地址和错误原因）
