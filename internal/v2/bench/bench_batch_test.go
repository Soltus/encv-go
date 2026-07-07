package bench_test

import (
	"fmt"
	"testing"

	v2crypto "github.com/Soltus/encv-go/internal/v2/crypto"
)

func BenchmarkBatchKEKDerivation(b *testing.B) {
	password := "batch-test-password-12345"
	numFiles := 1000

	b.Run(fmt.Sprintf("%d_files_same_password", numFiles), func(b *testing.B) {
		for b.Loop() {
			for i := 0; i < numFiles; i++ {
				salt := make([]byte, 32)
				for j := range salt {
					salt[j] = byte(i ^ j)
				}
				_ = v2crypto.DeriveKEK(password, salt)
			}
		}
	})

	b.Run(fmt.Sprintf("%d_files_different_passwords", numFiles), func(b *testing.B) {
		for b.Loop() {
			for i := 0; i < numFiles; i++ {
				salt := make([]byte, 32)
				for j := range salt {
					salt[j] = byte(i ^ j)
				}
				pw := fmt.Sprintf("password-%d", i)
				_ = v2crypto.DeriveKEK(pw, salt)
			}
		}
	})
}
