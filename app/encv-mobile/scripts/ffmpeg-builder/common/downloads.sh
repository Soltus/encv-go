#!/usr/bin/env bash
# common/downloads.sh - 下载 + sha256 校验 + 解压 + 镜像兜底
#
# 用法：
#   source common/downloads.sh
#   download_and_verify "https://ffmpeg.org/releases/ffmpeg-8.0.tar.xz" \
#       "ffmpeg-8.0.tar.xz" \
#       "ab09c7..."
#   extract_to "ffmpeg-8.0.tar.xz" "$BUILD_DIR"
#
# 镜像兜底（按顺序尝试）：
#   1) 原始 URL
#   2) https://gh-proxy.com/<url>  (GitHub 加速)
#   3) 跳过（fail 显式报错）
#
# 设计：
#   - 不写死 NDK / 任何私有镜像（沙盒/外网用户都能跑）
#   - sha256 不匹配 → 自动重试下一个镜像
#   - 解压后清理压缩包（节省磁盘）

set -euo pipefail

# 镜像候选（前缀形式）
# 用法: build_mirror_candidates <original_url> → 输出多行 URL
build_mirror_candidates() {
    local original="$1"
    echo "$original"
    # GitHub 加速（gh-proxy.com 是公开镜像，CI runner 通常可访问）
    if [[ "$original" == *"github.com"* ]] || [[ "$original" == *"codeload.github.com"* ]]; then
        echo "https://gh-proxy.com/${original}"
    fi
    # SourceForge 直链兜底（有时候 302 跳到 CDN）
    if [[ "$original" == *"sourceforge.net"* ]]; then
        echo "${original}&use_mirror=autoselect"
    fi
}

# 下载并校验
# 用法: download_and_verify <url> <output_path> <sha256>
#   - sha256 为空时跳过校验（仅日志警告）
#   - 镜像全部失败 → die
download_and_verify() {
    local url="$1"
    local out_path="$2"
    local expected_sha="${3:-}"

    local out_dir
    out_dir="$(dirname "$out_path")"
    mkdir -p "$out_dir"

    if [ -f "$out_path" ]; then
        if [ -n "$expected_sha" ]; then
            local actual_sha
            actual_sha="$(sha256sum "$out_path" | cut -d' ' -f1)"
            if [ "$actual_sha" = "$expected_sha" ]; then
                log_ok "cache hit: $out_path (sha256 match)"
                return 0
            fi
            log_warn "cache sha256 mismatch (expected=$expected_sha, actual=$actual_sha), re-downloading"
            rm -f "$out_path"
        else
            log_ok "cache hit: $out_path (no sha check)"
            return 0
        fi
    fi

    local tried=()
    while IFS= read -r candidate; do
        log_info "downloading: $candidate → $out_path"
        if curl -fsSL --connect-timeout 15 --max-time 600 --retry 2 -o "$out_path" "$candidate" 2>"${out_dir}/.curl.err"; then
            :
        else
            log_warn "download failed: $candidate ($(cat "${out_dir}/.curl.err" 2>/dev/null | head -1 || echo unknown))"
            tried+=("$candidate")
            rm -f "$out_path"
            continue
        fi

        if [ -n "$expected_sha" ]; then
            local actual_sha
            actual_sha="$(sha256sum "$out_path" | cut -d' ' -f1)"
            if [ "$actual_sha" != "$expected_sha" ]; then
                log_warn "sha256 mismatch for $candidate (expected=$expected_sha, got=$actual_sha)"
                tried+=("$candidate")
                rm -f "$out_path"
                continue
            fi
            log_ok "downloaded + sha256 verified: $out_path"
        else
            log_ok "downloaded (no sha check): $out_path"
        fi
        rm -f "${out_dir}/.curl.err"
        return 0
    done < <(build_mirror_candidates "$url")

    die "All download candidates failed for $url. Tried: ${tried[*]}"
}

# 解压（自动识别 tar.xz / tar.gz / tar.bz2 / zip）
# 用法: extract_to <archive> <dest_dir>
#   成功后清理压缩包
extract_to() {
    local archive="$1"
    local dest_dir="$2"
    require_file "$archive" "archive to extract"
    mkdir -p "$dest_dir"

    log_info "extracting: $archive → $dest_dir"
    case "$archive" in
        *.tar.xz|*.txz)
            tar -xJf "$archive" -C "$dest_dir" ;;
        *.tar.gz|*.tgz)
            tar -xzf "$archive" -C "$dest_dir" ;;
        *.tar.bz2|*.tbz2)
            tar -xjf "$archive" -C "$dest_dir" ;;
        *.tar.zst|*.tzst)
            tar --zstd -xf "$archive" -C "$dest_dir" ;;
        *.zip)
            unzip -q -o "$archive" -d "$dest_dir" ;;
        *)
            die "Unknown archive format: $archive (supported: tar.xz/gz/bz2/zst, zip)" ;;
    esac
    log_ok "extracted: $archive"
}

# 找到解压后的源码根目录（避免硬编码目录名，兼容 libogg-1.3.5 / lame-3.100 / flac-x.y.z）
# 用法: find_source_root <parent_dir> <name_pattern>
#   name_pattern 是 shell glob（不写死具体版本号）
find_source_root() {
    local parent="$1"
    local pattern="$2"
    require_dir "$parent" "source parent dir"

    # 1) 精确匹配 pattern
    local found
    found="$(find "$parent" -maxdepth 1 -mindepth 1 -type d -name "$pattern" 2>/dev/null | head -1 || true)"
    if [ -n "$found" ]; then
        echo "$found"
        return 0
    fi
    # 2) pattern 放宽（用 * 包围）
    local fuzzy
    fuzzy="$(find "$parent" -maxdepth 1 -mindepth 1 -type d -name "*${pattern}*" 2>/dev/null | head -1 || true)"
    if [ -n "$fuzzy" ]; then
        log_warn "loose source dir match: $pattern → $fuzzy"
        echo "$fuzzy"
        return 0
    fi
    die "source dir not found in $parent (pattern: $pattern)"
}
