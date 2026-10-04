package peerlink

// hub_stale_peer_test.go —— 「这台设备大概已经不在了」必须**可被识别**（2026-10-05）
//
// 对应 docs/persisted-state-selfhealing.md 的 P0-3。
//
// 为什么要有它：Hub 会永久保留已配对设备（解配才是删除入口，这是对的），
//   于是列表里会躺着两类"坏身份"而无人告知：
//     never_linked    —— 配对**从未真正连上**（地址不对 / token 失效），用户以为配好了；
//     offline_too_long —— 设备早已不在（重装 / 换机 / 长期关机），云控还会推错对象。
//   二者都属于"持久化的坏值没被识别"，与 baseUrl 的 :16666 同一类。
//
// 反向锁同样重要：手机息屏几小时、后端刚重启 ⇒ **不得**误判成"设备没了"。

import (
	"testing"
	"time"
)

func TestPeerStale_NeverLinked(t *testing.T) {
	now := time.Now()
	p := &Peer{ID: "p1", PairedAt: now.Add(-11 * time.Minute), LastSeen: now.Add(-11 * time.Minute)}
	if stale, reason := peerStale(p, false, now); !stale || reason != "never_linked" {
		t.Fatalf("配对 11 分钟从未连上 ⇒ 应 stale=never_linked, got %v/%q", stale, reason)
	}
}

func TestPeerStale_OfflineTooLong(t *testing.T) {
	now := time.Now()
	p := &Peer{ID: "p2", PairedAt: now.Add(-72 * time.Hour), LastSeen: now.Add(-25 * time.Hour)}
	if stale, reason := peerStale(p, false, now); !stale || reason != "offline_too_long" {
		t.Fatalf("连续离线 25 小时 ⇒ 应 stale=offline_too_long, got %v/%q", stale, reason)
	}
}

// TestPeerStale_DoesNotMistakeJitter —— 抖动不得被当成"设备没了"
func TestPeerStale_DoesNotMistakeJitter(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		p    *Peer
	}{
		{"刚配对 1 分钟还没连上（正在连）", &Peer{PairedAt: now.Add(-1 * time.Minute), LastSeen: now.Add(-1 * time.Minute)}},
		{"连过后离线 5 分钟（后端刚重启）", &Peer{PairedAt: now.Add(-time.Hour), LastSeen: now.Add(-5 * time.Minute)}},
		{"离线 23 小时（还没到阈值）", &Peer{PairedAt: now.Add(-48 * time.Hour), LastSeen: now.Add(-23 * time.Hour)}},
	}
	for _, c := range cases {
		if stale, reason := peerStale(c.p, false, now); stale {
			t.Errorf("%s ⇒ 不得判 stale, got reason=%q", c.name, reason)
		}
	}
	// 在线的设备永不 stale
	online := &Peer{PairedAt: now.Add(-48 * time.Hour), LastSeen: now.Add(-48 * time.Hour)}
	if stale, _ := peerStale(online, true, now); stale {
		t.Error("在线设备不得判 stale")
	}
}

// TestHub_ListPeers_ReportsStale —— 列表必须把 stale 与原因吐给 UI（否则 UI 无从判断）
func TestHub_ListPeers_ReportsStale(t *testing.T) {
	h := NewHub("h", "v")
	now := time.Now()
	h.peers["p-never"] = &Peer{ID: "p-never", DeviceID: "d1", PairedAt: now.Add(-30 * time.Minute), LastSeen: now.Add(-30 * time.Minute)}
	h.peers["p-ok"] = &Peer{ID: "p-ok", DeviceID: "d2", PairedAt: now.Add(-time.Hour), LastSeen: now, Online: true}

	var never, ok map[string]any
	for _, it := range h.ListPeers() {
		switch it["id"] {
		case "p-never":
			never = it
		case "p-ok":
			ok = it
		}
	}
	if never == nil || ok == nil {
		t.Fatalf("列表缺少设备: %+v", h.ListPeers())
	}
	if never["stale"] != true || never["staleReason"] != "never_linked" {
		t.Fatalf("p-never 应带 stale=never_linked, got %+v", never)
	}
	if ok["stale"] != false || ok["online"] != true {
		t.Fatalf("p-ok 不得 stale, got %+v", ok)
	}
	// 脱敏红线：列表里绝不能出现密钥字段
	for _, it := range h.ListPeers() {
		for _, banned := range []string{"remoteKey", "localKey", "RemoteKey", "LocalKey"} {
			if _, found := it[banned]; found {
				t.Fatalf("ListPeers 泄漏密钥字段 %q", banned)
			}
		}
	}
}
