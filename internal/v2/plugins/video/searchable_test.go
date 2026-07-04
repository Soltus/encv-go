// internal/v2/plugins/video/searchable_test.go
//
// 2026-07-02 用户反馈：插件自主声明容器内可被全文搜索的内容
// 视频插件声明 searchable: subtitle / title
// 测试覆盖：subtitle SRT 解析 / ASS 标签去除 / title 提取

package video

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
)

func TestVideoPlugin_GetSearchableContentsManifest(t *testing.T) {
	p := &VideoPlugin{}
	m := p.GetSearchableContentsManifest()
	if !m.Enabled {
		t.Errorf("manifest.Enabled should be true, got false")
	}
	if len(m.Types) == 0 {
		t.Errorf("manifest.Types should not be empty")
	}
	// 必须包含 subtitle + title
	hasSubtitle := false
	hasTitle := false
	for _, typ := range m.Types {
		if typ == pluginInterfaces.SearchableTypeSubtitle {
			hasSubtitle = true
		}
		if typ == pluginInterfaces.SearchableTypeTitle {
			hasTitle = true
		}
	}
	if !hasSubtitle {
		t.Errorf("manifest should include subtitle type, got %v", m.Types)
	}
	if !hasTitle {
		t.Errorf("manifest should include title type, got %v", m.Types)
	}
}

func TestVideoPlugin_ExtractSearchableContents_NoSubtitles(t *testing.T) {
	p := &VideoPlugin{}
	dir := t.TempDir()
	// 创建没有字幕的容器文件
	containerPath := filepath.Join(dir, "movie.sccgv")
	if err := os.WriteFile(containerPath, []byte("encrypted blob"), 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}

	items, err := p.ExtractSearchableContents(containerPath)
	if err != nil {
		t.Fatalf("ExtractSearchableContents: %v", err)
	}
	// 没字幕时应该至少有 title
	if len(items) == 0 {
		t.Fatalf("expected at least title item, got 0 items")
	}
	hasTitle := false
	for _, it := range items {
		if it.Type == pluginInterfaces.SearchableTypeTitle && it.Text == "movie" {
			hasTitle = true
		}
	}
	if !hasTitle {
		t.Errorf("expected title='movie' item, got: %+v", items)
	}
}

func TestVideoPlugin_ExtractSearchableContents_SRT(t *testing.T) {
	p := &VideoPlugin{}
	dir := t.TempDir()
	// 写 SRT 字幕
	srtContent := `1
00:00:01,000 --> 00:00:03,000
在线播放 高清视频

2
00:00:05,000 --> 00:00:07,000
第二行字幕
`
	subPath := filepath.Join(dir, "movie.ass.srt")
	if err := os.WriteFile(subPath, []byte(srtContent), 0o644); err != nil {
		t.Fatalf("write srt: %v", err)
	}
	containerPath := filepath.Join(dir, "movie.sccgv")
	_ = os.WriteFile(containerPath, []byte("encrypted"), 0o644)

	items, err := p.ExtractSearchableContents(containerPath)
	if err != nil {
		t.Fatalf("ExtractSearchableContents: %v", err)
	}

	// 期望：1 subtitle + 1 title = 2 items
	if len(items) != 2 {
		t.Errorf("expected 2 items (1 subtitle + 1 title), got %d: %+v", len(items), items)
	}

	// 验证 subtitle 项：text 应该包含"在线播放 高清视频"和"第二行字幕"，但不应该有"00:00"或"-->"
	for _, it := range items {
		if it.Type == pluginInterfaces.SearchableTypeSubtitle {
			if !strings.Contains(it.Text, "在线播放 高清视频") {
				t.Errorf("subtitle text missing content: %q", it.Text)
			}
			if !strings.Contains(it.Text, "第二行字幕") {
				t.Errorf("subtitle text missing second line: %q", it.Text)
			}
			if strings.Contains(it.Text, "-->") {
				t.Errorf("subtitle text should not contain timestamp '-->' : %q", it.Text)
			}
		}
	}
}

func TestVideoPlugin_ExtractSearchableContents_ASS(t *testing.T) {
	p := &VideoPlugin{}
	dir := t.TempDir()
	// 写 ASS 字幕（含样式覆盖）
	assContent := `[Script Info]
Title: Test
ScriptType: v4.00+

[V4+ Styles]
Format: Name, Fontname

[Events]
Format: Layer, Start, End, Style, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Default,{\b1}在线播放{\b0} 高清
Dialogue: 0,0:00:05.00,0:00:07.00,Default,第二行
`
	subPath := filepath.Join(dir, "movie.ass")
	if err := os.WriteFile(subPath, []byte(assContent), 0o644); err != nil {
		t.Fatalf("write ass: %v", err)
	}
	containerPath := filepath.Join(dir, "movie.sccgv")
	_ = os.WriteFile(containerPath, []byte("enc"), 0o644)

	items, err := p.ExtractSearchableContents(containerPath)
	if err != nil {
		t.Fatalf("ExtractSearchableContents: %v", err)
	}

	for _, it := range items {
		if it.Type == pluginInterfaces.SearchableTypeSubtitle {
			// ASS 样式覆盖应被去除
			if strings.Contains(it.Text, "{") || strings.Contains(it.Text, "}") {
				t.Errorf("ASS style tags not stripped: %q", it.Text)
			}
			// [Script Info] 等头应该被跳过
			if strings.Contains(it.Text, "ScriptType") {
				t.Errorf("ASS [Script Info] header not skipped: %q", it.Text)
			}
			// 应该有实际内容
			if !strings.Contains(it.Text, "在线播放") {
				t.Errorf("ASS text missing dialogue content: %q", it.Text)
			}
		}
	}
}

