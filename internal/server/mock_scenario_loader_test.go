// internal/server/mock_scenario_loader_test.go
//
// T2 验收：loader 全场景通过。
//
// 覆盖：
//   - YAML / JSON 解析
//   - 多文件加载
//   - 错误聚合（单文件失败不影响其他）
//   - 重复 id 第一个赢
//   - 目录空 / 目录不存在 → Go fallback
//   - 优先级：YAML 覆盖 Go
//   - Hot reload
package server

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ════════════════════════════════════════════════════════════════
// 基础解析
// ════════════════════════════════════════════════════════════════

func TestLoader_LoadYAML_BasicFields(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "test.yaml", `
id: load_basic
description: basic test
steps:
  - id: s1
    events:
      - type: stream_start
        data: { scenario: load_basic }
      - type: stream_end
        data: { finishReason: stop }
`)

	l := NewScenarioLoader(dir)
	if err := l.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	scens := l.GetScenarios()
	if len(scens) != 1 {
		t.Fatalf("scenarios = %d, want 1", len(scens))
	}
	if scens[0].ID != "load_basic" {
		t.Errorf("id = %q, want load_basic", scens[0].ID)
	}
}

func TestLoader_LoadYAML_AllEventTypes(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "all_events.yaml", `
id: all_events
steps:
  - id: s1
    events:
      - type: stream_start
        data: { scenario: all_events }
      - type: text_delta
        data: { text: "通用 UI 文案，不含硬编码数据" }
      - type: tool_call
        data:
          id: call_1
          name: search_files
          args: { ext: ".mp4" }
      - type: mock_branch_choice
        data:
          options:
            - id: a
              label: 选项 A
            - id: b
              label: 选项 B
      - type: stream_end
        data: { finishReason: stop }
`)

	l := NewScenarioLoader(dir)
	if err := l.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	scens := l.GetScenarios()
	if len(scens) != 1 {
		t.Fatalf("scenarios = %d, want 1", len(scens))
	}
	// tool_call 会被自动注入 tool_result → 转换后有 6 个 event
	// (stream_start + text_delta + tool_call + auto tool_result + mock_branch_choice + stream_end)
	step := scens[0].Steps[0]
	if len(step.Events) != 6 {
		t.Errorf("step events = %d, want 6 (5 declared + 1 auto tool_result)", len(step.Events))
	}
	// 验证 auto-injected tool_result 紧随 tool_call
	var foundPair bool
	for i := 0; i < len(step.Events)-1; i++ {
		if step.Events[i].Type == "tool_call" && step.Events[i+1].Type == "tool_result" {
			foundPair = true
			break
		}
	}
	if !foundPair {
		t.Error("tool_call not followed by auto-injected tool_result")
	}
}

func TestLoader_LoadYAML_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 3; i++ {
		writeYAML(t, dir, "s"+string(rune('0'+i))+".yaml", `
id: multi_`+string(rune('0'+i))+`
steps:
  - id: s1
    events:
      - type: stream_start
        data: { scenario: multi_`+string(rune('0'+i))+` }
      - type: stream_end
        data: { finishReason: stop }
`)
	}

	l := NewScenarioLoader(dir)
	if err := l.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	scens := l.GetScenarios()
	if len(scens) != 3 {
		t.Errorf("scenarios = %d, want 3", len(scens))
	}
}

func TestLoader_LoadJSON_EquivalentToYAML(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "json_test.json", `{
  "id": "json_test",
  "description": "json load test",
  "steps": [
    {
      "id": "s1",
      "events": [
        {"type": "stream_start", "data": {"scenario": "json_test"}},
        {"type": "stream_end", "data": {"finishReason": "stop"}}
      ]
    }
  ]
}`)

	l := NewScenarioLoader(dir)
	if err := l.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	scens := l.GetScenarios()
	if len(scens) != 1 {
		t.Fatalf("scenarios = %d, want 1", len(scens))
	}
	if scens[0].ID != "json_test" {
		t.Errorf("id = %q, want json_test", scens[0].ID)
	}
}

// ════════════════════════════════════════════════════════════════
// 错误处理
// ════════════════════════════════════════════════════════════════

func TestLoader_RejectMissingID(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "no_id.yaml", `
description: no id
steps:
  - id: s1
    events:
      - type: stream_end
        data: { finishReason: stop }
`)
	l := NewScenarioLoader(dir)
	// Go fallback nil → LoadAll 仍然返回 nil（不阻断），但 scenarios 为空
	_ = l.LoadAll(context.Background())
	scens := l.GetScenarios()
	if len(scens) != 0 {
		t.Errorf("scenarios = %d, want 0 (rejected file)", len(scens))
	}
}

func TestLoader_RejectDuplicateID(t *testing.T) {
	dir := t.TempDir()
	yaml := `
id: dup
steps:
  - id: s1
    events:
      - type: stream_start
        data: { scenario: dup }
      - type: stream_end
        data: { finishReason: stop }
`
	writeYAML(t, dir, "a.yaml", yaml)
	writeYAML(t, dir, "b.yaml", yaml)

	l := NewScenarioLoader(dir)
	if err := l.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	scens := l.GetScenarios()
	if len(scens) != 1 {
		t.Errorf("scenarios = %d, want 1 (duplicate rejected)", len(scens))
	}
}

func TestLoader_RejectEmptySteps(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "empty.yaml", `id: empty_steps`)
	l := NewScenarioLoader(dir)
	_ = l.LoadAll(context.Background())
	if got := len(l.GetScenarios()); got != 0 {
		t.Errorf("scenarios = %d, want 0", got)
	}
}

