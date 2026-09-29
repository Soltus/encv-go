package routing

import (
	"testing"
)

func TestMatchesPrefix_Exact(t *testing.T) {
	if !MatchesPrefix("/stream", "/stream") {
		t.Error("expected exact match")
	}
	if !MatchesPrefix("/api", "/api") {
		t.Error("expected exact match")
	}
}

func TestMatchesPrefix_Prefix(t *testing.T) {
	if !MatchesPrefix("/api/", "/api") {
		t.Error("expected prefix match with trailing slash")
	}
	if !MatchesPrefix("/api/config", "/api") {
		t.Error("expected prefix match")
	}
	if !MatchesPrefix("/api/v1/users/123", "/api") {
		t.Error("expected deep prefix match")
	}
}

func TestMatchesPrefix_NotSimilar(t *testing.T) {
	if MatchesPrefix("/streamer", "/stream") {
		t.Error("should not match /streamer for /stream")
	}
	if MatchesPrefix("/apiv2", "/api") {
		t.Error("should not match /apiv2 for /api")
	}
	if MatchesPrefix("/api-other", "/api") {
		t.Error("should not match /api-other for /api")
	}
}

func TestMatchesViteDeny_Exact(t *testing.T) {
	exact := ViteDenyRule{Match: "/favicon.ico", Mode: "exact", Why: "test"}
	if !MatchesViteDeny("/favicon.ico", exact) {
		t.Error("expected exact match")
	}
	if MatchesViteDeny("/favicon.ico/", exact) {
		t.Error("should not match with trailing slash")
	}
	if MatchesViteDeny("/favicon.icon", exact) {
		t.Error("should not match different ext")
	}
}

func TestMatchesViteDeny_PrefixWithSlash(t *testing.T) {
	prefix := ViteDenyRule{Match: "/@vite/", Mode: "prefix", Why: "test"}
	if !MatchesViteDeny("/@vite/client", prefix) {
		t.Error("expected prefix match")
	}
	if !MatchesViteDeny("/@vite/", prefix) {
		t.Error("expected exact prefix match")
	}
	if MatchesViteDeny("/@vite-other", prefix) {
		t.Error("should not match /@vite-other")
	}
	if MatchesViteDeny("/@vite", prefix) {
		t.Error("should not match without trailing slash")
	}
}

func TestMatchesViteDeny_PrefixWithoutSlash(t *testing.T) {
	manifest := ViteDenyRule{Match: "/manifest", Mode: "prefix", Why: "test"}
	if !MatchesViteDeny("/manifest", manifest) {
		t.Error("expected exact match")
	}
	if !MatchesViteDeny("/manifest.json", manifest) {
		t.Error("expected .json match")
	}
	if !MatchesViteDeny("/manifest.webmanifest", manifest) {
		t.Error("expected .webmanifest match")
	}
	if !MatchesViteDeny("/manifest/", manifest) {
		t.Error("expected / match")
	}
	if !MatchesViteDeny("/manifest/foo", manifest) {
		t.Error("expected /foo match")
	}
	if MatchesViteDeny("/manifest-foo", manifest) {
		t.Error("should not match /manifest-foo")
	}
}

func TestPickUpstream_RootToVite(t *testing.T) {
	up := PickUpstream("/", "", "")
	if up.Name != GetViteUpstream().Name {
		t.Errorf("expected %s, got %s", GetViteUpstream().Name, up.Name)
	}
}

func TestPickUpstream_PlayerToVite(t *testing.T) {
	up := PickUpstream("/player", "", "")
	if up.Name != GetViteUpstream().Name {
		t.Errorf("expected Vite, got %s", up.Name)
	}
	up = PickUpstream("/player?path=/test", "", "")
	if up.Name != GetViteUpstream().Name {
		t.Errorf("expected Vite with query, got %s", up.Name)
	}
}

