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
	closed           bool // 【并发安全】Close 必须幂等，见 Close 的注释
}

func NewSequentialSeekableDecryptReader(cr EncryptedContainerReader, password string) (DecryptReader, error) {
	return newSequentialSeekableDecryptReader(cr, password, nil)
}

func newSequentialSeekableDecryptReader(cr EncryptedContainerReader, password string, prebuiltIndex *fragmentRangeIndex) (DecryptReader, error) {
	manifest := cr.GetManifest()

	key, iv, err := deriveKeyAndIV(cr, password)
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

	// 分片自带 nonce（v4 segment 栈适配而来）→ keystream 从这个分片自己的计数器**从 0 开始**，
	// 偏移量取分片内偏移；否则沿用旧行为：KVI 的 iv + 全局偏移。
	baseIV, streamOffset := r.iv, absoluteOffset
	if fragNonce := decodeFragmentNonce(frag.Nonce); len(fragNonce) > 0 {
		baseIV, streamOffset = fragNonce, localOffset
		if needDiscard {
			streamOffset = 0 // 无法 seek，靠丢弃推进时计数器也从 0 起算
		}
	}

	stream, err := buildCTRStreamAtOffset(r.key, baseIV, streamOffset)
	if err != nil {
		_ = rawReader.Close()
		return err
	}
	// 【完整性】从分片起点读时，边读边校验密文 CRC（见 crcGuardReader 的注释）。
	// 损坏的数据必须变成错误，不能当明文往外吐。
	streamReader := &cipher.StreamReader{S: stream, R: crcGuardFor(rawReader, &frag, localOffset)}

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

// Close 释放本条流占用的资源。
//
// 【关键】必须幂等：上层存在**重复 Close** 的真实调用链
// （server.serveEncryptedFile 里 `defer decryptReader.Close()` +
//  `defer prov.Close()` 会各关一次同一个 decryptReader）。
// 不幂等的后果不是"多打一条日志"，而是**把全局共享文件句柄的引用计数多减一次**
// （fileContainerReader 的句柄来自 globalFileHandlePool，按路径共享）⇒
// 计数提前归零 ⇒ fd 在别的并发请求还在读时被关闭 ⇒
// 那些请求拿到 "file already closed"，表现为 416 / 500 / 实体体被截断
// （2026-10-02 模拟器内后端实测：大文件并发 Range 约 3/24 条被截断）。
func (r *SequentialSeekableDecryptReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.currentReader != nil {
		_ = r.currentReader.Close()
		r.currentReader = nil
		r.currentDecryptor = nil
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
