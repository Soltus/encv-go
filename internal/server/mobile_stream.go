package server

// mobile_stream.go — SSE 流式端点：list_files_stream / alist_encrypt_stream / plugin_files_stream / alist_decode_filename。

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/container/detector"
	alistencrypt "github.com/Soltus/encv-go/internal/v2/plugins/alistencrypt"
	"github.com/gin-gonic/gin"
)

func (s *Server) writeSSEEvent(c *gin.Context, flusher http.Flusher, data string) {
	c.Writer.Write([]byte("data: " + data + "\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

func (s *Server) handleListFilesStreamGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	if queryPath == "" {
		queryPath = "/"
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		flusher = nil
	}

	// 🆕 2026-06-15 multi-mount 适配：mount 虚拟根 /d 走 mount list（与 handleListFilesGin 同步）
	if (queryPath == "/d" || queryPath == "/d/") && s.mountRegistry != nil {
		files := mountListAsFiles(s.mountRegistry.List())
		for _, fi := range files {
			data, _ := json.Marshal(fi)
			s.writeSSEEvent(c, flusher, string(data))
		}
		s.writeSSEEvent(c, flusher, `[DONE]`)
		return
	}

	absPath, err := s.resolveUserPath(queryPath)
	if err != nil {
		s.writeSSEEvent(c, flusher, `{"error":"invalid path"}`)
		s.writeSSEEvent(c, flusher, `[DONE]`)
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		errMsg := fmt.Sprintf(`{"error":"cannot read directory: %s"}`, err.Error())
		s.writeSSEEvent(c, flusher, errMsg)
		s.writeSSEEvent(c, flusher, `[DONE]`)
		return
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		filePath := queryPath + "/" + entry.Name()
		if queryPath == "/" {
			filePath = "/" + entry.Name()
		}

		info, err := entry.Info()
		if err != nil {
			fi := mobileservice.FileInfo{
				Name:        entry.Name(),
				Path:        filePath,
				IsDirectory: entry.IsDir(),
				Size:        0,
				Modified:    "",
			}
			data, _ := json.Marshal(fi)
			s.writeSSEEvent(c, flusher, string(data))
			continue
		}

		isEncrypted := false
		if !entry.IsDir() {
			entryAbsPath := filepath.Join(absPath, entry.Name())
			if _, detectErr := detector.DetectContainer(entryAbsPath); detectErr == nil {
				isEncrypted = true
			}
		}

		fi := mobileservice.FileInfo{
			Name:        entry.Name(),
			Path:        filePath,
			IsDirectory: entry.IsDir(),
			IsEncrypted: isEncrypted,
			Size:        info.Size(),
			Modified:    info.ModTime().Format(time.RFC3339),
		}
		data, _ := json.Marshal(fi)
		s.writeSSEEvent(c, flusher, string(data))
	}

	s.writeSSEEvent(c, flusher, `[DONE]`)
}

func (s *Server) handleAlistEncryptStreamGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	password := c.Query("password")
	if queryPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'path' query parameter is required"})
		return
	}

	absPath, err := s.resolveUserPath(queryPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}

	slog.Info("API: alist-encrypt stream", "path", absPath)

	// 走统一范式：构造 FileContentProvider，调 ContentHandler.ServeFile
	// 与 v4 容器预览共享同一套 HTTP 协议处理（Range/206/Content-Length/Content-Range）
	var plugin alistencrypt.AlistEncryptPlugin
	rc, size, _, showName, err := plugin.Stream(absPath, password)
	if err != nil {
		slog.Error("API: alist-encrypt stream open failed", "error", err)
		writeServiceErrorGin(c, err)
		return
	}
	sr, ok := rc.(*alistencrypt.SeekableDecryptReader)
	if !ok {
		_ = rc.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal: unexpected reader type"})
		return
	}
	prov := alistencrypt.NewAlistEncryptFileProvider(sr, size, showName)
	defer prov.Close()
	s.contentHandler.ServeFile(c.Writer, c.Request, prov)
}

func (s *Server) handleAlistDecodeFilenameGin(c *gin.Context) {
	encoded := utils.DecodeGinQueryParam(c.Query("encoded"))
	password := c.Query("password")
	encType := c.DefaultQuery("enc_type", "aesctr")

	if encoded == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'encoded' query parameter is required"})
		return
	}

	plainName := alistencrypt.DecodeName(encoded, password, encType)
	c.JSON(http.StatusOK, gin.H{
		"plain_name": plainName,
		"success":    plainName != "",
	})
}

func (s *Server) handlePluginFilesStreamGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	if queryPath == "" {
		queryPath = "/"
	}
	extensionsStr := c.Query("extensions")
	if extensionsStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'extensions' query parameter is required"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		flusher = nil
	}

	absPath, err := s.resolveUserPath(queryPath)
	if err != nil {
		s.writeSSEEvent(c, flusher, `{"error":"invalid path"}`)
		s.writeSSEEvent(c, flusher, `[DONE]`)
		return
	}

	extSet := make(map[string]bool)
	for _, ext := range strings.Split(extensionsStr, ",") {
		e := strings.TrimSpace(strings.ToLower(ext))
		if e != "" {
			extSet[e] = true
		}
	}

	const maxResults = 500
	count := 0

	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if count >= maxResults {
			return fs.SkipAll
		}

		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		if !extSet[ext] {
			return nil
		}

		relPath, _ := filepath.Rel(absPath, path)
		filePath := queryPath + "/" + relPath
		if queryPath == "/" {
			filePath = "/" + relPath
		}

		info, err := d.Info()
		if err != nil {
			fi := mobileservice.FileInfo{
				Name:        name,
				Path:        filePath,
				IsDirectory: false,
				Size:        0,
				Modified:    "",
			}
			data, _ := json.Marshal(fi)
			s.writeSSEEvent(c, flusher, string(data))
			count++
			return nil
		}

		isEncrypted := false
		if _, detectErr := detector.DetectContainer(path); detectErr == nil {
			isEncrypted = true
		}

		fi := mobileservice.FileInfo{
			Name:        name,
			Path:        filePath,
			IsDirectory: false,
			IsEncrypted: isEncrypted,
			Size:        info.Size(),
			Modified:    info.ModTime().Format(time.RFC3339),
		}
		data, _ := json.Marshal(fi)
		s.writeSSEEvent(c, flusher, string(data))
		count++
		return nil
	})

	if err != nil && count < maxResults {
		errMsg := fmt.Sprintf(`{"error":"walk failed: %s"}`, err.Error())
		s.writeSSEEvent(c, flusher, errMsg)
	}

	s.writeSSEEvent(c, flusher, `[DONE]`)
}
