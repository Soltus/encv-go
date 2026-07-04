# KuiklyUI 切换评估 vs 当前 Compose 修复

## 一、当前问题本质分析

CI 报错的 **3 个错误** 全部是编码疏忽，不是框架缺陷：

| # | 错误 | 根因 | 修复量 |
|---|------|------|--------|
| 1 | L266 `Unresolved reference 'outlined'` | 大小写：`Icons.outlined` → `Icons.Outlined` | 改 2 字符 × 2 处 |
| 2 | L273 同上 | 同上 | 同上 |
| 3 | L303 `MutableState<Float> has no method 'setValue'` | `by` 委托不适用于 `mutableStateOf<Float>`（Kotlin 2.1.0 + Compose BOM 2024.06.00） | 改为显式 `.value` 模式，约 6 行 |

**结论：这是 AI 编码错误，不是 Compose 框架问题。**

---

## 二、KuiklyUI 切换评估

### 2.1 Kuikly 是什么

腾讯开源的**跨平台 UI 框架**（基于 Kotlin Multiplatform），支持 Android/iOS/鸿蒙/H5/小程序/macOS。提供两种范式：
- **Kuikly DSL**：自研声明式 DSL
- **Compose DSL**：基于修改版 Jetpack Compose 1.7.3（包名从 `androidx.compose.*` 改为 `com.tencent.kuikly.compose.*`）

### 2.2 架构兼容性分析

| 维度 | 当前架构 | Kuikly 要求 | 兼容性 |
|------|---------|------------|--------|
| **模块类型** | `com.android.library`（标准 Android Library） | 需要 KMP `commonMain` + 多平台 sourceSet | ❌ 不兼容 |
| **构建插件** | AGP + Kotlin Android + compose compiler | KSP 注解处理器 + Kuikly Gradle 插件 | ❌ 需大改 |
| **页面入口** | `Activity` / `@Composable function` | `ComposeContainer` + `@Page("name")` 注解 | ❌ 不兼容 |
| **包名依赖** | `androidx.compose.*`（标准） | `com.tencent.kuikly.compose.*`（除 runtime 外全部替换） | ❌ 全量替换 |
| **运行时** | Capacitor 加载独立 APK | 需要 Kuikly Core 引擎初始化 | ❌ 需集成 |
| **图标系统** | `material-icons-extended` 的 `Icon` 组件 | **不支持 `Icon` 组件**，需用 `Image` 替代 | ❌ 需重写 |
| **目标平台** | 仅 Android | 强制多平台（commonMain） | ⚠️ 过度工程 |

### 2.3 API 兼容性清单（MpvControls.kt 用到的 API）

| 当前使用的 Compose API | Kuikly 状态 | 影响 |
|-----------------------|------------|------|
| `Icon` + `Icons.Outlined.XXX` | **❌ 不支持 Icon** | 音量/字幕/音轨/全屏等 8 个按钮全要改为 Image |
| `IconButton` | ✅ 可用 | — |
| `Slider` / `SliderDefaults` | ✅ 可用 | — |
| `MaterialTheme` (colorScheme) | ✅ 可用 | — |
| `CircularProgressIndicator` | ✅ 可用 | — |
| `OutlinedButton` | **❌ 不支持** | SpeedChip、ErrorLayout 重试/返回按钮需改用 Button |
| `Text` + `fontWeight/fontSize` | ✅ 可用 | — |
| `Modifier.clickable` | ✅ 可用 | — |
| `animateFloatAsState` | 🚧 淡入淡出可能不生效 | LoadingLayout 动画 |
| `Brush.verticalGradient` | ✅ 可用 | — |
| `windowInsetsPadding` | ✅ 可用 | — |
| `remember { mutableStateOf<T>() }` | ✅ 可用（runtime 未改） | 但仍不能用 `by` 委托 |

### 2.4 切换工作量估算

| 任务 | 文件数 | 预估改动量 |
|------|--------|-----------|
| build.gradle.kts 改为 KMP + KSP + Kuikly 依赖 | 1 | 重写 |
| 所有 .kt 文件 import 替换（`androidx.*` → `kuikly.compose.*`） | 8 | 全文件 |
| `Icon` 组件 → `Image` + painterResource 重写 | 3 | ~20处 |
| `OutlinedButton` → `Button` 自定义样式 | 2 | ~4处 |
| Activity 入口 → ComposeContainer + @Page | 1 | 重构入口 |
| Kuikly Core 引擎集成到 Capacitor 宿主 | 2-3 | 新代码 |
| CI 构建流程适配（KMP 编译链） | 1-2 | 新流程 |
| **合计** | **~15+ 文件** | **大量重写** |

对比：**修复当前 3 个错误 = 改 1 个文件约 10 行**

### 2.5 切换风险

1. **高集成风险**：Kuikly 需要在 Capacitor Android 宿主中初始化 Core 引擎，与现有 Plugin 加载机制冲突
2. **API 缩减**：MpvControls 用到的 `Icon`、`OutlinedButton` 在 Kuikly 中不可用或需变通
3. **生态隔离**：无法使用标准 Compose 生态（Material3、icons-extended 示例代码均不可直接参考）
4. **维护负担**：引入腾讯内部版本 Compose（包名已改），升级路径依赖腾讯
5. **仅 Android 收益为零**：跨平台能力对 MPV 播放器插件无意义（iOS 用原生 AVPlayer）

---

## 三、推荐方案

### 方案 A（推荐）：修复当前 3 个 Compose 错误

**理由**：
- 错误本质是 AI 编码疏忽，不是框架局限
- 修复量极小（~10 行改动）
- 零架构风险
- 项目已有大量正常工作的 Compose 代码（MpvPlayerScreen.kt、MpvProgressBar.kt 等均已编译通过）

**具体修复**：
1. `Icons.outlined` → `Icons.Outlined`（import L32-33 + 引用 L266/L273）
2. `var sliderVolume by remember { mutableStateOf(volume) }` → 显式 `.value` 模式（L303 + 5 处引用）

### 方案 B：切换到 Kuikly

**仅在以下条件同时满足时考虑**：
- 未来需要 MPV 播放器的 iOS/鸿蒙版本
- 接受 2-3 周的完整重写周期
- 接受 Kuikly API 子集限制（无 Icon、无 OutlinedTextField 等）

---

## 四、执行计划（方案 A）

### Step 1: 修正 Icons.Outlined 大小写（4 处）
### Step 2: 修正 mutableStateOf by 委托（1 处声明 + 5 处引用）
### Step 3: 清理日志文件（job_logs.zip + job_logs_extracted/）
