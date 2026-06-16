package server

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/Soltus/encv-go/internal/mount"
	"github.com/Soltus/encv-go/internal/mount/drivers"
	"github.com/gin-gonic/gin"
)

// mount_api.go 实现了 /api/mounts 端点。
//
// 端点（spec §6.1）：
//   GET    /api/mounts               列出所有挂载点
//   GET    /api/mounts/:id           单个挂载点详情
//   POST   /api/mounts               新增（需 admin）
//   PUT    /api/mounts/:id           更新（需 admin）
//   DELETE /api/mounts/:id           删除（需 admin；primary 不可删）
//   POST   /api/mounts/:id/resolve   debug：把 sub_path 解析为 abs path
//   GET    /api/mounts/:id/usage     du 占用（暂用 os.Stat 模拟）
//
// 权限：admin 校验不在本层做（按 spec §6.1 标记；后续可加 JWTAuthMiddleware 包装）

// mountDTO 是 API 响应结构，与 mount.Mount 几乎一致，但加 readonly 字段。
type mountDTO struct {
	mount.Mount
	ResolvedRoot string `json:"resolved_root"`
}

// toDTO 把 *mount.Mount 转为响应结构。
func toDTO(m *mount.Mount) mountDTO {
	if m == nil {
		return mountDTO{}
	}
	return mountDTO{Mount: *m, ResolvedRoot: m.RootPath}
}

// handleListMountsGin GET /api/mounts
func (s *Server) handleListMountsGin(c *gin.Context) {
	if s.mountRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mount registry not initialized"})
		return
	}
	mounts := s.mountRegistry.List()
	dtos := make([]mountDTO, 0, len(mounts))
	for _, m := range mounts {
		dtos = append(dtos, toDTO(m))
	}
	c.JSON(http.StatusOK, gin.H{
		"mounts":  dtos,
		"drivers": s.mountRegistry.ListDrivers(),
	})
}

// 🆕 2026-06-16: handleRefreshMountsGin POST /api/mounts/refresh
//   用途：用户主动触发 mount registry 重新 Bootstrap（补齐缺失的 primary/automation/sandbox）
//   场景：真机历史上持久化的 mounts.json 只含 primary（automation 缺失）
//   - 老 API 不能重跑 Bootstrap（GET /api/mounts 只读）
//   - 真机用户调"刷新挂载点"按钮 → POST /api/mounts/refresh
//   - 后端：list 当前 mount → 调 BootstrapFromConfig（idempotent） → Save → 返回新 list + added diff
//   - 同时 slog.Info 推到 DevLogs（用户能立即在 DevLogs 看到「automation mount 已补齐」）
func (s *Server) handleRefreshMountsGin(c *gin.Context) {
	if s.mountRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mount registry not initialized"})
		return
	}
	// 1. 记录调用前 mount names
	beforeList := make([]string, 0, len(s.mountRegistry.List()))
	for _, m := range s.mountRegistry.List() {
		beforeList = append(beforeList, m.Name)
	}
	// 2. 重新 Bootstrap（idempotent — 已存在的不会覆盖，用户的自定义 mount 保留）
	if err := s.mountRegistry.BootstrapFromConfig(c.Request.Context()); err != nil {
		slog.Error("mount refresh: bootstrap failed", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 3. Save（落盘 — 防止下次启动又缺）
	if err := s.mountRegistry.Save(); err != nil {
		slog.Warn("mount refresh: save failed", "err", err)
	}
	// 4. 计算 added diff
	afterList := make([]string, 0, len(s.mountRegistry.List()))
	for _, m := range s.mountRegistry.List() {
		afterList = append(afterList, m.Name)
	}
	added := mount.DiffStrings(beforeList, afterList)
	if len(added) > 0 {
		slog.Info("mount refresh: 补齐缺失的挂载点", "added", added, "total", afterList)
	} else {
		slog.Info("mount refresh: 完整，无需补齐", "total", afterList)
	}
	// 5. 返回新 list
	mounts := s.mountRegistry.List()
	dtos := make([]mountDTO, 0, len(mounts))
	for _, m := range mounts {
		dtos = append(dtos, toDTO(m))
	}
	c.JSON(http.StatusOK, gin.H{
		"mounts":  dtos,
		"drivers": s.mountRegistry.ListDrivers(),
		"added":   added,
		"before":  beforeList,
		"after":   afterList,
	})
}

// handleGetMountGin GET /api/mounts/:id
func (s *Server) handleGetMountGin(c *gin.Context) {
	id := c.Param("id")
	if s.mountRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mount registry not initialized"})
		return
	}
	m := s.mountRegistry.GetByID(id)
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mount not found"})
		return
	}
	c.JSON(http.StatusOK, toDTO(m))
}

