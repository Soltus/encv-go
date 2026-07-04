package block

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/stretchr/testify/assert"
)

// TestWriteReadBlock_KVI_Roundtrip 验证 KVI 类型块的写入和读取一致性
func TestWriteReadBlock_KVI_Roundtrip(t *testing.T) {
	originalData := []byte(`{"key": "value"}`)

	var buf bytes.Buffer
	crc, err := WriteBlock(&buf, types.BlockTypeKVI_v2, originalData)
	assert.NoError(t, err)
	assert.NotZero(t, crc)

	header, err := ReadBlockHeader(bytes.NewReader(buf.Bytes()))
	assert.NoError(t, err)
	assert.Equal(t, types.BlockTypeKVI_v2, header.Type)
	assert.Equal(t, uint64(len(originalData)), header.Length)
	assert.Equal(t, crc, header.CRC32)

	data, err := ReadBlockData(bytes.NewReader(buf.Bytes()[GetBlockHeader_v2_Size():]), header)
	assert.NoError(t, err)
	assert.Equal(t, originalData, data)
}

// TestWriteReadBlock_Manifest_Roundtrip 验证 Manifest 类型块的写入和读取一致性
func TestWriteReadBlock_Manifest_Roundtrip(t *testing.T) {
	manifestJSON := []byte(`{"version":2,"kind":"video","fragments":[]}`)

	var buf bytes.Buffer
	crc, err := WriteBlock(&buf, types.BlockTypeManifest_v2, manifestJSON)
	assert.NoError(t, err)
	assert.NotZero(t, crc)

	allBytes := buf.Bytes()
	reader := bytes.NewReader(allBytes)

	header, err := ReadBlockHeader(reader)
	assert.NoError(t, err)
	assert.Equal(t, types.BlockTypeManifest_v2, header.Type)
	assert.Equal(t, uint64(len(manifestJSON)), header.Length)
	assert.Equal(t, crc, header.CRC32)

	data, err := ReadBlockData(reader, header)
	assert.NoError(t, err)
	assert.Equal(t, manifestJSON, data)
}

// TestWriteReadBlock_Data_Roundtrip 验证 Data 类型块使用随机二进制数据的 roundtrip 一致性
func TestWriteReadBlock_Data_Roundtrip(t *testing.T) {
	randomData := make([]byte, 1024)
	rand.Read(randomData)

	var buf bytes.Buffer
	crc, err := WriteBlock(&buf, types.BlockTypeData_v2, randomData)
	assert.NoError(t, err)
	assert.NotZero(t, crc)

	allBytes := buf.Bytes()
	reader := bytes.NewReader(allBytes)

	header, err := ReadBlockHeader(reader)
	assert.NoError(t, err)
	assert.Equal(t, types.BlockTypeData_v2, header.Type)
	assert.Equal(t, uint64(len(randomData)), header.Length)
	assert.Equal(t, crc, header.CRC32)

	data, err := ReadBlockData(reader, header)
	assert.NoError(t, err)
	assert.Equal(t, randomData, data)
}

// TestReadBlockHeader_EmptyReader 验证从空 reader 读取块头应返回错误
func TestReadBlockHeader_EmptyReader(t *testing.T) {
	emptyReader := bytes.NewReader(nil)

	header, err := ReadBlockHeader(emptyReader)
	assert.Error(t, err)
	assert.Nil(t, header)
	assert.ErrorIs(t, err, io.EOF)
}

// TestReadBlockHeader_TruncatedData 验证只写入部分字节时 ReadBlockHeader 应返回错误
func TestReadBlockHeader_TruncatedData(t *testing.T) {
	truncated := make([]byte, 2)
	copy(truncated, []byte{0x01, 0x02})

	header, err := ReadBlockHeader(bytes.NewReader(truncated))
	assert.Error(t, err)
	assert.Nil(t, header)
}

// TestWriteBlock_ReturnsCRC 验证 WriteBlock 返回值是正确的 CRC32 校验值
func TestWriteBlock_ReturnsCRC(t *testing.T) {
	testData := []byte("hello, block world!")

	var buf bytes.Buffer
	crc, err := WriteBlock(&buf, types.BlockTypeData_v2, testData)
	assert.NoError(t, err)

	expectedTotalSize := GetBlockHeader_v2_Size() + int64(len(testData))
	assert.Equal(t, expectedTotalSize, int64(buf.Len()))

	allBytes := buf.Bytes()
	reader := bytes.NewReader(allBytes)

	header, err := ReadBlockHeader(reader)
	assert.NoError(t, err)
	assert.Equal(t, crc, header.CRC32)

	data, err := ReadBlockData(reader, header)
	assert.NoError(t, err)
	assert.Equal(t, testData, data)
}

// TestMultipleBlocks_SequentialRead 验证连续写入多个块后可以按顺序正确读回每个块
func TestMultipleBlocks_SequentialRead(t *testing.T) {
	kviData := []byte(`{"salt":"abc","iv":"def"}`)
	dataPayload := make([]byte, 256)
	rand.Read(dataPayload)
	manifestData := []byte(`{"version":2,"fragments":[{"id":"f1"}]}`)

	var buf bytes.Buffer

	crcKVI, err := WriteBlock(&buf, types.BlockTypeKVI_v2, kviData)
	assert.NoError(t, err)

	crcData, err := WriteBlock(&buf, types.BlockTypeData_v2, dataPayload)
	assert.NoError(t, err)

	crcManifest, err := WriteBlock(&buf, types.BlockTypeManifest_v2, manifestData)
	assert.NoError(t, err)

	reader := bytes.NewReader(buf.Bytes())

	header1, err := ReadBlockHeader(reader)
	assert.NoError(t, err)
	assert.Equal(t, types.BlockTypeKVI_v2, header1.Type)
	assert.Equal(t, crcKVI, header1.CRC32)

	data1, err := ReadBlockData(reader, header1)
	assert.NoError(t, err)
	assert.Equal(t, kviData, data1)

	header2, err := ReadBlockHeader(reader)
	assert.NoError(t, err)
	assert.Equal(t, types.BlockTypeData_v2, header2.Type)
	assert.Equal(t, crcData, header2.CRC32)

	data2, err := ReadBlockData(reader, header2)
	assert.NoError(t, err)
	assert.Equal(t, dataPayload, data2)

	header3, err := ReadBlockHeader(reader)
	assert.NoError(t, err)
	assert.Equal(t, types.BlockTypeManifest_v2, header3.Type)
	assert.Equal(t, crcManifest, header3.CRC32)

	data3, err := ReadBlockData(reader, header3)
	assert.NoError(t, err)
	assert.Equal(t, manifestData, data3)
}
