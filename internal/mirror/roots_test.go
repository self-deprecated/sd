package mirror_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/mirror"
)

func TestBuildPlanRejectsSourceRootsThatAreNotDirectories(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "actual", "README.md"), "hello")
	if err := os.Symlink(filepath.Join(root, "actual"), filepath.Join(root, "src")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	write(t, filepath.Join(root, "file"), "not a directory")
	for _, source := range []string{"src", "file", "missing"} {
		_, err := mirror.BuildPlan("app", root, config.MirrorConfig{Source: source}, filepath.Join(t.TempDir(), "mirror"))
		if err == nil || !strings.Contains(err.Error(), "source") {
			t.Errorf("source %s: err = %v, want a source error", source, err)
		}
	}
}

// On case-insensitive file systems (the default on macOS and Windows) a
// differently cased path names the same directory.
func TestBuildPlanRejectsCaseAliasedOverlap(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "Mono")
	write(t, filepath.Join(workspace, "projects", "app", "README.md"), "hello")
	alias := filepath.Join(parent, "mono")
	if _, err := os.Stat(alias); err != nil {
		t.Skip("file system is case-sensitive")
	}
	for _, destination := range []string{alias, filepath.Join(alias, "projects", "app")} {
		_, err := mirror.BuildPlan("app", workspace, config.MirrorConfig{Source: "projects/app"}, destination)
		if err == nil || !strings.Contains(err.Error(), "overlaps") {
			t.Errorf("destination %s: err = %v, want an overlap error", destination, err)
		}
	}
}
