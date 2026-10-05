package server

// peerlink_edge_runtime.go —— P2a Task 2.2：**把 Edge 挂进 Go 进程**
//
// 角色：本端（手机/另一台设备）扫 Hub 的二维码后，作为 Edge **主动出网**连上去并常驻。
// 长连接放 Go 侧（E2 决策）：息屏 / 后台 / Activity 重建都不影响链路（R7/R9），
// Go 进程由 EncvGoService（已是前台服务）承载。
//
// ⚠️ 存储纪律：token / psk **只存进程内存**，不落盘（Task 5.2）。
//    ⇒ 进程重启后必须重新扫码配对（安全优先，与"信任态重启失效"同一套语义）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/bundle"
	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/fts"
	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// ── Edge 运行时状态 ─────────────────────────────────────────────────

type edgeRuntime struct {
	hubURL    string
	peerID    string
	deviceID  string
	startedAt time.Time
	cancel    context.CancelFunc
	edge      *peerlink.Edge
	lastErr   string
}

// peerEdgeHandlers 允许测试/宿主覆盖 Edge 的三个处理器（默认走本端真实实现）。
type peerEdgeHandlers struct {
	OnSearch      func(peerlink.SearchRequest) (json.RawMessage, error)
	OnRead        func(peerlink.ReadRequest) ([]byte, int64, error)
	OnAgentInvoke func(peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome
	// OnBundleUpdate 云控热更新（2026-10-04）：Hub 下发指令 → 本端拉包 → 原子生效/回滚
	OnBundleUpdate func(peerlink.BundleUpdateRequest) peerlink.BundleUpdateResult
	// OnBundleRollback 云控回滚（2026-10-05）：把上一版备份搬回来
	OnBundleRollback func(peerlink.BundleRollbackRequest) peerlink.BundleRollbackResult
	// OnBundleReload 云控重载/重启（2026-10-06）：web / activity / app 三级
	OnBundleReload func(peerlink.BundleReloadRequest) peerlink.BundleReloadResult
}

func (s *Server) edgeHandlers() peerEdgeHandlers {
	if s.peerEdgeHandlersOverride != nil {
		return *s.peerEdgeHandlersOverride
	}
	return peerEdgeHandlers{
		OnSearch:       s.peerLocalSearch,
		OnRead:         s.peerLocalRead,
		OnAgentInvoke:    s.PeerAgentInvokeHandler,
		OnBundleUpdate:   s.peerLocalBundleUpdate,
		OnBundleRollback: s.peerLocalBundleRollback,
		OnBundleReload:   s.peerLocalBundleReload,
	}
}

// ── 云控热更新：**执行端**（2026-10-04）────────────────────────────────
//
// bundleTargets 是「包名 → 本端可写目录」的映射。只有登记在册的包才允许被云控覆盖，
// 且目录必须是**应用私有可写目录**（不在 APK / Go 二进制内）—— 否则"热更新"就是空话。
// bundleTargetDirFor / bundleTargetFileFor 是「包名 → 目标」的**纯函数**版本。
//
// 为什么要有：本端热更状态（/bundle/local）与回滚都要按同一份清单扫描，
// 而 Server 方法在非 Server 上下文（以及未来的纯函数单测）里拿不到 ⇒ 抽成包级函数，
// 方法版只是转发（避免"清单写两份"的漂移）。
func bundleTargetDirFor(name string) (string, bool) {
	switch name {
	case "preview-assets":
		// 已验证过的先例：容器预览页，整包替换即生效，不需要换 APK、也不需要重启
		return previewAssetsDir(), true
	case "web":
		// 主应用 SPA（I4）：目录先就位，WebView 改从本目录加载后即可热更主界面
		return config.AppDataDir("web-bundle"), true
	default:
		return "", false
	}
}

func bundleTargetFileFor(name string) (string, bool) {
	if name != "go-binary" {
		return "", false
	}
	files := strings.TrimSpace(os.Getenv("ENCV_APP_FILES_DIR"))
	if files == "" {
		return "", false
	}
	return filepath.Join(files, "encv-go"), true
}

func (s *Server) bundleTargetDir(name string) (string, bool) { return bundleTargetDirFor(name) }

// bundleTargetFile 是「包名 → 单文件目标」的映射（I3：Go 二进制热更新）。
//
// ⚠️ 只有 go-binary 一个入口：换的是**执行体**，必须带 ABI 且可回滚，
//    绝不接受任意路径（否则一次云控就能把设备写成砖）。
func (s *Server) bundleTargetFile(name string) (string, bool) { return bundleTargetFileFor(name) }

// peerLocalBundleUpdate 执行一次云控下发的资源包更新。
//
// 纪律（与 bundle.Apply 的不变式一致）：
//   - 包名不认识 / 没带 sha256 ⇒ **请求本身不对** ⇒ Rejected（发起端 400，不计熔断）
//   - 下载/校验/切换任一步失败 ⇒ 回滚到上一版，旧资源继续可用（绝不留下半新半旧）
//   - 回报文本脱敏：不含设备绝对路径
func (s *Server) peerLocalBundleUpdate(req peerlink.BundleUpdateRequest) (out peerlink.BundleUpdateResult) {
	out.Name = req.Name

	isFileTarget := false
	target, ok := s.bundleTargetDir(req.Name)
	if !ok {
		if ft, ok2 := s.bundleTargetFile(req.Name); ok2 {
			target, isFileTarget, ok = ft, true, true
		}
	}
	if !ok {
		out.Rejected = true
		out.Error = "unknown_bundle:" + sanitizeBundleName(req.Name)
		return out
	}
	if strings.TrimSpace(req.SHA256) == "" {
		out.Rejected = true
		out.Error = "missing_sha256"
		return out
	}
	// 换执行体必须声明 ABI —— Kotlin 侧会比对 Build.SUPPORTED_ABIS[0]，缺了它设备会拒绝使用
	if isFileTarget && strings.TrimSpace(abiOfArgs(req)) == "" {
		out.Rejected = true
		out.Error = "missing_abi"
		return out
	}

	// 数据面：基于本端已知的会合点地址拼下载 URL（Hub 不占控制连接传大数据）
	s.peerEdgeMu.Lock()
	rt := s.peerEdge
	s.peerEdgeMu.Unlock()
	if rt == nil || rt.edge == nil {
		out.Error = "edge_not_running"
		return out
	}
	hub := rt.edge.HubURL()
	token := rt.edge.Token()
	if hub == "" || token == "" {
		out.Error = "edge_not_connected"
		return out
	}
	dl := strings.TrimRight(hub, "/") + "/bundle/download?name=" + url.QueryEscape(req.Name) +
		"&version=" + url.QueryEscape(req.Version) + "&token=" + url.QueryEscape(token)

	tmpDir := config.AppDataDir("tmp")
	zipPath := filepath.Join(tmpDir, req.Name+"-"+req.Version+".zip")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		out.Error = "tmp_unavailable"
		return out
	}
	defer os.Remove(zipPath) // 无论成败都不留 zip（内容已落到目标目录/staging）

	ctx, cancel := context.WithTimeout(context.Background(), peerlink.BundleCallTimeout)
	defer cancel()

	slog.Info("peerlink bundle update begin", "name", req.Name, "version", req.Version)
	if err := bundle.Download(ctx, dl, zipPath, bundleMaxZipBytes, peerlink.BundleCallTimeout); err != nil {
		out.Error = "download_failed:" + truncateBundleErr(err.Error())
		return out
	}

	opts := bundle.Options{MaxBytes: bundleMaxZipBytes, Timeout: peerlink.BundleCallTimeout}
	var res bundle.Result
	var err error
	if isFileTarget {
		// I3：换**执行体**（Go 二进制）—— 原子 rename 覆盖 + sidecar（version/abi）
		res, err = bundle.ApplyFile(bundle.FileSpec{
			Name:     req.Name,
			Target:   target,
			Version:  req.Version,
			ABI:      strings.TrimSpace(abiOfArgs(req)),
			SHA256:   req.SHA256,
			Required: req.Required,
		}, zipPath, opts)
	} else {
		res, err = bundle.Apply(bundle.Spec{
			Name:      req.Name,
			TargetDir: target,
			Version:   req.Version,
			SHA256:    req.SHA256,
			Required:  req.Required,
		}, zipPath, opts)
	}
	if err != nil {
		// ⚠️ 切换/校验失败 ⇒ 必须回滚：目标此刻可能已被移走（Apply 内部已尽力恢复，
		//    这里再兜一次），回滚后旧版继续服务（目录型＝不白屏，文件型＝后端还能起）。
		var rerr error
		if isFileTarget {
			rerr = bundle.RollbackFile(req.Name, target)
		} else {
			rerr = bundle.Rollback(req.Name, target)
		}
		if rerr == nil {
			out.RolledBack = true
		}
		out.Error = "apply_failed:" + truncateBundleErr(err.Error())
		slog.Warn("peerlink bundle update failed, rolled back", "name", req.Name, "version", req.Version, "error", err.Error())
		return out
	}

	out.Ok = true
	out.AppliedVersion = res.Version
	out.PreviousVersion = res.PreviousVersion
	if isFileTarget {
		// 换的是执行体：rename 后**运行中的进程仍持有旧 inode** ⇒ 必须重启进程才生效。
		// Kotlin 侧下次启动（APP 冷启动 / 服务重启）即自动使用新二进制。
		slog.Info("peerlink bundle update applied (binary; requires process restart to take effect)",
			"name", req.Name, "version", res.Version, "prev", res.PreviousVersion)
		return out
	}
	slog.Info("peerlink bundle update applied", "name", req.Name, "version", res.Version, "prev", res.PreviousVersion)
	return out
}

