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
//
// License/Icon：硬编码到 goLibMeta 表（LLM 知识库）。缺失时 = license='unknown' / icon='cube'。
//                前端 useLibraries composable 走 npm/GitHub fallback 解析。
type LibraryItem struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	VersionRaw  string `json:"version_range,omitempty"`
	Source      string `json:"source"`
	Kind        string `json:"kind"`
	Importance  string `json:"importance"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	License     string `json:"license"`
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

// goLibMeta 库元信息表（LLM 知识库）
// 缺字段时 = icon='cube' / license='unknown' / description=空字符串
type goLibMetaT struct {
	Description string
	Icon        string
	License     string
}

var goLibMeta = map[string]goLibMetaT{
	// direct deps
	"github.com/SaveTheRbtz/zstd-seekable-format-go/pkg": {Description: "zstd seekable 格式 — 加密容器 v4 压缩后端", Icon: "archive", License: "MIT"},
	"github.com/abema/go-mp4":                            {Description: "go-mp4 — MP4 box 解析（视频元数据）", Icon: "film", License: "Apache-2.0"},
	"github.com/dsoprea/go-exif/v3":                      {Description: "go-exif — 图像 EXIF 解析", Icon: "image", License: "MIT"},
	"github.com/dustin/go-humanize":                       {Description: "go-humanize — 数字/字节可读化输出", Icon: "text", License: "MIT"},
	"github.com/fsnotify/fsnotify":                        {Description: "fsnotify — 文件系统监控（配置热重载）", Icon: "eye", License: "BSD-3-Clause"},
	"github.com/fxamacker/cbor/v2":                        {Description: "CBOR v2 — Concise Binary Object Representation 编码", Icon: "code", License: "MIT"},
	"github.com/gin-contrib/cors":                         {Description: "Gin CORS 中间件（跨域请求）", Icon: "globe", License: "MIT"},
	"github.com/gin-gonic/gin":                           {Description: "Gin — Go HTTP Web 框架", Icon: "server", License: "MIT"},
	"github.com/google/uuid":                              {Description: "google/uuid — UUID 生成", Icon: "key", License: "BSD-3-Clause"},
	"github.com/gorilla/websocket":                        {Description: "gorilla/websocket — WebSocket 实现", Icon: "sync", License: "BSD-2-Clause"},
	"github.com/klauspost/compress":                       {Description: "klauspost/compress — 压缩算法集合（gzip/zstd/snappy）", Icon: "archive", License: "Apache-2.0"},
	"github.com/pterm/pterm":                              {Description: "pterm — 终端 UI 渲染（进度条/表格/颜色）", Icon: "terminal", License: "MIT"},
	"github.com/spf13/cobra":                              {Description: "Cobra — Go CLI 框架", Icon: "terminal", License: "Apache-2.0"},
	"github.com/stretchr/testify":                         {Description: "stretchr/testify — Go 测试断言库", Icon: "flask", License: "MIT"},
	"github.com/invopop/jsonschema":                       {Description: "invopop/jsonschema — Go struct → JSON Schema", Icon: "document-text", License: "MIT"},
	"github.com/wk8/go-ordered-map/v2":                    {Description: "wk8/go-ordered-map — 保留插入顺序的 map", Icon: "list", License: "MIT"},
	"golang.org/x/crypto":                                {Description: "Go 扩展加密包（chacha20/sha256/scrypt 等）", Icon: "lock-closed", License: "BSD-3-Clause"},
	"golang.org/x/net":                                   {Description: "Go 扩展网络包", Icon: "globe", License: "BSD-3-Clause"},
	"golang.org/x/sys":                                   {Description: "Go 扩展系统调用包", Icon: "cog", License: "BSD-3-Clause"},
	"gopkg.in/yaml.v3":                                   {Description: "yaml.v3 — YAML 解析/编码", Icon: "document-text", License: "MIT"},
	// 常用 indirect deps
	"go.mongodb.org/mongo-driver/v2": {Description: "MongoDB Go Driver v2", Icon: "server", License: "Apache-2.0"},
	"google.golang.org/protobuf":      {Description: "Protobuf 运行时（gRPC 兼容）", Icon: "cube", License: "BSD-3-Clause"},
	"github.com/bytedance/sonic":      {Description: "bytedance/sonic — 高性能 JSON 编解码", Icon: "flash", License: "Apache-2.0"},
	"github.com/go-playground/validator/v10": {Description: "Go struct 验证器", Icon: "checkmark-circle", License: "MIT"},
	"github.com/json-iterator/go":     {Description: "json-iterator — 高性能 JSON 替代品", Icon: "code", License: "MIT"},
	"github.com/quic-go/quic-go":      {Description: "quic-go — QUIC 协议实现（HTTP/3）", Icon: "globe", License: "MIT"},
}

var goLibDefault = goLibMetaT{
	Description: "",
	Icon:        "cube",
	License:     "unknown",
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
		Icon:        "logo-google",
		License:     "BSD-3-Clause",
	})

	// 2. Go 第三方 deps
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range bi.Deps {
			ver := dep.Version
			if ver == "" {
				ver = "(unknown)"
			}
			meta, ok := goLibMeta[dep.Path]
			if !ok {
				meta = goLibDefault
			}
			items = append(items, LibraryItem{
				Name:        dep.Path,
				Version:     ver,
				Source:      "go.mod",
				Kind:        "dependency",
				Importance:  classifyGoDep(dep.Path),
				Description: meta.Description,
				Icon:        meta.Icon,
				License:     meta.License,
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
