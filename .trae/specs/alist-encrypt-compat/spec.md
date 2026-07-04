# 移动端插件系统适配修复 Spec（Round 5 — 架构委托彻底化）

## Why

Round 4 虽然修复了 container tab 过滤、图标、幂等、任务命名等表面问题，但审计发现 **Files.vue 仍然是插件系统的"上帝组件"**——它直接硬编码了大量本应由 Feature 模块决定的逻辑。Feature 接口太薄，Files.vue 承担了远超"文件浏览视图"职责的插件特定代码。

### 核心问题：Files.vue 中的插件逻辑泄漏清单

| # | 位置 | 泄漏内容 | 应由谁决定 |
|---|------|---------|-----------|
| A1 | [L250, L318](file:///workspace/app/encv-mobile/src/views/Files.vue#L250) | `file.isEncrypted` badge 渲染 | Feature.getBadge() |
| A2 | [L774-780](file:///workspace/app/encv-mobile/src/views/Files.vue#L774) | `isAlistEncrypted(file)` 点击分支 | Feature.handleClick?() |
| A3 | [L782-788](file:///workspace/app/encv-mobile/src/views/Files.vue#L782) | `file.isEncrypted` 点击路由分支 | Feature.handleClick?() |
| A4 | [L883](file:///workspace/app/encv-mobile/src/views/Files.vue#L883) | `file.isEncrypted` 长按操作分支 | 已通过 getFileActions() 委托 ✅ |
| B1 | [L63](file:///workspace/app/encv-mobile/src/views/Files.vue#L63) | `plugin.containerExtension`, `plugin.supportedExtensions` 展示 | 可接受（UI 组装层职责） |
| B2 | [L1178](file:///workspace/app/encv-mobile/src/views/Files.vue#L1178) | `plugin.supportedExtensions` 文件搜索 | Feature.shouldListFile?(ext) |
| C1 | [L1165-1166](file:///workspace/app/encv-mobile/src/views/Files.vue#L1165) | `isContainerFile()` 定义在 Files.vue 内部 | FileFeature.isContainerFile?() |
| C2 | [L1169-1173](file:///workspace/app/encv-mobile/src/views/Files.vue#L1169) | `getPluginIcon()` 定义在 Files.vue 内部 | FileFeature.getIcon?() 或 PluginMeta |

### 问题本质

**FileFeature 接口只覆盖了"操作入口"(actions)、"徽章"(badge)、"字幕"(subtitle)**，但缺少以下关键能力：

1. **文件点击行为判定** — "这个文件被点击后应该怎么处理？"（流式预览？容器信息页？播放器？）
2. **文件分类判定** — "这个文件是容器/加密后的还是原始文件？"（container/origin tab 过滤）
3. **文件列表资格判定** — "这个扩展名的文件是否应该出现在该插件的文件列表中？"

Files.vue 因为接口缺失，被迫自己实现这些判断，导致插件特定逻辑泄漏到视图层。

## What Changes

### 核心原则：扩展 FileFeature 接口，将 Files.vue 中的插件判定逻辑收归 Feature 系统

- **新增 `isContainerFile?`** — 文件分类判定（container vs origin）
- **新增 `handleClick?`** — 文件点击行为（返回 ClickResult 或 null 表示不拦截）
- **Files.vue 中 `file.isEncrypted` badge 改为使用 Feature.getBadge()**
- **Files.vue handleFileClick 改为优先查询 Feature.handleClick()**

## Impact

- Affected code:
  - `src/types/file-feature.ts` — FileFeature 接口扩展
  - `src/features/alist-encrypt/index.ts` — 实现新接口方法
  - `src/composables/useFileFeatures.ts` — 新增聚合查询函数
  - `src/views/Files.vue` — 删除内联的插件判定逻辑，改为调用 Feature 系统
  - `__tests__/` — 测试更新

---

## ADDED Requirements (Round 5)

### REQ-21: FileFeature 接口扩展（P0）

#### 当前接口（[file-feature.ts L23-L31](file:///workspace/app/encv-mobile/src/types/file-feature.ts#L23-L31)）

```typescript
export interface FileFeature {
  id: string
  isActive(file: FileItem): boolean
  getBadge?(file: FileItem): FileBadge | null | Promise<FileBadge | null>
  getSubtitle?(file: FileItem): FileSubtitle | null | Promise<FileSubtitle | null>
  getFileActions?(file: FileItem): FileAction[] | Promise<FileAction[]>
  onActivate?(): void
  onDeactivate?(): void
}
```

#### 新增接口方法

```typescript
export interface ClickResult {
  handled: boolean        // true = Feature 已处理此点击，Files.vue 不再执行默认逻辑
  action?: 'preview' | 'player' | 'custom'
  route?: string         // 如 action='preview' 或 'custom'，目标路由路径
  query?: Record<string, string>  // 路由 query 参数
  streamUrl?: string     // 如 action='player'，播放 URL
}

export interface FileFeature {
  // ... 现有方法 ...

  /** 该文件是否为此插件的"容器/加密后"文件 */
  isContainerFile?(file: FileItem): boolean

  /**
   * 处理文件点击事件。
   * 返回 null/undefined 表示此 Feature 不处理该文件（交给下一个 Feature 或默认逻辑）。
   * 返回 ClickResult { handled: true } 表示已拦截。
   */
  handleClick?(file: FileItem): ClickResult | null | Promise<ClickResult | null>

  /** 插件在插件列表中显示的图标（可选，覆盖默认图标） */
  icon?: any
}
```

### REQ-22: alist-encrypt 实现新接口（P0）

[alist-encrypt/index.ts](file:///workspace/app/encv-mobile/src/features/alist-encrypt/index.ts) 扩展实现：

```typescript
export function createAlistEncryptFeature(): FileFeature {
  return {
    id: 'alist-encrypt',
    isActive: (file) => !file.isDirectory,
    // ... 现有方法 ...

    // 新增：
    isContainerFile: (file) => isAlistEncrypted(file),

    handleClick: (file): ClickResult | null => {
      if (!isAlistEncrypted(file)) return null  // 不处理非加密文件
      return {
        handled: true,
        action: 'player',
        route: '/player',
        query: { path: file.path, name: file.name },
      }
      },

    icon: lockClosed,
  }
}
```

> 注：`streamUrl` 不在 handleClick 中返回（需要密码），而是在 handleClick 中返回一个标记让 Files.vue 知道需要先弹密码框。或者更简单地：handleClick 返回 `{ handled: true, action: 'custom' }`，Files.vue 检测到 custom 后调用已有的 `isAlistEncrypted` 分支。

**简化方案**：handleClick 只返回 `{ handled: true }`，Files.vue 检测到后走现有的 isAlistEncrypted 分支（promptPassword → player）。这样不改变现有密码流程。

### REQ-23: useFileFeatures 新增聚合函数（P0）

[useFileFeatures.ts](file:///workspace/app/encv-mobile/src/composables/useFileFeatures.ts) 新增：

```typescript
export function findClickHandler(file: FileItem): ClickResult | null {
  for (const feature of registry.values()) {
    if (feature.isActive(file) && feature.handleClick) {
      const result = feature.handleClick(file)
      if (result?.handled) return result
    }
  }
  return null
}

export function isAnyContainerFile(file: FileItem): boolean {
  for (const feature of registry.values()) {
    if (feature.isActive(file) && feature.isContainerFile?.(file)) return true
  }
  return false
}

export function getFeatureIcon(featureId: string): any | undefined {
  const feature = registry.get(featureId)
  return feature?.icon
}
```

### REQ-24: Files.vue 删除内联插件逻辑（P0）

#### 24.1: handleFileClick 委托给 Feature.handleClick

```typescript
async function handleFileClick(file: FileItem) {
  // ① 优先询问 Feature 系统是否有自定义点击处理
  const clickResult = findClickHandler(file)
  if (clickResult?.handled) {
    // Feature 已处理 —— 走 Feature 指定的路径
    if (clickResult.action === 'player') {
      // alist-encrypt 流式解密预览（密码框 + 播放器）
      const password = await promptPassword(file.name)
      if (!password) return
      const streamUrl = getStreamUrl(file, password)
      router.push({ path: '/player', query: { path: file.path, name: file.name, streamUrl } })
      return
    }
    if (clickResult.route) {
      router.push({ path: clickResult.route, query: clickResult.query ?? {} })
      return
    }
    return
  }

  // ② Feature 未处理 —— 走默认逻辑（原有 isEncrypted + getFileCategory 分支保持不变）
  if (file.isEncrypted) { /* ... */ }
  const category = getFileCategory(file.name)
  /* ... */
}
```

#### 24.2: 文件项 badge 使用 Feature.getBadge()

当前 L250/L318 直接检查 `file.isEncrypted` 来渲染 badge。改为通过 Feature 系统获取 badge：

```html
<!-- 当前 -->
<ion-badge v-if="file.isEncrypted" color="warning" slot="end">Enc</ion-badge>

<!-- 改为 -->
<ion-badge v-if="getFileBadge(file)" :color="getFileBadge(file).color" slot="end">{{ getFileBadge(file).text }}</ion-badge>
```

其中 `getFileBadge` 从 `useFileFeatures` 导入（已存在的 `collectBadges` 返回单个 badge 或 null）。

#### 24.3: isContainerFile 改为使用 isAnyContainerFile

删除 Files.vue 中的 `isContainerFile` 函数定义（L1165-1166），改为从 `useFileFeatures` 导入 `isAnyContainerFile`。

#### 24.4: getPluginIcon 改为使用 getFeatureIcon

```typescript
// 当前
function getPluginIcon(name: string): string { ... }

// 改为
function getPluginIcon(plugin: PluginMeta): any {
  const featureIcon = getFeatureIcon(plugin.name)
  if (featureIcon) return featureIcon
  // fallback 映射表保留用于非 Feature 插件
  const icons: Record<string, string> = { video: filmOutline, audio: musicalNotesOutline, image: imageOutline, pdf: documentTextOutline, text: documentOutline, wps: documentOutline }
  return icons[plugin.name] || lockClosed
}
```

### REQ-25: Mock 测试覆盖

- [ ] FileFeature 接口包含 isContainerFile, handleClick, icon 方法
- [ ] createAlistEncryptFeature 实现了新方法
- [ ] findClickHandler 对 isAlistEncrypted 文件返回 handled=true
- [ ] findClickHandler 对普通文件返回 null
- [ ] isAnyContainerFile 对加密文件返回 true
- [ ] getFeatureIcon 返回 Feature 定义的 icon
