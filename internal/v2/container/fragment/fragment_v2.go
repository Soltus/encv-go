package fragment // 逻辑分片

import (
	"fmt"
	"log/slog"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/dustin/go-humanize"
)

// fragmentLogger 是 fragment 包的日志记录器
var fragmentLogger = logger.WithComponent("fragment")

// CreateLogicalFragmentsFromSizeAligned 创建一个与物理分片边界对齐的逻辑分片列表。
// 这确保了没有任何一个逻辑分片会跨越物理分片的边界，对于兼容底层物理打包器至关重要。
//
// 参数:
//   - totalSize: 原始数据的总大小。
//   - baseLogicalSize: 期望的逻辑分片大小（例如，由 CalculateFragmentSize 计算得出）。
//   - physicalChunkSize: 物理分片的大小。
//   - fragType: 逻辑分片的类型。
func CreateLogicalFragmentsFromSizeAligned(totalSize int64, baseLogicalSize int64, physicalChunkSize int64, fragType types.FragmentType_v2) ([]types.Fragment_v2, error) {
	if baseLogicalSize <= 0 {
		return nil, fmt.Errorf("base logical size must be positive")
	}
	if physicalChunkSize <= 0 {
		return nil, fmt.Errorf("physical chunk size must be positive")
	}
	if totalSize < 0 {
		return nil, fmt.Errorf("total size cannot be negative")
	}
	if baseLogicalSize > physicalChunkSize {
		return nil, fmt.Errorf("base logical size (%s) cannot be larger than physical chunk size (%s)", humanize.Bytes(uint64(baseLogicalSize)), humanize.Bytes(uint64(physicalChunkSize)))
	}

	var fragments []types.Fragment_v2
	var currentOffset uint64 = 0
	fragmentIndex := 0

	for currentOffset < uint64(totalSize) {
		// 计算当前物理分片的结束边界
		currentPhysicalChunkIndex := currentOffset / uint64(physicalChunkSize)
		endOfPhysicalChunk := (currentPhysicalChunkIndex + 1) * uint64(physicalChunkSize)

		// 计算此物理分片中剩余的空间
		remainingSpaceInChunk := endOfPhysicalChunk - currentOffset

		// 确定当前逻辑分片的大小
		// 它是我们期望的大小，但最大不能超过当前物理分片的剩余空间
		fragmentSize := uint64(baseLogicalSize)
		if fragmentSize > remainingSpaceInChunk {
			fragmentSize = remainingSpaceInChunk
		}

		// 确保不会因为计算问题产生0大小的分片
		if fragmentSize == 0 {
			break
		}

		fragID := fmt.Sprintf("logical_fragment_%d", fragmentIndex)
		frag := types.Fragment_v2{
			ID:                fragID,
			Type:              fragType,
			Length:            fragmentSize,
			GlobalStartOffset: currentOffset,
		}
		fragments = append(fragments, frag)

		currentOffset += fragmentSize
		fragmentIndex++
	}

	return fragments, nil
}

