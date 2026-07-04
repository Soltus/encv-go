package config

import "encoding/json"

// ConfigProvider 定义了获取插件配置的抽象接口
// 任何实现了此接口的结构体都可以为 ENCV 插件提供配置
type ConfigProvider interface {
	GetPluginSettings(pluginName string) (json.RawMessage, error)
}
