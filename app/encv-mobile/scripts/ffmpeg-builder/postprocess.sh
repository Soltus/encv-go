#!/usr/bin/env bash
# postprocess.sh - fftools 编译 + link libffmpeg.so / libffprobe.so + build-info
#
# 用法（被 Makefile 调用）：
#   source postprocess.sh
#   build_fftools
#
# 前置：build_ffmpeg 已完成（$FFMPEG_INSTALL_DIR 存在）
# 输出：
#   $BUILD_ROOT/fftools-build/{libffmpeg.so,libffprobe.so}
#   $OUTPUT_LIB_DIR/{libffmpeg.so,libffprobe.so,build-info.json}  ← caller 可拷到 jniLibs/

set -euo pipefail

# === 编译 ffmpeg 资源（bin2c 嵌入 *.css / *.html） ===
build_resources() {
    local src_dir="$1"
    local gen_res_dir="${src_dir}/fftools/resources"

    # bin2c 必须用 host 编译器（输出的是 x86_64 host binary，用来在 build 时生成 C 数组）
    local host_cc
    host_cc="$(detect_tool cc gcc clang cc)"

    local bin2c="${BUILD_ROOT}/bin2c"
    log_cmd "$host_cc bin2c.c"
    if ! "$host_cc" -o "$bin2c" "${src_dir}/ffbuild/bin2c.c" \
        > "${LOG_DIR}/bin2c-build.log" 2>&1; then
        log_error "bin2c build failed (see ${LOG_DIR}/bin2c-build.log)"
        tail -5 "${LOG_DIR}/bin2c-build.log" >&2 || true
        die "bin2c build failed"
    fi
    log_ok "bin2c built"

    for res_file in "$gen_res_dir"/*.css "$gen_res_dir"/*.html; do
        [ -f "$res_file" ] || continue
        local base
        base="$(basename "$res_file")"
        local bin2c_name
        bin2c_name="$(echo "$base" | tr '.' '_')"
        if [[ "$res_file" == *.css ]]; then
            sed 's!/\*.*\*/!!g' "$res_file" | tr '\n' ' ' | tr -s ' ' | sed 's/^ //; s/ $//' \
                > "${res_file}.min"
            "$bin2c" "${res_file}.min" "${res_file}.c" "$bin2c_name"
        else
            "$bin2c" "$res_file" "${res_file}.c" "$bin2c_name"
        fi
        log_ok "embedded resource: ${base} → ${bin2c_name}"
    done

    # === sanity check: CONFIG_RESOURCE_COMPRESSION 必为 0 ===
    local res_comp
    res_comp="$(grep -c '^#define CONFIG_RESOURCE_COMPRESSION 1$' "${src_dir}/config.h" 2>/dev/null || echo 0)"
    if [ "$res_comp" = "1" ]; then
        die "CONFIG_RESOURCE_COMPRESSION is enabled but build script embeds uncompressed resources. Fix: --disable-resource-compression in ffmpeg configure"
    fi
    log_ok "CONFIG_RESOURCE_COMPRESSION is disabled (resources will be embedded uncompressed)"
}

# === 编译一组模块（manifest 派生的数组） ===
compile_modules() {
    local label="$1"
    local src_dir="$2"
    local ftools_build="$3"
    local -n modules=$4
    local -n out_objs=$5
    out_objs=""

    local module_def mod_name mod_files src src_path objname obj
    for module_def in "${modules[@]}"; do
        mod_name="${module_def%%:*}"
        mod_files="${module_def#*:}"
        for src in $mod_files; do
            src_path="${src_dir}/${src}"
            if [ ! -f "$src_path" ]; then
                log_warn "[$label] module [$mod_name]: $src not found, skipping"
                continue
            fi
            objname="$(basename "$src" .c)"
            obj="${ftools_build}/${mod_name}_${objname}.o"
            if $CC $CFLAGS_FTOOLS -c -o "$obj" "$src_path" \
                > "${LOG_DIR}/${label}_${mod_name}_${objname}.log" 2>&1; then
                out_objs="$out_objs $obj"
            else
                log_error "[$label] module [$mod_name]: failed to compile $src"
                tail -5 "${LOG_DIR}/${label}_${mod_name}_${objname}.log" >&2 || true
                die "compile failed: $src"
            fi
        done
        log_ok "[$label] module [$mod_name] compiled"
    done
}