// 根据文件总大小和用户配置，智能计算最终的分片大小
func CalculateFragmentSize(totalFileSize int64, userConfiguredPhysicalSize int64) int64 {

	const (
		minLogicalSize        int64 = 4 * 1024 * 1024   // 逻辑分片最小 4MB
		defaultMaxLogicalSize int64 = 120 * 1024 * 1024 // 默认逻辑分片最大 120MB
		largeMaxLogicalSize   int64 = 240 * 1024 * 1024 // 极限逻辑分片最大 240MB
		smallFileThreshold    int64 = 100 * 1024 * 1024 // 小文件阈值
		targetFragments       int64 = 100               // 大文件的目标分片数
	)

	// 1. 【核心修正】确定最终的物理分片大小
	// userConfiguredPhysicalSize == 0 表示不分片，整个加密流作为一个物理块
	var physicalChunkSize int64
	if userConfiguredPhysicalSize > 0 {
		physicalChunkSize = userConfiguredPhysicalSize
	} else {
		physicalChunkSize = totalFileSize // 不分片，物理块大小等于文件大小
	}

	// 2. 动态确定逻辑分片的最大值
	maxLogicalSize := defaultMaxLogicalSize
	// 当物理块很大（或不分片）且文件本身也很大时，启用更大的逻辑分片
	if (userConfiguredPhysicalSize == 0 || userConfiguredPhysicalSize > 1*1024*1024*1024) && totalFileSize > 30*1024*1024*1024 {
		maxLogicalSize = largeMaxLogicalSize
		fragmentLogger.Info("large file detected, using increased logical chunk size",
			slog.String("max_logical_size", humanize.Bytes(uint64(maxLogicalSize))),
			slog.String("file_size", humanize.Bytes(uint64(totalFileSize))),
		)
	}

	// 3. 【关键约束】逻辑分片的最大值不能超过物理分片的大小
	if maxLogicalSize > physicalChunkSize {
		maxLogicalSize = physicalChunkSize
	}

	var logicalChunkSize int64

	// 4. 对小文件进行特殊处理
	if totalFileSize <= smallFileThreshold {
		logicalChunkSize = min(totalFileSize, maxLogicalSize)
	} else {
		// 5. 对大文件，使用目标分片数进行智能计算
		idealSize := totalFileSize / int64(targetFragments)
		if idealSize < minLogicalSize {
			idealSize = minLogicalSize
		} else if idealSize > maxLogicalSize {
			idealSize = maxLogicalSize
		}
		logicalChunkSize = idealSize
	}

	// 6. 最终大小也不能小于最小逻辑大小
	if logicalChunkSize < minLogicalSize {
		logicalChunkSize = minLogicalSize
	}

	// 7. 最终大小不能超过物理分片大小（此检查为健壮性保留）
	if logicalChunkSize > physicalChunkSize {
		logicalChunkSize = physicalChunkSize
	}

	return logicalChunkSize
}

// CreateLogicalFragmentsFromSize 仅根据总大小和分片大小，创建逻辑分片元数据。
// 这是最高效的方式，因为它不需要任何 I/O 操作。
func CreateLogicalFragmentsFromSize(totalSize int64, fragmentSize int64, frag_type types.FragmentType_v2) ([]types.Fragment_v2, error) {
	if fragmentSize <= 0 {
		return nil, fmt.Errorf("fragment size must be positive")
	}
	if totalSize < 0 {
		return nil, fmt.Errorf("total size cannot be negative")
	}

	var logicalFragments []types.Fragment_v2
	var globalOffset uint64 = 0
	chunkIndex := 0
	remaining := totalSize

	for remaining > 0 {
		currentChunkSize := fragmentSize
		if remaining < fragmentSize {
			currentChunkSize = remaining
		}

		fragID := fmt.Sprintf("logical_fragment_%d", chunkIndex) // ID 生成策略可能需要泛化
		frag := types.Fragment_v2{
			ID:                fragID,
			Type:              frag_type,
			Length:            uint64(currentChunkSize),
			GlobalStartOffset: globalOffset,
		}
		logicalFragments = append(logicalFragments, frag)
		globalOffset += uint64(currentChunkSize)
		remaining -= currentChunkSize
		chunkIndex++
	}

	return logicalFragments, nil
}

func ValidateGlobalStartOffsets(manifest *types.Manifest_v2) error {
	var lastEnd uint64 = 0
	for _, frag := range manifest.Fragments {
		if frag.Type != types.FragmentType_SeekableStream {
			continue
		}
		if frag.GlobalStartOffset != lastEnd {
			return fmt.Errorf(
				"fragment %s has discontinuous GlobalStartOffset: got %d, expected %d (previous end)",
				frag.ID, frag.GlobalStartOffset, lastEnd,
			)
		}
		lastEnd += uint64(frag.Length)
	}
	fragmentLogger.Info("validated fragment offsets",
		slog.Uint64("total_logical_size", lastEnd),
		slog.Int("fragment_count", len(manifest.Fragments)),
	)
	return nil
}
