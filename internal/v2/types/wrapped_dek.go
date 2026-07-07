package types

import (
	"encoding/base64"
	"errors"
)

var (
	ErrInvalidWrappedDEK = errors.New("invalid wrapped DEK structure")
	ErrDecryptDEKFailed  = errors.New("failed to decrypt DEK: GCM tag mismatch")
)

const (
	WrappedDEKAlgAES256GCM = "aes-256-gcm"

	DEKNonceSize = 12
	DEKTagSize   = 16
)

type WrappedDEK struct {
	Algorithm        string `json:"algorithm"`
	NonceBase64      string `json:"nonce_base64"`
	CiphertextBase64 string `json:"ciphertext_base64"`
	TagBase64        string `json:"tag_base64"`
	AADBase64        string `json:"aad_base64,omitempty"`
}

func (w *WrappedDEK) Nonce() ([]byte, error) {
	return base64.StdEncoding.DecodeString(w.NonceBase64)
}

func (w *WrappedDEK) Ciphertext() ([]byte, error) {
	return base64.StdEncoding.DecodeString(w.CiphertextBase64)
}

func (w *WrappedDEK) Tag() ([]byte, error) {
	return base64.StdEncoding.DecodeString(w.TagBase64)
}

func (w *WrappedDEK) AAD() ([]byte, error) {
	if w.AADBase64 == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(w.AADBase64)
}

func NewWrappedDEK(nonce, ciphertext, tag, aad []byte) *WrappedDEK {
	w := &WrappedDEK{
		Algorithm:        WrappedDEKAlgAES256GCM,
		NonceBase64:      base64.StdEncoding.EncodeToString(nonce),
		CiphertextBase64: base64.StdEncoding.EncodeToString(ciphertext),
		TagBase64:        base64.StdEncoding.EncodeToString(tag),
	}
	if len(aad) > 0 {
		w.AADBase64 = base64.StdEncoding.EncodeToString(aad)
	}
	return w
}

func (w *WrappedDEK) IsValid() bool {
	if w == nil {
		return false
	}
	if w.Algorithm != WrappedDEKAlgAES256GCM {
		return false
	}
	nonce, err := w.Nonce()
	if err != nil || len(nonce) != DEKNonceSize {
		return false
	}
	tag, err := w.Tag()
	if err != nil || len(tag) != DEKTagSize {
		return false
	}
	_, err = w.Ciphertext()
	return err == nil
}
