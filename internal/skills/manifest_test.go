package skills

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// ─── MatchesGlob ─────────────────────────────────────────────

func TestMatchesGlob_StarStar_MatchesAll(t *testing.T) {
	if !MatchesGlob("anything", "**") {
		t.Error("** should match anything")
	}
}

func TestMatchesGlob_WorkspacePrefix(t *testing.T) {
	if !MatchesGlob("workspace/foo.txt", "workspace/**") {
		t.Error("workspace/** should match workspace/foo.txt")
	}
	if !MatchesGlob("workspace", "workspace/**") {
		t.Error("workspace/** should match bare workspace")
	}
	if MatchesGlob("other/foo", "workspace/**") {
		t.Error("workspace/** should NOT match other/foo")
	}
}

func TestMatchesGlob_SingleStar_OneLevel(t *testing.T) {
	if !MatchesGlob("dir/file.txt", "dir/*") {
		t.Error("dir/* should match dir/file.txt")
	}
	if MatchesGlob("dir/sub/file.txt", "dir/*") {
		t.Error("dir/* should NOT match dir/sub/file.txt (multi-level)")
	}
}

func TestMatchesGlob_ExactMatch(t *testing.T) {
	if !MatchesGlob("foo.txt", "foo.txt") {
		t.Error("should match exact")
	}
	if MatchesGlob("foo.txt", "bar.txt") {
		t.Error("should NOT match different")
	}
}

func TestMatchesGlob_NormalizesPathSeparator(t *testing.T) {
	if !MatchesGlob("workspace\\foo", "workspace/**") {
		t.Error("backslash should be normalized to /")
	}
}

// ─── FilesystemPermissions.CanRead/CanWrite ──────────────────

func TestFilesystemPermissions_DefaultWorkspace(t *testing.T) {
	fp := &FilesystemPermissions{
		Read:  []string{"workspace/**"},
		Write: []string{"workspace/**"},
	}
	if !fp.CanRead("workspace/foo.txt") {
		t.Error("should allow reading workspace/foo.txt")
	}
	if !fp.CanWrite("workspace/foo.txt") {
		t.Error("should allow writing workspace/foo.txt")
	}
	if fp.CanRead("/etc/passwd") {
		t.Error("should NOT allow reading /etc/passwd")
	}
}

func TestFilesystemPermissions_NilSafe(t *testing.T) {
	var fp *FilesystemPermissions
	if fp.CanRead("anything") {
		t.Error("nil perm should reject all")
	}
}

// ─── SkillPermissions.IsSandboxed ───────────────────────────

func TestSkillPermissions_NilIsSandboxed(t *testing.T) {
	var p *SkillPermissions
	if !p.IsSandboxed() {
		t.Error("nil permissions = sandboxed")
	}
}

func TestSkillPermissions_NetworkAllowedBreaksSandbox(t *testing.T) {
	p := &SkillPermissions{
		Network: &NetworkPermission{Allowed: true},
	}
	if p.IsSandboxed() {
		t.Error("network.Allowed=true should break sandbox")
	}
}

func TestSkillPermissions_ShellAllowedBreaksSandbox(t *testing.T) {
	p := &SkillPermissions{
		Shell: &ShellPermission{Allowed: true},
	}
	if p.IsSandboxed() {
		t.Error("shell.Allowed=true should break sandbox")
	}
}

func TestSkillPermissions_ExternalFilesystemBreaksSandbox(t *testing.T) {
	p := &SkillPermissions{
		Filesystem: &FilesystemPermissions{
			Read:  []string{"workspace/**", "/etc/**"},
			Write: []string{"workspace/**"},
		},
	}
	if p.IsSandboxed() {
		t.Error("external read path /etc/** should break sandbox")
	}
}

func TestSkillPermissions_WorkspaceOnlyIsSandboxed(t *testing.T) {
	p := &SkillPermissions{
		Filesystem: &FilesystemPermissions{
			Read:  []string{"workspace/**"},
			Write: []string{"workspace/**"},
		},
	}
	if !p.IsSandboxed() {
		t.Error("workspace-only should be sandboxed")
	}
}

func TestSkillPermissions_RequestsAnyPermission(t *testing.T) {
	cases := []struct {
		p    *SkillPermissions
		want bool
	}{
		{nil, false},
		{&SkillPermissions{}, false},
		{&SkillPermissions{Filesystem: &FilesystemPermissions{}}, true},
		{&SkillPermissions{Network: &NetworkPermission{}}, true},
		{&SkillPermissions{Packages: &PackagePermission{}}, true},
		{&SkillPermissions{Shell: &ShellPermission{}}, true},
	}
	for _, c := range cases {
		if got := c.p.RequestsAnyPermission(); got != c.want {
			t.Errorf("RequestsAnyPermission for %+v = %v, want %v", c.p, got, c.want)
		}
	}
}

