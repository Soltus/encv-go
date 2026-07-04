//go:build !android

package utils

import "fmt"

func GetBuildInfo() (map[string]interface{}, error) {
	return nil, fmt.Errorf("build info not available on this platform")
}
