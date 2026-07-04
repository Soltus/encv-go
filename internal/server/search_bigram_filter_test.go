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

// TestHasSufficientBigramOverlapEx_Relaxed 验证 BigramRelaxed 档的过滤逻辑。
//
// 规则：共享 >= 1 个 bigram 即通过（结果过少时放宽，宁多勿少）。
// 用 queryBigrams=["在线","线视","视频"]（n=3，Relaxed 阈值=1）。
func TestHasSufficientBigramOverlapEx_Relaxed(t *testing.T) {
	queryBigrams := []string{"在线", "线视", "视频"} // 来自 "在线视频"，n=3

	cases := []struct {
		name     string
		fileName string
		shared   int // 期望共享的 bigram 数量（注释说明，便于人工核对）
		want     bool
	}{
		{
			name:     "共享 2/3（在线+视频）→ 通过",
			fileName: "在线播放-高清视频.mp4",
			shared:   2,
			want:     true, // 2 >= 1
		},
		{
			name:     "共享 1/3（仅在线）→ 通过",
			fileName: "在线文档.pdf",
			shared:   1,
			want:     true, // 1 >= 1
		},
		{
			name:     "共享 1/3（仅线视）→ 通过",
			fileName: "线视图集.mp4",
			shared:   1,
			want:     true,
		},
		{
			name:     "共享 1/3（仅视频）→ 通过",
			fileName: "视频合集.mp4",
			shared:   1,
			want:     true,
		},
		{
			name:     "共享 0/3 → 过滤",
			fileName: "无关文件.txt",
			shared:   0,
			want:     false, // 0 < 1
		},
		{
			name:     "共享 3/3（全部命中）→ 通过",
			fileName: "在线线视视频.mp4",
			shared:   3,
			want:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramRelaxed)
			if got != tc.want {
				t.Errorf("hasSufficientBigramOverlapEx(%q, %v, BigramRelaxed) = %v, want %v (shared=%d, threshold=1)",
					tc.fileName, queryBigrams, got, tc.want, tc.shared)
			}
		})
	}
}

// TestHasSufficientBigramOverlapEx_Medium 验证 BigramMedium 档的过滤逻辑。
//
// 规则：共享 >= 一半 bigram（向上取整），是默认强度。
// 用 queryBigrams=["在线","线视","视频"]（n=3，Medium 阈值=ceil(3/2)=2）。
// 与 Relaxed 对照：shared=1 的用例在 Relaxed 通过、Medium 被过滤。
func TestHasSufficientBigramOverlapEx_Medium(t *testing.T) {
	queryBigrams := []string{"在线", "线视", "视频"} // n=3, threshold=(3+1)/2=2

	cases := []struct {
		name     string
		fileName string
		shared   int
		want     bool
	}{
		{
			name:     "共享 2/3（在线+视频）→ 通过",
			fileName: "在线播放-高清视频.mp4",
			shared:   2,
			want:     true, // 2 >= 2
		},
		{
			name:     "共享 1/3（仅在线）→ 过滤",
			fileName: "在线文档.pdf",
			shared:   1,
			want:     false, // 1 < 2
		},
		{
			name:     "共享 1/3（仅线视）→ 过滤",
			fileName: "线视图集.mp4",
			shared:   1,
			want:     false,
		},
		{
			name:     "共享 1/3（仅视频）→ 过滤",
			fileName: "视频合集.mp4",
			shared:   1,
			want:     false,
		},
		{
			name:     "共享 0/3 → 过滤",
			fileName: "无关文件.txt",
			shared:   0,
			want:     false,
		},
		{
			name:     "共享 3/3（全部命中）→ 通过",
			fileName: "在线视频合集.mp4",
			shared:   3,
			want:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramMedium)
			if got != tc.want {
				t.Errorf("hasSufficientBigramOverlapEx(%q, %v, BigramMedium) = %v, want %v (shared=%d, threshold=2)",
					tc.fileName, queryBigrams, got, tc.want, tc.shared)
			}
		})
	}
}

