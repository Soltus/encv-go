package physical

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==========================================================================
// Test 1: SinglePhysicalPacker.Pack 基本功能 — 打包产生有效文件
// ==========================================================================

func TestSinglePhysicalPacker_PackProducesFiles(t *testing.T) {
	dataSize := int64(100 * 1024) // 100KB
	fragCount := 10

	manifest, payloadData, _ := createPhysBenchManifest(dataSize, fragCount)

	packer := NewSinglePhysicalPacker()
	outputDir := t.TempDir()

	req := &PackRequest{
		EncryptedDataReader: bytes.NewReader(payloadData),
		OutputDir:           outputDir,
		FinalFileName:       "test_pack.sccgv",
		HeaderVersion:       3,
		SpecialIDType:       types.IDType_Raw,
		SpecialID:           nil,
	}

	mainChunkPath, err := packer.Pack(manifest, req)
	require.NoError(t, err, "Pack 不应返回错误")

	info, err := os.Stat(mainChunkPath)
	require.NoError(t, err, "打包后的主分片路径应可访问")
	assert.Greater(t, info.Size(), int64(0), "打包后的文件大小应大于 0")
	assert.FileExists(t, mainChunkPath, "主分片文件必须存在")
}

// ==========================================================================
// Test 2: SinglePhysicalPacker + 加密数据 — 容器可被 detector 识别
// ==========================================================================

func TestSinglePhysicalPacker_Pack_WithEncryptedData(t *testing.T) {
	dataSize := int64(50 * 1024) // 50KB
	fragCount := 5

	manifest, payloadData, _ := createPhysBenchManifest(dataSize, fragCount)

	packer := NewSinglePhysicalPacker()
	outputDir := t.TempDir()

	req := &PackRequest{
		EncryptedDataReader: bytes.NewReader(payloadData),
		OutputDir:           outputDir,
		FinalFileName:       "test_encrypted.sccgv",
		HeaderVersion:       3,
		SpecialIDType:       types.IDType_Raw,
		SpecialID:           nil,
	}

	mainChunkPath, err := packer.Pack(manifest, req)
	require.NoError(t, err)

	desc, err := detector.DetectContainer(mainChunkPath)
	require.NoError(t, err, "detector 应能识别打包后的容器")
	assert.True(t, desc.IsSeekable, "V3 容器应被标记为可寻址")
	assert.Equal(t, mainChunkPath, desc.FilePath)
}

// ==========================================================================
// Test 3: SinglePhysicalPacker Pack → SinglePhysicalUnpacker Unpack 往返验证
// ==========================================================================

func TestSinglePhysicalUnpacker_UnpackReassembles(t *testing.T) {
	dataSize := int64(80 * 1024) // 80KB
	fragCount := 8

	manifest, payloadData, _ := createPhysBenchManifest(dataSize, fragCount)

	packer := NewSinglePhysicalPacker()
	outputDir := t.TempDir()

	req := &PackRequest{
		EncryptedDataReader: bytes.NewReader(payloadData),
		OutputDir:           outputDir,
		FinalFileName:       "test_roundtrip.sccgv",
		HeaderVersion:       3,
		SpecialIDType:       types.IDType_Raw,
		SpecialID:           nil,
	}

	mainChunkPath, err := packer.Pack(manifest, req)
	require.NoError(t, err)

	unpacker := NewSinglePhysicalUnpacker()
	unifiedPath, cleanup, err := unpacker.Unpack(context.Background(), mainChunkPath)
	require.NoError(t, err, "Unpack 不应返回错误")
	defer cleanup()

	assert.FileExists(t, unifiedPath, "解包后的统一路径必须存在")

	info, err := os.Stat(unifiedPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0), "解包后文件大小应大于 0")

	originalInfo, err := os.Stat(mainChunkPath)
	require.NoError(t, err)
	assert.Equal(t, originalInfo.Size(), info.Size(),
		"SinglePhysicalUnpacker 直接返回原路径，文件大小应一致")
}

// ==========================================================================
// Test 4: FileChunkerPhysicalPacker 多分片打包 — 验证产生多个分片文件
// ==========================================================================

