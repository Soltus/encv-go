# 跨进程 IPC 重构 — 行业对比与决策依据

> 本文档为 [spec.md](./spec.md) 的支撑材料，详述为什么选 HTTP localhost、为什么不选文件 mtime、为什么不选 Unix socket、为什么不选 env var 协商。

---

## 一、为什么"共享文件 mtime"是反模式

### 1.1 它看起来是这么工作的

```
Kotlin (parent)                          Go (child)
       │                                      │
       │  启动时 touch heartbeat file         │
       │ ───────────────────────────────────▶│
       │                                      │  每 2s: 更新 mtime
       │  1s poll lastModified()              │
       │ ◀──── mtime: 12:00:02 ──────────────│
       │ ◀──── mtime: 12:00:04 ──────────────│
       │ ◀──── mtime: stale (>8s) ────────────│
       │  判定 hang → kill Go                 │
```

**表面问题**：实现简单，无网络栈依赖。

### 1.2 它实际是这么工作的

```
Kotlin (parent)                          Go (child)
       │                                      │
       │  启动时 touch <filesDir>/.heartbeat  │
       │ ─────┐                               │
       │      ▼                               │
       │  <filesDir>/.encv_heartbeat         │
       │                                      │
       │  Go 写到 <servingDir>/.heartbeat     │
       │      ┌──────▶ ──不同文件──┐         │
       │      │                     │         │
       │  ❌ Kotlin 看不到 Go 写    │         │
       │  ❌ 8s 后误判 hang         │         │
```

**真实问题**：
1. **路径协商**：两个进程必须事先**知道**同一个文件路径 → 必有"路径 A vs 路径 B" bug
2. **权限依赖**：写在共享存储（Android）需要权限；写在 app 私有目录 Go 不知道
3. **mtime 精度**：FAT32/exFAT 精度 2s（Android 共享存储），ext4 精度 1s
4. **时间漂移**：系统时钟回拨（NTP 校时）mtime 倒退 → 误判
5. **文件系统失败**：磁盘满、inode 耗尽 → mtime 不更新 → 误判
6. **没有 ack**：child 写完不知道 parent 看到了没

### 1.3 它违反了什么原则

| 原则 | 违反方式 |
|------|---------|
| **Single Source of Truth** | 路径分散在 config、env、文件、代码 4 处 |
| **Strongly Typed Contract** | 文件 mtime 是隐式契约，编译期无法检查 |
| **Failure Isolation** | 文件系统失败 = 整个 IPC 失败 |
| **Observable** | mtime 看不到 child 真实状态（hang? busy? idle?）|

---

## 二、为什么"env var 协商路径"是反模式

### 2.1 它看起来优雅

```bash
# Kotlin set
ProcessBuilder.environment()["ENCV_HEARTBEAT_PATH"] = "/data/data/pkg/files/.encv_heartbeat"
ProcessBuilder.environment()["ENCV_SERVING_DIR"] = "/storage/emulated/0"

# Go 读
path := os.Getenv("ENCV_HEARTBEAT_PATH")
```

**表面问题**：声明式、显式、好调试。

### 2.2 它实际带来的复杂性

1. **Schema 漂移**：env var 列表是隐式 API，没人能列出"Go 端实际读哪些 env"
2. **不可见**：Go 端 `os.Getenv` 散落各处，新人不知道哪些是 parent 注入的
3. **无验证**：env var 类型错误（数字写成字符串）只有运行时发现
4. **无默认值**：env 没设时 Go 必须有 fallback chain（4 层 fallback 就是这么来的）
5. **跨进程边界模糊**：env 是 parent → child 单向，child 改 env parent 看不到
6. **Android 限制**：Android 11+ scoped storage 让 env 里的路径可能不可用

### 2.3 env var 适合的场景

| 适合 | 不适合 |
|------|--------|
| 配置开关（`ENCV_MOBILE=1`）| 路径协商 |
| 简单字符串（`HOME=...`）| 结构化数据 |
| 单向 parent → child | 双向状态同步 |
| 启动期固定值 | 运行时变化值 |

