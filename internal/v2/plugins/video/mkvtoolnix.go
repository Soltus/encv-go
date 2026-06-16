package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/utils/ffmpeg"
)

// TrackInfo 用于解析 mkvmerge -J 的输出
type TrackInfo struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	// 可以添加更多字段，如 codec_id
}

type MkvmergeInfo struct {
	Tracks []TrackInfo `json:"tracks"`
}

// getVideoTrackID 使用 mkvmerge -J 获取第一个视频轨道的 ID
func getVideoTrackID(filePath string) (string, error) {
	cmd := exec.Command("mkvmerge", "-J", filePath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mkvmerge -J failed: %w", err)
	}

	var info MkvmergeInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return "", fmt.Errorf("failed to parse mkvmerge JSON output: %w", err)
	}

	for _, track := range info.Tracks {
		if track.Type == "video" {
			slog.Info("Found video track", "component", "DIAG", "track_id", track.ID)
			return fmt.Sprintf("%d", track.ID), nil
		}
	}

	return "", fmt.Errorf("no video track found in the file")
}

func getVideoTrackIDWithFFProbe(filePath string) (string, error) {
	output, err := ffmpeg.Probe(context.Background(), "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=index", "-of", "csv=p=0", filePath)
	if err != nil {
		return "", fmt.Errorf("ffprobe failed to get video track ID: %w", err)
	}
	idx := strings.TrimSpace(string(output))
	if idx == "" {
		return "", fmt.Errorf("no video track found in the file")
	}
	return idx, nil
}

// IsMkvPartOfSplit 检查 MKV 文件是否是一个分片集的一部分
func IsMkvPartOfSplit(filePath string) (bool, error) {
	cmd := exec.Command("mkvinfo", "-p", filePath)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("mkvinfo failed: %w", err)
	}

	outputStr := string(output)
	// 【关键修复】使用正确的本地化关键词 "剪辑 UID"
	hasPrevUID := strings.Contains(outputStr, "| + 上一剪辑 UID")
	hasNextUID := strings.Contains(outputStr, "| + 下一剪辑 UID")

	// 英文环境下的检查
	hasPrevUID = hasPrevUID || strings.Contains(outputStr, "| + Previous segment UID")
	hasNextUID = hasNextUID || strings.Contains(outputStr, "| + Next segment UID")

	if hasPrevUID || hasNextUID {
		slog.Info("File is a split part", "component", "DIAG", "has_prev_uid", hasPrevUID, "has_next_uid", hasNextUID)
		return true, nil
	}

	fmt.Println("-> [DIAG] File is not a split part.")
	return false, nil
}

// MKVMergeIdentity 是 mkvmerge --identify -J 的 JSON 输出结构
type MKVMergeIdentity struct {
	Container struct {
		Type       string `json:"type"`
		Duration   string `json:"duration"`
		FileSize   string `json:"file_size"`
		TrackCount int    `json:"track_count"`
	} `json:"container"`
	Tracks []struct {
		ID       int    `json:"id"`
		Type     string `json:"type"`
		CodecID  string `json:"codec_id"`
		Language string `json:"properties.language"`
		UID      int64  `json:"properties.uid"`
		// 视频特有
		Width  int `json:"properties.video_width"`
		Height int `json:"properties.video_height"`
		// 音频特有
		Channels     int `json:"properties.audio_channels"`
		SamplingFreq int `json:"properties.audio_sampling_frequency"`
	} `json:"tracks"`
}

// MKVChapterXML 是 mkvextract chapters 输出的 XML 结构
type MKVChapterXML struct {
	XMLName xml.Name `xml:"Chapters"`
	Edition []struct {
		EditionFlagHidden  bool `xml:"EditionFlagHidden"`
		EditionFlagDefault bool `xml:"EditionFlagDefault"`
		ChapterAtoms       []struct {
			ChapterUID       int64 `xml:"ChapterUID"`
			ChapterTimeStart int64 `xml:"ChapterTimeStart"`
			ChapterTimeEnd   int64 `xml:"ChapterTimeEnd"`
			ChapterDisplay   []struct {
				ChapterString   string `xml:"ChapterString"`
				ChapterLanguage string `xml:"ChapterLanguage"`
			} `xml:"ChapterDisplay"`
		} `xml:"ChapterAtom"`
	} `xml:"EditionEntry"`
}

