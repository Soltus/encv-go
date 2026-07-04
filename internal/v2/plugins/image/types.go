package image

import (
	"time"

	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/dsoprea/go-exif/v3"
)

const IndexKindImage types.IndexKind = "image"

// ImageIndex 是解密图像所需的所有元数据的容器
type ImageIndex struct {
	ID                string `json:"id"`
	OriginalFileSize  int64  `json:"original_file_size"`
	MimeType          string `json:"mime_type"`
	Format            string `json:"format"`
	OriginalFilename  string `json:"original_filename"`
	OriginalInputPath string `json:"originalInputPath"`
	OriginalFileMD5   string `json:"original_file_md5"`
	EncryptedFileMD5  string `json:"encrypted_file_md5"`

	// --- 图片特有字段 ---
	Make             string     // 相机制造商
	Model            string     // 相机型号
	DateTimeOriginal *time.Time // 拍摄时间，使用指针以表示可能不存在
	Software         string     // 处理软件
	Width            int        // 图片宽度
	Height           int        // 图片高度
	Orientation      int        // 旋转方向

	// --- GPS 信息 ---
	GPSLatitude  *exif.GpsDegrees // 纬度
	GPSLongitude *exif.GpsDegrees // 经度

	// --- 其他 EXIF 标签 ---
	// 使用 map 存储未能明确结构化的其他所有 EXIF 标签，提供最大灵活性
	ExifTags map[string]interface{}
}

// 【新增】实现 Index 接口
func (i *ImageIndex) GetOriginalFilename() string { return i.OriginalFilename }
func (v *ImageIndex) GetOriginalFileSize() int64  { return v.OriginalFileSize }
func (v *ImageIndex) GetOriginalFileMD5() string  { return v.OriginalFileMD5 }
func (v *ImageIndex) GetEncryptedFileMD5() string { return v.EncryptedFileMD5 }
func (v *ImageIndex) GetMimeType() string         { return v.MimeType }

// 视频容器专用的 KVI
type ImageKVI_v2 struct {
	types.KVI
	ImageIndex *ImageIndex `json:"image_index"`
}

func (v ImageKVI_v2) GetKind() types.IndexKind {
	return IndexKindImage
}

// 【关键新增】实现 KVIProvider 接口
func (v ImageKVI_v2) GetEncryptionInfo() types.KVI {
	return v.KVI
}

func (v ImageKVI_v2) GetIndex() types.Index {
	return v.ImageIndex
}