func TestLoader_RejectEmptyEvents(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "no_events.yaml", `
id: no_events
steps:
  - id: s1
`)
	l := NewScenarioLoader(dir)
	_ = l.LoadAll(context.Background())
	if got := len(l.GetScenarios()); got != 0 {
		t.Errorf("scenarios = %d, want 0", got)
	}
}

func TestLoader_RejectHardcodedData(t *testing.T) {
	dir := t.TempDir()
	// 硬编码路径
	writeYAML(t, dir, "bad1.yaml", `
id: hardcoded_path
steps:
  - id: s1
    events:
      - type: text_delta
        data: { text: "Found file: Movies/2024/big.mp4" }
`)
	// 硬编码文件大小
	writeYAML(t, dir, "bad2.yaml", `
id: hardcoded_size
steps:
  - id: s1
    events:
      - type: text_delta
        data: { text: "Size: 524MB" }
`)
	// 工具结果事件
	writeYAML(t, dir, "bad3.yaml", `
id: tool_result_event
steps:
  - id: s1
    events:
      - type: tool_result
        data: { id: "x", result: "{}" }
`)

	l := NewScenarioLoader(dir)
	_ = l.LoadAll(context.Background())
	if got := len(l.GetScenarios()); got != 0 {
		t.Errorf("scenarios = %d, want 0 (all 3 files should be rejected)", got)
	}
}

func TestLoader_ErrorAggregation_OneBadDoesNotBlockOthers(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "bad.yaml", `id: empty_steps`)
	writeYAML(t, dir, "good.yaml", `
id: good
steps:
  - id: s1
    events:
      - type: stream_end
        data: { finishReason: stop }
`)

	l := NewScenarioLoader(dir)
	_ = l.LoadAll(context.Background())
	scens := l.GetScenarios()
	if len(scens) != 1 {
		t.Fatalf("scenarios = %d, want 1 (good.yaml should still load)", len(scens))
	}
	if scens[0].ID != "good" {
		t.Errorf("loaded id = %q, want good", scens[0].ID)
	}
}

// ════════════════════════════════════════════════════════════════
// Fallback 行为
// ════════════════════════════════════════════════════════════════

func TestLoader_DirEmpty_UsesGoFallback(t *testing.T) {
	dir := t.TempDir() // 空目录

	l := NewScenarioLoader(dir)
	l.GoFallback = func() []*MockScenario {
		return []*MockScenario{
			{ID: "fallback_1", Description: "test"},
		}
	}
	if err := l.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	scens := l.GetScenarios()
	if len(scens) != 1 {
		t.Fatalf("scenarios = %d, want 1", len(scens))
	}
	if scens[0].ID != "fallback_1" {
		t.Errorf("id = %q, want fallback_1", scens[0].ID)
	}
}

func TestLoader_DirNotFound_UsesGoFallback(t *testing.T) {
	l := NewScenarioLoader("/nonexistent/path/that/should/not/exist")
	l.GoFallback = func() []*MockScenario {
		return []*MockScenario{{ID: "fallback_2"}}
	}
	if err := l.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	scens := l.GetScenarios()
	if len(scens) != 1 || scens[0].ID != "fallback_2" {
		t.Errorf("expected fallback_2, got %+v", scens)
	}
}

func TestLoader_Priority_YAMLOverridesGo(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "yaml_first.yaml", `
id: yaml_wins
steps:
  - id: s1
    events:
      - type: stream_end
        data: { finishReason: stop }
`)

	l := NewScenarioLoader(dir)
	l.GoFallback = func() []*MockScenario {
		// Go fallback 包含更多剧本
		return []*MockScenario{
			{ID: "yaml_wins"}, // 重复 id
			{ID: "go_extra_1"},
			{ID: "go_extra_2"},
		}
	}
	_ = l.LoadAll(context.Background())
	scens := l.GetScenarios()
	if len(scens) != 1 {
		t.Fatalf("scenarios = %d, want 1 (YAML should fully override Go)", len(scens))
	}
	if scens[0].ID != "yaml_wins" {
		t.Errorf("id = %q, want yaml_wins", scens[0].ID)
	}
}

// ════════════════════════════════════════════════════════════════
// 热重载
// ════════════════════════════════════════════════════════════════

func TestLoader_HotReload_FileChange(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "watch.yaml", `
id: watch_v1
steps:
  - id: s1
    events:
      - type: stream_end
        data: { finishReason: stop }
`)

	l := NewScenarioLoader(dir)
	_ = l.LoadAll(context.Background())
	if got := l.GetScenarios(); len(got) != 1 || got[0].ID != "watch_v1" {
		t.Fatalf("initial: got %+v, want watch_v1", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = l.Watch(ctx)
	}()

	// 等 watcher 启动
	time.Sleep(100 * time.Millisecond)

	// 修改文件
	writeYAML(t, dir, "watch.yaml", `
id: watch_v2
steps:
  - id: s1
    events:
      - type: stream_end
        data: { finishReason: stop }
`)

	// 等防抖 + reload
	time.Sleep(800 * time.Millisecond)

	scens := l.GetScenarios()
	if len(scens) != 1 {
		t.Fatalf("scenarios after reload = %d, want 1", len(scens))
	}
	if scens[0].ID != "watch_v2" {
		t.Errorf("id after reload = %q, want watch_v2", scens[0].ID)
	}

	cancel()
	wg.Wait()
}

// ════════════════════════════════════════════════════════════════
// 工具
// ════════════════════════════════════════════════════════════════

func writeYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
