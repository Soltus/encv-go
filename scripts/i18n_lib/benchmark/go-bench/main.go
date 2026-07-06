package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

func main() {
	dir := "/tmp/i18n-formats-5k"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	fmt.Println()
	fmt.Println("🔬 Go 格式解析性能测试")
	fmt.Println("============================================================")
	fmt.Printf("目录: %s\n", dir)
	fmt.Println()

	type result struct {
		name string
		ms   float64
	}
	var results []result

	bench := func(name string, fn func() map[string]map[string]string) {
		for i := 0; i < 3; i++ {
			fn()
		}
		times := make([]float64, 5)
		for i := 0; i < 5; i++ {
			t0 := time.Now()
			r := fn()
			t1 := time.Now()
			times[i] = float64(t1.Sub(t0).Microseconds()) / 1000.0
			if i == 0 {
				firstLang := ""
				for k := range r {
					firstLang = k
					break
				}
				keyCount := len(r[firstLang])
				langCount := len(r)
				total := langCount * keyCount
				fmt.Printf("  %s: %d 语言 × %d keys = %d 条目\n", name, langCount, keyCount, total)
			}
		}
		sort.Float64s(times)
		median := times[len(times)/2]
		results = append(results, result{name, median})
		fmt.Printf("  耗时: %.0fms (中位)\n", median)
		fmt.Println()
	}

	jsonPath := dir + "/dict.json"
	jsonBytes, _ := os.ReadFile(jsonPath)
	fmt.Printf("📄 JSON: %d KB\n", len(jsonBytes)/1024)
	bench("JSON (encoding/json)", func() map[string]map[string]string {
		var m map[string]map[string]string
		json.Unmarshal(jsonBytes, &m)
		return m
	})

	jsoncPath := dir + "/dict.jsonc"
	jsoncBytes, _ := os.ReadFile(jsoncPath)
	fmt.Printf("📄 JSONC: %d KB\n", len(jsoncBytes)/1024)
	bench("JSONC (去注释+json)", func() map[string]map[string]string {
		stripped := stripJSONC(string(jsoncBytes))
		var m map[string]map[string]string
		json.Unmarshal([]byte(stripped), &m)
		return m
	})

	fmt.Println("============================================================")
	fmt.Println("📊 性能汇总（从快到慢）")
	fmt.Println("============================================================")
	sort.Slice(results, func(i, j int) bool { return results[i].ms < results[j].ms })
	fastest := results[0].ms

	fmt.Println()
	fmt.Printf("  %-24s %8s %10s   %s\n", "格式", "耗时", "速度比", "注释")
	fmt.Printf("  %s %s %s   %s\n", dash(24), dash(8), dash(10), dash(8))

	comments := map[string]string{
		"JSON (encoding/json)": "❌ 无",
		"JSONC (去注释+json)":  "✅ 有",
	}

	for _, r := range results {
		ratio := fmt.Sprintf("%.1fx", r.ms/fastest)
		msStr := fmt.Sprintf("%.0fms", r.ms)
		c := comments[r.name]
		fmt.Printf("  %-24s %8s %10s   %s\n", r.name, msStr, ratio, c)
	}

	fmt.Println()
	fmt.Printf("💡 最快: %s (%.0fms)\n", results[0].name, results[0].ms)
	fmt.Printf("💡 最慢: %s (%.0fms)\n", results[len(results)-1].name, results[len(results)-1].ms)
	fmt.Printf("💡 差距: %.1fx\n", results[len(results)-1].ms/results[0].ms)

	fmt.Println()
	fmt.Println("============================================================")
}

func dash(n int) string {
	s := make([]byte, n)
	for i := range s {
		s[i] = '-'
	}
	return string(s)
}

func stripJSONC(text string) string {
	result := make([]byte, 0, len(text))
	inString := false
	stringChar := byte(0)
	i := 0
	n := len(text)

	for i < n {
		c := text[i]
		if inString {
			result = append(result, c)
			if c == '\\' && i+1 < n {
				result = append(result, text[i+1])
				i += 2
				continue
			}
			if c == stringChar {
				inString = false
			}
			i++
		} else {
			if c == '"' || c == '\'' {
				inString = true
				stringChar = c
				result = append(result, c)
				i++
			} else if c == '/' && i+1 < n && text[i+1] == '/' {
				for i < n && text[i] != '\n' {
					i++
				}
			} else if c == '/' && i+1 < n && text[i+1] == '*' {
				i += 2
				for i < n-1 && !(text[i] == '*' && text[i+1] == '/') {
					i++
				}
				i += 2
			} else {
				result = append(result, c)
				i++
			}
		}
	}

	s := string(result)
	var cleaned []byte
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				i = j - 1
				continue
			}
		}
		cleaned = append(cleaned, s[i])
	}
	return string(cleaned)
}
