package openlist

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// FileInfoResponse 是 /api/fs/link 的响应结构
type FileInfoResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		URL           string              `json:"url"`
		Header        map[string][]string `json:"header"`
		Expiration    interface{}         `json:"Expiration"` // 可能是 null 或 string
		Concurrency   int                 `json:"concurrency"`
		PartSize      int                 `json:"part_size"`
		ContentLength int64               `json:"content_length"`
	} `json:"data"`
}

// 获取 OpenList 文件的真实下载链接和请求头
// 考虑到加密容器可能是分片的，因此需要 token
func OpenListGetFileURL(routePath, host, token string) (*FileInfoResponse, error) {
	apiURL := fmt.Sprintf("%s/api/fs/link", host)

	reqBody, err := json.Marshal(map[string]string{"path": routePath})
	if err != nil {
		return nil, fmt.Errorf("failed to create request body: %w", err)
	}

	resp, err := utils.MakeAuthenticatedRequest("POST", apiURL, string(reqBody), token)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenList API: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	bodyString := string(bodyBytes)
	log.Printf("-> [OpenList Debug] Raw response from /api/fs/link for routePath '%s': %s", routePath, bodyString)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenList API returned non-200 status: %d, body: %s", resp.StatusCode, bodyString)
	}

	var fileInfo FileInfoResponse
	if err := json.Unmarshal(bodyBytes, &fileInfo); err != nil {
		return nil, fmt.Errorf("failed to parse OpenList API response: %w, body: %s", err, bodyString)
	}

	// --- 关键修正：检查嵌套在 data 中的 URL ---
	if fileInfo.Data.URL == "" {
		return nil, fmt.Errorf("OpenList API returned an empty URL in the response, body: %s", bodyString)
	}

	return &fileInfo, nil
}

// 验证 OpenList 的签名
func OpenListVerifySign(path, sign, token string) bool {
	parts := strings.SplitN(sign, ":", 2)
	if len(parts) != 2 {
		return false
	}

	signature, expireTimestampStr := parts[0], parts[1]
	expireTS, err := strconv.ParseInt(expireTimestampStr, 10, 64)
	if err != nil {
		return false
	}

	if expireTS != 0 && time.Now().Unix() > expireTS {
		return false
	}

	var pathsToTest []string
	pathsToTest = append(pathsToTest, path)
	if strings.HasPrefix(path, "/") {
		pathsToTest = append(pathsToTest, path[1:])
	}

	for _, p := range pathsToTest {
		toSign := fmt.Sprintf("%s:%d", p, expireTS)
		h := hmac.New(sha256.New, []byte(token))
		h.Write([]byte(toSign))

		signatureWithPadding := base64.URLEncoding.EncodeToString(h.Sum(nil))
		signatureWithoutPadding := strings.TrimRight(signatureWithPadding, "=")

		if hmac.Equal([]byte(signature), []byte(signatureWithPadding)) {
			log.Printf("-> [Signature Debug] Signature matched for path: '%s' (with padding)", p)
			return true
		}
		if hmac.Equal([]byte(signature), []byte(signatureWithoutPadding)) {
			log.Printf("-> [Signature Debug] Signature matched for path: '%s' (without padding)", p)
			return true
		}
	}

	log.Printf("-> [Signature Debug] Signature did not match for any path variant. Original path: '%s'", path)
	return false
}

// OpenListURLResolver 现在通过复用签名来工作
type OpenListURLResolver struct {
	host     string // OpenList 主机地址
	token    string // 站点 token
	basePath string // 主容器文件所在的逻辑目录，例如 "/encv/go/output"
}

// NewOpenListURLResolver 创建一个新的解析器实例，它将复用主容器文件的签名。
func NewOpenListURLResolver(host, token, originalContainerPath string) *OpenListURLResolver {
	// 从原始路径中提取目录部分
	// originalContainerPath 示例: "/encv/go/output/321.4pm.sccgv"
	basePath := filepath.Dir(originalContainerPath) // 结果: "/encv/go/output"

	return &OpenListURLResolver{
		host:     host,
		token:    token,
		basePath: basePath,
	}
}

// ResolveURL 为给定的物理分片路径获取一个带签名的 URL。
func (r *OpenListURLResolver) ResolveURL(physicalPath string) (string, error) {
	// 1. 构建物理分片的完整逻辑路径
	// physicalPath 示例: "321.4pm.0001"
	chunkLogicalPath := filepath.Join(r.basePath, physicalPath) // 在 Windows 上可能产生 "\encv\go\output\321.4pm.0001"

	// 【关键修复】强制将所有反斜杠替换为正斜杠，以兼容 Web 服务器
	chunkLogicalPath = strings.ReplaceAll(chunkLogicalPath, "\\", "/")

	log.Printf("DEBUG: [OpenListURLResolver] Resolved path for '%s' to '%s'", physicalPath, chunkLogicalPath)

	// 2. 调用 OpenListGetFileURL 函数为这个分片获取一个新的签名 URL
	fileInfo, err := OpenListGetFileURL(chunkLogicalPath, r.host, r.token)
	if err != nil {
		return "", fmt.Errorf("failed to get signed URL for chunkLogicalPath '%s': %w", chunkLogicalPath, err)
	}

	// 3. 返回获取到的 URL
	return fileInfo.Data.URL, nil
}

