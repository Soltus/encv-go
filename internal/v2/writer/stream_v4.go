// internal/v2/writer/stream_v4.go
//
// **流式** v4 容器写入：明文按任意大小的块喂进来，密文随写随出，
// Go 侧常驻内存只与 SegmentSize 有关，**与文件总大小无关**。
//
// 存在的理由（整块写入为什么不够用）：
// WriteV4Container / WriteV4ContainerTo 要求调用方同时把「完整明文 + 完整密文 +
// 完整容器」三份字节摆在内存里（跨 JS↔Go 边界还要再拷贝一次）。浏览器里那条路
// 会在 Go 的线性内存上限上直接 fatal error —— 实测 1GB 明文即报
// `runtime: out of memory: cannot allocate … (3229417472 in use)`
// （证据由 app/encv-mobile/pw-enc-stream.ts 采集）。
// 这里把写入改成增量的：明文分片进一个固定容量的段缓冲，CTR 逐片推进 keystream，
// 攒满一段就排版成整块的段数据 Append 出去（写法见 openSeg.write）。
//
// 产物布局与整块路径**逐字节同构**（段布局复用同一个 writeOneSegment，
// header/footer 同一套），所以主线 reader / CLI / wasm 都能原样打开它：
//
//	[Header(2048B)][Segment0][Segment1]…[Manifest(XOR)][Footer(12B)]
//
// **为什么 Sink 不是 io.Writer**：
// 容器头必须先于数据出现在产物里，但头里的 ManifestOffset/ManifestLength/
// GlobalCRC32 只有写完全部数据才知道 —— 顺序写不支持「回头改第 0 字节」。
// 约定：数据段先按容器内顺序 Append，最后用 Finish 一次性交付「头 + 尾」，
// 调用方按 head ‖ (已 Append 的数据) ‖ tail 组装即可。
//
// ⚠️ Append 交出的字节**不转移所有权**：下一次 Append 会复用同一块底层缓冲，
// sink 实现必须在 Append 内拷贝（MemSink 就是这么做的）。
package writer

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// DefaultStreamSegmentSize 是默认的 Segment 大小（4MB）。
//
// 它决定流式加密的**常驻内存下限**：一个 Segment 的密文要攒够了才能回头写
// SegmentHeader（头里有 DataLength，而 Streaming 一开始并不知道这段会有多长）。
// 调小更省内存（每段多付 34B 段头 + 16B nonce 的相对开销），调大省段数。
const DefaultStreamSegmentSize = 4 << 20

// V4StreamSink 是流式 v4 写入的落点（接口说明见文件顶部注释）。
type V4StreamSink interface {
	// Append 追加一段已成型的数据，调用顺序 == 容器内偏移顺序。
	// 实现必须在内部拷贝 p（p 会被写入器复用）。
	Append(p []byte) error

	// Finish 交付容器的「头」与「尾」：
	//   head = 2048 字节信封头（落在产物最前面）
	//   tail = 混淆后的 manifest + 12 字节 footer（紧跟已 Append 的数据之后）
	Finish(head []byte, tail []byte) error
}

// V4StreamParams 是流式 v4 写入的全部入参。
//
// 与 V4WriteParams 的关系：语义能对上的字段保持同名；差别是这里没有 SegmentResults
// （密文边加密边产出），取而代之的是 SegmentSize 与回调 OnManifest
// （manifest 里凡「要写完全部明文才知道」的字段，都在那个回调里填）。
type V4StreamParams struct {
	// IsMain / ContainerType / IsSeekable / IDType / IDData / PasswordHint
	// 与 V4WriteParams 同名同义，直接传给 types.CreateHeaderV4。
	IsMain        bool
	ContainerType uint16
	IsSeekable    bool
	IDType        types.IDType
	IDData        []byte
	PasswordHint  [16]byte

	// Manifest 是 manifest 的初始内容。调用方负责 Version / ContainerType /
	// ContainerID / OriginalName / WrappedDEK 等「写之前就知道」的字段；
	// Segments / Playlists / KVI / MACSaltBase64 由本写入器填。
	Manifest *types.Manifest_v4

	// Key 是 AES-CTR 数据密钥（分层密钥体系下即 DEK）。
	// MacKey 是 HMAC-SHA1-80 密钥；EnableHMAC=true 时必须非空，且必须由
	// Manifest.MACSaltBase64 记录的 mac_salt 派生（否则 reader 校验一定过不去）。
	Key    []byte
	MacKey []byte

	// CipherMode：0=AES-128-CTR（默认）/ 1=AES-256-CTR，必须与 len(Key) 一致。
	CipherMode uint16

	// EnableHMAC 是否在每段末尾写 10 字节 MAC（见 V4WriteParams 同名字段注释）。
	EnableHMAC bool

	// SegmentSize 单个 Segment 的明文上限（<=0 时取 DefaultStreamSegmentSize）。
	SegmentSize int64

	// OnManifest 在序列化 manifest 之前被调用，是填 KVI / 插件 index 的**最后机会**
	// —— 明文总长与 MD5 只有到这里才有值。
	OnManifest func(m *types.Manifest_v4, plainSize int64, plainMD5 string) error
}

