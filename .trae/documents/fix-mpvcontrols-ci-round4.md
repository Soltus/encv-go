# MpvControls.kt CI Round 4 修复计划

## 错误分析（来自 CI 日志 L176-178）

```
e: MpvControls.kt:266:41 Unresolved reference 'outlined'.
e: MpvControls.kt:273:41 Unresolved reference 'outlined'.
e: MpvControls.kt:303:22 Type 'MutableState<Float>' has no method 'setValue(Nothing?, KMutableProperty0<*>, Float)', so it cannot serve as a delegate for var
```

## 修复步骤

### Step 1: 修正 import — `icons.outlined` → `Icons.Outlined`

**文件**: [MpvControls.kt](app/encv-mobile/plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvControls.kt)

- **L32** `import androidx.compose.material.icons.outlined.Subtitles` → `import androidx.compose.material.icons.Icons.Outlined.Subtitles`
- **L33** `import androidx.compose.material.icons.outlined.MusicNote` → `import androidx.compose.material.icons.Icons.Outlined.MusicNote`

原因：`material-icons-extended` 的 Outlined 图标包路径是 `androidx.compose.material.icons.Icons.Outlined`（**大写 O**），不是小写的 `outlined`。

### Step 2: 修正引用点 — `Icons.outlined.` → `Icons.Outlined.`

- **L266** `imageVector = Icons.outlined.Subtitles` → `imageVector = Icons.Outlined.Subtitles`
- **L273** `imageVector = Icons.outlined.MusicNote` → `imageVector = Icons.Outlined.MusicNote`

### Step 3: 修正 mutableStateOf 的 by 委托

**L303 当前代码**:
```kotlin
var sliderVolume by remember { mutableStateOf(volume) }
```

**改为显式 `.value` 访问模式**:
```kotlin
val sliderVolumeState = remember { mutableStateOf(volume) }
```

然后 VolumeSliderRow 内所有 `sliderVolume` 引用替换为 `sliderVolumeState.value`：
- **L311**: `if (sliderVolume > 0f)` → `if (sliderVolumeState.value > 0f)`
- **L317**: `value = sliderVolume,` → `value = sliderVolumeState.value,`
- **L318**: `{ sliderVolume = it }` → `{ sliderVolumeState.value = it }`
- **L319**: `onVolumeChange(sliderVolume)` → `onVolumeChange(sliderVolumeState.value)`
- **L331**: `${(sliderVolume * 100).toInt()}%` → `${(sliderVolumeState.value * 100).toInt()}%`

原因：Kotlin 2.1.0 + Compose BOM 2024.06.00 中，`mutableStateOf<Float>()` 返回的 `MutableState<Float>` 不支持 `by` 委托语法（缺少 `setValue` 运算符）。需要使用显式 `.value` 访问。

### Step 4: 清理日志文件

删除 `/workspace/job_logs_extracted/` 和 `/workspace/job_logs.zip`
