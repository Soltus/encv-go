// internal/server/runtime_api_test.go
//
// 🆕 2026-06-14：跨进程 IPC 重构 — RuntimeInfo + /health 单元测试
//
// 覆盖：
//   - snapshotRuntimeInfo 正确填充所有字段
//   - /api/runtime 路由返回有效 JSON
//   - heartbeat_ok 在新 → 陈旧 时正确切换
//   - 并演读写 lastHeartbeatMs 无 race（go test -race）
//
// 用 build tag 隔离：mock_generator_test.go 中依赖已删除的 minimalMP4 等
// 函数，pre-existing broken test 阻断本包 go test。加 `runtime` tag 才跑本测试。
// 跑法：go test -tags runtime ./internal/server/

//go:build runtime
// +build runtime

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newTestServerForRuntime(t *testing.T) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &Server{
		instanceID: "test-instance-001",
		version:    "0.0.0-test",
		servingDir: "/tmp/encv-test-runtime",
	}
	// 填充 runtimeInfo
	s.runtimeInfo = RuntimeInfo{
		PID:        12345,
		Version:    s.version,
		InstanceID: s.instanceID,
		ServingDir: s.servingDir,
		Port:       2025,
		StartedAt:  time.Now().UnixMilli(),
		Mobile:     true,
		ConfigPath: "/tmp/encv-test-runtime/config.json",
	}
	// 初始化心跳
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().UnixMilli())
	return s
}

func TestHandleRuntimeAPI_ReturnsValidJSON(t *testing.T) {
	s := newTestServerForRuntime(t)
	r := gin.New()
	r.GET("/api/runtime", s.handleRuntimeAPI)

	req := httptest.NewRequest(http.MethodGet, "/api/runtime", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
	var got RuntimeInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode failed: %v, body=%s", err, w.Body.String())
	}
	if got.PID != 12345 {
		t.Errorf("expected PID=12345, got %d", got.PID)
	}
	if got.Version != "0.0.0-test" {
		t.Errorf("expected Version=0.0.0-test, got %q", got.Version)
	}
	if got.InstanceID != "test-instance-001" {
		t.Errorf("expected InstanceID=test-instance-001, got %q", got.InstanceID)
	}
	if got.ServingDir != "/tmp/encv-test-runtime" {
		t.Errorf("expected ServingDir=/tmp/encv-test-runtime, got %q", got.ServingDir)
	}
	if got.Port != 2025 {
		t.Errorf("expected Port=2025, got %d", got.Port)
	}
	if got.Mobile != true {
		t.Errorf("expected Mobile=true, got %v", got.Mobile)
	}
	if !got.HeartbeatOK {
		t.Errorf("expected HeartbeatOK=true (just initialized), got %v", got.HeartbeatOK)
	}
	if got.UptimeMs < 0 {
		t.Errorf("expected UptimeMs>=0, got %d", got.UptimeMs)
	}
}

func TestHandleRuntimeAPI_HeartbeatStaleReportsFalse(t *testing.T) {
	s := newTestServerForRuntime(t)
	// 模拟心跳 31s 前（超过 HeartbeatStaleThreshold=30s）
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().Add(-31*time.Second).UnixMilli())

	r := gin.New()
	r.GET("/api/runtime", s.handleRuntimeAPI)

	req := httptest.NewRequest(http.MethodGet, "/api/runtime", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got RuntimeInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if got.HeartbeatOK {
		t.Errorf("expected HeartbeatOK=false (stale 31s), got true")
	}
	if got.HeartbeatAgeMs < 31000 {
		t.Errorf("expected HeartbeatAgeMs>=31000, got %d", got.HeartbeatAgeMs)
	}
}

