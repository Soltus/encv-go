# 修复 Lynx 黑屏 — 缺少 root.render()

## 根因确认

### 对比官方 entry 格式

**官方格式** (`devtool-switch/src/index.tsx`)：
```tsx
import { root } from '@lynx-js/react';
import App from './App.jsx';
root.render(<App />);     // ← 关键！挂载组件
```

**我们当前的 App.tsx**：
```tsx
export function App() {    // ← 只导出函数，没人调用 root.render()
  return <page>...</page>
}
```

### 为什么 onLoadSuccess/onFirstScreen 仍然触发

Lynx 有两层渲染：
1. **模板层 (TASM)**：解析 `.lynx.bundle` 二进制 → 构建 DOM 树 → 触发 `onLoadSuccess`/`onFirstScreen`
2. **React 层 (ReactLynx)**：执行 JS bundle 中的 React 代码 → 通过 `root.render()` 挂载到 TASM DOM 上

**没有 `root.render()`，第 2 步永远不会发生！** 模板层报告成功是因为它只关心第 1 步。

## 修复步骤

### Step 1：重构 entry 文件，添加 root.render()

**方案**：将 `src/App.tsx` 改为标准 entry 格式，创建独立的组件文件。

1. 创建 `src/components/AppComponent.tsx`：从当前 App.tsx 移入纯组件逻辑
2. 重写 `src/App.tsx` 为标准 entry 格式：`import + root.render()`

具体改动：

**新文件 `src/components/AppComponent.tsx`**（从当前 App.tsx 复制，去掉 export）：

```tsx
import { useCallback, useEffect, useState, useInitData, useLynxGlobalEventListener } from "@lynx-js/react";
import { PlayerControls } from "./PlayerControls";
import "../App.css";

interface InitData {
  filePath: string;
  fileName: string;
  mimeType: string;
  isExternal: boolean;
}

type PlayerState = "idle" | "loading" | "playing" | "paused" | "ended" | "error";

export function AppComponent() {
  // ... 所有现有逻辑不变 ...
}
```

**重写 `src/App.tsx` 为 entry**：

```tsx
import { root } from '@lynx-js/react';
import { AppComponent } from './components/AppComponent';

root.render(<AppComponent />);

if (import.meta.webpackHot) {
  import.meta.webpackHot.accept();
}
```

### Step 2：更新 lynx.config.ts 的 entry 路径（如果需要）

当前配置是 `entry: './src/App.tsx'`，如果改为上述结构则不需要修改（entry 仍然是 App.tsx）。

## 预期效果

- `root.render(<AppComponent />)` 执行后，React 组件树正确挂载到 LynxView
- 用户能看到深蓝色背景 (#001030) + 居中的 ▶ 播放按钮 + 文件名
- 这是 Lynx ReactLynx 应用的**标准初始化模式**
