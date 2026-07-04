//go:build android

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	buildInfoMu   sync.RWMutex
	buildInfoData map[string]interface{}
	buildInfoErr  error
	buildInfoInit bool
)

func GetBuildInfo() (map[string]interface{}, error) {
	buildInfoMu.RLock()
	if buildInfoInit {
		data, err := buildInfoData, buildInfoErr
		buildInfoMu.RUnlock()
		return data, err
	}
	buildInfoMu.RUnlock()

	buildInfoMu.Lock()
	defer buildInfoMu.Unlock()

	if buildInfoInit {
		return buildInfoData, buildInfoErr
	}

	var lastErr error
	for _, dir := range []string{
		os.Getenv("ENCV_LIB_DIR"),
		os.Getenv("HOME"),
	} {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, "build-info.json")
		data, err := os.ReadFile(path)
		if err != nil {
			lastErr = fmt.Errorf("failed to read build-info.json from %s: %w", dir, err)
			continue
		}
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			lastErr = fmt.Errorf("failed to parse build-info.json from %s: %w", dir, err)
			continue
		}
		buildInfoData = result
		buildInfoInit = true
		return buildInfoData, nil
	}

	if lastErr != nil {
		buildInfoErr = lastErr
	} else {
		buildInfoErr = fmt.Errorf("ENCV_LIB_DIR and HOME not set")
	}
	buildInfoInit = true
	return nil, buildInfoErr
}
