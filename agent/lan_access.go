package agent

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// LanAddress is one routable IPv4 address a peer on the same
// LAN could use to reach this agent. The URL field is computed
// by [EnumerateIPv4] from the IP and the supplied port so the
// front-end does not have to format it again.
//
// JSON shape (Go field tags):
//
//	{"interface":"en0","ip":"192.168.1.10","url":"http://192.168.1.10:5245"}
//
// The struct is shared between the LAN-access HTTP handler
// and the front-end type. Field names are the wire contract;
// renaming them is a breaking change.
type LanAddress struct {
	Interface string `json:"interface"`
	IP        string `json:"ip"`
	URL       string `json:"url"`
}

// EnumerateIPv4 walks every active network interface and
// returns the IPv4 addresses that look reachable from a peer
// on the local network:
//
//   - 127.0.0.0/8 (loopback)         → excluded
//   - 169.254.0.0/16 (link-local)    → excluded
//   - 0.0.0.0                         → excluded
//   - any other unicast IPv4 address  → returned, deduped,
//                                      sorted by interface name
//                                      and then by IP
//
// Down / loopback-only interfaces are silently skipped. The
// function returns an error only when [net.Interfaces] itself
// fails (which essentially never happens on Linux/macOS/Windows
// but the API is documented to return one).
//
// The function is intentionally synchronous and side-effect
// free so it can be called from any HTTP handler without
// making the agent core async-aware.
func EnumerateIPv4() ([]LanAddress, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("enumerate interfaces: %w", err)
	}

	// de-dup by IP — the same address can be bound on multiple
	// interfaces (e.g. utun / bridge on macOS aliases).
	seen := make(map[string]struct{})
	out := make([]LanAddress, 0, len(ifs))

	for _, iface := range ifs {
		// Skip interfaces that are administratively down or
		// flagged as loopback — their addresses cannot be
		// reached from another device on the LAN.
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			// A single bad interface should not poison the
			// whole enumeration; log-skip and move on.
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if ip == nil {
				continue
			}
			// We deliberately return only IPv4. IPv6 is out of
			// scope for this task (per the spec).
			if ip.To4() == nil {
				continue
			}
			if isLoopback(ip) || isLinkLocal(ip) || ip.IsUnspecified() {
				continue
			}
			key := ip.String()
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, LanAddress{
				Interface: iface.Name,
				IP:        key,
			})
		}
	}

	// Stable ordering: by interface name, then by IP. This
	// keeps the front-end's diff between two polls meaningful.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Interface != out[j].Interface {
			return out[i].Interface < out[j].Interface
		}
		return out[i].IP < out[j].IP
	})
	return out, nil
}

// isLoopback reports whether ip belongs to the 127.0.0.0/8
// block. We re-implement the check (rather than calling
// ip.IsLoopback) because IsLoopback is IPv6-aware and we want
// to be explicit about the IPv4 semantics this function is
// used for.
//
// ip may be a 4-byte or 16-byte slice; both forms are accepted.
func isLoopback(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 127
}

// isLinkLocal reports whether ip is in the 169.254.0.0/16
// block (RFC 3927). We do not use ip.IsLinkLocal because the
// stdlib helper also matches IPv6 fe80::/10, which is out of
// scope here.
//
// ip may be a 4-byte or 16-byte slice; both forms are accepted.
func isLinkLocal(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 169 && v4[1] == 254
}

// formatLANURLs rewrites the URL field on each entry with
// the supplied port. It is a small helper kept next to
// [EnumerateIPv4] so the format is centralised: the wire
// contract is "http://{ip}:{port}" with no trailing slash and
// no path.
//
// A non-positive port is treated as 0 and the URL is built
// as "http://{ip}:0" so callers can still see the IP; this
// is mostly defensive (the HTTP handler should never pass 0).
func formatLANURLs(addrs []LanAddress, port int) []LanAddress {
	for i := range addrs {
		addrs[i].URL = fmt.Sprintf("http://%s:%d", addrs[i].IP, port)
	}
	return addrs
}

// hasNonLoopbackAddress is a tiny convenience used by tests
// and by the HTTP handler: it returns true if at least one
// address was enumerated. It exists so callers do not have to
// import the "len() == 0" idiom.
func hasNonLoopbackAddress(addrs []LanAddress) bool {
	return len(addrs) > 0
}

// joinHostPort is a thin wrapper used by the HTTP handler so
// we do not import net everywhere. It returns "host:port" for
// any non-empty host and a bare ":port" otherwise.
func joinHostPort(host string, port int) string {
	if strings.TrimSpace(host) == "" {
		return fmt.Sprintf(":%d", port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// IsPrivateOrPublic classifies a *net.IPNet into one of the
// documented wire categories used by the LAN-access feature:
//
//   - "unspecified"  0.0.0.0
//   - "loopback"     127.0.0.0/8
//   - "link-local"   169.254.0.0/16 (RFC 3927)
//   - "private"      RFC 1918: 10/8, 172.16/12, 192.168/16
//   - "public"       everything else (routable on the internet)
//
// The classification is IPv4-only; nil/empty nets, nil IP, and
// IPv6 nets are reported as "unknown" so callers can treat them
// defensively without panicking. The function does not consult
// ipNet.Mask — every RFC 1918 block is matched on its address
// bytes alone, which matches the spec's "construct *net.IPNet
// mock" testing approach where the mask is mostly irrelevant.
func IsPrivateOrPublic(ipNet *net.IPNet) string {
	if ipNet == nil {
		return "unknown"
	}
	ip := ipNet.IP
	if ip == nil {
		return "unknown"
	}
	v4 := ip.To4()
	if v4 == nil {
		return "unknown"
	}
	// 0.0.0.0 (anycast / "bind to all" sentinel)
	if v4[0] == 0 {
		return "unspecified"
	}
	// 127.0.0.0/8 — host-internal loopback
	if v4[0] == 127 {
		return "loopback"
	}
	// 169.254.0.0/16 — RFC 3927 link-local (used by DHCP-less hosts)
	if v4[0] == 169 && v4[1] == 254 {
		return "link-local"
	}
	// RFC 1918 private ranges. Boundary checks below guard against
	// the well-known off-by-one traps (172.15 is public, 172.32 is
	// public; 192.167 is public, 192.169 is public).
	if v4[0] == 10 {
		return "private"
	}
	if v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31 {
		return "private"
	}
	if v4[0] == 192 && v4[1] == 168 {
		return "private"
	}
	return "public"
}
