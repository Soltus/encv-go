package plugins_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initPluginsForTaskOptions(t *testing.T) {
	t.Helper()
	cfg := &config.Config{
		Password: "global-test-pw",
		PluginSettings: map[string]json.RawMessage{
			"video":         json.RawMessage(`{"ext": ".sccgv"}`),
		"alist_encrypt": json.RawMessage(`{"ext": ".bin", "enc_type": "aesctr"}`),
		"text":          json.RawMessage(`{"ext": ".sccgt"}`),
		"audio":         json.RawMessage(`{"ext": ".sccga"}`),
		"image":         json.RawMessage(`{"ext": ".sccgi"}`),
		"pdf":           json.RawMessage(`{"ext": ".sccgpdf"}`),
		"wps":           json.RawMessage(`{"ext": ".sccgwps"}`),
		},
	}
	ctx := config.NewContext(context.Background(), cfg)
	err := plugins.InitializePlugins(ctx)
	require.NoError(t, err, "InitializePlugins should succeed")
}

func getPluginByName(name string) plugins.Plugin {
	for _, p := range plugins.Plugins {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

func TestVideoPlugin_GetTaskOptions(t *testing.T) {
	initPluginsForTaskOptions(t)
	p := getPluginByName("video")
	require.NotNil(t, p, "video plugin should exist")
	opts := p.GetTaskOptions()
	assert.Equal(t, pluginInterfaces.PasswordGlobal, opts.PasswordStrategy, "video should use global password")
	assert.True(t, opts.SupportVersionSelect, "video should support version select")
	assert.NotEmpty(t, opts.SupportedVersions, "video should have supported versions")
	require.Len(t, opts.ExtraFields, 6, "video should have 6 extra fields")
	assert.Equal(t, "stream_preset", opts.ExtraFields[0].Key)
	assert.Equal(t, "select", opts.ExtraFields[0].Type)
	assert.False(t, opts.ExtraFields[0].Required)
	assert.Equal(t, "balanced", opts.ExtraFields[0].DefaultValue)
	assert.Contains(t, opts.ExtraFields[0].Options, "balanced")
	assert.Contains(t, opts.ExtraFields[0].Options, "high_quality")
	assert.Equal(t, "Balanced", opts.ExtraFields[0].OptionLabels["balanced"])
	assert.Equal(t, "High Quality", opts.ExtraFields[0].OptionLabels["high_quality"])
	assert.Equal(t, "encrypt", opts.ExtraFields[0].Condition)

	assert.Equal(t, "encrypt_filename", opts.ExtraFields[1].Key)
	assert.Equal(t, "bool", opts.ExtraFields[1].Type)
	assert.Equal(t, "false", opts.ExtraFields[1].DefaultValue)
	assert.Equal(t, "encrypt", opts.ExtraFields[1].Condition)

	assert.Equal(t, "fn_rounds", opts.ExtraFields[2].Key)
	assert.Equal(t, "select", opts.ExtraFields[2].Type)
	assert.Equal(t, "8", opts.ExtraFields[2].DefaultValue)
	assert.Equal(t, []string{"4", "8", "12", "16"}, opts.ExtraFields[2].Options)
	assert.Equal(t, "8 (Recommended)", opts.ExtraFields[2].OptionLabels["8"])
	assert.Equal(t, "encrypt", opts.ExtraFields[2].Condition)

	assert.Equal(t, "fn_charset", opts.ExtraFields[3].Key)
	assert.Equal(t, "select", opts.ExtraFields[3].Type)
	assert.Equal(t, "alnum", opts.ExtraFields[3].DefaultValue)
	assert.Contains(t, opts.ExtraFields[3].Options, "alnum")
	assert.Contains(t, opts.ExtraFields[3].Options, "full")
	assert.Equal(t, "Alphanumeric", opts.ExtraFields[3].OptionLabels["alnum"])
	assert.Equal(t, "Full (Alnum+Symbols+Hanzi+Emoji)", opts.ExtraFields[3].OptionLabels["full"])
	assert.Equal(t, "encrypt", opts.ExtraFields[3].Condition)

	assert.Equal(t, "fn_deconfuse", opts.ExtraFields[4].Key)
	assert.Equal(t, "bool", opts.ExtraFields[4].Type)
	assert.Equal(t, "false", opts.ExtraFields[4].DefaultValue)
	assert.Equal(t, "encrypt", opts.ExtraFields[4].Condition)

	assert.Equal(t, "fn_structured", opts.ExtraFields[5].Key)
	assert.Equal(t, "bool", opts.ExtraFields[5].Type)
	assert.Equal(t, "false", opts.ExtraFields[5].DefaultValue)
	assert.Equal(t, "encrypt", opts.ExtraFields[5].Condition)
}

func TestAlistEncryptPlugin_GetTaskOptions(t *testing.T) {
	initPluginsForTaskOptions(t)
	p := getPluginByName("alist_encrypt")
	require.NotNil(t, p, "alist_encrypt plugin should exist")
	opts := p.GetTaskOptions()
	assert.Equal(t, pluginInterfaces.PasswordIndependent, opts.PasswordStrategy, "alist_encrypt should use independent password")
	assert.False(t, opts.SupportVersionSelect, "alist_encrypt should NOT support version select")
	require.Len(t, opts.ExtraFields, 3, "alist_encrypt should have 3 extra fields")
	assert.Equal(t, "plugin_password", opts.ExtraFields[0].Key)
	assert.Equal(t, "password", opts.ExtraFields[0].Type)
	assert.False(t, opts.ExtraFields[0].Required, "plugin_password should not be required")
	assert.Equal(t, "encode_filename", opts.ExtraFields[1].Key)
	assert.Equal(t, "bool", opts.ExtraFields[1].Type)
	assert.Equal(t, "false", opts.ExtraFields[1].DefaultValue)
	assert.Equal(t, "encrypt", opts.ExtraFields[1].Condition)
	assert.Equal(t, "enc_type", opts.ExtraFields[2].Key)
	assert.Equal(t, "select", opts.ExtraFields[2].Type)
	assert.Equal(t, "aesctr", opts.ExtraFields[2].DefaultValue)
	assert.Equal(t, []string{"aesctr"}, opts.ExtraFields[2].Options)
	assert.Equal(t, "AES-CTR-128", opts.ExtraFields[2].OptionLabels["aesctr"])
	assert.Equal(t, "encrypt", opts.ExtraFields[2].Condition)
}

func TestOtherPlugins_DefaultToGlobal(t *testing.T) {
	initPluginsForTaskOptions(t)
	for _, name := range []string{"text", "audio", "image", "pdf", "wps"} {
		p := getPluginByName(name)
		require.NotNil(t, p, "%s plugin should exist", name)
		assert.Equal(t, pluginInterfaces.PasswordGlobal, p.GetTaskOptions().PasswordStrategy,
			"plugin %s should default to global password strategy", name)
		assert.False(t, p.GetTaskOptions().SupportVersionSelect,
			"plugin %s should NOT support version select", name)
		extraFields := p.GetTaskOptions().ExtraFields
		require.Len(t, extraFields, 5, "%s should have 5 extra fields (FNConfig filename encryption)", name)
		assert.Equal(t, "encrypt_filename", extraFields[0].Key, "%s[0]", name)
		assert.Equal(t, "fn_rounds", extraFields[1].Key, "%s[1]", name)
		assert.Equal(t, "fn_charset", extraFields[2].Key, "%s[2]", name)
		assert.Equal(t, "fn_deconfuse", extraFields[3].Key, "%s[3]", name)
		assert.Equal(t, "fn_structured", extraFields[4].Key, "%s[4]", name)
		for i, f := range extraFields {
			assert.Equal(t, "encrypt", f.Condition, "%s[%d] condition should be 'encrypt'", name, i)
		}
	}
}
