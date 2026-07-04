package alistencrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"golang.org/x/crypto/pbkdf2"
)

type aesCtr struct {
	password string
	fileSize int64
	key      []byte
	iv       []byte
	sourceIv []byte
	block    cipher.Block
	stream   cipher.Stream
	position int64
}

func NewAesCtr(password string, fileSize int64) (Cipher, error) {
	a := &aesCtr{
		password: password,
		fileSize: fileSize,
	}

	passwdOutward := password
	if len(password) != 32 {
		key := pbkdf2.Key([]byte(password), []byte("AES-CTR"), 1000, 16, sha256.New)
		passwdOutward = hex.EncodeToString(key)
	}
	passwdSalt := passwdOutward + strconv.FormatInt(fileSize, 10)

	keyHash := md5.Sum([]byte(passwdSalt))
	a.key = keyHash[:]

	ivHash := md5.Sum([]byte(strconv.FormatInt(fileSize, 10)))
	a.iv = make([]byte, 16)
	copy(a.iv, ivHash[:])

	a.sourceIv = make([]byte, 16)
	copy(a.sourceIv, a.iv)

	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	a.block = block
	a.stream = cipher.NewCTR(block, a.iv)

	return a, nil
}

func (a *aesCtr) SetPosition(position int64) error {
	if position < 0 {
		return fmt.Errorf("position cannot be negative")
	}

	a.iv = make([]byte, len(a.sourceIv))
	copy(a.iv, a.sourceIv)

	blockCount := position / 16

	a.incrementIV(blockCount)

	a.stream = cipher.NewCTR(a.block, a.iv)

	offset := position % 16
	if offset > 0 {
		discard := make([]byte, offset)
		a.stream.XORKeyStream(discard, discard)
	}

	a.position = position
	return nil
}

func (a *aesCtr) incrementIV(increment int64) {
	const maxUint32 = uint64(0xffffffff)
	inc := uint64(increment)
	incrementBig := int64(inc / maxUint32)
	incrementLittle := int64(inc%maxUint32) - incrementBig

	overflow := int64(0)
	for idx := 0; idx < 4; idx++ {
		offset := 12 - idx*4
		num := int64(uint32(a.iv[offset])<<24 | uint32(a.iv[offset+1])<<16 |
			uint32(a.iv[offset+2])<<8 | uint32(a.iv[offset+3]))
		incPart := overflow
		if idx == 0 {
			incPart += incrementLittle
		}
		if idx == 1 {
			incPart += incrementBig
		}
		num += incPart
		numBig := num / int64(maxUint32)
		numLittle := num%int64(maxUint32) - numBig
		overflow = numBig
		v := uint32(numLittle)
		a.iv[offset] = byte(v >> 24)
		a.iv[offset+1] = byte(v >> 16)
		a.iv[offset+2] = byte(v >> 8)
		a.iv[offset+3] = byte(v)
	}
}

func (a *aesCtr) Encrypt(data []byte) {
	a.stream.XORKeyStream(data, data)
	a.position += int64(len(data))
}

func (a *aesCtr) Decrypt(data []byte) {
	a.Encrypt(data)
}

func (a *aesCtr) Algorithm() string {
	return "AES-128-CTR"
}

func (a *aesCtr) BlockSize() int {
	return 16
}

func init() {
	Register("aesctr", func(password string, fileSize int64) (Cipher, error) {
		return NewAesCtr(password, fileSize)
	})
}
