package server

import "testing"

func TestBuildChatCompletionsURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect string
	}{
		// 关键 case: 用户填的 base_url 已经含 /v1 → 不能重复拼接
		{"openai-with-v1", "https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
		{"openai-with-v1-trailing-slash", "https://api.openai.com/v1/", "https://api.openai.com/v1/chat/completions"},
		{"openai-without-v1", "https://api.openai.com", "https://api.openai.com/v1/chat/completions"},
		{"openai-uppercase-v1", "https://api.openai.com/V1", "https://api.openai.com/v1/chat/completions"},
		// 代理路径
		{"proxy-with-v1", "https://proxy.example.com/openai/v1", "https://proxy.example.com/openai/v1/chat/completions"},
		{"proxy-without-v1", "https://proxy.example.com/openai", "https://proxy.example.com/openai/v1/chat/completions"},
		// 边界 case
		{"empty", "", "/v1/chat/completions"},
		{"only-slash", "/", "/v1/chat/completions"},
		{"trailing-slash", "https://api.openai.com/", "https://api.openai.com/v1/chat/completions"},
		// 关键：不能误伤 /v1beta /v2 等其他路径
		{"v1beta", "https://api.example.com/v1beta", "https://api.example.com/v1beta/v1/chat/completions"},
		{"v2", "https://api.example.com/v2", "https://api.example.com/v2/v1/chat/completions"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildChatCompletionsURL(c.input)
			if got != c.expect {
				t.Errorf("input=%q\n  got:  %s\n  want: %s", c.input, got, c.expect)
			}
		})
	}
}
