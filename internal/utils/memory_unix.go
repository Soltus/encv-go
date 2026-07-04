//go:build !windows

package utils

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// getAvailableMemoryFromSys 尝试通过多种方式获取可用内存
func getAvailableMemoryFromSys() uint64 {
	// 1. 优先尝试使用 golang.org/x/sys/unix
	if mem := getFromSysinfo(); mem > 0 {
		return mem
	}

	// 2. 降级到解析 /proc/meminfo
	if mem := getFromProcMeminfo(); mem > 0 {
		return mem
	}

	return 0
}

// getFromSysinfo 使用 unix.Sysinfo 获取内存
func getFromSysinfo() uint64 {
	// windows 上忽略 linter 的提示
	var info unix.Sysinfo_t
	err := unix.Sysinfo(&info)
	if err != nil {
		// 记录错误，但不要中断，让下一个方法尝试
		// fmt.Printf("WARN: unix.Sysinfo failed: %v\n", err)
		return 0
	}
	// info.Freeram 是可用物理内存，单位是内存单元
	// info.Unit 是内存单元的大小（字节）
	return info.Freeram * uint64(info.Unit)
}

// getFromProcMeminfo 通过解析 /proc/meminfo 获取内存
func getFromProcMeminfo() uint64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		// fmt.Printf("WARN: failed to open /proc/meminfo: %v\n", err)
		return 0
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// 查找 "MemAvailable:" 这一行
		if fields[0] == "MemAvailable:" {
			// 值在第二个字段，单位是 KB
			val, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0
			}
			// 转换为字节
			return val * 1024
		}
	}

	return 0
}
