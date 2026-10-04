// internal/v2/reader/container_handle_lifecycle_test.go
//
// 回归锁：**全局文件句柄池（globalFileHandlePool）的引用计数必须配平**。
//
// 真实事故（2026-10-01，模拟器内真实后端 + 并发 Range 请求实测）：
//   /stream 的并发请求里约 25% 会失败 —— 客户端收到
//     · 416 "Seek Not Supported"（19 字节）
//     · 或 500（后端 panic：nil pointer dereference，栈在 content.go 的 io.Copy）
//   后端日志同时刷屏：
//     WARN: [fileContainerReader] Failed to close initial main file handle:
//           close <container>: file already closed
//
// 根因链：
//   1. acquireMainFile() 从 globalFileHandlePool.Get() 拿到的是**全局共享**的
//      *os.File（按路径引用计数），并存进 r.initMainFileHandle。
//   2. fileContainerReader.Close() 却直接 initMainFileHandle.Close()，
//      **绕过池**：别的 reader 正拿着同一个 fd 在读，被这一下关掉 ⇒
//      它们的 Read 立刻失败 "file already closed"。
//   3. 上层把读失败当成"文件不可寻址" ⇒ 非 0 偏移的 Range 被拒（416）；
//      LocalFileProvider 的 loadIntoMemory 失败后 GetReader() 返回 nil ⇒
//      ContentHandler 的 io.Copy 对 nil reader 解引用 ⇒ panic ⇒ 500。
//
// 同一个进程里（HTTP 服务就是如此）多个请求共用同一个池，所以这不是"理论并发问题"：
// 播放器边下边播 / 拖动时会并发开多条 Range 连接，真机同款必现。
package reader

import (
	"bytes"
	"crypto/rand"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/Soltus/encv-go/internal/testguard"
)

// newFixtureContainer 写一个真实容器到磁盘（复用 nonce 栈用例的构造法）
func newFixtureContainer(t *testing.T, plainSize int) (string, []byte) {
	t.Helper()
	plain := make([]byte, plainSize)
	for i := range plain {
		plain[i] = byte(i % 251)
	}
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return writeNonceFixture(t, plain, 1024, false, iv), plain
}

// TestFileContainerReader_CloseMustNotBreakOtherReaders
//
// 确定性复现（不依赖调度）：cr1 关掉自己，**不能**让随后从同一个池取句柄的
// cr2 读到一个已关闭的 fd。修前：cr2.Read 返回 "file already closed"。
func TestFileContainerReader_CloseMustNotBreakOtherReaders(t *testing.T) {
	path, _ := newFixtureContainer(t, 8192)

	factory, err := NewDecryptReaderFactory(path, nonceGuardPassword)
	require.NoError(t, err)
	defer factory.Close()
	f := factory.(*decryptReaderFactory)

	newCR := func() *fileContainerReader {
		cr, err := NewFileContainerReaderFromMetadata(
			f.containerPath, f.cachedManifest, f.cachedManifestV4,
			f.cachedHeaderVersion, f.physicalOffsets,
		)
		require.NoError(t, err)
		return cr
	}

	cr1 := newCR()
	fragID := cr1.GetFragments()[0].ID
	// 取一个 fragment reader：这会让池里该路径的引用计数 +1，且尚未归还
	rc1, err := cr1.GetFragmentReader(fragID)
	require.NoError(t, err)
	require.NoError(t, cr1.Close(), "关容器不应该报错")

	// 另一个容器 reader 复用同一个池（HTTP 服务里就是下一个请求）
	cr2 := newCR()
	rc2, err := cr2.GetFragmentReader(fragID)
	require.NoError(t, err)
	defer rc2.Close()

	buf := make([]byte, 64)
	_, err = rc2.Read(buf)
	require.NoError(t, err,
		"关闭一个容器 reader 后，同进程其它 reader 必须仍能读（共享句柄被私自 close 了）")

	_ = rc1.Close()
}

// TestDecryptReaderFactory_ConcurrentReadersSameContainer
//
// 集成层：多 goroutine 通过**同一个（缓存的）factory** 并发读同一个容器，
// 每条流都必须完整、逐字节正确。修前这一条会间歇性失败（读错误 / 内容与明文不符）。
func TestDecryptReaderFactory_ConcurrentReadersSameContainer(t *testing.T) {
	path, plain := newFixtureContainer(t, 64*1024)

	factory, err := NewDecryptReaderFactory(path, nonceGuardPassword)
	require.NoError(t, err)
	defer factory.Close()

	const workers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers*3)

	for round := 0; round < 3; round++ {
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				dr, err := factory.NewDecryptReader()
				if err != nil {
					errCh <- err
					return
				}
				defer dr.Close()
				got, err := io.ReadAll(dr)
				if err != nil {
					errCh <- err
					return
				}
				if !bytes.Equal(got, plain) {
					errCh <- io.ErrUnexpectedEOF
					return
				}
			}()
		}
		wg.Wait()
	}

	select {
	case err := <-errCh:
		t.Fatalf("并发读同一个容器失败：%v（共享句柄被其中一个 reader 私自关闭了？）", err)
	default:
	}
}

