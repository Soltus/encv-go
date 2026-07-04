# Failure F2: dev-openlist.sh 写 config 到 fork cwd，但 OpenList 从 ${DATA_DIR}/config.json 读

**Phase**: P3 (C4 dev 启动脚本)
**Task**: Task 3.1 (写 dev-openlist.sh)
**Date**: 2026-06-02
**Status**: ✅ 已修

---

## 复现命令

```bash
cd /workspace/app/encv-mobile
bash scripts/dev-openlist.sh --data /tmp/openlist-data3
# → 看到 "写入 config.json (dist_dir=./public/dist, http_port=5244) 成功"
# → OpenList 启动，reading config file: /tmp/openlist-data3/config.json
#   ↑ 注意读的是 /tmp 下的，不是 fork 下的
```

## 实际输出

```bash
$ cat /tmp/openlist-data3/config.json | grep -E '"dist_dir"|"http_port"'
    "address": "0.0.0.0",
    "http_port": 5244,
  "dist_dir": "",        ← 空！我们的配置没生效
```

## 根因

1. **dev-openlist.sh 早期版本**写 `fork_dir/config.json`（用相对路径 `./public/dist`）
2. **OpenList 启动时默认从 `${DATA_DIR}/config.json` 读配置**（参见 `cmd/root.go`）：
   ```go
   RootCmd.PersistentFlags().StringVar(&flags.ConfigPath, "config", "",
       "path to config.json (relative to current working directory; defaults to [data directory]/config.json, where [data directory] is set by --data)")
   ```
3. 所以 fork 目录的 `config.json` 被 OpenList 忽略
4. OpenList 启动时发现 `${DATA_DIR}/config.json` 不存在 → 自动生成默认配置 → `dist_dir=""`

**症状链**：
- 脚本说写入成功（fork dir 确实有文件）
- 但 OpenList 启动时读的是 data dir 的文件
- data dir 没有我们写的文件，OpenList 用自动生成的默认配置
- 结果：dist_dir 是空，OpenList 用 embed.FS（如果 fork 有的话）或者无法服务前端

**这次因为我们没有去验证 OpenList 是否真的从 disk 读前端，而是先验证了 OpenList 能跑**——所以 F2 是潜在的、没在第一时间暴露的 bug。

## 修复

[dev-openlist.sh:152-167](file:///workspace/app/encv-mobile/scripts/dev-openlist.sh#L152-L167)：

```bash
# OpenList 启动时默认从 ${DATA_DIR}/config.json 读配置（也可显式 --config 指定）
# 我们写一份到 data dir，用绝对路径避免相对路径解析问题
if [[ "${NO_CONFIG}" -eq 0 ]]; then
  CONFIG_FILE="${DATA_DIR}/config.json"
  ABS_DIST_DIR="$(cd "${FORK_DIR}/public/dist" && pwd)"
  log "写入 ${CONFIG_FILE} (dist_dir=${ABS_DIST_DIR}, http_port=${PORT})"
  cat > "${CONFIG_FILE}" <<EOF
{
  "dist_dir": "${ABS_DIST_DIR}",   # 绝对路径
  "scheme": {
    "address": "0.0.0.0",
    "http_port": ${PORT},
    ...
  }
}
EOF
else
  log "--no-config：跳过 config.json 写入"
fi
```

**两个关键改动**：
1. **写到 `${DATA_DIR}/config.json`** 而不是 fork cwd
2. **dist_dir 用绝对路径** `$(cd ${FORK_DIR}/public/dist && pwd)` 而不是相对路径

## 验证

```bash
$ bash scripts/dev-openlist.sh --data /tmp/openlist-data3
[dev-openlist] 写入 /tmp/openlist-data3/config.json (dist_dir=/workspace/app/openlist/Hi-Sillot-OpenList/public/dist, http_port=5244)
INFO[2026-06-02 22:19:10] reading config file: /tmp/openlist-data3/config.json
INFO[2026-06-02 22:19:10] load config from env with prefix: OPENLIST_
Successfully created the admin user and the initial password is: tqgWpAbP
start HTTP server @ 0.0.0.0:5244

$ cat /tmp/openlist-data3/config.json | grep -E '"dist_dir"|"http_port"'
    "http_port": 5244,
  "dist_dir": "/workspace/app/openlist/Hi-Sillot-OpenList/public/dist",   ✅ 正确

$ curl -s http://127.0.0.1:5244/api/public/settings | head -1
{"code":200,"message":"success","data":{"allow_indexed":"false",...}}   ✅ 真实 OpenList 响应
```

## 教训

1. **跨工具的配置路径约定要查源码**：OpenList 用 `--data` 推导 `config.json` 位置，光看 flag doc 不够
2. **相对路径有歧义**：脚本和 OpenList 进程的 cwd 不一定相同，**永远用绝对路径**
3. **测试要端到端**：不能只验证「配置写入成功」就以为生效，要看 OpenList 实际读到的内容（用 `reading config file: ...` 日志 + `cat` 实际内容双确认）
4. **第一次跑通可能假阳性**：F2 是配置类的 bug，只有深查 `dist_dir` 字段才能发现
