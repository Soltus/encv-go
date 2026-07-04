# 计划：CI NDK 集成 + 播放器 UI 完善

## Part A：CI 集成 NDK 编译 libplayer.so

### 问题

`setup-mpv-libs.sh` 现在的 Phase 2 调用 `build-player-so.sh` → `ndk-build`，但 CI 环境没有安装 NDK。

### 方案：在 workflow 中添加 NDK 安装步骤

#### Step A1：修改 [android.yml](file:///workspace/.github/workflows/android.yml)

在 "Setup Android SDK" 步骤之后、"Setup mpv native libraries" 之前，插入：

```yaml
- name: Setup NDK for libplayer.so build
  run: sdkmanager "ndk;26.1.10909125"
  env:
    ANDROID_HOME: ${{ steps.sdk-setup.outputs.path }}
    ANDROID_NDK_HOME: ${{ steps.sdk-setup.outputs.path }}/ndk/26.1.10909125
```

注意：`setup-android@v3` action 的输出中 `path` 就是 `$ANDROID_SDK_ROOT`。NDK 安装后位于 `{ANDROID_SDK_ROOT}/ndk/{version}/`。

#### Step A2：确保 setup-mpv-libs.sh 能找到 NDK

当前 [build-player-so.sh](file:///workspace/app/encv-mobile/scripts/build-player-so.sh) 已有自动搜索逻辑：
1. `$ANDROID_NDK_HOME`
2. `$HOME/Android/Sdk/ndk/*/`

CI 中通过 `ANDROID_NDK_HOME` 环境变量传入即可。

---

## Part B：播放器 UI 完善

### 当前状态分析

**现有组件**：
| 组件 | 当前实现 | 问题 |
|------|----------|------|
| 播放/暂停 | 文字 `▶`/`⏸` 36px | 无背景圈，不够醒目 |
| 全屏按钮 | 文字 `⤢`/`⤓` | 同上 |
| 进度条 | 4px 高细线 + 点击跳转 | 无拖拽把手、无缓冲进度 |
| 加载状态 | "Loading..." 文字 | 无动画指示器 |
| 错误状态 | 红色标题+灰文字 | 功能够用但可更美观 |
| idle 状态 | 居中大 ▶ + 文件名 | 基本可用 |
| 控制栏隐藏 | 点击切换 showControls | ✅ 有基础逻辑 |
| 返回/关闭按钮 | ❌ 缺失 | 用户无法退出播放器 |

**Lynx CSS 能力边界**（必须遵守）：
- ✅ Flexbox 布局（flex-direction, justify-content, align-items）
- ✅ 基础样式（color, font-size, padding, margin, background-color, border-radius）
- ✅ text-overflow: ellipsis, text-maxline
- ✅ bindtap 事件
- ⚠️ 不支持 position: absolute → 用 Flexbox 替代
- ⚠️ 不支持 transform → 无法做旋转动画
- ⚠️ 不支持 linear-gradient → 用纯色替代
- ⚠️ 不支持 opacity 动画 → 用颜色透明度模拟
- ⚠️ 不支持 ::before/::after 伪元素 → 需要用真实元素
- ⚠️ 不支持 box-shadow → 用 border 模拟或省略

### 设计目标

打造一个**现代视频播放器界面**，参考 VLC/MPV 原生播放器的布局：

```
┌─────────────────────────────────────┐
│ ← Back        File Name.mkv    ⛶ FS │  ← TopBar (固定)
│                                     │
│                                     │
│            ┌───────┐               │
│            │   ▶   │               │  ← CenterControls
│            └───────┘               │     (大圆形播放按钮)
│                                     │
│                                     │
│  0:12  ━━━━━●━━━━━━━━━━━  23:45  │  ← BottomBar (固定)
│                                     │
└─────────────────────────────────────┘
```

### 实施步骤

#### Step B1：PlayerControls.tsx — 重构为完整控制栏

**TopBar 改进**：
- 左侧添加 **返回按钮** `✕` 或 `←`（调用 `NativeModules.MpvPlayerModule` 的 back/close 方法，或直接 finish Activity）
- 右侧保留全屏按钮
- 文件名居中，超长时省略号截断

**CenterControls 改进**：
- 播放按钮改为**大圆圈背景**（用 view 的 border-radius: 50% + background-color 模拟）
- 图标居中在圆圈内
- 圆圈尺寸 72x72px，视觉焦点明确
- idle 状态下显示文件名缩略信息在按钮下方

**BottomBar 改进**：
- 进度条高度增加到 **6px**
- 添加**缓冲进度条**（浅灰色底层，深灰色已加载，蓝色已播放）— 三层结构
- 进度条右侧添加**圆形拖拽把手**（用小 view 圆形模拟，12x12px 白色圆点）
- 时间标签字号增大到 13px，增加可读性
- 时间格式优化：< 1h 显示 `M:SS`，≥ 1h 显示 `H:MM:SS`

**新增功能**：
- Loading 状态改为**三点点动画**（用 3 个小圆点 view + 不同透明度模拟脉冲效果）

#### Step B2：App.css — 新增样式类

需要新增/修改的样式：

```css
/* TopBar */
.TopBar {
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px 8px;
  width: 100%;
}
.BackButton { color: #ffffff; font-size: 22px; padding: 8px; }

/* CenterControls - 大圆播放按钮 */
.PlayButtonCircle {
  width: 72px;
  height: 72px;
  border-radius: 36px;
  background-color: rgba(255,255,255,0.2);
  justify-content: center;
  align-items: center;
}
.PlayButtonIcon { color: #ffffff; font-size: 32px; }

/* BottomBar - 三层进度条 */
.ProgressBarContainer {
  flex-direction: row;
  align-items: center;
  padding: 0 16px 12px;
  width: 100%;
}
.SliderTrackOuter {
  flex: 1;
  height: 6px;
  background-color: rgba(255,255,255,0.15);
  border-radius: 3px;
  overflow: hidden;
}
.SliderBuffered {
  height: 6px;
  background-color: rgba(255,255,255,0.3);
  border-radius: 3px;
}
.SliderFill {
  height: 6px;
  background-color: #4a90d9;
  border-radius: 3px;
  position: absolute; /* ⚠️ Lynx 可能不支持，降级方案见下文 */
}
.SliderThumb {
  width: 12px;
  height: 12px;
  border-radius: 6px;
  background-color: #ffffff;
  margin-left: -6px;
}

/* Loading 动画 */
.LoadingDots {
  flex-direction: row;
  justify-content: center;
  align-items: center;
  gap: 6px;
}
.LoadingDot {
  width: 8px;
  height: 8px;
  border-radius: 4px;
  background-color: #4a90d9;
}
.Dot1 { opacity: 1.0; }
.Dot2 { opacity: 0.5; }
.Dot3 { opacity: 0.2; }
```

> **⚠️ Lynx position:absolute 限制处理**：如果 `position: absolute` 不可用，改用嵌套 view 结构实现三层进度条：
> ```html
> <view className="SliderTrackOuter">
>   <view className="SliderBuffered" style={{width: bufferedPct + "%"}}>
>     <view className="SliderFill" style={{width: playedPct + "%"}}/>
>   </view>
> </view>
> ```

#### Step B3：AppComponent.tsx — 新增返回/关闭功能

添加 handleBack 函数：
- 通过 NativeModules 调用新的 `finish()` 方法（需在 MpvPlayerModule 中添加）
- 或者利用 LynxContext.activity?.finishActivity() 关闭 PlayerActivity

新增 **buffered 进度跟踪**（如果 MPVLib 支持）：
- 目前 mpv 的 observeProperty 可以观察 `cache-buffered-state` 或类似属性
- 如果不可用则暂时不显示缓冲层，只显示已播放层

#### Step B4：MpvPlayerModule.kt — 添加 finish() 方法

```kotlin
@LynxMethod
fun finish(callback: Callback) {
    try {
        activity?.finish()
        callback.invoke(true)
    } catch (e: Exception) {
        callback.invoke(e.message)
    }
}
```

同时在 typing.d.ts 中声明。

#### Step B5：typing.d.ts 更新

添加 `finish` 方法声明。

## 文件改动清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `.github/workflows/android.yml` | 修改 | 添加 NDK 安装步骤 |
| `lynx-player/src/components/PlayerControls.tsx` | 重写 | 完整现代播放器 UI |
| `lynx-player/src/App.css` | 重写 | 所有新样式类 |
| `lynx-player/src/components/AppComponent.tsx` | 修改 | 添加返回功能 + buffered 进度 |
| `android-overlay/.../MpvPlayerModule.kt` | 修改 | 添加 finish() 方法 |
| `lynx-player/src/typing.d.ts` | 修改 | 添加 finish 类型声明 |

## 实施顺序

1. **A1-A2**: CI NDK 集成（先让构建能跑通）
2. **B4-B5**: 后端 finish() 方法（UI 需要的基础能力）
3. **B1**: PlayerControls.tsx 重构（核心 UI）
4. **B2**: App.css 样式（视觉效果）
5. **B3**: AppComponent.tsx 集成（串联所有功能）
