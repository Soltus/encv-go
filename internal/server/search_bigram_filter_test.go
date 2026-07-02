package server

import (
	"testing"
)

// TestExtractBigrams 验证从查询中提取非单字 token（CJK bigram + 英文单词）。
//
// Tokenize 行为：
//   - 中文连续字符按 2-gram 滑窗切分（在线视频 → 在线/线视/视频）
//   - 英文按单词切分（hello world → hello/world）
//   - 数字、标点、空白作为分隔符
//   - 单字 CJK 字符被过滤（长度 < 2）
func TestExtractBigrams(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantLen int
		want    []string
	}{
		{
			name:    "纯中文 4 字 → 3 个 bigram",
			query:   "在线视频",
			wantLen: 3,
			want:    []string{"在线", "线视", "视频"},
		},
		{
			name:    "纯中文 2 字 → 1 个 bigram",
			query:   "在线",
			wantLen: 1,
			want:    []string{"在线"},
		},
		{
			name:    "空字符串 → 0 bigram",
			query:   "",
			wantLen: 0,
		},
		{
			name:    "单字 CJK → 0 bigram（过滤单字）",
			query:   "在",
			wantLen: 0,
		},
		{
			name:    "英文单词 → 1 bigram",
			query:   "video",
			wantLen: 1,
			want:    []string{"video"},
		},
		{
			name:    "空格分隔多 token",
			query:   "在线 高清",
			wantLen: 2,
			want:    []string{"在线", "高清"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractBigrams(tc.query)
			if len(got) != tc.wantLen {
				t.Fatalf("extractBigrams(%q) = %v, want len=%d", tc.query, got, tc.wantLen)
			}
			if tc.want != nil {
				for i, w := range tc.want {
					if i >= len(got) || got[i] != w {
						t.Errorf("extractBigrams(%q)[%d] = %q, want %q", tc.query, i, got, w)
					}
				}
			}
			t.Logf("✅ extractBigrams(%q) = %v", tc.query, got)
		})
	}
}

// TestHasSufficientBigramOverlap 验证文件名与查询共享 bigram 的过滤逻辑。
//
// 规则：共享 bigram 数量 >= 查询 bigram 数量的一半（向上取整）
//   - 查询 "在线视频" bigrams=["在线","线视","视频"]（3 个，阈值=2）
//   - "在线播放-高清视频.mp4" 共享 ["在线","视频"]（2 个）→ true
//   - "在线文档.pdf" 共享 ["在线"]（1 个）→ false
func TestHasSufficientBigramOverlap(t *testing.T) {
	queryBigrams := []string{"在线", "线视", "视频"} // 来自 "在线视频"

	cases := []struct {
		name     string
		fileName string
		want     bool
	}{
		{
			name:     "匹配 2/3 bigram（在线+视频）→ 通过",
			fileName: "在线播放-高清视频.mp4",
			want:     true,
		},
		{
			name:     "匹配 1/3 bigram（仅在线）→ 过滤",
			fileName: "在线文档.pdf",
			want:     false,
		},
		{
			name:     "匹配 0/3 bigram → 过滤",
			fileName: "无关文件.txt",
			want:     false,
		},
		{
			name:     "匹配 3/3 bigram → 通过",
			fileName: "在线视频合集.mp4",
			want:     true,
		},
		{
			name:     "大小写不敏感（英文 bigram）",
			fileName: "MyVideo.mp4",
			want:     false, // 中文 bigrams 不会匹配英文文件名
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSufficientBigramOverlap(tc.fileName, queryBigrams)
			if got != tc.want {
				t.Errorf("hasSufficientBigramOverlap(%q, %v) = %v, want %v",
					tc.fileName, queryBigrams, got, tc.want)
			}
			t.Logf("✅ %q → %v", tc.fileName, got)
		})
	}
}

// TestHasSufficientBigramOverlap_EmptyQuery 验证查询无 bigram 时不过滤。
//
// 场景：查询是纯单字（如 "在"）或空字符串 → extractBigrams 返回 []
// 此时 hasSufficientBigramOverlap 应返回 true（不过滤任何文件），
// 因为无 bigram 可用作过滤依据。
func TestHasSufficientBigramOverlap_EmptyQuery(t *testing.T) {
	if !hasSufficientBigramOverlap("任意文件.txt", nil) {
		t.Error("empty query bigrams should not filter (return true)")
	}
	if !hasSufficientBigramOverlap("任意文件.txt", []string{}) {
		t.Error("empty query bigrams should not filter (return true)")
	}
	t.Log("✅ empty query bigrams → no filter (return true)")
}

// TestHasSufficientBigramOverlap_EnglishQuery 验证英文查询的过滤逻辑。
//
// 场景：查询 "video audio" bigrams=["video","audio"]（2 个，阈值=1）
//   - "my video file.mp4" 共享 ["video"]（1 个）→ 1>=1 → true
//   - "audio only.mp3" 共享 ["audio"]（1 个）→ 1>=1 → true
//   - "image.png" 共享 0 个 → false
func TestHasSufficientBigramOverlap_EnglishQuery(t *testing.T) {
	queryBigrams := []string{"video", "audio"} // 来自 "video audio"

	cases := []struct {
		name     string
		fileName string
		want     bool
	}{
		{"含 video → 通过", "my VIDEO file.mp4", true}, // 大小写不敏感
		{"含 audio → 通过", "Audio recording.mp3", true},
		{"含两者 → 通过", "video audio mixer.mp4", true},
		{"都不含 → 过滤", "image.png", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSufficientBigramOverlap(tc.fileName, queryBigrams)
			if got != tc.want {
				t.Errorf("hasSufficientBigramOverlap(%q, %v) = %v, want %v",
					tc.fileName, queryBigrams, got, tc.want)
			}
		})
	}
}
