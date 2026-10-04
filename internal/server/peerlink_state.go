package server

// peerlink_state.go —— 互联状态的持久化与**重启自恢复**（2026-10-04）
//
// 背景（用户 2026-10-04）：原设计"psk / token / 密钥只存进程内存"过于谨慎 ——
// **服务端一重启，已配对设备全部失效，必须重新扫码**。真机联调时每改一次后端
// 就要重扫一次，这个成本不可接受。
//
// 现在的行为：
//   - Hub（桌面/云）：配对成功后把"设备 + token + 双向 AEAD 密钥"写进应用私有目录；
//     进程重启 ⇒ 启动时读回 ⇒ **设备带着旧 token 重连即可，不用重新扫码**。
//   - Edge（手机）：扫码配对成功后记住"会合点地址 + token"；Go 进程重启
//     （APP 冷启动 / 后端崩溃重启）⇒ 启动时**自动重新连上**，不用再扫一次。
//   - 解配 / 主动断开 ⇒ 立即删除落盘记录（钥匙可以收回来）。
//   - `ENCV_PEERLINK_PERSIST=0` ⇒ 整体关闭，行为退回旧纪律。
//
// ⚠️ 与"信任态"的关系：trust_device 仍是**进程内存**（重启后要重新点一次信任），
//    本文件只恢复"身份与通道"，不恢复授权 —— 恢复的是"能连上"，不是"能免确认执行"。

import (
	"log/slog"
	"os"
	"runtime"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/peerlink"
)

func runtimeGOOS() string { return runtime.GOOS }

// peerlinkStateDir 落盘目录（空串 = 禁用持久化）。
func peerlinkStateDir() string {
	if os.Getenv("ENCV_PEERLINK_PERSIST") == "0" {
		return ""
	}
	return config.AppDataDir("peerlink")
}

// restorePeerlinkState 启动时恢复 Hub 侧已配对设备，返回恢复条数。
//
// ⚠️ 只恢复"已配对"身份，Online 恒为 false（在线状态只能由长连接重建来证明）——
//    从磁盘读出一个"在线"是假的，会让 UI 显示一台其实没连上的设备。
func (s *Server) restorePeerlinkState() int {
	if s.peerHub == nil || !s.peerStore.Enabled() {
		return 0
	}
	items, err := s.peerStore.LoadPeers()
	if err != nil {
		// 坏文件：不恢复，但不阻止启动（保守）
		slog.Warn("peerlink state load failed, starting without restored peers", "error", err.Error())
		return 0
	}
	return s.peerHub.RestoreState(items)
}

// savePeerlinkPeers 把当前已配对设备落盘（配对 / 解配后调用）。
func (s *Server) savePeerlinkPeers() {
	if s.peerHub == nil || !s.peerStore.Enabled() {
		return
	}
	if err := s.peerStore.SavePeers(s.peerHub.ExportState()); err != nil {
		slog.Warn("peerlink state save failed", "error", err.Error())
	}
}

// restoreEdgeSession 启动时恢复本端作为 Edge 的会话并**自动重连**。
//
// 返回是否真的拉起了 Edge。调用点：Server.Start()（此时配置与目录都已就绪）。
func (s *Server) restoreEdgeSession() bool {
	if !s.peerStore.Enabled() {
		return false
	}
	sess, ok, err := s.peerStore.LoadEdgeSession()
	if err != nil {
		slog.Warn("peerlink edge session load failed", "error", err.Error())
		return false
	}
	if !ok {
		return false
	}
	s.peerEdgeMu.Lock()
	running := s.peerEdge != nil && s.peerEdge.cancel != nil
	s.peerEdgeMu.Unlock()
	if running {
		return false // 已经连着（比如刚扫完码），不要重复拉起
	}
	slog.Info("peerlink edge session restored, reconnecting", "hub", hubHostOf(sess.Hub))
	s.startEdgeLocked(sess.Hub, sess.PeerID, sess.DeviceID, sess.Token)
	return true
}

// saveEdgeSession 记住本端作为 Edge 的会话（扫码配对成功后调用）。
func (s *Server) saveEdgeSession(hub, peerID, deviceID, token string) {
	if !s.peerStore.Enabled() {
		return
	}
	if err := s.peerStore.SaveEdgeSession(peerlink.EdgeSession{
		Hub:      hub,
		Token:    token,
		PeerID:   peerID,
		DeviceID: deviceID,
		Platform: runtimeGOOS(),
	}); err != nil {
		slog.Warn("peerlink edge session save failed", "error", err.Error())
	}
}

// clearEdgeSession 忘记本端作为 Edge 的会话（解配 / 主动断开）。
func (s *Server) clearEdgeSession() {
	if !s.peerStore.Enabled() {
		return
	}
	if err := s.peerStore.ClearEdgeSession(); err != nil {
		slog.Warn("peerlink edge session clear failed", "error", err.Error())
	}
}
