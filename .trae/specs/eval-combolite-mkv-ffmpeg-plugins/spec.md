# 评估：使用 ComboLite 将安卓 MPV 播放器重构为可插拔插件

## Why

当前安卓端存在 **两套播放器后端**：
1. **MPV 原生播放器**：C++/JNI/Kotlin/Lynx 全栈，内嵌完整 FFmpeg 解码链，APK 体积贡献大（~30-50MB .so），与宿主 App 强耦合
2. **ArtPlayer Web 播放器**：纯 JS/HTML5 video（[ArtPlayerView.vue](file:///workspace/app/encv-mobile/src/views/ArtPlayerView.vue)），零原生依赖，运行在 Capacitor WebView 中

**核心痛点**：
- MPV 播放器的 10+ 个 .so 文件（libmpv.so + 8个 FFmpeg .so + libplayer.so 等）显著增加 APK 体积
- 用户不一定需要原生播放能力（Web 播放器对多数场景已够用）
- MPV 与 Lynx UI 框架深度耦合，无法按需加载/卸载
- 未来可能引入更多播放器后端（ExoPlayer、系统 MediaPlayer 等），需要统一抽象

**用户明确条件**：
- 只评估 MPV 播放器，不涉及 FFmpeg（Go 后端的 libffmpeg.so 保持现状）
- UI 可以用 Jetpack Compose 重写（解决 Lynx 耦合问题）
- 目标是按需使用 + 体积优化

## ComboLite 是什么

**ComboLite** (github.com/lnzz123/ComboLite) — Android Kotlin 插件化框架：

- 面向 **Jetpack Compose** 的 UI 插件框架
- **0 Hook / 0 反射**，基于官方公开 API（代理模式）
- 插件以 **APK 形式**打包，运行时从 assets 或网络动态加载
- 支持插件自带 **jniLibs (.so)**、资源、Activity/Service 组件
- PluginManager API：安装/卸载/生命周期/崩溃隔离
- **专为 Compose 设计** — 与本项目的 Compose 重写方向完美匹配

---

## 当前架构深度分析

### 一、MPV 播放器组件清单与体积

#### 1.1 原生层（C++/JNI）— 两个构建变体各持一份副本

| 文件 | 行数 | 职责 |
|------|------|------|
| [main.cpp](file:///workspace/app/encv-mobile/android/app/src/main/jni/main.cpp) | ~77 | MPV 创建/销毁/命令，FFmpeg JNI 初始化 |
| [render.cpp](file:///workspace/app/encv-mobile/android/app/src/main/jni/render.cpp) | ~27 | Surface attach/detach |
| [property.cpp](file:///workspace/app/encv-mobile/android/app/src/main/jni/property.cpp) | ~100 | 属性读写/观察 |
| [event.cpp](file:///workspace/app/encv-mobile/android/app/src/main/jni/event.cpp) | ~98 | 事件线程循环→Java 转发 |
| [thumbnail.cpp](file:///workspace/app/encv-mobile/android/app/src/main/jni/thumbnail.cpp) | ~111 | 截图+swscale 缩放 |
| [log.cpp](file:///workspace/app/encv-mobile/android/app/src/main/jni/log.cpp) | ~30 | Logcat 桥接 |
| [jni_utils.cpp/h](file:///workspace/app/encv-mobile/android/app/src/main/jni/jni_utils.cpp) | ~40 | JNI 工具宏 |

**构建产物**: `libplayer.so` (~50-100KB)

#### 1.2 预编译依赖（来自 mpv-android-lib AAR v0.1.12）

通过 [setup-mpv-libs.sh](file:///workspace/app/encv-mobile/scripts/setup-mpv-libs.sh) 从 Maven AAR 解压：

| 库文件 | 估算大小 | 用途 |
|--------|---------|------|
| **libmpv.so** | ~15-25 MB | MPV 播放引擎（静态链接了内部 FFmpeg） |
| libavcodec.so | ~3-5 MB | 解码器（外部可见，被 libmpv 使用） |
| libavformat.so | ~1-2 MB | 解复用 |
| libavutil.so | ~500 KB | 工具函数 |
| libswresample.so | ~200 KB | 音频重采样 |
| libswscale.so | ~300 KB | 视频缩放 |
| libavfilter.so | ~1-2 MB | 音视频滤波 |
| libavdevice.so | ~100 KB | 设备输入输出 |
| libxml2.so | ~200 KB | XML 解析（用于解析等） |
| libc++_shared.so | ~1 MB | C++ 运行时 |

**预估总原生体积: ~25-40 MB**（arm64-v8a 仅）

> 注：精确大小依赖 NDK 编译配置和 strip 程度。mpv-android-lib 的 libmpv.so 本身是体积大户，因为它静态链接了一套完整的 FFmpeg。

#### 1.3 Kotlin 层

| 文件 | 行数 | 职责 |
|------|------|------|
| [MPVLib.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/is/xyz/mpv/MPVLib.kt) | ~183 | 加载 libmpv.so + libplayer.so，JNI 方法声明，观察者模式 |
| [MpvPlayerModule.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt) | ~576 | **LynxModule** 子类，完整播控 API，SurfaceView 管理，事件分发 |
| [PlayerActivityLynx.kt](file:////workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt) | ~426 | AppCompatActivity 托管 LynxView + MPV |
| [PlayerOverlayManager.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerOverlayManager.kt) | ~249 | 悬浮窗模式管理（overlay 变体） |

#### 1.4 Lynx 前端层（将被 Compose 替代）

| 文件 | 行数 | 职责 |
|------|------|------|
| [PlayerApp.tsx](file:///workspace/app/encv-mobile/lynx-player/src/player/PlayerApp.tsx) | ~366 | 主组件：状态机、NativeModules 调用、错误分类 |
| [PlayerControls.tsx](file:///workspace/app/encv-mobile/lynx-player/src/player/PlayerControls.tsx) | ~213 | UI 层：视频/音频/错误/锁定 4 种界面 |
| [ProgressBar.tsx](file:///workspace/app/encv-mobile/lynx-player/src/player/ProgressBar.tsx) | N | 进度条 |
| [index.tsx](file:///workspace/app/encv-mobile/lynx-player/src/player/index.tsx) | 9 | 入口 |

### 二、现有替代播放器：ArtPlayer

项目已有 **ArtPlayer Web 播放器** ([ArtPlayerView.vue](file:///workspace/app/encv-mobile/src/views/ArtPlayerView.vue)，~463 行)：

| 特性 | ArtPlayer | MPV |
|------|-----------|-----|
| 技术栈 | JS/HTML5 `<video>` + artplayer.js 库 | C++/JNI/SurfaceView |
| 原生依赖 | **零** | ~25-40 MB .so |
| 解码能力 | 依赖系统 MediaCodec | 完整 FFmpeg + libass 字幕 |
| 格式支持 | 受限于浏览器/WebView | 极广（MKV/FLV/AVI 等） |
| 字幕支持 | 基础（WebVTT） | 完整（ASS/SSA/srt） |
| 适用场景 | 常见格式快速预览 | 专业播放/复杂容器 |

**结论**：ArtPlayer 已证明"轻量播放器"的可行性。MPV 作为"重量级专业播放器"，适合作为可选插件。

### 三、当前调用链路

```
用户点击播放文件
    ↓
路由判断（Lynx 模式 / Capacitor 模式）
    ↓                        ↓
PlayerActivityLynx      ArtPlayerView.vue (Capacitor WebView)
    ↓                        ↓
LynxView + MpvPlayerModule  Artplayer(JS)
    ↓                        ↓
MPV (libmpv.so)           HTML5 <video>
    ↓
FFmpeg 系列 .so (解码)
    ↓
SurfaceView 渲染
```

---

## ComboLite 插件化方案设计

### 架构目标图

```
┌─────────────────────────────────────────────────────┐
│               Host App (轻量化)                       │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │
│  │ Lynx 主UI │  │ 文件列表  │  │ PlayerEntry       │  │
│  │ (现有)    │  │ (现有)   │  │ (播放器调度器)     │  │
│  └──────────┘  └──────────┘  └──────┬───────────┘  │
│                                      │              │
│                    ┌─────────────────┼─────────┐    │
│                    ▼                 ▼         │    │
│            ┌──────────────┐  ┌────────────┐     │    │
│            │ Plugin APK   │  │ 内置 Web   │     │    │
│            │ "mpv-player" │  │ ArtPlayer  │     │    │
│            ├──────────────┤  └────────────┘     │    │
│            │ Compose UI   │                     │    │
│            │ libplayer.so │                     │    │
│            │ libmpv.so    │                     │    │
│            │ FFmpeg .so ×8│                     │    │
│            └──────────────┘                     │    │
└─────────────────────────────────────────────────────┘
```

### 插件结构设计

```
:plugin-mpv-player (Gradle library module → 打包为 APK)
├── src/main/
│   ├── java/com/encvgo/plugin/mpv/
│   │   ├── MpvPluginEntry.java        // IPluginEntry 实现（ComboLite 入口）
│   │   ├── MpvPlayerActivity.java     // BasePluginActivity 子类（Compose UI）
│   │   ├── MpvPlayerScreen.kt         // Compose 播放界面
│   │   ├── MpvControls.kt             // Compose 控制条
│   │   ├── MpvEngine.kt               // MPV 引擎封装（从 MpvPlayerModule 迁移）
│   │   └── MPVLib.kt                  // JNI 加载（迁移）
│   ├── jni/                           // C++ JNI 层（迁移）
│   │   ├── *.cpp, *.h
│   │   └── Android.mk
│   ├── jniLibs/arm64-v8a/             // 预编译 .so（由 setup-mpv-libs.sh 输出到此）
│   │   ├── libmpv.so
│   │   ├── libplayer.so
│   │   └── lib*.so (FFmpeg 系列)
│   └── assets/                        // 如需 mpv 配置文件
├── build.gradle.kts
│   ├── id("com.android.library")
│   ├── compileOnly(comboLiteCore)
│   ├── implementation(composeBom, composeUi, composeMaterial3)
│   └── implementation(lynxRuntime?) // 如果仍需 Lynx 通信
└── proguard-rules.pro
```

### 宿主侧适配

```kotlin
// Host App: PlayerEntry.kt — 播放器调度器
object PlayerEntry {
    fun play(context: Context, filePath: String, fileName: String, mimeType: String) {
        val pluginManager = PluginManager.getInstance(context)
        val mpvPlugin = pluginManager.getInstalledPlugin("mpv-player")

        when {
            mpvPlugin != null && mpvPlugin.enabled -> {
                // 通过 ComboLite 启动 MPV 插件的 Activity
                val intent = pluginManager.createPluginIntent(
                    mpvPlugin,
                    MpvPlayerActivity::class.java,
                    Bundle().apply {
                        putString("file_path", filePath)
                        putString("file_name", fileName)
                        putString("mime_type", mimeType)
                    }
                )
                context.startActivity(intent)
            }
            else -> {
                // 回退到内置 ArtPlayer (WebView)
                val intent = Intent(context, ArtPlayerActivity::class.java).apply {
                    putExtra("file_path", filePath)
                    putExtra("file_name", fileName)
                }
                context.startActivity(intent)
            }
        }
    }

    fun isMpvAvailable(context: Context): Boolean {
        return PluginManager.getInstance(context)
            .getInstalledPlugin("mpv-player")?.enabled == true
    }
}
```

---

## 评估维度

### 维度 1：技术可行性

| 子项 | 可行性 | 分析 |
|------|--------|------|
| **Compose UI 重写** | ✅ **可行** | 用户已接受此方案。PlayerControls.tsx (~213行) + PlayerApp.tsx 状态机 (~366行) 可映射为 Compose @Composable 函数。Material3 组件可直接替代自定义 LynX view |
| **native .so 打包进插件 APK** | ✅ **完全支持** | ComboLite 明确支持插件 jniLibs。安装时 .so 随 APK 一起解压 |
| **Kotlin/C++ 代码迁移到插件模块** | ✅ **直接** | MpvPlayerModule、MPVLib、PlayerActivityLynx 的逻辑可直接迁移。JNI 代码无需修改 |
| **Activity 托管** | ✅ **BasePluginActivity** | ComboLite 提供 BasePluginActivity，支持代理模式启动，兼容 SurfaceView 渲染 |
| **宿主→插件 Intent 通信** | ✅ **标准 Android** | 文件路径/名称/mimeType 通过 Bundle 传递，无需额外 IPC |
| **插件生命周期** | ✅ **自动管理** | ComboLite 管理 install/start/stop/destroy 完整周期 |
| **崩溃隔离** | ✅ **内置** | MPV 崩溃不会导致宿主崩溃；插件可自动重启或禁用 |
| **缩略图功能 (thumbnail.cpp)** | ✅ 可迁移 | swscale + libavcodec 依赖已在插件 .so 中包含 |
| **两个构建变体统一** | ✅ **自然解决** | app 和 android-overlay 都依赖同一插件 APK，代码重复消除 |
| **Lynx 前端代码处置** | ✅ **可废弃** | lynx-player/ 目录可整体移除（或保留给非插件模式做 fallback） |

**之前的核心阻塞（Lynx 耦合）已被用户接受的 Compose 重写方案解除。**

### 维度 2：体积收益分析

| 场景 | 当前 APK | ComboLite 方案 | 节省 |
|------|----------|---------------|------|
| **用户不需要 MPV**（仅用 ArtPlayer） | 包含全部 ~25-40 MB .so | **不下载/不安装 MPV 插件** | **-25~40 MB** |
| **用户选择安装 MPV** | 同上 | 首次使用时下载插件 APK (~25-40 MB) | APK 安装包减小，但总存储不变 |
| **未来更新 MPV** | 需发版整个 App | 仅更新插件 APK | **大幅减小更新包体积** |
| **多播放器并存** | 所有播放器都打包 | 按需安装 | **线性节省** |

**关键收益**：对于只需要基础播放功能的用户（占大多数），APK 体积可减少 **25-40 MB**。

### 维度 3：架构优势

| 维度 | 当前 | ComboLite 插件化后 |
|------|------|-------------------|
| **耦合度** | MPV 深度嵌入宿主（LynxModule 注册、JNI 加载、布局注入） | 宿主仅依赖 PluginManager API + Intent 协议 |
| **变体维护** | app/ 和 android-overlay/ 各持一份相同代码 | 单一插件源，两变体共享 |
| **扩展性** | 新增播放器需改宿主代码 | 新增播放器 = 新建插件模块 |
| **测试隔离** | MPV 测试需启动完整 App | 插件可独立测试 |
| **发布节奏** | MPV 更新跟随 App 发版 | MPV 插件可独立发版 |
| **崩溃影响** | MPV native crash → App 进程崩溃 | 插件崩溃隔离，宿主存活 |

### 维度 4：成本评估

| 任务 | 工作量 | 说明 |
|------|--------|------|
| Compose UI 重写（替代 Lynx 前端） | **3-5 天** | PlayerControls + PlayerApp + ProgressBar → Compose。含全屏/锁定/音频/错误 4 种状态 |
| 插件 Module 骨架搭建 | **0.5 天** | Gradle 配置、IPluginEntry 实现、AndroidManifest |
| Kotlin 层代码迁移 | **1-2 天** | MpvPlayerModule → MpvEngine + MpvPlayerActivity |
| JNI/C++ 层迁移 | **0.5 天** | 移动 jni/ 目录，更新 Android.mk 路径 |
| 构建脚本适配 | **0.5 天** | setup-mpv-libs.sh / build-player-so.sh 输出到插件模块 |
| 宿主侧适配（PlayerEntry 调度器） | **1 天** | 路由逻辑改造、fallback 到 ArtPlayer |
| 两变体验证（app + overlay） | **1 天** | 确保两种模式下插件正常加载和运行 |
| **合计** | **7.5-11 天** | 约 **2 周** |

对比之前的 4-6 周（含 Lynx 集成方案），因为 Compose 重写路径清晰，工作量更可控。

### 维度 5：风险与缓解

| 风险 | 级别 | 缓解措施 |
|------|------|---------|
| Compose UI 视觉还原度 | 中 | PlayerControls.tsx 的 CSS 样式可 1:1 映射为 Material3 + 自定义 Modifier；可迭代逼近 |
| ComboLite 框架成熟度 | 低-中 | 0 Hook/0 反射，基于官方 API，71 commits 活跃开发；有 sample-plugin 参考 |
| 插件首次加载延迟 | 低 | 首次安装后后续启动无额外开销；可预下载 |
| SurfaceView 在插件 Activity 中渲染 | 低-中 | BasePluginActivity 是真实 Activity，SurfaceView 工作方式与普通 Activity 无异 |
| Go 后端流地址获取 | 无变化 | 插件 Activity 仍可通过 GoBackendModule（或改为 Binder/Intent）获取 |

---

## 与替代方案对比

| 方案 | 体积优化 | 按需加载 | 开发成本 | 维护成本 | 扩展性 |
|------|---------|---------|---------|---------|--------|
| **A: 维持现状** | ❌ 无 | ❌ 无 | 零 | 高（双变体重复） | 低 |
| **B: 提取 Library Module（非插件）** | ❌ 无 | ❌ 无 | 1-2 天 | 中 | 中 |
| **C: ComboLite 插件化（本方案）** | ✅ **-25~40MB** | ✅ 完整 | **2 周** | 低 | **高** |
| **D: 动态下载 .so（非 ComboLite）** | ✅ 部分 | ⚠️ 手动 | 3-5 天 | 高（自定义加载器） | 低 |

**方案 C 是唯一同时解决体积优化 + 按需加载 + 架构解耦的选项。**

---

## 结论与建议

### 最终评估结论

| 评估项 | 评分 (1-5) | 说明 |
|--------|-----------|------|
| 技术可行性 | **4.5** | Compose 重写解除 Lynx 阻塞；其余均为标准 Android 开发 |
| 体积收益 | **5** | 对不需要原生播放的用户可减 25-40 MB APK 体积 |
| 架构改进 | **4.5** | 解耦、消除代码重复、支持多播放器扩展 |
| 成本可控性 | **4** | ~2 周工作量，路径清晰，风险点有限 |
| 长期维护性 | **4** | ComboLite 0 Hook 保证稳定性；插件独立演进 |
| 团队学习成本 | **3** | 需学习 Combo Lite API + Compose（如团队尚不熟悉） |

**综合评分: 4.2 / 5 — 推荐执行**

### 推荐实施路径

```
Phase 1: 基础设施 (2天)
  └→ 创建 :plugin-mpv-player module + ComboLite 集成骨架

Phase 2: Compose UI 重写 (4天)
  └→ PlayerControls + PlayerApp → Compose (@Composable)

Phase 3: 后端迁移 (2天)
  └→ Kotlin + JNI 代码迁入插件模块 + 构建脚本适配

Phase 4: 宿主适配 (2天)
  └→ PlayerEntry 调度器 + ArtPlayer fallback + 双变体验证

Phase 5: 测试与收尾 (2天)
  └→ 功能回归 + 边界情况 + 体积验证
```

### FFmpeg（Go 后端）不受影响

本次重构范围 **严格限定于 MPV 播放器**。Go 后端的 `libffmpeg.so` / `libffprobe.so` 及其 CGO dlopen 加载机制 **完全不改动**。