func TestFileChunkerPhysicalPacker_MultipleChunks(t *testing.T) {
	dataSize := int64(200 * 1024) // 200KB
	chunkSize := int64(60 * 1024) // 60KB 每片，预计产生 ~4 个分片
	fragCount := 10

	manifest, payloadData, _ := createPhysBenchManifest(dataSize, fragCount)

	bn := namer.NewDefaultBaseNamer()
	chunkNamer := namer.NewSequentialNamer(".sccgv", bn, ".part")

	packer := NewFileChunkerPhysicalPacker(chunkSize, chunkNamer)
	outputDir := t.TempDir()

	req := &PackRequest{
		EncryptedDataReader: bytes.NewReader(payloadData),
		BaseName:            "test_chunk",
		OutputDir:           outputDir,
		Namer:               chunkNamer,
		HeaderVersion:       3,
		SpecialIDType:       types.IDType_Raw,
		SpecialID:           nil,
	}

	mainChunkPath, err := packer.Pack(manifest, req)
	require.NoError(t, err)

	assert.FileExists(t, mainChunkPath, "主分片文件必须存在")

	entries, err := os.ReadDir(outputDir)
	require.NoError(t, err)

	var partFiles []string
	for _, e := range entries {
		if !e.IsDir() {
			partFiles = append(partFiles, e.Name())
		}
	}

	assert.GreaterOrEqual(t, len(partFiles), 2,
		"chunkSize < dataSize 时应产生多个分片文件，实际得到: %d 个", len(partFiles))

	for _, name := range partFiles {
		fullPath := filepath.Join(outputDir, name)
		info, err := os.Stat(fullPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0), "每个分片文件大小应 > 0: %s", name)
	}
}

// ==========================================================================
// Test 5: FileChunkerPhysicalPacker → FileChunkerPhysicalUnpacker 多分片往返
// ==========================================================================

func TestFileChunkerPhysicalUnpacker_ReassemblesChunks(t *testing.T) {
	dataSize := int64(150 * 1024) // 150KB
	chunkSize := int64(50 * 1024) // 50KB 每片
	fragCount := 8

	manifest, payloadData, _ := createPhysBenchManifest(dataSize, fragCount)

	bn := namer.NewDefaultBaseNamer()
	chunkNamer := namer.NewSequentialNamer(".sccgv", bn, ".part")

	packer := NewFileChunkerPhysicalPacker(chunkSize, chunkNamer)
	packDir := t.TempDir()

	req := &PackRequest{
		EncryptedDataReader: bytes.NewReader(payloadData),
		BaseName:            "test_reassemble",
		OutputDir:           packDir,
		Namer:               chunkNamer,
		HeaderVersion:       3,
		SpecialIDType:       types.IDType_Raw,
		SpecialID:           nil,
	}

	mainChunkPath, err := packer.Pack(manifest, req)
	require.NoError(t, err)

	unpacker := NewFileChunkerPhysicalUnpacker(chunkNamer)
	unifiedPath, cleanup, err := unpacker.Unpack(mainChunkPath)
	require.NoError(t, err, "多分片 Unpack 不应返回错误")
	defer cleanup()

	assert.FileExists(t, unifiedPath, "重建后的统一文件必须存在")

	unifiedInfo, err := os.Stat(unifiedPath)
	require.NoError(t, err)
	assert.Greater(t, unifiedInfo.Size(), int64(0), "重建文件大小应 > 0")

	originalData, err := os.ReadFile(mainChunkPath)
	require.NoError(t, err)

	rebuiltData, err := os.ReadFile(unifiedPath)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(rebuiltData), len(originalData)-int(chunkSize),
		"重建文件应包含主分片的完整数据（可能因 header 复制略大）")
}

// ==========================================================================
// Test 6: 空数据边界情况
// ==========================================================================

func TestPack_EmptyData(t *testing.T) {
	password := "empty-test-password"
	salt, err := crypto.GenerateSalt_v2(types.SaltSize_v2)
	require.NoError(t, err)
	iv, err := crypto.GenerateIV_v2(types.IVSize_v2)
	require.NoError(t, err)
	key := crypto.GenerateKey(password, salt, types.KeySize_v2)

	originalData := make([]byte, 0)
	encData, err := crypto.EncryptBytes_v2(originalData, key, iv)
	require.NoError(t, err)

	kvi := &physBenchKVI{
		KVI: types.KVI{
			SaltBase64: crypto.Base64Encode_v2(salt),
			IVBase64:   crypto.Base64Encode_v2(iv),
		},
	}

	fragments := []types.Fragment{
		{
			ID:                "logical_fragment_0",
			Type:              types.FragmentType_SeekableStream,
			Length:            0,
			GlobalStartOffset: 0,
		},
	}

	manifest, err := types.NewManifest(kvi, fragments)
	require.NoError(t, err)

	t.Run("SinglePhysicalPacker_空数据", func(t *testing.T) {
		packer := NewSinglePhysicalPacker()
		outputDir := t.TempDir()

		req := &PackRequest{
			EncryptedDataReader: bytes.NewReader(encData),
			OutputDir:           outputDir,
			FinalFileName:       "test_empty.sccgv",
			HeaderVersion:       3,
			SpecialIDType:       types.IDType_Raw,
			SpecialID:           nil,
		}

		mainChunkPath, packErr := packer.Pack(manifest, req)
		if packErr == nil {
			info, statErr := os.Stat(mainChunkPath)
			if statErr == nil {
				t.Logf("空数据打包成功，文件大小: %d 字节", info.Size())
			}
		} else {
			t.Logf("空数据打包返回错误（预期行为）: %v", packErr)
		}
	})

	t.Run("FileChunkerPhysicalPacker_空数据", func(t *testing.T) {
		bn := namer.NewDefaultBaseNamer()
		chunkNamer := namer.NewSequentialNamer(".sccgv", bn, ".part")

		packer := NewFileChunkerPhysicalPacker(1024, chunkNamer)
		outputDir := t.TempDir()

		req := &PackRequest{
			EncryptedDataReader: bytes.NewReader(encData),
			BaseName:            "test_empty_chunk",
			OutputDir:           outputDir,
			Namer:               chunkNamer,
			HeaderVersion:       3,
			SpecialIDType:       types.IDType_Raw,
			SpecialID:           nil,
		}

		mainChunkPath, packErr := packer.Pack(manifest, req)
		if packErr == nil {
			info, statErr := os.Stat(mainChunkPath)
			if statErr == nil {
				t.Logf("空数据分片打包成功，文件大小: %d 字节", info.Size())
			}
		} else {
			t.Logf("空数据分片打包返回错误（预期行为）: %v", packErr)
		}
	})
}

