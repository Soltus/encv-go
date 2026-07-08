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
	AirBin              string
	NodeBin             string
}

func Resolve(repoRoot, mobileDir, mobileDataDir, pluginWebDir, simverseFrontendDir, airBin, nodeBin string) *Paths {
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

	return p
}

func (p *Paths) Validate() error {
	if _, err := os.Stat(p.MobileDir); os.IsNotExist(err) {
		return fmt.Errorf("mobile dir not found: %s", p.MobileDir)
	}
	return nil
}