// IdentifyWithMKVMerge 使用 mkvmerge 获取文件的详细信息
// DEAD CODE: no callers found in project. Kept for potential future use.
// NOTE: has no IsMobile() guard — if re-activated, add mobile fallback (e.g. ffprobe-based identification).
func IdentifyWithMKVMerge(filePath string) (*MKVMergeIdentity, error) {
	cmd := exec.Command("mkvmerge", "-J", filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mkvmerge identify failed: %w", err)
	}

	var identity MKVMergeIdentity
	if err := json.Unmarshal(output, &identity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mkvmerge JSON: %w", err)
	}

	return &identity, nil
}

// ExtractChaptersWithMKVExtract 使用 mkvextract 提取章节到临时 XML 文件
func ExtractChaptersWithMKVExtract(filePath string) ([]MKVChapterInfo, error) {
	// 创建临时文件来存储章节 XML
	tempFile, err := os.CreateTemp("", "encv-chapters-*.xml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file for chapters: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath) // 确保函数退出时删除临时文件

	// 调用 mkvextract
	cmd := exec.Command("mkvextract", filePath, "chapters", tempPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mkvextract chapters failed: %w", err)
	}

	// 解析 XML
	xmlData, err := os.ReadFile(tempPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp chapters file: %w", err)
	}

	var chapterXML MKVChapterXML
	if err := xml.Unmarshal(xmlData, &chapterXML); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chapter XML: %w", err)
	}

	if len(chapterXML.Edition) == 0 || len(chapterXML.Edition[0].ChapterAtoms) == 0 {
		return nil, nil // 没有章节
	}

	var chapters []MKVChapterInfo
	for _, atom := range chapterXML.Edition[0].ChapterAtoms {
		startTime := time.Duration(atom.ChapterTimeStart) * time.Nanosecond
		endTime := time.Duration(atom.ChapterTimeEnd) * time.Nanosecond
		title := "Untitled Chapter"
		if len(atom.ChapterDisplay) > 0 {
			title = atom.ChapterDisplay[0].ChapterString
		}
		chapters = append(chapters, MKVChapterInfo{
			ID:        len(chapters) + 1, // 使用 1-based ID
			Title:     title,
			StartTime: startTime,
			EndTime:   endTime,
		})
	}

	return chapters, nil
}

// checkFileForCues 使用 mkvinfo 快速检查文件是否包含 Cues 元素
func checkFileForCues(filePath string) (bool, error) {
	cmd := exec.Command("mkvinfo", "-v", filePath)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(output), "|+ Cues"), nil
}

func extractKeyFrameOffsetsFromMKV(filePath string) ([]uint64, error) {
	// 在解析 Cues 之前，先检查文件是否有 Cues
	cmdCheck := exec.Command("mkvinfo", "-v", filePath)
	outputCheck, err := cmdCheck.Output()
	if err == nil {
		if !strings.Contains(string(outputCheck), "|+ Cues") {
			fmt.Println("-> [DIAG] WARNING: File does not seem to contain a Cues element. mkvextract will likely fail.")
		} else {
			fmt.Println("-> [DIAG] File seems to contain a Cues element.")
		}
	} else {
		slog.Warn("Could not run mkvinfo to check for Cues", "component", "DIAG", "error", err)
	}

	// 方法1：尝试使用Cues元素（最可靠）
	fmt.Println("-> [DIAG] Method 1: Trying mkvextract for Cues...")
	offsets, err := extractKeyFrameOffsetsFromMKVCues(filePath)
	if err == nil && len(offsets) > 0 {
		slog.Info("Got keyframes from Cues", "component", "DIAG", "count", len(offsets))
		return offsets, nil
	}
	slog.Error("Cues extraction failed", "component", "DIAG", "error", err)

	// 方法2：使用mkvinfo解析
	fmt.Println("-> [DIAG] Method 2: Trying mkvinfo parsing...")
	cmd := exec.Command("mkvinfo", "-v", filePath)
	output, err := cmd.Output()
	if err != nil {
		slog.Error("mkvinfo command failed", "component", "DIAG", "error", err)
		slog.Info("Command was", "component", "DIAG", "command", cmd.String())
	} else {
		slog.Info("mkvinfo raw output (first 500 chars)", "component", "DIAG", "output", string(output)[:min(500, len(output))])
		segmentPos, err := getSegmentPosition(filePath)
		if err != nil {
			slog.Error("Could not get segment position", "component", "DIAG", "error", err)
		}
		offsets, err := parseMKVInfoForKeyFrames(output, segmentPos)
		if err == nil && len(offsets) > 0 {
			slog.Info("Got keyframes from mkvinfo", "component", "DIAG", "count", len(offsets))
			return offsets, nil
		}
		slog.Error("mkvinfo parsing failed", "component", "DIAG", "error", err)
	}

	return nil, nil
}

