# scripts/split_mobile_api.py
# 拆分 mobile_api.go 为 12 个子域文件
# 策略：按函数边界切分；新文件继承原 imports；运行后用 goimports 清理未用的
import re
import os

SRC = "internal/server/mobile_api.go"
with open(SRC, "r") as f:
    content = f.read()
lines = content.split("\n")

# 1) 找到顶层 func 起始行（缩进=0）
top_funcs = []
for i, line in enumerate(lines):
    if re.match(r"^func ", line):
        top_funcs.append((i, re.match(r"^func (\(\S+ \*\w+\) )?(\w+)\(", line).group(2)))
# print(f"Found {len(top_funcs)} top-level funcs")

# 2) 计算每个函数的结束行：花括号平衡
def find_end(start):
    depth = 0
    saw_open = False
    for i in range(start, len(lines)):
        for j, ch in enumerate(lines[i]):
            if ch == "{":
                depth += 1
                saw_open = True
            elif ch == "}":
                depth -= 1
                if saw_open and depth == 0:
                    return i
    return len(lines) - 1

# 计算 func_body
funcs_meta = []
for i, (start, name) in enumerate(top_funcs):
    end = find_end(start)
    if i + 1 < len(top_funcs):
        end = min(end, top_funcs[i+1][0] - 1)
    funcs_meta.append((name, start, end, "\n".join(lines[start:end+1])))

# 3) 提取原 imports 块
imports_match = re.search(r"^import\s*\((.*?)\n\)\n", content, re.DOTALL | re.MULTILINE)
imports_block = imports_match.group(0) if imports_match else ""
# 解析每行 import（去掉注释、空白）
def parse_imports(block):
    raw = re.sub(r"^import\s*\(|^\)\s*$", "", block, flags=re.MULTILINE)
    lines = [l.strip() for l in raw.split("\n") if l.strip() and not l.strip().startswith("//")]
    return lines

all_imports = parse_imports(imports_block)
# print(f"Original imports: {len(all_imports)}")

# 4) 分组配置：每个新文件名 → 包含的 func 列表
GROUPS = {
    "mobile_api.go": {
        "keep_in_source": True,
        "funcs": ["isValidSiteID", "writeServiceErrorGin", "handlePingGin", "handleHealthGin", "handleServerShutdownGin"],
        "doc": "通用 helper + 健康检查入口。业务子域 handler 已拆分到 mobile_*.go。",
    },
    "mobile_files.go": {
        "funcs": ["handleListFilesGin", "mountListAsFiles", "handleDeleteFileGin", "handleCreateDirectoryGin", "handleUploadFileGin", "handleServiceGuardGin", "handlePluginOpenlistProxyGin", "handleReadFileContentGin", "handleTextPreviewExtsGin", "handleFileInfoGin", "handleRenameFileGin", "handleFileExistsGin", "handleEncryptOutputExistsGin"],
        "doc": "文件 CRUD：list / delete / create / upload / rename / info / exists / encrypt_output_exists。",
    },
    "mobile_tasks.go": {
        "funcs": ["handleGetTasksGin", "handleCreateTaskGin", "handleCreateTaskBatchGin", "handleCancelTaskGin", "handleCancelRunGin", "handleResumeRunGin", "handleResumeAllPausedGin", "handleGetRunSummaryGin", "handleListRunsGin", "handleRetryTaskGin", "handleRemoveTaskGin", "handleClearCompletedTasksGin"],
        "doc": "任务系统 CRUD：list / create / batch_create / cancel / resume / list_runs / retry / remove / clear。",
    },
    "mobile_search.go": {
        "funcs": ["handleSearchFilesGin", "searchFilesWithMounts", "searchAcrossAllMounts", "searchWebdavMounts", "handleIndexStatsGin", "handleIndexRebuildGin", "handleIndexClearGin", "handleVectorSearchTasksGin", "handleVectorSearchFilesGin", "vectorSearchFallback", "expandCJKQueryForSearch", "buildSearchCacheKey", "writeSearchResponseGin", "extractBigrams", "hasSufficientBigramOverlap", "hasSufficientBigramOverlapEx", "countSharedBigrams", "computeHybridScore", "handleSearchStatsGin"],
        "doc": "搜索 + 向量搜索 + CJK 扩展 + bigram 过滤 + hybrid 评分。包含 LRU 缓存 hook（在 search_cache.go）。",
    },
    "mobile_webdav.go": {
        "funcs": ["handleTestWebDAVGin", "handlePermissionsGin", "canUseWebdavIndex", "handleTestLocalWebDAVGin", "handleWebDavLocalInfoGin", "handleWebDavManifestGin", "handleStreamExternalFileGin"],
        "doc": "WebDAV 探针 / 权限 / 索引 / 本地信息 / manifest / 外部流。",
    },
    "mobile_openlist.go": {
        "funcs": ["handleRemoteInfoGin", "handleListOpenlistSitesGin", "handleAddOpenlistSiteGin", "handleUpdateOpenlistSiteGin", "handleDeleteOpenlistSiteGin"],
        "doc": "Openlist 远程站点的 CRUD。",
    },
    "mobile_logs.go": {
        "funcs": ["handleAPILogsGin", "handleAPILogsRecentGin", "writeConfigToFile", "handleBuildInfoGin", "handleGetContainerVersionsGin", "handleFFmpegStatusGin", "handleAutomationReportGin"],
        "doc": "API 日志查询 + 构建信息 + FFmpeg 状态 + 自动化测试报告。",
    },
    "mobile_sparse.go": {
        "funcs": ["handleSparseContainerWriteGin", "handleSparseContainerProbeGin", "handleSparseContainerCleanupGin"],
        "doc": "Sparse container：write / probe / cleanup。",
    },
    "mobile_metadata.go": {
        "funcs": ["handleTagsListGin", "handleTagsMutateGin", "handlePluginsGin", "taskOptionsToGinH", "handlePredictPluginGin", "handleContainerExtensionsGin"],
        "doc": "标签 + 插件元数据 + 容器扩展名 + 任务选项。",
    },
    "mobile_stream.go": {
        "funcs": ["writeSSEEvent", "handleListFilesStreamGin", "handleAlistEncryptStreamGin", "handleAlistDecodeFilenameGin", "handlePluginFilesStreamGin"],
        "doc": "SSE 流式端点：list_files_stream / alist_encrypt_stream / plugin_files_stream / alist_decode_filename。",
    },
    "mobile_database.go": {
        "funcs": ["handleDatabaseInfo", "getAvailableEngines", "handleDatabaseExport", "handleDatabaseImport", "handleDatabaseBackup", "handleDatabaseRestore"],
        "doc": "数据库管理：info / export / import / backup / restore / available_engines。",
    },
}

