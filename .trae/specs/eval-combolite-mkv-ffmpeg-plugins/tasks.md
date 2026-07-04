# Tasks

## Phase 0: 决策点（用户确认）

- [ ] **决策**: 评估结论是否认可？选择哪个方案？
  - **C: ComboLite 插件化（推荐，~2 周）** — MPV 播放器提取为 Compose UI 插件
  - A: 维持现状（零成本）
  - B: 提取 Library Module 但不做插件（1-2 天，无体积优化）
  - 其他方向

## Phase 1: 基础设施 — ComboLite 集成骨架（2 天）

- [ ] Task 1.1: 宿主 App 集成 ComboLite Core
  - [ ] 在 `android/build.gradle` (root) 添加 comboLite aar2apk 插件
  - [ ] 在 `libs.versions.toml` (或 build.gradle) 添加 `combolite-core` 依赖声明
  - [ ] 在 `app/build.gradle` 添加 `implementation(comboLiteCore)` + `packagePlugins` 配置
  - [ ] 在 `android-overlay/.../build.gradle` 同步添加依赖
  - [ ] 验证宿主 App 编译通过

- [ ] Task 1.2: 创建 `:plugin-mpv-player` Gradle module
  - [ ] 在 `app/encv-mobile/` 下新建 `plugin-mpv-player/` 目录
  - [ ] 创建 `build.gradle.kts`（library plugin, compileOnly comboLiteCore, Compose 依赖）
  - [ ] 在宿主 `settings.gradle` 注册 `include(":plugin-mpv-player")`
  - [ ] 创建 `IPluginEntry` 实现类 `MpvPluginEntry`
  - [ ] 创建基础包结构 `com.encvgo.plugin.mpv/`
  - [ ] 配置 aar2apk 声明此模块为插件

## Phase 2: Compose UI 重写（4 天）

