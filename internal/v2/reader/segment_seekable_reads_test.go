// internal/v2/reader/segment_seekable_reads_test.go
//
// 回归锁：**打开一个大容器不应该把每个段的段头都读一遍**。
//
// 背景：段头（34B）分散在整个容器里，逐段读一遍等于随机读遍整个文件。
// 字节来自内存时无所谓；来自网络（HTTP Range / 远端源）时就是上千个 Round Trip ——
// 实测 1GB/1MB 段的容器打开要 1024 个 Range 请求。
// SegmentSeekableReader 因此加了 samplePlainSizes（抽样段头推算各段明文长度）。
//
// 这里锁两件事：
//  1. 构造 SegmentSeekableReader 的随机读次数**与段数无关**（抽样命中）
//  2. 抽出来的长度必须与逐段读（原路径）完全一致 —— 不能为了省 IO 算错
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

// countingSource 统计底层被随机读的次数（模拟"每次随机读 = 一次网络往返"）。
type countingSource struct {
	data  []byte
	off   int64
	reads int
}

func (c *countingSource) ReadAt(p []byte, off int64) (int, error) {
	c.reads++
	if off >= int64(len(c.data)) {
		return 0, io.EOF
	}
	n := copy(p, c.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// Read/Seek/Close：SegmentSeekableReader 的 openContainerFile 要求 Reader+Seeker+Closer
func (c *countingSource) Read(p []byte) (int, error) {
	if c.off >= int64(len(c.data)) {
		return 0, io.EOF
	}
	n := copy(p, c.data[c.off:])
	c.off += int64(n)
	c.reads++
	return n, nil
}
func (c *countingSource) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		c.off = offset
	case io.SeekCurrent:
		c.off += offset
	case io.SeekEnd:
		c.off = int64(len(c.data)) + offset
	}
	return c.off, nil
}
func (c *countingSource) Close() error { return nil }
func (c *countingSource) Size() int64  { return int64(len(c.data)) }
func (c *countingSource) Name() string { return "counting-source" }

func writeSegmentsFixture(t *testing.T, plain []byte, segSize int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "multi-seg.sccgt")

	salt, err := crypto.GenerateSalt_v2(16)
	if err != nil {
		t.Fatalf("salt: %v", err)
	}
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
		"iv_base64":   crypto.Base64Encode_v2(make([]byte, 16)),
	})
	if err != nil {
		t.Fatalf("kvi: %v", err)
	}

	var results []*crypto.SegmentEncryptionResult
	var ids []string
	for i, off := 0, 0; ; i++ {
		end := off + segSize
		if end > len(plain) {
			end = len(plain)
		}
		res, err := crypto.EncryptSegment(plain[off:end], dek, nil, uint32(i), crypto.CompressionModeNone)
		if err != nil {
			t.Fatalf("EncryptSegment: %v", err)
		}
		results = append(results, res)
		ids = append(ids, fmt.Sprintf("seg-%d", i))
		off = end
		if off >= len(plain) {
			break
		}
	}
	segments := make([]types.Segment_v4, len(ids))
	for i, id := range ids {
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
			ContainerID:   "multi-seg",
			ContainerType: "text",
			Segments:      segments,
			Playlists:     map[string][]string{"default": ids},
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

// TestSegmentSeekableReader_PropagatesNeedBytes 抽样时"字节还没供给"必须**上抛**，
// 不能被当成"抽样不适用"退回逐段读。
//
// 不守这条会怎样：每次重试只会多命中一个段头，于是 O(n²) 次段头读取 ——
// 实测 1GB 容器触发 13 万次 readSegmentHeader（Range 请求也有上千个）。
func TestSegmentSeekableReader_PropagatesNeedBytes(t *testing.T) {
	plain := bytes.Repeat([]byte("0123456789abcdef"), 4096)
	path := writeSegmentsFixture(t, plain, 1024)

	info, err := OpenV4Container(path, nonceGuardPassword)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}
	if len(info.Manifest.Segments) < 32 {
		t.Fatalf("fixture 段数太少（%d）", len(info.Manifest.Segments))
	}

	// 一个永远缺字节的源：任何读取都返回 NeedBytesError
	info.Src = &neverSuppliedSource{}
	_, err = NewSegmentSeekableReader(info, "")
	if err == nil {
		t.Fatal("字节根本没供给，构造却成功了")
	}
	if !IsNeedBytes(err) {
		t.Errorf("缺字节时必须把 NeedBytesError 原样上抛，实际是：%v", err)
	}
}

// neverSuppliedSource 模拟"字节尚未供给"的按需源（浏览器 openStream 就是这种）。
type neverSuppliedSource struct{ off int64 }

type needBytesErr struct{ off int64 }

func (e *needBytesErr) Error() string     { return "需要容器字节（测试用）" }
func (e *needBytesErr) IsNeedBytes() bool { return true }

func (s *neverSuppliedSource) ReadAt(p []byte, off int64) (int, error) {
	return 0, &needBytesErr{off: off}
}
func (s *neverSuppliedSource) Read(p []byte) (int, error) { return s.ReadAt(p, s.off) }
func (s *neverSuppliedSource) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekStart {
		s.off = offset
	}
	return s.off, nil
}
func (s *neverSuppliedSource) Close() error { return nil }
func (s *neverSuppliedSource) Size() int64  { return 1 << 30 }
func (s *neverSuppliedSource) Name() string { return "never-supplied" }

// TestSegmentSeekableReader_SamplesSegmentHeaders 构造时的随机读次数必须与段数无关。
func TestSegmentSeekableReader_SamplesSegmentHeaders(t *testing.T) {
	// 64 段 / 1KB 段：足够多，抽样能省下数量级的随机读
	plain := bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64KB
	path := writeSegmentsFixture(t, plain, 1024)

	info, err := OpenV4Container(path, nonceGuardPassword)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}
	segs := len(info.Manifest.Segments)
	if segs < 32 {
		t.Fatalf("fixture 段数太少（%d），看不出抽样效果", segs)
	}

	// 用计数源替换，统计构造 SegmentSeekableReader 期间发生的随机读次数
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	cs := &countingSource{data: data}
	info.Src = cs
	r, err := NewSegmentSeekableReader(info, "")
	if err != nil {
		t.Fatalf("NewSegmentSeekableReader: %v", err)
	}
	defer r.Close()

	if cs.reads > 16 {
		t.Errorf("构造时随机读了 %d 次（段数 %d）—— 说明还在逐段读段头，抽样没生效", cs.reads, segs)
	}

	// 抽出来的长度必须与原路径一致：这里用"总长 == 明文长度"来验
	if got := r.totalSize(); got != int64(len(plain)) {
		t.Errorf("抽样算出的明文总长 %d != 实际 %d（为了省 IO 算错了）", got, len(plain))
	}

	// 再真读一遍内容，确保按抽样长度寻址也能解出正确字节
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("读明文失败：%v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Error("按抽样长度寻址解出的内容与明文不一致")
	}
}
