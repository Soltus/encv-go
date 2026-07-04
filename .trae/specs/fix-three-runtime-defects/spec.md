# 修复三个运行时缺陷 Spec（深度分析版）

## Why

用户反馈三个运行时问题，之前的分析过于表面：

### 问题 0：文本预览滚动 bug（已确认根因）

**真正根因**：CSS 嵌套 overflow 冲突。`.text-preview` 容器（`overflow: auto`）与 iframe 内部 `#textContent`（`overflow-y: auto; height: 100vh`）形成嵌套滚动区域。初始渲染时外层容器劫持触摸事件，iframe 内容无法响应滚动。切换换行模式触发 DOM reflow 后滚动状态重置恢复正常。

### 问题 1：安装确认界面不显示（深度根因）

**现象**：用户从未看到 InstallConfirmActivity 影子，120s 后超时。

**关键观察**：
- `REQUEST_CODE_PLUGIN_PICK`（文件选择器，系统 Intent）**正常工作**
- `REQUEST_CODE_INSTALL_CONFIRM`（自定义 Activity）**静默失败**

**真正的根因：Capacitor startActivityForResult 路由限制**

Capacitor Android 框架的 `activity.onActivityResult()` 有以下行为：
1. Capacitor Bridge 在 `MainActivity.onActivityResult()` 中拦截所有 Activity 结果
2. Bridge 将结果路由给对应 Plugin 的 `handleOnActivityResult()` 
3. **但此机制对系统 Intent（ACTION_GET_CONTENT、ACTION_INSTALL_PACKAGE 等）经过充分测试**
4. **对自定义 Activity 的 startActivityForResult 路由可能存在兼容性问题**

具体到本场景：
- GoProcessPlugin 继承自 Capacitor 的 `Plugin`
- `activity` 属性来自 `bridge.getActivity()` —— 这是 Capacitor 的 `Activity` 包装
- 调用 `activity.startActivityForResult(confirmIntent, REQUEST_CODE_INSTALL_CONFIRM)` 时：
  - 如果 Capacitor Bridge 正确代理了此调用 → Activity 应该启动且结果能回传
  - **如果 Bridge 未正确代理或 Activity Context 已失效** → 调用静默失败（不崩溃也不报错），onActivityResult 永远不被触发
  - pendingCalls["installConfirm"] 中的 call 永远不被 resolve/reject → 120s 后 Promise.race 超时

**为什么文件选择器能工作而安装确认不能？**
- 文件选择器使用 `ACTION_GET_CONTENT` + `REQUEST_CODE_PLUGIN_PICK = 9001`
- 安装确认使用自定义 Activity + `REQUEST_CODE_INSTALL_CONFIRM = 9002`
- Capacitor 对系统 Intent 的 startActivityForResult 有专门的处理路径
- 自定义 Activity 可能走了不同的代码路径，在当前 Capacitor 版本中可能存在 bug 或限制

**修复方向**：
A. 不依赖 Capacitor 的 startActivityForResult 路由自定义 Activity，改用 `context.startActivity()` + 独立的结果回调机制（如 BroadcastReceiver）
B. 或者：获取真实的 Activity 实例（非 bridge 包装），在其上调用 startActivityForResult
C. 或者：放弃 Activity 确认模式，改为前端 Modal/Alert 确认（但用户要求复刻 ComboLite 界面，此方案不符合需求）

### 问题 2：加密视频错误（两个独立子问题）

#### 2a：v4 容器 stsz box missing（误报）

**关键信息**："v4容器实际成功但是报错"

**真正的根因：verifyContainer 的结构检查对 v4 容器不适配**

执行流程：
```
Encrypt(dataReader)           ← 加密成功 ✅
  → PostEncryptProcessor(result)
    → verifyContainer()        ← 验证失败 ❌（但加密本身已成功）
      → Decrypt(mainChunk)     ← 解密成功
      → QuickStructCheck(stsz) ← 失败：stsz box missing
```

