// Stage 8 (borrow-nuclear-boy-2026q2)：autoExtractMemories 从用户消息中提取偏好与项目信息。
//
// 借鉴自 /tmp/nuclear-boy/memory/.../MemoryStore.kt L673-741。
//
// 三种 pattern：
//   1. 用户偏好：    "我(喜欢|习惯|常用|偏好) X"  → preferred_language / style
//   2. 技术栈：      "我(用|的|写) X, Y, Z"       → tech_stack
//   3. build 命令：  "我(常用|总是|通常) npm"     → build_command
//
// 落地策略：
//   - 每 5 轮对话跑一次 autoExtract（频次可调）
//   - 用正则匹配（nuclear-boy 模式）
//   - 提取结果存为 UserProfile（confidence=0.7），由 prompt builder 注入 system prompt
package memory

import (
	"regexp"
	"strings"
)

// AutoExtractConfidence 提取时的默认 confidence（nuclear-boy L673-741）。
const AutoExtractConfidence = 0.7

// AutoExtractSource 提取来源标识。
const AutoExtractSource = "auto_extracted"

// 三个正则 pattern（nuclear-boy MemoryStore.kt L673-741）
var (
	// 用户偏好：先识别"我(喜欢|习惯|常用|偏好)"，再从剩余文本里挑出第一个
	// 长度 ≥ 2 的英文/中文 token 作为 value（避免把 "用/的/写" 误当 value）。
	patternUserPreferenceKeyword = regexp.MustCompile(`我(喜欢|习惯|常用|偏好)`)
	// 候选 value token（英文/中文，长度 2-20，避免 1 字助词）
	// 必须以字母/中文开头（避免 "npm"/"go" 等 2 字母 build tool 误入）
	// → 改为：英文必须 ≥ 3 字符 OR 在已知 value 字典中。
	patternValueToken = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+#.\-]{2,19}|[a-zA-Z]{2}|[一-龥]{2,20}`)
	// 技术栈
	patternTechStack = regexp.MustCompile(`我(用|的|写)\s*([a-zA-Z][a-zA-Z0-9+#.\-]*(?:\s*[,，]\s*[a-zA-Z][a-zA-Z0-9+#.\-]*){0,4})`)
	// build 命令
	patternBuildCommand = regexp.MustCompile(`我(常用|总是|通常)\s*(?:用\s*)?(npm|pnpm|yarn|gradle|maven|go|cargo|pip|uv|poetry|bun)`)
)

// AutoExtract 从用户消息中提取偏好，返回 UserProfile 列表。
//
// 借鉴 nuclear-boy MemoryStore.kt L673-741 extractMemoriesFromMessage。
func AutoExtract(message string) []UserProfile {
	var out []UserProfile
	out = append(out, extractUserPreference(message)...)
	out = append(out, extractTechStack(message)...)
	out = append(out, extractBuildCommand(message)...)
	return out
}

// extractUserPreference 提取"我习惯/喜欢/常用/偏好 X"。
func extractUserPreference(msg string) []UserProfile {
	// 先找关键词位置
	kwIdx := patternUserPreferenceKeyword.FindStringIndex(msg)
	if kwIdx == nil {
		return nil
	}
	// 已知 build tool 列表（避免被 user preference 误捕）
	buildTools := map[string]bool{
		"npm": true, "pnpm": true, "yarn": true, "bun": true,
		"gradle": true, "maven": true, "go": true, "cargo": true,
		"pip": true, "uv": true, "poetry": true,
	}
	// 关键词后从下一个字符开始找 value
	rest := msg[kwIdx[1]:]
	tokIdx := patternValueToken.FindStringIndex(rest)
	if tokIdx == nil {
		return nil
	}
	value := rest[tokIdx[0]:tokIdx[1]]
	if buildTools[strings.ToLower(value)] {
		return nil
	}
	// 简单分类
	keyword := msg[kwIdx[0]:kwIdx[1]]
	key := preferenceKeyFromValue(keyword, value)
	return []UserProfile{{
		Key:        key,
		Value:      value,
		Confidence: AutoExtractConfidence,
		Source:     AutoExtractSource,
	}}
}

// preferenceKeyFromValue 根据值猜 key。
func preferenceKeyFromValue(keyword, value string) string {
	// 简化：基于 value 特征分类
	lower := strings.ToLower(value)
	switch {
	case isLanguage(lower):
		return "preferred_language"
	case isFramework(lower):
		return "preferred_framework"
	case isEditor(lower):
		return "preferred_editor"
	case isStyle(lower):
		return "code_style"
	default:
		return "user_preference"
	}
}

func isLanguage(s string) bool {
	langs := []string{"typescript", "javascript", "python", "go", "rust", "java", "kotlin", "swift", "ruby", "php", "c++", "c#", "scala"}
	for _, l := range langs {
		if s == l {
			return true
		}
	}
	return false
}

func isFramework(s string) bool {
	frameworks := []string{"react", "vue", "angular", "svelte", "next", "nuxt", "express", "fastapi", "django", "flask", "gin", "echo", "spring", "flutter"}
	for _, f := range frameworks {
		if s == f {
			return true
		}
	}
	return false
}

func isEditor(s string) bool {
	editors := []string{"vscode", "vim", "emacs", "intellij", "sublime", "atom", "xcode", "android studio"}
	for _, e := range editors {
		if s == e {
			return true
		}
	}
	return false
}

func isStyle(s string) bool {
	styles := []string{"tabs", "spaces", "semicolons", "arrow", "function", "tabs缩进", "空格缩进", "箭头函数", "分号", "无分号", "单引号", "双引号"}
	for _, st := range styles {
		if s == st {
			return true
		}
	}
	return false
}

// extractTechStack 提取"我用 X, Y" → tech_stack。
func extractTechStack(msg string) []UserProfile {
	// 简化版：直接找 "我用/的/写 X" 模式
	re := regexp.MustCompile(`我(用|的|写)\s*([a-zA-Z][a-zA-Z0-9+#.\-]*(?:\s*[,，]\s*[a-zA-Z][a-zA-Z0-9+#.\-]*){0,4})`)
	matches := re.FindAllStringSubmatch(msg, -1)
	var out []UserProfile
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		stack := strings.TrimSpace(m[2])
		// 拆 token
		parts := splitStack(stack)
		if len(parts) == 0 {
			continue
		}
		out = append(out, UserProfile{
			Key:        "tech_stack",
			Value:      strings.Join(parts, ","),
			Confidence: AutoExtractConfidence,
			Source:     AutoExtractSource,
		})
	}
	return out
}

func splitStack(s string) []string {
	// 按 , ， 分割
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, "，", " ")
	fields := strings.Fields(s)
	return fields
}

// extractBuildCommand 提取"我常用 npm/pnpm/..."。
func extractBuildCommand(msg string) []UserProfile {
	matches := patternBuildCommand.FindAllStringSubmatch(msg, -1)
	var out []UserProfile
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		out = append(out, UserProfile{
			Key:        "build_command",
			Value:      strings.ToLower(m[2]),
			Confidence: AutoExtractConfidence,
			Source:     AutoExtractSource,
		})
	}
	return out
}

// AutoExtractAndStore 从消息提取并存入 store。
// 每 5 轮调用一次（频次可调）。
func AutoExtractAndStore(s *Store, message string) int {
	profiles := AutoExtract(message)
	for _, p := range profiles {
		s.SaveProfile(p)
	}
	return len(profiles)
}
