#!/usr/bin/env bash
# common/manifest.sh - 读 ffmpeg-feature-manifest.json 派生 configure 选项
#
# 用法（必须在 ffmpeg_builder_init 之后调用）：
#   source common/manifest.sh
#   load_manifest
#   echo "$DECODERS $ENCODERS ..."
#   echo "${LIBFFMPEG_SO_MODULES[@]}"
#
# 暴露变量：
#   DECODERS, ENCODERS, MUXERS, DEMUXERS, PARSERS, PROTOCOLS, FILTERS (逗号分隔字符串)
#   LIBFFMPEG_SO_MODULES, LIBFFPROBE_SO_MODULES (bash 数组，元素 "modname:file1 file2 ...")
#   FFMPEG_VERSION, FFMPEG_LICENSE
#   EXTERNAL_LIBS (空格分隔)
#   MANIFEST_SHA256
#
# 设计：
#   - 不内联 eval；用 python3 一次性 dump 成可 source 的临时文件，再 source
#   - 支持 _shared 复用解析（libffprobe 复用 libffmpeg 的 opt_common）
#   - 任意 step 失败 → die（不返回部分结果）

set -euo pipefail

# 内部：检测 JSON parser
_manifest_pick_parser() {
    if command -v jq >/dev/null 2>&1; then
        echo jq
        return 0
    fi
    if command -v python3 >/dev/null 2>&1; then
        echo python3
        return 0
    fi
    die "manifest.sh needs jq or python3 to parse $MANIFEST_FILE"
}

# 内部：把 manifest 解析成一组可 source 的 export 行
# 输出到 stdout，调用方 source 之
_manifest_dump_vars() {
    local parser="$1"
    local manifest_path="$2"

    if [ "$parser" = "jq" ]; then
        # jq 一次性 dump（不依赖 python3）
        jq -r '
            .ffmpeg.version as $fv |
            .ffmpeg.license as $fl |
            (.ffmpeg.decoders | join(",")) as $dec |
            (.ffmpeg.encoders | join(",")) as $enc |
            (.ffmpeg.muxers   | join(",")) as $mux |
            (.ffmpeg.demuxers | join(",")) as $dem |
            (.ffmpeg.parsers  | join(",")) as $par |
            (.ffmpeg.protocols| join(",")) as $pro |
            (.ffmpeg.filters  | join(",")) as $flt |
            (.ffmpeg.external_libs | join(" ")) as $ext |
            [
              "FFMPEG_VERSION=" + ($fv | @sh),
              "FFMPEG_LICENSE=" + ($fl | @sh),
              "DECODERS=" + ($dec | @sh),
              "ENCODERS=" + ($enc | @sh),
              "MUXERS=" + ($mux | @sh),
              "DEMUXERS=" + ($dem | @sh),
              "PARSERS=" + ($par | @sh),
              "PROTOCOLS=" + ($pro | @sh),
              "FILTERS=" + ($flt | @sh),
              "EXTERNAL_LIBS=" + ($ext | @sh)
            ] | .[]
        ' "$manifest_path"
        # ftools_modules 用 bash 数组（jq 拼字符串带空格易踩坑，单独走 python3 兜底）
        return 0
    fi

    # python3 解析：dump 上面所有简单变量 + 数组
    python3 - "$manifest_path" << 'PYEOF'
import json, sys, shlex

m = json.load(open(sys.argv[1]))
f = m["ffmpeg"]

def emit(name, value):
    print(f'{name}={shlex.quote(value)}')

emit("FFMPEG_VERSION", f["version"])
emit("FFMPEG_LICENSE", f["license"])
emit("DECODERS", ",".join(f["decoders"]))
emit("ENCODERS", ",".join(f["encoders"]))
emit("MUXERS",   ",".join(f["muxers"]))
emit("DEMUXERS", ",".join(f["demuxers"]))
emit("PARSERS",  ",".join(f["parsers"]))
emit("PROTOCOLS",",".join(f["protocols"]))
emit("FILTERS",  ",".join(f["filters"]))
emit("EXTERNAL_LIBS", " ".join(f.get("external_libs", [])))

# ftools_modules → bash 数组
for lib_name, modules in m["ftools_modules"].items():
    var_name = lib_name.upper().replace(".", "_") + "_MODULES"
    print(f'{var_name}=(')
    for mod_name, files in modules.items():
        # bool true → 复用其他 lib 的同名（去掉 _shared 后缀）模块
        if not isinstance(files, list):
            if isinstance(files, bool) and files:
                resolved_name = mod_name.replace("_shared", "")
                resolved = None
                for other_lib, other_mods in m["ftools_modules"].items():
                    if resolved_name in other_mods and isinstance(other_mods[resolved_name], list):
                        resolved = other_mods[resolved_name]
                        break
                if resolved is None:
                    continue
                files = resolved
            else:
                continue
        files_str = " ".join(files)
        print(f'  "{mod_name}:{files_str}"')
    print(")")
PYEOF
}