func parseMKVInfoForKeyFrames(output []byte, segmentPos uint64) ([]uint64, error) {
	var offsets []uint64
	lines := strings.Split(string(output), "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// 方法1：查找包含 "keyframe" 和 "position:" 的行
		if strings.Contains(line, "keyframe") && strings.Contains(line, "position:") {
			if offset := extractPositionFromLine(line); offset > 0 {
				// 计算全局偏移量
				globalOffset := segmentPos + offset
				offsets = append(offsets, globalOffset)
				continue
			}
		}

		// 方法2：如果方法1失败，尝试解析文件偏移量
		// mkvinfo可能在不同行显示偏移量
		if strings.Contains(line, "keyframe") {
			// 查找下一行是否包含偏移量信息
			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if strings.Contains(nextLine, "position:") {
					if offset := extractPositionFromLine(nextLine); offset > 0 {
						offsets = append(offsets, offset)
					}
				}
			}
		}
	}

	if len(offsets) == 0 {
		return nil, fmt.Errorf("no keyframes found in mkvinfo output")
	}

	return offsets, nil
}

func extractPositionFromLine(line string) uint64 {
	parts := strings.Split(line, "position:")
	if len(parts) < 2 {
		return 0
	}

	posStr := strings.TrimSpace(parts[1])
	// 移除可能的右括号和其他字符
	posStr = strings.TrimSuffix(posStr, ")")
	posStr = strings.Fields(posStr)[0] // 取第一个字段

	if offset, err := strconv.ParseUint(posStr, 10, 64); err == nil {
		return offset
	}

	return 0
}

func extractKeyFrameOffsetsFromMKVCues(filePath string) ([]uint64, error) {
	slog.Info("Attempting to extract keyframes from MKV", "component", "DIAG", "file", filePath)

	videoTrackID, err := getVideoTrackID(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get video track ID for cues extraction: %w", err)
	}
	slog.Info("Found video track ID for cues extraction", "component", "DIAG", "track_id", videoTrackID)

	// 步骤 2: 获取 Segment 元素的全局文件偏移量
	segmentPos, err := getSegmentPosition(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get segment position: %w", err)
	}

	// 【关键修复】步骤 3: 使用正确的、完整的语法提取 Cues
	// 命令格式: mkvextract <file> cues <TID>:-
	cmd := exec.Command("mkvextract", filePath, "cues", fmt.Sprintf("%s:-", videoTrackID))
	slog.Info("Executing corrected command", "component", "DIAG", "command", cmd.String())

	output, err := cmd.Output()
	if err != nil {
		// 如果命令失败，现在我们可以提供明确的错误信息
		return nil, fmt.Errorf("mkvextract cues command failed (track ID: %s): %w", videoTrackID, err)
	}

	var offsets []uint64
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "CueClusterPosition:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				if clusterPos, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64); err == nil {
					// 计算全局偏移量
					globalOffset := segmentPos + clusterPos
					offsets = append(offsets, globalOffset)
				}
			}
		}
	}

	if len(offsets) == 0 {
		return nil, fmt.Errorf("no cues found in mkvextract output for track ID %s", videoTrackID)
	}

	slog.Info("Got keyframes from Cues", "component", "DIAG", "count", len(offsets), "track_id", videoTrackID)
	return offsets, nil
}

