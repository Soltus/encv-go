//go:build windows
// +build windows

package main

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/pkg/encv"
	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/registry"
)

// --- openas 命令 ---
var openasCmd = &cobra.Command{
	Use:   "openas",
	Short: "Registers 'Open' action for ENCV files (Windows only)",
	RunE: func(cmd *cobra.Command, args []string) error { // Use RunE to return errors
		if runtime.GOOS != "windows" {
			return fmt.Errorf("the 'openas' command is only available on Windows")
		}
		if err := handleOpenAsCommand(cfg); err != nil {
			return fmt.Errorf("failed to register file associations: %w", err)
		}
		log.Println("✅ File associations for 'Open' action registered successfully!")
		log.Println("You can now double-click on an ENCV file to decrypt it.")
		return nil
	},
}

// --- open-stream 命令 ---
var openStreamCmd = &cobra.Command{
	Use:   "open-stream [path to container]",
	Short: "Streams a media file from a running ENCV server to mpv",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputPath := args[0]
		discoveryStartPort := cfg.Server.Port
		if discoveryStartPort == 0 {
			discoveryStartPort = 1999
			log.Printf("INFO: Server port in config is 0 (any port). Starting discovery from default port %d.", discoveryStartPort)
		} else {
			log.Printf("INFO: Starting discovery from configured port %d.", discoveryStartPort)
		}

		serverAddr, _, err := encv.FindServer(discoveryStartPort, 20)
		if err != nil {
			log.Println("--------------------------------------------------")
			log.Println("🔴 ENCV Server is not running.")
			log.Printf("-> Please start it first by running: encv.exe start-server")
			log.Printf("-> Or check if it's running near the configured port: %d", discoveryStartPort)
			log.Println("--------------------------------------------------")
			os.Exit(1)
		}

		// Assume prepareSubtitles exists and is defined elsewhere
		subtitles, err := prepareSubtitles(inputPath, cfg)
		if err != nil {
			log.Printf("Warning: An error occurred while preparing subtitles: %v. Playing without subtitles.", err)
		}

		encodedPath := url.QueryEscape(inputPath)
		streamURL := fmt.Sprintf("http://%s/stream?file=%s", serverAddr, encodedPath)

		mpvArgs := []string{streamURL}
		for _, sub := range subtitles {
			mpvArgs = append(mpvArgs, fmt.Sprintf("--sub-files=%s", sub.Path))
		}

		log.Printf("-> Starting mpv with arguments: %v", mpvArgs)
		logFile := filepath.Join(os.TempDir(), "encv_mpv.log")
		cmd2 := exec.Command("mpv", append(mpvArgs, "--log-file="+logFile, "--msg-level=all=v")...)

		var out, stderr bytes.Buffer
		cmd2.Stdout = &out
		cmd2.Stderr = &stderr

		if err := cmd2.Run(); err != nil {
			log.Println("--------------------------------------------------")
			log.Println("🔴 Failed to run mpv.")
			log.Printf("Error: %v", err)
			log.Println("--- MPV Stdout ---")
			log.Println(out.String())
			log.Println("--- MPV Stderr ---")
			log.Println(stderr.String())
			log.Println("--------------------------------------------------")
			log.Fatalf("Please check the MPV output above for details.")
		}
		log.Println("-> mpv closed.")
	},
}

// handleOpenAsCommand 在 Windows 注册表中注册文件关联（双击行为）
// 用处不大，有空再优化。
func handleOpenAsCommand(cfg *config.Config) error {
	// exePath, err := os.Executable()
	// if err != nil {
	// 	return fmt.Errorf("failed to get executable path: %w", err)
	// }
	// exePathQuoted := fmt.Sprintf(`"%s"`, exePath)

	// // 定义不同类型的命令模板
	// // 视频和音频使用流式播放
	// streamCommand := fmt.Sprintf(`%s open-stream "%%1"`, exePathQuoted)
	// // 图片和文本使用临时文件
	// tempFileCommand := fmt.Sprintf(`%s open-temp "%%1"`, exePathQuoted)

	// // 定义要注册的扩展名、类型和对应的命令
	// extensionsToRegister := map[string]struct {
	// 	ext     string
	// 	command string
	// }{
	// 	"video": {cfg.BinExtGroup.Video, streamCommand},            // cfg.BinExtGroup 已弃用
	// 	"audio": {cfg.BinExtGroup.Audio, streamCommand},
	// 	"image": {cfg.BinExtGroup.Image, tempFileCommand},
	// 	"text":  {cfg.BinExtGroup.Text, tempFileCommand},
	// }

	// for kind, item := range extensionsToRegister {
	// 	if item.ext == "" {
	// 		log.Printf("Warning: Extension for kind '%s' is not configured, skipping.", kind)
	// 		continue
	// 	}

	// 	log.Printf("-> Registering .%s extension with command: %s", item.ext, item.command)
	// 	if err := registerSingleExtension(item.ext, item.command); err != nil {
	// 		return fmt.Errorf("failed to register .%s: %w", item.ext, err)
	// 	}
	// }

	return nil
}

// registerSingleExtension 为单个扩展名创建注册表项
// 现在接受一个完整的 command 字符串作为参数
func registerSingleExtension(ext, command string) error {
	progID := fmt.Sprintf("encv.%s", ext)

	// --- 注册表操作 ---

	// 1. 创建 HKEY_CLASSES_ROOT\.ext 项
	extKey, _, err := registry.CreateKey(registry.CLASSES_ROOT, `.`+ext, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("could not create extension key: %w", err)
	}
	defer extKey.Close()

	if err := extKey.SetStringValue("", progID); err != nil {
		return fmt.Errorf("could not set extension key's default value: %w", err)
	}

	// 2. 创建 HKEY_CLASSES_ROOT\encv.ext 项
	progIDKey, _, err := registry.CreateKey(registry.CLASSES_ROOT, progID, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("could not create ProgID key: %w", err)
	}
	defer progIDKey.Close()

	if err := progIDKey.SetStringValue("", "ENCV Encrypted File"); err != nil {
		return fmt.Errorf("could not set ProgID description: %w", err)
	}

	// 3. 创建 HKEY_CLASSES_ROOT\encv.ext\shell\open\command 项
	commandKey, _, err := registry.CreateKey(registry.CLASSES_ROOT, progID+`\shell\open\command`, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("could not create command key: %w", err)
	}
	defer commandKey.Close()

	// 设置要执行的命令
	if err := commandKey.SetStringValue("", command); err != nil {
		return fmt.Errorf("could not set command: %w", err)
	}

	return nil
}
