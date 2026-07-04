//go:build windows
// +build windows

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/registry"
)

// --- register / unregister 命令 ---
var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Registers file associations and context menu (Windows only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "windows" {
			return fmt.Errorf("the 'register' command is only available on Windows")
		}
		if err := RegisterFileAssociations(); err != nil {
			return fmt.Errorf("failed to register file associations: %w", err)
		}
		return nil
	},
}

var unregisterCmd = &cobra.Command{
	Use:   "unregister",
	Short: "Unregisters file associations and context menu (Windows only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "windows" {
			return fmt.Errorf("the 'unregister' command is only available on Windows")
		}
		if err := UnregisterFileAssociations(); err != nil {
			return fmt.Errorf("failed to unregister file associations: %w", err)
		}
		return nil
	},
}

// RegisterFileAssociations 注册文件关联和右键菜单
func RegisterFileAssociations() error {
	fmt.Println("-> 正在注册 .sccgv 文件关联...")
	return doRegister()
}

// UnregisterFileAssociations 取消注册文件关联和右键菜单
func UnregisterFileAssociations() error {
	fmt.Println("-> 正在取消注册 .sccgv 文件关联...")
	return doUnregister()
}

// getExtensionsFromConfig 从配置中提取所有需要注册的文件后缀名
func getExtensionsFromConfig() ([]string, error) {
	// exePath, err := os.Executable()
	// if err != nil {
	// 	return nil, fmt.Errorf("无法获取可执行文件路径: %w", err)
	// }
	// exeDir := filepath.Dir(exePath)
	// configPath := filepath.Join(exeDir, "config.user.json")

	// cfg, err := config.Load(configPath)
	// if err != nil {
	// 	return nil, fmt.Errorf("无法加载配置文件 '%s': %w", configPath, err)
	// }

	// 过滤掉可能的空值
	var finalExtensions []string
	for _, ext := range plugins.GetAllRegisteredContainerExtensions() {
		if ext != "." {
			finalExtensions = append(finalExtensions, ext)
		}
	}

	if len(finalExtensions) == 0 {
		return nil, fmt.Errorf("配置文件中没有找到任何有效的文件扩展名")
	}

	return finalExtensions, nil
}

// doRegister 包含实际的注册逻辑
func doRegister() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法获取当前可执行文件路径: %w", err)
	}
	exePathQuoted := fmt.Sprintf(`"%s"`, exePath)

	// 1. 从配置中获取所有需要注册的后缀名
	extensions, err := getExtensionsFromConfig()
	if err != nil {
		return err
	}
	fmt.Printf("-> [Reg] Found extensions in config: %v\n", extensions)

	// 2. 为所有后缀名创建关联，统一指向一个 ProgID
	progIDKey := `encv_auto_file` // 使用一个通用的 ProgID
	for _, ext := range extensions {
		fmt.Printf("-> [Reg] Registering for extension: %s\n", ext)
		k, _, err := registry.CreateKey(registry.CLASSES_ROOT, ext, registry.ALL_ACCESS)
		if err != nil {
			return fmt.Errorf("创建 %s 键失败: %w", ext, err)
		}
		k.Close()
		k, _, err = registry.CreateKey(registry.CLASSES_ROOT, ext, registry.WRITE)
		if err != nil {
			return err
		}
		if err := k.SetStringValue("", progIDKey); err != nil {
			return fmt.Errorf("设置 %s 默认值失败: %w", ext, err)
		}
		k.Close()
	}

	// 3. 创建通用的 ProgID
	fmt.Printf("-> [Reg] Creating ProgID: %s\n", progIDKey)
	if err := createProgID(progIDKey, "ENCV Encrypted File"); err != nil {
		return err
	}

	// 4. 创建右键菜单 (只需要为 ProgID 创建一次)
	shellKey := fmt.Sprintf(`%s\shell`, progIDKey)

	// 4.1 创建 "查看索引" 一级菜单
	fmt.Println("-> [Reg] Creating 'View Index' menu item...")
	viewCmd := fmt.Sprintf(`%s --hide-console kvi "%%1"`, exePathQuoted)
	if err := createSimpleMenuItem(shellKey, "view_index", "查看索引(&I)", viewCmd); err != nil {
		return err
	}

	// 4.2 创建 "解压" 主菜单项
	fmt.Println("-> [Reg] Creating 'Extract' main menu item...")
	extractMenuKey := fmt.Sprintf(`%s\extract_menu`, shellKey)
	k, _, err := registry.CreateKey(registry.CLASSES_ROOT, extractMenuKey, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("创建 '解压' 主菜单失败: %w", err)
	}
	defer k.Close()

	if err := k.SetStringValue("", "解压(&E)"); err != nil {
		return fmt.Errorf("设置 '解压' 主菜单默认值失败: %w", err)
	}
	if err := k.SetStringValue("MUIVerb", "解压(&E)"); err != nil {
		return fmt.Errorf("设置 '解压' 主菜单 MUIVerb 失败: %w", err)
	}
	if err := k.SetStringValue("ExtendedSubCommandsKey", "encv_extract_menu"); err != nil {
		return fmt.Errorf("设置 '解压' 主菜单 ExtendedSubCommandsKey 失败: %w", err)
	}

	// 4.3 在 HKCR 根目录下创建子菜单的定义
	fmt.Println("-> [Reg] Creating submenu definitions under HKCR...")
	subMenuRootKey := `encv_extract_menu`
	k, _, err = registry.CreateKey(registry.CLASSES_ROOT, subMenuRootKey, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("创建子菜单根键失败: %w", err)
	}
	defer k.Close()

	subMenuShellKey := fmt.Sprintf(`%s\shell`, subMenuRootKey)
	extractSubItems := map[string]struct{ label, command string }{
		"extract_to_folder": {"解压到指定文件夹(&F)", fmt.Sprintf(`%s --hide-console decrypt --mode=to-folder "%%1"`, exePathQuoted)},
		"extract_current":   {"解压到当前文件夹(&C)", fmt.Sprintf(`%s --hide-console decrypt --mode=here "%%1"`, exePathQuoted)},
		"extract_same":      {"解压到同名文件夹(&S)", fmt.Sprintf(`%s --hide-console decrypt --mode=to-subfolder "%%1"`, exePathQuoted)},
	}

	for name, item := range extractSubItems {
		fmt.Printf("-> [Reg] Creating submenu item: %s\n", name)
		if err := createSimpleMenuItem(subMenuShellKey, name, item.label, item.command); err != nil {
			return err
		}
	}

	fmt.Println("✅ 注册成功！")
	return nil
}

