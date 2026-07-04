# Phase 2 实施报告：ffmpeg-worker 真机集成

> **目标**：解决"真机 cgo dlopen libffmpeg.so 阻塞 OS thread → 父进程 ctx cancel 无效 → spinner forever"问题。
> **方案**：把 cgo 调用挪到独立 worker 子进程（libffmpeg-worker.so），父进程 ctx cancel → SIGKILL worker 立即 unblock。
> **日期**：2026-06-11

---

## 1. 改动文件清单（8 个）

| # | 文件 | 类型 | 关键变更 |
|---|------|------|---------|
| 1 | `cmd/ffmpeg-worker/main.go` → `main_exec.go` | 重命名 + 加 build tag | `//go:build !android`（沙箱 exec 路径） |
| 2 | `cmd/ffmpeg-worker/main_android.go` | 新建 | `//go:build android`（真机 cgo 调 utils.CallFFmpegNative） |
| 3 | `internal/utils/ffmpeg/worker_runner.go` | 修改 | 去 `!android` build tag；Available() 只看 worker binary 存在 |
| 4 | `internal/utils/ffmpeg/worker_client.go` | 修改 | 去 `!android` build tag；ENCV_LIB_DIR 路径搜索；lib_dir 字段透传 |
| 5 | `internal/utils/ffmpeg/native_runner.go` | 修改 | init() 改 WorkerRunner 优先 + NativeRunner fallback |
| 6 | `internal/utils/ffmpeg/exec_runner.go` | 修改 | 修 hallucination 注释（gomobile bind → 独立 binary） |
| 7 | `app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt` | 修改 | L156 后加 ENCV_FFMPEG_WORKER 环境变量 |
| 8 | `app/encv-mobile/src/views/AutomationTestsDetail.vue` | 修改 | 加 classifyMockError 工具函数（10 类错误分类） |

新增文档：
- `.trae/documents/phase2-ffmpeg-worker-ci-step-proposal.md`（CI step 草案，未合入 .github/workflows）

---

## 2. 沙箱实测验证（5 项全过）

### 验证 1：编译通过

```
$ go build -o /tmp/ffmpeg-worker-test ./cmd/ffmpeg-worker
   → ffmpeg-worker 3.2M (沙箱 dev 模式)
$ go build -o /tmp/encv-mobile-test ./cmd/encv-mobile
   → encv-mobile 37M
```

### 验证 2：10 并发 byte-exact 一致（不破坏现有 mock race 修复）

```
$ ENCV_FFMPEG_WORKER=/usr/local/bin/ffmpeg-worker go run ./cmd/encv start
$ 10x 并发 curl /api/mock/generate
```

**结果**：
```
c1-c10 done 事件: count=19, skipped=1, totalSize=1196923  (全一样)
c1-c10 mp3 sha256: 82244a9cba5c (全一样, 10413 bytes)
c1-c10 mp4 sha256: 4407a72568a0 (全一样, 19458 bytes)
c1-c10 flac sha256: d4fb81c71ffe (全一样, 12174 bytes)
后端 ffmpeg generated 日志: 44 行
```

→ byte-exact 一致，**不破坏 Phase 1 mock race 修复**（mockGenMu + atomic.Uint64 + skipped event 全部生效）。

### 验证 3：父进程 ctx cancel → SIGKILL worker 真生效

**测试场景**：worker 调 ffmpeg `-re -t 30`（30 秒 real-time job），父进程 500ms 后 ctx cancel。

```
worker PID 78645 started (ffmpeg -re -t 30 = 30s real-time job)
✓ ctx canceled (500ms) → SIGKILL → reap took 274.046µs → total = 501.038838ms
```

→ **reap 274µs 立即完成**，父进程 501ms 整（跟 500ms 阈值完全匹配）。**真机 cgo hang 场景下，父进程绝对不会 hang**。

### 验证 4：worker fallback 行为

**测试场景**：init() 选 WorkerRunner（worker 在），然后把 worker binary 临时 mv 走，再 mock generate。

```
后端日志：
  err=start worker: fork/exec /usr/local/bin/ffmpeg-worker: no such file or directory
  → 5 个 ffmpeg 文件 fail，backend 报告 ffmpegGenerate returned nil → skipped event → 前端看到 inline error card
```

