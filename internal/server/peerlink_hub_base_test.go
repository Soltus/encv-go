package server

// peerlink_hub_base_test.go —— Hub 基址归一化回归锁（2026-10-03 扫码配对 404 真 bug）
//
// 背景：票据里的 `hub` **带路径**（`https://host/api/peerlink`），而本端拼远端接口时
// 会再拼 `/api/peerlink/...` ⇒ 双前缀 ⇒ 404 ⇒ 扫码配对在真机上直接失败。
// `hubBaseURL` 必须把已带的 peerlink 前缀剥掉，同时**保留**其它路径/端口。

import "testing"

func TestHubBaseURL_StripsPeerlinkPrefixOnce(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"票据里的带路径 hub", "https://hub.example.com/api/peerlink", "https://hub.example.com"},
		{"带尾斜杠", "https://hub.example.com/api/peerlink/", "https://hub.example.com"},
		{"回环带端口", "http://127.0.0.1:2025/api/peerlink", "http://127.0.0.1:2025"},
		{"已经是基址", "https://hub.example.com", "https://hub.example.com"},
		{"基址带尾斜杠", "https://hub.example.com/", "https://hub.example.com"},
		{"带端口不带路径", "http://localhost:2025", "http://localhost:2025"},
		{"其它路径要保留", "https://gw.example.com/encv/api/peerlink", "https://gw.example.com/encv"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hubBaseURL(c.in); got != c.want {
				t.Fatalf("hubBaseURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// 归一化后再拼接口路径，必须正好一个 peerlink 前缀（双前缀 = 404）
func TestHubBaseURL_PairEndpointHasSinglePrefix(t *testing.T) {
	got := hubBaseURL("https://hub.example.com/api/peerlink") + "/api/peerlink/pair"
	want := "https://hub.example.com/api/peerlink/pair"
	if got != want {
		t.Fatalf("拼出来的配对地址不对: got %q, want %q", got, want)
	}
}
