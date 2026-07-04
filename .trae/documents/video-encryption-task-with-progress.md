# 视频插件加密任务 — 前后端完整实现方案（续）

## 现状盘点

### 后端已完成
- ✅ `MobileTask` 结构体扩展（Phase/Speed/Eta/TargetPath/cancelFn）
- ✅ `updateProgress()` 方法 + WebSocket `task:progress` 广播
- ✅ `formatSpeed()` / `formatDuration()` 辅助函数
- ✅ `monitorFileProgress()` goroutine（3秒轮询文件大小变化）
- ✅ `processEncrypt/processDecrypt` 阶段进度回调 + targetPath 输出目录
- ✅ `Cancel()` 方法调用 `cancelFn()`
- ✅ API `handleCreateTask` 支持 targetPath
- ✅ ffmpeg `-threads 2` 内存保护

### 后端遗留问题（需修复）
1. **ffmpeg stderr 未捕获**：`content_preprocessor.go` 中 ffmpeg 命令用 `cmd.Stderr = os.Stderr`，进度信息直接输出到进程 stderr，无法解析
2. **context 取消未传播**：`processEncrypt` 创建了 `context.WithCancel` 但未传递给插件管道，ffmpeg 进程无法被取消
3. **h264_nvenc 硬编码**：`transcodeToFastStartMP4` 使用 NVIDIA 硬件编码，Android 设备不支持
4. **monitorFileProgress 进度计算不准确**：用源文件大小作为 totalSize，但加密后输出可能更大（加密开销）或更小（压缩），导致进度不准
5. **取消时无临时文件清理**：ffmpeg 中断后临时文件残留
6. **无磁盘空间预检**：大文件加密可能耗尽磁盘空间

### 前端待完成
1. `EncvTask` 类型缺少 phase/speed/eta/targetPath 字段
2. `useEventBus.ts` 缺少 `task:progress` 事件定义
3. `Tasks.vue` 无 `task:progress` 事件监听
4. 进度 UI 只有进度条，无阶段标签/速度/ETA
5. i18n 缺少阶段翻译

---

## 实现步骤

### Step 1：前端 — EncvTask 类型扩展 + 事件总线

**文件**：`src/api/encv.ts`

扩展 `EncvTask` 接口：
```ts
export interface EncvTask {
  id: string
  type: TaskType
  sourcePath: string
  targetPath?: string
  status: TaskStatus
  progress: number
  phase?: string
  speed?: string
  eta?: string
  error?: string
  createdAt: string
}
```

**文件**：`src/composables/useEventBus.ts`

添加 `task:progress` 事件定义：
```ts
'task:progress': { id: string; progress: number; phase: string; speed: string; eta: string }
```

### Step 2：前端 — Tasks.vue 事件监听 + 进度 UI

**文件**：`src/views/Tasks.vue`

1. 添加 `task:progress` 事件监听：
```ts
function onTaskProgress(data: { id: string; progress: number; phase: string; speed: string; eta: string }) {
  const idx = tasks.value.findIndex(t => t.id === data.id)
  if (idx !== -1) {
    tasks.value[idx] = {
      ...tasks.value[idx],
      progress: data.progress,
      phase: data.phase,
      speed: data.speed,
      eta: data.eta,
    }
  }
}
```

2. 进度区域 UI 增强：
- 进度条保持 `ion-progress-bar`
- 进度条下方显示：阶段标签 | 速度 | ETA
- 阶段标签用 `ion-badge` 或 `ion-chip` 小标签
- 速度和 ETA 用小字灰色文字
- 完成时显示绿色勾 + 耗时

3. 取消按钮优化：
- running 状态的任务在卡片内直接显示取消按钮（不需要滑动）
- cancelling 状态显示 spinner

4. 任务卡片增强：
- 显示源文件名（已有）
- 显示文件大小（需后端返回）
- 完成时显示输出路径

### Step 3：前端 — i18n 翻译

**文件**：`src/composables/useI18n.ts`

添加阶段翻译（中英文）：
```
tasks.phaseAnalyzing: '分析文件中...' / 'Analyzing...'
tasks.phaseInitializing: '初始化引擎...' / 'Initializing...'
tasks.phasePreprocessing: '预处理中...' / 'Preprocessing...'
tasks.phaseEncrypting: '加密中...' / 'Encrypting...'
tasks.phasePacking: '打包中...' / 'Packing...'
tasks.phaseVerifying: '验证中...' / 'Verifying...'
tasks.phaseDecrypting: '解密中...' / 'Decrypting...'
tasks.phaseCompleted: '已完成' / 'Completed'
tasks.eta: '剩余' / 'ETA'
```

### Step 4：后端 — ffmpeg 进度捕获 + context 取消传播

**文件**：`internal/v2/plugins/video/content_preprocessor.go`

1. 新增 `runFFmpegWithProgress` 函数：
   - 捕获 ffmpeg stderr 输出
   - 解析 `time=` 和 `speed=` 行
   - 通过回调函数报告进度
   - 支持 context 取消（`cmd.Process.Kill()`）

