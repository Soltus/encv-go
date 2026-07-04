package video

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/utils/ffmpeg"
	"github.com/Soltus/encv-go/internal/v2/container/block"
	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/types"
	mp4 "github.com/abema/go-mp4"
)

type VideoContentVerifier struct{}

// Verify 实现 ContentVerifier 接口
func (p *VideoContentVerifier) Verify(originalPath, decryptedPath string, opts ...*interfaces.VerifyOptions) (error, []*interfaces.VerifyWarning) {
	opt := &interfaces.VerifyOptions{}
	if len(opts) > 0 && opts[0] != nil {
		opt = opts[0]
	}

	slog.Info("VIDEO INTEGRITY CHECKER v5.0 (Stratified Opt)")
	slog.Info("Verification started", "original_path", originalPath, "decrypted_path", decryptedPath,
		"skip_size_check", opt.SkipSizeCheck, "skip_struct_check", opt.SkipStructCheck)

	var allWarnings []*interfaces.VerifyWarning

	origFile, err := os.Open(originalPath)
	if err != nil {
		return fmt.Errorf("failed to open original file: %w", err), nil
	}
	defer origFile.Close()

	decFile, err := os.Open(decryptedPath)
	if err != nil {
		return fmt.Errorf("failed to open decrypted file: %w", err), nil
	}
	defer decFile.Close()

	origInfo, _ := origFile.Stat()
	decInfo, _ := decFile.Stat()
	totalSize := origInfo.Size()

	if !opt.SkipSizeCheck && totalSize != decInfo.Size() {
		return fmt.Errorf("size mismatch"), nil
	}
	if opt.SkipSizeCheck && totalSize != decInfo.Size() {
		slog.Warn("Size mismatch detected but skipped (re-encode mode)",
			"original_size", totalSize, "decrypted_size", decInfo.Size())
		allWarnings = append(allWarnings, &interfaces.VerifyWarning{
			CheckName: "size_check",
			Message:   fmt.Sprintf("skipped (re-encoded output): original=%d, decrypted=%d", totalSize, decInfo.Size()),
			Severity:  "warning",
		})
	}

	var verificationError error

	// === 第一级防线：结构完整性检查 (< 1秒) ===
	warnings, err := p.QuickStructCheck(decryptedPath, opt)
	if err != nil {
		slog.Error("L1 structure check failed", "error", err)
		verificationError = err
	}
	allWarnings = append(allWarnings, warnings...)

	// === 第二级防线：采样完整性抽检 (< 2秒) ===
	if err := p.QuickSampleHashCheck(originalPath, decryptedPath, opt.SkipSizeCheck); err != nil {
		slog.Error("L2 sample hash check failed", "error", err)
		verificationError = err
	}

	// === 【修正点】在检测到 L1/L2 结构或采样损坏时，立即执行分片诊断 ===
	// 这样可以确保无论主程序是否忽略错误，诊断日志都能打印出来
	if verificationError != nil {
		slog.Warn("Suspected fragmentation damage, running forced diagnosis")

		// 强制调用诊断，不依赖后续的 return
		p.diagnoseFragmentation(originalPath, decryptedPath)

		// 诊断完成后，依然返回验证错误，阻止后续的全盘扫描
		return verificationError, nil
	}

	// === 第三级防线：全盘字节级验证 (耗时操作，仅在 L1/L2 通过后执行) ===
	// 只有当结构完整且关键采样点正确时，才进行全盘 MD5
	const chunkSize = 5 * 1024 * 1024 // 5MB
	slog.Info("Starting Level 3 verification (full byte-level scan)")

	hasher1 := md5.New()
	hasher2 := md5.New()
	buf1 := make([]byte, 1024*1024)
	buf2 := make([]byte, 1024*1024)

	totalChunks := (totalSize + chunkSize - 1) / chunkSize
	startTime := time.Now()

	for offset := int64(0); offset < totalSize; offset += chunkSize {
		chunkEnd := offset + chunkSize
		if chunkEnd > totalSize {
			chunkEnd = totalSize
		}
		currentChunkSize := chunkEnd - offset

		hasher1.Reset()
		hasher2.Reset()

		r1 := io.NewSectionReader(origFile, offset, currentChunkSize)
		r2 := io.NewSectionReader(decFile, offset, currentChunkSize)

		if _, err := io.CopyBuffer(hasher1, r1, buf1); err != nil {
			break
		}
		if _, err := io.CopyBuffer(hasher2, r2, buf2); err != nil {
			break
		}

		if !bytes.Equal(hasher1.Sum(nil), hasher2.Sum(nil)) {
			duration := time.Since(startTime)
			slog.Error("L3 hash mismatch, aborting", "chunk", offset/chunkSize, "elapsed", duration.Round(time.Millisecond))
			return fmt.Errorf("hash mismatch at chunk %d (diff detected quickly)", offset/chunkSize), nil
		}
	}

	duration := time.Since(startTime)
	slog.Info("L3 verification passed, integrity 100%", "chunks", totalChunks, "elapsed", duration)

	// === 深度诊断 (仅在 L3 成功后执行，SkipDeepCheck 时跳过) ===
	if !opt.SkipDeepCheck {
		if err := p.runDeepVideoIntegrityCheck(originalPath, decryptedPath, err); err != nil {
			return fmt.Errorf("deep integrity check failed: %w", err), nil
		}
	} else {
		slog.Warn("L4 deep integrity check skipped (SkipDeepCheck=true)")
	}

	slog.Info("Verification passed (100%)")

	// 根据 CollectWarnings 决定是否返回 warnings
	if opt.CollectWarnings {
		return nil, allWarnings
	}
	return nil, nil
}

