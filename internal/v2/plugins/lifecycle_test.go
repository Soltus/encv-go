package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/namer"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// basePlugin 是 Plugin 接口的零值实现，用于生命周期测试。
// 它刻意**不**实现 Disposable，以便测试「未实现可选接口时不报错」这条路径。
type basePlugin struct {
	name    string
	initErr error
}

func (p *basePlugin) Name() string { return p.name }
func (p *basePlugin) GetDefaultSettings() json.RawMessage {
	return json.RawMessage(`{}`)
}
func (p *basePlugin) GetSettingsSchemaType() interface{}                       { return struct{}{} }
func (p *basePlugin) GetContainerExtension() string                            { return ".stub" }
func (p *basePlugin) GetSettingFields() []pluginInterfaces.SettingField        { return nil }
func (p *basePlugin) Initialize(context.Context) error                         { return p.initErr }
func (p *basePlugin) GetMetadataExtractor() pluginInterfaces.MetadataExtractor { return nil }
func (p *basePlugin) GetContentPreprocessor() pluginInterfaces.ContentPreprocessor {
	return nil
}
func (p *basePlugin) GetContentVirifier() pluginInterfaces.ContentVerifier          { return nil }
func (p *basePlugin) GetChunkNamer() namer.ChunkNamer                               { return nil }
func (p *basePlugin) SupportedMimePrefixes() []string                               { return nil }
func (p *basePlugin) SupportedExtensions() []string                                 { return nil }
func (p *basePlugin) ShouldProcess(string) bool                                     { return true }
func (p *basePlugin) GroupFiles(in []string, _, _ string) ([]string, error)         { return in, nil }
func (p *basePlugin) PreEncryptProcessor(types.Index, string, string, string) error { return nil }
func (p *basePlugin) Encrypt(io.Reader) (*crypto.EncryptionResult, error) {
	return nil, nil
}
func (p *basePlugin) PostEncryptProcessor(*crypto.EncryptionResult) (string, error) {
	return "", nil
}
func (p *basePlugin) CanDecrypt(string) bool                    { return false }
func (p *basePlugin) PreDecryptProcessor(string, string) error  { return nil }
func (p *basePlugin) Decrypt(string, string) (string, error)    { return "", nil }
func (p *basePlugin) PostDecryptProcessor(string) error         { return nil }
func (p *basePlugin) ContainerType() uint16                     { return 0 }
func (p *basePlugin) DefaultIsSeekable(string) bool             { return false }
func (p *basePlugin) DisasterZones(string) []types.DisasterZone { return nil }
func (p *basePlugin) SupportedContainerVersions() []int         { return types.SupportedVersions }
func (p *basePlugin) DefaultContainerVersion() int              { return types.DefaultContainerVersion }
func (p *basePlugin) ValidateVersion(int) error                 { return nil }
func (p *basePlugin) GetTaskOptions() pluginInterfaces.TaskOptions {
	return pluginInterfaces.TaskOptions{}
}

// disposablePlugin 在 basePlugin 之上实现 Disposable，并记录被卸载的顺序。
type disposablePlugin struct {
	basePlugin
	order      *[]string
	disposeErr error
}

func (p *disposablePlugin) Dispose() error {
	if p.order != nil {
		*p.order = append(*p.order, p.name)
	}
	return p.disposeErr
}

// installTestPlugins 临时替换全局插件列表，测试结束自动还原。
// 生命周期函数都接受显式列表，因此这里主要是为了验证全局入口的行为一致性。
func installTestPlugins(t *testing.T, list []Plugin) {
	t.Helper()
	old := Plugins
	Plugins = list
	ResetPluginStates()
	t.Cleanup(func() {
		Plugins = old
		ResetPluginStates()
	})
}

func TestInitializeAll_AllSucceed_MarksActive(t *testing.T) {
	var order []string
	list := []Plugin{
		&disposablePlugin{basePlugin: basePlugin{name: "a"}, order: &order},
		&disposablePlugin{basePlugin: basePlugin{name: "b"}, order: &order},
	}

	require.NoError(t, InitializeAll(context.Background(), list))

	assert.Equal(t, StateActive, mustState(t, "a"))
	assert.Equal(t, StateActive, mustState(t, "b"))
	assert.Empty(t, order, "初始化成功不应触发 Dispose")
}

