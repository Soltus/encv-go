# verification-discipline 详情

> 本文件为 [verification-discipline.md](../rules/verification-discipline.md) 的详情文档。
>
> 索引位于 [`.trae/rules/verification-discipline.md`](../rules/verification-discipline.md)。本文件汇总索引未包含的详细反模式、错误处理流程、沙箱可下载源分析、Gradle Kotlin DSL 检测脚本、gomobile 类型映射详解、Guard self-test 范式、实战反例。

---

## 三、WebFetch / WebSearch 红线（完整版）

### 3.1 必须先 Grep 完本地再 WebFetch

**写代码前先做**：

```bash
# 例：想确认 IPluginEntryClass 的包路径
Grep pattern="IPluginEntryClass"  # 拿到真实包路径
Read  <file with import>           # 看实际 import
```

只有本地确实不存在（全新外部库），才允许 WebFetch。

### 3.2 WebFetch 短超时纪律

WebFetch 是无超时/长超时的工具。本地仓库**永远比外网**准确。

```yaml
禁用: WebFetch 一个不熟悉的 GitHub URL 等待返回
替代: Grep 本地看是否已引用；找不到就停下来问用户
```

### 3.3 WebSearch 红线

WebSearch 用于「补完知识盲区」而非「验证已知」。

- ✅ 用 WebSearch 找新 API 文档（CLI 升级了，文档没读）
- ❌ 用 WebSearch 验证我以为的包路径

---

## 四、CI 失败诊断纪律（完整版）

### 4.1 找到失败点的流程

```
0. 先看 0_build.txt 的最后 200 行（Post job cleanup / Cache saved → 说明 build 成功）
1. 找 "FAILED" / "exit code 1" / "BUILD FAILED" 行
2. 上溯 30 行，看 "What went wrong" / "Caused by"
3. 再下溯看 Exception stacktrace
4. 把失败定位到具体 task + dependency + source line
```

### 4.2 不要在没读 log 前臆测根因

**反面教材**（用户原话：「更本没读日志就开始改」）：
- 看到 `androidx.core:core-ktx` 报错就猜是版本问题
- 没读 build/0_build.txt 实际行就脑补「Vite 8 不支持 --prod」

**正确做法**：
- 先 `grep -n "FAIL\|Error\|Could not" 0_build.txt`
- 定位行号 → `Read` 那个区段
- 找到根因再改

---

## 五、错误处理流程（用户已踩过的坑）

| 用户反馈 | 对应反模式 | 正确做法 |
|---------|-----------|---------|
| 「哪来的幻觉」 | WebFetch 想象中的 URL | 先 Grep 本地 |
| 「还 curl 阻塞半天」 | WebFetch 默认阻塞 | 本地工具 < 1s |
| 「用本地工具」 | 默认 WebFetch/curl | Grep / Read / Glob |
| 「用 Read 读实际行」 | 凭印象写代码 | Read 文件确认存在 |
| 「不要在没读 log 前瞎改」 | 跳过日志直接改 | 先 `cat`/`grep` 日志 |

---

## 七、沙箱可下载范围分析（实战归纳）

> 沙箱里的网络并不全通。盲目 `curl <github-release>` / `curl <google-cdn>` 经常 timeout。**先测后下**。
>
> **Kotlin 编译器准备已工程化** → 直接跑 [`/workspace/.trae/scripts/setup-kotlinc.sh`](../scripts/setup-kotlinc.sh) 一键就绪（自动读 `libs.versions.toml`、从 Maven Central 拉 4 个 jar、写 `/usr/local/bin/kotlinc-<version>` wrapper）。

### 7.1 已确认可达的源（Maven 协议族）

| 源 | URL | 用途 |
|----|-----|------|
| **Maven Central** | `https://repo1.maven.org/maven2/` | 任何 JVM 库（Kotlin / AndroidX / ksp 等都镜像到这里） |
| **npm registry** | `https://registry.npmjs.org/` | node 包 |
| **Gradle Plugin Portal** | `https://plugins.gradle.org/m2/` | Gradle 插件 |
| **Gradle distributions** | `https://services.gradle.org/distributions/` | Gradle 二进制 |
| **GitHub 主页** | `https://github.com` | 列表浏览、API |
| **cache-redirector.jetbrains.com** | (slow but reachable) | JetBrains 工具链 |

