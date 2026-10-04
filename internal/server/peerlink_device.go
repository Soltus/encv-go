package server

// peerlink_device.go —— **设备指纹**（2026-10-05）
//
// 问题：deviceId 原来是"每次进程启动随机生成"的（`peerlink.NewHub` 里的 peerID）。
//   ⇒ 同一台安卓机**重装 / 清缓存**后重新配对，Hub 侧就多一条 peer 记录
//      （真机实测同机遗留两条：ac5e1c75ede3 / 6454ab90716f），
//      列表里躺着永远离线的"旧自己"，云控推送也会推错对象。
//
// 指纹取值优先级（稳定性从高到低）：
//   1. `ENCV_DEVICE_ID`    —— 显式覆盖（开发/自测/CI 需要确定性）
//   2. `ENCV_ANDROID_ID`   —— Kotlin 注入的 Settings.Secure.ANDROID_ID
//                             跨重装/清缓存稳定，仅在恢复出厂 / 换签名时变
//   3. 落盘随机 UUID        —— <peerlink 目录>/device.json，首次生成后固定
//   4. 随机（进程内）       —— 持久化不可用时退回旧行为
//
// ⚠️ 指纹只用于"识别同一台设备"，**不是密钥、不承担鉴权**：
//    鉴权仍然只靠票据派生的 token 与 AEAD 密钥。指纹会出现在配对请求与设备列表里，
//    所以不直接透传 ANDROID_ID 原文（取它的 sha256 前缀），避免把系统标识原样扩散。

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
)

// deviceFile 落盘指纹的文件名（与 peers.json 同目录、同 0600 纪律）。
const deviceFile = "device.json"

// persistedDevice 落盘的随机指纹（无 ANDROID_ID 时的兜底）。
type persistedDevice struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Source    string    `json:"source"`
}

// deviceFingerprint 解析本端设备指纹（永不返回空串）。
func deviceFingerprint(st *peerlink.Store) string {
	if v := strings.TrimSpace(os.Getenv("ENCV_DEVICE_ID")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("ENCV_ANDROID_ID")); v != "" {
		sum := sha256.Sum256([]byte("encv-android-id:" + v))
		return "a" + hex.EncodeToString(sum[:])[:31]
	}
	if st != nil && st.Enabled() {
		if id := loadOrCreateDeviceID(st); id != "" {
			return id
		}
	}
	return randomDeviceID()
}

// loadOrCreateDeviceID 读回落盘指纹；没有就生成并写死（之后不再变）。
func loadOrCreateDeviceID(st *peerlink.Store) string {
	var cur persistedDevice
	ok, err := st.LoadState(deviceFile, &cur)
	if err != nil {
		slog.Warn("peerlink device id load failed", "error", err.Error())
	}
	if ok && strings.TrimSpace(cur.ID) != "" {
		return strings.TrimSpace(cur.ID)
	}
	id := randomDeviceID()
	if err := st.SaveState(deviceFile, persistedDevice{ID: id, CreatedAt: time.Now(), Source: "generated"}); err != nil {
		// 写不进去不能让启动失败；退化成"本次进程内固定"（至少本次配对自洽）
		slog.Warn("peerlink device id save failed", "error", err.Error())
	}
	return id
}

func randomDeviceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand 失败在实践中不会发生；退化成时间戳也好于空串（空串会让 Hub 认不出设备）
		return "t" + hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return "r" + hex.EncodeToString(b)
}
