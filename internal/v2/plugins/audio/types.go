package audio

import (
	"github.com/Soltus/encv-go/internal/v2/types"
)

const IndexKindAudio types.IndexKind = "audio"

type AudioIndex struct {
	ID                string `json:"id"`
	OriginalFileSize  int64  `json:"original_file_size"`
	MimeType          string `json:"mime_type"`
	Format            string `json:"format"` // e.g., "plain", "markdown"
	OriginalFilename  string `json:"original_filename"`
	OriginalInputPath string `json:"originalInputPath"`
	OriginalFileMD5   string `json:"original_file_md5"`
	EncryptedFileMD5  string `json:"encrypted_file_md5"`
}

// 【新增】实现 Index 接口
func (t *AudioIndex) GetOriginalFilename() string { return t.OriginalFilename }
func (t *AudioIndex) GetOriginalFileSize() int64  { return t.OriginalFileSize }
func (v *AudioIndex) GetOriginalFileMD5() string  { return v.OriginalFileMD5 }
func (v *AudioIndex) GetEncryptedFileMD5() string { return v.EncryptedFileMD5 }
func (t *AudioIndex) GetMimeType() string         { return t.MimeType }

// 视频容器专用的 KVI
type AudioKVI_v2 struct {
	types.KVI
	AudioIndex *AudioIndex `json:"audio_index"`
}

func (v AudioKVI_v2) GetKind() types.IndexKind {
	return IndexKindAudio
}

// 【关键新增】实现 KVIProvider 接口
func (v AudioKVI_v2) GetEncryptionInfo() types.KVI {
	return v.KVI
}

func (v AudioKVI_v2) GetIndex() types.Index {
	return v.AudioIndex
}
