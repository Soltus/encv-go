package reader

import (
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// VirtualSeekableDecryptReader 实现了高性能的可寻址解密流。
type VirtualSeekableDecryptReader struct {
	containerReader EncryptedContainerReader
	key             []byte
	iv              []byte

	streamFragments      []types.Fragment
	seekIndex            *fragmentRangeIndex
	currentFragmentIndex int

	// currentRawReader 是指向当前 fragment 的底层加密数据读取器
	currentRawReader io.ReadCloser
	// 直接保存解密后的数据读取器，避免多层包装导致的状态混乱
	currentDataReader io.Reader

	globalOffset int64

	// 【性能优化】buffer pool 用于复用内存，减少 GC 压力
	bufPool sync.Pool
}

func NewVirtualSeekableDecryptReader(cr EncryptedContainerReader, password string) (DecryptReader, error) {
	return newVirtualSeekableDecryptReader(cr, password, nil)
}

func newVirtualSeekableDecryptReader(cr EncryptedContainerReader, password string, prebuiltIndex *fragmentRangeIndex) (DecryptReader, error) {
	manifest := cr.GetManifest()
	kviProvider, err := cr.GetKVIProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal KVI from manifest: %w", err)
	}

	key, iv, err := deriveKeyAndIV(kviProvider, password)
	if err != nil {
		return nil, err
	}

	seekableFragments := filterFragmentsByType(manifest.Fragments, string(types.FragmentType_SeekableStream))
	if len(seekableFragments) == 0 {
		return nil, fmt.Errorf("no seekable stream fragments found in manifest")
	}
	index := prebuiltIndex
	if index == nil {
		index = newFragmentRangeIndex(seekableFragments)
	}

	r := &VirtualSeekableDecryptReader{
		containerReader: cr,
		key:             key,
		iv:              iv,
		streamFragments: index.fragments,
		seekIndex:       index,
		bufPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 32*1024)
			},
		},
	}

	// 初始化到第一个 fragment
	r.currentFragmentIndex = 0
	if err := r.setupCurrentFragmentReader(); err != nil {
		return nil, fmt.Errorf("failed to initialize first fragment reader: %w", err)
	}

	return r, nil
}

// setupCurrentFragmentReader 准备当前分片的读取器，采用多路径自适应策略
func (r *VirtualSeekableDecryptReader) setupCurrentFragmentReader() error {
	if r.currentRawReader != nil {
		_ = r.currentRawReader.Close()
		r.currentRawReader = nil
		r.currentDataReader = nil
	}

	if r.currentFragmentIndex >= len(r.streamFragments) {
		return io.EOF
	}
	frag := &r.streamFragments[r.currentFragmentIndex]

	// 计算在当前 fragment 内的局部偏移
	localOffset := uint64(0)
	if r.globalOffset > int64(frag.GlobalStartOffset) {
		localOffset = uint64(r.globalOffset) - frag.GlobalStartOffset
	}
	if localOffset >= frag.Length {
		return fmt.Errorf("seek offset (%d) beyond fragment %s length (%d)", localOffset, frag.ID, frag.Length)
	}

	// 获取 Fragment 的 Reader (此时返回的是完整的 Fragment Payload)
	rawReader, err := r.containerReader.GetFragmentReader(frag.ID)
	if err != nil {
		return fmt.Errorf("container is corrupt: failed to get reader for fragment '%s': %w", frag.ID, err)
	}

	streamOffset := frag.GlobalStartOffset
	needDiscard := localOffset > 0
	if localOffset > 0 {
		if seeker, ok := rawReader.(io.Seeker); ok {
			if _, seekErr := seeker.Seek(int64(localOffset), io.SeekStart); seekErr == nil {
				streamOffset += localOffset
				needDiscard = false
			}
		}
	}

	stream, err := buildCTRStreamAtOffset(r.key, r.iv, streamOffset)
	if err != nil {
		_ = rawReader.Close()
		return err
	}
	streamReader := &cipher.StreamReader{S: stream, R: rawReader}
	if needDiscard {
		if err := discardReaderBytes(streamReader, localOffset, &r.bufPool); err != nil {
			_ = rawReader.Close()
			return err
		}
	}

	// 此时 streamReader 已经同步到了 absGlobalOffset
	r.currentRawReader = rawReader
	r.currentDataReader = streamReader
	return nil
}

