package video

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/utils/ffmpeg"
	"github.com/Soltus/encv-go/internal/v2/reader"
)

type VideoContentPreprocessor struct {
	settings         VideoPluginConfig
	index            *VideoIndex
	outputDir        string
	splitPartPaths   map[string]bool
	ctx              context.Context
	onFFmpegProgress func(percent float64, speed string)
}

var (
	ffmpegTimeRe  = regexp.MustCompile(`time=(\d+):(\d+):(\d+\.\d+)`)
	ffmpegSpeedRe = regexp.MustCompile(`speed=\s*([\d.]+)\w*`)
	encoderCache  struct {
		sync.Once
		preferred string
	}
)

func detectPreferredEncoder() string {
	encoders := []struct {
		name string
		args []string
	}{
		{"h264_nvenc", []string{"-f", "lavfi", "-i", "nullsrc=s=256x256:d=0.1", "-c:v", "h264_nvenc", "-f", "null", "-"}},
		{"h264_mediacodec", []string{"-f", "lavfi", "-i", "nullsrc=s=256x256:d=0.1", "-c:v", "h264_mediacodec", "-f", "null", "-"}},
		{"libx264", nil},
	}

	for _, enc := range encoders {
		if enc.args == nil {
			return enc.name
		}
		// 🆕 2026-06-15：ffmpeg.Run(ctx, args) → ffmpeg.Encode(ctx, args) 返回 *EncodeResult
		if _, err := ffmpeg.Encode(context.Background(), append([]string{"-y", "-threads", "1"}, enc.args...)...); err == nil {
			slog.Info("Detected available encoder", "component", "CONTENT_PREPROCESSOR", "encoder", enc.name)
			return enc.name
		}
	}
	return "libx264"
}

func getPreferredEncoder() string {
	encoderCache.Do(func() {
		encoderCache.preferred = detectPreferredEncoder()
	})
	return encoderCache.preferred
}

func (p *VideoContentPreprocessor) runFFmpegCmd(args []string, tempPath string) error {
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// 🆕 2026-06-15：ffmpeg.RunWithOutput(ctx, args...) → ffmpeg.Encode(ctx, args...) 返回 *EncodeResult
	res, err := ffmpeg.Encode(ctx, args...)
	stdout := []byte(nil)
	stderrStr := ""
	exitCode := 0
	if res != nil {
		stdout = res.Stdout
		stderrStr = res.Stderr
		exitCode = res.ExitCode
	}
	if err != nil || exitCode != 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg command failed: %w", err)
	}

	var totalDuration float64
	if p.index != nil && p.index.DurationSeconds > 0 {
		totalDuration = p.index.DurationSeconds
	}

	lines := strings.Split(stderrStr, "\n")
	for _, line := range lines {
		if totalDuration > 0 && p.onFFmpegProgress != nil {
			var timeSec float64
			if m := ffmpegTimeRe.FindStringSubmatch(line); len(m) > 3 {
				h, _ := strconv.ParseFloat(m[1], 64)
				mn, _ := strconv.ParseFloat(m[2], 64)
				s, _ := strconv.ParseFloat(m[3], 64)
				timeSec = h*3600 + mn*60 + s
			}

			var speedStr string
			if m := ffmpegSpeedRe.FindStringSubmatch(line); len(m) > 1 {
				speedStr = m[1] + "x"
			}

			if timeSec > 0 {
				percent := (timeSec / totalDuration) * 100
				if percent > 99 {
					percent = 99
				}
				p.onFFmpegProgress(percent, speedStr)
			}
		}
	}

	_ = stdout

	return nil
}

func (p *VideoContentPreprocessor) ensureOutputDir() error {
	if p.outputDir == "" {
		return fmt.Errorf("outputDir is empty")
	}
	info, err := os.Stat(p.outputDir)
	if err == nil && info.IsDir() {
		return nil
	}
	if os.IsNotExist(err) {
		slog.Info("Creating output directory", "dir", p.outputDir)
		return os.MkdirAll(p.outputDir, 0755)
	}
	return fmt.Errorf("outputDir exists but is not a directory: %s", p.outputDir)
}