func TestPickUpstream_TabsToVite(t *testing.T) {
	for _, p := range []string{"/tabs", "/tabs/", "/tabs/home", "/tabs/files", "/tabs/settings/server/http"} {
		up := PickUpstream(p, "", "")
		if up.Name != GetViteUpstream().Name {
			t.Errorf("path %s: expected Vite, got %s", p, up.Name)
		}
	}
}

func TestPickUpstream_ViteDevArtifacts(t *testing.T) {
	vitePaths := []string{
		"/@vite/client",
		"/@fs/workspace/foo.ts",
		"/@id/virtual",
		"/@react-refresh",
		"/@client",
		"/src/main.ts",
		"/src/views/ArtPlayerView.vue",
		"/node_modules/.vite/deps/vue.js",
		"/node_modules/foo/bar.js",
		"/favicon.ico",
		"/sw.js",
		"/manifest.json",
		"/manifest.webmanifest",
		"/assets/index.js",
		"/public/logo.png",
	}
	for _, p := range vitePaths {
		up := PickUpstream(p, "", "")
		if up.Name != GetViteUpstream().Name {
			t.Errorf("path %s: expected Vite, got %s", p, up.Name)
		}
	}
}

func TestPickUpstream_OpenlistUi(t *testing.T) {
	up := PickUpstream("/openlist-ui/", "", "")
	if up.Name != "plugin-openlist-web" {
		t.Errorf("expected plugin-openlist-web, got %s", up.Name)
	}
	up = PickUpstream("/openlist-ui/src/main.ts", "", "")
	if up.Name != "plugin-openlist-web" {
		t.Errorf("expected plugin-openlist-web, got %s", up.Name)
	}
}

func TestPickUpstream_OpenlistDirect(t *testing.T) {
	up := PickUpstream("/openlist/foo", "", "")
	if up.Name != "openlist-direct" {
		t.Errorf("expected openlist-direct, got %s", up.Name)
	}
	if up.PathRewrite == nil {
		t.Fatal("expected pathRewrite")
	}
	if up.PathRewrite("/openlist") != "/" {
		t.Errorf("pathRewrite /openlist -> %s", up.PathRewrite("/openlist"))
	}
	if up.PathRewrite("/openlist/foo") != "/foo" {
		t.Errorf("pathRewrite /openlist/foo -> %s", up.PathRewrite("/openlist/foo"))
	}
}

func TestPickUpstream_PluginSpaCookieFallback(t *testing.T) {
	up := PickUpstream("/src/main.ts", "", "__plugin_spa=1; foo=bar")
	if up.Name != "plugin-openlist-web" {
		t.Errorf("cookie fallback: expected plugin-openlist-web, got %s", up.Name)
	}
}

func TestPickUpstream_PluginSpaRefererFallback(t *testing.T) {
	up := PickUpstream("/src/main.ts", "https://x.com/openlist-ui/main", "")
	if up.Name != "plugin-openlist-web" {
		t.Errorf("referer fallback: expected plugin-openlist-web, got %s", up.Name)
	}
}

func TestPickUpstream_BackendDefault(t *testing.T) {
	backendPaths := []string{
		"/api/config",
		"/api/service-guard",
		"/api/container-version/supported",
		"/agent-api/api/models",
		"/agent-api/api/chat",
		"/preview/text",
		"/stream?path=/01-plain-media/sample.mp4",
		"/decrypt?path=/01-plain-media/sample.mp4",
		"/play",
		"/p/",
		"/p/abc",
		"/ws",
		"/health",
		"/ping",
	}
	for _, p := range backendPaths {
		up := PickUpstream(p, "", "")
		if up.Name != EncvGoUpstream.Name {
			t.Errorf("path %s: expected encv-go, got %s", p, up.Name)
		}
	}
}

func TestPickUpstream_QueryNotAffect(t *testing.T) {
	up := PickUpstream("/stream?path=%252Ftest", "", "")
	if up.Name != EncvGoUpstream.Name {
		t.Errorf("query should not affect: expected encv-go, got %s", up.Name)
	}
	up = PickUpstream("/@vite/client?v=123", "", "")
	if up.Name != GetViteUpstream().Name {
		t.Errorf("query should not affect vite: expected Vite, got %s", up.Name)
	}
}