// ─── ParseManifest ───────────────────────────────────────────

func TestParseManifest_Valid(t *testing.T) {
	yamlData := []byte(`
name: encrypt-file
version: 1.0.0
description: 加密指定文件
author: test
permissions:
  filesystem:
    read: ["workspace/**"]
    write: ["workspace/**"]
  network:
    allowed: false
parameters:
  - name: path
    type: path
    required: true
    description: 要加密的文件路径
entry_point: main:run
`)
	m, err := ParseManifest(yamlData)
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}
	if m.Name != "encrypt-file" {
		t.Errorf("Name = %q, want %q", m.Name, "encrypt-file")
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", m.Version)
	}
	if m.EntryPoint != "main:run" {
		t.Errorf("EntryPoint = %q", m.EntryPoint)
	}
	if len(m.Parameters) != 1 {
		t.Fatalf("Parameters len = %d, want 1", len(m.Parameters))
	}
	if m.Parameters[0].Name != "path" {
		t.Errorf("Parameter[0].Name = %q", m.Parameters[0].Name)
	}
	if !m.Permissions.IsSandboxed() {
		t.Error("expected sandboxed")
	}
}

func TestParseManifest_MissingName(t *testing.T) {
	yamlData := []byte(`
version: 1.0.0
description: x
`)
	_, err := ParseManifest(yamlData)
	if err == nil {
		t.Error("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention name, got: %v", err)
	}
}

func TestParseManifest_DefaultsEntryPoint(t *testing.T) {
	yamlData := []byte(`
name: x
version: 1.0.0
description: x
`)
	m, err := ParseManifest(yamlData)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.EntryPoint != "main:run" {
		t.Errorf("EntryPoint default = %q, want main:run", m.EntryPoint)
	}
	if m.Author != "community" {
		t.Errorf("Author default = %q, want community", m.Author)
	}
}

func TestParseManifest_MissingVersion(t *testing.T) {
	yamlData := []byte(`
name: x
description: y
`)
	_, err := ParseManifest(yamlData)
	if err == nil {
		t.Error("expected error for missing version")
	}
}

// ─── SafeUnzip ───────────────────────────────────────────────

// 构造测试 zip 文件
func createTestZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSafeUnzip_NormalFiles(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	dest := filepath.Join(dir, "out")
	createTestZip(t, zipPath, map[string]string{
		"a.txt":       "hello",
		"sub/b.txt":   "world",
		"sub/c/d.txt": "deep",
	})
	if err := SafeUnzip(zipPath, dest); err != nil {
		t.Fatalf("SafeUnzip failed: %v", err)
	}
	// 验证文件已解压
	for name, want := range map[string]string{
		"a.txt":       "hello",
		"sub/b.txt":   "world",
		"sub/c/d.txt": "deep",
	} {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Errorf("missing %s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s content = %q, want %q", name, got, want)
		}
	}
}

func TestSafeUnzip_BlocksZipSlip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")
	dest := filepath.Join(dir, "out")
	// 构造越界条目
	createTestZip(t, zipPath, map[string]string{
		"../etc/passwd": "evil content",
	})
	err := SafeUnzip(zipPath, dest)
	if err == nil {
		t.Error("expected error for ZIP-slip")
	}
	if !strings.Contains(err.Error(), "ZIP-slip") {
		t.Errorf("error should mention ZIP-slip, got: %v", err)
	}
}

func TestSafeUnzip_BlocksDeepZipSlip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")
	dest := filepath.Join(dir, "out")
	// 深度越界
	createTestZip(t, zipPath, map[string]string{
		"foo/../../../../etc/passwd": "evil",
	})
	err := SafeUnzip(zipPath, dest)
	if err == nil {
		t.Error("expected error for deep ZIP-slip")
	}
}

func TestSafeUnzip_RefusesSymlinks(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "symlink.zip")
	dest := filepath.Join(dir, "out")

	// 手动构造含 symlink 的 zip
	f, _ := os.Create(zipPath)
	w := zip.NewWriter(f)
	hdr := &zip.FileHeader{
		Name: "evil_link",
	}
	hdr.SetMode(os.ModeSymlink | 0o777)
	fw, _ := w.CreateHeader(hdr)
	fw.Write([]byte("/etc/passwd"))
	w.Close()
	f.Close()

	err := SafeUnzip(zipPath, dest)
	if err == nil {
		t.Error("expected error for symlink")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error should mention symlink, got: %v", err)
	}
}
