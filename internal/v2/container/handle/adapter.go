package handle

import (
	"encoding/base64"

	"github.com/Soltus/encv-go/internal/v2/types"
)

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
	switch v4.ContainerType {
	case "video":
		kind = "video"
	case "audio":
		kind = "audio"
	case "image":
		kind = "image"
	case "document":
		kind = "PDF"
	case "text":
		kind = "text"
	default:
		kind = types.IndexKind(v4.ContainerType)
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