func TestHandleRuntimeAPI_NoHeartbeatYet(t *testing.T) {
	s := newTestServerForRuntime(t)
	// lastHeartbeatMs = 0（启动后还没 tick）
	atomic.StoreInt64(&s.lastHeartbeatMs, 0)

	r := gin.New()
	r.GET("/api/runtime", s.handleRuntimeAPI)

	req := httptest.NewRequest(http.MethodGet, "/api/runtime", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got RuntimeInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if got.HeartbeatOK {
		t.Errorf("expected HeartbeatOK=false (no heartbeat yet), got true")
	}
	if got.HeartbeatAgeMs != -1 {
		t.Errorf("expected HeartbeatAgeMs=-1 (sentinel), got %d", got.HeartbeatAgeMs)
	}
}

func TestHandleHealthGin_JSON(t *testing.T) {
	s := newTestServerForRuntime(t)
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().UnixMilli())

	r := gin.New()
	r.GET("/health", s.handleHealthGin)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type=application/json, got %q", got)
	}
	var body struct {
		Status         string `json:"status"`
		HeartbeatAgeMs int64  `json:"heartbeat_age_ms"`
		HeartbeatOK    bool   `json:"heartbeat_ok"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failed: %v, body=%s", err, w.Body.String())
	}
	if body.Status != "ok" {
		t.Errorf("expected status=ok, got %q", body.Status)
	}
	if !body.HeartbeatOK {
		t.Errorf("expected heartbeat_ok=true (just initialized), got false")
	}
}

func TestHandleHealthGin_HeartbeatStaleJSON(t *testing.T) {
	s := newTestServerForRuntime(t)
	// 模拟心跳 35s 前
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().Add(-35*time.Second).UnixMilli())

	r := gin.New()
	r.GET("/health", s.handleHealthGin)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		Status         string `json:"status"`
		HeartbeatAgeMs int64  `json:"heartbeat_age_ms"`
		HeartbeatOK    bool   `json:"heartbeat_ok"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if body.HeartbeatOK {
		t.Error("expected heartbeat_ok=false (stale 35s)")
	}
	if body.HeartbeatAgeMs < 35000 {
		t.Errorf("expected heartbeat_age_ms>=35000, got %d", body.HeartbeatAgeMs)
	}
}

func TestTouchHeartbeat_UpdatesField(t *testing.T) {
	s := newTestServerForRuntime(t)
	// 模拟心跳 40s 前
	stale := time.Now().Add(-40 * time.Second).UnixMilli()
	atomic.StoreInt64(&s.lastHeartbeatMs, stale)

	// Touch
	s.TouchHeartbeat()

	now := time.Now().UnixMilli()
	got := atomic.LoadInt64(&s.lastHeartbeatMs)
	if got < now-100 || got > now+100 {
		t.Errorf("TouchHeartbeat did not update lastHeartbeatMs: got %d, expected near %d", got, now)
	}
}

func TestLastHeartbeatMs_ConcurrentReadWrite(t *testing.T) {
	// 这个测试主要是想跑 -race，验证 atomic 操作无 race
	s := newTestServerForRuntime(t)
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().UnixMilli())

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 1 个写者
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				s.TouchHeartbeat()
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// 8 个读者
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = atomic.LoadInt64(&s.lastHeartbeatMs)
			}
		}()
	}

	// 等读者完成
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(stop)
	}()
	wg.Wait()
}

func TestStartHeartbeatLoopInMemory_UpdatesField(t *testing.T) {
	s := newTestServerForRuntime(t)
	atomic.StoreInt64(&s.lastHeartbeatMs, 0)

	// 用一个 cancel ctx 短跑
	ctx, cancel := context.WithCancel(context.Background())
	s.startHeartbeatLoopInMemory(ctx)

	// 启动首次立即更新
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&s.lastHeartbeatMs) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&s.lastHeartbeatMs) == 0 {
		t.Error("startHeartbeatLoopInMemory should immediately write lastHeartbeatMs")
	}

	// 2s 后再读，应更新过
	prev := atomic.LoadInt64(&s.lastHeartbeatMs)
	time.Sleep(2500 * time.Millisecond)
	cur := atomic.LoadInt64(&s.lastHeartbeatMs)
	if cur <= prev {
		t.Errorf("expected lastHeartbeatMs to advance after 2.5s, prev=%d cur=%d", prev, cur)
	}

	// cancel 后应停止（goroutine 退出，但无法直接验证 — 至少 2.5s 后字段没继续涨）
	cancel()
	time.Sleep(200 * time.Millisecond)
	final := atomic.LoadInt64(&s.lastHeartbeatMs)
	time.Sleep(2500 * time.Millisecond)
	if atomic.LoadInt64(&s.lastHeartbeatMs) != final {
		t.Error("after cancel, lastHeartbeatMs should not advance")
	}
}
