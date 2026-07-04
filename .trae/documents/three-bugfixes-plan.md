# 三个测试问题修复计划

## 问题 1：加密解密覆盖二次确认逻辑错误

### 现状
[Files.vue](file:///workspace/app/encv-mobile/src/views/Files.vue) L564-583 和 L624-643 中，`handleEncryptFile` 和 `handleDecryptFile` 的二次确认逻辑：
```ts
const isDefaultPath = targetPath === parentDir
if (isDefaultPath) {
  // 弹出覆盖确认
}
```
当 `targetPath === parentDir`（即目标路径与源文件所在目录相同）时就弹出确认，这是**伪逻辑**——同目录并不意味着产物会覆盖。

### 根因
加密产物命名规则：源文件 `foo.mp4` → 产物 `foo.mp4.encv`，解密产物：`foo.mp4.encv` → `foo.mp4`。
- **加密**：产物是 `.encv` 后缀，与源文件不同名，不会覆盖源文件。但如果目标目录下已存在同名 `.encv` 文件，才会覆盖。
- **解密**：产物是去掉 `.encv` 后缀的文件，如果目标目录下已存在同名原始文件，才会覆盖。

### 修复方案
1. 后端新增 `GET /api/files/exists?path=xxx` API，检查指定路径的文件/目录是否存在
2. 前端 `encv.ts` 新增 `checkFileExists(path)` 函数
3. `handleEncryptFile` 中：根据 `targetPath + '/' + file.name + '.encv'` 构造产物路径，调用 `checkFileExists` 检查，只有存在时才弹出确认
4. `handleDecryptFile` 中：根据 `targetPath + '/' + file.name.replace(/\.encv$/, '')` 构造产物路径，调用 `checkFileExists` 检查，只有存在时才弹出确认
5. 确认弹窗文案调整：明确提示"目标文件已存在，加密/解密将覆盖"

### 涉及文件
- `internal/server/mobile_api.go` — 新增 `handleFileExistsAPI`
- `internal/server/server.go` — 注册路由
- `app/encv-mobile/src/api/encv.ts` — 新增 `checkFileExists`
- `app/encv-mobile/src/views/Files.vue` — 修改确认逻辑

---

## 问题 2：播放器入口设计错误（底部 tab → 首页卡片）

### 现状
[Tabs.vue](file:///workspace/app/encv-mobile/src/views/Tabs.vue) L11-14 添加了一个底部 player tab-button，点击调用 `openPlayerHome()` 打开独立的 Lynx PlayerActivity。

用户要求的是：**主应用添加首页**，首页上有**播放器卡片入口**，而不是底部 tab。

### 修复方案
1. **创建 HomePage.vue**：主应用的新首页，包含：
   - 欢迎标题/应用名
   - 播放器卡片入口（大卡片，带图标+文字，点击调用 `openPlayerHome()` 或 `openInPlayer`）
   - 最近文件/快捷操作区域（可复用现有文件列表逻辑）
2. **修改路由**：`/` 重定向从 `/tabs/files` 改为 `/tabs/home`
3. **修改 Tabs.vue**：
   - 底部 tab 栏：移除 player tab-button
   - 添加 home tab-button（首页图标），href 指向 `/tabs/home`
   - tab 顺序调整为：首页、文件、任务、远端、设置、DevLogs
4. **添加路由子项**：`/tabs/home` → `HomePage.vue`

### 涉及文件
- `app/encv-mobile/src/views/Tabs.vue` — 移除 player tab，添加 home tab
- `app/encv-mobile/src/views/HomePage.vue` — 新建首页组件
- `app/encv-mobile/src/router/index.ts` — 添加 home 路由，修改默认重定向

---

## 问题 3：Lynx 播放器播放失败 + 无 UI 显示 + eventReporter 日志

### 现状
1. **播放失败**：PlayerView.vue 中 `startPlayback` 通过 `globalThis.NativeModules.MpvPlayerModule.play()` 调用原生模块，但可能 initData 未正确传递
2. **无 UI 显示**：PlayerControls.vue 有错误状态渲染（`isError` computed），但 `playerState` 可能停留在 `idle`/`loading`，事件监听可能不工作
3. **eventReporter 日志**：`eventReporter service not found or event name is null` 是 Lynx 引擎内部日志，与 `sendGlobalEvent` 机制相关

### 根因分析

#### 3a. 全局事件监听方式错误
当前代码使用 `globalThis.addEventListener('mpv:state-change', ...)`，但 Lynx 的全局事件机制是 `GlobalEventEmitter`：
- Kotlin 端通过 `lynxContext?.sendGlobalEvent(eventName, params)` 发送事件
- JS 端应通过 `lynx.getJSModule('GlobalEventEmitter').addListener(eventName, callback)` 监听
- `globalThis.addEventListener` 是浏览器 API，在 Lynx 运行时中不存在

#### 3b. initData 未被读取
Kotlin 端通过 `renderTemplateUrl("player.lynx.bundle", initData)` 传入 initData，但 vue-lynx 中没有读取 `lynx.__globalProps` 或 initData 的代码。PlayerView.vue 从 `route.query` 读取文件信息，但 Lynx 页面没有通过 URL query 传参的机制。

#### 3c. eventReporter 日志
`eventReporter service not found or event name is null` 是 Lynx 引擎在找不到事件上报服务时的内部日志。这不是致命错误，但大量重复说明有事件在持续触发。需要在 `lynx.config.ts` 中配置 `enableEventReporter: false` 或在 `LynxViewBuilder` 中禁用。

### 修复方案

#### 3a. 修复全局事件监听
PlayerView.vue 中替换 `globalThis.addEventListener` 为 `GlobalEventEmitter`：
```ts
const lynx = (globalThis as any).lynx
const emitter = lynx?.getJSModule?.('GlobalEventEmitter')

onMounted(() => {
  emitter?.addListener('mpv:state-change', onMpvStateChange)
  emitter?.addListener('mpv:position-update', onMpvPositionUpdate)
})

onUnmounted(() => {
  emitter?.removeListener('mpv:state-change', onMpvStateChange)
  emitter?.removeListener('mpv:position-update', onMpvPositionUpdate)
})
```

注意：`sendGlobalEvent` 传递的参数是数组形式（`JavaOnlyArray`），回调参数是展开的数组元素，所以 `onMpvStateChange` 的参数签名应为 `(event: any)` 而不是 `(event: CustomEvent)`。

#### 3b. 修复 initData 读取
Lynx 的 initData 通过 `lynx.__globalProps` 访问。修改 PlayerView.vue：
```ts
const initData = computed(() => {
  try {
    const lynx = (globalThis as any).lynx
    return lynx?.__globalProps || {}
  } catch {
    return {}
  }
})
const filePath = computed(() => initData.value.filePath || '')
const fileName = computed(() => initData.value.fileName || 'Unknown')
const mimeType = computed(() => initData.value.mimeType || '')
const isExternal = computed(() => !!initData.value.isExternal)
const mediaType = ref<'video' | 'audio'>(
  initData.value.mediaType === 'audio' ? 'audio' : 'video'
)
```
移除 `vue-router` 的 `useRoute()` 依赖（Lynx 页面内不使用 URL query 传参）。

#### 3c. 抑制 eventReporter 日志
在 `lynx.config.ts` 中添加配置禁用事件上报：
```ts
export default defineConfig({
  plugins: [pluginVueLynx()],
  source: { entry: './src/main.ts' },
  output: { ... },
  // 禁用 Lynx 事件上报服务，避免 eventReporter 日志
  lynx: {
    enableEventReporter: false,
  },
})
```
如果 rspeedy 不支持此配置项，则在 Kotlin 端 `LynxViewBuilder` 中设置。

#### 3d. 增强错误 UI
PlayerControls.vue 的 `isError` 条件是 `props.error && props.state !== 'loading'`，但 `playerState` 为 `idle` 时 `isLoading` 为 true（L69: `isLoading = state === 'loading' || state === 'idle'`），所以 idle 状态下即使有 error 也不会显示错误。

修复：将 `isLoading` 条件改为仅 `state === 'loading'`，idle 状态下如果有 error 应显示错误界面。

### 涉及文件
- `app/encv-mobile/lynx-player/src/views/PlayerView.vue` — 修复事件监听 + initData 读取
- `app/encv-mobile/lynx-player/src/components/PlayerControls.vue` — 修复错误 UI 显示条件
- `app/encv-mobile/lynx-player/lynx.config.ts` — 禁用 eventReporter（如果支持）
- `app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt` — 禁用 eventReporter（如果 rspeedy 不支持）

---

## 执行顺序

1. **问题 1**：加密解密覆盖确认逻辑修复（后端 API + 前端逻辑）
2. **问题 2**：首页 + 播放器卡片入口（新建 HomePage + 路由 + Tabs 修改）
3. **问题 3a**：Lynx 全局事件监听修复（GlobalEventEmitter 替换 addEventListener）
4. **问题 3b**：Lynx initData 读取修复（__globalProps 替换 route.query）
5. **问题 3c**：eventReporter 日志抑制
6. **问题 3d**：错误 UI 显示修复
7. **验证**：`npm run build` 构建通过
