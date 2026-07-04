package types

import (
	"encoding/json"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// ContainerPatch 定义了用于覆盖容器元数据（Manifest 和 Header）的配置。
type ContainerPatch struct {
	// Index 是一个通用的 Map，用于覆盖 Manifest.KVI.Index 中的字段
	// 程序会根据 Manifest.Kind (如 video) 将此 map 映射/合并到具体的结构体中
	Index map[string]interface{} `json:"index" yaml:"index"`

	// SpecialID 定义了用于覆盖 EnvelopeHeaderV3.SpecialID 的数据
	SpecialID *SpecialIDPatch `json:"special_id" yaml:"special_id"`
}

// SpecialIDPatch 定义了 EnvelopeHeaderV3 特殊 ID 的覆盖配置
// 为了用户友好，Content 字段接收原始结构（用于 CBOR）或字符串（用于 Raw），
// 而不是 Base64 编码的字符串。
type SpecialIDPatch struct {
	Type    string      `json:"type" yaml:"type"`       // "cbor" 或 "raw"
	Content interface{} `json:"content" yaml:"content"` // 用户输入的原始内容
}

// ApplyContainerPatch 将补丁应用到现有的 Manifest 和 SpecialID 配置中
// 这是一个核心转换函数，将通用的 YAML 配置映射到具体的二进制结构。
// 返回：
//   - manifest: 应用补丁后的新 Manifest
//   - idBytes: 序列化后的 SpecialID 字节数据 (CBOR/Raw bytes)，供 Packer 生成 Header
//   - error: 错误信息
func ApplyContainerPatch(
	originalManifest *Manifest,
	patch *ContainerPatch,
) (*Manifest, []byte, error) {
	if patch == nil {
		return originalManifest, nil, nil
	}

	// 1. 处理 Index (Manifest) 的覆盖
	newManifest, err := applyIndexPatch(originalManifest, patch.Index)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply index patch: %w", err)
	}

	// 2. 处理 SpecialID (Header) 的覆盖
	// 返回序列化好的 ID 字节，供上层（Packer）使用
	idBytes, err := parseSpecialIDPatch(patch.SpecialID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse special_id patch: %w", err)
	}

	return newManifest, idBytes, nil
}

// applyIndexPatch 处理 Index 的覆盖逻辑
func applyIndexPatch(original *Manifest, indexPatch map[string]interface{}) (*Manifest, error) {
	if len(indexPatch) == 0 {
		return original, nil
	}

	// 1. 反序列化 KVI 的当前状态 (RawMessage -> Map)
	var kviMap map[string]interface{}
	if err := json.Unmarshal(original.KVI, &kviMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal KVI for patching: %w", err)
	}

	// 2. 确定目标字段名 (如 "video_index", "text_index")
	// Manifest.Kind 是 "video"，所以字段名应该是 "video_index"
	targetKey := fmt.Sprintf("%s_index", original.Kind)

	// 3. 获取当前 Index
	// 如果当前 KVI 中没有该字段，初始化为空 map
	originalIndex, exists := kviMap[targetKey].(map[string]interface{})
	if !exists {
		// 尝试从 map[string]interface{} 中提取，确保类型断言正确
		if val, ok := kviMap[targetKey]; ok {
			if m, ok := val.(map[string]interface{}); ok {
				originalIndex = m
			}
		} else {
			// 如果字段完全不存在，初始化为空
			originalIndex = make(map[string]interface{})
			kviMap[targetKey] = originalIndex
		}
	}
	if originalIndex == nil {
		originalIndex = make(map[string]interface{})
	}

	// 4. 合并 (使用深度合并，确保嵌套字段也被覆盖)
	deepMerge(originalIndex, indexPatch)

	// 5. 更新 kviMap
	kviMap[targetKey] = originalIndex

	// 6. 序列化回 Manifest
	newKVIBytes, err := json.Marshal(kviMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal patched KVI: %w", err)
	}

	// 7. 创建新 Manifest
	newManifest := *original // 复制原结构
	newManifest.KVI = json.RawMessage(newKVIBytes)

	return &newManifest, nil
}

// parseSpecialIDPatch 解析 SpecialID 配置并返回字节
// 支持 type="cbor" (内容为 Map) 和 type="raw" (内容为 String)
func parseSpecialIDPatch(patch *SpecialIDPatch) ([]byte, error) {
	if patch == nil {
		return nil, nil
	}

	switch patch.Type {
	case "cbor":
		// 将用户提供的 Map 结构序列化为 CBOR 字节
		if patch.Content == nil {
			return nil, fmt.Errorf("special_id content is required for type 'cbor'")
		}
		return cbor.Marshal(patch.Content)

	case "raw":
		// 将用户提供的字符串转换为字节
		if patch.Content == nil {
			return nil, fmt.Errorf("special_id content is required for type 'raw'")
		}
		strVal, ok := patch.Content.(string)
		if !ok {
			return nil, fmt.Errorf("special_id content must be a string for type 'raw'")
		}
		return []byte(strVal), nil

	default:
		return nil, fmt.Errorf("unknown special_id type: %s", patch.Type)
	}
}

// deepMerge 简单的深度合并实现
// 将 src map 递归合并到 dst map 中，覆盖相同键的值
func deepMerge(dst map[string]interface{}, src map[string]interface{}) {
	for k, srcVal := range src {
		if dstVal, exists := dst[k]; exists {
			// 如果两边都是 Map，递归合并
			if dstMap, ok := dstVal.(map[string]interface{}); ok {
				if srcMap, ok := srcVal.(map[string]interface{}); ok {
					deepMerge(dstMap, srcMap)
					continue
				}
			}
			// 类型不匹配或非 Map 类型，直接覆盖
			dst[k] = srcVal
		} else {
			// 新增键
			dst[k] = srcVal
		}
	}
}