func (p *VideoContentPreprocessor) Preprocess(inputPath string) (result io.ReadCloser, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Preprocess panicked", "file", filepath.Base(inputPath), "panic", r)
			resultErr = fmt.Errorf("preprocessing failed: internal error: %v", r)
		}
	}()

	slog.Info("Analyzing for optimal processing", "component", "CONTENT_PREPROCESSOR", "file", filepath.Base(inputPath))

	if p.ctx != nil {
		select {
		case <-p.ctx.Done():
			return nil, p.ctx.Err()
		default:
		}
	}

	format, err := ffmpeg.DetectVideoFormat(inputPath)
	if err != nil {
		slog.Warn("Could not detect format, falling back to transcoding", "component", "CONTENT_PREPROCESSOR", "file", filepath.Base(inputPath), "error", err)
		r, path, err := p.transcodeToFastStartMP4(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to transcode to fast-start MP4: %w", err)
		}
		p.updateWithPreprocessedInfo(path, "mp4")
		return r, nil
	}

	isMkv := strings.ToLower(format) == "mkv"
	if isMkv && p.settings.KeepMkvForMkvSource {
		if p.settings.SkipMergeForSplitMKV && p.splitPartPaths != nil && p.splitPartPaths[inputPath] {
			fmt.Println("-> [CONTENT_PREPROCESSOR] Strategy: Split MKV part with SkipMerge enabled. Using original file directly.")
			r, err := os.Open(inputPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open split MKV part: %w", err)
			}
			p.updateWithPreprocessedInfo(inputPath, "mkv")
			return r, nil
		}

		if p.settings.PluginCacheDir != "" {
			cacheDir := filepath.Clean(p.settings.PluginCacheDir)
			inputDir := filepath.Clean(filepath.Dir(inputPath))
			if cacheDir == inputDir {
				fmt.Println("-> [CONTENT_PREPROCESSOR] Strategy: Source is already a merged MKV in cache dir. Using directly.")
				r, err := os.Open(inputPath)
				if err != nil {
					return nil, fmt.Errorf("failed to open cached merged MKV: %w", err)
				}
				p.updateWithPreprocessedInfo(inputPath, "mkv")
				return r, nil
			}
		}

		fmt.Println("-> [CONTENT_PREPROCESSOR] Strategy: Source is MKV and 'keep_mkv' is enabled. Remuxing with mkvmerge.")
		r, path, err := p.remapWithMKVMerge(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to remux MKV: %w", err)
		}
		p.updateWithPreprocessedInfo(path, "mkv")
		return r, nil
	}

	isMp4 := strings.ToLower(format) == "mp4"
	if isMp4 {
		fmt.Println("-> [CONTENT_PREPROCESSOR] Detected MP4, checking for fast-start...")
		if fast, err := isMP4FastStart(inputPath); err == nil && fast {
			fmt.Println("-> [CONTENT_PREPROCESSOR] Input is already a fast-start MP4, using file directly (no processing).")
			r, _ := os.Open(inputPath)
			p.updateWithPreprocessedInfo(inputPath, "mp4")
			return r, nil
		} else {
			if err != nil {
				slog.Warn("Could not verify fast-start status, proceeding with remuxing", "component", "CONTENT_PREPROCESSOR", "error", err)
			} else {
				fmt.Println("-> [CONTENT_PREPROCESSOR] Input is not a fast-start MP4, remuxing is required.")
			}
			r, path, err := p.remapMP4ForFastStart(inputPath)
			if err != nil {
				return nil, fmt.Errorf("failed to remux MP4: %w", err)
			}
			p.updateWithPreprocessedInfo(path, "mp4")
			return r, nil
		}
	}

	slog.Info("Transcoding to fast-start MP4", "component", "CONTENT_PREPROCESSOR", "format", format)
	r, path, err := p.transcodeToFastStartMP4(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to transcode to fast-start MP4: %w", err)
	}
	p.updateWithPreprocessedInfo(path, "mp4")
	return r, nil
}

