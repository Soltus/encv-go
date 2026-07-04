package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"strings"
	"golang.org/x/crypto/scrypt"
)

func main() {
	key, _ := scrypt.Key([]byte("encv-agent-key-v1"), []byte("encv-mobile-salt-2024"), 16384, 8, 1, 32)
	stored := "enc:283pjDd2NjzFu5xDWMtYoA==:Zj8S01DeuFWuwiCjeL+0N6b+bc22OTqOz6kGOQKqrPVyNy5mJKmvkFpeltqeQWk6U/GlKHkAhe6KNz0QmktCTQ=="
	raw := stored[4:]

	// 按 ':' 分割
	parts := strings.SplitN(raw, ":", 2)
	fmt.Printf("parts count=%d\n", len(parts))
	fmt.Printf("parts[0]=%s parts[1]=%s\n", parts[0], parts[1][:20]+"...")

	iv, _ := base64.StdEncoding.DecodeString(parts[0])
	ct, _ := base64.StdEncoding.DecodeString(parts[1])
	fmt.Printf("iv len=%d ct len=%d\n", len(iv), len(ct))

	block, _ := aes.NewCipher(key)
	stream := cipher.NewCBCDecrypter(block, iv)
	stream.CryptBlocks(ct, ct)

	pad := int(ct[len(ct)-1])
	fmt.Printf("padding=%d decrypted='%s'\n", pad, string(ct[:len(ct)-pad]))
}
