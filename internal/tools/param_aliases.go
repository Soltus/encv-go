// Stage 4 (borrow-nuclear-boy-2026q2)：参数别名容错 + Tool priority 排序 + executeSafe paramHint。
//
// 借鉴自 /tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/ToolRegistry.kt：
//   - L168-196: Tool priority 排序（priorityTools 置顶 0 / requiresConfirmation 最后 2）
//   - L236-258: executeSafe 失败时附 required param hint
//   - L776-798: parseToolParams 容错（JSON 解析失败 fallback emptyMap）
package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ParamAliases 定义工具的"别名 → 主参数"映射。
// 来源：nuclear-boy HANDOVER2.0.md §四 参数别名表。
//
// 用法：
//   aliases := tools.DefaultParamAliases()
//   normalized := tools.NormalizeArgs("read_file", args, aliases)
var DefaultParamAliases = map[string]map[string]string{
	"read_file": {
		"filePath": "path",
		"filename": "path",
	},
	"web_fetch": {
		"link": "url",
		"query": "url",
	},
	"generate_docx": {"output_path": "path"},
	"generate_xlsx": {"output_path": "path"},
	"create_project": {
		"path":         "name",
		"projectName":  "name",
		"project_name": "name",
	},
}

// NormalizeArgs 把 args 里的别名 key 替换为主参数 key。
// 例如 {"filePath": "/a.txt"} → {"path": "/a.txt"}。
// 不修改原 map，返回新 map（避免外部状态污染）。
func NormalizeArgs(toolName string, args map[string]string, aliases map[string]map[string]string) map[string]string {
	if len(args) == 0 {
		return args
	}
	toolAliases, ok := aliases[toolName]
	if !ok || len(toolAliases) == 0 {
		return args
	}

	out := make(map[string]string, len(args))
	for k, v := range args {
		if main, isAlias := toolAliases[k]; isAlias {
			// 防止主参数已被显式设置（保留显式值）
			if _, alreadySet := out[main]; !alreadySet {
				out[main] = v
				continue
			}
		}
		out[k] = v
	}
	return out
}

// RequiredParam 描述一个工具的必填参数。
// 借鉴 nuclear-boy ToolRegistry.kt L22-28 ToolDefinition.parameters。
type RequiredParam struct {
	Name string
	Type string // "string" / "int" / "bool" / "float"
}

// BuildParamHint 当工具调用失败（缺参数）时生成给 LLM 看的错误提示。
// 借鉴 nuclear-boy ToolRegistry.kt L236-258 "示例: ${tool.name}(${tool.parameters.joinToString { "${it.name}=\"...\"" }})"。
//
// 输出格式（LLM 友好）：
//
//   read_file 调用失败: 缺少必填参数 path (string)
//   示例: read_file(path="相对路径或绝对路径")
func BuildParamHint(toolName string, required []RequiredParam, providedArgs map[string]string) string {
	// 找出实际缺失的参数
	var missing []RequiredParam
	for _, p := range required {
		if _, ok := providedArgs[p.Name]; !ok {
			missing = append(missing, p)
		}
	}

	var sb strings.Builder
	if len(missing) > 0 {
		sb.WriteString(fmt.Sprintf("%s 调用失败: 缺少必填参数 ", toolName))
		parts := make([]string, 0, len(missing))
		for _, p := range missing {
			parts = append(parts, fmt.Sprintf("%s (%s)", p.Name, p.Type))
		}
		sb.WriteString(strings.Join(parts, ", "))
	} else {
		sb.WriteString(fmt.Sprintf("%s 调用失败: 参数不合法", toolName))
	}

	// 加示例
	sb.WriteString("\n示例: ")
	sb.WriteString(toolName)
	sb.WriteString("(")
	parts := make([]string, 0, len(required))
	for _, p := range required {
		placeholder := "..."
		switch p.Type {
		case "string":
			placeholder = fmt.Sprintf("%q", "<value>")
		case "int":
			placeholder = "0"
		case "bool":
			placeholder = "true"
		case "float":
			placeholder = "0.0"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", p.Name, placeholder))
	}
	sb.WriteString(strings.Join(parts, ", "))
	sb.WriteString(")")

	return sb.String()
}

// ParseArgsJSON 把 LLM 传入的 argsJSON 解析为 map[string]string。
// 借鉴 nuclear-boy parseToolParams 容错：JSON 解析失败 → 返回空 map，不抛错。
func ParseArgsJSON(argsJSON string) map[string]string {
	if argsJSON == "" {
		return map[string]string{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		// 失败 fallback 空 map（nuclear-boy L778）
		return map[string]string{}
	}

	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}

// PriorityToolWeights 描述工具的排序权重。
// 借鉴 nuclear-boy ToolRegistry.kt L168-196。
type PriorityToolWeights struct {
	// PriorityTools 置顶（权重 0）
	PriorityTools map[string]bool
	// ConfirmTools 置底（权重 2）
	ConfirmTools map[string]bool
}

// SortToolsByPriority 按权重排序工具。
// 排序规则（nuclear-boy L168-196）：
//   - PriorityTools 中的工具 → 权重 0
//   - 其他工具 → 权重 1
//   - ConfirmTools 中的工具 → 权重 2
//   - 权重相同时按工具名字母序
func SortToolsByPriority(toolNames []string, weights PriorityToolWeights) []string {
	type weighted struct {
		name   string
		weight int
	}
	items := make([]weighted, 0, len(toolNames))
	for _, name := range toolNames {
		w := 1
		if weights.PriorityTools[name] {
			w = 0
		} else if weights.ConfirmTools[name] {
			w = 2
		}
		items = append(items, weighted{name, w})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].weight != items[j].weight {
			return items[i].weight < items[j].weight
		}
		return items[i].name < items[j].name
	})

	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.name)
	}
	return out
}
