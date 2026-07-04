package reader

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// SequentialDecryptReader 用于解密原子文件
type SequentialDecryptReader struct {
	containerReader   EncryptedContainerReader
	key, iv           []byte
	atomicFragments   []types.Fragment
	currentFragIndex  int
	currentFragReader io.ReadCloser
	currentDecryptor  io.Reader
}

func NewSequentialDecryptReader(cr EncryptedContainerReader, password string) (DecryptReader, error) {
	manifest := cr.GetManifest()
	kviProvider, err := cr.GetKVIProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal KVI from manifest: %w", err)
	}

	key, iv, err := deriveKeyAndIV(kviProvider, password)
	if err != nil {
		return nil, err
	}

	r := &SequentialDecryptReader{
		containerReader: cr,
		key:             key,
		iv:              iv,
		atomicFragments: filterFragmentsByType(manifest.Fragments, string(types.FragmentType_AtomicFile)),
	}
	return r, nil
}

func (r *SequentialDecryptReader) Read(p []byte) (n int, err error) {
	if len(r.atomicFragments) == 0 {
		return 0, io.EOF
	}
	if r.currentDecryptor == nil {
		if err := r.setupNextFragmentDecryptor(); err != nil {
			return 0, err // io.EOF or other error
		}
	}
	n, err = r.currentDecryptor.Read(p)
	if err == io.EOF {
		r.currentFragReader.Close()
		r.currentFragReader = nil
		r.currentDecryptor = nil
		r.currentFragIndex++
		// 【直接复用】源自旧代码的清晰递归逻辑
		return r.Read(p)
	}
	return n, err
}

func (r *SequentialDecryptReader) setupNextFragmentDecryptor() error {
	if r.currentFragIndex >= len(r.atomicFragments) {
		return io.EOF
	}
	frag := &r.atomicFragments[r.currentFragIndex]
	rawReader, err := r.containerReader.GetFragmentReader(frag.ID)
	if err != nil {
		return fmt.Errorf("failed to get reader for atomic file %s: %w", frag.ID, err)
	}
	r.currentFragReader = rawReader

	block, err := aes.NewCipher(r.key)
	if err != nil {
		return fmt.Errorf("failed to create aes cipher: %w", err)
	}
	stream := cipher.NewCTR(block, r.iv)
	r.currentDecryptor = &cipher.StreamReader{S: stream, R: rawReader}
	return nil
}

func (r *SequentialDecryptReader) Close() error {
	if r.currentFragReader != nil {
		r.currentFragReader.Close()
	}
	r.currentFragReader = nil
	return nil
}