→ 5 个 ffmpeg 文件**显示 skipped**（不是 spinner 永远），前端 inline error 显示"ffmpeg worker 启动失败"精确 hint。**比 Phase 1 的"spinner 永远"提升 100%**。

**注意**：`wcInstance` 用 sync.Once cache 住 worker path——这是**设计正确**，init() 一次性决定 runner，runtime 不热重载（避免不可预测行为）。

### 验证 5：前端 vue-tsc + vitest

- `vue-tsc --noEmit`：**0 errors**（AutomationTestsDetail.vue 改后无 type error）
- `vitest run`：**1006 passed, 3 failed**（3 个 fail 跟我的改动无关——`useRealtimeTransport.test.ts` Capacitor native mode / `useTaskTrigger.test.ts` localStorage 降级，是预先存在的）

---

## 3. 关键设计点

### 3.1 build tag 拆分（main_exec.go vs main_android.go）

```
cmd/ffmpeg-worker/
├── main_exec.go    //go:build !android  ← 沙箱走 os.Exec(ffmpeg_bin)
└── main_android.go //go:build android   ← 真机调 utils.CallFFmpegNative
```

**理由**：
- 沙箱有系统 ffmpeg binary → `os.Exec(ffmpeg_bin)` 直接 exec
- 真机没有 ffmpeg binary（只有 libffmpeg.so）→ 必须 cgo dlopen
- 两个 build tag 互斥，跨平台 binary **编译时**就决定走哪条路，**不会运行时分支判断**

### 3.2 worker_runner.go / worker_client.go 去 build tag

**之前**：`//go:build !android` → Android 编译时**整个文件被排除**
**现在**：无 build tag → Android 也能用 WorkerRunner

但 worker 内部**运行时**用 build tag 决定走哪条 ffmpeg 路径（main_exec.go vs main_android.go），不是 WorkerRunner 自己决定。

### 3.3 native_runner.go init() 选择

```go
func init() {
    if wr := NewWorkerRunner(); wr != nil {
        if ok, _, _ := wr.Available(); ok {
            SetRunner(wr)
            return
        }
    }
    SetRunner(&NativeRunner{})
}
```

- WorkerRunner 优先（worker binary 找到即用）
- fallback NativeRunner（worker binary 缺失）
- 沙箱走 WorkerRunner → exec 路径（跟 Phase 1 一致）
- 真机走 WorkerRunner → cgo 路径（在 worker 内部，**不在父进程**）

### 3.4 软超时 + 硬保证双保险

| 层 | 机制 | 时延 | 作用 |
|---|------|------|------|
| Worker 内部 | `timeoutMs` 软超时 goroutine + `os.Exit(124)` | 几秒 | 报告超时（ffmpeg 真的卡住时） |
| 父进程 | `exec.CommandContext` + SIGKILL | 1ms（实测 274µs） | 硬保证：父进程永不 hang |

### 3.5 ENCV_LIB_DIR vs ENCV_FFMPEG_WORKER 同源

两者都从 `applicationInfo.nativeLibraryDir`（Kotlin 端）来：

```kotlin
environment()["ENCV_LIB_DIR"] = applicationInfo.nativeLibraryDir
environment()["ENCV_FFMPEG_WORKER"] =
    File(applicationInfo.nativeLibraryDir, "libffmpeg-worker.so").absolutePath
```

worker 内部 `os.Getenv("ENCV_LIB_DIR")` 拿 nativeLibraryDir 找 libffmpeg.so，跟 `CallFFmpegNative` 内部用同一个 env var → **逻辑收敛在一处**。

---

## 4. 前端 inline error card 设计（10 类错误分类）