func (p *VideoContentPreprocessor) updateWithPreprocessedInfo(preprocessedPath, finalFormat string) {
	p.index.Format = finalFormat

	if mimeType, err := utils.DetectFileMIMEType(preprocessedPath); err == nil {
		p.index.MimeType = mimeType
	}

	output, err := ffmpeg.Probe(context.Background(), "-v", "quiet", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", preprocessedPath)
	if err == nil {
		if d, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64); err == nil {
			p.index.DurationSeconds = d
			slog.Info("Updated duration", "component", "CONTENT_PREPROCESSOR", "duration_seconds", d)
		}
	}

	var chapters []MKVChapterInfo
	if finalFormat == "mkv" {
		chapters, err = ExtractChaptersWithMKVExtract(preprocessedPath)
		if err != nil {
			slog.Warn("mkvextract failed, falling back to ffprobe", "component", "CONTENT_PREPROCESSOR", "error", err)
			chapters, err = extractChaptersWithFFprobe(preprocessedPath)
		}
	} else {
		chapters, err = extractChaptersWithFFprobe(preprocessedPath)
	}
	if err == nil && chapters != nil {
		p.index.Chapters = chapters
		slog.Info("Updated chapters", "component", "CONTENT_PREPROCESSOR", "count", len(chapters))
	} else {
		slog.Warn("Could not extract chapters from preprocessed file", "component", "CONTENT_PREPROCESSOR", "error", err)
	}

	keyFrames, err := extractKeyFrameOffsets(preprocessedPath, finalFormat)
	if err == nil {
		p.index.KeyFrameOffsets = keyFrames
		slog.Info("Found keyframes for intelligent splitting", "component", "CONTENT_PREPROCESSOR", "count", len(keyFrames))
	} else {
		slog.Warn("Could not extract keyframes for intelligent splitting", "component", "CONTENT_PREPROCESSOR", "error", err)
		p.index.KeyFrameOffsets = nil
	}
}

func extractKeyFrameOffsets(filePath string, format string) ([]uint64, error) {
	if strings.ToLower(format) == "mkv" {
		return extractKeyFrameOffsetsFromMKV(filePath)
	}

	fmt.Println("-> [DIAG] Attempting optimized FFProbe extraction...")
	offsets, err := extractKeyFrameOffsetsWithFFProbe(filePath)
	if err == nil && len(offsets) > 0 {
		return offsets, nil
	}
	slog.Info("FFProbe failed or empty, attempting binary NAL scan", "component", "DIAG", "error", err)

	return nil, fmt.Errorf("all keyframe extraction methods failed")
}

func extractKeyFrameOffsetsWithFFProbe(filePath string) ([]uint64, error) {
	fmt.Println("-> [DIAG] Optimized: Extracting exact keyframe positions in a single pass.")

	// 🆕 2026-06-15：ffmpeg.Probe(ctx, args) 签名调整
	output, err := ffmpeg.Probe(context.Background(),
		"-v", "error",
		"-select_streams", "v:0",
		"-skip_frame", "nokey",
		"-show_entries", "frame=pkt_pts_time,pkt_pos",
		"-of", "csv=p=0",
		filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("ffprobe keyframe command failed: %w", err)
	}

	var offsets []uint64
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			posStr := strings.TrimSpace(parts[1])

			if posStr == "N/A" || posStr == "" {
				continue
			}

			if offset, err := strconv.ParseUint(posStr, 10, 64); err == nil {
				offsets = append(offsets, offset)
			}
		}
	}

	if len(offsets) == 0 {
		return nil, fmt.Errorf("no valid keyframes found with ffprobe")
	}

	slog.Info("Extracted exact keyframe positions in a single pass", "component", "DIAG", "count", len(offsets))
	return offsets, nil
}

