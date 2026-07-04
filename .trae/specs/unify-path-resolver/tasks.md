# Tasks

- [x] Task 1: 创建 `usePathResolver.ts` composable
  - [x] 1.1 实现 `normalize(rawPath: string): string` — 去除重复 `/`、`\` 转 `/`、确保绝对路径以 `/` 开头、trim 空白
  - [x] 1.2 实现 `resolveFileItem(file: FileItem): string` — 提取 file.path 并 normalize
  - [x] 1.3 实现 `isAbsolutePath(path: string): boolean`
  - [x] 1.4 实现 `getMockPaths(): string[] | null` — DEV 环境返回预设 mock 路径数组，非 DEV 返回 null
  - [x] 1.5 编写单元测试覆盖 normalize 边界（空字符串、重复斜杠、Windows 路径、已规范路径）

- [x] Task 2: 修改 Files.vue 加密/解密入口使用 resolveFileItem
  - [x] 2.1 import usePathResolver，在 handleEncryptFile/handleDecryptFile 中用 `resolveFileItem(file)` 替代 `file.path`
  - [x] 2.2 验证 router.push 的 query.source 值为归一化后的路径

- [x] Task 3: 修改 Tasks.vue processQueryAction 使用 normalize
  - [x] 3.1 import usePathResolver，对 `route.query.source` 做 `normalize()` 后传入 `openNewTaskModal()`
  - [x] 3.2 处理 source 为 undefined/null 的防御场景

- [x] Task 4: 修改 useTaskForm.ts doPredict 添加路径校验与归一化
  - [x] 4.1 在 doPredict 入口处对 sourcePath 做 normalize（双重保障：即使调用方已归一化也再处理一次）
  - [x] 4.2 添加空路径守卫：sourcePath 为空时直接 return 不调 API（消除 400）
  - [x] 4.3 非绝对路径自动补 `/` 前缀

- [x] Task 5: 验证 predictPlugin 不再报 400
  - [x] 5.1 vue-tsc --noEmit + vite build 零错误 ✅
  - [ ] 5.2 浏览器测试 FAB 入口 → modal 正常打开 → 控制台无 400 错误
  - [ ] 5.3 浏览器测试长按加密入口 → modal 打开 + 路径预填 → predictPlugin 成功或静默跳过

# Task Dependencies
- [Task 2, 3, 4] depends on [Task 1]
- [Task 5] depends on [Task 2, 3, 4]