// peerLocalBundleRollback 执行一次云控下发的**回滚**（2026-10-05）。
//
// 与"更新失败自动回滚"的区别：那是安装器内部的自保动作；这是**云控主动发起**的
// 一键回滚 —— 一次坏包下发之后，运维要能在几秒内把设备退回上一版，
// 而不是重新打一个旧版本包再走一遍下载/校验/切换。
//
// 语义：
//   - 包名不认识 / **没有备份可退** ⇒ Rejected（发起端 400，不计熔断）
//   - 目录型（web / preview-assets）：整目录搬回上一版，版本 = 备份里的 version.json
//   - 文件型（go-binary）：搬回备份二进制，并**删除 sidecar** ⇒ 回到 APK 内置二进制
func (s *Server) peerLocalBundleRollback(req peerlink.BundleRollbackRequest) (out peerlink.BundleRollbackResult) {
	out.Name = req.Name

	isFileTarget := false
	target, ok := s.bundleTargetDir(req.Name)
	if !ok {
		if ft, ok2 := s.bundleTargetFile(req.Name); ok2 {
			target, isFileTarget, ok = ft, true, true
		}
	}
	if !ok {
		out.Rejected = true
		out.Error = "unknown_bundle:" + sanitizeBundleName(req.Name)
		return out
	}

	// 先记下"被撤掉的是哪版"（云控台账要能回答"从哪版退下来的"）
	out.PreviousVersion = localBundleVersion(target, isFileTarget)

	if isFileTarget {
		if err := bundle.RollbackFile(req.Name, target); err != nil {
			out.Rejected = true
			out.Error = "no_backup"
			return out
		}
		// ⚠️ 换执行体的回滚 = 退回 **APK 内置**二进制。
		//    `.version` / `.abi` 是 Kotlin 判定"这是热更通道放进去的二进制"的依据
		//    （见 EncvGoService.findExecutableBinary）⇒ 必须一起删掉，
		//    否则 Kotlin 会继续用 filesDir 里那个"自称某版本"的回滚件，
		//    而它其实已经是备份件 ⇒ 版本显示与实际执行体不一致。
		_ = os.Remove(target + ".version")
		_ = os.Remove(target + ".abi")
		out.Version = "apk"
	} else {
		if err := bundle.Rollback(req.Name, target); err != nil {
			out.Rejected = true
			out.Error = "no_backup"
			return out
		}
		out.Version = bundle.CurrentVersion(target)
	}

	out.Ok = true
	slog.Info("peerlink bundle rollback applied", "name", req.Name, "from", out.PreviousVersion, "to", out.Version)
	return out
}