func (p *VideoContentPreprocessor) remapWithMKVMerge(inputPath string) (io.ReadCloser, string, error) {
	fmt.Println("-> [DIAG] Checking original file for Cues...")
	if hasCues, err := checkFileForCues(inputPath); err == nil {
		if hasCues {
			fmt.Println("-> [DIAG] Original file HAS Cues.")
		} else {
			fmt.Println("-> [DIAG] Original file DOES NOT have Cues. This is likely the root cause.")
		}
	} else {
		slog.Warn("Could not check original file for Cues", "component", "DIAG", "error", err)
	}
	if err := p.ensureOutputDir(); err != nil {
		return nil, "", fmt.Errorf("failed to ensure output dir: %w", err)
	}
	tmpDir := filepath.Join(p.outputDir, ".encv_tmp")
	os.MkdirAll(tmpDir, 0755)
	tempFile, err := os.CreateTemp(tmpDir, "encv-pre-*.mkv")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file for MKV remuxing: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	fmt.Println("-> [DIAG] Attempting to create Cues for all video tracks with 'iframes' mode.")
	cmd := exec.Command("mkvmerge", "--cues", "video:iframes", "-o", tempPath, inputPath)
	slog.Info("Executing command", "component", "DIAG", "command", cmd.String())

	if p.ctx != nil {
		select {
		case <-p.ctx.Done():
			os.Remove(tempPath)
			return nil, tempPath, p.ctx.Err()
		default:
		}
	}

	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(tempPath)
		return nil, tempPath, fmt.Errorf("mkvmerge remuxing failed: %w", err)
	}

	hasCues, _ := checkFileForCues(tempPath)
	if !hasCues {
		slog.Error("'video:iframes' succeeded but created no Cues, trying with 'all' as a last resort", "component", "DIAG")

		fmt.Println("-> [DIAG] Attempting to create Cues for all video tracks with 'all' mode.")
		cmdAll := exec.Command("mkvmerge", "--cues", "video:all", "-o", tempPath, inputPath)
		slog.Info("Executing command", "component", "DIAG", "command", cmdAll.String())
		cmdAll.Stderr = os.Stderr
		if err := cmdAll.Run(); err != nil {
			os.Remove(tempPath)
			return nil, tempPath, fmt.Errorf("mkvmerge remuxing failed with both 'iframes' and 'all': %w", err)
		}
		fmt.Println("-> [DIAG] SUCCESS: mkvmerge with 'video:all' Cues succeeded.")
	} else {
		fmt.Println("-> [DIAG] SUCCESS: mkvmerge with 'video:iframes' Cues succeeded and Cues were created.")
	}

	slog.Info("Remuxed MKV", "component", "CONTENT_PREPROCESSOR", "temp_path", tempPath)
	r, _ := reader.NewTempFileReadCloser(tempPath)
	return r, tempPath, nil
}

func (p *VideoContentPreprocessor) remapMKVWithFFmpeg(inputPath string) (io.ReadCloser, string, error) {
	if err := p.ensureOutputDir(); err != nil {
		return nil, "", fmt.Errorf("failed to ensure output dir: %w", err)
	}
	tmpDir := filepath.Join(p.outputDir, ".encv_tmp")
	os.MkdirAll(tmpDir, 0755)
	tempFile, err := os.CreateTemp(tmpDir, "encv-pre-*.mkv")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file for MKV remuxing: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	args := []string{"-y", "-i", inputPath, "-c", "copy", "-reserve_index_space", "500", tempPath}
	// 🆕 2026-06-15：ffmpeg.Run(ctx, args) → ffmpeg.Encode(ctx, args) 返回 *EncodeResult
	if _, err := ffmpeg.Encode(context.Background(), args...); err != nil {
		os.Remove(tempPath)
		return nil, tempPath, fmt.Errorf("ffmpeg MKV remuxing failed: %w", err)
	}

	slog.Info("Remuxed MKV with ffmpeg", "component", "CONTENT_PREPROCESSOR", "temp_path", tempPath)
	r, _ := reader.NewTempFileReadCloser(tempPath)
	return r, tempPath, nil
}

func (p *VideoContentPreprocessor) remapMP4ForFastStart(inputPath string) (io.ReadCloser, string, error) {
	fmt.Println("-> [CONTENT_PREPROCESSOR] Remuxing MP4 for fast-start...")
	if err := p.ensureOutputDir(); err != nil {
		return nil, "", fmt.Errorf("failed to ensure output dir: %w", err)
	}
	tmpDir := filepath.Join(p.outputDir, ".encv_tmp")
	os.MkdirAll(tmpDir, 0755)
	tempFile, err := os.CreateTemp(tmpDir, "encv-pre-*.mp4")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file for MP4 remuxing: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	args := []string{"-y", "-threads", "2", "-i", inputPath, "-c", "copy", "-movflags", "+faststart", tempPath}
	if err := p.runFFmpegCmd(args, tempPath); err != nil {
		os.Remove(tempPath)
		return nil, tempPath, fmt.Errorf("ffmpeg failed to remux MP4: %w", err)
	}

	slog.Info("Remuxed to fast-start", "component", "CONTENT_PREPROCESSOR", "temp_path", tempPath)
	r, _ := reader.NewTempFileReadCloser(tempPath)
	return r, tempPath, nil
}

