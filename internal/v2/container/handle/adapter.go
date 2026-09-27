package handle

import (
	"encoding/base64"
	"encoding/json"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// kviIndexKeys 把 KVI 里的 *_index 键映射到真实索引类型。
//
// ⚠️ 不能只按容器类型（ContainerType）猜：v4 的 document 是个粗粒度分类，
// PDF 和 WPS/Office（doc/docx/xls/xlsx/ppt/pptx）都属于它。旧实现把 document
// 一律映射成 "PDF"，于是 docx 容器被按 PDF 索引解析 —— 索引里根本没有 pdf_index，
// 解析出空索引 → 拿不到原始文件名 → 解密时把输出目录当文件创建（is a directory），
// /stream 也会返回 0 字节。真实索引类型只能看 KVI 里到底写了哪个 *_index。
var kviIndexKeys = []struct {
	key  string
	kind types.IndexKind
}{
	{"wps_index", "WPS"},
	{"pdf_index", "PDF"},
	{"text_index", "text"},
	{"video_index", "video"},
	{"audio_index", "audio"},
	{"image_index", "image"},
}

func inferKindFromKVI(rawKVI json.RawMessage) (types.IndexKind, bool) {
	if len(rawKVI) == 0 {
		return "", false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(rawKVI, &probe); err != nil {
		return "", false
	}
	for _, e := range kviIndexKeys {
		if _, ok := probe[e.key]; ok {
			return e.kind, true
		}
	}
	return "", false
}

func AdaptV4ToV2(v4 *types.Manifest_v4, header *types.EnvelopeHeaderV4) *types.Manifest {
	fragments := make([]types.Fragment, len(v4.Segments))
	var runningOffset uint64
	for i, seg := range v4.Segments {
		nonce, _ := base64.StdEncoding.DecodeString(seg.Nonce)

		var encDataSize uint64
		var physicalOffset uint64

		if len(nonce) > 0 {
			encDataSize = seg.Size - uint64(types.SegmentHeaderSize) - uint64(len(nonce))
			physicalOffset = seg.Offset + uint64(types.SegmentHeaderSize) + uint64(len(nonce))
		} else {
			encDataSize = seg.Size
			physicalOffset = seg.Offset
		}

		fragments[i] = types.Fragment{
			ID:                seg.ID,
			Type:              types.FragmentType_SeekableStream,
			Length:            encDataSize,
			GlobalStartOffset: runningOffset,
			DataCRC32:         0,
			PhysicalPath:      "",
			PhysicalOffset:    physicalOffset,
		}
		runningOffset += encDataSize
	}

	var kind types.IndexKind
	// 优先按 KVI 里实际存在的 *_index 判断；判断不出来才回退到容器类型（旧行为）。
	if inferred, ok := inferKindFromKVI(v4.KVI); ok {
		kind = inferred
	} else {
		switch v4.ContainerType {
		case "video":
			kind = "video"
		case "audio":
			kind = "audio"
		case "image":
			kind = "image"
		case "document":
			kind = "PDF" // 旧容器没有 *_index 键时的兜底，保持历史行为
		case "text":
			kind = "text"
		default:
			kind = types.IndexKind(v4.ContainerType)
		}
	}

	return &types.Manifest{
		Version:   int64(header.Version),
		Kind:      kind,
		KVI:       v4.KVI,
		Fragments: fragments,
	}
}

func indexKindToContainerType(kind types.IndexKind) uint16 {
	switch kind {
	case "video":
		return types.ContainerTypeVideo
	case "audio":
		return types.ContainerTypeAudio
	case "image":
		return types.ContainerTypeImage
	case "PDF", "WPS":
		return types.ContainerTypeDocument
	case "text":
		return types.ContainerTypeText
	default:
		return types.ContainerTypeUnknown
	}
}

func hasSeekableFragment(mf *types.Manifest) bool {
	for _, frag := range mf.Fragments {
		if frag.Type == types.FragmentType_SeekableStream {
			return true
		}
	}
	return false
}
