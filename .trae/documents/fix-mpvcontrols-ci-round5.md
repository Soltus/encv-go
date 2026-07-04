# MpvControls.kt CI 修复（基于已通过代码对照）

## 根因分析

对比同项目中**已编译通过**的 `MpvPlayerScreen.kt` 和 `MpvProgressBar.kt`，发现 MpvControls.kt 只有 2 类错误：

### 错误 1：缺少 `setValue` import → by 委托报错

**证据**：MpvPlayerScreen.kt L53 `var volume by remember { mutableStateOf(1f) }` 编译通过（Float + by 委托），因为它有 `import androidx.compose.runtime.setValue`。MpvControls.kt 缺此 import。

### 错误 2：`Icons.outlined` 大小写错误

material-icons-extended 的正确路径是 `Icons.Outlined`（大写 O），不是小写 `outlined`。

## 修复步骤（共改 5 处）

### Step 1：添加缺失的 setValue import

在 L44 (`import androidx.compose.runtime.mutableStateOf`) 之后添加：

```kotlin
import androidx.compose.runtime.setValue
```

这会同时修复 L303 的 `by` 委托错误（无需改 VolumeSliderRow 的任何逻辑代码）。

### Step 2：修正 Subtitles 图标 import（L32）

```diff
- import androidx.compose.material.icons.outlined.Subtitles
+ import androidx.compose.material.icons.Icons.Outlined.Subtitles
```

### Step 3：修正 MusicNote 图标 import（L33）

```diff
- import androidx.compose.material.icons.outlined.MusicNote
+ import androidx.compose.material.icons.Icons.Outlined.MusicNote
```

### Step 4：修正 Subtitles 引用点（L266）

```diff
- imageVector = Icons.outlined.Subtitles,
+ imageVector = Icons.Outlined.Subtitles,
```

### Step 5：修正 MusicNote 引用点（L273）

```diff
- imageVector = Icons.outlined.MusicNote,
+ imageVector = Icons.Outlined.MusicNote,
```

## 参照标准（本项目内权威）

以下 API 用法已在 CI 中验证通过，MpvControls.kt 遵循相同模式：

| 模式 | 来源文件 | 行号 |
|------|---------|------|
| `var x by remember { mutableStateOf<T>(init) }` | MpvPlayerScreen.kt | L46, L49-L53 |
| `mutableLongStateOf(0L)` | MpvPlayerScreen.kt | L47-48 |
| `var sliderValue by remember { mutableFloatStateOf(x) }` | MpvProgressBar.kt | L44 |
| `Slider(...)` + `SliderDefaults.colors(...)` | MpvProgressBar.kt | L57-75 |
| `MaterialTheme.colorScheme.*` | 两文件均用 | 全文 |
| `Icons.Default.XXX` (Filled icons) | MpvControls.kt 本身 | L204, L279-280 等 |

## 清理

完成后删除 `/workspace/job_logs_extracted/` 和 `/workspace/job_logs.zip`。