// getSegmentPosition 通过解析文件头找到 Segment 元素的起始位置
func getSegmentPosition(filePath string) (uint64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	// EBML ID for Segment is 0x18538067
	segmentID := []byte{0x18, 0x53, 0x80, 0x67}

	// 创建一个缓冲区来读取文件
	buf := make([]byte, 4096) // 4KB 应该足够找到 Segment ID
	bytesRead, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return 0, err
	}
	if bytesRead < len(segmentID) {
		return 0, fmt.Errorf("file too small to contain a segment ID")
	}

	// 在缓冲区中搜索 Segment ID
	for i := 0; i <= bytesRead-len(segmentID); i++ {
		if bytes.Equal(buf[i:i+len(segmentID)], segmentID) {
			// 找到了！Segment ID 本身的长度是 4 字节
			// 它的起始位置就是我们需要的
			return uint64(i), nil
		}
	}

	return 0, fmt.Errorf("segment ID (0x18538067) not found in the first %d bytes of the file", bytesRead)
}

// MkvInfo 存储 mkvinfo -p 的关键信息
type MkvInfo struct {
	Path        string
	SegmentUID  int64
	PrevUID     int64
	NextUID     int64
	IsSplitPart bool
}

// batchGetMkvInfos 批量获取 MKV 文件信息
func batchGetMkvInfos(paths []string) (map[string]*MkvInfo, error) {
	infos := make(map[string]*MkvInfo)
	for _, path := range paths {
		info, err := getMkvInfo(path)
		if err != nil {
			slog.Warn("Could not get MKV info", "component", "VIDEO_PLUGIN", "path", path, "error", err)
			continue
		}
		infos[path] = info
	}
	return infos, nil
}

// getMkvInfo 解析单个 MKV 文件的 mkvinfo -p 输出
func getMkvInfo(path string) (*MkvInfo, error) {
	cmd := exec.Command("mkvinfo", "-p", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mkvinfo failed on %s: %w", path, err)
	}

	info := &MkvInfo{Path: path}
	outputStr := string(output)

	// 使用正则表达式提取 UID（注意是十六进制）
	// | + 剪辑 UID: 0x80a65bbe06bad953bde8af1918689d95
	reUID := regexp.MustCompile(`\|\s*\+\s*剪辑 UID:\s*(0x[0-9a-fA-F]+)`)
	if match := reUID.FindStringSubmatch(outputStr); match != nil {
		info.SegmentUID, _ = strconv.ParseInt(match[1], 0, 64) // ParseInt 自动处理 0x 前缀
	}

	rePrevUID := regexp.MustCompile(`\|\s*\+\s*上一剪辑 UID:\s*(0x[0-9a-fA-F]+)`)
	if match := rePrevUID.FindStringSubmatch(outputStr); match != nil {
		info.PrevUID, _ = strconv.ParseInt(match[1], 0, 64)
	}

	reNextUID := regexp.MustCompile(`\|\s*\+\s*下一剪辑 UID:\s*(0x[0-9a-fA-F]+)`)
	if match := reNextUID.FindStringSubmatch(outputStr); match != nil {
		info.NextUID, _ = strconv.ParseInt(match[1], 0, 64)
	}

	info.IsSplitPart = (info.PrevUID != 0) || (info.NextUID != 0)

	return info, nil
}

// groupSplitParts 根据 PrevUID/NextUID 链接关系将分片分组
func groupSplitParts(infos map[string]*MkvInfo) [][]string {
	var allSets [][]string
	processedUIDs := make(map[int64]bool)

	for _, info := range infos {
		// 如果不是分片，或者这个分片已经被处理过了，就跳过
		if !info.IsSplitPart || processedUIDs[info.SegmentUID] {
			continue
		}

		// 从当前分片开始，找到整个有序的链条
		sortedPaths, chainUIDs := findAndSortChain(info, infos)
		if len(sortedPaths) > 0 {
			allSets = append(allSets, sortedPaths)
		}

		// 标记这个链条中的所有 UID 为已处理，防止重复分组
		for _, uid := range chainUIDs {
			processedUIDs[uid] = true
		}
	}

	return allSets
}

