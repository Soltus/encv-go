// internal/server/mock_scenario_loader.go
//
// 剧本加载器：扫描目录、解析 YAML/JSON、校验、注册到 MockEngine。
//
// 行为矩阵：
//
//	目录存在 + 有文件  → 加载 YAML（priority 0，覆盖一切）
//	目录存在 + 空      → 加载 Go fallback（向后兼容）
//	目录不存在         → 加载 Go fallback（向后兼容）
//	目录存在 + 全无效  → 加载 Go fallback + log error（不阻断）
//
// 热重载：
//   - fsnotify 监听 *.yaml / *.json 变更
//   - 新文件 / 修改文件 → 重新 LoadAll() 并原子替换
//   - 失败 log error 但不中断 watcher
//   - 活跃 stream 不受影响（旧剧本继续）
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// ════════════════════════════════════════════════════════════════
// Loader
// ════════════════════════════════════════════════════════════════

// ScenarioLoader 负责扫描 + 解析 + 校验 + 注册。
//
// 线程安全：
//   - LoadAll / Watch 内部用 mu 保护 scenarios 切片
//   - GetScenarios() 返回**副本**（不暴露内部状态）
type ScenarioLoader struct {
	dir   string
	mu    sync.RWMutex
	scens []*MockScenario
	// GoFallback 注入：当 dir 不存在 / 为空 / 全无效时使用
	GoFallback func() []*MockScenario
}

// NewScenarioLoader 构造 loader。dir 可以不存在（不会 panic，LoadAll 时降级）。
func NewScenarioLoader(dir string) *ScenarioLoader {
	return &ScenarioLoader{dir: dir}
}

// LoadAll 扫描目录、解析、校验、注册。
//
// 行为：
//   1. dir 为空 / 不存在 → 直接走 GoFallback
//   2. 扫描 *.yaml + *.json 逐个解析
//   3. 单个文件失败 log error 继续（错误聚合，不阻断）
//   4. 重复 id → 第一个赢，第二个 log error 跳过
//   5. 全部失败 → 走 GoFallback
func (l *ScenarioLoader) LoadAll(ctx context.Context) error {
	yamlScenarios, yamlErr := l.scanDir()

	var finalScens []*MockScenario
	if len(yamlScenarios) > 0 {
		// YAML 优先：完全覆盖 Go fallback
		finalScens = yamlScenarios
		slog.Info("mock scenarios: loaded from YAML", "count", len(yamlScenarios), "dir", l.dir)
	} else {
		// YAML 空 / 不可用 → 降级到 Go 字面量
		if l.GoFallback != nil {
			finalScens = l.GoFallback()
			if yamlErr != nil {
				slog.Warn("mock scenarios: YAML load failed, using Go fallback",
					"dir", l.dir, "error", yamlErr, "fallback_count", len(finalScens))
			} else {
				slog.Info("mock scenarios: YAML dir empty/missing, using Go fallback",
					"dir", l.dir, "fallback_count", len(finalScens))
			}
		} else {
			slog.Warn("mock scenarios: no YAML and no Go fallback configured",
				"dir", l.dir, "yaml_err", yamlErr)
		}
	}

	l.mu.Lock()
	l.scens = finalScens
	l.mu.Unlock()
	return nil
}

// GetScenarios 返回当前加载的剧本副本（线程安全）。
func (l *ScenarioLoader) GetScenarios() []*MockScenario {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*MockScenario, len(l.scens))
	copy(out, l.scens)
	return out
}

// GetLoadedYAMLScenarios 仅返回从 YAML 加载的剧本（用于 E2E / CI 红线扫描）。
//
// 返回 LoadedScenario（未经转换），便于检查原始 YAML 字段。
// 注意：原始 LoadedScenario 不在 loader 内部保存（转换后丢弃），
// 此方法在每次调用时**重新解析目录**（只读，不修改状态）。
func (l *ScenarioLoader) GetLoadedYAMLScenarios() []LoadedScenario {
	scenarios, _ := l.scanDirAsLoaded()
	return scenarios
}

// ════════════════════════════════════════════════════════════════
// 目录扫描 + 解析
// ════════════════════════════════════════════════════════════════

// scanDir 扫描目录返回转换后的 MockScenario 列表（去重 + 校验）。
func (l *ScenarioLoader) scanDir() ([]*MockScenario, error) {
	scenarios, err := l.scanDirAsLoaded()
	if err != nil {
		return nil, err
	}

	out := make([]*MockScenario, 0, len(scenarios))
	seenIDs := make(map[string]bool, len(scenarios))
	for i := range scenarios {
		s := &scenarios[i]
		if seenIDs[s.ID] {
			slog.Error("mock loader: duplicate scenario id, skipping", "id", s.ID)
			continue
		}
		seenIDs[s.ID] = true
		out = append(out, s.ConvertToMockScenario())
	}
	return out, nil
}