// doUnregister 包含实际的反注册逻辑
func doUnregister() error {
	// 1. 尝试从配置中获取所有后缀名
	extensions, err := getExtensionsFromConfig()
	// progIDKey := `encv_auto_file`
	if err != nil {
		fmt.Printf("-> [Warn] 无法从配置加载扩展名: %v。将尝试删除硬编码的键。\n", err)
		// 如果加载失败，回退到删除一组已知的键
		extensions = []string{".sccgt", ".sccgi", ".sccga", ".sccgv", ".sccgf"}
	}
	fmt.Printf("-> [Reg] Unregistering extensions: %v\n", extensions)

	fmt.Println("-> [Reg] Deleting registry keys...")

	// 2. 删除所有注册的后缀名
	for _, ext := range extensions {
		fmt.Printf("-> [Reg] Deleting extension key: %s\n", ext)
		registry.DeleteKey(registry.CLASSES_ROOT, ext)
	}

	// 3. 删除 ProgID 和所有子菜单
	keysToDelete := []string{
		`encv_extract_menu\shell\extract_to_folder\command`,
		`encv_extract_menu\shell\extract_to_folder`,
		`encv_extract_menu\shell\extract_current\command`,
		`encv_extract_menu\shell\extract_current`,
		`encv_extract_menu\shell\extract_same\command`,
		`encv_extract_menu\shell\extract_same`,
		`encv_extract_menu\shell`,
		`encv_extract_menu`,
		`encv_auto_file\shell\extract_menu\command`,
		`encv_auto_file\shell\extract_menu`,
		`encv_auto_file\shell\view_index\command`,
		`encv_auto_file\shell\view_index`,
		`encv_auto_file\shell`,
		`encv_auto_file`,
	}

	for _, key := range keysToDelete {
		fmt.Printf("-> [Reg] Deleting: %s\n", key)
		registry.DeleteKey(registry.CLASSES_ROOT, key)
	}

	fmt.Println("✅ 取消注册成功！")
	return nil
}

// createSimpleMenuItem 是一个辅助函数，用于创建菜单项及其命令
func createSimpleMenuItem(basePath, name, label, command string) error {
	menuKey := fmt.Sprintf(`%s\%s`, basePath, name)
	k, _, err := registry.CreateKey(registry.CLASSES_ROOT, menuKey, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("创建菜单项 %s 失败: %w", name, err)
	}
	defer k.Close()

	if err := k.SetStringValue("", label); err != nil {
		return fmt.Errorf("设置菜单项 %s 标签失败: %w", name, err)
	}

	cmdKey := fmt.Sprintf(`%s\command`, menuKey)
	cmdK, _, err := registry.CreateKey(registry.CLASSES_ROOT, cmdKey, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("创建命令 %s 失败: %w", name, err)
	}
	defer cmdK.Close()

	if err := cmdK.SetStringValue("", command); err != nil {
		return fmt.Errorf("设置命令 %s 失败: %w", name, err)
	}
	return nil
}

func createProgID(path, description string) error {
	k, _, err := registry.CreateKey(registry.CLASSES_ROOT, path, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("创建 ProgID %s 失败: %w", path, err)
	}
	defer k.Close()

	if err := k.SetStringValue("", description); err != nil {
		return fmt.Errorf("设置 ProgID %s 描述失败: %w", path, err)
	}
	return nil
}
