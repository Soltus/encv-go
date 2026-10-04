// internal/server/agent_diag_bridge.go —— 远程调试诊断工具（2026-10-04）
//
// 背景：远程调试（PeerLink 远程 Agent 调用）此前只有 5 个**文件系统**只读工具
// （list_mounts / list_files / read_file / stat_file / get_storage_info）。
// 真机排障时最先需要的两件事一件都拿不到：
//
//	① 对端现在是什么状态？（版本 / 平台 / 挂载点可用性 / 互联是否连上 / 索引建完没）
//	② 对端日志说了什么？（此前只能让对方在手机上翻 DevLogs 念给你听）
//
// 本文件补两个 needConfirm=false 的只读工具：get_device_info / read_logs。
// 它们与 fs 工具走完全相同的链路（执行端审批 → 幂等 → 审计 → 发起端契约），
// 只是用途是"看状态"而不是"看文件"。
//
// 脱敏纪律：
//   - 不返回挂载点的**物理绝对路径**（R14：手机绝对路径属内部细节，不该过 wire）
//   - 日志单条 message 截断到 2000 字符，条数上限 200（控制面小报文，R13）
//   - 互联失败原因 lastErr 保留 —— 它是"连不上"类故障的唯一线索，排障价值高于脱敏收益

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/logger"
)

// ─── 工具 schema ────────────────────────────────────────────────

var diagToolGetDeviceInfoSchema = map[string]interface{}{
	"type":       "object",
	"properties": map[string]interface{}{},
	"required":   []string{},
}

var diagToolReadLogsSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"limit": map[string]interface{}{
			"type":        "integer",
			"description": "最多返回多少条（默认 50，上限 200）",
			"default":     50,
			"minimum":     1,
			"maximum":     200,
		},
		"levels": map[string]interface{}{
			"type":        "array",
			"description": "按级别过滤（可多值，如 [\"warn\",\"error\"]；不传 = 不过滤）",
			"items":       map[string]interface{}{"type": "string", "enum": []string{"error", "warn", "info", "debug"}},
			"uniqueItems": true,
		},
		"level": map[string]interface{}{
			"type":        "string",
			"description": "单级别过滤（旧参数，与 levels 二选一；levels 优先）",
			"enum":        []string{"error", "warn", "info", "debug"},
		},
		"keyword": map[string]interface{}{
			"type":        "string",
			"description": "关键词子串过滤（大小写不敏感，匹配 message / source / origin / tags）。" +
				"⚠️ 过滤在执行端完成，只回传命中结果 —— 日志量大时务必带上它",
		},
		"source": map[string]interface{}{
			"type":        "string",
			"description": "日志来源：go（后端）/ frontend（WebView 前端）/ all（两者，= DevLogs 同款）",
			"enum":        []string{"all", "go", "frontend"},
			"default":     "all",
		},
		"since": map[string]interface{}{
			"type":        "string",
			"description": "只要晚于该时间戳的日志（格式 HH:MM:SS，与日志 timestamp 同格式）",
		},
	},
	"required": []string{},
}

// ListDiagTools 返回诊断工具的元信息（与 fs 工具同形状：name/description/parameters/needConfirm/kind）。
func (s *Server) ListDiagTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name": "get_device_info",
			"description": "读取**本机**（执行端）的运行时画像：平台 / 版本 / 进程 uptime / 挂载点可用性 / " +
				"双端互联状态（是否连上会合点、最后一次失败原因）/ 全文索引状态。" +
				"远程排障第一步就该调它——先确认对端是谁、活着没有、索引建完没。",
			"parameters":  diagToolGetDeviceInfoSchema,
			"needConfirm": false,
			"kind":        "diagnostic",
		},
		{
			"name": "read_logs",
			"description": "读取**本机**（执行端）最近的结构化日志（含 level/message/timestamp/tags/source）。" +
				"默认 source=all ⇒ 与 DevLogs 页面**同款**（Go 后端日志 + WebView 前端日志）。" +
				"支持多级别（levels）、关键词（keyword）、来源（source）、since 增量。" +
				"⚠️ 过滤**全部在执行端完成**，只回传结果；limit 上限 200、单条 2000 字符。" +
				"排障时用它看对端到底报了什么错——先加 keyword/level 过滤，别一次拉全量。",
			"parameters":  diagToolReadLogsSchema,
			"needConfirm": false,
			"kind":        "diagnostic",
		},
	}
}

// executeDiagTool 派发诊断工具（与 executeFSTool 同层级，由 executeAgentTool 调用）。
func (s *Server) executeDiagTool(ctx context.Context, toolName, argsJSON string) (string, error) {
	switch toolName {
	case "get_device_info":
		return s.diagGetDeviceInfo(argsJSON)
	case "read_logs":
		return s.diagReadLogs(argsJSON)
	default:
		return "", fmt.Errorf("unknown diag tool: %s", toolName)
	}
}

