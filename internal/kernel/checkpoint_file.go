package kernel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// FileCheckpointStore 磁盘版 checkpoint 存储（WorkManager 风格）
//
// 用途：长任务中断后（WorkManager 杀进程 / 设备重启 / app 切后台被系统回收），
// 上次 Checkpoint 的 state 可被 Restore 出来继续执行。
//
// 目录结构：
//
//	<root>/
//	  <traceID>/
//	    <name1>.json
//	    <name2>.json
//	    ...
//
// 线程安全：内部加锁。
// 文件写入：先写临时文件再 rename（避免半写文件被读到）。
type FileCheckpointStore struct {
	root string
	mu   sync.Mutex
}

// NewFileCheckpointStore 构造磁盘 checkpoint 存储
// root 不存在会自动创建。
func NewFileCheckpointStore(root string) (*FileCheckpointStore, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	return &FileCheckpointStore{root: root}, nil
}

// Put 写入 checkpoint
func (f *FileCheckpointStore) Put(traceID, name string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	dir := filepath.Join(f.root, sanitizeID(traceID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	finalPath := filepath.Join(dir, sanitizeID(name)+".json")
	tmpPath := finalPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, finalPath)
}

// Get 读取 checkpoint
func (f *FileCheckpointStore) Get(traceID, name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	p := filepath.Join(f.root, sanitizeID(traceID), sanitizeID(name)+".json")
	return os.ReadFile(p)
}

// Delete 删除某 trace 的所有 checkpoint
func (f *FileCheckpointStore) Delete(traceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return os.RemoveAll(filepath.Join(f.root, sanitizeID(traceID)))
}

// ─── 测试辅助 ─────────────────────────────────────────────

// Snapshot 返回某 trace 的所有 checkpoint name → data（测试 / debug 用）
func (f *FileCheckpointStore) Snapshot(traceID string) (map[string][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	dir := filepath.Join(f.root, sanitizeID(traceID))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]byte{}, nil
		}
		return nil, err
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		name := e.Name()[:len(e.Name())-len(".json")]
		out[name] = data
	}
	return out, nil
}

// sanitizeID 去掉路径分隔符和特殊字符
func sanitizeID(s string) string {
	// 简化：用 hex 编码确保文件系统安全
	// 性能 OK，因为 Checkpoint 调用频率低
	if s == "" {
		return "_empty_"
	}
	out := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// 防止 unused import 警告
var _ = json.Marshal
