# Vue 组件未导入问题排查与解决方案

> 沉淀时间：2026-07-06
> 问题场景：插件设置页面后缀名设置项不显示，根因是 `InputWithHistory` 组件在模板中使用但未导入，Vite 构建不报错，Biome 自动修复误删 import。

## 一、问题现象

### 1.1 表面症状

- 插件设置页面中，string/integer 类型的配置项（如后缀名 `ext`）不显示
- 控制台无报错，Vite 构建成功
- 之前能正常工作的代码，运行 Biome `lint:fix` 后出现问题

### 1.2 根因链路

```
Biome noUnusedImports 规则无法识别 Vue 模板中使用的组件
  → 误将 InputWithHistory 等组件导入标记为"未使用"
  → biome lint --write 自动删除这些 import
  → Vue 模板中 <InputWithHistory> 标签无法解析为组件
  → Vue 编译器将其当作原生自定义元素处理，不报错
  → 页面渲染时该位置空白，无任何错误提示
```

## 二、为什么 Vite 构建不报错？

### 2.1 Vue 3 的宽松处理策略

Vue 3 编译器对于模板中未注册的组件：
- **开发环境**：仅在控制台输出 `Failed to resolve component` 警告
- **生产构建**：不报错，生成 `resolveComponent` 运行时调用
- 运行时如果找不到组件，会将其当作原生 HTML 元素或自定义元素渲染

### 2.2 验证方法

```bash
# 故意删除一个组件导入，然后构建
rm -rf dist && vite build
# 结果：构建成功，无任何错误提示
```

### 2.3 为什么不报错？

Vue 的设计哲学：
- 组件名和原生 HTML 标签名可能冲突（如 `<section>`、`<header>`）
- 自定义元素（Web Components）是合法的 HTML 元素
- Vue 无法静态判断一个标签到底是"未导入的组件"还是"自定义元素"

## 三、Biome 误删问题

### 3.1 根因

Biome 2.x 的 Vue SFC 分析能力不完整：
- `noUnusedImports` 规则只能分析 `<script>` 块中的引用
- 无法识别 `<template>` 中使用的组件、变量、指令等
- 导致模板中使用的组件导入被误判为"未使用"

### 3.2 临时解决方案

在 `biome.jsonc` 中禁用相关规则：

```jsonc
{
  "linter": {
    "rules": {
      "correctness": {
        "noUnusedImports": "off",
        "noUnusedVariables": "off"
      }
    }
  }
}
```

> ⚠️ 注意：禁用规则只是避免误删，**并不能帮助发现未导入的组件**。

## 四、解决方案：自定义 Vite 插件

### 4.1 方案选型对比

| 方案 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| ESLint + eslint-plugin-vue | 准确、生态成熟 | 重、慢、与 Biome 重复 | ❌ 不采用 |
| unplugin-vue-components | 自动导入，无需手动写 import | 构建耗时增加（+46%） | ⚠️ 可选兜底 |
| 自定义 Vite 插件 | 轻量、快速、精准检测 | 需要自己维护 | ✅ 主方案 |

### 4.2 最终策略：双插件兜底

```
开发阶段 (dev)：自定义检查插件 → 控制台实时警告
构建阶段 (build)：
  1. 自定义检查插件 → 发现未导入组件直接报错，构建失败
  2. unplugin-vue-components → 自动导入兜底（可选）
```

### 4.3 自定义插件特性

- ✅ 支持开发模式热更新检测（保存文件立即检查）
- ✅ 支持构建阶段报错（failOnError）
- ✅ 性能优化：文件级缓存、排除 node_modules 等目录
- ✅ 准确识别：默认导入 + 命名导入
- ✅ 可配置：全局组件白名单、排除规则
- ✅ 自动识别 Ionic 组件（`ion-` 前缀）

### 4.4 插件位置

```
packages/shared-components/src/vite-plugins/vue-component-check.ts
```

## 五、使用方法

### 5.1 基本配置

在 `vite.config.ts` 中添加：

```typescript
import { vueComponentCheckPlugin } from '../packages/shared-components/src/vite-plugins/vue-component-check'

export default defineConfig({
  plugins: [
    vueComponentCheckPlugin({
      dev: process.env.NODE_ENV !== 'production',
      failOnError: process.env.NODE_ENV === 'production',
    }),
    vue(),
    // ... 其他插件
  ],
})
```

### 5.2 配置选项

```typescript
interface VueComponentCheckOptions {
  componentDirs?: string[];      // 组件目录，默认 ['src/components']
  globalComponents?: string[];   // 全局组件白名单（如第三方库组件）
  failOnError?: boolean;         // 发现错误时是否终止构建，默认 true
  dev?: boolean;                 // 是否为开发模式（仅警告不报错），默认 false
  exclude?: RegExp[];            // 排除文件规则
}
```

### 5.3 第三方库组件配置

如果使用了第三方库的组件（如 `@tdesign-vue-next/chat`、`markstream-vue`），需要加入 `globalComponents` 白名单：

```typescript
vueComponentCheckPlugin({
  globalComponents: [
    'ChatThinking',
    'ChatMarkdown',
    'MarkdownRender',
  ],
})
```

### 5.4 + unplugin-vue-components 双插件兜底（可选）

