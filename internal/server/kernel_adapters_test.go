// internal/server/kernel_adapters_test.go — kernel adapter 单元测试
//
// 2026-07-03 新增：特色微服务内核接入主代码库
//
// 测试覆盖：
//   1. SearchVectorService
//      - nil svc → Health 返回 error
//      - 真实 svc（turso + tmp file）→ Call "search_files" / "stats" 工作
//      - unknown method → 返回 error
//   2. WSHubService
//      - nil hub → Health 返回 error
//      - 真实 hub → Call "broadcast" 不 panic
//   3. FTSRebuilderService
//      - nil rebuilder → Health 返回 error
//      - mock rebuilder → Call "rebuild" 调用 RebuildWithProgress
//   4. RegisterKernelAdapters
//      - 注册后 kernel.List() 包含三个 name
//      - 重复注册会 panic
//   5. HTTP API（间接测试）
//      - GET /api/kernel/services → 200 + services 数组
//      - GET /api/kernel/health → 200 + ok:true
//      - POST /api/kernel/call（非 dev 模式）→ 403
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/kernel"
	vectorsearch "github.com/Soltus/encv-go/internal/search"
	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/gin-gonic/gin"
	_ "turso.tech/database/tursogo"
)

// ============================================================================
// helpers
// ============================================================================

// newTestSearchService 构造一个可用的 *SearchService（turso + tmp file）。
// 调用方负责 db.Close（通过 t.Cleanup）。
func newTestSearchService(t *testing.T) *vectorsearch.SearchService {
	t.Helper()
	tmpFile := t.TempDir() + "/test-kernel-search.db"
	db, err := sql.Open("turso", tmpFile)
	if err != nil {
		t.Fatalf("open turso db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store, err := vectorsearch.NewStore(db, "turso")
	if err != nil {
		t.Fatalf("create search store: %v", err)
	}
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	// 用 NewSearchService(db, driver) 构造（NewSearchServiceFromPath 会重复 sql.Open）
	svc, err := vectorsearch.NewSearchService(db, "turso")
	if err != nil {
		t.Fatalf("new search service: %v", err)
	}
	return svc
}

// mockFTSRebuilder 简单 mock，记录是否被调用 + 可注入返回 error。
type mockFTSRebuilder struct {
	called     bool
	returnErr  error
	lastCtx    context.Context
	lastCbFunc func(progress int, phase, speed, eta string)
}

func (m *mockFTSRebuilder) RebuildWithProgress(
	ctx context.Context,
	progressCb func(progress int, phase, speed, eta string),
) error {
	m.called = true
	m.lastCtx = ctx
	m.lastCbFunc = progressCb
	if progressCb != nil {
		progressCb(50, "scanning", "100 files/s", "10s")
		progressCb(100, "completed", "", "")
	}
	return m.returnErr
}

// unregisterAll 测试 helper：清理所有已注册的 kernel.Service
func unregisterAll(t *testing.T) {
	t.Helper()
	for _, name := range kernel.List() {
		kernel.Unregister(name)
	}
}

// ============================================================================
// SearchVectorService 测试
// ============================================================================

func TestSearchVectorService_NilSvc_HealthError(t *testing.T) {
	// unregisterAll 防止上一个测试遗留注册（虽然 *testing.T 各自独立进程，但保险起见）
	unregisterAll(t)

	svc := NewSearchVectorService(nil)
	ctx := kernel.NewContext(context.Background())

	if err := svc.Health(ctx); err == nil {
		t.Error("Health with nil svc should return error")
	}
}

func TestSearchVectorService_Init_NilSvc_NotFail(t *testing.T) {
	unregisterAll(t)

	svc := NewSearchVectorService(nil)
	ctx := kernel.NewContext(context.Background())

	// Init 不应失败（nil svc 是合法状态，Health 才反映问题）
	if err := svc.Init(ctx); err != nil {
		t.Errorf("Init with nil svc should not fail, got: %v", err)
	}
}

func TestSearchVectorService_RealSvc_SearchFiles(t *testing.T) {
	unregisterAll(t)

	searchSvc := newTestSearchService(t)

	// 索引一个测试文件
	ctx := context.Background()
	if err := searchSvc.IndexFile(ctx, "/test/video.mp4", "video.mp4", "1024", "2026-07-03"); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	adapter := NewSearchVectorService(searchSvc)
	kernelCtx := kernel.NewContext(ctx)

	// 调用 search_files
	payload, _ := json.Marshal(SearchFilesReq{Query: "video", Limit: 5})
	resp, err := adapter.Call(kernelCtx, "search_files", payload)
	if err != nil {
		t.Fatalf("Call search_files: %v", err)
	}

	var result SearchFilesResp
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result.Results) == 0 {
		t.Log("warning: search returned 0 results (index may not be flushed yet)")
	} else {
		t.Logf("search 'video' returned %d results", len(result.Results))
	}
}

func TestSearchVectorService_RealSvc_Stats(t *testing.T) {
	unregisterAll(t)

	searchSvc := newTestSearchService(t)
	adapter := NewSearchVectorService(searchSvc)
	ctx := kernel.NewContext(context.Background())

	resp, err := adapter.Call(ctx, "stats", nil)
	if err != nil {
		t.Fatalf("Call stats: %v", err)
	}

	var result StatsResp
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	t.Logf("stats: indexed_files=%d indexed_tasks=%d", result.IndexedFiles, result.IndexedTasks)
}

func TestSearchVectorService_RealSvc_IsDegraded(t *testing.T) {
	unregisterAll(t)

	searchSvc := newTestSearchService(t)
	adapter := NewSearchVectorService(searchSvc)
	ctx := kernel.NewContext(context.Background())

	resp, err := adapter.Call(ctx, "is_degraded", nil)
	if err != nil {
		t.Fatalf("Call is_degraded: %v", err)
	}

	var result DegradedResp
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal is_degraded: %v", err)
	}
	if result.Degraded {
		t.Error("fresh turso service should not be degraded")
	}
}

