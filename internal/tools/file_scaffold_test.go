package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── ResolvePath ─────────────────────────────────────────────

func TestResolvePath_RelativePath(t *testing.T) {
	root := t.TempDir()
	got, err := ResolvePath(root, "foo.txt")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	want := filepath.Join(root, "foo.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolvePath_NestedPath(t *testing.T) {
	root := t.TempDir()
	got, err := ResolvePath(root, "a/b/c.txt")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if !strings.HasPrefix(got, root) {
		t.Errorf("got %q, should be under %q", got, root)
	}
}

func TestResolvePath_BlocksTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := ResolvePath(root, "../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "traversal") {
		t.Errorf("error should mention traversal, got: %v", err)
	}
}

func TestResolvePath_BlocksDeepTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := ResolvePath(root, "a/b/../../../../etc/passwd")
	if err == nil {
		t.Error("expected error for deep traversal")
	}
}

func TestResolvePath_BlocksAbsoluteOutside(t *testing.T) {
	root := t.TempDir()
	_, err := ResolvePath(root, "/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute path outside root")
	}
}

// ─── ShouldSkipDir ──────────────────────────────────────────

func TestShouldSkipDir_DefaultDirs(t *testing.T) {
	for _, name := range []string{".git", "node_modules", "__pycache__", "build", ".gradle"} {
		if !ShouldSkipDir(name, DefaultSkipDirs) {
			t.Errorf("%q should be skipped", name)
		}
	}
}

func TestShouldSkipDir_NotSkipped(t *testing.T) {
	for _, name := range []string{"src", "lib", "tests", "docs", "main.go"} {
		if ShouldSkipDir(name, DefaultSkipDirs) {
			t.Errorf("%q should NOT be skipped", name)
		}
	}
}

func TestShouldSkipDir_HiddenDirs(t *testing.T) {
	for _, name := range []string{".github", ".vscode", ".idea"} {
		if !ShouldSkipDir(name, nil) {
			t.Errorf("%q should be skipped (hidden)", name)
		}
	}
}

func TestShouldSkipDir_DotAndDotDot(t *testing.T) {
	if ShouldSkipDir(".", nil) {
		t.Error("'.' should NOT be skipped (we need it for walk)")
	}
	if ShouldSkipDir("..", nil) {
		t.Error("'..' should NOT be skipped")
	}
}

// ─── SearchFiles ─────────────────────────────────────────────

func TestSearchFiles_Basic(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "b.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "sub", "c.txt"), []byte("x"), 0o644)

	got, err := SearchFiles(root, "txt", nil)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 txt files, got %d: %+v", len(got), got)
	}
}

func TestSearchFiles_SkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "node_modules"), 0o755)
	os.WriteFile(filepath.Join(root, "node_modules", "x.js"), []byte("x"), 0o644)

	got, err := SearchFiles(root, "", nil)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	for _, f := range got {
		if strings.Contains(f, ".git") || strings.Contains(f, "node_modules") {
			t.Errorf("should skip %q", f)
		}
	}
	if len(got) != 1 {
		t.Errorf("expected 1 file (only a.txt), got %d: %+v", len(got), got)
	}
}

func TestSearchFiles_PatternMatch(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "foo.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "bar.py"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "baz.go"), []byte("x"), 0o644)

	got, err := SearchFiles(root, ".go", nil)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 .go files, got %d", len(got))
	}
}

// ─── BuildReadme ─────────────────────────────────────────────

func TestBuildReadme_Python(t *testing.T) {
	rm := BuildReadme("myproj", TechStackPython)
	if !strings.Contains(rm, "myproj") {
		t.Error("README should contain project name")
	}
	if !strings.Contains(rm, "python") {
		t.Error("Python README should mention python")
	}
	if !strings.Contains(rm, "requirements.txt") {
		t.Error("Python README should mention requirements.txt")
	}
}

func TestBuildReadme_Go(t *testing.T) {
	rm := BuildReadme("myproj", TechStackGo)
	if !strings.Contains(rm, "go mod") {
		t.Error("Go README should mention go mod")
	}
	if !strings.Contains(rm, "go test") {
		t.Error("Go README should mention go test")
	}
}

func TestBuildReadme_AllStacks(t *testing.T) {
	stacks := []TechStack{TechStackPython, TechStackKotlin, TechStackJS, TechStackGo}
	for _, s := range stacks {
		rm := BuildReadme("test", s)
		if !strings.Contains(rm, "test") {
			t.Errorf("%s README should contain project name", s)
		}
	}
}

// ─── BuildGitignore ──────────────────────────────────────────

func TestBuildGitignore_Python(t *testing.T) {
	gi := BuildGitignore(TechStackPython)
	if !strings.Contains(gi, "__pycache__") {
		t.Error("Python .gitignore should include __pycache__")
	}
	if !strings.Contains(gi, ".venv") {
		t.Error("Python .gitignore should include .venv")
	}
}

func TestBuildGitignore_Go(t *testing.T) {
	gi := BuildGitignore(TechStackGo)
	if !strings.Contains(gi, "vendor/") {
		t.Error("Go .gitignore should include vendor/")
	}
}

func TestBuildGitignore_JS(t *testing.T) {
	gi := BuildGitignore(TechStackJS)
	if !strings.Contains(gi, "node_modules/") {
		t.Error("JS .gitignore should include node_modules/")
	}
}

func TestBuildGitignore_Kotlin(t *testing.T) {
	gi := BuildGitignore(TechStackKotlin)
	if !strings.Contains(gi, ".gradle/") {
		t.Error("Kotlin .gitignore should include .gradle/")
	}
}

// ─── ScaffoldProject ─────────────────────────────────────────

func TestScaffoldProject_Go(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "myproj")
	r, err := ScaffoldProject(dest, "myproj", TechStackGo)
	if err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}
	if r.ProjectName != "myproj" {
		t.Errorf("ProjectName = %q", r.ProjectName)
	}
	// 验证文件已创建
	readme, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil {
		t.Fatalf("README not created: %v", err)
	}
	if !strings.Contains(string(readme), "myproj") {
		t.Error("README should contain project name")
	}
	gi, err := os.ReadFile(filepath.Join(dest, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}
	if !strings.Contains(string(gi), "vendor/") {
		t.Error("Go .gitignore should include vendor/")
	}
}

func TestScaffoldProject_AllStacks(t *testing.T) {
	stacks := []TechStack{TechStackPython, TechStackKotlin, TechStackJS, TechStackGo}
	for _, s := range stacks {
		dest := filepath.Join(t.TempDir(), "proj")
		if _, err := ScaffoldProject(dest, "proj", s); err != nil {
			t.Errorf("ScaffoldProject(%s): %v", s, err)
		}
	}
}

func TestScaffoldProject_EmptyName(t *testing.T) {
	_, err := ScaffoldProject(t.TempDir(), "", TechStackGo)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestScaffoldProject_InvalidStack(t *testing.T) {
	_, err := ScaffoldProject(t.TempDir(), "proj", TechStack("rust"))
	if err == nil {
		t.Error("expected error for invalid stack")
	}
}

func TestIsValidTechStack(t *testing.T) {
	valid := []TechStack{TechStackPython, TechStackKotlin, TechStackJS, TechStackGo}
	invalid := []TechStack{TechStack("rust"), TechStack(""), TechStack("C++")}
	for _, s := range valid {
		if !IsValidTechStack(s) {
			t.Errorf("%s should be valid", s)
		}
	}
	for _, s := range invalid {
		if IsValidTechStack(s) {
			t.Errorf("%s should NOT be valid", s)
		}
	}
}
