# Files.vue 优化计划 — 剩余步骤（Step 4~6）

> **前置状态**: Step 1（插件加载状态）✅ / Step 2（useThumbnailCache.ts 懒加载优化）✅ / Step 3（缓存管理集成到 CacheDetail.vue）✅
> **当前目标**: 抽取可测试逻辑 → 搭建测试基础设施 → 编写并跑通全部测试

---

## Step 4: 抽取 `useFileList.ts` 可测试逻辑

### 目标

从 [Files.vue](file:///workspace/app/encv-mobile/src/views/Files.vue) 中迁移**纯逻辑函数**到独立 composable，使它们可被单元测试直接 import 调用。

### 需要迁移的内容（Files.vue 当前内联定义）

| 行号 | 符号 | 类型 | 说明 |
|------|------|------|------|
| L405 | `imageExts` | `Set<string>` | 图片扩展名集合 |
| L407-L411 | `isImageFile(file)` | 纯函数 | 判断 FileItem 是否为图片 |
| L466-L470 | `SORT_CYCLE` | 常量数组 | 排序循环状态表 |
| L472-L477 | `cycleSort()` | 函数（依赖 ref） | 循环切换排序方式 |
| L461-L464 | `sortLabel` computed 逻辑 | 纯计算 | 排序标签文本 |
| L479-L490 | `getFileIcon(file)` | 纯函数 | 文件图标映射 |
| L492-L502 | `getFileIconColor(file)` | 纯函数 | 文件图标颜色映射 |
| L445-L459 | `sortedFiles` 核心排序 | 纯函数（提取版） | 多字段排序 + 目录置顶 |

### 新文件：`src/composables/useFileList.ts`

**导出清单**:

```typescript
// 常量
export const IMAGE_EXTENSIONS: Readonly<Set<string>>
export const SORT_CYCLE: readonly SortState[]
export type SortBy = 'name' | 'size' | 'time'
export interface SortState { by: SortBy; desc: boolean }

// 纯函数（不依赖 Vue ref/computed，可直接单测）
export function isImageFile(file: FileItem): boolean
export function getFileIcon(file: FileItem): IconName  // 返回字符串 key 或 icon 对象
export function getFileIconColor(file: FileItem): string
export function getSortLabel(sortBy: SortBy, desc: boolean): string
export function cycleSortState(current: SortState): SortState
export function sortFiles(files: FileItem[], sortBy: SortBy, desc: boolean): FileItem[]

// Composable（包装 ref 状态，供 Vue 组件使用）
export function useFileListSort() {
  // 返回 { sortBy, sortDesc, sortLabel, cycleSort, sortedFiles(files) }
}
```

### Files.vue 改动

1. 删除 L405 `imageExts`、L407-L411 `isImageFile()`、L466-L470 `SORT_CYCLE`、L472-L477 `cycleSort()`、L461-L464 `sortLabel`、L479-L490 `getFileIcon()`、L492-L502 `getFileIconColor()`
2. 新增 `import { isImageFile, getFileIcon, getFileIconColor, useFileListSort } from '@/composables/useFileList'`
3. `const { sortBy, sortDesc, sortLabel, cycleSort } = useFileListSort()`
4. `sortedFiles` computed 改为调用 `sortFiles(files.value, sortBy.value, sortDesc.value)`
5. 删除不再需要的 icon 导入：`videocam, musicalNotes, image, document as documentIcon, documentText, lockClosed, folder`（改由 useFileList 内部管理）

### 验证

```bash
cd app/encv-mobile && vue-tsc --noEmit && vite build
```

---

## Step 5: 测试基础设施 + 用例编写

### 5a. 安装依赖

```bash
cd app/encv-mobile && npm install -D vitest @vue/test-utils jsdom @vitest/coverage-v8
```

### 5b. 创建 `vitest.config.ts`

```typescript
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    globals: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
    },
  },
})
```

### 5c. package.json 追加 scripts

```json
{
  "test": "vitest",
  "test:run": "vitest run",
  "test:coverage": "vitest run --coverage"
}
```

### 5d. 测试文件清单

#### `__tests__/files.logic.test.ts` — 纯逻辑测试 (~15 cases)

从 `useFileList.ts` 和 `useThumbnailCache.ts` 导入纯函数测试：

| # | 测试项 | 输入 | 期望输出 |
|---|--------|------|----------|
| B1.1 | `isImageFile` — jpg | `{name:'a.jpg',isDirectory:false}` | true |
| B1.2 | `isImageFile` — png/gif/webp/bmp/svg/heic/heif/avif | 各扩展名 | 全部 true |
| B1.3 | `isImageFile` — 大写 JPG | `{name:'A.JPG'}` | true（大小写不敏感）|
| B1.4 | `isImageFile` — mp4 | `{name:'b.mp4'}` | false |
| B1.5 | `isImageFile` — 无扩展名 | `{name:'Makefile'}` | false |
| B1.6 | `isImageFile` — 目录 | `{name:'pics',isDirectory:true}` | false |
| B1.7 | `IMAGE_EXTENSIONS` 数量 | - | 包含 10 种扩展名 |
| B1.8 | `getFileIcon` — video | video 类别文件 | videocam 图标 |
| B1.9 | `getFileIcon` — audio | audio 类别文件 | musicalNotes 图标 |
| B1.10 | `getFileIcon` — image | image 类别文件 | image 图标 |
| B1.11 | `getFileIcon` — encrypted | 加密文件 | lockClosed 图标 |
| B1.12 | `getFileIcon` — directory | 目录 | folder 图标 |
| B1.13 | `getFileIconColor` — video/audio/image/encrypted/dir | 各类别 | danger/tertiary/success/warning/primary |
| B1.14 | `cycleSortState` — 完整循环 | 连续调用 6 次 | name↑→name↓→size↑→size↓→time↑→time↓→name↑ |
| B1.15 | `sortFiles` — name 升序 | `[B,A,C]` + name↑ | `[A,B,C]` |
| B1.16 | `sortFiles` — name 降序 | `[B,A,C]` + name↓ | `[C,B,A]` |
| B1.17 | `sortFiles` — size 升序 | size=[300,100,200] + size↑ | 按 100,200,300 |
| B1.18 | `sortFiles` — 时间降序 | modified 混合 + time↓ | 最新在前 |
| B1.19 | `sortFiles` — 目录置顶 | 混合目录和文件 | 目录始终在最前 |
| B1.20 | `getSortLabel` | 各组合 | 正确中文标签+箭头 |

#### `__tests__/composables.test.ts` — Composable 测试 (~5 cases)

| # | 测试项 | 覆盖点 |
|---|--------|--------|
| B2.1 | `clearThumbCache` + `getThumbCacheSize` | 清空后 size=0 |
| B2.2 | LRU 淘汰 | 插入 THUMB_CACHE_MAX+1 条后最早条目被淘汰 |
| B2.3 | `useThumbnailCache` 返回值 | 包含 thumbnailUrls/setupLazyThumbnails/onThumbError |
| B2.4 | `onThumbError` 清理 | 调用后 reactive ref 和 module cache 都清除 |
| B2.5 | `getSortLabel` 边界 | 未知 sortBy 返回 fallback |

#### `__tests__/api.mock.test.ts` — API Mock 测试 (~6 cases)

| # | 测试项 | Mock 方式 |
|---|--------|-----------|
| B3.1 | `listFiles` 成功 | `vi.spyOn(globalThis,'fetch')` mock → FileItem[] |
| B3.2 | `listFiles` 403 → PermissionDeniedError | mock Response(403) |
| B3.3 | `listFiles` 404 → NotFoundError | mock Response(404) |
| B3.4 | `searchFiles` 带 TTL 缓存 | 调用 2 次，第 2 次命中缓存不发请求 |
| B3.5 | `fetchPlugins` / `fetchTags` | mock 返回 PluginMeta[] / TagInfo[] |
| B3.6 | `getExternalStreamUrl` | DEV 空字符串 vs PROD 完整 URL |

---

## Step 6: 最终验证

### 6a. TypeScript + Vite 构建

```bash
cd app/encv-mobile && vue-tsc --noEmit && vite build
```

**预期**: 零错误零警告

### 6b. 运行全部测试

```bash
cd app/encv-mobile && npm run test:run
```

**预期**: 全部 ~26 个测试用例通过

### 6c. 覆盖率报告

```bash
cd app/encv-mobile && npm run test:coverage
```

**预期**: useFileList.ts 纯函数覆盖率 ≥ 90%，composable 覆盖率 ≥ 80%

---

## 文件变更总览

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| **新建** | `src/composables/useFileList.ts` | 纯逻辑抽取 |
| **新建** | `__tests__/files.logic.test.ts` | 纯逻辑 ~20 tests |
| **新建** | `__tests__/composables.test.ts` | Composable ~5 tests |
| **新建** | `__tests__/api.mock.test.ts` | API Mock ~6 tests |
| **新建** | `vitest.config.ts` | Vitest 配置 |
| **修改** | `src/views/Files.vue` | 删除内联逻辑，改为 import |
| **修改** | `package.json` | 添加 devDeps + scripts |
