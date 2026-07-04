// internal/tools/get_metadata.go
//
// 元数据查询工具 — 返回单文件的完整元信息（基础 + 视频/音频探测）。
//
// 关键能力：
//   - 基础字段：path / size / mtime / mode / mime / extension 等
//   - 视频字段：通过 ffprobe 拿 duration / width / height / codec / bitrate
//   - 音频字段：通过 ffprobe 拿 duration / bitrate / sample_rate
//   - ffprobe 容错：缺失/超时都跳过媒体字段，**不**让整个工具失败
//
// 参考：
//   - Spec: /workspace/.trae/specs/agent-tools-scenarios-v2/spec.md
//   - Requirement: get_metadata 工具
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── 参数 / 结果 ───────────────────────────────────────────────

// GetMetadataArgs 工具参数。
type GetMetadataArgs struct {
	MountID    string `json:"mount_id"`
	RelPath    string `json:"rel_path"`
	IncludeHash bool  `json:"include_hash,omitempty"`
}

// GetMetadataResult 工具结果。
type GetMetadataResult struct {
	// 基础字段
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	MTime      string `json:"mtime"`
	Mode       string `json:"mode"`
	Mime       string `json:"mime"`
	Extension  string `json:"extension"`
	IsHidden   bool   `json:"is_hidden"`
	IsSymlink  bool   `json:"is_symlink"`
	IsDir      bool   `json:"is_dir"`

	// 视频/音频字段（可选）
	Video *VideoMeta `json:"video,omitempty"`
	Audio *AudioMeta `json:"audio,omitempty"`

	// 按需
	SHA256 string `json:"sha256,omitempty"`
}

// VideoMeta 视频文件元数据（ffprobe 探测）。
type VideoMeta struct {
	Duration  float64 `json:"duration"`  // 秒
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Codec     string  `json:"codec"`
	Bitrate   int64   `json:"bitrate"`
	FrameRate float64 `json:"frame_rate"`
	HasAudio  bool    `json:"has_audio"`
}

// AudioMeta 音频文件元数据。
type AudioMeta struct {
	Duration    float64 `json:"duration"`
	Bitrate     int64   `json:"bitrate"`
	SampleRate  int     `json:"sample_rate"`
	Channels    int     `json:"channels"`
	Codec       string  `json:"codec"`
	HasCoverArt bool    `json:"has_cover_art"`
}

// ─── ToolDef ────────────────────────────────────────────────────

// GetMetadataDef 返回 get_metadata 的 ToolDef。
func GetMetadataDef() *ToolDef {
	return &ToolDef{
		Name:        "get_metadata",
		Description: "查询单文件元信息（基础字段 + 视频/音频探测）。视频/音频字段通过 ffprobe 拿，ffprobe 缺失或超时会自动跳过。",
		Kind:        KindMetadata,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["mount_id","rel_path"],
			"properties":{
				"mount_id":{"type":"string"},
				"rel_path":{"type":"string"},
				"include_hash":{"type":"boolean","default":false,"description":"按需计算 SHA-256"}
			}
		}`,
		Handler: getMetadataHandler,
	}
}

// ─── Handler ────────────────────────────────────────────────────

func getMetadataHandler(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
	var args GetMetadataArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errResult(fmt.Sprintf("invalid args: %v", err)), nil
	}
	if args.MountID == "" || args.RelPath == "" {
		return errResult("mount_id and rel_path are required"), nil
	}
	if deps == nil || deps.ResolveMount == nil {
		return errResult("deps not initialized"), nil
	}
	rootAbs, ok := deps.ResolveMount(args.MountID)
	if !ok {
		return errResult(fmt.Sprintf("mount not found: %s", args.MountID)), nil
	}
	absPath, err := safeJoin(rootAbs, args.RelPath)
	if err != nil {
		return errResult(err.Error()), nil
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		return errResult(fmt.Sprintf("lstat: %v", err)), nil
	}

	res := GetMetadataResult{
		Path:      "/" + strings.TrimPrefix(strings.ReplaceAll(strings.TrimPrefix(absPath, rootAbs), string(os.PathSeparator), "/"), "/"),
		Size:      info.Size(),
		MTime:     info.ModTime().UTC().Format(time.RFC3339),
		Mode:      info.Mode().String(),
		Mime:      mimeOf(absPath),
		Extension: strings.TrimPrefix(strings.ToLower(filepath.Ext(info.Name())), "."),
		IsHidden:  strings.HasPrefix(info.Name(), "."),
		IsSymlink: info.Mode()&os.ModeSymlink != 0,
		IsDir:     info.IsDir(),
	}

	// 视频/音频探测（ffprobe 容错）
	if !info.IsDir() {
		mime := res.Mime
		if strings.HasPrefix(mime, "video/") {
			if v, err := probeVideo(ctx, absPath); err == nil {
				res.Video = v
			}
		} else if strings.HasPrefix(mime, "audio/") {
			if a, err := probeAudio(ctx, absPath); err == nil {
				res.Audio = a
			}
		}
	}

	// SHA-256 按需
	if args.IncludeHash && !info.IsDir() {
		res.SHA256, _ = sha256File(absPath)
	}

	b, _ := json.Marshal(res)
	return ToolResult{Result: string(b), Status: "success"}, nil
}

// mimeOf 简单 mime 嗅探（按扩展名）。
func mimeOf(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".srt":
		return "application/x-subrip"
	case ".vtt":
		return "text/vtt"
	case ".txt", ".md", ".log":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}
