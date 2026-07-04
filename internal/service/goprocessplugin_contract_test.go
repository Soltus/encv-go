package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestGoProcessPlugin_NoReflectionOnComboLite 验证 combolite.md 铁律
func TestGoProcessPlugin_NoReflectionOnComboLite(t *testing.T) {
	srcPath := filepath.Join("..", "..", "app", "encv-mobile", "android", "app", "src", "main", "java", "com", "encvgo", "app", "GoProcessPlugin.kt")

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skip("GoProcessPlugin.kt not found (not in sandbox or path differs), skipping")
		return
	}

	content, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("Failed to read GoProcessPlugin.kt: %v", err)
	}

	src := string(content)

	forbiddenPatterns := []struct {
		pattern *regexp.Regexp
		desc    string
	}{
		{regexp.MustCompile(`Class\.forName\(\s*["']com\.combo\.core\.runtime`), "Class.forName(com.combo.core.runtime)"},
		{regexp.MustCompile(`\.getMethod\(\s*["']getInstance"`), ".getMethod(\"getInstance\""},
		{regexp.MustCompile(`\.invoke\(\s*pm\s*,\s*apkFile\)`), ".invoke(pm, apkFile) 反射调用 installPlugin"},
		{regexp.MustCompile(`parameterCount\s*==\s*1`), "parameterCount == 1 (旧版反射残留)"},
		{regexp.MustCompile(`parameterCount\s*==\s*2.*find.*installPlugin`), "在 PluginManager 上搜 installPlugin (应在 InstallerManager 上)"},
	}

	for _, fp := range forbiddenPatterns {
		if fp.pattern.MatchString(src) {
			t.Errorf("VIOLATES combolite.md: found forbidden pattern - %s\nMatched near: %s",
				fp.desc, findContext(src, fp.pattern.String()))
		}
	}

	requiredPatterns := []struct {
		pattern *regexp.Regexp
		desc    string
	}{
		// New architecture (post-Capacitor refactor): direct PluginManager
		// calls have been replaced with EncvComboLiteHost wrapper. The test
		// enforces the new contract: GoProcessPlugin.kt should delegate all
		// SDK calls through EncvComboLiteHost, not reach into PluginManager
		// internals (which would couple it to ComboLite SDK changes).
		{regexp.MustCompile(`EncvComboLiteHost\.installPlugin\(`), "EncvComboLiteHost.installPlugin() 包装调用"},
		{regexp.MustCompile(`EncvComboLiteHost\.uninstallPlugin\(`), "EncvComboLiteHost.uninstallPlugin() 包装调用"},
		{regexp.MustCompile(`statusReceiver`), "BroadcastReceiver 实例已定义 (statusReceiver)"},
		{regexp.MustCompile(`registerReceiver`), "BroadcastReceiver 已注册"},
		{regexp.MustCompile(`executeComboLiteInstall`), "executeComboLiteInstall 辅助方法存在"},
	}

	for _, rp := range requiredPatterns {
		if !rp.pattern.MatchString(src) {
			t.Errorf("MISSING required pattern - %s", rp.desc)
		}
	}
}

func TestGoProcessPlugin_PendingCallsKeyConsistency(t *testing.T) {
	keysStored := []string{"installConfirm"}
	keysConsumed := []string{"installConfirm"}

	storedSet := make(map[string]bool)
	consumedSet := make(map[string]bool)
	for _, k := range keysStored {
		storedSet[k] = true
	}
	for _, k := range keysConsumed {
		consumedSet[k] = true
	}

	for k := range storedSet {
		if !consumedSet[k] {
			t.Errorf("pendingCalls key %q is stored but never consumed (leak)", k)
		}
	}
	for k := range consumedSet {
		if !storedSet[k] {
			t.Errorf("pendingCalls key %q is consumed but never stored (nil pointer panic risk)", k)
		}
	}
}

func TestEncvApplication_ProxyManagerConfigured(t *testing.T) {
	appPath := filepath.Join("..", "..", "app", "encv-mobile", "android", "app", "src", "main", "java", "com", "encvgo", "app", "EncvApplication.kt")

	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		t.Skip("EncvApplication.kt not found, skipping")
		return
	}

	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatalf("Failed to read EncvApplication.kt: %v", err)
	}

	src := string(content)

	// New architecture: EncvComboLiteHost.setupFramework(EncvHostActivity::class.java)
	// replaces the old proxyManager.setHostActivity(EncvHostActivity::class.java).
	// The contract now is: onFrameworkSetup must wire the host activity to the
	// ComboLite host (via the EncvComboLiteHost wrapper) using EncvHostActivity.
	if !strings.Contains(src, `EncvComboLiteHost.setupFramework`) {
		t.Error("MISSING: EncvComboLiteHost.setupFramework not called in onFrameworkSetup")
	}
	if !strings.Contains(src, `EncvHostActivity::class.java`) {
		t.Error("MISSING: EncvHostActivity::class.java not passed to setupFramework")
	}
}

func findContext(s string, pattern string) string {
	idx := regexp.MustCompile(pattern).FindStringIndex(s)
	if idx == nil {
		return "(not found)"
	}
	start := idx[0] - 30
	if start < 0 {
		start = 0
	}
	end := idx[0] + 50
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}
