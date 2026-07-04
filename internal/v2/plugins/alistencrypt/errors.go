package alistencrypt

import "fmt"

var (
	ErrExtensionRequired = fmt.Errorf("this encryption algorithm requires an extension package; only AES-128-CTR is built-in")
	ErrInvalidPassword   = fmt.Errorf("invalid password or password mismatch")
	ErrInvalidFormat     = fmt.Errorf("invalid file format: not a valid alist-encrypt file")
)

type DecryptionError struct {
	Reason string
	Err    error
}

func (e *DecryptionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Reason, e.Err)
	}
	return e.Reason
}

func (e *DecryptionError) Unwrap() error { return e.Err }
