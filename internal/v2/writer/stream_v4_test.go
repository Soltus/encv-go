// internal/v2/writer/stream_v4_test.go
//
// 流式 v4 容器写入的回归锁。
//
// 这里守的不是"能写完"，而是四件容易悄悄退化、又不会报错的事：
//  1. 产物必须是主线 reader 能原样打开的容器（整块路径同源，不许漂移）
//  2. **不能**把整个文件攒在内存里（单次 Append 的体积必须有上界）
//  3. 每段必须换 nonce（CTR 复用 keystream = 二时间垫）
//  4. 开了 HMAC 就必须真的能挡住篡改（失败不解 CTR）
package writer

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"

	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

const streamTestPassword = "stream-test-password"

// streamFixture 组装一套流式写入的参数。
//
// salt/macSalt 都显式生成并由**同一个** salt 派生密钥：OpenV4Container 会按
// manifest 里的 salt/mac_salt 重新派生，只要这里派生不一致，reader 必然认为口令错。
func streamFixture(enableHMAC bool, segSize int64) (*MemSink, *V4StreamParams, []byte) {
	salt, err := crypto.GenerateSalt_v2(16)
	if err != nil {
		panic(err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		panic(err)
	}
	dek := make([]byte, crypto.KeySizeForCipherMode_v4(crypto.CipherModeAES128CTR))
	if _, err = rand.Read(dek); err != nil {
		panic(err)
	}
	hint, err := crypto.CalculatePasswordHint(streamTestPassword, salt)
	if err != nil {
		panic(err)
	}

	var macKey []byte
	if enableHMAC {
		macKey = crypto.DeriveMACKey(streamTestPassword, macSalt)
	}

	// 分层密钥：随机 DEK 由口令派生的 KEK 封装。
	// ⚠️ 少了这一环 reader 会fallback到"password+salt 直接派生"，解出同样长度的一堆乱码 ——
	// 而且是"看着像能读、内容全错"，正是不加 MAC 时最难发现的一类问题。
	wrapped, err := crypto.WrapDEK(dek, crypto.DeriveKEK(streamTestPassword, salt), nil)
	if err != nil {
		panic(err)
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "stream-test-container",
		ContainerType: "text",
		OriginalName:  "stream.bin",
		WrappedDEK:    wrapped,
	}
	// ⚠️ mac_salt 必须在这里显式写进 manifest：让 writer 事后再"补一个"的话，
	// 加密用一个 mac_key、校验用另一个 —— MAC 会永远验不过。
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	params := &V4StreamParams{
		IsMain:        true,
		ContainerType: types.ContainerTypeText,
		IDType:        types.IDType_Raw,
		IDData:        []byte("0123456789abcdef"),
		PasswordHint:  hint,
		Manifest:      manifest,
		Key:           dek,
		MacKey:        macKey,
		CipherMode:    uint16(crypto.CipherModeAES128CTR),
		EnableHMAC:    enableHMAC,
		SegmentSize:   segSize,
		OnManifest: func(m *types.Manifest_v4, plainSize int64, plainMD5 string) error {
			kvi, err := json.Marshal(map[string]string{
				"salt_base64": base64.StdEncoding.EncodeToString(salt),
				"iv_base64":   base64.StdEncoding.EncodeToString(make([]byte, crypto.IVSize_v2)),
			})
			if err != nil {
				return err
			}
			m.KVI = kvi
			return nil
		},
	}
	return NewMemSink(), params, dek
}

// openStreamContainer 用主线 reader 打开内存里的容器并读出全部明文。
func openStreamContainer(t *testing.T, data []byte) []byte {
	t.Helper()
	src := containerhandle.NewBytesSource(data, "stream-test")
	info, err := reader.OpenV4ContainerFromSource(src, streamTestPassword)
	if err != nil {
		t.Fatalf("主线 reader 打不开流式产物：%v", err)
	}
	r, err := reader.NewSegmentSeekableReader(info, "")
	if err != nil {
		t.Fatalf("创建 seekable reader 失败：%v", err)
	}
	defer r.Close()
	plain, err := readAll(r)
	if err != nil {
		t.Fatalf("读明文失败：%v", err)
	}
	return plain
}

func readAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

func pattern(size int) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = byte(i%251) ^ byte(i>>8)
	}
	return out
}

