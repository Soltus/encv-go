# 修复 ExtensionsPage.vue 构建错误 + 全面排查计划

## 问题根因分析

### 核心错误：第 209 行使用未导入的 `GoProcess`

[ExtensionsPage.vue:209](file:///workspace/app/encv-mobile/src/views/ExtensionsPage.vue#L209) 调用了 `GoProcess.installPlugin({ apkPath })`，但：
- [GoProcess.ts:13](file:///workspace/app/encv-mobile/src/plugins/GoProcess.ts#L13) 中 `const GoProcess = registerPlugin(...)` 是**未导出的内部变量**
- 该文件的设计模式是：**所有插件方法都通过 export 的包装函数暴露**（如 `restartBackend()`、`stopBackend()` 等）
- 但 `installPlugin` 方法**缺少对应的包装函数**

### 上次修复的问题（打地鼠式修复）
上次只看到 import 行有 TS2459 就移除了 `GoProcess`，没有检查文件内是否实际使用了它。结果导致 TS2552——同一个变量的两个不同报错分两次出现。

## 修改方案

### 修改 1：在 GoProcess.ts 添加 `installExtensionApk` 包装函数

> **命名说明**：项目中 `plugin` 有多重含义（encv-go 扩展插件、Capacitor 原生插件、combo/lite 模式等），使用 `installExtensionApk` 明确语义——通过 GoProcess 安装一个 APK 作为扩展插件。

在 [GoProcess.ts](file:///workspace/app/encv-mobile/src/plugins/GoProcess.ts) 中新增 export 函数，与现有模式保持一致：

```typescript
export async function installExtensionApk(apkPath: string): Promise<{ success: boolean; method?: string }> {
  try {
    return await GoProcess.installPlugin({ apkPath })
  } catch (e) {
    console.error('[ENCV] GoProcess.installPlugin() failed:', e)
    return { success: false }
  }
}
```

### 修改 2：ExtensionsPage.vue 使用新函数替代原始调用

将第 209 行从：
```typescript
const result = await GoProcess.installPlugin({ apkPath })
```
改为：
```typescript
const result = await installExtensionApk(apkPath)
```

import 行更新为：
```typescript
import { isNative, installExtensionApk } from '@/plugins/GoProcess'
```

## 全面排查结果

已对整个 `src/` 目录进行 grep 排查，结论：

| 文件 | 状态 |
|------|------|
| [ExtensionsPage.vue](file:///workspace/app/encv-mobile/src/views/ExtensionsPage.vue) | ⚠️ 唯一问题文件，第 209 行直接使用裸 `GoProcess` |
| [Settings.vue](file:///workspace/app/encv-mobile/src/views/Settings.vue) | ✅ 只用 `isNative` |
| [ServerDetail.vue](file:///workspace/app/encv-mobile/src/views/ServerDetail.vue) | ✅ 全部使用包装函数 |
| [Files.vue](file:///workspace/app/encv-mobile/src/views/Files.vue) | ✅ 全部使用包装函数 |
| [ArtPlayerView.vue](file:///workspace/app/encv-mobile/src/views/ArtPlayerView.vue) | ✅ 只用 `isNative` |
| [PluginSettings.vue](file:///workspace/app/encv-mobile/src/views/PluginSettings.vue) | ✅ 只用 `isNative` |
| [App.vue](file:///workspace/app/encv-mobile/src/App.vue) | ✅ 全部使用包装函数 |
| [useServerStatus.ts](file:///workspace/app/encv-mobile/src/composables/useServerStatus.ts) | ✅ 全部使用包装函数 |

**仅此一处需要修复**，无其他隐患。

## 关于 CI 只输出一个错误的说明

这是 **vue-tsc 的默认行为**，不是配置问题：

- vue-tsc 底层调用 tsc，tsc 在遇到**无法继续类型检查的错误**（如 `Cannot find name`）时会在该文件处停止增量检查
- `vue-tsc --noEmit` 本身会输出所有文件的错误，但当某个 `.vue` 文件的 `<script setup>` 有致命引用错误时，可能影响后续文件的解析
- **这不是 CI 截断输出**，而是编译器本身提前终止

如果希望强制看到更多错误，可以在 `package.json` 的 build 脚本中考虑改用：
```json
"build": "vue-tsc --noEmit 2>&1 && vite build"
```
但这不会改变 vue-tsc 的错误报告行为——它已经是尽可能多地报告了。

**根本解决方案是保证代码无误**，而不是依赖编译器输出全部错误。