// TestHasSufficientBigramOverlapEx_Strict 验证 BigramStrict 档的过滤逻辑。
//
// 规则：共享全部 bigram 才通过（结果过多时收紧）。
// 用 queryBigrams=["在线","线视","视频"]（n=3，Strict 阈值=3）。
// 与 Medium 对照：shared=2 的用例在 Medium 通过、Strict 被过滤。
func TestHasSufficientBigramOverlapEx_Strict(t *testing.T) {
	queryBigrams := []string{"在线", "线视", "视频"} // n=3, threshold=3

	cases := []struct {
		name     string
		fileName string
		shared   int
		want     bool
	}{
		{
			name:     "共享 2/3（在线+视频，缺线视）→ 过滤",
			fileName: "在线播放-高清视频.mp4",
			shared:   2,
			want:     false, // 2 < 3
		},
		{
			name:     "共享 1/3（仅在线）→ 过滤",
			fileName: "在线文档.pdf",
			shared:   1,
			want:     false,
		},
		{
			name:     "共享 3/3（在线视频）→ 通过",
			fileName: "在线视频.mp4",
			shared:   3,
			want:     true, // 3 >= 3
		},
		{
			name:     "共享 3/3（在线视频合集）→ 通过",
			fileName: "在线视频合集.mp4",
			shared:   3,
			want:     true,
		},
		{
			name:     "共享 3/3（在线线视视频）→ 通过",
			fileName: "在线线视视频.mp4",
			shared:   3,
			want:     true,
		},
		{
			name:     "共享 3/3（重复匹配每个 bigram 只算一次）→ 通过",
			fileName: "在线视频在线线视视频.mp4",
			shared:   3,
			want:     true, // 3 个 bigram 各命中一次，shared=3
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramStrict)
			if got != tc.want {
				t.Errorf("hasSufficientBigramOverlapEx(%q, %v, BigramStrict) = %v, want %v (shared=%d, threshold=3)",
					tc.fileName, queryBigrams, got, tc.want, tc.shared)
			}
		})
	}
}

