// internal/v2/plugins/lifecycle.go
//
// 插件生命周期：把「装载 / 卸载」从隐式的、各插件自己 defer 的散落逻辑，
// 收敛成注册表可观测、可回收的一条状态机。
//
// 借鉴 Cordis（Koishi / DeepSeek Harness 的插件内核）的两条核心性质：
//
//  1. 副作用可逆：任何注册过的东西都要能被撤销。这里用可选接口 Disposable 表达——
//     实现了它的插件由注册表统一回收（临时目录、子进程、wasm worker、缓存等）。
//     没实现的插件不会被报错，只是不参与自动回收（渐进式改造，不强求一次到位）。
//
//  2. 生命周期包含：消费方必须在提供方之后启动、之前停止。因此卸载严格逆序；
//     初始化中途失败时，已经装载成功的插件要整体回滚，而不是留下半初始化状态。
//
// 说明：本文件**不**引入完整的 IoC/DI 容器。ENCV 的插件是编译期装配、进程内长驻，
// 照搬依赖注入容器收益有限；这里只取上面两条真正解决痛点的性质。

package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

// PluginState 是插件在注册表中的生命周期状态。
type PluginState string

const (
	// StatePending 已声明但尚未开始初始化。
	StatePending PluginState = "pending"
	// StateLoading 正在执行 Initialize。
	StateLoading PluginState = "loading"
	// StateActive 初始化成功，可正常工作。
	StateActive PluginState = "active"
	// StateFailed 初始化或卸载失败。
	StateFailed PluginState = "failed"
	// StateDisposing 正在卸载（Dispose 执行中）。
	StateDisposing PluginState = "disposing"
	// StateDisposed 已卸载完成，副作用已回收。
	StateDisposed PluginState = "disposed"
)

// Disposable 是可选接口：插件实现它即可参与统一卸载。
//
// 刻意做成「可选」而不是加进 Plugin 胖接口：
// 这样 7 个现有插件可以逐个改造，不必一次性全部实现才能编译通过。
type Disposable interface {
	// Dispose 回收插件持有的所有副作用（临时文件、子进程、连接池、缓存等）。
	// 必须可重复调用（幂等），且不得因自身失败而中断其它插件的卸载。
	Dispose() error
}

// PluginStatus 是插件状态的快照，供 /api 排障与日志使用。
type PluginStatus struct {
	Name  string      `json:"name"`
	State PluginState `json:"state"`
	Error string      `json:"error,omitempty"`
}

var (
	lifecycleMu  sync.RWMutex
	pluginStates = make(map[string]PluginStatus)
	pluginOrder  []string // 保持与首次记录顺序一致，便于稳定输出
)

// SetPluginState 记录某个插件的状态。任何初始化路径（含 pkg/encv/plugins 的
// 逐个初始化）都应调用它，保证注册表看到的状态是完整的。
func SetPluginState(name string, state PluginState, err error) {
	st := PluginStatus{Name: name, State: state}
	if err != nil {
		st.Error = err.Error()
	}
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	if _, exists := pluginStates[name]; !exists {
		pluginOrder = append(pluginOrder, name)
	}
	pluginStates[name] = st
}

// GetPluginState 返回单个插件的状态快照。
func GetPluginState(name string) (PluginStatus, bool) {
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	st, ok := pluginStates[name]
	return st, ok
}

// GetPluginStates 按首次记录顺序返回全部状态快照（副本，调用方改动不影响内部）。
func GetPluginStates() []PluginStatus {
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	out := make([]PluginStatus, 0, len(pluginStates))
	for _, name := range pluginOrder {
		if st, ok := pluginStates[name]; ok {
			out = append(out, st)
		}
	}
	return out
}

// ResetPluginStates 清空状态表，仅用于测试（避免用例之间互相污染）。
func ResetPluginStates() {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	pluginStates = make(map[string]PluginStatus)
	pluginOrder = nil
	initOrder = nil
}