// Read 实现 io.Reader 接口，使用健壮的循环逻辑
func (r *VirtualSeekableDecryptReader) Read(p []byte) (n int, err error) {
	totalRead := 0
	for totalRead < len(p) {
		if r.currentFragmentIndex >= len(r.streamFragments) {
			return totalRead, io.EOF
		}
		if r.currentDataReader == nil {
			if setupErr := r.setupCurrentFragmentReader(); setupErr != nil {
				if errors.Is(setupErr, io.EOF) {
					return totalRead, io.EOF
				}
				return totalRead, setupErr
			}
		}

		bytesRead, readErr := r.currentDataReader.Read(p[totalRead:])
		totalRead += bytesRead
		r.globalOffset += int64(bytesRead)

		if readErr == io.EOF {
			if r.currentRawReader != nil {
				_ = r.currentRawReader.Close()
				r.currentRawReader = nil
			}
			r.currentDataReader = nil
			r.currentFragmentIndex++
			continue
		}
		if readErr != nil {
			return totalRead, readErr
		}
	}
	return totalRead, nil
}

// Seek 实现 io.Seeker 接口，修正了多余逻辑
func (r *VirtualSeekableDecryptReader) Seek(offset int64, whence int) (int64, error) {
	totalSize := r.seekIndex.total()

	var newGlobalOffset int64
	switch whence {
	case io.SeekStart:
		newGlobalOffset = offset
	case io.SeekCurrent:
		newGlobalOffset = r.globalOffset + offset
	case io.SeekEnd:
		newGlobalOffset = totalSize + offset
	default:
		return r.globalOffset, fmt.Errorf("invalid whence value: %d", whence)
	}

	if newGlobalOffset < 0 {
		return r.globalOffset, fmt.Errorf("negative seek position")
	}

	// 允许在文件末尾 Seek
	if newGlobalOffset > totalSize {
		return r.globalOffset, fmt.Errorf("seek offset %d out of bounds [0, %d]", newGlobalOffset, totalSize)
	}

	// 如果目标位置与当前位置相同，则无需任何操作
	if newGlobalOffset == r.globalOffset {
		return r.globalOffset, nil
	}

	fragIdx, _, ok := r.seekIndex.find(newGlobalOffset)
	if !ok {
		return r.globalOffset, fmt.Errorf("seek position %d not inside any fragment", newGlobalOffset)
	}
	if fragIdx == len(r.streamFragments) {
		r.currentFragmentIndex = len(r.streamFragments)
		r.currentDataReader = nil
		r.globalOffset = newGlobalOffset
		return r.globalOffset, nil
	}

	// 先更新 index，再调用 setup
	r.currentFragmentIndex = fragIdx
	r.globalOffset = newGlobalOffset

	if r.currentRawReader != nil {
		_ = r.currentRawReader.Close()
		r.currentRawReader = nil
	}
	r.currentDataReader = nil

	// setupCurrentFragmentReader 会根据 r.globalOffset 自动定位到正确位置
	if err := r.setupCurrentFragmentReader(); err != nil {
		return r.globalOffset, err
	}

	return r.globalOffset, nil
}

func (r *VirtualSeekableDecryptReader) Close() error {
	if r.currentRawReader != nil {
		return r.currentRawReader.Close()
	}
	return nil
}

// --- 辅助函数 ---

// deriveKeyAndIV 从 KVI 和密码派生密钥和 IV
func deriveKeyAndIV(kviProvider types.KVIProvider, password string) (key, iv []byte, err error) {
	salt, err := crypto.Base64Decode_v2(kviProvider.GetEncryptionInfo().SaltBase64)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode salt: %w", err)
	}
	iv, err = crypto.Base64Decode_v2(kviProvider.GetEncryptionInfo().IVBase64)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode iv: %w", err)
	}
	key = crypto.GenerateKey(password, salt, types.KeySize_v2)
	return key, iv, nil
}

// filterFragmentsByType 筛选出指定类型的 Fragment
func filterFragmentsByType(frags []types.Fragment, fragType string) []types.Fragment {
	var result []types.Fragment
	for _, f := range frags {
		if string(f.Type) == fragType {
			result = append(result, f)
		}
	}
	return result
}
