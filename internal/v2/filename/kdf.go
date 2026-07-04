package filename

import (
	"crypto/hmac"
	"crypto/sha256"
)

type DerivedKeys struct {
	SboxSeed  []byte
	RoundKeys [][]byte
}

func DeriveKeys(password, salt []byte, rounds int) *DerivedKeys {
	master := sha256.Sum256(append(password, salt...))

	sboxSeed := hkdfExpand(master[:], []byte("enc-fn.sbox"), 256)

	totalKeyBytes := rounds * 16
	keyMaterial := hkdfExpand(master[:], []byte("enc-fn.rounds"), totalKeyBytes)

	roundKeys := make([][]byte, rounds)
	for i := 0; i < rounds; i++ {
		roundKeys[i] = make([]byte, 16)
		copy(roundKeys[i], keyMaterial[i*16:(i+1)*16])
	}

	return &DerivedKeys{
		SboxSeed:  sboxSeed,
		RoundKeys: roundKeys,
	}
}

func hkdfExpand(prk, info []byte, length int) []byte {
	hashLen := sha256.Size
	n := (length + hashLen - 1) / hashLen
	if n > 255 {
		n = 255
	}

	result := make([]byte, 0, n*hashLen)
	prev := make([]byte, 0)
	for i := 1; i <= n; i++ {
		mac := hmac.New(sha256.New, prk)
		mac.Write(prev)
		mac.Write(info)
		mac.Write([]byte{byte(i)})
		prev = mac.Sum(nil)
		result = append(result, prev...)
	}
	return result[:length]
}
