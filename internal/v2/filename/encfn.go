package filename

import (
	"bytes"
	"strconv"
)

type FNConfig struct {
	Password   []byte
	Salt       []byte
	Charsets   []FNCharset
	Deconfuse  bool
	Rounds     int
	Structured bool
}

func (c *FNConfig) Validate() error {
	if c.Rounds < 1 {
		c.Rounds = 8
	}
	if len(c.Charsets) == 0 {
		c.Charsets = []FNCharset{FNAlnum}
	}
	return nil
}

func (c *FNConfig) Encode(plaintext []byte) (string, error) {
	if len(plaintext) == 0 {
		return "", ErrFNEmptyInput
	}
	c.Validate()

	table, err := BuildCharsetTable(c.Charsets, c.Deconfuse)
	if err != nil {
		return "", err
	}

	keys := DeriveKeys(c.Password, c.Salt, c.Rounds)
	sbox := GenerateSBox(keys.SboxSeed)

	padded := make([]byte, len(plaintext))
	copy(padded, plaintext)
	originalLen := len(padded)
	if len(padded)%2 != 0 {
		padded = append(padded, 0)
	}

	encrypted := FeistelEncrypt(padded, sbox, keys.RoundKeys)

	if c.Structured {
		return encodeStructuredWithLen(encrypted, table, originalLen)
	}
	return encodeCompactWithLen(encrypted, table, originalLen), nil
}

func (c *FNConfig) Decode(encoded string) ([]byte, error) {
	if len(encoded) == 0 {
		return nil, ErrFNEmptyInput
	}
	c.Validate()

	table, err := BuildCharsetTable(c.Charsets, c.Deconfuse)
	if err != nil {
		return nil, err
	}

	var encrypted []byte
	var originalLen int
	if c.Structured {
		encrypted, originalLen, err = decodeStructuredWithLen(encoded, table)
		if err != nil {
			return nil, err
		}
	} else {
		encrypted, originalLen, err = decodeCompactWithLen(encoded, table)
		if err != nil {
			return nil, err
		}
	}

	keys := DeriveKeys(c.Password, c.Salt, c.Rounds)
	sbox := GenerateSBox(keys.SboxSeed)

	decrypted := FeistelDecrypt(encrypted, sbox, keys.RoundKeys)
	if originalLen > 0 && originalLen < len(decrypted) {
		decrypted = decrypted[:originalLen]
	}
	return decrypted, nil
}

func encodeStructured(data []byte, table []rune) (string, error) {
	body := EncodeToCharset(data, table)
	tableHash := crc8IEEE(runesToBytes(table))
	crcData := crc8IEEE(data)

	var buf bytes.Buffer
	buf.WriteRune('S')
	buf.WriteString(strconv.FormatUint(uint64(len(body)), 10))
	buf.WriteRune(':')
	buf.WriteString(body)
	buf.WriteRune(':')
	buf.WriteString(strconv.FormatUint(uint64(tableHash), 10))
	buf.WriteRune(',')
	buf.WriteString(strconv.FormatUint(uint64(crcData), 10))

	return buf.String(), nil
}

func decodeStructured(s string, table []rune) ([]byte, error) {
	if len(s) < 2 || s[0] != 'S' {
		return nil, ErrFNInvalidFormat
	}

	colon1 := -1
	for i := 1; i < len(s); i++ {
		if s[i] == ':' {
			colon1 = i
			break
		}
	}
	if colon1 <= 1 {
		return nil, ErrFNInvalidFormat
	}

	bodyLen, err := strconv.ParseUint(s[1:colon1], 10, 64)
	if err != nil {
		return nil, ErrFNInvalidFormat
	}

	rest := s[colon1+1:]
	lastColon := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == ':' {
			lastColon = i
			break
		}
	}
	if lastColon < 0 {
		return nil, ErrFNInvalidFormat
	}

	body := rest[:lastColon]
	checkPart := rest[lastColon+1:]

	commaIdx := -1
	for i := 0; i < len(checkPart); i++ {
		if checkPart[i] == ',' {
			commaIdx = i
			break
		}
	}
	if commaIdx < 0 {
		return nil, ErrFNInvalidFormat
	}

	expectedTableCRC, err := strconv.ParseUint(checkPart[:commaIdx], 10, 64)
	if err != nil {
		return nil, ErrFNInvalidFormat
	}
	expectedDataCRC, err := strconv.ParseUint(checkPart[commaIdx+1:], 10, 64)
	if err != nil {
		return nil, ErrFNInvalidFormat
	}

	actualTableCRC := uint64(crc8IEEE(runesToBytes(table)))
	if expectedTableCRC != actualTableCRC {
		return nil, ErrFNChecksumMismatch
	}

	if uint64(bodyLen) != uint64(len(body)) {
		return nil, ErrFNInvalidFormat
	}

	data, err := DecodeFromCharset(body, table)
	if err != nil {
		return nil, err
	}

	actualDataCRC := uint64(crc8IEEE(data))
	if expectedDataCRC != actualDataCRC {
		return nil, ErrFNChecksumMismatch
	}

	return data, nil
}

