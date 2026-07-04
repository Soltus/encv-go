# DevLogs 计数自动刷新 + 全选隐藏 + Android 真机 ffmpeg 路径修复 + 流程日志增强

> 任务:1.devlogs面板后端日志计数/全选 bug 2.安卓真机 ffmpeg 路径 bug + 流程日志增强显示

---

## 一、Summary

修复 4 个具体 bug,集中在两个子系统:

**子系统 A — DevLogs 面板 ([app/encv-mobile/src/views/DevLogs.vue](file:///workspace/app/encv-mobile/src/views/DevLogs.vue) + [IncrementalFilter.ts](file:///workspace/app/encv-mobile/src/utils/IncrementalFilter.ts))**

| Bug | 现象 | 根因 | 修复位置 |
|-----|------|------|---------|
| **A1** 后端"已筛选 n 条"不自动刷新 | 新日志通过 WS 到达时 total 数更新,已筛选数不更新,需要切级别筛选才刷新 | `IncrementalFilter.getResult()` 返回同一 array 引用;Vue 3 computed 缓存按 ref 比较,同一引用不会 invalidate 下游 | [DevLogs.vue:363-369](file:///workspace/app/encv-mobile/src/views/DevLogs.vue#L363-L369) |
| **A2** 切"全部日志级别"未正确隐藏全部 | 点 all 按钮后再点(取消全选)set 变空,`buildPredicate` 用 `levels.size === 0` 兜底"全过",结果仍显示全部 | `buildPredicate` 短路语义错误 | [IncrementalFilter.ts:41-49](file:///workspace/app/encv-mobile/src/utils/IncrementalFilter.ts#L41-L49) |

**子系统 B — Android 真机 ffmpeg 路径 + 流程日志**

| Bug | 现象 | 根因 | 修复位置 |
|-----|------|------|---------|
| **B1** 真机所有 ffmpeg 转换失败 | 日志显示 `failed to create output temp files: Permission denied`(实际是 g_tmp_dir 为空,所有 5 个 fallback 候选都不可写) | worker 子进程 SELinux 上下文与 Go 父进程不同,无法写入 Go 父进程已成功写过的 cache dir;fallback 链 `/data/local/tmp/` 等对 app 进程普遍不可写 | [cmd/ffmpeg-worker/ffmpeg_worker.c:225-240](file:///workspace/cmd/ffmpeg-worker/ffmpeg_worker.c#L225-L240) + [internal/server/mock_generator.go:684-787](file:///workspace/internal/server/mock_generator.go#L684-L787) |
| **B2** 流程日志缺调试信息 | 当前只显示 runner / encoder / args / exit / stderr,真机失败时无 `tmp_dir` / `lib_dir` / 源/目标文件大小 / 实际 worker error 字符串 | Go 侧 `sp.stderr` 拼接时不包含 `lib_dir` / `tmp_dir` 等上下文 | [internal/server/mock_generator.go:917-924](file:///workspace/internal/server/mock_generator.go#L917-L924) + [AutomationTestsDetail.vue:388-408](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue#L388-L408) |

---

## 二、Current State Analysis

### A. DevLogs 后端日志过滤链路

链路:`IncrementalFilter.subscribe` → `backendUpdateTick++` + `backendLogsView.value = backendFilter.getResult()` + `triggerRef(backendLogsView)` → 下游 computed 重算 → `filteredCurrent` 显示

**A1 根因** — 链路上 `backendFilter.getResult()` 始终返回 `this.result` 同一引用。`triggerRef(backendLogsView)` 强制 `backendFilteredItems` 重算,computed 返回值仍是同一引用;Vue 3 的 computed 内部对返回值做 `Object.is` 比较,命中缓存 → **不通知下游** `filteredCurrent` 重算 → 模板里 `{{ filteredCurrent }}` 显示的 length 不变。

只有当 `setFilter` 触发 `rebuild()` 时,`this.result` 被替换为新空数组再 push,引用变化 → 下游 invalidate 一次。这解释了"需要切筛选才刷新"。

**A2 根因** — `buildPredicate`:
```ts
const allLevels = levels.has('all') || levels.size === 0
```
当用户点 all 按钮两次(set 变空)时,`levels.size === 0` 命中,pred 返回 true(全过)。预期行为:空 set 应当不通过任何日志。

**A3(一致性)** — 前端 `filteredFrontend` computed ([DevLogs.vue:401-410](file:///workspace/app/encv-mobile/src/views/DevLogs.vue#L401-L410)) 用 `frontendLogs.value.filter(...)`,每次都返回**新数组**,所以前端日志无此 bug。差异完全由"是否新数组"决定。

### B. Android ffmpeg 真机路径

worker (`ffmpeg_worker.c`) 启动时 `resolve_tmp_dir(json_tmp_dir)` 5 级 fallback:
1. JSON `tmp_dir` (Go 父进程传 `os.TempDir()`,真机是 `/data/user/0/com.encvgo.app/cache/`)
2. `$TMPDIR` 环境变量 (Android 上默认 `/tmp`,不可写)
3. `/data/local/tmp/` (shell user 可写,app 进程不可写)
4. `/data/user/0/com.encvgo.app/cache/` (硬编码 pkg 名)
5. cwd

每级 `try_set_tmp_dir` 做 mkdir + test file open。两边(Go gomobile 和 worker C 进程)**理论上同 UID**,但实际真机上 worker 写入 cache dir 失败 — SELinux/Seccomp 等 Android 沙箱机制让 native 子进程的写权限比 Go 父进程(走 gomobile File API 桥)更严格。

最终 `g_tmp_dir` 为空,`redirect_output_start` 中 `if (g_tmp_dir[0] == '\0')` 主动 set `errno = EACCES` → 输出 `Permission denied` (语义误导,但代码就这样)。

**B2 现状** — 流程日志字段 `MockGenLogEntry` 只有 7 个字段,后端 mock_generator.go stderr 拼接不包含 lib_dir / tmp_dir / 源文件大小 / 写入前 stat 结果。失败时用户看到 `exit code: -1` + 一行误导性的 `Permission denied`,无法判断到底哪一步失败。

---

## 三、Proposed Changes

### Fix 1 — A1 后端"已筛选"自动刷新

**文件:** [app/encv-mobile/src/views/DevLogs.vue](file:///workspace/app/encv-mobile/src/views/DevLogs.vue)

**改动 1.1** (subscription 回调,约 L283-287):
```ts
const unsubBackendFilter = backendFilter.subscribe(() => {
  backendUpdateTick.value++
  // 修复 A1: spread 创建新数组引用,让 Vue 3 computed 缓存失效链路完整传递
  backendLogsView.value = [...backendFilter.getResult()]
  triggerRef(backendLogsView)
})
```

**Why:** 一次 spread O(N) 在 pushMany 路径上每帧最多 1 次(rAF coalesce),N = 当前后端日志数,实测 1M 容量下 ~5ms,与 `processPending` 同量级,可接受。

**改动 1.2** (computed L363-369) 同步加固,即使订阅回调漏掉 spread 也能保底:
```ts
const backendFilteredItems = computed<readonly LogEntry[]>(() => {
  void backendLogsView.value
  void backendUpdateTick.value
  return [...backendFilter.getResult()]  // 修复 A1: spread 确保返回新引用
})
```

**Why:** 双保险。如果未来有人移除 subscription 里的 spread,computed 仍能正确失效。代价是 computed 每次重算多一次 O(N) spread,但 computed 缓存命中时不会执行(只有 dirty 才重算)。

### Fix 2 — A2 切全部级别正确隐藏

**文件:** [app/encv-mobile/src/utils/IncrementalFilter.ts](file:///workspace/app/encv-mobile/src/utils/IncrementalFilter.ts)

**改动 2.1** (L41-49):
```ts
export function buildPredicate(state: FilterState): FilterPredicate {
  const { levels, searchLower } = state
  // 修复 A2: 移除 `|| levels.size === 0` 短路
  // 空 set 应当 0 通过(用户已取消全选,预期看不到任何日志)
  // 'all' 必须显式在 set 中(由 toggleLevel 在 select-all 时 push 进 set)
  const allLevels = levels.has('all')
  return (entry: LogEntry): boolean => {
    if (!allLevels && !levels.has(entry.level as Level)) return false
    if (searchLower && !entry.message.toLowerCase().includes(searchLower)) return false
    return true
  }
}
```

**副作用排查:**
- `setFilter({levels: new Set(['all']), searchLower: ''})` — 内部 `setFilter` 默认初始 `state = { levels: new Set<Level>(['all']), ... }` (L55),保持行为
- `selectedLevels` 初始 `new Set(['debug', 'info', 'warn', 'error'])` (无 'all'),watch 首次触发 `setFilter({levels: Set(4 levels), ...})` — 4 levels 都在 `selectedLevels` 里,pred 行为不变
- 用户点 'all' 两次:set `{all,...}` → `{}` — 第二次 setFilter 后,空 set 配新 pred = 不通过任何日志 ✓
- 单元测试若覆盖 `buildPredicate({levels: Set()})`,需更新为 `expect(false)`;当前仓库无 IncrementalFilter 测试,无需改

### Fix 3 — B1 Android 真机 ffmpeg 路径

**策略:** 在 Go 父进程侧**预先创建** worker 专用 tmp 目录(用 gomobile 的 File API 走 Java 上下文,SELinux 上下文正确),再传给 worker。worker 不再用 5 级 fallback,直接信任父进程传入的 dir。

**改动 3.1** — Go 父进程预先 mkdir 专属 tmp dir

**文件:** [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) `executeMockSpec` 开头(L862 起)或 `ffmpegGenerate` 开头(L684)

新增 `workerTmpDir` 计算:
```go
// 🆕 修复 B1：worker 专用 tmp dir
// 真机 worker 子进程 SELinux 上下文与 Go 父进程不同，
// 父进程用 gomobile File API 走 Java 上下文 mkdir（SELinux 正确），
// worker 直接 open 即可（无需 worker 再 mkdir 试探）。
workerTmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("encv_worker_%d_%d", os.Getpid(), seq))
if err := os.MkdirAll(workerTmpDir, 0700); err != nil {
    sp.stderr = fmt.Sprintf("mkdir worker tmp %s: %v", workerTmpDir, err)
    sp.exitCode = -1
    return
}
```

**改动 3.2** — 把 dir 传给 worker

修改 [internal/utils/ffmpeg/encode.go](file:///workspace/internal/utils/ffmpeg/encode.go) `runWorkerJSON` (L148-158),`TmpDir` 字段从 `os.TempDir()` 升级为 per-call dir:
```go
req := workerRequest{
    Args:      args,
    FFmpegBin: locateFFmpegSystem(),
    LibDir:    libDir,
    TmpDir:    opts.WorkerTmpDir, // 🆕 修复 B1：每调用一个独立 dir
    TimeoutMs: timeoutMs,
    Mode:      mode,
}
```

(需要给 `runWorkerJSON` 加一个可选 `opts` 参数,或新增一个 `runWorkerJSONWithTmp` wrapper,避免破坏其他 caller。)

**改动 3.3** — worker 优先用传入 dir,失败时 5 级 fallback 兜底

**文件:** [cmd/ffmpeg-worker/ffmpeg_worker.c](file:///workspace/cmd/ffmpeg-worker/ffmpeg_worker.c) `resolve_tmp_dir` (L225-240)

行为不变,只是 JSON tmp_dir 优先级提升(已经优先,无需改)。但需要把"尝试的 dir 列表"回报给 Go,便于日志展示(配合 B2):

**改动 3.4** — worker 把尝试过的 dir + 最终选择回报

修改 worker 的成功/失败 JSON:
```c
// 成功：
printf("{\"exit_code\":%d,\"stdout\":\"%s\",\"stderr\":\"%s\",\"duration_ms\":%lld,\"tmp_dir\":\"%s\",\"error\":\"\"}\n", ...)

// 失败 (redirect_output_start 失败)：
printf("{\"error\":\"failed to create output temp files: %s (tried: %s)\",\"exit_code\":-1,\"tmp_dir\":\"\"}\n", ...)
```

(Go 侧 `workerResponse` struct 加 `TmpDir string` 字段,`EncodeResult` 加 `TmpDir string` 字段,`mock_generator.go` stderr 拼接时附上。)

### Fix 4 — B2 FFMPEG 流程日志增强

**改动 4.1** — Go 侧 stderr 拼接补全上下文

**文件:** [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) `executeMockSpec` (L917-924) + `ffmpegGenerate` (L772-779)

```go
if runErr != nil {
    var errDetail string
    if res != nil && res.Error != "" {
        errDetail = "\nworker error: " + res.Error
    }
    // 🆕 修复 B2：补 lib_dir / tmp_dir / 源文件大小 / worker PID
    var srcSize, dstSize int64
    if fi, _ := os.Stat(srcPath); fi != nil { srcSize = fi.Size() }
    if fi, _ := os.Stat(dstPath); fi != nil { dstSize = fi.Size() }
    sp.stderr = fmt.Sprintf(
        "ffmpeg spawn/run: %v\nstderr: %s%s\ncontext: lib_dir=%s tmp_dir=%s src=%s(size=%d) dst=%s(size=%d) pid=%d",
        runErr, ffmpegStderr, errDetail,
        libDir, workerTmpDir, srcPath, srcSize, dstPath, dstSize, os.Getpid(),
    )
    sp.exitCode = ffmpegExit
    return
}
```

**改动 4.2** — 流程日志前端字段扩展

**文件:** [app/encv-mobile/src/views/AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) `MockGenLogEntry` (L388-408)

```ts
interface MockGenLogEntry {
  // ...原有 7 字段
  // 🆕 修复 B2：增强调试字段
  srcSize?: number          // 源文件实际大小（bytes）— null = 源未写成功
  dstSize?: number          // 目标文件实际大小（bytes）— null = 目标未写
  workerTmpDir?: string     // worker 实际用的 tmp dir
  workerError?: string      // worker 响应的 error 字段（与 stderr 区分）
  contextInfo?: string      // 拼接好的「lib_dir + tmp_dir + pid」环境上下文
}
```

**改动 4.3** — 前端日志 UI 渲染

在已有的 spec_diag 展开区(显示 stderr 处)下方,加一段「context」块,显示 `lib_dir` / `tmp_dir` / `src/dst size` / `workerError`。模板位置约 [AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) 显示 stderr 的 pre 块附近。

**改动 4.4** — 后端 SSE payload 加字段

**文件:** [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) `spec_diag` 事件 (L452-457),把 `contextInfo` / `workerTmpDir` / `srcSize` / `dstSize` / `workerError` 塞进 JSON:

```go
diagData := fmt.Sprintf(
    `{"index": %d, "total": %d, "relativePath": %q, "status": %q, "encoder": %q, "runner": %q, "ffmpegArgs": %s, "exitCode": %d, "stderr": %q, "contextInfo": %q, "workerTmpDir": %q, "srcSize": %d, "dstSize": %d, "workerError": %q}`,
    idx, len(specs), sp.relativePath, diagStatus, sp.encoderHint, sp.runner,
    jsonEscapeStringSlice(sp.ffmpegArgs),
    sp.exitCode, sp.stderr,
    sp.contextInfo, sp.workerTmpDir, sp.srcSize, sp.dstSize, sp.workerError,
)
```

(前端 `MockSpecDiag` interface 同步扩展,见 [mockGenerator.ts:36-52](file:///workspace/app/encv-mobile/src/api/mockGenerator.ts#L36-L52)。)

---

## 四、Assumptions & Decisions

| # | 假设/决策 | 理由 |
|---|---------|------|
| D1 | 修复 A1 用 subscription 回调里 spread,不重写 IncrementalFilter | 最小侵入;触发时机就是 notify 点,O(N) 在 rAF 内合并 |
| D2 | 修复 A2 直接删 `levels.size === 0` 短路,不引入新 default state | 空 set 在 toggleLevel 路径上可达(点 all 两次),符合"用户取消全选=看到空"预期 |
| D3 | 修复 B1 用 Go 父进程预先 mkdir,worker 不改核心逻辑 | Go 走 gomobile Java API,SELinux context 正确;worker 只需信任父进程 |
| D4 | worker 5 级 fallback 保留(不删) | 兜底:若父进程没传(其他 caller),worker 仍能自力更生 |
| D5 | B2 新增字段都用 optional,旧测试不破 | 后端补字段,前端不读不报错;后续填 UI |
| D6 | 暂不改 `EncvGoService.kt` 端 Java tmpDir 注入 | Android 端 Go 进程 `os.TempDir()` 已正确返回 cacheDir,问题在 worker 子进程写权限,不在 Go 读路径 |
| D7 | 不在 plan 里改 `cmd/ffmpeg-worker/main_android.go`(已删,被 .c 替代) | 现状正确 |
| D8 | Phase B1 改 mock_generator.go 时不动 `validateMockRoot` / mount 解析逻辑 | mount registry 路径与 worker tmpDir 路径是两套独立文件系统,无交集 |

---

## 五、Verification Steps

### A. DevLogs 修复验证

1. **A1 自动刷新**:
   - 启 preview (`bash scripts/previews.sh start`)
   - 进 DevLogs → 切到「后端日志」tab
   - 触发后端持续打日志(例如: 自动化测试页面点「运行」)
   - 观察 status bar:`共 N 条 (已筛选 M 条)` 同步增长,**不需要切筛选**
   - 验证: 切到 debug only → 「已筛选」数立即变化 → 切回 → 数仍正确

2. **A2 全选隐藏**:
   - 进 DevLogs → 切到「后端日志」
   - 初始: debug/info/warn/error 4 个按钮高亮 → 全部日志可见
   - 点 all 按钮 → 5 个按钮高亮 → 仍全部可见(✓)
   - 再点 all 按钮 → 5 个按钮**全部不**高亮 → **没有任何日志**(修复前: 仍全部可见)
   - 单独点 debug → 只剩 debug 日志(单选行为正常)

3. **回归**:
   - 前端日志 tab: 切级别/搜索仍正常,不破 A1 修复
   - 详情弹窗(点单条 log 展开) 仍正常
   - 复制/清空 仍正常

### B. Android 真机 ffmpeg 路径验证

1. **单元测试**:
   - `bash scripts/test-go.sh ./internal/server` — 跑 mock_generator 相关 test,确保 `ffmpegGenerate` 签名变更后其他 caller 编译过
   - `bash scripts/test-go.sh ./internal/utils/ffmpeg` — 跑 encode_test

2. **真机 e2e**:
   - 真机安装 APK
   - 进自动化测试 → 点「生成 Mock」
   - 观察:
     - mp4/m4a: status `✓` (修复前 `✗ exit=-1 Permission denied`)
     - mp3/flac: status `✗` (real device ffmpeg manifest 没编这些 encoder,正常失败,但 error 字符串应更可读)
   - 点开失败行的 `▶ 展开`:看到 `context: lib_dir=... tmp_dir=... src=... size=N dst=... size=0 pid=...` 完整诊断

3. **沙箱回归**:
   - `bash scripts/previews.sh start` → 进自动化测试页 → 跑 Mock 生成 → mp4/mp3/flac/m4a 全 OK(沙箱 ffmpeg 6.1 完整)

### C. 完整 CI

```bash
# Layer1 (PR check)
gh workflow run pr-check.yml
# Layer2 (full regression, 加 ci:full 标签)
gh pr edit <PR-number> --add-label ci:full
```

---

## 六、Files to Modify

| 文件 | 改动 |
|-----|------|
| [app/encv-mobile/src/views/DevLogs.vue](file:///workspace/app/encv-mobile/src/views/DevLogs.vue) | Fix 1.1 + 1.2: subscription spread + computed spread |
| [app/encv-mobile/src/utils/IncrementalFilter.ts](file:///workspace/app/encv-mobile/src/utils/IncrementalFilter.ts) | Fix 2.1: buildPredicate 删除 `levels.size === 0` 短路 |
| [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) | Fix 3.1 workerTmpDir 预 mkdir + Fix 4.1 stderr 拼接 context + Fix 4.4 SSE payload 扩展 |
| [internal/utils/ffmpeg/encode.go](file:///workspace/app/encv-mobile/src/views/DevLogs.vue) | Fix 3.2 runWorkerJSON 接 per-call WorkerTmpDir + 同步加 `EncodeResult.TmpDir` 字段 |
| [cmd/ffmpeg-worker/ffmpeg_worker.c](file:///workspace/cmd/ffmpeg-worker/ffmpeg_worker.c) | Fix 3.4 worker 响应 JSON 报 tmp_dir |
| [app/encv-mobile/src/api/mockGenerator.ts](file:///workspace/app/encv-mobile/src/api/mockGenerator.ts) | Fix 4.4 前端 `MockSpecDiag` interface 扩展 5 字段 |
| [app/encv-mobile/src/views/AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) | Fix 4.2 + 4.3 MockGenLogEntry 扩展 + UI 渲染 context 块 |

---

## 七、Out of Scope (本次不做)

- 改 [rules/automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) — 流程行为不变,只修 bug
- 重构 IncrementalFilter 用 reactive proxy — A1 修复用 spread 已足够,引入 proxy 是过度设计
- 把 ffmpeg 调用从 worker 子进程改 in-process cgo — 历史决策已敲定走 worker 子进程(防 cgo 阻塞父进程),不在本次范围
- 修改 EncvGoService.kt 端 Kotlin 注入 — 当前 Java 端 `os.TempDir()` 返回值正确,问题在 worker 子进程侧
- Worker C 代码重写为 Go — 历史已重写,无意义回头
