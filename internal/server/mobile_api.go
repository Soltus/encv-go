package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/mount"
	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/Soltus/encv-go/internal/search"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/Soltus/encv-go/internal/v2/testutil"
	alistencrypt "github.com/Soltus/encv-go/internal/v2/plugins/alistencrypt"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/gin-gonic/gin"
)

func isValidSiteID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func writeServiceErrorGin(c *gin.Context, err error) {
	switch err.(type) {
	case *mobileservice.PermissionError:
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case *mobileservice.ForbiddenError:
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case *mobileservice.NotFoundError:
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case *mobileservice.BadRequestError:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case *mobileservice.UnsupportedMediaTypeError:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func (s *Server) handlePingGin(c *gin.Context) {
	c.JSON(http.StatusOK, types.PingResponse{
		Status:        types.ServiceStatuses.OK,
		Version:       s.version,
		InstanceID:    s.instanceID,
		ServerDirPath: s.servingDir,
		WebdavDirPath: s.webdavDir,
	})
}

func (s *Server) handleHealthGin(c *gin.Context) {
	// 🆕 2026-06-14：跨进程 IPC 重构 — 改为 JSON 响应（含心跳状态）
	//
	// 历史：返回 {"status":"ok"}。parent（Kotlin EncvGoService）只能判存活不能判 hang。
	// 新：返回 {"status","heartbeat_age_ms","heartbeat_ok"}。
	//     parent 通过 heartbeat_ok 字段判 hang，连续 N 次 false 才销毁进程。
	//
	// 向后兼容：保留 "status" 字段，前端 / 沙箱 preview-gateway 用 code==200 判断不受影响。
	hbMs := atomic.LoadInt64(&s.lastHeartbeatMs)
	var hbAgeMs int64 = -1
	var hbOK bool = false
	if hbMs > 0 {
		hbAgeMs = time.Now().UnixMilli() - hbMs
		hbOK = hbAgeMs < HeartbeatStaleThreshold.Milliseconds()
	}
	c.JSON(http.StatusOK, gin.H{
		"status":           "ok",
		"heartbeat_age_ms": hbAgeMs,
		"heartbeat_ok":     hbOK,
	})
}

func (s *Server) handleServerShutdownGin(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "shutting_down"})
	go func() {
		slog.Info("Shutdown requested via API")
		if s.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.server.Shutdown(ctx); err != nil {
				slog.Error("Server shutdown error", "error", err)
			}
		}
		s.readerService.Cleanup()
		os.Exit(0)
	}()
}

func (s *Server) handleListFilesGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	slog.Info("API: list files", "path", queryPath)

	// 🆕 2026-06-15 multi-mount 适配：mount 虚拟根 "/d" 或 "/d/"
	//   在 mount 系统里，根目录 /d 不是真实 FS 路径，而是 mount 列表入口
	//   直接返 mount list 转 FileInfo[]，让前端用同一份 FileInfo 列表展示
	//   （前端的 Files.vue 无需另起 listMounts() 拉取 — 走 /api/files 同一路径）
	if (queryPath == "/d" || queryPath == "/d/") && s.mountRegistry != nil {
		slog.Info("API: list files (mount root)", "path", queryPath, "mountCount", len(s.mountRegistry.List()))
		files := mountListAsFiles(s.mountRegistry.List())
		c.JSON(http.StatusOK, gin.H{"files": files})
		return
	}
	if queryPath == "/d" || queryPath == "/d/" {
		slog.Warn("API: list files mount root but registry is nil", "path", queryPath)
	}

	files, err := s.mobileSvc.ListFiles(queryPath)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	if tag := c.Query("tag"); tag != "" {
		taggedPaths := GlobalTagStore.GetFilesByTag(tag)
		taggedSet := make(map[string]bool, len(taggedPaths))
		for _, p := range taggedPaths {
			taggedSet[p] = true
		}
		filtered := make([]mobileservice.FileInfo, 0, len(files))
		for _, f := range files {
			if taggedSet[f.Path] {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}

	slog.Info("API: list files result", "path", queryPath, "count", len(files))
	c.JSON(http.StatusOK, gin.H{"files": files})
}

// mountListAsFiles 把 mount registry 列表转 FileInfo（前端可直接当目录展示）
//   🆕 2026-06-15 multi-mount 适配：Files.vue 根 /d 直接展示 mount 列表
func mountListAsFiles(mounts []*mount.Mount) []mobileservice.FileInfo {
	files := make([]mobileservice.FileInfo, 0, len(mounts))
	for _, m := range mounts {
		if !m.Enabled {
			continue
		}
		files = append(files, mobileservice.FileInfo{
			Name:        m.Name,
			Path:        m.MountPath, // /d/<name>
			IsDirectory: true,
			Size:        0,
			Modified:    m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			// 额外字段：前端在 ion-item 副标题用 driver badge + mount_path + root_path 展示
			MountDriver: m.Driver,
			MountPath:   m.MountPath,
			MountRoot:   m.RootPath,
		})
	}
	return files
}

func (s *Server) handleDeleteFileGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	slog.Warn("API: delete file", "path", queryPath)

	absPath, err := s.resolveUserPath(queryPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := s.mobileSvc.GetTaskManager().Create("delete", absPath, "", "", 0, "")
	task.OriginalPath = absPath
	// 🆕 2026-06-23 WS 时序修复：Create 不再内部广播，外部补 RunId 兜底 + 持久化 + 广播
	s.mobileSvc.GetTaskManager().FinalizeCreatedTask(task)

	c.JSON(http.StatusAccepted, gin.H{"taskId": task.ID})
}

func (s *Server) handleCreateDirectoryGin(c *gin.Context) {
	var req struct {
		ParentPath string `json:"parent_path"`
		Name       string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	slog.Info("API: create directory", "parent_path", req.ParentPath, "name", req.Name)

	err := s.mobileSvc.CreateDirectory(req.ParentPath, req.Name)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "created"})
}

func (s *Server) handleUploadFileGin(c *gin.Context) {
	targetPath := utils.DecodeGinQueryParam(c.Query("path"))
	if targetPath == "" {
		targetPath = "/"
	}
	slog.Info("API: upload file", "target_path", targetPath)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid 'file' form field"})
		return
	}
	defer file.Close()

	if header.Filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename is empty"})
		return
	}

	result, err := s.mobileSvc.UploadFile(targetPath, header.Filename, file, 0)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleServiceGuardGin 处理 GET /api/service-guard
//
// 2026-06-10 简化：只检查 servingDir 是否挂载到 /storage/emulated/0（mobile 真机 / dev preview 的标准路径）。
//
// 历史版本还检查 01-plain-media marker（4 个子目录 + 文件数）。2026-06-10 改造：
//   - Node CLI scripts/generate-mock-files.ts 已废弃（重复入口）
//   - service-guard 不再强制 mock 数据就位（用户主动调 /api/mock/generate 生成）
//   - 用户没主动按"生成 Mock"按钮时，目录是空的——这是预期行为
//
// 所以现在守卫只验证"servingDir 是不是 mobile 标准路径"，不关心里面有没有数据。
func (s *Server) handleServiceGuardGin(c *gin.Context) {
	// 🆕 2026-06-15 multi-mount: expectedDir 来自 primary mount 解析（不再是硬编码 /storage/emulated/0）
	//   - 真机：primary.RootPath = /storage/emulated/0（LocalDriver 绑 cfg.Server.Dir）
	//   - dev 沙箱：primary.RootPath = 当前 cfg.Server.Dir（LocalDriver Abs 解析）
	//   - 后端 start 时 cfg.Server.Dir 被 ApplyMobileOverlay 改为 /storage/emulated/0（mobile mode）
	//     或者 = 当前 cwd（普通 mode）—— primary mount 的 RootPath 跟这个对齐
	primaryExpected := "/storage/emulated/0" // 兜底
	if s.mountRegistry != nil {
		if pm := s.mountRegistry.GetByName("primary"); pm != nil && pm.RootPath != "" {
			primaryExpected = pm.RootPath
		}
	}
	expectedDir := primaryExpected

	// 1. 解析成绝对路径
	absDir, err := filepath.Abs(s.servingDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ready":  false,
			"detail": fmt.Sprintf("servingDir 解析失败: %v", err),
		})
		return
	}

	envDevPreview := os.Getenv("ENCV_DEV_PREVIEW") == "1"
	envMobile := os.Getenv("ENCV_MOBILE") == "1"

	// 2. servingDir 必须 == /storage/emulated/0
	if absDir != expectedDir {
		c.JSON(http.StatusForbidden, gin.H{
			"ready":         false,
			"servingDir":    absDir,
			"expected":      expectedDir,
			"envDevPreview": envDevPreview,
			"envMobile":     envMobile,
			"detail":        fmt.Sprintf("servingDir=%q 不是 mobile 真机/预览标准路径 %q", absDir, expectedDir),
			"remediation": []gin.H{
				{
					"scenario": "B1 — 用 mobile overlay 启动（推荐）",
					"command":  "make dev-mobile",
					"explain":  "自动 ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 → ApplyMobileOverlay → servingDir=/storage/emulated/0",
				},
				{
					"scenario": "B2 — 手工等价命令",
					"command":  "ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start",
					"explain":  "同上但手工设 env（pm2 gateway spawn air 时确保透传）",
				},
				{
					"scenario": "B3 — 桌面端正常模式（无 mobile overlay）",
					"command":  "go run ./cmd/encv start",
					"explain":  "不要设 ENCV_DEV_PREVIEW / ENCV_MOBILE，servingDir 自动 = 当前工作目录（通常 /workspace，service-guard 仍会 BLOCK）",
				},
			},
		})
		return
	}

	// 3. 目录必须可读
	if _, statErr := os.Stat(absDir); statErr != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"ready":      false,
			"servingDir": absDir,
			"detail":     fmt.Sprintf("servingDir 不可读: %v", statErr),
		})
		return
	}

	// ✅ 一切就绪
	c.JSON(http.StatusOK, gin.H{
		"ready":         true,
		"servingDir":    absDir,
		"expected":      expectedDir,
		"envDevPreview": envDevPreview,
		"envMobile":     envMobile,
		"nextStep":      "前端访问 OpenPreview 激活 :16666 入口（http://localhost:16666/）",
	})
}

