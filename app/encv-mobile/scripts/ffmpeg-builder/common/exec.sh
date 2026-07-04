#!/usr/bin/env bash
# common/exec.sh - 统一错误处理 + 日志 tail
#
# 用法：source "$(dirname "${BASH_SOURCE[0]}")/common/exec.sh"
# 暴露：die / run / run_logged
#
# 关键设计：
#   - 所有失败带"最近 30 行日志"输出（避免每次都手 tail）
#   - 默认使用 set -euo pipefail（外层 caller 已 set）
#   - run_logged 包装命令，把 stdout/stderr 写到 $LOG_DIR/<name>.log

set -euo pipefail

# 退出 + 提示
die() {
    log_error "$@"
    exit 1
}

# run_logged <logname> <cmd...>
# 把命令 stdout+stderr 写到 $LOG_DIR/<logname>.log，失败时 tail 30 行
run_logged() {
    local logname="$1"
    shift
    local logfile="${LOG_DIR:-/tmp}/${logname}.log"
    mkdir -p "$(dirname "$logfile")"
    log_cmd "$*"
    if ! "$@" > "$logfile" 2>&1; then
        log_error "Command failed: $*"
        log_error "=== Last 30 lines of $logfile ==="
        tail -30 "$logfile" >&2 || true
        die "See $logfile for full output"
    fi
}

# require_file <path> [<message>]
require_file() {
    local path="$1"
    local msg="${2:-Required file not found: $path}"
    if [ ! -f "$path" ]; then
        die "$msg"
    fi
}

# require_cmd <cmd> [<install-hint>]
require_cmd() {
    local cmd="$1"
    local hint="${2:-Install with: mise install $cmd}"
    if ! command -v "$cmd" >/dev/null 2>&1; then
        die "Required command not found: $cmd ($hint)"
    fi
}

# require_dir <path> [<message>]
require_dir() {
    local path="$1"
    local msg="${2:-Required directory not found: $path}"
    if [ ! -d "$path" ]; then
        die "$msg"
    fi
}
