# Jetpack Compose 本地编码参考（来自 Android 官方文档）

> 来源：developer.android.com/develop/ui/compose/state （2026-05-27 拉取）
> 本文件作为本项目所有 .kt Compose 代码的编写校验标准。

---

## 一、State 管理（核心！反复出错点）

### 1.1 `mutableStateOf` + `by` 委托 — 完整写法

**官方原文：**
> `by` 委托语法需要以下导入：
> ```
> import androidx.compose.runtime.getValue
> import androidx.compose.runtime.setValue
> ```

**三种等价声明方式（官方示例）：**
```kotlin
// 方式 1：直接访问 .value
val mutableState = remember { mutableStateOf(default) }

// 方式 2：by 委托（需要 getValue + setValue import）✅ 推荐用于简单状态
var value by remember { mutableStateOf(default) }

// 方式 3：解构声明
val (value, setValue) = remember { mutableStateOf(default) }
```

**本项目验证通过的实例** (`MpvPlayerScreen.kt`)：
```kotlin
import androidx.compose.runtime.getValue    // 必须有
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue     // 必须有！！！

// L46: PlayerState (自定义类) ✅ 编译通过
var playerState by remember { mutableStateOf<PlayerState>(PlayerState.Idle) }
// L49: Boolean ✅
var showControls by remember { mutableStateOf(true) }
// L52: Float ✅ ← 这证明了 Float + by 委托完全没问题
var volume by remember { mutableStateOf(1f) }
```

### 1.2 `mutableFloatStateOf` vs `mutableStateOf<Float>` — 都可以用

**本项目验证** (`MpvProgressBar.kt` L44)：
```kotlin
import androidx.compose.runtime.mutableFloatStateOf
var sliderValue by remember { mutableFloatStateOf(clampedProgress)}  // ✅ 通过
```

**两者区别：**
- `mutableFloatStateOf(x)` 返回 `MutableFloatState`，原生支持 `by`
- `mutableStateOf(x)` 返回 `MutableState<Float>`，也支持 `by`（只要 import 了 `setValue`）

---

## 二、Material Icons Extended（反复出错点）

### 2.1 包结构（Android 官方 API Reference 确认）

```
androidx.compose.material.icons.Icons          ← 根对象
├── Icons.AutoMirrored.Filled.XXX              ← 自动镜像填充图标
├── Icons.AutoMirrored.Outlined.XXX            ← 自动镜像描边图标
├── Icons.Default.XXX                          ← 填充图标（Filled 的别名）
├── Icons.Filled.XXX                           ← 填充图标
├── Icons.Outlined.XXX                         ← 描边图标 ⚠️ 大写 O
└── Icons.Rounded.XXX                          ← 圆角图标
```

### 2.2 正确的 Import 写法

```kotlin
// Filled 图标（本项目 MpvControls.kt 已正确使用的）
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.PlayArrow       // ✅ 或 Icons.Default.PlayArrow
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.VolumeUp
import androidx.compose.material.icons.filled.VolumeMute      // ✅ 注意是 VolumeMute 不是 VolumeOff
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.material.icons.filled.FullscreenExit
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.LockOpen
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.automirrored.filled.ArrowBack

// Outlined 图标（⚠️ 大写 O）
import androidx.compose.material.icons.outlined.Subtitles         // ✅ 正确
import androidx.compose.material.icons.outlined.MusicNote         // ✅ 正确
```

### 2.3 使用方式

```kotlin
Icon(
    imageVector = Icons.Default.PlayArrow,        // Filled
    contentDescription = "Play"
)
Icon(
    imageVector = Icons.Outlined.Subtitles,       // Outlined ⚠️ 大写 O
    contentDescription = "Subtitle"
)
```

---

## 三、Material3 组件速查

| 组件 | Import | 本项目使用位置 |
|------|--------|--------------|
| `MaterialTheme` | `androidx.compose.material3.MaterialTheme` | 全局 |
| `Text` | `androidx.compose.material3.Text` | 全局 |
| `Icon` | `androidx.compose.material3.Icon` | MpvControls |
| `IconButton` | `androidx.compose.material3.IconButton` | MpvControls |
| `Slider` | `androidx.compose.material3.Slider` | MpvControls, MpvProgressBar |
| `SliderDefaults` | `androidx.compose.material3.SliderDefaults` | MpvControls, MpvProgressBar |
| `OutlinedButton` | `androidx.compose.material3.OutlinedButton` | MpvControls |
| `CircularProgressIndicator` | `androidx.compose.material3.CircularProgressIndicator` | MpvControls |
| `Surface` | `androidx.compose.material3.Surface` | MpvPlayerScreen |

---

## 四、Foundation Layout 组件

| 组件 | Import |
|------|--------|
| `Column`, `Row`, `Box` | `androidx.compose.foundation.layout.*` |
| `Spacer` | `androidx.compose.foundation.layout.Spacer` |
| `fillMaxSize`, `fillMaxWidth` | `androidx.compose.foundation.layout.*` |
| `padding`, `size`, `width`, `height` | `androidx.compose.foundation.layout.*` |
| `background` | `androidx.compose.foundation.background` |
| `clickable` | `androidx.compose.foundation.clickable` |
| `windowInsetsPadding` | `androidx.compose.foundation.layout.windowInsetsPadding` |
| `WindowInsets` / `statusBars` / `navigationBars` | `androidx.compose.foundation.layout.*` |

---

## 五、Animation

```kotlin
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.runtime.getValue  // animateFloatAsState 用 by 也需要这个

val alpha by animateFloatAsState(targetValue = if (visible) 1f else 0f, label = "alpha")
```

---

## 六、Compose Runtime 完整 Import 清单

写任何 Compose 文件时，runtime 包按需选择：

```kotlin
// 基础（几乎所有 Composable 都需要）
import androidx.compose.runtime.Composable

// State + by 委托（用 var x by remember { ... } 时必须全套）
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.mutableFloatStateOf   // 可选，Float 专用
import androidx.compose.runtime.mutableIntStateOf     // 可选，Int 专用
import androidx.compose.runtime.mutableLongStateOf    // 可选，Long 专用

// 记忆
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope

// 副作用
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.snapshotFlow
```

---

## 七、本项目编译通过的"金标准"文件

以下文件已在 CI 中通过 Kotlin 编码，作为 API 用法的权威参照：

| 文件 | 验证过的模式 |
|------|------------|
| `MpvPlayerScreen.kt` | State(by委托+多种类型)、LaunchedEffect、DisposableEffect、Surface、pointerInput |
| `MpvProgressBar.kt` | mutableFloatStateOf+by、Slider+SliderDefaults、pointerInput+detectTapGestures |
