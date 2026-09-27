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
	EncPreviewDir       string
	EncViteBin          string
	AirBin              string
	NodeBin             string
	PythonBin           string
}

func Resolve(repoRoot, mobileDir, mobileDataDir, pluginWebDir, simverseFrontendDir, encPreviewDir, airBin, nodeBin string) *Paths {
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

	if encPreviewDir != "" {
		p.EncPreviewDir = encPreviewDir
	} else {
		p.EncPreviewDir = filepath.Join(p.RepoRoot, "app", "enc-preview")
	}
	p.EncViteBin = firstExisting(
		filepath.Join(p.EncPreviewDir, "node_modules", "vite", "bin", "vite.js"),
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

	if v := os.Getenv("PYTHON_BIN"); v != "" {
		p.PythonBin = v
	} else {
		p.PythonBin = firstExisting("/usr/local/bin/python3", "/usr/bin/python3")
		if p.PythonBin == "" {
			p.PythonBin = "python3"
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