func TestPickUpstream_SimilarPrefix(t *testing.T) {
	up := PickUpstream("/streamer", "", "")
	if up.Name != EncvGoUpstream.Name {
		t.Errorf("/streamer should go to encv-go, got %s", up.Name)
	}
	up = PickUpstream("/tabsfoo", "", "")
	if up.Name != EncvGoUpstream.Name {
		t.Errorf("/tabsfoo should go to encv-go, got %s", up.Name)
	}
}

func TestPickUpstream_UnknownDefault(t *testing.T) {
	up := PickUpstream("/foo-bar-baz", "", "")
	if up.Name != EncvGoUpstream.Name {
		t.Errorf("unknown path should default to encv-go, got %s", up.Name)
	}
}

func TestPickUpstream_EmptyUrl(t *testing.T) {
	up := PickUpstream("", "", "")
	if up.Name != GetViteUpstream().Name {
		t.Errorf("empty url should go to Vite, got %s", up.Name)
	}
}

func TestPickUpstream_NewBackendEndpoints(t *testing.T) {
	newEndpoints := []string{
		"/api/v2/new-endpoint",
		"/api/v3/ai/something",
		"/custom-route",
	}
	for _, p := range newEndpoints {
		up := PickUpstream(p, "", "")
		if up.Name != EncvGoUpstream.Name {
			t.Errorf("new endpoint %s should auto go to encv-go, got %s", p, up.Name)
		}
	}
}

func TestViteDeny_HasWhy(t *testing.T) {
	for _, rule := range ViteDeny {
		if rule.Why == "" {
			t.Errorf("rule %s missing why", rule.Match)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// crypto-ui（@encv/crypto WASM 预览页）路由
//
// 背景：Vite 8 dev 不基于 <base href> 改写模块内部 import，
// 页面发出的 /src/...、/@fs/... 请求不带 /crypto-ui 前缀，
// 只靠入口 HTML 种下的 spa cookie 归位（见 README §11）。
// ─────────────────────────────────────────────────────────────────────────────

func TestPickUpstream_EncvUi(t *testing.T) {
	for _, p := range []string{"/encv-ui", "/encv-ui/", "/encv-ui/main.js", "/encv-ui/main.js?v=1"} {
		up := PickUpstream(p, "", "")
		if up.Name != "encv-preview" {
			t.Errorf("path %s: expected encv-preview, got %s", p, up.Name)
		}
	}
}

func TestPickUpstream_EncvUiStripsPrefix(t *testing.T) {
	// 这是「剥前缀」的上游：静态目录托管，/encv-ui/style.css → /style.css。
	var encv *Upstream
	for _, up := range SpecialUpstreams {
		if up.Match == "/encv-ui" {
			encv = up
		}
	}
	if encv == nil {
		t.Fatal("special upstreams 缺少 /encv-ui")
	}
	if encv.Target != "http://127.0.0.1:5179" {
		t.Errorf("unexpected target: %s", encv.Target)
	}
	if encv.PathRewrite == nil {
		t.Fatal("/encv-ui 必须配置 PathRewrite（静态目录按根路径提供文件）")
	}
	if got := encv.PathRewrite("/encv-ui/main.js"); got != "/main.js" {
		t.Errorf("PathRewrite(/encv-ui/main.js) = %s, want /main.js", got)
	}
	if got := encv.PathRewrite("/encv-ui"); got != "/" {
		t.Errorf("PathRewrite(/encv-ui) = %s, want /", got)
	}
}

func TestPickUpstream_EncvUiSimilarPrefix(t *testing.T) {
	// /encv-ui-legacy 之类的前缀不能撞到演示页
	up := PickUpstream("/encv-ui-legacy/", "", "")
	if up.Name == "encv-preview" {
		t.Errorf("/encv-ui-legacy should not route to encv-preview, got %s", up.Name)
	}
}
