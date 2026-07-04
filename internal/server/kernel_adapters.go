// internal/server/kernel_adapters.go — 把现有 service 包装为 kernel.Service
//
// 2026-07-03 新增：特色微服务内核接入主代码库（internal/kernel 孤儿包盘活）
//
// 设计原则：
//   - 不改动被包装的 service 包源码（adapter 模式，桥接层在 server 包内）
//   - 不破坏现有直接 method call 路径（新旧两条路径并存，渐进式迁移）
//   - 优先暴露 JSON 友好方法（search / broadcast / rebuild）
//   - TaskManager / MobileService 暂不接入（参数含 *MobileTask 等复杂类型，
//     套 json.RawMessage 会丢失类型安全；它们是 kernel 的调用方而非服务）
//
// 三个 adapter：
//   1. SearchVectorService — 包装 *vectorsearch.SearchService（search_files / search_tasks / index_*）
//   2. WSHubService        — 包装 *service.WSHub（broadcast）
//   3. FTSRebuilderService — 包装 service.FTSRebuilder（rebuild，支持 ctx 透传）
//
// 调用示例：
//
//	// 旧：results, err := s.searchSvc.SearchFiles(ctx, query, limit)
//	// 新：
//	resp, err := kernel.CallTyped[SearchFilesReq, SearchFilesResp](
//	    ctx, "search.vector", "search_files", SearchFilesReq{Query: q, Limit: lim},
//	)
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Soltus/encv-go/internal/kernel"
	vectorsearch "github.com/Soltus/encv-go/internal/search"
	mobileservice "github.com/Soltus/encv-go/internal/service"
)

// ============================================================================
// SearchVectorService — 包装 *vectorsearch.SearchService
// ============================================================================

// SearchVectorService 把向量搜索服务注册为 kernel.Service。
//
// 支持的 method：
//   - "search_files"  : payload = SearchFilesReq, 返回 SearchFilesResp
//   - "search_tasks"  : payload = SearchTasksReq, 返回 SearchTasksResp
//   - "index_file"    : payload = IndexFileReq,   返回 EmptyResp
//   - "index_task"    : payload = IndexTaskReq,   返回 EmptyResp
//   - "delete_file"   : payload = DeleteReq,      返回 EmptyResp
//   - "delete_task"   : payload = DeleteReq,      返回 EmptyResp
//   - "stats"         : payload = nil,            返回 StatsResp
//   - "is_degraded"   : payload = nil,            返回 DegradedResp
type SearchVectorService struct {
	svc *vectorsearch.SearchService
}

// NewSearchVectorService 构造 adapter。svc 可为 nil（Health 会返回错误）。
func NewSearchVectorService(svc *vectorsearch.SearchService) *SearchVectorService {
	return &SearchVectorService{svc: svc}
}

func (s *SearchVectorService) Name() string { return "search.vector" }

func (s *SearchVectorService) Init(ctx kernel.ServiceContext) error {
	if s.svc == nil {
		// 不阻断启动 — search 不可用时其他服务仍可注册
		slog.Warn("kernel: search.vector init skipped (svc is nil, search disabled)")
		return nil
	}
	slog.Info("kernel: search.vector init ok", "degraded", s.svc.IsDegraded())
	return nil
}

func (s *SearchVectorService) Health(ctx kernel.ServiceContext) error {
	if s.svc == nil {
		return errors.New("search service not initialized")
	}
	if s.svc.IsDegraded() {
		// L2 降级：Health 仍返回 nil（ degraded 不等于不健康），
		// 调用方通过 is_degraded method 自行判断是否要 warn 用户
		return nil
	}
	return nil
}

