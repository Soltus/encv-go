package server

// mobile_openlist.go — Openlist 远程站点的 CRUD。

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/gin-gonic/gin"
)

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
