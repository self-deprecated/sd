package mirror_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/mirror"
)

func TestLoadConfigRejectsUnsupportedVersions(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "mirrors.yaml"), `version: 2
mirrors: {}
`)
	_, err := mirror.LoadConfig(root)
	if err == nil || !strings.Contains(err.Error(), "unsupported .sd/mirrors.yaml version 2") {
		t.Fatalf("expected unsupported mirrors version error, got %v", err)
	}

	root = t.TempDir()
	write(t, filepath.Join(root, ".sd", "mirrors.yaml"), `version: 1
mirrors: {}
`)
	write(t, filepath.Join(root, ".sd", "mirrors.local.yaml"), `version: 2
paths: {}
`)
	_, err = mirror.LoadConfig(root)
	if err == nil || !strings.Contains(err.Error(), "unsupported .sd/mirrors.local.yaml version 2") {
		t.Fatalf("expected unsupported local version error, got %v", err)
	}
}
