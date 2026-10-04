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
	"runtime"
	"strings"
	"time"

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
		"level": map[string]interface{}{
			"type":        "string",
			"description": "按级别过滤：error / warn / info / debug（不传 = 不过滤）",
			"enum":        []string{"error", "warn", "info", "debug"},
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
			"description": "读取**本机**（执行端）最近的结构化日志（环形缓冲，容量 500，含 level/message/timestamp/tags）。" +
				"支持按 level 过滤与 since 增量拉取。排障时用它看对端到底报了什么错。",
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

	return okJSON(map[string]interface{}{
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

func (s *Server) diagReadLogs(argsJSON string) (string, error) {
	var args struct {
		Limit int    `json:"limit"`
		Level string `json:"level"`
		Since string `json:"since"`
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
	args.Level = strings.ToLower(strings.TrimSpace(args.Level))

	entries := logger.DefaultLogBuffer.Snapshot()

	// 环形缓冲按时间正序返回；调用方通常只关心**最近**的 ⇒ 从尾部取
	items := make([]map[string]string, 0, args.Limit)
	matched := 0
	for i := len(entries) - 1; i >= 0 && len(items) < args.Limit; i-- {
		e := entries[i]
		if e == nil {
			continue
		}
		if args.Level != "" && !strings.EqualFold(e["level"], args.Level) {
			continue
		}
		if args.Since != "" && e["timestamp"] <= args.Since {
			continue
		}
		matched++
		out := make(map[string]string, len(e))
		for k, v := range e {
			if k == "message" && len(v) > diagLogMaxMessageChars {
				v = v[:diagLogMaxMessageChars] + "…(truncated)"
			}
			out[k] = v
		}
		items = append(items, out)
	}
	// 反转为时间正序，便于阅读
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}

	return okJSON(map[string]interface{}{
		"count":    len(items),
		"capacity": 500,
		"matched":  matched,
		"items":    items,
		"server":   map[string]string{"goos": runtime.GOOS},
	}), nil
}
