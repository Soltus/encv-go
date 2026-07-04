package alistencrypt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	sourceChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-~+"
	OrigPrefix  = "orig_"
)

type mixBase64 struct {
	chars     [65]byte
	decodeMap map[byte]int
}

func newMixBase64(passwd string) *mixBase64 {
	m := &mixBase64{
		decodeMap: make(map[byte]int),
	}

	var secret string
	if len(passwd) == 64 {
		secret = passwd
	} else {
		secret = initKSA(passwd + "mix64")
	}

	for i := 0; i < 64; i++ {
		m.chars[i] = secret[i]
	}
	if len(secret) > 64 {
		m.chars[64] = secret[64]
	} else {
		m.chars[64] = '+'
	}

	for i := 0; i < 65; i++ {
		m.decodeMap[m.chars[i]] = i
	}

	return m
}

func initKSA(passwd string) string {
	key := sha256.Sum256([]byte(passwd))

	sbox := make([]int, len(sourceChars))
	for i := range sbox {
		sbox[i] = i
	}

	K := make([]byte, len(sourceChars))
	for i := 0; i < len(sourceChars); i++ {
		K[i] = key[i%len(key)]
	}

	j := 0
	for i := 0; i < len(sourceChars); i++ {
		j = (j + sbox[i] + int(K[i])) % len(sourceChars)
		sbox[i], sbox[j] = sbox[j], sbox[i]
	}

	sourceBytes := []byte(sourceChars)
	var secret bytes.Buffer
	for _, idx := range sbox {
		secret.WriteByte(sourceBytes[idx])
	}

	return secret.String()
}

func (m *mixBase64) Encode(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var result bytes.Buffer

	i := 0
	for ; i+3 <= len(data); i += 3 {
		b0, b1, b2 := data[i], data[i+1], data[i+2]
		result.WriteByte(m.chars[b0>>2])
		result.WriteByte(m.chars[((b0&3)<<4)|(b1>>4)])
		result.WriteByte(m.chars[((b1&15)<<2)|(b2>>6)])
		result.WriteByte(m.chars[b2&63])
	}

	remaining := len(data) - i
	if remaining == 1 {
		b0 := data[i]
		result.WriteByte(m.chars[b0>>2])
		result.WriteByte(m.chars[(b0&3)<<4])
		result.WriteByte(m.chars[64])
		result.WriteByte(m.chars[64])
	} else if remaining == 2 {
		b0, b1 := data[i], data[i+1]
		result.WriteByte(m.chars[b0>>2])
		result.WriteByte(m.chars[((b0&3)<<4)|(b1>>4)])
		result.WriteByte(m.chars[(b1&15)<<2])
		result.WriteByte(m.chars[64])
	}

	return result.String()
}

func (m *mixBase64) EncodeString(s string) string {
	return m.Encode([]byte(s))
}

func (m *mixBase64) Decode(base64Str string) ([]byte, error) {
	if len(base64Str) == 0 {
		return nil, nil
	}
	if len(base64Str)%4 != 0 {
		return nil, errors.New("invalid base64 string length")
	}

	size := (len(base64Str) / 4) * 3
	paddingChar := string(m.chars[64])
	if strings.HasSuffix(base64Str, paddingChar+paddingChar) {
		size -= 2
	} else if strings.HasSuffix(base64Str, paddingChar) {
		size -= 1
	}

	buffer := make([]byte, size)
	j := 0

	for i := 0; i < len(base64Str); i += 4 {
		enc1, ok1 := m.decodeMap[base64Str[i]]
		enc2, ok2 := m.decodeMap[base64Str[i+1]]
		enc3, ok3 := m.decodeMap[base64Str[i+2]]
		enc4, ok4 := m.decodeMap[base64Str[i+3]]

		if !ok1 || !ok2 || !ok3 || !ok4 {
			return nil, errors.New("invalid character in base64 string")
		}

		buffer[j] = byte((enc1 << 2) | (enc2 >> 4))
		j++

		if enc3 != 64 && j < size {
			buffer[j] = byte(((enc2 & 15) << 4) | (enc3 >> 2))
			j++
		}
		if enc4 != 64 && j < size {
			buffer[j] = byte(((enc3 & 3) << 6) | enc4)
			j++
		}
	}

	return buffer[:j], nil
}

