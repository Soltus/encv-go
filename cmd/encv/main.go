package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/pkg/encv"
	"github.com/spf13/cobra"
)

// Version 会在编译时通过 -ldflags 注入，如果不是，则默认为 "dev"
// 注入方式：go build -ldflags="-X main.Version=v1.2.3" ./cmd/encv
var Version = "dev"

// 全局变量，由 PersistentPreRun 初始化，供所有子命令使用
var (
	cfg        *config.Config
	rootCtx    context.Context
	configPath string
)

// --- init 函数：添加所有命令到根命令，并定义标志 ---
func init() {
	// 添加子命令
	rootCmd.AddCommand(analyzeV2Cmd)
	rootCmd.AddCommand(manifestV2Cmd)
	rootCmd.AddCommand(kviV2Cmd)
	rootCmd.AddCommand(decryptV2Cmd)
	rootCmd.AddCommand(encryptV2Cmd)
	rootCmd.AddCommand(playV2Cmd)
	addServersCommands(rootCmd)
	addPlatformSpecificCommands(rootCmd)

	// 全局标志
	rootCmd.PersistentFlags().StringP("config", "c", "", "Path to config file")

	// 为命令添加标志
	manifestV2Cmd.Flags().StringP("save", "s", "", "Save Manifest content to a specified JSON file.")
	kviV2Cmd.Flags().StringP("save", "s", "", "Save KVI content to a specified JSON file.")
	decryptV2Cmd.Flags().StringP("password", "p", "", "Password for decryption (overrides config)")
	decryptV2Cmd.Flags().StringP("output", "o", "", "Output directory for decrypted files")
	encryptV2Cmd.Flags().StringP("password", "p", "", "Password for encryption (overrides config)")
	encryptV2Cmd.Flags().StringP("output", "o", "", "Output directory for encrypted files (overrides config)")
	// play-v2 的标志，包含 OS 相关的默认值
	defaultPlayer := "mpv"
	if runtime.GOOS == "windows" {
		defaultPlayer = "mpv.exe"
	}
	playV2Cmd.Flags().StringP("player", "r", defaultPlayer, "Media player to use (e.g., mpv, vlc)")
}

// --- main 函数：入口点，变得非常简洁 ---
func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}
}

// --- 根命令 ---
var rootCmd = &cobra.Command{
	Use:   "encv",
	Short: "ENCV is a tool for encrypting and decrypting files.",
	Long:  `ENCV is a powerful command-line tool for encrypting and decrypting files and directories using a custom container format.`,
	// PersistentPreRun 会在每个子命令运行前执行，非常适合用来做通用的初始化工作
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// 1. 初始化基础 slog（控制台输出），确保在配置加载前 slog 全局可用
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

		// 2. 从 flag 获取可能的配置路径
		configFlag, _ := cmd.Flags().GetString("config")
		var err error
		if configFlag != "" {
			configPath = configFlag
		} else {
			configPath, _ = config.FindConfigPath("")
		}

		cfg, err = config.Load(configPath)
		if err != nil {
			log.Fatalf("Failed to load base config: %v", err)
		}

		// 4. 根据配置重新初始化结构化日志（支持文件输出和级别控制）
		logLevel := logger.LevelInfo
		switch cfg.Log.Level {
		case "debug":
			logLevel = logger.LevelDebug
		case "info":
			logLevel = logger.LevelInfo
		case "warn":
			logLevel = logger.LevelWarn
		case "error":
			logLevel = logger.LevelError
		}

		var logFile string
		if cfg.Log.File != "" {
			logFile = cfg.Log.File
		}

		if err := logger.Init(logLevel, logFile); err != nil {
			slog.Warn("Failed to initialize structured logging, using console defaults", "error", err)
		} else {
			slog.Info("Structured logging initialized",
				slog.String("level", cfg.Log.Level),
				slog.String("file", logFile),
			)
		}

		// 5. 记录启动信息
		slog.Info("encv started",
			slog.String("version", Version),
			slog.String("config_path", configPath),
			slog.Any("args", os.Args),
		)

		rootCtx = config.NewContext(context.Background(), cfg)
	},
}

// --- analyze-v2 命令 ---
var analyzeV2Cmd = &cobra.Command{
	Use:   "analyze-v2 [path to container]",
	Short: "Analyzes a v2 ENCV container file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		encv.Init(rootCtx)
		containerPath := args[0]
		if _, err := encv.AnalyzeContainerV2(rootCtx, containerPath, true); err != nil {
			log.Fatalf("Analysis failed for '%s': %v", containerPath, err)
		}
	},
}

// --- manifest-v2 命令 ---
var manifestV2Cmd = &cobra.Command{
	Use:   "manifest-v2 [path to container]",
	Short: "Extracts and prints the manifest from a v2 container",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		encv.Init(rootCtx)
		containerPath := args[0]
		savePath, _ := cmd.Flags().GetString("save")

		manifestData, err := encv.ExtractManifest_v2(containerPath)
		if err != nil {
			log.Fatalf("Failed to extract Manifest from '%s': %v", containerPath, err)
		}

		if savePath == "" {
			fmt.Println("--- Manifest Content (v2) ---")
			var prettyJSON interface{}
			if err := json.Unmarshal(manifestData, &prettyJSON); err != nil {
				fmt.Printf("%s\n", string(manifestData))
			} else {
				indentedJSON, _ := json.MarshalIndent(prettyJSON, "", "  ")
				fmt.Printf("%s\n", string(indentedJSON))
			}
		} else {
			if err := os.WriteFile(savePath, manifestData, 0644); err != nil {
				log.Fatalf("Failed to save Manifest to '%s': %v", savePath, err)
			}
			utils.PrintSuccess("Manifest content saved to: %s", savePath)
		}
	},
}

