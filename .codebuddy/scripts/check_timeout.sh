#!/bin/bash
# 命令超时检查工具
# 用法: ./check_timeout.sh "curl https://example.com"

check_and_fix_command() {
    local cmd="$1"
    local fixed_cmd=""
    
    # 如果已经包含超时参数，直接返回原命令
    if echo "$cmd" | grep -Eq '\-m[[:space:]]+[0-9]+|--max-time[[:space:]]+[0-9]+|-w[[:space:]]+[0-9]+|--timeout[[:space:]]+[0-9]+'; then
        echo "$cmd"
        exit 0
    fi
    
    # curl/wget 添加 --max-time 30
    if echo "$cmd" | grep -Eq '^curl[[:space:]]|^wget[[:space:]]'; then
        fixed_cmd=$(echo "$cmd" | sed 's/^\(curl\|wget\)/\1 --max-time 30/')
        echo "$fixed_cmd"
        exit 0
    fi
    
    # ssh 添加 -o ConnectTimeout=10
    if echo "$cmd" | grep -Eq '^ssh[[:space:]]'; then
        fixed_cmd=$(echo "$cmd" | sed 's/^\(ssh\)/\1 -o ConnectTimeout=10/')
        echo "$fixed_cmd"
        exit 0
    fi
    
    # nc/netcat 添加 -w 5
    if echo "$cmd" | grep -Eq '^nc[[:space:]]|^netcat[[:space:]]'; then
        fixed_cmd=$(echo "$cmd" | sed 's/^\(nc\|netcat\)/\1 -w 5/')
        echo "$fixed_cmd"
        exit 0
    fi
    
    # ping 添加 -w 5
    if echo "$cmd" | grep -Eq '^ping[[:space:]]'; then
        fixed_cmd=$(echo "$cmd" | sed 's/^\(ping\)/\1 -w 5/')
        echo "$fixed_cmd"
        exit 0
    fi
    
    # 默认返回原命令（不强制修改未知命令）
    echo "$cmd"
}

check_and_fix_command "$1"