package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Soltus/encv-go/internal/v2/plugins"
)

func main() {
	type PluginMeta struct {
		Name                  string        `json:"name"`
		SupportedExtensions   []interface{} `json:"supportedExtensions"`
		SupportedMimePrefixes []interface{} `json:"supportedMimePrefixes"`
		ContainerExtension    string        `json:"containerExtension"`
	}
	var metas []PluginMeta
	for _, p := range plugins.Plugins {
		exts := p.SupportedExtensions()
		if exts == nil {
			exts = []string{}
		}
		mimes := p.SupportedMimePrefixes()
		if mimes == nil {
			mimes = []string{}
		}
		metas = append(metas, PluginMeta{
			Name:                  p.Name(),
			SupportedExtensions:   toIfaceSlice(exts),
			SupportedMimePrefixes: toIfaceSlice(mimes),
			ContainerExtension:    p.GetContainerExtension(),
		})
	}
	out, _ := json.MarshalIndent(metas, "", "  ")
	fmt.Println(string(out))
	// 验证排序后的 plugin name 列表
	names := make([]string, 0)
	for _, p := range metas {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	fmt.Println("PLUGIN_COUNT:", len(names))
	fmt.Println("PLUGINS:", names)
}

func toIfaceSlice(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
