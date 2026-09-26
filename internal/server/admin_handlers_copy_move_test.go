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
	"time"

	"github.com/Soltus/encv-go/internal/config"
	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCopyMoveTestServer(t *testing.T, servingDir string) (*gin.Engine, *Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// 【关键】copy / move / rename 早已改成异步任务：handler 通过
	// s.mobileSvc.GetTaskManager() 建任务，只回 202 + taskId。
	// 旧测试只填了 servingDir 而没注入 mobileSvc → handler 里 nil 指针直接 panic，
	// 整个 server 包测试进程被 SIGSEGV 打断（后续用例全部无法执行）。
	s := &Server{
		servingDir: servingDir,
		mobileSvc:  mobileservice.NewMobileService(servingDir, &config.Config{}),
	}
	adminGroup := router.Group("/api/file")
	adminGroup.POST("/copy", s.handleFileCopyGin)
	adminGroup.POST("/move", s.handleFileMoveGin)
	return router, s
}

// waitTaskResult 等待异步任务落到终态，并返回终态任务。
//
// 终态判定与后端一致：completed / failed / cancelled（见 internal/service）。
func waitTaskResult(t *testing.T, s *Server, taskID string) *mobileservice.MobileTask {
	t.Helper()
	require.NotEmpty(t, taskID, "异步契约：响应必须带 taskId")

	var task *mobileservice.MobileTask
	require.Eventually(t, func() bool {
		got, err := s.mobileSvc.GetTaskManager().Get(taskID)
		if err != nil {
			return false
		}
		task = got
		switch task.Status {
		case "completed", "failed", "cancelled":
			return true
		}
		return false
	}, 10*time.Second, 20*time.Millisecond, "任务 %s 未能在超时前到达终态", taskID)
	return task
}

func TestHandleFileCopyGin_Success(t *testing.T) {
	tmpDir := t.TempDir()
	srcContent := []byte("hello copy test")
	srcPath := filepath.Join(tmpDir, "source.txt")
	require.NoError(t, os.WriteFile(srcPath, srcContent, 0644))

	router, srv := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "source.txt",
		"destPath": "copied.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/copy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// 异步契约：202 Accepted + taskId（不再是同步 200 + code 0 + "File copied"）
	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	taskID, _ := resp["taskId"].(string)
	task := waitTaskResult(t, srv, taskID)
	require.Equal(t, "completed", task.Status, "copy 任务应完成，实际状态 %s / %s", task.Status, task.Error)

	destAbsPath := filepath.Join(tmpDir, "copied.txt")
	destContent, err := os.ReadFile(destAbsPath)
	require.NoError(t, err)
	assert.Equal(t, srcContent, destContent)
}

// TestHandleFileCopyGin_DestExists 锁住「目标已存在 → 拒绝，绝不覆盖」。
//
// 背景：改成异步任务后 handler 的 os.Stat 冲突校验被误删，任务层 os.Create 变成
// 静默覆盖；而 CopyRollbackStrategy 只做 os.Remove(target)，一旦覆盖过，
// 回滚就等于删掉用户原有文件。现在校验放在 FileTaskHandler 执行前（唯一可靠防线），
// HTTP 仍是 202，冲突体现为任务 failed。
func TestHandleFileCopyGin_DestExists(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "copied.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("src content"), 0644))
	require.NoError(t, os.WriteFile(destPath, []byte("dest content"), 0644))

	router, srv := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "source.txt",
		"destPath": "copied.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/copy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	taskID, _ := resp["taskId"].(string)
	task := waitTaskResult(t, srv, taskID)
	require.Equal(t, "failed", task.Status, "目标已存在时任务必须失败")
	assert.Contains(t, task.Error, "already exists")

	destContent, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "dest content", string(destContent), "目标文件必须原封不动（不得覆盖）")
}

// TestHandleFileCopyGin_OverwriteRefusedWithoutTrashStore 锁住 fail-safe 语义。
//
// 覆盖的前提是「可恢复」：被覆盖的文件必须先进回收站并落库（否则无法还原）。
// 测试用的 Server 没有注入 task store（与 server 启动早期一致），
// 因此 TrashManager 拒绝入站 → 覆盖请求必须失败，绝不能硬覆盖。
//
// 覆盖成功的路径（含回收站断言）见 internal/service/file_task_handler_test.go。
func TestHandleFileCopyGin_OverwriteRefusedWithoutTrashStore(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "copied.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("src content"), 0644))
	require.NoError(t, os.WriteFile(destPath, []byte("dest content"), 0644))

	router, srv := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]any{
		"srcPath":   "source.txt",
		"destPath":  "copied.txt",
		"overwrite": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/copy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	taskID, _ := resp["taskId"].(string)
	task := waitTaskResult(t, srv, taskID)
	require.Equal(t, "failed", task.Status, "无法安全覆盖时必须失败")

	content, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "dest content", string(content), "目标文件必须原封不动")
}

func TestHandleFileCopyGin_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	router, srv := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "nonexistent.txt",
		"destPath": "target.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/copy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// 异步契约：源不存在的校验下沉到任务里 → HTTP 仍是 202，任务以 failed 终结。
	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	taskID, _ := resp["taskId"].(string)
	task := waitTaskResult(t, srv, taskID)
	require.Equal(t, "failed", task.Status, "源不存在时任务应失败")
	assert.Contains(t, task.Error, "no such file", "失败原因应指出源文件不存在")
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

	router, srv := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "source.txt",
		"destPath": "moved.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// 异步契约：202 Accepted + taskId
	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	taskID, _ := resp["taskId"].(string)
	task := waitTaskResult(t, srv, taskID)
	require.Equal(t, "completed", task.Status, "move 任务应完成，实际状态 %s / %s", task.Status, task.Error)

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

	router, srv := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "source.txt",
		"destPath": "moved.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// 与 copy 一致：异步契约 + 目标存在时拒绝覆盖
	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	taskID, _ := resp["taskId"].(string)
	task := waitTaskResult(t, srv, taskID)
	require.Equal(t, "failed", task.Status, "目标已存在时任务必须失败")
	assert.Contains(t, task.Error, "already exists")

	destContent, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "existing dest", string(destContent), "目标文件必须原封不动（不得覆盖）")

	assert.FileExists(t, srcPath, "任务失败时源文件必须还在")
}

func TestHandleFileMoveGin_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	router, srv := setupCopyMoveTestServer(t, tmpDir)

	body, _ := json.Marshal(map[string]string{
		"srcPath":  "nonexistent.txt",
		"destPath": "target.txt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/file/move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// 异步契约：源不存在的校验下沉到任务里 → HTTP 202，任务 failed
	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	taskID, _ := resp["taskId"].(string)
	task := waitTaskResult(t, srv, taskID)
	require.Equal(t, "failed", task.Status, "源不存在时任务应失败")
	assert.Contains(t, task.Error, "no such file")
}
