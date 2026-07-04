package interfaces

// SettingField 描述了插件所需的单个配置项的元信息
type SettingField struct {
	// Key 是配置项的键，例如 "ext", "chunk_size_mb"
	Key string
	// Type 是配置项的类型，例如 "string", "bool", "number", "text"
	Type string
	// DefaultValue 是配置项的默认值
	DefaultValue interface{}
	// Help 是对配置项的人类可读描述
	Help string
	// Options 仅当 Type 为 "select" 时有效，提供可选项列表
	Options []string
}

// PluginMeta 描述了一个插件的完整配置元信息
type PluginMeta struct {
	// Name 是插件的唯一标识符
	Name string
	// SettingFields 是该插件所需的所有配置项的详细列表
	SettingFields []SettingField
}
