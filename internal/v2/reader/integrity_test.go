package reader

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"

	_ "github.com/Soltus/encv-go/internal/testguard"
)

// ============================================================================
// 回归锁：吐字节前的整片预校验（reader.VerifyContainerIntegrity）
// ----------------------------------------------------------------------------
// 这是 server（/stream、/decrypt）与 service（外部文件）两条路径**共用**的同一份
// 实现，别在调用方各写一套。
//
// 存在理由：分块 CRC 只能"读到损坏块才中止"，那时 HTTP 头已经发出去了 ⇒
// 客户端只会看到"播到一半卡住"。预校验让我们能在吐第一个字节前返回 422。
// ============================================================================

func newLocalFactory(t *testing.T, path, password string) DecryptReaderFactory {
	t.Helper()
	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory: %v", err)
	}
	return factory
}

// TestVerifyContainerIntegrity_Healthy 正常容器必须放行（不能误伤正常文件）
func TestVerifyContainerIntegrity_Healthy(t *testing.T) {
	path, password := writeIntegrityContainer(t, bytes.Repeat([]byte("healthy"), 500))
	factory := newLocalFactory(t, path, password)
	defer factory.Close()

	if err := VerifyContainerIntegrity(factory, 8<<20); err != nil {
		t.Errorf("正常容器被误判为损坏：%v", err)
	}
}

// TestVerifyContainerIntegrity_Tampered 篡改必须被拦下（本机制存在的意义）
func TestVerifyContainerIntegrity_Tampered(t *testing.T) {
	path, password := writeIntegrityContainer(t, bytes.Repeat([]byte("tampered"), 500))
	tamperCipherByte(t, path)

	factory := newLocalFactory(t, path, password)
	defer factory.Close()

	err := VerifyContainerIntegrity(factory, 8<<20)
	if err == nil {
		t.Fatal("篡改后的容器竟然通过了预校验 ⇒ 客户端仍会收到损坏数据")
	}
	if !errors.Is(err, types.ErrDataCorrupted) {
		t.Errorf("期望 types.ErrDataCorrupted（上层映射 422），实际：%v", err)
	}
}

// TestVerifyContainerIntegrity_Skips 不该干活的场合必须直接跳过（别拖慢首字节）
func TestVerifyContainerIntegrity_Skips(t *testing.T) {
	path, password := writeIntegrityContainer(t, bytes.Repeat([]byte("skip"), 500))
	tamperCipherByte(t, path) // 文件确实是坏的，但下面几种情况都不该去校验
	factory := newLocalFactory(t, path, password)
	defer factory.Close()

	// 超过阈值（大文件交给分块 CRC，避免首字节变慢）
	if err := VerifyContainerIntegrity(factory, MaxPreVerifySize+1); err != nil {
		t.Errorf("超过 %d 字节就不该预校验，却返回：%v", MaxPreVerifySize, err)
	}
	// 大小为 0 / 负数
	if err := VerifyContainerIntegrity(factory, 0); err != nil {
		t.Errorf("size=0 不该预校验，却返回：%v", err)
	}
	// 非本地工厂（远程/alist 流没有 NewRawContainerReader）
	if err := VerifyContainerIntegrity(remoteOnlyFactory{}, 8<<20); err != nil {
		t.Errorf("非本地工厂必须跳过，却返回：%v", err)
	}
}

// TestVerifyContainerIntegrity_LegacyNoCRC 老容器（无 CRC 元数据）必须不受影响
func TestVerifyContainerIntegrity_LegacyNoCRC(t *testing.T) {
	// 直接构造"没有 CRC 元数据"的 fragment：走真实容器 reader，但把 CRC 抹成 0
	path, password := writeIntegrityContainer(t, bytes.Repeat([]byte("legacy"), 500))
	factory := newLocalFactory(t, path, password)
	defer factory.Close()

	raw, ok := factory.(RawContainerFactory)
	if !ok {
		t.Fatalf("本地工厂应当实现 RawContainerFactory")
	}
	cr, err := raw.NewRawContainerReader()
	if err != nil {
		t.Fatalf("NewRawContainerReader: %v", err)
	}
	defer cr.Close()
	frags := cr.GetFragments()
	if len(frags) == 0 {
		t.Fatal("容器没有 fragment")
	}
	saved := frags[0].DataCRC32
	frags[0].DataCRC32 = 0 // 伪装成老容器
	defer func() { frags[0].DataCRC32 = saved }()

	if err := VerifyContainerIntegrity(factory, 8<<20); err != nil {
		t.Errorf("老容器（DataCRC32=0）必须跳过校验，却返回：%v", err)
	}
}

// --- 小工具 ---

// writeIntegrityContainer 造一个本地 v4 容器（fragment 栈，writer 会写 CRC 元数据）。
//
// ⚠️ 不能用 compose 造：`compose → plugins/text → reader`，在 reader 包的测试里
//    会形成 import cycle。改用包内既有的夹具 createV4PluginPathContainer（writer 直出）。
func writeIntegrityContainer(t *testing.T, plain []byte) (string, string) {
	t.Helper()
	return createV4PluginPathContainer(t, types.ContainerTypeVideo, plain)
}

// fragment 栈的 v4 容器：数据区紧跟 2048 字节头，没有 segment header
func tamperCipherByte(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	var one [1]byte
	const off = 2048 + 8
	if _, err := f.ReadAt(one[:], off); err != nil {
		t.Fatalf("read: %v", err)
	}
	one[0] ^= 0x01
	if _, err := f.WriteAt(one[:], off); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// remoteOnlyFactory 模拟远程/alist 工厂：没有 NewRawContainerReader ⇒ 必须被跳过
type remoteOnlyFactory struct{ DecryptReaderFactory }