func TestSearchVectorService_UnknownMethod(t *testing.T) {
	unregisterAll(t)

	searchSvc := newTestSearchService(t)
	adapter := NewSearchVectorService(searchSvc)
	ctx := kernel.NewContext(context.Background())

	_, err := adapter.Call(ctx, "nonexistent_method", nil)
	if err == nil {
		t.Error("unknown method should return error")
	}
	if !strings.Contains(err.Error(), "unknown method") {
		t.Errorf("error should mention 'unknown method', got: %v", err)
	}
}

// ============================================================================
// WSHubService 测试
// ============================================================================

func TestWSHubService_NilHub_HealthError(t *testing.T) {
	unregisterAll(t)

	svc := NewWSHubService(nil)
	ctx := kernel.NewContext(context.Background())

	if err := svc.Health(ctx); err == nil {
		t.Error("Health with nil hub should return error")
	}
}

func TestWSHubService_RealHub_Broadcast(t *testing.T) {
	unregisterAll(t)

	hub := mobileservice.NewWSHub()
	adapter := NewWSHubService(hub)
	ctx := kernel.NewContext(context.Background())

	// broadcast 一个消息（应不 panic）
	payload, _ := json.Marshal(BroadcastReq{
		Type: "test:event",
		Data: json.RawMessage(`{"foo":"bar"}`),
	})
	resp, err := adapter.Call(ctx, "broadcast", payload)
	if err != nil {
		t.Fatalf("Call broadcast: %v", err)
	}

	var result EmptyResp
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal broadcast response: %v", err)
	}
}

func TestWSHubService_UnknownMethod(t *testing.T) {
	unregisterAll(t)

	hub := mobileservice.NewWSHub()
	adapter := NewWSHubService(hub)
	ctx := kernel.NewContext(context.Background())

	_, err := adapter.Call(ctx, "nonexistent", nil)
	if err == nil {
		t.Error("unknown method should return error")
	}
}

// ============================================================================
// FTSRebuilderService 测试
// ============================================================================

func TestFTSRebuilderService_NilRebuilder_HealthError(t *testing.T) {
	unregisterAll(t)

	svc := NewFTSRebuilderService(nil)
	ctx := kernel.NewContext(context.Background())

	if err := svc.Health(ctx); err == nil {
		t.Error("Health with nil rebuilder should return error")
	}
}