// TestV4StreamWriter_RoundTrip 流式写入的产物必须能被主线 reader 逐字节解开。
//
// 刻意用不规则的分块（含 1 字节）写入：早期版本在这里会算出错误的段边界。
func TestV4StreamWriter_RoundTrip(t *testing.T) {
	const total = 300_000
	const segSize = int64(4096)
	plain := pattern(total)

	sink, params, _ := streamFixture(false, segSize)
	w, err := NewV4StreamWriter(sink, params)
	if err != nil {
		t.Fatalf("NewV4StreamWriter: %v", err)
	}

	written := 0
	for _, size := range []int{1, 3, 511, 4096, 65_536, 1} {
		for written+size <= total {
			n, err := w.Write(plain[written : written+size])
			if err != nil {
				t.Fatalf("Write(%d): %v", size, err)
			}
			written += n
		}
	}
	if written != total {
		t.Fatalf("written = %d, want %d", written, total)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	wantSegs := int((total + int(segSize) - 1) / int(segSize))
	if w.SegmentCount() != wantSegs {
		t.Errorf("SegmentCount = %d, want %d", w.SegmentCount(), wantSegs)
	}
	if got := openStreamContainer(t, sink.Bytes()); !bytes.Equal(got, plain) {
		t.Errorf("往返不一致：got %d 字节, want %d", len(got), len(plain))
	}
}

// TestV4StreamWriter_DoesNotBufferWholeFile 锁定"流式"这件事本身。
//
// 只看"能写完"是不够的：实现完全可以在 Write 里把明文全攒起来、Close 时一次性
// Append —— 产物一样是对的，但内存行为和整块路径没区别，浏览器照样会被打死。
// 这里断言两件事：① 中途就有多个 Append；② 单次 Append 的体积有常数上界。
func TestV4StreamWriter_DoesNotBufferWholeFile(t *testing.T) {
	plain := pattern(1 << 20) // 1MB
	const segSize = int64(4096)

	sink, params, _ := streamFixture(false, segSize)
	w, err := NewV4StreamWriter(sink, params)
	if err != nil {
		t.Fatalf("NewV4StreamWriter: %v", err)
	}

	maxAppend := 0
	appearsEarly := false
	step := 8192
	for off := 0; off < len(plain); off += step {
		if _, err := w.Write(plain[off : off+step]); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if len(sink.pieces) > 3 {
			appearsEarly = true
		}
		for _, p := range sink.pieces {
			if len(p) > maxAppend {
				maxAppend = len(p)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if !appearsEarly {
		t.Error("数据直到写入后期才产出 —— 这不是流式实现")
	}
	// 段环境影响评价：一段 = SegmentationHeader(34) + nonce(16) + 密文(<=segSize)
	bound := int(segSize) + types.SegmentHeaderSize + crypto.IVSize_v2 + 16
	if maxAppend > bound {
		t.Errorf("单次 Append = %d 字节，超过上界 %d（流式写入的内存只应与 SegmentSize 有关）", maxAppend, bound)
	}
}

// TestV4StreamWriter_EmptyInput 空输入也要产出合法容器（一段空段）。
func TestV4StreamWriter_EmptyInput(t *testing.T) {
	sink, params, _ := streamFixture(false, 4096)
	w, err := NewV4StreamWriter(sink, params)
	if err != nil {
		t.Fatalf("NewV4StreamWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if w.SegmentCount() != 1 {
		t.Errorf("SegmentCount = %d, want 1（空文件也要有一段，否则 reader 无从读起）", w.SegmentCount())
	}
	if got := openStreamContainer(t, sink.Bytes()); len(got) != 0 {
		t.Errorf("空容器解出 %d 字节明文，want 0", len(got))
	}
}

// TestV4StreamWriter_SegmentNoncesAreUnique CTR 下复用 nonce 等同于泄露明文异或。
func TestV4StreamWriter_SegmentNoncesAreUnique(t *testing.T) {
	plain := pattern(40_960)
	sink, params, _ := streamFixture(false, 4096)
	w, err := NewV4StreamWriter(sink, params)
	if err != nil {
		t.Fatalf("NewV4StreamWriter: %v", err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	seen := map[string]bool{}
	for _, seg := range params.Manifest.Segments {
		if seg.Nonce == "" {
			t.Fatalf("segment %s 的 nonce 是空的（明文段？）", seg.ID)
		}
		if seen[seg.Nonce] {
			t.Errorf("nonce 重复：%s（同一把密钥 + 同一个 nonce = 二时间垫）", seg.Nonce)
		}
		seen[seg.Nonce] = true
	}
}

// TestV4StreamWriter_HMAC 开了 MAC：正常容器可读，改一个密文字节必须报错。
func TestV4StreamWriter_HMAC(t *testing.T) {
	plain := pattern(9_999)
	sink, params, _ := streamFixture(true, 4096)
	w, err := NewV4StreamWriter(sink, params)
	if err != nil {
		t.Fatalf("NewV4StreamWriter: %v", err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	container := sink.Bytes()
	if got := openStreamContainer(t, container); !bytes.Equal(got, plain) {
		t.Error("启用 HMAC 后往返不一致")
	}

	// 篡改密文：往第一段密文中间翻一个 bit。CTR 没有 MAC 时这只会污染 1 字节，
	// 开了 MAC 就必须直接拒读（reader 的规定是"先验 MAC，不解 CTR"）。
	tampered := append([]byte(nil), container...)
	tamperAt := types.EnvelopeHeaderSize_v4 + types.SegmentHeaderSize + crypto.IVSize_v2 + 100
	tampered[tamperAt] ^= 0x01

	src := containerhandle.NewBytesSource(tampered, "tampered")
	info, err := reader.OpenV4ContainerFromSource(src, streamTestPassword)
	if err != nil {
		t.Fatalf("打开被篡改的容器失败（manifest 本身没坏）：%v", err)
	}
	r, err := reader.NewSegmentSeekableReader(info, "")
	if err != nil {
		t.Fatalf("创建 seekable reader 失败：%v", err)
	}
	defer r.Close()
	if _, err := readAll(r); err == nil {
		t.Error("密文被篡改却没报错 —— MAC 没生效或根本没写入（EnableHMAC 与 manifest.MacSalt 必须配套）")
	}

	// 对照组：**不开** MAC 时同样的篡改必须是"静默"的（只污染 1 字节）。
	// 这条对照证明上面那条失败确实是 MAC 拦下来的，而不是别的什么东西在报错。
	sink2, params2, _ := streamFixture(false, 4096)
	w2, err := NewV4StreamWriter(sink2, params2)
	if err != nil {
		t.Fatalf("NewV4StreamWriter(对照组): %v", err)
	}
	if _, err := w2.Write(plain); err != nil {
		t.Fatalf("Write(对照组): %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("Close(对照组): %v", err)
	}
	tampered2 := append([]byte(nil), sink2.Bytes()...)
	tampered2[tamperAt] ^= 0x01
	got := openStreamContainer(t, tampered2)
	if len(got) != len(plain) {
		t.Fatalf("对照组长度异常：%d vs %d", len(got), len(plain))
	}
	diff := 0
	for i := range got {
		if got[i] != plain[i] {
			diff++
		}
	}
	if diff != 1 {
		t.Errorf("对照组篡改后差异字节数 = %d，want 1（CTR 无认证：翻 1 bit 只脏 1 字节且不报错）", diff)
	}
}

// TestV4StreamWriter_Lifecycle 写后再写 / 重复收尾必须显式报错，而不是静默产出半成品。
func TestV4StreamWriter_Lifecycle(t *testing.T) {
	sink, params, _ := streamFixture(false, 4096)
	w, err := NewV4StreamWriter(sink, params)
	if err != nil {
		t.Fatalf("NewV4StreamWriter: %v", err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := w.Write([]byte("more")); err == nil {
		t.Error("Close 之后还能 Write —— 会静默写出无人负责的尾随数据")
	}
	if err := w.Close(); err == nil {
		t.Error("Close 可以重复调用 —— head/tail 会被覆盖两次")
	}
}

// TestV4StreamWriter_KeyLengthMismatch CipherMode 与密钥长度必须对得上。
func TestV4StreamWriter_KeyLengthMismatch(t *testing.T) {
	_, params, _ := streamFixture(false, 4096)
	params.CipherMode = uint16(crypto.CipherModeAES256CTR) // 声明 256，密钥还是 16 字节
	if _, err := NewV4StreamWriter(NewMemSink(), params); err == nil {
		t.Error("CipherMode 与密钥长度不一致却放行 —— 这种容器在别处会读到 CRC 错误的密文")
	}
}
