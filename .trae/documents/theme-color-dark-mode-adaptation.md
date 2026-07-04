# 主题色暗黑模式不生效 — 根因修复计划

## 真正的根因：CSS Custom Property 层叠覆盖

### 问题机制

```
DOM 继承链：
<html (:root)>                          body (class="dark")
  ├─ inline style 设置:                  ├─ variables.css 规则设置:
  │  --ion-color-primary: #8b5cf6        │  --ion-color-primary: #6a9eff
  │  --ion-color-primary-rgb: ...        │  --ion-color-primary-rgb: ...
  │  （用户选择的 Purple）                │  （硬编码默认蓝）
  │                                      │
  └───────────── 继承 ───────────────────→ ├─ 直接声明 WIN 了继承值
                                             ↓
                       <ion-button> 读 var(--ion-color-primary)
                                             ↓
                                       得到 #6a9eff ❌
                                       （用户色被覆盖！）
```

### CSS 规范原理

CSS 自定义属性（Custom Properties）遵循普通层叠规则：

> 当一个自定义属性在某个元素上被**直接声明**（通过匹配该元素的选择器规则），它会覆盖从父元素**继承**来的同名属性值——即使父元素的值是通过 inline style 设置的。

**关键代码位置**：
- [useTheme.ts:77](app/encv-mobile/src/composables/useTheme.ts#L77) — 在 `document.documentElement`（即 `<html>` = `:root`）上设 inline style
- [variables.css:73-79](app/encv-mobile/src/theme/variables.css#L73-L79) — 在 `body.dark` 上直接重新声明了 `--ion-color-primary-*` 全部 7 个变量

**结果**：`body.dark` 的直接声明「赢」了 `:root` inline style 的继承值 → 用户色永远到不了子组件。

---

## 修复方案（二选一）

### 方案 A：删除 `body.dark` 中被动态管理的变量 ✅ 推荐

**原理**：`--ion-color-primary-*` 这组变量由 `useTheme.ts` 在运行时完全管理，`variables.css` 中的静态默认值只是 fallback。既然运行时会始终设置它们，`body.dark` 中的重复声明就是**有害的冲突源**。

**改动文件**：仅 [variables.css](app/encv-mobile/src/theme/variables.css)

**具体操作**：从 `body.dark {}` 块中删除以下 7 行：

```css
body.dark {
  --ion-color-primary: #6a9eff;           ← 删除
  --ion-color-primary-rgb: 106, 158, 255;  ← 删除
  --ion-color-primary-contrast: #000000;   ← 删除
  --ion-color-primary-contrast-rgb: 0,0,0;← 删除
  --ion-color-primary-shade: #5d8be0;      ← 删除
  --ion-color-primary-tint: #79a8ff;       ← 删除
  // ... 其余变量保持不变（secondary/tertiary/success 等）
}
```

**保留不变的暗色变量**（这些不由 useTheme 管理，无冲突）：
- `--ion-color-secondary-*`
- `--ion-color-tertiary-*`
- `--ion-color-success/warning/danger/medium/light-*`
- 所有背景/文字/overlay 变量

**优点**：
- 改动最小（只删 7 行）
- 不增加任何复杂度
- 根除冲突源

**缺点**：
- 如果 `useTheme.initTheme()` 在首次渲染前没执行完，暗色模式下 primary 会 fallback 到 `:root` 的亮色值（`#4f8cff`），这个窗口期极短（<1ms）

---

### 方案 B：inline style 加 `!important`

**改动文件**：仅 [useTheme.ts](app/encv-mobile/src/composables/useTheme.ts)

**具体操作**：`applyColor()` 中全部加 `'important'` 参数：

```typescript
function applyColor(color: string) {
  // ...
  root.style.setProperty('--ion-color-primary', color, 'important')
  root.style.setProperty('--ion-color-primary-rgb', rgb, 'important')
  root.style.setProperty('--ion-color-primary-contrast', contrast, 'important')
  root.style.setProperty('--ion-color-primary-contrast-rgb', hexToRgb(contrast), 'important')
  root.style.setProperty('--ion-color-primary-shade', darker(color, 10), 'important')
  root.style.setProperty('--ion-color-primary-tint', lighter(color, 10), 'important')
}
```

**优点**：不改 CSS 文件

**缺点**：
- 使用 `!important` 违反最佳实践
- 治标不治本——冲突源仍在，未来可能引发其他问题

---

## 决定：采用方案 A

方案 A 是正确架构修复——动态管理的变量不应在样式表中重复声明。

---

## 实施步骤

1. **编辑 `variables.css`**：从 `body.dark {}` 块中删除 7 行 `--ion-color-primary-*` 声明
2. **验证**：确认亮色/暗色模式下切换主题色均正常生效

## 验证场景

| # | 操作 | 预期 |
|---|------|------|
| 1 | 亮色模式选 Purple | 按钮/FAB/toggle 显示紫色 ✅ |
| 2 | 切换暗色模式 | 保持紫色（不再回退为蓝色）✅ |
| 3 | 暗色模式选 Orange | 显示橙色 ✅ |
| 4 | 暗色切回亮色 | 保持用户选择的颜色 ✅ |
| 5 | 刷新页面恢复 | localStorage 读取后正确应用 ✅ |
| 6 | 未选过自定义色时暗色模式 | 显示 `:root` 默认值 `#4f8cff` 作为 fallback ✅ |
