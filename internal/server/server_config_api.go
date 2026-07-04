package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/auth"
	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleGetConfigGin(c *gin.Context) {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if s.configPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "config path not available"})
		return
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		slog.Error("Failed to read config file", "path", s.configPath, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read config"})
		return
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Error("Failed to parse config file", "path", s.configPath, "error", err)
		c.Data(http.StatusOK, "application/json", data)
		return
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		slog.Error("Failed to marshal config", "error", err)
		c.Data(http.StatusOK, "application/json", data)
		return
	}

	c.Data(http.StatusOK, "application/json", out)
}

func (s *Server) handlePutConfigGin(c *gin.Context) {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if s.configPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "config path not available"})
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	if errMsg := validateWebdavRouteInConfig(raw); errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}

	if errMsg := validateContainerExtensionsInConfig(raw); errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}

	delete(raw, "mobile")

	if os.Getenv("ENCV_MOBILE") == "1" || os.Getenv("ENCV_DEV_PREVIEW") == "1" {
		sanitizeOverlayTargetFields(raw, s.configPath)
	}

	existingData, readErr := os.ReadFile(s.configPath)
	if readErr == nil {
		var existing map[string]interface{}
		if json.Unmarshal(existingData, &existing) == nil {
			for k, v := range raw {
				existing[k] = v
			}
			raw = existing
			slog.Info("Merged incoming config with existing file", "path", s.configPath)
		}
	} else {
		slog.Warn("No existing config to merge with (first write)", "path", s.configPath, "error", readErr)
	}

	indented, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to format config"})
		return
	}

	if err := os.WriteFile(s.configPath, append(indented, '\n'), 0644); err != nil {
		slog.Error("Failed to write config file", "path", s.configPath, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write config"})
		return
	}

	slog.Info("Config updated via API", "path", s.configPath)

	var newCfg config.Config
	needsRestart := false
	if err := json.Unmarshal(body, &newCfg); err != nil {
		slog.Warn("Config written but failed to parse for hot reload", "error", err)
		c.JSON(http.StatusOK, gin.H{"message": "config saved (hot reload skipped)"})
		return
	}

	s.cfg.Password = newCfg.Password
	s.cfg.Recover = newCfg.Recover
	s.cfg.OutputPath = newCfg.OutputPath
	s.cfg.PluginSettings = newCfg.PluginSettings

	if newCfg.Admin.Password != s.cfg.Admin.Password {
		s.cfg.Admin.Password = newCfg.Admin.Password
		if newCfg.Admin.Password != "" {
			s.jwtManager = auth.NewJWTManager(newCfg.Admin.Password, 7*24*time.Hour)
		} else {
			s.jwtManager = nil
		}
		slog.Info("Admin password hot-reloaded")
	}

	s.cfg.Webdav.Username = newCfg.Webdav.Username
	s.cfg.Webdav.Password = newCfg.Webdav.Password

	if newCfg.Log.Level != s.cfg.Log.Level {
		s.cfg.Log.Level = newCfg.Log.Level
		slog.Info("Log level hot-reloaded", "level", newCfg.Log.Level)
	}

	if newCfg.Server.Port != s.cfg.Server.Port {
		needsRestart = true
	}
	if newCfg.Webdav.Root != s.cfg.Webdav.Root || newCfg.Webdav.Dir != s.cfg.Webdav.Dir {
		needsRestart = true
	}
	if newCfg.Server.Dir != s.cfg.Server.Dir {
		needsRestart = true
	}

	s.cfg.Server = newCfg.Server
	s.cfg.Webdav.Root = newCfg.Webdav.Root
	s.cfg.Webdav.Dir = newCfg.Webdav.Dir
	s.cfg.Log = newCfg.Log

	msg := "config updated"
	if needsRestart {
		msg = "config saved, some changes require restart to take effect"
	}
	c.JSON(http.StatusOK, gin.H{"message": msg, "needsRestart": needsRestart})
}

