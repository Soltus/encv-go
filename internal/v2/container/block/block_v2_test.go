// internal/v2/container/block/block_v2_test.go
package block

import (
	"bytes"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

func TestReadWriteBlock(t *testing.T) {
	data := []byte("this is some test data for a block")
	var buf bytes.Buffer

	// Write block
	_, err := WriteBlock(&buf, types.BlockTypeKVI_v2, data)
	if err != nil {
		t.Fatalf("Failed to write block: %v", err)
	}

	// Read block
	bufReader := bytes.NewReader(buf.Bytes())
	header, err := ReadBlockHeader(bufReader)
	if err != nil {
		t.Fatalf("Failed to read block header: %v", err)
	}

	if header.Type != types.BlockTypeKVI_v2 {
		t.Errorf("Expected block type %d, got %d", types.BlockTypeKVI_v2, header.Type)
	}
	if header.Length != uint64(len(data)) {
		t.Errorf("Expected length %d, got %d", len(data), header.Length)
	}

	readData, err := ReadBlockData(bufReader, header)
	if err != nil {
		t.Fatalf("Failed to read block data: %v", err)
	}

	if !bytes.Equal(readData, data) {
		t.Errorf("Read data does not match original data")
	}
}
