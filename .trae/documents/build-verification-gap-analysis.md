# 构建验证遗漏分析

## 问题

用户指出：本地 `vue-tsc --noEmit` 没有报出 TS6133（未使用导入）错误，但 CI 构建却报错了。

## 原因分析

tsconfig.json 中 **已经配置了** `noUnusedLocals: true` 和 `noUnusedParameters: true`，理论上 `vue-tsc --noEmit` 应该报出这些错误。

实际没报错的原因是：**`vue-tsc` 对 `.vue` 文件的类型检查存在已知限制**。`vue-tsc` 使用 `@vue/language-core` 解析 `.vue` SFC，在 `<script setup>` 中，未使用的导入有时不会被标记为错误，因为 Vue 编译器可能认为这些导入是模板引用或组件注册。这是 `vue-tsc` 的一个已知行为差异。

具体到本次：
- `watch` 和 `getExternalStreamUrl` 在 `<script setup>` 中导入但未使用
- `vue-tsc` 没有报错，但更严格的构建流程（如 Vite 的 `build` 阶段或 CI 中的 `vue-tsc --noEmit` 配合不同版本）可能报错

## 改进措施

在项目规则中记录：**构建验证不能仅依赖 `vue-tsc --noEmit`**，还应运行 `vite build`，因为 Vite 构建阶段的 Rollup 会检测未使用的导入并报错。