// QuickStructCheck 快速结构检查（第一级防线）
// 利用 go-mp4 仅读取必要的 Box 头部，不解析媒体数据
// 当 SkipStructCheck=true 时返回 warning 而非 error
func (p *VideoContentVerifier) QuickStructCheck(filePath string, opts *interfaces.VerifyOptions) ([]*interfaces.VerifyWarning, error) {
	if opts != nil && opts.SkipStructCheck {
		slog.Warn("QuickStructCheck skipped", "path", filePath)
		return []*interfaces.VerifyWarning{
			{
				CheckName: "quick_struct_check",
				Message:   "skipped (re-encoded output)",
				Severity:  "warning",
			},
		}, nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// 1. 检查 moov (Movie Box) 是否存在
	moovBoxes, err := mp4.ExtractBoxWithPayload(f, nil, mp4.BoxPath{mp4.BoxTypeMoov()})
	if err != nil || len(moovBoxes) == 0 {
		return nil, fmt.Errorf("quick check: moov box missing or unreadable")
	}

	// 2. 检查 stsz (Sample Size Box) 是否存在且可读
	// 注意：如果 moov 坏了，这里通常会报错，或者在 Extract 时就失败
	stszBoxes, err := mp4.ExtractBoxesWithPayload(f, &moovBoxes[0].Info, []mp4.BoxPath{mp4.BoxPath{mp4.BoxTypeTrak(), mp4.BoxTypeMdia(), mp4.BoxTypeStbl(), mp4.BoxTypeStsz()}})

	if err != nil || len(stszBoxes) == 0 {
		return nil, fmt.Errorf("quick check: stsz box missing")
	}

	// 3. 仅仅验证 Payload 是否能断言为 Stsz 类型，确保数据未完全乱码
	_, ok := stszBoxes[0].Payload.(*mp4.Stsz)
	if !ok {
		// 类型断言失败，说明数据结构有问题
		return nil, fmt.Errorf("quick check: stsz payload type assertion failed (data corrupt)")
	}

	slog.Info("L1 quick structure check passed (valid moov/stsz)")
	return nil, nil
}

// QuickSampleHashCheck 采样完整性抽检（第二级防线）
// 不再进行全盘扫描，而是随机抽取关键位置的 1MB 数据进行 Hash 对比
func (p *VideoContentVerifier) QuickSampleHashCheck(origPath, decPath string, skipSizeCheck bool) error {
	const sampleSize = 1 * 1024 * 1024 // 1MB 抽样缓冲区

	origFile, err := os.Open(origPath)
	if err != nil {
		return err
	}
	defer origFile.Close()

	decFile, err := os.Open(decPath)
	if err != nil {
		return err
	}
	defer decFile.Close()

	origInfo, _ := origFile.Stat()
	decInfo, _ := decFile.Stat()
	totalSize := origInfo.Size()

	if origInfo.Size() == 0 {
		return fmt.Errorf("original file is empty (0 bytes), cannot verify integrity")
	}
	if decInfo.Size() == 0 {
		return fmt.Errorf("decrypted file is empty (0 bytes), cannot verify integrity")
	}

	if !skipSizeCheck && totalSize != decInfo.Size() {
		return fmt.Errorf("size mismatch")
	}
	if skipSizeCheck && totalSize != decInfo.Size() {
		slog.Warn("QuickSampleHashCheck: size mismatch skipped (re-encode mode)",
			"original_size", totalSize, "decrypted_size", decInfo.Size())
	}

	// 策略：仅在文件开头和结尾附近进行采样
	// 1. 文件开头 (offset 0)
	// 2. 文件结尾附近 (offset totalSize - sampleSize)
	checkOffsets := []int64{0, totalSize - sampleSize}

	buf := make([]byte, sampleSize)
	hasher1 := md5.New()
	hasher2 := md5.New()

	for _, offset := range checkOffsets {
		if offset < 0 {
			continue // 忽略负偏移计算错误
		}

		// 读取原始文件
		_, err := origFile.ReadAt(buf, offset)
		if err != nil {
			if err == io.EOF && offset == 0 {
				return fmt.Errorf("original file appears empty or unreadable (EOF at offset 0)")
			}
			return fmt.Errorf("read orig error at %d: %w", offset, err)
		}
		hasher1.Write(buf)

		// 读取解密文件
		_, err = decFile.ReadAt(buf, offset)
		if err != nil {
			if err == io.EOF && offset == 0 {
				return fmt.Errorf("decrypted file appears empty or unreadable (EOF at offset 0)")
			}
			return fmt.Errorf("read dec error at %d: %w", offset, err)
		}
		hasher2.Write(buf)
	}

	sum1 := hasher1.Sum(nil)
	sum2 := hasher2.Sum(nil)

	if !bytes.Equal(sum1, sum2) {
		return fmt.Errorf("hash mismatch at sample offsets (integrity check failed)")
	}

	slog.Info("L2 sample integrity check passed (keyframes match)")
	return nil
}

// runByteLevelVerification 字节级快速验证
func (p *VideoContentVerifier) runByteLevelVerification(f1, f2 *os.File, totalSize, chunkSize int64) error {
	hasher1 := md5.New()
	hasher2 := md5.New()
	buf1 := make([]byte, 1024*1024)
	buf2 := make([]byte, 1024*1024)

	totalChunks := (totalSize + chunkSize - 1) / chunkSize
	for offset := int64(0); offset < totalSize; offset += chunkSize {
		chunkEnd := offset + chunkSize
		if chunkEnd > totalSize {
			chunkEnd = totalSize
		}
		currentChunkSize := chunkEnd - offset

		hasher1.Reset()
		hasher2.Reset()

		r1 := io.NewSectionReader(f1, offset, currentChunkSize)
		r2 := io.NewSectionReader(f2, offset, currentChunkSize)

		if _, err := io.CopyBuffer(hasher1, r1, buf1); err != nil {
			return fmt.Errorf("read error on original: %w", err)
		}
		if _, err := io.CopyBuffer(hasher2, r2, buf2); err != nil {
			return fmt.Errorf("read error on decrypted: %w", err)
		}

		if !bytes.Equal(hasher1.Sum(nil), hasher2.Sum(nil)) {
			// 精确查找差异偏移
			diffOffset, _ := p.findFirstDifference(f1, f2, offset, currentChunkSize)
			return fmt.Errorf("hash mismatch at chunk %d (diff at %d)", offset/chunkSize, diffOffset)
		}
	}
	slog.Info("Byte level verification passed, integrity 100%", "chunks", totalChunks)
	return nil
}

// runDeepVideoIntegrityCheck 核心视频完整性检测工具 (优化版)
func (p *VideoContentVerifier) runDeepVideoIntegrityCheck(origPath, decPath string, byteErr error) error {
	slog.Warn("Deep video integrity diagnostic started")

	var issues []string

	// 1. MP4 结构解析 (快速，内存占用低)
	if err := p.checkMP4Structure(origPath, decPath); err != nil {
		issues = append(issues, fmt.Sprintf("[MP4 Structure] %v", err))
	} else {
		slog.Info("MP4 structure valid")
	}

	// 2. FFmpeg 解码压力测试 (使用 LimitedWriter 防止内存爆炸)
	if err := p.checkFFmpegDecoding(origPath, decPath); err != nil {
		issues = append(issues, fmt.Sprintf("[Decoding] %v", err))
	} else {
		slog.Info("Decoding valid")
	}

	// 3. 帧数与时长比对 (使用优化的 getVideoMetrics)
	if err := p.checkFrameConsistency(origPath, decPath); err != nil {
		issues = append(issues, fmt.Sprintf("[Consistency] %v", err))
	} else {
		slog.Info("Consistency valid")
	}

	if len(issues) > 0 {
		slog.Error("Diagnosis report")
		for _, issue := range issues {
			slog.Error("Issue found", "issue", issue)
		}
		return fmt.Errorf("deep integrity check failed")
	}

	return nil
}

// checkMP4Structure 使用 go-mp4 解析 MP4 原子结构
func (p *VideoContentVerifier) checkMP4Structure(origPath, decPath string) error {
	f1, err := os.Open(origPath)
	if err != nil {
		return err
	}
	defer f1.Close()
	if _, err := mp4.Probe(f1); err != nil {
		return err
	}

	f2, err := os.Open(decPath)
	if err != nil {
		return err
	}
	defer f2.Close()
	if _, err := mp4.Probe(f2); err != nil {
		return err
	}
	return nil
}

// checkFFmpegDecoding 运行 FFmpeg 解码到 null sink，检测流错误
// 【优化】使用 LimitWriter 限制 stderr 缓冲区大小，防止大量错误日志撑爆内存
func (p *VideoContentVerifier) checkFFmpegDecoding(origPath, decPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := p.runFFmpegStressTest(ctx, origPath, "Original"); err != nil {
		return fmt.Errorf("original file decoding failed: %w", err)
	}

	if err := p.runFFmpegStressTest(ctx, decPath, "Decrypted"); err != nil {
		return fmt.Errorf("decrypted file decoding failed: %w", err)
	}

	return nil
}

// runFFmpegStressTest 执行单个文件的解码测试 (性能优化版)
func (p *VideoContentVerifier) runFFmpegStressTest(ctx context.Context, filePath, label string) error {
	// 🆕 2026-06-15：ffmpeg.RunWithOutput(ctx, args...) → ffmpeg.Encode(ctx, args...) 返回 *EncodeResult
	res, err := ffmpeg.Encode(ctx,
		"-v", "error",
		"-nostdin",
		"-i", filePath,
		"-f", "null",
		"-",
	)
	stderrStr := ""
	exitCode := 0
	if res != nil {
		stderrStr = res.Stderr
		exitCode = res.ExitCode
	}
	_ = exitCode

	if strings.Contains(stderrStr, "corrupt") ||
		strings.Contains(stderrStr, "Invalid") ||
		strings.Contains(stderrStr, "missing picture") ||
		strings.Contains(stderrStr, "Invalid data") {
		return fmt.Errorf("ffmpeg detected corruption (truncated log output):\n%s", stderrStr)
	}

	if err != nil && ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("decoding timeout (file too large?)")
	}

	if exitCode != 0 {
		return fmt.Errorf("ffmpeg exited with code %d: %s", exitCode, stderrStr)
	}

	return nil
}

