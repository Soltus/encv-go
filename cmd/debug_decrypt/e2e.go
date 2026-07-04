package main

import (
	"fmt"
	"os"

	aenc "github.com/Soltus/encv-go/internal/v2/plugins/alistencrypt"
)

func main() {
	password := "8682268"
	filePath := "/storage/emulated/0/hyYGPCwJPQ3+xrdAvfnn2.bin"
	outputDir := "/tmp"

	fmt.Println("=== Using Project's DecryptFile ===")
	outputPath, err := aenc.DecryptFile(filePath, outputDir, password, "aes-ctr")
	if err != nil {
		fmt.Printf("❌ DecryptFile error: %v\n", err)
		if de, ok := err.(*aenc.DecryptionError); ok {
			fmt.Printf("   Reason: %s\n", de.Reason)
			if de.Err != nil {
				fmt.Printf("   Inner: %v\n", de.Err)
			}
		}
		return
	}

	fi, statErr := os.Stat(outputPath)
	if statErr != nil {
		fmt.Printf("⚠️ Output file not found at %s\n", outputPath)
		files, _ := os.ReadDir(outputDir)
		for _, f := range files {
			fmt.Printf("  Found: %s (%d bytes)\n", f.Name(), func() int64 {
				info, _ := f.Info()
				if info != nil {
					return info.Size()
				}
				return 0
			}())
		}
		return
	}

	fmt.Printf("✅ Decrypted: %s (%d bytes)\n", fi.Name(), fi.Size())

	fmt.Println("\n=== Filename Decode Test ===")
	showName := aenc.ConvertShowName("hyYGPCwJPQ3+xrdAvfnn2.bin", password, "aes-ctr")
	fmt.Printf("ConvertShowName('hyYGPCwJPQ3+xrdAvfnn2.bin') = %q\n", showName)

	realName := aenc.ConvertRealName("CAD放样.mp4", password, "aes-ctr")
	fmt.Printf("ConvertRealName('CAD放样.mp4') = %q\n", realName)
}
