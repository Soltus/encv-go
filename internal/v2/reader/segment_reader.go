// internal/v2/reader/segment_reader.go
//
// v4 Segment 读取模块（Task 12: 集成 MAC 校验前置 + zstd 压缩解压）。
//
// 本文件实现 ENCV v4 容器的 Segment 级别解密路径，遵循 WinZip AE-2 规范的
// "Encrypt-then-MAC" 顺序：
//
//	读取 SegmentHeader → 切分密文边界 → 校验 HMAC → AES-CTR 解密 → 可选 zstd 解压
//
// 安全保证（与 writer 对称）：
//   1. 加密 Segment **必须** 先校验 HMAC，失败立即返回 crypto.ErrMACMismatch
//   2. 失败时不解密、不解压（防 zstd 解压炸弹 + 防 CTR 比特翻转攻击线索泄露）
//   3. 校验通过后才执行 AES-CTR + 可选解压
//   4. 解压失败明确返回 error（不静默返回"未解压数据"）
//
// 密钥派生策略（OpenV4Container 时一次性完成）：
//   - encrypt_key = PBKDF2-SHA256(password, encrypt_salt, 100000, KeySizeForCipherMode(CipherMode))
//   - mac_key     = PBKDF2-SHA256(password, mac_salt, 100000, 32)
//   - 旧 v4 容器（Manifest 无 mac_salt 字段）→ mac_key 留空，reader 跳过 MAC 校验
//
// 磁盘布局（v4 升级后 34 字节 SegmentHeader）：
//
//	[SegmentHeader(34B)][Nonce(16B)][Ciphertext(DataLength B)][HMAC(MACSize B)][SeekTable(var)]
//
// 向后兼容：
//   - SegmentHeader.MACSize = 0 时（旧 v4 容器 / EnableHMAC=false）跳过 MAC 校验
//   - SegmentHeader.ModeFlagEncrypted = 0 时（明文 Segment）跳过 Nonce/Ciphertext/HMAC
//   - SegmentHeader.SeekTableLength = 0 时无 zstd 压缩
package reader

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"

	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/crypto/compression"
	"github.com/Soltus/encv-go/internal/v2/filename"
	"github.com/Soltus/encv-go/internal/v2/types"

	seekable "github.com/SaveTheRbtz/zstd-seekable-format-go/pkg"
)

// V4ContainerInfo 封装一个 v4 容器的元数据 + 派生密钥。
//
// 关键字段：
//   - EncryptKey: AES-CTR 加密密钥（16 或 32 字节，由 Header.CipherMode 决定）
//   - MacKey:     HMAC-SHA1-80 用的 MAC 密钥（32 字节；旧 v4 容器为 nil）
//   - MacSalt:    16 字节 mac_salt（从 Manifest.MACSaltBase64 解码；旧 v4 容器为空）
//   - Key:        等同于 EncryptKey，保留旧 API 兼容
type V4ContainerInfo struct {
	Header   *types.EnvelopeHeaderV4
	Footer   *types.EnvelopeFooterV4
	Manifest *types.Manifest_v4
	FilePath string

	// Key 是旧字段名，语义上等同于 EncryptKey（保留向后兼容）。
	// 长度由 Header.CipherMode 决定：0=16B（AES-128），1=32B（AES-256）。
	Key []byte

	// EncryptKey 是 AES-CTR 加密密钥（16 或 32 字节）。
	// 派生参数：PBKDF2-SHA256(password, encrypt_salt, 100000, keySize)
	// keySize = KeySizeForCipherMode_v4(Header.CipherMode)
	EncryptKey []byte

	// MacKey 是 HMAC-SHA1-80 用的 MAC 密钥（32 字节）。
	// 派生参数：PBKDF2-SHA256(password, mac_salt, 100000, 32)
	// 旧 v4 容器（Manifest 无 mac_salt 字段）→ 留空（nil）
	// 留空时 reader 跳过 MAC 校验（依赖 SegmentHeader.MACSize=0 的双保险）
	MacKey []byte

	// MacSalt 是从 Manifest.MACSaltBase64 提取的 16 字节 mac_salt。
	// 旧 v4 容器（Manifest JSON 无 mac_salt_base64 字段）→ 长度为 0。
	// 派生 mac_key 时使用；reader 不直接消费。
	MacSalt []byte
}

