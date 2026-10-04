# Android 模拟器"真机"测试通路

> 目标：在没有真机的情况下，拿到**与真机等价的证据**（Android 文件系统/权限/mount 语义、
> 真实后端解密流、真实浏览器解码）。
> 环境事实与全部结论均来自 2026-09-30 ~ 2026-10-01 在本开发容器内的实测，
> 逐条记录见 `.codebuddy/memory/2026-10-01.md`。

## 1. 环境的硬事实（先接受，再设计）

| 事实 | 证据 | 影响 |
| --- | --- | --- |
| 无 `/dev/kvm`，CPUID 不暴露 vmx/svm | `emulator -accel-check`、`docker info` rootless | 只能 QEMU **TCG** 软件翻译，模拟器内比宿主慢约 **90×** |
| APK 内 WebView（Chromium 133）初始化即崩 | logcat：`F/crashpad … CRASHPAD MINIDUMP` + `Fatal signal 5 (SIGTRAP) SI_KERNEL`，`pc=0`、无 tombstone | **模拟器内做不了 UI 级真机测试** |
| Go 后端在模拟器里完全正常 | `GOOS=android GOARCH=amd64 ./cmd/encv` push 进去，`--help` 正常 EXIT=0 | 后端侧的 Android 语义**可以**真机验证 |

结论：**把真机验证拆成两半**——后端跑在模拟器里（Android 语义等价），前端跑在宿主机真实
Chromium（替代会崩的 WebView），用 `adb forward` 打通。这就是"混合通路"。

## 2. 三条通路与对应闸门

| 通路 | 脚本 | 断言 | 当前状态 |
| --- | --- | --- | --- |
| **① 后端契约**（主力，快、稳） | `scripts/emu-backend-check.sh` | `/ping`、mount 语义、目录列举、全量解密**逐字节**、Range 首段、**中间偏移 seek**、失败路径不静默 200 | ✅ 绿 |
| **② 端到端播放** | `scripts/hybrid-e2e.sh`（编排 ① + 浏览器）<br>`scripts/hybrid-play.ts` | ① + `/stream` 206 + `<video>` 真实解码 + `currentTime` 前进 + 无错误卡片 | ✅ 绿 |
| **③ APK 真机冒烟** | `scripts/emu-smoke.sh` | 安装 + AOT + 冷启动 → 进程存活、到前台、无崩溃/ANR | ⚠️ **预期红**（已知环境限制：WebView 崩溃）。2026-10-01 本次**未执行**：现编 debug APK 时 gradle 卡在 maven 限流重试（见下方 §5 坑 9） |

> 通路 ① 一接上就抓出了一个**真 bug**（不是环境问题）：`/stream` 对小文件的 HTTP Range
> **静默失效** —— 响应头写 `bytes N-M/size`，实体体却从文件头开始返回。详见 §7。

③ 现在是红的，这正是它的价值：环境一旦改善（有 KVM / 换 WebView / 换系统镜像），它会自己转绿，
不需要人肉判断。判红时会打印命中原因，并区分"app 代码问题"与"环境限制"。

## 3. 一键跑法

```bash
emuctl start                       # 无头启动（冷启动 5~15 分钟，会话内保持常开）
bash scripts/hybrid-e2e.sh         # 自包含：缺样例现造、缺 dist 现构建、跑完自清
bash scripts/hybrid-e2e.sh --only-backend   # 只跑 ①（秒级，不需要 dist/浏览器）
bash scripts/emu-smoke.sh --build           # APK 冒烟（缺 APK 时现编 x86_64 版）
```

`hybrid-e2e.sh` 的退出码：0=全绿，1=有断言失败，2=用法/前置错误。
证据落盘：`/tmp/emutest/hybrid-player.png`、`/tmp/emutest/smoke-*.png`、
`/tmp/emutest/smoke-logcat.txt`。

### 手工分步（排查时用）

```bash
adb forward tcp:12025 tcp:2025                      # 模拟器内后端 :2025
bash scripts/emu-backend-check.sh --api http://127.0.0.1:12025 \
     --path /d/primary/out/sample.4pm.sccgv --plain /tmp/src/sample.mp4
```

## 4. emuctl（`/usr/local/bin/emuctl`，真源 `.ide/bin/emuctl`）

新增的两个能力是「无 KVM 下能不能跑起来」的关键（此前靠人肉敲命令，容易漏）：

- `emuctl tune`：放宽 AM 各类超时（`activity_start_timeout=600s` 等）、关无线/蓝牙扫描、
  关动画、`setenforce 0`。**`start`/`wait` 末尾会自动调一次**，幂等。
  不放宽超时 → Zygote fork + ART 初始化 > 10s → `Killing <pid>: start timeout`。
- `emuctl install <apk>`：装完**自动 AOT**（`cmd package compile -m speed -f`）。
  不做 AOT → ART 走解释/JIT，冷启动必超时（实测系统 Settings 启动 47s 超时 → AOT 后 **489ms**）。
  不需要就加 `--no-aot`。
- `emuctl aot <pkg>`：单独补 AOT。