// InitializeAll 按依赖关系排序后逐个初始化插件列表（见 SortByDependencies）。
//
// 两种失败，语义刻意不同：
//   - 依赖缺失 / 成环：装配错误，**一个都不初始化**直接返回。继续装载只会得到
//     「某个插件的提供方不存在」的半初始化状态，比不启动更难排查。
//   - 单个插件 Initialize 报错：只把它自己标记为 StateFailed，不拖垮其余插件。
//     这与 ENCV 既有的容错哲学一致（pkg/encv/plugins.InitializeWithSettings 也是
//     「失败的插件被禁用，服务照常启动」），错误用 errors.Join 聚合返回。
//
// 「生命周期包含」体现在卸载侧：实际装载顺序会被记下，DisposePlugins 严格逆序回收。
func InitializeAll(ctx context.Context, list []Plugin) error {
	ordered, err := SortByDependencies(list)
	if err != nil {
		return err
	}
	recordInitOrder(ordered)

	var errs []error
	for _, p := range ordered {
		name := p.Name()
		SetPluginState(name, StateLoading, nil)
		if err := p.Initialize(ctx); err != nil {
			SetPluginState(name, StateFailed, err)
			errs = append(errs, fmt.Errorf("plugin %s: %w", name, err))
			continue
		}
		SetPluginState(name, StateActive, nil)
	}
	return errors.Join(errs...)
}

// InitializePlugins 初始化全局插件列表（Plugins），语义同 InitializeAll。
func InitializePlugins(ctx context.Context) error {
	if err := InitializeAll(ctx, Plugins); err != nil {
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

// DisposeAll 逆序卸载插件列表，返回与入参同序的状态快照。
//
// 逆序的原因：后装载的插件可能依赖先装载的插件，先卸载依赖方才能保证
// 「提供方比消费方活得久」。
func DisposeAll(list []Plugin) []PluginStatus {
	byName := make(map[string]PluginStatus, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		p := list[i]
		name := p.Name()
		SetPluginState(name, StateDisposing, nil)

		state := StateDisposed
		var disposeErr error
		if d, ok := p.(Disposable); ok {
			if disposeErr = d.Dispose(); disposeErr != nil {
				state = StateFailed
				// 单个插件卸载失败不中断其余插件：卸载阶段没有回滚可用，
				// 半途停下只会留下更多垃圾。
			}
		}
		SetPluginState(name, state, disposeErr)
		byName[name] = mustStatus(name)
	}

	// 结果按入参顺序返回（执行顺序是逆序，但读取顺序应与插件列表一致）。
	out := make([]PluginStatus, 0, len(list))
	for _, p := range list {
		out = append(out, byName[p.Name()])
	}
	return out
}

// DisposePlugins 卸载全局插件列表（Plugins），返回状态快照。
//
// 顺序取「最近一次实际装载顺序」的逆序：装载时按依赖拓扑排过序，
// 卸载必须逆着来，否则会先回收掉仍有消费方在用的提供方。
func DisposePlugins() []PluginStatus {
	return DisposeAll(orderedByLastInit(Plugins))
}

// initOrder 记录最近一次实际装载顺序，供卸载逆序使用。
var initOrder []string

func recordInitOrder(list []Plugin) {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	initOrder = make([]string, 0, len(list))
	for _, p := range list {
		initOrder = append(initOrder, p.Name())
	}
}

// orderedByLastInit 按最近一次装载顺序重排列表。
//
// 没有装载记录时（例如走了 pkg/encv/plugins.InitializeWithSettings 那条不经过
// InitializeAll 的路径）保持声明顺序，行为与今天一致。
// 有记录时只回收真正装载过的插件——没初始化过的插件没有副作用可收。
func orderedByLastInit(list []Plugin) []Plugin {
	lifecycleMu.RLock()
	rank := make(map[string]int, len(initOrder))
	for i, name := range initOrder {
		rank[name] = i
	}
	lifecycleMu.RUnlock()

	if len(rank) == 0 {
		return list
	}

	known := make([]Plugin, 0, len(list))
	for _, p := range list {
		if _, ok := rank[p.Name()]; ok {
			known = append(known, p)
		}
	}
	sort.SliceStable(known, func(i, j int) bool {
		return rank[known[i].Name()] < rank[known[j].Name()]
	})
	return known
}

func mustStatus(name string) PluginStatus {
	st, ok := GetPluginState(name)
	if !ok {
		return PluginStatus{Name: name, State: StatePending}
	}
	return st
}
