// Stage 7 (borrow-nuclear-boy-2026q2)：SkillManifest YAML 解析与 4 维权限模型。
//
// 借鉴自 /tmp/nuclear-boy/skills/src/main/java/com/nuclearboy/skills/SkillManifest.kt。
//
// 关键设计：
//   - SkillPermissions 4 维：filesystem / network / packages / shell
//   - isSandboxed 计算属性：无 network + 无 shell + filesystem 限定在 workspace
//   - matchesGlob 支持 ** 与 * 通配（最小子集）
package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillManifest 一个 skill 的完整元数据（对应 nuclear-boy SkillManifest.kt L9-20）。
type SkillManifest struct {
	Name        string            `yaml:"name" json:"name"`
	Version     string            `yaml:"version" json:"version"`
	Description string            `yaml:"description" json:"description"`
	Author      string            `yaml:"author,omitempty" json:"author,omitempty"`
	Homepage    string            `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	Permissions SkillPermissions  `yaml:"permissions" json:"permissions"`
	Parameters  []SkillParameter  `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	EntryPoint  string            `yaml:"entry_point,omitempty" json:"entry_point,omitempty"`
	Triggers    *SkillTriggers    `yaml:"triggers,omitempty" json:"triggers,omitempty"`
}

// SkillTriggers 自动触发条件（对应 nuclear-boy SkillManifest.kt L25-29）。
type SkillTriggers struct {
	OnStartup    bool `yaml:"on_startup,omitempty" json:"on_startup,omitempty"`
	OnNewProject bool `yaml:"on_new_project,omitempty" json:"on_new_project,omitempty"`
}

// SkillParameter skill 接受的参数定义。
type SkillParameter struct {
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type" json:"type"` // "string" / "number" / "boolean" / "path"
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
}

// SkillPermissions 4 维权限（对应 nuclear-boy L35-66）。
type SkillPermissions struct {
	Filesystem *FilesystemPermissions `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	Network    *NetworkPermission     `yaml:"network,omitempty" json:"network,omitempty"`
	Packages   *PackagePermission     `yaml:"packages,omitempty" json:"packages,omitempty"`
	Shell      *ShellPermission       `yaml:"shell,omitempty" json:"shell,omitempty"`
}

// FilesystemPermissions 文件系统权限（glob allowlist）。
type FilesystemPermissions struct {
	Read  []string `yaml:"read,omitempty" json:"read,omitempty"`
	Write []string `yaml:"write,omitempty" json:"write,omitempty"`
}

// CanRead 路径是否在读 allowlist。
func (f *FilesystemPermissions) CanRead(path string) bool {
	if f == nil {
		return false
	}
	for _, p := range f.Read {
		if MatchesGlob(path, p) {
			return true
		}
	}
	return false
}

// CanWrite 路径是否在写 allowlist。
func (f *FilesystemPermissions) CanWrite(path string) bool {
	if f == nil {
		return false
	}
	for _, p := range f.Write {
		if MatchesGlob(path, p) {
			return true
		}
	}
	return false
}

// NetworkPermission 网络权限。
type NetworkPermission struct {
	Allowed       bool     `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	AllowedHosts  []string `yaml:"allowed_hosts,omitempty" json:"allowed_hosts,omitempty"`
}

// PackagePermission 包安装权限（仅限白名单 package manager）。
type PackagePermission struct {
	Allowed   bool     `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	Managers  []string `yaml:"managers,omitempty" json:"managers,omitempty"` // pip / npm / go
}

// ShellPermission shell 执行权限。
type ShellPermission struct {
	Allowed      bool     `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	AllowedCmds  []string `yaml:"allowed_commands,omitempty" json:"allowed_commands,omitempty"`
}

// RequestsAnyPermission 是否请求了任何权限。
func (p *SkillPermissions) RequestsAnyPermission() bool {
	if p == nil {
		return false
	}
	return p.Filesystem != nil || p.Network != nil || p.Packages != nil || p.Shell != nil
}

// IsSandboxed 是否沙箱化（无网络、无 shell、filesystem 限定 workspace）。
//
// 借鉴 nuclear-boy SkillManifest.kt L52-65。
func (p *SkillPermissions) IsSandboxed() bool {
	if p == nil {
		return true // 无权限 → 沙箱
	}
	if p.Network != nil && p.Network.Allowed {
		return false
	}
	if p.Shell != nil && p.Shell.Allowed {
		return false
	}
	if p.Filesystem != nil {
		hasExternalRead := false
		for _, p := range p.Filesystem.Read {
			if !strings.HasPrefix(p, "workspace") {
				hasExternalRead = true
				break
			}
		}
		hasExternalWrite := false
		for _, p := range p.Filesystem.Write {
			if !strings.HasPrefix(p, "workspace") {
				hasExternalWrite = true
				break
			}
		}
		if hasExternalRead || hasExternalWrite {
			return false
		}
	}
	return true
}

// MatchesGlob 最小 glob 匹配（支持 `**` 和 `*`）。
//
// 借鉴 nuclear-boy FilesystemPermissions.kt L92-110。
//
// 规则：
//   - pattern == "**" → match all
//   - pattern == "workspace/**" → match path starting with "workspace/"
//   - 含 "**" 部分匹配
//   - `*` 匹配单层（不含 /）
func MatchesGlob(path, pattern string) bool {
	normalizedPath := strings.TrimLeft(path, "/")
	normalizedPath = strings.ReplaceAll(normalizedPath, "\\", "/")
	normalizedPattern := strings.TrimLeft(pattern, "/")
	normalizedPattern = strings.ReplaceAll(normalizedPattern, "\\", "/")

	if normalizedPattern == "**" {
		return true
	}
	if normalizedPattern == "workspace/**" {
		return strings.HasPrefix(normalizedPath, "workspace/") || normalizedPath == "workspace"
	}
	// 简化：用 strings.HasPrefix 替代完整 glob（保留接口以备扩展）
	if strings.HasSuffix(normalizedPattern, "/**") {
		prefix := strings.TrimSuffix(normalizedPattern, "/**")
		return strings.HasPrefix(normalizedPath, prefix+"/") || normalizedPath == prefix
	}
	if strings.HasSuffix(normalizedPattern, "/*") {
		prefix := strings.TrimSuffix(normalizedPattern, "/*")
		if !strings.HasPrefix(normalizedPath, prefix+"/") {
			return false
		}
		rest := strings.TrimPrefix(normalizedPath, prefix+"/")
		return !strings.Contains(rest, "/")
	}
	return normalizedPath == normalizedPattern
}

// ParseManifest 解析 skill.yaml 内容为 SkillManifest。
func ParseManifest(data []byte) (*SkillManifest, error) {
	var m SkillManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse skill manifest: %w", err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("skill manifest missing required field: name")
	}
	if m.Version == "" {
		return nil, fmt.Errorf("skill manifest missing required field: version")
	}
	if m.EntryPoint == "" {
		m.EntryPoint = "main:run"
	}
	if m.Author == "" {
		m.Author = "community"
	}
	return &m, nil
}
