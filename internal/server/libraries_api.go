package server

import (
	"encoding/json"
	"net/http"
	"runtime"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// 🆕 2026-06-17：/api/libraries 端点
//
// 数据源：
//   1. Go stdlib 版本（runtime.Version()）
//   2. Go 第三方 deps（runtime/debug.ReadBuildInfo()）
//   3. Android deps（前端从 native bridge 读 android-deps.json 后，通过 ?android_manifest={json} 传过来）
//
// 状态：所有 deps 标记 status=active（能解析到 path+version 即视为正常）。
//       broken/historical 由前端 useLibraries composable 计算（前端能 cross-check manifest vs source）。
type LibraryItem struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	VersionRaw  string `json:"version_range,omitempty"`
	Source      string `json:"source"`
	Kind        string `json:"kind"`
	Importance  string `json:"importance"`
	Description string `json:"description"`
}

type LibrariesResponse struct {
	Items []LibraryItem `json:"items"`
}

// Go deps importance 分类（与前端 manifest 的分类保持一致）
var goCoreLibs = map[string]bool{
	"github.com/gin-gonic/gin":     true, // HTTP 框架 - 核心
	"github.com/spf13/cobra":       true, // CLI - 核心
	"github.com/gorilla/websocket": true, // WS - 核心
	"github.com/fsnotify/fsnotify": true, // FS watch - 核心
}

func classifyGoDep(path string) string {
	if goCoreLibs[path] {
		return "core"
	}
	return "light"
}

func (s *Server) handleLibrariesGin(c *gin.Context) {
	items := []LibraryItem{}

	// 1. Go 自身
	items = append(items, LibraryItem{
		Name:        "Go",
		Version:     runtime.Version(),
		Source:      "runtime.Version()",
		Kind:        "runtime",
		Importance:  "core",
		Description: "后端运行时",
	})

	// 2. Go 第三方 deps
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range bi.Deps {
			ver := dep.Version
			if ver == "" {
				ver = "(unknown)"
			}
			items = append(items, LibraryItem{
				Name:        dep.Path,
				Version:     ver,
				Source:      "go.mod",
				Kind:        "dependency",
				Importance:  classifyGoDep(dep.Path),
				Description: "",
			})
		}
	}

	// 3. Android deps via query param（前端从 native bridge 推送过来）
	if raw := c.Query("android_manifest"); raw != "" {
		var androidItems []LibraryItem
		if err := json.Unmarshal([]byte(raw), &androidItems); err == nil {
			items = append(items, androidItems...)
		}
		// 解析失败时静默忽略，不污染响应
	}

	c.JSON(http.StatusOK, LibrariesResponse{Items: items})
}
