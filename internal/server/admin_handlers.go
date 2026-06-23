package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Soltus/encv-go/internal/auth"
	"github.com/Soltus/encv-go/internal/injector"
	"github.com/Soltus/encv-go/internal/routes"
	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleLoginGin(c *gin.Context) {
	jwtManager := s.jwtManager
	password := s.cfg.Admin.Password

	if c.Request.URL.Path != routes.Login {
		c.Status(http.StatusNotFound)
		return
	}

	if c.Request.Method == http.MethodGet {
		token := auth.GetTokenFromCookie(c.Request)
		if token != "" && jwtManager != nil {
			if _, err := jwtManager.ValidateToken(token); err == nil {
				slog.Debug("user already logged in")
				tmpl := template.Must(template.New("already_logged").Parse(`<!DOCTYPE html>
<html>
<head>
    <title>Already Logged In</title>
    <style>
        body { font-family: sans-serif; text-align: center; padding: 2em; }
        .message { margin: 2em; padding: 1em; background: #f0f8ff; border-radius: 4px; }
        a { color: #007bff; margin: 0 1em; }
    </style>
</head>
<body>
<div id="` + injector.InjectorID + `"></div>
    <div class="message">
        <h2>You are already logged in</h2>
        <p>
            <a href="` + routes.FSProxy + `/">Go to Files</a>
            <a href="` + routes.OpenListProxy + `/sites">OpenList</a>
            <a href="` + routes.Logout + `">Logout</a>
        </p>
    </div>
</body>
</html>`))
				c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
				tmpl.Execute(c.Writer, nil)
				return
			} else {
				slog.Debug("invalid token on login page", "error", err)
				auth.ClearAuthCookie(c.Writer)
			}
		}

		redirectURL := c.Query("redirect_url")
		if redirectURL == "" {
			redirectURL = c.Request.Referer()
		}

		var savedRedirectURL string
		if redirectURL != "" && !isLoginRelatedURL(redirectURL) {
			auth.SetRedirectCookie(c.Writer, redirectURL)
			savedRedirectURL = redirectURL
		}

		tmpl, _ := template.New("login").Parse(auth.LoginPageTmpl)
		data := map[string]interface{}{
			"RedirectURL": savedRedirectURL,
		}
		c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(c.Writer, data)
		return
	}

	// POST
	var req struct {
		Password    string `form:"password" json:"password"`
		RedirectURL string `form:"redirect_url" json:"redirect_url"`
	}

	if err := c.ShouldBind(&req); err != nil {
		tmpl, _ := template.New("login").Parse(auth.LoginPageTmpl)
		data := map[string]interface{}{
			"Error":       "Invalid request",
			"RedirectURL": "",
		}
		c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(c.Writer, data)
		return
	}

	if req.Password != password {
		tmpl, _ := template.New("login").Parse(auth.LoginPageTmpl)
		data := map[string]interface{}{
			"Error":       "Invalid password",
			"RedirectURL": "",
		}
		c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(c.Writer, data)
		return
	}

	if jwtManager == nil {
		c.Redirect(http.StatusFound, routes.FSProxy+"/")
		return
	}

	token, err := jwtManager.CreateToken()
	if err != nil {
		tmpl, _ := template.New("login").Parse(auth.LoginPageTmpl)
		data := map[string]interface{}{
			"Error":       "Failed to create token",
			"RedirectURL": "",
		}
		c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(c.Writer, data)
		return
	}

	auth.SetAuthCookie(c.Writer, token, 7*24*time.Hour)

	if req.RedirectURL != "" && !isLoginRelatedURL(req.RedirectURL) {
		auth.ClearRedirectCookie(c.Writer)
		c.Redirect(http.StatusFound, req.RedirectURL)
		return
	}

	if redirectURL := auth.GetRedirectCookie(c.Request); redirectURL != "" {
		auth.ClearRedirectCookie(c.Writer)
		if !isLoginRelatedURL(redirectURL) {
			c.Redirect(http.StatusFound, redirectURL)
			return
		}
	}

	c.Redirect(http.StatusFound, routes.FSProxy+"/")
}

func (s *Server) handleLogoutGin(c *gin.Context) {
	auth.ClearAuthCookie(c.Writer)
	c.Redirect(http.StatusFound, routes.Login)
}

