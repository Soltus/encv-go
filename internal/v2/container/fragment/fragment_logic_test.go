package fragment

import (
	"fmt"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/stretchr/testify/assert"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// ========== CreateLogicalFragmentsFromSize 测试 ==========

func TestCreateLogicalFragments_AtomicFile_OneFragment(t *testing.T) {
	frags, err := CreateLogicalFragmentsFromSize(1024, 2048, types.FragmentType_AtomicFile)
	assert.NoError(t, err)
	assert.Len(t, frags, 1)
	assert.Equal(t, uint64(1024), frags[0].Length)
}

func TestCreateLogicalFragments_EvenSplit(t *testing.T) {
	frags, err := CreateLogicalFragmentsFromSize(10000, 2500, types.FragmentType_SeekableStream)
	assert.NoError(t, err)
	assert.Len(t, frags, 4)
	for _, f := range frags {
		assert.Equal(t, uint64(2500), f.Length)
	}
}

func TestCreateLogicalFragments_OddRemainder(t *testing.T) {
	frags, err := CreateLogicalFragmentsFromSize(10000, 3000, types.FragmentType_SeekableStream)
	assert.NoError(t, err)
	assert.Len(t, frags, 4)
	assert.Equal(t, uint64(3000), frags[0].Length)
	assert.Equal(t, uint64(3000), frags[1].Length)
	assert.Equal(t, uint64(3000), frags[2].Length)
	assert.Equal(t, uint64(1000), frags[3].Length)
}

func TestCreateLogicalFragments_SingleByte(t *testing.T) {
	frags, err := CreateLogicalFragmentsFromSize(1, 1, types.FragmentType_SeekableStream)
	assert.NoError(t, err)
	assert.Len(t, frags, 1)
	assert.Equal(t, uint64(1), frags[0].Length)
}

func TestCreateLogicalFragments_ZeroTotalSize(t *testing.T) {
	frags, err := CreateLogicalFragmentsFromSize(0, 1024, types.FragmentType_SeekableStream)
	assert.NoError(t, err)
	assert.Empty(t, frags)
}

func TestCreateLogicalFragments_NegativeTotalSize(t *testing.T) {
	_, err := CreateLogicalFragmentsFromSize(-1, 1024, types.FragmentType_SeekableStream)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "total size cannot be negative")
}

func TestCreateLogicalFragments_ZeroFragSize(t *testing.T) {
	_, err := CreateLogicalFragmentsFromSize(1024, 0, types.FragmentType_SeekableStream)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fragment size must be positive")
}

func TestCreateLogicalFragments_OffsetsAreContinuous(t *testing.T) {
	const fragSize int64 = 2500
	const totalSize int64 = 10000
	frags, err := CreateLogicalFragmentsFromSize(totalSize, fragSize, types.FragmentType_SeekableStream)
	assert.NoError(t, err)

	for i, f := range frags {
		expectedOffset := uint64(i) * uint64(fragSize)
		assert.Equal(t, expectedOffset, f.GlobalStartOffset,
			"fragment %d offset mismatch", i)
	}
}

func TestCreateLogicalFragment_IDsAreSequential(t *testing.T) {
	frags, err := CreateLogicalFragmentsFromSize(10000, 3000, types.FragmentType_SeekableStream)
	assert.NoError(t, err)

	for i, f := range frags {
		expectedID := fmt.Sprintf("logical_fragment_%d", i)
		assert.Equal(t, expectedID, f.ID,
			"fragment %d ID mismatch", i)
	}
}

// ========== CreateLogicalFragmentsFromSizeAligned 测试 ==========

func TestAligned_NoAlignmentNeeded(t *testing.T) {
	const baseLogical = int64(500)
	const physical = int64(500)
	const total = int64(1500)

	frags, err := CreateLogicalFragmentsFromSizeAligned(total, baseLogical, physical, types.FragmentType_SeekableStream)
	assert.NoError(t, err)
	assert.Len(t, frags, 3)
	for _, f := range frags {
		assert.Equal(t, uint64(baseLogical), f.Length)
	}
}

func TestAligned_FragmentsFitInOneChunk(t *testing.T) {
	frags, err := CreateLogicalFragmentsFromSizeAligned(800, 800, 1000, types.FragmentType_SeekableStream)
	assert.NoError(t, err)
	assert.Len(t, frags, 1)
	assert.Equal(t, uint64(800), frags[0].Length)
}

