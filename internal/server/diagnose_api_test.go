// internal/server/diagnose_api_test.go
//
// 🆕 2026-06-15：/api/diagnose 单元测试
//
// 覆盖：
//   - buildDiagnoseInfo 字段完整性（核心字段必填）
//   - aggregateStatus 聚合规则（heartbeat / fs / android deps）
//   - diagnoseServingDir 各种边界（空 / 不存在 / 可写 / 不可写）
//   - scanInt / scanLastInt 辅助函数
//
// 用 build tag 隔离：mock_generator_test.go 中依赖已删除的 minimalMP4 等
// 函数，pre-existing broken test 阻断本包 go test。加 `runtime` tag 才跑本测试。
// 跑法：go test -tags runtime ./internal/server/

//go:build runtime
// +build runtime

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newTestServerForDiagnose(t *testing.T) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &Server{
		instanceID: "test-instance-diag-001",
		version:    "0.0.0-diag-test",
		servingDir: t.TempDir(), // 自动清理的临时目录
	}
	s.runtimeInfo = RuntimeInfo{
		PID:        12345,
		Version:    s.version,
		InstanceID: s.instanceID,
		ServingDir: s.servingDir,
		Port:       2025,
		StartedAt:  time.Now().UnixMilli(),
		Mobile:     true,
		ConfigPath: filepath.Join(s.servingDir, "config.json"),
	}
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().UnixMilli())
	return s
}

func TestHandleDiagnoseGin_Returns200JSON(t *testing.T) {
	s := newTestServerForDiagnose(t)
	r := gin.New()
	r.GET("/api/diagnose", s.handleDiagnoseGin)

	req := httptest.NewRequest(http.MethodGet, "/api/diagnose", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("expected JSON content-type, got %q", got)
	}
	var info DiagnoseInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode failed: %v, body=%s", err, w.Body.String())
	}
	if info.Status != "ok" {
		t.Errorf("expected status=ok, got %q (warnings=%v errors=%v)", info.Status, info.Warnings, info.Errors)
	}
	if info.Backend == nil {
		t.Fatal("backend field is nil")
	}
	if info.Backend.PID != 12345 {
		t.Errorf("expected backend.PID=12345, got %d", info.Backend.PID)
	}
	if !info.Backend.HeartbeatOK {
		t.Error("expected backend.HeartbeatOK=true (just initialized)")
	}
	if info.GOOS != runtime.GOOS {
		t.Errorf("expected goos=%q, got %q", runtime.GOOS, info.GOOS)
	}
	if info.GoVersion == "" {
		t.Error("go_version should be non-empty")
	}
	if info.Timestamp == "" {
		t.Error("timestamp should be non-empty")
	}
	if !info.Filesystem.Exists {
		t.Error("expected filesystem.exists=true (t.TempDir created)")
	}
	if !info.Filesystem.Readable {
		t.Error("expected filesystem.readable=true (t.TempDir readable)")
	}
	if !info.Filesystem.Writable {
		t.Error("expected filesystem.writable=true (t.TempDir writable)")
	}
	// 临时目录不在 dev preview → process_tree scope = production
	if info.ProcessTree.Scope != "production" {
		t.Errorf("expected process_tree.scope=production (no ENCV_DEV_PREVIEW), got %q", info.ProcessTree.Scope)
	}
}

func TestHandleDiagnoseGin_HeartbeatStaleReportsOffline(t *testing.T) {
	s := newTestServerForDiagnose(t)
	// 模拟心跳 31s 前
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().Add(-31*time.Second).UnixMilli())

	r := gin.New()
	r.GET("/api/diagnose", s.handleDiagnoseGin)
	req := httptest.NewRequest(http.MethodGet, "/api/diagnose", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var info DiagnoseInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if info.Status != "offline" {
		t.Errorf("expected status=offline (stale heartbeat), got %q", info.Status)
	}
}

