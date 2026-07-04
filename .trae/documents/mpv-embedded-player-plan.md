# MPV 播放器并行调试方案（A/B/C 三轨并行）

## 核心调试策略

**并行实施 + 前端切换 = CI 一次验证全部**

```
设置页面播放器选项：
├── Artplayer (内置)              ← 现有，不变
├── MPV-Activity (透明 Activity)   ← 方案 C ⚡ 最快落地
├── MPV-Fragment (Fragment嵌入)    ← 方案 B 备选 experimental
├── MPV-Compose (ComposeView原生)  ← 方案 A ⭐ 最终目标
└── 外部打开                        ← 现有，不变

用户切换选项 → 同一 CI 构建可测试三种 MPV 播放方式
```

---

## 调试铁律（新增）：并行调试原则

> **当有多种技术方案可选时，应并行实施所有可行方案，通过前端配置/选项切换来选择方案，
> 而非逐一尝试。这样 CI 一次构建即可验证所有方案的编译正确性和基本功能，
> 避免反复提交浪费 CI 资源和上下文切换成本。**

### 适用场景
1. 多种 UI 实现方案（如本例的 Activity/Fragment/ComposeView）
2. 多种数据源方案（本地/网络/缓存）
3. 多种渲染方案（Canvas/SVG/WebGL）

### 实施规范
- **每个方案独立代码路径**，不互相依赖（一个方案崩溃不影响其他方案）
- **前端提供统一切换入口**（Settings.vue select），选项旁显示方案状态标签
- **Kotlin 后端根据 mode 参数分发到不同实现**
- **日志中必须打印当前使用的方案名**（如 `[ModeC-Activity]`），便于定位问题
- **experimental 方案标记为默认不启用**，但代码必须编译通过

### 反模式（禁止）
- ❌ 先做 A → CI → 发现不行 → 改 B → 再 CI → 再改 C（浪费 3 次 CI）
- ❌ 只实现一种"最优方案"，其他方案等出问题再考虑
- ✅ 一次提交包含 A+B+C 全部代码，CI 通过后用户自由切换测试

> **此铁律需同步写入 `saturation-debugging.md` §1.5**

---

## 方案对比总览

| 维度 | 方案 C: 透明 Activity | 方案 B: Fragment 嵌入 | 方案 A: ComposeView 原生 |
|------|---------------------|---------------------|------------------------|
| **用户体验** | ⚠️ 无视觉跳转(透明主题)，但仍是独立 Activity | ✅ 视觉嵌入页面 | ✅ 完全无跳转，与 Artplayer 一致 |
| **白屏风险** | ✅ setResult 可回传错误到 Files.vue | ⚠️ 需改造 BaseHostActivity | ✅ 直接回传 JS callback |
| **复杂度** | **低**（改动最小） | **高**（ComboLite 不支持 Fragment） | **中**（需新建 Service） |
| **CI 验证可行性** | ✅ 本次可验证 | ⚠️ 框架代码，标记 experimental | ✅ 本次可验证 |
| **推荐优先级** | **Phase 1 先做** | 备选（风险大） | Phase 2 目标 |

---

## Task 清单

### Task 1: 基础架构 — mode 分发机制

**目标**：PlayerEntry.kt 支持 `mpv-*` 子模式分发，Settings.vue 扩展为 5 个选项。

- [ ] **SubTask 1.1**: PlayerEntry.kt `play()` — mode 归一化处理
  - `"mpv"` / `"mpv-plugin"` → 统一映射到默认 MPV 子模式（初始用 `mpv-activity` 即方案 C）
  - 新增 `mpv-activity` / `mpv-fragment` / `mpv-compose` 三个子模式常量
  - 日志输出 `[ModeX]` 前缀标识当前方案

- [ ] **SubTask 1.2**: PlayerEntry.kt `startMpvPlayer()` — 按 mode 分发
  ```kotlin
  private fun startMpvPlayer(..., mode: String): PlayResult {
      return when (mode) {
          "mpv-activity"  -> { Log.i(TAG, "[ModeC-Activity] starting..."); startMpvViaActivity(...) }
          "mpv-fragment"  -> { Log.i(TAG, "[ModeB-Fragment] starting..."); startMpvViaFragment(...) }
          "mpv-compose"   -> { Log.i(TAG, "[ModeA-Compose] starting..."); startMpvViaCompose(...) }
          else            -> { Log.i(TAG, "[ModeC-Activity] fallback"); startMpvViaActivity(...) }
      }
  }
  ```

