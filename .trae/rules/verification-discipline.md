# 验证纪律规则（Verification Discipline）

> 来自踩坑：曾把 `io.github.lnzz123` 包对应的真实库（com.combo.core.*）幻觉为「Hi-Sillot/ComboLite」仓库，并对不存在的 URL 发起 WebFetch 阻塞等待。此规则强制以下验证纪律。
>
> **完整内容 + 沙箱下载源 + gomobile 类型映射 + Guard self-test 范式 + 实战反例**：[详情文档](../rule-library/verification-discipline.md)

## 一、铁律：核实先于生成

**任何"我以为我知道"的命名（库名 / 仓库 / URL / 包名 / 类名）必须先验证，再用于回复。**

### 1.1 触发"验证"的操作前提

出现以下任一情况，**禁止直接生成**：

- 引用第三方库的包名/类名/仓库
- 给出 GitHub URL / docs URL
- 引用外部 API 的方法签名
- 推荐 npm/pip/maven 坐标
- 引用 CI 输出"我记得是这样"

### 1.2 验证顺序（**本地优先**）

```
1️⃣  Grep / Glob / Read   搜本地仓库（最快、零网络）
2️⃣  cargo test / go test  跑实际构建/单元测试
3️⃣  webfetch + websearch  仅在 1+2 拿不到时；用且只用一次；带短超时
4️⃣  生成结论
```

### 1.3 禁止的反模式

- ❌ 凭印象写出「Hi-Sillot/ComboLite」然后 WebFetch 该 URL（用户原话："根本没有 Hi-Sillot/ComboLite 这个库，哪来的幻觉？"）
- ❌ 给出 Maven 坐标时写 `androidx.core:core-ktx`（无版本）却不验证是否能被 BOM 解析
- ❌ 引用一个方法名而不读 source 验证它存在
- ❌ 用 WebSearch「也许能找到」式发散查询

## 二、本地工具速查（优先用这些）

| 想做 | 用这个 | 备注 |
|------|--------|------|
| 找类/函数/常量定义 | `Grep -n` | 1ms |
| 找文件位置 | `Glob` | 1ms |
| 读源码 | `Read` | 1ms |
| 读 CI 日志 | `grep` 本地日志副本 | 不要 WebFetch |
| 验证方法签名 | `Read` 源文件 | 必做 |
| 看依赖坐标 | `Read libs.versions.toml` | 不要 WebFetch |
| 验证库存在 | `Grep` 仓库 + toml | 不要 WebSearch |
| 验证 URL 可达 | `WebFetch` 1 次 + 短超时 | 30s 兜底 |

## 三、CI 失败诊断纪律

### 3.1 找到失败点的流程

```
0. 先看 0_build.txt 的最后 200 行（Post job cleanup / Cache saved → 说明 build 成功）
1. 找 "FAILED" / "exit code 1" / "BUILD FAILED" 行
2. 上溯 30 行，看 "What went wrong" / "Caused by"
3. 再下溯看 Exception stacktrace
4. 把失败定位到具体 task + dependency + source line
```

### 3.2 不要在没读 log 前臆测根因

**正确做法**：

- 先 `grep -n "FAIL\|Error\|Could not" 0_build.txt`
- 定位行号 → `Read` 那个区段
- 找到根因再改

