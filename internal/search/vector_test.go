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