// pluginOpenlistUpstream plugin-openlist 独立 Vite dev server
// （由 pm2 app `plugin-openlist-vite` 拉起，独立于 encv-mobile vite :5173）
const pluginOpenlistUpstream = "http://127.0.0.1:5174"

// handlePluginOpenlistProxyGin 反向代理 /api/preview/plugin-openlist/* → :5174/*
//
// 为什么需要：
//   - plugin-openlist 是独立 Capacitor 插件的 Vite dev server（:5174），
//     跟 encv-mobile 主 vite (5173) 没有父子关系。
//   - 前端点击 "预览 OpenList Plugin" 不能直接跳 :5174（破坏 OpenPreview 会话，
//     且 Capacitor native 端 127.0.0.1 指向设备本身，不通）。
//   - vite.config.ts 的 openlist-ui-proxy 只能代理 :5244（OpenList 真实前端），
//     不能代理 :5174（plugin-openlist 是另一个独立 vite 进程）。
//
// 方案：encv-go 后端（独立后端）做 reverse proxy 协调，
// 前端跳相对路径 /api/preview/plugin-openlist/ → encv-go → :5174。
//
// 完全不依赖 vite，符合"独立后端协调"原则。
func (s *Server) handlePluginOpenlistProxyGin(c *gin.Context) {
	target, err := url.Parse(pluginOpenlistUpstream)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid upstream: " + err.Error()})
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// Director 改写 host + path，透传 header
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		// req.URL.Path 已经被 ReverseProxy 设为原始请求路径，
		// 我们要 strip 掉 /api/preview/plugin-openlist 前缀
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api/preview/plugin-openlist")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
	}
	// 自定义 ErrorHandler：upstream 不可达时返回明确错误（不要 502 难诊断）
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		slog.Error("[plugin-openlist proxy] upstream error", "err", err, "url", req.URL.String())
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(rw, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>plugin-openlist 未运行</title></head>
<body style="font-family:system-ui;padding:24px;max-width:560px;margin:auto">
<h2>plugin-openlist dev server 未运行</h2>
<p>upstream: %s</p>
<p>err: %s</p>
<p>启动方式（pm2 统一管理）：<code>npx pm2 start ecosystem.config.cjs --only plugin-openlist-vite</code></p>
<p>或直接：<code>cd app/encv-mobile/plugin-openlist/web && pnpm dev</code></p>
</body></html>`, pluginOpenlistUpstream, err.Error())
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func (s *Server) handleReadFileContentGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	slog.Info("API: read file content", "path", queryPath)

	result, err := s.mobileSvc.ReadFileContent(queryPath)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) handleTextPreviewExtsGin(c *gin.Context) {
	builtIn := config.GetTextPreviewExtensions()
	var custom []string
	if s.cfg.Preview != nil {
		custom = s.cfg.Preview.TextExtensions
	}
	c.JSON(http.StatusOK, gin.H{
		"extensions":        builtIn,
		"custom_extensions": custom,
	})
}

func (s *Server) handleFileInfoGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	if queryPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'path' query parameter is required"})
		return
	}

	result, err := s.mobileSvc.GetFileInfo(queryPath)
	if err != nil {
		if _, ok := err.(*mobileservice.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) handleRenameFileGin(c *gin.Context) {
	var req mobileservice.RenameFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	slog.Info("API: rename file original_name", "path", req.Path, "newName", req.NewName,
		"hasPassword", req.Password != "")

	result, err := s.mobileSvc.RenameFile(&req)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) handleGetTasksGin(c *gin.Context) {
	// 🆕 2026-06-23 Task 5：分页 API（10 万任务虚拟滚动支撑）
	//   - ?runId=xxx  → 只返回 task.RunId == runId 的 task（空则不过滤）
	//   - &offset=0   → 默认 0
	//   - &limit=100  → 默认 100，最大 500（防滥用）
	//   - 响应头 X-Total-Count = 过滤后、分页前的总数
	runId := c.Query("runId")

	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}

	tasks, totalCount := s.mobileSvc.GetTaskManager().ListPaginated(runId, offset, limit)
	c.Header("X-Total-Count", strconv.Itoa(totalCount))
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (s *Server) handleCreateTaskGin(c *gin.Context) {
	var req struct {
		Type              string            `json:"type"`
		SourcePath        string            `json:"sourcePath"`
		TargetPath        string            `json:"targetPath,omitempty"`
		Password          string            `json:"password,omitempty"`
		SecondaryPassword string            `json:"secondaryPassword,omitempty"`
		Version           int               `json:"version,omitempty"`
		PluginName        string            `json:"pluginName,omitempty"`
		ExtraFields       map[string]string `json:"extraFields,omitempty"`
		// 🆕 2026-06-18 Task 16：加解密参数持久化
		CipherMode      int    `json:"cipherMode,omitempty"`
		CompressionMode string `json:"compressionMode,omitempty"`
		// 🆕 v6 2026-06-18：runId + triggeredBy（单一数据源）
		RunId       string `json:"runId,omitempty"`
		TriggeredBy string `json:"triggeredBy,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	slog.Info("API: create task", "type", req.Type, "source", req.SourcePath,
		"target", req.TargetPath, "version", req.Version,
		"pluginName", req.PluginName,
		"hasPassword", req.Password != "",
		"hasSecondaryPassword", req.SecondaryPassword != "",
		"hasExtraFields", len(req.ExtraFields) > 0,
		"cipherMode", req.CipherMode,
		"compressionMode", req.CompressionMode,
		"runId", req.RunId,
		"triggeredBy", req.TriggeredBy)

	// 🆕 v6 2026-06-18：统一走 CreateWithRunMeta（含 crypto params + run meta）
	//   - runId 非空 → 自动化测试/AI agent 任务，前端按 runId 聚合
	//   - runId 空 → 后端兜底派生 "manual-${id}"（2026-06-22），不再有孤儿 task
	//   - triggeredBy 空 → 后端兜底 'user'（2026-06-22）
	compressionMode := req.CompressionMode
	if compressionMode == "" {
		compressionMode = "none"
	}
	task := s.mobileSvc.GetTaskManager().CreateWithRunMeta(
		req.Type, req.SourcePath, req.TargetPath,
		req.Password, req.SecondaryPassword, req.Version, req.PluginName, req.ExtraFields,
		req.CipherMode, compressionMode,
		req.RunId, req.TriggeredBy,
	)

	s.UpdateTaskSearchIndex(task.ID, task.SourcePath, task.Type, task.SourcePath, task.Status)

	c.JSON(http.StatusCreated, task)
}