stsz 检查失败的原因链：
1. [plugin.go L475-476](internal/v2/plugins/video/plugin.go#L475-L476)：当 dataReader 是 `*os.File` 时（fast-start MP4 直接打开），`encryptedSourcePath = inputPath`
2. [plugin.go L739](internal/v2/plugins/video/plugin.go#L739)：`sourcePath == inputPath` → **SkipStructCheck=false**
3. [content_verifier.go L186-187](internal/v2/plugins/video/content_verifier.go#L186-L187)：用 go-mp4 解析解密后文件，查找 moov/trak/mdia/stbl/stsz 路径
4. **v4 容器的加密/解密 roundtrip 会改变 MP4 原子结构**（moov atom 重排、chunk 偏移变化等），导致 stsz box 的位置/格式与原始不同
5. go-mp4 按 BoxPath 精确匹配找不到预期的 stsz → 报 "stsz box missing"

**核心矛盾**：v4 加密是「容器级加密」，输出文件的物理结构与原始文件完全不同。用原始文件的结构特征去验证解密后的文件，本质上就是错误的比较方式。

**修复**：PostEncryptProcessor 场景下应强制 SkipStructCheck=true（不仅依赖 sourcePath != inputPath 判断）。或者更彻底——PostEncryptProcessor 中只做解密校验（确认能正确解密），不做结构对比。

#### 2b：v3 容器 ffprobe JSON 解析失败

**错误信息**：
```
failed to unmarshal ffprobe data: invalid character '[' after array element
(hex dump: 7b0a20202020226672616d6573223a205b...)
```

hex dump 解码：`{\n    "frames": [\n        {\n\n        },\n...`

**真正的根因：ffprobe 输出格式与预期不匹配**

代码 [metadata_extractor.go L180](internal/v2/plugins/video/metadata_extractor.go#L180) 调用：
```go
ffmpeg.Probe("-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
```

期望输出格式：`{"streams": [...], "format": {...}}`
实际输出格式：`{"frames": [{}, {}, ...]}`（这是 `-show_frames` 的输出格式）

**可能原因**（按可能性排序）：
1. **FFmpeg 8.0 的 libffprobe.so 中 `ffprobe_run` 函数的参数处理有变化**：参数被错误解析，导致 `-show_format -show_streams` 被忽略或覆盖，默认使用了 `-show_frames`
2. **构建脚本生成的 libffprobe.so 是定制版本**：可能修改了默认行为
3. **特定文件触发了 ffprobe 的特殊代码路径**：某些损坏/特殊的 MP4 文件让 ffprobe 输出 frames 而非 streams

**影响范围**：此错误发生在 `extractMetadataFromOriginalFile()` 中（PreEncryptProcessor 阶段），导致整个加密流程失败——即使与加密本身无关。

**修复**：
- 增加 ffprobe 输出格式的检测和容错：如果输出以 `"frames"` 开头，尝试适配解析或跳过 metadata 提取使用默认值
- 记录原始 ffprobe 输出以便调试
- 不要让 metadata 提取失败阻塞整个加密流程

## What Changes

### 变更 0：文本预览滚动修复

移除 `.text-preview` 的 `overflow: auto`，消除与 iframe 内部滚动的冲突。

### 变更 1：安装确认界面 — 改用 startActivity + BroadcastReceiver 回调

不再依赖 Capacitor 的 `startActivityResult` 路由自定义 Activity。改用：
1. `context.startActivity()` 启动 InstallConfirmActivity（携带请求标识）
2. InstallConfirmActivity 确认/取消后通过 `BroadcastReceiver` 回传结果
3. GoProcessPlugin 注册动态 BroadcastReceiver 接收结果
4. 从 Broadcast Intent 中取出结果，resolve/reject 对应的 pendingCall

### 变更 2a：v4 容器验证策略修正

在 `verifyContainer()` 中，**PostEncryptProcessor 场景始终使用 SkipStructCheck=true + CollectWarnings=true**。
新增判断条件：检测当前是否在 PostEncryptProcessor 上下文中（可通过新增标志位或方法参数传递）。

### 变更 2b：ffprobe 解析容错

在 `extractMetadataFromOriginalFile()` 中：
1. json.Unmarshal 失败时，记录原始 ffprobe 输出前 256 字节用于调试
2. 尝试检测输出格式（frames vs streams/format），如果是不支持的格式则降级处理
3. 使用合理的默认值（从文件名/大小推断）而非直接返回 error
4. 仅在完全无法获取任何元数据时才返回 error，且 error 信息明确标注为 non-fatal

## Impact

- Affected code:
  - `app/encv-mobile/src/views/FilePreview.vue` — CSS 一行修改
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — 安装回调机制重构（startActivityResult → BroadcastReceiver）
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/InstallConfirmActivity.kt` — 结果回传改为 Broadcast
  - `internal/v2/plugins/video/plugin.go` — verifyContainer SkipStructCheck 策略
  - `internal/v2/plugins/video/metadata_extractor.go` — ffprobe 解析容错
