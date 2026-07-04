package server

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/plugins/alistencrypt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPassword = "test_password_123"

func setupAlistEncryptTestRouter(t *testing.T, servingDir string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	s := &Server{servingDir: servingDir}
	router.GET("/api/alist-encrypt/stream", s.handleAlistEncryptStreamGin)
	router.GET("/api/alist-encrypt/decode-filename", s.handleAlistDecodeFilenameGin)
	return router
}

// ===== handleAlistDecodeFilenameGin =====

func TestHandleAlistDecodeFilenameGin_NormalDecode(t *testing.T) {
	router := setupAlistEncryptTestRouter(t, "")

	plainName := "test.txt"
	encoded := alistencrypt.EncodeName(plainName, testPassword, "aesctr")
	if encoded == "" {
		t.Fatal("EncodeName returned empty for test.txt")
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/decode-filename?encoded="+url.QueryEscape(encoded)+"&password="+testPassword, nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.Equal(t, plainName, response["plain_name"])
}

func TestHandleAlistDecodeFilenameGin_EmptyEncoded(t *testing.T) {
	router := setupAlistEncryptTestRouter(t, "")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/decode-filename?password="+testPassword, nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "'encoded'")
}

func TestHandleAlistDecodeFilenameGin_WrongPassword(t *testing.T) {
	router := setupAlistEncryptTestRouter(t, "")

	plainName := "secret.txt"
	encoded := alistencrypt.EncodeName(plainName, testPassword, "aesctr")
	if encoded == "" {
		t.Fatal("EncodeName returned empty for secret.txt")
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/decode-filename?encoded="+url.QueryEscape(encoded)+"&password=wrong_password", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.False(t, response["success"].(bool))
	assert.Equal(t, "", response["plain_name"])
}

func TestHandleAlistDecodeFilenameGin_DefaultEncType(t *testing.T) {
	router := setupAlistEncryptTestRouter(t, "")

	plainName := "data.json"
	encoded := alistencrypt.EncodeName(plainName, testPassword, "aesctr")
	if encoded == "" {
		t.Fatal("EncodeName returned empty for hello.pdf")
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/decode-filename?encoded="+url.QueryEscape(encoded)+"&password="+testPassword, nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.Equal(t, plainName, response["plain_name"])
}

// ===== handleAlistEncryptStreamGin =====

func TestHandleAlistEncryptStreamGin_EmptyPath(t *testing.T) {
	router := setupAlistEncryptTestRouter(t, "")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/stream?password="+testPassword, nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "'path'")
}

func TestHandleAlistEncryptStreamGin_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	router := setupAlistEncryptTestRouter(t, tmpDir)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/stream?path=/nonexistent.bin&password="+testPassword, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func makeV2EncryptedFile(t *testing.T, dir string, filename string, plainData []byte, password string) string {
	t.Helper()
	cipher, err := alistencrypt.NewAesCtr(password, int64(len(plainData)))
	require.NoError(t, err)
	encrypted := make([]byte, len(plainData))
	copy(encrypted, plainData)
	cipher.Encrypt(encrypted)

	headerBuf := make([]byte, 32)
	copy(headerBuf[:6], []byte(alistencrypt.AECTR2Magic))
	headerBuf[6] = 0x02
	headerBuf[7] = 0x00
	binary.BigEndian.PutUint64(headerBuf[24:32], uint64(len(plainData)))

	binPath := filepath.Join(dir, filename)
	err = os.WriteFile(binPath, append(headerBuf, encrypted...), 0644)
	require.NoError(t, err)
	return binPath
}

func TestHandleAlistEncryptStreamGin_ValidFileAndPassword(t *testing.T) {
	tmpDir := t.TempDir()
	router := setupAlistEncryptTestRouter(t, tmpDir)

	plainData := []byte("Hello, this is plaintext content for streaming test!")
	makeV2EncryptedFile(t, tmpDir, "test_video.bin", plainData, testPassword)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/stream?path=/test_video.bin&password="+testPassword, nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "/")
	assert.Equal(t, plainData, w.Body.Bytes())
}

func TestHandleAlistEncryptStreamGin_RangeStartExceedsSize(t *testing.T) {
	tmpDir := t.TempDir()
	router := setupAlistEncryptTestRouter(t, tmpDir)

	plainData := []byte("small")
	makeV2EncryptedFile(t, tmpDir, "small.bin", plainData, testPassword)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/stream?path=/small.bin&password="+testPassword, nil)
	req.Header.Set("Range", "bytes=100-200")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, w.Code)
}

func TestHandleAlistEncryptStreamGin_InvalidRangeFormat(t *testing.T) {
	tmpDir := t.TempDir()
	router := setupAlistEncryptTestRouter(t, tmpDir)

	plainData := []byte("data")
	makeV2EncryptedFile(t, tmpDir, "data.bin", plainData, testPassword)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/stream?path=/data.bin&password="+testPassword, nil)
	req.Header.Set("Range", "bytes=garbage")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAlistEncryptStreamGin_NegativeRangeValues(t *testing.T) {
	tmpDir := t.TempDir()
	router := setupAlistEncryptTestRouter(t, tmpDir)

	plainData := []byte("data")
	makeV2EncryptedFile(t, tmpDir, "data.bin", plainData, testPassword)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/stream?path=/data.bin&password="+testPassword, nil)
	req.Header.Set("Range", "bytes=-5--1")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, w.Code)
}

func TestHandleAlistEncryptStreamGin_RangePartialContent(t *testing.T) {
	tmpDir := t.TempDir()
	router := setupAlistEncryptTestRouter(t, tmpDir)

	plainData := []byte("0123456789ABCDEFGHIJ")
	makeV2EncryptedFile(t, tmpDir, "range.bin", plainData, testPassword)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/stream?path=/range.bin&password="+testPassword, nil)
	req.Header.Set("Range", "bytes=5-14")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, "56789ABCDE", w.Body.String())
}

func TestHandleAlistEncryptStreamGin_WrongPasswordReturnsGarbageNotError(t *testing.T) {
	tmpDir := t.TempDir()
	router := setupAlistEncryptTestRouter(t, tmpDir)

	plainData := []byte("correct_plaintext")
	makeV2EncryptedFile(t, tmpDir, "secret.bin", plainData, testPassword)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/alist-encrypt/stream?path=/secret.bin&password=wrongpass", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.Bytes()
	if string(body) == string(plainData) {
		t.Error("wrong password should produce garbled output, not plaintext")
	}
}