func (s *SearchVectorService) Call(ctx kernel.ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	if s.svc == nil {
		return nil, errors.New("search service not initialized")
	}

	switch method {
	case "search_files":
		var req SearchFilesReq
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("search_files: bad payload: %w", err)
		}
		results, err := s.svc.SearchFiles(ctx, req.Query, req.Limit)
		if err != nil {
			return nil, fmt.Errorf("search_files: %w", err)
		}
		return json.Marshal(SearchFilesResp{Results: results})

	case "search_tasks":
		var req SearchTasksReq
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("search_tasks: bad payload: %w", err)
		}
		results, err := s.svc.SearchTasks(ctx, req.Query, req.Limit)
		if err != nil {
			return nil, fmt.Errorf("search_tasks: %w", err)
		}
		return json.Marshal(SearchTasksResp{Results: results})

	case "index_file":
		var req IndexFileReq
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("index_file: bad payload: %w", err)
		}
		if err := s.svc.IndexFile(ctx, req.Path, req.Name, req.Size, req.Mtime); err != nil {
			return nil, fmt.Errorf("index_file: %w", err)
		}
		return json.Marshal(EmptyResp{})

	case "index_task":
		var req IndexTaskReq
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("index_task: bad payload: %w", err)
		}
		if err := s.svc.IndexTask(ctx, req.TaskID, req.Name, req.TaskType, req.SourcePath, req.Status); err != nil {
			return nil, fmt.Errorf("index_task: %w", err)
		}
		return json.Marshal(EmptyResp{})

	case "delete_file":
		var req DeleteReq
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("delete_file: bad payload: %w", err)
		}
		if err := s.svc.DeleteFile(ctx, req.RefID); err != nil {
			return nil, fmt.Errorf("delete_file: %w", err)
		}
		return json.Marshal(EmptyResp{})

	case "delete_task":
		var req DeleteReq
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("delete_task: bad payload: %w", err)
		}
		if err := s.svc.DeleteTask(ctx, req.RefID); err != nil {
			return nil, fmt.Errorf("delete_task: %w", err)
		}
		return json.Marshal(EmptyResp{})

	case "stats":
		stats := s.svc.Stats()
		return json.Marshal(StatsResp{IndexedFiles: stats["files"], IndexedTasks: stats["tasks"]})

	case "is_degraded":
		return json.Marshal(DegradedResp{Degraded: s.svc.IsDegraded()})

	default:
		return nil, fmt.Errorf("unknown method %q for search.vector", method)
	}
}

// ─── SearchVector payload 类型 ─────────────────────────────────────────

type SearchFilesReq struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type SearchFilesResp struct {
	Results []vectorsearch.SearchResult `json:"results"`
}

type SearchTasksReq struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type SearchTasksResp struct {
	Results []vectorsearch.SearchResult `json:"results"`
}

type IndexFileReq struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Size  string `json:"size"`
	Mtime string `json:"mtime"`
}

type IndexTaskReq struct {
	TaskID     string `json:"task_id"`
	Name       string `json:"name"`
	TaskType   string `json:"task_type"`
	SourcePath string `json:"source_path"`
	Status     string `json:"status"`
}

type DeleteReq struct {
	RefID string `json:"ref_id"`
}

type EmptyResp struct{}

type StatsResp struct {
	IndexedFiles int `json:"indexed_files"`
	IndexedTasks int `json:"indexed_tasks"`
}

type DegradedResp struct {
	Degraded bool `json:"degraded"`
}

// ============================================================================
// WSHubService — 包装 *service.WSHub
// ============================================================================

// WSHubService 把 WebSocket hub 注册为 kernel.Service。
//
// 支持的 method：
//   - "broadcast" : payload = BroadcastReq, 返回 EmptyResp
//
// 设计说明：
//   - TaskManager 已通过 Broadcaster interface 解耦 WSHub，本 adapter 不强制替代
//   - 未来 AI agent tool 调用 / kernel Bus 跨服务事件可走本 adapter
//   - 旧的 broadcaster 字段保留，作为 fallback（渐进式迁移）
type WSHubService struct {
	hub *mobileservice.WSHub
}

// NewWSHubService 构造 adapter。hub 可为 nil（Health 会返回错误）。
func NewWSHubService(hub *mobileservice.WSHub) *WSHubService {
	return &WSHubService{hub: hub}
}

func (w *WSHubService) Name() string { return "ws.hub" }

func (w *WSHubService) Init(ctx kernel.ServiceContext) error {
	if w.hub == nil {
		slog.Warn("kernel: ws.hub init skipped (hub is nil)")
		return nil
	}
	slog.Info("kernel: ws.hub init ok")
	return nil
}

func (w *WSHubService) Health(ctx kernel.ServiceContext) error {
	if w.hub == nil {
		return errors.New("ws hub not initialized")
	}
	return nil
}

func (w *WSHubService) Call(ctx kernel.ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	if w.hub == nil {
		return nil, errors.New("ws hub not initialized")
	}

	switch method {
	case "broadcast":
		var req BroadcastReq
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("broadcast: bad payload: %w", err)
		}
		// Data 字段是 json.RawMessage，传给 Broadcast 时需要先 unmarshal 到 interface{}
		// （WSHub.Broadcast 内部会再 json.Marshal，传 RawMessage 会被二次序列化成字符串）
		var data interface{}
		if len(req.Data) > 0 {
			var v interface{}
			if err := json.Unmarshal(req.Data, &v); err != nil {
				return nil, fmt.Errorf("broadcast: bad data field: %w", err)
			}
			data = v
		}
		w.hub.Broadcast(req.Type, data)
		return json.Marshal(EmptyResp{})

	default:
		return nil, fmt.Errorf("unknown method %q for ws.hub", method)
	}
}

