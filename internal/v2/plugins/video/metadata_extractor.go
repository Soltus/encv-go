// internal/v2/plugins/video/metadata_extractor.go

package video

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/utils/ffmpeg"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// logger 是视频插件的日志记录器
var videoLogger = logger.WithComponent("video.metadata_extractor")

// VideoMetadataExtractor 实现 plugins.MetadataExtractor 接口
type VideoMetadataExtractor struct {
	// 可以在这里注入依赖，例如配置
	settings VideoPluginConfig
	index    *VideoIndex
}

// ExtractMetadata 提取视频元数据
func (e *VideoMetadataExtractor) ExtractMetadata(inputPath string) (types.Index, error) {
	videoLogger.Info("analyzing video",
		slog.String("file", filepath.Base(inputPath)),
		slog.String("path", inputPath),
	)

	metadata, err := extractMetadataFromOriginalFile(inputPath)
	if err != nil {
		videoLogger.Error("failed to extract metadata",
			slog.String("file", filepath.Base(inputPath)),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("failed to get metadata from original file: %w", err)
	}

	// 安全检查：确保 metadata 不为 nil
	if metadata == nil {
		videoLogger.Error("metadata extraction returned nil",
			slog.String("file", inputPath),
		)
		return nil, fmt.Errorf("metadata extraction returned nil for file: %s", inputPath)
	}

	// 将提取出的所有信息复制到共享的 index 中
	if e.index == nil {
		e.index = &VideoIndex{}
	}
	*e.index = *metadata
	e.index.OriginalInputPath = inputPath

	videoLogger.Info("metadata extraction completed",
		slog.String("file", filepath.Base(inputPath)),
		slog.Int("width", metadata.Width),
		slog.Int("height", metadata.Height),
		slog.Float64("duration", metadata.DurationSeconds),
	)

	// 【关键】返回共享 index 的地址
	return e.index, nil
}

// sanitizeFFProbeOutput 清理 ffprobe 输出的 JSON 数据，提高解析容错性。
// 返回清理后的数据和可能的警告信息（如果数据有问题但可修复）。
// 如果 JSON 被截断无法修复，返回错误。
func sanitizeFFProbeOutput(data []byte) ([]byte, string, error) {
	var warnings []string

	data = removeBOM(data, &warnings)
	data = removeTrailingCommas(data, &warnings)

	if err := checkJSONBalanced(data); err != nil {
		return nil, "", fmt.Errorf("ffprobe output appears truncated: %w", err)
	}

	warningStr := strings.Join(warnings, "; ")
	return data, warningStr, nil
}

func removeBOM(data []byte, warnings *[]string) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		*warnings = append(*warnings, "removed UTF-8 BOM")
		return data[3:]
	}
	if len(data) >= 2 {
		if data[0] == 0xFE && data[1] == 0xFF {
			*warnings = append(*warnings, "removed UTF-16 BE BOM (data may be corrupted)")
			return data[2:]
		}
		if data[0] == 0xFF && data[1] == 0xFE {
			*warnings = append(*warnings, "removed UTF-16 LE BOM (data may be corrupted)")
			return data[2:]
		}
	}
	return data
}

func removeTrailingCommas(data []byte, warnings *[]string) []byte {
	result := make([]byte, 0, len(data))
	modified := false
	for i := 0; i < len(data); i++ {
		if data[i] == ',' && i+1 < len(data) {
			next := data[i+1]
			if next == '}' || next == ']' || next == ' ' || next == '\t' || next == '\n' || next == '\r' {
				j := i + 1
				for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
					j++
				}
				if j < len(data) && (data[j] == '}' || data[j] == ']') {
					modified = true
					continue
				}
			}
		}
		result = append(result, data[i])
	}
	if modified {
		*warnings = append(*warnings, "removed trailing commas before } or ]")
	}
	return result
}

func checkJSONBalanced(data []byte) error {
	var depthObj, depthArr int
	inString := false
	escaped := false
	for _, b := range data {
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' && inString {
			escaped = true
			continue
		}
		if b == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch b {
		case '{':
			depthObj++
		case '}':
			depthObj--
		case '[':
			depthArr++
		case ']':
			depthArr--
		}
	}
	if depthObj != 0 {
		return fmt.Errorf("unbalanced braces: %d unclosed", depthObj)
	}
	if depthArr != 0 {
		return fmt.Errorf("unbalanced brackets: %d unclosed", depthArr)
	}
	return nil
}