// 🆕 2026-06-23 真实架构实现：批量创建 task endpoint
//
// 架构原则（替代 client 预占位野路子）：
//   - 前端 submitRun 阶段收集本层所有 step 的 task 定义 → 一次性 POST /api/tasks/batch
//   - 后端批量创建所有 task（后端生成 UUID 作为唯一权威源）→ 一次性返回所有 task
//   - 前端拿到后一次性 push 到 store → UI 立即显示 1 个 group N task（不慢慢累加）
//   - 不存在 client ID 覆盖后端 ID 的野路子
//
// 请求体：
//
//	{
//	  "runId": "run-xxx",
//	  "triggeredBy": "automation",
//	  "tasks": [
//	    { "type": "encrypt", "sourcePath": "/mock/sample.mp4", "pluginName": "video", ... },
//	    ...
//	  ]
//	}
//
// 返回：HTTP 201，body 是 task 数组
func (s *Server) handleCreateTaskBatchGin(c *gin.Context) {
	var req struct {
		RunId       string `json:"runId,omitempty"`
		TriggeredBy string `json:"triggeredBy,omitempty"`
		Tasks       []struct {
			Type              string            `json:"type"`
			SourcePath        string            `json:"sourcePath"`
			TargetPath        string            `json:"targetPath,omitempty"`
			Password          string            `json:"password,omitempty"`
			SecondaryPassword string            `json:"secondaryPassword,omitempty"`
			Version           int               `json:"version,omitempty"`
			PluginName        string            `json:"pluginName,omitempty"`
			ExtraFields       map[string]string `json:"extraFields,omitempty"`
			CipherMode        int               `json:"cipherMode,omitempty"`
			CompressionMode   string            `json:"compressionMode,omitempty"`
		} `json:"tasks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	if len(req.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tasks array is empty"})
		return
	}

	slog.Info("API: batch create tasks",
		"count", len(req.Tasks),
		"runId", req.RunId,
		"triggeredBy", req.TriggeredBy)

	specs := make([]mobileservice.BatchTaskSpec, 0, len(req.Tasks))
	for _, t := range req.Tasks {
		specs = append(specs, mobileservice.BatchTaskSpec{
			Type:              t.Type,
			SourcePath:        t.SourcePath,
			TargetPath:        t.TargetPath,
			Password:          t.Password,
			SecondaryPassword: t.SecondaryPassword,
			Version:           t.Version,
			PluginName:        t.PluginName,
			ExtraFields:       t.ExtraFields,
			CipherMode:        t.CipherMode,
			CompressionMode:   t.CompressionMode,
		})
	}

	tasks := s.mobileSvc.GetTaskManager().CreateBatch(specs, req.RunId, req.TriggeredBy)

	// 增量更新搜索索引
	if s.searchSvc != nil {
		ctx := context.Background()
		batch := make([]vectorsearch.TaskIndexItem, 0, len(tasks))
		for _, t := range tasks {
			batch = append(batch, vectorsearch.TaskIndexItem{
				ID:         t.ID,
				Name:       t.SourcePath,
				TaskType:   t.Type,
				SourcePath: t.SourcePath,
				Status:     t.Status,
			})
		}
		if err := s.searchSvc.IndexTasksBatch(ctx, batch); err != nil {
			slog.Warn("batch update task search index failed", "error", err)
		}
	}

	c.JSON(http.StatusCreated, tasks)
}

func (s *Server) handleCancelTaskGin(c *gin.Context) {
	id := c.Param("id")

	task, err := s.mobileSvc.GetTaskManager().Cancel(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// handleCancelRunGin 批量取消指定 runId 下所有非终态 task。
//
// 🆕 2026-06-23 spec ws-timing-batch-throughput-100k Task 2.2：
//   - 前端 cancelRun 一次 API 调用取消整个 run（替代逐个 cancelTask 循环）
//   - 路由：POST /api/runs/:runId/cancel
func (s *Server) handleCancelRunGin(c *gin.Context) {
	runId := c.Param("runId")
	if runId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runId is required"})
		return
	}
	err := s.mobileSvc.GetTaskManager().CancelByRunId(runId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "runId": runId})
}

// handleResumeRunGin 恢复指定 runId 下所有 paused 的 task。
//
// 路由：POST /api/runs/:runId/resume
func (s *Server) handleResumeRunGin(c *gin.Context) {
	runId := c.Param("runId")
	if runId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runId is required"})
		return
	}
	count, err := s.mobileSvc.GetTaskManager().ResumePausedByRunId(runId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resumed", "runId": runId, "resumed": count})
}

// handleResumeAllPausedGin 恢复所有 paused 状态的 task。
//
// 路由：POST /api/tasks/resume-all
func (s *Server) handleResumeAllPausedGin(c *gin.Context) {
	count, err := s.mobileSvc.GetTaskManager().ResumeAllPaused()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resumed", "resumed": count})
}

// handleGetRunSummaryGin 返回指定 run 的聚合计数（SQL COUNT + GROUP BY status）。
//
// 🆕 2026-06-23 spec backend-sql-authority-view-pagination Task 1.4：
//   - 后端是唯一权威，聚合计数由 SQL 出，不依赖前端 store
//   - 前端 group card 显示 summary.total/passed/failed（不靠 store.tasks 算）
//   - 路由：GET /api/runs/:runId/summary
//   - 响应：{runId, total, passed, failed, running, pending, cancelled, percent}
func (s *Server) handleGetRunSummaryGin(c *gin.Context) {
	runId := c.Param("runId")
	if runId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runId is required"})
		return
	}
	summary := s.mobileSvc.GetTaskManager().GetRunSummary(runId)
	c.JSON(http.StatusOK, summary)
}

// handleListRunsGin 返回所有 run 列表（带 summary，避免 N+1 查询）。
//
// 🆕 2026-06-23 spec backend-sql-authority-view-pagination Task 2.4：
//   - 后端是唯一权威，run 列表由 SQL GROUP BY 出
//   - 每个 run 带 summary（handler 层批量补，避免前端 N+1 调用 /summary）
//   - 路由：GET /api/runs
//   - 响应：{runs: [{runId, startedAt, triggeredBy, summary: {...}}, ...]}
func (s *Server) handleListRunsGin(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	runs := tm.ListRuns()
	// 批量补 summary（避免前端 N+1 调用 /summary）
	type runWithSummary struct {
		RunID       string                   `json:"runId"`
		StartedAt   time.Time                `json:"startedAt"`
		TriggeredBy string                   `json:"triggeredBy"`
		Summary     mobileservice.RunSummary `json:"summary"`
	}
	result := make([]runWithSummary, 0, len(runs))
	for _, r := range runs {
		summary := tm.GetRunSummary(r.RunID)
		result = append(result, runWithSummary{
			RunID:       r.RunID,
			StartedAt:   r.StartedAt,
			TriggeredBy: r.TriggeredBy,
			Summary:     summary,
		})
	}
	c.JSON(http.StatusOK, gin.H{"runs": result})
}

func (s *Server) handleRetryTaskGin(c *gin.Context) {
	id := c.Param("id")

	task, err := s.mobileSvc.GetTaskManager().Retry(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

func (s *Server) handleRemoveTaskGin(c *gin.Context) {
	id := c.Param("id")

	if err := s.mobileSvc.GetTaskManager().RemoveTask(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleClearCompletedTasksGin(c *gin.Context) {
	count := s.mobileSvc.GetTaskManager().ClearCompleted()
	c.JSON(http.StatusOK, gin.H{"ok": true, "removed": count})
}

func (s *Server) handleTestWebDAVGin(c *gin.Context) {
	var req struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid JSON"})
		return
	}

	result, err := s.mobileSvc.TestWebDAV(req.URL, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) handlePermissionsGin(c *gin.Context) {
	storage := s.mobileSvc.CheckStoragePermission()
	slog.Info("API: check permissions", "storage", storage)
	c.JSON(http.StatusOK, gin.H{"storage": storage})
}

func (s *Server) canUseWebdavIndex() bool {
	return s.webdavFS != nil && s.webdavFS.Dir() == s.servingDir
}

func (s *Server) handleSearchFilesGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	keyword := c.Query("keyword")
	recursive := c.Query("recursive") == "true"

	slog.Info("API: search files", "path", queryPath, "keyword", keyword, "recursive", recursive)

	if recursive && keyword != "" {
		var files []mobileservice.FileInfo

		mobileFiles, err := s.mobileSvc.SearchFiles(queryPath, keyword, true)
		if err != nil {
			writeServiceErrorGin(c, err)
			return
		}

		if s.canUseWebdavIndex() {
			for _, f := range mobileFiles {
				if !s.webdavFS.IsContainerExtension(f.Name) {
					files = append(files, f)
				}
			}
			entries := s.webdavFS.SearchInIndex(keyword, queryPath, 200)
			for _, e := range entries {
				files = append(files, mobileservice.FileInfo{
					Name:        e.Name,
					Path:        e.Path,
					IsDirectory: e.IsDir,
					Size:        e.Size,
				})
			}
		} else {
			files = mobileFiles
		}

		slog.Info("API: search files result", "path", queryPath, "keyword", keyword, "count", len(files))
		c.JSON(http.StatusOK, gin.H{"files": files})
		return
	}

	files, err := s.mobileSvc.SearchFiles(queryPath, keyword, recursive)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}

func (s *Server) handleIndexStatsGin(c *gin.Context) {
	stats := s.mobileSvc.GetIndexStats()
	if stats.TotalFiles == 0 && !stats.IsIndexing {
		s.mobileSvc.RebuildIndex()
		stats = s.mobileSvc.GetIndexStats()
	}
	stats.Source = "mobile"

	if s.canUseWebdavIndex() {
		wdStats := s.webdavFS.GetIndexStats()
		ordinaryFiles := stats.TotalFiles
		containerPhysicalCount := 0
		if ordinaryFiles > 0 && wdStats.Containers > 0 {
			containerPhysicalCount = wdStats.Containers
		}
		stats.TotalFiles = ordinaryFiles - containerPhysicalCount + wdStats.TotalFiles
		if stats.TotalFiles < 0 {
			stats.TotalFiles = 0
		}
		stats.Containers = wdStats.Containers
		stats.Source = "webdav"
	}

	c.JSON(http.StatusOK, stats)
}

func (s *Server) handleIndexRebuildGin(c *gin.Context) {
	s.mobileSvc.RebuildIndex()
	source := "mobile"
	if s.canUseWebdavIndex() {
		source = "webdav"
	}
	c.JSON(http.StatusOK, gin.H{"status": "indexing", "source": source})
}

func (s *Server) handleTestLocalWebDAVGin(c *gin.Context) {
	if s.webdavFS == nil {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"error":     "WebDAV not enabled",
		})
		return
	}

	result := gin.H{
		"available":    true,
		"url":          fmt.Sprintf("http://127.0.0.1:%d%s", s.cfg.Server.Port, s.webdavPath),
		"authRequired": s.cfg.Webdav.Username != "" || s.cfg.Webdav.Password != "",
		"details": gin.H{
			"propfindRoot": "fail",
			"authWorks":    "skip",
			"dirReadable":  "fail",
		},
	}

	webdavURL := fmt.Sprintf("http://127.0.0.1:%d%s", s.cfg.Server.Port, s.webdavPath)
	details := gin.H{
		"propfindRoot": "fail",
		"authWorks":    "skip",
		"dirReadable":  "fail",
	}

	propfindBody := `<?xml version="1.0" encoding="UTF-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`
	req, err := http.NewRequest("PROPFIND", webdavURL, bytes.NewBufferString(propfindBody))
	if err == nil {
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		req.Header.Set("Depth", "1")
		if s.cfg.Webdav.Username != "" {
			req.SetBasicAuth(s.cfg.Webdav.Username, s.cfg.Webdav.Password)
		}
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusMultiStatus {
				details["propfindRoot"] = "ok"
				details["dirReadable"] = "ok"
			} else if resp.StatusCode == http.StatusUnauthorized {
				details["propfindRoot"] = "ok"
				details["dirReadable"] = "fail"
			}
		}
	}

	if s.cfg.Webdav.Username != "" {
		details["authWorks"] = "fail"
		req2, err := http.NewRequest("PROPFIND", webdavURL, bytes.NewBufferString(propfindBody))
		if err == nil {
			req2.Header.Set("Content-Type", "application/xml; charset=utf-8")
			req2.Header.Set("Depth", "0")
			req2.SetBasicAuth(s.cfg.Webdav.Username, s.cfg.Webdav.Password)
			client := &http.Client{Timeout: 3 * time.Second}
			resp2, err := client.Do(req2)
			if err == nil {
				defer resp2.Body.Close()
				if resp2.StatusCode == http.StatusMultiStatus {
					details["authWorks"] = "ok"
				}
			}
		}
	}

	result["details"] = details
	c.JSON(http.StatusOK, result)
}

