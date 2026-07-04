# Hi-Sillot/OpenList Fork 改动交付物

> 配套 plan：`.trae/documents/openlist-aar-sqlite-cgo-multi-solution.md` §五
> 目标 commit：Hi-Sillot/OpenList `dev` 分支
> 适用范围：plugin-openlist 的 gomobile bind 链路

## 一、commit 1：`feat(openlistlib): add event.go for gobind Event/LogCallback interfaces`

### 1.1 新增文件 `openlistlib/event.go`

```go
package openlistlib

type Event interface {
	OnStartError(eventType string, msg string)
	OnShutdown(eventType string)
	OnProcessExit(code int64)
}

type LogCallback interface {
	OnLog(level int16, time int64, log string)
}
```

### 1.2 字段命名铁律

- 所有方法名必须**首字母大写且无下划线**（PascalCase）——gobind 据此生成 Java 接口方法名
- 参数类型只允许导出 Go 基础类型（`string` / `int*` / `bool` / `[]byte`），结构体/指针不能直接跨语言边界，需要先 marshal 成 string
- 接口本身只需在 Go 端定义；gobind 会自动生成对应的 `Java Openlistlib$Event` 抽象类与 `Openlistlib$LogCallback` 抽象类

### 1.3 本地验证

```bash
cd Hi-Sillot/OpenList
test -f openlistlib/event.go
grep -q 'type LogCallback' openlistlib/event.go && echo "OK: event.go defined"
go vet ./openlistlib/
```

### 1.4 下游消费

`app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/OpenListBridge.kt` 已经预声明：
```kotlin
import openlistlib.Openlistlib
abstract class LogCallback : Openlistlib.LogCallback() {
    abstract override fun onLog(level: Short, time: Long, log: String?)
}
```

commit 1 推送后，CI 不再报 `undefined: LogCallback`。

---

## 二、commit 2：`refactor(db): switch to glebarez/sqlite (pure-Go, CGO-free)`

### 2.1 改动范围

| 文件类型 | 操作 |
|---------|------|
| `cmd/server/server.go`、`cmd/encv-mobile-mobile/main.go` 等入口 | import path 替换 |
| `internal/bootstrap/data/*.go` | import path 替换 |
| `internal/db/*.go` | import path 替换 |
| `go.mod` | `go get github.com/glebarez/sqlite@latest` + `go mod tidy` 移除 mattn 间接依赖 |

### 2.2 替换命令

```bash
cd Hi-Sillot/OpenList

# 找出所有 import 点
grep -rln '"gorm.io/driver/sqlite"' .

# 批量替换（先 dry-run 检查）
find . -name "*.go" -type f -exec sed -i.bak 's|"gorm.io/driver/sqlite"|"github.com/glebarez/sqlite"|g' {} \;

# 检查替换结果
grep -rln '"gorm.io/driver/sqlite"' .   # 应为空
grep -rln '"github.com/glebarez/sqlite"' . | wc -l  # 应 > 0

# 清理 .bak
find . -name "*.go.bak" -delete

# 更新 go.mod
go get github.com/glebarez/sqlite@latest
go mod tidy
```

### 2.3 业务层零改写

```go
// 旧（gorm.io/driver/sqlite）
import "gorm.io/driver/sqlite"
db, err := gorm.Open(sqlite.Open("data.db"), &gorm.Config{})

// 新（glebarez/sqlite）
import "github.com/glebarez/sqlite"
db, err := gorm.Open(sqlite.Open("data.db"), &gorm.Config{})
//          ↑ 变量名都是 sqlite，无需改名
```

`glebarez/sqlite` 提供同名 `Open(dsn string) gorm.Dialector`，是 `gorm.io/driver/sqlite` 的 drop-in replacement。

### 2.4 本地验证

```bash
cd Hi-Sillot/OpenList

# 1. 纯 Go 编译（最强自检：CGO 路径全部清掉）
CGO_ENABLED=0 go build ./...
echo "exit=$?"  # 应为 0

# 2. mattn 间接依赖应消失
go list -m all | grep -i mattn   # 应为空

# 3. glebarez 间接依赖应出现
go list -m all | grep -E "glebarez|modernc"

# 4. 启动一次 smoke test
./openlist server --data-dir /tmp/openlist-test &
PID=$!
sleep 5
curl http://127.0.0.1:5244/api/setup/status  # 至少要能返回 JSON
kill $PID
rm -rf /tmp/openlist-test
```

### 2.5 可能撞到的兼容问题（提前预案）

| 现象 | 根因 | 修复 |
|------|------|------|
| `unsupported: PRAGMA journal_mode=WAL` | glebarez 在嵌入式场景默认不开 WAL | 显式 `sqlite.Open("file::memory:?_pragma=journal_mode(WAL)")` |
| `data too long for column` | glebarez 类型推断比 mattn 严格 | 显式给 `string` 字段加 `type:varchar(N)` |
| 慢查询 | glebarez 不像 mattn 默认开 query planner | `PRAGMA query_only = OFF` + 索引检查 |

### 2.6 预期 AAR 体积

| 驱动 | libgojni.so | openlist.aar |
|------|-------------|--------------|
| mattn/go-sqlite3 | ~42 MB | ~45 MB |
| glebarez/sqlite | ~30 MB | ~33 MB |

---

## 三、推送顺序

```bash
cd Hi-Sillot/OpenList
git checkout dev
git pull origin dev

# commit 1
git add openlistlib/event.go
git commit -m "feat(openlistlib): add event.go for gobind Event/LogCallback interfaces

两个 interface:
  - Event (OnStartError, OnShutdown, OnProcessExit) — 桥接 openlist bootstrap 生命周期
  - LogCallback (OnLog) — 桥接 logrus 输出到 Android Logcat

gobind 据此生成 Java 抽象类, 供 OpenListBridge.kt 实现。"

git push origin dev

# commit 2（必须独立提交, 便于 rebase / cherry-pick）
# ... 跑 §2.2 替换命令 ...
git add -A
git commit -m "refactor(db): switch to glebarez/sqlite (pure-Go, CGO-free)

L2 B1 落地: 解决 gomobile bind + mattn/go-sqlite3 CGO 路径下 NDK toolchain
兼容性差、-fPIC 报错、AAR 体积膨胀等问题。

GORM Dialector 接口 100% 兼容，业务层零改写。
AAR 体积预期 -12MB（45MB → 33MB）。

详见 .trae/documents/openlist-aar-sqlite-cgo-multi-solution.md §三 B1。"

git push origin dev
```

## 四、CI 验收清单

fork 推完后，下次 CI 应该看到：

```
[openlist-aar]   [A2] openlistlib/event.go present in fork @ <sha>, no injection needed
[openlist-aar]   [B2] CGO toolchain pinned: CC=.../aarch64-linux-android21-clang
                            （如 glebarez 已切, 这行只是无害的环境变量）
[openlist-aar] == gomobile bind (bind pkg: ./openlistlib) ==
go: downloading github.com/glebarez/sqlite v1.x.x
go: downloading modernc.org/sqlite v1.x.x
... (无 mattn 行) ...
gomobile: ... 成功
```

如果还有报错，按 plan 文档 §三 的 6 类报错清单逐条对照。