func (info *V4ContainerInfo) ResolveDisplayName(physicalName string, password string) (string, error) {
	return filename.ResolveDisplayName(
		context.Background(), physicalName, info.Manifest, info.Header.Flags, password, filename.FNConfig{},
	)
}

// OpenV4Container 打开 v4 容器并派生所有需要的密钥。
//
// 关键逻辑：
//  1. 解析 Header 拿到 CipherMode（0=AES-128, 1=AES-256）
//  2. 从 KVI 拿到 encrypt_salt，派生 encrypt_key（长度由 CipherMode 决定）
//  3. 从 Manifest.MACSaltBase64 拿到 mac_salt（如有），派生 mac_key
//  4. 旧 v4 容器兼容：Manifest 无 mac_salt_base64 字段 → mac_key = nil
func OpenV4Container(filePath string, password string) (*V4ContainerInfo, error) {
	src, err := containerhandle.NewFileSource(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open container source: %w", err)
	}

	h, err := containerhandle.Open(src)
	if err != nil {
		src.Close()
		return nil, fmt.Errorf("failed to open container handle: %w", err)
	}
	defer h.Close()

	if h.Version() != 4 {
		return nil, fmt.Errorf("not a v4 container (version: %d)", h.Version())
	}

	var kvi struct {
		SaltBase64 string `json:"salt_base64"`
		IVBase64   string `json:"iv_base64"`
	}
	if err := json.Unmarshal(h.Manifest().KVI, &kvi); err != nil {
		return nil, fmt.Errorf("failed to parse KVI from manifest: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(kvi.SaltBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt from KVI: %w", err)
	}

	hdr := h.HeaderV4()
	if hdr.PasswordHint != [16]byte{} {
		if !crypto.VerifyPasswordHint(hdr.PasswordHint, password, salt) {
			return nil, fmt.Errorf("%w: password hint verification failed", types.ErrWrongPassword)
		}
	}

	// 派生 encrypt_key：长度由 Header.CipherMode 决定
	keySize := crypto.KeySizeForCipherMode_v4(crypto.CipherMode_v4(hdr.CipherMode))
	encryptKey := crypto.GenerateKey_v4(password, salt, keySize)

	// MacSalt 提取：mac_salt 改存于 Manifest（v4 Header offset 36-2028 被 SpecialID
	// 完全占据，无法插入 16 字节），详见 header_v4.go 注释。
	//
	// 向后兼容：旧 v4 容器的 Manifest JSON 没有 mac_salt_base64 字段
	// → 字段为 "" → macSalt 保持 nil（长度 0）→ types.HasMACSalt 返回 false
	var macSalt []byte
	if mfV4 := h.ManifestV4(); mfV4 != nil && mfV4.MACSaltBase64 != "" {
		macSalt, err = base64.StdEncoding.DecodeString(mfV4.MACSaltBase64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode mac salt from manifest: %w", err)
		}
		// 防御：解码后长度应严格为 MACSaltLength (16)
		if len(macSalt) != crypto.MACSaltLength {
			return nil, fmt.Errorf("decoded mac salt has length %d, want %d", len(macSalt), crypto.MACSaltLength)
		}
	}

	// 派生 mac_key：仅在 mac_salt 存在时派生
	var macKey []byte
	if types.HasMACSalt(macSalt) {
		macKey = crypto.DeriveMACKey(password, macSalt)
	}

	return &V4ContainerInfo{
		Header:     hdr,
		Footer:     h.FooterV4(),
		Manifest:   h.ManifestV4(),
		FilePath:   filePath,
		Key:        encryptKey, // 旧字段，向后兼容
		EncryptKey: encryptKey,
		MacKey:     macKey,
		MacSalt:    macSalt,
	}, nil
}

// SegmentSeekableReader 提供 v4 Segment 列表的随机访问（io.ReadSeeker）。
type SegmentSeekableReader struct {
	info       *V4ContainerInfo
	file       *os.File
	playlist   []types.Segment_v4
	plainSizes []int64
	offset     int64
}

