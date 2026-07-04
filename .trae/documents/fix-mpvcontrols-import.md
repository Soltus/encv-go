# 修复 MpvControls.kt 全部编译错误（CI 日志分析）

## 错误清单（共 5 类 15+ 处）

### 错误 A: import 路径大小写（4 处）
`androidx.compose.material.icons.Outlined` 包不存在，正确为全小写 `outlined`

| 行号 | 当前（错误） | 正确 |
|------|-------------|------|
| L32 | `Icons.Outlined.Subtitles` | `Icons.outlined.Subtitles` |
| L33 | `Icons.Outlined.Audiotrack` | `Icons.outlined.Audiotrack` |
| L262 | `Icons.Outlined.Subtitles` | `Icons.outlined.Subtitles` |
| L269 | `Icons.Outlined.Audiotrack` | `Icons.outlined.Audiotrack` |

### 错误 B: 缺少 Material3 组件 import（2 处）
新增的 `VolumeSliderRow` 使用了 `Slider` 和 `SliderDefaults` 但未 import：

```kotlin
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
```

### 错误 C: 缺少 Compose Runtime import（2 处）
新增的 `VolumeSliderRow` 使用了 `remember` 和 `mutableFloatStateOf`：

```kotlin
import androidx.compose.runtime.remember
import androidx.compose.runtime.mutableFloatStateOf
```
（需确认现有 import 区是否已包含这些；如已包含则忽略此项）

### 错误 D: AudioOnlyLayout 签名/调用不匹配
`AudioOnlyLayout` 函数签名已添加 `volume` + `onVolumeChange` 参数，但需确认：
- 函数定义处参数完整
- MpvControls 中调用 AudioOnlyLayout 时传参完整（含 `volume=volume`, `onVolumeChange=onVolumeChange`）
- AudioOnlyLayout 内部调用 `VolumeIcon` 和 `VolumeSliderRow` 存在且正确

### 错误 E: VolumeOff 图标引用
`Icons.Default.VolumeOff` 在 Material Icons 中可能不存在。Compose Material Icons 的音量图标集：
- ✅ `Icons.Default.VolumeUp`
- ✅ `Icons.Default.VolumeDown`
- ✅ `Icons.Default.VolumeMute`（或 `VolumeOff`）
- 需确认 `VolumeOff` 是否可用，若不可用则替换为 `VolumeMute`

## 修复步骤

### Step 1: 修复所有 import 问题
在文件顶部 import 区域：
1. `Outlined` → `outlined`（2 处）
2. 追加 `Slider`, `SliderDefaults` import（如果缺失）
3. 追加 `remember`, `mutableFloatStateOf` import（如果缺失）

### Step 2: 修复所有引用处的 Outlined 大小写（2 处）
L262、L269 的 `Icons.Outlined` → `Icons.outlined`

### Step 3: 确认/修复 VolumeOff 图标
检查 `Icons.Default.VolumeOff` 是否可解析，若不行改用 `Icons.Default.VolumeMute`

### Step 4: 验证函数调用链完整性
逐个确认：
- `MpvControls()` → `VideoPlaybackLayout()` 参数传递完整
- `MpvControls()` → `AudioOnlyLayout()` 参数传递完整
- `VideoPlaybackLayout()` → `BottomBar()` / `VolumeIcon()` / `VolumeSliderRow()` 参数匹配
- `AudioOnlyLayout()` → `VolumeIcon()` / `VolumeSliderRow()` 参数匹配

## 清理
- 删除 `job_logs_extracted/`
- 删除 `job_logs.zip`