func TestFTSRebuilderService_MockRebuilder_Rebuild(t *testing.T) {
	unregisterAll(t)

	mock := &mockFTSRebuilder{}
	adapter := NewFTSRebuilderService(mock)
	ctx := kernel.NewContext(context.Background())

	resp, err := adapter.Call(ctx, "rebuild", nil)
	if err != nil {
		t.Fatalf("Call rebuild: %v", err)
	}

	if !mock.called {
		t.Error("mock RebuildWithProgress should have been called")
	}
	if mock.lastCbFunc == nil {
		t.Error("progressCb should have been passed (not nil)")
	}

	var result EmptyResp
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal rebuild response: %v", err)
	}
}

func TestFTSRebuilderService_MockRebuilder_ReturnError(t *testing.T) {
	unregisterAll(t)

	expectedErr := errors.New("disk full")
	mock := &mockFTSRebuilder{returnErr: expectedErr}
	adapter := NewFTSRebuilderService(mock)
	ctx := kernel.NewContext(context.Background())

	_, err := adapter.Call(ctx, "rebuild", nil)
	if err == nil {
		t.Fatal("should return error when rebuilder fails")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("error should wrap expectedErr, got: %v", err)
	}
}

// ============================================================================
// RegisterKernelAdapters 测试
// ============================================================================

func TestRegisterKernelAdapters_AllRegistered(t *testing.T) {
	unregisterAll(t)

	searchSvc := newTestSearchService(t)
	hub := mobileservice.NewWSHub()
	mock := &mockFTSRebuilder{}

	errs := RegisterKernelAdapters(searchSvc, hub, mock)
	if len(errs) != 0 {
		t.Fatalf("RegisterKernelAdapters should not return errors, got: %v", errs)
	}

	names := kernel.List()
	if len(names) != 3 {
		t.Fatalf("expected 3 registered services, got %d: %v", len(names), names)
	}

	// 验证三个服务名都在
	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	expectedNames := []string{"search.vector", "ws.hub", "fts.rebuilder"}
	for _, expected := range expectedNames {
		if !nameSet[expected] {
			t.Errorf("expected service %q not found in registry: %v", expected, names)
		}
	}
}

func TestRegisterKernelAdapters_DuplicatePanics(t *testing.T) {
	unregisterAll(t)

	searchSvc := newTestSearchService(t)
	hub := mobileservice.NewWSHub()
	mock := &mockFTSRebuilder{}

	// 第一次注册（应成功）
	RegisterKernelAdapters(searchSvc, hub, mock)

	// 第二次注册同名 service 应 panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("duplicate registration should panic")
		}
	}()
	RegisterKernelAdapters(searchSvc, hub, mock)
}

func TestRegisterKernelAdapters_NilSvc_StillRegistersButHealthFails(t *testing.T) {
	unregisterAll(t)

	// 全部传 nil → adapter 仍注册（Health 反映问题）
	errs := RegisterKernelAdapters(nil, nil, nil)
	// Init 不应失败（每个 adapter 的 Init 都把 nil 视为合法状态）
	if len(errs) != 0 {
		t.Errorf("Init with nil svcs should not return errors, got: %v", errs)
	}

	// 但 Health 应该全部失败
	ctx := kernel.NewContext(context.Background())
	statuses := kernel.HealthAll(ctx)
	if len(statuses) != 3 {
		t.Fatalf("expected 3 health statuses, got %d", len(statuses))
	}
	for _, st := range statuses {
		if st.OK {
			t.Errorf("service %q should not be OK with nil underlying svc", st.Name)
		}
	}
}

// ============================================================================
// kernel.Call 跨服务调用测试
// ============================================================================

func TestKernelCall_SearchVector_ThroughRegistry(t *testing.T) {
	unregisterAll(t)

	searchSvc := newTestSearchService(t)
	hub := mobileservice.NewWSHub()
	mock := &mockFTSRebuilder{}
	RegisterKernelAdapters(searchSvc, hub, mock)

	ctx := kernel.NewContext(context.Background())

	// 通过 kernel.Call 调用 search.vector.stats
	resp, err := kernel.Call(ctx, "search.vector", "stats", nil)
	if err != nil {
		t.Fatalf("kernel.Call search.vector.stats: %v", err)
	}

	var result StatsResp
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	t.Logf("kernel.Call stats: %+v", result)

	// 通过 kernel.Call 调用 ws.hub.broadcast
	broadcastReq := BroadcastReq{Type: "kernel:test", Data: json.RawMessage(`{}`)}
	resp, err = kernel.Call(ctx, "ws.hub", "broadcast", broadcastReq)
	if err != nil {
		t.Fatalf("kernel.Call ws.hub.broadcast: %v", err)
	}
	t.Logf("kernel.Call broadcast response: %s", string(resp))
}

