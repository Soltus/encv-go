package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/gin-gonic/gin"
)

func newTestGinContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	return c, w
}

func TestGetConfig_DoesNotApplyMobileOverlay(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")

	rawConfig := map[string]interface{}{
		"server": map[string]interface{}{
			"dir":  "/",
			"port": float64(2025),
		},
		"mobile": map[string]interface{}{
			"server": map[string]interface{}{
				"dir": "/storage/emulated/0",
			},
		},
		"password": "test-key",
	}
	data, _ := json.Marshal(rawConfig)
	os.WriteFile(cfgPath, data, 0644)

	s := &Server{
		configPath: cfgPath,
		cfg:        &config.Config{},
	}

	c, w := newTestGinContext("GET", "/api/config", nil)
	s.handleGetConfigGin(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)

	serverObj, ok := result["server"].(map[string]interface{})
	if !ok {
		t.Fatal("server object missing from response")
	}
	serverDir := serverObj["dir"].(string)
	if serverDir != "/" {
		t.Errorf("GET /api/config returned server.dir=%q (overlay leaked!), want %q", serverDir, "/")
	}
}

func TestPutConfig_StripsMobileSectionAndProtectsOverlayFields(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")

	existingConfig := `{"server":{"dir":"/","port":2025},"password":"old-key","output_path":"./output"}`
	os.WriteFile(cfgPath, []byte(existingConfig), 0644)

	s := &Server{
		configPath: cfgPath,
		cfg:        &config.Config{},
	}

	bodyWithMobile := map[string]interface{}{
		"server": map[string]interface{}{
			"dir":  "/storage/emulated/0",
			"port": float64(2025),
		},
		"mobile": map[string]interface{}{
			"server": map[string]interface{}{
				"dir": "/storage/emulated/0",
			},
		},
		"output_path": "/storage/emulated/0/encv-output",
		"password":    "new-key",
	}
	bodyData, _ := json.Marshal(bodyWithMobile)

	t.Setenv("ENCV_MOBILE", "1")
	defer os.Unsetenv("ENCV_MOBILE")

	c, w := newTestGinContext("PUT", "/api/config", bodyData)
	s.handlePutConfigGin(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	writtenData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	var written map[string]interface{}
	json.Unmarshal(writtenData, &written)

	if _, hasMobile := written["mobile"]; hasMobile {
		t.Errorf("mobile section was written to file! content:\n%s", string(writtenData))
	}

	serverObj := written["server"].(map[string]interface{})
	serverDir := serverObj["dir"].(string)
	if serverDir == "/storage/emulated/0" {
		t.Errorf("PUT wrote overlay value server.dir=%q - mobile value leaked to persistence!", serverDir)
	}
	if serverDir != "/" {
		t.Errorf("server.dir should be original value /, got %q", serverDir)
	}

	outputPath, _ := written["output_path"].(string)
	if outputPath == "/storage/emulated/0/encv-output" {
		t.Errorf("PUT wrote overlay value output_path=%q - mobile value leaked!", outputPath)
	}
	if outputPath != "./output" {
		t.Errorf("output_path should be original value ./output, got %q", outputPath)
	}
}

func TestPutConfig_MobileOnlyRequestDoesNotCorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")

	existingConfig := `{"server":{"dir":"/","port":2025,"password":"secret"},"output_path":"./output"}`
	os.WriteFile(cfgPath, []byte(existingConfig), 0644)

	s := &Server{
		configPath: cfgPath,
		cfg:        &config.Config{},
	}

	mobileOnlyBody := map[string]interface{}{
		"mobile": map[string]interface{}{
			"server": map[string]interface{}{"dir": "/storage/emulated/0"},
			"output": map[string]interface{}{"path": "/storage/emulated/0/encv-output"},
		},
	}
	bodyData, _ := json.Marshal(mobileOnlyBody)

	c, w := newTestGinContext("PUT", "/api/config", bodyData)
	s.handlePutConfigGin(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	writtenData, _ := os.ReadFile(cfgPath)
	var written map[string]interface{}
	json.Unmarshal(writtenData, &written)

	if _, hasMobile := written["mobile"]; hasMobile {
		t.Error("mobile-only PUT should not persist mobile section")
	}

	serverObj := written["server"].(map[string]interface{})
	if serverObj["dir"] != "/" {
		t.Errorf("server.dir should remain unchanged as /, got %q", serverObj["dir"])
	}
}

func TestPutConfig_NormalUserEditPreservesChanges(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")

	existingConfig := `{"server":{"dir":"/","port":2025},"password":"old-key"}`
	os.WriteFile(cfgPath, []byte(existingConfig), 0644)

	s := &Server{
		configPath: cfgPath,
		cfg:        &config.Config{},
	}

	normalEdit := map[string]interface{}{
		"server": map[string]interface{}{
			"dir":  "/",
			"port": float64(3030),
		},
		"password": "new-key",
	}
	bodyData, _ := json.Marshal(normalEdit)

	c, w := newTestGinContext("PUT", "/api/config", bodyData)
	s.handlePutConfigGin(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var written map[string]interface{}
	json.Unmarshal(_readFile(t, cfgPath), &written)

	serverObj := written["server"].(map[string]interface{})
	if serverObj["port"] != float64(3030) {
		t.Errorf("port change not saved: got %v", serverObj["port"])
	}
	if written["password"] != "new-key" {
		t.Errorf("password change not saved: got %v", written["password"])
	}
}

func _readFile(t *testing.T, p string) []byte {
	d, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return d
}
