# 播放器控件 + 路径校验 + 设置页箭头 修复方案

## 问题分析

### 问题 1：播放器控件全部错位，缺少控件，太丑

**根因**：
1. **CSS 布局问题**：Lynx 使用 Flexbox 但默认 `flex-direction: column`，而 `.TopBar` 和 `.BottomBar` 需要 `flex-direction: row`，当前虽然设置了但可能被父级覆盖
2. **控件不完整**：缺少成熟播放器应有的：快进/快退按钮、音量控制、倍速选择、锁屏按钮、手势提示
3. **视觉设计差**：使用 Unicode 字符（✕ ⤓ ⤢ ⏸ ▶）代替图标，按钮圆形背景 `rgba(255,255,255,0.2)` 太暗看不清，进度条太细（5px），整体没有 Material Design 规范
4. **交互缺失**：没有自动隐藏控件（5秒无操作后隐藏）、没有滑动手势支持、没有双击快进/快退
5. **ProgressBar 放在 BottomBar 内部但 BottomBar 又被 VideoControls/AudioControls 嵌套，层级混乱**

**修复方案**：
- 完全重写 PlayerControls.tsx 和 App.css
- 参考 ExoPlayer/VLC/YouTube 等成熟播放器的控件布局
- 使用 SVG path 代替 Unicode 字符作为图标（Lynx 支持 `<image>` 加载 base64 SVG）
- 实现自动隐藏控件（3秒无操作淡出）
- 添加快进/快退 ±10s 按钮
- 添加倍速选择按钮
- 添加锁屏模式
- 进度条加粗到 4px，拖动时放大到 6px
- 半透明渐变遮罩背景（顶部和底部）
- 优化 Slider 组件样式

### 问题 2：路径校验没有工作

**根因**：
1. **`ion-invalid` 类名无效**：Ionic 的 `ion-input` 不支持通过 `:class` 绑定 `ion-invalid` 来触发错误样式。Ionic 有自己的验证机制（`errorText` + `ion-invalid` 是内部类名）
2. **缺少 `errorText` 属性**：Ionic 7+ 的 `ion-input` 支持 `errorText` prop 来显示错误信息，当前没有使用
3. **缺少 `ion-touched` 类**：Ionic 的错误样式需要 `ion-invalid` + `ion-touched` 同时存在才显示

**修复方案**：
- 移除 `:class="{ 'ion-invalid': sourcePathError }"` 无效绑定
- 使用 `ion-input` 的 `:error-text` prop 显示错误信息
- 使用 `:class="{ 'ion-invalid ion-touched': sourcePathError }"` 触发 Ionic 内置错误样式
- 或者直接用独立的错误提示 `<ion-note>` 元素（更可靠）

### 问题 3：folderOpen 图标不可见 + 插件配置入口双箭头

**根因**：
1. **folderOpen 图标不可见**：`ion-button fill="clear"` 在 `ion-item` 的 `slot="end"` 中，可能被 `ion-item` 的默认内边距或 `--inner-padding-end` 压缩。另外 `folderOpen` 图标可能和 `ion-item` 的 `detail` 箭头重叠
2. **插件配置入口双箭头**：`Settings.vue` 第 78 行 `<ion-item button @click="goPlugins" detail>` 同时有 `detail` 属性（Ionic 自动渲染右箭头）和手动添加的 `<ion-icon :icon="chevronForward" slot="end">`，导致两个箭头

**修复方案**：
- **Settings.vue**：移除 `goPlugins` 项上的 `<ion-icon :icon="chevronForward" slot="end">`，保留 `detail` 属性即可（Ionic 自动渲染箭头）
- **folderOpen 按钮**：确保 `ion-button` 有足够尺寸，添加 `style="--padding-start: 8px; --padding-end: 8px"` 确保可见
- **其他有 `detail` + 手动箭头的项**：同样移除手动箭头

---

## 实现步骤

### Step 1：修复 Settings.vue 双箭头问题

**文件**：`src/views/Settings.vue`

1. 移除 `goPlugins` 项上的手动 `chevronForward` 图标（第 83 行），保留 `detail` 属性
2. 检查其他所有 `button` + `detail` 的 `ion-item`，移除手动添加的 `chevronForward`（第 49、63、227 行）
3. folderOpen 按钮添加明确尺寸样式

### Step 2：修复 PluginSettings.vue 同样的问题

**文件**：`src/views/PluginSettings.vue`

