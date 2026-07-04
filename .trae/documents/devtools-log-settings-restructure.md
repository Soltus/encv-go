# DevTools 日志设置重构 — 三级页面拆分 + 导出适配

> 状态:Phase 3 待批准 · Phase 4 待用户确认后执行
> 范围:`app/encv-mobile` 前端 (i18n + 路由 + 2 个 .vue + 1 个 composable export)
> 不动:Go backend / `schema.json` 的 `log.file` schema / `GoProcess` plugin interface

---

## 1. Summary

把当前 `DevToolsDetail.vue` 里的"调试工具"section(除 vConsole 外)和"日志设置"section 合并,迁到一个新的三级页面 `LogSettingsDetail.vue`(`/tabs/settings/devtools/log-settings`),原 `DevToolsDetail.vue` 上放一个入口。删除不再需要的「日志文件名」UI 和「查看日志」入口。「导出日志」改用 `config.log.level` 作为阈值过滤,而不是 DevLogs 页面里的 UI 临时筛选。

---

## 2. Current State Analysis

### 2.1 `DevToolsDetail.vue` 当前结构 (5 个 section)

| Section | 内容 | 去向 |
|---------|------|------|
| **调试工具** | vConsole toggle / 导出日志 / 查看日志 / 清空日志 | vConsole 留下,其余 3 个移到新页面 |
| **自动化测试** | 3 个 entry (AutomationTests / WebDav / SparseContainer) | 不动 |
| **沙箱预览** | dev 专属 2 个 entry | 不动 |
| **日志设置** | log.level preset cards / log.file InputWithHistory | log.level 移到新页面,log.file 删除 UI |
| **Compose 原型** | prototype cards | 不动 |

### 2.2 相关 i18n 键 (`/workspace/app/encv-mobile/src/i18n/common.ts`)

| Key | 现状 | 去向 |
|-----|------|------|
| `devtools.debugTools` | "调试工具" | 保留(只剩 vConsole) |
| `devtools.vconsole` / `vconsoleDesc` | 保留 | 保留 |
| `devtools.exportLogs` / `exportLogsDesc` / `exportSuccess` / `exportFailed` | 保留 | 移到新页面,文案不变 |
| `devtools.openLog` / `openLogDesc` / `openLogFailed` | 保留 | **删除** |
| `devtools.clearLogs` / `clearLogsDesc` / `clearSuccess` / `clearFailed` / `clearLogsConfirm` | 保留 | 移到新页面,文案不变 |
| `devtools.logFilePlaceholder` | 保留 | **删除** |
| `devtools.devtoolsDesc` | "调试工具、日志管理、Compose UI 原型" | 改文案,去掉"日志管理" |
| `settings.logSettings` | "日志设置" | 复用为新页面 title |
| 🆕 `devtools.logSettings` / `devtools.logSettingsDesc` | — | **新增** (DevTools 入口项) |

### 2.3 当前 `handleExportLogs` 逻辑 (`DevToolsDetail.vue` L314-327)

```ts
async function handleExportLogs() {
  if (!isNative()) return
  try {
    await saveDevLogs(getFrontendLogsJson())   // 前端日志全量,无 level 过滤
    const result = await exportLogs()           // Go 后端 logcat + log file,后端自带 level 过滤
    // toast success/fail
  }
}
```

**问题**:
- 前端日志(内存里)无 level 阈值过滤,信息量大、噪音多
- 用户期望"导出时使用日志级别设置" — 即用持久化的 `config.log.level` 而非 DevLogs 页面的临时 UI 筛选

### 2.4 `DevLogs.vue` 日志级别筛选 (L311)

```ts
const selectedLevels = ref<Set<string>>(new Set(['debug', 'info', 'warn', 'error']))
// 配合 backendFilter.setFilter({ levels: new Set<Level>([...selectedLevels.value] as Level[]), searchLower })
```

这是**组件本地 UI 临时状态**,非持久化,跟 `config.log.level` 是两个独立概念。`handleExportLogs` 当前**没有**引用它,不需要解耦。

### 2.5 `schema.json` `LogConfig` ($defs L249-275)

```json
{
  "level": { "type": "string", "enum": ["debug", "info", "warn", "error"], "default": "info" },
  "file":  { "type": "string", "description": "..." }
}
```

- `level`:**保留**,UI 移到新页面
- `file`:**保留 schema**,Go 后端仍在用,只是不再暴露给 mobile UI

### 2.6 `Settings.vue` L161 过滤逻辑

```ts
if (!['server', 'admin', 'webdav', 'proxy', 'log'].includes(section.key)) {
  // 显示在 Settings 主页
}
```

