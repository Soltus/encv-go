# 修复计划：信息/流式播放/解密三大功能适配 alist_encrypt 插件（v2）

> **版本**: v2 — 结构化重构，含完整影响评估
> **范围**: Player 调用契约统一 + FileInfo 扩展点设计 + 密码传递链路

---

## 一、现状审计（影响范围基线）

### 1.1 /player 路由调用方清单（全部 8 处）

| # | 调用位置 | 发送的 query 参数 | 是否正常 |
|---|---------|------------------|---------|
| A1 | [Files.vue:542](app/encv-mobile/src/views/Files.vue#L542) | `{ path, name }` | ✅ 标准模式 |
| A2 | [Files.vue:549](app/encv-mobile/src/views/Files.vue#L549) | `openPlayer(path, name, mimeType)` 原生 | ✅ 原生模式 |
| A3 | [Files.vue:557](app/encv-mobile/src/views/Files.vue#L557) | `{ path, name }` | ✅ 标准模式 |
| A4 | [Files.vue:565](app/encv-mobile/src/views/Files.vue#L565) | `{ path, name }` | ✅ 标准模式 |
| A5 | [Files.vue:570](app/encv-mobile/src/views/Files.vue#L570) | `{ path, name }` | ✅ 标准模式 |
| A6 | [Files.vue:800](app/encv-mobile/src/views/Files.vue#L800) | `{ path, name, streamUrl }` | ⚠️ streamUrl 被 Player 忽略 |
| A7 | [Files.vue:816](app/encv-mobile/src/views/Files.vue#L816) | `{ path, name, streamUrl }` | ⚠️ streamUrl 被 Player 忽略 |
| A8 | [FilePreview.vue:324](app/encv-mobile/src/views/FilePreview.vue#L324) | `{ path, name }` | ✅ ENCV 容器视频 |
| A9 | [actions.ts:27](app/encv-mobile/src/features/alist-encrypt/actions.ts#L27) | `{ streamUrl, name }` | ❌ **缺 path 参数** |
| A10 | [HomePage.vue:77](app/encv-mobile/src/views/HomePage.vue#L77) | 无参数 | ✅ 首页入口 |

### 1.2 ArtPlayerView 接收端现状

[ArtPlayerView.vue:332-333](app/encv-mobile/src/views/ArtPlayerView.vue#L332-L333):
```typescript
filePath.value = (route.query.path as string) || ''   // 只读 path
fileName.value = (route.query.name as string) || ''     // 只读 name
```

[ArtPlayerView.vue:95-98](app/encv-mobile/src/views/ArtPlayerView.vue#L95-L98):
```typescript
const streamUrl = computed(() => {
  if (!filePath.value) return ''
  return getFileStreamUrl(filePath.value)     // ← 完全忽略 route.query.streamUrl
})
```

**关键缺陷**：
- `streamUrl` query 参数被发送但从未被读取（A6/A7/A9 三处）
- `getFileStreamUrl()` 只生成标准 `/api/files/stream?path=...` URL
- 无法生成 alist-encrypt 专用流 URL (`/api/alist-encrypt/stream?...&password=...`)

### 1.3 FileInfo 展示层现状

[FileInfo.vue:58-61](app/encv-mobile/src/views/FileInfo.vue#L58-L61) 加密标识：
```html
<div class="info-row" v-if="info.is_encrypted">
  <ion-badge color="warning">Yes</ion-badge>
</div>
```
→ 仅依赖后端 `is_encrypted` 字段 → 对 .bin 文件始终 false

[FileInfo.vue:62-66](app/encv-mobile/src/views/FileInfo.vue#L62-L66) 解码名称：
```html
<div v-if="decodedName" class="info-row decoded-name">
  <span>{{ t('fileInfo.originalName') }}: {{ decodedName }}</span>
</div>
```
→ 依赖 `isAlistEncrypted(fileItem)` + `loadDecodedName()` → 有条件工作

**缺失**：无插件扩展点，无法让 feature plugin 注入自定义信息行。

### 1.4 Feature Plugin 架构

当前仅注册了 **1 个** feature plugin：`alist-encrypt`。

接口定义 ([file-feature.ts](app/encv-mobile/src/types/file-feature.ts))：
```
FileFeature {
  id, isActive(file), getBadge?(file), getSubtitle?(file),
  getFileActions?(file), isContainerFile?(file),
  handleClick?(file) → ClickResult { handled, action?, route?, query? },
  onActivate?, onDeactivate?, icon?
}
```

注意 `ClickResult` 已有 `route` + `query` 字段——这是 feature 导航到 Player 的**正规通道**。
但 `actions.ts` 的 handler 直接用 `router.push()` 绕过了这个机制。

---

## 二、修复方案（按优先级排序）

### 修复 P0：统一 Player 调用契约（影响 3 处调用方）

#### 问题本质

Player 需要**两种流源**：
1. **标准流**：`/api/files/stream?path=...` — 通过 `getFileStreamUrl(path)` 生成
2. **插件自定义流**：`/api/alist-encode/stream?...&password=...` — 通过插件专用函数生成

当前 `streamUrl` computed 只支持 #1。需要扩展为支持 #2。

#### 修改点 2a：ArtPlayerView.vue — 支持 `streamUrl` query 参数

**文件**: `app/encv-mobile/src/views/ArtPlayerView.vue`

**改动**：

```typescript
// 新增 ref
const overrideStreamUrl = ref<string>('')

onMounted(() => {
  filePath.value = (route.query.path as string) || ''
  fileName.value = (route.query.name as string) || ''

  // 新增：读取外部传入的 streamUrl（优先于自动生成）
  const qsu = route.query.streamUrl as string
  if (qsu) overrideStreamUrl.value = qsu

  // ...
})

// 修改 streamUrl computed：优先使用 override
const streamUrl = computed(() => {
  // 如果有外部传入的 streamURL，直接使用
  if (overrideStreamUrl.value) return overrideStreamUrl.value
  // 否则按原有逻辑从 filePath 自动生成
  if (!filePath.value) return ''
  return getFileStreamUrl(filePath.value)
})
```

**影响分析**：

| 调用方 | 变更前行为 | 变更后行为 | 兼容性 |
|--------|-----------|-----------|--------|
| A1-A5（标准 `{path,name}`） | `streamUrl=''` → `getFileStreamUrl(path)` ✅ | 不变（override 为空） | ✅ 无影响 |
| A6/A7（`{path,name,streamUrl}`） | `streamUrl` 被忽略 ❌ | 使用传入的 `streamUrl` ✅ | ✅ 修复 |
| A9/actions.ts（`{streamUrl,name}` 缺 path） | `filePath=''` → 空字符串 ❌ | `streamUrl` 生效但 `filePath` 缺失需额外处理 | ⚠️ 见下 |
| A10/HomePage（无参数） | 首页空状态 | 不变 | ✅ 无影响 |

#### 修改点 2b：actions.ts — 补齐 path 参数

**文件**: `app/encv-mobile/src/features/alist-encrypt/actions.ts`

**改动**：

```typescript
// 修改前（错误 - 缺少 path）
router.push({ path: '/player', query: { streamUrl: url, name: decodedName } })

// 修改后（符合标准契约）
router.push({
  path: '/player',
  query: {
    path: f.path,           // ← 补齐：Player 用此做文件信息展示
    name: decodedName,      // ← 解码后的文件名（标题栏显示）
    streamUrl: url,         // ← alist-encrypt 专用流地址
  }
})
```

**影响分析**：
- 仅影响 `alist-encrypt` feature 的「流式预览」操作
- 与 Files.vue 第 816 行的单击播放模式完全对齐
- 不影响任何其他功能或插件

#### 修改点 2c（可选加固）：Files.vue A6/A7 也受益

Files.vue 第 800 和 816 行已发送 `streamUrl` 但之前被忽略。修复 2a 后这两处**自动修复**，无需单独改代码。

---

### 修复 P1：FileInfo 增加 Alist-Encrypt 信息展示

#### 方案选择

**方案 A（推荐）：在 FileInfo.vue 中直接检测**

优点：简单直接，无架构变更
缺点：如果未来新增加密类型插件，需要再次改 FileInfo

**方案 B：通过 FileFeature 接口增加 `getInfoSections` 扩展点**

优点：符合开放-封闭原则，新插件无需改 FileInfo
缺点：需要改接口定义 + FileInfo 模板 + 注册逻辑，工作量较大

**本计划采用方案 A**，同时在接口定义中预留 `getInfoSections` 占位注释供未来扩展。

#### 修改点：FileInfo.vue

**文件**: `app/encv-mobile/src/views/FileInfo.vue`

**改动位置 1** — 新增 `isAlistEnc` ref 和检测逻辑（在 `fetchFileInfo` 回调中）：

```typescript
import { isAlistEncrypted, loadDecodedName, getDecodedName } from '@/features/alist-encrypt/useAlistEncrypt'

const isAlistEnc = ref(false)

async function fetchFileInfo() {
  // ...existing fetch logic...
  
  // 在 data 赋值后追加检测
  const fileItem: FileItem = {
    name: data.name,
    path: data.path,
    size: data.size,
    isDirectory: false,
    modifiedTime: data.modified_time,
    mimeType: data.mime_type,
  }
  
  isAlistEnc.value = isAlistEncrypted(fileItem)
  if (isAlistEnc.value) {
    await loadDecodedName(fileItem)
    decodedName.value = getDecodedName(data.path) || null
  }
}
```

**改动位置 2** — Template 中增加加密标识行（在基本信息卡片内，MIME 行之后）：

```html
<!-- Alist-Encrypt 加密标识 -->
<ion-card v-if="isAlistEnc" class="info-card">
  <ion-card-header>
    <ion-card-title>{{ t('files.alistEncrypt') || 'Alist-Encrypt' }}</ion-card-title>
  </ion-card-header>
  <ion-card-content>
    <div class="info-row">
      <span class="info-label">{{ t('files.encrypted') || '加密状态' }}</span>
      <ion-badge color="danger">{{ t('files.yes') || '是' }}</ion-badge>
    </div>
    <div v-if="decodedName" class="info-row decoded-name">
      <span class="info-label">{{ t('fileInfo.originalName') }}</span>
      <span class="info-value decoded-text">{{ decodedName }}</span>
    </div>
  </ion-card-content>
</ion-card>
```

**影响分析**：

| 场景 | 变更前 | 变更后 |
|------|--------|--------|
| 普通 .mp4 文件 | 显示基本信息 | 不变（isAlistEnc=false） |
| ENCV 容器 .sccgt | 基本信息 + ENCV 详情卡 | 不变（isAlistEnc=false） |
| **Alist-Encrypt .bin** | 基本信息 + 底部弱显示解码名 | **基本信息 + 独立加密标识卡片 + 解码名** |
| 目录 | N/A | 不变（目录不进 FileInfo） |

---

### 修复 P2：解密操作密码传递优化

#### 当前流程

```
用户点击"解密"
  → actions.ts handler: openNewTask(f.path, 'decrypt')
    → NewTaskModal 打开（空白表单）
      → 用户手动输入密码（如未缓存）
        → doPredict() → 创建任务
```

#### 优化方案

利用已有的 `sessionPassword` 缓存机制（`useAlistEncrypt.ts` 中的 Map），在 NewTaskModal 初始化时检查并预填：

**修改点 2d：actions.ts 解密 handler — 先查缓存再弹框**

```typescript
handler: async (f: FileItem) => {
  let password = getSessionPassword(f.path)
  if (!password) {
    password = await promptPassword(f.name)
    if (password == null) return
    setSessionPassword(f.path, password)
  }
  const { openNewTask } = useNewTaskModal()
  openNewTask(f.path, 'decrypt')
}
```

**修改点 2e：useNewTaskModal 或 NewTaskModal — 支持初始密码注入**

在 `state` reactive 对象初始化时，从 session 缓存读取并设置 `secondaryPassword`：

```typescript
// openNewTask() 内部，state 初始化后追加：
if (taskType === 'decrypt') {
  const cachedPwd = getSessionPassword(initialSourcePath)
  if (cachedPwd) state.secondaryPassword = cachedPwd
}
```

**影响分析**：
- 仅影响解密任务的密码字段默认值
- 加密任务和其他操作不受影响
- 用户仍可在 modal 中修改/清除预填的密码

---

## 三、修改文件汇总与执行顺序

| 步骤 | 文件 | 改动量 | 依赖 | 影响的其他插件 |
|------|------|--------|------|---------------|
| **S1** | `views/ArtPlayerView.vue` | +8 行（ref + onMounted + computed 改写） | 无 | 所有 /player 调用方（向后兼容） |
| **S2** | `features/alist-encrypt/actions.ts` | +1 行（补 `path: f.path`） | 无 | 仅 alist-encrypt 本身 |
| **S3** | `views/FileInfo.vue` | +25 行（import + ref + 检测逻辑 + template 卡片） | S1 无关 | 无（独立页面） |
| **S4** | `composables/useNewTaskModal.ts` | +5 行（session password 读取） | 无 | 仅解密场景 |

**总改动量**: ~38 行净增量，0 行删除

---

## 四、验证矩阵

| 测试用例 | 预期结果 | 涉及修改 |
|---------|---------|---------|
| **T1**: 长按 .bin → 流式预览 → 输入密码 | Player 正常播放视频，标题显示 CAD放样.mp4 | S1+S2 |
| **T2**: 单击 .bin → 输入密码 | 同 T1（Files.vue:816 的 streamUrl 也生效） | S1（自动修复） |
| **T3**: 长按 .bin → 信息 | 显示红色 "Alist-Encrypt 加密" badge + 原始文件名 | S3 |
| **T4**: 长按 .bin → 解密 | 弹窗打开，密码如有缓存则预填 | S4 |
| **T5**: 单击普通 .mp4 | 正常播放（不受影响） | S1（兼容验证） |
| **T6**: ENCV 容器 .sccgt → 信息 | 原有 ENCV 详情卡正常显示 | S3（不影响） |
| **T7**: 目录长按菜单 | 不包含上述操作（不变） | 无 |
| **T8**: 首页进入 Player | 空状态（不变） | S1（兼容验证） |

---

## 五、风险与回滚

| 风险 | 概率 | 缓解措施 |
|------|------|---------|
| S1 导致旧版 Player 调用异常 | 低 | `overrideStreamUrl` 默认为空字符串，computed fallback 到原逻辑 |
| S3 的 import 循环依赖 | 低 | `useAlistEncrypt` 不导入 FileInfo，单向依赖 |
| S4 的 getSessionPassword 未导出 | 低 | 已确认在 index.ts 中 re-export |