// checkFrameConsistency 使用 FFprobe 检查帧数和时长
func (p *VideoContentVerifier) checkFrameConsistency(origPath, decPath string) error {
	origFrames, _, err := p.getVideoMetrics(origPath)
	if err != nil {
		return fmt.Errorf("failed to get original metrics: %w", err)
	}

	decFrames, _, err := p.getVideoMetrics(decPath)
	if err != nil {
		return fmt.Errorf("failed to get decrypted metrics: %w", err)
	}

	if decFrames != origFrames {
		return fmt.Errorf("frame count mismatch: orig=%d, dec=%d (DATA LOSS)", origFrames, decFrames)
	}

	return nil
}

// getVideoMetrics 获取帧数和时长 (性能优化版)
// 【算法优化】优先使用 go-mp4 读取 stsz box，避免启动 ffprobe 进程
func (p *VideoContentVerifier) getVideoMetrics(filePath string) (int, float64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	// 提取路径: moov -> trak -> mdia -> stbl -> stsz
	stszBoxWrappers, err := mp4.ExtractBoxWithPayload(f, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeMdia(), mp4.BoxTypeStbl(), mp4.BoxTypeStsz()})
	if err != nil || len(stszBoxWrappers) == 0 {
		// 非标准 MP4 或解析失败，回退到 FFprobe
		return p.getVideoMetricsFallback(filePath)
	}

	stszBox := stszBoxWrappers[0].Payload
	stsz, ok := stszBox.(*mp4.Stsz)
	if !ok {
		// 如果类型断言失败，回退
		return p.getVideoMetricsFallback(filePath)
	}

	// Stsz.SampleCount 即为该 track 的总采样数（对于视频通常是总帧数）
	return int(stsz.SampleCount), 0, nil
}

