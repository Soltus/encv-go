package alistencrypt

// Cipher defines the extensible cipher abstraction
type Cipher interface {
	SetPosition(position int64) error
	Encrypt(data []byte)
	Decrypt(data []byte)
	Algorithm() string
	BlockSize() int
}

// CipherFactory creates Cipher instances
type CipherFactory func(password string, fileSize int64) (Cipher, error)