// segmentPlainSize 计算一个 Segment 的明文大小。
//
// 关键逻辑：
//   - 明文 Segment（ModeFlagEncrypted=0）→ DataLength
//   - 加密未压缩 Segment（Encrypted=1, CompressionZstd=0）→ DataLength
//   - 加密 zstd 压缩 Segment → 解析 seek table，从 frame 元数据得解压后总大小
//
// seek table Size() 返回的是 zstd 解压后总大小，调用方无需实际解压即可得知
// plaintext size，因此 `plainSizes` 可在构造时正确填充。
func (r *SegmentSeekableReader) segmentPlainSize(seg types.Segment_v4) (int64, error) {
	// 读取 SegmentHeader 以判断 ModeFlags
	segHeader, _, err := r.readSegmentHeader(seg)
	if err != nil {
		return 0, err
	}

	if segHeader.ModeFlags&types.ModeFlagCompressionZstd == 0 {
		// 不压缩：plaintext size = ciphertext size = DataLength
		return int64(segHeader.DataLength), nil
	}

	// 压缩：必须读 seek table 算解压大小
	// 重新走一遍完整流程取 seek table（避免重复解析）
	_, _, seekTable, err := r.readSegmentParts(seg)
	if err != nil {
		return 0, err
	}
	if len(seekTable) == 0 {
		return 0, fmt.Errorf("segment '%s' is compressed but has no seek table", seg.ID)
	}
	st, err := seekable.NewSeekTable(seekTable)
	if err != nil {
		return 0, fmt.Errorf("failed to parse seek table for segment '%s': %w", seg.ID, err)
	}
	return int64(st.Size()), nil
}

// readSegmentHeader 从 file 读取一个 Segment 的 34 字节 Header 并解析。
// 返回 (segHeader, payloadSize, error)。payloadSize = seg.Size - SegmentHeaderSize
// （注意 seg.Size 包含 Header、Nonce、Ciphertext、HMAC、SeekTable）。
func (r *SegmentSeekableReader) readSegmentHeader(seg types.Segment_v4) (*types.SegmentHeader, int, error) {
	headerSize := int64(types.SegmentHeaderSize)
	if int64(seg.Size) < headerSize {
		return nil, 0, fmt.Errorf("segment '%s' size %d < header size %d", seg.ID, seg.Size, headerSize)
	}
	payloadSize := int(seg.Size) - types.SegmentHeaderSize

	segHeaderBytes := make([]byte, headerSize)
	if _, err := r.file.Seek(int64(seg.Offset), io.SeekStart); err != nil {
		return nil, 0, fmt.Errorf("failed to seek to segment '%s': %w", seg.ID, err)
	}
	if _, err := io.ReadFull(r.file, segHeaderBytes); err != nil {
		return nil, 0, fmt.Errorf("failed to read segment header '%s': %w", seg.ID, err)
	}

	segHeader := &types.SegmentHeader{}
	if err := segHeader.UnmarshalBinary(segHeaderBytes); err != nil {
		return nil, 0, fmt.Errorf("failed to parse segment header '%s': %w", seg.ID, err)
	}
	return segHeader, payloadSize, nil
}

// readSegmentParts 读取一个 Segment 的 Header + 切分后的各部分。
// 返回 (segHeader, payloadPart, seekTable, error)。
//
// payloadPart 根据 ModeFlags 切分：
//   - ModeFlagEncrypted=0（明文）：payloadPart = 整段 payload
//   - ModeFlagEncrypted=1（加密）：payloadPart = Nonce || Ciphertext [|| HMAC]
//   - seekTable 单独切出（无论是否加密，seek table 总在末尾）
func (r *SegmentSeekableReader) readSegmentParts(seg types.Segment_v4) (*types.SegmentHeader, []byte, []byte, error) {
	segHeader, payloadSize, err := r.readSegmentHeader(seg)
	if err != nil {
		return nil, nil, nil, err
	}

	payload := make([]byte, payloadSize)
	if _, err := io.ReadFull(r.file, payload); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read segment payload '%s': %w", seg.ID, err)
	}

	// seek table 总在 payload 末尾（如有）
	seekTableLength := int(segHeader.SeekTableLength)
	var seekTable []byte
	if seekTableLength > 0 {
		if seekTableLength > len(payload) {
			return nil, nil, nil, fmt.Errorf("segment '%s' seek table length %d exceeds payload %d",
				seg.ID, seekTableLength, len(payload))
		}
		seekTable = payload[len(payload)-seekTableLength:]
		payload = payload[:len(payload)-seekTableLength]
	}

	return segHeader, payload, seekTable, nil
}

