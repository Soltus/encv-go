# 修复播放器样式异常 + 播放无响应（v3 — 基于 error 级别日志完整分析）

## Error 级别日志完整时间线与根因分析

### 时间线重建（从应用启动到日志结束，共 ~16 秒）

```
时间        事件                                                        性质
─────────────────────────────────────────────────────────────────────────────
19:19:27.98  应用进程启动 (com.encvgo.app)                                系统事件
19:19:28.35  DevToolLifecycle.nativeSyncStateToNative No implementation   ⚠️ Release包缺devtool.so
19:19:28.77  libEGL eglQueryStringImpl display is EGL_NO_DISPLAY          ⚠️ EGL未初始化(Surface还没创建)
19:19:30.70  GPUAUX Null anb #1                                          ⚠️ ANB缓冲区为空
19:19:31.08  GPUAUX Null anb #2
19:19:33.31  GPUAUX Null anb #3,#4,#5                                   ← Lynx尚未加载
19:19:34.38  GPUAUX Null anb #6,#7
19:19:35.25  js_cache_manager: meta.json file_path is empty              ❌ Lynx缓存未配置
19:19:35.34  lynx_plugin: load original lynx so                          ✅ Lynx引擎开始加载
19:19:35.39  timing_map: duplicated timing_key ×4                        ⚠️ 时序重复(非致命)
19:19:36.47  lynx_inspector_owner_native_glue: ptr is null #1            ❌ Inspector指针为空
19:19:36.54  ★ DestroyLayoutNode #1: raw-text / text / view / view      ❌ 首次全量销毁
19:19:36.60  DestroyLayoutNode #2: view                                  ❌
19:19:39.23  ptr is null
19:19:39.31  ★ DestroyLayoutNode #3: raw-text / text                    ❌ (+2.8s)
19:19:39.90  ptr is null
19:19:39.97  ptr is null
19:19:40.44  ★ DestroyLayoutNode #4: raw-text / text                    ❌ (+1.1s)
19:19:40.77  ptr is null
19:19:40.82  ptr is null
19:19:41.15  ★ DestroyLayoutNode #5: raw-text / text                    ❌ (+0.7s)
19:19:41.44  ptr is null ×2
19:19:41.51  ptr is null ×2
19:19:41.94  ★ DestroyLayoutNode #6: raw-text / text                    ❌ (+0.8s)
19:19:42.48  ptr is null
19:19:42.53  ptr is null
19:19:43.36  RecyclerView: No adapter attached; skipping layout           ❌ 最终状态
```

### 关键发现

#### 发现 A：`ptr is null` 与 `DestroyLayoutNode` 强关联（共 24 次 ptr null + 6 次 DestroyLayout）

`lynx_inspector_owner_native_glue.cc(53): ptr is null` 出现 **24 次**，每次都紧邻 `DestroyLayoutNodeBeforeRemoveFromParent`。这是 **Lynx DevTools Inspector** 的空指针 — Release 包不含 devtool.so（L5 已确认），Inspector 初始化失败导致 ptr=null，后续每次布局变更都触发空指针错误。

**但这不是崩溃原因**，只是副作用。

#### 发现 B：DestroyLayoutNode 从"全量"退化为"仅文本"

| 次数 | 销毁的 tag | 间隔 |
|------|-----------|------|
| #1 (19:19:36.54) | raw-text, text, **view**, **view** | — |
| #2 (19:19:36.60) | **view** | 0.06s |
| #3 (19:19:39.31) | raw-text, text | 2.8s |
| #4 (19:19:40.44) | raw-text, text | 1.1s |
| #5 (19:19:41.15) | raw-text, text | 0.7s |
| #6 (19:19:41.94) | raw-text, text | 0.8s |

**第一次销毁了含 view 的完整子树**，后续 **只销毁文本节点**（raw-text、text）。这意味着：
- view 容器本身在第一次后就稳定了
- 但容器内的 **文本内容反复被销毁重建**

这强烈暗示是 **CSS 文本相关属性不兼容**触发的（如 `text-overflow: ellipsis`、`font-size` 单位、`line-height` 等）。

#### 发现 C：RecyclerView 无适配器（最终状态）

```
19:19:43.36  RecyclerView: No adapter attached; skipping layout
```
这是日志的最后一条。Lynx 内部使用 RecyclerView 做列表渲染，No adapter 说明 **LynxView 的模板渲染可能未完全完成或被中断**。

#### 发现 D：零条 MPV/PlayerActivity/GoBackend 日志

整个 error 级别日志中 **完全没有自定义模块的错误**。两种解释：
1. 这些模块确实没有报 error（操作可能成功了但前端没反应？）
2. 或代码路径根本没执行到（initData 为空导致提前 return？）

#### 发现 E：EGL_NO_DISPLAY + GPUAUX Null anb（Surface 问题）

