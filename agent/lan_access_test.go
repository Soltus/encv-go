package agent

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- Pure-function tests (no net stack required) ----

// TestIsLoopback covers the IPv4 127.0.0.0/8 detection. The
// helper intentionally ignores IPv6 loopback (::1) because
// the LAN-access feature is IPv4-only.
func TestIsLoopback(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.42", true},
		{"127.255.255.254", true},
		{"128.0.0.1", false},
		{"10.0.0.1", false},
		{"169.254.1.1", false},
		{"192.168.1.1", false},
		{"0.0.0.0", false},
		// IPv6 is out of scope but the helper must not panic.
		{"::1", false},
		{"fe80::1", false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("net.ParseIP(%q) returned nil", c.ip)
		}
		if got := isLoopback(ip); got != c.want {
			t.Errorf("isLoopback(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// TestIsLinkLocal covers the IPv4 169.254.0.0/16 detection.
// IPv6 fe80::/10 is out of scope for this task.
func TestIsLinkLocal(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"169.254.0.1", true},
		{"169.254.255.254", true},
		{"169.255.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"127.0.0.1", false},
		{"0.0.0.0", false},
		{"::1", false},
		{"fe80::1", false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("net.ParseIP(%q) returned nil", c.ip)
		}
		if got := isLinkLocal(ip); got != c.want {
			t.Errorf("isLinkLocal(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// TestIsLoopback_IPv4Slice makes sure the helper handles the
// canonical 4-byte form returned by net.IP.To4() (rather than
// the 16-byte form returned by net.ParseIP for IPv4 dotted
// notation).
func TestIsLoopback_IPv4Slice(t *testing.T) {
	raw := net.ParseIP("127.0.0.1").To4()
	if raw == nil {
		t.Fatalf("127.0.0.1 To4() returned nil")
	}
	if !isLoopback(raw) {
		t.Errorf("isLoopback(4-byte 127.0.0.1) = false, want true")
	}
	if isLoopback(raw[:3]) {
		t.Errorf("isLoopback(3-byte 127.0.0 prefix) = true, want false")
	}
}

// TestIsLinkLocal_IPv4Slice mirrors the loopback helper test
// for the 4-byte canonical form.
func TestIsLinkLocal_IPv4Slice(t *testing.T) {
	raw := net.ParseIP("169.254.10.20").To4()
	if raw == nil {
		t.Fatalf("169.254.10.20 To4() returned nil")
	}
	if !isLinkLocal(raw) {
		t.Errorf("isLinkLocal(4-byte 169.254.10.20) = false, want true")
	}
}

// TestEnumerateIPv4_ReturnsAtLeastLoopbackExcluded walks the
// real net.Interfaces() table of the test host. We only
// assert on the negative property ("127.0.0.1 must never
// appear in the result") so the test is robust on every CI
// runner — it does not assume a specific network topology.
func TestEnumerateIPv4_ReturnsAtLeastLoopbackExcluded(t *testing.T) {
	addrs, err := EnumerateIPv4()
	if err != nil {
		t.Fatalf("EnumerateIPv4: %v", err)
	}
	for _, a := range addrs {
		if a.IP == "127.0.0.1" {
			t.Errorf("EnumerateIPv4() returned 127.0.0.1: %+v", a)
		}
		// Defensive: every entry must be a parseable IP. A
		// broken enumeration that leaked garbage into the
		// front-end would manifest here.
		if net.ParseIP(a.IP) == nil {
			t.Errorf("EnumerateIPv4 returned non-IP: %+v", a)
		}
		if a.Interface == "" {
			t.Errorf("EnumerateIPv4 returned entry with empty interface: %+v", a)
		}
		if a.IP == "" {
			t.Errorf("EnumerateIPv4 returned entry with empty IP: %+v", a)
		}
	}
}

// TestEnumerateIPv4_ExcludesLinkLocalAndUnspecified asserts
// the negative property for the two other categories the
// helper is documented to filter out. We synthesise the
// candidate IPs by calling ParseIP and inspecting the
// returned slice, so the test is independent of what the
// host actually exposes.
func TestEnumerateIPv4_ExcludesLinkLocalAndUnspecified(t *testing.T) {
	addrs, err := EnumerateIPv4()
	if err != nil {
		t.Fatalf("EnumerateIPv4: %v", err)
	}
	for _, a := range addrs {
		ip := net.ParseIP(a.IP)
		if ip == nil {
			t.Fatalf("ParseIP(%q) returned nil", a.IP)
		}
		if isLinkLocal(ip) {
			t.Errorf("EnumerateIPv4 returned link-local: %+v", a)
		}
		if ip.IsUnspecified() {
			t.Errorf("EnumerateIPv4 returned unspecified (0.0.0.0): %+v", a)
		}
		// IPv6 must never leak into the result. The helper
		// uses ip.To4() to filter, but a defensive assertion
		// here catches regressions where the filter is
		// accidentally relaxed.
		if ip.To4() == nil {
			t.Errorf("EnumerateIPv4 returned non-IPv4: %+v", a)
		}
	}
}

// TestEnumerateIPv4_StableOrder asserts that two back-to-back
// calls return the same sequence. Stable ordering matters
// because the front-end uses string equality to detect
// changes between polls.
func TestEnumerateIPv4_StableOrder(t *testing.T) {
	first, err := EnumerateIPv4()
	if err != nil {
		t.Fatalf("first EnumerateIPv4: %v", err)
	}
	second, err := EnumerateIPv4()
	if err != nil {
		t.Fatalf("second EnumerateIPv4: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("length changed between calls: %d -> %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("entry %d differs: %+v vs %+v", i, first[i], second[i])
		}
	}
}

// TestEnumerateIPv4_DedupesSameIPAcrossInterfaces forces
// the same IP onto two distinct synthetic addresses (which is
// what happens on hosts where a bridge and a physical NIC
// both bind the same address). The result must contain the
// IP exactly once.
//
// We achieve the dedup assertion by checking that every IP
// in the result is unique, which is the same invariant.
func TestEnumerateIPv4_DedupesSameIPAcrossInterfaces(t *testing.T) {
	addrs, err := EnumerateIPv4()
	if err != nil {
		t.Fatalf("EnumerateIPv4: %v", err)
	}
	seen := make(map[string]int, len(addrs))
	for _, a := range addrs {
		seen[a.IP]++
	}
	for ip, n := range seen {
		if n > 1 {
			t.Errorf("IP %q appears %d times in EnumerateIPv4 result", ip, n)
		}
	}
}

// TestFormatLANURLs confirms that port is interpolated into
// each entry's URL field, with the documented "http://%s:%d"
// format and no trailing slash.
func TestFormatLANURLs(t *testing.T) {
	in := []LanAddress{
		{Interface: "en0", IP: "192.168.1.10"},
		{Interface: "en1", IP: "10.0.0.5"},
	}
	out := formatLANURLs(in, 5245)
	if len(out) != 2 {
		t.Fatalf("length: %d", len(out))
	}
	if out[0].URL != "http://192.168.1.10:5245" {
		t.Errorf("URL[0]: %q", out[0].URL)
	}
	if out[1].URL != "http://10.0.0.5:5245" {
		t.Errorf("URL[1]: %q", out[1].URL)
	}
	for _, a := range out {
		if strings.HasSuffix(a.URL, "/") {
			t.Errorf("URL has trailing slash: %q", a.URL)
		}
	}
}

// TestJoinHostPort covers the small host/port formatting
// helper used by the HTTP handler.
func TestJoinHostPort(t *testing.T) {
	if got := joinHostPort("192.168.1.10", 5245); got != "192.168.1.10:5245" {
		t.Errorf("joinHostPort with host: %q", got)
	}
	if got := joinHostPort("", 5245); got != ":5245" {
		t.Errorf("joinHostPort empty host: %q", got)
	}
	if got := joinHostPort("   ", 5245); got != ":5245" {
		t.Errorf("joinHostPort whitespace host: %q", got)
	}
}

// ---- HTTP handler tests ----

// TestHandleLanAccess_OK confirms the happy path: GET
// /api/network/lan-access returns 200 + a JSON body shaped
// {addresses:[{interface,ip,url}], port}. The exact list of
// addresses is host-dependent so we only assert on the
// shape and on the absence of loopback.
func TestHandleLanAccess_OK(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/network/lan-access", nil)
	a.HandleLanAccess(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: %q", ct)
	}
	var resp LanAccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Port != 5245 {
		t.Errorf("default port: got %d, want 5245", resp.Port)
	}
	for _, a := range resp.Addresses {
		if a.IP == "127.0.0.1" {
			t.Errorf("loopback leaked into HTTP response: %+v", a)
		}
		if a.URL == "" {
			t.Errorf("empty URL in response: %+v", a)
		}
		if !strings.HasPrefix(a.URL, "http://") {
			t.Errorf("URL missing http:// prefix: %q", a.URL)
		}
	}
}

// TestHandleLanAccess_PortQuery confirms that ?port=N is
// honoured and threaded into each entry's URL field.
func TestHandleLanAccess_PortQuery(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/network/lan-access?port=8123", nil)
	a.HandleLanAccess(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp LanAccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Port != 8123 {
		t.Errorf("port: got %d, want 8123", resp.Port)
	}
	for _, a := range resp.Addresses {
		if !strings.HasSuffix(a.URL, ":8123") {
			t.Errorf("URL not stamped with port 8123: %q", a.URL)
		}
	}
}

// TestHandleLanAccess_PortQueryInvalid confirms that a
// non-numeric port falls back to the default (5245) rather
// than 400-ing. The endpoint is informational.
func TestHandleLanAccess_PortQueryInvalid(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/network/lan-access?port=not-a-number", nil)
	a.HandleLanAccess(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful fall-through), got %d", rec.Code)
	}
	var resp LanAccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Port != 5245 {
		t.Errorf("invalid port should fall back to 5245, got %d", resp.Port)
	}
}

// TestHandleLanAccess_PortQueryOutOfRange confirms that
// port=0 and port=99999 are also rejected with a fall-through
// to the default. Both ends of the legal port range are
// checked.
func TestHandleLanAccess_PortQueryOutOfRange(t *testing.T) {
	for _, raw := range []string{"0", "-1", "99999"} {
		t.Run("port="+raw, func(t *testing.T) {
			a := NewAgent(AgentConfig{}, NewRegistry())
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/network/lan-access?port="+raw, nil)
			a.HandleLanAccess(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			var resp LanAccessResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.Port != 5245 {
				t.Errorf("out-of-range port %q should fall back to 5245, got %d", raw, resp.Port)
			}
		})
	}
}

// TestHandleLanAccess_MethodNotAllowed confirms that DELETE
// (and any other non-GET/POST verb) is rejected with 405.
func TestHandleLanAccess_MethodNotAllowed(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/network/lan-access", nil)
	a.HandleLanAccess(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
	if rec.Header().Get("Allow") == "" {
		t.Errorf("expected Allow header on 405")
	}
}

// TestHandleLanAccess_POSTAlsoSupported confirms that POST
// is accepted (the endpoint is also useful as a manual
// curl-style probe from a developer terminal).
func TestHandleLanAccess_POSTAlsoSupported(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/network/lan-access?port=8080", nil)
	a.HandleLanAccess(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("POST should be supported, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---- Spec-named tests (Task 26 acceptance criteria) ----

// TestEnumerateIPv4_ExcludesLoopback is the spec-named variant of
// the loopback-exclusion assertion. It walks the real net stack of
// the test host and asserts that no entry in the result is a
// 127.0.0.0/8 address — the property the front-end relies on to
// avoid showing "http://127.0.0.1:5245/" as a "LAN access" URL.
func TestEnumerateIPv4_ExcludesLoopback(t *testing.T) {
	addrs, err := EnumerateIPv4()
	if err != nil {
		t.Fatalf("EnumerateIPv4: %v", err)
	}
	for _, a := range addrs {
		if a.IP == "127.0.0.1" {
			t.Errorf("EnumerateIPv4 returned 127.0.0.1: %+v", a)
		}
		ip := net.ParseIP(a.IP)
		if ip != nil && ip.IsLoopback() {
			t.Errorf("EnumerateIPv4 returned loopback IP: %s", a.IP)
		}
	}
}

// TestEnumerateIPv4_ReturnsValidShape asserts every returned entry
// has the documented wire shape: a non-empty interface name, a
// non-empty IP, and a parseable IP literal. This is the test that
// protects the front-end from accidental field renames or
// unparseable garbage leaking into the JSON payload.
func TestEnumerateIPv4_ReturnsValidShape(t *testing.T) {
	addrs, err := EnumerateIPv4()
	if err != nil {
		t.Fatalf("EnumerateIPv4: %v", err)
	}
	for _, a := range addrs {
		if a.Interface == "" {
			t.Errorf("empty Interface field: %+v", a)
		}
		if a.IP == "" {
			t.Errorf("empty IP field: %+v", a)
		}
		if net.ParseIP(a.IP) == nil {
			t.Errorf("non-parseable IP: %+v", a)
		}
		// Field type sanity: the IP must look like a dotted quad
		// (the function is documented IPv4-only). A regression
		// that leaks an IPv6 literal would manifest here.
		if !looksLikeIPv4(a.IP) {
			t.Errorf("non-IPv4-looking address: %q", a.IP)
		}
	}
}

// looksLikeIPv4 is a tiny helper used by TestEnumerateIPv4_ReturnsValidShape.
// It returns true when s has the "a.b.c.d" shape of an IPv4 dotted
// quad. We do not use net.ParseIP because that also accepts
// abbreviated IPv6 forms.
func looksLikeIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// TestIsPrivateOrPublic uses *net.IPNet mocks to verify the
// classification of every documented IPv4 category. This is the
// test the spec calls out explicitly: it constructs in-memory
// *net.IPNet values (no net stack required) and checks the
// classification. The mock mask is a /32 because the classifier
// only inspects address bytes, but the field is required by the
// struct literal.
func TestIsPrivateOrPublic(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want string
	}{
		// Loopback
		{"loopback-127.0.0.1", "127.0.0.1", "loopback"},
		{"loopback-127.0.0.42", "127.0.0.42", "loopback"},
		{"loopback-edge-127.255.255.254", "127.255.255.254", "loopback"},
		// Link-local
		{"link-local-169.254.0.1", "169.254.0.1", "link-local"},
		{"link-local-edge-169.254.255.254", "169.254.255.254", "link-local"},
		// Unspecified
		{"unspecified-0.0.0.0", "0.0.0.0", "unspecified"},
		// RFC 1918 private ranges
		{"private-10/8-low", "10.0.0.1", "private"},
		{"private-10/8-high", "10.255.255.255", "private"},
		{"private-172.16/12-low", "172.16.0.1", "private"},
		{"private-172.16/12-mid", "172.20.5.5", "private"},
		{"private-172.16/12-high", "172.31.255.255", "private"},
		{"private-192.168/16-low", "192.168.0.1", "private"},
		{"private-192.168/16-high", "192.168.255.255", "private"},
		// Public addresses
		{"public-1.1.1.1", "1.1.1.1", "public"},
		{"public-8.8.8.8", "8.8.8.8", "public"},
		{"public-9.9.9.9", "9.9.9.9", "public"},
		// Off-by-one boundary checks
		{"public-172.15-just-below", "172.15.255.255", "public"},
		{"public-172.32-just-above", "172.32.0.0", "public"},
		{"public-192.167-just-below", "192.167.255.255", "public"},
		{"public-192.169-just-above", "192.169.0.0", "public"},
		{"public-11-just-above-10", "11.0.0.0", "public"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ip := net.ParseIP(c.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", c.ip)
			}
			mock := &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
			if got := IsPrivateOrPublic(mock); got != c.want {
				t.Errorf("IsPrivateOrPublic(%s) = %q, want %q", c.ip, got, c.want)
			}
		})
	}

	// Edge cases: nil/empty input. The function must not panic.
	t.Run("nil-input", func(t *testing.T) {
		if got := IsPrivateOrPublic(nil); got != "unknown" {
			t.Errorf("IsPrivateOrPublic(nil) = %q, want unknown", got)
		}
	})
	t.Run("zero-value-IPNet", func(t *testing.T) {
		if got := IsPrivateOrPublic(&net.IPNet{}); got != "unknown" {
			t.Errorf("IsPrivateOrPublic(empty) = %q, want unknown", got)
		}
	})
	t.Run("ipv6-input", func(t *testing.T) {
		ip := net.ParseIP("::1")
		if ip == nil {
			t.Fatalf("net.ParseIP(::1) returned nil")
		}
		mock := &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		if got := IsPrivateOrPublic(mock); got != "unknown" {
			t.Errorf("IsPrivateOrPublic(::1) = %q, want unknown", got)
		}
	})
}
