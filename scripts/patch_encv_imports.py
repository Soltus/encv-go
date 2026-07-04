#!/usr/bin/env python3
"""
扫描 encv_*.ts 找出对 ./encv_core 等模块的未声明引用，自动在头部加 import。
"""
import os
import re

API_DIR = "/workspace/app/encv-mobile/src/api"

SOURCES = {
    "./encv_core": [
        "getApiBaseUrl", "setApiBaseUrl", "getServerUrl", "resetServerUrl",
        "getWebSocketUrl", "proxySafeEncode", "isOpenPreviewBrowser",
        "getPersistedBackendIdentity", "DEFAULT_API_BASE_URL", "DEV_SANDBOX_ENTRY",
        "SERVER_URL_KEY", "SERVER_INSTANCE_ID_KEY", "SERVER_VERSION_KEY",
    ],
    "./encv_files": [
        "getFileExtension", "getFileCategory", "formatFileSize",
        "FileItem", "FileListResponse", "PermissionDeniedError", "NotFoundError",
    ],
    "./encv_tasks": ["EncvTask", "TaskType", "TaskStatus", "TaskStep"],
    "./encv_admin": [
        "checkServerStatus", "isTextPreviewable", "fetchTextPreviewExts",
        "invalidateTextExtsCache", "TextPreviewExts",
    ],
    "./encv_webdav": ["WebDAVConfig", "RemoteInfo"],
    "./encv_perf": ["PerformanceSummary", "PhaseTiming", "PerformanceMetrics", "CalibrationResult"],
}


def needs_import(content, name):
    if re.search(rf"\b{re.escape(name)}\b", content) is None:
        return False
    if re.search(rf"import\s+\{{[^}}]*\b{re.escape(name)}\b[^}}]*\}}", content):
        return False
    return True


def patch(file_path):
    with open(file_path) as f:
        content = f.read()
    original = content
    basename = os.path.basename(file_path).replace(".ts", "")

    # 1) 移除未用的 import
    for m in re.finditer(r"import\s+\{([^}]+)\}\s+from\s+'([^']+)'", content):
        names = [n.strip() for n in m.group(1).split(",") if n.strip()]
        used = [n for n in names if re.search(rf"\b{re.escape(n)}\b", content.split(m.group(0))[0] + content.split(m.group(0))[1])]
        if not used:
            content = content.replace(m.group(0) + "\n", "")
        elif len(used) < len(names):
            content = content.replace(m.group(0), f"import {{ {', '.join(used)} }} from '{m.group(2)}'")

    # 2) 添加缺失的 import
    for src, names in SOURCES.items():
        if src.endswith(basename):
            continue
        needed = [n for n in names if needs_import(content, n)]
        if not needed:
            continue
        existing_match = re.search(rf"import\s+\{{([^}}]+)\}}\s+from\s+'{re.escape(src)}'", content)
        if existing_match:
            existing_names = [n.strip() for n in existing_match.group(1).split(",") if n.strip()]
            new_names = list(dict.fromkeys(existing_names + needed))
            content = content.replace(existing_match.group(0), f"import {{ {', '.join(new_names)} }} from '{src}'")
        else:
            content = f"import {{ {', '.join(needed)} }} from '{src}'\n\n" + content

    if content != original:
        with open(file_path, "w") as f:
            f.write(content)
        return True
    return False


def main():
    for fn in sorted(os.listdir(API_DIR)):
        if fn.startswith("encv_") and fn.endswith(".ts"):
            path = os.path.join(API_DIR, fn)
            if patch(path):
                print(f"  patched {fn}")


if __name__ == "__main__":
    main()
