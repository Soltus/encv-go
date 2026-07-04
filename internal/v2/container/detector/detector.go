package detector

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/Soltus/encv-go/internal/v2/container/envelope"
	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// ErrNotAnENCVContainer 表示输入流不是一个 ENCV 容器
var ErrNotAnENCVContainer = errors.New("not an ENCV container")

// ErrEmptyInput 表示输入流为空
var ErrEmptyInput = errors.New("empty input")

// ErrHeaderTruncated 表示 ENCV 容器头被截断（不足 2048 字节）
var ErrHeaderTruncated = errors.New("header truncated")

// DetectResult 是流式容器检测的结构化结果
//
// 字段语义：
//   - IsENCVContainer:  是否为 ENCV 容器（仅基于魔数 ENCV，与文件扩展名无关）
//   - Version:          容器版本（0=未识别, 2=v2, 3=v3, 4=v4）
//   - ContainerType:    容器类型（0=unknown, 1=video, 2=audio, 3=image, 4=document, 5=text）
//   - IsSeekable:       容器是否可 seek（v4 Header.IsSeekable）
//   - CipherMode:       v4 加密算法（0=AES-128-CTR, 1=AES-256-CTR，v2/v3 始终为 0）
type DetectResult struct {
	IsENCVContainer bool
	Version         uint16
	ContainerType   uint16
	IsSeekable      bool
	CipherMode      uint16
}

// MinimumHeaderBytes 是 DetectContainerFromReader 读取的最小字节数：
// 4 字节魔数 + 2 字节版本号 = 6 字节
const MinimumHeaderBytes = 6

// DetectContainerFromReader 从任意 io.Reader 流式检测 ENCV 容器。
//
// 检测逻辑（仅基于字节流内容，不依赖任何文件扩展名）：
//  1. 至少读取 6 字节（魔数 + 版本号），不足则返回 ErrEmptyInput 或 ErrHeaderTruncated
//  2. 校验魔数是否为 ENCV（"E","N","C","V"），否则返回 IsENCVContainer=false
//  3. 用 DetectHeaderVersion 解析版本号
//  4. 如果是 v4 容器且输入 >= 2048 字节，尝试读取完整 v4 Header
//     - ContainerType / IsSeekable / CipherMode 来自 Header
//     - 否则 Header 派生字段为零值
//  5. 返回 DetectResult 结构化结果
//
// 与 DetectContainerType(path) 的区别：本函数接受 io.Reader，
// 适用于流式场景（HTTP body / 内存 buffer / pipe），不依赖文件系统。
func DetectContainerFromReader(r io.Reader) (DetectResult, error) {
	// 读前 6 字节
	head := make([]byte, MinimumHeaderBytes)
	n, err := io.ReadFull(r, head)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || n == 0 {
			if n == 0 {
				return DetectResult{}, ErrEmptyInput
			}
			return DetectResult{}, ErrHeaderTruncated
		}
		return DetectResult{}, fmt.Errorf("failed to read magic bytes: %w", err)
	}

	// 校验魔数
	magic := [4]byte{head[0], head[1], head[2], head[3]}
	if !bytes.Equal(magic[:], types.MagicHeader_v2[:]) {
		return DetectResult{
			IsENCVContainer: false,
		}, nil
	}

	// 解析版本
	version := types.DetectHeaderVersion(head)
	if version == 0 {
		return DetectResult{
			IsENCVContainer: false,
		}, nil
	}

	result := DetectResult{
		IsENCVContainer: true,
		Version:         uint16(version),
	}

	// 尝试读完整 v4 Header
	if version == 4 {
		buf := make([]byte, types.EnvelopeHeaderSize_v4)
		copy(buf[:MinimumHeaderBytes], head)
		if _, err := io.ReadFull(r, buf[MinimumHeaderBytes:]); err == nil {
			// 完整 Header 可读
			if hdr, herr := parseV4HeaderBytes(buf); herr == nil {
				result.ContainerType = hdr.ContainerType
				result.IsSeekable = hdr.IsSeekable != 0
				result.CipherMode = hdr.CipherMode
			}
		}
		// Header 不足 2048 字节：result 保持 zero value，仍报告为 v4 容器
	}

	return result, nil
}