- [ ] Task 2.1: Compose 播放主界面 (`MpvPlayerScreen.kt`)
  - [ ] 将 [PlayerApp.tsx](file:///workspace/app/encv-mobile/lynx-player/src/player/PlayerApp.tsx) 的状态机迁移为 Compose State
  - [ ] 状态列表：idle / loading / playing / paused / ended / error / audio_only
  - [ ] 错误分类逻辑（classifyError）移植到 Kotlin
  - [ ] Go 后端流地址获取逻辑（startPlayback）移植
  - [ ] MPV 引擎调用封装（替代 NativeModules.MpvPlayerModule）

- [ ] Task 2.2: Compose 控制层 (`MpvControls.kt`)
  - [ ] 将 [PlayerControls.tsx](file:///workspace/app/encv-mobile/lynx-player/src/player/PlayerControls.tsx) 的 4 种界面模式迁移：
    - 视频播放界面（TopBar + CenterControls + BottomBar + ProgressBar）
    - 音频播放界面（AudioCover + AudioBottomSection）
    - 错误界面（ErrorContent + RetryBtn）
    - 加载界面（LoadingSpinner）
    - 锁定界面（LockBar + ProgressBar only）
  - [ ] 进度条组件 (`MpvProgressBar.kt`) 迁移
  - [ ] 全屏/方向/速度/锁定等交互逻辑移植

- [ ] Task 2.3: Plugin Activity 托管 (`MpvPlayerActivity.kt`)
  - [ ] 继承 `BasePluginActivity`（ComboLite 提供）
  - [ ] 使用 `setContent { MpvPlayerScreen(...) }` 渲染 Compose UI
  - [ ] 从 Intent 解析 filePath / fileName / mimeType / isExternal
  - [ ] 管理 MPV 引擎生命周期（create → init → attachSurface → play → destroy）
  - [ ] 处理 SurfaceView 嵌入 Compose 布局（AndroidView wrapper）

## Phase 3: 后端迁移（2 天）

- [ ] Task 3.1: Kotlin 层迁移
  - [ ] 从 `app/` 和 `android-overlay/` 迁移 `MPVLib.kt` → 插件模块
  - [ ] 从 `MpvPlayerModule.kt` 提取核心逻辑 → `MpvEngine.kt`（去掉 LynxModule 基类，改为普通类）
  - [ ] 保留事件观察者模式（EventObserver），改为回调/LiveData 暴露给 Compose
  - [ ] SurfaceView 管理逻辑迁移到 MpvPlayerActivity

- [ ] Task 3.2: JNI/C++ 层迁移
  - [ ] 从 `app/jni/` 和 `android-overlay/jni/` 移动所有 `.cpp/.h` → `plugin-mpv-player/jni/`
  - [ ] 迁移 `Android.mk`, `Application.mk`
  - [ ] 迁移 `include/` 目录（mpv/client.h 等）
  - [ ] 更新 Android.mk 中 PREBUILT_DIR 路径指向插件模块的 jniLibs

- [ ] Task 3.3: 构建脚本适配
  - [ ] 修改 `setup-mpv-libs.sh`: 输出路径从 `android-overlay/.../jniLibs/` 改为 `plugin-mpv-player/src/main/jniLibs/`
  - [ ] 修改 `build-player-so.sh`: ndk-build 路径指向插件模块的 jni/
  - [ ] 验证 libplayer.so 正确输出到插件 jniLibs/

## Phase 4: 宿主适配（2 天）

- [ ] Task 4.1: PlayerEntry 播放器调度器
  - [ ] 创建 `PlayerEntry.kt` 单例（在 app 模块中）
  - [ ] 实现 `play(context, filePath, fileName, mimeType)` 方法
  - [ ] 优先检测并启动 MPV 插件 Activity
  - [ ] Fallback 到 ArtPlayer (WebView)
  - [ ] 实现 `isMpvAvailable()` 查询方法

- [ ] Task 4.2: 路由改造
  - [ ] 修改 Lynx 主 UI 的播放入口：调用 `PlayerEntry.play()` 替代直接启动 PlayerActivityLynx
  - [ ] 修改 Capacitor 路由：同样走 PlayerEntry
  - [ ] 修改 overlay 入口（PlayerOverlayManager）：走 PlayerEntry
  - [ ] ArtPlayer fallback 路径验证：确保 WebView 播放器正常工作

- [ ] Task 4.3: 清理旧代码
  - [ ] 标记/删除 `app/` 中的 MPV 相关文件（MpvPlayerModule, MPVLib, PlayerActivityLynx 等）
  - [ ] 标记/删除 `android-overlay/` 中的重复 MPV 文件
  - [ ] 决定 `lynx-player/` 目录处置（删除 or 归档）
  - [ ] 确保 Host APK 不再包含 MPV .so 文件（体积验证）

## Phase 5: 测试与收尾（2 天）

- [ ] Task 5.1: 编译验证
  - [ ] `:plugin-mpv-player` 模块独立编译通过
  - [ ] 插件 APK 打包成功（aar2apk 输出）
  - [ ] 宿主 `:app` 编译通过（不包含 MPV .so）
  - [ ] 宿主 `:android-overlay` 编译通过
  - [ ] 最终 Host APK 体积对比（应有 ~25-40 MB 减少）

- [ ] Task 5.2: 功能回归验证
  - [ ] **MPV 插件已安装时**:
    - [ ] 视频文件播放正常（加载→播放→暂停→恢复→seek→全屏→退出）
    - [ ] 音频文件播放正常（audio_only 模式，无 SurfaceView）
    - [ ] 错误处理正常（文件损坏/网络错误/解码失败分类显示）
    - [ ] 缩略图生成正常（如保留此功能）
    - [ ] Go 后端 → 流地址 → MPV 播放完整链路
    - [ ] 锁定界面、倍速、进度条交互正常
  - [ ] **MPV 插件未安装时**:
    - [ ] 自动 fallback 到 ArtPlayer
    - [ ] ArtPlayer 播放常见格式视频正常
    - [ ] 不崩溃、不 ANR
  - [ ] **两个变体**:
    - [ ] app 变体（Capacitor/Lynx 主模式）插件加载正常
    - [ ] android-overlay 变体（悬浮窗模式）插件加载正常

- [ ] Task 5.3: 边界情况
  - [ ] 插件安装中途取消 → 回退到 ArtPlayer
  - [ ] 插件崩溃 → 宿主存活，提示用户
  - [ ] 插件更新 → 无缝替换
  - [ ] 内存不足时插件加载失败 → 优雅降级
