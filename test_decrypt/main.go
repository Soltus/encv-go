package main
import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/scrypt"
)
func main() {
	key, _ := scrypt.Key([]byte("encv-agent-key-v1"), []byte("encv-mobile-salt-2024"), 16384, 8, 1, 32)
	stored := "enc:283pjDd2NjzFu5xDWMtYoA==:Zj8S01DeuFWuwiCjeL+0N6b+bc22OTqOz6kGOQKqrPVyNy5mJKmvkFpeltqeQWk6U/GlKHkAhe6KNz0QmktCTQ=="
	raw := stored[4:]
	data, _ := base64.StdEncoding.DecodeString(raw)
	fmt.Printf("data len=%d\n", len(data))
	iv := data[:aes.BlockSize]
	ct := data[aes.BlockSize:]
	block, _ := aes.NewCipher(key)
	stream := cipher.NewCBCDecrypter(block, iv)
	stream.CryptBlocks(ct, ct)
	pad := int(ct[len(ct)-1])
	fmt.Printf("padding=%d decrypted='%s'\n", pad, string(ct[:len(ct)-pad]))
}
