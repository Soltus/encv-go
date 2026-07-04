package tools

import (
	"reflect"
	"strings"
	"testing"
)

// ─── NormalizeArgs 测试 ──────────────────────────────────────

func TestNormalizeArgs_NoAliasesForTool(t *testing.T) {
	args := map[string]string{"path": "/a.txt"}
	got := NormalizeArgs("nonexistent_tool", args, DefaultParamAliases)
	if !reflect.DeepEqual(got, args) {
		t.Errorf("no aliases for tool should return unchanged, got %v", got)
	}
}

func TestNormalizeArgs_ReplacesAlias(t *testing.T) {
	args := map[string]string{"filePath": "/a.txt"}
	got := NormalizeArgs("read_file", args, DefaultParamAliases)
	if got["path"] != "/a.txt" {
		t.Errorf("filePath should be replaced with path, got %v", got)
	}
	if _, exists := got["filePath"]; exists {
		t.Error("filePath key should be removed after replacement")
	}
}

func TestNormalizeArgs_PreservesExplicitValue(t *testing.T) {
	// 显式设置的主参数优先于别名
	args := map[string]string{
		"path":     "/explicit.txt",
		"filePath": "/alias.txt",
	}
	got := NormalizeArgs("read_file", args, DefaultParamAliases)
	if got["path"] != "/explicit.txt" {
		t.Errorf("explicit value should win over alias, got %v", got["path"])
	}
}

func TestNormalizeArgs_EmptyInput(t *testing.T) {
	got := NormalizeArgs("read_file", map[string]string{}, DefaultParamAliases)
	if got == nil {
		t.Error("empty input should return empty map, not nil")
	}
}

func TestNormalizeArgs_FilenameAlias(t *testing.T) {
	args := map[string]string{"filename": "/b.txt"}
	got := NormalizeArgs("read_file", args, DefaultParamAliases)
	if got["path"] != "/b.txt" {
		t.Errorf("filename should be replaced with path, got %v", got)
	}
}

func TestNormalizeArgs_WebFetchLinkToUrl(t *testing.T) {
	args := map[string]string{"link": "https://example.com"}
	got := NormalizeArgs("web_fetch", args, DefaultParamAliases)
	if got["url"] != "https://example.com" {
		t.Errorf("link should be replaced with url, got %v", got)
	}
}

// ─── BuildParamHint 测试 ─────────────────────────────────────

func TestBuildParamHint_SingleMissing(t *testing.T) {
	required := []RequiredParam{{Name: "path", Type: "string"}}
	provided := map[string]string{}
	got := BuildParamHint("read_file", required, provided)
	if !strings.Contains(got, "缺少必填参数 path (string)") {
		t.Errorf("hint should mention missing param, got: %s", got)
	}
	if !strings.Contains(got, `read_file(path="<value>")`) {
		t.Errorf("hint should include example call, got: %s", got)
	}
}

func TestBuildParamHint_AllProvided_NoMissing(t *testing.T) {
	required := []RequiredParam{{Name: "path", Type: "string"}}
	provided := map[string]string{"path": "/a.txt"}
	got := BuildParamHint("read_file", required, provided)
	// 所有参数都提供 → 应走"参数不合法"分支（因为没缺失）
	if !strings.Contains(got, "read_file") {
		t.Errorf("hint should mention tool name, got: %s", got)
	}
}

func TestBuildParamHint_MultipleTypes(t *testing.T) {
	required := []RequiredParam{
		{Name: "name", Type: "string"},
		{Name: "max_iter", Type: "int"},
		{Name: "verbose", Type: "bool"},
	}
	provided := map[string]string{}
	got := BuildParamHint("agent_run", required, provided)
	if !strings.Contains(got, "name (string)") {
		t.Error("should include name (string)")
	}
	if !strings.Contains(got, "max_iter (int)") {
		t.Error("should include max_iter (int)")
	}
	if !strings.Contains(got, "verbose (bool)") {
		t.Error("should include verbose (bool)")
	}
	if !strings.Contains(got, `name="<value>"`) {
		t.Error("string placeholder should be quoted")
	}
}

// ─── ParseArgsJSON 容错测试 ──────────────────────────────────

func TestParseArgsJSON_Empty(t *testing.T) {
	if got := ParseArgsJSON(""); got == nil {
		t.Error("empty should return empty map, not nil")
	}
}

func TestParseArgsJSON_Valid(t *testing.T) {
	got := ParseArgsJSON(`{"path":"/a.txt","max":10}`)
	if got["path"] != "/a.txt" {
		t.Errorf("path should be /a.txt, got %q", got["path"])
	}
	if got["max"] != "10" {
		t.Errorf("max should be string 10, got %q", got["max"])
	}
}

func TestParseArgsJSON_InvalidJSON_ReturnsEmptyMap(t *testing.T) {
	// nuclear-boy L776-798 容错：JSON 解析失败 → fallback emptyMap（不抛错）
	got := ParseArgsJSON(`{not valid json`)
	if got == nil {
		t.Error("should return empty map (not nil) on invalid JSON")
	}
	if len(got) != 0 {
		t.Errorf("invalid JSON should give empty map, got %v", got)
	}
}

// ─── SortToolsByPriority 测试 ────────────────────────────────

func TestSortToolsByPriority_PriorityToolsFirst(t *testing.T) {
	tools := []string{"search_files", "read_file", "web_search", "write_file"}
	weights := PriorityToolWeights{
		PriorityTools: map[string]bool{"read_file": true, "write_file": true},
		ConfirmTools:  map[string]bool{"search_files": true},
	}
	got := SortToolsByPriority(tools, weights)

	// read_file / write_file 应该是前 2
	if got[0] != "read_file" || got[1] != "write_file" {
		t.Errorf("priority tools should be first, got order: %v", got)
	}
	// search_files 应该最后
	if got[len(got)-1] != "search_files" {
		t.Errorf("confirm tool should be last, got order: %v", got)
	}
}

func TestSortToolsByPriority_AlphabeticalFallback(t *testing.T) {
	tools := []string{"zeta", "alpha", "mu"}
	weights := PriorityToolWeights{}
	got := SortToolsByPriority(tools, weights)
	if got[0] != "alpha" || got[1] != "mu" || got[2] != "zeta" {
		t.Errorf("same-weight should sort alphabetically, got: %v", got)
	}
}

func TestSortToolsByPriority_StableOrder(t *testing.T) {
	// 验证 sort.SliceStable 行为：相同权重保持原顺序
	tools := []string{"a", "b", "c"}
	weights := PriorityToolWeights{}
	got1 := SortToolsByPriority(tools, weights)
	got2 := SortToolsByPriority(tools, weights)
	if !reflect.DeepEqual(got1, got2) {
		t.Errorf("sort should be deterministic, got %v vs %v", got1, got2)
	}
}
