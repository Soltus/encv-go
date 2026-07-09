package physical

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

// SinglePhysicalPacker 将数据打包成一个单一、完整的容器文件
type SinglePhysicalPacker struct{}

func NewSinglePhysicalPacker() *SinglePhysicalPacker {
	return &SinglePhysicalPacker{}
}

// Pack 实现 PhysicalPacker 接口
// SinglePhysicalPacker namer 和 startIdx 参数暂时无用，保留接口兼容性
func (p *SinglePhysicalPacker) Pack(manifest *types.Manifest, req *PackRequest) (string, error) {
	outputPath := filepath.Join(req.OutputDir, req.FinalFileName)
	tempPath := outputPath + ".tmp"

	var err error

	if req.HeaderVersion == 4 {
		header, headerErr := types.CreateHeaderV4(
			true,
			req.ContainerType,
			req.IsSeekable,
			req.SpecialIDType,
			req.SpecialID,
			req.PasswordHint,
		)
		if headerErr != nil {
			return "", fmt.Errorf("failed to prepare v4 header: %w", headerErr)
		}

		tempWriter, writerErr := writer.NewSingleFileContainerWriterV4(tempPath, header)
		if writerErr != nil {
			return "", fmt.Errorf("failed to create v4 container writer: %w", writerErr)
		}

		if req.WrappedDEK != nil {
			tempWriter.SetWrappedDEK(req.WrappedDEK)
		}

		finalPath, packErr := p.writeAndClose(req.EncryptedDataReader, manifest, tempWriter, tempPath, outputPath)
		if packErr != nil {
			return "", packErr
		}

		log.Printf("✅ [SinglePhysicalPacker] Packed (V4) to: %s\n", finalPath)
		return finalPath, nil
	}

	var header *types.EnvelopeHeaderV3

	if req.HeaderVersion == 3 {
		idData := req.SpecialID
		header, err = types.CreateHeaderV3(
			true,
			req.SpecialIDType,
			idData,
		)
		if err != nil {
			return "", fmt.Errorf("failed to prepare v3 header: %w", err)
		}
	}

	tempWriter, err := writer.NewSingleFileContainerWriter(tempPath, header)
	if err != nil {
		return "", fmt.Errorf("failed to create container writer: %w", err)
	}

	finalPath, err := p.writeAndClose(req.EncryptedDataReader, manifest, tempWriter, tempPath, outputPath)
	if err != nil {
		return "", err
	}

	log.Printf("✅ [SinglePhysicalPacker] Packed to: %s\n", finalPath)
	return finalPath, nil
}

const streamCopyBufferSize = 1 * 1024 * 1024

func (p *SinglePhysicalPacker) writeAndClose(data io.Reader, manifest *types.Manifest, tempWriter *writer.SingleFileContainerWriter, tempPath, outputPath string) (string, error) {
	var success bool
	defer func() {
		if !success {
			tempWriter.Close()
			os.Remove(tempPath)
		}
	}()

	dataFrags := make([]*types.Fragment, 0, len(manifest.Fragments))
	for i := range manifest.Fragments {
		frag := &manifest.Fragments[i]
		if frag.Type == types.FragmentType_Metadata {
			log.Printf("DEBUG: Skipping metadata fragment '%s' (not part of data stream)", frag.ID)
			continue
		}
		dataFrags = append(dataFrags, frag)
	}

	if len(dataFrags) == 1 {
		if err := p.writeSingleFragmentStream(data, dataFrags[0], tempWriter); err != nil {
			return "", err
		}
	} else {
		buf := make([]byte, streamCopyBufferSize)
		for _, frag := range dataFrags {
			if err := p.writeFragmentStream(data, frag, tempWriter, buf); err != nil {
				return "", err
			}
		}
	}

	if err := tempWriter.WriteManifest(manifest); err != nil {
		return "", fmt.Errorf("failed to write manifest: %w", err)
	}

	if err := tempWriter.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	if err := os.Rename(tempPath, outputPath); err != nil {
		return "", fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return outputPath, nil
}

func (p *SinglePhysicalPacker) writeSingleFragmentStream(data io.Reader, frag *types.Fragment, tempWriter *writer.SingleFileContainerWriter) error {
	if err := tempWriter.BeginFragment(frag); err != nil {
		return fmt.Errorf("failed to begin fragment: %w", err)
	}

	buf := make([]byte, streamCopyBufferSize)
	var totalWritten int64 = 0

	for {
		n, readErr := data.Read(buf)
		if n > 0 {
			if err := tempWriter.WriteFragmentData(buf[:n]); err != nil {
				return fmt.Errorf("failed to write fragment data: %w", err)
			}
			totalWritten += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("failed to read data: %w", readErr)
		}
	}

	if err := tempWriter.FinishFragment(); err != nil {
		return fmt.Errorf("failed to finish fragment: %w", err)
	}

	frag.Length = uint64(totalWritten)
	return nil
}

func (p *SinglePhysicalPacker) writeFragmentStream(data io.Reader, frag *types.Fragment, tempWriter *writer.SingleFileContainerWriter, buf []byte) error {
	if err := tempWriter.BeginFragment(frag); err != nil {
		return fmt.Errorf("failed to begin fragment: %w", err)
	}

	remaining := int64(frag.Length)
	for remaining > 0 {
		readSize := int64(len(buf))
		if remaining < readSize {
			readSize = remaining
		}
		n, readErr := io.ReadFull(data, buf[:readSize])
		if n > 0 {
			if err := tempWriter.WriteFragmentData(buf[:n]); err != nil {
				return fmt.Errorf("failed to write fragment data: %w", err)
			}
			remaining -= int64(n)
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("failed to read data for fragment: %w", readErr)
		}
	}

	if err := tempWriter.FinishFragment(); err != nil {
		return fmt.Errorf("failed to finish fragment: %w", err)
	}
	return nil
}

// 单一文件物理解包器
type SinglePhysicalUnpacker struct{}

func NewSinglePhysicalUnpacker() *SinglePhysicalUnpacker {
	return &SinglePhysicalUnpacker{}
}

func (u *SinglePhysicalUnpacker) Unpack(ctx context.Context, mainContainerPath string) (string, func(), error) {
	// 直接返回原始路径，无需清理，忽略 ctx
	return mainContainerPath, func() {}, nil
}
