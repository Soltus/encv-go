package crypto

import (
	"fmt"
	"io"
	"os"

	"github.com/Soltus/encv-go/internal/v2/types"
)

type EncryptionResult struct {
	TempPath             string
	Salt                 []byte
	IV                   []byte
	SaltIVHeaderSize     int64
	EncryptedPayloadSize int64

	WrappedDEK *types.WrappedDEK
}

type EncryptionContext struct {
	Salt       []byte
	IV         []byte
	DEK        []byte
	WrappedDEK *types.WrappedDEK
}

func PrepareEncryptionContext(password string) (*EncryptionContext, error) {
	salt, err := GenerateSalt_v2(types.SaltSize_v2)
	if err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	iv, err := GenerateIV_v2(types.IVSize_v2)
	if err != nil {
		return nil, fmt.Errorf("failed to generate iv: %w", err)
	}

	kek := DeriveKEK(password, salt)

	dek, err := GenerateDEK(KeySize_v4_128)
	if err != nil {
		return nil, fmt.Errorf("failed to generate DEK: %w", err)
	}

	aad := salt
	wrappedDEK, err := WrapDEK(dek, kek, aad)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap DEK: %w", err)
	}

	return &EncryptionContext{
		Salt:       salt,
		IV:         iv,
		DEK:        dek,
		WrappedDEK: wrappedDEK,
	}, nil
}

func EncryptToTempFile_v2(src io.Reader, password string, outputDir string) (*EncryptionResult, error) {
	ctx, err := PrepareEncryptionContext(password)
	if err != nil {
		return nil, err
	}

	tempFile, err := os.CreateTemp(outputDir, "*.enc.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	bytesWritten := 0

	if _, err := tempFile.Write(ctx.Salt); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to write salt: %w", err)
	}
	bytesWritten += len(ctx.Salt)

	if _, err := tempFile.Write(ctx.IV); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to write iv: %w", err)
	}
	bytesWritten += len(ctx.IV)

	saltIVSize := int64(bytesWritten)

	if err := EncryptStream_v2(src, tempFile, ctx.DEK, ctx.IV); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to encrypt stream: %w", err)
	}

	fileInfo, err := tempFile.Stat()
	if err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to stat temp file: %w", err)
	}
	totalFileSize := fileInfo.Size()

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	payloadSize := totalFileSize - saltIVSize

	return &EncryptionResult{
		TempPath:             tempPath,
		Salt:                 ctx.Salt,
		IV:                   ctx.IV,
		SaltIVHeaderSize:     saltIVSize,
		EncryptedPayloadSize: payloadSize,
		WrappedDEK:           ctx.WrappedDEK,
	}, nil
}
