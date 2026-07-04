package server

// mobile_metadata.go — 标签 + 插件元数据 + 容器扩展名 + 任务选项。

import (
	"fmt"
	"net/http"

	"github.com/Soltus/encv-go/internal/v2/plugins"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/gin-gonic/gin"
)

// PluginMeta 插件元数据（从 plugins.Plugins 提取，前端 /api/plugins 响应）。
type PluginMeta struct {
	Name                  string   `json:"name"`
	SupportedExtensions   []string `json:"supportedExtensions"`
	SupportedMimePrefixes []string `json:"supportedMimePrefixes"`
	ContainerExtension    string   `json:"containerExtension"`
	TaskOptions           gin.H    `json:"taskOptions"`
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