// decryptSegmentPayload 解密（并解压）一个 v4 Segment 的内容。
//
// 这是 reader 路径的核心，封装了所有 v4 升级布局的处理：
//  1. 解析 SegmentHeader 拿 ModeFlags / DataLength / NonceSize / MACSize / SeekTableLength
//  2. 切分 nonce / ciphertext / mac / seekTable
//  3. 加密 Segment（ModeFlagEncrypted=1）：
//     a. MACSize > 0 → crypto.DecryptSegment 强制 MAC 校验 + 解密 + 解压
//     b. MACSize == 0 → 跳过 MAC 校验，仅解密（向后兼容 / NoHMAC）
//  4. 明文 Segment（ModeFlagEncrypted=0）：payload 整体作为 plaintext
//
// 严格顺序保证：
//   - MAC 校验失败 → 立即返回 crypto.ErrMACMismatch（不解密、不解压）
//   - 解压失败 → 返回 compression.ErrDecompressionFailed 包装错误
func decryptSegmentPayload(info *V4ContainerInfo, seg types.Segment_v4) ([]byte, error) {
	f, err := os.Open(info.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open container file: %w", err)
	}
	defer f.Close()

	// 1. 读 SegmentHeader
	headerSize := int64(types.SegmentHeaderSize)
	if int64(seg.Size) < headerSize {
		return nil, fmt.Errorf("segment '%s' size %d < header size %d", seg.ID, seg.Size, headerSize)
	}
	segHeaderBytes := make([]byte, headerSize)
	if _, err := f.Seek(int64(seg.Offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to segment '%s': %w", seg.ID, err)
	}
	if _, err := io.ReadFull(f, segHeaderBytes); err != nil {
		return nil, fmt.Errorf("failed to read segment header '%s': %w", seg.ID, err)
	}
	segHeader := &types.SegmentHeader{}
	if err := segHeader.UnmarshalBinary(segHeaderBytes); err != nil {
		return nil, fmt.Errorf("failed to parse segment header '%s': %w", seg.ID, err)
	}

	// 2. 读 payload（Header 之后的所有字节）
	payloadSize := int(seg.Size) - types.SegmentHeaderSize
	if payloadSize < 0 {
		return nil, fmt.Errorf("segment '%s' payload size negative: %d", seg.ID, payloadSize)
	}
	payload := make([]byte, payloadSize)
	if _, err := io.ReadFull(f, payload); err != nil {
		return nil, fmt.Errorf("failed to read segment payload '%s': %w", seg.ID, err)
	}

	// 3. 切 seek table（总在末尾）
	seekTableLength := int(segHeader.SeekTableLength)
	var seekTable []byte
	if seekTableLength > 0 {
		if seekTableLength > len(payload) {
			return nil, fmt.Errorf("segment '%s' seek table length %d exceeds payload %d",
				seg.ID, seekTableLength, len(payload))
		}
		seekTable = payload[len(payload)-seekTableLength:]
		payload = payload[:len(payload)-seekTableLength]
	}

	// 4. 明文 Segment：直接返回 payload
	if segHeader.ModeFlags&types.ModeFlagEncrypted == 0 {
		return payload, nil
	}

	// 5. 加密 Segment：切分 nonce / ciphertext / mac
	nonceSize := int(segHeader.NonceSize)
	if nonceSize > len(payload) {
		return nil, fmt.Errorf("segment '%s' nonce size %d exceeds remaining payload %d",
			seg.ID, nonceSize, len(payload))
	}
	nonce := payload[:nonceSize]
	remaining := payload[nonceSize:]

	dataLength := int(segHeader.DataLength)
	if dataLength > len(remaining) {
		return nil, fmt.Errorf("segment '%s' data length %d exceeds remaining payload %d",
			seg.ID, dataLength, len(remaining))
	}
	ciphertext := remaining[:dataLength]
	remaining = remaining[dataLength:]

	var mac []byte
	if segHeader.MACSize > 0 {
		macSize := int(segHeader.MACSize)
		if macSize > len(remaining) {
			return nil, fmt.Errorf("segment '%s' MAC size %d exceeds remaining payload %d",
				seg.ID, macSize, len(remaining))
		}
		mac = remaining[:macSize]
	}

	// 6. 选择压缩模式字符串
	compressionMode := crypto.CompressionModeNone
	if segHeader.ModeFlags&types.ModeFlagCompressionZstd != 0 {
		compressionMode = crypto.CompressionModeZstd
	}

	// 7. 解密（+ 校验 MAC + 解压）
	if segHeader.MACSize > 0 {
		// 有 MAC：必须校验
		if info.MacKey == nil {
			return nil, fmt.Errorf("segment '%s' has MACSize %d but no MacKey available (container missing mac_salt)",
				seg.ID, segHeader.MACSize)
		}
		plaintext, decErr := crypto.DecryptSegment(
			ciphertext, nonce, info.EncryptKey, info.MacKey, mac, compressionMode, seekTable,
		)
		if decErr != nil {
			return nil, decErr // 包含 crypto.ErrMACMismatch / ErrDecompressionFailed
		}
		return plaintext, nil
	}

	// 无 MAC：仅解密（向后兼容旧 v4 容器 / EnableHMAC=false）
	plaintext, decErr := crypto.DecryptBytes_v2(ciphertext, info.EncryptKey, nonce)
	if decErr != nil {
		return nil, fmt.Errorf("failed to decrypt segment '%s': %w", seg.ID, decErr)
	}

	// 可选 zstd 解压
	if compressionMode == crypto.CompressionModeZstd {
		if len(seekTable) == 0 {
			return nil, fmt.Errorf("segment '%s' is compressed but has no seek table", seg.ID)
		}
		dec, decErr := compression.DecompressZstdSeekable(plaintext, seekTable)
		if decErr != nil {
			return nil, fmt.Errorf("%w: %v", crypto.ErrDecompressionFailed, decErr)
		}
		return dec, nil
	}

	return plaintext, nil
}

// NewSegmentSeekableReader 构造一个 v4 Segment 随机访问 reader。
func NewSegmentSeekableReader(info *V4ContainerInfo, playlistName string) (*SegmentSeekableReader, error) {
	playlist, err := info.Manifest.ResolvePlaylist(playlistName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve playlist '%s': %w", playlistName, err)
	}

	f, err := os.Open(info.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// 构造时一次性计算每个 Segment 的明文大小
	// （压缩 Segment 需读 seek table，因此打开文件计算）
	tmpReader := &SegmentSeekableReader{
		info:     info,
		file:     f,
		playlist: playlist,
		offset:   0,
	}
	plainSizes := make([]int64, len(playlist))
	for i, seg := range playlist {
		sz, err := tmpReader.segmentPlainSize(seg)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("failed to compute plaintext size for segment '%s': %w", seg.ID, err)
		}
		plainSizes[i] = sz
	}

	return &SegmentSeekableReader{
		info:       info,
		file:       f,
		playlist:   playlist,
		plainSizes: plainSizes,
		offset:     0,
	}, nil
}

func (r *SegmentSeekableReader) totalSize() int64 {
	var total int64
	for _, sz := range r.plainSizes {
		total += sz
	}
	return total
}

func (r *SegmentSeekableReader) locateOffset(off int64) (segIdx int, offsetInSegment int64, ok bool) {
	var accumulated int64
	for i, sz := range r.plainSizes {
		segEnd := accumulated + sz
		if off < segEnd {
			return i, off - accumulated, true
		}
		accumulated = segEnd
	}
	if off == accumulated {
		return len(r.playlist), 0, true
	}
	return -1, 0, false
}

// ReadAt 在指定 plaintext 偏移处读取 p，返回字节数与 error。
//
// 严格顺序：
//  1. locateOffset → 找目标 Segment
//  2. decryptSegmentPayload（内部完成 MAC 校验 + 解密 + 解压）
//  3. 从 plaintext 复制到 p
func (r *SegmentSeekableReader) ReadAt(p []byte, off int64) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	total := r.totalSize()
	if off >= total {
		return 0, io.EOF
	}
	if off < 0 {
		return 0, fmt.Errorf("negative offset: %d", off)
	}

	segIdx, offsetInSeg, ok := r.locateOffset(off)
	if !ok || segIdx >= len(r.playlist) {
		return 0, io.EOF
	}

	written := 0
	for written < len(p) && segIdx < len(r.playlist) {
		seg := r.playlist[segIdx]

		plainData, decErr := decryptSegmentPayload(r.info, seg)
		if decErr != nil {
			return written, fmt.Errorf("failed to decrypt segment '%s': %w", seg.ID, decErr)
		}

		available := len(plainData) - int(offsetInSeg)
		if available <= 0 {
			segIdx++
			offsetInSeg = 0
			continue
		}

		toCopy := len(p) - written
		if toCopy > available {
			toCopy = available
		}
		copy(p[written:], plainData[offsetInSeg:offsetInSeg+int64(toCopy)])
		written += toCopy
		segIdx++
		offsetInSeg = 0
	}

	if written == 0 {
		return 0, io.EOF
	}
	return written, nil
}