// ─── get_device_info ────────────────────────────────────────────

func (s *Server) diagGetDeviceInfo(argsJSON string) (string, error) {
	_ = argsJSON // 无参数

	rt := s.snapshotRuntimeInfo()

	// 挂载点：只暴露 id/type/public_path/available —— **不含物理路径**
	mounts := make([]map[string]interface{}, 0, 2)
	for _, m := range s.ListFSMounts() {
		mounts = append(mounts, map[string]interface{}{
			"id":         m.ID,
			"type":       m.Type,
			"publicPath": m.PublicPath,
			"available":  m.Available,
		})
	}

	// 双端互联状态（本端作为 Edge 连到 Hub 的那条链路）
	edge := map[string]interface{}{"running": false}
	s.peerEdgeMu.Lock()
	rtEdge := s.peerEdge
	s.peerEdgeMu.Unlock()
	if rtEdge != nil {
		connected := false
		lastErr := ""
		attempts := 0
		if rtEdge.edge != nil {
			connected = rtEdge.edge.Online()
			lastErr = rtEdge.edge.LastError()
			attempts = rtEdge.edge.Attempts()
		}
		edge = map[string]interface{}{
			"running":   true,
			"hub":       rtEdge.hubURL,
			"connected": connected,
			"lastErr":   lastErr,
			"attempts":  attempts,
			"startedAt": rtEdge.startedAt.Format(time.RFC3339),
		}
	}

	// 全文索引状态（与联邦搜索回给发起端的 indexState 同一判据）
	indexState := "unavailable"
	indexedFiles := 0
	if idx := GetFullTextIndex(); idx != nil {
		indexState = ftsIndexState(idx)
		indexedFiles = idx.Stats().TotalFiles
	}

	// 本端 peerlink 身份（只有 ID 与名字，不含 token/psk）
	peer := map[string]interface{}{"hub": false}
	if s.peerHub != nil {
		info := s.peerHub.Info()
		peer = map[string]interface{}{
			"hub":    true,
			"peerId": info["peerId"],
			"name":   info["name"],
		}
	}

	// 主应用 SPA 热更包（I4，2026-10-05）：装机后靠它远程确认"热更到底生没生效"。
	//
	//	Kotlin 侧 MainActivity 只会在这个目录同时存在 index.html 与 version.json 时
	//	把 Capacitor 本地服务指向它 ⇒ 这里把"装了哪版"回给发起端，
	//	云控下发后才能确认生效，而不是靠"看起来变了"来判断。
	webBundle := map[string]interface{}{"installed": false}
	if dir := config.AppDataDir("web-bundle"); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
			v := ""
			if b, err := os.ReadFile(filepath.Join(dir, "version.json")); err == nil {
				var vv struct {
					Version string `json:"version"`
				}
				if json.Unmarshal(b, &vv) == nil {
					v = vv.Version
				}
			}
			webBundle = map[string]interface{}{"installed": true, "version": v}
		}
	}

	return okJSON(map[string]interface{}{
		"webBundle": webBundle,
		"platform": map[string]interface{}{
			"goos":   runtime.GOOS,
			"goarch": runtime.GOARCH,
			"pid":    rt.PID,
			"mobile": rt.Mobile,
		},
		"version":     rt.Version,
		"instanceId":  rt.InstanceID,
		"uptimeMs":    rt.UptimeMs,
		"heartbeatOk": rt.HeartbeatOK,
		"now":         time.Now().Format(time.RFC3339),
		"mounts":      mounts,
		"edge":        edge,
		"searchIndex": map[string]interface{}{
			"state":        indexState,
			"indexedFiles": indexedFiles,
		},
		"peerlink": peer,
		"server":   map[string]string{"goos": runtime.GOOS},
	}), nil
}

// ─── read_logs ─────────────────────────────────────────────────

// diagLogMaxMessageChars 单条日志 message 截断长度（防止一条巨型日志顶爆 RPC 报文）。
const diagLogMaxMessageChars = 2000

// diagLogSources 可选的日志来源（source 参数）。
//
//	go       —— Go 后端日志（logger.DefaultLogBuffer，与 DevLogs「后端日志」同源）
//	frontend —— WebView 前端日志（经 POST /api/logs/frontend 回传，见 frontend_logs.go）
//	all      —— 两者都给 ⇒ **DevLogs 同款**
const (
	diagSourceGo       = "go"
	diagSourceFrontend = "frontend"
	diagSourceAll      = "all"
)