改完 `.ide/bin/emuctl` 后，运行期同步：`install -m 755 .ide/bin/emuctl /usr/local/bin/emuctl`
（镜像构建期由 `.ide/Dockerfile` 的 `COPY` 完成）。

## 5. 已固化的坑（别再踩）

1. **`adb shell "pkill -f <模式>"` 会挂起**：模式会匹配到会话自身；后台进程持有 stdout 时
   `adb shell` 不返回。两条都要 `timeout` + `nohup` + 重定向。
2. **primary mount 的 root 取自 `mounts.json`，不是 config 的 `mobile.server.dir`**。
   所以容器虚拟路径必须现算（`/api/mounts` → primary 的 `resolved_root`），
   写死 `/d/primary/out/...` 换环境后会静默失效。`hybrid-e2e.sh` 已现算。
3. **`ERR_BLOCKED_BY_ORB` 常常是假象**：后端把路径解析错 → 404 text/plain → 被 ORB 拦。
   先 `curl` 看真实状态码，别直接判 CORS。
4. **放宽 `service_start_foreground_timeout_ms` 对 `startForeground` 类 ANR 无效**：
   要靠应用自己在 `onStartCommand` 主线程首行调 `startForeground`。
5. **arm64-only 的 `.so` 在 x86_64 模拟器上要走 Berberis 翻译层**（还会崩）。
   正确做法是编 x86_64 原生：`EMU_X86_64=1`（Gradle `abiFilters` 追加 `x86_64`）
   + `android-common.sh build-go --abi x86_64`。
6. **模拟器别反复冷启动**：一次 5~15 分钟；快照（`adb emu avd snapshot`）在 TCG 下加载失败
   （Error -22），不可依赖。
7. **前端伺服必须是薄反代，不能是纯静态**（真机上页面就是 Go 后端自己提供的，前端用相对路径
   `/stream`）：纯静态伺服会把 `/stream` 兜底成 index.html（Bun 甚至因 Range 返回 206），
   播放器拿到 HTML 当视频解 → 必然"播放失败"。
8. **pnpm 11.28 默认供应链策略会拒装**：`ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION`
   （lockfile 里有包发布不足 24h）。本机（已装过 node_modules）绕过：`pnpm install --trust-lockfile`
   （**`npm_config_*` 环境变量无效，必须走 CLI 参数**）；只想出 dist 就直接
   `./node_modules/.bin/vite build`。**别为它改 CI 脚本**。
9. **现编 APK 会卡在 gradle 依赖解析**（maven 镜像限流 → `ErrorHandlingModuleComponentRepository`
   里 sleep 重试，静默几十分钟无输出；`jstack` 可取证）。所以 `emu-smoke.sh` 最好用**现成 APK**
   （`--apk <path>`），别指望 `--build` 一次成功。

## 6. 环境前置（容器重启后要重装的两样）

```bash
apt-get install -y ffmpeg                      # 生成样例容器（fMP4 视频）
pnpm install --trust-lockfile                  # pnpm 11.28 默认供应链策略会拒装（见下方坑 7）
```

## 7. 本通路抓到的真 bug：小文件 HTTP Range 静默失效（已修，2026-10-01）

**现象**：`/stream?path=…` 带 `Range: bytes=N-…` 时，状态码 206、
`Content-Range: bytes N-M/44966` 都正确，**但返回的字节是文件头**（偏移 0 起）。
全量请求（无 Range）逐字节正确，所以"能播放/能下载"的测试全绿，只有 seek 断言会红。

**根因**：`internal/v2/provider/local_provider.go`。小文件（≤3MB；不可寻址容器 ≤150MB）走
内存缓存分支，`GetReader()` 与 `GetSeeker()` 各 `bytes.NewReader(cachedData)` new 了
**两个独立实例**；而 `ContentHandler.ServeFile` 的顺序是
`reader := GetReader()` → `seeker := GetSeeker()` → `seeker.Seek(start)` → `io.Copy(w, reader)`。
Seek 作用在没人读的那个实例上 ⇒ 等价于 Range 被忽略。

**修法**：新增 `cachedStream()`，两个方法共用同一个 `*cachedReadCloser`。

**为什么旧测试没抓到**：`TestServeFile_SeekableProvider_SeeksCorrectly` 用 mock 时把
**同一个** `bytes.Reader` 同时塞给 `ReaderVal`/`SeekerVal`，恰好绕开了这个坑。
⇒ **教训**："两个方法必须共享同一状态"这类契约，必须用**真实对象**写测试，mock 会把它掩盖掉。

**回归锁**：`internal/v2/handler/content_range_cached_test.go`（真实 LocalFileProvider + 最小 fake），
覆盖小文件缓存分支、多偏移、大文件流式分支、provider 层共享位置四条。先红后绿已验证。

## 8. 断言为什么这么写（反"假绿"）

- 只看 `/stream` 是 206 是不够的：还要**全量逐字节 == 明文**（解密完整性）、
  **中间偏移 seek 字节一致**（拖动/随机读正确性）、**失败路径不返回 200**。
- 只看 `video:playing` 事件是不够的：还要 `<video>.readyState>=2 && !paused`，
  并采样两次 `currentTime` 确认**真的在前进**（否则"加载成功但卡死第一帧"也会绿）。