func (s *Server) handleFileAnalyzeGin(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": "Bad request: path is required",
			"data":    nil,
		})
		return
	}

	absPath, err := s.resolveUserPath(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    44,
				"message": "Not found: the file " + absPath + " does not exist. | req.Path:" + req.Path + " | err: " + err.Error(),
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50,
			"message": "Internal server error: could not stat file",
			"data":    nil,
		})
		return
	}

	var htmlContent string

	currentExt := strings.ToLower(filepath.Ext(absPath))

	containerExtensions := plugins.GetAllRegisteredContainerExtensions()
	isContainerFile := false
	for _, ext := range containerExtensions {
		if currentExt == strings.ToLower(ext) {
			isContainerFile = true
			break
		}
	}

	if isContainerFile {
		htmlContent, err = detector.AnalyzeContainerV2(c.Request.Context(), absPath, false)
		if err != nil {
			slog.Error("failed to analyze container", "path", absPath, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50,
				"message": "Analysis failed: " + err.Error(),
				"data":    nil,
			})
			return
		}
	} else {
		htmlContent = fmt.Sprintf(`
			<h3>Basic File Information</h3>
			<table>
				<tr><td><strong>File Name:</strong></td><td>%s</td></tr>
				<tr><td><strong>Size:</strong></td><td>%d bytes</td></tr>
				<tr><td><strong>Mode:</strong></td><td>%s</td></tr>
				<tr><td><strong>Modified:</strong></td><td>%s</td></tr>
			</table>`,
			template.HTMLEscapeString(stat.Name()),
			stat.Size(),
			stat.Mode(),
			stat.ModTime().Format(time.RFC1123),
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "",
		"data": gin.H{
			"htmlContent": htmlContent,
		},
	})
}

func (s *Server) handleFileRenameGin(c *gin.Context) {
	var req struct {
		OldPath string `json:"oldPath" binding:"required"`
		NewName string `json:"newName" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": "Bad request: oldPath and newName are required",
			"data":    nil,
		})
		return
	}

	if req.NewName == "." || req.NewName == ".." || filepath.Base(req.NewName) != req.NewName {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": "Bad request: newName is not a valid filename",
			"data":    nil,
		})
		return
	}

	oldAbsPath, err := s.resolveUserPath(req.OldPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	newAbsPath := filepath.Join(filepath.Dir(oldAbsPath), req.NewName)

	task := s.mobileSvc.GetTaskManager().Create("rename", oldAbsPath, newAbsPath, "", 0, "")
	task.OriginalPath = oldAbsPath
	// 🆕 2026-06-23 WS 时序修复：Create 不再内部广播，外部补 RunId 兜底 + 持久化 + 广播
	s.mobileSvc.GetTaskManager().FinalizeCreatedTask(task)

	c.JSON(http.StatusAccepted, gin.H{
		"taskId": task.ID,
	})
}

func (s *Server) handleFileCopyGin(c *gin.Context) {
	var req struct {
		SrcPath  string `json:"srcPath" binding:"required"`
		DestPath string `json:"destPath" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": "Bad request: srcPath and destPath are required",
			"data":    nil,
		})
		return
	}

	srcAbsPath, err := s.resolveUserPath(req.SrcPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	destAbsPath, err := s.resolveUserPath(req.DestPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	task := s.mobileSvc.GetTaskManager().Create("copy", srcAbsPath, destAbsPath, "", 0, "")
	// 🆕 2026-06-23 WS 时序修复：Create 不再内部广播，外部补 RunId 兜底 + 持久化 + 广播
	s.mobileSvc.GetTaskManager().FinalizeCreatedTask(task)

	c.JSON(http.StatusAccepted, gin.H{
		"taskId": task.ID,
	})
}

func (s *Server) handleFileMoveGin(c *gin.Context) {
	var req struct {
		SrcPath  string `json:"srcPath" binding:"required"`
		DestPath string `json:"destPath" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": "Bad request: srcPath and destPath are required",
			"data":    nil,
		})
		return
	}

	srcAbsPath, err := s.resolveUserPath(req.SrcPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	destAbsPath, err := s.resolveUserPath(req.DestPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    51,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	task := s.mobileSvc.GetTaskManager().Create("move", srcAbsPath, destAbsPath, "", 0, "")
	task.OriginalPath = srcAbsPath
	// 🆕 2026-06-23 WS 时序修复：Create 不再内部广播，外部补 RunId 兜底 + 持久化 + 广播
	s.mobileSvc.GetTaskManager().FinalizeCreatedTask(task)

	c.JSON(http.StatusAccepted, gin.H{
		"taskId": task.ID,
	})
}

func (s *Server) handleHelloGin(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "OK",
		"data":    nil,
	})
}

func isLoginRelatedURL(url string) bool {
	if url == "" {
		return false
	}
	loginPaths := []string{
		routes.Login,
		routes.Logout,
	}
	for _, path := range loginPaths {
		if strings.Contains(url, path) {
			return true
		}
	}
	return false
}