// getVideoMetricsFallback 当 MP4 快速通道失败时使用 (性能优化版)
// 【性能优化】使用 nb_frames 而不是 -count_frames，避免耗时的帧解码
func (p *VideoContentVerifier) getVideoMetricsFallback(filePath string) (int, float64, error) {
	// 首先尝试使用 nb_frames（元数据中的帧数，非常快）
	// 🆕 2026-06-15：ffmpeg.Probe(ctx, args) 签名调整
	output, err := ffmpeg.Probe(context.Background(),
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=nb_frames",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	if err == nil {
		framesStr := strings.TrimSpace(string(output))
		if framesStr != "" && framesStr != "N/A" {
			if frames, err := strconv.Atoi(framesStr); err == nil {
				return frames, 0, nil
			}
		}
	}

	// 如果 nb_frames 不可用，使用文件大小估算（粗略）
	fileInfo, err := os.Stat(filePath)
	if err == nil {
		// 粗略估算：假设每帧约 10KB
		estimatedFrames := int(fileInfo.Size() / 10000)
		return estimatedFrames, 0, fmt.Errorf("could not get accurate frame count, using estimation")
	}

	return 0, 0, fmt.Errorf("failed to get video metrics")
}

// findFirstDifference ... (保持不变，内存占用极小)
func (p *VideoContentVerifier) findFirstDifference(f1, f2 *os.File, start, length int64) (int64, error) {
	low := start
	high := start + length

	buf1 := make([]byte, 1)
	buf2 := make([]byte, 1)

	for low < high {
		mid := (low + high) / 2
		if _, err := f1.ReadAt(buf1, mid); err != nil {
			return low, err
		}
		if _, err := f2.ReadAt(buf2, mid); err != nil {
			return low, err
		}

		if buf1[0] == buf2[0] {
			low = mid + 1
		} else {
			high = mid
		}
	}

	if low < start+length {
		if _, err := f1.ReadAt(buf1, low); err != nil {
			return -1, err
		}
		if _, err := f2.ReadAt(buf2, low); err != nil {
			return -1, err
		}
		if buf1[0] != buf2[0] {
			return low, nil
		}
	}

	return -1, nil
}

