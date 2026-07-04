package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/pkg/encv"
	"github.com/spf13/cobra"
)

// 剧本外置 spec：CLI flag — 把外部目录注入到 agent_settings.mock_scenarios_dir。
// 优先级（高→低）：flag > config.user.json > config.dev.json
// 留空 = 走 Go 字面量 fallback（保持向后兼容）。
var (
	flagMockScenariosDir      string
	flagMockScenariosHotReload bool
)

func init() {
	startCmd.Flags().StringVar(&flagMockScenariosDir, "mock-scenarios-dir", "",
		"Path to a directory containing mock scenario YAML/JSON files. "+
			"Empty = use Go literal fallback. See internal/server/mock_scenarios/SCHEMA.md.")
	startCmd.Flags().BoolVar(&flagMockScenariosHotReload, "mock-scenarios-hot-reload", false,
		"Enable fsnotify hot-reload for the mock scenarios directory. "+
			"Requires --mock-scenarios-dir to be set.")
}

func addServersCommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(startCmd)
}

// applyMockScenariosFlags 把 CLI flag 注入 cfg.Agent（如果 flag 非零值）。
// 注意：cfg 是 *config.Config 指针；通过 s.getAgentConfig() 在 server 侧读取。
// 这里直接修改 cfg 内存对象（不改配置文件），进程退出后消失。
func applyMockScenariosFlags() {
	if flagMockScenariosDir == "" && !flagMockScenariosHotReload {
		return
	}
	if cfg == nil {
		return
	}
	// AgentSettings 是 json.RawMessage — 解析为结构体、改字段、再序列化。
	// 这一段写得略冗长但保持对空值/格式错误的容错。
	raw := cfg.AgentSettings
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var agentMap map[string]interface{}
	if err := json.Unmarshal(raw, &agentMap); err != nil {
		slog.Warn("applyMockScenariosFlags: invalid agent_settings json, skipping",
			"error", err)
		return
	}
	if agentMap == nil {
		agentMap = map[string]interface{}{}
	}
	if flagMockScenariosDir != "" {
		agentMap["mock_scenarios_dir"] = flagMockScenariosDir
	}
	if flagMockScenariosHotReload {
		agentMap["mock_scenarios_hot_reload"] = true
	}
	updated, err := json.Marshal(agentMap)
	if err != nil {
		slog.Warn("applyMockScenariosFlags: marshal failed", "error", err)
		return
	}
	cfg.AgentSettings = updated
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts ENCV server and keeps it running in the foreground",
	Run: func(cmd *cobra.Command, args []string) {
		encv.Init(rootCtx)
		// 把 CLI flag 注入 cfg 后再 NewServer（loader 在 NewServer 内读 agent_settings）
		applyMockScenariosFlags()
		s := encv.NewServer(rootCtx, configPath)
		backendAddr, err := s.Start(Version)
		if err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}

		utils.PrintSection("Server Started")
		utils.PrintKV("Serving files", cfg.Server.Dir)
		utils.PrintKV("API endpoint", fmt.Sprintf("http://%s", backendAddr))
		utils.PrintKV("WebDAV endpoint", fmt.Sprintf("http://%s/%s", backendAddr, cfg.Webdav.Root))
		if len(cfg.Proxy.Sites) > 0 {
			utils.PrintKV("OpenList Sites", fmt.Sprintf("http://%s/openlist/sites", backendAddr))
		}
		// 打印剧本状态：演示团队最关心的信息
		if s.ScenariosDir() != "" {
			utils.PrintKV("Mock scenarios", fmt.Sprintf("YAML @ %s", s.ScenariosDir()))
		} else {
			utils.PrintKV("Mock scenarios", "builtin (Go literal fallback)")
		}
		utils.PrintSection("How to Play")
		utils.PrintInfo("Press Ctrl+C to stop the server")

		select {}
	},
}