// --- 本地 OpenList 插件自动发现 ---

// LocalLoopbackSiteID 是自动注册到 cfg.Proxy.Sites 的内置站点 ID
const LocalLoopbackSiteID = "local-loopback"

// LocalOpenListDefaultURL 是本地 OpenList 插件默认监听的地址
const LocalOpenListDefaultURL = "http://127.0.0.1:5244"

// LocalOpenListDefaultPort 是本地 OpenList 插件默认监听的端口
const LocalOpenListDefaultPort = 5244

// LocalOpenListProbeTimeout 探测本地 OpenList 的超时时间
const LocalOpenListProbeTimeout = 2 * time.Second

// lastOpenListHeartbeat 记录 encv-go 最近一次通过本地反代通道与 OpenList 通信的时间（unix 毫秒）
var lastOpenListHeartbeat atomic.Int64

func init() {
	lastOpenListHeartbeat.Store(time.Now().UnixMilli())
}

// MarkOpenListHeartbeat 在每次 encv-go 反代 OpenList 请求时被调用，刷新心跳
func MarkOpenListHeartbeat() {
	lastOpenListHeartbeat.Store(time.Now().UnixMilli())
}

// GetLastOpenListHeartbeat 返回最近一次心跳的 unix 毫秒
func GetLastOpenListHeartbeat() int64 {
	return lastOpenListHeartbeat.Load()
}

// tryRegisterLocalLoopback 探测 127.0.0.1:5244 上的本地 OpenList 插件。
// 如果插件可用，将其作为内置站点注册到 sites 中。
// 重复调用是安全的：已存在则跳过；已存在但 Host 变化时更新。
func tryRegisterLocalLoopback(sites map[string]types.ProxySiteConfig) error {
	if sites == nil {
		return fmt.Errorf("sites map is nil")
	}

	client := &http.Client{Timeout: LocalOpenListProbeTimeout}
	probeURL := LocalOpenListDefaultURL + "/api/site/list"
	resp, err := client.Get(probeURL)
	if err != nil {
		return fmt.Errorf("local openlist probe failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("local openlist probe returned status %d", resp.StatusCode)
	}

	builtin := types.ProxySiteConfig{
		Host:        LocalOpenListDefaultURL,
		Description: "本地 OpenList（Plugin）",
		BuiltIn:     true,
	}

	if existing, ok := sites[LocalLoopbackSiteID]; ok {
		if existing.BuiltIn {
			existing.Host = builtin.Host
			existing.Description = builtin.Description
			sites[LocalLoopbackSiteID] = existing
		}
		return nil
	}

	sites[LocalLoopbackSiteID] = builtin
	return nil
}

// TryRegisterLocalLoopback 导出版本，server.Start() 在启动时调用
func TryRegisterLocalLoopback(sites map[string]types.ProxySiteConfig) {
	_ = tryRegisterLocalLoopback(sites)
}

// IsLocalLoopbackSiteID 判断给定的 siteId 是否为内置的本地 OpenList 站点
func IsLocalLoopbackSiteID(siteId string) bool {
	return siteId == LocalLoopbackSiteID
}

// LocalOpenListStatus 是 /openlist/local/status 端点的响应结构
type LocalOpenListStatus struct {
	Running       bool  `json:"running"`
	PID           int   `json:"pid"`
	Port          int   `json:"port"`
	DataDirSize   int64 `json:"dataDirSize"`
	LastHeartbeat int64 `json:"lastHeartbeat"`
}

// ProbeLocalOpenList 探测本地 OpenList 是否在运行
func ProbeLocalOpenList() bool {
	client := &http.Client{Timeout: LocalOpenListProbeTimeout}
	resp, err := client.Get(LocalOpenListDefaultURL + "/api/site/list")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetLocalOpenListStatus 返回供前端查询的本地 OpenList 状态
func GetLocalOpenListStatus() LocalOpenListStatus {
	return LocalOpenListStatus{
		Running:       ProbeLocalOpenList(),
		PID:           0,
		Port:          LocalOpenListDefaultPort,
		DataDirSize:   0,
		LastHeartbeat: GetLastOpenListHeartbeat(),
	}
}