func (m *mixBase64) DecodeString(base64Str string) (string, error) {
	data, err := m.Decode(base64Str)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func getSourceChar(index int) byte {
	if index < 0 || index >= len(sourceChars) {
		return sourceChars[0]
	}
	return sourceChars[index]
}

type crc6 struct {
	table [256]byte
}

func newCRC6() *crc6 {
	c := &crc6{}
	c.generateTable6()
	return c
}

func (c *crc6) generateTable6() {
	for i := 0; i < 256; i++ {
		curr := byte(i)
		for j := 0; j < 8; j++ {
			if (curr & 0x01) != 0 {
				curr = ((curr >> 1) ^ 0x30)
			} else {
				curr = curr >> 1
			}
		}
		c.table[i] = curr
	}
}

func (c *crc6) checksum(data []byte) int {
	val := byte(0)
	for _, b := range data {
		val = c.table[val^b]
	}
	return int(val)
}

var globalCRC6 = newCRC6()

func GetPasswdOutward(password string, encType string) string {
	if len(password) == 32 {
		return password
	}

	salt := "AES-CTR"
	switch encType {
	case "rc4md5":
		salt = "RC4"
	case "chacha20":
		salt = "ChaCha20"
	}
	key := pbkdf2.Key([]byte(password), []byte(salt), 1000, 16, sha256.New)
	return hex.EncodeToString(key)
}

func EncodeName(plainName string, password string, encType string) string {
	passwdOutward := GetPasswdOutward(password, encType)
	mix64 := newMixBase64(passwdOutward)

	encodedName := mix64.EncodeString(plainName)

	checkData := encodedName + passwdOutward
	crc6Bit := globalCRC6.checksum([]byte(checkData))
	crc6Check := getSourceChar(crc6Bit)

	return encodedName + string(crc6Check)
}

func DecodeName(encodedName string, password string, encType string) string {
	if len(encodedName) < 2 {
		return ""
	}

	crc6Check := encodedName[len(encodedName)-1]
	passwdOutward := GetPasswdOutward(password, encType)
	mix64 := newMixBase64(passwdOutward)

	subEncName := encodedName[:len(encodedName)-1]

	checkData := subEncName + passwdOutward
	crc6Bit := globalCRC6.checksum([]byte(checkData))
	if getSourceChar(crc6Bit) != crc6Check {
		return ""
	}

	decoded, err := mix64.DecodeString(subEncName)
	if err != nil {
		return ""
	}

	return decoded
}

func ConvertShowName(encodedName string, password string, encType string) string {
	decoded, err := url.PathUnescape(encodedName)
	if err != nil {
		decoded = encodedName
	}

	fileName := path.Base(decoded)
	ext := path.Ext(fileName)
	encName := strings.TrimSuffix(fileName, ext)

	showName := DecodeName(encName, password, encType)
	if showName == "" {
		return OrigPrefix + fileName
	}

	return showName
}

// ConvertRealName 把用户可见的原文件名转换为上传用的容器文件名。
// 范式：EncodeName(完整原名) + 容器后缀（如 .bin）。
// 本函数把 ext 从原名剥出再编码（而非编码整个含 ext 的名），保证容器文件名 = "encoded<ext>.bin"。
// 这样既不重复 ext，又让 EncodeName 只处理 baseName 字符。
func ConvertRealName(showName string, password string, encType string) string {
	fileName := path.Base(showName)

	if strings.HasPrefix(fileName, OrigPrefix) {
		return strings.TrimPrefix(fileName, OrigPrefix)
	}

	decoded, err := url.PathUnescape(fileName)
	if err != nil {
		decoded = fileName
	}

	ext := path.Ext(decoded)
	baseName := strings.TrimSuffix(decoded, ext)

	encName := EncodeName(baseName, password, encType)

	return encName + ext
}
