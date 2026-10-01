package server

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/container/compose"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"

	_ "github.com/Soltus/encv-go/internal/testguard"
)

// ============================================================================
// 回归锁：吐第一个字节**之前**的完整性预校验（serveEncryptedFile 的 3.5 步）
// ----------------------------------------------------------------------------
// 为什么还要预校验：分块 CRC 只能在"读到损坏块"时中止 —— 那时 200 + Content-Length
// 已经发出去了，客户端拿到的是截断流（真机：播到一半卡住，没人知道是文件坏了）。
// 预校验 ⇒ 422 data_corrupted，前端可以直接提示"文件数据已损坏"。
//
// 触发条件（严格限制，避免拖慢首字节）：
//   仅本地容器 + 不带 Range + 大小 ≤ 64MB + 容器真的带 CRC 元数据
// ============================================================================

// 单个加密 segment 布局：[2048 头][SegmentHeader 34B][Nonce 16B][Ciphertext][MAC 10B]
const segmentCipherStart = 2048 + 34 + 16

func writeComposeContainer(t *testing.T, plain []byte) (string, string) {
	t.Helper()
	const password = "serve-integrity-password"
	dir := t.TempDir()
	path := filepath.Join(dir, "serve_v4.sccgt")

	var buf bytes.Buffer
	if err := compose.EncryptBytes(plain, compose.Options{
		Password:         password,
		ContainerType:    types.ContainerTypeText,
		ContainerTypeStr: "text",
		OriginalName:     "note.txt",
	}, &buf); err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path, password
}

// TestVerifyContainerIntegrity_Healthy 正常容器必须放行（不能误伤）
func TestVerifyContainerIntegrity_Healthy(t *testing.T) {
	path, password := writeComposeContainer(t, bytes.Repeat([]byte("healthy"), 500))
	factory, err := reader.NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory: %v", err)
	}
	defer factory.Close()

	if err := verifyContainerIntegrity(factory, 8<<20); err != nil {
		t.Errorf("正常容器被误判为损坏：%v", err)
	}
}

// TestVerifyContainerIntegrity_Tampered 篡改必须被拦下（这是本机制存在的意义）
func TestVerifyContainerIntegrity_Tampered(t *testing.T) {
	path, password := writeComposeContainer(t, bytes.Repeat([]byte("tampered"), 500))

	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var one [1]byte
	if _, err := f.ReadAt(one[:], segmentCipherStart+8); err != nil {
		t.Fatalf("read: %v", err)
	}
	one[0] ^= 0x01
	if _, err := f.WriteAt(one[:], segmentCipherStart+8); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	factory, err := reader.NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory: %v", err)
	}
	defer factory.Close()

	err = verifyContainerIntegrity(factory, 8<<20)
	if err == nil {
		t.Fatal("篡改后的容器竟然通过了预校验 ⇒ 客户端仍会收到损坏数据")
	}
	if !errors.Is(err, types.ErrDataCorrupted) {
		t.Errorf("期望 types.ErrDataCorrupted（上层映射 422），实际：%v", err)
	}
}

// TestShouldPreVerify 触发条件（别把代价转嫁给大文件/拖动请求）
func TestShouldPreVerify(t *testing.T) {
	// 全量小文件 → 校验
	if !shouldPreVerify(&http.Request{Header: http.Header{}}, 8<<20) {
		t.Error("不带 Range 的小文件应当预校验")
	}
	// 带 Range（拖动/续传）→ 不校验（交给分块 CRC）
	r := &http.Request{Header: http.Header{}}
	r.Header.Set("Range", "bytes=0-1023")
	if shouldPreVerify(r, 8<<20) {
		t.Error("带 Range 的请求不应预校验（为 1KB 读整片不划算）")
	}
	// 超过阈值 → 不校验（首字节延迟）
	if shouldPreVerify(&http.Request{Header: http.Header{}}, 200<<20) {
		t.Error("超过 64MB 的文件不应预校验")
	}
	// 0 长度 → 不校验
	if shouldPreVerify(&http.Request{Header: http.Header{}}, 0) {
		t.Error("0 长度不应预校验")
	}
}
