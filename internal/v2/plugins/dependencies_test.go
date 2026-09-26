package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// depPlugin 是声明了依赖关系的可测插件，同时记录自己被初始化/卸载的顺序。
type depPlugin struct {
	basePlugin
	provides     []string
	requires     []string
	initOrder    *[]string
	disposeOrder *[]string
}

func (p *depPlugin) Provides() []string { return p.provides }
func (p *depPlugin) Requires() []string { return p.requires }

func (p *depPlugin) Initialize(ctx context.Context) error {
	if p.initOrder != nil {
		*p.initOrder = append(*p.initOrder, p.name)
	}
	return p.basePlugin.Initialize(ctx)
}

func (p *depPlugin) Dispose() error {
	if p.disposeOrder != nil {
		*p.disposeOrder = append(*p.disposeOrder, p.name)
	}
	return nil
}

func names(list []Plugin) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.Name())
	}
	return out
}

func TestSortByDependencies_ProviderGoesFirst(t *testing.T) {
	// 声明顺序里消费方在前，排序后提供方必须被提到前面。
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "consumer"}, requires: []string{"crypto"}},
		&depPlugin{basePlugin: basePlugin{name: "provider"}, provides: []string{"crypto"}},
	}

	got, err := SortByDependencies(list)
	require.NoError(t, err)
	assert.Equal(t, []string{"provider", "consumer"}, names(got))
}

func TestSortByDependencies_KeepsOriginalOrderWhenUnconstrained(t *testing.T) {
	// 稳定性：无依赖约束时不应打乱既有装配顺序，否则 diff 无法解释。
	list := []Plugin{
		&basePlugin{name: "c"},
		&basePlugin{name: "a"},
		&depPlugin{basePlugin: basePlugin{name: "b"}, provides: []string{"x"}},
	}
	got, err := SortByDependencies(list)
	require.NoError(t, err)
	assert.Equal(t, []string{"c", "a", "b"}, names(got))
}

func TestSortByDependencies_Chain(t *testing.T) {
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "c"}, requires: []string{"b.svc"}},
		&depPlugin{basePlugin: basePlugin{name: "a"}, provides: []string{"a.svc"}},
		&depPlugin{basePlugin: basePlugin{name: "b"}, provides: []string{"b.svc"}, requires: []string{"a.svc"}},
	}
	got, err := SortByDependencies(list)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, names(got))
}

func TestSortByDependencies_SelfProvidedIsSatisfied(t *testing.T) {
	// 「我提供并复用某个能力」不应被判成自环。
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "self"}, provides: []string{"s"}, requires: []string{"s"}},
	}
	got, err := SortByDependencies(list)
	require.NoError(t, err)
	assert.Equal(t, []string{"self"}, names(got))
}

func TestSortByDependencies_MissingDependencyReportsAll(t *testing.T) {
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "a"}, requires: []string{"nope"}},
		&depPlugin{basePlugin: basePlugin{name: "b"}, requires: []string{"also-nope"}},
	}
	_, err := SortByDependencies(list)
	require.ErrorIs(t, err, ErrMissingDependency)
	// 一次性报全，而不是「改一个、跑一次」。
	assert.Contains(t, err.Error(), `"a" requires "nope"`)
	assert.Contains(t, err.Error(), `"b" requires "also-nope"`)
}

func TestSortByDependencies_Cycle(t *testing.T) {
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "a"}, provides: []string{"a.svc"}, requires: []string{"b.svc"}},
		&depPlugin{basePlugin: basePlugin{name: "b"}, provides: []string{"b.svc"}, requires: []string{"a.svc"}},
	}
	_, err := SortByDependencies(list)
	require.ErrorIs(t, err, ErrDependencyCycle)
	assert.Contains(t, err.Error(), "a")
	assert.Contains(t, err.Error(), "b")
}

func TestSortByDependencies_EmptyServiceNamesIgnored(t *testing.T) {
	// 服务名是自由字符串，空串不该变成一条永远无法满足的依赖。
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "a"}, provides: []string{" "}, requires: []string{""}},
	}
	got, err := SortByDependencies(list)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, names(got))
}

func TestInitializeAll_SortsBeforeInitializing(t *testing.T) {
	var initOrder []string
	list := []Plugin{
		&depPlugin{
			basePlugin: basePlugin{name: "consumer"},
			requires:   []string{"crypto"},
			initOrder:  &initOrder,
		},
		&depPlugin{
			basePlugin: basePlugin{name: "provider"},
			provides:   []string{"crypto"},
			initOrder:  &initOrder,
		},
	}

	require.NoError(t, InitializeAll(context.Background(), list))
	assert.Equal(t, []string{"provider", "consumer"}, initOrder)
}

func TestInitializeAll_DependencyErrorSkipsEverything(t *testing.T) {
	var initOrder []string
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "ok"}, initOrder: &initOrder},
		&depPlugin{basePlugin: basePlugin{name: "broken"}, requires: []string{"nope"}, initOrder: &initOrder},
	}

	err := InitializeAll(context.Background(), list)
	require.ErrorIs(t, err, ErrMissingDependency)
	// 装配错误：一个都不初始化，避免留下半初始化状态。
	assert.Empty(t, initOrder)
}

func TestDisposePlugins_FollowsReverseOfActualInitOrder(t *testing.T) {
	var initOrder, disposeOrder []string
	list := []Plugin{
		&depPlugin{
			basePlugin:   basePlugin{name: "consumer"},
			requires:     []string{"crypto"},
			initOrder:    &initOrder,
			disposeOrder: &disposeOrder,
		},
		&depPlugin{
			basePlugin:   basePlugin{name: "provider"},
			provides:     []string{"crypto"},
			initOrder:    &initOrder,
			disposeOrder: &disposeOrder,
		},
	}
	installTestPlugins(t, list)

	require.NoError(t, InitializePlugins(context.Background()))
	require.Equal(t, []string{"provider", "consumer"}, initOrder)

	DisposePlugins()
	// 消费方必须先于提供方被回收。
	assert.Equal(t, []string{"consumer", "provider"}, disposeOrder)
}

func TestInitializeAll_StillToleratesIndividualFailure(t *testing.T) {
	// 依赖解析通过后，单个插件 Initialize 失败仍然只影响它自己。
	boom := errors.New("boom")
	var initOrder []string
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "a"}, initOrder: &initOrder},
		&depPlugin{basePlugin: basePlugin{name: "b", initErr: boom}, initOrder: &initOrder},
		&depPlugin{basePlugin: basePlugin{name: "c"}, initOrder: &initOrder},
	}
	err := InitializeAll(context.Background(), list)
	require.ErrorIs(t, err, boom)
	assert.Equal(t, []string{"a", "b", "c"}, initOrder)
	assert.Equal(t, StateActive, mustState(t, "a"))
	assert.Equal(t, StateFailed, mustState(t, "b"))
	assert.Equal(t, StateActive, mustState(t, "c"))
}