// V4StreamWriter 是 io.WriteCloser：Write 喂明文，Close 收尾。
type V4StreamWriter struct {
	sink   V4StreamSink
	params *V4StreamParams
	header *types.EnvelopeHeaderV4

	cur *openSeg

	ids       []string // 已封存的 segment id，顺序 == playlist "default" 的顺序
	dataBytes int64    // 已 Append 的数据字节数（不含 header / manifest / footer）
	tailBytes int64    // Close 时写入的 manifest+footer 长度
	globalCRC hash.Hash32
	plainSize int64
	plainHash hash.Hash // md5 随写随算，Close 时交给 OnManifest
	scratch   bytes.Buffer
	closed    bool
}

// openSeg 是一个正在攒数据的 Segment。
type openSeg struct {
	id       uint32
	nonce    []byte
	stream   cipher.Stream
	mac      hash.Hash
	dataCRC  hash.Hash32
	buf      []byte // 密文缓冲，容量 = SegmentSize
	plainLen int64
}

// NewV4StreamWriter 创建一个流式 v4 容器写入器。
//
// 与整块写入不同：**不需要**预先知道明文总长（SegmentHeader 的 DataLength 是每个
// Segment 自己的密文长度，在 seal 那一刻才确定）。
func NewV4StreamWriter(sink V4StreamSink, params *V4StreamParams) (*V4StreamWriter, error) {
	if sink == nil {
		return nil, fmt.Errorf("stream writer: sink 不能为 nil")
	}
	if params == nil || params.Manifest == nil {
		return nil, fmt.Errorf("stream writer: params.Manifest 不能为 nil")
	}
	if len(params.Key) == 0 {
		return nil, fmt.Errorf("stream writer: Key 不能为空")
	}
	if want := crypto.KeySizeForCipherMode_v4(crypto.CipherMode_v4(params.CipherMode)); len(params.Key) != want {
		return nil, fmt.Errorf("stream writer: CipherMode=%d 需要 %d 字节密钥，实得 %d", params.CipherMode, want, len(params.Key))
	}
	if params.EnableHMAC && len(params.MacKey) == 0 {
		return nil, fmt.Errorf("stream writer: EnableHMAC=true 时 MacKey 不能为空")
	}
	if params.SegmentSize <= 0 {
		params.SegmentSize = DefaultStreamSegmentSize
	}

	header, err := types.CreateHeaderV4(params.IsMain, params.ContainerType, params.IsSeekable, params.IDType, params.IDData, params.PasswordHint)
	if err != nil {
		return nil, fmt.Errorf("stream writer: failed to create v4 header: %w", err)
	}
	header.CipherMode = params.CipherMode

	// mac_salt 与 encrypt salt 是**两个独立通道**（见 Manifest_v4.MACSaltBase64 注释）。
	// 调用方没给就在这里生成并记进 manifest —— 绝不留空让后面「再补一个」：
	// 那会让加密用一个 mac_key、校验用另一个（MAC 永远验不过）。
	if params.Manifest.MACSaltBase64 == "" {
		macSalt, err := crypto.GenerateMACSalt()
		if err != nil {
			return nil, fmt.Errorf("stream writer: failed to generate mac salt: %w", err)
		}
		params.Manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)
	}

	return &V4StreamWriter{
		sink:      sink,
		params:    params,
		header:    header,
		globalCRC: crc32.NewIEEE(),
		plainHash: md5.New(),
		ids:       make([]string, 0, 1),
	}, nil
}