// localBundleVersion 读本端某个包**当前生效**的版本（未安装返回空串）。
func localBundleVersion(target string, isFile bool) string {
	if isFile {
		return strings.TrimSpace(readTextSidecar(target + ".version"))
	}
	return bundle.CurrentVersion(target)
}

// abiOfArgs 取出下发指令里的架构声明（go-binary 必填）。
func abiOfArgs(req peerlink.BundleUpdateRequest) string {
	return strings.TrimSpace(req.ABI)
}

func sanitizeBundleName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

// truncateBundleErr 回报文本上限（防止把设备绝对路径 / 栈信息整段回传）。
func truncateBundleErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

// ── 本端真实实现（被对端查询时执行）────────────────────────────────

// peerLocalSearch 对端查询**本端**索引（复用 FTS5 全文索引，与 /api/files/search-fulltext 同源）。
func (s *Server) peerLocalSearch(req peerlink.SearchRequest) (json.RawMessage, error) {
	idx := GetFullTextIndex()
	if idx == nil {
		return nil, errors.New("fulltext_unavailable")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200 // R13：控制面小报文
	}
	res, err := idx.Search(context.Background(), req.Q, fts.SearchOptions{Limit: limit, IncludeDirs: false})
	if err != nil {
		// 2026-10-04：查询表达式本身不合法（空查询 / 无效 regex / 括号不配对等）属于
		// **调用方的问题**，必须标 rejected ⇒ 发起端给 400 且**不计入熔断**；
		// 否则几次输错关键词就把 peer 熔断掉，连坐所有合法调用（与 bad_path 同一类坑）。
		if errors.Is(err, fts.ErrInvalidQuery) {
			return nil, fmt.Errorf("%w: bad_query", peerlink.ErrPeerRejected)
		}
		return nil, err
	}
	items := make([]map[string]any, 0, len(res))
	for _, r := range res {
		items = append(items, map[string]any{
			"path":  r.Path,
			"name":  r.Name,
			"score": r.Score,
		})
	}
	// 🆕 2026-10-04：把**本端索引状态**一起回给发起端。
	//
	//	背景（真机事故）：手机文件量大时索引构建会在 10 分钟超时中断 ⇒ 库里要么全空、
	//	要么只有部分 ⇒ 联邦搜索返回 0 条，而 UI 上和"真的没有匹配"长得一模一样，
	//	用户（和我们排查时）完全无法区分 ⇒ 长期当成"搜索坏了"却查不出原因。
	//
	//	四个状态：
	//	  building —— 正在后台构建，结果会**逐渐变全**
	//	  empty    —— 一条都没索引（可能还在扫 / 构建失败 / 根本没有文件）
	//	  partial  —— 有数据但**从未完整构建完**（被超时打断过），结果可能不全
	//	  ready    —— 完整构建过（MarkBuilt 更新过 IndexedAt）
	indexState := ftsIndexState(idx)

	return json.Marshal(map[string]any{
		"items":        items,
		"indexState":   indexState,
		"indexedFiles": idx.Stats().TotalFiles,
	})
}

