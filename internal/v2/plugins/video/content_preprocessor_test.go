package video

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureOutputDir_Empty(t *testing.T) {
	p := &VideoContentPreprocessor{outputDir: ""}
	err := p.ensureOutputDir()
	if err == nil {
		t.Fatal("expected error for empty outputDir, got nil")
	}
	if !strings.Contains(err.Error(), "outputDir is empty") {
		t.Fatalf("unexpected error message: %s", err)
	}
}

func TestEnsureOutputDir_PathIsFileNotDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not_a_dir")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	p := &VideoContentPreprocessor{outputDir: filePath}
	err := p.ensureOutputDir()
	if err == nil {
		t.Fatal("expected error when outputDir is a file, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error message: %s", err)
	}
}

func TestEnsureOutputDir_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	p := &VideoContentPreprocessor{outputDir: tmpDir}
	if err := p.ensureOutputDir(); err != nil {
		t.Fatalf("expected nil for existing dir, got: %v", err)
	}
}

func TestEnsureOutputDir_DoesNotExist_CreatesIt(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "nested", "new", "dir")

	p := &VideoContentPreprocessor{outputDir: newDir}
	if err := p.ensureOutputDir(); err != nil {
		t.Fatalf("ensureOutputDir failed: %v", err)
	}

	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("created dir does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("path exists but is not a directory")
	}
}

func TestEnsureOutputDir_CreateTempAfterEnsure(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "deep", "output")

	p := &VideoContentPreprocessor{outputDir: nestedDir}

	if err := p.ensureOutputDir(); err != nil {
		t.Fatalf("ensureOutputDir failed: %v", err)
	}

	tempFile, err := os.CreateTemp(p.outputDir, "encv-pre-*.mp4")
	if err != nil {
		t.Fatalf("CreateTemp failed after ensureOutputDir: %v", err)
	}
	tempFile.Close()
	os.Remove(tempFile.Name())
}

func TestOutputDirSetBeforePreprocess(t *testing.T) {
	tmpDir := t.TempDir()

	plugin := &VideoPlugin{}
	plugin.SetOutputDir(tmpDir)

	preprocessor := plugin.GetContentPreprocessor()
	if preprocessor == nil {
		t.Fatal("GetContentPreprocessor returned nil")
	}

	vcp, ok := preprocessor.(*VideoContentPreprocessor)
	if !ok {
		t.Fatalf("expected *VideoContentPreprocessor, got %T", preprocessor)
	}

	if vcp.outputDir == "" {
		t.Fatal("outputDir is empty after SetOutputDir + GetContentPreprocessor")
	}
	if vcp.outputDir != tmpDir {
		t.Errorf("outputDir mismatch: got %q, want %q", vcp.outputDir, tmpDir)
	}

	if err := vcp.ensureOutputDir(); err != nil {
		t.Fatalf("ensureOutputDir should succeed after SetOutputDir: %v", err)
	}
}
