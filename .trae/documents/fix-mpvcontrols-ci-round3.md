# MpvControls.kt CI 编译修复（第3轮）

## 错误分析（11 个编译错误，3 大类）

### 🔴 类别 A：VideoPlaybackLayout / BottomBar 函数定义缺少参数（8 处）

**根因**：之前修改时只更新了 `MpvControls()` 的签名和调用处传参，但 **忘记更新 `VideoPlaybackLayout()` 和 `BottomBar()` 的函数定义本身**！

| 行号 | 错误 | 原因 |
|------|------|------|
| L125 | `No parameter 'volume' found` | VideoPlaybackLayout 定义中无此参数 |
| L133 | `No parameter 'onVolumeChange' found` | 同上 |
| L134 | `No parameter 'onToggleSubtitle' found` | 同上 |
| L135 | `No parameter 'onCycleAudio' found` | 同上 |
| L645 | `Unresolved reference 'volume'` | BottomBar 定义中无此参数 |
| L649 | `Unresolved reference 'onVolumeChange'` | 同上 |
| L650 | `Unresolved reference 'onToggleSubtitle'` | 同上 |
| L651 | `Unresolved reference 'onCycleAudio'` | 同上 |

**修复**：
1. `VideoPlaybackLayout` 签名（L584-600）追加 4 个参数：`volume: Float`, `onVolumeChange: (Float) -> Unit`, `onToggleSubtitle: () -> Unit`, `onCycleAudio: () -> Unit`
2. `BottomBar` 签名（L232-244）确认已有这 4 个参数（上次已加，需验证）

### 🟡 类别 B：Icons.outlined 引用仍无法解析（2 处）

| 行号 | 错误 |
|------|------|
| L266 | `Unresolved reference 'outlined'` |
| L273 | `Unresolved reference 'outlined'` |

**原因**：import 已改为 `Icons.outlined`（小写），但该图标在当前 Compose Material Icons 版本中可能不存在。`Audiotrack` 在 outlined 集合中不存在。

**修复**：
- `Subtitles` → 保留 `Icons.outlined.Subtitles`（标准图标）
- `Audiotrack` → 替换为 `Icons.outlined.MusicNote` 或 `Icons.Default.MusicNote`（更通用的音频图标）

### 🟢 类别 C：mutableFloatStateOf delegate 不兼容（1 处）

| 行号 | 错误 |
|------|------|
| L303 | `Type 'MutableFloatState' has no method 'setValue(...)'` |

**原因**：Compose Runtime 版本中 `mutableFloatStateOf` 返回的 `MutableFloatState` 不支持 Kotlin `by` 委托语法中的 `setValue` operator。

**修复**：`var sliderVolume by remember { mutableFloatStateOf(volume) }` → `var sliderVolume by remember { mutableStateOf(volume) }`，同时移除 `mutableFloatStateOf` 的 import。

## 修复步骤

### Step 1: VideoPlaybackLayout 追加缺失的 4 个参数
在 L599 `onToggleFullscreen: () -> Unit` 之后、`onBack: () -> Unit` 之前插入：
```kotlin
    volume: Float = 1f,
    onVolumeChange: (Float) -> Unit,
    onToggleSubtitle: () -> Unit,
    onCycleAudio: () -> Unit,
```

### Step 2: 替换不存在的 Audiotrack 图标
- L273: `Icons.outlined.Audiotrack` → `Icons.outlined.MusicNote`
- L33 import: `Audiotrack` → `MusicNote`

### Step 3: 修复 mutableFloatStateOf 兼容性
- L303: `mutableFloatStateOf(volume)` → `mutableStateOf(volume)`
- L44 import: 删除 `mutableFloatStateOf`，保留 `mutableStateOf`（如已有则只删除前者）

### Step 4: 清理日志
- 删除 `job_logs_extracted/`
- 删除 `job_logs.zip`