func (r *SegmentSeekableReader) Seek(offset int64, whence int) (int64, error) {
	total := r.totalSize()
	switch whence {
	case io.SeekStart:
		r.offset = offset
	case io.SeekCurrent:
		r.offset += offset
	case io.SeekEnd:
		r.offset = total + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if r.offset < 0 {
		r.offset = 0
		return 0, fmt.Errorf("cannot seek to negative offset")
	}
	if r.offset > total {
		r.offset = total
	}
	return r.offset, nil
}

func (r *SegmentSeekableReader) Read(p []byte) (n int, err error) {
	n, err = r.ReadAt(p, r.offset)
	r.offset += int64(n)
	return n, err
}

func (r *SegmentSeekableReader) Close() error {
	return r.file.Close()
}

// SegmentSequentialReader 提供 v4 Segment 列表的顺序流式访问（io.Reader）。
type SegmentSequentialReader struct {
	info      *V4ContainerInfo
	file      *os.File
	playlist  []types.Segment_v4
	segIndex  int
	segReader io.Reader
}

func NewSegmentSequentialReader(info *V4ContainerInfo, playlistName string) (*SegmentSequentialReader, error) {
	playlist, err := info.Manifest.ResolvePlaylist(playlistName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve playlist '%s': %w", playlistName, err)
	}

	f, err := os.Open(info.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return &SegmentSequentialReader{
		info:     info,
		file:     f,
		playlist: playlist,
		segIndex: 0,
	}, nil
}

func (r *SegmentSequentialReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	if r.segIndex >= len(r.playlist) {
		return 0, io.EOF
	}

	if r.segReader == nil {
		if err := r.setupNextSegment(); err != nil {
			return 0, err
		}
	}

	n, err = r.segReader.Read(p)
	if err == io.EOF {
		r.segReader = nil
		r.segIndex++
		if n > 0 {
			return n, nil
		}
		return r.Read(p)
	}
	return n, err
}

func (r *SegmentSequentialReader) setupNextSegment() error {
	if r.segIndex >= len(r.playlist) {
		return io.EOF
	}

	seg := r.playlist[r.segIndex]

	plainData, err := decryptSegmentPayload(r.info, seg)
	if err != nil {
		return fmt.Errorf("failed to decrypt segment '%s': %w", seg.ID, err)
	}

	r.segReader = newBytesReader(plainData)
	return nil
}

func (r *SegmentSequentialReader) Close() error {
	return r.file.Close()
}

type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data, pos: 0}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func SeekByTime(info *V4ContainerInfo, timeSeconds float64) (segmentIndex int, offsetInSegment int64, err error) {
	segments := info.Manifest.Segments
	if len(segments) == 0 {
		return 0, 0, fmt.Errorf("manifest has no segments")
	}

	for i, seg := range segments {
		segEnd := seg.StartTime + seg.Duration
		if timeSeconds < segEnd || (i == len(segments)-1 && timeSeconds >= seg.StartTime) {
			if timeSeconds < seg.StartTime {
				return i, 0, nil
			}
			progress := (timeSeconds - seg.StartTime) / seg.Duration
			if progress < 0 {
				progress = 0
			}
			if progress > 1 {
				progress = 1
			}
			// 复用 size 计算逻辑（压缩 Segment 需读 seek table）
			plainSize, sizeErr := computeSegmentPlainSize(info, seg)
			if sizeErr != nil {
				plainSize = int64(seg.Size) - int64(types.SegmentHeaderSize) - 16
				if plainSize < 0 {
					plainSize = 0
				}
			}
			offset := int64(progress * float64(plainSize))
			return i, offset, nil
		}
	}

	return 0, 0, fmt.Errorf("time %v is beyond the end of the content", timeSeconds)
}

// computeSegmentPlainSize 计算一个 Segment 的明文大小（SeekByTime 辅助）。
//
// 对压缩 Segment 必须读 seek table 才能得知解压后大小；为避免 SeekByTime
// 重复打开文件，此函数每次都重新打开一次（频率低，开销可接受）。
func computeSegmentPlainSize(info *V4ContainerInfo, seg types.Segment_v4) (int64, error) {
	f, err := os.Open(info.FilePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	tmpReader := &SegmentSeekableReader{
		info:     info,
		file:     f,
		playlist: nil,
	}
	return tmpReader.segmentPlainSize(seg)
}
