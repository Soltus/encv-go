package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
)

const (
	PasswordHintSize     = 16
	PasswordCheckMessage = "ENCV_PASSWORD_CHECK"
)

func CalculatePasswordHint(password string, salt []byte) ([16]byte, error) {
	key := GenerateKey(password, salt, KeySize_v2)

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(PasswordCheckMessage))

	var hint [16]byte
	copy(hint[:], mac.Sum(nil)[:PasswordHintSize])

	return hint, nil
}

func VerifyPasswordHint(hint [16]byte, password string, salt []byte) bool {
	calculated, err := CalculatePasswordHint(password, salt)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(calculated[:], hint[:]) == 1
}