// scanDirAsLoaded 扫描目录返回原始 LoadedScenario 列表（未转换、未去重）。
//
// 行为：
//   - dir 不存在 / 为空 / 非目录 → 返回 nil, nil（由调用方决定 fallback）
//   - 文件读取 / 解析 / 校验失败 → log error 跳过该文件，继续下一个
//   - 重复 id → 在 caller 层去重（loader.scanDir 阶段）
func (l *ScenarioLoader) scanDirAsLoaded() ([]LoadedScenario, error) {
	if l.dir == "" {
		return nil, nil
	}
	info, err := os.Stat(l.dir)
	if err != nil || !info.IsDir() {
		return nil, nil // 目录不存在 → 静默降级
	}

	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var out []LoadedScenario
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		path := filepath.Join(l.dir, name)
		loaded, err := l.parseFile(path)
		if err != nil {
			slog.Error("mock loader: skip file", "path", path, "error", err)
			continue
		}
		if err := loaded.Validate(); err != nil {
			slog.Error("mock loader: skip file (validation failed)", "path", path, "error", err)
			continue
		}
		out = append(out, *loaded)
	}
	return out, nil
}

// parseFile 读取并解析单个 YAML/JSON 文件。
func (l *ScenarioLoader) parseFile(path string) (*LoadedScenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return parseLoadedScenarioBytes(data, path)
}

// parseLoadedScenarioBytes 从字节解析单个 YAML/JSON 剧本。
// 文件名仅用于日志/扩展名推断。返回 *LoadedScenario（未转换、未校验）
// 以便调用方按需 Validate + ConvertToMockScenario。
func parseLoadedScenarioBytes(data []byte, filename string) (*LoadedScenario, error) {
	var s LoadedScenario
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("json unmarshal: %w", err)
		}
	default: // .yaml / .yml
		if err := yaml.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("yaml unmarshal: %w", err)
		}
	}
	return &s, nil
}

// ════════════════════════════════════════════════════════════════
// 热重载 Watcher
// ════════════════════════════════════════════════════════════════

// Watch 启动 fsnotify 监听 dir 下的 *.yaml / *.json 变更。
//
// 行为：
//   - ctx 取消 → 优雅退出
//   - 监听事件：Create / Write / Remove / Rename
//   - 防抖：500ms 内多次事件合并为一次 reload
//   - 失败 log error 但不中断 watcher
//   - 活跃 stream 不受影响（旧剧本继续，新 stream 用新剧本）
//
// 阻塞直到 ctx 取消。
func (l *ScenarioLoader) Watch(ctx context.Context) error {
	if l.dir == "" {
		slog.Info("mock loader: watch skipped (empty dir)")
		<-ctx.Done()
		return ctx.Err()
	}
	info, err := os.Stat(l.dir)
	if err != nil || !info.IsDir() {
		slog.Warn("mock loader: watch skipped (dir not exist)", "dir", l.dir)
		<-ctx.Done()
		return ctx.Err()
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close() //nolint:errcheck

	if err := watcher.Add(l.dir); err != nil {
		return fmt.Errorf("watch dir: %w", err)
	}

	slog.Info("mock loader: watching dir for changes", "dir", l.dir)

	// 防抖定时器：500ms 内合并多次事件
	var debounceTimer *time.Timer
	const debounceDelay = 500 * time.Millisecond

	reload := func() {
		if err := l.LoadAll(ctx); err != nil {
			slog.Error("mock loader: reload failed", "error", err)
		}
	}

	scheduleReload := func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(debounceDelay, reload)
	}

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			slog.Info("mock loader: watch stopped (ctx done)")
			return ctx.Err()

		case ev, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("watcher events channel closed")
			}
			// 只关心 YAML/JSON 变更
			ext := strings.ToLower(filepath.Ext(ev.Name))
			if ext != ".yaml" && ext != ".yml" && ext != ".json" {
				continue
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
				slog.Debug("mock loader: change detected",
					"file", ev.Name, "op", ev.Op.String())
				scheduleReload()
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("watcher errors channel closed")
			}
			slog.Error("mock loader: watch error", "error", err)
			// 不 return — 继续监听
		}
	}
}

// ════════════════════════════════════════════════════════════════
// 兼容性保留
// ════════════════════════════════════════════════════════════════

// defaultGoFallback 保留为占位（向后兼容，调用方已全部切到 mockScenariosBuiltin）。
// 当前实现：直接复用 package-level 变量。No-op。
func defaultGoFallback() []*MockScenario {
	return mockScenariosBuiltin
}
