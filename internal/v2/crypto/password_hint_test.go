package crypto

import (
	"testing"
)

func TestCalculatePasswordHint_SameInputSameOutput(t *testing.T) {
	password := "test-password-123"
	salt := []byte("fixed-salt-for-testing")

	hint1, err := CalculatePasswordHint(password, salt)
	if err != nil {
		t.Fatalf("CalculatePasswordHint failed: %v", err)
	}

	hint2, err := CalculatePasswordHint(password, salt)
	if err != nil {
		t.Fatalf("CalculatePasswordHint failed: %v", err)
	}

	if hint1 != hint2 {
		t.Error("same password+salt should produce identical hints")
	}
}

func TestCalculatePasswordHint_DifferentPasswords(t *testing.T) {
	salt := []byte("fixed-salt-for-testing")

	hint1, err := CalculatePasswordHint("password-a", salt)
	if err != nil {
		t.Fatalf("CalculatePasswordHint failed: %v", err)
	}

	hint2, err := CalculatePasswordHint("password-b", salt)
	if err != nil {
		t.Fatalf("CalculatePasswordHint failed: %v", err)
	}

	if hint1 == hint2 {
		t.Error("different passwords should produce different hints")
	}
}

func TestCalculatePasswordHint_DifferentSalts(t *testing.T) {
	password := "test-password"

	hint1, err := CalculatePasswordHint(password, []byte("salt-alpha"))
	if err != nil {
		t.Fatalf("CalculatePasswordHint failed: %v", err)
	}

	hint2, err := CalculatePasswordHint(password, []byte("salt-beta"))
	if err != nil {
		t.Fatalf("CalculatePasswordHint failed: %v", err)
	}

	if hint1 == hint2 {
		t.Error("different salts should produce different hints")
	}
}

func TestVerifyPasswordHint_CorrectPassword(t *testing.T) {
	password := "my-secret-password"
	salt := []byte("test-salt-value")

	hint, err := CalculatePasswordHint(password, salt)
	if err != nil {
		t.Fatalf("CalculatePasswordHint failed: %v", err)
	}

	if !VerifyPasswordHint(hint, password, salt) {
		t.Error("VerifyPasswordHint should return true for correct password")
	}
}

func TestVerifyPasswordHint_WrongPassword(t *testing.T) {
	correctPassword := "my-secret-password"
	wrongPassword := "wrong-password"
	salt := []byte("test-salt-value")

	hint, err := CalculatePasswordHint(correctPassword, salt)
	if err != nil {
		t.Fatalf("CalculatePasswordHint failed: %v", err)
	}

	if VerifyPasswordHint(hint, wrongPassword, salt) {
		t.Error("VerifyPasswordHint should return false for wrong password")
	}
}

func TestCalculatePasswordHint_EmptyPassword(t *testing.T) {
	salt := []byte("test-salt")

	hint, err := CalculatePasswordHint("", salt)
	if err != nil {
		t.Fatalf("CalculatePasswordHint with empty password should not fail: %v", err)
	}

	var zeroHint [16]byte
	if hint == zeroHint {
		t.Error("empty password should still produce a non-zero hint")
	}
}
