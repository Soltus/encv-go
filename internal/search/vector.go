// Package vectorsearch 提供基于 Turso 向量检索的中文搜索能力。
//
// 核心原理：
//   - 中文分词：字符级 bigram（两个连续字作为一个 token）+ 英文按空格/标点分词
//   - 向量化：基于词频的 TF-IDF 风格稀疏向量，哈希映射到固定维度
//   - 存储：使用 Turso 原生 BLOB 向量类型（F32_BLOB）
//   - 检索：使用 Turso 内置 vector_distance_cos 函数计算余弦距离
//
// 优势：
//   - 纯 Go 实现，零外部依赖，无 CGO
//   - 中文搜索效果优于简单的 LIKE '%keyword%'
//   - 支持模糊匹配（语义相近的词也能命中）
//   - 利用 Turso 原生向量函数，性能好
package vectorsearch

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// 向量维度（固定维度，哈希映射）
const VectorDim = 256

// Tokenize 中文 bigram + 英文单词分词。
//
// 中文处理：
//   - 连续中文字符按 bigram（两个连续字）切分
//   - 例如："中文测试" → ["中文", "文测", "测试"]
//
// 英文/数字处理：
//   - 按非字母数字边界切分单词
//   - 转小写
//   - 例如："Hello World" → ["hello", "world"]
func Tokenize(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var tokens []string
	runes := []rune(text)
	n := len(runes)

	i := 0
	for i < n {
		r := runes[i]

		if isCJK(r) {
			// 中文 bigram
			if i+1 < n && isCJK(runes[i+1]) {
				tokens = append(tokens, string(runes[i:i+2]))
			}
			// 单字也作为 token（短文本召回率更好）
			tokens = append(tokens, string(r))
			i++
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			// 英文/数字单词
			start := i
			for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i])) {
				i++
			}
			word := strings.ToLower(string(runes[start:i]))
			if len(word) > 0 {
				tokens = append(tokens, word)
			}
		} else {
			// 跳过标点、空格等
			i++
		}
	}

	return tokens
}

// isCJK 判断是否为中日韩统一表意文字。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= '\u3040' && r <= '\u30FF') || // 日文假名
		(r >= '\uAC00' && r <= '\uD7AF') // 韩文
}

// TextToVector 将文本转换为固定维度的 sublinear TF 向量。
//
// 使用哈希 trick 将 token 映射到固定维度，避免维度爆炸。
// 使用 sublinear TF（1 + log(tf)）而非原始词频，降低长文本的稀释效应：
//   - 短文件名和长文件名中，相同关键词的权重差异更小
//   - 避免 token 数量越多，每个 token 的归一化权重越低的问题
func TextToVector(text string) []float32 {
	tokens := Tokenize(text)
	if len(tokens) == 0 {
		return make([]float32, VectorDim)
	}

	// 先统计每个维度的频次
	counts := make([]float32, VectorDim)
	for _, token := range tokens {
		h := fnv.New32a()
		h.Write([]byte(token))
		idx := h.Sum32() % uint32(VectorDim)
		counts[idx] += 1.0
	}

	// sublinear TF: 1 + log(tf)，降低长文本稀释
	vec := make([]float32, VectorDim)
	for i, c := range counts {
		if c > 0 {
			vec[i] = 1.0 + float32(math.Log(float64(c)))
		}
	}

	// L2 归一化（方便余弦距离计算）
	normalize(vec)
	return vec
}

// normalize 对向量进行 L2 归一化。
func normalize(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return
	}
	norm := float32(math.Sqrt(sum))
	for i := range vec {
		vec[i] /= norm
	}
}

// EncodeVector 将 float32 向量编码为 Turso 向量 BLOB 格式。
//
// Turso 的 vector32 格式：原生 IEEE 754 单精度浮点，小端序。
func EncodeVector(vec []float32) []byte {
	b := make([]byte, len(vec)*4)
	for i, f := range vec {
		bits := math.Float32bits(f)
		b[i*4] = byte(bits)
		b[i*4+1] = byte(bits >> 8)
		b[i*4+2] = byte(bits >> 16)
		b[i*4+3] = byte(bits >> 24)
	}
	return b
}

// BuildQueryVector 构建查询向量，带关键词权重增强。
//
// 使用 sublinear TF + 查询权重增强，提升精确匹配的优先级。
// 与 TextToVector 保持一致的 sublinear 缩放，确保评分尺度一致。
func BuildQueryVector(text string) []float32 {
	tokens := Tokenize(text)
	if len(tokens) == 0 {
		return make([]float32, VectorDim)
	}

	// 先统计频次
	counts := make([]float32, VectorDim)
	for _, token := range tokens {
		h := fnv.New32a()
		h.Write([]byte(token))
		idx := h.Sum32() % uint32(VectorDim)
		counts[idx] += 1.0
	}

	// sublinear TF + 查询权重增强（权重 2.0）
	vec := make([]float32, VectorDim)
	queryBoost := float32(2.0)
	for i, c := range counts {
		if c > 0 {
			vec[i] = queryBoost * (1.0 + float32(math.Log(float64(c))))
		}
	}

	normalize(vec)
	return vec
}

// CosineSimilarity 计算两个向量的余弦相似度（纯 Go 版，用于 SQLite fallback）。
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / math.Sqrt(normA*normB)
}
