# 抽屉增强 + 图像懒加载预览 — 实施计划

## 状态概览

以下 3 项需求的代码已全部写入 `Files.vue`，但存在 **1 个 TypeScript 编译错误** 阻塞构建：

| # | 需求 | 代码状态 | 构建状态 |
|---|------|---------|---------|
| 1 | 抽屉"所有文件"入口 | ✅ 已写入 L55-58 | ⚠️ 被 TS 错误阻塞 |
| 2 | 切换入口后立即清空旧列表 | ✅ 已写入 L955-1000 | ⚠️ 被 TS 错误阻塞 |
| 3 | 图像懒加载预览 | ✅ 已写入 L388-396, L884-901 | ❌ TS2339 @ L886 |

## 唯一阻塞：TS2339 类型错误

**文件**: `src/views/Files.vue` **L886**

```typescript
// 当前代码（报错）
const targets = document.querySelectorAll<Element>('.lazy-thumb-target')
// error TS2339: Property 'querySelectorAll' does not exist on type 'string'
```

**原因**: Vue SFC `<script setup lang="ts>` 中 `document` 的类型可能被覆盖或泛型参数未正确推导。

**修复方案**: 使用 `as` 类型断言替代泛型参数：

```typescript
const targets = document.querySelectorAll('.lazy-thumb-target') as NodeListOf<Element>
```

## 实施步骤

### Step 1: 修复 L886 TS 类型错误

- **文件**: `/workspace/app/encv-mobile/src/views/Files.vue`
- **行号**: L886
- **操作**: 将 `document.querySelectorAll<Element>('.lazy-thumb-target')` 改为 `document.querySelectorAll('.lazy-thumb-target') as NodeListOf<Element>`
- **同时**: 确保 `forEach` 回调中的 `(el: Element)` 类型标注保留（已正确）

### Step 2: 验证构建

```bash
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit && npx vite build
```

期望：零错误零警告。

### Step 3（可选优化）: 插件模式下的 loading 状态隔离

当前模板结构中，`v-if="loading"` (L87) 的 spinner 和 `v-if="selectedPlugin"` (L118) 的插件列表是**同级 div**，切换模式时两者会同时显示片刻（spinner + 空插件列表）。

可选改进：在插件模式 div 内部增加独立的 loading 判断，或在非插件模式下才显示全局 spinner。优先级低，可后续迭代。