// handleWebDavLocalInfoGin 返回本地 webdav endpoint 的元信息（用户名/密码/是否启用），
// 供前端自动化测试时构造 Basic Auth header，避免触发浏览器原生 401 弹窗
func (s *Server) handleWebDavLocalInfoGin(c *gin.Context) {
	if s.webdavFS == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled":       false,
			"authRequired":  false,
			"username":      "",
			"password":      "",
			"webdavPath":    s.webdavPath,
			"serverBaseUrl": fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Server.Port),
		})
		return
	}
	authRequired := s.cfg.Webdav.Username != "" || s.cfg.Webdav.Password != ""
	c.JSON(http.StatusOK, gin.H{
		"enabled":       true,
		"authRequired":  authRequired,
		"username":      s.cfg.Webdav.Username,
		"password":      s.cfg.Webdav.Password,
		"webdavPath":    s.webdavPath,
		"serverBaseUrl": fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Server.Port),
	})
}

// handleWebDavManifestGin GET /api/webdav/manifest
//
// 🆕 2026-06-17：多挂载点 webdav manifest API（multi-mount-storage-refactor spec 续）
//
// 返回所有 webdav enabled mounts 的 manifest snapshot（含 virtual tree + container map）。
// 前端 WebDavAutomationTestsDetail.vue 用此 manifest 动态生成测试用例，
// 避免硬编码 /webdav/01-plain-media/video/ 等路径。
//
// Query params：
//   - mount (可选): 指定单个 mount name（不指定则返回所有 enabled mounts）
//
// Response:
//   {
//     "mounts": [
//       {
//         "name": "automation",
//         "mount_path": "/d/automation",
//         "root_path": "...",
//         "webdav_path": "/d/automation",
//         "is_default": false,
//         "index_ready": true,
//         "manifest": { ... }   // webdav.ManifestSnapshot
//       },
//       ...
//     ]
//   }
func (s *Server) handleWebDavManifestGin(c *gin.Context) {
	mountNameFilter := c.Query("mount")

	if len(s.webdavFSByMount) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":  "no webdav mounts enabled",
			"mounts": []any{},
		})
		return
	}

	type mountManifestDTO struct {
		Name       string                  `json:"name"`
		MountPath  string                  `json:"mount_path"`
		RootPath   string                  `json:"root_path"`
		WebdavPath string                  `json:"webdav_path"`
		IsDefault  bool                    `json:"is_default"`
		Manifest   map[string]any          `json:"manifest"`
	}

	mounts := make([]mountManifestDTO, 0, len(s.webdavFSByMount))
	for name, entry := range s.webdavFSByMount {
		if mountNameFilter != "" && name != mountNameFilter {
			continue
		}
		// 拿 manifest snapshot
		snap := entry.fs.GetManifest()
		// 序列化为通用 map（避免 import cycle / 复杂类型）
		b, _ := json.Marshal(snap)
		var manifestMap map[string]any
		_ = json.Unmarshal(b, &manifestMap)

		dto := mountManifestDTO{
			Name:       name,
			WebdavPath: entry.webdavPath,
			Manifest:   manifestMap,
		}
		if entry.mount != nil {
			dto.MountPath = entry.mount.MountPath
			dto.RootPath = entry.mount.RootPath
			dto.IsDefault = entry.mount.Name == mount.NamePrimary
		}
		mounts = append(mounts, dto)
	}

	c.JSON(http.StatusOK, gin.H{
		"mounts":      mounts,
		"server_base": fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Server.Port),
	})
}

func (s *Server) handleIndexClearGin(c *gin.Context) {
	s.mobileSvc.ClearIndex()
	c.JSON(http.StatusOK, gin.H{"status": "cleared"})
}

