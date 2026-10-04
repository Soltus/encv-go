package server

// peerlink_hub_url_r3_test.go —— P2c Task 2.10（**R3**）回归锁
//
// 契约：配置 / 二维码里的**跨端** Hub 地址**禁止明文 http**（只有 https，或本机回环用于自测）。
// 理由（spec §0）：手机端在 NAT 后主动出网到 Hub，明文 http = 凭据与业务报文在公网裸奔。
// 这条同时钉住"二维码内容不含内网明文地址"这条红线。

import "testing"

func TestValidateHubURL_RejectsPlaintextHTTPExceptLoopback(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"https 公网", "https://hub.example.com/api/peerlink", true},
		{"https 带端口", "https://hub.example.com:8443/api/peerlink", true},
		{"回环 http（沙箱自测）", "http://127.0.0.1:2025/api/peerlink", true},
		{"localhost http", "http://localhost:2025/api/peerlink", true},
		{"IPv6 回环 http", "http://[::1]:2025/api/peerlink", true},
		// ↓↓↓ R3 红线
		{"明文 http 公网", "http://hub.example.com/api/peerlink", false},
		{"明文 http LAN（内网地址诱惑）", "http://192.168.1.50:2025/api/peerlink", false},
		{"明文 http 10.x", "http://10.0.0.5:2025", false},
		{"非 http(s) scheme", "ws://hub.example.com/api/peerlink", false},
		{"缺少 host", "https://", false},
		{"空串", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := validateHubURL(c.in)
			if c.ok {
				if err != nil {
					t.Fatalf("应接受 %q，实际 err=%v", c.in, err)
				}
				if got == "" {
					t.Fatalf("应返回规范化地址，got 空串")
				}
				return
			}
			if err == nil {
				t.Fatalf("R3 红线：不得接受 %q（got=%q）", c.in, got)
			}
		})
	}
}

// 尾斜杠要被规范化掉：Hub 地址会参与 URL 拼接，多余斜杠会拼出 //api/...
func TestValidateHubURL_TrimsTrailingSlash(t *testing.T) {
	got, err := validateHubURL("https://hub.example.com/api/peerlink/")
	if err != nil {
		t.Fatalf("https 地址应被接受: %v", err)
	}
	if got != "https://hub.example.com/api/peerlink" {
		t.Fatalf("尾斜杠应被去掉, got %q", got)
	}
}