// TestSequentialSeekableDecryptReader_CloseIsIdempotent
//
// 大文件（>3MB，走流式分支）用的是 SequentialSeekableDecryptReader。
// 上层真实调用链会**关两次**（server.serveEncryptedFile 里 provider 关一次、
// 老的 `defer decryptReader.Close()` 再关一次）。第二次 Close 会把
// globalFileHandlePool 的引用计数多减一次 ⇒ 共享 fd 提前被关 ⇒
// 并发的其它请求读到 "file already closed"（实体体被截断 / 416 / 500）。
func TestSequentialSeekableDecryptReader_CloseIsIdempotent(t *testing.T) {
	path, _ := newFixtureContainer(t, 4096)
	factory, err := NewDecryptReaderFactory(path, nonceGuardPassword)
	require.NoError(t, err)
	defer factory.Close()

	// poolRefs 读句柄池里该路径当前的引用计数（白盒，但这是唯一能**确定性**
	// 观察"多减一次引用"的方式：共享 fd 被提前关闭只在并发下才显现为读错误）
	poolRefs := func() int64 {
		globalFileHandlePool.mu.Lock()
		defer globalFileHandlePool.mu.Unlock()
		if h, ok := globalFileHandlePool.holds[path]; ok {
			return h.count
		}
		return 0
	}

	// 两条流同时存在（模拟并发请求共用同一个容器的共享句柄）
	r1, err := factory.NewDecryptReader()
	require.NoError(t, err)
	buf := make([]byte, 64)
	_, err = r1.Read(buf)
	require.NoError(t, err)

	r2, err := factory.NewDecryptReader()
	require.NoError(t, err)
	_, err = r2.Read(buf)
	require.NoError(t, err)

	require.NoError(t, r1.Close(), "第一次 Close 不应报错")
	afterFirst := poolRefs()

	require.NoError(t, r1.Close(), "第二次 Close 必须幂等（不能报错）")
	afterSecond := poolRefs()

	assert.Equal(t, afterFirst, afterSecond,
		"重复 Close 把共享句柄的引用计数多减了一次 ⇒ 并发的其它请求会读到 'file already closed'")

	require.NoError(t, r2.Close())
}

// TestDecryptReaderFactory_ConcurrentReaders_LargeStreamedContainer
//
// 大文件（>3MB）走的是**流式分支**，与 ≤3MB 的内存缓存分支是两套代码路径。
// 真机上（8.6MB 样例、并发 3 路 1MB Range）实测约 3/24 条响应被截断，
// 后端日志 "Stream to client was interrupted ...: file already closed"。
func TestDecryptReaderFactory_ConcurrentReaders_LargeStreamedContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("大样例并发用例较慢，-short 下跳过")
	}
	const size = 4 << 20 // 4MB > 3MB 阈值 → 流式分支
	plain := make([]byte, size)
	for i := range plain {
		plain[i] = byte(i % 251)
	}
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("rand: %v", err)
	}
	path := writeNonceFixture(t, plain, 65536, false, iv)

	factory, err := NewDecryptReaderFactory(path, nonceGuardPassword)
	require.NoError(t, err)
	defer factory.Close()

	const workers = 4
	var wg sync.WaitGroup
	errCh := make(chan error, workers*2)
	for round := 0; round < 2; round++ {
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				dr, err := factory.NewDecryptReader()
				if err != nil {
					errCh <- err
					return
				}
				defer dr.Close()
				got, err := io.ReadAll(dr)
				if err != nil {
					errCh <- err
					return
				}
				if !bytes.Equal(got, plain) {
					errCh <- io.ErrUnexpectedEOF
					return
				}
			}()
		}
		wg.Wait()
	}
	select {
	case err := <-errCh:
		t.Fatalf("大文件流式分支并发读失败：%v（共享句柄引用计数被多减 / 被提前关闭）", err)
	default:
	}
}

// TestFileContainerReader_CloseIsIdempotent 重复关闭不得报 "file already closed"
func TestFileContainerReader_CloseIsIdempotent(t *testing.T) {
	path, _ := newFixtureContainer(t, 4096)
	factory, err := NewDecryptReaderFactory(path, nonceGuardPassword)
	require.NoError(t, err)
	defer factory.Close()
	f := factory.(*decryptReaderFactory)

	cr, err := NewFileContainerReaderFromMetadata(
		f.containerPath, f.cachedManifest, f.cachedManifestV4,
		f.cachedHeaderVersion, f.physicalOffsets,
	)
	require.NoError(t, err)
	_, err = cr.GetFragmentReader(cr.GetFragments()[0].ID)
	require.NoError(t, err)

	assert.NoError(t, cr.Close())
	assert.NoError(t, cr.Close(), "重复关闭容器 reader 不应报错")
}
