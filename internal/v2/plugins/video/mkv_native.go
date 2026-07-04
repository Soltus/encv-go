package video

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	ebmlIDEBCues                = 0x1C53BB6B
	ebmlIDEBSegment             = 0x18538067
	ebmlIDEBSegmentInfo         = 0x1549A966
	ebmlIDEBPrevUID             = 0x3CB923
	ebmlIDEBNextUID             = 0x3EB923
	ebmlIDEBCuePoint            = 0xBB
	ebmlIDEBCueTime             = 0xB3
	ebmlIDEBCueTrackPositions   = 0xB7
	ebmlIDEBCueTrack            = 0xF7
	ebmlIDEBCueClusterPosition  = 0xF1
	ebmlIDEBCueRelativePosition = 0xF0
	ebmlIDEBCueCodecState       = 0x6A
	ebmlIDEBCueReference        = 0xDB
	ebmlIDEBChapters            = 0x1043A770
	ebmlIDEBEditionEntry        = 0x45B9
	ebmlIDEBCChapterAtom        = 0xB6
	ebmlIDEBCChapterUID         = 0x73C4
	ebmlIDEBCChapterTimeStart   = 0x91
	ebmlIDEBCChapterTimeEnd     = 0x92
	ebmlIDEBCChapterString      = 0x85
	ebmlIDEBCChapterDisplay     = 0x80
	ebmlIDEBCChapterFlagEnabled = 0x4598
	ebmlIDEBCueTrackNumber      = 0xF7
)

func readVINT(r io.Reader) (uint64, int, error) {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, 0, err
	}

	b := buf[0]
	var length int
	var value uint64

	switch {
	case b&0x80 != 0:
		length = 1
		value = uint64(b & 0x7F)
	case b&0x40 != 0:
		length = 2
		value = uint64(b & 0x3F)
	case b&0x20 != 0:
		length = 3
		value = uint64(b & 0x1F)
	case b&0x10 != 0:
		length = 4
		value = uint64(b & 0x0F)
	case b&0x08 != 0:
		length = 5
		value = uint64(b & 0x07)
	case b&0x04 != 0:
		length = 6
		value = uint64(b & 0x03)
	case b&0x02 != 0:
		length = 7
		value = uint64(b & 0x01)
	case b == 0x01:
		length = 8
		value = 0
	default:
		return 0, 0, fmt.Errorf("invalid VINT leading byte: 0x%02X", b)
	}

	if length > 1 {
		rest := make([]byte, length-1)
		if _, err := io.ReadFull(r, rest); err != nil {
			return 0, 0, err
		}
		for _, rb := range rest {
			value = (value << 8) | uint64(rb)
		}
	}

	return value, length, nil
}

func readElementID(r io.Reader) (uint64, int, error) {
	return readVINT(r)
}

func readElementSize(r io.Reader) (uint64, int, error) {
	val, len, err := readVINT(r)
	if err != nil {
		return 0, 0, err
	}
	if val == 0x00FFFFFFFFFFFFFF {
		return 0, len, nil
	}
	return val, len, err
}

type ebmlElement struct {
	ID   uint64
	Size uint64
	Data []byte
}

func readElement(r io.ReadSeeker) (*ebmlElement, error) {
	id, _, err := readElementID(r)
	if err != nil {
		return nil, err
	}

	size, _, err := readElementSize(r)
	if err != nil {
		return nil, err
	}

	if size > 100*1024*1024 {
		return &ebmlElement{ID: id, Size: size}, nil
	}

	data := make([]byte, size)
	if size > 0 {
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
	}

	return &ebmlElement{ID: id, Size: size, Data: data}, nil
}

func findElementIn(data []byte, targetID uint64) ([]byte, bool) {
	r := bytes.NewReader(data)
	for r.Len() > 0 {
		id, _, err := readElementID(r)
		if err != nil {
			return nil, false
		}
		size, _, err := readElementSize(r)
		if err != nil {
			return nil, false
		}

		if id == targetID {
			result := make([]byte, size)
			if size > 0 {
				if _, err := io.ReadFull(r, result); err != nil {
					return nil, false
				}
			}
			return result, true
		}

		if size > 0 {
			r.Seek(int64(size), io.SeekCurrent)
		}
	}
	return nil, false
}

func findElementsIn(data []byte, targetID uint64) [][]byte {
	var results [][]byte
	r := bytes.NewReader(data)
	for r.Len() > 0 {
		id, _, err := readElementID(r)
		if err != nil {
			break
		}
		size, _, err := readElementSize(r)
		if err != nil {
			break
		}

		if id == targetID {
			result := make([]byte, size)
			if size > 0 {
				if _, err := io.ReadFull(r, result); err != nil {
					break
				}
			}
			results = append(results, result)
			continue
		}

		if size > 0 {
			r.Seek(int64(size), io.SeekCurrent)
		}
	}
	return results
}

func readUint(data []byte) uint64 {
	var val uint64
	for _, b := range data {
		val = (val << 8) | uint64(b)
	}
	return val
}

func CheckFileForCuesNative(filePath string) (bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	const searchLimit = 64 * 1024 * 1024

	for {
		id, _, err := readElementID(f)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return false, nil
			}
			return false, err
		}

		size, _, err := readElementSize(f)
		if err != nil {
			return false, err
		}

		if id == ebmlIDEBCues {
			return true, nil
		}

		if id == ebmlIDEBSegment {
			continue
		}

		if size > 0 {
			seekSize := int64(size)
			if seekSize > searchLimit {
				return false, nil
			}
			if _, err := f.Seek(seekSize, io.SeekCurrent); err != nil {
				return false, err
			}
		}
	}
}