- [ ] **SubTask 1.3**: Settings.vue — 扩展播放器选项列表
  ```vue
  <ion-select-option value="artplayer">{{ t('settings.builtInArtplayer') }}</ion-select-option>
  <ion-select-option value="mpv-activity">MPV (Activity)</ion-select-option>
  <ion-select-option value="mpv-fragment" :disabled="true">MPV (Fragment) [实验]</ion-select-option>
  <ion-select-option value="mpv-compose" :disabled="true">MPV (Compose) [实验]</ion-select-option>
  <ion-select-option value="external">{{ t('settings.openExternal') }}</ion-select-option>
  ```
  - mpv-fragment 和 mpv-compose 初始 disabled（实验性方案）
  - 所有 MPV 子选项共享同一个 MPV 插件状态徽章逻辑

- [ ] **SubTask 1.4**: useI18n.ts — 添加新选项翻译 key
  - `settings.mpvPlayerActivity` / `settings.mpvPlayerFragment` / `settings.mpvPlayerCompose`

---

### Task 2: 方案 C 实现 — 透明 Activity + 错误回传（⚡ 最快落地）

**目标**：EncvHostActivity 透明启动，失败时通过 setResult 回传错误到 Files.vue 显示 banner。

- [ ] **SubTask 2.1**: AndroidManifest.xml — EncvHostActivity 设置透明主题
  ```xml
  <activity android:name=".EncvHostActivity"
      android:theme="@android:style/Theme.Translucent.NoTitleBar"
      android:exported="false" />
  ```

- [ ] **SubTask 2.2**: EncvHostActivity.kt — 所有失败路径确保 setResult + finish()
  - 当前已有 `finishWithResult()` 实现 ✅
  - 补充：成功播放完成时的 setResult（用户按返回键退出时）
  - 补充：onDestroy 中未 setResult 的兜底（防止泄漏）

- [ ] **SubTask 2.3**: GoProcessPlugin.kt — 新增 `REQUEST_CODE_MPV_PLAYER` + onActivityResult 处理
  ```kotlin
  private const val REQUEST_CODE_MPV_PLAYER = 9003

  @CapacitorPlugin(
      name = "GoProcess",
      requestCodes = [REQUEST_CODE_PLUGIN_PICK, REQUEST_CODE_INSTALL_CONFIRM, REQUEST_CODE_MPV_PLAYER]
  )
  ```
  - `openPlayer()` 当 mode 为 `mpv-*` 时使用 `startActivityForResult()` 而非 `startActivity()`
  - `onActivityResult()` 中提取 `player_success` / `player_error` / `player_error_detail`
  - 将结果存入 pendingCall 或通过 Promise 返回

- [ ] **SubTask 2.4**: GoProcess.ts — openPlayer() 支持 Promise 返回 PlayResult
  - 当 mode 包含 `mpv-activity` 时，使用 `{ callback: Promise }` 模式等待 ActivityResult
  - 其他 mode 保持现有同步返回行为

- [ ] **SubTask 2.5**: Files.vue — playMedia() 处理 ActivityResult 错误
  - `openPlayer()` 返回 `PlayResult { success: false }` 时显示红色 error banner（已有机制 ✅）
  - 确保 banner 可关闭、可展开详情

---

### Task 3: 方案 A 实现 — ComposeView 原生嵌入（⭐ 最终目标）

**目标**：MPV 播放器以 ComposeView 形式嵌入 WebView 页面，用户无跳转感。

- [ ] **SubTask 3.1**: MpvEmbedService.kt（新建）— ComposeView 生命周期管理
  ```kotlin
  object MpvEmbedService {
      fun startEmbed(activity: Activity, containerId: String, filePath: String, fileName: String): PlayResult
      fun stopEmbed(): Boolean
      fun isEmbedded(): Boolean
  }
  ```
  - 内部管理 ComposeView 实例和 MpvPlayerScreen Composable
  - 处理 Compose 生命周期（start/stop/dispose）

- [ ] **SubTask 3.2**: GoProcessPlugin.kt — 新增 `startMpvInPlace()` / `stopMpvInPlace()`
  - `startMpvInPlace()`: 在当前 Activity 中创建 ComposeView 并附加到指定 View ID
  - `stopMpvInPlace()`: 移除 ComposeView，恢复 WebView 正常布局
  - 需要 BridgeActivity 引用或 View 定位能力

- [ ] **SubTask 3.3**: GoProcess.ts — 对应函数封装
  ```typescript
  export async function startMpvInPlace(filePath: string, fileName: string): Promise<PlayResult>
  export async function stopMpvInPlace(): Promise<{ success: boolean }>
  ```

- [ ] **SubTask 3.4**: Files.vue — mpv-container 管理
  ```vue
  <div id="mpv-container" v-if="showMpvPlayer" class="mpv-overlay"></div>
  ```
  - 播放时隐藏文件列表、显示容器
  - 返回/关闭时恢复文件列表