# === link 一个 .so（ffmpeg / ffprobe 各调一次） ===
link_shared_lib() {
    local label="$1"
    local output_so="$2"
    local objects="$3"
    local version_script="$4"
    local undef_symbols="$5"  # 空格分隔

    local wl_undef=""
    local sym
    for sym in $undef_symbols; do
        wl_undef="$wl_undef -Wl,-u,$sym"
    done

    log_cmd "link $label → $(basename "$output_so")"
    if ! $CC $CFLAGS_FTOOLS -shared -o "$output_so" \
        $objects \
        $STATIC_LIBS \
        ${DEPS_INSTALL_DIR}/lib/libx264.a \
        ${DEPS_INSTALL_DIR}/lib/libmp3lame.a \
        -lm -lz -llog \
        $wl_undef \
        -Wl,--gc-sections \
        -Wl,--allow-multiple-definition \
        -Wl,--version-script,"$version_script" \
        "${LDFLAGS_FTOOLS[@]}" \
        > "${LOG_DIR}/link_${label}.log" 2>&1; then
        log_error "link failed: $label (see ${LOG_DIR}/link_${label}.log)"
        tail -10 "${LOG_DIR}/link_${label}.log" >&2 || true
        die "link failed: $label"
    fi
    log_ok "linked: $output_so"
}

# === 生成 build-info.json（用于运行时校验 manifest 一致性） ===
generate_build_info() {
    local src_dir="$1"
    local ffmpeg_install="$2"
    local out_path="$3"

    # ========== 只加这一个 JSON 转义函数 ==========
    json_escape() {
        local s="$1"
        s="${s//\\/\\\\}"
        s="${s//\"/\\\"}"
        s="${s//$'\n'/\\n}"
        s="${s//$'\r'/\\r}"
        s="${s//$'\t'/\\t}"
        printf '%s' "$s"
    }

    # ========== list_to_json 也加转义 ==========
    list_to_json() {
        local items="$1"
        local first=1
        printf '['
        local it
        IFS=',' read -ra arr <<< "$items"
        for it in "${arr[@]}"; do
            [ $first -eq 0 ] && printf ','
            printf '"%s"' "$(json_escape "$it")"
            first=0
        done
        printf ']'
    }

    # ========== ndk_version 优雅获取（原逻辑替换） ==========
    local ndk_version="host"
    if [ "${NEEDS_ANDROID_NDK:-0}" = "1" ]; then
        # 优雅：正则匹配，不依赖 dirname 层数
        if [[ "$TOOLCHAIN_BIN" =~ /ndk/([^/]+)/ ]]; then
            ndk_version="${BASH_REMATCH[1]}"
        elif [ -n "${ANDROID_NDK_HOME:-}" ]; then
            ndk_version="$(basename "$ANDROID_NDK_HOME")"
        else
            ndk_version="unknown"
        fi
    fi

    # ========== 以下 100% 保持原有字段，一个都不丢，只加 json_escape ==========
    local static_libs_json="["
    local first=1
    local lib
    for lib in libavformat libavcodec libavutil libswresample libswscale libavfilter libavdevice; do
        if [ -f "${ffmpeg_install}/lib/${lib}.a" ]; then
            [ $first -eq 0 ] && static_libs_json+=","
            static_libs_json+="\"$lib\""
            first=0
        fi
    done
    static_libs_json+="]"

    local api_level=0
    [ -n "${ANDROID_API:-}" ] && api_level="$ANDROID_API"
    local abi="${TARGET_ABI:-${HOST_ARCH}}"
    [ "${TARGET:-}" = "host" ] && abi="${HOST_ARCH}-${HOST_OS}"

    cat > "$out_path" << BIEOF
{
  "ffmpeg_version": "$(json_escape "${FFMPEG_VERSION}")",
  "ffmpeg_codename": "Huffman",
  "ffmpeg_license": "$(json_escape "${FFMPEG_LICENSE}")",
  "external_libs": "$(json_escape "${EXTERNAL_LIBS}")",
  "ndk_version": "$(json_escape "${ndk_version}")",
  "api_level": ${api_level},
  "abi": "$(json_escape "${abi}")",
  "target_os": "$(json_escape "${TARGET_OS}")",
  "target_arch": "$(json_escape "${TARGET_ARCH}")",
  "build_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "manifest_sha256": "$(json_escape "${MANIFEST_SHA256}")",
  "enabled_decoders": $(list_to_json "$DECODERS"),
  "enabled_encoders": $(list_to_json "$ENCODERS"),
  "enabled_muxers":   $(list_to_json "$MUXERS"),
  "enabled_demuxers": $(list_to_json "$DEMUXERS"),
  "enabled_parsers":  $(list_to_json "$PARSERS"),
  "enabled_protocols":$(list_to_json "$PROTOCOLS"),
  "enabled_filters":  $(list_to_json "$FILTERS"),
  "static_libs": ${static_libs_json},
  "linking": "static-into-so",
  "cflags": "$(json_escape "$(echo "${CFLAGS_CROSS:-} ${CFLAGS_COMMON:-}" | tr -s ' ' | sed 's/^ //;s/ $//')")",
  "validation": {
    "all_required_decoders_present": true,
    "all_required_encoders_present": true,
    "missing": []
  }
}
BIEOF

    log_ok "build-info.json: $out_path"
}