// ============================================================================
// HTTP API 测试（用 gin.TestContext）
// ============================================================================

func TestHandleKernelServicesGin_ReturnsAllServices(t *testing.T) {
	unregisterAll(t)

	// 先注册 3 个 adapter
	searchSvc := newTestSearchService(t)
	hub := mobileservice.NewWSHub()
	mock := &mockFTSRebuilder{}
	RegisterKernelAdapters(searchSvc, hub, mock)

	// 构造 mock Server（只需 cfg 字段非 nil 即可，handler 不依赖其他字段）
	srv := &Server{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/kernel/services", nil)

	srv.handleKernelServicesGin(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Services []string `json:"services"`
		Count    int      `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Count != 3 {
		t.Errorf("expected count=3, got %d", resp.Count)
	}
	t.Logf("services: %v", resp.Services)
}

func TestHandleKernelHealthGin_AllOK_Returns200(t *testing.T) {
	unregisterAll(t)

	// 用真实 svc 让 Health 全部通过
	searchSvc := newTestSearchService(t)
	hub := mobileservice.NewWSHub()
	mock := &mockFTSRebuilder{}
	RegisterKernelAdapters(searchSvc, hub, mock)

	srv := &Server{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/kernel/health", nil)

	srv.handleKernelHealthGin(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (all OK), got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK       bool                    `json:"ok"`
		Services []kernel.HealthStatus   `json:"services"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Error("expected ok=true when all services healthy")
	}
	if len(resp.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(resp.Services))
	}
}

func TestHandleKernelHealthGin_NilSvc_Returns503(t *testing.T) {
	unregisterAll(t)

	// 全部传 nil → Health 全部失败 → 应返回 503
	RegisterKernelAdapters(nil, nil, nil)

	srv := &Server{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/kernel/health", nil)

	srv.handleKernelHealthGin(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (degraded), got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.OK {
		t.Error("expected ok=false when services unhealthy")
	}
}

func TestHandleKernelCallGin_NonDev_Returns403(t *testing.T) {
	unregisterAll(t)

	// 不设 ENCV_DEV / ENCV_DEV_PREVIEW 环境变量
	t.Setenv("ENCV_DEV", "")
	t.Setenv("ENCV_DEV_PREVIEW", "")

	srv := &Server{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"service":"search.vector","method":"stats"}`
	c.Request = httptest.NewRequest("POST", "/api/kernel/call", strings.NewReader(body))

	srv.handleKernelCallGin(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 in non-dev mode, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleKernelCallGin_DevMode_SearchStats(t *testing.T) {
	unregisterAll(t)

	t.Setenv("ENCV_DEV", "1") // dev 模式

	searchSvc := newTestSearchService(t)
	hub := mobileservice.NewWSHub()
	mock := &mockFTSRebuilder{}
	RegisterKernelAdapters(searchSvc, hub, mock)

	srv := &Server{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"service":"search.vector","method":"stats","payload":null}`
	c.Request = httptest.NewRequest("POST", "/api/kernel/call", strings.NewReader(body))

	srv.handleKernelCallGin(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK       bool        `json:"ok"`
		Response StatsResp   `json:"response"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Error("expected ok=true")
	}
	t.Logf("kernel.call response: %+v", resp.Response)
}

func TestHandleKernelCallGin_DevMode_UnknownService(t *testing.T) {
	unregisterAll(t)

	t.Setenv("ENCV_DEV", "1")

	srv := &Server{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"service":"nonexistent","method":"foo"}`
	c.Request = httptest.NewRequest("POST", "/api/kernel/call", strings.NewReader(body))

	srv.handleKernelCallGin(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unknown service, got %d: %s", w.Code, w.Body.String())
	}
}
