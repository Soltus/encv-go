// internal/v2/plugins/assembly.go
//
// 插件装配：把「启用哪些插件、以什么顺序装载」从编译期切片下标变成配置。
//
// 借鉴 Cordis / DeepSeek Harness 的 preset 概念——「一套插件集合」是一个可声明
// 的东西，移动端、桌面端、OpenList 插件各自挑自己要的那套，而不是共用一份
// 硬编码切片。
//
// 与依赖排序（dependencies.go）的分工：
//
//	配置表达「期望顺序」，依赖声明表达「硬约束」；约束赢。
//	所以顺序 = SortByDependencies(SelectEnabled(Plugins, cfg.Enabled))。
//
// 兼容底线：不配置时行为与今天完全一致（全部插件、声明顺序）。

package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// ErrUnknownPlugin 装配配置里出现了不存在的插件名。
var ErrUnknownPlugin = errors.New("unknown plugin in assembly config")

// SelectEnabled 按名字裁剪插件列表。
//
//   - enabled 为空：原样返回全部插件（保持声明顺序），既有行为不变。
//   - enabled 非空：只保留列出的插件，并按配置顺序排列；重复名字按首次出现计。
//   - 出现未知名字：报错并附上可用插件名，绝不静默忽略。
func SelectEnabled(list []Plugin, enabled []string) ([]Plugin, error) {
	if len(enabled) == 0 {
		return list, nil
	}

	byName := make(map[string]Plugin, len(list))
	available := make([]string, 0, len(list))
	for _, p := range list {
		byName[p.Name()] = p
		available = append(available, p.Name())
	}

	var unknown []string
	out := make([]Plugin, 0, len(enabled))
	seen := make(map[string]bool, len(enabled))
	for _, name := range enabled {
		p, ok := byName[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		if seen[name] {
			continue // 重复项：保留首次出现的位置
		}
		seen[name] = true
		out = append(out, p)
	}
	if len(unknown) > 0 {
		sort.Strings(available)
		return nil, fmt.Errorf("%w: %s (available: %s)",
			ErrUnknownPlugin, strings.Join(unknown, ", "), strings.Join(available, ", "))
	}
	return out, nil
}

// InitializePluginsWith 按装配配置初始化全局插件列表。
//
// 两种错误的处理刻意不同（与 InitializeAll 一致）：
//   - 装配错误（未知插件名 / 依赖缺失 / 成环）：直接返回，一个插件都不初始化。
//     这属于「配置写错了」，继续跑只会得到一个说不清的半初始化状态。
//   - 单个插件 Initialize 失败：只标记自己为 StateFailed，错误聚合返回，
//     由调用方决定要不要当成致命错误（服务端的既有哲学是不致命）。
func InitializePluginsWith(ctx context.Context, enabled []string) error {
	selected, err := SelectEnabled(Plugins, enabled)
	if err != nil {
		return err
	}
	// 被裁剪掉的插件登记为 pending：否则它们在状态表里完全不存在，
	// /api 与前端会「看不见」，排障时无法区分「没装」和「装了但没启用」。
	markExcluded(Plugins, selected)

	if err := InitializeAll(ctx, selected); err != nil {
		return err
	}
	if conflicts := ValidateExtensionUniqueness(); len(conflicts) > 0 {
		for _, c := range conflicts {
			slog.Error("container extension conflict detected",
				"extension", c.Extension,
				"conflicting_plugins", strings.Join(c.PluginNames, ", "),
			)
		}
	}
	return nil
}

// markExcluded 把「编译期存在、本次未启用」的插件登记为 StatePending。
func markExcluded(all, selected []Plugin) {
	chosen := make(map[string]bool, len(selected))
	for _, p := range selected {
		chosen[p.Name()] = true
	}
	for _, p := range all {
		if !chosen[p.Name()] {
			SetPluginState(p.Name(), StatePending, nil)
		}
	}
}