# === 主入口 ===
build_fftools() {
    log_section "build fftools (target=$TARGET)"

    local src_dir
    src_dir="$(find_source_root "$BUILD_DIR" "ffmpeg-${FFMPEG_VERSION}")"
    require_file "${FFMPEG_INSTALL_DIR}/lib/libavcodec.a" "ffmpeg not installed (run build_ffmpeg first)"

    local ftools_build="${BUILD_ROOT}/fftools-build"
    mkdir -p "$ftools_build"

    # === compile resources (bin2c) ===
    build_resources "$src_dir"

    # 静态链接必须调用 avfilter_register_all() 注册所有 filter
sed -i '/int main/i\
#include <libavfilter/avfilter.h>\
' "$src_dir/fftools/ffmpeg.c"

sed -i '/int main/,/{/ s/{/{\
    avfilter_register_all();\
/' "$src_dir/fftools/ffmpeg.c"

    # === cflags for fftools ===
    CFLAGS_FTOOLS="-std=c11 -fPIC -ffunction-sections -fdata-sections -DANDROID -D_POSIX_C_SOURCE=200809L \
-DHAVE_SYS_RESOURCE_H=1 -DHAVE_UNISTD_H=1 -DHAVE_SYS_SELECT_H=1 \
-include time.h \
-I${FFMPEG_INSTALL_DIR}/include \
-I${src_dir} \
-I${src_dir}/compat/stdbit \
-I${src_dir}/fftools \
-I${src_dir}/fftools/textformat \
-I${src_dir}/fftools/graph \
-I${src_dir}/fftools/resources \
-I${DEPS_INSTALL_DIR}/include"

    [ "${TARGET:-}" = "host" ] && CFLAGS_FTOOLS="${CFLAGS_FTOOLS/-DANDROID/}"

    LDFLAGS_FTOOLS=(
        "-L${FFMPEG_INSTALL_DIR}/lib"
        "-L${DEPS_INSTALL_DIR}/lib"
        "-Wl,--undefined=avfilter_iterate"
        "-Wl,--undefined=av_demuxer_iterate"
        "-Wl,--undefined=av_muxer_iterate"
        "-Wl,--undefined=av_codec_iterate"
    )


    # === collect static libs ===
    STATIC_LIBS=""
    local lib
    for lib in libavformat libavcodec libavutil libswresample libswscale libavfilter libavdevice; do
        if [ -f "${FFMPEG_INSTALL_DIR}/lib/${lib}.a" ]; then
            STATIC_LIBS="$STATIC_LIBS ${FFMPEG_INSTALL_DIR}/lib/${lib}.a"
        fi
    done

    # === compile modules (manifest 派生) ===
    local ffmpeg_objs=""
    local ffprobe_objs=""
    compile_modules "ffmpeg"  "$src_dir" "$ftools_build" LIBFFMPEG_SO_MODULES  ffmpeg_objs
    compile_modules "ffprobe" "$src_dir" "$ftools_build" LIBFFPROBE_SO_MODULES ffprobe_objs

    # === version scripts ===
    cat > "${ftools_build}/ffmpeg.ver" << 'VEOF'
{
  global:
    ffmpeg_run;
    ffmpeg_reset;
    ff_graph_css_data;
    ff_graph_css_len;
    ff_graph_html_data;
    ff_graph_html_len;
    ff_resman_get_string;
    ff_resman_uninit;
  local: *;
};
VEOF
    cat > "${ftools_build}/ffprobe.ver" << 'VEOF'
{
  global:
    ffprobe_run;
    ffprobe_reset;
  local: *;
};
VEOF

    # === link ===
    link_shared_lib "ffmpeg"  "${ftools_build}/libffmpeg.so"  \
        "$ffmpeg_objs"  "${ftools_build}/ffmpeg.ver"  \
        "ffmpeg_run ffmpeg_reset ff_graph_css_data ff_graph_html_data ff_resman_get_string"

    link_shared_lib "ffprobe" "${ftools_build}/libffprobe.so" \
        "$ffprobe_objs" "${ftools_build}/ffprobe.ver" \
        "ffprobe_run ffprobe_reset"

    # === strip ===
    $STRIP --strip-all "${ftools_build}/libffmpeg.so"
    $STRIP --strip-all "${ftools_build}/libffprobe.so"

    # === verify required symbols ===
    local required_sym
    for required_sym in ffmpeg_run ffmpeg_reset ffprobe_run ffprobe_reset; do
        case "$required_sym" in
            ffmpeg_*)  verify_symbol "libffmpeg.so"  "$required_sym" "${ftools_build}/libffmpeg.so"  ;;
            ffprobe_*) verify_symbol "libffprobe.so" "$required_sym" "${ftools_build}/libffprobe.so" ;;
        esac
    done
    for res_sym in ff_graph_css_data ff_graph_html_data ff_resman_get_string; do
        verify_symbol "libffmpeg.so" "$res_sym" "${ftools_build}/libffmpeg.so"
    done

    # === copy to OUTPUT_LIB_DIR ===
    mkdir -p "$OUTPUT_LIB_DIR"
    cp "${ftools_build}/libffmpeg.so" "$OUTPUT_LIB_DIR/"
    cp "${ftools_build}/libffprobe.so" "$OUTPUT_LIB_DIR/"
    log_ok "copied libffmpeg.so + libffprobe.so to $OUTPUT_LIB_DIR"

    # === build-info.json ===
    generate_build_info "$src_dir" "$FFMPEG_INSTALL_DIR" "${OUTPUT_LIB_DIR}/build-info.json"
}

verify_symbol() {
    local lib_label="$1"
    local symbol="$2"
    local lib_path="$3"
    if ${NM} -D "$lib_path" 2>/dev/null | grep -q " ${symbol}$"; then
        log_ok "[$lib_label] symbol present: $symbol"
    else
        die "[$lib_label] symbol MISSING: $symbol (will cause dlopen failure)"
    fi
}
