package plugins

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectEnabled_EmptyMeansAll(t *testing.T) {
	list := []Plugin{
		&basePlugin{name: "a"},
		&basePlugin{name: "b"},
	}
	got, err := SelectEnabled(list, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, names(got))

	got, err = SelectEnabled(list, []string{})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, names(got))
}

func TestSelectEnabled_FollowsConfigOrder(t *testing.T) {
	list := []Plugin{
		&basePlugin{name: "a"},
		&basePlugin{name: "b"},
		&basePlugin{name: "c"},
	}
	got, err := SelectEnabled(list, []string{"c", "a"})
	require.NoError(t, err)
	assert.Equal(t, []string{"c", "a"}, names(got), "未列出的 b 应被裁剪掉")
}

func TestSelectEnabled_UnknownNameIsAnError(t *testing.T) {
	list := []Plugin{&basePlugin{name: "a"}}
	_, err := SelectEnabled(list, []string{"a", "typo"})
	require.ErrorIs(t, err, ErrUnknownPlugin)
	// 报错时带上可用插件名，省得再去翻源码
	assert.Contains(t, err.Error(), "typo")
	assert.Contains(t, err.Error(), "a")
}

func TestSelectEnabled_DuplicateKeepsFirstPosition(t *testing.T) {
	list := []Plugin{
		&basePlugin{name: "a"},
		&basePlugin{name: "b"},
	}
	got, err := SelectEnabled(list, []string{"b", "a", "b"})
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "a"}, names(got))
}

func TestInitializePluginsWith_OnlyEnabledAreInitialized(t *testing.T) {
	var initOrder, disposeOrder []string
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "a"}, initOrder: &initOrder, disposeOrder: &disposeOrder},
		&depPlugin{basePlugin: basePlugin{name: "b"}, initOrder: &initOrder, disposeOrder: &disposeOrder},
		&depPlugin{basePlugin: basePlugin{name: "c"}, initOrder: &initOrder, disposeOrder: &disposeOrder},
	}
	installTestPlugins(t, list)

	require.NoError(t, InitializePluginsWith(context.Background(), []string{"c", "a"}))
	assert.Equal(t, []string{"c", "a"}, initOrder)
	assert.Equal(t, StateActive, mustState(t, "a"))
	assert.Equal(t, StateActive, mustState(t, "c"))
	assert.Equal(t, StatePending, mustState(t, "b"), "未启用的插件不应被初始化")

	// 卸载只碰装载过的插件：没初始化过的没有副作用可收。
	DisposePlugins()
	assert.Equal(t, []string{"a", "c"}, disposeOrder)
}

func TestInitializePluginsWith_ConfigOrderYieldsToDependencies(t *testing.T) {
	// 配置说 consumer 先，但依赖要求 provider 先——约束赢。
	var initOrder []string
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "provider"}, provides: []string{"x"}, initOrder: &initOrder},
		&depPlugin{basePlugin: basePlugin{name: "consumer"}, requires: []string{"x"}, initOrder: &initOrder},
	}
	installTestPlugins(t, list)

	require.NoError(t, InitializePluginsWith(context.Background(), []string{"consumer", "provider"}))
	assert.Equal(t, []string{"provider", "consumer"}, initOrder)
}

func TestInitializePluginsWith_UnknownNameFailsFast(t *testing.T) {
	var initOrder []string
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "a"}, initOrder: &initOrder},
	}
	installTestPlugins(t, list)

	err := InitializePluginsWith(context.Background(), []string{"a", "nope"})
	require.ErrorIs(t, err, ErrUnknownPlugin)
	assert.Empty(t, initOrder, "装配错误时一个插件都不该被初始化")
}

func TestInitializePlugins_StillInitializesEverything(t *testing.T) {
	// 兼容底线：不配置装配时行为与改造前一致。
	var initOrder []string
	list := []Plugin{
		&depPlugin{basePlugin: basePlugin{name: "a"}, initOrder: &initOrder},
		&depPlugin{basePlugin: basePlugin{name: "b"}, initOrder: &initOrder},
	}
	installTestPlugins(t, list)

	require.NoError(t, InitializePlugins(context.Background()))
	assert.Equal(t, []string{"a", "b"}, initOrder)
}
