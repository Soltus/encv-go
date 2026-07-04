# Trae Web 沙箱网络行为

> 诊断时间：2026-05-27 ~ 2026-06-10 | 环境：CI=true, non-interactive sandbox
>
> **完整内容 + 实测诊断 + 401 根因 + mock 浏览器日志规范**：[详情文档](../rule-library/trae_web_sandbox_network.md)

## 一、核心结论

| 进程 | 出站 | MCP 代理 (`:18080`) | 能否拉依赖 |
|------|------|---------------------|-----------|
| **curl** / wget | ✅ 白名单放行 | — | ✅ |
| **Node.js** | ❌ TCP 拦截 | ✅ HTTP CONNECT（undici 自动走） | ✅ |
| **Java/JVM** | ❌ TCP 拦截 | ❌ SOCKS 协议不匹配超时 | ❌ |

**沙箱架构**：

```
curl/wget ─▶ 出站白名单 (放行)
Node.js   ─▶ NODE_OPTIONS 注入 EnvHttpProxyAgent ─▶ :18080 HTTP CONNECT ─▶ 外网
Java/JVM  ─▶ http_proxy env → SocksSocketImpl(SOCKS) → :18080 (协议不匹配) → 超时
                                                                          └─▶ 直连(被拦)
```

**关键事实**：
- DNS 正常（maven.aliyun.com 12 个 IP）
- Java `ProxySelector` 返回 `DIRECT`，TCP 连接超时
- 任何 JDK 版本都受影响（17.0.2 / 21.0.2 / 25.0.2 全 fail）
- **`env -i` 清空 env** → 直连被沙箱拦截（**无法绕过**）

## 二、自动注入环境变量

```bash
http_proxy=http://127.0.0.1:18080
https_proxy=http://127.0.0.1:18080
HTTP_PROXY=http://127.0.0.1:18080
HTTPS_PROXY=http://127.0.0.1:18080
no_proxy=localhost,127.0.0.1,.svc,.cluster.local,::1
NODE_OPTIONS=--require /app/mcp_proxy_bootstrap/preload.cjs   # undici 自动用 HTTP CONNECT
PREVIEW_PROXY_PUBLIC_PORT=16000
```

**`env -i` 也清不掉**（沙箱强制注入）。