// calculateSplitSetHash 计算分片集的哈希值，用于缓存标识
// 【修复】只使用文件路径和大小，不使用修改时间，避免文件时间变化导致缓存失效
func calculateSplitSetHash(sortedPaths []string) string {
	h := sha256.New()
	for _, path := range sortedPaths {
		// 只包含文件路径和大小（不包含修改时间）
		info, err := os.Stat(path)
		if err == nil {
			h.Write([]byte(path))
			h.Write([]byte(fmt.Sprintf("%d", info.Size())))
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16] // 使用前16个字符作为标识
}

// getCachedMergedPath 检查缓存中是否存在有效的合并文件
func getCachedMergedPath(sortedPaths []string, cacheDir string) (string, bool) {
	if cacheDir == "" {
		slog.Info("getCachedMergedPath: cacheDir is empty", "component", "VIDEO_PLUGIN")
		return "", false
	}

	hash := calculateSplitSetHash(sortedPaths)
	cachePath := filepath.Join(cacheDir, "encv-merged-"+hash+".mkv")
	slog.Info("getCachedMergedPath", "component", "VIDEO_PLUGIN", "hash", hash, "cache_path", cachePath)

	// 检查缓存文件是否存在且有效
	info, err := os.Stat(cachePath)
	if err != nil {
		slog.Info("getCachedMergedPath: cache file not found", "component", "VIDEO_PLUGIN", "error", err)
		return "", false
	}
	if info.Size() == 0 {
		slog.Info("getCachedMergedPath: cache file is empty", "component", "VIDEO_PLUGIN")
		return "", false
	}
	slog.Info("getCachedMergedPath: cache file exists", "component", "VIDEO_PLUGIN", "size", info.Size())

	// 验证缓存文件的完整性
	verifyCmd := exec.Command("mkvinfo", "-P", cachePath)
	if err := verifyCmd.Run(); err != nil {
		slog.Warn("Cached file verification failed, will re-merge", "component", "VIDEO_PLUGIN", "error", err)
		os.Remove(cachePath)
		return "", false
	}

	slog.Info("Using cached merged file", "component", "VIDEO_PLUGIN", "cache_path", cachePath)
	return cachePath, true
}

// saveToCache 将合并后的文件保存到缓存目录
func saveToCache(sourcePath string, sortedPaths []string, cacheDir string) (string, error) {
	if cacheDir == "" {
		slog.Info("saveToCache: cacheDir is empty, returning source path", "component", "VIDEO_PLUGIN")
		return sourcePath, nil // 没有缓存目录，直接使用原文件
	}

	slog.Info("saveToCache: attempting to save to cacheDir", "component", "VIDEO_PLUGIN", "cache_dir", cacheDir)

	// 确保缓存目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		slog.Error("saveToCache: failed to create cache directory", "component", "VIDEO_PLUGIN", "error", err)
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}
	slog.Info("saveToCache: cache directory ensured", "component", "VIDEO_PLUGIN")

	hash := calculateSplitSetHash(sortedPaths)
	cachePath := filepath.Join(cacheDir, "encv-merged-"+hash+".mkv")

	// 复制文件到缓存
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	cacheFile, err := os.Create(cachePath)
	if err != nil {
		return "", fmt.Errorf("failed to create cache file: %w", err)
	}
	defer cacheFile.Close()

	if _, err := io.Copy(cacheFile, sourceFile); err != nil {
		os.Remove(cachePath)
		return "", fmt.Errorf("failed to copy to cache: %w", err)
	}

	slog.Info("Saved merged file to cache", "component", "VIDEO_PLUGIN", "cache_path", cachePath)
	return cachePath, nil
}

