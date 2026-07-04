//go:build android

package service

import (
	"context"
	"fmt"
)

func Preview(ctx context.Context, inputPath string) error {
	return fmt.Errorf("preview not available on mobile; use MpvPlayerModule (Kotlin/MPVLib JNI) instead")
}
