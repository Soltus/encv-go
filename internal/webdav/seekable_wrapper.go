package webdav

import (
	"bytes"
	"errors"
	"io"
)

// maxCacheSize 定义了 seekableWrapper 的最大缓存大小，防止内存溢出
const maxCacheSize = 100 * 1024 * 1024 // 100MB

// seekableWrapper 将一个不可寻址的 io.Reader 包装成一个可寻址的 io.Reader
type seekableWrapper struct {
	source    io.Reader    // 底层数据源
	buffer    bytes.Buffer // 内存缓存
	pos       int64        // 当前读取位置
	totalRead int64        // 从 source 读取的总字节数
}

// newSeekableWrapper 创建一个新的包装器
func newSeekableWrapper(source io.Reader) *seekableWrapper {
	return &seekableWrapper{
		source: source,
	}
}

// Read 实现了 io.Reader 接口
func (s *seekableWrapper) Read(p []byte) (int, error) {
	// 1. 如果当前位置在缓存范围内，从缓存读取
	bufferBytes := s.buffer.Bytes()
	if s.pos < int64(len(bufferBytes)) {
		n := copy(p, bufferBytes[s.pos:])
		s.pos += int64(n)
		// 如果 p 已被填满，直接返回
		if n == len(p) {
			return n, nil
		}
		// 否则，继续从 source 读取剩余部分
		p = p[n:]
	}

	// 2. 从 source 读取新数据
	n, err := s.source.Read(p)
	s.totalRead += int64(n)

	// 3. 将新数据写入缓存（如果未超过限制）
	if s.buffer.Len()+n <= maxCacheSize {
		s.buffer.Write(p[:n])
	}

	// 4. 更新位置
	s.pos += int64(n)

	return n, err
}

// Seek 实现了 io.Seeker 接口
func (s *seekableWrapper) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = s.pos + offset
	case io.SeekEnd:
		// 对于非 Seek 流，获取总长度需要读完整个文件，代价太高。
		// 我们不支持这种操作，WebDAV 客户端也很少使用。
		return 0, errors.New("seek from end is not supported for non-seekable streams")
	default:
		return 0, errors.New("invalid whence value")
	}

	if newPos < 0 {
		return 0, errors.New("negative position")
	}

	// 如果目标位置在缓存范围内，直接移动指针
	if newPos <= int64(s.buffer.Len()) {
		s.pos = newPos
		return s.pos, nil
	}

	// 如果目标位置超出缓存，需要从 source 读取并丢弃数据
	// 计算需要读取的字节数
	toReadAndDiscard := newPos - int64(s.buffer.Len())

	// 注意：这些数据我们也要缓存起来，所以不能直接用 io.CopyN
	discardBuf := make([]byte, 32*1024) // 32KB 缓冲区
	var totalDiscarded int64
	for totalDiscarded < toReadAndDiscard {
		readSize := int64(len(discardBuf))
		if toReadAndDiscard-totalDiscarded < readSize {
			readSize = toReadAndDiscard - totalDiscarded
		}
		n, err := s.source.Read(discardBuf[:readSize])
		if err != nil && err != io.EOF {
			return 0, err
		}
		if n == 0 {
			break // 到达文件末尾
		}

		totalDiscarded += int64(n)
		s.totalRead += int64(n)

		// 将丢弃的数据也写入缓存（如果未超过限制）
		if s.buffer.Len()+n <= maxCacheSize {
			s.buffer.Write(discardBuf[:n])
		}
	}

	s.pos = newPos
	return s.pos, nil
}

// Close 实现了 io.Closer 接口
// 如果 source 实现了 Close，则关闭它
func (s *seekableWrapper) Close() error {
	if closer, ok := s.source.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
