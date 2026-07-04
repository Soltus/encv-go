# 修复 FFmpeg fftools 编译和链接错误

## 错误分析

构建日志暴露了 4 个问题：

### 问题 1：`ffmpeg.c` 重复定义 `ffmpeg_reset`

```
fftools/ffmpeg.c:1018:6: note: previous definition is here
void ffmpeg_reset(void) {
```

**根因**：补丁脚本第 102-106 行用 `cat >> fftools/ffmpeg.c` 追加了 `void ffmpeg_reset(void) {}`，但 FFmpeg 7.1.1 的 `ffmpeg.c` 内部已经定义了 `ffmpeg_reset()` 函数（第 1018 行）。追加导致重复定义。

**修复**：不再追加 `ffmpeg_reset`，因为 FFmpeg 7.1.1 已经内置了该函数。同理检查 `ffprobe_reset` 是否也已内置。

### 问题 2：`ffmpeg_dec.c` 找不到 `<stdbit.h>`

```
fatal error: 'stdbit.h' file not found
#include <stdbit.h>
```

**根因**：`<stdbit.h>` 是 C23 标准头文件，FFmpeg 7.1.1 使用了它，但 NDK r26 的 Clang 17 不完全支持 C23。这是 FFmpeg 7.1.1 与 NDK r26 的兼容性问题。

**修复**：添加 `-std=c11 -D_POSIX_C_SOURCE=200809L` 到 CFLAGS，强制使用 C11 标准。但 `<stdbit.h>` 是 C23 专有头文件，C11 模式下不会包含它。需要检查 FFmpeg 7.1.1 是否有条件编译守卫。实际上 FFmpeg 7.1.1 应该有 `#if __STDC_VERSION__ >= 202311L` 守卫，所以设置 `-std=c11` 就能绕过。

### 问题 3：`ffmpeg_opt.c` 和 `opt_common.c` 的 `struct tm` 前向声明错误

```
note: forward declaration of 'struct tm'
```

**根因**：`<time.h>` 未被包含，导致 `struct tm` 未定义。NDK 的头文件在严格模式下可能不自动包含 `<time.h>`。

**修复**：添加 `-include time.h` 到 CFLAGS，强制在所有编译单元前包含 `<time.h>`。

### 问题 4：链接时 `ff_log2_tab` 重复符号

```
ld.lld: error: duplicate symbol: ff_log2_tab
>>> defined at log2_tab.c:23 (./libavutil/log2_tab.c:23)
>>>            log2_tab.o:(ff_log2_tab) in archive libavformat.a
>>>            log2_tab.o:(.rodata+0x0) in archive libswscale.a
```

**根因**：FFmpeg 的 `log2_tab.c` 被多个库（libavformat、libswscale、libavfilter 等）各自编译了一份，每个 `.a` 都包含 `ff_log2_tab` 符号。使用 `--whole-archive` 时，链接器看到多个定义就报错。

**修复**：将 `--whole-archive` 改为普通链接（不用 `--whole-archive`），或者添加 `-Wl,--allow-multiple-definition` 链接器标志。推荐使用 `--allow-multiple-definition`，因为 FFmpeg 内部保证这些重复符号的值相同。

---

## 实施步骤

### Step 1：修复 `ffmpeg_reset` 重复定义

修改补丁逻辑（第 93-112 行）：

```bash
echo "=== Patching ffmpeg source ==="
cd "${BUILD_DIR}/ffmpeg-${FFMPEG_VERSION}"

# 重命名 main 函数
sed -i 's/^int main(/int ffmpeg_run(/' fftools/ffmpeg.c
sed -i 's/^int main(void)/int ffmpeg_run(void)/' fftools/ffmpeg.c

sed -i 's/^int main(/int ffprobe_run(/' fftools/ffprobe.c
sed -i 's/^int main(void)/int ffprobe_run(void)/' fftools/ffprobe.c

# 仅在 ffmpeg_reset 未定义时追加（FFmpeg 7.1.1 已内置）
if ! grep -q "void ffmpeg_reset" fftools/ffmpeg.c; then
    cat >> fftools/ffmpeg.c << 'PATCH'

void ffmpeg_reset(void) {
}
PATCH
fi

# 仅在 ffprobe_reset 未定义时追加
if ! grep -q "void ffprobe_reset" fftools/ffprobe.c; then
    cat >> fftools/ffprobe.c << 'PATCH'

void ffprobe_reset(void) {
}
PATCH
fi
```

### Step 2：修复 CFLAGS — 添加 C11 标准和 time.h

修改 CFLAGS（第 192 行）：

```bash
CFLAGS="-std=c11 -fPIC -DANDROID -D_POSIX_C_SOURCE=200809L -include time.h -I${FFMPEG_INSTALL}/include -I${FFMPEG_SRC} -I${FFMPEG_SRC}/fftools -I${FFMPEG_SRC}/libavcodec -I${FFMPEG_SRC}/libavformat -I${FFMPEG_SRC}/libavutil -I${FFMPEG_SRC}/libavfilter -I${FFMPEG_SRC}/libswresample -I${FFMPEG_SRC}/libswscale -I${FFMPEG_SRC}/libavdevice -I${FFMPEG_SRC}/libpostproc -I${X264_INSTALL}/include"
```

关键变更：
- 添加 `-std=c11`：强制 C11 模式，避免 C23 的 `<stdbit.h>` 被包含
- 添加 `-D_POSIX_C_SOURCE=200809L`：确保 POSIX 函数可用
- 添加 `-include time.h`：强制包含 `<time.h>`，解决 `struct tm` 前向声明问题

### Step 3：修复链接 — 允许多重定义

修改链接命令（第 224-235 行和第 255-266 行），添加 `-Wl,--allow-multiple-definition`：

```bash
echo "Linking libffmpeg.so..."
$CC $CFLAGS -shared -o "${FTOOLS_BUILD}/libffmpeg.so" \
    $FFMPEG_OBJS \
    -Wl,--whole-archive \
    $STATIC_LIBS \
    -Wl,--no-whole-archive \
    ${X264_INSTALL}/lib/libx264.a \
    -lm -llog \
    -Wl,--allow-multiple-definition \
    $LDFLAGS > "${LOG_DIR}/link_ffmpeg.log" 2>&1 || {
    ...
}
```

同理修改 `libffprobe.so` 的链接命令。

### Step 4：验证

构建完成后检查：
1. `nm -D libffprobe.so | grep av_log` — 确认 FFmpeg 符号已静态链接
2. `nm -D libffprobe.so | grep ffprobe_run` — 确认入口函数存在
3. 无 `NEEDED` 条目指向 `libavutil.so` 等 — 确认无动态依赖
