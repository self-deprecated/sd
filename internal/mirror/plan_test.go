package mirror_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/mirror"
)

func TestBuildPlanSelectsFilesAppliesOverlaysAndProtectsVCS(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "projects", "demo")
	dest := filepath.Join(root, "mirrors", "demo")
	write(t, filepath.Join(source, "README.md"), "new readme")
	write(t, filepath.Join(source, "CONTEXT.public.md"), "public context")
	write(t, filepath.Join(source, ".env"), "secret")
	write(t, filepath.Join(source, "node_modules", "pkg", "index.js"), "module")
	write(t, filepath.Join(source, "apps", "mobile", "node_modules", "expo-auth-session"), "nested module")
	write(t, filepath.Join(source, ".jj", "repo"), "private vcs")
	write(t, filepath.Join(dest, "README.md"), "old readme")
	write(t, filepath.Join(dest, "old.txt"), "stale")
	write(t, filepath.Join(dest, "node_modules", "local", "index.js"), "local module cache")
	write(t, filepath.Join(dest, ".jj", "repo"), "mirror vcs")

	cfg := config.MirrorConfig{
		Source:      "projects/demo",
		Destination: config.MirrorDestination{Root: "public-mirrors", Path: "demo"},
		Include:     []string{"**"},
		Exclude:     []string{"node_modules/**", ".env", "CONTEXT.public.md"},
		Preserve:    []string{"node_modules/**"},
		Overlays:    []config.MirrorOverlay{{From: "CONTEXT.public.md", To: "CONTEXT.md"}},
	}
	plan, err := mirror.BuildPlan("demo", root, cfg, dest)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	assertOps(t, "creates", plan.Creates, []string{"CONTEXT.md"})
	assertOps(t, "updates", plan.Updates, []string{"README.md"})
	assertOps(t, "deletes", plan.Deletes, []string{"old.txt"})

	if err := mirror.ApplyPlan(plan); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	assertFile(t, filepath.Join(dest, "README.md"), "new readme")
	assertFile(t, filepath.Join(dest, "CONTEXT.md"), "public context")
	assertFile(t, filepath.Join(dest, ".jj", "repo"), "mirror vcs")
	assertAbsent(t, filepath.Join(dest, ".env"))
	assertAbsent(t, filepath.Join(dest, "node_modules", "pkg", "index.js"))
	assertAbsent(t, filepath.Join(dest, "apps", "mobile", "node_modules", "expo-auth-session"))
	assertFile(t, filepath.Join(dest, "node_modules", "local", "index.js"), "local module cache")
	assertAbsent(t, filepath.Join(dest, "old.txt"))
}

func assertOps(t *testing.T, label string, got []mirror.FileOp, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s len = %d want %d: %#v", label, len(got), len(want), got)
	}
	for i := range want {
		if got[i].Path != want[i] {
			t.Fatalf("%s[%d] = %q want %q", label, i, got[i].Path, want[i])
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q want %q", path, string(data), want)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists or stat failed with non-ENOENT: %v", path, err)
	}
}

func TestBuildPlanAppliesRecursiveExcludes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	write(t, filepath.Join(source, "a", "private", "secret.txt"), "secret")
	write(t, filepath.Join(source, "private", "top.txt"), "secret")
	// A directory matching an exclude is pruned with everything below it.
	write(t, filepath.Join(source, "a", "private", "deeper", "pruned.txt"), "secret")
	write(t, filepath.Join(source, "a", "public.txt"), "public")
	cfg := config.MirrorConfig{Source: source, Exclude: []string{"**/private/*"}}
	plan, err := mirror.BuildPlan("app", root, cfg, filepath.Join(root, "mirror"))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	assertOps(t, "files", plan.Files, []string{"a/public.txt"})
}

func TestBuildPlanRejectsMalformedPatterns(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	write(t, filepath.Join(source, "README.md"), "hello")
	for _, cfg := range []config.MirrorConfig{
		{Source: source, Include: []string{"[abc"}},
		{Source: source, Exclude: []string{"docs/[z-a"}},
		{Source: source, Preserve: []string{"build**"}},
	} {
		_, err := mirror.BuildPlan("app", root, cfg, filepath.Join(root, "mirror"))
		if err == nil || !strings.Contains(err.Error(), "pattern") {
			t.Errorf("BuildPlan(%+v) error = %v, want pattern error", cfg, err)
		}
	}
}
