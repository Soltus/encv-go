package peerlink

// capabilities.go —— 受控端**能力自省**（vNext Round 12）
//
// 为什么要有它（2026-10-06 教训，血的）：
//
//	此前云控侧全靠**硬编码约定**去猜设备能做什么：
//	  · 包名/文件名必须是 "opencv-go"（写错 ⇒ missing required files）
//	  · ABI 必须是 "arm64-v8a"（写错 ⇒ missing_abi）
//	  · 版本靠 ldflags 注入（变量写错大小写 ⇒ 静默失效，永远 dev）
//	  · 支持哪些方法、哪些重载级别 —— Hub 侧根本没有途径知道
//
//	这些约定**只有掌握全部上下文的人（agent）才说得对**，换个设备、换个构建、
//	换个人维护就崩 ⇒ 用户评价："一堆脆弱的校验 / 自导自演"。
//
// 正解：**能力协商** —— Hub 不问"你应该是啥"，而是问"你是啥、你能做啥"，
// 受控端自己回答。云控据此决策：不支持就直接说不支持，UI 禁用按钮并给出原因，
// 而不是推一个注定失败的包、等真机报错。
//
// 设计原则：
//   - 能力**由执行端自报**，Hub 不做任何"按名字推断"
//   - 未知字段一律留空（表示"不知道"），绝不猜
//   - 版本/ABI 等"身份信息"与"能力清单"分开：身份错了是设备问题，能力缺失是版本问题

// MethodPeerCapabilities 是 Hub → Edge 的**能力查询**方法名。
const MethodPeerCapabilities = "peer_capabilities"

// PeerCapabilitiesRequest 能力查询请求（无参数，留作扩展）。
type PeerCapabilitiesRequest struct {
	// 未来可加：只查某类能力
}

// BundleCap 某个资源包在本端的状态（自报，不是 Hub 猜的）。
type BundleCap struct {
	Name string `json:"name"`
	// Installed 本端**是否真的装了**（判定与 Kotlin 一致：目录/文件 + version.json）
	Installed bool `json:"installed"`
	// Version 本端生效版本（空串 = 没装或不知道）
	Version string `json:"version,omitempty"`
	// Target 本端落地路径（**脱敏**：Hub 侧只用于诊断，不含密钥）
	Target string `json:"target,omitempty"`
	// Rollable 有没有上一版可退
	Rollable bool `json:"rollable"`
}

// PeerCapabilities 受控端自报的能力清单。
type PeerCapabilities struct {
	// ── 身份（"我是谁"）──────────────────────────────────────────
	// BinaryVersion 本端运行中的 Go 二进制版本（构建注入）。
	// ⚠️ 恒为 "dev" 说明**版本注入没生效**，此时 Hub 不能据此判断跑的是哪个包。
	BinaryVersion string `json:"binaryVersion,omitempty"`
	// Abi 本端主 ABI（Android: arm64-v8a / armeabi-v7a / x86_64）。
	Abi string `json:"abi,omitempty"`
	Goos   string `json:"goos,omitempty"`
	Goarch string `json:"goarch,omitempty"`
	// PeerId 本端 peer 标识
	PeerId string `json:"peerId,omitempty"`
	// UptimeMs 本端进程已运行毫秒（用于判断"是不是刚重启过"）
	UptimeMs int64 `json:"uptimeMs,omitempty"`

	// ── 能力（"我能做啥"）────────────────────────────────────────
	// Methods 本端**实际接线**的 RPC 方法（来自 Edge.Supports，不是声明式常量）
	Methods []string `json:"methods"`
	// ReloadLevels 本端支持的重载级别（web / activity / app）
	ReloadLevels []string `json:"reloadLevels,omitempty"`
	// Bundles 各资源包在本端的真实状态
	Bundles []BundleCap `json:"bundles,omitempty"`

	// ── 自省完整性（诚实标注"我不知道"）──────────────────────────
	// Unknowns 本端无法确定的项（如 Abi 取不到）⇒ Hub 必须**显式提示**而不是当已知
	Unknowns []string `json:"unknowns,omitempty"`
}

// PeerCapabilitiesResult 能力查询的回报。
type PeerCapabilitiesResult struct {
	Ok bool `json:"ok"`
	// Caps 能力清单（Ok=false 时为 nil）
	Caps *PeerCapabilities `json:"caps,omitempty"`
	// Error 失败原因（脱敏）
	Error string `json:"error,omitempty"`
}