// ─── WSHub payload 类型 ─────────────────────────────────────────

type BroadcastReq struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// ============================================================================
// FTSRebuilderService — 包装 service.FTSRebuilder
// ============================================================================

// FTSRebuilderService 把 FTS 索引重建器注册为 kernel.Service。
//
// 支持的 method：
//   - "rebuild" : payload = RebuildReq（可选 progress_events bool），返回 EmptyResp
//     注：重建是长任务，进度通过 WS 事件推送（不通过 Call 返回值）
//
// 设计说明：
//   - FTSRebuilder interface 在 service 包定义，server 包提供 ftsRebuilderImpl 实现
//   - 本 adapter 让 AI agent tool / kernel Bus 能触发重建
//   - TaskManager 仍走 SetFTSRebuilder 直接调用路径（性能优先）
type FTSRebuilderService struct {
	rebuilder mobileservice.FTSRebuilder
}

// NewFTSRebuilderService 构造 adapter。rebuilder 可为 nil（Health 返回错误）。
func NewFTSRebuilderService(rebuilder mobileservice.FTSRebuilder) *FTSRebuilderService {
	return &FTSRebuilderService{rebuilder: rebuilder}
}

func (f *FTSRebuilderService) Name() string { return "fts.rebuilder" }

func (f *FTSRebuilderService) Init(ctx kernel.ServiceContext) error {
	if f.rebuilder == nil {
		slog.Warn("kernel: fts.rebuilder init skipped (rebuilder is nil)")
		return nil
	}
	slog.Info("kernel: fts.rebuilder init ok")
	return nil
}

func (f *FTSRebuilderService) Health(ctx kernel.ServiceContext) error {
	if f.rebuilder == nil {
		return errors.New("fts rebuilder not initialized")
	}
	return nil
}

func (f *FTSRebuilderService) Call(ctx kernel.ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	if f.rebuilder == nil {
		return nil, errors.New("fts rebuilder not initialized")
	}

	switch method {
	case "rebuild":
		// payload 可为空（无配置项）；进度通过 ctx 内的 progressCb 推送
		// 当前实现：不通过 kernel 返回进度（长任务，应该走任务系统）
		// progressCb 仅做日志记录（避免 kernel 调用方等待）
		progressCb := func(progress int, phase, speed, eta string) {
			slog.Info("kernel: fts.rebuild progress",
				"progress", progress, "phase", phase, "speed", speed, "eta", eta)
		}
		if err := f.rebuilder.RebuildWithProgress(ctx, progressCb); err != nil {
			return nil, fmt.Errorf("rebuild: %w", err)
		}
		return json.Marshal(EmptyResp{})

	default:
		return nil, fmt.Errorf("unknown method %q for fts.rebuilder", method)
	}
}

// ─── FTSRebuilder payload 类型 ─────────────────────────────────────────

type RebuildReq struct {
	// 预留：未来可加 servingDir override / maxDepth / batchSize 等
}

// ============================================================================
// RegisterKernelAdapters 在 NewServer 中调用，注册所有 adapter
// ============================================================================

// RegisterKernelAdapters 注册所有 kernel.Service adapter。
//
// 调用时机：NewServer 末尾，所有底层 service（searchSvc / wsHub / ftsRebuilder）已构造完成。
// 注册后立即调 Init（用 background ctx，因为此时还没收到任何 HTTP request）。
//
// 注意：
//   - 同名重复注册会 panic（kernel.Register 行为），测试用 kernel.Unregister 清理
//   - svc 为 nil 时 adapter 仍注册（Health 会返回错误，但 Init 不阻断）
//   - 返回 init 失败的 service 列表，让调用方决定是否 slog.Error
func RegisterKernelAdapters(
	searchSvc *vectorsearch.SearchService,
	wsHub *mobileservice.WSHub,
	ftsRebuilder mobileservice.FTSRebuilder,
) []error {
	adapters := []kernel.Service{
		NewSearchVectorService(searchSvc),
		NewWSHubService(wsHub),
		NewFTSRebuilderService(ftsRebuilder),
	}

	bgCtx := kernel.NewContext(context.Background())
	var errs []error
	for _, a := range adapters {
		// 先注册（让其他 service 能 Get 到它）
		kernel.Register(a)
		// 再 Init（失败不 unregister，让 Health 暴露问题）
		if err := a.Init(bgCtx); err != nil {
			slog.Error("kernel: service init failed",
				"name", a.Name(), "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", a.Name(), err))
		}
	}
	return errs
}
