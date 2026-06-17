package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/injector"
	"github.com/Soltus/encv-go/internal/openlist"
	"github.com/Soltus/encv-go/internal/openlist/web"
	"github.com/Soltus/encv-go/internal/routes"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/handler"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/Soltus/encv-go/internal/v2/provider"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/gin-gonic/gin"
)

type ProxyGin struct {
	cfg            *config.Config
	factoryCache   map[string]reader.DecryptReaderFactory
	contentHandler *handler.ContentHandler
}

func NewProxyGin(cfg *config.Config) *ProxyGin {
	contentHandler := handler.NewContentHandler()
	return &ProxyGin{
		cfg:            cfg,
		factoryCache:   make(map[string]reader.DecryptReaderFactory),
		contentHandler: contentHandler,
	}
}

func (p *ProxyGin) HandleRequest(c *gin.Context) {
	siteHost, _ := c.Get("siteHost")
	siteToken, _ := c.Get("siteToken")
	siteId, _ := c.Get("siteId")

	siteHostStr, _ := siteHost.(string)
	siteTokenStr, _ := siteToken.(string)
	siteIdStr, _ := siteId.(string)

	openlist.MarkOpenListHeartbeat()

	originalPath := c.Request.URL.Path
	re := regexp.MustCompile(`^/openlist/sites/` + regexp.QuoteMeta(siteIdStr) + `/+`)
	path := re.ReplaceAllString(originalPath, "/")

	if path == "" {
		path = "/"
	}

	slog.Info("[Proxy] siteHost", "host", siteHostStr, "original", originalPath, "path", path)

	if strings.HasPrefix(path, "/_preview/") {
		h := http.StripPrefix("/openlist/sites/"+siteIdStr+"/_preview/", web.PreviewHandler())
		h.ServeHTTP(c.Writer, c.Request)
		return
	}

	sign := c.Request.URL.Query().Get("sign")
	isInternalRequest, _ := c.Get("internal_request")
	isInternal := fmt.Sprintf("%v", isInternalRequest) == "true"

	if path == "" {
		c.Status(http.StatusBadRequest)
		c.Writer.Write([]byte("Missing 'path' parameter"))
		return
	}

	if path == "/decrypt" {
		p.handleDecrypt(c, siteHostStr, siteTokenStr)
		return
	}

	isDirectory := strings.HasSuffix(path, "/")

	if !isInternal && !p.cfg.Proxy.DisableSignatureVerification && !isDirectory {
		if sign == "" {
			c.Status(http.StatusBadRequest)
			c.Writer.Write([]byte("Missing 'sign' parameter"))
			return
		}
		if !openlist.OpenListVerifySign(path, sign, siteTokenStr) {
			slog.Error("Invalid signature for path", "path", path)
			c.Status(http.StatusForbidden)
			c.Writer.Write([]byte("Forbidden: Invalid signature"))
			return
		}
	} else if isDirectory {
		slog.Info("[Proxy] Directory request, skipping signature verification", "path", path)
	} else if p.cfg.Proxy.DisableSignatureVerification {
		slog.Info("[Security] Signature verification is disabled", "path", path)
	} else {
		slog.Info("[Proxy] Handling internal request, skipping signature check", "path", path)
	}

	slog.Info("Received valid request for", "path", path)

	if after, ok := strings.CutPrefix(path, "/d/"); ok {
		path = "/" + after
		slog.Info("[Proxy] Stripped /d/ prefix", "path", path)
	}

	if isDirectory {
		p.handleDirectoryRequest(c, path, siteHostStr, siteTokenStr)
		return
	}

	if plugins.IsContainer(path) {
		slog.Info("[Proxy] Detected ENCV container file", "path", path)
		p.serveEncryptedContainer(c, path, siteHostStr, siteTokenStr)
		return
	}

	slog.Info("[Proxy] Not a container file, handling as standard file", "path", path)
	p.serveStandardFile(c, path, siteHostStr, siteTokenStr)
}

