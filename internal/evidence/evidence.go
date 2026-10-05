// Package evidence 定义"可复查验收证据"记录（vNext 规格 EVD-001）。
//
// 规则：任何"完成"状态必须绑定一条 EvidenceRecord，标明 real / replay /
// synthetic，并携带工作区提交、制品摘要、运行时 target 与生产实例。
// 没有 evidenceId 的勾选一律无效；synthetic 证据不得满足生产验收。
package evidence

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

// SchemaVersion 当前证据记录 schema 版本。
const SchemaVersion = 1

// Mode 证据采集模式。
type Mode string

const (
	// ModeReal 真实环境：真实 Provider / 真实设备 / 真实网络 / 真实副作用。
	ModeReal Mode = "real"
	// ModeReplay 重放：用真实事件 schema 与投影器重放，可验证投影兼容性，
	// 不能证明网络、设备、更新或副作用执行。
	ModeReplay Mode = "replay"
	// ModeSynthetic 合成：Mock / 剧本 / 单元测试替身。只能满足 UI 与组件测试。
	ModeSynthetic Mode = "synthetic"
)

// Valid 报告 mode 是否为三个法定取值之一。
func (m Mode) Valid() bool {
	switch m {
	case ModeReal, ModeReplay, ModeSynthetic:
		return true
	}
	return false
}

// Record 一条验收证据。
type Record struct {
	SchemaVersion      int      `json:"schemaVersion"`
	EvidenceID         string   `json:"evidenceId"`
	RequirementID      string   `json:"requirementId"`
	WorkspaceCommit    string   `json:"workspaceCommit"`
	ArtifactDigest     string   `json:"artifactDigest"`
	RuntimeTargetID    string   `json:"runtimeTargetId"`
	ProducerInstanceID string   `json:"producerInstanceId"`
	Mode               Mode     `json:"mode"`
	Steps              []string `json:"steps,omitempty"`
	Observations       []string `json:"observations,omitempty"`
	LogRefs            []string `json:"logRefs,omitempty"`
	Result             string   `json:"result"`
	RecordedAt         string   `json:"recordedAt"`
}

// Validate 校验必填字段。缺失 requirementId / 非法 mode / 空 result 都拒绝落盘，
// 避免产生"看起来完成但没有内容"的证据。
func (r Record) Validate() error {
	if r.RequirementID == "" {
		return errors.New("evidence: requirementId is required")
	}
	if !r.Mode.Valid() {
		return fmt.Errorf("evidence: invalid mode %q (want real|replay|synthetic)", r.Mode)
	}
	if r.Result == "" {
		return errors.New("evidence: result is required")
	}
	return nil
}

// Store 证据存储（JSONL 追加）。
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore 在 dir 下创建 evidence.jsonl（目录 0700，文件 0600）。
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("evidence: store dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("evidence: mkdir %s: %w", dir, err)
	}
	return &Store{path: filepath.Join(dir, "evidence.jsonl")}, nil
}

// Append 追加一条证据。校验失败直接返回错误，不静默丢弃。
func (s *Store) Append(r Record) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.SchemaVersion == 0 {
		r.SchemaVersion = SchemaVersion
	}
	if r.EvidenceID == "" {
		id, err := NewEvidenceID()
		if err != nil {
			return err
		}
		r.EvidenceID = id
	}
	if r.RecordedAt == "" {
		r.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("evidence: marshal: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("evidence: open %s: %w", s.path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("evidence: write: %w", err)
	}
	return nil
}

// List 读取全部证据。损坏行返回错误（证据不允许静默跳过）。
func (s *Store) List() ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: read: %w", err)
	}
	out := make([]Record, 0)
	for _, raw := range splitLines(data) {
		if len(raw) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("evidence: corrupt line: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}

// Filter 按 requirementId 与 mode 过滤；空值表示不过滤该维度。
func (s *Store) Filter(requirementID string, mode Mode) ([]Record, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(all))
	for _, r := range all {
		if requirementID != "" && r.RequirementID != requirementID {
			continue
		}
		if mode != "" && r.Mode != mode {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	var out [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			out = append(out, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}

// NewEvidenceID 生成随机证据 id。
func NewEvidenceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("evidence: rand: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// CurrentWorkspaceCommit 取构建期注入的 VCS revision（go build 默认带 -buildvcs）。
// 取不到返回空串，调用方不得据此伪造证据。
func CurrentWorkspaceCommit() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, st := range bi.Settings {
			if st.Key == "vcs.revision" {
				return st.Value
			}
		}
	}
	return ""
}

// CurrentArtifactDigest 对当前可执行文件取 sha256，作为"跑的到底是哪份制品"的摘要。
func CurrentArtifactDigest() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(exe)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// CurrentProducerInstanceID 当前进程实例标识（hostname + pid）。
func CurrentProducerInstanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// NewRecord 填充当前进程可自动采集的字段；RequirementID / Mode / Result 由调用方提供。
func NewRecord(requirementID string, mode Mode, runtimeTargetID, result string, steps, observations, logRefs []string) Record {
	return Record{
		SchemaVersion:      SchemaVersion,
		RequirementID:      requirementID,
		WorkspaceCommit:    CurrentWorkspaceCommit(),
		ArtifactDigest:     CurrentArtifactDigest(),
		RuntimeTargetID:    runtimeTargetID,
		ProducerInstanceID: CurrentProducerInstanceID(),
		Mode:               mode,
		Steps:              steps,
		Observations:       observations,
		LogRefs:            logRefs,
		Result:             result,
	}
}
