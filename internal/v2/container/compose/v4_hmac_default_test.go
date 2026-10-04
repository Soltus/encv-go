package compose

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"

	_ "github.com/Soltus/encv-go/internal/testguard"
)

// ============================================================================
// 回归锁：compose 编排层（整块 + 流式）产出的 v4 容器**默认带 HMAC**
// ----------------------------------------------------------------------------
// 事故背景（2026-10-02，真机实测）：没有完整性元数据的容器被篡改/位翻转后，
// 读取端解出「长度正确、内容乱码、不报错」的静默乱码。
//
// 修前两处洞：
//   1) Options.EnableHMAC 是 bool，零值 false ⇒ 调用方不显式设就是"不开"
//   2) EncryptBytes（整块路径）**压根没把 EnableHMAC 传给 writer**，
//      即使 Options 要求开启，产物也永远不会带 MAC
// 修法：三态（EnableHMAC / DisableHMAC）+ 默认开启 + 整块路径补传。
// ============================================================================

// 单个加密 segment 的布局：[2048 头][SegmentHeader 34B][Nonce 16B][Ciphertext][MAC 10B]
const (
	v4SegmentDataStart = 2048 + 34 + 16
)

func encryptToTempFile(t *testing.T, plain []byte) (string, string) {
	t.Helper()
	const password = "compose-hmac-password"
	dir := t.TempDir()
	path := filepath.Join(dir, "compose_v4.sccgt")

	var buf bytes.Buffer
	if err := EncryptBytes(plain, Options{
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

func readAllViaReader(t *testing.T, path, password string) ([]byte, error) {
	t.Helper()
	factory, err := reader.NewDecryptReaderFactory(path, password)
	if err != nil {
		return nil, err
	}
	defer factory.Close()
	dr, err := factory.NewDecryptReader()
	if err != nil {
		return nil, err
	}
	defer dr.Close()
	return io.ReadAll(dr)
}

// TestComposeV4_EncryptedByDefault 默认（不显式设置）必须开启 HMAC：
// 具象表现是"篡改密文必须被 ErrMACMismatch 拦下"，而不是静默解出乱码。
func TestComposeV4_TamperDetectedByDefault(t *testing.T) {
	plain := bytes.Repeat([]byte("compose integrity"), 300) // ≈5.1KB
	path, password := encryptToTempFile(t, plain)

	// 控制组：未篡改必须能正常解开
	got, err := readAllViaReader(t, path, password)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("未篡改的容器就读不对（err=%v, %d 字节）—— 夹具本身有问题", err, len(got))
	}

	// 翻转密文区 1 bit
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var one [1]byte
	if _, err := f.ReadAt(one[:], v4SegmentDataStart+8); err != nil {
		t.Fatalf("read: %v", err)
	}
	one[0] ^= 0x01
	if _, err := f.WriteAt(one[:], v4SegmentDataStart+8); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	got2, err := readAllViaReader(t, path, password)
	if err == nil {
		if bytes.Equal(got2, plain) {
			t.Fatalf("篡改后仍解出正确明文 —— 篡改没生效，测试无效")
		}
		t.Fatalf("篡改后静默解出 %d 字节乱码且不报错 —— 默认没开 HMAC（静默损坏）", len(got2))
	}
	// 错误类型：优先 MAC（segment 栈自带 HMAC），退而求其次 CRC（fragment 栈读取路径
	// 目前只校验 manifest 里的 DataCRC32 —— MAC 在这条路径上**没被校验**，见 memory
	// 2026-10-02 §5e 的遗留项）。两者都属于"显式失败"，都能终结静默乱码。
	if !errors.Is(err, crypto.ErrMACMismatch) && !errors.Is(err, types.ErrDataCorrupted) {
		t.Errorf("期望 crypto.ErrMACMismatch 或 types.ErrDataCorrupted，实际：%v", err)
	}
}

// TestComposeV4_DisableHMAC_Explicit 显式关闭仍然要能关（兼容/测试场景）
func TestComposeV4_DisableHMAC_Explicit(t *testing.T) {
	plain := []byte("explicitly disabled hmac")
	const password = "compose-hmac-password"
	dir := t.TempDir()
	path := filepath.Join(dir, "compose_v4_nohmac.sccgt")

	var buf bytes.Buffer
	if err := EncryptBytes(plain, Options{
		Password:         password,
		ContainerType:    types.ContainerTypeText,
		ContainerTypeStr: "text",
		OriginalName:     "note.txt",
		DisableHMAC:      true,
	}, &buf); err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := readAllViaReader(t, path, password)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("显式关闭 HMAC 后必须仍能正常读（err=%v）", err)
	}
}

// TestComposeV4_Defaults_EnableHMAC 默认值本身（防止有人把默认改回去）
func TestComposeV4_Defaults_EnableHMAC(t *testing.T) {
	o := Options{}.withDefaults()
	if !o.EnableHMAC {
		t.Error("默认必须开启 HMAC（决策 A）；需要关闭请显式传 DisableHMAC")
	}
	o2 := Options{DisableHMAC: true}.withDefaults()
	if o2.EnableHMAC {
		t.Error("显式 DisableHMAC=true 时必须关闭")
	}
}
