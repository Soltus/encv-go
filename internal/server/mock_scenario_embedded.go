// internal/server/mock_scenario_embedded.go
//
// 把 mock_scenarios/builtin/*.yaml 用 go:embed 编译进二进制。
// 替代原来的 agent_mock_scenarios.go（v1 Go 字面量 1119 行）+ agent_mock_v2_scenarios.go
// （v2 Go 字面量 490 行），改用 YAML 单一来源。
//
// 设计原则：
//   - 单一数据源：剧本只存在于 mock_scenarios/builtin/*.yaml
//   - 编译期嵌入：go:embed 把 YAML 编进二进制，无需运行时读文件
//   - 全局一次性解析：包初始化时解析，零运行时开销
//   - v1 / v2 拆分存储：mockScenarios（13 v1）由 NewMockEngine 消费；
//     mockScenariosV2（8 v2）由 MockEngineV2 / 外部 API 消费
package server

import (
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

//go:embed mock_scenarios/builtin/*.yaml
var embeddedBuiltinYAML embed.FS

// mockScenariosBuiltin 编译期内置 v1 剧本（13 个）。
// 文件命名约定：01_xxx.yaml ~ 13_xxx.yaml（无 v2_ 前缀）。
// 替代原来的 agent_mock_scenarios.go 12 个 scenarioXxx() 函数。
var mockScenariosBuiltin []*MockScenario

// mockScenariosV2 编译期内置 v2 剧本（8 个）。
// 文件命名约定：v2_01_xxx.yaml ~ v2_08_xxx.yaml。
// 替代原来的 agent_mock_v2_scenarios.go var mockScenariosV2 字面量。
// 被 agent_api.go / agent_mock_v2.go / 测试代码引用，保留作为 package-level var。
var mockScenariosV2 []*MockScenario

// v2Prefix v2 剧本 YAML 文件名前缀。
const v2Prefix = "v2_"

func init() {
	entries, err := embeddedBuiltinYAML.ReadDir("mock_scenarios/builtin")
	if err != nil {
		panic(fmt.Sprintf("embedded builtin scenarios read dir failed: %v", err))
	}

	v1 := make([]*MockScenario, 0, 16)
	v2 := make([]*MockScenario, 0, 8)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		data, err := embeddedBuiltinYAML.ReadFile("mock_scenarios/builtin/" + name)
		if err != nil {
			slog.Error("embedded builtin: read file failed", "file", name, "err", err)
			continue
		}
		loaded, err := parseLoadedScenarioBytes(data, name)
		if err != nil {
			slog.Error("embedded builtin: parse failed", "file", name, "err", err)
			continue
		}
		if err := loaded.Validate(); err != nil {
			slog.Error("embedded builtin: validation failed, skip", "file", name, "err", err)
			continue
		}
		sc := loaded.ConvertToMockScenario()
		if strings.HasPrefix(name, v2Prefix) {
			v2 = append(v2, sc)
		} else {
			v1 = append(v1, sc)
		}
	}

	// 稳定排序（按 ID）
	sortByID(v1)
	sortByID(v2)

	// 把 default_friendly 强制放第一个（保留旧 Go 字面量顺序契约：
	// NewMockEngine().AllScenarios()[0] == "default_friendly"）
	v1 = putDefaultFriendlyFirst(v1)

	mockScenariosBuiltin = v1
	mockScenariosV2 = v2
	slog.Info("mock scenarios embedded builtin loaded", "v1", len(v1), "v2", len(v2))
}

// sortByID 按 ID 字典序稳定排序。
func sortByID(s []*MockScenario) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].ID < s[j].ID })
}

// putDefaultFriendlyFirst 把 default_friendly 移至切片首部，其他顺序保持。
func putDefaultFriendlyFirst(s []*MockScenario) []*MockScenario {
	for i, sc := range s {
		if sc != nil && sc.ID == "default_friendly" {
			if i == 0 {
				return s
			}
			out := make([]*MockScenario, 0, len(s))
			out = append(out, sc)
			out = append(out, s[:i]...)
			out = append(out, s[i+1:]...)
			return out
		}
	}
	// 不应发生：mock_scenarios/builtin/01_default_friendly.yaml 是必含文件
	return s
}
