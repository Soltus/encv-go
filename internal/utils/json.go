package utils

import (
	"encoding/json"
	"fmt"
)

// 辅助函数。 合并两个 JSON 对象，userConfig 中的键会覆盖 defaults 中的键
func MergeJSONObjects(defaults, userConfig json.RawMessage) (json.RawMessage, error) {
	defaultsMap := make(map[string]interface{})
	userMap := make(map[string]interface{})

	if err := json.Unmarshal(defaults, &defaultsMap); err != nil {
		return nil, fmt.Errorf("invalid default settings JSON: %w", err)
	}
	if err := json.Unmarshal(userConfig, &userMap); err != nil {
		return nil, fmt.Errorf("invalid user settings JSON: %w", err)
	}

	// 合并：用户配置覆盖默认配置
	for key, userValue := range userMap {
		defaultsMap[key] = userValue
	}

	return json.Marshal(defaultsMap)
}
