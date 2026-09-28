// internal/v2/reader/factory_nonce_stack_test.go
//
// 回归锁：v4 **segment 栈**（wasm / 流式 writer 产出，每段独立随机 nonce）的容器，
// 必须能被 **fragment 读取栈**（CLI 插件路径走的就是这条）逐字节解开。
//
// 背景（真实事故）：
//
//	浏览器里 `encryptBegin/encryptWrite`（以及早前的 `encryptBytes`）产出的容器，
//	用 `encv decrypt-v2` 去解会得到「文件大小正确、内容全是乱码、**而且不报错**」的结果。
//	根因是两套内容组织层的 keystream 模型不同：
//	  - v4 segment 栈：每段一个随机 nonce，keystream 每段重置（CTR 本就该这么用）
//	  - fragment 栈：整条逻辑流共用 KVI 里那一个 iv，按全局偏移推进计数器
//	密钥对、偏移对，只有 keystream 不对 —— CTR 没有认证标签，所以静默出错。
//
// 解法不是"拒绝这类容器"，而是把每段的 nonce **带到分片上**
// （types.Fragment.Nonce ← container/handle.AdaptV4ToV2），读取端按段重置 keystream。
// 这里锁两件事：
//  1. 多段 + 每段独立 nonce → 顺序读与随机读都必须逐字节正确
//  2. 老容器（nonce 为空 / 整条流一条 keystream）行为不变
package reader

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"

	_ "github.com/Soltus/encv-go/internal/testguard"
)

const nonceGuardPassword = "nonce-stack-password"

// writeNonceFixture 写一个 v4 段栈容器到临时文件，返回路径。
//
// sharedNonce=false → 每段随机 nonce（真实加密就是这样，也是本文件要守的情况）
// sharedNonce=true  → 所有段复用同一个 nonce（老写入路径的形态）
func writeNonceFixture(t *testing.T, plain []byte, segSize int, sharedNonce bool, kviIV []byte) string {
	t.Helper()
	// 适配层/工厂靠扩展名认容器类型，给个 text 容器的后缀
	path := filepath.Join(t.TempDir(), "nonce-fixture.sccgt")

	salt, err := crypto.GenerateSalt_v2(16)
	if err != nil {
		t.Fatalf("salt: %v", err)
	}
	// 分层密钥：随机 DEK，由口令派生的 KEK 封装。
	// 少了它 deriveKeyAndIV 会走「password+salt 直接派生」的老路径（长度也不同），
	// 又是一例"长度对、内容是乱码"的坑。
	dek := make([]byte, 16)
	if _, err := rand.Read(dek); err != nil {
		t.Fatalf("rand: %v", err)
	}
	wrapped, err := crypto.WrapDEK(dek, crypto.DeriveKEK(nonceGuardPassword, salt), nil)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}
	kvi, err := json.Marshal(map[string]string{
		"salt_base64": crypto.Base64Encode_v2(salt),
		"iv_base64":   crypto.Base64Encode_v2(kviIV),
	})
	if err != nil {
		t.Fatalf("marshal kvi: %v", err)
	}

	var results []*crypto.SegmentEncryptionResult
	var segIDs []string
	for i, off := 0, 0; ; i++ {
		end := off + segSize
		if end > len(plain) {
			end = len(plain)
		}
		res, err := crypto.EncryptSegment(plain[off:end], dek, nil, uint32(i), crypto.CompressionModeNone)
		if err != nil {
			t.Fatalf("EncryptSegment: %v", err)
		}
		if sharedNonce {
			res.Nonce = append([]byte(nil), kviIV...)
		}
		results = append(results, res)
		segIDs = append(segIDs, fmt.Sprintf("seg-%d", i))
		off = end
		if off >= len(plain) {
			break
		}
	}

	segments := make([]types.Segment_v4, len(segIDs))
	for i, id := range segIDs {
		segments[i] = types.Segment_v4{ID: id}
	}

	var buf bytes.Buffer
	if err := writer.WriteV4ContainerTo(&buf, &writer.V4WriteParams{
		IsMain:        true,
		ContainerType: types.ContainerTypeText,
		IDType:        types.IDType_Raw,
		IDData:        []byte("0123456789abcdef"),
		Manifest: &types.Manifest_v4{
			Version:       4,
			ContainerID:   "nonce-fixture",
			ContainerType: "text",
			Segments:      segments,
			Playlists:     map[string][]string{"default": segIDs},
			KVI:           kvi,
			WrappedDEK:    wrapped,
		},
		SegmentResults: results,
	}); err != nil {
		t.Fatalf("WriteV4ContainerTo: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestDecryptReaderFactory_PerSegmentNonce 每段独立 nonce → 顺序读与随机读都要逐字节正确。
func TestDecryptReaderFactory_PerSegmentNonce(t *testing.T) {
	plain := bytes.Repeat([]byte("wasm-streamed-attachment"), 400) // ≈10.4KB → 多段
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("rand: %v", err)
	}
	path := writeNonceFixture(t, plain, 1024, false, iv)

	factory, err := NewDecryptReaderFactory(path, nonceGuardPassword)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory: %v", err)
	}
	defer factory.Close()

	r, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader: %v", err)
	}
	defer r.Close()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("读明文失败：%v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("顺序读解出 %d 字节，内容与明文不一致（per-fragment nonce 没生效 → 静默乱码）", len(got))
	}

	// 随机读：跨段 seek 到中段，keystream 必须按段内偏移重新对齐
	seeker, ok := r.(io.Seeker)
	if !ok {
		t.Fatal("该 reader 不支持 Seek，无法验证随机读")
	}
	mid := len(plain) / 2
	if _, err := seeker.Seek(int64(mid), io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	window := make([]byte, 128)
	if _, err := io.ReadFull(r, window); err != nil {
		t.Fatalf("随机读失败：%v", err)
	}
	if !bytes.Equal(window, plain[mid:mid+len(window)]) {
		t.Error("随机读的内容与明文不一致（段内偏移没被正确换算成 keystream 偏移）")
	}
}

// TestDecryptReaderFactory_LegacySingleKeystream 老容器（整条流一条 keystream）行为必须不变。
func TestDecryptReaderFactory_LegacySingleKeystream(t *testing.T) {
	original := make([]byte, 512)
	for i := range original {
		original[i] = byte(i)
	}
	path, password := createV4PluginPathContainer(t, types.ContainerTypeText, original)

	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory: %v", err)
	}
	defer factory.Close()

	r, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader: %v", err)
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("读明文失败：%v", err)
	}
	if !bytes.Equal(got, original) {
		t.Error("老容器（单一 keystream）解密结果变了 —— per-fragment nonce 改动误伤了旧路径")
	}
}