// handleCreateMountGin POST /api/mounts
func (s *Server) handleCreateMountGin(c *gin.Context) {
	var body mount.Mount
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}
	body.ID = "" // 服务端生成
	if err := s.mountRegistry.Create(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = s.mountRegistry.Save()
	c.JSON(http.StatusCreated, toDTO(&body))
}

// handleUpdateMountGin PUT /api/mounts/:id
func (s *Server) handleUpdateMountGin(c *gin.Context) {
	id := c.Param("id")
	old := s.mountRegistry.GetByID(id)
	if old == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mount not found"})
		return
	}
	var body mount.Mount
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}
	body.ID = id // 强制使用 URL 中的 id
	body.CreatedAt = old.CreatedAt
	if err := s.mountRegistry.Update(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated := s.mountRegistry.GetByID(id)
	_ = s.mountRegistry.Save()
	c.JSON(http.StatusOK, toDTO(updated))
}

// handleDeleteMountGin DELETE /api/mounts/:id
func (s *Server) handleDeleteMountGin(c *gin.Context) {
	id := c.Param("id")
	if err := s.mountRegistry.Delete(id); err != nil {
		switch err {
		case mount.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case mount.ErrPrimaryProtected:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	_ = s.mountRegistry.Save()
	c.JSON(http.StatusNoContent, nil)
}

// resolveRequestBody POST /api/mounts/:id/resolve
type resolveRequestBody struct {
	SubPath string `json:"sub_path"`
}

// handleResolveMountPathGin POST /api/mounts/:id/resolve
//
// 把虚拟 sub_path 解析为物理 abs path（debug 接口）。
func (s *Server) handleResolveMountPathGin(c *gin.Context) {
	id := c.Param("id")
	m := s.mountRegistry.GetByID(id)
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mount not found"})
		return
	}
	var body resolveRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}
	virtualPath := m.MountPath
	if body.SubPath != "" {
		virtualPath = virtualPath + "/" + body.SubPath
	}
	res, err := s.mountRegistry.Resolve(virtualPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"virtual_path": virtualPath,
		"abs_path":     res.AbsPath,
		"rel_path":     res.RelPath,
		"mount_name":   res.Mount.Name,
	})
}

// handleMountUsageGin GET /api/mounts/:id/usage
//
// 简单实现：递归 du。dev 工具，不做高频调用。
func (s *Server) handleMountUsageGin(c *gin.Context) {
	id := c.Param("id")
	m := s.mountRegistry.GetByID(id)
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mount not found"})
		return
	}
	// 简化版：只返回 Mount.RootPath 存在与否 + 文件数（用 ReadDir(".") 替代）
	driver, err := s.mountRegistry_instantiateDriver(m.Driver)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = driver // 未来用 driver 的 CheckPermission
	entries, err := readDirShallow(m.RootPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"mount_id":    m.ID,
		"root_path":   m.RootPath,
		"entry_count": len(entries),
	})
}

// mountRegistry_instantiateDriver 通过 registry 内部 factory 创建 driver。
// 由于 MountRegistry.instantiate 是私有的，这里绕一层：直接 import drivers 包。
func (s *Server) mountRegistry_instantiateDriver(name string) (interface{}, error) {
	switch name {
	case mount.DriverLocal:
		return drivers.NewLocalDriver(), nil
	case mount.DriverAppData:
		return drivers.NewAppDataDriver(), nil
	case mount.DriverSandbox:
		return drivers.NewSandboxDriver(), nil
	default:
		return nil, mount.ErrInvalidDriver
	}
}

// readDirShallow 浅读根目录项数。
func readDirShallow(root string) ([]string, error) {
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}
