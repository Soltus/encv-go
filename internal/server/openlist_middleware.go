package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/Soltus/encv-go/internal/openlist"
	"github.com/gin-gonic/gin"
)

func OpenlistSiteMiddleware(mss *openlist.MultiSiteServer) gin.HandlerFunc {
	return func(c *gin.Context) {
		siteId := c.Param("siteId")

		cfg := mss.GetConfig()
		siteConfig, exists := cfg.Proxy.Sites[siteId]
		if !exists {
			c.Status(http.StatusNotFound)
			c.Writer.Write([]byte("siteConfig not found"))
			c.Abort()
			return
		}

		token, hasToken := mss.GetTokenManager().GetToken(siteId)
		if !hasToken {
			c.Status(http.StatusUnauthorized)
			c.Writer.Write([]byte("Token required. Please set token first."))
			c.Abort()
			return
		}

		c.Set("siteHost", siteConfig.Host)
		c.Set("siteId", siteId)
		c.Set("siteToken", token)

		pathPrefix := "/openlist/sites/" + siteId
		c.Set("pathPrefix", pathPrefix)

		if strings.HasSuffix(c.Request.URL.Path, "/decrypt") {
			fileURL := c.Request.URL.Query().Get("file")
			if fileURL != "" {
				if parsedURL, err := url.Parse(fileURL); err == nil {
					if strings.HasPrefix(parsedURL.Path, pathPrefix) {
						cleanPath := strings.TrimPrefix(parsedURL.Path, pathPrefix)
						if cleanPath == "" {
							cleanPath = "/"
						}
						if after, ok := strings.CutPrefix(cleanPath, "/d/"); ok {
							cleanPath = "/" + after
						}
						c.Set("routePath", cleanPath)
						slog.Info("[MultiSite Middleware] Extracted routePath", "path", cleanPath)
					}
				}
			}
		}

		c.Next()
	}
}
