package server

// mobile_webdav.go — WebDAV 探针 / 权限 / 索引 / 本地信息 / manifest / 外部流。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Soltus/encv-go/internal/mount"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/gin-gonic/gin"
)

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
		Name       string         `json:"name"`
		MountPath  string         `json:"mount_path"`
		RootPath   string         `json:"root_path"`
		WebdavPath string         `json:"webdav_path"`
		IsDefault  bool           `json:"is_default"`
		Manifest   map[string]any `json:"manifest"`
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

func (s *Server) handleStreamExternalFileGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))

	slog.Info("API: stream external file", "path", queryPath)

	err := s.mobileSvc.StreamExternalFile(c.Writer, c.Request, queryPath)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}
}