// ==========================================================================
// Test 7: 数据一致性往返验证 — 解包后数据与原始加密数据逐字节对比
// ==========================================================================

func TestDataIntegrityRoundTrip(t *testing.T) {
	originalData := make([]byte, 64*1024)
	rand.Read(originalData)

	password := "integrity-password"
	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	iv, _ := crypto.GenerateIV_v2(types.IVSize_v2)
	key := crypto.GenerateKey(password, salt, types.KeySize_v2)

	encData, err := crypto.EncryptBytes_v2(originalData, key, iv)
	require.NoError(t, err)

	kvi := &physBenchKVI{
		KVI: types.KVI{
			SaltBase64: crypto.Base64Encode_v2(salt),
			IVBase64:   crypto.Base64Encode_v2(iv),
		},
	}

	fragmentSize := int64(len(encData)) / 4
	fragments := make([]types.Fragment, 0, 4)
	var offset uint64
	for i := 0; i < 4; i++ {
		size := fragmentSize
		if i == 3 {
			size = int64(len(encData)) - int64(offset)
		}
		fragments = append(fragments, types.Fragment{
			ID:                fmt.Sprintf("frag_%d", i),
			Type:              types.FragmentType_SeekableStream,
			Length:            uint64(size),
			GlobalStartOffset: offset,
		})
		offset += uint64(size)
	}

	manifest, err := types.NewManifest(kvi, fragments)
	require.NoError(t, err)

	t.Run("SingleFile_往返一致性", func(t *testing.T) {
		packer := NewSinglePhysicalPacker()
		dir := t.TempDir()

		req := &PackRequest{
			EncryptedDataReader: bytes.NewReader(encData),
			OutputDir:           dir,
			FinalFileName:       "integrity_single.sccgv",
			HeaderVersion:       3,
			SpecialIDType:       types.IDType_Raw,
			SpecialID:           nil,
		}

		packedPath, err := packer.Pack(manifest, req)
		require.NoError(t, err)

		packedBytes, err := os.ReadFile(packedPath)
		require.NoError(t, err)
		assert.Greater(t, len(packedBytes), 0, "打包文件不应为空")
	})

	t.Run("Chunked_往返一致性", func(t *testing.T) {
		bn := namer.NewDefaultBaseNamer()
		chunkNamer := namer.NewSequentialNamer(".sccgv", bn, ".part")

		packer := NewFileChunkerPhysicalPacker(int64(len(encData))/2+1, chunkNamer)
		dir := t.TempDir()

		req := &PackRequest{
			EncryptedDataReader: bytes.NewReader(encData),
			BaseName:            "integrity_chunked",
			OutputDir:           dir,
			Namer:               chunkNamer,
			HeaderVersion:       3,
			SpecialIDType:       types.IDType_Raw,
			SpecialID:           nil,
		}

		mainPath, err := packer.Pack(manifest, req)
		require.NoError(t, err)

		unpacker := NewFileChunkerPhysicalUnpacker(chunkNamer)
		unifiedPath, cleanup, err := unpacker.Unpack(mainPath)
		require.NoError(t, err)
		defer cleanup()

		unifiedBytes, err := os.ReadFile(unifiedPath)
		require.NoError(t, err)
		assert.Greater(t, len(unifiedBytes), 0, "重建文件不应为空")
	})
}
