// internal/v2/container/compose/contract_test.go
//
// 互认契约：**compose 产出的容器必须能被主应用那条路径原样解开**。
//
// 为什么这条锁最关键：
//
//	主应用 = CLI（cmd/encv）+ Capacitor 应用内嵌的 Go 后端（internal/server →
//	plugins.EncryptFileWithPlugin / DecryptContainerWithPlugin）。wasm 预览页走 compose。
//	两条路只要产物互不认，表现就是"浏览器加密的容器 CLI 解不开"（历史上正是
//	**长度正确、内容全乱、一声不响**）。所以这里不用"自己解自己"，而是
//	直接调用主应用**同一个入口** pkg/encv.DecryptPathV2 来验证。
package compose_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/container/compose"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/Soltus/encv-go/internal/v2/plugins/text"
	textplugin "github.com/Soltus/encv-go/internal/v2/plugins/text"
	"github.com/Soltus/encv-go/internal/v2/reader"
)

const contractPassword = "contract-password"

// fileSink 把流式加密的产出写进文件：head ‖ 碎块 ‖ tail。
type fileSink struct {
	f    *os.File
	head []byte
	tail []byte
}

func (s *fileSink) Append(p []byte) error {
	_, err := s.f.Write(append([]byte(nil), p...))
	return err
}

func (s *fileSink) Finish(head []byte, tail []byte) error {
	s.head = append([]byte(nil), head...)
	s.tail = append([]byte(nil), tail...)
	return nil
}

// writeContractContainer 用 compose 的**流式**通道产出一个容器文件。
func writeContractContainer(t *testing.T, path string, plain []byte) {
	t.Helper()

	// 头必须先于数据落盘，但头里的 manifest 偏移只有写完全部数据才知道 —— 所以
	// 先写个占位，最后把 head/tail 拼到正确位置（与前端 Blob 拼装同一套约定）。
	tmp, err := os.CreateTemp(filepath.Dir(path), "compose-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpName := tmp.Name()
	tmp.Close()

	f, err := os.OpenFile(tmpName, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	sink := &fileSink{f: f}

	// 插件 index 由调用方装配（这里模拟主应用塞 text 插件的 index）
	enc, err := compose.NewStreamEncryptor(compose.Options{
		Password:      contractPassword,
		ContainerType: 5, // text
		OriginalName:  "contract.txt",
		MimeType:      "text/plain; charset=utf-8",
		Format:        "plain",
		SegmentSize:   4096, // 强制多段
		KVIExtra: func(plainSize int64, plainMD5 string) map[string]interface{} {
			return map[string]interface{}{
				"text_index": &textplugin.TextIndex{
					ID:                "0",
					OriginalFileSize:  plainSize,
					MimeType:          "text/plain; charset=utf-8",
					Format:            "plain",
					OriginalFilename:  "contract.txt",
					OriginalInputPath: "contract.txt",
					OriginalFileMD5:   plainMD5,
				},
			}
		},
	}, sink)
	if err != nil {
		t.Fatalf("NewStreamEncryptor: %v", err)
	}
	if _, err := enc.Write(plain); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	f.Close()

	// 组装成完整容器：head ‖ 数据 ‖ tail
	data, err := os.ReadFile(tmpName)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	out := make([]byte, 0, len(sink.head)+len(data)+len(sink.tail))
	out = append(out, sink.head...)
	out = append(out, data...)
	out = append(out, sink.tail...)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	os.Remove(tmpName)
}

// TestComposeContainerOpensByMainlineReader compose 产物必须能被主线 reader 打开，
// 且明文长度/内容都对（这是 wasm 侧自己在用的那条路）。
func TestComposeContainerOpensByMainlineReader(t *testing.T) {
	plain := bytes.Repeat([]byte("compose contract "), 5000)
	dir := t.TempDir()
	container := filepath.Join(dir, "contract.txt.sccgt")
	writeContractContainer(t, container, plain)

	info, err := reader.OpenV4Container(container, contractPassword)
	if err != nil {
		t.Fatalf("主线 reader 打不开 compose 产物：%v", err)
	}
	sr, err := reader.NewSegmentSeekableReader(info, "")
	if err != nil {
		t.Fatalf("NewSegmentSeekableReader: %v", err)
	}
	defer sr.Close()
	got, err := readAll(sr)
	if err != nil {
		t.Fatalf("读明文失败：%v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("compose 产物被主线 reader 解出的内容与明文不一致（%d vs %d 字节）", len(got), len(plain))
	}
}

// TestComposeContainerDecryptedByMainApp compose 产物必须能被**主应用入口**解开
// —— 也就是 CLI 与 Capacitor 内嵌后端共用的 pkg/encv.DecryptPathV2。
func TestComposeContainerDecryptedByMainApp(t *testing.T) {
	plain := bytes.Repeat([]byte("主应用与 wasm 必须互认 "), 3000)
	dir := t.TempDir()
	container := filepath.Join(dir, "contract.txt.sccgt")
	writeContractContainer(t, container, plain)

	// 主应用的加解密都经过插件层。这里显式初始化 text 插件（口令与插件设置来自
	// 配置上下文，和 CLI 的 -p、Capacitor 内嵌后端读的是同一份配置）。
	// ⚠️ 插件必须先 Initialize：未初始化时 p.cfg 是 nil，取口令会直接空指针 panic。
	ctx := config.NewContext(context.Background(), &config.Config{
		Password:       contractPassword,
		PluginSettings: map[string]json.RawMessage{"text": json.RawMessage(`{"ext":".sccgt"}`)},
	})
	plugin := new(text.TextPlugin)
	if err := plugin.Initialize(ctx); err != nil {
		t.Fatalf("初始化 text 插件失败：%v", err)
	}

	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// DecryptContainerWithPlugin 就是 CLI 与 Capacitor 后端共用的那条高层 API
	if _, err := plugins.DecryptContainerWithPlugin(ctx, plugin, container, outDir, nil); err != nil {
		t.Fatalf("主应用入口解不开 compose 产物：%v", err)
	}

	// 找出解出的文件（插件会按 index 里的原始文件名命名）
	var found string
	filepath.WalkDir(outDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatalf("解密输出为空：%s", outDir)
	}
	got, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("读解密结果失败：%v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("主应用解出的内容与明文不一致（%d vs %d 字节）", len(got), len(plain))
	}
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	b := make([]byte, 32*1024)
	for {
		n, err := r.Read(b)
		buf.Write(b[:n])
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return buf.Bytes(), err
		}
	}
	return buf.Bytes(), nil
}