`log` section 已经被排除,主页不会渲染 `log.file` 输入框,行为已经正确。

---

## 3. Proposed Changes

### 3.1 🆕 新建 `/workspace/app/encv-mobile/src/views/LogSettingsDetail.vue`

新三级页面 (`/tabs/settings/devtools/log-settings`)。结构:

| 元素 | 来源 | 说明 |
|------|------|------|
| ion-back-button | `default-href="/tabs/settings/devtools"` | 返回 DevTools 主页 |
| ion-title | `t('settings.logSettings')` | 复用 i18n key |
| **Section 1: 日志级别** | 从 `DevToolsDetail.vue` L105-144 移植 | preset cards + reset-to-default 按钮 + synced badge |
| **Section 2: 导出日志** | 新实现 | button,触发 `handleExportLogs` |
| **Section 3: 清空日志** | 从 `DevToolsDetail.vue` L338-359 移植 | button,二次确认弹窗 |

**关键代码 — `handleExportLogs` 新实现:**

```ts
const LEVEL_RANK: Record<string, number> = { debug: 0, info: 1, warn: 2, error: 3 }

async function handleExportLogs() {
  if (!isNative()) return
  try {
    const configuredLevel = String(getFieldValue(['log', 'level']) ?? 'info')
    const threshold = LEVEL_RANK[configuredLevel] ?? 1
    // 过滤前端内存日志:level >= threshold 才保留
    const filteredFrontendLogs = frontendLogs.value.filter(
      (l) => (LEVEL_RANK[l.level] ?? 1) >= threshold,
    )
    await saveDevLogs(JSON.stringify(filteredFrontendLogs, null, 2))
    // 后端 logcat + log file 由 Go 后端自带 log.level 过滤(走的就是同一个 config.log.level)
    const result = await exportLogs()
    if (result.success) showToast({ message: t('devtools.exportSuccess'), duration: 1500, color: 'success' })
    else showToast({ message: t('devtools.exportFailed'), duration: 2000, color: 'danger' })
  } catch {
    showToast({ message: t('devtools.exportFailed'), duration: 2000, color: 'danger' })
  }
}
```

**为什么只过滤前端 logs,不过滤后端 logs:**
- `exportLogs()` 是 Go 后端 plugin API,负责把 logcat + Go log file 打包成 zip
- Go 后端启动时已经读取 `config.log.level` 作为 slog 阈值(后端自己过滤)
- 后端的 log file 和 logcat 输出**已经是**按 `config.log.level` 过滤后的
- 前端 logs 是内存里的 `useFrontendLogs` ref,没有阈值概念,需要手动过滤
- DevLogs 的 `selectedLevels` 不参与,跟用户要求一致

### 3.2 修改 `/workspace/app/encv-mobile/src/router/index.ts`

在 L95 之后插入新路由:

```ts
{
  // 🆕 2026-06-17：日志设置三级页面（vConsole 之外的日志相关设置 + 导出/清空）
  path: 'settings/devtools/log-settings',
  component: () => import('@/views/LogSettingsDetail.vue'),
},
```

### 3.3 修改 `/workspace/app/encv-mobile/src/views/DevToolsDetail.vue`

#### 3.3.1 删除整个「日志设置」section (L105-144)

整段 `<ion-list v-if="configLoaded">...</ion-list>` 移除。

#### 3.3.2 删除「调试工具」section 的导出/查看/清空 3 个 ion-item

L21-41 只留 vConsole toggle:

```vue
<ion-list>
  <ion-list-header>
    <ion-label>{{ t('devtools.debugTools') }}</ion-label>
  </ion-list-header>
  <ion-item>
    <ion-icon :icon="bugOutline" slot="start"></ion-icon>
    <ion-toggle :checked="vconsoleEnabled" @ionChange="handleVConsoleToggle">{{ t('devtools.vconsole') }}</ion-toggle>
  </ion-item>
</ion-list>
```

#### 3.3.3 在「调试工具」section 后插入「日志设置」入口项

```vue
<ion-item button @click="goLogSettings" detail>
  <ion-icon :icon="terminal" slot="start"></ion-icon>
  <ion-label>
    <h3>{{ t('settings.logSettings') }}</h3>
    <p>{{ t('devtools.logSettingsDesc') }}</p>
  </ion-label>
</ion-item>
```

#### 3.3.4 删除不再使用的 import / state / function

