package webdav

import (
	"errors"
	"io"
	"log/slog"
	"os"

	"github.com/Soltus/encv-go/internal/v2/provider"
)

// webdavFileAdapter 将 provider.FileContentProvider 适配为 goWebdav.File
type webdavFileAdapter struct {
	provider provider.FileContentProvider
	// reader 可能是原始的 reader，也可能是被 seekableWrapper 包装过的
	reader   io.ReadCloser
	fileInfo os.FileInfo
}

// newWebDAVFileAdapter 创建一个新的适配器实例
func newWebDAVFileAdapter(prov provider.FileContentProvider, fileInfo os.FileInfo) (*webdavFileAdapter, error) {
	reader := prov.GetReader()
	if reader == nil {
		return nil, errors.New("provider returned a nil reader")
	}

	// 【关键集成】检查 reader 是否支持 Seek
	if _, ok := reader.(io.Seeker); !ok {
		// 如果不支持，就用 seekableWrapper 包装它
		slog.Debug("Reader is not seekable, wrapping with seekableWrapper")
		reader = newSeekableWrapper(reader)
	}

	return &webdavFileAdapter{
		provider: prov,
		reader:   reader,
		fileInfo: fileInfo,
	}, nil
}

// --- 实现 goWebdav.File 接口 ---

func (a *webdavFileAdapter) Read(p []byte) (n int, err error) {
	return a.reader.Read(p)
}

// 【简化】Seek 方法现在可以非常简单，因为 reader 在构造时已经保证了支持 Seek
func (a *webdavFileAdapter) Seek(offset int64, whence int) (int64, error) {
	if seeker, ok := a.reader.(io.Seeker); ok {
		return seeker.Seek(offset, whence)
	}
	// 理论上，代码不应该执行到这里
	return 0, errors.New("seek is not supported, but it should have been wrapped")
}

func (a *webdavFileAdapter) Close() error {
	var readerErr, providerErr error
	if a.reader != nil {
		readerErr = a.reader.Close()
	}
	if a.provider != nil {
		providerErr = a.provider.Close()
	}
	if readerErr != nil {
		return readerErr
	}
	return providerErr
}

// ... (Readdir, Stat, Write 方法保持不变) ...

func (a *webdavFileAdapter) Readdir(count int) ([]os.FileInfo, error) {
	return nil, os.ErrInvalid
}

func (a *webdavFileAdapter) Stat() (os.FileInfo, error) {
	return a.fileInfo, nil
}

func (a *webdavFileAdapter) Write(p []byte) (n int, err error) {
	return 0, os.ErrPermission
}
