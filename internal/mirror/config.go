package mirror

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/self-deprecated/sd/internal/config"
)

type ConfigSet struct {
	Mirrors config.MirrorsFile
	Local   config.MirrorLocalFile
}

func LoadConfig(root string) (ConfigSet, error) {
	mf, err := config.ReadMirrors(filepath.Join(root, ".sd", "mirrors.yaml"))
	if err != nil {
		return ConfigSet{}, fmt.Errorf("read .sd/mirrors.yaml: %w", err)
	}
	if mf.Version != 1 {
		return ConfigSet{}, fmt.Errorf("unsupported .sd/mirrors.yaml version %d", mf.Version)
	}
	if mf.Mirrors == nil {
		mf.Mirrors = map[string]config.MirrorConfig{}
	}
	var local config.MirrorLocalFile
	localPath := filepath.Join(root, ".sd", "mirrors.local.yaml")
	if _, err := os.Stat(localPath); err == nil {
		local, err = config.ReadMirrorLocal(localPath)
		if err != nil {
			return ConfigSet{}, fmt.Errorf("read .sd/mirrors.local.yaml: %w", err)
		}
		if local.Version != 1 {
			return ConfigSet{}, fmt.Errorf("unsupported .sd/mirrors.local.yaml version %d", local.Version)
		}
	}
	if local.Paths == nil {
		local.Paths = map[string]string{}
	}
	return ConfigSet{Mirrors: mf, Local: local}, nil
}

func (cs ConfigSet) Mirror(id string) (config.MirrorConfig, bool) {
	cfg, ok := cs.Mirrors.Mirrors[id]
	return cfg, ok
}

func (cs ConfigSet) SortedMirrorIDs() []string {
	ids := make([]string, 0, len(cs.Mirrors.Mirrors))
	for id := range cs.Mirrors.Mirrors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
