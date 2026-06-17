// cmd/gen-sccgv-fixture/main.go
//
// 🆕 2026-06-17：为 WebDAV 自动化测试生成真实可解密的 v4 加密容器 fixture
//
// 调用 NewSingleFileContainerWriterV4 生成完整 v4 ENCV 容器（不是 mock_generator 的占位字节）。
// 真机 EncV App 可用此 fixture 跑 encrypted_container_preview 测试 module。
//
// 用法：
//   go run ./cmd/gen-sccgv-fixture \
//     -password "test-pass" \
//     -output /tmp/encv-test-fixtures \
//     -name container \
//     -size 8192
//
// 生成的容器：
//   <output>/<name>.sccgv
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/v2/container/manifest"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

func main() {
	password := flag.String("password", "test-pass", "Encryption password (for fixture roundtrip test)")
	outputDir := flag.String("output", "/tmp/encv-test-fixtures", "Output directory")
	name := flag.String("name", "container", "Base name (no extension)")
	sizeKB := flag.Int("size", 8, "Plaintext size in KB")
	ext := flag.String("ext", "mp4", "Original file extension (for container manifest hint)")
	flag.Parse()

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("mkdir %s: %v", *outputDir, err)
	}

	salt, err := crypto.GenerateSalt_v2(16)
	if err != nil {
		log.Fatalf("GenerateSalt: %v", err)
	}
	iv, err := crypto.GenerateIV_v2(types.IVSize_v2)
	if err != nil {
		log.Fatalf("GenerateIV: %v", err)
	}
	_ = crypto.GenerateKey(*password, salt, types.KeySize_v2)

	v4Header := &types.EnvelopeHeaderV4{
		Magic:         types.MagicHeader_v2,
		Version:       4,
		Flags:         1,
		ContainerType: types.ContainerTypeVideo,
		IsSeekable:    1,
		IDType:        uint32(types.IDType_Raw),
		IDLength:      0,
	}

	// 注册 video KVI provider（createV4ViaPluginPath 的 init 一样）
	if !types.HasKVIProvider("video") {
		types.RegisterKVIProvider("video", func(rawKVI json.RawMessage) (types.KVIProvider, error) {
			var kvi types.KVI
			if err := json.Unmarshal(rawKVI, &kvi); err != nil {
				return nil, err
			}
			return &defaultKVI{KVI: kvi}, nil
		})
	}

	plaintext := make([]byte, *sizeKB*1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	outPath := filepath.Join(*outputDir, fmt.Sprintf("%s.sccgv", *name))
	w, err := writer.NewSingleFileContainerWriterV4(outPath, v4Header)
	if err != nil {
		log.Fatalf("NewSingleFileContainerWriterV4: %v", err)
	}

	kviData, _ := json.Marshal(map[string]string{
		"salt_base64": base64.StdEncoding.EncodeToString(salt),
		"iv_base64":   base64.StdEncoding.EncodeToString(iv),
		"kind":        "video",
		"ext":         *ext,
	})
	if err := w.WriteKVI(kviData); err != nil {
		log.Fatalf("WriteKVI: %v", err)
	}

	frag := &types.Fragment{
		ID:   "seg_0",
		Type: types.FragmentType_SeekableStream,
	}
	if err := w.WriteFragment(frag, plaintext); err != nil {
		log.Fatalf("WriteFragment: %v", err)
	}

	manifestObj := &manifest.Manifest{
		Kind: "video",
		KVI:  json.RawMessage(kviData),
	}
	if err := w.WriteManifest(manifestObj); err != nil {
		log.Fatalf("WriteManifest: %v", err)
	}
	if err := w.Close(); err != nil {
		log.Fatalf("Close: %v", err)
	}

	// 打印使用说明
	fmt.Println("✓ v4 fixture generated:")
	fmt.Printf("  path:     %s\n", outPath)
	fmt.Printf("  password: %s\n", *password)
	fmt.Printf("  size:     %d KB\n", *sizeKB)
	fmt.Println()
	fmt.Println("Push to Android device:")
	fmt.Printf("  adb push %s /storage/emulated/0/encv-automation/03-encv-containers/%s.sccgv\n", outPath, *name)
}

// defaultKVI 默认 KVI provider，仿照 createV4ViaPluginPath 的 testE2EKVI
type defaultKVI struct {
	types.KVI
}

func (k *defaultKVI) GetKind() types.IndexKind     { return "video" }
func (k *defaultKVI) GetEncryptionInfo() types.KVI { return k.KVI }
func (k *defaultKVI) GetIndex() types.Index        { return &types.NoOpIndex{} }