// TestBigramOverlapThreeTiersComparison 三档严格度对照测试（核心）。
//
// 对同一 (fileName, queryBigrams) 组合，验证三档返回值符合阈值递进关系：
//   - Relaxed(>=1) ⊇ Medium(>=2) ⊇ Strict(>=3)
//   - 即 Strict=true 蕴含 Medium=true 蕴含 Relaxed=true
// 用 queryBigrams=["在线","线视","视频"]（n=3，Medium 阈值=2）。
func TestBigramOverlapThreeTiersComparison(t *testing.T) {
	queryBigrams := []string{"在线", "线视", "视频"} // n=3

	cases := []struct {
		name         string
		fileName     string
		shared       int
		expectRelax  bool // BigramRelaxed 阈值 1
		expectMedium bool // BigramMedium 阈值 2
		expectStrict bool // BigramStrict 阈值 3
	}{
		{
			name:         "共享 3/3 → 三档全通过",
			fileName:     "在线视频.mp4",
			shared:       3,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: true,
		},
		{
			name:         "共享 2/3（在线+视频）→ Relaxed+Medium 通过，Strict 过滤",
			fileName:     "在线播放-高清视频.mp4",
			shared:       2,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: false,
		},
		{
			name:         "共享 1/3（仅在线）→ 仅 Relaxed 通过",
			fileName:     "在线文档.pdf",
			shared:       1,
			expectRelax:  true,
			expectMedium: false,
			expectStrict: false,
		},
		{
			name:         "共享 1/3（仅线视）→ 仅 Relaxed 通过",
			fileName:     "线视图集.mp4",
			shared:       1,
			expectRelax:  true,
			expectMedium: false,
			expectStrict: false,
		},
		{
			name:         "共享 1/3（仅视频）→ 仅 Relaxed 通过",
			fileName:     "视频合集.mp4",
			shared:       1,
			expectRelax:  true,
			expectMedium: false,
			expectStrict: false,
		},
		{
			name:         "共享 0/3 → 三档全过滤",
			fileName:     "无关文件.txt",
			shared:       0,
			expectRelax:  false,
			expectMedium: false,
			expectStrict: false,
		},
		{
			name:         "共享 2/3（在线+线视，缺视频）→ Relaxed+Medium 通过，Strict 过滤",
			fileName:     "在线线视.mp4",
			shared:       2,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: false,
		},
		{
			name:         "共享 2/3（线视+视频，缺在线）→ Relaxed+Medium 通过，Strict 过滤",
			fileName:     "线视视频.mp4",
			shared:       2,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRelax := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramRelaxed)
			gotMedium := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramMedium)
			gotStrict := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramStrict)

			if gotRelax != tc.expectRelax {
				t.Errorf("Relaxed: hasSufficientBigramOverlapEx(%q,...,BigramRelaxed) = %v, want %v (shared=%d, threshold=1)",
					tc.fileName, gotRelax, tc.expectRelax, tc.shared)
			}
			if gotMedium != tc.expectMedium {
				t.Errorf("Medium: hasSufficientBigramOverlapEx(%q,...,BigramMedium) = %v, want %v (shared=%d, threshold=2)",
					tc.fileName, gotMedium, tc.expectMedium, tc.shared)
			}
			if gotStrict != tc.expectStrict {
				t.Errorf("Strict: hasSufficientBigramOverlapEx(%q,...,BigramStrict) = %v, want %v (shared=%d, threshold=3)",
					tc.fileName, gotStrict, tc.expectStrict, tc.shared)
			}

			// 阈值递进一致性校验：Strict=true 必须 蕴含 Medium=true 蕴含 Relaxed=true
			if gotStrict && !gotMedium {
				t.Errorf("一致性破坏：Strict=true 但 Medium=false（%q）", tc.fileName)
			}
			if gotMedium && !gotRelax {
				t.Errorf("一致性破坏：Medium=true 但 Relaxed=false（%q）", tc.fileName)
			}
		})
	}
}

// TestBigramOverlapBoundary_N1 验证 n=1 边界（三档阈值都=1，行为相同）。
//
// queryBigrams=["视频"]（n=1）：Relaxed>=1, Medium>=(1+1)/2=1, Strict>=1。
func TestBigramOverlapBoundary_N1(t *testing.T) {
	queryBigrams := []string{"视频"} // n=1, 三档阈值都=1

	cases := []struct {
		name         string
		fileName     string
		shared       int
		expectRelax  bool
		expectMedium bool
		expectStrict bool
	}{
		{
			name:         "共享 1/1（精确命中）→ 三档全通过",
			fileName:     "视频.mp4",
			shared:       1,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: true,
		},
		{
			name:         "共享 1/1（命中且文件名更长）→ 三档全通过",
			fileName:     "视频合集.mp4",
			shared:       1,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: true,
		},
		{
			name:         "共享 0/1 → 三档全过滤",
			fileName:     "无关.txt",
			shared:       0,
			expectRelax:  false,
			expectMedium: false,
			expectStrict: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRelax := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramRelaxed)
			gotMedium := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramMedium)
			gotStrict := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramStrict)
			if gotRelax != tc.expectRelax || gotMedium != tc.expectMedium || gotStrict != tc.expectStrict {
				t.Errorf("n=1 三档结果 (R=%v,M=%v,S=%v), want (R=%v,M=%v,S=%v) (shared=%d, file=%q)",
					gotRelax, gotMedium, gotStrict, tc.expectRelax, tc.expectMedium, tc.expectStrict, tc.shared, tc.fileName)
			}
		})
	}
}