func (p *VideoContentPreprocessor) transcodeToFastStartMP4(inputPath string) (io.ReadCloser, string, error) {
	fmt.Println("-> [CONTENT_PREPROCESSOR] Transcoding video to H.264/AAC MP4...")
	if err := p.ensureOutputDir(); err != nil {
		return nil, "", fmt.Errorf("failed to ensure output dir: %w", err)
	}
	tmpDir := filepath.Join(p.outputDir, ".encv_tmp")
	os.MkdirAll(tmpDir, 0755)
	tempFile, err := os.CreateTemp(tmpDir, "encv-pre-*.mp4")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file for transcoding: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	encoder := getPreferredEncoder()

	var args []string
	args = append(args, "-y", "-i", inputPath, "-threads", "2")

	switch encoder {
	case "h264_nvenc":
		args = append(args, "-c:v", "h264_nvenc", "-preset", "p4", "-qp", "28", "-profile:v", "high")
	case "h264_mediacodec":
		args = append(args, "-c:v", "h264_mediacodec", "-b:v", "5M")
	default:
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-profile:v", "high")
	}

	args = append(args, "-c:a", "aac", "-movflags", "+faststart", tempPath)

	slog.Info("Using encoder", "component", "CONTENT_PREPROCESSOR", "encoder", encoder, "command", strings.Join(args, " "))

	if err := p.runFFmpegCmd(args, tempPath); err != nil {
		if encoder != "libx264" {
			slog.Warn("Encoder failed, falling back to libx264", "component", "CONTENT_PREPROCESSOR", "encoder", encoder, "error", err)
			os.Remove(tempPath)

			if err := p.ensureOutputDir(); err != nil {
				return nil, "", fmt.Errorf("failed to ensure output dir: %w", err)
			}
			tmpDir := filepath.Join(p.outputDir, ".encv_tmp")
			os.MkdirAll(tmpDir, 0755)
			tempFile2, err2 := os.CreateTemp(tmpDir, "encv-pre-*.mp4")
			if err2 != nil {
				return nil, "", fmt.Errorf("failed to create temp file for fallback transcoding: %w", err2)
			}
			tempPath = tempFile2.Name()
			tempFile2.Close()

			fallbackArgs := []string{"-y", "-i", inputPath, "-threads", "2",
				"-c:v", "libx264", "-preset", "medium", "-crf", "23", "-profile:v", "high",
				"-c:a", "aac", "-movflags", "+faststart", tempPath}
			if err2 := p.runFFmpegCmd(fallbackArgs, tempPath); err2 != nil {
				os.Remove(tempPath)
				return nil, tempPath, fmt.Errorf("ffmpeg failed to transcode video (fallback): %w", err2)
			}
		} else {
			os.Remove(tempPath)
			return nil, tempPath, fmt.Errorf("ffmpeg failed to transcode video: %w", err)
		}
	}

	slog.Info("Transcoded successfully", "component", "CONTENT_PREPROCESSOR", "temp_path", tempPath)
	r, _ := reader.NewTempFileReadCloser(tempPath)
	return r, tempPath, nil
}

func isMP4FastStart(filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	bufferSize := int64(1024 * 1024)
	r := io.LimitReader(file, bufferSize)
	header := make([]byte, bufferSize)
	n, err := io.ReadFull(r, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}
	header = header[:n]

	offset := int64(0)
	for offset < int64(len(header)) {
		if offset+8 > int64(len(header)) {
			break
		}

		atomSize := int64(binary.BigEndian.Uint32(header[offset : offset+4]))
		atomType := string(header[offset+4 : offset+8])

		if atomSize == 1 {
			if offset+16 > int64(len(header)) {
				break
			}
			atomSize = int64(binary.BigEndian.Uint64(header[offset+8 : offset+16]))
			offset += 16
		} else {
			offset += 8
		}

		if atomSize < 8 {
			break
		}

		switch atomType {
		case "moov":
			return true, nil
		case "mdat":
			return false, nil
		}

		offset += atomSize - 8
	}

	return false, nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
