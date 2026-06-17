package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPluginsTestRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	s := &Server{}
	router.GET("/api/plugins", s.handlePluginsGin)
	return router
}

func TestHandlePluginsGin_ReturnsAllPlugins(t *testing.T) {
	router := setupPluginsTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/plugins", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	plugins, ok := response["plugins"].([]interface{})
	require.True(t, ok, "response should contain 'plugins' array")
	assert.Len(t, plugins, 7, "plugins array length should match plugins.Plugins slice length")
}

func TestHandlePluginsGin_ContainsVideoPlugin(t *testing.T) {
	router := setupPluginsTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/plugins", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string][]map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	found := false
	for _, p := range response["plugins"] {
		if p["name"] == "video" {
			found = true
			exts, ok := p["supportedExtensions"].([]interface{})
			require.True(t, ok, "video plugin should have supportedExtensions array")
			assert.NotEmpty(t, exts, "video plugin should have supported extensions")
			break
		}
	}
	assert.True(t, found, "result should contain a plugin with name='video'")
}

func TestHandlePluginsGin_EachPluginHasRequiredFields(t *testing.T) {
	router := setupPluginsTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/plugins", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string][]map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	for i, p := range response["plugins"] {
		_, hasName := p["name"]
		_, hasExts := p["supportedExtensions"]
		_, hasMime := p["supportedMimePrefixes"]
		_, hasContainer := p["containerExtension"]

		assert.True(t, hasName, "plugin at index %d should have 'name' field", i)
		assert.True(t, hasExts, "plugin at index %d should have 'supportedExtensions' field", i)
		assert.True(t, hasMime, "plugin at index %d should have 'supportedMimePrefixes' field", i)
		assert.True(t, hasContainer, "plugin at index %d should have 'containerExtension' field", i)
	}
}

func TestHandlePluginsGin_VideoPluginHasVideoMimePrefix(t *testing.T) {
	router := setupPluginsTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/plugins", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string][]map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	var videoPlugin map[string]interface{}
	for _, p := range response["plugins"] {
		if p["name"] == "video" {
			videoPlugin = p
			break
		}
	}
	require.NotNil(t, videoPlugin, "should find video plugin in response")

	mimePrefixes, ok := videoPlugin["supportedMimePrefixes"].([]interface{})
	require.True(t, ok, "video plugin should have supportedMimePrefixes array")
	assert.Contains(t, mimePrefixes, "video/", "video plugin's supportedMimePrefixes should contain 'video/'")
}

// 🆕 2026-06-17：修复前端崩溃 `Cannot read properties of null (reading 'length')`
// alist_encrypt 插件故意 SupportedExtensions() 返回 nil（"处理所有文件"语义）
// handler 层强制兜底为 []，确保前端模板 `p.supportedExtensions.length` 安全访问
// 本测试作为回归屏障——任何 plugin 改回返回 null 都会让此测试失败
func TestHandlePluginsGin_NoPluginHasNullArrayFields(t *testing.T) {
	router := setupPluginsTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/plugins", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// 用 strict JSON 解析，逐个 plugin 验证 SupportedExtensions 字段：
	// - 必须存在
	// - 不能是 nil（必须是非 nil 数组，即使为空）
	// 用 json.Unmarshal + 类型断言会比 raw string 检查更精确
	var response struct {
		Plugins []map[string]json.RawMessage `json:"plugins"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.NotEmpty(t, response.Plugins, "should have at least one plugin")

	for i, p := range response.Plugins {
		name := string(p["name"])
		// SupportedExtensions: 必须是 JSON array，不能是 null
		rawExt, hasExt := p["supportedExtensions"]
		require.True(t, hasExt, "plugin at index %d (%s) missing 'supportedExtensions'", i, name)
		assert.NotEqual(t, "null", string(rawExt),
			"plugin %q's supportedExtensions is JSON null — 前端模板 .length 会崩溃! "+
				"应改为 []string{} (handler 兜底 / plugin.SupportedExtensions() 改返回 [])", name)
		// 解析为数组类型
		var exts []string
		require.NoError(t, json.Unmarshal(rawExt, &exts),
			"plugin %q's supportedExtensions must be a JSON array, not %s", name, string(rawExt))

		// SupportedMimePrefixes: 同样
		rawMime, hasMime := p["supportedMimePrefixes"]
		require.True(t, hasMime, "plugin at index %d (%s) missing 'supportedMimePrefixes'", i, name)
		assert.NotEqual(t, "null", string(rawMime),
			"plugin %q's supportedMimePrefixes is JSON null — 前端模板 .length 会崩溃!", name)
		var mimes []string
		require.NoError(t, json.Unmarshal(rawMime, &mimes),
			"plugin %q's supportedMimePrefixes must be a JSON array", name)
	}
}
