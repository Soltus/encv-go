# 修复：fetchTextPreviewExts 崩溃 + IonMenu 抽屉无法打开

## 问题 1：fetchTextPreviewExts TypeError

**文件**: [encv.ts L365](app/encv-mobile/src/api/encv.ts#L365)

**错误**: `TypeError: data.custom_extensions is not iterable`

**根因**: 后端 API 返回的 `custom_extensions` 字段可能为 `null`、`undefined` 或非数组值（如空字符串），直接 `...data.custom_extensions` 展开到 Set 时抛出 TypeError。

**修复**:
```typescript
// Before (crashes on null/undefined)
const all = new Set([...data.extensions, ...data.custom_extensions])

// After (defensive)
const all = new Set([...(data.extensions || []), ...(data.custom_extensions || [])])
```

## 问题 2：IonMenu 抽屉无法打开

**文件**: [Files.vue L47](app/encv-mobile/src/views/Files.vue#L47) + [L11-12](app/encv-mobile/src/views/Files.vue#L11-L12)

**根因**: Ionic Vue 的 `IonMenu` 组件不支持通过 `:opened` prop 双向绑定控制开关。`opened` 是 **单向受控 prop**，组件内部状态管理与外部 ref 不同步——设置 `showSideDrawer = true` 时 IonMenu 不一定响应。

**正确做法**: 使用 Ionic 的 `menuController` API：
```typescript
import { menuController } from '@ionic/vue'

// 打开
await menuController.open('plugin-menu')
// 关闭
await menuController.close()
```

**修改点**:
1. 从 `@ionic/vue` 导入 `menuController`
2. 模板中移除 `:opened="showSideDrawer"` 和 `@ionDidClose`
3. 按钮 `@click` 改为调用 `openSideDrawer()` 函数 → 内部 `menuController.open('plugin-menu')`
4. 插件项点击后也用 `menuController.close()` 关闭抽屉
5. 可保留 `showSideDrawer` ref 用于其他逻辑（或完全移除）

## 文件清单

| 文件 | 修改 |
|------|------|
| `src/api/encv.ts` L365 | `custom_extensions` 防御性展开 |
| `src/views/Files.vue` | import menuController; IonMenu 移除 :opened; 按钮改用 menuController.open/close |

## 验证

```bash
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit && npx vite build
```
