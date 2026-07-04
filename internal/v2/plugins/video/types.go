package video

import (
	"time"

	"github.com/Soltus/encv-go/internal/v2/types"
)

const IndexKindVideo types.IndexKind = "video"

// VideoIndex 是加密视频的元数据索引文件
type VideoIndex struct {
	ID               string  `json:"id"`
	Format           string  `json:"format"`
	MimeType         string  `json:"mime_type"`
	DurationSeconds  float64 `json:"duration_seconds"`
	Resolution       string  `json:"resolution"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	OriginalFileSize int64   `json:"original_file_size"`
	// 原始视频文件的完整路径，用于 Packer 查找关联文件（如字幕）
	OriginalInputPath string                 `json:"originalInputPath"`
	OriginalFilename  string                 `json:"original_filename"`
	OriginalFileMD5   string                 `json:"original_file_md5"`
	EncryptedFileMD5  string                 `json:"encrypted_file_md5"`
	SubtitleTracks    []SubtitleTracks       `json:"subtitle_tracks,omitempty"`
	Chapters          []MKVChapterInfo       `json:"chapters,omitempty"`
	ChaptersV4        []types.ChapterInfo_v4 `json:"chapters_v4,omitempty"`
	KeyFrameOffsets   []uint64               `json:"key_frame_offsets,omitempty"` // 存储所有关键帧的字节偏移量
}

func (v *VideoIndex) GetOriginalFilename() string { return v.OriginalFilename }
func (v *VideoIndex) GetOriginalFileSize() int64  { return v.OriginalFileSize }
func (v *VideoIndex) GetOriginalFileMD5() string  { return v.OriginalFileMD5 }
func (v *VideoIndex) GetEncryptedFileMD5() string { return v.EncryptedFileMD5 }
func (v *VideoIndex) GetMimeType() string         { return v.MimeType }

// SubtitleTrack 表示一个字幕或弹幕轨道
type SubtitleTracks struct {
	Language string `json:"language"`
	FileSize string `json:"file_size"`
	// Filename 是需要恢复成的原始文件名 (e.g., "myvideo.ass")
	Filename string `json:"filename"`
	// Title 是加密后处理的字幕文件名，字幕本身不加密 (e.g., "myvideo.4pm.ass")
	Title string `json:"title"`
	Note  string `json:"note,omitempty"`
}

// MKVChapterInfo 是专门为 MKV 视频定义的章节信息结构
type ChapterInfo = types.ChapterInfo_v4

type MKVChapterInfo struct {
	ID        int           `json:"id"`
	Title     string        `json:"title"`
	StartTime time.Duration `json:"start_time"`
	EndTime   time.Duration `json:"end_time"`
}

func (m MKVChapterInfo) ToChapterInfoV4() types.ChapterInfo_v4 {
	return types.ChapterInfo_v4{
		Time:  m.StartTime.Seconds(),
		Title: m.Title,
	}
}

// 视频容器专用的 KVI
type VideoKVI_v2 struct {
	types.KVI
	VideoIndex *VideoIndex `json:"video_index"`
}

func (v VideoKVI_v2) GetKind() types.IndexKind {
	return IndexKindVideo
}

// 【关键新增】实现 KVIProvider 接口
func (v VideoKVI_v2) GetEncryptionInfo() types.KVI {
	return v.KVI
}

func (v VideoKVI_v2) GetIndex() types.Index {
	return v.VideoIndex
}