// TestBigramOverlapBoundary_N2 验证 n=2 边界（Relaxed==Medium，Strict 更严）。
//
// queryBigrams=["在线","视频"]（n=2）：Relaxed>=1, Medium>=(2+1)/2=1, Strict>=2。
// 此时 Relaxed 与 Medium 行为完全相同，Strict 需要全部命中。
func TestBigramOverlapBoundary_N2(t *testing.T) {
	queryBigrams := []string{"在线", "视频"} // n=2, Relaxed=1, Medium=1, Strict=2

	cases := []struct {
		name         string
		fileName     string
		shared       int
		expectRelax  bool
		expectMedium bool
		expectStrict bool
	}{
		{
			name:         "共享 2/2（两个都命中）→ 三档全通过",
			fileName:     "在线视频.mp4",
			shared:       2,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: true, // 2 >= 2
		},
		{
			name:         "共享 1/2（仅在线）→ Relaxed+Medium 通过，Strict 过滤",
			fileName:     "在线文档.mp4",
			shared:       1,
			expectRelax:  true,  // 1 >= 1
			expectMedium: true,  // 1 >= 1（Medium 阈值=1，n=2 时与 Relaxed 相同）
			expectStrict: false, // 1 < 2
		},
		{
			name:         "共享 1/2（仅视频）→ Relaxed+Medium 通过，Strict 过滤",
			fileName:     "视频.mp4",
			shared:       1,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: false,
		},
		{
			name:         "共享 0/2 → 三档全过滤",
			fileName:     "无关.txt",
			shared:       0,
			expectRelax:  false,
			expectMedium: false,
			expectStrict: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRelax := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramRelaxed)
			gotMedium := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramMedium)
			gotStrict := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramStrict)
			if gotRelax != tc.expectRelax || gotMedium != tc.expectMedium || gotStrict != tc.expectStrict {
				t.Errorf("n=2 三档结果 (R=%v,M=%v,S=%v), want (R=%v,M=%v,S=%v) (shared=%d, file=%q)",
					gotRelax, gotMedium, gotStrict, tc.expectRelax, tc.expectMedium, tc.expectStrict, tc.shared, tc.fileName)
			}
		})
	}
}

// TestBigramOverlapBoundary_N4 验证 n=4 边界（三档阈值分得最开）。
//
// queryBigrams=["在线","线视","视频","高清"]（n=4）：
// Relaxed>=1, Medium>=(4+1)/2=2, Strict>=4。
func TestBigramOverlapBoundary_N4(t *testing.T) {
	queryBigrams := []string{"在线", "线视", "视频", "高清"} // n=4

	cases := []struct {
		name         string
		fileName     string
		shared       int
		expectRelax  bool
		expectMedium bool
		expectStrict bool
	}{
		{
			name:         "共享 4/4（全部命中）→ 三档全通过",
			fileName:     "在线线视视频高清.mp4",
			shared:       4,
			expectRelax:  true,
			expectMedium: true, // 4 >= 2
			expectStrict: true, // 4 >= 4
		},
		{
			name:         "共享 2/4（在线+视频）→ Relaxed+Medium 通过，Strict 过滤",
			fileName:     "在线视频.mp4",
			shared:       2,
			expectRelax:  true,  // 2 >= 1
			expectMedium: true,  // 2 >= 2
			expectStrict: false, // 2 < 4
		},
		{
			name:         "共享 1/4（仅在线）→ 仅 Relaxed 通过",
			fileName:     "在线文档.mp4",
			shared:       1,
			expectRelax:  true,  // 1 >= 1
			expectMedium: false, // 1 < 2
			expectStrict: false,
		},
		{
			name:         "共享 0/4 → 三档全过滤",
			fileName:     "无关.txt",
			shared:       0,
			expectRelax:  false,
			expectMedium: false,
			expectStrict: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRelax := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramRelaxed)
			gotMedium := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramMedium)
			gotStrict := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramStrict)
			if gotRelax != tc.expectRelax || gotMedium != tc.expectMedium || gotStrict != tc.expectStrict {
				t.Errorf("n=4 三档结果 (R=%v,M=%v,S=%v), want (R=%v,M=%v,S=%v) (shared=%d, file=%q)",
					gotRelax, gotMedium, gotStrict, tc.expectRelax, tc.expectMedium, tc.expectStrict, tc.shared, tc.fileName)
			}
		})
	}
}

