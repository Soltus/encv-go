package server

import (
	"net/http"
	"testing"
)

func TestLatencyCategoryFromMs(t *testing.T) {
	tests := []struct {
		name     string
		ms       int64
		expected string
	}{
		{"fast_0ms", 0, "fast"},
		{"fast_49ms", 49, "fast"},
		{"normal_50ms", 50, "normal"},
		{"normal_199ms", 199, "normal"},
		{"slow_200ms", 200, "slow"},
		{"slow_999ms", 999, "slow"},
		{"very_slow_1000ms", 1000, "very-slow"},
		{"very_slow_5000ms", 5000, "very-slow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := latencyCategoryFromMs(tt.ms)
			if got != tt.expected {
				t.Errorf("latencyCategoryFromMs(%d) = %q, want %q", tt.ms, got, tt.expected)
			}
		})
	}
}

func TestClientIPFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*http.Request)
		remote   string
		expected string
	}{
		{
			name: "x_forwarded_for_single",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "192.168.1.1")
			},
			remote:   "10.0.0.1:12345",
			expected: "192.168.1.1",
		},
		{
			name: "x_forwarded_for_multiple",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1, 172.16.0.1")
			},
			remote:   "10.0.0.1:12345",
			expected: "192.168.1.1",
		},
		{
			name: "x_forwarded_for_with_spaces",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "  192.168.1.1  ")
			},
			remote:   "10.0.0.1:12345",
			expected: "192.168.1.1",
		},
		{
			name: "x_real_ip",
			setup: func(r *http.Request) {
				r.Header.Set("X-Real-IP", "10.0.0.5")
			},
			remote:   "10.0.0.1:12345",
			expected: "10.0.0.5",
		},
		{
			name:     "remote_addr_only",
			setup:    func(r *http.Request) {},
			remote:   "192.168.1.100:54321",
			expected: "192.168.1.100",
		},
		{
			name:     "remote_addr_no_port",
			setup:    func(r *http.Request) {},
			remote:   "192.168.1.100",
			expected: "192.168.1.100",
		},
		{
			name: "x_forwarded_for_overrides_x_real_ip",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "1.2.3.4")
				r.Header.Set("X-Real-IP", "5.6.7.8")
			},
			remote:   "9.10.11.12:1234",
			expected: "1.2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/test", nil)
			if err != nil {
				t.Fatal(err)
			}
			tt.setup(req)
			req.RemoteAddr = tt.remote

			got := clientIPFromRequest(req)
			if got != tt.expected {
				t.Errorf("clientIPFromRequest() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCheckWebdavRouteConflict(t *testing.T) {
	tests := []struct {
		name     string
		root     string
		expected string
	}{
		{"empty_root", "", "<root>"},
		{"no_conflict_normal", "/files", ""},
		{"conflict_api_prefix", "/api", "/api/"},
		{"conflict_api_subpath", "/api/v1", "/api/"},
		{"conflict_admin", "/admin", "/admin"},
		{"conflict_login", "/login", "/login"},
		{"conflict_p_prefix", "/p", "/p"},
		{"conflict_p_subpath", "/ping", "/p"},
		{"conflict_health", "/health", "/health"},
		{"conflict_webdav_prefix", "/webdav", ""},
		{"root_with_trailing_slash", "/api/", "/api/"},
		{"root_with_spaces", "  /api  ", "/api/"},
		{"conflict_ws", "/ws", "/ws"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkWebdavRouteConflict(tt.root)
			if got != tt.expected {
				t.Errorf("checkWebdavRouteConflict(%q) = %q, want %q", tt.root, got, tt.expected)
			}
		})
	}
}
