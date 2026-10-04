// Package peerlink —— 双端互联（桌面 web ⇄ 安卓）的对等链路核心。
//
// spec: .trae/specs/desktop-web-android-pairing
//
// 拓扑前提（2026-10-02 用户纠正后的决定性地基）：
//   - 桌面端（web）跑在 **cnb 公网服务器**（与 Go 后端同源）
//   - 安卓端在 **NAT/CGNAT 之后，没有公网可达地址**
//     ⇒ 两端**不同网**，跨端一律走「Hub（cnb 上的本进程）+ 手机端主动出网的长连接」。
//     ⇒ 本包不提供任何 LAN 直连能力；地址列表只用于记录，不用于入站连接。
//
// 存储纪律（硬性）：psk / token / 信任态 **只存进程内存**，
// 禁止写盘（localStorage / config.user.json / 任何文件）。进程重启即全部失效。
package peerlink

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Ticket 是桌面端申请的一次性配对票据（编码进二维码交给安卓端扫码）。
//
// 二维码内容形如：
//
//	{ "v":2, "hub":"https://<host>/api/peerlink", "pairingId":"...", "psk":"<hex 64>", "exp":120 }
//
// ⚠️ 不得包含任何内网地址（R3：https 页面请求 http 内网地址会被浏览器按混合内容拦截）。
type Ticket struct {
	PairingID string    `json:"pairingId"`
	PSKHex    string    `json:"psk"`
	Hub       string    `json:"hub"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// NewPairingID 生成配对 ID（16 字节随机 → hex）。
func NewPairingID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("peerlink: pairing id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Peer 是一台已配对的远端设备（长期身份；可持久化，不含任何密钥）。
type Peer struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"deviceId"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform"` // android / ios / web / electron
	PairedAt  time.Time `json:"pairedAt"`
	LastSeen  time.Time `json:"lastSeen"`
	Online    bool      `json:"online"`
	RemoteKey []byte    `json:"-"` // 出向报文的 AEAD 密钥（仅内存）
	LocalKey  []byte    `json:"-"` // 入向报文的 AEAD 密钥（仅内存）
}

// Session 是一次配对产生的会话凭据（**仅内存**，进程重启即失效）。
type Session struct {
	PeerID  string `json:"peerId"`
	Token   string `json:"token"` // 对端访问本端用的令牌
	Created time.Time
}

// shortID 从 deviceId 派生稳定的短 peer id（便于日志与 UI 显示）。
func shortID(deviceID string) string {
	if len(deviceID) <= 12 {
		return deviceID
	}
	return deviceID[:12]
}
