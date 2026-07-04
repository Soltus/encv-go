package reader

import (
	encvReader "github.com/Soltus/encv-go/internal/v2/reader"
)

func NewDecryptReaderFactory(containerPath string, password string) (encvReader.DecryptReaderFactory, error) {
	return encvReader.NewDecryptReaderFactory(containerPath, password)
}

func NewRemoteDecryptReaderFactory(containerURL string, password string, headers map[string][]string, urlResolver encvReader.URLResolver) (encvReader.DecryptReaderFactory, error) {
	return encvReader.NewRemoteDecryptReaderFactory(containerURL, password, headers, urlResolver)
}
