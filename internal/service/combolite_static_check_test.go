package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestComboLiteStaticCheck_ZeroReflection(t *testing.T) {
	ktDir := filepath.Join("..", "..", "app", "encv-mobile", "android", "app", "src", "main", "java", "com", "encvgo")

	files, err := os.ReadDir(ktDir)
	if os.IsNotExist(err) {
		t.Skip("Kotlin source dir not found, skipping ComboLite static check")
		return
	}
	if err != nil {
		t.Fatalf("Failed to read Kotlin dir: %v", err)
	}

	reflectionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`Class\.forName\(\s*["']com\.combo`),
		regexp.MustCompile(`\.getMethod\(\s*["']getInstance`),
		regexp.MustCompile(`\.invoke\(\s*\w+\s*,\s*["']installPlugin`),
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".kt") {
			continue
		}
		content, err := readFile(filepath.Join(ktDir, f.Name()))
		if err != nil {
			continue
		}

		for _, pat := range reflectionPatterns {
			if pat.MatchString(content) {
				lineNum := findLineNumber(content, pat.String())
				t.Errorf("REFLECTION FOUND in %s:%d: %s (violates combolite.md rule 1.1)",
					f.Name(), lineNum, pat.String())
			}
		}
	}

	if t.Failed() {
		t.Error("ComboLite static check FAILED: reflection code detected in .kt files")
	}
}

func TestComboLiteStaticCheck_InstallerManagerPath(t *testing.T) {
	ktDir := filepath.Join("..", "..", "app", "encv-mobile", "android", "app", "src", "main", "java", "com", "encvgo")

	files, _ := os.ReadDir(ktDir)

	wrongPattern := regexp.MustCompile(`PluginManager\.installPlugin\(`)
	correctPattern := regexp.MustCompile(`PluginManager\.installerManager\.installPlugin\(`)

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".kt") {
			continue
		}
		content, _ := readFile(filepath.Join(ktDir, f.Name()))

		if wrongPattern.MatchString(content) && !correctPattern.MatchString(content) {
			lineNum := findLineNumber(content, "PluginManager.installPlugin")
			t.Errorf("WRONG installPlugin path in %s:%d: uses PluginManager.installPlugin() instead of PluginManager.installerManager.installPlugin()",
				f.Name(), lineNum)
		}
	}
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func findLineNumber(s string, pattern string) int {
	lines := strings.Split(s, "\n")
	re := regexp.MustCompile(pattern)
	for i, line := range lines {
		if re.MatchString(line) {
			return i + 1
		}
	}
	return 0
}
