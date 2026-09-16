# i18n 工具链

跨项目 i18n 管理工具（`scripts/i18n-tool.py` + `scripts/i18n_lib/`）。

## 命令速查

| 命令 | 用途 |
| --- | --- |
| `scan --app <name>` / `--all` | 扫描源码用到的 key 并检查缺失 |
| `lint --app <name>` / `--all` | 全量 lint（缺失/变量一致/重复 key/英文质量）；`--all` 末尾打印性能报告 |
| `var-check --app <name>` | 翻译变量/参数一致性 |
| `dup --app <name>` | 近重复翻译检测（MinHash+LSH） |
| `stats` / `gen-types` / `db-init` / `db-query` / `find-key` | 统计 / 类型生成 / DB / 搜索 |
| `add-key <key> --zh .. --en ..` | 新增 key 到 TS 字典 |
| **`move-key <spec> --from A --to B`** | **迁移 key（app→shared 或跨 app）** |
| `benchmark` | 性能基准测试 |
| `compile-json` / `compile-android` | 编译字典给 Go/Kotlin 复用 |

## move-key（key 迁移）

把 key 从一个字典层迁移到另一层，复用 loader（shared 去重/缓存）与 addkey，
幂等、双 locale 自动对齐。**模块提升（lift）时下沉 i18n key 的标准做法**，取代一次性脚本。

```bash
# 把 buildReportZip 用到的 key 下沉到 shared 并注册进 sharedI18nModules
python3 scripts/i18n-tool.py move-key "tasks.report*"       --from encv-mobile --to shared --register
python3 scripts/i18n-tool.py move-key "tasks.performance."  --from encv-mobile --to shared --register
```

匹配语义（三选一）：

- `tasks.report*`（尾星）→ **字符串前缀**，匹配 `tasks.reportTitle` 等驼峰 key
- `tasks.performance.`（尾点）→ **点边界子树**，匹配 `tasks.performance.xxx`
- `tasks.reportTitle`（无后缀）→ **精确匹配**单个 key

选项：

- `--dry-run`：只规划、打印匹配数量，不落盘（可先预检）
- `--keep`：保留源字典中的 key（过渡兼容；loader 合并时重复 key 仅覆盖、不报错）
- `--register`：`--to shared` 时把目标模块注册进 `sharedI18nModules`

## 架构：shared 字典层

`i18n_lib/config.py` 的 `_discover_apps()` 按「shared 层」设计：
`app/packages/shared-components/src/i18n/*.ts` 会**并入每一个 app** 的字典与扫描目录。
因此 shared 内的源码被所有 app 扫描，**shared 模块用到的 key 必须定义在 shared 自己的字典里**，
否则会在各 app 报 `MISSING_KEY`。提升模块到 shared 时，用 `move-key --to shared` 把 key 一并下沉。

## 性能优化（loader / perf）

工具在 `lint --all` 下并行检查多个 app，历史上存在两个问题，现已修复：

1. **性能指标丢失（线程不安全）** — `perf.py` 的 `PerfTracker` 原用共享 `_active` dict，
   并行下多个 app 同名指标（如"字典加载"）互相覆盖，导致报告漏计。
   现改为 **线程局部存储 + 锁保护 metrics 列表**，每个 app 的指标独立、不丢失。

2. **shared 字典被每个 app 重复解析（缓存击穿）** — 每个 app 把 shared 文件并入自己的
   `i18n_files` 各自起 node 子进程解析，`lint --all` 下 shared 被解析 ~5 次。
   现 `loader.py` 用 **双重检查锁定 + 进程内 `_shared_cache`**：shared 字典进程内**只冷解析一次**，
   其余线程等待后命中缓存（性能报告"共享字典去重"小节会显示 `冷解析次数=1`）。
   另加 `_mem_cache`：同进程内重复 `load_all_dicts` 直接命中。

验证：`python3 scripts/i18n_lib/run-tests.py`（单元测试）、
`python3 scripts/i18n-tool.py benchmark`（含 shared 去重 + move-key 干跑覆盖）。