// mergeSplitPartsFromSet 合并一个已知的、已排序的分片集合
// 【新增】支持缓存目录，如果提供了 cacheDir，会直接在其中创建合并文件
func mergeSplitPartsFromSet(sortedPaths []string, outputDir string, cacheDir string) (string, error) {
	if len(sortedPaths) == 0 {
		return "", fmt.Errorf("no paths provided for merging")
	}

	slog.Info("mergeSplitPartsFromSet", "component", "VIDEO_PLUGIN", "cache_dir", cacheDir, "output_dir", outputDir, "paths", sortedPaths)

	// 【新增】首先检查缓存
	if cachedPath, found := getCachedMergedPath(sortedPaths, cacheDir); found {
		slog.Info("Cache hit, using cached file", "component", "VIDEO_PLUGIN", "cached_path", cachedPath)
		return cachedPath, nil
	}
	slog.Info("Cache miss, will merge files", "component", "VIDEO_PLUGIN")

	// 【修改】确定合并文件的创建位置
	// 如果配置了缓存目录且与输出目录不同，直接在缓存目录中创建
	mergeDir := outputDir
	if cacheDir != "" && cacheDir != outputDir {
		// 确保缓存目录存在
		if err := os.MkdirAll(cacheDir, 0755); err == nil {
			mergeDir = cacheDir
			slog.Info("Using cacheDir for merged file", "component", "VIDEO_PLUGIN", "merge_dir", mergeDir)
		} else {
			slog.Warn("Failed to create cacheDir, falling back to outputDir", "component", "VIDEO_PLUGIN", "error", err)
		}
	}

	// 【修改】使用哈希值作为文件名，确保缓存一致性
	hash := calculateSplitSetHash(sortedPaths)
	tempPath := filepath.Join(mergeDir, "encv-merged-"+hash+".mkv")

	// 检查文件是否已存在（可能是之前的缓存）
	if info, err := os.Stat(tempPath); err == nil && info.Size() > 0 {
		// 文件已存在，验证完整性
		verifyCmd := exec.Command("mkvinfo", "-P", tempPath)
		if verifyErr := verifyCmd.Run(); verifyErr == nil {
			slog.Info("Merged file already exists and valid", "component", "VIDEO_PLUGIN", "path", tempPath)
			return tempPath, nil
		}
		// 文件存在但无效，删除后重新创建
		os.Remove(tempPath)
	}

	// 创建临时文件
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for merging: %w", err)
	}
	tempFile.Close()

	// 【关键修复】正确构建 mkvmerge 命令，使用 '+' 进行追加
	args := []string{"-o", tempPath}
	for i, path := range sortedPaths {
		if i > 0 {
			args = append(args, "+") // 在第一个文件之后的所有文件前加上 '+'
		}
		args = append(args, path)
	}

	cmd := exec.Command("mkvmerge", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	slog.Info("Executing corrected merge command", "component", "VIDEO_PLUGIN", "command", cmd.String())

	if err := cmd.Run(); err != nil {
		os.Remove(tempPath) // 清理可能损坏的文件
		return "", fmt.Errorf("mkvmerge failed: %w", err)
	}

	// 【关键新增】生成后进行严格的完整性检查
	fmt.Println("-> [VIDEO_PLUGIN] Merge command succeeded. Verifying output file integrity with mkvinfo...")
	verifyCmd := exec.Command("mkvinfo", "-P", tempPath)
	if verifyErr := verifyCmd.Run(); verifyErr != nil {
		os.Remove(tempPath) // 清理无效文件
		return "", fmt.Errorf("mkvmerge succeeded but output file verification failed: %w", verifyErr)
	}

	slog.Info("Merged and verified file", "component", "VIDEO_PLUGIN", "path", tempPath)

	// 如果直接在缓存目录中创建，不需要额外复制
	if mergeDir == cacheDir {
		slog.Info("Merged file already in cacheDir", "component", "VIDEO_PLUGIN", "path", tempPath)
		return tempPath, nil
	}

	// 【新增】保存到缓存（如果配置了缓存目录且文件在 outputDir）
	if cacheDir != "" && cacheDir != outputDir {
		slog.Info("Attempting to save merged file to cache", "component", "VIDEO_PLUGIN")
		cachedPath, err := saveToCache(tempPath, sortedPaths, cacheDir)
		if err == nil && cachedPath != tempPath {
			// 成功保存到缓存，删除临时文件，返回缓存路径
			slog.Info("Successfully cached merged file, removing temp file", "component", "VIDEO_PLUGIN")
			os.Remove(tempPath)
			return cachedPath, nil
		}
		if err != nil {
			slog.Warn("Failed to save to cache", "component", "VIDEO_PLUGIN", "error", err)
		}
	}

	return tempPath, nil
}

