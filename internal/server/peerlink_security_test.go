package server

// peerlink_security_test.go —— P5 安全与降级收口（spec desktop-web-android-pairing）
//
// 锁定红线：
//  5.1 未配对一律 **401**（除有意开放的 hello / ticket / pair / pairing-status）
//  5.2 psk / token / 信任态 **零落盘**（结构性锁：peerlink 相关文件不得有任何写盘调用）
//  5.4 **R14** 访问日志脱敏：查询串里的 token / 查询词 / 远端路径**不得**进日志

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

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// TestPeerlink_UnpairedAll401 —— Task 5.1：全端点未配对 401
func TestPeerlink_UnpairedAll401(t *testing.T) {
	r, _ := newPeerlinkRouter()

	// 有意开放的端点（未配对也必须可用，否则无法完成首次配对）
	open := []struct{ method, path string }{
		{"GET", "/api/peerlink/hello"},
		{"POST", "/api/peerlink/ticket"},
		{"POST", "/api/peerlink/pair"},
		{"GET", "/api/peerlink/pairing/status?pairingId=whatever"},
	}

	// 必须鉴权的端点（无 token / 非运维身份 → 401）
	protected := []struct{ method, path string }{
		{"GET", "/api/peerlink/peers"},
		{"GET", "/api/peerlink/search?peerId=p&q=x"},
		{"GET", "/api/peerlink/file?peerId=p&path=/x"},
		{"POST", "/api/peerlink/agent/invoke"},
		{"GET", "/api/peerlink/agent/pending"},
		{"POST", "/api/peerlink/agent/approve"},
		{"GET", "/api/peerlink/agent/trust"},
		{"DELETE", "/api/peerlink/agent/trust?peerId=p"},
		{"GET", "/api/peerlink/agent/audit"},
		{"POST", "/api/peerlink/edge/pair"},
		{"GET", "/api/peerlink/edge/status"},
		{"POST", "/api/peerlink/edge/stop"},
		{"POST", "/api/peerlink/ping"},
		{"POST", "/api/peerlink/unpair"},
	}

	for _, c := range protected {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s 未鉴权应 401, got %d", c.method, c.path, rec.Code)
		}
	}

	for _, c := range open {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, strings.NewReader(`{}`)))
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s 是「有意开放」端点，不应 401", c.method, c.path)
		}
	}
}

// TestPeerlink_NoSecretPersisted —— Task 5.2：psk/token/信任态零落盘（结构性锁）
func TestPeerlink_NoSecretPersisted(t *testing.T) {
	// peerlink 包 + server 侧 peerlink 实现：**不得**出现任何写盘调用
	patterns := []string{
		"os.WriteFile", "os.Create", "ioutil.WriteFile", "os.OpenFile",
		"json.NewEncoder(f)", "gob.NewEncoder",
	}
	dirs := []string{"../peerlink", "."}
	checked := 0
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			t.Fatalf("读取目录失败 %s: %v", d, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") {
				continue
			}
			if !strings.Contains(name, "peerlink") {
				continue
			}
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(d, name))
			if err != nil {
				t.Fatalf("读取失败: %v", err)
			}
			checked++
			for _, p := range patterns {
				if bytes.Contains(b, []byte(p)) {
					t.Errorf("%s 出现写盘调用 %q ⇒ 违反「psk/token/信任态零落盘」", filepath.Join(d, name), p)
				}
			}
		}
	}
	if checked < 3 {
		t.Fatalf("扫描到的 peerlink 源文件过少（%d），锁可能失效", checked)
	}

	// 运行时侧：配对后的对外数据结构**不得**含密钥字段
	hub := peerlink.NewHub("h", "v")
	tk, err := hub.CreateTicket("https://hub.example", 2*time.Minute)
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	psk, _ := peerlink.DecodePSK(tk.PSKHex)
	pr, err := hub.Pair(tk.PairingID, "dev-1", "Pixel", "android", peerlink.Proof(psk, tk.PairingID, "dev-1"))
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if pr == nil || pr.Token == "" {
		t.Fatalf("配对应返回 token（内存态）")
	}
	blob, err := json.Marshal(hub.ListPeers())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, leak := range []string{pr.Token, tk.PSKHex, "secretKey", "encKey"} {
		if strings.Contains(string(blob), leak) {
			t.Errorf("对外列表泄露密钥材料 %q", leak)
		}
	}
}

// TestPeerlink_AccessLogSanitized —— Task 5.4 / R14：访问日志不得记录查询串
func TestPeerlink_AccessLogSanitized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	old := gin.DefaultWriter
	gin.DefaultWriter = &buf
	defer func() { gin.DefaultWriter = old }()

	r := gin.New()
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{Formatter: sanitizedLogFormatter}))
	r.GET("/api/peerlink/search", func(c *gin.Context) { c.Status(200) })
	r.GET("/api/files/search", func(c *gin.Context) { c.Status(200) })

	secretQ := "q=" + "工资单" + "&path=/sdcard/" + "私密" + "/a.pdf"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/peerlink/search?"+secretQ+"&token=SECRET-TOKEN", nil))

	logged := buf.String()
	if strings.Contains(logged, "工资单") || strings.Contains(logged, "私密") || strings.Contains(logged, "SECRET-TOKEN") {
		t.Fatalf("访问日志泄露了查询串: %s", logged)
	}
	if !strings.Contains(logged, "/api/peerlink/search") {
		t.Fatalf("访问日志应仍记录路径本身: %s", logged)
	}

	// 非敏感路径：查询串照常记录（不要过度脱敏影响排障）
	buf.Reset()
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest("GET", "/api/files/search?keyword=hello", nil))
	if !strings.Contains(buf.String(), "keyword=hello") {
		t.Fatalf("非敏感路径不应被脱敏: %s", buf.String())
	}
}
