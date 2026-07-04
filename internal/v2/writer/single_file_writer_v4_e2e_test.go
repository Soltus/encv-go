package writer

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Soltus/encv-go/internal/v2/container/detector"
	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

func init() {
	types.RegisterKVIProvider("video", func(rawKVI json.RawMessage) (types.KVIProvider, error) {
		var kvi types.KVI
		if err := json.Unmarshal(rawKVI, &kvi); err != nil {
			return nil, err
		}
		return testE2EKVI{KVI: kvi}, nil
	})
}

type testE2EKVI struct {
	types.KVI
}

func (k testE2EKVI) GetKind() types.IndexKind     { return "video" }
func (k testE2EKVI) GetEncryptionInfo() types.KVI { return k.KVI }
func (k testE2EKVI) GetIndex() types.Index        { return &types.NoOpIndex{} }

// createV4ViaPluginPath 使用 SingleFileContainerWriterV4（插件加密实际路径）构造完整 V4 容器
// 与 WriteV4Container（直接写入路径）不同，本函数走的是：
//
//	→ WriteHeader(V4 Magic=ENCV)
//	→ WriteKVI (Block 包裹)
//	→ WriteFragment (Block 包裹的加密数据)
//	→ WriteManifest → writeManifestV4 (XOR 混淆，无 Block header)
//	→ Close (回写 Header.ManifestOffset + 写 Footer)
func createV4ViaPluginPath(t *testing.T) (path string, password string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "test_plugin_path.sccgv")
	password = "test-password-e2e"

	salt, err := crypto.GenerateSalt_v2(16)
	require.NoError(t, err)
	_ = crypto.GenerateKey(password, salt, types.KeySize_v2)
	iv, err := crypto.GenerateIV_v2(types.IVSize_v2)
	require.NoError(t, err)

	v4Header := &types.EnvelopeHeaderV4{
		Magic:         types.MagicHeader_v2,
		Version:       4,
		Flags:         1,
		ContainerType: types.ContainerTypeVideo,
		IsSeekable:    1,
		IDType:        uint32(types.IDType_Raw),
		IDLength:      0,
	}

	w, err := NewSingleFileContainerWriterV4(path, v4Header)
	require.NoError(t, err)

	kviData, _ := json.Marshal(map[string]string{
		"salt_base64": base64.StdEncoding.EncodeToString(salt),
		"iv_base64":   base64.StdEncoding.EncodeToString(iv),
		"kind":        "video",
	})
	err = w.WriteKVI(kviData)
	require.NoError(t, err)

	plaintext := make([]byte, 512)
	for i := range plaintext {
		plaintext[i] = byte(i)
	}
	frag := &types.Fragment{
		ID:   "seg_0",
		Type: types.FragmentType_SeekableStream,
	}
	err = w.WriteFragment(frag, plaintext)
	require.NoError(t, err)

	manifestObj := &types.Manifest{
		Kind: "video",
		KVI:  json.RawMessage(kviData),
	}
	err = w.WriteManifest(manifestObj)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	return path, password
}

func TestSingleFileWriterV4_FullRoundtrip_PluginPath(t *testing.T) {
	path, _ := createV4ViaPluginPath(t)

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	magic := make([]byte, 4)
	_, err = f.Read(magic)
	require.NoError(t, err)
	assert.Equal(t, [4]byte{'E', 'N', 'C', 'V'}, [4]byte(magic),
		"文件头 4 字节必须为 ENCV (0x45 0x4E 0x43 0x56)")

	src, err := containerhandle.NewFileSource(path)
	require.NoError(t, err)
	defer src.Close()

	h, err := containerhandle.Open(src)
	require.NoError(t, err, "handle.Open 必须成功打开 SingleFileContainerWriterV4 产生的 V4 容器")
	defer h.Close()

	assert.Equal(t, 4, h.Version(), "容器版本必须为 4")
	assert.NotNil(t, h.ManifestV4(), "ManifestV4 不应为 nil")
	assert.Equal(t, 1, len(h.ManifestV4().Segments), "应有 1 个 segment")
	assert.Equal(t, "seg_0", h.ManifestV4().Segments[0].ID, "Segment ID 应匹配")
	assert.Equal(t, "video", h.ManifestV4().ContainerType, "ContainerType 应为 video")
	assert.True(t, h.IsSeekable(), "IsSeekable 应为 true")

	ct, err := detector.DetectContainer(path)
	require.NoError(t, err, "detector.DetectContainer 应能检测到该容器")
	assert.NotNil(t, ct, "DetectContainer 返回值不应为 nil")
	assert.True(t, ct.IsSeekable, "检测结果 IsSeekable 应为 true")
}

func TestSingleFileWriterV4_MagicBytes(t *testing.T) {
	path, _ := createV4ViaPluginPath(t)

	rawData, err := os.ReadFile(path)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rawData), 4, "文件至少应包含 4 字节 magic")

	assert.EqualValues(t, []byte("ENCV"), rawData[:4],
		"Magic 字节必须精确匹配 ENCV")
	assert.Equal(t, byte(0x45), rawData[0], "Magic[0] = E")
	assert.Equal(t, byte(0x4E), rawData[1], "Magic[1] = N")
	assert.Equal(t, byte(0x43), rawData[2], "Magic[2] = C")
	assert.Equal(t, byte(0x56), rawData[3], "Magic[3] = V")
}