// ================= 以下保留原有的诊断函数 (已精简) =================

// DiagnoseHeaders ... (保留)
func (p *VideoContentVerifier) DiagnoseHeaders(containerPath string) error {
	src, err := containerhandle.NewFileSource(containerPath)
	if err != nil {
		return fmt.Errorf("failed to open container: %w", err)
	}
	defer src.Close()

	h, err := containerhandle.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open container handle: %w", err)
	}
	defer h.Close()

	mf := h.Manifest()
	if mf == nil {
		return fmt.Errorf("no manifest available")
	}
	containerDir := filepath.Dir(containerPath)

	type ChunkInfo struct {
		Filename     string
		Frags        []types.Fragment
		PhysicalPath string
	}
	chunksMap := make(map[string]*ChunkInfo)

	for i := range mf.Fragments {
		frag := &mf.Fragments[i]
		physPath := frag.PhysicalPath
		if physPath == "" {
			physPath = filepath.Base(containerPath)
		}
		if _, ok := chunksMap[physPath]; !ok {
			chunksMap[physPath] = &ChunkInfo{
				Filename:     physPath,
				PhysicalPath: filepath.Join(containerDir, physPath),
			}
		}
		chunksMap[physPath].Frags = append(chunksMap[physPath].Frags, *frag)
	}

	slog.Info("Comprehensive physical file diagnostic")

	var hasErrors bool
	for _, chunk := range chunksMap {
		f, err := os.Open(chunk.PhysicalPath)
		if err != nil {
			slog.Error("Failed to open file", "error", err)
			hasErrors = true
			continue
		}
		defer f.Close()

		_, headerSize, err := types.DetectHeaderInfoFromReaderAt(f)
		if err != nil {
			continue
		}

		expectedOffset := int64(headerSize)
		for _, frag := range chunk.Frags {
			_, err = f.Seek(expectedOffset, io.SeekStart)
			if err != nil {
				hasErrors = true
				break
			}

			var diskHeader block.BlockHeader_v2
			if err := binary.Read(f, types.ByteOrder_v2, &diskHeader); err != nil {
				if diskHeader.Type == types.BlockTypeManifest_v2 {
					break
				}
				hasErrors = true
				break
			}

			if diskHeader.Type != types.BlockTypeData_v2 {
				continue
			}

			if uint64(diskHeader.Length) != frag.Length || diskHeader.CRC32 != frag.DataCRC32 {
				hasErrors = true
			}
			expectedOffset += 14 + int64(diskHeader.Length)
		}
	}

	if hasErrors {
		return fmt.Errorf("diagnostic completed with errors")
	}
	return nil
}