// ftsIndexState 由索引统计推出 federated search 需要暴露的状态。
//
// 判据说明：IndexedAt 只有 **完整** 构建完成（idx.MarkBuilt）才写入，
// 部分完成（idx.MarkPartial）刻意不写 ⇒ 这里是区分 partial / ready 的唯一依据。
func ftsIndexState(idx *fts.FileIndex) string {
	st := idx.Stats()
	switch {
	case st.IsIndexing:
		return "building"
	case st.TotalFiles == 0:
		return "empty"
	case st.IndexedAt == "":
		return "partial"
	default:
		return "ready"
	}
}

// peerLocalRead 对端读**本端**文件分片（在线打开 / 缩略图，R13 有上限）。
func (s *Server) peerLocalRead(req peerlink.ReadRequest) ([]byte, int64, error) {
	// ⚠️ 2026-10-04：路径非法 / 打不开这类"请求本身不对"的失败，必须包装成
	// peerlink.ErrPeerRejected —— 否则发起端会当成"对端故障"计入熔断，
	// 几次误传参数就把 peer 熔断掉，连坐所有**合法**请求（真机实测 2 次即开路）。
	abs, err := s.resolveUserPath(req.Path)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: bad_path", peerlink.ErrPeerRejected)
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: open_failed", peerlink.ErrPeerRejected)
	}
	defer f.Close()

	// 源文件总长度（2026-10-04：发起端要靠它表达 Range 语义）。
	// stat 失败不该让整次读操作失败 ⇒ 退化成 0（发起端会据此不谎称自己读完了）。
	var total int64
	if st, statErr := f.Stat(); statErr == nil {
		total = st.Size()
	}

	n := req.Length
	if n <= 0 {
		n = peerlink.DefaultReadChunk
	}
	if n > peerlink.MaxReadChunk {
		n = peerlink.MaxReadChunk
	}
	buf := make([]byte, n)
	if req.Offset > 0 {
		if _, err := f.Seek(req.Offset, io.SeekStart); err != nil {
			return nil, total, errors.New("seek_failed")
		}
	}
	read, err := io.ReadFull(f, buf)
	if err != nil && (err != io.ErrUnexpectedEOF && err != io.EOF) {
		return nil, total, errors.New("read_failed")
	}
	// ⚠️ 错误文本不含真实路径（R14 脱敏）
	return buf[:read], total, nil
}