// diagLevelsFrom 归一化级别过滤：levels（数组）优先，兼容旧的 level（单值）。
// 空 ⇒ 不过滤（全级别）。
func diagLevelsFrom(levels []string, single string) []string {
	out := make([]string, 0, len(levels)+1)
	for _, l := range levels {
		l = strings.ToLower(strings.TrimSpace(l))
		if l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		if l := strings.ToLower(strings.TrimSpace(single)); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func diagLevelAllowed(levels []string, level string) bool {
	if len(levels) == 0 {
		return true
	}
	l := strings.ToLower(strings.TrimSpace(level))
	for _, want := range levels {
		if l == want {
			return true
		}
	}
	return false
}

// diagKeywordHit 关键词匹配（**大小写不敏感**）：命中 message / source / origin / tags 任一。
func diagKeywordHit(keyword string, e map[string]string) bool {
	kw := strings.ToLower(strings.TrimSpace(keyword))
	if kw == "" {
		return true // 空关键词 = 不过滤（反向锁：不得因为没传关键词就一条都不返回）
	}
	for _, field := range []string{"message", "source", "origin", "tags"} {
		if strings.Contains(strings.ToLower(e[field]), kw) {
			return true
		}
	}
	return false
}

// diagCollectLogs 按来源收集日志（未过滤）。
func diagCollectLogs(source string) []map[string]string {
	out := make([]map[string]string, 0, 500)
	if source == diagSourceAll || source == diagSourceGo {
		out = append(out, logger.DefaultLogBuffer.Snapshot()...)
	}
	if source == diagSourceAll || source == diagSourceFrontend {
		out = append(out, frontendLogBuffer.Snapshot()...)
	}
	return out
}

// diagReadLogs —— 远程读对端日志（2026-10-05 扩展）。
//
// **过滤全部在执行端（安卓）完成**：等级 / 关键词 / 来源 / 时间增量都在这里过滤，
// 调用方只拿到结果 ⇒ 链路上不搬运无关日志（这是本工具存在的理由：日志量大，
// 全量回传会顶爆控制面）。
//
// 入参：
//
//	limit   返回条数（默认 50，上限 200）
//	levels  级别列表，如 ["warn","error"]（兼容旧参数 level 单值）
//	keyword 关键词子串（大小写不敏感）
//	source  go / frontend / all（默认 all = DevLogs 同款：后端 + 前端）
//	since   只回 timestamp > since（增量拉取）
//
// 出参：count（=returned，向后兼容）/ matched（过滤命中总数）/
//
//	returned / truncated（是否还有更多，可把 since 推到本批最后一条继续拉）。
func (s *Server) diagReadLogs(argsJSON string) (string, error) {
	var args struct {
		Limit   int      `json:"limit"`
		Level   string   `json:"level"`
		Levels  []string `json:"levels"`
		Keyword string   `json:"keyword"`
		Source  string   `json:"source"`
		Since   string   `json:"since"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errJSON("invalid_args", err.Error()), nil
	}
	if args.Limit <= 0 {
		args.Limit = 50
	}
	if args.Limit > 200 {
		args.Limit = 200
	}
	levels := diagLevelsFrom(args.Levels, args.Level)
	source := strings.ToLower(strings.TrimSpace(args.Source))
	switch source {
	case diagSourceGo, diagSourceFrontend:
	default:
		source = diagSourceAll
	}
	args.Since = strings.TrimSpace(args.Since)

	// ① 收集（未过滤）② 逐个过滤 —— 都在本端完成
	matched := make([]map[string]string, 0, 64)
	for _, e := range diagCollectLogs(source) {
		if e == nil {
			continue
		}
		if !diagLevelAllowed(levels, e["level"]) {
			continue
		}
		if !diagKeywordHit(args.Keyword, e) {
			continue
		}
		if args.Since != "" && e["timestamp"] <= args.Since {
			continue
		}
		matched = append(matched, e)
	}

	// ③ 时间正序（两源合并后重排；timestamp 是 HH:MM:SS，字符串序即时间序，
	//    与既有 /api/logs/recent 的 since 语义一致）
	sort.SliceStable(matched, func(i, j int) bool { return matched[i]["timestamp"] < matched[j]["timestamp"] })

	// ④ 只回**最近**的 limit 条
	start := 0
	if len(matched) > args.Limit {
		start = len(matched) - args.Limit
	}
	items := make([]map[string]string, 0, args.Limit)
	for _, e := range matched[start:] {
		out := make(map[string]string, len(e))
		for k, v := range e {
			if k == "message" && len(v) > diagLogMaxMessageChars {
				v = v[:diagLogMaxMessageChars] + "\u2026(truncated)"
			}
			out[k] = v
		}
		items = append(items, out)
	}

	return okJSON(map[string]interface{}{
		"count":     len(items), // 向后兼容：旧调用方读 count
		"returned":  len(items),
		"matched":   len(matched),
		"truncated": len(matched) > len(items),
		"source":    source,
		"levels":    levels,
		"keyword":   strings.TrimSpace(args.Keyword),
		"capacity":  500 + 300,
		"items":     items,
		"server":    map[string]string{"goos": runtime.GOOS},
	}), nil
}

