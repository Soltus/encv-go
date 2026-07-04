package reader

import (
	"crypto/cipher"
	"fmt"
	"io"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// SequentialSeekableDecryptReader 用于解密 SeekableStream 类型的 fragments
// 支持顺序读取和 Seek 操作
type SequentialSeekableDecryptReader struct {
	containerReader  EncryptedContainerReader
	key, iv          []byte
	fragments        []types.Fragment
	seekIndex        *fragmentRangeIndex
	currentIndex     int
	currentReader    io.ReadCloser
	currentDecryptor io.Reader
	currentOffset    int64 // 当前在全局数据流中的位置
	totalSize        int64 // 总数据大小
	discardBufPool   sync.Pool
}

func NewSequentialSeekableDecryptReader(cr EncryptedContainerReader, password string) (DecryptReader, error) {
	return newSequentialSeekableDecryptReader(cr, password, nil)
}

func newSequentialSeekableDecryptReader(cr EncryptedContainerReader, password string, prebuiltIndex *fragmentRangeIndex) (DecryptReader, error) {
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

	r := &SequentialSeekableDecryptReader{
		containerReader: cr,
		key:             key,
		iv:              iv,
		fragments:       index.fragments,
		seekIndex:       index,
		totalSize:       index.total(),
		currentIndex:    0,
		currentOffset:   0,
		discardBufPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 32*1024)
			},
		},
	}
	return r, nil
}

func (r *SequentialSeekableDecryptReader) Read(p []byte) (n int, err error) {
	if len(r.fragments) == 0 {
		return 0, io.EOF
	}
	if r.currentDecryptor == nil {
		if err := r.setupFragmentAtIndex(r.currentIndex); err != nil {
			return 0, err
		}
	}
	n, err = r.currentDecryptor.Read(p)
	r.currentOffset += int64(n)
	if err == io.EOF {
		r.currentReader.Close()
		r.currentReader = nil
		r.currentDecryptor = nil
		r.currentIndex++
		// 【关键修复】如果已经读取了一些字节，先返回这些字节
		// 下次调用 Read 时会继续读取下一个 fragment
		if n > 0 {
			return n, nil
		}
		// 如果没有读取到字节，递归读取下一个 fragment
		return r.Read(p)
	}
	return n, err
}

func (r *SequentialSeekableDecryptReader) setupFragmentAt(index int, localOffset uint64) error {
	if index >= len(r.fragments) {
		return io.EOF
	}
	frag := r.fragments[index]
	if localOffset > frag.Length {
		return fmt.Errorf("invalid local offset %d for fragment %s", localOffset, frag.ID)
	}

	if r.currentReader != nil {
		_ = r.currentReader.Close()
		r.currentReader = nil
		r.currentDecryptor = nil
	}

	// 获取 Fragment 的 Reader
	rawReader, err := r.containerReader.GetFragmentReader(frag.ID)
	if err != nil {
		return fmt.Errorf("failed to get reader for fragment %s: %w", frag.ID, err)
	}

	absoluteOffset := frag.GlobalStartOffset
	needDiscard := localOffset > 0

	if localOffset > 0 {
		if seeker, ok := rawReader.(io.Seeker); ok {
			if _, seekErr := seeker.Seek(int64(localOffset), io.SeekStart); seekErr == nil {
				absoluteOffset += localOffset
				needDiscard = false
			}
		}
	}

	stream, err := buildCTRStreamAtOffset(r.key, r.iv, absoluteOffset)
	if err != nil {
		_ = rawReader.Close()
		return err
	}
	streamReader := &cipher.StreamReader{S: stream, R: rawReader}

	if needDiscard {
		if err := discardReaderBytes(streamReader, localOffset, &r.discardBufPool); err != nil {
			_ = rawReader.Close()
			return err
		}
	}

	r.currentReader = rawReader
	r.currentDecryptor = streamReader
	r.currentIndex = index
	return nil
}

func (r *SequentialSeekableDecryptReader) setupFragmentAtIndex(index int) error {
	return r.setupFragmentAt(index, 0)
}

func (r *SequentialSeekableDecryptReader) Close() error {
	if r.currentReader != nil {
		r.currentReader.Close()
	}
	return r.containerReader.Close()
}

// Seek 实现 io.Seeker 接口，支持 HTTP Range 请求
func (r *SequentialSeekableDecryptReader) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = r.currentOffset + offset
	case io.SeekEnd:
		newOffset = r.totalSize + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}

	if newOffset < 0 {
		return 0, fmt.Errorf("cannot seek to negative offset: %d", newOffset)
	}
	if newOffset > r.totalSize {
		return 0, fmt.Errorf("cannot seek beyond end of file: %d > %d", newOffset, r.totalSize)
	}

	targetIndex, localOffset, ok := r.seekIndex.find(newOffset)
	if !ok {
		return 0, fmt.Errorf("could not find fragment for offset %d", newOffset)
	}
	if targetIndex == len(r.fragments) {
		r.currentIndex = targetIndex
		r.currentOffset = newOffset
		if r.currentReader != nil {
			_ = r.currentReader.Close()
			r.currentReader = nil
			r.currentDecryptor = nil
		}
		return newOffset, nil
	}

	if err := r.setupFragmentAt(targetIndex, localOffset); err != nil {
		return 0, err
	}

	r.currentOffset = newOffset
	return newOffset, nil
}
