package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/middleware"
	"github.com/Soltus/encv-go/internal/mount"
	"github.com/Soltus/encv-go/internal/webdav"
	"github.com/gin-gonic/gin"
	goWebdav "golang.org/x/net/webdav"
)

type webdavFSEntry struct {
	fs         webdav.IndexProvider
	fileSystem goWebdav.FileSystem
	mount      *mount.Mount
	webdavPath string
}

func (s *Server) InitWebDAV(r *gin.Engine) {
	s.webdavFSByMount = make(map[string]*webdavFSEntry)

	webdavEnabledMounts := []*mount.Mount{}
	if s.mountRegistry != nil {
		for _, m := range s.mountRegistry.List() {
			if !m.Enabled {
				continue
			}
			webdavEnabledMounts = append(webdavEnabledMounts, m)
		}
	}

	hasPrimary := false
	for _, m := range webdavEnabledMounts {
		if m.Name == mount.NamePrimary {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary && s.webdavDir != "" {
		webdavEnabledMounts = append(webdavEnabledMounts, &mount.Mount{
			Name:      mount.NamePrimary,
			MountPath: "/",
			RootPath:  s.webdavDir,
			Enabled:   true,
		})
	}

	for _, m := range webdavEnabledMounts {
		var urlPrefix string
		if m.MountPath == "/" || m.MountPath == "" {
			urlPrefix = s.webdavPath
		} else {
			mp := strings.TrimSuffix(m.MountPath, "/")
			if !strings.HasPrefix(mp, "/") {
				mp = "/" + mp
			}
			urlPrefix = mp + "/"
		}

		primaryLegacyPrefix := ""
		if m.Name == mount.NamePrimary && s.webdavPath != "" && s.webdavPath != urlPrefix {
			primaryLegacyPrefix = s.webdavPath
		}

		entry, fsErr := s.registerWebdavHandlerForURL(r, m, urlPrefix)
		if fsErr != nil {
			errMsg := fmt.Sprintf("WebDAV init failed for mount %s (root=%s, url=%s): %v", m.Name, m.RootPath, urlPrefix, fsErr)
			slog.Warn(errMsg)
			s.mountBootstrapErrors = append(s.mountBootstrapErrors, errMsg)
			continue
		}
		s.webdavFSByMount[m.Name] = entry

		if m.Name == mount.NamePrimary && s.webdavFS == nil {
			s.webdavFS = entry.fs
		}

		if primaryLegacyPrefix != "" {
			_, legacyErr := s.registerWebdavHandlerForURL(r, m, primaryLegacyPrefix)
			if legacyErr != nil {
				errMsg := fmt.Sprintf("WebDAV legacy URL init failed for mount %s (url=%s): %v", m.Name, primaryLegacyPrefix, legacyErr)
				slog.Warn(errMsg)
				s.mountBootstrapErrors = append(s.mountBootstrapErrors, errMsg)
			} else {
				slog.Info("WebDAV legacy /webdav/ alias also registered (backward compat for old clients + Windows Explorer single-webdav)",
					"url_prefix", primaryLegacyPrefix)
			}
		}
	}
}

func (s *Server) webdavLoggingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			method := r.Method
			path := r.URL.Path
			clientIP := clientIPFromRequest(r)
			ua := r.Header.Get("User-Agent")

			lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(lrw, r)

			elapsed := time.Since(start)
			latencyCategory := latencyCategoryFromMs(elapsed.Milliseconds())

			attrs := []any{
				"method", method,
				"path", path,
				"status", lrw.statusCode,
				"bytes", lrw.bytesWritten,
				"elapsed_ms", elapsed.Milliseconds(),
				"latency", latencyCategory,
				"client_ip", clientIP,
			}
			if ua != "" {
				attrs = append(attrs, "ua", ua)
			}

			switch {
			case lrw.statusCode >= 500:
				slog.Error("WebDAV", attrs...)
			case lrw.statusCode >= 400:
				slog.Warn("WebDAV", attrs...)
			default:
				slog.Info("WebDAV", attrs...)
			}
		})
	}
}

func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func latencyCategoryFromMs(ms int64) string {
	switch {
	case ms < 50:
		return "fast"
	case ms < 200:
		return "normal"
	case ms < 1000:
		return "slow"
	default:
		return "very-slow"
	}
}

func (s *Server) registerWebdavHandlerForURL(r *gin.Engine, m *mount.Mount, urlPrefix string) (*webdavFSEntry, error) {
	webdavFSRaw, indexProvider, fsErr := webdav.NewENCVFSForRoot(
		config.NewContext(context.Background(), s.cfg),
		m.RootPath,
		urlPrefix,
		s.readerService,
		s.chunkNamers,
	)
	if fsErr != nil {
		return nil, fsErr
	}

	fsConcrete, ok := webdavFSRaw.(goWebdav.FileSystem)
	if !ok {
		return nil, fmt.Errorf("WebDAV FS for mount %s (url=%s) does not implement goWebdav.FileSystem", m.Name, urlPrefix)
	}

	webdavHandler := &goWebdav.Handler{
		Prefix:     urlPrefix,
		FileSystem: fsConcrete,
		LockSystem: goWebdav.NewMemLS(),
	}

	authMiddleware := middleware.BasicAuthDynamic(s)
	loggingMiddleware := s.webdavLoggingMiddleware()
	protectedWebdavHandler := authMiddleware(loggingMiddleware(webdavHandler))

	webdavMethods := []string{
		"GET", "POST", "PUT", "PATCH", "HEAD", "OPTIONS", "DELETE", "CONNECT", "TRACE",
		"PROPFIND", "PROPPATCH", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK",
	}

	for _, method := range webdavMethods {
		r.Handle(method, urlPrefix+"*path", gin.WrapH(protectedWebdavHandler))
	}
	rootURL := strings.TrimSuffix(urlPrefix, "/")
	if rootURL != "" {
		for _, method := range webdavMethods {
			r.Handle(method, rootURL, func(c *gin.Context) {
				c.Request.URL.Path = urlPrefix
				protectedWebdavHandler.ServeHTTP(c.Writer, c.Request)
			})
		}
	}

	entry := &webdavFSEntry{
		fs:         indexProvider,
		fileSystem: fsConcrete,
		mount:      m,
		webdavPath: urlPrefix,
	}

	slog.Info("WebDAV handler registered",
		"mount", m.Name,
		"mount_path", m.MountPath,
		"root_path", m.RootPath,
		"url_prefix", urlPrefix,
		"fs_webdav_prefix", indexProvider.WebdavPrefix(),
	)

	return entry, nil
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(p []byte) (int, error) {
	if lrw.statusCode == 0 {
		lrw.statusCode = http.StatusOK
	}
	n, err := lrw.ResponseWriter.Write(p)
	lrw.bytesWritten += int64(n)
	return n, err
}

func (lrw *loggingResponseWriter) Flush() {
	if f, ok := lrw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