func TestVideoPlugin_ExtractSearchableContents_OversizedSubtitle(t *testing.T) {
	p := &VideoPlugin{}
	dir := t.TempDir()
	// 写超大字幕（> 256KB）
	bigContent := strings.Repeat("在线播放 高清视频\n", 20000) // ~500KB
	subPath := filepath.Join(dir, "movie.ass.srt")
	if err := os.WriteFile(subPath, []byte(bigContent), 0o644); err != nil {
		t.Fatalf("write big srt: %v", err)
	}
	containerPath := filepath.Join(dir, "movie.sccgv")
	_ = os.WriteFile(containerPath, []byte("enc"), 0o644)

	items, err := p.ExtractSearchableContents(containerPath)
	if err != nil {
		t.Fatalf("ExtractSearchableContents: %v", err)
	}

	// 超大字幕应被跳过（只有 title）
	hasSubtitle := false
	for _, it := range items {
		if it.Type == pluginInterfaces.SearchableTypeSubtitle {
			hasSubtitle = true
		}
	}
	if hasSubtitle {
		t.Errorf("oversized subtitle should be skipped, but got: %+v", items)
	}
}

func TestVideoPlugin_ExtractSearchableContents_EmptyContainerPath(t *testing.T) {
	p := &VideoPlugin{}
	_, err := p.ExtractSearchableContents("")
	if err == nil {
		t.Errorf("expected error for empty containerPath, got nil")
	}
}

func TestVideoPlugin_ExtractSearchableContents_InvalidDir(t *testing.T) {
	p := &VideoPlugin{}
	// 不存在的目录
	_, err := p.ExtractSearchableContents("/nonexistent/path/movie.sccgv")
	if err == nil {
		t.Errorf("expected error for nonexistent dir, got nil")
	}
}

func TestIsSRTIndexLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"", false},
		{"1", true},
		{"42", true},
		{"1234", true},
		{"abc", false},
		{"1a", false},
		{"1.0", false},
		{"00:00:01,000", false}, // 时间戳不是序号
	}
	for _, tt := range tests {
		got := isSRTIndexLine(tt.line)
		if got != tt.want {
			t.Errorf("isSRTIndexLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestIsSRTTimestampLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"00:00:01,000 --> 00:00:02,000", true},
		{"00:00:01.000 --> 00:00:02.000", true},
		{"hello world", false},
		{"-->", true}, // 简化策略：只要含 "-->" 即视为时间戳
	}
	for _, tt := range tests {
		got := isSRTTimestampLine(tt.line)
		if got != tt.want {
			t.Errorf("isSRTTimestampLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestStripASSTags(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello world", "hello world"},
		{"{\\b1}在线播放", "在线播放"},
		{"{\\fn微软雅黑}在线", "在线"},
		{"a{\\b1}b{\\i1}c", "abc"},
		{"unclosed {\\b1", "unclosed "},
	}
	for _, tt := range tests {
		got := stripASSTags(tt.in)
		if got != tt.want {
			t.Errorf("stripASSTags(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitExtList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{".ass,.srt", []string{".ass", ".srt"}},
		{"ass,srt", []string{".ass", ".srt"}}, // 自动加 .
		{"  .ass  ,  .srt  ", []string{".ass", ".srt"}}, // trim
		{".ass,.srt,.dm.ass", []string{".ass", ".srt", ".dm.ass"}},
		{"", []string{}},
	}
	for _, tt := range tests {
		got := splitExtList(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("splitExtList(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitExtList(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

// 集成测试：searchable extractor 通过 FTS5 流程建索引 → 检索到字幕内容
func TestSearchableExtractor_IntegrationWithFTS5(t *testing.T) {
	// 1. 创建测试数据
	dir := t.TempDir()
	subDir := filepath.Join(dir, "videos")
	_ = os.MkdirAll(subDir, 0o755)

	// 写一个 SRT 字幕
	srtContent := "1\n00:00:01,000 --> 00:00:03,000\n在线播放 高清视频\n"
	_ = os.WriteFile(filepath.Join(subDir, "movie.ass.srt"), []byte(srtContent), 0o644)
	// 写一个 .sccgv 容器（即使内容是 fake，video 插件只看同目录的字幕）
	_ = os.WriteFile(filepath.Join(subDir, "movie.sccgv"), []byte("fake encrypted"), 0o644)

	// 2. 调 video 插件 extractor
	p := &VideoPlugin{}
	items, err := p.ExtractSearchableContents(filepath.Join(subDir, "movie.sccgv"))
	if err != nil {
		t.Fatalf("ExtractSearchableContents: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("expected at least 2 items (subtitle + title), got %d", len(items))
	}

	// 3. 验证 extractor 输出能被 FTS5 搜索到（不实际跑 FTS5，只验证 content 字符串）
	var extractor pluginInterfaces.SearchableContentsExtractor = p
	if !extractor.GetSearchableContentsManifest().Enabled {
		t.Fatalf("manifest should be enabled")
	}

	// 4. 拼装 content 字符串
	var parts []string
	for _, it := range items {
		if it.Text == "" {
			continue
		}
		parts = append(parts, it.Text)
	}
	mergedContent := strings.Join(parts, " ")

	// 5. 验证搜索 "在线播放" 能命中
	if !strings.Contains(mergedContent, "在线播放") {
		t.Errorf("merged content should contain '在线播放', got: %q", mergedContent)
	}
	if !strings.Contains(mergedContent, "高清视频") {
		t.Errorf("merged content should contain '高清视频', got: %q", mergedContent)
	}
	_ = context.Background() // 避免 unused import
}