func (s *Server) handleStreamExternalFileGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))

	slog.Info("API: stream external file", "path", queryPath)

	err := s.mobileSvc.StreamExternalFile(c.Writer, c.Request, queryPath)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}
}

func (s *Server) handleRemoteInfoGin(c *gin.Context) {
	cfg := s.cfg
	port := s.actualPort
	if port <= 0 {
		port = cfg.Server.Port
	}

	webdavInfo := gin.H{
		"enabled":  cfg.Webdav.Root != "",
		"username": cfg.Webdav.Username,
		"root":     cfg.Webdav.Root,
	}
	if cfg.Webdav.Root != "" {
		root := cfg.Webdav.Root
		if root == "" {
			root = "/webdav/"
		}
		if !strings.HasPrefix(root, "/") {
			root = "/" + root
		}
		if !strings.HasSuffix(root, "/") {
			root += "/"
		}
		webdavInfo["url"] = fmt.Sprintf("http://127.0.0.1:%d%s", port, root)
	} else {
		webdavInfo["url"] = ""
	}

	openlistSites := make(map[string]interface{})
	builtinOrder := []string{}
	for siteId, siteCfg := range cfg.Proxy.Sites {
		proxyURL := fmt.Sprintf("http://127.0.0.1:%d/openlist/sites/%s/", port, siteId)
		openlistSites[siteId] = map[string]interface{}{
			"host":        siteCfg.Host,
			"description": siteCfg.Description,
			"proxyUrl":    proxyURL,
			"built_in":    siteCfg.BuiltIn,
		}
		if siteCfg.BuiltIn {
			builtinOrder = append([]string{siteId}, builtinOrder...)
		} else {
			builtinOrder = append(builtinOrder, siteId)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"webdav":        webdavInfo,
		"openlistSites": openlistSites,
		"openlistOrder": builtinOrder,
	})
}

func (s *Server) handleListOpenlistSitesGin(c *gin.Context) {
	sites := make(map[string]interface{})
	builtinOrder := []string{}
	for siteId, siteCfg := range s.cfg.Proxy.Sites {
		sites[siteId] = map[string]interface{}{
			"host":        siteCfg.Host,
			"description": siteCfg.Description,
			"built_in":    siteCfg.BuiltIn,
		}
		if siteCfg.BuiltIn {
			builtinOrder = append([]string{siteId}, builtinOrder...)
		} else {
			builtinOrder = append(builtinOrder, siteId)
		}
	}
	c.JSON(http.StatusOK, gin.H{"sites": sites, "order": builtinOrder})
}

func (s *Server) handleAddOpenlistSiteGin(c *gin.Context) {
	var req struct {
		SiteID      string `json:"siteId"`
		Host        string `json:"host"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if !isValidSiteID(req.SiteID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "site id must contain only letters, digits and underscores"})
		return
	}
	if req.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host is required"})
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	if _, exists := s.cfg.Proxy.Sites[req.SiteID]; exists {
		c.JSON(http.StatusConflict, gin.H{"error": "site id already exists"})
		return
	}

	if s.cfg.Proxy.Sites == nil {
		s.cfg.Proxy.Sites = make(map[string]types.ProxySiteConfig)
	}
	s.cfg.Proxy.Sites[req.SiteID] = types.ProxySiteConfig{
		Host:        req.Host,
		Description: req.Description,
	}

	if err := s.writeConfigToFile(); err != nil {
		slog.Error("Failed to write config after adding openlist site", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "site added"})
}

func (s *Server) handleUpdateOpenlistSiteGin(c *gin.Context) {
	siteId := c.Param("id")

	if !isValidSiteID(siteId) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "site id must contain only letters, digits and underscores"})
		return
	}

	var req struct {
		Host        string `json:"host"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	if _, exists := s.cfg.Proxy.Sites[siteId]; !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}

	s.cfg.Proxy.Sites[siteId] = types.ProxySiteConfig{
		Host:        req.Host,
		Description: req.Description,
	}

	if err := s.writeConfigToFile(); err != nil {
		slog.Error("Failed to write config after updating openlist site", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "site updated"})
}

func (s *Server) handleDeleteOpenlistSiteGin(c *gin.Context) {
	siteId := c.Param("id")

	if !isValidSiteID(siteId) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "site id must contain only letters, digits and underscores"})
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	if _, exists := s.cfg.Proxy.Sites[siteId]; !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}

	delete(s.cfg.Proxy.Sites, siteId)

	if err := s.writeConfigToFile(); err != nil {
		slog.Error("Failed to write config after deleting openlist site", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "site deleted"})
}

func (s *Server) handleFileExistsGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	exists, err := s.mobileSvc.FileExists(queryPath)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"exists": exists})
}

func (s *Server) handleEncryptOutputExistsGin(c *gin.Context) {
	sourcePath := utils.DecodeGinQueryParam(c.Query("sourcePath"))
	targetDir := utils.DecodeGinQueryParam(c.Query("targetDir"))
	if sourcePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourcePath is required"})
		return
	}
	exists, outputPath, err := s.mobileSvc.CheckEncryptOutputExists(sourcePath, targetDir)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"exists": exists, "outputPath": outputPath})
}

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

// 🆕 2026-06-16: GET /api/logs/recent — 返回 slog ring buffer 最近 N 条
//   用途：WS 失败降级 http-poll 时，前端 devlogs 仍能看到后端日志
//   参数：?since=ISO8601（可选；不传则返回全量）
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

// 🆕 2026-06-11 v7：接收前端自动化测试分析结果上报
// 用途：Tasks.vue 「查看报告」按钮触发，把 localStorage 历史聚合 + 错误聚类
//   通过 fetch 异步上报到后端，dev console 同时输出
//   后端把 payload 写到 log（聚合分析用） + 返回 ack
// 不阻塞前端 UI（fire-and-forget + silent fail）
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

// ============= ECv4 Sparse Container Capacity Boundary Test =============

