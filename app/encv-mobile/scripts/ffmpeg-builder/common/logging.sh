#!/usr/bin/env bash
# common/logging.sh - 统一彩色日志
#
# 用法：source "$(dirname "${BASH_SOURCE[0]}")/common/logging.sh"
# 暴露：log_info / log_warn / log_error / log_step / log_cmd
# 自动：检测 NO_COLOR 环境变量；输出到 stderr 不污染 stdout（命令管道友好）

set -euo pipefail

# 颜色（仅在 stdout 是 TTY 时启用）
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ] && command -v tput >/dev/null 2>&1 && [ "$(tput colors 2>/dev/null || echo 0)" -ge 8 ]; then
    _LOG_C_RESET=$'\033[0m'
    _LOG_C_GRAY=$'\033[90m'
    _LOG_C_BLUE=$'\033[34m'
    _LOG_C_GREEN=$'\033[32m'
    _LOG_C_YELLOW=$'\033[33m'
    _LOG_C_RED=$'\033[31m'
    _LOG_C_CYAN=$'\033[36m'
else
    _LOG_C_RESET=''
    _LOG_C_GRAY=''
    _LOG_C_BLUE=''
    _LOG_C_GREEN=''
    _LOG_C_YELLOW=''
    _LOG_C_RED=''
    _LOG_C_CYAN=''
fi

_log_emit() {
    local level="$1"
    local color="$2"
    shift 2
    # stderr 输出，不污染 stdout（便于子进程管道）
    printf '%s %s[%s]%s %s\n' \
        "$(date +'%H:%M:%S')" \
        "$color" "$level" "$_LOG_C_RESET" \
        "$*" >&2
}

log_info()  { _log_emit "INFO"  "$_LOG_C_BLUE"   "$@"; }
log_warn()  { _log_emit "WARN"  "$_LOG_C_YELLOW" "$@"; }
log_error() { _log_emit "ERROR" "$_LOG_C_RED"    "$@"; }
log_ok()    { _log_emit "OK"    "$_LOG_C_GREEN"  "$@"; }
log_step()  { _log_emit "STEP"  "$_LOG_C_CYAN"   "$@"; }
log_cmd()   { _log_emit "CMD"   "$_LOG_C_GRAY"   "$@"; }

# 步骤分隔线
log_section() {
    local title="$*"
    local bar
    bar="$(printf '=%.0s' $(seq 1 60))"
    printf '\n%s %s %s\n' "$bar" "$title" "$bar" >&2
}
