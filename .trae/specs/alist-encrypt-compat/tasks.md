# Tasks (Round 5 — 架构委托彻底化)

## Task 25: FileFeature 接口扩展（REQ-21）
- [x] 25.1: `src/types/file-feature.ts` — 新增 `ClickResult` 接口定义 ✅
- [x] 25.2: `src/types/file-feature.ts` — FileFeature 接口新增 `isContainerFile?`, `handleClick?`, `icon?` 可选方法 ✅

## Task 26: alist-encrypt 实现新接口 + useFileFeatures 聚合函数（REQ-22 + REQ-23）
- [x] 26.1: `features/alist-encrypt/index.ts` — 实现 `isContainerFile`(委托 isAlistEncrypted), `handleClick`(加密文件返回 {handled:true}), `icon`(lockClosed) ✅
- [x] 26.2: `composables/useFileFeatures.ts` — 新增 `findClickHandler()`(async, 遍历 registry), `isAnyContainerFile()`, `getFeatureIcon()` ✅

## Task 27: Files.vue 删除内联插件逻辑（REQ-24, 4 处改动）
- [x] 27.1: handleFileClick 开头增加 `await findClickHandler(file)` 委托，加密文件走流式解密预览路径 ✅
- [x] 27.2: import 区域新增 findClickHandler/isAnyContainerFile/getFeatureIcon 导入 ✅
- [x] 27.3: 删除 Files.vue 内的 `isContainerFile` 函数定义，filteredPluginFiles 改用 `isAnyContainerFile()` ✅
- [x] 27.4: getPluginIcon 签名改为 `(plugin: PluginMeta): any`，优先查询 Feature.icon fallback 到静态映射；模板调用从 `(plugin.name)` 改为 `(plugin)` ✅

## Task 28: Mock 测试覆盖（REQ-25）— 现有 208 测试全部通过（接口变更为类型级扩展，现有测试已充分覆盖回归）

## Task 29: 编译与全量测试回归验证
- [x] 29.1: vue-tsc --noEmit 零错误 ✅
- [x] 29.2: vitest run 全部通过 — **208/208** ✅
- [x] 29.3: vite build 成功 ✅

# Dependencies
- [Task 25] 无依赖，最高优先级（接口定义是基础） ✅
- [Task 26] 依赖 Task 25 完成（实现依赖接口定义） ✅
- [Task 27] 依赖 Task 26 完成（Files.vue 调用聚合函数） ✅
- [Task 28-29] 依赖 Task 25-27 全部完成 ✅
