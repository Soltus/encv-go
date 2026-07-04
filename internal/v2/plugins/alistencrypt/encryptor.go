package alistencrypt

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/v2/crypto"
)

func EncryptToFile(dataReader io.Reader, password string, outputDir string, settings *AlistEncryptPluginConfig) (*crypto.EncryptionResult, error) {
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return nil, err
	}

	fileSize := int64(len(data))

	cipher, err := Create(password, settings.EncType, fileSize)
	if err != nil {
		return nil, err
	}

	cipher.Encrypt(data)

	tempFile, err := os.CreateTemp(outputDir, "*.alistenc.tmp")
	if err != nil {
		return nil, err
	}
	tempPath := tempFile.Name()

	headerBuf := make([]byte, 32)
	copy(headerBuf[:6], []byte(AECTR2Magic))
	headerBuf[6] = 0x02
	headerBuf[7] = 0x00
	binary.BigEndian.PutUint64(headerBuf[24:32], uint64(fileSize))

	if _, err := tempFile.Write(headerBuf); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, err
	}

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, err
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return nil, err
	}

	return &crypto.EncryptionResult{
		TempPath:             tempPath,
		Salt:                 nil,
		IV:                   nil,
		SaltIVHeaderSize:     int64(len(headerBuf)),
		EncryptedPayloadSize: fileSize,
	}, nil
}

func RenameToFinalEncrypted(tempPath string, originalFilename string, outputDir string, suffix string, password string, encType string) (string, error) {
	encName := EncodeName(originalFilename, password, encType)
	finalName := encName + suffix
	finalPath := filepath.Join(outputDir, finalName)

	if err := os.Rename(tempPath, finalPath); err != nil {
		return "", err
	}

	return finalPath, nil
}

func PeekIsAECTR2(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	peekBuf := make([]byte, 6)
	n, _ := io.ReadFull(f, peekBuf)
	return n == 6 && bytes.Equal(peekBuf[:6], []byte(AECTR2Magic))
}