---

## 三、为什么"双向改写配置文件"是反模式

### 3.1 它看起来是配置集中

```json
// config.user.json — 单一来源！
{
  "mobile": { "server": { "dir": "/storage/emulated/0" } }
}
```

### 3.2 它实际的真相

1. **谁拥有？** Kotlin 写、Go 读 → 不是 single source，是 **shared mutable state**
2. **Schema 耦合**：Kotlin 必须知道 Go 的 config schema（`mobile.server.dir` 字段名）
3. **持久化副作用**：Kotlin 改 config → 下次启动行为变化 → 用户卸载重装才能重置
4. **并发风险**：Go 启动期 read config，Kotlin 同时 write config → 读到不一致状态
5. **调试噩梦**：config 内容取决于"上次谁改的"

### 3.3 配置文件的正确用法

| 场景 | 正确做法 |
|------|---------|
| 用户偏好（端口、密码）| 配置文件，**Kotlin 拥有** |
| 运行时状态（pid、startedAt）| **内存** + HTTP 端点暴露，**Go 拥有** |
| 跨进程协商 | **HTTP**，不用配置文件 |
| 设备差异（Android vs Desktop）| **默认值**写 config，**不要动态改** |

---

## 四、行业方案详细对比

### 4.1 Android Studio ↔ Gradle Daemon

**架构**：
- Gradle daemon 监听 `127.0.0.1:随机端口`（daemon 自己选）
- 启动后写 `~/.gradle/daemon/<version>/registry.bin`（**daemon 主动写**）
- AS 读 registry 找 daemon

**关键设计**：
- ✅ **Child 写，Parent 读**：daemon 写 registry，AS 读
- ✅ **Child 拥有路径**：daemon 自己决定监听哪个端口，自己写文件
- ✅ **HTTP 备用**：daemon 提供 HTTP API
- ❌ **不用 parent → child 改 registry**

**对我们的启发**：
- ✅ Go 端写 `state.json`（**Go 拥有**），Kotlin 读
- ❌ Kotlin 改 `config.user.json`（Kotlin 不应该改 Go 的运行时路径）

### 4.2 VS Code ↔ Language Server

**架构**：
- LSP server 启动后监听 stdio 或 socket
- VS Code 用 LSP 协议（JSON-RPC）通信
- 协议层有完整的 `initialize`、`shutdown`、`exit` 通知

**关键设计**：
- ✅ **协议化**：JSON-RPC，schema 强制
- ✅ **状态机**：`initialize` → `initialized` → 工作 → `shutdown` → `exit`
- ✅ **能力声明**：server 启动时声明自己支持哪些 LSP method

**对我们的启发**：
- ✅ Go 启动后通过 HTTP 端点**声明**自己支持什么、当前状态
- ✅ 加 `GET /api/runtime` 端点返回 server 自描述（pid、servingDir、startedAt）
- ❌ 不要用 stdio pipe（Kotlin EncvGoService 是 Android Service，不是 LSP）

### 4.3 Firebase CLI ↔ Emulator

**架构**：
- emulator 启动时监听 `127.0.0.1:<port>`（命令行 `--port` 指定或自动选）
- emulator 自己写 metadata 到 `~/.cache/firebase/emulators/<name>-<port>.json`
- CLI 读 metadata 拿端口

**关键设计**：
- ✅ **CLI 只读 metadata**：emulator 拥有 metadata 文件
- ✅ **HTTP API**：emulator 暴露 REST API
- ✅ **Health 端点**：`GET /` 返回 200 即 ready

**对我们的启发**：
- ✅ Go 启动后**主动写** `${runtimeDir}/state.json`（**Go 拥有**）
- ✅ Kotlin 读 `state.json` 拿端口、servingDir
- ✅ `GET /health` 即 ready 信号
- ❌ Kotlin 不写 metadata

### 4.4 Docker CLI ↔ dockerd

**架构**：
- dockerd 监听 Unix domain socket `/var/run/docker.sock`（或 Windows named pipe）
- CLI 用 REST API over Unix socket 通信
- 没有配置文件协商