```bash
pnpm add -D unplugin-vue-components
```

```typescript
import Components from 'unplugin-vue-components/vite'

export default defineConfig({
  plugins: [
    vueComponentCheckPlugin({
      failOnError: process.env.NODE_ENV === 'production',
    }),
    ...(process.env.NODE_ENV === 'production' ? [Components({
      dirs: ['src/components', '../packages/shared-components/src/components'],
      extensions: ['vue'],
      deep: true,
      dts: 'src/components.d.ts',
    })] : []),
    vue(),
  ],
})
```

## 六、多项目配置共享

### 6.1 为什么容易漏？

- 每个项目独立配置 `vite.config.ts`
- 新项目创建时容易忘记添加插件
- 插件升级时需要同步修改多个项目

### 6.2 解决方案

**共享包 + 文档规范**：
1. 插件代码统一放在 `packages/shared-components/src/vite-plugins/`
2. 新项目创建时必须按照本文档配置
3. CI 构建时插件会报错拦截，避免遗漏

### 6.3 已配置项目

| 项目 | 状态 | 位置 |
|------|------|------|
| encv-mobile | ✅ 已配置 | `app/encv-mobile/vite.config.ts` |
| simverse-frontend | ✅ 已配置 | `app/simverse-frontend/vite.config.ts` |

## 七、本次修复的问题清单

### 7.1 encv-mobile 项目（共修复 30+ 处）

**核心 Bug 修复**：
- `ConfigFieldItem.vue`：添加 `InputWithHistory` 导入 → 修复插件设置页面后缀名不显示

**agent 模块组件导入修复**：
- `TaskDetailModal.vue`：7 个组件导入
- `AgentTaskMessage.vue`：`StatusBadge`
- `AssistantMessage.vue`：`MarkdownStream`
- `BlockHeader.vue`：`StatusBadge`
- `ErrorMessage.vue`：`MessageAuthor`
- `FileChangeSummaryMessage.vue`：`StatusBadge`
- `MarkdownStream.vue`：`MarkdownRender`
- `PlanBlock.vue`：`BlockHeader`
- `ReasoningMessage.vue`：`MessageAuthor` + `StatusBadge`
- `WebSearchSummaryMessage.vue`：`MessageAuthor` + `StatusBadge`

**其他模块**：
- `ArtPlayerView.vue`：`ErrorStateCard`
- `HomePage.vue`：`AgentEntry`
- `GroupDetail.vue`：若干组件
- ... 共 112 个 Vue 文件全部通过检查

### 7.2 simverse-frontend 项目

- 39 个 Vue 文件全部通过检查，无需修复

### 7.3 packages/shared-components

- 作为库项目，不单独构建，组件被其他项目引用时检查

## 八、性能数据

### 8.1 自定义插件性能

| 项目 | 文件数 | 扫描耗时 |
|------|--------|----------|
| encv-mobile | 112 | ~7s |
| simverse-frontend | 39 | ~0.8s |

> 注：首次扫描较慢，热更新时单文件检查 < 10ms

### 8.2 unplugin-vue-components 性能开销

- encv-mobile 构建时间：2.71s → 3.95s（+46%）
- **建议**：开发阶段不启用，构建阶段可选启用

## 九、最佳实践

### 9.1 组件命名

- 组件文件使用 PascalCase：`InputWithHistory.vue`
- 模板中使用 PascalCase 标签：`<InputWithHistory />`
- 不要使用 kebab-case 标签，避免插件检测不到

### 9.2 导入规范

- 本地组件：默认导入，`.vue` 后缀不能省
  ```typescript
  import InputWithHistory from "@/components/InputWithHistory.vue";
  ```
- 第三方库组件：根据库的导出方式选择默认/命名导入
  ```typescript
  import MarkdownRender from "markstream-vue";
  import { ChatThinking } from "@tdesign-vue-next/chat";
  ```

### 9.3 全局组件

- Ionic 组件（`ion-` 前缀）插件自动识别，无需配置
- 其他全局注册的组件请加入 `globalComponents` 白名单

### 9.4 避免 Biome 误删

- 已禁用 `noUnusedImports` 和 `noUnusedVariables` 规则
- 不要随意重新启用这些规则，除非 Biome 官方修复了 Vue SFC 分析能力

## 十、已知局限

1. **命名导入的组件名可能与标签名不一致**：
   - 第三方库可能导出的是函数/对象，不是组件
   - 目前通过 `globalComponents` 白名单手动排除

2. **无法检测动态组件名**：
   - `<component :is="dynamicName">` 中 `dynamicName` 是变量时无法检测
   - 建议动态组件也显式导入并注册

3. **kebab-case 标签检测不全**：
   - 插件主要检测 PascalCase 标签（`InputWithHistory`）
   - kebab-case（`input-with-history`）可能漏检
   - 建议统一使用 PascalCase

## 十一、相关文件索引

| 文件 | 说明 |
|------|------|
| `packages/shared-components/src/vite-plugins/vue-component-check.ts` | 自定义 Vite 插件源码 |
| `app/encv-mobile/vite.config.ts` | encv-mobile 配置示例 |
| `app/simverse-frontend/vite.config.ts` | simverse-frontend 配置示例 |
| `app/biome.jsonc` | Biome 配置（已禁用相关规则） |