func extractMetadataFromOriginalFile(path string) (*VideoIndex, error) {
	videoLogger.Debug("using ffprobe/mkvtoolnix for metadata extraction",
		slog.String("file", filepath.Base(path)),
	)

	// 1. 使用 ffprobe 获取基础元数据
	// 🆕 2026-06-15：ffmpeg.Probe(ctx, args) 签名调整（原 args... → ctx, args...）
	output, err := ffmpeg.Probe(context.Background(), "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	if err != nil {
		errMsg := err.Error()
		// 🆕 2026-06-15：新增 exit status 8 / Invalid data 识别（真机 cgo 拒绝半截数据）
		if strings.Contains(errMsg, "ENGINE_LOAD_FAILED") || strings.Contains(errMsg, "ENGINE_SYMBOL_MISSING") {
			return nil, fmt.Errorf("video engine unavailable, please reinstall the app: %w", err)
		}
		if strings.Contains(errMsg, "No such file") || strings.Contains(errMsg, "Permission denied") {
			return nil, fmt.Errorf("cannot access file '%s': %w", filepath.Base(path), err)
		}
		if strings.Contains(errMsg, "exit status 8") || strings.Contains(errMsg, "Invalid data") {
			return nil, fmt.Errorf("mock file appears corrupted, please regenerate mock data (ffprobe rejected: %s): %w", filepath.Base(path), err)
		}
		return nil, fmt.Errorf("ffprobe failed on original file: %w", err)
	}

	sanitized, warning, err := sanitizeFFProbeOutput(output)
	if err != nil {
		return nil, fmt.Errorf("ffprobe output sanitization failed: %w", err)
	}
	if warning != "" {
		videoLogger.Warn("ffprobe output sanitized",
			slog.String("warnings", warning),
		)
	}

	var rawMeta types.FFProbeRawMetadata
	if err := json.Unmarshal(sanitized, &rawMeta); err != nil {
		hexLen := 256
		if len(sanitized) < hexLen {
			hexLen = len(sanitized)
		}
		videoLogger.Warn("ffprobe JSON unmarshal failed",
			slog.Any("error", err),
			slog.Int("hex_dump_bytes", hexLen),
			slog.String("hex_dump", hex.EncodeToString(sanitized[:hexLen])),
		)

		sanitizedStr := string(sanitized)
		if strings.Contains(sanitizedStr, `"frames"`) {
			videoLogger.Warn("ffprobe output uses 'frames' format instead of expected 'streams/format', building minimal index from file info",
				slog.String("file", filepath.Base(path)),
			)
			fileInfo, statErr := os.Stat(path)
			fileSize := int64(0)
			if statErr == nil {
				fileSize = fileInfo.Size()
			}
			fileName := filepath.Base(path)
			minimalIndex := &VideoIndex{
				Width:            0,
				Height:           0,
				OriginalFileSize: fileSize,
				OriginalFilename: fileName,
				DurationSeconds:  0,
				Resolution:       "unknown",
				Format:           "unknown",
			}
			videoLogger.Info("built minimal VideoIndex from file info as fallback",
				slog.Int64("size", fileSize),
				slog.String("name", fileName),
			)
			return minimalIndex, nil
		}

		return nil, fmt.Errorf("failed to unmarshal ffprobe data: %w (hex dump first %d bytes: %s)",
			err, hexLen, hex.EncodeToString(sanitized[:hexLen]))
	}

	var width, height int
	for _, s := range rawMeta.Streams {
		if s.CodecType == "video" {
			width = s.Width
			height = s.Height
			break
		}
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", path, err)
	}

	originalMD5, err := utils.FileMD5(path)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate MD5 for original file %s: %w", path, err)
	}

	chapters, err := extractChaptersWithFFprobe(path)
	if err != nil {
		videoLogger.Warn("could not extract chapters with ffprobe",
			slog.String("file", filepath.Base(path)),
			slog.Any("error", err),
		)

		// 如果 ffprobe 失败，并且文件是 MKV，再尝试 mkvextract
		format, _ := ffmpeg.DetectVideoFormat(path)
		if strings.ToLower(format) == "mkv" {
			videoLogger.Debug("attempting mkvextract for chapters",
				slog.String("file", filepath.Base(path)),
			)
			mkvChapters, err := ExtractChaptersWithMKVExtract(path)
			if err != nil {
				videoLogger.Warn("mkvextract also failed, proceeding without chapters",
					slog.String("file", filepath.Base(path)),
					slog.Any("error", err),
				)
				chapters = nil
			} else {
				chapters = mkvChapters
			}
		} else {
			chapters = nil
		}
	}

	// 3. 构建并返回 VideoIndex
	return &VideoIndex{
		Width:            width,
		Height:           height,
		OriginalFileSize: fileInfo.Size(),
		OriginalFileMD5:  originalMD5,
		OriginalFilename: filepath.Base(path),
		DurationSeconds:  parseDuration(rawMeta.Format.Duration), // 原始时长，可能不精确
		Resolution:       fmt.Sprintf("%dx%d", width, height),
		Format:           rawMeta.Format.FormatName, // 原始格式
		MimeType:         "",                        // 将在 Preprocessor 中权威获取
		Chapters:         chapters,                  // 【关键】从原始文件提取的章节
	}, nil
}

// 解析 HH:MM:SS.ms 格式的时长字符串
func parseDuration(d string) float64 {
	if d == "" {
		return 0
	}
	parts := strings.Split(d, ":")
	if len(parts) < 3 {
		// 尝试直接解析为秒数（某些情况下 ffprobe 可能会这样输出）
		if s, err := strconv.ParseFloat(d, 64); err == nil {
			return s
		}
		return 0
	}

	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	s, _ := strconv.ParseFloat(parts[2], 64)

	return h*3600 + m*60 + s
}

// 使用 ffprobe 提取章节
func extractChaptersWithFFprobe(path string) ([]MKVChapterInfo, error) {
	output, err := ffmpeg.Probe(context.Background(), "-v", "error", "-show_chapters", "-of", "json", path)
	if err != nil {
		return nil, fmt.Errorf("ffprobe command failed: %w", err)
	}

	var probeData struct {
		Chapters []struct {
			ID        int    `json:"id"`
			Title     string `json:"tags.title"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
		} `json:"chapters"`
	}

	if err := json.Unmarshal(output, &probeData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chapter data: %w", err)
	}

	// 如果 ffprobe 返回的章节列表为空，则返回 nil 表示没有章节
	if len(probeData.Chapters) == 0 {
		return nil, nil
	}

	var chapters []MKVChapterInfo
	for _, ch := range probeData.Chapters {
		start, _ := time.ParseDuration(ch.StartTime + "s")
		end, _ := time.ParseDuration(ch.EndTime + "s")
		chapters = append(chapters, MKVChapterInfo{
			ID:        ch.ID,
			Title:     ch.Title,
			StartTime: start,
			EndTime:   end,
		})
	}

	videoLogger.Debug("chapters extracted",
		slog.String("file", filepath.Base(path)),
		slog.Int("count", len(chapters)),
	)
	return chapters, nil
}
