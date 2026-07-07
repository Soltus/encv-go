package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"github.com/Soltus/encv-go/internal/v2/types"
	"golang.org/x/crypto/pbkdf2"
)

const (
	KEKKeySize    = 32
	KDFIterations = 100000

	WrappedDEKNonceSize = 12
	WrappedDEKTagSize   = 16
)

var (
	ErrInvalidKEKLength = errors.New("KEK must be 32 bytes for AES-256-GCM")
	ErrDecryptDEKFailed = errors.New("failed to decrypt DEK: GCM tag mismatch (wrong password?)")
)

func GenerateDEK(keyLen int) ([]byte, error) {
	if keyLen <= 0 {
		keyLen = KeySize_v4_128
	}
	dek := make([]byte, keyLen)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, err
	}
	return dek, nil
}

func DeriveKEK(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, KDFIterations, KEKKeySize, sha256.New)
}

func WrapDEK(dek, kek, aad []byte) (*types.WrappedDEK, error) {
	if len(kek) != KEKKeySize {
		return nil, ErrInvalidKEKLength
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, WrappedDEKNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	sealed := gcm.Seal(nil, nonce, dek, aad)

	ciphertext := sealed[:len(sealed)-WrappedDEKTagSize]
	tag := sealed[len(sealed)-WrappedDEKTagSize:]

	return types.NewWrappedDEK(nonce, ciphertext, tag, aad), nil
}

func UnwrapDEK(wd *types.WrappedDEK, kek []byte) ([]byte, error) {
	if !wd.IsValid() {
		return nil, types.ErrInvalidWrappedDEK
	}
	if len(kek) != KEKKeySize {
		return nil, ErrInvalidKEKLength
	}

	nonce, err := wd.Nonce()
	if err != nil {
		return nil, err
	}
	ciphertext, err := wd.Ciphertext()
	if err != nil {
		return nil, err
	}
	tag, err := wd.Tag()
	if err != nil {
		return nil, err
	}
	aad, err := wd.AAD()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	sealed := make([]byte, len(ciphertext)+len(tag))
	copy(sealed, ciphertext)
	copy(sealed[len(ciphertext):], tag)

	dek, err := gcm.Open(nil, nonce, sealed, aad)
	if err != nil {
		return nil, ErrDecryptDEKFailed
	}

	return dek, nil
}

func WrapDEKToBase64(dek, kek, aad []byte) (nonceB64, ciphertextB64, tagB64, aadB64 string, err error) {
	wd, err := WrapDEK(dek, kek, aad)
	if err != nil {
		return "", "", "", "", err
	}
	return wd.NonceBase64, wd.CiphertextBase64, wd.TagBase64, wd.AADBase64, nil
}

func UnwrapDEKFromBase64(nonceB64, ciphertextB64, tagB64, aadB64 string, kek []byte) ([]byte, error) {
	wd := &types.WrappedDEK{
		Algorithm:        types.WrappedDEKAlgAES256GCM,
		NonceBase64:      nonceB64,
		CiphertextBase64: ciphertextB64,
		TagBase64:        tagB64,
		AADBase64:        aadB64,
	}
	return UnwrapDEK(wd, kek)
}

func init() {
	_ = base64.StdEncoding
}
