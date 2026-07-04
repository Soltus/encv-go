# Phase 4 架构：彻底解决真机 mock generator hang + codec 补完

> **目标**：真机 ffmpeg 调用彻底不被 cgo 阻塞、Go 进程 hang 能被 Kotlin 主动探活并强杀重启、mp3/flac/alac 全部真机能编能跑。
>
> **核心思路**：把 cgo 阻塞装进 worker 子进程（父进程 SIGKILL unblock）+ 用文件 mtime 做跨进程探活 + 用 ffmpeg 内置+第三方库补全 encoder。

---

## 0. 现状分析（探索产出）

### 0.1 已确认的根因

| 层 | 问题 | 现状 |
|---|------|------|
| **FFmpeg 调用层** | 真机 cgo `CallFFmpegNative` → `libffmpeg.so` 内部状态污染 → 第二次调用 hang | [ffmpeg_dlopen.go:84](file:///workspace/internal/utils/ffmpeg_dlopen.go#L84) `run_fn(argc, argv)` 阻塞 OS thread |
| **Runner 抽象层** | `native_runner.go:88-95` 试 WorkerRunner 不可用就 fallback cgo | WorkerRunner.Available()=false（找不到 binary） |
| **子进程层** | `cmd/ffmpeg-worker/main_android.go` 早就设计好 cgo 子进程化（cgo 阻塞在子进程里） | **但 build-ffmpeg-android.sh 完全没编 libffmpeg-worker.so**（grep 0 匹配） |
| **探活层** | Kotlin `Process.isAlive()` 只能看 OS alive | **cgo hang 时 Go 进程 OS 层还活着** → isAlive=true → 不重启 |
| **前端层** | 30s abort 后只看到 2 行 entry | **没把 hang 性质透传给用户** |
| **Codec 层** | mp4/m4a 真机能编（-c copy / aac native） | **mp3/flac/alac 真机没 encoder**（libmp3lame/libFLAC/alac 都没编） |

### 0.2 已存在的"正解"（设计好了但没接通）

- ✅ `cmd/ffmpeg-worker/main_android.go` — cgo dlopen libffmpeg.so（cgo 阻塞在子进程）
- ✅ `cmd/ffmpeg-worker/main_exec.go` — 沙箱 exec ffmpeg binary
- ✅ `internal/utils/ffmpeg/worker_runner.go` — 父进程 exec.CommandContext + SIGKILL worker
- ✅ `internal/utils/ffmpeg/worker_client.go` — JSON over stdin/stdout IPC
- ✅ `native_runner.go:88-95` init() 自动选 WorkerRunner（仅当 Available）
- ✅ Kotlin [EncvGoService.kt:632-635](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt#L632-L635) 已设 `ENCV_FFMPEG_WORKER` 指向 `nativeLibraryDir/libffmpeg-worker.so`

**唯独缺一个 binary**：`libffmpeg-worker.so` 没人编。

---

## 1. 架构总览

```
┌─────────────────────────────────────────────────────────────────┐
│  Android (Kotlin)                                                │
│                                                                  │
│  ┌─────────────────┐    poll lastModified()      ┌────────────┐│
│  │  EncvGoService  │◄────────────────────────────┤ .encv_hb   ││
│  │  (worker exec)  │  1s tick, >8s stale = HANG  │  (mtime)   ││
│  │                 │──────────────────────────────►            ││
│  │                 │   kill goProcess + restart                ││
│  │                 │   + CustomEvent('encv:backend-status')    ││
│  └────────┬────────┘                                            │
│           │ exec("libencv-go.so" --goandroid)                   │
│           ▼                                                      │
│  ┌──────────────────────────────────────────────────────┐      │
│  │  Go 主进程 (libencv-go.so)                            │      │
│  │  Gin server :2025                                    │      │
│  │  ├── /api/mock/generate → mock_generator             │      │
│  │  │     └── executeMockSpec()                         │      │
│  │  │         └── ffmpeg.RunWithOutput() ──┐            │      │
│  │  └── ffmpeg.RunWithOutput()             │            │      │
│  │      init() 选 WorkerRunner ────────────┼──┐         │      │
│  │      ──► os.Exec(worker) ───────────────┼──┘         │      │
│  │      ◄── JSON {exit_code, stderr, ...}  │            │      │
│  │      ├── 每完成 1 次 ffmpeg → write .encv_hb        │      │
│  │      └── ctx cancel → cmd.Process.Kill() │            │      │
│  └────────────────────────────┬─────────────┘            │      │
│                                │ exec.CommandContext    │      │
│                                ▼ SIGKILL-able           │      │
│  ┌──────────────────────────────────────────────────────┐      │
│  │  Worker 子进程 (libffmpeg-worker.so)                  │      │
│  │  ──► cgo dlopen libffmpeg.so                         │      │
│  │  ──► utils.CallFFmpegNative(args)                     │      │
│  │       ↑ cgo 阻塞 OS thread (父进程能 SIGKILL 我)    │      │
│  └──────────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────────┘
```

**关键不变量**：
1. cgo 阻塞永远发生在 worker 子进程里（父进程能 SIGKILL）
2. 父进程 1 次 ffmpeg 调用的最坏等待时间 = `timeoutMs + 500ms`（kill+reap）
3. Kotlin 通过 `.encv_hb` 文件 mtime 探活，**绕过 HTTP 路由命名冲突**
4. Worker 子进程启动开销 < 50ms（不预热 dlopen，每次都冷启动 libffmpeg.so → **状态污染被消灭**）

---

## 2. 变更清单（按层）

### 2.1 [build-ffmpeg-android.sh](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh) — 编译产物补全

**A. 末尾新增 libffmpeg-worker.so 编译块**

```bash
echo "=== Building ffmpeg-worker (Go subprocess wrapper) ==="
mkdir -p "${BUILD_DIR}/ffmpeg-worker"
cd "${BUILD_DIR}/ffmpeg-worker"
cat > main_dummy.go <<'GOEOF'
// Ensure 'go build' picks up the android variant (main_android.go has //go:build android)
package main
GOEOF

# ⚠️ libffmpeg-worker.so 必须用 CGO 才能调 libffmpeg.so
# 但 Go 的 cgo + Android NDK 需要先在 ${NDK}/sysroot 把 libffmpeg.so 链进 buildroot
# 简化方案：worker 内部不走 cgo，而是 spawn 一个最小 ffmpeg 包装（见 main_android.go 重构）
GOOS=android GOARCH=arm64 CGO_ENABLED=1 \
    CC="${TOOLCHAIN}/bin/clang" \
    CXX="${TOOLCHAIN}/bin/clang++" \
    CGO_CFLAGS="-I${BUILD_DIR}/ffmpeg/include" \
    CGO_LDFLAGS="-L${BUILD_DIR}/ffmpeg/lib -lffmpeg -Wl,-rpath,${NATIVE_LIB_DIR}" \
    go build -buildmode=c-shared -ldflags='-s -w' \
    -o "${FTOOLS_BUILD}/libffmpeg-worker.so" \
    /workspace/cmd/ffmpeg-worker/

cp "${FTOOLS_BUILD}/libffmpeg-worker.so" "$OUTPUT_DIR/"
ls -la "$OUTPUT_DIR/libffmpeg-worker.so"
```

**B. 补 libFLAC/libogg 编译块**（Phase 3 codec 补完）

```bash
echo "=== Building libogg + libFLAC (Phase 3 codec completion) ==="
# 与 libmp3lame 同模式：下载源码 → CC=${TOOLCHAIN}/bin/clang → configure --host=aarch64-linux-android
# → make → make install
FLAC_VERSION="1.4.3"
OGG_VERSION="1.3.5"
# （详细 configure flags 与 lame 块同模式，参考 [build-ffmpeg-android.sh:71-130](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh#L71-L130)）
```

**C. ffmpeg configure 块补 `--enable-libflac`**

```bash
# 在 EXTRA_LDFLAGS 段加 -lFLAC -logg
# 在 ./configure 段加 --enable-libflac --enable-libogg
# alac 是 ffmpeg native encoder，无需外部库（确认 manifest 包含 alac）
```

**D. manifest 同步**

[ffmpeg-feature-manifest.json](file:///workspace/app/encv-mobile/scripts/ffmpeg-feature-manifest.json) `encoders` 列表加 `libmp3lame`、`flac`、`alac`，`muxers` 已有，`external_libs` 加 `libFLAC`、`libogg`。

### 2.2 [internal/utils/ffmpeg/worker_runner.go](file:///workspace/internal/utils/ffmpeg/worker_runner.go) — Worker 启动健壮性

**A. 增强 LocateWorker() 容错**

```go
func (w *WorkerRunner) locateWorker() (string, error) {
    // 1) 优先 ENCV_FFMPEG_WORKER env（Kotlin EncvGoService.kt:632 已设）
    if path := os.Getenv("ENCV_FFMPEG_WORKER"); path != "" {
        if _, err := os.Stat(path); err == nil {
            return path, nil
        }
        log.Warnf("ENCV_FFMPEG_WORKER=%s but stat failed: %v", path, err)
    }
    // 2) fallback: ${NATIVE_LIBRARY_PATH}/libffmpeg-worker.so
    if dir := os.Getenv("ENCV_LIB_DIR"); dir != "" {
        candidate := filepath.Join(dir, "libffmpeg-worker.so")
        if _, err := os.Stat(candidate); err == nil {
            return candidate, nil
        }
    }
    return "", fmt.Errorf("libffmpeg-worker.so not found in ENCV_FFMPEG_WORKER or ENCV_LIB_DIR")
}
```

**B. 加 .encv_hb heartbeat 写入**

```go
// runAndHeartbeat runs worker, writes heartbeat file after each call.
func runAndHeartbeat(ctx context.Context, args []string, heartbeat string) (Result, error) {
    result, err := workerRun(ctx, args)
    if heartbeat != "" {
        _ = os.WriteFile(heartbeat, []byte(strconv.FormatInt(time.Now().UnixMilli(), 10)), 0644)
    }
    return result, err
}
```

**C. 父进程加 hard timeout + SIGKILL 双保险**

```go
cmd := exec.CommandContext(ctx, workerPath)
if timeout > 0 {
    timer := time.AfterFunc(timeout+500*time.Millisecond, func() {
        if cmd.Process != nil {
            _ = cmd.Process.Kill()  // SIGKILL — 必杀
        }
    })
    defer timer.Stop()
}
```

### 2.3 [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) — heartbeat 路径透传

**A. ServingDir 心跳文件路径**

```go
// 启动时获取 servingDir（mock_generator 在 main.go 里通过 ctx 拿）
heartbeatPath := filepath.Join(servingDir, ".encv_heartbeat")
// 第一次启动时 touch 一次
_ = os.WriteFile(heartbeatPath, []byte(strconv.FormatInt(time.Now().UnixMilli(), 10)), 0644)
```

**B. 每次 ffmpeg 调后写心跳**（executeMockSpec 末尾）

```go
// 在 executeMockSpec 调 ffmpeg.RunWithOutput 成功后
if mp.heartbeatPath != "" {
    _ = os.WriteFile(mp.heartbeatPath, []byte(strconv.FormatInt(time.Now().UnixMilli(), 10)), 0644)
}
```

**C. timeout 改 5s**（用户已点过"30s 掩盖问题"）

```go
const executeTimeoutMs = 5000
```

### 2.4 [EncvGoService.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) — 文件 mtime 探活

**A. 心跳文件路径解析**

```kotlin
// 优先用 ENCV_SERVING_DIR（mock_generator 心跳文件位置），fallback 缓存目录
private val heartbeatFile: File by lazy {
    val dir = System.getenv("ENCV_SERVING_DIR") ?: System.getenv("ENCV_CACHE_DIR")
        ?: filesDir.absolutePath
    File(dir, ".encv_heartbeat")
}
```

**B. 替换 Process.isAlive() 监控为 mtime 探活**

```kotlin
// 替换 [EncvGoService.kt:145-181](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt#L145-L181) 的 startProcessAliveMonitor
private fun startProcessAliveMonitor() {
    monitorExecutor.scheduleWithFixedDelay({
        if (goProcess == null || !goProcess!!.isAlive) {
            handleDeadProcess("process_not_alive")
            return@scheduleWithFixedDelay
        }
        // 探活：mtime 必须 < 8s 前（8s = 5s ffmpeg timeout + 3s 容差）
        val mtimeMs = heartbeatFile.lastModified()
        val ageMs = System.currentTimeMillis() - mtimeMs
        if (mtimeMs == 0L) {
            // 心跳文件还没建 → 进程刚启动或 mock_generator 没跑过 ffmpeg → 跳过
            return@scheduleWithFixedDelay
        }
        if (ageMs > 8_000) {
            handleDeadProcess("heartbeat_stale_${ageMs}ms")
        }
    }, 1, 1, TimeUnit.SECONDS)
}

private fun handleDeadProcess(reason: String) {
    val reasonFull = "go_hang:$reason"
    publishFailure(reasonFull, "alive_monitor", null)
    try { goProcess?.destroyForcibly() } catch (_: Throwable) {}
    scheduleRestart(1L shl restartAttempts.coerceAtMost(2))  // 1s/2s/4s
}
```

**C. 启动 Go 进程后立即 touch 心跳文件**（避免 8s 内 mtime=0 误判）

```kotlin
// 在 startGoProcess() 末尾，goProcess.start() 之后
File(heartbeatFile.parent).mkdirs()
heartbeatFile.writeText(System.currentTimeMillis().toString())
```

**D. 已有 `startProcessAliveMonitor` 删除/重写**——避免与 mtime 探活冲突。

### 2.5 [AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) — 错误透传增强

**A. onBackendStatus 加 hang 性质识别**

```typescript
const source = raw.startsWith('go_exit') ? 'mockGenerate'
  : raw.startsWith('go_hang') ? 'mockGenerate'      // 🆕 hang 归 mockGenerate
  : raw.startsWith('heartbeat_stale') ? 'mockGenerate'  // 🆕 显式标识
  : raw.startsWith('timeout') ? 'mockGenerate'
  : raw.startsWith('no_binary') ? 'loadPlugins'
  : 'loadPlugins'

const title = raw.startsWith('go_hang')
  ? '后端无响应（hang 8s+），已自动重启'
  : '后端服务已退出'
```

### 2.6 [internal/server/server.go](file:///workspace/internal/server/server.go) — `/health` 不变

**不引入新路由**。文件 mtime 探活完全绕开 HTTP 层。

---

## 3. 关键决策与权衡

### 3.1 为什么是 worker 子进程化（A 方案）而不是其他

| 方案 | 优势 | 劣势 | 决策 |
|------|------|------|------|
| **A. libffmpeg-worker.so 子进程** | 改动最小、保留 Phase 2 设计、cgo 阻塞被 SIGKILL 收尾 | 多 ~30ms spawn 开销 | ✅ 选 |
| B. dlopen/dlclose 每次 | 父进程不阻塞 | ffmpeg.so atexit cleanup 不可靠，每次 +100ms | ❌ |
| C. ffmpeg-cli 完整 binary | 真正解耦 cgo | +30MB APK，无现成 build 脚本 | ❌ 太重 |

### 3.2 为什么是文件 mtime 探活而不是 HTTP 路由

| 方案 | 优势 | 劣势 | 决策 |
|------|------|------|------|
| **文件 mtime** | 零 HTTP 路由冲突、跨进程无状态 | Kotlin 需 file I/O（File.lastModified 是 native 调用，<1ms） | ✅ 选 |
| A. HTTP /health | 标准化 | 已被 [server.go:286](file:///workspace/internal/server/server.go#L286) 占用 | ❌ 用户已否 |
| B. /proc/stat CPU | 不改 Go | ffmpeg IO 等待时 CPU 可能 0，误判 | ❌ |
| C. pipe | 最可靠 | 跨进程 fd 维护复杂 | ❌ |

### 3.3 codec 补完范围（用户已确认"都做"）

- **libmp3lame 3.100**：mp3 encoder，LGPL 2.1 ✅
- **libFLAC 1.4.3 + libogg 1.3.5**：flac encoder，BSD 3-clause ✅
- **alac**：ffmpeg native encoder，零成本，只需 manifest 加 `alac`

### 3.4 已知不变量

1. **不重新设计 Router 抽象**（保持 WorkerRunner / NativeRunner 二选一）
2. **不引入新 HTTP 路由**（文件 mtime 探活）
3. **不修改 ffmpeg.c 源码**（依赖子进程化重置状态）
4. **不引入 gobject 跨进程通信**（stdin/stdout JSON 已够用）
5. **不修改 EncvComboLiteHost 公共 API**

---

## 4. 验证步骤

### 4.1 沙盒验证（CI 可跑）

```bash
# 1. 编译 libffmpeg-worker.so
bash app/encv-mobile/scripts/build-ffmpeg-android.sh
# 预期：脚本末尾出现 "libffmpeg-worker.so 4.2MB" 行

# 2. Go 单测
go test ./internal/utils/ffmpeg/...
# 预期：WorkerRunner.Available()=true（沙箱走 main_exec.go 路径）

# 3. 本地 mock generator
go run ./cmd/encv/ serve &
curl -X POST http://127.0.0.1:2025/api/mock/generate -d '{"root":"/tmp/test"}'
# 预期：22 entries 全 ok，servingDir/.encv_heartbeat 实时更新
```

### 4.2 真机验证（手动）

```bash
# 1. adb logcat -s EncvGoService:* mockGen:*
# 2. APK 启动 → automation test → mock generate
# 预期：
#    - logcat 每 1s 出现 "mtime age: 234ms" 之类
#    - 22 entries 全 ok（mp3/flac/alac 都成功）
#    - 不出现 "go_hang: heartbeat_stale"
# 3. 人为制造 hang（删 libffmpeg.so 重启 app）→ 8s 内应触发 CustomEvent('encv:backend-status') with error "go_hang:..."
#    前端应显示红色 inline error "后端无响应（hang 8s+），已自动重启"
# 4. hang 后 1s/2s/4s 内 Kotlin 自动重启 goProcess
```

### 4.3 回归验证

- 已编过的 libmp3lame 路径（[build-ffmpeg-android.sh](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh) 现存 lame 块）保持原结构
- 已编过的 libx264 路径保持原结构
- aac/alac/flac native encoder（ffmpeg 源码内置）保持 ffmpeg configure 开关即可
- 模拟跑 30 顺序 mock generate 仍 ≥99% PASS

---

## 5. 实施顺序

| Step | 文件 | 改动 | 风险 |
|------|------|------|------|
| 1 | [build-ffmpeg-android.sh](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh) | 末尾新增 libffmpeg-worker.so 编译块 | 低（独立块，错了不影响前面） |
| 2 | [build-ffmpeg-android.sh](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh) | 新增 libFLAC + libogg 编译块 + ffmpeg configure 加 `--enable-libflac` | 中（FLAC 编译与 x264/lame 同模式） |
| 3 | [ffmpeg-feature-manifest.json](file:///workspace/app/encv-mobile/scripts/ffmpeg-feature-manifest.json) | encoders 加 libmp3lame / flac / alac，external_libs 加 libFLAC / libogg | 极低 |
| 4 | [worker_runner.go](file:///workspace/internal/utils/ffmpeg/worker_runner.go) | 加 locateWorker 容错 + heartbeat 写入 + hard timeout | 低 |
| 5 | [mock_generator.go](file:///workspace/internal/server/mock_generator.go) | timeout 5s + heartbeat 写入 + 传 servingDir | 低 |
| 6 | [EncvGoService.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) | 替换 isAlive() 探活为 mtime 探活 + handleDeadProcess | 中（kotlin 逻辑变重） |
| 7 | [AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) | onBackendStatus 加 go_hang / heartbeat_stale 识别 | 极低 |
| 8 | 沙盒 go test + CI 跑 | 端到端验证 | — |

---

## 6. 假设与开放问题

### 假设

1. Kotlin `File.lastModified()` 在 Android 11+ 上是 native call，<1ms，1s 轮询无压力
2. ffmpeg configure 不会因为加 `--enable-libflac` 而破坏现有 `--enable-encoder=aac,alac,...` 列表
3. `cmd/ffmpeg-worker/main_android.go` 当前 cgo dlopen 实现可用，**不需要重写**
4. Worker 子进程 spawn 冷启动 < 50ms（含 dlopen libffmpeg.so 一次）
5. 真机 encoder 补完后 mp3/flac/alac 22 entries 全能跑通

### 开放问题（如出现再回头补）

- 第三方库许可证聚合文件（ffmpeg-kit 退役后没人维护 BSD-3/MIT/LGPL 声明）→ 单独 issue
- alac 真机测试稳定性（m4a 容器 + alac 流的 muxer 兼容性）→ 验证阶段再看
- 探活 mtime 阈值 8s 是否合适（应基于最长一次 ffmpeg 调用时间调整）→ 真机数据出来后调
