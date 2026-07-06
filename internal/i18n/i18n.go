package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed locales/zh-CN.json
var zhCNJSON []byte

//go:embed locales/en.json
var enJSON []byte

type Dictionary map[string]string

var (
	once     sync.Once
	dicts    map[string]Dictionary
	current  string
	loadErr  error
)

func initDicts() {
	dicts = make(map[string]Dictionary)

	var zhDict Dictionary
	if err := json.Unmarshal(zhCNJSON, &zhDict); err != nil {
		loadErr = fmt.Errorf("failed to load zh-CN dictionary: %w", err)
		return
	}
	dicts["zh-CN"] = zhDict

	var enDict Dictionary
	if err := json.Unmarshal(enJSON, &enDict); err != nil {
		loadErr = fmt.Errorf("failed to load en dictionary: %w", err)
		return
	}
	dicts["en"] = enDict

	current = detectLocale()
}

func detectLocale() string {
	envVars := []string{"ENCV_LANG", "LANG", "LC_ALL", "LC_MESSAGES"}
	for _, v := range envVars {
		if val := os.Getenv(v); val != "" {
			normalized := normalizeLocale(val)
			if _, ok := dicts[normalized]; ok {
				return normalized
			}
		}
	}
	return "zh-CN"
}

func normalizeLocale(l string) string {
	l = strings.Split(l, ".")[0]
	l = strings.Split(l, "@")[0]
	l = strings.ToLower(l)
	switch l {
	case "zh-cn", "zh_cn", "zh", "zh-hans":
		return "zh-CN"
	case "en-us", "en_us", "en":
		return "en"
	default:
		if strings.HasPrefix(l, "zh") {
			return "zh-CN"
		}
		return "en"
	}
}

func SetLocale(locale string) error {
	once.Do(initDicts)
	if loadErr != nil {
		return loadErr
	}
	normalized := normalizeLocale(locale)
	if _, ok := dicts[normalized]; !ok {
		return fmt.Errorf("unsupported locale: %s (normalized: %s)", locale, normalized)
	}
	current = normalized
	return nil
}

func GetLocale() string {
	once.Do(initDicts)
	return current
}

func T(key string) string {
	once.Do(initDicts)
	if loadErr != nil {
		return key
	}
	dict, ok := dicts[current]
	if !ok {
		return key
	}
	if val, ok := dict[key]; ok {
		return val
	}
	return key
}

func TWith(key string, vars map[string]string) string {
	val := T(key)
	for k, v := range vars {
		val = strings.ReplaceAll(val, "{{"+k+"}}", v)
	}
	return val
}

func AvailableLocales() []string {
	once.Do(initDicts)
	locales := make([]string, 0, len(dicts))
	for l := range dicts {
		locales = append(locales, l)
	}
	return locales
}

func LoadFromDir(dir string) error {
	once.Do(func() {})
	newDicts := make(map[string]Dictionary)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read locale dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		locale := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", entry.Name(), err)
		}
		var dict Dictionary
		if err := json.Unmarshal(data, &dict); err != nil {
			return fmt.Errorf("failed to parse %s: %w", entry.Name(), err)
		}
		newDicts[locale] = dict
	}

	if len(newDicts) == 0 {
		return fmt.Errorf("no locale files found in %s", dir)
	}

	dicts = newDicts
	if _, ok := dicts[current]; !ok {
		current = "zh-CN"
	}
	return nil
}
