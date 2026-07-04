// Stage 8 (borrow-nuclear-boy-2026q2)：三层记忆系统。
//
// 借鉴自 /tmp/nuclear-boy/memory/.../MemoryStore.kt。
//
// 三层结构：
//   1. ProjectMemory 项目级记忆（每个项目目录一条）
//   2. UserProfile  用户偏好（带 confidence 0-1）
//   3. SemanticMemory 语义记忆（带 recallCount 召回次数）
//
// 落地：本实现用内存 map（线程安全），便于测试与未来替换为 SQLite。
// 真实落地时换 StoreBackend 接口（Save/Load/Delete）即可对接 SQLite WAL。
package memory

import (
	"sync"
	"time"
)

// ProjectMemory 项目级记忆（对应 nuclear-boy ProjectMemoryEntity）。
type ProjectMemory struct {
	ProjectPath string    `json:"projectPath"`
	Summary     string    `json:"summary"`
	Files       []string  `json:"files"`      // 关键文件列表
	Conventions []string  `json:"conventions"` // 命名/架构约定
	UpdatedAt   time.Time `json:"updatedAt"`
}

// UserProfile 用户偏好（带 confidence 0-1）。
type UserProfile struct {
	Key        string    `json:"key"`        // e.g. "preferred_language"
	Value      string    `json:"value"`      // e.g. "TypeScript"
	Confidence float64   `json:"confidence"` // 0.0 - 1.0
	Source     string    `json:"source"`     // "user_explicit" / "auto_extracted" / "manual"
	UpdatedAt  time.Time `json:"updatedAt"`
}

// SemanticMemory 语义记忆（带 recallCount）。
type SemanticMemory struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	Embedding   []float32 `json:"embedding,omitempty"`
	RecallCount int       `json:"recallCount"`
	CreatedAt   time.Time `json:"createdAt"`
	LastUsedAt  time.Time `json:"lastUsedAt"`
}

// Store 三层记忆存储。
//
// 线程安全。所有方法都是值返回（拷贝），调用方可安全修改。
type Store struct {
	mu sync.RWMutex
	// projects key = projectPath
	projects map[string]*ProjectMemory
	// profiles key = key (e.g. "preferred_language")
	profiles map[string]*UserProfile
	// semantics key = id
	semantics map[string]*SemanticMemory
}

// NewStore 构造空 store。
func NewStore() *Store {
	return &Store{
		projects:  make(map[string]*ProjectMemory),
		profiles:  make(map[string]*UserProfile),
		semantics: make(map[string]*SemanticMemory),
	}
}

// ─── ProjectMemory CRUD ─────────────────────────────────────

// SaveProject 写入或更新项目记忆。
func (s *Store) SaveProject(p ProjectMemory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.UpdatedAt = time.Now()
	s.projects[p.ProjectPath] = &p
}

// GetProject 读项目记忆（无则 nil）。
func (s *Store) GetProject(path string) *ProjectMemory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.projects[path]
}

// DeleteProject 删除项目记忆。
func (s *Store) DeleteProject(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[path]; ok {
		delete(s.projects, path)
		return true
	}
	return false
}

// ListProjects 列出所有项目路径。
func (s *Store) ListProjects() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.projects))
	for k := range s.projects {
		out = append(out, k)
	}
	return out
}

// ─── UserProfile CRUD ───────────────────────────────────────

// SaveProfile 写入或更新用户偏好。
// 已存在的 key 会更新 value / confidence，取较高 confidence 保留。
func (s *Store) SaveProfile(p UserProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.UpdatedAt = time.Now()
	if existing, ok := s.profiles[p.Key]; ok {
		// 保留较高 confidence
		if existing.Confidence > p.Confidence {
			p.Confidence = existing.Confidence
		}
	}
	s.profiles[p.Key] = &p
}

// GetProfile 读用户偏好。
func (s *Store) GetProfile(key string) *UserProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profiles[key]
}

// DeleteProfile 删除用户偏好。
func (s *Store) DeleteProfile(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.profiles[key]; ok {
		delete(s.profiles, key)
		return true
	}
	return false
}

// ListProfiles 列出所有偏好（按 confidence 降序）。
func (s *Store) ListProfiles() []UserProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UserProfile, 0, len(s.profiles))
	for _, p := range s.profiles {
		out = append(out, *p)
	}
	// 简单排序：confidence 高 → 低
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Confidence > out[i].Confidence {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// HighConfidenceProfiles 返回 confidence > threshold 的偏好。
// 借鉴 nuclear-boy SystemPromptBuilder.kt L168-193（confidence > 0.5）。
func (s *Store) HighConfidenceProfiles(threshold float64, maxCount int) []UserProfile {
	all := s.ListProfiles()
	out := make([]UserProfile, 0, maxCount)
	for _, p := range all {
		if p.Confidence > threshold {
			out = append(out, p)
			if len(out) >= maxCount {
				break
			}
		}
	}
	return out
}

// ─── SemanticMemory CRUD ────────────────────────────────────

// SaveSemantic 写入语义记忆。
func (s *Store) SaveSemantic(m SemanticMemory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.LastUsedAt = now
	s.semantics[m.ID] = &m
}

// GetSemantic 读语义记忆（同时增加 recallCount）。
func (s *Store) GetSemantic(id string) *SemanticMemory {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.semantics[id]
	if !ok {
		return nil
	}
	m.RecallCount++
	m.LastUsedAt = time.Now()
	return m
}

// DeleteSemantic 删除语义记忆。
func (s *Store) DeleteSemantic(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.semantics[id]; ok {
		delete(s.semantics, id)
		return true
	}
	return false
}

// ListSemantics 列出所有语义记忆。
func (s *Store) ListSemantics() []SemanticMemory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SemanticMemory, 0, len(s.semantics))
	for _, m := range s.semantics {
		out = append(out, *m)
	}
	return out
}

// Stats 返回记忆统计。
func (s *Store) Stats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]int{
		"projects":  len(s.projects),
		"profiles":  len(s.profiles),
		"semantics": len(s.semantics),
	}
}
