# Checklist

## 评估结论确认
- [x] 用户已审阅评估结论并确认方向（方案 C: ComboLite 插件化）

## Phase 1: 基础设施

### ComboLite 集成
- [x] 宿主 root build.gradle 已添加 comboLite aar2apk 插件 (classpath 1.1.0)
- [x] comboLite-core 依赖已在 app/build.gradle 声明 (implementation "io.github.lnzz123:combolite-core:2.0.0")
- [x] app/build.gradle 已添加 Compose BOM + compose=true
- [x] 宿主 App 编译配置就绪

### 插件 Module 骨架
- [x] `:plugin-mpv-player` 目录结构创建完成
- [x] build.gradle.kts 配置正确（library plugin, SDK 35, Compose, compileOnly comboLiteCore）
- [x] settings.gradle 已注册 :plugin-mpv-player
- [x] MpvPluginEntry（IPluginEntry 实现）已创建

## Phase 2: Compose UI 重写

### MpvPlayerState + Theme
- [x] PlayerState sealed interface（7 种状态）+ MpvError enum（6 种错误）+ classifyError()
- [x] 深色 Material3 主题 EncvMpVPlayerTheme

### MpvPlayerScreen（主界面）
- [x] 状态机完整迁移（idle/loading/playing/paused/ended/error/audio_only）
- [x] 错误分类逻辑（classifyError）已移植到 Kotlin
- [x] 控制栏自动隐藏定时器（LaunchedEffect + delay(3000)）
- [x] positionUpdate 定时器（每秒轮询）
- [x] DisposableEffect 资源清理（engine.destroy()）

### MpvControls（控制层）
- [x] 视频播放界面完整（TopBar + CenterControls + BottomBar + ProgressBar）
- [x] 音频播放界面完整（AudioCover + AudioBottomSection）
- [x] 错误界面完整（ErrorContent + RetryBtn + BackBtn）
- [x] 加载界面完整（LoadingSpinner）
- [x] 锁定界面完整（LockBar + ProgressBar only）
- [x] 进度条组件完成（Slider + tap seek + 时间格式化 H:MM:SS / M:SS）
- [x] animateFloatAsState 控制栏显隐动画
- [x] 渐变遮罩 (Brush.verticalGradient)

### MpvPlayerActivity（Activity 托管）
- [x] Compose setContent 正确设置（EncvMpVPlayerTheme → Surface → MpvPlayerScreen）
- [x] Intent 参数解析正确（filePath, fileName, mimeType, isExternal）
- [x] 参数协议与 PlayerEntry 发送端一致

## Phase 3: 后端迁移

### Kotlin 层
- [x] MPVLib.kt 已迁入插件模块，包名保持 is.xyz.mpv（JNI 匹配）
- [x] MpvEngine.kt 已创建：独立类（非 LynxModule），20+ 播放控制方法
- [x] Event sealed class（9 种事件）+ PropertyChange sealed class（5 种属性变更）
- [x] listener 回调模式（eventListener / propertyChangeListener / logListener / stateListener）
- [ ] **TODO**: MpvPlayerActivity.createMpvEngine() 占位待实现 — 需构造 MpvEngine(context)
- [ ] **TODO**: MpvPlayerScreen.resolveStreamUrl() 占位待实现 — 需接入 Go 后端流地址解析
- [ ] **TODO**: 全屏切换待实现（Activity 方向切换或 WindowInsetsController）
- [ ] **TODO**: WindowInsetsSafeTop/Bottom 待接入 WindowInsets.systemBars

### JNI/C++ 层
- [x] 所有 .cpp/.h 文件已迁入 plugin-mpv-player/jni/（11 个文件）
- [x] Android.mk / Application.mk 已迁移
- [x] include/ 目录完整迁移（mpv/client.h, libavcodec/jni.h, libswscale/swscale.h）
- [x] overlay 与 app 的 jni 内容完全一致（无需合并）

### 构建脚本
- [x] setup-mpv-libs.sh 输出路径更新为 `plugin-mpv-player/src/main/jniLibs`
- [x] build-player-so.sh JNI_DIR 和 OUTPUT_DIR 更新为插件模块路径
- [ ] ndk-build 编译验证（需 NDK 环境，CI 中执行）

## Phase 4: 宿主适配

### PlayerEntry 调度器
- [x] PlayerEntry 单例已创建（app 模块）
- [x] play() 方法：优先 MPV 插件（PluginManager.createPluginIntent）→ Fallback ArtPlayer（Intent to PlayerActivityCapacitor）
- [x] isMpvAvailable() 方法可用（PluginManager 查询 + 异常安全）
- [x] ArtPlayer fallback 使用 Intent action 启动 Capacitor WebView ArtPlayer

### 路由改造
- [x] PlayerBridgeModule（LynxModule 子类）已创建：playFile() / playFileExternal() / isMpvAvailable()
- [x] PlayerActivity.kt 已改为委托者（onCreate → PlayerEntry.play() → finish()）
- [x] 所有现有 Intent 启动 PlayerActivity 的调用方无需修改（向后兼容）

### 清理
- [x] app/ 旧 MPVLib.kt 标记 @file:Suppress("unused") + 迁移注释
- [x] app/ 旧 MpvPlayerModule.kt 标记迁移
- [x] app/ 旧 PlayerActivityLynx.kt 标记迁移
- [x] jni/ 目录添加 MIGRATED.txt 标记
- [x] Host APK 不包含 MPV .so（jniLibs/ 目录不存在，无 externalNativeBuild 配置）
- [ ] ~~android-overlay~~ （N/A: 项目中无独立 overlay Gradle module）

## 架构一致性
- [x] 无循环依赖（插件 compileOnly vs 宿主 implementation comboliteCore）
- [x] JNI 包名一致（is.xyz.mpv ↔ native 函数签名）
- [x] Intent 参数协议一致（PlayerEntry ↔ MpvPlayerActivity 4 个 key 完全匹配）
- [x] ArtPlayer fallback 路径完备

## Phase 5: 测试与收尾（待 CI 执行）

### 编译验证
- [ ] `:plugin-mpv-player` 编译通过
- [ ] 插件 APK 打包成功（aar2apk 输出）
- [ ] 宿主 `:app` APK 编译成功（无 MPV .so）
- [ ] Host APK 体积对比确认减少 ~25-40 MB

### 功能回归（需真机/模拟器）
- [ ] 视频播放完整流程
- [ ] 音频播放（audio_only）
- [ ] 错误分类显示
- [ ] ArtPlayer fallback 正常
- [ ] 边界情况（插件崩溃隔离等）

## FFmpeg（Go 后端）— 不受影响
- [x] 确认 libffmpeg.so / libffprobe.so 未受影响
- [x] 确认 ffmpeg Runner 接口层未受影响