> 完整 4 类错误处理流程 + 错误反模式表 → [详情文档 §五](../rule-library/verification-discipline.md#五错误处理流程用户已踩过的坑)

## 四、强制 checklist

- [ ] 库名/包名/类名——本地 Grep 得到吗？
- [ ] 方法签名——Read 过源文件吗？
- [ ] URL——本地 Grep 验证过吗？
- [ ] 依赖坐标——`libs.versions.toml` 有版本吗？
- [ ] WebFetch——本地真没信息才用吗？
- [ ] WebSearch——补盲区，不是验证已知？
- [ ] CI 日志——定位到 task + line 了吗？

任何一项打勾失败 → 停下来用本地工具补足。

## 五、沙箱可下载范围

> 沙箱里网络不全通，盲目 `curl <github-release>` / `curl <google-cdn>` 常 timeout。**先测后下**。

| 可达 ✅ | URL | 用途 |
|--------|-----|------|
| Maven Central | `repo1.maven.org/maven2/` | JVM 库 |
| npm registry | `registry.npmjs.org/` | node |
| Gradle Plugin Portal | `plugins.gradle.org/m2/` | Gradle 插件 |
| Gradle dist | `services.gradle.org/distributions/` | Gradle 二进制 |
| GitHub 主页 | `github.com` | 浏览/API |

| 阻断 ❌ | 根因 |
|--------|------|
| `objects.githubusercontent.com` (CDN) | 沙箱代理阻断 |
| `github.com/.../releases/download/` | 同上 |
| `dl.google.com/dl/android/maven2/` | Google CDN 阻断 |
| `download.jetbrains.com/kotlin/` | JetBrains CDN 阻断 |

**拿 Kotlin 编译器标准做法**：

```bash
bash /workspace/.trae/scripts/setup-kotlinc.sh   # 推荐：自动读 toml + 拉 4 个 jar
```

**禁止**：`curl GitHub releases/...` / `curl download.jetbrains.com/...` / `curl dl.google.com/...`

> 手动做法 + setup-kotlinc.sh 详解 + kotlinc 沙箱验证标准流程 → [详情文档 §七.4/§七.5](../rule-library/verification-discipline.md#七沙箱可下载范围分析实战归纳)

## 六、Frontend 改动 commit 前必跑 `vue-tsc --noEmit`

> **v3 教训**：连续 3 轮把功能做完，commit 前只跑 `npx vitest run`（runtime 1006/1009 通过）+ 手动 `curl` 后端，**没跑 `vue-tsc --noEmit`**，前端压了 19 个 TypeScript 编译错误。用户原话：**「每次修改都不检验，更新铁律！」**

```bash
cd /workspace/app/encv-mobile && pnpm exec npx vue-tsc --noEmit 2>&1 | tail -80
```

**退出码 = 0 且无 `error TS****` 行** → 才允许 commit。

### 6.1 8 种 vue-tsc 错误

| 错误码 | 修复 |
|--------|------|
| `TS6133` | 删除未用 import / `_idx` 占位 |
| `TS2339` | 改用真实字段（`containerVersion` 非 `version`） |
| `TS2304` | 加 `import { X } from '@/...'` |
| `TS2305` | 改用 public API |
| `TS2322` | `:detail="false"` 而非 `detail="false"` |
| `TS2345` | `value ?? undefined` |
| `TS2678` | `as` 收窄 union |
| `TS2353` | 改字段名 |

> 完整 19 个错类型（v3 实战）+ 反面教材 + tsconfig deprecation 处理 → [详情文档 §八](../rule-library/verification-discipline.md#八frontend-改动-commit-前必跑-vue-tsc--noemiv3-教训-2026-06-11)

## 七、Guard self-test 铁律（Phase 18 元教训）

> 写好的 guard 也要 smoke test，否则可能比没 guard 还糟。

**任何新增 guard 都必须经过 break-it-back 反向测试**：

```bash
# 1. 正向：干净仓库 → 0 错
bash <guard>   # 期望 EXIT=0

# 2. 反向：故意制造 bug → guard 必须报错
sed -i '/^compose-foundation =/d' android/gradle/libs.versions.toml
bash <guard>   # 期望 EXIT≠0

# 3. 还原 → guard 重新 0 错
```

**checklist**：正向 EXIT=0 / 反向 EXIT≠0 / 回归 0 错 / 路径从 repo root 起 / `wc -l` 验证扫描量。

> Guard A 真实踩坑（find 范围限制导致 silent false-pass）+ 7 步金字塔（CI ↔ Guard A/B/C 反馈链）+ Go 改动三件套 → [详情文档 §七.9](../rule-library/verification-discipline.md#七沙箱可下载范围分析实战归纳)

## 八、Go 改动 commit 前必跑三件套（Phase 19 教训）

> **Phase 19 教训**：以为只改了 Go 格式，commit 前只跑 `gofmt -l` + `go build` 验证，没跑 `go test` → CI 立刻爆 3 个 pre-existing Go test 失败 + 1 个安全 bug。浪费一整次 CI 时间。

```bash
# 1. 格式
gofmt -l ./internal ./cmd ./pkg 2>/dev/null   # 应输出空

# 2. 静态检查
go vet ./internal/... ./cmd/...

# 3. 单元测试
go test ./internal/... ./cmd/... -count=1 -timeout 120s
```

**任何一项不过** → 不允许 commit。

## 九、gomobile Go→Java 命名 + 类型映射铁律（历史）

> **⚠️ 已废弃：本项目不再使用 gomobile。保留此节供历史参考。**
>
> **`ForceDBSync` → `forceDBSync`**（gomobile `lowerFirst` 只动首字符，保留 `DBSync` 全大写子词）。

| Go 类型 | Java 类型 | 备注 |
|--------|-----------|------|
| `int32` / `rune` | `int` | 32-bit |
| **`int`** | **`long`** | Go `int` 是 64-bit → Java `long` |
| `int64` | `long` | |
| `bool` | `boolean` | |
| `string` | `String` | |

**铁律**：gomobile 类型映射**必须**查 `golang.org/x/mobile/bind/genjava.go`，不能凭 Java 类型知识类推。

> Phase 17 ForceDBSync 实战 + Phase 21 onProcessExit Long 误判 + Go→Java 完整映射表 + 沙箱 `javap -p` 自检脚本 → [详情文档 §七.8](../rule-library/verification-discipline.md#七沙箱可下载范围分析实战归纳)

> 拆分：2026-06-11