// TestInitializeAll_FailureIsIsolated 锁住「一个插件失败不拖垮其它插件」：
// 失败的插件停在 StateFailed，成功的仍然是 StateActive，错误被聚合返回。
func TestInitializeAll_FailureIsIsolated(t *testing.T) {
	var order []string
	boom := errors.New("init boom")
	list := []Plugin{
		&disposablePlugin{basePlugin: basePlugin{name: "a"}, order: &order},
		&basePlugin{name: "b", initErr: boom},
		&disposablePlugin{basePlugin: basePlugin{name: "c"}, order: &order},
	}

	err := InitializeAll(context.Background(), list)
	require.ErrorIs(t, err, boom)

	assert.Equal(t, StateActive, mustState(t, "a"), "b 失败不应影响 a")
	assert.Equal(t, StateFailed, mustState(t, "b"))
	assert.Equal(t, StateActive, mustState(t, "c"), "失败之后的插件仍应被尝试初始化")
	assert.Empty(t, order, "初始化失败不应触发 Dispose（失败的插件本来就没装载成功）")

	st, _ := GetPluginState("b")
	assert.Contains(t, st.Error, "init boom")
}

func TestDisposeAll_ReverseOrder(t *testing.T) {
	var order []string
	list := []Plugin{
		&disposablePlugin{basePlugin: basePlugin{name: "a"}, order: &order},
		&disposablePlugin{basePlugin: basePlugin{name: "b"}, order: &order},
		&disposablePlugin{basePlugin: basePlugin{name: "c"}, order: &order},
	}
	require.NoError(t, InitializeAll(context.Background(), list))

	statuses := DisposeAll(list)
	require.Len(t, statuses, 3)
	assert.Equal(t, []string{"c", "b", "a"}, order)
	for _, s := range statuses {
		assert.Equal(t, StateDisposed, s.State)
		assert.Empty(t, s.Error)
	}
}

func TestDisposeAll_SkipsNonDisposable(t *testing.T) {
	list := []Plugin{&basePlugin{name: "plain"}}
	require.NoError(t, InitializeAll(context.Background(), list))

	statuses := DisposeAll(list)
	require.Len(t, statuses, 1)
	assert.Equal(t, StateDisposed, statuses[0].State, "未实现 Disposable 的插件也应标记为已卸载")
	assert.Empty(t, statuses[0].Error, "缺少 Dispose 不是错误")
}

func TestDisposeAll_CollectsDisposeErrorButContinues(t *testing.T) {
	var order []string
	boom := errors.New("dispose boom")
	list := []Plugin{
		&disposablePlugin{basePlugin: basePlugin{name: "a"}, order: &order, disposeErr: boom},
		&disposablePlugin{basePlugin: basePlugin{name: "b"}, order: &order},
	}

	statuses := DisposeAll(list)
	require.Len(t, statuses, 2)
	assert.Equal(t, []string{"b", "a"}, order, "某个插件 Dispose 失败不应中断其余插件的卸载")
	assert.Equal(t, StateFailed, statuses[0].State, "a 的 Dispose 失败应记录为 failed")
	assert.Contains(t, statuses[0].Error, "dispose boom")
}

func TestDisposePlugins_UsesGlobalRegistry(t *testing.T) {
	var order []string
	installTestPlugins(t, []Plugin{
		&disposablePlugin{basePlugin: basePlugin{name: "a"}, order: &order},
		&disposablePlugin{basePlugin: basePlugin{name: "b"}, order: &order},
	})
	require.NoError(t, InitializePlugins(context.Background()))

	DisposePlugins()
	assert.Equal(t, []string{"b", "a"}, order)
}

func TestGetPluginStates_OrderedAndIsolatedCopy(t *testing.T) {
	installTestPlugins(t, []Plugin{&basePlugin{name: "a"}, &basePlugin{name: "b"}})
	require.NoError(t, InitializePlugins(context.Background()))

	states := GetPluginStates()
	require.Len(t, states, 2)
	assert.Equal(t, "a", states[0].Name)
	assert.Equal(t, "b", states[1].Name)

	states[0].State = StateFailed
	assert.Equal(t, StateActive, mustState(t, "a"), "GetPluginStates 必须返回副本，避免调用方篡改内部状态")
}

func TestSetPluginState_RecordsError(t *testing.T) {
	installTestPlugins(t, nil)
	SetPluginState("ghost", StateFailed, errors.New("boom"))

	st, ok := GetPluginState("ghost")
	require.True(t, ok)
	assert.Equal(t, StateFailed, st.State)
	assert.Equal(t, "boom", st.Error)
}

func mustState(t *testing.T, name string) PluginState {
	t.Helper()
	st, ok := GetPluginState(name)
	require.True(t, ok, "插件 %s 应有状态记录", name)
	return st.State
}
