package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/encv/plugins"
)

func setupTestServer(t *testing.T, webdavUsername, webdavPassword string) (*Server, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "encv-webdav-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create subdir: %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello webdav test"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create test file: %v", err)
	}

	subFile := filepath.Join(subDir, "nested.txt")
	if err := os.WriteFile(subFile, []byte("nested content"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create nested file: %v", err)
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to find available port: %v", err)
	}
	availablePort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := &config.Config{
		Password: "test-password",
		Server: types.HttpServer{
			Port: availablePort,
			Dir:  tmpDir,
		},
		Webdav: types.WebdavServer{
			Root:     "/webdav/",
			Dir:      tmpDir,
			Username: webdavUsername,
			Password: webdavPassword,
		},
		Log: types.LogConfig{
			Level: "debug",
		},
		PluginSettings: map[string]json.RawMessage{},
	}

	if err := plugins.InitializeWithSettings(cfg.PluginSettings); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to initialize plugins: %v", err)
	}

	ctx := config.NewContext(context.Background(), cfg)
	s := NewServer(ctx, "")

	addr, err := s.Start("test")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to start server: %v", err)
	}

	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to parse server address '%s': %v", addr, splitErr)
	}
	if host == "" || host == "::" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	teardown := func() {
		s.Stop()
		os.RemoveAll(tmpDir)
	}

	return s, baseURL, teardown
}

func makePROPFIND(target string, username, password string) (*http.Response, error) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:resourcetype/>
    <d:displayname/>
    <d:getcontentlength/>
    <d:getlastmodified/>
  </d:prop>
