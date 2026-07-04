# 升级 FFmpeg 到 8.0 构建计划

## 背景

当前构建脚本使用 FFmpeg 7.1.1，编译 fftools 时遇到多个兼容性问题（`<stdbit.h>` C23 头文件、`struct tm` 前向声明、`ffmpeg_reset` 重复定义、`ff_log2_tab` 重复符号）。与其逐一修补 7.1.1 的兼容性问题，不如直接升级到 FFmpeg 8.0，从根本上解决这些问题。

## FFmpeg 8.0 关键变化（与本项目相关）

### 1. libpostproc 已完全移除
- `--enable-postproc` 和 `-lpostproc` 不再有效
- 静态链接列表中不能包含 `libpostproc.a`

### 2. fftools 源码结构变化
- FFmpeg 8.0 的 fftools 目录引入了 `textformat/` 子目录（用于 ffprobe 输出格式化）
- 新增文件：`fftools/textformat/` 目录下的多个 `.c` 文件
- `ffprobe.c` 依赖 `textformat` 模块
- `ffmpeg.c` 的 `main()` 函数签名和内部结构可能有变化

### 3. C11 编译器要求
- FFmpeg 8.0 要求 C11 兼容编译器
- 不再使用 C23 的 `<stdbit.h>`（8.0 已正确处理条件编译）
- NDK r26 的 Clang 17 支持 C11

### 4. YASM 已移除，强制使用 NASM
- 不影响本项目（Android ARM64 不使用 x86 汇编）

### 5. API breaking changes
- 本项目通过 dlopen 调用 fftools 的 `ffmpeg_run()`/`ffprobe_run()` 函数，不直接使用 libav* API
- 因此 API breaking changes 不影响本项目

## 实施步骤

### Step 1：更新版本号和下载 URL

修改 `build-ffmpeg-android.sh` 第 4 行：
```bash
FFMPEG_VERSION="8.0"
```

### Step 2：移除 libpostproc 相关内容

1. 从 `STATIC_LIBS` 循环中移除 `libpostproc`：
   ```bash
   for lib in libavformat libavcodec libavutil libswresample libswscale libavfilter libavdevice; do
   ```

2. 从 CFLAGS 的 `-I` 路径中移除 `-I${FFMPEG_SRC}/libpostproc`

3. 从 ffmpeg configure 选项中移除 `--enable-postproc`（如果有的话）

### Step 3：更新 fftools 源文件列表

FFmpeg 8.0 的 fftools 文件列表需要更新。关键变化：

**FFMPEG_FFTOOLS**（需要根据 8.0 源码确认完整列表，以下是预期变化）：
- 保留：`ffmpeg.c`, `ffmpeg_dec.c`, `ffmpeg_demux.c`, `ffmpeg_enc.c`, `ffmpeg_filter.c`, `ffmpeg_hw.c`, `ffmpeg_mux.c`, `ffmpeg_opt.c`, `cmdutils.c`, `opt_common.c`, `sync_queue.c`, `thread_queue.c`
- 可能新增：`ffmpeg_mux_init.c`

**FFPROBE_FFTOOLS**：
- 保留：`ffprobe.c`, `cmdutils.c`, `opt_common.c`
- 新增 textformat 模块文件（需要确认 8.0 的 ffprobe 依赖）

**策略**：改为动态检测 fftools 目录下的 `.c` 文件，而非硬编码列表：
```bash
# 动态收集 fftools 源文件（排除不需要的）
FFMPEG_FFTOOLS=""
for f in fftools/ffmpeg*.c fftools/cmdutils.c fftools/opt_common.c fftools/sync_queue.c fftools/thread_queue.c; do
    [ -f "${FFMPEG_SRC}/${f}" ] && FFMPEG_FFTOOLS="$FFMPEG_FFTOOLS $f"
done

FFPROBE_FFTOOLS=""
for f in fftools/ffprobe.c fftools/cmdutils.c fftools/opt_common.c; do
    [ -f "${FFMPEG_SRC}/${f}" ] && FFPROBE_FFTOOLS="$FFPROBE_FFTOOLS $f"
done
# 加上 textformat 子目录
for f in ${FFMPEG_SRC}/fftools/textformat/*.c; do
    [ -f "$f" ] && FFPROBE_FFTOOLS="$FFPROBE_FFTOOLS fftools/textformat/$(basename $f)"
done
```

### Step 4：修复 main() → ffmpeg_run()/ffprobe_run() 补丁

FFmpeg 8.0 的 `main()` 函数签名可能不同。补丁逻辑需要更健壮：

```bash
echo "=== Patching ffmpeg source ==="
cd "${BUILD_DIR}/ffmpeg-${FFMPEG_VERSION}"

# 使用更通用的 sed 模式匹配 main 函数定义
sed -i 's/^int main(/int ffmpeg_run(/' fftools/ffmpeg.c
sed -i 's/^int main(void)/int ffmpeg_run(void)/' fftools/ffmpeg.c
# 也匹配 wmain 等变体
sed -i 's/^int wmain(/int ffmpeg_run(/' fftools/ffmpeg.c

sed -i 's/^int main(/int ffprobe_run(/' fftools/ffprobe.c
sed -i 's/^int main(void)/int ffprobe_run(void)/' fftools/ffprobe.c

# 仅在函数未定义时追加 reset 函数
if ! grep -q "void ffmpeg_reset" fftools/ffmpeg.c; then
    cat >> fftools/ffmpeg.c << 'PATCH'

void ffmpeg_reset(void) {
}
PATCH
fi

if ! grep -q "void ffprobe_reset" fftools/ffprobe.c; then
    cat >> fftools/ffprobe.c << 'PATCH'

void ffprobe_reset(void) {
}
PATCH
fi
```

