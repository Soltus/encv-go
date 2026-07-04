package namer

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DefaultBaseNamer 是 BaseNamer 接口的默认实现
type DefaultBaseNamer struct{}

func NewDefaultBaseNamer() *DefaultBaseNamer {
	return &DefaultBaseNamer{}
}

func (n *DefaultBaseNamer) GenerateEncryptedBaseName(originalFilename string) string {
	base := filepath.Base(originalFilename)
	ext := filepath.Ext(base)
	cleanBaseName := strings.TrimSuffix(base, ext)

	if ext == "" {
		return cleanBaseName
	}

	reversedExt := generateReversedExt(ext)
	return fmt.Sprintf("%s.%s", cleanBaseName, reversedExt)
}
