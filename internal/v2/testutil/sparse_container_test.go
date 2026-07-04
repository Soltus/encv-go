package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSparseVirtualContainer_5x100MB(t *testing.T) {
	dir := t.TempDir()
	cfg := SparseContainerConfig{
		OutputDir:     dir,
		BaseName:      "small-test",
		FragmentCount: 5,
		FragmentSize:  100 * 1024 * 1024, // 100MB × 5 = 500MB
		ContainerType: 1,                  // video
	}

	res, err := WriteSparseVirtualContainer(cfg)
	if err != nil {
		t.Fatalf("write sparse container: %v", err)
	}

	// 1) 物理 main file size 应等于 virtual total (sparse)
	if res.PhysicalMain != res.VirtualTotal {
		t.Errorf("PhysicalMain=%d, want VirtualTotal=%d", res.PhysicalMain, res.VirtualTotal)
	}

	// 2) 实际物理占用 (blocks) 应远小于 virtual total
	if res.PhysicalUsed >= res.VirtualTotal/2 {
		t.Errorf("PhysicalUsed=%d, expected sparse (<< VirtualTotal=%d)", res.PhysicalUsed, res.VirtualTotal)
	}

	// 3) IsSparse 应为 true（100x 压缩比）
	if !res.IsSparse {
		t.Errorf("IsSparse=false, expected true; physical=%d, virtual=%d", res.PhysicalUsed, res.VirtualTotal)
	}

	// 4) 文件存在
	if _, err := os.Stat(res.MainFilePath); err != nil {
		t.Errorf("main file not exist: %v", err)
	}

	// 5) Header + manifest 可读且 CRC 正确
	if err := VerifySparseReadSafe(res.MainFilePath); err != nil {
		t.Errorf("verify sparse read: %v", err)
	}

	// 6) Cleanup 正确
	if err := CleanupSparseContainer(dir, "small-test"); err != nil {
		t.Errorf("cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "small-test.sccg")); !os.IsNotExist(err) {
		t.Errorf("main file not cleaned up")
	}
}

func TestWriteSparseVirtualContainer_100x128GB_sparse_physical_under_1MB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 100x128GB test in short mode")
	}
	dir := t.TempDir()
	cfg := DefaultSparseConfig(dir, "huge-test")

	res, err := WriteSparseVirtualContainer(cfg)
	if err != nil {
		t.Fatalf("write 100x128GB: %v", err)
	}

	// 声称 12.8TB
	const wantVirtual = int64(100) * 128 * 1024 * 1024 * 1024
	if res.VirtualTotal != wantVirtual {
		t.Errorf("VirtualTotal=%d, want %d", res.VirtualTotal, wantVirtual)
	}

	// 关键断言：实际物理占用 < 1MB
	const maxPhysicalBytes = int64(1024 * 1024) // 1MB
	if res.PhysicalUsed > maxPhysicalBytes {
		t.Errorf("PhysicalUsed=%d, expected < %d (sparse file failure)", res.PhysicalUsed, maxPhysicalBytes)
	}

	// Header 可读
	if err := VerifySparseReadSafe(res.MainFilePath); err != nil {
		t.Errorf("verify header: %v", err)
	}

	// 探测读 fragment 0
	probe, err := ReadSparseContainerEdgeProbe(res.MainFilePath, 0, cfg.FragmentSize)
	if err != nil {
		t.Fatalf("edge probe: %v", err)
	}
	if probe.BytesRead == 0 {
		t.Errorf("BytesRead=0, expected to read at least 1 byte")
	}
	if probe.HeapInUseKB > 50*1024 { // < 50MB
		t.Errorf("HeapInUseKB=%d, expected < 51200 (50MB)", probe.HeapInUseKB)
	}

	t.Logf("✅ 100x128GB sparse container: virtual=%d, physical=%d, isSparse=%v",
		res.VirtualTotal, res.PhysicalUsed, res.IsSparse)
	t.Logf("   edge probe fragment 0: seek=%dms, read=%dms, heapInUse=%dKB",
		probe.SeekDurationMs, probe.ReadDurationMs, probe.HeapInUseKB)
}

func TestDefaultSparseConfig_Defaults(t *testing.T) {
	cfg := DefaultSparseConfig("/tmp", "test")
	if cfg.FragmentCount != 100 {
		t.Errorf("FragmentCount=%d, want 100", cfg.FragmentCount)
	}
	if cfg.FragmentSize != 128*1024*1024*1024 {
		t.Errorf("FragmentSize=%d, want 128GB", cfg.FragmentSize)
	}
	if cfg.PhysicalChunkMB != 0 {
		t.Errorf("PhysicalChunkMB=%d, want 0", cfg.PhysicalChunkMB)
	}
}

func TestSparseResultJSON_Serialization(t *testing.T) {
	res := SparseResult{
		VirtualTotal:  1024,
		PhysicalMain:  1024,
		PhysicalUsed:  100,
		FragmentCount: 10,
		FragmentSize:  100,
		IsSparse:      true,
		MainFilePath:  "/tmp/test.sccg",
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"virtualTotalBytes":1024`) {
		t.Errorf("missing virtualTotalBytes in JSON: %s", b)
	}
	if !strings.Contains(string(b), `"isSparse":true`) {
		t.Errorf("missing isSparse in JSON: %s", b)
	}
}