func IsMkvPartOfSplitNative(filePath string) (bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	const headerLimit = 4 * 1024 * 1024
	buf := make([]byte, headerLimit)
	bytesRead, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}

	segmentData, found := findTopLevelElement(buf[:bytesRead], ebmlIDEBSegment)
	if !found {
		return false, nil
	}

	infoData, found := findElementIn(segmentData, ebmlIDEBSegmentInfo)
	if !found {
		return false, nil
	}

	_, hasPrev := findElementIn(infoData, ebmlIDEBPrevUID)
	_, hasNext := findElementIn(infoData, ebmlIDEBNextUID)

	return hasPrev || hasNext, nil
}

func findTopLevelElement(data []byte, targetID uint64) ([]byte, bool) {
	r := bytes.NewReader(data)
	for r.Len() > 0 {
		id, _, err := readElementID(r)
		if err != nil {
			return nil, false
		}
		size, _, err := readElementSize(r)
		if err != nil {
			return nil, false
		}

		if id == targetID {
			result := make([]byte, size)
			if size > 0 {
				if _, err := io.ReadFull(r, result); err != nil {
					return nil, false
				}
			}
			return result, true
		}

		if size > 0 {
			r.Seek(int64(size), io.SeekCurrent)
		}
	}
	return nil, false
}

func ExtractKeyFrameOffsetsFromMKVCuesNative(filePath string) ([]uint64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var segmentData []byte
	for {
		id, _, err := readElementID(f)
		if err != nil {
			return nil, fmt.Errorf("failed to find Segment element: %w", err)
		}
		size, _, err := readElementSize(f)
		if err != nil {
			return nil, fmt.Errorf("failed to read element size: %w", err)
		}

		if id == ebmlIDEBSegment {
			readSize := size
			if readSize > 100*1024*1024 {
				readSize = 100 * 1024 * 1024
			}
			segmentData = make([]byte, readSize)
			if readSize > 0 {
				n, err := io.ReadFull(f, segmentData)
				if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
					return nil, err
				}
				segmentData = segmentData[:n]
			}
			break
		}

		if size > 0 {
			if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}

	cuesData, found := findElementIn(segmentData, ebmlIDEBCues)
	if !found {
		return nil, fmt.Errorf("Cues element not found in MKV file")
	}

	cuePoints := findElementsIn(cuesData, ebmlIDEBCuePoint)
	var offsets []uint64

	for _, cpData := range cuePoints {
		trackPositions := findElementsIn(cpData, ebmlIDEBCueTrackPositions)
		for _, tpData := range trackPositions {
			clusterPosData, found := findElementIn(tpData, ebmlIDEBCueClusterPosition)
			if found && len(clusterPosData) > 0 {
				offset := readUint(clusterPosData)
				offsets = append(offsets, offset)
			}
		}
	}

	return offsets, nil
}

func ExtractChaptersFromMKVNative(filePath string) ([]MKVChapterInfo, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var segmentData []byte
	for {
		id, _, err := readElementID(f)
		if err != nil {
			return nil, fmt.Errorf("failed to find Segment element: %w", err)
		}
		size, _, err := readElementSize(f)
		if err != nil {
			return nil, fmt.Errorf("failed to read element size: %w", err)
		}

		if id == ebmlIDEBSegment {
			readSize := size
			if readSize > 100*1024*1024 {
				readSize = 100 * 1024 * 1024
			}
			segmentData = make([]byte, readSize)
			if readSize > 0 {
				n, err := io.ReadFull(f, segmentData)
				if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
					return nil, err
				}
				segmentData = segmentData[:n]
			}
			break
		}

		if size > 0 {
			if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}

	chaptersData, found := findElementIn(segmentData, ebmlIDEBChapters)
	if !found {
		return nil, fmt.Errorf("Chapters element not found in MKV file")
	}

	var chapters []MKVChapterInfo

	editionEntries := findElementsIn(chaptersData, ebmlIDEBEditionEntry)
	for _, edData := range editionEntries {
		chapterAtoms := findElementsIn(edData, ebmlIDEBCChapterAtom)
		for _, atomData := range chapterAtoms {
			var ch MKVChapterInfo

			if timeStartData, found := findElementIn(atomData, ebmlIDEBCChapterTimeStart); found {
				ch.StartTime = time.Duration(readUint(timeStartData))
			}
			if timeEndData, found := findElementIn(atomData, ebmlIDEBCChapterTimeEnd); found {
				ch.EndTime = time.Duration(readUint(timeEndData))
			}

			displays := findElementsIn(atomData, ebmlIDEBCChapterDisplay)
			for _, dispData := range displays {
				if stringData, found := findElementIn(dispData, ebmlIDEBCChapterString); found {
					ch.Title = string(stringData)
					break
				}
			}

			chapters = append(chapters, ch)
		}
	}

	return chapters, nil
}

func ReadMKVSegmentInfoNative(filePath string) (prevUID, nextUID []byte, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	const headerLimit = 4 * 1024 * 1024
	buf := make([]byte, headerLimit)
	bytesRead, readErr := io.ReadFull(f, buf)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return nil, nil, readErr
	}

	segmentData, found := findTopLevelElement(buf[:bytesRead], ebmlIDEBSegment)
	if !found {
		return nil, nil, fmt.Errorf("Segment element not found")
	}

	infoData, found := findElementIn(segmentData, ebmlIDEBSegmentInfo)
	if !found {
		return nil, nil, nil
	}

	if prevData, found := findElementIn(infoData, ebmlIDEBPrevUID); found {
		prevUID = prevData
	}
	if nextData, found := findElementIn(infoData, ebmlIDEBNextUID); found {
		nextUID = nextData
	}

	return prevUID, nextUID, nil
}

var _ = binary.BigEndian