// TestBigramOverlap_EmptyQueryEx 验证查询无 bigram 时 Ex 版本三档都不过滤。
//
// 场景：extractBigrams 返回 nil 或空切片（纯单字查询或空字符串）。
// 此时无 bigram 可用作过滤依据，三档严格度都应返回 true。
func TestBigramOverlap_EmptyQueryEx(t *testing.T) {
	cases := []struct {
		name       string
		fileName   string
		bigrams    []string
		strictness BigramStrictness
		want       bool
	}{
		{
			name:       "nil bigrams + BigramRelaxed → 不过滤",
			fileName:   "任意.txt",
			bigrams:    nil,
			strictness: BigramRelaxed,
			want:       true,
		},
		{
			name:       "nil bigrams + BigramStrict → 不过滤",
			fileName:   "任意.txt",
			bigrams:    nil,
			strictness: BigramStrict,
			want:       true,
		},
		{
			name:       "空切片 bigrams + BigramMedium → 不过滤",
			fileName:   "任意.txt",
			bigrams:    []string{},
			strictness: BigramMedium,
			want:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSufficientBigramOverlapEx(tc.fileName, tc.bigrams, tc.strictness)
			if got != tc.want {
				t.Errorf("hasSufficientBigramOverlapEx(%q, %v, strictness=%d) = %v, want %v",
					tc.fileName, tc.bigrams, tc.strictness, got, tc.want)
			}
		})
	}
}

// TestBigramOverlap_CaseInsensitive 验证英文查询大小写不敏感（三档对照）。
//
// 源码用 strings.ToLower(fileName) 后再 Contains 匹配，故大小写不影响命中。
// queryBigrams=["video","audio"]（n=2）：Relaxed>=1, Medium>=1, Strict>=2。
func TestBigramOverlap_CaseInsensitive(t *testing.T) {
	queryBigrams := []string{"video", "audio"} // n=2

	cases := []struct {
		name         string
		fileName     string
		shared       int
		expectRelax  bool
		expectMedium bool
		expectStrict bool
	}{
		{
			name:         "含大写 Video（仅 video 命中）→ Relaxed+Medium 通过，Strict 过滤",
			fileName:     "My Video.mp4",
			shared:       1,
			expectRelax:  true,  // 1 >= 1
			expectMedium: true,  // 1 >= 1（n=2 时 Medium 阈值=1）
			expectStrict: false, // 1 < 2
		},
		{
			name:         "含大写 AUDIO（仅 audio 命中）→ Relaxed+Medium 通过，Strict 过滤",
			fileName:     "AUDIO recording.mp3",
			shared:       1,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: false,
		},
		{
			name:         "含 Video Audio（两者都命中，大小写混合）→ 三档全通过",
			fileName:     "Video Audio Mix.mp4",
			shared:       2,
			expectRelax:  true,
			expectMedium: true,
			expectStrict: true, // 2 >= 2
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRelax := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramRelaxed)
			gotMedium := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramMedium)
			gotStrict := hasSufficientBigramOverlapEx(tc.fileName, queryBigrams, BigramStrict)
			if gotRelax != tc.expectRelax || gotMedium != tc.expectMedium || gotStrict != tc.expectStrict {
				t.Errorf("大小写不敏感三档结果 (R=%v,M=%v,S=%v), want (R=%v,M=%v,S=%v) (shared=%d, file=%q)",
					gotRelax, gotMedium, gotStrict, tc.expectRelax, tc.expectMedium, tc.expectStrict, tc.shared, tc.fileName)
			}
		})
	}
}
