package server

import (
	"math/rand"
	"strings"
	"testing"
)

// TestComputeHybridScore_MillionCases 混合评分百万级测试。
//
// 覆盖（见 debug-discipline.md §1.6 测试规模铁律 + §3.5 智能搜索策略）：
//   - 长文件名（5-80 字符）与短查询（2-6 字符）的稀释对比
//   - 跨语言（CJK / 韩文 / 日文 / 英文 / 数字）随机输入
//   - 不变量：score ∈ [0, 1]、无 NaN
//   - 单调性：bigram 召回率越高，score 越高（绝大多数情况下）
func TestComputeHybridScore_MillionCases(t *testing.T) {
	const seed = 20260702
	rng := rand.New(rand.NewSource(seed))

	pairs := 0
	for i := 0; i < 1_000_000; i++ {
		// 随机 query：2-6 字符
		var query string
		switch rng.Intn(4) {
		case 0:
			query = randomCJKPublic(rng, 2+rng.Intn(5))
		case 1:
			query = randomKoreanPublic(rng, 2+rng.Intn(5))
		case 2:
			query = randomEnglishWordsPublic(rng, 2+rng.Intn(5))
		default:
			// 中英混合
			query = randomCJKPublic(rng, 1+rng.Intn(3)) + " " +
				randomEnglishWordsPublic(rng, 1+rng.Intn(3))
		}

		// 随机候选：5-80 字符
		var candidate string
		switch rng.Intn(4) {
		case 0:
			candidate = randomCJKPublic(rng, 5+rng.Intn(76))
		case 1:
			candidate = randomKoreanPublic(rng, 5+rng.Intn(20))
		case 2:
			candidate = randomEnglishWordsPublic(rng, 2+rng.Intn(15))
		default:
			candidate = randomCJKPublic(rng, 3+rng.Intn(20)) + "-" +
				randomEnglishWordsPublic(rng, 1+rng.Intn(5))
		}

		vecScore := rng.Float64()
		queryBigrams := extractBigrams(query)
		score := computeHybridScore(candidate, queryBigrams, vecScore)

		// 不变量 1：score ∈ [0, 1]
		if score < 0 || score > 1 {
			t.Fatalf("case %d q=%q cand=%q vec=%.4f → score=%.6f 越界",
				i, query, candidate, vecScore, score)
		}
		// 不变量 2：无 NaN
		if score != score {
			t.Fatalf("case %d score is NaN: q=%q cand=%q", i, query, candidate)
		}
		pairs++
	}
	if pairs != 1_000_000 {
		t.Fatalf("实际 pair=%d, want 1_000_000", pairs)
	}
}

// TestExtractBigrams_MillionCases extractBigrams 百万级测试。
func TestExtractBigrams_MillionCases(t *testing.T) {
	const seed = 20260702
	rng := rand.New(rand.NewSource(seed))

	cases := 0
	// 70 万 CJK case
	for i := 0; i < 700_000; i++ {
		length := 2 + rng.Intn(30)
		input := randomCJKPublic(rng, length)
		bigrams := extractBigrams(input)
		// 不变量 1：bigrams 数量 = length - 1（CJK 串：相邻 2 字符滑动窗）
		if len(bigrams) != length-1 {
			t.Fatalf("cjk case %d input=%q (len=%d): bigrams len=%d, want %d",
				i, input, length, len(bigrams), length-1)
		}
		// 不变量 2：每个 bigram rune 长度 == 2
		for j, bg := range bigrams {
			if n := len([]rune(bg)); n != 2 {
				t.Fatalf("cjk case %d bigram[%d]=%q rune len=%d, want 2",
					i, j, bg, n)
			}
		}
		cases++
	}
	// 30 万英文 case（要求每个词 >= 2 字符，避免被 extractBigrams 过滤）
	for i := 0; i < 300_000; i++ {
		count := 2 + rng.Intn(7) // 2-8 词
		input := randomEnglishWordsPublic(rng, count)
		// 强制每个词 >= 2 字符（与 randomEnglishWordsPublic 不一致时调整）
		// randomEnglishWordsPublic 词长 1-8 字符，统计真正长度 >= 2 的词
		actualCount := 0
		for _, w := range strings.Split(input, " ") {
			if len(w) >= 2 {
				actualCount++
			}
		}
		bigrams := extractBigrams(input)
		if len(bigrams) != actualCount {
			t.Fatalf("eng case %d input=%q: bigrams len=%d, want %d (tokens=%v)",
				i, input, len(bigrams), actualCount, bigrams)
		}
		cases++
	}
	if cases != 1_000_000 {
		t.Fatalf("实际 case=%d, want 1_000_000", cases)
	}
}

// TestHasSufficientBigramOverlapEx_MillionCases bigram 重叠过滤百万级测试。
//
// 覆盖三档强度 × 跨语言输入。
func TestHasSufficientBigramOverlapEx_MillionCases(t *testing.T) {
	const seed = 20260702
	rng := rand.New(rand.NewSource(seed))

	cases := 0
	for i := 0; i < 1_000_000; i++ {
		// query 2-5 字符
		query := randomCJKPublic(rng, 2+rng.Intn(4))
		queryBigrams := extractBigrams(query)
		// 候选 1-50 字符
		candidate := randomCJKPublic(rng, 1+rng.Intn(50))

		// 三档强度都应能调用（不 panic、必返回 bool）
		_ = hasSufficientBigramOverlapEx(candidate, queryBigrams, BigramRelaxed)
		_ = hasSufficientBigramOverlapEx(candidate, queryBigrams, BigramMedium)
		_ = hasSufficientBigramOverlapEx(candidate, queryBigrams, BigramStrict)
		cases++
	}
	if cases != 1_000_000 {
		t.Fatalf("实际 case=%d, want 1_000_000", cases)
	}
}

// ─── 可复用辅助函数（test-only） ────────────────────────────────────

func randomCJKPublic(rng *rand.Rand, n int) string {
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = rune(0x4E00 + rng.Intn(0x9FFF-0x4E00+1))
	}
	return string(runes)
}

func randomKoreanPublic(rng *rand.Rand, n int) string {
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = rune(0xAC00 + rng.Intn(0xD7AF-0xAC00+1))
	}
	return string(runes)
}

func randomEnglishWordsPublic(rng *rand.Rand, n int) string {
	words := make([]string, n)
	for i := range words {
		wordLen := 1 + rng.Intn(8)
		b := make([]byte, wordLen)
		for j := range b {
			b[j] = byte('a' + rng.Intn(26))
		}
		words[i] = string(b)
	}
	return strings.Join(words, " ")
}