</d:propfind>`

	req, err := http.NewRequest("PROPFIND", target, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")

	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	return client.Do(req)
}

func TestWebDAV_PROPFIND_Root(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	resp, err := makePROPFIND(baseURL+"/webdav/", "testuser", "testpass")
	if err != nil {
		t.Fatalf("PROPFIND request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected 207 Multi-Status, got %d, body: %s", resp.StatusCode, string(respBody))
	}
}

func TestWebDAV_PROPFIND_WithoutSlash(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	resp, err := makePROPFIND(baseURL+"/webdav", "testuser", "testpass")
	if err != nil {
		t.Fatalf("PROPFIND request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected 207 Multi-Status for /webdav (no trailing slash), got %d, body: %s", resp.StatusCode, string(respBody))
	}
}

func TestWebDAV_AuthRequired(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	resp, err := makePROPFIND(baseURL+"/webdav/", "", "")
	if err != nil {
		t.Fatalf("PROPFIND request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized without credentials, got %d", resp.StatusCode)
	}
}

func TestWebDAV_AuthSuccess(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	resp, err := makePROPFIND(baseURL+"/webdav/", "testuser", "testpass")
	if err != nil {
		t.Fatalf("PROPFIND request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("Expected 207 Multi-Status with correct credentials, got %d", resp.StatusCode)
	}
}

func TestWebDAV_AuthWrong(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	resp, err := makePROPFIND(baseURL+"/webdav/", "wronguser", "wrongpass")
	if err != nil {
		t.Fatalf("PROPFIND request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized with wrong credentials, got %d", resp.StatusCode)
	}
}

func TestWebDAV_Options(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	req, err := http.NewRequest("OPTIONS", baseURL+"/webdav/", nil)
	if err != nil {
		t.Fatalf("Failed to create OPTIONS request: %v", err)
	}
	req.SetBasicAuth("testuser", "testpass")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request failed: %v", err)
	}
	defer resp.Body.Close()

	allow := resp.Header.Get("Allow")
	if allow == "" {
		t.Error("Expected Allow header in OPTIONS response")
	}
}

func TestWebDAV_GetFile(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	req, err := http.NewRequest("GET", baseURL+"/webdav/test.txt", nil)
	if err != nil {
		t.Fatalf("Failed to create GET request: %v", err)
	}
	req.SetBasicAuth("testuser", "testpass")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for GET file, got %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(respBody) != "hello webdav test" {
		t.Errorf("Expected file content 'hello webdav test', got '%s'", string(respBody))
	}
}

func TestWebDAV_NoAuth(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "", "")
	defer teardown()

	resp, err := makePROPFIND(baseURL+"/webdav/", "", "")
	if err != nil {
		t.Fatalf("PROPFIND request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected 207 Multi-Status without auth when no credentials configured, got %d, body: %s", resp.StatusCode, string(respBody))
	}
}

func TestWebDAV_PROPFIND_SubDir(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	resp, err := makePROPFIND(baseURL+"/webdav/subdir/", "testuser", "testpass")
	if err != nil {
		t.Fatalf("PROPFIND request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected 207 Multi-Status for subdirectory, got %d, body: %s", resp.StatusCode, string(respBody))
	}
}

func TestWebDAV_URLEncodedPath(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	encodedPath := url.PathEscape("subdir/nested.txt")
	req, err := http.NewRequest("GET", baseURL+"/webdav/"+encodedPath, nil)
	if err != nil {
		t.Fatalf("Failed to create GET request: %v", err)
	}
	req.SetBasicAuth("testuser", "testpass")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for URL-encoded path, got %d", resp.StatusCode)
	}
}

func TestWebDAV_TestLocalAPI_Enabled(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	resp, err := http.Get(baseURL + "/api/webdav/test-local")
	if err != nil {
		t.Fatalf("test-local request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if available, ok := result["available"].(bool); !ok || !available {
		t.Errorf("Expected available=true, got %v", result["available"])
	}

	if _, ok := result["url"]; !ok {
		t.Error("Expected url field in response")
	}

	if authRequired, ok := result["authRequired"].(bool); !ok || !authRequired {
		t.Errorf("Expected authRequired=true (credentials configured), got %v", result["authRequired"])
	}

	details, ok := result["details"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected details object in response")
	}

	if propfindRoot, ok := details["propfindRoot"].(string); !ok || propfindRoot != "ok" {
		t.Errorf("Expected propfindRoot=ok, got %v", details["propfindRoot"])
	}

	if authWorks, ok := details["authWorks"].(string); !ok || authWorks != "ok" {
		t.Errorf("Expected authWorks=ok, got %v", details["authWorks"])
	}

	if dirReadable, ok := details["dirReadable"].(string); !ok || dirReadable != "ok" {
		t.Errorf("Expected dirReadable=ok, got %v", details["dirReadable"])
	}
}

func TestWebDAV_TestLocalAPI_NoAuth(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "", "")
	defer teardown()

	resp, err := http.Get(baseURL + "/api/webdav/test-local")
	if err != nil {
		t.Fatalf("test-local request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if available, ok := result["available"].(bool); !ok || !available {
		t.Errorf("Expected available=true, got %v", result["available"])
	}

	if authRequired, ok := result["authRequired"].(bool); !ok || authRequired {
		t.Errorf("Expected authRequired=false (no credentials configured), got %v", result["authRequired"])
	}

	details, ok := result["details"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected details object in response")
	}

	if propfindRoot, ok := details["propfindRoot"].(string); !ok || propfindRoot != "ok" {
		t.Errorf("Expected propfindRoot=ok, got %v", details["propfindRoot"])
	}

	if authWorks, ok := details["authWorks"].(string); !ok || authWorks != "skip" {
		t.Errorf("Expected authWorks=skip (no auth configured), got %v", details["authWorks"])
	}
}

func TestWebDAV_TestLocalAPI_WrongAuth(t *testing.T) {
	_, baseURL, teardown := setupTestServer(t, "testuser", "testpass")
	defer teardown()

	details := testLocalWebDAVDetails(t, baseURL)

	if propfindRoot, ok := details["propfindRoot"].(string); !ok || propfindRoot != "ok" {
		t.Errorf("Expected propfindRoot=ok (server responds to PROPFIND), got %v", details["propfindRoot"])
	}

	if authWorks, ok := details["authWorks"].(string); !ok || authWorks != "ok" {
		t.Errorf("Expected authWorks=ok (test-local uses correct credentials), got %v", details["authWorks"])
	}
}

func testLocalWebDAVDetails(t *testing.T, baseURL string) map[string]interface{} {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/webdav/test-local")
	if err != nil {
		t.Fatalf("test-local request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	details, ok := result["details"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected details object in response")
	}
	return details
}
