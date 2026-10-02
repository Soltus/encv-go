package server

// servingdir_init_order_test.go —— 回归锁：**servingDir 必须在 NewServer 里就就绪**
//
// 历史缺陷（2026-10-02 真机端到端 scripts/emu-peerlink-e2e.sh 抓出，属环境/顺序类 bug）：
//   servingDir 原本只在 Start() 里赋值，而 NewServer 里**已经在用它** ⇒ 拿到空字符串：
//     · FTS5 启动后台建索引 ⇒ "FTS5 no entries to index" ⇒ 本地全文搜索 0 命中
//     · mount registry bootstrap ⇒ root 退化成 cwd（filepath.Abs("") = cwd）
//     · FTSRebuilder 带着空 dir 也建不出来
//   表现（真机日志）：`servingDir= FTS5 no entries to index`、`totalFiles=0`，
//   而同一目录的文件用 /api/files 列举却完全正常 ⇒ 极易被误判成"索引坏了"。
//
// 本用例锁死：**不调 Start()** 也要能拿到配置里的 servingDir（顺序错误会立刻变红）。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/types"
)

func TestNewServer_ServingDirReadyBeforeStart(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "encv-servingdir-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	old := os.Getenv("ENCV_MOUNTS_FILE")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)
	defer os.Setenv("ENCV_MOUNTS_FILE", old)

	cfg := &config.Config{
		Password: "test-password",
		Server: types.HttpServer{
			Dir: tmpDir,
		},
		Log:            types.LogConfig{Level: "error"},
		PluginSettings: map[string]json.RawMessage{},
	}

	s := NewServer(config.NewContext(context.Background(), cfg), "")

	// macOS/容器里 TMPDIR 可能是 symlink（/tmp → /private/tmp）：两边都 EvalSymlinks 再比
	want, err1 := filepath.EvalSymlinks(tmpDir)
	got, err2 := filepath.EvalSymlinks(s.servingDir)
	if err1 != nil || err2 != nil {
		t.Fatalf("EvalSymlinks: %v / %v", err1, err2)
	}
	if got != want {
		t.Fatalf("NewServer 结束后 servingDir 应是 %q, got %q（空值说明又退化成了 Start() 才赋值）", want, got)
	}
}
