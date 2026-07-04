package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/utils"
	encvPlugins "github.com/Soltus/encv-go/internal/v2/plugins"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
)

func IsContainer(path string) bool {
	return encvPlugins.IsContainer(path)
}

func InitializePlugins(ctx context.Context) error {
	return encvPlugins.InitializePlugins(ctx)
}

// Plugins 返回已注册的插件列表（由 internal/v2/plugins 维护）。
// 供 agent 集成等需要直接遍历插件的调用方使用。
func Plugins() []encvPlugins.Plugin {
	return encvPlugins.Plugins
}

func GetPluginMetas() []pluginInterfaces.PluginMeta {
	return encvPlugins.GetPluginMetas()
}

// InitializeWithSettings 是一个公开的初始化函数。
// 它根据传入的配置 map 来初始化插件。
// 对于在 map 中有配置的插件，使用其配置；对于没有配置的插件，使用其默认配置。
// 这确保了所有插件都被初始化，并且完全由外部配置驱动。
func InitializeWithSettings(userSettings map[string]json.RawMessage) error {
	// 创建一个 map 来记录哪些插件已经被处理了
	processedPlugins := make(map[string]bool)

	// --- 阶段 1: 处理所有在 userSettings 中明确配置了的插件 ---
	for pluginName, rawUserSettings := range userSettings {
		plugin, err := findPluginByName(pluginName)
		if err != nil {
			log.Printf("WARN: Skipping configuration for unknown plugin '%s': %v", pluginName, err)
			continue // 跳过未知的插件配置，不中断流程
		}

		log.Printf("Initializing plugin '%s' with user-provided settings.", pluginName)
		initializeSinglePlugin(plugin, rawUserSettings) // 不再检查返回的错误
		processedPlugins[pluginName] = true
	}

	// --- 阶段 2: 处理所有在 userSettings 中没有配置的插件，使用默认配置 ---
	for _, p := range encvPlugins.Plugins {
		pluginName := p.Name()
		if !processedPlugins[pluginName] {
			log.Printf("Initializing plugin '%s' with default settings.", pluginName)
			initializeSinglePlugin(p, nil) // 不再检查返回的错误
		}
	}

	// 【关键】此函数现在总是返回成功，保证不会中断 OpenList 启动
	return nil
}

// findPluginByName 根据名称在 plugins.Plugins 列表中查找插件实例
func findPluginByName(name string) (encvPlugins.Plugin, error) {
	for _, p := range encvPlugins.Plugins {
		if p.Name() == name {
			return p, nil
		}
	}
	return nil, fmt.Errorf("plugin '%s' not found", name)
}

// initializeSinglePlugin 是一个辅助函数，用于初始化单个插件
func initializeSinglePlugin(p encvPlugins.Plugin, rawUserSettings json.RawMessage) {
	pluginName := p.Name()
	var finalSettings json.RawMessage
	var err error

	// 1. 获取插件自身的默认配置
	defaultSettings := p.GetDefaultSettings()

	// 2. 尝试合并配置
	if rawUserSettings != nil {
		finalSettings, err = utils.MergeJSONObjects(defaultSettings, rawUserSettings)
		if err != nil {
			// 【关键】合并失败（通常是用户JSON格式错误），记录警告并使用默认配置
			log.Printf("WARN: Failed to merge settings for plugin '%s' (will use defaults): %v", pluginName, err)
			finalSettings = defaultSettings
		}
	} else {
		finalSettings = defaultSettings
	}

	// 3. 创建 Config 对象
	cfg := &config.Config{
		PluginSettings: map[string]json.RawMessage{
			pluginName: finalSettings,
		},
	}
	ctx := config.NewContext(context.Background(), cfg)

	// 4. 调用插件的 Initialize
	// 【关键】即使插件内部初始化失败，也只记录错误，不中断流程
	if err := p.Initialize(ctx); err != nil {
		log.Printf("ERROR: Plugin '%s' failed to initialize even with default settings. It will be disabled. Error: %v", pluginName, err)
		// 插件初始化失败，意味着它可能无法正常工作（如解密、加密），
		// 但至少服务启动了，管理员可以在日志中看到问题并去修复。
	} else {
		log.Printf("INFO: Plugin '%s' initialized successfully.", pluginName)
	}
}
