package mirror_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/mirror"
)

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func TestBuildPlanRefusesToExportSourceSymlinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	write(t, filepath.Join(root, "private.env"), "TOKEN=private")
	write(t, filepath.Join(source, "README.md"), "hello")
	symlink(t, filepath.Join("..", "private.env"), filepath.Join(source, "notes.txt"))
	if err := os.MkdirAll(filepath.Join(root, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join("..", "shared"), filepath.Join(source, "lib"))

	_, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source}, filepath.Join(root, "mirror"))
	if err == nil || !strings.Contains(err.Error(), "notes.txt") || !strings.Contains(err.Error(), "lib") || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected source symlink error naming notes.txt and lib, got %v", err)
	}
}

func TestBuildPlanIgnoresExcludedSourceSymlinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	write(t, filepath.Join(root, "private.env"), "TOKEN=private")
	write(t, filepath.Join(source, "README.md"), "hello")
	symlink(t, filepath.Join("..", "..", "private.env"), filepath.Join(source, ".tools", "link"))
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Exclude: []string{".tools/**"}}, filepath.Join(root, "mirror"))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	assertOps(t, "files", plan.Files, []string{"README.md"})
}

func TestBuildPlanRefusesDestinationSymlinks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root, dest string)
	}{
		{"file", func(t *testing.T, root, dest string) {
			write(t, filepath.Join(root, "outside.txt"), "outside")
			symlink(t, filepath.Join("..", "outside.txt"), filepath.Join(dest, "notes.txt"))
		}},
		{"parent directory", func(t *testing.T, root, dest string) {
			write(t, filepath.Join(root, "outside", "notes.txt"), "outside")
			if err := os.Remove(filepath.Join(dest, "docs", "notes.txt")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(dest, "docs")); err != nil {
				t.Fatal(err)
			}
			symlink(t, filepath.Join("..", "outside"), filepath.Join(dest, "docs"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "project")
			dest := filepath.Join(root, "mirror")
			write(t, filepath.Join(source, "notes.txt"), "public")
			write(t, filepath.Join(source, "docs", "notes.txt"), "public")
			write(t, filepath.Join(dest, "docs", "notes.txt"), "old")
			tc.setup(t, root, dest)

			plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source}, dest)
			if err == nil {
				_ = mirror.ApplyPlan(plan)
			}
			for _, outside := range []string{filepath.Join(root, "outside.txt"), filepath.Join(root, "outside", "notes.txt")} {
				if data, err := os.ReadFile(outside); err == nil && string(data) != "outside" {
					t.Errorf("%s was overwritten with %q", outside, data)
				}
			}
			if err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("expected destination symlink error, got %v", err)
			}
		})
	}
}

func TestBuildPlanAllowsDestinationSymlinksInPreservedPaths(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	dest := filepath.Join(root, "mirror")
	write(t, filepath.Join(source, "README.md"), "hello")
	write(t, filepath.Join(root, "python"), "binary")
	symlink(t, filepath.Join("..", "..", "..", "python"), filepath.Join(dest, ".venv", "bin", "python"))
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Preserve: []string{".venv/**"}}, dest)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	assertOps(t, "deletes", plan.Deletes, nil)
}

func TestApplyPlanNeverWritesOrDeletesThroughSymlinkedDirectories(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	dest := filepath.Join(root, "mirror")
	outside := filepath.Join(root, "outside")
	write(t, filepath.Join(source, "docs", "new.md"), "public")
	write(t, filepath.Join(dest, "stale", "old.txt"), "stale")
	write(t, filepath.Join(outside, "old.txt"), "keep me")
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source}, dest)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	assertOps(t, "creates", plan.Creates, []string{"docs/new.md"})
	assertOps(t, "deletes", plan.Deletes, []string{"stale/old.txt"})

	// The destination changes between planning and applying.
	if err := os.RemoveAll(filepath.Join(dest, "stale")); err != nil {
		t.Fatal(err)
	}
	symlink(t, outside, filepath.Join(dest, "docs"))
	symlink(t, outside, filepath.Join(dest, "stale"))

	if err := mirror.ApplyPlan(plan); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected ApplyPlan symlink error, got %v", err)
	}
	assertAbsent(t, filepath.Join(outside, "new.md"))
	plan.Creates = nil
	if err := mirror.ApplyPlan(plan); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected ApplyPlan delete symlink error, got %v", err)
	}
	assertFile(t, filepath.Join(outside, "old.txt"), "keep me")
}
