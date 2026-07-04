package server

// mobile_files.go — 文件 CRUD：list / delete / create / upload / rename / info / exists / encrypt_output_exists。

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/mount"
	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/gin-gonic/gin"
)

// pluginOpenlistUpstream plugin-openlist 独立 Vite dev server
// （由 pm2 app `plugin-openlist-vite` 拉起，独立于 encv-mobile vite :5173）
const pluginOpenlistUpstream = "http://127.0.0.1:5174"

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