2. 修改 `transcodeToFastStartMP4`：
   - 使用 `runFFmpegWithProgress` 替代 `cmd.Run()`
   - 添加 h264_nvenc → libx264 自动降级
   - 传递 context 用于取消

3. 修改 `remapMP4ForFastStart`：
   - 使用 `runFFmpegWithProgress` 替代 `cmd.Run()`
   - 传递 context 用于取消

4. 修改 `remapWithMKVMerge`：
   - 传递 context 用于取消

**关键设计**：由于插件管道（ContentPreprocessor → Encrypt → PostEncryptProcessor）没有进度回调接口，我们采用两层方案：
- **外层**：`monitorFileProgress` goroutine 轮询输出目录文件大小变化（已实现）
- **内层**：ffmpeg 命令本身解析 stderr 获取精确进度（新增）

### Step 5：后端 — context 取消传播到插件管道

**文件**：`internal/service/task_manager.go`

1. `processEncrypt` 中创建 context 并传递：
   - 将 context 存入 `config.Context` 中
   - 插件管道各阶段检查 `ctx.Done()`

**文件**：`internal/v2/plugins/video/plugin.go`

1. 在 `Encrypt` 方法中检查 context 取消
2. 在 `PostEncryptProcessor` 中检查 context 取消
3. 取消时清理临时文件

### Step 6：后端 — h264_nvenc 自动降级

**文件**：`internal/v2/plugins/video/content_preprocessor.go`

修改 `transcodeToFastStartMP4`：
1. 先尝试 h264_nvenc（NVIDIA GPU）
2. 失败后尝试 h264_mediacodec（Android MediaCodec）
3. 最终降级到 libx264（CPU 软编码）
4. 降级过程自动检测，无需用户配置

### Step 7：后端 — 磁盘空间预检 + 临时文件清理

**文件**：`internal/service/task_manager.go`

1. `processEncrypt` 开始前检查目标磁盘可用空间：
   - 预估输出大小 ≈ 源文件大小 × 1.5（加密+临时文件开销）
   - 可用空间不足时提前报错

2. 取消/失败时清理临时文件：
   - `processEncrypt` defer 中清理 outputDir 中的临时文件（encv-pre-* 模式）
   - 取消时 ffmpeg 进程被 Kill 后，临时文件自动不完整，需删除

### Step 8：后端 — monitorFileProgress 精度优化

**文件**：`internal/service/task_manager.go`

1. 改进进度计算：
   - 使用 ffmpeg 报告的 duration 作为总时长基准（而非文件大小）
   - 预处理阶段（ffmpeg 转码）：用 ffmpeg stderr 的 time/总时长 计算进度
   - 加密阶段：用文件大小比例计算进度
   - 打包阶段：用分片数量计算进度

2. 阶段进度映射：
   - analyzing: 0-5%
   - initializing: 5-10%
   - preprocessing: 10-40%（ffmpeg 转码最耗时）
   - encrypting: 40-80%
   - packing: 80-95%
   - verifying: 95-99%
   - completed: 100%

---

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `src/api/encv.ts` | EncvTask 类型扩展（phase/speed/eta/targetPath） |
| `src/composables/useEventBus.ts` | 添加 task:progress 事件定义 |
| `src/views/Tasks.vue` | 进度 UI 升级 + task:progress 监听 + 取消按钮优化 |
| `src/composables/useI18n.ts` | 阶段翻译（中英文） |
| `internal/v2/plugins/video/content_preprocessor.go` | ffmpeg 进度捕获 + context 取消 + 编码器降级 |
| `internal/service/task_manager.go` | context 传播 + 磁盘预检 + 临时文件清理 + 进度精度优化 |
| `internal/v2/plugins/video/plugin.go` | context 取消检查 + 临时文件清理 |

## 优先级

1. **Step 1-3**（前端核心）：类型扩展 + 事件监听 + 进度 UI + i18n — 用户可见的核心体验
2. **Step 4-5**（后端核心）：ffmpeg 进度捕获 + context 取消 — 稳定性和进度准确性
3. **Step 6-7**（后端增强）：编码器降级 + 磁盘预检 + 清理 — 边界处理
4. **Step 8**（后端优化）：进度精度优化 — 体验提升

## 风险与边界

- **ffmpeg stderr 解析**：ffmpeg 的 `-progress` 输出格式相对稳定，但不同版本可能有差异，正则需要容错
- **编码器降级**：Android 设备可能支持 MediaCodec 硬件编码，但 ffmpeg 需要编译时启用
- **磁盘空间预估**：加密后大小难以精确预估（取决于压缩率），1.5x 是保守估计
- **context 传播**：当前插件管道接口不接受 context 参数，需要通过 config.Context 间接传递
- **临时文件清理**：取消时可能存在竞态条件（ffmpeg 正在写入时被 Kill），需要安全删除
