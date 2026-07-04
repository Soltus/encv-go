package text

import (
	"github.com/Soltus/encv-go/internal/v2/types"
)

const IndexKindText types.IndexKind = "text"

// TextIndex 是解密文本所需的所有元数据的容器
type TextIndex struct {
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
func (t *TextIndex) GetOriginalFilename() string { return t.OriginalFilename }
func (t *TextIndex) GetOriginalFileSize() int64  { return t.OriginalFileSize }
func (v *TextIndex) GetOriginalFileMD5() string  { return v.OriginalFileMD5 }
func (v *TextIndex) GetEncryptedFileMD5() string { return v.EncryptedFileMD5 }
func (t *TextIndex) GetMimeType() string         { return t.MimeType }

// 视频容器专用的 KVI
type TextKVI_v2 struct {
	types.KVI
	TextIndex *TextIndex `json:"text_index"`
}

func (v TextKVI_v2) GetKind() types.IndexKind {
	return IndexKindText
}

// 【关键新增】实现 KVIProvider 接口
func (v TextKVI_v2) GetEncryptionInfo() types.KVI {
	return v.KVI
}

func (v TextKVI_v2) GetIndex() types.Index {
	return v.TextIndex
}
