package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ─── ToolKernel：AI Agent 工具调用的微内核适配层 ─────────────────────────────
//
// 2026-07-03 spec microkernel-split-start-stop Phase 4.4
//
// 核心价值：
//   第三方 AI Agent 调用工具时，不需要启动"整个后端"，
//   只按需激活对应的服务（如 search.vector、fts.rebuilder），
//   用完自动释放，满足"频繁多租户只拉起微内核处理小任务"的需求。
//
// 设计：
//   1. 工具 → 服务映射：每个 tool 声明自己依赖哪些 service
//   2. 按需激活：调用 tool 前自动激活依赖的 service
//   3. 自动释放：tool 完成后，引用计数 -1，idle 超时后 service 自动停用
//   4. 可观测：每次 tool 调用记录 activate 耗时、service 状态变化
//
// 与全局 Tool Registry 的区别：
//   - 全局 tool registry：tool 实例一直存在，调用直接 Invoke
//   - ToolKernel：tool 有生命周期，绑定到 MicroKernel，依赖的 service 按需激活
type ToolKernel struct {
	mk     *MicroKernel
	tools  map[string]*ToolEntry
	toolsMu sync.RWMutex
}

// ToolEntry 工具条目（含依赖声明）
type ToolEntry struct {
	Tool         Tool
	DependsOn    []string // 依赖的 service 名（激活顺序）
	WarmupOnBoot bool     // 是否在启动时预热
}

// NewToolKernel 构造 ToolKernel
func NewToolKernel(mk *MicroKernel) *ToolKernel {
	return &ToolKernel{
		mk:    mk,
		tools: make(map[string]*ToolEntry),
	}
}

// RegisterTool 注册一个工具（含依赖声明）
func (tk *ToolKernel) RegisterTool(tool Tool, dependsOn []string, warmupOnBoot bool) {
	if tool == nil {
		panic("kernel: RegisterTool with nil tool")
	}
	name := tool.Name()
	if name == "" {
		panic("kernel: RegisterTool with empty name")
	}
	tk.toolsMu.Lock()
	defer tk.toolsMu.Unlock()
	if _, exists := tk.tools[name]; exists {
		panic(fmt.Sprintf("kernel: tool %q already registered", name))
	}
	tk.tools[name] = &ToolEntry{
		Tool:         tool,
		DependsOn:    dependsOn,
		WarmupOnBoot: warmupOnBoot,
	}
}

// InvokeTool 调用工具（按需激活依赖的服务）。
//
// 流程：
//   1. 检查 tool 是否存在
//   2. 按顺序激活所有依赖的 service（已激活则跳过）
//   3. 引用计数 +1（每个依赖 service）
//   4. 调用 tool.Invoke
//   5. 引用计数 -1（触发 idle 计时器）
//   6. 返回结果
func (tk *ToolKernel) InvokeTool(ctx ServiceContext, toolName string, args json.RawMessage) (json.RawMessage, error) {
	if ctx == nil {
		return nil, errors.New("kernel: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tk.toolsMu.RLock()
	entry, ok := tk.tools[toolName]
	tk.toolsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, toolName)
	}

	// 激活所有依赖的 service
	activateStart := time.Now()
	for _, svcName := range entry.DependsOn {
		if err := tk.mk.Activate(ctx, svcName); err != nil {
			return nil, fmt.Errorf("kernel: activate %q for tool %q: %w", svcName, toolName, err)
		}
	}
	activateMs := time.Since(activateStart).Milliseconds()

	// 引用计数 +1（每个依赖 service）
	for _, svcName := range entry.DependsOn {
		sl, ok := tk.mk.GetServiceLifecycle(svcName)
		if ok {
			sl.acquire()
		}
	}

	// 调用结束时释放引用
	defer func() {
		for _, svcName := range entry.DependsOn {
			sl, ok := tk.mk.GetServiceLifecycle(svcName)
			if ok {
				sl.release()
			}
		}
	}()

	// 派生子 ctx（service = tool name）
	childCtx := &serviceCtx{
		parent:    ctx,
		service:   "tool." + toolName,
		requestID: ctx.RequestID(),
		traceID:   ctx.TraceID(),
		created:   time.Now(),
		store:     checkpointStoreFrom(ctx),
	}

	start := time.Now()
	resp, err := entry.Tool.Invoke(childCtx, args)
	elapsed := time.Since(start)

	recordToolInvoke(toolName, err, elapsed)

	// 异步发 bus event（不影响主流程）
	PublishAsync(ctx, "tool.invoked", map[string]any{
		"tool":       toolName,
		"elapsed":    elapsed.Milliseconds(),
		"activateMs": activateMs,
		"ok":         err == nil,
		"err":        errStr(err),
	})

	if err != nil {
		return nil, fmt.Errorf("kernel: tool %q failed after %v (activate=%dms): %w",
			toolName, elapsed, activateMs, err)
	}
	return resp, nil
}

// ListTools 列出所有已注册的工具（含依赖信息）
func (tk *ToolKernel) ListTools() []ToolInfo {
	tk.toolsMu.RLock()
	defer tk.toolsMu.RUnlock()
	out := make([]ToolInfo, 0, len(tk.tools))
	for name, entry := range tk.tools {
		info := ToolInfo{
			Name:        name,
			Description: entry.Tool.Description(),
			DependsOn:   entry.DependsOn,
			WarmupOnBoot: entry.WarmupOnBoot,
		}
		// 检查依赖 service 的状态
		for _, svc := range entry.DependsOn {
			if sl, ok := tk.mk.GetServiceLifecycle(svc); ok {
				info.ServiceStatus = append(info.ServiceStatus, ServiceStatusInfo{
					Name:   svc,
					Status: sl.Status().String(),
				})
			}
		}
		out = append(out, info)
	}
	return out
}

// Warmup 预热所有 WarmupOnBoot=true 的工具
func (tk *ToolKernel) Warmup(ctx ServiceContext) error {
	tk.toolsMu.RLock()
	entries := make([]*ToolEntry, 0)
	for _, entry := range tk.tools {
		if entry.WarmupOnBoot {
			entries = append(entries, entry)
		}
	}
	tk.toolsMu.RUnlock()

	for _, entry := range entries {
		for _, svcName := range entry.DependsOn {
			if err := tk.mk.Activate(ctx, svcName); err != nil {
				return fmt.Errorf("warmup %q: %w", svcName, err)
			}
		}
	}
	return nil
}

// ToolInfo 工具信息（用于 API 暴露）
type ToolInfo struct {
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	DependsOn     []string           `json:"dependsOn"`
	WarmupOnBoot  bool               `json:"warmupOnBoot"`
	ServiceStatus []ServiceStatusInfo `json:"serviceStatus"`
}

// ServiceStatusInfo 服务状态
type ServiceStatusInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