# 内部：dump 数组部分（用 python3，避免 jq 拼复杂数组）
_manifest_dump_arrays() {
    local manifest_path="$1"
    if ! command -v python3 >/dev/null 2>&1; then
        die "manifest.sh arrays (ftools_modules) require python3 (jq fallback not implemented)"
    fi
    python3 - "$manifest_path" << 'PYEOF'
import json, sys
m = json.load(open(sys.argv[1]))
for lib_name, modules in m["ftools_modules"].items():
    var_name = lib_name.upper().replace(".", "_") + "_MODULES"
    print(f'{var_name}=(')
    for mod_name, files in modules.items():
        if not isinstance(files, list):
            if isinstance(files, bool) and files:
                resolved_name = mod_name.replace("_shared", "")
                resolved = None
                for other_lib, other_mods in m["ftools_modules"].items():
                    if resolved_name in other_mods and isinstance(other_mods[resolved_name], list):
                        resolved = other_mods[resolved_name]
                        break
                if resolved is None:
                    continue
                files = resolved
            else:
                continue
        files_str = " ".join(files)
        print(f'  "{mod_name}:{files_str}"')
    print(")")
PYEOF
}

# === 公共入口 ===
load_manifest() {
    require_file "$MANIFEST_FILE" "manifest not found: $MANIFEST_FILE"
    local parser
    parser="$(_manifest_pick_parser)"

    # 简单变量（jq 或 python3 都行）
    _manifest_dump_vars "$parser" "$MANIFEST_FILE" > "${BUILD_DIR}/.manifest-vars.sh"

    # 数组（必须有 python3）
    _manifest_dump_arrays "$MANIFEST_FILE" > "${BUILD_DIR}/.manifest-arrays.sh"

    # source 两个 dump
    # shellcheck disable=SC1090,SC1091
    source "${BUILD_DIR}/.manifest-vars.sh"
    # shellcheck disable=SC1090,SC1091
    source "${BUILD_DIR}/.manifest-arrays.sh"

    # manifest checksum（用于 build-info.json）
    MANIFEST_SHA256="$(sha256sum "$MANIFEST_FILE" | cut -d' ' -f1)"

    export DECODERS ENCODERS MUXERS DEMUXERS PARSERS PROTOCOLS FILTERS
    export EXTERNAL_LIBS FFMPEG_VERSION FFMPEG_LICENSE MANIFEST_SHA256
    export LIBFFMPEG_SO_MODULES LIBFFPROBE_SO_MODULES

    log_info "manifest loaded: ffmpeg $FFMPEG_VERSION, $(echo "$ENCODERS" | tr ',' '\n' | wc -l) encoders, $(echo "$DECODERS" | tr ',' '\n' | wc -l) decoders"
    log_info "  encoders: $ENCODERS"
    log_info "  decoders: $DECODERS"
    log_info "  external libs: $EXTERNAL_LIBS"
}
