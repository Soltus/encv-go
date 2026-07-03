package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Soltus/encv-go/internal/auth"
	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/openlist"
	"github.com/Soltus/encv-go/internal/openlist/web"
	"github.com/Soltus/encv-go/internal/routes"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(s *Server, r *gin.Engine) {
	r.GET("/ping", s.handlePingGin)
	r.GET("/health", s.handleHealthGin)
	r.GET("/api/runtime", s.handleRuntimeAPI)
	r.GET("/api/diagnose", s.handleDiagnoseGin)
	// 🆕 2026-07-03：特色微服务内核 HTTP API（spec kernel-integration）
	//   - GET  /api/kernel/services  : 列出已注册的 kernel.Service
	//   - GET  /api/kernel/health    : 聚合 Health（任一失败返回 503）
	//   - POST /api/kernel/call      : 通过 kernel.Call 调用 service.method（dev only）
	r.GET("/api/kernel/services", s.handleKernelServicesGin)
	r.GET("/api/kernel/health", s.handleKernelHealthGin)
	r.POST("/api/kernel/call", s.handleKernelCallGin)

	// 🆕 2026-07-03：特色微服务内核 Lifecycle HTTP API（spec android-workmanager-split-start-stop Phase 1.3）
	//   - GET  /api/kernel/pools              : 列出受管 Pool 状态
	//   - POST /api/kernel/restore            : 手动触发 Restore（dev only，Stop+Start 循环）
	//   - GET  /api/kernel/lifecycle/stats    : Lifecycle 启停耗时 + 内存 + MemGuard
	//   - POST /api/kernel/lifecycle/start    : 启动 Lifecycle（dev only）
	//   - POST /api/kernel/lifecycle/stop     : 停止 Lifecycle（dev only，委托 in-flight 给 Ledger）
	//
	// 用户硬约束（spec §六）：
	//   - 启动 ≤ 500ms / 停止 ≤ 200ms（lifecycle/stats 暴露 lastStartDurationMs / lastStopDurationMs）
	//   - 内存守卫（lifecycle/stats 暴露 memGuardEnabled / memGuardTriggered）
	//   - 不消耗 TCP 端口（Lifecycle 是进程内对象）
	r.GET("/api/kernel/pools", s.handleKernelPoolsGin)
	r.POST("/api/kernel/restore", s.handleKernelRestoreGin)
	r.GET("/api/kernel/lifecycle/stats", s.handleKernelLifecycleStatsGin)
	r.POST("/api/kernel/lifecycle/start", s.handleKernelLifecycleStartGin)
	r.POST("/api/kernel/lifecycle/stop", s.handleKernelLifecycleStopGin)
	// 🆕 异步 job 提交（dev only）— Cypress restart-restore 测试用
	r.POST("/api/kernel/submit", s.handleKernelSubmitGin)

	// 🆕 2026-07-03：dev-only 进程自杀端点（spec android-workmanager-split-start-stop Phase 1.6.2）
	//   - POST /api/dev/kill-backend : 触发 os.Exit(1)，pm2 自动重启
	//   - 用途：Cypress E2E kernel-restart-restore 测试模拟进程崩溃
	//   - 非 dev 模式返回 403
	r.POST("/api/dev/kill-backend", s.handleKillBackendGin)
	r.GET("/stream", gin.WrapF(s.handleStreamRequest))
	r.GET("/decrypt", gin.WrapF(s.handleStreamRequest))
	r.GET("/preview/*filepath", gin.WrapH(http.StripPrefix("/preview", web.PreviewHandler())))
	r.GET("/api/config", s.handleGetConfigGin)
	r.PUT("/api/config", s.handlePutConfigGin)
	r.GET("/api/config/schema", s.handleConfigSchemaGin)
	r.GET("/api/files", s.handleListFilesGin)
	r.GET("/api/files/stream", s.handleListFilesStreamGin)
	r.GET("/api/files/plugin-stream", s.handlePluginFilesStreamGin)
	r.DELETE("/api/files", s.handleDeleteFileGin)
	r.POST("/api/files/mkdir", s.handleCreateDirectoryGin)
	r.POST("/api/files/upload", s.handleUploadFileGin)
	r.GET("/api/service-guard", s.handleServiceGuardGin)
	r.GET("/api/file", s.handleReadFileContentGin)
	r.GET("/api/file/text-preview-exts", s.handleTextPreviewExtsGin)
	r.GET("/api/file/info", s.handleFileInfoGin)
	r.POST("/api/file/rename", s.handleFileRenameGin)
	r.POST("/api/file/copy", s.handleFileCopyGin)
	r.POST("/api/file/move", s.handleFileMoveGin)
	r.PATCH("/api/file/rename", s.handleRenameFileGin)
	r.GET("/api/tasks", s.handleGetTasksGin)
	r.POST("/api/tasks", s.handleCreateTaskGin)
	r.POST("/api/tasks/batch", s.handleCreateTaskBatchGin)
	r.POST("/api/tasks/predict-plugin", s.handlePredictPluginGin)
	r.POST("/api/tasks/:id/cancel", s.handleCancelTaskGin)
	r.POST("/api/runs/:runId/cancel", s.handleCancelRunGin)
	r.POST("/api/runs/:runId/resume", s.handleResumeRunGin)
	r.POST("/api/tasks/resume-all", s.handleResumeAllPausedGin)
	r.GET("/api/runs/:runId/summary", s.handleGetRunSummaryGin)
	r.GET("/api/runs", s.handleListRunsGin)
	r.POST("/api/tasks/:id/retry", s.handleRetryTaskGin)
	r.POST("/api/tasks/:id/rollback", s.handleRollbackTaskGin)
	r.DELETE("/api/tasks/:id", s.handleRemoveTaskGin)
	r.DELETE("/api/tasks", s.handleClearCompletedTasksGin)
	r.GET("/api/trash", s.handleListTrashGin)
	r.POST("/api/trash/restore", s.handleRestoreTrashGin)
	r.DELETE("/api/trash/:id", s.handlePurgeTrashGin)
	r.DELETE("/api/trash", s.handleEmptyTrashGin)
	r.GET("/api/tasks/:id/performance", s.handleGetTaskPerformance)
	r.GET("/api/performance/calibration", s.handleGetCalibration)
	r.POST("/api/performance/calibration", s.handleRecalibrate)
	r.GET("/api/performance/history", s.handleGetPerformanceHistory)
	r.GET("/api/database/info", s.handleDatabaseInfo)
	r.GET("/api/database/export", s.handleDatabaseExport)
	r.POST("/api/database/import", s.handleDatabaseImport)
	r.POST("/api/database/backup", s.handleDatabaseBackup)
	r.POST("/api/database/restore", s.handleDatabaseRestore)
	r.POST("/api/webdav/test", s.handleTestWebDAVGin)
	r.GET("/api/webdav/test-local", s.handleTestLocalWebDAVGin)
	r.GET("/api/webdav/local-info", s.handleWebDavLocalInfoGin)
	r.GET("/api/webdav/manifest", s.handleWebDavManifestGin)
	r.GET("/api/remote/info", s.handleRemoteInfoGin)
	r.GET("/api/remote/openlist", s.handleListOpenlistSitesGin)
	r.POST("/api/remote/openlist", s.handleAddOpenlistSiteGin)
	r.PUT("/api/remote/openlist/:id", s.handleUpdateOpenlistSiteGin)
	r.DELETE("/api/remote/openlist/:id", s.handleDeleteOpenlistSiteGin)
	r.GET("/api/permissions", s.handlePermissionsGin)
	r.POST("/api/server/shutdown", s.handleServerShutdownGin)
	r.GET("/api/files/exists", s.handleFileExistsGin)
	r.GET("/api/files/encrypt-output-exists", s.handleEncryptOutputExistsGin)
	r.GET("/api/files/search", s.handleSearchFilesGin)
	r.GET("/api/files/search-fulltext", s.handleSearchFilesFullTextGin)
	r.GET("/api/files/search-fulltext/stats", s.handleFullTextIndexStatsGin)
	// 🆕 2026-07-03：FTS 索引重建任务化（spec fts-rebuild-task）
	//   - POST 创建 rebuild_fts_index 任务，走任务系统（自带进度/耗时/取消）
	//   - 返回 taskId 供前端订阅 task:progress / task:completed WS 事件
	r.POST("/api/files/search-fulltext/rebuild", s.handleFullTextIndexRebuildGin)
	r.GET("/api/files/tags", s.handleTagsListGin)
	r.POST("/api/files/tags", s.handleTagsMutateGin)
	r.GET("/api/search/tasks", s.handleVectorSearchTasksGin)
	r.GET("/api/search/files", s.handleVectorSearchFilesGin)
	r.GET("/api/search/stats", s.handleSearchStatsGin)
	r.GET("/api/index/stats", s.handleIndexStatsGin)
	r.POST("/api/index/rebuild", s.handleIndexRebuildGin)
	r.POST("/api/index/clear", s.handleIndexClearGin)
	r.GET("/api/stream/external", s.handleStreamExternalFileGin)
	r.GET("/api/build-info", s.handleBuildInfoGin)
	r.GET("/api/libraries", s.handleLibrariesGin)
	r.GET("/api/ffmpeg-status", s.handleFFmpegStatusGin)
	r.POST("/api/dev/automation-report", s.handleAutomationReportGin)
	r.POST("/api/dev/sparse-container", s.handleSparseContainerWriteGin)
	r.GET("/api/dev/sparse-container/probe", s.handleSparseContainerProbeGin)
	r.DELETE("/api/dev/sparse-container", s.handleSparseContainerCleanupGin)
	r.GET("/api/container/versions", s.handleGetContainerVersionsGin)
	r.GET("/api/plugins", s.handlePluginsGin)
	r.GET("/api/plugins/container-extensions", s.handleContainerExtensionsGin)
	r.GET("/api/alist-encrypt/stream", s.handleAlistEncryptStreamGin)
	r.GET("/api/alist-encrypt/decode-filename", s.handleAlistDecodeFilenameGin)
	r.POST("/api/logs", s.handleAPILogsGin)
	r.GET("/api/logs/recent", s.handleAPILogsRecentGin)
	r.GET("/api/mounts", s.handleListMountsGin)
	r.GET("/api/mounts/:id", s.handleGetMountGin)
	r.POST("/api/mounts", s.handleCreateMountGin)
	r.PUT("/api/mounts/:id", s.handleUpdateMountGin)
	r.DELETE("/api/mounts/:id", s.handleDeleteMountGin)
	r.POST("/api/mounts/:id/resolve", s.handleResolveMountPathGin)
	r.GET("/api/mounts/:id/usage", s.handleMountUsageGin)
	r.POST("/api/mock/generate", s.handleMockGenerateGin)
	r.POST("/api/mock/reset", s.handleMockResetGin)
	r.GET("/ws", gin.WrapF(s.handleWebSocket))

	s.registerAgentRoutes(r)

	r.Any("/api/preview/plugin-openlist/*filepath", s.handlePluginOpenlistProxyGin)
	r.Any("/api/preview/plugin-openlist", s.handlePluginOpenlistProxyGin)

	r.GET(routes.Admin, func(c *gin.Context) {
		c.Redirect(http.StatusFound, routes.FSProxy+"/")
	})

	loginRequired := s.cfg.Admin.Password != ""
	if loginRequired {
		s.jwtManager = auth.NewJWTManager(s.cfg.Admin.Password, 7*24*time.Hour)
		slog.Info("Admin service requires login")
	} else {
		slog.Info("Admin service running without authentication (password is empty)")
	}

	r.GET(routes.Login, s.handleLoginGin)
	r.POST(routes.Login, s.handleLoginGin)
	r.Any(routes.Logout, s.handleLogoutGin)

	adminGroup := r.Group(routes.Admin)
	if loginRequired {
		adminGroup.Use(JWTAuthMiddleware(s.jwtManager))
	}
	adminGroup.POST("/file/analyze", s.handleFileAnalyzeGin)
	adminGroup.POST("/file/rename", s.handleFileRenameGin)
	adminGroup.POST("/file/copy", s.handleFileCopyGin)
	adminGroup.POST("/file/move", s.handleFileMoveGin)

	fsProxyGroup := r.Group(routes.FSProxy)
	if loginRequired {
		fsProxyGroup.Use(JWTAuthMiddleware(s.jwtManager))
	}
	fsProxyGroup.Any("/*path", s.handleFSProxyGin)

	r.Any(routes.FSProxyAPI+"/*path", func(c *gin.Context) {
		path := c.Param("path")
		c.Redirect(http.StatusTemporaryRedirect, "/api"+path)
	})

	r.GET(routes.OpenListProxy+"/local/status", LocalOpenListStatusHandler())

	if len(s.cfg.Proxy.Sites) > 0 {
		multiSiteServer := openlist.NewMultiSiteServer(config.NewContext(context.Background(), s.cfg))
		proxyGin := NewProxyGin(s.cfg)

		r.GET(routes.OpenListProxy+"/sites", handleOpenlistSitesGin(multiSiteServer))
		r.POST(routes.OpenListProxy+"/set-token", handleSetSiteTokenGin(multiSiteServer))
		r.POST(routes.OpenListProxy+"/delete-token", handleDeleteTokenGin(multiSiteServer))
		r.POST(routes.OpenListProxy+"/set-expiry", handleSetExpiryGin(multiSiteServer))

		openlistGroup := r.Group(routes.OpenListProxy + "/sites")
		openlistGroup.Use(OpenlistSiteMiddleware(multiSiteServer))
		if loginRequired {
			openlistGroup.Use(JWTAuthMiddleware(s.jwtManager))
		}
		openlistGroup.GET("/:siteId/*path", handleOpenlistProxyGin(proxyGin))
		openlistGroup.HEAD("/:siteId/*path", handleOpenlistProxyGin(proxyGin))
		openlistGroup.POST("/:siteId/*path", handleOpenlistProxyGin(proxyGin))
		openlistGroup.PUT("/:siteId/*path", handleOpenlistProxyGin(proxyGin))
		openlistGroup.DELETE("/:siteId/*path", handleOpenlistProxyGin(proxyGin))
		openlistGroup.PATCH("/:siteId/*path", handleOpenlistProxyGin(proxyGin))
		openlistGroup.OPTIONS("/:siteId/*path", handleOpenlistProxyGin(proxyGin))
	}
}
