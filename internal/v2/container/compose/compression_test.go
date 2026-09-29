package compose_test

// 压缩契约（2026-09-30 从"隐式硬编码"改成显式 + 可执行）：
//
//   - compose 这一层（wasm 预览页与主应用流式入口共用）**两条路都不支持压缩**；
//   - 传 zstd 必须**报错**，不能静默忽略 —— 请求了压缩却拿到没压缩的容器不会失败、
//     也不会告警，只有体积不对，等发现时数据已经按"压缩过"的预期流转了；
//   - 但"写入侧不支持"**不等于"压缩通道坏了"**：插件/存量路径产出的 seekable zstd
//     段仍然要能解（最后一条用例锁这个）。

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/container/compose"
	"github.com/Soltus/encv-go/internal/v2/crypto"
)

const compressionPassword = "compression-password"

func TestCompose_RejectsCompression(t *testing.T) {
	dir := t.TempDir()

	// ① 流式入口
	out, err := os.Create(filepath.Join(dir, "stream.sccgt"))
	if err != nil {
		t.Fatalf("创建输出文件失败：%v", err)
	}
	defer out.Close()
	_, err = compose.NewStreamEncryptor(
		compose.Options{Password: compressionPassword, Compression: crypto.CompressionModeZstd},
		&fileSink{f: out},
	)
	if !errors.Is(err, compose.ErrCompressionUnsupported) {
		t.Fatalf("流式入口请求 zstd 必须返回 ErrCompressionUnsupported，实际：%v", err)
	}

	// ② 整块入口（与流式同构，所以也拒绝）
	var buf bytes.Buffer
	err = compose.EncryptBytes([]byte("hello"), compose.Options{
		Password:    compressionPassword,
		Compression: crypto.CompressionModeZstd,
	}, &buf)
	if !errors.Is(err, compose.ErrCompressionUnsupported) {
		t.Fatalf("整块入口请求 zstd 必须返回 ErrCompressionUnsupported，实际：%v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("被拒绝时不应产出任何字节，实际写了 %d 字节", buf.Len())
	}
}

// 契约的另一半：不能误伤 —— 零值与显式 none 都要照常产出。
func TestCompose_AllowsNoCompression(t *testing.T) {
	for _, mode := range []string{"", crypto.CompressionModeNone} {
		var buf bytes.Buffer
		if err := compose.EncryptBytes([]byte("hello"), compose.Options{Password: compressionPassword, Compression: mode}, &buf); err != nil {
			t.Fatalf("Compression=%q 不应被拒绝：%v", mode, err)
		}
		if buf.Len() == 0 {
			t.Fatalf("Compression=%q 没有产出容器", mode)
		}
	}
}

// 写入侧不支持压缩，但**压缩通道本身必须可用**：插件/存量路径产出的
// seekable zstd 段（含 seek table）仍然要能原样解出来。
//
// 这条用例的意义是把"compose 不支持压缩"和"zstd 坏了"区分开：
// 前者是编排层的能力边界，后者会让存量压缩容器全部打不开。
func TestZstdSegment_RoundTripStillWorks(t *testing.T) {
	ctx, err := crypto.PrepareEncryptionContext(compressionPassword)
	if err != nil {
		t.Fatalf("准备加密上下文失败：%v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("生成 mac salt 失败：%v", err)
	}
	macKey := crypto.DeriveMACKey(compressionPassword, macSalt)

	plain := bytes.Repeat([]byte("compress-me "), 4096) // 够大才有压缩意义
	seg, err := crypto.EncryptSegment(plain, ctx.DEK, macKey, 0, crypto.CompressionModeZstd)
	if err != nil {
		t.Fatalf("zstd 段加密失败：%v", err)
	}
	if len(seg.SeekTable) == 0 {
		t.Fatal("zstd 段必须带 seek table（seekable zstd 靠它随机访存）")
	}
	if len(seg.EncryptedData) >= len(plain) {
		t.Fatalf("这段明文应当能被压小：明文 %d → 密文 %d 字节", len(plain), len(seg.EncryptedData))
	}

	back, err := crypto.DecryptSegment(seg.EncryptedData, seg.Nonce, ctx.DEK, macKey, seg.HMAC[:], crypto.CompressionModeZstd, seg.SeekTable)
	if err != nil {
		t.Fatalf("zstd 段解密失败：%v", err)
	}
	if !bytes.Equal(back, plain) {
		t.Fatalf("zstd 往返不一致：明文 %d → 解出 %d 字节", len(plain), len(back))
	}
}