// Write 喂入一块明文（io.Writer 语义）。
//
// 内部按 SegmentSize 切段：攒满一段就排版成整块数据 Append 出去，
// 所以 Go 侧同时只持有「一个 Segment 的密文缓冲」。
func (w *V4StreamWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("stream writer: Write after Close")
	}
	written := 0
	for len(p) > 0 {
		if w.cur == nil {
			if err := w.startSegment(); err != nil {
				return written, err
			}
		}
		room := int64(cap(w.cur.buf)) - w.cur.plainLen
		n := int64(len(p))
		if n > room {
			n = room
		}
		if err := w.cur.write(p[:n]); err != nil {
			return written, err
		}
		w.plainSize += n
		p = p[n:]
		written += int(n)
		if w.cur.plainLen == int64(cap(w.cur.buf)) {
			if err := w.sealSegment(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

// PlainSize 已写入的明文总字节数。
func (w *V4StreamWriter) PlainSize() int64 { return w.plainSize }

// SegmentCount 到目前为止封存的 Segment 数。
func (w *V4StreamWriter) SegmentCount() int { return len(w.ids) }

// ContainerSize 产物总字节数（header + 数据 + manifest/footer）；Close 之后才齐全。
func (w *V4StreamWriter) ContainerSize() int64 {
	return int64(types.EnvelopeHeaderSize_v4) + w.dataBytes + w.tailBytes
}

// Close 收尾：封最后一段 → 序列化 manifest → 交付 head/tail。
//
// 空输入也会产出**一个空 Segment**：reader 按 playlist 取段序列，
// 一段都没有的容器没有可读内容（与整块路径的行为保持一致）。
func (w *V4StreamWriter) Close() error {
	if w.closed {
		return fmt.Errorf("stream writer: Close 重复调用")
	}
	// 没写过数据的情况：startSegment + seal 出一个空段，保证容器至少有一段
	if err := w.sealSegment(); err != nil {
		return err
	}
	w.closed = true

	manifest := w.params.Manifest
	manifest.Playlists = map[string][]string{"default": append([]string(nil), w.ids...)}

	if w.params.OnManifest != nil {
		if err := w.params.OnManifest(manifest, w.plainSize, hex.EncodeToString(w.plainHash.Sum(nil))); err != nil {
			return fmt.Errorf("stream writer: OnManifest: %w", err)
		}
	}

	manifestJSON, err := manifest.SerializeToJSON_v4()
	if err != nil {
		return fmt.Errorf("stream writer: failed to serialize manifest: %w", err)
	}
	obfuscated, err := crypto.ObfuscateManifest(manifestJSON)
	if err != nil {
		return fmt.Errorf("stream writer: failed to obfuscate manifest: %w", err)
	}

	var tailBuf bytes.Buffer
	if _, err := tailBuf.Write(obfuscated); err != nil {
		return fmt.Errorf("stream writer: failed to write manifest: %w", err)
	}
	footer := &types.EnvelopeFooterV4{
		Magic:       types.MagicFooter_v2,
		GlobalCRC32: w.globalCRC.Sum32(),
	}
	if err := types.WriteFooterV4(&tailBuf, footer); err != nil {
		return fmt.Errorf("stream writer: failed to write footer: %w", err)
	}

	w.header.ManifestOffset = uint32(types.EnvelopeHeaderSize_v4) + uint32(w.dataBytes)
	w.header.ManifestLength = uint32(len(obfuscated))
	if manifest.OriginalName != "" && manifest.FilenameAlgorithm != "" {
		w.header.Flags |= types.FlagFilenameEncrypted
	}

	var head bytes.Buffer
	if err := types.WriteHeaderV4(&head, w.header); err != nil {
		return fmt.Errorf("stream writer: failed to write header: %w", err)
	}
	w.tailBytes = int64(tailBuf.Len())
	return w.sink.Finish(head.Bytes(), tailBuf.Bytes())
}

// startSegment 开一个新 Segment：随机 nonce + 一条新的 keystream。
//
// ⚠️ 每个 Segment 都换 nonce：CTR 模式下同一把密钥复用 nonce = keystream 复用，
// 两段密文相异或就得到两段明文相异或（教科书式的二时间垫漏洞）。
func (w *V4StreamWriter) startSegment() error {
	nonce, err := crypto.GenerateIV_v2(crypto.IVSize_v2)
	if err != nil {
		return fmt.Errorf("stream writer: failed to generate nonce: %w", err)
	}
	block, err := aes.NewCipher(w.params.Key)
	if err != nil {
		return fmt.Errorf("stream writer: failed to create cipher: %w", err)
	}
	id := uint32(len(w.ids))
	w.cur = &openSeg{
		id:      id,
		nonce:   nonce,
		stream:  cipher.NewCTR(block, nonce),
		mac:     hmac.New(sha1.New, w.params.MacKey),
		dataCRC: crc32.NewIEEE(),
		buf:     make([]byte, 0, w.params.SegmentSize),
	}
	// HMAC 覆盖 nonce‖ciphertext（nonce 先进，防 nonce 被整体替换后重放旧密文）
	w.cur.mac.Write(nonce)
	// manifest 里先占好位置：writeOneSegment 会按同一个下标回填 offset/size/nonce
	w.params.Manifest.Segments = append(w.params.Manifest.Segments, types.Segment_v4{ID: fmt.Sprintf("seg-%d", id)})
	return nil
}

// sealSegment 封当前段：复用主线的 writeOneSegment 排版并 Append 出去。
//
// 复用 writeOneSegment 是刻意的 —— 段布局只有一处实现，流式与整块不可能漂移。
// 它算出的 Segment.Offset 是相对 scratch 的（恒为 0），真实容器内偏移在这里补。
func (w *V4StreamWriter) sealSegment() error {
	if w.cur == nil {
		if len(w.ids) > 0 {
			return nil // 上一段刚封完、还没写入新数据
		}
		if err := w.startSegment(); err != nil { // 从未写过任何数据 → 产出一个空段
			return err
		}
	}
	cur := w.cur

	var macSum [crypto.HMACSize_v4]byte
	if w.params.EnableHMAC {
		copy(macSum[:], cur.mac.Sum(nil)[:crypto.HMACSize_v4])
	}
	result := &crypto.SegmentEncryptionResult{
		SegmentID:     cur.id,
		Nonce:         cur.nonce,
		EncryptedData: cur.buf,
		DataCRC32:     cur.dataCRC.Sum32(),
		HMAC:          macSum,
		ModeFlags:     types.ModeFlagEncrypted,
	}

	w.scratch.Reset()
	// 只投影 writeOneSegment 真正读的字段，避免把整块路径的语义混进来
	segParams := &V4WriteParams{Manifest: w.params.Manifest, EnableHMAC: w.params.EnableHMAC}
	if err := writeOneSegment(&w.scratch, w.globalCRC, int(cur.id), result, segParams, w.header); err != nil {
		return fmt.Errorf("stream writer: failed to write segment %d: %w", cur.id, err)
	}
	if err := w.sink.Append(w.scratch.Bytes()); err != nil {
		return fmt.Errorf("stream writer: sink.Append: %w", err)
	}
	w.params.Manifest.Segments[cur.id].Offset = uint64(types.EnvelopeHeaderSize_v4) + uint64(w.dataBytes)
	w.dataBytes += int64(w.scratch.Len())
	w.ids = append(w.ids, w.params.Manifest.Segments[cur.id].ID)
	w.cur = nil
	return nil
}

// write 把一块明文加密进当前段的缓冲，并推进 CRC32 / HMAC。
func (s *openSeg) write(p []byte) error {
	off := len(s.buf)
	if off+len(p) > cap(s.buf) {
		return fmt.Errorf("internal: segment %d overflow (cap=%d off=%d add=%d)", s.id, cap(s.buf), off, len(p))
	}
	s.buf = s.buf[:off+len(p)]
	// AES-CTR 是**流密码**：同一个 cipher.Stream 连续多次 XORKeyStream 会继续推进
	// keystream 计数器 —— 所以不必把整个 Segment 的明文摆在内存里也能算出正确密文。
	s.stream.XORKeyStream(s.buf[off:], p)
	out := s.buf[off:]
	s.dataCRC.Write(out)
	s.mac.Write(out)
	s.plainLen += int64(len(p))
	return nil
}

// ──────────────────────────────── MemSink ────────────────────────────────

// MemSink 把流式写入的产物攒在内存里，用于测试与「反正要整体落内存」的场景。
type MemSink struct {
	head   []byte
	pieces [][]byte
	tail   []byte
	bytes  int64
}

var _ V4StreamSink = (*MemSink)(nil)

// NewMemSink 创建一个空的内存 sink。
func NewMemSink() *MemSink { return &MemSink{} }

// Append 追加一段数据（内部拷贝：写入器会复用传进来的缓冲）。
func (m *MemSink) Append(p []byte) error {
	m.pieces = append(m.pieces, append([]byte(nil), p...))
	m.bytes += int64(len(p))
	return nil
}

// Finish 接收容器的头与尾。
func (m *MemSink) Finish(head []byte, tail []byte) error {
	m.head = append([]byte(nil), head...)
	m.tail = append([]byte(nil), tail...)
	m.bytes += int64(len(head)) + int64(len(tail))
	return nil
}

// Bytes 拼出完整容器字节（head ‖ pieces ‖ tail）。
func (m *MemSink) Bytes() []byte {
	out := make([]byte, 0, m.bytes)
	out = append(out, m.head...)
	for _, p := range m.pieces {
		out = append(out, p...)
	}
	out = append(out, m.tail...)
	return out
}

// Size 产物总字节数。
func (m *MemSink) Size() int64 { return m.bytes }