func (s *Server) handleConfigSchemaGin(c *gin.Context) {
	schemaPaths := []string{}

	if s.configPath != "" {
		dir := filepath.Dir(s.configPath)
		base := filepath.Base(s.configPath)
		schemaName := strings.TrimSuffix(base, filepath.Ext(base)) + ".schema.json"
		schemaPaths = append(schemaPaths, filepath.Join(dir, schemaName))
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		schemaPaths = append(schemaPaths, filepath.Join(exeDir, "config.schema.json"))
	}

	for _, p := range schemaPaths {
		data, err := os.ReadFile(p)
		if err == nil {
			c.Data(http.StatusOK, "application/json", data)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "schema file not found"})
}

func validateWebdavRouteInConfig(raw map[string]interface{}) string {
	wd, ok := raw["webdav"].(map[string]interface{})
	if !ok {
		return ""
	}
	root, ok := wd["root"].(string)
	if !ok || root == "" {
		return ""
	}
	cleaned := strings.TrimSpace(root)
	if cleaned == "/" || cleaned == "//" {
		return "webdav root cannot be '/' (would capture all routes and crash server)"
	}
	if !strings.HasPrefix(cleaned, "/") {
		return "webdav root must start with '/'"
	}
	return ""
}

func sanitizeOverlayTargetFields(raw map[string]interface{}, configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var existing map[string]interface{}
	if json.Unmarshal(data, &existing) != nil {
		return
	}

	overlayTargets := []struct {
		section string
		field   string
	}{
		{"server", "dir"},
		{"output_path", ""},
	}

	for _, target := range overlayTargets {
		if target.section == "output_path" {
			if incoming, ok := raw["output_path"].(string); ok {
				if original, hasOrig := existing["output_path"].(string); hasOrig && incoming != original {
					raw["output_path"] = original
				}
			}
			continue
		}

		sec, ok := raw[target.section].(map[string]interface{})
		if !ok {
			continue
		}
		incomingVal, hasIncoming := sec[target.field].(string)
		if !hasIncoming || incomingVal == "" {
			continue
		}

		existingSec, hasExistingSec := existing[target.section].(map[string]interface{})
		if !hasExistingSec {
			continue
		}
		originalVal, hasOriginal := existingSec[target.field].(string)
		if !hasOriginal {
			continue
		}

		if incomingVal != originalVal {
			sec[target.field] = originalVal
		}
	}
}

func validateContainerExtensionsInConfig(raw map[string]interface{}) string {
	ps, ok := raw["plugin_settings"].(map[string]interface{})
	if !ok {
		return ""
	}

	configuredPlugins := make(map[string]bool, len(ps))
	for name := range ps {
		configuredPlugins[name] = true
	}

	extToPlugin := make(map[string]string)
	for _, p := range plugins.Plugins {
		if configuredPlugins[p.Name()] {
			continue
		}
		ext := p.GetContainerExtension()
		if ext != "" && ext != "." {
			extToPlugin[ext] = p.Name()
		}
	}

	for pluginName, settingsRaw := range ps {
		settings, ok := settingsRaw.(map[string]interface{})
		if !ok {
			continue
		}

		var suffix string
		if s, ok := settings["suffix"].(string); ok && s != "" {
			suffix = s
		} else if ext, ok := settings["ext"].(string); ok && ext != "" {
			suffix = ext
		} else {
			continue
		}

		if existing, exists := extToPlugin[suffix]; exists && existing != pluginName {
			return fmt.Sprintf("container extension '%s' conflicts between plugin '%s' and '%s'; container extensions must be unique", suffix, existing, pluginName)
		}
		if prev, dup := extToPlugin[suffix]; dup && prev != pluginName {
			return fmt.Sprintf("container extension '%s' used by multiple configured plugins; container extensions must be unique", suffix)
		}
		extToPlugin[suffix] = pluginName
	}

	return ""
}
