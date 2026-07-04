# 全面修复 FFmpeg 构建脚本：所有已知问题一次性解决

## 完整审计结果

对 [build-ffmpeg-android.sh](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh) 做逐行审计，发现以下问题：

| # | 严重度 | 问题 | 影响 |
|---|--------|------|------|
| P1 | 🔴 致命 | `--gc-sections` 删除入口符号 | **当前报错：ffprobe_run 缺失** |
| P2 | 🟡 中等 | 无 `-Wl,--version-script` 控制符号暴露 | .so 导出过多内部符号，增大攻击面 |
| P3 | 🟢 低 | bin2c 编译器选择链可能为空 | 极端环境下静默失败 |

---

## P1（致命）：`--gc-sections` 删除入口符号

### 根因

链接器构建共享库时执行可达性分析：
- 从所有**外部未定义引用**出发，标记所有可达的代码/数据
- 删除不可达的部分（死代码消除）

`ffprobe_run` / `ffmpeg_run` 是 `.so` 的**对外 API**（Go 端通过 dlopen + dlsym 调用），但在 `.so 内部**没有任何函数调用它们**。因此链接器认为它们是死代码并删除。

**为什么 `ffmpeg_run` 本次幸存而 `ffprobe_run` 没有？**
偶然。`ffmpeg.c` 内部调用图更复杂（初始化链、回调注册、全局构造），恰好让 `ffmpeg_run` 保持在可达图中。这是**脆弱的隐式依赖**，不是设计保证。

### 修复

两个 `.so` 链接命令都添加 `-Wl,-u,SYMBOL`（`--undefined=SYMBOL`）：

> `-u` 告诉链接器："存在一个外部引用需要 SYMBOL"，因此 SYMBOL 的定义必须保留，不得被 gc-sections 删除。

**libffmpeg.so 链接命令（第 330 行）：**
```diff
 $CC $CFLAGS -shared -o "${FTOOLS_BUILD}/libffmpeg.so" \
     $FFMPEG_OBJS \
     $STATIC_LIBS \
     ${X264_INSTALL}/lib/libx264.a \
     -lm -lz -llog \
+    -Wl,-u,ffmpeg_run \
+    -Wl,-u,ffmpeg_reset \
     -Wl,--gc-sections \
     -Wl,--allow-multiple-definition \
```

**libffprobe.so 链接命令（第 344 行）：**
```diff
 $CC $CFLAGS -shared -o "${FTOOLS_BUILD}/libffprobe.so" \
     $FFPROBE_OBJS \
     $STATIC_LIBS \
     ${X264_INSTALL}/lib/libx264.a \
     -lm -lz -llog \
+    -Wl,-u,ffprobe_run \
+    -Wl,-u,ffprobe_reset \
     -Wl,--gc-sections \
     -Wl,--allow-multiple-definition \
```

---

## P2（中等）：控制符号暴露（安全加固）

当前 `.so` 导出**所有**全局符号（包括 FFmpeg 内部函数、静态辅助函数等）。应只导出必要的公开 API。

在脚本中生成 linker version script：

```bash
cat > "${FTOOLS_BUILD}/ffmpeg.ver" << 'VEOF'
{
  global:
    ffmpeg_run;
    ffmpeg_reset;
  local: *;
};
VEOF

cat > "${FTOOLS_BUILD}/ffprobe.ver" << 'VEOF'
{
  global:
    ffprobe_run;
    ffprobe_reset;
  local: *;
};
VEOF
```

链接命令添加 `-Wl,--version-script,${FTOOLS_BUILD}/ffmpeg.ver`：
```diff
     -Wl,--gc-sections \
     -Wl,--allow-multiple-definition \
+    -Wl,--version-script,"${FTOOLS_BUILD}/ffmpeg.ver" \
```

**效果**：
- 只有 `ffmpeg_run` / `ffmpeg_reset` 对外可见
- 所有 FFmpeg 内部符号（`av_*`, `ff_*`, `init_*` 等）变为 `LOCAL`，不导出
- 减小动态符号表体积，加速 dlopen/dlsym
- 降低逆向工程风险

---

## P3（低）：bin2c 编译器健壮性

第 246 行的编译器选择链：
```bash
$(${TOOLCHAIN}/bin/llvm-gcc 2>/dev/null || which gcc 2>/dev/null || echo "gcc")
```

改进为显式检查：
```bash
BIN2C_CC="$(command -v gcc 2>/dev/null || command -v cc 2>/dev/null || echo "gcc")"
$BIN2C_CC -o "${BUILD_DIR}/bin2c" ffbuild/bin2c.c
```

---

## 修改文件清单

仅修改 **1 个文件**：[`build-ffmpeg-android.sh`](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh)

| 位置 | 修改内容 |
|------|----------|
| 第 246 行附近 | bin2c 编译器选择改用 `command -v` |
| 第 330-337 行 | libffmpeg.so 链接：加 `-u` + version script |
| 第 343-355 行前 | 新增生成 `.ver` 文件 |
| 第 344-351 行 | libffprobe.so 链接：加 `-u` + version script |