func (p *ProxyGin) handleDecrypt(c *gin.Context, siteHost, siteToken string) {
	routePathVal, _ := c.Get("routePath")
	routePath, _ := routePathVal.(string)

	if routePath == "" {
		// durl 是完整 URL，不经过 SafeURLPathToRelative，由 url.Parse 处理
		durl := c.Request.URL.Query().Get("file")
		if durl == "" {
			c.Status(http.StatusBadRequest)
			c.Writer.Write([]byte("Bad Request: 'file' query parameter is missing"))
			return
		}
		slog.Info("[Proxy] No clean path in context, parsing from 'file' query", "durl", durl)

		u, err := url.Parse(durl)
		if err != nil {
			c.Status(http.StatusBadRequest)
			c.Writer.Write([]byte("Bad Request: invalid durl format"))
			return
		}

		routePath = u.Path
	}

	if after, ok := strings.CutPrefix(routePath, "/d/"); ok {
		routePath = "/" + after
	}

	slog.Info("[Proxy] routePath", "path", routePath)

	fileInfo, err := openlist.OpenListGetFileURL(routePath, siteHost, siteToken)
	if err != nil {
		slog.Error("Error getting stream URL for routePath", "path", routePath, "error", err)
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte("Failed to locate file"))
		return
	}
	streamURL := fileInfo.Data.URL
	slog.Info("[Proxy] Successfully translated durl to stream URL", "url", streamURL)

	slog.Info("[Proxy] Probing upstream content type with HEAD request...")
	headResp, err := utils.GetRemoteStreamWithRange(streamURL, fileInfo.Data.Header, 0, 0)
	if err != nil {
		slog.Error("[Proxy] Failed to probe upstream", "url", streamURL, "error", err)
		c.Status(http.StatusBadGateway)
		c.Writer.Write([]byte("Upstream server is unreachable or invalid"))
		return
	}
	defer headResp.Body.Close()

	contentType := headResp.Header.Get("Content-Type")
	slog.Info("[Proxy] Upstream responded with Content-Type", "type", contentType)

	slog.Info("[Proxy] Validating stream URL before decryption...")
	resp, err := utils.GetRemoteStreamWithRange(streamURL, fileInfo.Data.Header, 0, 5)
	if err != nil {
		slog.Error("[Proxy] Failed to validate stream URL", "url", streamURL, "error", err)
		c.Status(http.StatusBadGateway)
		c.Writer.Write([]byte("Upstream server is unreachable or invalid"))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		slog.Warn("[Proxy] Upstream server does not support Range requests", "status", resp.Status)
		p.serveDirectStream(c, streamURL, fileInfo.Data.Header)
		return
	}

	headerBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("[Proxy] Failed to read validation data", "url", streamURL, "error", err)
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte("Failed to validate upstream file"))
		return
	}

	isValid, err := detector.IsEncvContainerFromBytes(headerBytes)
	if err != nil {
		slog.Error("[Proxy] Validation check failed", "url", streamURL, "error", err)
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte("Failed to validate upstream file"))
		return
	}

	if isValid {
		slog.Info("[Proxy] File is a valid ENCV container. Proceeding with decryption.")
		p.serveEncryptedContainerWithURL(c, streamURL, fileInfo.Data.Header, siteHost, siteToken, routePath)
	} else {
		p.serveDirectStream(c, streamURL, fileInfo.Data.Header)
	}
}