// ── 远端配对（扫码后调用）──────────────────────────────────────────

type edgePairBody struct {
	Hub       string `json:"hub"`       // Hub 基址（二维码里带，必须是 https，R3）
	PairingID string `json:"pairingId"` // 二维码里的秘密
	PSK       string `json:"psk"`       // 二维码里的 psk（hex，与票据 PSKHex 同编码）
	DeviceID  string `json:"deviceId"`
	Name      string `json:"name"`
}

// validateHubURL R3：跨端地址**禁止明文 http**（混合内容 + 明文泄露）。
// 仅允许 https，或本机回环（开发/自测）。
func validateHubURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", errors.New("invalid_hub")
	}
	if u.Scheme == "https" {
		return strings.TrimRight(u.String(), "/"), nil
	}
	host := u.Hostname()
	if u.Scheme == "http" && (host == "127.0.0.1" || host == "localhost" || host == "::1") {
		return strings.TrimRight(u.String(), "/"), nil
	}
	return "", errors.New("hub_must_be_https")
}

// hubBaseURL 归一出 Hub 的**基址**（scheme://host[:port]）。
//
// ⚠️ 真 bug（2026-10-03 抓出）：票据里的 `hub` 是**带路径**的
// （`https://host/api/peerlink`，见 handlePeerlinkTicket），而 pairToRemoteHub 又会
// 拼上 `/api/peerlink/pair` ⇒ 双前缀 `.../api/peerlink/api/peerlink/pair` ⇒ **404**
// （扫码配对在真机上直接失败：`pair_rejected:404`）。
// 这里统一把已带的 peerlink 前缀剥掉，两种写法都能用。
func hubBaseURL(raw string) string {
	h := strings.TrimRight(strings.TrimSpace(raw), "/")
	h = strings.TrimSuffix(h, "/api/peerlink")
	return strings.TrimRight(h, "/")
}

// hubHostOf 只取会合点主机用于日志（票据/地址里不落 token 等密钥到日志）。
func hubHostOf(hub string) string {
	u, err := url.Parse(hub)
	if err != nil {
		return "(unparsable)"
	}
	return u.Host
}

// pairRejectError 远端 Hub 拒绝了配对（HTTP 非 200）。
//
// 2026-10-05：把远端的**错误码**一并带回来（如 ticket_expired / ticket_used /
// ticket_not_found），否则"/edge/pair 502" 对手机端 UI 就是一团黑盒。
type pairRejectError struct {
	status int
	code   string // 远端响应的 error 字段（可能为空 = 旧版后端）
	detail string
}

func (e *pairRejectError) Error() string {
	if e.code != "" {
		return fmt.Sprintf("pair_rejected:%d:%s", e.status, e.code)
	}
	return fmt.Sprintf("pair_rejected:%d", e.status)
}

// RefreshQr 该失败是否属于"票据类"（用户刷新二维码重扫即可解决）。
func (e *pairRejectError) IsTicketFailure() bool {
	switch e.code {
	case "ticket_expired", "ticket_used", "ticket_not_found":
		return true
	}
	return false
}

// remotePairCode 从远端响应体里取 error 字段（取不到返回空串，向后兼容旧后端）。
func remotePairCode(raw []byte) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.Error)
}