### 7.2 已确认阻断的源（二进制 CDN）

| 源 | 状态 | 阻断原因 |
|----|------|----------|
| `https://objects.githubusercontent.com` (GitHub Objects CDN) | ❌ 404 | 沙箱代理阻断 |
| `https://github.com/.../releases/download/` (GitHub Releases 下载) | ❌ 404 | 同上 |
| `https://dl.google.com/dl/android/maven2/` | ❌ 404 | Google CDN 阻断 |
| `https://download.jetbrains.com/kotlin/` | ❌ 404 | JetBrains CDN 阻断 |

### 7.3 沙箱里**已经有**的工具（**不要重新装**）

```bash
# 来自 mise 安装
java 17.0.2  → /root/.local/share/mise/installs/java/17.0.2/bin/java
javac 17.0.2
gradle 8.14.4  → /root/.local/share/mise/installs/gradle/8.14.4/gradle-8.14.4/bin/gradle
mvn 3.9.10     → /root/.local/share/mise/installs/maven/3.9.10/apache-maven-3.9.10/bin/mvn

# pnpm 已通过 pnpm/action-setup 装好
pnpm

# 来自系统
apt / apt-get  →  但 apt 仓库**没** kotlin / gradle 包
```

### 7.4 拿 Kotlin 编译器的标准做法

**推荐：一键脚本**（自动读 `libs.versions.toml`、Maven Central 拉 4 个 jar、写 wrapper）：

```bash
bash /workspace/.trae/scripts/setup-kotlinc.sh
```

**禁止**用 `curl GitHub releases/download/.../kotlin-compiler-2.3.21.zip`（会超时）
**禁止**用 `curl download.jetbrains.com/kotlin/...`（沙箱 CDN 阻断）
**禁止**用 `curl dl.google.com/dl/android/maven2/...`（沙箱 CDN 阻断）

**手动做法**（不推荐，仅当脚本失败时备查）：用 Maven 拉 `kotlin-compiler-embeddable`，自建包装脚本：

```bash
# /usr/local/bin/kotlinc-2.3.21
KOTLIN_HOME="/tmp/kotlin-home"
mkdir -p "$KOTLIN_HOME/lib"
for art in kotlin-compiler-embeddable kotlin-stdlib kotlin-reflect; do
    if [ ! -f "$KOTLIN_HOME/lib/${art}-2.3.21.jar" ]; then
        curl -sL --max-time 30 -o "$KOTLIN_HOME/lib/${art}-2.3.21.jar" \
            "https://repo1.maven.org/maven2/org/jetbrains/kotlin/${art}/2.3.21/${art}-2.3.21.jar"
    fi
done
exec java -cp "$KOTLIN_HOME/lib/kotlin-compiler-embeddable-2.3.21.jar:$KOTLIN_HOME/lib/kotlin-stdlib-2.3.21.jar:$KOTLIN_HOME/lib/kotlin-reflect-2.3.21.jar" \
    org.jetbrains.kotlin.cli.jvm.K2JVMCompiler -kotlin-home "$KOTLIN_HOME" "$@"
```

**用法**（与原版 kotlinc 兼容）：

```bash
kotlinc-2.3.21 -version
kotlinc-2.3.21 -Xsuppress-version-warnings -no-stdlib -no-reflect <*.kt> -d /tmp/out
```

### 7.5 用 kotlinc 在沙箱里做语法验证的标准流程

```bash
# 步骤 1: 沙箱语法检查（不用 classpath，仅看 syntax）
cd /path/to/plugin
kotlinc-2.3.21 -no-stdlib -no-reflect -Xsuppress-version-warnings src/main/java/**/*.kt -d /tmp/out 2>/tmp/err.log

# 步骤 2: 过滤"真 bug" vs "缺依赖"两类错误
echo "=== 语法/抽象成员/Composable 错误（真 bug）==="
grep -E "Syntax error|Unclosed comment|Missing '|Expecting token|Unexpected token|abstract member|does not implement abstract|Composable invocations can only happen" /tmp/err.log
# 应该为空——如果不为空,说明 source 有真 bug

echo "=== Unresolved references（缺依赖，不是 source bug）==="
grep -c "unresolved reference" /tmp/err.log
# 数字大但都是预期的（android.*、com.combo.*、openlistlib.*、compose.*）
```

### 7.6 CI 错误 vs 沙箱验证的差异

