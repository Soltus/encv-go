// internal/v2/plugins/dependencies.go
//
// 插件依赖关系解析：把「装载顺序」从 Plugins 切片的隐式下标顺序，
// 变成插件自己声明的东西（interfaces.DependencyAware）。
//
// 借鉴 Cordis 的「服务声明 + 依赖拓扑」：装载顺序由依赖图决定，而不是由
// 谁恰好写在切片前面决定。取其三件事：
//
//  1. 顺序可推导：提供方一定先于消费方初始化，卸载时严格反过来。
//  2. 失败前置：依赖缺失/成环在装载**之前**就报出来，且一次性报全
//     （逐个报会逼着人反复「改一个、跑一次」）。
//  3. 渐进可用：不声明依赖的插件行为完全不变，排序对它们是稳定的。
//
// 同样是刻意的：这里不引入完整 IoC 容器、不做运行时动态增删插件。

package plugins

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
)

var (
	// ErrMissingDependency 某个插件 Requires 的服务没有任何插件提供。
	ErrMissingDependency = errors.New("plugin dependency not provided")
	// ErrDependencyCycle 依赖关系成环，无法给出任何合法的装载顺序。
	ErrDependencyCycle = errors.New("plugin dependency cycle")
)

// SortByDependencies 按声明的依赖关系对插件做稳定拓扑排序。
//
// 「稳定」的含义：在没有依赖约束的地方，保持入参的原始相对顺序——
// 这样排序不会平白打乱今天已有的装配顺序，diff 才是可解释的。
//
// 返回错误时不返回任何顺序：依赖缺失/成环属于装配错误，继续装载只会
// 得到一个「部分插件没有提供方」的半初始化状态，不如干脆别启动。
func SortByDependencies(list []Plugin) ([]Plugin, error) {
	if len(list) <= 1 {
		return list, nil
	}

	// 1. 建立「服务名 → 提供方下标」索引。
	providers := make(map[string][]int)
	for i, p := range list {
		for _, svc := range providesOf(p) {
			providers[svc] = append(providers[svc], i)
		}
	}

	// 2. 缺失依赖：一次性收集全部，避免「改一个、跑一次」的排障循环。
	var missing []string
	for _, p := range list {
		for _, svc := range requiresOf(p) {
			if _, ok := providers[svc]; ok {
				continue
			}
			missing = append(missing, fmt.Sprintf("plugin %q requires %q, but no plugin provides it", p.Name(), svc))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("%w:\n  - %s", ErrMissingDependency, strings.Join(missing, "\n  - "))
	}

	// 3. Kahn 排序：每轮扫描，把「依赖已全部就绪」的插件按原始下标顺序输出。
	emitted := make([]bool, len(list))
	out := make([]Plugin, 0, len(list))
	for len(out) < len(list) {
		progress := false
		for i, p := range list {
			if emitted[i] || !depsReady(p, providers, emitted) {
				continue
			}
			emitted[i] = true
			out = append(out, p)
			progress = true
		}
		if !progress {
			// 剩下的插件互相等待：必成环（缺失依赖已在第 2 步排除）。
			var names []string
			for i, p := range list {
				if !emitted[i] {
					names = append(names, p.Name())
				}
			}
			return nil, fmt.Errorf("%w: %s", ErrDependencyCycle, strings.Join(names, " -> "))
		}
	}
	return out, nil
}

// depsReady 判断插件依赖的服务是否都已经有提供方被输出。
func depsReady(p Plugin, providers map[string][]int, emitted []bool) bool {
	selfProvided := providesOf(p)
	for _, svc := range requiresOf(p) {
		// 自己提供的能力永远视为已满足（允许「提供并复用」而不判成自环）。
		if containsString(selfProvided, svc) {
			continue
		}
		ok := false
		for _, idx := range providers[svc] {
			if emitted[idx] {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func providesOf(p Plugin) []string {
	d, ok := p.(pluginInterfaces.DependencyAware)
	if !ok {
		return nil
	}
	return nonEmpty(d.Provides())
}

func requiresOf(p Plugin) []string {
	d, ok := p.(pluginInterfaces.DependencyAware)
	if !ok {
		return nil
	}
	return nonEmpty(d.Requires())
}

// nonEmpty 过滤空串与空白：服务名是自由字符串，空串没有意义，
// 不过滤的话会让「忘记填名字」变成一条永远无法满足的依赖。
func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func containsString(in []string, want string) bool {
	for _, s := range in {
		if s == want {
			return true
		}
	}
	return false
}