// handleSparseContainerWriteGin 写一个 sparse 虚拟容器（默认 100×128GB）
//
// POST /api/dev/sparse-container
// Body: {"fragmentCount": 100, "fragmentSizeGB": 128, "physicalChunkMB": 0, "containerType": 1}
// Response: SparseResult (virtual/physical/manifest size + isSparse)
func (s *Server) handleSparseContainerWriteGin(c *gin.Context) {
	var req struct {
		FragmentCount   int    `json:"fragmentCount"`
		FragmentSizeGB  int    `json:"fragmentSizeGB"`
		PhysicalChunkMB int    `json:"physicalChunkMB"`
		ContainerType   uint16 `json:"containerType"`
		BaseName        string `json:"baseName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}

	outputDir := "/tmp/encv-sparse-test"
	if req.BaseName == "" {
		req.BaseName = fmt.Sprintf("sparse-%d", time.Now().Unix())
	}

	// 1GB = 1024^3
	fragmentSize := int64(req.FragmentSizeGB) * 1024 * 1024 * 1024
	if fragmentSize <= 0 {
		fragmentSize = 128 * 1024 * 1024 * 1024
	}

	cfg := testutil.SparseContainerConfig{
		OutputDir:       outputDir,
		BaseName:        req.BaseName,
		FragmentCount:   req.FragmentCount,
		FragmentSize:    fragmentSize,
		PhysicalChunkMB: req.PhysicalChunkMB,
		ContainerType:   req.ContainerType,
		CipherMode:      0,
	}
	if cfg.ContainerType == 0 {
		cfg.ContainerType = 1
	}
	if cfg.FragmentCount == 0 {
		cfg.FragmentCount = 100
	}

	res, err := testutil.WriteSparseVirtualContainer(cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	slog.Info("[sparse-container] wrote",
		"baseName", req.BaseName,
		"virtualGB", res.VirtualTotal/(1024*1024*1024),
		"physicalKB", res.PhysicalUsed/1024,
		"isSparse", res.IsSparse,
		"durationMs", res.DurationMs,
	)
	c.JSON(http.StatusOK, res)
}

// handleSparseContainerProbeGin 探测 1 个 fragment 的读路径
//
// GET /api/dev/sparse-container/probe?mainPath=/tmp/.../x.sccg&fragmentIdx=0&fragmentSizeGB=128
// Response: EdgeProbeResult (seek/read duration + heap + physical/virtual size)
func (s *Server) handleSparseContainerProbeGin(c *gin.Context) {
	mainPath := c.Query("mainPath")
	if mainPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mainPath required"})
		return
	}
	fragmentIdx := 0
	if v := c.Query("fragmentIdx"); v != "" {
		fmt.Sscanf(v, "%d", &fragmentIdx)
	}
	fragmentSizeGB := 128
	if v := c.Query("fragmentSizeGB"); v != "" {
		fmt.Sscanf(v, "%d", &fragmentSizeGB)
	}
	fragmentSize := int64(fragmentSizeGB) * 1024 * 1024 * 1024

	probe, err := testutil.ReadSparseContainerEdgeProbe(mainPath, fragmentIdx, fragmentSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	slog.Info("[sparse-container] probe",
		"mainPath", mainPath,
		"fragmentIdx", fragmentIdx,
		"seekMs", probe.SeekDurationMs,
		"readMs", probe.ReadDurationMs,
		"heapInUseKB", probe.HeapInUseKB,
	)
	c.JSON(http.StatusOK, probe)
}

// handleSparseContainerCleanupGin 清理 sparse 容器产物
//
// DELETE /api/dev/sparse-container?baseName=xxx
// 默认清理 /tmp/encv-sparse-test/ 下所有 .sccg 产物
func (s *Server) handleSparseContainerCleanupGin(c *gin.Context) {
	baseName := c.Query("baseName")
	outputDir := "/tmp/encv-sparse-test"

	if baseName == "" {
		// 清理整个目录
		if err := os.RemoveAll(outputDir); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		slog.Info("[sparse-container] cleaned up entire output dir", "dir", outputDir)
	} else {
		if err := testutil.CleanupSparseContainer(outputDir, baseName); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		slog.Info("[sparse-container] cleaned up", "baseName", baseName)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "baseName": baseName, "outputDir": outputDir})
}

func (s *Server) handleTagsListGin(c *gin.Context) {
	allTags := GlobalTagStore.GetAllTags()
	type tagEntry struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	result := make([]tagEntry, 0, len(allTags))
	for name, count := range allTags {
		result = append(result, tagEntry{Name: name, Count: count})
	}
	c.JSON(http.StatusOK, gin.H{"tags": result})
}

func (s *Server) handleTagsMutateGin(c *gin.Context) {
	var req struct {
		Path   string `json:"path"`
		Tag    string `json:"tag"`
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	if req.Path == "" || req.Tag == "" || req.Action == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path, tag and action are required"})
		return
	}

	switch req.Action {
	case "add":
		GlobalTagStore.AddTag(req.Path, req.Tag)
		c.JSON(http.StatusOK, gin.H{"message": "tag added"})
	case "remove":
		GlobalTagStore.RemoveTag(req.Path, req.Tag)
		c.JSON(http.StatusOK, gin.H{"message": "tag removed"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "action must be 'add' or 'remove'"})
	}
}

type PluginMeta struct {
	Name                  string   `json:"name"`
	SupportedExtensions   []string `json:"supportedExtensions"`
	SupportedMimePrefixes []string `json:"supportedMimePrefixes"`
	ContainerExtension    string   `json:"containerExtension"`
	TaskOptions           gin.H    `json:"taskOptions"`
}

func (s *Server) handlePluginsGin(c *gin.Context) {
	var metas []PluginMeta
	for _, p := range plugins.Plugins {
		opts := p.GetTaskOptions()

		// 🆕 2026-06-17：nil 兜底为 []string{}
		// alist_encrypt 故意 SupportedExtensions() 返回 nil（"处理所有文件"语义）
		// 但 JSON 序列化为 null 后，前端模板 `p.supportedExtensions.length` 抛
		// `Cannot read properties of null (reading 'length')` 崩溃
		// → 在 API 输出层强制兜底为非 nil 空数组，前端安全访问
		supportedExts := p.SupportedExtensions()
		if supportedExts == nil {
			supportedExts = []string{}
		}
		supportedMimes := p.SupportedMimePrefixes()
		if supportedMimes == nil {
			supportedMimes = []string{}
		}

		metas = append(metas, PluginMeta{
			Name:                  p.Name(),
			SupportedExtensions:   supportedExts,
			SupportedMimePrefixes: supportedMimes,
			ContainerExtension:    p.GetContainerExtension(),
			TaskOptions:           taskOptionsToGinH(opts),
		})
	}
	c.JSON(200, gin.H{"plugins": metas})
}

func taskOptionsToGinH(opts pluginInterfaces.TaskOptions) gin.H {
	return gin.H{
		"passwordStrategy":     string(opts.PasswordStrategy),
		"supportVersionSelect": opts.SupportVersionSelect,
		"supportedVersions":    opts.SupportedVersions,
		"defaultVersion":       opts.DefaultVersion,
		"extraFields":          opts.ExtraFields,
	}
}

func (s *Server) handlePredictPluginGin(c *gin.Context) {
	var req struct {
		SourcePath string `json:"sourcePath"`
		Type       string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	var candidates []plugins.PluginCandidate
	if req.Type == "encrypt" {
		// ★ 关键修复: 先用 SafeResolveToAbsPath 把前端传来的路径解析为绝对路径，
		// 防止 mobile 模式下 servingDir=/storage/emulated/0 时，插件拿不到真实文件
		absSourcePath, err := s.resolveUserPath(req.SourcePath)
		if err != nil {
			c.JSON(200, gin.H{"candidates": []gin.H{}, "pluginName": nil, "taskOptions": nil, "error": fmt.Sprintf("invalid path: %v", err)})
			return
		}
		candidates = plugins.FindAllEncryptingPlugins(absSourcePath)
	} else {
		// ★ 关键修复: 同上，解密前必须先 resolve 绝对路径
		absSourcePath, err := s.resolveUserPath(req.SourcePath)
		if err != nil {
			c.JSON(200, gin.H{"candidates": []gin.H{}, "pluginName": nil, "taskOptions": nil, "error": fmt.Sprintf("invalid path: %v", err)})
			return
		}
		targetPlugin, err := plugins.FindDecryptingPlugin(absSourcePath)
		if err != nil || targetPlugin == nil {
			c.JSON(200, gin.H{"candidates": []gin.H{}, "pluginName": nil, "taskOptions": nil, "error": err.Error()})
			return
		}
		opts := targetPlugin.GetTaskOptions()
		candidates = []plugins.PluginCandidate{{
			Plugin: targetPlugin, Name: targetPlugin.Name(), MatchType: "container", Priority: 0,
		}}
		c.JSON(200, gin.H{
			"candidates":  []gin.H{{"name": targetPlugin.Name(), "matchType": "container", "priority": 0, "taskOptions": taskOptionsToGinH(opts)}},
			"pluginName":  targetPlugin.Name(),
			"taskOptions": taskOptionsToGinH(opts),
		})
		return
	}

	candidateList := make([]gin.H, 0, len(candidates))
	for _, cand := range candidates {
		opts := cand.Plugin.GetTaskOptions()
		candidateList = append(candidateList, gin.H{
			"name":        cand.Name,
			"matchType":   cand.MatchType,
			"priority":    cand.Priority,
			"taskOptions": taskOptionsToGinH(opts),
		})
	}

	firstName := ""
	var firstOptsH gin.H
	if len(candidateList) > 0 {
		firstName = candidateList[0]["name"].(string)
		firstOptsH = candidateList[0]["taskOptions"].(gin.H)
	}

	c.JSON(200, gin.H{
		"candidates":  candidateList,
		"pluginName":  firstName,
		"taskOptions": firstOptsH,
	})
}

func (s *Server) handleContainerExtensionsGin(c *gin.Context) {
	extMap := plugins.GetContainerExtensionsMap()
	conflicts := plugins.ValidateExtensionUniqueness()

	var conflictList []gin.H
	for _, c := range conflicts {
		conflictList = append(conflictList, gin.H{
			"extension":   c.Extension,
			"pluginNames": c.PluginNames,
		})
	}

	c.JSON(200, gin.H{
		"extensions": extMap,
		"conflicts":  conflictList,
	})
}

func (s *Server) writeSSEEvent(c *gin.Context, flusher http.Flusher, data string) {
	c.Writer.Write([]byte("data: " + data + "\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

func (s *Server) handleListFilesStreamGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	if queryPath == "" {
		queryPath = "/"
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		flusher = nil
	}

	// 🆕 2026-06-15 multi-mount 适配：mount 虚拟根 /d 走 mount list（与 handleListFilesGin 同步）
	if (queryPath == "/d" || queryPath == "/d/") && s.mountRegistry != nil {
		files := mountListAsFiles(s.mountRegistry.List())
		for _, fi := range files {
			data, _ := json.Marshal(fi)
			s.writeSSEEvent(c, flusher, string(data))
		}
		s.writeSSEEvent(c, flusher, `[DONE]`)
		return
	}

	absPath, err := s.resolveUserPath(queryPath)
	if err != nil {
		s.writeSSEEvent(c, flusher, `{"error":"invalid path"}`)
		s.writeSSEEvent(c, flusher, `[DONE]`)
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		errMsg := fmt.Sprintf(`{"error":"cannot read directory: %s"}`, err.Error())
		s.writeSSEEvent(c, flusher, errMsg)
		s.writeSSEEvent(c, flusher, `[DONE]`)
		return
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		filePath := queryPath + "/" + entry.Name()
		if queryPath == "/" {
			filePath = "/" + entry.Name()
		}

		info, err := entry.Info()
		if err != nil {
			fi := mobileservice.FileInfo{
				Name:        entry.Name(),
				Path:        filePath,
				IsDirectory: entry.IsDir(),
				Size:        0,
				Modified:    "",
			}
			data, _ := json.Marshal(fi)
			s.writeSSEEvent(c, flusher, string(data))
			continue
		}

		isEncrypted := false
		if !entry.IsDir() {
			entryAbsPath := filepath.Join(absPath, entry.Name())
			if _, detectErr := detector.DetectContainer(entryAbsPath); detectErr == nil {
				isEncrypted = true
			}
		}

		fi := mobileservice.FileInfo{
			Name:        entry.Name(),
			Path:        filePath,
			IsDirectory: entry.IsDir(),
			IsEncrypted: isEncrypted,
			Size:        info.Size(),
			Modified:    info.ModTime().Format(time.RFC3339),
		}
		data, _ := json.Marshal(fi)
		s.writeSSEEvent(c, flusher, string(data))
	}

	s.writeSSEEvent(c, flusher, `[DONE]`)
}

func (s *Server) handleAlistEncryptStreamGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	password := c.Query("password")
	if queryPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'path' query parameter is required"})
		return
	}

	absPath, err := s.resolveUserPath(queryPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}

	slog.Info("API: alist-encrypt stream", "path", absPath)

	// 走统一范式：构造 FileContentProvider，调 ContentHandler.ServeFile
	// 与 v4 容器预览共享同一套 HTTP 协议处理（Range/206/Content-Length/Content-Range）
	var plugin alistencrypt.AlistEncryptPlugin
	rc, size, _, showName, err := plugin.Stream(absPath, password)
	if err != nil {
		slog.Error("API: alist-encrypt stream open failed", "error", err)
		writeServiceErrorGin(c, err)
		return
	}
	sr, ok := rc.(*alistencrypt.SeekableDecryptReader)
	if !ok {
		_ = rc.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal: unexpected reader type"})
		return
	}
	prov := alistencrypt.NewAlistEncryptFileProvider(sr, size, showName)
	defer prov.Close()
	s.contentHandler.ServeFile(c.Writer, c.Request, prov)
}

func (s *Server) handleAlistDecodeFilenameGin(c *gin.Context) {
	encoded := utils.DecodeGinQueryParam(c.Query("encoded"))
	password := c.Query("password")
	encType := c.DefaultQuery("enc_type", "aesctr")

	if encoded == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'encoded' query parameter is required"})
		return
	}

	plainName := alistencrypt.DecodeName(encoded, password, encType)
	c.JSON(http.StatusOK, gin.H{
		"plain_name": plainName,
		"success":    plainName != "",
	})
}

func (s *Server) handlePluginFilesStreamGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	if queryPath == "" {
		queryPath = "/"
	}
	extensionsStr := c.Query("extensions")
	if extensionsStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'extensions' query parameter is required"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		flusher = nil
	}

	absPath, err := s.resolveUserPath(queryPath)
	if err != nil {
		s.writeSSEEvent(c, flusher, `{"error":"invalid path"}`)
		s.writeSSEEvent(c, flusher, `[DONE]`)
		return
	}

	extSet := make(map[string]bool)
	for _, ext := range strings.Split(extensionsStr, ",") {
		e := strings.TrimSpace(strings.ToLower(ext))
		if e != "" {
			extSet[e] = true
		}
	}

	const maxResults = 500
	count := 0

	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if count >= maxResults {
			return fs.SkipAll
		}

		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		if !extSet[ext] {
			return nil
		}

		relPath, _ := filepath.Rel(absPath, path)
		filePath := queryPath + "/" + relPath
		if queryPath == "/" {
			filePath = "/" + relPath
		}

		info, err := d.Info()
		if err != nil {
			fi := mobileservice.FileInfo{
				Name:        name,
				Path:        filePath,
				IsDirectory: false,
				Size:        0,
				Modified:    "",
			}
			data, _ := json.Marshal(fi)
			s.writeSSEEvent(c, flusher, string(data))
			count++
			return nil
		}

		isEncrypted := false
		if _, detectErr := detector.DetectContainer(path); detectErr == nil {
			isEncrypted = true
		}

		fi := mobileservice.FileInfo{
			Name:        name,
			Path:        filePath,
			IsDirectory: false,
			IsEncrypted: isEncrypted,
			Size:        info.Size(),
			Modified:    info.ModTime().Format(time.RFC3339),
		}
		data, _ := json.Marshal(fi)
		s.writeSSEEvent(c, flusher, string(data))
		count++
		return nil
	})

	if err != nil && count < maxResults {
		errMsg := fmt.Sprintf(`{"error":"walk failed: %s"}`, err.Error())
		s.writeSSEEvent(c, flusher, errMsg)
	}

	s.writeSSEEvent(c, flusher, `[DONE]`)
}

// ========== 数据库管理 API（备份/恢复/导入/导出/跨引擎迁移） ==========

// handleDatabaseInfo 获取当前数据库引擎信息。
//
// 返回：
//
//	{
//	  "engine": "sqlite",          // 当前引擎名
//	  "concurrency": 1,             // 推荐并发度
//	  "taskCount": 1234,            // 任务总数
//	  "hasCalibration": true        // 是否有校准数据
//	}
func (s *Server) handleDatabaseInfo(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	actualEngine := tm.GetStoreEngine()
	requestedEngine := s.cfg.Database.Engine
	if requestedEngine == "" {
		requestedEngine = "sqlite"
	}
	concurrency := tm.GetStoreConcurrency()

	fallbackReason := ""
	if requestedEngine != actualEngine {
		if requestedEngine == "turso" || requestedEngine == "libsql" {
			fallbackReason = "当前平台不支持 Turso/LibSQL 引擎，已自动回退到 SQLite"
		} else {
			fallbackReason = "引擎初始化失败，已自动回退到 SQLite"
		}
	}

	availableEngines := s.getAvailableEngines()

	totalTasks := 0
	hasCalibration := false
	if store := tm.GetStore(); store != nil {
		if tasks, err := store.ListTasks(tasksystem.TaskFilter{Limit: 0}); err == nil {
			totalTasks = len(tasks)
		}
		if cal, err := store.GetCalibration(); err == nil && cal != nil {
			hasCalibration = true
		}
	} else {
		tasks, _ := tm.ListPaginated("", 0, 0)
		totalTasks = len(tasks)
	}

	c.JSON(http.StatusOK, gin.H{
		"engine":           actualEngine,
		"requestedEngine":  requestedEngine,
		"fallbackReason":   fallbackReason,
		"availableEngines": availableEngines,
		"concurrency":      concurrency,
		"taskCount":        totalTasks,
		"hasCalibration":   hasCalibration,
	})
}

type EngineInfo struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func (s *Server) getAvailableEngines() []EngineInfo {
	isMobile := runtime.GOOS == "android" || runtime.GOOS == "ios"
	return []EngineInfo{
		{
			Name:      "sqlite",
			Label:     "SQLite",
			Available: true,
		},
		{
			Name:      "libsql",
			Label:     "LibSQL",
			Available: !isMobile,
			Reason:    func() string {
				if isMobile {
					return "移动端暂不支持 LibSQL 引擎"
				}
				return ""
			}(),
		},
		{
			Name:      "turso",
			Label:     "Turso",
			Available: false,
			Reason:    "Turso 云服务暂未开放",
		},
	}
}

// handleDatabaseExport 导出数据库为 JSON。
//
// 直接返回 JSON 格式的 DatabaseDump，可用于：
//   - 下载备份
//   - 跨引擎迁移（从 SQLite 导出，导入到 Turso）
func (s *Server) handleDatabaseExport(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()

	dump, err := tm.ExportDatabase()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 设置下载文件名
	filename := fmt.Sprintf("encv-db-%s-%s.json",
		dump.Engine,
		dump.ExportedAt.Format("20060102-150405"),
	)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/json")

	c.JSON(http.StatusOK, dump)
}

// handleDatabaseImport 从 JSON 导入数据库（全量替换）。
//
// 用于：
//   - 恢复备份
//   - 跨引擎迁移（从 SQLite 迁移到 Turso）
//
// 警告：导入会清空所有现有数据！
func (s *Server) handleDatabaseImport(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()

	var dump tasksystem.DatabaseDump
	if err := c.ShouldBindJSON(&dump); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}

	if dump.Version == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dump: missing version"})
		return
	}

	slog.Info("API: database import",
		"sourceEngine", dump.Engine,
		"taskCount", len(dump.Tasks),
		"trashCount", len(dump.Trash),
		"snapshotCount", len(dump.Snapshots),
		"metricCount", len(dump.Metrics),
	)

	if err := tm.ImportDatabase(&dump); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("API: database import completed")
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"imported": gin.H{
			"tasks":     len(dump.Tasks),
			"trash":     len(dump.Trash),
			"snapshots": len(dump.Snapshots),
			"metrics":   len(dump.Metrics),
		},
	})
}

// handleDatabaseBackup 备份数据库到本地文件。
//
// 不同于 export（返回 JSON 给前端），backup 直接在后端写入备份文件，
// 适合大数据库不经过前端内存中转。
func (s *Server) handleDatabaseBackup(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()

	dump, err := tm.ExportDatabase()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 生成备份文件路径（在 servingDir 下的 .backups 目录）
	backupDir := filepath.Join(s.servingDir, ".encv-backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create backup dir: " + err.Error()})
		return
	}

	filename := fmt.Sprintf("encv-db-%s-%s.json",
		dump.Engine,
		dump.ExportedAt.Format("20060102-150405"),
	)
	filePath := filepath.Join(backupDir, filename)

	data, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal: " + err.Error()})
		return
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write file: " + err.Error()})
		return
	}

	slog.Info("API: database backup created", "path", filePath, "size", len(data))

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"path":   filePath,
		"size":   len(data),
		"name":   filename,
	})
}

// handleDatabaseRestore 从本地备份文件恢复数据库。
//
// 请求体：{"path": "/path/to/backup.json"}
// 警告：恢复会清空所有现有数据！
func (s *Server) handleDatabaseRestore(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()

	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	if req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	// 安全检查：路径必须在 servingDir 内（防止路径穿越）
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	absServingDir, _ := filepath.Abs(s.servingDir)
	if !strings.HasPrefix(absPath, absServingDir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path outside serving directory"})
		return
	}

	// 读取文件
	data, err := os.ReadFile(absPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read file: " + err.Error()})
		return
	}

	var dump tasksystem.DatabaseDump
	if err := json.Unmarshal(data, &dump); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}

	slog.Info("API: database restore",
		"path", absPath,
		"sourceEngine", dump.Engine,
		"taskCount", len(dump.Tasks),
	)

	if err := tm.ImportDatabase(&dump); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("API: database restore completed")
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"restored": gin.H{
			"tasks":     len(dump.Tasks),
			"trash":     len(dump.Trash),
			"snapshots": len(dump.Snapshots),
			"metrics":   len(dump.Metrics),
		},
	})
}

// ─── 向量搜索 API（Turso 原生向量检索 + 中文 bigram）────────────

// handleVectorSearchTasksGin 任务语义搜索。
//
// 基于 Turso 原生向量检索（vector_distance_cos），支持中文模糊搜索。
// 相比简单的字符串匹配，向量搜索能找到语义相近的结果。
//
// Query 参数：
//   - q: 搜索关键词
//   - limit: 返回数量（默认 50）
func (s *Server) handleVectorSearchTasksGin(c *gin.Context) {
	query := c.Query("q")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if query == "" {
		c.JSON(http.StatusOK, gin.H{"tasks": []gin.H{}, "vector_search": false})
		return
	}

	if s.searchSvc == nil {
		// 向量搜索不可用时，降级为普通字符串匹配
		tasks, _ := s.mobileSvc.GetTaskManager().ListPaginated("", 0, 200)
		var matched []mobileservice.MobileTask
		q := strings.ToLower(query)
		for _, t := range tasks {
			if strings.Contains(strings.ToLower(t.SourcePath), q) ||
				strings.Contains(strings.ToLower(t.Type), q) {
				matched = append(matched, *t)
			}
		}
		c.JSON(http.StatusOK, gin.H{"tasks": matched, "vector_search": false})
		return
	}

	ctx := context.Background()
	results, err := s.searchSvc.SearchTasks(ctx, query, limit)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	// 把搜索结果转换成前端能识别的任务格式
	// 从 TaskManager 获取完整任务信息
	tm := s.mobileSvc.GetTaskManager()
	var tasks []*mobileservice.MobileTask
	for _, r := range results {
		if r.Score < 0.05 {
			continue // 过滤掉相似度太低的结果
		}
		if task, err := tm.Get(r.RefID); err == nil && task != nil {
			tasks = append(tasks, task)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":         tasks,
		"vector_search": true,
		"total":         len(results),
	})
}

// handleVectorSearchFilesGin 文件语义搜索。
//
// 结合现有文件系统搜索 + 向量重排序，提升中文搜索效果。
//
// Query 参数：
//   - q: 搜索关键词
//   - path: 搜索路径
//   - recursive: 是否递归搜索（true/false）
//   - limit: 返回数量
func (s *Server) handleVectorSearchFilesGin(c *gin.Context) {
	query := c.Query("q")
	path := utils.DecodeGinQueryParam(c.Query("path"))
	recursive := c.Query("recursive") == "true"
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if query == "" {
		c.JSON(http.StatusOK, gin.H{"files": []gin.H{}, "vector_search": false})
		return
	}

	// 先调用现有文件搜索拿到候选结果（遵守递归设置）
	files, err := s.mobileSvc.SearchFiles(path, query, recursive)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	// 如果向量搜索可用，且结果数量较多，用向量重排序
	if s.searchSvc != nil && len(files) > 3 {
		ctx := context.Background()
		// 先把结果批量索引到向量搜索
		batch := make([]vectorsearch.FileIndexItem, 0, len(files))
		for _, f := range files {
			batch = append(batch, vectorsearch.FileIndexItem{
				Path:  f.Path,
				Name:  f.Name,
				Size:  f.Size,
				MTime: f.Modified,
			})
		}
		s.searchSvc.IndexFilesBatch(ctx, batch)

		// 然后用向量搜索重新排序
		results, vErr := s.searchSvc.SearchFiles(ctx, query, limit)
		if vErr == nil && len(results) > 0 {
			// 按向量搜索结果重排序
			fileMap := make(map[string]mobileservice.FileInfo)
			for _, f := range files {
				fileMap[f.Path] = f
			}

			var sortedFiles []mobileservice.FileInfo
			for _, r := range results {
				if r.Score < 0.05 {
					continue
				}
				if f, ok := fileMap[r.RefID]; ok {
					sortedFiles = append(sortedFiles, f)
				}
			}

			// 把没进向量结果的文件也附在后面
			if len(sortedFiles) < len(files) {
				sortedSet := make(map[string]bool)
				for _, f := range sortedFiles {
					sortedSet[f.Path] = true
				}
				for _, f := range files {
					if !sortedSet[f.Path] {
						sortedFiles = append(sortedFiles, f)
					}
				}
			}

			c.JSON(http.StatusOK, gin.H{
				"files":         sortedFiles,
				"vector_search": true,
				"total":         len(sortedFiles),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"files":         files,
		"vector_search": false,
		"total":         len(files),
	})
}

// handleSearchStatsGin 获取搜索索引统计信息。
func (s *Server) handleSearchStatsGin(c *gin.Context) {
	if s.searchSvc == nil {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"engine":    "none",
		})
		return
	}

	stats := s.searchSvc.Stats()
	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"stats":     stats,
	})
}
