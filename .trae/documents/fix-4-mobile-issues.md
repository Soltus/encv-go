# 修复计划：4 个移动端问题

## 问题 1：关于界面第三方库加载构建信息失败

**根因**：`build-info.json` 由构建脚本生成在 `jniLibs/arm64-v8a/` 目录下，Go 后端通过 `ENCV_LIB_DIR` 环境变量读取。但 `build_info.go` 使用了 `sync.Once`，如果 `ENCV_LIB_DIR` 在首次调用时未设置，后续即使设置了也会返回缓存的错误。此外需要确认 `build-info.json` 是否确实被安装到 APK 的 `jniLibs` 目录中。

**修复步骤**：
1. 检查 `build-info.json` 是否被正确打包进 APK（CI 工作流的 `Verify native libraries` 步骤只检查 `.so` 文件）
2. 修复 `build_info.go`：移除 `sync.Once` 的错误缓存行为，改为每次调用都尝试读取（或在 `ENCV_LIB_DIR` 为空时延迟重试）
3. 确认 `handleBuildInfoGin` 返回 404 时前端正确显示错误提示

## 问题 2：视频打开方式与视频播放方式重复

**根因**：`Settings.vue` 中有"视频播放方式"（`encv_player_video`，选项：Artplayer/mpv/外部播放器），`PluginSettings.vue` 中又有"视频打开方式"（同一个 `encv_player_video`，选项：Artplayer/外部播放器）。两者操作的是同一个 `localStorage` key，但选项不同且 UI 位置重复。

**修复步骤**：
1. 从 `PluginSettings.vue` 中移除"移动端独有设置"区块中的"视频打开方式"（该设置已在 `Settings.vue` 的"播放器"区块中完整覆盖）
2. `PluginSettings.vue` 的"移动端独有设置"区块应只保留真正插件独有的设置，不应重复通用设置

## 问题 3：libffmpeg.so 膨胀到 26.7MB

**根因**：`--whole-archive` 强制链接了所有 FFmpeg `.a` 文件的全部目标文件，即使 `--disable-everything` 只启用了少量编解码器，`--whole-archive` 仍会包含未使用的代码。此外 `--disable-asm` 禁用 NEON 后，C 回退实现代码量更大。

**修复步骤**：
1. 移除 `--whole-archive`，改用普通链接。`--whole-archive` 最初是为了解决"静态库中未被直接引用的符号不会被链接"的问题，但 FFmpeg 的 `--disable-everything` + 精选编解码器模式下，所有需要的符号都会被 fftools 直接或间接引用
2. 如果移除 `--whole-archive` 后出现未定义符号，使用 `--require-definition` 或 `-Wl,--undefined` 精确指定需要的符号，而非全量包含
3. 添加 `--enable-small` 到 FFmpeg configure（优化代码大小而非速度）
4. 链接时添加 `-Wl,--gc-sections` + 编译时添加 `-ffunction-sections -fdata-sections`（启用死代码消除）
5. 使用 `llvm-strip --strip-all` 对最终 .so 进行符号剥离

## 问题 4：加密视频报错 "cannot locate symbol uncompress"

**根因**：`uncompress` 是 zlib (`libz`) 的函数。FFmpeg configure 默认启用 zlib（`--enable-zlib`），`libavformat` 使用 zlib 解压 MOV/MP4 容器中的压缩数据。但链接 fftools .so 时只用了 `-lm -llog`，**缺少 `-lz`**。

`dlopen` 使用 `RTLD_NOW` 标志，会立即解析所有符号。`libffprobe.so` 引用了 `uncompress` 但没有链接 zlib，Android 的 linker namespace 隔离导致运行时找不到 `libz.so`。

**修复步骤**：
1. 在链接 `libffmpeg.so` 和 `libffprobe.so` 的命令中添加 `-lz`（在 `-lm -llog` 后面）
2. 同时检查是否还需要其他系统库（如 `-lc` 标准库通常自动链接，但可能需要 `-landroid` 或其他）
3. 更新项目规则文档，记录 `-lz` 依赖

## 实施顺序

1. **问题 4**（最紧急，阻塞核心功能）：添加 `-lz` 到链接命令
2. **问题 2**（简单 UI 修复）：移除 PluginSettings 中的重复设置
3. **问题 3**（体积优化）：移除 `--whole-archive`，添加 size 优化选项
4. **问题 1**（非阻塞）：修复 build_info.go 的错误缓存问题