**关键设计**：
- ✅ **Unix socket 而非 TCP**：本地 IPC 更快、更安全
- ✅ **REST API**：HTTP 语义
- ✅ **零配置**：socket path 是约定的

**对我们的启发**：
- ✅ Android 上用 `127.0.0.1:<port>`（不用 Unix socket，因为 Android 限制）
- ✅ HTTP REST API
- ❌ 不要在 Kotlin 端维护 socket path 文件

### 4.5 Flutter CLI ↔ Dart VM Service

**架构**：
- Dart VM 启动后监听 `127.0.0.1:<port>/<auth-token>/`
- VM 启动时**打印** `Observatory listening on http://127.0.0.1:NNNN/auth=...` 到 stdout
- CLI 解析 stdout 拿 URL

**关键设计**：
- ✅ **stdout handshake**：child 启动时**主动报告**自己的地址
- ✅ **带 auth token**：URL 里有 token 防未授权访问
- ✅ **WebSocket upgrade**：observatory 协议在 HTTP 之上

**对我们的启发**：
- ✅ Go 启动后向 stderr/stdout 打印 `Runtime API: http://127.0.0.1:2025/api/runtime`（方便 CLI 解析）
- ✅ Kotlin 解析 stdout 拿 runtime URL（备用方案）
- ❌ 不要用环境变量传 URL

### 4.6 本项目内先例：preview-gateway

