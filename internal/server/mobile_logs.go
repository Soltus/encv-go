package server

// mobile_logs.go — API 日志查询 + 构建信息 + FFmpeg 状态 + 自动化测试报告。

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleAPILogsGin(c *gin.Context) {
	var req struct {
		Level     string `json:"level"`
		Message   string `json:"message"`
		Tag       string `json:"tag,omitempty"`
		Timestamp int64  `json:"timestamp,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
		return
	}
	msg := req.Message
	if req.Tag != "" {
		msg = "[" + req.Tag + "] " + msg
	}
	switch req.Level {
	case "error":
		slog.Error(msg)
	case "warn":
		slog.Warn(msg)
	case "debug":
		slog.Debug(msg)
	default:
		slog.Info(msg)
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleAPILogsRecentGin(c *gin.Context) {
	since := c.Query("since")
	entries := logger.DefaultLogBuffer.Snapshot()
	if since != "" {
		filtered := entries[:0]
		for _, e := range entries {
			if e["timestamp"] > since {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	c.JSON(http.StatusOK, gin.H{
		"logs":     entries,
		"count":    len(entries),
		"capacity": 500,
	})
}

func (s *Server) writeConfigToFile() error {
	if s.configPath == "" {
		return fmt.Errorf("config path not available")
	}

	raw, err := json.Marshal(s.cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return fmt.Errorf("failed to unmarshal config for filtering: %w", err)
	}

	if proxy, ok := generic["proxy"].(map[string]interface{}); ok {
		if sites, ok := proxy["sites"].(map[string]interface{}); ok {
			for id, raw := range sites {
				if entry, ok := raw.(map[string]interface{}); ok {
					if builtin, _ := entry["built_in"].(bool); builtin {
						delete(sites, id)
					}
				}
			}
		}
	}

	indented, err := json.MarshalIndent(generic, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(s.configPath, append(indented, '\n'), 0644)
}

func (s *Server) handleBuildInfoGin(c *gin.Context) {
	info, err := utils.GetBuildInfo()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	info["app_version"] = s.version
	c.JSON(http.StatusOK, info)
}

func (s *Server) handleGetContainerVersionsGin(c *gin.Context) {
	c.JSON(200, gin.H{
		"versions": []gin.H{
			{"version": 2, "status": "deprecated", "label": "V2 (已弃用)"},
			{"version": 3, "status": "stable", "label": "V3"},
			{"version": 4, "status": "recommended", "label": "V4 (推荐)"},
		},
		"default": s.cfg.GetEffectiveDefaultVersion(),
	})
}

func (s *Server) handleFFmpegStatusGin(c *gin.Context) {
	ffmpegOk, ffprobeOk, errMsg, ffmpegDetail, ffprobeDetail := utils.CheckFFmpegAvailable()
	c.JSON(http.StatusOK, gin.H{
		"ffmpeg_available":  ffmpegOk,
		"ffprobe_available": ffprobeOk,
		"error":             errMsg,
		"ffmpeg_detail":     ffmpegDetail,
		"ffprobe_detail":    ffprobeDetail,
	})
}

func (s *Server) handleAutomationReportGin(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}
	// 提取关键字段便于日志检索
	runCount, _ := payload["runCount"].(float64)
	failed, _ := payload["totalFailed"].(float64)
	passed, _ := payload["totalPassed"].(float64)
	webdav, _ := payload["webdavRunCount"].(float64)
	plugin, _ := payload["pluginRunCount"].(float64)
	failureRate, _ := payload["failureRate"].(float64)
	suspiciousCount := 0
	if bugs, ok := payload["suspiciousBugs"].([]any); ok {
		suspiciousCount = len(bugs)
	}
	// 失败/可疑 bug → warn 级别（运维日志监控可捞）
	logLevel := "info"
	if failed > 0 || suspiciousCount > 0 {
		logLevel = "warn"
	}
	slog.LogAttrs(c.Request.Context(), slog.LevelInfo,
		"[automation-report] 收到前端自动化测试分析上报",
		slog.String("level", logLevel),
		slog.Float64("runCount", runCount),
		slog.Float64("webdavRunCount", webdav),
		slog.Float64("pluginRunCount", plugin),
		slog.Float64("totalPassed", passed),
		slog.Float64("totalFailed", failed),
		slog.Float64("failureRate%", failureRate),
		slog.Int("suspiciousBugCount", suspiciousCount),
	)
	// 可疑 bug 详情单独打一行 JSON（方便日志检索）
	if bugs, ok := payload["suspiciousBugs"].([]any); ok && len(bugs) > 0 {
		bugJSON, _ := json.Marshal(bugs)
		slog.Warn("[automation-report] suspicious bugs: " + string(bugJSON))
	}
	// 最近失败用例详情
	if lastFailed, ok := payload["lastRunFailed"].([]any); ok && len(lastFailed) > 0 {
		lfJSON, _ := json.Marshal(lastFailed)
		slog.Warn("[automation-report] last run failed cases: " + string(lfJSON))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "received": true})
}
