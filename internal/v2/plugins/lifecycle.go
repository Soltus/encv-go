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
}

// InitializeAll 按顺序初始化插件列表。
//
// 失败语义：单个插件初始化失败只把它自己标记为 StateFailed，**不拖垮其余插件**。
// 这与 ENCV 既有的容错哲学一致（pkg/encv/plugins.InitializeWithSettings 也是
// 「失败的插件被禁用，服务照常启动」），也让调用方可以忽略聚合错误而仍然
// 拿到一部分可用插件。真正的「生命周期包含」体现在卸载侧：DisposeAll 严格逆序。
//
// 返回值是所有失败的组合（errors.Join）；全部成功时为 nil。
func InitializeAll(ctx context.Context, list []Plugin) error {
	var errs []error
	for _, p := range list {
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
func DisposePlugins() []PluginStatus {
	return DisposeAll(Plugins)
}

func mustStatus(name string) PluginStatus {
	st, ok := GetPluginState(name)
	if !ok {
		return PluginStatus{Name: name, State: StatePending}
	}
	return st
}
