package server

// peerlink_device_test.go —— **设备指纹**的回归锁（2026-10-05）
//
// 真机问题：deviceId 每次进程启动随机生成 ⇒ 同一台安卓机重装 / 清缓存后重新配对，
//   Hub 侧就多一条 peer 记录（真机实测同机两条：ac5e1c75ede3 / 6454ab90716f），
//   列表里躺着永远离线的"旧自己"，云控推送也会推错对象。
//
// 锁的语义：
//  1. ANDROID_ID 优先，且**派生**（不透传系统标识原文）
//  2. 同一台机器两次启动必须得到同一个 ID（落盘兜底）
//  3. 显式覆盖（ENCV_DEVICE_ID）优先于一切（CI / 自测要确定性）

import (
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/peerlink"
)

func TestDeviceFingerprint_AndroidIdIsDerived(t *testing.T) {
	t.Setenv("ENCV_ANDROID_ID", "0123456789abcdef")
	got := deviceFingerprint(peerlink.NewStore(t.TempDir()))
	if got == "" {
		t.Fatal("指纹不得为空")
	}
	if strings.Contains(got, "0123456789abcdef") {
		t.Fatalf("不得透传 ANDROID_ID 原文（它会出现在设备列表里）: %s", got)
	}
	// 稳定：同样的输入必须得到同样的输出（否则重装后还是认不出同一台设备）
	again := deviceFingerprint(peerlink.NewStore(t.TempDir()))
	if got != again {
		t.Fatalf("同一台设备的指纹必须稳定: %q vs %q", got, again)
	}
}

func TestDeviceFingerprint_ExplicitOverrideWins(t *testing.T) {
	t.Setenv("ENCV_ANDROID_ID", "0123456789abcdef")
	t.Setenv("ENCV_DEVICE_ID", "ci-fixed-id")
	if got := deviceFingerprint(peerlink.NewStore(t.TempDir())); got != "ci-fixed-id" {
		t.Fatalf("显式覆盖应优先, got %q", got)
	}
}

func TestDeviceFingerprint_PersistedAcrossRestart(t *testing.T) {
	t.Setenv("ENCV_ANDROID_ID", "")
	dir := t.TempDir()

	// 第一次启动：生成并落盘
	first := deviceFingerprint(peerlink.NewStore(dir))
	if first == "" {
		t.Fatal("指纹不得为空")
	}
	// 第二次启动（新 Store，同一目录）：必须读回同一个
	second := deviceFingerprint(peerlink.NewStore(dir))
	if first != second {
		t.Fatalf("同一台设备的指纹必须跨启动稳定: %q vs %q", first, second)
	}
}

// TestDeviceFingerprint_UsedAsHubPeerID —— 指纹必须真的落到 Hub（否则等于没做）
func TestDeviceFingerprint_UsedAsHubPeerID(t *testing.T) {
	t.Setenv("ENCV_ANDROID_ID", "0123456789abcdef")
	h := peerlink.NewHub("encv-go", "")
	h.SetDeviceID(deviceFingerprint(peerlink.NewStore(t.TempDir())))
	if got := h.Info()["peerId"]; got == "" || got == "hub" {
		t.Fatalf("Hub 的 peerId 应取设备指纹, got %q", got)
	}
	// 空串不得覆盖（防御：注入失败时保留随机 peerId，而不是变成空）
	h2 := peerlink.NewHub("encv-go", "")
	before := h2.Info()["peerId"]
	h2.SetDeviceID("   ")
	if h2.Info()["peerId"] != before {
		t.Fatal("空指纹不得覆盖既有 peerId")
	}
}