func_bodies = {name: body for name, _, _, body in funcs_meta}

# 5) 写每个新文件
for filename, cfg in GROUPS.items():
    if cfg.get("keep_in_source"):
        continue
    funcs_in_file = []
    for fname in cfg["funcs"]:
        if fname in func_bodies:
            funcs_in_file.append(func_bodies[fname])
        else:
            print(f"WARN: function {fname} not found")
    if not funcs_in_file:
        continue
    header = f"package server\n\n// {filename} — {cfg['doc']}\n\n"
    # 新文件继承原 imports（goimports 会清理未用到的）
    header += imports_block + "\n\n"
    body = "\n\n".join(funcs_in_file) + "\n"
    out_path = f"internal/server/{filename}"
    with open(out_path, "w") as f:
        f.write(header + body)
    out_lines = (header + body).count("\n")
    print(f"  {filename:30s}  {len(funcs_in_file):2d} funcs  {out_lines:4d} lines")

# 6) 瘦身 mobile_api.go：只保留 keep_in_source=true 的函数
keep_cfg = GROUPS["mobile_api.go"]
keep_funcs = [func_bodies[n] for n in keep_cfg["funcs"] if n in func_bodies]

new_header = "package server\n\n"
new_header += "// mobile_api.go — 通用 helper + 健康检查入口。\n"
new_header += "// 业务子域 handler 已拆分到 mobile_*.go：\n"
new_header += "//   - mobile_files.go    文件 CRUD\n"
new_header += "//   - mobile_search.go   搜索 + 向量 + CJK 扩展 + bigram\n"
new_header += "//   - mobile_tasks.go    任务 CRUD\n"
new_header += "//   - mobile_webdav.go   WebDAV + index\n"
new_header += "//   - mobile_openlist.go Openlist 站点\n"
new_header += "//   - mobile_metadata.go 标签 + 插件 + 容器扩展\n"
new_header += "//   - mobile_stream.go   SSE 流式\n"
new_header += "//   - mobile_database.go 数据库管理\n"
new_header += "//   - mobile_logs.go     API 日志 + 构建信息\n"
new_header += "//   - mobile_sparse.go   Sparse container\n"
new_header += "\n" + imports_block + "\n\n"
new_body = "\n\n".join(keep_funcs) + "\n"
with open(SRC, "w") as f:
    f.write(new_header + new_body)
out_lines = (new_header + new_body).count("\n")
print(f"  {'mobile_api.go (trimmed)':30s}  {len(keep_funcs):2d} funcs  {out_lines:4d} lines")

print("\nRun: goimports -w internal/server/mobile_*.go internal/server/mobile_api.go")