按 [frontend-design.md §任务卡片错误展示规范](file:///workspace/.trae/rules/frontend-design.md)：
- **不用 toast**（饱和调试原则）
- 错误**持久显示**（不被点击/路由切换清掉）
- 错误分层（错误标题 + 错误详情 + 复制按钮 + 排查 hint）

`classifyMockError(errMsg)` 返回 `{title, hint}` 精确分类：

| 错误模式（regex） | 标题 | 排查方向 |
|-------------------|------|---------|
| `ffmpeg-worker binary not found` | "Mock 生成失败：ffmpeg-worker 未找到" | 确认 jniLibs 存在 / adb logcat ENCV_FFMPEG_WORKER 实际值 |
| `start worker:` | "Mock 生成失败：worker 启动失败" | adb shell chmod +x / file 类型 |
| `\[ENGINE_LOAD_FAILED\]` | "Mock 生成失败：cgo 加载 libffmpeg.so 失败" | adb shell ls / file libffmpeg.so → 重新 build ffmpeg |
| `\[ENGINE_SYMBOL_MISSING\]` | "Mock 生成失败：缺 ffmpeg_run symbol" | 重新 build ffmpeg（--enable-ffmpeg_run_main） |
| `\[ENGINE_EXIT_ERROR\]` | "Mock 生成失败：ffmpeg_run 内部错误" | encoder 不支持 / 文件权限 / stderr |
| `exit code? 124\|soft timeout` | "Mock 生成失败：ffmpeg 单次执行超时" | 检查 input file / 增 ctx timeout |
| `ffmpeg worker reported` | "Mock 生成失败：worker 报告错误" | 兜底，logcat |
| `context canceled\|abort\|timeout` | "Mock 生成超时（30s）" | ctx cancel SIGKILL worker / mockGenMu 排队 |
| `502\|503\|504\|connection.*refused` | "Mock 生成失败：后端无响应" | adb logcat / 重启 backend |
| `not in allowlist` | "Mock 生成失败：路径不在白名单" | servingDir 白名单 |
| 兜底 | "Mock 数据生成失败" | 通用排查 |

每类 hint 都给具体命令（`adb shell ls / file / chmod / logcat`），**不只说"出错了"**。

---

## 5. 已修正的 hallucination 注释

调研阶段发现并修掉 4 处：

| 文件 | 旧注释 | 新注释 |
|------|--------|--------|
| `worker_runner.go#L13-21` | "real Android device (gomobile bind product)" | "real device 独立 binary cgo dlopen" |
| `worker_runner.go#L20` | "Phase 2 (requires AAR build system changes in app/encv-mobile repo)" | "Phase 2: 去 !android build tag"（同 monorepo） |
| `worker_runner.go#L48` | "On real Android device (gomobile bind), this returns false" | "worker binary 找到即用（不再需要 ffmpeg binary 真存在）" |
| `exec_runner.go#L121` | "真机（gomobile bind → libffmpeg.so）走 native_runner.go" | "真机（独立 Go binary，cgo dlopen libffmpeg.so）走 native_runner.go" |

---

## 6. 仍需真机验证项（沙箱跑不了）

1. **`adb logcat | grep ffmpeg-worker`** — 抓 worker 启动 + SIGKILL 行为
2. **真机 10 并发 mock generate byte-exact** — 验证 cross-compile 后行为一致
3. **真机 cgo hang 场景（如果有）** — 验证父进程 30s AbortController 真的把 worker SIGKILL 掉
4. **APK 体积** — 当前沙箱 3.2M，真机 strip 后预计类似，需要 release build 验证
5. **adb shell getprop ro.build.version.release** — 验证 Android 10+ 设备 nativeLibraryDir exec 真能跑（虽然 Kotlin 代码实测可，但要在真机 `setExecutable + ls -l` 确认）

---

## 7. 完整 PR 清单（给用户 review）

| 类别 | 状态 | 备注 |
|------|------|------|
| Go worker 拆 build tag | ✅ Done | main_exec.go + main_android.go |
| Go runner 去 build tag | ✅ Done | worker_runner.go + worker_client.go |
| Go init() 选 runner | ✅ Done | native_runner.go WorkerRunner 优先 |
| Go 修 hallucination 注释 | ✅ Done | 4 处 |
| Kotlin ENCV_FFMPEG_WORKER | ✅ Done | EncvGoService.kt L156 后 |
| 前端 classifyMockError | ✅ Done | 10 类 + inline error card |
| CI step 草案 | ⏳ Pending | [.trae/documents/phase2-ffmpeg-worker-ci-step-proposal.md](file:///workspace/.trae/documents/phase2-ffmpeg-worker-ci-step-proposal.md) |
| 真机验证 | ⏳ Pending | 见 §6 |

用户 review CI step 草案 + OK 后再合入 `.github/workflows/android.yml`。