### Step 5：修复 CFLAGS

```bash
CFLAGS="-std=c11 -fPIC -DANDROID -D_POSIX_C_SOURCE=200809L -include time.h \
  -I${FFMPEG_INSTALL}/include \
  -I${FFMPEG_SRC} \
  -I${FFMPEG_SRC}/fftools \
  -I${FFMPEG_SRC}/fftools/textformat \
  -I${FFMPEG_SRC}/libavcodec \
  -I${FFMPEG_SRC}/libavformat \
  -I${FFMPEG_SRC}/libavutil \
  -I${FFMPEG_SRC}/libavfilter \
  -I${FFMPEG_SRC}/libswresample \
  -I${FFMPEG_SRC}/libswscale \
  -I${FFMPEG_SRC}/libavdevice \
  -I${X264_INSTALL}/include"
```

关键变更：
- 添加 `-std=c11`：FFmpeg 8.0 要求 C11
- 添加 `-D_POSIX_C_SOURCE=200809L`：确保 POSIX 函数可用
- 添加 `-include time.h`：解决 `struct tm` 前向声明问题
- 添加 `-I${FFMPEG_SRC}/fftools/textformat`：FFmpeg 8.0 新增的 textformat 目录
- 移除 `-I${FFMPEG_SRC}/libpostproc`：已移除

### Step 6：修复链接 — 允许多重定义

FFmpeg 的多个静态库包含相同的 `ff_log2_tab` 等符号。添加链接器标志：

```bash
-Wl,--allow-multiple-definition
```

### Step 7：更新 ffmpeg configure 选项

移除不再有效的选项，确保与 8.0 兼容：

```bash
./configure \
    --prefix="${BUILD_DIR}/ffmpeg-install" \
    --enable-cross-compile \
    --cross-prefix="${TOOLCHAIN}/bin/llvm-" \
    --cc="$CC" \
    --ar="$AR" \
    --nm="$NM" \
    --ranlib="$RANLIB" \
    --strip="$STRIP" \
    --arch=${ARCH} \
    --target-os=android \
    --sysroot="${TOOLCHAIN}/sysroot" \
    --enable-shared \
    --enable-static \
    --disable-programs \
    --disable-doc \
    --disable-htmlpages \
    --disable-manpages \
    --disable-podpages \
    --disable-txtpages \
    --disable-everything \
    --enable-decoder=h264,hevc,aac,mp3,opus,vorbis,flac,pcm_s16le,pcm_s24le,pcm_s32le,aac_latm \
    --enable-encoder=aac,pcm_s16le,pcm_s24le,pcm_s32le \
    --enable-muxer=mp4,matroska,flac,mp3,adts,null \
    --enable-demuxer=mov,matroska,aac,mp3,flac,ogg,wav \
    --enable-parser=h264,hevc,aac,aac_latm,mpegaudio,opus,vorbis \
    --enable-protocol=file,pipe \
    --enable-filter=aresample \
    --enable-libx264 \
    --enable-gpl \
    --pkg-config="${BUILD_DIR}/pkg-config-wrapper" \
    --extra-cflags="-fPIC -DANDROID -I${X264_INSTALL}/include" \
    --extra-ldflags="-L${X264_INSTALL}/lib -lm" \
    --extra-libs="-lm"
```

注意：
- 移除 `--enable-postproc`（8.0 已移除 libpostproc）
- 移除 `--enable-encoder=h264`（8.0 中 h264 编码器需要 libx264，已通过 `--enable-libx264` 启用）
- h264 编码通过 libx264 的包装器提供，编码器名称为 `libx264`

### Step 8：更新缓存验证

缓存验证逻辑需要更新，确保检测到的是 8.0 版本的库：

```bash
if [ -f "${OUTPUT_DIR}/libffmpeg.so" ] && [ -f "${OUTPUT_DIR}/libffprobe.so" ]; then
    echo "✅ ffmpeg output already exists, checking symbols..."
    ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_run" && \
    ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_run" && {
        if ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "av_log"; then
            echo "✅ libffprobe.so contains FFmpeg symbols (static-linked)"
            echo "✅ All ffmpeg libraries cached and valid, skipping build"
            exit 0
        else
            echo "⚠️  libffprobe.so missing FFmpeg symbols (not static-linked), rebuilding..."
            rm -f "${OUTPUT_DIR}/libffmpeg.so" "${OUTPUT_DIR}/libffprobe.so"
        fi
    }
    echo "⚠️  Cached libraries missing expected symbols, rebuilding..."
fi
```

### Step 9：更新项目规则文档

更新 `.trae/rules/project_rules.md` 中的 FFmpeg 版本备注：
- 当前版本改为 8.0
- 移除"暂不升级到 8.x"的说明
- 更新升级注意事项

## 验证清单

1. `./configure` 成功完成，无错误
2. `make` 成功编译所有库
3. fftools 编译成功（无 `<stdbit.h>` 错误、无 `struct tm` 错误）
4. 链接成功（无 `ff_log2_tab` 重复符号错误）
5. `nm -D libffprobe.so | grep av_log` — 确认 FFmpeg 符号已静态链接
6. `nm -D libffprobe.so | grep ffprobe_run` — 确认入口函数存在
7. `readelf -d libffprobe.so | grep NEEDED` — 无 `libavutil.so` 等动态依赖
8. Go 后端 `go build ./internal/...` 编译通过