- `import { ... openLogViewer, saveDevLogs, ... } from '@/plugins/GoProcess'` — 移除 `openLogViewer` 和 `saveDevLogs`
- `import { getFrontendLogsJson } from '@/composables/useFrontendLogs'` — 移除
- `import { ... readerOutline, documentText, refreshOutline, ... } from 'ionicons/icons'` — 移除 `readerOutline`、`documentText`、`refreshOutline`、`cloudOutline` (log level card 不再在这里,这些 icon 不再使用)
- `import InputWithHistory from '@/components/InputWithHistory.vue'` — 移除
- `const logLevel` / `const logFile` / `const logLevelField` / `const logDefault` / `const isLogLevelCustomized` — 移除
- `resetLogLevelToDefault` / `handleLogLevelChange` / `handleLogFileChange` / `saveLogConfig` — 移除
- `handleExportLogs` / `handleOpenLogViewer` / `handleClearLogs` — 移除

#### 3.3.5 保留 / 新增

- 保留 `useDevTools` / `vconsoleEnabled` / `toggleVConsole` / `handleVConsoleToggle` (vConsole 还在)
- 保留 `useConfig` (虽然新页面也用,但 DevToolsDetail 主页有 vConsole 不需要 config)
- 实际上**整个 `useConfig` 在 DevToolsDetail 主页不再被引用** — 验证后删除该 import
- 新增 `function goLogSettings() { router.push('/tabs/settings/devtools/log-settings') }`

#### 3.3.6 styles 清理

L362-498 的 `.log-level-card` / `.field-label-row` / `.field-icon` / `.field-label-text` / `.required-mark` / `.sync-indicator` / `.reset-btn` / `.preset-cards` / `.preset-card*` / `@media (max-width: 599px)` 整段 — 全部删除(这些样式在 `LogSettingsDetail.vue` 里重建)

L500-509 的 `body.dark .preset-card*` — 一并删除

### 3.4 修改 `/workspace/app/encv-mobile/src/i18n/common.ts`

#### 3.4.1 新增键(中文段 L122 附近,英文段 L385 附近)

| Key | 中文 | 英文 |
|-----|------|------|
| `devtools.logSettings` | `日志设置` | `Log Settings` |
| `devtools.logSettingsDesc` | `配置日志级别、导出和清空日志` | `Configure log level, export, and clear logs` |

#### 3.4.2 修改 `devtools.devtoolsDesc`

- 中文:`'调试工具、日志管理、Compose UI 原型'` → `'调试工具、Compose UI 原型'`
- 英文:`'Debug tools, log management, Compose UI prototypes'` → `'Debug tools, Compose UI prototypes'`

#### 3.4.3 删除键(中文段 + 英文段同步)

- `devtools.openLog` (中文 + 英文)
- `devtools.openLogDesc`
- `devtools.openLogFailed`
- `devtools.logFilePlaceholder`

### 3.5 `useConfig` 在 DevToolsDetail 主页不再使用

- 删除 `import { useConfig } from '@/composables/useConfig'`
- 删除 `import { getDefaultValue } from '@/config/schemaParser'`
- 删除 `const { schemaFields, getFieldValue, setFieldValue, saveConfig, resetFieldToDefault } = useConfig()`
- 删除 `const configLoaded = computed(() => schemaFields.value.length > 0)`

### 3.6 暂时**不**删除的项 (避免越界)

| 项 | 不动理由 |
|----|---------|
| `schema.json` `log.file` 字段 | Go 后端 log file 仍依赖,删除会破坏后端 |
| `GoProcess.ts` `openLogViewer` / `saveDevLogs` plugin API 定义 | Go side 仍暴露;只移除前端 caller(`handleOpenLogViewer` 没人用了)。移除 plugin API 定义会涉及 Go 侧 + web.ts mock,超出本任务范围 |
| `web.ts` L73-74 `openLogViewer` / L205-207 / L209+ | 同上,plugin interface 契约不变 |

### 3.7 待清理 (可选,本次不做,留作后续 TODO)

`/workspace/app/encv-mobile/src/plugins/GoProcess.ts` L294-310 的 `openLogViewer` / `saveDevLogs` 顶层 wrapper — 因为本任务结束后前端不再调用,可作为后续 cleanup 任务。

---

## 4. Assumptions & Decisions

| 决策 | 理由 |
|------|------|
| 导出时**只**过滤前端 logs,不过滤后端 | 后端 logcat/log file 已由 Go 后端按 `config.log.level` 自带阈值过滤,前端再过滤会双重 |
| 新页面使用**现成的** `settings.logSettings` i18n 键作为 title | 避免新增 `logSettings.title` 冗余键 |
| vConsole 留在 DevTools 主页 | 用户原话明确"除了一个vConsole开关"不移动 |
| 删除 `log.file` UI 但**保留** schema | 后端 log file 功能不受影响,仅 UI 不暴露 |
| **不**删除 `openLogViewer` plugin API | 超出本任务范围(需 Go 侧同步),只移除 caller |
| 新页面 export 复用 `saveDevLogs` + `exportLogs` 两步 | 沿用现有 `handleExportLogs` 流程,只改 filter 逻辑 |
| LEVEL 阈值映射用 0/1/2/3 rank,>= 才保留 | 符合 slog 行业惯例;`debug=0` 最详细,`error=3` 最严格 |
| 入口项使用 `terminal` icon | 跟原 log level card 头部 icon 一致 |

