//go:build windows
// +build windows

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/registry"
)

// --- 协议相关命令 ---
var registerProtocolCmd = &cobra.Command{
	Use:   "register-protocol",
	Short: "在Windows中注册 encv:// 自定义协议",
	Run: func(cmd *cobra.Command, args []string) {
		if err := RegisterProtocol(cfg); err != nil {
			log.Fatalf("注册协议失败: %v", err)
		}
	},
}

var unregisterProtocolCmd = &cobra.Command{
	Use:   "unregister-protocol",
	Short: "在Windows中取消注册 encv:// 自定义协议",
	Run: func(cmd *cobra.Command, args []string) {
		if err := UnregisterProtocol(); err != nil {
			log.Fatalf("取消注册协议失败: %v", err)
		}
	},
}

// RegisterProtocol 注册 encv:// 自定义协议
// 这个函数将由 main.go 中的命令行入口调用
func RegisterProtocol(cfg *config.Config) error {
	log.Println("-> 正在注册 encv:// 协议...")
	return doRegisterProtocol(cfg)
}

// UnregisterProtocol 取消注册 encv:// 自定义协议
func UnregisterProtocol() error {
	log.Println("-> 正在取消注册 encv:// 协议...")
	return doUnregisterProtocol()
}

// doRegisterProtocol 包含实际的注册逻辑
func doRegisterProtocol(cfg *config.Config) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法获取当前可执行文件路径: %w", err)
	}
	exePathQuoted := fmt.Sprintf(`"%s"`, exePath)

	// 定义协议被激活时要执行的命令
	// 它将调用主程序，并传递 'open-protocol' 子命令和完整的URL
	protocolCommand := fmt.Sprintf(`%s open-protocol "%%1"`, exePathQuoted)

	// --- 注册表操作 ---

	// 1. 创建 HKEY_CLASSES_ROOT\encv 项
	protocolKey, _, err := registry.CreateKey(registry.CLASSES_ROOT, `encv`, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("could not create protocol key: %w", err)
	}
	defer protocolKey.Close()

	// 2. 设置协议的默认值和标识
	if err := protocolKey.SetStringValue("", "URL:ENCV Protocol"); err != nil {
		return fmt.Errorf("could not set protocol description: %w", err)
	}
	if err := protocolKey.SetStringValue("URL Protocol", ""); err != nil {
		return fmt.Errorf("could not set URL Protocol flag: %w", err)
	}

	// 3. 创建 HKEY_CLASSES_ROOT\encv\shell\open\command 项
	commandKey, _, err := registry.CreateKey(registry.CLASSES_ROOT, `encv\shell\open\command`, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("could not create protocol command key: %w", err)
	}
	defer commandKey.Close()

	// 4. 设置要执行的命令
	if err := commandKey.SetStringValue("", protocolCommand); err != nil {
		return fmt.Errorf("could not set protocol command: %w", err)
	}

	fmt.Println("✅ 协议注册成功！")
	return nil
}

// doUnregisterProtocol 包含实际的反注册逻辑
func doUnregisterProtocol() error {
	fmt.Println("-> [Reg] Deleting protocol registry key...")
	if err := registry.DeleteKey(registry.CLASSES_ROOT, "encv"); err != nil {
		return fmt.Errorf("failed to delete protocol key: %w", err)
	}

	fmt.Println("✅ 协议取消注册成功！")
	return nil
}
