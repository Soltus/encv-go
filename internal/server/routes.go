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