func (p *ProxyGin) handleDirectoryRequest(c *gin.Context, path, siteHost, siteToken string) {
	if siteHost == "" || siteToken == "" {
		slog.Error("Site host or token is missing for path", "path", path)
	}
	indexFiles := []string{"index.html", "README.md"}

	for _, indexFile := range indexFiles {
		indexPath := path + indexFile
		if plugins.IsContainer(indexPath) {
			slog.Info("[Proxy] Found index container", "path", indexPath)
			p.serveEncryptedContainer(c, indexPath, siteHost, siteToken)
			return
		}

		fileInfo, err := openlist.OpenListGetFileURL(indexPath, siteHost, siteToken)
		if err == nil && fileInfo.Data.URL != "" {
			slog.Info("[Proxy] Found index file", "path", indexPath)
			p.serveDirectStream(c, fileInfo.Data.URL, fileInfo.Data.Header)
			return
		}
	}

	slog.Info("[Proxy] No index file found for directory", "path", path)
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Directory listing for %s</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
            padding: 20px; 
            background-color: var(--hope-ui-background, #ffffff);
            color: var(--hope-ui-text, #1a1a1a);
            transition: background-color 0.3s, color 0.3s;
        }
        body.dark {
            background-color: var(--hope-ui-background, #1a1a1a);
            color: var(--hope-ui-text, #e0e0e0);
        }
        h1 { color: var(--hope-ui-primary, #0066cc); margin-bottom: 20px; }
        .file-list { list-style: none; padding: 0; }
        .file-item { 
            margin: 8px 0; 
            padding: 12px; 
            background: var(--hope-ui-surface, #f5f5f5); 
            border-radius: 6px;
            border: 1px solid var(--hope-ui-border, #e0e0e0);
        }
        .file-link { 
            color: var(--hope-ui-primary, #0066cc); 
            text-decoration: none; 
            font-weight: 500;
        }
        .file-link:hover { text-decoration: underline; }
        .toggle-theme {
            position: fixed;
            top: 20px;
            right: 20px;
            background: var(--hope-ui-surface, #f5f5f5);
            border: 1px solid var(--hope-ui-border, #e0e0e0);
            padding: 8px 12px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <button class="toggle-theme" onclick="document.body.classList.toggle('dark')">🌙/☀️</button>
    <h1>Directory listing for %s</h1>
    <p>No index file found. This directory may contain encrypted files that require direct access.</p>
    <ul class="file-list">
        <li class="file-item"><a href="../" class="file-link">.. (Parent Directory)</a></li>
    </ul>
    <script>
        if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
            document.body.classList.add('dark');
        }
    </script>
</body>
</html>`, path, path)

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Writer.Write([]byte(html))
}

func (p *ProxyGin) serveEncryptedContainer(c *gin.Context, path string, siteHost, siteToken string) {
	if siteHost == "" || siteToken == "" {
		slog.Error("Site host or token is missing for path", "path", path)
	}

	fileInfo, err := openlist.OpenListGetFileURL(path, siteHost, siteToken)
	if err != nil {
		slog.Error("Error getting file URL", "path", path, "error", err)
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte("Failed to locate file"))
		return
	}
	p.serveEncryptedContainerWithURL(c, fileInfo.Data.URL, fileInfo.Data.Header, siteHost, siteToken, path)
}

func (p *ProxyGin) serveEncryptedContainerWithURL(c *gin.Context, containerURL string, headers map[string][]string, siteHost, siteToken, routePath string) {
	urlResolver := openlist.NewOpenListURLResolver(siteHost, siteToken, routePath)

	factory, err := reader.NewRemoteDecryptReaderFactory(containerURL, p.cfg.Password, headers, urlResolver)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte(fmt.Sprintf("failed to create remote decrypt reader factory: %v", err)))
		return
	}

	decryptReader, err := factory.NewDecryptReader()
	if err != nil {
		factory.Close()
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte(fmt.Sprintf("failed to create remote decrypt reader: %v", err)))
		return
	}

	prov, err := provider.NewRemoteFileProvider(factory, decryptReader)
	if err != nil {
		decryptReader.Close()
		factory.Close()
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte(err.Error()))
		return
	}
	defer prov.Close()

	p.contentHandler.ServeFile(c.Writer, c.Request, prov)
}

func (p *ProxyGin) serveStandardFile(c *gin.Context, path string, siteHost, siteToken string) {
	if siteHost == "" || siteToken == "" {
		slog.Error("Site host or token is missing for path", "path", path)
	}

	var fileURL string
	var headers map[string][]string

	if strings.HasPrefix(path, "/p/") {
		slog.Info("[Proxy] Intercepted internal link", "path", path)
		fileURL = siteHost + path + "?" + c.Request.URL.RawQuery
	} else {
		fileInfo, err := openlist.OpenListGetFileURL(path, siteHost, siteToken)
		if err != nil {
			slog.Error("Error getting file URL", "path", path, "error", err)
			c.Status(http.StatusInternalServerError)
			c.Writer.Write([]byte("Failed to locate file"))
			return
		}
		fileURL = fileInfo.Data.URL
		headers = fileInfo.Data.Header
	}

	p.serveDirectStream(c, fileURL, headers)
}

func (p *ProxyGin) serveDirectStream(c *gin.Context, fileURL string, headers map[string][]string) {
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		slog.Error("Error creating request to download file", "error", err)
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte("Failed to download file"))
		return
	}

	for key, values := range headers {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}

	// 【P0 修复】默认 30s 超时
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Error downloading file", "url", fileURL, "error", err)
		c.Status(http.StatusInternalServerError)
		c.Writer.Write([]byte("Failed to download file"))
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		c.Header(key, values[0])
		for _, v := range values[1:] {
			c.Writer.Header().Add(key, v)
		}
	}

	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Allow-Credentials", "true")

	c.Status(resp.StatusCode)

	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		slog.Error("Error streaming file to client", "error", err)
	}
}

func handleOpenlistSitesGin(mss *openlist.MultiSiteServer) gin.HandlerFunc {
	return func(c *gin.Context) {
		html := generateTokenInputPageGin(mss)
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Writer.Write([]byte(html))
	}
}

func handleSetSiteTokenGin(mss *openlist.MultiSiteServer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SiteID string `json:"siteId"`
			Token  string `json:"token"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.Status(http.StatusBadRequest)
			c.Writer.Write([]byte("Invalid request"))
			return
		}

		cfg := mss.GetConfig()
		if _, exists := cfg.Proxy.Sites[req.SiteID]; !exists {
			c.Status(http.StatusNotFound)
			c.Writer.Write([]byte("Site not found"))
			return
		}

		mss.GetTokenManager().SetToken(req.SiteID, req.Token)
		c.Redirect(http.StatusFound, routes.OpenListProxy+"/sites")
	}
}

func handleDeleteTokenGin(mss *openlist.MultiSiteServer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SiteID string `json:"siteId"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.Status(http.StatusBadRequest)
			c.Writer.Write([]byte("Invalid request"))
			return
		}

		mss.GetTokenManager().RemoveToken(req.SiteID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Token deleted successfully",
		})
	}
}

func handleSetExpiryGin(mss *openlist.MultiSiteServer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SiteID string `json:"siteId"`
			Days   int    `json:"days"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.Status(http.StatusBadRequest)
			c.Writer.Write([]byte("Invalid request"))
			return
		}

		if req.Days < 1 || req.Days > 365 {
			c.Status(http.StatusBadRequest)
			c.Writer.Write([]byte("Days must be between 1 and 365"))
			return
		}

		if err := mss.GetTokenManager().SetTokenExpiry(req.SiteID, req.Days); err != nil {
			c.Status(http.StatusInternalServerError)
			c.Writer.Write([]byte(err.Error()))
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Token expiry updated successfully",
		})
	}
}

func handleOpenlistProxyGin(proxy *ProxyGin) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxy.HandleRequest(c)
	}
}

func handleOpenlistPreviewGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		siteId := c.Param("siteId")
		h := http.StripPrefix("/openlist/sites/"+siteId+"/_preview/", web.PreviewHandler())
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func generateTokenInputPageGin(mss *openlist.MultiSiteServer) string {
	cfg := mss.GetConfig()
	tokenManager := mss.GetTokenManager()

	var siteCards string
	for siteId, siteConfig := range cfg.Proxy.Sites {
		_, hasToken := tokenManager.GetToken(siteId)
		status := "❌ No Token"
		expiryInfo := ""

		if hasToken {
			status = "✅ Configured"
			if siteToken := tokenManager.GetSiteToken(siteId); siteToken != nil {
				expiryInfo = fmt.Sprintf("Expires: %s", siteToken.ExpiresAt.Format("2006-01-02 15:04:05"))
			}
		}

		accessLinkStyle := "style='display:none;'"
		accessLink := ""
		if hasToken {
			accessLinkStyle = ""
			accessLink = fmt.Sprintf(`<a href="%s" target="_blank" class="access-link" %s>Visit Site</a>`,
				siteConfig.Host, accessLinkStyle)
		}

		expiryButtonDisabled := ""
		deleteButtonDisabled := ""
		if !hasToken {
			expiryButtonDisabled = "disabled"
			deleteButtonDisabled = "disabled"
		}

		siteCards += fmt.Sprintf(`
    <div class="site-card">
        <h3>%s</h3>
        <p>%s</p>
        <p class="status">Status: %s</p>
        <p class="expiry">%s</p>
        <form method="post" action="%s/set-token" class="token-form">
            <input type="hidden" name="siteId" value="%s">
            <input type="password" name="token" placeholder="Enter token for this site" required>
            <button type="submit">Save Token</button>
        </form>
        <div class="action-buttons">
            %s
            <button onclick="showExpiryDialog('%s')" class="expiry-btn" %s>Set Expiry</button>
            <button onclick="deleteToken('%s')" class="delete-btn" %s>Delete Token</button>
        </div>
    </div>
`, siteId, siteConfig.Description, status, expiryInfo,
			routes.OpenListProxy, siteId,
			accessLink,
			siteId, expiryButtonDisabled,
			siteId, deleteButtonDisabled)
	}

	return generateSiteManagementHTML(siteCards, cfg.Proxy.Sites, tokenManager)
}

func generateSiteManagementHTML(siteCards string, sites map[string]types.ProxySiteConfig, tokenManager *openlist.TokenManager) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>ENCV - OpenList Sites</title>
    <style>
        :root {
            --bg-color: #f4f4f9;
            --text-color: #333;
            --muted-text-color: #666;
            --card-bg-color: #f9f9f9;
            --border-color: #ddd;
            --header-text-color: #333;
            --status-text-color: #333;
            --expiry-text-color: #888;
            --input-bg-color: #fff;
            --input-border-color: #ddd;
            --input-text-color: #333;
            --btn-primary-bg: #007bff;
            --btn-primary-text: #fff;
            --btn-success-bg: #28a745;
            --btn-success-text: #fff;
            --btn-success-hover-bg: #218838;
            --btn-warning-bg: #ffc107;
            --btn-warning-text: #212529;
            --btn-danger-bg: #dc3545;
            --btn-danger-text: #fff;
            --btn-disabled-opacity: 0.5;
            --modal-bg-color: #fefefe;
            --modal-overlay-bg: rgba(0,0,0,0.4);
            --link-color: #007bff;
            --link-hover-color: #0056b3;
            --selection-bg: rgba(46, 170, 220, 0.3);
        }

        body.hope-ui-dark {
            --bg-color: #1a1a1a;
            --text-color: #e6edf3;
            --muted-text-color: #8b949e;
            --card-bg-color: #161b22;
            --border-color: #30363d;
            --header-text-color: #e6edf3;
            --status-text-color: #e6edf3;
            --expiry-text-color: #8b949e;
            --input-bg-color: #0d1117;
            --input-border-color: #30363d;
            --input-text-color: #e6edf3;
            --btn-primary-bg: #238636;
            --btn-primary-text: #fff;
            --btn-success-bg: #238636;
            --btn-success-text: #fff;
            --btn-success-hover-bg: #2ea043;
            --btn-warning-bg: #9e6a03;
            --btn-warning-text: #fff;
            --btn-danger-bg: #da3633;
            --btn-danger-text: #fff;
            --btn-disabled-opacity: 0.3;
            --modal-bg-color: #161b22;
            --modal-overlay-bg: rgba(0,0,0,0.8);
            --link-color: #58a6ff;
            --link-hover-color: #79c0ff;
            --selection-bg: rgba(46, 170, 220, 0.4);
        }

        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
            max-width: 1200px; 
            margin: 20px auto; 
            padding: 20px; 
            background-color: var(--bg-color);
            color: var(--text-color);
            transition: background-color 0.3s ease, color 0.3s ease;
        }
        
        .header { 
            text-align: center; 
            margin-bottom: 30px; 
        }
        .header h1 {
            color: var(--muted-text-color);
            margin-bottom: 10px;
        }
        .header p {
            color: var(--muted-text-color);
        }
        
        .sites-grid { 
            display: grid; 
            grid-template-columns: repeat(auto-fit, minmax(350px, 1fr)); 
            gap: 20px; 
        }
        
        .site-card { 
            border: 1px solid var(--border-color); 
            padding: 20px; 
            border-radius: 8px; 
            background-color: var(--card-bg-color);
            transition: transform 0.2s ease, box-shadow 0.2s ease;
        }
        .site-card:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(0,0,0,0.1);
        }
        
        .site-card h3 { 
            margin: 0 0 10px 0; 
            color: var(--text-color);
        }
        .site-card p { 
            margin: 5px 0; 
            color: var(--muted-text-color);
        }
        .status { 
            font-weight: bold; 
            margin: 10px 0; 
            color: var(--status-text-color);
        }
        .expiry { 
            font-size: 0.9em; 
            color: var(--expiry-text-color); 
            margin: 5px 0; 
        }
        
        .token-form { 
            display: flex; 
            gap: 10px; 
            margin: 15px 0; 
        }
        .token-form input { 
            flex: 1; 
            padding: 8px; 
            border: 1px solid var(--input-border-color); 
            border-radius: 4px; 
            background-color: var(--input-bg-color);
            color: var(--input-text-color);
        }
        .token-form input:focus {
            outline: none;
            border-color: var(--btn-primary-bg);
        }
        .token-form button { 
            padding: 8px 16px; 
            background: var(--btn-primary-bg); 
            color: var(--btn-primary-text); 
            border: none; 
            border-radius: 4px; 
            cursor: pointer;
            transition: background-color 0.2s ease;
        }
        .token-form button:hover {
            opacity: 0.9;
        }
        
        .action-buttons { 
            display: flex; 
            gap: 10px; 
            margin-top: 10px; 
            flex-wrap: wrap; 
        }
        
        .access-link { 
            display: inline-block; 
            padding: 8px 16px; 
            background: var(--btn-success-bg); 
            color: var(--btn-success-text); 
            text-decoration: none; 
            border-radius: 4px;
            transition: background-color 0.2s ease;
        }
        .access-link:hover { 
            background: var(--btn-success-hover-bg); 
        }
        
        .expiry-btn, .delete-btn { 
            padding: 6px 12px; 
            border: none; 
            border-radius: 4px; 
            cursor: pointer; 
            font-size: 0.9em;
            transition: all 0.2s ease;
        }
        .expiry-btn { 
            background: var(--btn-warning-bg); 
            color: var(--btn-warning-text); 
        }
        .delete-btn { 
            background: var(--btn-danger-bg); 
            color: var(--btn-danger-text); 
        }
        .expiry-btn:hover, .delete-btn:hover {
            opacity: 0.9;
            transform: translateY(-1px);
        }
        .expiry-btn:disabled, .delete-btn:disabled { 
            opacity: var(--btn-disabled-opacity); 
            cursor: not-allowed; 
            transform: none;
        }
        
        .modal { 
            display: none; 
            position: fixed; 
            z-index: 1000; 
            left: 0; 
            top: 0; 
            width: 100%%; 
            height: 100%%; 
            background-color: var(--modal-overlay-bg);
        }
        .modal-content { 
            background-color: var(--modal-bg-color); 
            margin: 15%% auto; 
            padding: 20px; 
            border: 1px solid var(--border-color);
            border-radius: 8px; 
            width: 300px;
            box-shadow: 0 4px 12px rgba(0,0,0,0.2);
        }
        .modal-content h3 { 
            margin-top: 0;
            color: var(--text-color);
        }
        .modal-content label {
            display: block;
            margin-bottom: 5px;
            color: var(--muted-text-color);
        }
        .modal-content input { 
            width: 100%%; 
            padding: 8px; 
            margin: 10px 0; 
            border: 1px solid var(--input-border-color); 
            border-radius: 4px;
            background-color: var(--input-bg-color);
            color: var(--input-text-color);
        }
        .modal-content input:focus {
            outline: none;
            border-color: var(--btn-primary-bg);
        }
        .modal-content button { 
            padding: 8px 16px; 
            margin-right: 10px; 
            border: none; 
            border-radius: 4px; 
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .confirm-btn { 
            background: var(--btn-primary-bg); 
            color: var(--btn-primary-text);
        }
        .confirm-btn:hover {
            opacity: 0.9;
        }
        .cancel-btn { 
            background: var(--muted-text-color); 
            color: var(--btn-primary-text);
        }
        .cancel-btn:hover {
            opacity: 0.8;
        }
    </style>
    <script>
        function deleteToken(siteId) {
            if (confirm('Are you sure you want to delete the token for ' + siteId + '?')) {
                fetch('%s/delete-token', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({siteId: siteId})
                }).then(() => location.reload());
            }
        }
        
        function showExpiryDialog(siteId) {
            document.getElementById('expirySiteId').value = siteId;
            document.getElementById('expiryModal').style.display = 'block';
        }
        
        function hideExpiryDialog() {
            document.getElementById('expiryModal').style.display = 'none';
        }
        
        function setExpiry() {
    const siteId = document.getElementById('expirySiteId').value;
    const daysInput = document.getElementById('expiryDays');
    const days = parseInt(daysInput.value, 10);

    if (isNaN(days) || days < 1 || days > 365) {
        alert('Error: Days must be a number between 1 and 365.');
        return;
    }

    const confirmBtn = document.querySelector('.modal-content .confirm-btn');
    const originalText = confirmBtn.innerText;
    confirmBtn.innerText = 'Setting...';
    confirmBtn.disabled = true;

    fetch('%s/set-expiry', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({siteId: siteId, days: days})
    })
    .then(response => {
        if (!response.ok) {
            return response.json().then(errData => {
                throw new Error(errData.message || "Server error: "+response.statusText);
            });
        }
        return response.json();
    })
    .then(data => {
        console.log('Success:', data.message);
        location.reload();
    })
    .catch(error => {
        console.error('Set expiry failed:', error);
        alert('Failed to set expiry: ' + error.message);
        confirmBtn.innerText = originalText;
        confirmBtn.disabled = false;
    });
}

        window.onclick = function(event) {
            const modal = document.getElementById('expiryModal');
            if (event.target == modal) {
                modal.style.display = 'none';
            }
        }
    </script>
</head>
<body>
    <div id="`+injector.InjectorID+`"></div>
    
    <div class="header">
        <h1>OpenList Site Management</h1>
        <p>Configure tokens for your OpenList sites</p>
    </div>
    <div class="sites-grid">
        %s
    </div>
    
    <div id="expiryModal" class="modal">
        <div class="modal-content">
            <h3>Set Token Expiry</h3>
            <input type="hidden" id="expirySiteId">
            <label for="expiryDays">Days until expiry:</label>
            <input type="number" id="expiryDays" min="1" max="365" value="30">
            <button class="confirm-btn" onclick="setExpiry()">Set</button>
            <button class="cancel-btn" onclick="hideExpiryDialog()">Cancel</button>
        </div>
    </div>
</body>
</html>
    `, routes.OpenListProxy, routes.OpenListProxy, siteCards)
}

// LocalOpenListStatusHandler 处理 GET /openlist/local/status，
// 返回本地 OpenList 插件的运行状态、端口、心跳等。
func LocalOpenListStatusHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, openlist.GetLocalOpenListStatus())
	}
}