| 维度 | CI（真） | 沙箱验证（本次） |
|------|---------|---------------|
| 是否有 android.jar | ✅ | ❌ |
| 是否有 combolite-core.jar | ✅ | ❌ |
| 是否有 openlist-classes.jar | ✅ | ❌ |
| 是否有 compose runtime | ✅ | ❌ |
| 能抓出 syntax error | ✅ | ✅ |
| 能抓出 abstract member error | ✅ | ✅（仅当 classpath 完整） |
| 能抓出 unresolved reference | ✅ | ❌（沙箱只能报"有 unresolved"，不能定真伪） |

**沙箱验证的定位**：抓 syntax / parse 错误（CI log 的核心 ~60% 错误都是这一类），然后给 CI 跑全量。**不是**替代 CI。

### 7.7 沙箱也能抓 Gradle Kotlin DSL 脚本编译错误

> **Phase 16 踩坑**：CI 报 `Unresolved reference 'foundation'.`（在 `build.gradle.kts:81-82`），
> Phase 14 修复时加 `implementation(libs.compose.foundation)` 但忘了在 `libs.versions.toml` 声明。
> **Gradle Kotlin DSL 脚本编译期就崩了**，不进入 source 编译期。
>
> **沙箱检测方法**：写一个 `bash` 脚本遍历所有 `build.gradle.kts`，
> 提取 `libs.X.Y` 引用，对照 toml alias（toml `-` → 访问器 `.`），**总失败数应 0**。

```bash
# 1. 提取 toml 全部 alias
TOML_LIBS=$(awk '/\[libraries\]/{flag=1; next} /^\[/{flag=0} flag && /^[a-zA-Z]/' \
    android/gradle/libs.versions.toml | sed -E 's/^([a-zA-Z0-9._-]+).*/\1/')
TOML_PLUGINS=$(awk '/\[plugins\]/{flag=1; next} /^\[/{flag=0} flag && /^[a-zA-Z]/' \
    android/gradle/libs.versions.toml | sed -E 's/^([a-zA-Z0-9._-]+).*/\1/')

# 2. 遍历所有 build.gradle.kts
FAIL=0
for f in $(find . -name "build.gradle.kts"); do
    for ref in $(grep -oE 'libs\.(plugins|[a-zA-Z0-9._-]+)\.[a-zA-Z0-9._-]+' "$f" | sort -u); do
        [[ "$ref" == libs.versions.* ]] && continue
        if [[ "$ref" == libs.plugins.* ]]; then
            alias="${ref#libs.plugins.}"; alias="${alias//./-}"
            echo "$TOML_PLUGINS" | grep -qx "$alias" || { echo "✗ $f: $ref"; FAIL=$((FAIL+1)); }
        else
            alias="${ref#libs.}"; alias="${alias//./-}"
            echo "$TOML_LIBS" | grep -qx "$alias" || { echo "✗ $f: $ref"; FAIL=$((FAIL+1)); }
        fi
    done
done
[ "$FAIL" -eq 0 ] && echo "✅ 所有 libs.* 引用都能在 toml 找到"
```

**根因 / 教训**：
- 任何 `libs.X.Y` 引用都必须有 toml alias 对应（`toml alias 用 '-' 分隔, 访问器用 '.'`）
- 改 build.gradle.kts 加新 `libs.*` 引用时,必须同时改 `libs.versions.toml` 声明
- Gradle Kotlin DSL 脚本编译错误会污染整个 multi-project（**任何子项目配置期失败 → 其他子项目的任务也死**）

### 7.8 gomobile Java 命名规则（**沙箱也能诊断**）

> **Phase 17 踩坑**：CI 报 `Unresolved reference 'forceDbSync'.`（`OpenListBridge.kt:306`），
> 一直猜是 gomobile 暴露了错误的方法名。其实**沙箱里能 100% 验证**：
> 读 gomobile 源码里的 `lowerFirst()` 函数 + 读 fork 实际 Go 函数名 → 直接推导出 Java 名。

**核心规则**（gomobile `cmd/gobind/gen.go:527 lowerFirst`）：

```go
func lowerFirst(s string) string {
    // 逐 rune 走,遇到非大写就停
    // 只 lowercase 第 1 个 rune,后面保留原样
    // 例: "ForceDBSync" → "forceDBSync"   (DBSync 作为一个子词保留)
    // 例: "SetConfigData" → "setConfigData"
    // 例: "IsRunning" → "isRunning"
}
```