- [ ] **SubTask 3.5**: 触摸事件穿透处理
  - ComposeView 覆盖在 WebView 上方时，需要处理触摸事件冲突
  - MPV 全屏播放时可消费所有触摸事件
  - 非全屏状态需要考虑返回键优先级

---

### Task 4: 方案 B 框架 — Fragment 嵌入（experimental）

**目标**：保留 Fragment 方案的框架代码，标记为 experimental。

- [ ] **SubTask 4.1**: MpvPlayerFragment.kt（新建）— 框架占位代码
  ```kotlin
  class MpvPlayerFragment : Fragment() {
      // TODO: ComboLite 不原生支持 Fragment 代理
      // 需要等待 ComboLite 提供 BaseHostFragment 或自行封装 ProxyManager
      companion object {
          const val TAG = "MpvPlayerFragment"
      }
  }
  ```

- [ ] **SubTask 4.2**: PlayerEntry.kt `startMpvViaFragment()` — 占位实现
  ```kotlin
  private fun startMpvViaFragment(...): PlayResult {
      Log.w(TAG, "[ModeB-Fragment] not yet implemented, falling back to ModeC")
      return startMpvViaActivity(...)  // 兜底到方案 C
  }
  ```

- [ ] **SubTask 4.3**: 标记为 experimental
  - Settings.vue 中选项显示 `[实验]` 且 disabled
  - 代码编译通过但不启用功能路径

---

### Task 5: 饱和调试日志

**目标**：每个方案有独立的日志前缀，便于在 logcat 中快速过滤。

- [ ] **SubTask 5.1**: 每个 mode 分发点输出方案名标识
  - `[ModeC-Activity]` / `[ModeB-Fragment]` / `[ModeA-Compose]`
  - 包含 filePath、fileName、mode 参数

- [ ] **SubTask 5.2**: 每个方案的成功/失败路径完整日志
  - 成功：`[ModeX] success ✓ cost=XXXms`
  - 失败：`[ModeX] failed ✗ reason=... detail=...`

- [ ] **SubTask 5.3**: EncvHostActivity 诊断增强
  - onCreate/onResume/onDestroy 时间戳
  - Intent extras 完整 dump
  - proxyStarted 状态转换日志

---

### Task 6: 验证

- [ ] **SubTask 6.1**: 前端构建验证
  - `cd app/encv-mobile && npx vue-tsc --noEmit && npx vite build` 通过

- [ ] **SubTask 6.2**: Kotlin 编译验证
  - Gradle build 编译无错误（CI 环境）

- [ ] **SubTask 6.3**: 功能验证清单
  - Artplayer 模式不受影响
  - MPV-Activity 模式可选择且能触发 startActivity
  - MPV-Fragment/Compose 模式显示为 disabled 但不崩溃
  - 外部打开模式不受影响
  - MPV 状态徽章对所有 MPV 子模式正确显示

---

## 改动文件总览

| 文件 | Task 1 | Task 2 | Task 3 | Task 4 | Task 5 |
|------|--------|--------|--------|--------|--------|
| `PlayerEntry.kt` | ✅ mode 分发重写 | — | — | 占位实现 | ✅ 日志 |
| `EncvHostActivity.kt` | — | ✅ setResult 完善 | — | — | ✅ 诊断 |
| `AndroidManifest.xml` | — | ✅ 透明主题 | — | — | — |
| `GoProcessPlugin.kt` | — | ✅ startActivityForResult | ✅ inPlace API | — | — |
| `GoProcess.ts` | — | ✅ Promise 返回 | ✅ 封装 | — | — |
| `Files.vue` | — | ✅ 错误处理 | ✅ container | — | — |
| `Settings.vue` | ✅ 5 个选项 | — | — | — | — |
| `useI18n.ts` | ✅ 新 key | — | — | — | — |
| `MpvEmbedService.kt` (新建) | — | — | ✅ 核心 | — | — |
| `MpvPlayerFragment.kt` (新建) | — | — | — | ✅ 框架 | — |

---

## 铁律合规检查

| 铁律 | 合规方式 |
|------|---------|
| **严禁 Toast** | 所有错误通过 Files.vue error banner 显示 |
| **严禁 fallback** | 失败后不自动切换方案，显示错误让用户主动选择 |
| **严禁白屏** | 方案 C: setResult 回传；方案 A: JS callback；方案 B: 兜底到 C |
| **饱和调试** | `[ModeX]` 前缀日志，每个方案独立诊断 |
| **并行调试** | A/B/C 三轨并行，前端 Settings 切换 |

---

## 后续动作（计划批准后）

1. **立即更新** `saturation-debugging.md` 新增 §1.5 并行调试原则
2. 按 Task 1→2→5→6 顺序实施（先落地方案 C，再做饱和调试，最后验证）
3. Task 3（方案 A）和 Task 4（方案 B）在同一批次编写代码，但 Task 3/4 初始 disabled