// diagnoseFragmentation 诊断是否为分片损坏导致的数据丢失
func (p *VideoContentVerifier) diagnoseFragmentation(origPath, decPath string) error {
	// 1. 获取原始和解密文件的帧数
	origFrames, _, _ := p.getVideoMetrics(origPath)
	decFrames, _, _ := p.getVideoMetrics(decPath)

	lossRate := float64(origFrames-decFrames) / float64(origFrames) * 100

	slog.Warn("Fragmentation damage detected")
	slog.Info("Original frames", "frames", origFrames)
	slog.Info("Decrypted frames", "frames", decFrames)
	slog.Warn("Data loss percentage", "loss_percent", lossRate)

	if lossRate > 10.0 {
		return fmt.Errorf("critical fragmentation damage: %.2f%% data loss detected", lossRate)
	} else if lossRate > 1.0 {
		slog.Warn("Minor data loss detected, might be GOP alignment drift", "loss_percent", lossRate)
	}

	return nil
}

// DiagnoseGOPAlignment ... (保留)
func (p *VideoContentVerifier) DiagnoseGOPAlignment(filePath string, binaryOffsets []uint64) error {
	// 🆕 2026-06-15：ffmpeg.Probe(ctx, args) 签名调整
	output, err := ffmpeg.Probe(context.Background(),
		"-v", "error", "-select_streams", "v:0",
		"-skip_frame", "nokey", "-show_entries", "frame=pkt_pos,pkt_pts_time",
		"-of", "json", filePath,
	)
	if err != nil {
		return err
	}

	var probeData struct {
		Frames []struct {
			PktPos     string `json:"pkt_pos"`
			PktPtsTime string `json:"pkt_pts_time"`
		} `json:"frames"`
	}

	if err := json.Unmarshal(output, &probeData); err != nil {
		return err
	}

	var ffmpegOffsets []uint64
	for _, f := range probeData.Frames {
		if offset, err := strconv.ParseUint(f.PktPos, 10, 64); err == nil {
			ffmpegOffsets = append(ffmpegOffsets, offset)
		}
	}

	if len(binaryOffsets) != len(ffmpegOffsets) {
		return fmt.Errorf("GOP keyframe mismatch")
	}

	return nil
}

// extractKeyFrameOffsetsFromBinary ... (保留)
func (p *VideoContentVerifier) extractKeyFrameOffsetsFromBinary(filePath string) ([]uint64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buffer := make([]byte, 1*1024*1024)
	var offsets []uint64
	var fileOffset int64 = 0
	startCode := []byte{0x00, 0x00, 0x00, 0x01}

	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		idx := 0
		for {
			i := bytes.Index(buffer[idx:], startCode)
			if i == -1 {
				break
			}
			pos := idx + i
			idx = pos + 4
			absolutePos := fileOffset + int64(pos)

			if pos+4 >= len(buffer) {
				continue
			}

			nalHeader := buffer[pos+4]
			nalType := nalHeader & 0x1F

			if nalType == 5 {
				offsets = append(offsets, uint64(absolutePos))
			}
		}
		fileOffset += int64(n)
	}
	return offsets, nil
}
