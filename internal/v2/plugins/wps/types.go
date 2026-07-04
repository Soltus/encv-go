package wps

import (
	"github.com/Soltus/encv-go/internal/v2/types"
)

const IndexKindWPS types.IndexKind = "WPS"

type WPSIndex struct {
	ID                string `json:"id"`
	OriginalFileSize  int64  `json:"original_file_size"`
	MimeType          string `json:"mime_type"`
	Format            string `json:"format"` // e.g., "plain", "markdown"
	OriginalFilename  string `json:"original_filename"`
	OriginalInputPath string `json:"originalInputPath"`
	OriginalFileMD5   string `json:"original_file_md5"`
	EncryptedFileMD5  string `json:"encrypted_file_md5"`
}

// 实现 Index 接口
func (t *WPSIndex) GetOriginalFilename() string { return t.OriginalFilename }
func (t *WPSIndex) GetOriginalFileSize() int64  { return t.OriginalFileSize }
func (v *WPSIndex) GetOriginalFileMD5() string  { return v.OriginalFileMD5 }
func (v *WPSIndex) GetEncryptedFileMD5() string { return v.EncryptedFileMD5 }
func (t *WPSIndex) GetMimeType() string         { return t.MimeType }

// 视频容器专用的 KVI
type WPSKVI_v2 struct {
	types.KVI
	WPSIndex *WPSIndex `json:"wps_index"`
}

func (v WPSKVI_v2) GetKind() types.IndexKind {
	return IndexKindWPS
}

// 【关键新增】实现 KVIProvider 接口
func (v WPSKVI_v2) GetEncryptionInfo() types.KVI {
	return v.KVI
}

func (v WPSKVI_v2) GetIndex() types.Index {
	return v.WPSIndex
}