1. folderOpen 按钮添加明确尺寸样式

### Step 3：修复 Tasks.vue 路径校验样式

**文件**：`src/views/Tasks.vue`

1. 移除 `:class="{ 'ion-invalid': sourcePathError }"` 无效绑定
2. 使用 `ion-input` 的 `:error-text` prop：
   ```html
   <ion-input
     v-model="newTaskPath"
     :label="t('tasks.sourcePath')"
     label-placement="stacked"
     placeholder="/path/to/file"
     :error-text="sourcePathError"
     :class="{ 'ion-invalid': !!sourcePathError, 'ion-touched': !!sourcePathError }"
     @ionInput="validateSourcePath"
   ></ion-input>
   ```
3. 移除独立的 `path-error-item`（改用 `error-text` 内联显示）
4. 同样修改 targetPath 输入框

### Step 4：重写播放器控件 — PlayerControls.tsx

**文件**：`lynx-player/src/components/PlayerControls.tsx`

完全重写，参考 ExoPlayer 控件布局：

```
┌─────────────────────────────────────┐
│ ▼ 渐变遮罩（顶部）                    │
│  [✕]  文件名.mp4            [⛶]     │ ← TopBar
│                                     │
│                                     │
│         [⏪]  [▶/⏸]  [⏩]          │ ← CenterArea（3个按钮）
│                                     │
│                                     │
│ ▲ 渐变遮罩（底部）                    │
│  0:45 ━━━━━━━━━━━━━━━━━━━ 3:21     │ ← ProgressBar
│  [🔒] [1x]              [⛶]        │ ← BottomBar
└─────────────────────────────────────┘
```

关键改进：
1. **SVG 图标**：用 base64 内联 SVG 替代 Unicode 字符
2. **渐变遮罩**：顶部和底部添加半透明黑色渐变，提高可读性
3. **快进/快退按钮**：±10s，居中区域三按钮布局
4. **倍速按钮**：1x → 1.5x → 2x → 0.5x 循环切换
5. **锁屏模式**：锁定后只显示进度条，隐藏其他控件
6. **自动隐藏**：5秒无操作后淡出控件（opacity 动画）
7. **进度条改进**：
   - 轨道高度 4px，拖动时 6px
   - 已播放部分用主题色
   - 拖动圆点 16px，白色带阴影
8. **音频模式**：
   - 大封面占位（圆角矩形 + 音符图标）
   - 下方进度条 + 播放按钮
   - 无全屏按钮

### Step 5：重写播放器样式 — App.css

**文件**：`lynx-player/src/App.css`

配合 PlayerControls 重写，关键样式：
1. `.TopGradient` / `.BottomGradient`：渐变遮罩
2. `.ControlBar`：统一按钮样式
3. `.ProgressTrack`：加粗进度条
4. `.SpeedChip`：倍速标签
5. `.LockButton`：锁屏按钮
6. 控件淡入淡出动画

### Step 6：更新 AppComponent.tsx 支持新控件

**文件**：`lynx-player/src/components/AppComponent.tsx`

1. 添加 `playbackRate` 状态
2. 添加 `locked` 状态
3. 添加 `controlsVisible` 自动隐藏逻辑（5秒定时器）
4. 添加 `handleSeekRelative`（±10s 快进/快退）
5. 添加 `handleCycleSpeed`（倍速循环切换）
6. 添加 `handleLock`（锁屏切换）
7. mpv 命令调用：`setPropertyDouble("speed", rate)` 设置倍速

---

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `src/views/Settings.vue` | 移除手动 chevronForward（detail 已自动渲染）+ folderOpen 按钮尺寸 |
| `src/views/PluginSettings.vue` | folderOpen 按钮尺寸 |
| `src/views/Tasks.vue` | 路径校验改用 error-text prop |
| `lynx-player/src/components/PlayerControls.tsx` | 完全重写：SVG 图标 + 渐变遮罩 + 快进快退 + 倍速 + 锁屏 + 自动隐藏 |
| `lynx-player/src/App.css` | 完全重写：新布局样式 + 动画 |
| `lynx-player/src/components/AppComponent.tsx` | 添加倍速/锁屏/自动隐藏/快进快退逻辑 |

## 优先级

1. **Step 1-3**（快速修复）：双箭头 + 路径校验 — 简单直接
2. **Step 4-6**（播放器重写）：核心体验提升 — 工作量最大但最重要