func TestHandleDiagnoseGin_HeartbeatZeroReportsOffline(t *testing.T) {
	s := newTestServerForDiagnose(t)
	// 模拟从未启动过 heartbeat loop
	atomic.StoreInt64(&s.lastHeartbeatMs, 0)

	r := gin.New()
	r.GET("/api/diagnose", s.handleDiagnoseGin)
	req := httptest.NewRequest(http.MethodGet, "/api/diagnose", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var info DiagnoseInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if info.Status != "offline" {
		t.Errorf("expected status=offline (no heartbeat yet), got %q", info.Status)
	}
}

func TestHandleDiagnoseGin_DevPreviewScopesProcessTree(t *testing.T) {
	s := newTestServerForDiagnose(t)
	// 模拟 dev preview
	t.Setenv("ENCV_DEV_PREVIEW", "1")

	r := gin.New()
	r.GET("/api/diagnose", s.handleDiagnoseGin)
	req := httptest.NewRequest(http.MethodGet, "/api/diagnose", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var info DiagnoseInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if info.ProcessTree.Scope != "dev" {
		t.Errorf("expected process_tree.scope=dev (ENCV_DEV_PREVIEW=1), got %q", info.ProcessTree.Scope)
	}
	// 探测了 air/vite/pm2/preview_gateway，detected=true
	if !info.ProcessTree.Air.Detected {
		t.Error("expected air.detected=true in dev mode")
	}
	if !info.ProcessTree.Vite.Detected {
		t.Error("expected vite.detected=true in dev mode")
	}
	if !info.ProcessTree.PM2.Detected {
		t.Error("expected pm2.detected=true in dev mode")
	}
	if !info.ProcessTree.PreviewGateway.Detected {
		t.Error("expected preview_gateway.detected=true in dev mode")
	}
}

func TestDiagnoseServingDir_Empty(t *testing.T) {
	fs := diagnoseServingDir("")
	if fs.StatError == "" {
		t.Error("expected stat_error for empty dir")
	}
	if fs.Exists || fs.Readable || fs.Writable {
		t.Error("expected all false for empty dir")
	}
}

func TestDiagnoseServingDir_Nonexistent(t *testing.T) {
	fs := diagnoseServingDir("/nonexistent/path/that/should/never/exist/123456")
	if fs.StatError == "" {
		t.Error("expected stat_error for nonexistent dir")
	}
	if fs.Exists {
		t.Error("expected exists=false for nonexistent dir")
	}
}

func TestDiagnoseServingDir_WritableTempDir(t *testing.T) {
	dir := t.TempDir()
	fs := diagnoseServingDir(dir)
	if fs.StatError != "" {
		t.Errorf("expected no stat_error, got %q", fs.StatError)
	}
	if !fs.Exists {
		t.Error("expected exists=true")
	}
	if !fs.Readable {
		t.Error("expected readable=true")
	}
	if !fs.Writable {
		t.Error("expected writable=true (t.TempDir is writable)")
	}
}

func TestDiagnoseServingDir_FileInsteadOfDir(t *testing.T) {
	// 路径指向文件而不是目录
	dir := t.TempDir()
	filePath := filepath.Join(dir, "regular-file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	fs := diagnoseServingDir(filePath)
	// 文件存在 → exists=true; 不是目录 → readable 反映 IsDir()=false
	// （IsDir()=false 时不触发可写性测试）
	if !fs.Exists {
		t.Error("expected exists=true for existing file")
	}
	if fs.Readable {
		t.Error("expected readable=false (path is a file, not dir)")
	}
	if fs.Writable {
		t.Error("expected writable=false (no write test for files)")
	}
}

func TestAggregateStatus_Offline(t *testing.T) {
	info := DiagnoseInfo{
		Backend: &RuntimeInfo{HeartbeatOK: false},
		Filesystem: DiagnoseFS{Exists: true, Readable: true},
	}
	if got := aggregateStatus(info); got != "offline" {
		t.Errorf("expected offline, got %q", got)
	}
}

func TestAggregateStatus_Degraded_NoReadable(t *testing.T) {
	info := DiagnoseInfo{
		Backend: &RuntimeInfo{HeartbeatOK: true},
		Filesystem: DiagnoseFS{Exists: false, Readable: false},
	}
	if got := aggregateStatus(info); got != "degraded" {
		t.Errorf("expected degraded (fs not readable), got %q", got)
	}
}

func TestAggregateStatus_Degraded_AndroidMissingFFmpeg(t *testing.T) {
	info := DiagnoseInfo{
		GOOS:       "android",
		Backend:    &RuntimeInfo{HeartbeatOK: true},
		Filesystem: DiagnoseFS{Exists: true, Readable: true},
		Dependencies: DiagnoseDeps{
			FFmpeg:  DiagnoseDep{Available: false},
			FFprobe: DiagnoseDep{Available: true},
		},
	}
	if got := aggregateStatus(info); got != "degraded" {
		t.Errorf("expected degraded (android missing ffmpeg), got %q", got)
	}
}

func TestAggregateStatus_OK_NonAndroidMissingFFmpegIsFine(t *testing.T) {
	info := DiagnoseInfo{
		GOOS:       "linux",
		Backend:    &RuntimeInfo{HeartbeatOK: true},
		Filesystem: DiagnoseFS{Exists: true, Readable: true},
		Dependencies: DiagnoseDeps{
			FFmpeg:  DiagnoseDep{Available: false},
			FFprobe: DiagnoseDep{Available: false},
		},
	}
	// 非 Android 平台：ffmpeg 缺失不算 degraded
	if got := aggregateStatus(info); got != "ok" {
		t.Errorf("expected ok (non-android, ffmpeg missing ok), got %q", got)
	}
}

func TestAggregateStatus_OK_AllChecksPass(t *testing.T) {
	info := DiagnoseInfo{
		Backend:    &RuntimeInfo{HeartbeatOK: true},
		Filesystem: DiagnoseFS{Exists: true, Readable: true},
		Dependencies: DiagnoseDeps{
			FFmpeg:  DiagnoseDep{Available: true},
			FFprobe: DiagnoseDep{Available: true},
		},
	}
	if got := aggregateStatus(info); got != "ok" {
		t.Errorf("expected ok, got %q", got)
	}
}

func TestScanInt_Basic(t *testing.T) {
	var got int
	n, _ := scanInt("12345", &got)
	if got != 12345 {
		t.Errorf("expected 12345, got %d", got)
	}
	if n != 1 {
		t.Errorf("expected n=1 (started=true), got %d", n)
	}
}

func TestScanInt_Empty(t *testing.T) {
	var got int = 999
	_, _ = scanInt("", &got)
	if got != 0 {
		t.Errorf("expected 0 for empty, got %d", got)
	}
}

func TestScanInt_LeadingNonDigit(t *testing.T) {
	var got int
	_, _ = scanInt("abc123", &got)
	if got != 0 {
		t.Errorf("expected 0 for non-digit prefix, got %d", got)
	}
}

func TestScanLastInt_FindsLastField(t *testing.T) {
	// 模拟 netstat 输出: "TCP    0.0.0.0:16666    0.0.0.0:0    LISTENING    12345"
	fields := []string{"TCP", "0.0.0.0:16666", "0.0.0.0:0", "LISTENING", "12345"}
	pid, _ := scanLastInt(fields)
	if pid != 12345 {
		t.Errorf("expected 12345, got %d", pid)
	}
}

func TestDiagnoseInfo_JSONSchema_HasAllFields(t *testing.T) {
	// 验证 JSON schema 锁定（防止后续 PR 误删字段）
	s := newTestServerForDiagnose(t)
	info := s.buildDiagnoseInfo()
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	required := []string{
		"status", "timestamp", "go_version", "goos", "goarch",
		"backend", "dependencies", "environment", "filesystem",
		"process_tree", "warnings", "errors",
	}
	for _, field := range required {
		if _, ok := generic[field]; !ok {
			t.Errorf("DiagnoseInfo JSON missing required field %q", field)
		}
	}
}

func TestWriteActualPortFile_AtomicWrite(t *testing.T) {
	// 验证端口公告文件能正确写出
	dir := t.TempDir()
	path := filepath.Join(dir, "encv-go.port")
	if err := writeActualPortFile(path, 2026, "instance-abc-123"); err != nil {
		t.Fatalf("writeActualPortFile failed: %v", err)
	}
	// 文件存在
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file exists: %v", err)
	}
	// 内容正确（2 行：port + instanceID）
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(data))
	}
	if lines[0] != "2026" {
		t.Errorf("expected port=2026, got %q", lines[0])
	}
	if lines[1] != "instance-abc-123" {
		t.Errorf("expected instanceID=instance-abc-123, got %q", lines[1])
	}
}

func TestWriteActualPortFile_OverwritesExisting(t *testing.T) {
	// 验证重新启动会覆盖旧文件（不是 append）
	dir := t.TempDir()
	path := filepath.Join(dir, "encv-go.port")
	if err := writeActualPortFile(path, 2025, "old-instance"); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if err := writeActualPortFile(path, 2026, "new-instance"); err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if lines[0] != "2026" || lines[1] != "new-instance" {
		t.Errorf("expected overwrite, got %q", string(data))
	}
}
