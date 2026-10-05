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

// MethodBundleRollback 是 Hub → Edge 的**云控回滚**方法名（2026-10-05）。
//
// 为什么要有它：设备端此前只有 `RollbackFile` 这个**本地**函数（"装坏了自动退回"），
// 云端**没有任何一键回滚的入口** ⇒ 一次坏包下发后，运维只能再推一个旧版本号
// （要重新打包 + 重新走一遍下载/校验/切换），设备在几分钟里一直是坏的。
// 回滚指令只有几十字节，与 bundle_update 同一条控制面。
const MethodBundleRollback = "bundle_rollback"

// MethodBundleReload 是 Hub → Edge 的**云控重载/重启**方法名（2026-10-06）。
//
// 为什么要有它：热更包应用后，此前**只能靠用户手动重启 App** 才生效
// （Kotlin 的 applyHotWebBundleIfPresent 只在启动时判定）⇒ 真机联调时
// "推完还要人手点一次" 既慢又容易让人以为没生效。
//
// 三级（由轻到重，云控按需要选择）：
//
//	web      —— 只让 WebView 重新加载资源（最快，用户几乎无感）
//	activity —— recreate 当前 Activity（重建页面与插件桥）
//	app      —— 整个 App 进程重启（最彻底，用于换执行体/换 Go 二进制）
const MethodBundleReload = "bundle_reload"

// 三级重载的合法取值。
const (
	ReloadLevelWeb      = "web"
	ReloadLevelActivity = "activity"
	ReloadLevelApp      = "app"
)

// BundleReloadRequest 云控下发的一次重载/重启指令。
type BundleReloadRequest struct {
	// Level 重载级别：web / activity / app。空串按 web 处理（最轻）。
	Level string `json:"level,omitempty"`
	// Reason 可选：为什么要重载（进台账/日志，便于事后解释"谁让设备重启了"）。
	Reason string `json:"reason,omitempty"`
}

// BundleReloadResult 执行端回报的一次重载结果。
type BundleReloadResult struct {
	Ok bool `json:"ok"`
	// Level 实际执行的级别（可能与请求的级别不同，例如不支持 app 级时降级）。
	Level string `json:"level,omitempty"`
	// Applied 是否**当场**就生效了（web 级=true；activity/app 级需要 Kotlin
	// 侧接管，Go 只能落指令 ⇒ false，但不算失败）。
	Applied bool `json:"applied,omitempty"`
	// Error 失败原因（**脱敏**：不得含设备绝对路径）。
	Error string `json:"error,omitempty"`
	// Rejected 请求本身不成立（未知 level / 本端不支持该级别）。
	Rejected bool `json:"rejected,omitempty"`
}

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
	// ABI 目标架构（arm64-v8a / armeabi-v7a / x86_64…）。
	//
	//	只有 **go-binary（换执行体）** 需要它：执行端会拒绝没有 ABI 的二进制更新，
	//	Kotlin 侧启动前还要与 Build.SUPPORTED_ABIS[0] 比对 ——
	//	把 arm64 的包装到别的架构上 = CANNOT LINK EXECUTABLE = 设备起不来后端。
	ABI string `json:"abi,omitempty"`
}

// BundleRollbackRequest 云控下发的一次**回滚**指令。
type BundleRollbackRequest struct {
	// Name 包名：web / preview-assets / go-binary。
	Name string `json:"name"`
	// Version 期望回滚到的版本（可空：执行端只有"上一版备份"这一份，以它为准）。
	// 留着是为了让云控台账能记"从哪版退到哪版"，也是未来多版本备份的扩展位。
	Version string `json:"version,omitempty"`
}

// BundleRollbackResult 执行端回报的一次回滚结果。
type BundleRollbackResult struct {
	Ok   bool   `json:"ok"`
	Name string `json:"name"`
	// Version 回滚后**生效**的版本（回滚成功时才有意义）。
	Version string `json:"version,omitempty"`
	// PreviousVersion 被撤掉的那版（"坏包"的版本号，云控台账要记）。
	PreviousVersion string `json:"previousVersion,omitempty"`
	// Error 失败原因（**脱敏**：不得含设备绝对路径）。
	Error string `json:"error,omitempty"`
	// Rejected 表示"这个请求本身不成立"（包名不认识 / 没有备份可退）
	//
	//	⇒ 与 bundle_update 同一套纪律：发起端给 **400** 且**不计入熔断**
	//	（"没备份可退"不是设备故障，把它算成故障会连坐后续所有云控指令）。
	Rejected bool `json:"-"`
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