// parseV4HeaderBytes 解析 2048 字节的 v4 Header 二进制数据。
// 这是一个轻量级解析器，仅提取 detector 关心的字段。
func parseV4HeaderBytes(buf []byte) (*types.EnvelopeHeaderV4, error) {
	if len(buf) < types.EnvelopeHeaderSize_v4 {
		return nil, ErrHeaderTruncated
	}
	hdr := &types.EnvelopeHeaderV4{
		Magic:         [4]byte{buf[0], buf[1], buf[2], buf[3]},
		Version:       binary.LittleEndian.Uint16(buf[4:6]),
		Flags:         binary.LittleEndian.Uint16(buf[6:8]),
		ContainerType: binary.LittleEndian.Uint16(buf[8:10]),
		IsSeekable:    buf[10],
		Reserved1:     buf[11],
		IDType:        binary.LittleEndian.Uint32(buf[12:16]),
		IDLength:      binary.LittleEndian.Uint32(buf[16:20]),
	}
	copy(hdr.PasswordHint[:], buf[20:36])
	copy(hdr.SpecialID[:], buf[36:36+types.SpecialIDMaxLenV4])
	hdr.ManifestOffset = binary.LittleEndian.Uint32(buf[2028:2032])
	hdr.ManifestLength = binary.LittleEndian.Uint32(buf[2032:2036])
	hdr.HeaderCRC32 = binary.LittleEndian.Uint32(buf[2036:2040])
	// CipherMode 在 offset 2040-2042（复用 Reserved2 前 2 字节）
	hdr.CipherMode = binary.LittleEndian.Uint16(buf[types.CipherModeOffsetV4 : types.CipherModeOffsetV4+2])
	return hdr, nil
}

func IsEncvContainerFromBytes(data []byte) (bool, error) {
	if len(data) < 6 {
		return false, nil
	}
	version := types.DetectHeaderVersion(data[:6])
	if version == 0 {
		return false, nil
	}

	if version == 4 {
		footerSize := types.EnvelopeFooterSize_v4
		if len(data) < footerSize {
			return false, nil
		}
		footerData := data[len(data)-footerSize:]
		footer := &types.EnvelopeFooterV4{}
		if err := binary.Read(bytes.NewReader(footerData), binary.LittleEndian, footer); err != nil {
			return false, nil
		}
		return bytes.Equal(footer.Magic[:], types.MagicFooter_v2[:]), nil
	}

	size := types.EnvelopeFooterSize_v2
	if len(data) < size {
		return false, nil
	}
	footerData := data[len(data)-size:]
	_, err := envelope.ParseEnvelopeFooterFromBytes(footerData)
	return err == nil, nil
}

func DetectContainer(filePath string) (*types.ContainerDescriptor, error) {
	src, err := containerhandle.NewFileSource(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()

	h, err := containerhandle.Open(src)
	if err != nil {
		return nil, fmt.Errorf("detection failed: %w", err)
	}
	defer h.Close()

	return &types.ContainerDescriptor{
		FilePath:   filePath,
		IsSeekable: h.IsSeekable(),
	}, nil
}

func DetectContainerType(path string) (uint16, error) {
	src, err := containerhandle.NewFileSource(path)
	if err != nil {
		return types.ContainerTypeUnknown, err
	}
	defer src.Close()

	h, err := containerhandle.Open(src)
	if err != nil {
		return types.ContainerTypeUnknown, err
	}
	defer h.Close()

	return h.ContainerType(), nil
}

func DetectIsSeekable(path string) (bool, error) {
	src, err := containerhandle.NewFileSource(path)
	if err != nil {
		return false, err
	}
	defer src.Close()

	h, err := containerhandle.Open(src)
	if err != nil {
		return false, err
	}
	defer h.Close()

	return h.IsSeekable(), nil
}

func DetectV4Header(path string) (*types.EnvelopeHeaderV4, error) {
	src, err := containerhandle.NewFileSource(path)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	h, err := containerhandle.Open(src)
	if err != nil {
		return nil, err
	}
	defer h.Close()

	if h.Version() != 4 {
		return nil, fmt.Errorf("file is not a v4 container (detected version: %d)", h.Version())
	}
	return h.HeaderV4(), nil
}

func DetectIndexKind(filePath string) (types.IndexKind, error) {
	src, err := containerhandle.NewFileSource(filePath)
	if err != nil {
		return "", fmt.Errorf("invalid container (cannot open file): %w", err)
	}
	defer src.Close()

	h, err := containerhandle.Open(src)
	if err != nil {
		return "", fmt.Errorf("invalid container (cannot detect header): %w", err)
	}
	defer h.Close()

	mf := h.Manifest()
	if mf != nil && mf.Kind != "" {
		return mf.Kind, nil
	}
	return "", fmt.Errorf("could not determine index kind from manifest")
}