---

## 5. Verification Steps

### 5.1 构建/类型

```bash
cd /workspace/app/encv-mobile && pnpm run type-check
cd /workspace/app/encv-mobile && pnpm run build
```

### 5.2 路由可达

启动 dev server (`pm2 start /workspace/ecosystem.config.cjs`) 后手动验证:

1. Settings → DevTools → 出现新的「日志设置」入口
2. 点击入口 → 跳到 `/tabs/settings/devtools/log-settings`,title 显示「日志设置」
3. 页面 back 按钮 → 回到 DevTools 主页
4. DevTools 主页:调试工具 section 只剩 vConsole toggle;日志设置 section 已消失;自动化/沙箱/Compose 不变

### 5.3 日志级别持久化

1. 在新页面把 log level 改到 `error`
2. 杀掉 app 重启
3. 重新进入新页面,确认 level 仍是 `error`(走 `config.log.level` 持久化)

### 5.4 导出日志(关键路径)

1. 设 log level = `warn`
2. 触发若干 frontend log(console.debug + console.info + console.warn + console.error)
3. 点击「导出日志」→ 弹出 "日志已导出" toast
4. 解压 zip → 打开 frontend logs JSON → 应该**没有** debug / info 级别,只有 warn / error
5. (后端)打开 logcat 文件 → 也应该只有 warn / error(Go 后端按 `config.log.level` 过滤)
6. 设 log level = `debug` → 重复 2-5 步,确认 4 个级别都包含

### 5.5 清空日志

1. 点击「清空日志」→ 弹窗确认
2. 确认 → "日志已清空" toast
3. 检查 logcat buffer 已清空

### 5.6 i18n 文案

中文:Settings → DevTools → 入口项「日志设置」+ 副标题「配置日志级别、导出和清空日志」
英文:Settings → DevTools → 入口项 "Log Settings" + 副标题 "Configure log level, export, and clear logs"
DevTools 主页副标题去掉"日志管理"。

### 5.7 已删除项确认

- 主页「查看日志」按钮已消失(整个 ion-item 移除)
- 主页「日志设置」section(log level preset cards + log file input)已消失
- 主页「日志文件名」输入框已消失
- `InputWithHistory` 在 DevToolsDetail 不再被引用

### 5.8 后端回归(防 schema 字段被误删)

```bash
cd /workspace && bash scripts/test-go.sh ./internal/config
```

schema.json `log.file` 字段必须仍然存在(只移 UI,schema 不动)。

### 5.9 沙箱预览(如开发)

```bash
# 启动 preview-gateway,验证 OpenPreview 链接 200 OK
bash /workspace/scripts/previews.sh start
bash /workspace/scripts/previews.sh status
# 触发 OpenPreview(url=http://localhost:16666/),手动点开路径核对路由
```

---

## 6. File Manifest

| 操作 | 路径 | 行数估计 |
|------|------|---------|
| 🆕 新建 | `app/encv-mobile/src/views/LogSettingsDetail.vue` | ~280 (从 DevToolsDetail 移植 log level + 新 export/clear) |
| ✏️ 修改 | `app/encv-mobile/src/router/index.ts` | +5 行 (新 route) |
| ✏️ 修改 | `app/encv-mobile/src/views/DevToolsDetail.vue` | -120 / +10 行 (瘦身) |
| ✏️ 修改 | `app/encv-mobile/src/i18n/common.ts` | +2 keys (中英各 1 行 ×2) / -4 keys ×2 / 改 1 key ×2 |

总改动:~5 个文件,净减约 100 行。

---

## 7. Out of Scope (后续 TODO)

- `GoProcess.ts` 的 `openLogViewer` / `saveDevLogs` 顶层 wrapper 清理(需 Go 侧同步)
- `web.ts` mock 中相应方法清理
- `devtools.openLog*` / `devtools.logFilePlaceholder` i18n key 实际删除(本计划已列入修改清单)
- `useConfig` 中如果 log.file 完全不再被前端读写,后端默认值一致性检查
- 自动化测试覆盖新 LogSettingsDetail 页面(本任务为 UI 重构,先手动验证)
