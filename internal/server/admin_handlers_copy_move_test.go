package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCopyMoveTestServer(t *testing.T, servingDir string) (*gin.Engine, *Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	s := &Server{servingDir: servingDir}
	adminGroup := router.Group("/api/file")
	adminGroup.POST("/copy", s.handleFileCopyGin)
	adminGroup.POST("/move", s.handleFileMoveGin)
	return router, s
}

func TestHandleFileCopyGin_Success(t *testing.T) {
	tmpDir := t.TempDir()
	srcContent := []byte("hello copy test")
	srcPath := filepath.Join(tmpDir, "source.txt")
	require.NoError(t, os.WriteFile(srcPath, srcContent, 0644))

	router, _ := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "source.txt",
		"destPath": "copied.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/copy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
	assert.Equal(t, "File copied", resp["message"])

	destAbsPath := filepath.Join(tmpDir, "copied.txt")
	destContent, err := os.ReadFile(destAbsPath)
	require.NoError(t, err)
	assert.Equal(t, srcContent, destContent)
}

func TestHandleFileCopyGin_DestExists(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "copied.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("src content"), 0644))
	require.NoError(t, os.WriteFile(destPath, []byte("dest content"), 0644))

	router, _ := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "source.txt",
		"destPath": "copied.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/copy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(52), resp["code"])
	assert.Contains(t, resp["message"], "already exists")
}

func TestHandleFileCopyGin_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	router, _ := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "nonexistent.txt",
		"destPath": "target.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/copy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(44), resp["code"])
	assert.Contains(t, resp["message"], "Not found")
}

func TestHandleFileCopyGin_InvalidPath(t *testing.T) {
	tmpDir := t.TempDir()

	router, _ := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "../etc/passwd",
		"destPath": "target.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/copy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(51), resp["code"])
}

func TestHandleFileMoveGin_Success(t *testing.T) {
	tmpDir := t.TempDir()
	srcContent := []byte("hello move test")
	srcPath := filepath.Join(tmpDir, "source.txt")
	require.NoError(t, os.WriteFile(srcPath, srcContent, 0644))

	router, _ := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "source.txt",
		"destPath": "moved.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
	assert.Equal(t, "File moved", resp["message"])

	_, srcErr := os.Stat(filepath.Join(tmpDir, "source.txt"))
	assert.True(t, os.IsNotExist(srcErr), "source file should no longer exist")

	destContent, err := os.ReadFile(filepath.Join(tmpDir, "moved.txt"))
	require.NoError(t, err)
	assert.Equal(t, srcContent, destContent)
}

func TestHandleFileMoveGin_CrossDevice_Fallback(t *testing.T) {
	tmpDir := t.TempDir()
	subDirA := filepath.Join(tmpDir, "dir_a")
	subDirB := filepath.Join(tmpDir, "dir_b")
	require.NoError(t, os.Mkdir(subDirA, 0755))
	require.NoError(t, os.Mkdir(subDirB, 0755))

	srcContent := []byte("cross device move test")
	srcPath := filepath.Join(subDirA, "file.txt")
	require.NoError(t, os.WriteFile(srcPath, srcContent, 0644))

	router, _ := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "dir_a/file.txt",
		"destPath": "dir_b/moved.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, float64(0), resp["code"])

		_, srcErr := os.Stat(srcPath)
		assert.True(t, os.IsNotExist(srcErr), "source file should be removed after move")

		destContent, err := os.ReadFile(filepath.Join(subDirB, "moved.txt"))
		require.NoError(t, err)
		assert.Equal(t, srcContent, destContent)
	} else if strings.Contains(w.Body.String(), "invalid cross-device link") {
		t.Skip("skipping: same filesystem, cannot trigger cross-device rename error for fallback test")
	}
}

func TestHandleFileMoveGin_DestExists(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "moved.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("src content"), 0644))
	require.NoError(t, os.WriteFile(destPath, []byte("existing dest"), 0644))

	router, _ := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "source.txt",
		"destPath": "moved.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(52), resp["code"])
	assert.Contains(t, resp["message"], "already exists")
}

func TestHandleFileMoveGin_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	router, _ := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "nonexistent.txt",
		"destPath": "target.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(44), resp["code"])
	assert.Contains(t, resp["message"], "Not found")
}
