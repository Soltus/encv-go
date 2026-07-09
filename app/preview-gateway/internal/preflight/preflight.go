package preflight

import (
	"fmt"
	"log"
	"os"

	"preview-gateway/internal/paths"
)

func Run(p *paths.Paths) error {
	log.Println("[preflight] Running preflight checks...")

	if err := os.MkdirAll(p.MobileDataDir, 0755); err != nil {
		return fmt.Errorf("failed to create mobile data dir %s: %w", p.MobileDataDir, err)
	}
	log.Printf("[preflight] Mobile data dir: %s (ok)", p.MobileDataDir)

	if _, err := os.Stat(p.MobileDir); os.IsNotExist(err) {
		return fmt.Errorf("mobile dir not found: %s", p.MobileDir)
	}
	log.Printf("[preflight] Mobile dir: %s (ok)", p.MobileDir)

	if _, err := os.Stat(p.AirBin); os.IsNotExist(err) {
		log.Printf("[preflight] Warning: air binary not found at %s", p.AirBin)
	} else {
		log.Printf("[preflight] Air binary: %s (ok)", p.AirBin)
	}

	log.Println("[preflight] All checks passed")
	return nil
}