> Node.js 注入机制 + Java 失败根因链 + DNS 特殊情况 → [详情文档 §三](../rule-library/trae_web_sandbox_network.md#三自动注入的环境变量)

## 三、Maven 仓库可达性（curl 测试）

| 仓库 | 状态 |
|------|------|
| Maven Central (`repo.maven.org`) | ✅ 200 |
| Aliyun Google / Central / Gradle Plugin / Public | ✅ 200 |
| Tencent Public / Maven-Tencent | ✅ 200 |
| `maven.google.com` | ✅ 200 |
| Gradle Plugin Portal | ✅ 200 |

**特殊**：`repo.maven.org` 偶尔 NXDOMAIN，建议优先用国内镜像。

## 四、对构建的影响

| 场景 | Java 网络 | Gradle 构建 |
|------|----------|------------|
| **GitHub Actions CI** | ✅ | ✅ |
| **Trae Web 沙箱本地** | ❌ | ❌ |

### Go 构建（mise 管理）

- `go.mod` 要求 **Go 1.25.1** → `mise.toml` 必须 `go = "1.25.1"`
- 编译必须：`cd /workspace && mise exec -- go build ./cmd/encv/`
- ⚠️ **不要**直接 `go build`（可能用系统旧版本）

> 沙箱内 Java 构建方案 A/B/C + Gradle 配置细节 → [详情文档 §五](../rule-library/trae_web_sandbox_network.md#五对构建的影响)

## 五、CDN 阻断清单（**禁止**直连）

> 沙箱的出站 TCP 走白名单，但**部分主流 CDN 即使能 DNS 解析也会被 404 / 阻断**。盲目 `curl <github-release>` 经常卡 30s+ 然后 exit 28 (timeout)。**先看这张表再用 curl**。

| 源 | 状态 | 替代 |
|----|------|------|
| `objects.githubusercontent.com` (GitHub Release CDN) | ❌ 404 | **走 gh-proxy.com 镜像**（18 MB / 10s OK） |
| `github.com/.../releases/download/` | ❌ 404 | `gh-proxy.com` / `mirror.ghproxy.com` / `ghfast.top` |
| `dl.google.com/dl/android/maven2/` | ❌ 404 | `maven.google.com` / 阿里云 Google 镜像 |
| `download.jetbrains.com/kotlin/` | ❌ 404 | Maven Central `kotlin-compiler-embeddable` |
| `services.gradle.org/distributions/` | ✅ 200 | 已是上游源 |

**GitHub Release 镜像**：`gh-proxy.com` / `mirror.ghproxy.com` / `ghfast.top`（实测 18 MB / 10s OK）。用法：`curl -fsSL --max-time 60 "https://gh-proxy.com/<原 URL>" -o /tmp/file.tar.gz`。**注意**：不要用 `https://ghproxy.com/...`（缺 `-`）会 404。

**拉二进制决策树**：

```
1. 有 Maven 坐标? → Maven Central
2. 是 Gradle?     → services.gradle.org
3. 是 npm?       → npm registry
4. 是 Go module? → proxy.golang.org
5. 是 GitHub Release? → gh-proxy.com 镜像
```

> 识别 CDN 阻断的 3 步诊断（DNS / curl 短超时 / 走 MCP 代理）+ 反模式 → [详情文档 §七](../rule-library/trae_web_sandbox_network.md#七cdn-阻断清单禁止-curl-直连这些源)

## 六、Kotlin 编译器一键拉取

```bash
bash /workspace/.trae/scripts/setup-kotlinc.sh
```

**输出**：6 步全过 + `kotlinc-jvm 2.3.21 (JRE 17.0.2+8-86)` 验证 + 退出码 0。

**8 大设计要点**：自动读 toml 不硬编码 / monorepo 多布局兼容 / 100% 走 Maven Central / `curl --max-time 60` 失败立即 exit 2 / 已有 jar size 合理就 skip / `/usr/local/bin/kotlinc-<version>` wrapper / 跑 `kotlinc -version` 验证 / 退出码 0/1/2/3 分级。

> setup-kotlinc.sh 6 步输出 + 8 大设计要点表 + 沙箱 kotlinc 调试标准流程 → [详情文档 §八](../rule-library/trae_web_sandbox_network.md#八kotlin-编译器一键拉取方案)

## 七、Preview Proxy 路由必须由 OpenPreview 工具注册

> **铁律：起完 dev server 必须调用 `OpenPreview` 工具，否则 16000 代理一律 400。**

**症状**：`curl http://127.0.0.1:16000/openlist-ui/` → 400

**根因**：`agent-tool-host` 在 16000 暴露 preview proxy，但**不会自动发现**沙箱里监听的 dev server 端口——需要 agent 显式调用 `OpenPreview` 工具注册。

**修复**：

```bash
# 1. 起 dev server，拿到 command_id
# 2. 调用 OpenPreview
OpenPreview(command_id="<起服务那条命令的 id>", preview_url="http://localhost:16666/")
# 3. 验证
for path in / /api/service-guard /openlist-ui/; do
  code=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:16000$path")
  echo "  16000$path → $code"   # 全 200 / 302
done
```

**注意事项**：

- **OpenPreview 只能用启动 dev server 的那条命令的 command_id**（非后续 curl ID）
- **代理路由有 session/TTL**——长时间不活动需重新调用
- **多个 dev server 时，OpenPreview 只能注册一个端口**（默认 / 路径）
- previews.sh 是 bash，**没有 OpenPreview 工具能力**——必须 agent 收尾调

> 401 真实源头诊断（3 段证据链）+ 9.1.2 收敛结论 + trae 网关层 401 vs encv-go 业务 401 区分 → [详情文档 §九.1.2](../rule-library/trae_web_sandbox_network.md#九preview-链路实际可达性矩阵2026-06-08-实战实测)

## 八、Preview 链路 3 档调试

| 档位 | 前端 | API | WS | 适合 |
|------|------|-----|----|------|
| **沙箱浏览器（OpenPreview）** | `https://run-agent-...trae.cn/` | ✅ `/api/*` 同源 | ⚠️ 受限 | Files/Tasks/Settings |
| **沙箱本地** | `http://localhost:16666/` | ✅ | ✅ | 完整功能 + DevLogs |
| **APK 真机 + adb reverse** | `capacitor://localhost` | `http://127.0.0.1:2025` ✅ | `ws://127.0.0.1:2025/ws` ✅ | 真机性能最真实 |

> 9.1.1 WS 升级细节 + 9.2 useApiBaseProbe 全失败链路 + 9.3 三方案 (X/Y/Z) + 9.5 链路矩阵 + 9.6 绝对禁止 → [详情文档 §九](../rule-library/trae_web_sandbox_network.md#九preview-链路实际可达性矩阵2026-06-08-实战实测)

## 九、401 区分铁律（**agent 必会**）

> 401 不是 encv-go 也不是 preview-gateway 也不是 agent-tool-host 返的——是 **trae 外网边缘网关**返的。沙箱内 loopback 永远复现不了。

**快速区分**：

| 响应 `content-type` | 来源 | 应对 |
|---------------------|------|------|
| `text/html` | trae 网关（session cookie 缺失 / rate limit） | 用户"请重新登录 trae" |
| `application/json` | encv-go 业务（auth 头缺失） | 按业务 error code 处理 |

**前端规范**：`useApiBaseProbe` 把 content-type + body preview 一起塞进 err，agent 在 mock 浏览器 console 区分是 trae 网关 401 还是 encv-go 业务 401。

> 401 证据链（encv-go 无 401 路径 / gateway 无 auth / agent-tool-host 日志无 401）+ 应对策略 → [详情文档 §九.1.2](../rule-library/trae_web_sandbox_network.md#九preview-链路实际可达性矩阵2026-06-08-实战实测)

## 十、mock 浏览器日志规范（agent 排查必备）

> **沙箱架构硬约束**：用户**只能在 agent-tool-host 提供的模拟浏览器里**预览。该浏览器：✅ 刷新 / ✅ console 日志（DevLogs tab 可见） / ❌ 完整 DevTools / ❌ Network 面板。
>
> → **诊断时只能靠 console 日志**，所有关键路径必须把 trace 打到 console。

**`useApiBaseProbe` 必打日志模板**（`console.info` 不用 debug——DevLogs 默认隐藏 debug）：

| 时机 | 模板 |
|------|------|
| 入口 | `[probe] start origin=<o> cached=<c> force=<f>` |
| 缓存 | `[probe] step [1] try cached: <url>` |
| current origin | `[probe] step [1.5] try current origin: <o>` |
| loopback | `[probe] step [2] try loopback: <url>` |
| 命中 | `[probe] commit baseUrl=<u> source=<s> latency=<L>ms` |
| 全失败 | `[probe] step [4] no candidates available, all-failed` + `[probe] FAIL all-candidates-failed \| trace: ...` |

**错误抛出规范**：throw `Error("all-candidates-failed | trace: " + log.join(" | "))` 把整条 log 数组串成单行。

> 全部日志规范表格（[1] / [1.5] / [2] / [expand] / [lan] / 命中 / 全失败）+ 401 / 502 / WS 各种 case 模板 + agent 必做（看 `[error]` 贴完整堆栈 / 让用户硬刷新 / 不要"开 DevTools"）→ [详情文档 §九.4](../rule-library/trae_web_sandbox_network.md#九preview-链路实际可达性矩阵2026-06-08-实战实测)

## 十一、绝对禁止

- ❌ 假设 OpenPreview 模式下能**完整**功能调试（API 2026-06-10 已修复，WS 仍受限）
- ❌ 让前端 throw 阻塞 `mounted` hook 不带 console.info 兜底
- ❌ 试图注册多个 OpenPreview（agent-tool-host 用最后一次的端口）
- ❌ 让 vite 加回 `/api` proxy（决策 D9 已固化）
- ❌ 在 mock 浏览器里让用户"开 DevTools 看 Network"——**没有 DevTools**
- ❌ 在 encv-go / preview-gateway 加 auth 头绕过 401（401 在它们之前）

> 跨文档引用（verification-discipline §7 / setup-kotlinc.sh / previews.sh / android.md）→ [详情文档 §十](../rule-library/trae_web_sandbox_network.md#十跨文档引用)

> 拆分：2026-06-11
