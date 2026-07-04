package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

const (
	ObfuscationSaltSize = 16
)

var (
	ErrObfuscatedDataTooShort = errors.New("obfuscated data too short to contain salt")
)

func deriveXorKeystream(salt []byte, length int) []byte {
	keyStream := make([]byte, 0, length)
	buf := make([]byte, len(salt)+4)
	copy(buf, salt)
	counter := uint32(0)

	for len(keyStream) < length {
		binary.LittleEndian.PutUint32(buf[len(salt):], counter)
		hash := sha256.Sum256(buf)
		remaining := length - len(keyStream)
		if remaining < sha256.Size {
			keyStream = append(keyStream, hash[:remaining]...)
		} else {
			keyStream = append(keyStream, hash[:]...)
		}
		counter++
	}

	return keyStream
}

func ObfuscateManifest(plainData []byte) ([]byte, error) {
	salt := make([]byte, ObfuscationSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	keyStream := deriveXorKeystream(salt, len(plainData))

	xoredData := make([]byte, len(plainData))
	for i := range plainData {
		xoredData[i] = plainData[i] ^ keyStream[i]
	}

	result := make([]byte, ObfuscationSaltSize+len(xoredData))
	copy(result[:ObfuscationSaltSize], salt)
	copy(result[ObfuscationSaltSize:], xoredData)

	return result, nil
}

func DeobfuscateManifest(obfuscatedData []byte) ([]byte, error) {
	if len(obfuscatedData) < ObfuscationSaltSize {
		return nil, ErrObfuscatedDataTooShort
	}

	salt := obfuscatedData[:ObfuscationSaltSize]
	data := obfuscatedData[ObfuscationSaltSize:]

	keyStream := deriveXorKeystream(salt, len(data))

	plainData := make([]byte, len(data))
	for i := range data {
		plainData[i] = data[i] ^ keyStream[i]
	}

	return plainData, nil
}