// findAndSortChain 从任意一个分片开始，找到整个有序的链条
func findAndSortChain(startInfo *MkvInfo, allInfos map[string]*MkvInfo) ([]string, []int64) {
	// 1. 创建 UID 到 Info 的快速查找映射
	uidToInfo := make(map[int64]*MkvInfo)
	for _, info := range allInfos {
		if info.IsSplitPart {
			uidToInfo[info.SegmentUID] = info
		}
	}

	// 2. 回溯找到链表的头（第一个分片）
	headInfo := startInfo
	for headInfo.PrevUID != 0 {
		if prevInfo, exists := uidToInfo[headInfo.PrevUID]; exists {
			headInfo = prevInfo
		} else {
			// 链条断了，我们就在这里停止
			slog.Warn("Split chain is broken", "component", "VIDEO_PLUGIN", "uid", headInfo.SegmentUID)
			break
		}
	}

	// 3. 从头开始，沿着 NextUID 链条遍历，收集所有分片
	var sortedPaths []string
	var chainUIDs []int64
	currentInfo := headInfo
	for currentInfo != nil {
		sortedPaths = append(sortedPaths, currentInfo.Path)
		chainUIDs = append(chainUIDs, currentInfo.SegmentUID)

		if currentInfo.NextUID == 0 {
			break // 到达链尾
		}
		currentInfo = uidToInfo[currentInfo.NextUID]
	}

	return sortedPaths, chainUIDs
}

func getMkvInfoNative(path string) (*MkvInfo, error) {
	info := &MkvInfo{Path: path}
	prevUID, nextUID, err := ReadMKVSegmentInfoNative(path)
	if err != nil {
		return nil, fmt.Errorf("native MKV info extraction failed on %s: %w", path, err)
	}
	if len(prevUID) > 0 {
		h := hex.EncodeToString(prevUID)
		info.PrevUID, _ = strconv.ParseInt("0x"+h, 0, 64)
	}
	if len(nextUID) > 0 {
		h := hex.EncodeToString(nextUID)
		info.NextUID, _ = strconv.ParseInt("0x"+h, 0, 64)
	}
	info.IsSplitPart = (info.PrevUID != 0) || (info.NextUID != 0)
	return info, nil
}

func mergeSplitPartsWithFFmpeg(sortedPaths []string, outputDir string, cacheDir string) (string, error) {
	if len(sortedPaths) == 1 {
		return sortedPaths[0], nil
	}

	mergeDir := outputDir
	if cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0755); err == nil {
			mergeDir = cacheDir
		}
	}

	hash := calculateSplitSetHash(sortedPaths)
	tempPath := filepath.Join(mergeDir, "encv-merged-"+hash+".mkv")

	if info, err := os.Stat(tempPath); err == nil && info.Size() > 0 {
		return tempPath, nil
	}

	concatFile, err := os.CreateTemp(mergeDir, "encv-concat-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create concat list: %w", err)
	}
	concatPath := concatFile.Name()
	defer os.Remove(concatPath)

	for _, p := range sortedPaths {
		fmt.Fprintf(concatFile, "file '%s'\n", p)
	}
	concatFile.Close()

	args := []string{"-y", "-f", "concat", "-safe", "0", "-i", concatPath, "-c", "copy", tempPath}
	// 🆕 2026-06-15：ffmpeg.Run(ctx, args) → ffmpeg.Encode(ctx, args)，返回 *EncodeResult
	if _, err := ffmpeg.Encode(context.Background(), args...); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("ffmpeg concat merge failed: %w", err)
	}

	return tempPath, nil
}
