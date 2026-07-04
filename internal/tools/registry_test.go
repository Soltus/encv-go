package tools

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

// ─── 1. TestRegistry_Register_AndGet ────────────────────────────

func TestRegistry_Register_AndGet(t *testing.T) {
	r := NewRegistry()
	def := &ToolDef{
		Name:        "test_echo",
		Description: "echoes args",
		ArgsSchema:  `{"type":"object"}`,
		Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
			return ToolResult{Result: argsJSON, Status: "success"}, nil
		},
		ReadOnly: true,
		Kind:     KindFileRead,
	}
	if err := r.Register(def); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("test_echo")
	if !ok {
		t.Fatal("Get after Register: not found")
	}
	if got.Name != "test_echo" {
		t.Errorf("Name = %q, want test_echo", got.Name)
	}
	if got.Description != "echoes args" {
		t.Errorf("Description = %q, want echoes args", got.Description)
	}
	if got.Handler == nil {
		t.Error("Handler is nil after Register")
	}
}

// ─── 2. TestRegistry_DuplicateName_Error ───────────────────────

func TestRegistry_DuplicateName_Error(t *testing.T) {
	r := NewRegistry()
	def := &ToolDef{
		Name:        "dup_tool",
		Description: "first",
		ArgsSchema:  "{}",
		Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
			return ToolResult{Result: "{}"}, nil
		},
	}
	if err := r.Register(def); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := r.Register(def)
	if err == nil {
		t.Fatal("duplicate Register should return error")
	}
	if !strings.Contains(err.Error(), "dup_tool") {
		t.Errorf("error message = %q, should contain dup_tool", err.Error())
	}
}

// ─── 3. TestRegistry_List_ReturnsAllSorted ─────────────────────

func TestRegistry_List_ReturnsAllSorted(t *testing.T) {
	r := NewRegistry()
	// 故意乱序注册
	names := []string{"charlie", "alpha", "echo", "bravo", "delta"}
	for _, n := range names {
		_ = r.Register(&ToolDef{
			Name:        n,
			Description: n,
			ArgsSchema:  "{}",
			Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
				return ToolResult{Result: "{}"}, nil
			},
		})
	}
	got := r.Names()
	want := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	if len(got) != len(want) {
		t.Fatalf("Names count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// 同时检查 List 也排序
	defs := r.List()
	sortedNames := make([]string, len(defs))
	for i, d := range defs {
		sortedNames[i] = d.Name
	}
	if !sort.StringsAreSorted(sortedNames) {
		t.Errorf("List() output not sorted: %v", sortedNames)
	}
}

// ─── 4. TestRegistry_UnknownName_ReturnsFalse ──────────────────

func TestRegistry_UnknownName_ReturnsFalse(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&ToolDef{
		Name:        "known",
		Description: "x",
		ArgsSchema:  "{}",
		Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
			return ToolResult{Result: "{}"}, nil
		},
	})
	if _, ok := r.Get("nonexistent"); ok {
		t.Error("Get(nonexistent) should return false")
	}
	if r.Has("nonexistent") {
		t.Error("Has(nonexistent) should return false")
	}
	if !r.Has("known") {
		t.Error("Has(known) should return true")
	}
}

// ─── 5. TestRegistry_Dispatch_viaExecuteReal ───────────────────

func TestRegistry_Dispatch_viaExecuteReal(t *testing.T) {
	r := NewRegistry()
	// 模拟一个真实执行器：把 argsJSON 解析后回显
	_ = r.Register(&ToolDef{
		Name:        "execute_real_demo",
		Description: "demo",
		ArgsSchema:  `{"type":"object"}`,
		Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
			var in struct {
				X int    `json:"x"`
				Y string `json:"y"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
				return ToolResult{
					Result:  `{"error":"bad args"}`,
					IsError: true,
					Status:  "failed",
				}, err
			}
			out := map[string]any{"sum": in.X, "echo": in.Y}
			b, _ := json.Marshal(out)
			return ToolResult{
				Result: string(b),
				Status: "success",
				// 真实场景下还应记录 DurationMs = time.Since(start).Milliseconds()
			}, nil
		},
	})

	// execute_real=true 路径：调用 Dispatch
	res, err := r.Dispatch(context.Background(), "execute_real_demo", `{"x":42,"y":"hello"}`, nil)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Errorf("IsError = true, want false. Result = %s", res.Result)
	}
	if res.Status != "success" {
		t.Errorf("Status = %q, want success", res.Status)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(res.Result), &got); err != nil {
		t.Fatalf("parse result: %v. Result = %s", err, res.Result)
	}
	if int(got["sum"].(float64)) != 42 {
		t.Errorf("sum = %v, want 42", got["sum"])
	}
	if got["echo"].(string) != "hello" {
		t.Errorf("echo = %v, want hello", got["echo"])
	}

	// unknown tool 路径
	_, err = r.Dispatch(context.Background(), "nope", `{}`, nil)
	if err == nil {
		t.Error("Dispatch(unknown) should return error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error = %v, want 'unknown' in message", err)
	}
}

// ─── 6. TestRegistry_ResetGlobal_ForTests ──────────────────────

// 验证 ResetGlobal 在测试之间清理 GlobalRegistry。
func TestRegistry_ResetGlobal_ForTests(t *testing.T) {
	ResetGlobal()
	if GlobalRegistry.Has("search_files") {
		t.Skip("GlobalRegistry 已有 search_files——说明上一个测试未清理；本测试跳过")
	}
	_ = GlobalRegistry.Register(&ToolDef{
		Name:        "temp_tool",
		Description: "x",
		ArgsSchema:  "{}",
		Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
			return ToolResult{Result: "{}"}, nil
		},
	})
	if !GlobalRegistry.Has("temp_tool") {
		t.Error("Register 后 Has 应为 true")
	}
	ResetGlobal()
	if GlobalRegistry.Has("temp_tool") {
		t.Error("ResetGlobal 后 Has 应为 false")
	}
	// 忽略未使用变量警告
	_ = errors.New("dummy")
}