**Go 函数名 → Java 方法名 速查**（fork `openlistlib/` 实际导出）：

| Go 函数（PascalCase） | Java 方法（camelCase） | 本项目调用点 |
|----------------------|----------------------|-------------|
| `GetOutboundIP()` | `getOutboundIP()` | 未用 |
| `GetOutboundIPString()` | `getOutboundIPString()` | 未用 |
| `SetConfigData(path)` | `setConfigData(String)` | [OpenListBridge.kt:101](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt) ✅ |
| `SetConfigLogStd(b)` | `setConfigLogStd(boolean)` | 未用 |
| `SetConfigDebug(b)` | `setConfigDebug(boolean)` | 未用 |
| `SetConfigNoPrefix(b)` | `setConfigNoPrefix(boolean)` | 未用 |
| `SetAdminPassword(pwd)` | `setAdminPassword(String)` | [OpenListBridge.kt:330](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt) ✅ |
| `Init(e Event, cb LogCallback)` | `init(Event, LogCallback)` | [OpenListBridge.kt:111](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt) ✅ |
| `IsRunning(t)` | `isRunning(String)` | [OpenListBridge.kt:294](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt) ✅ |
| `Start()` | `start()` | [OpenListBridge.kt:233](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt) ✅ |
| `Shutdown(timeout)` | `shutdown(long)` | [OpenListBridge.kt:269](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt) ✅ |
| **`ForceDBSync()`** | **`forceDBSync()`** | [OpenListBridge.kt:309](file:///workspace/app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt) ✅（Phase 17 修） |

**沙箱自检脚本**（验证任意 Go 名字 → Java 名）：

```bash
# 用 gomobile 源码里的 lowerFirst 跑（直接复用其逻辑）
cat > /tmp/lowerFirst.go <<'EOF'
package main
import ("fmt"; "unicode"; "unicode/utf8")
func lowerFirst(s string) string {
    if s == "" { return "" }
    var conv []rune
    for len(s) > 0 {
        r, n := utf8.DecodeRuneInString(s)
        if !unicode.IsUpper(r) {
            if l := len(conv); l > 1 { conv[l-1] = unicode.ToUpper(conv[l-1]) }
            return string(conv) + s
        }
        conv = append(conv, unicode.ToLower(r))
        s = s[n:]
    }
    return string(conv)
}
func main() { fmt.Println(lowerFirst("ForceDBSync")) }
EOF
go run /tmp/lowerFirst.go
# 期望输出: forceDBSync
```

**特别提醒**：
- `DBSync` / `HTTPClient` 这种**全大写子词**会被保留（`lowerFirst` 只动首字符）
- 写 Kotlin 包装函数时**注意 A2 fallback**：[`build-openlist-aar.sh:381`](file:///workspace/scripts/build-openlist-aar.sh) 只在 fork 缺 `openlistlib/event.go` 时才注入。Hi-Sillot/OpenList@`404daf0` 已自带 event.go（`OnProcessExit(code int)`）→ A2 fallback 被跳过 → gomobile 在 64-bit Android 上生成 `onProcessExit(Long)`（Go `int` 是 64-bit，映射到 Java `long`，见 [genjava.go:117-120](file:///root/go/pkg/mod/golang.org/x/mobile@v0.0.0-20260602190626-68735029466e/bind/genjava.go#L117-L120)）→ Kotlin 端必须写 `code: Long`。**Phase 21 误判为 `Int` 引发 CI 编译失败，已回滚**。Phase 17 风险**仍 OPEN**（机制是 fork 同步 A2 fallback，不是类型修正）。

### 7.8.1 gomobile Go→Java 类型映射铁律（Phase 21 教训）

> **Phase 21 教训**：凭「Java `int` 是 32-bit → Go `int` 也应该是 32-bit」想当然，把 `onProcessExit(code: Long)` 改成 `code: Int`，CI 立刻报 `onProcessExit overrides nothing`，浪费 5 分钟构建时间。

**铁律**：gomobile Go→Java 类型映射**必须**查 `golang.org/x/mobile/bind/genjava.go`，不能凭 Java 的类型知识类推。

**正确映射**（64-bit Android = 我们唯一的目标 ABI）：

| Go 类型 | Java 类型 | 备注 |
|--------|-----------|------|
| `bool` | `boolean` | |
| `int8` | `byte` | |
| `int16` | `short` | |
| `int32` / `rune` | **`int`** | 32-bit Java int，**不会**随平台变 |
| **`int`** | **`long`** | Go `int` 是 64-bit（linux/arm64）→ Java `long` |
| `int64` | `long` | |
| `uint32` | `long` | 无符号加宽到有符号 long |
| `uint64` | `long` | 同上 |
| `float32` | `float` | |
| `float64` | `double` | |
| `string` | `String` | |
| `[]byte` | `byte[]` | |

**速查**（gomobile `bind/genjava.go:117-120` 原文）：
```go
case types.Int16:                     kind = java.Short
case types.Int32, types.UntypedRune:  kind = java.Int
case types.Int64, types.UntypedInt:   kind = java.Long  // ← Go int 在 64-bit 平台走这里
```

**沙箱自检脚本**（验证 gomobile 实际生成的 Java 签名）：
```bash
# 在沙箱里手动跑 gomobile 一次，把 Event.class 反编译看方法签名
cd /tmp && mkdir test-bind && cd test-bind
cat > go.mod <<'EOF'
module test
go 1.22
require golang.org/x/mobile v0.0.0-20260602190626-68735029466e
EOF
cat > event.go <<'EOF'
package test
type Event interface {
    OnProcessExit(code int)
    OnStartError(t string, err string)
}
EOF
go mod tidy
gomobile bind -target=android/arm64 .
unzip -p test.aar classes.jar > classes.jar
javap -p -classpath classes.jar test.Event
# 期望: fun onProcessExit(p0: Long): Unit
#       fun onStartError(p0: String, p1: String): Unit
```

**预防 checklist**（gomobile 改动 commit 前必跑）：
- [ ] 读 `genjava.go` 确认目标 Go 类型的 case 分支
- [ ] 沙箱跑 `javap -p` 看实际生成的 Java 签名（见上）
- [ ] 写 Kotlin 后本地用 Guard B（kotlinc pre-flight）验证能 override

**反面教材**（Phase 21 我的错）：
- ❌ 凭「Java int 是 32-bit」推断 Go int → Java int
- ❌ 看了 K-Sillot/OpenList-Mobile 的 `Long` 后不深究，反而认定它错
- ❌ 用户挑战我时，没主动查 gomobile 源码
- ✅ 正确做法：先 `Read /root/go/pkg/mod/golang.org/x/mobile/.../bind/genjava.go`，再决定类型

### 7.9 守卫也要被 test 验证（Guard self-test discipline）

> **Phase 18 元教训**：写好的 guard 自己也要 smoke test，否则可能比没 guard 还糟。

#### 7.9.1 反面教材（Guard A 盲点）

**事件**：Guard A（TOML alias guard）第一版：

```bash
# ❌ 第一版
for f in $(find android -name "build.gradle.kts"); do
    for ref in $(grep -oE 'libs\.(plugins|[a-zA-Z0-9._-]+)\.[a-zA-Z0-9._-]+' "$f" | sort -u); do
        ...
    done
done
```

**症状**：
- 破坏 `libs.versions.toml`（删 `compose-foundation` 2 行）后，guard **没报错**
- 实际只检查了 22 个 `libs.*` refs，漏掉 14 个
- 漏的是 `plugin-openlist/build.gradle.kts` 和 `plugin-mpv-player/build.gradle.kts`（**不在 `android/` 目录下**，是 `android/` 的 sibling）

**根因链路**：
```
Guard A 期望: 遍历整个 monorepo 的所有 build.gradle.kts
实际遍历:   find android -name "build.gradle.kts"
              ↑ 范围被限制在 android/ 子树
              ↑ plugin-openlist/ 在 app/encv-mobile/ 下, 不在 android/ 下
              ↑ 漏 14/36 个 refs
              ↑ 静默 false-pass
              ↑ CI 仍然在 3 min Gradle 配置期才报 unresolved
```

**为什么 smoke test 没抓到**：smoke test 在 `app/encv-mobile/` 目录下跑，
本身 `cd` 路径就有歧义 → 测了但被 `cd` 逻辑吞了，**没暴露**真正的盲点。

#### 7.9.2 修复：故意坏掉（Break-it-back test）

**任何新增的"自动化守卫"都必须经过「故意坏掉」的反向测试**：

```bash
# === 守卫自检模板 ===

# 1. 在干净仓库上跑 → 期望 0 错
bash <guard-script>
# 预期: "✅ 所有 libs.* 引用都能在 toml 找到" / EXIT=0

# 2. 故意引入已知 bug → 期望抓出
cp android/gradle/libs.versions.toml /tmp/toml.bak
# 删除 toml 中某个 alias (模拟 Phase 16 错)
sed -i '/^compose-foundation =/d' android/gradle/libs.versions.toml
bash <guard-script>
# 预期: "✗ plugin-openlist/build.gradle.kts:81: libs.compose.foundation" / EXIT≠0
# 验证抓到了! ✓

# 3. 还原
cp /tmp/toml.bak android/gradle/libs.versions.toml
bash <guard-script>
# 预期: 0 错 / EXIT=0
```

**对 Guard A 的实际修复**：
```bash
# ✅ 修复后
for f in $(find . -name "build.gradle.kts"); do  # ← 从 repo root, 不限子目录
    for ref in $(grep -oE 'libs\.(plugins|[a-zA-Z0-9._-]+)\.[a-zA-Z0-9._-]+' "$f" | sort -u); do
        ...
    done
done
# 验证: 36 refs (之前 22), 破坏后精准抓 2 错
```

#### 7.9.3 Guard self-test checklist

写完任何 guard（bash 脚本 / GitHub Action / gradle task）后, **必做**：

- [ ] **正向测试**：干净仓库跑 guard → 0 错, EXIT=0
- [ ] **反向测试（必做）**：故意制造 guard 想抓的 bug → guard 必须报错并定位到具体文件 + 行
- [ ] **回归测试**：还原 bug → guard 重新 0 错
- [ ] **路径覆盖测试**：`find` / `grep` 范围从 **repo root** 开始, 不限子目录（除非 guard 明确只针对某子项目）
- [ ] **silent-fail 排查**：guard 跑通但**实际没遍历到目标**也算失败（用 `wc -l` 或计数变量验证扫描量）

#### 7.9.4 守卫与 CI 反馈链的金字塔

```
   ┌──────────────────┐
   │   CI 编译 (3-5min)│   ← 最晚发现, 但最准确
   ├──────────────────┤
   │  Guard B (10s)   │   ← kotlinc pre-flight, 抓真 unresolved
   ├──────────────────┤
   │  Guard A + C (<1s)│  ← grep 静态扫描, 零网络
   └──────────────────┘
   越下越早发现, 越上越准. 守卫 = 把错误往下推, 早发现早治.
```

**关键认知**：guard 不是要替代 CI 编译, 是要把错误**往前推**——从 3 min 推到 1 s。
但 guard **必须**经过 §7.9.3 的 self-test, 否则会把"假绿"伪装成"真绿",
反而**增加**调试时间（你以为有 guard 罩着, 实际它在睡觉）。

#### 7.9.5 历史教训时间线

| Phase | 错类型 | Guard 抓到? | 耗时 |
|-------|--------|------------|------|
| 14 | `dist/* to` 嵌套块注释 | ❌（当时无 Guard B） | CI 4 min |
| 15 | setup-kotlinc.sh 脚本 bug | ❌（脚本本身无 self-test） | 手动排错 20 min |
| 16 | `libs.compose.foundation` 缺 toml | ❌（当时无 Guard A） | CI 3 min |
| 17 | `forceDbSync` 命名错 | ❌（当时无 Guard B） | CI 3 min |
| 17 | `snapshot.running` Map property | ❌（当时无 Guard C） | CI 3 min |
| **18+** | **任何同类** | **✅ Guard A/B/C 抓** | **< 10s** |

#### 7.9.6 反向测试在沙箱里的标准做法

```bash
# === 在 /workspace (CI repo root) 跑 ===

# Guard A 范例
cd /workspace
TOML=app/encv-mobile/android/gradle/libs.versions.toml
[ -f "$TOML" ] || TOML=android/gradle/libs.versions.toml

# 1. 正向
for f in $(find . -name "build.gradle.kts"); do
    refs=$(grep -oE 'libs\.[a-zA-Z0-9._-]+\.[a-zA-Z0-9._-]+' "$f" | sort -u | wc -l)
    [ "$refs" -gt 0 ] && echo "  $f: $refs refs"
done
# 预期: 总 refs = 36 (含 plugin-openlist + plugin-mpv-player)

# 2. 反向
cp "$TOML" /tmp/toml.bak
sed -i '/^compose-foundation =/d' "$TOML"
<run guard>
# 预期: EXIT≠0, 报错至少 1 个 line
# 验证: guard 真能抓!

# 3. 还原
cp /tmp/toml.bak "$TOML"
<run guard>
# 预期: EXIT=0
```

**关键纪律**：
- 沙箱 smoke test **必须从 CI repo root 跑**（不能 `cd app/encv-mobile` 再 `cd app/encv-mobile` 套两层）
- 破坏测试**至少跑 1 次正向 + 1 次反向**, 不能省
- guard 写完 24 小时内必须 self-test, 否则遗忘成本 > 测试成本

#### 7.9.7 guard 写完后的强制 checklist（commit 前）

- [ ] guard 脚本 bash -n 通过（无语法错）
- [ ] YAML 嵌入的 guard 用 `python3 -c "import yaml; yaml.safe_load(open('android.yml'))"` 验证
- [ ] **正向测试**跑过且 EXIT=0
- [ ] **反向测试**跑过且 guard 真报错（不是 silent pass）
- [ ] 路径范围从 repo root 起（不限子目录）
- [ ] 已记录在 `tasks.md` Phase N 的子项
- [ ] **§7.9 引用过**, 元教训已写下来

任何一项没做 → **不允许 commit**。

#### 7.9.8 Go 改动 commit 前必跑三件套（Phase 19 教训）

> **Phase 19 教训**：以为只改了 Go 格式（gofmt 整库），commit 前只跑 `gofmt -l` + `go build` 验证，
> 没跑 `go test` → CI 立刻爆 3 个 pre-existing Go test 失败 + 1 个安全 bug。
> 浪费一整次 CI 时间。

**Go 改动 commit 前必跑**（顺序无关，但全要跑）：

```bash
# 1. 格式
gofmt -l ./internal ./cmd ./pkg 2>/dev/null  # 应输出空
[ $? -eq 0 ] && echo "✅ gofmt"

# 2. 类型 / 静态检查
go vet ./internal/... ./cmd/... 2>&1
[ $? -eq 0 ] && echo "✅ go vet"

# 3. 单元测试
go test ./internal/... ./cmd/... -count=1 -timeout 120s 2>&1 | tee /tmp/gotest.log
[ $? -eq 0 ] && echo "✅ go test"
```

**任何一项不过** → 不允许 commit。这是从"gofmt 整库"commit 把 3 个 pre-existing test 失败 + 1 个安全漏洞带进 CI 后总结的铁律。

**Phase 19 真实战果**：
| 检查 | 之前 | 之后 |
|------|------|------|
| `gofmt -l` | ✅ | ✅ |
| `go build` | ✅ | ✅ |
| **`go test`** | **❌ 漏跑** | **✅ 必跑** |
| `go vet` | ❌ 漏跑 | ✅ 必跑 |

新增的两个 test failure 类型（§7.9 guard 抓不到，因为是运行时不是编译时）：
1. **架构漂移型** — 旧 contract test 期望老 API（PluginManager.isInitialized），代码已重构走新封装（EncvComboLiteHost）
2. **silent-fallback 型** — alist_encrypt plugin 静默把 `.sccgv` 改为 `.bin`，掩盖真实冲突

---

## 八、Frontend 改动 commit 前必跑 `vue-tsc --noEmit`（v3 教训 2026-06-11）

> **v3 教训**：连续 3 轮把「测试报告自动化」「WebDAV Basic Auth」「ECv4 128GB Sparse」做完，commit 前只跑 `npx vitest run`（runtime 1006/1009 通过）+ 手动 `curl` 后端，**没跑 `vue-tsc --noEmit`**，结果前端压了 19 个 TypeScript 编译错误：
> - 5 个未用 import（`ECV4` / `getTaskMetadata` / `TriggeredBy` / `IonButton` / `clearTriggeredBy`）
> - 4 个 string→boolean 误传（`detail="false"` / `spellcheck="false"`）
> - 3 个属性不存在（`task.version` / `_getAllForTesting` / `goSparseContainerTest`）
> - 2 个 type 转换（`TaskStatus` / `TaskType` 没 cast）
> - 2 个 case 不可达（switch 维度不匹配）
> - 1 个 null→undefined（`string \| null` vs `string \| undefined`）
> - 1 个 version 字段名错（`version` vs `containerVersion`）
> - 1 个未声明导出（`_getAllForTesting` 测试 bug）
>
> 用户原话：**「每次修改都不检验，更新铁律！」**

### 8.1 铁律

**任何 `app/encv-mobile/src/` 下的修改(`.vue` / `.ts` / `.tsx`)完成 + commit 前必跑:**

```bash
cd /workspace/app/encv-mobile && pnpm exec npx vue-tsc --noEmit 2>&1 | tail -80
```

**退出码 = 0 且无 `error TS****` 行** → 才允许 commit。

### 8.2 已知会出现的 vue-tsc 错误模式 + 修复铁律

| 错误码 | 含义 | 修复模式 |
|--------|------|---------|
| `TS6133 'X' is declared but its value is never read` | import 进来没用 | 删除 import 项,或在 v-for 用 `_idx` 占位 |
| `TS2339 Property 'X' does not exist on type 'Y'` | 字段名错 | 改用真实字段名（如 `task.containerVersion` 不是 `task.version`）|
| `TS2304 Cannot find name 'X'` | 没 import | 加 `import { X } from '@/constants/...'` |
| `TS2305 Module '...' has no exported member 'X'` | 测试引用了未导出的内部函数 | 改用 public API（如 `getTriggeredBy` 代替 `_getAllForTesting`）|
| `TS2322 Type 'string' is not assignable to type 'boolean \| undefined'` | `detail="false"` 写成 string | 改 `:detail="false"` |
| `TS2345 Argument of type 'string \| null' is not assignable to parameter of type 'string \| undefined'` | `ref<string \| null>` 没 coalesce | `value ?? undefined` |
| `TS2678 Type '"X"' is not comparable to type '"a' \| "b"'` | switch 维度不在 union 中 | 加 `as 'a' \| 'b' \| 'X'` 收窄,或删 case |
| `TS2353 Object literal may only specify known properties` | EncvTask 没 `version` 字段(用 `containerVersion`) | 改字段名 |

### 8.3 反面教材 (v3 真实案例)

```ts
// ❌ 错：v-for 写了 idx 但没用
<ion-item v-for="(testCase, idx) in WEBDAV_TEST_CASES" :key="testCase.id" ...>
  <!-- idx 没用上 → vue-tsc 报 TS6133 -->

// ✅ 对：未用变量前加下划线
<ion-item v-for="(testCase, _idx) in WEBDAV_TEST_CASES" :key="testCase.id" ...>
```

```ts
// ❌ 错：string 误传给 boolean prop
<ion-item button detail="false" ...>  // detail 是 boolean，字符串"false"永远 truthy

// ✅ 对：v-bind to boolean
<ion-item button :detail="false" ...>
```

```ts
// ❌ 错：测试引用未导出的内部 API
import { _getAllForTesting } from '@/composables/useTaskTrigger'
const map = _getAllForTesting()  // TS2305 编译失败

// ✅ 对：用 public API 验证可观察行为
expect(getTriggeredBy('task-599')).toBe('automation')  // 通过行为反推内部状态
```

### 8.4 tsconfig 已知 deprecation

`baseUrl` 在 TypeScript 7.0 会移除,但当前项目 tsconfig 仍用。**这是 warning 不是 error**,
vue-tsc exit 0 + 无 `error TS****` 行就允许 commit。**不需要** 立刻改 tsconfig。

### 8.5 与其它守卫的配合

```
CI (5 min build)
  ↓ 兜底
vitest run (90s)       ← 抓 runtime
  ↓
vue-tsc --noEmit (20s) ← 抓编译期类型,本节铁律
  ↓
go test ./internal/...  ← 抓 Go runtime
  ↓
grep 残留扫描 (< 1s)    ← 抓 hardcode / 残留
```

**每一层都必跑**,任一不过 → 不 commit。

**CI 守卫不会抓运行时 test 失败**（gofmt guard / kotlinc guard / toml guard 都是编译期守卫），
所以沙箱的 `go test ./internal/...` 是**唯一**提前发现这类问题的环节。

> 拆分：2026-06-11
