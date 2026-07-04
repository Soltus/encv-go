package alistencrypt

import (
	"fmt"
	"sync"
)

var (
	registries   = make(map[string]CipherFactory)
	registriesMu sync.RWMutex
)

func init() {
}

// Register registers a cipher factory for the given encType (extension point)
// Warning: the main application only includes AES-128-CTR; other algorithms must be introduced via extension packages
func Register(encType string, factory CipherFactory) {
	registriesMu.Lock()
	defer registriesMu.Unlock()
	registries[encType] = factory
}

// Create creates a Cipher instance based on encType
// Returns ErrExtensionRequired if encType is not registered
func Create(password string, encType string, fileSize int64) (Cipher, error) {
	registriesMu.RLock()
	factory, ok := registries[encType]
	registriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: enc_type=%q is not supported (only 'aesctr' is built-in)", ErrExtensionRequired, encType)
	}
	return factory(password, fileSize)
}