**架构**（[app/preview-gateway/src/children.ts](file:///workspace/app/preview-gateway/src/children.ts)）：
- 启 encv-go 时设 `cmd`、`args`、`env`、`cwd`、`readyUrl`（**HTTP GET URL**）
- gateway 1s poll `readyUrl` 直到 2xx
- 失败即崩 gateway（让 pm2 重启整套）

**为什么这是对的**：
- ✅ **child 自己选择端口**：env var `ENCV_SERVER_PORT` 注入端口，Go 选 → CLI 知道
- ✅ **HTTP /health 探活**：标准做法
- ✅ **零文件依赖**：无 heartbeat file、无 state.json 协商
- ✅ **env 极简**：只有 `PATH`、`ENCV_SERVER_PORT` 等配置开关，无路径协商

**对我们的启发（最重要）**：
- ✅ **Kotlin 应该复用同款模式**：Go 启动后，Kotlin 1s poll `/health` 直到 ready
- ✅ **不需要 heartbeat file**：HTTP /health 已经包含 liveness 信息
- ❌ **Kotlin 不应该发明新机制**（heartbeat file、env var 协商路径）

---

## 五、HTTP localhost 的 4 个关键优势

### 5.1 强类型契约

```go
// /api/runtime 响应
type RuntimeInfo struct {
    PID         int    `json:"pid"`
    ServingDir  string `json:"serving_dir"`
    Port        int    `json:"port"`
    // ...
}
```

vs. 文件 mtime（隐式 long 类型）：

```kotlin
val mtime = heartbeatFile.lastModified()  // 啥意思？单位？超时阈值？
```

### 5.2 可观测

| 探查内容 | HTTP 端点 | 文件 mtime |
|---------|-----------|-----------|
| 进程是否活着 | `GET /health` 200 | 文件 mtime 最近 |
| 启动时间 | `started_at` 字段 | mtime 但单位混乱 |
| 真实 servingDir | `serving_dir` 字段 | 猜 |
| 是否 ready | `status: ok` | 不知道 |
| 心跳延迟 | `heartbeat_age_ms` 字段 | mtime - now 算 |
| 进程退出原因 | 日志 / exit code | mtime 不动了 |

### 5.3 失败隔离

| 失败场景 | HTTP | 文件 mtime |
|---------|------|-----------|
| 网络抖动 | HTTP 失败 → 重试 | mtime 仍更新（Go 写文件没失败）|
| 磁盘满 | Go 端 fallback | mtime 不更新 → 误判 hang |
| 权限拒绝 | Go 端 fallback | Go 写失败 → mtime 0 → 误判 |
| 进程 hang | HTTP 1s 失败 → 判 hang | mtime 不更新 → 8s 后判 hang（慢）|

### 5.4 跨平台一致

| 平台 | HTTP localhost | 文件 mtime |
|------|---------------|-----------|
| Android | `127.0.0.1:2025` | `/sdcard` 不可写 / `/data/data/pkg/files` 私有 |
| Linux | `127.0.0.1:2025` | 任何目录 |
| macOS | `127.0.0.1:2025` | 任何目录 |
| Windows | `127.0.0.1:2025` | `%TEMP%` 等 |
| 沙箱 | `127.0.0.1:2025` | /workspace |

**HTTP 在所有平台一致，文件路径处处不同。**

---

## 六、备选方案为什么没选

### 6.1 Unix domain socket（Linux/macOS 性能更好）

**优点**：
- 比 TCP loopback 更快（少 1 次握手）
- 文件系统权限控制

**缺点**：
- Android 不支持（SELinux 限制）
- Windows 用 named pipe，API 不一致
- 路径仍需协商（要不就硬编码 `/tmp/encv.sock`）

**结论**：跨平台一致性更重要，HTTP localhost 够用。

### 6.2 stdio pipe（VS Code 风格）

**优点**：
- 零网络栈依赖
- 启动时自动建立

**缺点**：
- Kotlin EncvGoService 用 `ProcessBuilder.redirectErrorStream(true)` 把 stderr 合到 stdout → Kotlin 用 BufferedReader 读 stdout 找 "ready" 关键词
- LSP 协议完整但 overhead 大
- 字符串协议易碎

**结论**：当前 stdout 关键词匹配已经是这个模式，但不够结构化。HTTP 是更结构化的"stdio"。

### 6.3 SharedPreferences / DataStore（Kotlin 端）

**优点**：
- Android 原生
- 跨进程访问（MODE_MULTI_PROCESS）

**缺点**：
- Kotlin 自己用，不是 Go 用的
- 不能跨平台（Desktop dev 用不到）
- 不解决 parent → child 路径协商

**结论**：Kotlin 内部状态可以用 SharedPreferences，跨进程通信不能用。

### 6.4 gRPC over localhost

**优点**：
- 强类型契约
- 性能好

**缺点**：
- 需要 protobuf 编译器
- Kotlin + Go 都要 gRPC 库
- overkill（小项目）

**结论**：当前规模不需要。如果未来 10+ 端点再考虑。

---

## 七、最终方案总结

| 协调场景 | 重构前 | 重构后 |
|---------|--------|--------|
| Go 启动时 | Kotlin 等 stdout 关键词（"ready"）| Kotlin 1s poll `GET /health` |
| 心跳探活 | Kotlin poll 文件 mtime | Kotlin poll `GET /health`（返回 `heartbeat_ok`）|
| 拿 servingDir | Kotlin 改 `config.user.json.mobile.server.dir` 让 Go 读 | Kotlin 调 `GET /api/runtime` 拿 Go **实际在用**的路径 |
| 拿端口 | Kotlin 探 `configPort..configPort+10` 端口 | Kotlin 1s poll `GET /health` 任意端口（`MAX_PORT_SCAN` 仍保留）|
| 启动超时 | Kotlin 等 stdout 关键词或 health 200 | 同左，**机制不变** |
| Go 进程退出 | Kotlin 读 stderr 推 UI | 同左，**机制不变** |
| Go hang 检测 | Kotlin poll mtime | Kotlin poll `GET /health`，`heartbeat_ok=false` 连续 N 次判 hang |

**净效果**：
- Kotlin EncvGoService.kt 净减约 100 行
- Go 端净增约 50 行（`/api/runtime` 端点 + 内存心跳）
- 删 5 个耦合点（2 env var + 1 file + 2 配置文件字段）
- 跨平台一致：Android / Linux / macOS / Windows / 沙箱 都用同款 HTTP 探活
- 调试体验大幅提升：`curl :2025/api/runtime | jq .` 一眼看穿