func crc8IEEE(data []byte) byte {
	var crc byte = 0
	for _, b := range data {
		crc ^= b
		for j := 0; j < 8; j++ {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ 0x07
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func runesToBytes(runes []rune) []byte {
	buf := make([]byte, len(runes)*4)
	idx := 0
	for _, r := range runes {
		idx += encodeRuneToUTF8(buf[idx:], r)
	}
	return buf[:idx]
}

func encodeRuneToUTF8(buf []byte, r rune) int {
	if r <= 0x7F {
		buf[0] = byte(r)
		return 1
	} else if r <= 0x7FF {
		buf[0] = byte(0xC0 | (r >> 6))
		buf[1] = byte(0x80 | (r & 0x3F))
		return 2
	} else if r <= 0xFFFF {
		buf[0] = byte(0xE0 | (r >> 12))
		buf[1] = byte(0x80 | ((r >> 6) & 0x3F))
		buf[2] = byte(0x80 | (r & 0x3F))
		return 3
	} else {
		buf[0] = byte(0xF0 | (r >> 18))
		buf[1] = byte(0x80 | ((r >> 12) & 0x3F))
		buf[2] = byte(0x80 | ((r >> 6) & 0x3F))
		buf[3] = byte(0x80 | (r & 0x3F))
		return 4
	}
}

const compactLenPrefixChars = 2

func encodeCompactWithLen(data []byte, table []rune, originalLen int) string {
	base := uint64(len(table))
	body := EncodeToCharset(data, table)

	lenDigits := make([]rune, compactLenPrefixChars)
	val := uint64(originalLen)
	for i := 0; i < compactLenPrefixChars; i++ {
		lenDigits[compactLenPrefixChars-1-i] = table[val%base]
		val /= base
	}

	prefix := string(lenDigits)
	return prefix + body
}

func decodeCompactWithLen(encoded string, table []rune) ([]byte, int, error) {
	base := uint64(len(table))

	runes := []rune(encoded)
	if len(runes) < compactLenPrefixChars {
		return nil, 0, ErrFNInvalidFormat
	}

	var originalLen uint64
	for i := 0; i < compactLenPrefixChars; i++ {
		ch := runes[i]
		idx := -1
		for j, r := range table {
			if r == ch {
				idx = j
				break
			}
		}
		if idx < 0 {
			return nil, 0, ErrFNCharsetMismatch
		}
		originalLen = originalLen*base + uint64(idx)
	}

	body := string(runes[compactLenPrefixChars:])
	data, err := DecodeFromCharset(body, table)
	if err != nil {
		return nil, 0, err
	}

	return data, int(originalLen), nil
}

func encodeStructuredWithLen(data []byte, table []rune, originalLen int) (string, error) {
	body := EncodeToCharset(data, table)
	tableHash := crc8IEEE(runesToBytes(table))
	crcData := crc8IEEE(data)

	var buf bytes.Buffer
	buf.WriteRune('S')
	buf.WriteString(strconv.FormatUint(uint64(len(body)), 10))
	buf.WriteRune(':')
	buf.WriteString(body)
	buf.WriteRune(':')
	buf.WriteString(strconv.FormatUint(uint64(tableHash), 10))
	buf.WriteRune(',')
	buf.WriteString(strconv.FormatUint(uint64(crcData), 10))
	buf.WriteRune(',')
	buf.WriteString(strconv.FormatUint(uint64(originalLen), 10))

	return buf.String(), nil
}

func decodeStructuredWithLen(s string, table []rune) ([]byte, int, error) {
	if len(s) < 2 || s[0] != 'S' {
		return nil, 0, ErrFNInvalidFormat
	}

	colon1 := -1
	for i := 1; i < len(s); i++ {
		if s[i] == ':' {
			colon1 = i
			break
		}
	}
	if colon1 <= 1 {
		return nil, 0, ErrFNInvalidFormat
	}

	bodyLen, err := strconv.ParseUint(s[1:colon1], 10, 64)
	if err != nil {
		return nil, 0, ErrFNInvalidFormat
	}

	rest := s[colon1+1:]
	lastColon := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == ':' {
			lastColon = i
			break
		}
	}
	if lastColon < 0 {
		return nil, 0, ErrFNInvalidFormat
	}

	body := rest[:lastColon]
	checkPart := rest[lastColon+1:]

	commaIdx := -1
	commaIdx2 := -1
	for i := 0; i < len(checkPart); i++ {
		if checkPart[i] == ',' {
			if commaIdx < 0 {
				commaIdx = i
			} else if commaIdx2 < 0 {
				commaIdx2 = i
			}
		}
	}
	if commaIdx < 0 || commaIdx2 < 0 {
		data, err := decodeStructured(s, table)
		return data, 0, err
	}

	expectedTableCRC, err := strconv.ParseUint(checkPart[:commaIdx], 10, 64)
	if err != nil {
		return nil, 0, ErrFNInvalidFormat
	}
	expectedDataCRC, err := strconv.ParseUint(checkPart[commaIdx+1:commaIdx2], 10, 64)
	if err != nil {
		return nil, 0, ErrFNInvalidFormat
	}
	originalLen, err := strconv.ParseUint(checkPart[commaIdx2+1:], 10, 64)
	if err != nil {
		return nil, 0, ErrFNInvalidFormat
	}

	actualTableCRC := uint64(crc8IEEE(runesToBytes(table)))
	if expectedTableCRC != actualTableCRC {
		return nil, int(originalLen), ErrFNChecksumMismatch
	}

	if uint64(bodyLen) != uint64(len(body)) {
		return nil, int(originalLen), ErrFNInvalidFormat
	}

	data, err := DecodeFromCharset(body, table)
	if err != nil {
		return nil, int(originalLen), err
	}

	actualDataCRC := uint64(crc8IEEE(data))
	if expectedDataCRC != actualDataCRC {
		return nil, int(originalLen), ErrFNChecksumMismatch
	}

	return data, int(originalLen), nil
}
