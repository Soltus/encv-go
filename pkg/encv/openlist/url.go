package openlist

import (
	encvOpenlist "github.com/Soltus/encv-go/internal/openlist"
)

func NewOpenListURLResolver(host, token, originalContainerPath string) *encvOpenlist.OpenListURLResolver {
	return encvOpenlist.NewOpenListURLResolver(host, token, originalContainerPath)
}
