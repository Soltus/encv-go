package vectorsearch

import (
	"math"
	"testing"
)

func TestTokenizeChinese(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"中文测试", []string{"中文", "中", "文测", "文", "测试", "测", "试"}},
		{"文件", []string{"文件", "文", "件"}},
		{"", nil},
	}
	for _, tt := range tests {
		got := Tokenize(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("Tokenize(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestTokenizeMixed(t *testing.T) {
	tokens := Tokenize("我的 video.mp4 文件")
	if len(tokens) == 0 {
		t.Error("expected tokens, got none")
	}
	t.Logf("tokens: %v", tokens)
}

func TestTextToVector(t *testing.T) {
	vec1 := TextToVector("中文文件测试")
	vec2 := TextToVector("文件测试中文")
	vec3 := TextToVector("完全不同的内容")

	if len(vec1) != VectorDim {
		t.Errorf("vector dim = %d, want %d", len(vec1), VectorDim)
	}

	// 相似文本应该有较高的相似度
	sim12 := CosineSimilarity(vec1, vec2)
	sim13 := CosineSimilarity(vec1, vec3)

	t.Logf("sim(中文文件测试, 文件测试中文) = %.4f", sim12)
	t.Logf("sim(中文文件测试, 完全不同的内容) = %.4f", sim13)

	if sim12 <= sim13 {
		t.Errorf("similar texts should have higher similarity: sim12=%.4f, sim13=%.4f", sim12, sim13)
	}
}

func TestVectorNormalization(t *testing.T) {
	vec := TextToVector("测试向量归一化")
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.001 {
		t.Errorf("vector norm = %.4f, want ~1.0", norm)
	}
}

func TestQueryVector(t *testing.T) {
	docVec := TextToVector("加密视频文件的处理流程")
	queryVec := BuildQueryVector("加密视频")

	sim := CosineSimilarity(docVec, queryVec)
	t.Logf("query similarity = %.4f", sim)

	if sim <= 0 {
		t.Error("query should have positive similarity with matching doc")
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	vec := TextToVector("测试编码解码")
	encoded := EncodeVector(vec)

	if len(encoded) != len(vec)*4 {
		t.Errorf("encoded len = %d, want %d", len(encoded), len(vec)*4)
	}
}

// TestTokenizeChinese_Comprehensive 中文分词综合测试（bigram 滑窗 + 单字）。
// 验证连续 CJK 字符的 bigram 切分、CJK 与非 CJK 边界、空字符串等。
func TestTokenizeChinese_Comprehensive(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// 标准 4 字 CJK：3 bigram + 4 单字
		{"四字CJK", "中文测试", []string{"中文", "中", "文测", "文", "测试", "测", "试"}},
		// 2 字 CJK：1 bigram + 2 单字
		{"两字CJK", "文件", []string{"文件", "文", "件"}},
		// 空字符串：返回 nil
		{"空字符串", "", nil},
		// 单字 CJK：无 bigram，只有单字
		{"单字CJK", "中", []string{"中"}},
		// 2 字 CJK 复核
		{"两字CJK_中文", "中文", []string{"中文", "中", "文"}},
		// 3 字 CJK：2 bigram + 3 单字
		{"三字CJK", "中文字", []string{"中文", "中", "文字", "文", "字"}},
		// 英文 letter 起始：unicode.IsLetter 对 Han 为真，"a中" 被作为一个 token 吞下
		{"英文后单字CJK", "a中", []string{"a中"}},
		// 单字 CJK + 英文：CJK 先走 CJK 分支（"a" 非 CJK 无 bigram），再切英文 "a"
		{"单字CJK后英文", "中a", []string{"中", "a"}},
		// 6 字 CJK：5 bigram + 6 单字 = 11 token
		{"六字CJK", "在线视频高清", []string{"在线", "在", "线视", "线", "视频", "视", "频高", "频", "高清", "高", "清"}},
		// 4 字 CJK 复核
		{"四字CJK_在线视频", "在线视频", []string{"在线", "在", "线视", "线", "视频", "视", "频"}},
		// 长英文：连续 letter 作为一个 token，转小写
		{"长英文单词", "abcdefghij", []string{"abcdefghij"}},
		// 中英混合：空格分隔，"混合" 仍构成 bigram
		{"中英空格混合", "中文 English 混合", []string{"中文", "中", "文", "english", "混合", "混", "合"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTokenizeKorean 韩文分词测试（Hangul 范围 \uAC00-\uD7AF 按 CJK 处理）。
func TestTokenizeKorean(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// 5 字韩文：4 bigram + 5 单字 = 9 token
		{"五字韩文", "안녕하세요", []string{"안녕", "안", "녕하", "녕", "하세", "하", "세요", "세", "요"}},
		// 3 字韩文：2 bigram + 3 单字
		{"三字韩文", "한국어", []string{"한국", "한", "국어", "국", "어"}},
		// 单字韩文：无 bigram
		{"单字韩文", "가", []string{"가"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTokenizeJapanese 日文分词测试（假名范围 \u3040-\u30FF 按 CJK 处理）。
func TestTokenizeJapanese(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// 5 字平假名：4 bigram + 5 单字 = 9 token
		{"平假名こんにちは", "こんにちは", []string{"こん", "こ", "んに", "ん", "にち", "に", "ちは", "ち", "は"}},
		// 4 字片假名：3 bigram + 4 单字 = 7 token
		{"片假名カタカナ", "カタカナ", []string{"カタ", "カ", "タカ", "タ", "カナ", "カ", "ナ"}},
		// 4 字平假名：3 bigram + 4 单字 = 7 token
		{"平假名ひらがな", "ひらがな", []string{"ひら", "ひ", "らが", "ら", "がな", "が", "な"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTokenizeDigits 数字 token 化测试。
// letter+digit 连续时作为一个 token；digit 与 CJK 边界正确切分。
func TestTokenizeDigits(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// 纯数字：作为一个 token
		{"纯数字", "2026", []string{"2026"}},
		// 字母+数字连续：作为一个 token
		{"字母数字混合", "video2026", []string{"video2026"}},
		// 数字+字母连续：作为一个 token
		{"数字字母混合", "2026video", []string{"2026video"}},
		// CJK + 数字：CJK bigram + 单字 + 数字 token
		{"中文数字", "视频2026", []string{"视频", "视", "频", "2026"}},
		// 空格分隔的多个数字
		{"空格分隔数字", "2026 07 02", []string{"2026", "07", "02"}},
		// 点号分隔：letter+digit 与单独 digit
		{"点号分隔版本号", "v1.2.3", []string{"v1", "2", "3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTokenizePunctuation 标点/空白分隔测试。
// 标点、空白、连字符、下划线均作为分隔符跳过。
func TestTokenizePunctuation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// 纯标点：无 token，返回 nil
		{"纯at符号", "@@@", nil},
		// 纯感叹号：无 token
		{"纯感叹号", "!!!", nil},
		// CJK + 点号 + CJK：点号分隔，两边各自 bigram + 单字
		{"中文点号英文", "中文.英文", []string{"中文", "中", "文", "英文", "英", "文"}},
		// 连字符分隔英文
		{"连字符分隔", "hello-world", []string{"hello", "world"}},
		// 下划线分隔英文（_ 非 letter/digit）
		{"下划线分隔", "my_video_file", []string{"my", "video", "file"}},
		// 纯空格：TrimSpace 后为空，返回 nil
		{"纯空格", "  ", nil},
		// 制表符+换行：TrimSpace 后为空，返回 nil
		{"制表换行", "\t\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTokenizeEmoji emoji 处理测试。
// emoji 非 letter/digit/CJK，被跳过；emoji 后的 CJK 重新构成 bigram。
func TestTokenizeEmoji(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// CJK + emoji + CJK：emoji 跳过，两边各自 bigram + 单字
		{"中文emoji中文", "视频🎬电影", []string{"视频", "视", "频", "电影", "电", "影"}},
		// 纯 emoji：无 token，返回 nil
		{"纯emoji", "🎉🎉", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTokenizeEdgeCases 分词边界情况测试。
// 超长重复字符、大小写转换、中英数混合等。
func TestTokenizeEdgeCases(t *testing.T) {
	// 8 个重复 "在"：7 bigram + 8 单字 = 15 token
	repeat8 := []string{}
	for i := 0; i < 7; i++ {
		repeat8 = append(repeat8, "在在", "在")
	}
	repeat8 = append(repeat8, "在")

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// 8 个重复 CJK：7 bigram + 8 单字 = 15 token
		{"重复八字CJK", "在在在在在在在在", repeat8},
		// 全大写英文：转小写
		{"全大写英文", "VIDEO", []string{"video"}},
		// 多个英文单词空格分隔：转小写
		{"多英文单词", "Video Audio Mix", []string{"video", "audio", "mix"}},
		// 英文 letter 起始：unicode.IsLetter 对 Han 为真，"Mixed中英" 被作为一个 token 吞下
		{"英文后CJK", "Mixed中英", []string{"mixed中英"}},
		// 中英数混合：CJK bigram + 单字；"mp4文件2026" 因 letter/digit 连续（含 Han letter）作为一个 token
		{"中英数混合", "视频mp4文件2026", []string{"视频", "视", "频", "mp4文件2026"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTextToVector_EmptyAndDegenerate 向量化的空值与退化情况测试。
func TestTextToVector_EmptyAndDegenerate(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		// 空文本：返回 VectorDim 维全零向量
		{"空文本", ""},
		// 单字符英文
		{"单字符", "a"},
		// 单字符 CJK
		{"单字CJK", "测"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vec := TextToVector(tt.text)
			if len(vec) != VectorDim {
				t.Errorf("TextToVector(%q) dim = %d, want %d", tt.text, len(vec), VectorDim)
			}
		})
	}

	// 相同文本应产生相同向量（确定性）
	t.Run("相同文本向量相等", func(t *testing.T) {
		v1 := TextToVector("测试")
		v2 := TextToVector("测试")
		if len(v1) != len(v2) {
			t.Fatalf("长度不一致: %d vs %d", len(v1), len(v2))
		}
		for i := range v1 {
			if v1[i] != v2[i] {
				t.Errorf("v1[%d]=%v != v2[%d]=%v", i, v1[i], i, v2[i])
				return
			}
		}
	})

	// 空文本向量应为全零
	t.Run("空文本全零", func(t *testing.T) {
		vec := TextToVector("")
		for i, v := range vec {
			if v != 0 {
				t.Errorf("空文本向量[%d] = %v, want 0", i, v)
				return
			}
		}
	})
}

// TestCosineSimilarity_EdgeCases 余弦相似度边界情况测试。
// 验证零向量、自相似、维度不匹配、正交向量。
func TestCosineSimilarity_EdgeCases(t *testing.T) {
	t.Run("全零向量不除零", func(t *testing.T) {
		zero := make([]float32, VectorDim)
		// 不应 panic，应返回 0
		got := CosineSimilarity(zero, zero)
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("CosineSimilarity(零,零) = %v, 不应是 NaN/Inf", got)
		}
		if got != 0 {
			t.Errorf("CosineSimilarity(零,零) = %v, want 0", got)
		}
	})

	t.Run("自相似约为1", func(t *testing.T) {
		v := TextToVector("加密视频文件处理流程")
		got := CosineSimilarity(v, v)
		if math.Abs(got-1.0) > 0.001 {
			t.Errorf("CosineSimilarity(v,v) = %.6f, want ~1.0", got)
		}
	})

	t.Run("维度不同返回0", func(t *testing.T) {
		v1 := []float32{1.0, 0.0, 0.0}
		v2 := []float32{1.0, 0.0}
		got := CosineSimilarity(v1, v2)
		if got != 0 {
			t.Errorf("CosineSimilarity(维度不同) = %v, want 0", got)
		}
	})

	t.Run("正交向量为0", func(t *testing.T) {
		// e1 与 e2 正交
		v1 := []float32{1.0, 0.0, 0.0, 0.0}
		v2 := []float32{0.0, 1.0, 0.0, 0.0}
		got := CosineSimilarity(v1, v2)
		if math.Abs(got) > 1e-9 {
			t.Errorf("CosineSimilarity(正交) = %v, want 0", got)
		}
	})
}

// TestBuildQueryVector_EmptyAndNormal 查询向量的空值与正常情况测试。
func TestBuildQueryVector_EmptyAndNormal(t *testing.T) {
	t.Run("空查询全零", func(t *testing.T) {
		vec := BuildQueryVector("")
		if len(vec) != VectorDim {
			t.Fatalf("BuildQueryVector(\"\") dim = %d, want %d", len(vec), VectorDim)
		}
		for i, v := range vec {
			if v != 0 {
				t.Errorf("空查询向量[%d] = %v, want 0", i, v)
				return
			}
		}
	})

	t.Run("非空查询非全零", func(t *testing.T) {
		vec := BuildQueryVector("测试")
		if len(vec) != VectorDim {
			t.Fatalf("BuildQueryVector(\"测试\") dim = %d, want %d", len(vec), VectorDim)
		}
		nonZero := false
		for _, v := range vec {
			if v != 0 {
				nonZero = true
				break
			}
		}
		if !nonZero {
			t.Error("BuildQueryVector(\"测试\") 应有非零分量")
		}
	})

	t.Run("查询向量与文档向量正相似", func(t *testing.T) {
		// 查询向量与同文本文档向量应有正相似度（共享 token 哈希桶）
		docVec := TextToVector("测试关键词")
		queryVec := BuildQueryVector("测试关键词")
		sim := CosineSimilarity(docVec, queryVec)
		if sim <= 0 {
			t.Errorf("查询向量与文档向量相似度 = %.6f, 应 > 0", sim)
		}
	})
}

// TestEncodeVector_Roundtrip 向量编码往返测试。
// 验证编码后字节长度正确，且解码可还原原始向量。
func TestEncodeVector_Roundtrip(t *testing.T) {
	t.Run("全零向量往返", func(t *testing.T) {
		zero := make([]float32, VectorDim)
		encoded := EncodeVector(zero)
		if len(encoded) != VectorDim*4 {
			t.Fatalf("encoded len = %d, want %d", len(encoded), VectorDim*4)
		}
		// 字节应全为 0
		for i, b := range encoded {
			if b != 0 {
				t.Errorf("全零向量编码[%d] = %d, want 0", i, b)
				return
			}
		}
		// 解码回来仍全零
		decoded := decodeVector(encoded)
		if len(decoded) != VectorDim {
			t.Fatalf("decoded dim = %d, want %d", len(decoded), VectorDim)
		}
		for i, v := range decoded {
			if v != 0 {
				t.Errorf("解码[%d] = %v, want 0", i, v)
				return
			}
		}
	})

	t.Run("真实向量往返", func(t *testing.T) {
		vec := TextToVector("测试向量编码解码")
		encoded := EncodeVector(vec)
		if len(encoded) != VectorDim*4 {
			t.Fatalf("encoded len = %d, want %d", len(encoded), VectorDim*4)
		}
		// 解码后应与原向量一致
		decoded := decodeVector(encoded)
		if len(decoded) != len(vec) {
			t.Fatalf("decoded dim = %d, want %d", len(decoded), len(vec))
		}
		for i, v := range vec {
			if decoded[i] != v {
				t.Errorf("解码[%d] = %v, want %v", i, decoded[i], v)
				return
			}
		}
	})
}