func TestSingleFileWriterV4_ManifestOffsetAccuracy(t *testing.T) {
	path, _ := createV4ViaPluginPath(t)

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	header, err := types.ReadHeaderV4(f)
	require.NoError(t, err, "应能读取 V4 Header")

	assert.NotZero(t, header.ManifestOffset, "ManifestOffset 不能为零")
	assert.NotZero(t, header.ManifestLength, "ManifestLength 不能为零")

	fi, err := f.Stat()
	require.NoError(t, err)
	fileSize := fi.Size()

	manifestEnd := int64(header.ManifestOffset) + int64(header.ManifestLength)
	footerSize := int64(types.EnvelopeFooterSize_v4)
	expectedFooterStart := fileSize - footerSize

	assert.Equal(t, expectedFooterStart, manifestEnd,
		"Manifest 区域末尾应紧接 Footer 起始 (无间隙)")

	obfuscatedData := make([]byte, header.ManifestLength)
	n, err := f.ReadAt(obfuscatedData, int64(header.ManifestOffset))
	require.NoError(t, err)
	require.Equal(t, int(header.ManifestLength), n, "应读到完整的混淆后 Manifest 数据")

	deobfuscated, err := crypto.DeobfuscateManifest(obfuscatedData)
	require.NoError(t, err, "从 ManifestOffset 位置读取的数据必须能成功去混淆")

	var mf types.Manifest_v4
	err = json.Unmarshal(deobfuscated, &mf)
	require.NoError(t, err, "去混淆后的数据必须是合法的 JSON Manifest_v4")
	assert.Equal(t, uint16(4), mf.Version, "反序列化后 Version 必须为 4")
	assert.Equal(t, "video", mf.ContainerType)
	assert.Equal(t, 1, len(mf.Segments))
	assert.Equal(t, "seg_0", mf.Segments[0].ID)
}

func TestSingleFileWriterV4_FooterMagic(t *testing.T) {
	path, _ := createV4ViaPluginPath(t)

	rawData, err := os.ReadFile(path)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rawData), types.EnvelopeFooterSize_v4,
		"文件大小应至少包含 Footer")

	footerData := rawData[len(rawData)-types.EnvelopeFooterSize_v4:]
	assert.EqualValues(t, []byte("ENCV"), footerData[:4],
		"Footer Magic 也必须是 ENCV")
}

func TestSingleFileWriterV4_MultipleFragments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi_frag.sccgv")
	_ = "multi-frag-pw"

	salt, _ := crypto.GenerateSalt_v2(16)
	_ = crypto.GenerateKey("multi-frag-pw", salt, types.KeySize_v2)
	iv, _ := crypto.GenerateIV_v2(types.IVSize_v2)

	v4Header := &types.EnvelopeHeaderV4{
		Magic:         types.MagicHeader_v2,
		Version:       4,
		Flags:         1,
		ContainerType: types.ContainerTypeVideo,
		IsSeekable:    1,
		IDType:        uint32(types.IDType_Raw),
		IDLength:      0,
	}

	w, err := NewSingleFileContainerWriterV4(path, v4Header)
	require.NoError(t, err)

	kviData, _ := json.Marshal(map[string]string{
		"salt_base64": base64.StdEncoding.EncodeToString(salt),
		"iv_base64":   base64.StdEncoding.EncodeToString(iv),
	})
	require.NoError(t, w.WriteKVI(kviData))

	for i := 0; i < 3; i++ {
		data := make([]byte, 256)
		for j := range data {
			data[j] = byte(i*256 + j)
		}
		frag := &types.Fragment{
			ID:   string(rune('a'+byte(i))) + "_seg",
			Type: types.FragmentType_SeekableStream,
		}
		require.NoError(t, w.WriteFragment(frag, data))
	}

	manifestObj := &types.Manifest{
		Kind: "video",
		KVI:  json.RawMessage(kviData),
	}
	require.NoError(t, w.WriteManifest(manifestObj))
	require.NoError(t, w.Close())

	src, err := containerhandle.NewFileSource(path)
	require.NoError(t, err)
	defer src.Close()

	h, err := containerhandle.Open(src)
	require.NoError(t, err)
	defer h.Close()

	assert.Equal(t, 3, len(h.ManifestV4().Segments), "多片段场景：应有 3 个 segments")
	assert.Equal(t, "a_seg", h.ManifestV4().Segments[0].ID)
	assert.Equal(t, "b_seg", h.ManifestV4().Segments[1].ID)
	assert.Equal(t, "c_seg", h.ManifestV4().Segments[2].ID)

	ct, err := detector.DetectContainer(path)
	require.NoError(t, err)
	assert.NotNil(t, ct)
}