// pairToRemoteHub 拿票据去远端 Hub 完成配对（proof = HMAC(psk, pairingId, deviceId)）。
//
// ⚠️ 第三个返回值 sas 不能丢（2026-10-04）：远端 /api/peerlink/pair 会回 SAS 6 位安全码，
//    扫码端（手机）要**显示**它给人工核对 —— 桌面端在等"一致，信任该设备"，
//    手机端却什么都不显示 ⇒ 用户根本不知道该拿什么去比对（真机反馈：困惑）。
func pairToRemoteHub(hub, pairingID, pskB64, deviceID, name string) (peerID string, token string, sas string, err error) {
	psk, err := peerlink.DecodePSK(pskB64)
	if err != nil {
		return "", "", "", fmt.Errorf("bad_psk: %w", err)
	}
	body, _ := json.Marshal(map[string]string{
		"pairingId": pairingID,
		"deviceId":  deviceID,
		"name":      name,
		"platform":  runtime.GOOS,
		"proof":     peerlink.Proof(psk, pairingID, deviceID),
	})
	resp, err := http.Post(hubBaseURL(hub)+"/api/peerlink/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", "", fmt.Errorf("pair_request_failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// ⚠️ 2026-10-05：失败**原因**必须带回去。
		//    旧实现只回 `pair_rejected:401` ⇒ 手机端 UI 只知道"失败了"，
		//    不知道该让用户"刷新二维码重扫"还是"检查网络"，真机上就是一句没用的
		//    "连接失败"（真机事故里用户拿着过期码反复扫，界面毫无引导）。
		return "", "", "", &pairRejectError{status: resp.StatusCode, code: remotePairCode(raw), detail: strings.TrimSpace(string(raw))}
	}
	var out struct {
		PeerID string `json:"peerId"`
		Token  string `json:"token"`
		SAS    string `json:"sas"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Token == "" {
		return "", "", "", errors.New("bad_pair_response")
	}
	return out.PeerID, out.Token, out.SAS, nil
}

// ── HTTP API（本端 UI / 扫码后用）──────────────────────────────────

// handlePeerlinkEdgePair —— POST /api/peerlink/edge/pair
func (s *Server) handlePeerlinkEdgePair(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body edgePairBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	hub, err := validateHubURL(body.Hub)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_hub", "message": "Hub 地址必须是 https（本机回环除外）"})
		return
	}
	if strings.TrimSpace(body.PairingID) == "" || strings.TrimSpace(body.PSK) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "pairingId 与 psk 必填"})
		return
	}
	deviceID := strings.TrimSpace(body.DeviceID)
	if deviceID == "" {
		deviceID = s.peerHub.Info()["peerId"]
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = s.peerHub.Info()["name"]
	}

	peerID, token, sas, err := pairToRemoteHub(hub, body.PairingID, body.PSK, deviceID, name)
	if err != nil {
		// 2026-10-04：扫码端配对失败必须进后端日志（此前整个 peerlink 零日志，
		//   用户只能看到"连接中…"，DevLogs 里一条相关信息都没有）
		slog.Warn("peerlink edge pair failed", "hub", hubHostOf(hub), "detail", err.Error())
		// 2026-10-05：票据类失败要**单独标记** ⇒ 前端才能提示"刷新二维码重扫"，
		//   而不是把"码过期了"和"连不上会合点"混成一句"连接失败"。
		var re *pairRejectError
		if errors.As(err, &re) && re.IsTicketFailure() {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": "pair_failed", "reason": re.code, "refreshQr": true, "detail": err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "pair_failed", "detail": err.Error()})
		return
	}

	slog.Info("peerlink edge paired, connecting", "hub", hubHostOf(hub), "peerId", peerID)
	s.startEdgeLocked(hub, peerID, deviceID, token)
	// 记住这次配对（2026-10-04）：Go 进程重启 / APP 冷启动后**自动重连**，不用再扫一次码
	s.saveEdgeSession(hub, peerID, deviceID, token)

	// sas 回给扫码端 UI（PeerScanPanel）显示，供双端人工核对；
	// 不落盘、不进日志（安全码只在配对会话内有效）。
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"hub":     hub,
		"peerId":  peerID,
		"sas":     sas,
		"running": true,
	})
}

// startEdgeLocked 启动（或重启）Edge 长连接。token 只存内存。
func (s *Server) startEdgeLocked(hub, peerID, deviceID, token string) {
	s.peerEdgeMu.Lock()
	defer s.peerEdgeMu.Unlock()

	// 重复配对：先停掉旧的，避免双连接
	if s.peerEdge != nil && s.peerEdge.cancel != nil {
		s.peerEdge.cancel()
	}

	h := s.edgeHandlers()
	ctx, cancel := context.WithCancel(context.Background())
	// ⚠️ 必须先用 hubBaseURL 归一化（2026-10-04 真因修复）：
	//   票据里的 hub **已经自带** `/api/peerlink` 后缀（如 http://host/api/peerlink），
	//   这里再拼一次 ⇒ Edge 去连 `/api/peerlink/api/peerlink/ws`（双前缀）
	//   ⇒ 404 / bad handshake ⇒ 手机端永远"连接中…"、桌面端显示离线
	//   ⇒ 表现就是"信任设备后还是连接失败"。
	hubNormalized := hubBaseURL(hub) + "/api/peerlink"
	edge := peerlink.NewEdge(peerlink.EdgeOptions{
		HubURL:        hubNormalized,
		Token:         token,
		DeviceID:      deviceID,
		OnSearch:      h.OnSearch,
		OnRead:        h.OnRead,
		OnAgentInvoke: h.OnAgentInvoke,
		// ⚠️ 2026-10-05：这行**不能漏**。此前只把 OnBundleUpdate 加进了
		//    peerEdgeHandlers 映射与 edgeHandlers() 默认值，却没在这里传给 Edge
		//    ⇒ 设备端收到云控 bundle_update 一律回 "not_supported"
		//    （真机首测才暴露：Hub 侧 push 得到 502 + remote error: not_supported）。
		//    同一个坑对 bundle_rollback 一样成立 ⇒ 有 `Supports()` 可断言，别再靠猜。
		OnBundleUpdate:   h.OnBundleUpdate,
		OnBundleRollback: h.OnBundleRollback,
		OnBundleReload:   h.OnBundleReload,
	})
	rt := &edgeRuntime{
		hubURL:    hubNormalized,
		peerID:    peerID,
		deviceID:  deviceID,
		startedAt: time.Now(),
		cancel:    cancel,
		edge:      edge,
	}
	s.peerEdge = rt
	go edge.Start(ctx)
}

// edgeStaleAfterAttempts 连续重连失败多少次后判定"会合点**可能已经不存在了**"。
//
// 2026-10-05：与 baseUrl 的 16666 是**同一类**风险——持久化下来的值（这里是通过扫码
//   记住的会合点地址）失效后，程序只会一遍遍重试，既不报错到能看懂，也不给用户出路。
//   Hub 侧的表现是"这台设备零请求"（它在连一个到不了的地址），运维只看到 offline。
//   ⚠️ CNB / 开发环境的域名会随容器重建变化 ⇒ 这种"地址还在、服务没了"是常态，不是异常。
const edgeStaleAfterAttempts = 5

// edgeSessionStale 判定一个 Edge 会话是否已"僵死"（连不上且重试多次仍失败）。
//
// 判据保守：**只在明确连不上且重试够多次时才置 true** —— 手机没网 / 后端刚重启
// 这类"暂时连不上"不能劝用户重新扫码（那会把一次抖动放大成一次重新配对）。
func edgeSessionStale(connected bool, attempts int) bool {
	return !connected && attempts >= edgeStaleAfterAttempts
}

// handlePeerlinkEdgeStatus —— GET /api/peerlink/edge/status
func (s *Server) handlePeerlinkEdgeStatus(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	s.peerEdgeMu.Lock()
	rt := s.peerEdge
	s.peerEdgeMu.Unlock()
	if rt == nil {
		c.JSON(http.StatusOK, gin.H{"running": false})
		return
	}
	connected := rt.edge != nil && rt.edge.Online()
	// ⚠️ 失败原因必须回给 UI（2026-10-04）：running=true 但 connected=false 时，
	//    没有 lastErr 前端只能显示"连接中…"，永远看不出是地址不通还是代理不支持 WS 升级。
	lastErr := ""
	attempts := 0
	if rt.edge != nil {
		lastErr = rt.edge.LastError()
		attempts = rt.edge.Attempts()
	}
	c.JSON(http.StatusOK, gin.H{
		"running":   true,
		"hub":       rt.hubURL,
		"peerId":    rt.peerID,
		"connected": connected,
		"lastErr":   lastErr,
		"attempts":  attempts,
		// 2026-10-05：会合点地址可能已失效（如域名随容器重建变了）。
		// UI 据此从"连接中…"改成"这里可能已经不存在了，请重新扫码"并给出忘记入口。
		"stale":     edgeSessionStale(connected, attempts),
		"startedAt": rt.startedAt,
	})
}

// handlePeerlinkEdgeStop —— POST /api/peerlink/edge/stop
func (s *Server) handlePeerlinkEdgeStop(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	s.peerEdgeMu.Lock()
	rt := s.peerEdge
	s.peerEdge = nil
	s.peerEdgeMu.Unlock()
	if rt != nil && rt.cancel != nil {
		rt.cancel()
	}
	// 主动断开 = 忘记这个会合点（否则下次启动又自动连回去）
	s.clearEdgeSession()
	c.JSON(http.StatusOK, gin.H{"ok": true, "running": false})
}