```
19:19:28.77  EGL: display is EGL_NO_DISPLAY    ← Surface 创建前
19:19:30-34  GPUAUX: Null anb ×7               ← ANB(Android Native Buffer)为空
```
这两个在 Lynx 加载前就出现。**EGL_NO_DISPLAY** 意味着 OpenGL ES 显示未初始化 — 这可能是 MPV SurfaceView 无法正常工作的原因（mpv 需要 GL Surface 做视频渲染）。但如果 Surface 有问题，mpv initialize 应该会报错... 除非 mpv 根本没被调用。

---

## 修复方案（基于上述发现）

### Step 1：重写 App.css — 消除触发文本节点反复重建的 CSS 属性

**目标**：消除 `DestroyLayoutNodeBeforeRemoveFromParent tag:raw-text/tag:text` 反复触发

**文件**：`app/encv-mobile/lynx-player/src/App.css`

删除/替换以下属性（按可疑度排序）：

| 优先级 | 属性 | 行号 | 原因 |
|--------|------|------|------|
| P0 | `position: absolute` | L129-133 | SliderThumb — Lynx 不支持，导致布局计算异常 |
| P0 | `overflow: hidden` | L109 | SliderTrackOuter — 不支持，影响子元素裁剪逻辑 |
| P1 | `gap: 8px` | L140 | LoadingDots — 可能不支持，改 margin |
| P1 | `text-overflow: ellipsis` | L31-32 | TitleText — 可能不支持，改用 text-maxline |
| P2 | `border-radius: 36px` | L53 | PlayButtonCircle — 大值可能不稳定 |

进度条改为无 thumb 设计（纯宽度填充条），彻底消除 absolute 定位依赖。

### Step 2：前端全链路可观测性

**问题**：当前点击 ▶ 后如果出错，用户看不到任何反馈（error 状态只显示文字且可能因为布局抖动看不到）。

**文件**：`app/encv-mobile/lynx-player/src/components/AppComponent.tsx`

改动点：
1. `startPlayback()` 入口立即打点 + initData 空值保护：
```tsx
const startPlayback = async (data: InitData | null) => {
  lynxLog.info("startPlayback called, data=" + JSON.stringify(data));
  if (!data?.filePath) {
    lynxLog.error("startPlayback: filePath is empty!");
    setPlayerState("error");
    setErrorMessage(data ? "文件路径为空" : "未收到播放数据");
    return;
  }
  // ... 继续原有逻辑
}
```
2. 每个 await 步骤前后加 lynxLog.info：
```tsx
lynxLog.info("Step 1: checking backend status...");
const status = await GoBackendModule.getBackendStatus();
lynxLog.info("Step 1 result: " + JSON.stringify(status));

lynxLog.info("Step 2: getting stream url for " + data.filePath);
const url = await GoBackendModule.getStreamUrl(data.filePath, data.isExternal);
lynxLog.info("Step 2 result: " + (typeof url === "string" ? url.substring(0, 50) : "error"));
```
3. play() 调用前后加日志：
```tsx
lynxLog.info("Step 4: calling MpvPlayerModule.play...");
await MpvPlayerModule.play(url);
lynxLog.info("Step 4: play() returned successfully");
```

**文件**：`app/encv-mobile/lynx-player/src/components/PlayerControls.tsx`

改动点：
1. error 状态增加视觉明显的重试入口：
```tsx
<view className="ErrorContainer">
  <view className="PlayButtonCircle" bindtap={onPlayPause}>
    <text className="PlayIconLarge">🔄</text>
  </view>
  <text className="ErrorTitle">⚠ 播放失败</text>
  <text className="ErrorDetail">{error || "未知错误，点击重试"}</text>
</view>
```
2. idle 空文件名兜底：
```tsx
<text className="IdleTitle">{fileName || "等待文件信息..."}</text>
```

### Step 3：MPV 模块增加状态广播

**文件**：`app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt`

让前端能感知 MPV 生命周期关键节点：

```kotlin
// surfaceCreated() 中 attachSurface 成功后：
dispatchStateChange("surface_ready")  // 新增

// play() 中 surfaceReady=false 时：
if (!surfaceReady) {
    pendingUrl = url
    dispatchStateChange("waiting_surface")  // 新增
    return
}
// create() 成功后：
dispatchStateChange("mpv_ready")  // 新增
```

前端 AppComponent 中新增状态处理：
```tsx
case "surface_ready":
  lynxLog.info("MPV surface ready");
  if (pendingPlaybackData) startPlayback(pendingPlaybackData);
  break;
case "waiting_surface":
  setPlayerState("loading");
  setErrorMessage("正在初始化视频窗口...");
  break;
case "mpv_ready":
  lynxLog.info("MPV engine ready");
  break;
```

### Step 4：构建验证

```bash
cd app/encv-mobile/lynx-player && npm run build
# CSS 兼容性检查
grep -nE 'position:\s*absolute|overflow:\s*hidden|gap:\s|text-overflow' src/App.css \
  && echo "❌ 仍有不支持的CSS" || echo "✅ CSS兼容"
node --check ../scripts/post-cap-sync.mjs
```
