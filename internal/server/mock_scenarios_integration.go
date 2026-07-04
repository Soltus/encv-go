// internal/server/mock_scenarios_integration.go
//
// 剧本外置 spec：把 ScenarioLoader 接入 Server 生命周期。
//
// 行为矩阵：
//
//	agent.mock_scenarios_dir == ""  → 保持 NewMockEngine() builtin 剧本，零回归
//	agent.mock_scenarios_dir == "x"  →
//	    1. 构造 ScenarioLoader
//	    2. LoadAll 扫描 + 解析 + 校验
//	    3. 若 YAML 成功加载 → 替换 mockEngine（用 NewMockEngineWithScenarios）
//	    4. 若 YAML 解析失败 / 目录空 → 保留 builtin 剧本，log warn
//
// 热重载：
//   - Start() 阶段判断 agent.mock_scenarios_hot_reload + dir 非空
//   - 启 goroutine 跑 loader.Watch(ctx) — 检测到 *.yaml/*.json 变更 → 重新 LoadAll
//   - 失败 log error 不中断 watcher
//
// 设计原则：
//   - 零修改现有 NewServer(ctx, configPath) 签名（向后兼容）
//   - 全部读取逻辑走 s.getAgentConfig()（与 handleAgentChat 同源）
//   - 任何失败都是 log warn，不阻断启动
package server

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Soltus/encv-go/internal/config"
)

// loadScenariosFromAgentConfig 读取 agent_settings.mock_scenarios_dir 并加载 YAML 剧本。
//
// 调用时机：NewServer 末尾（toolDeps 注入之后）。
//
// 行为：
//   1. 通过 s.getAgentConfig() 拿到 agent.MockScenariosDir
//   2. 字符串为空 → 跳过（保持 builtin 剧本）
//   3. 构造 ScenarioLoader，调用 LoadAll(ctx)
//   4. 加载成功且非空 → 用 NewMockEngineWithScenarios 替换 s.mockEngine
//   5. 加载成功但为空 → 保留 builtin 剧本，log warn
//   6. 加载失败 → 保留 builtin 剧本，log error（**不**阻断启动）
//
// 关键约束：替换 mockEngine 后必须重新注入 realExecutor（NewMockEngineWithScenarios 是新实例）。
func (s *Server) loadScenariosFromAgentConfig() {
	agentCfg := s.getAgentConfig()
	dir := agentCfg.MockScenariosDir
	if dir == "" {
		slog.Debug("mock scenarios: using builtin (no dir configured)")
		return
	}
	s.scenariosDir = dir

	loader := NewScenarioLoader(dir)
	// Go 字面量 fallback 已废弃：剧本现在来自 mock_scenario_embedded.go 的 mockScenariosBuiltin。
	// 不再设置 GoFallback —— 若 YAML dir 加载失败 / 目录为空，loader.LoadAll 内部走 nil fallback 路径。
	_ = loader // loader 留作未来 Go fallback 重新注入用
	if err := loader.LoadAll(context.Background()); err != nil {
		slog.Error("mock scenarios: LoadAll failed, keeping builtin", "dir", dir, "error", err)
		return
	}
	scenarios := loader.GetScenarios()
	if len(scenarios) == 0 {
		slog.Warn("mock scenarios: YAML dir empty or all invalid, keeping builtin",
			"dir", dir)
		return
	}

	// 替换 mockEngine
	s.mockEngine = NewMockEngineWithScenarios(scenarios)
	s.mockEngine.SetRealExecutor(s.executeAgentTool)
	s.scenarioLoader = loader

	slog.Info("mock scenarios: YAML loaded and bound to MockEngine",
		"dir", dir, "count", len(scenarios))
}

// StartMockScenariosWatcher 启动热重载 watcher。
//
// 调用时机：Server.Start() 末尾（在 HTTP 监听启动之后）。
//
// 行为：
//   - agent.mock_scenarios_hot_reload == false → 立即返回
//   - dir 为空 → 立即返回
//   - 否则启动 goroutine 跑 loader.Watch(ctx)
//   - 失败 log error 不中断 watcher
//   - ctx 取消时优雅退出
//
// 返回的 stop 函数用于优雅关闭（虽然 ctx 取消已足够，stop 留作未来扩展）。
func (s *Server) StartMockScenariosWatcher(ctx context.Context) (stop func()) {
	if s.scenarioLoader == nil {
		return func() {}
	}
	agentCfg := s.getAgentConfig()
	if !agentCfg.MockScenariosHotReload {
		slog.Debug("mock scenarios: hot reload disabled")
		return func() {}
	}
	if s.scenariosDir == "" {
		return func() {}
	}

	// 构造子 ctx（与父 ctx 联动取消）
	watchCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		if err := s.scenarioLoader.Watch(watchCtx); err != nil && err != context.Canceled {
			slog.Error("mock scenarios: watcher exited with error", "error", err)
		}
	}()

	stop = func() {
		cancel()
		wg.Wait()
	}
	slog.Info("mock scenarios: hot reload watcher started", "dir", s.scenariosDir)
	return stop
}

// AllScenariosAsMap 返回当前 MockEngine 全部剧本的 map（id → scenario）。
// 用于前端 GET /api/agent/mock/scenarios 端点（未来扩展）。
func (s *Server) AllScenariosAsMap() map[string]*MockScenario {
	if s.mockEngine == nil {
		return nil
	}
	out := make(map[string]*MockScenario, 64)
	for _, sc := range s.mockEngine.AllScenarios() {
		out[sc.ID] = sc
	}
	return out
}

// ScenariosDir 返回当前生效的剧本目录（空字符串 = builtin）。
func (s *Server) ScenariosDir() string {
	return s.scenariosDir
}

// GetAgentConfig 暴露给外部的 agent 配置读取（带缓存）。
// 内部维护一个 atomic 缓存（agent 配置文件不常变，避免每次重复解析）。
//
// 当前实现：直接调 s.getAgentConfig()，未来扩展可加 cache。
func (s *Server) GetAgentConfig() *config.Agent {
	return s.getAgentConfig()
}
