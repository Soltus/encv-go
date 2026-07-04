// internal/tools/helpers.go
//
// 工具间共享的辅助函数。
package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// safeJoin 把 root + rel 拼成绝对路径，**严格**防止越权（.. / 绝对路径）。
//
//   - rel 为绝对路径 → 报错
//   - rel 含 ".." → 报错
//   - 拼完不在 root 下 → 报错
//
// 通过后返回干净的绝对路径。
func safeJoin(root, rel string) (string, error) {
	cleanRel := filepath.Clean("/" + rel)
	if strings.HasPrefix(cleanRel, "/..") || cleanRel == "/.." {
		return "", fmt.Errorf("rel_path escapes mount root")
	}
	full := filepath.Join(root, cleanRel)
	// 二次校验（防御性）
	cleanFull := filepath.Clean(full)
	cleanRoot := filepath.Clean(root)
	if !strings.HasPrefix(cleanFull, cleanRoot) {
		return "", fmt.Errorf("rel_path escapes mount root")
	}
	return cleanFull, nil
}

// sha256File 读取文件并计算 SHA-256。
// 大文件（>100MB）依然计算，但需时较久；按需调用。
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ffprobeRun 调 ffprobe -show_streams -show_format -of json file。
// 5s 超时；缺失/超时返错（不 panic）。
func ffprobeRun(ctx context.Context, path string) (map[string]any, error) {
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("ffprobe not in PATH: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin,
		"-v", "quiet",
		"-show_streams", "-show_format",
		"-of", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// extractStreams 从 ffprobe JSON 中分离视频/音频流。
func extractStreams(probe map[string]any) (video, audio map[string]any) {
	streams, _ := probe["streams"].([]any)
	for _, s := range streams {
		sm, _ := s.(map[string]any)
		codecType, _ := sm["codec_type"].(string)
		switch codecType {
		case "video":
			if video == nil {
				video = sm
			}
		case "audio":
			if audio == nil {
				audio = sm
			}
		}
	}
	return video, audio
}

// parseFloat 解析 ffprobe 字符串字段为 float64。
func parseFloat(v any) float64 {
	switch x := v.(type) {
	case string:
		var f float64
		fmt.Sscanf(x, "%f", &f)
		return f
	case float64:
		return x
	}
	return 0
}

// parseInt 解析 ffprobe 字符串字段为 int。
func parseInt(v any) int {
	switch x := v.(type) {
	case string:
		var i int
		fmt.Sscanf(x, "%d", &i)
		return i
	case float64:
		return int(x)
	}
	return 0
}

// parseRate 解析 ffprobe 比率字符串（如 "30/1"）为 float64。
func parseRate(v any) float64 {
	s, ok := v.(string)
	if !ok {
		return 0
	}
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return parseFloat(s)
	}
	num := parseFloat(parts[0])
	den := parseFloat(parts[1])
	if den == 0 {
		return 0
	}
	return num / den
}

// probeVideo 从 ffprobe 输出提取视频元数据。
func probeVideo(ctx context.Context, path string) (*VideoMeta, error) {
	probe, err := ffprobeRun(ctx, path)
	if err != nil {
		return nil, err
	}
	video, audio := extractStreams(probe)
	if video == nil {
		return nil, fmt.Errorf("no video stream")
	}
	vm := &VideoMeta{
		Width:     parseInt(video["width"]),
		Height:    parseInt(video["height"]),
		Codec:     asString(video["codec_name"]),
		Bitrate:   int64(parseFloat(asString(video["bit_rate"]))),
		FrameRate: parseRate(video["avg_frame_rate"]),
		HasAudio:  audio != nil,
	}
	// duration 优先从 format 取
	if fmt, ok := probe["format"].(map[string]any); ok {
		vm.Duration = parseFloat(asString(fmt["duration"]))
	}
	if vm.Duration == 0 {
		vm.Duration = parseFloat(asString(video["duration"]))
	}
	return vm, nil
}

// probeAudio 提取音频元数据。
func probeAudio(ctx context.Context, path string) (*AudioMeta, error) {
	probe, err := ffprobeRun(ctx, path)
	if err != nil {
		return nil, err
	}
	_, audio := extractStreams(probe)
	if audio == nil {
		return nil, fmt.Errorf("no audio stream")
	}
	am := &AudioMeta{
		Bitrate:    int64(parseFloat(asString(audio["bit_rate"]))),
		SampleRate: parseInt(audio["sample_rate"]),
		Channels:   parseInt(audio["channels"]),
		Codec:      asString(audio["codec_name"]),
	}
	if fmt, ok := probe["format"].(map[string]any); ok {
		am.Duration = parseFloat(asString(fmt["duration"]))
	}
	// cover art 探测（视频流存在且 disposition.attached_pic=1）
	streams, _ := probe["streams"].([]any)
	for _, s := range streams {
		sm, _ := s.(map[string]any)
		if disp, ok := sm["disposition"].(map[string]any); ok {
			if v, _ := disp["attached_pic"].(float64); v == 1 {
				am.HasCoverArt = true
			}
		}
	}
	return am, nil
}

// asString 任意 → string（nil 返回空）。
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
