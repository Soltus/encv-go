// bundle.go —— 云控热更新的**控制面**报文（2026-10-04）
//
// 定位：Hub（云）→ Edge（设备）只发**指令**（包名字 / 版本 / 摘要），
// 真正的**数据面**（zip 字节）走 Edge 主动出网的 HTTP GET
// （`<hubURL>/bundle/download?name=..&version=..&token=..`）。
//
// 为什么不把 zip 塞进 WS：R13 明确"控制面小报文"，一个 web 包几 MB～几十 MB，
// 走 WS 会长时间占住连接、且没有断点续传/Range；HTTP 还能顺带带上 token 鉴权。

package peerlink

import "time"

// MethodBundleUpdate 是 Hub → Edge 的 RPC 方法名（云控下发）。
const MethodBundleUpdate = "bundle_update"

// BundleCallTimeout 一次热更新的总超时（含"下载 + 解压 + 原子切换 + 回滚"）。
//
// 比 AgentCallTimeout(120s) 长得多：更新是分钟级动作（几十 MB 走移动网络）。
// 但仍要封顶 —— 无限等待会让云控侧永远看不到失败。
const BundleCallTimeout = 5 * time.Minute

// BundleUpdateRequest 云控下发的一次更新指令。
type BundleUpdateRequest struct {
	// Name 包名：preview-assets（容器预览页）/ web（主应用 SPA）/ ...
	Name string `json:"name"`
	// Version 目标版本；空串 = 由执行端自己判断"有没有更新"（预留）。
	Version string `json:"version"`
	// SHA256 期望摘要（十六进制）。执行端**必须**校验，不符直接拒绝。
	SHA256 string `json:"sha256"`
	// Size zip 字节数（给执行端做空间预检与进度）。
	Size int64 `json:"size,omitempty"`
	// Required 包内必须存在的文件（相对路径），用于挡住"能解开但内容不全"的半成品。
	Required []string `json:"required,omitempty"`
}

// BundleUpdateResult 执行端回报的一次更新结果。
type BundleUpdateResult struct {
	Ok   bool   `json:"ok"`
	Name string `json:"name"`
	// AppliedVersion 生效的版本（失败时为空）。
	AppliedVersion string `json:"appliedVersion,omitempty"`
	// PreviousVersion 被覆盖的上一版（回滚用得上）。
	PreviousVersion string `json:"previousVersion,omitempty"`
	// RolledBack 是否发生过回滚（true = 新包没生效，仍是上一版）。
	RolledBack bool `json:"rolledBack,omitempty"`
	// Error 失败原因（**脱敏**：不得含设备绝对路径）。
	Error string `json:"error,omitempty"`
	// Rejected 表示"这个请求本身不对"（包名不认识 / 没给摘要），而不是本端故障。
	//
	//	它**不过 wire**（执行端据此选 writeRejected，让发起端拿 400 且**不计入熔断**）——
	//	否则"云控推了一个不存在的包"会被当成"设备坏了"，几次就把 peer 熔断掉。
	Rejected bool `json:"-"`
}
