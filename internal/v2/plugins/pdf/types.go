package pdf

import (
	"github.com/Soltus/encv-go/internal/v2/types"
)

const IndexKindPDF types.IndexKind = "PDF"

type PDFIndex struct {
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
//
// ⚠️ 全部对 nil 接收者兜底：容器索引解析不出来时拿到的是 (*PDFIndex)(nil)，
// 直接解引用会让整个进程 panic —— 实测 WebDAV 后台索引器扫到这类容器时
// 会把服务直接带崩（internal/webdav/fs_v2.go 的 addOrUpdateEntry 调用链）。
func (t *PDFIndex) GetOriginalFilename() string {
	if t == nil {
		return ""
	}
	return t.OriginalFilename
}
func (t *PDFIndex) GetOriginalFileSize() int64 {
	if t == nil {
		return 0
	}
	return t.OriginalFileSize
}
func (v *PDFIndex) GetOriginalFileMD5() string {
	if v == nil {
		return ""
	}
	return v.OriginalFileMD5
}
func (v *PDFIndex) GetEncryptedFileMD5() string {
	if v == nil {
		return ""
	}
	return v.EncryptedFileMD5
}
func (t *PDFIndex) GetMimeType() string {
	if t == nil {
		return ""
	}
	return t.MimeType
}

// 视频容器专用的 KVI
type PDFKVI_v2 struct {
	types.KVI
	PDFIndex *PDFIndex `json:"pdf_index"`
}

func (v PDFKVI_v2) GetKind() types.IndexKind {
	return IndexKindPDF
}

// 【关键新增】实现 KVIProvider 接口
func (v PDFKVI_v2) GetEncryptionInfo() types.KVI {
	return v.KVI
}

func (v PDFKVI_v2) GetIndex() types.Index {
	return v.PDFIndex
}
