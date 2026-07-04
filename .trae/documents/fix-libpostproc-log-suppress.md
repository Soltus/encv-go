# 修复 libpostproc 缺失 + 抑制构建日志 + 评估 ffmpeg 版本升级

## 问题 1：libpostproc.so MISSING

### 根因
**FFmpeg 7.1.1 中 libpostproc 已被移除**。libpostproc 在 ffmpeg master 分支中于 2023 年 10 月被删除（commit `8c920c4c`），ffmpeg 7.1 是最后一个包含它的发布版本。但我们的 `--disable-everything` 配置实际上也禁用了 postproc，所以即使 7.1 也不会编译出 libpostproc.so。

**关键发现**：我们的构建脚本和 CI 验证步骤都引用了 `libpostproc`，但它从未被编译出来（被 `--disable-everything` 隐式禁用了）。

### 修复
1. **build-ffmpeg-android.sh**：从 `.so` 复制列表和 fftools 链接参数中移除 `libpostproc`
2. **android.yml**：从验证步骤中移除 `libpostproc.so`
3. **ffmpeg_dlopen.go**：确认 Go 代码中没有引用 libpostproc（已确认无引用）

## 问题 2：ffmpeg 构建日志过多

### 修复
1. **x264 configure/make**：输出重定向到日志文件，仅显示摘要
2. **ffmpeg configure**：保留输出（需要看到错误）
3. **ffmpeg make**：输出重定向到日志文件，仅显示摘要
4. **fftools 编译/链接**：输出重定向，仅显示摘要

## 问题 3：是否升级到 FFmpeg 8.x

### 分析

| 维度 | FFmpeg 7.1.1 | FFmpeg 8.1.1 |
|------|-------------|-------------|
| 发布时间 | 2024-09-30 | 2026-03-16 |
| libpostproc | ✅ 存在（但被 disable-everything 禁用） | ❌ 完全移除 |
| API 稳定性 | 成熟稳定 | 大版本升级，有 breaking changes |
| fftools 源码结构 | 可能与 8.x 不同 | 可能有文件增减/重命名 |
| 新功能 | - | Vulkan compute codecs, VVC 改进, D3D12 编码 |
| 我们的需求 | h264/hevc 编解码, ffprobe | 同上 |

### 结论：**暂不升级**

理由：
1. **我们只需要 h264/hevc 编解码和 ffprobe**，7.1.1 完全满足，8.x 的新功能（Vulkan compute, D3D12, VVC）在 Android 移动端用不到
2. **8.x 有 breaking API changes**（移除 libpostproc, 废弃 AVFrame 字段等），ffttools 源码结构可能有变化，需要额外适配工作
3. **7.1.1 是经过验证的稳定版本**，当前构建已通过，没必要冒升级风险
4. **如果未来需要 8.x 的特定功能**（如 VVC 解码），可以单独升级

## 实施步骤

### Step 1: 修复 build-ffmpeg-android.sh
- 从 `.so` 复制列表中移除 `libpostproc`
- 从 fftools 链接参数中移除 `-lpostproc`
- 抑制 x264/ffmpeg 编译日志（重定向到文件）
- 保留关键输出（configure 结果、符号验证、最终大小）

### Step 2: 修复 android.yml
- 从验证步骤中移除 `libpostproc.so`

### Step 3: 验证
- 确认 Go 代码无 libpostproc 引用
- 确认桌面端编译通过