// --- kvi-v2 命令 ---
var kviV2Cmd = &cobra.Command{
	Use:   "kvi-v2 [path to container]",
	Short: "Extracts and prints the KVI from a v2 container",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		encv.Init(rootCtx)
		containerPath := args[0]
		savePath, _ := cmd.Flags().GetString("save")

		kviData, err := encv.ExtractKVI_v2(containerPath)
		if err != nil {
			log.Fatalf("Failed to extract KVI from '%s': %v", containerPath, err)
		}

		if savePath == "" {
			fmt.Println("--- KVI Content (v2) ---")
			var prettyJSON interface{}
			if err := json.Unmarshal(kviData, &prettyJSON); err != nil {
				fmt.Printf("%s\n", string(kviData))
			} else {
				indentedJSON, _ := json.MarshalIndent(prettyJSON, "", "  ")
				fmt.Printf("%s\n", string(indentedJSON))
			}
		} else {
			if err := os.WriteFile(savePath, kviData, 0644); err != nil {
				log.Fatalf("Failed to save KVI to '%s': %v", savePath, err)
			}
			utils.PrintSuccess("KVI content saved to: %s", savePath)
		}
	},
}

// --- decrypt-v2 命令 ---
var decryptV2Cmd = &cobra.Command{
	Use:   "decrypt-v2 [path to file/dir]",
	Short: "Decrypts a v2 ENCV container or a directory of them",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputPath := args[0]

		// 【关键修正】从标志获取值，并直接覆盖 cfg 中的值
		passwordFlag, _ := cmd.Flags().GetString("password")
		if passwordFlag != "" {
			cfg.Password = passwordFlag
		}

		outputDirFlag, _ := cmd.Flags().GetString("output")
		finalOutputDir := outputDirFlag
		if finalOutputDir == "" {
			finalOutputDir = "./_decrypted_v2"
		}

		// 如果此时 cfg.Password 仍然为空，则提示用户输入
		if cfg.Password == "" {
			fmt.Print("Enter password: ")
			// 注意：这里需要您自行处理密码输入，例如使用 term.ReadPassword
			// bytePassword, _ := term.ReadPassword(int(syscall.Stdin))
			// cfg.Password = string(bytePassword)
			// fmt.Println()
			// 为了简化示例，这里暂时省略
			log.Fatal("Password is required.")
		}

		if err := os.MkdirAll(finalOutputDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}

		encv.Init(rootCtx)
		if err := encv.DecryptPathV2(rootCtx, inputPath, finalOutputDir); err != nil {
			log.Fatalf("Decryption process failed: %v", err)
		}
		utils.PrintSuccess("Decryption complete. Output: %s", finalOutputDir)
	},
}

// --- encrypt-v2 命令 ---
var encryptV2Cmd = &cobra.Command{
	Use:   "encrypt-v2 [path to file/dir]",
	Short: "Encrypts a file or directory into a v2 ENCV container",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputPath := args[0]
		// 这些标志会覆盖配置文件中的值
		passwordFlag, _ := cmd.Flags().GetString("password")
		outputPathFlag, _ := cmd.Flags().GetString("output")

		if passwordFlag != "" {
			cfg.Password = passwordFlag
		}
		if outputPathFlag != "" {
			cfg.OutputPath = outputPathFlag
		}

		encv.Init(rootCtx)
		if err := os.MkdirAll(cfg.OutputPath, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
		if err := encv.EncryptPathV2(rootCtx, inputPath, cfg.OutputPath); err != nil {
			log.Fatalf("Encryption process failed: %v", err)
		}
		utils.PrintSuccess("Encryption complete. Output: %s", cfg.OutputPath)
	},
}

// --- play-v2 命令 ---
var playV2Cmd = &cobra.Command{
	Use:   "play-v2 [path to container]",
	Short: "Decrypts and plays a media file from a v2 container",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputPath := args[0]
		player, _ := cmd.Flags().GetString("player")

		encv.Init(rootCtx)

		if cfg.Password == "" {
			fmt.Print("-> Please enter the password for decryption: ")
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				cfg.Password = scanner.Text()
			}
			if err := scanner.Err(); err != nil {
				log.Fatalf("Failed to read password from input: %v", err)
			}
			if cfg.Password == "" {
				log.Fatal("Error: Password cannot be empty.")
			}
		}

		utils.PrintInfo("Starting playback: %s (player: %s)", inputPath, player)
		if err := encv.PlayV2(rootCtx, inputPath, player); err != nil {
			log.Fatalf("Playback failed: %v", err)
		}
	},
}

// prepareSubtitles 查找与视频同名的字幕文件，并解密加密的字幕
func prepareSubtitles(videoPath string, cfg *config.Config) ([]SubtitleInfo, error) {
	var subtitles []SubtitleInfo

	// 1. 获取视频文件的目录和基础名
	videoDir := filepath.Dir(videoPath)
	videoBaseName := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))

	// 2. 定义要查找的字幕扩展名
	subExts := []string{".srt", ".ass", ".vtt"}

	// 3. 查找文件
	for _, ext := range subExts {
		potentialSubPath := filepath.Join(videoDir, videoBaseName+ext)

		if _, err := os.Stat(potentialSubPath); err == nil {
			// 是普通字幕，直接使用
			slog.Debug("Found standard subtitle", "path", potentialSubPath)
			subtitles = append(subtitles, SubtitleInfo{Path: potentialSubPath, IsTemp: false})
		}
	}

	return subtitles, nil
}

// SubtitleInfo 存储字幕文件信息
type SubtitleInfo struct {
	Path   string // 字幕文件的路径（原始或临时）
	IsTemp bool   // 是否是解密后的临时文件
}
