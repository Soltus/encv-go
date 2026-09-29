package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	RepoRoot            string
	MobileDir           string
	MobileDataDir       string
	PluginWebDir        string
	SimverseFrontendDir string
	EncvPreviewDir      string
	EncViteBin          string
	AirBin              string
	NodeBin             string
	// BunBin：预览页的静态服务是 bun + TypeScript（app/encv-preview/serve.ts）。
	// 项目不允许用 python 托管，所以这里不再有 PythonBin。
	BunBin              string
}

func Resolve(repoRoot, mobileDir, mobileDataDir, pluginWebDir, simverseFrontendDir, encvPreviewDir, airBin, nodeBin string) *Paths {
	p := &Paths{
		RepoRoot:      repoRoot,
		MobileDataDir: mobileDataDir,
	}

	if mobileDir != "" {
		p.MobileDir = mobileDir
	} else {
		p.MobileDir = filepath.Join(p.RepoRoot, "app", "encv-mobile")
	}

	if pluginWebDir != "" {
		p.PluginWebDir = pluginWebDir
	} else {
		p.PluginWebDir = filepath.Join(p.RepoRoot, "plugins", "plugin-openlist-web")
	}

	if simverseFrontendDir != "" {
		p.SimverseFrontendDir = simverseFrontendDir
	} else {
		p.SimverseFrontendDir = filepath.Join(p.RepoRoot, "app", "encv-mobile", "plugin-simverse", "web")
	}

	if encvPreviewDir != "" {
		p.EncvPreviewDir = encvPreviewDir
	} else {
		p.EncvPreviewDir = filepath.Join(p.RepoRoot, "app", "encv-preview")
	}
	p.EncViteBin = firstExisting(
		filepath.Join(p.EncvPreviewDir, "node_modules", "vite", "bin", "vite.js"),
		filepath.Join(p.MobileDir, "node_modules", "vite", "bin", "vite.js"),
	)

	if airBin != "" {
		p.AirBin = airBin
	} else {
		p.AirBin = filepath.Join(p.RepoRoot, "bin", "air")
	}

	if nodeBin != "" {
		p.NodeBin = nodeBin
	} else {
		p.NodeBin = "node"
	}

	if v := os.Getenv("BUN_BIN"); v != "" {
		p.BunBin = v
	} else {
		p.BunBin = firstExisting("/usr/local/bin/bun", "/usr/bin/bun")
		if p.BunBin == "" {
			p.BunBin = "bun"
		}
	}

	return p
}

func firstExisting(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	return ""
}

func (p *Paths) Validate() error {
	if _, err := os.Stat(p.MobileDir); os.IsNotExist(err) {
		return fmt.Errorf("mobile dir not found: %s", p.MobileDir)
	}
	return nil
}
