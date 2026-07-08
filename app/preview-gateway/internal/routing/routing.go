package routing

import (
	"net/url"
	"os"
	"strings"
)

type Upstream struct {
	Match       string
	Target      string
	WsTarget    string
	Name        string
	Hint        string
	Required    bool
	PathRewrite func(string) string
}

type ViteDenyRule struct {
	Match string
	Mode  string // "exact" or "prefix"
	Why   string
}

func getDefaultFrontend() string {
	v := os.Getenv("DEFAULT_FRONTEND")
	if v == "simverse" || v == "encv-mobile" {
		return v
	}
	return "encv-mobile"
}

func GetViteUpstream() *Upstream {
	if getDefaultFrontend() == "simverse" {
		return &Upstream{
			Target:   "http://127.0.0.1:5176",
			WsTarget: "ws://127.0.0.1:5176",
			Name:     "plugin-simverse-web",
			Hint:     "Check pm2 status for plugin-simverse-vite (:5176)",
			Required: true,
		}
	}
	return &Upstream{
		Target:   "http://127.0.0.1:8100",
		WsTarget: "ws://127.0.0.1:8100",
		Name:     "encv-mobile-vite",
		Hint:     "Check pm2 status for start-preview (encv-mobile vite :8100)",
		Required: true,
	}
}

var SpecialUpstreams = []*Upstream{
	{
		Match:    "/encv-mobile",
		Target:   "http://127.0.0.1:8100",
		WsTarget: "ws://127.0.0.1:8100",
		Name:     "encv-mobile-vite",
		Hint:     "Check pm2 status for start-preview (encv-mobile vite :8100)",
		Required: false,
		PathRewrite: func(p string) string {
			r := strings.TrimPrefix(p, "/encv-mobile")
			if r == "" {
				return "/"
			}
			if strings.HasPrefix(r, "/") {
				return r
			}
			return "/" + r
		},
	},
	{
		Match:    "/openlist-ui",
		Target:   "http://127.0.0.1:5174",
		WsTarget: "ws://127.0.0.1:5174",
		Name:     "plugin-openlist-web",
		Hint:     "Check pm2 status for plugin-openlist-vite",
		Required: false,
		PathRewrite: func(p string) string {
			r := strings.TrimPrefix(p, "/openlist-ui")
			if r == "" {
				return "/"
			}
			if strings.HasPrefix(r, "/") {
				return r
			}
			return "/" + r
		},
	},
	{
		Match:    "/openlist",
		Target:   "http://127.0.0.1:5244",
		WsTarget: "ws://127.0.0.1:5244",
		Name:     "openlist-direct",
		Hint:     "Check pm2 status for openlist (:5244)",
		Required: false,
		PathRewrite: func(p string) string {
			r := strings.TrimPrefix(p, "/openlist")
			if r == "" {
				return "/"
			}
			if strings.HasPrefix(r, "/") {
				return r
			}
			return "/" + r
		},
	},
	{
		Match:    "/simverse",
		Target:   "http://127.0.0.1:5176",
		WsTarget: "ws://127.0.0.1:5176",
		Name:     "plugin-simverse-web",
		Hint:     "Check pm2 status for plugin-simverse-vite (:5176)",
		Required: false,
		PathRewrite: func(p string) string {
			r := strings.TrimPrefix(p, "/simverse")
			if r == "" {
				return "/"
			}
			if strings.HasPrefix(r, "/") {
				return r
			}
			return "/" + r
		},
	},
}

var ViteDeny = []ViteDenyRule{
	{Match: "/", Mode: "exact", Why: "根路径：Vite serve index.html，SPA 入口"},
	{Match: "/player", Mode: "prefix", Why: "ArtPlayerView SPA（router/index.ts:12）"},
	{Match: "/tabs", Mode: "prefix", Why: "Tabs SPA 全部子路由（home/files/tasks/settings/devlogs/...）"},

	{Match: "/simverse-home", Mode: "prefix", Why: "SimVerse 首页（router/index.ts）"},
	{Match: "/simverse-world", Mode: "prefix", Why: "SimVerse 横屏世界视图（router/index.ts）"},
	{Match: "/simverse/chronicle", Mode: "prefix", Why: "SimVerse 编年史路由（router/index.ts）"},

	{Match: "/@vite/", Mode: "prefix", Why: "Vite HMR client + module graph"},
	{Match: "/@fs/", Mode: "prefix", Why: "Vite fs allowlist"},
	{Match: "/@id/", Mode: "prefix", Why: "Vite virtual module id"},
	{Match: "/@react-refresh", Mode: "exact", Why: "React HMR 桥"},
	{Match: "/@client", Mode: "exact", Why: "Vite 内部 client-side HMR"},
	{Match: "/src/", Mode: "prefix", Why: "Vite 源码模块"},
	{Match: "/node_modules/", Mode: "prefix", Why: "Vite 优化后的 deps"},

	{Match: "/assets/", Mode: "prefix", Why: "Vite build assets"},
	{Match: "/public/", Mode: "prefix", Why: "Vite public 目录"},
	{Match: "/favicon.ico", Mode: "exact", Why: "favicon"},
	{Match: "/sw.js", Mode: "exact", Why: "Service Worker"},
	{Match: "/manifest", Mode: "prefix", Why: "PWA manifest"},
}

var EncvGoUpstream = &Upstream{
	Target:   "http://127.0.0.1:2025",
	WsTarget: "ws://127.0.0.1:2025",
	Name:     "encv-go",
	Hint:     "Check pm2 status for start-preview (encv-go :2025)",
	Required: true,
}

func MatchesPrefix(path, prefix string) bool {
	norm := prefix
	if strings.HasSuffix(norm, "/") {
		norm = norm[:len(norm)-1]
	}
	if path == norm {
		return true
	}
	if strings.HasPrefix(path, norm+"/") {
		return true
	}
	return false
}

func MatchesViteDeny(path string, rule ViteDenyRule) bool {
	if rule.Mode == "exact" {
		return path == rule.Match
	}
	if path == rule.Match {
		return true
	}
	if !strings.HasPrefix(path, rule.Match) {
		return false
	}
	if strings.HasSuffix(rule.Match, "/") {
		return len(path) > len(rule.Match)
	}
	next := path[len(rule.Match)]
	return next == '/' || next == '.'
}

func PickUpstream(rawURL, referer, cookie string) *Upstream {
	pathOnly := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		pathOnly = u.Path
	}
	if pathOnly == "" {
		pathOnly = "/"
	}

	for _, up := range SpecialUpstreams {
		if up.Match != "" && MatchesPrefix(pathOnly, up.Match) {
			return up
		}
	}

	if strings.Contains(cookie, "__plugin_spa=1") {
		for _, up := range SpecialUpstreams {
			if up.Match == "/openlist-ui" {
				return up
			}
		}
	}
	if strings.Contains(referer, "/openlist-ui/") {
		for _, up := range SpecialUpstreams {
			if up.Match == "/openlist-ui" {
				return up
			}
		}
	}

	for _, rule := range ViteDeny {
		if MatchesViteDeny(pathOnly, rule) {
			return GetViteUpstream()
		}
	}

	return EncvGoUpstream
}

func GetAllUpstreams() []*Upstream {
	seen := make(map[string]bool)
	var result []*Upstream
	add := func(u *Upstream) {
		if u == nil || seen[u.Name] {
			return
		}
		seen[u.Name] = true
		result = append(result, u)
	}
	add(EncvGoUpstream)
	add(GetViteUpstream())
	for _, u := range SpecialUpstreams {
		add(u)
	}
	return result
}