func TestAligned_CrossesBoundary(t *testing.T) {
	const baseLogical = int64(500)
	const physical = int64(1000)
	const total = int64(1500)

	frags, err := CreateLogicalFragmentsFromSizeAligned(total, baseLogical, physical, types.FragmentType_SeekableStream)
	assert.NoError(t, err)

	assert.Len(t, frags, 3)
	assert.Equal(t, uint64(500), frags[0].Length)
	assert.Equal(t, uint64(500), frags[1].Length)
	assert.Equal(t, uint64(500), frags[2].Length)

	assert.Equal(t, uint64(0), frags[0].GlobalStartOffset)
	assert.Equal(t, uint64(500), frags[1].GlobalStartOffset)
	assert.Equal(t, uint64(1000), frags[2].GlobalStartOffset)
}

func TestAligned_BaseLargerThanPhysical(t *testing.T) {
	_, err := CreateLogicalFragmentsFromSizeAligned(1000, 1500, 1000, types.FragmentType_SeekableStream)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be larger than physical chunk size")
}

// ========== ValidateGlobalStartOffsets 测试 ==========

func TestValidate_ValidOffsets(t *testing.T) {
	manifest := &types.Manifest_v2{
		Fragments: []types.Fragment_v2{
			{ID: "f0", Type: types.FragmentType_SeekableStream, Length: 1000, GlobalStartOffset: 0},
			{ID: "f1", Type: types.FragmentType_SeekableStream, Length: 2000, GlobalStartOffset: 1000},
			{ID: "f2", Type: types.FragmentType_SeekableStream, Length: 3000, GlobalStartOffset: 3000},
		},
	}
	err := ValidateGlobalStartOffsets(manifest)
	assert.NoError(t, err)
}

func TestValidate_GapBetweenFragments(t *testing.T) {
	manifest := &types.Manifest_v2{
		Fragments: []types.Fragment_v2{
			{ID: "f0", Type: types.FragmentType_SeekableStream, Length: 1000, GlobalStartOffset: 0},
			{ID: "f1", Type: types.FragmentType_SeekableStream, Length: 2000, GlobalStartOffset: 2000},
		},
	}
	err := ValidateGlobalStartOffsets(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "discontinuous")
}

func TestValidate_OverlappingOffsets(t *testing.T) {
	manifest := &types.Manifest_v2{
		Fragments: []types.Fragment_v2{
			{ID: "f0", Type: types.FragmentType_SeekableStream, Length: 2000, GlobalStartOffset: 0},
			{ID: "f1", Type: types.FragmentType_SeekableStream, Length: 1000, GlobalStartOffset: 1000},
		},
	}
	err := ValidateGlobalStartOffsets(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "discontinuous")
}

func TestValidate_NonSeekableSkipped(t *testing.T) {
	manifest := &types.Manifest_v2{
		Fragments: []types.Fragment_v2{
			{ID: "atomic0", Type: types.FragmentType_AtomicFile, Length: 9999, GlobalStartOffset: 0},
			{ID: "atomic1", Type: types.FragmentType_AtomicFile, Length: 8888, GlobalStartOffset: 7777},
			{ID: "meta", Type: types.FragmentType_Metadata, Length: 100, GlobalStartOffset: 42},
		},
	}
	err := ValidateGlobalStartOffsets(manifest)
	assert.NoError(t, err, "AtomicFile 和 Metadata 类型的分片应被跳过，不检查连续性")
}

// ========== CalculateFragmentSize 测试 ==========

func TestCalculateFragmentSize_SmallFile(t *testing.T) {
	size := CalculateFragmentSize(50*1024*1024, 200*1024*1024)
	assert.LessOrEqual(t, size, int64(120*1024*1024))
	assert.GreaterOrEqual(t, size, int64(50*1024*1024))
}

func TestCalculateFragmentSize_LargeFile(t *testing.T) {
	size := CalculateFragmentSize(10*1024*1024*1024, 500*1024*1024)
	assert.GreaterOrEqual(t, size, int64(4*1024*1024))
	assert.LessOrEqual(t, size, int64(500*1024*1024))
}

func TestCalculateFragmentSize_ZeroPhysical(t *testing.T) {
	const fileSize = int64(500 * 1024 * 1024)
	size := CalculateFragmentSize(fileSize, 0)
	idealSize := fileSize / int64(100)
	assert.Equal(t, idealSize, size, "physicalSize=0 时逻辑大小应等于 fileSize/targetFragments")
}
